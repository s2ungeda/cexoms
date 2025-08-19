package backtest

import (
	"log"

	"github.com/mExOms/internal/strategies/arbitrage"
	"github.com/mExOms/internal/strategies/market_maker"
	"github.com/mExOms/pkg/types"
)

// ArbitrageStrategyAdapter adapts the arbitrage strategy for backtesting
type ArbitrageStrategyAdapter struct {
	detector     *arbitrage.Detector
	executor     *arbitrage.Executor
	config       arbitrage.Config
	opportunities []arbitrage.Opportunity
	capital      float64
	name         string
	metrics      *StrategyMetrics
}

// NewArbitrageStrategyAdapter creates a new arbitrage strategy adapter
func NewArbitrageStrategyAdapter(config arbitrage.Config) *ArbitrageStrategyAdapter {
	return &ArbitrageStrategyAdapter{
		config:  config,
		name:    "Arbitrage",
		metrics: &StrategyMetrics{Strategy: "Arbitrage"},
	}
}

// Initialize initializes the strategy
func (s *ArbitrageStrategyAdapter) Initialize(capital float64) error {
	s.capital = capital
	// In real implementation, would initialize detector and executor
	return nil
}

// OnMarketUpdate handles market data updates
func (s *ArbitrageStrategyAdapter) OnMarketUpdate(data *MarketDataPoint, state *BacktestState) []types.Order {
	// Update market data in strategy
	// Detect arbitrage opportunities
	// Generate orders for profitable opportunities
	
	var orders []types.Order
	
	// Simplified arbitrage detection
	for symbol, marketData := range state.MarketData {
		// Check if we have data from multiple exchanges
		if marketData.Exchange != data.Exchange && marketData.Symbol == data.Symbol {
			// Calculate spread
			spread := data.Ask - marketData.Bid
			spreadPct := spread / marketData.Bid
			
			// If spread exceeds threshold, create arbitrage orders
			if spreadPct > s.config.MinProfitRate {
				// Buy from cheaper exchange
				buyOrder := types.Order{
					Symbol:   symbol,
					Side:     types.SideBuy,
					Type:     types.OrderTypeMarket,
					Quantity: s.calculateOrderSize(state.Cash, marketData.Ask),
					Price:    marketData.Ask,
				}
				
				// Sell on expensive exchange
				sellOrder := types.Order{
					Symbol:   symbol,
					Side:     types.SideSell,
					Type:     types.OrderTypeMarket,
					Quantity: buyOrder.Quantity,
					Price:    data.Bid,
				}
				
				orders = append(orders, buyOrder, sellOrder)
				
				log.Printf("Arbitrage opportunity detected: %s spread %.2f%%", symbol, spreadPct*100)
			}
		}
	}
	
	return orders
}

// OnOrderFilled handles order fill events
func (s *ArbitrageStrategyAdapter) OnOrderFilled(trade *Trade) {
	// Update metrics
	s.metrics.TotalTrades++
	
	// Calculate P&L (simplified)
	var pnl float64
	if trade.Side == types.SideSell {
		pnl = trade.Quantity * (trade.Price - trade.Price*0.999) // Simplified
	}
	
	s.metrics.TotalPnL += pnl
	if pnl > 0 {
		s.metrics.WinningTrades++
	} else {
		s.metrics.LosingTrades++
	}
}

// GetName returns the strategy name
func (s *ArbitrageStrategyAdapter) GetName() string {
	return s.name
}

// GetMetrics returns strategy metrics
func (s *ArbitrageStrategyAdapter) GetMetrics() *StrategyMetrics {
	if s.metrics.TotalTrades > 0 {
		s.metrics.WinRate = float64(s.metrics.WinningTrades) / float64(s.metrics.TotalTrades)
		s.metrics.AvgPnL = s.metrics.TotalPnL / float64(s.metrics.TotalTrades)
	}
	return s.metrics
}

// calculateOrderSize calculates appropriate order size
func (s *ArbitrageStrategyAdapter) calculateOrderSize(availableCash float64, price float64) float64 {
	// Use portion of available capital
	maxOrderValue := availableCash * 0.1 // Use 10% per trade
	if maxOrderValue > s.config.MaxPositionSize {
		maxOrderValue = s.config.MaxPositionSize
	}
	
	return maxOrderValue / price
}

// MarketMakingStrategyAdapter adapts the market making strategy for backtesting
type MarketMakingStrategyAdapter struct {
	maker   market_maker.MarketMaker
	config  market_maker.Config
	capital float64
	name    string
	metrics *StrategyMetrics
	currentQuotes map[string]*Quote
}

// Quote represents a market maker quote
type Quote struct {
	BidPrice float64
	BidSize  float64
	AskPrice float64
	AskSize  float64
}

// NewMarketMakingStrategyAdapter creates a new market making strategy adapter
func NewMarketMakingStrategyAdapter(config market_maker.Config) *MarketMakingStrategyAdapter {
	return &MarketMakingStrategyAdapter{
		config:        config,
		name:          "MarketMaking",
		metrics:       &StrategyMetrics{Strategy: "MarketMaking"},
		currentQuotes: make(map[string]*Quote),
	}
}

// Initialize initializes the strategy
func (s *MarketMakingStrategyAdapter) Initialize(capital float64) error {
	s.capital = capital
	// In real implementation, would initialize market maker
	return nil
}

// OnMarketUpdate handles market data updates
func (s *MarketMakingStrategyAdapter) OnMarketUpdate(data *MarketDataPoint, state *BacktestState) []types.Order {
	var orders []types.Order
	
	// Cancel existing quotes if market moved significantly
	if quote, exists := s.currentQuotes[data.Symbol]; exists {
		midPrice := (data.Bid + data.Ask) / 2
		quoteMid := (quote.BidPrice + quote.AskPrice) / 2
		
		if math.Abs(midPrice-quoteMid)/quoteMid > 0.001 { // 0.1% threshold
			// Market moved, update quotes
			s.currentQuotes[data.Symbol] = s.generateQuote(data, state)
			
			// Generate new orders
			orders = s.createQuoteOrders(data.Symbol, s.currentQuotes[data.Symbol])
		}
	} else {
		// No existing quotes, create new ones
		s.currentQuotes[data.Symbol] = s.generateQuote(data, state)
		orders = s.createQuoteOrders(data.Symbol, s.currentQuotes[data.Symbol])
	}
	
	return orders
}

// generateQuote generates bid/ask quotes
func (s *MarketMakingStrategyAdapter) generateQuote(data *MarketDataPoint, state *BacktestState) *Quote {
	midPrice := (data.Bid + data.Ask) / 2
	
	// Calculate spread based on configuration
	spreadBps := s.config.BaseSpreadBps
	
	// Adjust for inventory
	if pos, exists := state.Positions[data.Symbol]; exists {
		inventoryRatio := pos.Quantity / s.config.MaxInventory
		// Skew quotes based on inventory
		if inventoryRatio > 0 {
			// Long inventory - make asks more aggressive
			spreadBps = spreadBps * (1 - s.config.InventorySkew*inventoryRatio)
		}
	}
	
	halfSpread := midPrice * spreadBps / 20000 // Convert bps to decimal and divide by 2
	
	return &Quote{
		BidPrice: midPrice - halfSpread,
		BidSize:  s.config.QuoteSize,
		AskPrice: midPrice + halfSpread,
		AskSize:  s.config.QuoteSize,
	}
}

// createQuoteOrders creates orders from quotes
func (s *MarketMakingStrategyAdapter) createQuoteOrders(symbol string, quote *Quote) []types.Order {
	orders := make([]types.Order, 0, 2*s.config.QuoteLevels)
	
	// Create bid orders
	for i := 0; i < s.config.QuoteLevels; i++ {
		levelSpacing := float64(i) * s.config.LevelSpacingBps / 10000
		bidPrice := quote.BidPrice * (1 - levelSpacing)
		
		orders = append(orders, types.Order{
			Symbol:   symbol,
			Side:     types.SideBuy,
			Type:     types.OrderTypeLimit,
			Price:    bidPrice,
			Quantity: quote.BidSize,
		})
	}
	
	// Create ask orders
	for i := 0; i < s.config.QuoteLevels; i++ {
		levelSpacing := float64(i) * s.config.LevelSpacingBps / 10000
		askPrice := quote.AskPrice * (1 + levelSpacing)
		
		orders = append(orders, types.Order{
			Symbol:   symbol,
			Side:     types.SideSell,
			Type:     types.OrderTypeLimit,
			Price:    askPrice,
			Quantity: quote.AskSize,
		})
	}
	
	return orders
}

// OnOrderFilled handles order fill events
func (s *MarketMakingStrategyAdapter) OnOrderFilled(trade *Trade) {
	// Update metrics
	s.metrics.TotalTrades++
	
	// For market making, calculate P&L based on spread capture
	var pnl float64
	if quote, exists := s.currentQuotes[trade.Symbol]; exists {
		midPrice := (quote.BidPrice + quote.AskPrice) / 2
		if trade.Side == types.SideBuy {
			// Bought below mid
			pnl = (midPrice - trade.Price) * trade.Quantity
		} else {
			// Sold above mid
			pnl = (trade.Price - midPrice) * trade.Quantity
		}
	}
	
	s.metrics.TotalPnL += pnl
	if pnl > 0 {
		s.metrics.WinningTrades++
	} else {
		s.metrics.LosingTrades++
	}
	
	// Update fees and slippage
	s.metrics.TotalFees += trade.Fee
	s.metrics.TotalSlippage += math.Abs(trade.Slippage) * trade.Value
}

// GetName returns the strategy name
func (s *MarketMakingStrategyAdapter) GetName() string {
	return s.name
}

// GetMetrics returns strategy metrics
func (s *MarketMakingStrategyAdapter) GetMetrics() *StrategyMetrics {
	if s.metrics.TotalTrades > 0 {
		s.metrics.WinRate = float64(s.metrics.WinningTrades) / float64(s.metrics.TotalTrades)
		s.metrics.AvgPnL = s.metrics.TotalPnL / float64(s.metrics.TotalTrades)
	}
	return s.metrics
}