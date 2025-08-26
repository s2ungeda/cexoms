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
)

// Client demonstrates how to use the OMS gRPC API
type Client struct {
	conn          *grpc.ClientConn
	accountClient pb.AccountServiceClient
	orderClient   pb.OrderServiceClient
	apiKey        string
}

// NewClient creates a new OMS client
func NewClient(address, apiKey string) (*Client, error) {
	// Create connection
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	return &Client{
		conn:          conn,
		accountClient: pb.NewAccountServiceClient(conn),
		orderClient:   pb.NewOrderServiceClient(conn),
		apiKey:        apiKey,
	}, nil
}

// Close closes the client connection
func (c *Client) Close() error {
	return c.conn.Close()
}

// getContext returns context with authentication
func (c *Client) getContext() context.Context {
	ctx := context.Background()
	return metadata.AppendToOutgoingContext(ctx, "x-api-key", c.apiKey)
}

// CreateAccount creates a new trading account
func (c *Client) CreateAccount(name, exchange, strategy string) (*pb.Account, error) {
	req := &pb.CreateAccountRequest{
		Name:             name,
		Type:             pb.AccountType_ACCOUNT_TYPE_SUB,
		Exchange:         exchange,
		Strategy:         strategy,
		SpotEnabled:      true,
		FuturesEnabled:   true,
		MaxPositionUsdt:  "50000",
		MaxLeverage:      10,
	}

	resp, err := c.accountClient.CreateAccount(c.getContext(), req)
	if err != nil {
		return nil, err
	}

	return resp.Account, nil
}

// ListAccounts lists all accounts
func (c *Client) ListAccounts() ([]*pb.Account, error) {
	req := &pb.ListAccountsRequest{
		ActiveOnly: true,
	}

	resp, err := c.accountClient.ListAccounts(c.getContext(), req)
	if err != nil {
		return nil, err
	}

	return resp.Accounts, nil
}

// GetBalance gets account balance
func (c *Client) GetBalance(accountID string) ([]*pb.AccountBalance, error) {
	req := &pb.GetAccountBalanceRequest{
		AccountId: accountID,
	}

	resp, err := c.accountClient.GetAccountBalance(c.getContext(), req)
	if err != nil {
		return nil, err
	}

	return resp.Balances, nil
}

// Transfer transfers assets between accounts
func (c *Client) Transfer(from, to, asset, amount string) (*pb.AccountTransfer, error) {
	req := &pb.TransferRequest{
		FromAccount: from,
		ToAccount:   to,
		Asset:       asset,
		Amount:      amount,
		Reason:      "Client transfer",
	}

	resp, err := c.accountClient.Transfer(c.getContext(), req)
	if err != nil {
		return nil, err
	}

	return resp.Transfer, nil
}

// CreateOrder creates a new order
func (c *Client) CreateOrder(accountID, symbol string, side pb.OrderSide, quantity, price string) (*pb.Order, error) {
	req := &pb.OrderRequest{
		AccountId:     accountID,
		Exchange:      "binance",
		Symbol:        symbol,
		Side:          side,
		Type:          pb.OrderType_ORDER_TYPE_LIMIT,
		Quantity:      quantity,
		Price:         price,
		TimeInForce:   pb.TimeInForce_TIME_IN_FORCE_GTC,
		ClientOrderId: fmt.Sprintf("client-%d", time.Now().UnixNano()),
	}

	resp, err := c.orderClient.CreateOrder(c.getContext(), req)
	if err != nil {
		return nil, err
	}

	return resp.Order, nil
}

// GetOrder gets order details
func (c *Client) GetOrder(orderID string) (*pb.Order, error) {
	req := &pb.GetOrderRequest{
		OrderId: orderID,
	}

	resp, err := c.orderClient.GetOrder(c.getContext(), req)
	if err != nil {
		return nil, err
	}

	return resp.Order, nil
}

// CancelOrder cancels an order
func (c *Client) CancelOrder(orderID string) error {
	req := &pb.CancelOrderRequest{
		OrderId: orderID,
	}

	_, err := c.orderClient.CancelOrder(c.getContext(), req)
	return err
}

func main() {
	// Create client
	client, err := NewClient("localhost:50051", "your-api-key-here")
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	fmt.Println("=== OMS gRPC Client Example ===")

	// Example 1: List accounts
	fmt.Println("\n1. Listing accounts...")
	accounts, err := client.ListAccounts()
	if err != nil {
		log.Printf("Failed to list accounts: %v", err)
	} else {
		for _, acc := range accounts {
			fmt.Printf("   Account: %s (%s) - %s\n", acc.Name, acc.Id, acc.Strategy)
		}
	}

	// Example 2: Create account
	fmt.Println("\n2. Creating new account...")
	newAccount, err := client.CreateAccount("My Trading Bot", "binance", "market_making")
	if err != nil {
		log.Printf("Failed to create account: %v", err)
	} else {
		fmt.Printf("   Created: %s (%s)\n", newAccount.Name, newAccount.Id)
	}

	// Example 3: Get balance
	if len(accounts) > 0 {
		fmt.Println("\n3. Getting account balance...")
		balances, err := client.GetBalance(accounts[0].Id)
		if err != nil {
			log.Printf("Failed to get balance: %v", err)
		} else {
			for _, bal := range balances {
				fmt.Printf("   %s: %s (available: %s)\n", bal.Asset, bal.Total, bal.Available)
			}
		}
	}

	// Example 4: Create order
	if len(accounts) > 0 {
		fmt.Println("\n4. Creating order...")
		order, err := client.CreateOrder(
			accounts[0].Id,
			"BTCUSDT",
			pb.OrderSide_ORDER_SIDE_BUY,
			"0.001",
			"30000",
		)
		if err != nil {
			log.Printf("Failed to create order: %v", err)
		} else {
			fmt.Printf("   Order created: %s (status: %s)\n", order.OrderId, order.Status)

			// Cancel the order
			fmt.Println("\n5. Canceling order...")
			if err := client.CancelOrder(order.OrderId); err != nil {
				log.Printf("Failed to cancel order: %v", err)
			} else {
				fmt.Println("   Order canceled successfully")
			}
		}
	}

	// Example 5: Transfer between accounts
	if len(accounts) >= 2 {
		fmt.Println("\n6. Transferring between accounts...")
		transfer, err := client.Transfer(
			accounts[0].Id,
			accounts[1].Id,
			"USDT",
			"100",
		)
		if err != nil {
			log.Printf("Failed to transfer: %v", err)
		} else {
			fmt.Printf("   Transfer %s: %s USDT from %s to %s\n",
				transfer.Status, transfer.Amount, transfer.FromAccount, transfer.ToAccount)
		}
	}

	fmt.Println("\n=== Example completed ===")
}