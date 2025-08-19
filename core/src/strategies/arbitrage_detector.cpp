#include "strategies/arbitrage_detector.h"
#include <cstring>
#include <algorithm>
#include <cstdio>

namespace oms {
namespace strategies {

ArbitrageDetector::ArbitrageDetector(const ArbitrageConfig& config)
    : config_(config), opportunity_buffer_(OPPORTUNITY_BUFFER_SIZE) {
}

ArbitrageDetector::~ArbitrageDetector() {
    stop();
}

void ArbitrageDetector::updatePriceFeed(const char* exchange, const char* symbol,
                                       double bid_price, double bid_quantity,
                                       double ask_price, double ask_quantity) {
    // Get symbol index
    size_t sym_idx = getOrCreateSymbolIndex(symbol);
    if (sym_idx >= MAX_SYMBOLS) {
        return; // Symbol limit reached
    }
    
    // Find exchange slot in symbol prices
    SymbolPrices& sym_prices = symbol_prices_[sym_idx];
    
    // Linear search for exchange (small array, cache-friendly)
    int exchange_idx = -1;
    uint8_t count = sym_prices.exchange_count.load(std::memory_order_acquire);
    
    for (uint8_t i = 0; i < count; ++i) {
        if (std::strcmp(sym_prices.feeds[i].exchange, exchange) == 0) {
            exchange_idx = i;
            break;
        }
    }
    
    // Add new exchange if not found
    if (exchange_idx == -1) {
        if (count < MAX_EXCHANGES) {
            exchange_idx = count;
            sym_prices.exchange_count.fetch_add(1, std::memory_order_release);
        } else {
            return; // Exchange limit reached for this symbol
        }
    }
    
    // Update price feed (atomic-like update)
    PriceFeed& feed = sym_prices.feeds[exchange_idx];
    std::strncpy(feed.exchange, exchange, sizeof(feed.exchange) - 1);
    std::strncpy(feed.symbol, symbol, sizeof(feed.symbol) - 1);
    feed.bid_price = bid_price;
    feed.bid_quantity = bid_quantity;
    feed.ask_price = ask_price;
    feed.ask_quantity = ask_quantity;
    feed.timestamp_ns = getCurrentTimeNanos();
    
    processed_prices_.fetch_add(1, std::memory_order_relaxed);
}

void ArbitrageDetector::detectOpportunities() {
    if (!running_.load(std::memory_order_acquire)) {
        return;
    }
    
    uint64_t current_time = getCurrentTimeNanos();
    size_t sym_count = symbol_count_.load(std::memory_order_acquire);
    
    // Check each symbol
    for (size_t sym_idx = 0; sym_idx < sym_count; ++sym_idx) {
        SymbolPrices& sym_prices = symbol_prices_[sym_idx];
        uint8_t exchange_count = sym_prices.exchange_count.load(std::memory_order_acquire);
        
        if (exchange_count < 2) {
            continue; // Need at least 2 exchanges
        }
        
        // Check all exchange pairs
        for (uint8_t i = 0; i < exchange_count; ++i) {
            const PriceFeed& feed_i = sym_prices.feeds[i];
            
            // Skip stale prices (older than 1 second)
            if (current_time - feed_i.timestamp_ns > 1000000000) {
                continue;
            }
            
            for (uint8_t j = i + 1; j < exchange_count; ++j) {
                const PriceFeed& feed_j = sym_prices.feeds[j];
                
                // Skip stale prices
                if (current_time - feed_j.timestamp_ns > 1000000000) {
                    continue;
                }
                
                // Check both directions
                checkArbitrageOpportunity(feed_i, feed_j, sym_prices.symbol);
                checkArbitrageOpportunity(feed_j, feed_i, sym_prices.symbol);
            }
        }
    }
}

void ArbitrageDetector::checkArbitrageOpportunity(const PriceFeed& buy, const PriceFeed& sell, 
                                                  const char* symbol) {
    // Calculate gross profit
    double price_diff = sell.bid_price - buy.ask_price;
    if (price_diff <= 0) {
        return; // No profit
    }
    
    // Calculate profit rate
    double profit_rate = price_diff / buy.ask_price;
    if (profit_rate < config_.min_profit_rate) {
        return; // Below minimum profit rate
    }
    
    // Calculate fees
    double buy_fee = calculateFee(buy.exchange, buy.ask_price, true);
    double sell_fee = calculateFee(sell.exchange, sell.bid_price, true);
    double total_fee_rate = (buy_fee + sell_fee) / buy.ask_price;
    
    // Calculate net profit rate
    double net_profit_rate = profit_rate - total_fee_rate;
    if (net_profit_rate < config_.min_profit_rate) {
        return; // Not profitable after fees
    }
    
    // Calculate maximum quantity
    double max_quantity = std::min(buy.ask_quantity, sell.bid_quantity);
    double max_value = max_quantity * buy.ask_price;
    
    // Apply position size limit
    if (max_value > config_.max_position_size) {
        max_quantity = config_.max_position_size / buy.ask_price;
        max_value = config_.max_position_size;
    }
    
    // Calculate net profit amount
    double net_profit = max_quantity * price_diff - max_quantity * (buy_fee + sell_fee);
    
    if (net_profit < config_.min_profit_amount) {
        return; // Below minimum profit amount
    }
    
    // Create opportunity
    ArbitrageOpportunity opportunity;
    
    // Generate ID
    uint64_t timestamp = getCurrentTimeNanos();
    std::snprintf(opportunity.id, sizeof(opportunity.id), "%s_%s_%s_%llu", 
                 symbol, buy.exchange, sell.exchange, timestamp);
    
    // Fill opportunity details
    std::strncpy(opportunity.symbol, symbol, sizeof(opportunity.symbol) - 1);
    std::strncpy(opportunity.buy_exchange, buy.exchange, sizeof(opportunity.buy_exchange) - 1);
    std::strncpy(opportunity.sell_exchange, sell.exchange, sizeof(opportunity.sell_exchange) - 1);
    opportunity.buy_price = buy.ask_price;
    opportunity.sell_price = sell.bid_price;
    opportunity.max_quantity = max_quantity;
    opportunity.profit_rate = net_profit_rate;
    opportunity.net_profit = net_profit;
    opportunity.detected_at_ns = timestamp;
    opportunity.valid_until_ns = timestamp + config_.opportunity_ttl_ns;
    
    // Add to buffer (lock-free)
    if (opportunity_buffer_.push(opportunity)) {
        detected_count_.fetch_add(1, std::memory_order_relaxed);
    }
}

bool ArbitrageDetector::getNextOpportunity(ArbitrageOpportunity& opportunity) {
    return opportunity_buffer_.pop(opportunity);
}

void ArbitrageDetector::start() {
    running_.store(true, std::memory_order_release);
}

void ArbitrageDetector::stop() {
    running_.store(false, std::memory_order_release);
}

size_t ArbitrageDetector::getOrCreateSymbolIndex(const char* symbol) {
    std::string sym_str(symbol);
    
    // Check if symbol exists
    auto it = symbol_index_.find(sym_str);
    if (it != symbol_index_.end()) {
        return it->second;
    }
    
    // Create new symbol entry
    size_t new_idx = symbol_count_.fetch_add(1, std::memory_order_acq_rel);
    if (new_idx >= MAX_SYMBOLS) {
        symbol_count_.fetch_sub(1, std::memory_order_acq_rel);
        return MAX_SYMBOLS; // Limit reached
    }
    
    // Initialize symbol entry
    SymbolPrices& sym_prices = symbol_prices_[new_idx];
    std::strncpy(sym_prices.symbol, symbol, sizeof(sym_prices.symbol) - 1);
    
    // Add to index
    symbol_index_[sym_str] = new_idx;
    
    return new_idx;
}

double ArbitrageDetector::calculateFee(const std::string& exchange, double price, bool is_taker) {
    if (is_taker) {
        auto it = config_.taker_fees.find(exchange);
        if (it != config_.taker_fees.end()) {
            return price * it->second;
        }
    } else {
        auto it = config_.maker_fees.find(exchange);
        if (it != config_.maker_fees.end()) {
            return price * it->second;
        }
    }
    
    // Default fee
    return price * 0.001; // 0.1%
}

uint64_t ArbitrageDetector::getCurrentTimeNanos() const {
    return std::chrono::duration_cast<std::chrono::nanoseconds>(
        std::chrono::steady_clock::now().time_since_epoch()
    ).count();
}

} // namespace strategies
} // namespace oms