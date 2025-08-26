#include "account/multi_account_risk_engine.h"
#include <algorithm>
#include <cmath>
#include <immintrin.h> // For SIMD operations
#include <sstream>
#include <thread>

namespace oms {
namespace risk {

MultiAccountRiskEngine::MultiAccountRiskEngine() {
    // Initialize account data
    for (auto& account : accounts_) {
        account.active.store(false);
        account.kill_switch.store(false);
        account.position_count.store(0);
    }
}

bool MultiAccountRiskEngine::AddAccount(const std::string& account_id, 
                                        const AccountRiskMetrics& initial_metrics) {
    std::unique_lock<std::shared_mutex> lock(index_mutex_);
    
    // Find free slot
    size_t free_index = MAX_ACCOUNTS;
    for (size_t i = 0; i < MAX_ACCOUNTS; ++i) {
        if (!accounts_[i].active.load()) {
            free_index = i;
            break;
        }
    }
    
    if (free_index == MAX_ACCOUNTS) {
        return false; // No free slots
    }
    
    // Initialize account
    auto& account = accounts_[free_index];
    account.metrics = initial_metrics;
    account.metrics.account_id = account_id;
    account.position_count.store(0);
    account.kill_switch.store(false);
    account.active.store(true);
    
    // Update index map
    account_index_map_[account_id] = free_index;
    
    return true;
}

bool MultiAccountRiskEngine::RemoveAccount(const std::string& account_id) {
    std::unique_lock<std::shared_mutex> lock(index_mutex_);
    
    auto it = account_index_map_.find(account_id);
    if (it == account_index_map_.end()) {
        return false;
    }
    
    size_t index = it->second;
    accounts_[index].active.store(false);
    accounts_[index].position_count.store(0);
    account_index_map_.erase(it);
    
    return true;
}

bool MultiAccountRiskEngine::UpdateAccountLimits(const std::string& account_id, 
                                                const AccountRiskMetrics& limits) {
    size_t index = GetAccountIndex(account_id);
    if (index == MAX_ACCOUNTS) {
        return false;
    }
    
    auto& account = accounts_[index];
    account.metrics.max_exposure = limits.max_exposure;
    account.metrics.max_position_size = limits.max_position_size;
    account.metrics.daily_loss_limit = limits.daily_loss_limit;
    account.metrics.max_positions = limits.max_positions;
    account.metrics.max_leverage = limits.max_leverage;
    
    return true;
}

bool MultiAccountRiskEngine::UpdatePosition(const PositionRisk& position) {
    size_t index = GetAccountIndex(position.account_id);
    if (index == MAX_ACCOUNTS) {
        return false;
    }
    
    auto& account = accounts_[index];
    size_t pos_count = account.position_count.load();
    
    // Find existing position or free slot
    bool found = false;
    size_t pos_index = pos_count;
    
    for (size_t i = 0; i < pos_count; ++i) {
        if (account.positions[i].symbol == position.symbol) {
            pos_index = i;
            found = true;
            break;
        }
    }
    
    // Add new position if not found
    if (!found && pos_count < MAX_POSITIONS_PER_ACCOUNT) {
        pos_index = account.position_count.fetch_add(1);
    } else if (!found) {
        return false; // Max positions reached
    }
    
    // Update position (atomic operations)
    account.positions[pos_index] = position;
    account.positions[pos_index].timestamp_ns = 
        std::chrono::high_resolution_clock::now().time_since_epoch().count();
    
    // Update account metrics
    UpdateAccountMetrics(account);
    
    // Update aggregated risk
    UpdateAggregatedRisk();
    
    last_update_time_.store(account.positions[pos_index].timestamp_ns);
    
    return true;
}

bool MultiAccountRiskEngine::RemovePosition(const std::string& account_id, 
                                           const std::string& symbol) {
    size_t index = GetAccountIndex(account_id);
    if (index == MAX_ACCOUNTS) {
        return false;
    }
    
    auto& account = accounts_[index];
    size_t pos_count = account.position_count.load();
    
    // Find position
    for (size_t i = 0; i < pos_count; ++i) {
        if (account.positions[i].symbol == symbol) {
            // Move last position to this slot
            if (i < pos_count - 1) {
                account.positions[i] = account.positions[pos_count - 1];
            }
            account.position_count.fetch_sub(1);
            
            // Update metrics
            UpdateAccountMetrics(account);
            UpdateAggregatedRisk();
            
            return true;
        }
    }
    
    return false;
}

RiskCheckResult MultiAccountRiskEngine::CheckOrderRisk(const std::string& account_id, 
                                                       const Order& order) {
    auto start_time = std::chrono::high_resolution_clock::now();
    risk_check_count_.fetch_add(1);
    
    RiskCheckResult result;
    result.account_id = account_id;
    result.timestamp_ns = start_time.time_since_epoch().count();
    
    // Get account
    size_t index = GetAccountIndex(account_id);
    if (index == MAX_ACCOUNTS) {
        result.passed = false;
        result.reason = "Account not found";
        return result;
    }
    
    auto& account = accounts_[index];
    
    // Check kill switch
    if (account.kill_switch.load()) {
        result.passed = false;
        result.reason = "Kill switch active";
        return result;
    }
    
    // Calculate order exposure
    double order_value = order.quantity * order.price;
    double margin_required = CalculateMarginRequirement(order);
    
    // Check exposure limit
    double current_exposure = account.metrics.total_exposure.load();
    if (current_exposure + order_value > account.metrics.max_exposure) {
        result.passed = false;
        result.reason = "Exposure limit exceeded";
        result.current_value = current_exposure + order_value;
        result.limit_value = account.metrics.max_exposure;
        return result;
    }
    
    // Check available margin
    double available_margin = account.metrics.available_margin.load();
    if (margin_required > available_margin) {
        result.passed = false;
        result.reason = "Insufficient margin";
        result.current_value = available_margin;
        result.limit_value = margin_required;
        return result;
    }
    
    // Check position limit
    int32_t open_positions = account.metrics.open_positions.load();
    if (order.side == OrderSide::BUY || order.side == OrderSide::SELL) {
        if (open_positions >= account.metrics.max_positions) {
            result.passed = false;
            result.reason = "Position limit exceeded";
            result.current_value = open_positions;
            result.limit_value = account.metrics.max_positions;
            return result;
        }
    }
    
    // Check daily loss limit
    double daily_pnl = account.metrics.daily_pnl.load();
    if (daily_pnl < -account.metrics.daily_loss_limit) {
        result.passed = false;
        result.reason = "Daily loss limit exceeded";
        result.current_value = -daily_pnl;
        result.limit_value = account.metrics.daily_loss_limit;
        return result;
    }
    
    // Check position size
    if (order_value > account.metrics.max_position_size) {
        result.passed = false;
        result.reason = "Position size too large";
        result.current_value = order_value;
        result.limit_value = account.metrics.max_position_size;
        return result;
    }
    
    result.passed = true;
    result.reason = "All checks passed";
    
    // Track latency
    auto end_time = std::chrono::high_resolution_clock::now();
    auto latency_ns = std::chrono::duration_cast<std::chrono::nanoseconds>(
        end_time - start_time).count();
    total_check_latency_ns_.fetch_add(latency_ns);
    
    // Store result in ring buffer (simple implementation)
    size_t tail = risk_event_tail_.load();
    risk_events_[tail % RISK_EVENT_BUFFER_SIZE] = result;
    risk_event_tail_.store(tail + 1);
    
    return result;
}

RiskCheckResult MultiAccountRiskEngine::CheckPositionRisk(const std::string& account_id, 
                                                         const std::string& symbol) {
    RiskCheckResult result;
    result.account_id = account_id;
    result.timestamp_ns = std::chrono::high_resolution_clock::now().time_since_epoch().count();
    
    size_t index = GetAccountIndex(account_id);
    if (index == MAX_ACCOUNTS) {
        result.passed = false;
        result.reason = "Account not found";
        return result;
    }
    
    auto& account = accounts_[index];
    size_t pos_count = account.position_count.load();
    
    // Find position
    for (size_t i = 0; i < pos_count; ++i) {
        if (account.positions[i].symbol == symbol) {
            const auto& position = account.positions[i];
            
            // Check liquidation risk
            double price_distance = std::abs(position.mark_price - position.liquidation_price);
            double price_percent = price_distance / position.mark_price;
            
            if (price_percent < 0.05) { // Within 5% of liquidation
                result.passed = false;
                result.reason = "Near liquidation";
                result.current_value = price_percent * 100;
                result.limit_value = 5.0;
                return result;
            }
            
            // Check position P&L
            if (position.unrealized_pnl < -account.metrics.max_position_size * 0.1) {
                result.passed = false;
                result.reason = "Position loss too high";
                result.current_value = -position.unrealized_pnl;
                result.limit_value = account.metrics.max_position_size * 0.1;
                return result;
            }
            
            result.passed = true;
            result.reason = "Position healthy";
            return result;
        }
    }
    
    result.passed = false;
    result.reason = "Position not found";
    return result;
}

RiskCheckResult MultiAccountRiskEngine::CheckAccountRisk(const std::string& account_id) {
    RiskCheckResult result;
    result.account_id = account_id;
    result.timestamp_ns = std::chrono::high_resolution_clock::now().time_since_epoch().count();
    
    size_t index = GetAccountIndex(account_id);
    if (index == MAX_ACCOUNTS) {
        result.passed = false;
        result.reason = "Account not found";
        return result;
    }
    
    auto& account = accounts_[index];
    
    // Check margin ratio
    double margin_ratio = GetAccountMarginRatio(account_id);
    if (margin_ratio > 0.8) { // 80% margin used
        result.passed = false;
        result.reason = "High margin usage";
        result.current_value = margin_ratio * 100;
        result.limit_value = 80.0;
        return result;
    }
    
    // Check daily loss
    double daily_pnl = account.metrics.daily_pnl.load();
    if (daily_pnl < -account.metrics.daily_loss_limit * 0.8) {
        result.passed = false;
        result.reason = "Approaching daily loss limit";
        result.current_value = -daily_pnl;
        result.limit_value = account.metrics.daily_loss_limit;
        return result;
    }
    
    // Check drawdown
    double max_drawdown = account.metrics.max_drawdown.load();
    if (max_drawdown > 0.1) { // 10% drawdown
        result.passed = false;
        result.reason = "High drawdown";
        result.current_value = max_drawdown * 100;
        result.limit_value = 10.0;
        return result;
    }
    
    result.passed = true;
    result.reason = "Account healthy";
    return result;
}

bool MultiAccountRiskEngine::IsAccountAtRisk(const std::string& account_id) const {
    size_t index = GetAccountIndex(account_id);
    if (index == MAX_ACCOUNTS) {
        return false;
    }
    
    return accounts_[index].metrics.risk_exceeded.load();
}

double MultiAccountRiskEngine::GetAccountExposure(const std::string& account_id) const {
    size_t index = GetAccountIndex(account_id);
    if (index == MAX_ACCOUNTS) {
        return 0.0;
    }
    
    return accounts_[index].metrics.total_exposure.load();
}

double MultiAccountRiskEngine::GetAccountMarginRatio(const std::string& account_id) const {
    size_t index = GetAccountIndex(account_id);
    if (index == MAX_ACCOUNTS) {
        return 0.0;
    }
    
    const auto& metrics = accounts_[index].metrics;
    double used_margin = metrics.used_margin.load();
    double total_margin = used_margin + metrics.available_margin.load();
    
    return total_margin > 0 ? used_margin / total_margin : 0.0;
}

void MultiAccountRiskEngine::UpdateAggregatedRisk() {
    double total_exposure = 0.0;
    double total_margin_used = 0.0;
    double total_unrealized_pnl = 0.0;
    double total_realized_pnl = 0.0;
    double total_daily_pnl = 0.0;
    int32_t total_positions = 0;
    int32_t accounts_at_risk = 0;
    
    // Sum across all accounts
    for (const auto& account : accounts_) {
        if (!account.active.load()) continue;
        
        total_exposure += account.metrics.total_exposure.load();
        total_margin_used += account.metrics.used_margin.load();
        total_unrealized_pnl += account.metrics.unrealized_pnl.load();
        total_realized_pnl += account.metrics.realized_pnl.load();
        total_daily_pnl += account.metrics.daily_pnl.load();
        total_positions += account.metrics.open_positions.load();
        
        if (account.metrics.risk_exceeded.load()) {
            accounts_at_risk++;
        }
    }
    
    // Update aggregated metrics
    aggregated_risk_.total_exposure.store(total_exposure);
    aggregated_risk_.total_margin_used.store(total_margin_used);
    aggregated_risk_.total_unrealized_pnl.store(total_unrealized_pnl);
    aggregated_risk_.total_realized_pnl.store(total_realized_pnl);
    aggregated_risk_.total_daily_pnl.store(total_daily_pnl);
    aggregated_risk_.total_positions.store(total_positions);
    aggregated_risk_.accounts_at_risk.store(accounts_at_risk);
    aggregated_risk_.last_update_ns.store(
        std::chrono::high_resolution_clock::now().time_since_epoch().count());
    
    // Calculate correlation and concentration risk
    aggregated_risk_.correlation_risk = CalculateCorrelationRisk();
    aggregated_risk_.concentration_risk = CalculateConcentrationRisk();
}

AggregatedRisk MultiAccountRiskEngine::GetAggregatedRisk() const {
    return aggregated_risk_;
}

double MultiAccountRiskEngine::CalculateCorrelationRisk() const {
    // Simplified correlation risk calculation
    // In production, implement proper correlation matrix
    std::vector<std::string> symbols;
    std::unordered_map<std::string, double> symbol_exposure;
    
    // Collect symbol exposures across accounts
    for (const auto& account : accounts_) {
        if (!account.active.load()) continue;
        
        size_t pos_count = account.position_count.load();
        for (size_t i = 0; i < pos_count; ++i) {
            const auto& pos = account.positions[i];
            symbol_exposure[pos.symbol] += pos.position_size * pos.mark_price;
        }
    }
    
    // Calculate concentration in correlated assets
    double btc_exposure = 0.0;
    double eth_exposure = 0.0;
    double total_exposure = aggregated_risk_.total_exposure.load();
    
    for (const auto& [symbol, exposure] : symbol_exposure) {
        if (symbol.find("BTC") != std::string::npos) {
            btc_exposure += exposure;
        } else if (symbol.find("ETH") != std::string::npos) {
            eth_exposure += exposure;
        }
    }
    
    // Correlation risk increases with concentration in correlated assets
    double correlation_concentration = (btc_exposure + eth_exposure) / total_exposure;
    return std::min(correlation_concentration * 1.5, 1.0); // Scale factor
}

double MultiAccountRiskEngine::CalculateConcentrationRisk() const {
    if (aggregated_risk_.total_exposure.load() == 0) return 0.0;
    
    // Calculate Herfindahl index for position concentration
    std::vector<double> position_sizes;
    double total_exposure = 0.0;
    
    for (const auto& account : accounts_) {
        if (!account.active.load()) continue;
        
        size_t pos_count = account.position_count.load();
        for (size_t i = 0; i < pos_count; ++i) {
            double pos_value = account.positions[i].position_size * 
                              account.positions[i].mark_price;
            position_sizes.push_back(pos_value);
            total_exposure += pos_value;
        }
    }
    
    if (total_exposure == 0) return 0.0;
    
    // Calculate Herfindahl index
    double herfindahl = 0.0;
    for (double size : position_sizes) {
        double weight = size / total_exposure;
        herfindahl += weight * weight;
    }
    
    return herfindahl; // Higher value = higher concentration
}

void MultiAccountRiskEngine::EnableKillSwitch(const std::string& account_id) {
    if (account_id.empty()) {
        // Enable for all accounts
        for (auto& account : accounts_) {
            if (account.active.load()) {
                account.kill_switch.store(true);
            }
        }
    } else {
        size_t index = GetAccountIndex(account_id);
        if (index != MAX_ACCOUNTS) {
            accounts_[index].kill_switch.store(true);
        }
    }
}

void MultiAccountRiskEngine::DisableKillSwitch(const std::string& account_id) {
    if (account_id.empty()) {
        // Disable for all accounts
        for (auto& account : accounts_) {
            account.kill_switch.store(false);
        }
    } else {
        size_t index = GetAccountIndex(account_id);
        if (index != MAX_ACCOUNTS) {
            accounts_[index].kill_switch.store(false);
        }
    }
}

bool MultiAccountRiskEngine::IsKillSwitchActive(const std::string& account_id) const {
    if (account_id.empty()) {
        // Check if any account has kill switch active
        for (const auto& account : accounts_) {
            if (account.active.load() && account.kill_switch.load()) {
                return true;
            }
        }
        return false;
    } else {
        size_t index = GetAccountIndex(account_id);
        return index != MAX_ACCOUNTS ? accounts_[index].kill_switch.load() : false;
    }
}

double MultiAccountRiskEngine::GetAverageCheckLatency() const {
    uint64_t count = risk_check_count_.load();
    if (count == 0) return 0.0;
    
    uint64_t total_ns = total_check_latency_ns_.load();
    return static_cast<double>(total_ns) / count / 1000.0; // Convert to microseconds
}

// Private helper functions

size_t MultiAccountRiskEngine::GetAccountIndex(const std::string& account_id) const {
    std::shared_lock<std::shared_mutex> lock(index_mutex_);
    
    auto it = account_index_map_.find(account_id);
    if (it != account_index_map_.end()) {
        return it->second;
    }
    return MAX_ACCOUNTS;
}

void MultiAccountRiskEngine::UpdateAccountMetrics(AccountData& account) {
    double total_exposure = 0.0;
    double used_margin = 0.0;
    double unrealized_pnl = 0.0;
    int32_t open_positions = 0;
    
    size_t pos_count = account.position_count.load();
    
    for (size_t i = 0; i < pos_count; ++i) {
        const auto& pos = account.positions[i];
        double pos_value = std::abs(pos.position_size * pos.mark_price);
        
        total_exposure += pos_value;
        used_margin += pos.margin_used;
        unrealized_pnl += pos.unrealized_pnl;
        open_positions++;
    }
    
    // Update metrics atomically
    account.metrics.total_exposure.store(total_exposure);
    account.metrics.used_margin.store(used_margin);
    account.metrics.unrealized_pnl.store(unrealized_pnl);
    account.metrics.open_positions.store(open_positions);
    
    // Update available margin (simplified calculation)
    double total_balance = 10000.0; // This should come from account balance
    double available_margin = total_balance - used_margin + unrealized_pnl;
    account.metrics.available_margin.store(std::max(0.0, available_margin));
    
    // Check risk status
    bool risk_exceeded = false;
    if (total_exposure > account.metrics.max_exposure) risk_exceeded = true;
    if (available_margin < total_balance * 0.2) risk_exceeded = true; // 20% margin
    if (account.metrics.daily_pnl.load() < -account.metrics.daily_loss_limit) risk_exceeded = true;
    
    account.metrics.risk_exceeded.store(risk_exceeded);
    account.metrics.last_update_ns.store(
        std::chrono::high_resolution_clock::now().time_since_epoch().count());
}

double MultiAccountRiskEngine::CalculatePositionRisk(const PositionRisk& position) const {
    // Calculate position risk score
    double price_risk = std::abs(position.mark_price - position.liquidation_price) / 
                       position.mark_price;
    double size_risk = std::abs(position.position_size * position.mark_price);
    double pnl_risk = position.unrealized_pnl < 0 ? -position.unrealized_pnl : 0;
    
    return price_risk * 0.4 + size_risk * 0.4 + pnl_risk * 0.2;
}

double MultiAccountRiskEngine::CalculateMarginRequirement(const Order& order) const {
    // Simplified margin calculation
    double order_value = order.quantity * order.price;
    double leverage = 10.0; // Default leverage
    
    // Adjust for order type
    double margin_rate = 1.0 / leverage;
    if (order.type == OrderType::MARKET) {
        margin_rate *= 1.1; // 10% extra for market orders
    }
    
    return order_value * margin_rate;
}

bool MultiAccountRiskEngine::ValidateRiskLimits(const AccountData& account, 
                                               double additional_exposure) const {
    double new_exposure = account.metrics.total_exposure.load() + additional_exposure;
    
    // Check exposure limit
    if (new_exposure > account.metrics.max_exposure) return false;
    
    // Check position count
    if (account.metrics.open_positions.load() >= account.metrics.max_positions) return false;
    
    // Check daily loss
    if (account.metrics.daily_pnl.load() < -account.metrics.daily_loss_limit) return false;
    
    return true;
}

// SIMD optimized functions for performance
double MultiAccountRiskEngine::FastSum(const double* values, size_t count) const {
    double sum = 0.0;
    
    // Use AVX for vectorized sum
    #ifdef __AVX__
    size_t simd_count = count - (count % 4);
    __m256d sum_vec = _mm256_setzero_pd();
    
    for (size_t i = 0; i < simd_count; i += 4) {
        __m256d vec = _mm256_loadu_pd(&values[i]);
        sum_vec = _mm256_add_pd(sum_vec, vec);
    }
    
    double temp[4];
    _mm256_storeu_pd(temp, sum_vec);
    sum = temp[0] + temp[1] + temp[2] + temp[3];
    
    // Handle remaining elements
    for (size_t i = simd_count; i < count; ++i) {
        sum += values[i];
    }
    #else
    // Fallback to regular sum
    for (size_t i = 0; i < count; ++i) {
        sum += values[i];
    }
    #endif
    
    return sum;
}

double MultiAccountRiskEngine::FastMax(const double* values, size_t count) const {
    if (count == 0) return 0.0;
    
    double max_val = values[0];
    
    #ifdef __AVX__
    size_t simd_count = count - (count % 4);
    __m256d max_vec = _mm256_set1_pd(max_val);
    
    for (size_t i = 0; i < simd_count; i += 4) {
        __m256d vec = _mm256_loadu_pd(&values[i]);
        max_vec = _mm256_max_pd(max_vec, vec);
    }
    
    double temp[4];
    _mm256_storeu_pd(temp, max_vec);
    max_val = std::max({temp[0], temp[1], temp[2], temp[3]});
    
    // Handle remaining elements
    for (size_t i = simd_count; i < count; ++i) {
        max_val = std::max(max_val, values[i]);
    }
    #else
    // Fallback to regular max
    for (size_t i = 1; i < count; ++i) {
        max_val = std::max(max_val, values[i]);
    }
    #endif
    
    return max_val;
}

// Risk Monitor implementation

MultiAccountRiskMonitor::MultiAccountRiskMonitor(MultiAccountRiskEngine* engine)
    : engine_(engine) {}

MultiAccountRiskMonitor::~MultiAccountRiskMonitor() {
    Stop();
}

void MultiAccountRiskMonitor::Start() {
    if (running_.exchange(true)) return; // Already running
    
    monitor_thread_ = std::thread(&MultiAccountRiskMonitor::MonitorLoop, this);
}

void MultiAccountRiskMonitor::Stop() {
    if (!running_.exchange(false)) return; // Not running
    
    if (monitor_thread_.joinable()) {
        monitor_thread_.join();
    }
}

void MultiAccountRiskMonitor::RegisterRiskCallback(RiskCallback callback) {
    std::unique_lock<std::shared_mutex> lock(callback_mutex_);
    risk_callbacks_.push_back(callback);
}

void MultiAccountRiskMonitor::RegisterAlertCallback(AlertCallback callback) {
    std::unique_lock<std::shared_mutex> lock(callback_mutex_);
    alert_callbacks_.push_back(callback);
}

void MultiAccountRiskMonitor::MonitorLoop() {
    while (running_.load()) {
        CheckAccountRisks();
        CheckAggregatedRisks();
        
        // Sleep for monitoring interval
        std::this_thread::sleep_for(std::chrono::milliseconds(100));
    }
}

void MultiAccountRiskMonitor::CheckAccountRisks() {
    // Implementation of account risk checking
    // This would iterate through accounts and check various risk metrics
    // Triggering callbacks and alerts as needed
}

void MultiAccountRiskMonitor::CheckAggregatedRisks() {
    auto aggregated = engine_->GetAggregatedRisk();
    
    // Check total exposure
    if (aggregated.total_exposure.load() > 1000000) { // Example threshold
        TriggerAlert("SYSTEM", "Total exposure exceeds limit");
    }
    
    // Check correlation risk
    if (aggregated.correlation_risk > 0.7) {
        TriggerAlert("SYSTEM", "High correlation risk detected");
    }
    
    // Check concentration risk
    if (aggregated.concentration_risk > 0.5) {
        TriggerAlert("SYSTEM", "High concentration risk detected");
    }
}

void MultiAccountRiskMonitor::TriggerAlert(const std::string& account_id, 
                                          const std::string& message) {
    std::shared_lock<std::shared_mutex> lock(callback_mutex_);
    
    for (const auto& callback : alert_callbacks_) {
        callback(account_id, message);
    }
}

} // namespace risk
} // namespace oms