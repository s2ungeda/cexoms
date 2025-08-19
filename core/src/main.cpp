#include <iostream>
#include <thread>
#include <csignal>
#include <atomic>
#include <chrono>

#ifdef USE_CPU_AFFINITY
#include <sched.h>
#endif

#include "types.h"
#include "strategies/arbitrage_detector.h"
#include "strategies/market_maker.h"
#include "risk/risk_engine.h"

std::atomic<bool> g_running{true};

void signal_handler(int signal) {
    if (signal == SIGINT || signal == SIGTERM) {
        std::cout << "\nShutdown signal received..." << std::endl;
        g_running = false;
    }
}

void set_cpu_affinity(int cpu_id) {
#ifdef USE_CPU_AFFINITY
    cpu_set_t cpuset;
    CPU_ZERO(&cpuset);
    CPU_SET(cpu_id, &cpuset);
    
    pthread_t current_thread = pthread_self();
    if (pthread_setaffinity_np(current_thread, sizeof(cpu_set_t), &cpuset) != 0) {
        std::cerr << "Failed to set CPU affinity to core " << cpu_id << std::endl;
    } else {
        std::cout << "Thread bound to CPU core " << cpu_id << std::endl;
    }
#else
    std::cout << "CPU affinity not supported on this platform" << std::endl;
#endif
}

int main(int /*argc*/, char* /*argv*/[]) {
    // Set up signal handling
    std::signal(SIGINT, signal_handler);
    std::signal(SIGTERM, signal_handler);
    
    std::cout << "Multi-Exchange OMS Core Engine Starting..." << std::endl;
    std::cout << "Version: 1.0.0" << std::endl;
    std::cout << "CPU cores: " << std::thread::hardware_concurrency() << std::endl;
    
    // Initialize components
    try {
        // Risk Engine Configuration
        oms::risk::RiskConfig risk_config;
        risk_config.max_position_value = 1000000.0;  // $1M
        risk_config.max_order_value = 100000.0;      // $100k
        risk_config.daily_loss_limit = 50000.0;      // $50k
        risk_config.max_open_orders = 100;
        
        // Initialize Risk Engine
        oms::risk::RiskEngine risk_engine(risk_config);
        
        // Arbitrage Detector Configuration
        oms::strategies::ArbitrageConfig arb_config;
        arb_config.min_profit_rate = 0.001;  // 0.1%
        arb_config.max_position_size = 100000.0;   // $100k
        arb_config.min_profit_amount = 10.0;       // $10 minimum
        
        // Initialize Arbitrage Detector
        oms::strategies::ArbitrageDetector arb_detector(arb_config);
        
        // Market Maker Configuration
        oms::strategies::MarketMakerConfig mm_config;
        mm_config.base_spread_bps = 10;           // 0.1%
        mm_config.quote_size = 0.1;               // 0.1 BTC
        mm_config.max_inventory = 1.0;            // 1 BTC
        mm_config.quote_levels = 3;
        
        // Initialize Market Maker Engine
        oms::strategies::MarketMakerEngine mm_engine(mm_config);
        
        // Start components
        risk_engine.start();
        std::cout << "Risk Engine started" << std::endl;
        
        arb_detector.start();
        std::cout << "Arbitrage Detector started" << std::endl;
        
        mm_engine.start();
        std::cout << "Market Maker Engine started" << std::endl;
        
        // Main loop
        std::cout << "\nOMS Core Engine running. Press Ctrl+C to stop." << std::endl;
        
        auto last_stats_time = std::chrono::steady_clock::now();
        const auto stats_interval = std::chrono::seconds(10);
        
        while (g_running) {
            auto now = std::chrono::steady_clock::now();
            
            // Print statistics every 10 seconds
            if (now - last_stats_time >= stats_interval) {
                std::cout << "\n=== Performance Stats ===" << std::endl;
                std::cout << "Risk checks: " << risk_engine.getTotalChecks() 
                          << " (avg latency: " << risk_engine.getAverageLatencyUs() << " µs)" << std::endl;
                std::cout << "Arbitrage opportunities: " << arb_detector.getDetectedCount() 
                          << " (processed: " << arb_detector.getProcessedPrices() << ")" << std::endl;
                std::cout << "Market maker quotes: " << mm_engine.getQuotesGenerated() 
                          << " (updates: " << mm_engine.getMarketUpdates() << ")" << std::endl;
                std::cout << "========================" << std::endl;
                
                last_stats_time = now;
            }
            
            std::this_thread::sleep_for(std::chrono::milliseconds(100));
        }
        
        // Shutdown
        std::cout << "\nShutting down components..." << std::endl;
        
        mm_engine.stop();
        arb_detector.stop();
        risk_engine.stop();
        
        std::cout << "OMS Core Engine stopped successfully." << std::endl;
        
    } catch (const std::exception& e) {
        std::cerr << "Fatal error: " << e.what() << std::endl;
        return 1;
    }
    
    return 0;
}