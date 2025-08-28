package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/your-org/mExOms/pkg/types"
)

// TickerData represents real-time price data
type TickerData struct {
	Symbol    string    `json:"symbol"`
	Price     float64   `json:"price"`
	Volume    float64   `json:"volume"`
	High24h   float64   `json:"high_24h"`
	Low24h    float64   `json:"low_24h"`
	Change24h float64   `json:"change_24h"`
	Timestamp time.Time `json:"timestamp"`
}

// TickerStream handles WebSocket ticker streaming
type TickerStream struct {
	wsURL     string
	symbols   []string
	conn      *websocket.Conn
	mu        sync.RWMutex
	tickers   map[string]*TickerData
	callbacks map[string]func(*TickerData)
}

func main() {
	// Create ticker stream
	stream := NewTickerStream("ws://localhost:8080/ws/ticker", []string{
		"BTC/USDT",
		"ETH/USDT",
		"BNB/USDT",
		"SOL/USDT",
		"XRP/USDT",
	})

	// Register callbacks for specific symbols
	stream.OnTicker("BTC/USDT", func(ticker *TickerData) {
		fmt.Printf("BTC Price: $%.2f (24h: %.2f%%)\n", ticker.Price, ticker.Change24h)
	})

	stream.OnTicker("ETH/USDT", func(ticker *TickerData) {
		fmt.Printf("ETH Price: $%.2f (24h: %.2f%%)\n", ticker.Price, ticker.Change24h)
	})

	// Start streaming
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go stream.Start(ctx)

	// Example: Price monitoring with alerts
	go priceAlertMonitor(stream)

	// Example: Volume spike detection
	go volumeSpikeDetector(stream)

	// Example: Price statistics tracker
	go priceStatsTracker(stream)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down ticker stream...")
	cancel()
	time.Sleep(1 * time.Second)
}

func NewTickerStream(wsURL string, symbols []string) *TickerStream {
	return &TickerStream{
		wsURL:     wsURL,
		symbols:   symbols,
		tickers:   make(map[string]*TickerData),
		callbacks: make(map[string]func(*TickerData)),
	}
}

func (ts *TickerStream) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := ts.connect(); err != nil {
				log.Printf("Connection failed: %v, retrying in 5s...", err)
				time.Sleep(5 * time.Second)
				continue
			}

			if err := ts.subscribe(); err != nil {
				log.Printf("Subscription failed: %v", err)
				ts.conn.Close()
				continue
			}

			ts.handleMessages(ctx)
		}
	}
}

func (ts *TickerStream) connect() error {
	conn, _, err := websocket.DefaultDialer.Dial(ts.wsURL, nil)
	if err != nil {
		return err
	}
	ts.conn = conn
	fmt.Println("Connected to ticker stream")
	return nil
}

func (ts *TickerStream) subscribe() error {
	msg := map[string]interface{}{
		"action":  "subscribe",
		"channel": "ticker",
		"symbols": ts.symbols,
	}
	return ts.conn.WriteJSON(msg)
}

func (ts *TickerStream) handleMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			var msg map[string]interface{}
			if err := ts.conn.ReadJSON(&msg); err != nil {
				log.Printf("Read error: %v", err)
				return
			}

			if msg["type"] == "ticker" {
				ts.processTicker(msg)
			}
		}
	}
}

func (ts *TickerStream) processTicker(msg map[string]interface{}) {
	ticker := &TickerData{
		Symbol:    msg["symbol"].(string),
		Price:     msg["price"].(float64),
		Volume:    msg["volume"].(float64),
		High24h:   msg["high_24h"].(float64),
		Low24h:    msg["low_24h"].(float64),
		Change24h: msg["change_24h"].(float64),
		Timestamp: time.Now(),
	}

	ts.mu.Lock()
	ts.tickers[ticker.Symbol] = ticker
	ts.mu.Unlock()

	// Call registered callbacks
	if callback, ok := ts.callbacks[ticker.Symbol]; ok {
		callback(ticker)
	}
}

func (ts *TickerStream) OnTicker(symbol string, callback func(*TickerData)) {
	ts.mu.Lock()
	ts.callbacks[symbol] = callback
	ts.mu.Unlock()
}

func (ts *TickerStream) GetTicker(symbol string) *TickerData {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.tickers[symbol]
}

// Example: Price alert monitoring
func priceAlertMonitor(stream *TickerStream) {
	alerts := map[string]struct {
		high float64
		low  float64
	}{
		"BTC/USDT": {high: 70000, low: 60000},
		"ETH/USDT": {high: 3000, low: 2500},
	}

	for {
		for symbol, alert := range alerts {
			ticker := stream.GetTicker(symbol)
			if ticker == nil {
				continue
			}

			if ticker.Price >= alert.high {
				fmt.Printf("🚨 ALERT: %s reached HIGH target: $%.2f\n", symbol, ticker.Price)
			} else if ticker.Price <= alert.low {
				fmt.Printf("🚨 ALERT: %s reached LOW target: $%.2f\n", symbol, ticker.Price)
			}
		}
		time.Sleep(5 * time.Second)
	}
}

// Example: Volume spike detection
func volumeSpikeDetector(stream *TickerStream) {
	volumeHistory := make(map[string][]float64)
	
	for {
		for _, symbol := range stream.symbols {
			ticker := stream.GetTicker(symbol)
			if ticker == nil {
				continue
			}

			// Store volume history
			if _, ok := volumeHistory[symbol]; !ok {
				volumeHistory[symbol] = []float64{}
			}
			volumeHistory[symbol] = append(volumeHistory[symbol], ticker.Volume)
			
			// Keep only last 20 data points
			if len(volumeHistory[symbol]) > 20 {
				volumeHistory[symbol] = volumeHistory[symbol][1:]
			}

			// Calculate average volume
			if len(volumeHistory[symbol]) >= 10 {
				avg := average(volumeHistory[symbol][:len(volumeHistory[symbol])-1])
				current := ticker.Volume
				
				// Detect spike (2x average)
				if current > avg*2 {
					fmt.Printf("📊 VOLUME SPIKE: %s - Current: %.0f, Avg: %.0f (%.1fx)\n",
						symbol, current, avg, current/avg)
				}
			}
		}
		time.Sleep(10 * time.Second)
	}
}

// Example: Price statistics tracker
func priceStatsTracker(stream *TickerStream) {
	stats := make(map[string]*PriceStats)
	
	for {
		fmt.Println("\n=== Price Statistics (1 min) ===")
		for _, symbol := range stream.symbols {
			ticker := stream.GetTicker(symbol)
			if ticker == nil {
				continue
			}

			if _, ok := stats[symbol]; !ok {
				stats[symbol] = &PriceStats{
					Symbol: symbol,
					High:   ticker.Price,
					Low:    ticker.Price,
					Open:   ticker.Price,
				}
			}

			// Update stats
			s := stats[symbol]
			s.UpdateCount++
			if ticker.Price > s.High {
				s.High = ticker.Price
			}
			if ticker.Price < s.Low {
				s.Low = ticker.Price
			}
			s.Last = ticker.Price
			
			// Print 1-minute stats
			if s.UpdateCount%12 == 0 { // Assuming ~5s intervals
				volatility := (s.High - s.Low) / s.Open * 100
				change := (s.Last - s.Open) / s.Open * 100
				
				fmt.Printf("%s: Open: $%.2f, High: $%.2f, Low: $%.2f, Last: $%.2f, Change: %.2f%%, Volatility: %.2f%%\n",
					s.Symbol, s.Open, s.High, s.Low, s.Last, change, volatility)
				
				// Reset for next minute
				s.Open = s.Last
				s.High = s.Last
				s.Low = s.Last
				s.UpdateCount = 0
			}
		}
		time.Sleep(5 * time.Second)
	}
}

type PriceStats struct {
	Symbol      string
	Open        float64
	High        float64
	Low         float64
	Last        float64
	UpdateCount int
}

func average(values []float64) float64 {
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}