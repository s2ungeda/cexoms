package strategies

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// ParameterUpdate represents a parameter update request
type ParameterUpdate struct {
	ParameterName string
	OldValue      interface{}
	NewValue      interface{}
	Timestamp     time.Time
	RequestID     string
	Status        UpdateStatus
	Error         error
}

// UpdateStatus represents the status of a parameter update
type UpdateStatus string

const (
	UpdateStatusPending   UpdateStatus = "pending"
	UpdateStatusValidating UpdateStatus = "validating"
	UpdateStatusApplying  UpdateStatus = "applying"
	UpdateStatusCompleted UpdateStatus = "completed"
	UpdateStatusFailed    UpdateStatus = "failed"
	UpdateStatusRolledBack UpdateStatus = "rolled_back"
)

// ValidationRule defines a rule for parameter validation
type ValidationRule struct {
	Name      string
	Validator func(oldValue, newValue interface{}) error
}

// RealtimeModifier manages real-time strategy parameter updates
type RealtimeModifier struct {
	mu sync.RWMutex
	
	// Strategy reference
	strategy Strategy
	
	// Update tracking
	pendingUpdates   map[string]*ParameterUpdate
	updateHistory    []ParameterUpdate
	
	// Validation rules
	validationRules  map[string][]ValidationRule
	
	// Safety mechanisms
	maxUpdateRate    time.Duration // Minimum time between updates
	lastUpdateTime   map[string]time.Time
	rollbackEnabled  bool
	
	// Monitoring
	updateCallbacks  []func(update *ParameterUpdate)
	
	// Logger
	logger *zap.Logger
	
	// Context for lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewRealtimeModifier creates a new real-time modifier
func NewRealtimeModifier(strategy Strategy, logger *zap.Logger) *RealtimeModifier {
	return &RealtimeModifier{
		strategy:         strategy,
		pendingUpdates:   make(map[string]*ParameterUpdate),
		updateHistory:    make([]ParameterUpdate, 0),
		validationRules:  make(map[string][]ValidationRule),
		lastUpdateTime:   make(map[string]time.Time),
		maxUpdateRate:    5 * time.Second, // Default 5 seconds between updates
		rollbackEnabled:  true,
		updateCallbacks:  make([]func(update *ParameterUpdate), 0),
		logger:           logger,
	}
}

// Start starts the real-time modifier
func (m *RealtimeModifier) Start(ctx context.Context) error {
	m.mu.Lock()
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.mu.Unlock()
	
	// Start update processor
	m.wg.Add(1)
	go m.processUpdates()
	
	m.logger.Info("Real-time modifier started")
	return nil
}

// Stop stops the real-time modifier
func (m *RealtimeModifier) Stop() error {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Unlock()
	
	m.wg.Wait()
	
	m.logger.Info("Real-time modifier stopped")
	return nil
}

// UpdateParameter requests a parameter update
func (m *RealtimeModifier) UpdateParameter(paramName string, newValue interface{}) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Check update rate limit
	if lastUpdate, exists := m.lastUpdateTime[paramName]; exists {
		if time.Since(lastUpdate) < m.maxUpdateRate {
			return "", fmt.Errorf("update rate limit exceeded for %s, please wait %v",
				paramName, m.maxUpdateRate-time.Since(lastUpdate))
		}
	}
	
	// Get current value
	currentParams := m.strategy.GetParameters()
	oldValue, exists := currentParams[paramName]
	if !exists {
		return "", fmt.Errorf("parameter %s does not exist", paramName)
	}
	
	// Create update request
	updateID := fmt.Sprintf("update-%s-%d", paramName, time.Now().UnixNano())
	update := &ParameterUpdate{
		ParameterName: paramName,
		OldValue:      oldValue,
		NewValue:      newValue,
		Timestamp:     time.Now(),
		RequestID:     updateID,
		Status:        UpdateStatusPending,
	}
	
	m.pendingUpdates[updateID] = update
	
	m.logger.Info("Parameter update requested",
		zap.String("parameter", paramName),
		zap.Any("old_value", oldValue),
		zap.Any("new_value", newValue),
		zap.String("request_id", updateID))
	
	return updateID, nil
}

// processUpdates processes pending parameter updates
func (m *RealtimeModifier) processUpdates() {
	defer m.wg.Done()
	
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.processPendingUpdates()
		}
	}
}

// processPendingUpdates processes all pending updates
func (m *RealtimeModifier) processPendingUpdates() {
	m.mu.Lock()
	// Get pending updates
	updates := make([]*ParameterUpdate, 0)
	for _, update := range m.pendingUpdates {
		if update.Status == UpdateStatusPending {
			updates = append(updates, update)
		}
	}
	m.mu.Unlock()
	
	// Process each update
	for _, update := range updates {
		m.processUpdate(update)
	}
}

// processUpdate processes a single parameter update
func (m *RealtimeModifier) processUpdate(update *ParameterUpdate) {
	// Update status
	m.updateStatus(update, UpdateStatusValidating)
	
	// Validate update
	if err := m.validateUpdate(update); err != nil {
		m.updateStatus(update, UpdateStatusFailed)
		update.Error = err
		m.completeUpdate(update)
		return
	}
	
	// Check if strategy can be safely updated
	if !m.isSafeToUpdate() {
		// Queue for later
		m.logger.Info("Deferring update - strategy busy",
			zap.String("parameter", update.ParameterName))
		return
	}
	
	// Apply update
	m.updateStatus(update, UpdateStatusApplying)
	
	// Store pre-update metrics for rollback
	preUpdateMetrics := m.strategy.GetMetrics()
	
	// Apply the update
	params := map[string]interface{}{
		update.ParameterName: update.NewValue,
	}
	
	if err := m.strategy.UpdateParameters(params); err != nil {
		m.updateStatus(update, UpdateStatusFailed)
		update.Error = err
		m.completeUpdate(update)
		return
	}
	
	// Monitor for issues
	if m.rollbackEnabled {
		go m.monitorUpdate(update, preUpdateMetrics)
	} else {
		m.updateStatus(update, UpdateStatusCompleted)
		m.completeUpdate(update)
	}
}

// validateUpdate validates a parameter update
func (m *RealtimeModifier) validateUpdate(update *ParameterUpdate) error {
	// Type validation
	if err := m.validateType(update.ParameterName, update.OldValue, update.NewValue); err != nil {
		return fmt.Errorf("type validation failed: %w", err)
	}
	
	// Custom validation rules
	m.mu.RLock()
	rules, exists := m.validationRules[update.ParameterName]
	m.mu.RUnlock()
	
	if exists {
		for _, rule := range rules {
			if err := rule.Validator(update.OldValue, update.NewValue); err != nil {
				return fmt.Errorf("validation rule '%s' failed: %w", rule.Name, err)
			}
		}
	}
	
	// Strategy-specific validation
	if err := m.strategy.ValidateParameters(map[string]interface{}{
		update.ParameterName: update.NewValue,
	}); err != nil {
		return fmt.Errorf("strategy validation failed: %w", err)
	}
	
	return nil
}

// validateType validates parameter type compatibility
func (m *RealtimeModifier) validateType(paramName string, oldValue, newValue interface{}) error {
	// Check if types match
	oldType := fmt.Sprintf("%T", oldValue)
	newType := fmt.Sprintf("%T", newValue)
	
	if oldType != newType {
		// Allow numeric conversions
		if isNumeric(oldValue) && isNumeric(newValue) {
			return nil
		}
		return fmt.Errorf("type mismatch: expected %s, got %s", oldType, newType)
	}
	
	return nil
}

// isSafeToUpdate checks if it's safe to update parameters
func (m *RealtimeModifier) isSafeToUpdate() bool {
	// Check if strategy has pending orders
	metrics := m.strategy.GetMetrics()
	if metrics.ActiveOrders > 0 {
		return false
	}
	
	// Check if risk limits are being approached
	if err := m.strategy.CheckRiskLimits(); err != nil {
		return false
	}
	
	return true
}

// monitorUpdate monitors an update for issues and rollback if needed
func (m *RealtimeModifier) monitorUpdate(update *ParameterUpdate, preUpdateMetrics StrategyMetrics) {
	// Monitor for 30 seconds
	monitorDuration := 30 * time.Second
	checkInterval := 1 * time.Second
	
	startTime := time.Now()
	
	for time.Since(startTime) < monitorDuration {
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(checkInterval):
			// Check metrics
			currentMetrics := m.strategy.GetMetrics()
			
			// Check for significant degradation
			if m.shouldRollback(preUpdateMetrics, currentMetrics) {
				m.logger.Warn("Rolling back parameter update due to performance degradation",
					zap.String("parameter", update.ParameterName),
					zap.Any("old_value", update.OldValue),
					zap.Any("new_value", update.NewValue))
				
				// Rollback
				m.rollbackUpdate(update)
				return
			}
		}
	}
	
	// Update successful
	m.updateStatus(update, UpdateStatusCompleted)
	m.completeUpdate(update)
}

// shouldRollback determines if an update should be rolled back
func (m *RealtimeModifier) shouldRollback(preMetrics, postMetrics StrategyMetrics) bool {
	// Check for significant PnL degradation
	if postMetrics.RealizedPnL.LessThan(preMetrics.RealizedPnL.Sub(decimal.NewFromFloat(1000))) {
		return true
	}
	
	// Check for significant drawdown increase
	if postMetrics.MaxDrawdown.GreaterThan(preMetrics.MaxDrawdown.Mul(decimal.NewFromFloat(1.5))) {
		return true
	}
	
	// Check for error rate spike
	preErrorRate := float64(preMetrics.FailedOrders) / float64(preMetrics.TotalOrders+1)
	postErrorRate := float64(postMetrics.FailedOrders) / float64(postMetrics.TotalOrders+1)
	
	if postErrorRate > preErrorRate*2 && postErrorRate > 0.1 {
		return true
	}
	
	return false
}

// rollbackUpdate rolls back a parameter update
func (m *RealtimeModifier) rollbackUpdate(update *ParameterUpdate) {
	params := map[string]interface{}{
		update.ParameterName: update.OldValue,
	}
	
	if err := m.strategy.UpdateParameters(params); err != nil {
		m.logger.Error("Failed to rollback parameter update",
			zap.String("parameter", update.ParameterName),
			zap.Error(err))
		update.Error = fmt.Errorf("rollback failed: %w", err)
		m.updateStatus(update, UpdateStatusFailed)
	} else {
		m.updateStatus(update, UpdateStatusRolledBack)
	}
	
	m.completeUpdate(update)
}

// completeUpdate completes an update and moves it to history
func (m *RealtimeModifier) completeUpdate(update *ParameterUpdate) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Remove from pending
	delete(m.pendingUpdates, update.RequestID)
	
	// Add to history
	m.updateHistory = append(m.updateHistory, *update)
	
	// Update last update time
	m.lastUpdateTime[update.ParameterName] = time.Now()
	
	// Notify callbacks
	for _, callback := range m.updateCallbacks {
		go callback(update)
	}
	
	// Log result
	if update.Status == UpdateStatusCompleted {
		m.logger.Info("Parameter update completed",
			zap.String("parameter", update.ParameterName),
			zap.Any("new_value", update.NewValue),
			zap.String("request_id", update.RequestID))
	} else {
		m.logger.Error("Parameter update failed",
			zap.String("parameter", update.ParameterName),
			zap.String("status", string(update.Status)),
			zap.Error(update.Error),
			zap.String("request_id", update.RequestID))
	}
}

// updateStatus updates the status of an update
func (m *RealtimeModifier) updateStatus(update *ParameterUpdate, status UpdateStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	update.Status = status
}

// AddValidationRule adds a validation rule for a parameter
func (m *RealtimeModifier) AddValidationRule(paramName string, rule ValidationRule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.validationRules[paramName]; !exists {
		m.validationRules[paramName] = make([]ValidationRule, 0)
	}
	
	m.validationRules[paramName] = append(m.validationRules[paramName], rule)
}

// AddUpdateCallback adds a callback for parameter updates
func (m *RealtimeModifier) AddUpdateCallback(callback func(update *ParameterUpdate)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.updateCallbacks = append(m.updateCallbacks, callback)
}

// GetUpdateStatus returns the status of an update request
func (m *RealtimeModifier) GetUpdateStatus(requestID string) (*ParameterUpdate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Check pending updates
	if update, exists := m.pendingUpdates[requestID]; exists {
		updateCopy := *update
		return &updateCopy, nil
	}
	
	// Check history
	for i := len(m.updateHistory) - 1; i >= 0; i-- {
		if m.updateHistory[i].RequestID == requestID {
			updateCopy := m.updateHistory[i]
			return &updateCopy, nil
		}
	}
	
	return nil, fmt.Errorf("update request %s not found", requestID)
}

// GetUpdateHistory returns recent update history
func (m *RealtimeModifier) GetUpdateHistory(limit int) []ParameterUpdate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	start := len(m.updateHistory) - limit
	if start < 0 {
		start = 0
	}
	
	history := make([]ParameterUpdate, 0, limit)
	for i := start; i < len(m.updateHistory); i++ {
		history = append(history, m.updateHistory[i])
	}
	
	return history
}

// SetMaxUpdateRate sets the maximum update rate
func (m *RealtimeModifier) SetMaxUpdateRate(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.maxUpdateRate = duration
}

// SetRollbackEnabled enables or disables automatic rollback
func (m *RealtimeModifier) SetRollbackEnabled(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.rollbackEnabled = enabled
}

// Helper function to check if a value is numeric
func isNumeric(value interface{}) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, decimal.Decimal:
		return true
	default:
		return false
	}
}