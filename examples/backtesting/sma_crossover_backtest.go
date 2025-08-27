package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/your-org/mExOms/pkg/backtest"
	"github.com/your-org/mExOms/pkg/types"
	"go.uber.org/zap"
)

// SMAStrategy implements a simple moving average crossover strategy
type SMAStrategy struct {
	fastPeriod   int
	slowPeriod   int
	symbol       string
	position     float64
	prices       []float64
	fastSMA      []float64
	slowSMA      []float64
	logger       *zap.Logger
}

// NewSMAStrategy creates a new SMA crossover strategy
func NewSMAStrategy(symbol string, fastPeriod, slowPeriod int, logger *zap.Logger) *SMAStrategy {
	return &SMAStrategy{
		fastPeriod: fastPeriod,
		slowPeriod: slowPeriod,
		symbol:     symbol,
		prices:     make([]float64, 0),
		fastSMA:    make([]float64, 0),
		slowSMA:    make([]float64, 0),
		logger:     logger,
	}
}

// Initialize initializes the strategy
func (s *SMAStrategy) Initialize() error {
	s.logger.Info("Initializing SMA strategy",
		zap.String("symbol", s.symbol),
		zap.Int("fast_period", s.fastPeriod),
		zap.Int("slow_period", s.slowPeriod))
	return nil
}

// OnMarketData processes market data and generates signals
func (s *SMAStrategy) OnMarketData(data *types.MarketData) []*types.Signal {
	if data.Symbol != s.symbol {
		return nil
	}

	// Add price to history
	s.prices = append(s.prices, data.Price)
	
	// Calculate SMAs
	if len(s.prices) >= s.fastPeriod {
		fastSMA := s.calculateSMA(s.prices, s.fastPeriod)
		s.fastSMA = append(s.fastSMA, fastSMA)
	}
	
	if len(s.prices) >= s.slowPeriod {
		slowSMA := s.calculateSMA(s.prices, s.slowPeriod)
		s.slowSMA = append(s.slowSMA, slowSMA)
	}
	
	// Generate signals after we have enough data
	if len(s.fastSMA) < 2 || len(s.slowSMA) < 2 {
		return nil
	}
	
	// Get current and previous SMA values
	currFast := s.fastSMA[len(s.fastSMA)-1]
	prevFast := s.fastSMA[len(s.fastSMA)-2]
	currSlow := s.slowSMA[len(s.slowSMA)-1]
	prevSlow := s.slowSMA[len(s.slowSMA)-2]
	
	var signals []*types.Signal
	
	// Check for crossover
	if prevFast <= prevSlow && currFast > currSlow && s.position == 0 {
		// Golden cross - buy signal
		signal := &types.Signal{
			Symbol:    s.symbol,
			Side:      types.OrderSideBuy,
			OrderType: types.OrderTypeMarket,
			Quantity:  s.calculatePositionSize(data.Price),
			Timestamp: data.Timestamp,
		}
		signals = append(signals, signal)
		
		s.logger.Debug("Buy signal generated",
			zap.String("symbol", s.symbol),
			zap.Float64("price", data.Price),
			zap.Float64("fast_sma", currFast),
			zap.Float64("slow_sma", currSlow))
			
	} else if prevFast >= prevSlow && currFast < currSlow && s.position > 0 {
		// Death cross - sell signal
		signal := &types.Signal{
			Symbol:    s.symbol,
			Side:      types.OrderSideSell,
			OrderType: types.OrderTypeMarket,
			Quantity:  s.position,
			Timestamp: data.Timestamp,
		}
		signals = append(signals, signal)
		
		s.logger.Debug("Sell signal generated",
			zap.String("symbol", s.symbol),
			zap.Float64("price", data.Price),
			zap.Float64("fast_sma", currFast),
			zap.Float64("slow_sma", currSlow))
	}
	
	return signals
}

// OnFill processes fill events
func (s *SMAStrategy) OnFill(fill *types.Fill) {
	if fill.Symbol != s.symbol {
		return
	}
	
	if fill.Side == types.OrderSideBuy {
		s.position += fill.Quantity
	} else {
		s.position -= fill.Quantity
	}
	
	s.logger.Info("Fill processed",
		zap.String("symbol", fill.Symbol),
		zap.String("side", string(fill.Side)),
		zap.Float64("quantity", fill.Quantity),
		zap.Float64("price", fill.Price),
		zap.Float64("position", s.position))
}

// calculateSMA calculates simple moving average
func (s *SMAStrategy) calculateSMA(prices []float64, period int) float64 {
	if len(prices) < period {
		return 0
	}
	
	sum := 0.0
	start := len(prices) - period
	for i := start; i < len(prices); i++ {
		sum += prices[i]
	}
	
	return sum / float64(period)
}

// calculatePositionSize calculates position size (simplified)
func (s *SMAStrategy) calculatePositionSize(price float64) float64 {
	// Use 95% of available capital
	// In real implementation, this would check available capital
	capitalPerTrade := 95000.0
	return capitalPerTrade / price
}

// SMAStrategyFactory creates SMA strategies with given parameters
type SMAStrategyFactory struct {
	symbol     string
	fastPeriod int
	slowPeriod int
	logger     *zap.Logger
}

// CreateStrategy creates a new strategy instance
func (f *SMAStrategyFactory) CreateStrategy(params map[string]float64) types.Strategy {
	fastPeriod := f.fastPeriod
	slowPeriod := f.slowPeriod
	
	if val, exists := params["fast_period"]; exists {
		fastPeriod = int(val)
	}
	if val, exists := params["slow_period"]; exists {
		slowPeriod = int(val)
	}
	
	return NewSMAStrategy(f.symbol, fastPeriod, slowPeriod, f.logger)
}

func main() {
	var optimize bool
	flag.BoolVar(&optimize, "optimize", false, "Run parameter optimization")
	flag.Parse()
	
	// Setup logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	
	// Define backtest configuration
	config := backtest.BacktestConfig{
		StartTime:       time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:         time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		InitialCapital:  100000.0,
		Symbols:         []string{"BTCUSDT"},
		DataPath:        "./data/historical",
		StrategyName:    "SMA Crossover",
		BarInterval:     "1h",
		TickSize:        time.Hour,
		CommissionRate:  0.001,
		CommissionMin:   0.0,
		SlippageModel:   backtest.ModelLinear,
		SlippageParams: map[string]float64{
			"base_slippage": 0.0001,
			"impact_factor": 0.00001,
		},
		MaxPositionSize: 10.0,
	}
	
	symbol := "BTCUSDT"
	
	if optimize {
		// Run optimization
		logger.Info("Running parameter optimization")
		
		optimConfig := backtest.OptimizationConfig{
			Method: backtest.MethodGridSearch,
			Parameters: []backtest.ParameterRange{
				{
					Name: "fast_period",
					Min:  5,
					Max:  20,
					Step: 5,
					Type: backtest.TypeInt,
				},
				{
					Name: "slow_period",
					Min:  20,
					Max:  50,
					Step: 10,
					Type: backtest.TypeInt,
				},
			},
			Objective:      backtest.ObjectiveSharpeRatio,
			MinTrades:      10,
			MaxDrawdown:    0.3,
			MinSharpeRatio: 0.5,
			NumWorkers:     4,
		}
		
		factory := &SMAStrategyFactory{
			symbol:     symbol,
			fastPeriod: 10,
			slowPeriod: 30,
			logger:     logger,
		}
		
		optimizer := backtest.NewOptimizer(optimConfig, config, factory, logger)
		
		result, err := optimizer.Run()
		if err != nil {
			log.Fatal("Optimization failed:", err)
		}
		
		// Print optimization results
		fmt.Println("\n=== OPTIMIZATION RESULTS ===")
		fmt.Printf("Best Parameters: %+v\n", result.BestParameters)
		fmt.Printf("Best Score (Sharpe Ratio): %.4f\n", result.BestScore)
		fmt.Printf("Total Iterations: %d\n", result.TotalIterations)
		fmt.Printf("Duration: %s\n", result.Duration)
		
		if result.InSampleMetrics != nil {
			fmt.Println("\nIn-Sample Metrics:")
			printMetrics(result.InSampleMetrics)
		}
		
		// Show top 5 parameter sets
		fmt.Println("\nTop 5 Parameter Sets:")
		for i, paramSet := range result.AllResults {
			if i >= 5 || !paramSet.IsValid {
				break
			}
			fmt.Printf("%d. Fast=%d, Slow=%d, Score=%.4f\n",
				i+1,
				int(paramSet.Parameters["fast_period"]),
				int(paramSet.Parameters["slow_period"]),
				paramSet.Score)
		}
		
	} else {
		// Run single backtest
		strategy := NewSMAStrategy(symbol, 10, 30, logger)
		
		engine := backtest.NewBacktestEngine(config, strategy, logger)
		
		logger.Info("Starting backtest")
		
		result, err := engine.Run()
		if err != nil {
			log.Fatal("Backtest failed:", err)
		}
		
		// Print results
		fmt.Println("\n=== BACKTEST RESULTS ===")
		fmt.Printf("Strategy: %s\n", config.StrategyName)
		fmt.Printf("Period: %s to %s\n", config.StartTime.Format("2006-01-02"), config.EndTime.Format("2006-01-02"))
		fmt.Printf("Initial Capital: $%.2f\n", config.InitialCapital)
		
		printMetrics(result.Metrics)
		
		// Trade statistics
		fmt.Println("\n=== TRADE STATISTICS ===")
		fmt.Printf("Total Trades: %d\n", result.Statistics.TotalTrades)
		fmt.Printf("Winning Trades: %d\n", result.Statistics.WinningTrades)
		fmt.Printf("Losing Trades: %d\n", result.Statistics.LosingTrades)
		fmt.Printf("Win Rate: %.2f%%\n", result.Metrics.WinRate*100)
		fmt.Printf("Profit Factor: %.2f\n", result.Metrics.ProfitFactor)
		fmt.Printf("Average Win: $%.2f\n", result.Statistics.AvgProfit)
		fmt.Printf("Average Loss: $%.2f\n", result.Statistics.AvgLoss)
		fmt.Printf("Max Win: $%.2f\n", result.Statistics.MaxProfit)
		fmt.Printf("Max Loss: $%.2f\n", result.Statistics.MaxLoss)
		fmt.Printf("Max Win Streak: %d\n", result.Statistics.MaxWinStreak)
		fmt.Printf("Max Loss Streak: %d\n", result.Statistics.MaxLossStreak)
		
		// Sample trades
		fmt.Println("\n=== SAMPLE TRADES (First 10) ===")
		fmt.Println("Date\t\tSymbol\tSide\tQuantity\tPrice\t\tPnL")
		for i, trade := range result.Trades {
			if i >= 10 {
				break
			}
			fmt.Printf("%s\t%s\t%s\t%.4f\t\t$%.2f\t\t$%.2f\n",
				trade.Timestamp.Format("2006-01-02"),
				trade.Symbol,
				trade.Side,
				trade.Quantity,
				trade.Price,
				trade.PnL)
		}
		
		// Equity curve summary
		fmt.Println("\n=== EQUITY CURVE SUMMARY ===")
		if len(result.EquityCurve) > 0 {
			finalEquity := result.EquityCurve[len(result.EquityCurve)-1].Equity
			fmt.Printf("Final Equity: $%.2f\n", finalEquity)
			fmt.Printf("Total Return: %.2f%%\n", result.Metrics.TotalReturn*100)
			fmt.Printf("Peak Equity: $%.2f\n", getPeakEquity(result.EquityCurve))
			
			// Monthly returns
			monthlyReturns := calculateMonthlyReturns(result.EquityCurve)
			fmt.Println("\n=== MONTHLY RETURNS ===")
			for month, ret := range monthlyReturns {
				fmt.Printf("%s: %+.2f%%\n", month, ret*100)
			}
		}
	}
}

func printMetrics(metrics *backtest.PerformanceMetrics) {
	fmt.Println("\n=== PERFORMANCE METRICS ===")
	fmt.Printf("Total Return: %.2f%%\n", metrics.TotalReturn*100)
	fmt.Printf("Annualized Return: %.2f%%\n", metrics.AnnualizedReturn*100)
	fmt.Printf("Volatility: %.2f%%\n", metrics.Volatility*100)
	fmt.Printf("Sharpe Ratio: %.4f\n", metrics.SharpeRatio)
	fmt.Printf("Sortino Ratio: %.4f\n", metrics.SortinoRatio)
	fmt.Printf("Calmar Ratio: %.4f\n", metrics.CalmarRatio)
	fmt.Printf("Max Drawdown: %.2f%%\n", metrics.MaxDrawdown*100)
	fmt.Printf("Max Drawdown Days: %d\n", metrics.MaxDrawdownDays)
	fmt.Printf("VaR (95%): %.2f%%\n", metrics.VaR95*100)
	fmt.Printf("CVaR (95%): %.2f%%\n", metrics.CVaR95*100)
	fmt.Printf("Skewness: %.4f\n", metrics.Skewness)
	fmt.Printf("Kurtosis: %.4f\n", metrics.Kurtosis)
}

func getPeakEquity(curve []backtest.EquityPoint) float64 {
	peak := 0.0
	for _, point := range curve {
		if point.Equity > peak {
			peak = point.Equity
		}
	}
	return peak
}

func calculateMonthlyReturns(curve []backtest.EquityPoint) map[string]float64 {
	monthlyReturns := make(map[string]float64)
	
	if len(curve) < 2 {
		return monthlyReturns
	}
	
	// Group by month
	monthlyData := make(map[string][]backtest.EquityPoint)
	for _, point := range curve {
		month := point.Timestamp.Format("2006-01")
		monthlyData[month] = append(monthlyData[month], point)
	}
	
	// Calculate returns for each month
	var prevMonthEnd float64 = curve[0].Equity
	months := make([]string, 0, len(monthlyData))
	for month := range monthlyData {
		months = append(months, month)
	}
	sort.Strings(months)
	
	for _, month := range months {
		points := monthlyData[month]
		if len(points) > 0 {
			monthStart := prevMonthEnd
			monthEnd := points[len(points)-1].Equity
			if monthStart > 0 {
				monthlyReturns[month] = (monthEnd - monthStart) / monthStart
			}
			prevMonthEnd = monthEnd
		}
	}
	
	return monthlyReturns
}