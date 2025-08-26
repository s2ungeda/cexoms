package arbitrage

import (
	"time"

	"github.com/shopspring/decimal"
)

// Opportunity represents an arbitrage opportunity
type Opportunity struct {
	ID              string
	Type            OpportunityType
	Symbol          string
	BuyExchange     string
	SellExchange    string
	BuyPrice        decimal.Decimal
	SellPrice       decimal.Decimal
	Spread          decimal.Decimal
	SpreadPercent   decimal.Decimal
	MaxQuantity     decimal.Decimal
	EstimatedProfit decimal.Decimal
	FeesEstimate    decimal.Decimal
	NetProfit       decimal.Decimal
	Timestamp       time.Time
	ExpiresAt       time.Time
	Confidence      float64
}

// OpportunityType represents the type of arbitrage opportunity
type OpportunityType string

const (
	OpportunityTypeSimple      OpportunityType = "simple"      // Buy low, sell high
	OpportunityTypeTriangular  OpportunityType = "triangular"  // Three-way arbitrage
	OpportunityTypeCrossPair   OpportunityType = "cross_pair"  // Cross currency pairs
)

// MarketData represents market data from an exchange
type MarketData struct {
	Exchange    string
	Symbol      string
	BidPrice    decimal.Decimal
	BidQuantity decimal.Decimal
	AskPrice    decimal.Decimal
	AskQuantity decimal.Decimal
	Timestamp   time.Time
}

// ExchangeFees represents trading fees for an exchange
type ExchangeFees struct {
	Exchange       string
	TakerFee       decimal.Decimal
	MakerFee       decimal.Decimal
	WithdrawFee    map[string]decimal.Decimal // Asset -> Fee
	MinWithdrawAmt map[string]decimal.Decimal // Asset -> Min amount
}

// ArbitrageConfig contains arbitrage engine configuration
type ArbitrageConfig struct {
	// Opportunity detection
	MinSpreadPercent   decimal.Decimal       // Minimum spread to consider (e.g., 0.001 = 0.1%)
	MinProfitUSD       decimal.Decimal       // Minimum profit in USD
	MaxPositionUSD     decimal.Decimal       // Maximum position size
	
	// Risk management
	MaxOpenPositions   int                   // Maximum concurrent arbitrage positions
	MaxExposureUSD     decimal.Decimal       // Maximum total exposure
	RequiredConfidence float64               // Minimum confidence score (0-1)
	
	// Execution
	ExecutionTimeout   time.Duration         // Order execution timeout
	OrderType          string                // "market" or "limit"
	AggressiveMode     bool                  // Use market orders for speed
	
	// Monitoring
	ScanInterval       time.Duration         // Market scan interval
	DataStaleTimeout   time.Duration         // Max age for market data
	
	// Exchange settings
	EnabledExchanges   []string              // Exchanges to monitor
	ExchangePairs      map[string][]string   // Exchange -> enabled pairs
	FeeOverrides       map[string]ExchangeFees // Custom fee settings
}

// ExecutionPlan represents the execution plan for an arbitrage opportunity
type ExecutionPlan struct {
	OpportunityID   string
	Steps           []ExecutionStep
	TotalVolume     decimal.Decimal
	ExpectedProfit  decimal.Decimal
	RiskScore       float64
	CreatedAt       time.Time
}

// ExecutionStep represents a single step in the execution plan
type ExecutionStep struct {
	Order          int                    // Execution order
	Exchange       string
	AccountID      string
	Symbol         string
	Side           string                 // "buy" or "sell"
	Quantity       decimal.Decimal
	Price          decimal.Decimal        // Expected price
	OrderType      string                 // "market" or "limit"
	EstimatedCost  decimal.Decimal
	Dependencies   []int                  // Steps that must complete first
}

// ArbitrageResult represents the result of an arbitrage execution
type ArbitrageResult struct {
	OpportunityID   string
	Status          ExecutionStatus
	ExecutedSteps   []ExecutedStep
	TotalCost       decimal.Decimal
	TotalRevenue    decimal.Decimal
	ActualProfit    decimal.Decimal
	ExecutionTime   time.Duration
	CompletedAt     time.Time
	ErrorMessage    string
}

// ExecutedStep represents an executed step
type ExecutedStep struct {
	StepOrder      int
	OrderID        string
	Exchange       string
	Symbol         string
	Side           string
	ExecutedQty    decimal.Decimal
	ExecutedPrice  decimal.Decimal
	Fees           decimal.Decimal
	Status         string
}

// ExecutionStatus represents the status of arbitrage execution
type ExecutionStatus string

const (
	ExecutionStatusPending    ExecutionStatus = "pending"
	ExecutionStatusExecuting  ExecutionStatus = "executing"
	ExecutionStatusCompleted  ExecutionStatus = "completed"
	ExecutionStatusFailed     ExecutionStatus = "failed"
	ExecutionStatusCancelled  ExecutionStatus = "cancelled"
)

// TriangularOpportunity represents a triangular arbitrage opportunity
type TriangularOpportunity struct {
	ID              string
	Exchange        string
	Path            []string               // e.g., ["BTC", "ETH", "USDT", "BTC"]
	Pairs           []string               // e.g., ["ETHBTC", "ETHUSDT", "BTCUSDT"]
	Sides           []string               // e.g., ["buy", "sell", "buy"]
	Prices          []decimal.Decimal
	Quantities      []decimal.Decimal
	StartAmount     decimal.Decimal
	EndAmount       decimal.Decimal
	ProfitPercent   decimal.Decimal
	EstimatedTime   time.Duration
	Timestamp       time.Time
}

// Statistics represents arbitrage statistics
type Statistics struct {
	TotalOpportunities   int64
	ExecutedTrades       int64
	SuccessfulTrades     int64
	FailedTrades         int64
	TotalProfitUSD       decimal.Decimal
	TotalVolumeUSD       decimal.Decimal
	AverageProfitPercent decimal.Decimal
	BestTrade            *ArbitrageResult
	UpdatedAt            time.Time
}