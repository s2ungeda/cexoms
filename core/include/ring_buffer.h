#ifndef OMS_RING_BUFFER_H
#define OMS_RING_BUFFER_H

#include <atomic>
#include <cstddef>
#include <cstring>
#include <memory>
#include <new>

namespace oms {

template <typename T>
class RingBuffer {
private:
    static constexpr size_t CACHE_LINE_SIZE = 64;
    
    struct alignas(CACHE_LINE_SIZE) {
        std::atomic<size_t> head{0};
    } producer_;
    
    struct alignas(CACHE_LINE_SIZE) {
        std::atomic<size_t> tail{0};
    } consumer_;
    
    std::unique_ptr<T[]> buffer_;
    const size_t capacity_;
    const size_t mask_;
    
    static size_t next_power_of_two(size_t n) {
        n--;
        n |= n >> 1;
        n |= n >> 2;
        n |= n >> 4;
        n |= n >> 8;
        n |= n >> 16;
        n |= n >> 32;
        n++;
        return n;
    }
    
public:
    explicit RingBuffer(size_t capacity)
        : buffer_(std::make_unique<T[]>(next_power_of_two(capacity)))
        , capacity_(next_power_of_two(capacity))
        , mask_(capacity_ - 1) {
    }
    
    bool push(const T& item) {
        const auto current_head = producer_.head.load(std::memory_order_relaxed);
        const auto next_head = (current_head + 1) & mask_;
        
        if (next_head == consumer_.tail.load(std::memory_order_acquire)) {
            return false; // Buffer full
        }
        
        buffer_[current_head] = item;
        producer_.head.store(next_head, std::memory_order_release);
        return true;
    }
    
    bool pop(T& item) {
        const auto current_tail = consumer_.tail.load(std::memory_order_relaxed);
        
        if (current_tail == producer_.head.load(std::memory_order_acquire)) {
            return false; // Buffer empty
        }
        
        item = buffer_[current_tail];
        consumer_.tail.store((current_tail + 1) & mask_, std::memory_order_release);
        return true;
    }
    
    size_t size() const {
        const auto head = producer_.head.load(std::memory_order_acquire);
        const auto tail = consumer_.tail.load(std::memory_order_acquire);
        return (head - tail) & mask_;
    }
    
    bool empty() const {
        return producer_.head.load(std::memory_order_acquire) == 
               consumer_.tail.load(std::memory_order_acquire);
    }
    
    size_t capacity() const {
        return capacity_;
    }
};

} // namespace oms

#endif // OMS_RING_BUFFER_H