#pragma once

#include <atomic>
#include <memory>
#include <thread>
#include <unordered_map>
#include <vector>
#include <mutex>
#include <condition_variable>
#include <functional>

#ifdef _WIN32
    #include <winsock2.h>
    #include <ws2tcpip.h>
#else
    #include <sys/epoll.h>
    #include <sys/socket.h>
    #include <netinet/in.h>
    #include <arpa/inet.h>
    #include <fcntl.h>
    #include <unistd.h>
#endif

#include "client_session.h"
#include "message_handler.h"
#include "broadcaster.h"
#include "../include/ring_buffer.h"

namespace oms {
namespace tcp {

// TCP 서버 설정
struct ServerConfig {
    std::string address = "0.0.0.0";
    uint16_t port = 9090;
    int max_clients = 100;
    int worker_threads = 4;
    size_t recv_buffer_size = 65536;
    size_t send_buffer_size = 65536;
    int heartbeat_interval_ms = 30000;
    int client_timeout_ms = 60000;
};

// 서버 통계
struct ServerStats {
    std::atomic<uint64_t> total_connections{0};
    std::atomic<uint64_t> active_connections{0};
    std::atomic<uint64_t> messages_received{0};
    std::atomic<uint64_t> messages_sent{0};
    std::atomic<uint64_t> bytes_received{0};
    std::atomic<uint64_t> bytes_sent{0};
    std::atomic<uint64_t> errors{0};
};

class TcpServer {
public:
    explicit TcpServer(const ServerConfig& config);
    ~TcpServer();

    // 서버 시작/종료
    bool Start();
    void Stop();
    bool IsRunning() const { return running_.load(); }

    // 브로드캐스트 메서드
    void BroadcastMarketData(const MarketDataUpdate& data);
    void BroadcastOrderBook(const OrderBookUpdate& data);
    void BroadcastTrade(const TradeUpdate& data);

    // 특정 클라이언트에게 메시지 전송
    bool SendToClient(const std::string& client_id, const Message& msg);
    bool SendToClients(const std::vector<std::string>& client_ids, const Message& msg);

    // 클라이언트 관리
    size_t GetActiveConnections() const { return stats_.active_connections.load(); }
    std::vector<std::string> GetConnectedClients() const;
    bool DisconnectClient(const std::string& client_id, const std::string& reason);

    // 통계
    const ServerStats& GetStats() const { return stats_; }
    void ResetStats();

    // 메시지 핸들러 설정
    void SetMessageHandler(std::shared_ptr<MessageHandler> handler) {
        message_handler_ = handler;
    }

private:
    // 플랫폼별 I/O 처리
#ifdef _WIN32
    bool InitializeIOCP();
    void IOCPWorkerThread(int thread_id);
#else
    bool InitializeEpoll();
    void EpollWorkerThread(int thread_id);
#endif

    // 네트워크 초기화
    bool InitializeSocket();
    bool SetSocketNonBlocking(int fd);
    bool SetSocketReuseAddr(int fd);
    bool SetTcpNoDelay(int fd);
    
    // 연결 관리
    void AcceptConnections();
    void HandleNewConnection(int client_fd, const sockaddr_in& client_addr);
    void RemoveClient(const std::string& client_id);
    
    // 메시지 처리
    void ProcessClientMessage(ClientSession* session, const Message& msg);
    void HandleHeartbeat();
    
    // 클린업
    void CleanupDeadConnections();
    void CloseAllConnections();

private:
    ServerConfig config_;
    ServerStats stats_;
    
    // 네트워크
    int listen_fd_ = -1;
    
#ifdef _WIN32
    HANDLE iocp_ = INVALID_HANDLE_VALUE;
#else
    int epoll_fd_ = -1;
    struct epoll_event* events_ = nullptr;
#endif
    
    // 클라이언트 관리
    mutable std::mutex clients_mutex_;
    std::unordered_map<std::string, std::unique_ptr<ClientSession>> clients_;
    std::unordered_map<int, std::string> fd_to_client_id_;
    
    // 메시지 처리
    std::shared_ptr<MessageHandler> message_handler_;
    std::unique_ptr<Broadcaster> broadcaster_;
    
    // 워커 스레드
    std::vector<std::thread> worker_threads_;
    std::thread accept_thread_;
    std::thread heartbeat_thread_;
    
    // 동기화
    std::atomic<bool> running_{false};
    std::atomic<bool> stop_requested_{false};
    std::condition_variable stop_cv_;
    std::mutex stop_mutex_;
    
    // Lock-free 메시지 큐 (세션별)
    using MessageQueue = LockFreeRingBuffer<std::pair<std::string, Message>, 1024>;
    std::unique_ptr<MessageQueue> incoming_messages_;
    std::unique_ptr<MessageQueue> outgoing_messages_;
};

// 헬퍼 함수
std::string GetLastSocketError();
std::string FormatAddress(const sockaddr_in& addr);

} // namespace tcp
} // namespace oms