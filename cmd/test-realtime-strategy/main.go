package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/mExOms/pkg/strategies"
	"github.com/mExOms/pkg/strategies/templates"
	"github.com/mExOms/pkg/types"
)

// Mock implementations
type MockOrderManager struct{}

func (m *MockOrderManager) PlaceOrder(req *types.OrderRequest) (*types.Order, error) {
	return &types.Order{
		ID:         fmt.Sprintf("order-%d", time.Now().UnixNano()),
		AccountID:  req.AccountID,
		Symbol:     req.Symbol,
		Side:       req.Side,
		Type:       req.Type,
		Quantity:   req.Quantity,
		Price:      req.Price,
		Status:     types.OrderStatusNew,
		CreateTime: time.Now(),
	}, nil
}

func (m *MockOrderManager) CancelOrder(orderID string) error {
	return nil
}

func (m *MockOrderManager) GetOrder(orderID string) (*types.Order, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *MockOrderManager) GetActiveOrders(symbol string) ([]*types.Order, error) {
	return []*types.Order{}, nil
}

type MockPositionManager struct {
	positions map[string]*types.Position
}

func NewMockPositionManager() *MockPositionManager {
	return &MockPositionManager{
		positions: make(map[string]*types.Position),
	}
}

func (m *MockPositionManager) GetPosition(symbol string) (*types.Position, error) {
	pos, exists := m.positions[symbol]
	if !exists {
		return &types.Position{
			Symbol:   symbol,
			Quantity: decimal.Zero,
			Side:     types.PositionSideLong,
		}, nil
	}
	return pos, nil
}

func (m *MockPositionManager) UpdatePosition(pos *types.Position) error {
	m.positions[pos.Symbol] = pos
	return nil
}

func (m *MockPositionManager) GetAllPositions() ([]*types.Position, error) {
	return []*types.Position{}, nil
}

func (m *MockPositionManager) GetTotalExposure() decimal.Decimal {
	return decimal.Zero
}

type MockMarketDataProvider struct{}

func (m *MockMarketDataProvider) SubscribeTicker(symbol string) error { return nil }
func (m *MockMarketDataProvider) SubscribeOrderBook(symbol string) error { return nil }
func (m *MockMarketDataProvider) UnsubscribeTicker(symbol string) error { return nil }
func (m *MockMarketDataProvider) UnsubscribeOrderBook(symbol string) error { return nil }
func (m *MockMarketDataProvider) GetOrderBook(symbol string) (*types.OrderBook, error) {
	return nil, fmt.Errorf("not implemented")
}

func main() {
	// Create logger
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	
	logger, err := config.Build()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Sync()

	ctx := context.Background()

	// Test 1: Real-time Parameter Modification
	logger.Info("=== Test 1: Real-time Parameter Modification ===")
	testRealtimeModification(ctx, logger)

	// Test 2: Adaptive Parameter Control
	logger.Info("\n=== Test 2: Adaptive Parameter Control ===")
	testAdaptiveControl(ctx, logger)

	// Test 3: Version Management
	logger.Info("\n=== Test 3: Version Management ===")
	testVersionManagement(ctx, logger)

	logger.Info("All real-time strategy tests completed")
}

func testRealtimeModification(ctx context.Context, logger *zap.Logger) {
	// Create a grid trading strategy
	strategy := templates.NewGridTemplate(
		&MockOrderManager{},
		NewMockPositionManager(),
		&MockMarketDataProvider{},
		logger,
	)

	// Initialize strategy
	config := strategies.StrategyConfig{
		ID:        "grid-rt-001",
		Name:      "Real-time Grid Strategy",
		Type:      strategies.StrategyTypeGrid,
		Symbols:   []string{"BTC-USDT"},
		Exchanges: []string{"binance"},
		Parameters: map[string]interface{}{
			"gridLevels":     10,
			"gridSpacing":    1.0,
			"upperBound":     50000.0,
			"lowerBound":     40000.0,
			"orderSize":      0.01,
			"reinvestProfit": true,
		},
	}
	strategy.Initialize(config)

	// Create real-time modifier
	modifier := strategies.NewRealtimeModifier(strategy, logger)
	
	// Add validation rules
	modifier.AddValidationRule("gridLevels", strategies.ValidationRule{
		Name: "min_max_levels",
		Validator: func(oldValue, newValue interface{}) error {
			levels := newValue.(int)
			if levels < 5 || levels > 50 {
				return fmt.Errorf("grid levels must be between 5 and 50")
			}
			return nil
		},
	})

	modifier.AddValidationRule("gridSpacing", strategies.ValidationRule{
		Name: "positive_spacing",
		Validator: func(oldValue, newValue interface{}) error {
			spacing := newValue.(float64)
			if spacing <= 0 {
				return fmt.Errorf("grid spacing must be positive")
			}
			return nil
		},
	})

	// Add update callback
	modifier.AddUpdateCallback(func(update *strategies.ParameterUpdate) {
		logger.Info("Parameter update callback",
			zap.String("parameter", update.ParameterName),
			zap.String("status", string(update.Status)),
			zap.Any("new_value", update.NewValue))
	})

	// Start modifier
	if err := modifier.Start(ctx); err != nil {
		logger.Fatal("Failed to start modifier", zap.Error(err))
	}
	defer modifier.Stop()

	// Start strategy
	if err := strategy.Start(ctx); err != nil {
		logger.Fatal("Failed to start strategy", zap.Error(err))
	}
	defer strategy.Stop()

	// Test parameter updates
	time.Sleep(1 * time.Second)

	// Update 1: Change grid levels
	logger.Info("Requesting grid levels update")
	requestID1, err := modifier.UpdateParameter("gridLevels", 15)
	if err != nil {
		logger.Error("Failed to update grid levels", zap.Error(err))
	} else {
		logger.Info("Update requested", zap.String("request_id", requestID1))
	}

	time.Sleep(2 * time.Second)

	// Update 2: Change grid spacing
	logger.Info("Requesting grid spacing update")
	requestID2, err := modifier.UpdateParameter("gridSpacing", 1.5)
	if err != nil {
		logger.Error("Failed to update grid spacing", zap.Error(err))
	} else {
		logger.Info("Update requested", zap.String("request_id", requestID2))
	}

	time.Sleep(2 * time.Second)

	// Update 3: Invalid update (should fail validation)
	logger.Info("Requesting invalid update (negative spacing)")
	requestID3, err := modifier.UpdateParameter("gridSpacing", -0.5)
	if err != nil {
		logger.Info("Update correctly rejected", zap.Error(err))
	}

	time.Sleep(2 * time.Second)

	// Check update status
	if status, err := modifier.GetUpdateStatus(requestID1); err == nil {
		logger.Info("Update 1 status",
			zap.String("status", string(status.Status)),
			zap.Error(status.Error))
	}

	// Get update history
	history := modifier.GetUpdateHistory(10)
	logger.Info("Update history", zap.Int("total_updates", len(history)))
	for _, update := range history {
		logger.Info("Historical update",
			zap.String("parameter", update.ParameterName),
			zap.String("status", string(update.Status)),
			zap.Time("timestamp", update.Timestamp))
	}
}

func testAdaptiveControl(ctx context.Context, logger *zap.Logger) {
	// Create market making strategy
	strategy := templates.NewMarketMakingTemplate(
		&MockOrderManager{},
		NewMockPositionManager(),
		&MockMarketDataProvider{},
		logger,
	)

	// Initialize strategy
	config := strategies.StrategyConfig{
		ID:        "mm-adaptive-001",
		Name:      "Adaptive Market Making",
		Type:      strategies.StrategyTypeMarketMaking,
		Symbols:   []string{"ETH-USDT"},
		Exchanges: []string{"binance"},
		Parameters: map[string]interface{}{
			"spreadBps":       50,
			"orderDepth":      5,
			"sizingMode":      "dynamic",
			"maxInventory":    1.0,
			"inventoryTarget": 0.5,
			"skewFactor":      0.1,
		},
	}
	strategy.Initialize(config)

	// Create real-time modifier
	modifier := strategies.NewRealtimeModifier(strategy, logger)
	modifier.Start(ctx)
	defer modifier.Stop()

	// Create adaptive controller
	controller := strategies.NewAdaptiveController(strategy, modifier, logger)

	// Add adaptation rules
	controller.AddAdaptationRule(strategies.AdaptationRule{
		ParameterName:   "spreadBps",
		BaseValue:       50,
		Adjustments: map[strategies.MarketRegime]float64{
			strategies.MarketRegimeTrending:   0.8,  // Tighten spread in trending
			strategies.MarketRegimeRangebound: 1.0,  // Normal spread
			strategies.MarketRegimeVolatile:   1.5,  // Widen spread in volatile
			strategies.MarketRegimeQuiet:      1.2,  // Slightly wider in quiet
		},
		MinValue:        20,
		MaxValue:        100,
		AdaptationSpeed: 0.3,
	})

	controller.AddAdaptationRule(strategies.AdaptationRule{
		ParameterName:   "orderDepth",
		BaseValue:       5,
		Adjustments: map[strategies.MarketRegime]float64{
			strategies.MarketRegimeTrending:   1.2,  // More depth in trending
			strategies.MarketRegimeRangebound: 1.0,  // Normal depth
			strategies.MarketRegimeVolatile:   0.7,  // Less depth in volatile
			strategies.MarketRegimeQuiet:      0.8,  // Less depth in quiet
		},
		MinValue:        3,
		MaxValue:        10,
		AdaptationSpeed: 0.2,
	})

	// Start controller
	if err := controller.Start(ctx); err != nil {
		logger.Fatal("Failed to start adaptive controller", zap.Error(err))
	}
	defer controller.Stop()

	// Start strategy
	if err := strategy.Start(ctx); err != nil {
		logger.Fatal("Failed to start strategy", zap.Error(err))
	}
	defer strategy.Stop()

	// Simulate market regime changes
	go simulateMarketData(ctx, controller, logger)

	// Let adaptation run
	time.Sleep(30 * time.Second)

	// Check current regimes
	regimes := controller.GetCurrentRegimes()
	for symbol, regime := range regimes {
		logger.Info("Current market regime",
			zap.String("symbol", symbol),
			zap.String("regime", string(regime)))
	}

	// Check current parameters
	params := strategy.GetParameters()
	logger.Info("Current adapted parameters",
		zap.Any("parameters", params))
}

func testVersionManagement(ctx context.Context, logger *zap.Logger) {
	// Create arbitrage strategy
	strategy := templates.NewArbitrageTemplate(
		&MockOrderManager{},
		NewMockPositionManager(),
		&MockMarketDataProvider{},
		logger,
	)

	// Initialize strategy
	config := strategies.StrategyConfig{
		ID:        "arb-version-001",
		Name:      "Versioned Arbitrage Strategy",
		Type:      strategies.StrategyTypeArbitrage,
		Symbols:   []string{"BTC-USDT"},
		Exchanges: []string{"binance", "bybit"},
		Parameters: map[string]interface{}{
			"minSpreadPercent": 0.5,
			"maxPositionSize":  1.0,
			"executionMode":    "conservative",
		},
	}
	strategy.Initialize(config)

	// Create version manager
	versionManager := strategies.NewVersionManager(logger)

	// Create initial version
	v1ID, err := versionManager.CreateVersion(
		config.ID,
		config.Parameters,
		"Initial conservative configuration",
		"system",
		[]string{"production", "conservative"},
	)
	if err != nil {
		logger.Fatal("Failed to create version", zap.Error(err))
	}

	// Deploy initial version
	if err := versionManager.DeployVersion(config.ID, v1ID, strategy, "initial_deployment"); err != nil {
		logger.Fatal("Failed to deploy version", zap.Error(err))
	}

	// Start strategy
	ctx, cancel := context.WithCancel(ctx)
	if err := strategy.Start(ctx); err != nil {
		logger.Fatal("Failed to start strategy", zap.Error(err))
	}

	// Simulate some trading
	go simulateTrading(ctx, strategy, logger)

	time.Sleep(5 * time.Second)

	// Create more aggressive version
	v2Params := map[string]interface{}{
		"minSpreadPercent": 0.3,
		"maxPositionSize":  2.0,
		"executionMode":    "aggressive",
	}

	v2ID, err := versionManager.CreateVersion(
		config.ID,
		v2Params,
		"More aggressive configuration for higher volume",
		"user",
		[]string{"test", "aggressive"},
	)
	if err != nil {
		logger.Error("Failed to create version 2", zap.Error(err))
	}

	// Deploy new version
	logger.Info("Deploying aggressive version")
	if err := versionManager.DeployVersion(config.ID, v2ID, strategy, "performance_improvement"); err != nil {
		logger.Error("Failed to deploy version 2", zap.Error(err))
	}

	time.Sleep(5 * time.Second)

	// Create another version
	v3Params := map[string]interface{}{
		"minSpreadPercent": 0.4,
		"maxPositionSize":  1.5,
		"executionMode":    "balanced",
	}

	v3ID, err := versionManager.CreateVersion(
		config.ID,
		v3Params,
		"Balanced configuration",
		"optimizer",
		[]string{"optimized", "balanced"},
	)
	if err != nil {
		logger.Error("Failed to create version 3", zap.Error(err))
	}

	// Get active version
	if activeVersion, err := versionManager.GetActiveVersion(config.ID); err == nil {
		logger.Info("Current active version",
			zap.String("version_id", activeVersion.VersionID),
			zap.String("description", activeVersion.Description),
			zap.Any("parameters", activeVersion.Parameters))
	}

	// Get version history
	history := versionManager.GetVersionHistory(config.ID, 10)
	logger.Info("Version history", zap.Int("total_versions", len(history)))
	for _, version := range history {
		logger.Info("Version",
			zap.String("id", version.VersionID),
			zap.String("description", version.Description),
			zap.Time("created", version.CreatedAt),
			zap.Strings("tags", version.Tags))
	}

	// Get deployment history
	deployments := versionManager.GetDeploymentHistory(config.ID, 10)
	logger.Info("Deployment history", zap.Int("total_deployments", len(deployments)))
	for _, deployment := range deployments {
		logger.Info("Deployment",
			zap.String("id", deployment.DeploymentID),
			zap.String("version", deployment.VersionID),
			zap.String("status", string(deployment.Status)),
			zap.Time("start", deployment.StartTime))
	}

	// Rollback to conservative version
	logger.Info("Rolling back to conservative version")
	if err := versionManager.RollbackVersion(config.ID, v1ID, strategy, "manual_rollback"); err != nil {
		logger.Error("Failed to rollback", zap.Error(err))
	}

	time.Sleep(2 * time.Second)

	// Stop everything
	cancel()
	strategy.Stop()

	logger.Info("Version management test completed")
}

// simulateMarketData simulates market data updates
func simulateMarketData(ctx context.Context, controller *strategies.AdaptiveController, logger *zap.Logger) {
	basePrice := decimal.NewFromInt(3000) // ETH price
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
			// Generate random price movement
			change := decimal.NewFromFloat(rand.Float64()*60 - 30) // ±30
			newPrice := basePrice.Add(change)
			volume := decimal.NewFromFloat(rand.Float64() * 1000)
			
			// Update market analyzers (simplified - normally would come from real data)
			// This is just for demonstration
			basePrice = newPrice
		}
	}
}

// simulateTrading simulates trading activity
func simulateTrading(ctx context.Context, strategy strategies.Strategy, logger *zap.Logger) {
	orderCount := 0
	winCount := 0
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
			// Simulate order events
			orderCount++
			if rand.Float64() > 0.4 { // 60% win rate
				winCount++
			}
			
			// Update strategy metrics (simplified)
			// In real implementation, this would be done by the strategy itself
		}
	}
}