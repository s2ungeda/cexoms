#pragma once

#include <thread>
#include <atomic>
#include <memory>
#include <vector>
#include <unordered_map>
#include <shared_mutex>
#include <functional>

#include "../include/ring_buffer.h"
#include "proto/tcp/v1/messages.pb.h"

namespace oms {
namespace tcp {

class TcpServer;
class ClientSession;

// 브로드캐스트 메시지 타입
struct BroadcastMessage {
    enum Type {
        MARKET_DATA,
        ORDERBOOK,
        TRADE,
        ACCOUNT_UPDATE,
        ORDER_UPDATE
    };
    
    Type type;
    std::string symbol;
    std::string exchange;
    Message message;
    std::vector<std::string> target_clients; // 비어있으면 전체 브로드캐스트
};

class Broadcaster {
public:
    explicit Broadcaster(TcpServer* server);
    ~Broadcaster();

    // 브로드캐스터 시작/종료
    void Start();
    void Stop();

    // 마켓 데이터 브로드캐스트
    void BroadcastMarketData(const MarketDataUpdate& data);
    void BroadcastOrderBook(const OrderBookUpdate& data);
    void BroadcastTrade(const TradeUpdate& data);

    // 계좌 업데이트 (특정 클라이언트에게만)
    void BroadcastAccountUpdate(const std::string& client_id, 
                               const BalanceResponse& balance);
    void BroadcastOrderUpdate(const std::string& client_id, 
                            const OrderStatusUpdate& update);

    // 구독자 관리
    void UpdateSubscriptions(const std::string& client_id, 
                           const std::string& symbol,
                           ChannelType channel,
                           bool subscribe);

    // 성능 통계
    uint64_t GetQueuedMessages() const { return queued_messages_.load(); }
    uint64_t GetBroadcastedMessages() const { return broadcasted_messages_.load(); }
    uint64_t GetDroppedMessages() const { return dropped_messages_.load(); }

private:
    // 워커 스레드
    void WorkerThread();
    
    // 메시지 처리
    void ProcessBroadcastMessage(const BroadcastMessage& msg);
    
    // 구독자 찾기
    std::vector<std::string> GetSubscribedClients(const std::string& symbol, 
                                                 ChannelType channel);
    
    // 효율적인 브로드캐스트
    void BroadcastToClients(const std::vector<std::string>& clients, 
                          const Message& msg);
    
    // 메시지 직렬화 캐시
    struct SerializedMessage {
        std::vector<uint8_t> data;
        std::chrono::steady_clock::time_point timestamp;
    };
    
    void CacheSerializedMessage(const std::string& key, 
                               const std::vector<uint8_t>& data);
    bool GetCachedMessage(const std::string& key, 
                         std::vector<uint8_t>& data);
    void CleanupCache();

private:
    TcpServer* server_;
    
    // 워커 스레드
    std::thread worker_thread_;
    std::atomic<bool> running_{false};
    
    // 메시지 큐 (Lock-free)
    static constexpr size_t QUEUE_SIZE = 16384;
    using MessageQueue = LockFreeRingBuffer<BroadcastMessage, QUEUE_SIZE>;
    std::unique_ptr<MessageQueue> message_queue_;
    
    // 구독 관리 (symbol -> channel -> clients)
    mutable std::shared_mutex subscriptions_mutex_;
    std::unordered_map<std::string, 
        std::unordered_map<ChannelType, std::set<std::string>>> subscriptions_;
    
    // 역방향 매핑 (client -> subscriptions)
    std::unordered_map<std::string, 
        std::set<std::pair<std::string, ChannelType>>> client_subscriptions_;
    
    // 직렬화 캐시 (메시지 재사용)
    mutable std::shared_mutex cache_mutex_;
    std::unordered_map<std::string, SerializedMessage> serialization_cache_;
    static constexpr size_t MAX_CACHE_SIZE = 1000;
    static constexpr int CACHE_TTL_MS = 100; // 100ms
    
    // 통계
    std::atomic<uint64_t> queued_messages_{0};
    std::atomic<uint64_t> broadcasted_messages_{0};
    std::atomic<uint64_t> dropped_messages_{0};
    
    // 배치 처리
    static constexpr size_t BATCH_SIZE = 100;
    std::vector<BroadcastMessage> batch_buffer_;
};

} // namespace tcp
} // namespace oms