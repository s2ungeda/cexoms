package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/mExOms/services/binance/spot"
	"github.com/shopspring/decimal"
)

func main() {
	// Test API keys (use environment variables in production)
	apiKey := os.Getenv("BINANCE_API_KEY")
	apiSecret := os.Getenv("BINANCE_API_SECRET")
	
	// For testing without API keys
	if apiKey == "" || apiSecret == "" {
		fmt.Println("⚠️  No API keys provided, running in demo mode")
		apiKey = "test_key"
		apiSecret = "test_secret"
	}
	
	// Create Binance Spot client
	client, err := spot.NewBinanceSpot(apiKey, apiSecret, true) // testnet
	if err != nil {
		log.Fatal("Failed to create Binance client:", err)
	}
	defer client.Close()
	
	fmt.Println("✓ Binance Spot client created")
	
	// Test basic functionality
	testBasicFunctions(client)
	
	// Test WebSocket subscriptions
	testWebSocketStreams(client)
	
	// Wait for interrupt signal
	fmt.Println("\n✓ Press Ctrl+C to exit...")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan
	
	fmt.Println("\n✓ Shutting down...")
}

func testBasicFunctions(client *spot.BinanceSpot) {
	fmt.Println("\n=== Testing Basic Functions ===")
	
	// Test connection
	fmt.Println("✓ Client created")
	
	// Test with BTCUSDT
	symbol := "BTCUSDT"
	
	// Get exchange info (this will fail without real API keys)
	info, err := client.GetExchangeInfo()
	if err != nil {
		fmt.Printf("✗ Failed to get exchange info: %v (expected without API keys)\n", err)
	} else {
		fmt.Printf("✓ Exchange info retrieved: %d trading symbols\n", len(info.Symbols))
	}
	
	// Get ticker
	// GetTicker not implemented yet
	fmt.Printf("✓ Symbol selected: %s\n", symbol)
}

func testWebSocketStreams(client *spot.BinanceSpot) {
	fmt.Println("\n=== Testing WebSocket Streams ===")
	
	symbol := "BTCUSDT"
	
	// Subscribe to ticker
	if err := client.SubscribeTicker(symbol); err != nil {
		fmt.Printf("✗ Failed to subscribe to ticker: %v\n", err)
	} else {
		fmt.Printf("✓ Subscribed to %s ticker\n", symbol)
	}
	
	// Subscribe to trades
	if err := client.SubscribeTrades(symbol); err != nil {
		fmt.Printf("✗ Failed to subscribe to trades: %v\n", err)
	} else {
		fmt.Printf("✓ Subscribed to %s trades\n", symbol)
	}
	
	// Subscribe to order book
	if err := client.SubscribeOrderBook(symbol, 20); err != nil {
		fmt.Printf("✗ Failed to subscribe to order book: %v\n", err)
	} else {
		fmt.Printf("✓ Subscribed to %s order book\n", symbol)
	}
	
	// Subscribe to klines
	if err := client.SubscribeKline(symbol, "1m"); err != nil {
		fmt.Printf("✗ Failed to subscribe to klines: %v\n", err)
	} else {
		fmt.Printf("✓ Subscribed to %s 1m klines\n", symbol)
	}
}

func testOrderPlacement(client *spot.BinanceSpot) {
	fmt.Println("\n=== Testing Order Placement ===")
	
	symbol := "BTCUSDT"
	
	// Get current price for limit order
	// ticker, err := client.GetTicker(symbol)
	// if err != nil {
	// 	fmt.Printf("✗ Cannot test orders without ticker price\n")
	// 	return
	// }
	
	currentPrice := decimal.NewFromInt(40000) // Use fixed price for demo
	
	// Place a limit buy order at 90% of current price
	limitPrice := currentPrice.Mul(decimal.NewFromFloat(0.9))
	testOrder := &types.Order{
		Symbol:   symbol,
		Side:     types.OrderSideBuy,
		Type:     types.OrderTypeLimit,
		Price:    limitPrice,
		Quantity: decimal.NewFromFloat(0.001), // Small amount for testing
	}
	
	orderResp, err := client.CreateOrder(testOrder)
	if err != nil {
		fmt.Printf("✗ Failed to create test order: %v (expected without API keys)\n", err)
	} else {
		fmt.Printf("✓ Test order created: ID=%s, Status=%s\n", orderResp.OrderID, orderResp.Status)
		
		// Cancel the test order
		time.Sleep(2 * time.Second)
		if err := client.CancelOrder(context.Background(), symbol, orderResp.OrderID); err != nil {
			fmt.Printf("✗ Failed to cancel test order: %v\n", err)
		} else {
			fmt.Println("✓ Test order cancelled")
		}
	}
}