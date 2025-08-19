package backtest_test

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/mExOms/internal/backtest"
	"github.com/mExOms/internal/strategies/arbitrage"
	"github.com/mExOms/internal/strategies/market_maker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBacktestEngine_ArbitrageStrategy(t *testing.T) {
	// Create backtest configuration
	config := backtest.BacktestConfig{
		StartTime:      time.Now().AddDate(0, -1, 0), // 1 month ago
		EndTime:        time.Now(),
		InitialCapital: 100000.0,
		Symbols:        []string{"BTCUSDT", "ETHUSDT"},
		Exchanges:      []string{"binance", "bybit"},
		TickInterval:   1 * time.Second,
		SpreadMultiplier: 1.0,
		SlippageModel: backtest.SlippageModel{
			Type:       "fixed",
			BaseRate:   0.0001, // 0.01%
			ImpactRate: 0.0,
		},
		FeeModel: backtest.FeeModel{
			MakerFee: 0.0002, // 0.02%
			TakerFee: 0.0004, // 0.04%
		},
		LatencySimulation: backtest.LatencySimulation{
			Enabled:     true,
			BaseLatency: 10 * time.Millisecond,
			Jitter:      5 * time.Millisecond,
		},
		OutputPath: "./backtest_results/arbitrage",
	}

	// Create backtest engine
	engine := backtest.NewEngine(config)

	// Use synthetic data provider for testing
	dataProvider := backtest.NewSyntheticDataProvider()
	err := engine.SetDataProvider(dataProvider)
	require.NoError(t, err)

	// Create arbitrage strategy
	arbConfig := arbitrage.Config{
		MinProfitRate:    0.001, // 0.1%
		MaxPositionSize:  10000,
		ExecutionTimeout: 500 * time.Millisecond,
	}
	strategy := backtest.NewArbitrageStrategyAdapter(arbConfig)

	// Add strategy to engine
	err = engine.AddStrategy(strategy)
	require.NoError(t, err)

	// Initialize engine
	err = engine.Initialize(config)
	require.NoError(t, err)

	// Run backtest
	result, err := engine.Run()
	require.NoError(t, err)

	// Verify results
	assert.NotNil(t, result)
	assert.Greater(t, result.TotalTrades, 0)
	assert.Greater(t, result.FinalCapital, 0.0)
	
	// Check metrics
	assert.GreaterOrEqual(t, result.WinRate, 0.0)
	assert.LessOrEqual(t, result.WinRate, 1.0)
	assert.Greater(t, result.SharpeRatio, -5.0)
	assert.Less(t, result.SharpeRatio, 10.0)

	// Generate report
	analyzer := backtest.NewDefaultPerformanceAnalyzer()
	err = analyzer.GenerateReport(result, config.OutputPath)
	require.NoError(t, err)

	log.Printf("Arbitrage Backtest Results:")
	log.Printf("  Total Trades: %d", result.TotalTrades)
	log.Printf("  Win Rate: %.2f%%", result.WinRate*100)
	log.Printf("  Total Return: %.2f%%", result.TotalReturnPct)
	log.Printf("  Sharpe Ratio: %.2f", result.SharpeRatio)
	log.Printf("  Max Drawdown: %.2f%%", result.MaxDrawdownPct)
}

func TestBacktestEngine_MarketMakingStrategy(t *testing.T) {
	// Create backtest configuration
	config := backtest.BacktestConfig{
		StartTime:      time.Now().AddDate(0, 0, -7), // 1 week ago
		EndTime:        time.Now(),
		InitialCapital: 50000.0,
		Symbols:        []string{"BTCUSDT"},
		Exchanges:      []string{"binance"},
		TickInterval:   100 * time.Millisecond,
		SpreadMultiplier: 1.0,
		SlippageModel: backtest.SlippageModel{
			Type:       "linear",
			BaseRate:   0.00005,
			ImpactRate: 0.00001,
		},
		FeeModel: backtest.FeeModel{
			MakerFee: -0.0001, // Maker rebate
			TakerFee: 0.0004,
		},
		OutputPath: "./backtest_results/market_making",
	}

	// Create backtest engine
	engine := backtest.NewEngine(config)

	// Create market making strategy
	mmConfig := market_maker.Config{
		Symbol:          "BTCUSDT",
		BaseSpreadBps:   10,    // 0.1%
		MinSpreadBps:    5,     // 0.05%
		MaxSpreadBps:    50,    // 0.5%
		QuoteSize:       0.01,  // 0.01 BTC
		QuoteLevels:     3,
		LevelSpacingBps: 2,
		MaxInventory:    0.5,   // 0.5 BTC
		InventorySkew:   0.5,
		UpdateInterval:  100 * time.Millisecond,
		RiskLimits: market_maker.RiskLimits{
			MaxPositionValue: 25000,
			StopLossPercent:  0.02,
			MaxDailyLoss:     1000,
		},
	}
	strategy := backtest.NewMarketMakingStrategyAdapter(mmConfig)

	// Add strategy to engine
	err := engine.AddStrategy(strategy)
	require.NoError(t, err)

	// Initialize engine
	err = engine.Initialize(config)
	require.NoError(t, err)

	// Run backtest
	result, err := engine.Run()
	require.NoError(t, err)

	// Verify results
	assert.NotNil(t, result)
	assert.Greater(t, result.TotalTrades, 0)
	
	// Market making should have high win rate but small profits per trade
	assert.Greater(t, result.WinRate, 0.5)
	
	// Check risk metrics
	assert.Less(t, result.MaxDrawdownPct, 20.0) // Max 20% drawdown

	log.Printf("Market Making Backtest Results:")
	log.Printf("  Total Trades: %d", result.TotalTrades)
	log.Printf("  Win Rate: %.2f%%", result.WinRate*100)
	log.Printf("  Average Trade: $%.2f", result.AverageTrade)
	log.Printf("  Total Fees: $%.2f", result.TotalFees)
	log.Printf("  Profit Factor: %.2f", result.ProfitFactor)
}

func TestBacktestEngine_MultipleStrategies(t *testing.T) {
	// Create backtest configuration
	config := backtest.BacktestConfig{
		StartTime:      time.Now().AddDate(0, 0, -1), // 1 day ago
		EndTime:        time.Now(),
		InitialCapital: 200000.0,
		Symbols:        []string{"BTCUSDT", "ETHUSDT"},
		Exchanges:      []string{"binance", "bybit"},
		TickInterval:   500 * time.Millisecond,
		OutputPath:     "./backtest_results/multi_strategy",
	}

	// Create backtest engine
	engine := backtest.NewEngine(config)

	// Add arbitrage strategy
	arbStrategy := backtest.NewArbitrageStrategyAdapter(arbitrage.Config{
		MinProfitRate:   0.001,
		MaxPositionSize: 50000,
	})
	engine.AddStrategy(arbStrategy)

	// Add market making strategy
	mmStrategy := backtest.NewMarketMakingStrategyAdapter(market_maker.Config{
		Symbol:        "BTCUSDT",
		BaseSpreadBps: 15,
		QuoteSize:     0.01,
		MaxInventory:  1.0,
	})
	engine.AddStrategy(mmStrategy)

	// Initialize and run
	err := engine.Initialize(config)
	require.NoError(t, err)

	result, err := engine.Run()
	require.NoError(t, err)

	// Check strategy-specific metrics
	assert.Len(t, result.StrategyMetrics, 2)
	
	arbMetrics := result.StrategyMetrics["Arbitrage"]
	assert.NotNil(t, arbMetrics)
	assert.GreaterOrEqual(t, arbMetrics.TotalTrades, 0)

	mmMetrics := result.StrategyMetrics["MarketMaking"]
	assert.NotNil(t, mmMetrics)
	assert.GreaterOrEqual(t, mmMetrics.TotalTrades, 0)

	log.Printf("Multi-Strategy Backtest Results:")
	log.Printf("  Total Return: %.2f%%", result.TotalReturnPct)
	log.Printf("  Arbitrage Trades: %d", arbMetrics.TotalTrades)
	log.Printf("  Market Making Trades: %d", mmMetrics.TotalTrades)
}

func TestDataProvider_FileProvider(t *testing.T) {
	// Skip if no test data available
	t.Skip("Requires historical data files")

	config := backtest.BacktestConfig{
		StartTime:  time.Now().AddDate(0, -1, 0),
		EndTime:    time.Now(),
		DataPath:   "./testdata/market_data",
		Symbols:    []string{"BTCUSDT"},
		Exchanges:  []string{"binance"},
	}

	provider := backtest.NewFileDataProvider()
	err := provider.Initialize(config)
	require.NoError(t, err)

	// Read some data points
	count := 0
	for provider.HasNext() && count < 100 {
		data, err := provider.Next()
		require.NoError(t, err)
		
		assert.NotEmpty(t, data.Symbol)
		assert.Greater(t, data.Bid, 0.0)
		assert.Greater(t, data.Ask, 0.0)
		assert.Greater(t, data.Ask, data.Bid) // Spread should be positive
		
		count++
	}

	assert.Greater(t, count, 0)
}

func BenchmarkBacktestEngine(b *testing.B) {
	config := backtest.BacktestConfig{
		StartTime:      time.Now().AddDate(0, 0, -1),
		EndTime:        time.Now(),
		InitialCapital: 100000.0,
		Symbols:        []string{"BTCUSDT"},
		Exchanges:      []string{"binance"},
		TickInterval:   100 * time.Millisecond,
	}

	for i := 0; i < b.N; i++ {
		engine := backtest.NewEngine(config)
		
		strategy := backtest.NewArbitrageStrategyAdapter(arbitrage.Config{
			MinProfitRate:   0.001,
			MaxPositionSize: 10000,
		})
		
		engine.AddStrategy(strategy)
		engine.Initialize(config)
		engine.Run()
	}
}