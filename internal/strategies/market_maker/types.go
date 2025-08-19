package marketmaker

import (
	"time"

	"github.com/mExOms/oms/pkg/types"
	"github.com/shopspring/decimal"
)

// MarketMakerConfig contains configuration for market making strategy
type MarketMakerConfig struct {
	// Symbol configuration
	Symbol        string          `json:"symbol"`
	Exchange      string          `json:"exchange"`
	Account       string          `json:"account"`
	
	// Spread configuration
	SpreadBps     decimal.Decimal `json:"spread_bps"`      // Spread in basis points
	MinSpreadBps  decimal.Decimal `json:"min_spread_bps"`  // Minimum spread
	MaxSpreadBps  decimal.Decimal `json:"max_spread_bps"`  // Maximum spread
	
	// Quote configuration
	QuoteSize     decimal.Decimal `json:"quote_size"`      // Size per quote
	QuoteLevels   int             `json:"quote_levels"`    // Number of price levels
	LevelSpacing  decimal.Decimal `json:"level_spacing"`   // Spacing between levels (bps)
	
	// Inventory management
	MaxInventory  decimal.Decimal `json:"max_inventory"`   // Maximum inventory
	TargetInventory decimal.Decimal `json:"target_inventory"` // Target inventory
	InventorySkew decimal.Decimal `json:"inventory_skew"`  // Skew factor
	
	// Risk management
	MaxPositionValue decimal.Decimal `json:"max_position_value"`
	StopLossPercent  decimal.Decimal `json:"stop_loss_percent"`
	MaxDailyLoss     decimal.Decimal `json:"max_daily_loss"`
	
	// Execution
	RefreshRate      time.Duration   `json:"refresh_rate"`
	UsePostOnly      bool            `json:"use_post_only"`
	EnableHedging    bool            `json:"enable_hedging"`
	HedgeRatio       decimal.Decimal `json:"hedge_ratio"`
	
	// Market conditions
	VolatilityWindow time.Duration   `json:"volatility_window"`
	MinVolatility    decimal.Decimal `json:"min_volatility"`
	MaxVolatility    decimal.Decimal `json:"max_volatility"`
}

// Quote represents a single quote (bid or ask)
type Quote struct {
	Side       types.OrderSide
	Price      decimal.Decimal
	Quantity   decimal.Decimal
	OrderID    string
	PlacedAt   time.Time
	LastUpdate time.Time
}

// QuoteLevel represents a price level with quotes
type QuoteLevel struct {
	Price      decimal.Decimal
	BidQuote   *Quote
	AskQuote   *Quote
	Spread     decimal.Decimal
	MidPrice   decimal.Decimal
}

// MarketState represents current market conditions
type MarketState struct {
	BidPrice       decimal.Decimal
	AskPrice       decimal.Decimal
	MidPrice       decimal.Decimal
	LastPrice      decimal.Decimal
	Volume24h      decimal.Decimal
	Volatility     decimal.Decimal
	OrderBookDepth decimal.Decimal
	Timestamp      time.Time
}

// InventoryState represents current inventory position
type InventoryState struct {
	Position        decimal.Decimal
	PositionValue   decimal.Decimal
	AveragePrice    decimal.Decimal
	UnrealizedPnL   decimal.Decimal
	RealizedPnL     decimal.Decimal
	TotalPnL        decimal.Decimal
	LastUpdate      time.Time
}

// MMMetrics represents market maker performance metrics
type MMMetrics struct {
	// Trading metrics
	TotalVolume      decimal.Decimal
	BuyVolume        decimal.Decimal
	SellVolume       decimal.Decimal
	NumTrades        int
	NumBuys          int
	NumSells         int
	
	// Spread metrics
	AverageSpread    decimal.Decimal
	SpreadCapture    decimal.Decimal
	TimeAtBestBid    time.Duration
	TimeAtBestAsk    time.Duration
	
	// PnL metrics
	GrossProfit      decimal.Decimal
	TradingFees      decimal.Decimal
	NetProfit        decimal.Decimal
	Sharpe           decimal.Decimal
	MaxDrawdown      decimal.Decimal
	
	// Risk metrics
	MaxPosition      decimal.Decimal
	AvgPosition      decimal.Decimal
	VolumeRatio      decimal.Decimal // Buy/Sell ratio
	
	// Execution metrics
	FillRate         float64
	MakerRatio       float64 // Maker fills vs taker fills
	CancelRate       float64
	
	// Time period
	StartTime        time.Time
	EndTime          time.Time
}

// SpreadCalculator calculates dynamic spreads
type SpreadCalculator interface {
	CalculateSpread(state *MarketState, inventory *InventoryState) decimal.Decimal
}

// InventoryManager manages inventory and position limits
type InventoryManager interface {
	GetTargetPosition() decimal.Decimal
	GetPositionLimit(side types.OrderSide) decimal.Decimal
	UpdatePosition(trade *types.Trade)
	GetSkewAdjustment() decimal.Decimal
}

// RiskManager manages risk for market making
type RiskManager interface {
	CheckOrderRisk(order *types.Order) error
	CheckPositionRisk(position decimal.Decimal) error
	GetMaxOrderSize(side types.OrderSide) decimal.Decimal
	ShouldStop() bool
}

// QuoteGenerator generates quotes based on market conditions
type QuoteGenerator interface {
	GenerateQuotes(market *MarketState, inventory *InventoryState) ([]*Quote, error)
	UpdateQuotes(quotes []*Quote, market *MarketState) ([]*Quote, error)
}

// OrderManager manages order lifecycle
type OrderManager interface {
	PlaceQuotes(quotes []*Quote) error
	CancelQuotes(quotes []*Quote) error
	CancelAllQuotes() error
	GetActiveQuotes() []*Quote
	UpdateOrderStatus(orderID string, status types.OrderStatus)
}

// PriceLevel represents an order book price level
type PriceLevel struct {
	Price    decimal.Decimal
	Quantity decimal.Decimal
	Orders   int
}

// OrderBookSnapshot represents a point-in-time order book state
type OrderBookSnapshot struct {
	Symbol    string
	Bids      []PriceLevel
	Asks      []PriceLevel
	Timestamp time.Time
}

// TradeFlow represents recent trade flow analysis
type TradeFlow struct {
	BuyVolume      decimal.Decimal
	SellVolume     decimal.Decimal
	BuyCount       int
	SellCount      int
	NetFlow        decimal.Decimal
	VolumeRatio    decimal.Decimal
	AverageSize    decimal.Decimal
	LargeTradeRatio decimal.Decimal
	Period         time.Duration
}

// Greeks represents option-like sensitivities (for advanced strategies)
type Greeks struct {
	Delta   decimal.Decimal // Position sensitivity to price
	Gamma   decimal.Decimal // Delta sensitivity to price
	Vega    decimal.Decimal // Sensitivity to volatility
	Theta   decimal.Decimal // Time decay
}