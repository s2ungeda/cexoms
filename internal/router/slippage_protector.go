package router

import (
	"fmt"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// SlippageProtector protects against excessive slippage
type SlippageProtector struct {
	maxSlippageBps int // Maximum slippage in basis points
	config         SlippageConfig
}

// SlippageConfig contains slippage protection configuration
type SlippageConfig struct {
	// Price impact thresholds
	WarningThresholdBps  int // Warn if slippage exceeds this
	BlockingThresholdBps int // Block if slippage exceeds this
	
	// Volume impact thresholds
	MaxVolumeImpact float64 // Maximum % of total volume
	
	// Spread thresholds
	MaxSpreadBps int // Maximum acceptable spread in bps
	
	// Time-based protections
	VolatilityWindow  int // Minutes to look back for volatility
	MaxVolatilityStd  float64 // Maximum standard deviations
}

// NewSlippageProtector creates a new slippage protector
func NewSlippageProtector(maxSlippageBps int) *SlippageProtector {
	config := SlippageConfig{
		WarningThresholdBps:  maxSlippageBps / 2,
		BlockingThresholdBps: maxSlippageBps,
		MaxVolumeImpact:     0.1,  // 10% of volume
		MaxSpreadBps:        50,   // 0.5% spread
		VolatilityWindow:    60,   // 1 hour
		MaxVolatilityStd:    2.0,  // 2 standard deviations
	}

	return &SlippageProtector{
		maxSlippageBps: maxSlippageBps,
		config:         config,
	}
}

// CheckMarketImpact checks if an order would cause excessive market impact
func (sp *SlippageProtector) CheckMarketImpact(request RouteRequest, conditions *MarketConditions) string {
	// Check spread
	if warning := sp.checkSpread(conditions); warning != "" {
		return warning
	}

	// Check volume impact
	if warning := sp.checkVolumeImpact(request, conditions); warning != "" {
		return warning
	}

	// Check price impact
	if warning := sp.checkPriceImpact(request, conditions); warning != "" {
		return warning
	}

	// Check volatility
	if warning := sp.checkVolatility(conditions); warning != "" {
		return warning
	}

	return ""
}

// EstimateSlippage estimates slippage for a given order
func (sp *SlippageProtector) EstimateSlippage(request RouteRequest, book *AggregatedOrderBook) (decimal.Decimal, error) {
	if book == nil {
		return decimal.Zero, fmt.Errorf("no order book data")
	}

	remainingQty := request.Quantity
	totalCost := decimal.Zero
	
	// Walk through order book levels
	levels := book.Asks
	if request.Side == types.OrderSideSell {
		levels = book.Bids
	}

	for _, level := range levels {
		if remainingQty.IsZero() {
			break
		}

		// Calculate quantity at this level
		levelQty := decimal.Min(remainingQty, level.TotalSize)
		levelCost := levelQty.Mul(level.Price)
		
		totalCost = totalCost.Add(levelCost)
		remainingQty = remainingQty.Sub(levelQty)
	}

	// Check if we have enough liquidity
	if remainingQty.GreaterThan(decimal.Zero) {
		return decimal.Zero, fmt.Errorf("insufficient liquidity: %s remaining", remainingQty)
	}

	// Calculate average execution price
	avgPrice := totalCost.Div(request.Quantity)
	
	// Calculate slippage from reference price
	var referencePrice decimal.Decimal
	if request.OrderType == types.OrderTypeLimit && !request.Price.IsZero() {
		referencePrice = request.Price
	} else if len(levels) > 0 {
		referencePrice = levels[0].Price
	} else {
		return decimal.Zero, fmt.Errorf("no reference price available")
	}

	slippage := avgPrice.Sub(referencePrice).Div(referencePrice).Abs()
	
	return slippage, nil
}

// CalculateOptimalSlices calculates optimal order slices to minimize slippage
func (sp *SlippageProtector) CalculateOptimalSlices(request RouteRequest, book *AggregatedOrderBook) []OrderSlice {
	slices := []OrderSlice{}
	
	// Determine slice size based on liquidity
	avgLevelSize := sp.calculateAverageLevelSize(book, request.Side)
	optimalSliceSize := avgLevelSize.Mul(decimal.NewFromFloat(0.3)) // 30% of average level
	
	// Ensure minimum slice size
	minSliceSize := request.Quantity.Div(decimal.NewFromInt(100)) // At least 1% per slice
	if optimalSliceSize.LessThan(minSliceSize) {
		optimalSliceSize = minSliceSize
	}
	
	// Create slices
	remainingQty := request.Quantity
	sliceNum := 0
	
	for remainingQty.GreaterThan(decimal.Zero) {
		sliceSize := decimal.Min(optimalSliceSize, remainingQty)
		
		slice := OrderSlice{
			Number:   sliceNum + 1,
			Quantity: sliceSize,
			Delay:    sliceNum * 30, // 30 seconds between slices
		}
		
		// Estimate impact for this slice
		slippage, _ := sp.EstimateSlippage(RouteRequest{
			Symbol:   request.Symbol,
			Side:     request.Side,
			Quantity: sliceSize,
		}, book)
		
		slice.EstimatedSlippage = slippage
		
		slices = append(slices, slice)
		remainingQty = remainingQty.Sub(sliceSize)
		sliceNum++
	}
	
	return slices
}

// ValidateExecution validates execution parameters against slippage limits
func (sp *SlippageProtector) ValidateExecution(routes []Route, marketData map[string]*AggregatedOrderBook) error {
	for _, route := range routes {
		book, exists := marketData[route.Symbol]
		if !exists {
			return fmt.Errorf("no market data for %s", route.Symbol)
		}

		// Estimate slippage for this route
		slippage, err := sp.EstimateSlippage(RouteRequest{
			Symbol:   route.Symbol,
			Side:     types.OrderSideBuy, // Would need actual side
			Quantity: route.Quantity,
			Price:    route.Price,
		}, book)
		
		if err != nil {
			return fmt.Errorf("failed to estimate slippage for %s: %w", route.Venue, err)
		}

		// Convert to basis points
		slippageBps := slippage.Mul(decimal.NewFromInt(10000)).IntPart()
		
		if int(slippageBps) > sp.config.BlockingThresholdBps {
			return fmt.Errorf("excessive slippage on %s: %d bps (max: %d bps)", 
				route.Venue, slippageBps, sp.config.BlockingThresholdBps)
		}
	}

	return nil
}

// GetSlippageMetrics returns current slippage metrics
func (sp *SlippageProtector) GetSlippageMetrics(symbol string, book *AggregatedOrderBook) *SlippageMetrics {
	metrics := &SlippageMetrics{
		Symbol:    symbol,
		Timestamp: book.LastUpdate,
	}

	// Calculate spread
	if len(book.Bids) > 0 && len(book.Asks) > 0 {
		spread := book.Asks[0].Price.Sub(book.Bids[0].Price)
		midPrice := book.Bids[0].Price.Add(book.Asks[0].Price).Div(decimal.NewFromInt(2))
		metrics.SpreadBps = spread.Div(midPrice).Mul(decimal.NewFromInt(10000)).IntPart()
	}

	// Calculate depth metrics
	metrics.BidDepth = sp.calculateDepthMetrics(book.Bids)
	metrics.AskDepth = sp.calculateDepthMetrics(book.Asks)

	// Calculate imbalance
	if !book.TotalBidSize.IsZero() {
		metrics.OrderBookImbalance = book.TotalAskSize.Sub(book.TotalBidSize).Div(book.TotalBidSize.Add(book.TotalAskSize)).Abs().Float64()
	}

	return metrics
}

// Helper methods

func (sp *SlippageProtector) checkSpread(conditions *MarketConditions) string {
	if conditions.Spread.IsZero() {
		return ""
	}

	// Calculate spread in basis points
	midPrice := conditions.Spread.Div(decimal.NewFromInt(2))
	if midPrice.IsZero() {
		return ""
	}

	spreadBps := conditions.Spread.Div(midPrice).Mul(decimal.NewFromInt(10000)).IntPart()
	
	if int(spreadBps) > sp.config.MaxSpreadBps {
		return fmt.Sprintf("Wide spread detected: %d bps (max: %d bps)", spreadBps, sp.config.MaxSpreadBps)
	}

	return ""
}

func (sp *SlippageProtector) checkVolumeImpact(request RouteRequest, conditions *MarketConditions) string {
	totalVolume := conditions.Liquidity.TotalBidVolume
	if request.Side == types.OrderSideBuy {
		totalVolume = conditions.Liquidity.TotalAskVolume
	}

	if totalVolume.IsZero() {
		return "No liquidity available"
	}

	volumeImpact := request.Quantity.Div(totalVolume).Float64()
	
	if volumeImpact > sp.config.MaxVolumeImpact {
		return fmt.Sprintf("Order size too large: %.1f%% of available volume (max: %.1f%%)", 
			volumeImpact*100, sp.config.MaxVolumeImpact*100)
	}

	return ""
}

func (sp *SlippageProtector) checkPriceImpact(request RouteRequest, conditions *MarketConditions) string {
	// Estimate how many price levels the order would consume
	levels := conditions.Liquidity.AskLiquidity
	if request.Side == types.OrderSideSell {
		levels = conditions.Liquidity.BidLiquidity
	}

	if len(levels) == 0 {
		return "No depth data available"
	}

	// Find which level would be reached
	remainingQty := request.Quantity
	var lastPrice decimal.Decimal
	
	for _, level := range levels {
		lastPrice = level.Price
		if remainingQty.LessThanOrEqual(level.CumulativeVolume) {
			break
		}
	}

	// Calculate price impact
	firstPrice := levels[0].Price
	priceImpact := lastPrice.Sub(firstPrice).Div(firstPrice).Abs()
	impactBps := priceImpact.Mul(decimal.NewFromInt(10000)).IntPart()

	if int(impactBps) > sp.config.WarningThresholdBps {
		return fmt.Sprintf("High price impact expected: %d bps", impactBps)
	}

	return ""
}

func (sp *SlippageProtector) checkVolatility(conditions *MarketConditions) string {
	if conditions.Volatility > sp.config.MaxVolatilityStd*0.02 { // Assuming 2% = 1 std dev
		return fmt.Sprintf("High volatility: %.1f%% (max: %.1f%%)", 
			conditions.Volatility*100, sp.config.MaxVolatilityStd*2)
	}

	return ""
}

func (sp *SlippageProtector) calculateAverageLevelSize(book *AggregatedOrderBook, side types.OrderSide) decimal.Decimal {
	levels := book.Asks
	if side == types.OrderSideSell {
		levels = book.Bids
	}

	if len(levels) == 0 {
		return decimal.Zero
	}

	// Calculate average size of top 5 levels
	totalSize := decimal.Zero
	count := 0
	
	for i, level := range levels {
		if i >= 5 {
			break
		}
		totalSize = totalSize.Add(level.TotalSize)
		count++
	}

	if count == 0 {
		return decimal.Zero
	}

	return totalSize.Div(decimal.NewFromInt(int64(count)))
}

func (sp *SlippageProtector) calculateDepthMetrics(levels []AggregatedLevel) DepthMetrics {
	metrics := DepthMetrics{
		Levels: make([]LevelMetrics, 0, len(levels)),
	}

	cumVolume := decimal.Zero
	cumValue := decimal.Zero

	for i, level := range levels {
		cumVolume = cumVolume.Add(level.TotalSize)
		cumValue = cumValue.Add(level.TotalSize.Mul(level.Price))

		levelMetric := LevelMetrics{
			Price:            level.Price,
			Size:             level.TotalSize,
			CumulativeSize:   cumVolume,
			CumulativeValue:  cumValue,
			VenueCount:       level.VenueCount,
		}

		metrics.Levels = append(metrics.Levels, levelMetric)

		// Calculate metrics at different depths
		if i == 4 { // Top 5 levels
			metrics.Size5Levels = cumVolume
			metrics.Value5Levels = cumValue
		} else if i == 9 { // Top 10 levels
			metrics.Size10Levels = cumVolume
			metrics.Value10Levels = cumValue
		}
	}

	metrics.TotalSize = cumVolume
	metrics.TotalValue = cumValue

	return metrics
}

// Types for slippage protection

type OrderSlice struct {
	Number            int
	Quantity          decimal.Decimal
	Delay             int // Seconds to wait before execution
	EstimatedSlippage decimal.Decimal
}

type SlippageMetrics struct {
	Symbol             string
	Timestamp          time.Time
	SpreadBps          int64
	BidDepth           DepthMetrics
	AskDepth           DepthMetrics
	OrderBookImbalance float64
}

type DepthMetrics struct {
	Levels        []LevelMetrics
	TotalSize     decimal.Decimal
	TotalValue    decimal.Decimal
	Size5Levels   decimal.Decimal  // Size available in top 5 levels
	Size10Levels  decimal.Decimal  // Size available in top 10 levels
	Value5Levels  decimal.Decimal  // Value in top 5 levels
	Value10Levels decimal.Decimal  // Value in top 10 levels
}

type LevelMetrics struct {
	Price           decimal.Decimal
	Size            decimal.Decimal
	CumulativeSize  decimal.Decimal
	CumulativeValue decimal.Decimal
	VenueCount      int
}