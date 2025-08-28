# mExOms Examples

이 디렉토리에는 mExOms의 다양한 기능을 보여주는 예제 코드가 포함되어 있습니다.

## 예제 목록

### 기본 거래
- `basic_order.go` - 기본적인 주문 실행
- `order_types.go` - 다양한 주문 유형 (시장가, 지정가, 스톱)
- `order_management.go` - 주문 수정, 취소, 조회

### 멀티계좌 거래
- `multi_account_trading.go` - 여러 계좌에서 동시 거래
- `account_rebalancing.go` - 계좌 간 자산 재분배
- `api_key_rotation.go` - API 키 자동 교체

### 스마트 오더 라우팅
- `smart_router_example.go` - 최적 거래소/계좌 선택
- `order_splitting.go` - 대량 주문 분할 실행
- `liquidity_aggregation.go` - 여러 거래소의 유동성 활용

### 리스크 관리
- `risk_limits.go` - 포지션/손실 한도 설정
- `stop_loss_take_profit.go` - 자동 손절/익절
- `portfolio_risk.go` - 포트폴리오 리스크 관리

### 자동화 전략
- `arbitrage/` - 차익거래 전략
  - `simple_arbitrage.go` - 기본 차익거래
  - `triangular_arbitrage.go` - 삼각 차익거래
  - `cross_exchange_arbitrage.go` - 거래소 간 차익거래

- `market-making/` - 마켓 메이킹 전략
  - `simple_market_maker.go` - 기본 마켓 메이커
  - `dynamic_spread.go` - 동적 스프레드 조정
  - `inventory_management.go` - 재고 관리

### 백테스팅
- `backtesting/` - 전략 백테스트
  - `sma_crossover_backtest.go` - SMA 크로스오버
  - `mean_reversion_backtest.go` - 평균 회귀
  - `momentum_backtest.go` - 모멘텀 전략
  - `portfolio_backtest.go` - 포트폴리오 백테스트

### WebSocket 실시간 데이터
- `websocket/` - WebSocket 예제
  - `ticker_stream.go` - 실시간 가격 스트림
  - `orderbook_stream.go` - 오더북 업데이트
  - `trade_stream.go` - 체결 데이터 스트림
  - `account_stream.go` - 계좌 업데이트 스트림

### 모니터링 및 알림
- `monitoring/` - 시스템 모니터링
  - `health_check.go` - 시스템 상태 체크
  - `performance_metrics.go` - 성능 메트릭 수집
  - `alert_notifications.go` - 알림 설정

## 예제 실행 방법

### 1. 환경 설정
```bash
# API 키 설정 (이미 설정되어 있다면 생략)
./scripts/vault-add-keys.sh binance_spot YOUR_API_KEY YOUR_SECRET_KEY

# 시스템 시작
make run-all
```

### 2. 예제 실행
```bash
# 기본 주문 예제
go run examples/basic_order.go

# 스마트 라우터 예제
go run examples/smart_router_example.go

# 차익거래 예제
go run examples/arbitrage/simple_arbitrage.go
```

### 3. 백테스트 실행
```bash
# SMA 크로스오버 백테스트
go run examples/backtesting/sma_crossover_backtest.go \
  --symbol BTC/USDT \
  --start 2024-01-01 \
  --end 2024-12-31 \
  --initial-capital 10000
```

## 주요 예제 상세 설명

### Smart Router Example
```go
// 스마트 라우터를 사용한 최적 실행
router := router.NewSmartRouter(exchanges, accounts)

// 최적의 거래소와 계좌 자동 선택
result, err := router.ExecuteOrder(ctx, &types.Order{
    Symbol:   "BTC/USDT",
    Side:     types.OrderSideBuy,
    Type:     types.OrderTypeMarket,
    Quantity: 1.0,
})
```

### Multi-Account Trading
```go
// 여러 계좌에서 동시 거래
manager := account.NewManager(vault)

// 전략별로 다른 계좌 사용
mainAccount := manager.GetAccount("main")
arbAccount := manager.GetAccount("arbitrage")
mmAccount := manager.GetAccount("market_maker")
```

### Risk Management
```go
// 리스크 엔진 설정
riskEngine := risk.NewEngine()
riskEngine.SetMaxPositionSize("BTC/USDT", 10.0)
riskEngine.SetMaxDailyLoss(1000.0)
riskEngine.SetMaxLeverage(3.0)

// 주문 전 리스크 체크
if err := riskEngine.CheckOrder(order); err != nil {
    log.Printf("Risk check failed: %v", err)
    return
}
```

## 더 많은 리소스

- [API 문서](../api/)
- [아키텍처 가이드](../architecture/)
- [개발자 가이드](../user-guides/developer-guide.md)
- [FAQ](../FAQ.md)

## 기여하기

새로운 예제나 개선사항이 있다면 PR을 보내주세요!

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/new-example`)
3. Commit your changes (`git commit -am 'Add new example'`)
4. Push to the branch (`git push origin feature/new-example`)
5. Create a new Pull Request