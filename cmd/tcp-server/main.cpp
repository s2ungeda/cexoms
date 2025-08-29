#include <iostream>
#include <signal.h>
#include <atomic>
#include <chrono>
#include <thread>
#include "core/tcp_server/tcp_server.h"

std::atomic<bool> g_running{true};

// Signal handler
void signal_handler(int sig) {
    std::cout << "\nReceived signal " << sig << ", shutting down..." << std::endl;
    g_running = false;
}

void print_usage(const char* program) {
    std::cout << "Usage: " << program << " [port] [options]" << std::endl;
    std::cout << "Options:" << std::endl;
    std::cout << "  --worker-threads N    Number of worker threads (default: CPU cores)" << std::endl;
    std::cout << "  --io-threads N        Number of I/O threads (default: 2)" << std::endl;
    std::cout << "  --max-clients N       Maximum number of clients (default: 1000)" << std::endl;
    std::cout << "  --buffer-size N       Client buffer size in KB (default: 64)" << std::endl;
    std::cout << "  --help                Show this help message" << std::endl;
}

int main(int argc, char* argv[]) {
    // Default configuration
    ServerConfig config;
    config.port = 9090;
    config.worker_threads = std::thread::hardware_concurrency();
    config.io_threads = 2;
    config.max_clients = 1000;
    config.recv_buffer_size = 65536;
    config.send_buffer_size = 65536;
    
    // Parse command line arguments
    if (argc > 1) {
        try {
            config.port = std::stoi(argv[1]);
        } catch (...) {
            if (std::string(argv[1]) == "--help") {
                print_usage(argv[0]);
                return 0;
            }
            std::cerr << "Invalid port number: " << argv[1] << std::endl;
            return 1;
        }
    }
    
    // Parse additional options
    for (int i = 2; i < argc; i += 2) {
        if (i + 1 >= argc) break;
        
        std::string opt = argv[i];
        std::string val = argv[i + 1];
        
        try {
            if (opt == "--worker-threads") {
                config.worker_threads = std::stoi(val);
            } else if (opt == "--io-threads") {
                config.io_threads = std::stoi(val);
            } else if (opt == "--max-clients") {
                config.max_clients = std::stoi(val);
            } else if (opt == "--buffer-size") {
                config.recv_buffer_size = std::stoi(val) * 1024;
                config.send_buffer_size = config.recv_buffer_size;
            } else {
                std::cerr << "Unknown option: " << opt << std::endl;
            }
        } catch (...) {
            std::cerr << "Invalid value for " << opt << ": " << val << std::endl;
        }
    }
    
    // Setup signal handlers
    signal(SIGINT, signal_handler);
    signal(SIGTERM, signal_handler);
#ifndef _WIN32
    signal(SIGPIPE, SIG_IGN);  // Ignore broken pipe
#endif
    
    // Print startup information
    std::cout << "=== mExOms TCP Server ===" << std::endl;
    std::cout << "Port: " << config.port << std::endl;
    std::cout << "Worker threads: " << config.worker_threads << std::endl;
    std::cout << "I/O threads: " << config.io_threads << std::endl;
    std::cout << "Max clients: " << config.max_clients << std::endl;
    std::cout << "Buffer size: " << config.recv_buffer_size / 1024 << " KB" << std::endl;
    std::cout << std::endl;
    
    // Create and start server
    TcpServer server(config);
    
    if (!server.Start()) {
        std::cerr << "Failed to start server" << std::endl;
        return 1;
    }
    
    std::cout << "Server started successfully" << std::endl;
    
    // Main loop - print statistics
    auto last_stats_time = std::chrono::steady_clock::now();
    ServerStats last_stats = server.GetStats();
    
    while (g_running) {
        std::this_thread::sleep_for(std::chrono::seconds(5));
        
        auto now = std::chrono::steady_clock::now();
        auto elapsed = std::chrono::duration_cast<std::chrono::seconds>(
            now - last_stats_time).count();
        
        if (elapsed > 0) {
            ServerStats stats = server.GetStats();
            
            // Calculate rates
            uint64_t msg_rate = (stats.messages_received - last_stats.messages_received) / elapsed;
            uint64_t bytes_rate = (stats.bytes_received - last_stats.bytes_received) / elapsed;
            
            std::cout << "\r[Stats] Clients: " << stats.active_connections
                     << " | Msg/s: " << msg_rate
                     << " | MB/s: " << (bytes_rate / 1024 / 1024)
                     << " | Total Msg: " << stats.messages_received
                     << " | Errors: " << stats.errors
                     << "        " << std::flush;
            
            last_stats = stats;
            last_stats_time = now;
        }
    }
    
    // Shutdown
    std::cout << "\nShutting down server..." << std::endl;
    server.Stop();
    
    // Print final statistics
    ServerStats final_stats = server.GetStats();
    std::cout << "\n=== Final Statistics ===" << std::endl;
    std::cout << "Total connections: " << final_stats.total_connections << std::endl;
    std::cout << "Total messages received: " << final_stats.messages_received << std::endl;
    std::cout << "Total messages sent: " << final_stats.messages_sent << std::endl;
    std::cout << "Total bytes received: " << final_stats.bytes_received / 1024 / 1024 << " MB" << std::endl;
    std::cout << "Total bytes sent: " << final_stats.bytes_sent / 1024 / 1024 << " MB" << std::endl;
    std::cout << "Total errors: " << final_stats.errors << std::endl;
    
    return 0;
}