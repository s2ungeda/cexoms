#ifndef MULTI_ACCOUNT_RISK_ENGINE_SIMPLE_H
#define MULTI_ACCOUNT_RISK_ENGINE_SIMPLE_H

#include <atomic>
#include <array>
#include <string>
#include <unordered_map>
#include <vector>
#include <chrono>
#include <mutex>
#include "../types.h"

namespace oms {
namespace risk {

// Simplified account risk metrics without atomics in struct
struct AccountRiskData {
    std::string account_id;
    double total_exposure = 0.0;
    double used_margin = 0.0;
    double available_margin = 0.0;
    double unrealized_pnl = 0.0;
    double daily_pnl = 0.0;
    int32_t open_positions = 0;
    
    // Risk limits
    double max_exposure = 1000000.0;
    double max_position_size = 100000.0;
    double daily_loss_limit = 50000.0;
    int32_t max_positions = 20;
    
    bool risk_exceeded = false;
    bool kill_switch = false;
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

// Simplified multi-account risk engine
class MultiAccountRiskEngine {
public:
    static constexpr size_t MAX_ACCOUNTS = 256;
    
    MultiAccountRiskEngine();
    ~MultiAccountRiskEngine() = default;
    
    // Account management
    bool AddAccount(const std::string& account_id, double max_exposure, 
                   double daily_loss_limit, int32_t max_positions);
    bool RemoveAccount(const std::string& account_id);
    
    // Position updates
    bool UpdateAccountMetrics(const std::string& account_id, 
                            double exposure, double pnl, int32_t positions);
    
    // Risk checks (< 50 microseconds target)
    RiskCheckResult CheckOrderRisk(const std::string& account_id, 
                                  double order_value, double margin_required);
    
    // Fast queries
    bool IsAccountAtRisk(const std::string& account_id);
    double GetAccountExposure(const std::string& account_id);
    
    // Kill switch
    void EnableKillSwitch(const std::string& account_id = "");
    void DisableKillSwitch(const std::string& account_id = "");
    bool IsKillSwitchActive(const std::string& account_id = "");
    
    // Stats
    uint64_t GetRiskCheckCount() const { return risk_check_count_.load(); }
    double GetAverageCheckLatency() const;
    
private:
    // Account storage
    std::array<AccountRiskData, MAX_ACCOUNTS> accounts_;
    std::unordered_map<std::string, size_t> account_map_;
    mutable std::mutex mutex_;
    
    // Performance tracking
    std::atomic<uint64_t> risk_check_count_{0};
    std::atomic<uint64_t> total_latency_ns_{0};
    
    // Helper
    size_t GetAccountIndex(const std::string& account_id);
};

} // namespace risk
} // namespace oms

#endif // MULTI_ACCOUNT_RISK_ENGINE_SIMPLE_H