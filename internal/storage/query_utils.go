package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// QueryUtils provides utilities for querying storage files using system tools
type QueryUtils struct {
	basePath string
}

// NewQueryUtils creates a new query utilities instance
func NewQueryUtils(basePath string) *QueryUtils {
	return &QueryUtils{
		basePath: basePath,
	}
}

// GrepQuery performs a grep search on storage files
type GrepQuery struct {
	Pattern     string
	Account     string
	StorageType StorageType
	CaseSensitive bool
	InvertMatch   bool
	Count         bool
}

// Grep searches for patterns in storage files
func (qu *QueryUtils) Grep(query GrepQuery) ([]string, error) {
	// Build grep command
	args := []string{}
	
	if !query.CaseSensitive {
		args = append(args, "-i")
	}
	
	if query.InvertMatch {
		args = append(args, "-v")
	}
	
	if query.Count {
		args = append(args, "-c")
	}
	
	// Always include line for context
	args = append(args, "-H") // Print filename
	args = append(args, query.Pattern)
	
	// Build file pattern
	filePattern := qu.buildFilePattern(query.Account, query.StorageType)
	
	// Find files matching pattern
	cmd := exec.Command("bash", "-c", fmt.Sprintf("find %s -name '*.jsonl*' | xargs grep %s", filePattern, strings.Join(args, " ")))
	
	output, err := cmd.Output()
	if err != nil {
		// Grep returns error if no matches found, which is not really an error
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return []string{}, nil
		}
		return nil, err
	}
	
	lines := strings.Split(string(output), "\n")
	var results []string
	for _, line := range lines {
		if line != "" {
			results = append(results, line)
		}
	}
	
	return results, nil
}

// JqQuery represents a jq query
type JqQuery struct {
	Filter      string
	Account     string
	StorageType StorageType
	Raw         bool // Output raw strings, not JSON
	Compact     bool // Compact output
}

// Jq runs a jq query on storage files
func (qu *QueryUtils) Jq(query JqQuery) ([]string, error) {
	// Build jq command
	args := []string{}
	
	if query.Raw {
		args = append(args, "-r")
	}
	
	if query.Compact {
		args = append(args, "-c")
	}
	
	args = append(args, query.Filter)
	
	// Build file pattern
	filePattern := qu.buildFilePattern(query.Account, query.StorageType)
	
	// Find and process files
	cmd := exec.Command("bash", "-c", 
		fmt.Sprintf("find %s -name '*.jsonl' -exec jq %s {} \\; 2>/dev/null", 
			filePattern, strings.Join(args, " ")))
	
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("jq query failed: %w", err)
	}
	
	lines := strings.Split(string(output), "\n")
	var results []string
	for _, line := range lines {
		if line != "" {
			results = append(results, line)
		}
	}
	
	return results, nil
}

// QueryExamples provides example queries for users
func (qu *QueryUtils) QueryExamples() []QueryExample {
	return []QueryExample{
		{
			Description: "모든 거래 찾기 (Find all trades)",
			GrepCmd:     `grep '"event":"order_filled"' /path/to/storage/*/trading_log/*/*.jsonl`,
			JqCmd:       `jq 'select(.event == "order_filled")' /path/to/storage/*/trading_log/*/*.jsonl`,
		},
		{
			Description: "특정 계정의 모든 활동 (All activity for specific account)",
			GrepCmd:     `grep '"account":"binance_main"' /path/to/storage/*/*/*/*.jsonl`,
			JqCmd:       `jq 'select(.account == "binance_main")' /path/to/storage/*/*/*/*.jsonl`,
		},
		{
			Description: "오늘의 모든 거래량 계산 (Calculate today's volume)",
			JqCmd:       `jq -s 'map(select(.event == "order_filled") | .price * .quantity) | add' /path/to/storage/*/trading_log/$(date +%Y/%m/%d)/*.jsonl`,
		},
		{
			Description: "전략별 성과 보기 (View performance by strategy)",
			JqCmd:       `jq 'select(.strategy == "momentum") | .performance' /path/to/storage/*/strategy_log/*/*.jsonl`,
		},
		{
			Description: "대량 주문 찾기 (Find large orders)",
			JqCmd:       `jq 'select(.quantity > 1000)' /path/to/storage/*/trading_log/*/*.jsonl`,
		},
		{
			Description: "실패한 전송 찾기 (Find failed transfers)",
			GrepCmd:     `grep '"status":"failed"' /path/to/storage/*/transfer_log/*/*.jsonl`,
			JqCmd:       `jq 'select(.status == "failed")' /path/to/storage/*/transfer_log/*/*.jsonl`,
		},
		{
			Description: "최신 스냅샷 가져오기 (Get latest snapshot)",
			JqCmd:       `jq -s 'sort_by(.timestamp) | last' /path/to/storage/*/state_snapshot/*/*.jsonl`,
		},
		{
			Description: "압축 파일 검색 (Search compressed files)",
			GrepCmd:     `zgrep '"symbol":"BTC/USDT"' /path/to/storage/*/*/*/*.jsonl.gz`,
		},
	}
}

// ExportToCSV exports query results to CSV format
func (qu *QueryUtils) ExportToCSV(jqFilter, outputFile string, query JqQuery) error {
	// Build CSV conversion filter
	csvFilter := fmt.Sprintf("%s | @csv", jqFilter)
	
	query.Filter = csvFilter
	query.Raw = true
	
	results, err := qu.Jq(query)
	if err != nil {
		return err
	}
	
	// Write to file
	file, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer file.Close()
	
	for _, line := range results {
		fmt.Fprintln(file, line)
	}
	
	return nil
}

// GenerateReport generates a summary report
func (qu *QueryUtils) GenerateReport(account string, reportType string) (string, error) {
	switch reportType {
	case "daily":
		return qu.generateDailyReport(account)
	case "performance":
		return qu.generatePerformanceReport(account)
	case "risk":
		return qu.generateRiskReport(account)
	default:
		return "", fmt.Errorf("unknown report type: %s", reportType)
	}
}

// generateDailyReport creates a daily summary report
func (qu *QueryUtils) generateDailyReport(account string) (string, error) {
	report := ReportData{
		Title:     fmt.Sprintf("Daily Report - %s", account),
		Timestamp: fmt.Sprintf("%s", time.Now().Format("2006-01-02 15:04:05")),
		Sections:  []ReportSection{},
	}
	
	// Trading activity
	tradingQuery := JqQuery{
		Filter:      `select(.event == "order_filled") | {symbol, side, price, quantity}`,
		Account:     account,
		StorageType: StorageTypeTradingLog,
		Compact:     true,
	}
	
	trades, _ := qu.Jq(tradingQuery)
	report.Sections = append(report.Sections, ReportSection{
		Title: "Trading Activity",
		Data: map[string]interface{}{
			"total_trades": len(trades),
			"trades":       trades,
		},
	})
	
	// Add more sections as needed
	
	reportJSON, _ := json.MarshalIndent(report, "", "  ")
	return string(reportJSON), nil
}

// generatePerformanceReport creates a performance report
func (qu *QueryUtils) generatePerformanceReport(account string) (string, error) {
	// Implementation similar to daily report but focused on performance metrics
	return "", nil
}

// generateRiskReport creates a risk report
func (qu *QueryUtils) generateRiskReport(account string) (string, error) {
	// Implementation similar to daily report but focused on risk metrics
	return "", nil
}

// buildFilePattern builds a file pattern for searches
func (qu *QueryUtils) buildFilePattern(account string, storageType StorageType) string {
	if account != "" && storageType != "" {
		return fmt.Sprintf("%s/%s/%s", qu.basePath, account, storageType)
	} else if account != "" {
		return fmt.Sprintf("%s/%s", qu.basePath, account)
	} else if storageType != "" {
		return fmt.Sprintf("%s/*/%s", qu.basePath, storageType)
	}
	return qu.basePath
}

// QueryExample represents a query example
type QueryExample struct {
	Description string
	GrepCmd     string
	JqCmd       string
}

// ReportData represents report structure
type ReportData struct {
	Title     string          `json:"title"`
	Timestamp string          `json:"timestamp"`
	Sections  []ReportSection `json:"sections"`
}

// ReportSection represents a section in a report
type ReportSection struct {
	Title string                 `json:"title"`
	Data  map[string]interface{} `json:"data"`
}

// Add missing import
import "time"