package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"sort"
	"time"

	pb "github.com/your-org/mExOms/proto/gen/go/oms/v1"
	"google.golang.org/grpc"
)

// PortfolioConfig defines target allocations
type PortfolioConfig struct {
	Name              string                       `json:"name"`
	Exchange          string                       `json:"exchange"`
	TargetAllocations map[string]float64          `json:"target_allocations"` // Symbol -> percentage
	RebalanceThreshold float64                     `json:"rebalance_threshold"` // Deviation threshold
	MinTradeValue     float64                      `json:"min_trade_value"`     // Minimum trade in USDT
	CheckInterval     time.Duration                `json:"check_interval_hours"`
	MaxSlippage       float64                      `json:"max_slippage"`        // Maximum price slippage
}

// Asset represents a portfolio holding
type Asset struct {
	Symbol         string
	Quantity       float64
	CurrentPrice   float64
	Value          float64
	CurrentPercent float64
	TargetPercent  float64
	Deviation      float64
}

// RebalanceAction represents a trade to execute
type RebalanceAction struct {
	Symbol    string
	Side      pb.OrderSide
	Quantity  float64
	Value     float64
	Reason    string
}

// AutoRebalancer manages portfolio rebalancing
type AutoRebalancer struct {
	client    pb.OrderServiceClient
	ctx       context.Context
	config    PortfolioConfig
	portfolio map[string]*Asset
}

func NewAutoRebalancer(conn *grpc.ClientConn, configFile string) (*AutoRebalancer, error) {
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %v", err)
	}
	
	var config PortfolioConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}
	
	// Validate allocations sum to 100%
	var totalAllocation float64
	for _, percent := range config.TargetAllocations {
		totalAllocation += percent
	}
	if math.Abs(totalAllocation-100.0) > 0.01 {
		return nil, fmt.Errorf("target allocations must sum to 100%%, got %.2f%%", totalAllocation)
	}
	
	return &AutoRebalancer{
		client:    pb.NewOrderServiceClient(conn),
		ctx:       context.Background(),
		config:    config,
		portfolio: make(map[string]*Asset),
	}, nil
}

func (r *AutoRebalancer) Run() {
	fmt.Printf("=== Auto Portfolio Rebalancer ===\n")
	fmt.Printf("Portfolio: %s\n", r.config.Name)
	fmt.Printf("Exchange: %s\n", r.config.Exchange)
	fmt.Printf("Rebalance Threshold: %.1f%%\n", r.config.RebalanceThreshold)
	fmt.Printf("Check Interval: %s\n", r.config.CheckInterval)
	
	fmt.Println("\nTarget Allocations:")
	for symbol, percent := range r.config.TargetAllocations {
		fmt.Printf("  %s: %.1f%%\n", symbol, percent)
	}
	fmt.Println()
	
	// Initial portfolio check
	r.checkAndRebalance()
	
	// Set up periodic checks
	ticker := time.NewTicker(r.config.CheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			r.checkAndRebalance()
		case <-r.ctx.Done():
			fmt.Println("Rebalancer stopped")
			return
		}
	}
}

func (r *AutoRebalancer) checkAndRebalance() {
	fmt.Printf("\n[%s] Checking portfolio...\n", 
		time.Now().Format("2006-01-02 15:04:05"))
	
	// Load current portfolio
	if err := r.loadPortfolio(); err != nil {
		log.Printf("Failed to load portfolio: %v", err)
		return
	}
	
	// Display current state
	r.displayPortfolio()
	
	// Check if rebalancing is needed
	actions := r.calculateRebalanceActions()
	
	if len(actions) == 0 {
		fmt.Println("✓ Portfolio is balanced within threshold")
		return
	}
	
	// Execute rebalancing
	fmt.Printf("\n🔄 Rebalancing needed - %d actions\n", len(actions))
	r.executeRebalance(actions)
}

func (r *AutoRebalancer) loadPortfolio() error {
	// Get account balances
	balances, err := r.client.GetBalance(r.ctx, &pb.GetBalanceRequest{
		Exchange: r.config.Exchange,
	})
	if err != nil {
		return err
	}
	
	// Clear existing portfolio
	r.portfolio = make(map[string]*Asset)
	
	// Get prices for all assets
	var totalValue float64
	
	for symbol := range r.config.TargetAllocations {
		// Get balance for base asset (e.g., BTC from BTCUSDT)
		baseAsset := symbol[:len(symbol)-4] // Remove USDT suffix
		
		var quantity float64
		for _, bal := range balances.Balances {
			if bal.Asset == baseAsset {
				quantity = bal.Free + bal.Locked
				break
			}
		}
		
		// Get current price
		ticker, err := r.client.GetTicker(r.ctx, &pb.GetTickerRequest{
			Exchange: r.config.Exchange,
			Symbol:   symbol,
		})
		if err != nil {
			log.Printf("Failed to get price for %s: %v", symbol, err)
			continue
		}
		
		value := quantity * ticker.LastPrice
		totalValue += value
		
		r.portfolio[symbol] = &Asset{
			Symbol:        symbol,
			Quantity:      quantity,
			CurrentPrice:  ticker.LastPrice,
			Value:         value,
			TargetPercent: r.config.TargetAllocations[symbol],
		}
	}
	
	// Add USDT balance
	for _, bal := range balances.Balances {
		if bal.Asset == "USDT" {
			totalValue += bal.Free
			r.portfolio["USDT"] = &Asset{
				Symbol:       "USDT",
				Quantity:     bal.Free,
				CurrentPrice: 1.0,
				Value:        bal.Free,
			}
			break
		}
	}
	
	// Calculate current percentages and deviations
	for _, asset := range r.portfolio {
		if totalValue > 0 {
			asset.CurrentPercent = (asset.Value / totalValue) * 100
			asset.Deviation = asset.CurrentPercent - asset.TargetPercent
		}
	}
	
	return nil
}

func (r *AutoRebalancer) displayPortfolio() {
	fmt.Println("\nCurrent Portfolio:")
	fmt.Println("Symbol    | Quantity  | Price     | Value     | Current% | Target% | Deviation")
	fmt.Println("----------|-----------|-----------|-----------|----------|---------|----------")
	
	// Sort by value
	assets := make([]*Asset, 0, len(r.portfolio))
	for _, asset := range r.portfolio {
		assets = append(assets, asset)
	}
	sort.Slice(assets, func(i, j int) bool {
		return assets[i].Value > assets[j].Value
	})
	
	var totalValue float64
	for _, asset := range assets {
		totalValue += asset.Value
		
		deviationStr := fmt.Sprintf("%.1f%%", asset.Deviation)
		if math.Abs(asset.Deviation) > r.config.RebalanceThreshold {
			deviationStr = fmt.Sprintf("⚠️  %.1f%%", asset.Deviation)
		}
		
		fmt.Printf("%-9s | %9.4f | $%8.2f | $%8.2f | %7.1f%% | %6.1f%% | %s\n",
			asset.Symbol,
			asset.Quantity,
			asset.CurrentPrice,
			asset.Value,
			asset.CurrentPercent,
			asset.TargetPercent,
			deviationStr)
	}
	
	fmt.Printf("\nTotal Portfolio Value: $%.2f\n", totalValue)
}

func (r *AutoRebalancer) calculateRebalanceActions() []RebalanceAction {
	actions := []RebalanceAction{}
	
	// Calculate total portfolio value
	var totalValue float64
	for _, asset := range r.portfolio {
		totalValue += asset.Value
	}
	
	if totalValue < r.config.MinTradeValue {
		fmt.Println("Portfolio value too small for rebalancing")
		return actions
	}
	
	// Find assets that need rebalancing
	for symbol, asset := range r.portfolio {
		if symbol == "USDT" {
			continue // Skip stable coin
		}
		
		// Check if deviation exceeds threshold
		if math.Abs(asset.Deviation) > r.config.RebalanceThreshold {
			// Calculate target value
			targetValue := totalValue * (asset.TargetPercent / 100)
			valueChange := targetValue - asset.Value
			
			// Skip if trade is too small
			if math.Abs(valueChange) < r.config.MinTradeValue {
				continue
			}
			
			var action RebalanceAction
			action.Symbol = symbol
			action.Value = math.Abs(valueChange)
			
			if valueChange > 0 {
				// Need to buy
				action.Side = pb.OrderSide_BUY
				action.Quantity = valueChange / asset.CurrentPrice
				action.Reason = fmt.Sprintf("Below target by %.1f%%", -asset.Deviation)
			} else {
				// Need to sell
				action.Side = pb.OrderSide_SELL
				action.Quantity = -valueChange / asset.CurrentPrice
				action.Reason = fmt.Sprintf("Above target by %.1f%%", asset.Deviation)
			}
			
			actions = append(actions, action)
		}
	}
	
	// Sort actions by value (largest first)
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].Value > actions[j].Value
	})
	
	return actions
}

func (r *AutoRebalancer) executeRebalance(actions []RebalanceAction) {
	fmt.Println("\nRebalancing Actions:")
	for _, action := range actions {
		sideStr := "BUY "
		if action.Side == pb.OrderSide_SELL {
			sideStr = "SELL"
		}
		fmt.Printf("- %s %.6f %s (~$%.2f) - %s\n",
			sideStr, action.Quantity, action.Symbol, action.Value, action.Reason)
	}
	
	// Execute sells first to get USDT
	var successCount int
	
	// Execute SELL orders first
	for _, action := range actions {
		if action.Side == pb.OrderSide_SELL {
			if r.executeTrade(action) {
				successCount++
			}
			time.Sleep(1 * time.Second) // Rate limiting
		}
	}
	
	// Wait for sells to settle
	time.Sleep(3 * time.Second)
	
	// Execute BUY orders
	for _, action := range actions {
		if action.Side == pb.OrderSide_BUY {
			if r.executeTrade(action) {
				successCount++
			}
			time.Sleep(1 * time.Second)
		}
	}
	
	fmt.Printf("\n✓ Rebalancing complete: %d/%d trades successful\n",
		successCount, len(actions))
	
	// Save rebalance history
	r.saveRebalanceHistory(actions)
}

func (r *AutoRebalancer) executeTrade(action RebalanceAction) bool {
	// Get current orderbook for better execution
	book, err := r.client.GetOrderBook(r.ctx, &pb.GetOrderBookRequest{
		Exchange: r.config.Exchange,
		Symbol:   action.Symbol,
		Limit:    5,
	})
	if err != nil {
		log.Printf("Failed to get orderbook for %s: %v", action.Symbol, err)
		return false
	}
	
	// Calculate limit price with slippage protection
	var limitPrice float64
	if action.Side == pb.OrderSide_BUY && len(book.Asks) > 0 {
		limitPrice = book.Asks[0].Price * (1 + r.config.MaxSlippage/100)
	} else if action.Side == pb.OrderSide_SELL && len(book.Bids) > 0 {
		limitPrice = book.Bids[0].Price * (1 - r.config.MaxSlippage/100)
	} else {
		log.Printf("No orderbook data for %s", action.Symbol)
		return false
	}
	
	// Round quantity to appropriate decimals
	quantity := math.Floor(action.Quantity*10000) / 10000
	
	// Create order
	order := &pb.CreateOrderRequest{
		Symbol:      action.Symbol,
		Exchange:    r.config.Exchange,
		Market:      "spot",
		Type:        pb.OrderType_LIMIT,
		Side:        action.Side,
		Quantity:    quantity,
		Price:       limitPrice,
		TimeInForce: pb.TimeInForce_FOK, // Fill or Kill
		ClientOrderId: fmt.Sprintf("REBAL_%s_%d", action.Symbol, time.Now().Unix()),
	}
	
	fmt.Printf("\nExecuting %s %f %s @ $%.2f... ",
		action.Side, quantity, action.Symbol, limitPrice)
	
	resp, err := r.client.CreateOrder(r.ctx, order)
	if err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		return false
	}
	
	if resp.Status == pb.OrderStatus_FILLED {
		fmt.Printf("✅ Filled @ $%.2f\n", resp.ExecutedPrice)
		return true
	} else {
		fmt.Printf("❌ Not filled (status: %s)\n", resp.Status)
		
		// Cancel if not filled
		r.client.CancelOrder(r.ctx, &pb.CancelOrderRequest{
			OrderId:  resp.OrderId,
			Exchange: r.config.Exchange,
			Symbol:   action.Symbol,
		})
		
		return false
	}
}

func (r *AutoRebalancer) saveRebalanceHistory(actions []RebalanceAction) {
	history := map[string]interface{}{
		"timestamp":      time.Now(),
		"portfolio_name": r.config.Name,
		"actions":        actions,
		"portfolio":      r.portfolio,
	}
	
	filename := fmt.Sprintf("rebalance_history_%s.json",
		time.Now().Format("20060102_150405"))
	
	data, _ := json.MarshalIndent(history, "", "  ")
	ioutil.WriteFile(filename, data, 0644)
}

func (r *AutoRebalancer) getPerformanceStats() {
	// Get historical performance
	stats, err := r.client.GetPortfolioStats(r.ctx,
		&pb.GetPortfolioStatsRequest{
			Exchange: r.config.Exchange,
			Period:   "30d",
		})
	if err != nil {
		log.Printf("Failed to get stats: %v", err)
		return
	}
	
	fmt.Println("\n=== Portfolio Performance (30 days) ===")
	fmt.Printf("Total Return: %.2f%%\n", stats.TotalReturnPercent)
	fmt.Printf("Sharpe Ratio: %.2f\n", stats.SharpeRatio)
	fmt.Printf("Max Drawdown: %.2f%%\n", stats.MaxDrawdown)
	fmt.Printf("Rebalances: %d\n", stats.NumRebalances)
}

func createSampleConfig(filename string) {
	config := PortfolioConfig{
		Name:     "Balanced Crypto Portfolio",
		Exchange: "binance",
		TargetAllocations: map[string]float64{
			"BTCUSDT":  40.0,
			"ETHUSDT":  30.0,
			"BNBUSDT":  20.0,
			"SOLUSDT":  10.0,
		},
		RebalanceThreshold: 5.0,     // 5% deviation triggers rebalance
		MinTradeValue:      10.0,    // $10 minimum trade
		CheckInterval:      24,      // Check daily (hours)
		MaxSlippage:        0.5,     // 0.5% max slippage
	}
	
	data, _ := json.MarshalIndent(config, "", "  ")
	ioutil.WriteFile(filename, data, 0644)
	fmt.Printf("Created sample config: %s\n", filename)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run auto_rebalancer.go <config.json>")
		fmt.Println("Creating sample config: portfolio_config_sample.json")
		createSampleConfig("portfolio_config_sample.json")
		return
	}
	
	// Connect to OMS
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to connect to OMS: %v", err)
	}
	defer conn.Close()
	
	// Create and run rebalancer
	rebalancer, err := NewAutoRebalancer(conn, os.Args[1])
	if err != nil {
		log.Fatalf("Failed to create rebalancer: %v", err)
	}
	
	// Show initial stats
	rebalancer.getPerformanceStats()
	
	// Run the rebalancer
	rebalancer.Run()
}

// Example output:
// === Auto Portfolio Rebalancer ===
// Portfolio: Balanced Crypto Portfolio
// Exchange: binance
// Rebalance Threshold: 5.0%
// Check Interval: 24h0m0s
//
// Target Allocations:
//   BTCUSDT: 40.0%
//   ETHUSDT: 30.0%
//   BNBUSDT: 20.0%
//   SOLUSDT: 10.0%
//
// [2025-01-27 15:30:00] Checking portfolio...
//
// Current Portfolio:
// Symbol    | Quantity  | Price     | Value     | Current% | Target% | Deviation
// ----------|-----------|-----------|-----------|----------|---------|----------
// BTCUSDT   |    0.0500 | $50000.00 | $ 2500.00 |    45.5% |   40.0% | ⚠️   5.5%
// ETHUSDT   |    0.8000 | $ 3200.00 | $ 2560.00 |    46.5% |   30.0% | ⚠️  16.5%
// BNBUSDT   |    1.0000 | $  350.00 | $  350.00 |     6.4% |   20.0% | ⚠️ -13.6%
// SOLUSDT   |    2.0000 | $   45.00 | $   90.00 |     1.6% |   10.0% | ⚠️  -8.4%
// USDT      |    0.0000 | $    1.00 | $    0.00 |     0.0% |    0.0% |    0.0%
//
// Total Portfolio Value: $5500.00
//
// 🔄 Rebalancing needed - 4 actions
//
// Rebalancing Actions:
// - SELL 0.350000 ETHUSDT (~$1120.00) - Above target by 16.5%
// - BUY  2.114286 BNBUSDT (~$740.00) - Below target by -13.6%
// - BUY  10.222222 SOLUSDT (~$460.00) - Below target by -8.4%
// - SELL 0.006000 BTCUSDT (~$300.00) - Above target by 5.5%
//
// Executing SELL 0.350000 ETHUSDT @ $3168.00... ✅ Filled @ $3168.00
// Executing SELL 0.006000 BTCUSDT @ $49500.00... ✅ Filled @ $49500.00
// Executing BUY 2.114286 BNBUSDT @ $352.50... ✅ Filled @ $352.00
// Executing BUY 10.222222 SOLUSDT @ $45.45... ✅ Filled @ $45.40
//
// ✓ Rebalancing complete: 4/4 trades successful