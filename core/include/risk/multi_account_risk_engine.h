#ifndef MULTI_ACCOUNT_RISK_ENGINE_H
#define MULTI_ACCOUNT_RISK_ENGINE_H

#include <atomic>
#include <array>
#include <chrono>
#include <string>
#include <unordered_map>
#include <memory>
#include <shared_mutex>
#include <vector>
#include <functional>
#include <thread>
#include "types.h"

namespace oms {
namespace risk {

// Account-specific position information
struct AccountPosition {
    std::atomic<double> quantity{0.0};
    std::atomic<double> notional{0.0};
    std::atomic<double> entry_price{0.0};
    std::atomic<double> mark_price{0.0};
    std::atomic<double> unrealized_pnl{0.0};
    std::atomic<double> realized_pnl{0.0};
    std::atomic<int> leverage{1};
    std::atomic<double> margin_used{0.0};
};

// Account risk limits
struct AccountRiskLimits {
    std::string account_id;
    double max_position_value;      // Maximum position value per symbol
    double max_total_exposure;      // Maximum total exposure
    double max_order_value;         // Maximum single order value
    double daily_loss_limit;        // Daily loss limit
    double max_leverage;            // Maximum leverage
    int max_open_orders;           // Maximum open orders
    int max_positions;             // Maximum number of positions
    
    AccountRiskLimits() : 
        max_position_value(100000.0),
        max_total_exposure(500000.0),
        max_order_value(10000.0),
        daily_loss_limit(5000.0),
        max_leverage(10.0),
        max_open_orders(100),
        max_positions(50) {}
};

// Global risk limits
struct GlobalRiskLimits {
    double max_total_exposure;      // Maximum total exposure across all accounts
    double max_correlated_exposure; // Maximum correlated exposure
    double daily_loss_limit;        // Total daily loss limit
    double var_limit;              // Value at Risk limit
    int max_total_orders;          // Maximum total orders
    
    GlobalRiskLimits() :
        max_total_exposure(5000000.0),
        max_correlated_exposure(2000000.0),
        daily_loss_limit(50000.0),
        var_limit(100000.0),
        max_total_orders(1000) {}
};

// Risk metrics
struct RiskMetrics {
    double total_exposure;
    double total_unrealized_pnl;
    double total_realized_pnl;
    double total_margin_used;
    double margin_ratio;
    double leverage_ratio;
    int total_positions;
    int total_orders;
    std::chrono::microseconds last_check_latency;
};

// Account risk state
struct AccountRiskState {
    std::string account_id;
    AccountRiskLimits limits;
    std::unordered_map<std::string, AccountPosition> positions;
    std::atomic<double> total_exposure{0.0};
    std::atomic<double> daily_pnl{0.0};
    std::atomic<double> total_margin{0.0};
    std::atomic<int> open_orders{0};
    std::atomic<bool> trading_allowed{true};
    std::chrono::steady_clock::time_point last_update;
};

// Risk check result
struct RiskCheckResult {
    bool passed{false};
    std::string reason;
    double available_size{0.0};
    double margin_required{0.0};
    std::chrono::microseconds latency;
};

// Multi-account risk engine with lock-free operations
class MultiAccountRiskEngine {
public:
    static constexpr size_t MAX_ACCOUNTS = 200;    // Max sub-accounts
    static constexpr size_t MAX_SYMBOLS = 1000;
    
    MultiAccountRiskEngine(const GlobalRiskLimits& global_limits);
    ~MultiAccountRiskEngine();
    
    // Account management
    void addAccount(const std::string& account_id, const AccountRiskLimits& limits);
    void removeAccount(const std::string& account_id);
    void updateAccountLimits(const std::string& account_id, const AccountRiskLimits& limits);
    
    // Pre-trade risk check (< 50 microseconds target)
    RiskCheckResult checkOrder(const std::string& account_id, const Order& order);
    
    // Batch risk check for smart routing
    std::vector<RiskCheckResult> checkOrderMultiAccount(const Order& order, 
                                                        const std::vector<std::string>& accounts);
    
    // Position updates (lock-free)
    void updatePosition(const std::string& account_id, const std::string& symbol,
                       double quantity, double price, double mark_price);
    void closePosition(const std::string& account_id, const std::string& symbol);
    
    // Order count management
    void incrementOrderCount(const std::string& account_id);
    void decrementOrderCount(const std::string& account_id);
    
    // PnL updates
    void updatePnL(const std::string& account_id, const std::string& symbol,
                   double realized_pnl, double unrealized_pnl);
    
    // Risk metrics
    RiskMetrics getAccountMetrics(const std::string& account_id) const;
    RiskMetrics getGlobalMetrics() const;
    
    // Emergency controls
    void haltTrading(const std::string& account_id);
    void resumeTrading(const std::string& account_id);
    void haltAllTrading();
    void resumeAllTrading();
    bool isTradingAllowed(const std::string& account_id) const;
    
    // Kill switch
    void activateKillSwitch();
    bool isKillSwitchActive() const { return kill_switch_.load(); }
    
    // Daily reset
    void resetDailyPnL();
    void resetDailyPnL(const std::string& account_id);
    
    // Statistics
    size_t getTotalChecks() const { return total_checks_.load(); }
    double getAverageLatencyUs() const;
    size_t getRejectedOrders() const { return rejected_orders_.load(); }
    
    // Get global limits (for monitoring)
    const GlobalRiskLimits& getGlobalLimits() const { return global_limits_; }
    
    // Control
    void start();
    void stop();
    bool isRunning() const { return running_.load(); }
    
private:
    // Global limits
    GlobalRiskLimits global_limits_;
    
    // Account states
    mutable std::shared_mutex accounts_mutex_;
    std::unordered_map<std::string, std::unique_ptr<AccountRiskState>> accounts_;
    
    // Global risk state (atomic for lock-free reads)
    std::atomic<double> global_exposure_{0.0};
    std::atomic<double> global_daily_pnl_{0.0};
    std::atomic<int> global_open_orders_{0};
    
    // Control flags
    std::atomic<bool> running_{false};
    std::atomic<bool> kill_switch_{false};
    
    // Statistics
    std::atomic<size_t> total_checks_{0};
    std::atomic<size_t> rejected_orders_{0};
    std::atomic<uint64_t> total_latency_ns_{0};
    
    // Helper methods
    bool checkAccountLimits(AccountRiskState* account, const Order& order);
    bool checkGlobalLimits(const Order& order);
    double calculateMarginRequired(const Order& order, int leverage);
    double calculatePositionValue(double quantity, double price);
    void updateGlobalExposure();
    void log(const std::string& message);
    
    // Performance optimization
    size_t hashSymbol(const std::string& symbol);
    AccountRiskState* getAccount(const std::string& account_id);
};

// Risk alert callback
using RiskAlertCallback = std::function<void(const std::string& account_id, 
                                            const std::string& alert_type,
                                            const std::string& message)>;

// Risk monitor for real-time alerts
class RiskMonitor {
public:
    RiskMonitor(MultiAccountRiskEngine* engine);
    
    // Register alert callbacks
    void registerAlertCallback(RiskAlertCallback callback);
    
    // Start/stop monitoring
    void start();
    void stop();
    
    // Alert thresholds
    void setMarginAlertThreshold(double threshold) { margin_alert_threshold_ = threshold; }
    void setDrawdownAlertThreshold(double threshold) { drawdown_alert_threshold_ = threshold; }
    void setExposureAlertThreshold(double threshold) { exposure_alert_threshold_ = threshold; }
    
private:
    MultiAccountRiskEngine* engine_;
    std::vector<RiskAlertCallback> callbacks_;
    std::thread monitor_thread_;
    std::atomic<bool> running_{false};
    
    // Alert thresholds
    double margin_alert_threshold_{0.8};    // 80% margin usage
    double drawdown_alert_threshold_{0.5};  // 50% of daily limit
    double exposure_alert_threshold_{0.9};  // 90% of max exposure
    
    void monitorLoop();
    void checkAndAlert();
};

} // namespace risk
} // namespace oms

#endif // MULTI_ACCOUNT_RISK_ENGINE_H