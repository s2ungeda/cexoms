package backtest

import (
	"sync"
	"time"

	"github.com/shopspring/decimal"
	
	"github.com/mExOms/pkg/types"
)

// SimulatedOrderBook simulates an order book for backtesting
type SimulatedOrderBook struct {
	mu sync.RWMutex
	
	// Current prices by symbol
	prices    map[string]decimal.Decimal
	
	// Spread configuration
	spreadBps int
	
	// Depth levels
	depthLevels int
}

// NewSimulatedOrderBook creates a new simulated order book
func NewSimulatedOrderBook(spreadBps int) *SimulatedOrderBook {
	return &SimulatedOrderBook{
		prices:      make(map[string]decimal.Decimal),
		spreadBps:   spreadBps,
		depthLevels: 10,
	}
}

// UpdatePrice updates the mid price for a symbol
func (s *SimulatedOrderBook) UpdatePrice(symbol string, price decimal.Decimal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.prices[symbol] = price
}

// GetOrderBook generates a simulated order book
func (s *SimulatedOrderBook) GetOrderBook(symbol string, exchange string) *types.OrderBook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	midPrice, exists := s.prices[symbol]
	if !exists {
		return nil
	}
	
	// Calculate spread
	spreadMultiplier := decimal.NewFromInt(int64(s.spreadBps)).Div(decimal.NewFromInt(20000))
	halfSpread := midPrice.Mul(spreadMultiplier)
	
	// Generate bid levels
	bids := make([]types.OrderBookLevel, s.depthLevels)
	for i := 0; i < s.depthLevels; i++ {
		// Price decreases for each level
		levelSpread := halfSpread.Mul(decimal.NewFromInt(int64(i + 1)))
		price := midPrice.Sub(levelSpread)
		
		// Quantity increases with distance from mid (more liquidity deeper)
		quantity := decimal.NewFromFloat(1.0 + float64(i)*0.5)
		
		bids[i] = types.OrderBookLevel{
			Price:    price,
			Quantity: quantity,
		}
	}
	
	// Generate ask levels
	asks := make([]types.OrderBookLevel, s.depthLevels)
	for i := 0; i < s.depthLevels; i++ {
		// Price increases for each level
		levelSpread := halfSpread.Mul(decimal.NewFromInt(int64(i + 1)))
		price := midPrice.Add(levelSpread)
		
		// Quantity increases with distance from mid
		quantity := decimal.NewFromFloat(1.0 + float64(i)*0.5)
		
		asks[i] = types.OrderBookLevel{
			Price:    price,
			Quantity: quantity,
		}
	}
	
	return &types.OrderBook{
		Symbol:   symbol,
		Exchange: exchange,
		Bids:     bids,
		Asks:     asks,
		Time:     time.Now(),
	}
}

// GetBestBid returns the best bid price
func (s *SimulatedOrderBook) GetBestBid(symbol string) decimal.Decimal {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	midPrice, exists := s.prices[symbol]
	if !exists {
		return decimal.Zero
	}
	
	spreadMultiplier := decimal.NewFromInt(int64(s.spreadBps)).Div(decimal.NewFromInt(20000))
	return midPrice.Sub(midPrice.Mul(spreadMultiplier))
}

// GetBestAsk returns the best ask price
func (s *SimulatedOrderBook) GetBestAsk(symbol string) decimal.Decimal {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	midPrice, exists := s.prices[symbol]
	if !exists {
		return decimal.Zero
	}
	
	spreadMultiplier := decimal.NewFromInt(int64(s.spreadBps)).Div(decimal.NewFromInt(20000))
	return midPrice.Add(midPrice.Mul(spreadMultiplier))
}

// GetMidPrice returns the mid price
func (s *SimulatedOrderBook) GetMidPrice(symbol string) decimal.Decimal {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return s.prices[symbol]
}

// SimulateMarketImpact simulates the price impact of a market order
func (s *SimulatedOrderBook) SimulateMarketImpact(symbol string, side types.OrderSide, quantity decimal.Decimal) decimal.Decimal {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	midPrice, exists := s.prices[symbol]
	if !exists {
		return decimal.Zero
	}
	
	// Simple linear impact model
	// Impact increases with size
	impactBps := quantity.Mul(decimal.NewFromFloat(10)).IntPart() // 10 bps per unit
	impactMultiplier := decimal.NewFromInt(impactBps).Div(decimal.NewFromInt(10000))
	
	impact := midPrice.Mul(impactMultiplier)
	
	if side == types.OrderSideBuy {
		return midPrice.Add(impact)
	} else {
		return midPrice.Sub(impact)
	}
}

// GetLiquidity returns available liquidity at a price level
func (s *SimulatedOrderBook) GetLiquidity(symbol string, price decimal.Decimal, side types.OrderSide) decimal.Decimal {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	midPrice, exists := s.prices[symbol]
	if !exists {
		return decimal.Zero
	}
	
	// Calculate distance from mid price
	distance := price.Sub(midPrice).Abs().Div(midPrice)
	
	// Liquidity increases with distance
	baseLiquidity := decimal.NewFromFloat(1.0)
	distanceMultiplier := distance.Mul(decimal.NewFromInt(100)).Add(decimal.NewFromInt(1))
	
	return baseLiquidity.Mul(distanceMultiplier)
}