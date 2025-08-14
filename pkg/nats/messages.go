package nats

import (
	"time"
	
	"github.com/mExOms/oms/pkg/types"
)

// OrderMessage represents an order message
type OrderMessage struct {
	Action    string       `json:"action"` // create, update, cancel, fill
	Order     types.Order  `json:"order"`
	Timestamp time.Time    `json:"timestamp"`
}

// MarketDataMessage represents market data update
type MarketDataMessage struct {
	Exchange   string           `json:"exchange"`
	Symbol     string           `json:"symbol"`
	MarketData types.MarketData `json:"market_data"`
	Timestamp  time.Time        `json:"timestamp"`
}

// PositionMessage represents position update
type PositionMessage struct {
	Exchange  string         `json:"exchange"`
	Symbol    string         `json:"symbol"`
	Position  types.Position `json:"position"`
	Timestamp time.Time      `json:"timestamp"`
}

// RiskAlertMessage represents risk management alerts
type RiskAlertMessage struct {
	Level       string    `json:"level"` // warning, critical
	Type        string    `json:"type"`  // position_limit, loss_limit, etc
	Exchange    string    `json:"exchange"`
	Symbol      string    `json:"symbol"`
	Message     string    `json:"message"`
	CurrentValue float64  `json:"current_value"`
	LimitValue   float64  `json:"limit_value"`
	Timestamp   time.Time `json:"timestamp"`
}

// ExecutionMessage represents trade execution
type ExecutionMessage struct {
	OrderID      string    `json:"order_id"`
	Exchange     string    `json:"exchange"`
	Symbol       string    `json:"symbol"`
	Side         string    `json:"side"`
	Price        float64   `json:"price"`
	Quantity     float64   `json:"quantity"`
	ExecutionID  string    `json:"execution_id"`
	Timestamp    time.Time `json:"timestamp"`
}

// BalanceMessage represents balance update
type BalanceMessage struct {
	Exchange  string          `json:"exchange"`
	Balances  []types.Balance `json:"balances"`
	Timestamp time.Time       `json:"timestamp"`
}

// SystemMessage represents system-wide messages
type SystemMessage struct {
	Type      string    `json:"type"` // info, warning, error
	Component string    `json:"component"`
	Message   string    `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// OrderAction constants
const (
	OrderActionCreate = "create"
	OrderActionUpdate = "update"
	OrderActionCancel = "cancel"
	OrderActionFill   = "fill"
	OrderActionReject = "reject"
)

// Risk alert levels
const (
	RiskLevelWarning  = "warning"
	RiskLevelCritical = "critical"
)

// Risk alert types
const (
	RiskTypePositionLimit = "position_limit"
	RiskTypeLossLimit     = "loss_limit"
	RiskTypeLeverageLimit = "leverage_limit"
	RiskTypePriceDeviation = "price_deviation"
	RiskTypeOrderRate     = "order_rate"
)