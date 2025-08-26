package optimization

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/mExOms/pkg/backtest"
	"github.com/mExOms/pkg/strategies"
)

// SensitivityResult represents sensitivity analysis for one parameter
type SensitivityResult struct {
	ParameterName   string
	BaseValue       interface{}
	TestedValues    []interface{}
	MetricValues    []decimal.Decimal
	Sensitivity     decimal.Decimal // Change in metric per unit change in parameter
	Stability       decimal.Decimal // Inverse of variance
	OptimalValue    interface{}
	OptimalMetric   decimal.Decimal
}

// SensitivityAnalyzer performs parameter sensitivity analysis
type SensitivityAnalyzer struct {
	strategy        strategies.Strategy
	backtestConfig  *backtest.BacktestConfig
	dataProvider    backtest.DataProvider
	baseParameters  map[string]interface{}
	metric          string
	logger          *zap.Logger
}

// NewSensitivityAnalyzer creates a new sensitivity analyzer
func NewSensitivityAnalyzer(
	strategy strategies.Strategy,
	backtestConfig *backtest.BacktestConfig,
	dataProvider backtest.DataProvider,
	baseParameters map[string]interface{},
	metric string,
	logger *zap.Logger,
) *SensitivityAnalyzer {
	return &SensitivityAnalyzer{
		strategy:       strategy,
		backtestConfig: backtestConfig,
		dataProvider:   dataProvider,
		baseParameters: baseParameters,
		metric:         metric,
		logger:         logger,
	}
}

// AnalyzeParameter analyzes sensitivity for a single parameter
func (a *SensitivityAnalyzer) AnalyzeParameter(
	ctx context.Context,
	paramName string,
	paramRange ParameterRange,
) (*SensitivityResult, error) {
	a.logger.Info("Analyzing parameter sensitivity",
		zap.String("parameter", paramName),
		zap.String("type", paramRange.Type))
	
	// Get test values
	testValues := a.getTestValues(paramRange)
	metricValues := make([]decimal.Decimal, 0, len(testValues))
	
	// Test each value
	for _, value := range testValues {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		
		// Create parameters with test value
		testParams := make(map[string]interface{})
		for k, v := range a.baseParameters {
			testParams[k] = v
		}
		testParams[paramName] = value
		
		// Run backtest
		metricValue, err := a.runBacktest(ctx, testParams)
		if err != nil {
			a.logger.Error("Backtest failed",
				zap.String("parameter", paramName),
				zap.Any("value", value),
				zap.Error(err))
			continue
		}
		
		metricValues = append(metricValues, metricValue)
	}
	
	// Calculate sensitivity metrics
	result := &SensitivityResult{
		ParameterName: paramName,
		BaseValue:     a.baseParameters[paramName],
		TestedValues:  testValues,
		MetricValues:  metricValues,
	}
	
	// Calculate sensitivity and stability
	a.calculateSensitivity(result, paramRange)
	a.calculateStability(result)
	a.findOptimal(result)
	
	return result, nil
}

// AnalyzeAllParameters analyzes sensitivity for all parameters
func (a *SensitivityAnalyzer) AnalyzeAllParameters(
	ctx context.Context,
	parameters []ParameterRange,
) (map[string]*SensitivityResult, error) {
	results := make(map[string]*SensitivityResult)
	
	for _, param := range parameters {
		result, err := a.AnalyzeParameter(ctx, param.Name, param)
		if err != nil {
			return nil, fmt.Errorf("failed to analyze %s: %w", param.Name, err)
		}
		results[param.Name] = result
	}
	
	return results, nil
}

// AnalyzeInteractions analyzes parameter interactions
func (a *SensitivityAnalyzer) AnalyzeInteractions(
	ctx context.Context,
	param1, param2 ParameterRange,
	gridSize int,
) ([][]decimal.Decimal, error) {
	a.logger.Info("Analyzing parameter interaction",
		zap.String("param1", param1.Name),
		zap.String("param2", param2.Name))
	
	// Create value grids
	values1 := a.getGridValues(param1, gridSize)
	values2 := a.getGridValues(param2, gridSize)
	
	// Result matrix
	results := make([][]decimal.Decimal, len(values1))
	for i := range results {
		results[i] = make([]decimal.Decimal, len(values2))
	}
	
	// Test combinations
	for i, val1 := range values1 {
		for j, val2 := range values2 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			
			// Create test parameters
			testParams := make(map[string]interface{})
			for k, v := range a.baseParameters {
				testParams[k] = v
			}
			testParams[param1.Name] = val1
			testParams[param2.Name] = val2
			
			// Run backtest
			metricValue, err := a.runBacktest(ctx, testParams)
			if err != nil {
				a.logger.Error("Interaction test failed",
					zap.String("param1", param1.Name),
					zap.Any("value1", val1),
					zap.String("param2", param2.Name),
					zap.Any("value2", val2),
					zap.Error(err))
				results[i][j] = decimal.NewFromFloat(-999999)
				continue
			}
			
			results[i][j] = metricValue
		}
	}
	
	return results, nil
}

// getTestValues returns values to test for a parameter
func (a *SensitivityAnalyzer) getTestValues(param ParameterRange) []interface{} {
	switch param.Type {
	case "float":
		// Test ±10%, ±20%, ±50% from base
		base := a.baseParameters[param.Name].(float64)
		return []interface{}{
			base * 0.5,
			base * 0.8,
			base * 0.9,
			base,
			base * 1.1,
			base * 1.2,
			base * 1.5,
		}
	case "int":
		// Similar for int
		base := a.baseParameters[param.Name].(int)
		return []interface{}{
			base / 2,
			int(float64(base) * 0.8),
			int(float64(base) * 0.9),
			base,
			int(float64(base) * 1.1),
			int(float64(base) * 1.2),
			base * 2,
		}
	case "bool":
		return []interface{}{true, false}
	case "enum":
		return param.Values
	default:
		return []interface{}{a.baseParameters[param.Name]}
	}
}

// getGridValues returns evenly spaced values for grid analysis
func (a *SensitivityAnalyzer) getGridValues(param ParameterRange, gridSize int) []interface{} {
	values := make([]interface{}, 0, gridSize)
	
	switch param.Type {
	case "float":
		step := (param.Max - param.Min) / float64(gridSize-1)
		for i := 0; i < gridSize; i++ {
			values = append(values, param.Min+float64(i)*step)
		}
	case "int":
		step := int((param.Max - param.Min) / float64(gridSize-1))
		if step < 1 {
			step = 1
		}
		for i := 0; i < gridSize; i++ {
			val := int(param.Min) + i*step
			if val > int(param.Max) {
				val = int(param.Max)
			}
			values = append(values, val)
		}
	case "enum":
		return param.Values
	default:
		values = append(values, a.baseParameters[param.Name])
	}
	
	return values
}

// runBacktest runs a single backtest with given parameters
func (a *SensitivityAnalyzer) runBacktest(ctx context.Context, params map[string]interface{}) (decimal.Decimal, error) {
	// Clone strategy
	strategy := a.cloneStrategy()
	
	// Update parameters
	if err := strategy.UpdateParameters(params); err != nil {
		return decimal.Zero, fmt.Errorf("failed to update parameters: %w", err)
	}
	
	// Reset data provider
	if err := a.dataProvider.Reset(); err != nil {
		return decimal.Zero, fmt.Errorf("failed to reset data provider: %w", err)
	}
	
	// Run backtest
	engine := backtest.NewBacktestEngine(
		a.backtestConfig,
		strategy,
		a.dataProvider,
		a.logger,
	)
	
	if err := engine.Run(ctx); err != nil {
		return decimal.Zero, fmt.Errorf("backtest failed: %w", err)
	}
	
	// Get metric value
	metrics := engine.GetMetrics()
	return a.getMetricValue(metrics), nil
}

// getMetricValue extracts the specified metric from backtest results
func (a *SensitivityAnalyzer) getMetricValue(metrics *backtest.BacktestMetrics) decimal.Decimal {
	switch a.metric {
	case "sharpe":
		return metrics.SharpeRatio
	case "return":
		return metrics.TotalReturn
	case "profit_factor":
		return metrics.ProfitFactor
	case "sortino":
		return metrics.SortinoRatio
	case "max_drawdown":
		return metrics.MaxDrawdown.Neg()
	default:
		return metrics.TotalReturn
	}
}

// calculateSensitivity calculates parameter sensitivity
func (a *SensitivityAnalyzer) calculateSensitivity(result *SensitivityResult, param ParameterRange) {
	if len(result.MetricValues) < 2 {
		result.Sensitivity = decimal.Zero
		return
	}
	
	// For numeric parameters, calculate slope
	if param.Type == "float" || param.Type == "int" {
		// Find indices of base value and nearby values
		var baseIdx int
		for i, val := range result.TestedValues {
			if val == result.BaseValue {
				baseIdx = i
				break
			}
		}
		
		// Calculate local sensitivity around base value
		if baseIdx > 0 && baseIdx < len(result.TestedValues)-1 {
			// Central difference
			leftVal := toFloat64(result.TestedValues[baseIdx-1])
			rightVal := toFloat64(result.TestedValues[baseIdx+1])
			leftMetric := result.MetricValues[baseIdx-1]
			rightMetric := result.MetricValues[baseIdx+1]
			
			deltaParam := rightVal - leftVal
			deltaMetric := rightMetric.Sub(leftMetric)
			
			if deltaParam != 0 {
				result.Sensitivity = deltaMetric.Div(decimal.NewFromFloat(deltaParam))
			}
		}
	} else {
		// For non-numeric, use range of metric values
		minMetric := result.MetricValues[0]
		maxMetric := result.MetricValues[0]
		
		for _, m := range result.MetricValues {
			if m.LessThan(minMetric) {
				minMetric = m
			}
			if m.GreaterThan(maxMetric) {
				maxMetric = m
			}
		}
		
		result.Sensitivity = maxMetric.Sub(minMetric)
	}
}

// calculateStability calculates parameter stability
func (a *SensitivityAnalyzer) calculateStability(result *SensitivityResult) {
	if len(result.MetricValues) < 2 {
		result.Stability = decimal.Zero
		return
	}
	
	// Calculate variance
	sum := decimal.Zero
	for _, m := range result.MetricValues {
		sum = sum.Add(m)
	}
	mean := sum.Div(decimal.NewFromInt(int64(len(result.MetricValues))))
	
	variance := decimal.Zero
	for _, m := range result.MetricValues {
		diff := m.Sub(mean)
		variance = variance.Add(diff.Mul(diff))
	}
	variance = variance.Div(decimal.NewFromInt(int64(len(result.MetricValues))))
	
	// Stability is inverse of coefficient of variation
	if !mean.IsZero() && !variance.IsZero() {
		cv := decimal.NewFromFloat(math.Sqrt(variance.InexactFloat64())).Div(mean.Abs())
		result.Stability = decimal.NewFromInt(1).Div(cv.Add(decimal.NewFromFloat(0.001))) // Avoid div by zero
	} else {
		result.Stability = decimal.NewFromInt(100) // Perfect stability if no variance
	}
}

// findOptimal finds the optimal parameter value
func (a *SensitivityAnalyzer) findOptimal(result *SensitivityResult) {
	if len(result.MetricValues) == 0 {
		return
	}
	
	// Find best metric value
	bestIdx := 0
	bestMetric := result.MetricValues[0]
	
	for i, m := range result.MetricValues {
		if m.GreaterThan(bestMetric) {
			bestIdx = i
			bestMetric = m
		}
	}
	
	result.OptimalValue = result.TestedValues[bestIdx]
	result.OptimalMetric = bestMetric
}

// cloneStrategy creates a copy of the strategy
func (a *SensitivityAnalyzer) cloneStrategy() strategies.Strategy {
	// Simplified - in production would properly clone
	return a.strategy
}

// toFloat64 converts interface to float64
func toFloat64(val interface{}) float64 {
	switch v := val.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case decimal.Decimal:
		return v.InexactFloat64()
	default:
		return 0
	}
}

// GenerateSensitivityReport generates a sensitivity analysis report
func GenerateSensitivityReport(results map[string]*SensitivityResult) string {
	report := "=== Sensitivity Analysis Report ===\n\n"
	
	// Sort parameters by sensitivity
	params := make([]string, 0, len(results))
	for param := range results {
		params = append(params, param)
	}
	
	sort.Slice(params, func(i, j int) bool {
		return results[params[i]].Sensitivity.Abs().GreaterThan(
			results[params[j]].Sensitivity.Abs())
	})
	
	report += "Parameter Rankings (by sensitivity):\n"
	report += fmt.Sprintf("%-20s | %-12s | %-12s | %-12s | %-12s\n",
		"Parameter", "Sensitivity", "Stability", "Base Value", "Optimal")
	report += "-------------------- | ------------ | ------------ | ------------ | ------------\n"
	
	for _, param := range params {
		result := results[param]
		report += fmt.Sprintf("%-20s | %12s | %12s | %12v | %12v\n",
			param,
			result.Sensitivity.StringFixed(4),
			result.Stability.StringFixed(2),
			result.BaseValue,
			result.OptimalValue)
	}
	
	report += "\nDetailed Results:\n\n"
	
	for _, param := range params {
		result := results[param]
		report += fmt.Sprintf("Parameter: %s\n", param)
		report += fmt.Sprintf("  Base Value: %v\n", result.BaseValue)
		report += fmt.Sprintf("  Optimal Value: %v (Metric: %s)\n", 
			result.OptimalValue, result.OptimalMetric.StringFixed(4))
		report += fmt.Sprintf("  Sensitivity: %s\n", result.Sensitivity.StringFixed(6))
		report += fmt.Sprintf("  Stability Score: %s\n", result.Stability.StringFixed(2))
		report += "  Tested Values and Metrics:\n"
		
		for i, val := range result.TestedValues {
			if i < len(result.MetricValues) {
				report += fmt.Sprintf("    %v -> %s\n", val, result.MetricValues[i].StringFixed(4))
			}
		}
		report += "\n"
	}
	
	return report
}