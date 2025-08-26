package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mExOms/internal/transfer"
	"github.com/mExOms/pkg/cache"
	"github.com/mExOms/pkg/storage"
	"github.com/mExOms/pkg/types"
)

func main() {
	ctx := context.Background()

	// Initialize storage
	accountStorage, err := storage.NewAccountStorage("./data")
	if err != nil {
		log.Fatal(err)
	}
	defer accountStorage.Close()

	// Initialize account cache
	accountCache := cache.NewAccountCache()

	// Mock exchanges (in real implementation, use actual exchange clients)
	exchanges := make(map[string]types.Exchange)
	
	// Create transfer manager
	transferManager := transfer.NewManager(exchanges, accountCache, accountStorage)
	
	// Start transfer manager
	if err := transferManager.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer transferManager.Stop()

	// Set rebalance configuration
	rebalanceConfig := transfer.RebalanceConfig{
		Enabled:  true,
		Schedule: "0 0 * * *", // Daily at midnight
		MinMainBalance: map[string]float64{
			"USDT": 10000.0,
			"BTC":  0.1,
			"ETH":  1.0,
		},
		MaxSubBalance: map[string]float64{
			"USDT": 50000.0,
			"BTC":  1.0,
			"ETH":  10.0,
		},
		TargetRatios: map[string]map[string]float64{
			"arbitrage": {
				"USDT": 0.3,
				"BTC":  0.1,
				"ETH":  0.1,
			},
			"market_making": {
				"USDT": 0.4,
				"BTC":  0.15,
				"ETH":  0.15,
			},
			"trend_following": {
				"USDT": 0.3,
				"BTC":  0.05,
				"ETH":  0.05,
			},
		},
	}
	transferManager.SetRebalanceConfig(rebalanceConfig)

	// Example 1: Transfer USDT from main to sub account
	fmt.Println("Example 1: Transfer USDT from main to sub account")
	transferReq1 := &transfer.TransferRequest{
		Exchange:    "binance",
		FromAccount: "main",
		ToAccount:   "sub_arbitrage",
		Asset:       "USDT",
		Amount:      10000.0,
		Type:        transfer.TransferTypeMainToSub,
		Reason:      "Initial funding for arbitrage strategy",
	}
	
	if err := transferManager.RequestTransfer(transferReq1); err != nil {
		log.Printf("Failed to request transfer: %v", err)
	} else {
		fmt.Printf("Transfer requested: %s\n", transferReq1.ID)
	}

	// Wait for transfer to complete
	time.Sleep(2 * time.Second)

	// Check transfer status
	status, err := transferManager.GetTransferStatus(transferReq1.ID)
	if err != nil {
		log.Printf("Failed to get transfer status: %v", err)
	} else {
		fmt.Printf("Transfer status: %s\n", status.Status)
	}

	// Example 2: Transfer profits back to main account
	fmt.Println("\nExample 2: Transfer profits back to main account")
	transferReq2 := &transfer.TransferRequest{
		Exchange:    "binance",
		FromAccount: "sub_arbitrage",
		ToAccount:   "main",
		Asset:       "USDT",
		Amount:      500.0,
		Type:        transfer.TransferTypeSubToMain,
		Reason:      "Profit collection",
	}
	
	if err := transferManager.RequestTransfer(transferReq2); err != nil {
		log.Printf("Failed to request transfer: %v", err)
	}

	// Example 3: Transfer between sub accounts
	fmt.Println("\nExample 3: Transfer between sub accounts")
	transferReq3 := &transfer.TransferRequest{
		Exchange:    "binance",
		FromAccount: "sub_arbitrage",
		ToAccount:   "sub_market_making",
		Asset:       "BTC",
		Amount:      0.05,
		Type:        transfer.TransferTypeSubToSub,
		Reason:      "Rebalance for market making",
	}
	
	if err := transferManager.RequestTransfer(transferReq3); err != nil {
		log.Printf("Failed to request transfer: %v", err)
	}

	// Get transfer history
	fmt.Println("\nTransfer History:")
	history := transferManager.GetTransferHistory("binance", "", 10)
	for _, transfer := range history {
		fmt.Printf("- %s: %s -> %s, %.4f %s, Status: %s\n",
			transfer.CreatedAt.Format("15:04:05"),
			transfer.FromAccount,
			transfer.ToAccount,
			transfer.Amount,
			transfer.Asset,
			transfer.Status,
		)
	}

	// Get rebalance report
	fmt.Println("\nRebalance Report:")
	report := transferManager.GetRebalanceReport()
	fmt.Printf("- Enabled: %v\n", report["enabled"])
	fmt.Printf("- Success Count: %v\n", report["success_count"])
	fmt.Printf("- Failure Count: %v\n", report["failure_count"])
	fmt.Printf("- Total Volume: %v\n", report["total_volume"])

	// Create scheduler for automated tasks
	scheduler := transfer.NewScheduler(transferManager)
	if err := scheduler.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer scheduler.Stop()

	// Get scheduled tasks
	fmt.Println("\nScheduled Tasks:")
	tasks := scheduler.GetAllTasks()
	for id, task := range tasks {
		fmt.Printf("- %s: %s (Enabled: %v, Next Run: %s)\n",
			id,
			task.Name,
			task.Enabled,
			task.NextRun.Format("2006-01-02 15:04:05"),
		)
	}

	// Enable hourly rebalancing
	if err := scheduler.EnableTask("hourly_rebalance"); err != nil {
		log.Printf("Failed to enable hourly rebalance: %v", err)
	}

	// Keep running for demonstration
	fmt.Println("\nPress Ctrl+C to exit...")
	select {}
}