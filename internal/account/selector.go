package account

import (
	"fmt"
	"sort"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// Selector implements account selection strategies
type Selector struct {
	manager *Manager
}

// NewSelector creates a new account selector
func NewSelector(manager *Manager) *Selector {
	return &Selector{
		manager: manager,
	}
}

// SelectAccount selects the best account for given strategy and requirements
func (s *Selector) SelectAccount(strategy string, requirements types.AccountRequirements) (*types.Account, error) {
	s.manager.mu.RLock()
	defer s.manager.mu.RUnlock()

	// Get candidate accounts
	candidates := s.filterCandidates(strategy, requirements)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no suitable accounts found for strategy: %s", strategy)
	}

	// Score and rank candidates
	scoredAccounts := s.scoreAccounts(candidates, requirements)
	
	// Sort by score (highest first)
	sort.Slice(scoredAccounts, func(i, j int) bool {
		return scoredAccounts[i].score > scoredAccounts[j].score
	})

	// Return best account
	best := scoredAccounts[0]
	
	// Update last used time
	best.account.LastUsed = time.Now()
	
	// Update rate limit
	s.manager.UpdateRateLimit(best.account.ID, requirements.RequiredWeight)

	return best.account, nil
}

// SelectAccountForOrder selects the best account for a specific order
func (s *Selector) SelectAccountForOrder(order *types.Order) (*types.Account, error) {
	// Determine requirements from order
	market := types.MarketTypeSpot
	if order.MarginType != "" {
		market = types.MarketTypeFutures
	}

	orderValue := order.Quantity.Mul(order.Price)
	
	requirements := types.AccountRequirements{
		Market:         market,
		Symbol:         order.Symbol,
		OrderSize:      orderValue,
		RequiredWeight: s.calculateOrderWeight(order),
	}

	// Extract strategy from order metadata
	strategy := ""
	if order.Metadata != nil {
		if s, ok := order.Metadata["strategy"].(string); ok {
			strategy = s
		}
	}

	return s.SelectAccount(strategy, requirements)
}

// filterCandidates filters accounts based on requirements
func (s *Selector) filterCandidates(strategy string, requirements types.AccountRequirements) []*types.Account {
	var candidates []*types.Account

	for _, account := range s.manager.accounts {
		// Skip inactive accounts
		if !account.Active {
			continue
		}

		// Check strategy match
		if strategy != "" && account.Strategy != strategy {
			continue
		}

		// Check market type
		if requirements.Market == types.MarketTypeSpot && !account.SpotEnabled {
			continue
		}
		if requirements.Market == types.MarketTypeFutures && !account.FuturesEnabled {
			continue
		}

		// Check balance requirements
		if !requirements.MinBalance.IsZero() {
			balance := s.manager.balances[account.ID]
			if balance == nil || balance.TotalUSDT.LessThan(requirements.MinBalance) {
				continue
			}
		}

		// Check max balance
		if !requirements.MaxBalance.IsZero() {
			balance := s.manager.balances[account.ID]
			if balance != nil && balance.TotalUSDT.GreaterThan(requirements.MaxBalance) {
				continue
			}
		}

		// Check rate limit availability
		if !s.hasAvailableRateLimit(account, requirements.RequiredWeight) {
			continue
		}

		// Check position limits
		if !requirements.OrderSize.IsZero() && account.MaxPositionUSDT.GreaterThan(decimal.Zero) {
			metrics := s.manager.metrics[account.ID]
			if metrics != nil {
				// Calculate current exposure
				positions := s.manager.positions[account.ID]
				currentExposure := s.calculateExposure(positions)
				
				if currentExposure.Add(requirements.OrderSize).GreaterThan(account.MaxPositionUSDT) {
					continue
				}
			}
		}

		// Check leverage requirements
		if requirements.Leverage > 0 && account.MaxLeverage > 0 {
			if requirements.Leverage > account.MaxLeverage {
				continue
			}
		}

		candidates = append(candidates, account)
	}

	return candidates
}

// scoredAccount holds an account with its selection score
type scoredAccount struct {
	account *types.Account
	score   float64
}

// scoreAccounts scores accounts based on various factors
func (s *Selector) scoreAccounts(accounts []*types.Account, requirements types.AccountRequirements) []scoredAccount {
	var scored []scoredAccount

	for _, account := range accounts {
		score := s.calculateScore(account, requirements)
		scored = append(scored, scoredAccount{
			account: account,
			score:   score,
		})
	}

	return scored
}

// calculateScore calculates a selection score for an account
func (s *Selector) calculateScore(account *types.Account, requirements types.AccountRequirements) float64 {
	score := 100.0

	// Factor 1: Rate limit availability (40% weight)
	rateLimitScore := s.calculateRateLimitScore(account, requirements.RequiredWeight)
	score += rateLimitScore * 0.4

	// Factor 2: Balance optimization (20% weight)
	balanceScore := s.calculateBalanceScore(account, requirements)
	score += balanceScore * 0.2

	// Factor 3: Recent usage (20% weight)
	usageScore := s.calculateUsageScore(account)
	score += usageScore * 0.2

	// Factor 4: Performance metrics (10% weight)
	performanceScore := s.calculatePerformanceScore(account)
	score += performanceScore * 0.1

	// Factor 5: Risk utilization (10% weight)
	riskScore := s.calculateRiskScore(account, requirements)
	score += riskScore * 0.1

	// Bonus for exact strategy match
	if account.Strategy != "" && account.Strategy == getStrategyFromRequirements(requirements) {
		score += 10
	}

	// Penalty for sub accounts when main is preferred
	if account.Type == types.AccountTypeSub {
		score -= 5
	}

	return score
}

// calculateRateLimitScore scores based on available rate limit
func (s *Selector) calculateRateLimitScore(account *types.Account, requiredWeight int) float64 {
	available := s.getAvailableWeight(account)
	total := account.RateLimitWeight

	if total == 0 {
		return 100 // No rate limit
	}

	// Percentage of available weight
	percentAvailable := float64(available) / float64(total) * 100
	
	// Penalty if close to limit
	if available < requiredWeight*2 {
		percentAvailable *= 0.5
	}

	return percentAvailable
}

// calculateBalanceScore scores based on balance suitability
func (s *Selector) calculateBalanceScore(account *types.Account, requirements types.AccountRequirements) float64 {
	balance := s.manager.balances[account.ID]
	if balance == nil {
		return 0
	}

	// Ideal balance is 2-5x the order size
	if !requirements.OrderSize.IsZero() {
		idealMin := requirements.OrderSize.Mul(decimal.NewFromInt(2))
		idealMax := requirements.OrderSize.Mul(decimal.NewFromInt(5))

		if balance.TotalUSDT.GreaterThanOrEqual(idealMin) && balance.TotalUSDT.LessThanOrEqual(idealMax) {
			return 100
		}

		// Too much balance is less optimal (capital efficiency)
		if balance.TotalUSDT.GreaterThan(idealMax) {
			excess := balance.TotalUSDT.Sub(idealMax).Div(idealMax).InexactFloat64()
			return 100 - (excess * 10) // Reduce score by 10 points per 100% excess
		}
	}

	return 50 // Default middle score
}

// calculateUsageScore scores based on recent usage patterns
func (s *Selector) calculateUsageScore(account *types.Account) float64 {
	timeSinceLastUse := time.Since(account.LastUsed)

	// Prefer accounts that haven't been used recently (load distribution)
	if timeSinceLastUse > 5*time.Minute {
		return 100
	} else if timeSinceLastUse > 1*time.Minute {
		return 80
	} else if timeSinceLastUse > 30*time.Second {
		return 60
	} else if timeSinceLastUse > 10*time.Second {
		return 40
	}

	return 20 // Recently used
}

// calculatePerformanceScore scores based on account performance metrics
func (s *Selector) calculatePerformanceScore(account *types.Account) float64 {
	metrics := s.manager.metrics[account.ID]
	if metrics == nil {
		return 50 // No metrics, neutral score
	}

	score := 50.0

	// Win rate contribution
	if metrics.WinRate > 0 {
		score += (metrics.WinRate - 0.5) * 50 // 50-100 for win rate > 50%
	}

	// PnL contribution
	if metrics.TotalPnL.IsPositive() {
		score += 10
	} else if metrics.TotalPnL.IsNegative() {
		score -= 10
	}

	// Daily loss limit check
	if account.DailyLossLimit.GreaterThan(decimal.Zero) && metrics.TodayPnL.IsNegative() {
		lossPercent := metrics.TodayPnL.Abs().Div(account.DailyLossLimit).InexactFloat64()
		if lossPercent > 0.8 {
			score -= 30 // Heavy penalty when close to daily loss limit
		} else if lossPercent > 0.5 {
			score -= 15
		}
	}

	return score
}

// calculateRiskScore scores based on risk utilization
func (s *Selector) calculateRiskScore(account *types.Account, requirements types.AccountRequirements) float64 {
	positions := s.manager.positions[account.ID]
	if positions == nil {
		return 100 // No positions, full score
	}

	currentExposure := s.calculateExposure(positions)
	
	// Check against max position limit
	if account.MaxPositionUSDT.GreaterThan(decimal.Zero) {
		utilization := currentExposure.Div(account.MaxPositionUSDT).InexactFloat64()
		
		// Lower utilization is better for new positions
		if utilization < 0.3 {
			return 100
		} else if utilization < 0.5 {
			return 80
		} else if utilization < 0.7 {
			return 60
		} else if utilization < 0.9 {
			return 40
		}
		
		return 20 // High utilization
	}

	return 80 // No limit set, slightly reduced score
}

// Helper methods

func (s *Selector) hasAvailableRateLimit(account *types.Account, requiredWeight int) bool {
	rl, exists := s.manager.rateLimitTracker[account.ID]
	
	if !exists {
		return true
	}

	// Check if window has expired
	if time.Since(rl.WindowStart) > time.Minute {
		return true
	}

	return rl.UsedWeight+requiredWeight <= account.RateLimitWeight
}

func (s *Selector) getAvailableWeight(account *types.Account) int {
	rl, exists := s.manager.rateLimitTracker[account.ID]
	
	if !exists || time.Since(rl.WindowStart) > time.Minute {
		return account.RateLimitWeight
	}

	return account.RateLimitWeight - rl.UsedWeight
}

func (s *Selector) calculateExposure(positions *types.AccountPosition) decimal.Decimal {
	if positions == nil || len(positions.Positions) == 0 {
		return decimal.Zero
	}

	totalExposure := decimal.Zero
	for _, pos := range positions.Positions {
		if pos != nil {
			exposure := pos.Quantity.Abs().Mul(pos.EntryPrice)
			totalExposure = totalExposure.Add(exposure)
		}
	}

	return totalExposure
}

func (s *Selector) calculateOrderWeight(order *types.Order) int {
	weight := 1 // Base weight

	// Add weight for order type
	switch order.Type {
	case types.OrderTypeMarket:
		weight += 1
	case types.OrderTypeLimit:
		weight += 0
	case types.OrderTypeStopLoss, types.OrderTypeTakeProfit:
		weight += 2
	}

	// Add weight for order size (large orders may use more weight)
	// This is exchange-specific, simplified here
	
	return weight
}

func getStrategyFromRequirements(requirements types.AccountRequirements) string {
	// Extract strategy from requirements if stored in metadata
	// This is a placeholder - implement based on actual requirements structure
	return ""
}