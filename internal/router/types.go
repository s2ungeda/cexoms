package router

import (
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// RouteRequest represents a request to route an order
type RouteRequest struct {
	Symbol          string                 `json:"symbol"`
	Side            types.OrderSide        `json:"side"`
	Quantity        decimal.Decimal        `json:"quantity"`
	OrderType       types.OrderType        `json:"order_type"`
	Price           decimal.Decimal        `json:"price,omitempty"`           // For limit orders
	TimeInForce     types.TimeInForce      `json:"time_in_force,omitempty"`
	MaxSlippage     decimal.Decimal        `json:"max_slippage,omitempty"`    // Maximum acceptable slippage
	Urgency         Urgency                `json:"urgency"`                    // How quickly to execute
	PreferredVenues []string               `json:"preferred_venues,omitempty"` // Preferred exchanges
	AvoidVenues     []string               `json:"avoid_venues,omitempty"`     // Exchanges to avoid
	Strategy        RoutingStrategy        `json:"strategy"`                   // Routing strategy
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// RouteResponse represents the routing decision
type RouteResponse struct {
	RequestID       string           `json:"request_id"`
	Routes          []Route          `json:"routes"`
	TotalQuantity   decimal.Decimal  `json:"total_quantity"`
	EstimatedPrice  decimal.Decimal  `json:"estimated_price"`
	EstimatedFees   decimal.Decimal  `json:"estimated_fees"`
	EstimatedTime   time.Duration    `json:"estimated_time"`
	Confidence      float64          `json:"confidence"` // 0.0 to 1.0
	Warnings        []string         `json:"warnings,omitempty"`
}

// Route represents a single routing path
type Route struct {
	Venue           string                 `json:"venue"`           // Exchange name
	Account         string                 `json:"account"`         // Account to use
	Market          string                 `json:"market"`          // spot/futures
	Symbol          string                 `json:"symbol"`          // Symbol on this venue
	Quantity        decimal.Decimal        `json:"quantity"`        // Quantity for this route
	OrderType       types.OrderType        `json:"order_type"`
	Price           decimal.Decimal        `json:"price,omitempty"`
	EstimatedPrice  decimal.Decimal        `json:"estimated_price"`
	EstimatedFee    decimal.Decimal        `json:"estimated_fee"`
	Priority        int                    `json:"priority"`        // Execution priority
	SplitRatio      decimal.Decimal        `json:"split_ratio"`     // Percentage of total order
	Metadata        map[string]interface{} `json:"metadata,omitempty"` // Additional metadata
}

// Urgency defines how quickly an order should be executed
type Urgency string

const (
	UrgencyLow       Urgency = "low"        // Can wait for better prices
	UrgencyNormal    Urgency = "normal"     // Standard execution
	UrgencyHigh      Urgency = "high"       // Execute quickly
	UrgencyImmediate Urgency = "immediate"  // Execute ASAP
)

// RoutingStrategy defines the routing strategy
type RoutingStrategy string

const (
	StrategyBestPrice      RoutingStrategy = "best_price"       // Optimize for best price
	StrategyLowestFee      RoutingStrategy = "lowest_fee"       // Minimize fees
	StrategyFastest        RoutingStrategy = "fastest"          // Fastest execution
	StrategyMinSlippage    RoutingStrategy = "min_slippage"     // Minimize market impact
	StrategyBalanced       RoutingStrategy = "balanced"         // Balance all factors
	StrategyVWAP           RoutingStrategy = "vwap"             // Match VWAP
	StrategyTWAP           RoutingStrategy = "twap"             // Time-weighted average
	StrategyIceberg        RoutingStrategy = "iceberg"          // Hide large orders
)

// VenueInfo contains information about a trading venue
type VenueInfo struct {
	Name            string                          `json:"name"`
	Exchange        string                          `json:"exchange"`
	Market          string                          `json:"market"`
	Account         string                          `json:"account"`
	Available       bool                            `json:"available"`
	TradingFees     TradingFees                     `json:"trading_fees"`
	Limits          TradingLimits                   `json:"limits"`
	SupportedOrders []types.OrderType               `json:"supported_orders"`
	LastUpdate      time.Time                       `json:"last_update"`
	Metadata        map[string]interface{}          `json:"metadata"`
}

// TradingFees contains fee information
type TradingFees struct {
	MakerFee        decimal.Decimal `json:"maker_fee"`        // As percentage (0.001 = 0.1%)
	TakerFee        decimal.Decimal `json:"taker_fee"`
	FeeAsset        string          `json:"fee_asset"`        // Asset used for fees
	TierLevel       int             `json:"tier_level"`
	Volume30d       decimal.Decimal `json:"volume_30d"`       // 30-day volume for tier
}

// TradingLimits contains trading limits
type TradingLimits struct {
	MinOrderSize    decimal.Decimal `json:"min_order_size"`
	MaxOrderSize    decimal.Decimal `json:"max_order_size"`
	MinNotional     decimal.Decimal `json:"min_notional"`     // Minimum order value
	MaxDailyVolume  decimal.Decimal `json:"max_daily_volume"`
	RateLimitPerMin int             `json:"rate_limit_per_min"`
}

// MarketConditions represents current market conditions
type MarketConditions struct {
	Symbol          string                    `json:"symbol"`
	Timestamp       time.Time                 `json:"timestamp"`
	Volatility      float64                   `json:"volatility"`      // 24h volatility
	Spread          decimal.Decimal           `json:"spread"`          // Bid-ask spread
	Liquidity       LiquidityInfo             `json:"liquidity"`
	TrendDirection  string                    `json:"trend_direction"` // "up", "down", "sideways"
	Volume24h       decimal.Decimal           `json:"volume_24h"`
	OrderBooks      map[string]*types.OrderBook `json:"order_books"`     // venue -> order book
}

// LiquidityInfo contains liquidity information
type LiquidityInfo struct {
	BidLiquidity    []LiquidityLevel `json:"bid_liquidity"`
	AskLiquidity    []LiquidityLevel `json:"ask_liquidity"`
	TotalBidVolume  decimal.Decimal  `json:"total_bid_volume"`
	TotalAskVolume  decimal.Decimal  `json:"total_ask_volume"`
	ImbalanceRatio  decimal.Decimal  `json:"imbalance_ratio"`  // Ask/Bid ratio
}

// LiquidityLevel represents liquidity at a price level
type LiquidityLevel struct {
	Price           decimal.Decimal `json:"price"`
	Volume          decimal.Decimal `json:"volume"`
	CumulativeVolume decimal.Decimal `json:"cumulative_volume"`
	Venues          []string        `json:"venues"` // Which venues provide this liquidity
}

// ExecutionReport represents the result of order execution
type ExecutionReport struct {
	RequestID       string                `json:"request_id"`
	Status          ExecutionStatus       `json:"status"`
	ExecutedRoutes  []ExecutedRoute       `json:"executed_routes"`
	TotalExecuted   decimal.Decimal       `json:"total_executed"`
	AveragePrice    decimal.Decimal       `json:"average_price"`
	TotalFees       decimal.Decimal       `json:"total_fees"`
	SlippageBps     int                   `json:"slippage_bps"`     // Basis points
	ExecutionTime   time.Duration         `json:"execution_time"`
	Timestamp       time.Time             `json:"timestamp"`
	Errors          []string              `json:"errors,omitempty"`
}

// ExecutedRoute represents an executed route
type ExecutedRoute struct {
	Venue           string          `json:"venue"`
	OrderID         string          `json:"order_id"`
	Quantity        decimal.Decimal `json:"quantity"`
	ExecutedQty     decimal.Decimal `json:"executed_qty"`
	Price           decimal.Decimal `json:"price"`
	Fee             decimal.Decimal `json:"fee"`
	Status          string          `json:"status"`
	Timestamp       time.Time       `json:"timestamp"`
}

// ExecutionStatus represents the execution status
type ExecutionStatus string

const (
	ExecutionPending    ExecutionStatus = "pending"
	ExecutionInProgress ExecutionStatus = "in_progress"
	ExecutionCompleted  ExecutionStatus = "completed"
	ExecutionPartial    ExecutionStatus = "partial"
	ExecutionFailed     ExecutionStatus = "failed"
	ExecutionCancelled  ExecutionStatus = "cancelled"
)

// RoutingConfig contains router configuration
type RoutingConfig struct {
	MaxVenues           int             `json:"max_venues"`            // Maximum venues to split across
	MinSplitSize        decimal.Decimal `json:"min_split_size"`        // Minimum size for splitting
	MaxSlippageBps      int             `json:"max_slippage_bps"`      // Maximum slippage in basis points
	SmartRoutingEnabled bool            `json:"smart_routing_enabled"`
	FeeOptimization     bool            `json:"fee_optimization"`
	LiquidityThreshold  decimal.Decimal `json:"liquidity_threshold"`   // Minimum liquidity required
	RefreshInterval     time.Duration   `json:"refresh_interval"`      // Market data refresh interval
	ExecutionTimeout    time.Duration   `json:"execution_timeout"`
	RetryAttempts       int             `json:"retry_attempts"`
}

// PerformanceMetrics tracks router performance
type PerformanceMetrics struct {
	TotalOrders         int64           `json:"total_orders"`
	SuccessfulOrders    int64           `json:"successful_orders"`
	FailedOrders        int64           `json:"failed_orders"`
	AverageSlippageBps  float64         `json:"average_slippage_bps"`
	TotalVolume         decimal.Decimal `json:"total_volume"`
	TotalFeesSaved      decimal.Decimal `json:"total_fees_saved"`
	AverageExecutionTime time.Duration  `json:"average_execution_time"`
	VenueDistribution   map[string]int64 `json:"venue_distribution"`
	StrategyPerformance map[string]*StrategyMetrics `json:"strategy_performance"`
}

// StrategyMetrics tracks performance by strategy
type StrategyMetrics struct {
	OrderCount         int64           `json:"order_count"`
	SuccessRate        float64         `json:"success_rate"`
	AverageSlippage    float64         `json:"average_slippage"`
	AverageExecutionTime time.Duration `json:"average_execution_time"`
}

// SimulationRequest represents a request to simulate routing
type SimulationRequest struct {
	RouteRequest    RouteRequest     `json:"route_request"`
	MarketScenario  string           `json:"market_scenario"`  // "normal", "volatile", "illiquid"
	IncludeCosts    bool             `json:"include_costs"`
}

// SimulationResult represents simulation results
type SimulationResult struct {
	Routes          []Route         `json:"routes"`
	ExpectedPrice   decimal.Decimal `json:"expected_price"`
	ExpectedFees    decimal.Decimal `json:"expected_fees"`
	ExpectedSlippage decimal.Decimal `json:"expected_slippage"`
	ExecutionRisk   float64         `json:"execution_risk"`   // 0.0 to 1.0
	Recommendations []string        `json:"recommendations"`
}