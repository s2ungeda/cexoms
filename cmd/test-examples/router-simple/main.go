package main

import (
	"fmt"
	"time"

	"github.com/mExOms/internal/router"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

func main() {
	fmt.Println("=== Smart Order Router Test (Simple) ===")
	
	// Test Order Splitter
	fmt.Println("\n1. Order Splitting Test")
	splitter := router.NewOrderSplitter(nil)
	
	order := &types.Order{
		ClientOrderID: "test-001",
		Symbol:        "BTCUSDT",
		Side:          types.OrderSideBuy,
		Quantity:      decimal.NewFromInt(10),
		Price:         decimal.NewFromInt(40000),
	}
	
	// Fixed split
	fmt.Println("\n  Fixed Split (2 BTC chunks):")
	slices, err := splitter.SplitFixed(order, decimal.NewFromInt(2))
	if err == nil {
		for i, slice := range slices {
			fmt.Printf("  - Slice %d: %s BTC\n", i+1, slice.Quantity)
		}
	}
	
	// TWAP split
	fmt.Println("\n  TWAP Split (5 intervals):")
	slices, err = splitter.SplitTWAP(order, 10*time.Minute, 5)
	if err == nil {
		for i, slice := range slices {
			fmt.Printf("  - Slice %d: %s BTC at %s\n", 
				i+1, slice.Quantity, slice.ExecuteAt.Format("15:04:05"))
		}
	}
	
	// Test Fee Optimizer
	fmt.Println("\n2. Fee Optimization Test")
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
		"binance": decimal.NewFromInt(50000000),
		"okx":     decimal.NewFromInt(20000000),
	}
	
	totalFees := optimizer.CalculateTotalFees(routes, types.OrderSideBuy, volumeInfo)
	fmt.Printf("\n  Total estimated fees: $%s\n", totalFees)
	
	// Get fee suggestions
	fmt.Println("\n  Fee Optimization Suggestions:")
	suggestions := optimizer.SuggestFeeOptimizations("binance", volumeInfo["binance"])
	for _, s := range suggestions {
		fmt.Printf("  - %s\n", s.Description)
	}
	
	// Test Worker Pool
	fmt.Println("\n3. Worker Pool Test")
	pool := router.NewWorkerPool(3)
	pool.Start()
	
	fmt.Println("  Submitting 5 tasks to 3 workers...")
	for i := 0; i < 5; i++ {
		taskID := i
		pool.Submit(func() {
			fmt.Printf("  - Task %d executed\n", taskID)
			time.Sleep(100 * time.Millisecond)
		})
	}
	
	time.Sleep(1 * time.Second)
	pool.Stop()
	
	fmt.Println("\n=== Test Complete ===")
}