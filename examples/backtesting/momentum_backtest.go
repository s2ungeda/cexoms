package main

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/your-org/mExOms/pkg/backtest"
	"github.com/your-org/mExOms/pkg/types"
	"go.uber.org/zap"
)

// MomentumStrategy implements a momentum strategy with RSI and volume filters
type MomentumStrategy struct {
	symbol           string
	lookbackPeriod   int
	rsiPeriod        int
	rsiOverbought    float64
	rsiOversold      float64
	volumeMultiplier float64
	positionSize     float64
	position         float64
	prices           []float64
	volumes          []float64
	avgVolume        float64
	logger           *zap.Logger
}

// NewMomentumStrategy creates a new momentum strategy
func NewMomentumStrategy(symbol string, lookback, rsiPeriod int, rsiOverbought, rsiOversold, volumeMultiplier float64, logger *zap.Logger) *MomentumStrategy {
	return &MomentumStrategy{
		symbol:           symbol,
		lookbackPeriod:   lookback,
		rsiPeriod:        rsiPeriod,
		rsiOverbought:    rsiOverbought,
		rsiOversold:      rsiOversold,
		volumeMultiplier: volumeMultiplier,
		positionSize:     0.95, // Use 95% of capital
		prices:           make([]float64, 0),
		volumes:          make([]float64, 0),
		logger:           logger,
	}
}

// Initialize initializes the strategy
func (s *MomentumStrategy) Initialize() error {
	s.logger.Info("Initializing momentum strategy",
		zap.String("symbol", s.symbol),
		zap.Int("lookback", s.lookbackPeriod),
		zap.Int("rsi_period", s.rsiPeriod))
	return nil
}

// OnMarketData processes market data and generates signals
func (s *MomentumStrategy) OnMarketData(data *types.MarketData) []*types.Signal {
	if data.Symbol != s.symbol {
		return nil
	}

	// Add price and volume to history
	s.prices = append(s.prices, data.Price)
	s.volumes = append(s.volumes, data.Volume)
	
	// Update average volume
	if len(s.volumes) >= 20 {
		sum := 0.0
		start := len(s.volumes) - 20
		for i := start; i < len(s.volumes); i++ {
			sum += s.volumes[i]
		}
		s.avgVolume = sum / 20
	}
	
	// Need enough data
	maxPeriod := s.lookbackPeriod
	if s.rsiPeriod > maxPeriod {
		maxPeriod = s.rsiPeriod
	}
	if len(s.prices) < maxPeriod+1 {
		return nil
	}
	
	var signals []*types.Signal
	
	// Calculate momentum
	momentum := s.calculateMomentum()
	
	// Calculate RSI
	rsi := s.calculateRSI()
	
	// Check volume filter
	volumeOK := s.avgVolume > 0 && data.Volume >= s.avgVolume*s.volumeMultiplier
	
	// Generate signals
	if s.position == 0 {
		// Long entry - positive momentum, RSI not overbought, high volume
		if momentum > 0 && rsi < s.rsiOverbought && volumeOK {
			signal := &types.Signal{
				Symbol:    s.symbol,
				Side:      types.OrderSideBuy,
				OrderType: types.OrderTypeMarket,
				Quantity:  s.calculateQuantity(data.Price),
				Timestamp: data.Timestamp,
			}
			signals = append(signals, signal)
			
			// Trailing stop loss
			stopPrice := data.Price * 0.97 // 3% trailing stop
			stopSignal := &types.Signal{
				Symbol:    s.symbol,
				Side:      types.OrderSideSell,
				OrderType: types.OrderTypeStopLoss,
				Quantity:  s.calculateQuantity(data.Price),
				StopPrice: stopPrice,
				Timestamp: data.Timestamp,
			}
			signals = append(signals, stopSignal)
			
			s.logger.Debug("Momentum long signal",
				zap.String("symbol", s.symbol),
				zap.Float64("price", data.Price),
				zap.Float64("momentum", momentum),
				zap.Float64("rsi", rsi),
				zap.Float64("volume_ratio", data.Volume/s.avgVolume))
		}
		
		// Short entry - negative momentum, RSI not oversold, high volume
		if momentum < 0 && rsi > s.rsiOversold && volumeOK {
			signal := &types.Signal{
				Symbol:    s.symbol,
				Side:      types.OrderSideSell,
				OrderType: types.OrderTypeMarket,
				Quantity:  s.calculateQuantity(data.Price),
				Timestamp: data.Timestamp,
			}
			signals = append(signals, signal)
			
			// Trailing stop loss
			stopPrice := data.Price * 1.03 // 3% trailing stop
			stopSignal := &types.Signal{
				Symbol:    s.symbol,
				Side:      types.OrderSideBuy,
				OrderType: types.OrderTypeStopLoss,
				Quantity:  s.calculateQuantity(data.Price),
				StopPrice: stopPrice,
				Timestamp: data.Timestamp,
			}
			signals = append(signals, stopSignal)
			
			s.logger.Debug("Momentum short signal",
				zap.String("symbol", s.symbol),
				zap.Float64("price", data.Price),
				zap.Float64("momentum", momentum),
				zap.Float64("rsi", rsi),
				zap.Float64("volume_ratio", data.Volume/s.avgVolume))
		}
	} else {
		// Exit signals
		if s.position > 0 {
			// Exit long - momentum turns negative or RSI overbought
			if momentum < 0 || rsi > s.rsiOverbought {
				signal := &types.Signal{
					Symbol:    s.symbol,
					Side:      types.OrderSideSell,
					OrderType: types.OrderTypeMarket,
					Quantity:  math.Abs(s.position),
					Timestamp: data.Timestamp,
				}
				signals = append(signals, signal)
				
				s.logger.Debug("Momentum long exit",
					zap.String("symbol", s.symbol),
					zap.Float64("momentum", momentum),
					zap.Float64("rsi", rsi))
			}
		} else if s.position < 0 {
			// Exit short - momentum turns positive or RSI oversold
			if momentum > 0 || rsi < s.rsiOversold {
				signal := &types.Signal{
					Symbol:    s.symbol,
					Side:      types.OrderSideBuy,
					OrderType: types.OrderTypeMarket,
					Quantity:  math.Abs(s.position),
					Timestamp: data.Timestamp,
				}
				signals = append(signals, signal)
				
				s.logger.Debug("Momentum short exit",
					zap.String("symbol", s.symbol),
					zap.Float64("momentum", momentum),
					zap.Float64("rsi", rsi))
			}
		}
	}
	
	return signals
}

// OnFill processes fill events
func (s *MomentumStrategy) OnFill(fill *types.Fill) {
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

// calculateMomentum calculates price momentum
func (s *MomentumStrategy) calculateMomentum() float64 {
	if len(s.prices) < s.lookbackPeriod+1 {
		return 0
	}
	
	currentPrice := s.prices[len(s.prices)-1]
	pastPrice := s.prices[len(s.prices)-1-s.lookbackPeriod]
	
	if pastPrice == 0 {
		return 0
	}
	
	return (currentPrice - pastPrice) / pastPrice
}

// calculateRSI calculates Relative Strength Index
func (s *MomentumStrategy) calculateRSI() float64 {
	if len(s.prices) < s.rsiPeriod+1 {
		return 50 // Neutral
	}
	
	gains := 0.0
	losses := 0.0
	
	// Calculate average gains and losses
	start := len(s.prices) - s.rsiPeriod - 1
	for i := start + 1; i < len(s.prices); i++ {
		change := s.prices[i] - s.prices[i-1]
		if change > 0 {
			gains += change
		} else {
			losses += math.Abs(change)
		}
	}
	
	avgGain := gains / float64(s.rsiPeriod)
	avgLoss := losses / float64(s.rsiPeriod)
	
	if avgLoss == 0 {
		return 100 // Max RSI
	}
	
	rs := avgGain / avgLoss
	rsi := 100 - (100 / (1 + rs))
	
	return rsi
}

// calculateQuantity calculates position size
func (s *MomentumStrategy) calculateQuantity(price float64) float64 {
	// Simple fixed percentage of capital
	capital := 95000.0 * s.positionSize
	return capital / price
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
		StrategyName:    "Momentum",
		BarInterval:     "4h",
		TickSize:        time.Hour * 4,
		CommissionRate:  0.001,
		SlippageModel:   backtest.ModelOrderBook,
		SlippageParams: map[string]float64{
			"base_slippage":     0.0001,
			"participation_rate": 0.1,
		},
	}
	
	// Test on multiple symbols
	symbols := []string{"BTCUSDT", "ETHUSDT"}
	
	fmt.Println("=== MOMENTUM STRATEGY BACKTEST ===")
	fmt.Printf("Period: %s to %s\n", config.StartTime.Format("2006-01-02"), config.EndTime.Format("2006-01-02"))
	fmt.Printf("Initial Capital: $%.2f\n\n", config.InitialCapital)
	
	aggregateResults := struct {
		TotalReturn      float64
		TotalTrades      int
		WinningSymbols   int
		BestSymbol       string
		BestReturn       float64
		WorstSymbol      string
		WorstReturn      float64
		AvgSharpe        float64
		AvgMaxDrawdown   float64
	}{}
	
	symbolResults := make(map[string]*backtest.BacktestResult)
	
	// Run backtest for each symbol
	for _, symbol := range symbols {
		fmt.Printf("\n--- Testing %s ---\n", symbol)
		
		strategy := NewMomentumStrategy(
			symbol,
			20,    // lookback period
			14,    // RSI period
			70.0,  // RSI overbought
			30.0,  // RSI oversold
			1.5,   // volume multiplier
			logger,
		)
		
		// Update config for single symbol
		singleConfig := config
		singleConfig.Symbols = []string{symbol}
		
		engine := backtest.NewBacktestEngine(singleConfig, strategy, logger)
		
		result, err := engine.Run()
		if err != nil {
			log.Printf("Backtest failed for %s: %v", symbol, err)
			continue
		}
		
		symbolResults[symbol] = result
		
		// Update aggregate results
		aggregateResults.TotalReturn += result.Metrics.TotalReturn
		aggregateResults.TotalTrades += len(result.Trades)
		aggregateResults.AvgSharpe += result.Metrics.SharpeRatio
		aggregateResults.AvgMaxDrawdown += result.Metrics.MaxDrawdown
		
		if result.Metrics.TotalReturn > 0 {
			aggregateResults.WinningSymbols++
		}
		
		if result.Metrics.TotalReturn > aggregateResults.BestReturn {
			aggregateResults.BestReturn = result.Metrics.TotalReturn
			aggregateResults.BestSymbol = symbol
		}
		
		if aggregateResults.WorstSymbol == "" || result.Metrics.TotalReturn < aggregateResults.WorstReturn {
			aggregateResults.WorstReturn = result.Metrics.TotalReturn
			aggregateResults.WorstSymbol = symbol
		}
		
		// Print symbol results
		fmt.Printf("Return: %.2f%%\n", result.Metrics.TotalReturn*100)
		fmt.Printf("Sharpe Ratio: %.2f\n", result.Metrics.SharpeRatio)
		fmt.Printf("Max Drawdown: %.2f%%\n", result.Metrics.MaxDrawdown*100)
		fmt.Printf("Total Trades: %d\n", len(result.Trades))
		fmt.Printf("Win Rate: %.2f%%\n", result.Metrics.WinRate*100)
		fmt.Printf("Profit Factor: %.2f\n", result.Metrics.ProfitFactor)
	}
	
	// Calculate averages
	numSymbols := float64(len(symbols))
	aggregateResults.AvgSharpe /= numSymbols
	aggregateResults.AvgMaxDrawdown /= numSymbols
	
	// Print aggregate results
	fmt.Println("\n=== AGGREGATE RESULTS ===")
	fmt.Printf("Average Return: %.2f%%\n", aggregateResults.TotalReturn/numSymbols*100)
	fmt.Printf("Total Trades: %d\n", aggregateResults.TotalTrades)
	fmt.Printf("Winning Symbols: %d/%d\n", aggregateResults.WinningSymbols, len(symbols))
	fmt.Printf("Best Symbol: %s (%.2f%%)\n", aggregateResults.BestSymbol, aggregateResults.BestReturn*100)
	fmt.Printf("Worst Symbol: %s (%.2f%%)\n", aggregateResults.WorstSymbol, aggregateResults.WorstReturn*100)
	fmt.Printf("Average Sharpe: %.2f\n", aggregateResults.AvgSharpe)
	fmt.Printf("Average Max Drawdown: %.2f%%\n", aggregateResults.AvgMaxDrawdown*100)
	
	// Correlation analysis
	fmt.Println("\n=== CORRELATION ANALYSIS ===")
	if btcResult, btcExists := symbolResults["BTCUSDT"]; btcExists {
		if ethResult, ethExists := symbolResults["ETHUSDT"]; ethExists {
			// Simple correlation check based on trade timing
			btcTrades := btcResult.Trades
			ethTrades := ethResult.Trades
			
			overlappingTrades := 0
			for _, btcTrade := range btcTrades {
				for _, ethTrade := range ethTrades {
					// Check if trades happened on same day
					if btcTrade.Timestamp.Format("2006-01-02") == ethTrade.Timestamp.Format("2006-01-02") {
						if btcTrade.Side == ethTrade.Side {
							overlappingTrades++
						}
						break
					}
				}
			}
			
			correlationScore := float64(overlappingTrades) / float64(len(btcTrades))
			fmt.Printf("Trade Correlation: %.2f%% (same direction trades on same days)\n", correlationScore*100)
		}
	}
	
	// Monthly breakdown
	fmt.Println("\n=== MONTHLY PERFORMANCE ===")
	monthlyPnL := make(map[string]float64)
	
	for symbol, result := range symbolResults {
		fmt.Printf("\n%s Monthly Returns:\n", symbol)
		
		// Calculate monthly returns from trades
		for _, trade := range result.Trades {
			month := trade.Timestamp.Format("2006-01")
			monthlyPnL[month] += trade.PnL
		}
	}
	
	// Sort and display months
	months := make([]string, 0, len(monthlyPnL))
	for month := range monthlyPnL {
		months = append(months, month)
	}
	sort.Strings(months)
	
	for _, month := range months {
		pnl := monthlyPnL[month]
		fmt.Printf("%s: %+.2f\n", month, pnl)
	}
	
	// Risk metrics comparison
	fmt.Println("\n=== RISK METRICS COMPARISON ===")
	fmt.Printf("%-10s %10s %10s %10s %10s %10s\n", "Symbol", "Volatility", "VaR(95%)", "CVaR(95%)", "Skewness", "Kurtosis")
	fmt.Println(strings.Repeat("-", 70))
	
	for symbol, result := range symbolResults {
		fmt.Printf("%-10s %9.2f%% %9.2f%% %9.2f%% %10.2f %10.2f\n",
			symbol,
			result.Metrics.Volatility*100,
			result.Metrics.VaR95*100,
			result.Metrics.CVaR95*100,
			result.Metrics.Skewness,
			result.Metrics.Kurtosis)
	}
}