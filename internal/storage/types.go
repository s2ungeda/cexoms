package storage

import (
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// StorageType represents different types of storage data
type StorageType string

const (
	StorageTypeTradingLog     StorageType = "trading_log"
	StorageTypeStateSnapshot  StorageType = "state_snapshot"
	StorageTypeStrategyLog    StorageType = "strategy_log"
	StorageTypeTransferLog    StorageType = "transfer_log"
	StorageTypeRiskLog        StorageType = "risk_log"
)

// TradingLog represents a single trading event
type TradingLog struct {
	ID          string            `json:"id"`
	Timestamp   time.Time         `json:"timestamp"`
	Account     string            `json:"account"`
	Exchange    string            `json:"exchange"`
	Symbol      string            `json:"symbol"`
	Event       string            `json:"event"` // "order_placed", "order_filled", "order_canceled", etc.
	OrderID     string            `json:"order_id,omitempty"`
	Side        types.OrderSide   `json:"side,omitempty"`
	Type        types.OrderType   `json:"type,omitempty"`
	Price       decimal.Decimal   `json:"price,omitempty"`
	Quantity    decimal.Decimal   `json:"quantity,omitempty"`
	Status      types.OrderStatus `json:"status,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// StateSnapshot represents a snapshot of account state
type StateSnapshot struct {
	Timestamp   time.Time                      `json:"timestamp"`
	Account     string                         `json:"account"`
	Exchange    string                         `json:"exchange"`
	Balances    map[string]decimal.Decimal     `json:"balances"`
	Positions   []types.Position               `json:"positions"`
	OpenOrders  []types.Order                  `json:"open_orders"`
	RiskMetrics map[string]interface{}         `json:"risk_metrics"`
	Metadata    map[string]interface{}         `json:"metadata"`
}

// StrategyLog represents strategy execution events
type StrategyLog struct {
	ID           string                 `json:"id"`
	Timestamp    time.Time              `json:"timestamp"`
	Strategy     string                 `json:"strategy"`
	Account      string                 `json:"account"`
	Event        string                 `json:"event"` // "signal_generated", "entry", "exit", "rebalance", etc.
	Signal       string                 `json:"signal,omitempty"`
	Confidence   float64                `json:"confidence,omitempty"`
	Positions    []PositionDetail       `json:"positions,omitempty"`
	Performance  PerformanceMetrics     `json:"performance,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// TransferLog represents inter-account transfers
type TransferLog struct {
	ID            string          `json:"id"`
	Timestamp     time.Time       `json:"timestamp"`
	FromAccount   string          `json:"from_account"`
	ToAccount     string          `json:"to_account"`
	FromExchange  string          `json:"from_exchange,omitempty"`
	ToExchange    string          `json:"to_exchange,omitempty"`
	Asset         string          `json:"asset"`
	Amount        decimal.Decimal `json:"amount"`
	TransactionID string          `json:"transaction_id,omitempty"`
	Status        string          `json:"status"` // "pending", "completed", "failed"
	Fee           decimal.Decimal `json:"fee,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// PositionDetail contains detailed position information
type PositionDetail struct {
	Symbol       string          `json:"symbol"`
	Side         types.Side      `json:"side"`
	Quantity     decimal.Decimal `json:"quantity"`
	EntryPrice   decimal.Decimal `json:"entry_price"`
	CurrentPrice decimal.Decimal `json:"current_price"`
	UnrealizedPnL decimal.Decimal `json:"unrealized_pnl"`
}

// PerformanceMetrics contains strategy performance metrics
type PerformanceMetrics struct {
	TotalPnL      decimal.Decimal `json:"total_pnl"`
	WinRate       float64         `json:"win_rate"`
	SharpeRatio   float64         `json:"sharpe_ratio"`
	MaxDrawdown   float64         `json:"max_drawdown"`
	TotalTrades   int             `json:"total_trades"`
	WinningTrades int             `json:"winning_trades"`
	LosingTrades  int             `json:"losing_trades"`
}

// StorageConfig contains configuration for storage
type StorageConfig struct {
	BasePath           string        `json:"base_path"`
	MaxFileSize        int64         `json:"max_file_size"`        // in bytes
	RotationInterval   time.Duration `json:"rotation_interval"`
	CompressionEnabled bool          `json:"compression_enabled"`
	RetentionDays      int           `json:"retention_days"`
}

// QueryOptions represents options for querying stored data
type QueryOptions struct {
	StartTime   time.Time
	EndTime     time.Time
	Account     string
	Exchange    string
	Symbol      string
	Event       string
	Strategy    string
	Limit       int
	Offset      int
}