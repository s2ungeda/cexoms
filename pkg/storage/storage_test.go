package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
	
	"fmt"
	
	"github.com/mExOms/oms/pkg/types"
)

func TestFileStorage(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "storage_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	
	// Create file storage
	fs, err := NewFileStorage(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()
	
	// Test trade logging
	trade := &types.Trade{
		ID:           "12345",
		Symbol:       "BTCUSDT",
		Price:        "30000.00",
		Quantity:     "0.001",
		Time:         time.Now().Unix(),
		IsBuyerMaker: true,
	}
	
	if err := fs.LogTrade(trade); err != nil {
		t.Errorf("Failed to log trade: %v", err)
	}
	
	// Test order logging
	order := &types.OrderResponse{
		OrderID:      "67890",
		Symbol:       "BTCUSDT",
		Side:         "BUY",
		Type:         "LIMIT",
		Status:       "FILLED",
		Price:        "30000.00",
		Quantity:     "0.001",
		ExecutedQty:  "0.001",
		TransactTime: time.Now().Unix(),
	}
	
	if err := fs.LogOrder(order); err != nil {
		t.Errorf("Failed to log order: %v", err)
	}
	
	// Force flush
	fs.flushBuffer(fs.getTradeLogPath("BTCUSDT"))
	fs.flushBuffer(fs.getOrderLogPath("BTCUSDT"))
	
	// Verify files exist
	tradePath := filepath.Join(tempDir, "logs", time.Now().Format("2006/01/02"), "trades_BTCUSDT.jsonl")
	if _, err := os.Stat(tradePath); os.IsNotExist(err) {
		t.Error("Trade log file not created")
	}
	
	orderPath := filepath.Join(tempDir, "logs", time.Now().Format("2006/01/02"), "orders_BTCUSDT.jsonl")
	if _, err := os.Stat(orderPath); os.IsNotExist(err) {
		t.Error("Order log file not created")
	}
	
	// Test snapshot
	state := map[string]interface{}{
		"positions": map[string]interface{}{
			"BTCUSDT": map[string]interface{}{
				"symbol":   "BTCUSDT",
				"quantity": "0.1",
				"price":    "30000.00",
			},
		},
		"timestamp": time.Now().Unix(),
	}
	
	if err := fs.SaveSnapshot(state); err != nil {
		t.Errorf("Failed to save snapshot: %v", err)
	}
	
	// Test report
	report := map[string]interface{}{
		"date":         time.Now().Format("2006-01-02"),
		"total_trades": 100,
		"pnl":         "1234.56",
		"fees":        "12.34",
	}
	
	if err := fs.SaveReport("daily_pnl", report); err != nil {
		t.Errorf("Failed to save report: %v", err)
	}
}

func TestLogReader(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "reader_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	
	// Create file storage and log some data
	fs, err := NewFileStorage(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	
	// Log multiple trades
	for i := 0; i < 10; i++ {
		trade := &types.Trade{
			ID:           fmt.Sprintf("%d", i),
			Symbol:       "BTCUSDT",
			Price:        "30000.00",
			Quantity:     "0.001",
			Time:         time.Now().Unix(),
			IsBuyerMaker: i%2 == 0,
		}
		fs.LogTrade(trade)
	}
	
	// Force flush
	fs.flushBuffer(fs.getTradeLogPath("BTCUSDT"))
	fs.Close()
	
	// Create reader and test
	reader := NewLogReader(tempDir)
	
	// Read trades
	trades, err := reader.ReadTradeLogs(time.Now(), "BTCUSDT")
	if err != nil {
		t.Errorf("Failed to read trade logs: %v", err)
	}
	
	if len(trades) != 10 {
		t.Errorf("Expected 10 trades, got %d", len(trades))
	}
	
	// Test date range
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -1)
	allTrades, err := reader.ReadDateRange(startDate, endDate, "trades", "BTCUSDT")
	if err != nil {
		t.Errorf("Failed to read date range: %v", err)
	}
	
	if len(allTrades) != 10 {
		t.Errorf("Expected 10 trades in date range, got %d", len(allTrades))
	}
	
	// Test available dates
	dates, err := reader.GetAvailableDates("trades")
	if err != nil {
		t.Errorf("Failed to get available dates: %v", err)
	}
	
	if len(dates) != 1 {
		t.Errorf("Expected 1 available date, got %d", len(dates))
	}
	
	// Test symbols
	symbols, err := reader.GetSymbols(time.Now(), "trades")
	if err != nil {
		t.Errorf("Failed to get symbols: %v", err)
	}
	
	if len(symbols) != 1 || symbols[0] != "BTCUSDT" {
		t.Errorf("Expected [BTCUSDT], got %v", symbols)
	}
}

func TestLogRotator(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "rotator_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	
	// Create old log file
	oldLogDir := filepath.Join(tempDir, "logs", "2023/01/01")
	os.MkdirAll(oldLogDir, 0755)
	oldLogPath := filepath.Join(oldLogDir, "trades_BTCUSDT.jsonl")
	os.WriteFile(oldLogPath, []byte("{}\n"), 0644)
	
	// Set modification time to 10 days ago
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	os.Chtimes(oldLogPath, oldTime, oldTime)
	
	// Create rotator with 7 day retention and 1 day compression
	rotator := NewLogRotator(tempDir, 7, 1)
	
	// Run rotation
	if err := rotator.RotateLogs(); err != nil {
		t.Errorf("Failed to rotate logs: %v", err)
	}
	
	// Verify old file was deleted (older than 7 days)
	if _, err := os.Stat(oldLogPath); !os.IsNotExist(err) {
		t.Error("Old log file should have been deleted")
	}
	
	// Create a file that should be compressed
	recentLogDir := filepath.Join(tempDir, "logs", time.Now().Add(-2*24*time.Hour).Format("2006/01/02"))
	os.MkdirAll(recentLogDir, 0755)
	recentLogPath := filepath.Join(recentLogDir, "trades_ETHUSDT.jsonl")
	os.WriteFile(recentLogPath, []byte("{}\n"), 0644)
	
	// Set modification time to 2 days ago
	recentTime := time.Now().Add(-2 * 24 * time.Hour)
	os.Chtimes(recentLogPath, recentTime, recentTime)
	
	// Run rotation again
	if err := rotator.RotateLogs(); err != nil {
		t.Errorf("Failed to rotate logs: %v", err)
	}
	
	// Verify file was compressed
	if _, err := os.Stat(recentLogPath + ".gz"); os.IsNotExist(err) {
		t.Error("Recent log file should have been compressed")
	}
	
	// Verify original was removed
	if _, err := os.Stat(recentLogPath); !os.IsNotExist(err) {
		t.Error("Original log file should have been removed after compression")
	}
}

func BenchmarkFileStorage(b *testing.B) {
	tempDir, _ := os.MkdirTemp("", "bench_test")
	defer os.RemoveAll(tempDir)
	
	fs, _ := NewFileStorage(tempDir)
	defer fs.Close()
	
	trade := &types.Trade{
		ID:           "12345",
		Symbol:       "BTCUSDT",
		Price:        "30000.00",
		Quantity:     "0.001",
		Time:         time.Now().Unix(),
		IsBuyerMaker: true,
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fs.LogTrade(trade)
	}
}