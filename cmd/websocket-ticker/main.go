package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/adshao/go-binance/v2"
	"github.com/adshao/go-binance/v2/futures"
)

type MarketData struct {
	Symbol        string
	SpotBid       float64
	SpotAsk       float64
	SpotLast      float64
	FuturesBid    float64
	FuturesAsk    float64
	FuturesLast   float64
	UpdateTime    time.Time
}

type MarketDataStore struct {
	mu     sync.RWMutex
	data   map[string]*MarketData
	symbols []string
}

func NewMarketDataStore(symbols []string) *MarketDataStore {
	data := make(map[string]*MarketData)
	for _, symbol := range symbols {
		data[symbol] = &MarketData{Symbol: symbol}
	}
	return &MarketDataStore{
		data:    data,
		symbols: symbols,
	}
}

func (s *MarketDataStore) UpdateSpot(symbol string, bid, ask, last float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if md, ok := s.data[symbol]; ok {
		md.SpotBid = bid
		md.SpotAsk = ask
		md.SpotLast = last
		md.UpdateTime = time.Now()
	}
}

func (s *MarketDataStore) UpdateFutures(symbol string, bid, ask, last float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if md, ok := s.data[symbol]; ok {
		md.FuturesBid = bid
		md.FuturesAsk = ask
		md.FuturesLast = last
		md.UpdateTime = time.Now()
	}
}

func (s *MarketDataStore) GetSnapshot() map[string]*MarketData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	snapshot := make(map[string]*MarketData)
	for k, v := range s.data {
		snapshot[k] = v
	}
	return snapshot
}

func main() {
	log.Println("Starting WebSocket Ticker Service...")

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

	symbols := []string{"BTCUSDT", "ETHUSDT", "XRPUSDT"}
	store := NewMarketDataStore(symbols)

	// Start display routine
	go displayMarketData(ctx, store)

	// Start WebSocket connections
	var wg sync.WaitGroup
	wg.Add(2)

	// Spot WebSocket
	go func() {
		defer wg.Done()
		connectSpotWebSocket(ctx, store, symbols)
	}()

	// Futures WebSocket
	go func() {
		defer wg.Done()
		connectFuturesWebSocket(ctx, store, symbols)
	}()

	wg.Wait()
	log.Println("WebSocket Ticker Service stopped")
}

func connectSpotWebSocket(ctx context.Context, store *MarketDataStore, symbols []string) {
	// Use book ticker for real-time bid/ask prices
	streams := make([]string, 0)
	for _, symbol := range symbols {
		streams = append(streams, strings.ToLower(symbol)+"@bookTicker")
	}

	wsHandler := func(event *binance.WsBookTickerEvent) {
		if event.Symbol == "" {
			return
		}
		
		bid, _ := strconv.ParseFloat(event.BestBidPrice, 64)
		ask, _ := strconv.ParseFloat(event.BestAskPrice, 64)
		
		// For book ticker, we don't have last price, so use mid price
		last := (bid + ask) / 2
		
		store.UpdateSpot(event.Symbol, bid, ask, last)
	}

	errHandler := func(err error) {
		log.Printf("Spot WebSocket error: %v", err)
		// Reconnect after error
		time.Sleep(5 * time.Second)
		go connectSpotWebSocket(ctx, store, symbols)
	}

	doneC, stopC, err := binance.WsCombinedBookTickerServe(streams, wsHandler, errHandler)
	if err != nil {
		log.Printf("Error starting spot WebSocket: %v", err)
		time.Sleep(5 * time.Second)
		go connectSpotWebSocket(ctx, store, symbols)
		return
	}

	log.Println("Spot WebSocket connected")

	select {
	case <-ctx.Done():
		close(stopC)
	case <-doneC:
		log.Println("Spot WebSocket disconnected")
		// Reconnect
		time.Sleep(5 * time.Second)
		go connectSpotWebSocket(ctx, store, symbols)
	}
}

func connectFuturesWebSocket(ctx context.Context, store *MarketDataStore, symbols []string) {
	// Use book ticker for real-time bid/ask prices
	streams := make([]string, 0)
	for _, symbol := range symbols {
		streams = append(streams, strings.ToLower(symbol)+"@bookTicker")
	}

	wsHandler := func(event *futures.WsBookTickerEvent) {
		if event.Symbol == "" {
			return
		}
		
		bid, _ := strconv.ParseFloat(event.BestBidPrice, 64)
		ask, _ := strconv.ParseFloat(event.BestAskPrice, 64)
		
		// For book ticker, we don't have last price, so use mid price
		last := (bid + ask) / 2
		
		store.UpdateFutures(event.Symbol, bid, ask, last)
	}

	errHandler := func(err error) {
		log.Printf("Futures WebSocket error: %v", err)
		// Reconnect after error
		time.Sleep(5 * time.Second)
		go connectFuturesWebSocket(ctx, store, symbols)
	}

	doneC, stopC, err := futures.WsCombinedBookTickerServe(streams, wsHandler, errHandler)
	if err != nil {
		log.Printf("Error starting futures WebSocket: %v", err)
		time.Sleep(5 * time.Second)
		go connectFuturesWebSocket(ctx, store, symbols)
		return
	}

	log.Println("Futures WebSocket connected")

	select {
	case <-ctx.Done():
		close(stopC)
	case <-doneC:
		log.Println("Futures WebSocket disconnected")
		// Reconnect
		time.Sleep(5 * time.Second)
		go connectFuturesWebSocket(ctx, store, symbols)
	}
}

func displayMarketData(ctx context.Context, store *MarketDataStore) {
	ticker := time.NewTicker(100 * time.Millisecond) // Fast updates for WebSocket data
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Clear screen
			fmt.Print("\033[H\033[2J")
			
			// Header
			fmt.Println("=== Multi-Exchange OMS - Real-time WebSocket Market Data ===")
			fmt.Printf("Time: %s\n\n", time.Now().Format("15:04:05.000"))

			// Table header
			fmt.Printf("%-10s %-8s %-12s %-12s %-12s %-8s %-10s\n",
				"Symbol", "Market", "Bid", "Ask", "Last", "Spread", "Premium")
			fmt.Println(strings.Repeat("-", 80))

			// Get snapshot
			data := store.GetSnapshot()

			// Display each symbol
			for _, symbol := range store.symbols {
				if md, ok := data[symbol]; ok {
					// Spot data
					if md.SpotBid > 0 && md.SpotAsk > 0 {
						spread := ((md.SpotAsk - md.SpotBid) / md.SpotBid) * 100
						fmt.Printf("%-10s %-8s $%-11.2f $%-11.2f $%-11.2f %-7.3f%% %-10s\n",
							symbol, "SPOT", md.SpotBid, md.SpotAsk, md.SpotLast, spread, "-")
					}

					// Futures data
					if md.FuturesBid > 0 && md.FuturesAsk > 0 {
						spread := ((md.FuturesAsk - md.FuturesBid) / md.FuturesBid) * 100
						premium := ""
						if md.SpotLast > 0 {
							prem := ((md.FuturesLast - md.SpotLast) / md.SpotLast) * 100
							premium = fmt.Sprintf("%.3f%%", prem)
						}
						fmt.Printf("%-10s %-8s $%-11.2f $%-11.2f $%-11.2f %-7.3f%% %-10s\n",
							symbol, "FUTURES", md.FuturesBid, md.FuturesAsk, md.FuturesLast, spread, premium)
					}

					// Arbitrage opportunity
					if md.SpotBid > 0 && md.FuturesBid > 0 {
						// Check if futures bid > spot ask (sell futures, buy spot)
						if md.FuturesBid > md.SpotAsk {
							arbProfit := ((md.FuturesBid - md.SpotAsk) / md.SpotAsk) * 100
							fmt.Printf("%-10s %-8s >>> ARBITRAGE: %.3f%% (Buy Spot @ $%.2f, Sell Futures @ $%.2f)\n",
								symbol, "ARB", arbProfit, md.SpotAsk, md.FuturesBid)
						}
						// Check if spot bid > futures ask (sell spot, buy futures)
						if md.SpotBid > md.FuturesAsk && md.FuturesAsk > 0 {
							arbProfit := ((md.SpotBid - md.FuturesAsk) / md.FuturesAsk) * 100
							fmt.Printf("%-10s %-8s >>> ARBITRAGE: %.3f%% (Buy Futures @ $%.2f, Sell Spot @ $%.2f)\n",
								symbol, "ARB", arbProfit, md.FuturesAsk, md.SpotBid)
						}
					}
					
					// Show last update time
					if !md.UpdateTime.IsZero() {
						fmt.Printf("%-10s %-8s Last update: %s\n", 
							symbol, "", time.Since(md.UpdateTime).Round(time.Millisecond))
					}
					
					fmt.Println()
				}
			}

			fmt.Println("\nPress Ctrl+C to exit")
		}
	}
}