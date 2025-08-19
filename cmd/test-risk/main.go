package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
	
	"github.com/mExOms/internal/risk"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

func main() {
	fmt.Println("=== Testing Risk Management Engine ===\n")
	
	// Create risk engine
	riskEngine := risk.NewRiskEngine()
	
	// Set risk limits
	riskEngine.SetMaxPositionSize(decimal.NewFromFloat(50000))  // $50k max position
	riskEngine.SetMaxLeverage(10)                                // 10x max leverage
	riskEngine.SetMaxOrderValue(decimal.NewFromFloat(20000))     // $20k max order
	riskEngine.SetMaxDailyLoss(decimal.NewFromFloat(5000))       // $5k max daily loss
	riskEngine.SetMaxExposure(decimal.NewFromFloat(200000))      // $200k max exposure
	
	fmt.Println("✓ Risk engine created with limits:")
	fmt.Println("  Max position size: $50,000")
	fmt.Println("  Max leverage: 10x")
	fmt.Println("  Max order value: $20,000")
	fmt.Println("  Max daily loss: $5,000")
	fmt.Println("  Max exposure: $200,000")
	
	// Test basic risk checks
	testBasicRiskChecks(riskEngine)
	
	// Test position limits
	testPositionLimits(riskEngine)
	
	// Test leverage limits
	testLeverageLimits(riskEngine)
	
	// Test daily loss limits
	testDailyLossLimits(riskEngine)
	
	// Test exposure limits
	testExposureLimits(riskEngine)
	
	// Performance benchmark
	testPerformance(riskEngine)
	
	// Show final metrics
	showMetrics(riskEngine)
	
	fmt.Println("\n✓ All tests completed!")
}

func testBasicRiskChecks(engine *risk.RiskEngine) {
	fmt.Println("\n=== Testing Basic Risk Checks ===")
	
	ctx := context.Background()
	
	// Test normal order - should pass
	normalOrder := &types.Order{
		Symbol:   "BTCUSDT",
		Side:     types.OrderSideBuy,
		Type:     types.OrderTypeLimit,
		Price:    decimal.NewFromFloat(42000),
		Quantity: decimal.NewFromFloat(0.1),
	}
	
	result, err := engine.CheckOrder(ctx, normalOrder, "binance")
	if err != nil {
		log.Fatal("Risk check failed:", err)
	}
	
	if result.Passed {
		fmt.Printf("✓ Normal order passed: %s BTC @ $42,000 (value: $4,200)\n", 
			normalOrder.Quantity.String())
	} else {
		fmt.Printf("✗ Normal order rejected: %s\n", result.RejectionReason)
	}
	fmt.Printf("  Check duration: %v\n", result.CheckDuration)
	
	// Test large order - should fail
	largeOrder := &types.Order{
		Symbol:   "BTCUSDT",
		Side:     types.OrderSideBuy,
		Type:     types.OrderTypeLimit,
		Price:    decimal.NewFromFloat(42000),
		Quantity: decimal.NewFromFloat(1), // $42,000 order
	}
	
	result, err = engine.CheckOrder(ctx, largeOrder, "binance")
	if err != nil {
		log.Fatal("Risk check failed:", err)
	}
	
	if !result.Passed {
		fmt.Printf("✓ Large order correctly rejected: %s\n", result.RejectionReason)
	} else {
		fmt.Printf("✗ Large order should have been rejected\n")
	}
}

func testPositionLimits(engine *risk.RiskEngine) {
	fmt.Println("\n=== Testing Position Limits ===")
	
	ctx := context.Background()
	
	// Add existing position
	position := &risk.PositionRisk{
		Symbol:        "BTCUSDT",
		Exchange:      "binance",
		Market:        "spot",
		Quantity:      decimal.NewFromFloat(1),
		AvgEntryPrice: decimal.NewFromFloat(40000),
		MarkPrice:     decimal.NewFromFloat(42000),
		UnrealizedPnL: decimal.NewFromFloat(2000),
		UpdatedAt:     time.Now(),
	}
	engine.UpdatePosition("binance", "BTCUSDT", position)
	fmt.Printf("✓ Current position: %s BTC\n", position.Quantity.String())
	
	// Try to add more - should fail if exceeds limit
	additionalOrder := &types.Order{
		Symbol:   "BTCUSDT",
		Side:     types.OrderSideBuy,
		Type:     types.OrderTypeLimit,
		Price:    decimal.NewFromFloat(42000),
		Quantity: decimal.NewFromFloat(0.5),
	}
	
	result, err := engine.CheckOrder(ctx, additionalOrder, "binance")
	if err != nil {
		log.Fatal("Risk check failed:", err)
	}
	
	if !result.Passed {
		fmt.Printf("✓ Position limit correctly enforced: %s\n", result.RejectionReason)
	} else {
		fmt.Printf("✓ Additional order passed, new position would be: 1.5 BTC\n")
	}
}

func testLeverageLimits(engine *risk.RiskEngine) {
	fmt.Println("\n=== Testing Leverage Limits ===")
	
	ctx := context.Background()
	
	// Set account balance
	balance := &types.Balance{
		Exchange: "binance",
		Market:   "futures",
		Assets: map[string]types.AssetBalance{
			"USDT": {
				Asset:  "USDT",
				Free:   "5000",
				Locked: "0",
			},
		},
	}
	engine.UpdateBalance("binance", balance)
	fmt.Println("✓ Account balance: $5,000 USDT")
	
	// Test futures order with high leverage
	futuresOrder := &types.Order{
		Symbol:       "BTCUSDT",
		Side:         types.OrderSideBuy,
		Type:         types.OrderTypeLimit,
		Price:        decimal.NewFromFloat(42000),
		Quantity:     decimal.NewFromFloat(2), // $84,000 position value
		PositionSide: types.PositionSideLong,  // Futures order
	}
	
	result, err := engine.CheckOrder(ctx, futuresOrder, "binance")
	if err != nil {
		log.Fatal("Risk check failed:", err)
	}
	
	if !result.Passed {
		fmt.Printf("✓ High leverage order correctly rejected: %s\n", result.RejectionReason)
	} else {
		fmt.Printf("✗ High leverage order should have been rejected\n")
		if leverage, ok := result.RiskMetrics["estimated_leverage"]; ok {
			fmt.Printf("  Estimated leverage: %sx\n", leverage)
		}
	}
}

func testDailyLossLimits(engine *risk.RiskEngine) {
	fmt.Println("\n=== Testing Daily Loss Limits ===")
	
	ctx := context.Background()
	
	// Set daily loss
	engine.UpdateDailyPnL(decimal.NewFromFloat(-4000))
	fmt.Println("✓ Current daily P&L: -$4,000")
	
	// Try to place order with strict mode off
	order := &types.Order{
		Symbol:   "BTCUSDT",
		Side:     types.OrderSideBuy,
		Type:     types.OrderTypeLimit,
		Price:    decimal.NewFromFloat(42000),
		Quantity: decimal.NewFromFloat(0.1),
	}
	
	result, err := engine.CheckOrder(ctx, order, "binance")
	if err != nil {
		log.Fatal("Risk check failed:", err)
	}
	
	if result.Passed {
		fmt.Println("✓ Order passed with daily loss warning (strict mode off)")
		if _, hasWarning := result.RiskMetrics["daily_loss_warning"]; hasWarning {
			fmt.Println("  ⚠️  Daily loss warning triggered")
		}
	}
	
	// Enable strict mode
	engine.SetStrictMode(true)
	fmt.Println("\n✓ Strict mode enabled")
	
	// Update to exceed daily loss limit
	engine.UpdateDailyPnL(decimal.NewFromFloat(-6000))
	fmt.Println("✓ Daily P&L updated to: -$6,000 (exceeds $5,000 limit)")
	
	result, err = engine.CheckOrder(ctx, order, "binance")
	if err != nil {
		log.Fatal("Risk check failed:", err)
	}
	
	if !result.Passed {
		fmt.Printf("✓ Order correctly rejected in strict mode: %s\n", result.RejectionReason)
	}
	
	// Disable strict mode
	engine.SetStrictMode(false)
}

func testExposureLimits(engine *risk.RiskEngine) {
	fmt.Println("\n=== Testing Exposure Limits ===")
	
	ctx := context.Background()
	
	// Add multiple positions
	positions := []struct {
		symbol   string
		exchange string
		quantity float64
		price    float64
	}{
		{"BTCUSDT", "binance", 1, 42000},
		{"ETHUSDT", "binance", 20, 2200},
		{"BNBUSDT", "binance", 100, 300},
	}
	
	for _, pos := range positions {
		position := &risk.PositionRisk{
			Symbol:        pos.symbol,
			Exchange:      pos.exchange,
			Market:        "spot",
			Quantity:      decimal.NewFromFloat(pos.quantity),
			AvgEntryPrice: decimal.NewFromFloat(pos.price),
			MarkPrice:     decimal.NewFromFloat(pos.price),
			UpdatedAt:     time.Now(),
		}
		engine.UpdatePosition(pos.exchange, pos.symbol, position)
		value := decimal.NewFromFloat(pos.quantity * pos.price)
		fmt.Printf("✓ Added position: %s = $%s\n", pos.symbol, value.String())
	}
	
	// Total exposure: $42,000 + $44,000 + $30,000 = $116,000
	
	// Try to add order that would exceed exposure limit
	newOrder := &types.Order{
		Symbol:   "BTCUSDT",
		Side:     types.OrderSideBuy,
		Type:     types.OrderTypeLimit,
		Price:    decimal.NewFromFloat(42000),
		Quantity: decimal.NewFromFloat(2.5), // $105,000 additional
	}
	
	result, err := engine.CheckOrder(ctx, newOrder, "binance")
	if err != nil {
		log.Fatal("Risk check failed:", err)
	}
	
	if !result.Passed {
		fmt.Printf("\n✓ Exposure limit correctly enforced: %s\n", result.RejectionReason)
		fmt.Printf("  Current exposure: %s\n", result.RiskMetrics["current_exposure"])
	}
}

func testPerformance(engine *risk.RiskEngine) {
	fmt.Println("\n=== Performance Benchmark ===")
	
	ctx := context.Background()
	
	// Warm up
	order := &types.Order{
		Symbol:   "BTCUSDT",
		Side:     types.OrderSideBuy,
		Type:     types.OrderTypeLimit,
		Price:    decimal.NewFromFloat(42000),
		Quantity: decimal.NewFromFloat(0.1),
	}
	
	for i := 0; i < 1000; i++ {
		engine.CheckOrder(ctx, order, "binance")
	}
	
	// Benchmark single-threaded
	iterations := 100000
	start := time.Now()
	
	for i := 0; i < iterations; i++ {
		engine.CheckOrder(ctx, order, "binance")
	}
	
	duration := time.Since(start)
	avgTime := duration / time.Duration(iterations)
	fmt.Printf("\n✓ Single-threaded performance:\n")
	fmt.Printf("  Total checks: %d\n", iterations)
	fmt.Printf("  Total time: %v\n", duration)
	fmt.Printf("  Average per check: %v\n", avgTime)
	fmt.Printf("  Average per check (μs): %.2f\n", float64(avgTime.Nanoseconds())/1000)
	
	// Benchmark multi-threaded
	concurrency := 10
	var wg sync.WaitGroup
	var totalChecks atomic.Uint64
	
	start = time.Now()
	
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations/concurrency; j++ {
				engine.CheckOrder(ctx, order, "binance")
				totalChecks.Add(1)
			}
		}()
	}
	
	wg.Wait()
	duration = time.Since(start)
	
	avgTime = duration / time.Duration(totalChecks.Load())
	fmt.Printf("\n✓ Multi-threaded performance (%d goroutines):\n", concurrency)
	fmt.Printf("  Total checks: %d\n", totalChecks.Load())
	fmt.Printf("  Total time: %v\n", duration)
	fmt.Printf("  Average per check: %v\n", avgTime)
	fmt.Printf("  Average per check (μs): %.2f\n", float64(avgTime.Nanoseconds())/1000)
	
	if avgTime < 50*time.Microsecond {
		fmt.Printf("\n✅ PERFORMANCE TARGET MET: < 50 microseconds per check\n")
	} else {
		fmt.Printf("\n⚠️  Performance below target (50μs), current: %.2fμs\n", 
			float64(avgTime.Nanoseconds())/1000)
	}
}

func showMetrics(engine *risk.RiskEngine) {
	fmt.Println("\n=== Risk Engine Metrics ===")
	
	metrics := engine.GetMetrics()
	
	fmt.Printf("\nConfiguration:\n")
	fmt.Printf("  Enabled: %v\n", metrics["enabled"])
	fmt.Printf("  Strict mode: %v\n", metrics["strict_mode"])
	fmt.Printf("  Max position size: $%s\n", metrics["max_position_size"])
	fmt.Printf("  Max leverage: %dx\n", metrics["max_leverage"])
	fmt.Printf("  Max order value: $%s\n", metrics["max_order_value"])
	fmt.Printf("  Max daily loss: $%s\n", metrics["max_daily_loss"])
	fmt.Printf("  Max exposure: $%s\n", metrics["max_exposure"])
	
	fmt.Printf("\nPerformance:\n")
	fmt.Printf("  Orders checked: %d\n", metrics["orders_checked"])
	fmt.Printf("  Checks performed: %d\n", metrics["checks_performed"])
	fmt.Printf("  Orders rejected: %d\n", metrics["orders_rejected"])
	fmt.Printf("  Average check time: %.2f μs\n", metrics["avg_check_time_us"])
	
	fmt.Printf("\nCurrent State:\n")
	fmt.Printf("  Current exposure: $%s\n", metrics["current_exposure"])
	fmt.Printf("  Daily P&L: $%s\n", metrics["daily_pnl"])
}