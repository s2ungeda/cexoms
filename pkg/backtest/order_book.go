package backtest

import (
	"math"
	"sync"
	
	"github.com/your-org/mExOms/pkg/types"
)

// SimulatedOrderBook simulates order book for backtesting
type SimulatedOrderBook struct {
	books           map[string]*OrderBook
	volatility      map[string]float64
	averageVolume   map[string]float64
	mu              sync.RWMutex
}

// OrderBook represents a simulated order book
type OrderBook struct {
	Symbol    string
	Bids      []PriceLevel
	Asks      []PriceLevel
	LastPrice float64
	Spread    float64
	UpdateTime int64
}

// NewSimulatedOrderBook creates a new simulated order book
func NewSimulatedOrderBook() *SimulatedOrderBook {
	return &SimulatedOrderBook{
		books:         make(map[string]*OrderBook),
		volatility:    make(map[string]float64),
		averageVolume: make(map[string]float64),
	}
}

// Update updates order book with new data
func (sob *SimulatedOrderBook) Update(data *OrderBookData) {
	sob.mu.Lock()
	defer sob.mu.Unlock()
	
	book := &OrderBook{
		Symbol:     data.Symbol,
		Bids:       data.Bids,
		Asks:       data.Asks,
		UpdateTime: data.Timestamp.Unix(),
	}
	
	// Calculate spread
	if len(data.Bids) > 0 && len(data.Asks) > 0 {
		book.Spread = data.Asks[0].Price - data.Bids[0].Price
		book.LastPrice = (data.Asks[0].Price + data.Bids[0].Price) / 2
	}
	
	sob.books[data.Symbol] = book
	
	// Update volatility estimate
	sob.updateVolatility(data.Symbol, book.LastPrice)
}

// UpdateFromTick updates order book from tick data
func (sob *SimulatedOrderBook) UpdateFromTick(tick *TickData) {
	sob.mu.Lock()
	defer sob.mu.Unlock()
	
	book, exists := sob.books[tick.Symbol]
	if !exists {
		book = &OrderBook{
			Symbol: tick.Symbol,
			Bids:   make([]PriceLevel, 0),
			Asks:   make([]PriceLevel, 0),
		}
		sob.books[tick.Symbol] = book
	}
	
	// Update with tick data
	book.LastPrice = tick.Price
	book.UpdateTime = tick.Timestamp.Unix()
	
	// Create simple order book from bid/ask
	if tick.BidPrice > 0 && tick.BidSize > 0 {
		book.Bids = []PriceLevel{{Price: tick.BidPrice, Quantity: tick.BidSize}}
	}
	if tick.AskPrice > 0 && tick.AskSize > 0 {
		book.Asks = []PriceLevel{{Price: tick.AskPrice, Quantity: tick.AskSize}}
	}
	
	if len(book.Bids) > 0 && len(book.Asks) > 0 {
		book.Spread = book.Asks[0].Price - book.Bids[0].Price
	}
	
	// Update volume tracking
	sob.updateAverageVolume(tick.Symbol, tick.Volume)
	
	// Update volatility
	sob.updateVolatility(tick.Symbol, tick.Price)
}

// GetSpread returns the current spread for a symbol
func (sob *SimulatedOrderBook) GetSpread(symbol string) float64 {
	sob.mu.RLock()
	defer sob.mu.RUnlock()
	
	book, exists := sob.books[symbol]
	if !exists {
		return 0
	}
	
	return book.Spread
}

// GetMarketDepth returns the market depth for a side
func (sob *SimulatedOrderBook) GetMarketDepth(symbol string, side types.OrderSide) float64 {
	sob.mu.RLock()
	defer sob.mu.RUnlock()
	
	book, exists := sob.books[symbol]
	if !exists {
		return 0
	}
	
	depth := 0.0
	
	if side == types.OrderSideBuy {
		// Sum ask quantities
		for _, level := range book.Asks {
			depth += level.Quantity
		}
	} else {
		// Sum bid quantities
		for _, level := range book.Bids {
			depth += level.Quantity
		}
	}
	
	return depth
}

// GetLevels returns order book levels for a side
func (sob *SimulatedOrderBook) GetLevels(symbol string, side types.OrderSide, maxLevels int) []PriceLevel {
	sob.mu.RLock()
	defer sob.mu.RUnlock()
	
	book, exists := sob.books[symbol]
	if !exists {
		return nil
	}
	
	var levels []PriceLevel
	
	if side == types.OrderSideBuy {
		// For buy orders, we consume asks
		levels = book.Asks
	} else {
		// For sell orders, we consume bids
		levels = book.Bids
	}
	
	if len(levels) > maxLevels {
		return levels[:maxLevels]
	}
	
	return levels
}

// GetVolatility returns estimated volatility for a symbol
func (sob *SimulatedOrderBook) GetVolatility(symbol string) float64 {
	sob.mu.RLock()
	defer sob.mu.RUnlock()
	
	vol, exists := sob.volatility[symbol]
	if !exists {
		return 0.01 // Default 1% volatility
	}
	
	return vol
}

// GetAverageVolume returns average volume for a symbol
func (sob *SimulatedOrderBook) GetAverageVolume(symbol string) float64 {
	sob.mu.RLock()
	defer sob.mu.RUnlock()
	
	vol, exists := sob.averageVolume[symbol]
	if !exists {
		return 1000000 // Default volume
	}
	
	return vol
}

// updateVolatility updates volatility estimate
func (sob *SimulatedOrderBook) updateVolatility(symbol string, price float64) {
	// Simple exponential moving average of price changes
	book, exists := sob.books[symbol]
	if !exists || book.LastPrice == 0 {
		return
	}
	
	priceChange := math.Abs((price - book.LastPrice) / book.LastPrice)
	
	currentVol, exists := sob.volatility[symbol]
	if !exists {
		sob.volatility[symbol] = priceChange
	} else {
		// EMA with alpha = 0.1
		sob.volatility[symbol] = currentVol*0.9 + priceChange*0.1
	}
}

// updateAverageVolume updates average volume
func (sob *SimulatedOrderBook) updateAverageVolume(symbol string, volume float64) {
	currentAvg, exists := sob.averageVolume[symbol]
	if !exists {
		sob.averageVolume[symbol] = volume
	} else {
		// EMA with alpha = 0.05
		sob.averageVolume[symbol] = currentAvg*0.95 + volume*0.05
	}
}

// GetBestBid returns the best bid price
func (sob *SimulatedOrderBook) GetBestBid(symbol string) (float64, float64) {
	sob.mu.RLock()
	defer sob.mu.RUnlock()
	
	book, exists := sob.books[symbol]
	if !exists || len(book.Bids) == 0 {
		return 0, 0
	}
	
	return book.Bids[0].Price, book.Bids[0].Quantity
}

// GetBestAsk returns the best ask price
func (sob *SimulatedOrderBook) GetBestAsk(symbol string) (float64, float64) {
	sob.mu.RLock()
	defer sob.mu.RUnlock()
	
	book, exists := sob.books[symbol]
	if !exists || len(book.Asks) == 0 {
		return 0, 0
	}
	
	return book.Asks[0].Price, book.Asks[0].Quantity
}

// GetMidPrice returns the mid price
func (sob *SimulatedOrderBook) GetMidPrice(symbol string) float64 {
	bidPrice, _ := sob.GetBestBid(symbol)
	askPrice, _ := sob.GetBestAsk(symbol)
	
	if bidPrice == 0 || askPrice == 0 {
		// Fall back to last price
		sob.mu.RLock()
		book, exists := sob.books[symbol]
		sob.mu.RUnlock()
		
		if exists {
			return book.LastPrice
		}
		return 0
	}
	
	return (bidPrice + askPrice) / 2
}

// SimulateMarketImpact simulates the market impact of a large order
func (sob *SimulatedOrderBook) SimulateMarketImpact(symbol string, side types.OrderSide, quantity float64) float64 {
	sob.mu.RLock()
	defer sob.mu.RUnlock()
	
	book, exists := sob.books[symbol]
	if !exists {
		return 0
	}
	
	var levels []PriceLevel
	if side == types.OrderSideBuy {
		levels = book.Asks
	} else {
		levels = book.Bids
	}
	
	if len(levels) == 0 {
		return 0
	}
	
	remainingQty := quantity
	totalCost := 0.0
	basePrice := levels[0].Price
	
	// Walk through order book
	for _, level := range levels {
		if remainingQty <= 0 {
			break
		}
		
		fillQty := math.Min(remainingQty, level.Quantity)
		totalCost += fillQty * level.Price
		remainingQty -= fillQty
	}
	
	// If not enough liquidity, estimate additional impact
	if remainingQty > 0 {
		// Use square-root impact model
		totalDepth := sob.GetMarketDepth(symbol, side)
		if totalDepth > 0 {
			impactFactor := math.Sqrt(remainingQty / totalDepth)
			lastPrice := levels[len(levels)-1].Price
			impactPrice := lastPrice * (1 + impactFactor*0.001) // 0.1% per sqrt(participation)
			totalCost += remainingQty * impactPrice
		}
	}
	
	// Calculate average execution price
	avgPrice := totalCost / quantity
	
	// Return impact as difference from best price
	return math.Abs(avgPrice - basePrice)
}