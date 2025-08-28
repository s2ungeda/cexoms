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

	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Simplified Binance market data feeder that publishes directly to NATS
// This matches the subjects expected by the monitoring dashboard

type TickerData struct {
	Symbol        string    `json:"symbol"`
	Price         float64   `json:"price"`
	Volume        float64   `json:"volume"`
	High          float64   `json:"high"`
	Low           float64   `json:"low"`
	Change        float64   `json:"change"`
	ChangePct     float64   `json:"change_pct"`
	Bid           float64   `json:"bid"`
	Ask           float64   `json:"ask"`
	LastTradeTime time.Time `json:"last_trade_time"`
	Timestamp     time.Time `json:"timestamp"`
}

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Connect directly to NATS
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	logger.Info("Connected to NATS, starting Binance WebSocket streams...")

	// Symbols to track
	symbols := []string{"btcusdt", "ethusdt", "bnbusdt", "solusdt", "xrpusdt"}
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start ticker stream
	go startTickerStream(ctx, symbols, nc, logger)
	
	// Only publish real data - no demo data
	// go publishDemoData(ctx, nc, logger)

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	<-sigChan
	logger.Info("Shutting down...")
}

func startTickerStream(ctx context.Context, symbols []string, nc *nats.Conn, logger *zap.Logger) {
	streams := make([]string, len(symbols))
	for i, symbol := range symbols {
		streams[i] = symbol + "@ticker"
	}
	wsURL := fmt.Sprintf("wss://stream.binance.com:9443/ws/%s", strings.Join(streams, "/"))

	for {
		select {
		case <-ctx.Done():
			return
		default:
			logger.Info("Connecting to Binance ticker stream...")
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				logger.Error("WebSocket connection failed", zap.Error(err))
				time.Sleep(5 * time.Second)
				continue
			}

			handleTickerStream(ctx, conn, nc, logger)
			conn.Close()
			time.Sleep(1 * time.Second)
		}
	}
}

func handleTickerStream(ctx context.Context, conn *websocket.Conn, nc *nats.Conn, logger *zap.Logger) {
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

			if eventType, ok := msg["e"].(string); ok && eventType == "24hrTicker" {
				ticker := parseTickerMessage(msg)
				publishTicker(ticker, nc, logger)
			}
		}
	}
}

func parseTickerMessage(msg map[string]interface{}) TickerData {
	symbol := strings.ToUpper(getString(msg, "s"))
	
	price, _ := decimal.NewFromString(getString(msg, "c"))
	volume, _ := decimal.NewFromString(getString(msg, "v"))
	high, _ := decimal.NewFromString(getString(msg, "h"))
	low, _ := decimal.NewFromString(getString(msg, "l"))
	change, _ := decimal.NewFromString(getString(msg, "p"))
	changePct, _ := decimal.NewFromString(getString(msg, "P"))
	bid, _ := decimal.NewFromString(getString(msg, "b"))
	ask, _ := decimal.NewFromString(getString(msg, "a"))
	
	return TickerData{
		Symbol:        symbol,
		Price:         price.InexactFloat64(),
		Volume:        volume.InexactFloat64(),
		High:          high.InexactFloat64(),
		Low:           low.InexactFloat64(),
		Change:        change.InexactFloat64(),
		ChangePct:     changePct.InexactFloat64(),
		Bid:           bid.InexactFloat64(),
		Ask:           ask.InexactFloat64(),
		LastTradeTime: time.Now(),
		Timestamp:     time.Now(),
	}
}

func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

func publishTicker(ticker TickerData, nc *nats.Conn, logger *zap.Logger) {
	// Publish to the subject format expected by dashboard
	subject := fmt.Sprintf("market.data.binance.%s", ticker.Symbol)
	
	data, err := json.Marshal(ticker)
	if err != nil {
		logger.Error("Failed to marshal ticker", zap.Error(err))
		return
	}

	if err := nc.Publish(subject, data); err != nil {
		logger.Error("Failed to publish ticker", 
			zap.String("subject", subject),
			zap.Error(err))
	} else {
		logger.Info("Published ticker", 
			zap.String("symbol", ticker.Symbol),
			zap.Float64("price", ticker.Price))
	}
}

func publishDemoData(ctx context.Context, nc *nats.Conn, logger *zap.Logger) {
	// Publish some demo position and order data periodically
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Publish demo position update
			position := map[string]interface{}{
				"event_type": "UPDATE",
				"position": map[string]interface{}{
					"symbol":         "BTCUSDT",
					"exchange":       "binance",
					"market":         "spot",
					"side":           "LONG",
					"quantity":       "0.1",
					"entry_price":    "45000",
					"mark_price":     "45500",
					"unrealized_pnl": "50",
					"leverage":       1,
				},
				"timestamp": time.Now(),
			}

			posData, _ := json.Marshal(position)
			subject := "position.update.binance.spot.BTCUSDT"
			if err := nc.Publish(subject, posData); err != nil {
				logger.Error("Failed to publish position", zap.Error(err))
			} else {
				logger.Info("Published demo position update")
			}

			// Publish demo order
			order := map[string]interface{}{
				"event_type": "NEW",
				"order": map[string]interface{}{
					"id":         fmt.Sprintf("ORD_%d", time.Now().Unix()),
					"symbol":     "BTCUSDT",
					"exchange":   "binance",
					"market":     "spot",
					"side":       "BUY",
					"type":       "LIMIT",
					"price":      "45000",
					"quantity":   "0.01",
					"status":     "NEW",
					"created_at": time.Now(),
				},
				"timestamp": time.Now(),
			}

			orderData, _ := json.Marshal(order)
			orderSubject := "order.event.binance.spot.BTCUSDT"
			if err := nc.Publish(orderSubject, orderData); err != nil {
				logger.Error("Failed to publish order", zap.Error(err))
			} else {
				logger.Info("Published demo order")
			}

			// Publish system health
			health := map[string]interface{}{
				"status": "healthy",
				"components": []map[string]interface{}{
					{
						"name":    "binance",
						"status":  "healthy",
						"message": "Connected to Binance WebSocket",
					},
					{
						"name":    "nats",
						"status":  "healthy", 
						"message": "NATS connection active",
					},
				},
				"timestamp": time.Now(),
			}

			healthData, _ := json.Marshal(health)
			if err := nc.Publish("oms.health.system", healthData); err != nil {
				logger.Error("Failed to publish health", zap.Error(err))
			}
		}
	}
}