package storage

import (
	"time"

	"github.com/shopspring/decimal"
)

// TradeRecord represents a trade record for storage
type TradeRecord struct {
	ID           string          `json:"id"`
	AccountID    string          `json:"account_id"`
	Exchange     string          `json:"exchange"`
	Symbol       string          `json:"symbol"`
	Side         string          `json:"side"`
	Price        decimal.Decimal `json:"price"`
	Quantity     decimal.Decimal `json:"quantity"`
	Fee          decimal.Decimal `json:"fee"`
	FeeAsset     string          `json:"fee_asset"`
	OrderID      string          `json:"order_id"`
	RealizedPnL  decimal.Decimal `json:"realized_pnl"`
	Strategy     string          `json:"strategy,omitempty"`
	Timestamp    time.Time       `json:"timestamp"`
}

// OrderRecord represents an order record for storage
type OrderRecord struct {
	OrderID      string          `json:"order_id"`
	AccountID    string          `json:"account_id"`
	Exchange     string          `json:"exchange"`
	Symbol       string          `json:"symbol"`
	Side         string          `json:"side"`
	Type         string          `json:"type"`
	Price        decimal.Decimal `json:"price"`
	Quantity     decimal.Decimal `json:"quantity"`
	FilledQty    decimal.Decimal `json:"filled_qty"`
	Status       string          `json:"status"`
	Strategy     string          `json:"strategy,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	Timestamp    time.Time       `json:"timestamp"`
}

// AccountSnapshot represents a complete account state snapshot
type AccountSnapshot struct {
	AccountID    string                     `json:"account_id"`
	Exchange     string                     `json:"exchange"`
	Balances     map[string]Balance         `json:"balances"`
	Positions    map[string]Position        `json:"positions"`
	Orders       map[string]OrderRecord     `json:"orders"`
	TotalUSDT    decimal.Decimal            `json:"total_usdt"`
	FreeUSDT     decimal.Decimal            `json:"free_usdt"`
	LockedUSDT   decimal.Decimal            `json:"locked_usdt"`
	PnL          PnLSnapshot                `json:"pnl"`
	RiskMetrics  RiskSnapshot               `json:"risk_metrics"`
	Timestamp    time.Time                  `json:"timestamp"`
}

// Balance represents asset balance
type Balance struct {
	Asset     string          `json:"asset"`
	Free      decimal.Decimal `json:"free"`
	Locked    decimal.Decimal `json:"locked"`
	Total     decimal.Decimal `json:"total"`
	USDTValue decimal.Decimal `json:"usdt_value"`
}

// Position represents a trading position
type Position struct {
	Symbol       string          `json:"symbol"`
	Side         string          `json:"side"`
	Quantity     decimal.Decimal `json:"quantity"`
	EntryPrice   decimal.Decimal `json:"entry_price"`
	MarkPrice    decimal.Decimal `json:"mark_price"`
	UnrealizedPnL decimal.Decimal `json:"unrealized_pnl"`
	RealizedPnL  decimal.Decimal `json:"realized_pnl"`
	Margin       decimal.Decimal `json:"margin"`
	Leverage     int             `json:"leverage"`
}

// PnLSnapshot represents P&L at a point in time
type PnLSnapshot struct {
	TotalPnL      decimal.Decimal `json:"total_pnl"`
	TodayPnL      decimal.Decimal `json:"today_pnl"`
	WeekPnL       decimal.Decimal `json:"week_pnl"`
	MonthPnL      decimal.Decimal `json:"month_pnl"`
	UnrealizedPnL decimal.Decimal `json:"unrealized_pnl"`
	RealizedPnL   decimal.Decimal `json:"realized_pnl"`
}

// RiskSnapshot represents risk metrics at a point in time
type RiskSnapshot struct {
	TotalExposure  decimal.Decimal `json:"total_exposure"`
	MarginUsage    decimal.Decimal `json:"margin_usage"`
	MaxLeverage    int             `json:"max_leverage"`
	CurrentLeverage decimal.Decimal `json:"current_leverage"`
	RiskScore      float64         `json:"risk_score"`
	VaR95          decimal.Decimal `json:"var_95"`
	MaxDrawdown    decimal.Decimal `json:"max_drawdown"`
}

// StrategyLog represents strategy execution log
type StrategyLog struct {
	ID          string                 `json:"id"`
	StrategyID  string                 `json:"strategy_id"`
	Type        string                 `json:"type"` // decision, action, error, info
	Action      string                 `json:"action"`
	AccountID   string                 `json:"account_id"`
	Symbol      string                 `json:"symbol,omitempty"`
	Details     map[string]interface{} `json:"details"`
	Metrics     map[string]float64     `json:"metrics,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
}

// TransferRecord represents an account transfer record
type TransferRecord struct {
	ID          string          `json:"id"`
	FromAccount string          `json:"from_account"`
	ToAccount   string          `json:"to_account"`
	Exchange    string          `json:"exchange"`
	Asset       string          `json:"asset"`
	Amount      decimal.Decimal `json:"amount"`
	Status      string          `json:"status"`
	TxID        string          `json:"tx_id,omitempty"`
	Fee         decimal.Decimal `json:"fee,omitempty"`
	Reason      string          `json:"reason"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	RequestedAt time.Time       `json:"requested_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	Timestamp   time.Time       `json:"timestamp"`
}

// DailyReport represents daily trading report
type DailyReport struct {
	Date        string                    `json:"date"`
	AccountID   string                    `json:"account_id"`
	StartBalance decimal.Decimal           `json:"start_balance"`
	EndBalance  decimal.Decimal           `json:"end_balance"`
	PnL         decimal.Decimal           `json:"pnl"`
	PnLPercent  decimal.Decimal           `json:"pnl_percent"`
	TotalTrades int                       `json:"total_trades"`
	WinTrades   int                       `json:"win_trades"`
	LossTrades  int                       `json:"loss_trades"`
	WinRate     float64                   `json:"win_rate"`
	TotalVolume decimal.Decimal           `json:"total_volume"`
	Fees        decimal.Decimal           `json:"fees"`
	ByStrategy  map[string]StrategyReport `json:"by_strategy"`
	BySymbol    map[string]SymbolReport   `json:"by_symbol"`
	Timestamp   time.Time                 `json:"timestamp"`
}

// StrategyReport represents strategy-specific daily report
type StrategyReport struct {
	PnL         decimal.Decimal `json:"pnl"`
	Trades      int             `json:"trades"`
	WinRate     float64         `json:"win_rate"`
	Volume      decimal.Decimal `json:"volume"`
	SharpeRatio float64         `json:"sharpe_ratio"`
}

// SymbolReport represents symbol-specific daily report
type SymbolReport struct {
	PnL     decimal.Decimal `json:"pnl"`
	Trades  int             `json:"trades"`
	Volume  decimal.Decimal `json:"volume"`
	AvgWin  decimal.Decimal `json:"avg_win"`
	AvgLoss decimal.Decimal `json:"avg_loss"`
}