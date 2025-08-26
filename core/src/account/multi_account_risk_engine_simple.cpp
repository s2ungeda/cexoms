#include "account/multi_account_risk_engine_simple.h"
#include <algorithm>
#include <chrono>

namespace oms {
namespace risk {

MultiAccountRiskEngine::MultiAccountRiskEngine() {
    // Initialize accounts
    for (auto& account : accounts_) {
        account.account_id.clear();
    }
}

bool MultiAccountRiskEngine::AddAccount(const std::string& account_id, 
                                       double max_exposure, 
                                       double daily_loss_limit, 
                                       int32_t max_positions) {
    std::lock_guard<std::mutex> lock(mutex_);
    
    // Check if account already exists
    if (account_map_.find(account_id) != account_map_.end()) {
        return false;
    }
    
    // Find free slot
    size_t free_index = MAX_ACCOUNTS;
    for (size_t i = 0; i < MAX_ACCOUNTS; ++i) {
        if (accounts_[i].account_id.empty()) {
            free_index = i;
            break;
        }
    }
    
    if (free_index == MAX_ACCOUNTS) {
        return false; // No free slots
    }
    
    // Initialize account
    auto& account = accounts_[free_index];
    account.account_id = account_id;
    account.max_exposure = max_exposure;
    account.daily_loss_limit = daily_loss_limit;
    account.max_positions = max_positions;
    account.total_exposure = 0.0;
    account.daily_pnl = 0.0;
    account.open_positions = 0;
    account.risk_exceeded = false;
    account.kill_switch = false;
    
    // Update map
    account_map_[account_id] = free_index;
    
    return true;
}

bool MultiAccountRiskEngine::RemoveAccount(const std::string& account_id) {
    std::lock_guard<std::mutex> lock(mutex_);
    
    auto it = account_map_.find(account_id);
    if (it == account_map_.end()) {
        return false;
    }
    
    size_t index = it->second;
    accounts_[index].account_id.clear();
    account_map_.erase(it);
    
    return true;
}

bool MultiAccountRiskEngine::UpdateAccountMetrics(const std::string& account_id, 
                                                 double exposure, 
                                                 double pnl, 
                                                 int32_t positions) {
    std::lock_guard<std::mutex> lock(mutex_);
    
    size_t index = GetAccountIndex(account_id);
    if (index == MAX_ACCOUNTS) {
        return false;
    }
    
    auto& account = accounts_[index];
    account.total_exposure = exposure;
    account.daily_pnl = pnl;
    account.open_positions = positions;
    
    // Check risk status
    account.risk_exceeded = false;
    if (exposure > account.max_exposure) {
        account.risk_exceeded = true;
    }
    if (pnl < -account.daily_loss_limit) {
        account.risk_exceeded = true;
    }
    
    return true;
}

RiskCheckResult MultiAccountRiskEngine::CheckOrderRisk(const std::string& account_id, 
                                                      double order_value, 
                                                      double margin_required) {
    auto start_time = std::chrono::high_resolution_clock::now();
    risk_check_count_.fetch_add(1);
    
    RiskCheckResult result;
    result.account_id = account_id;
    result.timestamp_ns = start_time.time_since_epoch().count();
    
    {
        std::lock_guard<std::mutex> lock(mutex_);
        
        size_t index = GetAccountIndex(account_id);
        if (index == MAX_ACCOUNTS) {
            result.passed = false;
            result.reason = "Account not found";
            return result;
        }
        
        const auto& account = accounts_[index];
        
        // Check kill switch
        if (account.kill_switch) {
            result.passed = false;
            result.reason = "Kill switch active";
            return result;
        }
        
        // Check exposure limit
        double new_exposure = account.total_exposure + order_value;
        if (new_exposure > account.max_exposure) {
            result.passed = false;
            result.reason = "Exposure limit exceeded";
            result.current_value = new_exposure;
            result.limit_value = account.max_exposure;
            return result;
        }
        
        // Check daily loss
        if (account.daily_pnl < -account.daily_loss_limit) {
            result.passed = false;
            result.reason = "Daily loss limit exceeded";
            result.current_value = -account.daily_pnl;
            result.limit_value = account.daily_loss_limit;
            return result;
        }
        
        // Check position limit
        if (account.open_positions >= account.max_positions) {
            result.passed = false;
            result.reason = "Position limit exceeded";
            result.current_value = account.open_positions;
            result.limit_value = account.max_positions;
            return result;
        }
        
        result.passed = true;
        result.reason = "All checks passed";
    }
    
    // Track latency
    auto end_time = std::chrono::high_resolution_clock::now();
    auto latency_ns = std::chrono::duration_cast<std::chrono::nanoseconds>(
        end_time - start_time).count();
    total_latency_ns_.fetch_add(latency_ns);
    
    return result;
}

bool MultiAccountRiskEngine::IsAccountAtRisk(const std::string& account_id) {
    std::lock_guard<std::mutex> lock(mutex_);
    
    size_t index = GetAccountIndex(account_id);
    if (index == MAX_ACCOUNTS) {
        return false;
    }
    
    return accounts_[index].risk_exceeded;
}

double MultiAccountRiskEngine::GetAccountExposure(const std::string& account_id) {
    std::lock_guard<std::mutex> lock(mutex_);
    
    size_t index = GetAccountIndex(account_id);
    if (index == MAX_ACCOUNTS) {
        return 0.0;
    }
    
    return accounts_[index].total_exposure;
}

void MultiAccountRiskEngine::EnableKillSwitch(const std::string& account_id) {
    std::lock_guard<std::mutex> lock(mutex_);
    
    if (account_id.empty()) {
        // Enable for all accounts
        for (auto& account : accounts_) {
            if (!account.account_id.empty()) {
                account.kill_switch = true;
            }
        }
    } else {
        size_t index = GetAccountIndex(account_id);
        if (index != MAX_ACCOUNTS) {
            accounts_[index].kill_switch = true;
        }
    }
}

void MultiAccountRiskEngine::DisableKillSwitch(const std::string& account_id) {
    std::lock_guard<std::mutex> lock(mutex_);
    
    if (account_id.empty()) {
        // Disable for all accounts
        for (auto& account : accounts_) {
            account.kill_switch = false;
        }
    } else {
        size_t index = GetAccountIndex(account_id);
        if (index != MAX_ACCOUNTS) {
            accounts_[index].kill_switch = false;
        }
    }
}

bool MultiAccountRiskEngine::IsKillSwitchActive(const std::string& account_id) {
    std::lock_guard<std::mutex> lock(mutex_);
    
    if (account_id.empty()) {
        // Check if any account has kill switch active
        for (const auto& account : accounts_) {
            if (!account.account_id.empty() && account.kill_switch) {
                return true;
            }
        }
        return false;
    } else {
        size_t index = GetAccountIndex(account_id);
        return index != MAX_ACCOUNTS ? accounts_[index].kill_switch : false;
    }
}

double MultiAccountRiskEngine::GetAverageCheckLatency() const {
    uint64_t count = risk_check_count_.load();
    if (count == 0) return 0.0;
    
    uint64_t total_ns = total_latency_ns_.load();
    return static_cast<double>(total_ns) / count / 1000.0; // Convert to microseconds
}

size_t MultiAccountRiskEngine::GetAccountIndex(const std::string& account_id) {
    auto it = account_map_.find(account_id);
    if (it != account_map_.end()) {
        return it->second;
    }
    return MAX_ACCOUNTS;
}

} // namespace risk
} // namespace oms