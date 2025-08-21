package risk

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// RiskEngine implements real-time risk management with lock-free operations
type RiskEngine struct {
	// Atomic counters for performance metrics
	orderCount     atomic.Uint64
	checkCount     atomic.Uint64
	rejectedCount  atomic.Uint64
	processingTime atomic.Int64 // in nanoseconds
	
	// Risk limits (using atomic values for lock-free access)
	maxPositionSize    atomic.Value // decimal.Decimal
	maxLeverage        atomic.Uint32
	maxOrderValue      atomic.Value // decimal.Decimal
	maxDailyLoss       atomic.Value // decimal.Decimal
	maxExposure        atomic.Value // decimal.Decimal
	
	// Current state (lock-free)
	currentExposure    atomic.Value // decimal.Decimal
	dailyPnL           atomic.Value // decimal.Decimal
	
	// Position tracking
	positions     sync.Map // symbol -> *PositionRisk
	balances      sync.Map // exchange -> *types.Balance
	
	// Configuration
	enabled       atomic.Bool
	strictMode    atomic.Bool // Reject all orders if any limit is breached
}

// PositionRisk tracks risk metrics for a single position
type PositionRisk struct {
	Symbol        string
	Exchange      string
	Market        string
	Quantity      decimal.Decimal
	AvgEntryPrice decimal.Decimal
	MarkPrice     decimal.Decimal
	UnrealizedPnL decimal.Decimal
	Leverage      int
	MarginUsed    decimal.Decimal
	UpdatedAt     time.Time
}

// RiskCheckResult contains the result of a risk check
type RiskCheckResult struct {
	Passed          bool
	RejectionReason string
	CheckDuration   time.Duration
	RiskMetrics     map[string]interface{}
}

// NewRiskEngine creates a new risk management engine
func NewRiskEngine() *RiskEngine {
	re := &RiskEngine{}
	
	// Initialize default limits
	re.maxPositionSize.Store(decimal.NewFromFloat(100000))    // $100k max position
	re.maxLeverage.Store(uint32(10))                          // 10x max leverage
	re.maxOrderValue.Store(decimal.NewFromFloat(50000))       // $50k max order
	re.maxDailyLoss.Store(decimal.NewFromFloat(10000))        // $10k max daily loss
	re.maxExposure.Store(decimal.NewFromFloat(500000))        // $500k max exposure
	
	// Initialize current state
	re.currentExposure.Store(decimal.Zero)
	re.dailyPnL.Store(decimal.Zero)
	
	// Enable by default
	re.enabled.Store(true)
	re.strictMode.Store(false)
	
	return re
}

// CheckOrder performs risk checks on an order (target: < 50 microseconds)
func (re *RiskEngine) CheckOrder(ctx context.Context, order *types.Order, exchange string) (*RiskCheckResult, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start).Nanoseconds()
		re.processingTime.Store(duration)
		re.checkCount.Add(1)
	}()
	
	// Fast path: if risk engine is disabled
	if !re.enabled.Load() {
		return &RiskCheckResult{
			Passed:        true,
			CheckDuration: time.Since(start),
		}, nil
	}
	
	result := &RiskCheckResult{
		Passed:      true,
		RiskMetrics: make(map[string]interface{}),
	}
	
	// 1. Check order value limit
	orderValue := order.Price.Mul(order.Quantity)
	maxOrderValue := re.maxOrderValue.Load().(decimal.Decimal)
	if orderValue.GreaterThan(maxOrderValue) {
		result.Passed = false
		result.RejectionReason = fmt.Sprintf("order value %s exceeds limit %s", 
			orderValue.String(), maxOrderValue.String())
		re.rejectedCount.Add(1)
		result.CheckDuration = time.Since(start)
		return result, nil
	}
	
	// 2. Check position size limit
	positionKey := fmt.Sprintf("%s:%s", exchange, order.Symbol)
	var currentPosition decimal.Decimal
	
	if pos, exists := re.positions.Load(positionKey); exists {
		posRisk := pos.(*PositionRisk)
		currentPosition = posRisk.Quantity
	}
	
	newPosition := currentPosition
	if order.Side == types.OrderSideBuy {
		newPosition = newPosition.Add(order.Quantity)
	} else {
		newPosition = newPosition.Sub(order.Quantity)
	}
	
	maxPosition := re.maxPositionSize.Load().(decimal.Decimal)
	if newPosition.Abs().GreaterThan(maxPosition) {
		result.Passed = false
		result.RejectionReason = fmt.Sprintf("position size %s exceeds limit %s", 
			newPosition.Abs().String(), maxPosition.String())
		re.rejectedCount.Add(1)
		result.CheckDuration = time.Since(start)
		return result, nil
	}
	
	// 3. Check leverage limit (for futures)
	if order.PositionSide != "" { // Futures order
		// Estimate leverage based on position value and margin
		positionValue := newPosition.Abs().Mul(order.Price)
		
		// Get account balance for margin calculation
		var availableMargin decimal.Decimal
		if bal, exists := re.balances.Load(exchange); exists {
			balance := bal.(*types.Balance)
			if usdtBalance, ok := balance.Assets["USDT"]; ok {
				availableMargin = decimal.RequireFromString(usdtBalance.Free)
			}
		}
		
		if !availableMargin.IsZero() {
			estimatedLeverage := positionValue.Div(availableMargin)
			maxLeverage := decimal.NewFromInt(int64(re.maxLeverage.Load()))
			
			if estimatedLeverage.GreaterThan(maxLeverage) {
				result.Passed = false
				result.RejectionReason = fmt.Sprintf("estimated leverage %s exceeds limit %s", 
					estimatedLeverage.StringFixed(1), maxLeverage.String())
				re.rejectedCount.Add(1)
				result.CheckDuration = time.Since(start)
				return result, nil
			}
			
			result.RiskMetrics["estimated_leverage"] = estimatedLeverage.StringFixed(2)
		}
	}
	
	// 4. Check daily loss limit
	dailyPnL := re.dailyPnL.Load().(decimal.Decimal)
	maxDailyLoss := re.maxDailyLoss.Load().(decimal.Decimal)
	
	if dailyPnL.IsNegative() && dailyPnL.Abs().GreaterThan(maxDailyLoss) {
		if re.strictMode.Load() {
			result.Passed = false
			result.RejectionReason = fmt.Sprintf("daily loss %s exceeds limit %s", 
				dailyPnL.Abs().String(), maxDailyLoss.String())
			re.rejectedCount.Add(1)
			result.CheckDuration = time.Since(start)
			return result, nil
		}
		result.RiskMetrics["daily_loss_warning"] = true
	}
	
	// 5. Check total exposure limit
	currentExposure := re.currentExposure.Load().(decimal.Decimal)
	additionalExposure := orderValue
	newExposure := currentExposure.Add(additionalExposure)
	maxExposure := re.maxExposure.Load().(decimal.Decimal)
	
	if newExposure.GreaterThan(maxExposure) {
		result.Passed = false
		result.RejectionReason = fmt.Sprintf("total exposure %s exceeds limit %s", 
			newExposure.String(), maxExposure.String())
		re.rejectedCount.Add(1)
		result.CheckDuration = time.Since(start)
		return result, nil
	}
	
	// Update metrics
	result.RiskMetrics["order_value"] = orderValue.String()
	result.RiskMetrics["new_position_size"] = newPosition.String()
	result.RiskMetrics["current_exposure"] = currentExposure.String()
	result.RiskMetrics["daily_pnl"] = dailyPnL.String()
	result.CheckDuration = time.Since(start)
	
	re.orderCount.Add(1)
	return result, nil
}

// UpdatePosition updates position risk metrics
func (re *RiskEngine) UpdatePosition(exchange, symbol string, position *PositionRisk) {
	positionKey := fmt.Sprintf("%s:%s", exchange, symbol)
	re.positions.Store(positionKey, position)
	
	// Recalculate total exposure
	re.recalculateExposure()
}

// UpdateBalance updates account balance for an exchange
func (re *RiskEngine) UpdateBalance(exchange string, balance *types.Balance) {
	re.balances.Store(exchange, balance)
}

// UpdateDailyPnL updates the daily P&L
func (re *RiskEngine) UpdateDailyPnL(pnl decimal.Decimal) {
	re.dailyPnL.Store(pnl)
}

// SetMaxPositionSize sets the maximum position size limit
func (re *RiskEngine) SetMaxPositionSize(size decimal.Decimal) {
	re.maxPositionSize.Store(size)
}

// SetMaxLeverage sets the maximum leverage limit
func (re *RiskEngine) SetMaxLeverage(leverage uint32) {
	re.maxLeverage.Store(leverage)
}

// SetMaxOrderValue sets the maximum order value limit
func (re *RiskEngine) SetMaxOrderValue(value decimal.Decimal) {
	re.maxOrderValue.Store(value)
}

// SetMaxDailyLoss sets the maximum daily loss limit
func (re *RiskEngine) SetMaxDailyLoss(loss decimal.Decimal) {
	re.maxDailyLoss.Store(loss)
}

// SetMaxExposure sets the maximum total exposure limit
func (re *RiskEngine) SetMaxExposure(exposure decimal.Decimal) {
	re.maxExposure.Store(exposure)
}

// Enable enables the risk engine
func (re *RiskEngine) Enable() {
	re.enabled.Store(true)
}

// Disable disables the risk engine (dangerous!)
func (re *RiskEngine) Disable() {
	re.enabled.Store(false)
}

// SetStrictMode sets strict mode (reject all orders if any limit is breached)
func (re *RiskEngine) SetStrictMode(strict bool) {
	re.strictMode.Store(strict)
}

// GetMetrics returns current risk metrics
func (re *RiskEngine) GetMetrics() map[string]interface{} {
	avgProcessingTime := float64(0)
	if checkCount := re.checkCount.Load(); checkCount > 0 {
		avgProcessingTime = float64(re.processingTime.Load()) / float64(checkCount)
	}
	
	return map[string]interface{}{
		"enabled":              re.enabled.Load(),
		"strict_mode":          re.strictMode.Load(),
		"orders_checked":       re.orderCount.Load(),
		"checks_performed":     re.checkCount.Load(),
		"orders_rejected":      re.rejectedCount.Load(),
		"avg_check_time_ns":    avgProcessingTime,
		"avg_check_time_us":    avgProcessingTime / 1000,
		"current_exposure":     re.currentExposure.Load().(decimal.Decimal).String(),
		"daily_pnl":            re.dailyPnL.Load().(decimal.Decimal).String(),
		"max_position_size":    re.maxPositionSize.Load().(decimal.Decimal).String(),
		"max_leverage":         re.maxLeverage.Load(),
		"max_order_value":      re.maxOrderValue.Load().(decimal.Decimal).String(),
		"max_daily_loss":       re.maxDailyLoss.Load().(decimal.Decimal).String(),
		"max_exposure":         re.maxExposure.Load().(decimal.Decimal).String(),
	}
}

// Reset resets daily metrics (should be called at start of trading day)
func (re *RiskEngine) Reset() {
	re.dailyPnL.Store(decimal.Zero)
	re.orderCount.Store(0)
	re.checkCount.Store(0)
	re.rejectedCount.Store(0)
	re.processingTime.Store(0)
}

// recalculateExposure recalculates total exposure across all positions
func (re *RiskEngine) recalculateExposure() {
	totalExposure := decimal.Zero
	
	re.positions.Range(func(key, value interface{}) bool {
		position := value.(*PositionRisk)
		positionValue := position.Quantity.Abs().Mul(position.MarkPrice)
		totalExposure = totalExposure.Add(positionValue)
		return true
	})
	
	re.currentExposure.Store(totalExposure)
}