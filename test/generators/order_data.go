package generators

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// OrderGenerator generates realistic order data for testing
type OrderGenerator struct {
	rand      *rand.Rand
	orderSeq  int
	marketGen *MarketDataGenerator
}

// NewOrderGenerator creates a new order generator
func NewOrderGenerator(seed int64) *OrderGenerator {
	return &OrderGenerator{
		rand:      rand.New(rand.NewSource(seed)),
		orderSeq:  1000,
		marketGen: NewMarketDataGenerator(seed),
	}
}

// GenerateMarketOrder generates a market order
func (g *OrderGenerator) GenerateMarketOrder(symbol string, side types.OrderSide) *types.Order {
	g.orderSeq++
	
	// Market orders typically have round quantities
	quantities := []float64{0.001, 0.01, 0.1, 0.5, 1.0, 2.0, 5.0, 10.0}
	quantity := quantities[g.rand.Intn(len(quantities))]
	
	return &types.Order{
		OrderID:       fmt.Sprintf("MKT_%d", g.orderSeq),
		ClientOrderID: fmt.Sprintf("CLIENT_MKT_%d", g.orderSeq),
		Symbol:        symbol,
		Side:          side,
		Type:          types.OrderTypeMarket,
		Quantity:      decimal.NewFromFloat(quantity),
		Status:        types.OrderStatusNew,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

// GenerateLimitOrder generates a limit order
func (g *OrderGenerator) GenerateLimitOrder(symbol string, side types.OrderSide, priceOffset float64) *types.Order {
	g.orderSeq++
	
	// Get current market price
	ticker := g.marketGen.GenerateTicker(symbol)
	basePrice := ticker.Price.InexactFloat64()
	
	// Calculate limit price based on side and offset
	var price float64
	if side == types.OrderSideBuy {
		// Buy orders typically below market
		price = basePrice * (1 - priceOffset)
	} else {
		// Sell orders typically above market
		price = basePrice * (1 + priceOffset)
	}
	
	// Generate quantity with preference for round numbers
	quantity := g.generateOrderQuantity()
	
	return &types.Order{
		OrderID:       fmt.Sprintf("LMT_%d", g.orderSeq),
		ClientOrderID: fmt.Sprintf("CLIENT_LMT_%d", g.orderSeq),
		Symbol:        symbol,
		Side:          side,
		Type:          types.OrderTypeLimit,
		Price:         decimal.NewFromFloat(price),
		Quantity:      decimal.NewFromFloat(quantity),
		Status:        types.OrderStatusNew,
		TimeInForce:   types.TimeInForceGTC,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

// GenerateStopLossOrder generates a stop loss order
func (g *OrderGenerator) GenerateStopLossOrder(symbol string, position *types.Position, stopPercent float64) *types.Order {
	g.orderSeq++
	
	entryPrice := position.EntryPrice.InexactFloat64()
	
	var stopPrice float64
	if position.Side == types.Side("LONG") {
		// For long positions, stop below entry
		stopPrice = entryPrice * (1 - stopPercent)
	} else {
		// For short positions, stop above entry
		stopPrice = entryPrice * (1 + stopPercent)
	}
	
	return &types.Order{
		OrderID:       fmt.Sprintf("STP_%d", g.orderSeq),
		ClientOrderID: fmt.Sprintf("CLIENT_STP_%d", g.orderSeq),
		Symbol:        symbol,
		Side:          g.getOpposingSide(position.Side),
		Type:          types.OrderTypeStopLoss,
		StopPrice:     decimal.NewFromFloat(stopPrice),
		Quantity:      position.Amount,
		Status:        types.OrderStatusNew,
		TimeInForce:   types.TimeInForceGTC,
		ReduceOnly:    true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

// GenerateTakeProfitOrder generates a take profit order
func (g *OrderGenerator) GenerateTakeProfitOrder(symbol string, position *types.Position, profitPercent float64) *types.Order {
	g.orderSeq++
	
	entryPrice := position.EntryPrice.InexactFloat64()
	
	var limitPrice float64
	if position.Side == types.Side("LONG") {
		// For long positions, take profit above entry
		limitPrice = entryPrice * (1 + profitPercent)
	} else {
		// For short positions, take profit below entry
		limitPrice = entryPrice * (1 - profitPercent)
	}
	
	return &types.Order{
		OrderID:       fmt.Sprintf("TP_%d", g.orderSeq),
		ClientOrderID: fmt.Sprintf("CLIENT_TP_%d", g.orderSeq),
		Symbol:        symbol,
		Side:          g.getOpposingSide(position.Side),
		Type:          types.OrderTypeTakeProfit,
		Price:         decimal.NewFromFloat(limitPrice),
		Quantity:      position.Amount,
		Status:        types.OrderStatusNew,
		TimeInForce:   types.TimeInForceGTC,
		ReduceOnly:    true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

// GenerateOrderBatch generates a batch of mixed orders
func (g *OrderGenerator) GenerateOrderBatch(symbol string, count int) []*types.Order {
	orders := make([]*types.Order, count)
	
	for i := 0; i < count; i++ {
		// Mix of order types
		orderType := g.rand.Float64()
		
		var order *types.Order
		if orderType < 0.3 {
			// 30% market orders
			side := g.randomSide()
			order = g.GenerateMarketOrder(symbol, side)
		} else if orderType < 0.8 {
			// 50% limit orders
			side := g.randomSide()
			offset := 0.001 + g.rand.Float64()*0.01 // 0.1% to 1.1% offset
			order = g.GenerateLimitOrder(symbol, side, offset)
		} else {
			// 20% stop orders (need a position first)
			position := g.generatePosition(symbol)
			if g.rand.Float64() < 0.5 {
				stopPercent := 0.01 + g.rand.Float64()*0.04 // 1% to 5% stop
				order = g.GenerateStopLossOrder(symbol, position, stopPercent)
			} else {
				profitPercent := 0.02 + g.rand.Float64()*0.08 // 2% to 10% profit
				order = g.GenerateTakeProfitOrder(symbol, position, profitPercent)
			}
		}
		
		orders[i] = order
	}
	
	return orders
}

// GenerateOrderResponse generates a response for an order
func (g *OrderGenerator) GenerateOrderResponse(order *types.Order, success bool) *types.OrderResponse {
	if !success {
		return &types.OrderResponse{
			OrderID: "",
			Status:  types.OrderStatusRejected,
			Message: "Insufficient balance",
		}
	}
	
	response := &types.OrderResponse{
		OrderID:        order.OrderID,
		ClientOrderID:  order.ClientOrderID,
		Status:         types.OrderStatusNew,
		FilledQuantity: decimal.Zero,
		FilledPrice:    decimal.Zero,
		CreatedAt:      time.Now(),
	}
	
	// Simulate partial or full fill for market orders
	if order.Type == types.OrderTypeMarket {
		fillPercent := 0.5 + g.rand.Float64()*0.5 // 50% to 100% fill
		response.Status = types.OrderStatusPartiallyFilled
		response.FilledQuantity = order.Quantity.Mul(decimal.NewFromFloat(fillPercent))
		
		// Use current market price for fill
		ticker := g.marketGen.GenerateTicker(order.Symbol)
		response.FilledPrice = ticker.Price
		
		if fillPercent >= 0.99 {
			response.Status = types.OrderStatusFilled
			response.FilledQuantity = order.Quantity
		}
	}
	
	return response
}

// Helper methods

func (g *OrderGenerator) generateOrderQuantity() float64 {
	// 70% chance of round number
	if g.rand.Float64() < 0.7 {
		roundQtys := []float64{0.001, 0.01, 0.1, 0.25, 0.5, 1.0, 2.0, 5.0, 10.0}
		return roundQtys[g.rand.Intn(len(roundQtys))]
	}
	
	// 30% chance of random quantity
	return 0.001 + g.rand.Float64()*9.999
}

func (g *OrderGenerator) randomSide() types.OrderSide {
	if g.rand.Float64() < 0.5 {
		return types.OrderSideBuy
	}
	return types.OrderSideSell
}

func (g *OrderGenerator) getOpposingSide(side types.Side) types.OrderSide {
	if side == types.Side("LONG") {
		return types.OrderSideSell
	}
	return types.OrderSideBuy
}

func (g *OrderGenerator) generatePosition(symbol string) *types.Position {
	ticker := g.marketGen.GenerateTicker(symbol)
	quantity := g.generateOrderQuantity()
	
	side := types.Side("LONG")
	if g.rand.Float64() > 0.5 {
		side = types.Side("SHORT")
	}
	
	return &types.Position{
		Symbol:     symbol,
		Side:       side,
		Amount:     decimal.NewFromFloat(quantity),
		EntryPrice: ticker.Price,
		MarkPrice:  ticker.Price,
	}
}