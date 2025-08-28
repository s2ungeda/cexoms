package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/your-org/mExOms/internal/account"
	"github.com/your-org/mExOms/pkg/types"
	pb "github.com/your-org/mExOms/pkg/proto/oms/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// MultiAccountTrader demonstrates trading across multiple accounts
type MultiAccountTrader struct {
	client      pb.OrderServiceClient
	accounts    map[string]*types.Account
	mu          sync.RWMutex
}

func main() {
	// Connect to OMS server
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewOrderServiceClient(conn)
	ctx := context.Background()

	// Initialize multi-account trader
	trader := &MultiAccountTrader{
		client:   client,
		accounts: make(map[string]*types.Account),
	}

	// Example 1: Setup multiple accounts
	fmt.Println("=== Setting up Multiple Accounts ===")
	accounts := []string{"main", "arbitrage", "market_maker", "dca_bot"}
	
	for _, accName := range accounts {
		acc, err := trader.setupAccount(ctx, accName)
		if err != nil {
			log.Printf("Failed to setup account %s: %v", accName, err)
			continue
		}
		trader.accounts[accName] = acc
		fmt.Printf("Account '%s' setup successfully\n", accName)
	}

	// Example 2: Check balances across all accounts
	fmt.Println("\n=== Account Balances ===")
	trader.displayAllBalances(ctx)

	// Example 3: Distribute orders across accounts based on strategy
	fmt.Println("\n=== Distributed Order Execution ===")
	
	// Main account - Large position trade
	mainOrder := &pb.CreateOrderRequest{
		AccountId: "main",
		Exchange:  "binance",
		Market:    "spot",
		Symbol:    "BTC/USDT",
		Side:      pb.OrderSide_BUY,
		Type:      pb.OrderType_LIMIT,
		Price:     65000,
		Quantity:  0.1,
	}
	
	// Arbitrage account - Quick arbitrage opportunity
	arbOrder := &pb.CreateOrderRequest{
		AccountId: "arbitrage",
		Exchange:  "binance",
		Market:    "spot",
		Symbol:    "ETH/USDT",
		Side:      pb.OrderSide_BUY,
		Type:      pb.OrderType_MARKET,
		Quantity:  0.5,
	}
	
	// Market maker account - Provide liquidity
	mmBuyOrder := &pb.CreateOrderRequest{
		AccountId: "market_maker",
		Exchange:  "binance",
		Market:    "spot",
		Symbol:    "BNB/USDT",
		Side:      pb.OrderSide_BUY,
		Type:      pb.OrderType_LIMIT,
		Price:     580,
		Quantity:  1.0,
	}
	
	mmSellOrder := &pb.CreateOrderRequest{
		AccountId: "market_maker",
		Exchange:  "binance",
		Market:    "spot",
		Symbol:    "BNB/USDT",
		Side:      pb.OrderSide_SELL,
		Type:      pb.OrderType_LIMIT,
		Price:     582,
		Quantity:  1.0,
	}

	// Execute orders concurrently
	var wg sync.WaitGroup
	orderChan := make(chan *pb.CreateOrderResponse, 4)
	
	// Place orders concurrently
	wg.Add(4)
	go trader.placeOrderAsync(ctx, mainOrder, orderChan, &wg)
	go trader.placeOrderAsync(ctx, arbOrder, orderChan, &wg)
	go trader.placeOrderAsync(ctx, mmBuyOrder, orderChan, &wg)
	go trader.placeOrderAsync(ctx, mmSellOrder, orderChan, &wg)
	
	// Wait for all orders to complete
	go func() {
		wg.Wait()
		close(orderChan)
	}()
	
	// Process order results
	for resp := range orderChan {
		if resp != nil && resp.Order != nil {
			fmt.Printf("Order %s placed on account '%s': Status=%s\n",
				resp.Order.OrderId,
				resp.Order.AccountId,
				resp.Order.Status)
		}
	}

	// Example 4: Account performance comparison
	fmt.Println("\n=== Account Performance ===")
	trader.compareAccountPerformance(ctx)

	// Example 5: Risk distribution across accounts
	fmt.Println("\n=== Risk Distribution ===")
	trader.analyzeRiskDistribution(ctx)

	// Example 6: Rebalancing between accounts
	fmt.Println("\n=== Account Rebalancing ===")
	trader.rebalanceAccounts(ctx, 10000.0) // Target $10,000 per account
}

func (t *MultiAccountTrader) setupAccount(ctx context.Context, accountId string) (*types.Account, error) {
	// Get account info from server
	resp, err := t.client.GetAccount(ctx, &pb.GetAccountRequest{
		AccountId: accountId,
	})
	if err != nil {
		return nil, err
	}
	
	return &types.Account{
		ID:       resp.Account.Id,
		Name:     resp.Account.Name,
		Type:     types.AccountType(resp.Account.Type),
		Exchange: resp.Account.Exchange,
		Status:   types.AccountStatus(resp.Account.Status),
	}, nil
}

func (t *MultiAccountTrader) placeOrderAsync(ctx context.Context, order *pb.CreateOrderRequest, 
	respChan chan<- *pb.CreateOrderResponse, wg *sync.WaitGroup) {
	defer wg.Done()
	
	resp, err := t.client.CreateOrder(ctx, order)
	if err != nil {
		log.Printf("Failed to place order on account %s: %v", order.AccountId, err)
		return
	}
	
	respChan <- resp
}

func (t *MultiAccountTrader) displayAllBalances(ctx context.Context) {
	totalUSDT := 0.0
	
	for accName, acc := range t.accounts {
		balance, err := t.client.GetBalance(ctx, &pb.GetBalanceRequest{
			AccountId: acc.ID,
			Exchange:  "binance",
			Market:    "spot",
		})
		if err != nil {
			log.Printf("Failed to get balance for %s: %v", accName, err)
			continue
		}
		
		accountTotal := 0.0
		fmt.Printf("\nAccount: %s\n", accName)
		for _, asset := range balance.Assets {
			if asset.Free > 0 || asset.Locked > 0 {
				total := asset.Free + asset.Locked
				fmt.Printf("  %s: %.4f (Free: %.4f, Locked: %.4f)\n", 
					asset.Symbol, total, asset.Free, asset.Locked)
				
				if asset.Symbol == "USDT" {
					accountTotal += total
				}
			}
		}
		fmt.Printf("  Account USDT Value: $%.2f\n", accountTotal)
		totalUSDT += accountTotal
	}
	
	fmt.Printf("\nTotal Portfolio Value: $%.2f\n", totalUSDT)
}

func (t *MultiAccountTrader) compareAccountPerformance(ctx context.Context) {
	for accName, acc := range t.accounts {
		stats, err := t.client.GetAccountStats(ctx, &pb.GetAccountStatsRequest{
			AccountId: acc.ID,
			Period:    pb.TimePeriod_DAY,
		})
		if err != nil {
			log.Printf("Failed to get stats for %s: %v", accName, err)
			continue
		}
		
		fmt.Printf("\nAccount: %s\n", accName)
		fmt.Printf("  24h PnL: $%.2f (%.2f%%)\n", stats.Pnl, stats.PnlPercent)
		fmt.Printf("  Win Rate: %.1f%%\n", stats.WinRate)
		fmt.Printf("  Total Trades: %d\n", stats.TotalTrades)
		fmt.Printf("  Active Positions: %d\n", stats.ActivePositions)
	}
}

func (t *MultiAccountTrader) analyzeRiskDistribution(ctx context.Context) {
	totalRisk := 0.0
	
	for accName, acc := range t.accounts {
		risk, err := t.client.GetAccountRisk(ctx, &pb.GetAccountRiskRequest{
			AccountId: acc.ID,
		})
		if err != nil {
			log.Printf("Failed to get risk for %s: %v", accName, err)
			continue
		}
		
		fmt.Printf("\nAccount: %s\n", accName)
		fmt.Printf("  Position Risk: $%.2f\n", risk.PositionRisk)
		fmt.Printf("  Leverage: %.2fx\n", risk.Leverage)
		fmt.Printf("  Margin Usage: %.1f%%\n", risk.MarginUsage)
		fmt.Printf("  Max Drawdown: %.2f%%\n", risk.MaxDrawdown)
		
		totalRisk += risk.PositionRisk
	}
	
	fmt.Printf("\nTotal Portfolio Risk: $%.2f\n", totalRisk)
}

func (t *MultiAccountTrader) rebalanceAccounts(ctx context.Context, targetBalance float64) {
	fmt.Printf("Rebalancing accounts to target: $%.2f each\n", targetBalance)
	
	// Get current balances
	balances := make(map[string]float64)
	totalBalance := 0.0
	
	for accName, acc := range t.accounts {
		balance, err := t.client.GetBalance(ctx, &pb.GetBalanceRequest{
			AccountId: acc.ID,
			Exchange:  "binance",
			Market:    "spot",
		})
		if err != nil {
			continue
		}
		
		for _, asset := range balance.Assets {
			if asset.Symbol == "USDT" {
				balances[accName] = asset.Free + asset.Locked
				totalBalance += balances[accName]
				break
			}
		}
	}
	
	// Calculate transfers needed
	avgBalance := totalBalance / float64(len(t.accounts))
	fmt.Printf("Current average balance: $%.2f\n", avgBalance)
	
	// Plan transfers
	transfers := []struct {
		from   string
		to     string
		amount float64
	}{}
	
	for accName, balance := range balances {
		diff := balance - targetBalance
		if diff > 10 { // Only transfer if difference > $10
			// Find account that needs funds
			for toAcc, toBalance := range balances {
				if toAcc != accName && toBalance < targetBalance-10 {
					transferAmount := min(diff, targetBalance-toBalance)
					transfers = append(transfers, struct {
						from   string
						to     string
						amount float64
					}{accName, toAcc, transferAmount})
					
					// Update balances for next iteration
					balances[accName] -= transferAmount
					balances[toAcc] += transferAmount
					break
				}
			}
		}
	}
	
	// Execute transfers
	for _, transfer := range transfers {
		fmt.Printf("Transferring $%.2f from %s to %s\n", 
			transfer.amount, transfer.from, transfer.to)
		
		_, err := t.client.TransferBetweenAccounts(ctx, &pb.TransferRequest{
			FromAccountId: transfer.from,
			ToAccountId:   transfer.to,
			Asset:         "USDT",
			Amount:        transfer.amount,
		})
		if err != nil {
			log.Printf("Transfer failed: %v", err)
		}
	}
	
	fmt.Println("Rebalancing complete!")
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}