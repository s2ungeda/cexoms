package backtest

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/your-org/mExOms/pkg/types"
	"go.uber.org/zap"
)

// BacktestEngine runs strategy backtests
type BacktestEngine struct {
	config             BacktestConfig
	strategy           types.Strategy
	dataProvider       DataProvider
	slippageModel      SlippageModel
	positionManager    *PositionManager
	performanceTracker *PerformanceTracker
	orderBook          *SimulatedOrderBook
	eventQueue         *EventQueue
	logger             *zap.Logger
	
	currentTime        time.Time
	pendingOrders      map[string]*types.Order
	executedOrders     map[string]*types.Order
	orderCounter       int64
	
	mu                 sync.RWMutex
	ctx                context.Context
	cancel             context.CancelFunc
}

// NewBacktestEngine creates a new backtest engine
func NewBacktestEngine(config BacktestConfig, strategy types.Strategy, logger *zap.Logger) *BacktestEngine {
	ctx, cancel := context.WithCancel(context.Background())
	
	// Create components
	dataProvider := NewFileDataProvider(config.DataPath)
	slippageModel := CreateSlippageModel(config.SlippageModel, config.SlippageParams)
	
	return &BacktestEngine{
		config:             config,
		strategy:           strategy,
		dataProvider:       dataProvider,
		slippageModel:      slippageModel,
		positionManager:    NewPositionManager(config.InitialCapital),
		performanceTracker: NewPerformanceTracker(),
		orderBook:          NewSimulatedOrderBook(),
		eventQueue:         NewEventQueue(100000),
		logger:             logger,
		pendingOrders:      make(map[string]*types.Order),
		executedOrders:     make(map[string]*types.Order),
		ctx:                ctx,
		cancel:             cancel,
	}
}

// Run executes the backtest
func (e *BacktestEngine) Run() (*BacktestResult, error) {
	startTime := time.Now()
	
	e.logger.Info("Starting backtest",
		zap.Time("start_date", e.config.StartTime),
		zap.Time("end_date", e.config.EndTime),
		zap.String("strategy", e.config.StrategyName))
	
	// Initialize
	if err := e.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize: %w", err)
	}
	
	// Load historical data
	if err := e.loadData(); err != nil {
		return nil, fmt.Errorf("failed to load data: %w", err)
	}
	
	// Initialize strategy
	if err := e.strategy.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize strategy: %w", err)
	}
	
	// Set initial equity
	e.performanceTracker.SetInitialCapital(e.config.InitialCapital)
	e.performanceTracker.UpdateEquity(e.config.StartTime, e.config.InitialCapital)
	
	// Main event loop
	if err := e.runEventLoop(); err != nil {
		return nil, fmt.Errorf("event loop failed: %w", err)
	}
	
	// Calculate final metrics
	metrics := e.performanceTracker.CalculateMetrics()
	trades := e.collectTrades()
	
	result := &BacktestResult{
		Config:       e.config,
		Metrics:      metrics,
		Trades:       trades,
		EquityCurve:  e.performanceTracker.GetEquityCurve(),
		DrawdownCurve: e.performanceTracker.GetDrawdownCurve(),
		Duration:     time.Since(startTime),
	}
	
	// Calculate trade statistics
	e.calculateTradeStatistics(result)
	
	e.logger.Info("Backtest completed",
		zap.Float64("total_return", metrics.TotalReturn),
		zap.Float64("sharpe_ratio", metrics.SharpeRatio),
		zap.Float64("max_drawdown", metrics.MaxDrawdown),
		zap.Int("total_trades", len(trades)),
		zap.Duration("duration", result.Duration))
	
	return result, nil
}

// initialize prepares the engine for backtesting
func (e *BacktestEngine) initialize() error {
	// Validate configuration
	if e.config.EndTime.Before(e.config.StartTime) {
		return fmt.Errorf("end time before start time")
	}
	
	if e.config.InitialCapital <= 0 {
		return fmt.Errorf("invalid initial capital: %.2f", e.config.InitialCapital)
	}
	
	// Initialize data provider
	if err := e.dataProvider.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize data provider: %w", err)
	}
	
	e.currentTime = e.config.StartTime
	
	return nil
}

// loadData loads historical data into event queue
func (e *BacktestEngine) loadData() error {
	e.logger.Info("Loading historical data")
	
	// Load data for each symbol
	for _, symbol := range e.config.Symbols {
		// Load tick data
		ticks, err := e.dataProvider.LoadTicks(symbol, e.config.StartTime, e.config.EndTime)
		if err != nil {
			e.logger.Warn("Failed to load tick data",
				zap.String("symbol", symbol),
				zap.Error(err))
		} else {
			for _, tick := range ticks {
				e.eventQueue.Push(Event{
					Type:      EventTypeTick,
					Timestamp: tick.Timestamp,
					Data:      tick,
				})
			}
			e.logger.Info("Loaded tick data",
				zap.String("symbol", symbol),
				zap.Int("count", len(ticks)))
		}
		
		// Load order book data if available
		orderBooks, err := e.dataProvider.LoadOrderBooks(symbol, e.config.StartTime, e.config.EndTime)
		if err != nil {
			e.logger.Debug("No order book data available",
				zap.String("symbol", symbol))
		} else {
			for _, ob := range orderBooks {
				e.eventQueue.Push(Event{
					Type:      EventTypeOrderBook,
					Timestamp: ob.Timestamp,
					Data:      ob,
				})
			}
			e.logger.Info("Loaded order book data",
				zap.String("symbol", symbol),
				zap.Int("count", len(orderBooks)))
		}
		
		// Load bar data
		bars, err := e.dataProvider.LoadBars(symbol, e.config.StartTime, e.config.EndTime, e.config.BarInterval)
		if err != nil {
			e.logger.Warn("Failed to load bar data",
				zap.String("symbol", symbol),
				zap.Error(err))
		} else {
			for _, bar := range bars {
				e.eventQueue.Push(Event{
					Type:      EventTypeBar,
					Timestamp: bar.Timestamp,
					Data:      bar,
				})
			}
			e.logger.Info("Loaded bar data",
				zap.String("symbol", symbol),
				zap.Int("count", len(bars)))
		}
	}
	
	// Sort all events by timestamp
	e.eventQueue.Sort()
	
	totalEvents := e.eventQueue.Size()
	e.logger.Info("Data loading complete",
		zap.Int("total_events", totalEvents))
	
	if totalEvents == 0 {
		return fmt.Errorf("no data loaded for backtest period")
	}
	
	return nil
}

// runEventLoop processes all events
func (e *BacktestEngine) runEventLoop() error {
	lastUpdateTime := e.config.StartTime
	updateInterval := time.Hour * 24 // Daily equity updates
	
	for {
		// Check for cancellation
		select {
		case <-e.ctx.Done():
			return e.ctx.Err()
		default:
		}
		
		// Get next batch of events
		nextTime := e.currentTime.Add(e.config.TickSize)
		events := e.eventQueue.GetEventsUntil(nextTime)
		
		if len(events) == 0 && e.eventQueue.Size() == 0 {
			// No more events
			break
		}
		
		// Process events
		for _, event := range events {
			e.currentTime = event.Timestamp
			
			switch event.Type {
			case EventTypeTick:
				e.processTick(event.Data.(*TickData))
			case EventTypeBar:
				e.processBar(event.Data.(*BarData))
			case EventTypeOrderBook:
				e.processOrderBook(event.Data.(*OrderBookData))
			}
		}
		
		// Check pending orders
		e.checkPendingOrders()
		
		// Update equity periodically
		if e.currentTime.Sub(lastUpdateTime) >= updateInterval {
			e.updateEquity()
			lastUpdateTime = e.currentTime
		}
		
		// Advance time
		if len(events) == 0 {
			e.currentTime = nextTime
		}
	}
	
	// Final equity update
	e.updateEquity()
	
	// Close any remaining positions
	e.closeAllPositions()
	
	return nil
}

// processTick processes tick data
func (e *BacktestEngine) processTick(tick *TickData) {
	// Update order book simulation
	e.orderBook.UpdateFromTick(tick)
	
	// Update strategy
	marketData := &types.MarketData{
		Symbol:   tick.Symbol,
		Price:    tick.Price,
		Volume:   tick.Volume,
		BidPrice: tick.BidPrice,
		BidSize:  tick.BidSize,
		AskPrice: tick.AskPrice,
		AskSize:  tick.AskSize,
		Timestamp: tick.Timestamp,
	}
	
	// Get strategy signals
	signals := e.strategy.OnMarketData(marketData)
	
	// Process signals
	for _, signal := range signals {
		e.processSignal(signal)
	}
}

// processBar processes bar data
func (e *BacktestEngine) processBar(bar *BarData) {
	// Create market data from bar
	marketData := &types.MarketData{
		Symbol:    bar.Symbol,
		Price:     bar.Close,
		Volume:    bar.Volume,
		Open:      bar.Open,
		High:      bar.High,
		Low:       bar.Low,
		Close:     bar.Close,
		Timestamp: bar.Timestamp,
	}
	
	// Get strategy signals
	signals := e.strategy.OnMarketData(marketData)
	
	// Process signals
	for _, signal := range signals {
		e.processSignal(signal)
	}
}

// processOrderBook processes order book updates
func (e *BacktestEngine) processOrderBook(ob *OrderBookData) {
	// Update order book simulation
	e.orderBook.Update(ob)
}

// processSignal processes a trading signal
func (e *BacktestEngine) processSignal(signal *types.Signal) {
	if signal == nil || signal.Quantity <= 0 {
		return
	}
	
	// Create order from signal
	order := &types.Order{
		ID:        e.generateOrderID(),
		Symbol:    signal.Symbol,
		Side:      signal.Side,
		Type:      signal.OrderType,
		Quantity:  signal.Quantity,
		Price:     signal.Price,
		StopPrice: signal.StopPrice,
		TimeInForce: types.TimeInForceGTC,
		Timestamp: e.currentTime,
		Status:    types.OrderStatusNew,
	}
	
	// Validate order
	if err := e.validateOrder(order); err != nil {
		e.logger.Warn("Order validation failed",
			zap.String("order_id", order.ID),
			zap.Error(err))
		return
	}
	
	// Add to pending orders
	e.mu.Lock()
	e.pendingOrders[order.ID] = order
	e.mu.Unlock()
	
	e.logger.Debug("Order created",
		zap.String("order_id", order.ID),
		zap.String("symbol", order.Symbol),
		zap.String("side", string(order.Side)),
		zap.Float64("quantity", order.Quantity))
}

// checkPendingOrders checks and executes pending orders
func (e *BacktestEngine) checkPendingOrders() {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	for orderID, order := range e.pendingOrders {
		executed := false
		var executionPrice float64
		
		// Get current market price
		midPrice := e.orderBook.GetMidPrice(order.Symbol)
		if midPrice == 0 {
			continue // No market data available
		}
		
		switch order.Type {
		case types.OrderTypeMarket:
			// Market orders execute immediately
			executed = true
			if order.Side == types.OrderSideBuy {
				askPrice, _ := e.orderBook.GetBestAsk(order.Symbol)
				if askPrice > 0 {
					executionPrice = askPrice
				} else {
					executionPrice = midPrice * 1.001 // 0.1% penalty
				}
			} else {
				bidPrice, _ := e.orderBook.GetBestBid(order.Symbol)
				if bidPrice > 0 {
					executionPrice = bidPrice
				} else {
					executionPrice = midPrice * 0.999 // 0.1% penalty
				}
			}
			
		case types.OrderTypeLimit:
			// Check if limit price is reached
			if order.Side == types.OrderSideBuy {
				askPrice, _ := e.orderBook.GetBestAsk(order.Symbol)
				if askPrice > 0 && askPrice <= order.Price {
					executed = true
					executionPrice = order.Price
				}
			} else {
				bidPrice, _ := e.orderBook.GetBestBid(order.Symbol)
				if bidPrice > 0 && bidPrice >= order.Price {
					executed = true
					executionPrice = order.Price
				}
			}
			
		case types.OrderTypeStopLoss:
			// Check if stop price is triggered
			if order.Side == types.OrderSideSell && midPrice <= order.StopPrice {
				executed = true
				bidPrice, _ := e.orderBook.GetBestBid(order.Symbol)
				if bidPrice > 0 {
					executionPrice = bidPrice
				} else {
					executionPrice = midPrice * 0.995 // 0.5% penalty for stop loss
				}
			} else if order.Side == types.OrderSideBuy && midPrice >= order.StopPrice {
				executed = true
				askPrice, _ := e.orderBook.GetBestAsk(order.Symbol)
				if askPrice > 0 {
					executionPrice = askPrice
				} else {
					executionPrice = midPrice * 1.005 // 0.5% penalty for stop loss
				}
			}
		}
		
		if executed {
			// Calculate slippage
			slippage := e.slippageModel.CalculateSlippage(SlippageRequest{
				Symbol:        order.Symbol,
				Side:          order.Side,
				Quantity:      order.Quantity,
				BasePrice:     executionPrice,
				Volatility:    e.orderBook.GetVolatility(order.Symbol),
				Spread:        e.orderBook.GetSpread(order.Symbol),
				MarketDepth:   e.orderBook.GetMarketDepth(order.Symbol, order.Side),
				AverageVolume: e.orderBook.GetAverageVolume(order.Symbol),
			})
			
			// Apply slippage
			if order.Side == types.OrderSideBuy {
				executionPrice *= (1 + slippage)
			} else {
				executionPrice *= (1 - slippage)
			}
			
			// Execute order
			e.executeOrder(order, executionPrice, slippage)
			
			// Remove from pending
			delete(e.pendingOrders, orderID)
		}
	}
}

// executeOrder executes an order
func (e *BacktestEngine) executeOrder(order *types.Order, price float64, slippage float64) {
	// Calculate commission
	commission := e.calculateCommission(order.Symbol, order.Quantity, price)
	
	// Create trade
	trade := &Trade{
		ID:         order.ID,
		Symbol:     order.Symbol,
		Side:       order.Side,
		Quantity:   order.Quantity,
		Price:      price,
		Commission: commission,
		Slippage:   slippage * price * order.Quantity,
		Timestamp:  e.currentTime,
	}
	
	// Update position and get PnL
	pnl := e.positionManager.UpdatePosition(trade)
	trade.PnL = pnl
	
	// Record trade
	e.performanceTracker.RecordTrade(trade)
	
	// Update order status
	order.Status = types.OrderStatusFilled
	order.FilledQuantity = order.Quantity
	order.AveragePrice = price
	e.executedOrders[order.ID] = order
	
	// Notify strategy
	fill := &types.Fill{
		OrderID:    order.ID,
		Symbol:     order.Symbol,
		Side:       order.Side,
		Quantity:   order.Quantity,
		Price:      price,
		Commission: commission,
		Timestamp:  e.currentTime,
	}
	e.strategy.OnFill(fill)
	
	e.logger.Debug("Order executed",
		zap.String("order_id", order.ID),
		zap.String("symbol", order.Symbol),
		zap.Float64("price", price),
		zap.Float64("slippage", slippage),
		zap.Float64("pnl", pnl))
}

// validateOrder validates an order
func (e *BacktestEngine) validateOrder(order *types.Order) error {
	// Check available capital
	availableCapital := e.positionManager.GetAvailableCapital()
	requiredCapital := order.Quantity * order.Price
	
	if order.Side == types.OrderSideBuy && requiredCapital > availableCapital {
		return fmt.Errorf("insufficient capital: required %.2f, available %.2f", 
			requiredCapital, availableCapital)
	}
	
	// Check position limits
	position := e.positionManager.GetPosition(order.Symbol)
	if e.config.MaxPositionSize > 0 {
		newPosition := position.Quantity
		if order.Side == types.OrderSideBuy {
			newPosition += order.Quantity
		} else {
			newPosition -= order.Quantity
		}
		
		if math.Abs(newPosition) > e.config.MaxPositionSize {
			return fmt.Errorf("position size limit exceeded: %.2f > %.2f",
				math.Abs(newPosition), e.config.MaxPositionSize)
		}
	}
	
	return nil
}

// calculateCommission calculates trading commission
func (e *BacktestEngine) calculateCommission(symbol string, quantity, price float64) float64 {
	notional := quantity * price
	
	// Apply commission model
	commission := 0.0
	
	if e.config.CommissionRate > 0 {
		commission = notional * e.config.CommissionRate
	}
	
	if e.config.CommissionMin > 0 && commission < e.config.CommissionMin {
		commission = e.config.CommissionMin
	}
	
	return commission
}

// updateEquity updates the equity curve
func (e *BacktestEngine) updateEquity() {
	// Get current prices
	prices := make(map[string]float64)
	for _, symbol := range e.config.Symbols {
		price := e.orderBook.GetMidPrice(symbol)
		if price > 0 {
			prices[symbol] = price
		}
	}
	
	// Calculate total equity
	equity := e.positionManager.GetTotalEquity(prices)
	
	// Update performance tracker
	e.performanceTracker.UpdateEquity(e.currentTime, equity)
}

// closeAllPositions closes all open positions at market
func (e *BacktestEngine) closeAllPositions() {
	positions := e.positionManager.GetPositions()
	
	for symbol, position := range positions {
		if position.Quantity == 0 {
			continue
		}
		
		// Create market order to close position
		side := types.OrderSideSell
		if position.Quantity < 0 {
			side = types.OrderSideBuy
		}
		
		order := &types.Order{
			ID:        e.generateOrderID(),
			Symbol:    symbol,
			Side:      side,
			Type:      types.OrderTypeMarket,
			Quantity:  math.Abs(position.Quantity),
			Timestamp: e.currentTime,
			Status:    types.OrderStatusNew,
		}
		
		// Execute immediately at mid price
		midPrice := e.orderBook.GetMidPrice(symbol)
		if midPrice > 0 {
			e.executeOrder(order, midPrice, 0)
		}
	}
}

// collectTrades collects all executed trades
func (e *BacktestEngine) collectTrades() []*Trade {
	trades := make([]*Trade, 0, len(e.performanceTracker.trades))
	trades = append(trades, e.performanceTracker.trades...)
	
	// Sort by timestamp
	sort.Slice(trades, func(i, j int) bool {
		return trades[i].Timestamp.Before(trades[j].Timestamp)
	})
	
	return trades
}

// calculateTradeStatistics calculates detailed trade statistics
func (e *BacktestEngine) calculateTradeStatistics(result *BacktestResult) {
	if len(result.Trades) == 0 {
		return
	}
	
	stats := &result.Statistics
	stats.TotalTrades = len(result.Trades)
	
	var (
		profits      []float64
		losses       []float64
		winStreak    int
		lossStreak   int
		maxWinStreak int
		maxLossStreak int
		totalHoldTime time.Duration
		profitTrades  int
	)
	
	for i, trade := range result.Trades {
		if trade.PnL > 0 {
			profits = append(profits, trade.PnL)
			profitTrades++
			winStreak++
			lossStreak = 0
			if winStreak > maxWinStreak {
				maxWinStreak = winStreak
			}
		} else if trade.PnL < 0 {
			losses = append(losses, math.Abs(trade.PnL))
			winStreak = 0
			lossStreak++
			if lossStreak > maxLossStreak {
				maxLossStreak = lossStreak
			}
		}
		
		// Calculate hold time (time to next opposite trade)
		if i < len(result.Trades)-1 {
			for j := i + 1; j < len(result.Trades); j++ {
				if result.Trades[j].Symbol == trade.Symbol &&
					result.Trades[j].Side != trade.Side {
					totalHoldTime += result.Trades[j].Timestamp.Sub(trade.Timestamp)
					break
				}
			}
		}
	}
	
	stats.WinningTrades = profitTrades
	stats.LosingTrades = len(result.Trades) - profitTrades
	stats.MaxWinStreak = maxWinStreak
	stats.MaxLossStreak = maxLossStreak
	
	// Calculate averages
	if len(profits) > 0 {
		sum := 0.0
		max := 0.0
		for _, p := range profits {
			sum += p
			if p > max {
				max = p
			}
		}
		stats.AvgProfit = sum / float64(len(profits))
		stats.MaxProfit = max
	}
	
	if len(losses) > 0 {
		sum := 0.0
		max := 0.0
		for _, l := range losses {
			sum += l
			if l > max {
				max = l
			}
		}
		stats.AvgLoss = sum / float64(len(losses))
		stats.MaxLoss = max
	}
	
	if stats.TotalTrades > 0 {
		stats.AvgHoldTime = totalHoldTime / time.Duration(stats.TotalTrades)
	}
	
	// Kelly Criterion
	if stats.AvgLoss > 0 && len(profits) > 0 {
		winRate := float64(len(profits)) / float64(stats.TotalTrades)
		avgWinLossRatio := stats.AvgProfit / stats.AvgLoss
		stats.KellyPercent = winRate - (1-winRate)/avgWinLossRatio
	}
}

// generateOrderID generates a unique order ID
func (e *BacktestEngine) generateOrderID() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	e.orderCounter++
	return fmt.Sprintf("BT-%d-%d", e.currentTime.Unix(), e.orderCounter)
}

// Stop stops the backtest engine
func (e *BacktestEngine) Stop() {
	e.cancel()
}

// GetProgress returns backtest progress
func (e *BacktestEngine) GetProgress() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	totalDuration := e.config.EndTime.Sub(e.config.StartTime)
	elapsed := e.currentTime.Sub(e.config.StartTime)
	
	if totalDuration > 0 {
		return float64(elapsed) / float64(totalDuration)
	}
	
	return 0
}

// GetCurrentTime returns the current simulation time
func (e *BacktestEngine) GetCurrentTime() time.Time {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	return e.currentTime
}