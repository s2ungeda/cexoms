package risk

import (
	"fmt"
	"sync"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// MonitoringInterval represents monitoring check intervals
type MonitoringInterval time.Duration

const (
	MonitoringIntervalRealtime  = MonitoringInterval(100 * time.Millisecond)
	MonitoringIntervalFast      = MonitoringInterval(1 * time.Second)
	MonitoringIntervalNormal    = MonitoringInterval(5 * time.Second)
	MonitoringIntervalSlow      = MonitoringInterval(30 * time.Second)
)

// Alert represents a risk alert
type Alert struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Severity   string          `json:"severity"` // "info", "warning", "critical"
	Account    string          `json:"account"`
	Symbol     string          `json:"symbol"`
	Message    string          `json:"message"`
	Value      decimal.Decimal `json:"value"`
	Threshold  decimal.Decimal `json:"threshold"`
	Timestamp  time.Time       `json:"timestamp"`
	Resolved   bool            `json:"resolved"`
	ResolvedAt *time.Time      `json:"resolved_at"`
}

// RiskMonitor provides real-time risk monitoring
type RiskMonitor struct {
	mu sync.RWMutex
	
	// Managers
	riskManager      *RiskManager
	limitManager     *RiskLimitManager
	stopLossManager  *StopLossManager
	
	// Monitoring state
	isRunning        bool
	stopCh           chan struct{}
	interval         MonitoringInterval
	
	// Alerts
	activeAlerts     map[string]*Alert
	alertHistory     []Alert
	maxAlertHistory  int
	
	// Callbacks
	onAlert          func(alert *Alert)
	onMetricsUpdate  func(metrics map[string]*RiskMetrics)
	
	// Position and price tracking
	positions        map[string]map[string]*types.Position // account -> symbol -> position
	prices           map[string]decimal.Decimal            // symbol -> price
}

// NewRiskMonitor creates a new risk monitor
func NewRiskMonitor(riskManager *RiskManager, limitManager *RiskLimitManager, stopLossManager *StopLossManager) *RiskMonitor {
	return &RiskMonitor{
		riskManager:     riskManager,
		limitManager:    limitManager,
		stopLossManager: stopLossManager,
		interval:        MonitoringIntervalNormal,
		activeAlerts:    make(map[string]*Alert),
		alertHistory:    make([]Alert, 0),
		maxAlertHistory: 1000,
		positions:       make(map[string]map[string]*types.Position),
		prices:          make(map[string]decimal.Decimal),
		stopCh:          make(chan struct{}),
	}
}

// Start starts the risk monitoring
func (m *RiskMonitor) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.isRunning {
		return fmt.Errorf("monitor already running")
	}
	
	m.isRunning = true
	go m.monitoringLoop()
	
	return nil
}

// Stop stops the risk monitoring
func (m *RiskMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.isRunning {
		close(m.stopCh)
		m.isRunning = false
	}
}

// SetInterval sets the monitoring interval
func (m *RiskMonitor) SetInterval(interval MonitoringInterval) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.interval = interval
}

// UpdatePosition updates position information
func (m *RiskMonitor) UpdatePosition(account string, position *types.Position) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.positions[account]; !exists {
		m.positions[account] = make(map[string]*types.Position)
	}
	
	if position.Quantity == 0 {
		delete(m.positions[account], position.Symbol)
	} else {
		m.positions[account][position.Symbol] = position
	}
	
	// Update in risk manager
	m.riskManager.UpdatePosition(account, position)
}

// UpdatePrice updates price information
func (m *RiskMonitor) UpdatePrice(symbol string, price decimal.Decimal) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.prices[symbol] = price
	
	// Update stop loss manager
	triggered := m.stopLossManager.UpdatePrice(symbol, price)
	for _, account := range triggered {
		m.createAlert(&Alert{
			Type:      "STOP_LOSS_TRIGGERED",
			Severity:  "critical",
			Account:   account,
			Symbol:    symbol,
			Message:   fmt.Sprintf("Stop loss triggered for %s at %s", symbol, price),
			Value:     price,
			Timestamp: time.Now(),
		})
	}
}

// SetAlertCallback sets the callback for alerts
func (m *RiskMonitor) SetAlertCallback(callback func(alert *Alert)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onAlert = callback
}

// SetMetricsCallback sets the callback for metrics updates
func (m *RiskMonitor) SetMetricsCallback(callback func(metrics map[string]*RiskMetrics)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onMetricsUpdate = callback
}

// GetActiveAlerts returns all active alerts
func (m *RiskMonitor) GetActiveAlerts() []*Alert {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	alerts := make([]*Alert, 0, len(m.activeAlerts))
	for _, alert := range m.activeAlerts {
		alerts = append(alerts, alert)
	}
	
	return alerts
}

// ResolveAlert marks an alert as resolved
func (m *RiskMonitor) ResolveAlert(alertID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	alert, exists := m.activeAlerts[alertID]
	if !exists {
		return fmt.Errorf("alert %s not found", alertID)
	}
	
	now := time.Now()
	alert.Resolved = true
	alert.ResolvedAt = &now
	
	// Move to history
	m.alertHistory = append(m.alertHistory, *alert)
	if len(m.alertHistory) > m.maxAlertHistory {
		m.alertHistory = m.alertHistory[1:]
	}
	
	delete(m.activeAlerts, alertID)
	
	return nil
}

// monitoringLoop is the main monitoring loop
func (m *RiskMonitor) monitoringLoop() {
	ticker := time.NewTicker(time.Duration(m.interval))
	defer ticker.Stop()
	
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.performChecks()
		}
	}
}

// performChecks performs all risk checks
func (m *RiskMonitor) performChecks() {
	m.mu.RLock()
	accounts := make([]string, 0, len(m.positions))
	for account := range m.positions {
		accounts = append(accounts, account)
	}
	m.mu.RUnlock()
	
	// Check each account
	metricsMap := make(map[string]*RiskMetrics)
	
	for _, account := range accounts {
		// Get risk metrics
		metrics := m.riskManager.GetAccountRiskMetrics(account)
		metricsMap[account] = metrics
		
		// Check drawdown
		m.checkDrawdown(account, metrics)
		
		// Check exposure
		m.checkExposure(account, metrics)
		
		// Check position count
		m.checkPositionCount(account, metrics)
		
		// Check all limits
		m.checkLimits(account)
		
		// Check margin levels for futures
		m.checkMarginLevels(account)
	}
	
	// Call metrics update callback
	if m.onMetricsUpdate != nil {
		go m.onMetricsUpdate(metricsMap)
	}
}

// checkDrawdown checks drawdown limits
func (m *RiskMonitor) checkDrawdown(account string, metrics *RiskMetrics) {
	limit, exists := m.limitManager.GetLimit(account, LimitTypeMaxDrawdown)
	if !exists || !limit.Enabled {
		return
	}
	
	drawdownPercent := decimal.NewFromFloat(metrics.CurrentDrawdown)
	
	// Update limit usage
	m.limitManager.UpdateUsage(account, LimitTypeMaxDrawdown, drawdownPercent)
	
	// Check if alert needed
	if drawdownPercent.GreaterThan(limit.Value.Mul(decimal.NewFromFloat(0.8))) {
		severity := "warning"
		if drawdownPercent.GreaterThan(limit.Value) {
			severity = "critical"
		}
		
		m.createAlert(&Alert{
			Type:      "DRAWDOWN_ALERT",
			Severity:  severity,
			Account:   account,
			Message:   fmt.Sprintf("Drawdown at %.2f%% (limit: %.2f%%)", metrics.CurrentDrawdown*100, limit.Value.InexactFloat64()*100),
			Value:     drawdownPercent,
			Threshold: limit.Value,
			Timestamp: time.Now(),
		})
	}
}

// checkExposure checks exposure limits
func (m *RiskMonitor) checkExposure(account string, metrics *RiskMetrics) {
	limit, exists := m.limitManager.GetLimit(account, LimitTypeMaxExposure)
	if !exists || !limit.Enabled {
		return
	}
	
	// Update limit usage
	m.limitManager.UpdateUsage(account, LimitTypeMaxExposure, metrics.TotalExposure)
	
	// Check if alert needed
	if metrics.TotalExposure.GreaterThan(limit.Value.Mul(decimal.NewFromFloat(0.8))) {
		severity := "warning"
		if metrics.TotalExposure.GreaterThan(limit.Value) {
			severity = "critical"
		}
		
		m.createAlert(&Alert{
			Type:      "EXPOSURE_ALERT",
			Severity:  severity,
			Account:   account,
			Message:   fmt.Sprintf("Exposure at %s (limit: %s)", metrics.TotalExposure, limit.Value),
			Value:     metrics.TotalExposure,
			Threshold: limit.Value,
			Timestamp: time.Now(),
		})
	}
}

// checkPositionCount checks position count limits
func (m *RiskMonitor) checkPositionCount(account string, metrics *RiskMetrics) {
	limit, exists := m.limitManager.GetLimit(account, LimitTypeMaxPositions)
	if !exists || !limit.Enabled {
		return
	}
	
	positionCount := decimal.NewFromInt(int64(metrics.OpenPositions))
	
	// Update limit usage
	m.limitManager.UpdateUsage(account, LimitTypeMaxPositions, positionCount)
	
	// Check if alert needed
	if positionCount.GreaterThanOrEqual(limit.Value) {
		m.createAlert(&Alert{
			Type:      "POSITION_COUNT_ALERT",
			Severity:  "warning",
			Account:   account,
			Message:   fmt.Sprintf("Position count at %d (limit: %s)", metrics.OpenPositions, limit.Value),
			Value:     positionCount,
			Threshold: limit.Value,
			Timestamp: time.Now(),
		})
	}
}

// checkLimits checks all configured limits
func (m *RiskMonitor) checkLimits(account string) {
	errors := m.limitManager.CheckAllLimits(account)
	for _, err := range errors {
		m.createAlert(&Alert{
			Type:      "LIMIT_BREACH",
			Severity:  "critical",
			Account:   account,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
	}
}

// checkMarginLevels checks margin levels for futures positions
func (m *RiskMonitor) checkMarginLevels(account string) {
	m.mu.RLock()
	positions, exists := m.positions[account]
	m.mu.RUnlock()
	
	if !exists {
		return
	}
	
	for symbol, position := range positions {
		// Check if leverage is too high
		if position.Leverage > 10 {
			m.createAlert(&Alert{
				Type:      "HIGH_LEVERAGE",
				Severity:  "warning",
				Account:   account,
				Symbol:    symbol,
				Message:   fmt.Sprintf("High leverage detected: %dx", int(position.Leverage)),
				Value:     decimal.NewFromFloat(position.Leverage),
				Threshold: decimal.NewFromInt(10),
				Timestamp: time.Now(),
			})
		}
		
		// Check unrealized PnL
		unrealizedPnL := decimal.NewFromFloat(position.UnrealizedPNL)
		if unrealizedPnL.LessThan(decimal.NewFromFloat(-1000)) {
			m.createAlert(&Alert{
				Type:      "LARGE_UNREALIZED_LOSS",
				Severity:  "warning",
				Account:   account,
				Symbol:    symbol,
				Message:   fmt.Sprintf("Large unrealized loss: %s", unrealizedPnL),
				Value:     unrealizedPnL,
				Timestamp: time.Now(),
			})
		}
	}
}

// createAlert creates a new alert
func (m *RiskMonitor) createAlert(alert *Alert) {
	alert.ID = fmt.Sprintf("%s_%s_%d", alert.Type, alert.Account, time.Now().UnixNano())
	
	// Check if similar alert already exists
	for id, existing := range m.activeAlerts {
		if existing.Type == alert.Type && 
		   existing.Account == alert.Account && 
		   existing.Symbol == alert.Symbol &&
		   !existing.Resolved {
			// Update existing alert
			m.activeAlerts[id] = alert
			return
		}
	}
	
	// Add new alert
	m.activeAlerts[alert.ID] = alert
	
	// Call alert callback
	if m.onAlert != nil {
		go m.onAlert(alert)
	}
}

// GetRiskSummary returns a summary of current risk status
func (m *RiskMonitor) GetRiskSummary() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	summary := map[string]interface{}{
		"active_alerts":     len(m.activeAlerts),
		"total_accounts":    len(m.positions),
		"monitoring_status": m.isRunning,
		"last_check":        time.Now(),
	}
	
	// Count alerts by severity
	severityCount := map[string]int{
		"info":     0,
		"warning":  0,
		"critical": 0,
	}
	
	for _, alert := range m.activeAlerts {
		severityCount[alert.Severity]++
	}
	
	summary["alerts_by_severity"] = severityCount
	
	return summary
}