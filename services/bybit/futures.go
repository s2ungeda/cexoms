package bybit

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// BybitFutures implements the Exchange and FuturesExchange interfaces for Bybit Futures trading
type BybitFutures struct {
	client       *Client
	exchangeType types.ExchangeType
	marketType   types.MarketType
	symbolsCache map[string]*FuturesSymbol
	lastUpdate   time.Time
	positionMode string // "MergedSingle" or "BothSide"
}

// NewBybitFutures creates a new Bybit Futures exchange instance
func NewBybitFutures(apiKey, apiSecret string, testnet bool) *BybitFutures {
	return &BybitFutures{
		client:       NewClient(apiKey, apiSecret, testnet),
		exchangeType: types.ExchangeBybit,
		marketType:   types.MarketTypeFutures,
		symbolsCache: make(map[string]*FuturesSymbol),
		positionMode: "MergedSingle", // Default position mode
	}
}

// GetName returns the exchange name
func (b *BybitFutures) GetName() string {
	return string(b.exchangeType)
}

// GetType returns the exchange type
func (b *BybitFutures) GetType() types.ExchangeType {
	return b.exchangeType
}

// GetMarketType returns the market type
func (b *BybitFutures) GetMarketType() types.MarketType {
	return b.marketType
}

// Initialize initializes the exchange
func (b *BybitFutures) Initialize(ctx context.Context) error {
	// Load symbols
	if err := b.loadSymbols(); err != nil {
		return fmt.Errorf("failed to load symbols: %w", err)
	}

	// Get position mode
	if err := b.getPositionMode(); err != nil {
		return fmt.Errorf("failed to get position mode: %w", err)
	}

	// Test connectivity
	if _, err := b.client.GetServerTime(); err != nil {
		return fmt.Errorf("failed to connect to Bybit: %w", err)
	}

	return nil
}

// GetAccountInfo returns account information
func (b *BybitFutures) GetAccountInfo(ctx context.Context) (*types.AccountInfo, error) {
	var accountResult struct {
		UID         string `json:"uid"`
		AccountType string `json:"accountType"`
	}

	err := b.client.Request(http.MethodGet, "/user/query-info", nil, &accountResult)
	if err != nil {
		return nil, fmt.Errorf("failed to get account info: %w", err)
	}

	// Get balances
	balances, err := b.GetBalances(ctx)
	if err != nil {
		return nil, err
	}

	return &types.AccountInfo{
		Exchange:    b.exchangeType,
		AccountID:   accountResult.UID,
		AccountType: "futures",
		Balances:    balances,
		UpdateTime:  time.Now(),
	}, nil
}

// GetBalances returns account balances
func (b *BybitFutures) GetBalances(ctx context.Context) ([]types.Balance, error) {
	params := map[string]interface{}{
		"accountType": "CONTRACT", // USDT perpetual
	}

	var result struct {
		List []struct {
			TotalEquity      string `json:"totalEquity"`
			AccountIMRate    string `json:"accountIMRate"`
			AccountMMRate    string `json:"accountMMRate"`
			TotalPerpUPL     string `json:"totalPerpUPL"`
			TotalWalletBalance string `json:"totalWalletBalance"`
			AccountLTV       string `json:"accountLTV"`
			TotalMarginBalance string `json:"totalMarginBalance"`
			Coin             []FuturesBalance `json:"coin"`
		} `json:"list"`
	}

	err := b.client.Request(http.MethodGet, "/account/wallet-balance", params, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to get balances: %w", err)
	}

	var balances []types.Balance
	if len(result.List) > 0 && len(result.List[0].Coin) > 0 {
		for _, b := range result.List[0].Coin {
			equity, _ := decimal.NewFromString(b.Equity)
			walletBalance, _ := decimal.NewFromString(b.WalletBalance)
			availableBalance, _ := decimal.NewFromString(b.AvailableBalance)
			unrealizedPnl, _ := decimal.NewFromString(b.UnrealizedPnl)

			balances = append(balances, types.Balance{
				Asset:         b.Coin,
				Free:          availableBalance,
				Locked:        walletBalance.Sub(availableBalance),
				Total:         equity,
				UnrealizedPnL: unrealizedPnl,
			})
		}
	}

	return balances, nil
}

// PlaceOrder places an order
func (b *BybitFutures) PlaceOrder(ctx context.Context, order *types.Order) (*types.Order, error) {
	if order == nil {
		return nil, fmt.Errorf("order cannot be nil")
	}

	// Validate order
	if err := b.validateOrder(order); err != nil {
		return nil, err
	}

	// Convert order type
	orderType := b.convertOrderType(order.Type)
	side := b.convertOrderSide(order.Side)

	params := map[string]interface{}{
		"category":    CategoryLinear, // USDT perpetual
		"symbol":      order.Symbol,
		"side":        side,
		"orderType":   orderType,
		"qty":         order.Quantity.String(),
		"timeInForce": TimeInForceGTC,
		"orderLinkId": order.ClientOrderID,
		"reduceOnly":  order.ReduceOnly,
		"closeOnTrigger": false,
	}

	// Add price for limit orders
	if order.Type == types.OrderTypeLimit || order.Type == types.OrderTypeLimitMaker {
		params["price"] = order.Price.String()
	}

	// Add position index for hedge mode
	if b.positionMode == "BothSide" {
		if order.Side == types.OrderSideBuy {
			params["positionIdx"] = 1 // Long position
		} else {
			params["positionIdx"] = 2 // Short position
		}
	} else {
		params["positionIdx"] = 0 // One-way mode
	}

	var result struct {
		OrderId     string `json:"orderId"`
		OrderLinkId string `json:"orderLinkId"`
	}

	err := b.client.Request(http.MethodPost, "/order/create", params, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to place order: %w", err)
	}

	// Update order with exchange ID
	order.ExchangeOrderID = result.OrderId
	order.Status = types.OrderStatusNew
	order.CreatedAt = time.Now()
	order.UpdatedAt = time.Now()

	return order, nil
}

// CancelOrder cancels an order
func (b *BybitFutures) CancelOrder(ctx context.Context, symbol, orderID string) error {
	params := map[string]interface{}{
		"category": CategoryLinear,
		"symbol":   symbol,
	}

	// Check if it's a client order ID or exchange order ID
	if len(orderID) > 20 {
		params["orderId"] = orderID
	} else {
		params["orderLinkId"] = orderID
	}

	err := b.client.Request(http.MethodPost, "/order/cancel", params, nil)
	if err != nil {
		return fmt.Errorf("failed to cancel order: %w", err)
	}

	return nil
}

// GetOrder gets order information
func (b *BybitFutures) GetOrder(ctx context.Context, symbol, orderID string) (*types.Order, error) {
	params := map[string]interface{}{
		"category": CategoryLinear,
		"symbol":   symbol,
		"limit":    1,
	}

	// Check if it's a client order ID or exchange order ID
	if len(orderID) > 20 {
		params["orderId"] = orderID
	} else {
		params["orderLinkId"] = orderID
	}

	var result struct {
		List []Order `json:"list"`
	}

	err := b.client.Request(http.MethodGet, "/order/realtime", params, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	if len(result.List) == 0 {
		return nil, fmt.Errorf("order not found")
	}

	return b.convertOrder(&result.List[0]), nil
}

// GetOpenOrders gets all open orders
func (b *BybitFutures) GetOpenOrders(ctx context.Context, symbol string) ([]*types.Order, error) {
	params := map[string]interface{}{
		"category": CategoryLinear,
		"limit":    500,
	}

	if symbol != "" {
		params["symbol"] = symbol
	}

	var result struct {
		List []Order `json:"list"`
	}

	err := b.client.Request(http.MethodGet, "/order/realtime", params, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to get open orders: %w", err)
	}

	orders := make([]*types.Order, len(result.List))
	for i, o := range result.List {
		orders[i] = b.convertOrder(&o)
	}

	return orders, nil
}

// FuturesExchange specific methods

// GetPositions returns all positions
func (b *BybitFutures) GetPositions(ctx context.Context) ([]*types.Position, error) {
	params := map[string]interface{}{
		"category": CategoryLinear,
		"settleCoin": "USDT",
	}

	var result struct {
		List []Position `json:"list"`
	}

	err := b.client.Request(http.MethodGet, "/position/list", params, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to get positions: %w", err)
	}

	positions := make([]*types.Position, 0)
	for _, p := range result.List {
		// Skip empty positions
		size, _ := decimal.NewFromString(p.Size)
		if size.IsZero() {
			continue
		}

		positions = append(positions, b.convertPosition(&p))
	}

	return positions, nil
}

// GetPosition returns position for a specific symbol
func (b *BybitFutures) GetPosition(ctx context.Context, symbol string) (*types.Position, error) {
	params := map[string]interface{}{
		"category": CategoryLinear,
		"symbol":   symbol,
	}

	var result struct {
		List []Position `json:"list"`
	}

	err := b.client.Request(http.MethodGet, "/position/list", params, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to get position: %w", err)
	}

	// Find position with size > 0
	for _, p := range result.List {
		size, _ := decimal.NewFromString(p.Size)
		if !size.IsZero() {
			return b.convertPosition(&p), nil
		}
	}

	return nil, nil // No position
}

// SetLeverage sets leverage for a symbol
func (b *BybitFutures) SetLeverage(ctx context.Context, symbol string, leverage int) error {
	if leverage < 1 || leverage > 100 {
		return fmt.Errorf("invalid leverage: %d", leverage)
	}

	params := map[string]interface{}{
		"category":     CategoryLinear,
		"symbol":       symbol,
		"buyLeverage":  strconv.Itoa(leverage),
		"sellLeverage": strconv.Itoa(leverage),
	}

	err := b.client.Request(http.MethodPost, "/position/set-leverage", params, nil)
	if err != nil {
		return fmt.Errorf("failed to set leverage: %w", err)
	}

	return nil
}

// SetMarginMode sets margin mode (ISOLATED/CROSSED)
func (b *BybitFutures) SetMarginMode(ctx context.Context, symbol string, marginMode types.MarginMode) error {
	tradeMode := 0 // 0: cross margin, 1: isolated margin
	if marginMode == types.MarginModeIsolated {
		tradeMode = 1
	}

	params := map[string]interface{}{
		"category":  CategoryLinear,
		"symbol":    symbol,
		"tradeMode": tradeMode,
		"buyLeverage": "0",
		"sellLeverage": "0",
	}

	err := b.client.Request(http.MethodPost, "/position/switch-isolated", params, nil)
	if err != nil {
		return fmt.Errorf("failed to set margin mode: %w", err)
	}

	return nil
}

// GetFundingRate gets funding rate for a symbol
func (b *BybitFutures) GetFundingRate(ctx context.Context, symbol string) (*types.FundingRate, error) {
	params := map[string]interface{}{
		"category": CategoryLinear,
		"symbol":   symbol,
		"limit":    1,
	}

	var result struct {
		List []struct {
			Symbol               string `json:"symbol"`
			FundingRate          string `json:"fundingRate"`
			FundingRateTimestamp string `json:"fundingRateTimestamp"`
		} `json:"list"`
	}

	err := b.client.PublicRequest(http.MethodGet, "/market/funding/history", params, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to get funding rate: %w", err)
	}

	if len(result.List) == 0 {
		return nil, fmt.Errorf("funding rate not found")
	}

	rate, _ := decimal.NewFromString(result.List[0].FundingRate)
	timestamp, _ := strconv.ParseInt(result.List[0].FundingRateTimestamp, 10, 64)

	return &types.FundingRate{
		Symbol:      symbol,
		Rate:        rate,
		Time:        time.Unix(0, timestamp*int64(time.Millisecond)),
		NextFunding: time.Unix(0, timestamp*int64(time.Millisecond)).Add(8 * time.Hour),
	}, nil
}

// Additional methods required by the interface

func (b *BybitFutures) GetOrderHistory(ctx context.Context, symbol string, limit int) ([]*types.Order, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	params := map[string]interface{}{
		"category": CategoryLinear,
		"limit":    limit,
	}

	if symbol != "" {
		params["symbol"] = symbol
	}

	var result struct {
		List []Order `json:"list"`
	}

	err := b.client.Request(http.MethodGet, "/order/history", params, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to get order history: %w", err)
	}

	orders := make([]*types.Order, len(result.List))
	for i, o := range result.List {
		orders[i] = b.convertOrder(&o)
	}

	return orders, nil
}

func (b *BybitFutures) GetTrades(ctx context.Context, symbol string, limit int) ([]*types.Trade, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	params := map[string]interface{}{
		"category": CategoryLinear,
		"limit":    limit,
	}

	if symbol != "" {
		params["symbol"] = symbol
	}

	var result struct {
		List []Trade `json:"list"`
	}

	err := b.client.Request(http.MethodGet, "/execution/list", params, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to get trades: %w", err)
	}

	trades := make([]*types.Trade, len(result.List))
	for i, t := range result.List {
		trades[i] = b.convertTrade(&t)
	}

	return trades, nil
}

func (b *BybitFutures) GetSymbolInfo(ctx context.Context, symbol string) (*types.SymbolInfo, error) {
	// Check cache first
	if sym, ok := b.symbolsCache[symbol]; ok {
		return b.convertFuturesSymbolInfo(sym), nil
	}

	// Reload symbols if not in cache
	if err := b.loadSymbols(); err != nil {
		return nil, err
	}

	sym, ok := b.symbolsCache[symbol]
	if !ok {
		return nil, fmt.Errorf("symbol %s not found", symbol)
	}

	return b.convertFuturesSymbolInfo(sym), nil
}

func (b *BybitFutures) GetMarketData(ctx context.Context, symbols []string) (map[string]*types.MarketData, error) {
	params := map[string]interface{}{
		"category": CategoryLinear,
	}

	var result struct {
		List []Ticker `json:"list"`
	}

	err := b.client.PublicRequest(http.MethodGet, "/market/tickers", params, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to get market data: %w", err)
	}

	// Create symbol set for filtering
	symbolSet := make(map[string]bool)
	if len(symbols) > 0 {
		for _, s := range symbols {
			symbolSet[s] = true
		}
	}

	marketData := make(map[string]*types.MarketData)
	for _, ticker := range result.List {
		// Filter by requested symbols if specified
		if len(symbolSet) > 0 && !symbolSet[ticker.Symbol] {
			continue
		}

		marketData[ticker.Symbol] = b.convertTicker(&ticker)
	}

	return marketData, nil
}

func (b *BybitFutures) GetOrderBook(ctx context.Context, symbol string, depth int) (*types.OrderBook, error) {
	if depth <= 0 || depth > 200 {
		depth = 50
	}

	params := map[string]interface{}{
		"category": CategoryLinear,
		"symbol":   symbol,
		"limit":    depth,
	}

	var result OrderBook

	err := b.client.PublicRequest(http.MethodGet, "/market/orderbook", params, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to get order book: %w", err)
	}

	return b.convertOrderBook(&result), nil
}

func (b *BybitFutures) GetKlines(ctx context.Context, symbol string, interval types.KlineInterval, limit int) ([]*types.Kline, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}

	params := map[string]interface{}{
		"category": CategoryLinear,
		"symbol":   symbol,
		"interval": b.convertInterval(interval),
		"limit":    limit,
	}

	var result struct {
		List [][]string `json:"list"`
	}

	err := b.client.PublicRequest(http.MethodGet, "/market/kline", params, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to get klines: %w", err)
	}

	return b.convertKlines(result.List), nil
}

// Helper methods

func (b *BybitFutures) loadSymbols() error {
	params := map[string]interface{}{
		"category": CategoryLinear,
	}

	var result struct {
		List []FuturesSymbol `json:"list"`
	}

	err := b.client.PublicRequest(http.MethodGet, "/market/instruments-info", params, &result)
	if err != nil {
		return fmt.Errorf("failed to get symbols: %w", err)
	}

	// Update cache
	b.symbolsCache = make(map[string]*FuturesSymbol)
	for i := range result.List {
		sym := &result.List[i]
		b.symbolsCache[sym.Symbol] = sym
	}
	b.lastUpdate = time.Now()

	return nil
}

func (b *BybitFutures) getPositionMode() error {
	var result struct {
		IsModified  bool   `json:"isModified"`
		PositionMode string `json:"positionMode"`
	}

	err := b.client.Request(http.MethodGet, "/position/switch-mode", nil, &result)
	if err != nil {
		return err
	}

	b.positionMode = result.PositionMode
	return nil
}

func (b *BybitFutures) validateOrder(order *types.Order) error {
	if order.Symbol == "" {
		return fmt.Errorf("symbol is required")
	}
	if order.Quantity.IsZero() || order.Quantity.IsNegative() {
		return fmt.Errorf("invalid quantity")
	}
	if order.Type == types.OrderTypeLimit && order.Price.IsZero() {
		return fmt.Errorf("price is required for limit orders")
	}
	return nil
}

func (b *BybitFutures) convertOrderType(orderType types.OrderType) string {
	switch orderType {
	case types.OrderTypeMarket:
		return OrderTypeMarket
	case types.OrderTypeLimit:
		return OrderTypeLimit
	case types.OrderTypeLimitMaker:
		return OrderTypeLimitMaker
	default:
		return OrderTypeLimit
	}
}

func (b *BybitFutures) convertOrderSide(side types.OrderSide) string {
	switch side {
	case types.OrderSideBuy:
		return SideBuy
	case types.OrderSideSell:
		return SideSell
	default:
		return SideBuy
	}
}

func (b *BybitFutures) convertOrder(o *Order) *types.Order {
	qty, _ := decimal.NewFromString(o.Qty)
	price, _ := decimal.NewFromString(o.Price)
	executedQty, _ := decimal.NewFromString(o.CumExecQty)
	executedValue, _ := decimal.NewFromString(o.CumExecValue)
	fee, _ := decimal.NewFromString(o.CumExecFee)

	order := &types.Order{
		ClientOrderID:   o.OrderLinkId,
		ExchangeOrderID: o.OrderId,
		Symbol:          o.Symbol,
		Side:            b.parseOrderSide(o.Side),
		Type:            b.parseOrderType(o.OrderType),
		Status:          b.parseOrderStatus(o.OrderStatus),
		Price:           price,
		Quantity:        qty,
		ExecutedQty:     executedQty,
		RemainingQty:    qty.Sub(executedQty),
		Fee:             fee,
		ReduceOnly:      o.ReduceOnly,
	}

	// Calculate average price if executed
	if executedQty.IsPositive() && executedValue.IsPositive() {
		order.AvgPrice = executedValue.Div(executedQty)
	}

	// Parse timestamps
	if createTime, err := strconv.ParseInt(o.CreateTime, 10, 64); err == nil {
		order.CreatedAt = time.Unix(0, createTime*int64(time.Millisecond))
	}
	if updateTime, err := strconv.ParseInt(o.UpdateTime, 10, 64); err == nil {
		order.UpdatedAt = time.Unix(0, updateTime*int64(time.Millisecond))
	}

	return order
}

func (b *BybitFutures) convertPosition(p *Position) *types.Position {
	size, _ := decimal.NewFromString(p.Size)
	entryPrice, _ := decimal.NewFromString(p.EntryPrice)
	markPrice, _ := decimal.NewFromString(p.MarkPrice)
	liquidationPrice, _ := decimal.NewFromString(p.LiqPrice)
	unrealizedPnl, _ := decimal.NewFromString(p.UnrealizedPnl)
	realizedPnl, _ := decimal.NewFromString(p.CumRealizedPnl)
	leverage, _ := strconv.Atoi(p.Leverage)
	isolatedMargin, _ := decimal.NewFromString(p.IsolatedMargin)

	// Convert side
	var side types.PositionSide
	if p.Side == PositionSideLong {
		side = types.PositionSideLong
	} else {
		side = types.PositionSideShort
	}

	// Convert margin mode
	marginMode := types.MarginModeCrossed
	if p.PositionMM != "" && !isolatedMargin.IsZero() {
		marginMode = types.MarginModeIsolated
	}

	return &types.Position{
		Symbol:           p.Symbol,
		Side:             side,
		Amount:           size,
		EntryPrice:       entryPrice,
		MarkPrice:        markPrice,
		LiquidationPrice: liquidationPrice,
		UnrealizedPnL:    unrealizedPnl,
		RealizedPnL:      realizedPnl,
		Leverage:         leverage,
		MarginMode:       marginMode,
		IsolatedMargin:   isolatedMargin,
		UpdateTime:       time.Now(),
	}
}

func (b *BybitFutures) convertTrade(t *Trade) *types.Trade {
	price, _ := decimal.NewFromString(t.ExecPrice)
	qty, _ := decimal.NewFromString(t.ExecQty)
	fee, _ := decimal.NewFromString(t.ExecFee)

	trade := &types.Trade{
		TradeID:         t.ExecId,
		OrderID:         t.OrderId,
		ClientOrderID:   t.OrderLinkId,
		Symbol:          t.Symbol,
		Side:            b.parseOrderSide(t.Side),
		Price:           price,
		Quantity:        qty,
		Fee:             fee,
		FeeCurrency:     "USDT", // Bybit linear futures fees are in USDT
		IsMaker:         t.IsMaker,
	}

	// Parse timestamp
	if execTime, err := strconv.ParseInt(t.ExecTime, 10, 64); err == nil {
		trade.Time = time.Unix(0, execTime*int64(time.Millisecond))
	}

	return trade
}

func (b *BybitFutures) convertFuturesSymbolInfo(s *FuturesSymbol) *types.SymbolInfo {
	minQty, _ := decimal.NewFromString(s.LotSizeFilter.MinOrderQty)
	maxQty, _ := decimal.NewFromString(s.LotSizeFilter.MaxOrderQty)
	qtyStep, _ := decimal.NewFromString(s.LotSizeFilter.QtyStep)
	tickSize, _ := decimal.NewFromString(s.PriceFilter.TickSize)
	minLeverage, _ := strconv.Atoi(s.LeverageFilter.MinLeverage)
	maxLeverage, _ := strconv.Atoi(s.LeverageFilter.MaxLeverage)

	// Calculate min notional (min qty * min price)
	minPrice, _ := decimal.NewFromString(s.PriceFilter.MinPrice)
	minNotional := minQty.Mul(minPrice)

	return &types.SymbolInfo{
		Symbol:              s.Symbol,
		BaseAsset:           s.BaseCoin,
		QuoteAsset:          s.QuoteCoin,
		Status:              s.Status,
		MinQty:              minQty,
		MaxQty:              maxQty,
		StepSize:            qtyStep,
		MinNotional:         minNotional,
		TickSize:            tickSize,
		MinLeverage:         minLeverage,
		MaxLeverage:         maxLeverage,
		ContractType:        s.ContractType,
		IsFuturesTradingAllowed: s.Status == "Trading",
		IsSpotTradingAllowed:    false,
		IsMarginTradingAllowed:  false,
	}
}

func (b *BybitFutures) convertTicker(t *Ticker) *types.MarketData {
	lastPrice, _ := decimal.NewFromString(t.LastPrice)
	highPrice, _ := decimal.NewFromString(t.HighPrice24h)
	lowPrice, _ := decimal.NewFromString(t.LowPrice24h)
	volume, _ := decimal.NewFromString(t.Volume24h)
	quoteVolume, _ := decimal.NewFromString(t.Turnover24h)
	priceChange, _ := decimal.NewFromString(t.Price24hPcnt)

	// Calculate price change percent (Bybit provides it as decimal)
	priceChangePercent := priceChange.Mul(decimal.NewFromInt(100))

	return &types.MarketData{
		Symbol:             t.Symbol,
		Price:              lastPrice,
		Bid:                decimal.Zero, // Not provided in ticker
		Ask:                decimal.Zero, // Not provided in ticker
		High24h:            highPrice,
		Low24h:             lowPrice,
		Volume24h:          volume,
		QuoteVolume24h:     quoteVolume,
		PriceChangePercent: priceChangePercent,
		UpdateTime:         time.Now(),
	}
}

func (b *BybitFutures) convertOrderBook(ob *OrderBook) *types.OrderBook {
	orderBook := &types.OrderBook{
		Symbol:     ob.Symbol,
		UpdateTime: time.Unix(0, ob.Ts*int64(time.Millisecond)),
		Bids:       make([]types.PriceLevel, len(ob.Bids)),
		Asks:       make([]types.PriceLevel, len(ob.Asks)),
	}

	// Convert bids
	for i, bid := range ob.Bids {
		if len(bid) >= 2 {
			price, _ := decimal.NewFromString(bid[0])
			qty, _ := decimal.NewFromString(bid[1])
			orderBook.Bids[i] = types.PriceLevel{
				Price:    price,
				Quantity: qty,
			}
		}
	}

	// Convert asks
	for i, ask := range ob.Asks {
		if len(ask) >= 2 {
			price, _ := decimal.NewFromString(ask[0])
			qty, _ := decimal.NewFromString(ask[1])
			orderBook.Asks[i] = types.PriceLevel{
				Price:    price,
				Quantity: qty,
			}
		}
	}

	return orderBook
}

func (b *BybitFutures) convertKlines(klines [][]string) []*types.Kline {
	result := make([]*types.Kline, 0, len(klines))
	
	for _, k := range klines {
		if len(k) >= 7 {
			openTime, _ := strconv.ParseInt(k[0], 10, 64)
			open, _ := decimal.NewFromString(k[1])
			high, _ := decimal.NewFromString(k[2])
			low, _ := decimal.NewFromString(k[3])
			close, _ := decimal.NewFromString(k[4])
			volume, _ := decimal.NewFromString(k[5])
			quoteVolume, _ := decimal.NewFromString(k[6])

			result = append(result, &types.Kline{
				OpenTime:    time.Unix(0, openTime*int64(time.Millisecond)),
				Open:        open,
				High:        high,
				Low:         low,
				Close:       close,
				Volume:      volume,
				QuoteVolume: quoteVolume,
			})
		}
	}

	// Reverse to get oldest first
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return result
}

func (b *BybitFutures) convertInterval(interval types.KlineInterval) string {
	switch interval {
	case types.KlineInterval1m:
		return "1"
	case types.KlineInterval3m:
		return "3"
	case types.KlineInterval5m:
		return "5"
	case types.KlineInterval15m:
		return "15"
	case types.KlineInterval30m:
		return "30"
	case types.KlineInterval1h:
		return "60"
	case types.KlineInterval2h:
		return "120"
	case types.KlineInterval4h:
		return "240"
	case types.KlineInterval6h:
		return "360"
	case types.KlineInterval12h:
		return "720"
	case types.KlineInterval1d:
		return "D"
	case types.KlineInterval1w:
		return "W"
	case types.KlineInterval1M:
		return "M"
	default:
		return "60" // Default to 1h
	}
}

func (b *BybitFutures) parseOrderSide(side string) types.OrderSide {
	switch side {
	case SideBuy:
		return types.OrderSideBuy
	case SideSell:
		return types.OrderSideSell
	default:
		return types.OrderSideBuy
	}
}

func (b *BybitFutures) parseOrderType(orderType string) types.OrderType {
	switch orderType {
	case OrderTypeMarket:
		return types.OrderTypeMarket
	case OrderTypeLimit:
		return types.OrderTypeLimit
	case OrderTypeLimitMaker:
		return types.OrderTypeLimitMaker
	default:
		return types.OrderTypeLimit
	}
}

func (b *BybitFutures) parseOrderStatus(status string) types.OrderStatus {
	switch status {
	case OrderStatusNew:
		return types.OrderStatusNew
	case OrderStatusPartiallyFilled:
		return types.OrderStatusPartiallyFilled
	case OrderStatusFilled:
		return types.OrderStatusFilled
	case OrderStatusCancelled:
		return types.OrderStatusCanceled
	case OrderStatusRejected:
		return types.OrderStatusRejected
	default:
		return types.OrderStatusNew
	}
}