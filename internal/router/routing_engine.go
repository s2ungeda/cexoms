package router

import (
	"sync"

	"github.com/shopspring/decimal"
)

// RoutingEngine handles routing logic and optimization
type RoutingEngine struct {
	mu     sync.RWMutex
	config *RouterConfig
	rules  []RoutingRule
}

// RoutingRule defines a routing rule
type RoutingRule struct {
	Name      string
	Priority  int
	Condition func(*RoutingRequest) bool
	Action    func(*RoutingRequest) *RoutingRequest
}

// NewRoutingEngine creates a new routing engine
func NewRoutingEngine(config *RouterConfig) *RoutingEngine {
	re := &RoutingEngine{
		config: config,
		rules:  make([]RoutingRule, 0),
	}

	// Add default rules
	re.addDefaultRules()

	return re
}

// addDefaultRules adds default routing rules
func (re *RoutingEngine) addDefaultRules() {
	// Rule: Split large orders
	re.AddRule(RoutingRule{
		Name:     "split_large_orders",
		Priority: 100,
		Condition: func(req *RoutingRequest) bool {
			// Check if order is large (example: > $100,000)
			orderValue := req.Quantity.Mul(req.Price)
			return orderValue.GreaterThan(decimal.NewFromInt(100000))
		},
		Action: func(req *RoutingRequest) *RoutingRequest {
			// Mark for splitting
			if req.Metadata == nil {
				req.Metadata = make(map[string]interface{})
			}
			req.Metadata["split_order"] = true
			return req
		},
	})

	// Rule: Apply slippage tolerance
	re.AddRule(RoutingRule{
		Name:     "apply_slippage",
		Priority: 90,
		Condition: func(req *RoutingRequest) bool {
			return req.OrderType == "limit" && !req.MaxSlippage.IsZero()
		},
		Action: func(req *RoutingRequest) *RoutingRequest {
			// Adjust price based on slippage tolerance
			if req.Side == "buy" {
				// For buy orders, increase price by slippage
				req.Price = req.Price.Mul(decimal.NewFromFloat(1).Add(req.MaxSlippage))
			} else {
				// For sell orders, decrease price by slippage
				req.Price = req.Price.Mul(decimal.NewFromFloat(1).Sub(req.MaxSlippage))
			}
			return req
		},
	})

	// Rule: Minimum execution size
	re.AddRule(RoutingRule{
		Name:     "min_execution_size",
		Priority: 80,
		Condition: func(req *RoutingRequest) bool {
			return !req.MinExecution.IsZero()
		},
		Action: func(req *RoutingRequest) *RoutingRequest {
			if req.Metadata == nil {
				req.Metadata = make(map[string]interface{})
			}
			req.Metadata["min_execution"] = req.MinExecution
			return req
		},
	})
}

// AddRule adds a routing rule
func (re *RoutingEngine) AddRule(rule RoutingRule) {
	re.mu.Lock()
	defer re.mu.Unlock()
	re.rules = append(re.rules, rule)
}

// ApplyRules applies all routing rules to a request
func (re *RoutingEngine) ApplyRules(req *RoutingRequest) *RoutingRequest {
	re.mu.RLock()
	defer re.mu.RUnlock()

	// Sort rules by priority
	// In production, keep them sorted
	result := req
	for _, rule := range re.rules {
		if rule.Condition(result) {
			result = rule.Action(result)
		}
	}

	return result
}

// OptimizeRoute optimizes a route based on current conditions
func (re *RoutingEngine) OptimizeRoute(route *Route, req *RoutingRequest) *Route {
	// Apply optimizations
	optimized := *route

	// Adjust for market impact
	if req.Quantity.GreaterThan(route.AvailableLiquidity.Mul(decimal.NewFromFloat(0.1))) {
		// Large order relative to liquidity
		// Increase estimated fees for market impact
		impactFee := route.EstimatedFees.Mul(decimal.NewFromFloat(1.5))
		optimized.EstimatedFees = impactFee
	}

	// Adjust score based on historical performance
	// In production, this would use historical data
	
	return &optimized
}

// CalculateOptimalSplit calculates optimal order split
func (re *RoutingEngine) CalculateOptimalSplit(totalQty decimal.Decimal, routes []*Route) []decimal.Decimal {
	splits := make([]decimal.Decimal, len(routes))
	
	// Simple proportional split based on liquidity
	totalLiquidity := decimal.Zero
	for _, route := range routes {
		totalLiquidity = totalLiquidity.Add(route.AvailableLiquidity)
	}

	if totalLiquidity.IsZero() {
		return splits
	}

	remainingQty := totalQty
	for i, route := range routes {
		proportion := route.AvailableLiquidity.Div(totalLiquidity)
		splitQty := totalQty.Mul(proportion)
		
		// Ensure we don't exceed available liquidity
		if splitQty.GreaterThan(route.AvailableLiquidity) {
			splitQty = route.AvailableLiquidity
		}

		// Ensure we don't exceed remaining quantity
		if splitQty.GreaterThan(remainingQty) {
			splitQty = remainingQty
		}

		splits[i] = splitQty
		remainingQty = remainingQty.Sub(splitQty)
	}

	return splits
}