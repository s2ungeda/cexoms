package backtest

import (
	"sync"
	
	"github.com/your-org/mExOms/pkg/types"
)

// PositionManager manages positions during backtesting
type PositionManager struct {
	positions       map[string]*Position
	availableCapital float64
	initialCapital  float64
	totalCommission float64
	totalSlippage   float64
	mu              sync.RWMutex
}

// NewPositionManager creates a new position manager
func NewPositionManager(initialCapital float64) *PositionManager {
	return &PositionManager{
		positions:        make(map[string]*Position),
		availableCapital: initialCapital,
		initialCapital:   initialCapital,
	}
}

// GetPosition returns the position for a symbol
func (pm *PositionManager) GetPosition(symbol string) *Position {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	pos, exists := pm.positions[symbol]
	if !exists {
		return &Position{
			Symbol:   symbol,
			Quantity: 0,
		}
	}
	
	return &Position{
		Symbol:        pos.Symbol,
		Side:          pos.Side,
		Quantity:      pos.Quantity,
		EntryPrice:    pos.EntryPrice,
		CurrentPrice:  pos.CurrentPrice,
		RealizedPnL:   pos.RealizedPnL,
		UnrealizedPnL: pos.UnrealizedPnL,
		Commission:    pos.Commission,
	}
}

// UpdatePosition updates position based on a trade
func (pm *PositionManager) UpdatePosition(trade *Trade) float64 {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pos, exists := pm.positions[trade.Symbol]
	if !exists {
		// Create new position
		pos = &Position{
			Symbol:     trade.Symbol,
			Side:       trade.Side,
			Quantity:   0,
			EntryPrice: 0,
		}
		pm.positions[trade.Symbol] = pos
	}
	
	// Calculate PnL for closing trades
	pnl := 0.0
	
	if pos.Quantity > 0 && trade.Side == types.OrderSideSell {
		// Closing long position
		closeQty := min(pos.Quantity, trade.Quantity)
		pnl = closeQty * (trade.Price - pos.EntryPrice)
		
		// Update position
		if closeQty >= pos.Quantity {
			// Position fully closed
			pos.Quantity = 0
			pos.RealizedPnL += pnl
		} else {
			// Partially closed
			pos.Quantity -= closeQty
			pos.RealizedPnL += pnl
		}
		
		// Update available capital
		pm.availableCapital += closeQty * trade.Price - trade.Commission
		
	} else if pos.Quantity < 0 && trade.Side == types.OrderSideBuy {
		// Closing short position
		closeQty := min(-pos.Quantity, trade.Quantity)
		pnl = closeQty * (pos.EntryPrice - trade.Price)
		
		// Update position
		if closeQty >= -pos.Quantity {
			// Position fully closed
			pos.Quantity = 0
			pos.RealizedPnL += pnl
		} else {
			// Partially closed
			pos.Quantity += closeQty
			pos.RealizedPnL += pnl
		}
		
		// Update available capital (return borrowed amount)
		pm.availableCapital += pnl - trade.Commission
		
	} else {
		// Opening or adding to position
		if trade.Side == types.OrderSideBuy {
			// Opening/adding long
			if pos.Quantity >= 0 {
				// Average entry price
				totalCost := pos.Quantity * pos.EntryPrice + trade.Quantity * trade.Price
				pos.Quantity += trade.Quantity
				pos.EntryPrice = totalCost / pos.Quantity
			} else {
				// Flipping from short to long
				pos.Quantity += trade.Quantity
				if pos.Quantity > 0 {
					pos.EntryPrice = trade.Price
					pos.Side = types.OrderSideBuy
				}
			}
			
			// Update available capital
			pm.availableCapital -= trade.Quantity * trade.Price + trade.Commission
			
		} else {
			// Opening/adding short
			if pos.Quantity <= 0 {
				// Average entry price
				totalValue := -pos.Quantity * pos.EntryPrice + trade.Quantity * trade.Price
				pos.Quantity -= trade.Quantity
				pos.EntryPrice = totalValue / -pos.Quantity
			} else {
				// Flipping from long to short
				pos.Quantity -= trade.Quantity
				if pos.Quantity < 0 {
					pos.EntryPrice = trade.Price
					pos.Side = types.OrderSideSell
				}
			}
			
			// For shorts, we receive cash but need to track the liability
			pm.availableCapital += trade.Quantity * trade.Price - trade.Commission
		}
	}
	
	// Track costs
	pm.totalCommission += trade.Commission
	pm.totalSlippage += trade.Slippage
	
	// Update current price
	pos.CurrentPrice = trade.Price
	
	return pnl
}

// GetTotalEquity calculates total equity including positions
func (pm *PositionManager) GetTotalEquity(prices map[string]float64) float64 {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	equity := pm.availableCapital
	
	// Add unrealized PnL from open positions
	for symbol, pos := range pm.positions {
		if pos.Quantity == 0 {
			continue
		}
		
		currentPrice := pos.CurrentPrice
		if price, exists := prices[symbol]; exists {
			currentPrice = price
			pos.CurrentPrice = price
		}
		
		if pos.Quantity > 0 {
			// Long position
			pos.UnrealizedPnL = pos.Quantity * (currentPrice - pos.EntryPrice)
			equity += pos.Quantity * currentPrice
		} else {
			// Short position
			pos.UnrealizedPnL = -pos.Quantity * (pos.EntryPrice - currentPrice)
			// For shorts, we have the cash but owe the shares
			// Equity impact is the PnL only
			equity += pos.UnrealizedPnL
		}
	}
	
	return equity
}

// GetAvailableCapital returns available capital for trading
func (pm *PositionManager) GetAvailableCapital() float64 {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	return pm.availableCapital
}

// GetPositions returns all positions
func (pm *PositionManager) GetPositions() map[string]*Position {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	positions := make(map[string]*Position)
	for symbol, pos := range pm.positions {
		positions[symbol] = &Position{
			Symbol:        pos.Symbol,
			Side:          pos.Side,
			Quantity:      pos.Quantity,
			EntryPrice:    pos.EntryPrice,
			CurrentPrice:  pos.CurrentPrice,
			RealizedPnL:   pos.RealizedPnL,
			UnrealizedPnL: pos.UnrealizedPnL,
			Commission:    pos.Commission,
		}
	}
	
	return positions
}

// GetStatistics returns position statistics
func (pm *PositionManager) GetStatistics() map[string]float64 {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	totalRealizedPnL := 0.0
	totalUnrealizedPnL := 0.0
	openPositions := 0
	
	for _, pos := range pm.positions {
		totalRealizedPnL += pos.RealizedPnL
		if pos.Quantity != 0 {
			openPositions++
			totalUnrealizedPnL += pos.UnrealizedPnL
		}
	}
	
	return map[string]float64{
		"total_realized_pnl":   totalRealizedPnL,
		"total_unrealized_pnl": totalUnrealizedPnL,
		"total_commission":     pm.totalCommission,
		"total_slippage":       pm.totalSlippage,
		"available_capital":    pm.availableCapital,
		"open_positions":       float64(openPositions),
	}
}

// Helper function for min
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}