package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/mExOms/pkg/backtest"
	"github.com/mExOms/pkg/optimization"
	"github.com/mExOms/pkg/strategies"
	"github.com/mExOms/pkg/strategies/templates"
	"github.com/mExOms/pkg/types"
)

// Mock implementations (same as test-backtest)
type MockOrderManager struct {
	engine *backtest.BacktestEngine
}

func (m *MockOrderManager) PlaceOrder(req *types.OrderRequest) (*types.Order, error) {
	if m.engine != nil {
		return m.engine.PlaceOrder(req)
	}
	return nil, fmt.Errorf("engine not set")
}

func (m *MockOrderManager) CancelOrder(orderID string) error {
	if m.engine != nil {
		return m.engine.CancelOrder(orderID)
	}
	return fmt.Errorf("engine not set")
}

func (m *MockOrderManager) GetOrder(orderID string) (*types.Order, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *MockOrderManager) GetActiveOrders(symbol string) ([]*types.Order, error) {
	return nil, nil
}

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
	return decimal.Zero
}

type MockMarketDataProvider struct{}

func (m *MockMarketDataProvider) SubscribeTicker(symbol string) error { return nil }
func (m *MockMarketDataProvider) SubscribeOrderBook(symbol string) error { return nil }
func (m *MockMarketDataProvider) UnsubscribeTicker(symbol string) error { return nil }
func (m *MockMarketDataProvider) UnsubscribeOrderBook(symbol string) error { return nil }
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
	outputDir := "optimization_results"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		logger.Fatal("Failed to create output directory", zap.Error(err))
	}

	// Time period for testing
	startTime := time.Now().AddDate(0, -6, 0) // 6 months ago
	endTime := time.Now()

	// Base backtest configuration
	backtestConfig := &backtest.BacktestConfig{
		StartTime: startTime,
		EndTime:   endTime,
		InitialBalance: map[string]decimal.Decimal{
			"USDT": decimal.NewFromInt(100000),
		},
		TickInterval: time.Second,
		SpreadBps:    20,
		SlippageBps:  10,
		TakerFeeBps:  10,
		MakerFeeBps:  5,
		Symbols:      []string{"BTC-USDT"},
		Exchanges:    []string{"binance"},
	}

	// Test different optimization methods
	ctx := context.Background()

	// 1. Grid Search Optimization
	logger.Info("Starting Grid Search Optimization")
	runGridSearchOptimization(ctx, backtestConfig, outputDir, logger)

	// 2. Genetic Algorithm Optimization
	logger.Info("Starting Genetic Algorithm Optimization")
	runGeneticOptimization(ctx, backtestConfig, outputDir, logger)

	// 3. Walk-Forward Analysis
	logger.Info("Starting Walk-Forward Analysis")
	runWalkForwardAnalysis(ctx, startTime, endTime, outputDir, logger)

	// 4. Sensitivity Analysis
	logger.Info("Starting Sensitivity Analysis")
	runSensitivityAnalysis(ctx, backtestConfig, outputDir, logger)

	logger.Info("All optimizations completed")
}

func runGridSearchOptimization(
	ctx context.Context,
	backtestConfig *backtest.BacktestConfig,
	outputDir string,
	logger *zap.Logger,
) {
	// Create grid trading strategy
	strategy := templates.NewGridTemplate(
		&MockOrderManager{},
		NewMockPositionManager(),
		&MockMarketDataProvider{},
		logger,
	)

	// Initialize strategy
	strategyConfig := strategies.StrategyConfig{
		ID:        "grid-opt-001",
		Name:      "Grid Trading Optimization",
		Type:      strategies.StrategyTypeGrid,
		Symbols:   []string{"BTC-USDT"},
		Exchanges: []string{"binance"},
		Parameters: map[string]interface{}{
			"gridLevels":     10,
			"gridSpacing":    1.0,
			"upperBound":     50000.0,
			"lowerBound":     40000.0,
			"orderSize":      0.01,
			"reinvestProfit": true,
		},
	}
	strategy.Initialize(strategyConfig)

	// Define parameter ranges for optimization
	parameters := []optimization.ParameterRange{
		{
			Name: "gridLevels",
			Type: "int",
			Min:  5,
			Max:  20,
			Step: 5,
		},
		{
			Name: "gridSpacing",
			Type: "float",
			Min:  0.5,
			Max:  2.0,
			Step: 0.5,
		},
		{
			Name: "orderSize",
			Type: "float",
			Min:  0.005,
			Max:  0.02,
			Step: 0.005,
		},
	}

	// Create data provider
	dataProvider := backtest.NewSyntheticDataProvider(
		"BTC-USDT",
		backtestConfig.StartTime,
		backtestConfig.EndTime,
		decimal.NewFromInt(45000),
		0.5,
	)

	// Create optimization config
	optConfig := &optimization.OptimizationConfig{
		Strategy:         strategy,
		BacktestConfig:   backtestConfig,
		DataProvider:     dataProvider,
		Parameters:       parameters,
		MetricToOptimize: "sharpe",
		MaxIterations:    100,
		Threads:          4,
	}

	// Run grid search
	optimizer := optimization.NewOptimizer(optConfig, logger)
	result, err := optimizer.RunGridSearch(ctx)
	if err != nil {
		logger.Error("Grid search failed", zap.Error(err))
		return
	}

	// Save results
	if result != nil {
		saveOptimizationResult("grid_search", result, optimizer.GetAllResults(), outputDir, logger)
	}
}

func runGeneticOptimization(
	ctx context.Context,
	backtestConfig *backtest.BacktestConfig,
	outputDir string,
	logger *zap.Logger,
) {
	// Create market making strategy
	strategy := templates.NewMarketMakingTemplate(
		&MockOrderManager{},
		NewMockPositionManager(),
		&MockMarketDataProvider{},
		logger,
	)

	// Initialize strategy
	strategyConfig := strategies.StrategyConfig{
		ID:        "mm-opt-001",
		Name:      "Market Making Optimization",
		Type:      strategies.StrategyTypeMarketMaking,
		Symbols:   []string{"BTC-USDT"},
		Exchanges: []string{"binance"},
		Parameters: map[string]interface{}{
			"spreadBps":       50,
			"orderDepth":      5,
			"sizingMode":      "dynamic",
			"maxInventory":    1.0,
			"inventoryTarget": 0.5,
			"skewFactor":      0.1,
		},
	}
	strategy.Initialize(strategyConfig)

	// Define parameter ranges
	parameters := []optimization.ParameterRange{
		{
			Name: "spreadBps",
			Type: "int",
			Min:  20,
			Max:  100,
			Step: 10,
		},
		{
			Name: "orderDepth",
			Type: "int",
			Min:  3,
			Max:  10,
			Step: 1,
		},
		{
			Name: "skewFactor",
			Type: "float",
			Min:  0.05,
			Max:  0.3,
			Step: 0.05,
		},
		{
			Name: "maxInventory",
			Type: "float",
			Min:  0.5,
			Max:  2.0,
			Step: 0.25,
		},
	}

	// Create data provider
	dataProvider := backtest.NewSyntheticDataProvider(
		"BTC-USDT",
		backtestConfig.StartTime,
		backtestConfig.EndTime,
		decimal.NewFromInt(45000),
		0.5,
	)

	// Create optimization config
	optConfig := &optimization.OptimizationConfig{
		Strategy:         strategy,
		BacktestConfig:   backtestConfig,
		DataProvider:     dataProvider,
		Parameters:       parameters,
		MetricToOptimize: "profit_factor",
		MaxIterations:    50,
		PopulationSize:   30,
		CrossoverRate:    0.8,
		MutationRate:     0.1,
		EliteRatio:       0.1,
		Threads:          4,
	}

	// Run genetic algorithm
	optimizer := optimization.NewOptimizer(optConfig, logger)
	result, err := optimizer.RunGeneticAlgorithm(ctx)
	if err != nil {
		logger.Error("Genetic algorithm failed", zap.Error(err))
		return
	}

	// Save results
	if result != nil {
		saveOptimizationResult("genetic_algorithm", result, optimizer.GetAllResults(), outputDir, logger)
	}
}

func runWalkForwardAnalysis(
	ctx context.Context,
	startTime, endTime time.Time,
	outputDir string,
	logger *zap.Logger,
) {
	// Create arbitrage strategy
	strategy := templates.NewArbitrageTemplate(
		&MockOrderManager{},
		NewMockPositionManager(),
		&MockMarketDataProvider{},
		logger,
	)

	// Base parameters
	baseParams := map[string]interface{}{
		"minSpreadPercent": 0.5,
		"maxPositionSize":  1.0,
		"executionMode":    "aggressive",
	}

	// Initialize strategy
	strategyConfig := strategies.StrategyConfig{
		ID:         "arb-wf-001",
		Name:       "Arbitrage Walk-Forward",
		Type:       strategies.StrategyTypeArbitrage,
		Symbols:    []string{"BTC-USDT"},
		Exchanges:  []string{"binance", "bybit"},
		Parameters: baseParams,
	}
	strategy.Initialize(strategyConfig)

	// Define parameter ranges
	parameters := []optimization.ParameterRange{
		{
			Name: "minSpreadPercent",
			Type: "float",
			Min:  0.2,
			Max:  1.0,
			Step: 0.1,
		},
		{
			Name: "maxPositionSize",
			Type: "float",
			Min:  0.5,
			Max:  2.0,
			Step: 0.25,
		},
	}

	// Create data provider
	dataProvider := backtest.NewSyntheticDataProvider(
		"BTC-USDT",
		startTime,
		endTime,
		decimal.NewFromInt(45000),
		0.5,
	)

	// Backtest config template
	backtestConfig := &backtest.BacktestConfig{
		InitialBalance: map[string]decimal.Decimal{
			"USDT": decimal.NewFromInt(100000),
		},
		TickInterval: time.Second,
		SpreadBps:    20,
		SlippageBps:  10,
		TakerFeeBps:  10,
		MakerFeeBps:  5,
		Symbols:      []string{"BTC-USDT"},
		Exchanges:    []string{"binance", "bybit"},
	}

	// Optimization config for walk-forward
	optConfig := &optimization.OptimizationConfig{
		Strategy:         strategy,
		BacktestConfig:   backtestConfig,
		DataProvider:     dataProvider,
		Parameters:       parameters,
		MetricToOptimize: "sharpe",
		MaxIterations:    20,
		Threads:          2,
	}

	// Walk-forward config
	wfConfig := &optimization.WalkForwardConfig{
		Strategy:           strategy,
		DataProvider:       dataProvider,
		Parameters:         parameters,
		MetricToOptimize:   "sharpe",
		TotalPeriod:        endTime.Sub(startTime),
		OptimizationRatio:  0.7, // 70% for optimization, 30% for validation
		WindowSize:         30 * 24 * time.Hour, // 30 days
		StepSize:           7 * 24 * time.Hour,  // 7 days
		OptimizationType:   "grid",
		OptimizationConfig: optConfig,
	}

	// Run walk-forward analysis
	analyzer := optimization.NewWalkForwardAnalyzer(wfConfig, logger)
	results, err := analyzer.Run(ctx, startTime, endTime)
	if err != nil {
		logger.Error("Walk-forward analysis failed", zap.Error(err))
		return
	}

	// Generate and save report
	report := analyzer.GenerateReport()
	reportPath := filepath.Join(outputDir, "walk_forward_report.txt")
	if err := os.WriteFile(reportPath, []byte(report), 0644); err != nil {
		logger.Error("Failed to save walk-forward report", zap.Error(err))
		return
	}

	logger.Info("Walk-forward analysis complete",
		zap.Int("windows", len(results)),
		zap.String("robustness_score", analyzer.GetRobustnessScore().StringFixed(2)),
		zap.String("report", reportPath))
}

func runSensitivityAnalysis(
	ctx context.Context,
	backtestConfig *backtest.BacktestConfig,
	outputDir string,
	logger *zap.Logger,
) {
	// Create grid strategy for sensitivity analysis
	strategy := templates.NewGridTemplate(
		&MockOrderManager{},
		NewMockPositionManager(),
		&MockMarketDataProvider{},
		logger,
	)

	// Base parameters
	baseParams := map[string]interface{}{
		"gridLevels":     10,
		"gridSpacing":    1.0,
		"upperBound":     50000.0,
		"lowerBound":     40000.0,
		"orderSize":      0.01,
		"reinvestProfit": true,
	}

	// Initialize strategy
	strategyConfig := strategies.StrategyConfig{
		ID:         "grid-sens-001",
		Name:       "Grid Sensitivity Analysis",
		Type:       strategies.StrategyTypeGrid,
		Symbols:    []string{"BTC-USDT"},
		Exchanges:  []string{"binance"},
		Parameters: baseParams,
	}
	strategy.Initialize(strategyConfig)

	// Create data provider
	dataProvider := backtest.NewSyntheticDataProvider(
		"BTC-USDT",
		backtestConfig.StartTime,
		backtestConfig.EndTime,
		decimal.NewFromInt(45000),
		0.5,
	)

	// Create sensitivity analyzer
	analyzer := optimization.NewSensitivityAnalyzer(
		strategy,
		backtestConfig,
		dataProvider,
		baseParams,
		"sharpe",
		logger,
	)

	// Define parameters to analyze
	parameters := []optimization.ParameterRange{
		{
			Name: "gridLevels",
			Type: "int",
			Min:  5,
			Max:  20,
			Step: 1,
		},
		{
			Name: "gridSpacing",
			Type: "float",
			Min:  0.5,
			Max:  2.0,
			Step: 0.1,
		},
		{
			Name: "orderSize",
			Type: "float",
			Min:  0.005,
			Max:  0.02,
			Step: 0.001,
		},
	}

	// Run sensitivity analysis
	results, err := analyzer.AnalyzeAllParameters(ctx, parameters)
	if err != nil {
		logger.Error("Sensitivity analysis failed", zap.Error(err))
		return
	}

	// Generate and save report
	report := optimization.GenerateSensitivityReport(results)
	reportPath := filepath.Join(outputDir, "sensitivity_report.txt")
	if err := os.WriteFile(reportPath, []byte(report), 0644); err != nil {
		logger.Error("Failed to save sensitivity report", zap.Error(err))
		return
	}

	logger.Info("Sensitivity analysis complete",
		zap.Int("parameters", len(results)),
		zap.String("report", reportPath))

	// Analyze parameter interactions
	logger.Info("Analyzing parameter interactions")
	interactionResults, err := analyzer.AnalyzeInteractions(
		ctx,
		parameters[0], // gridLevels
		parameters[1], // gridSpacing
		5,             // 5x5 grid
	)
	if err != nil {
		logger.Error("Interaction analysis failed", zap.Error(err))
		return
	}

	// Save interaction results
	interactionReport := fmt.Sprintf("Parameter Interaction: %s vs %s\n\n", 
		parameters[0].Name, parameters[1].Name)
	for i, row := range interactionResults {
		for j, val := range row {
			interactionReport += fmt.Sprintf("%8s ", val.StringFixed(2))
		}
		interactionReport += "\n"
	}

	interactionPath := filepath.Join(outputDir, "interaction_analysis.txt")
	if err := os.WriteFile(interactionPath, []byte(interactionReport), 0644); err != nil {
		logger.Error("Failed to save interaction report", zap.Error(err))
		return
	}
}

func saveOptimizationResult(
	method string,
	best *optimization.OptimizationResult,
	all []optimization.OptimizationResult,
	outputDir string,
	logger *zap.Logger,
) {
	// Create report
	report := fmt.Sprintf("=== %s Optimization Results ===\n\n", method)
	report += fmt.Sprintf("Best Parameters Found:\n")
	for k, v := range best.Parameters {
		report += fmt.Sprintf("  %s: %v\n", k, v)
	}
	report += fmt.Sprintf("\nBest Metric Value: %s\n", best.MetricValue.StringFixed(4))
	report += fmt.Sprintf("Timestamp: %s\n", best.Timestamp.Format(time.RFC3339))

	if best.BacktestMetrics != nil {
		report += fmt.Sprintf("\nBacktest Metrics:\n")
		report += fmt.Sprintf("  Total Return: %s\n", best.BacktestMetrics.TotalReturn.StringFixed(4))
		report += fmt.Sprintf("  Sharpe Ratio: %s\n", best.BacktestMetrics.SharpeRatio.StringFixed(4))
		report += fmt.Sprintf("  Max Drawdown: %s\n", best.BacktestMetrics.MaxDrawdown.StringFixed(4))
		report += fmt.Sprintf("  Win Rate: %.2f%%\n", best.BacktestMetrics.WinRate*100)
		report += fmt.Sprintf("  Profit Factor: %s\n", best.BacktestMetrics.ProfitFactor.StringFixed(2))
	}

	report += fmt.Sprintf("\nTotal Evaluations: %d\n", len(all))

	// Save report
	filename := fmt.Sprintf("%s_results.txt", method)
	reportPath := filepath.Join(outputDir, filename)
	if err := os.WriteFile(reportPath, []byte(report), 0644); err != nil {
		logger.Error("Failed to save optimization report", 
			zap.String("method", method),
			zap.Error(err))
		return
	}

	logger.Info("Optimization results saved",
		zap.String("method", method),
		zap.String("file", reportPath),
		zap.String("best_metric", best.MetricValue.StringFixed(4)))
}