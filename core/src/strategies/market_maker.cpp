#include "strategies/market_maker.h"
#include <cmath>
#include <algorithm>
#include <cstring>

namespace oms {
namespace strategies {

MarketMakerEngine::MarketMakerEngine(const MarketMakerConfig& config)
    : config_(config), quote_buffer_(QUOTE_BUFFER_SIZE) {
    // Initialize price history
    std::fill(price_history_.begin(), price_history_.end(), 0.0);
}

MarketMakerEngine::~MarketMakerEngine() {
    stop();
}

void MarketMakerEngine::updateMarketData(const char* symbol, double bid_price, double bid_size,
                                        double ask_price, double ask_size, double last_price) {
    MarketSnapshot new_state;
    new_state.bid_price = bid_price;
    new_state.ask_price = ask_price;
    new_state.mid_price = (bid_price + ask_price) / 2.0;
    new_state.last_price = last_price;
    new_state.bid_size = bid_size;
    new_state.ask_size = ask_size;
    new_state.timestamp_ns = getCurrentTimeNanos();
    
    // Update price history for volatility calculation
    size_t idx = price_index_.fetch_add(1) % PRICE_HISTORY_SIZE;
    price_history_[idx] = new_state.mid_price;
    
    // Calculate volatility
    new_state.volatility = calculateVolatility();
    
    // Update market state
    market_state_ = new_state;
    market_version_.fetch_add(1);
    market_updates_.fetch_add(1);
}

void MarketMakerEngine::updatePosition(const char* symbol, double position, double avg_price) {
    InventorySnapshot new_state;
    new_state.position = position;
    new_state.avg_price = avg_price;
    new_state.position_value = position * avg_price;
    new_state.timestamp_ns = getCurrentTimeNanos();
    
    // Calculate unrealized PnL
    MarketSnapshot market = market_state_;
    if (market.mid_price > 0) {
        new_state.unrealized_pnl = position * (market.mid_price - avg_price);
    }
    
    inventory_state_ = new_state;
    inventory_version_.fetch_add(1);
}

void MarketMakerEngine::generateQuotes() {
    if (!running_.load()) return;
    
    MarketSnapshot market = market_state_;
    InventorySnapshot inventory = inventory_state_;
    
    // Skip if no market data
    if (market.mid_price <= 0 || market.bid_price <= 0 || market.ask_price <= 0) {
        return;
    }
    
    // Calculate dynamic spread
    double spread = calculateSpread(market, inventory);
    
    // Clear existing quotes
    active_quotes_.store(0);
    
    // Generate quotes for each level
    for (int level = 0; level < config_.quote_levels; ++level) {
        // Bid quote
        generateQuoteLevel("BTCUSDT", Side::BUY, market.mid_price, spread, level);
        
        // Ask quote  
        generateQuoteLevel("BTCUSDT", Side::SELL, market.mid_price, spread, level);
    }
    
    // Push all quotes to ring buffer
    size_t quote_count = active_quotes_.load();
    for (size_t i = 0; i < quote_count; ++i) {
        quote_buffer_.push(current_quotes_[i]);
        quotes_generated_.fetch_add(1);
    }
}

bool MarketMakerEngine::getNextQuote(MMQuote& quote) {
    return quote_buffer_.pop(quote);
}

void MarketMakerEngine::start() {
    running_.store(true);
}

void MarketMakerEngine::stop() {
    running_.store(false);
}

double MarketMakerEngine::calculateSpread(const MarketSnapshot& market, const InventorySnapshot& inventory) {
    double base_spread = config_.base_spread_bps / 10000.0;
    
    // Volatility adjustment
    double vol_factor = 1.0 + (market.volatility * config_.volatility_factor);
    
    // Inventory skew
    double inventory_ratio = inventory.position / config_.max_inventory;
    double skew_factor = getInventorySkew(inventory.position);
    
    // Final spread
    double spread = base_spread * vol_factor * skew_factor;
    
    // Apply limits
    double min_spread = config_.min_spread_bps / 10000.0;
    double max_spread = config_.max_spread_bps / 10000.0;
    
    return std::max(min_spread, std::min(max_spread, spread));
}

double MarketMakerEngine::calculateVolatility() {
    double sum = 0.0;
    double sum_sq = 0.0;
    int count = 0;
    
    // Calculate returns
    std::vector<double> returns;
    for (size_t i = 1; i < PRICE_HISTORY_SIZE; ++i) {
        if (price_history_[i] > 0 && price_history_[i-1] > 0) {
            double ret = std::log(price_history_[i] / price_history_[i-1]);
            returns.push_back(ret);
            sum += ret;
            count++;
        }
    }
    
    if (count < 2) return 0.0;
    
    // Calculate standard deviation
    double mean = sum / count;
    for (double ret : returns) {
        sum_sq += std::pow(ret - mean, 2);
    }
    
    return std::sqrt(sum_sq / (count - 1));
}

double MarketMakerEngine::getInventorySkew(double position) {
    double inventory_ratio = position / config_.max_inventory;
    
    // Exponential skew based on inventory
    // When long: widen ask spread, tighten bid spread
    // When short: widen bid spread, tighten ask spread
    return 1.0 + config_.inventory_skew * std::abs(inventory_ratio);
}

void MarketMakerEngine::generateQuoteLevel(const char* symbol, Side side, double mid_price, 
                                          double spread, int level) {
    size_t idx = active_quotes_.fetch_add(1);
    if (idx >= MAX_QUOTES) {
        active_quotes_.fetch_sub(1);
        return;
    }
    
    MMQuote& quote = current_quotes_[idx];
    strncpy(quote.symbol, symbol, sizeof(quote.symbol) - 1);
    strncpy(quote.exchange, "binance", sizeof(quote.exchange) - 1);
    quote.side = side;
    quote.level = level;
    quote.quantity = config_.quote_size;
    quote.timestamp_ns = getCurrentTimeNanos();
    
    // Calculate price based on side and level
    double level_spread = spread * (1.0 + level * config_.level_spacing_bps / 10000.0);
    
    if (side == Side::BUY) {
        quote.price = mid_price * (1.0 - level_spread);
    } else {
        quote.price = mid_price * (1.0 + level_spread);
    }
    
    // Apply inventory skew
    InventorySnapshot inventory = inventory_state_;
    double inventory_ratio = inventory.position / config_.max_inventory;
    
    if (inventory_ratio > 0) {
        // Long position: make asks more aggressive, bids less aggressive
        if (side == Side::SELL) {
            quote.price *= (1.0 - std::abs(inventory_ratio) * config_.inventory_skew * 0.5);
        } else {
            quote.price *= (1.0 + std::abs(inventory_ratio) * config_.inventory_skew * 0.5);
        }
    } else if (inventory_ratio < 0) {
        // Short position: make bids more aggressive, asks less aggressive
        if (side == Side::BUY) {
            quote.price *= (1.0 + std::abs(inventory_ratio) * config_.inventory_skew * 0.5);
        } else {
            quote.price *= (1.0 - std::abs(inventory_ratio) * config_.inventory_skew * 0.5);
        }
    }
}

uint64_t MarketMakerEngine::getCurrentTimeNanos() const {
    return std::chrono::duration_cast<std::chrono::nanoseconds>(
        std::chrono::high_resolution_clock::now().time_since_epoch()
    ).count();
}

// SpreadCalculator implementation
SpreadCalculator::SpreadCalculator(const MarketMakerConfig& config) : config_(config) {}

double SpreadCalculator::calculate(double volatility, double inventory_ratio, double book_depth) {
    double base_spread = config_.base_spread_bps / 10000.0;
    
    // Apply adjustments
    double vol_adj = volatilityAdjustment(volatility);
    double inv_adj = inventoryAdjustment(inventory_ratio);
    double depth_adj = depthAdjustment(book_depth);
    
    // Combined spread
    double spread = base_spread * vol_adj * inv_adj * depth_adj;
    
    // Apply limits
    double min_spread = config_.min_spread_bps / 10000.0;
    double max_spread = config_.max_spread_bps / 10000.0;
    
    return std::max(min_spread, std::min(max_spread, spread));
}

void SpreadCalculator::getBidAskSpreads(double base_spread, double inventory_ratio,
                                       double& bid_spread, double& ask_spread) {
    // Base spreads
    bid_spread = base_spread;
    ask_spread = base_spread;
    
    // Skew based on inventory
    double skew_factor = config_.inventory_skew;
    
    if (inventory_ratio > 0) {
        // Long position: tighten ask, widen bid
        ask_spread *= (1.0 - skew_factor * std::abs(inventory_ratio));
        bid_spread *= (1.0 + skew_factor * std::abs(inventory_ratio));
    } else if (inventory_ratio < 0) {
        // Short position: tighten bid, widen ask
        bid_spread *= (1.0 - skew_factor * std::abs(inventory_ratio));
        ask_spread *= (1.0 + skew_factor * std::abs(inventory_ratio));
    }
}

double SpreadCalculator::volatilityAdjustment(double volatility) {
    // Higher volatility = wider spread
    return 1.0 + volatility * config_.volatility_factor;
}

double SpreadCalculator::inventoryAdjustment(double inventory_ratio) {
    // Higher inventory = wider spread (more risk)
    return 1.0 + std::pow(std::abs(inventory_ratio), 2) * 0.5;
}

double SpreadCalculator::depthAdjustment(double book_depth) {
    // Thinner book = wider spread
    if (book_depth < 10.0) {
        return 1.2;
    } else if (book_depth < 50.0) {
        return 1.1;
    }
    return 1.0;
}

// RiskChecker implementation
RiskChecker::RiskChecker(const MarketMakerConfig& config) : config_(config) {}

bool RiskChecker::checkQuote(const MMQuote& quote, const InventorySnapshot& inventory) {
    // Check position limits
    double new_position = inventory.position;
    if (quote.side == Side::BUY) {
        new_position += quote.quantity;
    } else {
        new_position -= quote.quantity;
    }
    
    if (std::abs(new_position) > config_.max_inventory) {
        return false;
    }
    
    // Check position value
    double position_value = std::abs(new_position * quote.price);
    if (position_value > config_.max_position_value) {
        return false;
    }
    
    // Check stop loss
    double pnl_percent = inventory.unrealized_pnl / (inventory.position_value + 1e-10);
    if (pnl_percent < -config_.stop_loss_percent) {
        return false;
    }
    
    return true;
}

bool RiskChecker::shouldStop(const InventorySnapshot& inventory, double daily_pnl) {
    // Check daily loss limit
    if (daily_pnl < -config_.max_daily_loss) {
        return true;
    }
    
    // Check position stop loss
    double pnl_percent = inventory.unrealized_pnl / (inventory.position_value + 1e-10);
    if (pnl_percent < -config_.stop_loss_percent) {
        return true;
    }
    
    // Check consecutive losses
    if (consecutive_losses_.load() > 10) {
        return true;
    }
    
    return false;
}

void RiskChecker::updatePnL(double pnl) {
    double current_loss = daily_loss_.load();
    daily_loss_.store(current_loss + pnl);
    
    if (pnl < 0) {
        consecutive_losses_.fetch_add(1);
    } else {
        consecutive_losses_.store(0);
    }
}

} // namespace strategies
} // namespace oms