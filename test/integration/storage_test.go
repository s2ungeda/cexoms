package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mExOms/internal/storage"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorageSystem(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create temporary directory for testing
	tempDir := t.TempDir()

	config := storage.StorageConfig{
		BasePath:           tempDir,
		MaxFileSize:        1024 * 1024, // 1MB
		RotationInterval:   time.Hour,
		CompressionEnabled: false, // Disable for testing
		RetentionDays:      7,
	}

	// Create storage manager
	manager, err := storage.NewManager(config)
	require.NoError(t, err)
	defer manager.Close()

	// Test account
	testAccount := "test_account"
	testExchange := "binance"

	t.Run("Trading Log Storage", func(t *testing.T) {
		// Log some trades
		for i := 0; i < 10; i++ {
			order := &types.Order{
				ID:       fmt.Sprintf("order_%d", i),
				Symbol:   "BTC/USDT",
				Side:     types.OrderSideBuy,
				Type:     types.OrderTypeLimit,
				Price:    decimal.NewFromFloat(50000 + float64(i)),
				Quantity: decimal.NewFromFloat(0.1),
				Status:   types.OrderStatusFilled,
			}

			err := manager.LogTrade(testAccount, testExchange, "BTC/USDT", "order_filled", order)
			assert.NoError(t, err)
		}

		// Flush to ensure data is written
		err = manager.Flush()
		assert.NoError(t, err)

		// Read back the logs
		opts := storage.QueryOptions{
			Account:   testAccount,
			StartTime: time.Now().Add(-time.Hour),
			EndTime:   time.Now(),
		}

		logs, err := manager.GetTradingLogs(opts)
		assert.NoError(t, err)
		assert.Len(t, logs, 10)

		// Verify log content
		firstLog := logs[0]
		assert.Equal(t, testAccount, firstLog.Account)
		assert.Equal(t, testExchange, firstLog.Exchange)
		assert.Equal(t, "BTC/USDT", firstLog.Symbol)
		assert.Equal(t, "order_filled", firstLog.Event)
	})

	t.Run("State Snapshot", func(t *testing.T) {
		// Register snapshot handler
		manager.RegisterSnapshotHandler(testAccount, func(account string) (*storage.StateSnapshot, error) {
			return &storage.StateSnapshot{
				Timestamp: time.Now(),
				Account:   account,
				Exchange:  testExchange,
				Balances: map[string]decimal.Decimal{
					"USDT": decimal.NewFromFloat(10000),
					"BTC":  decimal.NewFromFloat(0.5),
				},
				Positions: []types.Position{
					{
						Symbol:     "BTC/USDT",
						Side:       types.Side("LONG"),
						Amount:     decimal.NewFromFloat(0.5),
						EntryPrice: decimal.NewFromFloat(50000),
						MarkPrice:  decimal.NewFromFloat(51000),
					},
				},
				OpenOrders: []types.Order{},
				RiskMetrics: map[string]interface{}{
					"total_exposure": "25000",
					"open_positions": 1,
				},
			}, nil
		})

		// Take a snapshot
		err := manager.TakeSnapshot(testAccount)
		assert.NoError(t, err)

		// Flush
		err = manager.Flush()
		assert.NoError(t, err)

		// Get latest snapshot
		snapshot, err := manager.GetLatestSnapshot(testAccount)
		assert.NoError(t, err)
		assert.NotNil(t, snapshot)
		assert.Equal(t, testAccount, snapshot.Account)
		assert.Equal(t, decimal.NewFromFloat(10000), snapshot.Balances["USDT"])
	})

	t.Run("Strategy Logs", func(t *testing.T) {
		// Log strategy events
		positions := []storage.PositionDetail{
			{
				Symbol:        "ETH/USDT",
				Side:          types.Side("LONG"),
				Quantity:      decimal.NewFromFloat(10),
				EntryPrice:    decimal.NewFromFloat(3000),
				CurrentPrice:  decimal.NewFromFloat(3100),
				UnrealizedPnL: decimal.NewFromFloat(1000),
			},
		}

		performance := &storage.PerformanceMetrics{
			TotalPnL:      decimal.NewFromFloat(5000),
			WinRate:       0.65,
			SharpeRatio:   1.8,
			MaxDrawdown:   0.12,
			TotalTrades:   100,
			WinningTrades: 65,
			LosingTrades:  35,
		}

		err := manager.LogStrategy("momentum", testAccount, "signal_generated", "BUY", 0.85, positions, performance)
		assert.NoError(t, err)

		// Flush and read back
		manager.Flush()

		opts := storage.QueryOptions{
			Account:   testAccount,
			Strategy:  "momentum",
			StartTime: time.Now().Add(-time.Hour),
			EndTime:   time.Now(),
		}

		logs, err := manager.GetStrategyLogs(opts)
		assert.NoError(t, err)
		assert.Len(t, logs, 1)

		log := logs[0]
		assert.Equal(t, "momentum", log.Strategy)
		assert.Equal(t, "signal_generated", log.Event)
		assert.Equal(t, 0.85, log.Confidence)
	})

	t.Run("Transfer Logs", func(t *testing.T) {
		// Log a transfer
		err := manager.LogTransfer(
			testAccount,
			"other_account",
			testExchange,
			"bybit",
			"USDT",
			decimal.NewFromFloat(1000),
			decimal.NewFromFloat(5),
			"completed",
		)
		assert.NoError(t, err)

		// Flush and read back
		manager.Flush()

		opts := storage.QueryOptions{
			Account:   testAccount,
			StartTime: time.Now().Add(-time.Hour),
			EndTime:   time.Now(),
		}

		logs, err := manager.GetTransferLogs(opts)
		assert.NoError(t, err)
		assert.Len(t, logs, 1)

		log := logs[0]
		assert.Equal(t, testAccount, log.FromAccount)
		assert.Equal(t, "other_account", log.ToAccount)
		assert.Equal(t, decimal.NewFromFloat(1000), log.Amount)
	})

	t.Run("Query Utils", func(t *testing.T) {
		queryUtils := storage.NewQueryUtils(tempDir)

		// Test query examples
		examples := queryUtils.QueryExamples()
		assert.NotEmpty(t, examples)

		// Verify examples have both grep and jq commands
		for _, example := range examples {
			assert.NotEmpty(t, example.Description)
			assert.True(t, example.GrepCmd != "" || example.JqCmd != "")
		}
	})

	t.Run("Storage Cleaner", func(t *testing.T) {
		cleaner := storage.NewCleaner(config)

		// Get storage stats
		stats, err := cleaner.GetStorageStats()
		assert.NoError(t, err)
		assert.NotNil(t, stats)
		assert.Greater(t, stats.TotalFiles, 0)

		// Test account exists in stats
		accountStats, exists := stats.Accounts[testAccount]
		assert.True(t, exists)
		assert.Greater(t, accountStats.TotalFiles, 0)
	})

	t.Run("Account Summary", func(t *testing.T) {
		summary, err := manager.GetAccountSummary(
			testAccount,
			time.Now().Add(-time.Hour),
			time.Now(),
		)
		assert.NoError(t, err)
		assert.NotNil(t, summary)
		assert.Equal(t, testAccount, summary.Account)
		assert.Equal(t, 10, summary.TotalTrades)
		assert.Equal(t, 1, summary.TotalStrategies)
		assert.Equal(t, 1, summary.TotalTransfers)
	})

	t.Run("File Structure", func(t *testing.T) {
		// Verify directory structure
		accountDir := filepath.Join(tempDir, testAccount)
		assert.DirExists(t, accountDir)

		// Check for storage type directories
		tradingLogDir := filepath.Join(accountDir, string(storage.StorageTypeTradingLog))
		assert.DirExists(t, tradingLogDir)

		// Check for date-based subdirectories
		now := time.Now()
		yearDir := filepath.Join(tradingLogDir, fmt.Sprintf("%04d", now.Year()))
		assert.DirExists(t, yearDir)

		monthDir := filepath.Join(yearDir, fmt.Sprintf("%02d", now.Month()))
		assert.DirExists(t, monthDir)

		dayDir := filepath.Join(monthDir, fmt.Sprintf("%02d", now.Day()))
		assert.DirExists(t, dayDir)

		// Check for JSONL files
		files, err := os.ReadDir(dayDir)
		assert.NoError(t, err)
		assert.NotEmpty(t, files)

		// Verify file naming convention
		for _, file := range files {
			assert.True(t, strings.HasSuffix(file.Name(), ".jsonl") || 
				strings.HasSuffix(file.Name(), ".jsonl.gz"))
			assert.Contains(t, file.Name(), testAccount)
			assert.Contains(t, file.Name(), storage.StorageTypeTradingLog)
		}
	})
}

// Add missing imports
import (
	"fmt"
	"strings"
)