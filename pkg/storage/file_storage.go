package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	
	"github.com/mExOms/oms/pkg/types"
)

type FileStorage struct {
	dataDir    string
	mu         sync.RWMutex
	buffers    map[string]*Buffer
	flushTicker *time.Ticker
}

type Buffer struct {
	data []interface{}
	mu   sync.Mutex
}

func NewFileStorage(dataDir string) (*FileStorage, error) {
	// Create data directories
	dirs := []string{
		filepath.Join(dataDir, "logs"),
		filepath.Join(dataDir, "snapshots"),
		filepath.Join(dataDir, "reports"),
	}
	
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	
	fs := &FileStorage{
		dataDir:     dataDir,
		buffers:     make(map[string]*Buffer),
		flushTicker: time.NewTicker(5 * time.Second),
	}
	
	// Start background flush
	go fs.backgroundFlush()
	
	return fs, nil
}

func (fs *FileStorage) LogTrade(trade *types.Trade) error {
	fs.mu.Lock()
	bufferKey := fs.getTradeLogPath(trade.Symbol)
	
	if _, exists := fs.buffers[bufferKey]; !exists {
		fs.buffers[bufferKey] = &Buffer{
			data: make([]interface{}, 0, 1000),
		}
	}
	buffer := fs.buffers[bufferKey]
	fs.mu.Unlock()
	
	buffer.mu.Lock()
	buffer.data = append(buffer.data, trade)
	shouldFlush := len(buffer.data) >= 1000
	buffer.mu.Unlock()
	
	if shouldFlush {
		return fs.flushBuffer(bufferKey)
	}
	
	return nil
}

func (fs *FileStorage) LogOrder(order *types.OrderResponse) error {
	fs.mu.Lock()
	bufferKey := fs.getOrderLogPath(order.Symbol)
	
	if _, exists := fs.buffers[bufferKey]; !exists {
		fs.buffers[bufferKey] = &Buffer{
			data: make([]interface{}, 0, 1000),
		}
	}
	buffer := fs.buffers[bufferKey]
	fs.mu.Unlock()
	
	buffer.mu.Lock()
	buffer.data = append(buffer.data, order)
	shouldFlush := len(buffer.data) >= 1000
	buffer.mu.Unlock()
	
	if shouldFlush {
		return fs.flushBuffer(bufferKey)
	}
	
	return nil
}

func (fs *FileStorage) SaveSnapshot(state interface{}) error {
	now := time.Now()
	path := fs.getSnapshotPath(now)
	
	// Create directory if not exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create snapshot directory: %w", err)
	}
	
	// Write snapshot
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}
	
	return os.WriteFile(path, data, 0644)
}

func (fs *FileStorage) LoadSnapshot(timestamp time.Time) (interface{}, error) {
	path := fs.getSnapshotPath(timestamp)
	
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read snapshot: %w", err)
	}
	
	var state interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}
	
	return state, nil
}

func (fs *FileStorage) SaveReport(reportType string, data interface{}) error {
	now := time.Now()
	filename := fmt.Sprintf("%s_%s.json", reportType, now.Format("20060102_150405"))
	path := filepath.Join(fs.dataDir, "reports", now.Format("2006/01"), filename)
	
	// Create directory if not exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create report directory: %w", err)
	}
	
	// Write report
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}
	
	return os.WriteFile(path, jsonData, 0644)
}

func (fs *FileStorage) backgroundFlush() {
	for range fs.flushTicker.C {
		fs.mu.RLock()
		keys := make([]string, 0, len(fs.buffers))
		for k := range fs.buffers {
			keys = append(keys, k)
		}
		fs.mu.RUnlock()
		
		for _, key := range keys {
			fs.flushBuffer(key)
		}
	}
}

func (fs *FileStorage) flushBuffer(bufferKey string) error {
	fs.mu.RLock()
	buffer, exists := fs.buffers[bufferKey]
	fs.mu.RUnlock()
	
	if !exists {
		return nil
	}
	
	buffer.mu.Lock()
	if len(buffer.data) == 0 {
		buffer.mu.Unlock()
		return nil
	}
	
	// Copy data and clear buffer
	data := make([]interface{}, len(buffer.data))
	copy(data, buffer.data)
	buffer.data = buffer.data[:0]
	buffer.mu.Unlock()
	
	// Create directory if not exists
	dir := filepath.Dir(bufferKey)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}
	
	// Open file in append mode
	file, err := os.OpenFile(bufferKey, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()
	
	// Write data as JSONL (JSON Lines)
	encoder := json.NewEncoder(file)
	for _, item := range data {
		if err := encoder.Encode(item); err != nil {
			return fmt.Errorf("failed to write log entry: %w", err)
		}
	}
	
	return nil
}

func (fs *FileStorage) getTradeLogPath(symbol string) string {
	now := time.Now()
	return filepath.Join(fs.dataDir, "logs", now.Format("2006/01/02"), 
		fmt.Sprintf("trades_%s.jsonl", symbol))
}

func (fs *FileStorage) getOrderLogPath(symbol string) string {
	now := time.Now()
	return filepath.Join(fs.dataDir, "logs", now.Format("2006/01/02"), 
		fmt.Sprintf("orders_%s.jsonl", symbol))
}

func (fs *FileStorage) getSnapshotPath(timestamp time.Time) string {
	return filepath.Join(fs.dataDir, "snapshots", 
		timestamp.Format("2006/01/02/15"), 
		fmt.Sprintf("state_%s.json", timestamp.Format("150405")))
}

func (fs *FileStorage) Close() error {
	// Stop background flush
	fs.flushTicker.Stop()
	
	// Flush all buffers
	fs.mu.RLock()
	keys := make([]string, 0, len(fs.buffers))
	for k := range fs.buffers {
		keys = append(keys, k)
	}
	fs.mu.RUnlock()
	
	for _, key := range keys {
		if err := fs.flushBuffer(key); err != nil {
			return err
		}
	}
	
	return nil
}