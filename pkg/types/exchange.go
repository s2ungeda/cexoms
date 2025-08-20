package types

import (
	"context"
	"time"
	
	"github.com/shopspring/decimal"
)

// Exchange defines the interface that all exchange connectors must implement
type Exchange interface {
	// Connection management
	Connect(ctx context.Context) error
	Disconnect() error
	IsConnected() bool
	
	// Order operations (WebSocket preferred, REST as fallback)
	CreateOrder(ctx context.Context, order *Order) (*Order, error)
	CancelOrder(ctx context.Context, symbol string, orderID string) error
	GetOrder(ctx context.Context, symbol string, orderID string) (*Order, error)
	GetOpenOrders(ctx context.Context, symbol string) ([]*Order, error)
	
	// Account operations
	GetBalance(ctx context.Context) (*Balance, error)
	GetPositions(ctx context.Context) ([]*Position, error)
	
	// Market data
	SubscribeMarketData(ctx context.Context, symbols []string) error
	UnsubscribeMarketData(ctx context.Context, symbols []string) error
	
	// Exchange info
	GetExchangeInfo() ExchangeInfo
	GetSymbolInfo(symbol string) (*SymbolInfo, error)
	
	// WebSocket order manager (returns nil if not supported)
	GetWebSocketOrderManager() WebSocketOrderManager
	
	// WebSocket capabilities
	GetWebSocketInfo() ExchangeWebSocketInfo
}


// Position represents a trading position
type Position struct {
	Exchange      string
	Symbol        string
	Side          Side
	Quantity      float64
	EntryPrice    float64
	MarkPrice     float64
	UnrealizedPNL float64
	UnrealizedPnL float64 // Alias for compatibility
	RealizedPNL   float64
	Margin        float64
	Leverage      float64
	UpdatedAt     time.Time
}

// Helper method for compatibility
func (p *Position) GetUnrealizedPnL() float64 {
	return p.UnrealizedPNL
}


// MarketData represents market data snapshot
type MarketData struct {
	Exchange     string
	Symbol       string
	BidPrice     float64
	BidQuantity  float64
	AskPrice     float64
	AskQuantity  float64
	LastPrice    float64
	Volume24h    float64
	Timestamp    time.Time
}

// SymbolInfo contains symbol trading rules
type SymbolInfo struct {
	Symbol              string
	BaseAsset           string
	QuoteAsset          string
	Status              string
	MinPrice            float64
	MaxPrice            float64
	TickSize            float64
	MinQuantity         float64
	MaxQuantity         float64
	StepSize            float64
	MinNotional         float64
	IsSpotTradingAllowed bool
	IsMarginTradingAllowed bool
}



// Enums
type Side string
type OrderType string
type OrderStatus string
type TimeInForce string

type ExchangeType string
const (
	ExchangeBinanceSpot    ExchangeType = "BINANCE_SPOT"
	ExchangeBinanceFutures ExchangeType = "BINANCE_FUTURES"
	ExchangeBybitSpot      ExchangeType = "BYBIT_SPOT"
	ExchangeBybitFutures   ExchangeType = "BYBIT_FUTURES"
	ExchangeOKXSpot        ExchangeType = "OKX_SPOT"
	ExchangeOKXFutures     ExchangeType = "OKX_FUTURES"
	ExchangeUpbit          ExchangeType = "UPBIT"
)

// PriceLevel represents a price and quantity pair in order book
type PriceLevel struct {
	Price    decimal.Decimal `json:"price"`
	Quantity decimal.Decimal `json:"quantity"`
}