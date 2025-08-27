package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	pb "github.com/your-org/mExOms/proto/gen/go/oms/v1"
	"google.golang.org/grpc"
)

// DCAConfig holds configuration for Dollar Cost Averaging strategy
type DCAConfig struct {
	Symbol          string        `json:"symbol"`
	Exchange        string        `json:"exchange"`
	InvestmentAmount float64      `json:"investment_amount"`  // Amount in quote currency (USDT)
	Interval        time.Duration `json:"interval_minutes"`   // How often to buy
	TotalDuration   time.Duration `json:"total_days"`        // Total duration of DCA
	StartTime       time.Time     `json:"start_time"`
	MinOrderAmount  float64       `json:"min_order_amount"`  // Exchange minimum
}

// DCABot implements a Dollar Cost Averaging strategy
type DCABot struct {
	client      pb.OrderServiceClient
	ctx         context.Context
	config      DCAConfig
	totalSpent  float64
	totalBought float64
	numBuys     int
	history     []DCAExecution
}

// DCAExecution records each DCA buy
type DCAExecution struct {
	Timestamp    time.Time
	Price        float64
	Quantity     float64
	SpentAmount  float64
	OrderId      string
	CumulativeQty float64
	AveragePrice float64
}

func NewDCABot(conn *grpc.ClientConn, configFile string) (*DCABot, error) {
	// Load configuration
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %v", err)
	}
	
	var config DCAConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}
	
	// Set defaults if not specified
	if config.MinOrderAmount == 0 {
		config.MinOrderAmount = 10.0 // $10 minimum
	}
	
	return &DCABot{
		client:  pb.NewOrderServiceClient(conn),
		ctx:    context.Background(),
		config: config,
		history: make([]DCAExecution, 0),
	}, nil
}

func (bot *DCABot) Run() {
	fmt.Println("=== Dollar Cost Averaging Bot ===")
	fmt.Printf("Symbol: %s\n", bot.config.Symbol)
	fmt.Printf("Exchange: %s\n", bot.config.Exchange)
	fmt.Printf("Investment per interval: $%.2f\n", bot.config.InvestmentAmount)
	fmt.Printf("Interval: %s\n", bot.config.Interval)
	fmt.Printf("Total duration: %s\n", bot.config.TotalDuration)
	fmt.Printf("Start time: %s\n", bot.config.StartTime.Format(time.RFC3339))
	
	// Calculate total number of buys
	totalBuys := int(bot.config.TotalDuration / bot.config.Interval)
	fmt.Printf("Total planned buys: %d\n", totalBuys)
	fmt.Printf("Total investment: $%.2f\n", bot.config.InvestmentAmount * float64(totalBuys))
	fmt.Println()
	
	// Wait for start time if in future
	if time.Until(bot.config.StartTime) > 0 {
		fmt.Printf("Waiting until %s to start...\n", bot.config.StartTime.Format(time.Kitchen))
		time.Sleep(time.Until(bot.config.StartTime))
	}
	
	// Create ticker for periodic buys
	ticker := time.NewTicker(bot.config.Interval)
	defer ticker.Stop()
	
	// Execute first buy immediately
	bot.executeBuy()
	
	// Set up end time
	endTime := bot.config.StartTime.Add(bot.config.TotalDuration)
	
	for {
		select {
		case <-ticker.C:
			if time.Now().After(endTime) {
				fmt.Println("\nDCA period completed!")
				bot.printSummary()
				return
			}
			bot.executeBuy()
			
		case <-bot.ctx.Done():
			fmt.Println("\nBot stopped")
			bot.printSummary()
			return
		}
	}
}

func (bot *DCABot) executeBuy() {
	fmt.Printf("\n[%s] Executing DCA buy #%d\n", 
		time.Now().Format("2006-01-02 15:04:05"), bot.numBuys+1)
	
	// Get current market price
	ticker, err := bot.client.GetTicker(bot.ctx, &pb.GetTickerRequest{
		Exchange: bot.config.Exchange,
		Symbol:   bot.config.Symbol,
	})
	if err != nil {
		log.Printf("Failed to get ticker: %v", err)
		return
	}
	
	currentPrice := ticker.LastPrice
	fmt.Printf("Current price: $%.2f\n", currentPrice)
	
	// Calculate quantity to buy
	quantity := bot.config.InvestmentAmount / currentPrice
	
	// Check minimum order amount
	if bot.config.InvestmentAmount < bot.config.MinOrderAmount {
		log.Printf("Investment amount $%.2f below minimum $%.2f", 
			bot.config.InvestmentAmount, bot.config.MinOrderAmount)
		return
	}
	
	// Create market order
	order := &pb.CreateOrderRequest{
		Symbol:      bot.config.Symbol,
		Exchange:    bot.config.Exchange,
		Market:      "spot",
		Type:        pb.OrderType_MARKET,
		Side:        pb.OrderSide_BUY,
		QuoteQuantity: bot.config.InvestmentAmount, // Buy exactly this much USDT worth
		TimeInForce: pb.TimeInForce_IOC,
		ClientOrderId: fmt.Sprintf("DCA_%s_%d", bot.config.Symbol, time.Now().Unix()),
	}
	
	// Execute order
	resp, err := bot.client.CreateOrder(bot.ctx, order)
	if err != nil {
		log.Printf("Failed to create order: %v", err)
		return
	}
	
	fmt.Printf("Order executed! ID: %s\n", resp.OrderId)
	fmt.Printf("Bought: %f %s\n", resp.ExecutedQuantity, bot.config.Symbol)
	fmt.Printf("Spent: $%.2f\n", resp.ExecutedQuantity * resp.ExecutedPrice)
	
	// Update statistics
	bot.totalSpent += resp.ExecutedQuantity * resp.ExecutedPrice
	bot.totalBought += resp.ExecutedQuantity
	bot.numBuys++
	
	averagePrice := bot.totalSpent / bot.totalBought
	
	// Record execution
	execution := DCAExecution{
		Timestamp:     time.Now(),
		Price:         resp.ExecutedPrice,
		Quantity:      resp.ExecutedQuantity,
		SpentAmount:   resp.ExecutedQuantity * resp.ExecutedPrice,
		OrderId:       resp.OrderId,
		CumulativeQty: bot.totalBought,
		AveragePrice:  averagePrice,
	}
	bot.history = append(bot.history, execution)
	
	// Show running statistics
	fmt.Printf("\n--- Running Statistics ---\n")
	fmt.Printf("Total invested: $%.2f\n", bot.totalSpent)
	fmt.Printf("Total quantity: %f %s\n", bot.totalBought, bot.config.Symbol)
	fmt.Printf("Average price: $%.2f\n", averagePrice)
	fmt.Printf("Current value: $%.2f\n", bot.totalBought * currentPrice)
	
	// Calculate performance
	currentValue := bot.totalBought * currentPrice
	profit := currentValue - bot.totalSpent
	profitPercent := (profit / bot.totalSpent) * 100
	
	if profit >= 0 {
		fmt.Printf("Profit: $%.2f (%.2f%%)\n", profit, profitPercent)
	} else {
		fmt.Printf("Loss: $%.2f (%.2f%%)\n", -profit, -profitPercent)
	}
	
	// Save history to file
	bot.saveHistory()
}

func (bot *DCABot) saveHistory() {
	filename := fmt.Sprintf("dca_history_%s_%s.json", 
		bot.config.Symbol, bot.config.StartTime.Format("20060102"))
	
	data, err := json.MarshalIndent(bot.history, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal history: %v", err)
		return
	}
	
	if err := ioutil.WriteFile(filename, data, 0644); err != nil {
		log.Printf("Failed to save history: %v", err)
	}
}

func (bot *DCABot) printSummary() {
	fmt.Println("\n=== DCA Bot Summary ===")
	fmt.Printf("Strategy: %s on %s\n", bot.config.Symbol, bot.config.Exchange)
	fmt.Printf("Duration: %s to %s\n", 
		bot.config.StartTime.Format("2006-01-02"),
		time.Now().Format("2006-01-02"))
	fmt.Printf("Number of buys: %d\n", bot.numBuys)
	fmt.Printf("Total invested: $%.2f\n", bot.totalSpent)
	fmt.Printf("Total quantity: %f %s\n", bot.totalBought, bot.config.Symbol)
	
	if bot.numBuys > 0 {
		avgPrice := bot.totalSpent / bot.totalBought
		fmt.Printf("Average buy price: $%.2f\n", avgPrice)
		
		// Get current price for final valuation
		ticker, err := bot.client.GetTicker(bot.ctx, &pb.GetTickerRequest{
			Exchange: bot.config.Exchange,
			Symbol:   bot.config.Symbol,
		})
		
		if err == nil {
			currentValue := bot.totalBought * ticker.LastPrice
			profit := currentValue - bot.totalSpent
			profitPercent := (profit / bot.totalSpent) * 100
			
			fmt.Printf("\nCurrent price: $%.2f\n", ticker.LastPrice)
			fmt.Printf("Current value: $%.2f\n", currentValue)
			
			if profit >= 0 {
				fmt.Printf("Total profit: $%.2f (%.2f%%)\n", profit, profitPercent)
			} else {
				fmt.Printf("Total loss: $%.2f (%.2f%%)\n", -profit, -profitPercent)
			}
		}
	}
	
	// Price analysis
	if len(bot.history) > 0 {
		var minPrice, maxPrice float64 = bot.history[0].Price, bot.history[0].Price
		for _, exec := range bot.history {
			if exec.Price < minPrice {
				minPrice = exec.Price
			}
			if exec.Price > maxPrice {
				maxPrice = exec.Price
			}
		}
		
		fmt.Printf("\nPrice range during DCA:\n")
		fmt.Printf("Lowest buy: $%.2f\n", minPrice)
		fmt.Printf("Highest buy: $%.2f\n", maxPrice)
	}
}

func createSampleConfig(filename string) {
	config := DCAConfig{
		Symbol:           "BTCUSDT",
		Exchange:        "binance",
		InvestmentAmount: 100.0,                         // $100 per buy
		Interval:        24 * 60,                        // Daily (in minutes)
		TotalDuration:   30 * 24 * 60,                   // 30 days (in minutes)
		StartTime:       time.Now().Add(1 * time.Minute), // Start in 1 minute
		MinOrderAmount:  10.0,
	}
	
	data, _ := json.MarshalIndent(config, "", "  ")
	ioutil.WriteFile(filename, data, 0644)
	fmt.Printf("Created sample config: %s\n", filename)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run dca_bot.go <config.json>")
		fmt.Println("Creating sample config: dca_config_sample.json")
		createSampleConfig("dca_config_sample.json")
		return
	}
	
	// Connect to OMS
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to connect to OMS: %v", err)
	}
	defer conn.Close()
	
	// Create and run DCA bot
	bot, err := NewDCABot(conn, os.Args[1])
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}
	
	bot.Run()
}

// Example output:
// === Dollar Cost Averaging Bot ===
// Symbol: BTCUSDT
// Exchange: binance
// Investment per interval: $100.00
// Interval: 24h0m0s
// Total duration: 720h0m0s
// Start time: 2025-01-27T14:30:00Z
// Total planned buys: 30
// Total investment: $3000.00
//
// [2025-01-27 14:30:00] Executing DCA buy #1
// Current price: $50245.50
// Order executed! ID: binance-dca-001
// Bought: 0.001991 BTCUSDT
// Spent: $100.00
//
// --- Running Statistics ---
// Total invested: $100.00
// Total quantity: 0.001991 BTCUSDT
// Average price: $50245.50
// Current value: $100.00
// Profit: $0.00 (0.00%)
//
// [2025-01-28 14:30:00] Executing DCA buy #2
// Current price: $49850.00
// Order executed! ID: binance-dca-002
// Bought: 0.002006 BTCUSDT
// Spent: $100.00
//
// --- Running Statistics ---
// Total invested: $200.00
// Total quantity: 0.003997 BTCUSDT
// Average price: $50047.02
// Current value: $199.25
// Loss: $0.75 (0.38%)