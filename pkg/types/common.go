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
	OrderTypeStop            = "STOP"
	OrderTypeStopLimit       = "STOP_LIMIT"
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

// Order represents a trading order
type Order struct {
	ID           string          `json:"id"`
	Symbol       string          `json:"symbol"`
	Side         string          `json:"side"`
	Type         string          `json:"type"`
	Price        decimal.Decimal `json:"price,omitempty"`
	Quantity     decimal.Decimal `json:"quantity"`
	StopPrice    decimal.Decimal `json:"stop_price,omitempty"`
	TimeInForce  string          `json:"time_in_force,omitempty"`
	ReduceOnly   bool            `json:"reduce_only,omitempty"`
	ClosePosition bool           `json:"close_position,omitempty"`
	PositionSide string          `json:"position_side,omitempty"`
	WorkingType  string          `json:"working_type,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
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
	ID           string `json:"id"`
	Symbol       string `json:"symbol"`
	Price        string `json:"price"`
	Quantity     string `json:"quantity"`
	QuoteQty     string `json:"quote_qty,omitempty"`
	Time         int64  `json:"time"`
	IsBuyerMaker bool   `json:"is_buyer_maker"`
}

// OrderBook represents market depth
type OrderBook struct {
	Symbol       string       `json:"symbol"`
	LastUpdateID int64        `json:"last_update_id"`
	Bids         []PriceLevel `json:"bids"`
	Asks         []PriceLevel `json:"asks"`
}

// Kline represents candlestick data
type Kline struct {
	Symbol    string `json:"symbol"`
	Interval  string `json:"interval"`
	OpenTime  int64  `json:"open_time"`
	CloseTime int64  `json:"close_time"`
	Open      string `json:"open"`
	High      string `json:"high"`
	Low       string `json:"low"`
	Close     string `json:"close"`
	Volume    string `json:"volume"`
	IsFinal   bool   `json:"is_final"`
}

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

// Balance represents account balance
type Balance struct {
	Exchange string                    `json:"exchange"`
	Market   string                    `json:"market"`
	Assets   map[string]AssetBalance   `json:"assets"`
}

// AssetBalance represents balance for a single asset
type AssetBalance struct {
	Asset  string `json:"asset"`
	Free   string `json:"free"`
	Locked string `json:"locked"`
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