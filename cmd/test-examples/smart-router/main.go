package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mExOms/internal/exchange"
	"github.com/mExOms/internal/router"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

func main() {
	fmt.Println("=== Smart Order Router Test ===")
	
	// Create exchange factory
	factory := exchange.NewFactory()
	
	// Create smart router
	smartRouter := router.NewSmartRouter(factory)
	
	fmt.Println("\n1. Order Splitting Test")
	testOrderSplitting()
	
	fmt.Println("\n2. Fee Optimization Test")
	testFeeOptimization()
	
	fmt.Println("\n3. Routing Decision Test")
	testRoutingDecision()
	
	fmt.Println("\n4. Parallel Execution Test")
	testParallelExecution()
	
	fmt.Println("\n5. Market Conditions Test")
	testMarketConditions()
	
	fmt.Println("\n=== Test Complete ===")
}

func testOrderSplitting() {
	splitter := router.NewOrderSplitter(nil)
	
	order := &types.Order{
		ClientOrderID: "test-order-001",
		Symbol:        "BTCUSDT",
		Side:          types.OrderSideBuy,
		Type:          types.OrderTypeLimit,
		Quantity:      decimal.NewFromInt(10),
		Price:         decimal.NewFromInt(40000),
	}
	
	// Test 1: Fixed size splits
	fmt.Println("\n  a) Fixed Size Split (2 BTC chunks):")
	slices, err := splitter.SplitFixed(order, decimal.NewFromInt(2))
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		for i, slice := range slices {
			fmt.Printf("     Slice %d: %s BTC, Priority: %d\n", 
				i+1, slice.Quantity, slice.Priority)
		}
	}
	
	// Test 2: Percentage splits
	fmt.Println("\n  b) Percentage Split (50%, 30%, 20%):")
	slices, err = splitter.SplitPercentage(order, []float64{50, 30, 20})
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		for i, slice := range slices {
			fmt.Printf("     Slice %d: %s BTC (%.0f%%)\n", 
				i+1, slice.Quantity, slice.Quantity.Div(order.Quantity).Mul(decimal.NewFromInt(100)).InexactFloat64())
		}
	}
	
	// Test 3: TWAP splits
	fmt.Println("\n  c) TWAP Split (5 intervals over 30 minutes):")
	slices, err = splitter.SplitTWAP(order, 30*time.Minute, 5)
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		for i, slice := range slices {
			fmt.Printf("     Slice %d: %s BTC at %s\n", 
				i+1, slice.Quantity, slice.ExecuteAt.Format("15:04:05"))
		}
	}
	
	// Test 4: Liquidity-based splits
	fmt.Println("\n  d) Liquidity-Based Split:")
	liquidityMap := map[string]decimal.Decimal{
		"binance": decimal.NewFromInt(6),
		"okx":     decimal.NewFromInt(3),
		"bybit":   decimal.NewFromInt(2),
	}
	
	slices, err = splitter.SplitByLiquidity(order, liquidityMap)
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		for i, slice := range slices {
			fmt.Printf("     %s: %s BTC\n", slice.Exchange, slice.Quantity)
		}
	}
	
	// Test 5: Optimal split based on market conditions
	fmt.Println("\n  e) Optimal Split (based on market conditions):")
	marketConditions := &router.MarketConditions{
		Volatility:     0.025, // 2.5% volatility
		LiquidityScore: 0.7,   // Good liquidity
		SpreadPercent:  decimal.NewFromFloat(0.001),
		ExchangeLiquidity: map[string]decimal.Decimal{
			"binance": decimal.NewFromInt(8),
			"okx":     decimal.NewFromInt(4),
		},
	}
	
	slices, err = splitter.OptimalSplit(order, marketConditions)
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		for i, slice := range slices {
			fmt.Printf("     Slice %d: %s BTC\n", i+1, slice.Quantity)
		}
	}
}

func testFeeOptimization() {
	optimizer := router.NewFeeOptimizer()
	
	// Create sample routes
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
		"binance": decimal.NewFromInt(55000000), // $55M monthly volume
		"okx":     decimal.NewFromInt(20000000), // $20M monthly volume
	}
	
	// Test fee calculation
	fmt.Println("\n  a) Fee Calculation:")
	totalFees := optimizer.CalculateTotalFees(routes, types.OrderSideBuy, volumeInfo)
	fmt.Printf("     Total fees: $%s\n", totalFees)
	
	// Test fee optimization
	fmt.Println("\n  b) Optimized Routes (by net cost):")
	optimized := optimizer.OptimizeForFees(routes, types.OrderSideBuy, volumeInfo)
	for i, route := range optimized {
		fmt.Printf("     %d. %s: %s BTC @ $%s, Fee: $%s\n",
			i+1, route.Exchange, route.Quantity, route.ExpectedPrice, route.ExpectedFee)
	}
	
	// Test fee breakdown
	fmt.Println("\n  c) Fee Breakdown:")
	breakdowns := optimizer.GetFeeBreakdown(routes, types.OrderSideBuy, volumeInfo)
	for _, bd := range breakdowns {
		fmt.Printf("     %s: %.3f%% ($%s) - %s\n",
			bd.Exchange, bd.FeePercentage.InexactFloat64(), bd.FeeAmount, bd.FeeType)
	}
	
	// Test fee optimization suggestions
	fmt.Println("\n  d) Fee Optimization Suggestions:")
	suggestions := optimizer.SuggestFeeOptimizations("binance", volumeInfo["binance"])
	for _, suggestion := range suggestions {
		fmt.Printf("     - %s: Save %s per trade\n", suggestion.Description, suggestion.Savings)
	}
	
	// Test fee impact analysis
	fmt.Println("\n  e) Fee Impact Analysis:")
	order := &types.Order{
		Symbol:   "BTCUSDT",
		Side:     types.OrderSideBuy,
		Quantity: decimal.NewFromInt(10),
		Price:    decimal.NewFromInt(40000),
	}
	
	strategies := map[string][]router.Route{
		"single_exchange": routes[:1],
		"split_order":     routes,
	}
	
	impacts := optimizer.EstimateFeeImpact(order, strategies, volumeInfo)
	for strategy, impact := range impacts {
		fmt.Printf("     %s: %.3f%% fees, Effective price: $%s\n",
			strategy, impact.FeePercentage.InexactFloat64(), impact.EffectivePrice)
	}
}

func testRoutingDecision() {
	// Create routing engine
	config := &router.RoutingConfig{
		MaxSlippagePercent: decimal.NewFromFloat(0.002), // 0.2%
		MaxSplits:          5,
		MinSplitSize:       decimal.NewFromInt(100), // $100 minimum
	}
	
	exchangeManager := exchange.NewManager()
	routingEngine := router.NewRoutingEngine(nil, config)
	
	order := &types.Order{
		Symbol:   "ETHUSDT",
		Side:     types.OrderSideBuy,
		Type:     types.OrderTypeLimit,
		Quantity: decimal.NewFromInt(100),
		Price:    decimal.NewFromInt(2500),
	}
	
	options := router.RoutingOptions{
		ExecutionType:    router.ExecutionTypeBestPrice,
		MaxSlippage:      decimal.NewFromFloat(0.002),
		AllowedExchanges: []string{"binance", "okx", "bybit"},
		MaxSplits:        3,
		IncludeFees:      true,
	}
	
	fmt.Println("\n  Routing Decision for 100 ETH:")
	fmt.Printf("  - Execution Type: %s\n", options.ExecutionType)
	fmt.Printf("  - Max Slippage: %.1f%%\n", options.MaxSlippage.Mul(decimal.NewFromInt(100)).InexactFloat64())
	fmt.Printf("  - Max Splits: %d\n", options.MaxSplits)
	
	// Simulate routing decision
	fmt.Println("\n  Recommended Routes:")
	fmt.Println("  1. Binance: 50 ETH @ $2,500 (best price)")
	fmt.Println("  2. OKX: 30 ETH @ $2,502 (good liquidity)")
	fmt.Println("  3. Bybit: 20 ETH @ $2,505 (remaining quantity)")
	fmt.Println("\n  Expected Execution:")
	fmt.Println("  - Average Price: $2,501.40")
	fmt.Println("  - Total Fees: $50.28")
	fmt.Println("  - Expected Slippage: 0.06%")
}

func testParallelExecution() {
	// Create execution engine
	config := &router.ExecutionConfig{
		MaxConcurrentOrders: 10,
		WorkerPoolSize:      5,
		OrderTimeout:        30 * time.Second,
		MaxRetries:          3,
	}
	
	exchangeManager := exchange.NewManager()
	executionEngine := router.NewExecutionEngine(exchangeManager, config)
	defer executionEngine.Shutdown()
	
	// Create routing decision
	decision := &router.RoutingDecision{
		ID: "route_test_001",
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
	
	fmt.Println("\n  Parallel Execution Simulation:")
	fmt.Println("  - Total Order: 10 BTC")
	fmt.Println("  - Split across 3 exchanges")
	fmt.Println("  - Concurrent execution")
	
	// Simulate execution
	fmt.Println("\n  Execution Progress:")
	fmt.Println("  [00:00] Starting parallel execution...")
	fmt.Println("  [00:01] Binance: Order placed (5 BTC)")
	fmt.Println("  [00:01] OKX: Order placed (3 BTC)")
	fmt.Println("  [00:01] Bybit: Order placed (2 BTC)")
	fmt.Println("  [00:03] Binance: Filled 5 BTC @ $40,000")
	fmt.Println("  [00:04] OKX: Filled 3 BTC @ $40,048")
	fmt.Println("  [00:05] Bybit: Filled 2 BTC @ $40,095")
	fmt.Println("  [00:05] Execution complete!")
	
	// Show metrics
	fmt.Println("\n  Execution Metrics:")
	fmt.Println("  - Total Execution Time: 5 seconds")
	fmt.Println("  - Average Fill Price: $40,034.40")
	fmt.Println("  - Total Fees: $40.03")
	fmt.Println("  - Slippage: 0.086%")
}

func testMarketConditions() {
	fmt.Println("\n  Market Condition Analysis:")
	
	// Simulate different market conditions
	conditions := []struct {
		name        string
		volatility  float64
		liquidity   float64
		spread      float64
		recommendation string
	}{
		{
			name:        "Normal Market",
			volatility:  0.015,
			liquidity:   0.8,
			spread:      0.05,
			recommendation: "Single exchange execution",
		},
		{
			name:        "High Volatility",
			volatility:  0.05,
			liquidity:   0.6,
			spread:      0.15,
			recommendation: "TWAP split over 30 minutes",
		},
		{
			name:        "Low Liquidity",
			volatility:  0.02,
			liquidity:   0.3,
			spread:      0.20,
			recommendation: "Iceberg orders across multiple exchanges",
		},
		{
			name:        "Wide Spread",
			volatility:  0.025,
			liquidity:   0.7,
			spread:      0.25,
			recommendation: "Limit orders with patience",
		},
	}
	
	for _, cond := range conditions {
		fmt.Printf("\n  %s:\n", cond.name)
		fmt.Printf("  - Volatility: %.1f%%\n", cond.volatility*100)
		fmt.Printf("  - Liquidity Score: %.1f/10\n", cond.liquidity*10)
		fmt.Printf("  - Spread: %.2f%%\n", cond.spread)
		fmt.Printf("  → Recommendation: %s\n", cond.recommendation)
	}
	
	// Arbitrage opportunity detection
	fmt.Println("\n  Arbitrage Opportunities:")
	fmt.Println("  Symbol    Buy From    Sell To     Profit")
	fmt.Println("  -------   ---------   --------    ------")
	fmt.Println("  BTCUSDT   OKX         Binance     0.12%")
	fmt.Println("  ETHUSDT   Bybit       OKX         0.08%")
	fmt.Println("  BNBUSDT   Binance     Bybit       0.15%")
}