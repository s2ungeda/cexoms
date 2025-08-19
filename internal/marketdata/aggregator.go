package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"strings"
	natslib "github.com/nats-io/nats.go"
)

// PriceData represents aggregated price data
type PriceData struct {
	Exchange    string    `json:"exchange"`
	Symbol      string    `json:"symbol"`
	BidPrice    float64   `json:"bid_price"`
	BidQuantity float64   `json:"bid_quantity"`
	AskPrice    float64   `json:"ask_price"`
	AskQuantity float64   `json:"ask_quantity"`
	LastPrice   float64   `json:"last_price"`
	Volume24h   float64   `json:"volume_24h"`
	Timestamp   time.Time `json:"timestamp"`
}

// Aggregator collects market data from multiple exchanges
type Aggregator struct {
	mu sync.RWMutex
	
	// Price cache
	prices map[string]map[string]PriceData // exchange -> symbol -> price
	
	// NATS connection
	nc *natslib.Conn
	js natslib.JetStreamContext
	
	// Subscriptions
	subs []*natslib.Subscription
	
	// Context for shutdown
	ctx    context.Context
	cancel context.CancelFunc
}

// NewAggregator creates a new market data aggregator
func NewAggregator(natsURL string) (*Aggregator, error) {
	nc, err := natslib.Connect(natsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}
	
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Aggregator{
		prices: make(map[string]map[string]PriceData),
		nc:     nc,
		js:     js,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// Start begins listening for market data updates
func (a *Aggregator) Start() error {
	// Subscribe to market data from all exchanges
	exchanges := []string{"binance", "bybit", "okx"}
	
	for _, exchange := range exchanges {
		subject := fmt.Sprintf("marketdata.%s.spot.>", exchange)
		sub, err := a.nc.Subscribe(subject, a.handleMarketData)
		if err != nil {
			return fmt.Errorf("failed to subscribe to %s: %w", subject, err)
		}
		a.subs = append(a.subs, sub)
		log.Printf("Subscribed to market data from %s", exchange)
	}
	
	// Start price update publisher
	go a.publishPriceUpdates()
	
	return nil
}

// Stop gracefully shuts down the aggregator
func (a *Aggregator) Stop() error {
	a.cancel()
	
	// Unsubscribe from all subscriptions
	for _, sub := range a.subs {
		if err := sub.Unsubscribe(); err != nil {
			log.Printf("Error unsubscribing: %v", err)
		}
	}
	
	// Close NATS connection
	a.nc.Close()
	
	return nil
}

// handleMarketData processes incoming market data messages
func (a *Aggregator) handleMarketData(msg *natslib.Msg) {
	// Parse subject to extract exchange and symbol
	// Format: marketdata.{exchange}.{market}.{symbol}
	parts := strings.Split(msg.Subject, ".")
	if len(parts) < 4 {
		log.Printf("Invalid subject format: %s", msg.Subject)
		return
	}
	
	exchange := parts[1]
	symbol := parts[3]
	
	// Parse message data
	var data map[string]interface{}
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("Failed to parse market data: %v", err)
		return
	}
	
	// Extract price information
	price := PriceData{
		Exchange:  exchange,
		Symbol:    symbol,
		Timestamp: time.Now(),
	}
	
	// Try to extract standard fields
	if bid, ok := getFloat64(data, "bid_price", "bid", "best_bid"); ok {
		price.BidPrice = bid
	}
	if bidQty, ok := getFloat64(data, "bid_quantity", "bid_qty", "bid_size"); ok {
		price.BidQuantity = bidQty
	}
	if ask, ok := getFloat64(data, "ask_price", "ask", "best_ask"); ok {
		price.AskPrice = ask
	}
	if askQty, ok := getFloat64(data, "ask_quantity", "ask_qty", "ask_size"); ok {
		price.AskQuantity = askQty
	}
	if last, ok := getFloat64(data, "last_price", "last", "price"); ok {
		price.LastPrice = last
	}
	if vol, ok := getFloat64(data, "volume_24h", "volume", "vol"); ok {
		price.Volume24h = vol
	}
	
	// Update cache
	a.mu.Lock()
	if a.prices[exchange] == nil {
		a.prices[exchange] = make(map[string]PriceData)
	}
	a.prices[exchange][symbol] = price
	a.mu.Unlock()
}

// publishPriceUpdates periodically publishes aggregated price updates
func (a *Aggregator) publishPriceUpdates() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			a.publishCurrentPrices()
		case <-a.ctx.Done():
			return
		}
	}
}

// publishCurrentPrices publishes the current price snapshot
func (a *Aggregator) publishCurrentPrices() {
	a.mu.RLock()
	snapshot := make(map[string]map[string]PriceData)
	for exchange, symbols := range a.prices {
		snapshot[exchange] = make(map[string]PriceData)
		for symbol, price := range symbols {
			snapshot[exchange][symbol] = price
		}
	}
	a.mu.RUnlock()
	
	// Publish to NATS
	data, err := json.Marshal(snapshot)
	if err != nil {
		log.Printf("Failed to marshal price snapshot: %v", err)
		return
	}
	
	if err := a.nc.Publish("prices.snapshot", data); err != nil {
		log.Printf("Failed to publish price snapshot: %v", err)
	}
}

// GetPrices returns current prices for specified symbols
func (a *Aggregator) GetPrices(symbols []string) []PriceData {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	var result []PriceData
	
	// If no symbols specified, return all
	if len(symbols) == 0 {
		for _, exchangePrices := range a.prices {
			for _, price := range exchangePrices {
				result = append(result, price)
			}
		}
		return result
	}
	
	// Return only requested symbols
	symbolSet := make(map[string]bool)
	for _, s := range symbols {
		symbolSet[s] = true
	}
	
	for _, exchangePrices := range a.prices {
		for symbol, price := range exchangePrices {
			if symbolSet[symbol] {
				result = append(result, price)
			}
		}
	}
	
	return result
}

// GetPrice returns the best price for a symbol across all exchanges
func (a *Aggregator) GetPrice(symbol string) (*PriceData, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	var bestPrice *PriceData
	
	for _, exchangePrices := range a.prices {
		if price, ok := exchangePrices[symbol]; ok {
			if bestPrice == nil || price.Timestamp.After(bestPrice.Timestamp) {
				p := price
				bestPrice = &p
			}
		}
	}
	
	if bestPrice == nil {
		return nil, fmt.Errorf("no price data for symbol %s", symbol)
	}
	
	return bestPrice, nil
}

// Helper function to extract float64 from various field names
func getFloat64(data map[string]interface{}, fields ...string) (float64, bool) {
	for _, field := range fields {
		if val, ok := data[field]; ok {
			switch v := val.(type) {
			case float64:
				return v, true
			case string:
				// Try to parse string as float
				var f float64
				if _, err := fmt.Sscanf(v, "%f", &f); err == nil {
					return f, true
				}
			}
		}
	}
	return 0, false
}