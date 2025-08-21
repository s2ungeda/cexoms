package main

import (
	"fmt"
	"log"
	"time"

	"github.com/mExOms/internal/storage"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

func main() {
	// Storage configuration
	config := storage.StorageConfig{
		BasePath:           "./storage_data",
		MaxFileSize:        10 * 1024 * 1024, // 10MB
		RotationInterval:   24 * time.Hour,    // Daily rotation
		CompressionEnabled: true,
		RetentionDays:      30,
	}

	// Create storage manager
	manager, err := storage.NewManager(config)
	if err != nil {
		log.Fatal("Failed to create storage manager:", err)
	}
	defer manager.Close()

	// Example accounts
	accounts := []string{"binance_main", "binance_spot", "binance_futures"}

	// 1. Log some trading activity
	fmt.Println("=== Logging Trading Activity ===")
	for _, account := range accounts {
		for i := 0; i < 5; i++ {
			order := &types.Order{
				ID:       fmt.Sprintf("order_%s_%d", account, i),
				Symbol:   "BTC/USDT",
				Side:     types.OrderSideBuy,
				Type:     types.OrderTypeLimit,
				Price:    decimal.NewFromFloat(50000 + float64(i*100)),
				Quantity: decimal.NewFromFloat(0.1),
				Status:   types.OrderStatusFilled,
				Metadata: map[string]interface{}{
					"strategy": "momentum",
					"signal":   "bullish",
				},
			}

			err := manager.LogTrade(account, "binance", "BTC/USDT", "order_filled", order)
			if err != nil {
				log.Printf("Failed to log trade: %v", err)
			}
		}
	}

	// 2. Register snapshot handlers
	fmt.Println("\n=== Registering Snapshot Handlers ===")
	for _, account := range accounts {
		accountCopy := account // Capture loop variable
		manager.RegisterSnapshotHandler(accountCopy, func(acc string) (*storage.StateSnapshot, error) {
			return &storage.StateSnapshot{
				Timestamp: time.Now(),
				Account:   acc,
				Exchange:  "binance",
				Balances: map[string]decimal.Decimal{
					"USDT": decimal.NewFromFloat(10000),
					"BTC":  decimal.NewFromFloat(0.5),
					"ETH":  decimal.NewFromFloat(10),
				},
				Positions: []types.Position{
					{
						Symbol:        "BTC/USDT",
						Side:          types.Side("LONG"),
						Amount:        decimal.NewFromFloat(0.5),
						EntryPrice:    decimal.NewFromFloat(50000),
						MarkPrice:     decimal.NewFromFloat(51000),
						UnrealizedPnL: decimal.NewFromFloat(500),
					},
				},
				OpenOrders: []types.Order{},
				RiskMetrics: map[string]interface{}{
					"total_exposure":   "25000",
					"open_positions":   1,
					"current_drawdown": 0.02,
					"daily_pnl":        "500",
				},
			}, nil
		})
	}

	// 3. Take snapshots
	fmt.Println("\n=== Taking Snapshots ===")
	for _, account := range accounts {
		err := manager.TakeSnapshot(account)
		if err != nil {
			log.Printf("Failed to take snapshot for %s: %v", account, err)
		} else {
			fmt.Printf("Snapshot taken for %s\n", account)
		}
	}

	// 4. Log strategy execution
	fmt.Println("\n=== Logging Strategy Execution ===")
	positions := []storage.PositionDetail{
		{
			Symbol:        "ETH/USDT",
			Side:          types.Side("LONG"),
			Quantity:      decimal.NewFromFloat(10),
			EntryPrice:    decimal.NewFromFloat(3000),
			CurrentPrice:  decimal.NewFromFloat(3100),
			UnrealizedPnL: decimal.NewFromFloat(1000),
		},
	}

	performance := &storage.PerformanceMetrics{
		TotalPnL:      decimal.NewFromFloat(5000),
		WinRate:       0.65,
		SharpeRatio:   1.8,
		MaxDrawdown:   0.12,
		TotalTrades:   100,
		WinningTrades: 65,
		LosingTrades:  35,
	}

	err = manager.LogStrategy("momentum", accounts[0], "signal_generated", "BUY", 0.85, positions, performance)
	if err != nil {
		log.Printf("Failed to log strategy: %v", err)
	}

	// 5. Log inter-account transfer
	fmt.Println("\n=== Logging Inter-Account Transfer ===")
	err = manager.LogTransfer(
		accounts[0],
		accounts[1],
		"binance",
		"binance",
		"USDT",
		decimal.NewFromFloat(1000),
		decimal.NewFromFloat(0), // No fee for internal transfer
		"completed",
	)
	if err != nil {
		log.Printf("Failed to log transfer: %v", err)
	}

	// Flush all data
	manager.Flush()

	// 6. Query stored data
	fmt.Println("\n=== Querying Stored Data ===")
	
	// Get trading logs
	opts := storage.QueryOptions{
		Account:   accounts[0],
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now(),
	}

	tradingLogs, err := manager.GetTradingLogs(opts)
	if err != nil {
		log.Printf("Failed to get trading logs: %v", err)
	} else {
		fmt.Printf("Found %d trading logs for %s\n", len(tradingLogs), accounts[0])
	}

	// Get latest snapshot
	snapshot, err := manager.GetLatestSnapshot(accounts[0])
	if err != nil {
		log.Printf("Failed to get latest snapshot: %v", err)
	} else {
		fmt.Printf("Latest snapshot for %s:\n", accounts[0])
		fmt.Printf("  Timestamp: %s\n", snapshot.Timestamp.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Balances: USDT=%s, BTC=%s\n", 
			snapshot.Balances["USDT"], 
			snapshot.Balances["BTC"])
		fmt.Printf("  Positions: %d\n", len(snapshot.Positions))
	}

	// Get account summary
	summary, err := manager.GetAccountSummary(
		accounts[0],
		time.Now().Add(-time.Hour),
		time.Now(),
	)
	if err != nil {
		log.Printf("Failed to get account summary: %v", err)
	} else {
		fmt.Printf("\nAccount Summary for %s:\n", accounts[0])
		fmt.Printf("  Total Trades: %d\n", summary.TotalTrades)
		fmt.Printf("  Total Volume: %s\n", summary.TotalVolume)
		fmt.Printf("  Total Strategies: %d\n", summary.TotalStrategies)
		fmt.Printf("  Total Transfers: %d\n", summary.TotalTransfers)
	}

	// 7. Query examples
	fmt.Println("\n=== Query Examples ===")
	queryUtils := storage.NewQueryUtils(config.BasePath)
	examples := queryUtils.QueryExamples()
	
	fmt.Println("Example queries you can run on the stored data:")
	for i, example := range examples[:3] { // Show first 3 examples
		fmt.Printf("\n%d. %s\n", i+1, example.Description)
		if example.GrepCmd != "" {
			fmt.Printf("   Grep: %s\n", example.GrepCmd)
		}
		if example.JqCmd != "" {
			fmt.Printf("   Jq: %s\n", example.JqCmd)
		}
	}

	// 8. Storage statistics
	fmt.Println("\n=== Storage Statistics ===")
	cleaner := storage.NewCleaner(config)
	stats, err := cleaner.GetStorageStats()
	if err != nil {
		log.Printf("Failed to get storage stats: %v", err)
	} else {
		fmt.Printf("Total Files: %d\n", stats.TotalFiles)
		fmt.Printf("Total Size: %.2f MB\n", float64(stats.TotalSize)/(1024*1024))
		fmt.Printf("Accounts: %d\n", len(stats.Accounts))
		
		for account, accountStats := range stats.Accounts {
			fmt.Printf("\n  %s:\n", account)
			fmt.Printf("    Files: %d\n", accountStats.TotalFiles)
			fmt.Printf("    Size: %.2f KB\n", float64(accountStats.TotalSize)/1024)
		}
	}

	fmt.Println("\n=== Storage Example Complete ===")
	fmt.Printf("Data stored in: %s\n", config.BasePath)
	fmt.Println("You can now use grep/jq to query the JSONL files directly!")
}