package backtest

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/mExOms/pkg/types"
)

// DefaultPerformanceAnalyzer implements performance analysis
type DefaultPerformanceAnalyzer struct{}

// NewDefaultPerformanceAnalyzer creates a new performance analyzer
func NewDefaultPerformanceAnalyzer() *DefaultPerformanceAnalyzer {
	return &DefaultPerformanceAnalyzer{}
}

// Analyze analyzes trading performance
func (a *DefaultPerformanceAnalyzer) Analyze(trades []Trade, equityCurve []EquityPoint) *BacktestResult {
	if len(trades) == 0 || len(equityCurve) == 0 {
		return &BacktestResult{
			TotalTrades: 0,
		}
	}

	result := &BacktestResult{
		TotalTrades: len(trades),
		Trades:      trades,
		EquityCurve: equityCurve,
	}

	// Calculate basic metrics
	a.calculateTradeMetrics(trades, result)
	a.calculateReturnMetrics(equityCurve, result)
	a.calculateRiskMetrics(equityCurve, trades, result)
	a.calculateDailyReturns(equityCurve, result)

	return result
}

// calculateTradeMetrics calculates trade-based metrics
func (a *DefaultPerformanceAnalyzer) calculateTradeMetrics(trades []Trade, result *BacktestResult) {
	var totalPnL float64
	var winningPnL float64
	var losingPnL float64
	var totalFees float64
	var totalSlippage float64

	consecutiveWins := 0
	consecutiveLosses := 0
	currentStreak := 0
	isWinStreak := false

	for i, trade := range trades {
		// Calculate P&L for each trade
		var pnl float64
		if i > 0 && trades[i-1].Symbol == trade.Symbol {
			// Simplified P&L calculation
			if trade.Side == types.SideSell {
				// Selling - calculate profit from previous buy
				pnl = (trade.Price - trades[i-1].Price) * trade.Quantity
			}
		}

		totalPnL += pnl
		totalFees += trade.Fee
		totalSlippage += math.Abs(trade.Slippage) * trade.Value

		if pnl > 0 {
			result.WinningTrades++
			winningPnL += pnl
			
			if isWinStreak {
				currentStreak++
			} else {
				currentStreak = 1
				isWinStreak = true
			}
			
			if currentStreak > consecutiveWins {
				consecutiveWins = currentStreak
			}
		} else if pnl < 0 {
			result.LosingTrades++
			losingPnL += math.Abs(pnl)
			
			if !isWinStreak {
				currentStreak++
			} else {
				currentStreak = 1
				isWinStreak = false
			}
			
			if currentStreak > consecutiveLosses {
				consecutiveLosses = currentStreak
			}
		}

		// Track best/worst trades
		if pnl > result.BestTrade {
			result.BestTrade = pnl
		}
		if pnl < result.WorstTrade {
			result.WorstTrade = pnl
		}
	}

	// Calculate aggregated metrics
	result.TotalFees = totalFees
	result.TotalSlippage = totalSlippage
	result.MaxConsecutiveWins = consecutiveWins
	result.MaxConsecutiveLosses = consecutiveLosses

	if result.TotalTrades > 0 {
		result.WinRate = float64(result.WinningTrades) / float64(result.TotalTrades)
		result.AverageTrade = totalPnL / float64(result.TotalTrades)
	}

	if losingPnL > 0 {
		result.ProfitFactor = winningPnL / losingPnL
	}
}

// calculateReturnMetrics calculates return-based metrics
func (a *DefaultPerformanceAnalyzer) calculateReturnMetrics(equityCurve []EquityPoint, result *BacktestResult) {
	if len(equityCurve) < 2 {
		return
	}

	initialEquity := equityCurve[0].Equity
	finalEquity := equityCurve[len(equityCurve)-1].Equity

	result.InitialCapital = initialEquity
	result.FinalCapital = finalEquity
	result.TotalReturn = finalEquity - initialEquity
	
	if initialEquity > 0 {
		result.TotalReturnPct = (result.TotalReturn / initialEquity) * 100
	}
}

// calculateRiskMetrics calculates risk metrics
func (a *DefaultPerformanceAnalyzer) calculateRiskMetrics(equityCurve []EquityPoint, trades []Trade, result *BacktestResult) {
	if len(equityCurve) < 2 {
		return
	}

	// Calculate maximum drawdown
	maxEquity := equityCurve[0].Equity
	maxDrawdown := 0.0
	maxDrawdownPct := 0.0

	for _, point := range equityCurve {
		if point.Equity > maxEquity {
			maxEquity = point.Equity
		}
		
		drawdown := maxEquity - point.Equity
		drawdownPct := 0.0
		if maxEquity > 0 {
			drawdownPct = (drawdown / maxEquity) * 100
		}
		
		if drawdown > maxDrawdown {
			maxDrawdown = drawdown
		}
		if drawdownPct > maxDrawdownPct {
			maxDrawdownPct = drawdownPct
		}
	}

	result.MaxDrawdown = maxDrawdown
	result.MaxDrawdownPct = maxDrawdownPct

	// Calculate Sharpe Ratio
	if len(result.DailyReturns) > 1 {
		result.SharpeRatio = a.CalculateSharpeRatio(result.DailyReturns, 0.0)
	}

	// Calculate Sortino Ratio
	if len(result.DailyReturns) > 1 {
		result.SortinoRatio = a.calculateSortinoRatio(result.DailyReturns, 0.0)
	}

	// Calculate Calmar Ratio
	if maxDrawdownPct > 0 && result.TotalReturnPct != 0 {
		annualizedReturn := result.TotalReturnPct // Simplified - should annualize properly
		result.CalmarRatio = annualizedReturn / maxDrawdownPct
	}
}

// calculateDailyReturns calculates daily returns from equity curve
func (a *DefaultPerformanceAnalyzer) calculateDailyReturns(equityCurve []EquityPoint, result *BacktestResult) {
	// Group equity points by day
	dailyEquity := make(map[string]float64)
	dailyTrades := make(map[string]int)
	dailyVolume := make(map[string]float64)

	for _, point := range equityCurve {
		day := point.Timestamp.Format("2006-01-02")
		dailyEquity[day] = point.Equity
	}

	// Count trades per day
	for _, trade := range result.Trades {
		day := trade.Timestamp.Format("2006-01-02")
		dailyTrades[day]++
		dailyVolume[day] += trade.Value
	}

	// Sort days
	var days []string
	for day := range dailyEquity {
		days = append(days, day)
	}
	sort.Strings(days)

	// Calculate daily returns
	for i := 1; i < len(days); i++ {
		prevEquity := dailyEquity[days[i-1]]
		currEquity := dailyEquity[days[i]]
		
		if prevEquity > 0 {
			dailyReturn := currEquity - prevEquity
			dailyReturnPct := (dailyReturn / prevEquity) * 100
			
			date, _ := time.Parse("2006-01-02", days[i])
			
			result.DailyReturns = append(result.DailyReturns, DailyReturn{
				Date:      date,
				Return:    dailyReturn,
				ReturnPct: dailyReturnPct,
				Equity:    currEquity,
				Trades:    dailyTrades[days[i]],
				Volume:    dailyVolume[days[i]],
			})
		}
	}
}

// CalculateSharpeRatio calculates the Sharpe ratio
func (a *DefaultPerformanceAnalyzer) CalculateSharpeRatio(returns []float64, riskFreeRate float64) float64 {
	if len(returns) < 2 {
		return 0.0
	}

	// Calculate average return
	sum := 0.0
	for _, r := range returns {
		sum += r
	}
	avgReturn := sum / float64(len(returns))

	// Calculate standard deviation
	sumSquaredDiff := 0.0
	for _, r := range returns {
		diff := r - avgReturn
		sumSquaredDiff += diff * diff
	}
	
	stdDev := math.Sqrt(sumSquaredDiff / float64(len(returns)-1))
	
	if stdDev == 0 {
		return 0.0
	}

	// Annualized Sharpe ratio (assuming daily returns)
	excessReturn := avgReturn - riskFreeRate/252 // Daily risk-free rate
	return (excessReturn / stdDev) * math.Sqrt(252)
}

// calculateSortinoRatio calculates the Sortino ratio
func (a *DefaultPerformanceAnalyzer) calculateSortinoRatio(returns []float64, targetReturn float64) float64 {
	if len(returns) < 2 {
		return 0.0
	}

	// Calculate average return
	sum := 0.0
	for _, r := range returns {
		sum += r
	}
	avgReturn := sum / float64(len(returns))

	// Calculate downside deviation
	sumSquaredDownside := 0.0
	downsideCount := 0
	
	for _, r := range returns {
		if r < targetReturn {
			diff := r - targetReturn
			sumSquaredDownside += diff * diff
			downsideCount++
		}
	}
	
	if downsideCount == 0 {
		return 0.0
	}
	
	downsideDev := math.Sqrt(sumSquaredDownside / float64(downsideCount))
	
	if downsideDev == 0 {
		return 0.0
	}

	// Annualized Sortino ratio
	excessReturn := avgReturn - targetReturn/252
	return (excessReturn / downsideDev) * math.Sqrt(252)
}

// CalculateMaxDrawdown calculates maximum drawdown
func (a *DefaultPerformanceAnalyzer) CalculateMaxDrawdown(equityCurve []EquityPoint) (float64, float64) {
	if len(equityCurve) == 0 {
		return 0.0, 0.0
	}

	maxEquity := equityCurve[0].Equity
	maxDrawdown := 0.0
	maxDrawdownPct := 0.0

	for _, point := range equityCurve {
		if point.Equity > maxEquity {
			maxEquity = point.Equity
		}
		
		drawdown := maxEquity - point.Equity
		drawdownPct := 0.0
		if maxEquity > 0 {
			drawdownPct = drawdown / maxEquity
		}
		
		if drawdown > maxDrawdown {
			maxDrawdown = drawdown
		}
		if drawdownPct > maxDrawdownPct {
			maxDrawdownPct = drawdownPct
		}
	}

	return maxDrawdown, maxDrawdownPct
}

// GenerateReport generates a backtest report
func (a *DefaultPerformanceAnalyzer) GenerateReport(result *BacktestResult, outputPath string) error {
	// Create output directory
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate multiple report formats
	if err := a.generateJSONReport(result, outputPath); err != nil {
		return err
	}

	if err := a.generateCSVReport(result, outputPath); err != nil {
		return err
	}

	if err := a.generateSummaryReport(result, outputPath); err != nil {
		return err
	}

	return nil
}

// generateJSONReport generates a JSON format report
func (a *DefaultPerformanceAnalyzer) generateJSONReport(result *BacktestResult, outputPath string) error {
	filepath := filepath.Join(outputPath, "backtest_result.json")
	
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return fmt.Errorf("failed to write JSON report: %w", err)
	}

	return nil
}

// generateCSVReport generates CSV reports for trades and equity curve
func (a *DefaultPerformanceAnalyzer) generateCSVReport(result *BacktestResult, outputPath string) error {
	// Generate trades CSV
	tradesPath := filepath.Join(outputPath, "trades.csv")
	if err := a.writeTradesToCSV(result.Trades, tradesPath); err != nil {
		return err
	}

	// Generate equity curve CSV
	equityPath := filepath.Join(outputPath, "equity_curve.csv")
	if err := a.writeEquityCurveToCSV(result.EquityCurve, equityPath); err != nil {
		return err
	}

	// Generate daily returns CSV
	returnsPath := filepath.Join(outputPath, "daily_returns.csv")
	if err := a.writeDailyReturnsToCSV(result.DailyReturns, returnsPath); err != nil {
		return err
	}

	return nil
}

// writeTradesToCSV writes trades to CSV file
func (a *DefaultPerformanceAnalyzer) writeTradesToCSV(trades []Trade, filepath string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{
		"Timestamp", "Symbol", "Exchange", "Side", "Price", "Quantity",
		"Fee", "Slippage", "ActualPrice", "Value", "Strategy",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write trades
	for _, trade := range trades {
		record := []string{
			trade.Timestamp.Format(time.RFC3339),
			trade.Symbol,
			trade.Exchange,
			string(trade.Side),
			fmt.Sprintf("%.8f", trade.Price),
			fmt.Sprintf("%.8f", trade.Quantity),
			fmt.Sprintf("%.8f", trade.Fee),
			fmt.Sprintf("%.6f", trade.Slippage),
			fmt.Sprintf("%.8f", trade.ActualPrice),
			fmt.Sprintf("%.8f", trade.Value),
			trade.Strategy,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

// writeEquityCurveToCSV writes equity curve to CSV file
func (a *DefaultPerformanceAnalyzer) writeEquityCurveToCSV(equityCurve []EquityPoint, filepath string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"Timestamp", "Equity", "Drawdown", "Positions"}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write equity points
	for _, point := range equityCurve {
		record := []string{
			point.Timestamp.Format(time.RFC3339),
			fmt.Sprintf("%.2f", point.Equity),
			fmt.Sprintf("%.4f", point.Drawdown),
			fmt.Sprintf("%d", point.Positions),
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

// writeDailyReturnsToCSV writes daily returns to CSV file
func (a *DefaultPerformanceAnalyzer) writeDailyReturnsToCSV(returns []DailyReturn, filepath string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"Date", "Return", "ReturnPct", "Equity", "Trades", "Volume"}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write daily returns
	for _, ret := range returns {
		record := []string{
			ret.Date.Format("2006-01-02"),
			fmt.Sprintf("%.2f", ret.Return),
			fmt.Sprintf("%.4f", ret.ReturnPct),
			fmt.Sprintf("%.2f", ret.Equity),
			fmt.Sprintf("%d", ret.Trades),
			fmt.Sprintf("%.2f", ret.Volume),
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

// generateSummaryReport generates a human-readable summary report
func (a *DefaultPerformanceAnalyzer) generateSummaryReport(result *BacktestResult, outputPath string) error {
	filepath := filepath.Join(outputPath, "summary.txt")
	
	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write summary
	fmt.Fprintf(file, "BACKTEST SUMMARY REPORT\n")
	fmt.Fprintf(file, "=======================\n\n")
	
	fmt.Fprintf(file, "Configuration:\n")
	fmt.Fprintf(file, "  Start Time: %s\n", result.Config.StartTime.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(file, "  End Time: %s\n", result.Config.EndTime.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(file, "  Duration: %s\n", result.Duration)
	fmt.Fprintf(file, "  Initial Capital: $%.2f\n\n", result.InitialCapital)
	
	fmt.Fprintf(file, "Performance Summary:\n")
	fmt.Fprintf(file, "  Final Capital: $%.2f\n", result.FinalCapital)
	fmt.Fprintf(file, "  Total Return: $%.2f (%.2f%%)\n", result.TotalReturn, result.TotalReturnPct)
	fmt.Fprintf(file, "  Max Drawdown: $%.2f (%.2f%%)\n", result.MaxDrawdown, result.MaxDrawdownPct)
	fmt.Fprintf(file, "  Sharpe Ratio: %.2f\n", result.SharpeRatio)
	fmt.Fprintf(file, "  Sortino Ratio: %.2f\n", result.SortinoRatio)
	fmt.Fprintf(file, "  Calmar Ratio: %.2f\n\n", result.CalmarRatio)
	
	fmt.Fprintf(file, "Trading Statistics:\n")
	fmt.Fprintf(file, "  Total Trades: %d\n", result.TotalTrades)
	fmt.Fprintf(file, "  Winning Trades: %d\n", result.WinningTrades)
	fmt.Fprintf(file, "  Losing Trades: %d\n", result.LosingTrades)
	fmt.Fprintf(file, "  Win Rate: %.2f%%\n", result.WinRate*100)
	fmt.Fprintf(file, "  Profit Factor: %.2f\n", result.ProfitFactor)
	fmt.Fprintf(file, "  Average Trade: $%.2f\n", result.AverageTrade)
	fmt.Fprintf(file, "  Best Trade: $%.2f\n", result.BestTrade)
	fmt.Fprintf(file, "  Worst Trade: $%.2f\n", result.WorstTrade)
	fmt.Fprintf(file, "  Max Consecutive Wins: %d\n", result.MaxConsecutiveWins)
	fmt.Fprintf(file, "  Max Consecutive Losses: %d\n\n", result.MaxConsecutiveLosses)
	
	fmt.Fprintf(file, "Costs:\n")
	fmt.Fprintf(file, "  Total Fees: $%.2f\n", result.TotalFees)
	fmt.Fprintf(file, "  Total Slippage: $%.2f\n\n", result.TotalSlippage)
	
	// Strategy-specific metrics
	if len(result.StrategyMetrics) > 0 {
		fmt.Fprintf(file, "Strategy Metrics:\n")
		for name, metrics := range result.StrategyMetrics {
			fmt.Fprintf(file, "\n  %s:\n", name)
			fmt.Fprintf(file, "    Total Trades: %d\n", metrics.TotalTrades)
			fmt.Fprintf(file, "    Win Rate: %.2f%%\n", metrics.WinRate*100)
			fmt.Fprintf(file, "    Total P&L: $%.2f\n", metrics.TotalPnL)
			fmt.Fprintf(file, "    Avg P&L: $%.2f\n", metrics.AvgPnL)
			fmt.Fprintf(file, "    Sharpe Ratio: %.2f\n", metrics.SharpeRatio)
		}
	}

	return nil
}