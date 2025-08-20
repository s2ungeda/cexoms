package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/mExOms/services/binance"
	"github.com/shopspring/decimal"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	ctx := context.Background()

	// Create WebSocket order manager
	wsManager, err := createWebSocketManager()
	if err != nil {
		log.Fatalf("Failed to create WebSocket manager: %v", err)
	}

	// Connect to WebSocket
	fmt.Println("Connecting to Binance WebSocket API...")
	if err := wsManager.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer wsManager.Disconnect()

	fmt.Printf("✓ Connected to WebSocket\n\n")

	switch command {
	case "buy":
		if len(os.Args) < 5 {
			fmt.Println("Usage: test-ws-spot-trading buy SYMBOL QUANTITY PRICE")
			fmt.Println("Example: test-ws-spot-trading buy TRXUSDT 10 0.15")
			os.Exit(1)
		}
		createBuyOrder(ctx, wsManager, os.Args[2], os.Args[3], os.Args[4])
	case "sell":
		if len(os.Args) < 5 {
			fmt.Println("Usage: test-ws-spot-trading sell SYMBOL QUANTITY PRICE")
			fmt.Println("Example: test-ws-spot-trading sell TRXUSDT 10 0.16")
			os.Exit(1)
		}
		createSellOrder(ctx, wsManager, os.Args[2], os.Args[3], os.Args[4])
	case "cancel":
		if len(os.Args) < 4 {
			fmt.Println("Usage: test-ws-spot-trading cancel SYMBOL ORDER_ID")
			fmt.Println("Example: test-ws-spot-trading cancel BTCUSDT 123456789")
			os.Exit(1)
		}
		cancelOrder(ctx, wsManager, os.Args[2], os.Args[3])
	case "latency":
		testLatency(ctx, wsManager)
	case "metrics":
		showMetrics(wsManager)
	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("Binance Spot WebSocket Trading Test")
	fmt.Println("\nCommands:")
	fmt.Println("  buy SYMBOL QTY PRICE  - Create buy order via WebSocket")
	fmt.Println("  sell SYMBOL QTY PRICE - Create sell order via WebSocket")
	fmt.Println("  cancel SYMBOL ID      - Cancel order via WebSocket")
	fmt.Println("  latency               - Test WebSocket latency")
	fmt.Println("  metrics               - Show WebSocket metrics")
	fmt.Println("\nExamples:")
	fmt.Println("  test-ws-spot-trading buy TRXUSDT 10 0.15")
	fmt.Println("  test-ws-spot-trading sell TRXUSDT 10 0.16")
	fmt.Println("  test-ws-spot-trading cancel TRXUSDT 123456789")
	fmt.Println("  test-ws-spot-trading latency")
}

func createWebSocketManager() (*binance.BinanceWSOrderManager, error) {
	// Get credentials from Vault
	vaultClient, err := binance.GetVaultClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %v", err)
	}

	keys, err := vaultClient.GetExchangeKeys("binance", "spot")
	if err != nil {
		return nil, fmt.Errorf("failed to get API keys from Vault: %v", err)
	}

	// WebSocket configuration
	wsConfig := types.WebSocketConfig{
		URL:                "wss://ws-api.binance.com:443/ws-api/v3",
		APIKey:             keys["api_key"],
		SecretKey:          keys["secret_key"],
		PingInterval:       30 * time.Second,
		ReconnectInterval:  5 * time.Second,
		MessageTimeout:     10 * time.Second,
		EnableCompression:  true,
		EnableHeartbeat:    true,
	}

	return binance.NewBinanceWSOrderManager(wsConfig), nil
}

func createBuyOrder(ctx context.Context, wsManager *binance.BinanceWSOrderManager, symbol, quantity, price string) {
	symbol = strings.ToUpper(symbol)
	
	// Safety check
	fmt.Printf("\n⚠️  SAFETY CHECK ⚠️\n")
	fmt.Printf("You are about to place a BUY order via WebSocket:\n")
	fmt.Printf("Symbol: %s\n", symbol)
	fmt.Printf("Quantity: %s\n", quantity)
	fmt.Printf("Price: %s\n", price)
	
	qty := decimal.RequireFromString(quantity)
	prc := decimal.RequireFromString(price)
	fmt.Printf("Total Cost: %s USDT\n", qty.Mul(prc).String())
	
	fmt.Print("\nConfirm order? (yes/no): ")
	var confirm string
	fmt.Scanln(&confirm)
	
	if confirm != "yes" {
		fmt.Println("Order cancelled")
		return
	}

	// Create order via WebSocket
	order := &types.Order{
		Symbol:      symbol,
		Side:        types.OrderSideBuy,
		Type:        types.OrderTypeLimit,
		Quantity:    qty,
		Price:       prc,
		TimeInForce: types.TimeInForceGTC,
	}

	start := time.Now()
	orderResp, err := wsManager.CreateOrder(ctx, order)
	if err != nil {
		log.Fatalf("Failed to create buy order: %v", err)
	}
	elapsed := time.Since(start)

	fmt.Printf("\n✅ Buy order created successfully via WebSocket!\n")
	fmt.Printf("Order ID: %s\n", orderResp.OrderID)
	fmt.Printf("Status: %s\n", orderResp.Status)
	fmt.Printf("Execution time: %v\n", elapsed)
	
	// Show WebSocket advantage
	fmt.Printf("\n📊 WebSocket Performance:\n")
	fmt.Printf("- Execution time: %v (REST API typical: 50-200ms)\n", elapsed)
	fmt.Printf("- Connection: Persistent (no handshake overhead)\n")
}

func createSellOrder(ctx context.Context, wsManager *binance.BinanceWSOrderManager, symbol, quantity, price string) {
	symbol = strings.ToUpper(symbol)
	
	// Safety check
	fmt.Printf("\n⚠️  SAFETY CHECK ⚠️\n")
	fmt.Printf("You are about to place a SELL order via WebSocket:\n")
	fmt.Printf("Symbol: %s\n", symbol)
	fmt.Printf("Quantity: %s\n", quantity)
	fmt.Printf("Price: %s\n", price)
	
	qty := decimal.RequireFromString(quantity)
	prc := decimal.RequireFromString(price)
	fmt.Printf("Total Value: %s USDT\n", qty.Mul(prc).String())
	
	fmt.Print("\nConfirm order? (yes/no): ")
	var confirm string
	fmt.Scanln(&confirm)
	
	if confirm != "yes" {
		fmt.Println("Order cancelled")
		return
	}

	// Create order via WebSocket
	order := &types.Order{
		Symbol:      symbol,
		Side:        types.OrderSideSell,
		Type:        types.OrderTypeLimit,
		Quantity:    qty,
		Price:       prc,
		TimeInForce: types.TimeInForceGTC,
	}

	start := time.Now()
	orderResp, err := wsManager.CreateOrder(ctx, order)
	if err != nil {
		log.Fatalf("Failed to create sell order: %v", err)
	}
	elapsed := time.Since(start)

	fmt.Printf("\n✅ Sell order created successfully via WebSocket!\n")
	fmt.Printf("Order ID: %s\n", orderResp.OrderID)
	fmt.Printf("Status: %s\n", orderResp.Status)
	fmt.Printf("Execution time: %v\n", elapsed)
}

func cancelOrder(ctx context.Context, wsManager *binance.BinanceWSOrderManager, symbol, orderID string) {
	symbol = strings.ToUpper(symbol)
	
	fmt.Printf("Cancelling order %s via WebSocket...\n", orderID)
	
	start := time.Now()
	err := wsManager.CancelOrder(ctx, symbol, orderID)
	if err != nil {
		log.Fatalf("Failed to cancel order: %v", err)
	}
	elapsed := time.Since(start)

	fmt.Printf("✅ Order cancelled successfully via WebSocket!\n")
	fmt.Printf("Execution time: %v\n", elapsed)
}

func testLatency(ctx context.Context, wsManager *binance.BinanceWSOrderManager) {
	fmt.Println("=== WebSocket Latency Test ===")
	fmt.Println("Testing with 5 pings...")
	
	var totalLatency time.Duration
	minLatency := time.Hour
	maxLatency := time.Duration(0)
	
	for i := 0; i < 5; i++ {
		latency, err := wsManager.GetLatency()
		if err != nil {
			fmt.Printf("Ping %d failed: %v\n", i+1, err)
			continue
		}
		
		totalLatency += latency
		if latency < minLatency {
			minLatency = latency
		}
		if latency > maxLatency {
			maxLatency = latency
		}
		
		fmt.Printf("Ping %d: %v\n", i+1, latency)
		time.Sleep(500 * time.Millisecond)
	}
	
	avgLatency := totalLatency / 5
	fmt.Printf("\nResults:\n")
	fmt.Printf("Average: %v\n", avgLatency)
	fmt.Printf("Min: %v\n", minLatency)
	fmt.Printf("Max: %v\n", maxLatency)
	fmt.Printf("\nCompare to REST API typical latency: 50-200ms\n")
}

func showMetrics(wsManager *binance.BinanceWSOrderManager) {
	metrics := wsManager.GetMetrics()
	
	fmt.Println("=== WebSocket Metrics ===")
	fmt.Printf("Connected: %v\n", metrics.Connected)
	fmt.Printf("Connection Uptime: %v\n", metrics.ConnectionUptime)
	fmt.Printf("Messages Sent: %d\n", metrics.MessagesSent)
	fmt.Printf("Messages Received: %d\n", metrics.MessagesReceived)
	fmt.Printf("Orders Sent: %d\n", metrics.OrdersSent)
	fmt.Printf("Orders Successful: %d\n", metrics.OrdersSuccessful)
	fmt.Printf("Orders Failed: %d\n", metrics.OrdersFailed)
	fmt.Printf("Average Latency: %v\n", metrics.AverageLatency)
	fmt.Printf("Last Latency: %v\n", metrics.LastLatency)
	fmt.Printf("Reconnect Count: %d\n", metrics.ReconnectCount)
}