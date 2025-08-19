package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	binance "github.com/adshao/go-binance/v2"
	"github.com/mExOms/internal/marketdata"
	natslib "github.com/nats-io/nats.go"
)

type MarketDataService struct {
	nc         *natslib.Conn
	aggregator *marketdata.Aggregator
	binance    *binance.Client
	symbols    []string
	doneC      chan struct{}
	wsHandlers map[string]chan struct{}
}

func main() {
	// Configuration
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}
	
	symbols := strings.Split(os.Getenv("SYMBOLS"), ",")
	if len(symbols) == 0 || symbols[0] == "" {
		symbols = []string{"BTCUSDT", "ETHUSDT", "BNBUSDT", "SOLUSDT", "XRPUSDT"}
	}
	
	// Create service
	service, err := NewMarketDataService(natsURL, symbols)
	if err != nil {
		log.Fatalf("Failed to create market data service: %v", err)
	}
	
	// Start service
	if err := service.Start(); err != nil {
		log.Fatalf("Failed to start service: %v", err)
	}
	
	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	
	log.Println("Shutting down market data service...")
	if err := service.Stop(); err != nil {
		log.Printf("Error stopping service: %v", err)
	}
}

func NewMarketDataService(natsURL string, symbols []string) (*MarketDataService, error) {
	// Connect to NATS
	nc, err := natslib.Connect(natsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}
	
	// Create aggregator
	aggregator, err := marketdata.NewAggregator(natsURL)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to create aggregator: %w", err)
	}
	
	// Create Binance client
	binanceClient := binance.NewClient("", "")
	
	return &MarketDataService{
		nc:         nc,
		aggregator: aggregator,
		binance:    binanceClient,
		symbols:    symbols,
		doneC:      make(chan struct{}),
		wsHandlers: make(map[string]chan struct{}),
	}, nil
}

func (s *MarketDataService) Start() error {
	log.Printf("Starting market data service for symbols: %v", s.symbols)
	
	// Start aggregator
	if err := s.aggregator.Start(); err != nil {
		return fmt.Errorf("failed to start aggregator: %w", err)
	}
	
	// Start WebSocket streams for each symbol
	for _, symbol := range s.symbols {
		if err := s.startSymbolStream(symbol); err != nil {
			log.Printf("Failed to start stream for %s: %v", symbol, err)
			continue
		}
		
		// Also start a ticker stream for more complete data
		if err := s.startTickerStream(symbol); err != nil {
			log.Printf("Failed to start ticker stream for %s: %v", symbol, err)
		}
		
		// Small delay to avoid rate limits
		time.Sleep(100 * time.Millisecond)
	}
	
	// Start REST API price poller as backup
	go s.pollPrices()
	
	return nil
}

func (s *MarketDataService) Stop() error {
	close(s.doneC)
	
	// Stop all WebSocket handlers
	for _, stopC := range s.wsHandlers {
		close(stopC)
	}
	
	// Wait a bit for handlers to stop
	time.Sleep(500 * time.Millisecond)
	
	// Stop aggregator
	if err := s.aggregator.Stop(); err != nil {
		log.Printf("Error stopping aggregator: %v", err)
	}
	
	// Close NATS connection
	s.nc.Close()
	
	return nil
}

func (s *MarketDataService) startSymbolStream(symbol string) error {
	wsDepthHandler := func(event *binance.WsDepthEvent) {
		// Convert to our format and publish
		data := map[string]interface{}{
			"symbol":       event.Symbol,
			"bid_price":    event.Bids[0].Price,
			"bid_quantity": event.Bids[0].Quantity,
			"ask_price":    event.Asks[0].Price,
			"ask_quantity": event.Asks[0].Quantity,
			"update_id":    event.LastUpdateID,
			"timestamp":    time.Now(),
		}
		
		s.publishMarketData("binance", "spot", symbol, data)
	}
	
	errHandler := func(err error) {
		log.Printf("WebSocket error for %s: %v", symbol, err)
	}
	
	doneC, stopC, err := binance.WsDepthServe(symbol, wsDepthHandler, errHandler)
	if err != nil {
		return fmt.Errorf("failed to start depth stream: %w", err)
	}
	
	s.wsHandlers[fmt.Sprintf("depth_%s", symbol)] = stopC
	
	// Monitor the done channel
	go func() {
		select {
		case <-doneC:
			log.Printf("Depth stream for %s closed", symbol)
		case <-s.doneC:
			return
		}
	}()
	
	log.Printf("Started depth stream for %s", symbol)
	return nil
}

func (s *MarketDataService) startTickerStream(symbol string) error {
	wsTickerHandler := func(event *binance.WsBookTickerEvent) {
		// Convert to our format and publish
		data := map[string]interface{}{
			"symbol":       event.Symbol,
			"bid_price":    event.BestBidPrice,
			"bid_quantity": event.BestBidQty,
			"ask_price":    event.BestAskPrice,
			"ask_quantity": event.BestAskQty,
			"timestamp":    time.Now(),
		}
		
		s.publishMarketData("binance", "spot", symbol, data)
	}
	
	errHandler := func(err error) {
		log.Printf("Ticker WebSocket error for %s: %v", symbol, err)
	}
	
	doneC, stopC, err := binance.WsBookTickerServe(symbol, wsTickerHandler, errHandler)
	if err != nil {
		return fmt.Errorf("failed to start ticker stream: %w", err)
	}
	
	s.wsHandlers[fmt.Sprintf("ticker_%s", symbol)] = stopC
	
	// Monitor the done channel
	go func() {
		select {
		case <-doneC:
			log.Printf("Ticker stream for %s closed", symbol)
		case <-s.doneC:
			return
		}
	}()
	
	log.Printf("Started ticker stream for %s", symbol)
	return nil
}

func (s *MarketDataService) pollPrices() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			s.fetchPrices()
		case <-s.doneC:
			return
		}
	}
}

func (s *MarketDataService) fetchPrices() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	
	// Fetch ticker prices for all symbols
	prices, err := s.binance.NewListPricesService().Do(ctx)
	if err != nil {
		log.Printf("Failed to fetch prices: %v", err)
		return
	}
	
	// Create a map for quick lookup
	priceMap := make(map[string]string)
	for _, p := range prices {
		priceMap[p.Symbol] = p.Price
	}
	
	// Fetch 24hr ticker for volume data
	tickers, err := s.binance.NewListPriceChangeStatsService().Do(ctx)
	if err != nil {
		log.Printf("Failed to fetch tickers: %v", err)
		return
	}
	
	// Process our symbols
	for _, ticker := range tickers {
		// Check if this is one of our symbols
		found := false
		for _, symbol := range s.symbols {
			if ticker.Symbol == symbol {
				found = true
				break
			}
		}
		
		if !found {
			continue
		}
		
		// Publish the data
		data := map[string]interface{}{
			"symbol":      ticker.Symbol,
			"last_price":  priceMap[ticker.Symbol],
			"bid_price":   ticker.BidPrice,
			"ask_price":   ticker.AskPrice,
			"volume_24h":  ticker.Volume,
			"high_24h":    ticker.HighPrice,
			"low_24h":     ticker.LowPrice,
			"change_24h":  ticker.PriceChangePercent,
			"timestamp":   time.Now(),
		}
		
		s.publishMarketData("binance", "spot", ticker.Symbol, data)
	}
}

func (s *MarketDataService) publishMarketData(exchange, market, symbol string, data map[string]interface{}) {
	subject := fmt.Sprintf("marketdata.%s.%s.%s", exchange, market, symbol)
	
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("Failed to marshal market data: %v", err)
		return
	}
	
	if err := s.nc.Publish(subject, jsonData); err != nil {
		log.Printf("Failed to publish market data: %v", err)
	}
}