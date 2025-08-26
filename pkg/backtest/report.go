package backtest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
)

// Report represents a backtest report
type Report struct {
	// Metadata
	StrategyName    string
	StrategyType    string
	BacktestPeriod  Period
	GeneratedAt     time.Time
	
	// Performance summary
	Performance     PerformanceSummary
	
	// Risk metrics
	Risk            RiskSummary
	
	// Trading statistics
	Trading         TradingSummary
	
	// Monthly returns
	MonthlyReturns  []MonthlyReturn
	
	// Equity curve data
	EquityCurve     []EquityDataPoint
	
	// Trade log
	Trades          []TradeDetail
	
	// Drawdown periods
	DrawdownPeriods []DrawdownPeriod
}

// Period represents a time period
type Period struct {
	Start time.Time
	End   time.Time
}

// PerformanceSummary summarizes performance metrics
type PerformanceSummary struct {
	TotalReturn        string
	AnnualizedReturn   string
	TotalPnL          string
	MaxEquity         string
	MinEquity         string
	EndingEquity      string
}

// RiskSummary summarizes risk metrics
type RiskSummary struct {
	Volatility         string
	SharpeRatio        string
	SortinoRatio       string
	MaxDrawdown        string
	MaxDrawdownDuration string
	VaR95             string // Value at Risk at 95% confidence
	CVaR95            string // Conditional VaR at 95% confidence
}

// TradingSummary summarizes trading statistics
type TradingSummary struct {
	TotalTrades       int
	WinningTrades     int
	LosingTrades      int
	WinRate           string
	AverageWin        string
	AverageLoss       string
	LargestWin        string
	LargestLoss       string
	ProfitFactor      string
	ExpectancyPerTrade string
	TotalFees         string
	TotalSlippage     string
	TimeInMarket      string
}

// MonthlyReturn represents returns for a month
type MonthlyReturn struct {
	Year   int
	Month  string
	Return string
}

// EquityDataPoint represents a point on the equity curve
type EquityDataPoint struct {
	Date   string
	Equity float64
}

// TradeDetail represents detailed trade information
type TradeDetail struct {
	EntryTime  string
	ExitTime   string
	Symbol     string
	Side       string
	Quantity   string
	EntryPrice string
	ExitPrice  string
	PnL        string
	PnLPercent string
	Fees       string
	Duration   string
}

// DrawdownPeriod represents a drawdown period
type DrawdownPeriod struct {
	StartDate  string
	EndDate    string
	Depth      string
	Duration   string
	Recovery   string
}

// GenerateReport generates a comprehensive backtest report
func GenerateReport(engine *BacktestEngine, strategyName, strategyType string) *Report {
	metrics := engine.GetMetrics()
	equity := engine.GetEquityCurve()
	trades := engine.GetTrades()
	
	report := &Report{
		StrategyName: strategyName,
		StrategyType: strategyType,
		BacktestPeriod: Period{
			Start: engine.config.StartTime,
			End:   engine.config.EndTime,
		},
		GeneratedAt: time.Now(),
	}
	
	// Fill performance summary
	report.fillPerformanceSummary(metrics, equity)
	
	// Fill risk summary
	report.fillRiskSummary(metrics, equity)
	
	// Fill trading summary
	report.fillTradingSummary(metrics)
	
	// Calculate monthly returns
	report.calculateMonthlyReturns(equity)
	
	// Prepare equity curve data
	report.prepareEquityCurve(equity)
	
	// Prepare trade details
	report.prepareTradeDetails(trades)
	
	// Calculate drawdown periods
	report.calculateDrawdownPeriods(equity)
	
	return report
}

// fillPerformanceSummary fills the performance summary
func (r *Report) fillPerformanceSummary(metrics *BacktestMetrics, equity []EquityPoint) {
	if len(equity) == 0 {
		return
	}
	
	startEquity := equity[0].Equity
	endEquity := equity[len(equity)-1].Equity
	maxEquity := startEquity
	minEquity := startEquity
	
	for _, point := range equity {
		if point.Equity.GreaterThan(maxEquity) {
			maxEquity = point.Equity
		}
		if point.Equity.LessThan(minEquity) {
			minEquity = point.Equity
		}
	}
	
	totalPnL := endEquity.Sub(startEquity)
	
	r.Performance = PerformanceSummary{
		TotalReturn:      formatPercent(metrics.TotalReturn),
		AnnualizedReturn: formatPercent(metrics.AnnualizedReturn),
		TotalPnL:        formatMoney(totalPnL),
		MaxEquity:       formatMoney(maxEquity),
		MinEquity:       formatMoney(minEquity),
		EndingEquity:    formatMoney(endEquity),
	}
}

// fillRiskSummary fills the risk summary
func (r *Report) fillRiskSummary(metrics *BacktestMetrics, equity []EquityPoint) {
	// Calculate VaR and CVaR
	var95, cvar95 := calculateVaR(equity, 0.95)
	
	r.Risk = RiskSummary{
		Volatility:          formatPercent(metrics.Volatility),
		SharpeRatio:         formatDecimal(metrics.SharpeRatio, 2),
		SortinoRatio:        formatDecimal(metrics.SortinoRatio, 2),
		MaxDrawdown:         formatPercent(metrics.MaxDrawdown),
		MaxDrawdownDuration: formatDuration(metrics.MaxDrawdownDuration),
		VaR95:              formatPercent(var95),
		CVaR95:             formatPercent(cvar95),
	}
}

// fillTradingSummary fills the trading summary
func (r *Report) fillTradingSummary(metrics *BacktestMetrics) {
	expectancy := decimal.Zero
	if metrics.TotalTrades > 0 {
		winProb := decimal.NewFromFloat(metrics.WinRate)
		lossProb := decimal.NewFromFloat(1 - metrics.WinRate)
		expectancy = metrics.AverageWin.Mul(winProb).Sub(metrics.AverageLoss.Mul(lossProb))
	}
	
	r.Trading = TradingSummary{
		TotalTrades:        metrics.TotalTrades,
		WinningTrades:      metrics.WinningTrades,
		LosingTrades:       metrics.LosingTrades,
		WinRate:            formatPercent(decimal.NewFromFloat(metrics.WinRate)),
		AverageWin:         formatMoney(metrics.AverageWin),
		AverageLoss:        formatMoney(metrics.AverageLoss),
		LargestWin:         formatMoney(metrics.LargestWin),
		LargestLoss:        formatMoney(metrics.LargestLoss),
		ProfitFactor:       formatDecimal(metrics.ProfitFactor, 2),
		ExpectancyPerTrade: formatMoney(expectancy),
		TotalFees:          formatMoney(metrics.TotalFees),
		TotalSlippage:      formatMoney(metrics.TotalSlippage),
		TimeInMarket:       fmt.Sprintf("%.1f%%", metrics.TimeInMarket*100),
	}
}

// calculateMonthlyReturns calculates returns by month
func (r *Report) calculateMonthlyReturns(equity []EquityPoint) {
	if len(equity) < 2 {
		return
	}
	
	// Group equity points by month
	monthlyData := make(map[string][]EquityPoint)
	
	for _, point := range equity {
		key := fmt.Sprintf("%d-%02d", point.Timestamp.Year(), point.Timestamp.Month())
		monthlyData[key] = append(monthlyData[key], point)
	}
	
	// Calculate returns for each month
	months := make([]string, 0, len(monthlyData))
	for month := range monthlyData {
		months = append(months, month)
	}
	sort.Strings(months)
	
	r.MonthlyReturns = make([]MonthlyReturn, 0)
	
	for _, monthKey := range months {
		points := monthlyData[monthKey]
		if len(points) < 2 {
			continue
		}
		
		startEquity := points[0].Equity
		endEquity := points[len(points)-1].Equity
		
		if startEquity.IsZero() {
			continue
		}
		
		monthlyReturn := endEquity.Sub(startEquity).Div(startEquity)
		
		year, _ := strconv.Atoi(monthKey[:4])
		month, _ := strconv.Atoi(monthKey[5:])
		
		r.MonthlyReturns = append(r.MonthlyReturns, MonthlyReturn{
			Year:   year,
			Month:  time.Month(month).String(),
			Return: formatPercent(monthlyReturn),
		})
	}
}

// prepareEquityCurve prepares equity curve data for charting
func (r *Report) prepareEquityCurve(equity []EquityPoint) {
	r.EquityCurve = make([]EquityDataPoint, 0)
	
	// Sample data points (max 1000 points for visualization)
	step := 1
	if len(equity) > 1000 {
		step = len(equity) / 1000
	}
	
	for i := 0; i < len(equity); i += step {
		r.EquityCurve = append(r.EquityCurve, EquityDataPoint{
			Date:   equity[i].Timestamp.Format("2006-01-02 15:04"),
			Equity: equity[i].Equity.InexactFloat64(),
		})
	}
}

// prepareTradeDetails prepares detailed trade information
func (r *Report) prepareTradeDetails(trades []TradeExecution) {
	// Group trades by position (simplified)
	// In production, would properly match entry and exit trades
	
	r.Trades = make([]TradeDetail, 0)
	
	// For demonstration, show individual trades
	for _, trade := range trades {
		detail := TradeDetail{
			EntryTime:  trade.Timestamp.Format("2006-01-02 15:04:05"),
			Symbol:     trade.Symbol,
			Side:       string(trade.Side),
			Quantity:   formatDecimal(trade.Quantity, 4),
			EntryPrice: formatDecimal(trade.Price, 2),
			Fees:       formatMoney(trade.Fee),
		}
		
		r.Trades = append(r.Trades, detail)
		
		// Limit to first 100 trades for report
		if len(r.Trades) >= 100 {
			break
		}
	}
}

// calculateDrawdownPeriods identifies and calculates drawdown periods
func (r *Report) calculateDrawdownPeriods(equity []EquityPoint) {
	if len(equity) < 2 {
		return
	}
	
	r.DrawdownPeriods = make([]DrawdownPeriod, 0)
	
	peak := equity[0].Equity
	var drawdownStart time.Time
	inDrawdown := false
	
	for _, point := range equity {
		if point.Equity.GreaterThan(peak) {
			// New peak, end of drawdown
			if inDrawdown {
				period := DrawdownPeriod{
					StartDate: drawdownStart.Format("2006-01-02"),
					EndDate:   point.Timestamp.Format("2006-01-02"),
					Duration:  formatDuration(point.Timestamp.Sub(drawdownStart)),
					Recovery:  "Full",
				}
				r.DrawdownPeriods = append(r.DrawdownPeriods, period)
				inDrawdown = false
			}
			peak = point.Equity
		} else {
			// In drawdown
			if !inDrawdown {
				drawdownStart = point.Timestamp
				inDrawdown = true
			}
			
			// Calculate drawdown depth
			drawdown := peak.Sub(point.Equity).Div(peak)
			
			// Update current drawdown period
			if inDrawdown && len(r.DrawdownPeriods) > 0 {
				currentPeriod := &r.DrawdownPeriods[len(r.DrawdownPeriods)-1]
				currentPeriod.Depth = formatPercent(drawdown)
			}
		}
	}
	
	// If still in drawdown at end
	if inDrawdown {
		finalDrawdown := peak.Sub(equity[len(equity)-1].Equity).Div(peak)
		period := DrawdownPeriod{
			StartDate: drawdownStart.Format("2006-01-02"),
			EndDate:   equity[len(equity)-1].Timestamp.Format("2006-01-02"),
			Depth:     formatPercent(finalDrawdown),
			Duration:  formatDuration(equity[len(equity)-1].Timestamp.Sub(drawdownStart)),
			Recovery:  "Ongoing",
		}
		r.DrawdownPeriods = append(r.DrawdownPeriods, period)
	}
	
	// Sort by drawdown depth
	sort.Slice(r.DrawdownPeriods, func(i, j int) bool {
		// Parse percentages for comparison
		depthI, _ := strconv.ParseFloat(r.DrawdownPeriods[i].Depth[:len(r.DrawdownPeriods[i].Depth)-1], 64)
		depthJ, _ := strconv.ParseFloat(r.DrawdownPeriods[j].Depth[:len(r.DrawdownPeriods[j].Depth)-1], 64)
		return depthI > depthJ
	})
	
	// Keep only top 10 drawdowns
	if len(r.DrawdownPeriods) > 10 {
		r.DrawdownPeriods = r.DrawdownPeriods[:10]
	}
}

// ToJSON converts the report to JSON
func (r *Report) ToJSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// ToHTML generates an HTML report
func (r *Report) ToHTML() (string, error) {
	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <title>Backtest Report - {{.StrategyName}}</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; background: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; background: white; padding: 20px; }
        h1, h2 { color: #333; }
        .metric-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 20px; margin: 20px 0; }
        .metric-card { background: #f9f9f9; padding: 15px; border-radius: 5px; }
        .metric-label { color: #666; font-size: 0.9em; }
        .metric-value { font-size: 1.5em; font-weight: bold; color: #2c3e50; }
        table { width: 100%; border-collapse: collapse; margin: 20px 0; }
        th, td { padding: 10px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background: #f5f5f5; font-weight: bold; }
        .positive { color: #27ae60; }
        .negative { color: #e74c3c; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Backtest Report: {{.StrategyName}}</h1>
        <p>Strategy Type: {{.StrategyType}}</p>
        <p>Period: {{.BacktestPeriod.Start.Format "2006-01-02"}} to {{.BacktestPeriod.End.Format "2006-01-02"}}</p>
        <p>Generated: {{.GeneratedAt.Format "2006-01-02 15:04:05"}}</p>
        
        <h2>Performance Summary</h2>
        <div class="metric-grid">
            <div class="metric-card">
                <div class="metric-label">Total Return</div>
                <div class="metric-value">{{.Performance.TotalReturn}}</div>
            </div>
            <div class="metric-card">
                <div class="metric-label">Annualized Return</div>
                <div class="metric-value">{{.Performance.AnnualizedReturn}}</div>
            </div>
            <div class="metric-card">
                <div class="metric-label">Total P&L</div>
                <div class="metric-value">{{.Performance.TotalPnL}}</div>
            </div>
        </div>
        
        <h2>Risk Metrics</h2>
        <div class="metric-grid">
            <div class="metric-card">
                <div class="metric-label">Sharpe Ratio</div>
                <div class="metric-value">{{.Risk.SharpeRatio}}</div>
            </div>
            <div class="metric-card">
                <div class="metric-label">Max Drawdown</div>
                <div class="metric-value negative">{{.Risk.MaxDrawdown}}</div>
            </div>
            <div class="metric-card">
                <div class="metric-label">Volatility</div>
                <div class="metric-value">{{.Risk.Volatility}}</div>
            </div>
        </div>
        
        <h2>Trading Statistics</h2>
        <table>
            <tr>
                <th>Metric</th>
                <th>Value</th>
            </tr>
            <tr>
                <td>Total Trades</td>
                <td>{{.Trading.TotalTrades}}</td>
            </tr>
            <tr>
                <td>Win Rate</td>
                <td class="positive">{{.Trading.WinRate}}</td>
            </tr>
            <tr>
                <td>Profit Factor</td>
                <td>{{.Trading.ProfitFactor}}</td>
            </tr>
            <tr>
                <td>Average Win</td>
                <td class="positive">{{.Trading.AverageWin}}</td>
            </tr>
            <tr>
                <td>Average Loss</td>
                <td class="negative">{{.Trading.AverageLoss}}</td>
            </tr>
            <tr>
                <td>Total Fees</td>
                <td class="negative">{{.Trading.TotalFees}}</td>
            </tr>
        </table>
        
        <h2>Monthly Returns</h2>
        <table>
            <tr>
                <th>Year</th>
                <th>Month</th>
                <th>Return</th>
            </tr>
            {{range .MonthlyReturns}}
            <tr>
                <td>{{.Year}}</td>
                <td>{{.Month}}</td>
                <td class="{{if hasPrefix .Return "-"}}negative{{else}}positive{{end}}">{{.Return}}</td>
            </tr>
            {{end}}
        </table>
    </div>
</body>
</html>
`
	
	// Create template functions
	funcMap := template.FuncMap{
		"hasPrefix": func(s, prefix string) bool {
			return len(s) > 0 && s[0] == '-'
		},
	}
	
	// Parse and execute template
	t, err := template.New("report").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return "", err
	}
	
	var buf bytes.Buffer
	if err := t.Execute(&buf, r); err != nil {
		return "", err
	}
	
	return buf.String(), nil
}

// Helper functions

func formatPercent(d decimal.Decimal) string {
	return fmt.Sprintf("%.2f%%", d.Mul(decimal.NewFromInt(100)).InexactFloat64())
}

func formatMoney(d decimal.Decimal) string {
	return fmt.Sprintf("$%.2f", d.InexactFloat64())
}

func formatDecimal(d decimal.Decimal, precision int) string {
	format := fmt.Sprintf("%%.%df", precision)
	return fmt.Sprintf(format, d.InexactFloat64())
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days > 0 {
		return fmt.Sprintf("%d days", days)
	}
	return d.Round(time.Hour).String()
}

func calculateVaR(equity []EquityPoint, confidence float64) (decimal.Decimal, decimal.Decimal) {
	if len(equity) < 2 {
		return decimal.Zero, decimal.Zero
	}
	
	// Calculate returns
	returns := make([]float64, 0, len(equity)-1)
	for i := 1; i < len(equity); i++ {
		if !equity[i-1].Equity.IsZero() {
			ret := equity[i].Equity.Sub(equity[i-1].Equity).Div(equity[i-1].Equity).InexactFloat64()
			returns = append(returns, ret)
		}
	}
	
	if len(returns) == 0 {
		return decimal.Zero, decimal.Zero
	}
	
	// Sort returns
	sort.Float64s(returns)
	
	// Calculate VaR index
	varIndex := int(float64(len(returns)) * (1 - confidence))
	if varIndex >= len(returns) {
		varIndex = len(returns) - 1
	}
	
	// VaR is the return at the confidence level
	var95 := decimal.NewFromFloat(-returns[varIndex])
	
	// CVaR is the average of returns worse than VaR
	sum := 0.0
	count := 0
	for i := 0; i <= varIndex; i++ {
		sum += returns[i]
		count++
	}
	
	cvar95 := decimal.Zero
	if count > 0 {
		cvar95 = decimal.NewFromFloat(-sum / float64(count))
	}
	
	return var95, cvar95
}