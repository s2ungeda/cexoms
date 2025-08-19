package marketmaker

import (
	"math"
	"time"

	"github.com/shopspring/decimal"
)

// DynamicSpreadCalculator calculates spreads based on market conditions and inventory
type DynamicSpreadCalculator struct {
	config        *MarketMakerConfig
	priceHistory  []decimal.Decimal
	tradeHistory  []*TradeFlow
	lastUpdate    time.Time
}

// NewDynamicSpreadCalculator creates a new spread calculator
func NewDynamicSpreadCalculator(config *MarketMakerConfig) *DynamicSpreadCalculator {
	return &DynamicSpreadCalculator{
		config:       config,
		priceHistory: make([]decimal.Decimal, 0, 1000),
		tradeHistory: make([]*TradeFlow, 0, 100),
		lastUpdate:   time.Now(),
	}
}

// CalculateSpread calculates the optimal spread based on current conditions
func (sc *DynamicSpreadCalculator) CalculateSpread(state *MarketState, inventory *InventoryState) decimal.Decimal {
	// Base spread
	baseSpread := sc.config.SpreadBps
	
	// Adjust for volatility
	volAdjustment := sc.calculateVolatilityAdjustment(state.Volatility)
	
	// Adjust for inventory skew
	inventoryAdjustment := sc.calculateInventoryAdjustment(inventory)
	
	// Adjust for market depth
	depthAdjustment := sc.calculateDepthAdjustment(state.OrderBookDepth)
	
	// Adjust for recent trade flow
	flowAdjustment := sc.calculateFlowAdjustment()
	
	// Calculate final spread
	finalSpread := baseSpread.
		Mul(volAdjustment).
		Mul(inventoryAdjustment).
		Mul(depthAdjustment).
		Mul(flowAdjustment)
	
	// Apply min/max constraints
	if finalSpread.LessThan(sc.config.MinSpreadBps) {
		finalSpread = sc.config.MinSpreadBps
	} else if finalSpread.GreaterThan(sc.config.MaxSpreadBps) {
		finalSpread = sc.config.MaxSpreadBps
	}
	
	return finalSpread
}

// calculateVolatilityAdjustment adjusts spread based on volatility
func (sc *DynamicSpreadCalculator) calculateVolatilityAdjustment(volatility decimal.Decimal) decimal.Decimal {
	// Higher volatility = wider spread
	if volatility.IsZero() {
		return decimal.NewFromFloat(1.0)
	}
	
	// Normalize volatility to expected range
	normalizedVol := volatility.Div(sc.config.MinVolatility)
	
	// Apply logarithmic scaling
	adjustment := decimal.NewFromFloat(1.0).Add(
		normalizedVol.Sub(decimal.NewFromFloat(1.0)).Mul(decimal.NewFromFloat(0.5)),
	)
	
	// Cap adjustment
	maxAdjustment := decimal.NewFromFloat(2.0)
	if adjustment.GreaterThan(maxAdjustment) {
		adjustment = maxAdjustment
	}
	
	return adjustment
}

// calculateInventoryAdjustment adjusts spread based on inventory position
func (sc *DynamicSpreadCalculator) calculateInventoryAdjustment(inventory *InventoryState) decimal.Decimal {
	if inventory.Position.IsZero() || sc.config.MaxInventory.IsZero() {
		return decimal.NewFromFloat(1.0)
	}
	
	// Calculate inventory ratio
	inventoryRatio := inventory.Position.Div(sc.config.MaxInventory)
	
	// Apply skew factor
	skewAdjustment := inventoryRatio.Mul(sc.config.InventorySkew)
	
	// Base adjustment
	adjustment := decimal.NewFromFloat(1.0)
	
	// If long inventory, increase ask spread (make it harder to buy more)
	// If short inventory, increase bid spread (make it harder to sell more)
	if inventory.Position.IsPositive() {
		// Long position - widen ask spread
		adjustment = adjustment.Add(skewAdjustment.Abs())
	} else {
		// Short position - widen bid spread
		adjustment = adjustment.Add(skewAdjustment.Abs())
	}
	
	return adjustment
}

// calculateDepthAdjustment adjusts spread based on order book depth
func (sc *DynamicSpreadCalculator) calculateDepthAdjustment(depth decimal.Decimal) decimal.Decimal {
	// Thinner book = wider spread
	if depth.IsZero() {
		return decimal.NewFromFloat(1.5)
	}
	
	// Define reference depth (e.g., $100k)
	referenceDepth := decimal.NewFromInt(100000)
	
	// Calculate depth ratio
	depthRatio := depth.Div(referenceDepth)
	
	// Inverse relationship - less depth means higher adjustment
	adjustment := decimal.NewFromFloat(2.0).Sub(depthRatio)
	
	// Constrain between 0.8 and 1.5
	minAdjustment := decimal.NewFromFloat(0.8)
	maxAdjustment := decimal.NewFromFloat(1.5)
	
	if adjustment.LessThan(minAdjustment) {
		adjustment = minAdjustment
	} else if adjustment.GreaterThan(maxAdjustment) {
		adjustment = maxAdjustment
	}
	
	return adjustment
}

// calculateFlowAdjustment adjusts spread based on recent trade flow
func (sc *DynamicSpreadCalculator) calculateFlowAdjustment() decimal.Decimal {
	if len(sc.tradeHistory) == 0 {
		return decimal.NewFromFloat(1.0)
	}
	
	// Get recent trade flow (last 5 minutes)
	recentFlow := sc.getRecentTradeFlow(5 * time.Minute)
	if recentFlow == nil {
		return decimal.NewFromFloat(1.0)
	}
	
	// Calculate flow imbalance
	totalVolume := recentFlow.BuyVolume.Add(recentFlow.SellVolume)
	if totalVolume.IsZero() {
		return decimal.NewFromFloat(1.0)
	}
	
	buyRatio := recentFlow.BuyVolume.Div(totalVolume)
	
	// Adjust spread based on flow imbalance
	// More buying = widen spread, More selling = tighten spread
	adjustment := decimal.NewFromFloat(1.0)
	
	if buyRatio.GreaterThan(decimal.NewFromFloat(0.6)) {
		// Heavy buying - widen spread
		adjustment = adjustment.Add(buyRatio.Sub(decimal.NewFromFloat(0.5)).Mul(decimal.NewFromFloat(0.5)))
	} else if buyRatio.LessThan(decimal.NewFromFloat(0.4)) {
		// Heavy selling - tighten spread slightly
		adjustment = adjustment.Sub(decimal.NewFromFloat(0.5).Sub(buyRatio).Mul(decimal.NewFromFloat(0.2)))
	}
	
	return adjustment
}

// UpdatePriceHistory updates the price history for volatility calculation
func (sc *DynamicSpreadCalculator) UpdatePriceHistory(price decimal.Decimal) {
	sc.priceHistory = append(sc.priceHistory, price)
	
	// Keep only recent history (e.g., last 1000 prices)
	if len(sc.priceHistory) > 1000 {
		sc.priceHistory = sc.priceHistory[len(sc.priceHistory)-1000:]
	}
	
	sc.lastUpdate = time.Now()
}

// UpdateTradeFlow updates the trade flow history
func (sc *DynamicSpreadCalculator) UpdateTradeFlow(flow *TradeFlow) {
	sc.tradeHistory = append(sc.tradeHistory, flow)
	
	// Keep only recent history
	cutoff := time.Now().Add(-30 * time.Minute)
	filtered := make([]*TradeFlow, 0)
	
	for _, f := range sc.tradeHistory {
		if f.Period > 0 { // Assuming Period indicates age
			filtered = append(filtered, f)
		}
	}
	
	sc.tradeHistory = filtered
}

// CalculateVolatility calculates price volatility over a time window
func (sc *DynamicSpreadCalculator) CalculateVolatility(window time.Duration) decimal.Decimal {
	if len(sc.priceHistory) < 2 {
		return sc.config.MinVolatility
	}
	
	// Calculate returns
	returns := make([]float64, 0, len(sc.priceHistory)-1)
	
	for i := 1; i < len(sc.priceHistory); i++ {
		if sc.priceHistory[i-1].IsZero() {
			continue
		}
		
		ret := sc.priceHistory[i].Sub(sc.priceHistory[i-1]).Div(sc.priceHistory[i-1])
		returns = append(returns, ret.InexactFloat64())
	}
	
	if len(returns) == 0 {
		return sc.config.MinVolatility
	}
	
	// Calculate standard deviation
	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))
	
	variance := 0.0
	for _, r := range returns {
		variance += math.Pow(r-mean, 2)
	}
	variance /= float64(len(returns))
	
	// Annualized volatility (assuming 365 days)
	volatility := math.Sqrt(variance) * math.Sqrt(365*24*60) // Per minute data
	
	return decimal.NewFromFloat(volatility)
}

// getRecentTradeFlow gets aggregated trade flow for a time period
func (sc *DynamicSpreadCalculator) getRecentTradeFlow(period time.Duration) *TradeFlow {
	if len(sc.tradeHistory) == 0 {
		return nil
	}
	
	// Aggregate recent flows
	aggregated := &TradeFlow{
		Period: period,
	}
	
	for _, flow := range sc.tradeHistory {
		aggregated.BuyVolume = aggregated.BuyVolume.Add(flow.BuyVolume)
		aggregated.SellVolume = aggregated.SellVolume.Add(flow.SellVolume)
		aggregated.BuyCount += flow.BuyCount
		aggregated.SellCount += flow.SellCount
	}
	
	// Calculate derived metrics
	aggregated.NetFlow = aggregated.BuyVolume.Sub(aggregated.SellVolume)
	
	totalVolume := aggregated.BuyVolume.Add(aggregated.SellVolume)
	if !totalVolume.IsZero() {
		aggregated.VolumeRatio = aggregated.BuyVolume.Div(totalVolume)
	}
	
	totalCount := aggregated.BuyCount + aggregated.SellCount
	if totalCount > 0 {
		aggregated.AverageSize = totalVolume.Div(decimal.NewFromInt(int64(totalCount)))
	}
	
	return aggregated
}

// GetBidAskSkew calculates different spreads for bid and ask based on inventory
func (sc *DynamicSpreadCalculator) GetBidAskSkew(
	baseSpread decimal.Decimal,
	inventory *InventoryState,
) (bidSpread, askSpread decimal.Decimal) {
	
	if inventory.Position.IsZero() || sc.config.MaxInventory.IsZero() {
		return baseSpread, baseSpread
	}
	
	// Calculate inventory ratio (-1 to 1)
	inventoryRatio := inventory.Position.Div(sc.config.MaxInventory)
	
	// Apply skew
	// Positive inventory: tighten bid (buy less), widen ask (sell more)
	// Negative inventory: widen bid (buy more), tighten ask (sell less)
	skewFactor := inventoryRatio.Mul(sc.config.InventorySkew)
	
	bidAdjustment := decimal.NewFromFloat(1.0).Sub(skewFactor)
	askAdjustment := decimal.NewFromFloat(1.0).Add(skewFactor)
	
	// Apply adjustments
	bidSpread = baseSpread.Mul(bidAdjustment)
	askSpread = baseSpread.Mul(askAdjustment)
	
	// Ensure minimum spread
	halfMinSpread := sc.config.MinSpreadBps.Div(decimal.NewFromInt(2))
	
	if bidSpread.LessThan(halfMinSpread) {
		bidSpread = halfMinSpread
	}
	if askSpread.LessThan(halfMinSpread) {
		askSpread = halfMinSpread
	}
	
	return bidSpread, askSpread
}