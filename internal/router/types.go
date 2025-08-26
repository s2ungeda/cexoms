package router

import (
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// RoutingRequest represents a request to route an order
type RoutingRequest struct {
	ID           string                 `json:"id"`
	Symbol       string                 `json:"symbol"`
	Side         string                 `json:"side"`
	Quantity     decimal.Decimal        `json:"quantity"`
	OrderType    string                 `json:"order_type"`
	Price        decimal.Decimal        `json:"price,omitempty"`
	TimeInForce  string                 `json:"time_in_force,omitempty"`
	Strategy     string                 `json:"strategy,omitempty"`
	AccountID    string                 `json:"account_id,omitempty"` // Optional: specify account
	MaxSlippage  decimal.Decimal        `json:"max_slippage,omitempty"`
	MinExecution decimal.Decimal        `json:"min_execution,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// RoutingResult represents the result of order routing
type RoutingResult struct {
	RequestID        string           `json:"request_id"`
	Symbol           string           `json:"symbol"`
	TotalQuantity    decimal.Decimal  `json:"total_quantity"`
	ExecutedQuantity decimal.Decimal  `json:"executed_quantity"`
	AveragePrice     decimal.Decimal  `json:"average_price"`
	TotalFees        decimal.Decimal  `json:"total_fees"`
	Routes           []*ExecutedRoute `json:"routes"`
	StartTime        time.Time        `json:"start_time"`
	EndTime          time.Time        `json:"end_time"`
	Success          bool             `json:"success"`
	Errors           []error          `json:"errors,omitempty"`
}

// ExecutedRoute represents an executed order route
type ExecutedRoute struct {
	Exchange         string          `json:"exchange"`
	AccountID        string          `json:"account_id"`
	OrderID          string          `json:"order_id"`
	Price            decimal.Decimal `json:"price"`
	ExecutedQuantity decimal.Decimal `json:"executed_quantity"`
	Fees             decimal.Decimal `json:"fees"`
	Status           string          `json:"status"`
	ExecutedAt       time.Time       `json:"executed_at"`
}

// Route represents a potential routing option
type Route struct {
	Exchange           string          `json:"exchange"`
	AccountID          string          `json:"account_id"`
	Price              decimal.Decimal `json:"price"`
	AvailableLiquidity decimal.Decimal `json:"available_liquidity"`
	EstimatedFees      decimal.Decimal `json:"estimated_fees"`
	RateLimitWeight    int             `json:"rate_limit_weight"`
	Score              decimal.Decimal `json:"score"`
	Priority           int             `json:"priority"`
}

// AccountRoute represents an account available for routing
type AccountRoute struct {
	AccountID   string          `json:"account_id"`
	Exchange    string          `json:"exchange"`
	Available   bool            `json:"available"`
	RateLimit   int             `json:"rate_limit"`
	Balance     decimal.Decimal `json:"balance"`
	Permissions []string        `json:"permissions"`
}

// MarketData represents aggregated market data
type MarketData struct {
	Exchange    string          `json:"exchange"`
	Symbol      string          `json:"symbol"`
	BidPrice    decimal.Decimal `json:"bid_price"`
	AskPrice    decimal.Decimal `json:"ask_price"`
	BidVolume   decimal.Decimal `json:"bid_volume"`
	AskVolume   decimal.Decimal `json:"ask_volume"`
	Spread      decimal.Decimal `json:"spread"`
	SpreadBps   decimal.Decimal `json:"spread_bps"`
	Volume24h   decimal.Decimal `json:"volume_24h"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// ArbitrageOpportunity represents a cross-exchange arbitrage opportunity
type ArbitrageOpportunity struct {
	ID              string                 `json:"id"`
	Symbol          string                 `json:"symbol"`
	BuyExchange     string                 `json:"buy_exchange"`
	BuyAccount      string                 `json:"buy_account"`
	SellExchange    string                 `json:"sell_exchange"`
	SellAccount     string                 `json:"sell_account"`
	BuyPrice        decimal.Decimal        `json:"buy_price"`
	SellPrice       decimal.Decimal        `json:"sell_price"`
	ProfitPercent   decimal.Decimal        `json:"profit_percent"`
	ProfitAmount    decimal.Decimal        `json:"profit_amount"`
	MaxQuantity     decimal.Decimal        `json:"max_quantity"`
	RequiredCapital decimal.Decimal        `json:"required_capital"`
	EstimatedFees   decimal.Decimal        `json:"estimated_fees"`
	NetProfit       decimal.Decimal        `json:"net_profit"`
	Confidence      decimal.Decimal        `json:"confidence"`
	ExpiresAt       time.Time              `json:"expires_at"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// RoutingMetrics tracks routing performance
type RoutingMetrics struct {
	TotalOrders      int64           `json:"total_orders"`
	SuccessfulOrders int64           `json:"successful_orders"`
	FailedOrders     int64           `json:"failed_orders"`
	TotalVolume      decimal.Decimal `json:"total_volume"`
	TotalFees        decimal.Decimal `json:"total_fees"`
	AverageSlippage  decimal.Decimal `json:"average_slippage"`
	BestRoute        string          `json:"best_route"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// FeeStructure represents exchange fee structure
type FeeStructure struct {
	Exchange     string          `json:"exchange"`
	MakerFee     decimal.Decimal `json:"maker_fee"`
	TakerFee     decimal.Decimal `json:"taker_fee"`
	TierLevel    int             `json:"tier_level"`
	VolumeUSD30d decimal.Decimal `json:"volume_usd_30d"`
}

// LiquiditySnapshot represents liquidity at a point in time
type LiquiditySnapshot struct {
	Exchange      string          `json:"exchange"`
	Symbol        string          `json:"symbol"`
	BidLiquidity  []PriceLevel    `json:"bid_liquidity"`
	AskLiquidity  []PriceLevel    `json:"ask_liquidity"`
	TotalBidValue decimal.Decimal `json:"total_bid_value"`
	TotalAskValue decimal.Decimal `json:"total_ask_value"`
	Timestamp     time.Time       `json:"timestamp"`
}

// PriceLevel represents a price level in the order book
type PriceLevel struct {
	Price    decimal.Decimal `json:"price"`
	Quantity decimal.Decimal `json:"quantity"`
	Value    decimal.Decimal `json:"value"`
}

// RoutingStrategy defines different routing strategies
type RoutingStrategy string

const (
	RoutingStrategyBestPrice     RoutingStrategy = "best_price"
	RoutingStrategyBestLiquidity RoutingStrategy = "best_liquidity"
	RoutingStrategyLowestFee     RoutingStrategy = "lowest_fee"
	RoutingStrategyFastest       RoutingStrategy = "fastest"
	RoutingStrategyBalanced      RoutingStrategy = "balanced"
	RoutingStrategyArbitrage     RoutingStrategy = "arbitrage"
)

// AccountSelection criteria for selecting accounts
type AccountSelection struct {
	PreferredAccounts []string        `json:"preferred_accounts"`
	ExcludedAccounts  []string        `json:"excluded_accounts"`
	MinBalance        decimal.Decimal `json:"min_balance"`
	MaxPositionSize   decimal.Decimal `json:"max_position_size"`
	RequirePermission string          `json:"require_permission"`
}