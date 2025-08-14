package router

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
	
	"github.com/mExOms/oms/internal/exchange"
	"github.com/mExOms/oms/pkg/cache"
	"github.com/mExOms/oms/pkg/types"
	"github.com/shopspring/decimal"
)

// SmartRouter implements intelligent order routing across multiple exchanges
type SmartRouter struct {
	exchanges     map[string]types.Exchange
	exchangeCache *cache.MemoryCache
	balanceCache  *cache.MemoryCache
	priceCache    *cache.MemoryCache
	factory       *exchange.Factory
	mu            sync.RWMutex
}

// NewSmartRouter creates a new smart order router
func NewSmartRouter(factory *exchange.Factory) *SmartRouter {
	return &SmartRouter{
		exchanges:     make(map[string]types.Exchange),
		exchangeCache: cache.NewMemoryCache(),
		balanceCache:  cache.NewMemoryCache(),
		priceCache:    cache.NewMemoryCache(),
		factory:       factory,
	}
}

// AddExchange adds an exchange to the router
func (sr *SmartRouter) AddExchange(name string, exchange types.Exchange) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	
	sr.exchanges[name] = exchange
	return nil
}

// RouteOrder routes an order to the best exchange based on price and liquidity
func (sr *SmartRouter) RouteOrder(ctx context.Context, order *types.Order) (*types.Order, error) {
	// Find best exchange for the order
	bestExchange, err := sr.findBestExchange(ctx, order)
	if err != nil {
		return nil, fmt.Errorf("failed to find best exchange: %w", err)
	}
	
	// Check balance on selected exchange
	if err := sr.checkBalance(ctx, bestExchange, order); err != nil {
		return nil, fmt.Errorf("insufficient balance: %w", err)
	}
	
	// Route order to selected exchange
	return bestExchange.CreateOrder(ctx, order)
}

// SplitOrder splits a large order across multiple exchanges
func (sr *SmartRouter) SplitOrder(ctx context.Context, order *types.Order, maxOrderSize decimal.Decimal) ([]*types.Order, error) {
	remainingQty := order.Quantity
	var orders []*types.Order
	
	// Get exchanges sorted by best price
	exchanges := sr.getExchangesByBestPrice(ctx, order.Symbol, order.Side)
	
	for _, exch := range exchanges {
		if remainingQty.LessThanOrEqual(decimal.Zero) {
			break
		}
		
		// Calculate order size for this exchange
		orderQty := remainingQty
		if orderQty.GreaterThan(maxOrderSize) {
			orderQty = maxOrderSize
		}
		
		// Check available liquidity
		liquidity, err := sr.getAvailableLiquidity(ctx, exch, order.Symbol, order.Side)
		if err != nil {
			continue
		}
		
		if orderQty.GreaterThan(liquidity) {
			orderQty = liquidity
		}
		
		// Skip if quantity too small
		if orderQty.LessThan(decimal.NewFromFloat(0.001)) {
			continue
		}
		
		// Create split order
		splitOrder := *order
		splitOrder.Quantity = orderQty
		
		// Check balance
		if err := sr.checkBalance(ctx, exch, &splitOrder); err != nil {
			continue
		}
		
		// Execute order
		resp, err := exch.CreateOrder(ctx, &splitOrder)
		if err != nil {
			continue
		}
		
		orders = append(orders, resp)
		remainingQty = remainingQty.Sub(orderQty)
	}
	
	if remainingQty.GreaterThan(decimal.Zero) {
		return orders, fmt.Errorf("could not fill entire order, remaining: %s", remainingQty.String())
	}
	
	return orders, nil
}

// findBestExchange finds the best exchange for an order based on price
func (sr *SmartRouter) findBestExchange(ctx context.Context, order *types.Order) (types.Exchange, error) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	
	type exchangePrice struct {
		exchange types.Exchange
		price    decimal.Decimal
		volume   decimal.Decimal
	}
	
	var candidates []exchangePrice
	
	// Get prices from all exchanges
	for name, exch := range sr.exchanges {
		// Check if exchange is connected
		if !exch.IsConnected() {
			continue
		}
		
		// Get ticker from cache or fetch
		cacheKey := fmt.Sprintf("ticker:%s:%s", name, order.Symbol)
		ticker, found := sr.priceCache.Get(cacheKey)
		if !found {
			// In production, we would subscribe to WebSocket tickers
			continue
		}
		
		tickerData, ok := ticker.(*types.Ticker)
		if !ok {
			continue
		}
		
		// Get relevant price based on order side
		var price, volume decimal.Decimal
		if order.Side == types.OrderSideBuy {
			price = decimal.RequireFromString(tickerData.AskPrice)
			volume = decimal.RequireFromString(tickerData.AskQty)
		} else {
			price = decimal.RequireFromString(tickerData.BidPrice)
			volume = decimal.RequireFromString(tickerData.BidQty)
		}
		
		// Skip if price is zero
		if price.IsZero() {
			continue
		}
		
		candidates = append(candidates, exchangePrice{
			exchange: exch,
			price:    price,
			volume:   volume,
		})
	}
	
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no available exchanges for %s", order.Symbol)
	}
	
	// Sort by best price
	sort.Slice(candidates, func(i, j int) bool {
		if order.Side == types.OrderSideBuy {
			// For buy orders, lower price is better
			return candidates[i].price.LessThan(candidates[j].price)
		} else {
			// For sell orders, higher price is better
			return candidates[i].price.GreaterThan(candidates[j].price)
		}
	})
	
	return candidates[0].exchange, nil
}

// checkBalance checks if there is sufficient balance for an order
func (sr *SmartRouter) checkBalance(ctx context.Context, exch types.Exchange, order *types.Order) error {
	// Get balance from cache or fetch
	cacheKey := fmt.Sprintf("balance:%s", exch.GetExchangeInfo().Name)
	balance, found := sr.balanceCache.Get(cacheKey)
	if !found {
		// Fetch balance
		bal, err := exch.GetBalance(ctx)
		if err != nil {
			return fmt.Errorf("failed to get balance: %w", err)
		}
		balance = bal
		sr.balanceCache.Set(cacheKey, bal, 10*time.Second)
	}
	
	balData, ok := balance.(*types.Balance)
	if !ok {
		return fmt.Errorf("invalid balance data")
	}
	
	// Check based on order side
	if order.Side == types.OrderSideBuy {
		// For buy orders, check quote currency (e.g., USDT for BTCUSDT)
		// This is simplified - in production we'd parse the symbol properly
		requiredAmount := order.Quantity.Mul(order.Price)
		
		// Check USDT balance (simplified)
		if usdtBalance, exists := balData.Assets["USDT"]; exists {
			available := decimal.RequireFromString(usdtBalance.Free)
			if available.LessThan(requiredAmount) {
				return fmt.Errorf("insufficient USDT balance: need %s, have %s", 
					requiredAmount.String(), available.String())
			}
		} else {
			return fmt.Errorf("no USDT balance found")
		}
	} else {
		// For sell orders, check base currency (e.g., BTC for BTCUSDT)
		// Check if we have enough of the asset to sell
		// This is simplified - in production we'd parse the symbol properly
		if btcBalance, exists := balData.Assets["BTC"]; exists {
			available := decimal.RequireFromString(btcBalance.Free)
			if available.LessThan(order.Quantity) {
				return fmt.Errorf("insufficient BTC balance: need %s, have %s", 
					order.Quantity.String(), available.String())
			}
		}
	}
	
	return nil
}

// getExchangesByBestPrice returns exchanges sorted by best price for a symbol
func (sr *SmartRouter) getExchangesByBestPrice(ctx context.Context, symbol, side string) []types.Exchange {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	
	type exchangePrice struct {
		exchange types.Exchange
		price    decimal.Decimal
	}
	
	var candidates []exchangePrice
	
	for name, exch := range sr.exchanges {
		if !exch.IsConnected() {
			continue
		}
		
		// Get price from cache
		cacheKey := fmt.Sprintf("ticker:%s:%s", name, symbol)
		ticker, found := sr.priceCache.Get(cacheKey)
		if !found {
			continue
		}
		
		tickerData, ok := ticker.(*types.Ticker)
		if !ok {
			continue
		}
		
		var price decimal.Decimal
		if side == types.OrderSideBuy {
			price = decimal.RequireFromString(tickerData.AskPrice)
		} else {
			price = decimal.RequireFromString(tickerData.BidPrice)
		}
		
		if price.IsZero() {
			continue
		}
		
		candidates = append(candidates, exchangePrice{
			exchange: exch,
			price:    price,
		})
	}
	
	// Sort by best price
	sort.Slice(candidates, func(i, j int) bool {
		if side == types.OrderSideBuy {
			return candidates[i].price.LessThan(candidates[j].price)
		} else {
			return candidates[i].price.GreaterThan(candidates[j].price)
		}
	})
	
	result := make([]types.Exchange, len(candidates))
	for i, c := range candidates {
		result[i] = c.exchange
	}
	
	return result
}

// getAvailableLiquidity estimates available liquidity for a symbol on an exchange
func (sr *SmartRouter) getAvailableLiquidity(ctx context.Context, exch types.Exchange, symbol, side string) (decimal.Decimal, error) {
	// In production, this would analyze order book depth
	// For now, we'll use ticker quantity as a simple approximation
	
	name := exch.GetExchangeInfo().Name
	cacheKey := fmt.Sprintf("ticker:%s:%s", name, symbol)
	ticker, found := sr.priceCache.Get(cacheKey)
	if !found {
		return decimal.Zero, fmt.Errorf("no ticker data for %s on %s", symbol, name)
	}
	
	tickerData, ok := ticker.(*types.Ticker)
	if !ok {
		return decimal.Zero, fmt.Errorf("invalid ticker data")
	}
	
	if side == types.OrderSideBuy {
		return decimal.RequireFromString(tickerData.AskQty), nil
	} else {
		return decimal.RequireFromString(tickerData.BidQty), nil
	}
}

// DetectArbitrage detects arbitrage opportunities across exchanges
func (sr *SmartRouter) DetectArbitrage(ctx context.Context, symbols []string, minProfitPercent decimal.Decimal) []ArbitrageOpportunity {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	
	var opportunities []ArbitrageOpportunity
	
	for _, symbol := range symbols {
		// Collect prices from all exchanges
		type priceInfo struct {
			exchange string
			bidPrice decimal.Decimal
			askPrice decimal.Decimal
			bidQty   decimal.Decimal
			askQty   decimal.Decimal
		}
		
		var prices []priceInfo
		
		for name, exch := range sr.exchanges {
			if !exch.IsConnected() {
				continue
			}
			
			cacheKey := fmt.Sprintf("ticker:%s:%s", name, symbol)
			ticker, found := sr.priceCache.Get(cacheKey)
			if !found {
				continue
			}
			
			tickerData, ok := ticker.(*types.Ticker)
			if !ok {
				continue
			}
			
			bid := decimal.RequireFromString(tickerData.BidPrice)
			ask := decimal.RequireFromString(tickerData.AskPrice)
			
			if bid.IsZero() || ask.IsZero() {
				continue
			}
			
			prices = append(prices, priceInfo{
				exchange: name,
				bidPrice: bid,
				askPrice: ask,
				bidQty:   decimal.RequireFromString(tickerData.BidQty),
				askQty:   decimal.RequireFromString(tickerData.AskQty),
			})
		}
		
		// Check for arbitrage opportunities
		for i := 0; i < len(prices); i++ {
			for j := i + 1; j < len(prices); j++ {
				// Check if we can buy on one exchange and sell on another for profit
				
				// Buy on exchange i, sell on exchange j
				buyPrice := prices[i].askPrice
				sellPrice := prices[j].bidPrice
				profit := sellPrice.Sub(buyPrice).Div(buyPrice).Mul(decimal.NewFromInt(100))
				
				if profit.GreaterThan(minProfitPercent) {
					opportunities = append(opportunities, ArbitrageOpportunity{
						Symbol:       symbol,
						BuyExchange:  prices[i].exchange,
						SellExchange: prices[j].exchange,
						BuyPrice:     buyPrice,
						SellPrice:    sellPrice,
						ProfitPercent: profit,
						MaxQuantity:  decimal.Min(prices[i].askQty, prices[j].bidQty),
					})
				}
				
				// Buy on exchange j, sell on exchange i
				buyPrice = prices[j].askPrice
				sellPrice = prices[i].bidPrice
				profit = sellPrice.Sub(buyPrice).Div(buyPrice).Mul(decimal.NewFromInt(100))
				
				if profit.GreaterThan(minProfitPercent) {
					opportunities = append(opportunities, ArbitrageOpportunity{
						Symbol:       symbol,
						BuyExchange:  prices[j].exchange,
						SellExchange: prices[i].exchange,
						BuyPrice:     buyPrice,
						SellPrice:    sellPrice,
						ProfitPercent: profit,
						MaxQuantity:  decimal.Min(prices[j].askQty, prices[i].bidQty),
					})
				}
			}
		}
	}
	
	// Sort by profit percentage
	sort.Slice(opportunities, func(i, j int) bool {
		return opportunities[i].ProfitPercent.GreaterThan(opportunities[j].ProfitPercent)
	})
	
	return opportunities
}

// UpdateMarketData updates cached market data for routing decisions
func (sr *SmartRouter) UpdateMarketData(exchange string, symbol string, ticker *types.Ticker) {
	cacheKey := fmt.Sprintf("ticker:%s:%s", exchange, symbol)
	sr.priceCache.Set(cacheKey, ticker, 5*time.Second)
}

// UpdateBalance updates cached balance data
func (sr *SmartRouter) UpdateBalance(exchange string, balance *types.Balance) {
	cacheKey := fmt.Sprintf("balance:%s", exchange)
	sr.balanceCache.Set(cacheKey, balance, 10*time.Second)
}

// ArbitrageOpportunity represents a potential arbitrage trade
type ArbitrageOpportunity struct {
	Symbol        string
	BuyExchange   string
	SellExchange  string
	BuyPrice      decimal.Decimal
	SellPrice     decimal.Decimal
	ProfitPercent decimal.Decimal
	MaxQuantity   decimal.Decimal
}