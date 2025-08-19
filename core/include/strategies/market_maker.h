#ifndef MARKET_MAKER_H
#define MARKET_MAKER_H

#include <atomic>
#include <array>
#include <chrono>
#include <unordered_map>
#include <vector>
#include <string>
#include "types.h"
#include "ring_buffer.h"

namespace oms {
namespace strategies {

// Quote represents a single market maker quote
struct MMQuote {
    char symbol[16];
    char exchange[16];
    Side side;
    double price;
    double quantity;
    int level;
    uint64_t timestamp_ns;
    
    MMQuote() : side(Side::BUY), price(0), quantity(0), level(0), timestamp_ns(0) {
        symbol[0] = '\0';
        exchange[0] = '\0';
    }
};

// Market state for fast calculations
struct MarketSnapshot {
    double bid_price;
    double ask_price;
    double mid_price;
    double last_price;
    double bid_size;
    double ask_size;
    double volatility;
    uint64_t timestamp_ns;
    
    MarketSnapshot() : bid_price(0), ask_price(0), mid_price(0), last_price(0),
                      bid_size(0), ask_size(0), volatility(0), timestamp_ns(0) {}
};

// Inventory state
struct InventorySnapshot {
    double position;
    double avg_price;
    double unrealized_pnl;
    double realized_pnl;
    double position_value;
    uint64_t timestamp_ns;
    
    InventorySnapshot() : position(0), avg_price(0), unrealized_pnl(0),
                         realized_pnl(0), position_value(0), timestamp_ns(0) {}
};

// Configuration for market maker
struct MarketMakerConfig {
    double base_spread_bps;      // Base spread in basis points
    double min_spread_bps;       // Minimum spread
    double max_spread_bps;       // Maximum spread
    double quote_size;           // Size per quote level
    int quote_levels;            // Number of quote levels
    double level_spacing_bps;    // Spacing between levels
    double max_inventory;        // Maximum position
    double inventory_skew;       // Skew factor
    double volatility_factor;    // Volatility adjustment factor
    
    // Risk limits
    double max_position_value;
    double stop_loss_percent;
    double max_daily_loss;
    
    MarketMakerConfig() : base_spread_bps(10), min_spread_bps(5), max_spread_bps(50),
                         quote_size(1.0), quote_levels(3), level_spacing_bps(2),
                         max_inventory(100.0), inventory_skew(0.5), volatility_factor(1.0),
                         max_position_value(100000.0), stop_loss_percent(0.02),
                         max_daily_loss(1000.0) {}
};

// High-performance market maker engine
class MarketMakerEngine {
public:
    static constexpr size_t MAX_QUOTES = 20;
    static constexpr size_t QUOTE_BUFFER_SIZE = 1024;
    static constexpr size_t PRICE_HISTORY_SIZE = 1000;
    
    MarketMakerEngine(const MarketMakerConfig& config);
    ~MarketMakerEngine();
    
    // Market data updates (lock-free)
    void updateMarketData(const char* symbol, double bid_price, double bid_size,
                         double ask_price, double ask_size, double last_price);
    
    // Position updates
    void updatePosition(const char* symbol, double position, double avg_price);
    
    // Quote generation (called from dedicated thread)
    void generateQuotes();
    
    // Get generated quotes (lock-free)
    bool getNextQuote(MMQuote& quote);
    
    // Control
    void start();
    void stop();
    
    // Statistics
    uint64_t getQuotesGenerated() const { return quotes_generated_.load(); }
    uint64_t getMarketUpdates() const { return market_updates_.load(); }
    
private:
    // Configuration
    MarketMakerConfig config_;
    
    // Market state (protected by memory ordering, not atomic)
    MarketSnapshot market_state_;
    InventorySnapshot inventory_state_;
    std::atomic<uint64_t> market_version_{0};
    std::atomic<uint64_t> inventory_version_{0};
    
    // Price history for volatility calculation
    std::array<double, PRICE_HISTORY_SIZE> price_history_;
    std::atomic<size_t> price_index_{0};
    
    // Quote generation
    oms::RingBuffer<MMQuote> quote_buffer_;
    std::array<MMQuote, MAX_QUOTES> current_quotes_;
    std::atomic<size_t> active_quotes_{0};
    
    // Statistics
    std::atomic<uint64_t> quotes_generated_{0};
    std::atomic<uint64_t> market_updates_{0};
    
    // Control
    std::atomic<bool> running_{false};
    
    // Internal methods
    double calculateSpread(const MarketSnapshot& market, const InventorySnapshot& inventory);
    double calculateVolatility();
    double getInventorySkew(double position);
    void generateQuoteLevel(const char* symbol, Side side, double mid_price, 
                           double spread, int level);
    uint64_t getCurrentTimeNanos() const;
};

// Spread calculator optimized for speed
class SpreadCalculator {
public:
    SpreadCalculator(const MarketMakerConfig& config);
    
    // Calculate optimal spread based on conditions
    double calculate(double volatility, double inventory_ratio, double book_depth);
    
    // Get bid/ask specific spreads
    void getBidAskSpreads(double base_spread, double inventory_ratio,
                         double& bid_spread, double& ask_spread);
    
private:
    const MarketMakerConfig& config_;
    
    double volatilityAdjustment(double volatility);
    double inventoryAdjustment(double inventory_ratio);
    double depthAdjustment(double book_depth);
};

// Fast risk checker
class RiskChecker {
public:
    RiskChecker(const MarketMakerConfig& config);
    
    // Check if quote passes risk limits
    bool checkQuote(const MMQuote& quote, const InventorySnapshot& inventory);
    
    // Check if should stop trading
    bool shouldStop(const InventorySnapshot& inventory, double daily_pnl);
    
    // Update PnL tracking
    void updatePnL(double pnl);
    
private:
    const MarketMakerConfig& config_;
    std::atomic<double> daily_loss_{0.0};
    std::atomic<int> consecutive_losses_{0};
};

} // namespace strategies
} // namespace oms

#endif // MARKET_MAKER_H