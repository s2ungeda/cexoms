package spot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/adshao/go-binance/v2"
	"github.com/gorilla/websocket"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// WsManager handles WebSocket connections for Binance Spot
type WsManager struct {
	apiKey    string
	apiSecret string
	testnet   bool
	
	// User data stream
	listenKey       string
	userConn        *websocket.Conn
	userStopCh      chan struct{}
	userConnected   bool
	
	// Market data streams
	marketConns     map[string]*websocket.Conn  // symbol -> connection
	marketStopChs   map[string]chan struct{}    // symbol -> stop channel
	
	// Callbacks
	orderCallbacks       []types.OrderUpdateCallback
	balanceCallbacks     []types.BalanceUpdateCallback
	marketDataCallbacks  map[string][]types.MarketDataCallback
	
	// Synchronization
	mu              sync.RWMutex
	reconnectDelay  time.Duration
	keepAliveTimer  *time.Timer
}

// NewWsManager creates a new WebSocket manager
func NewWsManager(apiKey, apiSecret string, testnet bool) *WsManager {
	return &WsManager{
		apiKey:              apiKey,
		apiSecret:           apiSecret,
		testnet:             testnet,
		marketConns:         make(map[string]*websocket.Conn),
		marketStopChs:       make(map[string]chan struct{}),
		marketDataCallbacks: make(map[string][]types.MarketDataCallback),
		reconnectDelay:      5 * time.Second,
	}
}

// Initialize starts the WebSocket connections
func (wm *WsManager) Initialize(ctx context.Context) error {
	// Start user data stream
	if err := wm.startUserDataStream(ctx); err != nil {
		log.Printf("Failed to start user data stream: %v", err)
		// Don't return error as market data can still work
	}
	
	return nil
}

// startUserDataStream starts the user data WebSocket stream
func (wm *WsManager) startUserDataStream(ctx context.Context) error {
	client := binance.NewClient(wm.apiKey, wm.apiSecret)
	if wm.testnet {
		client.BaseURL = "https://testnet.binance.vision/api"
	}
	
	// Get listen key
	listenKey, err := client.NewStartUserStreamService().Do(ctx)
	if err != nil {
		return fmt.Errorf("failed to get listen key: %v", err)
	}
	
	wm.mu.Lock()
	wm.listenKey = listenKey
	wm.mu.Unlock()
	
	// Connect to user data stream
	wsURL := fmt.Sprintf("wss://stream.binance.com:9443/ws/%s", listenKey)
	if wm.testnet {
		wsURL = fmt.Sprintf("wss://testnet.binance.vision/ws/%s", listenKey)
	}
	
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to user stream: %v", err)
	}
	
	wm.mu.Lock()
	wm.userConn = conn
	wm.userStopCh = make(chan struct{})
	wm.userConnected = true
	wm.mu.Unlock()
	
	// Start reading messages
	go wm.readUserStream()
	
	// Start keepalive
	wm.startKeepAlive(ctx, client)
	
	log.Printf("User data stream connected")
	return nil
}

// readUserStream reads messages from user data stream
func (wm *WsManager) readUserStream() {
	defer func() {
		wm.mu.Lock()
		wm.userConnected = false
		if wm.userConn != nil {
			wm.userConn.Close()
		}
		wm.mu.Unlock()
	}()
	
	for {
		select {
		case <-wm.userStopCh:
			return
		default:
			_, message, err := wm.userConn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("User stream error: %v", err)
				}
				// Try to reconnect
				go wm.reconnectUserStream()
				return
			}
			
			// Parse message
			var event map[string]interface{}
			if err := json.Unmarshal(message, &event); err != nil {
				log.Printf("Failed to parse user stream message: %v", err)
				continue
			}
			
			// Handle different event types
			eventType, _ := event["e"].(string)
			switch eventType {
			case "executionReport":
				wm.handleOrderUpdate(event)
			case "outboundAccountPosition":
				wm.handleBalanceUpdate(event)
			default:
				// Unknown event type
			}
		}
	}
}

// handleOrderUpdate processes order update events
func (wm *WsManager) handleOrderUpdate(event map[string]interface{}) {
	update := &types.OrderUpdate{
		Symbol:        event["s"].(string),
		ClientOrderID: event["c"].(string),
		Side:          event["S"].(string),
		Type:          event["o"].(string),
		TimeInForce:   event["f"].(string),
		Quantity:      parseDecimalField(event["q"]),
		Price:         parseDecimalField(event["p"]),
		Status:        event["X"].(string),
		OrderID:       fmt.Sprintf("%.0f", event["i"].(float64)),
		LastFilledQty: parseDecimalField(event["l"]),
		FilledQty:     parseDecimalField(event["z"]),
		LastPrice:     parseDecimalField(event["L"]),
		FeeAsset:      getStringField(event, "N"),
		Fee:           parseDecimalField(event["n"]),
		Time:          int64(event["T"].(float64)),
		UpdateTime:    int64(event["E"].(float64)),
	}
	
	// Calculate executed quote quantity
	if update.FilledQty.GreaterThan(decimal.Zero) && update.Price.GreaterThan(decimal.Zero) {
		update.ExecutedQuoteQty = update.FilledQty.Mul(update.Price)
	}
	
	// Call callbacks
	wm.mu.RLock()
	callbacks := wm.orderCallbacks
	wm.mu.RUnlock()
	
	for _, callback := range callbacks {
		go callback(update)
	}
}

// handleBalanceUpdate processes balance update events
func (wm *WsManager) handleBalanceUpdate(event map[string]interface{}) {
	balances, ok := event["B"].([]interface{})
	if !ok {
		return
	}
	
	updates := make([]*types.BalanceUpdate, 0)
	for _, b := range balances {
		balance := b.(map[string]interface{})
		asset := balance["a"].(string)
		free := parseDecimalField(balance["f"])
		locked := parseDecimalField(balance["l"])
		
		updates = append(updates, &types.BalanceUpdate{
			Asset:      asset,
			Free:       free,
			Locked:     locked,
			Total:      free.Add(locked),
			UpdateTime: int64(event["E"].(float64)),
		})
	}
	
	// Call callbacks
	wm.mu.RLock()
	callbacks := wm.balanceCallbacks
	wm.mu.RUnlock()
	
	for _, callback := range callbacks {
		go callback(updates)
	}
}

// startKeepAlive starts the keepalive timer for user stream
func (wm *WsManager) startKeepAlive(ctx context.Context, client *binance.Client) {
	wm.keepAliveTimer = time.NewTimer(30 * time.Minute)
	
	go func() {
		for {
			select {
			case <-wm.keepAliveTimer.C:
				wm.mu.RLock()
				listenKey := wm.listenKey
				wm.mu.RUnlock()
				
				if listenKey != "" {
					err := client.NewKeepaliveUserStreamService().ListenKey(listenKey).Do(ctx)
					if err != nil {
						log.Printf("Failed to keepalive user stream: %v", err)
					}
				}
				
				// Reset timer
				wm.keepAliveTimer.Reset(30 * time.Minute)
			case <-wm.userStopCh:
				wm.keepAliveTimer.Stop()
				return
			}
		}
	}()
}

// reconnectUserStream attempts to reconnect user data stream
func (wm *WsManager) reconnectUserStream() {
	time.Sleep(wm.reconnectDelay)
	
	ctx := context.Background()
	if err := wm.startUserDataStream(ctx); err != nil {
		log.Printf("Failed to reconnect user stream: %v", err)
		// Try again
		go wm.reconnectUserStream()
	}
}

// SubscribeMarketData subscribes to market data for a symbol
func (wm *WsManager) SubscribeMarketData(ctx context.Context, symbol string, callback types.MarketDataCallback) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	
	// Add callback
	if wm.marketDataCallbacks[symbol] == nil {
		wm.marketDataCallbacks[symbol] = make([]types.MarketDataCallback, 0)
	}
	wm.marketDataCallbacks[symbol] = append(wm.marketDataCallbacks[symbol], callback)
	
	// Check if already connected
	if _, exists := wm.marketConns[symbol]; exists {
		return nil
	}
	
	// Connect to market stream
	streams := []string{
		fmt.Sprintf("%s@trade", symbol),
		fmt.Sprintf("%s@depth20@100ms", symbol),
		fmt.Sprintf("%s@ticker", symbol),
	}
	
	wsURL := fmt.Sprintf("wss://stream.binance.com:9443/stream?streams=%s", 
		joinStreams(streams))
	if wm.testnet {
		wsURL = fmt.Sprintf("wss://testnet.binance.vision/stream?streams=%s", 
			joinStreams(streams))
	}
	
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to market stream: %v", err)
	}
	
	wm.marketConns[symbol] = conn
	wm.marketStopChs[symbol] = make(chan struct{})
	
	// Start reading messages
	go wm.readMarketStream(symbol, conn)
	
	log.Printf("Market data stream connected for %s", symbol)
	return nil
}

// readMarketStream reads messages from market data stream
func (wm *WsManager) readMarketStream(symbol string, conn *websocket.Conn) {
	defer func() {
		conn.Close()
		wm.mu.Lock()
		delete(wm.marketConns, symbol)
		delete(wm.marketStopChs, symbol)
		wm.mu.Unlock()
	}()
	
	stopCh := wm.marketStopChs[symbol]
	
	for {
		select {
		case <-stopCh:
			return
		default:
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("Market stream error for %s: %v", symbol, err)
				}
				// Try to reconnect
				go wm.reconnectMarketStream(symbol)
				return
			}
			
			// Parse message
			var streamData struct {
				Stream string                 `json:"stream"`
				Data   map[string]interface{} `json:"data"`
			}
			
			if err := json.Unmarshal(message, &streamData); err != nil {
				log.Printf("Failed to parse market stream message: %v", err)
				continue
			}
			
			// Create market data update
			marketData := &types.MarketDataUpdate{
				Symbol:    symbol,
				Timestamp: time.Now(),
			}
			
			// Handle different stream types
			if streamData.Data["e"] == "trade" {
				marketData.LastPrice = parseDecimalField(streamData.Data["p"])
				marketData.LastQty = parseDecimalField(streamData.Data["q"])
			} else if streamData.Data["e"] == "24hrTicker" {
				marketData.BidPrice = parseDecimalField(streamData.Data["b"])
				marketData.BidQty = parseDecimalField(streamData.Data["B"])
				marketData.AskPrice = parseDecimalField(streamData.Data["a"])
				marketData.AskQty = parseDecimalField(streamData.Data["A"])
				marketData.Volume = parseDecimalField(streamData.Data["v"])
				marketData.QuoteVolume = parseDecimalField(streamData.Data["q"])
				marketData.High24h = parseDecimalField(streamData.Data["h"])
				marketData.Low24h = parseDecimalField(streamData.Data["l"])
			}
			
			// Call callbacks
			wm.mu.RLock()
			callbacks := wm.marketDataCallbacks[symbol]
			wm.mu.RUnlock()
			
			for _, callback := range callbacks {
				go callback(marketData)
			}
		}
	}
}

// reconnectMarketStream attempts to reconnect market data stream
func (wm *WsManager) reconnectMarketStream(symbol string) {
	time.Sleep(wm.reconnectDelay)
	
	// Get callbacks
	wm.mu.RLock()
	callbacks := wm.marketDataCallbacks[symbol]
	wm.mu.RUnlock()
	
	if len(callbacks) == 0 {
		return
	}
	
	ctx := context.Background()
	// Resubscribe with first callback
	if err := wm.SubscribeMarketData(ctx, symbol, callbacks[0]); err != nil {
		log.Printf("Failed to reconnect market stream for %s: %v", symbol, err)
		// Try again
		go wm.reconnectMarketStream(symbol)
	}
}

// UnsubscribeMarketData unsubscribes from market data
func (wm *WsManager) UnsubscribeMarketData(symbol string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	
	// Close connection
	if conn, exists := wm.marketConns[symbol]; exists {
		conn.Close()
	}
	
	// Close stop channel
	if stopCh, exists := wm.marketStopChs[symbol]; exists {
		close(stopCh)
	}
	
	// Remove from maps
	delete(wm.marketConns, symbol)
	delete(wm.marketStopChs, symbol)
	delete(wm.marketDataCallbacks, symbol)
	
	return nil
}

// RegisterOrderCallback registers a callback for order updates
func (wm *WsManager) RegisterOrderCallback(callback types.OrderUpdateCallback) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.orderCallbacks = append(wm.orderCallbacks, callback)
}

// RegisterBalanceCallback registers a callback for balance updates
func (wm *WsManager) RegisterBalanceCallback(callback types.BalanceUpdateCallback) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.balanceCallbacks = append(wm.balanceCallbacks, callback)
}

// IsConnected returns whether user stream is connected
func (wm *WsManager) IsConnected() bool {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return wm.userConnected
}

// Close closes all WebSocket connections
func (wm *WsManager) Close() error {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	
	// Stop keepalive timer
	if wm.keepAliveTimer != nil {
		wm.keepAliveTimer.Stop()
	}
	
	// Close user stream
	if wm.userStopCh != nil {
		close(wm.userStopCh)
	}
	if wm.userConn != nil {
		wm.userConn.Close()
	}
	
	// Close market streams
	for _, conn := range wm.marketConns {
		conn.Close()
	}
	for _, stopCh := range wm.marketStopChs {
		close(stopCh)
	}
	
	// Clear maps
	wm.marketConns = make(map[string]*websocket.Conn)
	wm.marketStopChs = make(map[string]chan struct{})
	wm.marketDataCallbacks = make(map[string][]types.MarketDataCallback)
	
	return nil
}

// Helper functions
func parseDecimalField(v interface{}) decimal.Decimal {
	switch val := v.(type) {
	case string:
		d, _ := decimal.NewFromString(val)
		return d
	case float64:
		return decimal.NewFromFloat(val)
	default:
		return decimal.Zero
	}
}

func getStringField(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func joinStreams(streams []string) string {
	result := ""
	for i, s := range streams {
		if i > 0 {
			result += "/"
		}
		result += s
	}
	return result
}