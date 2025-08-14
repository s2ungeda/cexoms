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

// Router routes orders to optimal accounts based on various criteria
type Router struct {
	mu sync.RWMutex
	
	manager      *Manager
	exchanges    map[string]types.ExchangeMultiAccount
	rules        []RoutingRule
	metrics      map[string]*RoutingMetrics
	config       *RouterConfig
}

// RouterConfig contains router configuration
type RouterConfig struct {
	// Selection strategy
	Strategy             SelectionStrategy
	LoadBalancingEnabled bool
	
	// Rate limit management
	RateLimitBuffer      int           // Reserve buffer (e.g., 200 weight)
	RotationCooldown     time.Duration // Cooldown after rotation
	
	// Performance tracking
	MetricsWindow        time.Duration
	MetricsRetention     time.Duration
}

// SelectionStrategy defines how accounts are selected
type SelectionStrategy string

const (
	StrategyLeastUsed     SelectionStrategy = "least_used"
	StrategyRoundRobin    SelectionStrategy = "round_robin"
	StrategyBestFit       SelectionStrategy = "best_fit"
	StrategyLowestLatency SelectionStrategy = "lowest_latency"
)

// RoutingRule defines custom routing rules
type RoutingRule struct {
	Name      string
	Priority  int
	Condition func(order *types.Order) bool
	Selector  func(accounts []*types.Account) *types.Account
}

// RoutingMetrics tracks routing performance
type RoutingMetrics struct {
	AccountID       string
	OrdersRouted    int
	SuccessRate     float64
	AvgLatency      time.Duration
	LastUsed        time.Time
	ConsecutiveFails int
}

// NewRouter creates a new account router
func NewRouter(manager *Manager, config *RouterConfig) *Router {
	if config == nil {
		config = &RouterConfig{
			Strategy:         StrategyBestFit,
			RateLimitBuffer:  200,
			RotationCooldown: 5 * time.Minute,
			MetricsWindow:    1 * time.Hour,
			MetricsRetention: 24 * time.Hour,
		}
	}
	
	r := &Router{
		manager:   manager,
		exchanges: make(map[string]types.ExchangeMultiAccount),
		rules:     make([]RoutingRule, 0),
		metrics:   make(map[string]*RoutingMetrics),
		config:    config,
	}
	
	// Add default routing rules
	r.addDefaultRules()
	
	// Start metrics cleanup
	go r.cleanupMetricsLoop()
	
	return r
}

// RegisterExchange registers an exchange with multi-account support
func (r *Router) RegisterExchange(name string, exchange types.ExchangeMultiAccount) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.exchanges[name] = exchange
}

// AddRule adds a custom routing rule
func (r *Router) AddRule(rule RoutingRule) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.rules = append(r.rules, rule)
	
	// Sort by priority (higher priority first)
	sort.Slice(r.rules, func(i, j int) bool {
		return r.rules[i].Priority > r.rules[j].Priority
	})
}

// RouteOrder selects the best account for an order and routes it
func (r *Router) RouteOrder(ctx context.Context, exchange string, order *types.Order) (*types.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	// Get exchange
	exch, exists := r.exchanges[exchange]
	if !exists {
		return nil, fmt.Errorf("exchange %s not registered", exchange)
	}
	
	// Determine requirements
	req := r.getOrderRequirements(order)
	
	// Get candidate accounts
	filter := types.AccountFilter{
		Exchange: exchange,
		Active:   &[]bool{true}[0],
		Market:   req.Market,
	}
	
	accounts, err := r.manager.ListAccounts(filter)
	if err != nil {
		return nil, err
	}
	
	if len(accounts) == 0 {
		return nil, fmt.Errorf("no active accounts found for %s", exchange)
	}
	
	// Apply routing rules
	for _, rule := range r.rules {
		if rule.Condition(order) {
			if selected := rule.Selector(accounts); selected != nil {
				return r.selectAndPrepare(ctx, exch, selected, order)
			}
		}
	}
	
	// Apply default selection strategy
	selected := r.selectByStrategy(accounts, order)
	if selected == nil {
		return nil, fmt.Errorf("no suitable account found")
	}
	
	return r.selectAndPrepare(ctx, exch, selected, order)
}

// RouteOrderWithFallback routes order with fallback accounts
func (r *Router) RouteOrderWithFallback(ctx context.Context, exchange string, order *types.Order, maxAttempts int) (*types.Account, error) {
	attempted := make(map[string]bool)
	
	for i := 0; i < maxAttempts; i++ {
		account, err := r.RouteOrder(ctx, exchange, order)
		if err != nil {
			return nil, err
		}
		
		if attempted[account.ID] {
			continue
		}
		attempted[account.ID] = true
		
		// Try to use this account
		if err := r.validateAccountForOrder(account, order); err == nil {
			return account, nil
		}
		
		// Mark failed attempt
		r.recordFailure(account.ID)
		
		// Try next account
		time.Sleep(100 * time.Millisecond)
	}
	
	return nil, fmt.Errorf("failed to route order after %d attempts", maxAttempts)
}

// selectAndPrepare selects account and prepares exchange
func (r *Router) selectAndPrepare(ctx context.Context, exch types.ExchangeMultiAccount, account *types.Account, order *types.Order) (*types.Account, error) {
	// Set account on exchange
	if err := exch.SetAccount(account.ID); err != nil {
		return nil, fmt.Errorf("failed to set account: %w", err)
	}
	
	// Update metrics
	r.recordUsage(account.ID)
	
	// Update rate limit usage
	weight := r.estimateOrderWeight(order)
	r.manager.UpdateRateLimit(account.ID, weight)
	
	return account, nil
}

// selectByStrategy applies the configured selection strategy
func (r *Router) selectByStrategy(accounts []*types.Account, order *types.Order) *types.Account {
	switch r.config.Strategy {
	case StrategyLeastUsed:
		return r.selectLeastUsed(accounts)
		
	case StrategyRoundRobin:
		return r.selectRoundRobin(accounts)
		
	case StrategyBestFit:
		return r.selectBestFit(accounts, order)
		
	case StrategyLowestLatency:
		return r.selectLowestLatency(accounts)
		
	default:
		return accounts[0] // Fallback to first
	}
}

// selectLeastUsed selects account with lowest rate limit usage
func (r *Router) selectLeastUsed(accounts []*types.Account) *types.Account {
	var best *types.Account
	lowestUsage := float64(1.0)
	
	for _, account := range accounts {
		metrics, _ := r.manager.GetMetrics(account.ID)
		if metrics == nil {
			return account // No metrics = unused
		}
		
		usage := float64(metrics.UsedWeight) / float64(account.RateLimitWeight)
		if usage < lowestUsage {
			lowestUsage = usage
			best = account
		}
	}
	
	return best
}

// selectRoundRobin selects accounts in round-robin fashion
func (r *Router) selectRoundRobin(accounts []*types.Account) *types.Account {
	// Sort by last used time
	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].LastUsed.Before(accounts[j].LastUsed)
	})
	
	// Check cooldown
	if time.Since(accounts[0].LastUsed) < r.config.RotationCooldown {
		// All accounts in cooldown, use least recently used
		return accounts[0]
	}
	
	return accounts[0]
}

// selectBestFit selects the best fitting account for the order
func (r *Router) selectBestFit(accounts []*types.Account, order *types.Order) *types.Account {
	type scoredAccount struct {
		account *types.Account
		score   float64
	}
	
	scored := make([]scoredAccount, 0, len(accounts))
	
	for _, account := range accounts {
		score := r.calculateFitScore(account, order)
		scored = append(scored, scoredAccount{account, score})
	}
	
	// Sort by score (highest first)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})
	
	return scored[0].account
}

// selectLowestLatency selects account with best performance
func (r *Router) selectLowestLatency(accounts []*types.Account) *types.Account {
	var best *types.Account
	lowestLatency := time.Duration(1<<63 - 1) // Max duration
	
	for _, account := range accounts {
		if metrics, exists := r.metrics[account.ID]; exists {
			if metrics.AvgLatency < lowestLatency && metrics.ConsecutiveFails < 3 {
				lowestLatency = metrics.AvgLatency
				best = account
			}
		} else {
			// No metrics = potentially fastest
			return account
		}
	}
	
	if best == nil {
		best = accounts[0]
	}
	
	return best
}

// calculateFitScore calculates how well an account fits an order
func (r *Router) calculateFitScore(account *types.Account, order *types.Order) float64 {
	score := 100.0
	
	// Strategy match
	if orderStrategy, ok := order.Metadata["strategy"].(string); ok {
		if account.Strategy == orderStrategy {
			score += 50
		}
	}
	
	// Rate limit availability
	metrics, _ := r.manager.GetMetrics(account.ID)
	if metrics != nil {
		available := float64(account.RateLimitWeight - metrics.UsedWeight)
		score += available / float64(account.RateLimitWeight) * 30
	} else {
		score += 30 // Full availability
	}
	
	// Balance sufficiency
	orderValue := order.Quantity.Mul(order.Price)
	balance, _ := r.manager.GetBalance(account.ID)
	if balance != nil && !balance.TotalUSDT.IsZero() {
		if balance.TotalUSDT.GreaterThanOrEqual(orderValue) {
			score += 20
		} else {
			score -= 50 // Insufficient balance
		}
	}
	
	// Performance history
	if perfMetrics, exists := r.metrics[account.ID]; exists {
		score += perfMetrics.SuccessRate * 20
		score -= float64(perfMetrics.ConsecutiveFails) * 10
	}
	
	// Position limits
	if !account.MaxPositionUSDT.IsZero() && metrics != nil {
		usage := orderValue.Div(account.MaxPositionUSDT).InexactFloat64()
		if usage > 0.8 {
			score -= 30 // Near position limit
		}
	}
	
	return score
}

// getOrderRequirements determines requirements from order
func (r *Router) getOrderRequirements(order *types.Order) types.AccountRequirements {
	// Determine market type
	market := types.MarketTypeSpot
	if order.PositionSide != "" || order.ReduceOnly {
		market = types.MarketTypeFutures
	}
	
	// Calculate order value
	orderValue := order.Quantity
	if !order.Price.IsZero() {
		orderValue = order.Quantity.Mul(order.Price)
	}
	
	// Estimate weight (simplified)
	weight := 1
	if order.Type == types.OrderTypeMarket {
		weight = 1
	} else {
		weight = 2
	}
	
	return types.AccountRequirements{
		RequiredWeight: weight,
		Market:         market,
		Symbol:         order.Symbol,
		OrderSize:      orderValue,
	}
}

// validateAccountForOrder validates if account can handle order
func (r *Router) validateAccountForOrder(account *types.Account, order *types.Order) error {
	// Check if account is active
	if !account.Active {
		return fmt.Errorf("account %s is not active", account.ID)
	}
	
	// Check market support
	market := types.MarketTypeSpot
	if order.PositionSide != "" {
		market = types.MarketTypeFutures
	}
	
	if market == types.MarketTypeSpot && !account.SpotEnabled {
		return fmt.Errorf("account %s does not support spot trading", account.ID)
	}
	
	if market == types.MarketTypeFutures && !account.FuturesEnabled {
		return fmt.Errorf("account %s does not support futures trading", account.ID)
	}
	
	// Check balance
	orderValue := order.Quantity.Mul(order.Price)
	balance, _ := r.manager.GetBalance(account.ID)
	if balance != nil && balance.TotalUSDT.LessThan(orderValue) {
		return fmt.Errorf("insufficient balance in account %s", account.ID)
	}
	
	// Check position limits
	if !account.MaxPositionUSDT.IsZero() && orderValue.GreaterThan(account.MaxPositionUSDT) {
		return fmt.Errorf("order exceeds position limit for account %s", account.ID)
	}
	
	return nil
}

// estimateOrderWeight estimates API weight for an order
func (r *Router) estimateOrderWeight(order *types.Order) int {
	weight := 1 // Base weight
	
	// Add weight based on order type
	switch order.Type {
	case types.OrderTypeMarket:
		weight = 1
	case types.OrderTypeLimit:
		weight = 1
	case types.OrderTypeStop, types.OrderTypeStopLimit:
		weight = 2
	}
	
	// Add weight for special features
	if order.ReduceOnly {
		weight += 1
	}
	
	return weight
}

// recordUsage records account usage
func (r *Router) recordUsage(accountID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if _, exists := r.metrics[accountID]; !exists {
		r.metrics[accountID] = &RoutingMetrics{
			AccountID: accountID,
		}
	}
	
	r.metrics[accountID].OrdersRouted++
	r.metrics[accountID].LastUsed = time.Now()
}

// recordFailure records routing failure
func (r *Router) recordFailure(accountID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if metrics, exists := r.metrics[accountID]; exists {
		metrics.ConsecutiveFails++
		metrics.SuccessRate = float64(metrics.OrdersRouted-metrics.ConsecutiveFails) / float64(metrics.OrdersRouted)
	}
}

// recordSuccess records successful order
func (r *Router) recordSuccess(accountID string, latency time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if metrics, exists := r.metrics[accountID]; exists {
		metrics.ConsecutiveFails = 0
		
		// Update average latency
		total := metrics.AvgLatency * time.Duration(metrics.OrdersRouted-1)
		metrics.AvgLatency = (total + latency) / time.Duration(metrics.OrdersRouted)
		
		metrics.SuccessRate = float64(metrics.OrdersRouted-metrics.ConsecutiveFails) / float64(metrics.OrdersRouted)
	}
}

// addDefaultRules adds default routing rules
func (r *Router) addDefaultRules() {
	// Large orders go to main account
	r.AddRule(RoutingRule{
		Name:     "large_orders_to_main",
		Priority: 100,
		Condition: func(order *types.Order) bool {
			orderValue := order.Quantity.Mul(order.Price)
			return orderValue.GreaterThan(decimal.NewFromInt(50000))
		},
		Selector: func(accounts []*types.Account) *types.Account {
			for _, acc := range accounts {
				if acc.Type == types.AccountTypeMain {
					return acc
				}
			}
			return nil
		},
	})
	
	// Strategy-specific routing
	r.AddRule(RoutingRule{
		Name:     "strategy_match",
		Priority: 90,
		Condition: func(order *types.Order) bool {
			_, hasStrategy := order.Metadata["strategy"]
			return hasStrategy
		},
		Selector: func(accounts []*types.Account) *types.Account {
			strategy, _ := accounts[0].Metadata["strategy"].(string)
			for _, acc := range accounts {
				if acc.Strategy == strategy {
					return acc
				}
			}
			return nil
		},
	})
	
	// Futures orders to futures-enabled accounts
	r.AddRule(RoutingRule{
		Name:     "futures_routing",
		Priority: 80,
		Condition: func(order *types.Order) bool {
			return order.PositionSide != ""
		},
		Selector: func(accounts []*types.Account) *types.Account {
			for _, acc := range accounts {
				if acc.FuturesEnabled {
					return acc
				}
			}
			return nil
		},
	})
}

// cleanupMetricsLoop periodically cleans up old metrics
func (r *Router) cleanupMetricsLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	
	for range ticker.C {
		r.mu.Lock()
		cutoff := time.Now().Add(-r.config.MetricsRetention)
		
		for id, metrics := range r.metrics {
			if metrics.LastUsed.Before(cutoff) {
				delete(r.metrics, id)
			}
		}
		r.mu.Unlock()
	}
}

// GetRoutingStats returns routing statistics
func (r *Router) GetRoutingStats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	stats := make(map[string]interface{})
	accountStats := make(map[string]interface{})
	
	totalOrders := 0
	totalFailures := 0
	
	for accountID, metrics := range r.metrics {
		totalOrders += metrics.OrdersRouted
		totalFailures += metrics.ConsecutiveFails
		
		accountStats[accountID] = map[string]interface{}{
			"orders_routed":    metrics.OrdersRouted,
			"success_rate":     metrics.SuccessRate,
			"avg_latency_ms":   metrics.AvgLatency.Milliseconds(),
			"last_used":        metrics.LastUsed,
			"consecutive_fails": metrics.ConsecutiveFails,
		}
	}
	
	stats["total_orders"] = totalOrders
	stats["total_failures"] = totalFailures
	stats["accounts"] = accountStats
	stats["active_rules"] = len(r.rules)
	
	return stats
}