package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/your-org/mExOms/pkg/types"
	"go.uber.org/zap"
)

// StrategyState represents the current state of a strategy
type StrategyState string

const (
	StateIdle       StrategyState = "idle"
	StateStarting   StrategyState = "starting"
	StateRunning    StrategyState = "running"
	StatePausing    StrategyState = "pausing"
	StatePaused     StrategyState = "paused"
	StateStopping   StrategyState = "stopping"
	StateStopped    StrategyState = "stopped"
	StateError      StrategyState = "error"
)

// StrategyInfo contains runtime information about a strategy
type StrategyInfo struct {
	ID              string
	Name            string
	Type            string
	State           StrategyState
	AssignedAccount string
	AllocatedCapital float64
	StartTime       time.Time
	LastUpdateTime  time.Time
	Performance     StrategyPerformance
	Config          map[string]interface{}
	Dependencies    []string // Other strategies this depends on
	Conflicts       []string // Strategies that conflict with this one
}

// StrategyPerformance tracks strategy performance metrics
type StrategyPerformance struct {
	TotalPnL        float64
	RealizedPnL     float64
	UnrealizedPnL   float64
	TotalTrades     int
	WinningTrades   int
	LosingTrades    int
	SharpeRatio     float64
	MaxDrawdown     float64
	CurrentDrawdown float64
	LastUpdate      time.Time
}

// ConflictRule defines rules for strategy conflicts
type ConflictRule struct {
	Strategy1  string
	Strategy2  string
	Type       string // "symbol", "account", "resource"
	Resolution string // "stop_first", "stop_second", "queue", "reject"
}

// Orchestrator manages multiple trading strategies
type Orchestrator struct {
	strategies       map[string]*StrategyInfo
	strategyHandlers map[string]types.Strategy
	accountManager   types.AccountManager
	capitalAllocator *CapitalAllocator
	conflictRules    []ConflictRule
	killSwitch       *KillSwitch
	logger           *zap.Logger
	mu               sync.RWMutex
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
}

// NewOrchestrator creates a new strategy orchestrator
func NewOrchestrator(accountManager types.AccountManager, logger *zap.Logger) *Orchestrator {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Orchestrator{
		strategies:       make(map[string]*StrategyInfo),
		strategyHandlers: make(map[string]types.Strategy),
		accountManager:   accountManager,
		capitalAllocator: NewCapitalAllocator(),
		conflictRules:    DefaultConflictRules(),
		killSwitch:       NewKillSwitch(logger),
		logger:           logger,
		ctx:              ctx,
		cancel:           cancel,
	}
}

// RegisterStrategy registers a new strategy with the orchestrator
func (o *Orchestrator) RegisterStrategy(strategy types.Strategy, config map[string]interface{}) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	strategyID := strategy.GetID()
	if _, exists := o.strategies[strategyID]; exists {
		return fmt.Errorf("strategy %s already registered", strategyID)
	}

	// Check for conflicts with existing strategies
	if conflicts := o.checkConflicts(strategy); len(conflicts) > 0 {
		return fmt.Errorf("strategy conflicts detected: %v", conflicts)
	}

	// Assign account based on strategy requirements
	account, err := o.assignAccount(strategy)
	if err != nil {
		return fmt.Errorf("failed to assign account: %v", err)
	}

	info := &StrategyInfo{
		ID:              strategyID,
		Name:            strategy.GetName(),
		Type:            strategy.GetType(),
		State:           StateIdle,
		AssignedAccount: account,
		Config:          config,
		Dependencies:    strategy.GetDependencies(),
		Conflicts:       strategy.GetConflicts(),
	}

	o.strategies[strategyID] = info
	o.strategyHandlers[strategyID] = strategy

	o.logger.Info("Strategy registered",
		zap.String("strategy_id", strategyID),
		zap.String("type", info.Type),
		zap.String("account", account))

	return nil
}

// StartStrategy starts a registered strategy
func (o *Orchestrator) StartStrategy(strategyID string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	info, exists := o.strategies[strategyID]
	if !exists {
		return fmt.Errorf("strategy %s not found", strategyID)
	}

	if info.State != StateIdle && info.State != StateStopped {
		return fmt.Errorf("strategy %s is not in a startable state: %s", strategyID, info.State)
	}

	// Check dependencies
	if err := o.checkDependencies(info); err != nil {
		return fmt.Errorf("dependency check failed: %v", err)
	}

	// Allocate capital
	allocation, err := o.capitalAllocator.AllocateCapital(strategyID, info.Config)
	if err != nil {
		return fmt.Errorf("capital allocation failed: %v", err)
	}
	info.AllocatedCapital = allocation

	// Update state
	info.State = StateStarting
	info.StartTime = time.Now()
	info.LastUpdateTime = time.Now()

	// Start strategy in goroutine
	strategy := o.strategyHandlers[strategyID]
	o.wg.Add(1)
	go o.runStrategy(strategy, info)

	o.logger.Info("Strategy started",
		zap.String("strategy_id", strategyID),
		zap.Float64("allocated_capital", allocation))

	return nil
}

// StopStrategy stops a running strategy
func (o *Orchestrator) StopStrategy(strategyID string, force bool) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	info, exists := o.strategies[strategyID]
	if !exists {
		return fmt.Errorf("strategy %s not found", strategyID)
	}

	if info.State != StateRunning && info.State != StatePaused {
		return fmt.Errorf("strategy %s is not running: %s", strategyID, info.State)
	}

	// Check if other strategies depend on this one
	if !force {
		for id, s := range o.strategies {
			if id != strategyID && s.State == StateRunning {
				for _, dep := range s.Dependencies {
					if dep == strategyID {
						return fmt.Errorf("strategy %s depends on %s", id, strategyID)
					}
				}
			}
		}
	}

	info.State = StateStopping

	// Signal strategy to stop
	if handler, ok := o.strategyHandlers[strategyID]; ok {
		handler.Stop()
	}

	// Release capital
	o.capitalAllocator.ReleaseCapital(strategyID)

	o.logger.Info("Strategy stopping",
		zap.String("strategy_id", strategyID),
		zap.Bool("forced", force))

	return nil
}

// PauseStrategy pauses a running strategy
func (o *Orchestrator) PauseStrategy(strategyID string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	info, exists := o.strategies[strategyID]
	if !exists {
		return fmt.Errorf("strategy %s not found", strategyID)
	}

	if info.State != StateRunning {
		return fmt.Errorf("strategy %s is not running", strategyID)
	}

	info.State = StatePausing

	if handler, ok := o.strategyHandlers[strategyID]; ok {
		handler.Pause()
	}

	o.logger.Info("Strategy pausing", zap.String("strategy_id", strategyID))
	return nil
}

// ResumeStrategy resumes a paused strategy
func (o *Orchestrator) ResumeStrategy(strategyID string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	info, exists := o.strategies[strategyID]
	if !exists {
		return fmt.Errorf("strategy %s not found", strategyID)
	}

	if info.State != StatePaused {
		return fmt.Errorf("strategy %s is not paused", strategyID)
	}

	info.State = StateRunning

	if handler, ok := o.strategyHandlers[strategyID]; ok {
		handler.Resume()
	}

	o.logger.Info("Strategy resumed", zap.String("strategy_id", strategyID))
	return nil
}

// runStrategy executes a strategy
func (o *Orchestrator) runStrategy(strategy types.Strategy, info *StrategyInfo) {
	defer o.wg.Done()

	// Update state
	o.mu.Lock()
	info.State = StateRunning
	o.mu.Unlock()

	// Create strategy context
	strategyCtx := &types.StrategyContext{
		ID:               info.ID,
		Account:          info.AssignedAccount,
		AllocatedCapital: info.AllocatedCapital,
		Config:           info.Config,
		Logger:           o.logger.With(zap.String("strategy", info.ID)),
	}

	// Run strategy
	err := strategy.Run(strategyCtx)

	// Update final state
	o.mu.Lock()
	if err != nil {
		info.State = StateError
		o.logger.Error("Strategy error",
			zap.String("strategy_id", info.ID),
			zap.Error(err))
	} else {
		info.State = StateStopped
		o.logger.Info("Strategy stopped",
			zap.String("strategy_id", info.ID))
	}
	o.mu.Unlock()
}

// checkConflicts checks for conflicts with existing strategies
func (o *Orchestrator) checkConflicts(strategy types.Strategy) []string {
	var conflicts []string

	for id, info := range o.strategies {
		if info.State == StateRunning || info.State == StateStarting {
			// Check explicit conflicts
			for _, conflict := range strategy.GetConflicts() {
				if conflict == info.Type || conflict == id {
					conflicts = append(conflicts, fmt.Sprintf("%s conflicts with %s", strategy.GetID(), id))
				}
			}

			// Check conflict rules
			for _, ruleouzrule := range o.conflictRules {
				if (rule.Strategy1 == strategy.GetType() && rule.Strategy2 == info.Type) ||
					(rule.Strategy2 == strategy.GetType() && rule.Strategy1 == info.Type) {
					conflicts = append(conflicts, fmt.Sprintf("conflict rule: %s <-> %s", rule.Strategy1, rule.Strategy2))
				}
			}
		}
	}

	return conflicts
}

// checkDependencies checks if all required dependencies are running
func (o *Orchestrator) checkDependencies(info *StrategyInfo) error {
	for _, dep := range info.Dependencies {
		depInfo, exists := o.strategies[dep]
		if !exists {
			return fmt.Errorf("dependency %s not found", dep)
		}
		if depInfo.State != StateRunning {
			return fmt.Errorf("dependency %s is not running", dep)
		}
	}
	return nil
}

// assignAccount assigns an appropriate account to a strategy
func (o *Orchestrator) assignAccount(strategy types.Strategy) (string, error) {
	requirements := strategy.GetAccountRequirements()
	
	// Get available accounts
	accounts := o.accountManager.GetAccounts()
	
	// Find best matching account
	for _, account := range accounts {
		if o.meetsRequirements(account, requirements) {
			// Check if account is not overloaded
			activeStrategies := o.getAccountStrategies(account.ID)
			if len(activeStrategies) < account.MaxStrategies {
				return account.ID, nil
			}
		}
	}

	return "", fmt.Errorf("no suitable account found for strategy requirements")
}

// meetsRequirements checks if an account meets strategy requirements
func (o *Orchestrator) meetsRequirements(account *types.Account, req types.AccountRequirements) bool {
	if req.MinBalance > 0 && account.Balance < req.MinBalance {
		return false
	}
	
	if req.Exchange != "" && account.Exchange != req.Exchange {
		return false
	}
	
	if req.Market != "" && account.Market != req.Market {
		return false
	}
	
	if req.Features != nil {
		for _, feature := range req.Features {
			if !account.HasFeature(feature) {
				return false
			}
		}
	}
	
	return true
}

// getAccountStrategies returns strategies assigned to an account
func (o *Orchestrator) getAccountStrategies(accountID string) []string {
	var strategies []string
	for id, info := range o.strategies {
		if info.AssignedAccount == accountID && 
			(info.State == StateRunning || info.State == StateStarting) {
			strategies = append(strategies, id)
		}
	}
	return strategies
}

// GetStrategies returns all registered strategies
func (o *Orchestrator) GetStrategies() []*StrategyInfo {
	o.mu.RLock()
	defer o.mu.RUnlock()

	strategies := make([]*StrategyInfo, 0, len(o.strategies))
	for _, info := range o.strategies {
		strategies = append(strategies, info)
	}
	return strategies
}

// GetStrategy returns information about a specific strategy
func (o *Orchestrator) GetStrategy(strategyID string) (*StrategyInfo, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	info, exists := o.strategies[strategyID]
	if !exists {
		return nil, fmt.Errorf("strategy %s not found", strategyID)
	}
	return info, nil
}

// UpdatePerformance updates strategy performance metrics
func (o *Orchestrator) UpdatePerformance(strategyID string, perf StrategyPerformance) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	info, exists := o.strategies[strategyID]
	if !exists {
		return fmt.Errorf("strategy %s not found", strategyID)
	}

	info.Performance = perf
	info.LastUpdateTime = time.Now()

	// Check if strategy should be stopped due to poor performance
	if perf.CurrentDrawdown > 0.2 { // 20% drawdown
		o.logger.Warn("High drawdown detected",
			zap.String("strategy_id", strategyID),
			zap.Float64("drawdown", perf.CurrentDrawdown))
		
		// Trigger kill switch if enabled
		if o.killSwitch.IsEnabled() {
			o.killSwitch.TriggerForStrategy(strategyID, "High drawdown")
		}
	}

	return nil
}

// Shutdown gracefully shuts down the orchestrator
func (o *Orchestrator) Shutdown() error {
	o.logger.Info("Shutting down strategy orchestrator")

	// Stop all running strategies
	for id, info := range o.strategies {
		if info.State == StateRunning || info.State == StateStarting {
			o.StopStrategy(id, true)
		}
	}

	// Cancel context
	o.cancel()

	// Wait for all strategies to stop
	done := make(chan struct{})
	go func() {
		o.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		o.logger.Info("All strategies stopped")
	case <-time.After(30 * time.Second):
		o.logger.Warn("Timeout waiting for strategies to stop")
	}

	return nil
}

// DefaultConflictRules returns default conflict rules
func DefaultConflictRules() []ConflictRule {
	return []ConflictRule{
		{
			Strategy1:  "market_maker",
			Strategy2:  "arbitrage",
			Type:       "symbol",
			Resolution: "queue",
		},
		{
			Strategy1:  "grid_trading",
			Strategy2:  "trend_following",
			Type:       "symbol",
			Resolution: "stop_first",
		},
		{
			Strategy1:  "scalping",
			Strategy2:  "scalping",
			Type:       "account",
			Resolution: "reject",
		},
	}
}