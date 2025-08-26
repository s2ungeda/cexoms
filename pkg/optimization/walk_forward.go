package optimization

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/mExOms/pkg/backtest"
	"github.com/mExOms/pkg/strategies"
)

// WalkForwardConfig holds walk-forward analysis configuration
type WalkForwardConfig struct {
	Strategy          strategies.Strategy
	DataProvider      backtest.DataProvider
	Parameters        []ParameterRange
	MetricToOptimize  string
	
	// Time windows
	TotalPeriod       time.Duration
	OptimizationRatio float64 // e.g., 0.8 = 80% for optimization, 20% for validation
	WindowSize        time.Duration
	StepSize          time.Duration
	
	// Optimization settings
	OptimizationType  string // "grid", "genetic", "bayesian"
	OptimizationConfig *OptimizationConfig
}

// WalkForwardResult represents result of one walk-forward window
type WalkForwardResult struct {
	Window            int
	OptimizationStart time.Time
	OptimizationEnd   time.Time
	ValidationStart   time.Time
	ValidationEnd     time.Time
	
	// Best parameters from optimization
	OptimalParameters map[string]interface{}
	OptimizationMetric decimal.Decimal
	
	// Validation results
	ValidationMetrics *backtest.BacktestMetrics
	ValidationReturn  decimal.Decimal
	ValidationSharpe  decimal.Decimal
	
	// Out-of-sample performance
	IsOverfit bool
}

// WalkForwardAnalyzer performs walk-forward optimization
type WalkForwardAnalyzer struct {
	config  *WalkForwardConfig
	logger  *zap.Logger
	results []WalkForwardResult
}

// NewWalkForwardAnalyzer creates a new walk-forward analyzer
func NewWalkForwardAnalyzer(config *WalkForwardConfig, logger *zap.Logger) *WalkForwardAnalyzer {
	return &WalkForwardAnalyzer{
		config:  config,
		logger:  logger,
		results: make([]WalkForwardResult, 0),
	}
}

// Run performs walk-forward analysis
func (w *WalkForwardAnalyzer) Run(ctx context.Context, startTime, endTime time.Time) ([]WalkForwardResult, error) {
	w.logger.Info("Starting walk-forward analysis",
		zap.Time("start", startTime),
		zap.Time("end", endTime),
		zap.Duration("window_size", w.config.WindowSize),
		zap.Duration("step_size", w.config.StepSize))
	
	windowCount := 0
	currentStart := startTime
	
	for currentStart.Add(w.config.WindowSize).Before(endTime) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		
		windowCount++
		
		// Calculate optimization and validation periods
		windowEnd := currentStart.Add(w.config.WindowSize)
		if windowEnd.After(endTime) {
			windowEnd = endTime
		}
		
		optimizationDuration := time.Duration(float64(windowEnd.Sub(currentStart)) * w.config.OptimizationRatio)
		optimizationEnd := currentStart.Add(optimizationDuration)
		
		w.logger.Info("Processing walk-forward window",
			zap.Int("window", windowCount),
			zap.Time("opt_start", currentStart),
			zap.Time("opt_end", optimizationEnd),
			zap.Time("val_start", optimizationEnd),
			zap.Time("val_end", windowEnd))
		
		// Run optimization on in-sample period
		optimalParams, optMetric, err := w.optimizeWindow(ctx, currentStart, optimizationEnd)
		if err != nil {
			w.logger.Error("Optimization failed",
				zap.Int("window", windowCount),
				zap.Error(err))
			currentStart = currentStart.Add(w.config.StepSize)
			continue
		}
		
		// Validate on out-of-sample period
		valMetrics, err := w.validateParameters(ctx, optimalParams, optimizationEnd, windowEnd)
		if err != nil {
			w.logger.Error("Validation failed",
				zap.Int("window", windowCount),
				zap.Error(err))
			currentStart = currentStart.Add(w.config.StepSize)
			continue
		}
		
		// Check for overfitting
		isOverfit := w.detectOverfitting(optMetric, valMetrics)
		
		result := WalkForwardResult{
			Window:             windowCount,
			OptimizationStart:  currentStart,
			OptimizationEnd:    optimizationEnd,
			ValidationStart:    optimizationEnd,
			ValidationEnd:      windowEnd,
			OptimalParameters:  optimalParams,
			OptimizationMetric: optMetric,
			ValidationMetrics:  valMetrics,
			ValidationReturn:   valMetrics.TotalReturn,
			ValidationSharpe:   valMetrics.SharpeRatio,
			IsOverfit:          isOverfit,
		}
		
		w.results = append(w.results, result)
		
		// Move to next window
		currentStart = currentStart.Add(w.config.StepSize)
	}
	
	// Calculate summary statistics
	w.calculateSummaryStats()
	
	return w.results, nil
}

// optimizeWindow runs optimization for a specific time window
func (w *WalkForwardAnalyzer) optimizeWindow(ctx context.Context, startTime, endTime time.Time) (map[string]interface{}, decimal.Decimal, error) {
	// Create optimization config for this window
	optConfig := *w.config.OptimizationConfig
	optConfig.BacktestConfig.StartTime = startTime
	optConfig.BacktestConfig.EndTime = endTime
	
	// Create optimizer
	optimizer := NewOptimizer(&optConfig, w.logger)
	
	// Run optimization based on type
	var result *OptimizationResult
	var err error
	
	switch w.config.OptimizationType {
	case "grid":
		result, err = optimizer.RunGridSearch(ctx)
	case "genetic":
		result, err = optimizer.RunGeneticAlgorithm(ctx)
	case "bayesian":
		result, err = optimizer.RunBayesianOptimization(ctx)
	default:
		return nil, decimal.Zero, fmt.Errorf("unknown optimization type: %s", w.config.OptimizationType)
	}
	
	if err != nil {
		return nil, decimal.Zero, err
	}
	
	if result == nil {
		return nil, decimal.Zero, fmt.Errorf("optimization returned no result")
	}
	
	return result.Parameters, result.MetricValue, nil
}

// validateParameters runs backtest with parameters on validation period
func (w *WalkForwardAnalyzer) validateParameters(ctx context.Context, params map[string]interface{}, startTime, endTime time.Time) (*backtest.BacktestMetrics, error) {
	// Clone strategy
	strategy := w.cloneStrategy()
	
	// Update parameters
	if err := strategy.UpdateParameters(params); err != nil {
		return nil, fmt.Errorf("failed to update parameters: %w", err)
	}
	
	// Create backtest config for validation period
	backtestConfig := *w.config.OptimizationConfig.BacktestConfig
	backtestConfig.StartTime = startTime
	backtestConfig.EndTime = endTime
	
	// Reset data provider
	if err := w.config.DataProvider.Reset(); err != nil {
		return nil, fmt.Errorf("failed to reset data provider: %w", err)
	}
	
	// Run backtest
	engine := backtest.NewBacktestEngine(
		&backtestConfig,
		strategy,
		w.config.DataProvider,
		w.logger,
	)
	
	if err := engine.Run(ctx); err != nil {
		return nil, fmt.Errorf("validation backtest failed: %w", err)
	}
	
	return engine.GetMetrics(), nil
}

// detectOverfitting checks if strategy is overfit
func (w *WalkForwardAnalyzer) detectOverfitting(optMetric decimal.Decimal, valMetrics *backtest.BacktestMetrics) bool {
	// Get validation metric value
	var valMetric decimal.Decimal
	switch w.config.MetricToOptimize {
	case "sharpe":
		valMetric = valMetrics.SharpeRatio
	case "return":
		valMetric = valMetrics.TotalReturn
	case "profit_factor":
		valMetric = valMetrics.ProfitFactor
	default:
		valMetric = valMetrics.TotalReturn
	}
	
	// Check if validation performance is significantly worse
	if optMetric.IsPositive() && valMetric.IsNegative() {
		return true
	}
	
	// Check if validation metric is less than 50% of optimization metric
	if optMetric.IsPositive() {
		ratio := valMetric.Div(optMetric)
		if ratio.LessThan(decimal.NewFromFloat(0.5)) {
			return true
		}
	}
	
	return false
}

// calculateSummaryStats calculates summary statistics across all windows
func (w *WalkForwardAnalyzer) calculateSummaryStats() {
	if len(w.results) == 0 {
		return
	}
	
	// Calculate average validation performance
	var totalReturn, totalSharpe decimal.Decimal
	var overfitCount int
	
	for _, result := range w.results {
		totalReturn = totalReturn.Add(result.ValidationReturn)
		totalSharpe = totalSharpe.Add(result.ValidationSharpe)
		if result.IsOverfit {
			overfitCount++
		}
	}
	
	avgReturn := totalReturn.Div(decimal.NewFromInt(int64(len(w.results))))
	avgSharpe := totalSharpe.Div(decimal.NewFromInt(int64(len(w.results))))
	overfitRate := float64(overfitCount) / float64(len(w.results))
	
	w.logger.Info("Walk-forward analysis complete",
		zap.Int("total_windows", len(w.results)),
		zap.String("avg_validation_return", avgReturn.String()),
		zap.String("avg_validation_sharpe", avgSharpe.String()),
		zap.Float64("overfit_rate", overfitRate))
}

// GetResults returns all walk-forward results
func (w *WalkForwardAnalyzer) GetResults() []WalkForwardResult {
	results := make([]WalkForwardResult, len(w.results))
	copy(results, w.results)
	return results
}

// GetRobustnessScore calculates strategy robustness score
func (w *WalkForwardAnalyzer) GetRobustnessScore() decimal.Decimal {
	if len(w.results) == 0 {
		return decimal.Zero
	}
	
	// Score based on:
	// 1. Consistency of returns
	// 2. Low overfit rate
	// 3. Positive validation performance
	
	var positiveWindows int
	var returns []float64
	
	for _, result := range w.results {
		if result.ValidationReturn.IsPositive() {
			positiveWindows++
		}
		returns = append(returns, result.ValidationReturn.InexactFloat64())
	}
	
	// Calculate consistency (1 - coefficient of variation)
	mean, stddev := calculateMeanStddev(returns)
	consistency := 1.0
	if mean > 0 {
		consistency = 1.0 - (stddev / mean)
		if consistency < 0 {
			consistency = 0
		}
	}
	
	// Calculate scores
	winRate := float64(positiveWindows) / float64(len(w.results))
	overfitRate := 0.0
	for _, result := range w.results {
		if result.IsOverfit {
			overfitRate += 1.0
		}
	}
	overfitRate /= float64(len(w.results))
	
	// Combined score (0-100)
	score := (winRate*0.4 + consistency*0.4 + (1-overfitRate)*0.2) * 100
	
	return decimal.NewFromFloat(score)
}

// cloneStrategy creates a copy of the strategy
func (w *WalkForwardAnalyzer) cloneStrategy() strategies.Strategy {
	// Simplified - in production would properly clone
	return w.config.Strategy
}

// calculateMeanStddev calculates mean and standard deviation
func calculateMeanStddev(values []float64) (mean, stddev float64) {
	if len(values) == 0 {
		return 0, 0
	}
	
	// Calculate mean
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean = sum / float64(len(values))
	
	// Calculate stddev
	sumSq := 0.0
	for _, v := range values {
		diff := v - mean
		sumSq += diff * diff
	}
	variance := sumSq / float64(len(values))
	stddev = math.Sqrt(variance)
	
	return
}

// GenerateReport generates a walk-forward analysis report
func (w *WalkForwardAnalyzer) GenerateReport() string {
	report := "=== Walk-Forward Analysis Report ===\n\n"
	
	report += fmt.Sprintf("Total Windows: %d\n", len(w.results))
	report += fmt.Sprintf("Window Size: %s\n", w.config.WindowSize)
	report += fmt.Sprintf("Step Size: %s\n", w.config.StepSize)
	report += fmt.Sprintf("Optimization Ratio: %.1f%%\n\n", w.config.OptimizationRatio*100)
	
	report += "Window Results:\n"
	report += "Window | Opt Period | Val Period | Opt Metric | Val Return | Val Sharpe | Overfit\n"
	report += "-------|------------|------------|------------|------------|------------|--------\n"
	
	for _, result := range w.results {
		overfitStr := "No"
		if result.IsOverfit {
			overfitStr = "Yes"
		}
		
		report += fmt.Sprintf("%6d | %10s | %10s | %10s | %10s | %10s | %7s\n",
			result.Window,
			result.OptimizationStart.Format("2006-01-02"),
			result.ValidationStart.Format("2006-01-02"),
			result.OptimizationMetric.StringFixed(4),
			result.ValidationReturn.StringFixed(4),
			result.ValidationSharpe.StringFixed(4),
			overfitStr)
	}
	
	report += "\nRobustness Score: " + w.GetRobustnessScore().StringFixed(2) + "/100\n"
	
	return report
}