#include "performance/optimization.h"
#include <cstdlib>
#include <new>
#include <unistd.h>
#include <mutex>
#include <random>

#ifdef __linux__
#include <sys/mman.h>
#ifdef HAS_NUMA
#include <numa.h>
#endif
#endif

namespace oms::performance {

// Additional memory pool implementations and utilities

// Aligned memory allocation
void* aligned_alloc_wrapper(size_t alignment, size_t size) {
#ifdef _WIN32
    return _aligned_malloc(size, alignment);
#else
    void* ptr = nullptr;
    if (posix_memalign(&ptr, alignment, size) != 0) {
        return nullptr;
    }
    return ptr;
#endif
}

void aligned_free_wrapper(void* ptr) {
#ifdef _WIN32
    _aligned_free(ptr);
#else
    free(ptr);
#endif
}

// Page-aligned memory allocator for large buffers
class PageAlignedAllocator {
private:
    static constexpr size_t PAGE_SIZE = 4096;
    
public:
    static void* allocate(size_t size) {
#ifdef __linux__
        // Use mmap for large allocations to get page-aligned memory
        void* ptr = mmap(nullptr, size, PROT_READ | PROT_WRITE, 
                        MAP_PRIVATE | MAP_ANONYMOUS | MAP_POPULATE, -1, 0);
        
        if (ptr == MAP_FAILED) {
            throw std::bad_alloc();
        }
        
        // Advise kernel about memory usage pattern
        madvise(ptr, size, MADV_SEQUENTIAL | MADV_WILLNEED);
        
        return ptr;
#else
        return aligned_alloc_wrapper(PAGE_SIZE, size);
#endif
    }
    
    static void deallocate(void* ptr, size_t size) {
#ifdef __linux__
        munmap(ptr, size);
#else
        aligned_free_wrapper(ptr);
#endif
    }
    
    static void prefault_memory(void* ptr, size_t size) {
        // Touch each page to ensure it's mapped
        volatile char* mem = static_cast<volatile char*>(ptr);
        for (size_t i = 0; i < size; i += PAGE_SIZE) {
            mem[i] = 0;
        }
    }
};

// Fixed-size object pool with NUMA awareness
template<typename T>
class NUMAObjectPool {
private:
    struct PoolChunk {
        void* memory;
        size_t size;
        int numa_node;
    };
    
    std::vector<PoolChunk> chunks_;
    MemoryPool<T> pools_[8]; // Support up to 8 NUMA nodes
    int num_numa_nodes_;
    
public:
    NUMAObjectPool(size_t initial_size = 1024) {
#if defined(__linux__) && defined(HAS_NUMA)
        if (numa_available() >= 0) {
            num_numa_nodes_ = numa_num_configured_nodes();
        } else {
            num_numa_nodes_ = 1;
        }
#else
        num_numa_nodes_ = 1;
#endif
        
        // Initialize pools for each NUMA node
        for (int i = 0; i < num_numa_nodes_; ++i) {
            allocate_numa_chunk(i, initial_size);
        }
    }
    
    ~NUMAObjectPool() {
        for (const auto& chunk : chunks_) {
#if defined(__linux__) && defined(HAS_NUMA)
            numa_free(chunk.memory, chunk.size);
#else
            free(chunk.memory);
#endif
        }
    }
    
    template<typename... Args>
    T* allocate_on_node(int numa_node, Args&&... args) {
        if (numa_node < 0 || numa_node >= num_numa_nodes_) {
            numa_node = 0;
        }
        
        return pools_[numa_node].allocate(std::forward<Args>(args)...);
    }
    
    template<typename... Args>
    T* allocate_local(Args&&... args) {
        int numa_node = 0;
#if defined(__linux__) && defined(HAS_NUMA)
        if (numa_available() >= 0) {
            numa_node = numa_node_of_cpu(sched_getcpu());
        }
#endif
        return allocate_on_node(numa_node, std::forward<Args>(args)...);
    }
    
    void deallocate(T* ptr) {
        // Find which pool owns this object
        for (int i = 0; i < num_numa_nodes_; ++i) {
            pools_[i].deallocate(ptr);
        }
    }
    
private:
    void allocate_numa_chunk(int numa_node, size_t count) {
        size_t size = count * sizeof(T);
        void* memory = nullptr;
        
#if defined(__linux__) && defined(HAS_NUMA)
        if (numa_available() >= 0) {
            memory = numa_alloc_onnode(size, numa_node);
        } else {
            memory = malloc(size);
        }
#else
        memory = malloc(size);
#endif
        
        if (memory) {
            chunks_.push_back({memory, size, numa_node});
            
            // Initialize free list for this chunk
            T* objects = static_cast<T*>(memory);
            for (size_t i = 0; i < count; ++i) {
                pools_[numa_node].deallocate(&objects[i]);
            }
        }
    }
};

// Lock-free stack for memory recycling
template<typename T>
class LockFreeStack {
private:
    struct Node {
        T data;
        std::atomic<Node*> next;
        
        template<typename... Args>
        Node(Args&&... args) : data(std::forward<Args>(args)...), next(nullptr) {}
    };
    
    CACHE_ALIGNED std::atomic<Node*> head_{nullptr};
    CACHE_ALIGNED std::atomic<size_t> size_{0};
    
public:
    ~LockFreeStack() {
        Node* current = head_.load();
        while (current) {
            Node* next = current->next.load();
            delete current;
            current = next;
        }
    }
    
    void push(T&& value) {
        Node* new_node = new Node(std::move(value));
        Node* old_head = head_.load(std::memory_order_relaxed);
        
        do {
            new_node->next.store(old_head, std::memory_order_relaxed);
        } while (!head_.compare_exchange_weak(old_head, new_node,
                                             std::memory_order_release,
                                             std::memory_order_relaxed));
        
        size_.fetch_add(1, std::memory_order_relaxed);
    }
    
    bool pop(T& value) {
        Node* old_head = head_.load(std::memory_order_acquire);
        
        while (old_head) {
            Node* next = old_head->next.load(std::memory_order_relaxed);
            
            if (head_.compare_exchange_weak(old_head, next,
                                           std::memory_order_release,
                                           std::memory_order_acquire)) {
                value = std::move(old_head->data);
                delete old_head;
                size_.fetch_sub(1, std::memory_order_relaxed);
                return true;
            }
        }
        
        return false;
    }
    
    size_t size() const {
        return size_.load(std::memory_order_relaxed);
    }
    
    bool empty() const {
        return head_.load(std::memory_order_acquire) == nullptr;
    }
};

// Slab allocator for fixed-size objects
template<typename T>
class SlabAllocator {
private:
    static constexpr size_t SLAB_SIZE = 64 * 1024; // 64KB slabs
    static constexpr size_t OBJECTS_PER_SLAB = SLAB_SIZE / sizeof(T);
    
    struct Slab {
        CACHE_ALIGNED std::atomic<size_t> allocated_mask{0};
        alignas(alignof(T)) char data[SLAB_SIZE];
        
        T* get_object(size_t index) {
            return reinterpret_cast<T*>(&data[index * sizeof(T)]);
        }
    };
    
    std::vector<std::unique_ptr<Slab>> slabs_;
    CACHE_ALIGNED std::atomic<size_t> current_slab_{0};
    
public:
    SlabAllocator() {
        slabs_.reserve(16);
        slabs_.emplace_back(std::make_unique<Slab>());
    }
    
    template<typename... Args>
    T* allocate(Args&&... args) {
        while (true) {
            size_t slab_idx = current_slab_.load(std::memory_order_acquire);
            
            if (slab_idx >= slabs_.size()) {
                // Need to allocate new slab
                add_slab();
                continue;
            }
            
            Slab* slab = slabs_[slab_idx].get();
            size_t mask = slab->allocated_mask.load(std::memory_order_acquire);
            
            // Find first free slot
            for (size_t i = 0; i < OBJECTS_PER_SLAB; ++i) {
                if (!(mask & (1ULL << i))) {
                    // Try to allocate this slot
                    size_t new_mask = mask | (1ULL << i);
                    if (slab->allocated_mask.compare_exchange_weak(mask, new_mask,
                                                                   std::memory_order_release,
                                                                   std::memory_order_acquire)) {
                        T* obj = slab->get_object(i);
                        new (obj) T(std::forward<Args>(args)...);
                        return obj;
                    }
                }
            }
            
            // Slab is full, try next one
            current_slab_.compare_exchange_weak(slab_idx, slab_idx + 1,
                                               std::memory_order_release,
                                               std::memory_order_relaxed);
        }
    }
    
    void deallocate(T* ptr) {
        if (!ptr) return;
        
        // Find which slab owns this object
        for (size_t slab_idx = 0; slab_idx < slabs_.size(); ++slab_idx) {
            Slab* slab = slabs_[slab_idx].get();
            char* slab_start = slab->data;
            char* slab_end = slab_start + SLAB_SIZE;
            char* obj_ptr = reinterpret_cast<char*>(ptr);
            
            if (obj_ptr >= slab_start && obj_ptr < slab_end) {
                // Found the slab
                size_t obj_idx = (obj_ptr - slab_start) / sizeof(T);
                
                ptr->~T();
                
                // Clear the allocation bit
                size_t mask = slab->allocated_mask.load(std::memory_order_acquire);
                size_t new_mask = mask & ~(1ULL << obj_idx);
                while (!slab->allocated_mask.compare_exchange_weak(mask, new_mask,
                                                                   std::memory_order_release,
                                                                   std::memory_order_acquire)) {
                    new_mask = mask & ~(1ULL << obj_idx);
                }
                
                // Update current slab if this one has free space
                size_t current = current_slab_.load(std::memory_order_acquire);
                if (current > slab_idx) {
                    current_slab_.compare_exchange_weak(current, slab_idx,
                                                       std::memory_order_release,
                                                       std::memory_order_relaxed);
                }
                
                return;
            }
        }
    }
    
private:
    void add_slab() {
        static std::mutex slab_mutex;
        std::lock_guard<std::mutex> lock(slab_mutex);
        
        // Double-check after acquiring lock
        if (current_slab_.load(std::memory_order_acquire) >= slabs_.size()) {
            slabs_.emplace_back(std::make_unique<Slab>());
        }
    }
};

} // namespace oms::performance