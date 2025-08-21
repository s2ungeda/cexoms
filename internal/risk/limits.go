package risk

import (
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// LimitType represents different types of risk limits
type LimitType string

const (
	LimitTypeMaxLoss        LimitType = "MAX_LOSS"
	LimitTypeMaxDrawdown    LimitType = "MAX_DRAWDOWN"
	LimitTypeMaxExposure    LimitType = "MAX_EXPOSURE"
	LimitTypeMaxPositions   LimitType = "MAX_POSITIONS"
	LimitTypeDailyLoss      LimitType = "DAILY_LOSS"
	LimitTypeConcentration  LimitType = "CONCENTRATION"
	LimitTypeLeverage       LimitType = "LEVERAGE"
)

// RiskLimit represents a risk limit configuration
type RiskLimit struct {
	Type        LimitType       `json:"type"`
	Value       decimal.Decimal `json:"value"`
	CurrentUsed decimal.Decimal `json:"current_used"`
	Enabled     bool            `json:"enabled"`
	Actions     []LimitAction   `json:"actions"`
	LastChecked time.Time       `json:"last_checked"`
}

// LimitAction represents an action to take when limit is breached
type LimitAction struct {
	Threshold   float64 `json:"threshold"` // Percentage of limit (0.8 = 80%)
	Action      string  `json:"action"`    // "warn", "restrict", "close_all"
	Notification bool   `json:"notification"`
}

// RiskLimitManager manages risk limits
type RiskLimitManager struct {
	mu     sync.RWMutex
	limits map[string]map[LimitType]*RiskLimit // account -> limit type -> limit
	
	// Callbacks for limit breaches
	onLimitBreach func(account string, limit *RiskLimit, breachLevel float64)
}

// NewRiskLimitManager creates a new risk limit manager
func NewRiskLimitManager() *RiskLimitManager {
	return &RiskLimitManager{
		limits: make(map[string]map[LimitType]*RiskLimit),
	}
}

// SetLimit sets a risk limit for an account
func (m *RiskLimitManager) SetLimit(account string, limitType LimitType, value decimal.Decimal, actions []LimitAction) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.limits[account]; !exists {
		m.limits[account] = make(map[LimitType]*RiskLimit)
	}
	
	m.limits[account][limitType] = &RiskLimit{
		Type:        limitType,
		Value:       value,
		CurrentUsed: decimal.Zero,
		Enabled:     true,
		Actions:     actions,
		LastChecked: time.Now(),
	}
}

// CheckLimit checks if a value would breach a limit
func (m *RiskLimitManager) CheckLimit(account string, limitType LimitType, newValue decimal.Decimal) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	accountLimits, exists := m.limits[account]
	if !exists {
		return nil
	}
	
	limit, exists := accountLimits[limitType]
	if !exists || !limit.Enabled {
		return nil
	}
	
	if newValue.GreaterThan(limit.Value) {
		return fmt.Errorf("limit breach: %s would exceed %s limit of %s", 
			newValue, limitType, limit.Value)
	}
	
	// Check action thresholds
	usage := newValue.Div(limit.Value).InexactFloat64()
	for _, action := range limit.Actions {
		if usage >= action.Threshold {
			m.triggerAction(account, limit, usage, action)
		}
	}
	
	return nil
}

// UpdateUsage updates the current usage of a limit
func (m *RiskLimitManager) UpdateUsage(account string, limitType LimitType, currentValue decimal.Decimal) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if accountLimits, exists := m.limits[account]; exists {
		if limit, exists := accountLimits[limitType]; exists {
			limit.CurrentUsed = currentValue
			limit.LastChecked = time.Now()
			
			// Check if any actions need to be triggered
			usage := currentValue.Div(limit.Value).InexactFloat64()
			for _, action := range limit.Actions {
				if usage >= action.Threshold {
					m.triggerAction(account, limit, usage, action)
				}
			}
		}
	}
}

// GetLimit returns a specific limit for an account
func (m *RiskLimitManager) GetLimit(account string, limitType LimitType) (*RiskLimit, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if accountLimits, exists := m.limits[account]; exists {
		if limit, exists := accountLimits[limitType]; exists {
			return limit, true
		}
	}
	
	return nil, false
}

// GetAllLimits returns all limits for an account
func (m *RiskLimitManager) GetAllLimits(account string) map[LimitType]*RiskLimit {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if accountLimits, exists := m.limits[account]; exists {
		// Return a copy
		result := make(map[LimitType]*RiskLimit)
		for k, v := range accountLimits {
			result[k] = v
		}
		return result
	}
	
	return nil
}

// EnableLimit enables a specific limit
func (m *RiskLimitManager) EnableLimit(account string, limitType LimitType) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if accountLimits, exists := m.limits[account]; exists {
		if limit, exists := accountLimits[limitType]; exists {
			limit.Enabled = true
		}
	}
}

// DisableLimit disables a specific limit
func (m *RiskLimitManager) DisableLimit(account string, limitType LimitType) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if accountLimits, exists := m.limits[account]; exists {
		if limit, exists := accountLimits[limitType]; exists {
			limit.Enabled = false
		}
	}
}

// SetBreachCallback sets the callback for limit breaches
func (m *RiskLimitManager) SetBreachCallback(callback func(account string, limit *RiskLimit, breachLevel float64)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onLimitBreach = callback
}

// CheckAllLimits checks all limits for an account
func (m *RiskLimitManager) CheckAllLimits(account string) []error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var errors []error
	
	if accountLimits, exists := m.limits[account]; exists {
		for _, limit := range accountLimits {
			if !limit.Enabled {
				continue
			}
			
			if limit.CurrentUsed.GreaterThan(limit.Value) {
				errors = append(errors, fmt.Errorf("%s limit breached: %s > %s", 
					limit.Type, limit.CurrentUsed, limit.Value))
			}
		}
	}
	
	return errors
}

// ResetDailyLimits resets daily limits (called at start of trading day)
func (m *RiskLimitManager) ResetDailyLimits() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for _, accountLimits := range m.limits {
		for _, limit := range accountLimits {
			if limit.Type == LimitTypeDailyLoss {
				limit.CurrentUsed = decimal.Zero
				limit.LastChecked = time.Now()
			}
		}
	}
}

// triggerAction triggers an action for a limit breach
func (m *RiskLimitManager) triggerAction(account string, limit *RiskLimit, usage float64, action LimitAction) {
	// Call the breach callback if set
	if m.onLimitBreach != nil {
		go m.onLimitBreach(account, limit, usage)
	}
	
	// Log the action
	fmt.Printf("[RISK LIMIT] Account: %s, Limit: %s, Usage: %.2f%%, Action: %s\n", 
		account, limit.Type, usage*100, action.Action)
}

// GetLimitStatus returns the status of all limits for an account
func (m *RiskLimitManager) GetLimitStatus(account string) map[LimitType]LimitStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	status := make(map[LimitType]LimitStatus)
	
	if accountLimits, exists := m.limits[account]; exists {
		for limitType, limit := range accountLimits {
			usage := float64(0)
			if limit.Value.GreaterThan(decimal.Zero) {
				usage = limit.CurrentUsed.Div(limit.Value).InexactFloat64()
			}
			
			status[limitType] = LimitStatus{
				Type:        limitType,
				Enabled:     limit.Enabled,
				Value:       limit.Value,
				CurrentUsed: limit.CurrentUsed,
				Usage:       usage,
				Status:      m.getLimitStatusLevel(usage),
				LastChecked: limit.LastChecked,
			}
		}
	}
	
	return status
}

// LimitStatus represents the current status of a limit
type LimitStatus struct {
	Type        LimitType       `json:"type"`
	Enabled     bool            `json:"enabled"`
	Value       decimal.Decimal `json:"value"`
	CurrentUsed decimal.Decimal `json:"current_used"`
	Usage       float64         `json:"usage"` // 0.0 to 1.0+
	Status      string          `json:"status"` // "safe", "warning", "critical", "breached"
	LastChecked time.Time       `json:"last_checked"`
}

// getLimitStatusLevel returns the status level based on usage
func (m *RiskLimitManager) getLimitStatusLevel(usage float64) string {
	switch {
	case usage >= 1.0:
		return "breached"
	case usage >= 0.9:
		return "critical"
	case usage >= 0.7:
		return "warning"
	default:
		return "safe"
	}
}