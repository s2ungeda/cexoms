package bybit

import (
	"encoding/json"
)

// Bybit API Response Structures

// BaseResponse is the common response structure
type BaseResponse struct {
	RetCode    int         `json:"retCode"`
	RetMsg     string      `json:"retMsg"`
	Result     interface{} `json:"result"`
	RetExtInfo interface{} `json:"retExtInfo"`
	Time       int64       `json:"time"`
}

// AccountInfo represents account information
type AccountInfo struct {
	UID         string `json:"uid"`
	AccountType string `json:"accountType"`
	MemberLevel string `json:"memberLevel"`
}

// SpotBalance represents spot wallet balance
type SpotBalance struct {
	Coin     string `json:"coin"`
	CoinID   string `json:"coinId"`
	CoinName string `json:"coinName"`
	Total    string `json:"total"`
	Free     string `json:"free"`
	Locked   string `json:"locked"`
}

// FuturesBalance represents futures wallet balance
type FuturesBalance struct {
	Coin                  string `json:"coin"`
	Equity                string `json:"equity"`
	WalletBalance         string `json:"walletBalance"`
	PositionMargin        string `json:"positionMargin"`
	AvailableBalance      string `json:"availableBalance"`
	OrderMargin           string `json:"orderMargin"`
	OccClosingFee         string `json:"occClosingFee"`
	OccFundingFee         string `json:"occFundingFee"`
	UnrealizedPnl         string `json:"unrealizedPnl"`
	CumRealizedPnl        string `json:"cumRealizedPnl"`
	GivenCash             string `json:"givenCash"`
	ServiceCash           string `json:"serviceCash"`
}

// Symbol represents trading pair information
type Symbol struct {
	Symbol             string `json:"symbol"`
	BaseCoin           string `json:"baseCoin"`
	QuoteCoin          string `json:"quoteCoin"`
	Status             string `json:"status"`
	BasePrecision      string `json:"basePrecision"`
	QuotePrecision     string `json:"quotePrecision"`
	MinOrderQty        string `json:"minOrderQty"`
	MaxOrderQty        string `json:"maxOrderQty"`
	MinOrderAmt        string `json:"minOrderAmt"`
	MaxOrderAmt        string `json:"maxOrderAmt"`
	TickSize           string `json:"tickSize"`
}

// FuturesSymbol represents futures contract information
type FuturesSymbol struct {
	Symbol           string `json:"symbol"`
	ContractType     string `json:"contractType"`
	Status           string `json:"status"`
	BaseCoin         string `json:"baseCoin"`
	QuoteCoin        string `json:"quoteCoin"`
	LaunchTime       string `json:"launchTime"`
	DeliveryTime     string `json:"deliveryTime"`
	DeliveryFeeRate  string `json:"deliveryFeeRate"`
	PriceScale       string `json:"priceScale"`
	LeverageFilter   LeverageFilter `json:"leverageFilter"`
	PriceFilter      PriceFilter    `json:"priceFilter"`
	LotSizeFilter    LotSizeFilter  `json:"lotSizeFilter"`
}

// Filter types for futures
type LeverageFilter struct {
	MinLeverage  string `json:"minLeverage"`
	MaxLeverage  string `json:"maxLeverage"`
	LeverageStep string `json:"leverageStep"`
}

type PriceFilter struct {
	MinPrice string `json:"minPrice"`
	MaxPrice string `json:"maxPrice"`
	TickSize string `json:"tickSize"`
}

type LotSizeFilter struct {
	MaxOrderQty         string `json:"maxOrderQty"`
	MinOrderQty         string `json:"minOrderQty"`
	QtyStep             string `json:"qtyStep"`
	PostOnlyMaxOrderQty string `json:"postOnlyMaxOrderQty"`
}

// Order represents an order
type Order struct {
	OrderId            string `json:"orderId"`
	OrderLinkId        string `json:"orderLinkId"`
	Symbol             string `json:"symbol"`
	Price              string `json:"price"`
	Qty                string `json:"qty"`
	Side               string `json:"side"`
	OrderType          string `json:"orderType"`
	TimeInForce        string `json:"timeInForce"`
	OrderStatus        string `json:"orderStatus"`
	CumExecQty         string `json:"cumExecQty"`
	CumExecValue       string `json:"cumExecValue"`
	CumExecFee         string `json:"cumExecFee"`
	StopOrderType      string `json:"stopOrderType"`
	TriggerDirection   string `json:"triggerDirection"`
	TriggerBy          string `json:"triggerBy"`
	TriggerPrice       string `json:"triggerPrice"`
	CreateTime         string `json:"createTime"`
	UpdateTime         string `json:"updateTime"`
	ReduceOnly         bool   `json:"reduceOnly"`
	CloseOnTrigger     bool   `json:"closeOnTrigger"`
	PlaceType          string `json:"placeType"`
}

// Trade represents a trade/fill
type Trade struct {
	Symbol          string `json:"symbol"`
	OrderId         string `json:"orderId"`
	OrderLinkId     string `json:"orderLinkId"`
	Side            string `json:"side"`
	OrderPrice      string `json:"orderPrice"`
	OrderQty        string `json:"orderQty"`
	OrderType       string `json:"orderType"`
	ExecId          string `json:"execId"`
	ExecPrice       string `json:"execPrice"`
	ExecQty         string `json:"execQty"`
	ExecFee         string `json:"execFee"`
	ExecType        string `json:"execType"`
	ExecTime        string `json:"execTime"`
	IsMaker         bool   `json:"isMaker"`
	FeeRate         string `json:"feeRate"`
	TradeTime       string `json:"tradeTime"`
}

// Position represents a futures position
type Position struct {
	PositionIdx          int    `json:"positionIdx"`
	RiskId               int    `json:"riskId"`
	RiskLimitValue       string `json:"riskLimitValue"`
	Symbol               string `json:"symbol"`
	Side                 string `json:"side"`
	Size                 string `json:"size"`
	PositionValue        string `json:"positionValue"`
	EntryPrice           string `json:"entryPrice"`
	IsolatedMargin       string `json:"isolatedMargin"`
	AutoAddMargin        int    `json:"autoAddMargin"`
	Leverage             string `json:"leverage"`
	PositionBalance      string `json:"positionBalance"`
	MarkPrice            string `json:"markPrice"`
	LiqPrice             string `json:"liqPrice"`
	BustPrice            string `json:"bustPrice"`
	PositionMM           string `json:"positionMM"`
	PositionIM           string `json:"positionIM"`
	TpslMode             string `json:"tpslMode"`
	TakeProfit           string `json:"takeProfit"`
	StopLoss             string `json:"stopLoss"`
	TrailingStop         string `json:"trailingStop"`
	UnrealizedPnl        string `json:"unrealizedPnl"`
	CumRealizedPnl       string `json:"cumRealizedPnl"`
	AdlRankIndicator     int    `json:"adlRankIndicator"`
	IsReduceOnly         bool   `json:"isReduceOnly"`
	MmrSysUpdateTime     string `json:"mmrSysUpdatedTime"`
	LeverageSysUpdateTime string `json:"leverageSysUpdatedTime"`
	CreatedTime          string `json:"createdTime"`
	UpdatedTime          string `json:"updatedTime"`
}

// WebSocket message types
type WSMessage struct {
	Topic string          `json:"topic"`
	Type  string          `json:"type"`
	Ts    int64          `json:"ts"`
	Data  json.RawMessage `json:"data"`
}

type WSOrderUpdate struct {
	Symbol          string `json:"symbol"`
	OrderId         string `json:"orderId"`
	OrderLinkId     string `json:"orderLinkId"`
	Side            string `json:"side"`
	OrderType       string `json:"orderType"`
	Price           string `json:"price"`
	Qty             string `json:"qty"`
	TimeInForce     string `json:"timeInForce"`
	OrderStatus     string `json:"orderStatus"`
	CumExecQty      string `json:"cumExecQty"`
	CumExecValue    string `json:"cumExecValue"`
	CumExecFee      string `json:"cumExecFee"`
	StopOrderType   string `json:"stopOrderType"`
	TriggerBy       string `json:"triggerBy"`
	TriggerPrice    string `json:"triggerPrice"`
	ReduceOnly      bool   `json:"reduceOnly"`
	CloseOnTrigger  bool   `json:"closeOnTrigger"`
	CreatedTime     string `json:"createdTime"`
	UpdatedTime     string `json:"updatedTime"`
}

type WSExecutionUpdate struct {
	Symbol          string `json:"symbol"`
	IsLeverage      string `json:"isLeverage"`
	OrderId         string `json:"orderId"`
	OrderLinkId     string `json:"orderLinkId"`
	Side            string `json:"side"`
	OrderPrice      string `json:"orderPrice"`
	OrderQty        string `json:"orderQty"`
	OrderType       string `json:"orderType"`
	StopOrderType   string `json:"stopOrderType"`
	ExecId          string `json:"execId"`
	ExecPrice       string `json:"execPrice"`
	ExecQty         string `json:"execQty"`
	ExecType        string `json:"execType"`
	ExecValue       string `json:"execValue"`
	ExecFee         string `json:"execFee"`
	FeeRate         string `json:"feeRate"`
	LeavesQty       string `json:"leavesQty"`
	IsMaker         bool   `json:"isMaker"`
	ExecTime        string `json:"execTime"`
}

type WSPositionUpdate struct {
	PositionIdx     int    `json:"positionIdx"`
	TradeMode       int    `json:"tradeMode"`
	RiskId          int    `json:"riskId"`
	RiskLimitValue  string `json:"riskLimitValue"`
	Symbol          string `json:"symbol"`
	Side            string `json:"side"`
	Size            string `json:"size"`
	EntryPrice      string `json:"entryPrice"`
	Leverage        string `json:"leverage"`
	PositionValue   string `json:"positionValue"`
	PositionBalance string `json:"positionBalance"`
	MarkPrice       string `json:"markPrice"`
	PositionIM      string `json:"positionIM"`
	PositionMM      string `json:"positionMM"`
	TakeProfit      string `json:"takeProfit"`
	StopLoss        string `json:"stopLoss"`
	TrailingStop    string `json:"trailingStop"`
	UnrealizedPnl   string `json:"unrealizedPnl"`
	CumRealizedPnl  string `json:"cumRealizedPnl"`
	CreatedTime     string `json:"createdTime"`
	UpdatedTime     string `json:"updatedTime"`
	TpslMode        string `json:"tpslMode"`
	LiqPrice        string `json:"liqPrice"`
	BustPrice       string `json:"bustPrice"`
	IsReduceOnly    bool   `json:"isReduceOnly"`
	AutoAddMargin   int    `json:"autoAddMargin"`
	AdlRankIndicator int   `json:"adlRankIndicator"`
}

// Ticker represents market ticker data
type Ticker struct {
	Symbol        string `json:"symbol"`
	LastPrice     string `json:"lastPrice"`
	HighPrice24h  string `json:"highPrice24h"`
	LowPrice24h   string `json:"lowPrice24h"`
	PrevPrice24h  string `json:"prevPrice24h"`
	Volume24h     string `json:"volume24h"`
	Turnover24h   string `json:"turnover24h"`
	Price24hPcnt  string `json:"price24hPcnt"`
	UsdIndexPrice string `json:"usdIndexPrice"`
}

// OrderBook represents order book data
type OrderBook struct {
	Symbol string     `json:"s"`
	Bids   [][]string `json:"b"` // [price, quantity]
	Asks   [][]string `json:"a"` // [price, quantity]
	Ts     int64      `json:"ts"`
	U      int64      `json:"u"` // Update ID
}

// Kline represents candlestick data
type Kline struct {
	StartTime  int64  `json:"startTime"`
	OpenPrice  string `json:"openPrice"`
	HighPrice  string `json:"highPrice"`
	LowPrice   string `json:"lowPrice"`
	ClosePrice string `json:"closePrice"`
	Volume     string `json:"volume"`
	Turnover   string `json:"turnover"`
}

// Constants
const (
	// Order sides
	SideBuy  = "Buy"
	SideSell = "Sell"

	// Order types
	OrderTypeMarket          = "Market"
	OrderTypeLimit           = "Limit"
	OrderTypeLimitMaker      = "Limit_maker"

	// Time in force
	TimeInForceGTC = "GTC" // Good Till Cancelled
	TimeInForceIOC = "IOC" // Immediate or Cancel
	TimeInForceFOK = "FOK" // Fill or Kill
	TimeInForcePostOnly = "PostOnly"

	// Order status
	OrderStatusNew             = "New"
	OrderStatusPartiallyFilled = "PartiallyFilled"
	OrderStatusFilled          = "Filled"
	OrderStatusCancelled       = "Cancelled"
	OrderStatusRejected        = "Rejected"

	// Position mode
	PositionModeHedge    = 0 // Hedge mode
	PositionModeOneWay   = 1 // One-way mode

	// Position side
	PositionSideBoth  = "Both"
	PositionSideLong  = "Buy"
	PositionSideShort = "Sell"

	// Category
	CategorySpot    = "spot"
	CategoryLinear  = "linear"  // USDT perpetual
	CategoryInverse = "inverse" // Coin margined

	// WebSocket topics
	TopicOrder      = "order"
	TopicExecution  = "execution"
	TopicPosition   = "position"
	TopicWallet     = "wallet"
)