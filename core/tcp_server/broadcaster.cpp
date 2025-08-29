#include "broadcaster.h"
#include "tcp_server.h"
#include <iostream>
#include <algorithm>

namespace oms {
namespace tcp {

Broadcaster::Broadcaster(TcpServer* server)
    : server_(server) {
    message_queue_ = std::make_unique<MessageQueue>();
    batch_buffer_.reserve(BATCH_SIZE);
}

Broadcaster::~Broadcaster() {
    Stop();
}

void Broadcaster::Start() {
    if (running_.exchange(true)) {
        return; // 이미 실행 중
    }
    
    worker_thread_ = std::thread(&Broadcaster::WorkerThread, this);
}

void Broadcaster::Stop() {
    if (!running_.exchange(false)) {
        return; // 이미 정지됨
    }
    
    if (worker_thread_.joinable()) {
        worker_thread_.join();
    }
}

void Broadcaster::BroadcastMarketData(const MarketDataUpdate& data) {
    BroadcastMessage msg;
    msg.type = BroadcastMessage::MARKET_DATA;
    msg.symbol = data.symbol();
    msg.exchange = data.exchange();
    
    // Protocol Buffers 메시지로 래핑
    Message wrapper;
    wrapper.set_type(MessageType::MARKET_DATA_UPDATE);
    wrapper.set_allocated_market_data(new MarketDataUpdate(data));
    msg.message = std::move(wrapper);
    
    // Lock-free 큐에 추가
    if (!message_queue_->push(msg)) {
        dropped_messages_++;
        std::cerr << "Broadcast queue full, dropping market data" << std::endl;
    } else {
        queued_messages_++;
    }
}

void Broadcaster::BroadcastOrderBook(const OrderBookUpdate& data) {
    BroadcastMessage msg;
    msg.type = BroadcastMessage::ORDERBOOK;
    msg.symbol = data.symbol();
    msg.exchange = data.exchange();
    
    Message wrapper;
    wrapper.set_type(MessageType::ORDERBOOK_UPDATE);
    wrapper.set_allocated_orderbook(new OrderBookUpdate(data));
    msg.message = std::move(wrapper);
    
    if (!message_queue_->push(msg)) {
        dropped_messages_++;
        std::cerr << "Broadcast queue full, dropping orderbook" << std::endl;
    } else {
        queued_messages_++;
    }
}

void Broadcaster::BroadcastTrade(const TradeUpdate& data) {
    BroadcastMessage msg;
    msg.type = BroadcastMessage::TRADE;
    msg.symbol = data.symbol();
    msg.exchange = data.exchange();
    
    Message wrapper;
    wrapper.set_type(MessageType::TRADE_UPDATE);
    wrapper.set_allocated_trade(new TradeUpdate(data));
    msg.message = std::move(wrapper);
    
    if (!message_queue_->push(msg)) {
        dropped_messages_++;
        std::cerr << "Broadcast queue full, dropping trade" << std::endl;
    } else {
        queued_messages_++;
    }
}

void Broadcaster::BroadcastAccountUpdate(const std::string& client_id, 
                                       const BalanceResponse& balance) {
    BroadcastMessage msg;
    msg.type = BroadcastMessage::ACCOUNT_UPDATE;
    msg.target_clients.push_back(client_id);
    
    Message wrapper;
    wrapper.set_type(MessageType::BALANCE_RESPONSE);
    wrapper.set_allocated_balance_response(new BalanceResponse(balance));
    msg.message = std::move(wrapper);
    
    if (!message_queue_->push(msg)) {
        dropped_messages_++;
    } else {
        queued_messages_++;
    }
}

void Broadcaster::BroadcastOrderUpdate(const std::string& client_id, 
                                     const OrderStatusUpdate& update) {
    BroadcastMessage msg;
    msg.type = BroadcastMessage::ORDER_UPDATE;
    msg.target_clients.push_back(client_id);
    
    Message wrapper;
    wrapper.set_type(MessageType::ORDER_STATUS_UPDATE);
    wrapper.set_allocated_order_update(new OrderStatusUpdate(update));
    msg.message = std::move(wrapper);
    
    if (!message_queue_->push(msg)) {
        dropped_messages_++;
    } else {
        queued_messages_++;
    }
}

void Broadcaster::WorkerThread() {
    batch_buffer_.clear();
    
    while (running_.load()) {
        // 배치로 메시지 가져오기
        batch_buffer_.clear();
        size_t count = 0;
        
        BroadcastMessage msg;
        while (count < BATCH_SIZE && message_queue_->pop(msg)) {
            batch_buffer_.push_back(std::move(msg));
            count++;
            queued_messages_--;
        }
        
        if (batch_buffer_.empty()) {
            std::this_thread::sleep_for(std::chrono::microseconds(100));
            continue;
        }
        
        // 배치 처리
        for (const auto& msg : batch_buffer_) {
            ProcessBroadcastMessage(msg);
        }
        
        // 캐시 정리 (주기적으로)
        static auto last_cleanup = std::chrono::steady_clock::now();
        auto now = std::chrono::steady_clock::now();
        if (std::chrono::duration_cast<std::chrono::seconds>(
                now - last_cleanup).count() > 1) {
            CleanupCache();
            last_cleanup = now;
        }
    }
}

void Broadcaster::ProcessBroadcastMessage(const BroadcastMessage& msg) {
    // 타겟 클라이언트 결정
    std::vector<std::string> target_clients;
    
    if (msg.target_clients.empty()) {
        // 브로드캐스트 - 구독자 찾기
        ChannelType channel;
        switch (msg.type) {
            case BroadcastMessage::MARKET_DATA:
                channel = ChannelType::TICKER;
                break;
            case BroadcastMessage::ORDERBOOK:
                channel = ChannelType::ORDERBOOK;
                break;
            case BroadcastMessage::TRADE:
                channel = ChannelType::TRADES;
                break;
            default:
                return;
        }
        
        target_clients = GetSubscribedClients(msg.symbol, channel);
    } else {
        target_clients = msg.target_clients;
    }
    
    if (target_clients.empty()) {
        return;
    }
    
    // 메시지 직렬화 (캐시 확인)
    std::string cache_key = std::to_string(msg.type) + ":" + 
                           msg.symbol + ":" + 
                           std::to_string(msg.message.timestamp().seconds());
    
    std::vector<uint8_t> serialized;
    if (!GetCachedMessage(cache_key, serialized)) {
        // 직렬화
        size_t msg_size = msg.message.ByteSize();
        serialized.resize(4 + msg_size);
        
        // 길이 헤더 (Big Endian)
        serialized[0] = (msg_size >> 24) & 0xFF;
        serialized[1] = (msg_size >> 16) & 0xFF;
        serialized[2] = (msg_size >> 8) & 0xFF;
        serialized[3] = msg_size & 0xFF;
        
        // 메시지 본문
        msg.message.SerializeToArray(serialized.data() + 4, msg_size);
        
        // 캐시 저장
        CacheSerializedMessage(cache_key, serialized);
    }
    
    // 클라이언트에게 전송
    BroadcastToClients(target_clients, serialized);
    
    broadcasted_messages_++;
}

std::vector<std::string> Broadcaster::GetSubscribedClients(
    const std::string& symbol, ChannelType channel) {
    
    std::shared_lock<std::shared_mutex> lock(subscriptions_mutex_);
    
    auto sym_it = subscriptions_.find(symbol);
    if (sym_it == subscriptions_.end()) {
        return {};
    }
    
    auto ch_it = sym_it->second.find(channel);
    if (ch_it == sym_it->second.end()) {
        return {};
    }
    
    return std::vector<std::string>(ch_it->second.begin(), ch_it->second.end());
}

void Broadcaster::BroadcastToClients(const std::vector<std::string>& clients,
                                   const std::vector<uint8_t>& data) {
    for (const auto& client_id : clients) {
        if (!server_->SendToClient(client_id, data.data(), data.size())) {
            // 전송 실패 - 클라이언트가 연결 해제되었을 수 있음
        }
    }
}

void Broadcaster::UpdateSubscriptions(const std::string& client_id,
                                    const std::string& symbol,
                                    ChannelType channel,
                                    bool subscribe) {
    std::lock_guard<std::shared_mutex> lock(subscriptions_mutex_);
    
    if (subscribe) {
        subscriptions_[symbol][channel].insert(client_id);
        client_subscriptions_[client_id].insert({symbol, channel});
    } else {
        auto sym_it = subscriptions_.find(symbol);
        if (sym_it != subscriptions_.end()) {
            auto ch_it = sym_it->second.find(channel);
            if (ch_it != sym_it->second.end()) {
                ch_it->second.erase(client_id);
                if (ch_it->second.empty()) {
                    sym_it->second.erase(ch_it);
                }
                if (sym_it->second.empty()) {
                    subscriptions_.erase(sym_it);
                }
            }
        }
        
        auto client_it = client_subscriptions_.find(client_id);
        if (client_it != client_subscriptions_.end()) {
            client_it->second.erase({symbol, channel});
            if (client_it->second.empty()) {
                client_subscriptions_.erase(client_it);
            }
        }
    }
}

void Broadcaster::CacheSerializedMessage(const std::string& key,
                                       const std::vector<uint8_t>& data) {
    std::lock_guard<std::shared_mutex> lock(cache_mutex_);
    
    // 캐시 크기 제한
    if (serialization_cache_.size() >= MAX_CACHE_SIZE) {
        // 가장 오래된 항목 제거
        auto oldest = serialization_cache_.begin();
        for (auto it = serialization_cache_.begin(); 
             it != serialization_cache_.end(); ++it) {
            if (it->second.timestamp < oldest->second.timestamp) {
                oldest = it;
            }
        }
        serialization_cache_.erase(oldest);
    }
    
    SerializedMessage msg;
    msg.data = data;
    msg.timestamp = std::chrono::steady_clock::now();
    serialization_cache_[key] = std::move(msg);
}

bool Broadcaster::GetCachedMessage(const std::string& key,
                                 std::vector<uint8_t>& data) {
    std::shared_lock<std::shared_mutex> lock(cache_mutex_);
    
    auto it = serialization_cache_.find(key);
    if (it != serialization_cache_.end()) {
        auto age_ms = std::chrono::duration_cast<std::chrono::milliseconds>(
            std::chrono::steady_clock::now() - it->second.timestamp).count();
        
        if (age_ms <= CACHE_TTL_MS) {
            data = it->second.data;
            return true;
        }
    }
    
    return false;
}

void Broadcaster::CleanupCache() {
    std::lock_guard<std::shared_mutex> lock(cache_mutex_);
    
    auto now = std::chrono::steady_clock::now();
    
    for (auto it = serialization_cache_.begin(); 
         it != serialization_cache_.end();) {
        auto age_ms = std::chrono::duration_cast<std::chrono::milliseconds>(
            now - it->second.timestamp).count();
        
        if (age_ms > CACHE_TTL_MS) {
            it = serialization_cache_.erase(it);
        } else {
            ++it;
        }
    }
}

} // namespace tcp
} // namespace oms