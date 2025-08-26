#include "risk/multi_account_risk_engine.h"
#include <iostream>
#include <algorithm>
#include <cmath>
#include <iomanip>
#include <ctime>

namespace oms {
namespace risk {

// Helper functions for atomic double operations
template<typename T>
static void atomic_add(std::atomic<T>& target, T value) {
    T current = target.load();
    while (!target.compare_exchange_weak(current, current + value));
}

template<typename T>
static void atomic_sub(std::atomic<T>& target, T value) {
    T current = target.load();
    while (!target.compare_exchange_weak(current, current - value));
}

MultiAccountRiskEngine::MultiAccountRiskEngine(const GlobalRiskLimits& global_limits)
    : global_limits_(global_limits) {
    std::cout << "MultiAccountRiskEngine initialized with global limits:" << std::endl;
    std::cout << "  Max total exposure: $" << global_limits_.max_total_exposure << std::endl;
    std::cout << "  Daily loss limit: $" << global_limits_.daily_loss_limit << std::endl;
}

MultiAccountRiskEngine::~MultiAccountRiskEngine() {
    stop();
}

void MultiAccountRiskEngine::addAccount(const std::string& account_id, 
                                       const AccountRiskLimits& limits) {
    std::unique_lock<std::shared_mutex> lock(accounts_mutex_);
    
    auto account = std::make_unique<AccountRiskState>();
    account->account_id = account_id;
    account->limits = limits;
    account->last_update = std::chrono::steady_clock::now();
    
    accounts_[account_id] = std::move(account);
    
    std::cout << "Added account " << account_id << " with limits:" << std::endl;
    std::cout << "  Max position value: $" << limits.max_position_value << std::endl;
    std::cout << "  Max leverage: " << limits.max_leverage << "x" << std::endl;
}

void MultiAccountRiskEngine::removeAccount(const std::string& account_id) {
    std::unique_lock<std::shared_mutex> lock(accounts_mutex_);
    accounts_.erase(account_id);
}

void MultiAccountRiskEngine::updateAccountLimits(const std::string& account_id,
                                               const AccountRiskLimits& limits) {
    std::shared_lock<std::shared_mutex> lock(accounts_mutex_);
    if (auto it = accounts_.find(account_id); it != accounts_.end()) {
        it->second->limits = limits;
    }
}

RiskCheckResult MultiAccountRiskEngine::checkOrder(const std::string& account_id, 
                                                  const Order& order) {
    auto start = std::chrono::high_resolution_clock::now();
    RiskCheckResult result;
    
    // Increment check counter
    total_checks_.fetch_add(1);
    
    // Check kill switch first
    if (kill_switch_.load()) {
        result.passed = false;
        result.reason = "Kill switch activated";
        rejected_orders_.fetch_add(1);
        return result;
    }
    
    // Get account
    AccountRiskState* account = getAccount(account_id);
    if (!account) {
        result.passed = false;
        result.reason = "Account not found";
        rejected_orders_.fetch_add(1);
        return result;
    }
    
    // Check if trading is allowed for this account
    if (!account->trading_allowed.load()) {
        result.passed = false;
        result.reason = "Trading halted for account";
        rejected_orders_.fetch_add(1);
        return result;
    }
    
    // Check account-specific limits
    if (!checkAccountLimits(account, order)) {
        result.passed = false;
        result.reason = "Account risk limit exceeded";
        rejected_orders_.fetch_add(1);
        return result;
    }
    
    // Check global limits
    if (!checkGlobalLimits(order)) {
        result.passed = false;
        result.reason = "Global risk limit exceeded";
        rejected_orders_.fetch_add(1);
        return result;
    }
    
    // Calculate margin required
    int leverage = account->limits.max_leverage;
    // Note: Order struct doesn't have leverage field, using account default
    result.margin_required = calculateMarginRequired(order, leverage);
    
    // Check margin availability
    double available_margin = account->total_margin.load() - 
                            (account->total_exposure.load() / leverage);
    if (result.margin_required > available_margin) {
        result.passed = false;
        result.reason = "Insufficient margin";
        rejected_orders_.fetch_add(1);
        return result;
    }
    
    // All checks passed
    result.passed = true;
    result.available_size = order.quantity;
    
    // Record latency
    auto end = std::chrono::high_resolution_clock::now();
    result.latency = std::chrono::duration_cast<std::chrono::microseconds>(end - start);
    total_latency_ns_.fetch_add(result.latency.count() * 1000);
    
    return result;
}

std::vector<RiskCheckResult> MultiAccountRiskEngine::checkOrderMultiAccount(
    const Order& order, const std::vector<std::string>& accounts) {
    std::vector<RiskCheckResult> results;
    results.reserve(accounts.size());
    
    for (const auto& account_id : accounts) {
        results.push_back(checkOrder(account_id, order));
    }
    
    return results;
}

bool MultiAccountRiskEngine::checkAccountLimits(AccountRiskState* account, 
                                              const Order& order) {
    double order_value = calculatePositionValue(order.quantity, order.price);
    
    // Check max order value
    if (order_value > account->limits.max_order_value) {
        return false;
    }
    
    // Check max position value
    auto pos_it = account->positions.find(order.symbol);
    double current_position_value = 0.0;
    if (pos_it != account->positions.end()) {
        current_position_value = pos_it->second.notional.load();
    }
    
    // For opposite side orders, calculate net position
    double net_position_value = current_position_value;
    if (pos_it != account->positions.end()) {
        double current_qty = pos_it->second.quantity.load();
        if ((order.side == Side::BUY && current_qty < 0) ||
            (order.side == Side::SELL && current_qty > 0)) {
            // Reducing position
            net_position_value = std::abs(current_qty - order.quantity) * order.price;
        } else {
            // Increasing position
            net_position_value = (std::abs(current_qty) + order.quantity) * order.price;
        }
    } else {
        net_position_value = order_value;
    }
    
    if (net_position_value > account->limits.max_position_value) {
        return false;
    }
    
    // Check total exposure
    double new_exposure = account->total_exposure.load() + order_value;
    if (new_exposure > account->limits.max_total_exposure) {
        return false;
    }
    
    // Check daily loss
    if (account->daily_pnl.load() < -account->limits.daily_loss_limit) {
        return false;
    }
    
    // Check open orders limit
    if (account->open_orders.load() >= account->limits.max_open_orders) {
        return false;
    }
    
    // Check positions limit
    if (pos_it == account->positions.end() && 
        account->positions.size() >= account->limits.max_positions) {
        return false;
    }
    
    return true;
}

bool MultiAccountRiskEngine::checkGlobalLimits(const Order& order) {
    double order_value = calculatePositionValue(order.quantity, order.price);
    
    // Check global exposure
    double new_global_exposure = global_exposure_.load() + order_value;
    if (new_global_exposure > global_limits_.max_total_exposure) {
        return false;
    }
    
    // Check global daily loss
    if (global_daily_pnl_.load() < -global_limits_.daily_loss_limit) {
        return false;
    }
    
    // Check total orders
    if (global_open_orders_.load() >= global_limits_.max_total_orders) {
        return false;
    }
    
    return true;
}

void MultiAccountRiskEngine::updatePosition(const std::string& account_id,
                                          const std::string& symbol,
                                          double quantity, double price, 
                                          double mark_price) {
    AccountRiskState* account = getAccount(account_id);
    if (!account) return;
    
    auto& position = account->positions[symbol];
    double old_quantity = position.quantity.load();
    double old_notional = position.notional.load();
    
    // Update position atomically
    position.quantity.store(quantity);
    position.notional.store(std::abs(quantity) * mark_price);
    position.entry_price.store(price);
    position.mark_price.store(mark_price);
    
    // Update unrealized PnL
    if (quantity != 0) {
        double unrealized_pnl = 0.0;
        if (quantity > 0) {
            unrealized_pnl = (mark_price - price) * quantity;
        } else {
            unrealized_pnl = (price - mark_price) * std::abs(quantity);
        }
        position.unrealized_pnl.store(unrealized_pnl);
    } else {
        position.unrealized_pnl.store(0.0);
    }
    
    // Update account exposure
    double exposure_delta = position.notional.load() - old_notional;
    atomic_add(account->total_exposure, exposure_delta);
    atomic_add(global_exposure_, exposure_delta);
    
    // Update timestamp
    account->last_update = std::chrono::steady_clock::now();
}

void MultiAccountRiskEngine::closePosition(const std::string& account_id,
                                         const std::string& symbol) {
    AccountRiskState* account = getAccount(account_id);
    if (!account) return;
    
    auto it = account->positions.find(symbol);
    if (it != account->positions.end()) {
        double notional = it->second.notional.load();
        atomic_sub(account->total_exposure, notional);
        atomic_sub(global_exposure_, notional);
        account->positions.erase(it);
    }
}

void MultiAccountRiskEngine::incrementOrderCount(const std::string& account_id) {
    AccountRiskState* account = getAccount(account_id);
    if (account) {
        account->open_orders.fetch_add(1);
        global_open_orders_.fetch_add(1);
    }
}

void MultiAccountRiskEngine::decrementOrderCount(const std::string& account_id) {
    AccountRiskState* account = getAccount(account_id);
    if (account) {
        account->open_orders.fetch_sub(1);
        global_open_orders_.fetch_sub(1);
    }
}

void MultiAccountRiskEngine::updatePnL(const std::string& account_id,
                                     const std::string& symbol,
                                     double realized_pnl, 
                                     double unrealized_pnl) {
    AccountRiskState* account = getAccount(account_id);
    if (!account) return;
    
    // Update position PnL
    auto it = account->positions.find(symbol);
    if (it != account->positions.end()) {
        double old_unrealized = it->second.unrealized_pnl.load();
        it->second.unrealized_pnl.store(unrealized_pnl);
        atomic_add(it->second.realized_pnl, realized_pnl);
        
        // Update account daily PnL
        atomic_add(account->daily_pnl, realized_pnl);
        atomic_add(global_daily_pnl_, realized_pnl);
    }
}

RiskMetrics MultiAccountRiskEngine::getAccountMetrics(const std::string& account_id) const {
    RiskMetrics metrics{};
    
    std::shared_lock<std::shared_mutex> lock(accounts_mutex_);
    auto it = accounts_.find(account_id);
    if (it == accounts_.end()) {
        return metrics;
    }
    
    const auto& account = it->second;
    metrics.total_exposure = account->total_exposure.load();
    metrics.total_positions = account->positions.size();
    metrics.total_orders = account->open_orders.load();
    
    // Calculate total PnL and margin
    for (const auto& [symbol, position] : account->positions) {
        metrics.total_unrealized_pnl += position.unrealized_pnl.load();
        metrics.total_realized_pnl += position.realized_pnl.load();
        metrics.total_margin_used += position.margin_used.load();
    }
    
    // Calculate ratios
    if (account->total_margin.load() > 0) {
        metrics.margin_ratio = metrics.total_margin_used / account->total_margin.load();
        metrics.leverage_ratio = metrics.total_exposure / account->total_margin.load();
    }
    
    return metrics;
}

RiskMetrics MultiAccountRiskEngine::getGlobalMetrics() const {
    RiskMetrics metrics{};
    
    metrics.total_exposure = global_exposure_.load();
    metrics.total_orders = global_open_orders_.load();
    
    std::shared_lock<std::shared_mutex> lock(accounts_mutex_);
    for (const auto& [account_id, account] : accounts_) {
        for (const auto& [symbol, position] : account->positions) {
            metrics.total_unrealized_pnl += position.unrealized_pnl.load();
            metrics.total_realized_pnl += position.realized_pnl.load();
            metrics.total_margin_used += position.margin_used.load();
        }
        metrics.total_positions += account->positions.size();
    }
    
    return metrics;
}

void MultiAccountRiskEngine::haltTrading(const std::string& account_id) {
    AccountRiskState* account = getAccount(account_id);
    if (account) {
        account->trading_allowed.store(false);
        log("Trading halted for account: " + account_id);
    }
}

void MultiAccountRiskEngine::resumeTrading(const std::string& account_id) {
    AccountRiskState* account = getAccount(account_id);
    if (account) {
        account->trading_allowed.store(true);
        log("Trading resumed for account: " + account_id);
    }
}

void MultiAccountRiskEngine::haltAllTrading() {
    std::shared_lock<std::shared_mutex> lock(accounts_mutex_);
    for (const auto& [account_id, account] : accounts_) {
        account->trading_allowed.store(false);
    }
    log("Trading halted for all accounts");
}

void MultiAccountRiskEngine::resumeAllTrading() {
    std::shared_lock<std::shared_mutex> lock(accounts_mutex_);
    for (const auto& [account_id, account] : accounts_) {
        account->trading_allowed.store(true);
    }
    log("Trading resumed for all accounts");
}

bool MultiAccountRiskEngine::isTradingAllowed(const std::string& account_id) const {
    AccountRiskState* account = const_cast<MultiAccountRiskEngine*>(this)->getAccount(account_id);
    return account ? account->trading_allowed.load() : false;
}

void MultiAccountRiskEngine::activateKillSwitch() {
    kill_switch_.store(true);
    haltAllTrading();
    log("KILL SWITCH ACTIVATED - All trading halted");
}

void MultiAccountRiskEngine::resetDailyPnL() {
    std::shared_lock<std::shared_mutex> lock(accounts_mutex_);
    for (const auto& [account_id, account] : accounts_) {
        account->daily_pnl.store(0.0);
        for (auto& [symbol, position] : account->positions) {
            position.realized_pnl.store(0.0);
        }
    }
    global_daily_pnl_.store(0.0);
    log("Daily PnL reset for all accounts");
}

void MultiAccountRiskEngine::resetDailyPnL(const std::string& account_id) {
    AccountRiskState* account = getAccount(account_id);
    if (account) {
        double old_pnl = account->daily_pnl.load();
        account->daily_pnl.store(0.0);
        atomic_sub(global_daily_pnl_, old_pnl);
        
        for (auto& [symbol, position] : account->positions) {
            position.realized_pnl.store(0.0);
        }
        log("Daily PnL reset for account: " + account_id);
    }
}

double MultiAccountRiskEngine::getAverageLatencyUs() const {
    size_t checks = total_checks_.load();
    if (checks == 0) return 0.0;
    
    uint64_t total_ns = total_latency_ns_.load();
    return static_cast<double>(total_ns) / (checks * 1000.0);
}

void MultiAccountRiskEngine::start() {
    running_.store(true);
    kill_switch_.store(false);
    log("MultiAccountRiskEngine started");
}

void MultiAccountRiskEngine::stop() {
    running_.store(false);
    log("MultiAccountRiskEngine stopped");
}

double MultiAccountRiskEngine::calculateMarginRequired(const Order& order, int leverage) {
    double notional = calculatePositionValue(order.quantity, order.price);
    return notional / leverage;
}

double MultiAccountRiskEngine::calculatePositionValue(double quantity, double price) {
    return std::abs(quantity * price);
}

void MultiAccountRiskEngine::updateGlobalExposure() {
    double total = 0.0;
    std::shared_lock<std::shared_mutex> lock(accounts_mutex_);
    for (const auto& [account_id, account] : accounts_) {
        total += account->total_exposure.load();
    }
    global_exposure_.store(total);
}

AccountRiskState* MultiAccountRiskEngine::getAccount(const std::string& account_id) {
    std::shared_lock<std::shared_mutex> lock(accounts_mutex_);
    auto it = accounts_.find(account_id);
    return (it != accounts_.end()) ? it->second.get() : nullptr;
}

size_t MultiAccountRiskEngine::hashSymbol(const std::string& symbol) {
    return std::hash<std::string>{}(symbol) % MAX_SYMBOLS;
}

void MultiAccountRiskEngine::log(const std::string& message) {
    auto now = std::chrono::system_clock::now();
    auto time_t = std::chrono::system_clock::to_time_t(now);
    std::cout << "[" << std::put_time(std::localtime(&time_t), "%Y-%m-%d %H:%M:%S") 
              << "] [RISK] " << message << std::endl;
}

// Risk Monitor implementation

RiskMonitor::RiskMonitor(MultiAccountRiskEngine* engine) : engine_(engine) {}

void RiskMonitor::registerAlertCallback(RiskAlertCallback callback) {
    callbacks_.push_back(callback);
}

void RiskMonitor::start() {
    running_.store(true);
    monitor_thread_ = std::thread(&RiskMonitor::monitorLoop, this);
}

void RiskMonitor::stop() {
    running_.store(false);
    if (monitor_thread_.joinable()) {
        monitor_thread_.join();
    }
}

void RiskMonitor::monitorLoop() {
    while (running_.load()) {
        checkAndAlert();
        std::this_thread::sleep_for(std::chrono::seconds(1));
    }
}

void RiskMonitor::checkAndAlert() {
    // Get global metrics
    auto global_metrics = engine_->getGlobalMetrics();
    
    // Check global exposure
    if (global_metrics.total_exposure > engine_->getGlobalLimits().max_total_exposure * exposure_alert_threshold_) {
        for (const auto& callback : callbacks_) {
            callback("GLOBAL", "HIGH_EXPOSURE", 
                    "Global exposure at " + std::to_string(global_metrics.total_exposure));
        }
    }
    
    // Check each account
    // Implementation would iterate through accounts and check individual metrics
}

} // namespace risk
} // namespace oms