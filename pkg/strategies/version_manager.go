package strategies

import (
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// StrategyVersion represents a version of strategy parameters
type StrategyVersion struct {
	VersionID    string
	StrategyID   string
	Parameters   map[string]interface{}
	Description  string
	CreatedAt    time.Time
	CreatedBy    string
	Performance  *VersionPerformance
	Tags         []string
}

// VersionPerformance tracks performance of a version
type VersionPerformance struct {
	TotalReturn     decimal.Decimal
	SharpeRatio     decimal.Decimal
	MaxDrawdown     decimal.Decimal
	WinRate         float64
	TotalTrades     int
	ActiveDuration  time.Duration
	LastUpdated     time.Time
}

// DeploymentStatus represents deployment state
type DeploymentStatus string

const (
	DeploymentStatusActive    DeploymentStatus = "active"
	DeploymentStatusInactive  DeploymentStatus = "inactive"
	DeploymentStatusTesting   DeploymentStatus = "testing"
	DeploymentStatusRollback  DeploymentStatus = "rollback"
)

// VersionDeployment represents a deployment of a version
type VersionDeployment struct {
	DeploymentID  string
	VersionID     string
	Status        DeploymentStatus
	StartTime     time.Time
	EndTime       *time.Time
	Reason        string
}

// VersionManager manages strategy versions and deployments
type VersionManager struct {
	mu sync.RWMutex
	
	// Version storage
	versions        map[string]*StrategyVersion
	deployments     map[string]*VersionDeployment
	activeVersions  map[string]string // strategyID -> versionID
	
	// Performance tracking
	performanceTracker map[string]*PerformanceTracker
	
	// Safety settings
	maxVersions     int
	autoRollback    bool
	testDuration    time.Duration
	
	// Logger
	logger         *zap.Logger
}

// PerformanceTracker tracks version performance
type PerformanceTracker struct {
	versionID      string
	startMetrics   StrategyMetrics
	currentMetrics StrategyMetrics
	startTime      time.Time
}

// NewVersionManager creates a new version manager
func NewVersionManager(logger *zap.Logger) *VersionManager {
	return &VersionManager{
		versions:           make(map[string]*StrategyVersion),
		deployments:        make(map[string]*VersionDeployment),
		activeVersions:     make(map[string]string),
		performanceTracker: make(map[string]*PerformanceTracker),
		maxVersions:        100,
		autoRollback:       true,
		testDuration:       30 * time.Minute,
		logger:             logger,
	}
}

// CreateVersion creates a new version of strategy parameters
func (v *VersionManager) CreateVersion(
	strategyID string,
	parameters map[string]interface{},
	description string,
	createdBy string,
	tags []string,
) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	
	// Generate version ID
	versionID := fmt.Sprintf("%s-v%d", strategyID, time.Now().Unix())
	
	// Create version
	version := &StrategyVersion{
		VersionID:   versionID,
		StrategyID:  strategyID,
		Parameters:  v.copyParameters(parameters),
		Description: description,
		CreatedAt:   time.Now(),
		CreatedBy:   createdBy,
		Tags:        tags,
		Performance: &VersionPerformance{
			TotalReturn:  decimal.Zero,
			SharpeRatio:  decimal.Zero,
			MaxDrawdown:  decimal.Zero,
			WinRate:      0,
			TotalTrades:  0,
			LastUpdated:  time.Now(),
		},
	}
	
	// Store version
	v.versions[versionID] = version
	
	// Clean old versions if needed
	v.cleanOldVersions(strategyID)
	
	v.logger.Info("Created strategy version",
		zap.String("version_id", versionID),
		zap.String("strategy_id", strategyID),
		zap.String("description", description),
		zap.Strings("tags", tags))
	
	return versionID, nil
}

// DeployVersion deploys a version to a strategy
func (v *VersionManager) DeployVersion(
	strategyID string,
	versionID string,
	strategy Strategy,
	reason string,
) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	
	// Validate version exists
	version, exists := v.versions[versionID]
	if !exists {
		return fmt.Errorf("version %s not found", versionID)
	}
	
	if version.StrategyID != strategyID {
		return fmt.Errorf("version %s does not belong to strategy %s", versionID, strategyID)
	}
	
	// Get current active version
	currentVersionID, hasActive := v.activeVersions[strategyID]
	
	// End current deployment if exists
	if hasActive {
		v.endDeployment(strategyID, currentVersionID, "replaced")
	}
	
	// Create new deployment
	deploymentID := fmt.Sprintf("deploy-%s-%d", versionID, time.Now().Unix())
	deployment := &VersionDeployment{
		DeploymentID: deploymentID,
		VersionID:    versionID,
		Status:       DeploymentStatusTesting,
		StartTime:    time.Now(),
		Reason:       reason,
	}
	
	// Update strategy parameters
	if err := strategy.UpdateParameters(version.Parameters); err != nil {
		return fmt.Errorf("failed to update strategy parameters: %w", err)
	}
	
	// Store deployment
	v.deployments[deploymentID] = deployment
	v.activeVersions[strategyID] = versionID
	
	// Start performance tracking
	v.performanceTracker[versionID] = &PerformanceTracker{
		versionID:      versionID,
		startMetrics:   strategy.GetMetrics(),
		currentMetrics: strategy.GetMetrics(),
		startTime:      time.Now(),
	}
	
	// Schedule test period evaluation
	if v.autoRollback {
		go v.evaluateTestDeployment(strategyID, versionID, deploymentID, strategy)
	}
	
	v.logger.Info("Deployed strategy version",
		zap.String("strategy_id", strategyID),
		zap.String("version_id", versionID),
		zap.String("deployment_id", deploymentID),
		zap.String("reason", reason))
	
	return nil
}

// evaluateTestDeployment evaluates a test deployment
func (v *VersionManager) evaluateTestDeployment(
	strategyID, versionID, deploymentID string,
	strategy Strategy,
) {
	// Wait for test duration
	time.Sleep(v.testDuration)
	
	v.mu.Lock()
	defer v.mu.Unlock()
	
	// Check if still active
	if v.activeVersions[strategyID] != versionID {
		return
	}
	
	deployment, exists := v.deployments[deploymentID]
	if !exists || deployment.Status != DeploymentStatusTesting {
		return
	}
	
	// Evaluate performance
	tracker, exists := v.performanceTracker[versionID]
	if !exists {
		return
	}
	
	currentMetrics := strategy.GetMetrics()
	tracker.currentMetrics = currentMetrics
	
	// Check if performance is acceptable
	if v.isPerformanceAcceptable(tracker) {
		// Promote to active
		deployment.Status = DeploymentStatusActive
		v.logger.Info("Test deployment promoted to active",
			zap.String("version_id", versionID))
	} else {
		// Rollback
		v.logger.Warn("Test deployment failed, rolling back",
			zap.String("version_id", versionID))
		
		// Find previous stable version
		previousVersion := v.findPreviousStableVersion(strategyID, versionID)
		if previousVersion != nil {
			v.rollbackToVersion(strategyID, previousVersion.VersionID, strategy, "auto_rollback")
		}
	}
}

// isPerformanceAcceptable checks if version performance is acceptable
func (v *VersionManager) isPerformanceAcceptable(tracker *PerformanceTracker) bool {
	start := tracker.startMetrics
	current := tracker.currentMetrics
	
	// Check for significant PnL degradation
	pnlChange := current.RealizedPnL.Sub(start.RealizedPnL)
	if pnlChange.LessThan(decimal.NewFromFloat(-1000)) {
		return false
	}
	
	// Check for increased error rate
	if start.TotalOrders > 0 {
		startErrorRate := float64(start.FailedOrders) / float64(start.TotalOrders)
		currentErrorRate := float64(current.FailedOrders) / float64(current.TotalOrders)
		
		if currentErrorRate > startErrorRate*2 && currentErrorRate > 0.1 {
			return false
		}
	}
	
	// Check win rate degradation
	if current.TotalOrders > 10 {
		winRate := float64(current.ProfitableOrders) / float64(current.TotalOrders)
		if winRate < 0.3 { // Less than 30% win rate
			return false
		}
	}
	
	return true
}

// RollbackVersion rolls back to a specific version
func (v *VersionManager) RollbackVersion(
	strategyID string,
	versionID string,
	strategy Strategy,
	reason string,
) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	
	return v.rollbackToVersion(strategyID, versionID, strategy, reason)
}

// rollbackToVersion internal rollback function
func (v *VersionManager) rollbackToVersion(
	strategyID, versionID string,
	strategy Strategy,
	reason string,
) error {
	// Validate version
	version, exists := v.versions[versionID]
	if !exists {
		return fmt.Errorf("version %s not found", versionID)
	}
	
	// End current deployment
	currentVersionID, hasActive := v.activeVersions[strategyID]
	if hasActive {
		v.endDeployment(strategyID, currentVersionID, "rollback")
	}
	
	// Create rollback deployment
	deploymentID := fmt.Sprintf("rollback-%s-%d", versionID, time.Now().Unix())
	deployment := &VersionDeployment{
		DeploymentID: deploymentID,
		VersionID:    versionID,
		Status:       DeploymentStatusRollback,
		StartTime:    time.Now(),
		Reason:       reason,
	}
	
	// Update strategy parameters
	if err := strategy.UpdateParameters(version.Parameters); err != nil {
		return fmt.Errorf("failed to rollback parameters: %w", err)
	}
	
	// Update state
	v.deployments[deploymentID] = deployment
	v.activeVersions[strategyID] = versionID
	
	// After stabilization, mark as active
	go func() {
		time.Sleep(5 * time.Minute)
		v.mu.Lock()
		if v.activeVersions[strategyID] == versionID {
			deployment.Status = DeploymentStatusActive
		}
		v.mu.Unlock()
	}()
	
	v.logger.Info("Rolled back strategy version",
		zap.String("strategy_id", strategyID),
		zap.String("version_id", versionID),
		zap.String("reason", reason))
	
	return nil
}

// findPreviousStableVersion finds the last stable version
func (v *VersionManager) findPreviousStableVersion(strategyID, currentVersionID string) *StrategyVersion {
	var candidates []*StrategyVersion
	
	for _, version := range v.versions {
		if version.StrategyID == strategyID && version.VersionID != currentVersionID {
			candidates = append(candidates, version)
		}
	}
	
	// Sort by creation time (newest first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CreatedAt.After(candidates[j].CreatedAt)
	})
	
	// Find first version with good performance
	for _, version := range candidates {
		if version.Performance != nil && 
			version.Performance.TotalReturn.IsPositive() &&
			version.Performance.MaxDrawdown.LessThan(decimal.NewFromFloat(0.2)) {
			return version
		}
	}
	
	// Return most recent if no good performance found
	if len(candidates) > 0 {
		return candidates[0]
	}
	
	return nil
}

// endDeployment ends a deployment
func (v *VersionManager) endDeployment(strategyID, versionID, reason string) {
	// Find active deployment
	for _, deployment := range v.deployments {
		if deployment.VersionID == versionID && 
			deployment.EndTime == nil &&
			(deployment.Status == DeploymentStatusActive || deployment.Status == DeploymentStatusTesting) {
			now := time.Now()
			deployment.EndTime = &now
			deployment.Status = DeploymentStatusInactive
			
			// Update version performance
			if tracker, exists := v.performanceTracker[versionID]; exists {
				v.updateVersionPerformance(versionID, tracker)
			}
			
			break
		}
	}
}

// updateVersionPerformance updates version performance metrics
func (v *VersionManager) updateVersionPerformance(versionID string, tracker *PerformanceTracker) {
	version, exists := v.versions[versionID]
	if !exists {
		return
	}
	
	start := tracker.startMetrics
	current := tracker.currentMetrics
	duration := time.Since(tracker.startTime)
	
	// Calculate performance
	totalReturn := current.RealizedPnL.Sub(start.RealizedPnL)
	winRate := 0.0
	if current.TotalOrders > 0 {
		winRate = float64(current.ProfitableOrders) / float64(current.TotalOrders)
	}
	
	// Update performance
	version.Performance = &VersionPerformance{
		TotalReturn:    totalReturn,
		SharpeRatio:    current.SharpeRatio,
		MaxDrawdown:    current.MaxDrawdown,
		WinRate:        winRate,
		TotalTrades:    current.TotalOrders,
		ActiveDuration: duration,
		LastUpdated:    time.Now(),
	}
}

// GetActiveVersion returns the active version for a strategy
func (v *VersionManager) GetActiveVersion(strategyID string) (*StrategyVersion, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	
	versionID, exists := v.activeVersions[strategyID]
	if !exists {
		return nil, fmt.Errorf("no active version for strategy %s", strategyID)
	}
	
	version, exists := v.versions[versionID]
	if !exists {
		return nil, fmt.Errorf("version %s not found", versionID)
	}
	
	// Return a copy
	versionCopy := *version
	return &versionCopy, nil
}

// GetVersionHistory returns version history for a strategy
func (v *VersionManager) GetVersionHistory(strategyID string, limit int) []StrategyVersion {
	v.mu.RLock()
	defer v.mu.RUnlock()
	
	var history []StrategyVersion
	for _, version := range v.versions {
		if version.StrategyID == strategyID {
			history = append(history, *version)
		}
	}
	
	// Sort by creation time (newest first)
	sort.Slice(history, func(i, j int) bool {
		return history[i].CreatedAt.After(history[j].CreatedAt)
	})
	
	// Apply limit
	if limit > 0 && len(history) > limit {
		history = history[:limit]
	}
	
	return history
}

// GetDeploymentHistory returns deployment history
func (v *VersionManager) GetDeploymentHistory(strategyID string, limit int) []VersionDeployment {
	v.mu.RLock()
	defer v.mu.RUnlock()
	
	var history []VersionDeployment
	for _, deployment := range v.deployments {
		// Check if deployment is for this strategy
		if version, exists := v.versions[deployment.VersionID]; exists {
			if version.StrategyID == strategyID {
				history = append(history, *deployment)
			}
		}
	}
	
	// Sort by start time (newest first)
	sort.Slice(history, func(i, j int) bool {
		return history[i].StartTime.After(history[j].StartTime)
	})
	
	// Apply limit
	if limit > 0 && len(history) > limit {
		history = history[:limit]
	}
	
	return history
}

// CompareVersions compares performance between versions
func (v *VersionManager) CompareVersions(versionID1, versionID2 string) (*VersionComparison, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	
	version1, exists1 := v.versions[versionID1]
	version2, exists2 := v.versions[versionID2]
	
	if !exists1 || !exists2 {
		return nil, fmt.Errorf("one or both versions not found")
	}
	
	comparison := &VersionComparison{
		Version1:           version1,
		Version2:           version2,
		ReturnDifference:   version1.Performance.TotalReturn.Sub(version2.Performance.TotalReturn),
		SharpeImprovement:  version1.Performance.SharpeRatio.Sub(version2.Performance.SharpeRatio),
		DrawdownImprovement: version2.Performance.MaxDrawdown.Sub(version1.Performance.MaxDrawdown),
		WinRateDifference:  version1.Performance.WinRate - version2.Performance.WinRate,
	}
	
	return comparison, nil
}

// VersionComparison represents a comparison between versions
type VersionComparison struct {
	Version1            *StrategyVersion
	Version2            *StrategyVersion
	ReturnDifference    decimal.Decimal
	SharpeImprovement   decimal.Decimal
	DrawdownImprovement decimal.Decimal
	WinRateDifference   float64
}

// copyParameters creates a deep copy of parameters
func (v *VersionManager) copyParameters(params map[string]interface{}) map[string]interface{} {
	copy := make(map[string]interface{})
	for k, v := range params {
		copy[k] = v
	}
	return copy
}

// cleanOldVersions removes old versions beyond limit
func (v *VersionManager) cleanOldVersions(strategyID string) {
	var versions []*StrategyVersion
	for _, version := range v.versions {
		if version.StrategyID == strategyID {
			versions = append(versions, version)
		}
	}
	
	if len(versions) <= v.maxVersions {
		return
	}
	
	// Sort by creation time (oldest first)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].CreatedAt.Before(versions[j].CreatedAt)
	})
	
	// Remove oldest versions
	toRemove := len(versions) - v.maxVersions
	for i := 0; i < toRemove; i++ {
		if v.activeVersions[strategyID] != versions[i].VersionID {
			delete(v.versions, versions[i].VersionID)
		}
	}
}