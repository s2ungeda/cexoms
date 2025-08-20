package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/mExOms/services/binance"
	"github.com/shopspring/decimal"
)

func main() {
	fmt.Println("=== Simple WebSocket Order Test ===")

	// WebSocket configuration
	wsConfig := types.WebSocketConfig{
		URL:                "wss://ws-api.binance.com:443/ws-api/v3",
		APIKey:             "YOUR_API_KEY", // Will be replaced from Vault
		SecretKey:          "YOUR_SECRET",  // Will be replaced from Vault
		PingInterval:       30 * time.Second,
		ReconnectInterval:  5 * time.Second,
		MessageTimeout:     10 * time.Second,
		EnableCompression:  true,
		EnableHeartbeat:    true,
	}

	// Get credentials from Vault
	vaultClient, err := binance.GetVaultClient()
	if err == nil {
		keys, err := vaultClient.GetExchangeKeys("binance", "spot")
		if err == nil {
			wsConfig.APIKey = keys["api_key"]
			wsConfig.SecretKey = keys["secret_key"]
			fmt.Println("✓ Retrieved API keys from Vault")
		}
	}

	// Create WebSocket order manager
	wsManager := binance.NewBinanceWSOrderManager(wsConfig)
	
	// Connect
	ctx := context.Background()
	fmt.Println("\nConnecting to WebSocket...")
	if err := wsManager.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	fmt.Printf("✓ Connected: %v\n", wsManager.IsConnected())

	// Test latency
	fmt.Println("\nTesting latency...")
	for i := 0; i < 3; i++ {
		latency, err := wsManager.GetLatency()
		if err != nil {
			fmt.Printf("Ping %d failed: %v\n", i+1, err)
		} else {
			fmt.Printf("Ping %d: %v\n", i+1, latency)
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Get metrics
	metrics := wsManager.GetMetrics()
	fmt.Printf("\n=== WebSocket Metrics ===\n")
	fmt.Printf("Connected: %v\n", metrics.Connected)
	fmt.Printf("Connection Uptime: %v\n", metrics.ConnectionUptime)
	fmt.Printf("Messages Sent: %d\n", metrics.MessagesSent)
	fmt.Printf("Messages Received: %d\n", metrics.MessagesReceived)
	fmt.Printf("Average Latency: %v\n", metrics.AverageLatency)

	// Test order (small amount, far from market)
	fmt.Println("\n=== Test Order (NOT EXECUTED) ===")
	testOrder := &types.Order{
		Symbol:      "TRXUSDT",
		Side:        types.OrderSideBuy,
		Type:        types.OrderTypeLimit,
		Quantity:    decimal.NewFromInt(10),
		Price:       decimal.NewFromFloat(0.05), // Far below market
		TimeInForce: types.TimeInForceGTC,
	}
	
	fmt.Printf("Symbol: %s\n", testOrder.Symbol)
	fmt.Printf("Side: %s\n", testOrder.Side)
	fmt.Printf("Quantity: %s\n", testOrder.Quantity)
	fmt.Printf("Price: %s\n", testOrder.Price)
	fmt.Printf("Total: %s USDT\n", testOrder.Quantity.Mul(testOrder.Price))

	// Uncomment to actually create order
	/*
	orderResp, err := wsManager.CreateOrder(ctx, testOrder)
	if err != nil {
		fmt.Printf("Order failed: %v\n", err)
	} else {
		fmt.Printf("\n✅ Order created!\n")
		fmt.Printf("Order ID: %s\n", orderResp.OrderID)
		fmt.Printf("Status: %s\n", orderResp.Status)
	}
	*/

	// Disconnect
	fmt.Println("\nDisconnecting...")
	wsManager.Disconnect()
}

// Helper function to get Vault client
func GetVaultClient() (*binance.VaultClient, error) {
	// This would be implemented to return actual Vault client
	return nil, fmt.Errorf("not implemented")
}