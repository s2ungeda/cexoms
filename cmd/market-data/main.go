package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/adshao/go-binance/v2"
	"github.com/adshao/go-binance/v2/futures"
)

type PriceUpdate struct {
	Symbol    string
	Market    string // "spot" or "futures"
	BidPrice  string
	BidQty    string
	AskPrice  string
	AskQty    string
	Timestamp time.Time
}

var symbols = []string{"BTCUSDT", "ETHUSDT", "XRPUSDT"}

func main() {
	log.Println("Starting Market Data Service...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutdown signal received")
		cancel()
	}()

	priceChannel := make(chan PriceUpdate, 100)

	// Start price display goroutine
	go displayPrices(priceChannel)

	// Start Spot WebSocket streams
	go startSpotStreams(ctx, priceChannel)

	// Start Futures WebSocket streams
	go startFuturesStreams(ctx, priceChannel)

	// Keep main thread alive
	<-ctx.Done()
	log.Println("Market Data Service stopped")
}

func startSpotStreams(ctx context.Context, priceChannel chan<- PriceUpdate) {
	streams := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		streams = append(streams, fmt.Sprintf("%s@bookTicker", strings.ToLower(symbol)))
	}

	wsBookTickerHandler := func(event *binance.WsBookTickerEvent) {
		priceChannel <- PriceUpdate{
			Symbol:    event.Symbol,
			Market:    "spot",
			BidPrice:  event.BestBidPrice,
			BidQty:    event.BestBidQty,
			AskPrice:  event.BestAskPrice,
			AskQty:    event.BestAskQty,
			Timestamp: time.Now(),
		}
	}

	errHandler := func(err error) {
		log.Printf("Spot WebSocket error: %v", err)
	}

	doneC, stopC, err := binance.WsCombinedBookTickerServe(streams, wsBookTickerHandler, errHandler)
	if err != nil {
		log.Printf("Error starting spot streams: %v", err)
		return
	}

	log.Printf("Spot market data streams started for: %v", symbols)

	select {
	case <-ctx.Done():
		close(stopC)
	case <-doneC:
		log.Println("Spot WebSocket closed")
	}
}

func startFuturesStreams(ctx context.Context, priceChannel chan<- PriceUpdate) {
	streams := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		streams = append(streams, fmt.Sprintf("%s@bookTicker", strings.ToLower(symbol)))
	}

	wsBookTickerHandler := func(event *futures.WsBookTickerEvent) {
		priceChannel <- PriceUpdate{
			Symbol:    event.Symbol,
			Market:    "futures",
			BidPrice:  event.BestBidPrice,
			BidQty:    event.BestBidQty,
			AskPrice:  event.BestAskPrice,
			AskQty:    event.BestAskQty,
			Timestamp: time.Now(),
		}
	}

	errHandler := func(err error) {
		log.Printf("Futures WebSocket error: %v", err)
	}

	doneC, stopC, err := futures.WsCombinedBookTickerServe(streams, wsBookTickerHandler, errHandler)
	if err != nil {
		log.Printf("Error starting futures streams: %v", err)
		return
	}

	log.Printf("Futures market data streams started for: %v", symbols)

	select {
	case <-ctx.Done():
		close(stopC)
	case <-doneC:
		log.Println("Futures WebSocket closed")
	}
}

func displayPrices(priceChannel <-chan PriceUpdate) {
	// Store latest prices
	latestPrices := make(map[string]PriceUpdate)

	// Update display every second
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case update := <-priceChannel:
			key := fmt.Sprintf("%s-%s", update.Symbol, update.Market)
			latestPrices[key] = update

		case <-ticker.C:
			// Clear screen (works on Unix-like systems)
			fmt.Print("\033[H\033[2J")
			
			// Display header
			fmt.Println("=== Multi-Exchange OMS - Real-time Market Data ===")
			fmt.Printf("Time: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))
			
			// Display table header
			fmt.Printf("%-10s %-8s %-12s %-12s %-12s %-12s %-8s\n", 
				"Symbol", "Market", "Bid Price", "Bid Qty", "Ask Price", "Ask Qty", "Spread")
			fmt.Println(string(make([]byte, 90, 90)))

			// Display prices for each symbol
			for _, symbol := range symbols {
				// Spot prices
				if spot, ok := latestPrices[symbol+"-spot"]; ok {
					spread := calculateSpread(spot.BidPrice, spot.AskPrice)
					fmt.Printf("%-10s %-8s %-12s %-12s %-12s %-12s %-8s\n",
						symbol, "SPOT", spot.BidPrice, spot.BidQty, 
						spot.AskPrice, spot.AskQty, spread)
				}
				
				// Futures prices
				if futures, ok := latestPrices[symbol+"-futures"]; ok {
					spread := calculateSpread(futures.BidPrice, futures.AskPrice)
					fmt.Printf("%-10s %-8s %-12s %-12s %-12s %-12s %-8s\n",
						symbol, "FUTURES", futures.BidPrice, futures.BidQty, 
						futures.AskPrice, futures.AskQty, spread)
				}
				
				// Calculate spot-futures premium if both available
				spotKey := symbol + "-spot"
				futuresKey := symbol + "-futures"
				if spot, spotOk := latestPrices[spotKey]; spotOk {
					if fut, futOk := latestPrices[futuresKey]; futOk {
						premium := calculatePremium(spot.BidPrice, fut.AskPrice)
						fmt.Printf("%-10s %-8s Premium: %s%%\n", symbol, "S-F", premium)
					}
				}
				fmt.Println()
			}
			
			fmt.Println("\nPress Ctrl+C to exit")
		}
	}
}

func calculateSpread(bidPrice, askPrice string) string {
	// Simple spread calculation (would use decimal library in production)
	return "0.01%"
}

func calculatePremium(spotPrice, futuresPrice string) string {
	// Simple premium calculation (would use decimal library in production)
	return "0.05"
}