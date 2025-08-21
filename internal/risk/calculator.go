package risk

import (
	"github.com/shopspring/decimal"
)

// PositionSizeCalculator provides various position sizing algorithms
type PositionSizeCalculator struct {
	defaultRiskPercent float64
}

// NewPositionSizeCalculator creates a new position size calculator
func NewPositionSizeCalculator(defaultRiskPercent float64) *PositionSizeCalculator {
	if defaultRiskPercent <= 0 {
		defaultRiskPercent = 2.0 // 2% default risk
	}
	return &PositionSizeCalculator{
		defaultRiskPercent: defaultRiskPercent,
	}
}

// FixedFractional calculates position size using fixed fractional method
func (c *PositionSizeCalculator) FixedFractional(balance, entryPrice, stopPrice decimal.Decimal, riskPercent float64) decimal.Decimal {
	if riskPercent <= 0 {
		riskPercent = c.defaultRiskPercent
	}
	
	// Risk amount = Balance * Risk%
	riskAmount := balance.Mul(decimal.NewFromFloat(riskPercent / 100))
	
	// Stop distance = |Entry - Stop|
	stopDistance := entryPrice.Sub(stopPrice).Abs()
	
	if stopDistance.IsZero() {
		return decimal.Zero
	}
	
	// Position size = Risk Amount / Stop Distance
	positionSize := riskAmount.Div(stopDistance)
	
	return positionSize
}

// KellyCriterion calculates position size using Kelly Criterion
func (c *PositionSizeCalculator) KellyCriterion(balance decimal.Decimal, winRate, avgWin, avgLoss float64) decimal.Decimal {
	if avgLoss == 0 {
		return decimal.Zero
	}
	
	// Kelly % = (p * b - q) / b
	// where p = win rate, q = loss rate, b = win/loss ratio
	p := winRate
	q := 1 - winRate
	b := avgWin / avgLoss
	
	kellyPercent := (p*b - q) / b
	
	// Apply Kelly fraction (usually 0.25 to be conservative)
	kellyPercent = kellyPercent * 0.25
	
	// Limit to maximum 10% of balance
	if kellyPercent > 0.10 {
		kellyPercent = 0.10
	}
	
	if kellyPercent <= 0 {
		return decimal.Zero
	}
	
	return balance.Mul(decimal.NewFromFloat(kellyPercent))
}

// VolatilityBased calculates position size based on volatility (ATR)
func (c *PositionSizeCalculator) VolatilityBased(balance, atr decimal.Decimal, riskPercent float64) decimal.Decimal {
	if riskPercent <= 0 {
		riskPercent = c.defaultRiskPercent
	}
	
	// Risk amount = Balance * Risk%
	riskAmount := balance.Mul(decimal.NewFromFloat(riskPercent / 100))
	
	// Position size = Risk Amount / (ATR * multiplier)
	// Using 2x ATR as stop distance
	stopDistance := atr.Mul(decimal.NewFromInt(2))
	
	if stopDistance.IsZero() {
		return decimal.Zero
	}
	
	return riskAmount.Div(stopDistance)
}

// OptimalF calculates position size using Optimal F method
func (c *PositionSizeCalculator) OptimalF(balance, maxLoss decimal.Decimal, optimalF float64) decimal.Decimal {
	if maxLoss.IsZero() || optimalF <= 0 {
		return decimal.Zero
	}
	
	// Position size = (Balance * Optimal F) / Max Loss
	return balance.Mul(decimal.NewFromFloat(optimalF)).Div(maxLoss)
}

// RiskParity calculates position size for equal risk contribution
func (c *PositionSizeCalculator) RiskParity(balance decimal.Decimal, numPositions int, volatility, correlation float64) decimal.Decimal {
	if numPositions <= 0 || volatility <= 0 {
		return decimal.Zero
	}
	
	// Simple risk parity: equal risk allocation
	// Advanced version would consider correlations
	targetRisk := 1.0 / float64(numPositions)
	
	// Adjust for volatility
	adjustedSize := targetRisk / volatility
	
	// Apply correlation adjustment (simplified)
	if correlation > 0 {
		adjustedSize = adjustedSize * (1 - correlation*0.5)
	}
	
	return balance.Mul(decimal.NewFromFloat(adjustedSize))
}

// MaxDrawdownBased calculates position size based on maximum drawdown limit
func (c *PositionSizeCalculator) MaxDrawdownBased(balance, maxDrawdown, currentDrawdown decimal.Decimal) decimal.Decimal {
	if maxDrawdown.IsZero() {
		maxDrawdown = decimal.NewFromFloat(0.20) // 20% default max drawdown
	}
	
	// Reduce position size as drawdown increases
	drawdownRatio := currentDrawdown.Div(maxDrawdown)
	
	// Linear reduction: 100% at 0 drawdown, 0% at max drawdown
	if drawdownRatio.GreaterThanOrEqual(decimal.NewFromInt(1)) {
		return decimal.Zero
	}
	
	multiplier := decimal.NewFromInt(1).Sub(drawdownRatio)
	
	// Base position size (e.g., 5% of balance)
	baseSize := balance.Mul(decimal.NewFromFloat(0.05))
	
	return baseSize.Mul(multiplier)
}

// DynamicPositionSizing adjusts position size based on recent performance
func (c *PositionSizeCalculator) DynamicPositionSizing(balance decimal.Decimal, recentWins, recentLosses int) decimal.Decimal {
	// Increase size after wins, decrease after losses
	totalTrades := recentWins + recentLosses
	if totalTrades == 0 {
		return balance.Mul(decimal.NewFromFloat(c.defaultRiskPercent / 100))
	}
	
	winRate := float64(recentWins) / float64(totalTrades)
	
	// Scale position size based on win rate
	// 50% win rate = normal size
	// >50% = increase size, <50% = decrease size
	scaleFactor := 0.5 + winRate
	
	// Limit scale factor between 0.5 and 1.5
	if scaleFactor < 0.5 {
		scaleFactor = 0.5
	} else if scaleFactor > 1.5 {
		scaleFactor = 1.5
	}
	
	baseSize := balance.Mul(decimal.NewFromFloat(c.defaultRiskPercent / 100))
	return baseSize.Mul(decimal.NewFromFloat(scaleFactor))
}