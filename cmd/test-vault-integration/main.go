package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mExOms/internal/account"
	"github.com/mExOms/internal/exchange"
	"github.com/mExOms/pkg/types"
	"github.com/mExOms/pkg/vault"
)

func main() {
	fmt.Println("=== Testing Vault Integration with Binance ===")

	// Test 1: Direct Vault Access
	fmt.Println("\n1. Testing direct Vault access...")
	testVaultAccess()

	// Test 2: Binance Spot Connection
	fmt.Println("\n2. Testing Binance Spot connection with Vault credentials...")
	testBinanceSpotConnection()

	// Test 3: Account Balance
	fmt.Println("\n3. Testing account balance retrieval...")
	testAccountBalance()
}

func testVaultAccess() {
	// Create Vault client
	vaultClient, err := vault.NewClient(vault.Config{})
	if err != nil {
		log.Fatalf("Failed to create vault client: %v", err)
	}

	// Get Binance Spot keys
	keys, err := vaultClient.GetExchangeKeys("binance", "spot")
	if err != nil {
		log.Fatalf("Failed to get API keys from Vault: %v", err)
	}

	fmt.Printf("✓ Successfully retrieved API keys from Vault\n")
	
	// Check if keys exist
	if apiKey, ok := keys["api_key"]; ok && len(apiKey) > 0 {
		fmt.Printf("✓ API Key found (length: %d)\n", len(apiKey))
	} else {
		fmt.Println("✗ API Key not found")
	}

	if secretKey, ok := keys["secret_key"]; ok && len(secretKey) > 0 {
		fmt.Printf("✓ Secret Key found (length: %d)\n", len(secretKey))
	} else {
		fmt.Println("✗ Secret Key not found")
	}
}

func testBinanceSpotConnection() {
	// Create account manager
	accountManager := account.NewManager()
	
	// Add a test account
	testAccount := &types.Account{
		ID:       "test-account-1",
		Name:     "Test Binance Account",
		Exchange: types.ExchangeBinanceSpot,
		Type:     types.AccountTypeSpot,
		Status:   types.AccountStatusActive,
	}
	
	if err := accountManager.AddAccount(testAccount); err != nil {
		log.Printf("Warning: Failed to add account: %v", err)
	}
	
	// Create exchange factory
	factory := exchange.NewFactory(accountManager)
	
	// Create Binance Spot exchange
	binanceSpot, err := factory.CreateExchange(types.ExchangeBinanceSpot)
	if err != nil {
		log.Fatalf("Failed to create Binance Spot exchange: %v", err)
	}

	// Connect to exchange
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := binanceSpot.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to Binance: %v", err)
	}

	fmt.Println("✓ Successfully connected to Binance Spot")
}

func testAccountBalance() {
	// Create account manager
	accountManager := account.NewManager()
	
	// Create exchange factory
	factory := exchange.NewFactory(accountManager)
	
	// Create and connect to Binance
	binanceSpot, err := factory.CreateExchange(types.ExchangeBinanceSpot)
	if err != nil {
		log.Fatalf("Failed to create exchange: %v", err)
	}

	ctx := context.Background()
	
	// Connect
	if err := binanceSpot.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}

	// Get balance
	balance, err := binanceSpot.GetBalance(ctx)
	if err != nil {
		log.Printf("Failed to get balance: %v", err)
		fmt.Println("Note: This might fail if the API key doesn't have proper permissions")
		return
	}

	fmt.Println("✓ Successfully retrieved account balance")
	
	// Display some balances
	displayed := 0
	for asset, amount := range balance.Balances {
		if amount.Free > 0 || amount.Locked > 0 {
			fmt.Printf("  %s: Free=%.8f, Locked=%.8f\n", asset, amount.Free, amount.Locked)
			displayed++
			if displayed >= 5 {
				fmt.Println("  ... (showing first 5 non-zero balances)")
				break
			}
		}
	}
	
	if displayed == 0 {
		fmt.Println("  No non-zero balances found")
	}
}