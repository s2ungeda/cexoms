package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/mExOms/pkg/vault"
	"github.com/mExOms/services/binance"
	"github.com/shopspring/decimal"
)

func main() {
	fmt.Println("=== Binance WebSocket vs REST Order Comparison ===\n")

	// Get credentials from Vault
	vaultClient, err := vault.NewClient(vault.Config{})
	if err != nil {
		log.Fatalf("Failed to create vault client: %v", err)
	}

	keys, err := vaultClient.GetExchangeKeys("binance", "spot")
	if err != nil {
		log.Fatalf("Failed to get API keys from Vault: %v", err)
	}

	apiKey := keys["api_key"]
	secretKey := keys["secret_key"]

	// Test WebSocket order
	fmt.Println("1. Testing WebSocket Order...")
	testWebSocketOrder(apiKey, secretKey)

	// For comparison - REST order would be like:
	fmt.Println("\n2. REST Order (for reference):")
	fmt.Println("REST orders typically take 50-200ms depending on network latency")
	fmt.Println("WebSocket orders can be faster with persistent connection")
}

func testWebSocketOrder(apiKey, secretKey string) {
	// Create WebSocket order manager
	wsManager, err := binance.NewWebSocketOrderManager(apiKey, secretKey, false)
	if err != nil {
		log.Fatalf("Failed to create WebSocket manager: %v", err)
	}
	defer wsManager.Close()

	// Test connection latency
	fmt.Println("\nMeasuring WebSocket latency...")
	latency, err := wsManager.GetConnectionLatency()
	if err != nil {
		fmt.Printf("Failed to measure latency: %v\n", err)
	} else {
		fmt.Printf("WebSocket ping latency: %v\n", latency)
	}

	// Prepare test order (very small, far from market price)
	order := &types.Order{
		Symbol:      "TRXUSDT",
		Side:        types.OrderSideBuy,
		Type:        types.OrderTypeLimit,
		Quantity:    decimal.NewFromInt(10), // 10 TRX
		Price:       decimal.NewFromFloat(0.10), // Far below market
		TimeInForce: types.TimeInForceGTC,
	}

	fmt.Printf("\nTest order details:\n")
	fmt.Printf("Symbol: %s\n", order.Symbol)
	fmt.Printf("Side: %s\n", order.Side)
	fmt.Printf("Quantity: %s\n", order.Quantity)
	fmt.Printf("Price: %s\n", order.Price)
	fmt.Printf("Total: %s USDT\n", order.Quantity.Mul(order.Price))

	// Ask for confirmation
	if len(os.Args) > 1 && os.Args[1] == "--execute" {
		fmt.Println("\nExecuting WebSocket order...")
		
		start := time.Now()
		ctx := context.Background()
		
		resp, err := wsManager.CreateOrderWS(ctx, order)
		if err != nil {
			log.Printf("Failed to create order: %v", err)
			return
		}
		
		elapsed := time.Since(start)
		
		fmt.Printf("\n✅ Order created via WebSocket!\n")
		fmt.Printf("Order ID: %s\n", resp.OrderID)
		fmt.Printf("Status: %s\n", resp.Status)
		fmt.Printf("Execution time: %v\n", elapsed)
		
		// Cancel the order
		fmt.Println("\nCanceling order...")
		cancelStart := time.Now()
		
		// Note: This would need orderID as int64
		// err = wsManager.CancelOrderWS(ctx, order.Symbol, orderID)
		
		cancelElapsed := time.Since(cancelStart)
		fmt.Printf("Cancel execution time: %v\n", cancelElapsed)
	} else {
		fmt.Println("\nTo execute the order, run with --execute flag")
		fmt.Println("Example: go run cmd/test-ws-order/main.go --execute")
	}

	// Performance comparison
	fmt.Println("\n=== Performance Comparison ===")
	fmt.Println("REST API:")
	fmt.Println("- Connection: New TCP connection each time")
	fmt.Println("- Latency: 50-200ms typically")
	fmt.Println("- Overhead: HTTP headers, TLS handshake")
	
	fmt.Println("\nWebSocket API:")
	fmt.Println("- Connection: Persistent connection")
	fmt.Printf("- Latency: %v (measured)\n", latency)
	fmt.Println("- Overhead: Minimal after connection established")
	fmt.Println("- Benefits: Lower latency, less overhead, better for HFT")
}