# Advanced Features Guide

## 1. 스마트 오더 라우팅

스마트 오더 라우터는 여러 거래소와 계좌에서 최적의 실행 경로를 자동으로 찾습니다.

### 기본 사용법

```go
// 스마트 라우터 초기화
router := router.NewSmartRouter(exchangeManager, accountManager)

// 최적 실행
result, err := router.ExecuteOrder(ctx, &types.Order{
    Symbol:   "BTC/USDT",
    Side:     types.OrderSideBuy,
    Type:     types.OrderTypeMarket,
    Quantity: 10.0, // 10 BTC 매수
})
```

### 고급 설정

```go
// 라우팅 옵션 설정
options := &router.RoutingOptions{
    MaxSlippage:      0.1,        // 최대 0.1% 슬리피지
    MinOrderSize:     0.001,      // 최소 주문 크기
    MaxSplits:        10,         // 최대 10개로 분할
    PreferredVenues:  []string{"binance", "okx"},
    AvoidVenues:      []string{"upbit"},
    AccountStrategy:  router.AccountStrategyLoadBalance,
}

result, err := router.ExecuteOrderWithOptions(ctx, order, options)
```

## 2. 리스크 관리 엔진

C++로 구현된 초고속 리스크 체크 시스템입니다.

### 리스크 한도 설정

```go
// 리스크 엔진 초기화
riskEngine := risk.NewEngine()

// 포지션 한도
riskEngine.SetMaxPositionSize("BTC/USDT", 100.0)
riskEngine.SetMaxPositionValue(1000000.0) // $1M

// 손실 한도
riskEngine.SetMaxDailyLoss(50000.0)      // $50k
riskEngine.SetMaxDrawdown(0.2)           // 20%

// 레버리지 한도
riskEngine.SetMaxLeverage(3.0)
riskEngine.SetMaxAccountLeverage("futures", 10.0)
```

### 실시간 리스크 모니터링

```go
// 리스크 메트릭 조회
metrics := riskEngine.GetRiskMetrics("main_account")
fmt.Printf("Current Exposure: $%.2f\n", metrics.TotalExposure)
fmt.Printf("Daily PnL: $%.2f\n", metrics.DailyPnL)
fmt.Printf("VaR (95%%): $%.2f\n", metrics.ValueAtRisk)
```

## 3. 차익거래 엔진

거래소 간 가격 차이를 자동으로 감지하고 실행합니다.

### 차익거래 설정

```go
// 차익거래 엔진 초기화
arbEngine := arbitrage.NewEngine(exchanges)

// 차익거래 파라미터
config := &arbitrage.Config{
    MinProfitPercent:  0.1,     // 최소 0.1% 수익
    MaxPositionSize:   10000.0, // 최대 $10k
    ExecutionTimeout:  5 * time.Second,
    EnableTriangular:  true,    // 삼각 차익거래 활성화
}

arbEngine.Start(config)
```

### 차익거래 모니터링

```go
// 차익거래 기회 구독
arbEngine.OnOpportunity(func(opp *arbitrage.Opportunity) {
    fmt.Printf("Arbitrage found: %s %.2f%% profit\n", 
        opp.Symbol, opp.ProfitPercent)
    
    // 자동 실행 또는 수동 확인
    if opp.ProfitPercent > 0.5 {
        arbEngine.Execute(opp)
    }
})
```

## 4. 마켓 메이킹

양방향 호가를 제공하여 스프레드 수익을 창출합니다.

### 마켓 메이커 설정

```go
// 마켓 메이커 초기화
mm := marketmaker.New(exchange, "ETH/USDT")

// 전략 파라미터
params := &marketmaker.Parameters{
    SpreadPercent:    0.1,      // 0.1% 스프레드
    OrderSize:        1.0,      // 1 ETH
    MaxInventory:     10.0,     // 최대 10 ETH 보유
    RefreshInterval:  1 * time.Second,
    SkewEnabled:      true,     // 재고 기반 스프레드 조정
}

mm.Start(params)
```

### 동적 스프레드 조정

```go
// 변동성 기반 스프레드
mm.EnableVolatilityAdjustment(true)
mm.SetVolatilityMultiplier(2.0)

// 재고 기반 스프레드
mm.EnableInventorySkew(true)
mm.SetTargetInventory(5.0) // 5 ETH 목표
```

## 5. 백테스팅 시스템

과거 데이터로 전략을 검증합니다.

### 백테스트 실행

```go
// 백테스트 엔진 초기화
backtest := backtest.NewEngine()

// 데이터 로드
backtest.LoadHistoricalData("BTC/USDT", "2024-01-01", "2024-12-31")

// 전략 설정
strategy := strategies.NewSMACrossover(20, 50)
backtest.SetStrategy(strategy)

// 백테스트 실행
results := backtest.Run(&backtest.Config{
    InitialCapital: 10000.0,
    Commission:     0.001,
    Slippage:       0.0005,
})

// 결과 분석
fmt.Printf("Total Return: %.2f%%\n", results.TotalReturn)
fmt.Printf("Sharpe Ratio: %.2f\n", results.SharpeRatio)
fmt.Printf("Max Drawdown: %.2f%%\n", results.MaxDrawdown)
```

### 최적화

```go
// 파라미터 최적화
optimizer := backtest.NewOptimizer()
optimizer.AddParameter("fast_ma", 10, 50, 5)
optimizer.AddParameter("slow_ma", 50, 200, 10)

bestParams := optimizer.Optimize(strategy, backtest.OptimizeSharpe)
```

## 6. 멀티계좌 관리

여러 계좌를 효율적으로 관리합니다.

### 계좌 그룹 관리

```go
// 계좌 매니저
manager := account.NewManager()

// 계좌 그룹 생성
manager.CreateGroup("arbitrage", []string{"arb1", "arb2", "arb3"})
manager.CreateGroup("market_making", []string{"mm1", "mm2"})

// 그룹별 작업
manager.ExecuteOnGroup("arbitrage", func(acc *Account) error {
    return acc.PlaceOrder(order)
})
```

### 자동 리밸런싱

```go
// 리밸런서 설정
rebalancer := account.NewRebalancer(manager)

// 목표 배분
targets := map[string]float64{
    "main":        0.5,  // 50%
    "arbitrage":   0.3,  // 30%
    "market_maker": 0.2, // 20%
}

rebalancer.Rebalance(targets)
```

## 7. 고급 모니터링

### Prometheus 메트릭

```go
// 커스텀 메트릭 등록
orderLatency := prometheus.NewHistogram(prometheus.HistogramOpts{
    Name: "oms_order_latency_seconds",
    Help: "Order execution latency",
})

// 메트릭 기록
start := time.Now()
placeOrder()
orderLatency.Observe(time.Since(start).Seconds())
```

### 실시간 알림

```go
// 알림 설정
alerter := monitoring.NewAlerter()

// 조건 기반 알림
alerter.AddRule(&monitoring.Rule{
    Name:      "high_loss",
    Condition: "daily_pnl < -1000",
    Action:    monitoring.ActionEmail | monitoring.ActionSlack,
})
```

## 8. 성능 최적화

### CPU 친화성

```go
// 특정 CPU 코어에 고정
performance.SetCPUAffinity([]int{0, 1, 2, 3})

// NUMA 노드 설정
performance.SetNUMANode(0)
```

### 메모리 풀

```go
// 사전 할당된 메모리 풀
pool := performance.NewMemoryPool(1000000) // 1M 객체
order := pool.GetOrder()
defer pool.PutOrder(order)
```

## 다음 단계

- [보안 가이드](../security.md)
- [프로덕션 배포](../deployment.md)
- [API 레퍼런스](../api/)