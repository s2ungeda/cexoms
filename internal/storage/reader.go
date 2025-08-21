package storage

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Reader handles reading data from storage files
type Reader struct {
	config StorageConfig
}

// NewReader creates a new storage reader
func NewReader(config StorageConfig) *Reader {
	return &Reader{
		config: config,
	}
}

// ReadTradingLogs reads trading logs based on query options
func (r *Reader) ReadTradingLogs(opts QueryOptions) ([]TradingLog, error) {
	files, err := r.findFiles(opts, StorageTypeTradingLog)
	if err != nil {
		return nil, err
	}

	var logs []TradingLog
	for _, file := range files {
		fileLogs, err := r.readTradingLogsFromFile(file, opts)
		if err != nil {
			// Log error but continue with other files
			fmt.Printf("Error reading file %s: %v\n", file, err)
			continue
		}
		logs = append(logs, fileLogs...)
	}

	// Apply limit and offset
	return applyPagination(logs, opts.Limit, opts.Offset), nil
}

// ReadStateSnapshots reads state snapshots
func (r *Reader) ReadStateSnapshots(opts QueryOptions) ([]StateSnapshot, error) {
	files, err := r.findFiles(opts, StorageTypeStateSnapshot)
	if err != nil {
		return nil, err
	}

	var snapshots []StateSnapshot
	for _, file := range files {
		fileSnapshots, err := r.readStateSnapshotsFromFile(file, opts)
		if err != nil {
			fmt.Printf("Error reading file %s: %v\n", file, err)
			continue
		}
		snapshots = append(snapshots, fileSnapshots...)
	}

	return applyPagination(snapshots, opts.Limit, opts.Offset), nil
}

// ReadStrategyLogs reads strategy logs
func (r *Reader) ReadStrategyLogs(opts QueryOptions) ([]StrategyLog, error) {
	files, err := r.findFiles(opts, StorageTypeStrategyLog)
	if err != nil {
		return nil, err
	}

	var logs []StrategyLog
	for _, file := range files {
		fileLogs, err := r.readStrategyLogsFromFile(file, opts)
		if err != nil {
			fmt.Printf("Error reading file %s: %v\n", file, err)
			continue
		}
		logs = append(logs, fileLogs...)
	}

	return applyPagination(logs, opts.Limit, opts.Offset), nil
}

// ReadTransferLogs reads transfer logs
func (r *Reader) ReadTransferLogs(opts QueryOptions) ([]TransferLog, error) {
	files, err := r.findFiles(opts, StorageTypeTransferLog)
	if err != nil {
		return nil, err
	}

	var logs []TransferLog
	for _, file := range files {
		fileLogs, err := r.readTransferLogsFromFile(file, opts)
		if err != nil {
			fmt.Printf("Error reading file %s: %v\n", file, err)
			continue
		}
		logs = append(logs, fileLogs...)
	}

	return applyPagination(logs, opts.Limit, opts.Offset), nil
}

// GetLatestSnapshot returns the most recent state snapshot for an account
func (r *Reader) GetLatestSnapshot(account string) (*StateSnapshot, error) {
	opts := QueryOptions{
		Account:   account,
		StartTime: time.Now().Add(-24 * time.Hour),
		EndTime:   time.Now(),
		Limit:     1,
	}

	snapshots, err := r.ReadStateSnapshots(opts)
	if err != nil {
		return nil, err
	}

	if len(snapshots) == 0 {
		return nil, fmt.Errorf("no snapshots found for account %s", account)
	}

	// Return the most recent one
	latest := &snapshots[0]
	for i := 1; i < len(snapshots); i++ {
		if snapshots[i].Timestamp.After(latest.Timestamp) {
			latest = &snapshots[i]
		}
	}

	return latest, nil
}

// findFiles finds relevant files based on query options
func (r *Reader) findFiles(opts QueryOptions, storageType StorageType) ([]string, error) {
	var files []string

	// If account is specified, look in specific account directory
	if opts.Account != "" {
		accountPath := filepath.Join(r.config.BasePath, opts.Account, string(storageType))
		if err := r.findFilesInPath(accountPath, opts.StartTime, opts.EndTime, &files); err != nil {
			return nil, err
		}
	} else {
		// Search all accounts
		entries, err := os.ReadDir(r.config.BasePath)
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			if entry.IsDir() {
				accountPath := filepath.Join(r.config.BasePath, entry.Name(), string(storageType))
				r.findFilesInPath(accountPath, opts.StartTime, opts.EndTime, &files)
			}
		}
	}

	return files, nil
}

// findFilesInPath recursively finds files in a path within the time range
func (r *Reader) findFilesInPath(basePath string, startTime, endTime time.Time, files *[]string) error {
	// Walk through year/month/day structure
	err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip directories that can't be accessed
		}

		if info.IsDir() {
			// Check if directory is within date range
			if !r.isDirectoryInRange(path, basePath, startTime, endTime) {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file matches our pattern
		if strings.HasSuffix(path, ".jsonl") || strings.HasSuffix(path, ".jsonl.gz") {
			// Extract timestamp from filename
			if r.isFileInRange(info.Name(), startTime, endTime) {
				*files = append(*files, path)
			}
		}

		return nil
	})

	return err
}

// isDirectoryInRange checks if a directory might contain files in the date range
func (r *Reader) isDirectoryInRange(dirPath, basePath string, startTime, endTime time.Time) bool {
	// Extract date components from directory path
	rel, err := filepath.Rel(basePath, dirPath)
	if err != nil {
		return true // If we can't determine, include it
	}

	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 {
		return true
	}

	// Try to parse year/month/day from path
	if len(parts) >= 1 {
		year := parseIntFromString(parts[0])
		if year > 0 {
			if year < startTime.Year() || year > endTime.Year() {
				return false
			}
		}
	}

	if len(parts) >= 2 {
		month := parseIntFromString(parts[1])
		if month > 0 {
			// More complex logic for month checking would go here
			// For simplicity, we'll include it if the year matches
		}
	}

	return true
}

// isFileInRange checks if a file is within the time range based on its name
func (r *Reader) isFileInRange(filename string, startTime, endTime time.Time) bool {
	// Extract timestamp from filename
	// Format: account_type_YYYYMMDD_HHMMSS.jsonl[.gz]
	parts := strings.Split(filename, "_")
	if len(parts) < 4 {
		return true // If we can't parse, include it
	}

	// Get date and time parts
	dateStr := parts[len(parts)-2]
	timeStr := strings.Split(parts[len(parts)-1], ".")[0]

	// Parse timestamp
	timestamp, err := time.Parse("20060102150405", dateStr+timeStr)
	if err != nil {
		return true // If we can't parse, include it
	}

	return !timestamp.Before(startTime) && !timestamp.After(endTime)
}

// readTradingLogsFromFile reads trading logs from a single file
func (r *Reader) readTradingLogsFromFile(filepath string, opts QueryOptions) ([]TradingLog, error) {
	reader, cleanup, err := r.openFile(filepath)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	var logs []TradingLog
	scanner := bufio.NewScanner(reader)
	
	for scanner.Scan() {
		var log TradingLog
		if err := json.Unmarshal(scanner.Bytes(), &log); err != nil {
			continue // Skip malformed lines
		}

		// Apply filters
		if r.matchesTradingLogFilters(&log, opts) {
			logs = append(logs, log)
		}
	}

	return logs, scanner.Err()
}

// readStateSnapshotsFromFile reads state snapshots from a single file
func (r *Reader) readStateSnapshotsFromFile(filepath string, opts QueryOptions) ([]StateSnapshot, error) {
	reader, cleanup, err := r.openFile(filepath)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	var snapshots []StateSnapshot
	scanner := bufio.NewScanner(reader)
	
	for scanner.Scan() {
		var snapshot StateSnapshot
		if err := json.Unmarshal(scanner.Bytes(), &snapshot); err != nil {
			continue
		}

		// Apply filters
		if r.matchesStateSnapshotFilters(&snapshot, opts) {
			snapshots = append(snapshots, snapshot)
		}
	}

	return snapshots, scanner.Err()
}

// readStrategyLogsFromFile reads strategy logs from a single file
func (r *Reader) readStrategyLogsFromFile(filepath string, opts QueryOptions) ([]StrategyLog, error) {
	reader, cleanup, err := r.openFile(filepath)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	var logs []StrategyLog
	scanner := bufio.NewScanner(reader)
	
	for scanner.Scan() {
		var log StrategyLog
		if err := json.Unmarshal(scanner.Bytes(), &log); err != nil {
			continue
		}

		// Apply filters
		if r.matchesStrategyLogFilters(&log, opts) {
			logs = append(logs, log)
		}
	}

	return logs, scanner.Err()
}

// readTransferLogsFromFile reads transfer logs from a single file
func (r *Reader) readTransferLogsFromFile(filepath string, opts QueryOptions) ([]TransferLog, error) {
	reader, cleanup, err := r.openFile(filepath)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	var logs []TransferLog
	scanner := bufio.NewScanner(reader)
	
	for scanner.Scan() {
		var log TransferLog
		if err := json.Unmarshal(scanner.Bytes(), &log); err != nil {
			continue
		}

		// Apply filters
		if r.matchesTransferLogFilters(&log, opts) {
			logs = append(logs, log)
		}
	}

	return logs, scanner.Err()
}

// openFile opens a file, handling compression if needed
func (r *Reader) openFile(filepath string) (io.Reader, func(), error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, nil, err
	}

	if strings.HasSuffix(filepath, ".gz") {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			file.Close()
			return nil, nil, err
		}
		
		cleanup := func() {
			gzReader.Close()
			file.Close()
		}
		
		return gzReader, cleanup, nil
	}

	cleanup := func() {
		file.Close()
	}

	return file, cleanup, nil
}

// Filter matching functions

func (r *Reader) matchesTradingLogFilters(log *TradingLog, opts QueryOptions) bool {
	if !log.Timestamp.IsZero() {
		if log.Timestamp.Before(opts.StartTime) || log.Timestamp.After(opts.EndTime) {
			return false
		}
	}

	if opts.Account != "" && log.Account != opts.Account {
		return false
	}

	if opts.Exchange != "" && log.Exchange != opts.Exchange {
		return false
	}

	if opts.Symbol != "" && log.Symbol != opts.Symbol {
		return false
	}

	if opts.Event != "" && log.Event != opts.Event {
		return false
	}

	return true
}

func (r *Reader) matchesStateSnapshotFilters(snapshot *StateSnapshot, opts QueryOptions) bool {
	if !snapshot.Timestamp.IsZero() {
		if snapshot.Timestamp.Before(opts.StartTime) || snapshot.Timestamp.After(opts.EndTime) {
			return false
		}
	}

	if opts.Account != "" && snapshot.Account != opts.Account {
		return false
	}

	if opts.Exchange != "" && snapshot.Exchange != opts.Exchange {
		return false
	}

	return true
}

func (r *Reader) matchesStrategyLogFilters(log *StrategyLog, opts QueryOptions) bool {
	if !log.Timestamp.IsZero() {
		if log.Timestamp.Before(opts.StartTime) || log.Timestamp.After(opts.EndTime) {
			return false
		}
	}

	if opts.Account != "" && log.Account != opts.Account {
		return false
	}

	if opts.Strategy != "" && log.Strategy != opts.Strategy {
		return false
	}

	if opts.Event != "" && log.Event != opts.Event {
		return false
	}

	return true
}

func (r *Reader) matchesTransferLogFilters(log *TransferLog, opts QueryOptions) bool {
	if !log.Timestamp.IsZero() {
		if log.Timestamp.Before(opts.StartTime) || log.Timestamp.After(opts.EndTime) {
			return false
		}
	}

	if opts.Account != "" && log.FromAccount != opts.Account && log.ToAccount != opts.Account {
		return false
	}

	return true
}

// Helper functions

func parseIntFromString(s string) int {
	var val int
	fmt.Sscanf(s, "%d", &val)
	return val
}

func applyPagination[T any](items []T, limit, offset int) []T {
	if offset >= len(items) {
		return []T{}
	}

	end := len(items)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}

	return items[offset:end]
}