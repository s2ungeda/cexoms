#include "tcp_server.h"
#include <cstring>
#include <unistd.h>

namespace oms {
namespace tcp {

// ProcessClientMessage - 클라이언트로부터 받은 메시지 처리
void TcpServer::ProcessClientMessage(ClientSession* session, const Message& msg) {
    // 통계 업데이트
    stats_.messages_received++;
    
    // 메시지 핸들러로 전달
    if (message_handler_) {
        message_handler_->HandleMessage(session, msg);
    } else {
        std::cerr << "No message handler set!" << std::endl;
    }
    
    // 특정 메시지 타입에 대한 추가 처리
    switch (msg.type()) {
        case MessageType::SUBSCRIBE_REQUEST:
            // 구독 정보를 브로드캐스터에 업데이트
            if (msg.has_subscribe_request()) {
                const auto& req = msg.subscribe_request();
                for (const auto& symbol : req.symbols()) {
                    for (const auto& channel : req.channels()) {
                        ChannelType ch_type;
                        if (channel == "ticker") ch_type = ChannelType::TICKER;
                        else if (channel == "orderbook") ch_type = ChannelType::ORDERBOOK;
                        else if (channel == "trades") ch_type = ChannelType::TRADES;
                        else continue;
                        
                        broadcaster_->UpdateSubscriptions(
                            session->GetId(), symbol, ch_type, true);
                    }
                }
            }
            break;
            
        case MessageType::UNSUBSCRIBE_REQUEST:
            // 구독 해제
            if (msg.has_unsubscribe_request()) {
                const auto& req = msg.unsubscribe_request();
                for (const auto& symbol : req.symbols()) {
                    for (const auto& channel : req.channels()) {
                        ChannelType ch_type;
                        if (channel == "ticker") ch_type = ChannelType::TICKER;
                        else if (channel == "orderbook") ch_type = ChannelType::ORDERBOOK;
                        else if (channel == "trades") ch_type = ChannelType::TRADES;
                        else continue;
                        
                        broadcaster_->UpdateSubscriptions(
                            session->GetId(), symbol, ch_type, false);
                    }
                }
            }
            break;
            
        case MessageType::HEARTBEAT_RESPONSE:
            // 클라이언트로부터 하트비트 응답 받음
            session->UpdateLastActivity();
            break;
    }
}

// SendToClient wrapper for raw data
bool TcpServer::SendToClient(const std::string& client_id, const Message& msg) {
    size_t msg_size = msg.ByteSize();
    std::vector<uint8_t> buffer(4 + msg_size);
    
    // Length header (Big Endian)
    buffer[0] = (msg_size >> 24) & 0xFF;
    buffer[1] = (msg_size >> 16) & 0xFF;
    buffer[2] = (msg_size >> 8) & 0xFF;
    buffer[3] = msg_size & 0xFF;
    
    // Message body
    msg.SerializeToArray(buffer.data() + 4, msg_size);
    
    return SendToClient(client_id, buffer.data(), buffer.size());
}

// Windows 플랫폼용 스텁 (현재는 Linux만 지원)
#ifdef _WIN32

bool TcpServer::InitializeIOCP() {
    std::cerr << "Windows IOCP not implemented yet" << std::endl;
    return false;
}

void TcpServer::IOCPWorkerThread(int thread_id) {
    // Not implemented
}

#endif // _WIN32

} // namespace tcp
} // namespace oms