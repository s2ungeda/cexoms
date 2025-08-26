package transfer

import "time"

type TransferConfig struct {
	EnableAutoRebalance bool                       `json:"enable_auto_rebalance"`
	RebalanceSchedule   string                     `json:"rebalance_schedule"`
	MinMainBalance      map[string]float64         `json:"min_main_balance"`
	MaxSubBalance       map[string]float64         `json:"max_sub_balance"`
	TargetRatios        map[string]map[string]float64 `json:"target_ratios"`
	MaxRetries          int                        `json:"max_retries"`
	RetryDelay          time.Duration              `json:"retry_delay"`
}

type TransferStats struct {
	TotalTransfers   int                `json:"total_transfers"`
	SuccessfulCount  int                `json:"successful_count"`
	FailedCount      int                `json:"failed_count"`
	RetryCount       int                `json:"retry_count"`
	TotalVolume      map[string]float64 `json:"total_volume"`
	AverageTime      time.Duration      `json:"average_time"`
	LastTransfer     *time.Time         `json:"last_transfer,omitempty"`
	LastRebalance    *time.Time         `json:"last_rebalance,omitempty"`
}

type AccountBalance struct {
	Exchange  string             `json:"exchange"`
	AccountID string             `json:"account_id"`
	Type      string             `json:"type"`
	Strategy  string             `json:"strategy,omitempty"`
	Balances  map[string]float64 `json:"balances"`
	UpdatedAt time.Time          `json:"updated_at"`
}

type RebalanceRequest struct {
	ID          string                 `json:"id"`
	Exchange    string                 `json:"exchange"`
	Accounts    []string               `json:"accounts"`
	Assets      []string               `json:"assets"`
	TargetRatios map[string]map[string]float64 `json:"target_ratios"`
	DryRun      bool                   `json:"dry_run"`
	CreatedAt   time.Time              `json:"created_at"`
}

type RebalanceResult struct {
	RequestID      string             `json:"request_id"`
	Success        bool               `json:"success"`
	TransferCount  int                `json:"transfer_count"`
	Transfers      []TransferRequest  `json:"transfers"`
	ErrorMessage   string             `json:"error_message,omitempty"`
	ExecutionTime  time.Duration      `json:"execution_time"`
	CompletedAt    time.Time          `json:"completed_at"`
}

type TransferFilter struct {
	Exchange    string         `json:"exchange,omitempty"`
	AccountID   string         `json:"account_id,omitempty"`
	Asset       string         `json:"asset,omitempty"`
	Type        TransferType   `json:"type,omitempty"`
	Status      TransferStatus `json:"status,omitempty"`
	StartTime   *time.Time     `json:"start_time,omitempty"`
	EndTime     *time.Time     `json:"end_time,omitempty"`
	MinAmount   float64        `json:"min_amount,omitempty"`
	MaxAmount   float64        `json:"max_amount,omitempty"`
	Limit       int            `json:"limit,omitempty"`
	Offset      int            `json:"offset,omitempty"`
}

type TransferSummary struct {
	Exchange        string                 `json:"exchange"`
	Period          string                 `json:"period"`
	StartTime       time.Time              `json:"start_time"`
	EndTime         time.Time              `json:"end_time"`
	TotalTransfers  int                    `json:"total_transfers"`
	SuccessRate     float64                `json:"success_rate"`
	VolumeByAsset   map[string]float64     `json:"volume_by_asset"`
	TransfersByType map[TransferType]int   `json:"transfers_by_type"`
	AverageAmount   map[string]float64     `json:"average_amount"`
	TopAccounts     []AccountTransferStats `json:"top_accounts"`
}

type AccountTransferStats struct {
	AccountID      string             `json:"account_id"`
	AccountType    string             `json:"account_type"`
	Strategy       string             `json:"strategy,omitempty"`
	TransferCount  int                `json:"transfer_count"`
	IncomingVolume map[string]float64 `json:"incoming_volume"`
	OutgoingVolume map[string]float64 `json:"outgoing_volume"`
	NetFlow        map[string]float64 `json:"net_flow"`
}

type TransferNotification struct {
	Type      string           `json:"type"`
	Severity  string           `json:"severity"`
	Transfer  *TransferRequest `json:"transfer,omitempty"`
	Message   string           `json:"message"`
	Timestamp time.Time        `json:"timestamp"`
}

const (
	NotificationTypeTransferComplete   = "TRANSFER_COMPLETE"
	NotificationTypeTransferFailed     = "TRANSFER_FAILED"
	NotificationTypeRebalanceComplete  = "REBALANCE_COMPLETE"
	NotificationTypeLowBalance         = "LOW_BALANCE"
	NotificationTypeHighVolume         = "HIGH_VOLUME"
	
	SeverityInfo    = "INFO"
	SeverityWarning = "WARNING"
	SeverityError   = "ERROR"
)

type TransferLimit struct {
	Asset            string  `json:"asset"`
	MinAmount        float64 `json:"min_amount"`
	MaxAmount        float64 `json:"max_amount"`
	DailyLimit       float64 `json:"daily_limit"`
	HourlyLimit      float64 `json:"hourly_limit"`
	MaxTransfersHour int     `json:"max_transfers_hour"`
	MaxTransfersDay  int     `json:"max_transfers_day"`
}

var DefaultTransferLimits = map[string]TransferLimit{
	"BTC": {
		Asset:            "BTC",
		MinAmount:        0.0001,
		MaxAmount:        10.0,
		DailyLimit:       50.0,
		HourlyLimit:      5.0,
		MaxTransfersHour: 20,
		MaxTransfersDay:  100,
	},
	"ETH": {
		Asset:            "ETH",
		MinAmount:        0.001,
		MaxAmount:        100.0,
		DailyLimit:       500.0,
		HourlyLimit:      50.0,
		MaxTransfersHour: 20,
		MaxTransfersDay:  100,
	},
	"USDT": {
		Asset:            "USDT",
		MinAmount:        10.0,
		MaxAmount:        1000000.0,
		DailyLimit:       5000000.0,
		HourlyLimit:      500000.0,
		MaxTransfersHour: 50,
		MaxTransfersDay:  200,
	},
}