#include "risk/risk_engine.h"
#include <algorithm>
#include <numeric>

namespace oms {
namespace risk {

RiskEngine::RiskEngine(const RiskConfig& config)
    : config_(config)
    , running_(false)
    , total_checks_(0)
    , total_latency_ns_(0) {
    // Initialize position map
    for (auto& pos : positions_) {
        pos.quantity.store(0.0);
        pos.value.store(0.0);
        pos.avg_price.store(0.0);
    }
    
    // Initialize daily PnL
    daily_pnl_ = 0.0;
    
    // Initialize open orders counter
    open_orders_ = 0;
}

void RiskEngine::start() {
    running_ = true;
    log("Risk Engine started");
}

void RiskEngine::stop() {
    running_ = false;
    log("Risk Engine stopped");
}

bool RiskEngine::checkOrder(const Order& order) {
    auto start = std::chrono::steady_clock::now();
    
    if (!running_) {
        return false;
    }
    
    // Multi-level risk checks
    bool passed = true;
    
    // 1. Check order value limit
    double order_value = order.price * order.quantity;
    if (order_value > config_.max_order_value) {
        log("Order value exceeds limit: " + std::to_string(order_value));
        passed = false;
    }
    
    // 2. Check position limit
    if (passed) {
        auto& pos = positions_[hashSymbol(order.symbol) % MAX_SYMBOLS];
        double new_position_value = pos.value + (order.side == Side::BUY ? order_value : -order_value);
        
        if (std::abs(new_position_value) > config_.max_position_value) {
            log("Position value would exceed limit: " + std::to_string(new_position_value));
            passed = false;
        }
    }
    
    // 3. Check daily loss limit
    if (passed) {
        if (daily_pnl_.load() < -config_.daily_loss_limit) {
            log("Daily loss limit exceeded: " + std::to_string(daily_pnl_.load()));
            passed = false;
        }
    }
    
    // 4. Check open orders limit
    if (passed) {
        if (open_orders_.load() >= config_.max_open_orders) {
            log("Open orders limit exceeded: " + std::to_string(open_orders_.load()));
            passed = false;
        }
    }
    
    // Update metrics
    auto end = std::chrono::steady_clock::now();
    auto latency = std::chrono::duration_cast<std::chrono::nanoseconds>(end - start).count();
    
    total_checks_++;
    total_latency_ns_ += latency;
    
    return passed;
}

void RiskEngine::updatePosition(const std::string& symbol, double quantity, double price) {
    auto& pos = positions_[hashSymbol(symbol) % MAX_SYMBOLS];
    
    // Update position
    double old_quantity = pos.quantity.load();
    pos.quantity.store(old_quantity + quantity);
    pos.value.store((old_quantity + quantity) * price);
    
    // Calculate realized PnL if reducing position
    if (old_quantity * quantity < 0) {
        double realized_pnl = calculateRealizedPnL(old_quantity, pos.avg_price.load(), quantity, price);
        double current_pnl = daily_pnl_.load();
        daily_pnl_.store(current_pnl + realized_pnl);
    }
    
    // Update average price
    double new_quantity = old_quantity + quantity;
    if (new_quantity != 0) {
        if (old_quantity * new_quantity > 0) {
            // Adding to position
            double old_avg = pos.avg_price.load();
            pos.avg_price.store((old_quantity * old_avg + quantity * price) / new_quantity);
        } else {
            // New position
            pos.avg_price.store(price);
        }
    }
}

void RiskEngine::updateOrderCount(int delta) {
    open_orders_ += delta;
}

double RiskEngine::getTotalExposure() const {
    double total = 0.0;
    for (const auto& pos : positions_) {
        total += std::abs(pos.value.load());
    }
    return total;
}

void RiskEngine::resetDailyPnL() {
    daily_pnl_ = 0.0;
    log("Daily PnL reset");
}

size_t RiskEngine::getTotalChecks() const {
    return total_checks_.load();
}

double RiskEngine::getAverageLatencyUs() const {
    size_t checks = total_checks_.load();
    if (checks == 0) return 0.0;
    
    return static_cast<double>(total_latency_ns_.load()) / checks / 1000.0;
}

void RiskEngine::log(const std::string& message) {
    // In production, this would write to a proper logging system
    // For now, we'll just store in a buffer
    static std::atomic<size_t> log_index{0};
    static std::array<std::string, 1000> log_buffer;
    
    size_t idx = log_index.fetch_add(1) % log_buffer.size();
    log_buffer[idx] = "[RiskEngine] " + message;
}

size_t RiskEngine::hashSymbol(const std::string& symbol) {
    // Simple hash function for symbol
    size_t hash = 0;
    for (char c : symbol) {
        hash = hash * 31 + c;
    }
    return hash;
}

double RiskEngine::calculateRealizedPnL(double old_quantity, double old_price, 
                                      double new_quantity, double new_price) {
    // Calculate PnL for the closed portion
    double closed_quantity = std::min(std::abs(old_quantity), std::abs(new_quantity));
    
    if (old_quantity > 0) {
        // Was long, now selling
        return closed_quantity * (new_price - old_price);
    } else {
        // Was short, now buying
        return closed_quantity * (old_price - new_price);
    }
}

} // namespace risk
} // namespace oms