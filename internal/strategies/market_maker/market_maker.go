package marketmaker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mExOms/internal/account"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// MarketMaker implements the market making strategy
type MarketMaker struct {
	mu sync.RWMutex
	
	// Configuration
	config          *MarketMakerConfig
	
	// Dependencies
	exchange        types.ExchangeMultiAccount
	accountRouter   *account.Router
	
	// Strategy components
	spreadCalc      SpreadCalculator
	inventoryMgr    InventoryManager
	quoteGen        QuoteGenerator
	riskMgr         RiskManager
	
	// State tracking
	activeOrders    map[string]*types.Order
	marketState     *MarketState
	running         bool
	
	// Performance tracking
	metrics         *MMMetrics
	startTime       time.Time
	
	// Channels
	stopCh          chan struct{}
	orderUpdateCh   chan *types.Order
	tradeUpdateCh   chan *types.Trade
	
	// Background workers
	wg              sync.WaitGroup
}

// NewMarketMaker creates a new market maker instance
func NewMarketMaker(
	config *MarketMakerConfig,
	exchange types.ExchangeMultiAccount,
	accountRouter *account.Router,
) *MarketMaker {
	
	// Create strategy components
	spreadCalc := NewDynamicSpreadCalculator(config)
	inventoryMgr := NewInventoryManager(config)
	
	mm := &MarketMaker{
		config:        config,
		exchange:      exchange,
		accountRouter: accountRouter,
		spreadCalc:    spreadCalc,
		inventoryMgr:  inventoryMgr,
		activeOrders:  make(map[string]*types.Order),
		metrics:       &MMMetrics{StartTime: time.Now()},
		stopCh:        make(chan struct{}),
		orderUpdateCh: make(chan *types.Order, 100),
		tradeUpdateCh: make(chan *types.Trade, 100),
	}
	
	// Create quote generator with dependencies
	mm.quoteGen = NewQuoteGenerator(config, spreadCalc, inventoryMgr)
	
	// Create risk manager
	mm.riskMgr = NewRiskManager(config, inventoryMgr)
	
	return mm
}

// Start starts the market making strategy
func (mm *MarketMaker) Start(ctx context.Context) error {
	mm.mu.Lock()
	if mm.running {
		mm.mu.Unlock()
		return fmt.Errorf("market maker already running")
	}
	mm.running = true
	mm.startTime = time.Now()
	mm.mu.Unlock()
	
	// Select account for this strategy
	account, err := mm.accountRouter.RouteOrder(ctx, mm.config.Exchange, &types.Order{
		Symbol: mm.config.Symbol,
	})
	if err != nil {
		return fmt.Errorf("failed to select account: %w", err)
	}
	
	// Set account on exchange
	mm.exchange.SetAccount(account.ID)
	
	// Subscribe to market data
	if err := mm.subscribeToMarketData(); err != nil {
		return fmt.Errorf("failed to subscribe to market data: %w", err)
	}
	
	// Subscribe to order/trade updates
	if err := mm.subscribeToOrderUpdates(); err != nil {
		return fmt.Errorf("failed to subscribe to order updates: %w", err)
	}
	
	// Start background workers
	mm.wg.Add(3)
	go mm.quoteWorker(ctx)
	go mm.orderWorker(ctx)
	go mm.metricsWorker(ctx)
	
	return nil
}

// Stop stops the market making strategy
func (mm *MarketMaker) Stop() error {
	mm.mu.Lock()
	if !mm.running {
		mm.mu.Unlock()
		return fmt.Errorf("market maker not running")
	}
	mm.running = false
	mm.mu.Unlock()
	
	// Cancel all active orders
	if err := mm.cancelAllOrders(); err != nil {
		// Log error but continue shutdown
		fmt.Printf("Error canceling orders: %v\n", err)
	}
	
	// Signal workers to stop
	close(mm.stopCh)
	
	// Wait for workers to finish
	mm.wg.Wait()
	
	// Calculate final metrics
	mm.finalizeMetrics()
	
	return nil
}

// quoteWorker manages quote generation and updates
func (mm *MarketMaker) quoteWorker(ctx context.Context) {
	defer mm.wg.Done()
	
	ticker := time.NewTicker(mm.config.RefreshRate)
	defer ticker.Stop()
	
	// Initial quote generation
	mm.refreshQuotes(ctx)
	
	for {
		select {
		case <-ticker.C:
			mm.refreshQuotes(ctx)
			
		case <-mm.stopCh:
			return
			
		case <-ctx.Done():
			return
		}
	}
}

// refreshQuotes updates market maker quotes
func (mm *MarketMaker) refreshQuotes(ctx context.Context) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	
	if !mm.running {
		return
	}
	
	// Check risk limits
	if mm.riskMgr.ShouldStop() {
		mm.cancelAllOrdersLocked()
		return
	}
	
	// Get current market state
	if mm.marketState == nil {
		return // No market data yet
	}
	
	// Get inventory state
	inventoryState := mm.inventoryMgr.(*InventoryManagerImpl).GetInventoryState()
	
	// Generate new quotes
	newQuotes, err := mm.quoteGen.GenerateQuotes(mm.marketState, inventoryState)
	if err != nil {
		fmt.Printf("Error generating quotes: %v\n", err)
		return
	}
	
	// Cancel existing orders that don't match new quotes
	mm.updateActiveOrders(ctx, newQuotes)
	
	// Place new orders
	for _, quote := range newQuotes {
		if _, exists := mm.findMatchingOrder(quote); !exists {
			mm.placeQuote(ctx, quote)
		}
	}
}

// placeQuote places a single quote order
func (mm *MarketMaker) placeQuote(ctx context.Context, quote *Quote) {
	order := &types.Order{
		ClientOrderID: quote.OrderID,
		Symbol:        mm.config.Symbol,
		Side:          quote.Side,
		Type:          types.OrderTypeLimit,
		Price:         quote.Price,
		Quantity:      quote.Quantity,
		TimeInForce:   types.TimeInForceGTC,
	}
	
	// Apply post-only if configured
	if mm.config.UsePostOnly {
		order.PostOnly = true
	}
	
	// Check risk before placing
	if err := mm.riskMgr.CheckOrderRisk(order); err != nil {
		fmt.Printf("Order failed risk check: %v\n", err)
		return
	}
	
	// Place order
	placedOrder, err := mm.exchange.CreateOrder(ctx, order)
	if err != nil {
		fmt.Printf("Failed to place order: %v\n", err)
		return
	}
	
	// Track order
	mm.activeOrders[placedOrder.ExchangeOrderID] = placedOrder
	quote.OrderID = placedOrder.ExchangeOrderID
	quote.LastUpdate = time.Now()
}

// updateActiveOrders cancels orders that don't match new quotes
func (mm *MarketMaker) updateActiveOrders(ctx context.Context, newQuotes []*Quote) {
	// Build map of new quotes for quick lookup
	newQuoteMap := make(map[string]bool)
	for _, quote := range newQuotes {
		key := fmt.Sprintf("%s_%s_%s", quote.Side, quote.Price, quote.Quantity)
		newQuoteMap[key] = true
	}
	
	// Cancel orders that don't match new quotes
	for orderID, order := range mm.activeOrders {
		key := fmt.Sprintf("%s_%s_%s", order.Side, order.Price, order.Quantity)
		if !newQuoteMap[key] {
			mm.exchange.CancelOrder(ctx, orderID)
			delete(mm.activeOrders, orderID)
		}
	}
}

// findMatchingOrder finds an active order matching a quote
func (mm *MarketMaker) findMatchingOrder(quote *Quote) (*types.Order, bool) {
	for _, order := range mm.activeOrders {
		if order.Side == quote.Side &&
			order.Price.Equal(quote.Price) &&
			order.Quantity.Equal(quote.Quantity) {
			return order, true
		}
	}
	return nil, false
}

// orderWorker processes order and trade updates
func (mm *MarketMaker) orderWorker(ctx context.Context) {
	defer mm.wg.Done()
	
	for {
		select {
		case order := <-mm.orderUpdateCh:
			mm.handleOrderUpdate(order)
			
		case trade := <-mm.tradeUpdateCh:
			mm.handleTradeUpdate(trade)
			
		case <-mm.stopCh:
			return
			
		case <-ctx.Done():
			return
		}
	}
}

// handleOrderUpdate processes order status updates
func (mm *MarketMaker) handleOrderUpdate(order *types.Order) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	
	// Update order status
	if existing, exists := mm.activeOrders[order.ExchangeOrderID]; exists {
		existing.Status = order.Status
		existing.FilledQuantity = order.FilledQuantity
		existing.UpdatedAt = order.UpdatedAt
		
		// Remove if terminal state
		if order.Status == types.OrderStatusFilled ||
			order.Status == types.OrderStatusCanceled ||
			order.Status == types.OrderStatusRejected {
			delete(mm.activeOrders, order.ExchangeOrderID)
		}
	}
}

// handleTradeUpdate processes trade executions
func (mm *MarketMaker) handleTradeUpdate(trade *types.Trade) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	
	// Update inventory
	mm.inventoryMgr.UpdatePosition(trade)
	
	// Update metrics
	mm.updateMetrics(trade)
	
	// Check if we need to rebalance
	inventory := mm.inventoryMgr.(*InventoryManagerImpl).GetInventoryState()
	if shouldHedge, hedgeAmount := mm.inventoryMgr.(*InventoryManagerImpl).ShouldHedge(); shouldHedge {
		// Create hedge order (would be placed on another exchange/instrument)
		fmt.Printf("Should hedge position: %v\n", hedgeAmount)
	}
}

// metricsWorker calculates and reports metrics
func (mm *MarketMaker) metricsWorker(ctx context.Context) {
	defer mm.wg.Done()
	
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			mm.reportMetrics()
			
		case <-mm.stopCh:
			return
			
		case <-ctx.Done():
			return
		}
	}
}

// subscribeToMarketData subscribes to real-time market data
func (mm *MarketMaker) subscribeToMarketData() error {
	// Subscribe to order book
	bookCallback := func(book *types.OrderBook) {
		mm.updateMarketState(book)
	}
	
	if err := mm.exchange.SubscribeOrderBook(mm.config.Symbol, bookCallback); err != nil {
		return err
	}
	
	// Subscribe to trades
	tradeCallback := func(trade *types.Trade) {
		mm.updateTradeFlow(trade)
	}
	
	return mm.exchange.SubscribeTrades(mm.config.Symbol, tradeCallback)
}

// subscribeToOrderUpdates subscribes to order/trade updates
func (mm *MarketMaker) subscribeToOrderUpdates() error {
	// This would connect to exchange WebSocket for order updates
	// For now, we'll simulate with channels
	return nil
}

// updateMarketState updates current market conditions
func (mm *MarketMaker) updateMarketState(book *types.OrderBook) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	
	if len(book.Bids) == 0 || len(book.Asks) == 0 {
		return
	}
	
	// Calculate market metrics
	bidPrice := book.Bids[0].Price
	askPrice := book.Asks[0].Price
	midPrice := bidPrice.Add(askPrice).Div(decimal.NewFromInt(2))
	
	// Calculate order book depth
	depth := mm.calculateBookDepth(book)
	
	// Update market state
	mm.marketState = &MarketState{
		BidPrice:       bidPrice,
		AskPrice:       askPrice,
		MidPrice:       midPrice,
		OrderBookDepth: depth,
		Timestamp:      book.UpdatedAt,
	}
	
	// Update spread calculator with price
	if calc, ok := mm.spreadCalc.(*DynamicSpreadCalculator); ok {
		calc.UpdatePriceHistory(midPrice)
	}
}

// calculateBookDepth calculates total order book depth
func (mm *MarketMaker) calculateBookDepth(book *types.OrderBook) decimal.Decimal {
	depth := decimal.Zero
	
	// Sum bid depth (top 10 levels)
	for i := 0; i < len(book.Bids) && i < 10; i++ {
		depth = depth.Add(book.Bids[i].Price.Mul(book.Bids[i].Quantity))
	}
	
	// Sum ask depth (top 10 levels)
	for i := 0; i < len(book.Asks) && i < 10; i++ {
		depth = depth.Add(book.Asks[i].Price.Mul(book.Asks[i].Quantity))
	}
	
	return depth
}

// updateTradeFlow updates trade flow analysis
func (mm *MarketMaker) updateTradeFlow(trade *types.Trade) {
	// This would aggregate trades into flow analysis
	// For now, we'll skip the implementation
}

// updateMetrics updates performance metrics
func (mm *MarketMaker) updateMetrics(trade *types.Trade) {
	if trade.Side == types.OrderSideBuy {
		mm.metrics.BuyVolume = mm.metrics.BuyVolume.Add(trade.Quantity)
		mm.metrics.NumBuys++
	} else {
		mm.metrics.SellVolume = mm.metrics.SellVolume.Add(trade.Quantity)
		mm.metrics.NumSells++
	}
	
	mm.metrics.TotalVolume = mm.metrics.TotalVolume.Add(trade.Quantity)
	mm.metrics.NumTrades++
	
	// Update fees
	fee := trade.Price.Mul(trade.Quantity).Mul(trade.FeeRate)
	mm.metrics.TradingFees = mm.metrics.TradingFees.Add(fee)
}

// cancelAllOrders cancels all active orders
func (mm *MarketMaker) cancelAllOrders() error {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	
	return mm.cancelAllOrdersLocked()
}

// cancelAllOrdersLocked cancels all orders (must hold lock)
func (mm *MarketMaker) cancelAllOrdersLocked() error {
	ctx := context.Background()
	
	for orderID := range mm.activeOrders {
		if err := mm.exchange.CancelOrder(ctx, orderID); err != nil {
			fmt.Printf("Failed to cancel order %s: %v\n", orderID, err)
		}
	}
	
	// Clear active orders
	mm.activeOrders = make(map[string]*types.Order)
	
	return nil
}

// reportMetrics reports current performance metrics
func (mm *MarketMaker) reportMetrics() {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	
	inventory := mm.inventoryMgr.(*InventoryManagerImpl).GetInventoryState()
	
	fmt.Printf("\n=== Market Maker Metrics ===\n")
	fmt.Printf("Symbol: %s, Exchange: %s, Account: %s\n", 
		mm.config.Symbol, mm.config.Exchange, mm.config.Account)
	fmt.Printf("Runtime: %v\n", time.Since(mm.startTime))
	fmt.Printf("\nVolume:\n")
	fmt.Printf("  Total: %v\n", mm.metrics.TotalVolume)
	fmt.Printf("  Buy: %v (%d trades)\n", mm.metrics.BuyVolume, mm.metrics.NumBuys)
	fmt.Printf("  Sell: %v (%d trades)\n", mm.metrics.SellVolume, mm.metrics.NumSells)
	fmt.Printf("\nPnL:\n")
	fmt.Printf("  Realized: %v\n", inventory.RealizedPnL)
	fmt.Printf("  Unrealized: %v\n", inventory.UnrealizedPnL)
	fmt.Printf("  Fees: %v\n", mm.metrics.TradingFees)
	fmt.Printf("  Net: %v\n", inventory.RealizedPnL.Sub(mm.metrics.TradingFees))
	fmt.Printf("\nPosition:\n")
	fmt.Printf("  Current: %v @ %v\n", inventory.Position, inventory.AveragePrice)
	fmt.Printf("  Value: %v\n", inventory.PositionValue)
	fmt.Printf("\nActive Orders: %d\n", len(mm.activeOrders))
	fmt.Printf("==========================\n\n")
}

// finalizeMetrics calculates final performance metrics
func (mm *MarketMaker) finalizeMetrics() {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	
	mm.metrics.EndTime = time.Now()
	
	// Calculate final PnL
	inventory := mm.inventoryMgr.(*InventoryManagerImpl).GetInventoryState()
	mm.metrics.NetProfit = inventory.RealizedPnL.Sub(mm.metrics.TradingFees)
	
	// Calculate spread capture
	if mm.metrics.NumTrades > 0 {
		avgBuyPrice := decimal.Zero
		avgSellPrice := decimal.Zero
		
		if mm.metrics.NumBuys > 0 {
			avgBuyPrice = mm.metrics.BuyVolume.Div(decimal.NewFromInt(int64(mm.metrics.NumBuys)))
		}
		if mm.metrics.NumSells > 0 {
			avgSellPrice = mm.metrics.SellVolume.Div(decimal.NewFromInt(int64(mm.metrics.NumSells)))
		}
		
		if !avgBuyPrice.IsZero() && !avgSellPrice.IsZero() {
			mm.metrics.SpreadCapture = avgSellPrice.Sub(avgBuyPrice)
		}
	}
}

// GetMetrics returns current performance metrics
func (mm *MarketMaker) GetMetrics() *MMMetrics {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	
	metrics := *mm.metrics
	return &metrics
}