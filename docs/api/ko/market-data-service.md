# 시장 데이터 서비스(MarketDataService) API 참조

시장 데이터 서비스는 다수의 암호화폐 거래소로부터 실시간 및 과거 시장 데이터를 제공합니다.

## 개요

- **서비스**: `oms.v1.MarketDataService`
- **기본 URL**: `grpc://localhost:8080` (개발환경)
- **인증**: 일부 엔드포인트에서 필요
- **데이터 소스**: 실시간 거래소 WebSocket 피드
- **업데이트 빈도**: 실시간 (sub-second 지연시간)

## 메서드

### GetOrderBook (호가창 조회)

심볼에 대한 현재 호가창(매수/매도 주문)을 조회합니다.

**요청**: `GetOrderBookRequest`
```protobuf
message GetOrderBookRequest {
  string symbol = 1;         // 거래쌍 (예: "BTCUSDT")
  string exchange = 2;       // 선택사항: 특정 거래소
  int32 depth = 3;          // 선택사항: 레벨 수 (기본값 20)
}
```

**응답**: `OrderBook`
```protobuf
message OrderBook {
  string symbol = 1;
  string exchange = 2;
  repeated OrderBookLevel bids = 3;   // 매수 주문
  repeated OrderBookLevel asks = 4;   // 매도 주문
  int64 timestamp = 5;               // 마지막 업데이트 시간
  int64 last_update_id = 6;          // 시퀀스 ID
}

message OrderBookLevel {
  string price = 1;         // 가격 레벨
  string quantity = 2;      // 해당 레벨의 총 수량
  int32 count = 3;          // 주문 수 (선택사항)
}
```

**예제**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "symbol": "BTCUSDT",
    "exchange": "binance",
    "depth": 10
  }' \
  localhost:8080 oms.v1.MarketDataService/GetOrderBook
```

**응답**:
```json
{
  "symbol": "BTCUSDT",
  "exchange": "binance",
  "bids": [
    {"price": "45000.00", "quantity": "1.5"},
    {"price": "44999.50", "quantity": "2.1"},
    {"price": "44999.00", "quantity": "0.8"}
  ],
  "asks": [
    {"price": "45000.50", "quantity": "1.2"},
    {"price": "45001.00", "quantity": "1.8"},
    {"price": "45001.50", "quantity": "0.9"}
  ],
  "timestamp": 1703894400000,
  "last_update_id": 12345678
}
```

### GetTicker (24시간 티커 통계 조회)

24시간 티커 통계를 조회합니다.

**요청**: `GetTickerRequest`
```protobuf
message GetTickerRequest {
  string symbol = 1;         // 거래쌍
  string exchange = 2;       // 선택사항: 특정 거래소
}
```

**응답**: `Ticker`
```protobuf
message Ticker {
  string symbol = 1;
  string exchange = 2;
  string last_price = 3;     // 마지막 거래 가격
  string bid_price = 4;      // 최고 매수 가격
  string ask_price = 5;      // 최저 매도 가격
  string high_price = 6;     // 24시간 최고가
  string low_price = 7;      // 24시간 최저가
  string volume = 8;         // 24시간 거래량
  string quote_volume = 9;   // 24시간 호가 거래량
  string price_change = 10;  // 24시간 가격 변화
  string price_change_percent = 11; // 24시간 변화율 %
  int64 timestamp = 12;      // 업데이트 타임스탬프
  int32 count = 13;          // 24시간 거래 횟수
}
```

**예제**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{"symbol": "BTCUSDT", "exchange": "binance"}' \
  localhost:8080 oms.v1.MarketDataService/GetTicker
```

### GetRecentTrades (최근 거래 내역 조회)

최근 거래 내역을 조회합니다.

**요청**: `GetRecentTradesRequest`
```protobuf
message GetRecentTradesRequest {
  string symbol = 1;         // 거래쌍
  string exchange = 2;       // 선택사항: 특정 거래소
  int32 limit = 3;          // 선택사항: 최대 거래 수 (기본값 100)
}
```

**응답**: `GetRecentTradesResponse`
```protobuf
message GetRecentTradesResponse {
  repeated Trade trades = 1;
}

message Trade {
  string id = 1;            // 거래 ID
  string price = 2;         // 거래 가격
  string quantity = 3;      // 거래 수량
  bool is_buyer_maker = 4;  // 매수자가 메이커인지 여부
  int64 timestamp = 5;      // 거래 타임스탬프
}
```

**예제**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "symbol": "BTCUSDT",
    "exchange": "binance",
    "limit": 50
  }' \
  localhost:8080 oms.v1.MarketDataService/GetRecentTrades
```

### GetKlines (K선 데이터 조회)

과거 캔들스틱 데이터를 조회합니다.

**요청**: `GetKlinesRequest`
```protobuf
message GetKlinesRequest {
  string symbol = 1;         // 거래쌍
  string exchange = 2;       // 선택사항: 특정 거래소
  string interval = 3;       // 시간프레임 (1m, 5m, 1h, 1d 등)
  int64 start_time = 4;      // 선택사항: 시작 타임스탬프
  int64 end_time = 5;        // 선택사항: 종료 타임스탬프
  int32 limit = 6;          // 선택사항: 최대 K선 수 (기본값 500)
}
```

**응답**: `GetKlinesResponse`
```protobuf
message GetKlinesResponse {
  repeated Kline klines = 1;
}

message Kline {
  int64 open_time = 1;       // 캔들 시작 시간
  string open = 2;           // 시가
  string high = 3;           // 고가
  string low = 4;            // 저가
  string close = 5;          // 종가
  string volume = 6;         // 거래량
  int64 close_time = 7;      // 캔들 종료 시간
  string quote_volume = 8;   // 호가 자산 거래량
  int32 trades = 9;          // 거래 횟수
  string taker_buy_volume = 10;      // 테이커 매수 거래량
  string taker_buy_quote_volume = 11; // 테이커 매수 호가 거래량
}
```

**예제**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "symbol": "BTCUSDT",
    "exchange": "binance",
    "interval": "1h",
    "limit": 24
  }' \
  localhost:8080 oms.v1.MarketDataService/GetKlines
```

### Subscribe (스트리밍)

실시간 시장 데이터 업데이트를 구독합니다.

**요청**: `SubscribeRequest`
```protobuf
message SubscribeRequest {
  SubscriptionType subscription_type = 1;
  string symbol = 2;         // 선택사항: 특정 심볼
  string exchange = 3;       // 선택사항: 특정 거래소
  string account_id = 4;     // 선택사항: 계정별 데이터용
  repeated string channels = 5; // 구독할 특정 채널
}

enum SubscriptionType {
  TICKER = 0;               // 티커 업데이트
  ORDER_BOOK = 1;           // 호가창 변경
  TRADES = 2;               // 거래 스트림
  KLINES = 3;               // 캔들스틱 업데이트
  ORDER_UPDATES = 4;        // 주문 상태 변경
  POSITION_UPDATES = 5;     // 포지션 변경
  BALANCE_UPDATES = 6;      // 잔고 변경
}
```

**응답 스트림**: `MarketDataUpdate`
```protobuf
message MarketDataUpdate {
  string type = 1;          // 업데이트 유형
  string symbol = 2;        // 심볼
  string exchange = 3;      // 거래소
  int64 timestamp = 4;      // 업데이트 타임스탬프
  
  oneof data {
    Ticker ticker = 5;
    OrderBook order_book = 6;
    Trade trade = 7;
    Kline kline = 8;
    OrderUpdate order_update = 9;
    PositionUpdate position_update = 10;
    BalanceUpdate balance_update = 11;
  }
}
```

**예제**:
```bash
# BTCUSDT 티커 업데이트 구독
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "subscription_type": "TICKER",
    "symbol": "BTCUSDT",
    "exchange": "binance"
  }' \
  localhost:8080 oms.v1.MarketDataService/Subscribe
```

**예제 스트림 응답**:
```json
{
  "type": "ticker",
  "symbol": "BTCUSDT",
  "exchange": "binance",
  "timestamp": 1703894400000,
  "ticker": {
    "symbol": "BTCUSDT",
    "last_price": "45123.45",
    "bid_price": "45123.00",
    "ask_price": "45123.90",
    "volume": "1234.56",
    "price_change_percent": "2.34"
  }
}
```

## 지원 거래소

| 거래소 | 현물 | 선물 | WebSocket | 과거 데이터 |
|--------|------|------|-----------|-------------|
| 바이낸스 | ✅ | ✅ | ✅ | ✅ |
| 바이비트 | ⏳ | ⏳ | ⏳ | ⏳ |
| OKX | ⏳ | ⏳ | ⏳ | ⏳ |
| 업비트 | ⏳ | ❌ | ⏳ | ⏳ |

## 데이터 품질

### 실시간 데이터
- **소스**: 직접 거래소 WebSocket 피드
- **지연시간**: 거래소로부터 <50ms
- **가동시간**: 99.9%+
- **데이터 무결성**: 체크섬 및 시퀀스 검증

### 과거 데이터
- **저장소**: 로컬 시계열 데이터베이스
- **보관 기간**: 
  - 틱: 30일
  - 1분 캔들: 2년
  - 1시간 캔들: 5년
  - 일간 캔들: 영구
- **백필**: 자동 갭 감지 및 보완

## 속도 제한

| 엔드포인트 | 분당 요청 | 비고 |
|-----------|-----------|------|
| GetOrderBook | 1000 | 심볼당 |
| GetTicker | 2000 | 모든 심볼 |
| GetRecentTrades | 1000 | 심볼당 |
| GetKlines | 500 | 요청당 |
| Subscribe | 연결 10개 | 계정당 |

## WebSocket 채널

### 공개 채널 (인증 불필요)
```bash
# 모든 심볼의 티커
channels: ["ticker@all"]

# 특정 심볼의 호가창
channels: ["orderbook@BTCUSDT@binance"]

# 특정 심볼의 거래
channels: ["trades@BTCUSDT@binance"]

# 특정 인터벌의 K선
channels: ["klines@BTCUSDT@binance@1h"]
```

### 비공개 채널 (인증 필요)
```bash
# 계정별 주문 업데이트
channels: ["orders@account-123"]

# 포지션 업데이트
channels: ["positions@account-123"]

# 잔고 업데이트
channels: ["balances@account-123"]
```

## 데이터 정규화

모든 데이터는 거래소 간에 정규화됩니다:

### 심볼 형식
- 표준 형식: `{BASE}{QUOTE}` (예: `BTCUSDT`)
- 원본 거래소 심볼은 메타데이터에 보존

### 가격/수량 정밀도
```protobuf
message SymbolInfo {
  string symbol = 1;
  string exchange = 2;
  int32 price_precision = 3;    // 가격의 소수점 자리수
  int32 quantity_precision = 4; // 수량의 소수점 자리수
  string min_quantity = 5;      // 최소 주문 크기
  string min_notional = 6;      // 최소 주문 가치
  string tick_size = 7;         // 가격 증분
  string step_size = 8;         // 수량 증분
}
```

### 타임스탬프 형식
- 모든 타임스탬프는 Unix 밀리초 (UTC)
- 모든 데이터 유형에서 일관됨

## 고급 기능

### 거래소 간 차익거래 데이터
```bash
# 차익거래 기회 조회
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "subscription_type": "ARBITRAGE",
    "symbol": "BTCUSDT",
    "min_profit_bps": 50
  }' \
  localhost:8080 oms.v1.MarketDataService/Subscribe
```

### 시장 깊이 분석
```bash
# 깊은 호가창 조회 (1000 레벨)
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "symbol": "BTCUSDT",
    "exchange": "binance",
    "depth": 1000
  }' \
  localhost:8080 oms.v1.MarketDataService/GetOrderBook
```

### 시간별 거래 내역
```bash
# 시간 범위의 모든 거래 조회
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "symbol": "BTCUSDT",
    "start_time": 1703894400000,
    "end_time": 1703898000000
  }' \
  localhost:8080 oms.v1.MarketDataService/GetTimeAndSales
```

## 오류 처리

### 일반적인 오류

#### 심볼을 찾을 수 없음
```json
{
  "code": 3,
  "message": "INVALID_ARGUMENT: 심볼 INVALID가 지원되지 않습니다",
  "details": [{
    "type": "UnsupportedSymbolError",
    "symbol": "INVALID",
    "supported_symbols": ["BTCUSDT", "ETHUSDT", "..."]
  }]
}
```

#### 거래소 사용 불가
```json
{
  "code": 14,
  "message": "UNAVAILABLE: 거래소 binance가 일시적으로 사용 불가능합니다",
  "details": [{
    "type": "ExchangeUnavailableError",
    "exchange": "binance",
    "retry_after": 30,
    "alternative_exchanges": ["bybit", "okx"]
  }]
}
```

### 데이터 갭 처리
```go
func handleKlineGaps(klines []Kline) {
    for i := 1; i < len(klines); i++ {
        expectedTime := klines[i-1].CloseTime + 1
        if klines[i].OpenTime > expectedTime {
            // 갭 감지, 누락된 데이터 요청
            missingKlines := requestMissingData(
                klines[i-1].CloseTime,
                klines[i].OpenTime,
            )
            klines = mergeSorted(klines, missingKlines)
        }
    }
}
```

## 성능 최적화

### 효율적인 구독
```go
// 여러 심볼을 효율적으로 구독
req := &SubscribeRequest{
    SubscriptionType: SubscriptionType_TICKER,
    Channels: []string{
        "ticker@BTCUSDT@binance",
        "ticker@ETHUSDT@binance",
        "ticker@ADAUSDT@binance",
    },
}
```

### 배치 요청
```go
// 여러 티커를 한 번에 요청
symbols := []string{"BTCUSDT", "ETHUSDT", "ADAUSDT"}
tickers := make([]*Ticker, len(symbols))

var wg sync.WaitGroup
for i, symbol := range symbols {
    wg.Add(1)
    go func(i int, symbol string) {
        defer wg.Done()
        ticker, err := client.GetTicker(ctx, &GetTickerRequest{
            Symbol: symbol,
        })
        if err == nil {
            tickers[i] = ticker
        }
    }(i, symbol)
}
wg.Wait()
```

## 통합 예제

### React 실시간 대시보드
```typescript
// 시장 데이터 스트림에 WebSocket 연결
const ws = new WebSocket('wss://api.mexoms.com/v1/stream');

ws.onmessage = (event) => {
  const update: MarketDataUpdate = JSON.parse(event.data);
  
  switch(update.type) {
    case 'ticker':
      updateTickerDisplay(update.ticker);
      break;
    case 'orderbook':
      updateOrderBookDisplay(update.order_book);
      break;
    case 'trade':
      addTradeToHistory(update.trade);
      break;
  }
};
```

### Python 거래 봇
```python
import grpc
from proto.oms.v1 import market_data_service_pb2_grpc as md_grpc
from proto.oms.v1 import market_data_service_pb2 as md_pb2

class MarketDataClient:
    def __init__(self, channel):
        self.stub = md_grpc.MarketDataServiceStub(channel)
    
    def get_ticker(self, symbol, exchange="binance"):
        request = md_pb2.GetTickerRequest(
            symbol=symbol,
            exchange=exchange
        )
        return self.stub.GetTicker(request)
    
    def subscribe_tickers(self, symbols):
        request = md_pb2.SubscribeRequest(
            subscription_type=md_pb2.TICKER,
            channels=[f"ticker@{sym}@binance" for sym in symbols]
        )
        
        for update in self.stub.Subscribe(request):
            yield update
```

## 관련 서비스

- [주문 서비스](./order-service.md) - 시장 데이터 기반 주문 실행
- [포지션 서비스](./position-service.md) - 손익 변화 모니터링
- [전략 서비스](./strategy-service.md) - 알고리즘 거래 전략