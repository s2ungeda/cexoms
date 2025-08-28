package risk

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/mExOms/pkg/nats"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
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
	AccountID       string          `json:"account_id"`
	Exchange        string          `json:"exchange"`
	TotalExposure   decimal.Decimal `json:"total_exposure"`
	OpenPositions   int             `json:"open_positions"`
	CurrentDrawdown float64         `json:"current_drawdown"`
	DailyPnL        decimal.Decimal `json:"daily_pnl"`
	VaR95           decimal.Decimal `json:"var_95"` // Value at Risk at 95% confidence
	MarginRatio     float64         `json:"margin_ratio"`
	Leverage        float64         `json:"leverage"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// RiskEngine is an alias for RiskManager for backward compatibility
type RiskEngine = RiskManager

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
	
	// NATS publishing
	natsClient       *nats.Client
	logger           *zap.Logger
	metricsTicker    *time.Ticker
	stopPublish      chan struct{}
	lastPublishedMetrics map[string]*RiskMetrics // Cache last published metrics
}

// NewRiskManager creates a new risk manager instance
func NewRiskManager(natsClient *nats.Client, logger *zap.Logger) *RiskManager {
	rm := &RiskManager{
		maxDrawdown:          0.10,  // 10% default
		maxExposure:          decimal.NewFromInt(100000), // $100k default
		maxPositionCount:     10,    // 10 positions default
		positions:            make(map[string]map[string]*types.Position),
		balances:             make(map[string]decimal.Decimal),
		pnlHistory:           make(map[string][]decimal.Decimal),
		natsClient:           natsClient,
		logger:               logger,
		metricsTicker:        time.NewTicker(5 * time.Second),
		stopPublish:          make(chan struct{}),
		lastPublishedMetrics: make(map[string]*RiskMetrics),
	}
	
	// Start publishing metrics if NATS client is provided
	if natsClient != nil {
		go rm.publishRiskMetrics()
	}
	
	return rm
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
	
	metrics := rm.calculateAccountMetrics(account)
	
	// Publish metrics if changed significantly
	if rm.shouldPublishMetrics(account, metrics) {
		go rm.publishAccountMetrics(account, metrics)
	}
	
	return metrics
}

// UpdatePosition updates position information for risk tracking
func (rm *RiskManager) UpdatePosition(account string, position *types.Position) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	if _, exists := rm.positions[account]; !exists {
		rm.positions[account] = make(map[string]*types.Position)
	}
	
	if position.Amount.IsZero() {
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
			exposure := pos.Amount.Mul(pos.MarkPrice)
			total = total.Add(exposure)
		}
	}
	
	return total
}

func (rm *RiskManager) calculateAccountMetrics(account string) *RiskMetrics {
	metrics := &RiskMetrics{
		AccountID:     account,
		TotalExposure: decimal.Zero,
		OpenPositions: 0,
		UpdatedAt:     time.Now(),
	}
	
	// Calculate exposure and position count
	if positions, exists := rm.positions[account]; exists {
		for _, pos := range positions {
			exposure := pos.Amount.Mul(pos.MarkPrice)
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

// GetMetrics returns risk metrics
func (rm *RiskManager) GetMetrics() map[string]interface{} {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	totalPositions := 0
	for _, positions := range rm.positions {
		totalPositions += len(positions)
	}
	
	return map[string]interface{}{
		"max_drawdown": rm.maxDrawdown,
		"max_exposure": rm.maxExposure.String(),
		"current_exposure": rm.calculateTotalExposure().String(),
		"total_positions": totalPositions,
		"auto_stop_loss": rm.autoStopLoss,
		"stop_loss_percent": rm.autoStopLossPercent,
	}
}

// publishRiskMetrics publishes risk metrics periodically to NATS
func (rm *RiskManager) publishRiskMetrics() {
	for {
		select {
		case <-rm.metricsTicker.C:
			rm.publishAllAccountMetrics()
		case <-rm.stopPublish:
			return
		}
	}
}

// publishAllAccountMetrics publishes metrics for all accounts
func (rm *RiskManager) publishAllAccountMetrics() {
	rm.mu.RLock()
	accounts := make([]string, 0, len(rm.balances))
	for account := range rm.balances {
		accounts = append(accounts, account)
	}
	rm.mu.RUnlock()
	
	for _, account := range accounts {
		metrics := rm.GetAccountRiskMetrics(account)
		if metrics != nil {
			rm.publishAccountMetrics(account, metrics)
		}
	}
	
	// Publish aggregated system risk metrics
	rm.publishSystemRiskMetrics()
}

// publishAccountMetrics publishes risk metrics for a specific account
func (rm *RiskManager) publishAccountMetrics(account string, metrics *RiskMetrics) error {
	if rm.natsClient == nil {
		return nil
	}
	
	// Determine exchange from account ID (assuming format: exchange_account)
	exchange := "unknown"
	if parts := strings.Split(account, "_"); len(parts) >= 1 {
		exchange = parts[0]
	}
	
	metrics.AccountID = account
	metrics.Exchange = exchange
	
	data, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("failed to marshal risk metrics: %w", err)
	}
	
	// Subject format: risk.metrics.{exchange}.{account}
	subject := fmt.Sprintf("risk.metrics.%s.%s", exchange, account)
	
	if err := rm.natsClient.Publish(subject, data); err != nil {
		rm.logger.Error("Failed to publish risk metrics",
			zap.String("account", account),
			zap.Error(err))
		return err
	}
	
	// Cache published metrics
	rm.mu.Lock()
	rm.lastPublishedMetrics[account] = metrics
	rm.mu.Unlock()
	
	return nil
}

// publishSystemRiskMetrics publishes overall system risk metrics
func (rm *RiskManager) publishSystemRiskMetrics() error {
	if rm.natsClient == nil {
		return nil
	}
	
	systemMetrics := map[string]interface{}{
		"total_exposure":    rm.GetCurrentExposure().String(),
		"max_exposure":      rm.maxExposure.String(),
		"exposure_ratio":    rm.GetCurrentExposure().Div(rm.maxExposure).InexactFloat64(),
		"max_drawdown":      rm.maxDrawdown,
		"total_accounts":    len(rm.balances),
		"total_positions":   rm.getTotalPositionCount(),
		"timestamp":         time.Now(),
	}
	
	data, err := json.Marshal(systemMetrics)
	if err != nil {
		return fmt.Errorf("failed to marshal system metrics: %w", err)
	}
	
	subject := "risk.metrics.system"
	if err := rm.natsClient.Publish(subject, data); err != nil {
		rm.logger.Error("Failed to publish system risk metrics", zap.Error(err))
		return err
	}
	
	return nil
}

// shouldPublishMetrics checks if metrics have changed significantly
func (rm *RiskManager) shouldPublishMetrics(account string, metrics *RiskMetrics) bool {
	rm.mu.RLock()
	lastMetrics, exists := rm.lastPublishedMetrics[account]
	rm.mu.RUnlock()
	
	if !exists {
		return true
	}
	
	// Publish if significant changes detected
	exposureChange := metrics.TotalExposure.Sub(lastMetrics.TotalExposure).Abs()
	exposureThreshold := lastMetrics.TotalExposure.Mul(decimal.NewFromFloat(0.05)) // 5% change
	
	if exposureChange.GreaterThan(exposureThreshold) {
		return true
	}
	
	if metrics.OpenPositions != lastMetrics.OpenPositions {
		return true
	}
	
	if math.Abs(metrics.CurrentDrawdown-lastMetrics.CurrentDrawdown) > 0.01 { // 1% change
		return true
	}
	
	// Publish at least every 30 seconds
	if time.Since(lastMetrics.UpdatedAt) > 30*time.Second {
		return true
	}
	
	return false
}

// getTotalPositionCount returns total number of positions across all accounts
func (rm *RiskManager) getTotalPositionCount() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	total := 0
	for _, positions := range rm.positions {
		total += len(positions)
	}
	return total
}

// Stop stops the risk manager
func (rm *RiskManager) Stop() {
	if rm.metricsTicker != nil {
		rm.metricsTicker.Stop()
	}
	close(rm.stopPublish)
}