# 포지션 서비스(PositionService) API 참조

포지션 서비스는 다중 거래소에서의 포지션 추적, 집계, 위험 지표를 제공합니다.

## 개요

- **서비스**: `oms.v1.PositionService`
- **기본 URL**: `grpc://localhost:8080` (개발환경)
- **인증**: 필수 (JWT 토큰 또는 API 키)
- **데이터 업데이트**: 실시간 (100ms 주기)

## 메서드

### GetPosition (포지션 조회)

특정 포지션의 세부 정보를 조회합니다.

**요청**: `GetPositionRequest`
```protobuf
message GetPositionRequest {
  string symbol = 1;         // 거래쌍 (예: "BTCUSDT")
  string exchange = 2;       // 선택사항: 특정 거래소
  string account_id = 3;     // 선택사항: 특정 계정
}
```

**응답**: `GetPositionResponse`
```protobuf
message GetPositionResponse {
  Position position = 1;
  PositionMetrics metrics = 2;
}

message Position {
  string symbol = 1;
  string exchange = 2;
  string account_id = 3;
  string side = 4;           // LONG, SHORT, NEUTRAL
  string size = 5;           // 포지션 크기
  string entry_price = 6;    // 평균 진입 가격
  string mark_price = 7;     // 표시 가격
  string unrealized_pnl = 8; // 미실현 손익
  string realized_pnl = 9;   // 실현 손익
  string margin = 10;        // 필요 증거금
  string leverage = 11;      // 레버리지
  int64 opened_at = 12;      // 포지션 시작 시간
  int64 updated_at = 13;     // 마지막 업데이트 시간
}

message PositionMetrics {
  string total_pnl = 1;      // 총 손익
  string pnl_percentage = 2; // 손익률
  string drawdown = 3;       // 최대 낙폭
  string win_rate = 4;       // 승률
  int32 trade_count = 5;     // 거래 횟수
}
```

**예제**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "symbol": "BTCUSDT",
    "exchange": "binance",
    "account_id": "main-account"
  }' \
  localhost:8080 oms.v1.PositionService/GetPosition
```

### ListPositions (포지션 목록)

계정의 모든 활성 포지션을 나열합니다.

**요청**: `ListPositionsRequest`
```protobuf
message ListPositionsRequest {
  string account_id = 1;     // 선택사항: 특정 계정
  string exchange = 2;       // 선택사항: 특정 거래소
  string symbol = 3;         // 선택사항: 특정 심볼
  bool include_zero = 4;     // 선택사항: 제로 포지션 포함
  int32 limit = 5;          // 선택사항: 최대 결과 수
  string page_token = 6;     // 선택사항: 페이지네이션 토큰
}
```

**응답**: `ListPositionsResponse`
```protobuf
message ListPositionsResponse {
  repeated Position positions = 1;
  PositionSummary summary = 2;
  string next_page_token = 3;
  int32 total_count = 4;
}

message PositionSummary {
  string total_unrealized_pnl = 1;  // 총 미실현 손익
  string total_realized_pnl = 2;    // 총 실현 손익
  string total_margin = 3;          // 총 필요 증거금
  string account_balance = 4;       // 계정 잔고
  string available_margin = 5;      // 사용 가능한 증거금
  int32 active_positions = 6;       // 활성 포지션 수
  string margin_ratio = 7;          // 증거금 비율
}
```

**예제**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "account_id": "main-account",
    "include_zero": false,
    "limit": 50
  }' \
  localhost:8080 oms.v1.PositionService/ListPositions
```

### GetAggregatedPositions (집계 포지션)

여러 거래소에서의 포지션을 집계합니다.

**요청**: `GetAggregatedPositionsRequest`
```protobuf
message GetAggregatedPositionsRequest {
  string symbol = 1;                    // 선택사항: 특정 심볼
  repeated string account_ids = 2;      // 선택사항: 특정 계정들
  repeated string exchanges = 3;        // 선택사항: 특정 거래소들
  AggregationType aggregation = 4;      // 집계 유형
}

enum AggregationType {
  BY_SYMBOL = 0;     // 심볼별 집계
  BY_STRATEGY = 1;   // 전략별 집계
  BY_ACCOUNT = 2;    // 계정별 집계
  BY_EXCHANGE = 3;   // 거래소별 집계
}
```

**응답**: `GetAggregatedPositionsResponse`
```protobuf
message GetAggregatedPositionsResponse {
  repeated AggregatedPosition positions = 1;
  AggregatedSummary summary = 2;
}

message AggregatedPosition {
  string key = 1;                   // 집계 키 (심볼, 전략, 계정 등)
  string net_size = 2;              // 순 포지션 크기
  string weighted_entry_price = 3;   // 가중 평균 진입 가격
  string total_unrealized_pnl = 4;   // 총 미실현 손익
  string total_realized_pnl = 5;     // 총 실현 손익
  repeated Position sub_positions = 6; // 구성 포지션들
  PositionRisk risk_metrics = 7;     // 위험 지표
}

message PositionRisk {
  string var_1d = 1;        // 1일 VaR (Value at Risk)
  string exposure = 2;      // 총 노출도
  string correlation = 3;   // 포지션 간 상관관계
  string concentration = 4; // 집중도 위험
}
```

**예제**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "symbol": "BTCUSDT",
    "aggregation": "BY_EXCHANGE"
  }' \
  localhost:8080 oms.v1.PositionService/GetAggregatedPositions
```

### GetRiskMetrics (위험 지표)

포지션 관련 위험 지표를 조회합니다.

**요청**: `GetRiskMetricsRequest`
```protobuf
message GetRiskMetricsRequest {
  string account_id = 1;     // 선택사항: 특정 계정
  string portfolio_id = 2;   // 선택사항: 특정 포트폴리오
  repeated string symbols = 3; // 선택사항: 특정 심볼들
}
```

**응답**: `GetRiskMetricsResponse`
```protobuf
message GetRiskMetricsResponse {
  RiskMetrics metrics = 1;
  repeated PositionRisk position_risks = 2;
  PortfolioRisk portfolio_risk = 3;
}

message RiskMetrics {
  // 포지션 위험
  string total_exposure = 1;     // 총 노출도
  string net_exposure = 2;       // 순 노출도
  string gross_exposure = 3;     // 총 노출도
  
  // 손익 위험
  string unrealized_pnl = 4;     // 미실현 손익
  string daily_pnl = 5;          // 일일 손익
  string max_drawdown = 6;       // 최대 낙폭
  
  // 증거금 위험
  string margin_usage = 7;       // 증거금 사용률
  string margin_available = 8;   // 사용 가능한 증거금
  string liquidation_risk = 9;   // 청산 위험도
  
  // 집중도 위험
  string symbol_concentration = 10;    // 심볼 집중도
  string exchange_concentration = 11;  // 거래소 집중도
  string strategy_concentration = 12;  // 전략 집중도
}

message PortfolioRisk {
  string sharpe_ratio = 1;       // 샤프 비율
  string sortino_ratio = 2;      // 소르티노 비율
  string max_drawdown = 3;       // 최대 낙폭
  string var_95 = 4;             // 95% VaR
  string cvar_95 = 5;            // 95% CVaR
  string beta = 6;               // 베타값
  string alpha = 7;              // 알파값
}
```

**예제**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "account_id": "main-account",
    "symbols": ["BTCUSDT", "ETHUSDT"]
  }' \
  localhost:8080 oms.v1.PositionService/GetRiskMetrics
```

## 포지션 유형

### 현물 거래
- **LONG**: 자산 보유 (매수 포지션)
- **NEUTRAL**: 포지션 없음 (잔고 0)

### 선물 거래
- **LONG**: 롱 포지션 (상승 베팅)
- **SHORT**: 숏 포지션 (하락 베팅)
- **NEUTRAL**: 포지션 없음

## 위험 지표 설명

### Value at Risk (VaR)
```
VaR_95% = 95% 신뢰구간에서 예상 최대 손실
```

### Sharpe Ratio
```
Sharpe Ratio = (평균 수익 - 무위험 수익률) / 수익의 표준편차
```

### Maximum Drawdown
```
Max Drawdown = (고점 - 저점) / 고점 × 100%
```

### Correlation
```
Correlation = 포지션 간 상관계수 (-1 ~ +1)
```

## 실시간 포지션 업데이트

MarketDataService를 통해 포지션 업데이트 구독:

```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "subscription_type": "POSITION_UPDATES",
    "account_id": "main-account"
  }' \
  localhost:8080 oms.v1.MarketDataService/Subscribe
```

업데이트 포함 내용:
- 포지션 크기 변경
- 손익 업데이트
- 증거금 변경
- 청산 가격 변경

## 포지션 관리 기능

### 헤지 감지
시스템이 자동으로 헤지 포지션을 감지합니다:

```go
// 예제: BTC 롱/숏 헤지 감지
if btcLongSize > 0 && btcShortSize > 0 {
    hedgeRatio := min(btcLongSize, btcShortSize) / max(btcLongSize, btcShortSize)
    if hedgeRatio > 0.8 {
        // 높은 헤지 비율 - 위험 감소
        alert("High hedge ratio detected: " + hedgeRatio)
    }
}
```

### 상관관계 분석
포지션 간 상관관계를 분석하여 집중도 위험을 평가합니다:

```json
{
  "correlation_matrix": {
    "BTCUSDT": {
      "ETHUSDT": 0.85,
      "ADAUSDT": 0.72,
      "DOTUSDT": 0.68
    }
  },
  "high_correlation_clusters": [
    ["BTCUSDT", "ETHUSDT"],
    ["ADAUSDT", "DOTUSDT"]
  ]
}
```

## 성능 최적화

### 포지션 캐시
빈번한 조회를 위해 포지션 데이터를 캐시합니다:

```go
type PositionCache struct {
    positions sync.Map
    ttl       time.Duration
}

func (c *PositionCache) GetPosition(key string) (*Position, bool) {
    if data, ok := c.positions.Load(key); ok {
        entry := data.(*CacheEntry)
        if time.Since(entry.Timestamp) < c.ttl {
            return entry.Position, true
        }
        c.positions.Delete(key) // 만료된 데이터 삭제
    }
    return nil, false
}
```

### 배치 위험 계산
여러 포지션의 위험 지표를 배치로 계산합니다:

```go
func calculateBatchRisk(positions []Position) []PositionRisk {
    var wg sync.WaitGroup
    risks := make([]PositionRisk, len(positions))
    
    for i, pos := range positions {
        wg.Add(1)
        go func(i int, pos Position) {
            defer wg.Done()
            risks[i] = calculatePositionRisk(pos)
        }(i, pos)
    }
    wg.Wait()
    
    return risks
}
```

## 알림 및 경고

### 위험 한도 경고
```bash
# 증거금 사용률이 80% 초과 시 경고
if margin_usage > 0.8 {
    send_alert("High margin usage: " + margin_usage * 100 + "%")
}

# 집중도 위험 경고  
if symbol_concentration > 0.5 {
    send_alert("High symbol concentration: " + symbol_concentration * 100 + "%")
}
```

### 손익 알림
```bash
# 일일 손실이 계정의 5% 초과 시
if daily_pnl < -account_balance * 0.05 {
    send_alert("Daily loss exceeds 5% of account balance")
}
```

## 오류 처리

### 포지션 찾을 수 없음
```json
{
  "code": 5,
  "message": "NOT_FOUND: 포지션이 존재하지 않습니다",
  "details": [{
    "type": "PositionNotFoundError",
    "symbol": "BTCUSDT",
    "account_id": "main-account",
    "exchange": "binance"
  }]
}
```

### 계정 액세스 거부
```json
{
  "code": 7,
  "message": "PERMISSION_DENIED: 계정에 대한 액세스 권한이 없습니다",
  "details": [{
    "type": "AccountAccessError",
    "account_id": "restricted-account",
    "required_permission": "read_positions"
  }]
}
```

## 모범 사례

### 1. 정기적인 위험 모니터링
```go
// 5분마다 위험 지표 확인
ticker := time.NewTicker(5 * time.Minute)
for range ticker.C {
    metrics, err := positionService.GetRiskMetrics(ctx, &GetRiskMetricsRequest{
        AccountId: "main-account",
    })
    if err != nil {
        log.Error("Failed to get risk metrics: %v", err)
        continue
    }
    
    checkRiskLimits(metrics)
}
```

### 2. 포지션 규모 관리
```go
// 단일 포지션이 포트폴리오의 10%를 초과하지 않도록 제한
func validatePositionSize(newPosition Position, portfolio Portfolio) error {
    positionValue := calculatePositionValue(newPosition)
    portfolioValue := calculatePortfolioValue(portfolio)
    
    if positionValue/portfolioValue > 0.1 {
        return errors.New("포지션 크기가 포트폴리오의 10%를 초과합니다")
    }
    
    return nil
}
```

### 3. 상관관계 기반 위험 관리
```go
// 높은 상관관계를 가진 포지션들의 총 노출도 제한
func checkCorrelationRisk(positions []Position) error {
    correlationMatrix := calculateCorrelationMatrix(positions)
    
    for i, pos1 := range positions {
        for j, pos2 := range positions {
            if i != j && correlationMatrix[i][j] > 0.8 {
                combinedExposure := pos1.Exposure + pos2.Exposure
                if combinedExposure > maxCorrelatedExposure {
                    return errors.New("상관관계가 높은 포지션들의 노출도가 한도를 초과합니다")
                }
            }
        }
    }
    
    return nil
}
```

## 통합 예제

### Python 포지션 모니터
```python
import grpc
from proto.oms.v1 import position_service_pb2_grpc as pos_grpc
from proto.oms.v1 import position_service_pb2 as pos_pb2

class PositionMonitor:
    def __init__(self, channel):
        self.stub = pos_grpc.PositionServiceStub(channel)
    
    def monitor_positions(self, account_id):
        request = pos_pb2.ListPositionsRequest(
            account_id=account_id,
            include_zero=False
        )
        
        response = self.stub.ListPositions(request)
        
        for position in response.positions:
            if float(position.unrealized_pnl) < -1000:  # $1000 손실 시 경고
                self.send_alert(f"Large unrealized loss: {position.symbol}")
    
    def get_risk_summary(self, account_id):
        request = pos_pb2.GetRiskMetricsRequest(account_id=account_id)
        response = self.stub.GetRiskMetrics(request)
        
        return {
            'total_exposure': response.metrics.total_exposure,
            'unrealized_pnl': response.metrics.unrealized_pnl,
            'margin_usage': response.metrics.margin_usage
        }
```

## 관련 서비스

- [주문 서비스](./order-service.md) - 포지션에 영향을 주는 주문 실행
- [계정 서비스](./account-service.md) - 계정 잔고 및 증거금 관리
- [시장 데이터 서비스](./market-data-service.md) - 실시간 포지션 업데이트