#include "tcp_server.h"
#include <random>
#include <sstream>
#include <iomanip>

namespace oms {
namespace tcp {

// 클라이언트에게 메시지 전송
bool TcpServer::SendToClient(const std::string& client_id, 
                           const uint8_t* data, size_t len) {
    ClientSession* session = nullptr;
    {
        std::lock_guard<std::mutex> lock(clients_mutex_);
        auto it = clients_.find(client_id);
        if (it != clients_.end()) {
            session = it->second.get();
        }
    }
    
    if (!session || !session->IsActive()) {
        return false;
    }
    
    if (!session->AddToSendBuffer(data, len)) {
        return false;
    }
    
    // EPOLLOUT 이벤트 활성화
    int fd = session->GetSocket();
    struct epoll_event ev;
    ev.events = EPOLLIN | EPOLLOUT | EPOLLET | EPOLLRDHUP;
    ev.data.fd = fd;
    epoll_ctl(epoll_fd_, EPOLL_CTL_MOD, fd, &ev);
    
    stats_.messages_sent++;
    
    return true;
}

bool TcpServer::SendToClient(const std::string& client_id, const Message& msg) {
    size_t msg_size = msg.ByteSize();
    std::vector<uint8_t> buffer(4 + msg_size);
    
    // Length header
    buffer[0] = (msg_size >> 24) & 0xFF;
    buffer[1] = (msg_size >> 16) & 0xFF;
    buffer[2] = (msg_size >> 8) & 0xFF;
    buffer[3] = msg_size & 0xFF;
    
    // Message body
    msg.SerializeToArray(buffer.data() + 4, msg_size);
    
    return SendToClient(client_id, buffer.data(), buffer.size());
}

// 여러 클라이언트에게 전송
bool TcpServer::SendToClients(const std::vector<std::string>& client_ids, 
                            const Message& msg) {
    // 메시지를 한 번만 직렬화
    size_t msg_size = msg.ByteSize();
    std::vector<uint8_t> buffer(4 + msg_size);
    
    buffer[0] = (msg_size >> 24) & 0xFF;
    buffer[1] = (msg_size >> 16) & 0xFF;
    buffer[2] = (msg_size >> 8) & 0xFF;
    buffer[3] = msg_size & 0xFF;
    
    msg.SerializeToArray(buffer.data() + 4, msg_size);
    
    bool all_success = true;
    for (const auto& client_id : client_ids) {
        if (!SendToClient(client_id, buffer.data(), buffer.size())) {
            all_success = false;
        }
    }
    
    return all_success;
}

// 연결된 클라이언트 목록
std::vector<std::string> TcpServer::GetConnectedClients() const {
    std::lock_guard<std::mutex> lock(clients_mutex_);
    
    std::vector<std::string> result;
    result.reserve(clients_.size());
    
    for (const auto& [id, session] : clients_) {
        if (session->IsActive()) {
            result.push_back(id);
        }
    }
    
    return result;
}

// 클라이언트 연결 해제
bool TcpServer::DisconnectClient(const std::string& client_id, 
                               const std::string& reason) {
    ClientSession* session = nullptr;
    {
        std::lock_guard<std::mutex> lock(clients_mutex_);
        auto it = clients_.find(client_id);
        if (it != clients_.end()) {
            session = it->second.get();
        }
    }
    
    if (!session) {
        return false;
    }
    
    // 연결 종료 메시지 전송
    Message msg;
    msg.set_type(MessageType::LOGOUT_RESPONSE);
    auto* logout = msg.mutable_logout_response();
    logout->set_success(true);
    // logout->set_reason(reason);
    
    SendToClient(client_id, msg);
    
    // 연결 제거
    RemoveClient(client_id);
    
    return true;
}

// 클라이언트 제거
void TcpServer::RemoveClient(const std::string& client_id) {
    int fd = -1;
    
    {
        std::lock_guard<std::mutex> lock(clients_mutex_);
        
        auto it = clients_.find(client_id);
        if (it == clients_.end()) {
            return;
        }
        
        fd = it->second->GetSocket();
        
        // epoll에서 제거
        if (epoll_fd_ >= 0 && fd >= 0) {
            epoll_ctl(epoll_fd_, EPOLL_CTL_DEL, fd, nullptr);
        }
        
        // fd 매핑 제거
        fd_to_client_id_.erase(fd);
        
        // 구독 정보 제거
        broadcaster_->UpdateSubscriptions(client_id, "", ChannelType::TICKER, false);
        
        // 클라이언트 제거
        clients_.erase(it);
    }
    
    stats_.active_connections--;
    
    std::cout << "Client disconnected: " << client_id << std::endl;
}

// 메시지 처리
void TcpServer::ProcessRawMessage(ClientSession* session, 
                                const std::vector<uint8_t>& raw_msg) {
    Message msg;
    if (!msg.ParseFromArray(raw_msg.data(), raw_msg.size())) {
        std::cerr << "Failed to parse message from " << session->GetId() << std::endl;
        return;
    }
    
    stats_.messages_received++;
    
    if (message_handler_) {
        message_handler_->HandleMessage(session, msg);
    }
}

// 하트비트 처리
void TcpServer::HandleHeartbeat() {
    while (running_.load()) {
        std::this_thread::sleep_for(
            std::chrono::milliseconds(config_.heartbeat_interval_ms));
        
        if (!running_.load()) break;
        
        // 타임아웃된 연결 정리
        CleanupDeadConnections();
        
        // 활성 클라이언트에게 하트비트 전송
        Message heartbeat;
        heartbeat.set_type(MessageType::HEARTBEAT_REQUEST);
        auto* hb = heartbeat.mutable_heartbeat_request();
        hb->set_sequence(std::chrono::steady_clock::now().time_since_epoch().count());
        
        auto clients = GetConnectedClients();
        SendToClients(clients, heartbeat);
    }
}

// 죽은 연결 정리
void TcpServer::CleanupDeadConnections() {
    std::vector<std::string> to_remove;
    
    {
        std::lock_guard<std::mutex> lock(clients_mutex_);
        for (const auto& [id, session] : clients_) {
            if (session->IsTimedOut(config_.client_timeout_ms)) {
                to_remove.push_back(id);
            }
        }
    }
    
    for (const auto& id : to_remove) {
        std::cout << "Removing timed out client: " << id << std::endl;
        RemoveClient(id);
    }
}

// 모든 연결 종료
void TcpServer::CloseAllConnections() {
    std::lock_guard<std::mutex> lock(clients_mutex_);
    
    for (auto& [id, session] : clients_) {
        if (session->GetSocket() >= 0) {
            close(session->GetSocket());
        }
    }
    
    clients_.clear();
    fd_to_client_id_.clear();
}

// 통계 초기화
void TcpServer::ResetStats() {
    stats_.total_connections = 0;
    stats_.messages_received = 0;
    stats_.messages_sent = 0;
    stats_.bytes_received = 0;
    stats_.bytes_sent = 0;
    stats_.errors = 0;
}

// 클라이언트 ID 생성
std::string TcpServer::GenerateClientId() {
    static std::atomic<uint64_t> counter{0};
    
    std::stringstream ss;
    ss << "client_" << std::setfill('0') << std::setw(8) 
       << counter.fetch_add(1);
    
    return ss.str();
}

} // namespace tcp
} // namespace oms