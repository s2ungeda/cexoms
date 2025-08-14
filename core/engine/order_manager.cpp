#include "order_manager.h"
#include <algorithm>
#include <chrono>
#include <sched.h>
#include <pthread.h>

namespace oms {

OrderManager::OrderManager(const Config& config) 
    : config_(config)
    , last_rate_check_(std::chrono::steady_clock::now()) {
    
    // Initialize ring buffers for each exchange type
    for (int i = 0; i <= static_cast<int>(ExchangeType::UPBIT); ++i) {
        auto exchange = static_cast<ExchangeType>(i);
        order_queues_[exchange] = std::make_unique<RingBuffer<Order>>(config.ring_buffer_size);
    }
}

OrderManager::~OrderManager() {
    Stop();
}

bool OrderManager::SubmitOrder(const Order& order) {
    // Rate limiting check
    auto now = std::chrono::steady_clock::now();
    if (now - last_rate_check_ >= std::chrono::seconds(1)) {
        orders_this_second_.store(0);
        last_rate_check_ = now;
    }
    
    if (orders_this_second_.fetch_add(1) >= config_.max_orders_per_second) {
        stats_.orders_rejected.fetch_add(1);
        return false;
    }
    
    // Submit to appropriate exchange queue
    auto& queue = order_queues_[order.exchange];
    if (!queue->push(order)) {
        stats_.orders_rejected.fetch_add(1);
        return false;
    }
    
    return true;
}

bool OrderManager::CancelOrder(OrderId order_id, ExchangeType exchange) {
    Order cancel_order;
    cancel_order.id = order_id;
    cancel_order.exchange = exchange;
    cancel_order.status = OrderStatus::CANCELED;
    
    return SubmitOrder(cancel_order);
}

bool OrderManager::UpdateOrder(const Order& order) {
    std::unique_lock lock(orders_mutex_);
    
    auto it = orders_.find(order.id);
    if (it == orders_.end()) {
        return false;
    }
    
    *it->second = order;
    return true;
}

std::shared_ptr<Order> OrderManager::GetOrder(OrderId order_id) const {
    std::shared_lock lock(orders_mutex_);
    
    auto it = orders_.find(order_id);
    if (it != orders_.end()) {
        return it->second;
    }
    
    return nullptr;
}

std::vector<Order> OrderManager::GetOrdersByExchange(ExchangeType exchange) const {
    std::shared_lock lock(orders_mutex_);
    
    std::vector<Order> result;
    auto it = exchange_orders_.find(exchange);
    if (it != exchange_orders_.end()) {
        result.reserve(it->second.size());
        for (OrderId id : it->second) {
            auto order_it = orders_.find(id);
            if (order_it != orders_.end()) {
                result.push_back(*order_it->second);
            }
        }
    }
    
    return result;
}

void OrderManager::Start() {
    if (running_.exchange(true)) {
        return; // Already running
    }
    
    processing_thread_ = std::thread([this] {
        SetCPUAffinity(config_.cpu_cores);
        ProcessingLoop();
    });
}

void OrderManager::Stop() {
    if (!running_.exchange(false)) {
        return; // Already stopped
    }
    
    if (processing_thread_.joinable()) {
        processing_thread_.join();
    }
}

void OrderManager::ProcessingLoop() {
    Order order;
    
    while (running_.load()) {
        bool processed = false;
        
        // Process orders from all exchange queues
        for (auto& [exchange, queue] : order_queues_) {
            if (queue->pop(order)) {
                auto start = std::chrono::high_resolution_clock::now();
                
                ProcessOrder(order);
                
                auto end = std::chrono::high_resolution_clock::now();
                auto latency_us = std::chrono::duration_cast<std::chrono::microseconds>(
                    end - start).count();
                
                UpdateLatencyStats(latency_us);
                stats_.orders_processed.fetch_add(1);
                processed = true;
            }
        }
        
        // Avoid busy waiting if no orders
        if (!processed) {
            std::this_thread::yield();
        }
    }
}

void OrderManager::ProcessOrder(const Order& order) {
    if (order.status == OrderStatus::CANCELED) {
        ProcessCancellation(order.id, order.exchange);
        return;
    }
    
    // Store order
    auto order_ptr = std::make_shared<Order>(order);
    order_ptr->id = next_order_id_.fetch_add(1);
    order_ptr->created_at = std::chrono::duration_cast<std::chrono::microseconds>(
        std::chrono::system_clock::now().time_since_epoch());
    
    {
        std::unique_lock lock(orders_mutex_);
        orders_[order_ptr->id] = order_ptr;
        exchange_orders_[order.exchange].push_back(order_ptr->id);
    }
    
    // TODO: Send to exchange connector via NATS
}

void OrderManager::ProcessCancellation(OrderId order_id, ExchangeType exchange) {
    std::unique_lock lock(orders_mutex_);
    
    auto it = orders_.find(order_id);
    if (it != orders_.end() && it->second->exchange == exchange) {
        it->second->status = OrderStatus::CANCELED;
        it->second->updated_at = std::chrono::duration_cast<std::chrono::microseconds>(
            std::chrono::system_clock::now().time_since_epoch());
    }
    
    // TODO: Send cancellation to exchange connector via NATS
}

void OrderManager::SetCPUAffinity(const std::vector<int>& cores) {
#ifdef __linux__
    cpu_set_t cpuset;
    CPU_ZERO(&cpuset);
    
    for (int core : cores) {
        CPU_SET(core, &cpuset);
    }
    
    pthread_t thread = pthread_self();
    pthread_setaffinity_np(thread, sizeof(cpu_set_t), &cpuset);
#endif
}

void OrderManager::UpdateLatencyStats(uint64_t latency_us) {
    stats_.total_latency_us.fetch_add(latency_us);
    
    // Update min latency
    uint64_t current_min = stats_.min_latency_us.load();
    while (latency_us < current_min && 
           !stats_.min_latency_us.compare_exchange_weak(current_min, latency_us));
    
    // Update max latency
    uint64_t current_max = stats_.max_latency_us.load();
    while (latency_us > current_max && 
           !stats_.max_latency_us.compare_exchange_weak(current_max, latency_us));
}

// AggregatedOrderBook implementation

void AggregatedOrderBook::UpdateBook(ExchangeType exchange, const Symbol& symbol,
                                   const std::vector<Level>& bids,
                                   const std::vector<Level>& asks) {
    std::unique_lock lock(book_mutex_);
    
    Book& book = books_[symbol][exchange];
    book.bids = bids;
    book.asks = asks;
    book.last_update = std::chrono::duration_cast<std::chrono::microseconds>(
        std::chrono::system_clock::now().time_since_epoch());
}

AggregatedOrderBook::Book AggregatedOrderBook::GetAggregatedBook(const Symbol& symbol) const {
    std::shared_lock lock(book_mutex_);
    
    auto symbol_it = books_.find(symbol);
    if (symbol_it == books_.end()) {
        return Book{};
    }
    
    std::vector<Book> exchange_books;
    for (const auto& [exchange, book] : symbol_it->second) {
        exchange_books.push_back(book);
    }
    
    return MergeBooks(exchange_books);
}

std::pair<AggregatedOrderBook::Level, AggregatedOrderBook::Level> 
AggregatedOrderBook::GetBestBidAsk(const Symbol& symbol) const {
    auto book = GetAggregatedBook(symbol);
    
    Level best_bid{0, 0, ExchangeType::BINANCE_SPOT, 0};
    Level best_ask{std::numeric_limits<Price>::max(), 0, ExchangeType::BINANCE_SPOT, 0};
    
    if (!book.bids.empty()) {
        best_bid = book.bids.front();
    }
    
    if (!book.asks.empty()) {
        best_ask = book.asks.front();
    }
    
    return {best_bid, best_ask};
}

ExchangeType AggregatedOrderBook::GetBestExchange(const Symbol& symbol, Side side,
                                                 Quantity quantity) const {
    auto book = GetAggregatedBook(symbol);
    
    if (side == Side::BUY && !book.asks.empty()) {
        // Find exchange with best ask price and sufficient liquidity
        Quantity cumulative = 0;
        for (const auto& level : book.asks) {
            cumulative += level.quantity;
            if (cumulative >= quantity) {
                return level.exchange;
            }
        }
        return book.asks.front().exchange;
    } else if (side == Side::SELL && !book.bids.empty()) {
        // Find exchange with best bid price and sufficient liquidity
        Quantity cumulative = 0;
        for (const auto& level : book.bids) {
            cumulative += level.quantity;
            if (cumulative >= quantity) {
                return level.exchange;
            }
        }
        return book.bids.front().exchange;
    }
    
    return ExchangeType::BINANCE_SPOT; // Default
}

AggregatedOrderBook::Book AggregatedOrderBook::MergeBooks(const std::vector<Book>& books) const {
    Book merged;
    
    // Merge all bids
    std::vector<Level> all_bids;
    for (const auto& book : books) {
        all_bids.insert(all_bids.end(), book.bids.begin(), book.bids.end());
    }
    
    // Sort bids by price (descending)
    std::sort(all_bids.begin(), all_bids.end(),
              [](const Level& a, const Level& b) { return a.price > b.price; });
    
    // Merge all asks
    std::vector<Level> all_asks;
    for (const auto& book : books) {
        all_asks.insert(all_asks.end(), book.asks.begin(), book.asks.end());
    }
    
    // Sort asks by price (ascending)
    std::sort(all_asks.begin(), all_asks.end(),
              [](const Level& a, const Level& b) { return a.price < b.price; });
    
    merged.bids = std::move(all_bids);
    merged.asks = std::move(all_asks);
    
    // Set last update to most recent
    for (const auto& book : books) {
        if (book.last_update > merged.last_update) {
            merged.last_update = book.last_update;
        }
    }
    
    return merged;
}

} // namespace oms