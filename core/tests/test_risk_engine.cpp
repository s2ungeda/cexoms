#include <cassert>
#include <iostream>
#include <chrono>
#include "../include/risk/risk_engine.h"

using namespace oms::risk;
using namespace oms;

void test_risk_engine_basic() {
    std::cout << "Testing Risk Engine basic functionality..." << std::endl;
    
    RiskConfig config;
    config.max_order_value = 10000.0;
    config.max_position_value = 50000.0;
    config.daily_loss_limit = 5000.0;
    config.max_open_orders = 10;
    
    RiskEngine engine(config);
    engine.start();
    
    // Test valid order
    Order order1;
    order1.symbol = "BTCUSDT";
    order1.side = Side::BUY;
    order1.price = 40000.0;
    order1.quantity = 0.1;  // Value = 4000
    
    assert(engine.checkOrder(order1) == true);
    std::cout << "✓ Valid order passed" << std::endl;
    
    // Test order exceeding limit
    Order order2;
    order2.symbol = "BTCUSDT";
    order2.side = Side::BUY;
    order2.price = 40000.0;
    order2.quantity = 0.5;  // Value = 20000, exceeds limit
    
    assert(engine.checkOrder(order2) == false);
    std::cout << "✓ Over-limit order rejected" << std::endl;
    
    engine.stop();
}

void test_risk_engine_performance() {
    std::cout << "\nTesting Risk Engine performance..." << std::endl;
    
    RiskConfig config;
    RiskEngine engine(config);
    engine.start();
    
    const int num_tests = 10000;
    Order order;
    order.symbol = "BTCUSDT";
    order.side = Side::BUY;
    order.price = 40000.0;
    order.quantity = 0.01;
    
    auto start = std::chrono::high_resolution_clock::now();
    
    for (int i = 0; i < num_tests; ++i) {
        engine.checkOrder(order);
    }
    
    auto end = std::chrono::high_resolution_clock::now();
    auto duration = std::chrono::duration_cast<std::chrono::microseconds>(end - start);
    
    double avg_latency = duration.count() / static_cast<double>(num_tests);
    std::cout << "Average latency: " << avg_latency << " μs" << std::endl;
    
    assert(avg_latency < 50.0);  // Should be under 50 microseconds
    std::cout << "✓ Performance target met" << std::endl;
    
    engine.stop();
}

void test_position_management() {
    std::cout << "\nTesting position management..." << std::endl;
    
    RiskConfig config;
    RiskEngine engine(config);
    engine.start();
    
    // Add position
    engine.updatePosition("BTCUSDT", 1.0, 40000.0);
    
    // Update position
    engine.updatePosition("BTCUSDT", -0.5, 41000.0);
    
    // Check exposure
    double exposure = engine.getTotalExposure();
    std::cout << "Total exposure: $" << exposure << std::endl;
    
    engine.stop();
    std::cout << "✓ Position management test passed" << std::endl;
}

int main() {
    std::cout << "=== C++ Core Engine Tests ===" << std::endl;
    
    try {
        test_risk_engine_basic();
        test_risk_engine_performance();
        test_position_management();
        
        std::cout << "\nAll tests passed! ✓" << std::endl;
        return 0;
    } catch (const std::exception& e) {
        std::cerr << "Test failed: " << e.what() << std::endl;
        return 1;
    }
}