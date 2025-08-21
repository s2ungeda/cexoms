package types

import (
	"context"
	"time"
)

// Exchange defines the interface that all exchange connectors must implement
type Exchange interface {
	// Basic info
	GetName() string
	GetType() ExchangeType
	GetMarketType() MarketType
	
	// Initialization
	Initialize(ctx context.Context) error
	
	// Account operations
	GetAccountInfo(ctx context.Context) (*AccountInfo, error)
	GetBalances(ctx context.Context) ([]Balance, error)
	
	// Order operations
	PlaceOrder(ctx context.Context, order *Order) (*Order, error)
	CancelOrder(ctx context.Context, symbol string, orderID string) error
	GetOrder(ctx context.Context, symbol string, orderID string) (*Order, error)
	GetOpenOrders(ctx context.Context, symbol string) ([]*Order, error)
	GetOrderHistory(ctx context.Context, symbol string, limit int) ([]*Order, error)
	
	// Trade operations
	GetTrades(ctx context.Context, symbol string, limit int) ([]*Trade, error)
	
	// Market data
	GetSymbolInfo(ctx context.Context, symbol string) (*SymbolInfo, error)
	GetMarketData(ctx context.Context, symbols []string) (map[string]*MarketData, error)
	GetOrderBook(ctx context.Context, symbol string, depth int) (*OrderBook, error)
	GetKlines(ctx context.Context, symbol string, interval KlineInterval, limit int) ([]*Kline, error)
	
	// WebSocket operations (optional - implement if supported)
	SubscribeOrderBook(symbol string, callback OrderBookCallback) error
	SubscribeTrades(symbol string, callback TradeCallback) error
	SubscribeTicker(symbol string, callback TickerCallback) error
	UnsubscribeAll() error
}

// FuturesExchange extends Exchange with futures-specific functionality
type FuturesExchange interface {
	Exchange
	
	// Position operations
	GetPositions(ctx context.Context) ([]*Position, error)
	GetPosition(ctx context.Context, symbol string) (*Position, error)
	
	// Futures-specific operations
	SetLeverage(ctx context.Context, symbol string, leverage int) error
	SetMarginMode(ctx context.Context, symbol string, marginMode MarginMode) error
	GetFundingRate(ctx context.Context, symbol string) (*FundingRate, error)
}

// ExchangeWebSocketInfo represents WebSocket capabilities
type ExchangeWebSocketInfo struct {
	SupportsOrderManagement bool
	SupportsAccountUpdates  bool
	SupportsMarketData      bool
	SupportsPositionUpdates bool
	MaxConnections          int
	PingInterval            time.Duration
}