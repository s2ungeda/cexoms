package router

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// FeeOptimizer optimizes routing decisions based on fee structures
type FeeOptimizer struct {
	mu           sync.RWMutex
	feeSchedules map[string]*FeeSchedule // venue -> fee schedule
	volumeTiers  map[string]*VolumeTier  // venue -> volume tier info
	feeCache     map[string]FeeRate      // cache for calculated fees
}

// FeeSchedule represents a venue's fee structure
type FeeSchedule struct {
	VenueName       string
	BaseMakerFee    decimal.Decimal
	BaseTakerFee    decimal.Decimal
	TierDiscounts   []TierDiscount
	SpecialPrograms []SpecialProgram
	FeeAsset        string // BNB, FTT, etc.
	LastUpdate      time.Time
}

// TierDiscount represents volume-based fee discounts
type TierDiscount struct {
	VolumeThreshold decimal.Decimal
	MakerDiscount   decimal.Decimal // Percentage discount
	TakerDiscount   decimal.Decimal
}

// SpecialProgram represents special fee programs (e.g., fee rebates)
type SpecialProgram struct {
	Name        string
	Type        string // "rebate", "discount", "zero_fee"
	Conditions  map[string]interface{}
	MakerAdjust decimal.Decimal // Fee adjustment (negative for rebates)
	TakerAdjust decimal.Decimal
}

// VolumeTier tracks user's volume tier on each venue
type VolumeTier struct {
	VenueName        string
	Current30dVolume decimal.Decimal
	CurrentTier      int
	NextTierVolume   decimal.Decimal
	UpdatedAt        time.Time
}

// FeeRate represents calculated fee rates
type FeeRate struct {
	MakerFee      decimal.Decimal
	TakerFee      decimal.Decimal
	EffectiveRate decimal.Decimal // After all discounts/rebates
}

// NewFeeOptimizer creates a new fee optimizer
func NewFeeOptimizer() *FeeOptimizer {
	return &FeeOptimizer{
		feeSchedules: make(map[string]*FeeSchedule),
		volumeTiers:  make(map[string]*VolumeTier),
		feeCache:     make(map[string]FeeRate),
	}
}

// UpdateFeeSchedule updates fee schedule for a venue
func (fo *FeeOptimizer) UpdateFeeSchedule(venue string, schedule *FeeSchedule) {
	fo.mu.Lock()
	defer fo.mu.Unlock()
	
	fo.feeSchedules[venue] = schedule
	// Clear cache for this venue
	delete(fo.feeCache, venue)
}

// UpdateVolumeTier updates volume tier information
func (fo *FeeOptimizer) UpdateVolumeTier(venue string, tier *VolumeTier) {
	fo.mu.Lock()
	defer fo.mu.Unlock()
	
	fo.volumeTiers[venue] = tier
	// Clear cache as fees might change
	delete(fo.feeCache, venue)
}

// CalculateFees calculates fees for a potential order
func (fo *FeeOptimizer) CalculateFees(venue string, orderType types.OrderType, quantity, price decimal.Decimal) (FeeEstimate, error) {
	fo.mu.RLock()
	defer fo.mu.RUnlock()

	schedule, exists := fo.feeSchedules[venue]
	if !exists {
		return FeeEstimate{}, fmt.Errorf("no fee schedule for venue %s", venue)
	}

	// Get effective fee rate
	feeRate := fo.getEffectiveFeeRate(venue, orderType)
	
	// Calculate notional value
	notional := quantity.Mul(price)
	
	// Calculate fee
	var fee decimal.Decimal
	if orderType == types.OrderTypeLimit {
		fee = notional.Mul(feeRate.MakerFee)
	} else {
		fee = notional.Mul(feeRate.TakerFee)
	}

	return FeeEstimate{
		Venue:        venue,
		OrderType:    orderType,
		Notional:     notional,
		Fee:          fee,
		FeeRate:      feeRate.EffectiveRate,
		FeeAsset:     schedule.FeeAsset,
		IncludesRebate: feeRate.EffectiveRate.IsNegative(),
	}, nil
}

// OptimizeRoutesByFee optimizes routes considering fees
func (fo *FeeOptimizer) OptimizeRoutesByFee(routes []Route, orderSide types.OrderSide) ([]Route, decimal.Decimal) {
	fo.mu.RLock()
	defer fo.mu.RUnlock()

	// Calculate total fees for each route
	routeFees := make([]RouteFeeInfo, len(routes))
	totalFees := decimal.Zero

	for i, route := range routes {
		feeInfo := fo.calculateRouteFee(route, orderSide)
		routeFees[i] = feeInfo
		totalFees = totalFees.Add(feeInfo.TotalFee)
		
		// Update route with fee estimate
		routes[i].EstimatedFee = feeInfo.TotalFee
	}

	// Sort routes by effective cost (price + fees for buys, price - fees for sells)
	optimizedRoutes := fo.sortRoutesByCost(routes, routeFees, orderSide)

	return optimizedRoutes, totalFees
}

// CompareVenueFees compares fees across venues for a given order
func (fo *FeeOptimizer) CompareVenueFees(orderSize decimal.Decimal, orderType types.OrderType) []VenueFeeComparison {
	fo.mu.RLock()
	defer fo.mu.RUnlock()

	comparisons := []VenueFeeComparison{}

	for venue, schedule := range fo.feeSchedules {
		feeRate := fo.getEffectiveFeeRate(venue, orderType)
		
		var effectiveFee decimal.Decimal
		if orderType == types.OrderTypeLimit {
			effectiveFee = feeRate.MakerFee
		} else {
			effectiveFee = feeRate.TakerFee
		}

		comparison := VenueFeeComparison{
			Venue:         venue,
			BaseFee:       schedule.BaseTakerFee,
			EffectiveFee:  effectiveFee,
			TierLevel:     fo.getCurrentTier(venue),
			HasRebate:     effectiveFee.IsNegative(),
			FeeAsset:      schedule.FeeAsset,
		}

		// Calculate fee savings compared to base fee
		if orderType == types.OrderTypeLimit {
			comparison.Savings = schedule.BaseMakerFee.Sub(effectiveFee)
		} else {
			comparison.Savings = schedule.BaseTakerFee.Sub(effectiveFee)
		}

		comparisons = append(comparisons, comparison)
	}

	// Sort by effective fee (lowest first)
	fo.sortVenueComparisons(comparisons)

	return comparisons
}

// EstimateFeeImpact estimates the fee impact of different routing strategies
func (fo *FeeOptimizer) EstimateFeeImpact(request RouteRequest, strategies []RoutingStrategy) map[RoutingStrategy]FeeImpact {
	impacts := make(map[RoutingStrategy]FeeImpact)

	for _, strategy := range strategies {
		impact := fo.calculateStrategyFeeImpact(request, strategy)
		impacts[strategy] = impact
	}

	return impacts
}

// Helper methods

func (fo *FeeOptimizer) getEffectiveFeeRate(venue string, orderType types.OrderType) FeeRate {
	// Check cache first
	if cached, exists := fo.feeCache[venue]; exists {
		return cached
	}

	schedule := fo.feeSchedules[venue]
	if schedule == nil {
		return FeeRate{}
	}

	// Start with base fees
	rate := FeeRate{
		MakerFee: schedule.BaseMakerFee,
		TakerFee: schedule.BaseTakerFee,
	}

	// Apply tier discounts
	if tier := fo.volumeTiers[venue]; tier != nil {
		for _, discount := range schedule.TierDiscounts {
			if tier.Current30dVolume.GreaterThanOrEqual(discount.VolumeThreshold) {
				rate.MakerFee = rate.MakerFee.Mul(decimal.NewFromInt(1).Sub(discount.MakerDiscount))
				rate.TakerFee = rate.TakerFee.Mul(decimal.NewFromInt(1).Sub(discount.TakerDiscount))
			}
		}
	}

	// Apply special programs
	for _, program := range schedule.SpecialPrograms {
		if fo.qualifiesForProgram(venue, program) {
			rate.MakerFee = rate.MakerFee.Add(program.MakerAdjust)
			rate.TakerFee = rate.TakerFee.Add(program.TakerAdjust)
		}
	}

	// Set effective rate based on order type
	if orderType == types.OrderTypeLimit {
		rate.EffectiveRate = rate.MakerFee
	} else {
		rate.EffectiveRate = rate.TakerFee
	}

	// Cache the result
	fo.feeCache[venue] = rate

	return rate
}

func (fo *FeeOptimizer) calculateRouteFee(route Route, orderSide types.OrderSide) RouteFeeInfo {
	notional := route.Quantity.Mul(route.EstimatedPrice)
	feeRate := fo.getEffectiveFeeRate(route.Venue, route.OrderType)
	
	var fee decimal.Decimal
	if route.OrderType == types.OrderTypeLimit {
		fee = notional.Mul(feeRate.MakerFee)
	} else {
		fee = notional.Mul(feeRate.TakerFee)
	}

	// Calculate effective cost
	var effectiveCost decimal.Decimal
	if orderSide == types.OrderSideBuy {
		effectiveCost = notional.Add(fee)
	} else {
		effectiveCost = notional.Sub(fee)
	}

	return RouteFeeInfo{
		Route:         route,
		TotalFee:      fee,
		FeeRate:       feeRate.EffectiveRate,
		EffectiveCost: effectiveCost,
		Notional:      notional,
	}
}

func (fo *FeeOptimizer) sortRoutesByCost(routes []Route, feeInfo []RouteFeeInfo, orderSide types.OrderSide) []Route {
	// Create index slice for sorting
	indices := make([]int, len(routes))
	for i := range indices {
		indices[i] = i
	}

	// Sort indices by effective cost
	for i := 0; i < len(indices); i++ {
		for j := i + 1; j < len(indices); j++ {
			costI := feeInfo[indices[i]].EffectiveCost
			costJ := feeInfo[indices[j]].EffectiveCost
			
			// For buys, prefer lower cost; for sells, prefer higher proceeds
			if orderSide == types.OrderSideBuy {
				if costJ.LessThan(costI) {
					indices[i], indices[j] = indices[j], indices[i]
				}
			} else {
				if costJ.GreaterThan(costI) {
					indices[i], indices[j] = indices[j], indices[i]
				}
			}
		}
	}

	// Reorder routes based on sorted indices
	sortedRoutes := make([]Route, len(routes))
	for i, idx := range indices {
		sortedRoutes[i] = routes[idx]
	}

	return sortedRoutes
}

func (fo *FeeOptimizer) getCurrentTier(venue string) int {
	if tier, exists := fo.volumeTiers[venue]; exists {
		return tier.CurrentTier
	}
	return 0
}

func (fo *FeeOptimizer) qualifiesForProgram(venue string, program SpecialProgram) bool {
	// Simplified qualification check
	// In production, implement actual program logic
	return false
}

func (fo *FeeOptimizer) sortVenueComparisons(comparisons []VenueFeeComparison) {
	// Sort by effective fee (lowest first)
	for i := 0; i < len(comparisons); i++ {
		for j := i + 1; j < len(comparisons); j++ {
			if comparisons[j].EffectiveFee.LessThan(comparisons[i].EffectiveFee) {
				comparisons[i], comparisons[j] = comparisons[j], comparisons[i]
			}
		}
	}
}

func (fo *FeeOptimizer) calculateStrategyFeeImpact(request RouteRequest, strategy RoutingStrategy) FeeImpact {
	// Simplified fee impact calculation
	// In production, simulate actual routing with each strategy
	
	impact := FeeImpact{
		Strategy:      strategy,
		EstimatedFees: decimal.Zero,
		FeePercentage: decimal.Zero,
	}

	// Base fee estimation
	avgFee := decimal.NewFromFloat(0.001) // 0.1% average
	notional := request.Quantity.Mul(request.Price)
	
	switch strategy {
	case StrategyLowestFee:
		impact.EstimatedFees = notional.Mul(avgFee.Mul(decimal.NewFromFloat(0.8)))
	case StrategyFastest:
		impact.EstimatedFees = notional.Mul(avgFee.Mul(decimal.NewFromFloat(1.2)))
	default:
		impact.EstimatedFees = notional.Mul(avgFee)
	}

	impact.FeePercentage = impact.EstimatedFees.Div(notional).Mul(decimal.NewFromInt(100))

	return impact
}

// Types for fee calculations

type FeeEstimate struct {
	Venue          string
	OrderType      types.OrderType
	Notional       decimal.Decimal
	Fee            decimal.Decimal
	FeeRate        decimal.Decimal
	FeeAsset       string
	IncludesRebate bool
}

type RouteFeeInfo struct {
	Route         Route
	TotalFee      decimal.Decimal
	FeeRate       decimal.Decimal
	EffectiveCost decimal.Decimal
	Notional      decimal.Decimal
}

type VenueFeeComparison struct {
	Venue        string
	BaseFee      decimal.Decimal
	EffectiveFee decimal.Decimal
	Savings      decimal.Decimal
	TierLevel    int
	HasRebate    bool
	FeeAsset     string
}

type FeeImpact struct {
	Strategy      RoutingStrategy
	EstimatedFees decimal.Decimal
	FeePercentage decimal.Decimal
	Venues        []string
}