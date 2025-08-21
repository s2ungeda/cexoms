package types

import (
	"time"
	
	"github.com/shopspring/decimal"
)


// Margin types
const (
	MarginTypeCross    = "CROSS"
	MarginTypeIsolated = "ISOLATED"
)

// Position mode types
const (
	PositionModeOneWay = "ONE_WAY"
	PositionModeHedge  = "HEDGE"
)

// MarginMode represents margin mode
type MarginMode string

const (
	MarginModeCrossed  MarginMode = "CROSSED"
	MarginModeIsolated MarginMode = "ISOLATED"
)

// Position represents a unified position (spot/futures)
type Position struct {
	Symbol           string          `json:"symbol"`
	Side             PositionSide    `json:"side"`
	Amount           decimal.Decimal `json:"amount"`
	EntryPrice       decimal.Decimal `json:"entry_price"`
	MarkPrice        decimal.Decimal `json:"mark_price"`
	UnrealizedPnL    decimal.Decimal `json:"unrealized_pnl"`
	RealizedPnL      decimal.Decimal `json:"realized_pnl"`
	Leverage         int             `json:"leverage,omitempty"`
	MarginMode       MarginMode      `json:"margin_mode,omitempty"`
	IsolatedMargin   decimal.Decimal `json:"isolated_margin,omitempty"`
	LiquidationPrice decimal.Decimal `json:"liquidation_price,omitempty"`
	UpdateTime       time.Time       `json:"update_time"`
}

// FuturesPosition represents a futures position
type FuturesPosition struct {
	Symbol                string          `json:"symbol"`
	PositionSide          string          `json:"position_side"`
	MarginType            string          `json:"margin_type"`
	Quantity              decimal.Decimal `json:"quantity"`
	EntryPrice            decimal.Decimal `json:"entry_price"`
	MarkPrice             decimal.Decimal `json:"mark_price"`
	UnrealizedPnL         decimal.Decimal `json:"unrealized_pnl"`
	RealizedPnL           decimal.Decimal `json:"realized_pnl"`
	Margin                decimal.Decimal `json:"margin"`
	IsolatedMargin        decimal.Decimal `json:"isolated_margin"`
	Leverage              int             `json:"leverage"`
	LiquidationPrice      decimal.Decimal `json:"liquidation_price"`
	MarginRatio           decimal.Decimal `json:"margin_ratio"`
	MaintenanceMargin     decimal.Decimal `json:"maintenance_margin"`
	InitialMargin         decimal.Decimal `json:"initial_margin"`
	PositionInitialMargin decimal.Decimal `json:"position_initial_margin"`
	OpenOrderInitialMargin decimal.Decimal `json:"open_order_initial_margin"`
	UpdateTime            time.Time       `json:"update_time"`
}

// FuturesAccount represents futures account information
type FuturesAccount struct {
	TotalBalance          decimal.Decimal    `json:"total_balance"`
	AvailableBalance      decimal.Decimal    `json:"available_balance"`
	TotalMargin           decimal.Decimal    `json:"total_margin"`
	TotalUnrealizedPnL    decimal.Decimal    `json:"total_unrealized_pnl"`
	TotalMaintenanceMargin decimal.Decimal   `json:"total_maintenance_margin"`
	Positions             []*FuturesPosition `json:"positions"`
	UpdateTime            time.Time          `json:"update_time"`
}

// FundingRate represents funding rate information
type FundingRate struct {
	Symbol      string          `json:"symbol"`
	Rate        decimal.Decimal `json:"rate"`
	Time        time.Time       `json:"time"`
	NextFunding time.Time       `json:"next_funding"`
}

// FuturesAsset represents an asset in futures account
type FuturesAsset struct {
	Asset               string          `json:"asset"`
	Balance             decimal.Decimal `json:"balance"`
	CrossBalance        decimal.Decimal `json:"cross_balance"`
	CrossUnPnL          decimal.Decimal `json:"cross_un_pnl"`
	AvailableBalance    decimal.Decimal `json:"available_balance"`
	MaxWithdrawAmount   decimal.Decimal `json:"max_withdraw_amount"`
	MarginAvailable     bool            `json:"margin_available"`
	UpdateTime          time.Time       `json:"update_time"`
}

// LeverageInfo represents leverage settings
type LeverageInfo struct {
	Symbol           string          `json:"symbol"`
	Leverage         int             `json:"leverage"`
	MaxNotionalValue decimal.Decimal `json:"max_notional_value"`
}

// MarginChangeRequest represents a margin change request
type MarginChangeRequest struct {
	Symbol       string          `json:"symbol"`
	PositionSide string          `json:"position_side"`
	Amount       decimal.Decimal `json:"amount"`
	Type         int             `json:"type"` // 1: Add, 2: Reduce
}


// FuturesKline represents futures candlestick data
type FuturesKline struct {
	Symbol       string          `json:"symbol"`
	Interval     string          `json:"interval"`
	OpenTime     int64           `json:"open_time"`
	CloseTime    int64           `json:"close_time"`
	Open         decimal.Decimal `json:"open"`
	High         decimal.Decimal `json:"high"`
	Low          decimal.Decimal `json:"low"`
	Close        decimal.Decimal `json:"close"`
	Volume       decimal.Decimal `json:"volume"`
	QuoteVolume  decimal.Decimal `json:"quote_volume"`
	TakerVolume  decimal.Decimal `json:"taker_volume"`
	TakerQuoteVolume decimal.Decimal `json:"taker_quote_volume"`
	Count        int             `json:"count"`
}

// FuturesDepth represents futures order book
type FuturesDepth struct {
	Symbol       string      `json:"symbol"`
	LastUpdateID int64       `json:"last_update_id"`
	Bids         []PriceLevel `json:"bids"`
	Asks         []PriceLevel `json:"asks"`
	Timestamp    time.Time   `json:"timestamp"`
}

// FuturesTrade represents a futures trade
type FuturesTrade struct {
	ID           int64           `json:"id"`
	Symbol       string          `json:"symbol"`
	Price        decimal.Decimal `json:"price"`
	Quantity     decimal.Decimal `json:"quantity"`
	QuoteQty     decimal.Decimal `json:"quote_qty"`
	Time         time.Time       `json:"time"`
	IsBuyerMaker bool            `json:"is_buyer_maker"`
}

// PositionRisk represents position risk information
type PositionRisk struct {
	Symbol             string          `json:"symbol"`
	Side               Side            `json:"side"`
	PositionAmt        decimal.Decimal `json:"position_amt"`
	PositionAmount     decimal.Decimal `json:"position_amount"` // Alias for backward compatibility
	EntryPrice         decimal.Decimal `json:"entry_price"`
	MarkPrice          decimal.Decimal `json:"mark_price"`
	UnrealizedProfit   decimal.Decimal `json:"unrealized_profit"`
	UnrealizedPnL      decimal.Decimal `json:"unrealized_pnl"` // Alias for backward compatibility
	LiquidationPrice   decimal.Decimal `json:"liquidation_price"`
	Leverage           int             `json:"leverage"`
	MaxNotional        decimal.Decimal `json:"max_notional"`
	MaxNotionalValue   decimal.Decimal `json:"max_notional_value"` // Alias for backward compatibility
	MarginType         string          `json:"margin_type"`
	IsolatedMargin     decimal.Decimal `json:"isolated_margin"`
	IsAutoAddMargin    bool            `json:"is_auto_add_margin"`
	PositionSide       string          `json:"position_side"`
	Notional           decimal.Decimal `json:"notional"`
	IsolatedWallet     decimal.Decimal `json:"isolated_wallet"`
	UpdateTime         time.Time       `json:"update_time"`
}

// OrderModifyRequest represents a request to modify an order
type OrderModifyRequest struct {
	Symbol      string          `json:"symbol"`
	OrderID     string          `json:"order_id"`
	Side        string          `json:"side"`
	PositionSide string         `json:"position_side,omitempty"`
	Quantity    decimal.Decimal `json:"quantity,omitempty"`
	Price       decimal.Decimal `json:"price,omitempty"`
}