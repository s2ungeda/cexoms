package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type TransferStorage struct {
	dataDir    string
	mu         sync.RWMutex
	buffer     []interface{}
	bufferSize int
	flushTimer *time.Timer
}

func NewTransferStorage(dataDir string) (*TransferStorage, error) {
	transferDir := filepath.Join(dataDir, "transfers")
	if err := os.MkdirAll(transferDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create transfer directory: %w", err)
	}
	
	return &TransferStorage{
		dataDir:    dataDir,
		buffer:     make([]interface{}, 0, 100),
		bufferSize: 100,
	}, nil
}

func (ts *TransferStorage) WriteTransfer(transfer interface{}) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	
	ts.buffer = append(ts.buffer, transfer)
	
	if len(ts.buffer) >= ts.bufferSize {
		return ts.flush()
	}
	
	if ts.flushTimer == nil {
		ts.flushTimer = time.AfterFunc(10*time.Second, func() {
			ts.mu.Lock()
			defer ts.mu.Unlock()
			ts.flush()
		})
	}
	
	return nil
}

func (ts *TransferStorage) flush() error {
	if len(ts.buffer) == 0 {
		return nil
	}
	
	now := time.Now()
	filePath := ts.getTransferFilePath(now)
	
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open transfer file: %w", err)
	}
	defer file.Close()
	
	encoder := json.NewEncoder(file)
	for _, transfer := range ts.buffer {
		if err := encoder.Encode(transfer); err != nil {
			return fmt.Errorf("failed to write transfer: %w", err)
		}
	}
	
	ts.buffer = ts.buffer[:0]
	
	if ts.flushTimer != nil {
		ts.flushTimer.Stop()
		ts.flushTimer = nil
	}
	
	return nil
}

func (ts *TransferStorage) QueryTransfers(filter map[string]interface{}) ([]interface{}, error) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	
	results := make([]interface{}, 0)
	
	startTime := time.Now().AddDate(0, 0, -30)
	if start, ok := filter["start_time"].(time.Time); ok {
		startTime = start
	}
	
	endTime := time.Now()
	if end, ok := filter["end_time"].(time.Time); ok {
		endTime = end
	}
	
	for date := startTime; !date.After(endTime); date = date.AddDate(0, 0, 1) {
		filePath := ts.getTransferFilePath(date)
		
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			continue
		}
		
		file, err := os.Open(filePath)
		if err != nil {
			continue
		}
		defer file.Close()
		
		decoder := json.NewDecoder(file)
		for {
			var transfer map[string]interface{}
			if err := decoder.Decode(&transfer); err != nil {
				break
			}
			
			if ts.matchesFilter(transfer, filter) {
				results = append(results, transfer)
			}
		}
	}
	
	limit := 1000
	if l, ok := filter["limit"].(int); ok && l > 0 {
		limit = l
	}
	
	if len(results) > limit {
		results = results[len(results)-limit:]
	}
	
	return results, nil
}

func (ts *TransferStorage) GetTransferStats(exchange, accountID string, period time.Duration) (map[string]interface{}, error) {
	endTime := time.Now()
	startTime := endTime.Add(-period)
	
	filter := map[string]interface{}{
		"start_time": startTime,
		"end_time":   endTime,
	}
	
	if exchange != "" {
		filter["exchange"] = exchange
	}
	if accountID != "" {
		filter["account_id"] = accountID
	}
	
	transfers, err := ts.QueryTransfers(filter)
	if err != nil {
		return nil, err
	}
	
	stats := map[string]interface{}{
		"total_transfers": len(transfers),
		"period":          period.String(),
		"start_time":      startTime,
		"end_time":        endTime,
		"volume_by_asset": make(map[string]float64),
		"success_count":   0,
		"failed_count":    0,
		"by_type":         make(map[string]int),
	}
	
	volumeByAsset := stats["volume_by_asset"].(map[string]float64)
	byType := stats["by_type"].(map[string]int)
	
	for _, t := range transfers {
		transfer := t.(map[string]interface{})
		
		if status, ok := transfer["status"].(string); ok {
			if status == "COMPLETED" {
				stats["success_count"] = stats["success_count"].(int) + 1
			} else if status == "FAILED" {
				stats["failed_count"] = stats["failed_count"].(int) + 1
			}
		}
		
		if asset, ok := transfer["asset"].(string); ok {
			if amount, ok := transfer["amount"].(float64); ok {
				volumeByAsset[asset] += amount
			}
		}
		
		if transferType, ok := transfer["type"].(string); ok {
			byType[transferType]++
		}
	}
	
	return stats, nil
}

func (ts *TransferStorage) matchesFilter(transfer, filter map[string]interface{}) bool {
	if exchange, ok := filter["exchange"].(string); ok && exchange != "" {
		if tExchange, ok := transfer["exchange"].(string); !ok || tExchange != exchange {
			return false
		}
	}
	
	if accountID, ok := filter["account_id"].(string); ok && accountID != "" {
		fromMatch := false
		toMatch := false
		
		if from, ok := transfer["from_account"].(string); ok && from == accountID {
			fromMatch = true
		}
		if to, ok := transfer["to_account"].(string); ok && to == accountID {
			toMatch = true
		}
		
		if !fromMatch && !toMatch {
			return false
		}
	}
	
	if asset, ok := filter["asset"].(string); ok && asset != "" {
		if tAsset, ok := transfer["asset"].(string); !ok || tAsset != asset {
			return false
		}
	}
	
	if status, ok := filter["status"].(string); ok && status != "" {
		if tStatus, ok := transfer["status"].(string); !ok || tStatus != status {
			return false
		}
	}
	
	if transferType, ok := filter["type"].(string); ok && transferType != "" {
		if tType, ok := transfer["type"].(string); !ok || tType != transferType {
			return false
		}
	}
	
	return true
}

func (ts *TransferStorage) getTransferFilePath(date time.Time) string {
	return filepath.Join(
		ts.dataDir,
		"transfers",
		date.Format("2006/01"),
		fmt.Sprintf("transfers_%s.jsonl", date.Format("20060102")),
	)
}

func (ts *TransferStorage) Close() error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	
	if ts.flushTimer != nil {
		ts.flushTimer.Stop()
	}
	
	return ts.flush()
}