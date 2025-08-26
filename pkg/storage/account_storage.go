package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mExOms/pkg/types"
)

// AccountStorage handles account-specific file storage
type AccountStorage struct {
	mu              sync.RWMutex
	baseStorage     *FileStorage
	baseDir         string
	buffers         map[string]*AccountBuffer
	rotators        *RotationManager
	transferStorage *TransferStorage
}

// AccountBuffer holds account-specific buffered data
type AccountBuffer struct {
	mu          sync.Mutex
	trades      []TradeRecord
	orders      []OrderRecord
	strategies  []StrategyLog
	lastFlush   time.Time
	maxSize     int
}

// NewAccountStorage creates a new account-specific storage
func NewAccountStorage(baseDir string) (*AccountStorage, error) {
	// Create base storage
	baseStorage, err := NewFileStorage(baseDir)
	if err != nil {
		return nil, err
	}

	// Create rotation manager
	rotationConfig := RotationConfig{
		MaxFileSize:     100 * 1024 * 1024, // 100MB
		RotationPeriod:  24 * time.Hour,
		CompressionAge:  7 * 24 * time.Hour,
		RetentionPeriod: 30 * 24 * time.Hour,
	}
	rotators := NewRotationManager(rotationConfig)

	// Create transfer storage
	transferStorage, err := NewTransferStorage(baseDir)
	if err != nil {
		return nil, err
	}

	as := &AccountStorage{
		baseStorage:     baseStorage,
		baseDir:         baseDir,
		buffers:         make(map[string]*AccountBuffer),
		rotators:        rotators,
		transferStorage: transferStorage,
	}

	// Start background tasks
	go as.periodicFlush()
	go as.hourlySnapshot()

	return as, nil
}

// LogAccountTrade logs a trade for a specific account
func (as *AccountStorage) LogAccountTrade(accountID string, trade TradeRecord) error {
	as.mu.Lock()
	buffer := as.getOrCreateBuffer(accountID)
	as.mu.Unlock()

	buffer.mu.Lock()
	buffer.trades = append(buffer.trades, trade)
	shouldFlush := len(buffer.trades) >= buffer.maxSize
	buffer.mu.Unlock()

	if shouldFlush {
		return as.flushAccountTrades(accountID)
	}

	return nil
}

// LogAccountOrder logs an order for a specific account
func (as *AccountStorage) LogAccountOrder(accountID string, order OrderRecord) error {
	as.mu.Lock()
	buffer := as.getOrCreateBuffer(accountID)
	as.mu.Unlock()

	buffer.mu.Lock()
	buffer.orders = append(buffer.orders, order)
	shouldFlush := len(buffer.orders) >= buffer.maxSize
	buffer.mu.Unlock()

	if shouldFlush {
		return as.flushAccountOrders(accountID)
	}

	return nil
}

// LogStrategyExecution logs strategy execution for an account
func (as *AccountStorage) LogStrategyExecution(accountID string, log StrategyLog) error {
	as.mu.Lock()
	buffer := as.getOrCreateBuffer(accountID)
	as.mu.Unlock()

	buffer.mu.Lock()
	buffer.strategies = append(buffer.strategies, log)
	shouldFlush := len(buffer.strategies) >= buffer.maxSize
	buffer.mu.Unlock()

	if shouldFlush {
		return as.flushStrategyLogs(accountID)
	}

	return nil
}

// SaveAccountSnapshot saves a complete account snapshot
func (as *AccountStorage) SaveAccountSnapshot(snapshot AccountSnapshot) error {
	timestamp := snapshot.Timestamp
	dir := filepath.Join(as.baseDir, "snapshots",
		timestamp.Format("2006/01/02"),
		fmt.Sprintf("%02d", timestamp.Hour()),
		snapshot.AccountID)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	filename := filepath.Join(dir, fmt.Sprintf("state_%s.json", timestamp.Format("150405")))
	
	// Check for rotation
	if err := as.rotators.CheckAndRotate(filename); err != nil {
		return fmt.Errorf("failed to rotate snapshot file: %w", err)
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	// Atomic write
	tmpFile := filename + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write snapshot: %w", err)
	}

	return os.Rename(tmpFile, filename)
}

// LogTransfer logs an account transfer
func (as *AccountStorage) LogTransfer(transfer TransferRecord) error {
	date := transfer.Timestamp.Format("2006/01/02")
	filename := filepath.Join(as.baseDir, "transfers", date, "transfers.jsonl")

	// Ensure directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create transfer directory: %w", err)
	}

	// Check for rotation
	if err := as.rotators.CheckAndRotate(filename); err != nil {
		return fmt.Errorf("failed to rotate transfer file: %w", err)
	}

	// Append to file
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open transfer file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(transfer)
}

// SaveDailyReport saves daily trading report
func (as *AccountStorage) SaveDailyReport(accountID string, report DailyReport) error {
	dir := filepath.Join(as.baseDir, "reports", report.Date[:7], accountID) // YYYY/MM/account_id
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create report directory: %w", err)
	}

	filename := filepath.Join(dir, fmt.Sprintf("daily_%s.json", report.Date))

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	return os.WriteFile(filename, data, 0644)
}

// GetAccountTrades retrieves trades for an account within date range
func (as *AccountStorage) GetAccountTrades(accountID string, start, end time.Time) ([]TradeRecord, error) {
	var allTrades []TradeRecord

	for date := start; date.Before(end); date = date.AddDate(0, 0, 1) {
		dateStr := date.Format("2006/01/02")
		pattern := filepath.Join(as.baseDir, "logs", dateStr, accountID, "*/trades.jsonl*")
		
		files, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}

		for _, file := range files {
			trades, err := as.readTradeFile(file)
			if err != nil {
				continue
			}
			allTrades = append(allTrades, trades...)
		}
	}

	return allTrades, nil
}

// GetLatestSnapshot retrieves the latest snapshot for an account
func (as *AccountStorage) GetLatestSnapshot(accountID string) (*AccountSnapshot, error) {
	// Search for latest snapshot
	basePattern := filepath.Join(as.baseDir, "snapshots", "*/*/*", accountID, "state_*.json")
	files, err := filepath.Glob(basePattern)
	if err != nil {
		return nil, fmt.Errorf("failed to search snapshots: %w", err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no snapshots found for account %s", accountID)
	}

	// Get the latest file (files are sorted by name due to timestamp format)
	latestFile := files[len(files)-1]

	data, err := os.ReadFile(latestFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read snapshot: %w", err)
	}

	var snapshot AccountSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}

	return &snapshot, nil
}

// Private methods

func (as *AccountStorage) getOrCreateBuffer(accountID string) *AccountBuffer {
	if buffer, exists := as.buffers[accountID]; exists {
		return buffer
	}

	buffer := &AccountBuffer{
		trades:     make([]TradeRecord, 0, 100),
		orders:     make([]OrderRecord, 0, 100),
		strategies: make([]StrategyLog, 0, 100),
		lastFlush:  time.Now(),
		maxSize:    100,
	}
	as.buffers[accountID] = buffer
	return buffer
}

func (as *AccountStorage) flushAccountTrades(accountID string) error {
	as.mu.RLock()
	buffer, exists := as.buffers[accountID]
	as.mu.RUnlock()

	if !exists || len(buffer.trades) == 0 {
		return nil
	}

	buffer.mu.Lock()
	trades := make([]TradeRecord, len(buffer.trades))
	copy(trades, buffer.trades)
	buffer.trades = buffer.trades[:0]
	buffer.lastFlush = time.Now()
	buffer.mu.Unlock()

	// Write trades to file
	date := time.Now().Format("2006/01/02")
	exchange := ""
	if len(trades) > 0 {
		exchange = trades[0].Exchange
	}
	
	filename := filepath.Join(as.baseDir, "logs", date, accountID, exchange, "trades.jsonl")
	return as.appendToFile(filename, trades)
}

func (as *AccountStorage) flushAccountOrders(accountID string) error {
	as.mu.RLock()
	buffer, exists := as.buffers[accountID]
	as.mu.RUnlock()

	if !exists || len(buffer.orders) == 0 {
		return nil
	}

	buffer.mu.Lock()
	orders := make([]OrderRecord, len(buffer.orders))
	copy(orders, buffer.orders)
	buffer.orders = buffer.orders[:0]
	buffer.lastFlush = time.Now()
	buffer.mu.Unlock()

	// Write orders to file
	date := time.Now().Format("2006/01/02")
	exchange := ""
	if len(orders) > 0 {
		exchange = orders[0].Exchange
	}
	
	filename := filepath.Join(as.baseDir, "logs", date, accountID, exchange, "orders.jsonl")
	return as.appendToFile(filename, orders)
}

func (as *AccountStorage) flushStrategyLogs(accountID string) error {
	as.mu.RLock()
	buffer, exists := as.buffers[accountID]
	as.mu.RUnlock()

	if !exists || len(buffer.strategies) == 0 {
		return nil
	}

	buffer.mu.Lock()
	logs := make([]StrategyLog, len(buffer.strategies))
	copy(logs, buffer.strategies)
	buffer.strategies = buffer.strategies[:0]
	buffer.lastFlush = time.Now()
	buffer.mu.Unlock()

	// Write strategy logs to file
	date := time.Now().Format("2006/01/02")
	strategyID := ""
	if len(logs) > 0 {
		strategyID = logs[0].StrategyID
	}
	
	filename := filepath.Join(as.baseDir, "strategies", date, strategyID, "executions.jsonl")
	return as.appendToFile(filename, logs)
}

func (as *AccountStorage) appendToFile(filename string, records interface{}) error {
	// Ensure directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Check for rotation
	if err := as.rotators.CheckAndRotate(filename); err != nil {
		return fmt.Errorf("failed to rotate file: %w", err)
	}

	// Open file
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Write records
	encoder := json.NewEncoder(file)
	
	switch v := records.(type) {
	case []TradeRecord:
		for _, record := range v {
			if err := encoder.Encode(record); err != nil {
				return err
			}
		}
	case []OrderRecord:
		for _, record := range v {
			if err := encoder.Encode(record); err != nil {
				return err
			}
		}
	case []StrategyLog:
		for _, record := range v {
			if err := encoder.Encode(record); err != nil {
				return err
			}
		}
	}

	return nil
}

func (as *AccountStorage) readTradeFile(filename string) ([]TradeRecord, error) {
	// Implementation would handle both regular and compressed files
	// Similar to the base FileStorage implementation
	return nil, nil
}

// Background tasks

func (as *AccountStorage) periodicFlush() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		as.mu.RLock()
		accountIDs := make([]string, 0, len(as.buffers))
		for id := range as.buffers {
			accountIDs = append(accountIDs, id)
		}
		as.mu.RUnlock()

		for _, accountID := range accountIDs {
			as.flushAccountTrades(accountID)
			as.flushAccountOrders(accountID)
			as.flushStrategyLogs(accountID)
		}
	}
}

func (as *AccountStorage) hourlySnapshot() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		// In production, this would trigger snapshot creation
		// for all active accounts
	}
}

// WriteTransfer writes a transfer record
func (as *AccountStorage) WriteTransfer(transfer interface{}) error {
	if as.transferStorage != nil {
		return as.transferStorage.WriteTransfer(transfer)
	}
	return fmt.Errorf("transfer storage not initialized")
}

// Close flushes all buffers and closes the storage
func (as *AccountStorage) Close() error {
	// Flush all buffers
	as.mu.RLock()
	accountIDs := make([]string, 0, len(as.buffers))
	for id := range as.buffers {
		accountIDs = append(accountIDs, id)
	}
	as.mu.RUnlock()

	for _, accountID := range accountIDs {
		as.flushAccountTrades(accountID)
		as.flushAccountOrders(accountID)
		as.flushStrategyLogs(accountID)
	}

	// Close transfer storage
	if as.transferStorage != nil {
		as.transferStorage.Close()
	}

	return as.baseStorage.Close()
}