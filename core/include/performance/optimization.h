#ifndef OMS_PERFORMANCE_OPTIMIZATION_H
#define OMS_PERFORMANCE_OPTIMIZATION_H

#include <cstddef>
#include <cstdint>
#include <atomic>
#include <immintrin.h>
#include <memory>
#include <vector>
#include <array>
#include <functional>

namespace oms::performance {

// Cache line size (typically 64 bytes on modern x86_64 processors)
constexpr size_t CACHE_LINE_SIZE = 64;

// Alignment macros
#define CACHE_ALIGNED alignas(CACHE_LINE_SIZE)
#define PREFETCH_READ(addr) __builtin_prefetch(addr, 0, 3)
#define PREFETCH_WRITE(addr) __builtin_prefetch(addr, 1, 3)
#define LIKELY(x) __builtin_expect(!!(x), 1)
#define UNLIKELY(x) __builtin_expect(!!(x), 0)

// Force inline macro
#ifdef __GNUC__
#define FORCE_INLINE __attribute__((always_inline)) inline
#else
#define FORCE_INLINE inline
#endif

// Memory barrier operations
FORCE_INLINE void memory_fence_acquire() {
    std::atomic_thread_fence(std::memory_order_acquire);
}

FORCE_INLINE void memory_fence_release() {
    std::atomic_thread_fence(std::memory_order_release);
}

FORCE_INLINE void memory_fence_seq_cst() {
    std::atomic_thread_fence(std::memory_order_seq_cst);
}

// CPU pause instruction for spinlock optimization
FORCE_INLINE void cpu_pause() {
    _mm_pause();
}

// Lock-free queue optimized for single producer, single consumer
template<typename T, size_t Size>
class alignas(CACHE_LINE_SIZE) SPSCQueue {
    static_assert((Size & (Size - 1)) == 0, "Size must be power of 2");
    
private:
    struct Node {
        CACHE_ALIGNED std::atomic<T> data;
    };
    
    CACHE_ALIGNED std::array<Node, Size> buffer_;
    CACHE_ALIGNED std::atomic<size_t> write_pos_{0};
    CACHE_ALIGNED std::atomic<size_t> read_pos_{0};
    
    static constexpr size_t MASK = Size - 1;
    
public:
    SPSCQueue() = default;
    
    FORCE_INLINE bool try_push(const T& item) {
        const size_t write = write_pos_.load(std::memory_order_relaxed);
        const size_t next_write = (write + 1) & MASK;
        
        if (UNLIKELY(next_write == read_pos_.load(std::memory_order_acquire))) {
            return false;
        }
        
        buffer_[write].data.store(item, std::memory_order_release);
        write_pos_.store(next_write, std::memory_order_release);
        return true;
    }
    
    FORCE_INLINE bool try_pop(T& item) {
        const size_t read = read_pos_.load(std::memory_order_relaxed);
        
        if (UNLIKELY(read == write_pos_.load(std::memory_order_acquire))) {
            return false;
        }
        
        item = buffer_[read].data.load(std::memory_order_relaxed);
        read_pos_.store((read + 1) & MASK, std::memory_order_release);
        return true;
    }
    
    FORCE_INLINE size_t size() const {
        const size_t write = write_pos_.load(std::memory_order_acquire);
        const size_t read = read_pos_.load(std::memory_order_acquire);
        return (write - read) & MASK;
    }
    
    FORCE_INLINE bool empty() const {
        return read_pos_.load(std::memory_order_acquire) == 
               write_pos_.load(std::memory_order_acquire);
    }
};

// Memory pool allocator with lock-free operations
template<typename T>
class MemoryPool {
private:
    struct Block {
        CACHE_ALIGNED std::atomic<Block*> next;
        alignas(alignof(T)) char data[sizeof(T)];
    };
    
    CACHE_ALIGNED std::atomic<Block*> free_list_{nullptr};
    std::vector<std::unique_ptr<Block[]>> chunks_;
    const size_t chunk_size_;
    std::atomic<size_t> allocated_count_{0};
    
public:
    explicit MemoryPool(size_t initial_size = 1024, size_t chunk_size = 1024) 
        : chunk_size_(chunk_size) {
        grow_pool(initial_size);
    }
    
    template<typename... Args>
    T* allocate(Args&&... args) {
        Block* block = free_list_.load(std::memory_order_acquire);
        
        while (block) {
            Block* next = block->next.load(std::memory_order_relaxed);
            if (free_list_.compare_exchange_weak(block, next, 
                                                std::memory_order_release,
                                                std::memory_order_acquire)) {
                T* obj = new (block->data) T(std::forward<Args>(args)...);
                allocated_count_.fetch_add(1, std::memory_order_relaxed);
                return obj;
            }
            block = free_list_.load(std::memory_order_acquire);
        }
        
        // Need to grow the pool
        grow_pool(chunk_size_);
        return allocate(std::forward<Args>(args)...);
    }
    
    void deallocate(T* ptr) {
        if (!ptr) return;
        
        ptr->~T();
        Block* block = reinterpret_cast<Block*>(
            reinterpret_cast<char*>(ptr) - offsetof(Block, data));
        
        Block* old_head = free_list_.load(std::memory_order_relaxed);
        do {
            block->next.store(old_head, std::memory_order_relaxed);
        } while (!free_list_.compare_exchange_weak(old_head, block,
                                                  std::memory_order_release,
                                                  std::memory_order_relaxed));
        
        allocated_count_.fetch_sub(1, std::memory_order_relaxed);
    }
    
    size_t allocated() const {
        return allocated_count_.load(std::memory_order_relaxed);
    }
    
private:
    void grow_pool(size_t size) {
        auto chunk = std::make_unique<Block[]>(size);
        
        for (size_t i = 0; i < size - 1; ++i) {
            chunk[i].next.store(&chunk[i + 1], std::memory_order_relaxed);
        }
        chunk[size - 1].next.store(nullptr, std::memory_order_relaxed);
        
        // Add to free list
        Block* old_head = free_list_.load(std::memory_order_relaxed);
        do {
            chunk[size - 1].next.store(old_head, std::memory_order_relaxed);
        } while (!free_list_.compare_exchange_weak(old_head, &chunk[0],
                                                  std::memory_order_release,
                                                  std::memory_order_relaxed));
        
        chunks_.push_back(std::move(chunk));
    }
};

// Branch prediction hints
template<typename T>
FORCE_INLINE T select_likely(bool condition, T true_val, T false_val) {
    return LIKELY(condition) ? true_val : false_val;
}

template<typename T>
FORCE_INLINE T select_unlikely(bool condition, T true_val, T false_val) {
    return UNLIKELY(condition) ? true_val : false_val;
}

// Zero-copy buffer view
template<typename T>
class BufferView {
private:
    const T* data_;
    size_t size_;
    
public:
    BufferView(const T* data, size_t size) : data_(data), size_(size) {}
    
    FORCE_INLINE const T* data() const { return data_; }
    FORCE_INLINE size_t size() const { return size_; }
    FORCE_INLINE const T& operator[](size_t idx) const { 
        return data_[idx]; 
    }
    
    FORCE_INLINE const T* begin() const { return data_; }
    FORCE_INLINE const T* end() const { return data_ + size_; }
};

// CPU affinity settings
class CPUAffinity {
public:
    static bool set_thread_affinity(int cpu_id);
    static bool set_numa_node(int numa_node);
    static int get_numa_node_for_cpu(int cpu_id);
    static std::vector<int> get_online_cpus();
    static int get_current_cpu();
};

// SIMD operations wrapper
class SIMDOps {
public:
    // Vector operations for price calculations
    static void add_prices_avx2(const double* a, const double* b, double* result, size_t count);
    static void multiply_prices_avx2(const double* prices, double multiplier, double* result, size_t count);
    static double sum_prices_avx2(const double* prices, size_t count);
    static void min_max_prices_avx2(const double* prices, size_t count, double& min_val, double& max_val);
    
    // Vector comparisons
    static void compare_prices_avx2(const double* a, const double* b, bool* result, size_t count);
    
    // Check CPU capabilities
    static bool has_avx2();
    static bool has_avx512();
};

} // namespace oms::performance

#endif // OMS_PERFORMANCE_OPTIMIZATION_H