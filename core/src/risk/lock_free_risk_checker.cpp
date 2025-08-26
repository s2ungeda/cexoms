#include "risk/lock_free_risk_checker.h"
#include <cstring>
#include <algorithm>

namespace oms {
namespace risk {

LockFreeRiskChecker::LockFreeRiskChecker() {
    // Initialize all accounts
    for (auto& account : accounts_) {
        account.trading_enabled.store(true);
        account.last_update_ns.store(0);
        
        // Initialize symbol hashes to 0 (empty)
        for (auto& hash : account.symbol_hashes) {
            hash.store(0);
        }
    }
    
    resetStats();
}

LockFreeRiskChecker::RiskCheckResult LockFreeRiskChecker::checkOrderFast(
    uint32_t account_hash, 
    uint32_t symbol_hash,
    double quantity, 
    double price,
    Side side,
    int leverage) {
    
    // Start timing
    auto start = std::chrono::high_resolution_clock::now();
    
    // Initialize all variables at the beginning to avoid goto issues
    RiskCheckResult result;
    result.passed = false;
    std::memset(result.reason, 0, sizeof(result.reason));
    
    double notional = 0.0;
    uint32_t account_idx = 0;
    uint32_t pos_slot = 0;
    uint32_t stored_hash = 0;
    double current_quantity = 0.0;
    double current_notional = 0.0;
    double new_quantity = 0.0;
    double new_notional = 0.0;
    double exposure_delta = 0.0;
    double current_exposure = 0.0;
    double new_exposure = 0.0;
    double daily_pnl = 0.0;
    double available_balance = 0.0;
    double margin_used = 0.0;
    double new_margin_used = 0.0;
    double margin_ratio = 0.0;
    
    // Increment check counter
    stats_.total_checks.fetch_add(1, std::memory_order_relaxed);
    
    // Get account data
    account_idx = getAccountIndex(account_hash);
    auto& account = accounts_[account_idx];
    
    // Check if account is enabled
    if (!account.trading_enabled.load(std::memory_order_acquire)) {
        std::strncpy(result.reason, "Account disabled", sizeof(result.reason) - 1);
        stats_.failed_checks.fetch_add(1, std::memory_order_relaxed);
        goto end;
    }
    
    // Calculate notional value
    notional = std::abs(quantity * price);
    
    // Check position limit
    if (notional > max_position_value_.load(std::memory_order_relaxed)) {
        std::strncpy(result.reason, "Position too large", sizeof(result.reason) - 1);
        stats_.failed_checks.fetch_add(1, std::memory_order_relaxed);
        goto end;
    }
    
    // Get current position
    pos_slot = getPositionSlot(symbol_hash, account_idx);
    
    // Check if this is the correct symbol (collision handling)
    stored_hash = account.symbol_hashes[pos_slot].load(std::memory_order_acquire);
    
    if (stored_hash == symbol_hash) {
        current_quantity = account.positions[pos_slot].quantity.load(std::memory_order_acquire);
        current_notional = account.positions[pos_slot].notional.load(std::memory_order_acquire);
    }
    
    // Calculate new position
    new_quantity = current_quantity;
    new_notional = current_notional;
    
    if ((side == Side::BUY && current_quantity >= 0) ||
        (side == Side::SELL && current_quantity <= 0)) {
        // Increasing position
        new_quantity = current_quantity + (side == Side::BUY ? quantity : -quantity);
        new_notional = std::abs(new_quantity * price);
    } else {
        // Reducing or reversing position
        double abs_new_qty = std::abs(current_quantity) - quantity;
        if (abs_new_qty < 0) {
            // Reversing position
            new_quantity = side == Side::BUY ? -abs_new_qty : abs_new_qty;
            new_notional = std::abs(new_quantity * price);
        } else {
            new_quantity = current_quantity > 0 ? abs_new_qty : -abs_new_qty;
            new_notional = abs_new_qty * price;
        }
    }
    
    // Check new position limit
    if (new_notional > max_position_value_.load(std::memory_order_relaxed)) {
        std::strncpy(result.reason, "Would exceed position limit", sizeof(result.reason) - 1);
        stats_.failed_checks.fetch_add(1, std::memory_order_relaxed);
        goto end;
    }
    
    // Calculate exposure change
    exposure_delta = new_notional - current_notional;
    current_exposure = account.total_exposure.load(std::memory_order_acquire);
    new_exposure = current_exposure + exposure_delta;
    
    // Check account exposure limit
    if (new_exposure > max_account_exposure_.load(std::memory_order_relaxed)) {
        std::strncpy(result.reason, "Would exceed exposure limit", sizeof(result.reason) - 1);
        stats_.failed_checks.fetch_add(1, std::memory_order_relaxed);
        goto end;
    }
    
    // Check daily loss limit
    daily_pnl = account.daily_pnl.load(std::memory_order_acquire);
    if (daily_pnl < -max_daily_loss_.load(std::memory_order_relaxed)) {
        std::strncpy(result.reason, "Daily loss limit reached", sizeof(result.reason) - 1);
        stats_.failed_checks.fetch_add(1, std::memory_order_relaxed);
        goto end;
    }
    
    // Calculate margin
    result.margin_required = calculateMarginRequired(notional, leverage);
    available_balance = account.available_balance.load(std::memory_order_acquire);
    margin_used = account.margin_used.load(std::memory_order_acquire);
    result.available_margin = available_balance - margin_used;
    
    // Check margin requirement
    if (result.margin_required > result.available_margin) {
        std::strncpy(result.reason, "Insufficient margin", sizeof(result.reason) - 1);
        stats_.failed_checks.fetch_add(1, std::memory_order_relaxed);
        goto end;
    }
    
    // Check minimum margin ratio
    new_margin_used = margin_used + result.margin_required;
    margin_ratio = new_margin_used / available_balance;
    if (margin_ratio > (1.0 - min_margin_ratio_.load(std::memory_order_relaxed))) {
        std::strncpy(result.reason, "Would exceed margin limit", sizeof(result.reason) - 1);
        stats_.failed_checks.fetch_add(1, std::memory_order_relaxed);
        goto end;
    }
    
    // All checks passed
    result.passed = true;
    stats_.passed_checks.fetch_add(1, std::memory_order_relaxed);
    
end:
    // Calculate latency
    auto end = std::chrono::high_resolution_clock::now();
    result.latency = std::chrono::duration_cast<std::chrono::nanoseconds>(end - start);
    updateLatencyStats(result.latency.count());
    
    return result;
}

void LockFreeRiskChecker::updateAccountData(uint32_t account_hash,
                                          double total_exposure,
                                          double available_balance,
                                          double margin_used,
                                          double daily_pnl) {
    uint32_t account_idx = getAccountIndex(account_hash);
    auto& account = accounts_[account_idx];
    
    account.total_exposure.store(total_exposure, std::memory_order_release);
    account.available_balance.store(available_balance, std::memory_order_release);
    account.margin_used.store(margin_used, std::memory_order_release);
    account.daily_pnl.store(daily_pnl, std::memory_order_release);
    
    // Update timestamp
    auto now = std::chrono::high_resolution_clock::now();
    auto ns = std::chrono::duration_cast<std::chrono::nanoseconds>(
        now.time_since_epoch()).count();
    account.last_update_ns.store(ns, std::memory_order_release);
}

void LockFreeRiskChecker::updatePosition(uint32_t account_hash,
                                       uint32_t symbol_hash,
                                       double quantity,
                                       double notional,
                                       double entry_price) {
    uint32_t account_idx = getAccountIndex(account_hash);
    auto& account = accounts_[account_idx];
    uint32_t pos_slot = getPositionSlot(symbol_hash, account_idx);
    
    auto& position = account.positions[pos_slot];
    
    // Update version first to signal update in progress
    uint64_t old_version = position.version.load(std::memory_order_acquire);
    position.version.store(old_version + 1, std::memory_order_release);
    
    // Update position data
    position.quantity.store(quantity, std::memory_order_release);
    position.notional.store(notional, std::memory_order_release);
    position.entry_price.store(entry_price, std::memory_order_release);
    
    // Store symbol hash if new position
    account.symbol_hashes[pos_slot].store(symbol_hash, std::memory_order_release);
    
    // Finalize by updating version again
    position.version.store(old_version + 2, std::memory_order_release);
}

void LockFreeRiskChecker::enableAccount(uint32_t account_hash) {
    uint32_t account_idx = getAccountIndex(account_hash);
    accounts_[account_idx].trading_enabled.store(true, std::memory_order_release);
}

void LockFreeRiskChecker::disableAccount(uint32_t account_hash) {
    uint32_t account_idx = getAccountIndex(account_hash);
    accounts_[account_idx].trading_enabled.store(false, std::memory_order_release);
}

bool LockFreeRiskChecker::isAccountEnabled(uint32_t account_hash) const {
    uint32_t account_idx = getAccountIndex(account_hash);
    return accounts_[account_idx].trading_enabled.load(std::memory_order_acquire);
}

void LockFreeRiskChecker::resetStats() {
    stats_.total_checks.store(0, std::memory_order_relaxed);
    stats_.passed_checks.store(0, std::memory_order_relaxed);
    stats_.failed_checks.store(0, std::memory_order_relaxed);
    stats_.total_latency_ns.store(0, std::memory_order_relaxed);
    stats_.min_latency_ns.store(UINT64_MAX, std::memory_order_relaxed);
    stats_.max_latency_ns.store(0, std::memory_order_relaxed);
}

// BatchRiskChecker implementation

BatchRiskChecker::BatchRiskChecker(LockFreeRiskChecker* checker) 
    : checker_(checker) {}

BatchRiskChecker::BatchResult BatchRiskChecker::checkBatch(
    const std::vector<Order>& orders,
    const std::vector<uint32_t>& account_hashes) {
    
    BatchResult result;
    result.count = std::min(orders.size(), MAX_BATCH_SIZE);
    
    auto start = std::chrono::high_resolution_clock::now();
    
    // Process orders in batch
    for (size_t i = 0; i < result.count; ++i) {
        const auto& order = orders[i];
        uint32_t account_hash = account_hashes[i];
        
        // Hash the symbol
        uint32_t symbol_hash = std::hash<std::string>{}(order.symbol);
        
        result.results[i] = checker_->checkOrderFast(
            account_hash,
            symbol_hash,
            order.quantity,
            order.price,
            order.side,
            1  // Default leverage
        );
    }
    
    auto end = std::chrono::high_resolution_clock::now();
    result.total_latency = std::chrono::duration_cast<std::chrono::nanoseconds>(end - start);
    
    return result;
}

} // namespace risk
} // namespace oms