package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	pb "github.com/mExOms/proto/oms/v1"
	"github.com/mExOms/internal/account"
	"github.com/mExOms/internal/api/grpc"
	"github.com/mExOms/internal/keymanager"
	"github.com/mExOms/internal/orders"
	"github.com/mExOms/internal/position"
	"github.com/mExOms/internal/storage"
	"github.com/mExOms/pkg/types"
)

func main() {
	fmt.Println("=== gRPC API Gateway Test ===")

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := startTestServer(ctx)
	time.Sleep(2 * time.Second) // Wait for server to start

	// Create client
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Create service clients
	accountClient := pb.NewAccountServiceClient(conn)
	orderClient := pb.NewOrderServiceClient(conn)

	// Add API key to context
	ctx = metadata.AppendToOutgoingContext(ctx, "x-api-key", "test-api-key")

	fmt.Println("\n=== Test 1: Create Account ===")
	testCreateAccount(ctx, accountClient)

	fmt.Println("\n=== Test 2: List Accounts ===")
	testListAccounts(ctx, accountClient)

	fmt.Println("\n=== Test 3: Get Account Balance ===")
	testGetBalance(ctx, accountClient)

	fmt.Println("\n=== Test 4: Account Transfer ===")
	testTransfer(ctx, accountClient)

	fmt.Println("\n=== Test 5: Create Order ===")
	testCreateOrder(ctx, orderClient)

	fmt.Println("\n=== Test Completed ===")
}

func startTestServer(ctx context.Context) *grpc.Server {
	// Initialize dependencies
	storageConfig := storage.StorageConfig{
		BasePath:           "/tmp/mexoms/grpc-test",
		MaxFileSize:        100 * 1024 * 1024,
		RotationInterval:   24 * time.Hour,
		CompressionEnabled: true,
		RetentionDays:      30,
	}

	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		log.Fatalf("Failed to create storage manager: %v", err)
	}

	// Create key manager
	keyManager := keymanager.NewVaultKeyManager(keymanager.VaultConfig{
		Address:    "http://localhost:8200",
		Token:      "dev-token",
		MountPath:  "secret",
		PathPrefix: "exchanges",
	})

	// Create account manager
	accountManager := account.NewManager(keyManager, storageManager, nil)

	// Create position manager
	positionManager := position.NewMultiAccountPositionManager(accountManager, nil)

	// Create transfer manager
	transferManager := account.NewTransferManager(accountManager)

	// Create order manager
	orderManager := orders.NewManager(nil, accountManager, storageManager)

	// Create and start server
	config := &grpc.Config{
		Port:             50051,
		EnableReflection: true,
		EnableAuth:       true,
	}

	server := grpc.NewServer(config, accountManager, orderManager, positionManager, transferManager)

	go func() {
		if err := server.Start(ctx); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	return server
}

func testCreateAccount(ctx context.Context, client pb.AccountServiceClient) {
	// Create main account
	req := &pb.CreateAccountRequest{
		Name:             "Test Main Account",
		Type:             pb.AccountType_ACCOUNT_TYPE_MAIN,
		Exchange:         "binance",
		Strategy:         "",
		SpotEnabled:      true,
		FuturesEnabled:   true,
		MaxPositionUsdt:  "100000",
		MaxLeverage:      20,
	}

	resp, err := client.CreateAccount(ctx, req)
	if err != nil {
		log.Printf("Failed to create account: %v", err)
		return
	}

	fmt.Printf("Created account: %s (%s)\n", resp.Account.Name, resp.Account.Id)

	// Create sub account
	req2 := &pb.CreateAccountRequest{
		Name:             "Test Arbitrage Account",
		Type:             pb.AccountType_ACCOUNT_TYPE_SUB,
		Exchange:         "binance",
		Strategy:         "arbitrage",
		SpotEnabled:      true,
		FuturesEnabled:   false,
		MaxPositionUsdt:  "50000",
		MaxLeverage:      10,
	}

	resp2, err := client.CreateAccount(ctx, req2)
	if err != nil {
		log.Printf("Failed to create sub account: %v", err)
		return
	}

	fmt.Printf("Created sub account: %s (%s)\n", resp2.Account.Name, resp2.Account.Id)
}

func testListAccounts(ctx context.Context, client pb.AccountServiceClient) {
	req := &pb.ListAccountsRequest{
		ActiveOnly: true,
		Exchange:   "binance",
	}

	resp, err := client.ListAccounts(ctx, req)
	if err != nil {
		log.Printf("Failed to list accounts: %v", err)
		return
	}

	fmt.Printf("Found %d accounts:\n", len(resp.Accounts))
	for _, acc := range resp.Accounts {
		fmt.Printf("  - %s (%s): %s, Strategy: %s\n", 
			acc.Name, acc.Id, acc.Type, acc.Strategy)
	}
}

func testGetBalance(ctx context.Context, client pb.AccountServiceClient) {
	// Get first account
	listResp, err := client.ListAccounts(ctx, &pb.ListAccountsRequest{})
	if err != nil || len(listResp.Accounts) == 0 {
		log.Printf("No accounts found")
		return
	}

	accountID := listResp.Accounts[0].Id
	req := &pb.GetAccountBalanceRequest{
		AccountId: accountID,
	}

	resp, err := client.GetAccountBalance(ctx, req)
	if err != nil {
		log.Printf("Failed to get balance: %v", err)
		return
	}

	fmt.Printf("Balance for account %s:\n", accountID)
	for _, balance := range resp.Balances {
		fmt.Printf("  %s: Total=%s, Available=%s, Locked=%s\n",
			balance.Asset, balance.Total, balance.Available, balance.Locked)
	}
}

func testTransfer(ctx context.Context, client pb.AccountServiceClient) {
	// Get accounts
	listResp, err := client.ListAccounts(ctx, &pb.ListAccountsRequest{})
	if err != nil || len(listResp.Accounts) < 2 {
		log.Printf("Need at least 2 accounts for transfer")
		return
	}

	req := &pb.TransferRequest{
		FromAccount: listResp.Accounts[0].Id,
		ToAccount:   listResp.Accounts[1].Id,
		Asset:       "USDT",
		Amount:      "1000",
		Reason:      "Test transfer",
	}

	resp, err := client.Transfer(ctx, req)
	if err != nil {
		log.Printf("Failed to transfer: %v", err)
		return
	}

	fmt.Printf("Transfer completed: %s -> %s, Amount: %s %s, Status: %s\n",
		resp.Transfer.FromAccount, resp.Transfer.ToAccount,
		resp.Transfer.Amount, resp.Transfer.Asset,
		resp.Transfer.Status)
}

func testCreateOrder(ctx context.Context, client pb.OrderServiceClient) {
	req := &pb.OrderRequest{
		Exchange:      "binance",
		Symbol:        "BTCUSDT",
		Side:          pb.OrderSide_ORDER_SIDE_BUY,
		Type:          pb.OrderType_ORDER_TYPE_LIMIT,
		Quantity:      "0.001",
		Price:         "30000",
		TimeInForce:   pb.TimeInForce_TIME_IN_FORCE_GTC,
		ClientOrderId: fmt.Sprintf("test-%d", time.Now().Unix()),
	}

	resp, err := client.CreateOrder(ctx, req)
	if err != nil {
		log.Printf("Failed to create order: %v", err)
		return
	}

	fmt.Printf("Order created: %s, Status: %s\n", 
		resp.Order.OrderId, resp.Order.Status)
}

// Mock account manager for testing
type mockAccountManager struct {
	accounts map[string]*types.Account
	balances map[string]*types.AccountBalance
}

func createMockAccountManager() types.AccountManager {
	return &mockAccountManager{
		accounts: make(map[string]*types.Account),
		balances: make(map[string]*types.AccountBalance),
	}
}

// Implement AccountManager interface methods...
// (Same implementation as in the position manager test)