package types

import (
	"time"
	
	"github.com/shopspring/decimal"
)

// Order sides (position specific for futures)
const (
	OrderSideBuy  = "BUY"
	OrderSideSell = "SELL"
)

// Order types
const (
	OrderTypeMarket          = "MARKET"
	OrderTypeLimit           = "LIMIT"
	OrderTypeLimitMaker      = "LIMIT_MAKER"
	OrderTypeStop            = "STOP"
	OrderTypeStopLimit       = "STOP_LIMIT"
	OrderTypeStopLoss        = "STOP_LOSS"
	OrderTypeStopLossLimit   = "STOP_LOSS_LIMIT"
	OrderTypeTakeProfit      = "TAKE_PROFIT"
	OrderTypeTakeProfitLimit = "TAKE_PROFIT_LIMIT"
)

// Order status
const (
	OrderStatusNew             = "NEW"
	OrderStatusPartiallyFilled = "PARTIALLY_FILLED"
	OrderStatusFilled          = "FILLED"
	OrderStatusCanceled        = "CANCELED"
	OrderStatusRejected        = "REJECTED"
	OrderStatusExpired         = "EXPIRED"
)

// Time in force
const (
	TimeInForceGTC = "GTC" // Good Till Cancel
	TimeInForceIOC = "IOC" // Immediate or Cancel
	TimeInForceFOK = "FOK" // Fill or Kill
	TimeInForceGTX = "GTX" // Good Till Crossing
)

// Position sides for futures
const (
	PositionSideLong  = "LONG"
	PositionSideShort = "SHORT"
	PositionSideBoth  = "BOTH"
)

// Type aliases for compatibility
type OrderSide = string
type PositionSide = string
type OrderType = string
type OrderStatus = string
type TimeInForce = string
type MarketType = string
type Side = string

// Market types
const (
	MarketTypeSpot    MarketType = "spot"
	MarketTypeFutures MarketType = "futures"
	MarketTypeMargin  MarketType = "margin"
)

// Exchange types
type ExchangeType string

const (
	ExchangeBinance        ExchangeType = "binance"
	ExchangeBinanceSpot    ExchangeType = "binance-spot"
	ExchangeBinanceFutures ExchangeType = "binance-futures"
	ExchangeBybit          ExchangeType = "bybit"
	ExchangeBybitSpot      ExchangeType = "bybit-spot"
	ExchangeBybitFutures   ExchangeType = "bybit-futures"
	ExchangeOKX            ExchangeType = "okx"
	ExchangeOKXSpot        ExchangeType = "okx-spot"
	ExchangeOKXFutures     ExchangeType = "okx-futures"
	ExchangeUpbit          ExchangeType = "upbit"
)

// Kline intervals
type KlineInterval string

const (
	KlineInterval1m  KlineInterval = "1m"
	KlineInterval3m  KlineInterval = "3m"
	KlineInterval5m  KlineInterval = "5m"
	KlineInterval15m KlineInterval = "15m"
	KlineInterval30m KlineInterval = "30m"
	KlineInterval1h  KlineInterval = "1h"
	KlineInterval2h  KlineInterval = "2h"
	KlineInterval4h  KlineInterval = "4h"
	KlineInterval6h  KlineInterval = "6h"
	KlineInterval8h  KlineInterval = "8h"
	KlineInterval12h KlineInterval = "12h"
	KlineInterval1d  KlineInterval = "1d"
	KlineInterval3d  KlineInterval = "3d"
	KlineInterval1w  KlineInterval = "1w"
	KlineInterval1M  KlineInterval = "1M"
)

// Order represents a trading order
type Order struct {
	ID              string                 `json:"id"`
	ClientOrderID   string                 `json:"client_order_id,omitempty"`
	ExchangeOrderID string                 `json:"exchange_order_id,omitempty"`
	Symbol          string                 `json:"symbol"`
	Side            OrderSide              `json:"side"`
	Type            OrderType              `json:"type"`
	Status          OrderStatus            `json:"status,omitempty"`
	Price           decimal.Decimal        `json:"price,omitempty"`
	Quantity        decimal.Decimal        `json:"quantity"`
	StopPrice       decimal.Decimal        `json:"stop_price,omitempty"`
	TimeInForce     TimeInForce            `json:"time_in_force,omitempty"`
	ReduceOnly      bool                   `json:"reduce_only,omitempty"`
	ClosePosition   bool                   `json:"close_position,omitempty"`
	PositionSide    PositionSide           `json:"position_side,omitempty"`
	WorkingType     string                 `json:"working_type,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at,omitempty"`
	MarginType      string                 `json:"margin_type,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	ExecutedQty     decimal.Decimal        `json:"executed_qty,omitempty"`
	RemainingQty    decimal.Decimal        `json:"remaining_qty,omitempty"`
	AvgPrice        decimal.Decimal        `json:"avg_price,omitempty"`
	Fee             decimal.Decimal        `json:"fee,omitempty"`
	FeeCurrency     string                 `json:"fee_currency,omitempty"`
	FilledQuantity  decimal.Decimal        `json:"filled_quantity,omitempty"`
	PostOnly        bool                   `json:"post_only,omitempty"`
}

// OrderResponse represents the response after creating/updating an order
type OrderResponse struct {
	OrderID      string `json:"order_id"`
	ClientID     string `json:"client_id"`
	Symbol       string `json:"symbol"`
	Side         string `json:"side"`
	Type         string `json:"type"`
	Status       string `json:"status"`
	Price        string `json:"price"`
	Quantity     string `json:"quantity"`
	ExecutedQty  string `json:"executed_qty"`
	TimeInForce  string `json:"time_in_force,omitempty"`
	ReduceOnly   bool   `json:"reduce_only,omitempty"`
	PositionSide string `json:"position_side,omitempty"`
	TransactTime int64  `json:"transact_time"`
}

// Trade represents an executed trade
type Trade struct {
	TradeID       string          `json:"trade_id"`
	OrderID       string          `json:"order_id"`
	ClientOrderID string          `json:"client_order_id,omitempty"`
	Symbol        string          `json:"symbol"`
	Side          OrderSide       `json:"side"`
	Price         decimal.Decimal `json:"price"`
	Quantity      decimal.Decimal `json:"quantity"`
	Fee           decimal.Decimal `json:"fee,omitempty"`
	FeeCurrency   string          `json:"fee_currency,omitempty"`
	Time          time.Time       `json:"time"`
	IsMaker       bool            `json:"is_maker"`
	IsBuyer       bool            `json:"is_buyer"`
	FeeRate       decimal.Decimal `json:"fee_rate,omitempty"`
}

// Callback types for WebSocket streams
type OrderBookCallback func(symbol string, orderBook *OrderBook)
type TradeCallback func(symbol string, trade *Trade)
type TickerCallback func(symbol string, ticker *Ticker)

// OrderBook is an alias for OrderBookData for backward compatibility
type OrderBook = OrderBookData

// Kline is an alias for KlineData for backward compatibility
type Kline = KlineData

// Ticker represents 24hr ticker statistics
type Ticker struct {
	Symbol       string `json:"symbol"`
	Price        string `json:"price"`
	Volume       string `json:"volume"`
	QuoteVolume  string `json:"quote_volume"`
	BidPrice     string `json:"bid_price"`
	BidQty       string `json:"bid_qty"`
	AskPrice     string `json:"ask_price"`
	AskQty       string `json:"ask_qty"`
	High         string `json:"high"`
	Low          string `json:"low"`
	Open         string `json:"open"`
	PriceChange  string `json:"price_change"`
	PricePercent string `json:"price_percent"`
}

// Balance represents account balance for a single asset
type Balance struct {
	Asset         string          `json:"asset"`
	Free          decimal.Decimal `json:"free"`
	Locked        decimal.Decimal `json:"locked"`
	Total         decimal.Decimal `json:"total"`
	UnrealizedPnL decimal.Decimal `json:"unrealized_pnl,omitempty"`
}

// AccountInfo represents account information
type AccountInfo struct {
	Exchange    ExchangeType `json:"exchange"`
	AccountID   string       `json:"account_id"`
	AccountType string       `json:"account_type"`
	Balances    []Balance    `json:"balances"`
	UpdateTime  time.Time    `json:"update_time"`
}

// SymbolInfo represents trading symbol information
type SymbolInfo struct {
	Symbol                   string          `json:"symbol"`
	BaseAsset                string          `json:"base_asset"`
	QuoteAsset               string          `json:"quote_asset"`
	Status                   string          `json:"status"`
	MinQty                   decimal.Decimal `json:"min_qty"`
	MaxQty                   decimal.Decimal `json:"max_qty"`
	StepSize                 decimal.Decimal `json:"step_size"`
	MinNotional              decimal.Decimal `json:"min_notional"`
	TickSize                 decimal.Decimal `json:"tick_size"`
	BasePrecision            int             `json:"base_precision"`
	QuotePrecision           int             `json:"quote_precision"`
	MinLeverage              int             `json:"min_leverage,omitempty"`
	MaxLeverage              int             `json:"max_leverage,omitempty"`
	ContractType             string          `json:"contract_type,omitempty"`
	IsSpotTradingAllowed     bool            `json:"is_spot_trading_allowed"`
	IsMarginTradingAllowed   bool            `json:"is_margin_trading_allowed"`
	IsFuturesTradingAllowed  bool            `json:"is_futures_trading_allowed"`
}

// MarketData represents current market data
type MarketData struct {
	Symbol             string          `json:"symbol"`
	Price              decimal.Decimal `json:"price"`
	Bid                decimal.Decimal `json:"bid"`
	Ask                decimal.Decimal `json:"ask"`
	BidQty             decimal.Decimal `json:"bid_qty"`
	AskQty             decimal.Decimal `json:"ask_qty"`
	High24h            decimal.Decimal `json:"high_24h"`
	Low24h             decimal.Decimal `json:"low_24h"`
	Volume24h          decimal.Decimal `json:"volume_24h"`
	QuoteVolume24h     decimal.Decimal `json:"quote_volume_24h"`
	PriceChangePercent decimal.Decimal `json:"price_change_percent"`
	UpdateTime         time.Time       `json:"update_time"`
}

// OrderBook represents order book with price levels
type OrderBookData struct {
	Symbol     string       `json:"symbol"`
	Bids       []PriceLevel `json:"bids"`
	Asks       []PriceLevel `json:"asks"`
	UpdateTime time.Time    `json:"update_time"`
	UpdatedAt  time.Time    `json:"updated_at"` // Alias for UpdateTime
}

// PriceLevel represents a price level in order book
type PriceLevel struct {
	Price    decimal.Decimal `json:"price"`
	Quantity decimal.Decimal `json:"quantity"`
}

// KlineData represents candlestick data
type KlineData struct {
	OpenTime    time.Time       `json:"open_time"`
	Open        decimal.Decimal `json:"open"`
	High        decimal.Decimal `json:"high"`
	Low         decimal.Decimal `json:"low"`
	Close       decimal.Decimal `json:"close"`
	Volume      decimal.Decimal `json:"volume"`
	QuoteVolume decimal.Decimal `json:"quote_volume"`
	CloseTime   time.Time       `json:"close_time"`
	Trades      int             `json:"trades"`
}

// ExchangeInfo represents exchange trading rules
type ExchangeInfo struct {
	Exchange      string         `json:"exchange"`
	Market        string         `json:"market"`
	Symbols       []Symbol       `json:"symbols"`
	Name          string         `json:"name"`
	Type          ExchangeType   `json:"type"`
	TestNet       bool           `json:"testnet"`
	RateLimits    RateLimits     `json:"rate_limits"`
	SupportedAPIs []string       `json:"supported_apis"`
}

// Symbol represents trading pair information
type Symbol struct {
	Symbol      string `json:"symbol"`
	Base        string `json:"base"`
	Quote       string `json:"quote"`
	MinQty      string `json:"min_qty"`
	MaxQty      string `json:"max_qty"`
	StepSize    string `json:"step_size"`
	MinNotional string `json:"min_notional"`
	Status      string `json:"status"`
}

// RateLimits defines exchange rate limits
type RateLimits struct {
	WeightPerMinute int `json:"weight_per_minute"`
	OrdersPerSecond int `json:"orders_per_second"`
	OrdersPerDay    int `json:"orders_per_day"`
}