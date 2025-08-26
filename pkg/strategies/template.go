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

// Strategy represents the base strategy interface
type Strategy interface {
	// Metadata
	GetID() string
	GetName() string
	GetType() StrategyType
	GetDescription() string
	GetVersion() string
	
	// Lifecycle
	Initialize(config StrategyConfig) error
	Start(ctx context.Context) error
	Stop() error
	Reset() error
	
	// Market Data
	OnTick(tick *types.Tick)
	OnOrderBook(book *types.OrderBook)
	OnTrade(trade *types.Trade)
	OnKline(kline *types.Kline)
	
	// Order Events
	OnOrderUpdate(order *types.Order)
	OnOrderFilled(order *types.Order)
	OnOrderCancelled(order *types.Order)
	OnOrderRejected(order *types.Order)
	
	// Position Events
	OnPositionUpdate(position *types.Position)
	OnPositionClosed(position *types.Position)
	
	// Risk Management
	CheckRiskLimits() error
	GetMaxPositionSize(symbol string) decimal.Decimal
	GetMaxDrawdown() decimal.Decimal
	
	// Performance
	GetMetrics() StrategyMetrics
	GetPnL() decimal.Decimal
	GetParameters() map[string]interface{}
	UpdateParameters(params map[string]interface{}) error
}

// StrategyType defines the type of strategy
type StrategyType string

const (
	StrategyTypeArbitrage    StrategyType = "arbitrage"
	StrategyTypeMarketMaking StrategyType = "market_making"
	StrategyTypeGrid         StrategyType = "grid"
	StrategyTypeTrend        StrategyType = "trend"
	StrategyTypeMeanReversion StrategyType = "mean_reversion"
	StrategyTypeCustom       StrategyType = "custom"
)

// StrategyConfig holds strategy configuration
type StrategyConfig struct {
	ID          string
	Name        string
	Type        StrategyType
	Version     string
	
	// Trading parameters
	Symbols     []string
	Exchanges   []string
	Accounts    []string
	
	// Risk parameters
	MaxPositionSize      decimal.Decimal
	MaxLeverage          int
	MaxDrawdownPercent   float64
	StopLossPercent      float64
	TakeProfitPercent    float64
	
	// Execution parameters
	OrderType            types.OrderType
	TimeInForce          types.TimeInForce
	MinOrderSize         decimal.Decimal
	MaxOrdersPerSymbol   int
	MaxOrdersPerSecond   int
	
	// Custom parameters
	Parameters           map[string]interface{}
}

// StrategyMetrics holds strategy performance metrics
type StrategyMetrics struct {
	StartTime           time.Time
	TotalTrades         int
	WinningTrades       int
	LosingTrades        int
	TotalPnL            decimal.Decimal
	RealizedPnL         decimal.Decimal
	UnrealizedPnL       decimal.Decimal
	MaxDrawdown         decimal.Decimal
	WinRate             float64
	ProfitFactor        float64
	SharpeRatio         decimal.Decimal
	SortinoRatio        decimal.Decimal
	AverageWin          decimal.Decimal
	AverageLoss         decimal.Decimal
	LargestWin          decimal.Decimal
	LargestLoss         decimal.Decimal
	ConsecutiveWins     int
	ConsecutiveLosses   int
	LastUpdateTime      time.Time
	
	// Order metrics
	TotalOrders         int
	ActiveOrders        int
	FilledOrders        int
	CancelledOrders     int
	RejectedOrders      int
	FailedOrders        int
	ProfitableOrders    int
}

// BaseStrategy provides common functionality for all strategies
type BaseStrategy struct {
	mu sync.RWMutex
	
	// Identity
	id          string
	name        string
	strategyType StrategyType
	version     string
	description string
	
	// Configuration
	config      StrategyConfig
	
	// State
	isRunning   bool
	ctx         context.Context
	cancel      context.CancelFunc
	
	// Dependencies (public for strategies to access)
	OrderManager    types.OrderManager
	PositionManager types.PositionManager
	MarketData      types.MarketDataProvider
	logger          *zap.Logger
	
	// Performance tracking
	metrics     StrategyMetrics
	trades      []TradeRecord
	
	// Risk management
	riskChecker types.RiskChecker
	
	// Parameters (can be updated at runtime)
	parameters  map[string]interface{}
	paramMutex  sync.RWMutex
}

// TradeRecord represents a completed trade
type TradeRecord struct {
	ID           string
	Symbol       string
	Side         types.OrderSide
	Quantity     decimal.Decimal
	EntryPrice   decimal.Decimal
	ExitPrice    decimal.Decimal
	EntryTime    time.Time
	ExitTime     time.Time
	PnL          decimal.Decimal
	Fees         decimal.Decimal
	NetPnL       decimal.Decimal
}

// NewBaseStrategy creates a new base strategy
func NewBaseStrategy(
	config StrategyConfig,
	orderManager types.OrderManager,
	positionManager types.PositionManager,
	marketData types.MarketDataProvider,
	logger *zap.Logger,
) *BaseStrategy {
	return &BaseStrategy{
		id:              config.ID,
		name:            config.Name,
		strategyType:    config.Type,
		version:         config.Version,
		config:          config,
		OrderManager:    orderManager,
		PositionManager: positionManager,
		MarketData:      marketData,
		logger:          logger,
		parameters:      config.Parameters,
		metrics: StrategyMetrics{
			StartTime: time.Now(),
		},
		trades: make([]TradeRecord, 0),
	}
}

// GetID returns the strategy ID
func (s *BaseStrategy) GetID() string {
	return s.id
}

// GetName returns the strategy name
func (s *BaseStrategy) GetName() string {
	return s.name
}

// GetType returns the strategy type
func (s *BaseStrategy) GetType() StrategyType {
	return s.strategyType
}

// GetDescription returns the strategy description
func (s *BaseStrategy) GetDescription() string {
	return s.description
}

// GetVersion returns the strategy version
func (s *BaseStrategy) GetVersion() string {
	return s.version
}

// Initialize initializes the strategy
func (s *BaseStrategy) Initialize(config StrategyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.config = config
	s.parameters = config.Parameters
	
	// Subscribe to market data
	for _, symbol := range config.Symbols {
		for _, exchange := range config.Exchanges {
			if err := s.MarketData.Subscribe(exchange, symbol); err != nil {
				return fmt.Errorf("failed to subscribe %s %s: %w", exchange, symbol, err)
			}
		}
	}
	
	s.logger.Info("Strategy initialized",
		zap.String("id", s.id),
		zap.String("name", s.name),
		zap.String("type", string(s.strategyType)))
	
	return nil
}

// Start starts the strategy
func (s *BaseStrategy) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.isRunning {
		return fmt.Errorf("strategy already running")
	}
	
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.isRunning = true
	s.metrics.StartTime = time.Now()
	
	s.logger.Info("Strategy started",
		zap.String("id", s.id),
		zap.String("name", s.name))
	
	return nil
}

// Stop stops the strategy
func (s *BaseStrategy) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if !s.isRunning {
		return fmt.Errorf("strategy not running")
	}
	
	// Cancel all pending orders
	if err := s.cancelAllOrders(); err != nil {
		s.logger.Error("Failed to cancel orders on stop", zap.Error(err))
	}
	
	// Close all positions if configured
	// This depends on strategy configuration
	
	s.cancel()
	s.isRunning = false
	
	s.logger.Info("Strategy stopped",
		zap.String("id", s.id),
		zap.String("name", s.name),
		zap.Float64("total_pnl", s.metrics.TotalPnL.InexactFloat64()))
	
	return nil
}

// Reset resets the strategy state
func (s *BaseStrategy) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.isRunning {
		return fmt.Errorf("cannot reset running strategy")
	}
	
	// Reset metrics
	s.metrics = StrategyMetrics{
		StartTime: time.Now(),
	}
	s.trades = make([]TradeRecord, 0)
	
	s.logger.Info("Strategy reset", zap.String("id", s.id))
	
	return nil
}

// CheckRiskLimits checks if risk limits are breached
func (s *BaseStrategy) CheckRiskLimits() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// Check max drawdown
	if s.config.MaxDrawdownPercent > 0 {
		currentDD := s.calculateDrawdown()
		if currentDD > s.config.MaxDrawdownPercent {
			return fmt.Errorf("max drawdown exceeded: %.2f%% > %.2f%%", 
				currentDD*100, s.config.MaxDrawdownPercent*100)
		}
	}
	
	// Check position limits
	positions, _ := s.PositionManager.GetAllPositions()
	for _, pos := range positions {
		if pos.Quantity.GreaterThan(s.config.MaxPositionSize) {
			return fmt.Errorf("position size exceeded for %s: %s > %s",
				pos.Symbol, pos.Quantity, s.config.MaxPositionSize)
		}
	}
	
	return nil
}

// GetMaxPositionSize returns max position size for a symbol
func (s *BaseStrategy) GetMaxPositionSize(symbol string) decimal.Decimal {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return s.config.MaxPositionSize
}

// GetMaxDrawdown returns the maximum drawdown
func (s *BaseStrategy) GetMaxDrawdown() decimal.Decimal {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return s.metrics.MaxDrawdown
}

// GetMetrics returns strategy metrics
func (s *BaseStrategy) GetMetrics() StrategyMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// Update calculated metrics
	s.updateMetrics()
	
	return s.metrics
}

// GetPnL returns total P&L
func (s *BaseStrategy) GetPnL() decimal.Decimal {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return s.metrics.TotalPnL
}

// GetParameters returns current parameters
func (s *BaseStrategy) GetParameters() map[string]interface{} {
	s.paramMutex.RLock()
	defer s.paramMutex.RUnlock()
	
	// Return a copy
	params := make(map[string]interface{})
	for k, v := range s.parameters {
		params[k] = v
	}
	
	return params
}

// UpdateParameters updates strategy parameters
func (s *BaseStrategy) UpdateParameters(params map[string]interface{}) error {
	s.paramMutex.Lock()
	defer s.paramMutex.Unlock()
	
	// Validate parameters
	if err := s.validateParameters(params); err != nil {
		return fmt.Errorf("invalid parameters: %w", err)
	}
	
	// Update parameters
	for k, v := range params {
		s.parameters[k] = v
	}
	
	s.logger.Info("Parameters updated",
		zap.String("strategy_id", s.id),
		zap.Any("params", params))
	
	return nil
}

// Helper methods

func (s *BaseStrategy) cancelAllOrders() error {
	orders := s.OrderManager.GetOpenOrders()
	for _, order := range orders {
		if err := s.OrderManager.CancelOrder(order.ID); err != nil {
			s.logger.Error("Failed to cancel order",
				zap.String("order_id", order.ID),
				zap.Error(err))
		}
	}
	return nil
}

func (s *BaseStrategy) calculateDrawdown() float64 {
	if len(s.trades) == 0 {
		return 0
	}
	
	peak := decimal.Zero
	maxDD := decimal.Zero
	cumPnL := decimal.Zero
	
	for _, trade := range s.trades {
		cumPnL = cumPnL.Add(trade.NetPnL)
		
		if cumPnL.GreaterThan(peak) {
			peak = cumPnL
		}
		
		drawdown := peak.Sub(cumPnL)
		if drawdown.GreaterThan(maxDD) {
			maxDD = drawdown
		}
	}
	
	if peak.IsZero() {
		return 0
	}
	
	return maxDD.Div(peak).InexactFloat64()
}

func (s *BaseStrategy) updateMetrics() {
	// Calculate win rate
	if s.metrics.TotalTrades > 0 {
		s.metrics.WinRate = float64(s.metrics.WinningTrades) / float64(s.metrics.TotalTrades)
	}
	
	// Calculate profit factor
	if s.metrics.AverageLoss.IsPositive() {
		grossProfit := s.metrics.AverageWin.Mul(decimal.NewFromInt(int64(s.metrics.WinningTrades)))
		grossLoss := s.metrics.AverageLoss.Mul(decimal.NewFromInt(int64(s.metrics.LosingTrades)))
		
		if grossLoss.IsPositive() {
			s.metrics.ProfitFactor = grossProfit.Div(grossLoss).InexactFloat64()
		}
	}
	
	s.metrics.LastUpdateTime = time.Now()
}

func (s *BaseStrategy) validateParameters(params map[string]interface{}) error {
	// Override this method in specific strategies
	return nil
}

// ValidateParameters validates parameter values
func (s *BaseStrategy) ValidateParameters(params map[string]interface{}) error {
	return s.validateParameters(params)
}

// GetConfig returns strategy configuration
func (s *BaseStrategy) GetConfig() StrategyConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return s.config
}

// PlaceOrder places an order with risk checks
func (s *BaseStrategy) PlaceOrder(req *types.OrderRequest) (*types.Order, error) {
	// Check risk limits
	if err := s.CheckRiskLimits(); err != nil {
		return nil, fmt.Errorf("risk check failed: %w", err)
	}
	
	// Place order
	order, err := s.OrderManager.PlaceOrder(req)
	if err != nil {
		return nil, fmt.Errorf("failed to place order: %w", err)
	}
	
	s.logger.Info("Order placed",
		zap.String("strategy_id", s.id),
		zap.String("order_id", order.ID),
		zap.String("symbol", order.Symbol),
		zap.String("side", string(order.Side)),
		zap.String("quantity", order.Quantity.String()))
	
	return order, nil
}

// RecordTrade records a completed trade
func (s *BaseStrategy) RecordTrade(trade TradeRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.trades = append(s.trades, trade)
	
	// Update metrics
	s.metrics.TotalTrades++
	s.metrics.TotalPnL = s.metrics.TotalPnL.Add(trade.NetPnL)
	
	if trade.NetPnL.IsPositive() {
		s.metrics.WinningTrades++
		s.metrics.ConsecutiveWins++
		s.metrics.ConsecutiveLosses = 0
		
		if trade.NetPnL.GreaterThan(s.metrics.LargestWin) {
			s.metrics.LargestWin = trade.NetPnL
		}
	} else if trade.NetPnL.IsNegative() {
		s.metrics.LosingTrades++
		s.metrics.ConsecutiveLosses++
		s.metrics.ConsecutiveWins = 0
		
		if trade.NetPnL.LessThan(s.metrics.LargestLoss) {
			s.metrics.LargestLoss = trade.NetPnL
		}
	}
}

// Default implementations for event handlers (to be overridden by specific strategies)

// OnTick handles tick data
func (s *BaseStrategy) OnTick(tick *types.Tick) {
	// Override in specific strategy
}

// OnOrderBook handles order book updates
func (s *BaseStrategy) OnOrderBook(book *types.OrderBook) {
	// Override in specific strategy
}

// OnTrade handles trade data
func (s *BaseStrategy) OnTrade(trade *types.Trade) {
	// Override in specific strategy
}

// OnKline handles kline/candle data
func (s *BaseStrategy) OnKline(kline *types.Kline) {
	// Override in specific strategy
}

// OnOrderUpdate handles order updates
func (s *BaseStrategy) OnOrderUpdate(order *types.Order) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Update order metrics
	if order.Status == types.OrderStatusNew {
		s.metrics.ActiveOrders++
	} else if order.Status == types.OrderStatusFilled {
		s.metrics.ActiveOrders--
		s.metrics.FilledOrders++
	} else if order.Status == types.OrderStatusCancelled {
		s.metrics.ActiveOrders--
		s.metrics.CancelledOrders++
	} else if order.Status == types.OrderStatusRejected {
		s.metrics.RejectedOrders++
	}
	s.metrics.TotalOrders++
}

// OnOrderFilled handles filled orders
func (s *BaseStrategy) OnOrderFilled(order *types.Order) {
	// Override in specific strategy
}

// OnOrderCancelled handles cancelled orders
func (s *BaseStrategy) OnOrderCancelled(order *types.Order) {
	// Override in specific strategy
}

// OnOrderRejected handles rejected orders
func (s *BaseStrategy) OnOrderRejected(order *types.Order) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.metrics.FailedOrders++
}

// OnPositionUpdate handles position updates
func (s *BaseStrategy) OnPositionUpdate(position *types.Position) {
	// Override in specific strategy
}

// OnPositionClosed handles closed positions
func (s *BaseStrategy) OnPositionClosed(position *types.Position) {
	// Override in specific strategy
}