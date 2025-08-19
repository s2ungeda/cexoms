package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	proto "github.com/mExOms/proto"
)

func main() {
	// Command line flags
	var (
		serverAddr = flag.String("server", "localhost:50051", "OMS server address")
		timeout    = flag.Duration("timeout", 30*time.Second, "Request timeout")
	)

	// Subcommands
	placeOrderCmd := flag.NewFlagSet("place", flag.ExitOnError)
	var (
		symbol    = placeOrderCmd.String("symbol", "", "Trading symbol (e.g., BTCUSDT)")
		side      = placeOrderCmd.String("side", "", "Order side (BUY or SELL)")
		orderType = placeOrderCmd.String("type", "LIMIT", "Order type (LIMIT or MARKET)")
		quantity  = placeOrderCmd.Float64("quantity", 0, "Order quantity")
		price     = placeOrderCmd.Float64("price", 0, "Order price (for LIMIT orders)")
		exchange  = placeOrderCmd.String("exchange", "binance", "Exchange name")
		market    = placeOrderCmd.String("market", "spot", "Market type (spot or futures)")
		account   = placeOrderCmd.String("account", "main", "Account ID")
	)

	cancelOrderCmd := flag.NewFlagSet("cancel", flag.ExitOnError)
	var (
		orderID = cancelOrderCmd.String("id", "", "Order ID to cancel")
	)

	getOrderCmd := flag.NewFlagSet("get-order", flag.ExitOnError)
	var (
		getOrderID = getOrderCmd.String("id", "", "Order ID to retrieve")
	)

	listOrdersCmd := flag.NewFlagSet("list-orders", flag.ExitOnError)
	var (
		listStatus = listOrdersCmd.String("status", "", "Filter by status (OPEN, FILLED, CANCELLED)")
		listSymbol = listOrdersCmd.String("symbol", "", "Filter by symbol")
	)

	balanceCmd := flag.NewFlagSet("balance", flag.ExitOnError)
	var (
		balanceExchange = balanceCmd.String("exchange", "binance", "Exchange name")
		balanceMarket   = balanceCmd.String("market", "spot", "Market type")
		balanceAccount  = balanceCmd.String("account", "main", "Account ID")
	)

	positionsCmd := flag.NewFlagSet("positions", flag.ExitOnError)
	var (
		posExchange = positionsCmd.String("exchange", "binance", "Exchange name")
		posAccount  = positionsCmd.String("account", "main", "Account ID")
	)

	flag.Parse()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Connect to server
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, *serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()

	client := proto.NewOrderServiceClient(conn)

	// Handle subcommands
	switch os.Args[1] {
	case "place":
		placeOrderCmd.Parse(os.Args[2:])
		if *symbol == "" || *side == "" || *quantity == 0 {
			fmt.Println("Error: symbol, side, and quantity are required")
			placeOrderCmd.PrintDefaults()
			os.Exit(1)
		}
		placeOrder(ctx, client, *symbol, *side, *orderType, *quantity, *price, *exchange, *market, *account)

	case "cancel":
		cancelOrderCmd.Parse(os.Args[2:])
		if *orderID == "" {
			fmt.Println("Error: order ID is required")
			cancelOrderCmd.PrintDefaults()
			os.Exit(1)
		}
		cancelOrder(ctx, client, *orderID)

	case "get-order":
		getOrderCmd.Parse(os.Args[2:])
		if *getOrderID == "" {
			fmt.Println("Error: order ID is required")
			getOrderCmd.PrintDefaults()
			os.Exit(1)
		}
		getOrder(ctx, client, *getOrderID)

	case "list-orders":
		listOrdersCmd.Parse(os.Args[2:])
		listOrders(ctx, client, *listStatus, *listSymbol)

	case "balance":
		balanceCmd.Parse(os.Args[2:])
		getBalance(ctx, client, *balanceExchange, *balanceMarket, *balanceAccount)

	case "positions":
		positionsCmd.Parse(os.Args[2:])
		getPositions(ctx, client, *posExchange, *posAccount)

	case "stream-prices":
		streamPrices(ctx, client)

	case "stream-orders":
		streamOrders(ctx, client)

	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func placeOrder(ctx context.Context, client proto.OrderServiceClient, symbol, side, orderType string, quantity, price float64, exchange, market, account string) {
	req := &proto.PlaceOrderRequest{
		Symbol:    symbol,
		Side:      side,
		OrderType: orderType,
		Quantity:  quantity,
		Price:     price,
		Exchange:  exchange,
		Market:    market,
		AccountId: account,
	}

	resp, err := client.PlaceOrder(ctx, req)
	if err != nil {
		log.Fatalf("Failed to place order: %v", err)
	}

	fmt.Printf("Order placed successfully!\n")
	fmt.Printf("Order ID: %s\n", resp.OrderId)
	fmt.Printf("Status: %s\n", resp.Status)
	fmt.Printf("Exchange Order ID: %s\n", resp.ExchangeOrderId)
	fmt.Printf("Created: %s\n", time.Unix(resp.CreatedAt, 0).Format(time.RFC3339))
}

func cancelOrder(ctx context.Context, client proto.OrderServiceClient, orderID string) {
	req := &proto.CancelOrderRequest{
		OrderId: orderID,
	}

	resp, err := client.CancelOrder(ctx, req)
	if err != nil {
		log.Fatalf("Failed to cancel order: %v", err)
	}

	fmt.Printf("Order cancelled successfully!\n")
	fmt.Printf("Order ID: %s\n", resp.OrderId)
	fmt.Printf("Status: %s\n", resp.Status)
}

func getOrder(ctx context.Context, client proto.OrderServiceClient, orderID string) {
	req := &proto.GetOrderRequest{
		OrderId: orderID,
	}

	resp, err := client.GetOrder(ctx, req)
	if err != nil {
		log.Fatalf("Failed to get order: %v", err)
	}

	printOrder(resp.Order)
}

func listOrders(ctx context.Context, client proto.OrderServiceClient, status, symbol string) {
	req := &proto.ListOrdersRequest{
		Status: status,
		Symbol: symbol,
	}

	resp, err := client.ListOrders(ctx, req)
	if err != nil {
		log.Fatalf("Failed to list orders: %v", err)
	}

	fmt.Printf("Found %d orders\n\n", len(resp.Orders))
	for _, order := range resp.Orders {
		printOrder(order)
		fmt.Println()
	}
}

func getBalance(ctx context.Context, client proto.OrderServiceClient, exchange, market, account string) {
	req := &proto.GetBalanceRequest{
		Exchange:  exchange,
		Market:    market,
		AccountId: account,
	}

	resp, err := client.GetBalance(ctx, req)
	if err != nil {
		log.Fatalf("Failed to get balance: %v", err)
	}

	fmt.Printf("Balance for %s %s (Account: %s)\n", exchange, market, account)
	fmt.Println("==========================================")
	
	for _, balance := range resp.Balances {
		fmt.Printf("%-10s: Free: %12.8f | Locked: %12.8f | Total: %12.8f\n",
			balance.Asset, balance.Free, balance.Locked, balance.Free+balance.Locked)
	}
}

func getPositions(ctx context.Context, client proto.OrderServiceClient, exchange, account string) {
	req := &proto.GetPositionsRequest{
		Exchange:  exchange,
		AccountId: account,
	}

	resp, err := client.GetPositions(ctx, req)
	if err != nil {
		log.Fatalf("Failed to get positions: %v", err)
	}

	fmt.Printf("Positions for %s (Account: %s)\n", exchange, account)
	fmt.Println("==========================================")
	
	for _, pos := range resp.Positions {
		fmt.Printf("Symbol: %s\n", pos.Symbol)
		fmt.Printf("  Side: %s | Size: %.8f | Entry: $%.2f\n", pos.Side, pos.Size, pos.EntryPrice)
		fmt.Printf("  Mark Price: $%.2f | PnL: $%.2f (%.2f%%)\n", pos.MarkPrice, pos.UnrealizedPnl, pos.PnlPercentage)
		fmt.Printf("  Leverage: %dx | Margin: $%.2f\n", pos.Leverage, pos.Margin)
		fmt.Println()
	}
}

func streamPrices(ctx context.Context, client proto.OrderServiceClient) {
	req := &proto.StreamPricesRequest{
		Symbols: []string{"BTCUSDT", "ETHUSDT", "XRPUSDT"},
	}

	stream, err := client.StreamPrices(ctx, req)
	if err != nil {
		log.Fatalf("Failed to stream prices: %v", err)
	}

	fmt.Println("Streaming prices... (Press Ctrl+C to stop)")
	fmt.Println("==========================================")

	for {
		resp, err := stream.Recv()
		if err != nil {
			log.Fatalf("Error receiving price update: %v", err)
		}

		fmt.Printf("[%s] %s %s - Bid: $%.2f (%.4f) | Ask: $%.2f (%.4f) | Last: $%.2f\n",
			time.Now().Format("15:04:05"),
			resp.Exchange,
			resp.Symbol,
			resp.BidPrice, resp.BidQuantity,
			resp.AskPrice, resp.AskQuantity,
			resp.LastPrice)
	}
}

func streamOrders(ctx context.Context, client proto.OrderServiceClient) {
	req := &proto.StreamOrdersRequest{}

	stream, err := client.StreamOrders(ctx, req)
	if err != nil {
		log.Fatalf("Failed to stream orders: %v", err)
	}

	fmt.Println("Streaming order updates... (Press Ctrl+C to stop)")
	fmt.Println("==========================================")

	for {
		resp, err := stream.Recv()
		if err != nil {
			log.Fatalf("Error receiving order update: %v", err)
		}

		fmt.Printf("[%s] Order Update:\n", time.Now().Format("15:04:05"))
		printOrder(resp.Order)
		fmt.Println()
	}
}

func printOrder(order *proto.Order) {
	fmt.Printf("Order ID: %s\n", order.OrderId)
	fmt.Printf("Exchange Order ID: %s\n", order.ExchangeOrderId)
	fmt.Printf("Symbol: %s | Side: %s | Type: %s\n", order.Symbol, order.Side, order.OrderType)
	fmt.Printf("Quantity: %.8f | Price: $%.2f\n", order.Quantity, order.Price)
	fmt.Printf("Filled: %.8f | Remaining: %.8f\n", order.FilledQuantity, order.Quantity-order.FilledQuantity)
	fmt.Printf("Status: %s\n", order.Status)
	fmt.Printf("Exchange: %s | Market: %s | Account: %s\n", order.Exchange, order.Market, order.AccountId)
	fmt.Printf("Created: %s\n", time.Unix(order.CreatedAt, 0).Format(time.RFC3339))
	if order.UpdatedAt > 0 {
		fmt.Printf("Updated: %s\n", time.Unix(order.UpdatedAt, 0).Format(time.RFC3339))
	}
}

func printUsage() {
	fmt.Println("OMS Client - Command Line Interface")
	fmt.Println("Usage: oms-client [command] [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  place          Place a new order")
	fmt.Println("  cancel         Cancel an existing order")
	fmt.Println("  get-order      Get order details")
	fmt.Println("  list-orders    List orders with optional filters")
	fmt.Println("  balance        Get account balance")
	fmt.Println("  positions      Get open positions (futures)")
	fmt.Println("  stream-prices  Stream real-time prices")
	fmt.Println("  stream-orders  Stream order updates")
	fmt.Println()
	fmt.Println("Global options:")
	fmt.Println("  -server string   OMS server address (default: localhost:50051)")
	fmt.Println("  -timeout duration Request timeout (default: 30s)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  # Place a limit order")
	fmt.Println("  oms-client place -symbol BTCUSDT -side BUY -quantity 0.001 -price 115000")
	fmt.Println()
	fmt.Println("  # Place a market order")
	fmt.Println("  oms-client place -symbol ETHUSDT -side SELL -type MARKET -quantity 0.1")
	fmt.Println()
	fmt.Println("  # Cancel an order")
	fmt.Println("  oms-client cancel -id order123")
	fmt.Println()
	fmt.Println("  # Get balance")
	fmt.Println("  oms-client balance -exchange binance -market spot")
	fmt.Println()
	fmt.Println("  # Stream prices")
	fmt.Println("  oms-client stream-prices")
}