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

type OrderBookUpdate struct {
	Symbol    string       `json:"symbol"`
	Bids      []PriceLevel `json:"bids"`
	Asks      []PriceLevel `json:"asks"`
	Timestamp time.Time    `json:"timestamp"`
}

type PriceLevel struct {
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
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

	logger.Info("Connected to NATS, starting Binance OrderBook streams...")

	// Symbols to track
	symbols := []string{"btcusdt", "ethusdt", "bnbusdt", "solusdt", "xrpusdt"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start orderbook stream
	go startOrderBookStream(ctx, symbols, nc, logger)

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	<-sigChan
	logger.Info("Shutting down...")
}

func startOrderBookStream(ctx context.Context, symbols []string, nc *nats.Conn, logger *zap.Logger) {
	// Create multi-stream URL for order book depth
	streams := make([]string, len(symbols))
	for i, symbol := range symbols {
		streams[i] = fmt.Sprintf("%s@depth20@100ms", symbol) // Top 20 levels, 100ms updates
	}
	wsURL := fmt.Sprintf("wss://stream.binance.com:9443/ws/%s", strings.Join(streams, "/"))

	for {
		select {
		case <-ctx.Done():
			return
		default:
			logger.Info("Connecting to Binance orderbook stream")
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				logger.Error("WebSocket connection failed", zap.Error(err))
				time.Sleep(5 * time.Second)
				continue
			}

			handleOrderBookStream(ctx, conn, nc, logger)
			conn.Close()
			time.Sleep(1 * time.Second)
		}
	}
}

func handleOrderBookStream(ctx context.Context, conn *websocket.Conn, nc *nats.Conn, logger *zap.Logger) {
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

			// Parse depth update
			if symbol, ok := msg["s"].(string); ok {
				orderBook := parseOrderBookMessage(msg)
				publishOrderBook(orderBook, nc, logger)
				
				// Also publish as market data update with orderBook field
				marketUpdate := map[string]interface{}{
					"symbol":    strings.ToUpper(symbol),
					"orderBook": orderBook,
					"timestamp": time.Now(),
				}
				
				data, _ := json.Marshal(marketUpdate)
				subject := fmt.Sprintf("market.data.binance.%s", strings.ToUpper(symbol))
				nc.Publish(subject, data)
			}
		}
	}
}

func parseOrderBookMessage(msg map[string]interface{}) OrderBookUpdate {
	symbol := strings.ToUpper(msg["s"].(string))
	
	ob := OrderBookUpdate{
		Symbol:    symbol,
		Timestamp: time.Now(),
		Bids:      make([]PriceLevel, 0),
		Asks:      make([]PriceLevel, 0),
	}

	// Parse bids
	if bidsData, ok := msg["b"].([]interface{}); ok {
		for i, bid := range bidsData {
			if i >= 10 { // Limit to top 10
				break
			}
			if bidArray, ok := bid.([]interface{}); ok && len(bidArray) >= 2 {
				price, _ := strconv.ParseFloat(bidArray[0].(string), 64)
				qty, _ := strconv.ParseFloat(bidArray[1].(string), 64)
				ob.Bids = append(ob.Bids, PriceLevel{Price: price, Quantity: qty})
			}
		}
	}

	// Parse asks
	if asksData, ok := msg["a"].([]interface{}); ok {
		for i, ask := range asksData {
			if i >= 10 { // Limit to top 10
				break
			}
			if askArray, ok := ask.([]interface{}); ok && len(askArray) >= 2 {
				price, _ := strconv.ParseFloat(askArray[0].(string), 64)
				qty, _ := strconv.ParseFloat(askArray[1].(string), 64)
				ob.Asks = append(ob.Asks, PriceLevel{Price: price, Quantity: qty})
			}
		}
	}

	return ob
}

func publishOrderBook(ob OrderBookUpdate, nc *nats.Conn, logger *zap.Logger) {
	// Publish to orderbook-specific subject
	subject := fmt.Sprintf("market.orderbook.binance.%s", ob.Symbol)
	
	data, err := json.Marshal(ob)
	if err != nil {
		logger.Error("Failed to marshal orderbook", zap.Error(err))
		return
	}

	if err := nc.Publish(subject, data); err != nil {
		logger.Error("Failed to publish orderbook", 
			zap.String("subject", subject),
			zap.Error(err))
	} else {
		logger.Debug("Published orderbook", 
			zap.String("symbol", ob.Symbol),
			zap.Int("bids", len(ob.Bids)),
			zap.Int("asks", len(ob.Asks)))
	}
}