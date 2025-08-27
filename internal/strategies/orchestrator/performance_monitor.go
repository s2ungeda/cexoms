package orchestrator

import (
	"fmt"
	"math"
	"sync"
	"time"

	"go.uber.org/zap"
)

// PerformanceMetrics contains detailed performance metrics
type PerformanceMetrics struct {
	StrategyID    string
	Period        string // "1h", "24h", "7d", "30d", "all"
	StartTime     time.Time
	EndTime       time.Time
	
	// Basic metrics
	TotalReturn       float64
	TotalReturnPct    float64
	RealizedPnL       float64
	UnrealizedPnL     float64
	
	// Trade statistics
	TotalTrades       int
	WinningTrades     int
	LosingTrades      int
	WinRate           float64
	AvgWinAmount      float64
	AvgLossAmount     float64
	ProfitFactor      float64
	ExpectedValue     float64
	
	// Risk metrics
	Volatility        float64
	SharpeRatio       float64
	SortinoRatio      float64
	MaxDrawdown       float64
	MaxDrawdownPct    float64
	CurrentDrawdown   float64
	RecoveryTime      time.Duration
	VaR95             float64 // Value at Risk at 95% confidence
	CVaR95            float64 // Conditional VaR at 95%
	
	// Efficiency metrics
	CalmarRatio       float64 // Annual return / Max Drawdown
	OmegaRatio        float64
	UlcerIndex        float64 // Measure of downside volatility
	
	// Market correlation
	Beta              float64
	Alpha             float64
	Correlation       float64
	
	// Time-based metrics
	ActiveTime        time.Duration
	AvgHoldingPeriod  time.Duration
	Turnover          float64
	
	// Cost analysis
	TotalFees         float64
	TotalSlippage     float64
	AvgSlippagePct    float64
}

// Trade represents a completed trade for analysis
type Trade struct {
	ID            string
	StrategyID    string
	Symbol        string
	Side          string
	EntryPrice    float64
	ExitPrice     float64
	Quantity      float64
	EntryTime     time.Time
	ExitTime      time.Time
	RealizedPnL   float64
	Fees          float64
	Slippage      float64
}

// PerformanceMonitor tracks and analyzes strategy performance
type PerformanceMonitor struct {
	strategies    map[string]*StrategyPerformance
	trades        map[string][]*Trade
	benchmarks    map[string]float64
	alerts        []PerformanceAlert
	logger        *zap.Logger
	mu            sync.RWMutex
	updateTicker  *time.Ticker
}

// StrategyPerformance contains performance data for a strategy
type StrategyPerformance struct {
	StrategyID         string
	Metrics            map[string]*PerformanceMetrics // By period
	DailyReturns       []float64
	EquityCurve        []EquityPoint
	DrawdownCurve      []DrawdownPoint
	LastUpdate         time.Time
	HighWaterMark      float64
	LowWaterMark       float64
	InitialCapital     float64
	CurrentCapital     float64
}

// EquityPoint represents a point on the equity curve
type EquityPoint struct {
	Timestamp time.Time
	Value     float64
}

// DrawdownPoint represents a point on the drawdown curve
type DrawdownPoint struct {
	Timestamp time.Time
	Drawdown  float64
	Duration  time.Duration
}

// PerformanceAlert defines performance-based alerts
type PerformanceAlert struct {
	ID           string
	StrategyID   string
	Type         string // "drawdown", "loss_streak", "sharpe_decline", etc
	Severity     string // "info", "warning", "critical"
	Message      string
	Threshold    float64
	CurrentValue float64
	Timestamp    time.Time
}

// NewPerformanceMonitor creates a new performance monitor
func NewPerformanceMonitor(logger *zap.Logger) *PerformanceMonitor {
	pm := &PerformanceMonitor{
		strategies:   make(map[string]*StrategyPerformance),
		trades:      make(map[string][]*Trade),
		benchmarks:  make(map[string]float64),
		alerts:      []PerformanceAlert{},
		logger:      logger,
		updateTicker: time.NewTicker(1 * time.Minute),
	}
	
	// Start background updater
	go pm.backgroundUpdater()
	
	return pm
}

// RegisterStrategy registers a strategy for performance monitoring
func (pm *PerformanceMonitor) RegisterStrategy(strategyID string, initialCapital float64) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pm.strategies[strategyID] = &StrategyPerformance{
		StrategyID:     strategyID,
		Metrics:        make(map[string]*PerformanceMetrics),
		DailyReturns:   []float64{},
		EquityCurve:    []EquityPoint{},
		DrawdownCurve:  []DrawdownPoint{},
		LastUpdate:     time.Now(),
		HighWaterMark:  initialCapital,
		LowWaterMark:   initialCapital,
		InitialCapital: initialCapital,
		CurrentCapital: initialCapital,
	}
	
	// Initialize metrics for different periods
	periods := []string{"1h", "24h", "7d", "30d", "all"}
	for _, period := range periods {
		pm.strategies[strategyID].Metrics[period] = &PerformanceMetrics{
			StrategyID: strategyID,
			Period:     period,
			StartTime:  time.Now(),
		}
	}
}

// RecordTrade records a completed trade
func (pm *PerformanceMonitor) RecordTrade(trade *Trade) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	// Add to trades list
	pm.trades[trade.StrategyID] = append(pm.trades[trade.StrategyID], trade)
	
	// Update strategy performance
	if perf, exists := pm.strategies[trade.StrategyID]; exists {
		perf.CurrentCapital += trade.RealizedPnL - trade.Fees
		
		// Update equity curve
		perf.EquityCurve = append(perf.EquityCurve, EquityPoint{
			Timestamp: trade.ExitTime,
			Value:     perf.CurrentCapital,
		})
		
		// Update high water mark
		if perf.CurrentCapital > perf.HighWaterMark {
			perf.HighWaterMark = perf.CurrentCapital
		}
		
		// Update low water mark
		if perf.CurrentCapital < perf.LowWaterMark {
			perf.LowWaterMark = perf.CurrentCapital
		}
		
		// Calculate current drawdown
		currentDrawdown := (perf.HighWaterMark - perf.CurrentCapital) / perf.HighWaterMark
		perf.DrawdownCurve = append(perf.DrawdownCurve, DrawdownPoint{
			Timestamp: trade.ExitTime,
			Drawdown:  currentDrawdown,
			Duration:  pm.calculateDrawdownDuration(perf),
		})
		
		// Update metrics immediately
		go pm.updateStrategyMetrics(trade.StrategyID)
	}
}

// updateStrategyMetrics updates all metrics for a strategy
func (pm *PerformanceMonitor) updateStrategyMetrics(strategyID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	perf, exists := pm.strategies[strategyID]
	if !exists {
		return
	}
	
	trades := pm.trades[strategyID]
	if len(trades) == 0 {
		return
	}
	
	// Update metrics for each period
	periods := []string{"1h", "24h", "7d", "30d", "all"}
	for _, period := range periods {
		metrics := pm.calculateMetrics(perf, trades, period)
		perf.Metrics[period] = metrics
		
		// Check for alerts
		pm.checkPerformanceAlerts(strategyID, metrics)
	}
	
	perf.LastUpdate = time.Now()
}

// calculateMetrics calculates performance metrics for a period
func (pm *PerformanceMonitor) calculateMetrics(perf *StrategyPerformance, trades []*Trade, period string) *PerformanceMetrics {
	metrics := &PerformanceMetrics{
		StrategyID: perf.StrategyID,
		Period:     period,
		EndTime:    time.Now(),
	}
	
	// Filter trades by period
	startTime := pm.getPeriodStartTime(period)
	metrics.StartTime = startTime
	
	periodTrades := pm.filterTradesByPeriod(trades, startTime)
	if len(periodTrades) == 0 {
		return metrics
	}
	
	// Basic metrics
	metrics.TotalTrades = len(periodTrades)
	
	var totalPnL, totalFees, totalSlippage float64
	var winAmount, lossAmount float64
	var holdingPeriods []time.Duration
	
	for _, trade := range periodTrades {
		totalPnL += trade.RealizedPnL
		totalFees += trade.Fees
		totalSlippage += trade.Slippage
		
		holdingPeriods = append(holdingPeriods, trade.ExitTime.Sub(trade.EntryTime))
		
		if trade.RealizedPnL > 0 {
			metrics.WinningTrades++
			winAmount += trade.RealizedPnL
		} else {
			metrics.LosingTrades++
			lossAmount += math.Abs(trade.RealizedPnL)
		}
	}
	
	metrics.RealizedPnL = totalPnL
	metrics.TotalReturn = totalPnL - totalFees
	metrics.TotalReturnPct = (metrics.TotalReturn / perf.InitialCapital) * 100
	
	// Win rate and profit factor
	if metrics.TotalTrades > 0 {
		metrics.WinRate = float64(metrics.WinningTrades) / float64(metrics.TotalTrades)
		
		if metrics.WinningTrades > 0 {
			metrics.AvgWinAmount = winAmount / float64(metrics.WinningTrades)
		}
		
		if metrics.LosingTrades > 0 {
			metrics.AvgLossAmount = lossAmount / float64(metrics.LosingTrades)
			metrics.ProfitFactor = winAmount / lossAmount
		}
		
		// Expected value
		metrics.ExpectedValue = (metrics.WinRate * metrics.AvgWinAmount) - 
			((1 - metrics.WinRate) * metrics.AvgLossAmount)
		
		// Average holding period
		totalHolding := time.Duration(0)
		for _, duration := range holdingPeriods {
			totalHolding += duration
		}
		metrics.AvgHoldingPeriod = totalHolding / time.Duration(len(holdingPeriods))
	}
	
	// Risk metrics
	pm.calculateRiskMetrics(metrics, periodTrades, perf)
	
	// Cost analysis
	metrics.TotalFees = totalFees
	metrics.TotalSlippage = totalSlippage
	if metrics.TotalTrades > 0 {
		avgTradeValue := pm.calculateAvgTradeValue(periodTrades)
		if avgTradeValue > 0 {
			metrics.AvgSlippagePct = (totalSlippage / float64(metrics.TotalTrades)) / avgTradeValue * 100
		}
	}
	
	return metrics
}

// calculateRiskMetrics calculates risk-related metrics
func (pm *PerformanceMonitor) calculateRiskMetrics(metrics *PerformanceMetrics, trades []*Trade, perf *StrategyPerformance) {
	if len(trades) == 0 {
		return
	}
	
	// Calculate returns series
	returns := pm.calculateReturns(trades, perf.InitialCapital)
	if len(returns) == 0 {
		return
	}
	
	// Volatility (annualized)
	metrics.Volatility = pm.calculateVolatility(returns) * math.Sqrt(252) // Annualized
	
	// Sharpe Ratio
	if metrics.Volatility > 0 {
		riskFreeRate := 0.02 // 2% annual risk-free rate
		annualReturn := metrics.TotalReturnPct * (252 / float64(len(returns)))
		metrics.SharpeRatio = (annualReturn - riskFreeRate) / metrics.Volatility
	}
	
	// Sortino Ratio (downside deviation)
	downsideVol := pm.calculateDownsideVolatility(returns, 0) * math.Sqrt(252)
	if downsideVol > 0 {
		annualReturn := metrics.TotalReturnPct * (252 / float64(len(returns)))
		metrics.SortinoRatio = annualReturn / downsideVol
	}
	
	// Max Drawdown
	pm.calculateDrawdownMetrics(metrics, perf)
	
	// VaR and CVaR
	metrics.VaR95 = pm.calculateVaR(returns, 0.95)
	metrics.CVaR95 = pm.calculateCVaR(returns, 0.95)
	
	// Calmar Ratio
	if metrics.MaxDrawdownPct > 0 {
		annualReturn := metrics.TotalReturnPct * (252 / float64(len(returns)))
		metrics.CalmarRatio = annualReturn / metrics.MaxDrawdownPct
	}
}

// calculateVolatility calculates the standard deviation of returns
func (pm *PerformanceMonitor) calculateVolatility(returns []float64) float64 {
	if len(returns) < 2 {
		return 0
	}
	
	// Calculate mean
	var sum float64
	for _, r := range returns {
		sum += r
	}
	mean := sum / float64(len(returns))
	
	// Calculate variance
	var variance float64
	for _, r := range returns {
		variance += math.Pow(r-mean, 2)
	}
	variance /= float64(len(returns) - 1)
	
	return math.Sqrt(variance)
}

// calculateDownsideVolatility calculates volatility of negative returns
func (pm *PerformanceMonitor) calculateDownsideVolatility(returns []float64, threshold float64) float64 {
	var downsideReturns []float64
	for _, r := range returns {
		if r < threshold {
			downsideReturns = append(downsideReturns, r)
		}
	}
	
	if len(downsideReturns) < 2 {
		return 0
	}
	
	return pm.calculateVolatility(downsideReturns)
}

// calculateReturns calculates returns from trades
func (pm *PerformanceMonitor) calculateReturns(trades []*Trade, initialCapital float64) []float64 {
	var returns []float64
	capital := initialCapital
	
	for _, trade := range trades {
		if capital > 0 {
			returnPct := (trade.RealizedPnL - trade.Fees) / capital
			returns = append(returns, returnPct)
			capital += trade.RealizedPnL - trade.Fees
		}
	}
	
	return returns
}

// calculateDrawdownMetrics calculates drawdown-related metrics
func (pm *PerformanceMonitor) calculateDrawdownMetrics(metrics *PerformanceMetrics, perf *StrategyPerformance) {
	if len(perf.DrawdownCurve) == 0 {
		return
	}
	
	var maxDD float64
	var maxDDDuration time.Duration
	var currentDDStart time.Time
	inDrawdown := false
	
	for _, point := range perf.DrawdownCurve {
		if point.Drawdown > maxDD {
			maxDD = point.Drawdown
		}
		
		if point.Drawdown > 0 && !inDrawdown {
			inDrawdown = true
			currentDDStart = point.Timestamp
		} else if point.Drawdown == 0 && inDrawdown {
			inDrawdown = false
			duration := point.Timestamp.Sub(currentDDStart)
			if duration > maxDDDuration {
				maxDDDuration = duration
			}
		}
	}
	
	metrics.MaxDrawdown = maxDD * perf.InitialCapital
	metrics.MaxDrawdownPct = maxDD * 100
	metrics.RecoveryTime = maxDDDuration
	
	// Current drawdown
	if len(perf.DrawdownCurve) > 0 {
		lastPoint := perf.DrawdownCurve[len(perf.DrawdownCurve)-1]
		metrics.CurrentDrawdown = lastPoint.Drawdown
	}
}

// calculateVaR calculates Value at Risk
func (pm *PerformanceMonitor) calculateVaR(returns []float64, confidence float64) float64 {
	if len(returns) == 0 {
		return 0
	}
	
	// Sort returns
	sorted := make([]float64, len(returns))
	copy(sorted, returns)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	
	// Find VaR at confidence level
	index := int((1 - confidence) * float64(len(sorted)))
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	
	return sorted[index]
}

// calculateCVaR calculates Conditional Value at Risk
func (pm *PerformanceMonitor) calculateCVaR(returns []float64, confidence float64) float64 {
	var_ := pm.calculateVaR(returns, confidence)
	
	var sum float64
	var count int
	
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

// calculateAvgTradeValue calculates average trade value
func (pm *PerformanceMonitor) calculateAvgTradeValue(trades []*Trade) float64 {
	if len(trades) == 0 {
		return 0
	}
	
	var totalValue float64
	for _, trade := range trades {
		totalValue += trade.EntryPrice * trade.Quantity
	}
	
	return totalValue / float64(len(trades))
}

// calculateDrawdownDuration calculates current drawdown duration
func (pm *PerformanceMonitor) calculateDrawdownDuration(perf *StrategyPerformance) time.Duration {
	if len(perf.DrawdownCurve) < 2 {
		return 0
	}
	
	// Find when current drawdown started
	for i := len(perf.DrawdownCurve) - 1; i >= 1; i-- {
		if perf.DrawdownCurve[i].Drawdown > 0 && perf.DrawdownCurve[i-1].Drawdown == 0 {
			return time.Since(perf.DrawdownCurve[i].Timestamp)
		}
	}
	
	// If in drawdown since beginning
	if perf.DrawdownCurve[0].Drawdown > 0 {
		return time.Since(perf.DrawdownCurve[0].Timestamp)
	}
	
	return 0
}

// checkPerformanceAlerts checks for performance-based alerts
func (pm *PerformanceMonitor) checkPerformanceAlerts(strategyID string, metrics *PerformanceMetrics) {
	// Drawdown alert
	if metrics.CurrentDrawdown > 0.1 { // 10% drawdown
		pm.createAlert(strategyID, "drawdown", "warning", 
			fmt.Sprintf("Current drawdown: %.2f%%", metrics.CurrentDrawdown*100),
			0.1, metrics.CurrentDrawdown)
	}
	
	if metrics.CurrentDrawdown > 0.2 { // 20% drawdown
		pm.createAlert(strategyID, "drawdown", "critical",
			fmt.Sprintf("Critical drawdown: %.2f%%", metrics.CurrentDrawdown*100),
			0.2, metrics.CurrentDrawdown)
	}
	
	// Sharpe ratio decline
	if metrics.SharpeRatio < 0.5 && metrics.TotalTrades > 20 {
		pm.createAlert(strategyID, "sharpe_decline", "warning",
			fmt.Sprintf("Low Sharpe ratio: %.2f", metrics.SharpeRatio),
			0.5, metrics.SharpeRatio)
	}
	
	// Losing streak
	consecutiveLosses := pm.countConsecutiveLosses(strategyID)
	if consecutiveLosses >= 5 {
		pm.createAlert(strategyID, "loss_streak", "warning",
			fmt.Sprintf("Consecutive losses: %d", consecutiveLosses),
			5, float64(consecutiveLosses))
	}
}

// createAlert creates a new performance alert
func (pm *PerformanceMonitor) createAlert(strategyID, alertType, severity, message string, threshold, currentValue float64) {
	alert := PerformanceAlert{
		ID:           fmt.Sprintf("%s_%s_%d", strategyID, alertType, time.Now().Unix()),
		StrategyID:   strategyID,
		Type:         alertType,
		Severity:     severity,
		Message:      message,
		Threshold:    threshold,
		CurrentValue: currentValue,
		Timestamp:    time.Now(),
	}
	
	pm.alerts = append(pm.alerts, alert)
	
	// Log alert
	switch severity {
	case "critical":
		pm.logger.Error("Performance alert", 
			zap.String("strategy_id", strategyID),
			zap.String("type", alertType),
			zap.String("message", message))
	case "warning":
		pm.logger.Warn("Performance alert",
			zap.String("strategy_id", strategyID),
			zap.String("type", alertType),
			zap.String("message", message))
	default:
		pm.logger.Info("Performance alert",
			zap.String("strategy_id", strategyID),
			zap.String("type", alertType),
			zap.String("message", message))
	}
}

// countConsecutiveLosses counts consecutive losing trades
func (pm *PerformanceMonitor) countConsecutiveLosses(strategyID string) int {
	trades := pm.trades[strategyID]
	if len(trades) == 0 {
		return 0
	}
	
	count := 0
	for i := len(trades) - 1; i >= 0; i-- {
		if trades[i].RealizedPnL < 0 {
			count++
		} else {
			break
		}
	}
	
	return count
}

// filterTradesByPeriod filters trades within a time period
func (pm *PerformanceMonitor) filterTradesByPeriod(trades []*Trade, startTime time.Time) []*Trade {
	var filtered []*Trade
	
	for _, trade := range trades {
		if trade.ExitTime.After(startTime) {
			filtered = append(filtered, trade)
		}
	}
	
	return filtered
}

// getPeriodStartTime gets the start time for a period
func (pm *PerformanceMonitor) getPeriodStartTime(period string) time.Time {
	now := time.Now()
	
	switch period {
	case "1h":
		return now.Add(-1 * time.Hour)
	case "24h":
		return now.Add(-24 * time.Hour)
	case "7d":
		return now.Add(-7 * 24 * time.Hour)
	case "30d":
		return now.Add(-30 * 24 * time.Hour)
	case "all":
		return time.Time{} // Beginning of time
	default:
		return now.Add(-24 * time.Hour)
	}
}

// backgroundUpdater periodically updates metrics
func (pm *PerformanceMonitor) backgroundUpdater() {
	for range pm.updateTicker.C {
		pm.mu.RLock()
		strategies := make([]string, 0, len(pm.strategies))
		for id := range pm.strategies {
			strategies = append(strategies, id)
		}
		pm.mu.RUnlock()
		
		// Update each strategy
		for _, strategyID := range strategies {
			pm.updateStrategyMetrics(strategyID)
		}
	}
}

// GetMetrics returns performance metrics for a strategy
func (pm *PerformanceMonitor) GetMetrics(strategyID, period string) (*PerformanceMetrics, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	perf, exists := pm.strategies[strategyID]
	if !exists {
		return nil, fmt.Errorf("strategy %s not found", strategyID)
	}
	
	metrics, exists := perf.Metrics[period]
	if !exists {
		return nil, fmt.Errorf("metrics for period %s not found", period)
	}
	
	return metrics, nil
}

// GetAllMetrics returns all metrics for a strategy
func (pm *PerformanceMonitor) GetAllMetrics(strategyID string) (map[string]*PerformanceMetrics, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	perf, exists := pm.strategies[strategyID]
	if !exists {
		return nil, fmt.Errorf("strategy %s not found", strategyID)
	}
	
	return perf.Metrics, nil
}

// GetEquityCurve returns the equity curve for a strategy
func (pm *PerformanceMonitor) GetEquityCurve(strategyID string) ([]EquityPoint, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	perf, exists := pm.strategies[strategyID]
	if !exists {
		return nil, fmt.Errorf("strategy %s not found", strategyID)
	}
	
	return perf.EquityCurve, nil
}

// GetAlerts returns recent alerts
func (pm *PerformanceMonitor) GetAlerts(strategyID string, since time.Time) []PerformanceAlert {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	var alerts []PerformanceAlert
	for _, alert := range pm.alerts {
		if (strategyID == "" || alert.StrategyID == strategyID) && alert.Timestamp.After(since) {
			alerts = append(alerts, alert)
		}
	}
	
	return alerts
}

// SetBenchmark sets a benchmark for comparison
func (pm *PerformanceMonitor) SetBenchmark(name string, value float64) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pm.benchmarks[name] = value
}

// CompareToBenchmark compares strategy performance to benchmark
func (pm *PerformanceMonitor) CompareToBenchmark(strategyID, benchmarkName, period string) (float64, error) {
	metrics, err := pm.GetMetrics(strategyID, period)
	if err != nil {
		return 0, err
	}
	
	pm.mu.RLock()
	benchmarkReturn, exists := pm.benchmarks[benchmarkName]
	pm.mu.RUnlock()
	
	if !exists {
		return 0, fmt.Errorf("benchmark %s not found", benchmarkName)
	}
	
	return metrics.TotalReturnPct - benchmarkReturn, nil
}

// Shutdown stops the performance monitor
func (pm *PerformanceMonitor) Shutdown() {
	pm.updateTicker.Stop()
	pm.logger.Info("Performance monitor shutdown")
}