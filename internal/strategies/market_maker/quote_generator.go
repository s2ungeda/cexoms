package marketmaker

import (
	"fmt"
	"time"

	"github.com/mExOms/oms/pkg/types"
	"github.com/shopspring/decimal"
)

// QuoteGeneratorImpl generates quotes based on market conditions
type QuoteGeneratorImpl struct {
	config           *MarketMakerConfig
	spreadCalculator SpreadCalculator
	inventoryManager InventoryManager
	
	// Quote tracking
	currentQuotes    []*Quote
	lastQuoteTime    time.Time
	quoteIDCounter   int64
}

// NewQuoteGenerator creates a new quote generator
func NewQuoteGenerator(
	config *MarketMakerConfig,
	spreadCalc SpreadCalculator,
	invManager InventoryManager,
) *QuoteGeneratorImpl {
	return &QuoteGeneratorImpl{
		config:           config,
		spreadCalculator: spreadCalc,
		inventoryManager: invManager,
		currentQuotes:    make([]*Quote, 0),
		lastQuoteTime:    time.Now(),
	}
}

// GenerateQuotes generates new quotes based on market state
func (qg *QuoteGeneratorImpl) GenerateQuotes(
	market *MarketState,
	inventory *InventoryState,
) ([]*Quote, error) {
	
	// Check if we should generate new quotes
	if time.Since(qg.lastQuoteTime) < qg.config.RefreshRate {
		return qg.currentQuotes, nil
	}
	
	// Calculate base spread
	baseSpread := qg.spreadCalculator.CalculateSpread(market, inventory)
	
	// Get bid/ask specific spreads based on inventory
	bidSpread, askSpread := qg.getBidAskSpreads(baseSpread, inventory)
	
	// Calculate mid price with inventory adjustment
	adjustedMid := qg.calculateAdjustedMidPrice(market.MidPrice, inventory)
	
	// Generate quote levels
	quotes := make([]*Quote, 0, qg.config.QuoteLevels*2)
	
	// Generate bid quotes
	bidQuotes := qg.generateBidQuotes(adjustedMid, bidSpread, market, inventory)
	quotes = append(quotes, bidQuotes...)
	
	// Generate ask quotes
	askQuotes := qg.generateAskQuotes(adjustedMid, askSpread, market, inventory)
	quotes = append(quotes, askQuotes...)
	
	// Update state
	qg.currentQuotes = quotes
	qg.lastQuoteTime = time.Now()
	
	return quotes, nil
}

// generateBidQuotes generates bid side quotes
func (qg *QuoteGeneratorImpl) generateBidQuotes(
	midPrice decimal.Decimal,
	spread decimal.Decimal,
	market *MarketState,
	inventory *InventoryState,
) []*Quote {
	
	quotes := make([]*Quote, 0, qg.config.QuoteLevels)
	
	// Get maximum buy size based on inventory
	maxBuySize := qg.inventoryManager.GetPositionLimit(types.OrderSideBuy)
	if maxBuySize.LessThanOrEqual(decimal.Zero) {
		return quotes // No room to buy
	}
	
	// Distribute size across levels
	sizePerLevel := qg.calculateSizePerLevel(types.OrderSideBuy, maxBuySize)
	
	// Generate each level
	for i := 0; i < qg.config.QuoteLevels; i++ {
		// Calculate price for this level
		levelSpread := spread.Add(
			qg.config.LevelSpacing.Mul(decimal.NewFromInt(int64(i))),
		)
		
		// Convert spread from bps to decimal
		spreadDecimal := levelSpread.Div(decimal.NewFromInt(10000))
		
		// Calculate bid price
		bidPrice := midPrice.Mul(decimal.NewFromFloat(1).Sub(spreadDecimal))
		
		// Round price to exchange tick size
		bidPrice = qg.roundPrice(bidPrice)
		
		// Check if price is competitive
		if market.BidPrice.GreaterThan(bidPrice) && i == 0 {
			// Our best bid is worse than market, adjust
			bidPrice = market.BidPrice.Add(qg.getMinTickSize())
		}
		
		// Create quote
		quote := &Quote{
			Side:      types.OrderSideBuy,
			Price:     bidPrice,
			Quantity:  sizePerLevel,
			PlacedAt:  time.Now(),
			OrderID:   qg.generateOrderID("bid", i),
		}
		
		quotes = append(quotes, quote)
	}
	
	return quotes
}

// generateAskQuotes generates ask side quotes
func (qg *QuoteGeneratorImpl) generateAskQuotes(
	midPrice decimal.Decimal,
	spread decimal.Decimal,
	market *MarketState,
	inventory *InventoryState,
) []*Quote {
	
	quotes := make([]*Quote, 0, qg.config.QuoteLevels)
	
	// Get maximum sell size based on inventory
	maxSellSize := qg.inventoryManager.GetPositionLimit(types.OrderSideSell)
	if maxSellSize.LessThanOrEqual(decimal.Zero) {
		return quotes // No inventory to sell
	}
	
	// Distribute size across levels
	sizePerLevel := qg.calculateSizePerLevel(types.OrderSideSell, maxSellSize)
	
	// Generate each level
	for i := 0; i < qg.config.QuoteLevels; i++ {
		// Calculate price for this level
		levelSpread := spread.Add(
			qg.config.LevelSpacing.Mul(decimal.NewFromInt(int64(i))),
		)
		
		// Convert spread from bps to decimal
		spreadDecimal := levelSpread.Div(decimal.NewFromInt(10000))
		
		// Calculate ask price
		askPrice := midPrice.Mul(decimal.NewFromFloat(1).Add(spreadDecimal))
		
		// Round price to exchange tick size
		askPrice = qg.roundPrice(askPrice)
		
		// Check if price is competitive
		if market.AskPrice.LessThan(askPrice) && i == 0 {
			// Our best ask is worse than market, adjust
			askPrice = market.AskPrice.Sub(qg.getMinTickSize())
		}
		
		// Create quote
		quote := &Quote{
			Side:      types.OrderSideSell,
			Price:     askPrice,
			Quantity:  sizePerLevel,
			PlacedAt:  time.Now(),
			OrderID:   qg.generateOrderID("ask", i),
		}
		
		quotes = append(quotes, quote)
	}
	
	return quotes
}

// UpdateQuotes updates existing quotes based on new market conditions
func (qg *QuoteGeneratorImpl) UpdateQuotes(
	quotes []*Quote,
	market *MarketState,
) ([]*Quote, error) {
	
	// Check if market moved significantly
	if !qg.shouldUpdateQuotes(quotes, market) {
		return quotes, nil
	}
	
	// Get current inventory state
	inventory := qg.inventoryManager.(*InventoryManagerImpl).GetInventoryState()
	
	// Generate fresh quotes
	return qg.GenerateQuotes(market, inventory)
}

// calculateAdjustedMidPrice adjusts mid price based on inventory
func (qg *QuoteGeneratorImpl) calculateAdjustedMidPrice(
	marketMid decimal.Decimal,
	inventory *InventoryState,
) decimal.Decimal {
	
	// Get skew adjustment from inventory manager
	skewAdjustment := qg.inventoryManager.GetSkewAdjustment()
	
	// Apply adjustment as a percentage of mid price
	adjustmentAmount := marketMid.Mul(skewAdjustment).Div(decimal.NewFromInt(100))
	
	return marketMid.Add(adjustmentAmount)
}

// getBidAskSpreads calculates separate bid and ask spreads
func (qg *QuoteGeneratorImpl) getBidAskSpreads(
	baseSpread decimal.Decimal,
	inventory *InventoryState,
) (bidSpread, askSpread decimal.Decimal) {
	
	if calc, ok := qg.spreadCalculator.(*DynamicSpreadCalculator); ok {
		return calc.GetBidAskSkew(baseSpread, inventory)
	}
	
	// Default to symmetric spread
	return baseSpread, baseSpread
}

// calculateSizePerLevel calculates quote size for each price level
func (qg *QuoteGeneratorImpl) calculateSizePerLevel(
	side types.OrderSide,
	maxSize decimal.Decimal,
) decimal.Decimal {
	
	// Start with configured quote size
	baseSize := qg.config.QuoteSize
	
	// Check against maximum allowed size
	totalSize := baseSize.Mul(decimal.NewFromInt(int64(qg.config.QuoteLevels)))
	
	if totalSize.GreaterThan(maxSize) {
		// Scale down proportionally
		baseSize = maxSize.Div(decimal.NewFromInt(int64(qg.config.QuoteLevels)))
	}
	
	// Apply level-based sizing (optional)
	// Could implement progressive sizing where outer levels have larger sizes
	
	return baseSize
}

// shouldUpdateQuotes checks if quotes need updating
func (qg *QuoteGeneratorImpl) shouldUpdateQuotes(quotes []*Quote, market *MarketState) bool {
	if len(quotes) == 0 {
		return true
	}
	
	// Find best bid and ask from our quotes
	var bestBid, bestAsk *Quote
	
	for _, quote := range quotes {
		if quote.Side == types.OrderSideBuy {
			if bestBid == nil || quote.Price.GreaterThan(bestBid.Price) {
				bestBid = quote
			}
		} else {
			if bestAsk == nil || quote.Price.LessThan(bestAsk.Price) {
				bestAsk = quote
			}
		}
	}
	
	// Check if we're still competitive
	tolerance := decimal.NewFromFloat(0.0001) // 0.01%
	
	if bestBid != nil {
		priceDiff := market.BidPrice.Sub(bestBid.Price).Abs()
		if priceDiff.Div(market.BidPrice).GreaterThan(tolerance) {
			return true
		}
	}
	
	if bestAsk != nil {
		priceDiff := market.AskPrice.Sub(bestAsk.Price).Abs()
		if priceDiff.Div(market.AskPrice).GreaterThan(tolerance) {
			return true
		}
	}
	
	// Check if refresh interval passed
	if time.Since(qg.lastQuoteTime) >= qg.config.RefreshRate {
		return true
	}
	
	return false
}

// roundPrice rounds price to exchange tick size
func (qg *QuoteGeneratorImpl) roundPrice(price decimal.Decimal) decimal.Decimal {
	// This would be exchange-specific
	// For now, round to 2 decimal places
	return price.Round(2)
}

// getMinTickSize returns minimum price increment
func (qg *QuoteGeneratorImpl) getMinTickSize() decimal.Decimal {
	// This would be exchange and symbol specific
	// For now, return 0.01
	return decimal.NewFromFloat(0.01)
}

// generateOrderID generates a unique order ID
func (qg *QuoteGeneratorImpl) generateOrderID(side string, level int) string {
	qg.quoteIDCounter++
	return fmt.Sprintf("mm_%s_%s_L%d_%d_%d",
		qg.config.Symbol,
		side,
		level,
		time.Now().Unix(),
		qg.quoteIDCounter,
	)
}

// GetQuoteMetrics returns metrics about current quotes
func (qg *QuoteGeneratorImpl) GetQuoteMetrics() map[string]interface{} {
	bidCount := 0
	askCount := 0
	totalBidSize := decimal.Zero
	totalAskSize := decimal.Zero
	
	var bestBid, bestAsk decimal.Decimal
	
	for _, quote := range qg.currentQuotes {
		if quote.Side == types.OrderSideBuy {
			bidCount++
			totalBidSize = totalBidSize.Add(quote.Quantity)
			if bestBid.IsZero() || quote.Price.GreaterThan(bestBid) {
				bestBid = quote.Price
			}
		} else {
			askCount++
			totalAskSize = totalAskSize.Add(quote.Quantity)
			if bestAsk.IsZero() || quote.Price.LessThan(bestAsk) {
				bestAsk = quote.Price
			}
		}
	}
	
	spread := decimal.Zero
	if !bestBid.IsZero() && !bestAsk.IsZero() {
		spread = bestAsk.Sub(bestBid)
	}
	
	return map[string]interface{}{
		"bid_count":      bidCount,
		"ask_count":      askCount,
		"total_bid_size": totalBidSize,
		"total_ask_size": totalAskSize,
		"best_bid":       bestBid,
		"best_ask":       bestAsk,
		"spread":         spread,
		"last_update":    qg.lastQuoteTime,
	}
}