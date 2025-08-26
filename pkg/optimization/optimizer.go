package optimization

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/mExOms/pkg/backtest"
	"github.com/mExOms/pkg/strategies"
)

// OptimizationResult represents the result of a parameter optimization run
type OptimizationResult struct {
	Parameters      map[string]interface{}
	MetricValue     decimal.Decimal
	BacktestMetrics *backtest.BacktestMetrics
	Timestamp       time.Time
}

// ParameterRange defines the range for a parameter
type ParameterRange struct {
	Name     string
	Type     string // "float", "int", "bool", "enum"
	Min      float64
	Max      float64
	Step     float64
	Values   []interface{} // For enum type
}

// OptimizationConfig holds optimization configuration
type OptimizationConfig struct {
	Strategy        strategies.Strategy
	BacktestConfig  *backtest.BacktestConfig
	DataProvider    backtest.DataProvider
	Parameters      []ParameterRange
	MetricToOptimize string // "sharpe", "return", "profit_factor", etc.
	MaxIterations   int
	PopulationSize  int
	CrossoverRate   float64
	MutationRate    float64
	EliteRatio      float64
	Threads         int
}

// Optimizer performs parameter optimization
type Optimizer struct {
	config *OptimizationConfig
	logger *zap.Logger
	
	// Results tracking
	bestResult     *OptimizationResult
	allResults     []OptimizationResult
	mu             sync.RWMutex
	
	// Progress tracking
	currentIteration int
	startTime        time.Time
}

// NewOptimizer creates a new optimizer
func NewOptimizer(config *OptimizationConfig, logger *zap.Logger) *Optimizer {
	if config.Threads <= 0 {
		config.Threads = 4
	}
	if config.PopulationSize <= 0 {
		config.PopulationSize = 50
	}
	if config.CrossoverRate <= 0 {
		config.CrossoverRate = 0.8
	}
	if config.MutationRate <= 0 {
		config.MutationRate = 0.1
	}
	if config.EliteRatio <= 0 {
		config.EliteRatio = 0.1
	}
	
	return &Optimizer{
		config:     config,
		logger:     logger,
		allResults: make([]OptimizationResult, 0),
	}
}

// RunGridSearch performs grid search optimization
func (o *Optimizer) RunGridSearch(ctx context.Context) (*OptimizationResult, error) {
	o.startTime = time.Now()
	o.logger.Info("Starting grid search optimization",
		zap.Int("parameters", len(o.config.Parameters)))
	
	// Generate all parameter combinations
	combinations := o.generateGridCombinations()
	o.logger.Info("Generated parameter combinations",
		zap.Int("total", len(combinations)))
	
	// Evaluate combinations in parallel
	results := make(chan OptimizationResult, len(combinations))
	errors := make(chan error, len(combinations))
	
	// Worker pool
	workChan := make(chan map[string]interface{}, len(combinations))
	var wg sync.WaitGroup
	
	// Start workers
	for i := 0; i < o.config.Threads; i++ {
		wg.Add(1)
		go o.gridSearchWorker(ctx, workChan, results, errors, &wg)
	}
	
	// Send work
	for _, params := range combinations {
		select {
		case <-ctx.Done():
			close(workChan)
			return nil, ctx.Err()
		case workChan <- params:
		}
	}
	close(workChan)
	
	// Wait for completion
	wg.Wait()
	close(results)
	close(errors)
	
	// Collect results
	for result := range results {
		o.addResult(result)
	}
	
	// Check for errors
	for err := range errors {
		if err != nil {
			o.logger.Error("Grid search worker error", zap.Error(err))
		}
	}
	
	return o.getBestResult(), nil
}

// RunGeneticAlgorithm performs genetic algorithm optimization
func (o *Optimizer) RunGeneticAlgorithm(ctx context.Context) (*OptimizationResult, error) {
	o.startTime = time.Now()
	o.logger.Info("Starting genetic algorithm optimization",
		zap.Int("population", o.config.PopulationSize),
		zap.Int("generations", o.config.MaxIterations))
	
	// Initialize population
	population := o.initializePopulation()
	
	for generation := 0; generation < o.config.MaxIterations; generation++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		
		o.currentIteration = generation
		
		// Evaluate fitness
		fitness := o.evaluatePopulation(ctx, population)
		if len(fitness) == 0 {
			continue
		}
		
		// Sort by fitness
		sort.Slice(population, func(i, j int) bool {
			return fitness[i].GreaterThan(fitness[j])
		})
		
		// Track best
		if generation == 0 || fitness[0].GreaterThan(o.bestResult.MetricValue) {
			result := o.evaluateParameters(ctx, population[0])
			if result != nil {
				o.addResult(*result)
			}
		}
		
		// Select elite
		eliteSize := int(float64(len(population)) * o.config.EliteRatio)
		if eliteSize < 1 {
			eliteSize = 1
		}
		newPopulation := make([]map[string]interface{}, eliteSize)
		copy(newPopulation, population[:eliteSize])
		
		// Crossover and mutation
		for len(newPopulation) < o.config.PopulationSize {
			parent1 := o.tournamentSelection(population, fitness)
			parent2 := o.tournamentSelection(population, fitness)
			
			child := o.crossover(parent1, parent2)
			child = o.mutate(child)
			
			newPopulation = append(newPopulation, child)
		}
		
		population = newPopulation
		
		// Log progress
		if generation%10 == 0 {
			o.logger.Info("Genetic algorithm progress",
				zap.Int("generation", generation),
				zap.String("best_fitness", o.bestResult.MetricValue.String()))
		}
	}
	
	return o.getBestResult(), nil
}

// RunBayesianOptimization performs Bayesian optimization
func (o *Optimizer) RunBayesianOptimization(ctx context.Context) (*OptimizationResult, error) {
	o.startTime = time.Now()
	o.logger.Info("Starting Bayesian optimization")
	
	// Initialize with random samples
	initialSamples := 20
	if initialSamples > o.config.MaxIterations/2 {
		initialSamples = o.config.MaxIterations / 2
	}
	
	// Random sampling phase
	for i := 0; i < initialSamples; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		
		params := o.randomParameters()
		result := o.evaluateParameters(ctx, params)
		if result != nil {
			o.addResult(*result)
		}
	}
	
	// Acquisition function optimization
	for i := initialSamples; i < o.config.MaxIterations; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		
		o.currentIteration = i
		
		// Get next point using acquisition function
		nextParams := o.acquisitionFunction()
		result := o.evaluateParameters(ctx, nextParams)
		if result != nil {
			o.addResult(*result)
		}
		
		// Log progress
		if i%10 == 0 {
			o.logger.Info("Bayesian optimization progress",
				zap.Int("iteration", i),
				zap.String("best_value", o.bestResult.MetricValue.String()))
		}
	}
	
	return o.getBestResult(), nil
}

// generateGridCombinations generates all parameter combinations for grid search
func (o *Optimizer) generateGridCombinations() []map[string]interface{} {
	combinations := []map[string]interface{}{make(map[string]interface{})}
	
	for _, param := range o.config.Parameters {
		newCombinations := []map[string]interface{}{}
		
		values := o.getParameterValues(param)
		for _, combo := range combinations {
			for _, value := range values {
				newCombo := make(map[string]interface{})
				for k, v := range combo {
					newCombo[k] = v
				}
				newCombo[param.Name] = value
				newCombinations = append(newCombinations, newCombo)
			}
		}
		combinations = newCombinations
	}
	
	return combinations
}

// getParameterValues returns all values for a parameter
func (o *Optimizer) getParameterValues(param ParameterRange) []interface{} {
	values := []interface{}{}
	
	switch param.Type {
	case "float":
		for v := param.Min; v <= param.Max; v += param.Step {
			values = append(values, v)
		}
	case "int":
		for v := int(param.Min); v <= int(param.Max); v += int(param.Step) {
			values = append(values, v)
		}
	case "bool":
		values = append(values, true, false)
	case "enum":
		values = param.Values
	}
	
	return values
}

// gridSearchWorker processes parameter combinations
func (o *Optimizer) gridSearchWorker(
	ctx context.Context,
	work <-chan map[string]interface{},
	results chan<- OptimizationResult,
	errors chan<- error,
	wg *sync.WaitGroup,
) {
	defer wg.Done()
	
	for params := range work {
		select {
		case <-ctx.Done():
			return
		default:
		}
		
		result := o.evaluateParameters(ctx, params)
		if result != nil {
			results <- *result
		}
	}
}

// evaluateParameters runs backtest with given parameters
func (o *Optimizer) evaluateParameters(ctx context.Context, params map[string]interface{}) *OptimizationResult {
	// Clone strategy
	strategy := o.cloneStrategy()
	
	// Update parameters
	if err := strategy.UpdateParameters(params); err != nil {
		o.logger.Error("Failed to update parameters",
			zap.Error(err),
			zap.Any("params", params))
		return nil
	}
	
	// Reset data provider
	if err := o.config.DataProvider.Reset(); err != nil {
		o.logger.Error("Failed to reset data provider", zap.Error(err))
		return nil
	}
	
	// Run backtest
	engine := backtest.NewBacktestEngine(
		o.config.BacktestConfig,
		strategy,
		o.config.DataProvider,
		o.logger,
	)
	
	if err := engine.Run(ctx); err != nil {
		o.logger.Error("Backtest failed",
			zap.Error(err),
			zap.Any("params", params))
		return nil
	}
	
	// Get metrics
	metrics := engine.GetMetrics()
	metricValue := o.getMetricValue(metrics)
	
	return &OptimizationResult{
		Parameters:      params,
		MetricValue:     metricValue,
		BacktestMetrics: metrics,
		Timestamp:       time.Now(),
	}
}

// getMetricValue extracts the optimization metric from backtest results
func (o *Optimizer) getMetricValue(metrics *backtest.BacktestMetrics) decimal.Decimal {
	switch o.config.MetricToOptimize {
	case "sharpe":
		return metrics.SharpeRatio
	case "return":
		return metrics.TotalReturn
	case "profit_factor":
		return metrics.ProfitFactor
	case "sortino":
		return metrics.SortinoRatio
	case "calmar":
		// Calmar ratio = Annualized Return / Max Drawdown
		if metrics.MaxDrawdown.IsZero() {
			return decimal.Zero
		}
		return metrics.AnnualizedReturn.Div(metrics.MaxDrawdown)
	default:
		return metrics.TotalReturn
	}
}

// initializePopulation creates initial population for genetic algorithm
func (o *Optimizer) initializePopulation() []map[string]interface{} {
	population := make([]map[string]interface{}, o.config.PopulationSize)
	
	for i := range population {
		population[i] = o.randomParameters()
	}
	
	return population
}

// randomParameters generates random parameter set
func (o *Optimizer) randomParameters() map[string]interface{} {
	params := make(map[string]interface{})
	
	for _, param := range o.config.Parameters {
		switch param.Type {
		case "float":
			value := param.Min + rand.Float64()*(param.Max-param.Min)
			params[param.Name] = value
		case "int":
			value := int(param.Min) + rand.Intn(int(param.Max-param.Min)+1)
			params[param.Name] = value
		case "bool":
			params[param.Name] = rand.Float64() > 0.5
		case "enum":
			if len(param.Values) > 0 {
				params[param.Name] = param.Values[rand.Intn(len(param.Values))]
			}
		}
	}
	
	return params
}

// evaluatePopulation evaluates fitness for entire population
func (o *Optimizer) evaluatePopulation(ctx context.Context, population []map[string]interface{}) []decimal.Decimal {
	fitness := make([]decimal.Decimal, len(population))
	var wg sync.WaitGroup
	
	// Parallel evaluation
	semaphore := make(chan struct{}, o.config.Threads)
	
	for i, params := range population {
		wg.Add(1)
		go func(idx int, p map[string]interface{}) {
			defer wg.Done()
			
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			
			result := o.evaluateParameters(ctx, p)
			if result != nil {
				fitness[idx] = result.MetricValue
			} else {
				fitness[idx] = decimal.NewFromFloat(-math.Inf(1))
			}
		}(i, params)
	}
	
	wg.Wait()
	return fitness
}

// tournamentSelection selects individual using tournament selection
func (o *Optimizer) tournamentSelection(population []map[string]interface{}, fitness []decimal.Decimal) map[string]interface{} {
	tournamentSize := 3
	
	best := rand.Intn(len(population))
	bestFitness := fitness[best]
	
	for i := 1; i < tournamentSize; i++ {
		candidate := rand.Intn(len(population))
		if fitness[candidate].GreaterThan(bestFitness) {
			best = candidate
			bestFitness = fitness[candidate]
		}
	}
	
	return population[best]
}

// crossover performs crossover between two parents
func (o *Optimizer) crossover(parent1, parent2 map[string]interface{}) map[string]interface{} {
	if rand.Float64() > o.config.CrossoverRate {
		// No crossover, return copy of parent1
		child := make(map[string]interface{})
		for k, v := range parent1 {
			child[k] = v
		}
		return child
	}
	
	child := make(map[string]interface{})
	
	for _, param := range o.config.Parameters {
		if rand.Float64() < 0.5 {
			child[param.Name] = parent1[param.Name]
		} else {
			child[param.Name] = parent2[param.Name]
		}
	}
	
	return child
}

// mutate performs mutation on individual
func (o *Optimizer) mutate(individual map[string]interface{}) map[string]interface{} {
	mutated := make(map[string]interface{})
	for k, v := range individual {
		mutated[k] = v
	}
	
	for _, param := range o.config.Parameters {
		if rand.Float64() < o.config.MutationRate {
			switch param.Type {
			case "float":
				// Gaussian mutation
				current := mutated[param.Name].(float64)
				stddev := (param.Max - param.Min) * 0.1
				mutated[param.Name] = math.Max(param.Min, 
					math.Min(param.Max, current+rand.NormFloat64()*stddev))
			case "int":
				// Random reset
				mutated[param.Name] = int(param.Min) + rand.Intn(int(param.Max-param.Min)+1)
			case "bool":
				// Flip
				mutated[param.Name] = !mutated[param.Name].(bool)
			case "enum":
				// Random selection
				if len(param.Values) > 0 {
					mutated[param.Name] = param.Values[rand.Intn(len(param.Values))]
				}
			}
		}
	}
	
	return mutated
}

// acquisitionFunction selects next point for Bayesian optimization
func (o *Optimizer) acquisitionFunction() map[string]interface{} {
	// Simple implementation: Upper Confidence Bound (UCB)
	// In production, would use Gaussian Process for better modeling
	
	o.mu.RLock()
	results := make([]OptimizationResult, len(o.allResults))
	copy(results, o.allResults)
	o.mu.RUnlock()
	
	if len(results) == 0 {
		return o.randomParameters()
	}
	
	// Calculate mean and std of observed points
	var sum, sumSq decimal.Decimal
	for _, r := range results {
		sum = sum.Add(r.MetricValue)
		sumSq = sumSq.Add(r.MetricValue.Mul(r.MetricValue))
	}
	
	n := decimal.NewFromInt(int64(len(results)))
	mean := sum.Div(n)
	variance := sumSq.Div(n).Sub(mean.Mul(mean))
	std := decimal.NewFromFloat(math.Sqrt(variance.InexactFloat64()))
	
	// Generate candidates and select best UCB
	bestParams := o.randomParameters()
	bestUCB := decimal.NewFromFloat(-math.Inf(1))
	
	for i := 0; i < 100; i++ {
		candidate := o.randomParameters()
		
		// Estimate UCB (simplified - assumes independence)
		exploration := std.Mul(decimal.NewFromFloat(2.0))
		ucb := mean.Add(exploration)
		
		if ucb.GreaterThan(bestUCB) {
			bestUCB = ucb
			bestParams = candidate
		}
	}
	
	return bestParams
}

// cloneStrategy creates a copy of the strategy
func (o *Optimizer) cloneStrategy() strategies.Strategy {
	// This is simplified - in production would properly clone
	return o.config.Strategy
}

// addResult adds a result to tracking
func (o *Optimizer) addResult(result OptimizationResult) {
	o.mu.Lock()
	defer o.mu.Unlock()
	
	o.allResults = append(o.allResults, result)
	
	if o.bestResult == nil || result.MetricValue.GreaterThan(o.bestResult.MetricValue) {
		o.bestResult = &result
		o.logger.Info("New best result found",
			zap.String("metric", o.config.MetricToOptimize),
			zap.String("value", result.MetricValue.String()),
			zap.Any("parameters", result.Parameters))
	}
}

// getBestResult returns the best result found
func (o *Optimizer) getBestResult() *OptimizationResult {
	o.mu.RLock()
	defer o.mu.RUnlock()
	
	if o.bestResult == nil {
		return nil
	}
	
	// Return a copy
	result := *o.bestResult
	return &result
}

// GetAllResults returns all optimization results
func (o *Optimizer) GetAllResults() []OptimizationResult {
	o.mu.RLock()
	defer o.mu.RUnlock()
	
	results := make([]OptimizationResult, len(o.allResults))
	copy(results, o.allResults)
	return results
}

// GetProgress returns optimization progress
func (o *Optimizer) GetProgress() (current, total int, elapsed time.Duration) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	
	current = len(o.allResults)
	total = o.config.MaxIterations
	elapsed = time.Since(o.startTime)
	return
}