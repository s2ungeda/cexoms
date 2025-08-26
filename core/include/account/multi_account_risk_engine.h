#ifndef MULTI_ACCOUNT_RISK_ENGINE_H
#define MULTI_ACCOUNT_RISK_ENGINE_H

#include <atomic>
#include <array>
#include <memory>
#include <string>
#include <unordered_map>
#include <vector>
#include <chrono>
#include <shared_mutex>
#include <functional>
#include <thread>
#include "../types.h"
#include "../ring_buffer.h"

namespace oms {
namespace risk {

// Account risk metrics
struct AccountRiskMetrics {
    std::string account_id;
    std::atomic<double> total_exposure{0.0};
    std::atomic<double> used_margin{0.0};
    std::atomic<double> available_margin{0.0};
    std::atomic<double> unrealized_pnl{0.0};
    std::atomic<double> realized_pnl{0.0};
    std::atomic<double> daily_pnl{0.0};
    std::atomic<double> max_drawdown{0.0};
    std::atomic<int32_t> open_positions{0};
    std::atomic<int32_t> daily_trades{0};
    std::atomic<uint64_t> last_update_ns{0};
    
    // Risk limits
    double max_exposure;
    double max_position_size;
    double daily_loss_limit;
    int32_t max_positions;
    int32_t max_leverage;
    
    // Risk states
    std::atomic<bool> risk_exceeded{false};
    std::atomic<bool> margin_call{false};
    std::atomic<bool> auto_deleverage{false};
};

// Position risk data
struct PositionRisk {
    std::string symbol;
    std::string account_id;
    double position_size;
    double entry_price;
    double mark_price;
    double liquidation_price;
    double unrealized_pnl;
    double margin_used;
    int32_t leverage;
    uint64_t timestamp_ns;
};

// Aggregated risk across all accounts
struct AggregatedRisk {
    std::atomic<double> total_exposure{0.0};
    std::atomic<double> total_margin_used{0.0};
    std::atomic<double> total_unrealized_pnl{0.0};
    std::atomic<double> total_realized_pnl{0.0};
    std::atomic<double> total_daily_pnl{0.0};
    std::atomic<int32_t> total_positions{0};
    std::atomic<int32_t> accounts_at_risk{0};
    std::atomic<uint64_t> last_update_ns{0};
    
    // Cross-account correlations
    double correlation_risk;
    double concentration_risk;
    double systematic_risk;
};

// Risk check result
struct RiskCheckResult {
    bool passed;
    std::string account_id;
    std::string reason;
    double current_value;
    double limit_value;
    uint64_t timestamp_ns;
};

// Multi-account risk engine
class MultiAccountRiskEngine {
public:
    static constexpr size_t MAX_ACCOUNTS = 256;
    static constexpr size_t MAX_POSITIONS_PER_ACCOUNT = 100;
    static constexpr size_t RISK_EVENT_BUFFER_SIZE = 10000;
    static constexpr uint64_t RISK_UPDATE_INTERVAL_NS = 1000000; // 1ms
    
    MultiAccountRiskEngine();
    ~MultiAccountRiskEngine() = default;
    
    // Account management
    bool AddAccount(const std::string& account_id, const AccountRiskMetrics& initial_metrics);
    bool RemoveAccount(const std::string& account_id);
    bool UpdateAccountLimits(const std::string& account_id, const AccountRiskMetrics& limits);
    
    // Position updates (lock-free)
    bool UpdatePosition(const PositionRisk& position);
    bool RemovePosition(const std::string& account_id, const std::string& symbol);
    
    // Risk checks (< 50 microseconds)
    RiskCheckResult CheckOrderRisk(const std::string& account_id, const Order& order);
    RiskCheckResult CheckPositionRisk(const std::string& account_id, const std::string& symbol);
    RiskCheckResult CheckAccountRisk(const std::string& account_id);
    
    // Fast risk queries
    bool IsAccountAtRisk(const std::string& account_id) const;
    double GetAccountExposure(const std::string& account_id) const;
    double GetAccountMarginRatio(const std::string& account_id) const;
    
    // Aggregated risk
    void UpdateAggregatedRisk();
    AggregatedRisk GetAggregatedRisk() const;
    double CalculateCorrelationRisk() const;
    double CalculateConcentrationRisk() const;
    
    // Emergency controls
    void EnableKillSwitch(const std::string& account_id = "");
    void DisableKillSwitch(const std::string& account_id = "");
    bool IsKillSwitchActive(const std::string& account_id = "") const;
    
    // Performance metrics
    uint64_t GetLastUpdateTime() const { return last_update_time_.load(); }
    uint64_t GetRiskCheckCount() const { return risk_check_count_.load(); }
    double GetAverageCheckLatency() const;
    
private:
    // Account storage (optimized for cache locality)
    struct AccountData {
        AccountRiskMetrics metrics;
        std::array<PositionRisk, MAX_POSITIONS_PER_ACCOUNT> positions;
        std::atomic<size_t> position_count{0};
        std::atomic<bool> active{false};
        std::atomic<bool> kill_switch{false};
        alignas(64) char padding[64]; // Prevent false sharing
    };
    
    // Lock-free data structures
    std::array<AccountData, MAX_ACCOUNTS> accounts_;
    std::unordered_map<std::string, size_t> account_index_map_;
    mutable std::shared_mutex index_mutex_;
    
    // Aggregated risk data
    AggregatedRisk aggregated_risk_;
    
    // Risk event buffer (using simple ring buffer for now)
    std::array<RiskCheckResult, RISK_EVENT_BUFFER_SIZE> risk_events_;
    std::atomic<size_t> risk_event_head_{0};
    std::atomic<size_t> risk_event_tail_{0};
    
    // Performance tracking
    std::atomic<uint64_t> last_update_time_{0};
    std::atomic<uint64_t> risk_check_count_{0};
    std::atomic<uint64_t> total_check_latency_ns_{0};
    
    // Helper functions
    size_t GetAccountIndex(const std::string& account_id) const;
    void UpdateAccountMetrics(AccountData& account);
    double CalculatePositionRisk(const PositionRisk& position) const;
    double CalculateMarginRequirement(const Order& order) const;
    bool ValidateRiskLimits(const AccountData& account, double additional_exposure) const;
    
    // Fast math helpers (using SIMD where possible)
    double FastSum(const double* values, size_t count) const;
    double FastMax(const double* values, size_t count) const;
    double FastStdDev(const double* values, size_t count, double mean) const;
};

// Risk monitor for continuous monitoring
class MultiAccountRiskMonitor {
public:
    using RiskCallback = std::function<void(const std::string&, const AccountRiskMetrics&)>;
    using AlertCallback = std::function<void(const std::string&, const std::string&)>;
    
    MultiAccountRiskMonitor(MultiAccountRiskEngine* engine);
    ~MultiAccountRiskMonitor();
    
    // Start/stop monitoring
    void Start();
    void Stop();
    
    // Register callbacks
    void RegisterRiskCallback(RiskCallback callback);
    void RegisterAlertCallback(AlertCallback callback);
    
    // Configure thresholds
    void SetMarginCallThreshold(double threshold) { margin_call_threshold_ = threshold; }
    void SetExposureAlertThreshold(double threshold) { exposure_alert_threshold_ = threshold; }
    void SetDrawdownAlertThreshold(double threshold) { drawdown_alert_threshold_ = threshold; }
    
private:
    MultiAccountRiskEngine* engine_;
    std::atomic<bool> running_{false};
    std::thread monitor_thread_;
    
    // Callbacks
    std::vector<RiskCallback> risk_callbacks_;
    std::vector<AlertCallback> alert_callbacks_;
    mutable std::shared_mutex callback_mutex_;
    
    // Thresholds
    std::atomic<double> margin_call_threshold_{0.8};      // 80% margin used
    std::atomic<double> exposure_alert_threshold_{0.9};   // 90% of max exposure
    std::atomic<double> drawdown_alert_threshold_{0.05};  // 5% drawdown
    
    // Monitoring loop
    void MonitorLoop();
    void CheckAccountRisks();
    void CheckAggregatedRisks();
    void TriggerAlert(const std::string& account_id, const std::string& message);
};

} // namespace risk
} // namespace oms

#endif // MULTI_ACCOUNT_RISK_ENGINE_H