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

	"github.com/adshao/go-binance/v2"
	"github.com/gorilla/websocket"
	"github.com/mExOms/internal/position"
	"github.com/mExOms/pkg/nats"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// MarketDataFeed connects to Binance and publishes real market data to NATS
type MarketDataFeed struct {
	logger          *zap.Logger
	natsClient      *nats.Client
	client          *binance.Client
	symbols         []string
	positionManager *position.PositionManager
}

// TickerData represents ticker information
type TickerData struct {
	Symbol    string          `json:"symbol"`
	Price     decimal.Decimal `json:"price"`
	Volume    decimal.Decimal `json:"volume"`
	High      decimal.Decimal `json:"high"`
	Low       decimal.Decimal `json:"low"`
	Change    decimal.Decimal `json:"change"`
	ChangePercent decimal.Decimal `json:"change_percent"`
	Timestamp time.Time       `json:"timestamp"`
}

// OrderBookData represents order book snapshot
type OrderBookData struct {
	Symbol    string              `json:"symbol"`
	Bids      []PriceLevel        `json:"bids"`
	Asks      []PriceLevel        `json:"asks"`
	Timestamp time.Time           `json:"timestamp"`
}

type PriceLevel struct {
	Price    decimal.Decimal `json:"price"`
	Quantity decimal.Decimal `json:"quantity"`
}

func main() {
	// Initialize logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	// Initialize NATS client
	natsClient, err := nats.NewClient(nats.Config{
		URL:           "nats://localhost:4222",
		MaxReconnects: 5,
	}, logger)
	if err != nil {
		logger.Fatal("Failed to connect to NATS", zap.Error(err))
	}
	defer natsClient.Close()

	// Initialize position manager
	posManager, err := position.NewPositionManager("/tmp/oms/snapshots", natsClient, logger)
	if err != nil {
		logger.Fatal("Failed to initialize position manager", zap.Error(err))
	}
	defer posManager.Close()

	// Create Binance client (no API keys needed for public data)
	client := binance.NewClient("", "")

	// Create market data feed
	feed := &MarketDataFeed{
		logger:          logger,
		natsClient:      natsClient,
		client:          client,
		symbols:         []string{"BTCUSDT", "ETHUSDT", "BNBUSDT", "SOLUSDT", "XRPUSDT"},
		positionManager: posManager,
	}

	// Start feeds
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Get initial market data
	feed.getInitialData(ctx)

	// Start WebSocket streams
	go feed.startTickerStream(ctx)
	go feed.startDepthStream(ctx)
	go feed.startTradeStream(ctx)
	
	// If API keys are provided, get account info (read-only)
	apiKey := os.Getenv("BINANCE_API_KEY")
	apiSecret := os.Getenv("BINANCE_API_SECRET")
	if apiKey != "" && apiSecret != "" {
		logger.Info("API credentials found, fetching account info...")
		authClient := binance.NewClient(apiKey, apiSecret)
		go feed.monitorAccount(ctx, authClient)
	} else {
		logger.Info("No API credentials, running in public data mode only")
	}

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	<-sigChan
	logger.Info("Shutting down market data feed...")
}

// Get initial market data via REST API
func (f *MarketDataFeed) getInitialData(ctx context.Context) {
	f.logger.Info("Fetching initial market data...")

	// Get 24hr ticker stats
	tickers, err := f.client.NewListPricesService().Do(ctx)
	if err != nil {
		f.logger.Error("Failed to get tickers", zap.Error(err))
		return
	}

	for _, ticker := range tickers {
		for _, symbol := range f.symbols {
			if ticker.Symbol == symbol {
				price, _ := decimal.NewFromString(ticker.Price)
				f.publishTicker(TickerData{
					Symbol:    ticker.Symbol,
					Price:     price,
					Timestamp: time.Now(),
				})
				
				// Update mark price in position manager
				f.positionManager.UpdateMarkPrice("binance", symbol, price)
			}
		}
	}
}

// Start ticker WebSocket stream
func (f *MarketDataFeed) startTickerStream(ctx context.Context) {
	wsURL := "wss://stream.binance.com:9443/ws/" + strings.ToLower(strings.Join(f.symbols, "@ticker/")) + "@ticker"
	
	for {
		select {
		case <-ctx.Done():
			return
		default:
			f.logger.Info("Connecting to ticker stream", zap.String("url", wsURL))
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				f.logger.Error("Failed to connect to ticker stream", zap.Error(err))
				time.Sleep(5 * time.Second)
				continue
			}

			f.handleTickerStream(ctx, conn)
			conn.Close()
			time.Sleep(1 * time.Second)
		}
	}
}

func (f *MarketDataFeed) handleTickerStream(ctx context.Context, conn *websocket.Conn) {
	defer conn.Close()
	
	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, message, err := conn.ReadMessage()
			if err != nil {
				f.logger.Error("Failed to read ticker message", zap.Error(err))
				return
			}

			var tickerEvent map[string]interface{}
			if err := json.Unmarshal(message, &tickerEvent); err != nil {
				f.logger.Error("Failed to parse ticker event", zap.Error(err))
				continue
			}

			if eventType, ok := tickerEvent["e"].(string); ok && eventType == "24hrTicker" {
				symbol := tickerEvent["s"].(string)
				price, _ := decimal.NewFromString(tickerEvent["c"].(string))
				volume, _ := decimal.NewFromString(tickerEvent["v"].(string))
				high, _ := decimal.NewFromString(tickerEvent["h"].(string))
				low, _ := decimal.NewFromString(tickerEvent["l"].(string))
				priceChange, _ := decimal.NewFromString(tickerEvent["p"].(string))
				priceChangePercent, _ := decimal.NewFromString(tickerEvent["P"].(string))

				ticker := TickerData{
					Symbol:        symbol,
					Price:         price,
					Volume:        volume,
					High:          high,
					Low:           low,
					Change:        priceChange,
					ChangePercent: priceChangePercent,
					Timestamp:     time.Now(),
				}

				f.publishTicker(ticker)
				
				// Update mark price in position manager
				f.positionManager.UpdateMarkPrice("binance", symbol, price)
			}
		}
	}
}

// Start depth (order book) WebSocket stream
func (f *MarketDataFeed) startDepthStream(ctx context.Context) {
	// Subscribe to partial book depth (top 20 levels)
	wsURL := "wss://stream.binance.com:9443/ws/" + strings.ToLower(f.symbols[0]) + "@depth20@100ms"
	
	for {
		select {
		case <-ctx.Done():
			return
		default:
			f.logger.Info("Connecting to depth stream", zap.String("url", wsURL))
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				f.logger.Error("Failed to connect to depth stream", zap.Error(err))
				time.Sleep(5 * time.Second)
				continue
			}

			f.handleDepthStream(ctx, conn)
			conn.Close()
			time.Sleep(1 * time.Second)
		}
	}
}

func (f *MarketDataFeed) handleDepthStream(ctx context.Context, conn *websocket.Conn) {
	defer conn.Close()
	
	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, message, err := conn.ReadMessage()
			if err != nil {
				f.logger.Error("Failed to read depth message", zap.Error(err))
				return
			}

			var depthUpdate map[string]interface{}
			if err := json.Unmarshal(message, &depthUpdate); err != nil {
				f.logger.Error("Failed to parse depth update", zap.Error(err))
				continue
			}

			// Parse and publish order book
			orderBook := f.parseOrderBook(f.symbols[0], depthUpdate)
			f.publishOrderBook(orderBook)
		}
	}
}

// Start trade WebSocket stream
func (f *MarketDataFeed) startTradeStream(ctx context.Context) {
	wsURL := "wss://stream.binance.com:9443/ws/" + strings.ToLower(f.symbols[0]) + "@trade"
	
	for {
		select {
		case <-ctx.Done():
			return
		default:
			f.logger.Info("Connecting to trade stream", zap.String("url", wsURL))
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				f.logger.Error("Failed to connect to trade stream", zap.Error(err))
				time.Sleep(5 * time.Second)
				continue
			}

			f.handleTradeStream(ctx, conn)
			conn.Close()
			time.Sleep(1 * time.Second)
		}
	}
}

func (f *MarketDataFeed) handleTradeStream(ctx context.Context, conn *websocket.Conn) {
	defer conn.Close()
	
	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, message, err := conn.ReadMessage()
			if err != nil {
				f.logger.Error("Failed to read trade message", zap.Error(err))
				return
			}

			var trade map[string]interface{}
			if err := json.Unmarshal(message, &trade); err != nil {
				f.logger.Error("Failed to parse trade", zap.Error(err))
				continue
			}

			f.publishTrade(trade)
		}
	}
}

// Monitor account (read-only)
func (f *MarketDataFeed) monitorAccount(ctx context.Context, client *binance.Client) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Get account info
			account, err := client.NewGetAccountService().Do(ctx)
			if err != nil {
				f.logger.Error("Failed to get account info", zap.Error(err))
				continue
			}

			// Publish account balances
			f.publishAccountInfo(account)

			// Get open orders (just to monitor, not to place)
			openOrders, err := client.NewListOpenOrdersService().Do(ctx)
			if err != nil {
				f.logger.Error("Failed to get open orders", zap.Error(err))
				continue
			}

			f.logger.Info("Account status",
				zap.Int("balances", len(account.Balances)),
				zap.Int("open_orders", len(openOrders)))
		}
	}
}

// Publishing methods
func (f *MarketDataFeed) publishTicker(ticker TickerData) {
	subject := fmt.Sprintf("market.data.binance.%s", ticker.Symbol)
	
	data, err := json.Marshal(ticker)
	if err != nil {
		f.logger.Error("Failed to marshal ticker", zap.Error(err))
		return
	}

	if err := f.natsClient.Publish(subject, data); err != nil {
		f.logger.Error("Failed to publish ticker", zap.Error(err))
	}
}

func (f *MarketDataFeed) publishOrderBook(orderBook OrderBookData) {
	subject := fmt.Sprintf("market.orderbook.binance.%s", orderBook.Symbol)
	
	data, err := json.Marshal(orderBook)
	if err != nil {
		f.logger.Error("Failed to marshal order book", zap.Error(err))
		return
	}

	if err := f.natsClient.Publish(subject, data); err != nil {
		f.logger.Error("Failed to publish order book", zap.Error(err))
	}
}

func (f *MarketDataFeed) publishTrade(trade map[string]interface{}) {
	symbol := trade["s"].(string)
	subject := fmt.Sprintf("market.trade.binance.%s", symbol)
	
	data, err := json.Marshal(trade)
	if err != nil {
		f.logger.Error("Failed to marshal trade", zap.Error(err))
		return
	}

	if err := f.natsClient.Publish(subject, data); err != nil {
		f.logger.Error("Failed to publish trade", zap.Error(err))
	}
}

func (f *MarketDataFeed) publishAccountInfo(account *binance.Account) {
	// Only publish non-zero balances
	balances := make(map[string]interface{})
	for _, balance := range account.Balances {
		free, _ := decimal.NewFromString(balance.Free)
		locked, _ := decimal.NewFromString(balance.Locked)
		
		if free.IsPositive() || locked.IsPositive() {
			balances[balance.Asset] = map[string]string{
				"free":   balance.Free,
				"locked": balance.Locked,
			}
		}
	}

	accountData := map[string]interface{}{
		"exchange":     "binance",
		"balances":     balances,
		"can_trade":    account.CanTrade,
		"can_withdraw": account.CanWithdraw,
		"can_deposit":  account.CanDeposit,
		"update_time":  account.UpdateTime,
		"timestamp":    time.Now(),
	}

	data, err := json.Marshal(accountData)
	if err != nil {
		f.logger.Error("Failed to marshal account info", zap.Error(err))
		return
	}

	subject := "account.info.binance"
	if err := f.natsClient.Publish(subject, data); err != nil {
		f.logger.Error("Failed to publish account info", zap.Error(err))
	}
}

func (f *MarketDataFeed) parseOrderBook(symbol string, data map[string]interface{}) OrderBookData {
	orderBook := OrderBookData{
		Symbol:    symbol,
		Timestamp: time.Now(),
		Bids:      make([]PriceLevel, 0),
		Asks:      make([]PriceLevel, 0),
	}

	// Parse bids
	if bids, ok := data["bids"].([]interface{}); ok {
		for i, bid := range bids {
			if i >= 10 { // Limit to top 10 levels
				break
			}
			if bidArray, ok := bid.([]interface{}); ok && len(bidArray) >= 2 {
				price, _ := decimal.NewFromString(bidArray[0].(string))
				quantity, _ := decimal.NewFromString(bidArray[1].(string))
				orderBook.Bids = append(orderBook.Bids, PriceLevel{
					Price:    price,
					Quantity: quantity,
				})
			}
		}
	}

	// Parse asks
	if asks, ok := data["asks"].([]interface{}); ok {
		for i, ask := range asks {
			if i >= 10 { // Limit to top 10 levels
				break
			}
			if askArray, ok := ask.([]interface{}); ok && len(askArray) >= 2 {
				price, _ := decimal.NewFromString(askArray[0].(string))
				quantity, _ := decimal.NewFromString(askArray[1].(string))
				orderBook.Asks = append(orderBook.Asks, PriceLevel{
					Price:    price,
					Quantity: quantity,
				})
			}
		}
	}

	return orderBook
}