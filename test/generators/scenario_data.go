package generators

import (
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// ScenarioType represents different market scenarios
type ScenarioType string

const (
	ScenarioBullRun      ScenarioType = "bull_run"
	ScenarioBearMarket   ScenarioType = "bear_market"
	ScenarioHighVolatile ScenarioType = "high_volatile"
	ScenarioFlashCrash   ScenarioType = "flash_crash"
	ScenarioSideways     ScenarioType = "sideways"
	ScenarioNormal       ScenarioType = "normal"
)

// ScenarioGenerator generates market scenarios for testing
type ScenarioGenerator struct {
	marketGen *MarketDataGenerator
	orderGen  *OrderGenerator
	scenario  ScenarioType
}

// NewScenarioGenerator creates a new scenario generator
func NewScenarioGenerator(seed int64) *ScenarioGenerator {
	return &ScenarioGenerator{
		marketGen: NewMarketDataGenerator(seed),
		orderGen:  NewOrderGenerator(seed),
		scenario:  ScenarioNormal,
	}
}

// SetScenario sets the current market scenario
func (g *ScenarioGenerator) SetScenario(scenario ScenarioType) {
	g.scenario = scenario
	
	// Adjust market generator parameters based on scenario
	switch scenario {
	case ScenarioBullRun:
		g.marketGen.SetVolatility(0.03) // Higher volatility, upward bias
	case ScenarioBearMarket:
		g.marketGen.SetVolatility(0.04) // Higher volatility, downward bias
	case ScenarioHighVolatile:
		g.marketGen.SetVolatility(0.08) // Very high volatility
	case ScenarioFlashCrash:
		g.marketGen.SetVolatility(0.15) // Extreme volatility
	case ScenarioSideways:
		g.marketGen.SetVolatility(0.01) // Low volatility
	default:
		g.marketGen.SetVolatility(0.02) // Normal volatility
	}
}

// GenerateMarketMovement generates a series of market updates for a scenario
func (g *ScenarioGenerator) GenerateMarketMovement(symbol string, duration time.Duration, updateInterval time.Duration) []MarketUpdate {
	updates := []MarketUpdate{}
	
	steps := int(duration / updateInterval)
	currentPrice := g.marketGen.basePrice
	
	for i := 0; i < steps; i++ {
		timestamp := time.Now().Add(updateInterval * time.Duration(i))
		
		// Apply scenario-specific price movement
		priceChange := g.calculatePriceChange(i, steps)
		currentPrice *= (1 + priceChange)
		
		// Ensure price stays positive
		if currentPrice <= 0 {
			currentPrice = g.marketGen.basePrice * 0.1
		}
		
		g.marketGen.SetBasePrice(currentPrice)
		
		// Generate market data at this price level
		ticker := g.marketGen.GenerateTicker(symbol)
		orderBook := g.marketGen.GenerateOrderBook(symbol, 20)
		trades := g.generateTrades(symbol, 5)
		
		update := MarketUpdate{
			Timestamp: timestamp,
			Ticker:    ticker,
			OrderBook: orderBook,
			Trades:    trades,
		}
		
		updates = append(updates, update)
	}
	
	return updates
}

// GenerateTradingSession generates a complete trading session
func (g *ScenarioGenerator) GenerateTradingSession(symbol string, duration time.Duration) *TradingSession {
	session := &TradingSession{
		StartTime: time.Now(),
		EndTime:   time.Now().Add(duration),
		Symbol:    symbol,
		Scenario:  g.scenario,
		Events:    []TradingEvent{},
	}
	
	// Generate market updates every second
	marketUpdates := g.GenerateMarketMovement(symbol, duration, time.Second)
	
	// Convert to trading events
	for _, update := range marketUpdates {
		// Market update event
		session.Events = append(session.Events, TradingEvent{
			Type:      EventTypeMarketUpdate,
			Timestamp: update.Timestamp,
			Data:      update,
		})
		
		// Occasionally generate orders (10% chance)
		if g.marketGen.rand.Float64() < 0.1 {
			order := g.generateScenarioOrder(symbol, update.Ticker.Price.InexactFloat64())
			session.Events = append(session.Events, TradingEvent{
				Type:      EventTypeOrderPlaced,
				Timestamp: update.Timestamp,
				Data:      order,
			})
		}
	}
	
	return session
}

// GenerateStressTestData generates data for stress testing
func (g *ScenarioGenerator) GenerateStressTestData() *StressTestData {
	return &StressTestData{
		HighFrequencyOrders:  g.generateHighFrequencyOrders(1000),
		LargeOrderBooks:      g.generateLargeOrderBooks(10),
		RapidPriceMovements:  g.generateRapidPriceMovements(100),
		ConcurrentPositions:  g.generateConcurrentPositions(50),
	}
}

// Helper methods

func (g *ScenarioGenerator) calculatePriceChange(step, totalSteps int) float64 {
	progress := float64(step) / float64(totalSteps)
	
	switch g.scenario {
	case ScenarioBullRun:
		// Steady upward trend with minor corrections
		trend := 0.001 // 0.1% per step
		noise := (g.marketGen.rand.Float64() - 0.3) * 0.002
		return trend + noise
		
	case ScenarioBearMarket:
		// Steady downward trend with minor rallies
		trend := -0.001 // -0.1% per step
		noise := (g.marketGen.rand.Float64() - 0.7) * 0.002
		return trend + noise
		
	case ScenarioHighVolatile:
		// Large random movements
		return (g.marketGen.rand.Float64() - 0.5) * 0.01 // ±0.5%
		
	case ScenarioFlashCrash:
		// Sudden drop followed by partial recovery
		if progress < 0.2 {
			return (g.marketGen.rand.Float64() - 0.5) * 0.002
		} else if progress < 0.3 {
			return -0.05 // -5% crash
		} else {
			return 0.01 * (g.marketGen.rand.Float64()) // Gradual recovery
		}
		
	case ScenarioSideways:
		// Small movements around the mean
		return (g.marketGen.rand.Float64() - 0.5) * 0.001 // ±0.05%
		
	default:
		// Normal market movement
		return g.marketGen.rand.NormFloat64() * 0.002
	}
}

func (g *ScenarioGenerator) generateTrades(symbol string, count int) []*types.Trade {
	trades := make([]*types.Trade, count)
	for i := 0; i < count; i++ {
		trades[i] = g.marketGen.GenerateTrade(symbol)
	}
	return trades
}

func (g *ScenarioGenerator) generateScenarioOrder(symbol string, currentPrice float64) *types.Order {
	// Order type depends on scenario
	switch g.scenario {
	case ScenarioBullRun:
		// More buy orders
		if g.marketGen.rand.Float64() < 0.7 {
			return g.orderGen.GenerateLimitOrder(symbol, types.OrderSideBuy, 0.001)
		}
		return g.orderGen.GenerateMarketOrder(symbol, types.OrderSideBuy)
		
	case ScenarioBearMarket:
		// More sell orders
		if g.marketGen.rand.Float64() < 0.7 {
			return g.orderGen.GenerateLimitOrder(symbol, types.OrderSideSell, 0.001)
		}
		return g.orderGen.GenerateMarketOrder(symbol, types.OrderSideSell)
		
	case ScenarioFlashCrash:
		// Panic selling
		return g.orderGen.GenerateMarketOrder(symbol, types.OrderSideSell)
		
	default:
		// Balanced orders
		side := types.OrderSideBuy
		if g.marketGen.rand.Float64() > 0.5 {
			side = types.OrderSideSell
		}
		return g.orderGen.GenerateLimitOrder(symbol, side, 0.002)
	}
}

func (g *ScenarioGenerator) generateHighFrequencyOrders(count int) []*types.Order {
	orders := make([]*types.Order, count)
	
	for i := 0; i < count; i++ {
		// Very small quantities, tight spreads
		side := types.OrderSideBuy
		if g.marketGen.rand.Float64() > 0.5 {
			side = types.OrderSideSell
		}
		
		order := g.orderGen.GenerateLimitOrder("BTCUSDT", side, 0.0001) // 0.01% spread
		order.Quantity = decimal.NewFromFloat(0.001) // Small size
		orders[i] = order
	}
	
	return orders
}

func (g *ScenarioGenerator) generateLargeOrderBooks(count int) []*types.OrderBook {
	books := make([]*types.OrderBook, count)
	
	for i := 0; i < count; i++ {
		// Generate order books with 1000 levels
		books[i] = g.marketGen.GenerateOrderBook("BTCUSDT", 1000)
	}
	
	return books
}

func (g *ScenarioGenerator) generateRapidPriceMovements(count int) []*types.Ticker {
	tickers := make([]*types.Ticker, count)
	
	// Start from base price
	currentPrice := g.marketGen.basePrice
	
	for i := 0; i < count; i++ {
		// Rapid ±1% movements
		change := (g.marketGen.rand.Float64() - 0.5) * 0.02
		currentPrice *= (1 + change)
		
		g.marketGen.SetBasePrice(currentPrice)
		tickers[i] = g.marketGen.GenerateTicker("BTCUSDT")
	}
	
	return tickers
}

func (g *ScenarioGenerator) generateConcurrentPositions(count int) []*types.Position {
	positions := make([]*types.Position, count)
	
	symbols := []string{"BTCUSDT", "ETHUSDT", "BNBUSDT", "ADAUSDT", "SOLUSDT"}
	
	for i := 0; i < count; i++ {
		symbol := symbols[i%len(symbols)]
		
		side := types.Side("LONG")
		if g.marketGen.rand.Float64() > 0.5 {
			side = types.Side("SHORT")
		}
		
		positions[i] = &types.Position{
			Symbol:        symbol,
			Side:          side,
			Amount:        decimal.NewFromFloat(0.1 + g.marketGen.rand.Float64()*0.9),
			EntryPrice:    decimal.NewFromFloat(30000 + g.marketGen.rand.Float64()*20000),
			MarkPrice:     decimal.NewFromFloat(30000 + g.marketGen.rand.Float64()*20000),
			UnrealizedPnL: decimal.NewFromFloat(-100 + g.marketGen.rand.Float64()*200),
			Leverage:      1 + g.marketGen.rand.Intn(10),
		}
	}
	
	return positions
}

// Data structures

// MarketUpdate represents a market data update
type MarketUpdate struct {
	Timestamp time.Time
	Ticker    *types.Ticker
	OrderBook *types.OrderBook
	Trades    []*types.Trade
}

// TradingEvent represents an event in a trading session
type TradingEvent struct {
	Type      EventType
	Timestamp time.Time
	Data      interface{}
}

// EventType represents the type of trading event
type EventType string

const (
	EventTypeMarketUpdate EventType = "market_update"
	EventTypeOrderPlaced  EventType = "order_placed"
	EventTypeOrderFilled  EventType = "order_filled"
	EventTypePositionOpen EventType = "position_open"
	EventTypePositionClose EventType = "position_close"
)

// TradingSession represents a complete trading session
type TradingSession struct {
	StartTime time.Time
	EndTime   time.Time
	Symbol    string
	Scenario  ScenarioType
	Events    []TradingEvent
}

// StressTestData contains data for stress testing
type StressTestData struct {
	HighFrequencyOrders []*types.Order
	LargeOrderBooks     []*types.OrderBook
	RapidPriceMovements []*types.Ticker
	ConcurrentPositions []*types.Position
}