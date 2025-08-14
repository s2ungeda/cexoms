package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
	
	"github.com/mExOms/oms/internal/position"
	"github.com/shopspring/decimal"
)

func main() {
	fmt.Println("=== Testing Integrated Position Management ===\n")
	
	// Create data directory for snapshots
	dataDir := "./data/snapshots"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatal("Failed to create data directory:", err)
	}
	
	// Create position manager
	posManager, err := position.NewPositionManager(dataDir)
	if err != nil {
		log.Fatal("Failed to create position manager:", err)
	}
	defer posManager.Close()
	
	fmt.Println("✓ Position manager created with shared memory")
	fmt.Println("  Shared memory: /dev/shm/oms_positions")
	fmt.Printf("  Snapshot directory: %s\n", dataDir)
	
	// Test basic position operations
	testBasicOperations(posManager)
	
	// Test multi-exchange positions
	testMultiExchangePositions(posManager)
	
	// Test aggregated positions
	testAggregatedPositions(posManager)
	
	// Test P&L calculations
	testPnLCalculations(posManager)
	
	// Test concurrent updates
	testConcurrentUpdates(posManager)
	
	// Test snapshot functionality
	testSnapshot(posManager)
	
	// Show final metrics
	showMetrics(posManager)
	
	fmt.Println("\n✓ All tests completed!")
}

func testBasicOperations(pm *position.PositionManager) {
	fmt.Println("\n=== Testing Basic Operations ===")
	
	// Create a position
	pos := &position.Position{
		Symbol:     "BTCUSDT",
		Exchange:   "binance",
		Market:     "spot",
		Side:       "LONG",
		Quantity:   decimal.NewFromFloat(0.5),
		EntryPrice: decimal.NewFromFloat(40000),
		MarkPrice:  decimal.NewFromFloat(42000),
		Leverage:   1,
		MarginUsed: decimal.NewFromFloat(20000),
	}
	
	// Update position
	if err := pm.UpdatePosition(pos); err != nil {
		log.Fatal("Failed to update position:", err)
	}
	
	fmt.Printf("✓ Created position: %s %s on %s\n", 
		pos.Quantity.String(), pos.Symbol, pos.Exchange)
	fmt.Printf("  Entry price: $%s\n", pos.EntryPrice.String())
	fmt.Printf("  Mark price: $%s\n", pos.MarkPrice.String())
	fmt.Printf("  Unrealized P&L: $%s\n", pos.UnrealizedPnL.String())
	fmt.Printf("  P&L %%: %s%%\n", pos.PnLPercent.StringFixed(2))
	
	// Retrieve position
	retrieved, exists := pm.GetPosition("binance", "BTCUSDT")
	if !exists {
		log.Fatal("Failed to retrieve position")
	}
	
	fmt.Printf("\n✓ Retrieved position: %s\n", retrieved.Symbol)
	fmt.Printf("  Position value: $%s\n", retrieved.PositionValue.String())
}

func testMultiExchangePositions(pm *position.PositionManager) {
	fmt.Println("\n=== Testing Multi-Exchange Positions ===")
	
	// Add positions on different exchanges
	positions := []struct {
		exchange string
		symbol   string
		quantity float64
		entry    float64
		mark     float64
		side     string
		leverage int
	}{
		{"binance", "ETHUSDT", 10, 2000, 2200, "LONG", 1},
		{"okx", "BTCUSDT", 0.3, 41000, 42000, "LONG", 1},
		{"okx", "ETHUSDT", 5, 2100, 2200, "LONG", 1},
		{"binance", "BNBUSDT", 50, 310, 300, "SHORT", 1},
		{"bybit", "BTCUSDT", 0.2, 40500, 42000, "LONG", 5}, // Futures with leverage
	}
	
	for _, p := range positions {
		pos := &position.Position{
			Symbol:     p.symbol,
			Exchange:   p.exchange,
			Market:     "spot",
			Side:       p.side,
			Quantity:   decimal.NewFromFloat(p.quantity),
			EntryPrice: decimal.NewFromFloat(p.entry),
			MarkPrice:  decimal.NewFromFloat(p.mark),
			Leverage:   p.leverage,
			MarginUsed: decimal.NewFromFloat(p.entry * p.quantity / float64(p.leverage)),
		}
		
		if p.leverage > 1 {
			pos.Market = "futures"
		}
		
		if err := pm.UpdatePosition(pos); err != nil {
			log.Printf("Failed to update position: %v", err)
			continue
		}
		
		fmt.Printf("✓ Added %s position: %s %s @ $%g (P&L: $%s)\n",
			p.exchange, decimal.NewFromFloat(p.quantity).String(), p.symbol, 
			p.entry, pos.UnrealizedPnL.StringFixed(2))
	}
	
	// Get positions by exchange
	fmt.Println("\n✓ Positions by exchange:")
	for _, exchange := range []string{"binance", "okx", "bybit"} {
		positions := pm.GetPositionsByExchange(exchange)
		fmt.Printf("  %s: %d positions\n", exchange, len(positions))
		for _, pos := range positions {
			fmt.Printf("    - %s: %s (P&L: $%s)\n", 
				pos.Symbol, pos.Quantity.String(), pos.UnrealizedPnL.StringFixed(2))
		}
	}
}

func testAggregatedPositions(pm *position.PositionManager) {
	fmt.Println("\n=== Testing Aggregated Positions ===")
	
	aggregated := pm.GetAggregatedPositions()
	
	fmt.Printf("\n✓ Aggregated positions across exchanges:\n")
	for symbol, agg := range aggregated {
		fmt.Printf("\n  %s:\n", symbol)
		fmt.Printf("    Total quantity: %s\n", agg.TotalQuantity.String())
		fmt.Printf("    Average entry: $%s\n", agg.AvgEntryPrice.StringFixed(2))
		fmt.Printf("    Total value: $%s\n", agg.TotalValue.StringFixed(2))
		fmt.Printf("    Total P&L: $%s\n", agg.TotalPnL.StringFixed(2))
		fmt.Printf("    Positions: %d exchanges\n", len(agg.Positions))
		for _, pos := range agg.Positions {
			fmt.Printf("      - %s: %s @ $%s\n", 
				pos.Exchange, pos.Quantity.String(), pos.EntryPrice.StringFixed(2))
		}
	}
}

func testPnLCalculations(pm *position.PositionManager) {
	fmt.Println("\n=== Testing P&L Calculations ===")
	
	// Calculate total P&L
	unrealizedTotal, realizedTotal := pm.CalculateTotalPnL()
	fmt.Printf("\n✓ Total P&L across all positions:\n")
	fmt.Printf("  Unrealized P&L: $%s\n", unrealizedTotal.StringFixed(2))
	fmt.Printf("  Realized P&L: $%s\n", realizedTotal.StringFixed(2))
	fmt.Printf("  Total P&L: $%s\n", unrealizedTotal.Add(realizedTotal).StringFixed(2))
	
	// Calculate P&L by exchange
	fmt.Printf("\n✓ P&L by exchange:\n")
	for _, exchange := range []string{"binance", "okx", "bybit"} {
		unrealized, realized := pm.CalculateExchangePnL(exchange)
		total := unrealized.Add(realized)
		if !total.IsZero() {
			fmt.Printf("  %s: Unrealized: $%s, Realized: $%s, Total: $%s\n",
				exchange, unrealized.StringFixed(2), realized.StringFixed(2), total.StringFixed(2))
		}
	}
	
	// Update mark prices to simulate price movement
	fmt.Printf("\n✓ Simulating price movements...\n")
	pm.UpdateMarkPrice("binance", "BTCUSDT", decimal.NewFromFloat(43000))
	pm.UpdateMarkPrice("binance", "ETHUSDT", decimal.NewFromFloat(2250))
	pm.UpdateMarkPrice("okx", "BTCUSDT", decimal.NewFromFloat(43000))
	
	// Recalculate P&L
	unrealizedNew, _ := pm.CalculateTotalPnL()
	pnlChange := unrealizedNew.Sub(unrealizedTotal)
	
	fmt.Printf("\n✓ After price updates:\n")
	fmt.Printf("  New unrealized P&L: $%s\n", unrealizedNew.StringFixed(2))
	fmt.Printf("  P&L change: $%s\n", pnlChange.StringFixed(2))
}

func testConcurrentUpdates(pm *position.PositionManager) {
	fmt.Println("\n=== Testing Concurrent Updates ===")
	
	// Simulate concurrent position updates
	var wg sync.WaitGroup
	symbols := []string{"BTCUSDT", "ETHUSDT", "BNBUSDT", "SOLUSDT", "ADAUSDT"}
	exchanges := []string{"binance", "okx", "bybit"}
	
	updateCount := 0
	start := time.Now()
	
	// Launch concurrent updaters
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			for j := 0; j < 100; j++ {
				symbol := symbols[j%len(symbols)]
				exchange := exchanges[j%len(exchanges)]
				
				pos := &position.Position{
					Symbol:     symbol,
					Exchange:   exchange,
					Market:     "spot",
					Side:       "LONG",
					Quantity:   decimal.NewFromFloat(float64(id+1) * 0.1),
					EntryPrice: decimal.NewFromFloat(40000 + float64(j)),
					MarkPrice:  decimal.NewFromFloat(40100 + float64(j)),
					Leverage:   1,
					MarginUsed: decimal.NewFromFloat(4000),
				}
				
				if err := pm.UpdatePosition(pos); err != nil {
					log.Printf("Goroutine %d: Failed to update: %v", id, err)
				}
				updateCount++
			}
		}(i)
	}
	
	wg.Wait()
	duration := time.Since(start)
	
	fmt.Printf("\n✓ Concurrent update test completed:\n")
	fmt.Printf("  Goroutines: 10\n")
	fmt.Printf("  Updates: %d\n", updateCount)
	fmt.Printf("  Duration: %v\n", duration)
	fmt.Printf("  Updates/sec: %.0f\n", float64(updateCount)/duration.Seconds())
	
	// Verify positions
	allPositions := pm.GetAllPositions()
	fmt.Printf("  Total positions tracked: %d\n", len(allPositions))
}

func testSnapshot(pm *position.PositionManager) {
	fmt.Println("\n=== Testing Snapshot Functionality ===")
	
	// Save snapshot
	if err := pm.SaveSnapshot(); err != nil {
		log.Printf("Failed to save snapshot: %v", err)
	} else {
		fmt.Println("✓ Snapshot saved successfully")
	}
	
	// List snapshot files
	snapshotDir := "./data/snapshots"
	var snapshotFiles []string
	
	err := filepath.Walk(snapshotDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if filepath.Ext(path) == ".json" {
			snapshotFiles = append(snapshotFiles, path)
		}
		return nil
	})
	
	if err == nil && len(snapshotFiles) > 0 {
		fmt.Printf("\n✓ Found %d snapshot files:\n", len(snapshotFiles))
		for i, file := range snapshotFiles {
			if i < 5 { // Show only first 5
				info, _ := os.Stat(file)
				fmt.Printf("  - %s (size: %d bytes)\n", 
					filepath.Base(file), info.Size())
			}
		}
		if len(snapshotFiles) > 5 {
			fmt.Printf("  ... and %d more\n", len(snapshotFiles)-5)
		}
	}
}

func showMetrics(pm *position.PositionManager) {
	fmt.Println("\n=== Position Manager Metrics ===")
	
	metrics := pm.GetRiskMetrics()
	
	fmt.Printf("\nPosition Summary:\n")
	fmt.Printf("  Total positions: %v\n", metrics["position_count"])
	fmt.Printf("  Total value: $%s\n", metrics["total_value"])
	fmt.Printf("  Total margin used: $%s\n", metrics["total_margin_used"])
	fmt.Printf("  Max leverage: %sx\n", metrics["max_leverage"])
	
	fmt.Printf("\nP&L Summary:\n")
	fmt.Printf("  Unrealized P&L: $%s\n", metrics["unrealized_pnl"])
	fmt.Printf("  Realized P&L: $%s\n", metrics["realized_pnl"])
	fmt.Printf("  Total P&L: $%s\n", metrics["total_pnl"])
	
	fmt.Printf("\nPerformance:\n")
	fmt.Printf("  Updates count: %v\n", metrics["updates_count"])
	fmt.Printf("  Reads count: %v\n", metrics["reads_count"])
	fmt.Printf("  Average calc time: %.2f μs\n", metrics["avg_calc_time_us"])
}