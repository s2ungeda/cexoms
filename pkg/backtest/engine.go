package backtest

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	
	"github.com/mExOms/pkg/strategies"
	"github.com/mExOms/pkg/types"
)

// BacktestEngine runs strategy backtests
type BacktestEngine struct {
	mu sync.RWMutex
	
	// Configuration
	config          *BacktestConfig
	
	// Strategy to test
	strategy        strategies.Strategy
	
	// Market data
	dataProvider    DataProvider
	orderBook       *SimulatedOrderBook
	
	// Simulation state
	currentTime     time.Time
	orders          map[string]*SimulatedOrder
	positions       map[string]*SimulatedPosition
	trades          []TradeExecution
	
	// Account state
	balance         map[string]decimal.Decimal // asset -> balance
	initialBalance  map[string]decimal.Decimal
	
	// Performance tracking
	equity          []EquityPoint
	metrics         *BacktestMetrics
	
	// Logger
	logger          *zap.Logger
}

// BacktestConfig holds backtest configuration
type BacktestConfig struct {
	StartTime       time.Time
	EndTime         time.Time
	InitialBalance  map[string]decimal.Decimal
	
	// Simulation parameters
	TickInterval    time.Duration
	SpreadBps       int             // Simulated spread in basis points
	SlippageBps     int             // Simulated slippage
	TakerFeeBps     int             // Taker fee in basis points
	MakerFeeBps     int             // Maker fee in basis points
	
	// Data parameters
	DataFrequency   string          // "tick", "1m", "5m", etc.
	Symbols         []string
	Exchanges       []string
}

// DataProvider provides historical market data
type DataProvider interface {
	// GetNextTick returns the next tick data
	GetNextTick() (*types.Tick, error)
	
	// GetNextCandle returns the next candle data
	GetNextCandle() (*types.Kline, error)
	
	// GetOrderBook returns order book snapshot at current time
	GetOrderBook(symbol string) (*types.OrderBook, error)
	
	// HasMoreData checks if more data is available
	HasMoreData() bool
	
	// Reset resets the data provider
	Reset() error
}

// SimulatedOrder represents an order in backtest
type SimulatedOrder struct {
	Order           types.Order
	PlacedAt        time.Time
	FilledAt        time.Time
	FilledPrice     decimal.Decimal
	Slippage        decimal.Decimal
	Fee             decimal.Decimal
}

// SimulatedPosition represents a position in backtest
type SimulatedPosition struct {
	Symbol          string
	Side            types.PositionSide
	Quantity        decimal.Decimal
	EntryPrice      decimal.Decimal
	EntryTime       time.Time
	CurrentPrice    decimal.Decimal
	RealizedPnL     decimal.Decimal
	UnrealizedPnL   decimal.Decimal
}

// TradeExecution represents a trade execution
type TradeExecution struct {
	OrderID         string
	Symbol          string
	Side            types.OrderSide
	Quantity        decimal.Decimal
	Price           decimal.Decimal
	Fee             decimal.Decimal
	Timestamp       time.Time
}

// EquityPoint represents account equity at a point in time
type EquityPoint struct {
	Timestamp       time.Time
	Equity          decimal.Decimal
	Balance         map[string]decimal.Decimal
	Positions       int
	UnrealizedPnL   decimal.Decimal
}

// BacktestMetrics holds backtest performance metrics
type BacktestMetrics struct {
	// Returns
	TotalReturn     decimal.Decimal
	AnnualizedReturn decimal.Decimal
	
	// Risk metrics
	Volatility      decimal.Decimal
	SharpeRatio     decimal.Decimal
	SortinoRatio    decimal.Decimal
	MaxDrawdown     decimal.Decimal
	MaxDrawdownDuration time.Duration
	
	// Trading metrics
	TotalTrades     int
	WinningTrades   int
	LosingTrades    int
	WinRate         float64
	AverageWin      decimal.Decimal
	AverageLoss     decimal.Decimal
	ProfitFactor    decimal.Decimal
	
	// Fees and costs
	TotalFees       decimal.Decimal
	TotalSlippage   decimal.Decimal
	
	// Time metrics
	TimeInMarket    float64 // Percentage of time with positions
	LongestWinStreak int
	LongestLossStreak int
}

// NewBacktestEngine creates a new backtest engine
func NewBacktestEngine(
	config *BacktestConfig,
	strategy strategies.Strategy,
	dataProvider DataProvider,
	logger *zap.Logger,
) *BacktestEngine {
	engine := &BacktestEngine{
		config:         config,
		strategy:       strategy,
		dataProvider:   dataProvider,
		logger:         logger,
		currentTime:    config.StartTime,
		orders:         make(map[string]*SimulatedOrder),
		positions:      make(map[string]*SimulatedPosition),
		trades:         make([]TradeExecution, 0),
		balance:        make(map[string]decimal.Decimal),
		initialBalance: make(map[string]decimal.Decimal),
		equity:         make([]EquityPoint, 0),
		metrics:        &BacktestMetrics{},
	}
	
	// Initialize balance
	for asset, amount := range config.InitialBalance {
		engine.balance[asset] = amount
		engine.initialBalance[asset] = amount
	}
	
	// Create simulated order book
	engine.orderBook = NewSimulatedOrderBook(config.SpreadBps)
	
	return engine
}

// Run runs the backtest
func (e *BacktestEngine) Run(ctx context.Context) error {
	e.logger.Info("Starting backtest",
		zap.Time("start_time", e.config.StartTime),
		zap.Time("end_time", e.config.EndTime))
	
	// Initialize strategy
	strategyConfig := strategies.StrategyConfig{
		Symbols:   e.config.Symbols,
		Exchanges: e.config.Exchanges,
	}
	
	if err := e.strategy.Initialize(strategyConfig); err != nil {
		return fmt.Errorf("failed to initialize strategy: %w", err)
	}
	
	// Start strategy
	if err := e.strategy.Start(ctx); err != nil {
		return fmt.Errorf("failed to start strategy: %w", err)
	}
	defer e.strategy.Stop()
	
	// Record initial equity
	e.recordEquity()
	
	// Main backtest loop
	for e.dataProvider.HasMoreData() && e.currentTime.Before(e.config.EndTime) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Process next tick
			if err := e.processNextTick(); err != nil {
				return fmt.Errorf("failed to process tick: %w", err)
			}
		}
	}
	
	// Close all positions at end
	e.closeAllPositions()
	
	// Calculate final metrics
	e.calculateMetrics()
	
	e.logger.Info("Backtest completed",
		zap.Any("metrics", e.metrics))
	
	return nil
}

// processNextTick processes the next tick of data
func (e *BacktestEngine) processNextTick() error {
	// Get next tick
	tick, err := e.dataProvider.GetNextTick()
	if err != nil {
		return err
	}
	
	e.currentTime = tick.Time
	
	// Update order book
	e.orderBook.UpdatePrice(tick.Symbol, tick.Price)
	
	// Update positions
	e.updatePositions(tick)
	
	// Check order fills
	e.checkOrderFills(tick)
	
	// Send tick to strategy
	e.strategy.OnTick(tick)
	
	// Get order book snapshot
	orderBook, err := e.dataProvider.GetOrderBook(tick.Symbol)
	if err == nil {
		e.strategy.OnOrderBook(orderBook)
	}
	
	// Record equity periodically
	if len(e.equity) == 0 || time.Since(e.equity[len(e.equity)-1].Timestamp) > time.Minute {
		e.recordEquity()
	}
	
	return nil
}

// updatePositions updates position prices and P&L
func (e *BacktestEngine) updatePositions(tick *types.Tick) {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	for symbol, pos := range e.positions {
		if symbol == tick.Symbol {
			pos.CurrentPrice = tick.Price
			
			// Calculate unrealized P&L
			if pos.Side == types.PositionSideLong {
				pos.UnrealizedPnL = tick.Price.Sub(pos.EntryPrice).Mul(pos.Quantity)
			} else {
				pos.UnrealizedPnL = pos.EntryPrice.Sub(tick.Price).Mul(pos.Quantity)
			}
		}
	}
}

// checkOrderFills checks if any orders should be filled
func (e *BacktestEngine) checkOrderFills(tick *types.Tick) {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	for id, simOrder := range e.orders {
		order := &simOrder.Order
		
		// Skip if already filled or not for this symbol
		if order.Status != types.OrderStatusNew || order.Symbol != tick.Symbol {
			continue
		}
		
		shouldFill := false
		fillPrice := order.Price
		
		// Check fill conditions
		switch order.Type {
		case types.OrderTypeMarket:
			shouldFill = true
			// Add slippage for market orders
			slippage := tick.Price.Mul(decimal.NewFromInt(int64(e.config.SlippageBps))).
				Div(decimal.NewFromInt(10000))
			
			if order.Side == types.OrderSideBuy {
				fillPrice = tick.Price.Add(slippage)
			} else {
				fillPrice = tick.Price.Sub(slippage)
			}
			
		case types.OrderTypeLimit:
			if order.Side == types.OrderSideBuy && tick.Price.LessThanOrEqual(order.Price) {
				shouldFill = true
			} else if order.Side == types.OrderSideSell && tick.Price.GreaterThanOrEqual(order.Price) {
				shouldFill = true
			}
		}
		
		if shouldFill {
			e.fillOrder(id, simOrder, fillPrice, tick.Time)
		}
	}
}

// fillOrder fills an order
func (e *BacktestEngine) fillOrder(orderID string, simOrder *SimulatedOrder, fillPrice decimal.Decimal, fillTime time.Time) {
	order := &simOrder.Order
	
	// Calculate fee
	feeBps := e.config.TakerFeeBps
	if order.Type == types.OrderTypeLimit {
		feeBps = e.config.MakerFeeBps
	}
	
	fee := fillPrice.Mul(order.Quantity).Mul(decimal.NewFromInt(int64(feeBps))).
		Div(decimal.NewFromInt(10000))
	
	// Update order
	order.Status = types.OrderStatusFilled
	order.FilledQuantity = order.Quantity
	order.UpdateTime = fillTime
	
	simOrder.FilledAt = fillTime
	simOrder.FilledPrice = fillPrice
	simOrder.Fee = fee
	
	// Update balance
	if order.Side == types.OrderSideBuy {
		// Deduct quote currency
		quoteCurrency := "USDT" // Simplified
		cost := fillPrice.Mul(order.Quantity).Add(fee)
		e.balance[quoteCurrency] = e.balance[quoteCurrency].Sub(cost)
		
		// Add base currency to position
		e.addToPosition(order.Symbol, types.PositionSideLong, order.Quantity, fillPrice, fillTime)
	} else {
		// Add quote currency
		quoteCurrency := "USDT"
		proceeds := fillPrice.Mul(order.Quantity).Sub(fee)
		e.balance[quoteCurrency] = e.balance[quoteCurrency].Add(proceeds)
		
		// Reduce position
		e.reducePosition(order.Symbol, order.Quantity, fillPrice, fillTime)
	}
	
	// Record trade
	trade := TradeExecution{
		OrderID:   orderID,
		Symbol:    order.Symbol,
		Side:      order.Side,
		Quantity:  order.Quantity,
		Price:     fillPrice,
		Fee:       fee,
		Timestamp: fillTime,
	}
	e.trades = append(e.trades, trade)
	
	// Notify strategy
	e.strategy.OnOrderFilled(order)
	
	e.logger.Debug("Order filled",
		zap.String("order_id", orderID),
		zap.String("symbol", order.Symbol),
		zap.String("side", string(order.Side)),
		zap.String("price", fillPrice.String()),
		zap.String("quantity", order.Quantity.String()),
		zap.String("fee", fee.String()))
}

// addToPosition adds to or creates a position
func (e *BacktestEngine) addToPosition(symbol string, side types.PositionSide, quantity, price decimal.Decimal, timestamp time.Time) {
	pos, exists := e.positions[symbol]
	if !exists {
		// Create new position
		e.positions[symbol] = &SimulatedPosition{
			Symbol:      symbol,
			Side:        side,
			Quantity:    quantity,
			EntryPrice:  price,
			EntryTime:   timestamp,
			CurrentPrice: price,
		}
	} else {
		// Add to existing position (average entry price)
		totalValue := pos.EntryPrice.Mul(pos.Quantity).Add(price.Mul(quantity))
		pos.Quantity = pos.Quantity.Add(quantity)
		pos.EntryPrice = totalValue.Div(pos.Quantity)
	}
}

// reducePosition reduces or closes a position
func (e *BacktestEngine) reducePosition(symbol string, quantity, exitPrice decimal.Decimal, timestamp time.Time) {
	pos, exists := e.positions[symbol]
	if !exists {
		return
	}
	
	// Calculate realized P&L
	var realizedPnL decimal.Decimal
	if pos.Side == types.PositionSideLong {
		realizedPnL = exitPrice.Sub(pos.EntryPrice).Mul(quantity)
	} else {
		realizedPnL = pos.EntryPrice.Sub(exitPrice).Mul(quantity)
	}
	
	pos.RealizedPnL = pos.RealizedPnL.Add(realizedPnL)
	pos.Quantity = pos.Quantity.Sub(quantity)
	
	// Remove position if fully closed
	if pos.Quantity.IsZero() {
		delete(e.positions, symbol)
	}
}

// recordEquity records current account equity
func (e *BacktestEngine) recordEquity() {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	// Calculate total equity
	equity := decimal.Zero
	
	// Add balances
	for asset, balance := range e.balance {
		// Convert to base currency (simplified - assumes USDT base)
		if asset == "USDT" {
			equity = equity.Add(balance)
		}
		// Would need price conversion for other assets
	}
	
	// Add unrealized P&L
	unrealizedPnL := decimal.Zero
	for _, pos := range e.positions {
		unrealizedPnL = unrealizedPnL.Add(pos.UnrealizedPnL)
	}
	
	equity = equity.Add(unrealizedPnL)
	
	// Record equity point
	point := EquityPoint{
		Timestamp:     e.currentTime,
		Equity:        equity,
		Balance:       make(map[string]decimal.Decimal),
		Positions:     len(e.positions),
		UnrealizedPnL: unrealizedPnL,
	}
	
	// Copy balances
	for asset, balance := range e.balance {
		point.Balance[asset] = balance
	}
	
	e.equity = append(e.equity, point)
}

// closeAllPositions closes all open positions at market
func (e *BacktestEngine) closeAllPositions() {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	for symbol, pos := range e.positions {
		// Create market order to close
		side := types.OrderSideSell
		if pos.Side == types.PositionSideShort {
			side = types.OrderSideBuy
		}
		
		order := &types.Order{
			ID:       fmt.Sprintf("close-%s-%d", symbol, time.Now().UnixNano()),
			Symbol:   symbol,
			Side:     side,
			Type:     types.OrderTypeMarket,
			Quantity: pos.Quantity,
			Status:   types.OrderStatusNew,
		}
		
		simOrder := &SimulatedOrder{
			Order:    *order,
			PlacedAt: e.currentTime,
		}
		
		// Fill immediately at current price
		e.fillOrder(order.ID, simOrder, pos.CurrentPrice, e.currentTime)
	}
}

// calculateMetrics calculates final backtest metrics
func (e *BacktestEngine) calculateMetrics() {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	if len(e.equity) < 2 {
		return
	}
	
	// Calculate returns
	initialEquity := e.equity[0].Equity
	finalEquity := e.equity[len(e.equity)-1].Equity
	
	e.metrics.TotalReturn = finalEquity.Sub(initialEquity).Div(initialEquity)
	
	// Calculate annualized return
	days := e.config.EndTime.Sub(e.config.StartTime).Hours() / 24
	if days > 0 {
		annualizedMultiple := math.Pow(1+e.metrics.TotalReturn.InexactFloat64(), 365/days)
		e.metrics.AnnualizedReturn = decimal.NewFromFloat(annualizedMultiple - 1)
	}
	
	// Calculate volatility and Sharpe ratio
	e.calculateRiskMetrics()
	
	// Calculate trading metrics
	e.calculateTradingMetrics()
	
	// Calculate drawdown
	e.calculateDrawdown()
}

// calculateRiskMetrics calculates risk-related metrics
func (e *BacktestEngine) calculateRiskMetrics() {
	if len(e.equity) < 2 {
		return
	}
	
	// Calculate daily returns
	returns := make([]float64, 0)
	for i := 1; i < len(e.equity); i++ {
		prevEquity := e.equity[i-1].Equity
		currEquity := e.equity[i].Equity
		
		if !prevEquity.IsZero() {
			ret := currEquity.Sub(prevEquity).Div(prevEquity).InexactFloat64()
			returns = append(returns, ret)
		}
	}
	
	if len(returns) == 0 {
		return
	}
	
	// Calculate mean return
	sum := 0.0
	for _, r := range returns {
		sum += r
	}
	meanReturn := sum / float64(len(returns))
	
	// Calculate standard deviation
	sumSquares := 0.0
	for _, r := range returns {
		sumSquares += math.Pow(r-meanReturn, 2)
	}
	
	variance := sumSquares / float64(len(returns))
	stdDev := math.Sqrt(variance)
	
	// Annualize volatility (assuming minute data)
	periodsPerYear := 365 * 24 * 60
	annualizedVol := stdDev * math.Sqrt(float64(periodsPerYear))
	e.metrics.Volatility = decimal.NewFromFloat(annualizedVol)
	
	// Calculate Sharpe ratio (assuming 0% risk-free rate)
	if annualizedVol > 0 {
		annualizedReturn := e.metrics.AnnualizedReturn.InexactFloat64()
		e.metrics.SharpeRatio = decimal.NewFromFloat(annualizedReturn / annualizedVol)
	}
	
	// Calculate Sortino ratio (downside volatility)
	downsideReturns := make([]float64, 0)
	for _, r := range returns {
		if r < 0 {
			downsideReturns = append(downsideReturns, r)
		}
	}
	
	if len(downsideReturns) > 0 {
		downsideSum := 0.0
		for _, r := range downsideReturns {
			downsideSum += r * r
		}
		
		downsideVol := math.Sqrt(downsideSum / float64(len(downsideReturns)))
		annualizedDownsideVol := downsideVol * math.Sqrt(float64(periodsPerYear))
		
		if annualizedDownsideVol > 0 {
			e.metrics.SortinoRatio = decimal.NewFromFloat(
				e.metrics.AnnualizedReturn.InexactFloat64() / annualizedDownsideVol)
		}
	}
}

// calculateTradingMetrics calculates trading-related metrics
func (e *BacktestEngine) calculateTradingMetrics() {
	e.metrics.TotalTrades = len(e.trades)
	
	if e.metrics.TotalTrades == 0 {
		return
	}
	
	// Group trades by position
	positions := make(map[string][]TradeExecution)
	for _, trade := range e.trades {
		key := fmt.Sprintf("%s-%s", trade.Symbol, trade.Timestamp.Format("2006-01-02"))
		positions[key] = append(positions[key], trade)
	}
	
	// Analyze closed positions
	totalWins := decimal.Zero
	totalLosses := decimal.Zero
	winStreak := 0
	lossStreak := 0
	currentWinStreak := 0
	currentLossStreak := 0
	
	for _, trades := range positions {
		// Calculate P&L for this position
		var pnl decimal.Decimal
		for _, trade := range trades {
			if trade.Side == types.OrderSideBuy {
				pnl = pnl.Sub(trade.Price.Mul(trade.Quantity))
			} else {
				pnl = pnl.Add(trade.Price.Mul(trade.Quantity))
			}
			pnl = pnl.Sub(trade.Fee)
		}
		
		if pnl.IsPositive() {
			e.metrics.WinningTrades++
			totalWins = totalWins.Add(pnl)
			currentWinStreak++
			currentLossStreak = 0
			if currentWinStreak > winStreak {
				winStreak = currentWinStreak
			}
		} else if pnl.IsNegative() {
			e.metrics.LosingTrades++
			totalLosses = totalLosses.Add(pnl.Abs())
			currentLossStreak++
			currentWinStreak = 0
			if currentLossStreak > lossStreak {
				lossStreak = currentLossStreak
			}
		}
		
		// Add fees
		for _, trade := range trades {
			e.metrics.TotalFees = e.metrics.TotalFees.Add(trade.Fee)
		}
	}
	
	// Calculate win rate
	if e.metrics.TotalTrades > 0 {
		e.metrics.WinRate = float64(e.metrics.WinningTrades) / float64(len(positions))
	}
	
	// Calculate average win/loss
	if e.metrics.WinningTrades > 0 {
		e.metrics.AverageWin = totalWins.Div(decimal.NewFromInt(int64(e.metrics.WinningTrades)))
	}
	
	if e.metrics.LosingTrades > 0 {
		e.metrics.AverageLoss = totalLosses.Div(decimal.NewFromInt(int64(e.metrics.LosingTrades)))
	}
	
	// Calculate profit factor
	if totalLosses.IsPositive() {
		e.metrics.ProfitFactor = totalWins.Div(totalLosses)
	}
	
	e.metrics.LongestWinStreak = winStreak
	e.metrics.LongestLossStreak = lossStreak
	
	// Calculate time in market
	totalTime := e.config.EndTime.Sub(e.config.StartTime)
	timeInMarket := 0.0
	
	for i := 1; i < len(e.equity); i++ {
		if e.equity[i].Positions > 0 {
			timeInMarket += e.equity[i].Timestamp.Sub(e.equity[i-1].Timestamp).Seconds()
		}
	}
	
	if totalTime.Seconds() > 0 {
		e.metrics.TimeInMarket = timeInMarket / totalTime.Seconds()
	}
}

// calculateDrawdown calculates maximum drawdown
func (e *BacktestEngine) calculateDrawdown() {
	if len(e.equity) < 2 {
		return
	}
	
	peak := e.equity[0].Equity
	maxDrawdown := decimal.Zero
	drawdownStart := e.equity[0].Timestamp
	maxDrawdownDuration := time.Duration(0)
	
	for _, point := range e.equity {
		// Update peak
		if point.Equity.GreaterThan(peak) {
			peak = point.Equity
			drawdownStart = point.Timestamp
		}
		
		// Calculate drawdown
		drawdown := peak.Sub(point.Equity).Div(peak)
		if drawdown.GreaterThan(maxDrawdown) {
			maxDrawdown = drawdown
		}
		
		// Track drawdown duration
		if drawdown.IsPositive() {
			duration := point.Timestamp.Sub(drawdownStart)
			if duration > maxDrawdownDuration {
				maxDrawdownDuration = duration
			}
		}
	}
	
	e.metrics.MaxDrawdown = maxDrawdown
	e.metrics.MaxDrawdownDuration = maxDrawdownDuration
}

// GetMetrics returns the backtest metrics
func (e *BacktestEngine) GetMetrics() *BacktestMetrics {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	return e.metrics
}

// GetEquityCurve returns the equity curve
func (e *BacktestEngine) GetEquityCurve() []EquityPoint {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	// Return a copy
	curve := make([]EquityPoint, len(e.equity))
	copy(curve, e.equity)
	return curve
}

// GetTrades returns all executed trades
func (e *BacktestEngine) GetTrades() []TradeExecution {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	// Return a copy
	trades := make([]TradeExecution, len(e.trades))
	copy(trades, e.trades)
	return trades
}

// PlaceOrder implements order placement for strategies during backtest
func (e *BacktestEngine) PlaceOrder(req *types.OrderRequest) (*types.Order, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	// Create order
	order := &types.Order{
		ID:          fmt.Sprintf("order-%d", time.Now().UnixNano()),
		AccountID:   req.AccountID,
		Exchange:    req.Exchange,
		Symbol:      req.Symbol,
		Side:        req.Side,
		Type:        req.Type,
		Status:      types.OrderStatusNew,
		Quantity:    req.Quantity,
		Price:       req.Price,
		TimeInForce: req.TimeInForce,
		CreateTime:  e.currentTime,
		UpdateTime:  e.currentTime,
	}
	
	// Create simulated order
	simOrder := &SimulatedOrder{
		Order:    *order,
		PlacedAt: e.currentTime,
	}
	
	e.orders[order.ID] = simOrder
	
	// Notify strategy
	e.strategy.OnOrderUpdate(order)
	
	return order, nil
}

// CancelOrder implements order cancellation for strategies during backtest
func (e *BacktestEngine) CancelOrder(orderID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	simOrder, exists := e.orders[orderID]
	if !exists {
		return fmt.Errorf("order not found: %s", orderID)
	}
	
	if simOrder.Order.Status != types.OrderStatusNew {
		return fmt.Errorf("order not cancellable: %s", simOrder.Order.Status)
	}
	
	// Update order status
	simOrder.Order.Status = types.OrderStatusCancelled
	simOrder.Order.UpdateTime = e.currentTime
	
	// Notify strategy
	e.strategy.OnOrderCancelled(&simOrder.Order)
	
	// Remove from active orders
	delete(e.orders, orderID)
	
	return nil
}