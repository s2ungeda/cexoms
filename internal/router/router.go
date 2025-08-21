package router

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mExOms/internal/exchange"
	"github.com/mExOms/pkg/types"
	"github.com/mExOms/pkg/utils"
	"github.com/shopspring/decimal"
)

// SmartRouter orchestrates intelligent order routing across multiple venues
type SmartRouter struct {
	mu                sync.RWMutex
	config            RoutingConfig
	venues            map[string]VenueConnector
	liquidityAgg      *LiquidityAggregator
	feeOptimizer      *FeeOptimizer
	orderSplitter     *OrderSplitter
	slippageProtector *SlippageProtector
	performanceTracker *PerformanceTracker
	activeRoutes      map[string]*ActiveRoute
	stopCh            chan struct{}
}

// VenueConnector wraps exchange client with routing metadata
type VenueConnector struct {
	Exchange    types.Exchange
	VenueInfo   *VenueInfo
	IsAvailable bool
	LastError   error
	LastCheck   time.Time
}

// ActiveRoute tracks an active routing execution
type ActiveRoute struct {
	RequestID     string
	Request       RouteRequest
	Routes        []Route
	Status        ExecutionStatus
	StartTime     time.Time
	LastUpdate    time.Time
	ExecutedRoutes []ExecutedRoute
	Errors        []string
}

// NewSmartRouter creates a new smart order router
func NewSmartRouter(config RoutingConfig) *SmartRouter {
	splitterConfig := SplitterConfig{
		MinOrderSize:      config.MinSplitSize,
		MaxOrderSize:      decimal.NewFromFloat(1000000), // Default max
		OptimalSplitRatio: decimal.NewFromFloat(0.3),
		MaxVenues:         config.MaxVenues,
		RoundingPrecision: 8,
	}

	return &SmartRouter{
		config:             config,
		venues:             make(map[string]VenueConnector),
		liquidityAgg:       NewLiquidityAggregator(config.RefreshInterval),
		feeOptimizer:       NewFeeOptimizer(),
		orderSplitter:      NewOrderSplitter(splitterConfig),
		slippageProtector:  NewSlippageProtector(config.MaxSlippageBps),
		performanceTracker: NewPerformanceTracker(),
		activeRoutes:       make(map[string]*ActiveRoute),
		stopCh:             make(chan struct{}),
	}
}

// AddVenue adds a trading venue to the router
func (sr *SmartRouter) AddVenue(name string, exchange types.Exchange, venueInfo *VenueInfo) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	connector := VenueConnector{
		Exchange:    exchange,
		VenueInfo:   venueInfo,
		IsAvailable: true,
		LastCheck:   time.Now(),
	}

	sr.venues[name] = connector
	
	// Add to liquidity aggregator
	client := &exchangeVenueClient{
		exchange: exchange,
		venueInfo: venueInfo,
	}
	sr.liquidityAgg.AddVenue(name, client)

	// Update fee schedules
	feeSchedule := &FeeSchedule{
		VenueName:    name,
		BaseMakerFee: venueInfo.TradingFees.MakerFee,
		BaseTakerFee: venueInfo.TradingFees.TakerFee,
		FeeAsset:     venueInfo.TradingFees.FeeAsset,
		LastUpdate:   time.Now(),
	}
	sr.feeOptimizer.UpdateFeeSchedule(name, feeSchedule)

	return nil
}

// Start starts the smart router
func (sr *SmartRouter) Start(ctx context.Context) error {
	// Start liquidity aggregation
	sr.liquidityAgg.Start(ctx)

	// Start venue health monitoring
	go sr.monitorVenueHealth(ctx)

	// Start performance tracking
	go sr.performanceTracker.Start(ctx)

	return nil
}

// Stop stops the smart router
func (sr *SmartRouter) Stop() {
	close(sr.stopCh)
	sr.liquidityAgg.Stop()
	sr.performanceTracker.Stop()
}

// RouteOrder routes an order across multiple venues
func (sr *SmartRouter) RouteOrder(ctx context.Context, request RouteRequest) (*RouteResponse, error) {
	startTime := time.Now()
	requestID := utils.GenerateID()

	// Validate request
	if err := sr.validateRequest(request); err != nil {
		return nil, fmt.Errorf("invalid route request: %w", err)
	}

	// Create active route tracking
	activeRoute := &ActiveRoute{
		RequestID:  requestID,
		Request:    request,
		Status:     ExecutionPending,
		StartTime:  startTime,
		LastUpdate: startTime,
	}
	
	sr.mu.Lock()
	sr.activeRoutes[requestID] = activeRoute
	sr.mu.Unlock()

	// Get market conditions
	marketConditions, err := sr.liquidityAgg.GetMarketConditions(request.Symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get market conditions: %w", err)
	}

	// Check slippage protection
	if sr.config.SmartRoutingEnabled {
		if warning := sr.slippageProtector.CheckMarketImpact(request, marketConditions); warning != "" {
			if request.Urgency != UrgencyImmediate {
				return nil, fmt.Errorf("slippage protection triggered: %s", warning)
			}
		}
	}

	// Get available venues
	availableVenues := sr.getAvailableVenues(request)
	if len(availableVenues) == 0 {
		return nil, fmt.Errorf("no available venues for routing")
	}

	// Aggregate liquidity information
	liquidityInfo := sr.aggregateLiquidity(request.Symbol, availableVenues)

	// Calculate optimal routes
	routes, err := sr.calculateOptimalRoutes(request, liquidityInfo, marketConditions)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate routes: %w", err)
	}

	// Optimize for fees if enabled
	if sr.config.FeeOptimization {
		totalFees := decimal.Zero
		routes, totalFees = sr.feeOptimizer.OptimizeRoutesByFee(routes, request.Side)
		for i := range routes {
			routes[i].EstimatedFee = totalFees.Div(decimal.NewFromInt(int64(len(routes))))
		}
	}

	// Estimate execution metrics
	estimatedPrice := sr.calculateVWAP(routes)
	estimatedFees := sr.calculateTotalFees(routes)
	estimatedTime := sr.estimateExecutionTime(routes, request.Urgency)

	// Update active route
	activeRoute.Routes = routes
	activeRoute.Status = ExecutionInProgress
	activeRoute.LastUpdate = time.Now()

	// Create response
	response := &RouteResponse{
		RequestID:      requestID,
		Routes:         routes,
		TotalQuantity:  request.Quantity,
		EstimatedPrice: estimatedPrice,
		EstimatedFees:  estimatedFees,
		EstimatedTime:  estimatedTime,
		Confidence:     sr.calculateConfidence(routes, marketConditions),
	}

	// Add warnings if any
	response.Warnings = sr.generateWarnings(request, routes, marketConditions)

	// Track performance
	sr.performanceTracker.RecordRouting(request, response)

	return response, nil
}

// ExecuteRoutes executes the calculated routes
func (sr *SmartRouter) ExecuteRoutes(ctx context.Context, requestID string) (*ExecutionReport, error) {
	sr.mu.RLock()
	activeRoute, exists := sr.activeRoutes[requestID]
	sr.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no active route found for request %s", requestID)
	}

	if activeRoute.Status != ExecutionInProgress {
		return nil, fmt.Errorf("route is not ready for execution: status=%s", activeRoute.Status)
	}

	executionStart := time.Now()
	executedRoutes := []ExecutedRoute{}
	var executionErrors []string
	totalExecuted := decimal.Zero
	totalFees := decimal.Zero

	// Execute routes based on strategy
	switch activeRoute.Request.Strategy {
	case StrategyIceberg, StrategyTWAP:
		// Execute with time delays
		executedRoutes, executionErrors = sr.executeWithTimeDelays(ctx, activeRoute)
	default:
		// Execute in parallel
		executedRoutes, executionErrors = sr.executeInParallel(ctx, activeRoute)
	}

	// Calculate execution metrics
	for _, route := range executedRoutes {
		totalExecuted = totalExecuted.Add(route.ExecutedQty)
		totalFees = totalFees.Add(route.Fee)
	}

	avgPrice := sr.calculateExecutedVWAP(executedRoutes)
	slippageBps := sr.calculateSlippage(activeRoute.Request.Price, avgPrice, activeRoute.Request.Side)

	// Update status
	status := ExecutionCompleted
	if len(executionErrors) > 0 {
		if totalExecuted.IsZero() {
			status = ExecutionFailed
		} else if totalExecuted.LessThan(activeRoute.Request.Quantity) {
			status = ExecutionPartial
		}
	}

	// Create execution report
	report := &ExecutionReport{
		RequestID:      requestID,
		Status:         status,
		ExecutedRoutes: executedRoutes,
		TotalExecuted:  totalExecuted,
		AveragePrice:   avgPrice,
		TotalFees:      totalFees,
		SlippageBps:    slippageBps,
		ExecutionTime:  time.Since(executionStart),
		Timestamp:      time.Now(),
		Errors:         executionErrors,
	}

	// Update active route
	activeRoute.Status = status
	activeRoute.ExecutedRoutes = executedRoutes
	activeRoute.Errors = executionErrors
	activeRoute.LastUpdate = time.Now()

	// Track performance
	sr.performanceTracker.RecordExecution(activeRoute.Request, report)

	return report, nil
}

// SimulateRoute simulates order routing without execution
func (sr *SmartRouter) SimulateRoute(ctx context.Context, request SimulationRequest) (*SimulationResult, error) {
	// Get market conditions based on scenario
	marketConditions := sr.simulateMarketConditions(request.MarketScenario, request.RouteRequest.Symbol)

	// Calculate routes
	routes, err := sr.RouteOrder(ctx, request.RouteRequest)
	if err != nil {
		return nil, err
	}

	// Simulate execution impact
	expectedSlippage := sr.simulateSlippage(routes.Routes, marketConditions)
	executionRisk := sr.assessExecutionRisk(routes.Routes, marketConditions)

	result := &SimulationResult{
		Routes:           routes.Routes,
		ExpectedPrice:    routes.EstimatedPrice,
		ExpectedFees:     routes.EstimatedFees,
		ExpectedSlippage: expectedSlippage,
		ExecutionRisk:    executionRisk,
		Recommendations:  sr.generateRecommendations(request.RouteRequest, routes, marketConditions),
	}

	return result, nil
}

// GetPerformanceMetrics returns router performance metrics
func (sr *SmartRouter) GetPerformanceMetrics() *PerformanceMetrics {
	return sr.performanceTracker.GetMetrics()
}

// Helper methods

func (sr *SmartRouter) validateRequest(request RouteRequest) error {
	if request.Symbol == "" {
		return fmt.Errorf("symbol is required")
	}
	if request.Quantity.IsZero() || request.Quantity.IsNegative() {
		return fmt.Errorf("invalid quantity")
	}
	if request.OrderType == types.OrderTypeLimit && request.Price.IsZero() {
		return fmt.Errorf("price required for limit orders")
	}
	return nil
}

func (sr *SmartRouter) getAvailableVenues(request RouteRequest) map[string]VenueConnector {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	available := make(map[string]VenueConnector)
	
	for name, connector := range sr.venues {
		// Skip if not available
		if !connector.IsAvailable {
			continue
		}

		// Skip if in avoid list
		skipVenue := false
		for _, avoid := range request.AvoidVenues {
			if name == avoid {
				skipVenue = true
				break
			}
		}
		if skipVenue {
			continue
		}

		// If preferred venues specified, only include those
		if len(request.PreferredVenues) > 0 {
			isPreferred := false
			for _, pref := range request.PreferredVenues {
				if name == pref {
					isPreferred = true
					break
				}
			}
			if !isPreferred {
				continue
			}
		}

		available[name] = connector
	}

	return available
}

func (sr *SmartRouter) aggregateLiquidity(symbol string, venues map[string]VenueConnector) map[string]*VenueLiquidity {
	liquidityInfo := make(map[string]*VenueLiquidity)
	
	for name := range venues {
		// Get order book from aggregator
		spreads := sr.liquidityAgg.GetVenueSpread(symbol)
		
		// Get liquidity depth
		bidLiquidity, _ := sr.liquidityAgg.GetLiquidityDepth(symbol, types.OrderSideSell, 10)
		askLiquidity, _ := sr.liquidityAgg.GetLiquidityDepth(symbol, types.OrderSideBuy, 10)
		
		venueLiq := &VenueLiquidity{
			Venue:       name,
			Spread:      spreads[name],
			LastUpdate:  time.Now(),
		}
		
		// Calculate total liquidity
		if len(bidLiquidity) > 0 {
			venueLiq.BidLiquidity = bidLiquidity[len(bidLiquidity)-1].CumulativeVolume
			venueLiq.BestBid = bidLiquidity[0].Price
		}
		if len(askLiquidity) > 0 {
			venueLiq.AskLiquidity = askLiquidity[len(askLiquidity)-1].CumulativeVolume
			venueLiq.BestAsk = askLiquidity[0].Price
		}
		
		liquidityInfo[name] = venueLiq
	}
	
	return liquidityInfo
}

func (sr *SmartRouter) calculateOptimalRoutes(request RouteRequest, liquidityInfo map[string]*VenueLiquidity, conditions *MarketConditions) ([]Route, error) {
	// Use order splitter to calculate splits
	splits, err := sr.orderSplitter.SplitOrder(request, liquidityInfo)
	if err != nil {
		return nil, err
	}

	// Convert splits to routes
	routes := make([]Route, 0, len(splits))
	for _, split := range splits {
		connector := sr.venues[split.Venue]
		
		route := Route{
			Venue:          split.Venue,
			Account:        connector.VenueInfo.Account,
			Market:         connector.VenueInfo.Market,
			Symbol:         request.Symbol,
			Quantity:       split.Quantity,
			OrderType:      request.OrderType,
			Price:          request.Price,
			EstimatedPrice: split.ExpectedCost.Div(split.Quantity),
			EstimatedFee:   decimal.Zero, // Will be calculated by fee optimizer
			Priority:       split.Priority,
			SplitRatio:     split.Percentage.Div(decimal.NewFromInt(100)),
		}
		
		routes = append(routes, route)
	}

	return routes, nil
}

func (sr *SmartRouter) monitorVenueHealth(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-sr.stopCh:
			return
		case <-ticker.C:
			sr.checkVenueHealth()
		}
	}
}

func (sr *SmartRouter) checkVenueHealth() {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	for name, connector := range sr.venues {
		// Simple health check - ping exchange
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := connector.Exchange.GetAccount(ctx)
		cancel()

		connector.LastCheck = time.Now()
		if err != nil {
			connector.IsAvailable = false
			connector.LastError = err
		} else {
			connector.IsAvailable = true
			connector.LastError = nil
		}

		sr.venues[name] = connector
	}
}

func (sr *SmartRouter) calculateVWAP(routes []Route) decimal.Decimal {
	totalValue := decimal.Zero
	totalQuantity := decimal.Zero

	for _, route := range routes {
		value := route.Quantity.Mul(route.EstimatedPrice)
		totalValue = totalValue.Add(value)
		totalQuantity = totalQuantity.Add(route.Quantity)
	}

	if totalQuantity.IsZero() {
		return decimal.Zero
	}

	return totalValue.Div(totalQuantity)
}

func (sr *SmartRouter) calculateTotalFees(routes []Route) decimal.Decimal {
	total := decimal.Zero
	for _, route := range routes {
		total = total.Add(route.EstimatedFee)
	}
	return total
}

func (sr *SmartRouter) estimateExecutionTime(routes []Route, urgency Urgency) time.Duration {
	switch urgency {
	case UrgencyImmediate:
		return 1 * time.Second
	case UrgencyHigh:
		return 5 * time.Second
	case UrgencyNormal:
		return 30 * time.Second
	case UrgencyLow:
		return 5 * time.Minute
	default:
		return 1 * time.Minute
	}
}

func (sr *SmartRouter) calculateConfidence(routes []Route, conditions *MarketConditions) float64 {
	// Simple confidence calculation
	confidence := 0.9

	// Reduce confidence for high volatility
	if conditions.Volatility > 0.05 {
		confidence -= 0.2
	}

	// Reduce confidence for many venues
	if len(routes) > 3 {
		confidence -= 0.1
	}

	// Reduce confidence for wide spreads
	if conditions.Spread.Div(conditions.OrderBooks[routes[0].Venue].Asks[0][0]).GreaterThan(decimal.NewFromFloat(0.01)) {
		confidence -= 0.1
	}

	if confidence < 0 {
		confidence = 0
	}

	return confidence
}

func (sr *SmartRouter) generateWarnings(request RouteRequest, routes []Route, conditions *MarketConditions) []string {
	warnings := []string{}

	// Check for high volatility
	if conditions.Volatility > 0.05 {
		warnings = append(warnings, "High market volatility detected")
	}

	// Check for low liquidity
	if conditions.Liquidity.TotalBidVolume.Add(conditions.Liquidity.TotalAskVolume).LessThan(request.Quantity.Mul(decimal.NewFromInt(2))) {
		warnings = append(warnings, "Low liquidity relative to order size")
	}

	// Check for venue concentration
	if len(routes) == 1 {
		warnings = append(warnings, "Order will be executed on single venue")
	}

	return warnings
}

func (sr *SmartRouter) executeInParallel(ctx context.Context, activeRoute *ActiveRoute) ([]ExecutedRoute, []string) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	
	executedRoutes := []ExecutedRoute{}
	errors := []string{}

	for _, route := range activeRoute.Routes {
		wg.Add(1)
		go func(r Route) {
			defer wg.Done()

			connector := sr.venues[r.Venue]
			
			// Create order
			order := &types.Order{
				Exchange:    r.Venue,
				Symbol:      r.Symbol,
				Side:        activeRoute.Request.Side,
				Type:        r.OrderType,
				Quantity:    r.Quantity,
				Price:       r.Price,
				TimeInForce: activeRoute.Request.TimeInForce,
			}

			// Place order
			placedOrder, err := connector.Exchange.PlaceOrder(ctx, order)
			
			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", r.Venue, err))
				return
			}

			executed := ExecutedRoute{
				Venue:       r.Venue,
				OrderID:     placedOrder.OrderID,
				Quantity:    r.Quantity,
				ExecutedQty: placedOrder.ExecutedQuantity,
				Price:       placedOrder.Price,
				Fee:         decimal.Zero, // Would need to get from order details
				Status:      string(placedOrder.Status),
				Timestamp:   time.Now(),
			}

			executedRoutes = append(executedRoutes, executed)
		}(route)
	}

	wg.Wait()
	return executedRoutes, errors
}

func (sr *SmartRouter) executeWithTimeDelays(ctx context.Context, activeRoute *ActiveRoute) ([]ExecutedRoute, []string) {
	executedRoutes := []ExecutedRoute{}
	errors := []string{}

	for _, route := range activeRoute.Routes {
		// Check if split decision has time delay
		if split, ok := route.Metadata["split_decision"].(SplitDecision); ok && split.TimeDelay > 0 {
			time.Sleep(time.Duration(split.TimeDelay) * time.Second)
		}

		connector := sr.venues[route.Venue]
		
		// Create order
		order := &types.Order{
			Exchange:    route.Venue,
			Symbol:      route.Symbol,
			Side:        activeRoute.Request.Side,
			Type:        route.OrderType,
			Quantity:    route.Quantity,
			Price:       route.Price,
			TimeInForce: activeRoute.Request.TimeInForce,
		}

		// Place order
		placedOrder, err := connector.Exchange.PlaceOrder(ctx, order)
		
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", route.Venue, err))
			continue
		}

		executed := ExecutedRoute{
			Venue:       route.Venue,
			OrderID:     placedOrder.OrderID,
			Quantity:    route.Quantity,
			ExecutedQty: placedOrder.ExecutedQuantity,
			Price:       placedOrder.Price,
			Fee:         decimal.Zero,
			Status:      string(placedOrder.Status),
			Timestamp:   time.Now(),
		}

		executedRoutes = append(executedRoutes, executed)
	}

	return executedRoutes, errors
}

func (sr *SmartRouter) calculateExecutedVWAP(routes []ExecutedRoute) decimal.Decimal {
	totalValue := decimal.Zero
	totalQuantity := decimal.Zero

	for _, route := range routes {
		value := route.ExecutedQty.Mul(route.Price)
		totalValue = totalValue.Add(value)
		totalQuantity = totalQuantity.Add(route.ExecutedQty)
	}

	if totalQuantity.IsZero() {
		return decimal.Zero
	}

	return totalValue.Div(totalQuantity)
}

func (sr *SmartRouter) calculateSlippage(expectedPrice, actualPrice decimal.Decimal, side types.OrderSide) int {
	if expectedPrice.IsZero() {
		return 0
	}

	slippage := actualPrice.Sub(expectedPrice).Div(expectedPrice)
	
	// For sells, invert the slippage
	if side == types.OrderSideSell {
		slippage = slippage.Neg()
	}

	// Convert to basis points
	bps := slippage.Mul(decimal.NewFromInt(10000)).IntPart()
	
	return int(bps)
}

func (sr *SmartRouter) simulateMarketConditions(scenario, symbol string) *MarketConditions {
	// Get current conditions
	conditions, _ := sr.liquidityAgg.GetMarketConditions(symbol)
	
	// Modify based on scenario
	switch scenario {
	case "volatile":
		conditions.Volatility = 0.1 // 10% volatility
		conditions.Spread = conditions.Spread.Mul(decimal.NewFromFloat(2))
	case "illiquid":
		conditions.Liquidity.TotalBidVolume = conditions.Liquidity.TotalBidVolume.Div(decimal.NewFromInt(10))
		conditions.Liquidity.TotalAskVolume = conditions.Liquidity.TotalAskVolume.Div(decimal.NewFromInt(10))
		conditions.Spread = conditions.Spread.Mul(decimal.NewFromFloat(3))
	}

	return conditions
}

func (sr *SmartRouter) simulateSlippage(routes []Route, conditions *MarketConditions) decimal.Decimal {
	// Simple slippage simulation
	baseSlippage := decimal.NewFromFloat(0.001) // 0.1%
	
	// Increase for high volatility
	if conditions.Volatility > 0.05 {
		baseSlippage = baseSlippage.Mul(decimal.NewFromFloat(2))
	}

	// Increase for multiple venues
	if len(routes) > 2 {
		baseSlippage = baseSlippage.Mul(decimal.NewFromFloat(1.5))
	}

	return baseSlippage
}

func (sr *SmartRouter) assessExecutionRisk(routes []Route, conditions *MarketConditions) float64 {
	risk := 0.1 // Base risk

	// Increase risk for volatile markets
	if conditions.Volatility > 0.05 {
		risk += 0.3
	}

	// Increase risk for low liquidity
	totalOrderSize := decimal.Zero
	for _, route := range routes {
		totalOrderSize = totalOrderSize.Add(route.Quantity)
	}
	
	liquidityRatio := conditions.Liquidity.TotalBidVolume.Add(conditions.Liquidity.TotalAskVolume).Div(totalOrderSize)
	if liquidityRatio.LessThan(decimal.NewFromInt(5)) {
		risk += 0.2
	}

	// Cap at 1.0
	if risk > 1.0 {
		risk = 1.0
	}

	return risk
}

func (sr *SmartRouter) generateRecommendations(request RouteRequest, response *RouteResponse, conditions *MarketConditions) []string {
	recommendations := []string{}

	// Check if order size is too large
	totalLiquidity := conditions.Liquidity.TotalBidVolume.Add(conditions.Liquidity.TotalAskVolume)
	if request.Quantity.GreaterThan(totalLiquidity.Div(decimal.NewFromInt(10))) {
		recommendations = append(recommendations, "Consider splitting order over time (TWAP) to reduce market impact")
	}

	// Check volatility
	if conditions.Volatility > 0.05 {
		recommendations = append(recommendations, "High volatility detected - consider using limit orders")
	}

	// Check venue concentration
	if len(response.Routes) < 3 && len(sr.venues) > 3 {
		recommendations = append(recommendations, "Consider distributing across more venues for better execution")
	}

	// Check urgency vs market conditions
	if request.Urgency == UrgencyImmediate && conditions.Spread.Div(response.EstimatedPrice).GreaterThan(decimal.NewFromFloat(0.005)) {
		recommendations = append(recommendations, "Wide spreads detected - consider reducing urgency for better price")
	}

	return recommendations
}

// exchangeVenueClient adapts exchange interface to VenueClient
type exchangeVenueClient struct {
	exchange  types.Exchange
	venueInfo *VenueInfo
}

func (e *exchangeVenueClient) GetOrderBook(ctx context.Context, symbol string) (*types.OrderBook, error) {
	return e.exchange.GetOrderBook(ctx, symbol)
}

func (e *exchangeVenueClient) GetVenueInfo() *VenueInfo {
	return e.venueInfo
}

func (e *exchangeVenueClient) IsConnected() bool {
	// Simple connectivity check
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	_, err := e.exchange.GetAccount(ctx)
	return err == nil
}