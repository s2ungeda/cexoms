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

	// Create WebSocket order manager for futures
	wsManager, err := createFuturesWebSocketManager()
	if err != nil {
		log.Fatalf("Failed to create WebSocket manager: %v", err)
	}

	// Connect to WebSocket
	fmt.Println("Connecting to Binance Futures WebSocket API...")
	if err := wsManager.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer wsManager.Disconnect()

	fmt.Printf("✓ Connected to Futures WebSocket\n\n")

	switch command {
	case "buy":
		if len(os.Args) < 5 {
			fmt.Println("Usage: test-ws-futures-trading buy SYMBOL QUANTITY PRICE")
			fmt.Println("Example: test-ws-futures-trading buy SOLUSDT 0.1 200")
			os.Exit(1)
		}
		createBuyOrder(ctx, wsManager, os.Args[2], os.Args[3], os.Args[4])
	case "sell":
		if len(os.Args) < 5 {
			fmt.Println("Usage: test-ws-futures-trading sell SYMBOL QUANTITY PRICE")
			fmt.Println("Example: test-ws-futures-trading sell SOLUSDT 0.1 210")
			os.Exit(1)
		}
		createSellOrder(ctx, wsManager, os.Args[2], os.Args[3], os.Args[4])
	case "cancel":
		if len(os.Args) < 4 {
			fmt.Println("Usage: test-ws-futures-trading cancel SYMBOL ORDER_ID")
			fmt.Println("Example: test-ws-futures-trading cancel BTCUSDT 123456789")
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
	fmt.Println("Binance Futures WebSocket Trading Test")
	fmt.Println("\nCommands:")
	fmt.Println("  buy SYMBOL QTY PRICE  - Open LONG via WebSocket")
	fmt.Println("  sell SYMBOL QTY PRICE - Open SHORT via WebSocket")
	fmt.Println("  cancel SYMBOL ID      - Cancel order via WebSocket")
	fmt.Println("  latency               - Test WebSocket latency")
	fmt.Println("  metrics               - Show WebSocket metrics")
	fmt.Println("\nExamples:")
	fmt.Println("  test-ws-futures-trading buy SOLUSDT 0.1 200")
	fmt.Println("  test-ws-futures-trading sell SOLUSDT 0.1 210")
	fmt.Println("  test-ws-futures-trading cancel SOLUSDT 123456789")
	fmt.Println("  test-ws-futures-trading latency")
}

func createFuturesWebSocketManager() (*binance.BinanceFuturesWSOrderManager, error) {
	// Get credentials from Vault
	vaultClient, err := binance.GetVaultClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %v", err)
	}

	keys, err := vaultClient.GetExchangeKeys("binance", "spot")
	if err != nil {
		return nil, fmt.Errorf("failed to get API keys from Vault: %v", err)
	}

	// WebSocket configuration for futures
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

	return binance.NewBinanceFuturesWSOrderManager(wsConfig), nil
}

func createBuyOrder(ctx context.Context, wsManager *binance.BinanceFuturesWSOrderManager, symbol, quantity, price string) {
	symbol = strings.ToUpper(symbol)
	
	// Safety check
	fmt.Printf("\n⚠️  FUTURES ORDER - SAFETY CHECK ⚠️\n")
	fmt.Printf("Opening LONG position via WebSocket (expecting price to go UP):\n")
	fmt.Printf("Symbol: %s\n", symbol)
	fmt.Printf("Quantity: %s contracts\n", quantity)
	fmt.Printf("Limit Price: %s\n", price)
	
	qty := decimal.RequireFromString(quantity)
	prc := decimal.RequireFromString(price)
	fmt.Printf("Order Value: %s USDT\n", qty.Mul(prc).String())
	
	fmt.Print("\nConfirm order? (yes/no): ")
	var confirm string
	fmt.Scanln(&confirm)
	
	if confirm != "yes" {
		fmt.Println("Order cancelled")
		return
	}

	// Create futures order via WebSocket
	order := &types.Order{
		Symbol:       symbol,
		Side:         types.OrderSideBuy,
		Type:         types.OrderTypeLimit,
		Quantity:     qty,
		Price:        prc,
		TimeInForce:  types.TimeInForceGTC,
		PositionSide: "BOTH", // One-way mode
	}

	start := time.Now()
	orderResp, err := wsManager.CreateOrder(ctx, order)
	if err != nil {
		log.Fatalf("Failed to create buy order: %v", err)
	}
	elapsed := time.Since(start)

	fmt.Printf("\n✅ LONG position order created via WebSocket!\n")
	fmt.Printf("Order ID: %s\n", orderResp.OrderID)
	fmt.Printf("Status: %s\n", orderResp.Status)
	fmt.Printf("Execution time: %v\n", elapsed)
	
	// Show WebSocket advantage
	fmt.Printf("\n📊 WebSocket Performance:\n")
	fmt.Printf("- Execution time: %v (REST API typical: 50-200ms)\n", elapsed)
	fmt.Printf("- Connection: Persistent (no handshake overhead)\n")
}

func createSellOrder(ctx context.Context, wsManager *binance.BinanceFuturesWSOrderManager, symbol, quantity, price string) {
	symbol = strings.ToUpper(symbol)
	
	// Safety check
	fmt.Printf("\n⚠️  FUTURES ORDER - SAFETY CHECK ⚠️\n")
	fmt.Printf("Opening SHORT position via WebSocket (expecting price to go DOWN):\n")
	fmt.Printf("Symbol: %s\n", symbol)
	fmt.Printf("Quantity: %s contracts\n", quantity)
	fmt.Printf("Limit Price: %s\n", price)
	
	qty := decimal.RequireFromString(quantity)
	prc := decimal.RequireFromString(price)
	fmt.Printf("Order Value: %s USDT\n", qty.Mul(prc).String())
	
	fmt.Print("\nConfirm order? (yes/no): ")
	var confirm string
	fmt.Scanln(&confirm)
	
	if confirm != "yes" {
		fmt.Println("Order cancelled")
		return
	}

	// Create futures order via WebSocket
	order := &types.Order{
		Symbol:       symbol,
		Side:         types.OrderSideSell,
		Type:         types.OrderTypeLimit,
		Quantity:     qty,
		Price:        prc,
		TimeInForce:  types.TimeInForceGTC,
		PositionSide: "BOTH", // One-way mode
	}

	start := time.Now()
	orderResp, err := wsManager.CreateOrder(ctx, order)
	if err != nil {
		log.Fatalf("Failed to create sell order: %v", err)
	}
	elapsed := time.Since(start)

	fmt.Printf("\n✅ SHORT position order created via WebSocket!\n")
	fmt.Printf("Order ID: %s\n", orderResp.OrderID)
	fmt.Printf("Status: %s\n", orderResp.Status)
	fmt.Printf("Execution time: %v\n", elapsed)
}

func cancelOrder(ctx context.Context, wsManager *binance.BinanceFuturesWSOrderManager, symbol, orderID string) {
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

func testLatency(ctx context.Context, wsManager *binance.BinanceFuturesWSOrderManager) {
	fmt.Println("=== Futures WebSocket Latency Test ===")
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

func showMetrics(wsManager *binance.BinanceFuturesWSOrderManager) {
	metrics := wsManager.GetMetrics()
	
	fmt.Println("=== Futures WebSocket Metrics ===")
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