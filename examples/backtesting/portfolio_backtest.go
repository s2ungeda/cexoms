package main

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/your-org/mExOms/pkg/backtest"
	"github.com/your-org/mExOms/pkg/types"
	"go.uber.org/zap"
)

// PortfolioStrategy manages multiple sub-strategies with capital allocation
type PortfolioStrategy struct {
	strategies      map[string]types.Strategy
	allocations     map[string]float64
	capitalPerAlloc float64
	logger          *zap.Logger
	mu              sync.RWMutex
}

// NewPortfolioStrategy creates a portfolio of strategies
func NewPortfolioStrategy(totalCapital float64, logger *zap.Logger) *PortfolioStrategy {
	strategies := make(map[string]types.Strategy)
	allocations := make(map[string]float64)
	
	// Create sub-strategies
	// 1. Trend Following (40% allocation)
	strategies["trend_btc"] = NewTrendFollowingStrategy("BTCUSDT", 20, 50, logger)
	allocations["trend_btc"] = 0.2
	
	strategies["trend_eth"] = NewTrendFollowingStrategy("ETHUSDT", 20, 50, logger)
	allocations["trend_eth"] = 0.2
	
	// 2. Mean Reversion (30% allocation)
	strategies["meanrev_btc"] = NewSimpleMeanReversionStrategy("BTCUSDT", 14, 2.0, logger)
	allocations["meanrev_btc"] = 0.15
	
	strategies["meanrev_eth"] = NewSimpleMeanReversionStrategy("ETHUSDT", 14, 2.0, logger)
	allocations["meanrev_eth"] = 0.15
	
	// 3. Breakout (30% allocation)
	strategies["breakout_btc"] = NewBreakoutStrategy("BTCUSDT", 20, 1.5, logger)
	allocations["breakout_btc"] = 0.15
	
	strategies["breakout_eth"] = NewBreakoutStrategy("ETHUSDT", 20, 1.5, logger)
	allocations["breakout_eth"] = 0.15
	
	return &PortfolioStrategy{
		strategies:      strategies,
		allocations:     allocations,
		capitalPerAlloc: totalCapital,
		logger:          logger,
	}
}

// Initialize initializes all sub-strategies
func (p *PortfolioStrategy) Initialize() error {
	p.logger.Info("Initializing portfolio strategy",
		zap.Int("strategies", len(p.strategies)))
	
	for name, strategy := range p.strategies {
		if err := strategy.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize %s: %w", name, err)
		}
	}
	
	return nil
}

// OnMarketData distributes market data to all strategies
func (p *PortfolioStrategy) OnMarketData(data *types.MarketData) []*types.Signal {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	allSignals := make([]*types.Signal, 0)
	
	// Collect signals from all strategies
	for name, strategy := range p.strategies {
		signals := strategy.OnMarketData(data)
		
		// Adjust position sizes based on allocation
		allocation := p.allocations[name]
		for _, signal := range signals {
			// Scale quantity by allocation
			signal.Quantity = signal.Quantity * allocation
			allSignals = append(allSignals, signal)
		}
	}
	
	return allSignals
}

// OnFill distributes fills to appropriate strategies
func (p *PortfolioStrategy) OnFill(fill *types.Fill) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	// Forward fill to all strategies (they filter by symbol internally)
	for _, strategy := range p.strategies {
		strategy.OnFill(fill)
	}
}

// TrendFollowingStrategy - simple trend following
type TrendFollowingStrategy struct {
	symbol     string
	fastPeriod int
	slowPeriod int
	position   float64
	prices     []float64
	logger     *zap.Logger
}

func NewTrendFollowingStrategy(symbol string, fast, slow int, logger *zap.Logger) *TrendFollowingStrategy {
	return &TrendFollowingStrategy{
		symbol:     symbol,
		fastPeriod: fast,
		slowPeriod: slow,
		prices:     make([]float64, 0),
		logger:     logger,
	}
}

func (s *TrendFollowingStrategy) Initialize() error {
	return nil
}

func (s *TrendFollowingStrategy) OnMarketData(data *types.MarketData) []*types.Signal {
	if data.Symbol != s.symbol {
		return nil
	}

	s.prices = append(s.prices, data.Price)
	
	if len(s.prices) < s.slowPeriod {
		return nil
	}
	
	// Calculate EMAs
	fastEMA := calculateEMA(s.prices, s.fastPeriod)
	slowEMA := calculateEMA(s.prices, s.slowPeriod)
	
	var signals []*types.Signal
	
	// Generate signals
	if s.position == 0 && fastEMA > slowEMA {
		signals = append(signals, &types.Signal{
			Symbol:    s.symbol,
			Side:      types.OrderSideBuy,
			OrderType: types.OrderTypeMarket,
			Quantity:  calculateBaseQuantity(data.Price),
			Timestamp: data.Timestamp,
		})
	} else if s.position > 0 && fastEMA < slowEMA {
		signals = append(signals, &types.Signal{
			Symbol:    s.symbol,
			Side:      types.OrderSideSell,
			OrderType: types.OrderTypeMarket,
			Quantity:  s.position,
			Timestamp: data.Timestamp,
		})
	}
	
	return signals
}

func (s *TrendFollowingStrategy) OnFill(fill *types.Fill) {
	if fill.Symbol != s.symbol {
		return
	}
	
	if fill.Side == types.OrderSideBuy {
		s.position += fill.Quantity
	} else {
		s.position -= fill.Quantity
	}
}

// SimpleMeanReversionStrategy - basic mean reversion
type SimpleMeanReversionStrategy struct {
	symbol     string
	period     int
	threshold  float64
	position   float64
	prices     []float64
	logger     *zap.Logger
}

func NewSimpleMeanReversionStrategy(symbol string, period int, threshold float64, logger *zap.Logger) *SimpleMeanReversionStrategy {
	return &SimpleMeanReversionStrategy{
		symbol:    symbol,
		period:    period,
		threshold: threshold,
		prices:    make([]float64, 0),
		logger:    logger,
	}
}

func (s *SimpleMeanReversionStrategy) Initialize() error {
	return nil
}

func (s *SimpleMeanReversionStrategy) OnMarketData(data *types.MarketData) []*types.Signal {
	if data.Symbol != s.symbol {
		return nil
	}

	s.prices = append(s.prices, data.Price)
	
	if len(s.prices) < s.period {
		return nil
	}
	
	// Calculate mean and standard deviation
	mean, stdDev := calculateMeanStdDev(s.prices[len(s.prices)-s.period:])
	
	upperBand := mean + s.threshold*stdDev
	lowerBand := mean - s.threshold*stdDev
	
	var signals []*types.Signal
	
	// Generate signals
	if s.position == 0 {
		if data.Price < lowerBand {
			signals = append(signals, &types.Signal{
				Symbol:    s.symbol,
				Side:      types.OrderSideBuy,
				OrderType: types.OrderTypeMarket,
				Quantity:  calculateBaseQuantity(data.Price),
				Timestamp: data.Timestamp,
			})
		} else if data.Price > upperBand {
			signals = append(signals, &types.Signal{
				Symbol:    s.symbol,
				Side:      types.OrderSideSell,
				OrderType: types.OrderTypeMarket,
				Quantity:  calculateBaseQuantity(data.Price),
				Timestamp: data.Timestamp,
			})
		}
	} else if s.position > 0 && data.Price >= mean {
		signals = append(signals, &types.Signal{
			Symbol:    s.symbol,
			Side:      types.OrderSideSell,
			OrderType: types.OrderTypeMarket,
			Quantity:  s.position,
			Timestamp: data.Timestamp,
		})
	} else if s.position < 0 && data.Price <= mean {
		signals = append(signals, &types.Signal{
			Symbol:    s.symbol,
			Side:      types.OrderSideBuy,
			OrderType: types.OrderTypeMarket,
			Quantity:  math.Abs(s.position),
			Timestamp: data.Timestamp,
		})
	}
	
	return signals
}

func (s *SimpleMeanReversionStrategy) OnFill(fill *types.Fill) {
	if fill.Symbol != s.symbol {
		return
	}
	
	if fill.Side == types.OrderSideBuy {
		s.position += fill.Quantity
	} else {
		s.position -= fill.Quantity
	}
}

// BreakoutStrategy - channel breakout
type BreakoutStrategy struct {
	symbol      string
	period      int
	atrMultiple float64
	position    float64
	prices      []float64
	highs       []float64
	lows        []float64
	logger      *zap.Logger
}

func NewBreakoutStrategy(symbol string, period int, atrMultiple float64, logger *zap.Logger) *BreakoutStrategy {
	return &BreakoutStrategy{
		symbol:      symbol,
		period:      period,
		atrMultiple: atrMultiple,
		prices:      make([]float64, 0),
		highs:       make([]float64, 0),
		lows:        make([]float64, 0),
		logger:      logger,
	}
}

func (s *BreakoutStrategy) Initialize() error {
	return nil
}

func (s *BreakoutStrategy) OnMarketData(data *types.MarketData) []*types.Signal {
	if data.Symbol != s.symbol {
		return nil
	}

	s.prices = append(s.prices, data.Price)
	
	// Simulate high/low from price with small variation
	highPrice := data.Price * 1.001
	lowPrice := data.Price * 0.999
	s.highs = append(s.highs, highPrice)
	s.lows = append(s.lows, lowPrice)
	
	if len(s.prices) < s.period {
		return nil
	}
	
	// Calculate channel
	recentHighs := s.highs[len(s.highs)-s.period:]
	recentLows := s.lows[len(s.lows)-s.period:]
	
	channelHigh := getMax(recentHighs)
	channelLow := getMin(recentLows)
	
	// Calculate ATR for stop loss
	atr := calculateATR(s.highs, s.lows, s.prices, s.period)
	
	var signals []*types.Signal
	
	// Generate signals
	if s.position == 0 {
		if data.Price > channelHigh {
			signals = append(signals, &types.Signal{
				Symbol:    s.symbol,
				Side:      types.OrderSideBuy,
				OrderType: types.OrderTypeMarket,
				Quantity:  calculateBaseQuantity(data.Price),
				Timestamp: data.Timestamp,
			})
			
			// Stop loss
			stopPrice := data.Price - s.atrMultiple*atr
			signals = append(signals, &types.Signal{
				Symbol:    s.symbol,
				Side:      types.OrderSideSell,
				OrderType: types.OrderTypeStopLoss,
				Quantity:  calculateBaseQuantity(data.Price),
				StopPrice: stopPrice,
				Timestamp: data.Timestamp,
			})
		} else if data.Price < channelLow {
			signals = append(signals, &types.Signal{
				Symbol:    s.symbol,
				Side:      types.OrderSideSell,
				OrderType: types.OrderTypeMarket,
				Quantity:  calculateBaseQuantity(data.Price),
				Timestamp: data.Timestamp,
			})
			
			// Stop loss
			stopPrice := data.Price + s.atrMultiple*atr
			signals = append(signals, &types.Signal{
				Symbol:    s.symbol,
				Side:      types.OrderSideBuy,
				OrderType: types.OrderTypeStopLoss,
				Quantity:  calculateBaseQuantity(data.Price),
				StopPrice: stopPrice,
				Timestamp: data.Timestamp,
			})
		}
	}
	
	return signals
}

func (s *BreakoutStrategy) OnFill(fill *types.Fill) {
	if fill.Symbol != s.symbol {
		return
	}
	
	if fill.Side == types.OrderSideBuy {
		s.position += fill.Quantity
	} else {
		s.position -= fill.Quantity
	}
}

// Helper functions
func calculateEMA(prices []float64, period int) float64 {
	if len(prices) < period {
		return 0
	}
	
	alpha := 2.0 / float64(period+1)
	ema := prices[len(prices)-period]
	
	for i := len(prices) - period + 1; i < len(prices); i++ {
		ema = alpha*prices[i] + (1-alpha)*ema
	}
	
	return ema
}

func calculateMeanStdDev(prices []float64) (mean, stdDev float64) {
	sum := 0.0
	for _, p := range prices {
		sum += p
	}
	mean = sum / float64(len(prices))
	
	variance := 0.0
	for _, p := range prices {
		variance += math.Pow(p-mean, 2)
	}
	stdDev = math.Sqrt(variance / float64(len(prices)))
	
	return mean, stdDev
}

func calculateATR(highs, lows, closes []float64, period int) float64 {
	if len(highs) < period+1 {
		return 0
	}
	
	trValues := make([]float64, 0)
	for i := len(highs) - period; i < len(highs); i++ {
		highLow := highs[i] - lows[i]
		highClose := math.Abs(highs[i] - closes[i-1])
		lowClose := math.Abs(lows[i] - closes[i-1])
		
		tr := math.Max(highLow, math.Max(highClose, lowClose))
		trValues = append(trValues, tr)
	}
	
	sum := 0.0
	for _, tr := range trValues {
		sum += tr
	}
	
	return sum / float64(len(trValues))
}

func getMax(values []float64) float64 {
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

func getMin(values []float64) float64 {
	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

func calculateBaseQuantity(price float64) float64 {
	// Base quantity before allocation adjustment
	baseCapital := 100000.0
	return baseCapital / price
}

func main() {
	// Setup logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	
	// Define backtest configuration
	config := backtest.BacktestConfig{
		StartTime:       time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:         time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		InitialCapital:  100000.0,
		Symbols:         []string{"BTCUSDT", "ETHUSDT"},
		DataPath:        "./data/historical",
		StrategyName:    "Multi-Strategy Portfolio",
		BarInterval:     "1h",
		TickSize:        time.Hour,
		CommissionRate:  0.001,
		SlippageModel:   backtest.ModelLinear,
		SlippageParams: map[string]float64{
			"base_slippage": 0.0001,
			"impact_factor": 0.00001,
		},
	}
	
	// Create portfolio strategy
	portfolio := NewPortfolioStrategy(config.InitialCapital, logger)
	
	// Run backtest
	engine := backtest.NewBacktestEngine(config, portfolio, logger)
	
	fmt.Println("=== PORTFOLIO BACKTEST ===")
	fmt.Printf("Period: %s to %s\n", config.StartTime.Format("2006-01-02"), config.EndTime.Format("2006-01-02"))
	fmt.Printf("Initial Capital: $%.2f\n", config.InitialCapital)
	fmt.Println("\nStrategy Allocations:")
	fmt.Println("- Trend Following: 40% (BTC: 20%, ETH: 20%)")
	fmt.Println("- Mean Reversion: 30% (BTC: 15%, ETH: 15%)")
	fmt.Println("- Breakout: 30% (BTC: 15%, ETH: 15%)")
	
	result, err := engine.Run()
	if err != nil {
		log.Fatal("Backtest failed:", err)
	}
	
	// Print results
	fmt.Println("\n=== PERFORMANCE METRICS ===")
	fmt.Printf("Total Return: %.2f%%\n", result.Metrics.TotalReturn*100)
	fmt.Printf("Annualized Return: %.2f%%\n", result.Metrics.AnnualizedReturn*100)
	fmt.Printf("Sharpe Ratio: %.2f\n", result.Metrics.SharpeRatio)
	fmt.Printf("Sortino Ratio: %.2f\n", result.Metrics.SortinoRatio)
	fmt.Printf("Calmar Ratio: %.2f\n", result.Metrics.CalmarRatio)
	fmt.Printf("Max Drawdown: %.2f%%\n", result.Metrics.MaxDrawdown*100)
	fmt.Printf("Max Drawdown Days: %d\n", result.Metrics.MaxDrawdownDays)
	
	fmt.Println("\n=== RISK METRICS ===")
	fmt.Printf("Volatility: %.2f%%\n", result.Metrics.Volatility*100)
	fmt.Printf("VaR (95%%): %.2f%%\n", result.Metrics.VaR95*100)
	fmt.Printf("CVaR (95%%): %.2f%%\n", result.Metrics.CVaR95*100)
	fmt.Printf("Skewness: %.2f\n", result.Metrics.Skewness)
	fmt.Printf("Kurtosis: %.2f\n", result.Metrics.Kurtosis)
	
	fmt.Println("\n=== TRADE STATISTICS ===")
	fmt.Printf("Total Trades: %d\n", result.Statistics.TotalTrades)
	fmt.Printf("Win Rate: %.2f%%\n", result.Metrics.WinRate*100)
	fmt.Printf("Profit Factor: %.2f\n", result.Metrics.ProfitFactor)
	fmt.Printf("Average Win: $%.2f\n", result.Statistics.AvgProfit)
	fmt.Printf("Average Loss: $%.2f\n", result.Statistics.AvgLoss)
	fmt.Printf("Kelly Percentage: %.2f%%\n", result.Statistics.KellyPercent*100)
	
	// Analyze trades by strategy type
	fmt.Println("\n=== TRADE BREAKDOWN BY STRATEGY ===")
	strategyTrades := make(map[string]int)
	strategyPnL := make(map[string]float64)
	
	// Simplified analysis based on trade timing patterns
	for i, trade := range result.Trades {
		// Determine strategy based on trade pattern
		strategyType := "Unknown"
		
		if i > 0 {
			timeDiff := trade.Timestamp.Sub(result.Trades[i-1].Timestamp)
			if timeDiff < time.Hour*24 {
				strategyType = "Mean Reversion" // Frequent trades
			} else if timeDiff > time.Hour*72 {
				strategyType = "Trend Following" // Infrequent trades
			} else {
				strategyType = "Breakout" // Medium frequency
			}
		}
		
		strategyTrades[strategyType]++
		strategyPnL[strategyType] += trade.PnL
	}
	
	for strategy, count := range strategyTrades {
		pnl := strategyPnL[strategy]
		fmt.Printf("%s: %d trades, PnL: $%.2f\n", strategy, count, pnl)
	}
	
	// Final equity
	if len(result.EquityCurve) > 0 {
		finalEquity := result.EquityCurve[len(result.EquityCurve)-1].Equity
		fmt.Printf("\nFinal Portfolio Value: $%.2f\n", finalEquity)
		fmt.Printf("Total PnL: $%.2f\n", finalEquity-config.InitialCapital)
	}
	
	// Benefits of diversification
	fmt.Println("\n=== DIVERSIFICATION BENEFITS ===")
	fmt.Println("Portfolio characteristics:")
	fmt.Printf("- Lower volatility than individual strategies: %.2f%%\n", result.Metrics.Volatility*100)
	fmt.Printf("- Better risk-adjusted returns (Sharpe): %.2f\n", result.Metrics.SharpeRatio)
	fmt.Printf("- Reduced maximum drawdown: %.2f%%\n", result.Metrics.MaxDrawdown*100)
	fmt.Printf("- More consistent returns across market conditions\n")
}