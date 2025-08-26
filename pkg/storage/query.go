package storage

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Query provides file-based query capabilities
type Query struct {
	baseDir string
}

// NewQuery creates a new query instance
func NewQuery(baseDir string) *Query {
	return &Query{
		baseDir: baseDir,
	}
}

// QueryFilter defines filters for querying data
type QueryFilter struct {
	AccountID  string
	Exchange   string
	Symbol     string
	Strategy   string
	StartTime  time.Time
	EndTime    time.Time
	MinAmount  decimal.Decimal
	MaxAmount  decimal.Decimal
	Side       string
	Status     string
}

// QueryResult holds query results
type QueryResult struct {
	Trades    []TradeRecord
	Orders    []OrderRecord
	Transfers []TransferRecord
	Count     int
	TotalSize int64
}

// QueryTrades queries trade records
func (q *Query) QueryTrades(filter QueryFilter) (*QueryResult, error) {
	result := &QueryResult{
		Trades: make([]TradeRecord, 0),
	}

	// Build file pattern
	for date := filter.StartTime; date.Before(filter.EndTime); date = date.AddDate(0, 0, 1) {
		pattern := q.buildTradePattern(date, filter)
		files, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}

		for _, file := range files {
			trades, err := q.readTradeFile(file, filter)
			if err != nil {
				continue
			}
			result.Trades = append(result.Trades, trades...)
		}
	}

	result.Count = len(result.Trades)
	return result, nil
}

// QueryOrders queries order records
func (q *Query) QueryOrders(filter QueryFilter) (*QueryResult, error) {
	result := &QueryResult{
		Orders: make([]OrderRecord, 0),
	}

	// Build file pattern
	for date := filter.StartTime; date.Before(filter.EndTime); date = date.AddDate(0, 0, 1) {
		pattern := q.buildOrderPattern(date, filter)
		files, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}

		for _, file := range files {
			orders, err := q.readOrderFile(file, filter)
			if err != nil {
				continue
			}
			result.Orders = append(result.Orders, orders...)
		}
	}

	result.Count = len(result.Orders)
	return result, nil
}

// QueryTransfers queries transfer records
func (q *Query) QueryTransfers(filter QueryFilter) (*QueryResult, error) {
	result := &QueryResult{
		Transfers: make([]TransferRecord, 0),
	}

	// Build file pattern
	for date := filter.StartTime; date.Before(filter.EndTime); date = date.AddDate(0, 0, 1) {
		pattern := filepath.Join(q.baseDir, "transfers", date.Format("2006/01/02"), "transfers.jsonl*")
		files, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}

		for _, file := range files {
			transfers, err := q.readTransferFile(file, filter)
			if err != nil {
				continue
			}
			result.Transfers = append(result.Transfers, transfers...)
		}
	}

	result.Count = len(result.Transfers)
	return result, nil
}

// GenerateReport generates a report based on query results
func (q *Query) GenerateReport(accountID string, date time.Time) (*DailyReport, error) {
	// Query trades for the day
	filter := QueryFilter{
		AccountID: accountID,
		StartTime: date.Truncate(24 * time.Hour),
		EndTime:   date.Truncate(24 * time.Hour).Add(24 * time.Hour),
	}

	tradeResult, err := q.QueryTrades(filter)
	if err != nil {
		return nil, err
	}

	// Calculate report metrics
	report := &DailyReport{
		Date:       date.Format("2006-01-02"),
		AccountID:  accountID,
		ByStrategy: make(map[string]StrategyReport),
		BySymbol:   make(map[string]SymbolReport),
		Timestamp:  time.Now(),
	}

	// Process trades
	for _, trade := range tradeResult.Trades {
		report.TotalTrades++
		report.TotalVolume = report.TotalVolume.Add(trade.Quantity.Mul(trade.Price))
		report.Fees = report.Fees.Add(trade.Fee)

		// Update strategy stats
		if trade.Strategy != "" {
			stratReport := report.ByStrategy[trade.Strategy]
			stratReport.Trades++
			stratReport.Volume = stratReport.Volume.Add(trade.Quantity.Mul(trade.Price))
			stratReport.PnL = stratReport.PnL.Add(trade.RealizedPnL)
			if trade.RealizedPnL.IsPositive() {
				report.WinTrades++
			} else if trade.RealizedPnL.IsNegative() {
				report.LossTrades++
			}
			report.ByStrategy[trade.Strategy] = stratReport
		}

		// Update symbol stats
		symReport := report.BySymbol[trade.Symbol]
		symReport.Trades++
		symReport.Volume = symReport.Volume.Add(trade.Quantity.Mul(trade.Price))
		symReport.PnL = symReport.PnL.Add(trade.RealizedPnL)
		report.BySymbol[trade.Symbol] = symReport
	}

	// Calculate win rate
	if report.TotalTrades > 0 {
		report.WinRate = float64(report.WinTrades) / float64(report.TotalTrades)
	}

	return report, nil
}

// Private methods

func (q *Query) buildTradePattern(date time.Time, filter QueryFilter) string {
	dateStr := date.Format("2006/01/02")
	
	if filter.AccountID != "" && filter.Exchange != "" {
		return filepath.Join(q.baseDir, "logs", dateStr, filter.AccountID, filter.Exchange, "trades.jsonl*")
	} else if filter.AccountID != "" {
		return filepath.Join(q.baseDir, "logs", dateStr, filter.AccountID, "*/trades.jsonl*")
	} else {
		return filepath.Join(q.baseDir, "logs", dateStr, "*/*/trades.jsonl*")
	}
}

func (q *Query) buildOrderPattern(date time.Time, filter QueryFilter) string {
	dateStr := date.Format("2006/01/02")
	
	if filter.AccountID != "" && filter.Exchange != "" {
		return filepath.Join(q.baseDir, "logs", dateStr, filter.AccountID, filter.Exchange, "orders.jsonl*")
	} else if filter.AccountID != "" {
		return filepath.Join(q.baseDir, "logs", dateStr, filter.AccountID, "*/orders.jsonl*")
	} else {
		return filepath.Join(q.baseDir, "logs", dateStr, "*/*/orders.jsonl*")
	}
}

func (q *Query) readTradeFile(filename string, filter QueryFilter) ([]TradeRecord, error) {
	file, err := q.openFile(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var trades []TradeRecord
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		var trade TradeRecord
		if err := json.Unmarshal(scanner.Bytes(), &trade); err != nil {
			continue
		}

		// Apply filters
		if q.matchesTradeFilter(trade, filter) {
			trades = append(trades, trade)
		}
	}

	return trades, scanner.Err()
}

func (q *Query) readOrderFile(filename string, filter QueryFilter) ([]OrderRecord, error) {
	file, err := q.openFile(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var orders []OrderRecord
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		var order OrderRecord
		if err := json.Unmarshal(scanner.Bytes(), &order); err != nil {
			continue
		}

		// Apply filters
		if q.matchesOrderFilter(order, filter) {
			orders = append(orders, order)
		}
	}

	return orders, scanner.Err()
}

func (q *Query) readTransferFile(filename string, filter QueryFilter) ([]TransferRecord, error) {
	file, err := q.openFile(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var transfers []TransferRecord
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		var transfer TransferRecord
		if err := json.Unmarshal(scanner.Bytes(), &transfer); err != nil {
			continue
		}

		// Apply filters
		if q.matchesTransferFilter(transfer, filter) {
			transfers = append(transfers, transfer)
		}
	}

	return transfers, scanner.Err()
}

func (q *Query) openFile(filename string) (bufio.Reader, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	// Check if compressed
	if strings.HasSuffix(filename, ".gz") {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			file.Close()
			return nil, err
		}
		return bufio.NewReader(gzReader), nil
	}

	return bufio.NewReader(file), nil
}

func (q *Query) matchesTradeFilter(trade TradeRecord, filter QueryFilter) bool {
	if filter.AccountID != "" && trade.AccountID != filter.AccountID {
		return false
	}
	if filter.Exchange != "" && trade.Exchange != filter.Exchange {
		return false
	}
	if filter.Symbol != "" && trade.Symbol != filter.Symbol {
		return false
	}
	if filter.Strategy != "" && trade.Strategy != filter.Strategy {
		return false
	}
	if filter.Side != "" && trade.Side != filter.Side {
		return false
	}
	if !filter.MinAmount.IsZero() && trade.Quantity.Mul(trade.Price).LessThan(filter.MinAmount) {
		return false
	}
	if !filter.MaxAmount.IsZero() && trade.Quantity.Mul(trade.Price).GreaterThan(filter.MaxAmount) {
		return false
	}
	return true
}

func (q *Query) matchesOrderFilter(order OrderRecord, filter QueryFilter) bool {
	if filter.AccountID != "" && order.AccountID != filter.AccountID {
		return false
	}
	if filter.Exchange != "" && order.Exchange != filter.Exchange {
		return false
	}
	if filter.Symbol != "" && order.Symbol != filter.Symbol {
		return false
	}
	if filter.Strategy != "" && order.Strategy != filter.Strategy {
		return false
	}
	if filter.Side != "" && order.Side != filter.Side {
		return false
	}
	if filter.Status != "" && order.Status != filter.Status {
		return false
	}
	return true
}

func (q *Query) matchesTransferFilter(transfer TransferRecord, filter QueryFilter) bool {
	if filter.AccountID != "" {
		if transfer.FromAccount != filter.AccountID && transfer.ToAccount != filter.AccountID {
			return false
		}
	}
	if filter.Exchange != "" && transfer.Exchange != filter.Exchange {
		return false
	}
	if filter.Status != "" && transfer.Status != filter.Status {
		return false
	}
	return true
}

// ExportToCSV exports query results to CSV format
func (q *Query) ExportToCSV(result *QueryResult, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write header based on result type
	if len(result.Trades) > 0 {
		fmt.Fprintln(file, "ID,AccountID,Exchange,Symbol,Side,Price,Quantity,Fee,RealizedPnL,Strategy,Timestamp")
		for _, trade := range result.Trades {
			fmt.Fprintf(file, "%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\n",
				trade.ID, trade.AccountID, trade.Exchange, trade.Symbol, trade.Side,
				trade.Price.String(), trade.Quantity.String(), trade.Fee.String(),
				trade.RealizedPnL.String(), trade.Strategy, trade.Timestamp.Format(time.RFC3339))
		}
	} else if len(result.Orders) > 0 {
		fmt.Fprintln(file, "OrderID,AccountID,Exchange,Symbol,Side,Type,Price,Quantity,Status,Strategy,CreatedAt")
		for _, order := range result.Orders {
			fmt.Fprintf(file, "%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\n",
				order.OrderID, order.AccountID, order.Exchange, order.Symbol, order.Side,
				order.Type, order.Price.String(), order.Quantity.String(),
				order.Status, order.Strategy, order.CreatedAt.Format(time.RFC3339))
		}
	}

	return nil
}