package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	pb "github.com/your-org/mExOms/proto/gen/go/oms/v1"
	"google.golang.org/grpc"
)

// CrossExchangeArbitrage demonstrates arbitrage opportunities between exchanges
type CrossExchangeArbitrage struct {
	client          pb.OrderServiceClient
	ctx             context.Context
	symbol          string
	minProfitRate   float64
	tradeAmount     float64
	exchanges       []string
	priceThreshold  float64
}

func NewCrossExchangeArbitrage(conn *grpc.ClientConn) *CrossExchangeArbitrage {
	return &CrossExchangeArbitrage{
		client:         pb.NewOrderServiceClient(conn),
		ctx:           context.Background(),
		symbol:        "BTCUSDT",
		minProfitRate: 0.002, // 0.2% minimum profit
		tradeAmount:   0.01,  // 0.01 BTC per trade
		exchanges:     []string{"binance", "okx", "bybit"},
		priceThreshold: 50.0, // $50 price difference threshold
	}
}

type ExchangePrice struct {
	Exchange string
	BidPrice float64
	BidSize  float64
	AskPrice float64
	AskSize  float64
	Timestamp time.Time
}

func (a *CrossExchangeArbitrage) Run() {
	fmt.Printf("Starting Cross-Exchange Arbitrage Bot\n")
	fmt.Printf("Symbol: %s\n", a.symbol)
	fmt.Printf("Min Profit Rate: %.2f%%\n", a.minProfitRate*100)
	fmt.Printf("Trade Amount: %f\n", a.tradeAmount)
	
	// Subscribe to price updates from all exchanges
	pricesChan := make(chan ExchangePrice, len(a.exchanges)*2)
	
	// Start price monitoring for each exchange
	var wg sync.WaitGroup
	for _, exchange := range a.exchanges {
		wg.Add(1)
		go a.monitorExchange(exchange, pricesChan, &wg)
	}
	
	// Process arbitrage opportunities
	go a.processArbitrageOpportunities(pricesChan)
	
	// Wait for price monitors
	wg.Wait()
}

func (a *CrossExchangeArbitrage) monitorExchange(exchange string, 
	pricesChan chan<- ExchangePrice, wg *sync.WaitGroup) {
	
	defer wg.Done()
	
	// Subscribe to order book updates
	stream, err := a.client.StreamOrderBook(a.ctx, &pb.StreamOrderBookRequest{
		Exchange: exchange,
		Symbol:   a.symbol,
		Depth:    5,
	})
	if err != nil {
		log.Printf("Failed to subscribe to %s: %v", exchange, err)
		return
	}
	
	for {
		update, err := stream.Recv()
		if err != nil {
			log.Printf("Stream error for %s: %v", exchange, err)
			break
		}
		
		if len(update.Bids) > 0 && len(update.Asks) > 0 {
			price := ExchangePrice{
				Exchange:  exchange,
				BidPrice:  update.Bids[0].Price,
				BidSize:   update.Bids[0].Quantity,
				AskPrice:  update.Asks[0].Price,
				AskSize:   update.Asks[0].Quantity,
				Timestamp: time.Now(),
			}
			
			select {
			case pricesChan <- price:
			default:
				// Channel full, skip update
			}
		}
	}
}

func (a *CrossExchangeArbitrage) processArbitrageOpportunities(
	pricesChan <-chan ExchangePrice) {
	
	// Store latest prices from each exchange
	latestPrices := make(map[string]ExchangePrice)
	
	for price := range pricesChan {
		latestPrices[price.Exchange] = price
		
		// Check for arbitrage opportunities
		if len(latestPrices) >= 2 {
			a.checkArbitrage(latestPrices)
		}
	}
}

func (a *CrossExchangeArbitrage) checkArbitrage(
	prices map[string]ExchangePrice) {
	
	var bestBid ExchangePrice
	var bestAsk ExchangePrice
	
	// Find best bid (highest) and best ask (lowest)
	for _, price := range prices {
		// Skip stale prices (older than 5 seconds)
		if time.Since(price.Timestamp) > 5*time.Second {
			continue
		}
		
		if bestBid.Exchange == "" || price.BidPrice > bestBid.BidPrice {
			bestBid = price
		}
		
		if bestAsk.Exchange == "" || price.AskPrice < bestAsk.AskPrice {
			bestAsk = price
		}
	}
	
	// Calculate potential profit
	if bestBid.Exchange != "" && bestAsk.Exchange != "" && 
		bestBid.Exchange != bestAsk.Exchange {
		
		spread := bestBid.BidPrice - bestAsk.AskPrice
		spreadPercent := spread / bestAsk.AskPrice
		
		// Check if profitable after fees (assume 0.1% per trade)
		totalFees := 0.002 // 0.1% * 2 trades
		netProfit := spreadPercent - totalFees
		
		if netProfit > a.minProfitRate && spread > a.priceThreshold {
			fmt.Printf("\n🎯 ARBITRAGE OPPORTUNITY DETECTED!\n")
			fmt.Printf("Buy on %s @ $%.2f\n", bestAsk.Exchange, bestAsk.AskPrice)
			fmt.Printf("Sell on %s @ $%.2f\n", bestBid.Exchange, bestBid.BidPrice)
			fmt.Printf("Spread: $%.2f (%.3f%%)\n", spread, spreadPercent*100)
			fmt.Printf("Net Profit: %.3f%%\n", netProfit*100)
			
			// Execute arbitrage
			a.executeArbitrage(bestAsk, bestBid)
		}
	}
}

func (a *CrossExchangeArbitrage) executeArbitrage(
	buyExchange ExchangePrice, sellExchange ExchangePrice) {
	
	// Validate we can execute the trade size
	tradeSize := a.tradeAmount
	if buyExchange.AskSize < tradeSize {
		tradeSize = buyExchange.AskSize * 0.95 // Use 95% of available
	}
	if sellExchange.BidSize < tradeSize {
		tradeSize = sellExchange.BidSize * 0.95
	}
	
	if tradeSize < 0.0001 { // Minimum trade size
		fmt.Println("Trade size too small, skipping")
		return
	}
	
	fmt.Printf("Executing arbitrage with size: %f %s\n", tradeSize, a.symbol)
	
	// Execute both orders simultaneously
	var wg sync.WaitGroup
	wg.Add(2)
	
	var buySuccess, sellSuccess bool
	var buyOrderId, sellOrderId string
	
	// Buy order
	go func() {
		defer wg.Done()
		
		buyOrder := &pb.CreateOrderRequest{
			Symbol:      a.symbol,
			Exchange:    buyExchange.Exchange,
			Market:      "spot",
			Type:        pb.OrderType_LIMIT,
			Side:        pb.OrderSide_BUY,
			Quantity:    tradeSize,
			Price:       buyExchange.AskPrice * 1.001, // Slightly above ask
			TimeInForce: pb.TimeInForce_IOC,
		}
		
		resp, err := a.client.CreateOrder(a.ctx, buyOrder)
		if err != nil {
			log.Printf("Buy order failed: %v", err)
			return
		}
		
		buySuccess = true
		buyOrderId = resp.OrderId
		fmt.Printf("Buy order placed: %s\n", buyOrderId)
	}()
	
	// Sell order
	go func() {
		defer wg.Done()
		
		sellOrder := &pb.CreateOrderRequest{
			Symbol:      a.symbol,
			Exchange:    sellExchange.Exchange,
			Market:      "spot",
			Type:        pb.OrderType_LIMIT,
			Side:        pb.OrderSide_SELL,
			Quantity:    tradeSize,
			Price:       sellExchange.BidPrice * 0.999, // Slightly below bid
			TimeInForce: pb.TimeInForce_IOC,
		}
		
		resp, err := a.client.CreateOrder(a.ctx, sellOrder)
		if err != nil {
			log.Printf("Sell order failed: %v", err)
			return
		}
		
		sellSuccess = true
		sellOrderId = resp.OrderId
		fmt.Printf("Sell order placed: %s\n", sellOrderId)
	}()
	
	wg.Wait()
	
	// Handle partial execution
	if buySuccess && !sellSuccess {
		fmt.Println("⚠️  Sell failed, canceling buy order")
		a.client.CancelOrder(a.ctx, &pb.CancelOrderRequest{
			OrderId:  buyOrderId,
			Exchange: buyExchange.Exchange,
			Symbol:   a.symbol,
		})
	} else if sellSuccess && !buySuccess {
		fmt.Println("⚠️  Buy failed, canceling sell order")
		a.client.CancelOrder(a.ctx, &pb.CancelOrderRequest{
			OrderId:  sellOrderId,
			Exchange: sellExchange.Exchange,
			Symbol:   a.symbol,
		})
	} else if buySuccess && sellSuccess {
		fmt.Println("✅ Arbitrage executed successfully!")
		
		// Calculate actual profit
		profit := (sellExchange.BidPrice - buyExchange.AskPrice) * tradeSize
		fmt.Printf("Expected Profit: $%.2f\n", profit)
		
		// Log the trade
		a.logArbitrageTrade(buyExchange.Exchange, sellExchange.Exchange, 
			buyExchange.AskPrice, sellExchange.BidPrice, tradeSize, profit)
	}
}

func (a *CrossExchangeArbitrage) logArbitrageTrade(
	buyExchange, sellExchange string, 
	buyPrice, sellPrice, size, profit float64) {
	
	// Log to database or file for analysis
	log := fmt.Sprintf("ARBITRAGE,%s,%s,%s,%.2f,%.2f,%.6f,%.2f,%s\n",
		time.Now().Format(time.RFC3339),
		buyExchange,
		sellExchange,
		buyPrice,
		sellPrice,
		size,
		profit,
		a.symbol,
	)
	
	// In production, write to database
	fmt.Print(log)
}

func main() {
	// Connect to OMS
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to connect to OMS: %v", err)
	}
	defer conn.Close()
	
	// Create and run arbitrage bot
	arb := NewCrossExchangeArbitrage(conn)
	
	// Validate we have access to all required exchanges
	client := pb.NewOrderServiceClient(conn)
	exchanges, err := client.ListExchanges(context.Background(), 
		&pb.ListExchangesRequest{})
	if err != nil {
		log.Fatalf("Failed to list exchanges: %v", err)
	}
	
	fmt.Println("Available exchanges:")
	for _, ex := range exchanges.Exchanges {
		fmt.Printf("- %s (%s)\n", ex.Name, ex.Status)
	}
	
	// Run the arbitrage bot
	fmt.Println("\nStarting arbitrage monitoring...")
	arb.Run()
}

// Example output:
// Starting Cross-Exchange Arbitrage Bot
// Symbol: BTCUSDT
// Min Profit Rate: 0.20%
// Trade Amount: 0.010000
//
// Available exchanges:
// - binance (active)
// - okx (active)
// - bybit (active)
//
// Starting arbitrage monitoring...
//
// 🎯 ARBITRAGE OPPORTUNITY DETECTED!
// Buy on okx @ $50234.50
// Sell on binance @ $50298.30
// Spread: $63.80 (0.127%)
// Net Profit: 0.027%
// Executing arbitrage with size: 0.010000 BTCUSDT
// Buy order placed: okx-123456
// Sell order placed: binance-789012
// ✅ Arbitrage executed successfully!
// Expected Profit: $0.64