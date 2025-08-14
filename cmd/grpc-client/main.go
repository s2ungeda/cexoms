package main

import (
	"context"
	"fmt"
	"log"
	"time"

	omsv1 "github.com/mExOms/oms/pkg/proto/oms/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func main() {
	// Connect to gRPC server
	conn, err := grpc.Dial("localhost:9090", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal("Failed to connect:", err)
	}
	defer conn.Close()

	fmt.Println("=== gRPC Client Example ===\n")

	// Create service clients
	authClient := omsv1.NewAuthServiceClient(conn)
	orderClient := omsv1.NewOrderServiceClient(conn)
	positionClient := omsv1.NewPositionServiceClient(conn)

	// 1. Authenticate with API key
	fmt.Println("1. Authenticating with API key...")
	authResp, err := authClient.Authenticate(context.Background(), &omsv1.AuthRequest{
		ApiKey: "demo-api-key",
		Secret: "demo-secret",
	})
	if err != nil {
		log.Fatal("Authentication failed:", err)
	}
	fmt.Printf("✓ Authenticated successfully\n")
	fmt.Printf("  Token: %s...\n", authResp.Token[:20])
	fmt.Printf("  Permissions: %v\n", authResp.Permissions)
	fmt.Println()

	// Create context with auth token
	authCtx := metadata.AppendToOutgoingContext(context.Background(),
		"authorization", fmt.Sprintf("Bearer %s", authResp.Token))

	// 2. Create an order
	fmt.Println("2. Creating a limit order...")
	orderReq := &omsv1.OrderRequest{
		Exchange: "binance",
		Symbol:   "BTCUSDT",
		Side:     omsv1.OrderSide_ORDER_SIDE_BUY,
		Type:     omsv1.OrderType_ORDER_TYPE_LIMIT,
		Price:    &omsv1.Decimal{Value: "42000"},
		Quantity: &omsv1.Decimal{Value: "0.1"},
		TimeInForce: omsv1.TimeInForce_TIME_IN_FORCE_GTC,
		Market:   omsv1.Market_MARKET_SPOT,
	}

	orderResp, err := orderClient.CreateOrder(authCtx, orderReq)
	if err != nil {
		// In demo mode, this will fail as exchange is not connected
		fmt.Printf("✗ Order creation failed (expected in demo): %v\n", err)
	} else {
		fmt.Printf("✓ Order created successfully\n")
		fmt.Printf("  Order ID: %s\n", orderResp.Order.Id)
		fmt.Printf("  Status: %s\n", orderResp.Order.Status)
	}
	fmt.Println()

	// 3. List positions
	fmt.Println("3. Listing all positions...")
	positionsResp, err := positionClient.ListPositions(authCtx, &omsv1.ListPositionsRequest{})
	if err != nil {
		log.Printf("Failed to list positions: %v", err)
	} else {
		fmt.Printf("✓ Found %d positions\n", positionsResp.Total)
		for i, pos := range positionsResp.Positions {
			if i < 5 { // Show first 5
				fmt.Printf("  - %s on %s: %s @ %s (P&L: %s)\n",
					pos.Symbol, pos.Exchange, pos.Quantity.Value,
					pos.EntryPrice.Value, pos.UnrealizedPnl.Value)
			}
		}
		if len(positionsResp.Positions) > 5 {
			fmt.Printf("  ... and %d more\n", len(positionsResp.Positions)-5)
		}
	}
	fmt.Println()

	// 4. Get risk metrics
	fmt.Println("4. Getting risk metrics...")
	riskResp, err := positionClient.GetRiskMetrics(authCtx, &omsv1.GetRiskMetricsRequest{})
	if err != nil {
		log.Printf("Failed to get risk metrics: %v", err)
	} else {
		fmt.Printf("✓ Risk metrics:\n")
		fmt.Printf("  Position count: %d\n", riskResp.Metrics.PositionCount)
		fmt.Printf("  Total value: %s\n", riskResp.Metrics.TotalValue.Value)
		fmt.Printf("  Total P&L: %s\n", riskResp.Metrics.TotalPnl.Value)
		fmt.Printf("  Max leverage: %sx\n", riskResp.Metrics.MaxLeverage.Value)
		fmt.Printf("  Avg calc time: %.2f μs\n", riskResp.Metrics.AvgCalcTimeUs)
	}
	fmt.Println()

	// 5. Test rate limiting
	fmt.Println("5. Testing rate limiting...")
	start := time.Now()
	successCount := 0
	rateLimitedCount := 0

	for i := 0; i < 150; i++ {
		_, err := positionClient.ListPositions(authCtx, &omsv1.ListPositionsRequest{})
		if err != nil {
			if err.Error() == "rpc error: code = ResourceExhausted desc = rate limit exceeded" {
				rateLimitedCount++
			}
		} else {
			successCount++
		}
	}

	elapsed := time.Since(start)
	fmt.Printf("✓ Sent 150 requests in %v\n", elapsed)
	fmt.Printf("  Successful: %d\n", successCount)
	fmt.Printf("  Rate limited: %d\n", rateLimitedCount)
	fmt.Println()

	// 6. Create API key (admin only)
	fmt.Println("6. Creating new API key...")
	apiKeyResp, err := authClient.CreateAPIKey(authCtx, &omsv1.CreateAPIKeyRequest{
		Name: "Test API Key",
		Permissions: []omsv1.Permission{
			omsv1.Permission_PERMISSION_READ_ORDERS,
			omsv1.Permission_PERMISSION_READ_POSITIONS,
		},
	})
	if err != nil {
		fmt.Printf("✗ Failed to create API key (expected - requires admin): %v\n", err)
	} else {
		fmt.Printf("✓ API key created:\n")
		fmt.Printf("  ID: %s\n", apiKeyResp.ApiKey.Id)
		fmt.Printf("  Secret: %s\n", apiKeyResp.Secret)
	}
	fmt.Println()

	// 7. Test streaming (when implemented)
	fmt.Println("7. Market data streaming:")
	fmt.Println("✗ Not implemented yet (coming soon)")
	fmt.Println()

	fmt.Println("=== Client example completed ===")
}