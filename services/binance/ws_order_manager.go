package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mExOms/pkg/types"
)

// BinanceWSOrderManager implements types.WebSocketOrderManager for Binance
type BinanceWSOrderManager struct {
	config    types.WebSocketConfig
	conn      *websocket.Conn
	mu        sync.RWMutex
	
	// Connection state
	connected    atomic.Bool
	reconnecting atomic.Bool
	stopCh       chan struct{}
	
	// Request/Response handling
	requestID    atomic.Int64
	responses    map[string]chan *WSOrderResponse
	respMu       sync.RWMutex
	
	// Callbacks
	orderUpdateCallbacks []types.OrderUpdateCallback
	callbackMu          sync.RWMutex
	
	// Metrics
	metrics      types.WebSocketMetrics
	metricsMu    sync.RWMutex
	connectedAt  time.Time
}

// WSOrderRequest represents a WebSocket order request
type WSOrderRequest struct {
	ID     string                 `json:"id"`
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
}

// WSOrderResponse represents a WebSocket order response
type WSOrderResponse struct {
	ID     string          `json:"id"`
	Status int             `json:"status"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *WSError        `json:"error,omitempty"`
}

// WSError represents WebSocket error
type WSError struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// NewBinanceWSOrderManager creates a new Binance WebSocket order manager
func NewBinanceWSOrderManager(config types.WebSocketConfig) *BinanceWSOrderManager {
	return &BinanceWSOrderManager{
		config:    config,
		responses: make(map[string]chan *WSOrderResponse),
		stopCh:    make(chan struct{}),
	}
}

// Connect establishes WebSocket connection
func (m *BinanceWSOrderManager) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.connected.Load() {
		return nil
	}

	// Create WebSocket connection
	dialer := websocket.DefaultDialer
	if m.config.EnableCompression {
		dialer.EnableCompression = true
	}

	conn, _, err := dialer.DialContext(ctx, m.config.URL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to WebSocket: %v", err)
	}

	m.conn = conn
	m.connected.Store(true)
	m.connectedAt = time.Now()

	// Update metrics
	m.updateMetric(func(metrics *types.WebSocketMetrics) {
		metrics.Connected = true
		metrics.ReconnectCount++
	})

	// Start handlers
	go m.readHandler()
	if m.config.EnableHeartbeat {
		go m.heartbeatHandler()
	}

	return nil
}

// Disconnect closes WebSocket connection
func (m *BinanceWSOrderManager) Disconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected.Load() {
		return nil
	}

	close(m.stopCh)
	m.connected.Store(false)

	if m.conn != nil {
		m.conn.Close()
	}

	// Update metrics
	m.updateMetric(func(metrics *types.WebSocketMetrics) {
		metrics.Connected = false
	})

	return nil
}

// IsConnected returns connection status
func (m *BinanceWSOrderManager) IsConnected() bool {
	return m.connected.Load()
}

// CreateOrder creates an order via WebSocket
func (m *BinanceWSOrderManager) CreateOrder(ctx context.Context, order *types.Order) (*types.OrderResponse, error) {
	if !m.connected.Load() {
		return nil, fmt.Errorf("WebSocket not connected")
	}

	timestamp := time.Now().UnixMilli()
	requestID := fmt.Sprintf("order_%d_%d", timestamp, m.requestID.Add(1))

	params := map[string]interface{}{
		"symbol":    order.Symbol,
		"side":      order.Side,
		"type":      order.Type,
		"quantity":  order.Quantity.String(),
		"timestamp": timestamp,
		"apiKey":    m.config.APIKey,
	}

	// Add order type specific parameters
	switch order.Type {
	case types.OrderTypeLimit:
		params["price"] = order.Price.String()
		params["timeInForce"] = order.TimeInForce
		if params["timeInForce"] == "" {
			params["timeInForce"] = types.TimeInForceGTC
		}
	case types.OrderTypeMarket:
		// Market orders don't need price
	case types.OrderTypeStop, types.OrderTypeStopLimit:
		params["stopPrice"] = order.StopPrice.String()
		if order.Type == types.OrderTypeStopLimit {
			params["price"] = order.Price.String()
		}
	}

	// Add optional parameters
	if order.ReduceOnly {
		params["reduceOnly"] = true
	}
	if order.PositionSide != "" {
		params["positionSide"] = order.PositionSide
	}

	// Generate signature
	signature := m.generateSignature(params)
	params["signature"] = signature
	
	// Debug: Print request details
	fmt.Printf("DEBUG - WebSocket Order Request:\n")
	fmt.Printf("  Method: %s\n", "order.place")
	fmt.Printf("  Params: %+v\n", params)

	// Send request and wait for response
	resp, err := m.sendRequest(ctx, "order.place", params, requestID)
	if err != nil {
		m.updateMetric(func(metrics *types.WebSocketMetrics) {
			metrics.OrdersFailed++
		})
		return nil, err
	}

	// Parse response
	var orderResp types.OrderResponse
	if err := json.Unmarshal(resp.Result, &orderResp); err != nil {
		return nil, fmt.Errorf("failed to parse order response: %v", err)
	}

	m.updateMetric(func(metrics *types.WebSocketMetrics) {
		metrics.OrdersSuccessful++
	})

	return &orderResp, nil
}

// CancelOrder cancels an order via WebSocket
func (m *BinanceWSOrderManager) CancelOrder(ctx context.Context, symbol string, orderID string) error {
	if !m.connected.Load() {
		return fmt.Errorf("WebSocket not connected")
	}

	timestamp := time.Now().UnixMilli()
	requestID := fmt.Sprintf("cancel_%d_%d", timestamp, m.requestID.Add(1))

	params := map[string]interface{}{
		"symbol":    symbol,
		"timestamp": timestamp,
		"apiKey":    m.config.APIKey,
	}

	// Try to parse as int64 first (for order ID)
	if id, err := strconv.ParseInt(orderID, 10, 64); err == nil {
		params["orderId"] = id
	} else {
		// Otherwise treat as client order ID
		params["origClientOrderId"] = orderID
	}

	// Generate signature
	signature := m.generateSignature(params)
	params["signature"] = signature

	// Send request
	_, err := m.sendRequest(ctx, "order.cancel", params, requestID)
	return err
}

// ModifyOrder modifies an existing order (not supported by Binance)
func (m *BinanceWSOrderManager) ModifyOrder(ctx context.Context, symbol string, orderID string, newPrice, newQuantity string) error {
	return fmt.Errorf("order modification not supported by Binance")
}

// GetOrderStatus gets order status via WebSocket
func (m *BinanceWSOrderManager) GetOrderStatus(ctx context.Context, symbol string, orderID string) (*types.Order, error) {
	if !m.connected.Load() {
		return nil, fmt.Errorf("WebSocket not connected")
	}

	timestamp := time.Now().UnixMilli()
	requestID := fmt.Sprintf("status_%d_%d", timestamp, m.requestID.Add(1))

	params := map[string]interface{}{
		"symbol":    symbol,
		"timestamp": timestamp,
		"apiKey":    m.config.APIKey,
	}

	// Try to parse as int64 first
	if id, err := strconv.ParseInt(orderID, 10, 64); err == nil {
		params["orderId"] = id
	} else {
		params["origClientOrderId"] = orderID
	}

	// Generate signature
	signature := m.generateSignature(params)
	params["signature"] = signature

	// Send request
	resp, err := m.sendRequest(ctx, "order.status", params, requestID)
	if err != nil {
		return nil, err
	}

	// Parse response
	var order types.Order
	if err := json.Unmarshal(resp.Result, &order); err != nil {
		return nil, fmt.Errorf("failed to parse order status: %v", err)
	}

	return &order, nil
}

// GetOpenOrders gets all open orders via WebSocket
func (m *BinanceWSOrderManager) GetOpenOrders(ctx context.Context, symbol string) ([]*types.Order, error) {
	if !m.connected.Load() {
		return nil, fmt.Errorf("WebSocket not connected")
	}

	timestamp := time.Now().UnixMilli()
	requestID := fmt.Sprintf("open_%d_%d", timestamp, m.requestID.Add(1))

	params := map[string]interface{}{
		"timestamp": timestamp,
		"apiKey":    m.config.APIKey,
	}

	if symbol != "" {
		params["symbol"] = symbol
	}

	// Generate signature
	signature := m.generateSignature(params)
	params["signature"] = signature

	// Send request
	resp, err := m.sendRequest(ctx, "openOrders.status", params, requestID)
	if err != nil {
		return nil, err
	}

	// Parse response
	var orders []*types.Order
	if err := json.Unmarshal(resp.Result, &orders); err != nil {
		return nil, fmt.Errorf("failed to parse open orders: %v", err)
	}

	return orders, nil
}

// SubscribeOrderUpdates subscribes to order update events
func (m *BinanceWSOrderManager) SubscribeOrderUpdates(ctx context.Context, callback types.OrderUpdateCallback) error {
	m.callbackMu.Lock()
	m.orderUpdateCallbacks = append(m.orderUpdateCallbacks, callback)
	m.callbackMu.Unlock()

	// Subscribe to user data stream
	timestamp := time.Now().UnixMilli()
	requestID := fmt.Sprintf("userdata_%d_%d", timestamp, m.requestID.Add(1))

	params := map[string]interface{}{
		"timestamp": timestamp,
		"apiKey":    m.config.APIKey,
	}

	// Generate signature
	signature := m.generateSignature(params)
	params["signature"] = signature

	// Send subscription request
	_, err := m.sendRequest(ctx, "userDataStream.start", params, requestID)
	return err
}

// GetLatency returns current WebSocket connection latency
func (m *BinanceWSOrderManager) GetLatency() (time.Duration, error) {
	if !m.connected.Load() {
		return 0, fmt.Errorf("WebSocket not connected")
	}

	start := time.Now()
	requestID := fmt.Sprintf("ping_%d", start.UnixNano())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := m.sendRequest(ctx, "ping", map[string]interface{}{}, requestID)
	if err != nil {
		return 0, err
	}

	latency := time.Since(start)
	m.updateMetric(func(metrics *types.WebSocketMetrics) {
		metrics.LastLatency = latency
		// Update average latency
		if metrics.AverageLatency == 0 {
			metrics.AverageLatency = latency
		} else {
			metrics.AverageLatency = (metrics.AverageLatency + latency) / 2
		}
	})

	return latency, nil
}

// GetMetrics returns WebSocket performance metrics
func (m *BinanceWSOrderManager) GetMetrics() *types.WebSocketMetrics {
	m.metricsMu.RLock()
	defer m.metricsMu.RUnlock()

	metrics := m.metrics
	if m.connected.Load() {
		metrics.ConnectionUptime = time.Since(m.connectedAt)
	}
	
	return &metrics
}

// sendRequest sends a request and waits for response
func (m *BinanceWSOrderManager) sendRequest(ctx context.Context, method string, params map[string]interface{}, requestID string) (*WSOrderResponse, error) {
	request := WSOrderRequest{
		ID:     requestID,
		Method: method,
		Params: params,
	}

	// Create response channel
	respChan := make(chan *WSOrderResponse, 1)
	m.respMu.Lock()
	m.responses[requestID] = respChan
	m.respMu.Unlock()

	// Clean up on exit
	defer func() {
		m.respMu.Lock()
		delete(m.responses, requestID)
		m.respMu.Unlock()
	}()

	// Send request
	m.mu.Lock()
	err := m.conn.WriteJSON(request)
	m.mu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}

	m.updateMetric(func(metrics *types.WebSocketMetrics) {
		metrics.MessagesSent++
		metrics.OrdersSent++
	})

	// Wait for response
	timeout := m.config.MessageTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	select {
	case resp := <-respChan:
		if resp.Error != nil {
			return nil, fmt.Errorf("request error: %d - %s", resp.Error.Code, resp.Error.Msg)
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(timeout):
		return nil, fmt.Errorf("request timeout")
	}
}

// readHandler handles incoming WebSocket messages
func (m *BinanceWSOrderManager) readHandler() {
	for {
		select {
		case <-m.stopCh:
			return
		default:
			var resp WSOrderResponse
			if err := m.conn.ReadJSON(&resp); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					m.handleDisconnect()
				}
				return
			}

			m.updateMetric(func(metrics *types.WebSocketMetrics) {
				metrics.MessagesReceived++
			})

			// Handle response
			if resp.ID != "" {
				m.respMu.RLock()
				if ch, ok := m.responses[resp.ID]; ok {
					select {
					case ch <- &resp:
					default:
					}
				}
				m.respMu.RUnlock()
			} else {
				// Handle stream updates (order updates, etc.)
				m.handleStreamUpdate(&resp)
			}
		}
	}
}

// heartbeatHandler sends periodic pings
func (m *BinanceWSOrderManager) heartbeatHandler() {
	ticker := time.NewTicker(m.config.PingInterval)
	if m.config.PingInterval == 0 {
		ticker = time.NewTicker(30 * time.Second)
	}
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			m.GetLatency()
			cancel()
		}
	}
}

// handleDisconnect handles disconnection and reconnection
func (m *BinanceWSOrderManager) handleDisconnect() {
	m.connected.Store(false)
	m.updateMetric(func(metrics *types.WebSocketMetrics) {
		metrics.Connected = false
	})

	// Attempt reconnection if enabled
	if m.config.ReconnectInterval > 0 && !m.reconnecting.Load() {
		m.reconnecting.Store(true)
		go m.reconnectLoop()
	}
}

// reconnectLoop attempts to reconnect
func (m *BinanceWSOrderManager) reconnectLoop() {
	defer m.reconnecting.Store(false)
	
	attempts := 0
	maxAttempts := m.config.MaxReconnectAttempts
	if maxAttempts == 0 {
		maxAttempts = 10
	}

	for attempts < maxAttempts {
		select {
		case <-m.stopCh:
			return
		case <-time.After(m.config.ReconnectInterval):
			attempts++
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err := m.Connect(ctx)
			cancel()
			
			if err == nil {
				return
			}
		}
	}
}

// handleStreamUpdate handles order update streams
func (m *BinanceWSOrderManager) handleStreamUpdate(resp *WSOrderResponse) {
	// Parse as order update
	var order types.Order
	if err := json.Unmarshal(resp.Result, &order); err == nil {
		m.callbackMu.RLock()
		callbacks := m.orderUpdateCallbacks
		m.callbackMu.RUnlock()

		for _, callback := range callbacks {
			go callback(&order)
		}
	}
}

// generateSignature generates HMAC SHA256 signature
func (m *BinanceWSOrderManager) generateSignature(params map[string]interface{}) string {
	// Sort keys for consistent ordering
	keys := make([]string, 0, len(params))
	for k := range params {
		if k != "signature" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	
	// Create query string in sorted order
	query := ""
	for _, k := range keys {
		if query != "" {
			query += "&"
		}
		query += fmt.Sprintf("%s=%v", k, params[k])
	}

	// Generate HMAC SHA256
	h := hmac.New(sha256.New, []byte(m.config.SecretKey))
	h.Write([]byte(query))
	return hex.EncodeToString(h.Sum(nil))
}

// updateMetric safely updates metrics
func (m *BinanceWSOrderManager) updateMetric(update func(*types.WebSocketMetrics)) {
	m.metricsMu.Lock()
	defer m.metricsMu.Unlock()
	update(&m.metrics)
}