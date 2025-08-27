# mExOms API 문서

다중 거래소 주문 관리 시스템(mExOms)은 여러 암호화폐 거래소에서 고빈도 거래를 위한 포괄적인 gRPC 및 REST API를 제공합니다.

## 개요

mExOms는 6개의 핵심 서비스를 제공합니다:

- **[주문 서비스(OrderService)](./order-service.md)** - 주문 관리 및 실행
- **[포지션 서비스(PositionService)](./position-service.md)** - 포지션 추적 및 위험 지표  
- **[시장 데이터 서비스(MarketDataService)](./market-data-service.md)** - 실시간 및 과거 시장 데이터
- **[인증 서비스(AuthService)](./auth-service.md)** - 인증 및 API 키 관리
- **[계정 서비스(AccountService)](./account-service.md)** - 다중 계정 운영 및 이체
- **[전략 서비스(StrategyService)](./strategy-service.md)** - 전략 배포 및 관리

## 빠른 시작

### 인증

모든 API 호출은 JWT 토큰 또는 API 키를 통한 인증이 필요합니다:

```bash
# JWT 토큰 획득
grpcurl -plaintext -d '{"username":"trader", "password":"secret"}' \
  localhost:8080 oms.v1.AuthService/Authenticate

# 후속 호출에서 토큰 사용
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  localhost:8080 oms.v1.OrderService/ListOrders
```

### 기본 주문 플로우

```bash
# 1. 주문 생성
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{"order": {"symbol": "BTCUSDT", "side": "BUY", "quantity": "1.0", "price": "50000"}}' \
  localhost:8080 oms.v1.OrderService/CreateOrder

# 2. 주문 상태 확인
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{"order_id": "order-123"}' \
  localhost:8080 oms.v1.OrderService/GetOrder

# 3. 필요시 주문 취소
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{"order_id": "order-123"}' \
  localhost:8080 oms.v1.OrderService/CancelOrder
```

## 프로토콜 사양

- **gRPC 프로토콜**: protobuf 직렬화를 사용하는 HTTP/2
- **TLS**: 프로덕션 환경에서 TLS 1.3 필수
- **인증**: JWT 토큰 또는 API 키
- **속도 제한**: 계정별 제한 적용
- **오류 처리**: 표준 gRPC 상태 코드

## 성능 특성

- **지연 시간**: <100μs 주문 처리
- **처리량**: 초당 100,000개+ 주문
- **가용성**: 99.9%+ 가동 시간
- **데이터 신선도**: 실시간 WebSocket 스트림

## 오류 코드

일반적인 gRPC 상태 코드:

| 코드 | 상태 | 설명 |
|------|------|------|
| 0 | OK | 성공 |
| 3 | INVALID_ARGUMENT | 잘못된 요청 매개변수 |
| 7 | PERMISSION_DENIED | 인증 필요 |
| 8 | RESOURCE_EXHAUSTED | 속도 제한 초과 |
| 14 | UNAVAILABLE | 서비스 일시적으로 사용 불가 |

## SDK 지원

공식 SDK 제공:

- Go: `github.com/mExOms/go-client`
- Python: `pip install mexoms-client`
- JavaScript: `npm install @mexoms/client`
- Java: 문서에서 Maven 좌표 확인

## 환경별 URL

| 환경 | gRPC | REST 게이트웨이 |
|------|------|-----------------|
| 개발 | localhost:8080 | http://localhost:8081 |
| 스테이징 | grpc.staging.mexoms.com:443 | https://api.staging.mexoms.com |
| 프로덕션 | grpc.mexoms.com:443 | https://api.mexoms.com |

## 다음 단계

1. API 액세스를 위한 [인증 가이드](./auth-service.md) 검토
2. 거래 작업을 위한 [주문 서비스](./order-service.md) 탐색
3. 실시간 데이터를 위한 [시장 데이터 서비스](./market-data-service.md) 확인
4. 실제 구현을 위한 [예제](../examples/) 참조

지원 문의: api-support@mexoms.com