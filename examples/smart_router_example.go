package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mExOms/internal/account"
	"github.com/mExOms/internal/router"
	"github.com/mExOms/pkg/security"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

func main() {
	// Initialize components
	vaultConfig := &security.VaultConfig{
		Address:        "http://localhost:8200",
		Token:          "dev-token",
		MountPath:      "secret",
		TTL:            5 * time.Minute,
		RotationPeriod: 30 * 24 * time.Hour,
	}

	vaultManager, err := security.NewVaultManager(vaultConfig)
	if err != nil {
		log.Fatalf("Failed to create vault manager: %v", err)
	}

	keyProvider := account.NewAPIKeyProvider(vaultManager)
	
	accountConfig := &account.Config{
		DataDir:          "/data/accounts",
		SnapshotInterval: 1 * time.Hour,
		MetricsRetention: 24 * time.Hour,
	}

	accountManager, err := account.NewManager(accountConfig, keyProvider)
	if err != nil {
		log.Fatalf("Failed to create account manager: %v", err)
	}

	// Create router configuration
	routerConfig := &router.RouterConfig{
		MaxSplitOrders:      5,
		MinOrderSize:        decimal.NewFromFloat(10), // $10 minimum
		SlippageTolerance:   decimal.NewFromFloat(0.002), // 0.2%
		ArbitrageThreshold:  decimal.NewFromFloat(0.1), // 0.1% profit minimum
		PriceUpdateInterval: 100 * time.Millisecond,
		RoutingStrategy:     router.RoutingStrategyBestPrice,
		MaxExecutionTime:    30 * time.Second,
		EnableArbitrage:     true,
		AccountSelection: router.AccountSelection{
			MinBalance:        decimal.NewFromFloat(100), // $100 minimum balance
			RequirePermission: "trade",
		},
		FeeStructures: map[string]router.FeeStructure{
			"binance": {
				Exchange: "binance",
				MakerFee: decimal.NewFromFloat(0.001),
				TakerFee: decimal.NewFromFloat(0.001),
			},
		},
	}

	// Create multi-account router
	smartRouter := router.NewMultiAccountRouter(routerConfig, accountManager)

	// Register exchanges (mock for example)
	// In production, these would be real exchange implementations
	// smartRouter.RegisterExchange("binance", binanceExchange)
	// smartRouter.RegisterExchange("bybit", bybitExchange)

	fmt.Println("=== Smart Order Router Example ===")

	// Example 1: Simple order routing
	fmt.Println("\n1. Simple Best Price Routing")
	simpleOrder := &router.RoutingRequest{
		ID:        "order_001",
		Symbol:    "BTCUSDT",
		Side:      types.OrderSideBuy,
		Quantity:  decimal.NewFromFloat(0.01), // 0.01 BTC
		OrderType: types.OrderTypeLimit,
		Price:     decimal.NewFromFloat(50000), // $50,000
		Strategy:  "manual",
	}

	result, err := smartRouter.RouteOrder(context.Background(), simpleOrder)
	if err != nil {
		log.Printf("Failed to route order: %v", err)
	} else {
		fmt.Printf("Order routed successfully:\n")
		fmt.Printf("- Executed on: %s\n", result.Routes[0].Exchange)
		fmt.Printf("- Account: %s\n", result.Routes[0].AccountID)
		fmt.Printf("- Executed Price: %s\n", result.AveragePrice.String())
		fmt.Printf("- Total Fees: %s\n", result.TotalFees.String())
	}

	// Example 2: Large order with splitting
	fmt.Println("\n2. Large Order with Splitting")
	largeOrder := &router.RoutingRequest{
		ID:        "order_002",
		Symbol:    "ETHUSDT",
		Side:      types.OrderSideBuy,
		Quantity:  decimal.NewFromFloat(100), // 100 ETH
		OrderType: types.OrderTypeLimit,
		Price:     decimal.NewFromFloat(3000), // $3,000
		Strategy:  "dca", // Dollar cost averaging
	}

	result, err = smartRouter.RouteOrder(context.Background(), largeOrder)
	if err != nil {
		log.Printf("Failed to route large order: %v", err)
	} else {
		fmt.Printf("Large order split across %d routes:\n", len(result.Routes))
		for i, route := range result.Routes {
			fmt.Printf("- Route %d: %s:%s, Qty: %s, Price: %s\n",
				i+1, route.Exchange, route.AccountID,
				route.ExecutedQuantity.String(), route.Price.String())
		}
		fmt.Printf("Average execution price: %s\n", result.AveragePrice.String())
	}

	// Example 3: Arbitrage routing
	fmt.Println("\n3. Arbitrage Routing")
	routerConfig.RoutingStrategy = router.RoutingStrategyArbitrage
	
	arbOrder := &router.RoutingRequest{
		ID:        "arb_001",
		Symbol:    "BNBUSDT",
		Side:      types.OrderSideBuy,
		Quantity:  decimal.NewFromFloat(10), // 10 BNB
		OrderType: types.OrderTypeLimit,
		Price:     decimal.NewFromFloat(300), // $300
		Strategy:  "arbitrage",
	}

	result, err = smartRouter.RouteOrder(context.Background(), arbOrder)
	if err != nil {
		log.Printf("Failed to route arbitrage order: %v", err)
	} else {
		if len(result.Routes) >= 2 {
			fmt.Println("Arbitrage executed:")
			fmt.Printf("- Buy on %s at %s\n", result.Routes[0].Exchange, result.Routes[0].Price.String())
			fmt.Printf("- Sell on %s at %s\n", result.Routes[1].Exchange, result.Routes[1].Price.String())
			// Calculate profit
			buyValue := result.Routes[0].Price.Mul(result.Routes[0].ExecutedQuantity)
			sellValue := result.Routes[1].Price.Mul(result.Routes[1].ExecutedQuantity)
			profit := sellValue.Sub(buyValue).Sub(result.TotalFees)
			fmt.Printf("- Net Profit: %s\n", profit.String())
		}
	}

	// Example 4: Account-specific routing
	fmt.Println("\n4. Account-Specific Routing")
	routerConfig.AccountSelection.PreferredAccounts = []string{"sub_spot_arb", "sub_market_making"}
	
	specificOrder := &router.RoutingRequest{
		ID:        "order_003",
		Symbol:    "BTCUSDT",
		Side:      types.OrderSideSell,
		Quantity:  decimal.NewFromFloat(0.05),
		OrderType: types.OrderTypeLimit,
		Price:     decimal.NewFromFloat(51000),
		AccountID: "sub_spot_arb", // Specify account
	}

	result, err = smartRouter.RouteOrder(context.Background(), specificOrder)
	if err != nil {
		log.Printf("Failed to route to specific account: %v", err)
	} else {
		fmt.Printf("Order routed to specified account: %s\n", result.Routes[0].AccountID)
	}

	// Example 5: Lowest fee routing
	fmt.Println("\n5. Lowest Fee Routing")
	routerConfig.RoutingStrategy = router.RoutingStrategyLowestFee
	
	feeOptimizedOrder := &router.RoutingRequest{
		ID:        "order_004",
		Symbol:    "LTCUSDT",
		Side:      types.OrderSideBuy,
		Quantity:  decimal.NewFromFloat(50),
		OrderType: types.OrderTypeLimit,
		Price:     decimal.NewFromFloat(100),
	}

	result, err = smartRouter.RouteOrder(context.Background(), feeOptimizedOrder)
	if err != nil {
		log.Printf("Failed to route order: %v", err)
	} else {
		fmt.Printf("Order routed to lowest fee exchange: %s\n", result.Routes[0].Exchange)
		fmt.Printf("Total fees: %s\n", result.TotalFees.String())
	}

	// Example 6: Get routing metrics
	fmt.Println("\n6. Routing Metrics")
	metrics := smartRouter.GetMetrics()
	fmt.Printf("Routing performance:\n")
	fmt.Printf("- Total Orders: %d\n", metrics.TotalOrders)
	fmt.Printf("- Success Rate: %.2f%%\n", float64(metrics.SuccessfulOrders)/float64(metrics.TotalOrders)*100)
	fmt.Printf("- Total Volume: %s\n", metrics.TotalVolume.String())
	fmt.Printf("- Average Slippage: %s%%\n", metrics.AverageSlippage.Mul(decimal.NewFromInt(100)).String())
	fmt.Printf("- Best Route: %s\n", metrics.BestRoute)

	// Example 7: Complex routing with multiple criteria
	fmt.Println("\n7. Complex Multi-Criteria Routing")
	routerConfig.RoutingStrategy = router.RoutingStrategyBalanced
	routerConfig.AccountSelection = router.AccountSelection{
		MinBalance:        decimal.NewFromFloat(1000),
		MaxPositionSize:   decimal.NewFromFloat(50000),
		RequirePermission: "trade",
		ExcludedAccounts:  []string{"main"}, // Don't use main account
	}

	complexOrder := &router.RoutingRequest{
		ID:          "order_005",
		Symbol:      "BTCUSDT",
		Side:        types.OrderSideBuy,
		Quantity:    decimal.NewFromFloat(1),
		OrderType:   types.OrderTypeLimit,
		Price:       decimal.NewFromFloat(50000),
		MaxSlippage: decimal.NewFromFloat(0.001), // 0.1% max slippage
		Strategy:    "grid_trading",
		Metadata: map[string]interface{}{
			"grid_level": 5,
			"urgency":    "normal",
		},
	}

	result, err = smartRouter.RouteOrder(context.Background(), complexOrder)
	if err != nil {
		log.Printf("Failed complex routing: %v", err)
	} else {
		fmt.Printf("Complex order routed:\n")
		fmt.Printf("- Selected Route: %s:%s\n", result.Routes[0].Exchange, result.Routes[0].AccountID)
		fmt.Printf("- Execution Time: %v\n", result.EndTime.Sub(result.StartTime))
	}

	fmt.Println("\n=== Smart Router Demo Complete ===")
}