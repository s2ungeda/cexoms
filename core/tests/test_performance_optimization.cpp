#include "performance/optimization.h"
#include "performance/profile.h"
#include "types.h"
#include <iostream>
#include <vector>
#include <random>
#include <iomanip>
#include <thread>

using namespace oms::performance;
using namespace oms;

// Test structures
struct TestOrder {
    OrderId id;
    Price price;
    Quantity quantity;
    Side side;
    
    TestOrder() = default;
    TestOrder(OrderId _id, Price _price, Quantity _qty, Side _side)
        : id(_id), price(_price), quantity(_qty), side(_side) {}
};

void test_spsc_queue() {
    std::cout << "\n=== Testing SPSC Queue ===" << std::endl;
    
    SPSCQueue<TestOrder, 1024> queue;
    
    // Producer thread
    std::thread producer([&queue]() {
        for (int i = 0; i < 1000; ++i) {
            TestOrder order{
                static_cast<OrderId>(i),
                100.0 + i * 0.01,
                static_cast<Quantity>(i % 100 + 1),
                (i % 2) ? Side::BUY : Side::SELL
            };
            
            while (!queue.try_push(order)) {
                cpu_pause();
            }
        }
    });
    
    // Consumer thread
    size_t consumed = 0;
    std::thread consumer([&queue, &consumed]() {
        TestOrder order;
        while (consumed < 1000) {
            if (queue.try_pop(order)) {
                consumed++;
            } else {
                cpu_pause();
            }
        }
    });
    
    producer.join();
    consumer.join();
    
    std::cout << "Successfully produced and consumed 1000 orders" << std::endl;
}

void test_memory_pool() {
    std::cout << "\n=== Testing Memory Pool ===" << std::endl;
    
    MemoryPool<TestOrder> pool(256);
    std::vector<TestOrder*> orders;
    
    // Allocate orders
    auto start = std::chrono::high_resolution_clock::now();
    for (int i = 0; i < 1000; ++i) {
        TestOrder* order = pool.allocate(
            static_cast<OrderId>(i),
            100.0 + i * 0.01,
            static_cast<Quantity>(i % 100 + 1),
            (i % 2) ? Side::BUY : Side::SELL
        );
        orders.push_back(order);
    }
    auto alloc_time = std::chrono::high_resolution_clock::now() - start;
    
    std::cout << "Allocated 1000 orders in " 
              << std::chrono::duration_cast<std::chrono::microseconds>(alloc_time).count() 
              << " μs" << std::endl;
    
    // Deallocate half
    for (size_t i = 0; i < orders.size(); i += 2) {
        pool.deallocate(orders[i]);
        orders[i] = nullptr;
    }
    
    // Reallocate
    start = std::chrono::high_resolution_clock::now();
    for (size_t i = 0; i < orders.size(); i += 2) {
        orders[i] = pool.allocate(
            static_cast<OrderId>(i),
            200.0 + i * 0.01,
            static_cast<Quantity>(i % 50 + 1),
            Side::BUY
        );
    }
    auto realloc_time = std::chrono::high_resolution_clock::now() - start;
    
    std::cout << "Reallocated 500 orders in " 
              << std::chrono::duration_cast<std::chrono::microseconds>(realloc_time).count() 
              << " μs" << std::endl;
    
    // Cleanup
    for (auto* order : orders) {
        if (order) pool.deallocate(order);
    }
}

void test_simd_operations() {
    std::cout << "\n=== Testing SIMD Operations ===" << std::endl;
    
    const size_t count = 10000;
    std::vector<double> prices_a(count);
    std::vector<double> prices_b(count);
    std::vector<double> result(count);
    
    // Initialize with random prices
    std::random_device rd;
    std::mt19937 gen(rd());
    std::uniform_real_distribution<double> dis(90.0, 110.0);
    
    for (size_t i = 0; i < count; ++i) {
        prices_a[i] = dis(gen);
        prices_b[i] = dis(gen);
    }
    
    // Test AVX2 capabilities
    std::cout << "AVX2 support: " << (SIMDOps::has_avx2() ? "Yes" : "No") << std::endl;
    std::cout << "AVX512 support: " << (SIMDOps::has_avx512() ? "Yes" : "No") << std::endl;
    
    // Benchmark addition
    {
        PROFILE_SCOPE("SIMD Add");
        SIMDOps::add_prices_avx2(prices_a.data(), prices_b.data(), result.data(), count);
    }
    
    // Benchmark multiplication
    {
        PROFILE_SCOPE("SIMD Multiply");
        SIMDOps::multiply_prices_avx2(prices_a.data(), 1.05, result.data(), count);
    }
    
    // Benchmark sum
    double sum;
    {
        PROFILE_SCOPE("SIMD Sum");
        sum = SIMDOps::sum_prices_avx2(prices_a.data(), count);
    }
    std::cout << "Sum of prices: " << std::fixed << std::setprecision(2) << sum << std::endl;
    
    // Benchmark min/max
    double min_val, max_val;
    {
        PROFILE_SCOPE("SIMD MinMax");
        SIMDOps::min_max_prices_avx2(prices_a.data(), count, min_val, max_val);
    }
    std::cout << "Min price: " << min_val << ", Max price: " << max_val << std::endl;
}

void test_cpu_affinity() {
    std::cout << "\n=== Testing CPU Affinity ===" << std::endl;
    
    auto cpus = CPUAffinity::get_online_cpus();
    std::cout << "Online CPUs: ";
    for (int cpu : cpus) {
        std::cout << cpu << " ";
    }
    std::cout << std::endl;
    
    int current_cpu = CPUAffinity::get_current_cpu();
    std::cout << "Current CPU: " << current_cpu << std::endl;
    
    if (!cpus.empty()) {
        // Try to set affinity to first CPU
        if (CPUAffinity::set_thread_affinity(cpus[0])) {
            std::cout << "Successfully set thread affinity to CPU " << cpus[0] << std::endl;
            std::cout << "New CPU: " << CPUAffinity::get_current_cpu() << std::endl;
        } else {
            std::cout << "Failed to set thread affinity" << std::endl;
        }
    }
}

void test_profiling() {
    std::cout << "\n=== Testing Profiling ===" << std::endl;
    
    // Enable profiling
    Profiler::instance().enable();
    
    // Simulate some operations
    for (int i = 0; i < 1000; ++i) {
        {
            PROFILE_SCOPE("OrderProcessing");
            std::this_thread::sleep_for(std::chrono::microseconds(10));
        }
        
        {
            PROFILE_SCOPE("RiskCheck");
            std::this_thread::sleep_for(std::chrono::microseconds(5));
        }
    }
    
    // Get and display stats
    auto order_stats = Profiler::instance().get_stats("OrderProcessing");
    auto risk_stats = Profiler::instance().get_stats("RiskCheck");
    
    std::cout << "Order Processing: " << std::endl;
    std::cout << "  Count: " << order_stats.count << std::endl;
    std::cout << "  Mean: " << order_stats.mean() / 1000.0 << " μs" << std::endl;
    std::cout << "  Min: " << order_stats.min_ns / 1000.0 << " μs" << std::endl;
    std::cout << "  Max: " << order_stats.max_ns / 1000.0 << " μs" << std::endl;
    std::cout << "  Stddev: " << order_stats.stddev() / 1000.0 << " μs" << std::endl;
    
    std::cout << "Risk Check: " << std::endl;
    std::cout << "  Count: " << risk_stats.count << std::endl;
    std::cout << "  Mean: " << risk_stats.mean() / 1000.0 << " μs" << std::endl;
    std::cout << "  Min: " << risk_stats.min_ns / 1000.0 << " μs" << std::endl;
    std::cout << "  Max: " << risk_stats.max_ns / 1000.0 << " μs" << std::endl;
    std::cout << "  Stddev: " << risk_stats.stddev() / 1000.0 << " μs" << std::endl;
}

void benchmark_branch_prediction() {
    std::cout << "\n=== Testing Branch Prediction Optimization ===" << std::endl;
    
    const size_t iterations = 10000000;
    std::vector<int> data(iterations);
    
    // Generate predictable pattern
    for (size_t i = 0; i < iterations; ++i) {
        data[i] = (i < iterations / 2) ? 1 : 0;
    }
    
    // Benchmark with likely hint
    int sum = 0;
    auto start = std::chrono::high_resolution_clock::now();
    for (size_t i = 0; i < iterations; ++i) {
        sum += select_likely(data[i] == 1, 10, 5);
    }
    auto likely_time = std::chrono::high_resolution_clock::now() - start;
    
    std::cout << "Likely hint time: " 
              << std::chrono::duration_cast<std::chrono::milliseconds>(likely_time).count() 
              << " ms (sum=" << sum << ")" << std::endl;
    
    // Shuffle data for unpredictable pattern
    std::random_device rd;
    std::mt19937 gen(rd());
    std::shuffle(data.begin(), data.end(), gen);
    
    // Benchmark with random pattern
    sum = 0;
    start = std::chrono::high_resolution_clock::now();
    for (size_t i = 0; i < iterations; ++i) {
        sum += select_unlikely(data[i] == 1, 10, 5);
    }
    auto random_time = std::chrono::high_resolution_clock::now() - start;
    
    std::cout << "Random pattern time: " 
              << std::chrono::duration_cast<std::chrono::milliseconds>(random_time).count() 
              << " ms (sum=" << sum << ")" << std::endl;
}

int main() {
    std::cout << "=== OMS Performance Optimization Test Suite ===" << std::endl;
    
    try {
        test_spsc_queue();
        test_memory_pool();
        test_simd_operations();
        test_cpu_affinity();
        test_profiling();
        benchmark_branch_prediction();
        
        std::cout << "\n=== All tests completed successfully ===" << std::endl;
    } catch (const std::exception& e) {
        std::cerr << "Test failed: " << e.what() << std::endl;
        return 1;
    }
    
    return 0;
}