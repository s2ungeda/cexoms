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

	"github.com/gorilla/websocket"
	"github.com/mExOms/pkg/nats"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Simple Binance WebSocket market data feeder
// Publishes real-time data to NATS for the monitoring dashboard

type TickerData struct {
	Symbol        string    `json:"s"`
	Price         string    `json:"c"`   // Current price
	Volume        string    `json:"v"`   // 24hr volume
	QuoteVolume   string    `json:"q"`   // 24hr quote volume
	PriceChange   string    `json:"p"`   // 24hr price change
	PricePercent  string    `json:"P"`   // 24hr price change percent
	High          string    `json:"h"`   // 24hr high
	Low           string    `json:"l"`   // 24hr low
	EventTime     int64     `json:"E"`   // Event time
}

type MarketData struct {
	Symbol    string          `json:"symbol"`
	Price     decimal.Decimal `json:"price"`
	Volume    decimal.Decimal `json:"volume"`
	High      decimal.Decimal `json:"high"`
	Low       decimal.Decimal `json:"low"`
	Change    decimal.Decimal `json:"change"`
	ChangePct decimal.Decimal `json:"change_pct"`
	Timestamp time.Time       `json:"timestamp"`
}

type DepthUpdate struct {
	Symbol    string      `json:"s"`
	Bids      [][]string  `json:"b"`  // [price, quantity]
	Asks      [][]string  `json:"a"`  // [price, quantity]
	LastUpdateID int64   `json:"u"`
}

type OrderBook struct {
	Symbol    string          `json:"symbol"`
	Bids      []PriceLevel    `json:"bids"`
	Asks      []PriceLevel    `json:"asks"`
	Timestamp time.Time       `json:"timestamp"`
}

type PriceLevel struct {
	Price    decimal.Decimal `json:"price"`
	Quantity decimal.Decimal `json:"quantity"`
}

func main() {
	// Initialize logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Initialize NATS client
	natsClient, err := nats.NewClient(&nats.Config{
		URL:      "nats://localhost:4222",
		ClientID: "binance-market-data",
	})
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer natsClient.Close()

	logger.Info("Connected to NATS, starting Binance WebSocket streams...")

	// Symbols to track
	symbols := []string{"btcusdt", "ethusdt", "bnbusdt", "solusdt", "xrpusdt"}
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start ticker streams
	go startTickerStream(ctx, symbols, natsClient, logger)
	
	// Start depth stream for BTC
	go startDepthStream(ctx, "btcusdt", natsClient, logger)

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	<-sigChan
	logger.Info("Shutting down...")
}

func startTickerStream(ctx context.Context, symbols []string, natsClient *nats.Client, logger *zap.Logger) {
	// Create multi-stream URL
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

			handleTickerStream(ctx, conn, natsClient, logger)
			conn.Close()
			time.Sleep(1 * time.Second)
		}
	}
}

func handleTickerStream(ctx context.Context, conn *websocket.Conn, natsClient *nats.Client, logger *zap.Logger) {
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

			// Check if it's a ticker event
			if eventType, ok := msg["e"].(string); ok && eventType == "24hrTicker" {
				ticker := parseTickerMessage(msg)
				publishMarketData(ticker, natsClient, logger)
			}
		}
	}
}

func startDepthStream(ctx context.Context, symbol string, natsClient *nats.Client, logger *zap.Logger) {
	wsURL := fmt.Sprintf("wss://stream.binance.com:9443/ws/%s@depth20@100ms", symbol)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			logger.Info("Connecting to Binance depth stream...", zap.String("symbol", symbol))
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				logger.Error("Depth WebSocket connection failed", zap.Error(err))
				time.Sleep(5 * time.Second)
				continue
			}

			handleDepthStream(ctx, conn, natsClient, logger)
			conn.Close()
			time.Sleep(1 * time.Second)
		}
	}
}

func handleDepthStream(ctx context.Context, conn *websocket.Conn, natsClient *nats.Client, logger *zap.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			var depth DepthUpdate
			err := conn.ReadJSON(&depth)
			if err != nil {
				logger.Error("Failed to read depth message", zap.Error(err))
				return
			}

			publishOrderBook(depth, natsClient, logger)
		}
	}
}

func parseTickerMessage(msg map[string]interface{}) MarketData {
	md := MarketData{
		Symbol:    strings.ToUpper(getString(msg, "s")),
		Timestamp: time.Now(),
	}

	// Parse decimal values
	md.Price, _ = decimal.NewFromString(getString(msg, "c"))
	md.Volume, _ = decimal.NewFromString(getString(msg, "v"))
	md.High, _ = decimal.NewFromString(getString(msg, "h"))
	md.Low, _ = decimal.NewFromString(getString(msg, "l"))
	md.Change, _ = decimal.NewFromString(getString(msg, "p"))
	md.ChangePct, _ = decimal.NewFromString(getString(msg, "P"))

	return md
}

func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

func publishMarketData(md MarketData, natsClient *nats.Client, logger *zap.Logger) {
	// Use the client's PublishMarketData method
	if err := natsClient.PublishMarketData("binance", "spot", md.Symbol, md); err != nil {
		logger.Error("Failed to publish market data", 
			zap.String("symbol", md.Symbol),
			zap.Error(err))
	} else {
		logger.Debug("Published market data", 
			zap.String("symbol", md.Symbol),
			zap.String("price", md.Price.String()))
	}
}

func publishOrderBook(depth DepthUpdate, natsClient *nats.Client, logger *zap.Logger) {
	ob := OrderBook{
		Symbol:    strings.ToUpper(depth.Symbol),
		Timestamp: time.Now(),
		Bids:      make([]PriceLevel, 0),
		Asks:      make([]PriceLevel, 0),
	}

	// Convert bids
	for i, bid := range depth.Bids {
		if i >= 10 { // Limit to top 10
			break
		}
		if len(bid) >= 2 {
			price, _ := decimal.NewFromString(bid[0])
			qty, _ := decimal.NewFromString(bid[1])
			ob.Bids = append(ob.Bids, PriceLevel{Price: price, Quantity: qty})
		}
	}

	// Convert asks
	for i, ask := range depth.Asks {
		if i >= 10 { // Limit to top 10
			break
		}
		if len(ask) >= 2 {
			price, _ := decimal.NewFromString(ask[0])
			qty, _ := decimal.NewFromString(ask[1])
			ob.Asks = append(ob.Asks, PriceLevel{Price: price, Quantity: qty})
		}
	}

	// Publish order book using PublishMarketData
	if err := natsClient.PublishMarketData("binance", "spot", ob.Symbol, ob); err != nil {
		logger.Error("Failed to publish order book", 
			zap.String("symbol", ob.Symbol),
			zap.Error(err))
	}
}