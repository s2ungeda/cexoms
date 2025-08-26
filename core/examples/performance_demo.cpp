#include "performance/optimization.h"
#include "performance/profile.h"
#include "types.h"
#include <iostream>
#include <vector>
#include <chrono>
#include <iomanip>

using namespace oms;
using namespace oms::performance;

// Simplified order structure for lock-free queue (trivially copyable)
struct FastOrder {
    OrderId id;
    uint32_t symbol_hash;  // Pre-hashed symbol
    Price price;
    Quantity quantity;
    Side side;
    OrderType type;
    uint8_t padding[3];  // Align to 32 bytes
};

static_assert(std::is_trivially_copyable_v<FastOrder>, "FastOrder must be trivially copyable");

// Example: High-performance order processor using optimization features
class OptimizedOrderProcessor {
private:
    // Lock-free queue for order processing
    SPSCQueue<FastOrder, 65536> order_queue_;
    
    // Memory pool for order allocation
    MemoryPool<Order> order_pool_;
    
    // Pre-allocated price arrays for SIMD operations
    std::vector<double> bid_prices_;
    std::vector<double> ask_prices_;
    std::vector<double> spreads_;
    
    // Performance stats
    std::atomic<size_t> orders_processed_{0};
    
public:
    OptimizedOrderProcessor(size_t max_symbols = 10000) 
        : order_pool_(1024),
          bid_prices_(max_symbols),
          ask_prices_(max_symbols),
          spreads_(max_symbols) {
        
        // Set thread affinity to dedicated CPU core
        auto cpus = CPUAffinity::get_online_cpus();
        if (cpus.size() > 1) {
            CPUAffinity::set_thread_affinity(cpus[1]);
            std::cout << "OrderProcessor thread pinned to CPU " << cpus[1] << std::endl;
        }
    }
    
    // Submit order (producer side)
    bool submit_order(const FastOrder& order) {
        PROFILE_SCOPE("SubmitOrder");
        
        // Use lock-free queue for submission
        return order_queue_.try_push(order);
    }
    
    // Process orders (consumer side)
    void process_orders() {
        PROFILE_SCOPE("ProcessOrders");
        
        FastOrder order;
        size_t batch_count = 0;
        
        // Process in batches for better cache efficiency
        while (order_queue_.try_pop(order)) {
            // Simulate order processing
            process_single_order(order);
            batch_count++;
            
            // Prefetch next queue elements
            if (batch_count % 16 == 0) {
                PREFETCH_READ(&order_queue_);
            }
        }
        
        orders_processed_ += batch_count;
    }
    
    // Calculate spreads using SIMD
    void calculate_spreads(size_t count) {
        PROFILE_SCOPE("CalculateSpreads");
        
        // Initialize test data
        for (size_t i = 0; i < count; ++i) {
            bid_prices_[i] = 100.0 + (i % 10) * 0.1;
            ask_prices_[i] = bid_prices_[i] + 0.05;
        }
        
        // Use SIMD to calculate spreads
        if (SIMDOps::has_avx2()) {
            // Calculate ask - bid for all symbols
            for (size_t i = 0; i < count; i += 4) {
                __m256d bids = _mm256_loadu_pd(&bid_prices_[i]);
                __m256d asks = _mm256_loadu_pd(&ask_prices_[i]);
                __m256d spread = _mm256_sub_pd(asks, bids);
                _mm256_storeu_pd(&spreads_[i], spread);
            }
        } else {
            // Fallback to scalar
            for (size_t i = 0; i < count; ++i) {
                spreads_[i] = ask_prices_[i] - bid_prices_[i];
            }
        }
        
        // Find min/max spreads using SIMD
        double min_spread, max_spread;
        SIMDOps::min_max_prices_avx2(spreads_.data(), count, min_spread, max_spread);
        
        std::cout << "Spread range: [" << std::fixed << std::setprecision(6) 
                  << min_spread << ", " << max_spread << "]" << std::endl;
    }
    
    // Demonstrate memory pool usage
    void test_memory_pool() {
        PROFILE_SCOPE("MemoryPoolTest");
        
        std::vector<Order*> orders;
        const size_t test_count = 10000;
        
        // Allocate orders from pool
        auto start = std::chrono::high_resolution_clock::now();
        for (size_t i = 0; i < test_count; ++i) {
            Order* order = order_pool_.allocate();
            order->id = i;
            order->symbol = "TEST";
            order->price = 100.0 + (i % 100) * 0.01;
            order->quantity = 1.0 + (i % 10);
            order->side = (i % 2) ? Side::BUY : Side::SELL;
            order->type = OrderType::LIMIT;
            order->status = OrderStatus::NEW;
            order->time_in_force = TimeInForce::GTC;
            orders.push_back(order);
        }
        auto alloc_time = std::chrono::high_resolution_clock::now() - start;
        
        // Deallocate back to pool
        start = std::chrono::high_resolution_clock::now();
        for (auto* order : orders) {
            order_pool_.deallocate(order);
        }
        auto dealloc_time = std::chrono::high_resolution_clock::now() - start;
        
        std::cout << "Memory pool performance:" << std::endl;
        std::cout << "  Allocation: " << test_count << " orders in " 
                  << std::chrono::duration_cast<std::chrono::microseconds>(alloc_time).count() 
                  << " μs" << std::endl;
        std::cout << "  Deallocation: " << test_count << " orders in " 
                  << std::chrono::duration_cast<std::chrono::microseconds>(dealloc_time).count() 
                  << " μs" << std::endl;
        std::cout << "  Currently allocated: " << order_pool_.allocated() << std::endl;
    }
    
    void print_stats() {
        std::cout << "\nPerformance Statistics:" << std::endl;
        std::cout << "  Orders processed: " << orders_processed_.load() << std::endl;
        
        // Print profiling stats
        auto profile_names = Profiler::instance().get_profile_names();
        for (const auto& name : profile_names) {
            auto stats = Profiler::instance().get_stats(name);
            std::cout << "  " << name << ": " 
                      << "mean=" << stats.mean() / 1000.0 << " μs, "
                      << "count=" << stats.count << std::endl;
        }
    }
    
private:
    void process_single_order(const FastOrder& order) {
        // Simulate order processing with branch prediction hints
        if (LIKELY(order.type == OrderType::LIMIT)) {
            // Most orders are limit orders
            validate_limit_order(order);
        } else if (UNLIKELY(order.type == OrderType::MARKET)) {
            // Market orders are less common
            validate_market_order(order);
        }
    }
    
    void validate_limit_order(const FastOrder& order) {
        // Validation logic
        (void)order;  // Suppress unused warning
        cpu_pause(); // Simulate work
    }
    
    void validate_market_order(const FastOrder& order) {
        // Validation logic
        (void)order;  // Suppress unused warning
        cpu_pause(); // Simulate work
    }
};

int main() {
    std::cout << "=== OMS Performance Optimization Demo ===" << std::endl;
    
    // Enable profiling
    Profiler::instance().enable();
    
    // Create optimized order processor
    OptimizedOrderProcessor processor;
    
    // Test memory pool
    processor.test_memory_pool();
    
    // Test SIMD operations
    processor.calculate_spreads(1000);
    
    // Submit test orders
    std::cout << "\nSubmitting orders..." << std::endl;
    uint32_t btc_hash = std::hash<std::string>{}("BTC-USDT");
    
    for (int i = 0; i < 10000; ++i) {
        FastOrder order;
        order.id = i;
        order.symbol_hash = btc_hash;
        order.price = 50000.0 + (i % 100);
        order.quantity = 0.1 * (1 + i % 10);
        order.side = (i % 2) ? Side::BUY : Side::SELL;
        order.type = (i % 10 == 0) ? OrderType::MARKET : OrderType::LIMIT;
        
        while (!processor.submit_order(order)) {
            cpu_pause();
        }
    }
    
    // Process orders
    std::cout << "Processing orders..." << std::endl;
    processor.process_orders();
    
    // Print statistics
    processor.print_stats();
    
    return 0;
}