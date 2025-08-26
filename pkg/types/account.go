package types

import (
	"time"

	"github.com/shopspring/decimal"
)

// AccountType represents the type of trading account
type AccountType string

const (
	AccountTypeMain     AccountType = "main"
	AccountTypeSub      AccountType = "sub"
	AccountTypeStrategy AccountType = "strategy"
)

// Account represents a trading account across exchanges
type Account struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Type        AccountType     `json:"type"`
	Exchange    string          `json:"exchange"`
	Market      MarketType      `json:"market"`
	Parent      string          `json:"parent,omitempty"`
	Strategy    string          `json:"strategy,omitempty"`
	VaultPath   string          `json:"vault_path,omitempty"`
	
	// Trading permissions
	SpotEnabled    bool `json:"spot_enabled"`
	FuturesEnabled bool `json:"futures_enabled"`
	MarginEnabled  bool `json:"margin_enabled"`
	
	// Risk limits
	MaxBalance     decimal.Decimal `json:"max_balance"`
	MaxPositionUSDT decimal.Decimal `json:"max_position_usdt"`
	MaxLeverage     int             `json:"max_leverage"`
	DailyLossLimit  decimal.Decimal `json:"daily_loss_limit"`
	
	// Rate limiting
	RateLimitWeight   int `json:"rate_limit_weight"`
	RateLimitOrders   int `json:"rate_limit_orders"`
	
	// Status
	Active      bool      `json:"active"`
	LastUsed    time.Time `json:"last_used"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	
	// Metadata for additional information
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	
	// Cached balance (internal use)
	cachedBalance *AccountBalance `json:"-"`
}

// GetBalance returns the cached balance for this account
// In production, this would fetch from the account manager
func (a *Account) GetBalance() (*AccountBalance, error) {
	if a.cachedBalance != nil {
		return a.cachedBalance, nil
	}
	// Return empty balance if not cached
	return &AccountBalance{
		AccountID: a.ID,
		Exchange:  a.Exchange,
		TotalUSDT: decimal.Zero,
		UpdatedAt: time.Now(),
	}, nil
}

// AccountBalance represents account balance information
type AccountBalance struct {
	AccountID     string                     `json:"account_id"`
	Exchange      string                     `json:"exchange"`
	Balances      map[string]*Balance        `json:"balances"`
	TotalUSDT     decimal.Decimal            `json:"total_usdt"`
	Available     decimal.Decimal            `json:"available"`
	Locked        decimal.Decimal            `json:"locked"`
	TotalExposure decimal.Decimal            `json:"total_exposure"`
	UpdateTime    time.Time                  `json:"update_time"`
	UpdatedAt     time.Time                  `json:"updated_at"`
}

// AccountPosition represents positions for an account
type AccountPosition struct {
	AccountID string                `json:"account_id"`
	Exchange  string                `json:"exchange"`
	Positions map[string]*Position  `json:"positions"`
	UpdatedAt time.Time             `json:"updated_at"`
}

// AccountMetrics represents performance metrics for an account
type AccountMetrics struct {
	AccountID       string          `json:"account_id"`
	TotalPnL        decimal.Decimal `json:"total_pnl"`
	TodayPnL        decimal.Decimal `json:"today_pnl"`
	WinRate         float64         `json:"win_rate"`
	TotalTrades     int             `json:"total_trades"`
	OpenPositions   int             `json:"open_positions"`
	UsedWeight      int             `json:"used_weight"`
	RemainingWeight int             `json:"remaining_weight"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// AccountTransfer represents a transfer between accounts
type AccountTransfer struct {
	ID                 string          `json:"id"`
	FromAccount        string          `json:"from_account"`
	ToAccount          string          `json:"to_account"`
	Exchange           string          `json:"exchange"`
	Asset              string          `json:"asset"`
	Amount             decimal.Decimal `json:"amount"`
	Status             string          `json:"status"`
	Reason             string          `json:"reason,omitempty"`
	ExchangeTransferID string          `json:"exchange_transfer_id,omitempty"`
	TxID               string          `json:"tx_id,omitempty"`
	ErrorMessage       string          `json:"error_message,omitempty"`
	RequestedAt        time.Time       `json:"requested_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	CompletedAt        time.Time       `json:"completed_at"`
	CreatedAt          time.Time       `json:"created_at"`
}

// AccountSelector is an interface for selecting accounts based on criteria
type AccountSelector interface {
	// SelectAccount selects the best account for a given strategy and requirements
	SelectAccount(strategy string, requirements AccountRequirements) (*Account, error)
	
	// SelectAccountForOrder selects the best account for a specific order
	SelectAccountForOrder(order *Order) (*Account, error)
	
	// GetAccount retrieves a specific account by ID
	GetAccount(accountID string) (*Account, error)
	
	// ListAccounts lists all accounts matching criteria
	ListAccounts(filter AccountFilter) ([]*Account, error)
	
	// UpdateAccountMetrics updates account usage metrics
	UpdateAccountMetrics(accountID string, metrics AccountMetrics) error
}

// AccountRequirements specifies requirements for account selection
type AccountRequirements struct {
	MinBalance      decimal.Decimal
	MaxBalance      decimal.Decimal
	RequiredWeight  int
	Market          MarketType
	Symbol          string
	OrderSize       decimal.Decimal
	Leverage        int
}

// AccountFilter specifies filter criteria for listing accounts
type AccountFilter struct {
	Exchange    string
	Type        AccountType
	Strategy    string
	Active      *bool
	Market      MarketType
	MinBalance  decimal.Decimal
}

// AccountManager manages accounts across exchanges
type AccountManager interface {
	AccountSelector
	
	// CreateAccount creates a new account
	CreateAccount(account *Account) error
	
	// UpdateAccount updates account information
	UpdateAccount(account *Account) error
	
	// DeleteAccount deletes an account
	DeleteAccount(accountID string) error
	
	// GetBalance retrieves account balance
	GetBalance(accountID string) (*AccountBalance, error)
	
	// UpdateBalance updates account balance
	UpdateBalance(accountID string, balance *AccountBalance) error
	
	// GetPositions retrieves account positions
	GetPositions(accountID string) (*AccountPosition, error)
	
	// Transfer transfers assets between accounts
	Transfer(transfer *AccountTransfer) error
	
	// GetTransferHistory retrieves transfer history
	GetTransferHistory(accountID string, limit int) ([]*AccountTransfer, error)
	
	// RebalanceAccounts rebalances assets across accounts
	RebalanceAccounts(rules RebalanceRules) error
	
	// GetMetrics retrieves account performance metrics
	GetMetrics(accountID string) (*AccountMetrics, error)
	
	// UpdateRateLimit updates rate limit usage for an account
	UpdateRateLimit(accountID string, weight int) error
	
	// RotateAccounts rotates accounts for rate limit distribution
	RotateAccounts(strategy string) error
}

// RebalanceRules defines rules for account rebalancing
type RebalanceRules struct {
	MinMainBalance    decimal.Decimal          `json:"min_main_balance"`
	MaxSubBalance     decimal.Decimal          `json:"max_sub_balance"`
	TargetAllocations map[string]decimal.Decimal `json:"target_allocations"`
	Threshold         decimal.Decimal          `json:"threshold"`
	DryRun            bool                     `json:"dry_run"`
}

// CreateAccountRequest represents a request to create a new account
type CreateAccountRequest struct {
	Name           string                 `json:"name"`
	Type           AccountType            `json:"type"`
	Exchange       string                 `json:"exchange"`
	Market         MarketType             `json:"market"`
	Parent         string                 `json:"parent,omitempty"`
	Strategy       string                 `json:"strategy,omitempty"`
	SpotEnabled    bool                   `json:"spot_enabled"`
	FuturesEnabled bool                   `json:"futures_enabled"`
	MaxBalance     decimal.Decimal        `json:"max_balance"`
	MaxLeverage    int                    `json:"max_leverage"`
	APIKey         string                 `json:"api_key,omitempty"`
	SecretKey      string                 `json:"secret_key,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// AccountTransferRequest represents a request to transfer assets between accounts
type AccountTransferRequest struct {
	FromAccountID string          `json:"from_account_id"`
	ToAccountID   string          `json:"to_account_id"`
	Asset         string          `json:"asset"`
	Amount        decimal.Decimal `json:"amount"`
}

// APICredentials holds API key information for an account
type APICredentials struct {
	APIKey     string `json:"api_key"`
	SecretKey  string `json:"secret_key"`
	Passphrase string `json:"passphrase,omitempty"` // For exchanges that require it
}