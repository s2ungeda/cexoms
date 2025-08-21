package main

import (
	"context"
	"fmt"
	"log"
	"os"

	binance "github.com/adshao/go-binance/v2"
	"github.com/mExOms/pkg/vault"
)

func main() {
	fmt.Println("=== Simple Vault Integration Test ===")

	// Step 1: Get credentials from Vault
	fmt.Println("\n1. Retrieving API keys from Vault...")
	
	vaultClient, err := vault.NewClient(vault.Config{})
	if err != nil {
		log.Fatalf("Failed to create vault client: %v", err)
	}

	// Get Binance Spot keys
	keys, err := vaultClient.GetExchangeKeys("binance", "spot")
	if err != nil {
		log.Fatalf("Failed to get API keys from Vault: %v", err)
	}

	apiKey, ok1 := keys["api_key"]
	secretKey, ok2 := keys["secret_key"]
	
	if !ok1 || !ok2 {
		log.Fatal("API keys not found in Vault")
	}

	fmt.Printf("✓ Retrieved API keys from Vault\n")
	fmt.Printf("  API Key length: %d\n", len(apiKey))
	fmt.Printf("  Secret Key length: %d\n", len(secretKey))

	// Step 2: Test Binance connection
	fmt.Println("\n2. Testing Binance API connection...")
	
	// Create Binance client with real credentials
	client := binance.NewClient(apiKey, secretKey)
	
	// Test with account info
	ctx := context.Background()
	account, err := client.NewGetAccountService().Do(ctx)
	if err != nil {
		fmt.Printf("✗ Failed to get account info: %v\n", err)
		fmt.Println("\nTrying server time instead (no auth required)...")
		
		// Test server time (doesn't require authentication)
		serverTime, err := client.NewServerTimeService().Do(ctx)
		if err != nil {
			log.Fatalf("Failed to get server time: %v", err)
		}
		fmt.Printf("✓ Server time: %d\n", serverTime)
		
		// Test with API key info
		apiKeyInfo, err := client.NewGetAPIKeyPermission().Do(ctx)
		if err != nil {
			fmt.Printf("✗ Failed to get API key permissions: %v\n", err)
			fmt.Println("\nPossible reasons:")
			fmt.Println("- API key/secret might be incorrect")
			fmt.Println("- API key might not have required permissions")
			fmt.Println("- IP restrictions might be in place")
			os.Exit(1)
		}
		
		fmt.Printf("✓ API Key permissions:\n")
		fmt.Printf("  Can Trade: %v\n", apiKeyInfo.EnableSpotAndMarginTrading)
		fmt.Printf("  Can Read: %v\n", apiKeyInfo.EnableReading)
		os.Exit(0)
	}

	fmt.Printf("✓ Successfully connected to Binance!\n")
	fmt.Printf("  Account Type: %s\n", account.AccountType)
	fmt.Printf("  Can Trade: %v\n", account.CanTrade)
	
	// Step 3: Show some balances
	fmt.Println("\n3. Account Balances:")
	count := 0
	for _, balance := range account.Balances {
		free := balance.Free
		locked := balance.Locked
		
		// Convert string to float for comparison
		var freeFloat, lockedFloat float64
		fmt.Sscanf(free, "%f", &freeFloat)
		fmt.Sscanf(locked, "%f", &lockedFloat)
		
		if freeFloat > 0 || lockedFloat > 0 {
			fmt.Printf("  %s: Free=%s, Locked=%s\n", balance.Asset, free, locked)
			count++
			if count >= 10 {
				fmt.Println("  ... (showing first 10 non-zero balances)")
				break
			}
		}
	}
	
	if count == 0 {
		fmt.Println("  No non-zero balances found")
	}

	fmt.Println("\n✓ Vault integration test completed successfully!")
}