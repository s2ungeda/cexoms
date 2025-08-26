package strategies

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	
	"github.com/mExOms/pkg/types"
)

// StrategyFactory creates strategy instances
type StrategyFactory struct {
	mu sync.RWMutex
	
	// Registered strategy builders
	builders map[StrategyType]StrategyBuilder
	
	// Dependencies
	orderManager    types.OrderManager
	positionManager types.PositionManager
	marketData      types.MarketDataProvider
	logger          *zap.Logger
}

// StrategyBuilder is a function that builds a strategy
type StrategyBuilder func(
	config StrategyConfig,
	orderManager types.OrderManager,
	positionManager types.PositionManager,
	marketData types.MarketDataProvider,
	logger *zap.Logger,
) (Strategy, error)

// NewStrategyFactory creates a new strategy factory
func NewStrategyFactory(
	orderManager types.OrderManager,
	positionManager types.PositionManager,
	marketData types.MarketDataProvider,
	logger *zap.Logger,
) *StrategyFactory {
	factory := &StrategyFactory{
		builders:        make(map[StrategyType]StrategyBuilder),
		orderManager:    orderManager,
		positionManager: positionManager,
		marketData:      marketData,
		logger:          logger,
	}
	
	// Register default strategy builders
	factory.registerDefaultStrategies()
	
	return factory
}

// RegisterStrategy registers a strategy builder
func (f *StrategyFactory) RegisterStrategy(strategyType StrategyType, builder StrategyBuilder) {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	f.builders[strategyType] = builder
	f.logger.Info("Registered strategy type",
		zap.String("type", string(strategyType)))
}

// CreateStrategy creates a strategy instance
func (f *StrategyFactory) CreateStrategy(config StrategyConfig) (Strategy, error) {
	f.mu.RLock()
	builder, exists := f.builders[config.Type]
	f.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("unknown strategy type: %s", config.Type)
	}
	
	// Create strategy instance
	strategy, err := builder(
		config,
		f.orderManager,
		f.positionManager,
		f.marketData,
		f.logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create strategy: %w", err)
	}
	
	// Initialize strategy
	if err := strategy.Initialize(config); err != nil {
		return nil, fmt.Errorf("failed to initialize strategy: %w", err)
	}
	
	f.logger.Info("Created strategy",
		zap.String("id", config.ID),
		zap.String("name", config.Name),
		zap.String("type", string(config.Type)))
	
	return strategy, nil
}

// GetAvailableStrategies returns list of available strategy types
func (f *StrategyFactory) GetAvailableStrategies() []StrategyType {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	types := make([]StrategyType, 0, len(f.builders))
	for t := range f.builders {
		types = append(types, t)
	}
	
	return types
}

// registerDefaultStrategies registers built-in strategy types
func (f *StrategyFactory) registerDefaultStrategies() {
	// Register arbitrage strategy
	f.RegisterStrategy(StrategyTypeArbitrage, func(
		config StrategyConfig,
		orderManager types.OrderManager,
		positionManager types.PositionManager,
		marketData types.MarketDataProvider,
		logger *zap.Logger,
	) (Strategy, error) {
		// Import templates package to avoid circular dependency
		// In real implementation, would use proper package structure
		return nil, fmt.Errorf("arbitrage strategy not implemented in factory")
	})
	
	// Register grid strategy
	f.RegisterStrategy(StrategyTypeGrid, func(
		config StrategyConfig,
		orderManager types.OrderManager,
		positionManager types.PositionManager,
		marketData types.MarketDataProvider,
		logger *zap.Logger,
	) (Strategy, error) {
		return nil, fmt.Errorf("grid strategy not implemented in factory")
	})
	
	// Register market making strategy
	f.RegisterStrategy(StrategyTypeMarketMaking, func(
		config StrategyConfig,
		orderManager types.OrderManager,
		positionManager types.PositionManager,
		marketData types.MarketDataProvider,
		logger *zap.Logger,
	) (Strategy, error) {
		return nil, fmt.Errorf("market making strategy not implemented in factory")
	})
}

// StrategyManager manages multiple strategy instances
type StrategyManager struct {
	mu sync.RWMutex
	
	// Active strategies
	strategies map[string]Strategy
	
	// Factory for creating strategies
	factory *StrategyFactory
	
	// Logger
	logger *zap.Logger
	
	// Context for lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewStrategyManager creates a new strategy manager
func NewStrategyManager(factory *StrategyFactory, logger *zap.Logger) *StrategyManager {
	return &StrategyManager{
		strategies: make(map[string]Strategy),
		factory:    factory,
		logger:     logger,
	}
}

// Start starts the strategy manager
func (m *StrategyManager) Start(ctx context.Context) error {
	m.mu.Lock()
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.mu.Unlock()
	
	// Start monitoring routine
	m.wg.Add(1)
	go m.monitorStrategies()
	
	m.logger.Info("Strategy manager started")
	return nil
}

// Stop stops the strategy manager and all strategies
func (m *StrategyManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Stop all strategies
	for id, strategy := range m.strategies {
		if err := strategy.Stop(); err != nil {
			m.logger.Error("Failed to stop strategy",
				zap.String("id", id),
				zap.Error(err))
		}
	}
	
	// Cancel context
	if m.cancel != nil {
		m.cancel()
	}
	
	// Wait for routines
	m.wg.Wait()
	
	m.logger.Info("Strategy manager stopped")
	return nil
}

// CreateAndStartStrategy creates and starts a new strategy
func (m *StrategyManager) CreateAndStartStrategy(config StrategyConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Check if strategy already exists
	if _, exists := m.strategies[config.ID]; exists {
		return fmt.Errorf("strategy already exists: %s", config.ID)
	}
	
	// Create strategy
	strategy, err := m.factory.CreateStrategy(config)
	if err != nil {
		return fmt.Errorf("failed to create strategy: %w", err)
	}
	
	// Start strategy
	if err := strategy.Start(m.ctx); err != nil {
		return fmt.Errorf("failed to start strategy: %w", err)
	}
	
	// Add to active strategies
	m.strategies[config.ID] = strategy
	
	m.logger.Info("Strategy created and started",
		zap.String("id", config.ID),
		zap.String("name", config.Name),
		zap.String("type", string(config.Type)))
	
	return nil
}

// StopStrategy stops a strategy
func (m *StrategyManager) StopStrategy(strategyID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	strategy, exists := m.strategies[strategyID]
	if !exists {
		return fmt.Errorf("strategy not found: %s", strategyID)
	}
	
	// Stop strategy
	if err := strategy.Stop(); err != nil {
		return fmt.Errorf("failed to stop strategy: %w", err)
	}
	
	// Remove from active strategies
	delete(m.strategies, strategyID)
	
	m.logger.Info("Strategy stopped",
		zap.String("id", strategyID))
	
	return nil
}

// UpdateStrategyParameters updates strategy parameters
func (m *StrategyManager) UpdateStrategyParameters(strategyID string, params map[string]interface{}) error {
	m.mu.RLock()
	strategy, exists := m.strategies[strategyID]
	m.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("strategy not found: %s", strategyID)
	}
	
	// Update parameters
	if err := strategy.UpdateParameters(params); err != nil {
		return fmt.Errorf("failed to update parameters: %w", err)
	}
	
	m.logger.Info("Strategy parameters updated",
		zap.String("id", strategyID),
		zap.Any("params", params))
	
	return nil
}

// GetStrategy returns a strategy by ID
func (m *StrategyManager) GetStrategy(strategyID string) (Strategy, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	strategy, exists := m.strategies[strategyID]
	if !exists {
		return nil, fmt.Errorf("strategy not found: %s", strategyID)
	}
	
	return strategy, nil
}

// GetAllStrategies returns all active strategies
func (m *StrategyManager) GetAllStrategies() map[string]Strategy {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Return a copy
	strategies := make(map[string]Strategy)
	for id, strategy := range m.strategies {
		strategies[id] = strategy
	}
	
	return strategies
}

// GetStrategyMetrics returns metrics for a strategy
func (m *StrategyManager) GetStrategyMetrics(strategyID string) (StrategyMetrics, error) {
	m.mu.RLock()
	strategy, exists := m.strategies[strategyID]
	m.mu.RUnlock()
	
	if !exists {
		return StrategyMetrics{}, fmt.Errorf("strategy not found: %s", strategyID)
	}
	
	return strategy.GetMetrics(), nil
}

// GetAllMetrics returns metrics for all strategies
func (m *StrategyManager) GetAllMetrics() map[string]StrategyMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	metrics := make(map[string]StrategyMetrics)
	for id, strategy := range m.strategies {
		metrics[id] = strategy.GetMetrics()
	}
	
	return metrics
}

// monitorStrategies monitors strategy health and performance
func (m *StrategyManager) monitorStrategies() {
	defer m.wg.Done()
	
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkStrategyHealth()
		}
	}
}

// checkStrategyHealth checks health of all strategies
func (m *StrategyManager) checkStrategyHealth() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	for id, strategy := range m.strategies {
		// Check risk limits
		if err := strategy.CheckRiskLimits(); err != nil {
			m.logger.Error("Strategy risk limit breached",
				zap.String("id", id),
				zap.Error(err))
			
			// Could implement auto-stop on risk breach
		}
		
		// Get metrics
		metrics := strategy.GetMetrics()
		
		// Check performance thresholds
		if metrics.MaxDrawdown.GreaterThan(decimal.NewFromFloat(0.2)) {
			m.logger.Warn("Strategy high drawdown",
				zap.String("id", id),
				zap.String("drawdown", metrics.MaxDrawdown.String()))
		}
	}
}