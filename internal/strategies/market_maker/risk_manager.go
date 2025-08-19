package marketmaker

import (
	"fmt"
	"sync"
	"time"

	"github.com/mExOms/oms/pkg/types"
	"github.com/shopspring/decimal"
)

// RiskManagerImpl implements risk management for market making
type RiskManagerImpl struct {
	mu sync.RWMutex
	
	config          *MarketMakerConfig
	inventoryMgr    InventoryManager
	
	// Risk tracking
	dailyLoss       decimal.Decimal
	maxDrawdown     decimal.Decimal
	consecutiveLoss int
	lastResetTime   time.Time
	
	// Circuit breakers
	stopped         bool
	stopReason      string
	
	// Loss tracking
	lossHistory     []decimal.Decimal
	highWaterMark   decimal.Decimal
}

// NewRiskManager creates a new risk manager
func NewRiskManager(config *MarketMakerConfig, invMgr InventoryManager) *RiskManagerImpl {
	return &RiskManagerImpl{
		config:        config,
		inventoryMgr:  invMgr,
		dailyLoss:     decimal.Zero,
		maxDrawdown:   decimal.Zero,
		lossHistory:   make([]decimal.Decimal, 0),
		lastResetTime: time.Now(),
	}
}

// CheckOrderRisk checks if an order passes risk checks
func (rm *RiskManagerImpl) CheckOrderRisk(order *types.Order) error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	// Check if stopped
	if rm.stopped {
		return fmt.Errorf("risk manager stopped: %s", rm.stopReason)
	}
	
	// Check position limits
	inventory := rm.inventoryMgr.(*InventoryManagerImpl).GetInventoryState()
	
	// Calculate position after order
	positionDelta := order.Quantity
	if order.Side == types.OrderSideSell {
		positionDelta = positionDelta.Neg()
	}
	
	newPosition := inventory.Position.Add(positionDelta)
	
	// Check max inventory
	if newPosition.Abs().GreaterThan(rm.config.MaxInventory) {
		return fmt.Errorf("order would exceed max inventory: %v > %v", 
			newPosition.Abs(), rm.config.MaxInventory)
	}
	
	// Check position value
	positionValue := newPosition.Abs().Mul(order.Price)
	if positionValue.GreaterThan(rm.config.MaxPositionValue) {
		return fmt.Errorf("order would exceed max position value: %v > %v",
			positionValue, rm.config.MaxPositionValue)
	}
	
	// Check daily loss limit
	if rm.dailyLoss.Abs().GreaterThan(rm.config.MaxDailyLoss) {
		return fmt.Errorf("daily loss limit exceeded: %v > %v",
			rm.dailyLoss.Abs(), rm.config.MaxDailyLoss)
	}
	
	// Check order size
	maxSize := rm.inventoryMgr.(*InventoryManagerImpl).GetMaxOrderSize(order.Side, order.Price)
	if order.Quantity.GreaterThan(maxSize) {
		return fmt.Errorf("order size exceeds limit: %v > %v", order.Quantity, maxSize)
	}
	
	return nil
}

// CheckPositionRisk checks if current position is within risk limits
func (rm *RiskManagerImpl) CheckPositionRisk(position decimal.Decimal) error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	if rm.stopped {
		return fmt.Errorf("risk manager stopped: %s", rm.stopReason)
	}
	
	// Check absolute position limit
	if position.Abs().GreaterThan(rm.config.MaxInventory) {
		return fmt.Errorf("position exceeds max inventory: %v > %v",
			position.Abs(), rm.config.MaxInventory)
	}
	
	return nil
}

// GetMaxOrderSize returns maximum allowed order size
func (rm *RiskManagerImpl) GetMaxOrderSize(side types.OrderSide) decimal.Decimal {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	if rm.stopped {
		return decimal.Zero
	}
	
	// Get base limit from inventory manager
	inventory := rm.inventoryMgr.(*InventoryManagerImpl).GetInventoryState()
	baseLimit := rm.inventoryMgr.GetPositionLimit(side)
	
	// Apply additional risk-based scaling
	riskScale := rm.calculateRiskScale()
	
	return baseLimit.Mul(riskScale)
}

// ShouldStop checks if trading should be stopped
func (rm *RiskManagerImpl) ShouldStop() bool {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	// Reset daily metrics if needed
	rm.checkDailyReset()
	
	// Already stopped
	if rm.stopped {
		return true
	}
	
	// Check stop conditions
	inventory := rm.inventoryMgr.(*InventoryManagerImpl).GetInventoryState()
	
	// Daily loss limit
	currentLoss := inventory.RealizedPnL.Add(inventory.UnrealizedPnL)
	if currentLoss.LessThan(rm.config.MaxDailyLoss.Neg()) {
		rm.stopped = true
		rm.stopReason = fmt.Sprintf("Daily loss limit exceeded: %v", currentLoss)
		return true
	}
	
	// Stop loss on position
	if !inventory.UnrealizedPnL.IsZero() && !inventory.PositionValue.IsZero() {
		lossPercent := inventory.UnrealizedPnL.Div(inventory.PositionValue).Abs()
		if lossPercent.GreaterThan(rm.config.StopLossPercent) {
			rm.stopped = true
			rm.stopReason = fmt.Sprintf("Stop loss triggered: %v%%", lossPercent.Mul(decimal.NewFromInt(100)))
			return true
		}
	}
	
	// Consecutive losses (if tracking)
	if rm.consecutiveLoss > 5 {
		rm.stopped = true
		rm.stopReason = fmt.Sprintf("Too many consecutive losses: %d", rm.consecutiveLoss)
		return true
	}
	
	return false
}

// UpdatePnL updates profit/loss tracking
func (rm *RiskManagerImpl) UpdatePnL(pnl decimal.Decimal) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	// Update daily loss
	rm.dailyLoss = rm.dailyLoss.Add(pnl)
	
	// Track consecutive losses
	if pnl.LessThan(decimal.Zero) {
		rm.consecutiveLoss++
	} else if pnl.GreaterThan(decimal.Zero) {
		rm.consecutiveLoss = 0
	}
	
	// Update high water mark and drawdown
	totalPnL := rm.calculateTotalPnL()
	if totalPnL.GreaterThan(rm.highWaterMark) {
		rm.highWaterMark = totalPnL
	} else {
		drawdown := rm.highWaterMark.Sub(totalPnL)
		if drawdown.GreaterThan(rm.maxDrawdown) {
			rm.maxDrawdown = drawdown
		}
	}
	
	// Add to history
	rm.lossHistory = append(rm.lossHistory, pnl)
	
	// Keep only recent history (last 100 trades)
	if len(rm.lossHistory) > 100 {
		rm.lossHistory = rm.lossHistory[len(rm.lossHistory)-100:]
	}
}

// calculateRiskScale calculates risk-based position scaling
func (rm *RiskManagerImpl) calculateRiskScale() decimal.Decimal {
	// Start with full size
	scale := decimal.NewFromFloat(1.0)
	
	// Reduce size based on daily loss
	if !rm.config.MaxDailyLoss.IsZero() {
		lossRatio := rm.dailyLoss.Abs().Div(rm.config.MaxDailyLoss)
		if lossRatio.GreaterThan(decimal.NewFromFloat(0.5)) {
			// Start reducing size after 50% of daily limit
			scale = scale.Sub(lossRatio.Sub(decimal.NewFromFloat(0.5)))
		}
	}
	
	// Reduce size based on consecutive losses
	if rm.consecutiveLoss > 2 {
		scale = scale.Mul(decimal.NewFromFloat(0.8))
	}
	if rm.consecutiveLoss > 4 {
		scale = scale.Mul(decimal.NewFromFloat(0.5))
	}
	
	// Ensure scale is positive
	if scale.LessThan(decimal.NewFromFloat(0.1)) {
		scale = decimal.NewFromFloat(0.1)
	}
	
	return scale
}

// calculateTotalPnL calculates total PnL from history
func (rm *RiskManagerImpl) calculateTotalPnL() decimal.Decimal {
	total := decimal.Zero
	for _, pnl := range rm.lossHistory {
		total = total.Add(pnl)
	}
	return total
}

// checkDailyReset checks if daily metrics should be reset
func (rm *RiskManagerImpl) checkDailyReset() {
	now := time.Now()
	
	// Reset at midnight UTC
	if now.Day() != rm.lastResetTime.Day() {
		rm.dailyLoss = decimal.Zero
		rm.consecutiveLoss = 0
		rm.stopped = false
		rm.stopReason = ""
		rm.lastResetTime = now
	}
}

// GetRiskMetrics returns current risk metrics
func (rm *RiskManagerImpl) GetRiskMetrics() map[string]interface{} {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	inventory := rm.inventoryMgr.(*InventoryManagerImpl).GetInventoryState()
	
	// Calculate risk utilization
	positionUtil := decimal.Zero
	if !rm.config.MaxInventory.IsZero() {
		positionUtil = inventory.Position.Abs().Div(rm.config.MaxInventory)
	}
	
	valueUtil := decimal.Zero
	if !rm.config.MaxPositionValue.IsZero() {
		valueUtil = inventory.PositionValue.Div(rm.config.MaxPositionValue)
	}
	
	lossUtil := decimal.Zero
	if !rm.config.MaxDailyLoss.IsZero() {
		lossUtil = rm.dailyLoss.Abs().Div(rm.config.MaxDailyLoss)
	}
	
	// Calculate Sharpe ratio (simplified)
	sharpe := rm.calculateSharpe()
	
	return map[string]interface{}{
		"stopped":           rm.stopped,
		"stop_reason":       rm.stopReason,
		"daily_loss":        rm.dailyLoss,
		"max_drawdown":      rm.maxDrawdown,
		"consecutive_loss":  rm.consecutiveLoss,
		"position_util":     positionUtil,
		"value_util":        valueUtil,
		"loss_util":         lossUtil,
		"risk_scale":        rm.calculateRiskScale(),
		"sharpe_ratio":      sharpe,
		"high_water_mark":   rm.highWaterMark,
	}
}

// calculateSharpe calculates simplified Sharpe ratio
func (rm *RiskManagerImpl) calculateSharpe() decimal.Decimal {
	if len(rm.lossHistory) < 2 {
		return decimal.Zero
	}
	
	// Calculate average return
	sum := decimal.Zero
	for _, pnl := range rm.lossHistory {
		sum = sum.Add(pnl)
	}
	avg := sum.Div(decimal.NewFromInt(int64(len(rm.lossHistory))))
	
	// Calculate standard deviation
	variance := decimal.Zero
	for _, pnl := range rm.lossHistory {
		diff := pnl.Sub(avg)
		variance = variance.Add(diff.Mul(diff))
	}
	
	variance = variance.Div(decimal.NewFromInt(int64(len(rm.lossHistory))))
	
	// Avoid division by zero
	if variance.IsZero() {
		return decimal.Zero
	}
	
	// Simplified Sharpe (not annualized)
	stdDev := decimal.NewFromFloat(variance.InexactFloat64()).Sqrt()
	
	if stdDev.IsZero() {
		return decimal.Zero
	}
	
	return avg.Div(stdDev)
}

// Reset resets risk manager state
func (rm *RiskManagerImpl) Reset() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	rm.dailyLoss = decimal.Zero
	rm.maxDrawdown = decimal.Zero
	rm.consecutiveLoss = 0
	rm.stopped = false
	rm.stopReason = ""
	rm.lossHistory = make([]decimal.Decimal, 0)
	rm.highWaterMark = decimal.Zero
	rm.lastResetTime = time.Now()
}

// ForceStop forces the risk manager to stop trading
func (rm *RiskManagerImpl) ForceStop(reason string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	rm.stopped = true
	rm.stopReason = reason
}

// Resume resumes trading after a stop
func (rm *RiskManagerImpl) Resume() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	rm.stopped = false
	rm.stopReason = ""
}