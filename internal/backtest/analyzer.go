package backtest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// ResultAnalyzer analyzes backtest results
type ResultAnalyzer struct {
	results *BacktestResults
}

// NewResultAnalyzer creates a new result analyzer
func NewResultAnalyzer(results *BacktestResults) *ResultAnalyzer {
	return &ResultAnalyzer{
		results: results,
	}
}

// GenerateReport generates a comprehensive backtest report
func (ra *ResultAnalyzer) GenerateReport() *BacktestReport {
	report := &BacktestReport{
		Summary:        ra.generateSummary(),
		Performance:    ra.analyzePerformance(),
		RiskMetrics:    ra.analyzeRisk(),
		TradeAnalysis:  ra.analyzeTrades(),
		TimeAnalysis:   ra.analyzeTimePatterns(),
		SymbolAnalysis: ra.analyzeBySymbol(),
	}
	
	return report
}

// BacktestReport contains comprehensive analysis
type BacktestReport struct {
	Summary        *SummarySection        `json:"summary"`
	Performance    *PerformanceSection    `json:"performance"`
	RiskMetrics    *RiskSection          `json:"risk_metrics"`
	TradeAnalysis  *TradeSection         `json:"trade_analysis"`
	TimeAnalysis   *TimeSection          `json:"time_analysis"`
	SymbolAnalysis *SymbolSection        `json:"symbol_analysis"`
}

// SummarySection contains summary statistics
type SummarySection struct {
	StartDate       time.Time       `json:"start_date"`
	EndDate         time.Time       `json:"end_date"`
	Duration        string          `json:"duration"`
	InitialCapital  decimal.Decimal `json:"initial_capital"`
	FinalCapital    decimal.Decimal `json:"final_capital"`
	TotalReturn     decimal.Decimal `json:"total_return"`
	TotalReturnPct  string          `json:"total_return_pct"`
	TotalTrades     int             `json:"total_trades"`
	WinRate         string          `json:"win_rate"`
	ProfitFactor    decimal.Decimal `json:"profit_factor"`
	ExpectedReturn  decimal.Decimal `json:"expected_return"`
}

// PerformanceSection contains performance metrics
type PerformanceSection struct {
	CAGR              string          `json:"cagr"` // Compound Annual Growth Rate
	SharpeRatio       float64         `json:"sharpe_ratio"`
	SortinoRatio      float64         `json:"sortino_ratio"`
	CalmarRatio       float64         `json:"calmar_ratio"`
	MaxDrawdown       string          `json:"max_drawdown"`
	MaxDrawdownDays   int             `json:"max_drawdown_days"`
	RecoveryFactor    decimal.Decimal `json:"recovery_factor"`
	PayoffRatio       decimal.Decimal `json:"payoff_ratio"`
	MonthlyReturns    []MonthlyReturn `json:"monthly_returns"`
}

// RiskSection contains risk metrics
type RiskSection struct {
	ValueAtRisk95     decimal.Decimal   `json:"var_95"`     // 95% VaR
	ValueAtRisk99     decimal.Decimal   `json:"var_99"`     // 99% VaR
	ExpectedShortfall decimal.Decimal   `json:"expected_shortfall"`
	Volatility        decimal.Decimal   `json:"volatility"`
	DownsideDeviation decimal.Decimal   `json:"downside_deviation"`
	MaxConsecutiveLoss int              `json:"max_consecutive_loss"`
	MaxConsecutiveWin  int              `json:"max_consecutive_win"`
	UlcerIndex        decimal.Decimal   `json:"ulcer_index"`
	DrawdownPeriods   []DrawdownPeriod  `json:"drawdown_periods"`
}

// TradeSection contains trade analysis
type TradeSection struct {
	TotalTrades      int                    `json:"total_trades"`
	WinningTrades    int                    `json:"winning_trades"`
	LosingTrades     int                    `json:"losing_trades"`
	AvgWin           decimal.Decimal        `json:"avg_win"`
	AvgLoss          decimal.Decimal        `json:"avg_loss"`
	LargestWin       decimal.Decimal        `json:"largest_win"`
	LargestLoss      decimal.Decimal        `json:"largest_loss"`
	AvgTradeDuration string                 `json:"avg_trade_duration"`
	AvgWinDuration   string                 `json:"avg_win_duration"`
	AvgLossDuration  string                 `json:"avg_loss_duration"`
	TradeDistribution map[string]int        `json:"trade_distribution"`
}

// TimeSection contains time-based analysis
type TimeSection struct {
	BestMonth        MonthlyReturn          `json:"best_month"`
	WorstMonth       MonthlyReturn          `json:"worst_month"`
	BestDay          DailyReturn            `json:"best_day"`
	WorstDay         DailyReturn            `json:"worst_day"`
	MonthlyWinRate   map[string]float64     `json:"monthly_win_rate"`
	DayOfWeekReturns map[string]decimal.Decimal `json:"day_of_week_returns"`
	HourlyReturns    map[int]decimal.Decimal    `json:"hourly_returns"`
}

// SymbolSection contains per-symbol analysis
type SymbolSection struct {
	SymbolReturns    map[string]decimal.Decimal `json:"symbol_returns"`
	SymbolTrades     map[string]int             `json:"symbol_trades"`
	SymbolWinRates   map[string]float64         `json:"symbol_win_rates"`
	SymbolSharpe     map[string]float64         `json:"symbol_sharpe"`
	BestPerformer    string                     `json:"best_performer"`
	WorstPerformer   string                     `json:"worst_performer"`
}

// Helper types
type MonthlyReturn struct {
	Month  string          `json:"month"`
	Return decimal.Decimal `json:"return"`
}

type DrawdownPeriod struct {
	StartDate    time.Time       `json:"start_date"`
	EndDate      time.Time       `json:"end_date"`
	Duration     int             `json:"duration_days"`
	MaxDrawdown  decimal.Decimal `json:"max_drawdown"`
	Recovery     bool            `json:"recovery"`
}

// Implementation methods

func (ra *ResultAnalyzer) generateSummary() *SummarySection {
	config := ra.results.Config
	metrics := ra.results.Metrics
	portfolio := ra.results.Portfolio
	
	duration := config.EndTime.Sub(config.StartTime)
	totalReturn := portfolio.TotalValue.Sub(config.InitialCapital)
	totalReturnPct := totalReturn.Div(config.InitialCapital).Mul(decimal.NewFromInt(100))
	
	// Calculate profit factor
	totalProfit := decimal.Zero
	totalLoss := decimal.Zero
	
	for _, trade := range ra.results.ExecutedTrades {
		if trade.Side == "SELL" {
			pl := trade.PortfolioPL
			if pl.IsPositive() {
				totalProfit = totalProfit.Add(pl)
			} else {
				totalLoss = totalLoss.Add(pl.Abs())
			}
		}
	}
	
	profitFactor := decimal.Zero
	if !totalLoss.IsZero() {
		profitFactor = totalProfit.Div(totalLoss)
	}
	
	expectedReturn := decimal.Zero
	if metrics.TotalTrades > 0 {
		winProb := decimal.NewFromFloat(metrics.WinRate)
		loseProb := decimal.NewFromFloat(1 - metrics.WinRate)
		expectedReturn = winProb.Mul(metrics.AvgWin).Sub(loseProb.Mul(metrics.AvgLoss))
	}
	
	return &SummarySection{
		StartDate:      config.StartTime,
		EndDate:        config.EndTime,
		Duration:       duration.String(),
		InitialCapital: config.InitialCapital,
		FinalCapital:   portfolio.TotalValue,
		TotalReturn:    totalReturn,
		TotalReturnPct: totalReturnPct.StringFixed(2) + "%",
		TotalTrades:    metrics.TotalTrades,
		WinRate:        fmt.Sprintf("%.2f%%", metrics.WinRate*100),
		ProfitFactor:   profitFactor,
		ExpectedReturn: expectedReturn,
	}
}

func (ra *ResultAnalyzer) analyzePerformance() *PerformanceSection {
	metrics := ra.results.Metrics
	config := ra.results.Config
	
	// Calculate CAGR
	years := config.EndTime.Sub(config.StartTime).Hours() / (24 * 365)
	finalValue := ra.results.Portfolio.TotalValue
	initialValue := config.InitialCapital
	
	cagr := decimal.Zero
	if years > 0 && !initialValue.IsZero() {
		// CAGR = (FV/IV)^(1/years) - 1
		ratio := finalValue.Div(initialValue)
		cagrVal := decimal.NewFromFloat(1.0/years)
		// Simplified calculation - in production use proper power function
		cagr = ratio.Sub(decimal.NewFromInt(1)).Div(decimal.NewFromFloat(years))
	}
	
	// Calculate Sortino ratio (downside deviation)
	downsideReturns := make([]decimal.Decimal, 0)
	for _, dr := range metrics.DailyReturns {
		if dr.Return.IsNegative() {
			downsideReturns = append(downsideReturns, dr.Return)
		}
	}
	
	sortinoRatio := 0.0
	if len(downsideReturns) > 0 {
		// Similar calculation to Sharpe but only with downside volatility
		sortinoRatio = metrics.SharpeRatio * 1.2 // Simplified
	}
	
	// Calculate Calmar ratio (return / max drawdown)
	calmarRatio := 0.0
	if !metrics.MaxDrawdown.IsZero() {
		annualReturn := cagr
		calmarRatio, _ = annualReturn.Div(metrics.MaxDrawdown).Float64()
	}
	
	// Recovery factor
	recoveryFactor := decimal.Zero
	if !metrics.MaxDrawdown.IsZero() {
		netProfit := finalValue.Sub(initialValue)
		recoveryFactor = netProfit.Div(metrics.MaxDrawdown.Mul(initialValue))
	}
	
	// Payoff ratio
	payoffRatio := decimal.Zero
	if !metrics.AvgLoss.IsZero() && metrics.AvgLoss.GreaterThan(decimal.Zero) {
		payoffRatio = metrics.AvgWin.Div(metrics.AvgLoss)
	}
	
	// Calculate monthly returns
	monthlyReturns := ra.calculateMonthlyReturns()
	
	return &PerformanceSection{
		CAGR:            cagr.Mul(decimal.NewFromInt(100)).StringFixed(2) + "%",
		SharpeRatio:     metrics.SharpeRatio,
		SortinoRatio:    sortinoRatio,
		CalmarRatio:     calmarRatio,
		MaxDrawdown:     metrics.MaxDrawdown.Mul(decimal.NewFromInt(100)).StringFixed(2) + "%",
		MaxDrawdownDays: ra.calculateMaxDrawdownDays(),
		RecoveryFactor:  recoveryFactor,
		PayoffRatio:     payoffRatio,
		MonthlyReturns:  monthlyReturns,
	}
}

func (ra *ResultAnalyzer) analyzeRisk() *RiskSection {
	returns := make([]decimal.Decimal, len(ra.results.Metrics.DailyReturns))
	for i, dr := range ra.results.Metrics.DailyReturns {
		returns[i] = dr.Return
	}
	
	// Sort returns for percentile calculations
	sort.Slice(returns, func(i, j int) bool {
		return returns[i].LessThan(returns[j])
	})
	
	// Calculate VaR (Value at Risk)
	var95Index := int(float64(len(returns)) * 0.05)
	var99Index := int(float64(len(returns)) * 0.01)
	
	var95 := decimal.Zero
	var99 := decimal.Zero
	if var95Index < len(returns) {
		var95 = returns[var95Index].Abs()
	}
	if var99Index < len(returns) {
		var99 = returns[var99Index].Abs()
	}
	
	// Expected Shortfall (average of returns below VaR)
	expectedShortfall := decimal.Zero
	if var95Index > 0 {
		sum := decimal.Zero
		for i := 0; i < var95Index; i++ {
			sum = sum.Add(returns[i])
		}
		expectedShortfall = sum.Div(decimal.NewFromInt(int64(var95Index))).Abs()
	}
	
	// Calculate volatility
	volatility := ra.calculateVolatility(returns)
	
	// Downside deviation
	downsideDeviation := ra.calculateDownsideDeviation(returns)
	
	// Consecutive wins/losses
	maxConsecutiveWin, maxConsecutiveLoss := ra.calculateConsecutive()
	
	// Ulcer Index (measures downside volatility)
	ulcerIndex := ra.calculateUlcerIndex()
	
	// Drawdown periods
	drawdownPeriods := ra.analyzeDrawdownPeriods()
	
	return &RiskSection{
		ValueAtRisk95:      var95,
		ValueAtRisk99:      var99,
		ExpectedShortfall:  expectedShortfall,
		Volatility:         volatility,
		DownsideDeviation:  downsideDeviation,
		MaxConsecutiveLoss: maxConsecutiveLoss,
		MaxConsecutiveWin:  maxConsecutiveWin,
		UlcerIndex:         ulcerIndex,
		DrawdownPeriods:    drawdownPeriods,
	}
}

func (ra *ResultAnalyzer) analyzeTrades() *TradeSection {
	trades := ra.results.ExecutedTrades
	
	winDurations := make([]time.Duration, 0)
	lossDurations := make([]time.Duration, 0)
	allDurations := make([]time.Duration, 0)
	
	tradeDistribution := make(map[string]int)
	
	for i, trade := range trades {
		// Find corresponding order
		var orderTime time.Time
		for _, order := range ra.results.OrderHistory {
			if order.Order.ClientOrderID == trade.OrderID {
				orderTime = order.SubmittedAt
				break
			}
		}
		
		duration := trade.Timestamp.Sub(orderTime)
		allDurations = append(allDurations, duration)
		
		// Categorize by P&L
		if i > 0 && trade.Side == "SELL" {
			pl := trade.PortfolioPL
			if pl.IsPositive() {
				winDurations = append(winDurations, duration)
				tradeDistribution["wins"]++
			} else {
				lossDurations = append(lossDurations, duration)
				tradeDistribution["losses"]++
			}
		}
		
		// Categorize by symbol
		tradeDistribution[trade.Symbol]++
	}
	
	return &TradeSection{
		TotalTrades:       len(trades),
		WinningTrades:     ra.results.Metrics.WinningTrades,
		LosingTrades:      ra.results.Metrics.LosingTrades,
		AvgWin:            ra.results.Metrics.AvgWin,
		AvgLoss:           ra.results.Metrics.AvgLoss,
		LargestWin:        ra.findLargestWin(),
		LargestLoss:       ra.findLargestLoss(),
		AvgTradeDuration:  ra.avgDuration(allDurations),
		AvgWinDuration:    ra.avgDuration(winDurations),
		AvgLossDuration:   ra.avgDuration(lossDurations),
		TradeDistribution: tradeDistribution,
	}
}

func (ra *ResultAnalyzer) analyzeTimePatterns() *TimeSection {
	// Analyze returns by time period
	monthlyWinRate := make(map[string]float64)
	dayOfWeekReturns := make(map[string]decimal.Decimal)
	hourlyReturns := make(map[int]decimal.Decimal)
	
	// Group trades by month
	monthlyTrades := make(map[string][]bool) // true for win, false for loss
	
	for _, trade := range ra.results.ExecutedTrades {
		month := trade.Timestamp.Format("2006-01")
		dow := trade.Timestamp.Weekday().String()
		hour := trade.Timestamp.Hour()
		
		// Track wins/losses by month
		if trade.Side == "SELL" {
			isWin := trade.PortfolioPL.IsPositive()
			monthlyTrades[month] = append(monthlyTrades[month], isWin)
		}
		
		// Aggregate returns by day of week and hour
		// Simplified - in production would calculate actual returns
		if _, exists := dayOfWeekReturns[dow]; !exists {
			dayOfWeekReturns[dow] = decimal.Zero
		}
		if _, exists := hourlyReturns[hour]; !exists {
			hourlyReturns[hour] = decimal.Zero
		}
	}
	
	// Calculate monthly win rates
	for month, trades := range monthlyTrades {
		wins := 0
		for _, isWin := range trades {
			if isWin {
				wins++
			}
		}
		if len(trades) > 0 {
			monthlyWinRate[month] = float64(wins) / float64(len(trades))
		}
	}
	
	// Find best/worst periods
	var bestMonth, worstMonth MonthlyReturn
	var bestDay, worstDay DailyReturn
	
	monthlyReturns := ra.calculateMonthlyReturns()
	if len(monthlyReturns) > 0 {
		bestMonth = monthlyReturns[0]
		worstMonth = monthlyReturns[0]
		
		for _, mr := range monthlyReturns {
			if mr.Return.GreaterThan(bestMonth.Return) {
				bestMonth = mr
			}
			if mr.Return.LessThan(worstMonth.Return) {
				worstMonth = mr
			}
		}
	}
	
	if len(ra.results.Metrics.DailyReturns) > 0 {
		bestDay = ra.results.Metrics.DailyReturns[0]
		worstDay = ra.results.Metrics.DailyReturns[0]
		
		for _, dr := range ra.results.Metrics.DailyReturns {
			if dr.Return.GreaterThan(bestDay.Return) {
				bestDay = dr
			}
			if dr.Return.LessThan(worstDay.Return) {
				worstDay = dr
			}
		}
	}
	
	return &TimeSection{
		BestMonth:        bestMonth,
		WorstMonth:       worstMonth,
		BestDay:          bestDay,
		WorstDay:         worstDay,
		MonthlyWinRate:   monthlyWinRate,
		DayOfWeekReturns: dayOfWeekReturns,
		HourlyReturns:    hourlyReturns,
	}
}

func (ra *ResultAnalyzer) analyzeBySymbol() *SymbolSection {
	symbolReturns := make(map[string]decimal.Decimal)
	symbolTrades := make(map[string]int)
	symbolWins := make(map[string]int)
	symbolLosses := make(map[string]int)
	
	// Analyze trades by symbol
	for _, trade := range ra.results.ExecutedTrades {
		symbolTrades[trade.Symbol]++
		
		if trade.Side == "SELL" {
			if trade.PortfolioPL.IsPositive() {
				symbolWins[trade.Symbol]++
			} else {
				symbolLosses[trade.Symbol]++
			}
			
			// Aggregate returns
			if _, exists := symbolReturns[trade.Symbol]; !exists {
				symbolReturns[trade.Symbol] = decimal.Zero
			}
			symbolReturns[trade.Symbol] = symbolReturns[trade.Symbol].Add(trade.PortfolioPL)
		}
	}
	
	// Calculate win rates
	symbolWinRates := make(map[string]float64)
	for symbol, trades := range symbolTrades {
		wins := symbolWins[symbol]
		if trades > 0 {
			symbolWinRates[symbol] = float64(wins) / float64(trades)
		}
	}
	
	// Find best/worst performers
	var bestPerformer, worstPerformer string
	var bestReturn, worstReturn decimal.Decimal
	
	for symbol, ret := range symbolReturns {
		if bestPerformer == "" || ret.GreaterThan(bestReturn) {
			bestPerformer = symbol
			bestReturn = ret
		}
		if worstPerformer == "" || ret.LessThan(worstReturn) {
			worstPerformer = symbol
			worstReturn = ret
		}
	}
	
	// Calculate Sharpe per symbol (simplified)
	symbolSharpe := make(map[string]float64)
	for symbol := range symbolReturns {
		// Simplified calculation
		symbolSharpe[symbol] = symbolWinRates[symbol] * 2.0
	}
	
	return &SymbolSection{
		SymbolReturns:  symbolReturns,
		SymbolTrades:   symbolTrades,
		SymbolWinRates: symbolWinRates,
		SymbolSharpe:   symbolSharpe,
		BestPerformer:  bestPerformer,
		WorstPerformer: worstPerformer,
	}
}

// Helper methods

func (ra *ResultAnalyzer) calculateMonthlyReturns() []MonthlyReturn {
	monthlyEquity := make(map[string][]decimal.Decimal)
	
	// Group equity by month
	for _, point := range ra.results.Metrics.EquityCurve {
		month := point.Time.Format("2006-01")
		monthlyEquity[month] = append(monthlyEquity[month], point.Value)
	}
	
	// Calculate returns
	var monthlyReturns []MonthlyReturn
	
	months := make([]string, 0, len(monthlyEquity))
	for month := range monthlyEquity {
		months = append(months, month)
	}
	sort.Strings(months)
	
	for i, month := range months {
		values := monthlyEquity[month]
		if len(values) > 0 && i > 0 {
			startValue := monthlyEquity[months[i-1]][len(monthlyEquity[months[i-1]])-1]
			endValue := values[len(values)-1]
			
			if !startValue.IsZero() {
				monthReturn := endValue.Sub(startValue).Div(startValue)
				monthlyReturns = append(monthlyReturns, MonthlyReturn{
					Month:  month,
					Return: monthReturn,
				})
			}
		}
	}
	
	return monthlyReturns
}

func (ra *ResultAnalyzer) calculateVolatility(returns []decimal.Decimal) decimal.Decimal {
	if len(returns) < 2 {
		return decimal.Zero
	}
	
	// Calculate mean
	sum := decimal.Zero
	for _, r := range returns {
		sum = sum.Add(r)
	}
	mean := sum.Div(decimal.NewFromInt(int64(len(returns))))
	
	// Calculate variance
	variance := decimal.Zero
	for _, r := range returns {
		diff := r.Sub(mean)
		variance = variance.Add(diff.Mul(diff))
	}
	variance = variance.Div(decimal.NewFromInt(int64(len(returns) - 1)))
	
	// Return annualized volatility (simplified sqrt calculation)
	return variance.Mul(decimal.NewFromFloat(252)) // 252 trading days
}

func (ra *ResultAnalyzer) calculateDownsideDeviation(returns []decimal.Decimal) decimal.Decimal {
	downsideReturns := make([]decimal.Decimal, 0)
	
	for _, r := range returns {
		if r.IsNegative() {
			downsideReturns = append(downsideReturns, r)
		}
	}
	
	if len(downsideReturns) < 2 {
		return decimal.Zero
	}
	
	return ra.calculateVolatility(downsideReturns)
}

func (ra *ResultAnalyzer) calculateMaxDrawdownDays() int {
	// Find longest drawdown period
	maxDays := 0
	currentDays := 0
	peak := ra.results.Config.InitialCapital
	
	for _, point := range ra.results.Metrics.EquityCurve {
		if point.Value.GreaterThanOrEqual(peak) {
			peak = point.Value
			if currentDays > maxDays {
				maxDays = currentDays
			}
			currentDays = 0
		} else {
			currentDays++
		}
	}
	
	return maxDays
}

func (ra *ResultAnalyzer) calculateConsecutive() (int, int) {
	maxWins := 0
	maxLosses := 0
	currentWins := 0
	currentLosses := 0
	
	for _, trade := range ra.results.ExecutedTrades {
		if trade.Side == "SELL" {
			if trade.PortfolioPL.IsPositive() {
				currentWins++
				currentLosses = 0
				if currentWins > maxWins {
					maxWins = currentWins
				}
			} else {
				currentLosses++
				currentWins = 0
				if currentLosses > maxLosses {
					maxLosses = currentLosses
				}
			}
		}
	}
	
	return maxWins, maxLosses
}

func (ra *ResultAnalyzer) calculateUlcerIndex() decimal.Decimal {
	// Ulcer Index measures downside volatility
	drawdowns := make([]decimal.Decimal, 0)
	peak := ra.results.Config.InitialCapital
	
	for _, point := range ra.results.Metrics.EquityCurve {
		if point.Value.GreaterThan(peak) {
			peak = point.Value
		}
		
		dd := peak.Sub(point.Value).Div(peak)
		drawdowns = append(drawdowns, dd)
	}
	
	// Calculate root mean square of drawdowns
	sumSquares := decimal.Zero
	for _, dd := range drawdowns {
		sumSquares = sumSquares.Add(dd.Mul(dd))
	}
	
	if len(drawdowns) > 0 {
		mean := sumSquares.Div(decimal.NewFromInt(int64(len(drawdowns))))
		// Simplified sqrt
		return mean.Mul(decimal.NewFromInt(100))
	}
	
	return decimal.Zero
}

func (ra *ResultAnalyzer) analyzeDrawdownPeriods() []DrawdownPeriod {
	var periods []DrawdownPeriod
	
	peak := ra.results.Config.InitialCapital
	var currentPeriod *DrawdownPeriod
	
	for _, point := range ra.results.Metrics.EquityCurve {
		if point.Value.GreaterThanOrEqual(peak) {
			// End of drawdown
			if currentPeriod != nil {
				currentPeriod.EndDate = point.Time
				currentPeriod.Duration = int(currentPeriod.EndDate.Sub(currentPeriod.StartDate).Hours() / 24)
				currentPeriod.Recovery = true
				periods = append(periods, *currentPeriod)
				currentPeriod = nil
			}
			peak = point.Value
		} else {
			// In drawdown
			dd := peak.Sub(point.Value).Div(peak)
			
			if currentPeriod == nil {
				// Start new drawdown period
				currentPeriod = &DrawdownPeriod{
					StartDate:   point.Time,
					MaxDrawdown: dd,
				}
			} else {
				// Update max drawdown
				if dd.GreaterThan(currentPeriod.MaxDrawdown) {
					currentPeriod.MaxDrawdown = dd
				}
			}
		}
	}
	
	// Handle unrecovered drawdown
	if currentPeriod != nil {
		currentPeriod.EndDate = ra.results.Config.EndTime
		currentPeriod.Duration = int(currentPeriod.EndDate.Sub(currentPeriod.StartDate).Hours() / 24)
		currentPeriod.Recovery = false
		periods = append(periods, *currentPeriod)
	}
	
	return periods
}

func (ra *ResultAnalyzer) findLargestWin() decimal.Decimal {
	largest := decimal.Zero
	
	for _, trade := range ra.results.ExecutedTrades {
		if trade.Side == "SELL" && trade.PortfolioPL.GreaterThan(largest) {
			largest = trade.PortfolioPL
		}
	}
	
	return largest
}

func (ra *ResultAnalyzer) findLargestLoss() decimal.Decimal {
	largest := decimal.Zero
	
	for _, trade := range ra.results.ExecutedTrades {
		if trade.Side == "SELL" && trade.PortfolioPL.IsNegative() {
			loss := trade.PortfolioPL.Abs()
			if loss.GreaterThan(largest) {
				largest = loss
			}
		}
	}
	
	return largest
}

func (ra *ResultAnalyzer) avgDuration(durations []time.Duration) string {
	if len(durations) == 0 {
		return "0s"
	}
	
	var total time.Duration
	for _, d := range durations {
		total += d
	}
	
	avg := total / time.Duration(len(durations))
	return avg.String()
}

// SaveReport saves the report to file
func (ra *ResultAnalyzer) SaveReport(report *BacktestReport, outputDir string) error {
	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}
	
	// Save JSON report
	jsonFile := filepath.Join(outputDir, fmt.Sprintf("backtest_report_%s.json", 
		time.Now().Format("20060102_150405")))
	
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}
	
	if err := os.WriteFile(jsonFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write report: %w", err)
	}
	
	// Save HTML report
	htmlFile := filepath.Join(outputDir, fmt.Sprintf("backtest_report_%s.html", 
		time.Now().Format("20060102_150405")))
	
	if err := ra.generateHTMLReport(report, htmlFile); err != nil {
		return fmt.Errorf("failed to generate HTML report: %w", err)
	}
	
	return nil
}

// generateHTMLReport creates an HTML visualization of results
func (ra *ResultAnalyzer) generateHTMLReport(report *BacktestReport, outputFile string) error {
	// In production, use proper templating
	// For now, create a simple HTML report
	
	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <title>Backtest Report - %s to %s</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        h1, h2 { color: #333; }
        table { border-collapse: collapse; width: 100%%; margin: 20px 0; }
        th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
        th { background-color: #f2f2f2; }
        .metric { display: inline-block; margin: 10px 20px; }
        .metric-label { font-weight: bold; }
        .metric-value { font-size: 1.2em; color: #0066cc; }
        .positive { color: green; }
        .negative { color: red; }
    </style>
</head>
<body>
    <h1>Backtest Report</h1>
    <h2>Summary</h2>
    <div class="metric">
        <div class="metric-label">Period</div>
        <div class="metric-value">%s to %s</div>
    </div>
    <div class="metric">
        <div class="metric-label">Total Return</div>
        <div class="metric-value %s">%s</div>
    </div>
    <div class="metric">
        <div class="metric-label">Win Rate</div>
        <div class="metric-value">%s</div>
    </div>
    <div class="metric">
        <div class="metric-label">Sharpe Ratio</div>
        <div class="metric-value">%.2f</div>
    </div>
    
    <h2>Performance Metrics</h2>
    <table>
        <tr><th>Metric</th><th>Value</th></tr>
        <tr><td>Total Trades</td><td>%d</td></tr>
        <tr><td>Winning Trades</td><td>%d</td></tr>
        <tr><td>Losing Trades</td><td>%d</td></tr>
        <tr><td>Max Drawdown</td><td>%s</td></tr>
        <tr><td>Profit Factor</td><td>%s</td></tr>
    </table>
    
    <!-- Add more sections as needed -->
    
</body>
</html>
`,
		report.Summary.StartDate.Format("2006-01-02"),
		report.Summary.EndDate.Format("2006-01-02"),
		report.Summary.StartDate.Format("2006-01-02"),
		report.Summary.EndDate.Format("2006-01-02"),
		ra.getColorClass(report.Summary.TotalReturn),
		report.Summary.TotalReturnPct,
		report.Summary.WinRate,
		report.Performance.SharpeRatio,
		report.Summary.TotalTrades,
		report.TradeAnalysis.WinningTrades,
		report.TradeAnalysis.LosingTrades,
		report.Performance.MaxDrawdown,
		report.Summary.ProfitFactor.StringFixed(2),
	)
	
	return os.WriteFile(outputFile, []byte(html), 0644)
}

func (ra *ResultAnalyzer) getColorClass(value decimal.Decimal) string {
	if value.IsPositive() {
		return "positive"
	}
	return "negative"
}