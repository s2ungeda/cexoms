package main

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/your-org/mExOms/pkg/backtest"
	"github.com/your-org/mExOms/pkg/types"
	"go.uber.org/zap"
)

// MeanReversionStrategy implements a Bollinger Bands mean reversion strategy
type MeanReversionStrategy struct {
	symbol         string
	bbPeriod      int
	bbStdDev      float64
	entryThreshold float64
	exitThreshold  float64
	stopLoss      float64
	position      float64
	entryPrice    float64
	prices        []float64
	logger        *zap.Logger
}

// NewMeanReversionStrategy creates a new mean reversion strategy
func NewMeanReversionStrategy(symbol string, bbPeriod int, bbStdDev, entryThreshold, exitThreshold, stopLoss float64, logger *zap.Logger) *MeanReversionStrategy {
	return &MeanReversionStrategy{
		symbol:         symbol,
		bbPeriod:      bbPeriod,
		bbStdDev:      bbStdDev,
		entryThreshold: entryThreshold,
		exitThreshold:  exitThreshold,
		stopLoss:      stopLoss,
		prices:        make([]float64, 0),
		logger:        logger,
	}
}

// Initialize initializes the strategy
func (s *MeanReversionStrategy) Initialize() error {
	s.logger.Info("Initializing mean reversion strategy",
		zap.String("symbol", s.symbol),
		zap.Int("bb_period", s.bbPeriod),
		zap.Float64("bb_stddev", s.bbStdDev))
	return nil
}

// OnMarketData processes market data and generates signals
func (s *MeanReversionStrategy) OnMarketData(data *types.MarketData) []*types.Signal {
	if data.Symbol != s.symbol {
		return nil
	}

	// Add price to history
	s.prices = append(s.prices, data.Price)
	
	// Need enough data for Bollinger Bands
	if len(s.prices) < s.bbPeriod {
		return nil
	}
	
	// Calculate Bollinger Bands
	middle, upper, lower := s.calculateBollingerBands()
	
	// Calculate position relative to bands
	bandWidth := upper - lower
	if bandWidth <= 0 {
		return nil
	}
	
	pricePosition := (data.Price - lower) / bandWidth
	
	var signals []*types.Signal
	
	// Check for entry signals
	if s.position == 0 {
		// Long entry - price near lower band
		if pricePosition < s.entryThreshold {
			signal := &types.Signal{
				Symbol:    s.symbol,
				Side:      types.OrderSideBuy,
				OrderType: types.OrderTypeMarket,
				Quantity:  s.calculatePositionSize(data.Price),
				Timestamp: data.Timestamp,
			}
			signals = append(signals, signal)
			
			// Set stop loss
			stopPrice := data.Price * (1 - s.stopLoss)
			stopSignal := &types.Signal{
				Symbol:    s.symbol,
				Side:      types.OrderSideSell,
				OrderType: types.OrderTypeStopLoss,
				Quantity:  s.calculatePositionSize(data.Price),
				StopPrice: stopPrice,
				Timestamp: data.Timestamp,
			}
			signals = append(signals, stopSignal)
			
			s.logger.Debug("Long entry signal",
				zap.String("symbol", s.symbol),
				zap.Float64("price", data.Price),
				zap.Float64("lower_band", lower),
				zap.Float64("position_in_band", pricePosition))
		}
		
		// Short entry - price near upper band
		if pricePosition > (1 - s.entryThreshold) {
			signal := &types.Signal{
				Symbol:    s.symbol,
				Side:      types.OrderSideSell,
				OrderType: types.OrderTypeMarket,
				Quantity:  s.calculatePositionSize(data.Price),
				Timestamp: data.Timestamp,
			}
			signals = append(signals, signal)
			
			// Set stop loss
			stopPrice := data.Price * (1 + s.stopLoss)
			stopSignal := &types.Signal{
				Symbol:    s.symbol,
				Side:      types.OrderSideBuy,
				OrderType: types.OrderTypeStopLoss,
				Quantity:  s.calculatePositionSize(data.Price),
				StopPrice: stopPrice,
				Timestamp: data.Timestamp,
			}
			signals = append(signals, stopSignal)
			
			s.logger.Debug("Short entry signal",
				zap.String("symbol", s.symbol),
				zap.Float64("price", data.Price),
				zap.Float64("upper_band", upper),
				zap.Float64("position_in_band", pricePosition))
		}
	} else {
		// Check for exit signals
		if s.position > 0 {
			// Exit long - price returned to middle or above
			if pricePosition > s.exitThreshold || data.Price >= middle {
				signal := &types.Signal{
					Symbol:    s.symbol,
					Side:      types.OrderSideSell,
					OrderType: types.OrderTypeMarket,
					Quantity:  math.Abs(s.position),
					Timestamp: data.Timestamp,
				}
				signals = append(signals, signal)
				
				s.logger.Debug("Long exit signal",
					zap.String("symbol", s.symbol),
					zap.Float64("price", data.Price),
					zap.Float64("middle_band", middle),
					zap.Float64("pnl_percent", (data.Price-s.entryPrice)/s.entryPrice*100))
			}
		} else if s.position < 0 {
			// Exit short - price returned to middle or below
			if pricePosition < (1-s.exitThreshold) || data.Price <= middle {
				signal := &types.Signal{
					Symbol:    s.symbol,
					Side:      types.OrderSideBuy,
					OrderType: types.OrderTypeMarket,
					Quantity:  math.Abs(s.position),
					Timestamp: data.Timestamp,
				}
				signals = append(signals, signal)
				
				s.logger.Debug("Short exit signal",
					zap.String("symbol", s.symbol),
					zap.Float64("price", data.Price),
					zap.Float64("middle_band", middle),
					zap.Float64("pnl_percent", (s.entryPrice-data.Price)/s.entryPrice*100))
			}
		}
	}
	
	return signals
}

// OnFill processes fill events
func (s *MeanReversionStrategy) OnFill(fill *types.Fill) {
	if fill.Symbol != s.symbol {
		return
	}
	
	if fill.Side == types.OrderSideBuy {
		if s.position <= 0 {
			s.entryPrice = fill.Price
		}
		s.position += fill.Quantity
	} else {
		if s.position >= 0 {
			s.entryPrice = fill.Price
		}
		s.position -= fill.Quantity
	}
	
	s.logger.Info("Fill processed",
		zap.String("symbol", fill.Symbol),
		zap.String("side", string(fill.Side)),
		zap.Float64("quantity", fill.Quantity),
		zap.Float64("price", fill.Price),
		zap.Float64("position", s.position))
}

// calculateBollingerBands calculates Bollinger Bands
func (s *MeanReversionStrategy) calculateBollingerBands() (middle, upper, lower float64) {
	if len(s.prices) < s.bbPeriod {
		return 0, 0, 0
	}
	
	// Calculate SMA (middle band)
	sum := 0.0
	start := len(s.prices) - s.bbPeriod
	for i := start; i < len(s.prices); i++ {
		sum += s.prices[i]
	}
	middle = sum / float64(s.bbPeriod)
	
	// Calculate standard deviation
	variance := 0.0
	for i := start; i < len(s.prices); i++ {
		variance += math.Pow(s.prices[i]-middle, 2)
	}
	stdDev := math.Sqrt(variance / float64(s.bbPeriod))
	
	// Calculate bands
	upper = middle + (s.bbStdDev * stdDev)
	lower = middle - (s.bbStdDev * stdDev)
	
	return middle, upper, lower
}

// calculatePositionSize calculates position size
func (s *MeanReversionStrategy) calculatePositionSize(price float64) float64 {
	// Use fixed position size for simplicity
	// In real implementation, this would use Kelly criterion or volatility-based sizing
	capitalPerTrade := 20000.0
	return capitalPerTrade / price
}

func main() {
	// Setup logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	
	// Test multiple parameter combinations
	parameterSets := []struct {
		name           string
		bbPeriod      int
		bbStdDev      float64
		entryThreshold float64
		exitThreshold  float64
		stopLoss      float64
	}{
		{"Conservative", 20, 2.0, 0.1, 0.5, 0.02},
		{"Standard", 20, 2.0, 0.15, 0.5, 0.03},
		{"Aggressive", 14, 1.5, 0.2, 0.4, 0.04},
		{"Tight Bands", 20, 1.5, 0.1, 0.6, 0.025},
		{"Wide Bands", 30, 2.5, 0.05, 0.5, 0.03},
	}
	
	// Define backtest configuration
	config := backtest.BacktestConfig{
		StartTime:       time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:         time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		InitialCapital:  100000.0,
		Symbols:         []string{"ETHUSDT"},
		DataPath:        "./data/historical",
		StrategyName:    "Mean Reversion",
		BarInterval:     "15m",
		TickSize:        time.Minute * 15,
		CommissionRate:  0.001,
		SlippageModel:   backtest.ModelSquareRoot,
		SlippageParams: map[string]float64{
			"base_slippage":    0.0001,
			"impact_coefficient": 0.1,
		},
		MaxPositionSize: 50.0,
	}
	
	// Results storage
	results := make([]struct {
		Name    string
		Metrics *backtest.PerformanceMetrics
		Trades  int
		Config  string
	}, 0)
	
	// Run backtests for each parameter set
	for _, params := range parameterSets {
		fmt.Printf("\n=== Testing %s Parameters ===\n", params.name)
		
		strategy := NewMeanReversionStrategy(
			"ETHUSDT",
			params.bbPeriod,
			params.bbStdDev,
			params.entryThreshold,
			params.exitThreshold,
			params.stopLoss,
			logger,
		)
		
		engine := backtest.NewBacktestEngine(config, strategy, logger)
		
		result, err := engine.Run()
		if err != nil {
			log.Printf("Backtest failed for %s: %v", params.name, err)
			continue
		}
		
		results = append(results, struct {
			Name    string
			Metrics *backtest.PerformanceMetrics
			Trades  int
			Config  string
		}{
			Name:    params.name,
			Metrics: result.Metrics,
			Trades:  len(result.Trades),
			Config:  fmt.Sprintf("BB(%d,%.1f) Entry:%.0f%% Exit:%.0f%% SL:%.1f%%", 
				params.bbPeriod, params.bbStdDev, 
				params.entryThreshold*100, params.exitThreshold*100, params.stopLoss*100),
		})
		
		// Print summary
		fmt.Printf("Return: %.2f%%, Sharpe: %.2f, Drawdown: %.2f%%, Trades: %d\n",
			result.Metrics.TotalReturn*100,
			result.Metrics.SharpeRatio,
			result.Metrics.MaxDrawdown*100,
			len(result.Trades))
	}
	
	// Print comparison table
	fmt.Println("\n=== STRATEGY COMPARISON ===")
	fmt.Printf("%-15s %-35s %10s %10s %10s %10s %10s %8s\n", 
		"Strategy", "Config", "Return", "Sharpe", "Sortino", "MaxDD", "Calmar", "Trades")
	fmt.Println(strings.Repeat("-", 120))
	
	for _, r := range results {
		fmt.Printf("%-15s %-35s %9.2f%% %10.2f %10.2f %9.2f%% %10.2f %8d\n",
			r.Name,
			r.Config,
			r.Metrics.TotalReturn*100,
			r.Metrics.SharpeRatio,
			r.Metrics.SortinoRatio,
			r.Metrics.MaxDrawdown*100,
			r.Metrics.CalmarRatio,
			r.Trades)
	}
	
	// Find best performing strategy
	var bestStrategy string
	var bestSharpe float64
	for _, r := range results {
		if r.Metrics.SharpeRatio > bestSharpe {
			bestSharpe = r.Metrics.SharpeRatio
			bestStrategy = r.Name
		}
	}
	
	fmt.Printf("\nBest Strategy (by Sharpe Ratio): %s with Sharpe %.2f\n", bestStrategy, bestSharpe)
	
	// Risk analysis
	fmt.Println("\n=== RISK ANALYSIS ===")
	for _, r := range results {
		fmt.Printf("\n%s Strategy:\n", r.Name)
		fmt.Printf("  VaR (95%%): %.2f%%\n", r.Metrics.VaR95*100)
		fmt.Printf("  CVaR (95%%): %.2f%%\n", r.Metrics.CVaR95*100)
		fmt.Printf("  Volatility: %.2f%%\n", r.Metrics.Volatility*100)
		fmt.Printf("  Skewness: %.2f\n", r.Metrics.Skewness)
		fmt.Printf("  Kurtosis: %.2f\n", r.Metrics.Kurtosis)
		fmt.Printf("  Win Rate: %.2f%%\n", r.Metrics.WinRate*100)
		fmt.Printf("  Profit Factor: %.2f\n", r.Metrics.ProfitFactor)
	}
}