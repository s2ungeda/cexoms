#include "tcp_server.h"
#include <iostream>
#include <cstring>

namespace oms {
namespace tcp {

TcpServer::TcpServer(const ServerConfig& config) 
    : config_(config) {
    broadcaster_ = std::make_unique<Broadcaster>(this);
}

TcpServer::~TcpServer() {
    Stop();
}

bool TcpServer::Start() {
    if (running_.load()) {
        return false;
    }

    // 소켓 초기화
    if (!InitializeSocket()) {
        return false;
    }

#ifdef _WIN32
    if (!InitializeIOCP()) {
        close(listen_fd_);
        return false;
    }
#else
    if (!InitializeEpoll()) {
        close(listen_fd_);
        return false;
    }
#endif

    running_ = true;
    stop_requested_ = false;

    // 워커 스레드 시작
    for (int i = 0; i < config_.worker_threads; ++i) {
#ifdef _WIN32
        worker_threads_.emplace_back(&TcpServer::IOCPWorkerThread, this, i);
#else
        worker_threads_.emplace_back(&TcpServer::EpollWorkerThread, this, i);
#endif
    }

    // Accept 스레드
    accept_thread_ = std::thread(&TcpServer::AcceptConnections, this);
    
    // Heartbeat 스레드
    heartbeat_thread_ = std::thread(&TcpServer::HandleHeartbeat, this);

    // 브로드캐스터 시작
    broadcaster_->Start();

    std::cout << "TCP Server started on " << config_.address 
              << ":" << config_.port << std::endl;
    
    return true;
}

void TcpServer::Stop() {
    if (!running_.load()) {
        return;
    }

    stop_requested_ = true;
    running_ = false;

    // 모든 연결 종료
    CloseAllConnections();

    // 스레드 종료 대기
    if (accept_thread_.joinable()) {
        accept_thread_.join();
    }
    
    if (heartbeat_thread_.joinable()) {
        heartbeat_thread_.join();
    }
    
    for (auto& thread : worker_threads_) {
        if (thread.joinable()) {
            thread.join();
        }
    }

    // 브로드캐스터 종료
    broadcaster_->Stop();

    // 리소스 정리
#ifdef _WIN32
    if (iocp_ != INVALID_HANDLE_VALUE) {
        CloseHandle(iocp_);
    }
#else
    if (epoll_fd_ != -1) {
        close(epoll_fd_);
    }
    delete[] events_;
#endif

    if (listen_fd_ != -1) {
        close(listen_fd_);
    }
}

bool TcpServer::InitializeSocket() {
    // 소켓 생성
    listen_fd_ = socket(AF_INET, SOCK_STREAM, 0);
    if (listen_fd_ < 0) {
        std::cerr << "Failed to create socket: " << GetLastSocketError() << std::endl;
        return false;
    }

    // 소켓 옵션 설정
    if (!SetSocketReuseAddr(listen_fd_)) {
        return false;
    }
    
    if (!SetSocketNonBlocking(listen_fd_)) {
        return false;
    }

    // 주소 바인딩
    sockaddr_in addr{};
    addr.sin_family = AF_INET;
    addr.sin_port = htons(config_.port);
    
    if (config_.address == "0.0.0.0") {
        addr.sin_addr.s_addr = INADDR_ANY;
    } else {
        inet_pton(AF_INET, config_.address.c_str(), &addr.sin_addr);
    }

    if (bind(listen_fd_, (sockaddr*)&addr, sizeof(addr)) < 0) {
        std::cerr << "Failed to bind: " << GetLastSocketError() << std::endl;
        return false;
    }

    // 리스닝 시작
    if (listen(listen_fd_, SOMAXCONN) < 0) {
        std::cerr << "Failed to listen: " << GetLastSocketError() << std::endl;
        return false;
    }

    return true;
}

void TcpServer::AcceptConnections() {
    while (running_.load()) {
        sockaddr_in client_addr{};
        socklen_t addr_len = sizeof(client_addr);
        
        int client_fd = accept(listen_fd_, (sockaddr*)&client_addr, &addr_len);
        if (client_fd < 0) {
            if (errno == EAGAIN || errno == EWOULDBLOCK) {
                std::this_thread::sleep_for(std::chrono::milliseconds(10));
                continue;
            }
            std::cerr << "Accept failed: " << GetLastSocketError() << std::endl;
            continue;
        }

        // 연결 수 확인
        if (GetActiveConnections() >= config_.max_clients) {
            std::cerr << "Max clients reached, rejecting connection" << std::endl;
            close(client_fd);
            continue;
        }

        HandleNewConnection(client_fd, client_addr);
    }
}

// 플랫폼별 구현은 별도 파일로 분리 예정

} // namespace tcp
} // namespace oms