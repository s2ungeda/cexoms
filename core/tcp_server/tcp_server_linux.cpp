#ifndef _WIN32

#include "tcp_server.h"
#include <sys/epoll.h>
#include <fcntl.h>
#include <errno.h>
#include <cstring>
#include <iostream>

namespace oms {
namespace tcp {

bool TcpServer::InitializeEpoll() {
    epoll_fd_ = epoll_create1(EPOLL_CLOEXEC);
    if (epoll_fd_ < 0) {
        std::cerr << "Failed to create epoll: " << strerror(errno) << std::endl;
        return false;
    }
    
    // Listen 소켓을 epoll에 추가
    struct epoll_event ev;
    ev.events = EPOLLIN | EPOLLET; // Edge-triggered
    ev.data.fd = listen_fd_;
    
    if (epoll_ctl(epoll_fd_, EPOLL_CTL_ADD, listen_fd_, &ev) < 0) {
        std::cerr << "Failed to add listen socket to epoll: " 
                  << strerror(errno) << std::endl;
        return false;
    }
    
    // 이벤트 배열 할당
    events_ = new epoll_event[config_.max_clients * 2];
    
    return true;
}

void TcpServer::EpollWorkerThread(int thread_id) {
    constexpr int MAX_EVENTS = 64;
    uint8_t buffer[config_.recv_buffer_size];
    
    while (running_.load()) {
        int nfds = epoll_wait(epoll_fd_, events_, MAX_EVENTS, 100);
        
        if (nfds < 0) {
            if (errno == EINTR) continue;
            std::cerr << "Epoll wait error: " << strerror(errno) << std::endl;
            break;
        }
        
        for (int i = 0; i < nfds; ++i) {
            int fd = events_[i].data.fd;
            uint32_t events = events_[i].events;
            
            // Listen 소켓에서 새 연결
            if (fd == listen_fd_) {
                // Accept는 별도 스레드에서 처리
                continue;
            }
            
            // 클라이언트 소켓 이벤트
            std::string client_id;
            {
                std::lock_guard<std::mutex> lock(clients_mutex_);
                auto it = fd_to_client_id_.find(fd);
                if (it == fd_to_client_id_.end()) {
                    continue;
                }
                client_id = it->second;
            }
            
            // 에러 또는 연결 종료
            if (events & (EPOLLERR | EPOLLHUP | EPOLLRDHUP)) {
                RemoveClient(client_id);
                continue;
            }
            
            // 읽기 가능
            if (events & EPOLLIN) {
                HandleClientRead(client_id, fd);
            }
            
            // 쓰기 가능
            if (events & EPOLLOUT) {
                HandleClientWrite(client_id, fd);
            }
        }
    }
}

void TcpServer::HandleClientRead(const std::string& client_id, int fd) {
    uint8_t buffer[config_.recv_buffer_size];
    
    while (true) {
        ssize_t n = recv(fd, buffer, sizeof(buffer), 0);
        
        if (n > 0) {
            // 클라이언트 세션 찾기
            ClientSession* session = nullptr;
            {
                std::lock_guard<std::mutex> lock(clients_mutex_);
                auto it = clients_.find(client_id);
                if (it != clients_.end()) {
                    session = it->second.get();
                }
            }
            
            if (session) {
                // 받은 데이터를 세션 버퍼에 추가
                if (!session->AddToRecvBuffer(buffer, n)) {
                    std::cerr << "Client " << client_id 
                              << " recv buffer full" << std::endl;
                    RemoveClient(client_id);
                    return;
                }
                
                // 완전한 메시지 추출 및 처리
                std::vector<uint8_t> message;
                while (session->ExtractMessage(message)) {
                    ProcessRawMessage(session, message);
                }
                
                stats_.bytes_received += n;
            }
        } else if (n == 0) {
            // 연결 종료
            RemoveClient(client_id);
            break;
        } else {
            if (errno == EAGAIN || errno == EWOULDBLOCK) {
                // 더 이상 읽을 데이터 없음
                break;
            } else {
                // 에러
                std::cerr << "recv error: " << strerror(errno) << std::endl;
                RemoveClient(client_id);
                break;
            }
        }
    }
}

void TcpServer::HandleClientWrite(const std::string& client_id, int fd) {
    ClientSession* session = nullptr;
    {
        std::lock_guard<std::mutex> lock(clients_mutex_);
        auto it = clients_.find(client_id);
        if (it != clients_.end()) {
            session = it->second.get();
        }
    }
    
    if (!session || !session->HasDataToSend()) {
        return;
    }
    
    uint8_t buffer[config_.send_buffer_size];
    
    while (session->HasDataToSend()) {
        size_t len = session->GetSendData(buffer, sizeof(buffer));
        if (len == 0) break;
        
        ssize_t n = send(fd, buffer, len, MSG_NOSIGNAL);
        
        if (n > 0) {
            stats_.bytes_sent += n;
            
            // 일부만 전송된 경우 처리 필요
            if (n < len) {
                // TODO: 나머지 데이터 다시 버퍼에 넣기
                break;
            }
        } else {
            if (errno == EAGAIN || errno == EWOULDBLOCK) {
                // 소켓 버퍼 가득 참
                break;
            } else {
                // 에러
                std::cerr << "send error: " << strerror(errno) << std::endl;
                RemoveClient(client_id);
                break;
            }
        }
    }
    
    // 모든 데이터 전송 완료시 EPOLLOUT 이벤트 제거
    if (!session->HasDataToSend()) {
        struct epoll_event ev;
        ev.events = EPOLLIN | EPOLLET | EPOLLRDHUP;
        ev.data.fd = fd;
        epoll_ctl(epoll_fd_, EPOLL_CTL_MOD, fd, &ev);
    }
}

void TcpServer::HandleNewConnection(int client_fd, const sockaddr_in& client_addr) {
    // 논블로킹 모드 설정
    if (!SetSocketNonBlocking(client_fd)) {
        close(client_fd);
        return;
    }
    
    // TCP_NODELAY 설정
    if (!SetTcpNoDelay(client_fd)) {
        close(client_fd);
        return;
    }
    
    // 클라이언트 세션 생성
    std::string client_id = GenerateClientId();
    auto session = std::make_unique<ClientSession>(client_id, client_fd);
    session->SetRemoteAddress(FormatAddress(client_addr));
    
    // Epoll에 추가
    struct epoll_event ev;
    ev.events = EPOLLIN | EPOLLET | EPOLLRDHUP;
    ev.data.fd = client_fd;
    
    if (epoll_ctl(epoll_fd_, EPOLL_CTL_ADD, client_fd, &ev) < 0) {
        std::cerr << "Failed to add client to epoll: " 
                  << strerror(errno) << std::endl;
        return;
    }
    
    // 클라이언트 맵에 추가
    {
        std::lock_guard<std::mutex> lock(clients_mutex_);
        clients_[client_id] = std::move(session);
        fd_to_client_id_[client_fd] = client_id;
    }
    
    stats_.active_connections++;
    stats_.total_connections++;
    
    std::cout << "Client connected: " << client_id 
              << " from " << session->GetRemoteAddress() << std::endl;
}

bool TcpServer::SetSocketNonBlocking(int fd) {
    int flags = fcntl(fd, F_GETFL, 0);
    if (flags < 0) {
        std::cerr << "fcntl F_GETFL failed: " << strerror(errno) << std::endl;
        return false;
    }
    
    flags |= O_NONBLOCK;
    if (fcntl(fd, F_SETFL, flags) < 0) {
        std::cerr << "fcntl F_SETFL failed: " << strerror(errno) << std::endl;
        return false;
    }
    
    return true;
}

bool TcpServer::SetSocketReuseAddr(int fd) {
    int opt = 1;
    if (setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt)) < 0) {
        std::cerr << "setsockopt SO_REUSEADDR failed: " 
                  << strerror(errno) << std::endl;
        return false;
    }
    return true;
}

bool TcpServer::SetTcpNoDelay(int fd) {
    int opt = 1;
    if (setsockopt(fd, IPPROTO_TCP, TCP_NODELAY, &opt, sizeof(opt)) < 0) {
        std::cerr << "setsockopt TCP_NODELAY failed: " 
                  << strerror(errno) << std::endl;
        return false;
    }
    return true;
}

std::string GetLastSocketError() {
    return strerror(errno);
}

std::string FormatAddress(const sockaddr_in& addr) {
    char buf[INET_ADDRSTRLEN];
    inet_ntop(AF_INET, &addr.sin_addr, buf, sizeof(buf));
    return std::string(buf) + ":" + std::to_string(ntohs(addr.sin_port));
}

} // namespace tcp
} // namespace oms

#endif // !_WIN32