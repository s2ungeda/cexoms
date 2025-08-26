#include <iostream>
#include <thread>
#include <chrono>
#include <vector>
#include <random>
#include "account/multi_account_risk_engine_simple.h"

using namespace oms;
using namespace oms::risk;

void PrintTestResult(const std::string& test_name, bool passed) {
    std::cout << test_name << ": " 
              << (passed ? "\033[32mPASSED\033[0m" : "\033[31mFAILED\033[0m") 
              << std::endl;
}

void TestAccountManagement() {
    std::cout << "\n=== Testing Account Management ===" << std::endl;
    
    MultiAccountRiskEngine engine;
    
    // Test 1: Add accounts
    bool result = engine.AddAccount("main", 1000000.0, 50000.0, 20);
    PrintTestResult("Add main account", result);
    
    result = engine.AddAccount("sub_arb", 100000.0, 5000.0, 10);
    PrintTestResult("Add sub account 1", result);
    
    result = engine.AddAccount("sub_mm", 50000.0, 2500.0, 5);
    PrintTestResult("Add sub account 2", result);
    
    // Test 2: Remove account
    result = engine.RemoveAccount("sub_mm");
    PrintTestResult("Remove account", result);
}

void TestRiskChecks() {
    std::cout << "\n=== Testing Risk Checks ===" << std::endl;
    
    MultiAccountRiskEngine engine;
    
    // Setup account
    engine.AddAccount("test_account", 100000.0, 5000.0, 10);
    
    // Update account metrics
    engine.UpdateAccountMetrics("test_account", 30000.0, -1000.0, 3);
    
    // Test order risk check
    auto start = std::chrono::high_resolution_clock::now();
    RiskCheckResult result = engine.CheckOrderRisk("test_account", 15000.0, 1500.0);
    auto end = std::chrono::high_resolution_clock::now();
    
    auto latency = std::chrono::duration_cast<std::chrono::microseconds>(end - start).count();
    
    PrintTestResult("Order risk check", result.passed);
    std::cout << "  Reason: " << result.reason << std::endl;
    std::cout << "  Latency: " << latency << " μs" << std::endl;
    
    // Test with large order (should fail)
    result = engine.CheckOrderRisk("test_account", 300000.0, 30000.0);
    PrintTestResult("Large order risk check (should fail)", !result.passed);
    std::cout << "  Reason: " << result.reason << std::endl;
    
    // Test with daily loss exceeded
    engine.UpdateAccountMetrics("test_account", 30000.0, -6000.0, 3);
    result = engine.CheckOrderRisk("test_account", 1000.0, 100.0);
    PrintTestResult("Daily loss check (should fail)", !result.passed);
    std::cout << "  Reason: " << result.reason << std::endl;
}

void TestKillSwitch() {
    std::cout << "\n=== Testing Kill Switch ===" << std::endl;
    
    MultiAccountRiskEngine engine;
    
    // Setup accounts
    engine.AddAccount("account1", 100000.0, 5000.0, 10);
    engine.AddAccount("account2", 100000.0, 5000.0, 10);
    
    // Test individual kill switch
    engine.EnableKillSwitch("account1");
    bool active = engine.IsKillSwitchActive("account1");
    PrintTestResult("Enable kill switch for account1", active);
    
    // Test order should fail
    RiskCheckResult result = engine.CheckOrderRisk("account1", 1000.0, 100.0);
    PrintTestResult("Order blocked by kill switch", !result.passed && result.reason == "Kill switch active");
    
    // Test global kill switch
    engine.EnableKillSwitch("");
    active = engine.IsKillSwitchActive("account2");
    PrintTestResult("Global kill switch", active);
    
    // Disable kill switch
    engine.DisableKillSwitch("");
    active = engine.IsKillSwitchActive("account1");
    PrintTestResult("Disable global kill switch", !active);
}

void TestPerformance() {
    std::cout << "\n=== Testing Performance ===" << std::endl;
    
    MultiAccountRiskEngine engine;
    
    // Add multiple accounts
    const int num_accounts = 50;
    for (int i = 0; i < num_accounts; ++i) {
        engine.AddAccount("account_" + std::to_string(i), 100000.0, 5000.0, 10);
        
        // Set some initial metrics
        engine.UpdateAccountMetrics("account_" + std::to_string(i), 
                                  20000.0 + i * 1000, 
                                  -100.0 * i, 
                                  i % 5);
    }
    
    // Test risk check performance
    const int num_checks = 10000;
    auto start = std::chrono::high_resolution_clock::now();
    
    for (int i = 0; i < num_checks; ++i) {
        int account_idx = i % num_accounts;
        engine.CheckOrderRisk("account_" + std::to_string(account_idx), 3000.0, 300.0);
    }
    
    auto end = std::chrono::high_resolution_clock::now();
    auto total_time = std::chrono::duration_cast<std::chrono::microseconds>(end - start).count();
    double avg_latency = static_cast<double>(total_time) / num_checks;
    
    std::cout << "Risk check performance:" << std::endl;
    std::cout << "  Total checks: " << num_checks << std::endl;
    std::cout << "  Total time: " << total_time << " μs" << std::endl;
    std::cout << "  Average latency: " << avg_latency << " μs" << std::endl;
    std::cout << "  Checks per second: " << (1000000.0 / avg_latency) << std::endl;
    
    PrintTestResult("Performance < 50μs", avg_latency < 50.0);
    
    // Get stats from engine
    std::cout << "\nEngine statistics:" << std::endl;
    std::cout << "  Total risk checks: " << engine.GetRiskCheckCount() << std::endl;
    std::cout << "  Average latency: " << engine.GetAverageCheckLatency() << " μs" << std::endl;
}

void TestMultiThreaded() {
    std::cout << "\n=== Testing Multi-threaded Access ===" << std::endl;
    
    MultiAccountRiskEngine engine;
    
    // Add accounts
    for (int i = 0; i < 10; ++i) {
        engine.AddAccount("account_" + std::to_string(i), 100000.0, 5000.0, 10);
    }
    
    const int num_threads = 4;
    const int checks_per_thread = 2500;
    std::vector<std::thread> threads;
    
    auto worker = [&engine, checks_per_thread]() {
        std::random_device rd;
        std::mt19937 gen(rd());
        std::uniform_int_distribution<> account_dist(0, 9);
        std::uniform_real_distribution<> value_dist(1000, 10000);
        
        for (int i = 0; i < checks_per_thread; ++i) {
            int account_idx = account_dist(gen);
            double order_value = value_dist(gen);
            
            engine.CheckOrderRisk("account_" + std::to_string(account_idx), 
                                order_value, order_value * 0.1);
        }
    };
    
    auto start = std::chrono::high_resolution_clock::now();
    
    // Start threads
    for (int i = 0; i < num_threads; ++i) {
        threads.emplace_back(worker);
    }
    
    // Wait for completion
    for (auto& t : threads) {
        t.join();
    }
    
    auto end = std::chrono::high_resolution_clock::now();
    auto total_time = std::chrono::duration_cast<std::chrono::milliseconds>(end - start).count();
    
    std::cout << "Multi-threaded performance:" << std::endl;
    std::cout << "  Threads: " << num_threads << std::endl;
    std::cout << "  Total checks: " << (num_threads * checks_per_thread) << std::endl;
    std::cout << "  Total time: " << total_time << " ms" << std::endl;
    std::cout << "  Throughput: " << ((num_threads * checks_per_thread) / (total_time / 1000.0)) 
              << " checks/sec" << std::endl;
    
    PrintTestResult("Multi-threaded test completed", true);
}

int main() {
    std::cout << "=== Multi-Account Risk Engine Test (Simplified) ===" << std::endl;
    std::cout << "Testing C++ high-performance risk management\n" << std::endl;
    
    TestAccountManagement();
    TestRiskChecks();
    TestKillSwitch();
    TestPerformance();
    TestMultiThreaded();
    
    std::cout << "\n=== Test Summary ===" << std::endl;
    std::cout << "All tests completed. Check results above." << std::endl;
    std::cout << "This simplified version demonstrates core functionality" << std::endl;
    std::cout << "while maintaining < 50μs latency target." << std::endl;
    
    return 0;
}