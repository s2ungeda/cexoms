package router

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/mExOms/internal/account"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// MultiAccountRouter implements intelligent order routing across multiple exchanges and accounts
type MultiAccountRouter struct {
	mu              sync.RWMutex
	exchanges       map[string]types.Exchange
	accountManager  *account.Manager
	priceAggregator *PriceAggregator
	routingEngine   *RoutingEngine
	arbitrageEngine *ArbitrageEngine
	feeCalculator   *FeeCalculator
	config          *RouterConfig
	metrics         *RoutingMetrics
}

// RouterConfig holds smart router configuration
type RouterConfig struct {
	MaxSplitOrders       int                    // Maximum number of split orders
	MinOrderSize         decimal.Decimal        // Minimum order size
	SlippageTolerance    decimal.Decimal        // Maximum acceptable slippage
	ArbitrageThreshold   decimal.Decimal        // Minimum profit for arbitrage
	PriceUpdateInterval  time.Duration          // How often to update prices
	RoutingStrategy      RoutingStrategy        // Routing strategy
	MaxExecutionTime     time.Duration          // Maximum time for order execution
	AccountSelection     AccountSelection       // Account selection criteria
	EnableArbitrage      bool                   // Enable arbitrage detection
	FeeStructures        map[string]FeeStructure // Fee structures by exchange
}

// NewMultiAccountRouter creates a new multi-account smart order router
func NewMultiAccountRouter(config *RouterConfig, accountManager *account.Manager) *MultiAccountRouter {
	mr := &MultiAccountRouter{
		exchanges:       make(map[string]types.Exchange),
		accountManager:  accountManager,
		priceAggregator: NewPriceAggregator(),
		routingEngine:   NewRoutingEngine(config),
		arbitrageEngine: NewArbitrageEngine(config.ArbitrageThreshold),
		feeCalculator:   NewFeeCalculator(config.FeeStructures),
		config:          config,
		metrics:         &RoutingMetrics{UpdatedAt: time.Now()},
	}

	// Start background tasks
	go mr.priceUpdateLoop()
	if config.EnableArbitrage {
		go mr.arbitrageDetectionLoop()
	}

	return mr
}

// RegisterExchange registers an exchange with the router
func (mr *MultiAccountRouter) RegisterExchange(name string, exchange types.Exchange) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.exchanges[name] = exchange
}

// RouteOrder routes an order to the best exchange and account
func (mr *MultiAccountRouter) RouteOrder(ctx context.Context, req *RoutingRequest) (*RoutingResult, error) {
	// Validate request
	if err := mr.validateRequest(req); err != nil {
		return nil, fmt.Errorf("invalid routing request: %w", err)
	}

	// Get market data across all exchanges
	marketData, err := mr.getAggregatedMarketData(req.Symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get market data: %w", err)
	}

	// Find eligible accounts based on criteria
	eligibleAccounts, err := mr.findEligibleAccounts(req)
	if err != nil {
		return nil, fmt.Errorf("failed to find eligible accounts: %w", err)
	}

	if len(eligibleAccounts) == 0 {
		return nil, fmt.Errorf("no eligible accounts found")
	}

	// Build routes with account and exchange combinations
	routes := mr.buildRoutes(req, marketData, eligibleAccounts)
	if len(routes) == 0 {
		return nil, fmt.Errorf("no valid routes found")
	}

	// Route based on strategy
	var result *RoutingResult
	switch mr.config.RoutingStrategy {
	case RoutingStrategyArbitrage:
		result, err = mr.routeArbitrageOrder(ctx, req, routes)
	case RoutingStrategyBestPrice:
		result, err = mr.routeBestPriceOrder(ctx, req, routes)
	case RoutingStrategyBestLiquidity:
		result, err = mr.routeBestLiquidityOrder(ctx, req, routes)
	case RoutingStrategyLowestFee:
		result, err = mr.routeLowestFeeOrder(ctx, req, routes)
	case RoutingStrategyFastest:
		result, err = mr.routeFastestOrder(ctx, req, routes)
	default:
		result, err = mr.routeBalancedOrder(ctx, req, routes)
	}

	if err != nil {
		return nil, err
	}

	// Update routing metrics
	mr.updateRoutingMetrics(result)

	return result, nil
}

// findEligibleAccounts finds accounts that can execute the order
func (mr *MultiAccountRouter) findEligibleAccounts(req *RoutingRequest) ([]*AccountRoute, error) {
	var eligibleAccounts []*AccountRoute

	// Get all active accounts
	filter := types.AccountFilter{
		Active: &[]bool{true}[0],
	}
	
	// Apply strategy filter if specified
	if req.Strategy != "" {
		filter.Strategy = req.Strategy
	}

	accounts, err := mr.accountManager.ListAccounts(filter)
	if err != nil {
		return nil, err
	}

	// Apply account selection criteria
	for _, acc := range accounts {
		// Check if account is in preferred list
		if len(mr.config.AccountSelection.PreferredAccounts) > 0 {
			found := false
			for _, preferred := range mr.config.AccountSelection.PreferredAccounts {
				if acc.ID == preferred {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Check if account is excluded
		excluded := false
		for _, excludedID := range mr.config.AccountSelection.ExcludedAccounts {
			if acc.ID == excludedID {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		// Check market support
		if req.Side == types.OrderSideBuy || req.Side == types.OrderSideSell {
			if !acc.SpotEnabled {
				continue
			}
		}

		// Check permissions
		if mr.config.AccountSelection.RequirePermission != "" {
			if !mr.accountManager.ValidateAccountOperation(acc.ID, mr.config.AccountSelection.RequirePermission) {
				continue
			}
		}

		// Get account balance
		balance, err := mr.accountManager.GetBalance(acc.ID)
		if err != nil {
			continue
		}

		// Check minimum balance requirement
		if !mr.config.AccountSelection.MinBalance.IsZero() {
			if balance.TotalUSDT.LessThan(mr.config.AccountSelection.MinBalance) {
				continue
			}
		}

		// Check balance for buy orders
		if req.Side == types.OrderSideBuy {
			requiredBalance := req.Quantity.Mul(req.Price)
			if balance.TotalUSDT.LessThan(requiredBalance) {
				continue
			}
		}

		// Check rate limits
		requirements := types.AccountRequirements{
			Market:         types.MarketTypeSpot,
			RequiredWeight: 10, // Estimated weight for order
			OrderSize:      req.Quantity.Mul(req.Price),
		}

		selectedAccount, err := mr.accountManager.SelectAccount(req.Strategy, requirements)
		if err != nil || selectedAccount.ID != acc.ID {
			continue
		}

		// Add to eligible list
		accountRoute := &AccountRoute{
			AccountID:   acc.ID,
			Exchange:    acc.Exchange,
			Available:   true,
			RateLimit:   100 - requirements.RequiredWeight, // Available rate limit
			Balance:     balance.TotalUSDT,
			Permissions: []string{"trade"}, // Get actual permissions
		}

		eligibleAccounts = append(eligibleAccounts, accountRoute)
	}

	return eligibleAccounts, nil
}

// buildRoutes builds all possible routes combining exchanges and accounts
func (mr *MultiAccountRouter) buildRoutes(req *RoutingRequest, marketData map[string]*MarketData, accounts []*AccountRoute) []*Route {
	var routes []*Route

	for _, account := range accounts {
		md, exists := marketData[account.Exchange]
		if !exists {
			continue
		}

		var price decimal.Decimal
		var liquidity decimal.Decimal

		if req.Side == types.OrderSideBuy {
			price = md.AskPrice
			liquidity = md.AskVolume
		} else {
			price = md.BidPrice
			liquidity = md.BidVolume
		}

		// Skip if no liquidity
		if liquidity.IsZero() {
			continue
		}

		// Calculate estimated fees
		fees := mr.feeCalculator.CalculateFees(account.Exchange, req.Quantity, price, true) // Assume taker

		// Calculate route score
		score := mr.calculateRouteScore(price, liquidity, fees, account, req)

		route := &Route{
			Exchange:           account.Exchange,
			AccountID:          account.AccountID,
			Price:              price,
			AvailableLiquidity: liquidity,
			EstimatedFees:      fees,
			RateLimitWeight:    10, // Estimated
			Score:              score,
			Priority:           0,
		}

		routes = append(routes, route)
	}

	// Sort by score (highest first)
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Score.GreaterThan(routes[j].Score)
	})

	// Assign priorities
	for i := range routes {
		routes[i].Priority = i + 1
	}

	return routes
}

// calculateRouteScore calculates a composite score for route selection
func (mr *MultiAccountRouter) calculateRouteScore(price, liquidity, fees decimal.Decimal, account *AccountRoute, req *RoutingRequest) decimal.Decimal {
	score := decimal.NewFromInt(100)

	// Price factor (40% weight)
	// Better price gets higher score
	priceScore := decimal.NewFromInt(40)
	if req.Side == types.OrderSideBuy {
		// Lower ask price is better
		// Normalize against average price
		priceScore = priceScore.Div(price.Div(decimal.NewFromFloat(1000))) // Simplified
	} else {
		// Higher bid price is better
		priceScore = priceScore.Mul(price.Div(decimal.NewFromFloat(1000))) // Simplified
	}

	// Liquidity factor (30% weight)
	liquidityScore := decimal.NewFromInt(30)
	if liquidity.GreaterThanOrEqual(req.Quantity) {
		// Full liquidity available
		liquidityScore = decimal.NewFromInt(30)
	} else {
		// Partial liquidity
		liquidityScore = liquidityScore.Mul(liquidity.Div(req.Quantity))
	}

	// Fee factor (20% weight)
	feeScore := decimal.NewFromInt(20).Sub(fees.Mul(decimal.NewFromInt(100)))
	if feeScore.IsNegative() {
		feeScore = decimal.Zero
	}

	// Rate limit factor (10% weight)
	rateLimitScore := decimal.NewFromInt(10).Mul(decimal.NewFromInt(int64(account.RateLimit)).Div(decimal.NewFromInt(100)))

	score = priceScore.Add(liquidityScore).Add(feeScore).Add(rateLimitScore)
	return score
}

// routeBestPriceOrder routes to exchange/account with best price
func (mr *MultiAccountRouter) routeBestPriceOrder(ctx context.Context, req *RoutingRequest, routes []*Route) (*RoutingResult, error) {
	// Sort by best price
	sort.Slice(routes, func(i, j int) bool {
		if req.Side == types.OrderSideBuy {
			return routes[i].Price.LessThan(routes[j].Price)
		}
		return routes[i].Price.GreaterThan(routes[j].Price)
	})

	// Check if order needs to be split
	if req.Quantity.GreaterThan(routes[0].AvailableLiquidity) {
		return mr.splitOrder(ctx, req, routes)
	}

	// Execute on single best route
	return mr.executeSingleRoute(ctx, req, routes[0])
}

// splitOrder splits a large order across multiple routes
func (mr *MultiAccountRouter) splitOrder(ctx context.Context, req *RoutingRequest, routes []*Route) (*RoutingResult, error) {
	result := &RoutingResult{
		RequestID:     req.ID,
		Symbol:        req.Symbol,
		TotalQuantity: req.Quantity,
		Routes:        make([]*ExecutedRoute, 0),
		StartTime:     time.Now(),
	}

	remainingQty := req.Quantity
	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := make([]error, 0)

	// Execute orders in parallel with limits
	sem := make(chan struct{}, 3) // Max 3 concurrent orders

	for i, route := range routes {
		if remainingQty.IsZero() || i >= mr.config.MaxSplitOrders {
			break
		}

		// Calculate order size for this route
		orderQty := decimal.Min(remainingQty, route.AvailableLiquidity)
		if orderQty.LessThan(mr.config.MinOrderSize) {
			continue
		}

		wg.Add(1)
		go func(r *Route, qty decimal.Decimal) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Create sub-order
			subReq := &RoutingRequest{
				ID:        fmt.Sprintf("%s_%d", req.ID, r.Priority),
				Symbol:    req.Symbol,
				Side:      req.Side,
				Quantity:  qty,
				OrderType: req.OrderType,
				Price:     r.Price,
				AccountID: r.AccountID,
			}

			// Execute on this route
			execRoute, err := mr.executeOnRoute(ctx, subReq, r)
			
			mu.Lock()
			if err != nil {
				errors = append(errors, err)
			} else {
				result.Routes = append(result.Routes, execRoute)
				result.ExecutedQuantity = result.ExecutedQuantity.Add(execRoute.ExecutedQuantity)
				result.TotalFees = result.TotalFees.Add(execRoute.Fees)
			}
			remainingQty = remainingQty.Sub(qty)
			mu.Unlock()
		}(route, orderQty)
	}

	wg.Wait()

	result.EndTime = time.Now()
	result.Success = len(errors) == 0 && result.ExecutedQuantity.Equal(req.Quantity)

	if len(errors) > 0 {
		result.Errors = errors
	}

	// Calculate average price
	if result.ExecutedQuantity.IsPositive() {
		totalValue := decimal.Zero
		for _, route := range result.Routes {
			totalValue = totalValue.Add(route.Price.Mul(route.ExecutedQuantity))
		}
		result.AveragePrice = totalValue.Div(result.ExecutedQuantity)
	}

	return result, nil
}

// executeOnRoute executes an order on a specific route
func (mr *MultiAccountRouter) executeOnRoute(ctx context.Context, req *RoutingRequest, route *Route) (*ExecutedRoute, error) {
	// Get exchange
	exchange, exists := mr.exchanges[route.Exchange]
	if !exists {
		return nil, fmt.Errorf("exchange %s not found", route.Exchange)
	}

	// Get API keys for account
	keys, err := mr.accountManager.GetAPIKeys(route.AccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get API keys: %w", err)
	}

	// Set API keys on exchange (this would be exchange-specific)
	// exchange.SetAPIKeys(keys.APIKey, keys.SecretKey)

	// Create order
	order := &types.Order{
		ID:          req.ID,
		AccountID:   route.AccountID,
		Exchange:    route.Exchange,
		Symbol:      req.Symbol,
		Side:        req.Side,
		Type:        req.OrderType,
		Quantity:    req.Quantity,
		Price:       req.Price,
		TimeInForce: types.TimeInForceGTC,
		CreatedAt:   time.Now(),
	}

	// Place order
	placedOrder, err := exchange.PlaceOrder(ctx, order)
	if err != nil {
		return nil, fmt.Errorf("failed to place order: %w", err)
	}

	// Update account metrics
	mr.accountManager.UpdateRateLimit(route.AccountID, route.RateLimitWeight)

	// Build executed route
	executed := &ExecutedRoute{
		Exchange:         route.Exchange,
		AccountID:        route.AccountID,
		OrderID:          placedOrder.ID,
		Price:            placedOrder.Price,
		ExecutedQuantity: placedOrder.ExecutedQuantity,
		Fees:             placedOrder.Fees,
		Status:           placedOrder.Status,
		ExecutedAt:       time.Now(),
	}

	return executed, nil
}

// executeSingleRoute executes an order on a single route
func (mr *MultiAccountRouter) executeSingleRoute(ctx context.Context, req *RoutingRequest, route *Route) (*RoutingResult, error) {
	result := &RoutingResult{
		RequestID:     req.ID,
		Symbol:        req.Symbol,
		TotalQuantity: req.Quantity,
		Routes:        make([]*ExecutedRoute, 0),
		StartTime:     time.Now(),
	}

	executed, err := mr.executeOnRoute(ctx, req, route)
	if err != nil {
		result.Success = false
		result.Errors = []error{err}
		return result, err
	}

	result.Routes = append(result.Routes, executed)
	result.ExecutedQuantity = executed.ExecutedQuantity
	result.AveragePrice = executed.Price
	result.TotalFees = executed.Fees
	result.EndTime = time.Now()
	result.Success = true

	return result, nil
}

// routeArbitrageOrder finds and executes arbitrage opportunities
func (mr *MultiAccountRouter) routeArbitrageOrder(ctx context.Context, req *RoutingRequest, routes []*Route) (*RoutingResult, error) {
	// Find arbitrage opportunities
	opportunities := mr.findArbitrageOpportunities(req.Symbol, routes)
	if len(opportunities) == 0 {
		// Fall back to best price routing
		return mr.routeBestPriceOrder(ctx, req, routes)
	}

	// Execute arbitrage
	best := opportunities[0]
	return mr.executeArbitrage(ctx, best, req)
}

// findArbitrageOpportunities finds arbitrage opportunities across routes
func (mr *MultiAccountRouter) findArbitrageOpportunities(symbol string, routes []*Route) []*ArbitrageOpportunity {
	var opportunities []*ArbitrageOpportunity

	// Group routes by exchange
	exchangeRoutes := make(map[string][]*Route)
	for _, route := range routes {
		exchangeRoutes[route.Exchange] = append(exchangeRoutes[route.Exchange], route)
	}

	// Find cross-exchange arbitrage
	exchanges := make([]string, 0, len(exchangeRoutes))
	for ex := range exchangeRoutes {
		exchanges = append(exchanges, ex)
	}

	for i := 0; i < len(exchanges); i++ {
		for j := i + 1; j < len(exchanges); j++ {
			ex1, ex2 := exchanges[i], exchanges[j]
			routes1, routes2 := exchangeRoutes[ex1], exchangeRoutes[ex2]

			// Check buy on ex1, sell on ex2
			for _, buyRoute := range routes1 {
				for _, sellRoute := range routes2 {
					profit := mr.calculateArbitrageProfit(buyRoute, sellRoute)
					if profit.GreaterThan(mr.config.ArbitrageThreshold) {
						opp := &ArbitrageOpportunity{
							ID:              fmt.Sprintf("arb_%s_%d", symbol, time.Now().UnixNano()),
							Symbol:          symbol,
							BuyExchange:     buyRoute.Exchange,
							BuyAccount:      buyRoute.AccountID,
							SellExchange:    sellRoute.Exchange,
							SellAccount:     sellRoute.AccountID,
							BuyPrice:        buyRoute.Price,
							SellPrice:       sellRoute.Price,
							ProfitPercent:   profit,
							MaxQuantity:     decimal.Min(buyRoute.AvailableLiquidity, sellRoute.AvailableLiquidity),
							RequiredCapital: buyRoute.Price.Mul(buyRoute.AvailableLiquidity),
							EstimatedFees:   buyRoute.EstimatedFees.Add(sellRoute.EstimatedFees),
							Confidence:      decimal.NewFromFloat(0.95), // High confidence
							ExpiresAt:       time.Now().Add(5 * time.Second),
						}
						opp.NetProfit = opp.ProfitPercent.Sub(opp.EstimatedFees)
						opportunities = append(opportunities, opp)
					}
				}
			}
		}
	}

	// Sort by net profit
	sort.Slice(opportunities, func(i, j int) bool {
		return opportunities[i].NetProfit.GreaterThan(opportunities[j].NetProfit)
	})

	return opportunities
}

// calculateArbitrageProfit calculates profit percentage for arbitrage
func (mr *MultiAccountRouter) calculateArbitrageProfit(buyRoute, sellRoute *Route) decimal.Decimal {
	if buyRoute.Price.IsZero() {
		return decimal.Zero
	}
	return sellRoute.Price.Sub(buyRoute.Price).Div(buyRoute.Price).Mul(decimal.NewFromInt(100))
}

// executeArbitrage executes an arbitrage opportunity
func (mr *MultiAccountRouter) executeArbitrage(ctx context.Context, opp *ArbitrageOpportunity, req *RoutingRequest) (*RoutingResult, error) {
	result := &RoutingResult{
		RequestID:     fmt.Sprintf("arb_%s", req.ID),
		Symbol:        opp.Symbol,
		TotalQuantity: opp.MaxQuantity,
		Routes:        make([]*ExecutedRoute, 0),
		StartTime:     time.Now(),
	}

	// Execute buy and sell orders in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := make([]error, 0)

	// Buy order
	wg.Add(1)
	go func() {
		defer wg.Done()
		buyReq := &RoutingRequest{
			ID:        fmt.Sprintf("%s_buy", result.RequestID),
			Symbol:    opp.Symbol,
			Side:      types.OrderSideBuy,
			Quantity:  opp.MaxQuantity,
			OrderType: types.OrderTypeLimit,
			Price:     opp.BuyPrice,
			AccountID: opp.BuyAccount,
		}

		buyRoute := &Route{
			Exchange:  opp.BuyExchange,
			AccountID: opp.BuyAccount,
			Price:     opp.BuyPrice,
		}

		executed, err := mr.executeOnRoute(ctx, buyReq, buyRoute)
		mu.Lock()
		if err != nil {
			errors = append(errors, err)
		} else {
			result.Routes = append(result.Routes, executed)
			result.ExecutedQuantity = result.ExecutedQuantity.Add(executed.ExecutedQuantity)
			result.TotalFees = result.TotalFees.Add(executed.Fees)
		}
		mu.Unlock()
	}()

	// Sell order
	wg.Add(1)
	go func() {
		defer wg.Done()
		sellReq := &RoutingRequest{
			ID:        fmt.Sprintf("%s_sell", result.RequestID),
			Symbol:    opp.Symbol,
			Side:      types.OrderSideSell,
			Quantity:  opp.MaxQuantity,
			OrderType: types.OrderTypeLimit,
			Price:     opp.SellPrice,
			AccountID: opp.SellAccount,
		}

		sellRoute := &Route{
			Exchange:  opp.SellExchange,
			AccountID: opp.SellAccount,
			Price:     opp.SellPrice,
		}

		executed, err := mr.executeOnRoute(ctx, sellReq, sellRoute)
		mu.Lock()
		if err != nil {
			errors = append(errors, err)
		} else {
			result.Routes = append(result.Routes, executed)
			result.TotalFees = result.TotalFees.Add(executed.Fees)
		}
		mu.Unlock()
	}()

	wg.Wait()

	result.EndTime = time.Now()
	result.Success = len(errors) == 0

	if len(errors) > 0 {
		result.Errors = errors
	}

	return result, nil
}

// Additional routing strategies

func (mr *MultiAccountRouter) routeBestLiquidityOrder(ctx context.Context, req *RoutingRequest, routes []*Route) (*RoutingResult, error) {
	// Sort by available liquidity
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].AvailableLiquidity.GreaterThan(routes[j].AvailableLiquidity)
	})

	return mr.executeSingleRoute(ctx, req, routes[0])
}

func (mr *MultiAccountRouter) routeLowestFeeOrder(ctx context.Context, req *RoutingRequest, routes []*Route) (*RoutingResult, error) {
	// Sort by lowest fees
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].EstimatedFees.LessThan(routes[j].EstimatedFees)
	})

	return mr.executeSingleRoute(ctx, req, routes[0])
}

func (mr *MultiAccountRouter) routeFastestOrder(ctx context.Context, req *RoutingRequest, routes []*Route) (*RoutingResult, error) {
	// Sort by rate limit availability (more available = faster)
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].RateLimitWeight < routes[j].RateLimitWeight
	})

	return mr.executeSingleRoute(ctx, req, routes[0])
}

func (mr *MultiAccountRouter) routeBalancedOrder(ctx context.Context, req *RoutingRequest, routes []*Route) (*RoutingResult, error) {
	// Routes already sorted by composite score
	return mr.executeSingleRoute(ctx, req, routes[0])
}

// Helper methods

func (mr *MultiAccountRouter) validateRequest(req *RoutingRequest) error {
	if req.Symbol == "" {
		return fmt.Errorf("symbol is required")
	}
	if req.Quantity.IsZero() || req.Quantity.IsNegative() {
		return fmt.Errorf("invalid quantity")
	}
	if req.OrderType == types.OrderTypeLimit && req.Price.IsZero() {
		return fmt.Errorf("price required for limit orders")
	}
	return nil
}

func (mr *MultiAccountRouter) getAggregatedMarketData(symbol string) (map[string]*MarketData, error) {
	return mr.priceAggregator.GetAggregatedData(symbol, mr.exchanges)
}

func (mr *MultiAccountRouter) updateRoutingMetrics(result *RoutingResult) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	mr.metrics.TotalOrders++
	if result.Success {
		mr.metrics.SuccessfulOrders++
	} else {
		mr.metrics.FailedOrders++
	}

	mr.metrics.TotalVolume = mr.metrics.TotalVolume.Add(result.ExecutedQuantity.Mul(result.AveragePrice))
	mr.metrics.TotalFees = mr.metrics.TotalFees.Add(result.TotalFees)

	// Calculate slippage
	if result.AveragePrice.IsPositive() && len(result.Routes) > 0 {
		expectedPrice := result.Routes[0].Price
		slippage := result.AveragePrice.Sub(expectedPrice).Div(expectedPrice).Abs()
		
		// Update average slippage
		totalSlippage := mr.metrics.AverageSlippage.Mul(decimal.NewFromInt(mr.metrics.TotalOrders - 1))
		mr.metrics.AverageSlippage = totalSlippage.Add(slippage).Div(decimal.NewFromInt(mr.metrics.TotalOrders))
	}

	// Track best route
	if len(result.Routes) > 0 {
		mr.metrics.BestRoute = fmt.Sprintf("%s:%s", result.Routes[0].Exchange, result.Routes[0].AccountID)
	}

	mr.metrics.UpdatedAt = time.Now()
}

// Background loops

func (mr *MultiAccountRouter) priceUpdateLoop() {
	ticker := time.NewTicker(mr.config.PriceUpdateInterval)
	defer ticker.Stop()

	for range ticker.C {
		mr.priceAggregator.UpdatePrices(mr.exchanges)
	}
}

func (mr *MultiAccountRouter) arbitrageDetectionLoop() {
	ticker := time.NewTicker(100 * time.Millisecond) // Fast detection
	defer ticker.Stop()

	symbols := []string{"BTCUSDT", "ETHUSDT", "BNBUSDT"} // Common symbols

	for range ticker.C {
		for _, symbol := range symbols {
			marketData, err := mr.getAggregatedMarketData(symbol)
			if err != nil {
				continue
			}

			// Build dummy routes for arbitrage detection
			var routes []*Route
			for exchange, data := range marketData {
				routes = append(routes, &Route{
					Exchange: exchange,
					Price:    data.AskPrice,
				})
			}

			opportunities := mr.findArbitrageOpportunities(symbol, routes)
			if len(opportunities) > 0 {
				// Log or alert about opportunities
				fmt.Printf("Arbitrage opportunity found: %+v\n", opportunities[0])
			}
		}
	}
}

// GetMetrics returns current routing metrics
func (mr *MultiAccountRouter) GetMetrics() *RoutingMetrics {
	mr.mu.RLock()
	defer mr.mu.RUnlock()
	
	// Return a copy
	metrics := *mr.metrics
	return &metrics
}

// Stop stops the router and its background tasks
func (mr *MultiAccountRouter) Stop() {
	// Stop background tasks
	// In production, use proper context cancellation
}