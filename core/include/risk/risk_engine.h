#ifndef RISK_ENGINE_H
#define RISK_ENGINE_H

#include <atomic>
#include <array>
#include <chrono>
#include <string>
#include "types.h"

namespace oms {
namespace risk {

// Position information
struct PositionInfo {
    std::atomic<double> quantity{0.0};
    std::atomic<double> value{0.0};
    std::atomic<double> avg_price{0.0};
};

// Risk configuration
struct RiskConfig {
    double max_position_value;     // Maximum position value per symbol
    double max_order_value;        // Maximum order value
    double daily_loss_limit;       // Daily loss limit
    int max_open_orders;          // Maximum open orders
    double max_leverage;          // Maximum leverage
    
    RiskConfig() : max_position_value(100000.0), max_order_value(10000.0),
                   daily_loss_limit(5000.0), max_open_orders(100),
                   max_leverage(10.0) {}
};

// High-performance risk engine
class RiskEngine {
public:
    static constexpr size_t MAX_SYMBOLS = 1000;
    
    RiskEngine(const RiskConfig& config);
    ~RiskEngine() = default;
    
    // Order risk check (< 50 microseconds)
    bool checkOrder(const Order& order);
    
    // Update position
    void updatePosition(const std::string& symbol, double quantity, double price);
    
    // Update open order count
    void updateOrderCount(int delta);
    
    // Get total exposure
    double getTotalExposure() const;
    
    // Reset daily PnL
    void resetDailyPnL();
    
    // Control
    void start();
    void stop();
    
    // Statistics
    size_t getTotalChecks() const;
    double getAverageLatencyUs() const;
    
private:
    // Configuration
    RiskConfig config_;
    
    // Position tracking (lock-free)
    std::array<PositionInfo, MAX_SYMBOLS> positions_;
    
    // Daily PnL tracking
    std::atomic<double> daily_pnl_{0.0};
    
    // Open orders count
    std::atomic<int> open_orders_{0};
    
    // Statistics
    std::atomic<size_t> total_checks_{0};
    std::atomic<uint64_t> total_latency_ns_{0};
    
    // Control
    std::atomic<bool> running_{false};
    
    // Helper methods
    size_t hashSymbol(const std::string& symbol);
    void log(const std::string& message);
    double calculateRealizedPnL(double old_quantity, double old_price, 
                               double new_quantity, double new_price);
};

} // namespace risk
} // namespace oms

#endif // RISK_ENGINE_H