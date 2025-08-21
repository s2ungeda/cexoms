package generators

import (
	"math/rand"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// MarketDataGenerator generates realistic market data for testing
type MarketDataGenerator struct {
	rand       *rand.Rand
	basePrice  float64
	volatility float64
}

// NewMarketDataGenerator creates a new market data generator
func NewMarketDataGenerator(seed int64) *MarketDataGenerator {
	return &MarketDataGenerator{
		rand:       rand.New(rand.NewSource(seed)),
		basePrice:  40000, // Default BTC price
		volatility: 0.02,  // 2% volatility
	}
}

// SetBasePrice sets the base price for generation
func (g *MarketDataGenerator) SetBasePrice(price float64) {
	g.basePrice = price
}

// SetVolatility sets the price volatility (0.01 = 1%)
func (g *MarketDataGenerator) SetVolatility(v float64) {
	g.volatility = v
}

// GenerateTicker generates a random ticker
func (g *MarketDataGenerator) GenerateTicker(symbol string) *types.Ticker {
	// Random walk from base price
	priceChange := g.rand.NormFloat64() * g.volatility * g.basePrice
	price := g.basePrice + priceChange
	
	// Ensure positive price
	if price <= 0 {
		price = g.basePrice
	}
	
	// Generate bid/ask spread (0.01% - 0.05%)
	spreadPercent := 0.0001 + g.rand.Float64()*0.0004
	halfSpread := price * spreadPercent / 2
	
	// Generate volume (random between 100-10000 units)
	volume := 100 + g.rand.Float64()*9900
	
	return &types.Ticker{
		Symbol:    symbol,
		Price:     decimal.NewFromFloat(price),
		Bid:       decimal.NewFromFloat(price - halfSpread),
		Ask:       decimal.NewFromFloat(price + halfSpread),
		Volume:    decimal.NewFromFloat(volume),
		High:      decimal.NewFromFloat(price * (1 + g.rand.Float64()*0.01)),
		Low:       decimal.NewFromFloat(price * (1 - g.rand.Float64()*0.01)),
		Timestamp: time.Now(),
	}
}

// GenerateOrderBook generates a realistic order book
func (g *MarketDataGenerator) GenerateOrderBook(symbol string, depth int) *types.OrderBook {
	ticker := g.GenerateTicker(symbol)
	midPrice := ticker.Price.InexactFloat64()
	
	ob := &types.OrderBook{
		Symbol:    symbol,
		Bids:      make([][2]decimal.Decimal, depth),
		Asks:      make([][2]decimal.Decimal, depth),
		Timestamp: time.Now(),
	}
	
	// Generate bids (decreasing prices)
	for i := 0; i < depth; i++ {
		// Price decreases by 0.01% - 0.05% per level
		priceStep := midPrice * (0.0001 + g.rand.Float64()*0.0004)
		price := midPrice - float64(i+1)*priceStep
		
		// Quantity increases with distance from mid (more liquidity deeper)
		baseQty := 0.1 + g.rand.Float64()*0.9
		quantity := baseQty * (1 + float64(i)*0.5)
		
		ob.Bids[i] = [2]decimal.Decimal{
			decimal.NewFromFloat(price),
			decimal.NewFromFloat(quantity),
		}
	}
	
	// Generate asks (increasing prices)
	for i := 0; i < depth; i++ {
		priceStep := midPrice * (0.0001 + g.rand.Float64()*0.0004)
		price := midPrice + float64(i+1)*priceStep
		
		baseQty := 0.1 + g.rand.Float64()*0.9
		quantity := baseQty * (1 + float64(i)*0.5)
		
		ob.Asks[i] = [2]decimal.Decimal{
			decimal.NewFromFloat(price),
			decimal.NewFromFloat(quantity),
		}
	}
	
	return ob
}

// GenerateTrade generates a random trade
func (g *MarketDataGenerator) GenerateTrade(symbol string) *types.Trade {
	ticker := g.GenerateTicker(symbol)
	
	// Randomly choose buy or sell
	side := types.OrderSideBuy
	if g.rand.Float64() > 0.5 {
		side = types.OrderSideSell
	}
	
	// Generate quantity (power law distribution for more realistic sizes)
	minQty := 0.001
	maxQty := 10.0
	quantity := minQty + (maxQty-minQty)*g.rand.Float64()*g.rand.Float64()
	
	return &types.Trade{
		ID:        g.generateTradeID(),
		Symbol:    symbol,
		Price:     ticker.Price,
		Quantity:  decimal.NewFromFloat(quantity),
		Side:      side,
		Timestamp: time.Now(),
	}
}

// GenerateKline generates a candlestick
func (g *MarketDataGenerator) GenerateKline(symbol, interval string) *types.Kline {
	// Start from current base price
	open := g.basePrice
	
	// Generate price movement
	highPercent := g.rand.Float64() * g.volatility
	lowPercent := g.rand.Float64() * g.volatility
	closeChange := g.rand.NormFloat64() * g.volatility
	
	high := open * (1 + highPercent)
	low := open * (1 - lowPercent)
	close := open * (1 + closeChange)
	
	// Ensure close is within high/low
	if close > high {
		high = close
	}
	if close < low {
		low = close
	}
	
	// Generate volume
	volume := 100 + g.rand.Float64()*9900
	
	// Parse interval for time calculation
	duration := g.parseDuration(interval)
	now := time.Now()
	
	return &types.Kline{
		Symbol:    symbol,
		Interval:  interval,
		OpenTime:  now.Add(-duration),
		CloseTime: now,
		Open:      decimal.NewFromFloat(open),
		High:      decimal.NewFromFloat(high),
		Low:       decimal.NewFromFloat(low),
		Close:     decimal.NewFromFloat(close),
		Volume:    decimal.NewFromFloat(volume),
	}
}

// GenerateHistoricalKlines generates a series of connected klines
func (g *MarketDataGenerator) GenerateHistoricalKlines(symbol, interval string, count int) []*types.Kline {
	klines := make([]*types.Kline, count)
	duration := g.parseDuration(interval)
	now := time.Now()
	
	// Start from count periods ago
	currentPrice := g.basePrice
	startTime := now.Add(-duration * time.Duration(count))
	
	for i := 0; i < count; i++ {
		openTime := startTime.Add(duration * time.Duration(i))
		closeTime := openTime.Add(duration)
		
		// Generate OHLC with continuity
		open := currentPrice
		
		// Random walk
		highPercent := g.rand.Float64() * g.volatility * 0.5
		lowPercent := g.rand.Float64() * g.volatility * 0.5
		closeChange := g.rand.NormFloat64() * g.volatility * 0.3
		
		high := open * (1 + highPercent)
		low := open * (1 - lowPercent)
		close := open * (1 + closeChange)
		
		// Ensure close is within high/low
		if close > high {
			high = close
		}
		if close < low {
			low = close
		}
		
		// Volume with some pattern (higher during "active" hours)
		baseVolume := 100 + g.rand.Float64()*900
		hourFactor := 1.0
		hour := openTime.Hour()
		if hour >= 8 && hour <= 16 { // Active hours
			hourFactor = 2.0
		}
		volume := baseVolume * hourFactor
		
		klines[i] = &types.Kline{
			Symbol:    symbol,
			Interval:  interval,
			OpenTime:  openTime,
			CloseTime: closeTime,
			Open:      decimal.NewFromFloat(open),
			High:      decimal.NewFromFloat(high),
			Low:       decimal.NewFromFloat(low),
			Close:     decimal.NewFromFloat(close),
			Volume:    decimal.NewFromFloat(volume),
		}
		
		// Next candle opens at this close
		currentPrice = close
	}
	
	// Update generator's base price to last close
	if count > 0 {
		g.basePrice = currentPrice
	}
	
	return klines
}

// Helper methods

func (g *MarketDataGenerator) generateTradeID() string {
	const charset = "0123456789"
	b := make([]byte, 10)
	for i := range b {
		b[i] = charset[g.rand.Intn(len(charset))]
	}
	return string(b)
}

func (g *MarketDataGenerator) parseDuration(interval string) time.Duration {
	switch interval {
	case "1m":
		return time.Minute
	case "3m":
		return 3 * time.Minute
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "30m":
		return 30 * time.Minute
	case "1h":
		return time.Hour
	case "2h":
		return 2 * time.Hour
	case "4h":
		return 4 * time.Hour
	case "1d":
		return 24 * time.Hour
	default:
		return time.Minute
	}
}