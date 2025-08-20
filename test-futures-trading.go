package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/adshao/go-binance/v2/futures"
	"github.com/mExOms/pkg/types"
	"github.com/mExOms/pkg/vault"
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
	case "balance":
		// Balance still uses REST API as WebSocket doesn't support account info
		showBalance(ctx)
	case "positions":
		// Positions still uses REST API
		showPositions(ctx)
	case "price":
		if len(os.Args) < 3 {
			fmt.Println("Usage: test-futures-trading price SYMBOL")
			os.Exit(1)
		}
		showPrice(ctx, os.Args[2])
	case "info":
		if len(os.Args) < 3 {
			fmt.Println("Usage: test-futures-trading info SYMBOL")
			os.Exit(1)
		}
		showSymbolInfo(ctx, os.Args[2])
	case "leverage":
		if len(os.Args) < 4 {
			fmt.Println("Usage: test-futures-trading leverage SYMBOL LEVERAGE")
			os.Exit(1)
		}
		setLeverage(ctx, os.Args[2], os.Args[3])
	case "buy":
		if len(os.Args) < 5 {
			fmt.Println("Usage: test-futures-trading buy SYMBOL QUANTITY PRICE")
			os.Exit(1)
		}
		createBuyOrder(ctx, wsManager, os.Args[2], os.Args[3], os.Args[4])
	case "sell":
		if len(os.Args) < 5 {
			fmt.Println("Usage: test-futures-trading sell SYMBOL QUANTITY PRICE")
			os.Exit(1)
		}
		createSellOrder(ctx, wsManager, os.Args[2], os.Args[3], os.Args[4])
	case "orders":
		if len(os.Args) < 3 {
			showAllOpenOrders(ctx, wsManager)
		} else {
			showOpenOrders(ctx, wsManager, os.Args[2])
		}
	case "cancel":
		if len(os.Args) < 4 {
			fmt.Println("Usage: test-futures-trading cancel SYMBOL ORDER_ID")
			os.Exit(1)
		}
		cancelOrder(ctx, wsManager, os.Args[2], os.Args[3])
	case "status":
		if len(os.Args) < 4 {
			fmt.Println("Usage: test-futures-trading status SYMBOL ORDER_ID")
			os.Exit(1)
		}
		showOrderStatus(ctx, wsManager, os.Args[2], os.Args[3])
	case "latency":
		testLatency(ctx, wsManager)
	case "metrics":
		showMetrics(wsManager)
	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("Binance Futures WebSocket Trading Test Tool")
	fmt.Println("\nCommands:")
	fmt.Println("  balance              - Show futures account balance (REST API)")
	fmt.Println("  positions            - Show open positions (REST API)")
	fmt.Println("  price SYMBOL         - Show current price (REST API)")
	fmt.Println("  info SYMBOL          - Show symbol trading rules (REST API)")
	fmt.Println("  leverage SYMBOL NUM  - Set leverage (REST API)")
	fmt.Println("  buy SYMBOL QTY PRICE - Open LONG via WebSocket")
	fmt.Println("  sell SYMBOL QTY PRICE- Open SHORT via WebSocket")
	fmt.Println("  orders [SYMBOL]      - Show open orders via WebSocket")
	fmt.Println("  cancel SYMBOL ID     - Cancel order via WebSocket")
	fmt.Println("  status SYMBOL ID     - Get order status via WebSocket")
	fmt.Println("  latency              - Test WebSocket latency")
	fmt.Println("  metrics              - Show WebSocket metrics")
	fmt.Println("\nExamples:")
	fmt.Println("  test-futures-trading balance")
	fmt.Println("  test-futures-trading positions")
	fmt.Println("  test-futures-trading price BTCUSDT")
	fmt.Println("  test-futures-trading leverage SOLUSDT 5")
	fmt.Println("  test-futures-trading buy SOLUSDT 0.1 200")
	fmt.Println("  test-futures-trading latency")
}

func createFuturesWebSocketManager() (*binance.BinanceFuturesWSOrderManager, error) {
	// Get credentials from Vault
	vaultClient, err := vault.NewClient(vault.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %v", err)
	}

	keys, err := vaultClient.GetExchangeKeys("binance", "spot")
	if err != nil {
		return nil, fmt.Errorf("failed to get API keys from Vault: %v", err)
	}

	// WebSocket configuration for futures
	wsConfig := types.WebSocketConfig{
		URL:                "wss://fstream-auth.binance.com/ws-fapi/v1",
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

func showOpenOrders(ctx context.Context, wsManager *binance.BinanceFuturesWSOrderManager, symbol string) {
	symbol = strings.ToUpper(symbol)
	
	fmt.Printf("Fetching open orders for %s via WebSocket...\n", symbol)
	
	start := time.Now()
	orders, err := wsManager.GetOpenOrders(ctx, symbol)
	if err != nil {
		log.Fatalf("Failed to get open orders: %v", err)
	}
	elapsed := time.Since(start)

	if len(orders) == 0 {
		fmt.Printf("No open orders for %s\n", symbol)
		return
	}

	fmt.Printf("=== Open Orders for %s (via WebSocket) ===\n", symbol)
	fmt.Printf("Retrieved in: %v\n\n", elapsed)
	
	for _, order := range orders {
		fmt.Printf("Order ID: %s\n", order.ID)
		fmt.Printf("Side: %s", order.Side)
		if order.Side == types.OrderSideBuy {
			fmt.Println(" (LONG)")
		} else {
			fmt.Println(" (SHORT)")
		}
		fmt.Printf("Type: %s\n", order.Type)
		fmt.Printf("Price: %s\n", order.Price)
		fmt.Printf("Quantity: %s\n", order.Quantity)
		fmt.Printf("Status: %s\n", order.Status)
		fmt.Printf("---\n")
	}
}

func showAllOpenOrders(ctx context.Context, wsManager *binance.BinanceFuturesWSOrderManager) {
	fmt.Println("Fetching all open orders via WebSocket...")
	
	start := time.Now()
	orders, err := wsManager.GetOpenOrders(ctx, "")
	if err != nil {
		log.Fatalf("Failed to get open orders: %v", err)
	}
	elapsed := time.Since(start)

	if len(orders) == 0 {
		fmt.Println("No open orders")
		return
	}

	fmt.Println("=== All Open Orders (via WebSocket) ===")
	fmt.Printf("Retrieved in: %v\n\n", elapsed)
	
	for _, order := range orders {
		fmt.Printf("Symbol: %s\n", order.Symbol)
		fmt.Printf("Order ID: %s\n", order.ID)
		fmt.Printf("Side: %s", order.Side)
		if order.Side == types.OrderSideBuy {
			fmt.Print(" (LONG)")
		} else {
			fmt.Print(" (SHORT)")
		}
		fmt.Printf("\nPrice: %s\n", order.Price)
		fmt.Printf("Quantity: %s\n", order.Quantity)
		fmt.Printf("Status: %s\n", order.Status)
		fmt.Printf("---\n")
	}
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

func showOrderStatus(ctx context.Context, wsManager *binance.BinanceFuturesWSOrderManager, symbol, orderID string) {
	symbol = strings.ToUpper(symbol)
	
	fmt.Printf("Getting order status via WebSocket...\n")
	
	start := time.Now()
	order, err := wsManager.GetOrderStatus(ctx, symbol, orderID)
	if err != nil {
		log.Fatalf("Failed to get order status: %v", err)
	}
	elapsed := time.Since(start)

	fmt.Printf("\n=== Order Status (via WebSocket) ===\n")
	fmt.Printf("Retrieved in: %v\n\n", elapsed)
	fmt.Printf("Order ID: %s\n", order.ID)
	fmt.Printf("Symbol: %s\n", order.Symbol)
	fmt.Printf("Side: %s", order.Side)
	if order.Side == types.OrderSideBuy {
		fmt.Println(" (LONG)")
	} else {
		fmt.Println(" (SHORT)")
	}
	fmt.Printf("Type: %s\n", order.Type)
	fmt.Printf("Price: %s\n", order.Price)
	fmt.Printf("Quantity: %s\n", order.Quantity)
	fmt.Printf("Status: %s\n", order.Status)
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

// Helper functions for REST API operations
func showBalance(ctx context.Context) {
	vaultClient, err := vault.NewClient(vault.Config{})
	if err != nil {
		log.Fatalf("Failed to create vault client: %v", err)
	}

	keys, err := vaultClient.GetExchangeKeys("binance", "spot")
	if err != nil {
		log.Fatalf("Failed to get API keys: %v", err)
	}

	client := futures.NewClient(keys["api_key"], keys["secret_key"])
	
	account, err := client.NewGetAccountService().Do(ctx)
	if err != nil {
		log.Fatalf("Failed to get futures account: %v", err)
	}

	fmt.Println("=== Futures Account Balance ===")
	fmt.Printf("Total Wallet Balance: %s USDT\n", account.TotalWalletBalance)
	fmt.Printf("Total Unrealized PnL: %s USDT\n", account.TotalUnrealizedProfit)
	fmt.Printf("Total Margin Balance: %s USDT\n", account.TotalMarginBalance)
	fmt.Printf("Available Balance:    %s USDT\n", account.AvailableBalance)
	fmt.Printf("Total Position Value: %s USDT\n", account.TotalPositionInitialMargin)
}

func showPositions(ctx context.Context) {
	vaultClient, err := vault.NewClient(vault.Config{})
	if err != nil {
		log.Fatalf("Failed to create vault client: %v", err)
	}

	keys, err := vaultClient.GetExchangeKeys("binance", "spot")
	if err != nil {
		log.Fatalf("Failed to get API keys: %v", err)
	}

	client := futures.NewClient(keys["api_key"], keys["secret_key"])
	
	positions, err := client.NewGetPositionRiskService().Do(ctx)
	if err != nil {
		log.Fatalf("Failed to get positions: %v", err)
	}

	fmt.Println("=== Open Positions ===")
	hasPositions := false
	
	for _, pos := range positions {
		posAmt, _ := strconv.ParseFloat(pos.PositionAmt, 64)
		if posAmt != 0 {
			hasPositions = true
			entryPrice, _ := strconv.ParseFloat(pos.EntryPrice, 64)
			markPrice, _ := strconv.ParseFloat(pos.MarkPrice, 64)
			unrealizedProfit, _ := strconv.ParseFloat(pos.UnRealizedProfit, 64)
			positionNotional, _ := strconv.ParseFloat(pos.Notional, 64)
			
			fmt.Printf("\nSymbol: %s\n", pos.Symbol)
			fmt.Printf("Position: %.4f %s\n", posAmt, func() string {
				if posAmt > 0 {
					return "(LONG)"
				}
				return "(SHORT)"
			}())
			fmt.Printf("Entry Price: %.2f\n", entryPrice)
			fmt.Printf("Mark Price: %.2f\n", markPrice)
			fmt.Printf("Position Value: %.2f USDT\n", positionNotional)
			fmt.Printf("Unrealized PnL: %.4f USDT\n", unrealizedProfit)
			fmt.Printf("Leverage: %sx\n", pos.Leverage)
		}
	}
	
	if !hasPositions {
		fmt.Println("No open positions")
	}
}

func showPrice(ctx context.Context, symbol string) {
	vaultClient, err := vault.NewClient(vault.Config{})
	if err != nil {
		log.Fatalf("Failed to create vault client: %v", err)
	}

	keys, err := vaultClient.GetExchangeKeys("binance", "spot")
	if err != nil {
		log.Fatalf("Failed to get API keys: %v", err)
	}

	client := futures.NewClient(keys["api_key"], keys["secret_key"])
	symbol = strings.ToUpper(symbol)
	
	ticker, err := client.NewListPricesService().Symbol(symbol).Do(ctx)
	if err != nil {
		log.Fatalf("Failed to get price: %v", err)
	}

	if len(ticker) == 0 {
		fmt.Printf("No price found for %s\n", symbol)
		return
	}

	fmt.Printf("%s Price: %s\n", symbol, ticker[0].Price)
	
	// Also show 24hr stats
	stats, err := client.NewListPriceChangeStatsService().Symbol(symbol).Do(ctx)
	if err == nil && len(stats) > 0 {
		fmt.Printf("24hr Change: %s (%s%%)\n", stats[0].PriceChange, stats[0].PriceChangePercent)
		fmt.Printf("24hr High:   %s\n", stats[0].HighPrice)
		fmt.Printf("24hr Low:    %s\n", stats[0].LowPrice)
		fmt.Printf("24hr Volume: %s contracts\n", stats[0].Volume)
	}
}

func showSymbolInfo(ctx context.Context, symbol string) {
	vaultClient, err := vault.NewClient(vault.Config{})
	if err != nil {
		log.Fatalf("Failed to create vault client: %v", err)
	}

	keys, err := vaultClient.GetExchangeKeys("binance", "spot")
	if err != nil {
		log.Fatalf("Failed to get API keys: %v", err)
	}

	client := futures.NewClient(keys["api_key"], keys["secret_key"])
	symbol = strings.ToUpper(symbol)
	
	info, err := client.NewExchangeInfoService().Do(ctx)
	if err != nil {
		log.Fatalf("Failed to get exchange info: %v", err)
	}

	var symbolInfo *futures.Symbol
	for _, s := range info.Symbols {
		if s.Symbol == symbol {
			symbolInfo = &s
			break
		}
	}

	if symbolInfo == nil {
		fmt.Printf("Symbol %s not found\n", symbol)
		return
	}

	fmt.Printf("=== %s Futures Trading Rules ===\n", symbol)
	fmt.Printf("Status: %s\n", symbolInfo.Status)
	
	// Find filters
	for _, f := range symbolInfo.Filters {
		filterType := f["filterType"].(string)
		switch filterType {
		case "LOT_SIZE":
			fmt.Printf("\nQuantity Rules:\n")
			fmt.Printf("  Min Qty: %s\n", f["minQty"])
			fmt.Printf("  Max Qty: %s\n", f["maxQty"])
			fmt.Printf("  Step Size: %s\n", f["stepSize"])
		case "PRICE_FILTER":
			fmt.Printf("\nPrice Rules:\n")
			fmt.Printf("  Min Price: %s\n", f["minPrice"])
			fmt.Printf("  Max Price: %s\n", f["maxPrice"])
			fmt.Printf("  Tick Size: %s\n", f["tickSize"])
		case "MIN_NOTIONAL":
			fmt.Printf("\nMin Order Value: %s USDT\n", f["notional"])
		}
	}
}

func setLeverage(ctx context.Context, symbol string, leverageStr string) {
	vaultClient, err := vault.NewClient(vault.Config{})
	if err != nil {
		log.Fatalf("Failed to create vault client: %v", err)
	}

	keys, err := vaultClient.GetExchangeKeys("binance", "spot")
	if err != nil {
		log.Fatalf("Failed to get API keys: %v", err)
	}

	client := futures.NewClient(keys["api_key"], keys["secret_key"])
	symbol = strings.ToUpper(symbol)
	leverage, err := strconv.Atoi(leverageStr)
	if err != nil {
		log.Fatalf("Invalid leverage: %s", leverageStr)
	}

	resp, err := client.NewChangeLeverageService().
		Symbol(symbol).
		Leverage(leverage).
		Do(ctx)
		
	if err != nil {
		log.Fatalf("Failed to set leverage: %v", err)
	}

	fmt.Printf("✅ Leverage set successfully!\n")
	fmt.Printf("Symbol: %s\n", resp.Symbol)
	fmt.Printf("Leverage: %dx\n", resp.Leverage)
}