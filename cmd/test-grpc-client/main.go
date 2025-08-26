package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	
	pb "github.com/mExOms/proto/oms/v1"
)

func main() {
	fmt.Println("=== gRPC API Client Test ===")
	
	// Connection options
	var opts []grpc.DialOption
	
	// TLS configuration
	useTLS := false // Set to true if server has TLS enabled
	if useTLS {
		config := &tls.Config{
			MinVersion: tls.VersionTLS13,
			ServerName: "localhost", // Must match server certificate
		}
		creds := credentials.NewTLS(config)
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithInsecure())
	}
	
	// Connect to server
	conn, err := grpc.Dial("localhost:50051", opts...)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()
	
	// Create clients
	authClient := pb.NewAuthServiceClient(conn)
	accountClient := pb.NewAccountServiceClient(conn)
	orderClient := pb.NewOrderServiceClient(conn)
	positionClient := pb.NewPositionServiceClient(conn)
	strategyClient := pb.NewStrategyServiceClient(conn)
	
	// Test authentication
	fmt.Println("\n1. Testing Authentication...")
	token := testAuthentication(authClient)
	
	// Create context with auth token
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
	
	// Test account operations
	fmt.Println("\n2. Testing Account Operations...")
	accountID := testAccountOperations(ctx, accountClient)
	
	// Test strategy operations
	fmt.Println("\n3. Testing Strategy Operations...")
	strategyID := testStrategyOperations(ctx, strategyClient, accountID)
	
	// Test order operations
	fmt.Println("\n4. Testing Order Operations...")
	testOrderOperations(ctx, orderClient, accountID)
	
	// Test position operations
	fmt.Println("\n5. Testing Position Operations...")
	testPositionOperations(ctx, positionClient, accountID)
	
	fmt.Println("\n=== All tests completed ===")
}

func testAuthentication(client pb.AuthServiceClient) string {
	// Authenticate with API key
	authReq := &pb.AuthRequest{
		ApiKey: "test_api_key",
		Secret: "test_secret",
	}
	
	authResp, err := client.Authenticate(context.Background(), authReq)
	if err != nil {
		// For testing, return a dummy token
		fmt.Printf("Authentication failed (expected in test): %v\n", err)
		return "dummy_token"
	}
	
	fmt.Printf("Authenticated successfully, token expires at: %v\n", authResp.ExpiresAt)
	return authResp.Token
}

func testAccountOperations(ctx context.Context, client pb.AccountServiceClient) string {
	// Create account
	createReq := &pb.CreateAccountRequest{
		Name:             "Test Trading Account",
		Type:             pb.AccountType_ACCOUNT_TYPE_SUB,
		Exchange:         "binance",
		Strategy:         "test_strategy",
		SpotEnabled:      true,
		FuturesEnabled:   true,
		MaxPositionUsdt:  "100000",
		MaxLeverage:      20,
	}
	
	createResp, err := client.CreateAccount(ctx, createReq)
	if err != nil {
		fmt.Printf("Failed to create account: %v\n", err)
		// Use a dummy account ID for testing
		return "test_account_001"
	}
	
	fmt.Printf("Created account: %s\n", createResp.Account.Id)
	
	// Get account details
	getReq := &pb.GetAccountRequest{
		AccountId: createResp.Account.Id,
	}
	
	getResp, err := client.GetAccount(ctx, getReq)
	if err != nil {
		fmt.Printf("Failed to get account: %v\n", err)
	} else {
		fmt.Printf("Account details: %+v\n", getResp.Account)
	}
	
	// List accounts
	listReq := &pb.ListAccountsRequest{
		ActiveOnly: true,
		Exchange:   "binance",
		PageSize:   10,
	}
	
	listResp, err := client.ListAccounts(ctx, listReq)
	if err != nil {
		fmt.Printf("Failed to list accounts: %v\n", err)
	} else {
		fmt.Printf("Found %d accounts\n", len(listResp.Accounts))
	}
	
	// Get account balance
	balanceReq := &pb.GetAccountBalanceRequest{
		AccountId: createResp.Account.Id,
	}
	
	balanceResp, err := client.GetAccountBalance(ctx, balanceReq)
	if err != nil {
		fmt.Printf("Failed to get balance: %v\n", err)
	} else {
		for _, balance := range balanceResp.Balances {
			fmt.Printf("Balance: %s = %s (available: %s)\n", 
				balance.Asset, balance.Total, balance.Available)
		}
	}
	
	return createResp.Account.Id
}

func testStrategyOperations(ctx context.Context, client pb.StrategyServiceClient, accountID string) string {
	// Create strategy
	createReq := &pb.CreateStrategyRequest{
		Name:        "Test Arbitrage Strategy",
		Description: "Test strategy for arbitrage trading",
		Type:        pb.StrategyType_STRATEGY_TYPE_ARBITRAGE,
		Accounts:    []string{accountID},
		Config: &pb.StrategyConfig{
			Symbols:              []string{"BTCUSDT", "ETHUSDT"},
			MaxPositionPerSymbol: "50000",
			MaxTotalExposure:     "100000",
			RiskLimit:            0.02,
			MaxOrdersPerSecond:   10,
		},
	}
	
	createResp, err := client.CreateStrategy(ctx, createReq)
	if err != nil {
		fmt.Printf("Failed to create strategy: %v\n", err)
		return "test_strategy_001"
	}
	
	fmt.Printf("Created strategy: %s\n", createResp.Strategy.Id)
	
	// Get strategy
	getReq := &pb.GetStrategyRequest{
		StrategyId: createResp.Strategy.Id,
	}
	
	getResp, err := client.GetStrategy(ctx, getReq)
	if err != nil {
		fmt.Printf("Failed to get strategy: %v\n", err)
	} else {
		fmt.Printf("Strategy status: %s\n", getResp.Strategy.Status)
	}
	
	// Start strategy
	startReq := &pb.StartStrategyRequest{
		StrategyId: createResp.Strategy.Id,
	}
	
	startResp, err := client.StartStrategy(ctx, startReq)
	if err != nil {
		fmt.Printf("Failed to start strategy: %v\n", err)
	} else {
		fmt.Printf("Strategy started, status: %s\n", startResp.Strategy.Status)
	}
	
	// Get strategy metrics
	metricsReq := &pb.GetStrategyMetricsRequest{
		StrategyId: createResp.Strategy.Id,
	}
	
	metricsResp, err := client.GetStrategyMetrics(ctx, metricsReq)
	if err != nil {
		fmt.Printf("Failed to get metrics: %v\n", err)
	} else {
		fmt.Printf("Strategy metrics - Total P&L: %s, Win rate: %.2f%%\n",
			metricsResp.Metrics.TotalPnl, metricsResp.Metrics.WinRate*100)
	}
	
	// Stop strategy
	time.Sleep(2 * time.Second)
	
	stopReq := &pb.StopStrategyRequest{
		StrategyId: createResp.Strategy.Id,
		Reason:     "Test completed",
	}
	
	stopResp, err := client.StopStrategy(ctx, stopReq)
	if err != nil {
		fmt.Printf("Failed to stop strategy: %v\n", err)
	} else {
		fmt.Printf("Strategy stopped, status: %s\n", stopResp.Strategy.Status)
	}
	
	return createResp.Strategy.Id
}

func testOrderOperations(ctx context.Context, client pb.OrderServiceClient, accountID string) {
	// Create order
	createReq := &pb.OrderRequest{
		AccountId:    accountID,
		Symbol:       "BTCUSDT",
		Side:         pb.OrderSide_BUY,
		Type:         pb.OrderType_LIMIT,
		Quantity:     "0.001",
		Price:        "40000",
		TimeInForce:  pb.TimeInForce_GTC,
	}
	
	createResp, err := client.CreateOrder(ctx, createReq)
	if err != nil {
		fmt.Printf("Failed to create order: %v\n", err)
		return
	}
	
	fmt.Printf("Created order: %s\n", createResp.Order.Id)
	
	// Get order details
	getReq := &pb.GetOrderRequest{
		OrderId: createResp.Order.Id,
	}
	
	getResp, err := client.GetOrder(ctx, getReq)
	if err != nil {
		fmt.Printf("Failed to get order: %v\n", err)
	} else {
		fmt.Printf("Order status: %s, filled: %s/%s\n",
			getResp.Order.Status, getResp.Order.FilledQuantity, getResp.Order.Quantity)
	}
	
	// List orders
	listReq := &pb.ListOrdersRequest{
		AccountId: accountID,
		Symbol:    "BTCUSDT",
		Limit:     10,
	}
	
	listResp, err := client.ListOrders(ctx, listReq)
	if err != nil {
		fmt.Printf("Failed to list orders: %v\n", err)
	} else {
		fmt.Printf("Found %d orders\n", len(listResp.Orders))
	}
	
	// Cancel order
	cancelReq := &pb.CancelOrderRequest{
		OrderId: createResp.Order.Id,
	}
	
	cancelResp, err := client.CancelOrder(ctx, cancelReq)
	if err != nil {
		fmt.Printf("Failed to cancel order: %v\n", err)
	} else {
		fmt.Printf("Order cancelled, status: %s\n", cancelResp.Order.Status)
	}
}

func testPositionOperations(ctx context.Context, client pb.PositionServiceClient, accountID string) {
	// Get position
	getReq := &pb.GetPositionRequest{
		AccountId: accountID,
		Symbol:    "BTCUSDT",
	}
	
	getResp, err := client.GetPosition(ctx, getReq)
	if err != nil {
		fmt.Printf("Failed to get position: %v\n", err)
	} else if getResp.Position != nil {
		fmt.Printf("Position: %s %s, P&L: %s\n",
			getResp.Position.Symbol, getResp.Position.Quantity, getResp.Position.UnrealizedPnl)
	}
	
	// List all positions
	listReq := &pb.ListPositionsRequest{
		AccountId: accountID,
	}
	
	listResp, err := client.ListPositions(ctx, listReq)
	if err != nil {
		fmt.Printf("Failed to list positions: %v\n", err)
	} else {
		fmt.Printf("Found %d positions\n", len(listResp.Positions))
		for _, pos := range listResp.Positions {
			fmt.Printf("  %s: %s @ %s, P&L: %s\n",
				pos.Symbol, pos.Quantity, pos.EntryPrice, pos.UnrealizedPnl)
		}
	}
	
	// Get aggregated positions
	aggReq := &pb.GetAggregatedPositionsRequest{
		GroupBy: "symbol",
	}
	
	aggResp, err := client.GetAggregatedPositions(ctx, aggReq)
	if err != nil {
		fmt.Printf("Failed to get aggregated positions: %v\n", err)
	} else {
		fmt.Printf("Aggregated positions:\n")
		for symbol, pos := range aggResp.Positions {
			fmt.Printf("  %s: Net %s, Total value: %s\n",
				symbol, pos.NetQuantity, pos.TotalValue)
		}
	}
	
	// Get risk metrics
	riskReq := &pb.GetRiskMetricsRequest{
		AccountId: accountID,
	}
	
	riskResp, err := client.GetRiskMetrics(ctx, riskReq)
	if err != nil {
		fmt.Printf("Failed to get risk metrics: %v\n", err)
	} else {
		fmt.Printf("Risk metrics - Leverage: %.2f, Margin ratio: %.2f%%\n",
			riskResp.Metrics.LeverageRatio, riskResp.Metrics.MarginRatio*100)
	}
}