#include <iostream>
#include <thread>
#include <chrono>
#include <vector>
#include <random>
#include "account/multi_account_risk_engine.h"

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
    AccountRiskMetrics main_account;
    main_account.max_exposure = 1000000.0;
    main_account.max_position_size = 100000.0;
    main_account.daily_loss_limit = 50000.0;
    main_account.max_positions = 20;
    main_account.max_leverage = 10;
    
    bool result = engine.AddAccount("main", main_account);
    PrintTestResult("Add main account", result);
    
    // Add sub accounts
    AccountRiskMetrics sub_account;
    sub_account.max_exposure = 100000.0;
    sub_account.max_position_size = 20000.0;
    sub_account.daily_loss_limit = 5000.0;
    sub_account.max_positions = 10;
    sub_account.max_leverage = 5;
    
    result = engine.AddAccount("sub_arb", sub_account);
    PrintTestResult("Add sub account 1", result);
    
    result = engine.AddAccount("sub_mm", sub_account);
    PrintTestResult("Add sub account 2", result);
    
    // Test 2: Update account limits
    sub_account.max_exposure = 150000.0;
    result = engine.UpdateAccountLimits("sub_arb", sub_account);
    PrintTestResult("Update account limits", result);
    
    // Test 3: Remove account
    result = engine.RemoveAccount("sub_mm");
    PrintTestResult("Remove account", result);
}

void TestPositionUpdates() {
    std::cout << "\n=== Testing Position Updates ===" << std::endl;
    
    MultiAccountRiskEngine engine;
    
    // Add test account
    AccountRiskMetrics account;
    account.max_exposure = 100000.0;
    account.max_position_size = 20000.0;
    account.daily_loss_limit = 5000.0;
    account.max_positions = 10;
    account.max_leverage = 5;
    
    engine.AddAccount("test_account", account);
    
    // Test position updates
    PositionRisk position;
    position.account_id = "test_account";
    position.symbol = "BTCUSDT";
    position.position_size = 0.5;
    position.entry_price = 30000.0;
    position.mark_price = 30500.0;
    position.liquidation_price = 28000.0;
    position.unrealized_pnl = 250.0;
    position.margin_used = 3000.0;
    position.leverage = 5;
    
    bool result = engine.UpdatePosition(position);
    PrintTestResult("Add position", result);
    
    // Update position
    position.mark_price = 31000.0;
    position.unrealized_pnl = 500.0;
    result = engine.UpdatePosition(position);
    PrintTestResult("Update position", result);
    
    // Check exposure
    double exposure = engine.GetAccountExposure("test_account");
    PrintTestResult("Get account exposure", exposure > 0);
    std::cout << "  Exposure: $" << exposure << std::endl;
    
    // Remove position
    result = engine.RemovePosition("test_account", "BTCUSDT");
    PrintTestResult("Remove position", result);
}

void TestRiskChecks() {
    std::cout << "\n=== Testing Risk Checks ===" << std::endl;
    
    MultiAccountRiskEngine engine;
    
    // Setup account
    AccountRiskMetrics account;
    account.max_exposure = 100000.0;
    account.max_position_size = 20000.0;
    account.daily_loss_limit = 5000.0;
    account.max_positions = 10;
    account.max_leverage = 5;
    
    engine.AddAccount("test_account", account);
    
    // Test order risk check
    Order order;
    order.symbol = "BTCUSDT";
    order.side = OrderSide::BUY;
    order.type = OrderType::LIMIT;
    order.quantity = 0.5;
    order.price = 30000.0;
    
    auto start = std::chrono::high_resolution_clock::now();
    RiskCheckResult result = engine.CheckOrderRisk("test_account", order);
    auto end = std::chrono::high_resolution_clock::now();
    
    auto latency = std::chrono::duration_cast<std::chrono::microseconds>(end - start).count();
    
    PrintTestResult("Order risk check", result.passed);
    std::cout << "  Reason: " << result.reason << std::endl;
    std::cout << "  Latency: " << latency << " μs" << std::endl;
    
    // Test with large order (should fail)
    order.quantity = 10.0;
    result = engine.CheckOrderRisk("test_account", order);
    PrintTestResult("Large order risk check (should fail)", !result.passed);
    std::cout << "  Reason: " << result.reason << std::endl;
    
    // Test account risk check
    result = engine.CheckAccountRisk("test_account");
    PrintTestResult("Account risk check", result.passed);
}

void TestKillSwitch() {
    std::cout << "\n=== Testing Kill Switch ===" << std::endl;
    
    MultiAccountRiskEngine engine;
    
    // Setup accounts
    AccountRiskMetrics account;
    account.max_exposure = 100000.0;
    engine.AddAccount("account1", account);
    engine.AddAccount("account2", account);
    
    // Test individual kill switch
    engine.EnableKillSwitch("account1");
    bool active = engine.IsKillSwitchActive("account1");
    PrintTestResult("Enable kill switch for account1", active);
    
    // Test order should fail
    Order order;
    order.symbol = "BTCUSDT";
    order.quantity = 0.1;
    order.price = 30000.0;
    
    RiskCheckResult result = engine.CheckOrderRisk("account1", order);
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
        AccountRiskMetrics account;
        account.max_exposure = 100000.0;
        account.max_position_size = 20000.0;
        account.daily_loss_limit = 5000.0;
        account.max_positions = 10;
        
        engine.AddAccount("account_" + std::to_string(i), account);
    }
    
    // Add positions to accounts
    std::random_device rd;
    std::mt19937 gen(rd());
    std::uniform_real_distribution<> price_dist(20000, 40000);
    std::uniform_real_distribution<> size_dist(0.1, 1.0);
    
    for (int i = 0; i < num_accounts; ++i) {
        for (int j = 0; j < 5; ++j) {
            PositionRisk position;
            position.account_id = "account_" + std::to_string(i);
            position.symbol = "SYMBOL_" + std::to_string(j);
            position.position_size = size_dist(gen);
            position.entry_price = price_dist(gen);
            position.mark_price = position.entry_price * 1.01;
            position.liquidation_price = position.entry_price * 0.9;
            position.margin_used = position.position_size * position.entry_price / 10;
            
            engine.UpdatePosition(position);
        }
    }
    
    // Test risk check performance
    const int num_checks = 10000;
    auto start = std::chrono::high_resolution_clock::now();
    
    for (int i = 0; i < num_checks; ++i) {
        Order order;
        order.symbol = "BTCUSDT";
        order.quantity = 0.1;
        order.price = 30000.0;
        
        int account_idx = i % num_accounts;
        engine.CheckOrderRisk("account_" + std::to_string(account_idx), order);
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
    
    // Test aggregated risk calculation
    start = std::chrono::high_resolution_clock::now();
    engine.UpdateAggregatedRisk();
    end = std::chrono::high_resolution_clock::now();
    
    auto agg_time = std::chrono::duration_cast<std::chrono::microseconds>(end - start).count();
    std::cout << "\nAggregated risk calculation: " << agg_time << " μs" << std::endl;
    
    auto aggregated = engine.GetAggregatedRisk();
    std::cout << "  Total exposure: $" << aggregated.total_exposure.load() << std::endl;
    std::cout << "  Total positions: " << aggregated.total_positions.load() << std::endl;
}

void TestRiskMonitor() {
    std::cout << "\n=== Testing Risk Monitor ===" << std::endl;
    
    MultiAccountRiskEngine engine;
    MultiAccountRiskMonitor monitor(&engine);
    
    // Setup test account
    AccountRiskMetrics account;
    account.max_exposure = 100000.0;
    account.max_position_size = 20000.0;
    account.daily_loss_limit = 5000.0;
    
    engine.AddAccount("test_account", account);
    
    // Register callbacks
    bool alert_triggered = false;
    monitor.RegisterAlertCallback(
        [&alert_triggered](const std::string& account_id, const std::string& message) {
            std::cout << "ALERT [" << account_id << "]: " << message << std::endl;
            alert_triggered = true;
        }
    );
    
    // Start monitoring
    monitor.Start();
    
    // Add high-risk position
    PositionRisk position;
    position.account_id = "test_account";
    position.symbol = "BTCUSDT";
    position.position_size = 5.0;
    position.entry_price = 30000.0;
    position.mark_price = 28000.0;  // Loss position
    position.liquidation_price = 27000.0;
    position.unrealized_pnl = -10000.0;
    position.margin_used = 15000.0;
    
    engine.UpdatePosition(position);
    
    // Give monitor time to detect
    std::this_thread::sleep_for(std::chrono::milliseconds(200));
    
    monitor.Stop();
    
    PrintTestResult("Risk monitor alert", alert_triggered);
}

int main() {
    std::cout << "=== Multi-Account Risk Engine Test ===" << std::endl;
    std::cout << "Testing C++ high-performance risk management\n" << std::endl;
    
    TestAccountManagement();
    TestPositionUpdates();
    TestRiskChecks();
    TestKillSwitch();
    TestPerformance();
    TestRiskMonitor();
    
    std::cout << "\n=== Test Summary ===" << std::endl;
    std::cout << "All tests completed. Check results above." << std::endl;
    
    return 0;
}