package router

import (
	"fmt"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// OrderSplitter handles order splitting logic
type OrderSplitter struct {
	config SplitterConfig
}

// SplitterConfig contains configuration for order splitting
type SplitterConfig struct {
	MinOrderSize      decimal.Decimal // Minimum order size per venue
	MaxOrderSize      decimal.Decimal // Maximum order size per venue
	OptimalSplitRatio decimal.Decimal // Optimal percentage per venue (e.g., 0.3 = 30%)
	MaxVenues         int             // Maximum venues to split across
	RoundingPrecision int32           // Decimal places for quantity rounding
}

// NewOrderSplitter creates a new order splitter
func NewOrderSplitter(config SplitterConfig) *OrderSplitter {
	return &OrderSplitter{
		config: config,
	}
}

// SplitOrder splits an order across multiple venues based on available liquidity
func (os *OrderSplitter) SplitOrder(request RouteRequest, liquidityInfo map[string]*VenueLiquidity) ([]SplitDecision, error) {
	// Validate input
	if request.Quantity.IsZero() || request.Quantity.IsNegative() {
		return nil, fmt.Errorf("invalid order quantity: %s", request.Quantity)
	}

	// Filter venues based on preferences and liquidity
	eligibleVenues := os.filterEligibleVenues(liquidityInfo, request)
	if len(eligibleVenues) == 0 {
		return nil, fmt.Errorf("no eligible venues found")
	}

	// Choose splitting strategy based on order characteristics
	var splits []SplitDecision
	var err error

	switch request.Strategy {
	case StrategyMinSlippage:
		splits, err = os.splitForMinimalSlippage(request, eligibleVenues)
	case StrategyIceberg:
		splits, err = os.splitIceberg(request, eligibleVenues)
	case StrategyVWAP:
		splits, err = os.splitForVWAP(request, eligibleVenues)
	case StrategyTWAP:
		splits, err = os.splitForTWAP(request, eligibleVenues)
	default:
		splits, err = os.splitProportionally(request, eligibleVenues)
	}

	if err != nil {
		return nil, err
	}

	// Validate and adjust splits
	splits = os.validateAndAdjustSplits(splits, request.Quantity)

	return splits, nil
}

// splitProportionally splits order proportionally based on available liquidity
func (os *OrderSplitter) splitProportionally(request RouteRequest, venues map[string]*VenueLiquidity) ([]SplitDecision, error) {
	totalLiquidity := os.calculateTotalLiquidity(venues, request.Side)
	if totalLiquidity.IsZero() {
		return nil, fmt.Errorf("no liquidity available")
	}

	splits := []SplitDecision{}
	remainingQty := request.Quantity

	// Sort venues by liquidity (highest first)
	sortedVenues := os.sortVenuesByLiquidity(venues, request.Side)

	for _, venue := range sortedVenues {
		if remainingQty.IsZero() {
			break
		}

		// Calculate proportional quantity
		venueLiquidity := os.getVenueLiquidity(venues[venue], request.Side)
		proportion := venueLiquidity.Div(totalLiquidity)
		splitQty := request.Quantity.Mul(proportion)

		// Apply constraints
		splitQty = os.applyConstraints(splitQty, remainingQty)

		if splitQty.GreaterThan(decimal.Zero) {
			split := SplitDecision{
				Venue:        venue,
				Quantity:     splitQty,
				Percentage:   splitQty.Div(request.Quantity).Mul(decimal.NewFromInt(100)),
				ExpectedCost: os.estimateCost(venues[venue], splitQty, request.Side),
				Priority:     len(splits) + 1,
			}
			splits = append(splits, split)
			remainingQty = remainingQty.Sub(splitQty)
		}

		// Limit number of venues
		if len(splits) >= os.config.MaxVenues {
			break
		}
	}

	// Distribute remaining quantity
	if remainingQty.GreaterThan(decimal.Zero) && len(splits) > 0 {
		splits[0].Quantity = splits[0].Quantity.Add(remainingQty)
	}

	return splits, nil
}

// splitForMinimalSlippage optimizes for minimal market impact
func (os *OrderSplitter) splitForMinimalSlippage(request RouteRequest, venues map[string]*VenueLiquidity) ([]SplitDecision, error) {
	splits := []SplitDecision{}
	remainingQty := request.Quantity

	// Sort venues by spread (tightest first)
	sortedVenues := os.sortVenuesBySpread(venues)

	for _, venue := range sortedVenues {
		if remainingQty.IsZero() {
			break
		}

		liquidity := venues[venue]
		
		// Calculate quantity that can be filled without significant slippage
		maxQtyNoSlippage := os.calculateMaxQtyWithoutSlippage(liquidity, request.Side, request.MaxSlippage)
		splitQty := decimal.Min(maxQtyNoSlippage, remainingQty)

		// Apply minimum order size
		if splitQty.LessThan(os.config.MinOrderSize) {
			continue
		}

		split := SplitDecision{
			Venue:           venue,
			Quantity:        splitQty,
			Percentage:      splitQty.Div(request.Quantity).Mul(decimal.NewFromInt(100)),
			ExpectedCost:    os.estimateCost(liquidity, splitQty, request.Side),
			ExpectedSlippage: os.estimateSlippage(liquidity, splitQty, request.Side),
			Priority:        len(splits) + 1,
		}

		splits = append(splits, split)
		remainingQty = remainingQty.Sub(splitQty)

		if len(splits) >= os.config.MaxVenues {
			break
		}
	}

	return splits, nil
}

// splitIceberg creates iceberg-style splits to hide order size
func (os *OrderSplitter) splitIceberg(request RouteRequest, venues map[string]*VenueLiquidity) ([]SplitDecision, error) {
	// Iceberg orders show only a small portion at a time
	visibleRatio := decimal.NewFromFloat(0.1) // Show only 10% at a time
	
	// Calculate visible size per venue
	visibleSize := request.Quantity.Mul(visibleRatio)
	if visibleSize.LessThan(os.config.MinOrderSize) {
		visibleSize = os.config.MinOrderSize
	}

	// Calculate number of slices needed
	numSlices := request.Quantity.Div(visibleSize).Ceil().IntPart()
	sliceSize := request.Quantity.Div(decimal.NewFromInt(numSlices))

	splits := []SplitDecision{}
	
	// Distribute slices across venues
	venueList := os.getVenueList(venues)
	for i := int64(0); i < numSlices; i++ {
		venueIdx := int(i) % len(venueList)
		venue := venueList[venueIdx]
		
		split := SplitDecision{
			Venue:         venue,
			Quantity:      sliceSize,
			Percentage:    sliceSize.Div(request.Quantity).Mul(decimal.NewFromInt(100)),
			TimeDelay:     int(i) * 5, // 5 second delay between slices
			IsIceberg:     true,
			SliceNumber:   int(i) + 1,
			TotalSlices:   int(numSlices),
			Priority:      int(i) + 1,
		}
		
		splits = append(splits, split)
	}

	return splits, nil
}

// splitForVWAP optimizes for Volume Weighted Average Price
func (os *OrderSplitter) splitForVWAP(request RouteRequest, venues map[string]*VenueLiquidity) ([]SplitDecision, error) {
	// VWAP strategy distributes orders based on historical volume patterns
	// For simplicity, we'll use current volume distribution
	
	totalVolume := decimal.Zero
	volumeMap := make(map[string]decimal.Decimal)
	
	for venue, liquidity := range venues {
		volume := liquidity.Volume24h
		volumeMap[venue] = volume
		totalVolume = totalVolume.Add(volume)
	}

	if totalVolume.IsZero() {
		return nil, fmt.Errorf("no volume data available")
	}

	splits := []SplitDecision{}
	remainingQty := request.Quantity

	for venue, volume := range volumeMap {
		if remainingQty.IsZero() {
			break
		}

		// Calculate proportion based on volume
		proportion := volume.Div(totalVolume)
		splitQty := request.Quantity.Mul(proportion)
		splitQty = os.applyConstraints(splitQty, remainingQty)

		if splitQty.GreaterThan(decimal.Zero) {
			split := SplitDecision{
				Venue:        venue,
				Quantity:     splitQty,
				Percentage:   splitQty.Div(request.Quantity).Mul(decimal.NewFromInt(100)),
				VolumeWeight: proportion,
				Priority:     len(splits) + 1,
			}
			splits = append(splits, split)
			remainingQty = remainingQty.Sub(splitQty)
		}
	}

	return splits, nil
}

// splitForTWAP creates time-weighted splits
func (os *OrderSplitter) splitForTWAP(request RouteRequest, venues map[string]*VenueLiquidity) ([]SplitDecision, error) {
	// TWAP distributes orders evenly over time
	// Define time intervals (e.g., 5 minutes)
	intervalMinutes := 5
	totalMinutes := 60 // Execute over 1 hour
	numIntervals := totalMinutes / intervalMinutes
	
	intervalSize := request.Quantity.Div(decimal.NewFromInt(int64(numIntervals)))
	
	splits := []SplitDecision{}
	venueList := os.getVenueList(venues)
	
	for i := 0; i < numIntervals; i++ {
		// Rotate through venues
		venueIdx := i % len(venueList)
		venue := venueList[venueIdx]
		
		split := SplitDecision{
			Venue:        venue,
			Quantity:     intervalSize,
			Percentage:   intervalSize.Div(request.Quantity).Mul(decimal.NewFromInt(100)),
			TimeDelay:    i * intervalMinutes * 60, // Convert to seconds
			TimeInterval: intervalMinutes,
			Priority:     i + 1,
		}
		
		splits = append(splits, split)
	}

	return splits, nil
}

// Helper methods

func (os *OrderSplitter) filterEligibleVenues(venues map[string]*VenueLiquidity, request RouteRequest) map[string]*VenueLiquidity {
	eligible := make(map[string]*VenueLiquidity)

	for venue, liquidity := range venues {
		// Skip if in avoid list
		if os.isInList(venue, request.AvoidVenues) {
			continue
		}

		// Check if preferred venue (if list is not empty)
		if len(request.PreferredVenues) > 0 && !os.isInList(venue, request.PreferredVenues) {
			continue
		}

		// Check minimum liquidity
		venueLiq := os.getVenueLiquidity(liquidity, request.Side)
		if venueLiq.GreaterThan(os.config.MinOrderSize) {
			eligible[venue] = liquidity
		}
	}

	return eligible
}

func (os *OrderSplitter) calculateTotalLiquidity(venues map[string]*VenueLiquidity, side types.OrderSide) decimal.Decimal {
	total := decimal.Zero
	for _, liquidity := range venues {
		total = total.Add(os.getVenueLiquidity(liquidity, side))
	}
	return total
}

func (os *OrderSplitter) getVenueLiquidity(liquidity *VenueLiquidity, side types.OrderSide) decimal.Decimal {
	if side == types.OrderSideBuy {
		return liquidity.AskLiquidity
	}
	return liquidity.BidLiquidity
}

func (os *OrderSplitter) applyConstraints(quantity, remaining decimal.Decimal) decimal.Decimal {
	// Apply minimum order size
	if quantity.LessThan(os.config.MinOrderSize) {
		return decimal.Zero
	}

	// Apply maximum order size
	if quantity.GreaterThan(os.config.MaxOrderSize) {
		quantity = os.config.MaxOrderSize
	}

	// Don't exceed remaining quantity
	if quantity.GreaterThan(remaining) {
		quantity = remaining
	}

	// Round to appropriate precision
	return quantity.Round(os.config.RoundingPrecision)
}

func (os *OrderSplitter) validateAndAdjustSplits(splits []SplitDecision, totalQuantity decimal.Decimal) []SplitDecision {
	// Calculate actual total
	actualTotal := decimal.Zero
	for _, split := range splits {
		actualTotal = actualTotal.Add(split.Quantity)
	}

	// Adjust for rounding errors
	diff := totalQuantity.Sub(actualTotal)
	if !diff.IsZero() && len(splits) > 0 {
		// Add difference to the largest split
		largestIdx := 0
		for i, split := range splits {
			if split.Quantity.GreaterThan(splits[largestIdx].Quantity) {
				largestIdx = i
			}
		}
		splits[largestIdx].Quantity = splits[largestIdx].Quantity.Add(diff)
	}

	// Recalculate percentages
	for i := range splits {
		splits[i].Percentage = splits[i].Quantity.Div(totalQuantity).Mul(decimal.NewFromInt(100))
	}

	return splits
}

func (os *OrderSplitter) sortVenuesByLiquidity(venues map[string]*VenueLiquidity, side types.OrderSide) []string {
	type venueLiq struct {
		venue     string
		liquidity decimal.Decimal
	}

	venueLiqs := []venueLiq{}
	for venue, liq := range venues {
		venueLiqs = append(venueLiqs, venueLiq{
			venue:     venue,
			liquidity: os.getVenueLiquidity(liq, side),
		})
	}

	// Sort by liquidity (highest first)
	for i := 0; i < len(venueLiqs); i++ {
		for j := i + 1; j < len(venueLiqs); j++ {
			if venueLiqs[j].liquidity.GreaterThan(venueLiqs[i].liquidity) {
				venueLiqs[i], venueLiqs[j] = venueLiqs[j], venueLiqs[i]
			}
		}
	}

	sorted := []string{}
	for _, vl := range venueLiqs {
		sorted = append(sorted, vl.venue)
	}

	return sorted
}

func (os *OrderSplitter) sortVenuesBySpread(venues map[string]*VenueLiquidity) []string {
	type venueSpread struct {
		venue  string
		spread decimal.Decimal
	}

	venueSpreads := []venueSpread{}
	for venue, liq := range venues {
		venueSpreads = append(venueSpreads, venueSpread{
			venue:  venue,
			spread: liq.Spread,
		})
	}

	// Sort by spread (tightest first)
	for i := 0; i < len(venueSpreads); i++ {
		for j := i + 1; j < len(venueSpreads); j++ {
			if venueSpreads[j].spread.LessThan(venueSpreads[i].spread) {
				venueSpreads[i], venueSpreads[j] = venueSpreads[j], venueSpreads[i]
			}
		}
	}

	sorted := []string{}
	for _, vs := range venueSpreads {
		sorted = append(sorted, vs.venue)
	}

	return sorted
}

func (os *OrderSplitter) calculateMaxQtyWithoutSlippage(liquidity *VenueLiquidity, side types.OrderSide, maxSlippage decimal.Decimal) decimal.Decimal {
	// Simplified calculation - in production, analyze order book depth
	var availableLiq decimal.Decimal
	if side == types.OrderSideBuy {
		availableLiq = liquidity.AskLiquidityDepth[0].Volume
	} else {
		availableLiq = liquidity.BidLiquidityDepth[0].Volume
	}

	// Apply slippage factor (e.g., only use 50% of top level)
	return availableLiq.Mul(decimal.NewFromFloat(0.5))
}

func (os *OrderSplitter) estimateCost(liquidity *VenueLiquidity, quantity decimal.Decimal, side types.OrderSide) decimal.Decimal {
	// Simplified cost estimation
	var price decimal.Decimal
	if side == types.OrderSideBuy {
		price = liquidity.BestAsk
	} else {
		price = liquidity.BestBid
	}
	
	return quantity.Mul(price)
}

func (os *OrderSplitter) estimateSlippage(liquidity *VenueLiquidity, quantity decimal.Decimal, side types.OrderSide) decimal.Decimal {
	// Simplified slippage estimation
	// In production, walk through order book levels
	impact := quantity.Div(os.getVenueLiquidity(liquidity, side))
	slippageBps := impact.Mul(decimal.NewFromInt(100)) // 1% of liquidity = 100bps slippage
	
	return slippageBps
}

func (os *OrderSplitter) isInList(venue string, list []string) bool {
	for _, v := range list {
		if v == venue {
			return true
		}
	}
	return false
}

func (os *OrderSplitter) getVenueList(venues map[string]*VenueLiquidity) []string {
	list := []string{}
	for venue := range venues {
		list = append(list, venue)
	}
	return list
}

// Types for order splitting

type SplitDecision struct {
	Venue            string          `json:"venue"`
	Quantity         decimal.Decimal `json:"quantity"`
	Percentage       decimal.Decimal `json:"percentage"`      // Percentage of total order
	ExpectedCost     decimal.Decimal `json:"expected_cost"`
	ExpectedSlippage decimal.Decimal `json:"expected_slippage"` // In basis points
	Priority         int             `json:"priority"`          // Execution priority
	TimeDelay        int             `json:"time_delay"`        // Delay in seconds (for TWAP/Iceberg)
	TimeInterval     int             `json:"time_interval"`     // Interval in minutes (for TWAP)
	VolumeWeight     decimal.Decimal `json:"volume_weight"`     // For VWAP
	IsIceberg        bool            `json:"is_iceberg"`
	SliceNumber      int             `json:"slice_number"`      // For iceberg orders
	TotalSlices      int             `json:"total_slices"`
}

type VenueLiquidity struct {
	Venue             string
	BestBid           decimal.Decimal
	BestAsk           decimal.Decimal
	BidLiquidity      decimal.Decimal // Total bid liquidity
	AskLiquidity      decimal.Decimal // Total ask liquidity
	Spread            decimal.Decimal
	BidLiquidityDepth []PriceLevel
	AskLiquidityDepth []PriceLevel
	Volume24h         decimal.Decimal
	LastUpdate        time.Time
}

type PriceLevel struct {
	Price  decimal.Decimal
	Volume decimal.Decimal
}