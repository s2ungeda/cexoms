#include <iostream>
#include <vector>
#include <thread>
#include <atomic>
#include <chrono>
#include <cstring>
#include <random>
#include <iomanip>

#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <unistd.h>

// TCP 벤치마크 클라이언트
class BenchmarkClient {
public:
    struct Stats {
        std::atomic<uint64_t> messages_sent{0};
        std::atomic<uint64_t> messages_received{0};
        std::atomic<uint64_t> bytes_sent{0};
        std::atomic<uint64_t> bytes_received{0};
        std::atomic<uint64_t> errors{0};
        std::atomic<uint64_t> total_latency_us{0};
        std::atomic<uint64_t> latency_samples{0};
    };

private:
    std::string host_;
    int port_;
    int num_connections_;
    int messages_per_second_;
    int message_size_;
    int duration_seconds_;
    
    std::atomic<bool> running_{false};
    Stats stats_;
    std::vector<std::thread> client_threads_;
    
public:
    BenchmarkClient(const std::string& host, int port, int connections, 
                   int msg_per_sec, int msg_size, int duration)
        : host_(host), port_(port), num_connections_(connections),
          messages_per_second_(msg_per_sec), message_size_(msg_size),
          duration_seconds_(duration) {}
    
    void Run() {
        std::cout << "=== TCP Server Benchmark ===" << std::endl;
        std::cout << "Host: " << host_ << ":" << port_ << std::endl;
        std::cout << "Connections: " << num_connections_ << std::endl;
        std::cout << "Messages/sec: " << messages_per_second_ << std::endl;
        std::cout << "Message size: " << message_size_ << " bytes" << std::endl;
        std::cout << "Duration: " << duration_seconds_ << " seconds" << std::endl;
        std::cout << std::endl;
        
        running_ = true;
        auto start_time = std::chrono::steady_clock::now();
        
        // 각 연결마다 스레드 생성
        for (int i = 0; i < num_connections_; ++i) {
            client_threads_.emplace_back(&BenchmarkClient::ClientThread, this, i);
        }
        
        // 진행 상황 모니터링
        MonitorProgress(start_time);
        
        // 종료
        running_ = false;
        for (auto& thread : client_threads_) {
            if (thread.joinable()) {
                thread.join();
            }
        }
        
        // 최종 결과 출력
        PrintFinalStats();
    }
    
private:
    void ClientThread(int client_id) {
        // 서버 연결
        int sock = socket(AF_INET, SOCK_STREAM, 0);
        if (sock < 0) {
            std::cerr << "Client " << client_id << ": Failed to create socket" << std::endl;
            stats_.errors++;
            return;
        }
        
        sockaddr_in server_addr{};
        server_addr.sin_family = AF_INET;
        server_addr.sin_port = htons(port_);
        inet_pton(AF_INET, host_.c_str(), &server_addr.sin_addr);
        
        if (connect(sock, (sockaddr*)&server_addr, sizeof(server_addr)) < 0) {
            std::cerr << "Client " << client_id << ": Connection failed" << std::endl;
            close(sock);
            stats_.errors++;
            return;
        }
        
        // 메시지 버퍼 준비
        std::vector<uint8_t> message(4 + message_size_);
        std::fill(message.begin() + 4, message.end(), 'X');
        
        // 길이 헤더 설정
        uint32_t len = message_size_;
        message[0] = (len >> 24) & 0xFF;
        message[1] = (len >> 16) & 0xFF;
        message[2] = (len >> 8) & 0xFF;
        message[3] = len & 0xFF;
        
        // 수신 스레드 시작
        std::thread recv_thread(&BenchmarkClient::ReceiveThread, this, sock);
        
        // 메시지 전송 루프
        auto msg_interval = std::chrono::microseconds(1000000 / (messages_per_second_ / num_connections_));
        auto next_send_time = std::chrono::steady_clock::now();
        
        while (running_) {
            auto now = std::chrono::steady_clock::now();
            
            if (now >= next_send_time) {
                // 타임스탬프 추가 (레이턴시 측정용)
                auto timestamp = now.time_since_epoch().count();
                std::memcpy(message.data() + 4, &timestamp, sizeof(timestamp));
                
                // 전송
                ssize_t n = send(sock, message.data(), message.size(), MSG_NOSIGNAL);
                if (n > 0) {
                    stats_.messages_sent++;
                    stats_.bytes_sent += n;
                } else {
                    stats_.errors++;
                }
                
                next_send_time += msg_interval;
            }
            
            // CPU 사용률 감소를 위한 짧은 대기
            std::this_thread::sleep_for(std::chrono::microseconds(10));
        }
        
        // 정리
        shutdown(sock, SHUT_RDWR);
        close(sock);
        
        if (recv_thread.joinable()) {
            recv_thread.join();
        }
    }
    
    void ReceiveThread(int sock) {
        std::vector<uint8_t> buffer(65536);
        std::vector<uint8_t> recv_buffer;
        
        while (running_) {
            ssize_t n = recv(sock, buffer.data(), buffer.size(), 0);
            
            if (n > 0) {
                stats_.bytes_received += n;
                recv_buffer.insert(recv_buffer.end(), buffer.begin(), buffer.begin() + n);
                
                // 메시지 추출
                while (recv_buffer.size() >= 4) {
                    uint32_t msg_len = (recv_buffer[0] << 24) |
                                      (recv_buffer[1] << 16) |
                                      (recv_buffer[2] << 8) |
                                      recv_buffer[3];
                    
                    if (recv_buffer.size() < 4 + msg_len) {
                        break;
                    }
                    
                    // 레이턴시 계산
                    if (msg_len >= sizeof(int64_t)) {
                        int64_t sent_time;
                        std::memcpy(&sent_time, recv_buffer.data() + 4, sizeof(sent_time));
                        
                        auto now = std::chrono::steady_clock::now().time_since_epoch().count();
                        auto latency_ns = now - sent_time;
                        auto latency_us = latency_ns / 1000;
                        
                        stats_.total_latency_us += latency_us;
                        stats_.latency_samples++;
                    }
                    
                    stats_.messages_received++;
                    
                    // 처리된 메시지 제거
                    recv_buffer.erase(recv_buffer.begin(), recv_buffer.begin() + 4 + msg_len);
                }
            } else if (n == 0) {
                break; // 연결 종료
            } else {
                if (errno != EAGAIN && errno != EWOULDBLOCK) {
                    break;
                }
            }
        }
    }
    
    void MonitorProgress(std::chrono::steady_clock::time_point start_time) {
        auto last_stats = stats_;
        auto last_time = start_time;
        
        while (running_) {
            std::this_thread::sleep_for(std::chrono::seconds(1));
            
            auto now = std::chrono::steady_clock::now();
            auto elapsed = std::chrono::duration_cast<std::chrono::seconds>(
                now - start_time).count();
            
            if (elapsed >= duration_seconds_) {
                break;
            }
            
            // 현재 통계
            auto current_stats = stats_;
            auto interval = std::chrono::duration_cast<std::chrono::milliseconds>(
                now - last_time).count() / 1000.0;
            
            // 초당 처리량 계산
            auto msg_per_sec = (current_stats.messages_sent - last_stats.messages_sent) / interval;
            auto mb_per_sec = (current_stats.bytes_sent - last_stats.bytes_sent) / interval / 1024 / 1024;
            
            // 평균 레이턴시
            uint64_t avg_latency_us = 0;
            if (current_stats.latency_samples > last_stats.latency_samples) {
                avg_latency_us = (current_stats.total_latency_us - last_stats.total_latency_us) /
                               (current_stats.latency_samples - last_stats.latency_samples);
            }
            
            std::cout << "[" << std::setw(3) << elapsed << "s] "
                     << "Sent: " << std::setw(8) << current_stats.messages_sent.load()
                     << " | Recv: " << std::setw(8) << current_stats.messages_received.load()
                     << " | Rate: " << std::setw(8) << std::fixed << std::setprecision(0) 
                     << msg_per_sec << " msg/s"
                     << " | " << std::setw(6) << std::setprecision(2) << mb_per_sec << " MB/s"
                     << " | Latency: " << std::setw(6) << avg_latency_us << " μs"
                     << " | Errors: " << current_stats.errors.load()
                     << std::endl;
            
            last_stats = current_stats;
            last_time = now;
        }
    }
    
    void PrintFinalStats() {
        std::cout << "\n=== Final Results ===" << std::endl;
        std::cout << "Total messages sent: " << stats_.messages_sent.load() << std::endl;
        std::cout << "Total messages received: " << stats_.messages_received.load() << std::endl;
        std::cout << "Total bytes sent: " << stats_.bytes_sent.load() / 1024 / 1024 << " MB" << std::endl;
        std::cout << "Total bytes received: " << stats_.bytes_received.load() / 1024 / 1024 << " MB" << std::endl;
        std::cout << "Total errors: " << stats_.errors.load() << std::endl;
        
        if (stats_.latency_samples > 0) {
            auto avg_latency = stats_.total_latency_us.load() / stats_.latency_samples.load();
            std::cout << "Average latency: " << avg_latency << " μs" << std::endl;
        }
        
        auto total_ops = stats_.messages_sent.load() + stats_.messages_received.load();
        auto ops_per_sec = total_ops / duration_seconds_;
        std::cout << "Operations/sec: " << ops_per_sec << std::endl;
    }
};

// 메인 함수
int main(int argc, char* argv[]) {
    // 기본값
    std::string host = "localhost";
    int port = 9090;
    int connections = 10;
    int msg_per_sec = 10000;
    int msg_size = 256;
    int duration = 30;
    
    // 명령줄 인자 파싱
    if (argc > 1) host = argv[1];
    if (argc > 2) port = std::stoi(argv[2]);
    if (argc > 3) connections = std::stoi(argv[3]);
    if (argc > 4) msg_per_sec = std::stoi(argv[4]);
    if (argc > 5) msg_size = std::stoi(argv[5]);
    if (argc > 6) duration = std::stoi(argv[6]);
    
    // 사용법
    if (argc == 2 && std::string(argv[1]) == "--help") {
        std::cout << "Usage: " << argv[0] 
                  << " [host] [port] [connections] [msg/sec] [msg_size] [duration]" 
                  << std::endl;
        std::cout << "Defaults: localhost 9090 10 10000 256 30" << std::endl;
        return 0;
    }
    
    // 벤치마크 실행
    BenchmarkClient benchmark(host, port, connections, msg_per_sec, msg_size, duration);
    benchmark.Run();
    
    return 0;
}