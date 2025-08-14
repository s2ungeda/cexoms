#ifndef OMS_ORDER_MANAGER_H
#define OMS_ORDER_MANAGER_H

#include <atomic>
#include <memory>
#include <unordered_map>
#include <shared_mutex>
#include <thread>
#include <vector>

#include "types.h"
#include "ring_buffer.h"

namespace oms {

class OrderManager {
public:
    struct Config {
        size_t ring_buffer_size = 1048576;  // 1MB
        size_t max_orders_per_second = 100000;
        size_t max_active_orders = 1000000;
        std::vector<int> cpu_cores = {2, 3};  // CPU affinity
    };

    explicit OrderManager(const Config& config);
    ~OrderManager();

    // Order operations
    bool SubmitOrder(const Order& order);
    bool CancelOrder(OrderId order_id, ExchangeType exchange);
    bool UpdateOrder(const Order& order);
    
    // Order retrieval
    std::shared_ptr<Order> GetOrder(OrderId order_id) const;
    std::vector<Order> GetOrdersByExchange(ExchangeType exchange) const;
    
    // Processing control
    void Start();
    void Stop();
    bool IsRunning() const { return running_.load(); }
    
    // Statistics
    struct Stats {
        std::atomic<uint64_t> orders_processed{0};
        std::atomic<uint64_t> orders_rejected{0};
        std::atomic<uint64_t> total_latency_us{0};
        std::atomic<uint64_t> min_latency_us{UINT64_MAX};
        std::atomic<uint64_t> max_latency_us{0};
    };
    
    const Stats& GetStats() const { return stats_; }

private:
    // Order processing
    void ProcessingLoop();
    void ProcessOrder(const Order& order);
    void ProcessCancellation(OrderId order_id, ExchangeType exchange);
    
    // CPU affinity
    void SetCPUAffinity(const std::vector<int>& cores);
    
    // Latency tracking
    void UpdateLatencyStats(uint64_t latency_us);
    
private:
    Config config_;
    Stats stats_;
    
    // Lock-free ring buffers for each exchange
    std::unordered_map<ExchangeType, std::unique_ptr<RingBuffer<Order>>> order_queues_;
    
    // Order storage with read-write lock for thread safety
    mutable std::shared_mutex orders_mutex_;
    std::unordered_map<OrderId, std::shared_ptr<Order>> orders_;
    
    // Exchange-specific order indices
    std::unordered_map<ExchangeType, std::vector<OrderId>> exchange_orders_;
    
    // Processing thread
    std::thread processing_thread_;
    std::atomic<bool> running_{false};
    
    // Order ID generation
    std::atomic<OrderId> next_order_id_{1};
    
    // Rate limiting
    std::atomic<uint32_t> orders_this_second_{0};
    std::chrono::steady_clock::time_point last_rate_check_;
};

// Aggregated order book for multi-exchange view
class AggregatedOrderBook {
public:
    struct Level {
        Price price;
        Quantity quantity;
        ExchangeType exchange;
        int num_orders;
    };
    
    struct Book {
        std::vector<Level> bids;
        std::vector<Level> asks;
        Timestamp last_update;
    };
    
    // Update order book for an exchange
    void UpdateBook(ExchangeType exchange, const Symbol& symbol, 
                   const std::vector<Level>& bids, 
                   const std::vector<Level>& asks);
    
    // Get aggregated book across all exchanges
    Book GetAggregatedBook(const Symbol& symbol) const;
    
    // Get best bid/ask across all exchanges
    std::pair<Level, Level> GetBestBidAsk(const Symbol& symbol) const;
    
    // Find best exchange for execution
    ExchangeType GetBestExchange(const Symbol& symbol, Side side, 
                                Quantity quantity) const;

private:
    mutable std::shared_mutex book_mutex_;
    
    // Symbol -> Exchange -> Book
    std::unordered_map<Symbol, 
        std::unordered_map<ExchangeType, Book>> books_;
    
    // Helper to merge order books
    Book MergeBooks(const std::vector<Book>& books) const;
};

} // namespace oms

#endif // OMS_ORDER_MANAGER_H