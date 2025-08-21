package storage

import (
	"fmt"
	"sync"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/robfig/cron/v3"
	"github.com/shopspring/decimal"
)

// Manager manages storage operations
type Manager struct {
	mu              sync.RWMutex
	writer          *Writer
	reader          *Reader
	config          StorageConfig
	snapshotCron    *cron.Cron
	cleanupCron     *cron.Cron
	snapshotHandlers map[string]SnapshotHandler // account -> handler
}

// SnapshotHandler is a function that provides snapshot data for an account
type SnapshotHandler func(account string) (*StateSnapshot, error)

// NewManager creates a new storage manager
func NewManager(config StorageConfig) (*Manager, error) {
	writer, err := NewWriter(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create writer: %w", err)
	}

	reader := NewReader(config)

	m := &Manager{
		writer:           writer,
		reader:           reader,
		config:           config,
		snapshotHandlers: make(map[string]SnapshotHandler),
	}

	// Setup cron jobs
	if err := m.setupCronJobs(); err != nil {
		return nil, fmt.Errorf("failed to setup cron jobs: %w", err)
	}

	return m, nil
}

// setupCronJobs sets up scheduled tasks
func (m *Manager) setupCronJobs() error {
	// Hourly snapshots
	m.snapshotCron = cron.New()
	_, err := m.snapshotCron.AddFunc("0 * * * *", m.takeSnapshots) // Every hour
	if err != nil {
		return fmt.Errorf("failed to add snapshot cron: %w", err)
	}
	m.snapshotCron.Start()

	// Daily cleanup
	m.cleanupCron = cron.New()
	_, err = m.cleanupCron.AddFunc("0 2 * * *", m.cleanupOldFiles) // 2 AM daily
	if err != nil {
		return fmt.Errorf("failed to add cleanup cron: %w", err)
	}
	m.cleanupCron.Start()

	return nil
}

// RegisterSnapshotHandler registers a handler to provide snapshot data for an account
func (m *Manager) RegisterSnapshotHandler(account string, handler SnapshotHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.snapshotHandlers[account] = handler
}

// LogTrade logs a trading event
func (m *Manager) LogTrade(account, exchange, symbol, event string, order *types.Order) error {
	log := TradingLog{
		ID:        generateID(),
		Timestamp: time.Now(),
		Account:   account,
		Exchange:  exchange,
		Symbol:    symbol,
		Event:     event,
	}

	if order != nil {
		log.OrderID = order.ID
		log.Side = order.Side
		log.Type = order.Type
		log.Price = order.Price
		log.Quantity = order.Quantity
		log.Status = order.Status
		log.Metadata = order.Metadata
	}

	return m.writer.WriteTradingLog(log)
}

// LogStrategy logs a strategy execution event
func (m *Manager) LogStrategy(strategy, account, event, signal string, confidence float64, positions []PositionDetail, performance *PerformanceMetrics) error {
	log := StrategyLog{
		ID:         generateID(),
		Timestamp:  time.Now(),
		Strategy:   strategy,
		Account:    account,
		Event:      event,
		Signal:     signal,
		Confidence: confidence,
		Positions:  positions,
	}

	if performance != nil {
		log.Performance = *performance
	}

	return m.writer.WriteStrategyLog(log)
}

// LogTransfer logs an inter-account transfer
func (m *Manager) LogTransfer(fromAccount, toAccount, fromExchange, toExchange, asset string, amount, fee decimal.Decimal, status string) error {
	log := TransferLog{
		ID:           generateID(),
		Timestamp:    time.Now(),
		FromAccount:  fromAccount,
		ToAccount:    toAccount,
		FromExchange: fromExchange,
		ToExchange:   toExchange,
		Asset:        asset,
		Amount:       amount,
		Status:       status,
		Fee:          fee,
	}

	return m.writer.WriteTransferLog(log)
}

// TakeSnapshot manually triggers a snapshot for an account
func (m *Manager) TakeSnapshot(account string) error {
	m.mu.RLock()
	handler, exists := m.snapshotHandlers[account]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no snapshot handler registered for account %s", account)
	}

	snapshot, err := handler(account)
	if err != nil {
		return fmt.Errorf("failed to get snapshot data: %w", err)
	}

	return m.writer.WriteStateSnapshot(*snapshot)
}

// takeSnapshots is called by cron to take snapshots of all accounts
func (m *Manager) takeSnapshots() {
	m.mu.RLock()
	handlers := make(map[string]SnapshotHandler)
	for k, v := range m.snapshotHandlers {
		handlers[k] = v
	}
	m.mu.RUnlock()

	for account, handler := range handlers {
		snapshot, err := handler(account)
		if err != nil {
			fmt.Printf("Failed to take snapshot for account %s: %v\n", account, err)
			continue
		}

		if err := m.writer.WriteStateSnapshot(*snapshot); err != nil {
			fmt.Printf("Failed to write snapshot for account %s: %v\n", account, err)
		}
	}
}

// cleanupOldFiles removes files older than retention period
func (m *Manager) cleanupOldFiles() {
	if m.config.RetentionDays <= 0 {
		return // No cleanup if retention is not set
	}

	cutoffTime := time.Now().AddDate(0, 0, -m.config.RetentionDays)
	
	// Implementation would walk through directories and remove old files
	// This is a placeholder - actual implementation would be more complex
	fmt.Printf("Cleaning up files older than %s\n", cutoffTime.Format("2006-01-02"))
}

// Query methods delegate to reader

// GetTradingLogs retrieves trading logs
func (m *Manager) GetTradingLogs(opts QueryOptions) ([]TradingLog, error) {
	return m.reader.ReadTradingLogs(opts)
}

// GetStateSnapshots retrieves state snapshots
func (m *Manager) GetStateSnapshots(opts QueryOptions) ([]StateSnapshot, error) {
	return m.reader.ReadStateSnapshots(opts)
}

// GetStrategyLogs retrieves strategy logs
func (m *Manager) GetStrategyLogs(opts QueryOptions) ([]StrategyLog, error) {
	return m.reader.ReadStrategyLogs(opts)
}

// GetTransferLogs retrieves transfer logs
func (m *Manager) GetTransferLogs(opts QueryOptions) ([]TransferLog, error) {
	return m.reader.ReadTransferLogs(opts)
}

// GetLatestSnapshot returns the most recent snapshot for an account
func (m *Manager) GetLatestSnapshot(account string) (*StateSnapshot, error) {
	return m.reader.GetLatestSnapshot(account)
}

// GetAccountSummary returns a summary of account activity
func (m *Manager) GetAccountSummary(account string, startTime, endTime time.Time) (*AccountSummary, error) {
	// Get trading logs
	tradingOpts := QueryOptions{
		Account:   account,
		StartTime: startTime,
		EndTime:   endTime,
	}
	
	tradingLogs, err := m.reader.ReadTradingLogs(tradingOpts)
	if err != nil {
		return nil, err
	}

	// Get strategy logs
	strategyLogs, err := m.reader.ReadStrategyLogs(tradingOpts)
	if err != nil {
		return nil, err
	}

	// Get transfer logs
	transferLogs, err := m.reader.ReadTransferLogs(tradingOpts)
	if err != nil {
		return nil, err
	}

	// Calculate summary
	summary := &AccountSummary{
		Account:          account,
		StartTime:        startTime,
		EndTime:          endTime,
		TotalTrades:      len(tradingLogs),
		TotalStrategies:  len(getUniqueStrategies(strategyLogs)),
		TotalTransfers:   len(transferLogs),
	}

	// Calculate volumes and fees
	for _, log := range tradingLogs {
		if log.Event == "order_filled" {
			volume := log.Price.Mul(log.Quantity)
			summary.TotalVolume = summary.TotalVolume.Add(volume)
		}
	}

	for _, log := range transferLogs {
		summary.TotalTransferVolume = summary.TotalTransferVolume.Add(log.Amount)
		summary.TotalFees = summary.TotalFees.Add(log.Fee)
	}

	return summary, nil
}

// Close closes the storage manager
func (m *Manager) Close() error {
	if m.snapshotCron != nil {
		m.snapshotCron.Stop()
	}
	if m.cleanupCron != nil {
		m.cleanupCron.Stop()
	}
	return m.writer.Close()
}

// Flush flushes all pending writes
func (m *Manager) Flush() error {
	return m.writer.Flush()
}

// AccountSummary represents a summary of account activity
type AccountSummary struct {
	Account             string
	StartTime           time.Time
	EndTime             time.Time
	TotalTrades         int
	TotalVolume         decimal.Decimal
	TotalStrategies     int
	TotalTransfers      int
	TotalTransferVolume decimal.Decimal
	TotalFees           decimal.Decimal
}

// Helper functions

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func getUniqueStrategies(logs []StrategyLog) []string {
	seen := make(map[string]bool)
	var strategies []string
	
	for _, log := range logs {
		if !seen[log.Strategy] {
			seen[log.Strategy] = true
			strategies = append(strategies, log.Strategy)
		}
	}
	
	return strategies
}