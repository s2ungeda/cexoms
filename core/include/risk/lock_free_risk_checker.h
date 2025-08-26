#ifndef LOCK_FREE_RISK_CHECKER_H
#define LOCK_FREE_RISK_CHECKER_H

#include <atomic>
#include <array>
#include <memory>
#include <chrono>
#include <vector>
#include "types.h"

namespace oms {
namespace risk {

// Lock-free position data
struct LockFreePosition {
    std::atomic<double> quantity{0.0};
    std::atomic<double> notional{0.0};
    std::atomic<double> entry_price{0.0};
    std::atomic<uint64_t> version{0};  // For ABA problem prevention
};

// Lock-free account data
struct LockFreeAccountData {
    std::atomic<double> total_exposure{0.0};
    std::atomic<double> available_balance{0.0};
    std::atomic<double> margin_used{0.0};
    std::atomic<double> daily_pnl{0.0};
    std::atomic<int> open_orders{0};
    std::atomic<bool> trading_enabled{true};
    std::atomic<uint64_t> last_update_ns{0};
    
    // Position slots (fixed size array for lock-free access)
    static constexpr size_t MAX_POSITIONS_PER_ACCOUNT = 100;
    std::array<LockFreePosition, MAX_POSITIONS_PER_ACCOUNT> positions;
    std::array<std::atomic<uint32_t>, MAX_POSITIONS_PER_ACCOUNT> symbol_hashes{};
};

// Ultra-fast risk checker using lock-free data structures
class LockFreeRiskChecker {
public:
    static constexpr size_t MAX_ACCOUNTS = 256;
    static constexpr size_t CACHE_LINE_SIZE = 64;
    
    // Result structure aligned to cache line
    struct alignas(CACHE_LINE_SIZE) RiskCheckResult {
        bool passed;
        double margin_required;
        double available_margin;
        std::chrono::nanoseconds latency;
        char reason[48];  // Fixed size for performance
    };
    
    LockFreeRiskChecker();
    ~LockFreeRiskChecker() = default;
    
    // Ultra-fast risk check (target < 10 microseconds)
    RiskCheckResult checkOrderFast(uint32_t account_hash, 
                                  uint32_t symbol_hash,
                                  double quantity, 
                                  double price,
                                  Side side,
                                  int leverage = 1);
    
    // Update account data (lock-free)
    void updateAccountData(uint32_t account_hash,
                          double total_exposure,
                          double available_balance,
                          double margin_used,
                          double daily_pnl);
    
    // Update position (lock-free with version control)
    void updatePosition(uint32_t account_hash,
                       uint32_t symbol_hash,
                       double quantity,
                       double notional,
                       double entry_price);
    
    // Control
    void enableAccount(uint32_t account_hash);
    void disableAccount(uint32_t account_hash);
    bool isAccountEnabled(uint32_t account_hash) const;
    
    // Statistics
    struct Stats {
        std::atomic<uint64_t> total_checks{0};
        std::atomic<uint64_t> passed_checks{0};
        std::atomic<uint64_t> failed_checks{0};
        std::atomic<uint64_t> total_latency_ns{0};
        std::atomic<uint64_t> min_latency_ns{UINT64_MAX};
        std::atomic<uint64_t> max_latency_ns{0};
    };
    
    const Stats& getStats() const { return stats_; }
    void resetStats();
    
    // Configuration
    void setMaxPositionValue(double value) { max_position_value_.store(value); }
    void setMaxAccountExposure(double value) { max_account_exposure_.store(value); }
    void setMaxDailyLoss(double value) { max_daily_loss_.store(value); }
    
private:
    // Account data array (aligned to avoid false sharing)
    alignas(CACHE_LINE_SIZE) std::array<LockFreeAccountData, MAX_ACCOUNTS> accounts_;
    
    // Risk limits (atomic for dynamic updates)
    std::atomic<double> max_position_value_{1000000.0};
    std::atomic<double> max_account_exposure_{5000000.0};
    std::atomic<double> max_daily_loss_{50000.0};
    std::atomic<double> min_margin_ratio_{0.05};  // 5% minimum margin
    
    // Statistics
    mutable Stats stats_;
    
    // Helper functions
    inline uint32_t getAccountIndex(uint32_t hash) const {
        return hash % MAX_ACCOUNTS;
    }
    
    inline uint32_t getPositionSlot(uint32_t symbol_hash, uint32_t account_idx) const {
        return symbol_hash % LockFreeAccountData::MAX_POSITIONS_PER_ACCOUNT;
    }
    
    inline double calculateMarginRequired(double notional, int leverage) const {
        return notional / leverage;
    }
    
    inline void updateLatencyStats(uint64_t latency_ns) {
        stats_.total_latency_ns.fetch_add(latency_ns, std::memory_order_relaxed);
        
        // Update min latency
        uint64_t current_min = stats_.min_latency_ns.load(std::memory_order_relaxed);
        while (latency_ns < current_min && 
               !stats_.min_latency_ns.compare_exchange_weak(current_min, latency_ns,
                                                           std::memory_order_relaxed));
        
        // Update max latency
        uint64_t current_max = stats_.max_latency_ns.load(std::memory_order_relaxed);
        while (latency_ns > current_max && 
               !stats_.max_latency_ns.compare_exchange_weak(current_max, latency_ns,
                                                           std::memory_order_relaxed));
    }
};

// Optimized batch risk checker for multiple orders
class BatchRiskChecker {
public:
    static constexpr size_t MAX_BATCH_SIZE = 64;
    
    struct BatchResult {
        std::array<LockFreeRiskChecker::RiskCheckResult, MAX_BATCH_SIZE> results;
        size_t count;
        std::chrono::nanoseconds total_latency;
    };
    
    BatchRiskChecker(LockFreeRiskChecker* checker);
    
    // Check multiple orders in batch (optimized for cache efficiency)
    BatchResult checkBatch(const std::vector<Order>& orders,
                          const std::vector<uint32_t>& account_hashes);
    
private:
    LockFreeRiskChecker* checker_;
};

} // namespace risk
} // namespace oms

#endif // LOCK_FREE_RISK_CHECKER_H