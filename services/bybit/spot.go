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

// BybitSpot implements the Exchange interface for Bybit Spot trading
type BybitSpot struct {
	client       *Client
	exchangeType types.ExchangeType
	marketType   types.MarketType
	symbolsCache map[string]*Symbol
	lastUpdate   time.Time
}

// NewBybitSpot creates a new Bybit Spot exchange instance
func NewBybitSpot(apiKey, apiSecret string, testnet bool) *BybitSpot {
	return &BybitSpot{
		client:       NewClient(apiKey, apiSecret, testnet),
		exchangeType: types.ExchangeBybit,
		marketType:   types.MarketTypeSpot,
		symbolsCache: make(map[string]*Symbol),
	}
}

// GetName returns the exchange name
func (b *BybitSpot) GetName() string {
	return string(b.exchangeType)
}

// GetType returns the exchange type
func (b *BybitSpot) GetType() types.ExchangeType {
	return b.exchangeType
}

// GetMarketType returns the market type
func (b *BybitSpot) GetMarketType() types.MarketType {
	return b.marketType
}

// Initialize initializes the exchange
func (b *BybitSpot) Initialize(ctx context.Context) error {
	// Load symbols
	if err := b.loadSymbols(); err != nil {
		return fmt.Errorf("failed to load symbols: %w", err)
	}

	// Test connectivity
	if _, err := b.client.GetServerTime(); err != nil {
		return fmt.Errorf("failed to connect to Bybit: %w", err)
	}

	return nil
}

// GetAccountInfo returns account information
func (b *BybitSpot) GetAccountInfo(ctx context.Context) (*types.AccountInfo, error) {
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
		AccountType: accountResult.AccountType,
		Balances:    balances,
		UpdateTime:  time.Now(),
	}, nil
}

// GetBalances returns account balances
func (b *BybitSpot) GetBalances(ctx context.Context) ([]types.Balance, error) {
	params := map[string]interface{}{
		"accountType": "SPOT",
	}

	var result struct {
		List []struct {
			Coin     []SpotBalance `json:"coin"`
		} `json:"list"`
	}

	err := b.client.Request(http.MethodGet, "/account/wallet-balance", params, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to get balances: %w", err)
	}

	var balances []types.Balance
	if len(result.List) > 0 && len(result.List[0].Coin) > 0 {
		for _, b := range result.List[0].Coin {
			free, _ := decimal.NewFromString(b.Free)
			locked, _ := decimal.NewFromString(b.Locked)
			total, _ := decimal.NewFromString(b.Total)

			balances = append(balances, types.Balance{
				Asset:  b.Coin,
				Free:   free,
				Locked: locked,
				Total:  total,
			})
		}
	}

	return balances, nil
}

// PlaceOrder places an order
func (b *BybitSpot) PlaceOrder(ctx context.Context, order *types.Order) (*types.Order, error) {
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
		"category":    CategorySpot,
		"symbol":      order.Symbol,
		"side":        side,
		"orderType":   orderType,
		"qty":         order.Quantity.String(),
		"timeInForce": TimeInForceGTC,
		"orderLinkId": order.ClientOrderID,
	}

	// Add price for limit orders
	if order.Type == types.OrderTypeLimit || order.Type == types.OrderTypeLimitMaker {
		params["price"] = order.Price.String()
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
func (b *BybitSpot) CancelOrder(ctx context.Context, symbol, orderID string) error {
	params := map[string]interface{}{
		"category": CategorySpot,
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
func (b *BybitSpot) GetOrder(ctx context.Context, symbol, orderID string) (*types.Order, error) {
	params := map[string]interface{}{
		"category": CategorySpot,
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
func (b *BybitSpot) GetOpenOrders(ctx context.Context, symbol string) ([]*types.Order, error) {
	params := map[string]interface{}{
		"category": CategorySpot,
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

// GetOrderHistory gets order history
func (b *BybitSpot) GetOrderHistory(ctx context.Context, symbol string, limit int) ([]*types.Order, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	params := map[string]interface{}{
		"category": CategorySpot,
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

// GetTrades gets trades for a specific order or symbol
func (b *BybitSpot) GetTrades(ctx context.Context, symbol string, limit int) ([]*types.Trade, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	params := map[string]interface{}{
		"category": CategorySpot,
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

// GetSymbolInfo gets symbol trading rules
func (b *BybitSpot) GetSymbolInfo(ctx context.Context, symbol string) (*types.SymbolInfo, error) {
	// Check cache first
	if sym, ok := b.symbolsCache[symbol]; ok {
		return b.convertSymbolInfo(sym), nil
	}

	// Reload symbols if not in cache
	if err := b.loadSymbols(); err != nil {
		return nil, err
	}

	sym, ok := b.symbolsCache[symbol]
	if !ok {
		return nil, fmt.Errorf("symbol %s not found", symbol)
	}

	return b.convertSymbolInfo(sym), nil
}

// GetMarketData gets current market data
func (b *BybitSpot) GetMarketData(ctx context.Context, symbols []string) (map[string]*types.MarketData, error) {
	params := map[string]interface{}{
		"category": CategorySpot,
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

// GetOrderBook gets order book for a symbol
func (b *BybitSpot) GetOrderBook(ctx context.Context, symbol string, depth int) (*types.OrderBook, error) {
	if depth <= 0 || depth > 200 {
		depth = 50
	}

	params := map[string]interface{}{
		"category": CategorySpot,
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

// GetKlines gets candlestick data
func (b *BybitSpot) GetKlines(ctx context.Context, symbol string, interval types.KlineInterval, limit int) ([]*types.Kline, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}

	params := map[string]interface{}{
		"category": CategorySpot,
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

func (b *BybitSpot) loadSymbols() error {
	params := map[string]interface{}{
		"category": CategorySpot,
	}

	var result struct {
		List []Symbol `json:"list"`
	}

	err := b.client.PublicRequest(http.MethodGet, "/market/instruments-info", params, &result)
	if err != nil {
		return fmt.Errorf("failed to get symbols: %w", err)
	}

	// Update cache
	b.symbolsCache = make(map[string]*Symbol)
	for i := range result.List {
		sym := &result.List[i]
		b.symbolsCache[sym.Symbol] = sym
	}
	b.lastUpdate = time.Now()

	return nil
}

func (b *BybitSpot) validateOrder(order *types.Order) error {
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

func (b *BybitSpot) convertOrderType(orderType types.OrderType) string {
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

func (b *BybitSpot) convertOrderSide(side types.OrderSide) string {
	switch side {
	case types.OrderSideBuy:
		return SideBuy
	case types.OrderSideSell:
		return SideSell
	default:
		return SideBuy
	}
}

func (b *BybitSpot) convertOrder(o *Order) *types.Order {
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

func (b *BybitSpot) convertTrade(t *Trade) *types.Trade {
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
		FeeCurrency:     "", // Bybit doesn't provide fee currency in execution response
		IsMaker:         t.IsMaker,
	}

	// Parse timestamp
	if execTime, err := strconv.ParseInt(t.ExecTime, 10, 64); err == nil {
		trade.Time = time.Unix(0, execTime*int64(time.Millisecond))
	}

	return trade
}

func (b *BybitSpot) convertSymbolInfo(s *Symbol) *types.SymbolInfo {
	minQty, _ := decimal.NewFromString(s.MinOrderQty)
	maxQty, _ := decimal.NewFromString(s.MaxOrderQty)
	minNotional, _ := decimal.NewFromString(s.MinOrderAmt)
	tickSize, _ := decimal.NewFromString(s.TickSize)
	basePrecision, _ := strconv.Atoi(s.BasePrecision)
	quotePrecision, _ := strconv.Atoi(s.QuotePrecision)

	return &types.SymbolInfo{
		Symbol:         s.Symbol,
		BaseAsset:      s.BaseCoin,
		QuoteAsset:     s.QuoteCoin,
		Status:         s.Status,
		MinQty:         minQty,
		MaxQty:         maxQty,
		MinNotional:    minNotional,
		TickSize:       tickSize,
		BasePrecision:  basePrecision,
		QuotePrecision: quotePrecision,
		IsSpotTradingAllowed: s.Status == "Trading",
		IsMarginTradingAllowed: false, // Bybit spot doesn't support margin
	}
}

func (b *BybitSpot) convertTicker(t *Ticker) *types.MarketData {
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

func (b *BybitSpot) convertOrderBook(ob *OrderBook) *types.OrderBook {
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

func (b *BybitSpot) convertKlines(klines [][]string) []*types.Kline {
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

func (b *BybitSpot) convertInterval(interval types.KlineInterval) string {
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

func (b *BybitSpot) parseOrderSide(side string) types.OrderSide {
	switch side {
	case SideBuy:
		return types.OrderSideBuy
	case SideSell:
		return types.OrderSideSell
	default:
		return types.OrderSideBuy
	}
}

func (b *BybitSpot) parseOrderType(orderType string) types.OrderType {
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

func (b *BybitSpot) parseOrderStatus(status string) types.OrderStatus {
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