#include "message_handler.h"
#include <iostream>
#include <sstream>
#include <chrono>

namespace oms {
namespace tcp {

MessageHandler::MessageHandler() {
    InitializeCallbacks();
}

void MessageHandler::InitializeCallbacks() {
    // 메시지 타입별 핸들러 등록
    callbacks_[MessageType::LOGIN_REQUEST] = 
        [this](ClientSession* s, const Message& m) {
            HandleLoginRequest(s, m.login_request());
        };
    
    callbacks_[MessageType::SUBSCRIBE_REQUEST] = 
        [this](ClientSession* s, const Message& m) {
            HandleSubscribeRequest(s, m.subscribe_request());
        };
    
    callbacks_[MessageType::ORDER_REQUEST] = 
        [this](ClientSession* s, const Message& m) {
            HandleOrderRequest(s, m.order_request());
        };
}

void MessageHandler::HandleMessage(ClientSession* session, const Message& msg) {
    auto it = callbacks_.find(msg.type());
    if (it != callbacks_.end()) {
        it->second(session, msg);
    } else {
        SendError(session, 1002, "Unknown message type");
    }
}

void MessageHandler::HandleLoginRequest(ClientSession* session, 
                                      const LoginRequest& req) {
    // 기본 구현 - 항상 성공
    std::cout << "Login request from " << session->GetId() 
              << " with API key: " << req.api_key() << std::endl;
    
    // 인증 성공 가정
    session->SetAuthenticated(req.api_key(), 
        static_cast<uint32_t>(ClientPermission::READ) | 
        static_cast<uint32_t>(ClientPermission::TRADE));
    
    auto response = BuildLoginResponse(true, 
        "session_" + session->GetId(), 
        "Login successful");
    
    SendResponse(session, response);
}

void MessageHandler::HandleSubscribeRequest(ClientSession* session,
                                          const SubscribeRequest& req) {
    if (!session->IsAuthenticated()) {
        SendError(session, 1001, "Not authenticated");
        return;
    }
    
    std::map<std::string, bool> results;
    
    for (const auto& symbol : req.symbols()) {
        bool success = true;
        for (const auto& channel : req.channels()) {
            ChannelType ch;
            if (channel == "ticker") ch = ChannelType::TICKER;
            else if (channel == "orderbook") ch = ChannelType::ORDERBOOK;
            else if (channel == "trades") ch = ChannelType::TRADES;
            else {
                success = false;
                break;
            }
            
            session->Subscribe(symbol, ch);
        }
        results[symbol] = success;
    }
    
    auto response = BuildSubscribeResponse(results);
    SendResponse(session, response);
}

void MessageHandler::SendResponse(ClientSession* session, const Message& response) {
    size_t msg_size = response.ByteSize();
    std::vector<uint8_t> buffer(4 + msg_size);
    
    // Length header
    buffer[0] = (msg_size >> 24) & 0xFF;
    buffer[1] = (msg_size >> 16) & 0xFF;
    buffer[2] = (msg_size >> 8) & 0xFF;
    buffer[3] = msg_size & 0xFF;
    
    // Message body
    response.SerializeToArray(buffer.data() + 4, msg_size);
    
    session->AddToSendBuffer(buffer.data(), buffer.size());
}

void MessageHandler::SendError(ClientSession* session, int32_t code, 
                             const std::string& message) {
    auto error_msg = BuildErrorMessage(code, message);
    SendResponse(session, error_msg);
}

Message MessageHandler::BuildLoginResponse(bool success, const std::string& token,
                                         const std::string& message) {
    Message msg;
    msg.set_type(MessageType::LOGIN_RESPONSE);
    
    auto* resp = msg.mutable_login_response();
    resp->set_success(success);
    resp->set_session_token(token);
    resp->set_message(message);
    
    if (success) {
        resp->add_permissions("read");
        resp->add_permissions("trade");
    }
    
    return msg;
}

Message MessageHandler::BuildSubscribeResponse(const std::map<std::string, bool>& results) {
    Message msg;
    msg.set_type(MessageType::SUBSCRIBE_RESPONSE);
    
    auto* resp = msg.mutable_subscribe_response();
    for (const auto& [symbol, success] : results) {
        (*resp->mutable_results())[symbol] = success;
    }
    
    return msg;
}

Message MessageHandler::BuildErrorMessage(int32_t code, const std::string& message) {
    Message msg;
    msg.set_type(MessageType::ERROR);
    
    auto* error = msg.mutable_error();
    error->set_code(code);
    error->set_message(message);
    
    return msg;
}

} // namespace tcp
} // namespace oms