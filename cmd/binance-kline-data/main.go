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

// KlineData represents candlestick/kline data
type KlineData struct {
	Symbol    string    `json:"symbol"`
	Interval  string    `json:"interval"`
	OpenTime  int64     `json:"open_time"`
	CloseTime int64     `json:"close_time"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
	Timestamp time.Time `json:"timestamp"`
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

	logger.Info("Connected to NATS, starting Binance Kline streams...")

	// Symbols and intervals to track
	symbols := []string{"btcusdt", "ethusdt", "bnbusdt", "solusdt", "xrpusdt"}
	intervals := []string{"1m", "5m", "1h", "1d"} // 1 minute, 5 minutes, 1 hour, 1 day

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start kline streams for each interval
	for _, interval := range intervals {
		go startKlineStream(ctx, symbols, interval, nc, logger)
	}

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	<-sigChan
	logger.Info("Shutting down...")
}

func startKlineStream(ctx context.Context, symbols []string, interval string, nc *nats.Conn, logger *zap.Logger) {
	// Create multi-stream URL for klines
	streams := make([]string, len(symbols))
	for i, symbol := range symbols {
		streams[i] = fmt.Sprintf("%s@kline_%s", symbol, interval)
	}
	wsURL := fmt.Sprintf("wss://stream.binance.com:9443/ws/%s", strings.Join(streams, "/"))

	for {
		select {
		case <-ctx.Done():
			return
		default:
			logger.Info("Connecting to Binance kline stream", 
				zap.String("interval", interval))
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				logger.Error("WebSocket connection failed", zap.Error(err))
				time.Sleep(5 * time.Second)
				continue
			}

			handleKlineStream(ctx, conn, interval, nc, logger)
			conn.Close()
			time.Sleep(1 * time.Second)
		}
	}
}

func handleKlineStream(ctx context.Context, conn *websocket.Conn, interval string, nc *nats.Conn, logger *zap.Logger) {
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

			// Parse kline event
			if eventType, ok := msg["e"].(string); ok && eventType == "kline" {
				kline := parseKlineMessage(msg)
				publishKline(kline, interval, nc, logger)
			}
		}
	}
}

func parseKlineMessage(msg map[string]interface{}) KlineData {
	klineMap := msg["k"].(map[string]interface{})
	
	symbol := strings.ToUpper(klineMap["s"].(string))
	openTime, _ := strconv.ParseInt(fmt.Sprintf("%.0f", klineMap["t"].(float64)), 10, 64)
	closeTime, _ := strconv.ParseInt(fmt.Sprintf("%.0f", klineMap["T"].(float64)), 10, 64)
	
	open, _ := strconv.ParseFloat(klineMap["o"].(string), 64)
	high, _ := strconv.ParseFloat(klineMap["h"].(string), 64)
	low, _ := strconv.ParseFloat(klineMap["l"].(string), 64)
	close, _ := strconv.ParseFloat(klineMap["c"].(string), 64)
	volume, _ := strconv.ParseFloat(klineMap["v"].(string), 64)
	
	return KlineData{
		Symbol:    symbol,
		Interval:  klineMap["i"].(string),
		OpenTime:  openTime,
		CloseTime: closeTime,
		Open:      open,
		High:      high,
		Low:       low,
		Close:     close,
		Volume:    volume,
		Timestamp: time.Now(),
	}
}

func publishKline(kline KlineData, interval string, nc *nats.Conn, logger *zap.Logger) {
	// Publish to kline-specific subject
	subject := fmt.Sprintf("market.kline.binance.%s.%s", kline.Symbol, interval)
	
	data, err := json.Marshal(kline)
	if err != nil {
		logger.Error("Failed to marshal kline", zap.Error(err))
		return
	}

	if err := nc.Publish(subject, data); err != nil {
		logger.Error("Failed to publish kline", 
			zap.String("subject", subject),
			zap.Error(err))
	} else {
		logger.Debug("Published kline", 
			zap.String("symbol", kline.Symbol),
			zap.String("interval", interval),
			zap.Float64("close", kline.Close))
	}
}