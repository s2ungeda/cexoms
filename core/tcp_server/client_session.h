#pragma once

#include <string>
#include <atomic>
#include <chrono>
#include <set>
#include <mutex>
#include <vector>
#include <memory>

#include "../include/ring_buffer.h"

namespace oms {
namespace tcp {

// 클라이언트 상태
enum class ClientState {
    CONNECTED,      // 연결됨
    AUTHENTICATED,  // 인증 완료
    DISCONNECTING,  // 연결 해제 중
    DISCONNECTED    // 연결 해제됨
};

// 클라이언트 권한
enum class ClientPermission {
    READ = 1,      // 데이터 조회
    TRADE = 2,     // 주문 실행
    TRANSFER = 4,  // 자금 이체
    ADMIN = 8      // 관리자
};

// 구독 채널 타입
enum class ChannelType {
    TICKER,
    ORDERBOOK,
    TRADES,
    ORDERS,
    POSITIONS,
    BALANCE
};

class ClientSession {
public:
    ClientSession(const std::string& id, int socket_fd);
    ~ClientSession();

    // 기본 정보
    const std::string& GetId() const { return id_; }
    int GetSocket() const { return socket_fd_; }
    ClientState GetState() const { return state_.load(); }
    
    // 상태 관리
    void SetState(ClientState state) { state_ = state; }
    bool IsAuthenticated() const { return state_ == ClientState::AUTHENTICATED; }
    bool IsActive() const { 
        auto state = state_.load();
        return state == ClientState::CONNECTED || state == ClientState::AUTHENTICATED;
    }

    // 인증
    void SetAuthenticated(const std::string& api_key, uint32_t permissions);
    bool HasPermission(ClientPermission perm) const {
        return (permissions_ & static_cast<uint32_t>(perm)) != 0;
    }
    const std::string& GetApiKey() const { return api_key_; }

    // 구독 관리
    bool Subscribe(const std::string& symbol, ChannelType channel);
    bool Unsubscribe(const std::string& symbol, ChannelType channel);
    bool IsSubscribed(const std::string& symbol, ChannelType channel) const;
    std::set<std::string> GetSubscribedSymbols(ChannelType channel) const;

    // 버퍼 관리
    bool AddToSendBuffer(const uint8_t* data, size_t len);
    bool HasDataToSend() const { return send_buffer_pos_ > 0; }
    size_t GetSendData(uint8_t* buffer, size_t max_len);
    
    bool AddToRecvBuffer(const uint8_t* data, size_t len);
    bool ExtractMessage(std::vector<uint8_t>& message);

    // 통계
    void UpdateLastActivity() { 
        last_activity_ = std::chrono::steady_clock::now(); 
    }
    
    bool IsTimedOut(int timeout_ms) const {
        auto now = std::chrono::steady_clock::now();
        auto duration = std::chrono::duration_cast<std::chrono::milliseconds>(
            now - last_activity_).count();
        return duration > timeout_ms;
    }

    uint64_t GetMessagesSent() const { return messages_sent_.load(); }
    uint64_t GetMessagesReceived() const { return messages_received_.load(); }
    uint64_t GetBytesSent() const { return bytes_sent_.load(); }
    uint64_t GetBytesReceived() const { return bytes_received_.load(); }

    // 연결 정보
    void SetRemoteAddress(const std::string& addr) { remote_address_ = addr; }
    const std::string& GetRemoteAddress() const { return remote_address_; }
    
    std::chrono::steady_clock::time_point GetConnectTime() const { 
        return connect_time_; 
    }

private:
    // 기본 정보
    std::string id_;
    int socket_fd_;
    std::atomic<ClientState> state_{ClientState::CONNECTED};
    
    // 인증 정보
    std::string api_key_;
    uint32_t permissions_ = 0;
    
    // 네트워크 정보
    std::string remote_address_;
    std::chrono::steady_clock::time_point connect_time_;
    std::chrono::steady_clock::time_point last_activity_;
    
    // 구독 관리 (symbol -> channels)
    mutable std::mutex subscriptions_mutex_;
    std::unordered_map<ChannelType, std::set<std::string>> subscriptions_;
    
    // 버퍼 관리
    static constexpr size_t BUFFER_SIZE = 65536;
    alignas(64) uint8_t recv_buffer_[BUFFER_SIZE];
    alignas(64) uint8_t send_buffer_[BUFFER_SIZE];
    
    size_t recv_buffer_pos_ = 0;
    size_t send_buffer_pos_ = 0;
    
    mutable std::mutex send_mutex_;
    mutable std::mutex recv_mutex_;
    
    // 통계
    std::atomic<uint64_t> messages_sent_{0};
    std::atomic<uint64_t> messages_received_{0};
    std::atomic<uint64_t> bytes_sent_{0};
    std::atomic<uint64_t> bytes_received_{0};
    
    // 메시지 프레이밍
    bool ParseMessageHeader(size_t& message_len);
};

} // namespace tcp
} // namespace oms