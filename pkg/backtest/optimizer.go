package backtest

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/your-org/mExOms/pkg/types"
	"go.uber.org/zap"
)

// OptimizationConfig contains configuration for optimization
type OptimizationConfig struct {
	// Optimization method
	Method OptimizationMethod
	
	// Parameters to optimize
	Parameters []ParameterRange
	
	// Objective function
	Objective ObjectiveFunction
	
	// Constraints
	MinTrades        int
	MaxDrawdown      float64
	MinSharpeRatio   float64
	
	// Algorithm settings
	MaxIterations    int
	PopulationSize   int
	CrossoverRate    float64
	MutationRate     float64
	EliteRatio       float64
	
	// Parallel execution
	NumWorkers       int
	
	// Walk-forward settings
	UseWalkForward   bool
	InSampleRatio    float64
	OutSamplePeriods int
}

// OptimizationMethod defines the optimization algorithm
type OptimizationMethod string

const (
	MethodGridSearch     OptimizationMethod = "grid_search"
	MethodRandomSearch   OptimizationMethod = "random_search"
	MethodGeneticAlgorithm OptimizationMethod = "genetic_algorithm"
	MethodBayesianOpt    OptimizationMethod = "bayesian"
)

// ObjectiveFunction defines what to optimize
type ObjectiveFunction string

const (
	ObjectiveSharpeRatio    ObjectiveFunction = "sharpe_ratio"
	ObjectiveTotalReturn    ObjectiveFunction = "total_return"
	ObjectiveProfitFactor   ObjectiveFunction = "profit_factor"
	ObjectiveCalmarRatio    ObjectiveFunction = "calmar_ratio"
	ObjectiveSortinoRatio   ObjectiveFunction = "sortino_ratio"
	ObjectiveCustom         ObjectiveFunction = "custom"
)

// ParameterRange defines the range for a parameter
type ParameterRange struct {
	Name     string
	Min      float64
	Max      float64
	Step     float64
	Type     ParameterType
	Current  float64
}

// ParameterType defines the type of parameter
type ParameterType string

const (
	TypeFloat   ParameterType = "float"
	TypeInt     ParameterType = "int"
	TypeBool    ParameterType = "bool"
)

// OptimizationResult contains the result of optimization
type OptimizationResult struct {
	BestParameters   map[string]float64
	BestScore        float64
	AllResults       []*ParameterSet
	ConvergenceHistory []float64
	InSampleMetrics  *PerformanceMetrics
	OutSampleMetrics *PerformanceMetrics
	TotalIterations  int
	Duration         time.Duration
}

// ParameterSet represents a set of parameters and their performance
type ParameterSet struct {
	Parameters map[string]float64
	Score      float64
	Metrics    *PerformanceMetrics
	IsValid    bool
	Reason     string
}

// Optimizer performs strategy parameter optimization
type Optimizer struct {
	config          OptimizationConfig
	backtestConfig  BacktestConfig
	strategyFactory types.StrategyFactory
	logger          *zap.Logger
	
	results         []*ParameterSet
	bestScore       float64
	bestParameters  map[string]float64
	iteration       int
	
	mu              sync.RWMutex
	ctx             context.Context
	cancel          context.CancelFunc
}

// NewOptimizer creates a new optimizer
func NewOptimizer(config OptimizationConfig, backtestConfig BacktestConfig, 
	strategyFactory types.StrategyFactory, logger *zap.Logger) *Optimizer {
	
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Optimizer{
		config:          config,
		backtestConfig:  backtestConfig,
		strategyFactory: strategyFactory,
		logger:          logger,
		results:         make([]*ParameterSet, 0),
		bestScore:       -math.MaxFloat64,
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Run performs the optimization
func (o *Optimizer) Run() (*OptimizationResult, error) {
	startTime := time.Now()
	
	o.logger.Info("Starting optimization",
		zap.String("method", string(o.config.Method)),
		zap.String("objective", string(o.config.Objective)),
		zap.Int("parameters", len(o.config.Parameters)))
	
	var err error
	
	switch o.config.Method {
	case MethodGridSearch:
		err = o.runGridSearch()
	case MethodRandomSearch:
		err = o.runRandomSearch()
	case MethodGeneticAlgorithm:
		err = o.runGeneticAlgorithm()
	case MethodBayesianOpt:
		err = o.runBayesianOptimization()
	default:
		return nil, fmt.Errorf("unknown optimization method: %s", o.config.Method)
	}
	
	if err != nil {
		return nil, err
	}
	
	// Run walk-forward analysis if enabled
	var outSampleMetrics *PerformanceMetrics
	if o.config.UseWalkForward {
		outSampleMetrics, err = o.runWalkForwardAnalysis()
		if err != nil {
			o.logger.Warn("Walk-forward analysis failed", zap.Error(err))
		}
	}
	
	// Get in-sample metrics for best parameters
	inSampleMetrics, _ := o.evaluateParameters(o.bestParameters)
	
	result := &OptimizationResult{
		BestParameters:   o.bestParameters,
		BestScore:        o.bestScore,
		AllResults:       o.results,
		InSampleMetrics:  inSampleMetrics.Metrics,
		OutSampleMetrics: outSampleMetrics,
		TotalIterations:  o.iteration,
		Duration:         time.Since(startTime),
	}
	
	o.logger.Info("Optimization completed",
		zap.Float64("best_score", o.bestScore),
		zap.Int("iterations", o.iteration),
		zap.Duration("duration", result.Duration))
	
	return result, nil
}

// runGridSearch performs grid search optimization
func (o *Optimizer) runGridSearch() error {
	// Generate all parameter combinations
	combinations := o.generateGridCombinations()
	totalCombinations := len(combinations)
	
	o.logger.Info("Running grid search",
		zap.Int("combinations", totalCombinations))
	
	// Process combinations in parallel
	workChan := make(chan map[string]float64, totalCombinations)
	resultChan := make(chan *ParameterSet, totalCombinations)
	
	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < o.config.NumWorkers; i++ {
		wg.Add(1)
		go o.optimizationWorker(&wg, workChan, resultChan)
	}
	
	// Queue all combinations
	for _, params := range combinations {
		workChan <- params
	}
	close(workChan)
	
	// Wait for completion
	go func() {
		wg.Wait()
		close(resultChan)
	}()
	
	// Collect results
	for result := range resultChan {
		o.updateBest(result)
	}
	
	return nil
}

// generateGridCombinations generates all parameter combinations for grid search
func (o *Optimizer) generateGridCombinations() []map[string]float64 {
	combinations := []map[string]float64{{}}
	
	for _, param := range o.config.Parameters {
		newCombinations := []map[string]float64{}
		
		// Generate values for this parameter
		values := o.generateParameterValues(param)
		
		// Create new combinations
		for _, combo := range combinations {
			for _, value := range values {
				newCombo := make(map[string]float64)
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

// generateParameterValues generates values for a parameter
func (o *Optimizer) generateParameterValues(param ParameterRange) []float64 {
	var values []float64
	
	if param.Type == TypeBool {
		return []float64{0, 1}
	}
	
	// Generate values from min to max with step
	current := param.Min
	for current <= param.Max {
		if param.Type == TypeInt {
			values = append(values, math.Round(current))
		} else {
			values = append(values, current)
		}
		current += param.Step
	}
	
	return values
}

// runRandomSearch performs random search optimization
func (o *Optimizer) runRandomSearch() error {
	o.logger.Info("Running random search",
		zap.Int("max_iterations", o.config.MaxIterations))
	
	workChan := make(chan map[string]float64, o.config.NumWorkers*2)
	resultChan := make(chan *ParameterSet, o.config.NumWorkers*2)
	
	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < o.config.NumWorkers; i++ {
		wg.Add(1)
		go o.optimizationWorker(&wg, workChan, resultChan)
	}
	
	// Generate random parameter sets
	go func() {
		for i := 0; i < o.config.MaxIterations; i++ {
			params := o.generateRandomParameters()
			workChan <- params
		}
		close(workChan)
	}()
	
	// Wait for completion
	go func() {
		wg.Wait()
		close(resultChan)
	}()
	
	// Collect results
	for result := range resultChan {
		o.updateBest(result)
	}
	
	return nil
}

// generateRandomParameters generates random parameter values
func (o *Optimizer) generateRandomParameters() map[string]float64 {
	params := make(map[string]float64)
	
	for _, param := range o.config.Parameters {
		if param.Type == TypeBool {
			if rand.Float64() > 0.5 {
				params[param.Name] = 1
			} else {
				params[param.Name] = 0
			}
		} else if param.Type == TypeInt {
			value := param.Min + rand.Float64()*(param.Max-param.Min)
			params[param.Name] = math.Round(value)
		} else {
			params[param.Name] = param.Min + rand.Float64()*(param.Max-param.Min)
		}
	}
	
	return params
}

// runGeneticAlgorithm performs genetic algorithm optimization
func (o *Optimizer) runGeneticAlgorithm() error {
	o.logger.Info("Running genetic algorithm",
		zap.Int("population_size", o.config.PopulationSize),
		zap.Int("max_generations", o.config.MaxIterations))
	
	// Initialize population
	population := o.initializePopulation()
	
	// Evolution loop
	for generation := 0; generation < o.config.MaxIterations; generation++ {
		// Evaluate fitness
		o.evaluatePopulation(population)
		
		// Sort by fitness
		sort.Slice(population, func(i, j int) bool {
			return population[i].Score > population[j].Score
		})
		
		// Update best
		if len(population) > 0 {
			o.updateBest(population[0])
		}
		
		// Check convergence
		if o.checkConvergence(population) {
			o.logger.Info("Converged", zap.Int("generation", generation))
			break
		}
		
		// Create next generation
		population = o.evolvePopulation(population)
		
		// Log progress
		if generation%10 == 0 {
			o.logger.Info("Generation progress",
				zap.Int("generation", generation),
				zap.Float64("best_score", o.bestScore))
		}
	}
	
	return nil
}

// initializePopulation creates initial population for GA
func (o *Optimizer) initializePopulation() []*ParameterSet {
	population := make([]*ParameterSet, o.config.PopulationSize)
	
	for i := 0; i < o.config.PopulationSize; i++ {
		params := o.generateRandomParameters()
		population[i] = &ParameterSet{
			Parameters: params,
		}
	}
	
	return population
}

// evaluatePopulation evaluates all individuals in parallel
func (o *Optimizer) evaluatePopulation(population []*ParameterSet) {
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, o.config.NumWorkers)
	
	for _, individual := range population {
		if individual.Score != 0 {
			continue // Already evaluated
		}
		
		wg.Add(1)
		semaphore <- struct{}{}
		
		go func(ind *ParameterSet) {
			defer wg.Done()
			defer func() { <-semaphore }()
			
			result, _ := o.evaluateParameters(ind.Parameters)
			ind.Score = result.Score
			ind.Metrics = result.Metrics
			ind.IsValid = result.IsValid
			ind.Reason = result.Reason
		}(individual)
	}
	
	wg.Wait()
}

// evolvePopulation creates next generation
func (o *Optimizer) evolvePopulation(population []*ParameterSet) []*ParameterSet {
	newPopulation := make([]*ParameterSet, 0, o.config.PopulationSize)
	
	// Elite selection
	eliteSize := int(float64(o.config.PopulationSize) * o.config.EliteRatio)
	for i := 0; i < eliteSize && i < len(population); i++ {
		newPopulation = append(newPopulation, population[i])
	}
	
	// Generate rest through crossover and mutation
	for len(newPopulation) < o.config.PopulationSize {
		// Tournament selection
		parent1 := o.tournamentSelection(population)
		parent2 := o.tournamentSelection(population)
		
		// Crossover
		var child *ParameterSet
		if rand.Float64() < o.config.CrossoverRate {
			child = o.crossover(parent1, parent2)
		} else {
			child = &ParameterSet{Parameters: parent1.Parameters}
		}
		
		// Mutation
		if rand.Float64() < o.config.MutationRate {
			o.mutate(child)
		}
		
		newPopulation = append(newPopulation, child)
	}
	
	return newPopulation
}

// tournamentSelection selects individual using tournament
func (o *Optimizer) tournamentSelection(population []*ParameterSet) *ParameterSet {
	tournamentSize := 3
	
	best := population[rand.Intn(len(population))]
	
	for i := 1; i < tournamentSize; i++ {
		candidate := population[rand.Intn(len(population))]
		if candidate.Score > best.Score {
			best = candidate
		}
	}
	
	return best
}

// crossover creates offspring from two parents
func (o *Optimizer) crossover(parent1, parent2 *ParameterSet) *ParameterSet {
	child := &ParameterSet{
		Parameters: make(map[string]float64),
	}
	
	// Uniform crossover
	for _, param := range o.config.Parameters {
		if rand.Float64() < 0.5 {
			child.Parameters[param.Name] = parent1.Parameters[param.Name]
		} else {
			child.Parameters[param.Name] = parent2.Parameters[param.Name]
		}
	}
	
	return child
}

// mutate applies mutation to individual
func (o *Optimizer) mutate(individual *ParameterSet) {
	for _, param := range o.config.Parameters {
		if rand.Float64() < 0.1 { // 10% chance per parameter
			if param.Type == TypeBool {
				// Flip boolean
				individual.Parameters[param.Name] = 1 - individual.Parameters[param.Name]
			} else {
				// Gaussian mutation
				stdDev := (param.Max - param.Min) * 0.1
				mutation := rand.NormFloat64() * stdDev
				newValue := individual.Parameters[param.Name] + mutation
				
				// Clamp to range
				if newValue < param.Min {
					newValue = param.Min
				} else if newValue > param.Max {
					newValue = param.Max
				}
				
				if param.Type == TypeInt {
					newValue = math.Round(newValue)
				}
				
				individual.Parameters[param.Name] = newValue
			}
		}
	}
}

// checkConvergence checks if population has converged
func (o *Optimizer) checkConvergence(population []*ParameterSet) bool {
	if len(population) < 2 {
		return false
	}
	
	// Check if top individuals have similar scores
	topN := int(float64(len(population)) * 0.1)
	if topN < 2 {
		topN = 2
	}
	
	maxScore := population[0].Score
	minScore := population[topN-1].Score
	
	// Converged if top scores are within 1%
	return (maxScore-minScore)/maxScore < 0.01
}

// runBayesianOptimization performs Bayesian optimization
func (o *Optimizer) runBayesianOptimization() error {
	// Simplified Bayesian optimization using Gaussian Process surrogate
	o.logger.Info("Running Bayesian optimization",
		zap.Int("max_iterations", o.config.MaxIterations))
	
	// Initial random sampling
	initialSamples := o.config.PopulationSize
	if initialSamples > o.config.MaxIterations/2 {
		initialSamples = o.config.MaxIterations / 2
	}
	
	// Evaluate initial samples
	for i := 0; i < initialSamples; i++ {
		params := o.generateRandomParameters()
		result, _ := o.evaluateParameters(params)
		o.updateBest(result)
	}
	
	// Acquisition function optimization
	for i := initialSamples; i < o.config.MaxIterations; i++ {
		// Find next point to evaluate using Expected Improvement
		nextParams := o.findNextPoint()
		result, _ := o.evaluateParameters(nextParams)
		o.updateBest(result)
		
		if i%10 == 0 {
			o.logger.Info("Bayesian optimization progress",
				zap.Int("iteration", i),
				zap.Float64("best_score", o.bestScore))
		}
	}
	
	return nil
}

// findNextPoint finds next point to evaluate using acquisition function
func (o *Optimizer) findNextPoint() map[string]float64 {
	// Simplified: Use random search with bias towards unexplored regions
	// In production, would use Gaussian Process and Expected Improvement
	
	bestEI := -math.MaxFloat64
	var bestParams map[string]float64
	
	// Sample random points and estimate expected improvement
	for i := 0; i < 100; i++ {
		params := o.generateRandomParameters()
		ei := o.estimateExpectedImprovement(params)
		
		if ei > bestEI {
			bestEI = ei
			bestParams = params
		}
	}
	
	return bestParams
}

// estimateExpectedImprovement estimates EI for parameter set
func (o *Optimizer) estimateExpectedImprovement(params map[string]float64) float64 {
	// Simplified: Use distance to nearest evaluated point
	// In production, would use GP predictions
	
	minDistance := math.MaxFloat64
	
	for _, result := range o.results {
		distance := o.parameterDistance(params, result.Parameters)
		if distance < minDistance {
			minDistance = distance
		}
	}
	
	// Higher EI for unexplored regions
	return minDistance
}

// parameterDistance calculates distance between parameter sets
func (o *Optimizer) parameterDistance(p1, p2 map[string]float64) float64 {
	distance := 0.0
	
	for _, param := range o.config.Parameters {
		diff := p1[param.Name] - p2[param.Name]
		normalized := diff / (param.Max - param.Min)
		distance += normalized * normalized
	}
	
	return math.Sqrt(distance)
}

// optimizationWorker processes parameter sets
func (o *Optimizer) optimizationWorker(wg *sync.WaitGroup, 
	workChan <-chan map[string]float64, resultChan chan<- *ParameterSet) {
	
	defer wg.Done()
	
	for params := range workChan {
		result, err := o.evaluateParameters(params)
		if err != nil {
			o.logger.Error("Failed to evaluate parameters",
				zap.Error(err))
			continue
		}
		
		resultChan <- result
	}
}

// evaluateParameters runs backtest with given parameters
func (o *Optimizer) evaluateParameters(params map[string]float64) (*ParameterSet, error) {
	// Create strategy with parameters
	strategy := o.strategyFactory.CreateStrategy(params)
	
	// Run backtest
	engine := NewBacktestEngine(o.backtestConfig, strategy, o.logger)
	result, err := engine.Run()
	if err != nil {
		return nil, err
	}
	
	// Calculate score based on objective
	score := o.calculateObjectiveScore(result)
	
	// Check constraints
	isValid := true
	reason := ""
	
	if len(result.Trades) < o.config.MinTrades {
		isValid = false
		reason = fmt.Sprintf("insufficient trades: %d < %d", 
			len(result.Trades), o.config.MinTrades)
	} else if result.Metrics.MaxDrawdown > o.config.MaxDrawdown {
		isValid = false
		reason = fmt.Sprintf("excessive drawdown: %.2f%% > %.2f%%",
			result.Metrics.MaxDrawdown*100, o.config.MaxDrawdown*100)
	} else if result.Metrics.SharpeRatio < o.config.MinSharpeRatio {
		isValid = false
		reason = fmt.Sprintf("low Sharpe ratio: %.2f < %.2f",
			result.Metrics.SharpeRatio, o.config.MinSharpeRatio)
	}
	
	if !isValid {
		score = -math.MaxFloat64
	}
	
	paramSet := &ParameterSet{
		Parameters: params,
		Score:      score,
		Metrics:    result.Metrics,
		IsValid:    isValid,
		Reason:     reason,
	}
	
	return paramSet, nil
}

// calculateObjectiveScore calculates score based on objective function
func (o *Optimizer) calculateObjectiveScore(result *BacktestResult) float64 {
	switch o.config.Objective {
	case ObjectiveSharpeRatio:
		return result.Metrics.SharpeRatio
	case ObjectiveTotalReturn:
		return result.Metrics.TotalReturn
	case ObjectiveProfitFactor:
		return result.Metrics.ProfitFactor
	case ObjectiveCalmarRatio:
		return result.Metrics.CalmarRatio
	case ObjectiveSortinoRatio:
		return result.Metrics.SortinoRatio
	default:
		return result.Metrics.SharpeRatio
	}
}

// updateBest updates best parameters if better score found
func (o *Optimizer) updateBest(result *ParameterSet) {
	o.mu.Lock()
	defer o.mu.Unlock()
	
	o.results = append(o.results, result)
	o.iteration++
	
	if result.IsValid && result.Score > o.bestScore {
		o.bestScore = result.Score
		o.bestParameters = make(map[string]float64)
		for k, v := range result.Parameters {
			o.bestParameters[k] = v
		}
		
		o.logger.Info("New best found",
			zap.Float64("score", o.bestScore),
			zap.Int("iteration", o.iteration))
	}
}

// runWalkForwardAnalysis performs walk-forward analysis
func (o *Optimizer) runWalkForwardAnalysis() (*PerformanceMetrics, error) {
	o.logger.Info("Running walk-forward analysis",
		zap.Int("periods", o.config.OutSamplePeriods))
	
	// TODO: Implement walk-forward analysis
	// Split data into in-sample and out-of-sample periods
	// Optimize on in-sample, test on out-of-sample
	// Roll forward and repeat
	
	return nil, nil
}

// GetProgress returns optimization progress
func (o *Optimizer) GetProgress() (int, float64) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	
	return o.iteration, o.bestScore
}

// Stop stops the optimization
func (o *Optimizer) Stop() {
	o.cancel()
}