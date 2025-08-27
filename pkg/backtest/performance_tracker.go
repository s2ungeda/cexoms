package backtest

import (
	"math"
	"sort"
	"time"
)

// PerformanceTracker tracks and calculates performance metrics
type PerformanceTracker struct {
	initialCapital   float64
	equityCurve      []EquityPoint
	trades           []*Trade
	drawdowns        []DrawdownPoint
	highWaterMark    float64
	lastCalculation  time.Time
	returns          []float64
}

// NewPerformanceTracker creates a new performance tracker
func NewPerformanceTracker() *PerformanceTracker {
	return &PerformanceTracker{
		equityCurve: make([]EquityPoint, 0),
		trades:      make([]*Trade, 0),
		drawdowns:   make([]DrawdownPoint, 0),
		returns:     make([]float64, 0),
	}
}

// SetInitialCapital sets the initial capital
func (pt *PerformanceTracker) SetInitialCapital(capital float64) {
	pt.initialCapital = capital
	pt.highWaterMark = capital
}

// UpdateEquity updates the equity curve
func (pt *PerformanceTracker) UpdateEquity(timestamp time.Time, equity float64) {
	// Calculate drawdown
	drawdown := 0.0
	if equity < pt.highWaterMark {
		drawdown = (pt.highWaterMark - equity) / pt.highWaterMark
	} else {
		pt.highWaterMark = equity
	}
	
	// Add to equity curve
	point := EquityPoint{
		Timestamp: timestamp,
		Equity:    equity,
		DrawDown:  drawdown,
	}
	pt.equityCurve = append(pt.equityCurve, point)
	
	// Update drawdown tracking
	pt.updateDrawdowns(timestamp, equity, drawdown)
	
	// Calculate return
	if len(pt.equityCurve) > 1 {
		prevEquity := pt.equityCurve[len(pt.equityCurve)-2].Equity
		if prevEquity > 0 {
			dailyReturn := (equity - prevEquity) / prevEquity
			pt.returns = append(pt.returns, dailyReturn)
		}
	}
}

// updateDrawdowns updates drawdown periods
func (pt *PerformanceTracker) updateDrawdowns(timestamp time.Time, equity, drawdown float64) {
	if drawdown > 0 {
		// In drawdown
		if len(pt.drawdowns) == 0 || pt.drawdowns[len(pt.drawdowns)-1].Recovered {
			// Start new drawdown
			dd := DrawdownPoint{
				StartTime:   timestamp,
				StartEquity: pt.highWaterMark,
				MinEquity:   equity,
				Drawdown:    drawdown,
			}
			pt.drawdowns = append(pt.drawdowns, dd)
		} else {
			// Update current drawdown
			currentDD := &pt.drawdowns[len(pt.drawdowns)-1]
			if equity < currentDD.MinEquity {
				currentDD.MinEquity = equity
				currentDD.Drawdown = (currentDD.StartEquity - equity) / currentDD.StartEquity
			}
		}
	} else if len(pt.drawdowns) > 0 && !pt.drawdowns[len(pt.drawdowns)-1].Recovered {
		// Recovered from drawdown
		currentDD := &pt.drawdowns[len(pt.drawdowns)-1]
		currentDD.EndTime = timestamp
		currentDD.Duration = timestamp.Sub(currentDD.StartTime)
		currentDD.Recovered = true
	}
}

// RecordTrade records a trade
func (pt *PerformanceTracker) RecordTrade(trade *Trade) {
	pt.trades = append(pt.trades, trade)
}

// GetCurrentDrawdown returns current drawdown percentage
func (pt *PerformanceTracker) GetCurrentDrawdown() float64 {
	if len(pt.equityCurve) == 0 {
		return 0
	}
	return pt.equityCurve[len(pt.equityCurve)-1].DrawDown
}

// GetLastCalculation returns the time of last metric calculation
func (pt *PerformanceTracker) GetLastCalculation() time.Time {
	return pt.lastCalculation
}

// GetEquityCurve returns the equity curve
func (pt *PerformanceTracker) GetEquityCurve() []EquityPoint {
	return pt.equityCurve
}

// GetDrawdownCurve returns drawdown periods
func (pt *PerformanceTracker) GetDrawdownCurve() []DrawdownPoint {
	return pt.drawdowns
}

// CalculateMetrics calculates all performance metrics
func (pt *PerformanceTracker) CalculateMetrics() *PerformanceMetrics {
	metrics := &PerformanceMetrics{}
	
	if len(pt.equityCurve) < 2 {
		return metrics
	}
	
	// Basic metrics
	finalEquity := pt.equityCurve[len(pt.equityCurve)-1].Equity
	metrics.TotalReturn = (finalEquity - pt.initialCapital) / pt.initialCapital
	
	// Time-based returns
	startTime := pt.equityCurve[0].Timestamp
	endTime := pt.equityCurve[len(pt.equityCurve)-1].Timestamp
	years := endTime.Sub(startTime).Hours() / 24 / 365.25
	if years > 0 {
		metrics.AnnualizedReturn = math.Pow(1+metrics.TotalReturn, 1/years) - 1
	}
	
	// Risk metrics
	if len(pt.returns) > 0 {
		metrics.Volatility = pt.calculateVolatility(pt.returns) * math.Sqrt(252) // Annualized
		
		if metrics.Volatility > 0 {
			riskFreeRate := 0.02 // 2% annual risk-free rate
			metrics.SharpeRatio = (metrics.AnnualizedReturn - riskFreeRate) / metrics.Volatility
		}
		
		// Sortino ratio (downside deviation)
		downsideVol := pt.calculateDownsideVolatility(pt.returns, 0) * math.Sqrt(252)
		if downsideVol > 0 {
			metrics.SortinoRatio = metrics.AnnualizedReturn / downsideVol
		}
		
		// VaR and CVaR
		metrics.VaR95 = pt.calculateVaR(pt.returns, 0.95)
		metrics.CVaR95 = pt.calculateCVaR(pt.returns, 0.95)
		
		// Distribution metrics
		metrics.Skewness = pt.calculateSkewness(pt.returns)
		metrics.Kurtosis = pt.calculateKurtosis(pt.returns)
	}
	
	// Drawdown metrics
	metrics.MaxDrawdown = pt.calculateMaxDrawdown()
	metrics.MaxDrawdownDays = pt.calculateMaxDrawdownDuration()
	
	if metrics.MaxDrawdown > 0 {
		metrics.CalmarRatio = metrics.AnnualizedReturn / metrics.MaxDrawdown
	}
	
	// Trade statistics
	pt.calculateTradeMetrics(metrics)
	
	pt.lastCalculation = time.Now()
	
	return metrics
}

// calculateVolatility calculates standard deviation of returns
func (pt *PerformanceTracker) calculateVolatility(returns []float64) float64 {
	if len(returns) < 2 {
		return 0
	}
	
	// Calculate mean
	sum := 0.0
	for _, r := range returns {
		sum += r
	}
	mean := sum / float64(len(returns))
	
	// Calculate variance
	variance := 0.0
	for _, r := range returns {
		variance += math.Pow(r-mean, 2)
	}
	variance /= float64(len(returns) - 1)
	
	return math.Sqrt(variance)
}

// calculateDownsideVolatility calculates volatility of returns below threshold
func (pt *PerformanceTracker) calculateDownsideVolatility(returns []float64, threshold float64) float64 {
	var downsideReturns []float64
	
	for _, r := range returns {
		if r < threshold {
			downsideReturns = append(downsideReturns, r-threshold)
		}
	}
	
	if len(downsideReturns) < 2 {
		return 0
	}
	
	// Calculate downside variance
	sum := 0.0
	for _, r := range downsideReturns {
		sum += r * r
	}
	
	return math.Sqrt(sum / float64(len(downsideReturns)))
}

// calculateVaR calculates Value at Risk
func (pt *PerformanceTracker) calculateVaR(returns []float64, confidence float64) float64 {
	if len(returns) == 0 {
		return 0
	}
	
	// Sort returns
	sorted := make([]float64, len(returns))
	copy(sorted, returns)
	sort.Float64s(sorted)
	
	// Find VaR at confidence level
	index := int((1 - confidence) * float64(len(sorted)))
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	
	return sorted[index]
}

// calculateCVaR calculates Conditional Value at Risk
func (pt *PerformanceTracker) calculateCVaR(returns []float64, confidence float64) float64 {
	var_ := pt.calculateVaR(returns, confidence)
	
	sum := 0.0
	count := 0
	
	for _, r := range returns {
		if r <= var_ {
			sum += r
			count++
		}
	}
	
	if count > 0 {
		return sum / float64(count)
	}
	
	return var_
}

// calculateSkewness calculates the skewness of returns
func (pt *PerformanceTracker) calculateSkewness(returns []float64) float64 {
	if len(returns) < 3 {
		return 0
	}
	
	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))
	
	variance := 0.0
	skewSum := 0.0
	
	for _, r := range returns {
		diff := r - mean
		variance += diff * diff
		skewSum += diff * diff * diff
	}
	
	variance /= float64(len(returns))
	stdDev := math.Sqrt(variance)
	
	if stdDev == 0 {
		return 0
	}
	
	skewness := (skewSum / float64(len(returns))) / math.Pow(stdDev, 3)
	
	return skewness
}

// calculateKurtosis calculates the kurtosis of returns
func (pt *PerformanceTracker) calculateKurtosis(returns []float64) float64 {
	if len(returns) < 4 {
		return 0
	}
	
	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))
	
	variance := 0.0
	kurtSum := 0.0
	
	for _, r := range returns {
		diff := r - mean
		variance += diff * diff
		kurtSum += diff * diff * diff * diff
	}
	
	variance /= float64(len(returns))
	
	if variance == 0 {
		return 0
	}
	
	kurtosis := (kurtSum / float64(len(returns))) / (variance * variance) - 3
	
	return kurtosis
}

// calculateMaxDrawdown calculates maximum drawdown
func (pt *PerformanceTracker) calculateMaxDrawdown() float64 {
	maxDD := 0.0
	
	for _, dd := range pt.drawdowns {
		if dd.Drawdown > maxDD {
			maxDD = dd.Drawdown
		}
	}
	
	return maxDD
}

// calculateMaxDrawdownDuration calculates max drawdown duration in days
func (pt *PerformanceTracker) calculateMaxDrawdownDuration() int {
	maxDays := 0
	
	for _, dd := range pt.drawdowns {
		days := int(dd.Duration.Hours() / 24)
		if days > maxDays {
			maxDays = days
		}
	}
	
	return maxDays
}

// calculateTradeMetrics calculates trade-based metrics
func (pt *PerformanceTracker) calculateTradeMetrics(metrics *PerformanceMetrics) {
	if len(pt.trades) == 0 {
		return
	}
	
	winningTrades := 0
	totalProfit := 0.0
	totalLoss := 0.0
	
	for _, trade := range pt.trades {
		if trade.PnL > 0 {
			winningTrades++
			totalProfit += trade.PnL
		} else if trade.PnL < 0 {
			totalLoss += math.Abs(trade.PnL)
		}
	}
	
	metrics.WinRate = float64(winningTrades) / float64(len(pt.trades))
	
	if totalLoss > 0 {
		metrics.ProfitFactor = totalProfit / totalLoss
	} else if totalProfit > 0 {
		metrics.ProfitFactor = 999.99 // Max value
	}
	
	// Expectancy ratio
	if len(pt.trades) > 0 {
		avgWin := 0.0
		avgLoss := 0.0
		
		if winningTrades > 0 {
			avgWin = totalProfit / float64(winningTrades)
		}
		
		losingTrades := len(pt.trades) - winningTrades
		if losingTrades > 0 {
			avgLoss = totalLoss / float64(losingTrades)
		}
		
		if avgLoss > 0 {
			metrics.ExpectancyRatio = (metrics.WinRate * avgWin - (1-metrics.WinRate) * avgLoss) / avgLoss
		}
	}
}