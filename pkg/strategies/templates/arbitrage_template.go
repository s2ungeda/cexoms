package templates

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	
	"github.com/mExOms/pkg/strategies"
	"github.com/mExOms/pkg/types"
)

// ArbitrageTemplate is a template for arbitrage strategies
type ArbitrageTemplate struct {
	*strategies.BaseStrategy
	
	// Arbitrage specific fields
	mu              sync.RWMutex
	priceFeeds      map[string]map[string]decimal.Decimal // exchange -> symbol -> price
	opportunities   map[string]*ArbitrageOpportunity
	activeArbitrages map[string]*ActiveArbitrage
	
	// Configuration
	minSpreadPercent decimal.Decimal
	minProfitUSD     decimal.Decimal
	maxLatencyMs     int64
	executionMode    string // "aggressive", "conservative", "balanced"
	
	// Channels
	priceChan       chan PriceUpdate
	opportunityChan chan *ArbitrageOpportunity
	
	wg              sync.WaitGroup
}

// ArbitrageOpportunity represents an arbitrage opportunity
type ArbitrageOpportunity struct {
	ID              string
	Symbol          string
	BuyExchange     string
	SellExchange    string
	BuyPrice        decimal.Decimal
	SellPrice       decimal.Decimal
	SpreadPercent   decimal.Decimal
	EstimatedProfit decimal.Decimal
	MaxVolume       decimal.Decimal
	Timestamp       time.Time
	ExpiresAt       time.Time
}

// ActiveArbitrage represents an active arbitrage position
type ActiveArbitrage struct {
	OpportunityID   string
	BuyOrderID      string
	SellOrderID     string
	Status          string // "pending", "partial", "filled", "failed"
	FilledBuyQty    decimal.Decimal
	FilledSellQty   decimal.Decimal
	StartTime       time.Time
}

// PriceUpdate represents a price update from an exchange
type PriceUpdate struct {
	Exchange  string
	Symbol    string
	BidPrice  decimal.Decimal
	AskPrice  decimal.Decimal
	Timestamp time.Time
}

// NewArbitrageTemplate creates a new arbitrage template
func NewArbitrageTemplate(base *strategies.BaseStrategy) *ArbitrageTemplate {
	return &ArbitrageTemplate{
		BaseStrategy:     base,
		priceFeeds:       make(map[string]map[string]decimal.Decimal),
		opportunities:    make(map[string]*ArbitrageOpportunity),
		activeArbitrages: make(map[string]*ActiveArbitrage),
		priceChan:        make(chan PriceUpdate, 1000),
		opportunityChan:  make(chan *ArbitrageOpportunity, 100),
		minSpreadPercent: decimal.NewFromFloat(0.001), // 0.1% default
		minProfitUSD:     decimal.NewFromFloat(1.0),   // $1 minimum profit
		maxLatencyMs:     100,
		executionMode:    "balanced",
	}
}

// Initialize initializes the arbitrage strategy
func (a *ArbitrageTemplate) Initialize(config strategies.StrategyConfig) error {
	// Initialize base strategy
	if err := a.BaseStrategy.Initialize(config); err != nil {
		return err
	}
	
	// Load arbitrage-specific parameters
	if spread, ok := config.Parameters["min_spread_percent"].(float64); ok {
		a.minSpreadPercent = decimal.NewFromFloat(spread)
	}
	
	if profit, ok := config.Parameters["min_profit_usd"].(float64); ok {
		a.minProfitUSD = decimal.NewFromFloat(profit)
	}
	
	if latency, ok := config.Parameters["max_latency_ms"].(float64); ok {
		a.maxLatencyMs = int64(latency)
	}
	
	if mode, ok := config.Parameters["execution_mode"].(string); ok {
		a.executionMode = mode
	}
	
	return nil
}

// Start starts the arbitrage strategy
func (a *ArbitrageTemplate) Start(ctx context.Context) error {
	if err := a.BaseStrategy.Start(ctx); err != nil {
		return err
	}
	
	// Start workers
	a.wg.Add(2)
	go a.priceAggregator()
	go a.opportunityHunter()
	
	return nil
}

// Stop stops the arbitrage strategy
func (a *ArbitrageTemplate) Stop() error {
	// Close channels
	close(a.priceChan)
	close(a.opportunityChan)
	
	// Wait for workers
	a.wg.Wait()
	
	return a.BaseStrategy.Stop()
}

// OnOrderBook handles order book updates
func (a *ArbitrageTemplate) OnOrderBook(book *types.OrderBook) {
	// Extract best bid/ask for arbitrage detection
	if len(book.Bids) > 0 && len(book.Asks) > 0 {
		update := PriceUpdate{
			Exchange:  book.Exchange,
			Symbol:    book.Symbol,
			BidPrice:  book.Bids[0].Price,
			AskPrice:  book.Asks[0].Price,
			Timestamp: time.Now(),
		}
		
		select {
		case a.priceChan <- update:
		default:
			// Channel full, drop oldest update
		}
	}
}

// OnOrderUpdate handles order updates
func (a *ArbitrageTemplate) OnOrderUpdate(order *types.Order) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Find active arbitrage
	for id, arb := range a.activeArbitrages {
		if arb.BuyOrderID == order.ID || arb.SellOrderID == order.ID {
			a.handleArbitrageOrderUpdate(id, arb, order)
			break
		}
	}
}

// priceAggregator aggregates prices from multiple exchanges
func (a *ArbitrageTemplate) priceAggregator() {
	defer a.wg.Done()
	
	for {
		select {
		case <-a.ctx.Done():
			return
		case update := <-a.priceChan:
			a.updatePriceFeed(update)
			a.detectOpportunities(update)
		}
	}
}

// updatePriceFeed updates the price feed
func (a *ArbitrageTemplate) updatePriceFeed(update PriceUpdate) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	if _, exists := a.priceFeeds[update.Exchange]; !exists {
		a.priceFeeds[update.Exchange] = make(map[string]decimal.Decimal)
	}
	
	// Store mid price for simplicity
	midPrice := update.BidPrice.Add(update.AskPrice).Div(decimal.NewFromInt(2))
	a.priceFeeds[update.Exchange][update.Symbol] = midPrice
}

// detectOpportunities detects arbitrage opportunities
func (a *ArbitrageTemplate) detectOpportunities(update PriceUpdate) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// Compare prices across exchanges for the same symbol
	for exchange, symbols := range a.priceFeeds {
		if exchange == update.Exchange {
			continue
		}
		
		if price, exists := symbols[update.Symbol]; exists {
			// Calculate spread
			buyPrice := update.AskPrice
			sellPrice := price
			
			if sellPrice.GreaterThan(buyPrice) {
				spread := sellPrice.Sub(buyPrice).Div(buyPrice)
				
				// Check if spread meets minimum threshold
				if spread.GreaterThan(a.minSpreadPercent) {
					opp := &ArbitrageOpportunity{
						ID:            fmt.Sprintf("arb-%d", time.Now().UnixNano()),
						Symbol:        update.Symbol,
						BuyExchange:   update.Exchange,
						SellExchange:  exchange,
						BuyPrice:      buyPrice,
						SellPrice:     sellPrice,
						SpreadPercent: spread,
						Timestamp:     time.Now(),
						ExpiresAt:     time.Now().Add(5 * time.Second),
					}
					
					// Calculate estimated profit (simplified)
					// In reality, need to consider fees, slippage, etc.
					opp.EstimatedProfit = a.calculateEstimatedProfit(opp)
					
					if opp.EstimatedProfit.GreaterThan(a.minProfitUSD) {
						select {
						case a.opportunityChan <- opp:
						default:
							// Channel full
						}
					}
				}
			}
		}
	}
}

// opportunityHunter executes arbitrage opportunities
func (a *ArbitrageTemplate) opportunityHunter() {
	defer a.wg.Done()
	
	for {
		select {
		case <-a.ctx.Done():
			return
		case opp := <-a.opportunityChan:
			if a.shouldExecute(opp) {
				go a.executeArbitrage(opp)
			}
		}
	}
}

// shouldExecute determines if an opportunity should be executed
func (a *ArbitrageTemplate) shouldExecute(opp *ArbitrageOpportunity) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	// Check if opportunity still valid
	if time.Now().After(opp.ExpiresAt) {
		return false
	}
	
	// Check if we already have an active arbitrage for this symbol
	for _, arb := range a.activeArbitrages {
		if arb.OpportunityID == opp.ID {
			return false
		}
	}
	
	// Check risk limits
	if err := a.CheckRiskLimits(); err != nil {
		a.Logger.Warn("Risk limits exceeded",
			zap.String("opportunity_id", opp.ID),
			zap.Error(err))
		return false
	}
	
	// Additional checks based on execution mode
	switch a.executionMode {
	case "aggressive":
		// Execute all profitable opportunities
		return true
	case "conservative":
		// Only execute high-profit opportunities
		return opp.SpreadPercent.GreaterThan(decimal.NewFromFloat(0.005))
	default: // balanced
		// Standard checks
		return opp.SpreadPercent.GreaterThan(decimal.NewFromFloat(0.002))
	}
}

// executeArbitrage executes an arbitrage opportunity
func (a *ArbitrageTemplate) executeArbitrage(opp *ArbitrageOpportunity) {
	startTime := time.Now()
	
	// Calculate optimal size
	size := a.calculateOptimalSize(opp)
	if size.IsZero() {
		return
	}
	
	// Create active arbitrage record
	arb := &ActiveArbitrage{
		OpportunityID: opp.ID,
		Status:        "pending",
		StartTime:     startTime,
	}
	
	a.mu.Lock()
	a.activeArbitrages[opp.ID] = arb
	a.mu.Unlock()
	
	// Place buy order
	buyReq := &types.OrderRequest{
		AccountID: a.getAccountForExchange(opp.BuyExchange),
		Exchange:  opp.BuyExchange,
		Symbol:    opp.Symbol,
		Side:      types.OrderSideBuy,
		Type:      types.OrderTypeLimit,
		Quantity:  size,
		Price:     opp.BuyPrice,
		TimeInForce: types.TimeInForceIOC, // Immediate or cancel
	}
	
	buyOrder, err := a.PlaceOrder(buyReq)
	if err != nil {
		a.Logger.Error("Failed to place buy order",
			zap.String("opportunity_id", opp.ID),
			zap.Error(err))
		a.markArbitrageFailed(opp.ID, "buy_order_failed")
		return
	}
	arb.BuyOrderID = buyOrder.ID
	
	// Place sell order simultaneously
	sellReq := &types.OrderRequest{
		AccountID: a.getAccountForExchange(opp.SellExchange),
		Exchange:  opp.SellExchange,
		Symbol:    opp.Symbol,
		Side:      types.OrderSideSell,
		Type:      types.OrderTypeLimit,
		Quantity:  size,
		Price:     opp.SellPrice,
		TimeInForce: types.TimeInForceIOC,
	}
	
	sellOrder, err := a.PlaceOrder(sellReq)
	if err != nil {
		a.Logger.Error("Failed to place sell order",
			zap.String("opportunity_id", opp.ID),
			zap.Error(err))
		// Cancel buy order
		a.OrderManager.CancelOrder(buyOrder.ID)
		a.markArbitrageFailed(opp.ID, "sell_order_failed")
		return
	}
	arb.SellOrderID = sellOrder.ID
	
	a.Logger.Info("Arbitrage executed",
		zap.String("opportunity_id", opp.ID),
		zap.String("symbol", opp.Symbol),
		zap.String("buy_exchange", opp.BuyExchange),
		zap.String("sell_exchange", opp.SellExchange),
		zap.String("spread", opp.SpreadPercent.String()),
		zap.Duration("execution_time", time.Since(startTime)))
}

// calculateEstimatedProfit calculates estimated profit for an opportunity
func (a *ArbitrageTemplate) calculateEstimatedProfit(opp *ArbitrageOpportunity) decimal.Decimal {
	// Simplified calculation - in production would include:
	// - Exchange fees
	// - Network fees
	// - Slippage estimates
	// - Transfer time risk
	
	// Assume 0.1% fee on each side
	fee := decimal.NewFromFloat(0.001)
	spread := opp.SellPrice.Sub(opp.BuyPrice)
	totalFees := opp.BuyPrice.Mul(fee).Add(opp.SellPrice.Mul(fee))
	
	// Assume we can trade 0.1 BTC
	size := decimal.NewFromFloat(0.1)
	grossProfit := spread.Mul(size)
	netProfit := grossProfit.Sub(totalFees.Mul(size))
	
	return netProfit
}

// calculateOptimalSize calculates optimal trade size
func (a *ArbitrageTemplate) calculateOptimalSize(opp *ArbitrageOpportunity) decimal.Decimal {
	// Get available balance on both exchanges
	// Check order book depth
	// Consider position limits
	// Return optimal size
	
	// Simplified - return fixed size
	return decimal.NewFromFloat(0.1)
}

// getAccountForExchange returns the account ID for an exchange
func (a *ArbitrageTemplate) getAccountForExchange(exchange string) string {
	// In production, would map exchanges to specific accounts
	// For now, return first available account
	if len(a.Config.Accounts) > 0 {
		return a.Config.Accounts[0]
	}
	return ""
}

// handleArbitrageOrderUpdate handles order updates for active arbitrages
func (a *ArbitrageTemplate) handleArbitrageOrderUpdate(id string, arb *ActiveArbitrage, order *types.Order) {
	if order.Status == types.OrderStatusFilled {
		if order.ID == arb.BuyOrderID {
			arb.FilledBuyQty = order.FilledQuantity
		} else if order.ID == arb.SellOrderID {
			arb.FilledSellQty = order.FilledQuantity
		}
		
		// Check if both sides filled
		if arb.FilledBuyQty.IsPositive() && arb.FilledSellQty.IsPositive() {
			arb.Status = "filled"
			a.recordArbitrageComplete(id, arb)
		}
	} else if order.Status == types.OrderStatusCancelled || order.Status == types.OrderStatusRejected {
		arb.Status = "failed"
		a.handleArbitrageFailed(id, arb)
	}
}

// recordArbitrageComplete records a completed arbitrage
func (a *ArbitrageTemplate) recordArbitrageComplete(id string, arb *ActiveArbitrage) {
	// Record trade
	// Update metrics
	// Clean up
	
	a.mu.Lock()
	delete(a.activeArbitrages, id)
	a.mu.Unlock()
	
	a.Logger.Info("Arbitrage completed",
		zap.String("opportunity_id", id),
		zap.Duration("total_time", time.Since(arb.StartTime)))
}

// handleArbitrageFailed handles failed arbitrage
func (a *ArbitrageTemplate) handleArbitrageFailed(id string, arb *ActiveArbitrage) {
	// Cancel any open orders
	// Record failure
	// Clean up
	
	a.mu.Lock()
	delete(a.activeArbitrages, id)
	a.mu.Unlock()
	
	a.Logger.Warn("Arbitrage failed",
		zap.String("opportunity_id", id),
		zap.String("status", arb.Status))
}

// markArbitrageFailed marks an arbitrage as failed
func (a *ArbitrageTemplate) markArbitrageFailed(id string, reason string) {
	a.mu.Lock()
	if arb, exists := a.activeArbitrages[id]; exists {
		arb.Status = "failed"
	}
	delete(a.activeArbitrages, id)
	a.mu.Unlock()
	
	a.Logger.Warn("Arbitrage marked as failed",
		zap.String("opportunity_id", id),
		zap.String("reason", reason))
}