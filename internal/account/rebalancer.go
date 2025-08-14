package account

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/mExOms/oms/pkg/types"
	"github.com/shopspring/decimal"
)

// Rebalancer implements advanced rebalancing strategies
type Rebalancer struct {
	mu sync.RWMutex
	
	transferManager *TransferManager
	accountManager  *Manager
	
	// Strategies
	strategies map[string]RebalanceStrategy
	
	// Configuration
	config *RebalancerConfig
	
	// Metrics
	metrics *RebalanceMetrics
}

// RebalancerConfig contains rebalancer configuration
type RebalancerConfig struct {
	// General settings
	Enabled              bool
	DryRun               bool
	MaxTransfersPerRun   int
	MinTransferAmount    decimal.Decimal
	
	// Strategy weights
	StrategyWeights      map[string]float64
	
	// Risk parameters
	MaxImbalanceRatio    float64 // e.g., 0.3 = 30% max deviation
	SafetyMargin         float64 // e.g., 0.1 = keep 10% buffer
	
	// Timing
	QuietPeriodStart     time.Time // Don't rebalance during active trading
	QuietPeriodEnd       time.Time
}

// RebalanceStrategy defines a rebalancing strategy interface
type RebalanceStrategy interface {
	Name() string
	Priority() int
	Analyze(accounts []*types.Account) (*RebalanceAnalysis, error)
	GenerateTransfers(analysis *RebalanceAnalysis, accounts []*types.Account) ([]*TransferRequest, error)
}

// RebalanceAnalysis contains analysis results
type RebalanceAnalysis struct {
	Strategy         string
	Timestamp        time.Time
	TotalBalance     decimal.Decimal
	ImbalanceRatio   float64
	Recommendations  []RebalanceRecommendation
	RequiresAction   bool
}

// RebalanceRecommendation represents a recommended action
type RebalanceRecommendation struct {
	FromAccount     string
	ToAccount       string
	Amount          decimal.Decimal
	Reason          string
	Priority        int
	ExpectedBenefit string
}

// RebalanceMetrics tracks rebalancing performance
type RebalanceMetrics struct {
	mu sync.RWMutex
	
	RunsTotal           int
	RunsSuccessful      int
	TransfersTotal      int
	TransfersSuccessful int
	TotalVolumeUSDT     decimal.Decimal
	LastRunTime         time.Time
	AverageRunDuration  time.Duration
}

// NewRebalancer creates a new rebalancer
func NewRebalancer(transferManager *TransferManager, accountManager *Manager, config *RebalancerConfig) *Rebalancer {
	if config == nil {
		config = &RebalancerConfig{
			Enabled:            true,
			DryRun:             false,
			MaxTransfersPerRun: 10,
			MinTransferAmount:  decimal.NewFromInt(100),
			MaxImbalanceRatio:  0.3,
			SafetyMargin:       0.1,
			StrategyWeights: map[string]float64{
				"proportional": 0.4,
				"performance":  0.3,
				"ratelimit":    0.3,
			},
		}
	}
	
	r := &Rebalancer{
		transferManager: transferManager,
		accountManager:  accountManager,
		strategies:      make(map[string]RebalanceStrategy),
		config:          config,
		metrics:         &RebalanceMetrics{},
	}
	
	// Register default strategies
	r.RegisterStrategy(NewProportionalStrategy())
	r.RegisterStrategy(NewPerformanceBasedStrategy())
	r.RegisterStrategy(NewRateLimitOptimizationStrategy())
	
	return r
}

// RegisterStrategy registers a rebalancing strategy
func (r *Rebalancer) RegisterStrategy(strategy RebalanceStrategy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.strategies[strategy.Name()] = strategy
}

// RunRebalancing executes rebalancing analysis and transfers
func (r *Rebalancer) RunRebalancing(ctx context.Context, exchange string) (*RebalanceResult, error) {
	startTime := time.Now()
	
	r.mu.RLock()
	if !r.config.Enabled {
		r.mu.RUnlock()
		return nil, fmt.Errorf("rebalancing is disabled")
	}
	
	// Check quiet period
	now := time.Now()
	if !r.config.QuietPeriodStart.IsZero() && !r.config.QuietPeriodEnd.IsZero() {
		quietStart := time.Date(now.Year(), now.Month(), now.Day(),
			r.config.QuietPeriodStart.Hour(), r.config.QuietPeriodStart.Minute(), 0, 0, now.Location())
		quietEnd := time.Date(now.Year(), now.Month(), now.Day(),
			r.config.QuietPeriodEnd.Hour(), r.config.QuietPeriodEnd.Minute(), 0, 0, now.Location())
		
		if now.After(quietStart) && now.Before(quietEnd) {
			r.mu.RUnlock()
			return nil, fmt.Errorf("in quiet period")
		}
	}
	r.mu.RUnlock()
	
	// Get accounts for exchange
	filter := types.AccountFilter{
		Exchange: exchange,
		Active:   &[]bool{true}[0],
	}
	
	accounts, err := r.accountManager.ListAccounts(filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}
	
	if len(accounts) < 2 {
		return nil, fmt.Errorf("need at least 2 accounts for rebalancing")
	}
	
	// Update account balances
	for _, account := range accounts {
		// In production, fetch fresh balances from exchange
		_, err := r.accountManager.GetBalance(account.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get balance for %s: %w", account.ID, err)
		}
	}
	
	// Run analysis with all strategies
	analyses := make([]*RebalanceAnalysis, 0)
	transfers := make([]*TransferRequest, 0)
	
	for name, strategy := range r.strategies {
		weight, hasWeight := r.config.StrategyWeights[name]
		if !hasWeight || weight <= 0 {
			continue
		}
		
		// Analyze
		analysis, err := strategy.Analyze(accounts)
		if err != nil {
			continue
		}
		
		if analysis.RequiresAction {
			analyses = append(analyses, analysis)
			
			// Generate transfers
			strategyTransfers, err := strategy.GenerateTransfers(analysis, accounts)
			if err == nil {
				// Apply weight to priority
				for _, transfer := range strategyTransfers {
					transfer.Priority = int(float64(transfer.Priority) * weight)
				}
				transfers = append(transfers, strategyTransfers...)
			}
		}
	}
	
	// Consolidate and prioritize transfers
	transfers = r.consolidateTransfers(transfers)
	
	// Apply limits
	if len(transfers) > r.config.MaxTransfersPerRun {
		transfers = transfers[:r.config.MaxTransfersPerRun]
	}
	
	// Execute transfers
	executed := make([]*types.AccountTransfer, 0)
	failed := make([]error, 0)
	
	for _, req := range transfers {
		// Skip if below minimum
		if req.Amount.LessThan(r.config.MinTransferAmount) {
			continue
		}
		
		if r.config.DryRun {
			// Log what would be done
			fmt.Printf("DRY RUN: Would transfer %s %s from %s to %s (reason: %s)\n",
				req.Amount.String(), req.Asset, req.FromAccount, req.ToAccount, req.Reason)
			continue
		}
		
		// Execute transfer
		transfer, err := r.transferManager.RequestTransfer(ctx, req)
		if err != nil {
			failed = append(failed, err)
			continue
		}
		
		if err := r.transferManager.ExecuteTransfer(ctx, transfer.ID); err != nil {
			failed = append(failed, err)
			continue
		}
		
		executed = append(executed, transfer)
	}
	
	// Update metrics
	r.updateMetrics(len(executed), len(failed), executed, time.Since(startTime))
	
	return &RebalanceResult{
		Timestamp:        startTime,
		Exchange:         exchange,
		AccountsAnalyzed: len(accounts),
		Analyses:         analyses,
		TransfersPlanned: len(transfers),
		TransfersExecuted: len(executed),
		TransfersFailed:  len(failed),
		Errors:           failed,
		Duration:         time.Since(startTime),
		DryRun:           r.config.DryRun,
	}, nil
}

// consolidateTransfers merges and prioritizes transfers
func (r *Rebalancer) consolidateTransfers(transfers []*TransferRequest) []*TransferRequest {
	// Group transfers by from-to pair
	grouped := make(map[string][]*TransferRequest)
	
	for _, transfer := range transfers {
		key := fmt.Sprintf("%s->%s:%s", transfer.FromAccount, transfer.ToAccount, transfer.Asset)
		grouped[key] = append(grouped[key], transfer)
	}
	
	// Consolidate each group
	consolidated := make([]*TransferRequest, 0)
	
	for _, group := range grouped {
		if len(group) == 1 {
			consolidated = append(consolidated, group[0])
			continue
		}
		
		// Merge transfers
		merged := &TransferRequest{
			FromAccount: group[0].FromAccount,
			ToAccount:   group[0].ToAccount,
			Asset:       group[0].Asset,
			Amount:      decimal.Zero,
			Priority:    0,
			Reason:      "consolidated",
		}
		
		reasons := make([]string, 0)
		for _, transfer := range group {
			merged.Amount = merged.Amount.Add(transfer.Amount)
			if transfer.Priority > merged.Priority {
				merged.Priority = transfer.Priority
			}
			reasons = append(reasons, transfer.Reason)
		}
		
		merged.Reason = fmt.Sprintf("consolidated: %v", reasons)
		consolidated = append(consolidated, merged)
	}
	
	// Sort by priority (highest first)
	sort.Slice(consolidated, func(i, j int) bool {
		return consolidated[i].Priority > consolidated[j].Priority
	})
	
	return consolidated
}

// updateMetrics updates rebalancing metrics
func (r *Rebalancer) updateMetrics(successful, failed int, transfers []*types.AccountTransfer, duration time.Duration) {
	r.metrics.mu.Lock()
	defer r.metrics.mu.Unlock()
	
	r.metrics.RunsTotal++
	if failed == 0 && successful > 0 {
		r.metrics.RunsSuccessful++
	}
	
	r.metrics.TransfersTotal += successful + failed
	r.metrics.TransfersSuccessful += successful
	
	// Calculate total volume
	for _, transfer := range transfers {
		r.metrics.TotalVolumeUSDT = r.metrics.TotalVolumeUSDT.Add(transfer.Amount)
	}
	
	r.metrics.LastRunTime = time.Now()
	
	// Update average duration
	if r.metrics.RunsTotal == 1 {
		r.metrics.AverageRunDuration = duration
	} else {
		total := r.metrics.AverageRunDuration * time.Duration(r.metrics.RunsTotal-1)
		r.metrics.AverageRunDuration = (total + duration) / time.Duration(r.metrics.RunsTotal)
	}
}

// GetMetrics returns rebalancing metrics
func (r *Rebalancer) GetMetrics() RebalanceMetrics {
	r.metrics.mu.RLock()
	defer r.metrics.mu.RUnlock()
	
	return *r.metrics
}

// RebalanceResult contains rebalancing results
type RebalanceResult struct {
	Timestamp         time.Time
	Exchange          string
	AccountsAnalyzed  int
	Analyses          []*RebalanceAnalysis
	TransfersPlanned  int
	TransfersExecuted int
	TransfersFailed   int
	Errors            []error
	Duration          time.Duration
	DryRun            bool
}

// Default Strategies

// ProportionalStrategy maintains proportional balance distribution
type ProportionalStrategy struct{}

func NewProportionalStrategy() *ProportionalStrategy {
	return &ProportionalStrategy{}
}

func (s *ProportionalStrategy) Name() string { return "proportional" }
func (s *ProportionalStrategy) Priority() int { return 100 }

func (s *ProportionalStrategy) Analyze(accounts []*types.Account) (*RebalanceAnalysis, error) {
	// Calculate total balance and target per account
	totalBalance := decimal.Zero
	accountBalances := make(map[string]decimal.Decimal)
	
	for _, account := range accounts {
		balance, err := account.GetBalance()
		if err != nil {
			continue
		}
		accountBalances[account.ID] = balance.TotalUSDT
		totalBalance = totalBalance.Add(balance.TotalUSDT)
	}
	
	if totalBalance.IsZero() {
		return &RebalanceAnalysis{
			Strategy:       s.Name(),
			Timestamp:      time.Now(),
			RequiresAction: false,
		}, nil
	}
	
	// Calculate target balance per account
	targetBalance := totalBalance.Div(decimal.NewFromInt(int64(len(accounts))))
	
	// Check imbalance
	maxDeviation := decimal.Zero
	recommendations := make([]RebalanceRecommendation, 0)
	
	for _, account := range accounts {
		balance := accountBalances[account.ID]
		deviation := balance.Sub(targetBalance).Abs()
		deviationRatio := deviation.Div(targetBalance)
		
		if deviationRatio.GreaterThan(maxDeviation) {
			maxDeviation = deviationRatio
		}
		
		// Generate recommendations for accounts far from target
		if deviationRatio.GreaterThan(decimal.NewFromFloat(0.2)) { // 20% deviation
			if balance.GreaterThan(targetBalance) {
				// Account has excess
				excess := balance.Sub(targetBalance).Mul(decimal.NewFromFloat(0.8)) // Move 80% of excess
				recommendations = append(recommendations, RebalanceRecommendation{
					FromAccount: account.ID,
					Amount:      excess,
					Reason:      "excess_balance",
					Priority:    int(deviationRatio.Mul(decimal.NewFromInt(100)).IntPart()),
				})
			} else {
				// Account has deficit
				deficit := targetBalance.Sub(balance).Mul(decimal.NewFromFloat(0.8))
				recommendations = append(recommendations, RebalanceRecommendation{
					ToAccount: account.ID,
					Amount:    deficit,
					Reason:    "deficit_balance",
					Priority:  int(deviationRatio.Mul(decimal.NewFromInt(100)).IntPart()),
				})
			}
		}
	}
	
	return &RebalanceAnalysis{
		Strategy:        s.Name(),
		Timestamp:       time.Now(),
		TotalBalance:    totalBalance,
		ImbalanceRatio:  maxDeviation.InexactFloat64(),
		Recommendations: recommendations,
		RequiresAction:  len(recommendations) > 0,
	}, nil
}

func (s *ProportionalStrategy) GenerateTransfers(analysis *RebalanceAnalysis, accounts []*types.Account) ([]*TransferRequest, error) {
	// Match sources and destinations
	sources := make([]RebalanceRecommendation, 0)
	destinations := make([]RebalanceRecommendation, 0)
	
	for _, rec := range analysis.Recommendations {
		if rec.FromAccount != "" {
			sources = append(sources, rec)
		} else if rec.ToAccount != "" {
			destinations = append(destinations, rec)
		}
	}
	
	transfers := make([]*TransferRequest, 0)
	
	// Simple matching algorithm
	for len(sources) > 0 && len(destinations) > 0 {
		source := sources[0]
		dest := destinations[0]
		
		amount := source.Amount
		if dest.Amount.LessThan(amount) {
			amount = dest.Amount
		}
		
		transfers = append(transfers, &TransferRequest{
			FromAccount: source.FromAccount,
			ToAccount:   dest.ToAccount,
			Asset:       "USDT",
			Amount:      amount,
			Reason:      fmt.Sprintf("proportional_rebalance: %s->%s", source.Reason, dest.Reason),
			Priority:    (source.Priority + dest.Priority) / 2,
		})
		
		// Update remaining amounts
		source.Amount = source.Amount.Sub(amount)
		dest.Amount = dest.Amount.Sub(amount)
		
		if source.Amount.IsZero() {
			sources = sources[1:]
		} else {
			sources[0] = source
		}
		
		if dest.Amount.IsZero() {
			destinations = destinations[1:]
		} else {
			destinations[0] = dest
		}
	}
	
	return transfers, nil
}

// PerformanceBasedStrategy allocates more to better performing accounts
type PerformanceBasedStrategy struct{}

func NewPerformanceBasedStrategy() *PerformanceBasedStrategy {
	return &PerformanceBasedStrategy{}
}

func (s *PerformanceBasedStrategy) Name() string { return "performance" }
func (s *PerformanceBasedStrategy) Priority() int { return 80 }

func (s *PerformanceBasedStrategy) Analyze(accounts []*types.Account) (*RebalanceAnalysis, error) {
	// This is a placeholder - implement based on trading performance metrics
	return &RebalanceAnalysis{
		Strategy:       s.Name(),
		Timestamp:      time.Now(),
		RequiresAction: false,
	}, nil
}

func (s *PerformanceBasedStrategy) GenerateTransfers(analysis *RebalanceAnalysis, accounts []*types.Account) ([]*TransferRequest, error) {
	// Implement performance-based transfer generation
	return []*TransferRequest{}, nil
}

// RateLimitOptimizationStrategy optimizes for rate limit usage
type RateLimitOptimizationStrategy struct{}

func NewRateLimitOptimizationStrategy() *RateLimitOptimizationStrategy {
	return &RateLimitOptimizationStrategy{}
}

func (s *RateLimitOptimizationStrategy) Name() string { return "ratelimit" }
func (s *RateLimitOptimizationStrategy) Priority() int { return 60 }

func (s *RateLimitOptimizationStrategy) Analyze(accounts []*types.Account) (*RebalanceAnalysis, error) {
	// Analyze rate limit usage patterns and recommend transfers
	// to accounts with more available rate limit capacity
	return &RebalanceAnalysis{
		Strategy:       s.Name(),
		Timestamp:      time.Now(),
		RequiresAction: false,
	}, nil
}

func (s *RateLimitOptimizationStrategy) GenerateTransfers(analysis *RebalanceAnalysis, accounts []*types.Account) ([]*TransferRequest, error) {
	// Implement rate limit optimization transfers
	return []*TransferRequest{}, nil
}