package nats

import (
	"fmt"
	"strings"
)

// Subject naming convention:
// {action}.{exchange}.{account}.{market}.{symbol}
// Examples:
// - orders.create.binance.main.spot.BTCUSDT
// - orders.cancel.binance.sub_arb.futures.ETHUSDT
// - positions.update.binance.sub_trend.futures.*
// - balance.update.binance.*.spot.*

// Actions
const (
	// Order actions
	ActionOrderCreate = "orders.create"
	ActionOrderCancel = "orders.cancel"
	ActionOrderUpdate = "orders.update"
	ActionOrderStatus = "orders.status"
	ActionOrderFilled = "orders.filled"
	
	// Position actions
	ActionPositionUpdate = "positions.update"
	ActionPositionClose  = "positions.close"
	
	// Balance actions
	ActionBalanceUpdate = "balance.update"
	ActionBalanceQuery  = "balance.query"
	
	// Transfer actions
	ActionTransferRequest   = "transfer.request"
	ActionTransferComplete  = "transfer.complete"
	ActionTransferFailed    = "transfer.failed"
	
	// Account actions
	ActionAccountCreate     = "account.create"
	ActionAccountUpdate     = "account.update"
	ActionAccountActivate   = "account.activate"
	ActionAccountDeactivate = "account.deactivate"
	
	// Market data actions
	ActionMarketOrderbook = "market.orderbook"
	ActionMarketTrades    = "market.trades"
	ActionMarketTicker    = "market.ticker"
	
	// System actions
	ActionSystemHealth    = "system.health"
	ActionSystemMetrics   = "system.metrics"
	ActionSystemAlert     = "system.alert"
)

// SubjectBuilder helps build NATS subjects
type SubjectBuilder struct {
	action   string
	exchange string
	account  string
	market   string
	symbol   string
}

// NewSubjectBuilder creates a new subject builder
func NewSubjectBuilder() *SubjectBuilder {
	return &SubjectBuilder{}
}

// WithAction sets the action
func (sb *SubjectBuilder) WithAction(action string) *SubjectBuilder {
	sb.action = action
	return sb
}

// WithExchange sets the exchange
func (sb *SubjectBuilder) WithExchange(exchange string) *SubjectBuilder {
	sb.exchange = exchange
	return sb
}

// WithAccount sets the account
func (sb *SubjectBuilder) WithAccount(account string) *SubjectBuilder {
	sb.account = account
	return sb
}

// WithMarket sets the market
func (sb *SubjectBuilder) WithMarket(market string) *SubjectBuilder {
	sb.market = market
	return sb
}

// WithSymbol sets the symbol
func (sb *SubjectBuilder) WithSymbol(symbol string) *SubjectBuilder {
	sb.symbol = symbol
	return sb
}

// Build creates the subject string
func (sb *SubjectBuilder) Build() string {
	parts := []string{sb.action}
	
	// Add exchange (required)
	if sb.exchange == "" {
		sb.exchange = "*"
	}
	parts = append(parts, sb.exchange)
	
	// Add account (required for multi-account)
	if sb.account == "" {
		sb.account = "*"
	}
	parts = append(parts, sb.account)
	
	// Add market
	if sb.market == "" {
		sb.market = "*"
	}
	parts = append(parts, sb.market)
	
	// Add symbol
	if sb.symbol == "" {
		sb.symbol = "*"
	}
	parts = append(parts, sb.symbol)
	
	return strings.Join(parts, ".")
}

// ParseSubject parses a NATS subject into components
func ParseSubject(subject string) (action, exchange, account, market, symbol string) {
	parts := strings.Split(subject, ".")
	
	if len(parts) >= 2 {
		action = parts[0] + "." + parts[1]
	}
	
	if len(parts) > 2 {
		exchange = parts[2]
	}
	
	if len(parts) > 3 {
		account = parts[3]
	}
	
	if len(parts) > 4 {
		market = parts[4]
	}
	
	if len(parts) > 5 {
		symbol = parts[5]
	}
	
	return
}

// Common subject patterns

// OrderSubject creates a subject for order operations
func OrderSubject(action, exchange, account, market, symbol string) string {
	return NewSubjectBuilder().
		WithAction(action).
		WithExchange(exchange).
		WithAccount(account).
		WithMarket(market).
		WithSymbol(symbol).
		Build()
}

// PositionSubject creates a subject for position operations
func PositionSubject(action, exchange, account, market, symbol string) string {
	return NewSubjectBuilder().
		WithAction(action).
		WithExchange(exchange).
		WithAccount(account).
		WithMarket(market).
		WithSymbol(symbol).
		Build()
}

// BalanceSubject creates a subject for balance operations
func BalanceSubject(action, exchange, account string) string {
	return NewSubjectBuilder().
		WithAction(action).
		WithExchange(exchange).
		WithAccount(account).
		Build()
}

// TransferSubject creates a subject for transfer operations
func TransferSubject(action, exchange, fromAccount, toAccount string) string {
	// Special format for transfers: transfer.{action}.{exchange}.{from}.{to}
	return fmt.Sprintf("transfer.%s.%s.%s.%s", action, exchange, fromAccount, toAccount)
}

// MarketDataSubject creates a subject for market data
func MarketDataSubject(dataType, exchange, symbol string) string {
	return fmt.Sprintf("market.%s.%s.*.%s", dataType, exchange, symbol)
}

// Stream names for JetStream

// GetStreamName returns the stream name for a given type
func GetStreamName(streamType string) string {
	return fmt.Sprintf("OMS_%s", strings.ToUpper(streamType))
}

// Stream configurations
const (
	StreamOrders    = "ORDERS"
	StreamPositions = "POSITIONS"
	StreamBalances  = "BALANCES"
	StreamTransfers = "TRANSFERS"
	StreamMarket    = "MARKET"
	StreamSystem    = "SYSTEM"
)

// GetStreamSubjects returns subjects for a stream
func GetStreamSubjects(streamName string) []string {
	switch streamName {
	case StreamOrders:
		return []string{"orders.>"}
	case StreamPositions:
		return []string{"positions.>"}
	case StreamBalances:
		return []string{"balance.>"}
	case StreamTransfers:
		return []string{"transfer.>"}
	case StreamMarket:
		return []string{"market.>"}
	case StreamSystem:
		return []string{"system.>"}
	default:
		return []string{}
	}
}

// Subscription helpers

// SubscribeToAccount creates a subscription pattern for all events from an account
func SubscribeToAccount(exchange, account string) string {
	return fmt.Sprintf("*.%s.%s.>", exchange, account)
}

// SubscribeToStrategy creates a subscription pattern for strategy-specific accounts
func SubscribeToStrategy(strategy string) string {
	// Assuming account names contain strategy identifier
	return fmt.Sprintf("*.*.%s*.>", strategy)
}

// SubscribeToOrdersForAccount creates a subscription for orders from specific account
func SubscribeToOrdersForAccount(exchange, account string) string {
	return fmt.Sprintf("orders.*.%s.%s.>", exchange, account)
}

// SubscribeToTransfersForAccount creates a subscription for transfers involving an account
func SubscribeToTransfersForAccount(account string) string {
	// Subscribe to transfers where account is either source or destination
	return fmt.Sprintf("transfer.*.*.%s.>", account)
}