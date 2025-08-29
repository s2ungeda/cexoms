#pragma once

#include <memory>
#include <functional>
#include <unordered_map>
#include "client_session.h"

// Protocol Buffers 생성 헤더 (나중에 생성될 예정)
#include "proto/tcp/v1/messages.pb.h"

namespace oms {
namespace tcp {

// 메시지 타입별 핸들러
using MessageCallback = std::function<void(ClientSession*, const Message&)>;

class MessageHandler {
public:
    MessageHandler() = default;
    virtual ~MessageHandler() = default;

    // 메시지 처리 메인 함수
    virtual void HandleMessage(ClientSession* session, const Message& msg);

    // 인증
    virtual void HandleLoginRequest(ClientSession* session, const LoginRequest& req);
    virtual void HandleLogoutRequest(ClientSession* session, const LogoutRequest& req);

    // 마켓 데이터 구독
    virtual void HandleSubscribeRequest(ClientSession* session, const SubscribeRequest& req);
    virtual void HandleUnsubscribeRequest(ClientSession* session, const UnsubscribeRequest& req);

    // 계좌 정보
    virtual void HandleBalanceRequest(ClientSession* session, const BalanceRequest& req);
    virtual void HandlePositionRequest(ClientSession* session, const PositionRequest& req);

    // 주문 처리
    virtual void HandleOrderRequest(ClientSession* session, const OrderRequest& req);
    virtual void HandleCancelOrderRequest(ClientSession* session, const CancelOrderRequest& req);

    // 시스템
    virtual void HandleHeartbeatRequest(ClientSession* session, const HeartbeatRequest& req);

    // 응답 전송 헬퍼
    void SendResponse(ClientSession* session, const Message& response);
    void SendError(ClientSession* session, int32_t code, const std::string& message);

protected:
    // 권한 체크
    bool CheckPermission(ClientSession* session, ClientPermission required);
    
    // 유효성 검증
    bool ValidateSymbol(const std::string& symbol);
    bool ValidateOrderRequest(const OrderRequest& req);
    
    // 메시지 빌더
    Message BuildLoginResponse(bool success, const std::string& token, 
                             const std::string& message);
    Message BuildSubscribeResponse(const std::map<std::string, bool>& results);
    Message BuildOrderResponse(bool success, const std::string& order_id, 
                             const std::string& message);
    Message BuildErrorMessage(int32_t code, const std::string& message);

private:
    // 콜백 매핑
    std::unordered_map<MessageType, MessageCallback> callbacks_;
    
    // 초기화
    void InitializeCallbacks();
};

// 실제 구현을 위한 OMS 통합 핸들러
class OmsMessageHandler : public MessageHandler {
public:
    OmsMessageHandler();
    ~OmsMessageHandler() override;

    // OMS 서비스 연동
    void SetOrderService(std::shared_ptr<void> order_service);
    void SetAccountService(std::shared_ptr<void> account_service);
    void SetMarketDataService(std::shared_ptr<void> market_data_service);

    // 오버라이드된 핸들러들
    void HandleLoginRequest(ClientSession* session, const LoginRequest& req) override;
    void HandleOrderRequest(ClientSession* session, const OrderRequest& req) override;
    void HandleBalanceRequest(ClientSession* session, const BalanceRequest& req) override;
    void HandlePositionRequest(ClientSession* session, const PositionRequest& req) override;

private:
    // OMS 서비스들 (void*로 선언하여 순환 의존성 방지)
    std::shared_ptr<void> order_service_;
    std::shared_ptr<void> account_service_;
    std::shared_ptr<void> market_data_service_;
    
    // API 키 검증
    bool VerifyApiKey(const std::string& api_key, const std::string& secret);
    
    // 세션 토큰 생성
    std::string GenerateSessionToken();
};

} // namespace tcp
} // namespace oms