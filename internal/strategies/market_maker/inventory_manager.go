package marketmaker

import (
	"sync"
	"time"

	"github.com/mExOms/oms/pkg/types"
	"github.com/shopspring/decimal"
)

// InventoryManagerImpl manages inventory and position tracking
type InventoryManagerImpl struct {
	mu sync.RWMutex
	
	config          *MarketMakerConfig
	currentPosition decimal.Decimal
	averagePrice    decimal.Decimal
	realizedPnL     decimal.Decimal
	
	// Trade history for PnL calculation
	trades          []*types.Trade
	
	// Position limits
	maxLongPosition  decimal.Decimal
	maxShortPosition decimal.Decimal
	
	// Statistics
	totalBuyVolume   decimal.Decimal
	totalSellVolume  decimal.Decimal
	numBuys          int
	numSells         int
	
	lastUpdate      time.Time
}

// NewInventoryManager creates a new inventory manager
func NewInventoryManager(config *MarketMakerConfig) *InventoryManagerImpl {
	return &InventoryManagerImpl{
		config:           config,
		currentPosition:  decimal.Zero,
		averagePrice:     decimal.Zero,
		realizedPnL:      decimal.Zero,
		trades:           make([]*types.Trade, 0),
		maxLongPosition:  config.MaxInventory,
		maxShortPosition: config.MaxInventory.Neg(),
		lastUpdate:       time.Now(),
	}
}

// GetTargetPosition returns the target inventory position
func (im *InventoryManagerImpl) GetTargetPosition() decimal.Decimal {
	im.mu.RLock()
	defer im.mu.RUnlock()
	
	return im.config.TargetInventory
}

// GetPositionLimit returns position limit for a given side
func (im *InventoryManagerImpl) GetPositionLimit(side types.OrderSide) decimal.Decimal {
	im.mu.RLock()
	defer im.mu.RUnlock()
	
	if side == types.OrderSideBuy {
		// Buying increases position
		return im.maxLongPosition.Sub(im.currentPosition)
	} else {
		// Selling decreases position
		return im.currentPosition.Sub(im.maxShortPosition)
	}
}

// UpdatePosition updates position based on a trade
func (im *InventoryManagerImpl) UpdatePosition(trade *types.Trade) {
	im.mu.Lock()
	defer im.mu.Unlock()
	
	// Store trade
	im.trades = append(im.trades, trade)
	
	// Update statistics
	if trade.Side == types.OrderSideBuy {
		im.totalBuyVolume = im.totalBuyVolume.Add(trade.Quantity)
		im.numBuys++
	} else {
		im.totalSellVolume = im.totalSellVolume.Add(trade.Quantity)
		im.numSells++
	}
	
	// Calculate position change
	positionChange := trade.Quantity
	if trade.Side == types.OrderSideSell {
		positionChange = positionChange.Neg()
	}
	
	// Update average price and realized PnL
	if im.currentPosition.IsZero() {
		// Starting new position
		im.averagePrice = trade.Price
	} else if im.currentPosition.Sign() == positionChange.Sign() {
		// Adding to position
		totalValue := im.currentPosition.Mul(im.averagePrice).Add(trade.Quantity.Mul(trade.Price))
		newPosition := im.currentPosition.Add(positionChange)
		
		if !newPosition.IsZero() {
			im.averagePrice = totalValue.Div(newPosition.Abs())
		}
	} else {
		// Reducing position or flipping
		closedQuantity := decimal.Min(im.currentPosition.Abs(), trade.Quantity)
		
		// Calculate realized PnL on closed portion
		var pnl decimal.Decimal
		if im.currentPosition.IsPositive() {
			// Long position being reduced by sell
			pnl = closedQuantity.Mul(trade.Price.Sub(im.averagePrice))
		} else {
			// Short position being reduced by buy
			pnl = closedQuantity.Mul(im.averagePrice.Sub(trade.Price))
		}
		
		im.realizedPnL = im.realizedPnL.Add(pnl)
		
		// Update average price if flipping position
		if trade.Quantity.GreaterThan(im.currentPosition.Abs()) {
			im.averagePrice = trade.Price
		}
	}
	
	// Update position
	im.currentPosition = im.currentPosition.Add(positionChange)
	im.lastUpdate = time.Now()
}

// GetSkewAdjustment returns price adjustment based on inventory skew
func (im *InventoryManagerImpl) GetSkewAdjustment() decimal.Decimal {
	im.mu.RLock()
	defer im.mu.RUnlock()
	
	if im.currentPosition.IsZero() || im.config.MaxInventory.IsZero() {
		return decimal.Zero
	}
	
	// Calculate inventory ratio
	inventoryRatio := im.currentPosition.Div(im.config.MaxInventory)
	
	// Apply skew factor
	// Positive inventory -> negative adjustment (lower prices to sell)
	// Negative inventory -> positive adjustment (higher prices to buy)
	adjustment := inventoryRatio.Mul(im.config.InventorySkew).Neg()
	
	return adjustment
}

// GetInventoryState returns current inventory state
func (im *InventoryManagerImpl) GetInventoryState() *InventoryState {
	im.mu.RLock()
	defer im.mu.RUnlock()
	
	// Calculate position value at current market price
	// This would need market price input in real implementation
	positionValue := im.currentPosition.Mul(im.averagePrice)
	
	// Calculate unrealized PnL (would need current market price)
	unrealizedPnL := decimal.Zero
	
	return &InventoryState{
		Position:      im.currentPosition,
		PositionValue: positionValue,
		AveragePrice:  im.averagePrice,
		UnrealizedPnL: unrealizedPnL,
		RealizedPnL:   im.realizedPnL,
		TotalPnL:      im.realizedPnL.Add(unrealizedPnL),
		LastUpdate:    im.lastUpdate,
	}
}

// GetMaxOrderSize returns maximum order size based on inventory
func (im *InventoryManagerImpl) GetMaxOrderSize(side types.OrderSide, currentPrice decimal.Decimal) decimal.Decimal {
	im.mu.RLock()
	defer im.mu.RUnlock()
	
	// Get position limit
	positionLimit := im.GetPositionLimit(side)
	
	// Check value limit
	currentValue := im.currentPosition.Abs().Mul(currentPrice)
	remainingValue := im.config.MaxPositionValue.Sub(currentValue)
	
	if remainingValue.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}
	
	// Convert value limit to quantity
	valueLimit := remainingValue.Div(currentPrice)
	
	// Return minimum of position and value limits
	return decimal.Min(positionLimit, valueLimit)
}

// RebalanceToTarget calculates orders needed to rebalance to target
func (im *InventoryManagerImpl) RebalanceToTarget(currentPrice decimal.Decimal) *types.Order {
	im.mu.RLock()
	defer im.mu.RUnlock()
	
	// Calculate difference from target
	targetDiff := im.config.TargetInventory.Sub(im.currentPosition)
	
	if targetDiff.Abs().LessThan(im.config.QuoteSize.Div(decimal.NewFromInt(10))) {
		// Close enough to target
		return nil
	}
	
	// Create rebalance order
	order := &types.Order{
		Symbol:   im.config.Symbol,
		Type:     types.OrderTypeLimit,
		Quantity: targetDiff.Abs(),
		Price:    currentPrice,
	}
	
	if targetDiff.IsPositive() {
		order.Side = types.OrderSideBuy
		// Buy slightly below market
		order.Price = currentPrice.Mul(decimal.NewFromFloat(0.999))
	} else {
		order.Side = types.OrderSideSell
		// Sell slightly above market
		order.Price = currentPrice.Mul(decimal.NewFromFloat(1.001))
	}
	
	return order
}

// GetMetrics returns inventory management metrics
func (im *InventoryManagerImpl) GetMetrics() map[string]interface{} {
	im.mu.RLock()
	defer im.mu.RUnlock()
	
	volumeRatio := decimal.Zero
	if !im.totalSellVolume.IsZero() {
		volumeRatio = im.totalBuyVolume.Div(im.totalSellVolume)
	}
	
	inventoryRatio := decimal.Zero
	if !im.config.MaxInventory.IsZero() {
		inventoryRatio = im.currentPosition.Div(im.config.MaxInventory)
	}
	
	return map[string]interface{}{
		"current_position":   im.currentPosition,
		"average_price":      im.averagePrice,
		"realized_pnl":       im.realizedPnL,
		"total_buy_volume":   im.totalBuyVolume,
		"total_sell_volume":  im.totalSellVolume,
		"num_buys":           im.numBuys,
		"num_sells":          im.numSells,
		"volume_ratio":       volumeRatio,
		"inventory_ratio":    inventoryRatio,
		"position_age":       time.Since(im.lastUpdate),
	}
}

// ShouldHedge determines if position should be hedged
func (im *InventoryManagerImpl) ShouldHedge() (bool, decimal.Decimal) {
	im.mu.RLock()
	defer im.mu.RUnlock()
	
	if !im.config.EnableHedging {
		return false, decimal.Zero
	}
	
	// Check if position exceeds hedge threshold
	hedgeThreshold := im.config.MaxInventory.Mul(decimal.NewFromFloat(0.7))
	
	if im.currentPosition.Abs().GreaterThan(hedgeThreshold) {
		// Calculate hedge amount
		hedgeAmount := im.currentPosition.Mul(im.config.HedgeRatio)
		return true, hedgeAmount
	}
	
	return false, decimal.Zero
}

// Reset resets inventory manager state
func (im *InventoryManagerImpl) Reset() {
	im.mu.Lock()
	defer im.mu.Unlock()
	
	im.currentPosition = decimal.Zero
	im.averagePrice = decimal.Zero
	im.realizedPnL = decimal.Zero
	im.trades = make([]*types.Trade, 0)
	im.totalBuyVolume = decimal.Zero
	im.totalSellVolume = decimal.Zero
	im.numBuys = 0
	im.numSells = 0
	im.lastUpdate = time.Now()
}