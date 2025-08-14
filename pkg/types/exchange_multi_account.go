package types

import (
	"context"
	"time"
	
	"github.com/shopspring/decimal"
)

// ExchangeMultiAccount extends the Exchange interface with multi-account support
type ExchangeMultiAccount interface {
	Exchange
	
	// Multi-account support
	SetAccount(accountID string) error
	GetCurrentAccount() string
	SupportSubAccounts() bool
	
	// Account-specific operations
	GetBalanceForAccount(ctx context.Context, accountID string) (*Balance, error)
	GetPositionsForAccount(ctx context.Context, accountID string) ([]*Position, error)
	GetOpenOrdersForAccount(ctx context.Context, accountID, symbol string) ([]*Order, error)
	
	// Sub-account management
	ListSubAccounts(ctx context.Context) ([]*SubAccountInfo, error)
	CreateSubAccount(ctx context.Context, name string) (*SubAccountInfo, error)
	EnableSubAccountTrading(ctx context.Context, subAccountID string, permissions TradingPermissions) error
	
	// Asset transfers
	TransferBetweenAccounts(ctx context.Context, transfer *AccountTransferRequest) (*AccountTransferResponse, error)
	GetTransferHistory(ctx context.Context, accountID string, limit int) ([]*TransferRecord, error)
}

// SubAccountInfo represents sub-account information
type SubAccountInfo struct {
	AccountID    string    `json:"account_id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	Type         string    `json:"type"`
	IsVirtual    bool      `json:"is_virtual"`
	CreateTime   time.Time `json:"create_time"`
	
	// Trading permissions
	SpotEnabled    bool `json:"spot_enabled"`
	FuturesEnabled bool `json:"futures_enabled"`
	MarginEnabled  bool `json:"margin_enabled"`
	
	// API key info
	APIKeyCreated bool      `json:"api_key_created"`
	APIKeyExpiry  time.Time `json:"api_key_expiry,omitempty"`
	
	// Status
	Active       bool `json:"active"`
	Locked       bool `json:"locked"`
	VIPLevel     int  `json:"vip_level"`
}

// TradingPermissions defines trading permissions for sub-accounts
type TradingPermissions struct {
	EnableSpot    bool `json:"enable_spot"`
	EnableFutures bool `json:"enable_futures"`
	EnableMargin  bool `json:"enable_margin"`
	EnableOptions bool `json:"enable_options"`
	
	// IP restrictions
	IPRestrict    bool     `json:"ip_restrict"`
	IPWhitelist   []string `json:"ip_whitelist,omitempty"`
	
	// Withdrawal permissions
	EnableWithdrawal bool            `json:"enable_withdrawal"`
	WithdrawalLimit  decimal.Decimal `json:"withdrawal_limit,omitempty"`
}

// AccountTransferRequest represents a transfer request between accounts
type AccountTransferRequest struct {
	FromAccountID   string          `json:"from_account_id"`
	ToAccountID     string          `json:"to_account_id"`
	Asset           string          `json:"asset"`
	Amount          decimal.Decimal `json:"amount"`
	
	// Optional fields
	FromAccountType string `json:"from_account_type,omitempty"` // SPOT, FUTURES, etc.
	ToAccountType   string `json:"to_account_type,omitempty"`
	TransferID      string `json:"transfer_id,omitempty"`       // Client transfer ID
}

// AccountTransferResponse represents the response of a transfer
type AccountTransferResponse struct {
	TransferID   string          `json:"transfer_id"`
	Status       string          `json:"status"`
	Amount       decimal.Decimal `json:"amount"`
	Asset        string          `json:"asset"`
	FromAccount  string          `json:"from_account"`
	ToAccount    string          `json:"to_account"`
	TransferTime time.Time       `json:"transfer_time"`
	TxID         string          `json:"tx_id,omitempty"`
}

// TransferRecord represents a historical transfer record
type TransferRecord struct {
	ID           string          `json:"id"`
	Asset        string          `json:"asset"`
	Amount       decimal.Decimal `json:"amount"`
	Type         string          `json:"type"` // IN, OUT
	Status       string          `json:"status"`
	FromAccount  string          `json:"from_account"`
	ToAccount    string          `json:"to_account"`
	TransferTime time.Time       `json:"transfer_time"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
}

// AccountRateLimits represents rate limits for a specific account
type AccountRateLimits struct {
	AccountID       string    `json:"account_id"`
	WeightUsed      int       `json:"weight_used"`
	WeightLimit     int       `json:"weight_limit"`
	OrdersUsed      int       `json:"orders_used"`
	OrdersLimit     int       `json:"orders_limit"`
	WindowStartTime time.Time `json:"window_start_time"`
	WindowEndTime   time.Time `json:"window_end_time"`
}

// MultiAccountBalance represents balances across multiple accounts
type MultiAccountBalance struct {
	Accounts      map[string]*Balance     `json:"accounts"`
	TotalUSDT     decimal.Decimal         `json:"total_usdt"`
	Distribution  map[string]decimal.Decimal `json:"distribution"` // Percentage by account
	LastUpdated   time.Time               `json:"last_updated"`
}

// MultiAccountPosition represents positions across multiple accounts
type MultiAccountPosition struct {
	Accounts     map[string][]*Position  `json:"accounts"`
	TotalNotional decimal.Decimal        `json:"total_notional"`
	NetExposure   decimal.Decimal        `json:"net_exposure"`
	LastUpdated   time.Time              `json:"last_updated"`
}