package account

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/mExOms/pkg/types"
)

// RotationStrategy defines how accounts are rotated
type RotationStrategy string

const (
	RotationStrategyRoundRobin RotationStrategy = "round_robin"
	RotationStrategyLeastUsed  RotationStrategy = "least_used"
	RotationStrategyWeighted   RotationStrategy = "weighted"
)

// AccountRotator manages account rotation for rate limit distribution
type AccountRotator struct {
	mu       sync.RWMutex
	manager  *Manager
	strategy RotationStrategy
	
	// Round-robin state
	roundRobinIndex map[string]int
	
	// Usage tracking
	accountUsage map[string]*UsageInfo
}

// UsageInfo tracks account usage for rotation
type UsageInfo struct {
	AccountID    string
	LastUsed     time.Time
	RequestCount int
	WeightUsed   int
	ErrorCount   int
}

// NewAccountRotator creates a new account rotator
func NewAccountRotator(manager *Manager, strategy RotationStrategy) *AccountRotator {
	return &AccountRotator{
		manager:         manager,
		strategy:        strategy,
		roundRobinIndex: make(map[string]int),
		accountUsage:    make(map[string]*UsageInfo),
	}
}

// GetNextAccount returns the next account to use based on rotation strategy
func (r *AccountRotator) GetNextAccount(exchange string, strategy string, requirements types.AccountRequirements) (*types.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get eligible accounts
	accounts := r.getEligibleAccounts(exchange, strategy, requirements)
	if len(accounts) == 0 {
		return nil, fmt.Errorf("no eligible accounts for rotation")
	}

	var selected *types.Account

	switch r.strategy {
	case RotationStrategyRoundRobin:
		selected = r.roundRobinSelect(accounts, strategy)
	case RotationStrategyLeastUsed:
		selected = r.leastUsedSelect(accounts)
	case RotationStrategyWeighted:
		selected = r.weightedSelect(accounts, requirements)
	default:
		selected = accounts[0]
	}

	// Update usage info
	r.updateUsage(selected.ID)

	return selected, nil
}

// GetAccountPool returns all accounts available for rotation
func (r *AccountRotator) GetAccountPool(exchange string, strategy string) ([]*types.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	filter := types.AccountFilter{
		Exchange: exchange,
		Strategy: strategy,
		Active:   &[]bool{true}[0],
	}

	return r.manager.ListAccounts(filter)
}

// ResetRotation resets rotation state for a strategy
func (r *AccountRotator) ResetRotation(strategy string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.roundRobinIndex, strategy)
	
	// Reset usage for accounts in this strategy
	for accountID, usage := range r.accountUsage {
		account, _ := r.manager.GetAccount(accountID)
		if account != nil && account.Strategy == strategy {
			usage.RequestCount = 0
			usage.WeightUsed = 0
			usage.ErrorCount = 0
		}
	}
}

// UpdateAccountHealth updates account health status based on errors
func (r *AccountRotator) UpdateAccountHealth(accountID string, success bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	usage, exists := r.accountUsage[accountID]
	if !exists {
		usage = &UsageInfo{AccountID: accountID}
		r.accountUsage[accountID] = usage
	}

	if !success {
		usage.ErrorCount++
		
		// Temporarily disable account if too many errors
		if usage.ErrorCount >= 5 {
			if account, err := r.manager.GetAccount(accountID); err == nil {
				account.Active = false
				r.manager.UpdateAccount(account)
			}
		}
	} else {
		// Reset error count on success
		usage.ErrorCount = 0
	}
}

// GetUsageStats returns usage statistics for all accounts
func (r *AccountRotator) GetUsageStats() map[string]*UsageInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[string]*UsageInfo)
	for k, v := range r.accountUsage {
		stats[k] = &UsageInfo{
			AccountID:    v.AccountID,
			LastUsed:     v.LastUsed,
			RequestCount: v.RequestCount,
			WeightUsed:   v.WeightUsed,
			ErrorCount:   v.ErrorCount,
		}
	}

	return stats
}

// Private methods

func (r *AccountRotator) getEligibleAccounts(exchange string, strategy string, requirements types.AccountRequirements) []*types.Account {
	filter := types.AccountFilter{
		Exchange: exchange,
		Strategy: strategy,
		Active:   &[]bool{true}[0],
		Market:   requirements.Market,
	}

	accounts, _ := r.manager.ListAccounts(filter)

	// Filter by requirements
	var eligible []*types.Account
	for _, account := range accounts {
		// Check rate limit
		if !r.manager.hasAvailableRateLimit(account.ID, requirements.RequiredWeight) {
			continue
		}

		// Check balance if required
		if !requirements.MinBalance.IsZero() {
			balance, _ := r.manager.GetBalance(account.ID)
			if balance == nil || balance.TotalUSDT.LessThan(requirements.MinBalance) {
				continue
			}
		}

		// Check error threshold
		if usage, exists := r.accountUsage[account.ID]; exists {
			if usage.ErrorCount >= 3 {
				continue // Skip accounts with recent errors
			}
		}

		eligible = append(eligible, account)
	}

	return eligible
}

func (r *AccountRotator) roundRobinSelect(accounts []*types.Account, strategy string) *types.Account {
	index, exists := r.roundRobinIndex[strategy]
	if !exists {
		index = 0
	}

	selected := accounts[index%len(accounts)]
	r.roundRobinIndex[strategy] = index + 1

	return selected
}

func (r *AccountRotator) leastUsedSelect(accounts []*types.Account) *types.Account {
	// Sort by usage (least used first)
	sort.Slice(accounts, func(i, j int) bool {
		usageI := r.getUsageInfo(accounts[i].ID)
		usageJ := r.getUsageInfo(accounts[j].ID)

		// First by request count
		if usageI.RequestCount != usageJ.RequestCount {
			return usageI.RequestCount < usageJ.RequestCount
		}

		// Then by last used time (older first)
		return usageI.LastUsed.Before(usageJ.LastUsed)
	})

	return accounts[0]
}

func (r *AccountRotator) weightedSelect(accounts []*types.Account, requirements types.AccountRequirements) *types.Account {
	type weightedAccount struct {
		account *types.Account
		weight  float64
	}

	weighted := make([]weightedAccount, len(accounts))

	for i, account := range accounts {
		weight := r.calculateWeight(account, requirements)
		weighted[i] = weightedAccount{
			account: account,
			weight:  weight,
		}
	}

	// Sort by weight (highest first)
	sort.Slice(weighted, func(i, j int) bool {
		return weighted[i].weight > weighted[j].weight
	})

	return weighted[0].account
}

func (r *AccountRotator) calculateWeight(account *types.Account, requirements types.AccountRequirements) float64 {
	weight := 100.0

	// Factor in rate limit availability
	available := r.manager.getAvailableWeight(account.ID)
	total := account.RateLimitWeight
	if total > 0 {
		weight *= float64(available) / float64(total)
	}

	// Factor in recent usage
	usage := r.getUsageInfo(account.ID)
	timeSinceUse := time.Since(usage.LastUsed)
	if timeSinceUse < 1*time.Minute {
		weight *= 0.5
	} else if timeSinceUse < 5*time.Minute {
		weight *= 0.8
	}

	// Factor in error rate
	if usage.RequestCount > 0 {
		errorRate := float64(usage.ErrorCount) / float64(usage.RequestCount)
		weight *= (1 - errorRate)
	}

	// Bonus for specific account types
	if account.Type == types.AccountTypeStrategy {
		weight *= 1.2
	}

	return weight
}

func (r *AccountRotator) updateUsage(accountID string) {
	usage, exists := r.accountUsage[accountID]
	if !exists {
		usage = &UsageInfo{AccountID: accountID}
		r.accountUsage[accountID] = usage
	}

	usage.LastUsed = time.Now()
	usage.RequestCount++
}

func (r *AccountRotator) getUsageInfo(accountID string) *UsageInfo {
	usage, exists := r.accountUsage[accountID]
	if !exists {
		return &UsageInfo{
			AccountID: accountID,
			LastUsed:  time.Time{},
		}
	}
	return usage
}

// RotationScheduler manages periodic account rotation tasks
type RotationScheduler struct {
	rotator  *AccountRotator
	manager  *Manager
	interval time.Duration
	stop     chan struct{}
	wg       sync.WaitGroup
}

// NewRotationScheduler creates a new rotation scheduler
func NewRotationScheduler(rotator *AccountRotator, manager *Manager, interval time.Duration) *RotationScheduler {
	return &RotationScheduler{
		rotator:  rotator,
		manager:  manager,
		interval: interval,
		stop:     make(chan struct{}),
	}
}

// Start begins the rotation scheduler
func (rs *RotationScheduler) Start() {
	rs.wg.Add(1)
	go rs.run()
}

// Stop halts the rotation scheduler
func (rs *RotationScheduler) Stop() {
	close(rs.stop)
	rs.wg.Wait()
}

func (rs *RotationScheduler) run() {
	defer rs.wg.Done()

	ticker := time.NewTicker(rs.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rs.performRotationTasks()
		case <-rs.stop:
			return
		}
	}
}

func (rs *RotationScheduler) performRotationTasks() {
	// Reset rate limits for inactive accounts
	rs.resetInactiveRateLimits()

	// Re-enable accounts with cleared errors
	rs.reenableHealthyAccounts()

	// Clean up old usage data
	rs.cleanupUsageData()
}

func (rs *RotationScheduler) resetInactiveRateLimits() {
	now := time.Now()
	
	for accountID, usage := range rs.rotator.accountUsage {
		if now.Sub(usage.LastUsed) > 5*time.Minute {
			// Reset rate limit tracker
			rs.manager.mu.Lock()
			delete(rs.manager.rateLimitTracker, accountID)
			rs.manager.mu.Unlock()
			
			// Reset weight used
			usage.WeightUsed = 0
		}
	}
}

func (rs *RotationScheduler) reenableHealthyAccounts() {
	for accountID, usage := range rs.rotator.accountUsage {
		// Re-enable accounts that have been error-free for 10 minutes
		if usage.ErrorCount == 0 && time.Since(usage.LastUsed) > 10*time.Minute {
			if account, err := rs.manager.GetAccount(accountID); err == nil && !account.Active {
				account.Active = true
				rs.manager.UpdateAccount(account)
			}
		}
	}
}

func (rs *RotationScheduler) cleanupUsageData() {
	// Remove usage data for deleted accounts
	rs.rotator.mu.Lock()
	defer rs.rotator.mu.Unlock()

	for accountID := range rs.rotator.accountUsage {
		if _, err := rs.manager.GetAccount(accountID); err != nil {
			delete(rs.rotator.accountUsage, accountID)
		}
	}
}