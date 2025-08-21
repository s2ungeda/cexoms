package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/mExOms/internal/account"
	"github.com/mExOms/internal/exchange"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	// Create account manager
	accountManager := account.NewManager()
	
	// Add a test account
	testAccount := &types.Account{
		ID:          "test-ws-account",
		Name:        "Test WebSocket Account",
		Exchange:    types.ExchangeBinanceSpot,
		Type:        types.AccountTypeSpot,
		Status:      types.AccountStatusActive,
		SpotEnabled: true,
	}
	
	if err := accountManager.AddAccount(testAccount); err != nil {
		log.Printf("Warning: Failed to add account: %v", err)
	}
	
	// Create exchange factory and Binance connector
	factory := exchange.NewFactory(accountManager)
	binanceSpot, err := factory.CreateExchange(types.ExchangeBinanceSpot)
	if err != nil {
		log.Fatalf("Failed to create Binance Spot exchange: %v", err)
	}

	// Connect to exchange
	ctx := context.Background()
	if err := binanceSpot.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to Binance: %v", err)
	}

	// Get WebSocket info
	wsInfo := binanceSpot.GetWebSocketInfo()
	fmt.Printf("=== Binance WebSocket Support ===\n")
	fmt.Printf("WebSocket Supported: %v\n", wsInfo.Supported)
	fmt.Printf("Create Order: %v\n", wsInfo.OrderSupport.CreateOrder)
	fmt.Printf("Cancel Order: %v\n", wsInfo.OrderSupport.CancelOrder)
	fmt.Printf("Order Updates: %v\n", wsInfo.OrderSupport.OrderUpdates)
	fmt.Printf("WebSocket URL: %s\n", wsInfo.BaseURL)
	fmt.Println()

	switch command {
	case "test":
		testWebSocketConnection(binanceSpot)
	case "latency":
		testLatency(binanceSpot)
	case "order":
		if len(os.Args) < 5 {
			fmt.Println("Usage: test-ws-trading order SYMBOL QUANTITY PRICE")
			os.Exit(1)
		}
		testOrder(binanceSpot, os.Args[2], os.Args[3], os.Args[4])
	case "compare":
		compareLatency(binanceSpot)
	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("WebSocket Trading Test Tool")
	fmt.Println("\nCommands:")
	fmt.Println("  test              - Test WebSocket connection")
	fmt.Println("  latency           - Measure WebSocket latency")
	fmt.Println("  order SYMBOL QTY PRICE - Create test order via WebSocket")
	fmt.Println("  compare           - Compare WebSocket vs REST latency")
	fmt.Println("\nExamples:")
	fmt.Println("  go run test-ws-trading.go test")
	fmt.Println("  go run test-ws-trading.go latency")
	fmt.Println("  go run test-ws-trading.go order TRXUSDT 10 0.10")
	fmt.Println("  go run test-ws-trading.go compare")
}

func testWebSocketConnection(exchange types.Exchange) {
	wsManager := exchange.GetWebSocketOrderManager()
	if wsManager == nil {
		fmt.Println("WebSocket order manager not available")
		return
	}

	fmt.Printf("WebSocket Connected: %v\n", wsManager.IsConnected())
	
	// Get metrics
	metrics := wsManager.GetMetrics()
	fmt.Printf("\n=== WebSocket Metrics ===\n")
	fmt.Printf("Connected: %v\n", metrics.Connected)
	fmt.Printf("Connection Uptime: %v\n", metrics.ConnectionUptime)
	fmt.Printf("Messages Sent: %d\n", metrics.MessagesSent)
	fmt.Printf("Messages Received: %d\n", metrics.MessagesReceived)
	fmt.Printf("Orders Sent: %d\n", metrics.OrdersSent)
	fmt.Printf("Orders Successful: %d\n", metrics.OrdersSuccessful)
	fmt.Printf("Reconnect Count: %d\n", metrics.ReconnectCount)
}

func testLatency(exchange types.Exchange) {
	wsManager := exchange.GetWebSocketOrderManager()
	if wsManager == nil {
		fmt.Println("WebSocket order manager not available")
		return
	}

	fmt.Println("Measuring WebSocket latency (5 pings)...")
	
	var totalLatency time.Duration
	for i := 0; i < 5; i++ {
		latency, err := wsManager.GetLatency()
		if err != nil {
			fmt.Printf("Ping %d failed: %v\n", i+1, err)
			continue
		}
		totalLatency += latency
		fmt.Printf("Ping %d: %v\n", i+1, latency)
		time.Sleep(500 * time.Millisecond)
	}
	
	avgLatency := totalLatency / 5
	fmt.Printf("\nAverage latency: %v\n", avgLatency)
}

func testOrder(exchange types.Exchange, symbol, quantity, price string) {
	fmt.Printf("\n=== Creating Test Order via WebSocket ===\n")
	fmt.Printf("Symbol: %s\n", symbol)
	fmt.Printf("Quantity: %s\n", quantity)
	fmt.Printf("Price: %s\n", price)
	
	// Create order
	order := &types.Order{
		Symbol:      symbol,
		Side:        types.OrderSideBuy,
		Type:        types.OrderTypeLimit,
		Quantity:    decimal.RequireFromString(quantity),
		Price:       decimal.RequireFromString(price),
		TimeInForce: types.TimeInForceGTC,
	}
	
	qty := decimal.RequireFromString(quantity)
	prc := decimal.RequireFromString(price)
	fmt.Printf("Total Value: %s USDT\n", qty.Mul(prc).String())
	
	fmt.Print("\nCreate order? (yes/no): ")
	var confirm string
	fmt.Scanln(&confirm)
	
	if confirm != "yes" {
		fmt.Println("Order cancelled")
		return
	}
	
	// Create order via WebSocket
	ctx := context.Background()
	start := time.Now()
	
	createdOrder, err := exchange.CreateOrder(ctx, order)
	if err != nil {
		fmt.Printf("Failed to create order: %v\n", err)
		return
	}
	
	elapsed := time.Since(start)
	
	fmt.Printf("\n✅ Order created successfully!\n")
	fmt.Printf("Order ID: %s\n", createdOrder.ID)
	fmt.Printf("Status: %s\n", createdOrder.Status)
	fmt.Printf("Execution time: %v\n", elapsed)
	
	// Check if WebSocket was used
	wsManager := exchange.GetWebSocketOrderManager()
	if wsManager != nil && wsManager.IsConnected() {
		fmt.Println("✅ Order created via WebSocket")
		
		metrics := wsManager.GetMetrics()
		fmt.Printf("WebSocket Orders Sent: %d\n", metrics.OrdersSent)
		fmt.Printf("WebSocket Orders Successful: %d\n", metrics.OrdersSuccessful)
	} else {
		fmt.Println("ℹ️  Order created via REST API (fallback)")
	}
}

func compareLatency(exchange types.Exchange) {
	fmt.Println("=== Comparing WebSocket vs REST Latency ===\n")
	
	// Test WebSocket latency
	wsManager := exchange.GetWebSocketOrderManager()
	if wsManager != nil && wsManager.IsConnected() {
		fmt.Println("WebSocket Latency:")
		var wsTotal time.Duration
		for i := 0; i < 3; i++ {
			latency, err := wsManager.GetLatency()
			if err == nil {
				wsTotal += latency
				fmt.Printf("  Test %d: %v\n", i+1, latency)
			}
			time.Sleep(500 * time.Millisecond)
		}
		fmt.Printf("  Average: %v\n", wsTotal/3)
	}
	
	// For REST API comparison
	fmt.Println("\nREST API Latency:")
	fmt.Println("  Typical: 50-200ms")
	fmt.Println("  Includes: TCP handshake, TLS negotiation, HTTP overhead")
	
	fmt.Println("\n=== Summary ===")
	fmt.Println("WebSocket advantages:")
	fmt.Println("- Persistent connection (no handshake overhead)")
	fmt.Println("- Lower latency for subsequent requests")
	fmt.Println("- Real-time order updates")
	fmt.Println("- Better for high-frequency trading")
}