package router

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// RoutingEngine handles the core routing logic
type RoutingEngine struct {
	router         *SmartRouter
	config         *RoutingConfig
	orderBookCache map[string]map[string]*types.OrderBook // exchange -> symbol -> orderbook
	cacheMu        sync.RWMutex
}

// RoutingConfig contains routing engine configuration
type RoutingConfig struct {
	// Slippage settings
	MaxSlippagePercent   decimal.Decimal
	SlippageCalculation  string // "linear", "square_root", "logarithmic"
	
	// Liquidity settings
	MinLiquidityRatio    decimal.Decimal // Min liquidity as ratio of order size
	LiquidityDepth       int             // Order book levels to consider
	
	// Split settings
	MaxSplits            int
	MinSplitSize         decimal.Decimal
	OptimalSplitRatio    decimal.Decimal
	
	// Fee settings
	ConsiderRebates      bool
	MaxTotalFeePercent   decimal.Decimal
}

// NewRoutingEngine creates a new routing engine
func NewRoutingEngine(router *SmartRouter, config *RoutingConfig) *RoutingEngine {
	if config == nil {
		config = &RoutingConfig{
			MaxSlippagePercent:  decimal.NewFromFloat(0.002), // 0.2%
			SlippageCalculation: "square_root",
			MinLiquidityRatio:   decimal.NewFromFloat(0.5),
			LiquidityDepth:      20,
			MaxSplits:           10,
			MinSplitSize:        decimal.NewFromFloat(100), // $100 minimum
			OptimalSplitRatio:   decimal.NewFromFloat(0.3), // 30% of available liquidity
			ConsiderRebates:     true,
			MaxTotalFeePercent:  decimal.NewFromFloat(0.001), // 0.1%
		}
	}
	
	return &RoutingEngine{
		router:         router,
		config:         config,
		orderBookCache: make(map[string]map[string]*types.OrderBook),
	}
}

// FindBestRoute finds the optimal route for an order
func (e *RoutingEngine) FindBestRoute(ctx context.Context, order *types.Order, options RoutingOptions) (*RoutingDecision, error) {
	// Get market depth across all exchanges
	marketDepth, err := e.getAggregatedMarketDepth(order.Symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get market depth: %w", err)
	}
	
	// Check if order needs splitting
	needsSplit, splitReason := e.shouldSplitOrder(order, marketDepth, options)
	
	if !needsSplit {
		// Find single best route
		route, err := e.findSingleRoute(order, marketDepth, options)
		if err != nil {
			return nil, err
		}
		
		return &RoutingDecision{
			ID:            fmt.Sprintf("route_%d", time.Now().UnixNano()),
			OriginalOrder: order,
			Routes:        []Route{*route},
			TotalQuantity: order.Quantity,
			CreatedAt:     time.Now(),
		}, nil
	}
	
	// Find optimal split routes
	routes, err := e.findSplitRoutes(order, marketDepth, options, splitReason)
	if err != nil {
		return nil, err
	}
	
	decision := &RoutingDecision{
		ID:            fmt.Sprintf("route_%d", time.Now().UnixNano()),
		OriginalOrder: order,
		Routes:        routes,
		TotalQuantity: order.Quantity,
		CreatedAt:     time.Now(),
	}
	
	// Calculate expected metrics
	e.calculateExpectedMetrics(decision, marketDepth)
	
	return decision, nil
}

// findSingleRoute finds the best single exchange route
func (e *RoutingEngine) findSingleRoute(order *types.Order, marketDepth *AggregatedMarketDepth, options RoutingOptions) (*Route, error) {
	var bestRoute *Route
	bestScore := decimal.NewFromFloat(-1)
	
	// Evaluate each exchange
	for exchange, depth := range marketDepth.ExchangeDepths {
		// Skip excluded exchanges
		if e.isExchangeExcluded(exchange, options) {
			continue
		}
		
		// Check if exchange is allowed
		if !e.isExchangeAllowed(exchange, options) {
			continue
		}
		
		// Calculate execution metrics
		metrics := e.calculateExecutionMetrics(order, depth, exchange)
		
		// Skip if insufficient liquidity
		if metrics.AvailableQuantity.LessThan(order.Quantity.Mul(e.config.MinLiquidityRatio)) {
			continue
		}
		
		// Skip if slippage too high
		if metrics.Slippage.GreaterThan(options.MaxSlippage) {
			continue
		}
		
		// Skip if fees too high
		if options.IncludeFees && metrics.TotalFee.GreaterThan(order.Quantity.Mul(order.Price).Mul(options.MaxFeePercent)) {
			continue
		}
		
		// Calculate score
		score := e.calculateRouteScore(metrics, options)
		
		if score.GreaterThan(bestScore) {
			bestScore = score
			bestRoute = &Route{
				Exchange:      exchange,
				Symbol:        order.Symbol,
				Quantity:      order.Quantity,
				ExpectedPrice: metrics.AveragePrice,
				ExpectedFee:   metrics.TotalFee,
				Priority:      1,
			}
		}
	}
	
	if bestRoute == nil {
		return nil, fmt.Errorf("no suitable exchange found for order")
	}
	
	return bestRoute, nil
}

// findSplitRoutes finds optimal routes for a split order
func (e *RoutingEngine) findSplitRoutes(order *types.Order, marketDepth *AggregatedMarketDepth, options RoutingOptions, splitReason string) ([]Route, error) {
	// Calculate optimal splits based on liquidity distribution
	splits := e.calculateOptimalSplits(order, marketDepth, options)
	
	if len(splits) == 0 {
		return nil, fmt.Errorf("unable to split order: %s", splitReason)
	}
	
	// Convert splits to routes
	routes := make([]Route, 0, len(splits))
	for i, split := range splits {
		routes = append(routes, Route{
			Exchange:      split.Exchange,
			Symbol:        order.Symbol,
			Quantity:      split.Quantity,
			ExpectedPrice: split.ExpectedPrice,
			ExpectedFee:   split.ExpectedFee,
			Priority:      i + 1,
		})
	}
	
	return routes, nil
}

// shouldSplitOrder determines if an order should be split
func (e *RoutingEngine) shouldSplitOrder(order *types.Order, marketDepth *AggregatedMarketDepth, options RoutingOptions) (bool, string) {
	// Check if splitting is disabled
	if options.MaxSplits <= 1 {
		return false, ""
	}
	
	// Calculate total available liquidity at acceptable price levels
	totalLiquidity := e.calculateTotalLiquidity(order, marketDepth, options.MaxSlippage)
	
	// Check if single exchange has enough liquidity
	for _, depth := range marketDepth.ExchangeDepths {
		exchangeLiquidity := e.calculateExchangeLiquidity(order, depth, options.MaxSlippage)
		if exchangeLiquidity.GreaterThanOrEqual(order.Quantity) {
			return false, ""
		}
	}
	
	// Check if total liquidity across exchanges is sufficient
	if totalLiquidity.LessThan(order.Quantity) {
		return true, "insufficient liquidity on single exchange"
	}
	
	// Check if splitting would reduce slippage significantly
	singleExchangeSlippage := e.estimateSingleExchangeSlippage(order, marketDepth)
	splitSlippage := e.estimateSplitSlippage(order, marketDepth)
	
	slippageReduction := singleExchangeSlippage.Sub(splitSlippage).Div(singleExchangeSlippage)
	if slippageReduction.GreaterThan(decimal.NewFromFloat(0.2)) { // 20% reduction
		return true, "splitting reduces slippage significantly"
	}
	
	// Check order size threshold
	orderValue := order.Quantity.Mul(order.Price)
	if orderValue.GreaterThan(decimal.NewFromInt(50000)) { // $50k
		return true, "large order size"
	}
	
	return false, ""
}

// calculateOptimalSplits calculates optimal order splits
func (e *RoutingEngine) calculateOptimalSplits(order *types.Order, marketDepth *AggregatedMarketDepth, options RoutingOptions) []OrderSplit {
	splits := make([]OrderSplit, 0)
	remainingQuantity := order.Quantity
	
	// Get exchanges sorted by liquidity score
	exchanges := e.sortExchangesByLiquidityScore(order, marketDepth, options)
	
	for _, exchange := range exchanges {
		if remainingQuantity.IsZero() || len(splits) >= options.MaxSplits {
			break
		}
		
		depth := marketDepth.ExchangeDepths[exchange.Name]
		
		// Calculate optimal quantity for this exchange
		optimalQty := e.calculateOptimalQuantity(remainingQuantity, depth, order.Side)
		
		// Skip if too small
		if optimalQty.LessThan(e.config.MinSplitSize) {
			continue
		}
		
		// Don't exceed remaining quantity
		if optimalQty.GreaterThan(remainingQuantity) {
			optimalQty = remainingQuantity
		}
		
		// Calculate execution price and fee
		execPrice := e.calculateExecutionPrice(optimalQty, depth, order.Side)
		fee := e.calculateFee(exchange.Name, optimalQty, execPrice)
		
		splits = append(splits, OrderSplit{
			Exchange:      exchange.Name,
			Quantity:      optimalQty,
			ExpectedPrice: execPrice,
			ExpectedFee:   fee,
		})
		
		remainingQuantity = remainingQuantity.Sub(optimalQty)
	}
	
	// If we couldn't split the entire order, try to allocate remaining to existing splits
	if remainingQuantity.GreaterThan(decimal.Zero) && len(splits) > 0 {
		perSplitExtra := remainingQuantity.Div(decimal.NewFromInt(int64(len(splits))))
		for i := range splits {
			splits[i].Quantity = splits[i].Quantity.Add(perSplitExtra)
		}
	}
	
	return splits
}

// calculateExecutionMetrics calculates metrics for order execution
func (e *RoutingEngine) calculateExecutionMetrics(order *types.Order, depth *ExchangeOrderBook, exchange string) *RouteExecutionMetrics {
	metrics := &RouteExecutionMetrics{
		Exchange: exchange,
	}
	
	// Get relevant price levels
	var levels []types.PriceLevel
	if order.Side == types.OrderSideBuy {
		levels = depth.Asks
	} else {
		levels = depth.Bids
	}
	
	// Calculate fill prices and quantities
	remainingQty := order.Quantity
	totalCost := decimal.Zero
	filledQty := decimal.Zero
	
	for _, level := range levels {
		if remainingQty.IsZero() {
			break
		}
		
		fillQty := decimal.Min(remainingQty, level.Quantity)
		fillCost := fillQty.Mul(level.Price)
		
		totalCost = totalCost.Add(fillCost)
		filledQty = filledQty.Add(fillQty)
		remainingQty = remainingQty.Sub(fillQty)
	}
	
	metrics.AvailableQuantity = filledQty
	
	if filledQty.GreaterThan(decimal.Zero) {
		metrics.AveragePrice = totalCost.Div(filledQty)
		
		// Calculate slippage
		if order.Type == types.OrderTypeLimit {
			if order.Side == types.OrderSideBuy {
				metrics.Slippage = metrics.AveragePrice.Sub(order.Price).Div(order.Price)
			} else {
				metrics.Slippage = order.Price.Sub(metrics.AveragePrice).Div(order.Price)
			}
		}
	}
	
	// Calculate fees
	metrics.TotalFee = e.calculateFee(exchange, filledQty, metrics.AveragePrice)
	
	return metrics
}

// calculateRouteScore calculates a score for a route based on execution type
func (e *RoutingEngine) calculateRouteScore(metrics *RouteExecutionMetrics, options RoutingOptions) decimal.Decimal {
	score := decimal.NewFromInt(100)
	
	switch options.ExecutionType {
	case ExecutionTypeBestPrice:
		// Prioritize price
		priceScore := decimal.NewFromInt(1).Sub(metrics.Slippage.Abs())
		score = score.Mul(priceScore)
		
	case ExecutionTypeMinSlippage:
		// Heavily weight slippage
		slippageScore := decimal.NewFromInt(1).Sub(metrics.Slippage.Abs().Mul(decimal.NewFromInt(10)))
		score = score.Mul(slippageScore)
		
	case ExecutionTypeMinFee:
		// Prioritize fees
		feePercent := metrics.TotalFee.Div(metrics.AveragePrice.Mul(metrics.AvailableQuantity))
		feeScore := decimal.NewFromInt(1).Sub(feePercent.Mul(decimal.NewFromInt(100)))
		score = score.Mul(feeScore)
		
	case ExecutionTypeBalanced:
		// Balance all factors
		priceScore := decimal.NewFromInt(1).Sub(metrics.Slippage.Abs())
		feePercent := metrics.TotalFee.Div(metrics.AveragePrice.Mul(metrics.AvailableQuantity))
		feeScore := decimal.NewFromInt(1).Sub(feePercent.Mul(decimal.NewFromInt(50)))
		liquidityScore := metrics.AvailableQuantity.Div(metrics.AvailableQuantity.Add(decimal.NewFromInt(1000)))
		
		score = score.Mul(priceScore).Mul(feeScore).Mul(liquidityScore)
	}
	
	return score
}

// Helper types

// AggregatedMarketDepth represents market depth across exchanges
type AggregatedMarketDepth struct {
	Symbol          string
	ExchangeDepths  map[string]*ExchangeOrderBook
	TotalBidVolume  decimal.Decimal
	TotalAskVolume  decimal.Decimal
}

// ExchangeOrderBook represents order book for a single exchange
type ExchangeOrderBook struct {
	Exchange string
	Bids     []types.PriceLevel
	Asks     []types.PriceLevel
	Timestamp time.Time
}

// RouteExecutionMetrics represents metrics for route execution
type RouteExecutionMetrics struct {
	Exchange          string
	AvailableQuantity decimal.Decimal
	AveragePrice      decimal.Decimal
	Slippage          decimal.Decimal
	TotalFee          decimal.Decimal
}

// OrderSplit represents a split portion of an order
type OrderSplit struct {
	Exchange      string
	Quantity      decimal.Decimal
	ExpectedPrice decimal.Decimal
	ExpectedFee   decimal.Decimal
}

// ExchangeScore represents exchange scoring for routing
type ExchangeScore struct {
	Name            string
	LiquidityScore  decimal.Decimal
	PriceScore      decimal.Decimal
	FeeScore        decimal.Decimal
	TotalScore      decimal.Decimal
}

// Helper methods

func (e *RoutingEngine) isExchangeExcluded(exchange string, options RoutingOptions) bool {
	for _, excluded := range options.ExcludedExchanges {
		if excluded == exchange {
			return true
		}
	}
	return false
}

func (e *RoutingEngine) isExchangeAllowed(exchange string, options RoutingOptions) bool {
	if len(options.AllowedExchanges) == 0 {
		return true
	}
	
	for _, allowed := range options.AllowedExchanges {
		if allowed == exchange {
			return true
		}
	}
	return false
}

func (e *RoutingEngine) calculateFee(exchange string, quantity, price decimal.Decimal) decimal.Decimal {
	// Simplified fee calculation
	// In production, would use actual fee schedules
	feeRate := decimal.NewFromFloat(0.001) // 0.1%
	return quantity.Mul(price).Mul(feeRate)
}

func (e *RoutingEngine) getAggregatedMarketDepth(symbol string) (*AggregatedMarketDepth, error) {
	// In production, this would aggregate real-time order book data
	// For now, return mock data
	return &AggregatedMarketDepth{
		Symbol:         symbol,
		ExchangeDepths: make(map[string]*ExchangeOrderBook),
	}, nil
}

func (e *RoutingEngine) calculateTotalLiquidity(order *types.Order, marketDepth *AggregatedMarketDepth, maxSlippage decimal.Decimal) decimal.Decimal {
	total := decimal.Zero
	
	for _, depth := range marketDepth.ExchangeDepths {
		liquidity := e.calculateExchangeLiquidity(order, depth, maxSlippage)
		total = total.Add(liquidity)
	}
	
	return total
}

func (e *RoutingEngine) calculateExchangeLiquidity(order *types.Order, depth *ExchangeOrderBook, maxSlippage decimal.Decimal) decimal.Decimal {
	var levels []types.PriceLevel
	var referencePrice decimal.Decimal
	
	if order.Side == types.OrderSideBuy {
		levels = depth.Asks
		if len(levels) > 0 {
			referencePrice = levels[0].Price
		}
	} else {
		levels = depth.Bids
		if len(levels) > 0 {
			referencePrice = levels[0].Price
		}
	}
	
	if referencePrice.IsZero() {
		return decimal.Zero
	}
	
	totalLiquidity := decimal.Zero
	
	for _, level := range levels {
		// Calculate slippage for this level
		var slippage decimal.Decimal
		if order.Side == types.OrderSideBuy {
			slippage = level.Price.Sub(referencePrice).Div(referencePrice)
		} else {
			slippage = referencePrice.Sub(level.Price).Div(referencePrice)
		}
		
		// Stop if slippage exceeds max
		if slippage.GreaterThan(maxSlippage) {
			break
		}
		
		totalLiquidity = totalLiquidity.Add(level.Quantity)
	}
	
	return totalLiquidity
}

func (e *RoutingEngine) estimateSingleExchangeSlippage(order *types.Order, marketDepth *AggregatedMarketDepth) decimal.Decimal {
	minSlippage := decimal.NewFromInt(999)
	
	for _, depth := range marketDepth.ExchangeDepths {
		metrics := e.calculateExecutionMetrics(order, depth, "")
		if metrics.AvailableQuantity.GreaterThanOrEqual(order.Quantity) {
			if metrics.Slippage.LessThan(minSlippage) {
				minSlippage = metrics.Slippage
			}
		}
	}
	
	return minSlippage
}

func (e *RoutingEngine) estimateSplitSlippage(order *types.Order, marketDepth *AggregatedMarketDepth) decimal.Decimal {
	// Simplified estimation
	// In production, would simulate actual split execution
	singleSlippage := e.estimateSingleExchangeSlippage(order, marketDepth)
	
	// Assume splitting reduces slippage by square root of split count
	estimatedSplits := decimal.NewFromInt(int64(e.config.MaxSplits))
	reductionFactor := decimal.NewFromFloat(math.Sqrt(estimatedSplits.InexactFloat64()))
	
	return singleSlippage.Div(reductionFactor)
}

func (e *RoutingEngine) sortExchangesByLiquidityScore(order *types.Order, marketDepth *AggregatedMarketDepth, options RoutingOptions) []ExchangeScore {
	scores := make([]ExchangeScore, 0)
	
	for exchange, depth := range marketDepth.ExchangeDepths {
		if e.isExchangeExcluded(exchange, options) || !e.isExchangeAllowed(exchange, options) {
			continue
		}
		
		metrics := e.calculateExecutionMetrics(order, depth, exchange)
		
		score := ExchangeScore{
			Name:           exchange,
			LiquidityScore: metrics.AvailableQuantity,
			PriceScore:     decimal.NewFromInt(1).Sub(metrics.Slippage.Abs()),
			FeeScore:       decimal.NewFromInt(1).Sub(metrics.TotalFee.Div(decimal.NewFromInt(1000))),
		}
		
		score.TotalScore = score.LiquidityScore.Mul(score.PriceScore).Mul(score.FeeScore)
		scores = append(scores, score)
	}
	
	// Sort by total score
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].TotalScore.GreaterThan(scores[j].TotalScore)
	})
	
	return scores
}

func (e *RoutingEngine) calculateOptimalQuantity(remainingQty decimal.Decimal, depth *ExchangeOrderBook, side types.OrderSide) decimal.Decimal {
	var levels []types.PriceLevel
	
	if side == types.OrderSideBuy {
		levels = depth.Asks
	} else {
		levels = depth.Bids
	}
	
	if len(levels) == 0 {
		return decimal.Zero
	}
	
	// Calculate available liquidity at top levels
	availableLiquidity := decimal.Zero
	levelsToConsider := int(math.Min(float64(e.config.LiquidityDepth), float64(len(levels))))
	
	for i := 0; i < levelsToConsider; i++ {
		availableLiquidity = availableLiquidity.Add(levels[i].Quantity)
	}
	
	// Take optimal ratio of available liquidity
	optimalQty := availableLiquidity.Mul(e.config.OptimalSplitRatio)
	
	// Don't exceed remaining quantity
	if optimalQty.GreaterThan(remainingQty) {
		optimalQty = remainingQty
	}
	
	return optimalQty
}

func (e *RoutingEngine) calculateExecutionPrice(quantity decimal.Decimal, depth *ExchangeOrderBook, side types.OrderSide) decimal.Decimal {
	var levels []types.PriceLevel
	
	if side == types.OrderSideBuy {
		levels = depth.Asks
	} else {
		levels = depth.Bids
	}
	
	remainingQty := quantity
	totalCost := decimal.Zero
	
	for _, level := range levels {
		if remainingQty.IsZero() {
			break
		}
		
		fillQty := decimal.Min(remainingQty, level.Quantity)
		fillCost := fillQty.Mul(level.Price)
		
		totalCost = totalCost.Add(fillCost)
		remainingQty = remainingQty.Sub(fillQty)
	}
	
	if quantity.GreaterThan(remainingQty) {
		filledQty := quantity.Sub(remainingQty)
		return totalCost.Div(filledQty)
	}
	
	return decimal.Zero
}

func (e *RoutingEngine) calculateExpectedMetrics(decision *RoutingDecision, marketDepth *AggregatedMarketDepth) {
	totalQuantity := decimal.Zero
	totalCost := decimal.Zero
	totalFees := decimal.Zero
	
	for _, route := range decision.Routes {
		totalQuantity = totalQuantity.Add(route.Quantity)
		totalCost = totalCost.Add(route.Quantity.Mul(route.ExpectedPrice))
		totalFees = totalFees.Add(route.ExpectedFee)
	}
	
	if totalQuantity.GreaterThan(decimal.Zero) {
		decision.ExpectedPrice = totalCost.Div(totalQuantity)
		decision.ExpectedFees = totalFees
		
		// Calculate expected slippage
		if decision.OriginalOrder.Type == types.OrderTypeLimit {
			if decision.OriginalOrder.Side == types.OrderSideBuy {
				decision.ExpectedSlippage = decision.ExpectedPrice.Sub(decision.OriginalOrder.Price).Div(decision.OriginalOrder.Price)
			} else {
				decision.ExpectedSlippage = decision.OriginalOrder.Price.Sub(decision.ExpectedPrice).Div(decision.OriginalOrder.Price)
			}
		}
	}
}