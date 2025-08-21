package router

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// LiquidityAggregator aggregates liquidity from multiple venues
type LiquidityAggregator struct {
	mu              sync.RWMutex
	venues          map[string]VenueClient
	orderBooks      map[string]map[string]*types.OrderBook // symbol -> venue -> order book
	aggregatedBooks map[string]*AggregatedOrderBook        // symbol -> aggregated book
	updateInterval  time.Duration
	stopCh          chan struct{}
}

// VenueClient interface for venue connections
type VenueClient interface {
	GetOrderBook(ctx context.Context, symbol string) (*types.OrderBook, error)
	GetVenueInfo() *VenueInfo
	IsConnected() bool
}

// AggregatedOrderBook represents liquidity aggregated from multiple venues
type AggregatedOrderBook struct {
	Symbol       string
	Bids         []AggregatedLevel
	Asks         []AggregatedLevel
	LastUpdate   time.Time
	VenueCount   int
	TotalBidSize decimal.Decimal
	TotalAskSize decimal.Decimal
}

// AggregatedLevel represents a price level with venue information
type AggregatedLevel struct {
	Price       decimal.Decimal
	TotalSize   decimal.Decimal
	VenueSizes  map[string]decimal.Decimal // venue -> size at this level
	VenueCount  int
}

// NewLiquidityAggregator creates a new liquidity aggregator
func NewLiquidityAggregator(updateInterval time.Duration) *LiquidityAggregator {
	return &LiquidityAggregator{
		venues:          make(map[string]VenueClient),
		orderBooks:      make(map[string]map[string]*types.OrderBook),
		aggregatedBooks: make(map[string]*AggregatedOrderBook),
		updateInterval:  updateInterval,
		stopCh:          make(chan struct{}),
	}
}

// AddVenue adds a venue to the aggregator
func (la *LiquidityAggregator) AddVenue(name string, client VenueClient) {
	la.mu.Lock()
	defer la.mu.Unlock()
	la.venues[name] = client
}

// Start starts the liquidity aggregation
func (la *LiquidityAggregator) Start(ctx context.Context) {
	go la.aggregationLoop(ctx)
}

// Stop stops the liquidity aggregation
func (la *LiquidityAggregator) Stop() {
	close(la.stopCh)
}

// GetAggregatedBook returns the aggregated order book for a symbol
func (la *LiquidityAggregator) GetAggregatedBook(symbol string) (*AggregatedOrderBook, error) {
	la.mu.RLock()
	defer la.mu.RUnlock()

	book, exists := la.aggregatedBooks[symbol]
	if !exists {
		return nil, fmt.Errorf("no aggregated book for symbol %s", symbol)
	}

	// Return a copy to avoid race conditions
	return la.copyAggregatedBook(book), nil
}

// GetBestPrices returns the best bid and ask prices across all venues
func (la *LiquidityAggregator) GetBestPrices(symbol string) (bestBid, bestAsk decimal.Decimal, err error) {
	book, err := la.GetAggregatedBook(symbol)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}

	if len(book.Bids) == 0 || len(book.Asks) == 0 {
		return decimal.Zero, decimal.Zero, fmt.Errorf("no liquidity available")
	}

	return book.Bids[0].Price, book.Asks[0].Price, nil
}

// GetLiquidityDepth returns available liquidity up to a certain price level
func (la *LiquidityAggregator) GetLiquidityDepth(symbol string, side types.OrderSide, depth int) ([]LiquidityLevel, error) {
	book, err := la.GetAggregatedBook(symbol)
	if err != nil {
		return nil, err
	}

	levels := []LiquidityLevel{}
	cumVolume := decimal.Zero

	if side == types.OrderSideBuy {
		for i, ask := range book.Asks {
			if i >= depth {
				break
			}
			cumVolume = cumVolume.Add(ask.TotalSize)
			levels = append(levels, LiquidityLevel{
				Price:            ask.Price,
				Volume:           ask.TotalSize,
				CumulativeVolume: cumVolume,
				Venues:           la.getVenuesForLevel(ask.VenueSizes),
			})
		}
	} else {
		for i, bid := range book.Bids {
			if i >= depth {
				break
			}
			cumVolume = cumVolume.Add(bid.TotalSize)
			levels = append(levels, LiquidityLevel{
				Price:            bid.Price,
				Volume:           bid.TotalSize,
				CumulativeVolume: cumVolume,
				Venues:           la.getVenuesForLevel(bid.VenueSizes),
			})
		}
	}

	return levels, nil
}

// GetVenueSpread returns the spread for each venue
func (la *LiquidityAggregator) GetVenueSpread(symbol string) map[string]decimal.Decimal {
	la.mu.RLock()
	defer la.mu.RUnlock()

	spreads := make(map[string]decimal.Decimal)
	
	if venueBooks, exists := la.orderBooks[symbol]; exists {
		for venue, book := range venueBooks {
			if len(book.Bids) > 0 && len(book.Asks) > 0 {
				spread := book.Asks[0][0].Sub(book.Bids[0][0])
				spreads[venue] = spread
			}
		}
	}

	return spreads
}

// GetMarketConditions analyzes current market conditions
func (la *LiquidityAggregator) GetMarketConditions(symbol string) (*MarketConditions, error) {
	la.mu.RLock()
	defer la.mu.RUnlock()

	aggBook, exists := la.aggregatedBooks[symbol]
	if !exists {
		return nil, fmt.Errorf("no data for symbol %s", symbol)
	}

	conditions := &MarketConditions{
		Symbol:      symbol,
		Timestamp:   time.Now(),
		OrderBooks:  make(map[string]*types.OrderBook),
	}

	// Calculate spread
	if len(aggBook.Bids) > 0 && len(aggBook.Asks) > 0 {
		conditions.Spread = aggBook.Asks[0].Price.Sub(aggBook.Bids[0].Price)
	}

	// Calculate liquidity info
	conditions.Liquidity = la.calculateLiquidityInfo(aggBook)

	// Copy individual order books
	if venueBooks, exists := la.orderBooks[symbol]; exists {
		for venue, book := range venueBooks {
			conditions.OrderBooks[venue] = book
		}
	}

	// Simple volatility calculation (would need historical data for accurate calculation)
	conditions.Volatility = la.estimateVolatility(symbol)

	// Determine trend (simplified)
	conditions.TrendDirection = la.determineTrend(symbol)

	return conditions, nil
}

// aggregationLoop continuously updates aggregated order books
func (la *LiquidityAggregator) aggregationLoop(ctx context.Context) {
	ticker := time.NewTicker(la.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-la.stopCh:
			return
		case <-ticker.C:
			la.updateAllOrderBooks(ctx)
		}
	}
}

// updateAllOrderBooks updates order books from all venues
func (la *LiquidityAggregator) updateAllOrderBooks(ctx context.Context) {
	// Get unique symbols across all order books
	symbols := la.getAllSymbols()

	// Update each symbol concurrently
	var wg sync.WaitGroup
	for _, symbol := range symbols {
		wg.Add(1)
		go func(s string) {
			defer wg.Done()
			la.updateSymbolOrderBooks(ctx, s)
		}(symbol)
	}
	wg.Wait()
}

// updateSymbolOrderBooks updates order books for a specific symbol
func (la *LiquidityAggregator) updateSymbolOrderBooks(ctx context.Context, symbol string) {
	venueBooks := make(map[string]*types.OrderBook)
	
	// Fetch from each venue concurrently
	var mu sync.Mutex
	var wg sync.WaitGroup
	
	for venueName, client := range la.venues {
		if !client.IsConnected() {
			continue
		}
		
		wg.Add(1)
		go func(name string, c VenueClient) {
			defer wg.Done()
			
			book, err := c.GetOrderBook(ctx, symbol)
			if err == nil && book != nil {
				mu.Lock()
				venueBooks[name] = book
				mu.Unlock()
			}
		}(venueName, client)
	}
	wg.Wait()

	// Update stored books and aggregate
	la.mu.Lock()
	defer la.mu.Unlock()

	if _, exists := la.orderBooks[symbol]; !exists {
		la.orderBooks[symbol] = make(map[string]*types.OrderBook)
	}

	for venue, book := range venueBooks {
		la.orderBooks[symbol][venue] = book
	}

	// Aggregate the books
	la.aggregatedBooks[symbol] = la.aggregateBooks(symbol, venueBooks)
}

// aggregateBooks combines order books from multiple venues
func (la *LiquidityAggregator) aggregateBooks(symbol string, venueBooks map[string]*types.OrderBook) *AggregatedOrderBook {
	// Map to store aggregated levels
	bidMap := make(map[string]*AggregatedLevel) // price string -> level
	askMap := make(map[string]*AggregatedLevel)

	// Aggregate bids and asks
	for venue, book := range venueBooks {
		// Process bids
		for _, bid := range book.Bids {
			price := bid[0]
			size := bid[1]
			priceStr := price.String()

			if level, exists := bidMap[priceStr]; exists {
				level.TotalSize = level.TotalSize.Add(size)
				level.VenueSizes[venue] = size
				level.VenueCount++
			} else {
				bidMap[priceStr] = &AggregatedLevel{
					Price:      price,
					TotalSize:  size,
					VenueSizes: map[string]decimal.Decimal{venue: size},
					VenueCount: 1,
				}
			}
		}

		// Process asks
		for _, ask := range book.Asks {
			price := ask[0]
			size := ask[1]
			priceStr := price.String()

			if level, exists := askMap[priceStr]; exists {
				level.TotalSize = level.TotalSize.Add(size)
				level.VenueSizes[venue] = size
				level.VenueCount++
			} else {
				askMap[priceStr] = &AggregatedLevel{
					Price:      price,
					TotalSize:  size,
					VenueSizes: map[string]decimal.Decimal{venue: size},
					VenueCount: 1,
				}
			}
		}
	}

	// Convert maps to sorted slices
	bids := la.sortBids(bidMap)
	asks := la.sortAsks(askMap)

	// Calculate totals
	totalBidSize := decimal.Zero
	totalAskSize := decimal.Zero
	
	for _, bid := range bids {
		totalBidSize = totalBidSize.Add(bid.TotalSize)
	}
	for _, ask := range asks {
		totalAskSize = totalAskSize.Add(ask.TotalSize)
	}

	return &AggregatedOrderBook{
		Symbol:       symbol,
		Bids:         bids,
		Asks:         asks,
		LastUpdate:   time.Now(),
		VenueCount:   len(venueBooks),
		TotalBidSize: totalBidSize,
		TotalAskSize: totalAskSize,
	}
}

// sortBids sorts bids in descending order (highest price first)
func (la *LiquidityAggregator) sortBids(bidMap map[string]*AggregatedLevel) []AggregatedLevel {
	bids := make([]AggregatedLevel, 0, len(bidMap))
	for _, level := range bidMap {
		bids = append(bids, *level)
	}

	// Simple bubble sort (for small datasets)
	for i := 0; i < len(bids); i++ {
		for j := i + 1; j < len(bids); j++ {
			if bids[j].Price.GreaterThan(bids[i].Price) {
				bids[i], bids[j] = bids[j], bids[i]
			}
		}
	}

	return bids
}

// sortAsks sorts asks in ascending order (lowest price first)
func (la *LiquidityAggregator) sortAsks(askMap map[string]*AggregatedLevel) []AggregatedLevel {
	asks := make([]AggregatedLevel, 0, len(askMap))
	for _, level := range askMap {
		asks = append(asks, *level)
	}

	// Simple bubble sort (for small datasets)
	for i := 0; i < len(asks); i++ {
		for j := i + 1; j < len(asks); j++ {
			if asks[j].Price.LessThan(asks[i].Price) {
				asks[i], asks[j] = asks[j], asks[i]
			}
		}
	}

	return asks
}

// Helper methods

func (la *LiquidityAggregator) getAllSymbols() []string {
	la.mu.RLock()
	defer la.mu.RUnlock()

	symbolMap := make(map[string]bool)
	for symbol := range la.orderBooks {
		symbolMap[symbol] = true
	}

	symbols := make([]string, 0, len(symbolMap))
	for symbol := range symbolMap {
		symbols = append(symbols, symbol)
	}

	return symbols
}

func (la *LiquidityAggregator) getVenuesForLevel(venueSizes map[string]decimal.Decimal) []string {
	venues := make([]string, 0, len(venueSizes))
	for venue := range venueSizes {
		venues = append(venues, venue)
	}
	return venues
}

func (la *LiquidityAggregator) copyAggregatedBook(book *AggregatedOrderBook) *AggregatedOrderBook {
	copy := &AggregatedOrderBook{
		Symbol:       book.Symbol,
		LastUpdate:   book.LastUpdate,
		VenueCount:   book.VenueCount,
		TotalBidSize: book.TotalBidSize,
		TotalAskSize: book.TotalAskSize,
		Bids:         make([]AggregatedLevel, len(book.Bids)),
		Asks:         make([]AggregatedLevel, len(book.Asks)),
	}

	copy.Bids = append(copy.Bids[:0], book.Bids...)
	copy.Asks = append(copy.Asks[:0], book.Asks...)

	return copy
}

func (la *LiquidityAggregator) calculateLiquidityInfo(book *AggregatedOrderBook) LiquidityInfo {
	info := LiquidityInfo{
		BidLiquidity:   []LiquidityLevel{},
		AskLiquidity:   []LiquidityLevel{},
		TotalBidVolume: book.TotalBidSize,
		TotalAskVolume: book.TotalAskSize,
	}

	// Calculate imbalance ratio
	if !book.TotalBidSize.IsZero() {
		info.ImbalanceRatio = book.TotalAskSize.Div(book.TotalBidSize)
	}

	// Convert aggregated levels to liquidity levels
	cumBid := decimal.Zero
	for _, bid := range book.Bids {
		cumBid = cumBid.Add(bid.TotalSize)
		info.BidLiquidity = append(info.BidLiquidity, LiquidityLevel{
			Price:            bid.Price,
			Volume:           bid.TotalSize,
			CumulativeVolume: cumBid,
			Venues:           la.getVenuesForLevel(bid.VenueSizes),
		})
	}

	cumAsk := decimal.Zero
	for _, ask := range book.Asks {
		cumAsk = cumAsk.Add(ask.TotalSize)
		info.AskLiquidity = append(info.AskLiquidity, LiquidityLevel{
			Price:            ask.Price,
			Volume:           ask.TotalSize,
			CumulativeVolume: cumAsk,
			Venues:           la.getVenuesForLevel(ask.VenueSizes),
		})
	}

	return info
}

func (la *LiquidityAggregator) estimateVolatility(symbol string) float64 {
	// Simplified volatility estimation
	// In production, use historical price data
	return 0.02 // 2% placeholder
}

func (la *LiquidityAggregator) determineTrend(symbol string) string {
	// Simplified trend determination
	// In production, use technical indicators
	return "sideways"
}

