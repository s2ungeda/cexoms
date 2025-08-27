package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// AutoControlConfig contains configuration for automatic controls
type AutoControlConfig struct {
	Enabled               bool
	CheckInterval         time.Duration
	
	// Conditional controls
	EnableConditionalStart bool
	EnableConditionalStop  bool
	EnableScheduledControl bool
	
	// Performance-based controls
	StopOnDrawdown        float64 // Stop if drawdown exceeds this
	StopOnLossStreak      int     // Stop after N consecutive losses
	PauseOnHighVolatility float64 // Pause if volatility exceeds this
	
	// Resource-based controls
	PauseOnLowMemory      int64   // Pause if free memory < MB
	PauseOnHighCPU        float64 // Pause if CPU usage > %
	
	// Time-based controls
	DailyStartTime        string  // Format: "HH:MM"
	DailyStopTime         string  // Format: "HH:MM"
	WeekendStop           bool    // Stop on weekends
	Timezone              string
}

// ControlCondition defines conditions for automatic control
type ControlCondition struct {
	ID          string
	Type        string // "market", "performance", "time", "system"
	Metric      string
	Operator    string // ">", "<", "==", "!=", "between"
	Threshold   interface{}
	Action      ControlAction
	StrategyID  string // Empty means applies to all
	Priority    int
	LastChecked time.Time
}

// ControlAction defines what action to take
type ControlAction string

const (
	ActionStart       ControlAction = "start"
	ActionStop        ControlAction = "stop"
	ActionPause       ControlAction = "pause"
	ActionResume      ControlAction = "resume"
	ActionScaleUp     ControlAction = "scale_up"
	ActionScaleDown   ControlAction = "scale_down"
	ActionRebalance   ControlAction = "rebalance"
)

// AutoController manages automatic control of strategies
type AutoController struct {
	config           AutoControlConfig
	conditions       []ControlCondition
	orchestrator     *Orchestrator
	performanceMonitor *PerformanceMonitor
	killSwitch       *KillSwitch
	logger           *zap.Logger
	mu               sync.RWMutex
	ctx              context.Context
	cancel           context.CancelFunc
	checkTicker      *time.Ticker
}

// NewAutoController creates a new auto controller
func NewAutoController(orchestrator *Orchestrator, perfMonitor *PerformanceMonitor, 
	killSwitch *KillSwitch, logger *zap.Logger) *AutoController {
	
	ctx, cancel := context.WithCancel(context.Background())
	
	return &AutoController{
		config: AutoControlConfig{
			Enabled:               true,
			CheckInterval:         30 * time.Second,
			EnableConditionalStart: true,
			EnableConditionalStop:  true,
			EnableScheduledControl: true,
			StopOnDrawdown:        0.15,  // 15% drawdown
			StopOnLossStreak:      7,     // 7 consecutive losses
			PauseOnHighVolatility: 0.5,   // 50% volatility
			PauseOnLowMemory:      512,   // 512MB
			PauseOnHighCPU:        90.0,  // 90% CPU
			WeekendStop:           true,
			Timezone:              "UTC",
		},
		conditions:         DefaultControlConditions(),
		orchestrator:      orchestrator,
		performanceMonitor: perfMonitor,
		killSwitch:        killSwitch,
		logger:            logger,
		ctx:               ctx,
		cancel:            cancel,
	}
}

// Start begins automatic control monitoring
func (ac *AutoController) Start() {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	
	if !ac.config.Enabled {
		ac.logger.Info("Auto controller disabled")
		return
	}
	
	ac.checkTicker = time.NewTicker(ac.config.CheckInterval)
	
	go ac.monitorLoop()
	
	ac.logger.Info("Auto controller started",
		zap.Duration("check_interval", ac.config.CheckInterval))
}

// Stop stops the auto controller
func (ac *AutoController) Stop() {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	
	if ac.checkTicker != nil {
		ac.checkTicker.Stop()
	}
	
	ac.cancel()
	ac.logger.Info("Auto controller stopped")
}

// monitorLoop continuously monitors conditions
func (ac *AutoController) monitorLoop() {
	for {
		select {
		case <-ac.ctx.Done():
			return
		case <-ac.checkTicker.C:
			ac.checkAllConditions()
		}
	}
}

// checkAllConditions checks all control conditions
func (ac *AutoController) checkAllConditions() {
	ac.mu.RLock()
	conditions := make([]ControlCondition, len(ac.conditions))
	copy(conditions, ac.conditions)
	ac.mu.RUnlock()
	
	// Check scheduled controls first
	if ac.config.EnableScheduledControl {
		ac.checkScheduledControls()
	}
	
	// Check each condition
	for i := range conditions {
		condition := &conditions[i]
		
		if ac.evaluateCondition(condition) {
			ac.executeAction(condition)
		}
		
		condition.LastChecked = time.Now()
	}
	
	// Check kill switch conditions
	ac.checkKillSwitchConditions()
}

// evaluateCondition evaluates if a condition is met
func (ac *AutoController) evaluateCondition(condition *ControlCondition) bool {
	switch condition.Type {
	case "market":
		return ac.evaluateMarketCondition(condition)
	case "performance":
		return ac.evaluatePerformanceCondition(condition)
	case "time":
		return ac.evaluateTimeCondition(condition)
	case "system":
		return ac.evaluateSystemCondition(condition)
	default:
		return false
	}
}

// evaluatePerformanceCondition checks performance metrics
func (ac *AutoController) evaluatePerformanceCondition(condition *ControlCondition) bool {
	// Get performance metrics
	metrics, err := ac.performanceMonitor.GetMetrics(condition.StrategyID, "24h")
	if err != nil {
		return false
	}
	
	var value float64
	switch condition.Metric {
	case "drawdown":
		value = metrics.CurrentDrawdown
	case "sharpe_ratio":
		value = metrics.SharpeRatio
	case "win_rate":
		value = metrics.WinRate
	case "profit_factor":
		value = metrics.ProfitFactor
	case "consecutive_losses":
		// Custom logic to count consecutive losses
		value = float64(ac.getConsecutiveLosses(condition.StrategyID))
	case "daily_pnl":
		value = metrics.RealizedPnL
	case "volatility":
		value = metrics.Volatility
	default:
		return false
	}
	
	return ac.compareValue(value, condition.Operator, condition.Threshold)
}

// evaluateMarketCondition checks market conditions
func (ac *AutoController) evaluateMarketCondition(condition *ControlCondition) bool {
	// This would integrate with market data feeds
	// For now, return false
	return false
}

// evaluateTimeCondition checks time-based conditions
func (ac *AutoController) evaluateTimeCondition(condition *ControlCondition) bool {
	now := time.Now()
	
	switch condition.Metric {
	case "hour_of_day":
		threshold, ok := condition.Threshold.(int)
		if !ok {
			return false
		}
		return ac.compareValue(float64(now.Hour()), condition.Operator, threshold)
		
	case "day_of_week":
		threshold, ok := condition.Threshold.(int)
		if !ok {
			return false
		}
		return ac.compareValue(float64(now.Weekday()), condition.Operator, threshold)
		
	case "time_since_start":
		// Get strategy start time
		strategy, err := ac.orchestrator.GetStrategy(condition.StrategyID)
		if err != nil {
			return false
		}
		
		duration := now.Sub(strategy.StartTime)
		threshold, ok := condition.Threshold.(time.Duration)
		if !ok {
			return false
		}
		
		return ac.compareValue(duration.Seconds(), condition.Operator, threshold.Seconds())
		
	default:
		return false
	}
}

// evaluateSystemCondition checks system health conditions
func (ac *AutoController) evaluateSystemCondition(condition *ControlCondition) bool {
	// This would integrate with system monitoring
	// For demonstration, using mock values
	var value float64
	
	switch condition.Metric {
	case "free_memory":
		// Get actual system memory
		value = 2048 // Mock: 2GB free
	case "cpu_usage":
		// Get actual CPU usage
		value = 45.0 // Mock: 45% CPU
	case "error_rate":
		// Calculate error rate
		value = 0.01 // Mock: 1% error rate
	default:
		return false
	}
	
	return ac.compareValue(value, condition.Operator, condition.Threshold)
}

// compareValue compares values based on operator
func (ac *AutoController) compareValue(value float64, operator string, threshold interface{}) bool {
	thresholdFloat, ok := ac.toFloat64(threshold)
	if !ok {
		return false
	}
	
	switch operator {
	case ">":
		return value > thresholdFloat
	case "<":
		return value < thresholdFloat
	case ">=":
		return value >= thresholdFloat
	case "<=":
		return value <= thresholdFloat
	case "==":
		return value == thresholdFloat
	case "!=":
		return value != thresholdFloat
	default:
		return false
	}
}

// toFloat64 converts interface to float64
func (ac *AutoController) toFloat64(val interface{}) (float64, bool) {
	switch v := val.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float32:
		return float64(v), true
	default:
		return 0, false
	}
}

// executeAction executes the control action
func (ac *AutoController) executeAction(condition *ControlCondition) {
	ac.logger.Info("Executing auto control action",
		zap.String("condition_id", condition.ID),
		zap.String("action", string(condition.Action)),
		zap.String("strategy_id", condition.StrategyID))
	
	var err error
	
	switch condition.Action {
	case ActionStart:
		if condition.StrategyID != "" {
			err = ac.orchestrator.StartStrategy(condition.StrategyID)
		}
		
	case ActionStop:
		if condition.StrategyID != "" {
			err = ac.orchestrator.StopStrategy(condition.StrategyID, false)
		} else {
			// Stop all strategies
			for _, strategy := range ac.orchestrator.GetStrategies() {
				if strategy.State == StateRunning {
					ac.orchestrator.StopStrategy(strategy.ID, false)
				}
			}
		}
		
	case ActionPause:
		if condition.StrategyID != "" {
			err = ac.orchestrator.PauseStrategy(condition.StrategyID)
		}
		
	case ActionResume:
		if condition.StrategyID != "" {
			err = ac.orchestrator.ResumeStrategy(condition.StrategyID)
		}
		
	case ActionScaleUp:
		// Increase capital allocation
		ac.scaleStrategy(condition.StrategyID, 1.5) // 50% increase
		
	case ActionScaleDown:
		// Decrease capital allocation
		ac.scaleStrategy(condition.StrategyID, 0.5) // 50% decrease
		
	case ActionRebalance:
		// Trigger rebalancing
		ac.triggerRebalance()
	}
	
	if err != nil {
		ac.logger.Error("Failed to execute action",
			zap.Error(err),
			zap.String("action", string(condition.Action)))
	}
}

// checkScheduledControls checks time-based scheduled controls
func (ac *AutoController) checkScheduledControls() {
	if !ac.config.EnableScheduledControl {
		return
	}
	
	now := time.Now()
	loc, _ := time.LoadLocation(ac.config.Timezone)
	nowInZone := now.In(loc)
	
	// Check weekend stop
	if ac.config.WeekendStop && (nowInZone.Weekday() == time.Saturday || nowInZone.Weekday() == time.Sunday) {
		// Stop all strategies on weekends
		for _, strategy := range ac.orchestrator.GetStrategies() {
			if strategy.State == StateRunning {
				ac.orchestrator.StopStrategy(strategy.ID, false)
				ac.logger.Info("Weekend stop activated", zap.String("strategy_id", strategy.ID))
			}
		}
		return
	}
	
	// Check daily schedule
	if ac.config.DailyStartTime != "" && ac.config.DailyStopTime != "" {
		currentTime, _ := time.Parse("15:04", nowInZone.Format("15:04"))
		startTime, _ := time.Parse("15:04", ac.config.DailyStartTime)
		stopTime, _ := time.Parse("15:04", ac.config.DailyStopTime)
		
		// Check if we should start strategies
		if currentTime.After(startTime) && currentTime.Before(stopTime) {
			for _, strategy := range ac.orchestrator.GetStrategies() {
				if strategy.State == StateStopped {
					ac.orchestrator.StartStrategy(strategy.ID)
					ac.logger.Info("Daily start activated", zap.String("strategy_id", strategy.ID))
				}
			}
		} else {
			// Outside trading hours - stop strategies
			for _, strategy := range ac.orchestrator.GetStrategies() {
				if strategy.State == StateRunning {
					ac.orchestrator.StopStrategy(strategy.ID, false)
					ac.logger.Info("Daily stop activated", zap.String("strategy_id", strategy.ID))
				}
			}
		}
	}
}

// checkKillSwitchConditions checks conditions that might trigger kill switch
func (ac *AutoController) checkKillSwitchConditions() {
	// Check each running strategy
	for _, strategy := range ac.orchestrator.GetStrategies() {
		if strategy.State != StateRunning {
			continue
		}
		
		// Get performance metrics
		metrics, err := ac.performanceMonitor.GetMetrics(strategy.ID, "24h")
		if err != nil {
			continue
		}
		
		// Check drawdown
		if ac.config.StopOnDrawdown > 0 && metrics.CurrentDrawdown > ac.config.StopOnDrawdown {
			ac.killSwitch.TriggerForStrategy(strategy.ID, 
				fmt.Sprintf("Drawdown %.2f%% exceeds limit", metrics.CurrentDrawdown*100))
		}
		
		// Check loss streak
		consecutiveLosses := ac.getConsecutiveLosses(strategy.ID)
		if ac.config.StopOnLossStreak > 0 && consecutiveLosses >= ac.config.StopOnLossStreak {
			ac.killSwitch.TriggerForStrategy(strategy.ID,
				fmt.Sprintf("%d consecutive losses", consecutiveLosses))
		}
		
		// Check volatility
		if ac.config.PauseOnHighVolatility > 0 && metrics.Volatility > ac.config.PauseOnHighVolatility {
			ac.orchestrator.PauseStrategy(strategy.ID)
			ac.logger.Warn("Strategy paused due to high volatility",
				zap.String("strategy_id", strategy.ID),
				zap.Float64("volatility", metrics.Volatility))
		}
	}
}

// getConsecutiveLosses gets consecutive loss count for a strategy
func (ac *AutoController) getConsecutiveLosses(strategyID string) int {
	// This would be implemented based on trade history
	// For now, return mock value
	return 0
}

// scaleStrategy scales capital allocation for a strategy
func (ac *AutoController) scaleStrategy(strategyID string, factor float64) {
	allocation, err := ac.orchestrator.capitalAllocator.GetAllocation(strategyID)
	if err != nil {
		ac.logger.Error("Failed to get allocation",
			zap.String("strategy_id", strategyID),
			zap.Error(err))
		return
	}
	
	newAmount := allocation.AllocatedAmount * factor
	
	// Update allocation
	ac.logger.Info("Scaling strategy allocation",
		zap.String("strategy_id", strategyID),
		zap.Float64("old_amount", allocation.AllocatedAmount),
		zap.Float64("new_amount", newAmount),
		zap.Float64("factor", factor))
}

// triggerRebalance triggers portfolio rebalancing
func (ac *AutoController) triggerRebalance() {
	ac.logger.Info("Triggering portfolio rebalance")
	// This would trigger the capital allocator's rebalance function
}

// AddCondition adds a new control condition
func (ac *AutoController) AddCondition(condition ControlCondition) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	
	ac.conditions = append(ac.conditions, condition)
	
	ac.logger.Info("Added control condition",
		zap.String("condition_id", condition.ID),
		zap.String("type", condition.Type),
		zap.String("action", string(condition.Action)))
}

// RemoveCondition removes a control condition
func (ac *AutoController) RemoveCondition(conditionID string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	
	for i, condition := range ac.conditions {
		if condition.ID == conditionID {
			ac.conditions = append(ac.conditions[:i], ac.conditions[i+1:]...)
			ac.logger.Info("Removed control condition", zap.String("condition_id", conditionID))
			return
		}
	}
}

// SetConfig updates the auto controller configuration
func (ac *AutoController) SetConfig(config AutoControlConfig) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	
	ac.config = config
	
	// Restart ticker if interval changed
	if ac.checkTicker != nil {
		ac.checkTicker.Stop()
		ac.checkTicker = time.NewTicker(config.CheckInterval)
	}
	
	ac.logger.Info("Updated auto controller config")
}

// GetStatus returns current auto controller status
func (ac *AutoController) GetStatus() map[string]interface{} {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	
	return map[string]interface{}{
		"enabled":         ac.config.Enabled,
		"conditions":      len(ac.conditions),
		"check_interval":  ac.config.CheckInterval,
		"config":         ac.config,
		"active_controls": ac.getActiveControls(),
	}
}

// getActiveControls returns currently active control conditions
func (ac *AutoController) getActiveControls() []string {
	var active []string
	
	for _, condition := range ac.conditions {
		if time.Since(condition.LastChecked) < ac.config.CheckInterval*2 {
			active = append(active, condition.ID)
		}
	}
	
	return active
}

// DefaultControlConditions returns default control conditions
func DefaultControlConditions() []ControlCondition {
	return []ControlCondition{
		{
			ID:        "high_drawdown_stop",
			Type:      "performance",
			Metric:    "drawdown",
			Operator:  ">",
			Threshold: 0.15, // 15% drawdown
			Action:    ActionStop,
			Priority:  1,
		},
		{
			ID:        "low_sharpe_pause",
			Type:      "performance",
			Metric:    "sharpe_ratio",
			Operator:  "<",
			Threshold: 0.5,
			Action:    ActionPause,
			Priority:  2,
		},
		{
			ID:        "loss_streak_stop",
			Type:      "performance",
			Metric:    "consecutive_losses",
			Operator:  ">",
			Threshold: 7,
			Action:    ActionStop,
			Priority:  1,
		},
		{
			ID:        "high_volatility_scale_down",
			Type:      "performance",
			Metric:    "volatility",
			Operator:  ">",
			Threshold: 0.5, // 50% volatility
			Action:    ActionScaleDown,
			Priority:  2,
		},
		{
			ID:        "weekend_stop",
			Type:      "time",
			Metric:    "day_of_week",
			Operator:  ">=",
			Threshold: 6, // Saturday
			Action:    ActionStop,
			Priority:  3,
		},
		{
			ID:        "low_memory_pause",
			Type:      "system",
			Metric:    "free_memory",
			Operator:  "<",
			Threshold: 512, // 512MB
			Action:    ActionPause,
			Priority:  1,
		},
		{
			ID:        "high_cpu_pause",
			Type:      "system",
			Metric:    "cpu_usage",
			Operator:  ">",
			Threshold: 90.0, // 90% CPU
			Action:    ActionPause,
			Priority:  1,
		},
	}
}