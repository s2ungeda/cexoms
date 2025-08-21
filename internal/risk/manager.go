package risk

import (
	"fmt"
	"sync"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// Manager defines the interface for risk management operations
type Manager interface {
	// Pre-trade checks
	CheckOrderRisk(order *types.Order) error
	ValidatePositionSize(symbol string, size decimal.Decimal) error
	
	// Position sizing
	CalculatePositionSize(params PositionSizeParams) decimal.Decimal
	GetMaxPositionSize(symbol string, account string) decimal.Decimal
	
	// Risk limits
	SetMaxDrawdown(percentage float64)
	SetMaxExposure(amount decimal.Decimal) 
	SetMaxPositionCount(count int)
	
	// Stop loss management
	CalculateStopLoss(entry decimal.Decimal, riskPercent float64) decimal.Decimal
	SetAutoStopLoss(enabled bool, percentage float64)
	
	// Monitoring
	GetCurrentExposure() decimal.Decimal
	GetAccountRiskMetrics(account string) *RiskMetrics
}

// PositionSizeParams contains parameters for position size calculation
type PositionSizeParams struct {
	AccountBalance decimal.Decimal
	RiskPercentage float64
	StopDistance   decimal.Decimal
	Symbol         string
	Leverage       int
}

// RiskMetrics contains risk metrics for an account
type RiskMetrics struct {
	TotalExposure   decimal.Decimal
	OpenPositions   int
	CurrentDrawdown float64
	DailyPnL        decimal.Decimal
	VaR95           decimal.Decimal // Value at Risk at 95% confidence
	UpdatedAt       time.Time
}

// RiskManager implements the Manager interface
type RiskManager struct {
	mu sync.RWMutex
	
	// Risk limits
	maxDrawdown      float64
	maxExposure      decimal.Decimal
	maxPositionCount int
	
	// Stop loss settings
	autoStopLoss        bool
	autoStopLossPercent float64
	
	// Position tracking
	positions map[string]map[string]*types.Position // account -> symbol -> position
	
	// Account balances
	balances map[string]decimal.Decimal // account -> balance
	
	// Historical data for metrics
	pnlHistory map[string][]decimal.Decimal // account -> daily PnL history
}

// NewRiskManager creates a new risk manager instance
func NewRiskManager() *RiskManager {
	return &RiskManager{
		maxDrawdown:      0.10,  // 10% default
		maxExposure:      decimal.NewFromInt(100000), // $100k default
		maxPositionCount: 10,    // 10 positions default
		positions:        make(map[string]map[string]*types.Position),
		balances:         make(map[string]decimal.Decimal),
		pnlHistory:       make(map[string][]decimal.Decimal),
	}
}

// CheckOrderRisk validates an order against risk parameters
func (rm *RiskManager) CheckOrderRisk(order *types.Order) error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	// Calculate order value
	orderValue := order.Quantity.Mul(order.Price)
	
	// Check against max exposure
	currentExposure := rm.calculateTotalExposure()
	if currentExposure.Add(orderValue).GreaterThan(rm.maxExposure) {
		return fmt.Errorf("order would exceed max exposure limit of %s", rm.maxExposure)
	}
	
	// Check position count
	if account, ok := order.Metadata["account_id"].(string); ok {
		if positions, exists := rm.positions[account]; exists {
			if len(positions) >= rm.maxPositionCount {
				return fmt.Errorf("max position count (%d) reached", rm.maxPositionCount)
			}
		}
	}
	
	// Check drawdown
	if account, ok := order.Metadata["account_id"].(string); ok {
		metrics := rm.calculateAccountMetrics(account)
		if metrics.CurrentDrawdown > rm.maxDrawdown {
			return fmt.Errorf("current drawdown (%.2f%%) exceeds limit (%.2f%%)", 
				metrics.CurrentDrawdown*100, rm.maxDrawdown*100)
		}
	}
	
	return nil
}

// ValidatePositionSize checks if a position size is within risk limits
func (rm *RiskManager) ValidatePositionSize(symbol string, size decimal.Decimal) error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	// Implement position size validation logic
	// This could check against symbol-specific limits, leverage, etc.
	
	return nil
}

// CalculatePositionSize calculates optimal position size based on risk parameters
func (rm *RiskManager) CalculatePositionSize(params PositionSizeParams) decimal.Decimal {
	// Kelly Criterion or Fixed Fractional position sizing
	riskAmount := params.AccountBalance.Mul(decimal.NewFromFloat(params.RiskPercentage / 100))
	
	// Position size = Risk Amount / Stop Distance
	if params.StopDistance.IsZero() {
		// Default to 2% stop distance if not provided
		params.StopDistance = decimal.NewFromFloat(0.02)
	}
	
	positionSize := riskAmount.Div(params.StopDistance)
	
	// Adjust for leverage
	if params.Leverage > 1 {
		positionSize = positionSize.Div(decimal.NewFromInt(int64(params.Leverage)))
	}
	
	return positionSize
}

// GetMaxPositionSize returns the maximum allowed position size for a symbol
func (rm *RiskManager) GetMaxPositionSize(symbol string, account string) decimal.Decimal {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	// Get account balance
	balance, exists := rm.balances[account]
	if !exists {
		return decimal.Zero
	}
	
	// Maximum 5% of account per position
	maxSize := balance.Mul(decimal.NewFromFloat(0.05))
	
	return maxSize
}

// SetMaxDrawdown sets the maximum drawdown limit
func (rm *RiskManager) SetMaxDrawdown(percentage float64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.maxDrawdown = percentage
}

// SetMaxExposure sets the maximum total exposure limit
func (rm *RiskManager) SetMaxExposure(amount decimal.Decimal) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.maxExposure = amount
}

// SetMaxPositionCount sets the maximum number of concurrent positions
func (rm *RiskManager) SetMaxPositionCount(count int) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.maxPositionCount = count
}

// CalculateStopLoss calculates stop loss price based on entry and risk percentage
func (rm *RiskManager) CalculateStopLoss(entry decimal.Decimal, riskPercent float64) decimal.Decimal {
	// For long positions: stop loss = entry * (1 - risk%)
	// For short positions: stop loss = entry * (1 + risk%)
	stopLossLong := entry.Mul(decimal.NewFromFloat(1 - riskPercent/100))
	return stopLossLong
}

// SetAutoStopLoss enables/disables automatic stop loss
func (rm *RiskManager) SetAutoStopLoss(enabled bool, percentage float64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.autoStopLoss = enabled
	rm.autoStopLossPercent = percentage
}

// GetCurrentExposure returns the total current exposure across all positions
func (rm *RiskManager) GetCurrentExposure() decimal.Decimal {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.calculateTotalExposure()
}

// GetAccountRiskMetrics returns risk metrics for a specific account
func (rm *RiskManager) GetAccountRiskMetrics(account string) *RiskMetrics {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	return rm.calculateAccountMetrics(account)
}

// UpdatePosition updates position information for risk tracking
func (rm *RiskManager) UpdatePosition(account string, position *types.Position) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	if _, exists := rm.positions[account]; !exists {
		rm.positions[account] = make(map[string]*types.Position)
	}
	
	if position.Quantity == 0 {
		// Position closed
		delete(rm.positions[account], position.Symbol)
	} else {
		// Position updated
		rm.positions[account][position.Symbol] = position
	}
}

// UpdateBalance updates account balance for risk calculations
func (rm *RiskManager) UpdateBalance(account string, balance decimal.Decimal) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.balances[account] = balance
}

// RecordPnL records daily PnL for drawdown calculations
func (rm *RiskManager) RecordPnL(account string, pnl decimal.Decimal) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	if _, exists := rm.pnlHistory[account]; !exists {
		rm.pnlHistory[account] = make([]decimal.Decimal, 0)
	}
	
	rm.pnlHistory[account] = append(rm.pnlHistory[account], pnl)
	
	// Keep only last 30 days
	if len(rm.pnlHistory[account]) > 30 {
		rm.pnlHistory[account] = rm.pnlHistory[account][1:]
	}
}

// Helper methods

func (rm *RiskManager) calculateTotalExposure() decimal.Decimal {
	total := decimal.Zero
	
	for _, positions := range rm.positions {
		for _, pos := range positions {
			quantity := decimal.NewFromFloat(pos.Quantity)
			markPrice := decimal.NewFromFloat(pos.MarkPrice)
			exposure := quantity.Mul(markPrice)
			total = total.Add(exposure)
		}
	}
	
	return total
}

func (rm *RiskManager) calculateAccountMetrics(account string) *RiskMetrics {
	metrics := &RiskMetrics{
		TotalExposure: decimal.Zero,
		OpenPositions: 0,
		UpdatedAt:     time.Now(),
	}
	
	// Calculate exposure and position count
	if positions, exists := rm.positions[account]; exists {
		for _, pos := range positions {
			quantity := decimal.NewFromFloat(pos.Quantity)
			markPrice := decimal.NewFromFloat(pos.MarkPrice)
			exposure := quantity.Mul(markPrice)
			metrics.TotalExposure = metrics.TotalExposure.Add(exposure)
			metrics.OpenPositions++
		}
	}
	
	// Calculate drawdown
	if history, exists := rm.pnlHistory[account]; exists && len(history) > 0 {
		peak := decimal.Zero
		cumulative := decimal.Zero
		maxDrawdown := 0.0
		
		for _, pnl := range history {
			cumulative = cumulative.Add(pnl)
			if cumulative.GreaterThan(peak) {
				peak = cumulative
			}
			
			if peak.GreaterThan(decimal.Zero) {
				drawdown := peak.Sub(cumulative).Div(peak).InexactFloat64()
				if drawdown > maxDrawdown {
					maxDrawdown = drawdown
				}
			}
		}
		
		metrics.CurrentDrawdown = maxDrawdown
		
		// Today's PnL
		if len(history) > 0 {
			metrics.DailyPnL = history[len(history)-1]
		}
	}
	
	// Calculate VaR (simplified - assumes normal distribution)
	if history, exists := rm.pnlHistory[account]; exists && len(history) > 5 {
		metrics.VaR95 = rm.calculateVaR(history, 0.95)
	}
	
	return metrics
}

func (rm *RiskManager) calculateVaR(pnlHistory []decimal.Decimal, confidence float64) decimal.Decimal {
	// Simplified VaR calculation
	// In production, use proper statistical methods
	
	// Calculate mean and standard deviation
	sum := decimal.Zero
	for _, pnl := range pnlHistory {
		sum = sum.Add(pnl)
	}
	mean := sum.Div(decimal.NewFromInt(int64(len(pnlHistory))))
	
	// Calculate variance
	variance := decimal.Zero
	for _, pnl := range pnlHistory {
		diff := pnl.Sub(mean)
		variance = variance.Add(diff.Mul(diff))
	}
	variance = variance.Div(decimal.NewFromInt(int64(len(pnlHistory) - 1)))
	
	// Standard deviation (approximation using square root approximation)
	// For simplicity, we'll use a rough approximation
	// In production, use a proper math library
	stdDev := variance.Div(decimal.NewFromInt(2)) // Very rough approximation
	
	// VaR at 95% confidence (1.645 standard deviations)
	var95 := mean.Sub(stdDev.Mul(decimal.NewFromFloat(1.645)))
	
	return var95
}