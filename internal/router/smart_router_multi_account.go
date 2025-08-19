package router

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
	
	"github.com/mExOms/internal/account"
	"github.com/mExOms/internal/exchange"
	"github.com/mExOms/pkg/cache"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// SmartRouterMultiAccount implements intelligent order routing with multi-account support
type SmartRouterMultiAccount struct {
	exchanges       map[string]types.ExchangeMultiAccount
	accountRouter   *account.Router
	accountManager  types.AccountManager
	exchangeCache   *cache.MemoryCache
	balanceCache    *cache.MemoryCache
	priceCache      *cache.MemoryCache
	factory         *exchange.Factory
	config          *SmartRouterConfig
	mu              sync.RWMutex
}

// SmartRouterConfig contains router configuration
type SmartRouterConfig struct {
	// Multi-account settings
	EnableAccountRouting bool
	AccountSelectionMode string // "best_fit", "round_robin", "least_used"
	
	// Order routing settings
	EnableSplitOrders    bool
	MaxOrderSplits       int
	MinOrderSizeUSDT     decimal.Decimal
	
	// Arbitrage settings
	EnableArbitrage      bool
	MinArbitrageProfitPct decimal.Decimal
	
	// Performance settings
	CacheTTL             time.Duration
	MaxConcurrentOrders  int
}

// NewSmartRouterMultiAccount creates a new multi-account smart router
func NewSmartRouterMultiAccount(factory *exchange.Factory, accountManager types.AccountManager, config *SmartRouterConfig) *SmartRouterMultiAccount {
	if config == nil {
		config = &SmartRouterConfig{
			EnableAccountRouting:  true,
			AccountSelectionMode:  "best_fit",
			EnableSplitOrders:     true,
			MaxOrderSplits:        5,
			MinOrderSizeUSDT:      decimal.NewFromInt(10),
			EnableArbitrage:       true,
			MinArbitrageProfitPct: decimal.NewFromFloat(0.1),
			CacheTTL:              5 * time.Second,
			MaxConcurrentOrders:   10,
		}
	}
	
	// Create account router
	routerConfig := &account.RouterConfig{
		Strategy:         account.SelectionStrategy(config.AccountSelectionMode),
		RateLimitBuffer:  200,
		RotationCooldown: 5 * time.Minute,
	}
	
	return &SmartRouterMultiAccount{
		exchanges:      make(map[string]types.ExchangeMultiAccount),
		accountRouter:  account.NewRouter(accountManager, routerConfig),
		accountManager: accountManager,
		exchangeCache:  cache.NewMemoryCache(),
		balanceCache:   cache.NewMemoryCache(),
		priceCache:     cache.NewMemoryCache(),
		factory:        factory,
		config:         config,
	}
}

// AddExchange adds a multi-account exchange to the router
func (sr *SmartRouterMultiAccount) AddExchange(name string, exchange types.ExchangeMultiAccount) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	
	sr.exchanges[name] = exchange
	sr.accountRouter.RegisterExchange(name, exchange)
	
	return nil
}

// RouteOrder routes an order to the best exchange and account
func (sr *SmartRouterMultiAccount) RouteOrder(ctx context.Context, order *types.Order) (*types.Order, error) {
	// Find best exchange based on price
	bestExchange, exchangeName, err := sr.findBestExchange(ctx, order)
	if err != nil {
		return nil, fmt.Errorf("failed to find best exchange: %w", err)
	}
	
	// Select best account for the order
	selectedAccount, err := sr.accountRouter.RouteOrder(ctx, exchangeName, order)
	if err != nil {
		return nil, fmt.Errorf("failed to select account: %w", err)
	}
	
	// Set account on exchange
	if err := bestExchange.SetAccount(selectedAccount.ID); err != nil {
		return nil, fmt.Errorf("failed to set account: %w", err)
	}
	
	// Check balance on selected account
	if err := sr.checkAccountBalance(ctx, bestExchange, selectedAccount, order); err != nil {
		// Try fallback accounts
		fallbackAccount, err := sr.accountRouter.RouteOrderWithFallback(ctx, exchangeName, order, 3)
		if err != nil {
			return nil, fmt.Errorf("insufficient balance across all accounts: %w", err)
		}
		selectedAccount = fallbackAccount
		
		// Set fallback account
		if err := bestExchange.SetAccount(selectedAccount.ID); err != nil {
			return nil, fmt.Errorf("failed to set fallback account: %w", err)
		}
	}
	
	// Update order metadata
	order.Metadata = map[string]interface{}{
		"exchange":   exchangeName,
		"account_id": selectedAccount.ID,
		"strategy":   selectedAccount.Strategy,
		"routed_at":  time.Now(),
	}
	
	// Route order to selected exchange and account
	executedOrder, err := bestExchange.CreateOrder(ctx, order)
	if err != nil {
		// Record failure for account metrics
		sr.accountRouter.RecordFailure(selectedAccount.ID)
		return nil, fmt.Errorf("failed to create order: %w", err)
	}
	
	// Record success
	sr.accountRouter.RecordSuccess(selectedAccount.ID, time.Since(order.CreatedAt))
	
	return executedOrder, nil
}

// SplitOrderAcrossAccounts splits a large order across multiple accounts
func (sr *SmartRouterMultiAccount) SplitOrderAcrossAccounts(ctx context.Context, order *types.Order) ([]*types.Order, error) {
	// Get all available accounts for the best exchange
	bestExchange, exchangeName, err := sr.findBestExchange(ctx, order)
	if err != nil {
		return nil, fmt.Errorf("failed to find best exchange: %w", err)
	}
	
	// Get accounts for this exchange
	filter := types.AccountFilter{
		Exchange: exchangeName,
		Active:   &[]bool{true}[0],
		Market:   sr.getMarketType(order),
	}
	
	accounts, err := sr.accountManager.ListAccounts(filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}
	
	// Calculate order splits based on account capacities
	splits, err := sr.calculateOptimalSplits(ctx, bestExchange, accounts, order)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate splits: %w", err)
	}
	
	// Execute orders on each account
	var executedOrders []*types.Order
	var mu sync.Mutex
	var wg sync.WaitGroup
	
	for _, split := range splits {
		wg.Add(1)
		go func(s orderSplit) {
			defer wg.Done()
			
			// Set account
			if err := bestExchange.SetAccount(s.accountID); err != nil {
				return
			}
			
			// Create order
			splitOrder := *order
			splitOrder.Quantity = s.quantity
			splitOrder.Metadata = map[string]interface{}{
				"exchange":     exchangeName,
				"account_id":   s.accountID,
				"parent_order": order.ClientOrderID,
				"split_index":  s.index,
			}
			
			executed, err := bestExchange.CreateOrder(ctx, &splitOrder)
			if err == nil {
				mu.Lock()
				executedOrders = append(executedOrders, executed)
				mu.Unlock()
			}
		}(split)
	}
	
	wg.Wait()
	
	if len(executedOrders) == 0 {
		return nil, fmt.Errorf("failed to execute any split orders")
	}
	
	return executedOrders, nil
}

// RouteArbitrageOrder routes arbitrage orders across exchanges and accounts
func (sr *SmartRouterMultiAccount) RouteArbitrageOrder(ctx context.Context, opportunity ArbitrageOpportunity) (*ArbitrageResult, error) {
	// Select accounts for buy and sell sides
	buyOrder := &types.Order{
		Symbol:   opportunity.Symbol,
		Side:     types.OrderSideBuy,
		Type:     types.OrderTypeLimit,
		Price:    opportunity.BuyPrice,
		Quantity: opportunity.MaxQuantity,
		Metadata: map[string]interface{}{
			"strategy": "arbitrage",
			"pair_id":  opportunity.ID,
		},
	}
	
	sellOrder := &types.Order{
		Symbol:   opportunity.Symbol,
		Side:     types.OrderSideSell,
		Type:     types.OrderTypeLimit,
		Price:    opportunity.SellPrice,
		Quantity: opportunity.MaxQuantity,
		Metadata: map[string]interface{}{
			"strategy": "arbitrage",
			"pair_id":  opportunity.ID,
		},
	}
	
	// Route buy order
	buyAccount, err := sr.accountRouter.RouteOrder(ctx, opportunity.BuyExchange, buyOrder)
	if err != nil {
		return nil, fmt.Errorf("failed to route buy order: %w", err)
	}
	
	// Route sell order (prefer different account for better rate limits)
	sellAccount, err := sr.accountRouter.RouteOrder(ctx, opportunity.SellExchange, sellOrder)
	if err != nil {
		return nil, fmt.Errorf("failed to route sell order: %w", err)
	}
	
	// Execute orders in parallel
	var buyResult, sellResult *types.Order
	var buyErr, sellErr error
	var wg sync.WaitGroup
	
	wg.Add(2)
	
	// Execute buy order
	go func() {
		defer wg.Done()
		buyExch := sr.exchanges[opportunity.BuyExchange]
		if err := buyExch.SetAccount(buyAccount.ID); err != nil {
			buyErr = err
			return
		}
		buyResult, buyErr = buyExch.CreateOrder(ctx, buyOrder)
	}()
	
	// Execute sell order
	go func() {
		defer wg.Done()
		sellExch := sr.exchanges[opportunity.SellExchange]
		if err := sellExch.SetAccount(sellAccount.ID); err != nil {
			sellErr = err
			return
		}
		sellResult, sellErr = sellExch.CreateOrder(ctx, sellOrder)
	}()
	
	wg.Wait()
	
	// Handle results
	result := &ArbitrageResult{
		Opportunity: opportunity,
		BuyOrder:    buyResult,
		SellOrder:   sellResult,
		BuyError:    buyErr,
		SellError:   sellErr,
	}
	
	// If one side failed, try to cancel the other
	if buyErr != nil && sellResult != nil {
		sellExch := sr.exchanges[opportunity.SellExchange]
		sellExch.CancelOrder(ctx, sellResult.ExchangeOrderID)
	} else if sellErr != nil && buyResult != nil {
		buyExch := sr.exchanges[opportunity.BuyExchange]
		buyExch.CancelOrder(ctx, buyResult.ExchangeOrderID)
	}
	
	return result, nil
}

// findBestExchange finds the best exchange and returns it with its name
func (sr *SmartRouterMultiAccount) findBestExchange(ctx context.Context, order *types.Order) (types.ExchangeMultiAccount, string, error) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	
	type exchangePrice struct {
		exchange types.ExchangeMultiAccount
		name     string
		price    decimal.Decimal
		volume   decimal.Decimal
	}
	
	var candidates []exchangePrice
	
	// Get prices from all exchanges
	for name, exch := range sr.exchanges {
		// Check if exchange is connected
		if !exch.IsConnected() {
			continue
		}
		
		// Get ticker from cache or fetch
		cacheKey := fmt.Sprintf("ticker:%s:%s", name, order.Symbol)
		ticker, found := sr.priceCache.Get(cacheKey)
		if !found {
			continue
		}
		
		tickerData, ok := ticker.(*types.Ticker)
		if !ok {
			continue
		}
		
		// Get relevant price based on order side
		var price, volume decimal.Decimal
		if order.Side == types.OrderSideBuy {
			price, _ = decimal.NewFromString(tickerData.AskPrice)
			volume, _ = decimal.NewFromString(tickerData.AskQty)
		} else {
			price, _ = decimal.NewFromString(tickerData.BidPrice)
			volume, _ = decimal.NewFromString(tickerData.BidQty)
		}
		
		// Skip if price is zero
		if price.IsZero() {
			continue
		}
		
		candidates = append(candidates, exchangePrice{
			exchange: exch,
			name:     name,
			price:    price,
			volume:   volume,
		})
	}
	
	if len(candidates) == 0 {
		return nil, "", fmt.Errorf("no available exchanges for %s", order.Symbol)
	}
	
	// Sort by best price
	sort.Slice(candidates, func(i, j int) bool {
		if order.Side == types.OrderSideBuy {
			return candidates[i].price.LessThan(candidates[j].price)
		} else {
			return candidates[i].price.GreaterThan(candidates[j].price)
		}
	})
	
	return candidates[0].exchange, candidates[0].name, nil
}

// checkAccountBalance checks if account has sufficient balance
func (sr *SmartRouterMultiAccount) checkAccountBalance(ctx context.Context, exch types.ExchangeMultiAccount, account *types.Account, order *types.Order) error {
	// Get balance from cache or fetch
	cacheKey := fmt.Sprintf("balance:%s:%s", exch.GetName(), account.ID)
	balance, found := sr.balanceCache.Get(cacheKey)
	
	if !found {
		// Fetch balance for specific account
		bal, err := exch.GetBalanceForAccount(ctx, account.ID, "")
		if err != nil {
			return fmt.Errorf("failed to get balance: %w", err)
		}
		balance = bal
		sr.balanceCache.Set(cacheKey, bal, sr.config.CacheTTL)
	}
	
	balData, ok := balance.(*types.Balance)
	if !ok {
		return fmt.Errorf("invalid balance data")
	}
	
	// Calculate required amount
	var requiredAmount decimal.Decimal
	if order.Side == types.OrderSideBuy {
		requiredAmount = order.Quantity.Mul(order.Price)
	} else {
		requiredAmount = order.Quantity
	}
	
	// Check if balance is sufficient
	if balData.Free.LessThan(requiredAmount) {
		return fmt.Errorf("insufficient balance: need %s, have %s", 
			requiredAmount.String(), balData.Free.String())
	}
	
	return nil
}

// calculateOptimalSplits calculates optimal order splits across accounts
func (sr *SmartRouterMultiAccount) calculateOptimalSplits(ctx context.Context, exch types.ExchangeMultiAccount, accounts []*types.Account, order *types.Order) ([]orderSplit, error) {
	var splits []orderSplit
	remainingQty := order.Quantity
	
	// Get balances and capacities for each account
	type accountCapacity struct {
		account   *types.Account
		available decimal.Decimal
		rateLimit int
	}
	
	var capacities []accountCapacity
	
	for _, account := range accounts {
		// Skip if wrong market type
		if order.PositionSide != "" && !account.FuturesEnabled {
			continue
		}
		if order.PositionSide == "" && !account.SpotEnabled {
			continue
		}
		
		// Get available balance
		balance, err := sr.accountManager.GetBalance(account.ID)
		if err != nil {
			continue
		}
		
		// Get available rate limit
		metrics, _ := sr.accountManager.GetMetrics(account.ID)
		availableWeight := account.RateLimitWeight
		if metrics != nil {
			availableWeight = account.RateLimitWeight - metrics.UsedWeight
		}
		
		// Calculate available capacity
		var available decimal.Decimal
		if order.Side == types.OrderSideBuy {
			available = balance.TotalUSDT.Div(order.Price)
		} else {
			// For sell orders, check specific asset balance
			// This is simplified - need proper asset parsing
			available = balance.TotalUSDT // Placeholder
		}
		
		// Apply account limits
		if !account.MaxPositionUSDT.IsZero() {
			maxQty := account.MaxPositionUSDT.Div(order.Price)
			if available.GreaterThan(maxQty) {
				available = maxQty
			}
		}
		
		capacities = append(capacities, accountCapacity{
			account:   account,
			available: available,
			rateLimit: availableWeight,
		})
	}
	
	// Sort by available capacity
	sort.Slice(capacities, func(i, j int) bool {
		return capacities[i].available.GreaterThan(capacities[j].available)
	})
	
	// Allocate orders to accounts
	for i, cap := range capacities {
		if remainingQty.IsZero() {
			break
		}
		
		// Calculate allocation for this account
		allocation := remainingQty
		if allocation.GreaterThan(cap.available) {
			allocation = cap.available
		}
		
		// Check minimum order size
		orderValue := allocation.Mul(order.Price)
		if orderValue.LessThan(sr.config.MinOrderSizeUSDT) {
			continue
		}
		
		splits = append(splits, orderSplit{
			accountID: cap.account.ID,
			quantity:  allocation,
			index:     i,
		})
		
		remainingQty = remainingQty.Sub(allocation)
	}
	
	if remainingQty.GreaterThan(decimal.Zero) {
		return splits, fmt.Errorf("cannot allocate full order quantity, remaining: %s", remainingQty.String())
	}
	
	return splits, nil
}

// getMarketType determines market type from order
func (sr *SmartRouterMultiAccount) getMarketType(order *types.Order) types.MarketType {
	if order.PositionSide != "" || order.ReduceOnly {
		return types.MarketTypeFutures
	}
	return types.MarketTypeSpot
}

// UpdateMarketData updates cached market data
func (sr *SmartRouterMultiAccount) UpdateMarketData(exchange string, symbol string, ticker *types.Ticker) {
	cacheKey := fmt.Sprintf("ticker:%s:%s", exchange, symbol)
	sr.priceCache.Set(cacheKey, ticker, sr.config.CacheTTL)
}

// UpdateAccountBalance updates cached account balance
func (sr *SmartRouterMultiAccount) UpdateAccountBalance(exchange, accountID string, balance *types.Balance) {
	cacheKey := fmt.Sprintf("balance:%s:%s", exchange, accountID)
	sr.balanceCache.Set(cacheKey, balance, sr.config.CacheTTL)
}

// GetRoutingStats returns routing statistics
func (sr *SmartRouterMultiAccount) GetRoutingStats() map[string]interface{} {
	stats := sr.accountRouter.GetRoutingStats()
	
	// Add exchange stats
	exchangeStats := make(map[string]interface{})
	sr.mu.RLock()
	for name, exch := range sr.exchanges {
		exchangeStats[name] = map[string]interface{}{
			"connected":      exch.IsConnected(),
			"current_account": exch.GetCurrentAccount(),
		}
	}
	sr.mu.RUnlock()
	
	stats["exchanges"] = exchangeStats
	stats["config"] = map[string]interface{}{
		"account_routing_enabled": sr.config.EnableAccountRouting,
		"split_orders_enabled":    sr.config.EnableSplitOrders,
		"arbitrage_enabled":       sr.config.EnableArbitrage,
	}
	
	return stats
}

// Helper types

type orderSplit struct {
	accountID string
	quantity  decimal.Decimal
	index     int
}

// ArbitrageResult represents the result of an arbitrage trade
type ArbitrageResult struct {
	Opportunity ArbitrageOpportunity
	BuyOrder    *types.Order
	SellOrder   *types.Order
	BuyError    error
	SellError   error
	ExecutedAt  time.Time
}