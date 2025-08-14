package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type LogReader struct {
	dataDir string
}

func NewLogReader(dataDir string) *LogReader {
	return &LogReader{
		dataDir: dataDir,
	}
}

// ReadTradeLogs reads trade logs for a specific date and symbol
func (lr *LogReader) ReadTradeLogs(date time.Time, symbol string) ([]interface{}, error) {
	path := filepath.Join(lr.dataDir, "logs", date.Format("2006/01/02"), 
		fmt.Sprintf("trades_%s.jsonl", symbol))
	
	return lr.readJSONLFile(path)
}

// ReadOrderLogs reads order logs for a specific date and symbol
func (lr *LogReader) ReadOrderLogs(date time.Time, symbol string) ([]interface{}, error) {
	path := filepath.Join(lr.dataDir, "logs", date.Format("2006/01/02"), 
		fmt.Sprintf("orders_%s.jsonl", symbol))
	
	return lr.readJSONLFile(path)
}

// ReadDateRange reads logs for a date range
func (lr *LogReader) ReadDateRange(startDate, endDate time.Time, logType, symbol string) ([]interface{}, error) {
	var allData []interface{}
	
	for date := startDate; !date.After(endDate); date = date.AddDate(0, 0, 1) {
		var data []interface{}
		var err error
		
		switch logType {
		case "trades":
			data, err = lr.ReadTradeLogs(date, symbol)
		case "orders":
			data, err = lr.ReadOrderLogs(date, symbol)
		default:
			return nil, fmt.Errorf("unknown log type: %s", logType)
		}
		
		if err != nil {
			// Skip if file doesn't exist
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		
		allData = append(allData, data...)
	}
	
	return allData, nil
}

// StreamLogs streams logs in real-time
func (lr *LogReader) StreamLogs(logPath string, handler func(interface{}) error) error {
	file, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()
	
	// Seek to end of file
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("failed to seek to end: %w", err)
	}
	
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var data interface{}
		if err := json.Unmarshal(scanner.Bytes(), &data); err != nil {
			continue // Skip invalid JSON
		}
		
		if err := handler(data); err != nil {
			return err
		}
	}
	
	return scanner.Err()
}

// readJSONLFile reads a JSON Lines file
func (lr *LogReader) readJSONLFile(path string) ([]interface{}, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	
	var data []interface{}
	scanner := bufio.NewScanner(file)
	
	for scanner.Scan() {
		var item interface{}
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			continue // Skip invalid JSON
		}
		data = append(data, item)
	}
	
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	
	return data, nil
}

// GetAvailableDates returns dates for which logs are available
func (lr *LogReader) GetAvailableDates(logType string) ([]time.Time, error) {
	logsDir := filepath.Join(lr.dataDir, "logs")
	var dates []time.Time
	
	err := filepath.Walk(logsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if info.IsDir() {
			// Try to parse directory as date
			rel, _ := filepath.Rel(logsDir, path)
			if date, err := time.Parse("2006/01/02", rel); err == nil {
				// Check if any logs exist for this date
				pattern := filepath.Join(path, fmt.Sprintf("%s_*.jsonl", logType))
				if matches, _ := filepath.Glob(pattern); len(matches) > 0 {
					dates = append(dates, date)
				}
			}
		}
		
		return nil
	})
	
	return dates, err
}

// GetSymbols returns all symbols for which logs exist on a given date
func (lr *LogReader) GetSymbols(date time.Time, logType string) ([]string, error) {
	dir := filepath.Join(lr.dataDir, "logs", date.Format("2006/01/02"))
	pattern := filepath.Join(dir, fmt.Sprintf("%s_*.jsonl", logType))
	
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	
	var symbols []string
	for _, match := range matches {
		base := filepath.Base(match)
		// Extract symbol from filename (e.g., "trades_BTCUSDT.jsonl" -> "BTCUSDT")
		if len(base) > len(logType)+6 {
			symbol := base[len(logType)+1 : len(base)-6]
			symbols = append(symbols, symbol)
		}
	}
	
	return symbols, nil
}