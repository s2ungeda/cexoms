package monitoring

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// AlertLevel represents the severity of an alert
type AlertLevel string

const (
	AlertLevelInfo     AlertLevel = "info"
	AlertLevelWarning  AlertLevel = "warning"
	AlertLevelCritical AlertLevel = "critical"
)

// Alert represents a monitoring alert
type Alert struct {
	ID          string
	Name        string
	Level       AlertLevel
	Category    string
	AccountID   string
	Exchange    string
	Symbol      string
	Message     string
	Value       float64
	Threshold   float64
	Timestamp   time.Time
	ResolvedAt  *time.Time
}

// AlertRule defines conditions for triggering alerts
type AlertRule struct {
	ID          string
	Name        string
	Category    string
	Level       AlertLevel
	Condition   AlertCondition
	Threshold   float64
	Duration    time.Duration // How long condition must be true
	Cooldown    time.Duration // Minimum time between alerts
	Actions     []AlertAction
}

// AlertCondition represents the condition to check
type AlertCondition func(ctx context.Context) (value float64, triggered bool, details map[string]interface{})

// AlertAction represents an action to take when alert triggers
type AlertAction func(alert *Alert) error

// AlertManager manages system alerts
type AlertManager struct {
	mu sync.RWMutex
	
	rules        map[string]*AlertRule
	activeAlerts map[string]*Alert
	alertHistory []Alert
	
	// Alert channels
	alertChan chan *Alert
	
	// Dependencies
	collector *Collector
	
	// Control
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewAlertManager creates a new alert manager
func NewAlertManager(collector *Collector) *AlertManager {
	am := &AlertManager{
		rules:        make(map[string]*AlertRule),
		activeAlerts: make(map[string]*Alert),
		alertHistory: make([]Alert, 0),
		alertChan:    make(chan *Alert, 1000),
		collector:    collector,
		stopCh:       make(chan struct{}),
	}
	
	// Initialize default alert rules
	am.initializeDefaultRules()
	
	return am
}

// Start starts the alert manager
func (am *AlertManager) Start(ctx context.Context) error {
	am.wg.Add(1)
	go am.checkAlerts(ctx)
	return nil
}

// Stop stops the alert manager
func (am *AlertManager) Stop() {
	close(am.stopCh)
	am.wg.Wait()
}

// AddRule adds a new alert rule
func (am *AlertManager) AddRule(rule *AlertRule) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.rules[rule.ID] = rule
}

// RemoveRule removes an alert rule
func (am *AlertManager) RemoveRule(ruleID string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	delete(am.rules, ruleID)
}

// GetActiveAlerts returns all active alerts
func (am *AlertManager) GetActiveAlerts() []Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()
	
	alerts := make([]Alert, 0, len(am.activeAlerts))
	for _, alert := range am.activeAlerts {
		alerts = append(alerts, *alert)
	}
	return alerts
}

// GetAlertChannel returns the alert notification channel
func (am *AlertManager) GetAlertChannel() <-chan *Alert {
	return am.alertChan
}

// checkAlerts periodically checks alert conditions
func (am *AlertManager) checkAlerts(ctx context.Context) {
	defer am.wg.Done()
	
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	conditionStates := make(map[string]time.Time) // Track when conditions became true
	lastAlertTimes := make(map[string]time.Time)  // Track last alert time for cooldown
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-am.stopCh:
			return
		case <-ticker.C:
			am.mu.RLock()
			rules := make([]*AlertRule, 0, len(am.rules))
			for _, rule := range am.rules {
				rules = append(rules, rule)
			}
			am.mu.RUnlock()
			
			// Check each rule
			for _, rule := range rules {
				value, triggered, details := rule.Condition(ctx)
				
				if triggered {
					// Check if condition just became true
					if _, exists := conditionStates[rule.ID]; !exists {
						conditionStates[rule.ID] = time.Now()
					}
					
					// Check if duration requirement is met
					if time.Since(conditionStates[rule.ID]) >= rule.Duration {
						// Check cooldown
						if lastAlert, exists := lastAlertTimes[rule.ID]; !exists || time.Since(lastAlert) >= rule.Cooldown {
							// Trigger alert
							alert := &Alert{
								ID:        fmt.Sprintf("%s-%d", rule.ID, time.Now().Unix()),
								Name:      rule.Name,
								Level:     rule.Level,
								Category:  rule.Category,
								Message:   am.formatAlertMessage(rule, value, details),
								Value:     value,
								Threshold: rule.Threshold,
								Timestamp: time.Now(),
							}
							
							// Extract account/exchange/symbol from details if available
							if accountID, ok := details["account_id"].(string); ok {
								alert.AccountID = accountID
							}
							if exchange, ok := details["exchange"].(string); ok {
								alert.Exchange = exchange
							}
							if symbol, ok := details["symbol"].(string); ok {
								alert.Symbol = symbol
							}
							
							am.triggerAlert(alert, rule)
							lastAlertTimes[rule.ID] = time.Now()
						}
					}
				} else {
					// Condition is false, reset state
					delete(conditionStates, rule.ID)
					
					// Check if we should resolve active alert
					am.mu.Lock()
					for id, activeAlert := range am.activeAlerts {
						if activeAlert.Name == rule.Name && activeAlert.ResolvedAt == nil {
							activeAlert.ResolvedAt = &[]time.Time{time.Now()}[0]
							am.alertHistory = append(am.alertHistory, *activeAlert)
							delete(am.activeAlerts, id)
						}
					}
					am.mu.Unlock()
				}
			}
		}
	}
}

// triggerAlert triggers an alert
func (am *AlertManager) triggerAlert(alert *Alert, rule *AlertRule) {
	am.mu.Lock()
	am.activeAlerts[alert.ID] = alert
	am.mu.Unlock()
	
	// Send to channel
	select {
	case am.alertChan <- alert:
	default:
		// Channel full, drop alert
	}
	
	// Execute actions
	for _, action := range rule.Actions {
		go func(a AlertAction) {
			if err := a(alert); err != nil {
				// Log error
			}
		}(action)
	}
}

// formatAlertMessage formats the alert message
func (am *AlertManager) formatAlertMessage(rule *AlertRule, value float64, details map[string]interface{}) string {
	msg := fmt.Sprintf("%s: %.2f (threshold: %.2f)", rule.Name, value, rule.Threshold)
	
	// Add details
	if len(details) > 0 {
		msg += " ["
		first := true
		for k, v := range details {
			if !first {
				msg += ", "
			}
			msg += fmt.Sprintf("%s: %v", k, v)
			first = false
		}
		msg += "]"
	}
	
	return msg
}

// initializeDefaultRules initializes default alert rules
func (am *AlertManager) initializeDefaultRules() {
	// High margin usage alert
	am.AddRule(&AlertRule{
		ID:       "high_margin_usage",
		Name:     "High Margin Usage",
		Category: "risk",
		Level:    AlertLevelWarning,
		Threshold: 0.8, // 80%
		Duration:  1 * time.Minute,
		Cooldown:  5 * time.Minute,
		Condition: am.createMarginUsageCondition(0.8),
	})
	
	// Critical margin level alert
	am.AddRule(&AlertRule{
		ID:       "critical_margin_level",
		Name:     "Critical Margin Level",
		Category: "risk",
		Level:    AlertLevelCritical,
		Threshold: 1.2, // 120% margin level
		Duration:  30 * time.Second,
		Cooldown:  2 * time.Minute,
		Condition: am.createMarginLevelCondition(1.2),
	})
	
	// Large position loss alert
	am.AddRule(&AlertRule{
		ID:       "large_position_loss",
		Name:     "Large Position Loss",
		Category: "position",
		Level:    AlertLevelWarning,
		Threshold: -1000, // -$1000 USD
		Duration:  30 * time.Second,
		Cooldown:  10 * time.Minute,
		Condition: am.createPositionLossCondition(-1000),
	})
	
	// Account drawdown alert
	am.AddRule(&AlertRule{
		ID:       "account_drawdown",
		Name:     "Account Drawdown",
		Category: "account",
		Level:    AlertLevelWarning,
		Threshold: 0.1, // 10% drawdown
		Duration:  5 * time.Minute,
		Cooldown:  30 * time.Minute,
		Condition: am.createDrawdownCondition(0.1),
	})
	
	// High API error rate
	am.AddRule(&AlertRule{
		ID:       "high_api_errors",
		Name:     "High API Error Rate",
		Category: "system",
		Level:    AlertLevelWarning,
		Threshold: 0.05, // 5% error rate
		Duration:  2 * time.Minute,
		Cooldown:  10 * time.Minute,
		Condition: am.createAPIErrorRateCondition(0.05),
	})
	
	// WebSocket disconnection
	am.AddRule(&AlertRule{
		ID:       "websocket_disconnected",
		Name:     "WebSocket Disconnected",
		Category: "connectivity",
		Level:    AlertLevelCritical,
		Threshold: 0,
		Duration:  10 * time.Second,
		Cooldown:  1 * time.Minute,
		Condition: am.createWebSocketCondition(),
	})
}

// Condition factory functions

func (am *AlertManager) createMarginUsageCondition(threshold float64) AlertCondition {
	return func(ctx context.Context) (float64, bool, map[string]interface{}) {
		// Check margin usage across all accounts
		// This is a simplified example - real implementation would query actual data
		return 0, false, nil
	}
}

func (am *AlertManager) createMarginLevelCondition(threshold float64) AlertCondition {
	return func(ctx context.Context) (float64, bool, map[string]interface{}) {
		// Check margin level across accounts
		return 0, false, nil
	}
}

func (am *AlertManager) createPositionLossCondition(threshold float64) AlertCondition {
	return func(ctx context.Context) (float64, bool, map[string]interface{}) {
		// Check for positions with large losses
		portfolio := am.collector.positionManager.GetPortfolioSummary()
		
		for accountID, summary := range portfolio.AccountSummaries {
			for symbol, pos := range summary.PositionsBySymbol {
				pnl := pos.UnrealizedPnL.InexactFloat64()
				if pnl < threshold {
					return pnl, true, map[string]interface{}{
						"account_id": accountID,
						"symbol":     symbol,
						"side":       pos.Side,
					}
				}
			}
		}
		
		return 0, false, nil
	}
}

func (am *AlertManager) createDrawdownCondition(threshold float64) AlertCondition {
	return func(ctx context.Context) (float64, bool, map[string]interface{}) {
		// Check account drawdown
		return 0, false, nil
	}
}

func (am *AlertManager) createAPIErrorRateCondition(threshold float64) AlertCondition {
	return func(ctx context.Context) (float64, bool, map[string]interface{}) {
		// Check API error rate from metrics
		return 0, false, nil
	}
}

func (am *AlertManager) createWebSocketCondition() AlertCondition {
	return func(ctx context.Context) (float64, bool, map[string]interface{}) {
		// Check WebSocket connection status
		return 0, false, nil
	}
}