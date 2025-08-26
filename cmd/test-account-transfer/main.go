package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mExOms/internal/account"
	"github.com/mExOms/internal/keymanager"
	"github.com/mExOms/internal/storage"
	"github.com/mExOms/pkg/types"
)

func main() {
	fmt.Println("=== Multi-Account Asset Transfer Test ===")
	
	// Initialize storage
	storageConfig := &storage.Config{
		BasePath:         "/tmp/mexoms/storage",
		RotationEnabled:  true,
		MaxFileSize:      100 * 1024 * 1024, // 100MB
		CompressionLevel: 6,
	}
	storageManager := storage.NewManager(storageConfig)
	defer storageManager.Close()
	
	// Initialize key manager
	keyManager := &keymanager.Manager{} // Mock for testing
	
	// Initialize account manager
	accountManager := account.NewManager(keyManager, storageManager, nil)
	defer accountManager.Stop()
	
	// Create test accounts
	ctx := context.Background()
	
	// Main account
	mainAccount, err := accountManager.CreateAccount(ctx, &types.CreateAccountRequest{
		Name:           "main",
		Type:           types.AccountTypeMain,
		Exchange:       "binance",
		Market:         types.MarketSpot,
		SpotEnabled:    true,
		FuturesEnabled: true,
		MaxBalance:     decimal.NewFromInt(1000000),
		Metadata: map[string]interface{}{
			"description": "Main trading account",
		},
	})
	if err != nil {
		log.Fatalf("Failed to create main account: %v", err)
	}
	fmt.Printf("Created main account: %s\n", mainAccount.ID)
	
	// Sub accounts for different strategies
	subAccounts := []struct {
		name     string
		strategy string
		maxBal   int64
	}{
		{"sub_arb", "arbitrage", 50000},
		{"sub_mm", "market_making", 30000},
		{"sub_trend", "trend_following", 20000},
	}
	
	for _, sub := range subAccounts {
		account, err := accountManager.CreateAccount(ctx, &types.CreateAccountRequest{
			Name:        sub.name,
			Type:        types.AccountTypeSub,
			Exchange:    "binance",
			Market:      types.MarketSpot,
			Parent:      mainAccount.ID,
			Strategy:    sub.strategy,
			SpotEnabled: true,
			MaxBalance:  decimal.NewFromInt(sub.maxBal),
			Metadata: map[string]interface{}{
				"strategy": sub.strategy,
			},
		})
		if err != nil {
			log.Fatalf("Failed to create sub account %s: %v", sub.name, err)
		}
		fmt.Printf("Created sub account: %s (%s)\n", account.ID, sub.strategy)
	}
	
	// Initialize transfer manager
	transferManager := account.NewTransferManager(accountManager)
	defer transferManager.Stop()
	
	// Set mock balances for testing
	accountManager.UpdateBalance(mainAccount.ID, &types.AccountBalance{
		AccountID:  mainAccount.ID,
		Exchange:   "binance",
		TotalUSDT:  decimal.NewFromInt(500000),
		Available:  decimal.NewFromInt(450000),
		Locked:     decimal.NewFromInt(50000),
		UpdateTime: time.Now(),
	})
	
	// Set sub account balances
	subBalances := []struct {
		name  string
		total int64
	}{
		{"sub_arb", 25000},
		{"sub_mm", 15000},
		{"sub_trend", 5000},
	}
	
	accounts, _ := accountManager.ListAccounts(types.AccountFilter{})
	for _, acc := range accounts {
		for _, sub := range subBalances {
			if acc.Name == sub.name {
				accountManager.UpdateBalance(acc.ID, &types.AccountBalance{
					AccountID:  acc.ID,
					Exchange:   "binance",
					TotalUSDT:  decimal.NewFromInt(sub.total),
					Available:  decimal.NewFromInt(sub.total),
					UpdateTime: time.Now(),
				})
			}
		}
	}
	
	// Display current balances
	fmt.Println("\n=== Current Balances ===")
	displayBalances(accountManager)
	
	// Test 1: Manual transfer
	fmt.Println("\n=== Test 1: Manual Transfer ===")
	fmt.Println("Transferring 10,000 USDT from main to sub_arb...")
	
	transfer1, err := transferManager.RequestTransfer(ctx, &account.TransferRequest{
		FromAccount: "main",
		ToAccount:   "sub_arb",
		Asset:       "USDT",
		Amount:      decimal.NewFromInt(10000),
		Reason:      "Top up arbitrage account",
		Priority:    1,
	})
	if err != nil {
		log.Printf("Transfer request failed: %v", err)
	} else {
		fmt.Printf("Transfer requested: %s\n", transfer1.ID)
		
		// Execute transfer
		err = transferManager.ExecuteTransfer(ctx, transfer1.ID)
		if err != nil {
			log.Printf("Transfer execution failed: %v", err)
		} else {
			fmt.Println("Transfer executed successfully")
		}
	}
	
	// Test 2: Rebalancing rules
	fmt.Println("\n=== Test 2: Automatic Rebalancing ===")
	
	// Initialize rebalancer
	rebalanceConfig := &account.RebalanceConfig{
		Enabled:               true,
		DryRun:               false,
		MinRebalanceAmount:    decimal.NewFromInt(100),
		MaxDailyRebalances:    50,
		MainAccountMinBalance: decimal.NewFromInt(100000),
		MainAccountMaxBalance: decimal.NewFromInt(1000000),
		SubAccountMinBalance:  decimal.NewFromInt(5000),
		SubAccountMaxBalance:  decimal.NewFromInt(50000),
		RebalanceThreshold:    0.1,
		EmergencyThreshold:    0.5,
		StrategyAllocations: map[string]decimal.Decimal{
			"arbitrage":       decimal.NewFromFloat(0.4),
			"market_making":   decimal.NewFromFloat(0.3),
			"trend_following": decimal.NewFromFloat(0.3),
		},
	}
	
	rebalancer := account.NewRebalancer(accountManager, transferManager, storageManager, rebalanceConfig)
	defer rebalancer.Stop()
	
	// Simulate low balance in trend following account
	fmt.Println("\nSimulating low balance in trend_following account...")
	for _, acc := range accounts {
		if acc.Name == "sub_trend" {
			accountManager.UpdateBalance(acc.ID, &types.AccountBalance{
				AccountID:  acc.ID,
				Exchange:   "binance",
				TotalUSDT:  decimal.NewFromInt(1000), // Below minimum
				Available:  decimal.NewFromInt(1000),
				UpdateTime: time.Now(),
			})
		}
	}
	
	// Run rebalancing
	fmt.Println("Running rebalance check...")
	err = rebalancer.ExecuteRule(ctx, "maintain_sub_minimum")
	if err != nil {
		log.Printf("Rebalance failed: %v", err)
	} else {
		fmt.Println("Rebalance completed")
	}
	
	// Display updated balances
	fmt.Println("\n=== Updated Balances ===")
	displayBalances(accountManager)
	
	// Test 3: Transfer history
	fmt.Println("\n=== Transfer History ===")
	history := transferManager.GetTransferHistory(10)
	for _, transfer := range history {
		fmt.Printf("- %s: %s -> %s, %s %s (%s)\n",
			transfer.RequestedAt.Format("15:04:05"),
			getAccountName(accountManager, transfer.FromAccount),
			getAccountName(accountManager, transfer.ToAccount),
			transfer.Amount.String(),
			transfer.Asset,
			transfer.Status,
		)
	}
	
	// Test 4: Account statistics
	fmt.Println("\n=== Account Statistics ===")
	stats := accountManager.GetStats()
	fmt.Printf("Total accounts: %d\n", stats["total_accounts"])
	fmt.Printf("Active accounts: %d\n", stats["active_accounts"])
	fmt.Printf("By exchange: %v\n", stats["by_exchange"])
	fmt.Printf("By type: %v\n", stats["by_type"])
	fmt.Printf("By strategy: %v\n", stats["by_strategy"])
	
	// Test 5: Transfer statistics
	fmt.Println("\n=== Transfer Statistics ===")
	transferStats := transferManager.GetStats()
	fmt.Printf("Pending transfers: %d\n", transferStats["pending_transfers"])
	fmt.Printf("Completed transfers: %d\n", transferStats["completed_transfers"])
	fmt.Printf("Daily limit: %s\n", transferStats["daily_limit"])
	fmt.Printf("Daily used: %s\n", transferStats["daily_used"])
	fmt.Printf("Daily remaining: %s\n", transferStats["daily_remaining"])
	
	// Test 6: Rebalancer statistics
	fmt.Println("\n=== Rebalancer Statistics ===")
	rebalanceStats := rebalancer.GetStats()
	fmt.Printf("Enabled: %v\n", rebalanceStats["enabled"])
	fmt.Printf("Total rules: %d\n", rebalanceStats["total_rules"])
	fmt.Printf("Active rules: %d\n", rebalanceStats["active_rules"])
	
	// Display rules
	fmt.Println("\n=== Rebalancing Rules ===")
	rules := rebalancer.GetRules()
	for _, rule := range rules {
		fmt.Printf("- %s: %s (Priority: %d, Enabled: %v)\n",
			rule.Name, rule.Description, rule.Priority, rule.Enabled)
	}
	
	fmt.Println("\n=== Test Completed ===")
}

// Helper functions

func displayBalances(manager *account.Manager) {
	accounts, _ := manager.ListAccounts(types.AccountFilter{})
	
	var totalBalance decimal.Decimal
	for _, acc := range accounts {
		balance, err := manager.GetBalance(acc.ID)
		if err != nil {
			fmt.Printf("- %s (%s): Balance unavailable\n", acc.Name, acc.Type)
			continue
		}
		
		fmt.Printf("- %s (%s): %s USDT", acc.Name, acc.Type, balance.TotalUSDT.String())
		if acc.Strategy != "" {
			fmt.Printf(" [%s]", acc.Strategy)
		}
		fmt.Println()
		
		totalBalance = totalBalance.Add(balance.TotalUSDT)
	}
	fmt.Printf("Total: %s USDT\n", totalBalance.String())
}

func getAccountName(manager *account.Manager, accountID string) string {
	account, err := manager.GetAccount(accountID)
	if err != nil {
		return accountID
	}
	return account.Name
}