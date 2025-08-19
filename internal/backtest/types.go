package backtest

import (
	"time"

	"github.com/mExOms/pkg/types"
)

// DataSource represents the source of market data
type DataSource string

const (
	DataSourceFile     DataSource = "file"
	DataSourceDatabase DataSource = "database"
	DataSourceAPI      DataSource = "api"
)

// BacktestConfig contains configuration for backtesting
type BacktestConfig struct {
	StartTime         time.Time              `json:"start_time"`
	EndTime           time.Time              `json:"end_time"`
	DataSource        DataSource             `json:"data_source"`
	DataPath          string                 `json:"data_path"`
	InitialCapital    float64                `json:"initial_capital"`
	Symbols           []string               `json:"symbols"`
	Exchanges         []string               `json:"exchanges"`
	TickInterval      time.Duration          `json:"tick_interval"`
	SpreadMultiplier  float64                `json:"spread_multiplier"`
	SlippageModel     SlippageModel          `json:"slippage_model"`
	FeeModel          FeeModel               `json:"fee_model"`
	LatencySimulation LatencySimulation      `json:"latency_simulation"`
	OutputPath        string                 `json:"output_path"`
}

// SlippageModel defines how slippage is calculated
type SlippageModel struct {
	Type       string  `json:"type"` // "fixed", "linear", "square_root"
	BaseRate   float64 `json:"base_rate"`
	ImpactRate float64 `json:"impact_rate"`
}

// FeeModel defines trading fees
type FeeModel struct {
	MakerFee float64            `json:"maker_fee"`
	TakerFee float64            `json:"taker_fee"`
	Custom   map[string]float64 `json:"custom"` // Exchange-specific fees
}

// LatencySimulation defines network latency simulation
type LatencySimulation struct {
	Enabled     bool          `json:"enabled"`
	BaseLatency time.Duration `json:"base_latency"`
	Jitter      time.Duration `json:"jitter"`
}

// MarketDataPoint represents a single point of market data
type MarketDataPoint struct {
	Timestamp time.Time
	Symbol    string
	Exchange  string
	Bid       float64
	Ask       float64
	BidSize   float64
	AskSize   float64
	Last      float64
	Volume    float64
}

// Trade represents a simulated trade
type Trade struct {
	ID              string
	Timestamp       time.Time
	Symbol          string
	Exchange        string
	Side            types.Side
	Price           float64
	Quantity        float64
	Fee             float64
	Slippage        float64
	ActualPrice     float64 // Price after slippage
	Value           float64 // Total value including fees
	PositionBefore  float64
	PositionAfter   float64
	BalanceBefore   float64
	BalanceAfter    float64
	Strategy        string
	OrderID         string
}

// Position represents a position during backtesting
type Position struct {
	Symbol    string
	Quantity  float64
	AvgPrice  float64
	Value     float64
	PnL       float64
	UpdatedAt time.Time
}

// BacktestResult contains the results of a backtest
type BacktestResult struct {
	Config           BacktestConfig
	StartTime        time.Time
	EndTime          time.Time
	Duration         time.Duration
	InitialCapital   float64
	FinalCapital     float64
	TotalReturn      float64
	TotalReturnPct   float64
	MaxDrawdown      float64
	MaxDrawdownPct   float64
	SharpeRatio      float64
	SortinoRatio     float64
	CalmarRatio      float64
	WinRate          float64
	ProfitFactor     float64
	TotalTrades      int
	WinningTrades    int
	LosingTrades     int
	TotalFees        float64
	TotalSlippage    float64
	AverageTrade     float64
	BestTrade        float64
	WorstTrade       float64
	MaxConsecutiveWins   int
	MaxConsecutiveLosses int
	DailyReturns     []DailyReturn
	Trades           []Trade
	EquityCurve      []EquityPoint
	Positions        map[string]Position
	StrategyMetrics  map[string]*StrategyMetrics
}

// DailyReturn represents returns for a single day
type DailyReturn struct {
	Date      time.Time
	Return    float64
	ReturnPct float64
	Equity    float64
	Trades    int
	Volume    float64
}

// EquityPoint represents a point on the equity curve
type EquityPoint struct {
	Timestamp time.Time
	Equity    float64
	Drawdown  float64
	Positions int
}

// StrategyMetrics contains metrics for a specific strategy
type StrategyMetrics struct {
	Strategy        string
	TotalTrades     int
	WinningTrades   int
	LosingTrades    int
	TotalPnL        float64
	AvgPnL          float64
	WinRate         float64
	ProfitFactor    float64
	SharpeRatio     float64
	MaxDrawdown     float64
	TotalFees       float64
	TotalSlippage   float64
}

// BacktestEngine interface defines the backtesting engine
type BacktestEngine interface {
	// Initialize the engine with config
	Initialize(config BacktestConfig) error
	
	// Run the backtest
	Run() (*BacktestResult, error)
	
	// Add a strategy to backtest
	AddStrategy(strategy Strategy) error
	
	// Set data provider
	SetDataProvider(provider DataProvider) error
	
	// Get current state
	GetState() *BacktestState
}

// Strategy interface for backtesting strategies
type Strategy interface {
	// Initialize the strategy
	Initialize(capital float64) error
	
	// Called on each market update
	OnMarketUpdate(data *MarketDataPoint, state *BacktestState) []types.Order
	
	// Called when an order is filled
	OnOrderFilled(trade *Trade)
	
	// Get strategy name
	GetName() string
	
	// Get strategy metrics
	GetMetrics() *StrategyMetrics
}

// DataProvider interface for providing market data
type DataProvider interface {
	// Initialize with config
	Initialize(config BacktestConfig) error
	
	// Get next data point
	Next() (*MarketDataPoint, error)
	
	// Check if more data is available
	HasNext() bool
	
	// Reset to beginning
	Reset() error
	
	// Get data for specific time
	GetDataAt(timestamp time.Time) ([]*MarketDataPoint, error)
}

// BacktestState represents the current state during backtesting
type BacktestState struct {
	CurrentTime     time.Time
	Cash            float64
	Equity          float64
	Positions       map[string]*Position
	OpenOrders      map[string]*types.Order
	CompletedTrades []Trade
	MarketData      map[string]*MarketDataPoint // symbol -> latest data
}

// OrderSimulator simulates order execution
type OrderSimulator interface {
	// Simulate order execution
	SimulateExecution(order *types.Order, marketData *MarketDataPoint, state *BacktestState) (*Trade, error)
	
	// Calculate slippage
	CalculateSlippage(order *types.Order, marketData *MarketDataPoint) float64
	
	// Calculate fees
	CalculateFees(order *types.Order, exchange string) float64
}

// PerformanceAnalyzer analyzes backtest performance
type PerformanceAnalyzer interface {
	// Analyze results
	Analyze(trades []Trade, equityCurve []EquityPoint) *BacktestResult
	
	// Calculate Sharpe ratio
	CalculateSharpeRatio(returns []float64, riskFreeRate float64) float64
	
	// Calculate maximum drawdown
	CalculateMaxDrawdown(equityCurve []EquityPoint) (float64, float64)
	
	// Generate report
	GenerateReport(result *BacktestResult, outputPath string) error
}