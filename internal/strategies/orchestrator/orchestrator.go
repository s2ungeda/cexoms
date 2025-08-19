package orchestrator

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/mExOms/internal/account"
	"github.com/mExOms/internal/strategies/arbitrage"
	"github.com/mExOms/internal/strategies/market_maker"
	"github.com/mExOms/pkg/types"
	"github.com/nats-io/nats.go"
)

// StrategyType represents the type of trading strategy
type StrategyType string

const (
	StrategyTypeArbitrage    StrategyType = "arbitrage"
	StrategyTypeMarketMaking StrategyType = "market_making"
)

// StrategyStatus represents the current status of a strategy
type StrategyStatus string

const (
	StatusRunning StrategyStatus = "running"
	StatusStopped StrategyStatus = "stopped"
	StatusPaused  StrategyStatus = "paused"
	StatusError   StrategyStatus = "error"
)

// Strategy interface that all strategies must implement
type Strategy interface {
	Start(ctx context.Context) error
	Stop() error
	GetType() StrategyType
	GetStatus() StrategyStatus
	GetMetrics() *StrategyMetrics
}

// StrategyMetrics contains performance metrics for a strategy
type StrategyMetrics struct {
	PnL              float64
	TotalTrades      int64
	WinningTrades    int64
	LosingTrades     int64
	MaxDrawdown      float64
	SharpeRatio      float64
	UpdatedAt        time.Time
	DailyPnL         float64
	ConsecutiveLosses int
}

// StrategyInstance represents a running strategy instance
type StrategyInstance struct {
	ID           string
	Type         StrategyType
	Strategy     Strategy
	Accounts     []string
	Config       interface{}
	Status       StrategyStatus
	StartedAt    time.Time
	StoppedAt    *time.Time
	Metrics      *StrategyMetrics
	ErrorMessage string
	mu           sync.RWMutex
}

// OrchestratorConfig contains configuration for the strategy orchestrator
type OrchestratorConfig struct {
	MaxConcurrentStrategies int
	KillSwitch             KillSwitchConfig
	CapitalAllocation      CapitalAllocationConfig
	MonitoringInterval     time.Duration
}

// KillSwitchConfig contains kill switch configuration
type KillSwitchConfig struct {
	MaxDailyLoss         float64
	MaxConsecutiveLosses int
	MaxDrawdown          float64
	Enabled              bool
}

// CapitalAllocationConfig contains capital allocation settings
type CapitalAllocationConfig struct {
	TotalCapital       float64
	MaxPerStrategy     float64
	MinPerStrategy     float64
	RebalanceInterval  time.Duration
	RiskAdjusted       bool
}

// Orchestrator manages all trading strategies
type Orchestrator struct {
	config            OrchestratorConfig
	strategies        map[string]*StrategyInstance
	accountManager    *account.Manager
	nc                *nats.Conn
	js                nats.JetStreamContext
	capitalAllocator  *CapitalAllocator
	riskMonitor       *RiskMonitor
	mu                sync.RWMutex
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
}

// New creates a new strategy orchestrator
func New(config OrchestratorConfig, accountManager *account.Manager, nc *nats.Conn) (*Orchestrator, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	o := &Orchestrator{
		config:         config,
		strategies:     make(map[string]*StrategyInstance),
		accountManager: accountManager,
		nc:             nc,
		js:             js,
		ctx:            ctx,
		cancel:         cancel,
	}

	// Initialize capital allocator
	o.capitalAllocator = NewCapitalAllocator(config.CapitalAllocation)

	// Initialize risk monitor
	o.riskMonitor = NewRiskMonitor(config.KillSwitch)

	return o, nil
}

// StartStrategy starts a new strategy instance
func (o *Orchestrator) StartStrategy(strategyType StrategyType, config interface{}, accounts []string) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Check concurrent strategy limit
	runningCount := 0
	for _, instance := range o.strategies {
		if instance.Status == StatusRunning {
			runningCount++
		}
	}

	if runningCount >= o.config.MaxConcurrentStrategies {
		return "", fmt.Errorf("maximum concurrent strategies limit reached: %d", o.config.MaxConcurrentStrategies)
	}

	// Allocate capital for the strategy
	capital, err := o.capitalAllocator.AllocateCapital(strategyType, accounts)
	if err != nil {
		return "", fmt.Errorf("failed to allocate capital: %w", err)
	}

	// Create strategy instance
	var strategy Strategy
	switch strategyType {
	case StrategyTypeArbitrage:
		// Create arbitrage strategy
		arbConfig, ok := config.(arbitrage.Config)
		if !ok {
			return "", fmt.Errorf("invalid config type for arbitrage strategy")
		}
		// Note: Actual arbitrage strategy creation would involve getting exchanges from accountManager
		// strategy = arbitrage.New(arbConfig, exchanges...)

	case StrategyTypeMarketMaking:
		// Create market making strategy
		mmConfig, ok := config.(market_maker.Config)
		if !ok {
			return "", fmt.Errorf("invalid config type for market making strategy")
		}
		// Note: Actual MM strategy creation would involve getting exchange from accountManager
		// strategy = market_maker.New(exchange, mmConfig)

	default:
		return "", fmt.Errorf("unknown strategy type: %s", strategyType)
	}

	// Create instance
	instance := &StrategyInstance{
		ID:        generateStrategyID(),
		Type:      strategyType,
		Strategy:  strategy,
		Accounts:  accounts,
		Config:    config,
		Status:    StatusStopped,
		StartedAt: time.Now(),
		Metrics:   &StrategyMetrics{UpdatedAt: time.Now()},
	}

	// Start the strategy
	if err := o.startStrategyInstance(instance); err != nil {
		o.capitalAllocator.ReleaseCapital(instance.ID, capital)
		return "", err
	}

	o.strategies[instance.ID] = instance

	// Publish strategy started event
	o.publishEvent("strategy.started", map[string]interface{}{
		"id":       instance.ID,
		"type":     strategyType,
		"accounts": accounts,
		"capital":  capital,
	})

	return instance.ID, nil
}

// StopStrategy stops a running strategy
func (o *Orchestrator) StopStrategy(strategyID string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	instance, exists := o.strategies[strategyID]
	if !exists {
		return fmt.Errorf("strategy not found: %s", strategyID)
	}

	if instance.Status != StatusRunning {
		return fmt.Errorf("strategy is not running: %s", instance.Status)
	}

	// Stop the strategy
	if err := instance.Strategy.Stop(); err != nil {
		return fmt.Errorf("failed to stop strategy: %w", err)
	}

	// Update status
	now := time.Now()
	instance.Status = StatusStopped
	instance.StoppedAt = &now

	// Release allocated capital
	o.capitalAllocator.ReleaseCapital(strategyID, 0) // Amount will be calculated internally

	// Publish strategy stopped event
	o.publishEvent("strategy.stopped", map[string]interface{}{
		"id":      strategyID,
		"type":    instance.Type,
		"metrics": instance.Metrics,
	})

	return nil
}

// GetStrategyStatus returns the status of a strategy
func (o *Orchestrator) GetStrategyStatus(strategyID string) (*StrategyInstance, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	instance, exists := o.strategies[strategyID]
	if !exists {
		return nil, fmt.Errorf("strategy not found: %s", strategyID)
	}

	return instance, nil
}

// ListStrategies returns all strategy instances
func (o *Orchestrator) ListStrategies() []*StrategyInstance {
	o.mu.RLock()
	defer o.mu.RUnlock()

	instances := make([]*StrategyInstance, 0, len(o.strategies))
	for _, instance := range o.strategies {
		instances = append(instances, instance)
	}

	return instances
}

// Start starts the orchestrator
func (o *Orchestrator) Start() error {
	o.wg.Add(2)

	// Start monitoring goroutine
	go o.monitorStrategies()

	// Start capital rebalancing goroutine
	go o.rebalanceCapital()

	log.Println("Strategy orchestrator started")
	return nil
}

// Stop stops the orchestrator and all running strategies
func (o *Orchestrator) Stop() error {
	log.Println("Stopping strategy orchestrator...")

	// Cancel context
	o.cancel()

	// Stop all running strategies
	o.mu.Lock()
	var wg sync.WaitGroup
	for id, instance := range o.strategies {
		if instance.Status == StatusRunning {
			wg.Add(1)
			go func(strategyID string, strategy Strategy) {
				defer wg.Done()
				if err := strategy.Stop(); err != nil {
					log.Printf("Error stopping strategy %s: %v", strategyID, err)
				}
			}(id, instance.Strategy)
		}
	}
	o.mu.Unlock()

	// Wait for all strategies to stop
	wg.Wait()

	// Wait for internal goroutines
	o.wg.Wait()

	log.Println("Strategy orchestrator stopped")
	return nil
}

// monitorStrategies monitors running strategies and enforces risk limits
func (o *Orchestrator) monitorStrategies() {
	defer o.wg.Done()

	ticker := time.NewTicker(o.config.MonitoringInterval)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			o.checkStrategies()
		}
	}
}

// checkStrategies checks all running strategies
func (o *Orchestrator) checkStrategies() {
	o.mu.RLock()
	strategies := make([]*StrategyInstance, 0)
	for _, instance := range o.strategies {
		if instance.Status == StatusRunning {
			strategies = append(strategies, instance)
		}
	}
	o.mu.RUnlock()

	// Check each strategy
	for _, instance := range strategies {
		// Update metrics
		metrics := instance.Strategy.GetMetrics()
		instance.mu.Lock()
		instance.Metrics = metrics
		instance.mu.Unlock()

		// Check kill switch conditions
		if o.config.KillSwitch.Enabled {
			if shouldStop, reason := o.riskMonitor.ShouldStopStrategy(instance); shouldStop {
				log.Printf("Kill switch triggered for strategy %s: %s", instance.ID, reason)
				if err := o.StopStrategy(instance.ID); err != nil {
					log.Printf("Failed to stop strategy %s: %v", instance.ID, err)
				}

				// Send alert
				o.publishEvent("kill_switch.triggered", map[string]interface{}{
					"strategy_id": instance.ID,
					"reason":      reason,
					"metrics":     metrics,
				})
			}
		}

		// Publish metrics update
		o.publishEvent("strategy.metrics", map[string]interface{}{
			"id":      instance.ID,
			"type":    instance.Type,
			"metrics": metrics,
		})
	}
}

// rebalanceCapital periodically rebalances capital allocation
func (o *Orchestrator) rebalanceCapital() {
	defer o.wg.Done()

	if o.config.CapitalAllocation.RebalanceInterval <= 0 {
		return
	}

	ticker := time.NewTicker(o.config.CapitalAllocation.RebalanceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			o.performRebalancing()
		}
	}
}

// performRebalancing performs capital rebalancing
func (o *Orchestrator) performRebalancing() {
	o.mu.RLock()
	strategies := make([]*StrategyInstance, 0)
	for _, instance := range o.strategies {
		if instance.Status == StatusRunning {
			strategies = append(strategies, instance)
		}
	}
	o.mu.RUnlock()

	// Rebalance capital based on performance
	allocations := o.capitalAllocator.Rebalance(strategies)

	// Publish rebalancing event
	o.publishEvent("capital.rebalanced", map[string]interface{}{
		"allocations": allocations,
		"timestamp":   time.Now(),
	})
}

// startStrategyInstance starts a strategy instance
func (o *Orchestrator) startStrategyInstance(instance *StrategyInstance) error {
	// Create strategy-specific context
	ctx, cancel := context.WithCancel(o.ctx)

	// Start the strategy
	go func() {
		if err := instance.Strategy.Start(ctx); err != nil {
			instance.mu.Lock()
			instance.Status = StatusError
			instance.ErrorMessage = err.Error()
			instance.mu.Unlock()

			log.Printf("Strategy %s error: %v", instance.ID, err)
		}
	}()

	// Wait briefly to ensure strategy starts
	time.Sleep(100 * time.Millisecond)

	// Update status
	instance.mu.Lock()
	if instance.Status != StatusError {
		instance.Status = StatusRunning
	}
	instance.mu.Unlock()

	// Store cancel function for later use
	_ = cancel // Would be stored in instance for stopping

	return nil
}

// publishEvent publishes an event to NATS
func (o *Orchestrator) publishEvent(subject string, data interface{}) {
	fullSubject := fmt.Sprintf("strategies.orchestrator.%s", subject)
	
	if err := o.nc.Publish(fullSubject, []byte(fmt.Sprintf("%v", data))); err != nil {
		log.Printf("Failed to publish event %s: %v", fullSubject, err)
	}
}

// generateStrategyID generates a unique strategy ID
func generateStrategyID() string {
	return fmt.Sprintf("strat_%d", time.Now().UnixNano())
}