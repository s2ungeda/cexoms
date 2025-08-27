package backtest

import (
	"time"
	
	"github.com/your-org/mExOms/pkg/types"
)

// BacktestResult contains the results of a backtest
type BacktestResult struct {
	StartTime      time.Time
	EndTime        time.Time
	InitialCapital float64
	FinalCapital   float64
	TotalReturn    float64
	
	Metrics        *PerformanceMetrics
	TradeStats     *TradeStatistics
	Trades         []*Trade
	EquityCurve    []EquityPoint
	DrawdownCurve  []DrawdownPoint
	
	Config         BacktestConfig
}

// PerformanceMetrics contains performance metrics
type PerformanceMetrics struct {
	TotalReturn       float64
	AnnualizedReturn  float64
	SharpeRatio       float64
	SortinoRatio      float64
	MaxDrawdown       float64
	MaxDrawdownDays   int
	CalmarRatio       float64
	WinRate           float64
	ProfitFactor      float64
	ExpectancyRatio   float64
	VaR95             float64
	CVaR95            float64
	Beta              float64
	Alpha             float64
	Volatility        float64
	Skewness          float64
	Kurtosis          float64
}

// TradeStatistics contains trade statistics
type TradeStatistics struct {
	TotalTrades      int
	WinningTrades    int
	LosingTrades     int
	WinRate          float64
	AvgTrade         float64
	AvgWin           float64
	AvgLoss          float64
	LargestWin       float64
	LargestLoss      float64
	ProfitFactor     float64
	TotalPnL         float64
	TotalCommission  float64
	TotalSlippage    float64
	AvgHoldingTime   time.Duration
	MaxConsecWins    int
	MaxConsecLosses  int
}

// EquityPoint represents a point on the equity curve
type EquityPoint struct {
	Timestamp time.Time
	Equity    float64
	DrawDown  float64
}

// DrawdownPoint represents a drawdown period
type DrawdownPoint struct {
	StartTime    time.Time
	EndTime      time.Time
	StartEquity  float64
	MinEquity    float64
	Drawdown     float64
	Duration     time.Duration
	Recovered    bool
}

// TickData represents historical tick data
type TickData struct {
	Symbol    string
	Timestamp time.Time
	Price     float64
	Volume    float64
	BidPrice  float64
	AskPrice  float64
	BidSize   float64
	AskSize   float64
}

// OrderBookData represents historical order book snapshot
type OrderBookData struct {
	Symbol    string
	Timestamp time.Time
	Bids      []PriceLevel
	Asks      []PriceLevel
}

// PriceLevel represents a price level in the order book
type PriceLevel struct {
	Price    float64
	Quantity float64
}

// DataProvider interface for loading historical data
type DataProvider interface {
	LoadTickData(symbol string, start, end time.Time) ([]*TickData, error)
	LoadOrderBookData(symbol string, start, end time.Time) ([]*OrderBookData, error)
	LoadKlineData(symbol string, interval string, start, end time.Time) ([]*types.Kline, error)
}

// SlippageModel interface for calculating slippage
type SlippageModel interface {
	Calculate(order *Order, marketPrice float64, orderBook *SimulatedOrderBook) float64
}

// Position represents a trading position
type Position struct {
	Symbol        string
	Side          types.OrderSide
	Quantity      float64
	EntryPrice    float64
	CurrentPrice  float64
	RealizedPnL   float64
	UnrealizedPnL float64
	Commission    float64
}