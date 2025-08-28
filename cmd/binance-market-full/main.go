package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// Trade represents a recent trade
type Trade struct {
	ID        string    `json:"id"`
	Symbol    string    `json:"symbol"`
	Price     float64   `json:"price"`
	Quantity  float64   `json:"quantity"`
	Side      string    `json:"side"`
	Timestamp time.Time `json:"timestamp"`
}

// OrderBookLevel represents a price level
type OrderBookLevel struct {
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
}

// MarketDataUpdate combines orderbook and trades
type MarketDataUpdate struct {
	Symbol    string           `json:"symbol"`
	OrderBook *OrderBook       `json:"orderBook,omitempty"`
	Trade     *Trade           `json:"trade,omitempty"`
	Timestamp time.Time        `json:"timestamp"`
}

type OrderBook struct {
	Bids []OrderBookLevel `json:"bids"`
	Asks []OrderBookLevel `json:"asks"`
}

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Connect to NATS
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	logger.Info("Connected to NATS, starting Binance market data streams...")

	// Symbols to track
	symbols := []string{"btcusdt", "ethusdt", "bnbusdt", "solusdt", "xrpusdt"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start combined stream for each symbol
	for _, symbol := range symbols {
		go startCombinedStream(ctx, symbol, nc, logger)
	}

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	<-sigChan
	logger.Info("Shutting down...")
}

func startCombinedStream(ctx context.Context, symbol string, nc *nats.Conn, logger *zap.Logger) {
	// Combined stream: depth + trade
	wsURL := fmt.Sprintf("wss://stream.binance.com:9443/ws/%s@depth20@100ms/%s@trade", symbol, symbol)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			logger.Info("Connecting to Binance combined stream", zap.String("symbol", symbol))
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				logger.Error("WebSocket connection failed", zap.Error(err))
				time.Sleep(5 * time.Second)
				continue
			}

			handleCombinedStream(ctx, conn, symbol, nc, logger)
			conn.Close()
			time.Sleep(1 * time.Second)
		}
	}
}

func handleCombinedStream(ctx context.Context, conn *websocket.Conn, symbol string, nc *nats.Conn, logger *zap.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			var msg map[string]interface{}
			err := conn.ReadJSON(&msg)
			if err != nil {
				logger.Error("Failed to read message", zap.Error(err))
				return
			}

			// Check event type
			if eventType, ok := msg["e"].(string); ok {
				switch eventType {
				case "depthUpdate":
					orderBook := parseDepthUpdate(msg)
					publishMarketData(strings.ToUpper(symbol), &orderBook, nil, nc, logger)
				case "trade":
					trade := parseTrade(msg)
					publishMarketData(strings.ToUpper(symbol), nil, &trade, nc, logger)
				}
			} else if _, ok := msg["lastUpdateId"]; ok {
				// Snapshot data
				orderBook := parseDepthSnapshot(msg)
				publishMarketData(strings.ToUpper(symbol), &orderBook, nil, nc, logger)
			}
		}
	}
}

func parseDepthUpdate(msg map[string]interface{}) OrderBook {
	ob := OrderBook{
		Bids: make([]OrderBookLevel, 0),
		Asks: make([]OrderBookLevel, 0),
	}

	// Parse bids
	if bidsData, ok := msg["b"].([]interface{}); ok {
		for i, bid := range bidsData {
			if i >= 10 { break }
			if bidArray, ok := bid.([]interface{}); ok && len(bidArray) >= 2 {
				price, _ := strconv.ParseFloat(bidArray[0].(string), 64)
				qty, _ := strconv.ParseFloat(bidArray[1].(string), 64)
				if qty > 0 { // Only add non-zero quantities
					ob.Bids = append(ob.Bids, OrderBookLevel{Price: price, Quantity: qty})
				}
			}
		}
	}

	// Parse asks
	if asksData, ok := msg["a"].([]interface{}); ok {
		for i, ask := range asksData {
			if i >= 10 { break }
			if askArray, ok := ask.([]interface{}); ok && len(askArray) >= 2 {
				price, _ := strconv.ParseFloat(askArray[0].(string), 64)
				qty, _ := strconv.ParseFloat(askArray[1].(string), 64)
				if qty > 0 { // Only add non-zero quantities
					ob.Asks = append(ob.Asks, OrderBookLevel{Price: price, Quantity: qty})
				}
			}
		}
	}

	return ob
}

func parseDepthSnapshot(msg map[string]interface{}) OrderBook {
	return parseDepthUpdate(msg) // Same format
}

func parseTrade(msg map[string]interface{}) Trade {
	tradeID := fmt.Sprintf("%v", msg["t"])
	symbol := strings.ToUpper(msg["s"].(string))
	price, _ := strconv.ParseFloat(msg["p"].(string), 64)
	qty, _ := strconv.ParseFloat(msg["q"].(string), 64)
	
	side := "BUY"
	if msg["m"].(bool) { // true = seller is maker = buyer initiated
		side = "SELL"
	}
	
	tradeTime := int64(msg["T"].(float64))
	
	return Trade{
		ID:        tradeID,
		Symbol:    symbol,
		Price:     price,
		Quantity:  qty,
		Side:      side,
		Timestamp: time.Unix(0, tradeTime*int64(time.Millisecond)),
	}
}

func publishMarketData(symbol string, orderBook *OrderBook, trade *Trade, nc *nats.Conn, logger *zap.Logger) {
	update := MarketDataUpdate{
		Symbol:    symbol,
		OrderBook: orderBook,
		Trade:     trade,
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(update)
	if err != nil {
		logger.Error("Failed to marshal market data", zap.Error(err))
		return
	}

	// Publish to market data subject
	subject := fmt.Sprintf("market.data.binance.%s", symbol)
	if err := nc.Publish(subject, data); err != nil {
		logger.Error("Failed to publish market data", 
			zap.String("subject", subject),
			zap.Error(err))
	} else {
		if orderBook != nil {
			logger.Debug("Published orderbook", 
				zap.String("symbol", symbol),
				zap.Int("bids", len(orderBook.Bids)),
				zap.Int("asks", len(orderBook.Asks)))
		}
		if trade != nil {
			logger.Debug("Published trade", 
				zap.String("symbol", symbol),
				zap.Float64("price", trade.Price),
				zap.String("side", trade.Side))
		}
	}
}