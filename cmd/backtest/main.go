package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/mExOms/oms/internal/backtest"
	"github.com/shopspring/decimal"
)

type Config struct {
	DataDir        string          `json:"data_dir"`
	StartDate      string          `json:"start_date"`
	EndDate        string          `json:"end_date"`
	InitialCapital float64         `json:"initial_capital"`
	Strategy       StrategyConfig  `json:"strategy"`
	TradingFees    float64         `json:"trading_fees"`
	OutputDir      string          `json:"output_dir"`
}

type StrategyConfig struct {
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters"`
}

func main() {
	var (
		configFile   = flag.String("config", "backtest.json", "Config file path")
		dataDir      = flag.String("data", "./backtest_data", "Historical data directory")
		strategyName = flag.String("strategy", "sma", "Strategy name (sma, momentum)")
		startDate    = flag.String("start", "", "Start date (YYYY-MM-DD)")
		endDate      = flag.String("end", "", "End date (YYYY-MM-DD)")
		capital      = flag.Float64("capital", 10000, "Initial capital")
		outputDir    = flag.String("output", "./backtest_results", "Output directory")
		loadData     = flag.Bool("load-data", false, "Load sample historical data")
	)
	flag.Parse()

	// Load sample data if requested
	if *loadData {
		if err := loadSampleData(*dataDir); err != nil {
			log.Fatal("Failed to load sample data:", err)
		}
		fmt.Println("Sample data loaded successfully")
		return
	}

	// Load or create config
	config, err := loadConfig(*configFile, *dataDir, *strategyName, *startDate, *endDate, *capital, *outputDir)
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	// Create event store
	eventStore, err := backtest.NewEventStore(config.DataDir)
	if err != nil {
		log.Fatal("Failed to create event store:", err)
	}
	defer eventStore.Close()

	// Print event store statistics
	stats := eventStore.GetStatistics()
	fmt.Printf("Event Store Statistics:\n")
	fmt.Printf("  Total Events: %v\n", stats["total_events"])
	fmt.Printf("  Total Files: %v\n", stats["total_files"])

	// Parse dates
	startTime, err := time.Parse("2006-01-02", config.StartDate)
	if err != nil {
		log.Fatal("Invalid start date:", err)
	}
	endTime, err := time.Parse("2006-01-02", config.EndDate)
	if err != nil {
		log.Fatal("Invalid end date:", err)
	}

	// Create backtest config
	btConfig := backtest.BacktestConfig{
		StartTime:        startTime,
		EndTime:          endTime,
		InitialCapital:   decimal.NewFromFloat(config.InitialCapital),
		TradingFees:      decimal.NewFromFloat(config.TradingFees),
		ExecutionLatency: 100 * time.Millisecond,
		DataFrequency:    1 * time.Minute,
	}

	// Create backtest engine
	engine, err := backtest.NewBacktestEngine(eventStore, btConfig)
	if err != nil {
		log.Fatal("Failed to create backtest engine:", err)
	}

	// Create strategy
	strategy, err := createStrategy(config.Strategy)
	if err != nil {
		log.Fatal("Failed to create strategy:", err)
	}

	// Run backtest
	fmt.Printf("\nRunning backtest...\n")
	fmt.Printf("  Strategy: %s\n", config.Strategy.Name)
	fmt.Printf("  Period: %s to %s\n", config.StartDate, config.EndDate)
	fmt.Printf("  Initial Capital: $%.2f\n\n", config.InitialCapital)

	ctx := context.Background()
	if err := engine.RunStrategy(ctx, strategy); err != nil {
		log.Fatal("Backtest failed:", err)
	}

	// Get results
	results := engine.GetResults()

	// Display summary
	displaySummary(results)

	// Generate report
	analyzer := backtest.NewResultAnalyzer(results)
	report := analyzer.GenerateReport()

	// Save report
	if err := analyzer.SaveReport(report, config.OutputDir); err != nil {
		log.Printf("Failed to save report: %v", err)
	} else {
		fmt.Printf("\nReport saved to %s\n", config.OutputDir)
	}
}

func loadConfig(configFile, dataDir, strategyName, startDate, endDate string, capital float64, outputDir string) (*Config, error) {
	// Try to load from file
	if _, err := os.Stat(configFile); err == nil {
		data, err := os.ReadFile(configFile)
		if err != nil {
			return nil, err
		}

		var config Config
		if err := json.Unmarshal(data, &config); err != nil {
			return nil, err
		}
		return &config, nil
	}

	// Create default config from flags
	config := &Config{
		DataDir:        dataDir,
		StartDate:      startDate,
		EndDate:        endDate,
		InitialCapital: capital,
		TradingFees:    0.001, // 0.1%
		OutputDir:      outputDir,
		Strategy: StrategyConfig{
			Name: strategyName,
			Parameters: map[string]interface{}{
				"short_period": 10,
				"long_period":  30,
				"lookback":     20,
				"threshold":    0.02,
			},
		},
	}

	// Validate dates
	if config.StartDate == "" || config.EndDate == "" {
		return nil, fmt.Errorf("start and end dates are required")
	}

	return config, nil
}

func createStrategy(config StrategyConfig) (backtest.TradingStrategy, error) {
	switch config.Name {
	case "sma", "moving_average":
		shortPeriod := getIntParam(config.Parameters, "short_period", 10)
		longPeriod := getIntParam(config.Parameters, "long_period", 30)
		return backtest.NewSimpleMovingAverageStrategy(shortPeriod, longPeriod), nil

	case "momentum":
		lookback := getIntParam(config.Parameters, "lookback", 20)
		threshold := getFloatParam(config.Parameters, "threshold", 0.02)
		return backtest.NewMomentumStrategy(lookback, threshold), nil

	default:
		return nil, fmt.Errorf("unknown strategy: %s", config.Name)
	}
}

func getIntParam(params map[string]interface{}, key string, defaultValue int) int {
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case float64:
			return int(v)
		}
	}
	return defaultValue
}

func getFloatParam(params map[string]interface{}, key string, defaultValue float64) float64 {
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		}
	}
	return defaultValue
}

func displaySummary(results *backtest.BacktestResults) {
	metrics := results.Metrics
	portfolio := results.Portfolio

	fmt.Printf("\n=== Backtest Results ===\n")
	fmt.Printf("Final Portfolio Value: $%.2f\n", portfolio.TotalValue.InexactFloat64())
	fmt.Printf("Total Return: %.2f%%\n", metrics.TotalReturn.Mul(decimal.NewFromInt(100)).InexactFloat64())
	fmt.Printf("Total Trades: %d\n", metrics.TotalTrades)
	fmt.Printf("Win Rate: %.2f%%\n", metrics.WinRate*100)
	fmt.Printf("Sharpe Ratio: %.2f\n", metrics.SharpeRatio)
	fmt.Printf("Max Drawdown: %.2f%%\n", metrics.MaxDrawdown.Mul(decimal.NewFromInt(100)).InexactFloat64())

	if metrics.TotalTrades > 0 {
		fmt.Printf("\n=== Trade Statistics ===\n")
		fmt.Printf("Winning Trades: %d\n", metrics.WinningTrades)
		fmt.Printf("Losing Trades: %d\n", metrics.LosingTrades)
		if !metrics.AvgWin.IsZero() {
			fmt.Printf("Average Win: $%.2f\n", metrics.AvgWin.InexactFloat64())
		}
		if !metrics.AvgLoss.IsZero() {
			fmt.Printf("Average Loss: $%.2f\n", metrics.AvgLoss.InexactFloat64())
		}
	}

	fmt.Printf("\n=== Positions ===\n")
	if len(portfolio.Positions) == 0 {
		fmt.Printf("No open positions\n")
	} else {
		for symbol, pos := range portfolio.Positions {
			fmt.Printf("%s: %.4f @ $%.2f (P&L: $%.2f)\n",
				symbol,
				pos.Quantity.InexactFloat64(),
				pos.CurrentPrice.InexactFloat64(),
				pos.UnrealizedPL.InexactFloat64(),
			)
		}
	}
}

func loadSampleData(dataDir string) error {
	// Create event store
	eventStore, err := backtest.NewEventStore(dataDir)
	if err != nil {
		return err
	}
	defer eventStore.Close()

	// Generate sample data for last 30 days
	now := time.Now()
	startTime := now.AddDate(0, 0, -30)

	// Symbols to generate data for
	symbols := []string{"BTCUSDT", "ETHUSDT", "BNBUSDT"}

	// Base prices
	basePrices := map[string]float64{
		"BTCUSDT": 45000,
		"ETHUSDT": 2500,
		"BNBUSDT": 300,
	}

	fmt.Println("Generating sample historical data...")

	for _, symbol := range symbols {
		basePrice := basePrices[symbol]
		currentTime := startTime
		price := basePrice

		for currentTime.Before(now) {
			// Generate price with some volatility
			change := (randomFloat() - 0.5) * 0.002 // +/- 0.2%
			price = price * (1 + change)

			// Create ticker event
			tickerEvent := &backtest.MarketEvent{
				Type:      backtest.EventTypeTicker,
				Exchange:  "binance",
				Symbol:    symbol,
				Timestamp: currentTime,
				Data: map[string]interface{}{
					"last_price": price,
					"bid_price":  price * 0.9999,
					"ask_price":  price * 1.0001,
					"volume":     randomFloat() * 1000000,
				},
			}

			if err := eventStore.RecordEvent(tickerEvent); err != nil {
				return err
			}

			// Create orderbook event every 5 minutes
			if currentTime.Minute()%5 == 0 {
				orderbookEvent := &backtest.MarketEvent{
					Type:      backtest.EventTypeOrderBook,
					Exchange:  "binance",
					Symbol:    symbol,
					Timestamp: currentTime,
					Data: map[string]interface{}{
						"bids": generateOrderBookSide(price, false),
						"asks": generateOrderBookSide(price, true),
					},
				}

				if err := eventStore.RecordEvent(orderbookEvent); err != nil {
					return err
				}
			}

			currentTime = currentTime.Add(1 * time.Minute)
		}
	}

	fmt.Printf("Generated sample data for %d symbols over %d days\n", len(symbols), 30)
	return nil
}

func generateOrderBookSide(basePrice float64, isAsk bool) []interface{} {
	side := make([]interface{}, 10)
	for i := 0; i < 10; i++ {
		var price float64
		if isAsk {
			price = basePrice * (1 + float64(i)*0.0001)
		} else {
			price = basePrice * (1 - float64(i)*0.0001)
		}
		quantity := randomFloat() * 10
		side[i] = []interface{}{price, quantity}
	}
	return side
}

func randomFloat() float64 {
	return float64(time.Now().UnixNano()%1000) / 1000
}