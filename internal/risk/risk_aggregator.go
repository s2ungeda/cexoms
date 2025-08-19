package risk

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// RiskAggregator provides advanced risk analytics across accounts
type RiskAggregator struct {
	mu sync.RWMutex
	
	riskManager    *MultiAccountRiskManager
	accountManager types.AccountManager
	
	// Historical data
	historicalData *HistoricalRiskData
	
	// Correlation matrix
	correlationMatrix map[string]map[string]float64
	
	// VaR calculations
	varCalculator *VaRCalculator
	
	// Stress testing
	stressTester *StressTester
	
	// Configuration
	config *AggregatorConfig
}

// AggregatorConfig contains configuration for risk aggregator
type AggregatorConfig struct {
	// Historical data settings
	HistoryRetention   time.Duration
	SampleInterval     time.Duration
	
	// Correlation settings
	CorrelationWindow  int // Number of periods
	MinCorrelation     float64
	
	// VaR settings
	VaRMethod          string  // "historical", "parametric", "monte_carlo"
	VaRConfidence      float64
	VaRHorizon         time.Duration
	
	// Stress test settings
	StressTestInterval time.Duration
}

// HistoricalRiskData stores historical risk metrics
type HistoricalRiskData struct {
	mu sync.RWMutex
	
	// Time series data per account
	accountSeries map[string]*TimeSeries
	
	// Global metrics history
	globalSeries *TimeSeries
	
	// P&L history
	pnlHistory map[string][]PnLPoint
}

// TimeSeries represents time series data
type TimeSeries struct {
	Timestamps []time.Time
	Values     []RiskSnapshot
}

// RiskSnapshot captures risk metrics at a point in time
type RiskSnapshot struct {
	Exposure      decimal.Decimal
	NetExposure   decimal.Decimal
	PnL           decimal.Decimal
	Leverage      decimal.Decimal
	OpenPositions int
}

// PnLPoint represents P&L at a specific time
type PnLPoint struct {
	Timestamp time.Time
	Value     decimal.Decimal
}

// NewRiskAggregator creates a new risk aggregator
func NewRiskAggregator(riskManager *MultiAccountRiskManager, accountManager types.AccountManager, config *AggregatorConfig) *RiskAggregator {
	if config == nil {
		config = &AggregatorConfig{
			HistoryRetention:   7 * 24 * time.Hour,
			SampleInterval:     5 * time.Minute,
			CorrelationWindow:  100,
			MinCorrelation:     0.3,
			VaRMethod:          "historical",
			VaRConfidence:      0.99,
			VaRHorizon:         24 * time.Hour,
			StressTestInterval: 1 * time.Hour,
		}
	}
	
	ra := &RiskAggregator{
		riskManager:       riskManager,
		accountManager:    accountManager,
		correlationMatrix: make(map[string]map[string]float64),
		config:            config,
	}
	
	// Initialize components
	ra.historicalData = &HistoricalRiskData{
		accountSeries: make(map[string]*TimeSeries),
		globalSeries:  &TimeSeries{},
		pnlHistory:    make(map[string][]PnLPoint),
	}
	
	ra.varCalculator = NewVaRCalculator(config.VaRMethod, config.VaRConfidence)
	ra.stressTester = NewStressTester()
	
	// Start background data collection
	go ra.collectHistoricalData()
	
	return ra
}

// CalculatePortfolioMetrics calculates comprehensive portfolio metrics
func (ra *RiskAggregator) CalculatePortfolioMetrics() *PortfolioMetrics {
	ra.mu.RLock()
	defer ra.mu.RUnlock()
	
	metrics := &PortfolioMetrics{
		Timestamp:         time.Now(),
		AccountMetrics:    make(map[string]*AccountMetrics),
		CorrelationMatrix: ra.correlationMatrix,
	}
	
	// Get current risk data
	report := ra.riskManager.GetRiskReport()
	
	// Calculate per-account metrics
	for accountID, risk := range report.AccountRisks {
		accountMetrics := &AccountMetrics{
			SharpeRatio:    ra.calculateSharpeRatio(accountID),
			MaxDrawdown:    risk.MaxDrawdown,
			VaR:            ra.calculateVaR(accountID),
			CVaR:           ra.calculateCVaR(accountID),
			Beta:           ra.calculateBeta(accountID),
			TrackingError:  ra.calculateTrackingError(accountID),
		}
		metrics.AccountMetrics[accountID] = accountMetrics
	}
	
	// Calculate portfolio-level metrics
	metrics.PortfolioVaR = ra.calculatePortfolioVaR()
	metrics.PortfolioCVaR = ra.calculatePortfolioCVaR()
	metrics.DiversificationRatio = ra.calculateDiversificationRatio()
	metrics.ConcentrationIndex = ra.calculateHerfindahlIndex()
	
	// Risk attribution
	metrics.RiskAttribution = ra.calculateRiskAttribution()
	
	return metrics
}

// CalculateCorrelations updates correlation matrix
func (ra *RiskAggregator) CalculateCorrelations() {
	ra.mu.Lock()
	defer ra.mu.Unlock()
	
	ra.historicalData.mu.RLock()
	defer ra.historicalData.mu.RUnlock()
	
	accounts := make([]string, 0)
	for accountID := range ra.historicalData.pnlHistory {
		accounts = append(accounts, accountID)
	}
	
	// Initialize correlation matrix
	for i, acc1 := range accounts {
		if _, exists := ra.correlationMatrix[acc1]; !exists {
			ra.correlationMatrix[acc1] = make(map[string]float64)
		}
		
		for j, acc2 := range accounts {
			if i == j {
				ra.correlationMatrix[acc1][acc2] = 1.0
				continue
			}
			
			// Calculate correlation
			corr := ra.calculateCorrelation(acc1, acc2)
			ra.correlationMatrix[acc1][acc2] = corr
		}
	}
}

// RunStressTests executes stress tests
func (ra *RiskAggregator) RunStressTests(ctx context.Context) *StressTestResults {
	scenarios := []StressScenario{
		{
			Name:          "Market Crash",
			MarketShock:   decimal.NewFromFloat(-0.20), // 20% drop
			VolumeShock:   decimal.NewFromFloat(2.0),   // 2x volume
			VolatilityMul: decimal.NewFromFloat(3.0),   // 3x volatility
		},
		{
			Name:          "Flash Crash",
			MarketShock:   decimal.NewFromFloat(-0.10), // 10% drop
			VolumeShock:   decimal.NewFromFloat(5.0),   // 5x volume
			VolatilityMul: decimal.NewFromFloat(5.0),   // 5x volatility
		},
		{
			Name:          "Liquidity Crisis",
			MarketShock:   decimal.NewFromFloat(-0.05),
			VolumeShock:   decimal.NewFromFloat(0.1), // 10% of normal volume
			VolatilityMul: decimal.NewFromFloat(2.0),
		},
		{
			Name:          "Correlation Break",
			MarketShock:   decimal.NewFromFloat(0),
			VolumeShock:   decimal.NewFromFloat(1.0),
			VolatilityMul: decimal.NewFromFloat(1.0),
			// Special handling for correlation changes
		},
	}
	
	return ra.stressTester.RunScenarios(ra.riskManager, scenarios)
}

// GetRiskDashboard returns comprehensive risk dashboard data
func (ra *RiskAggregator) GetRiskDashboard() *RiskDashboard {
	dashboard := &RiskDashboard{
		Timestamp: time.Now(),
	}
	
	// Current risk metrics
	report := ra.riskManager.GetRiskReport()
	dashboard.CurrentRisk = report
	
	// Portfolio metrics
	dashboard.PortfolioMetrics = ra.CalculatePortfolioMetrics()
	
	// Historical charts
	dashboard.ExposureChart = ra.getExposureTimeSeries()
	dashboard.PnLChart = ra.getPnLTimeSeries()
	dashboard.VaRChart = ra.getVaRTimeSeries()
	
	// Risk heatmap
	dashboard.RiskHeatmap = ra.generateRiskHeatmap()
	
	// Top risks
	dashboard.TopRisks = ra.identifyTopRisks()
	
	return dashboard
}

// Helper methods

// calculateSharpeRatio calculates Sharpe ratio for an account
func (ra *RiskAggregator) calculateSharpeRatio(accountID string) float64 {
	ra.historicalData.mu.RLock()
	defer ra.historicalData.mu.RUnlock()
	
	pnlHistory, exists := ra.historicalData.pnlHistory[accountID]
	if !exists || len(pnlHistory) < 2 {
		return 0
	}
	
	// Calculate returns
	returns := make([]float64, len(pnlHistory)-1)
	for i := 1; i < len(pnlHistory); i++ {
		if !pnlHistory[i-1].Value.IsZero() {
			ret := pnlHistory[i].Value.Sub(pnlHistory[i-1].Value).Div(pnlHistory[i-1].Value)
			returns[i-1] = ret.InexactFloat64()
		}
	}
	
	// Calculate mean and std dev
	mean := calculateMean(returns)
	stdDev := calculateStdDev(returns, mean)
	
	if stdDev == 0 {
		return 0
	}
	
	// Annualized Sharpe ratio (assuming daily returns)
	riskFreeRate := 0.02 / 252 // 2% annual rate
	sharpe := (mean - riskFreeRate) / stdDev * math.Sqrt(252)
	
	return sharpe
}

// calculateVaR calculates Value at Risk for an account
func (ra *RiskAggregator) calculateVaR(accountID string) decimal.Decimal {
	ra.historicalData.mu.RLock()
	defer ra.historicalData.mu.RUnlock()
	
	pnlHistory, exists := ra.historicalData.pnlHistory[accountID]
	if !exists || len(pnlHistory) < 2 {
		return decimal.Zero
	}
	
	// Calculate returns
	returns := make([]decimal.Decimal, len(pnlHistory)-1)
	for i := 1; i < len(pnlHistory); i++ {
		returns[i-1] = pnlHistory[i].Value.Sub(pnlHistory[i-1].Value)
	}
	
	// Historical VaR
	return ra.varCalculator.Calculate(returns)
}

// calculateCVaR calculates Conditional Value at Risk
func (ra *RiskAggregator) calculateCVaR(accountID string) decimal.Decimal {
	ra.historicalData.mu.RLock()
	defer ra.historicalData.mu.RUnlock()
	
	pnlHistory, exists := ra.historicalData.pnlHistory[accountID]
	if !exists || len(pnlHistory) < 2 {
		return decimal.Zero
	}
	
	// Calculate returns
	returns := make([]decimal.Decimal, len(pnlHistory)-1)
	for i := 1; i < len(pnlHistory); i++ {
		returns[i-1] = pnlHistory[i].Value.Sub(pnlHistory[i-1].Value)
	}
	
	// CVaR (Expected Shortfall)
	return ra.varCalculator.CalculateCVaR(returns)
}

// calculateBeta calculates beta relative to portfolio
func (ra *RiskAggregator) calculateBeta(accountID string) float64 {
	// Simplified beta calculation
	// In production, calculate against market benchmark
	return 1.0
}

// calculateTrackingError calculates tracking error
func (ra *RiskAggregator) calculateTrackingError(accountID string) float64 {
	// Simplified tracking error
	// In production, calculate against benchmark
	return 0.05
}

// calculatePortfolioVaR calculates portfolio-level VaR
func (ra *RiskAggregator) calculatePortfolioVaR() decimal.Decimal {
	// Aggregate VaR considering correlations
	report := ra.riskManager.GetRiskReport()
	
	totalExposure := report.GlobalRisk.TotalExposure
	if totalExposure.IsZero() {
		return decimal.Zero
	}
	
	// Simplified portfolio VaR
	// In production, use correlation matrix for proper calculation
	portfolioVolatility := decimal.NewFromFloat(0.02) // 2% daily vol
	confidenceLevel := decimal.NewFromFloat(2.33)     // 99% confidence
	
	return totalExposure.Mul(portfolioVolatility).Mul(confidenceLevel)
}

// calculatePortfolioCVaR calculates portfolio-level CVaR
func (ra *RiskAggregator) calculatePortfolioCVaR() decimal.Decimal {
	portfolioVaR := ra.calculatePortfolioVaR()
	// CVaR is typically 20-30% higher than VaR
	return portfolioVaR.Mul(decimal.NewFromFloat(1.25))
}

// calculateDiversificationRatio calculates portfolio diversification
func (ra *RiskAggregator) calculateDiversificationRatio() float64 {
	report := ra.riskManager.GetRiskReport()
	
	// Sum of individual exposures
	sumExposures := decimal.Zero
	for _, risk := range report.AccountRisks {
		sumExposures = sumExposures.Add(risk.TotalExposure)
	}
	
	if report.GlobalRisk.TotalExposure.IsZero() {
		return 0
	}
	
	// Diversification ratio = sum of exposures / portfolio exposure
	ratio := sumExposures.Div(report.GlobalRisk.TotalExposure).InexactFloat64()
	
	return ratio
}

// calculateHerfindahlIndex calculates concentration index
func (ra *RiskAggregator) calculateHerfindahlIndex() float64 {
	report := ra.riskManager.GetRiskReport()
	
	if report.GlobalRisk.TotalExposure.IsZero() {
		return 0
	}
	
	hhi := 0.0
	for _, risk := range report.AccountRisks {
		weight := risk.TotalExposure.Div(report.GlobalRisk.TotalExposure).InexactFloat64()
		hhi += weight * weight
	}
	
	return hhi
}

// calculateRiskAttribution attributes risk to different factors
func (ra *RiskAggregator) calculateRiskAttribution() map[string]float64 {
	attribution := make(map[string]float64)
	
	report := ra.riskManager.GetRiskReport()
	totalRisk := report.GlobalRisk.TotalExposure
	
	if totalRisk.IsZero() {
		return attribution
	}
	
	// Attribution by exchange
	for exchange, exposure := range report.GlobalRisk.ExposureByExchange {
		attribution["exchange_"+exchange] = exposure.Div(totalRisk).InexactFloat64()
	}
	
	// Attribution by strategy
	for strategy, exposure := range report.GlobalRisk.ExposureByStrategy {
		attribution["strategy_"+strategy] = exposure.Div(totalRisk).InexactFloat64()
	}
	
	return attribution
}

// calculateCorrelation calculates correlation between two accounts
func (ra *RiskAggregator) calculateCorrelation(acc1, acc2 string) float64 {
	pnl1 := ra.historicalData.pnlHistory[acc1]
	pnl2 := ra.historicalData.pnlHistory[acc2]
	
	if len(pnl1) < ra.config.CorrelationWindow || len(pnl2) < ra.config.CorrelationWindow {
		return 0
	}
	
	// Get recent returns
	returns1 := make([]float64, ra.config.CorrelationWindow)
	returns2 := make([]float64, ra.config.CorrelationWindow)
	
	start := len(pnl1) - ra.config.CorrelationWindow
	for i := 0; i < ra.config.CorrelationWindow-1; i++ {
		if !pnl1[start+i].Value.IsZero() {
			returns1[i] = pnl1[start+i+1].Value.Sub(pnl1[start+i].Value).Div(pnl1[start+i].Value).InexactFloat64()
		}
		if !pnl2[start+i].Value.IsZero() {
			returns2[i] = pnl2[start+i+1].Value.Sub(pnl2[start+i].Value).Div(pnl2[start+i].Value).InexactFloat64()
		}
	}
	
	return calculateCorrelation(returns1, returns2)
}

// Data visualization helpers

// getExposureTimeSeries returns exposure time series data
func (ra *RiskAggregator) getExposureTimeSeries() *TimeSeriesData {
	ra.historicalData.mu.RLock()
	defer ra.historicalData.mu.RUnlock()
	
	series := &TimeSeriesData{
		Name:   "Total Exposure",
		Points: make([]DataPoint, 0),
	}
	
	if ra.historicalData.globalSeries != nil {
		for i, ts := range ra.historicalData.globalSeries.Timestamps {
			series.Points = append(series.Points, DataPoint{
				Timestamp: ts,
				Value:     ra.historicalData.globalSeries.Values[i].Exposure.InexactFloat64(),
			})
		}
	}
	
	return series
}

// getPnLTimeSeries returns P&L time series data
func (ra *RiskAggregator) getPnLTimeSeries() *TimeSeriesData {
	series := &TimeSeriesData{
		Name:   "Portfolio P&L",
		Points: make([]DataPoint, 0),
	}
	
	// Aggregate P&L across all accounts
	// Implementation depends on data structure
	
	return series
}

// getVaRTimeSeries returns VaR time series data
func (ra *RiskAggregator) getVaRTimeSeries() *TimeSeriesData {
	series := &TimeSeriesData{
		Name:   "Portfolio VaR",
		Points: make([]DataPoint, 0),
	}
	
	// Calculate historical VaR values
	// Implementation depends on data structure
	
	return series
}

// generateRiskHeatmap generates risk heatmap data
func (ra *RiskAggregator) generateRiskHeatmap() *RiskHeatmap {
	heatmap := &RiskHeatmap{
		Rows:    make([]string, 0),
		Columns: make([]string, 0),
		Values:  make([][]float64, 0),
	}
	
	// Generate correlation heatmap
	for account := range ra.correlationMatrix {
		heatmap.Rows = append(heatmap.Rows, account)
		heatmap.Columns = append(heatmap.Columns, account)
	}
	
	sort.Strings(heatmap.Rows)
	sort.Strings(heatmap.Columns)
	
	for _, row := range heatmap.Rows {
		rowValues := make([]float64, len(heatmap.Columns))
		for j, col := range heatmap.Columns {
			if corr, exists := ra.correlationMatrix[row][col]; exists {
				rowValues[j] = corr
			}
		}
		heatmap.Values = append(heatmap.Values, rowValues)
	}
	
	return heatmap
}

// identifyTopRisks identifies top risk factors
func (ra *RiskAggregator) identifyTopRisks() []RiskFactor {
	risks := make([]RiskFactor, 0)
	
	report := ra.riskManager.GetRiskReport()
	
	// High concentration risk
	if report.GlobalRisk.ConcentrationRatio > 0.15 {
		risks = append(risks, RiskFactor{
			Name:     "High Concentration",
			Severity: "high",
			Value:    report.GlobalRisk.ConcentrationRatio,
			Message:  fmt.Sprintf("Position concentration at %.1f%%", report.GlobalRisk.ConcentrationRatio*100),
		})
	}
	
	// Multiple accounts at risk
	if report.GlobalRisk.AccountsAtRisk > 2 {
		risks = append(risks, RiskFactor{
			Name:     "Multiple Accounts at Risk",
			Severity: "medium",
			Value:    float64(report.GlobalRisk.AccountsAtRisk),
			Message:  fmt.Sprintf("%d accounts showing risk signals", report.GlobalRisk.AccountsAtRisk),
		})
	}
	
	// Sort by severity
	sort.Slice(risks, func(i, j int) bool {
		severityOrder := map[string]int{"critical": 3, "high": 2, "medium": 1, "low": 0}
		return severityOrder[risks[i].Severity] > severityOrder[risks[j].Severity]
	})
	
	return risks
}

// Background data collection

// collectHistoricalData collects historical risk data
func (ra *RiskAggregator) collectHistoricalData() {
	ticker := time.NewTicker(ra.config.SampleInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		ra.historicalData.mu.Lock()
		
		// Collect current snapshot
		report := ra.riskManager.GetRiskReport()
		
		// Update global series
		globalSnapshot := RiskSnapshot{
			Exposure:    report.GlobalRisk.TotalExposure,
			NetExposure: report.GlobalRisk.NetExposure,
			PnL:         report.GlobalRisk.DailyPnL,
		}
		
		ra.historicalData.globalSeries.Timestamps = append(ra.historicalData.globalSeries.Timestamps, time.Now())
		ra.historicalData.globalSeries.Values = append(ra.historicalData.globalSeries.Values, globalSnapshot)
		
		// Update account series
		for accountID, risk := range report.AccountRisks {
			if _, exists := ra.historicalData.accountSeries[accountID]; !exists {
				ra.historicalData.accountSeries[accountID] = &TimeSeries{}
			}
			
			snapshot := RiskSnapshot{
				Exposure:      risk.TotalExposure,
				NetExposure:   risk.NetExposure,
				PnL:           risk.DailyPnL,
				Leverage:      risk.CurrentLeverage,
				OpenPositions: risk.OpenPositions,
			}
			
			series := ra.historicalData.accountSeries[accountID]
			series.Timestamps = append(series.Timestamps, time.Now())
			series.Values = append(series.Values, snapshot)
		}
		
		// Update P&L history
		for accountID, risk := range report.AccountRisks {
			pnlPoint := PnLPoint{
				Timestamp: time.Now(),
				Value:     risk.RealizedPnL.Add(risk.UnrealizedPnL),
			}
			ra.historicalData.pnlHistory[accountID] = append(ra.historicalData.pnlHistory[accountID], pnlPoint)
		}
		
		// Clean old data
		ra.cleanOldData()
		
		ra.historicalData.mu.Unlock()
		
		// Update correlations periodically
		ra.CalculateCorrelations()
	}
}

// cleanOldData removes data older than retention period
func (ra *RiskAggregator) cleanOldData() {
	cutoff := time.Now().Add(-ra.config.HistoryRetention)
	
	// Clean global series
	ra.cleanTimeSeries(ra.historicalData.globalSeries, cutoff)
	
	// Clean account series
	for _, series := range ra.historicalData.accountSeries {
		ra.cleanTimeSeries(series, cutoff)
	}
	
	// Clean P&L history
	for accountID, history := range ra.historicalData.pnlHistory {
		cleaned := make([]PnLPoint, 0)
		for _, point := range history {
			if point.Timestamp.After(cutoff) {
				cleaned = append(cleaned, point)
			}
		}
		ra.historicalData.pnlHistory[accountID] = cleaned
	}
}

// cleanTimeSeries removes old data from time series
func (ra *RiskAggregator) cleanTimeSeries(series *TimeSeries, cutoff time.Time) {
	if series == nil || len(series.Timestamps) == 0 {
		return
	}
	
	// Find first index after cutoff
	startIdx := 0
	for i, ts := range series.Timestamps {
		if ts.After(cutoff) {
			startIdx = i
			break
		}
	}
	
	if startIdx > 0 {
		series.Timestamps = series.Timestamps[startIdx:]
		series.Values = series.Values[startIdx:]
	}
}

// Helper functions

func calculateMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func calculateStdDev(values []float64, mean float64) float64 {
	if len(values) <= 1 {
		return 0
	}
	
	sumSquares := 0.0
	for _, v := range values {
		diff := v - mean
		sumSquares += diff * diff
	}
	
	variance := sumSquares / float64(len(values)-1)
	return math.Sqrt(variance)
}

func calculateCorrelation(x, y []float64) float64 {
	if len(x) != len(y) || len(x) == 0 {
		return 0
	}
	
	meanX := calculateMean(x)
	meanY := calculateMean(y)
	
	numerator := 0.0
	denomX := 0.0
	denomY := 0.0
	
	for i := range x {
		diffX := x[i] - meanX
		diffY := y[i] - meanY
		numerator += diffX * diffY
		denomX += diffX * diffX
		denomY += diffY * diffY
	}
	
	if denomX == 0 || denomY == 0 {
		return 0
	}
	
	return numerator / math.Sqrt(denomX*denomY)
}

// Supporting types

// PortfolioMetrics contains comprehensive portfolio metrics
type PortfolioMetrics struct {
	Timestamp            time.Time
	AccountMetrics       map[string]*AccountMetrics
	PortfolioVaR         decimal.Decimal
	PortfolioCVaR        decimal.Decimal
	DiversificationRatio float64
	ConcentrationIndex   float64
	CorrelationMatrix    map[string]map[string]float64
	RiskAttribution      map[string]float64
}

// AccountMetrics contains metrics for a single account
type AccountMetrics struct {
	SharpeRatio   float64
	MaxDrawdown   decimal.Decimal
	VaR           decimal.Decimal
	CVaR          decimal.Decimal
	Beta          float64
	TrackingError float64
}

// RiskDashboard contains comprehensive risk dashboard data
type RiskDashboard struct {
	Timestamp        time.Time
	CurrentRisk      *RiskReport
	PortfolioMetrics *PortfolioMetrics
	ExposureChart    *TimeSeriesData
	PnLChart         *TimeSeriesData
	VaRChart         *TimeSeriesData
	RiskHeatmap      *RiskHeatmap
	TopRisks         []RiskFactor
}

// TimeSeriesData represents time series chart data
type TimeSeriesData struct {
	Name   string
	Points []DataPoint
}

// DataPoint represents a single data point
type DataPoint struct {
	Timestamp time.Time
	Value     float64
}

// RiskHeatmap represents heatmap data
type RiskHeatmap struct {
	Rows    []string
	Columns []string
	Values  [][]float64
}

// RiskFactor represents a risk factor
type RiskFactor struct {
	Name     string
	Severity string
	Value    float64
	Message  string
}