package main

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/adshao/go-binance/v2/futures"
	"github.com/mExOms/pkg/vault"
)

func main() {
	fmt.Println("=== Binance Futures Balance Check ===")

	// Get credentials from Vault
	vaultClient, err := vault.NewClient(vault.Config{})
	if err != nil {
		log.Fatalf("Failed to create vault client: %v", err)
	}

	// Try to get futures keys first, fallback to spot keys
	keys, err := vaultClient.GetExchangeKeys("binance", "futures")
	if err != nil {
		// Try spot keys as fallback (same API keys work for both)
		keys, err = vaultClient.GetExchangeKeys("binance", "spot")
		if err != nil {
			log.Fatalf("Failed to get API keys from Vault: %v", err)
		}
		fmt.Println("Note: Using spot API keys for futures")
	}

	apiKey := keys["api_key"]
	secretKey := keys["secret_key"]
	
	// Create Binance Futures client
	client := futures.NewClient(apiKey, secretKey)
	ctx := context.Background()

	// Get account info
	account, err := client.NewGetAccountService().Do(ctx)
	if err != nil {
		log.Fatalf("Failed to get futures account: %v", err)
	}

	fmt.Println("\n=== Futures Account Balance ===")
	fmt.Printf("Total Wallet Balance: %s USDT\n", account.TotalWalletBalance)
	fmt.Printf("Total Unrealized PnL: %s USDT\n", account.TotalUnrealizedProfit)
	fmt.Printf("Total Margin Balance: %s USDT\n", account.TotalMarginBalance)
	fmt.Printf("Available Balance:    %s USDT\n", account.AvailableBalance)
	fmt.Printf("Total Position Value: %s USDT\n", account.TotalPositionInitialMargin)
	
	fmt.Println("\nAsset Balances:")
	for _, asset := range account.Assets {
		walletBalance, _ := strconv.ParseFloat(asset.WalletBalance, 64)
		unrealizedProfit, _ := strconv.ParseFloat(asset.UnrealizedProfit, 64)
		
		if walletBalance > 0 || unrealizedProfit != 0 {
			fmt.Printf("  %-10s Wallet: %15.8f  Unrealized PnL: %15.8f\n",
				asset.Asset, walletBalance, unrealizedProfit)
		}
	}
	
	// Show positions if any
	fmt.Println("\nOpen Positions:")
	hasPositions := false
	for _, position := range account.Positions {
		posAmt, _ := strconv.ParseFloat(position.PositionAmt, 64)
		if posAmt != 0 {
			hasPositions = true
			unrealizedProfit, _ := strconv.ParseFloat(position.UnrealizedProfit, 64)
			fmt.Printf("  %-15s Amount: %10.4f  PnL: %10.4f USDT  Leverage: %sx\n",
				position.Symbol, posAmt, unrealizedProfit, position.Leverage)
		}
	}
	if !hasPositions {
		fmt.Println("  No open positions")
	}
}