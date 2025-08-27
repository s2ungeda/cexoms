package backtest

import (
	"math"
	
	"github.com/your-org/mExOms/pkg/types"
)

// LinearSlippageModel implements a linear slippage model
type LinearSlippageModel struct {
	BaseSlippage    float64 // Base slippage in percentage
	SizeMultiplier  float64 // Additional slippage per unit size
	VolatilityMult  float64 // Multiplier based on volatility
}

// NewLinearSlippageModel creates a new linear slippage model
func NewLinearSlippageModel(baseSlippage, sizeMultiplier, volatilityMult float64) *LinearSlippageModel {
	return &LinearSlippageModel{
		BaseSlippage:   baseSlippage,
		SizeMultiplier: sizeMultiplier,
		VolatilityMult: volatilityMult,
	}
}

// Calculate calculates slippage for an order
func (lsm *LinearSlippageModel) Calculate(order *Order, marketPrice float64, orderBook *SimulatedOrderBook) float64 {
	// Base slippage
	slippage := marketPrice * lsm.BaseSlippage
	
	// Size-based slippage
	marketDepth := orderBook.GetMarketDepth(order.Symbol, order.Side)
	if marketDepth > 0 {
		sizeImpact := (order.Quantity / marketDepth) * lsm.SizeMultiplier
		slippage += marketPrice * sizeImpact
	}
	
	// Volatility-based slippage
	volatility := orderBook.GetVolatility(order.Symbol)
	slippage += marketPrice * volatility * lsm.VolatilityMult
	
	// Direction based on order side
	if order.Side == types.OrderSideBuy {
		return slippage // Positive slippage for buys (pay more)
	}
	return -slippage // Negative slippage for sells (receive less)
}

// SquareRootSlippageModel implements square-root market impact
type SquareRootSlippageModel struct {
	ImpactCoefficient float64
	SpreadMultiplier  float64
}

// NewSquareRootSlippageModel creates a square-root slippage model
func NewSquareRootSlippageModel(impactCoef, spreadMult float64) *SquareRootSlippageModel {
	return &SquareRootSlippageModel{
		ImpactCoefficient: impactCoef,
		SpreadMultiplier:  spreadMult,
	}
}

// Calculate calculates slippage using square-root market impact
func (srsm *SquareRootSlippageModel) Calculate(order *Order, marketPrice float64, orderBook *SimulatedOrderBook) float64 {
	// Get spread
	spread := orderBook.GetSpread(order.Symbol)
	
	// Get average daily volume
	avgVolume := orderBook.GetAverageVolume(order.Symbol)
	if avgVolume == 0 {
		avgVolume = 1000000 // Default
	}
	
	// Square-root market impact: Impact = γ * sqrt(Q/V)
	// where γ is impact coefficient, Q is order size, V is average volume
	participation := order.Quantity / avgVolume
	impact := srsm.ImpactCoefficient * math.Sqrt(participation) * marketPrice
	
	// Add spread component
	spreadCost := spread * srsm.SpreadMultiplier / 2
	
	// Total slippage
	slippage := impact + spreadCost
	
	if order.Side == types.OrderSideBuy {
		return slippage
	}
	return -slippage
}

// OrderBookSlippageModel uses actual order book depth
type OrderBookSlippageModel struct {
	MaxLevels        int
	AdditionalImpact float64 // Impact beyond visible book
}

// NewOrderBookSlippageModel creates an order book based slippage model
func NewOrderBookSlippageModel(maxLevels int, additionalImpact float64) *OrderBookSlippageModel {
	return &OrderBookSlippageModel{
		MaxLevels:        maxLevels,
		AdditionalImpact: additionalImpact,
	}
}

// Calculate calculates slippage by walking through order book
func (obsm *OrderBookSlippageModel) Calculate(order *Order, marketPrice float64, orderBook *SimulatedOrderBook) float64 {
	// Get order book levels
	levels := orderBook.GetLevels(order.Symbol, order.Side, obsm.MaxLevels)
	
	if len(levels) == 0 {
		// No order book data, use simple model
		return marketPrice * 0.001 * obsm.getDirectionMultiplier(order.Side)
	}
	
	// Walk through order book to fill order
	remainingQty := order.Quantity
	totalCost := 0.0
	
	for _, level := range levels {
		if remainingQty <= 0 {
			break
		}
		
		// Fill from this level
		fillQty := math.Min(remainingQty, level.Quantity)
		totalCost += fillQty * level.Price
		remainingQty -= fillQty
	}
	
	// If order not fully filled from visible book, estimate remaining impact
	if remainingQty > 0 {
		lastPrice := levels[len(levels)-1].Price
		additionalCost := remainingQty * lastPrice * (1 + obsm.AdditionalImpact)
		totalCost += additionalCost
	}
	
	// Calculate average execution price
	avgPrice := totalCost / order.Quantity
	
	// Slippage is difference from market price
	return avgPrice - marketPrice
}

func (obsm *OrderBookSlippageModel) getDirectionMultiplier(side types.OrderSide) float64 {
	if side == types.OrderSideBuy {
		return 1.0
	}
	return -1.0
}

// NoSlippageModel implements zero slippage for testing
type NoSlippageModel struct{}

// Calculate always returns zero slippage
func (nsm *NoSlippageModel) Calculate(order *Order, marketPrice float64, orderBook *SimulatedOrderBook) float64 {
	return 0
}