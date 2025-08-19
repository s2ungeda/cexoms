#ifndef ARBITRAGE_DETECTOR_H
#define ARBITRAGE_DETECTOR_H

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

// Price feed structure for fast access
struct PriceFeed {
    char exchange[16];
    char symbol[16];
    double bid_price;
    double bid_quantity;
    double ask_price;
    double ask_quantity;
    uint64_t timestamp_ns;
    
    PriceFeed() : bid_price(0), bid_quantity(0), ask_price(0), ask_quantity(0), timestamp_ns(0) {
        exchange[0] = '\0';
        symbol[0] = '\0';
    }
};

// Arbitrage opportunity structure
struct ArbitrageOpportunity {
    char id[64];
    char symbol[16];
    char buy_exchange[16];
    char sell_exchange[16];
    double buy_price;
    double sell_price;
    double max_quantity;
    double profit_rate;
    double net_profit;
    uint64_t detected_at_ns;
    uint64_t valid_until_ns;
    
    ArbitrageOpportunity() : buy_price(0), sell_price(0), max_quantity(0), 
                            profit_rate(0), net_profit(0), detected_at_ns(0), valid_until_ns(0) {
        id[0] = '\0';
        symbol[0] = '\0';
        buy_exchange[0] = '\0';
        sell_exchange[0] = '\0';
    }
};

// Configuration for arbitrage detection
struct ArbitrageConfig {
    double min_profit_rate;      // Minimum profit rate (e.g., 0.001 = 0.1%)
    double min_profit_amount;    // Minimum profit in USDT
    double max_position_size;    // Maximum position size
    uint64_t opportunity_ttl_ns; // Opportunity time-to-live in nanoseconds
    
    // Exchange fees (simplified)
    std::unordered_map<std::string, double> taker_fees;
    std::unordered_map<std::string, double> maker_fees;
    
    ArbitrageConfig() : min_profit_rate(0.001), min_profit_amount(10.0),
                       max_position_size(10000.0), opportunity_ttl_ns(500000000) {} // 500ms
};

// High-performance arbitrage detector
class ArbitrageDetector {
public:
    static constexpr size_t MAX_EXCHANGES = 10;
    static constexpr size_t MAX_SYMBOLS = 100;
    static constexpr size_t OPPORTUNITY_BUFFER_SIZE = 1024;
    
    ArbitrageDetector(const ArbitrageConfig& config);
    ~ArbitrageDetector();
    
    // Update price feed (lock-free)
    void updatePriceFeed(const char* exchange, const char* symbol, 
                        double bid_price, double bid_quantity,
                        double ask_price, double ask_quantity);
    
    // Detect opportunities (called from dedicated thread)
    void detectOpportunities();
    
    // Get detected opportunities (lock-free)
    bool getNextOpportunity(ArbitrageOpportunity& opportunity);
    
    // Start/stop detection
    void start();
    void stop();
    
    // Statistics
    uint64_t getDetectedCount() const { return detected_count_.load(); }
    uint64_t getProcessedPrices() const { return processed_prices_.load(); }
    
private:
    // Price storage (optimized for cache locality)
    struct SymbolPrices {
        std::array<PriceFeed, MAX_EXCHANGES> feeds;
        std::atomic<uint8_t> exchange_count{0};
        char symbol[16];
        
        SymbolPrices() { symbol[0] = '\0'; }
    };
    
    // Fast symbol lookup
    std::unordered_map<std::string, size_t> symbol_index_;
    std::array<SymbolPrices, MAX_SYMBOLS> symbol_prices_;
    std::atomic<size_t> symbol_count_{0};
    
    // Configuration
    ArbitrageConfig config_;
    
    // Opportunity buffer (lock-free ring buffer)
    oms::RingBuffer<ArbitrageOpportunity> opportunity_buffer_;
    
    // Statistics
    std::atomic<uint64_t> detected_count_{0};
    std::atomic<uint64_t> processed_prices_{0};
    
    // Control
    std::atomic<bool> running_{false};
    
    // Helper methods
    size_t getOrCreateSymbolIndex(const char* symbol);
    void checkArbitrageOpportunity(const PriceFeed& buy, const PriceFeed& sell, const char* symbol);
    double calculateFee(const std::string& exchange, double price, bool is_taker);
    uint64_t getCurrentTimeNanos() const;
};

} // namespace strategies
} // namespace oms

#endif // ARBITRAGE_DETECTOR_H