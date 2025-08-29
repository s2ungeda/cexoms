#include "client_session.h"
#include <cstring>
#include <algorithm>

namespace oms {
namespace tcp {

ClientSession::ClientSession(const std::string& id, int socket_fd)
    : id_(id)
    , socket_fd_(socket_fd)
    , connect_time_(std::chrono::steady_clock::now())
    , last_activity_(std::chrono::steady_clock::now()) {
}

ClientSession::~ClientSession() {
    if (socket_fd_ >= 0) {
        close(socket_fd_);
    }
}

void ClientSession::SetAuthenticated(const std::string& api_key, uint32_t permissions) {
    api_key_ = api_key;
    permissions_ = permissions;
    SetState(ClientState::AUTHENTICATED);
}

bool ClientSession::Subscribe(const std::string& symbol, ChannelType channel) {
    std::lock_guard<std::mutex> lock(subscriptions_mutex_);
    subscriptions_[channel].insert(symbol);
    return true;
}

bool ClientSession::Unsubscribe(const std::string& symbol, ChannelType channel) {
    std::lock_guard<std::mutex> lock(subscriptions_mutex_);
    auto it = subscriptions_.find(channel);
    if (it != subscriptions_.end()) {
        it->second.erase(symbol);
        if (it->second.empty()) {
            subscriptions_.erase(it);
        }
        return true;
    }
    return false;
}

bool ClientSession::IsSubscribed(const std::string& symbol, ChannelType channel) const {
    std::lock_guard<std::mutex> lock(subscriptions_mutex_);
    auto it = subscriptions_.find(channel);
    if (it != subscriptions_.end()) {
        return it->second.count(symbol) > 0;
    }
    return false;
}

std::set<std::string> ClientSession::GetSubscribedSymbols(ChannelType channel) const {
    std::lock_guard<std::mutex> lock(subscriptions_mutex_);
    auto it = subscriptions_.find(channel);
    if (it != subscriptions_.end()) {
        return it->second;
    }
    return {};
}

bool ClientSession::AddToSendBuffer(const uint8_t* data, size_t len) {
    std::lock_guard<std::mutex> lock(send_mutex_);
    
    if (send_buffer_pos_ + len > BUFFER_SIZE) {
        return false; // Buffer full
    }
    
    std::memcpy(send_buffer_ + send_buffer_pos_, data, len);
    send_buffer_pos_ += len;
    bytes_sent_ += len;
    
    return true;
}

size_t ClientSession::GetSendData(uint8_t* buffer, size_t max_len) {
    std::lock_guard<std::mutex> lock(send_mutex_);
    
    size_t to_send = std::min(send_buffer_pos_, max_len);
    if (to_send > 0) {
        std::memcpy(buffer, send_buffer_, to_send);
        
        // Shift remaining data
        if (to_send < send_buffer_pos_) {
            std::memmove(send_buffer_, send_buffer_ + to_send, 
                        send_buffer_pos_ - to_send);
        }
        send_buffer_pos_ -= to_send;
    }
    
    return to_send;
}

bool ClientSession::AddToRecvBuffer(const uint8_t* data, size_t len) {
    std::lock_guard<std::mutex> lock(recv_mutex_);
    
    if (recv_buffer_pos_ + len > BUFFER_SIZE) {
        return false; // Buffer full
    }
    
    std::memcpy(recv_buffer_ + recv_buffer_pos_, data, len);
    recv_buffer_pos_ += len;
    bytes_received_ += len;
    
    UpdateLastActivity();
    
    return true;
}

bool ClientSession::ExtractMessage(std::vector<uint8_t>& message) {
    std::lock_guard<std::mutex> lock(recv_mutex_);
    
    // 최소 4바이트(길이 헤더) 필요
    if (recv_buffer_pos_ < 4) {
        return false;
    }
    
    // 메시지 길이 읽기 (Big Endian)
    uint32_t msg_len = (recv_buffer_[0] << 24) |
                       (recv_buffer_[1] << 16) |
                       (recv_buffer_[2] << 8) |
                       recv_buffer_[3];
    
    // 전체 메시지가 도착했는지 확인
    if (recv_buffer_pos_ < 4 + msg_len) {
        return false;
    }
    
    // 메시지 추출
    message.resize(msg_len);
    std::memcpy(message.data(), recv_buffer_ + 4, msg_len);
    
    // 버퍼에서 메시지 제거
    size_t total_len = 4 + msg_len;
    if (total_len < recv_buffer_pos_) {
        std::memmove(recv_buffer_, recv_buffer_ + total_len, 
                    recv_buffer_pos_ - total_len);
    }
    recv_buffer_pos_ -= total_len;
    
    messages_received_++;
    
    return true;
}

bool ClientSession::ParseMessageHeader(size_t& message_len) {
    if (recv_buffer_pos_ < 4) {
        return false;
    }
    
    message_len = (recv_buffer_[0] << 24) |
                  (recv_buffer_[1] << 16) |
                  (recv_buffer_[2] << 8) |
                  recv_buffer_[3];
    
    return true;
}

} // namespace tcp
} // namespace oms