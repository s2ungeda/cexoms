#include <iostream>
#include <string>
#include <vector>
#include <thread>
#include <atomic>
#include <chrono>
#include <cstring>

#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <unistd.h>

// 간단한 TCP 클라이언트 예제
class SimpleTcpClient {
public:
    SimpleTcpClient(const std::string& host, int port) 
        : host_(host), port_(port), socket_fd_(-1) {
    }
    
    ~SimpleTcpClient() {
        Disconnect();
    }
    
    bool Connect() {
        socket_fd_ = socket(AF_INET, SOCK_STREAM, 0);
        if (socket_fd_ < 0) {
            std::cerr << "Failed to create socket" << std::endl;
            return false;
        }
        
        sockaddr_in server_addr{};
        server_addr.sin_family = AF_INET;
        server_addr.sin_port = htons(port_);
        
        if (inet_pton(AF_INET, host_.c_str(), &server_addr.sin_addr) <= 0) {
            std::cerr << "Invalid address: " << host_ << std::endl;
            close(socket_fd_);
            socket_fd_ = -1;
            return false;
        }
        
        if (connect(socket_fd_, (sockaddr*)&server_addr, sizeof(server_addr)) < 0) {
            std::cerr << "Connection failed" << std::endl;
            close(socket_fd_);
            socket_fd_ = -1;
            return false;
        }
        
        std::cout << "Connected to " << host_ << ":" << port_ << std::endl;
        connected_ = true;
        
        // 수신 스레드 시작
        receive_thread_ = std::thread(&SimpleTcpClient::ReceiveThread, this);
        
        return true;
    }
    
    void Disconnect() {
        connected_ = false;
        
        if (socket_fd_ >= 0) {
            shutdown(socket_fd_, SHUT_RDWR);
            close(socket_fd_);
            socket_fd_ = -1;
        }
        
        if (receive_thread_.joinable()) {
            receive_thread_.join();
        }
    }
    
    bool SendMessage(const std::vector<uint8_t>& message) {
        if (!connected_ || socket_fd_ < 0) {
            return false;
        }
        
        // 메시지 길이 헤더 추가 (4 bytes, Big Endian)
        std::vector<uint8_t> frame;
        frame.reserve(4 + message.size());
        
        uint32_t len = message.size();
        frame.push_back((len >> 24) & 0xFF);
        frame.push_back((len >> 16) & 0xFF);
        frame.push_back((len >> 8) & 0xFF);
        frame.push_back(len & 0xFF);
        
        frame.insert(frame.end(), message.begin(), message.end());
        
        // 전송
        size_t total_sent = 0;
        while (total_sent < frame.size()) {
            ssize_t n = send(socket_fd_, 
                           frame.data() + total_sent, 
                           frame.size() - total_sent, 
                           MSG_NOSIGNAL);
            
            if (n < 0) {
                std::cerr << "Send failed" << std::endl;
                return false;
            }
            
            total_sent += n;
        }
        
        messages_sent_++;
        return true;
    }
    
    // 테스트용 메시지 생성
    void SendTestLogin(const std::string& api_key, const std::string& secret) {
        // 실제 구현에서는 Protocol Buffers 사용
        std::string msg = "LOGIN:" + api_key + ":" + secret;
        std::vector<uint8_t> data(msg.begin(), msg.end());
        
        if (SendMessage(data)) {
            std::cout << "Login request sent" << std::endl;
        }
    }
    
    void SendTestSubscribe(const std::string& symbol) {
        std::string msg = "SUBSCRIBE:" + symbol;
        std::vector<uint8_t> data(msg.begin(), msg.end());
        
        if (SendMessage(data)) {
            std::cout << "Subscribe request sent for " << symbol << std::endl;
        }
    }
    
    void SendTestOrder(const std::string& symbol, const std::string& side, 
                      double quantity, double price) {
        std::string msg = "ORDER:" + symbol + ":" + side + ":" + 
                         std::to_string(quantity) + ":" + std::to_string(price);
        std::vector<uint8_t> data(msg.begin(), msg.end());
        
        if (SendMessage(data)) {
            std::cout << "Order request sent" << std::endl;
        }
    }
    
    uint64_t GetMessagesSent() const { return messages_sent_; }
    uint64_t GetMessagesReceived() const { return messages_received_; }
    
private:
    void ReceiveThread() {
        std::vector<uint8_t> buffer(65536);
        std::vector<uint8_t> recv_buffer;
        
        while (connected_) {
            ssize_t n = recv(socket_fd_, buffer.data(), buffer.size(), 0);
            
            if (n > 0) {
                recv_buffer.insert(recv_buffer.end(), 
                                 buffer.begin(), 
                                 buffer.begin() + n);
                
                // 메시지 추출
                while (recv_buffer.size() >= 4) {
                    uint32_t msg_len = (recv_buffer[0] << 24) |
                                      (recv_buffer[1] << 16) |
                                      (recv_buffer[2] << 8) |
                                      recv_buffer[3];
                    
                    if (recv_buffer.size() < 4 + msg_len) {
                        break; // 완전한 메시지 아님
                    }
                    
                    // 메시지 처리
                    std::vector<uint8_t> message(
                        recv_buffer.begin() + 4, 
                        recv_buffer.begin() + 4 + msg_len);
                    
                    ProcessMessage(message);
                    
                    // 처리된 메시지 제거
                    recv_buffer.erase(recv_buffer.begin(), 
                                    recv_buffer.begin() + 4 + msg_len);
                    
                    messages_received_++;
                }
            } else if (n == 0) {
                std::cout << "Server disconnected" << std::endl;
                break;
            } else {
                if (errno != EAGAIN && errno != EWOULDBLOCK) {
                    std::cerr << "Receive error" << std::endl;
                    break;
                }
            }
            
            std::this_thread::sleep_for(std::chrono::milliseconds(1));
        }
    }
    
    void ProcessMessage(const std::vector<uint8_t>& message) {
        // 테스트용 - 실제로는 Protocol Buffers 파싱
        std::string msg(message.begin(), message.end());
        std::cout << "Received: " << msg << std::endl;
    }
    
private:
    std::string host_;
    int port_;
    int socket_fd_;
    std::atomic<bool> connected_{false};
    std::thread receive_thread_;
    
    std::atomic<uint64_t> messages_sent_{0};
    std::atomic<uint64_t> messages_received_{0};
};

// 사용 예제
int main(int argc, char* argv[]) {
    std::string host = "localhost";
    int port = 9090;
    
    if (argc > 1) host = argv[1];
    if (argc > 2) port = std::stoi(argv[2]);
    
    SimpleTcpClient client(host, port);
    
    if (!client.Connect()) {
        return 1;
    }
    
    // 테스트 시나리오
    std::cout << "\n=== Test Scenario ===" << std::endl;
    
    // 1. 로그인
    client.SendTestLogin("test_api_key", "test_secret");
    std::this_thread::sleep_for(std::chrono::seconds(1));
    
    // 2. 시세 구독
    client.SendTestSubscribe("BTC/USDT");
    client.SendTestSubscribe("ETH/USDT");
    std::this_thread::sleep_for(std::chrono::seconds(1));
    
    // 3. 주문 전송
    client.SendTestOrder("BTC/USDT", "BUY", 0.001, 65000.0);
    std::this_thread::sleep_for(std::chrono::seconds(1));
    
    // 4. 일정 시간 대기하며 메시지 수신
    std::cout << "\nWaiting for messages... (Press Ctrl+C to stop)" << std::endl;
    
    for (int i = 0; i < 30; ++i) {
        std::this_thread::sleep_for(std::chrono::seconds(1));
        
        if (i % 10 == 0) {
            std::cout << "Stats - Sent: " << client.GetMessagesSent() 
                     << ", Received: " << client.GetMessagesReceived() 
                     << std::endl;
        }
    }
    
    client.Disconnect();
    
    std::cout << "\nFinal Stats:" << std::endl;
    std::cout << "Messages sent: " << client.GetMessagesSent() << std::endl;
    std::cout << "Messages received: " << client.GetMessagesReceived() << std::endl;
    
    return 0;
}