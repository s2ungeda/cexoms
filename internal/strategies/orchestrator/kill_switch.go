package orchestrator

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// KillSwitchReason represents why the kill switch was triggered
type KillSwitchReason string

const (
	ReasonManual           KillSwitchReason = "manual"
	ReasonDrawdown         KillSwitchReason = "drawdown"
	ReasonLossLimit        KillSwitchReason = "loss_limit"
	ReasonSystemError      KillSwitchReason = "system_error"
	ReasonConnectivity     KillSwitchReason = "connectivity"
	ReasonRiskLimit        KillSwitchReason = "risk_limit"
	ReasonTimeSchedule     KillSwitchReason = "time_schedule"
	ReasonEmergency        KillSwitchReason = "emergency"
)

// KillSwitchState represents the current state
type KillSwitchState string

const (
	StateActive   KillSwitchState = "active"
	StateTriggered KillSwitchState = "triggered"
	StateDisabled  KillSwitchState = "disabled"
)

// KillSwitchConfig contains configuration for kill switch
type KillSwitchConfig struct {
	Enabled                bool
	MaxDrawdown           float64       // Maximum allowed drawdown percentage
	MaxDailyLoss         float64       // Maximum daily loss amount
	MaxConsecutiveLosses int           // Maximum consecutive losing trades
	EmergencyStopLoss    float64       // Emergency stop loss percentage
	AutoResetDelay       time.Duration // Auto reset after delay (0 = manual reset only)
	
	// Time-based controls
	TradingHours         []TradingWindow
	MaintenanceWindows   []MaintenanceWindow
	
	// System health checks
	RequireConnectivity  bool
	MaxLatency          time.Duration
	MinFreeMemory       int64  // Minimum free memory in MB
	MaxCPUUsage         float64 // Maximum CPU usage percentage
}

// TradingWindow defines allowed trading hours
type TradingWindow struct {
	Weekday   time.Weekday
	StartTime string // Format: "HH:MM"
	EndTime   string // Format: "HH:MM"
	Timezone  string
}

// MaintenanceWindow defines system maintenance periods
type MaintenanceWindow struct {
	StartTime time.Time
	EndTime   time.Time
	Reason    string
}

// KillSwitch manages emergency stops for all strategies
type KillSwitch struct {
	config           KillSwitchConfig
	state            KillSwitchState
	triggerTime      time.Time
	triggerReason    KillSwitchReason
	triggerMessage   string
	affectedStrategies []string
	logger           *zap.Logger
	mu               sync.RWMutex
	callbacks        []KillSwitchCallback
	resetTimer       *time.Timer
}

// KillSwitchCallback is called when kill switch is triggered
type KillSwitchCallback func(reason KillSwitchReason, message string, strategies []string)

// TriggerEvent represents a kill switch trigger event
type TriggerEvent struct {
	Timestamp    time.Time
	Reason       KillSwitchReason
	Message      string
	StrategyID   string
	Metrics      map[string]float64
}

// NewKillSwitch creates a new kill switch
func NewKillSwitch(logger *zap.Logger) *KillSwitch {
	return &KillSwitch{
		config: KillSwitchConfig{
			Enabled:              true,
			MaxDrawdown:         0.2,    // 20% max drawdown
			MaxDailyLoss:        10000,  // $10k max daily loss
			MaxConsecutiveLosses: 10,    // 10 consecutive losses
			EmergencyStopLoss:   0.3,    // 30% emergency stop
			AutoResetDelay:      1 * time.Hour,
			RequireConnectivity: true,
			MaxLatency:         500 * time.Millisecond,
			MinFreeMemory:      1024,    // 1GB minimum
			MaxCPUUsage:        80.0,    // 80% max CPU
		},
		state:  StateActive,
		logger: logger,
	}
}

// SetConfig updates kill switch configuration
func (ks *KillSwitch) SetConfig(config KillSwitchConfig) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	
	ks.config = config
	
	if !config.Enabled {
		ks.state = StateDisabled
		ks.logger.Info("Kill switch disabled")
	} else if ks.state == StateDisabled {
		ks.state = StateActive
		ks.logger.Info("Kill switch enabled")
	}
}

// IsEnabled returns if kill switch is enabled
func (ks *KillSwitch) IsEnabled() bool {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	
	return ks.config.Enabled && ks.state != StateDisabled
}

// IsTriggered returns if kill switch is currently triggered
func (ks *KillSwitch) IsTriggered() bool {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	
	return ks.state == StateTriggered
}

// Trigger activates the kill switch
func (ks *KillSwitch) Trigger(reason KillSwitchReason, message string) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	
	if ks.state == StateTriggered {
		return // Already triggered
	}
	
	ks.state = StateTriggered
	ks.triggerTime = time.Now()
	ks.triggerReason = reason
	ks.triggerMessage = message
	
	ks.logger.Error("Kill switch triggered",
		zap.String("reason", string(reason)),
		zap.String("message", message),
		zap.Time("trigger_time", ks.triggerTime))
	
	// Execute callbacks
	for _, callback := range ks.callbacks {
		go callback(reason, message, ks.affectedStrategies)
	}
	
	// Set auto-reset timer if configured
	if ks.config.AutoResetDelay > 0 {
		if ks.resetTimer != nil {
			ks.resetTimer.Stop()
		}
		
		ks.resetTimer = time.AfterFunc(ks.config.AutoResetDelay, func() {
			ks.AutoReset()
		})
	}
}

// TriggerForStrategy triggers kill switch for a specific strategy
func (ks *KillSwitch) TriggerForStrategy(strategyID string, reason string) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	
	// Add to affected strategies
	ks.affectedStrategies = append(ks.affectedStrategies, strategyID)
	
	// Trigger if not already triggered
	if ks.state != StateTriggered {
		ks.state = StateTriggered
		ks.triggerTime = time.Now()
		ks.triggerReason = ReasonDrawdown // Default reason
		ks.triggerMessage = fmt.Sprintf("Strategy %s: %s", strategyID, reason)
		
		ks.logger.Error("Kill switch triggered for strategy",
			zap.String("strategy_id", strategyID),
			zap.String("reason", reason))
		
		// Execute callbacks
		for _, callback := range ks.callbacks {
			go callback(ks.triggerReason, ks.triggerMessage, ks.affectedStrategies)
		}
	}
}

// Reset manually resets the kill switch
func (ks *KillSwitch) Reset() error {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	
	if ks.state != StateTriggered {
		return fmt.Errorf("kill switch is not triggered")
	}
	
	ks.state = StateActive
	ks.affectedStrategies = nil
	
	if ks.resetTimer != nil {
		ks.resetTimer.Stop()
		ks.resetTimer = nil
	}
	
	ks.logger.Info("Kill switch reset manually",
		zap.Duration("downtime", time.Since(ks.triggerTime)))
	
	return nil
}

// AutoReset automatically resets the kill switch
func (ks *KillSwitch) AutoReset() {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	
	if ks.state != StateTriggered {
		return
	}
	
	ks.state = StateActive
	ks.affectedStrategies = nil
	
	ks.logger.Info("Kill switch auto-reset",
		zap.Duration("downtime", time.Since(ks.triggerTime)),
		zap.String("original_reason", string(ks.triggerReason)))
}

// CheckDrawdown checks if drawdown exceeds limit
func (ks *KillSwitch) CheckDrawdown(currentDrawdown float64) {
	ks.mu.RLock()
	maxDrawdown := ks.config.MaxDrawdown
	enabled := ks.config.Enabled
	ks.mu.RUnlock()
	
	if !enabled || ks.IsTriggered() {
		return
	}
	
	if currentDrawdown > maxDrawdown {
		ks.Trigger(ReasonDrawdown, 
			fmt.Sprintf("Drawdown %.2f%% exceeds limit %.2f%%", 
				currentDrawdown*100, maxDrawdown*100))
	}
}

// CheckDailyLoss checks if daily loss exceeds limit
func (ks *KillSwitch) CheckDailyLoss(dailyLoss float64) {
	ks.mu.RLock()
	maxLoss := ks.config.MaxDailyLoss
	enabled := ks.config.Enabled
	ks.mu.RUnlock()
	
	if !enabled || ks.IsTriggered() {
		return
	}
	
	if dailyLoss > maxLoss {
		ks.Trigger(ReasonLossLimit,
			fmt.Sprintf("Daily loss $%.2f exceeds limit $%.2f", dailyLoss, maxLoss))
	}
}

// CheckConsecutiveLosses checks consecutive losses
func (ks *KillSwitch) CheckConsecutiveLosses(consecutiveLosses int) {
	ks.mu.RLock()
	maxLosses := ks.config.MaxConsecutiveLosses
	enabled := ks.config.Enabled
	ks.mu.RUnlock()
	
	if !enabled || ks.IsTriggered() {
		return
	}
	
	if consecutiveLosses >= maxLosses {
		ks.Trigger(ReasonLossLimit,
			fmt.Sprintf("%d consecutive losses exceeds limit %d", 
				consecutiveLosses, maxLosses))
	}
}

// CheckTradingHours checks if current time is within trading hours
func (ks *KillSwitch) CheckTradingHours() bool {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	
	if !ks.config.Enabled || len(ks.config.TradingHours) == 0 {
		return true // No restrictions
	}
	
	now := time.Now()
	
	// Check maintenance windows first
	for _, window := range ks.config.MaintenanceWindows {
		if now.After(window.StartTime) && now.Before(window.EndTime) {
			if ks.state != StateTriggered {
				ks.mu.RUnlock()
				ks.Trigger(ReasonTimeSchedule,
					fmt.Sprintf("Maintenance window: %s", window.Reason))
				ks.mu.RLock()
			}
			return false
		}
	}
	
	// Check trading hours
	for _, window := range ks.config.TradingHours {
		if window.Weekday == now.Weekday() {
			// Parse times
			loc, _ := time.LoadLocation(window.Timezone)
			nowInZone := now.In(loc)
			
			startTime, _ := time.Parse("15:04", window.StartTime)
			endTime, _ := time.Parse("15:04", window.EndTime)
			
			currentTime, _ := time.Parse("15:04", nowInZone.Format("15:04"))
			
			if currentTime.After(startTime) && currentTime.Before(endTime) {
				return true
			}
		}
	}
	
	// Outside trading hours
	if ks.state != StateTriggered {
		ks.mu.RUnlock()
		ks.Trigger(ReasonTimeSchedule, "Outside trading hours")
		ks.mu.RLock()
	}
	
	return false
}

// CheckSystemHealth checks system health metrics
func (ks *KillSwitch) CheckSystemHealth(metrics SystemHealthMetrics) {
	ks.mu.RLock()
	config := ks.config
	enabled := config.Enabled
	ks.mu.RUnlock()
	
	if !enabled || ks.IsTriggered() {
		return
	}
	
	// Check connectivity
	if config.RequireConnectivity && !metrics.Connected {
		ks.Trigger(ReasonConnectivity, "Lost connectivity to exchanges")
		return
	}
	
	// Check latency
	if metrics.Latency > config.MaxLatency {
		ks.Trigger(ReasonSystemError,
			fmt.Sprintf("High latency: %s > %s", metrics.Latency, config.MaxLatency))
		return
	}
	
	// Check memory
	if metrics.FreeMemoryMB < config.MinFreeMemory {
		ks.Trigger(ReasonSystemError,
			fmt.Sprintf("Low memory: %dMB < %dMB", 
				metrics.FreeMemoryMB, config.MinFreeMemory))
		return
	}
	
	// Check CPU
	if metrics.CPUUsage > config.MaxCPUUsage {
		ks.Trigger(ReasonSystemError,
			fmt.Sprintf("High CPU usage: %.2f%% > %.2f%%", 
				metrics.CPUUsage, config.MaxCPUUsage))
		return
	}
}

// RegisterCallback registers a callback for kill switch triggers
func (ks *KillSwitch) RegisterCallback(callback KillSwitchCallback) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	
	ks.callbacks = append(ks.callbacks, callback)
}

// GetStatus returns current kill switch status
func (ks *KillSwitch) GetStatus() map[string]interface{} {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	
	status := map[string]interface{}{
		"enabled":    ks.config.Enabled,
		"state":      ks.state,
		"config":     ks.config,
	}
	
	if ks.state == StateTriggered {
		status["trigger_time"] = ks.triggerTime
		status["trigger_reason"] = ks.triggerReason
		status["trigger_message"] = ks.triggerMessage
		status["affected_strategies"] = ks.affectedStrategies
		status["downtime"] = time.Since(ks.triggerTime).String()
		
		if ks.config.AutoResetDelay > 0 {
			remainingTime := ks.config.AutoResetDelay - time.Since(ks.triggerTime)
			if remainingTime > 0 {
				status["auto_reset_in"] = remainingTime.String()
			}
		}
	}
	
	return status
}

// SystemHealthMetrics contains system health information
type SystemHealthMetrics struct {
	Connected     bool
	Latency       time.Duration
	FreeMemoryMB  int64
	CPUUsage      float64
	DiskUsagePC   float64
	OpenOrders    int
	ActiveThreads int
	ErrorRate     float64
}

// EmergencyStop immediately stops all trading
func (ks *KillSwitch) EmergencyStop(reason string) {
	ks.mu.Lock()
	
	// Force trigger regardless of current state
	ks.state = StateTriggered
	ks.triggerTime = time.Now()
	ks.triggerReason = ReasonEmergency
	ks.triggerMessage = reason
	
	// Cancel auto-reset for emergency stops
	if ks.resetTimer != nil {
		ks.resetTimer.Stop()
		ks.resetTimer = nil
	}
	
	ks.logger.Error("EMERGENCY STOP ACTIVATED",
		zap.String("reason", reason),
		zap.Time("timestamp", ks.triggerTime))
	
	// Execute callbacks
	callbacks := ks.callbacks
	strategies := ks.affectedStrategies
	ks.mu.Unlock()
	
	// Execute callbacks outside of lock
	for _, callback := range callbacks {
		callback(ReasonEmergency, reason, strategies)
	}
}