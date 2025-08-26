package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/mExOms/pkg/backtest"
	"github.com/mExOms/pkg/strategies"
	"github.com/mExOms/pkg/strategies/templates"
	"github.com/mExOms/pkg/types"
)

// MockOrderManager implements types.OrderManager for testing
type MockOrderManager struct {
	engine *backtest.BacktestEngine
}

func (m *MockOrderManager) PlaceOrder(req *types.OrderRequest) (*types.Order, error) {
	return m.engine.PlaceOrder(req)
}

func (m *MockOrderManager) CancelOrder(orderID string) error {
	return m.engine.CancelOrder(orderID)
}

func (m *MockOrderManager) GetOrder(orderID string) (*types.Order, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *MockOrderManager) GetActiveOrders(symbol string) ([]*types.Order, error) {
	return nil, nil
}

// MockPositionManager implements types.PositionManager for testing
type MockPositionManager struct {
	positions map[string]*types.Position
}

func NewMockPositionManager() *MockPositionManager {
	return &MockPositionManager{
		positions: make(map[string]*types.Position),
	}
}

func (m *MockPositionManager) GetPosition(symbol string) (*types.Position, error) {
	pos, exists := m.positions[symbol]
	if !exists {
		return &types.Position{
			Symbol:   symbol,
			Quantity: decimal.Zero,
			Side:     types.PositionSideLong,
		}, nil
	}
	return pos, nil
}

func (m *MockPositionManager) UpdatePosition(pos *types.Position) error {
	m.positions[pos.Symbol] = pos
	return nil
}

func (m *MockPositionManager) GetAllPositions() ([]*types.Position, error) {
	positions := make([]*types.Position, 0, len(m.positions))
	for _, pos := range m.positions {
		positions = append(positions, pos)
	}
	return positions, nil
}

func (m *MockPositionManager) GetTotalExposure() decimal.Decimal {
	total := decimal.Zero
	for _, pos := range m.positions {
		total = total.Add(pos.Quantity.Mul(pos.AvgPrice))
	}
	return total
}

// MockMarketDataProvider implements types.MarketDataProvider for testing
type MockMarketDataProvider struct{}

func (m *MockMarketDataProvider) SubscribeTicker(symbol string) error {
	return nil
}

func (m *MockMarketDataProvider) SubscribeOrderBook(symbol string) error {
	return nil
}

func (m *MockMarketDataProvider) UnsubscribeTicker(symbol string) error {
	return nil
}

func (m *MockMarketDataProvider) UnsubscribeOrderBook(symbol string) error {
	return nil
}

func (m *MockMarketDataProvider) GetOrderBook(symbol string) (*types.OrderBook, error) {
	return nil, fmt.Errorf("not implemented")
}

func main() {
	// Create logger
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	
	logger, err := config.Build()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Sync()

	// Create output directory
	outputDir := "backtest_results"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		logger.Fatal("Failed to create output directory", zap.Error(err))
	}

	// Backtest configuration
	startTime := time.Now().AddDate(0, -1, 0) // 1 year ago
	endTime := time.Now()

	backtestConfig := &backtest.BacktestConfig{
		StartTime: startTime,
		EndTime:   endTime,
		InitialBalance: map[string]decimal.Decimal{
			"USDT": decimal.NewFromInt(100000), // $100k initial capital
		},
		TickInterval: time.Second,
		SpreadBps:    20,  // 0.20% spread
		SlippageBps:  10,  // 0.10% slippage
		TakerFeeBps:  10,  // 0.10% taker fee
		MakerFeeBps:  5,   // 0.05% maker fee
		Symbols:      []string{"BTC-USDT", "ETH-USDT"},
		Exchanges:    []string{"binance"},
	}

	// Test different strategies
	strategies := []struct {
		name        string
		strategyFunc func() strategies.Strategy
		config      strategies.StrategyConfig
	}{
		{
			name: "Arbitrage",
			strategyFunc: func() strategies.Strategy {
				return templates.NewArbitrageTemplate(
					nil, // Will be set by backtest engine
					nil, // Will be set by backtest engine
					nil, // Will be set by backtest engine
					logger,
				)
			},
			config: strategies.StrategyConfig{
				ID:        "arb-001",
				Name:      "Cross-Exchange Arbitrage",
				Type:      strategies.StrategyTypeArbitrage,
				Symbols:   []string{"BTC-USDT"},
				Exchanges: []string{"binance", "bybit"},
				Parameters: map[string]interface{}{
					"minSpreadPercent": 0.5,
					"maxPositionSize":  1.0,
					"executionMode":    "aggressive",
				},
			},
		},
		{
			name: "Grid Trading",
			strategyFunc: func() strategies.Strategy {
				return templates.NewGridTemplate(
					nil, // Will be set by backtest engine
					nil, // Will be set by backtest engine
					nil, // Will be set by backtest engine
					logger,
				)
			},
			config: strategies.StrategyConfig{
				ID:        "grid-001",
				Name:      "BTC Grid Trading",
				Type:      strategies.StrategyTypeGrid,
				Symbols:   []string{"BTC-USDT"},
				Exchanges: []string{"binance"},
				Parameters: map[string]interface{}{
					"gridLevels":     20,
					"gridSpacing":    0.5,  // 0.5% spacing
					"upperBound":     50000.0,
					"lowerBound":     40000.0,
					"orderSize":      0.01,
					"reinvestProfit": true,
				},
			},
		},
		{
			name: "Market Making",
			strategyFunc: func() strategies.Strategy {
				return templates.NewMarketMakingTemplate(
					nil, // Will be set by backtest engine
					nil, // Will be set by backtest engine
					nil, // Will be set by backtest engine
					logger,
				)
			},
			config: strategies.StrategyConfig{
				ID:        "mm-001",
				Name:      "ETH Market Making",
				Type:      strategies.StrategyTypeMarketMaking,
				Symbols:   []string{"ETH-USDT"},
				Exchanges: []string{"binance"},
				Parameters: map[string]interface{}{
					"spreadBps":       50,    // 0.50% spread
					"orderDepth":      5,     // 5 levels each side
					"sizingMode":      "dynamic",
					"maxInventory":    10.0,
					"inventoryTarget": 5.0,
					"skewFactor":      0.1,
				},
			},
		},
	}

	// Run backtests
	for _, test := range strategies {
		logger.Info("Starting backtest", 
			zap.String("strategy", test.name),
			zap.String("id", test.config.ID))

		// Create data provider
		// Using synthetic data for demonstration
		dataProvider := backtest.NewSyntheticDataProvider(
			test.config.Symbols[0],
			startTime,
			endTime,
			decimal.NewFromInt(45000), // Base price for BTC
			0.5, // Volatility
		)

		// Create backtest engine
		engine := backtest.NewBacktestEngine(
			backtestConfig,
			test.strategyFunc(),
			dataProvider,
			logger,
		)

		// Set up mock managers with engine
		orderManager := &MockOrderManager{engine: engine}
		positionManager := NewMockPositionManager()
		marketDataProvider := &MockMarketDataProvider{}

		// Inject dependencies into strategy
		strategy := test.strategyFunc()
		if baseStrategy, ok := strategy.(*strategies.BaseStrategy); ok {
			baseStrategy.OrderManager = orderManager
			baseStrategy.PositionManager = positionManager
			baseStrategy.MarketData = marketDataProvider
		}

		// Run backtest
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		if err := engine.Run(ctx); err != nil {
			logger.Error("Backtest failed",
				zap.String("strategy", test.name),
				zap.Error(err))
			cancel()
			continue
		}
		cancel()

		// Generate report
		report := backtest.GenerateReport(engine, test.config.Name, string(test.config.Type))

		// Save JSON report
		jsonPath := filepath.Join(outputDir, fmt.Sprintf("%s_report.json", test.config.ID))
		jsonData, err := report.ToJSON()
		if err != nil {
			logger.Error("Failed to generate JSON report",
				zap.String("strategy", test.name),
				zap.Error(err))
			continue
		}

		if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
			logger.Error("Failed to save JSON report",
				zap.String("strategy", test.name),
				zap.Error(err))
			continue
		}

		// Save HTML report
		htmlPath := filepath.Join(outputDir, fmt.Sprintf("%s_report.html", test.config.ID))
		htmlData, err := report.ToHTML()
		if err != nil {
			logger.Error("Failed to generate HTML report",
				zap.String("strategy", test.name),
				zap.Error(err))
			continue
		}

		if err := os.WriteFile(htmlPath, []byte(htmlData), 0644); err != nil {
			logger.Error("Failed to save HTML report",
				zap.String("strategy", test.name),
				zap.Error(err))
			continue
		}

		// Print summary
		printSummary(report, logger)

		logger.Info("Backtest completed",
			zap.String("strategy", test.name),
			zap.String("json_report", jsonPath),
			zap.String("html_report", htmlPath))
	}

	logger.Info("All backtests completed")
}

func printSummary(report *backtest.Report, logger *zap.Logger) {
	logger.Info("=== Backtest Summary ===",
		zap.String("strategy", report.StrategyName),
		zap.String("type", report.StrategyType),
		zap.String("period", fmt.Sprintf("%s to %s", 
			report.BacktestPeriod.Start.Format("2006-01-02"),
			report.BacktestPeriod.End.Format("2006-01-02"))))

	logger.Info("Performance Metrics",
		zap.String("total_return", report.Performance.TotalReturn),
		zap.String("annualized_return", report.Performance.AnnualizedReturn),
		zap.String("total_pnl", report.Performance.TotalPnL),
		zap.String("ending_equity", report.Performance.EndingEquity))

	logger.Info("Risk Metrics",
		zap.String("sharpe_ratio", report.Risk.SharpeRatio),
		zap.String("max_drawdown", report.Risk.MaxDrawdown),
		zap.String("volatility", report.Risk.Volatility),
		zap.String("var95", report.Risk.VaR95))

	logger.Info("Trading Statistics",
		zap.Int("total_trades", report.Trading.TotalTrades),
		zap.String("win_rate", report.Trading.WinRate),
		zap.String("profit_factor", report.Trading.ProfitFactor),
		zap.String("average_win", report.Trading.AverageWin),
		zap.String("average_loss", report.Trading.AverageLoss),
		zap.String("total_fees", report.Trading.TotalFees))

	// Show top 3 monthly returns
	if len(report.MonthlyReturns) > 0 {
		logger.Info("Recent Monthly Returns")
		for i, mr := range report.MonthlyReturns {
			if i >= 3 {
				break
			}
			logger.Info("Monthly return",
				zap.Int("year", mr.Year),
				zap.String("month", mr.Month),
				zap.String("return", mr.Return))
		}
	}
}