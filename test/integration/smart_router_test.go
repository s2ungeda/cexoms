package integration

import (
	"context"
	"testing"
	"time"

	"github.com/mExOms/internal/exchange"
	"github.com/mExOms/internal/router"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSmartRouterIntegration(t *testing.T) {
	// 거래소 매니저 생성
	exchangeManager := exchange.NewManager()
	
	// 라우팅 설정
	config := &router.RoutingConfig{
		MaxSlippagePercent: decimal.NewFromFloat(0.002), // 0.2% 최대 슬리피지
		MaxSplits:          5,
		MinSplitSize:       decimal.NewFromInt(100), // $100 최소 분할
	}
	
	// 라우팅 엔진 생성
	routingEngine := router.NewRoutingEngine(exchangeManager, config)
	
	t.Run("주문 분할", func(t *testing.T) {
		splitter := router.NewOrderSplitter(nil)
		
		order := &types.Order{
			ClientOrderID: "test_order_001",
			Symbol:        "BTCUSDT",
			Side:          types.OrderSideBuy,
			Type:          types.OrderTypeLimit,
			Quantity:      decimal.NewFromInt(10),
			Price:         decimal.NewFromInt(40000),
		}
		
		// 고정 크기 분할
		slices, err := splitter.SplitFixed(order, decimal.NewFromInt(2))
		require.NoError(t, err)
		assert.Len(t, slices, 5) // 10 BTC / 2 BTC = 5개
		
		// 비율 분할
		slices, err = splitter.SplitPercentage(order, []float64{50, 30, 20})
		require.NoError(t, err)
		assert.Len(t, slices, 3)
		assert.Equal(t, decimal.NewFromInt(5), slices[0].Quantity)    // 50%
		assert.Equal(t, decimal.NewFromInt(3), slices[1].Quantity)    // 30%
		assert.Equal(t, decimal.NewFromInt(2), slices[2].Quantity)    // 20%
		
		// TWAP 분할
		duration := 30 * time.Minute
		intervals := 6
		slices, err = splitter.SplitTWAP(order, duration, intervals)
		require.NoError(t, err)
		assert.Len(t, slices, intervals)
		
		// 시간 간격 확인
		for i := 1; i < len(slices); i++ {
			timeDiff := slices[i].ExecuteAt.Sub(slices[i-1].ExecuteAt)
			expectedInterval := duration / time.Duration(intervals)
			assert.Equal(t, expectedInterval, timeDiff)
		}
	})
	
	t.Run("수수료 최적화", func(t *testing.T) {
		optimizer := router.NewFeeOptimizer()
		
		routes := []router.Route{
			{
				Exchange:      "binance",
				Symbol:        "BTCUSDT",
				Quantity:      decimal.NewFromInt(5),
				ExpectedPrice: decimal.NewFromInt(40000),
				Market:        types.MarketTypeSpot,
			},
			{
				Exchange:      "okx",
				Symbol:        "BTCUSDT",
				Quantity:      decimal.NewFromInt(5),
				ExpectedPrice: decimal.NewFromInt(40050),
				Market:        types.MarketTypeSpot,
			},
		}
		
		volumeInfo := map[string]decimal.Decimal{
			"binance": decimal.NewFromInt(50000000), // $50M 월간 거래량
			"okx":     decimal.NewFromInt(20000000), // $20M 월간 거래량
		}
		
		// 총 수수료 계산
		totalFees := optimizer.CalculateTotalFees(routes, types.OrderSideBuy, volumeInfo)
		assert.True(t, totalFees.GreaterThan(decimal.Zero))
		
		// 수수료 최적화
		optimizedRoutes := optimizer.OptimizeForFees(routes, types.OrderSideBuy, volumeInfo)
		assert.Len(t, optimizedRoutes, len(routes))
		
		// 최적화 제안
		suggestions := optimizer.SuggestFeeOptimizations("binance", volumeInfo["binance"])
		assert.NotEmpty(t, suggestions)
	})
	
	t.Run("병렬 실행", func(t *testing.T) {
		// 워커 풀 테스트
		pool := router.NewWorkerPool(3)
		pool.Start()
		defer pool.Stop()
		
		done := make(chan bool, 5)
		
		// 5개 작업을 3개 워커로 처리
		for i := 0; i < 5; i++ {
			taskID := i
			pool.Submit(func() {
				time.Sleep(10 * time.Millisecond)
				done <- true
				t.Logf("Task %d completed", taskID)
			})
		}
		
		// 모든 작업 완료 대기
		for i := 0; i < 5; i++ {
			select {
			case <-done:
				// 작업 완료
			case <-time.After(1 * time.Second):
				t.Fatal("작업 시간 초과")
			}
		}
	})
	
	t.Run("시장 조건 분석", func(t *testing.T) {
		splitter := router.NewOrderSplitter(nil)
		
		order := &types.Order{
			Symbol:   "ETHUSDT",
			Side:     types.OrderSideBuy,
			Quantity: decimal.NewFromInt(100),
			Price:    decimal.NewFromInt(2500),
		}
		
		// 시장 조건에 따른 최적 분할
		marketConditions := &router.MarketConditions{
			Volatility:     0.025, // 2.5% 변동성
			LiquidityScore: 0.7,   // 좋은 유동성
			SpreadPercent:  decimal.NewFromFloat(0.001),
			ExchangeLiquidity: map[string]decimal.Decimal{
				"binance": decimal.NewFromInt(60),
				"okx":     decimal.NewFromInt(30),
				"bybit":   decimal.NewFromInt(10),
			},
		}
		
		slices, err := splitter.OptimalSplit(order, marketConditions)
		require.NoError(t, err)
		assert.NotEmpty(t, slices)
		
		// 총 수량 확인
		totalQty := decimal.Zero
		for _, slice := range slices {
			totalQty = totalQty.Add(slice.Quantity)
		}
		assert.True(t, totalQty.Equal(order.Quantity))
	})
	
	t.Run("실행 보고서", func(t *testing.T) {
		// 실행 엔진 설정
		config := &router.ExecutionConfig{
			MaxConcurrentOrders: 10,
			WorkerPoolSize:      5,
			OrderTimeout:        30 * time.Second,
			MaxRetries:          3,
		}
		
		executionEngine := router.NewExecutionEngine(exchangeManager, config)
		defer executionEngine.Shutdown()
		
		// 라우팅 결정
		decision := &router.RoutingDecision{
			ID: "test_route_001",
			OriginalOrder: &types.Order{
				Symbol:   "BTCUSDT",
				Side:     types.OrderSideBuy,
				Quantity: decimal.NewFromInt(10),
				Price:    decimal.NewFromInt(40000),
			},
			Routes: []router.Route{
				{
					Exchange:      "binance",
					Symbol:        "BTCUSDT",
					Quantity:      decimal.NewFromInt(5),
					ExpectedPrice: decimal.NewFromInt(40000),
					Priority:      1,
				},
				{
					Exchange:      "okx",
					Symbol:        "BTCUSDT",
					Quantity:      decimal.NewFromInt(3),
					ExpectedPrice: decimal.NewFromInt(40050),
					Priority:      1,
				},
				{
					Exchange:      "bybit",
					Symbol:        "BTCUSDT",
					Quantity:      decimal.NewFromInt(2),
					ExpectedPrice: decimal.NewFromInt(40100),
					Priority:      1,
				},
			},
			CreatedAt: time.Now(),
		}
		
		// 실행 시뮬레이션 (실제 거래소 연결 없이)
		ctx := context.Background()
		// report, err := executionEngine.Execute(ctx, decision)
		// 실제 테스트에서는 모의 거래소를 사용해야 함
		
		t.Log("실행 엔진 생성 완료")
	})
}

func TestRouterPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("성능 테스트 스킵")
	}
	
	splitter := router.NewOrderSplitter(nil)
	
	// 대량 주문 분할 성능 테스트
	order := &types.Order{
		Symbol:   "BTCUSDT",
		Quantity: decimal.NewFromInt(1000),
		Price:    decimal.NewFromInt(40000),
	}
	
	start := time.Now()
	
	// 100개로 분할
	slices, err := splitter.SplitFixed(order, decimal.NewFromInt(10))
	require.NoError(t, err)
	assert.Len(t, slices, 100)
	
	elapsed := time.Since(start)
	t.Logf("1000 BTC를 100개로 분할하는데 걸린 시간: %v", elapsed)
	assert.Less(t, elapsed, 100*time.Millisecond) // 100ms 이내
	
	// 수수료 계산 성능
	optimizer := router.NewFeeOptimizer()
	routes := make([]router.Route, 100)
	for i := range routes {
		routes[i] = router.Route{
			Exchange:      "binance",
			Symbol:        "BTCUSDT",
			Quantity:      decimal.NewFromInt(10),
			ExpectedPrice: decimal.NewFromInt(40000),
		}
	}
	
	volumeInfo := map[string]decimal.Decimal{
		"binance": decimal.NewFromInt(100000000),
	}
	
	start = time.Now()
	totalFees := optimizer.CalculateTotalFees(routes, types.OrderSideBuy, volumeInfo)
	elapsed = time.Since(start)
	
	t.Logf("100개 라우트의 수수료 계산 시간: %v", elapsed)
	assert.Less(t, elapsed, 10*time.Millisecond) // 10ms 이내
	assert.True(t, totalFees.GreaterThan(decimal.Zero))
}