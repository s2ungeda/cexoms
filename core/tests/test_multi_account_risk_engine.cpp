#include <iostream>
#include <thread>
#include <vector>
#include <random>
#include <chrono>
#include <iomanip>
#include "risk/multi_account_risk_engine.h"
#include "risk/lock_free_risk_checker.h"

using namespace oms;
using namespace oms::risk;

// Performance test for multi-account risk engine
void testMultiAccountRiskEngine() {
    std::cout << "\n=== Testing Multi-Account Risk Engine ===" << std::endl;
    
    // Create global limits
    GlobalRiskLimits global_limits;
    global_limits.max_total_exposure = 10000000.0;  // $10M
    global_limits.daily_loss_limit = 100000.0;      // $100k
    
    // Create risk engine
    MultiAccountRiskEngine engine(global_limits);
    engine.start();
    
    // Add multiple accounts
    std::vector<std::string> accounts = {
        "main", "sub_arbitrage", "sub_market_making", "sub_trend"
    };
    
    for (const auto& account_id : accounts) {
        AccountRiskLimits limits;
        limits.max_position_value = 500000.0;  // $500k per position
        limits.max_total_exposure = 2000000.0; // $2M total
        limits.max_leverage = 10.0;
        limits.daily_loss_limit = 20000.0;     // $20k
        
        engine.addAccount(account_id, limits);
    }
    
    // Test single order check
    Order test_order;
    test_order.symbol = "BTCUSDT";
    test_order.side = OrderSide::BUY;
    test_order.quantity = 1.0;
    test_order.price = 50000.0;
    test_order.leverage = 5;
    
    auto result = engine.checkOrder("main", test_order);
    std::cout << "Order check result: " << (result.passed ? "PASSED" : "FAILED") << std::endl;
    std::cout << "  Reason: " << result.reason << std::endl;
    std::cout << "  Margin required: $" << result.margin_required << std::endl;
    std::cout << "  Latency: " << result.latency.count() << " μs" << std::endl;
    
    // Performance test - multiple concurrent checks
    const int num_threads = 4;
    const int checks_per_thread = 10000;
    std::vector<std::thread> threads;
    std::atomic<int> total_passed{0};
    std::atomic<int> total_failed{0};
    
    auto worker = [&](int thread_id) {
        std::mt19937 gen(thread_id);
        std::uniform_int_distribution<> account_dist(0, accounts.size() - 1);
        std::uniform_real_distribution<> price_dist(40000, 60000);
        std::uniform_real_distribution<> qty_dist(0.1, 5.0);
        
        int passed = 0, failed = 0;
        
        for (int i = 0; i < checks_per_thread; ++i) {
            Order order;
            order.symbol = "BTCUSDT";
            order.side = (i % 2) ? OrderSide::BUY : OrderSide::SELL;
            order.quantity = qty_dist(gen);
            order.price = price_dist(gen);
            order.leverage = 5;
            
            auto result = engine.checkOrder(accounts[account_dist(gen)], order);
            if (result.passed) {
                passed++;
            } else {
                failed++;
            }
        }
        
        total_passed += passed;
        total_failed += failed;
    };
    
    // Start performance test
    auto start = std::chrono::high_resolution_clock::now();
    
    for (int i = 0; i < num_threads; ++i) {
        threads.emplace_back(worker, i);
    }
    
    for (auto& t : threads) {
        t.join();
    }
    
    auto end = std::chrono::high_resolution_clock::now();
    auto duration = std::chrono::duration_cast<std::chrono::milliseconds>(end - start);
    
    int total_checks = num_threads * checks_per_thread;
    double avg_latency = engine.getAverageLatencyUs();
    double throughput = (total_checks * 1000.0) / duration.count();
    
    std::cout << "\nPerformance Test Results:" << std::endl;
    std::cout << "  Total checks: " << total_checks << std::endl;
    std::cout << "  Passed: " << total_passed.load() << std::endl;
    std::cout << "  Failed: " << total_failed.load() << std::endl;
    std::cout << "  Total time: " << duration.count() << " ms" << std::endl;
    std::cout << "  Average latency: " << avg_latency << " μs" << std::endl;
    std::cout << "  Throughput: " << std::fixed << std::setprecision(0) 
              << throughput << " checks/second" << std::endl;
    
    // Test risk metrics
    auto global_metrics = engine.getGlobalMetrics();
    std::cout << "\nGlobal Risk Metrics:" << std::endl;
    std::cout << "  Total exposure: $" << global_metrics.total_exposure << std::endl;
    std::cout << "  Total positions: " << global_metrics.total_positions << std::endl;
    std::cout << "  Total orders: " << global_metrics.total_orders << std::endl;
}

// Performance test for lock-free risk checker
void testLockFreeRiskChecker() {
    std::cout << "\n=== Testing Lock-Free Risk Checker ===" << std::endl;
    
    LockFreeRiskChecker checker;
    
    // Configure limits
    checker.setMaxPositionValue(1000000.0);    // $1M
    checker.setMaxAccountExposure(5000000.0);  // $5M
    checker.setMaxDailyLoss(50000.0);         // $50k
    
    // Setup test accounts
    std::vector<uint32_t> account_hashes = {
        std::hash<std::string>{}("main"),
        std::hash<std::string>{}("sub1"),
        std::hash<std::string>{}("sub2"),
        std::hash<std::string>{}("sub3")
    };
    
    // Initialize accounts with some balance
    for (auto hash : account_hashes) {
        checker.updateAccountData(hash, 0.0, 1000000.0, 0.0, 0.0);
    }
    
    // Single check test
    uint32_t btc_hash = std::hash<std::string>{}("BTCUSDT");
    auto result = checker.checkOrderFast(
        account_hashes[0], btc_hash, 1.0, 50000.0, OrderSide::BUY, 10
    );
    
    std::cout << "Single check result: " << (result.passed ? "PASSED" : "FAILED") << std::endl;
    std::cout << "  Reason: " << result.reason << std::endl;
    std::cout << "  Latency: " << result.latency.count() << " ns" << std::endl;
    
    // Ultra-high performance test
    const int num_checks = 1000000;
    std::mt19937 gen(42);
    std::uniform_int_distribution<> account_dist(0, account_hashes.size() - 1);
    std::uniform_real_distribution<> price_dist(40000, 60000);
    std::uniform_real_distribution<> qty_dist(0.01, 2.0);
    
    auto start = std::chrono::high_resolution_clock::now();
    
    for (int i = 0; i < num_checks; ++i) {
        checker.checkOrderFast(
            account_hashes[account_dist(gen)],
            btc_hash,
            qty_dist(gen),
            price_dist(gen),
            (i % 2) ? OrderSide::BUY : OrderSide::SELL,
            10
        );
    }
    
    auto end = std::chrono::high_resolution_clock::now();
    auto duration = std::chrono::duration_cast<std::chrono::milliseconds>(end - start);
    
    const auto& stats = checker.getStats();
    double avg_latency_ns = static_cast<double>(stats.total_latency_ns.load()) / 
                           stats.total_checks.load();
    double throughput = (num_checks * 1000.0) / duration.count();
    
    std::cout << "\nUltra-High Performance Test Results:" << std::endl;
    std::cout << "  Total checks: " << stats.total_checks.load() << std::endl;
    std::cout << "  Passed: " << stats.passed_checks.load() << std::endl;
    std::cout << "  Failed: " << stats.failed_checks.load() << std::endl;
    std::cout << "  Total time: " << duration.count() << " ms" << std::endl;
    std::cout << "  Average latency: " << avg_latency_ns << " ns (" 
              << avg_latency_ns/1000.0 << " μs)" << std::endl;
    std::cout << "  Min latency: " << stats.min_latency_ns.load() << " ns" << std::endl;
    std::cout << "  Max latency: " << stats.max_latency_ns.load() << " ns" << std::endl;
    std::cout << "  Throughput: " << std::fixed << std::setprecision(0) 
              << throughput << " checks/second" << std::endl;
    
    // Test batch checker
    std::cout << "\nTesting Batch Checker:" << std::endl;
    BatchRiskChecker batch_checker(&checker);
    
    std::vector<Order> batch_orders;
    std::vector<uint32_t> batch_accounts;
    
    for (int i = 0; i < 32; ++i) {
        Order order;
        order.symbol = "BTCUSDT";
        order.side = (i % 2) ? OrderSide::BUY : OrderSide::SELL;
        order.quantity = 0.5;
        order.price = 50000.0;
        order.leverage = 10;
        
        batch_orders.push_back(order);
        batch_accounts.push_back(account_hashes[i % account_hashes.size()]);
    }
    
    auto batch_result = batch_checker.checkBatch(batch_orders, batch_accounts);
    std::cout << "  Batch size: " << batch_result.count << std::endl;
    std::cout << "  Batch latency: " << batch_result.total_latency.count() << " ns" << std::endl;
    std::cout << "  Per-order latency: " << batch_result.total_latency.count() / batch_result.count 
              << " ns" << std::endl;
}

// Test risk monitoring and alerts
void testRiskMonitor() {
    std::cout << "\n=== Testing Risk Monitor ===" << std::endl;
    
    GlobalRiskLimits global_limits;
    MultiAccountRiskEngine engine(global_limits);
    engine.start();
    
    // Add test account
    AccountRiskLimits limits;
    limits.max_position_value = 100000.0;
    limits.daily_loss_limit = 5000.0;
    engine.addAccount("test_account", limits);
    
    // Create risk monitor
    RiskMonitor monitor(&engine);
    
    // Register alert callback
    monitor.registerAlertCallback(
        [](const std::string& account_id, const std::string& alert_type, const std::string& message) {
            std::cout << "[ALERT] Account: " << account_id 
                     << ", Type: " << alert_type 
                     << ", Message: " << message << std::endl;
        }
    );
    
    // Start monitoring
    monitor.start();
    
    // Simulate some risky positions
    engine.updatePosition("test_account", "BTCUSDT", 10.0, 50000.0, 50000.0);
    
    std::this_thread::sleep_for(std::chrono::seconds(2));
    
    // Stop monitoring
    monitor.stop();
}

int main() {
    std::cout << "=== OMS Multi-Account Risk Engine Test ===" << std::endl;
    std::cout << "Testing high-performance risk checks with lock-free operations" << std::endl;
    
    // Run tests
    testMultiAccountRiskEngine();
    testLockFreeRiskChecker();
    testRiskMonitor();
    
    std::cout << "\nAll tests completed!" << std::endl;
    
    return 0;
}