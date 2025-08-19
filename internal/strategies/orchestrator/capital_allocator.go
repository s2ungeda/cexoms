package orchestrator

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// AllocationStrategy represents how capital is allocated
type AllocationStrategy string

const (
	AllocationEqual       AllocationStrategy = "equal"
	AllocationRiskParity  AllocationStrategy = "risk_parity"
	AllocationPerformance AllocationStrategy = "performance_based"
	AllocationCustom      AllocationStrategy = "custom"
)

// CapitalAllocation represents capital allocated to a strategy
type CapitalAllocation struct {
	StrategyID   string
	StrategyType StrategyType
	Amount       float64
	Percentage   float64
	UpdatedAt    time.Time
}

// CapitalAllocator manages capital allocation across strategies
type CapitalAllocator struct {
	config      CapitalAllocationConfig
	allocations map[string]*CapitalAllocation
	available   float64
	mu          sync.RWMutex
}

// NewCapitalAllocator creates a new capital allocator
func NewCapitalAllocator(config CapitalAllocationConfig) *CapitalAllocator {
	return &CapitalAllocator{
		config:      config,
		allocations: make(map[string]*CapitalAllocation),
		available:   config.TotalCapital,
	}
}

// AllocateCapital allocates capital for a new strategy
func (ca *CapitalAllocator) AllocateCapital(strategyType StrategyType, accounts []string) (float64, error) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	// Calculate allocation amount
	var amount float64
	
	// Determine base allocation
	switch strategyType {
	case StrategyTypeArbitrage:
		// Arbitrage strategies typically need more capital for simultaneous positions
		amount = ca.config.MaxPerStrategy * 0.8
	case StrategyTypeMarketMaking:
		// Market making needs capital for inventory
		amount = ca.config.MaxPerStrategy * 0.6
	default:
		amount = ca.config.MaxPerStrategy * 0.5
	}

	// Adjust based on number of accounts (more accounts = potentially more capital needed)
	accountMultiplier := math.Min(float64(len(accounts))*0.3+0.7, 2.0)
	amount *= accountMultiplier

	// Check constraints
	if amount > ca.config.MaxPerStrategy {
		amount = ca.config.MaxPerStrategy
	}
	if amount < ca.config.MinPerStrategy {
		return 0, fmt.Errorf("calculated allocation %.2f is below minimum %.2f", amount, ca.config.MinPerStrategy)
	}
	if amount > ca.available {
		return 0, fmt.Errorf("insufficient capital: requested %.2f, available %.2f", amount, ca.available)
	}

	// Update available capital
	ca.available -= amount

	return amount, nil
}

// ReleaseCapital releases capital from a stopped strategy
func (ca *CapitalAllocator) ReleaseCapital(strategyID string, returnedAmount float64) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	if allocation, exists := ca.allocations[strategyID]; exists {
		// If no specific amount provided, release the full allocation
		if returnedAmount == 0 {
			returnedAmount = allocation.Amount
		}
		
		ca.available += returnedAmount
		delete(ca.allocations, strategyID)
	}
}

// Rebalance rebalances capital based on strategy performance
func (ca *CapitalAllocator) Rebalance(strategies []*StrategyInstance) map[string]*CapitalAllocation {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	if !ca.config.RiskAdjusted {
		// Simple equal allocation
		return ca.equalAllocation(strategies)
	}

	// Risk-adjusted allocation based on performance
	return ca.riskAdjustedAllocation(strategies)
}

// equalAllocation allocates capital equally among strategies
func (ca *CapitalAllocator) equalAllocation(strategies []*StrategyInstance) map[string]*CapitalAllocation {
	if len(strategies) == 0 {
		return ca.allocations
	}

	perStrategy := ca.config.TotalCapital / float64(len(strategies))
	
	// Enforce limits
	if perStrategy > ca.config.MaxPerStrategy {
		perStrategy = ca.config.MaxPerStrategy
	}
	if perStrategy < ca.config.MinPerStrategy {
		perStrategy = ca.config.MinPerStrategy
	}

	newAllocations := make(map[string]*CapitalAllocation)
	for _, strategy := range strategies {
		newAllocations[strategy.ID] = &CapitalAllocation{
			StrategyID:   strategy.ID,
			StrategyType: strategy.Type,
			Amount:       perStrategy,
			Percentage:   perStrategy / ca.config.TotalCapital * 100,
			UpdatedAt:    time.Now(),
		}
	}

	ca.allocations = newAllocations
	return newAllocations
}

// riskAdjustedAllocation allocates capital based on risk-adjusted performance
func (ca *CapitalAllocator) riskAdjustedAllocation(strategies []*StrategyInstance) map[string]*CapitalAllocation {
	if len(strategies) == 0 {
		return ca.allocations
	}

	// Calculate risk-adjusted scores
	scores := make(map[string]float64)
	totalScore := 0.0

	for _, strategy := range strategies {
		score := ca.calculateRiskAdjustedScore(strategy)
		scores[strategy.ID] = score
		totalScore += score
	}

	// Allocate based on scores
	newAllocations := make(map[string]*CapitalAllocation)
	allocatedTotal := 0.0

	for _, strategy := range strategies {
		percentage := scores[strategy.ID] / totalScore
		amount := ca.config.TotalCapital * percentage

		// Enforce limits
		if amount > ca.config.MaxPerStrategy {
			amount = ca.config.MaxPerStrategy
		}
		if amount < ca.config.MinPerStrategy {
			amount = ca.config.MinPerStrategy
		}

		newAllocations[strategy.ID] = &CapitalAllocation{
			StrategyID:   strategy.ID,
			StrategyType: strategy.Type,
			Amount:       amount,
			Percentage:   amount / ca.config.TotalCapital * 100,
			UpdatedAt:    time.Now(),
		}

		allocatedTotal += amount
	}

	// Adjust for rounding errors
	if allocatedTotal < ca.config.TotalCapital {
		ca.available = ca.config.TotalCapital - allocatedTotal
	}

	ca.allocations = newAllocations
	return newAllocations
}

// calculateRiskAdjustedScore calculates a risk-adjusted score for a strategy
func (ca *CapitalAllocator) calculateRiskAdjustedScore(strategy *StrategyInstance) float64 {
	strategy.mu.RLock()
	metrics := strategy.Metrics
	strategy.mu.RUnlock()

	if metrics == nil {
		return 1.0 // Default score for new strategies
	}

	// Base score
	score := 1.0

	// Sharpe ratio contribution (higher is better)
	if metrics.SharpeRatio > 0 {
		score *= (1 + metrics.SharpeRatio*0.5)
	}

	// Win rate contribution
	if metrics.TotalTrades > 0 {
		winRate := float64(metrics.WinningTrades) / float64(metrics.TotalTrades)
		score *= (0.5 + winRate)
	}

	// Drawdown penalty (lower is better)
	if metrics.MaxDrawdown > 0 {
		score *= (1 - metrics.MaxDrawdown*0.5)
	}

	// Recent performance weight
	daysSinceStart := time.Since(strategy.StartedAt).Hours() / 24
	if daysSinceStart < 7 {
		// Reduce allocation for strategies with less than 7 days of data
		score *= 0.5
	}

	// Strategy type multiplier
	switch strategy.Type {
	case StrategyTypeArbitrage:
		score *= 1.2 // Arbitrage typically has better risk/reward
	case StrategyTypeMarketMaking:
		score *= 1.0
	default:
		score *= 0.8
	}

	return math.Max(score, 0.1) // Minimum score of 0.1
}

// GetAllocation returns the current allocation for a strategy
func (ca *CapitalAllocator) GetAllocation(strategyID string) (*CapitalAllocation, error) {
	ca.mu.RLock()
	defer ca.mu.RUnlock()

	allocation, exists := ca.allocations[strategyID]
	if !exists {
		return nil, fmt.Errorf("no allocation found for strategy %s", strategyID)
	}

	return allocation, nil
}

// GetTotalAllocated returns the total allocated capital
func (ca *CapitalAllocator) GetTotalAllocated() float64 {
	ca.mu.RLock()
	defer ca.mu.RUnlock()

	total := 0.0
	for _, allocation := range ca.allocations {
		total += allocation.Amount
	}

	return total
}

// GetAvailableCapital returns the available capital
func (ca *CapitalAllocator) GetAvailableCapital() float64 {
	ca.mu.RLock()
	defer ca.mu.RUnlock()

	return ca.available
}

// UpdateStrategyPerformance updates the performance metrics used for allocation
func (ca *CapitalAllocator) UpdateStrategyPerformance(strategyID string, metrics *StrategyMetrics) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	// This method would be called to update internal performance tracking
	// used for risk-adjusted allocation calculations
}

// GetAllocationSummary returns a summary of all allocations
func (ca *CapitalAllocator) GetAllocationSummary() map[string]interface{} {
	ca.mu.RLock()
	defer ca.mu.RUnlock()

	summary := map[string]interface{}{
		"total_capital":    ca.config.TotalCapital,
		"allocated":        ca.GetTotalAllocated(),
		"available":        ca.available,
		"allocation_count": len(ca.allocations),
		"allocations":      ca.allocations,
	}

	return summary
}