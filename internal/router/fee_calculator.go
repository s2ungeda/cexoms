package router

import (
	"sync"

	"github.com/shopspring/decimal"
)

// FeeCalculator calculates trading fees for different exchanges
type FeeCalculator struct {
	mu            sync.RWMutex
	feeStructures map[string]FeeStructure
}

// NewFeeCalculator creates a new fee calculator
func NewFeeCalculator(feeStructures map[string]FeeStructure) *FeeCalculator {
	fc := &FeeCalculator{
		feeStructures: make(map[string]FeeStructure),
	}

	// Initialize with provided fee structures
	for exchange, fees := range feeStructures {
		fc.feeStructures[exchange] = fees
	}

	// Add default fee structures if not provided
	fc.addDefaultFeeStructures()

	return fc
}

// addDefaultFeeStructures adds default fee structures for major exchanges
func (fc *FeeCalculator) addDefaultFeeStructures() {
	defaults := map[string]FeeStructure{
		"binance": {
			Exchange:  "binance",
			MakerFee:  decimal.NewFromFloat(0.001),   // 0.1%
			TakerFee:  decimal.NewFromFloat(0.001),   // 0.1%
			TierLevel: 0,
		},
		"bybit": {
			Exchange:  "bybit",
			MakerFee:  decimal.NewFromFloat(0.001),   // 0.1%
			TakerFee:  decimal.NewFromFloat(0.001),   // 0.1%
			TierLevel: 0,
		},
		"okx": {
			Exchange:  "okx",
			MakerFee:  decimal.NewFromFloat(0.0008),  // 0.08%
			TakerFee:  decimal.NewFromFloat(0.001),   // 0.1%
			TierLevel: 0,
		},
		"upbit": {
			Exchange:  "upbit",
			MakerFee:  decimal.NewFromFloat(0.0005),  // 0.05%
			TakerFee:  decimal.NewFromFloat(0.0005),  // 0.05%
			TierLevel: 0,
		},
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()

	for exchange, fees := range defaults {
		if _, exists := fc.feeStructures[exchange]; !exists {
			fc.feeStructures[exchange] = fees
		}
	}
}

// CalculateFees calculates trading fees for an order
func (fc *FeeCalculator) CalculateFees(exchange string, quantity, price decimal.Decimal, isTaker bool) decimal.Decimal {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	fees, exists := fc.feeStructures[exchange]
	if !exists {
		// Default to 0.1% if exchange not found
		return quantity.Mul(price).Mul(decimal.NewFromFloat(0.001))
	}

	orderValue := quantity.Mul(price)
	
	if isTaker {
		return orderValue.Mul(fees.TakerFee)
	}
	return orderValue.Mul(fees.MakerFee)
}

// UpdateFeeStructure updates fee structure for an exchange
func (fc *FeeCalculator) UpdateFeeStructure(exchange string, fees FeeStructure) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.feeStructures[exchange] = fees
}

// GetFeeStructure returns fee structure for an exchange
func (fc *FeeCalculator) GetFeeStructure(exchange string) (FeeStructure, bool) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	fees, exists := fc.feeStructures[exchange]
	return fees, exists
}

// CalculateNetAmount calculates the net amount after fees
func (fc *FeeCalculator) CalculateNetAmount(exchange string, quantity, price decimal.Decimal, isTaker bool, isBuy bool) decimal.Decimal {
	fees := fc.CalculateFees(exchange, quantity, price, isTaker)
	
	if isBuy {
		// For buy orders, we pay more (price + fees)
		return quantity.Mul(price).Add(fees)
	} else {
		// For sell orders, we receive less (price - fees)
		return quantity.Mul(price).Sub(fees)
	}
}

// CalculateBreakEvenPrice calculates the break-even price for arbitrage
func (fc *FeeCalculator) CalculateBreakEvenPrice(buyExchange, sellExchange string, buyPrice decimal.Decimal) decimal.Decimal {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	buyFees, _ := fc.feeStructures[buyExchange]
	sellFees, _ := fc.feeStructures[sellExchange]

	// Break-even = buyPrice * (1 + buyFee) / (1 - sellFee)
	buyMultiplier := decimal.NewFromFloat(1).Add(buyFees.TakerFee)
	sellMultiplier := decimal.NewFromFloat(1).Sub(sellFees.TakerFee)
	
	return buyPrice.Mul(buyMultiplier).Div(sellMultiplier)
}

// CalculateMinProfitSpread calculates minimum spread needed for profitable arbitrage
func (fc *FeeCalculator) CalculateMinProfitSpread(buyExchange, sellExchange string, minProfitPercent decimal.Decimal) decimal.Decimal {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	buyFees, _ := fc.feeStructures[buyExchange]
	sellFees, _ := fc.feeStructures[sellExchange]

	// Total fees as percentage
	totalFees := buyFees.TakerFee.Add(sellFees.TakerFee)
	
	// Minimum spread = total fees + desired profit
	return totalFees.Add(minProfitPercent.Div(decimal.NewFromInt(100)))
}

// EstimateSlippage estimates slippage based on order size and liquidity
func (fc *FeeCalculator) EstimateSlippage(orderSize, availableLiquidity decimal.Decimal) decimal.Decimal {
	if availableLiquidity.IsZero() {
		return decimal.Zero
	}

	// Simple linear slippage model
	// If order is 10% of liquidity, assume 0.1% slippage
	liquidityRatio := orderSize.Div(availableLiquidity)
	slippage := liquidityRatio.Mul(decimal.NewFromFloat(0.01))

	// Cap at 1% slippage
	maxSlippage := decimal.NewFromFloat(0.01)
	if slippage.GreaterThan(maxSlippage) {
		slippage = maxSlippage
	}

	return slippage
}

// GetTotalCost calculates total cost including fees and slippage
func (fc *FeeCalculator) GetTotalCost(exchange string, quantity, price, availableLiquidity decimal.Decimal, isBuy bool) decimal.Decimal {
	// Base cost
	baseCost := quantity.Mul(price)
	
	// Fees
	fees := fc.CalculateFees(exchange, quantity, price, true) // Assume taker
	
	// Slippage
	slippage := fc.EstimateSlippage(quantity, availableLiquidity)
	slippageCost := baseCost.Mul(slippage)
	
	if isBuy {
		// For buy: pay base + fees + slippage
		return baseCost.Add(fees).Add(slippageCost)
	} else {
		// For sell: receive base - fees - slippage
		return baseCost.Sub(fees).Sub(slippageCost)
	}
}