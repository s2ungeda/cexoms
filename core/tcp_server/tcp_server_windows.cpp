#ifdef _WIN32

#include "tcp_server.h"
#include <mswsock.h>
#include <iostream>
#include <cassert>

#pragma comment(lib, "ws2_32.lib")
#pragma comment(lib, "mswsock.lib")

// IOCP operation types
enum class IOCPOpType {
    Accept,
    Recv,
    Send,
    Disconnect
};

// IOCP operation context
struct IOCPContext {
    OVERLAPPED overlapped;
    IOCPOpType op_type;
    SOCKET socket;
    std::shared_ptr<ClientSession> session;
    WSABUF wsabuf;
    char buffer[8192];
    
    IOCPContext() {
        memset(&overlapped, 0, sizeof(overlapped));
        memset(buffer, 0, sizeof(buffer));
        wsabuf.buf = buffer;
        wsabuf.len = sizeof(buffer);
    }
};

bool TcpServer::InitializeIOCP() {
    // Create IOCP handle
    iocp_ = CreateIoCompletionPort(INVALID_HANDLE_VALUE, nullptr, 0, io_threads_);
    if (!iocp_) {
        std::cerr << "Failed to create IOCP: " << GetLastError() << std::endl;
        return false;
    }
    
    // Associate listen socket with IOCP
    if (!CreateIoCompletionPort(reinterpret_cast<HANDLE>(listen_fd_), iocp_, 0, 0)) {
        std::cerr << "Failed to associate listen socket with IOCP: " << GetLastError() << std::endl;
        return false;
    }
    
    // Load AcceptEx function
    GUID guid_accept_ex = WSAID_ACCEPTEX;
    DWORD bytes_returned = 0;
    
    int result = WSAIoctl(
        listen_fd_,
        SIO_GET_EXTENSION_FUNCTION_POINTER,
        &guid_accept_ex,
        sizeof(guid_accept_ex),
        &accept_ex_,
        sizeof(accept_ex_),
        &bytes_returned,
        nullptr,
        nullptr
    );
    
    if (result == SOCKET_ERROR) {
        std::cerr << "Failed to get AcceptEx function: " << WSAGetLastError() << std::endl;
        return false;
    }
    
    // Post initial accept operations
    for (int i = 0; i < 10; ++i) {
        PostAccept();
    }
    
    return true;
}

void TcpServer::PostAccept() {
    // Create accept socket
    SOCKET accept_socket = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
    if (accept_socket == INVALID_SOCKET) {
        std::cerr << "Failed to create accept socket: " << WSAGetLastError() << std::endl;
        return;
    }
    
    // Create IOCP context
    auto* ctx = new IOCPContext();
    ctx->op_type = IOCPOpType::Accept;
    ctx->socket = accept_socket;
    
    // Post AcceptEx
    DWORD bytes_received = 0;
    BOOL result = accept_ex_(
        listen_fd_,
        accept_socket,
        ctx->buffer,
        0,  // No initial data
        sizeof(sockaddr_in) + 16,
        sizeof(sockaddr_in) + 16,
        &bytes_received,
        &ctx->overlapped
    );
    
    if (!result && WSAGetLastError() != ERROR_IO_PENDING) {
        std::cerr << "AcceptEx failed: " << WSAGetLastError() << std::endl;
        closesocket(accept_socket);
        delete ctx;
    }
}

void TcpServer::IOCPWorkerThread() {
    while (running_) {
        DWORD bytes_transferred = 0;
        ULONG_PTR completion_key = 0;
        LPOVERLAPPED overlapped = nullptr;
        
        // Wait for completion
        BOOL result = GetQueuedCompletionStatus(
            iocp_,
            &bytes_transferred,
            &completion_key,
            &overlapped,
            100  // 100ms timeout
        );
        
        if (!result) {
            if (!overlapped) {
                // Timeout or error
                continue;
            }
            
            // Operation failed
            auto* ctx = CONTAINING_RECORD(overlapped, IOCPContext, overlapped);
            HandleIOCPError(ctx, GetLastError());
            continue;
        }
        
        // Handle completed operation
        auto* ctx = CONTAINING_RECORD(overlapped, IOCPContext, overlapped);
        
        switch (ctx->op_type) {
            case IOCPOpType::Accept:
                HandleAcceptComplete(ctx, bytes_transferred);
                break;
                
            case IOCPOpType::Recv:
                HandleRecvComplete(ctx, bytes_transferred);
                break;
                
            case IOCPOpType::Send:
                HandleSendComplete(ctx, bytes_transferred);
                break;
                
            case IOCPOpType::Disconnect:
                HandleDisconnectComplete(ctx);
                break;
        }
    }
}

void TcpServer::HandleAcceptComplete(IOCPContext* ctx, DWORD bytes_transferred) {
    SOCKET client_socket = ctx->socket;
    
    // Update socket context
    if (setsockopt(client_socket, SOL_SOCKET, SO_UPDATE_ACCEPT_CONTEXT,
                   reinterpret_cast<const char*>(&listen_fd_), sizeof(listen_fd_)) == SOCKET_ERROR) {
        std::cerr << "Failed to update accept context: " << WSAGetLastError() << std::endl;
        closesocket(client_socket);
        delete ctx;
        PostAccept();  // Post new accept
        return;
    }
    
    // Get client address
    sockaddr_in client_addr;
    int addr_len = sizeof(client_addr);
    getpeername(client_socket, reinterpret_cast<sockaddr*>(&client_addr), &addr_len);
    
    // Create session
    auto session = std::make_shared<ClientSession>(
        static_cast<int>(client_socket),
        client_addr,
        recv_buffer_size_,
        send_buffer_size_
    );
    
    // Associate with IOCP
    if (!CreateIoCompletionPort(reinterpret_cast<HANDLE>(client_socket), iocp_,
                                reinterpret_cast<ULONG_PTR>(session.get()), 0)) {
        std::cerr << "Failed to associate client socket with IOCP: " << GetLastError() << std::endl;
        closesocket(client_socket);
        delete ctx;
        PostAccept();
        return;
    }
    
    // Add to client map
    {
        std::lock_guard<std::mutex> lock(clients_mutex_);
        clients_[static_cast<int>(client_socket)] = session;
    }
    
    stats_.total_connections++;
    stats_.active_connections++;
    
    std::cout << "Client connected from " << inet_ntoa(client_addr.sin_addr)
              << ":" << ntohs(client_addr.sin_port) << std::endl;
    
    // Post initial receive
    PostReceive(session);
    
    // Clean up and post new accept
    delete ctx;
    PostAccept();
}

void TcpServer::PostReceive(std::shared_ptr<ClientSession> session) {
    auto* ctx = new IOCPContext();
    ctx->op_type = IOCPOpType::Recv;
    ctx->socket = session->GetSocket();
    ctx->session = session;
    
    DWORD flags = 0;
    int result = WSARecv(
        session->GetSocket(),
        &ctx->wsabuf,
        1,
        nullptr,
        &flags,
        &ctx->overlapped,
        nullptr
    );
    
    if (result == SOCKET_ERROR && WSAGetLastError() != ERROR_IO_PENDING) {
        std::cerr << "WSARecv failed: " << WSAGetLastError() << std::endl;
        delete ctx;
        DisconnectClient(session->GetSocket());
    }
}

void TcpServer::HandleRecvComplete(IOCPContext* ctx, DWORD bytes_transferred) {
    auto session = ctx->session;
    
    if (bytes_transferred == 0) {
        // Connection closed
        delete ctx;
        DisconnectClient(session->GetSocket());
        return;
    }
    
    // Process received data
    session->AppendReceiveData(ctx->buffer, bytes_transferred);
    stats_.bytes_received += bytes_transferred;
    
    // Process messages
    while (true) {
        auto message = session->ExtractMessage();
        if (!message.first) {
            break;
        }
        
        stats_.messages_received++;
        
        // Queue message for processing
        Message msg;
        msg.client_fd = session->GetSocket();
        msg.data = std::move(message.second);
        msg.timestamp = std::chrono::steady_clock::now();
        
        incoming_messages_->enqueue(std::move(msg));
    }
    
    // Post next receive
    delete ctx;
    PostReceive(session);
}

void TcpServer::PostSend(std::shared_ptr<ClientSession> session, const std::vector<uint8_t>& data) {
    auto* ctx = new IOCPContext();
    ctx->op_type = IOCPOpType::Send;
    ctx->socket = session->GetSocket();
    ctx->session = session;
    
    // Copy data to context buffer
    size_t copy_size = std::min(data.size(), sizeof(ctx->buffer));
    memcpy(ctx->buffer, data.data(), copy_size);
    ctx->wsabuf.len = static_cast<ULONG>(copy_size);
    
    int result = WSASend(
        session->GetSocket(),
        &ctx->wsabuf,
        1,
        nullptr,
        0,
        &ctx->overlapped,
        nullptr
    );
    
    if (result == SOCKET_ERROR && WSAGetLastError() != ERROR_IO_PENDING) {
        std::cerr << "WSASend failed: " << WSAGetLastError() << std::endl;
        delete ctx;
        DisconnectClient(session->GetSocket());
    }
}

void TcpServer::HandleSendComplete(IOCPContext* ctx, DWORD bytes_transferred) {
    stats_.bytes_sent += bytes_transferred;
    stats_.messages_sent++;
    
    // Check if there's more data to send
    auto session = ctx->session;
    if (session && !session->GetSendQueue().empty()) {
        // Send next message
        // Note: In production, implement proper send queue management
    }
    
    delete ctx;
}

void TcpServer::HandleDisconnectComplete(IOCPContext* ctx) {
    // Clean up
    delete ctx;
}

void TcpServer::HandleIOCPError(IOCPContext* ctx, DWORD error) {
    std::cerr << "IOCP operation failed: " << error << std::endl;
    stats_.errors++;
    
    if (ctx->session) {
        DisconnectClient(ctx->session->GetSocket());
    }
    
    delete ctx;
}

void TcpServer::DisconnectClient(int client_fd) {
    std::shared_ptr<ClientSession> session;
    
    {
        std::lock_guard<std::mutex> lock(clients_mutex_);
        auto it = clients_.find(client_fd);
        if (it != clients_.end()) {
            session = it->second;
            clients_.erase(it);
            stats_.active_connections--;
        }
    }
    
    if (session) {
        // Post disconnect operation
        auto* ctx = new IOCPContext();
        ctx->op_type = IOCPOpType::Disconnect;
        ctx->socket = client_fd;
        ctx->session = session;
        
        // Use DisconnectEx if available, otherwise just close
        closesocket(client_fd);
        delete ctx;
        
        std::cout << "Client disconnected: " << client_fd << std::endl;
    }
}

bool TcpServer::SendToClient(int client_fd, const std::vector<uint8_t>& data) {
    std::shared_ptr<ClientSession> session;
    
    {
        std::lock_guard<std::mutex> lock(clients_mutex_);
        auto it = clients_.find(client_fd);
        if (it != clients_.end()) {
            session = it->second;
        }
    }
    
    if (!session) {
        return false;
    }
    
    // Post send operation
    PostSend(session, data);
    return true;
}

#endif // _WIN32