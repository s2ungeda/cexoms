package position

import (
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// PositionOptimizer optimizes position sizing and allocation across accounts
type PositionOptimizer struct {
	mu sync.RWMutex
	
	positionManager *MultiAccountPositionManager
	accountManager  types.AccountManager
	
	// Configuration
	config *OptimizerConfig
	
	// Optimization models
	models map[string]OptimizationModel
}

// OptimizerConfig contains optimizer configuration
type OptimizerConfig struct {
	// Risk parameters
	MaxPortfolioRisk      decimal.Decimal
	MaxPositionRisk       decimal.Decimal
	TargetSharpeRatio     float64
	
	// Position sizing
	MinPositionSize       decimal.Decimal
	MaxPositionSize       decimal.Decimal
	PositionSizeIncrement decimal.Decimal
	
	// Correlation
	UseCorrelation        bool
	CorrelationWindow     int
	
	// Optimization
	OptimizationMethod    string // "kelly", "equal_weight", "risk_parity", "max_sharpe"
	RebalanceThreshold    decimal.Decimal
}

// OptimizationModel defines position optimization strategy
type OptimizationModel interface {
	Name() string
	OptimizePositions(accounts []*types.Account, signals []TradingSignal, constraints Constraints) (*OptimizationResult, error)
}

// TradingSignal represents a trading opportunity
type TradingSignal struct {
	Symbol          string
	Direction       types.OrderSide
	ExpectedReturn  decimal.Decimal
	ExpectedRisk    decimal.Decimal
	Confidence      decimal.Decimal
	Strategy        string
	TimeHorizon     string // "short", "medium", "long"
	Priority        int
}

// Constraints defines optimization constraints
type Constraints struct {
	MaxTotalExposure      decimal.Decimal
	MaxExposurePerSymbol  decimal.Decimal
	MaxExposurePerAccount decimal.Decimal
	MaxLeverage           decimal.Decimal
	RequiredMargin        decimal.Decimal
	
	// Account-specific constraints
	AccountConstraints    map[string]*AccountConstraints
}

// AccountConstraints defines constraints for a specific account
type AccountConstraints struct {
	MaxPositions    int
	MaxExposure     decimal.Decimal
	MaxLeverage     int
	AllowedSymbols  []string
	RestrictedHours []TimeWindow
}

// TimeWindow represents a time window
type TimeWindow struct {
	Start string
	End   string
}

// OptimizationResult contains optimization results
type OptimizationResult struct {
	Allocations       map[string][]PositionAllocation // accountID -> allocations
	ExpectedReturn    decimal.Decimal
	ExpectedRisk      decimal.Decimal
	ExpectedSharpe    float64
	DiversificationScore float64
	
	// Execution plan
	ExecutionPlan     []ExecutionStep
	EstimatedCost     decimal.Decimal
	RequiredMargin    decimal.Decimal
}

// PositionAllocation represents allocation for a position
type PositionAllocation struct {
	AccountID     string
	Symbol        string
	Side          types.OrderSide
	Size          decimal.Decimal
	TargetPrice   decimal.Decimal
	StopLoss      decimal.Decimal
	TakeProfit    decimal.Decimal
	RiskAmount    decimal.Decimal
	Weight        decimal.Decimal
}

// ExecutionStep represents a step in execution plan
type ExecutionStep struct {
	Priority      int
	AccountID     string
	Action        string // "open", "close", "reduce", "increase"
	Symbol        string
	Quantity      decimal.Decimal
	EstimatedCost decimal.Decimal
}

// NewPositionOptimizer creates a new position optimizer
func NewPositionOptimizer(positionManager *MultiAccountPositionManager, accountManager types.AccountManager, config *OptimizerConfig) *PositionOptimizer {
	if config == nil {
		config = &OptimizerConfig{
			MaxPortfolioRisk:      decimal.NewFromFloat(0.02), // 2% portfolio risk
			MaxPositionRisk:       decimal.NewFromFloat(0.01), // 1% per position
			TargetSharpeRatio:     1.5,
			MinPositionSize:       decimal.NewFromInt(100),
			MaxPositionSize:       decimal.NewFromInt(100000),
			PositionSizeIncrement: decimal.NewFromInt(100),
			UseCorrelation:        true,
			CorrelationWindow:     100,
			OptimizationMethod:    "risk_parity",
			RebalanceThreshold:    decimal.NewFromFloat(0.1), // 10% threshold
		}
	}
	
	po := &PositionOptimizer{
		positionManager: positionManager,
		accountManager:  accountManager,
		config:          config,
		models:          make(map[string]OptimizationModel),
	}
	
	// Register optimization models
	po.RegisterModel(NewKellyOptimizer(config))
	po.RegisterModel(NewRiskParityOptimizer(config))
	po.RegisterModel(NewMaxSharpeOptimizer(config))
	po.RegisterModel(NewEqualWeightOptimizer(config))
	
	return po
}

// RegisterModel registers an optimization model
func (po *PositionOptimizer) RegisterModel(model OptimizationModel) {
	po.mu.Lock()
	defer po.mu.Unlock()
	
	po.models[model.Name()] = model
}

// OptimizePortfolio optimizes portfolio allocation
func (po *PositionOptimizer) OptimizePortfolio(signals []TradingSignal) (*OptimizationResult, error) {
	po.mu.RLock()
	model, exists := po.models[po.config.OptimizationMethod]
	po.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("optimization method %s not found", po.config.OptimizationMethod)
	}
	
	// Get active accounts
	accounts, err := po.accountManager.ListAccounts(types.AccountFilter{
		Active: &[]bool{true}[0],
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}
	
	// Build constraints
	constraints := po.buildConstraints(accounts)
	
	// Run optimization
	result, err := model.OptimizePositions(accounts, signals, constraints)
	if err != nil {
		return nil, fmt.Errorf("optimization failed: %w", err)
	}
	
	// Validate result
	if err := po.validateOptimizationResult(result, constraints); err != nil {
		return nil, fmt.Errorf("invalid optimization result: %w", err)
	}
	
	// Generate execution plan
	result.ExecutionPlan = po.generateExecutionPlan(result)
	
	return result, nil
}

// CalculateOptimalPositionSize calculates optimal position size for a signal
func (po *PositionOptimizer) CalculateOptimalPositionSize(accountID string, signal TradingSignal) (decimal.Decimal, error) {
	// Get account
	account, err := po.accountManager.GetAccount(accountID)
	if err != nil {
		return decimal.Zero, err
	}
	
	// Get account balance
	balance, err := po.accountManager.GetBalance(accountID)
	if err != nil {
		return decimal.Zero, err
	}
	
	// Get current positions
	positions, err := po.positionManager.GetAccountPositions(accountID)
	if err != nil {
		return decimal.Zero, err
	}
	
	// Calculate available capital
	availableCapital := balance.TotalUSDT
	for _, pos := range positions {
		availableCapital = availableCapital.Sub(pos.Margin)
	}
	
	// Apply Kelly criterion or other sizing method
	optimalSize := po.calculateKellySize(signal, availableCapital)
	
	// Apply constraints
	optimalSize = po.applyPositionConstraints(optimalSize, account, signal)
	
	return optimalSize, nil
}

// RebalancePositions suggests rebalancing actions
func (po *PositionOptimizer) RebalancePositions() (*RebalanceRecommendation, error) {
	// Get current portfolio state
	portfolio := po.positionManager.GetPortfolioSummary()
	
	// Calculate current weights
	currentWeights := po.calculateCurrentWeights(portfolio)
	
	// Get target weights based on optimization
	targetWeights := po.calculateTargetWeights()
	
	// Calculate rebalancing needs
	recommendation := &RebalanceRecommendation{
		CurrentWeights: currentWeights,
		TargetWeights:  targetWeights,
		Actions:        make([]RebalanceAction, 0),
	}
	
	// Generate rebalancing actions
	for symbol, targetWeight := range targetWeights {
		currentWeight := currentWeights[symbol]
		diff := targetWeight.Sub(currentWeight)
		
		if diff.Abs().GreaterThan(po.config.RebalanceThreshold) {
			action := RebalanceAction{
				Symbol:        symbol,
				CurrentWeight: currentWeight,
				TargetWeight:  targetWeight,
				RequiredChange: diff,
			}
			
			if diff.GreaterThan(decimal.Zero) {
				action.Action = "increase"
			} else {
				action.Action = "decrease"
			}
			
			recommendation.Actions = append(recommendation.Actions, action)
		}
	}
	
	// Sort by priority
	sort.Slice(recommendation.Actions, func(i, j int) bool {
		return recommendation.Actions[i].RequiredChange.Abs().GreaterThan(
			recommendation.Actions[j].RequiredChange.Abs())
	})
	
	return recommendation, nil
}

// Helper methods

// buildConstraints builds optimization constraints
func (po *PositionOptimizer) buildConstraints(accounts []*types.Account) Constraints {
	constraints := Constraints{
		MaxTotalExposure:     decimal.NewFromInt(10000000), // $10M default
		MaxExposurePerSymbol: decimal.NewFromInt(1000000),  // $1M per symbol
		MaxLeverage:          decimal.NewFromInt(5),
		AccountConstraints:   make(map[string]*AccountConstraints),
	}
	
	// Build account-specific constraints
	for _, account := range accounts {
		accConstraints := &AccountConstraints{
			MaxPositions: 50,
			MaxExposure:  account.MaxPositionUSDT,
			MaxLeverage:  account.MaxLeverage,
		}
		
		constraints.AccountConstraints[account.ID] = accConstraints
	}
	
	return constraints
}

// validateOptimizationResult validates optimization result
func (po *PositionOptimizer) validateOptimizationResult(result *OptimizationResult, constraints Constraints) error {
	totalExposure := decimal.Zero
	symbolExposure := make(map[string]decimal.Decimal)
	
	for accountID, allocations := range result.Allocations {
		accountExposure := decimal.Zero
		
		for _, alloc := range allocations {
			exposure := alloc.Size.Mul(alloc.TargetPrice)
			accountExposure = accountExposure.Add(exposure)
			totalExposure = totalExposure.Add(exposure)
			
			symbolExposure[alloc.Symbol] = symbolExposure[alloc.Symbol].Add(exposure)
		}
		
		// Check account constraints
		if accConstraints, exists := constraints.AccountConstraints[accountID]; exists {
			if accountExposure.GreaterThan(accConstraints.MaxExposure) {
				return fmt.Errorf("account %s exceeds max exposure", accountID)
			}
		}
	}
	
	// Check total exposure
	if totalExposure.GreaterThan(constraints.MaxTotalExposure) {
		return fmt.Errorf("total exposure exceeds limit")
	}
	
	// Check symbol exposure
	for symbol, exposure := range symbolExposure {
		if exposure.GreaterThan(constraints.MaxExposurePerSymbol) {
			return fmt.Errorf("symbol %s exceeds max exposure", symbol)
		}
	}
	
	return nil
}

// generateExecutionPlan generates execution plan from optimization result
func (po *PositionOptimizer) generateExecutionPlan(result *OptimizationResult) []ExecutionStep {
	steps := make([]ExecutionStep, 0)
	priority := 1
	
	// Get current positions
	portfolio := po.positionManager.GetPortfolioSummary()
	
	// First, close positions that need to be closed
	for accountID, summary := range portfolio.AccountSummaries {
		for symbol, pos := range summary.PositionsBySymbol {
			// Check if position should be closed
			shouldClose := true
			if allocations, exists := result.Allocations[accountID]; exists {
				for _, alloc := range allocations {
					if alloc.Symbol == symbol {
						shouldClose = false
						break
					}
				}
			}
			
			if shouldClose {
				steps = append(steps, ExecutionStep{
					Priority:      priority,
					AccountID:     accountID,
					Action:        "close",
					Symbol:        symbol,
					Quantity:      pos.Quantity,
					EstimatedCost: decimal.Zero,
				})
				priority++
			}
		}
	}
	
	// Then, adjust existing positions
	for accountID, allocations := range result.Allocations {
		for _, alloc := range allocations {
			currentPos, _ := po.positionManager.GetPosition(accountID, alloc.Symbol)
			
			if currentPos != nil {
				// Position exists, check if adjustment needed
				diff := alloc.Size.Sub(currentPos.Quantity)
				
				if diff.Abs().GreaterThan(po.config.PositionSizeIncrement) {
					action := "increase"
					if diff.LessThan(decimal.Zero) {
						action = "reduce"
					}
					
					steps = append(steps, ExecutionStep{
						Priority:      priority,
						AccountID:     accountID,
						Action:        action,
						Symbol:        alloc.Symbol,
						Quantity:      diff.Abs(),
						EstimatedCost: diff.Abs().Mul(alloc.TargetPrice),
					})
					priority++
				}
			} else {
				// New position
				steps = append(steps, ExecutionStep{
					Priority:      priority,
					AccountID:     accountID,
					Action:        "open",
					Symbol:        alloc.Symbol,
					Quantity:      alloc.Size,
					EstimatedCost: alloc.Size.Mul(alloc.TargetPrice),
				})
				priority++
			}
		}
	}
	
	return steps
}

// calculateKellySize calculates position size using Kelly criterion
func (po *PositionOptimizer) calculateKellySize(signal TradingSignal, capital decimal.Decimal) decimal.Decimal {
	// Kelly fraction = (p * b - q) / b
	// where p = win probability, b = win/loss ratio, q = loss probability
	
	winProb := signal.Confidence
	lossProb := decimal.NewFromInt(1).Sub(winProb)
	
	// Calculate win/loss ratio from expected return and risk
	winLossRatio := signal.ExpectedReturn.Div(signal.ExpectedRisk)
	
	// Kelly fraction
	kellyFraction := winProb.Mul(winLossRatio).Sub(lossProb).Div(winLossRatio)
	
	// Apply Kelly fraction with safety factor (typically 0.25)
	safetyFactor := decimal.NewFromFloat(0.25)
	adjustedKelly := kellyFraction.Mul(safetyFactor)
	
	// Calculate position size
	positionSize := capital.Mul(adjustedKelly)
	
	return positionSize
}

// applyPositionConstraints applies constraints to position size
func (po *PositionOptimizer) applyPositionConstraints(size decimal.Decimal, account *types.Account, signal TradingSignal) decimal.Decimal {
	// Apply minimum size
	if size.LessThan(po.config.MinPositionSize) {
		return decimal.Zero
	}
	
	// Apply maximum size
	if size.GreaterThan(po.config.MaxPositionSize) {
		size = po.config.MaxPositionSize
	}
	
	// Apply account position limit
	if !account.MaxPositionUSDT.IsZero() && size.GreaterThan(account.MaxPositionUSDT) {
		size = account.MaxPositionUSDT
	}
	
	// Round to increment
	if !po.config.PositionSizeIncrement.IsZero() {
		increments := size.Div(po.config.PositionSizeIncrement).Floor()
		size = increments.Mul(po.config.PositionSizeIncrement)
	}
	
	return size
}

// calculateCurrentWeights calculates current portfolio weights
func (po *PositionOptimizer) calculateCurrentWeights(portfolio *PortfolioSummary) map[string]decimal.Decimal {
	weights := make(map[string]decimal.Decimal)
	
	if portfolio.TotalValue.IsZero() {
		return weights
	}
	
	for symbol, globalPos := range portfolio.GlobalPositions {
		weight := globalPos.TotalValue.Div(portfolio.TotalValue)
		weights[symbol] = weight
	}
	
	return weights
}

// calculateTargetWeights calculates target portfolio weights
func (po *PositionOptimizer) calculateTargetWeights() map[string]decimal.Decimal {
	// This is simplified - in production, use optimization result
	weights := make(map[string]decimal.Decimal)
	
	// For now, return equal weights
	// In production, this would come from the optimization model
	
	return weights
}

// Optimization Models

// KellyOptimizer implements Kelly criterion optimization
type KellyOptimizer struct {
	config *OptimizerConfig
}

func NewKellyOptimizer(config *OptimizerConfig) *KellyOptimizer {
	return &KellyOptimizer{config: config}
}

func (ko *KellyOptimizer) Name() string { return "kelly" }

func (ko *KellyOptimizer) OptimizePositions(accounts []*types.Account, signals []TradingSignal, constraints Constraints) (*OptimizationResult, error) {
	// Implement Kelly optimization
	// This is a placeholder - implement full Kelly optimization
	return &OptimizationResult{
		Allocations: make(map[string][]PositionAllocation),
	}, nil
}

// RiskParityOptimizer implements risk parity optimization
type RiskParityOptimizer struct {
	config *OptimizerConfig
}

func NewRiskParityOptimizer(config *OptimizerConfig) *RiskParityOptimizer {
	return &RiskParityOptimizer{config: config}
}

func (rp *RiskParityOptimizer) Name() string { return "risk_parity" }

func (rp *RiskParityOptimizer) OptimizePositions(accounts []*types.Account, signals []TradingSignal, constraints Constraints) (*OptimizationResult, error) {
	result := &OptimizationResult{
		Allocations: make(map[string][]PositionAllocation),
	}
	
	// Calculate total risk budget
	totalRiskBudget := rp.config.MaxPortfolioRisk
	
	// Allocate risk equally across signals
	riskPerSignal := totalRiskBudget.Div(decimal.NewFromInt(int64(len(signals))))
	
	// Allocate to accounts
	for _, signal := range signals {
		// Find best account for this signal
		bestAccount := rp.selectBestAccount(accounts, signal)
		if bestAccount == nil {
			continue
		}
		
		// Calculate position size for equal risk contribution
		positionSize := rp.calculateRiskParitySize(signal, riskPerSignal)
		
		allocation := PositionAllocation{
			AccountID:   bestAccount.ID,
			Symbol:      signal.Symbol,
			Side:        signal.Direction,
			Size:        positionSize,
			RiskAmount:  riskPerSignal,
			Weight:      riskPerSignal.Div(totalRiskBudget),
		}
		
		if _, exists := result.Allocations[bestAccount.ID]; !exists {
			result.Allocations[bestAccount.ID] = make([]PositionAllocation, 0)
		}
		
		result.Allocations[bestAccount.ID] = append(result.Allocations[bestAccount.ID], allocation)
	}
	
	return result, nil
}

func (rp *RiskParityOptimizer) selectBestAccount(accounts []*types.Account, signal TradingSignal) *types.Account {
	// Select account based on strategy match and available capacity
	for _, account := range accounts {
		if account.Strategy == signal.Strategy {
			return account
		}
	}
	
	// Fallback to first available account
	if len(accounts) > 0 {
		return accounts[0]
	}
	
	return nil
}

func (rp *RiskParityOptimizer) calculateRiskParitySize(signal TradingSignal, targetRisk decimal.Decimal) decimal.Decimal {
	// Position size = target risk / expected risk per unit
	if signal.ExpectedRisk.IsZero() {
		return decimal.Zero
	}
	
	return targetRisk.Div(signal.ExpectedRisk)
}

// MaxSharpeOptimizer implements maximum Sharpe ratio optimization
type MaxSharpeOptimizer struct {
	config *OptimizerConfig
}

func NewMaxSharpeOptimizer(config *OptimizerConfig) *MaxSharpeOptimizer {
	return &MaxSharpeOptimizer{config: config}
}

func (ms *MaxSharpeOptimizer) Name() string { return "max_sharpe" }

func (ms *MaxSharpeOptimizer) OptimizePositions(accounts []*types.Account, signals []TradingSignal, constraints Constraints) (*OptimizationResult, error) {
	// Implement Sharpe ratio maximization
	// This is a placeholder - implement full optimization
	return &OptimizationResult{
		Allocations: make(map[string][]PositionAllocation),
	}, nil
}

// EqualWeightOptimizer implements equal weight optimization
type EqualWeightOptimizer struct {
	config *OptimizerConfig
}

func NewEqualWeightOptimizer(config *OptimizerConfig) *EqualWeightOptimizer {
	return &EqualWeightOptimizer{config: config}
}

func (ew *EqualWeightOptimizer) Name() string { return "equal_weight" }

func (ew *EqualWeightOptimizer) OptimizePositions(accounts []*types.Account, signals []TradingSignal, constraints Constraints) (*OptimizationResult, error) {
	result := &OptimizationResult{
		Allocations: make(map[string][]PositionAllocation),
	}
	
	// Simple equal weight allocation
	weightPerSignal := decimal.NewFromInt(1).Div(decimal.NewFromInt(int64(len(signals))))
	
	// Distribute signals across accounts
	accountIndex := 0
	for _, signal := range signals {
		if accountIndex >= len(accounts) {
			accountIndex = 0
		}
		
		account := accounts[accountIndex]
		
		// Calculate equal weight position size
		balance, _ := ew.getAccountBalance(account.ID)
		positionSize := balance.Mul(weightPerSignal)
		
		allocation := PositionAllocation{
			AccountID: account.ID,
			Symbol:    signal.Symbol,
			Side:      signal.Direction,
			Size:      positionSize,
			Weight:    weightPerSignal,
		}
		
		if _, exists := result.Allocations[account.ID]; !exists {
			result.Allocations[account.ID] = make([]PositionAllocation, 0)
		}
		
		result.Allocations[account.ID] = append(result.Allocations[account.ID], allocation)
		
		accountIndex++
	}
	
	// Calculate expected metrics
	result.ExpectedReturn = ew.calculateExpectedReturn(signals, result.Allocations)
	result.ExpectedRisk = ew.calculateExpectedRisk(signals, result.Allocations)
	
	if !result.ExpectedRisk.IsZero() {
		riskFreeRate := 0.02 / math.Sqrt(252) // Daily risk-free rate
		excessReturn := result.ExpectedReturn.Sub(decimal.NewFromFloat(riskFreeRate))
		result.ExpectedSharpe = excessReturn.Div(result.ExpectedRisk).InexactFloat64()
	}
	
	return result, nil
}

func (ew *EqualWeightOptimizer) getAccountBalance(accountID string) decimal.Decimal {
	// Placeholder - get from account manager
	return decimal.NewFromInt(100000)
}

func (ew *EqualWeightOptimizer) calculateExpectedReturn(signals []TradingSignal, allocations map[string][]PositionAllocation) decimal.Decimal {
	totalReturn := decimal.Zero
	totalWeight := decimal.Zero
	
	for _, accountAllocs := range allocations {
		for _, alloc := range accountAllocs {
			// Find corresponding signal
			for _, signal := range signals {
				if signal.Symbol == alloc.Symbol {
					weightedReturn := signal.ExpectedReturn.Mul(alloc.Weight)
					totalReturn = totalReturn.Add(weightedReturn)
					totalWeight = totalWeight.Add(alloc.Weight)
					break
				}
			}
		}
	}
	
	if totalWeight.IsZero() {
		return decimal.Zero
	}
	
	return totalReturn.Div(totalWeight)
}

func (ew *EqualWeightOptimizer) calculateExpectedRisk(signals []TradingSignal, allocations map[string][]PositionAllocation) decimal.Decimal {
	// Simplified risk calculation - in production use covariance matrix
	totalRisk := decimal.Zero
	totalWeight := decimal.Zero
	
	for _, accountAllocs := range allocations {
		for _, alloc := range accountAllocs {
			// Find corresponding signal
			for _, signal := range signals {
				if signal.Symbol == alloc.Symbol {
					weightedRisk := signal.ExpectedRisk.Mul(alloc.Weight)
					totalRisk = totalRisk.Add(weightedRisk.Mul(weightedRisk))
					totalWeight = totalWeight.Add(alloc.Weight)
					break
				}
			}
		}
	}
	
	if totalRisk.IsZero() {
		return decimal.Zero
	}
	
	// Square root for portfolio risk (assumes no correlation)
	return decimal.NewFromFloat(math.Sqrt(totalRisk.InexactFloat64()))
}

// Supporting types

// RebalanceRecommendation contains rebalancing recommendations
type RebalanceRecommendation struct {
	CurrentWeights map[string]decimal.Decimal
	TargetWeights  map[string]decimal.Decimal
	Actions        []RebalanceAction
	EstimatedCost  decimal.Decimal
}

// RebalanceAction represents a rebalancing action
type RebalanceAction struct {
	Symbol         string
	Action         string // "increase", "decrease"
	CurrentWeight  decimal.Decimal
	TargetWeight   decimal.Decimal
	RequiredChange decimal.Decimal
}