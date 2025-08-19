package backtest

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/mExOms/internal/position"
	"github.com/mExOms/internal/risk"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// BacktestEngine runs backtests with historical data
type BacktestEngine struct {
	mu sync.RWMutex
	
	// Components
	eventStore      *EventStore
	riskEngine      *risk.RiskEngine
	positionManager *position.PositionManager
	
	// Configuration
	config BacktestConfig
	
	// State
	currentTime     time.Time
	portfolio       *Portfolio
	orderHistory    []*OrderRecord
	executedTrades  []*TradeRecord
	
	// Metrics
	metrics *BacktestMetrics
}

// BacktestConfig contains backtest configuration
type BacktestConfig struct {
	StartTime        time.Time
	EndTime          time.Time
	InitialCapital   decimal.Decimal
	TradingFees      decimal.Decimal // percentage
	SlippageModel    SlippageModel
	ExecutionLatency time.Duration
	DataFrequency    time.Duration
}

// SlippageModel defines how to calculate slippage
type SlippageModel interface {
	CalculateSlippage(order *types.Order, marketDepth map[string]interface{}) decimal.Decimal
}

// Portfolio tracks account state
type Portfolio struct {
	Cash         decimal.Decimal
	Positions    map[string]*PortfolioPosition
	TotalValue   decimal.Decimal
	UnrealizedPL decimal.Decimal
	RealizedPL   decimal.Decimal
	UpdatedAt    time.Time
}

// PortfolioPosition represents a position in the portfolio
type PortfolioPosition struct {
	Symbol       string
	Quantity     decimal.Decimal
	AvgCost      decimal.Decimal
	CurrentPrice decimal.Decimal
	UnrealizedPL decimal.Decimal
	RealizedPL   decimal.Decimal
}

// OrderRecord stores order history
type OrderRecord struct {
	Order         *types.Order
	SubmittedAt   time.Time
	ExecutedAt    time.Time
	ExecutedPrice decimal.Decimal
	ExecutedQty   decimal.Decimal
	Status        types.OrderStatus
	Slippage      decimal.Decimal
	Commission    decimal.Decimal
}

// TradeRecord stores executed trades
type TradeRecord struct {
	OrderID      string
	Symbol       string
	Side         types.OrderSide
	Price        decimal.Decimal
	Quantity     decimal.Decimal
	Commission   decimal.Decimal
	Timestamp    time.Time
	PortfolioPL  decimal.Decimal
}

// BacktestMetrics tracks performance metrics
type BacktestMetrics struct {
	TotalTrades      int
	WinningTrades    int
	LosingTrades     int
	TotalReturn      decimal.Decimal
	MaxDrawdown      decimal.Decimal
	SharpeRatio      float64
	WinRate          float64
	AvgWin           decimal.Decimal
	AvgLoss          decimal.Decimal
	ProfitFactor     decimal.Decimal
	MaxConsecutiveLosses int
	
	// Time series data
	EquityCurve      []EquityPoint
	DrawdownCurve    []DrawdownPoint
	DailyReturns     []DailyReturn
}

// EquityPoint represents portfolio value at a point in time
type EquityPoint struct {
	Time  time.Time
	Value decimal.Decimal
}

// DrawdownPoint represents drawdown at a point in time
type DrawdownPoint struct {
	Time     time.Time
	Drawdown decimal.Decimal
}

// DailyReturn represents daily return
type DailyReturn struct {
	Date   time.Time
	Return decimal.Decimal
}

// NewBacktestEngine creates a new backtest engine
func NewBacktestEngine(eventStore *EventStore, config BacktestConfig) (*BacktestEngine, error) {
	// Create position manager for backtest
	posManager, err := position.NewPositionManager("./backtest_data/positions")
	if err != nil {
		return nil, fmt.Errorf("failed to create position manager: %w", err)
	}
	
	// Create risk engine
	riskEngine := risk.NewRiskEngine()
	
	return &BacktestEngine{
		eventStore:      eventStore,
		riskEngine:      riskEngine,
		positionManager: posManager,
		config:          config,
		currentTime:     config.StartTime,
		portfolio: &Portfolio{
			Cash:      config.InitialCapital,
			Positions: make(map[string]*PortfolioPosition),
		},
		orderHistory:   make([]*OrderRecord, 0),
		executedTrades: make([]*TradeRecord, 0),
		metrics:        &BacktestMetrics{},
	}, nil
}

// RunStrategy runs a trading strategy through the backtest
func (be *BacktestEngine) RunStrategy(ctx context.Context, strategy TradingStrategy) error {
	// Initialize strategy
	if err := strategy.Initialize(be.config); err != nil {
		return fmt.Errorf("failed to initialize strategy: %w", err)
	}
	
	// Create time ticker for simulation
	ticker := time.NewTicker(be.config.DataFrequency)
	defer ticker.Stop()
	
	// Track equity for metrics
	equityTracker := make([]EquityPoint, 0)
	peakEquity := be.config.InitialCapital
	
	// Main backtest loop
	for be.currentTime.Before(be.config.EndTime) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		
		// Get market events for current time window
		events, err := be.getEventsForTimeWindow()
		if err != nil {
			return fmt.Errorf("failed to get events: %w", err)
		}
		
		// Update market state
		marketState := be.processMarketEvents(events)
		
		// Update portfolio prices
		be.updatePortfolio(marketState)
		
		// Generate signals
		signals := strategy.GenerateSignals(be.currentTime, marketState, be.portfolio)
		
		// Process signals into orders
		for _, signal := range signals {
			order := be.createOrderFromSignal(signal)
			
			// Risk check
			if err := be.validateOrder(order); err != nil {
				continue // Skip invalid orders
			}
			
			// Simulate order execution
			be.executeOrder(order, marketState)
		}
		
		// Update metrics
		currentEquity := be.calculatePortfolioValue()
		equityTracker = append(equityTracker, EquityPoint{
			Time:  be.currentTime,
			Value: currentEquity,
		})
		
		// Track drawdown
		if currentEquity.GreaterThan(peakEquity) {
			peakEquity = currentEquity
		}
		drawdown := peakEquity.Sub(currentEquity).Div(peakEquity)
		if drawdown.GreaterThan(be.metrics.MaxDrawdown) {
			be.metrics.MaxDrawdown = drawdown
		}
		
		// Advance time
		be.currentTime = be.currentTime.Add(be.config.DataFrequency)
	}
	
	// Finalize strategy
	strategy.Finalize()
	
	// Calculate final metrics
	be.calculateFinalMetrics(equityTracker)
	
	return nil
}

// getEventsForTimeWindow retrieves events for the current time window
func (be *BacktestEngine) getEventsForTimeWindow() ([]*MarketEvent, error) {
	endTime := be.currentTime.Add(be.config.DataFrequency)
	
	// Get events from all exchanges/symbols the strategy is interested in
	// For demo, we'll use a predefined list
	symbols := []string{"BTCUSDT", "ETHUSDT", "BNBUSDT"}
	exchanges := []string{"binance"}
	
	var allEvents []*MarketEvent
	
	for _, exchange := range exchanges {
		for _, symbol := range symbols {
			events, err := be.eventStore.GetEvents(exchange, symbol, be.currentTime, endTime)
			if err != nil {
				return nil, err
			}
			allEvents = append(allEvents, events...)
		}
	}
	
	// Sort by timestamp
	sort.Slice(allEvents, func(i, j int) bool {
		return allEvents[i].Timestamp.Before(allEvents[j].Timestamp)
	})
	
	return allEvents, nil
}

// processMarketEvents processes market events into market state
func (be *BacktestEngine) processMarketEvents(events []*MarketEvent) MarketState {
	state := NewMarketState()
	
	for _, event := range events {
		switch event.Type {
		case EventTypeOrderBook:
			state.UpdateOrderBook(event.Exchange, event.Symbol, event.Data)
		case EventTypeTrade:
			state.UpdateLastTrade(event.Exchange, event.Symbol, event.Data)
		case EventTypeTicker:
			state.UpdateTicker(event.Exchange, event.Symbol, event.Data)
		}
	}
	
	return state
}

// updateAverage updates running average
func (be *BacktestEngine) updateAverage(currentAvg, newValue decimal.Decimal, count int) decimal.Decimal {
	if count <= 1 {
		return newValue
	}
	
	// Running average: new_avg = old_avg + (new_value - old_avg) / count
	return currentAvg.Add(newValue.Sub(currentAvg).Div(decimal.NewFromInt(int64(count))))
}

// updatePortfolio updates portfolio with current market prices
func (be *BacktestEngine) updatePortfolio(marketState MarketState) {
	be.mu.Lock()
	defer be.mu.Unlock()
	
	totalValue := be.portfolio.Cash
	totalUnrealizedPL := decimal.Zero
	
	for symbol, pos := range be.portfolio.Positions {
		// Get current price
		currentPrice := marketState.GetPrice("binance", symbol)
		if currentPrice.IsZero() {
			currentPrice = pos.CurrentPrice // Use last known price
		}
		
		pos.CurrentPrice = currentPrice
		
		// Calculate unrealized P&L
		positionValue := pos.Quantity.Mul(currentPrice)
		costBasis := pos.Quantity.Mul(pos.AvgCost)
		pos.UnrealizedPL = positionValue.Sub(costBasis)
		
		totalValue = totalValue.Add(positionValue)
		totalUnrealizedPL = totalUnrealizedPL.Add(pos.UnrealizedPL)
	}
	
	be.portfolio.TotalValue = totalValue
	be.portfolio.UnrealizedPL = totalUnrealizedPL
	be.portfolio.UpdatedAt = be.currentTime
}

// createOrderFromSignal creates an order from a trading signal
func (be *BacktestEngine) createOrderFromSignal(signal *TradingSignal) *types.Order {
	return &types.Order{
		ClientOrderID: fmt.Sprintf("backtest_%d", len(be.orderHistory)),
		Symbol:        signal.Symbol,
		Side:          signal.Side,
		Type:          signal.OrderType,
		Price:         signal.Price,
		Quantity:      signal.Quantity,
		TimeInForce:   types.TimeInForceGTC,
		CreatedAt:     be.currentTime,
	}
}

// validateOrder validates an order against risk rules
func (be *BacktestEngine) validateOrder(order *types.Order) error {
	// Check available cash for buy orders
	if order.Side == types.OrderSideBuy {
		requiredCash := order.Price.Mul(order.Quantity)
		requiredCash = requiredCash.Add(requiredCash.Mul(be.config.TradingFees))
		
		if requiredCash.GreaterThan(be.portfolio.Cash) {
			return fmt.Errorf("insufficient cash: required %s, available %s", 
				requiredCash.String(), be.portfolio.Cash.String())
		}
	}
	
	// Check position for sell orders
	if order.Side == types.OrderSideSell {
		pos, exists := be.portfolio.Positions[order.Symbol]
		if !exists || pos.Quantity.LessThan(order.Quantity) {
			return fmt.Errorf("insufficient position: required %s, available %s",
				order.Quantity.String(), pos.Quantity.String())
		}
	}
	
	return nil
}

// executeOrder simulates order execution
func (be *BacktestEngine) executeOrder(order *types.Order, marketState MarketState) {
	be.mu.Lock()
	defer be.mu.Unlock()
	
	// Simulate execution latency
	executionTime := be.currentTime.Add(be.config.ExecutionLatency)
	
	// Get execution price (with slippage)
	marketPrice := marketState.GetPrice("binance", order.Symbol)
	slippage := be.calculateSlippage(order, marketState)
	
	var executionPrice decimal.Decimal
	if order.Side == types.OrderSideBuy {
		executionPrice = marketPrice.Add(marketPrice.Mul(slippage))
	} else {
		executionPrice = marketPrice.Sub(marketPrice.Mul(slippage))
	}
	
	// Calculate commission
	tradeValue := executionPrice.Mul(order.Quantity)
	commission := tradeValue.Mul(be.config.TradingFees)
	
	// Update portfolio
	if order.Side == types.OrderSideBuy {
		// Deduct cash
		totalCost := tradeValue.Add(commission)
		be.portfolio.Cash = be.portfolio.Cash.Sub(totalCost)
		
		// Add/update position
		if pos, exists := be.portfolio.Positions[order.Symbol]; exists {
			// Update average cost
			totalQuantity := pos.Quantity.Add(order.Quantity)
			totalCost := pos.Quantity.Mul(pos.AvgCost).Add(tradeValue)
			pos.AvgCost = totalCost.Div(totalQuantity)
			pos.Quantity = totalQuantity
		} else {
			// Create new position
			be.portfolio.Positions[order.Symbol] = &PortfolioPosition{
				Symbol:       order.Symbol,
				Quantity:     order.Quantity,
				AvgCost:      executionPrice,
				CurrentPrice: executionPrice,
			}
		}
	} else {
		// Sell order
		pos := be.portfolio.Positions[order.Symbol]
		
		// Calculate realized P&L
		costBasis := order.Quantity.Mul(pos.AvgCost)
		proceeds := tradeValue.Sub(commission)
		realizedPL := proceeds.Sub(costBasis)
		
		// Update portfolio
		be.portfolio.Cash = be.portfolio.Cash.Add(proceeds)
		be.portfolio.RealizedPL = be.portfolio.RealizedPL.Add(realizedPL)
		pos.RealizedPL = pos.RealizedPL.Add(realizedPL)
		
		// Update position quantity
		pos.Quantity = pos.Quantity.Sub(order.Quantity)
		if pos.Quantity.IsZero() {
			delete(be.portfolio.Positions, order.Symbol)
		}
	}
	
	// Record order execution
	orderRecord := &OrderRecord{
		Order:         order,
		SubmittedAt:   be.currentTime,
		ExecutedAt:    executionTime,
		ExecutedPrice: executionPrice,
		ExecutedQty:   order.Quantity,
		Status:        types.OrderStatusFilled,
		Slippage:      slippage,
		Commission:    commission,
	}
	be.orderHistory = append(be.orderHistory, orderRecord)
	
	// Record trade
	trade := &TradeRecord{
		OrderID:     order.ClientOrderID,
		Symbol:      order.Symbol,
		Side:        order.Side,
		Price:       executionPrice,
		Quantity:    order.Quantity,
		Commission:  commission,
		Timestamp:   executionTime,
		PortfolioPL: be.portfolio.RealizedPL,
	}
	be.executedTrades = append(be.executedTrades, trade)
	
	// Update metrics
	be.metrics.TotalTrades++
	if order.Side == types.OrderSideSell {
		// Calculate P&L for the trade
		var pl decimal.Decimal
		if pos, exists := be.portfolio.Positions[order.Symbol]; exists {
			pl = trade.Price.Sub(pos.AvgCost).Mul(trade.Quantity)
		} else {
			// Use realized P&L from the trade record
			pl = realizedPL
		}
		
		if pl.IsPositive() {
			be.metrics.WinningTrades++
			be.metrics.AvgWin = be.updateAverage(be.metrics.AvgWin, pl, be.metrics.WinningTrades)
		} else {
			be.metrics.LosingTrades++
			be.metrics.AvgLoss = be.updateAverage(be.metrics.AvgLoss, pl.Abs(), be.metrics.LosingTrades)
		}
	}
}

// calculateSlippage calculates execution slippage
func (be *BacktestEngine) calculateSlippage(order *types.Order, marketState MarketState) decimal.Decimal {
	if be.config.SlippageModel != nil {
		orderbook := marketState.GetOrderBook("binance", order.Symbol)
		return be.config.SlippageModel.CalculateSlippage(order, orderbook)
	}
	
	// Default: 0.05% slippage
	return decimal.NewFromFloat(0.0005)
}

// calculatePortfolioValue calculates total portfolio value
func (be *BacktestEngine) calculatePortfolioValue() decimal.Decimal {
	be.mu.RLock()
	defer be.mu.RUnlock()
	
	return be.portfolio.TotalValue
}

// calculateFinalMetrics calculates final backtest metrics
func (be *BacktestEngine) calculateFinalMetrics(equityCurve []EquityPoint) {
	if len(equityCurve) == 0 {
		return
	}
	
	// Total return
	initialValue := be.config.InitialCapital
	finalValue := equityCurve[len(equityCurve)-1].Value
	be.metrics.TotalReturn = finalValue.Sub(initialValue).Div(initialValue)
	
	// Win rate
	if be.metrics.TotalTrades > 0 {
		be.metrics.WinRate = float64(be.metrics.WinningTrades) / float64(be.metrics.TotalTrades)
	}
	
	// Calculate daily returns
	be.calculateDailyReturns(equityCurve)
	
	// Sharpe ratio (simplified - assuming 0% risk-free rate)
	if len(be.metrics.DailyReturns) > 1 {
		avgReturn := decimal.Zero
		for _, dr := range be.metrics.DailyReturns {
			avgReturn = avgReturn.Add(dr.Return)
		}
		avgReturn = avgReturn.Div(decimal.NewFromInt(int64(len(be.metrics.DailyReturns))))
		
		// Calculate standard deviation
		variance := decimal.Zero
		for _, dr := range be.metrics.DailyReturns {
			diff := dr.Return.Sub(avgReturn)
			variance = variance.Add(diff.Mul(diff))
		}
		variance = variance.Div(decimal.NewFromInt(int64(len(be.metrics.DailyReturns))))
		
		if !variance.IsZero() {
			// Annualized Sharpe ratio
			sharpe := avgReturn.Div(variance).Mul(decimal.NewFromFloat(252)) // 252 trading days
			be.metrics.SharpeRatio, _ = sharpe.Float64()
		}
	}
	
	be.metrics.EquityCurve = equityCurve
}

// calculateDailyReturns calculates daily returns from equity curve
func (be *BacktestEngine) calculateDailyReturns(equityCurve []EquityPoint) {
	dailyEquity := make(map[string]decimal.Decimal)
	
	// Group by day
	for _, point := range equityCurve {
		day := point.Time.Format("2006-01-02")
		dailyEquity[day] = point.Value
	}
	
	// Convert to sorted slice
	var days []string
	for day := range dailyEquity {
		days = append(days, day)
	}
	sort.Strings(days)
	
	// Calculate returns
	for i := 1; i < len(days); i++ {
		prevValue := dailyEquity[days[i-1]]
		currValue := dailyEquity[days[i]]
		
		if !prevValue.IsZero() {
			dailyReturn := currValue.Sub(prevValue).Div(prevValue)
			date, _ := time.Parse("2006-01-02", days[i])
			
			be.metrics.DailyReturns = append(be.metrics.DailyReturns, DailyReturn{
				Date:   date,
				Return: dailyReturn,
			})
		}
	}
}

// GetResults returns backtest results
func (be *BacktestEngine) GetResults() *BacktestResults {
	return &BacktestResults{
		Config:         be.config,
		Portfolio:      be.portfolio,
		OrderHistory:   be.orderHistory,
		ExecutedTrades: be.executedTrades,
		Metrics:        be.metrics,
	}
}

// BacktestResults contains complete backtest results
type BacktestResults struct {
	Config         BacktestConfig
	Portfolio      *Portfolio
	OrderHistory   []*OrderRecord
	ExecutedTrades []*TradeRecord
	Metrics        *BacktestMetrics
}