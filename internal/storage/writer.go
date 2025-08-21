package storage

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Writer handles writing data to storage files
type Writer struct {
	mu              sync.RWMutex
	config          StorageConfig
	writers         map[string]*fileWriter // key: account_type (e.g., "account1_trading_log")
	rotationTicker  *time.Ticker
	compressionPool sync.Pool
}

// fileWriter represents a single file writer
type fileWriter struct {
	mu           sync.Mutex
	file         *os.File
	writer       *bufio.Writer
	path         string
	bytesWritten int64
	lastRotation time.Time
	isCompressed bool
	gzWriter     *gzip.Writer
}

// NewWriter creates a new storage writer
func NewWriter(config StorageConfig) (*Writer, error) {
	// Create base directory if it doesn't exist
	if err := os.MkdirAll(config.BasePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	w := &Writer{
		config:  config,
		writers: make(map[string]*fileWriter),
		compressionPool: sync.Pool{
			New: func() interface{} {
				return &gzip.Writer{}
			},
		},
	}

	// Start rotation ticker
	if config.RotationInterval > 0 {
		w.rotationTicker = time.NewTicker(config.RotationInterval)
		go w.rotationLoop()
	}

	return w, nil
}

// WriteTradingLog writes a trading log entry
func (w *Writer) WriteTradingLog(log TradingLog) error {
	key := fmt.Sprintf("%s_%s", log.Account, StorageTypeTradingLog)
	data, err := json.Marshal(log)
	if err != nil {
		return fmt.Errorf("failed to marshal trading log: %w", err)
	}

	return w.write(key, log.Account, StorageTypeTradingLog, data)
}

// WriteStateSnapshot writes a state snapshot
func (w *Writer) WriteStateSnapshot(snapshot StateSnapshot) error {
	key := fmt.Sprintf("%s_%s", snapshot.Account, StorageTypeStateSnapshot)
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("failed to marshal state snapshot: %w", err)
	}

	return w.write(key, snapshot.Account, StorageTypeStateSnapshot, data)
}

// WriteStrategyLog writes a strategy log entry
func (w *Writer) WriteStrategyLog(log StrategyLog) error {
	key := fmt.Sprintf("%s_%s", log.Account, StorageTypeStrategyLog)
	data, err := json.Marshal(log)
	if err != nil {
		return fmt.Errorf("failed to marshal strategy log: %w", err)
	}

	return w.write(key, log.Account, StorageTypeStrategyLog, data)
}

// WriteTransferLog writes a transfer log entry
func (w *Writer) WriteTransferLog(log TransferLog) error {
	// Use from_account as the primary key
	key := fmt.Sprintf("%s_%s", log.FromAccount, StorageTypeTransferLog)
	data, err := json.Marshal(log)
	if err != nil {
		return fmt.Errorf("failed to marshal transfer log: %w", err)
	}

	return w.write(key, log.FromAccount, StorageTypeTransferLog, data)
}

// write handles the actual writing to file
func (w *Writer) write(key, account string, storageType StorageType, data []byte) error {
	w.mu.Lock()
	fw, exists := w.writers[key]
	if !exists {
		var err error
		fw, err = w.createFileWriter(account, storageType)
		if err != nil {
			w.mu.Unlock()
			return err
		}
		w.writers[key] = fw
	}
	w.mu.Unlock()

	fw.mu.Lock()
	defer fw.mu.Unlock()

	// Check if rotation is needed
	if w.shouldRotate(fw) {
		if err := w.rotateFile(fw, account, storageType); err != nil {
			return fmt.Errorf("failed to rotate file: %w", err)
		}
	}

	// Write data with newline for JSONL format
	data = append(data, '\n')
	
	var n int
	var err error
	if fw.gzWriter != nil {
		n, err = fw.gzWriter.Write(data)
	} else {
		n, err = fw.writer.Write(data)
	}
	
	if err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	fw.bytesWritten += int64(n)

	// Flush the buffer
	if fw.gzWriter != nil {
		err = fw.gzWriter.Flush()
	} else {
		err = fw.writer.Flush()
	}
	
	return err
}

// createFileWriter creates a new file writer
func (w *Writer) createFileWriter(account string, storageType StorageType) (*fileWriter, error) {
	// Create directory structure: base_path/account/type/YYYY/MM/DD/
	now := time.Now()
	dir := filepath.Join(
		w.config.BasePath,
		account,
		string(storageType),
		fmt.Sprintf("%04d", now.Year()),
		fmt.Sprintf("%02d", now.Month()),
		fmt.Sprintf("%02d", now.Day()),
	)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Create filename with timestamp
	filename := fmt.Sprintf("%s_%s_%s.jsonl",
		account,
		storageType,
		now.Format("20060102_150405"),
	)

	if w.config.CompressionEnabled {
		filename += ".gz"
	}

	path := filepath.Join(dir, filename)

	// Open file
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	fw := &fileWriter{
		file:         file,
		path:         path,
		lastRotation: now,
		isCompressed: w.config.CompressionEnabled,
	}

	if w.config.CompressionEnabled {
		fw.gzWriter = gzip.NewWriter(file)
		fw.writer = bufio.NewWriter(fw.gzWriter)
	} else {
		fw.writer = bufio.NewWriter(file)
	}

	return fw, nil
}

// shouldRotate checks if file rotation is needed
func (w *Writer) shouldRotate(fw *fileWriter) bool {
	// Check file size
	if w.config.MaxFileSize > 0 && fw.bytesWritten >= w.config.MaxFileSize {
		return true
	}

	// Check time interval
	if w.config.RotationInterval > 0 && time.Since(fw.lastRotation) >= w.config.RotationInterval {
		return true
	}

	// Check if it's a new day
	return fw.lastRotation.Day() != time.Now().Day()
}

// rotateFile rotates the current file
func (w *Writer) rotateFile(fw *fileWriter, account string, storageType StorageType) error {
	// Flush and close current file
	if fw.gzWriter != nil {
		fw.gzWriter.Close()
	}
	fw.writer.Flush()
	fw.file.Close()

	// Create new file writer
	newFw, err := w.createFileWriter(account, storageType)
	if err != nil {
		return err
	}

	// Update the file writer
	fw.file = newFw.file
	fw.writer = newFw.writer
	fw.gzWriter = newFw.gzWriter
	fw.path = newFw.path
	fw.bytesWritten = 0
	fw.lastRotation = time.Now()

	return nil
}

// rotationLoop handles periodic file rotation
func (w *Writer) rotationLoop() {
	for range w.rotationTicker.C {
		w.mu.RLock()
		writers := make(map[string]*fileWriter)
		for k, v := range w.writers {
			writers[k] = v
		}
		w.mu.RUnlock()

		for key, fw := range writers {
			fw.mu.Lock()
			if w.shouldRotate(fw) {
				// Extract account and storage type from key
				// Format: account_storageType
				parts := extractKeyParts(key)
				if len(parts) >= 2 {
					account := parts[0]
					storageType := StorageType(parts[1])
					w.rotateFile(fw, account, storageType)
				}
			}
			fw.mu.Unlock()
		}
	}
}

// Close closes all file writers
func (w *Writer) Close() error {
	if w.rotationTicker != nil {
		w.rotationTicker.Stop()
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	var lastErr error
	for _, fw := range w.writers {
		fw.mu.Lock()
		if fw.gzWriter != nil {
			if err := fw.gzWriter.Close(); err != nil {
				lastErr = err
			}
		}
		if err := fw.writer.Flush(); err != nil {
			lastErr = err
		}
		if err := fw.file.Close(); err != nil {
			lastErr = err
		}
		fw.mu.Unlock()
	}

	return lastErr
}

// Flush flushes all buffers
func (w *Writer) Flush() error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var lastErr error
	for _, fw := range w.writers {
		fw.mu.Lock()
		if fw.gzWriter != nil {
			if err := fw.gzWriter.Flush(); err != nil {
				lastErr = err
			}
		}
		if err := fw.writer.Flush(); err != nil {
			lastErr = err
		}
		fw.mu.Unlock()
	}

	return lastErr
}

// extractKeyParts extracts account and storage type from key
func extractKeyParts(key string) []string {
	// Simple implementation - in production, use more robust parsing
	// Expected format: account_storageType
	// Find the last underscore to separate account from storage type
	lastUnderscore := -1
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '_' {
			lastUnderscore = i
			break
		}
	}

	if lastUnderscore > 0 {
		return []string{key[:lastUnderscore], key[lastUnderscore+1:]}
	}

	return []string{key}
}