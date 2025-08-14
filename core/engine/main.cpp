#include <iostream>
#include <signal.h>
#include <thread>
#include <chrono>

#include "order_manager.h"

namespace {
    std::atomic<bool> running{true};
    
    void signal_handler(int sig) {
        std::cout << "\nReceived signal " << sig << ", shutting down..." << std::endl;
        running = false;
    }
}

int main(int argc, char* argv[]) {
    // Set up signal handlers
    signal(SIGINT, signal_handler);
    signal(SIGTERM, signal_handler);
    
    std::cout << "OMS Core Engine v1.0.0 starting..." << std::endl;
    
    // Configure order manager
    oms::OrderManager::Config config;
    config.ring_buffer_size = 1048576;  // 1MB
    config.max_orders_per_second = 100000;
    config.cpu_cores = {2, 3};  // Use CPU cores 2 and 3
    
    // Create order manager
    oms::OrderManager order_manager(config);
    
    // Start processing
    order_manager.Start();
    std::cout << "Order manager started on CPU cores: ";
    for (int core : config.cpu_cores) {
        std::cout << core << " ";
    }
    std::cout << std::endl;
    
    // Main loop - print statistics
    auto last_stats_time = std::chrono::steady_clock::now();
    uint64_t last_processed = 0;
    
    while (running.load()) {
        std::this_thread::sleep_for(std::chrono::seconds(1));
        
        auto now = std::chrono::steady_clock::now();
        auto elapsed = std::chrono::duration_cast<std::chrono::seconds>(
            now - last_stats_time).count();
        
        if (elapsed >= 10) {  // Print stats every 10 seconds
            const auto& stats = order_manager.GetStats();
            uint64_t processed = stats.orders_processed.load();
            uint64_t rejected = stats.orders_rejected.load();
            uint64_t total_latency = stats.total_latency_us.load();
            uint64_t min_latency = stats.min_latency_us.load();
            uint64_t max_latency = stats.max_latency_us.load();
            
            uint64_t orders_per_sec = (processed - last_processed) / elapsed;
            uint64_t avg_latency = processed > 0 ? total_latency / processed : 0;
            
            std::cout << "\n=== Statistics ===" << std::endl;
            std::cout << "Orders processed: " << processed 
                     << " (" << orders_per_sec << "/sec)" << std::endl;
            std::cout << "Orders rejected: " << rejected << std::endl;
            std::cout << "Latency (μs) - Min: " << min_latency 
                     << ", Avg: " << avg_latency 
                     << ", Max: " << max_latency << std::endl;
            
            last_processed = processed;
            last_stats_time = now;
        }
    }
    
    // Shutdown
    std::cout << "\nShutting down order manager..." << std::endl;
    order_manager.Stop();
    
    // Print final statistics
    const auto& stats = order_manager.GetStats();
    std::cout << "\n=== Final Statistics ===" << std::endl;
    std::cout << "Total orders processed: " << stats.orders_processed.load() << std::endl;
    std::cout << "Total orders rejected: " << stats.orders_rejected.load() << std::endl;
    
    std::cout << "OMS Core Engine stopped." << std::endl;
    return 0;
}