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


// MarketDataUpdate combines ticker and trades
type MarketDataUpdate struct {
	Symbol    string           `json:"symbol"`
	Ticker    *TickerData      `json:"ticker,omitempty"`
	Trade     *Trade           `json:"trade,omitempty"`
	Timestamp time.Time        `json:"timestamp"`
}

type TickerData struct {
	Symbol             string    `json:"symbol"`
	Price              float64   `json:"price"`
	PriceChange        float64   `json:"priceChange"`
	PriceChangePercent float64   `json:"priceChangePercent"`
	High               float64   `json:"high"`
	Low                float64   `json:"low"`
	Volume             float64   `json:"volume"`
	QuoteVolume        float64   `json:"quoteVolume"`
	OpenTime           time.Time `json:"openTime"`
	CloseTime          time.Time `json:"closeTime"`
	Count              int       `json:"count"`
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
	// Combined stream: ticker + trade
	wsURL := fmt.Sprintf("wss://stream.binance.com:9443/ws/%s@ticker/%s@trade", symbol, symbol)

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

			// Check if it's a combined stream message
			if _, ok := msg["stream"].(string); ok {
				// Combined stream format
				if data, ok := msg["data"].(map[string]interface{}); ok {
					if eventType, ok := data["e"].(string); ok {
						switch eventType {
						case "trade":
							trade := parseTrade(data)
							publishMarketData(strings.ToUpper(symbol), nil, &trade, nc, logger)
						case "24hrTicker":
							ticker := parseTicker(data)
							publishMarketData(strings.ToUpper(symbol), &ticker, nil, nc, logger)
						}
					}
				}
			} else {
				// Direct stream format
				if eventType, ok := msg["e"].(string); ok {
					switch eventType {
					case "trade":
						trade := parseTrade(msg)
						publishMarketData(strings.ToUpper(symbol), nil, &trade, nc, logger)
					case "24hrTicker":
						ticker := parseTicker(msg)
						publishMarketData(strings.ToUpper(symbol), &ticker, nil, nc, logger)
					}
				}
			}
		}
	}
}

func parseTicker(msg map[string]interface{}) TickerData {
	symbol := strings.ToUpper(msg["s"].(string))
	price, _ := strconv.ParseFloat(msg["c"].(string), 64)
	priceChange, _ := strconv.ParseFloat(msg["p"].(string), 64)
	priceChangePercent, _ := strconv.ParseFloat(msg["P"].(string), 64)
	high, _ := strconv.ParseFloat(msg["h"].(string), 64)
	low, _ := strconv.ParseFloat(msg["l"].(string), 64)
	volume, _ := strconv.ParseFloat(msg["v"].(string), 64)
	quoteVolume, _ := strconv.ParseFloat(msg["q"].(string), 64)
	count := int(msg["n"].(float64))
	
	openTime := int64(msg["O"].(float64))
	closeTime := int64(msg["C"].(float64))
	
	return TickerData{
		Symbol:             symbol,
		Price:              price,
		PriceChange:        priceChange,
		PriceChangePercent: priceChangePercent,
		High:               high,
		Low:                low,
		Volume:             volume,
		QuoteVolume:        quoteVolume,
		OpenTime:           time.Unix(0, openTime*int64(time.Millisecond)),
		CloseTime:          time.Unix(0, closeTime*int64(time.Millisecond)),
		Count:              count,
	}
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

func publishMarketData(symbol string, ticker *TickerData, trade *Trade, nc *nats.Conn, logger *zap.Logger) {
	update := MarketDataUpdate{
		Symbol:    symbol,
		Ticker:    ticker,
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
		if ticker != nil {
			logger.Debug("Published ticker", 
				zap.String("symbol", symbol),
				zap.Float64("price", ticker.Price),
				zap.Float64("change%", ticker.PriceChangePercent),
				zap.Float64("volume", ticker.Volume))
		}
		if trade != nil {
			logger.Debug("Published trade", 
				zap.String("symbol", symbol),
				zap.Float64("price", trade.Price),
				zap.String("side", trade.Side))
		}
	}
}