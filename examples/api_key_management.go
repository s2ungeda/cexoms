package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mExOms/internal/account"
	"github.com/mExOms/pkg/security"
	"github.com/mExOms/pkg/types"
)

func main() {
	// Initialize Vault configuration
	vaultConfig := &security.VaultConfig{
		Address:        "http://localhost:8200",
		Token:          "dev-token", // In production, use environment variable
		MountPath:      "secret",
		TTL:            5 * time.Minute,
		RotationPeriod: 30 * 24 * time.Hour,
		RetryAttempts:  3,
		RetryDelay:     1 * time.Second,
	}

	// Create Vault manager
	vaultManager, err := security.NewVaultManager(vaultConfig)
	if err != nil {
		log.Fatalf("Failed to create vault manager: %v", err)
	}

	// Create API key provider
	keyProvider := account.NewAPIKeyProvider(vaultManager)

	// Create account manager
	accountConfig := &account.Config{
		DataDir:          "/data/accounts",
		SnapshotInterval: 1 * time.Hour,
		MetricsRetention: 24 * time.Hour,
	}

	accountManager, err := account.NewManager(accountConfig, keyProvider)
	if err != nil {
		log.Fatalf("Failed to create account manager: %v", err)
	}

	// Start key rotation service
	accountManager.StartKeyRotation()

	// Example 1: Create accounts with different permissions
	fmt.Println("=== Creating Accounts ===")
	
	// Main account with full permissions
	mainAccount := &types.Account{
		ID:             "main",
		Name:           "Main Trading Account",
		Exchange:       "binance",
		Market:         types.MarketTypeSpot,
		Type:           types.AccountTypeMain,
		SpotEnabled:    true,
		FuturesEnabled: false,
		MaxBalanceUSDT: decimal.NewFromInt(100000),
		Strategy:       "",
	}
	
	if err := accountManager.CreateAccount(mainAccount); err != nil {
		log.Printf("Failed to create main account: %v", err)
	}

	// Sub-account for arbitrage
	arbAccount := &types.Account{
		ID:             "sub_spot_arb",
		Name:           "Arbitrage Account",
		Exchange:       "binance",
		Market:         types.MarketTypeSpot,
		Type:           types.AccountTypeSub,
		SpotEnabled:    true,
		FuturesEnabled: false,
		MaxBalanceUSDT: decimal.NewFromInt(10000),
		Strategy:       "arbitrage",
		ParentID:       "main",
	}
	
	if err := accountManager.CreateAccount(arbAccount); err != nil {
		log.Printf("Failed to create arbitrage account: %v", err)
	}

	// Market making account
	mmAccount := &types.Account{
		ID:             "sub_market_making",
		Name:           "Market Making Account",
		Exchange:       "binance",
		Market:         types.MarketTypeSpot,
		Type:           types.AccountTypeSub,
		SpotEnabled:    true,
		FuturesEnabled: false,
		MaxBalanceUSDT: decimal.NewFromInt(20000),
		Strategy:       "market_making",
		ParentID:       "main",
	}
	
	if err := accountManager.CreateAccount(mmAccount); err != nil {
		log.Printf("Failed to create market making account: %v", err)
	}

	// Example 2: Store API keys for accounts
	fmt.Println("\n=== Storing API Keys ===")
	
	// Store main account keys
	mainCreds := &types.APICredentials{
		APIKey:    "binance_main_api_key",
		SecretKey: "binance_main_secret_key",
	}
	
	if err := accountManager.StoreAPIKeys("main", mainCreds); err != nil {
		log.Printf("Failed to store main account keys: %v", err)
	} else {
		fmt.Println("Stored API keys for main account")
	}

	// Set custom permissions for main account
	mainPermissions := []string{"read", "trade", "transfer"}
	if err := accountManager.SetAccountPermissions("main", mainPermissions); err != nil {
		log.Printf("Failed to set main account permissions: %v", err)
	}

	// Store sub-account keys
	arbCreds := &types.APICredentials{
		APIKey:    "binance_arb_api_key",
		SecretKey: "binance_arb_secret_key",
	}
	
	if err := accountManager.StoreAPIKeys("sub_spot_arb", arbCreds); err != nil {
		log.Printf("Failed to store arbitrage account keys: %v", err)
	} else {
		fmt.Println("Stored API keys for arbitrage account")
	}

	// Example 3: Retrieve API keys
	fmt.Println("\n=== Retrieving API Keys ===")
	
	keys, err := accountManager.GetAPIKeys("main")
	if err != nil {
		log.Printf("Failed to get main account keys: %v", err)
	} else {
		fmt.Printf("Retrieved keys for main account: API Key = %s...\n", keys.APIKey[:10])
	}

	// Example 4: Select best account for operations
	fmt.Println("\n=== Account Selection for Operations ===")
	
	// Find best account for trading
	tradeAccount, err := accountManager.GetBestAccountForOperation("binance", types.MarketTypeSpot, "trade")
	if err != nil {
		log.Printf("Failed to find account for trading: %v", err)
	} else {
		fmt.Printf("Best account for trading: %s\n", tradeAccount.ID)
	}

	// Find account that can transfer (only main)
	transferAccount, err := accountManager.GetBestAccountForOperation("binance", types.MarketTypeSpot, "transfer")
	if err != nil {
		log.Printf("Failed to find account for transfer: %v", err)
	} else {
		fmt.Printf("Best account for transfer: %s\n", transferAccount.ID)
	}

	// Example 5: Validate operations
	fmt.Println("\n=== Operation Validation ===")
	
	// Check if sub-account can transfer (should be false)
	canTransfer := accountManager.ValidateAccountOperation("sub_spot_arb", "transfer")
	fmt.Printf("Can sub_spot_arb transfer? %v\n", canTransfer)

	// Check if sub-account can trade (should be true)
	canTrade := accountManager.ValidateAccountOperation("sub_spot_arb", "trade")
	fmt.Printf("Can sub_spot_arb trade? %v\n", canTrade)

	// Example 6: Account selection with requirements
	fmt.Println("\n=== Account Selection with Requirements ===")
	
	requirements := types.AccountRequirements{
		Market:         types.MarketTypeSpot,
		MinBalance:     decimal.NewFromInt(5000),
		RequiredWeight: 10,
		OrderSize:      decimal.NewFromInt(1000),
	}

	selectedAccount, err := accountManager.SelectAccount("arbitrage", requirements)
	if err != nil {
		log.Printf("Failed to select account: %v", err)
	} else {
		fmt.Printf("Selected account: %s for arbitrage strategy\n", selectedAccount.ID)
	}

	// Example 7: Manual key rotation
	fmt.Println("\n=== Manual Key Rotation ===")
	
	if err := accountManager.RotateAPIKeys("sub_spot_arb"); err != nil {
		log.Printf("Failed to rotate keys: %v", err)
	} else {
		fmt.Println("Successfully triggered key rotation for sub_spot_arb")
	}

	// Example 8: Check rotation status
	fmt.Println("\n=== Key Rotation Status ===")
	
	status := accountManager.GetKeyRotationStatus()
	fmt.Printf("Rotation status: %+v\n", status)

	// Example 9: Rate limit tracking
	fmt.Println("\n=== Rate Limit Management ===")
	
	// Simulate API calls
	accountManager.UpdateRateLimit("sub_spot_arb", 1)  // 1 weight for balance query
	accountManager.UpdateRateLimit("sub_spot_arb", 10) // 10 weight for order placement
	accountManager.UpdateRateLimit("sub_spot_arb", 5)  // 5 weight for order status

	// Check available rate limit
	rateLimitReq := types.AccountRequirements{
		Market:         types.MarketTypeSpot,
		RequiredWeight: 50, // Require 50 weight
	}

	availableAccount, err := accountManager.SelectAccount("", rateLimitReq)
	if err != nil {
		log.Printf("No account available with sufficient rate limit: %v", err)
	} else {
		fmt.Printf("Account %s has sufficient rate limit\n", availableAccount.ID)
	}

	fmt.Println("\n=== Multi-Account API Key Management Demo Complete ===")
}