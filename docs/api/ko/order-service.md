# 주문 서비스(OrderService) API 참조

주문 서비스는 주문 생성, 취소, 상태 조회를 포함한 모든 주문 관련 작업을 처리합니다.

## 개요

- **서비스**: `oms.v1.OrderService`
- **기본 URL**: `grpc://localhost:8080` (개발환경)
- **인증**: 필수 (JWT 토큰 또는 API 키)
- **속도 제한**: 계정당 분당 1000건 요청

## 메서드

### CreateOrder (주문 생성)

실행을 위한 새 주문을 생성합니다.

**요청**: `OrderRequest`
```protobuf
message OrderRequest {
  Order order = 1;
  string account_id = 2;  // 선택사항: 특정 계정
  string strategy_id = 3; // 선택사항: 전략 연결
}

message Order {
  string symbol = 1;        // 거래쌍 (예: "BTCUSDT")
  OrderSide side = 2;       // BUY 또는 SELL
  OrderType type = 3;       // MARKET, LIMIT, STOP_LOSS 등
  string quantity = 4;      // 주문 수량 (십진수 문자열)
  string price = 5;         // 가격 (지정가 주문용)
  string stop_price = 6;    // 정지 가격 (정지 주문용)
  TimeInForce time_in_force = 7; // GTC, IOC, FOK
  bool reduce_only = 8;     // 선물 전용: 포지션 감소
  bool post_only = 9;       // 메이커 전용 주문
}
```

**응답**: `OrderResponse`
```protobuf
message OrderResponse {
  Order order = 1;
  string order_id = 2;
  OrderStatus status = 3;
  string exchange = 4;
  string account_id = 5;
  int64 created_at = 6;
  string error_message = 7;
}
```

**예제**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "order": {
      "symbol": "BTCUSDT",
      "side": "BUY",
      "type": "LIMIT", 
      "quantity": "0.01",
      "price": "45000",
      "time_in_force": "GTC"
    },
    "account_id": "main-account"
  }' \
  localhost:8080 oms.v1.OrderService/CreateOrder
```

**성능**: 
- 지연시간: <100μs
- 성공률: >99.9%

### CancelOrder (주문 취소)

기존 주문을 취소합니다.

**요청**: `CancelOrderRequest`
```protobuf
message CancelOrderRequest {
  string order_id = 1;      // 필수: 취소할 주문 ID
  string symbol = 2;        // 선택사항: 검증용
  string account_id = 3;    // 선택사항: 특정 계정
}
```

**응답**: `OrderResponse`

**예제**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{"order_id": "order-123456"}' \
  localhost:8080 oms.v1.OrderService/CancelOrder
```

### GetOrder (주문 조회)

특정 주문의 세부 정보를 조회합니다.

**요청**: `GetOrderRequest`
```protobuf
message GetOrderRequest {
  string order_id = 1;      // 필수: 주문 ID
  string account_id = 2;    // 선택사항: 특정 계정
}
```

**응답**: `OrderResponse`

**예제**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{"order_id": "order-123456"}' \
  localhost:8080 oms.v1.OrderService/GetOrder
```

### ListOrders (주문 목록 조회)

선택적 필터링으로 주문 목록을 조회합니다.

**요청**: `ListOrdersRequest`
```protobuf
message ListOrdersRequest {
  string account_id = 1;    // 선택사항: 계정별 필터
  string symbol = 2;        // 선택사항: 심볼별 필터
  OrderStatus status = 3;   // 선택사항: 상태별 필터
  int64 start_time = 4;     // 선택사항: 시작 타임스탬프
  int64 end_time = 5;       // 선택사항: 종료 타임스탬프
  int32 limit = 6;          // 선택사항: 최대 결과 수 (기본값 100)
  string page_token = 7;    // 선택사항: 페이지네이션 토큰
}
```

**응답**: `ListOrdersResponse`
```protobuf
message ListOrdersResponse {
  repeated OrderResponse orders = 1;
  string next_page_token = 2;
  int32 total_count = 3;
}
```

**예제**:
```bash
# 활성 BTCUSDT 주문 모두 조회
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "symbol": "BTCUSDT",
    "status": "PENDING",
    "limit": 50
  }' \
  localhost:8080 oms.v1.OrderService/ListOrders
```

## 주문 유형

| 유형 | 설명 | 필수 필드 |
|------|------|-----------|
| MARKET | 시장가로 즉시 실행 | symbol, side, quantity |
| LIMIT | 특정 가격 또는 더 나은 가격으로 실행 | symbol, side, quantity, price |
| STOP_LOSS | 정지 가격으로 촉발되는 시장가 주문 | symbol, side, quantity, stop_price |
| STOP_LIMIT | 정지 가격으로 촉발되는 지정가 주문 | symbol, side, quantity, price, stop_price |
| TAKE_PROFIT | 수익 실현 주문 | symbol, side, quantity, stop_price |

## 주문 상태 흐름

```
NEW → PENDING → PARTIALLY_FILLED → FILLED
                    ↓
               CANCELLED/REJECTED/EXPIRED
```

| 상태 | 설명 |
|------|------|
| NEW | 주문 생성되었지만 거래소로 전송되지 않음 |
| PENDING | 주문이 거래소로 전송됨, 체결 대기 중 |
| PARTIALLY_FILLED | 주문이 부분적으로 체결됨 |
| FILLED | 주문이 완전히 체결됨 |
| CANCELLED | 사용자가 주문을 취소함 |
| REJECTED | 거래소에서 주문을 거부함 |
| EXPIRED | 주문이 만료됨 (시간 기반 주문) |

## 오류 처리

일반적인 오류 시나리오:

### 잔고 부족
```json
{
  "code": 3,
  "message": "INVALID_ARGUMENT: 주문을 위한 잔고가 부족합니다",
  "details": [{
    "type": "InsufficientBalanceError",
    "available": "100.00",
    "required": "150.00",
    "asset": "USDT"
  }]
}
```

### 잘못된 심볼
```json
{
  "code": 3,
  "message": "INVALID_ARGUMENT: 알 수 없는 심볼 INVALID",
  "details": [{
    "type": "InvalidSymbolError",
    "supported_symbols": ["BTCUSDT", "ETHUSDT", "..."]
  }]
}
```

### 속도 제한 초과
```json
{
  "code": 8,
  "message": "RESOURCE_EXHAUSTED: 속도 제한 초과",
  "details": [{
    "type": "RateLimitError",
    "retry_after": 60,
    "limit": 1000,
    "remaining": 0
  }]
}
```

## 모범 사례

### 1. 주문 크기 검증
```go
// 거래소 제한에 대해 주문 크기를 항상 검증
if quantity < minQuantity || quantity > maxQuantity {
    return errors.New("수량이 범위를 벗어남")
}
```

### 2. 가격 정밀도
```go
// 가격을 거래소 정밀도로 반올림
price = roundToPrecision(price, symbolInfo.PricePrecision)
```

### 3. 위험 관리
```go
// 주문 생성 전 포지션 한도 확인
if newPosition > maxPosition {
    return errors.New("포지션 한도 초과")
}
```

### 4. 오류 처리
```go
// 재시도 가능한 오류 처리
if isRetryable(err) && retries < maxRetries {
    time.Sleep(exponentialBackoff(retries))
    return retry(request)
}
```

## WebSocket 업데이트

MarketDataService를 통해 실시간 주문 업데이트를 구독:

```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{"subscription_type": "ORDER_UPDATES", "account_id": "main"}' \
  localhost:8080 oms.v1.MarketDataService/Subscribe
```

주문 업데이트 포함 내용:
- 상태 변경
- 부분 체결
- 최종 실행 세부사항
- 취소 확인

## 지원 거래소

| 거래소 | 현물 | 선물 | 옵션 |
|--------|------|------|------|
| 바이낸스 | ✅ | ✅ | ❌ |
| 바이비트 | ⏳ | ⏳ | ❌ |
| OKX | ⏳ | ⏳ | ❌ |
| 업비트 | ⏳ | ❌ | ❌ |

## 관련 서비스

- [포지션 서비스](./position-service.md) - 결과 포지션 모니터링
- [계정 서비스](./account-service.md) - 계정 잔고 확인
- [시장 데이터 서비스](./market-data-service.md) - 실시간 주문 업데이트