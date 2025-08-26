package router

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// PriceAggregator aggregates price data from multiple exchanges
type PriceAggregator struct {
	mu    sync.RWMutex
	cache map[string]map[string]*MarketData // symbol -> exchange -> data
}

// NewPriceAggregator creates a new price aggregator
func NewPriceAggregator() *PriceAggregator {
	return &PriceAggregator{
		cache: make(map[string]map[string]*MarketData),
	}
}

// GetAggregatedData gets aggregated market data for a symbol
func (pa *PriceAggregator) GetAggregatedData(symbol string, exchanges map[string]types.Exchange) (map[string]*MarketData, error) {
	pa.mu.RLock()
	cached, exists := pa.cache[symbol]
	pa.mu.RUnlock()

	// Return cached data if fresh
	if exists && pa.isFresh(cached) {
		return cached, nil
	}

	// Fetch fresh data
	return pa.fetchAndCache(symbol, exchanges)
}

// fetchAndCache fetches fresh market data and caches it
func (pa *PriceAggregator) fetchAndCache(symbol string, exchanges map[string]types.Exchange) (map[string]*MarketData, error) {
	marketData := make(map[string]*MarketData)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, exchange := range exchanges {
		wg.Add(1)
		go func(exName string, ex types.Exchange) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Get orderbook
			orderbook, err := ex.GetOrderBook(ctx, symbol, 5)
			if err != nil {
				return
			}

			// Get ticker
			ticker, err := ex.GetTicker(ctx, symbol)
			if err != nil {
				return
			}

			// Calculate spread
			bidPrice := orderbook.Bids[0].Price
			askPrice := orderbook.Asks[0].Price
			spread := askPrice.Sub(bidPrice)
			spreadBps := spread.Div(askPrice).Mul(decimal.NewFromInt(10000)) // basis points

			mu.Lock()
			marketData[exName] = &MarketData{
				Exchange:  exName,
				Symbol:    symbol,
				BidPrice:  bidPrice,
				AskPrice:  askPrice,
				BidVolume: orderbook.Bids[0].Quantity,
				AskVolume: orderbook.Asks[0].Quantity,
				Spread:    spread,
				SpreadBps: spreadBps,
				Volume24h: ticker.Volume,
				UpdatedAt: time.Now(),
			}
			mu.Unlock()
		}(name, exchange)
	}

	wg.Wait()

	// Cache the data
	pa.mu.Lock()
	pa.cache[symbol] = marketData
	pa.mu.Unlock()

	return marketData, nil
}

// UpdatePrices updates prices for all cached symbols
func (pa *PriceAggregator) UpdatePrices(exchanges map[string]types.Exchange) {
	pa.mu.RLock()
	symbols := make([]string, 0, len(pa.cache))
	for symbol := range pa.cache {
		symbols = append(symbols, symbol)
	}
	pa.mu.RUnlock()

	for _, symbol := range symbols {
		pa.fetchAndCache(symbol, exchanges)
	}
}

// isFresh checks if cached data is fresh
func (pa *PriceAggregator) isFresh(data map[string]*MarketData) bool {
	for _, md := range data {
		if time.Since(md.UpdatedAt) > 5*time.Second {
			return false
		}
	}
	return true
}

// GetBestPrice returns the best price across all exchanges
func (pa *PriceAggregator) GetBestPrice(symbol string, side string) (*MarketData, error) {
	pa.mu.RLock()
	defer pa.mu.RUnlock()

	data, exists := pa.cache[symbol]
	if !exists {
		return nil, fmt.Errorf("no data for symbol %s", symbol)
	}

	var best *MarketData
	for _, md := range data {
		if best == nil {
			best = md
			continue
		}

		if side == types.OrderSideBuy {
			// For buy orders, lower ask price is better
			if md.AskPrice.LessThan(best.AskPrice) {
				best = md
			}
		} else {
			// For sell orders, higher bid price is better
			if md.BidPrice.GreaterThan(best.BidPrice) {
				best = md
			}
		}
	}

	return best, nil
}

// GetSpreadAnalysis returns spread analysis across exchanges
func (pa *PriceAggregator) GetSpreadAnalysis(symbol string) map[string]decimal.Decimal {
	pa.mu.RLock()
	defer pa.mu.RUnlock()

	spreads := make(map[string]decimal.Decimal)
	data, exists := pa.cache[symbol]
	if !exists {
		return spreads
	}

	for exchange, md := range data {
		spreads[exchange] = md.SpreadBps
	}

	return spreads
}

// GetVolumeAnalysis returns volume analysis across exchanges
func (pa *PriceAggregator) GetVolumeAnalysis(symbol string) map[string]decimal.Decimal {
	pa.mu.RLock()
	defer pa.mu.RUnlock()

	volumes := make(map[string]decimal.Decimal)
	data, exists := pa.cache[symbol]
	if !exists {
		return volumes
	}

	totalVolume := decimal.Zero
	for _, md := range data {
		totalVolume = totalVolume.Add(md.Volume24h)
	}

	if totalVolume.IsZero() {
		return volumes
	}

	// Calculate volume percentage for each exchange
	for exchange, md := range data {
		percentage := md.Volume24h.Div(totalVolume).Mul(decimal.NewFromInt(100))
		volumes[exchange] = percentage
	}

	return volumes
}

// GetLiquidityDepth returns aggregated liquidity depth
func (pa *PriceAggregator) GetLiquidityDepth(symbol string, side string) decimal.Decimal {
	pa.mu.RLock()
	defer pa.mu.RUnlock()

	totalLiquidity := decimal.Zero
	data, exists := pa.cache[symbol]
	if !exists {
		return totalLiquidity
	}

	for _, md := range data {
		if side == types.OrderSideBuy {
			totalLiquidity = totalLiquidity.Add(md.AskVolume)
		} else {
			totalLiquidity = totalLiquidity.Add(md.BidVolume)
		}
	}

	return totalLiquidity
}