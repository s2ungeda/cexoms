package orchestrator

import (
	"fmt"
	"sync"
	"time"
)

// RiskMonitor monitors strategies for risk violations
type RiskMonitor struct {
	config           KillSwitchConfig
	dailyPnL         map[string]float64
	lastResetTime    time.Time
	violationCounts  map[string]int
	mu               sync.RWMutex
}

// NewRiskMonitor creates a new risk monitor
func NewRiskMonitor(config KillSwitchConfig) *RiskMonitor {
	return &RiskMonitor{
		config:          config,
		dailyPnL:        make(map[string]float64),
		lastResetTime:   time.Now(),
		violationCounts: make(map[string]int),
	}
}

// ShouldStopStrategy checks if a strategy should be stopped based on risk limits
func (rm *RiskMonitor) ShouldStopStrategy(instance *StrategyInstance) (bool, string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Reset daily metrics if needed
	rm.resetDailyMetricsIfNeeded()

	instance.mu.RLock()
	metrics := instance.Metrics
	instance.mu.RUnlock()

	if metrics == nil {
		return false, ""
	}

	// Check daily loss limit
	if metrics.DailyPnL < -rm.config.MaxDailyLoss {
		rm.violationCounts[instance.ID]++
		return true, fmt.Sprintf("daily loss %.2f exceeds limit %.2f", metrics.DailyPnL, -rm.config.MaxDailyLoss)
	}

	// Check consecutive losses
	if metrics.ConsecutiveLosses > rm.config.MaxConsecutiveLosses {
		rm.violationCounts[instance.ID]++
		return true, fmt.Sprintf("consecutive losses %d exceeds limit %d", metrics.ConsecutiveLosses, rm.config.MaxConsecutiveLosses)
	}

	// Check maximum drawdown
	if metrics.MaxDrawdown > rm.config.MaxDrawdown {
		rm.violationCounts[instance.ID]++
		return true, fmt.Sprintf("max drawdown %.2f%% exceeds limit %.2f%%", metrics.MaxDrawdown*100, rm.config.MaxDrawdown*100)
	}

	// Check if strategy has too many violations
	if violations := rm.violationCounts[instance.ID]; violations > 3 {
		return true, fmt.Sprintf("too many risk violations: %d", violations)
	}

	// Update daily PnL tracking
	rm.dailyPnL[instance.ID] = metrics.DailyPnL

	return false, ""
}

// CheckGlobalRisk checks global risk across all strategies
func (rm *RiskMonitor) CheckGlobalRisk(strategies []*StrategyInstance) (bool, string) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	totalDailyLoss := 0.0
	totalConsecutiveLosses := 0
	worstDrawdown := 0.0

	for _, instance := range strategies {
		instance.mu.RLock()
		metrics := instance.Metrics
		instance.mu.RUnlock()

		if metrics != nil {
			totalDailyLoss += metrics.DailyPnL
			if metrics.ConsecutiveLosses > 0 {
				totalConsecutiveLosses++
			}
			if metrics.MaxDrawdown > worstDrawdown {
				worstDrawdown = metrics.MaxDrawdown
			}
		}
	}

	// Check total daily loss across all strategies
	if totalDailyLoss < -rm.config.MaxDailyLoss*2 {
		return true, fmt.Sprintf("total daily loss %.2f exceeds global limit", totalDailyLoss)
	}

	// Check if too many strategies have consecutive losses
	if totalConsecutiveLosses > len(strategies)/2 && len(strategies) > 2 {
		return true, fmt.Sprintf("%d strategies have consecutive losses", totalConsecutiveLosses)
	}

	return false, ""
}

// UpdateMetrics updates risk metrics for a strategy
func (rm *RiskMonitor) UpdateMetrics(strategyID string, pnl float64, isWin bool) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Update daily PnL
	rm.dailyPnL[strategyID] += pnl

	// Reset violation count on profitable day
	if rm.dailyPnL[strategyID] > 0 {
		rm.violationCounts[strategyID] = 0
	}
}

// GetRiskStatus returns the current risk status
func (rm *RiskMonitor) GetRiskStatus() map[string]interface{} {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	totalDailyPnL := 0.0
	for _, pnl := range rm.dailyPnL {
		totalDailyPnL += pnl
	}

	return map[string]interface{}{
		"total_daily_pnl":     totalDailyPnL,
		"strategy_count":      len(rm.dailyPnL),
		"violation_counts":    rm.violationCounts,
		"last_reset":          rm.lastResetTime,
		"max_daily_loss":      rm.config.MaxDailyLoss,
		"kill_switch_enabled": rm.config.Enabled,
	}
}

// resetDailyMetricsIfNeeded resets daily metrics at day boundary
func (rm *RiskMonitor) resetDailyMetricsIfNeeded() {
	now := time.Now()
	if now.Day() != rm.lastResetTime.Day() || now.Sub(rm.lastResetTime) > 24*time.Hour {
		rm.dailyPnL = make(map[string]float64)
		rm.lastResetTime = now
		// Don't reset violation counts - they decay over time
		for id, count := range rm.violationCounts {
			if count > 0 {
				rm.violationCounts[id] = count - 1
			}
		}
	}
}

// EmergencyStopAll triggers emergency stop for all strategies
func (rm *RiskMonitor) EmergencyStopAll(reason string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// This would trigger emergency stop procedures
	// In a real implementation, this would communicate with the orchestrator
	// to stop all strategies immediately
}

// GetStrategyRiskScore calculates a risk score for a strategy
func (rm *RiskMonitor) GetStrategyRiskScore(instance *StrategyInstance) float64 {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	instance.mu.RLock()
	metrics := instance.Metrics
	instance.mu.RUnlock()

	if metrics == nil {
		return 0.5 // Neutral score for new strategies
	}

	score := 0.0

	// Daily PnL component (0-0.3)
	if dailyPnL, exists := rm.dailyPnL[instance.ID]; exists {
		if dailyPnL > 0 {
			score += 0.3
		} else if dailyPnL > -rm.config.MaxDailyLoss*0.5 {
			score += 0.15
		}
	}

	// Consecutive losses component (0-0.3)
	if metrics.ConsecutiveLosses == 0 {
		score += 0.3
	} else if metrics.ConsecutiveLosses < rm.config.MaxConsecutiveLosses/2 {
		score += 0.15
	}

	// Drawdown component (0-0.2)
	if metrics.MaxDrawdown < rm.config.MaxDrawdown*0.3 {
		score += 0.2
	} else if metrics.MaxDrawdown < rm.config.MaxDrawdown*0.6 {
		score += 0.1
	}

	// Violation history component (0-0.2)
	if violations := rm.violationCounts[instance.ID]; violations == 0 {
		score += 0.2
	} else if violations < 2 {
		score += 0.1
	}

	return score
}

// ShouldReducePosition checks if a strategy should reduce its position size
func (rm *RiskMonitor) ShouldReducePosition(instance *StrategyInstance) (bool, float64) {
	score := rm.GetStrategyRiskScore(instance)

	if score < 0.3 {
		// High risk - reduce position by 50%
		return true, 0.5
	} else if score < 0.5 {
		// Medium risk - reduce position by 25%
		return true, 0.75
	}

	// Low risk - no reduction needed
	return false, 1.0
}