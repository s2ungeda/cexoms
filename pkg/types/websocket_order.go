package types

import (
	"context"
	"time"
)

// WebSocketOrderManager defines the interface for WebSocket-based order operations
// All exchanges that support WebSocket orders must implement this interface
type WebSocketOrderManager interface {
	// Connect establishes WebSocket connection
	Connect(ctx context.Context) error
	
	// Disconnect closes WebSocket connection
	Disconnect() error
	
	// IsConnected returns connection status
	IsConnected() bool
	
	// CreateOrder creates an order via WebSocket
	CreateOrder(ctx context.Context, order *Order) (*OrderResponse, error)
	
	// CancelOrder cancels an order via WebSocket
	CancelOrder(ctx context.Context, symbol string, orderID string) error
	
	// ModifyOrder modifies an existing order via WebSocket (if supported)
	ModifyOrder(ctx context.Context, symbol string, orderID string, newPrice, newQuantity string) error
	
	// GetOrderStatus gets order status via WebSocket
	GetOrderStatus(ctx context.Context, symbol string, orderID string) (*Order, error)
	
	// GetOpenOrders gets all open orders via WebSocket
	GetOpenOrders(ctx context.Context, symbol string) ([]*Order, error)
	
	// SubscribeOrderUpdates subscribes to order update events
	SubscribeOrderUpdates(ctx context.Context, callback OrderUpdateCallback) error
	
	// GetLatency returns current WebSocket connection latency
	GetLatency() (time.Duration, error)
	
	// GetMetrics returns WebSocket performance metrics
	GetMetrics() *WebSocketMetrics
}

// OrderUpdateCallback is called when order status changes
type OrderUpdateCallback func(order *Order)

// WebSocketMetrics contains WebSocket performance metrics
type WebSocketMetrics struct {
	Connected          bool
	ConnectionUptime   time.Duration
	MessagesSent       int64
	MessagesReceived   int64
	OrdersSent         int64
	OrdersSuccessful   int64
	OrdersFailed       int64
	AverageLatency     time.Duration
	LastLatency        time.Duration
	ReconnectCount     int
}

// WebSocketConfig contains WebSocket connection configuration
type WebSocketConfig struct {
	// Connection settings
	URL                string
	APIKey             string
	SecretKey          string
	
	// Performance settings
	PingInterval       time.Duration
	PongTimeout        time.Duration
	ReconnectInterval  time.Duration
	MaxReconnectAttempts int
	
	// Message settings
	MessageTimeout     time.Duration
	MaxMessageSize     int64
	
	// Features
	EnableCompression  bool
	EnableHeartbeat    bool
}

// WebSocketOrderSupport indicates exchange WebSocket capabilities
type WebSocketOrderSupport struct {
	CreateOrder    bool
	CancelOrder    bool
	ModifyOrder    bool
	OrderStatus    bool
	OpenOrders     bool
	OrderUpdates   bool
}

// ExchangeWebSocketInfo provides WebSocket capabilities info
type ExchangeWebSocketInfo struct {
	Supported      bool
	OrderSupport   WebSocketOrderSupport
	BaseURL        string
	TestnetURL     string
	Documentation  string
}