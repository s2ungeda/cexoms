package binance

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/adshao/go-binance/v2"
	"github.com/mExOms/pkg/cache"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// SpotConnector implements types.Exchange interface for Binance Spot
type SpotConnector struct {
	clients     map[string]*binance.Client    // accountID -> client
	wsManagers  map[string]*WsOrderManager    // accountID -> websocket manager
	cache       *cache.MemoryCache
	rateLimiter *cache.RateLimiter
	accountType string // "main" or "sub"
	testnet     bool
	mu          sync.RWMutex

	// WebSocket callbacks
	orderBookCallbacks map[string]types.OrderBookCallback
	tradeCallbacks     map[string]types.TradeCallback
	tickerCallbacks    map[string]types.TickerCallback
	callbackMu         sync.RWMutex

	// WebSocket connections
	wsConnections map[string]chan struct{} // symbol -> stop channel
	wsMu          sync.Mutex
}

// NewSpotConnector creates a new Binance Spot connector with multi-account support
func NewSpotConnector(testnet bool) *SpotConnector {
	return &SpotConnector{
		clients:            make(map[string]*binance.Client),
		wsManagers:         make(map[string]*WsOrderManager),
		cache:              cache.NewMemoryCache(),
		rateLimiter:        cache.NewRateLimiter(1200, time.Minute), // Binance spot limit
		testnet:            testnet,
		orderBookCallbacks: make(map[string]types.OrderBookCallback),
		tradeCallbacks:     make(map[string]types.TradeCallback),
		tickerCallbacks:    make(map[string]types.TickerCallback),
		wsConnections:      make(map[string]chan struct{}),
	}
}

// AddAccount adds a new account client
func (sc *SpotConnector) AddAccount(accountID, apiKey, apiSecret string, accountType string) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	client := binance.NewClient(apiKey, apiSecret)
	if sc.testnet {
		client.BaseURL = "https://testnet.binance.vision/api"
	}

	sc.clients[accountID] = client
	
	// Initialize WebSocket order manager for the account
	wsManager := NewWsOrderManager(apiKey, apiSecret, sc.testnet)
	sc.wsManagers[accountID] = wsManager

	log.Printf("Added Binance Spot account: %s (type: %s)", accountID, accountType)
	return nil
}

// GetClient returns the client for the specified account
func (sc *SpotConnector) GetClient(accountID string) (*binance.Client, error) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	client, exists := sc.clients[accountID]
	if !exists {
		return nil, fmt.Errorf("account %s not found", accountID)
	}
	return client, nil
}

// GetName returns the exchange name
func (sc *SpotConnector) GetName() string {
	return "binance"
}

// GetType returns the exchange type
func (sc *SpotConnector) GetType() types.ExchangeType {
	return types.ExchangeTypeCEX
}

// GetMarketType returns the market type
func (sc *SpotConnector) GetMarketType() types.MarketType {
	return types.MarketTypeSpot
}

// Initialize initializes all account connections
func (sc *SpotConnector) Initialize(ctx context.Context) error {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	// Test connectivity for all accounts
	for accountID, client := range sc.clients {
		if err := client.NewPingService().Do(ctx); err != nil {
			log.Printf("Warning: Cannot ping Binance for account %s: %v", accountID, err)
			continue
		}
		
		// Initialize WebSocket manager
		if wsManager, exists := sc.wsManagers[accountID]; exists {
			if err := wsManager.Initialize(ctx); err != nil {
				log.Printf("Warning: Cannot initialize WebSocket for account %s: %v", accountID, err)
			}
		}
		
		log.Printf("Successfully initialized Binance Spot account: %s", accountID)
	}

	return nil
}

// GetAccountInfo returns account information
func (sc *SpotConnector) GetAccountInfo(ctx context.Context) (*types.AccountInfo, error) {
	return sc.GetAccountInfoForAccount(ctx, "main")
}

// GetAccountInfoForAccount returns account information for a specific account
func (sc *SpotConnector) GetAccountInfoForAccount(ctx context.Context, accountID string) (*types.AccountInfo, error) {
	client, err := sc.GetClient(accountID)
	if err != nil {
		return nil, err
	}

	if !sc.rateLimiter.Allow(fmt.Sprintf("account_info_%s", accountID)) {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	account, err := client.NewGetAccountService().Do(ctx)
	if err != nil {
		return nil, err
	}

	balances := make([]types.Balance, 0)
	for _, b := range account.Balances {
		free, _ := strconv.ParseFloat(b.Free, 64)
		locked, _ := strconv.ParseFloat(b.Locked, 64)
		
		if free > 0 || locked > 0 {
			balances = append(balances, types.Balance{
				Asset:     b.Asset,
				Free:      decimal.NewFromFloat(free),
				Locked:    decimal.NewFromFloat(locked),
				Total:     decimal.NewFromFloat(free + locked),
				Available: decimal.NewFromFloat(free),
			})
		}
	}

	return &types.AccountInfo{
		AccountID:    accountID,
		AccountType:  sc.accountType,
		CanTrade:     account.CanTrade,
		CanWithdraw:  account.CanWithdraw,
		CanDeposit:   account.CanDeposit,
		Balances:     balances,
		Permissions:  account.Permissions,
		UpdateTime:   account.UpdateTime,
		MakerFee:     float64(account.MakerCommission) / 10000, // Binance uses basis points
		TakerFee:     float64(account.TakerCommission) / 10000,
	}, nil
}

// GetBalances returns all balances
func (sc *SpotConnector) GetBalances(ctx context.Context) ([]types.Balance, error) {
	return sc.GetBalancesForAccount(ctx, "main")
}

// GetBalancesForAccount returns balances for a specific account
func (sc *SpotConnector) GetBalancesForAccount(ctx context.Context, accountID string) ([]types.Balance, error) {
	info, err := sc.GetAccountInfoForAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return info.Balances, nil
}

// Transfer operations for sub-accounts
func (sc *SpotConnector) TransferToSubAccount(ctx context.Context, toAccountID string, asset string, amount float64) (string, error) {
	client, err := sc.GetClient("main") // Only main account can initiate transfers
	if err != nil {
		return "", err
	}

	// Use universal transfer API
	res, err := client.NewSubAccountUniversalTransferService().
		FromEmail("").               // Empty means from master account
		ToEmail(toAccountID).        // Sub-account email or ID
		FromAccountType("SPOT").
		ToAccountType("SPOT").
		Asset(asset).
		Amount(amount).
		Do(ctx)
	
	if err != nil {
		return "", err
	}

	return strconv.FormatInt(res.TranId, 10), nil
}

func (sc *SpotConnector) TransferFromSubAccount(ctx context.Context, fromAccountID string, asset string, amount float64) (string, error) {
	client, err := sc.GetClient("main")
	if err != nil {
		return "", err
	}

	res, err := client.NewSubAccountUniversalTransferService().
		FromEmail(fromAccountID).
		ToEmail("").                 // Empty means to master account
		FromAccountType("SPOT").
		ToAccountType("SPOT").
		Asset(asset).
		Amount(amount).
		Do(ctx)
	
	if err != nil {
		return "", err
	}

	return strconv.FormatInt(res.TranId, 10), nil
}

func (sc *SpotConnector) TransferBetweenSubAccounts(ctx context.Context, fromAccountID, toAccountID string, asset string, amount float64) (string, error) {
	client, err := sc.GetClient("main")
	if err != nil {
		return "", err
	}

	res, err := client.NewSubAccountUniversalTransferService().
		FromEmail(fromAccountID).
		ToEmail(toAccountID).
		FromAccountType("SPOT").
		ToAccountType("SPOT").
		Asset(asset).
		Amount(amount).
		Do(ctx)
	
	if err != nil {
		return "", err
	}

	return strconv.FormatInt(res.TranId, 10), nil
}

// PlaceOrder places a new order
func (sc *SpotConnector) PlaceOrder(ctx context.Context, order *types.Order) (*types.Order, error) {
	return sc.PlaceOrderForAccount(ctx, order.AccountID, order)
}

// PlaceOrderForAccount places an order for a specific account
func (sc *SpotConnector) PlaceOrderForAccount(ctx context.Context, accountID string, order *types.Order) (*types.Order, error) {
	// Check if we should use WebSocket
	if wsManager, exists := sc.wsManagers[accountID]; exists && wsManager.IsConnected() {
		return wsManager.PlaceOrder(ctx, order)
	}

	// Fallback to REST API
	client, err := sc.GetClient(accountID)
	if err != nil {
		return nil, err
	}

	if !sc.rateLimiter.Allow(fmt.Sprintf("place_order_%s", accountID)) {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	svc := client.NewCreateOrderService().
		Symbol(order.Symbol).
		Side(binance.SideType(order.Side)).
		Type(binance.OrderType(order.Type))

	switch order.Type {
	case types.OrderTypeLimit:
		svc.TimeInForce(binance.TimeInForceTypeGTC).
			Price(order.Price.String()).
			Quantity(order.Quantity.String())
	case types.OrderTypeMarket:
		svc.Quantity(order.Quantity.String())
	case types.OrderTypeLimitMaker:
		svc.TimeInForce(binance.TimeInForceTypeGTC).
			Price(order.Price.String()).
			Quantity(order.Quantity.String())
	}

	if order.ClientOrderID != "" {
		svc.NewClientOrderID(order.ClientOrderID)
	}

	res, err := svc.Do(ctx)
	if err != nil {
		return nil, err
	}

	// Update order with response
	order.ID = strconv.FormatInt(res.OrderID, 10)
	order.Status = string(res.Status)
	order.CreatedAt = time.Unix(res.TransactTime/1000, (res.TransactTime%1000)*1000000)
	order.UpdatedAt = order.CreatedAt

	if res.Fills != nil && len(res.Fills) > 0 {
		executedQty := decimal.Zero
		executedQuoteQty := decimal.Zero
		totalFee := decimal.Zero
		
		for _, fill := range res.Fills {
			qty, _ := decimal.NewFromString(fill.Qty)
			price, _ := decimal.NewFromString(fill.Price)
			fee, _ := decimal.NewFromString(fill.Commission)
			
			executedQty = executedQty.Add(qty)
			executedQuoteQty = executedQuoteQty.Add(qty.Mul(price))
			totalFee = totalFee.Add(fee)
		}
		
		order.ExecutedQty = executedQty
		order.ExecutedQuoteQty = executedQuoteQty
		order.Fee = totalFee
		order.FeeCurrency = res.Fills[0].CommissionAsset
	}

	return order, nil
}

// CancelOrder cancels an order
func (sc *SpotConnector) CancelOrder(ctx context.Context, symbol string, orderID string) error {
	return sc.CancelOrderForAccount(ctx, "main", symbol, orderID)
}

// CancelOrderForAccount cancels an order for a specific account
func (sc *SpotConnector) CancelOrderForAccount(ctx context.Context, accountID string, symbol string, orderID string) error {
	// Check if we should use WebSocket
	if wsManager, exists := sc.wsManagers[accountID]; exists && wsManager.IsConnected() {
		return wsManager.CancelOrder(ctx, symbol, orderID, "")
	}

	// Fallback to REST API
	client, err := sc.GetClient(accountID)
	if err != nil {
		return err
	}

	if !sc.rateLimiter.Allow(fmt.Sprintf("cancel_order_%s", accountID)) {
		return fmt.Errorf("rate limit exceeded")
	}

	// Try to parse as numeric order ID first
	if orderIDInt, err := strconv.ParseInt(orderID, 10, 64); err == nil {
		_, err := client.NewCancelOrderService().
			Symbol(symbol).
			OrderID(orderIDInt).
			Do(ctx)
		return err
	}

	// Otherwise treat as client order ID
	_, err = client.NewCancelOrderService().
		Symbol(symbol).
		OrigClientOrderID(orderID).
		Do(ctx)
	
	return err
}

// GetOrder retrieves order information
func (sc *SpotConnector) GetOrder(ctx context.Context, symbol string, orderID string) (*types.Order, error) {
	return sc.GetOrderForAccount(ctx, "main", symbol, orderID)
}

// GetOrderForAccount retrieves order information for a specific account
func (sc *SpotConnector) GetOrderForAccount(ctx context.Context, accountID string, symbol string, orderID string) (*types.Order, error) {
	client, err := sc.GetClient(accountID)
	if err != nil {
		return nil, err
	}

	if !sc.rateLimiter.Allow(fmt.Sprintf("get_order_%s", accountID)) {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	var order *binance.Order
	
	// Try to parse as numeric order ID first
	if orderIDInt, err := strconv.ParseInt(orderID, 10, 64); err == nil {
		order, err = client.NewGetOrderService().
			Symbol(symbol).
			OrderID(orderIDInt).
			Do(ctx)
	} else {
		// Otherwise treat as client order ID
		order, err = client.NewGetOrderService().
			Symbol(symbol).
			OrigClientOrderID(orderID).
			Do(ctx)
	}

	if err != nil {
		return nil, err
	}

	return sc.convertBinanceOrder(order, accountID), nil
}

// GetOpenOrders retrieves all open orders
func (sc *SpotConnector) GetOpenOrders(ctx context.Context, symbol string) ([]*types.Order, error) {
	return sc.GetOpenOrdersForAccount(ctx, "main", symbol)
}

// GetOpenOrdersForAccount retrieves open orders for a specific account
func (sc *SpotConnector) GetOpenOrdersForAccount(ctx context.Context, accountID string, symbol string) ([]*types.Order, error) {
	client, err := sc.GetClient(accountID)
	if err != nil {
		return nil, err
	}

	if !sc.rateLimiter.Allow(fmt.Sprintf("open_orders_%s", accountID)) {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	svc := client.NewListOpenOrdersService()
	if symbol != "" {
		svc.Symbol(symbol)
	}

	orders, err := svc.Do(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*types.Order, 0, len(orders))
	for _, order := range orders {
		result = append(result, sc.convertBinanceOrder(order, accountID))
	}

	return result, nil
}

// GetOrderHistory retrieves order history
func (sc *SpotConnector) GetOrderHistory(ctx context.Context, symbol string, limit int) ([]*types.Order, error) {
	return sc.GetOrderHistoryForAccount(ctx, "main", symbol, limit)
}

// GetOrderHistoryForAccount retrieves order history for a specific account
func (sc *SpotConnector) GetOrderHistoryForAccount(ctx context.Context, accountID string, symbol string, limit int) ([]*types.Order, error) {
	client, err := sc.GetClient(accountID)
	if err != nil {
		return nil, err
	}

	if !sc.rateLimiter.Allow(fmt.Sprintf("order_history_%s", accountID)) {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	svc := client.NewListOrdersService().Symbol(symbol)
	if limit > 0 {
		svc.Limit(limit)
	}

	orders, err := svc.Do(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*types.Order, 0, len(orders))
	for _, order := range orders {
		result = append(result, sc.convertBinanceOrder(order, accountID))
	}

	return result, nil
}

// GetTrades retrieves recent trades
func (sc *SpotConnector) GetTrades(ctx context.Context, symbol string, limit int) ([]*types.Trade, error) {
	return sc.GetTradesForAccount(ctx, "main", symbol, limit)
}

// GetTradesForAccount retrieves trades for a specific account
func (sc *SpotConnector) GetTradesForAccount(ctx context.Context, accountID string, symbol string, limit int) ([]*types.Trade, error) {
	client, err := sc.GetClient(accountID)
	if err != nil {
		return nil, err
	}

	if !sc.rateLimiter.Allow(fmt.Sprintf("trades_%s", accountID)) {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	svc := client.NewListTradesService().Symbol(symbol)
	if limit > 0 {
		svc.Limit(limit)
	}

	trades, err := svc.Do(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*types.Trade, 0, len(trades))
	for _, trade := range trades {
		price, _ := strconv.ParseFloat(trade.Price, 64)
		quantity, _ := strconv.ParseFloat(trade.Quantity, 64)
		quoteQty, _ := strconv.ParseFloat(trade.QuoteQuantity, 64)
		commission, _ := strconv.ParseFloat(trade.Commission, 64)

		result = append(result, &types.Trade{
			ID:              strconv.FormatInt(trade.ID, 10),
			OrderID:         strconv.FormatInt(trade.OrderID, 10),
			Symbol:          symbol,
			Price:           decimal.NewFromFloat(price),
			Quantity:        decimal.NewFromFloat(quantity),
			QuoteQuantity:   decimal.NewFromFloat(quoteQty),
			Commission:      decimal.NewFromFloat(commission),
			CommissionAsset: trade.CommissionAsset,
			Time:            time.Unix(trade.Time/1000, (trade.Time%1000)*1000000),
			IsBuyer:         trade.IsBuyer,
			IsMaker:         trade.IsMaker,
			IsBestMatch:     trade.IsBestMatch,
		})
	}

	return result, nil
}

// Market data methods
func (sc *SpotConnector) GetSymbolInfo(ctx context.Context, symbol string) (*types.SymbolInfo, error) {
	// Use cache first
	cacheKey := fmt.Sprintf("symbol_info_%s", symbol)
	if cached, exists := sc.cache.Get(cacheKey); exists {
		return cached.(*types.SymbolInfo), nil
	}

	client, err := sc.GetClient("main")
	if err != nil {
		return nil, err
	}

	if !sc.rateLimiter.Allow("exchange_info") {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	info, err := client.NewExchangeInfoService().Symbol(symbol).Do(ctx)
	if err != nil {
		return nil, err
	}

	for _, s := range info.Symbols {
		if s.Symbol == symbol {
			symbolInfo := sc.convertBinanceSymbol(&s)
			
			// Cache for 1 hour
			sc.cache.Set(cacheKey, symbolInfo, time.Hour)
			
			return symbolInfo, nil
		}
	}

	return nil, fmt.Errorf("symbol %s not found", symbol)
}

// GetMarketData retrieves market data for multiple symbols
func (sc *SpotConnector) GetMarketData(ctx context.Context, symbols []string) (map[string]*types.MarketData, error) {
	client, err := sc.GetClient("main")
	if err != nil {
		return nil, err
	}

	if !sc.rateLimiter.Allow("market_data") {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	// Get 24hr ticker for all symbols
	tickers, err := client.NewListPrice24HrService().Do(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*types.MarketData)
	symbolSet := make(map[string]bool)
	for _, s := range symbols {
		symbolSet[s] = true
	}

	for _, ticker := range tickers {
		if _, ok := symbolSet[ticker.Symbol]; ok || len(symbols) == 0 {
			lastPrice, _ := strconv.ParseFloat(ticker.LastPrice, 64)
			bidPrice, _ := strconv.ParseFloat(ticker.BidPrice, 64)
			askPrice, _ := strconv.ParseFloat(ticker.AskPrice, 64)
			volume, _ := strconv.ParseFloat(ticker.Volume, 64)
			quoteVolume, _ := strconv.ParseFloat(ticker.QuoteVolume, 64)
			high, _ := strconv.ParseFloat(ticker.HighPrice, 64)
			low, _ := strconv.ParseFloat(ticker.LowPrice, 64)
			openPrice, _ := strconv.ParseFloat(ticker.OpenPrice, 64)

			result[ticker.Symbol] = &types.MarketData{
				Symbol:      ticker.Symbol,
				LastPrice:   decimal.NewFromFloat(lastPrice),
				BidPrice:    decimal.NewFromFloat(bidPrice),
				AskPrice:    decimal.NewFromFloat(askPrice),
				Volume:      decimal.NewFromFloat(volume),
				QuoteVolume: decimal.NewFromFloat(quoteVolume),
				High24h:     decimal.NewFromFloat(high),
				Low24h:      decimal.NewFromFloat(low),
				OpenPrice:   decimal.NewFromFloat(openPrice),
				Timestamp:   time.Now(),
			}
		}
	}

	return result, nil
}

// GetOrderBook retrieves the order book
func (sc *SpotConnector) GetOrderBook(ctx context.Context, symbol string, depth int) (*types.OrderBook, error) {
	client, err := sc.GetClient("main")
	if err != nil {
		return nil, err
	}

	if !sc.rateLimiter.Allow("order_book") {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	svc := client.NewDepthService().Symbol(symbol)
	if depth > 0 {
		svc.Limit(depth)
	}

	book, err := svc.Do(ctx)
	if err != nil {
		return nil, err
	}

	orderBook := &types.OrderBook{
		Symbol:    symbol,
		Timestamp: time.Now(),
		Bids:      make([]types.OrderBookEntry, 0, len(book.Bids)),
		Asks:      make([]types.OrderBookEntry, 0, len(book.Asks)),
	}

	for _, bid := range book.Bids {
		price, _ := strconv.ParseFloat(bid.Price, 64)
		quantity, _ := strconv.ParseFloat(bid.Quantity, 64)
		orderBook.Bids = append(orderBook.Bids, types.OrderBookEntry{
			Price:    decimal.NewFromFloat(price),
			Quantity: decimal.NewFromFloat(quantity),
		})
	}

	for _, ask := range book.Asks {
		price, _ := strconv.ParseFloat(ask.Price, 64)
		quantity, _ := strconv.ParseFloat(ask.Quantity, 64)
		orderBook.Asks = append(orderBook.Asks, types.OrderBookEntry{
			Price:    decimal.NewFromFloat(price),
			Quantity: decimal.NewFromFloat(quantity),
		})
	}

	return orderBook, nil
}

// GetKlines retrieves kline/candlestick data
func (sc *SpotConnector) GetKlines(ctx context.Context, symbol string, interval types.KlineInterval, limit int) ([]*types.Kline, error) {
	client, err := sc.GetClient("main")
	if err != nil {
		return nil, err
	}

	if !sc.rateLimiter.Allow("klines") {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	svc := client.NewKlinesService().
		Symbol(symbol).
		Interval(string(interval))
	
	if limit > 0 {
		svc.Limit(limit)
	}

	klines, err := svc.Do(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*types.Kline, 0, len(klines))
	for _, k := range klines {
		open, _ := strconv.ParseFloat(k.Open, 64)
		high, _ := strconv.ParseFloat(k.High, 64)
		low, _ := strconv.ParseFloat(k.Low, 64)
		close, _ := strconv.ParseFloat(k.Close, 64)
		volume, _ := strconv.ParseFloat(k.Volume, 64)
		quoteVolume, _ := strconv.ParseFloat(k.QuoteAssetVolume, 64)

		result = append(result, &types.Kline{
			Symbol:       symbol,
			Interval:     interval,
			OpenTime:     time.Unix(k.OpenTime/1000, (k.OpenTime%1000)*1000000),
			CloseTime:    time.Unix(k.CloseTime/1000, (k.CloseTime%1000)*1000000),
			Open:         decimal.NewFromFloat(open),
			High:         decimal.NewFromFloat(high),
			Low:          decimal.NewFromFloat(low),
			Close:        decimal.NewFromFloat(close),
			Volume:       decimal.NewFromFloat(volume),
			QuoteVolume:  decimal.NewFromFloat(quoteVolume),
			TradesCount:  k.TradeNum,
		})
	}

	return result, nil
}

// WebSocket operations
func (sc *SpotConnector) SubscribeOrderBook(symbol string, callback types.OrderBookCallback) error {
	sc.callbackMu.Lock()
	sc.orderBookCallbacks[symbol] = callback
	sc.callbackMu.Unlock()

	sc.wsMu.Lock()
	defer sc.wsMu.Unlock()

	// Check if already subscribed
	if _, exists := sc.wsConnections[fmt.Sprintf("orderbook_%s", symbol)]; exists {
		return nil
	}

	symbol = strings.ToLower(symbol)
	doneC, stopC, err := binance.WsPartialDepthServe(symbol, "20", func(event *binance.WsPartialDepthEvent) {
		orderBook := &types.OrderBook{
			Symbol:    strings.ToUpper(event.Symbol),
			Timestamp: time.Now(),
			Bids:      make([]types.OrderBookEntry, 0, len(event.Bids)),
			Asks:      make([]types.OrderBookEntry, 0, len(event.Asks)),
		}

		for _, bid := range event.Bids {
			orderBook.Bids = append(orderBook.Bids, types.OrderBookEntry{
				Price:    bid.Price,
				Quantity: bid.Quantity,
			})
		}

		for _, ask := range event.Asks {
			orderBook.Asks = append(orderBook.Asks, types.OrderBookEntry{
				Price:    ask.Price,
				Quantity: ask.Quantity,
			})
		}

		sc.callbackMu.RLock()
		if cb, ok := sc.orderBookCallbacks[strings.ToUpper(event.Symbol)]; ok {
			cb(orderBook)
		}
		sc.callbackMu.RUnlock()
	}, func(err error) {
		log.Printf("OrderBook WebSocket error for %s: %v", symbol, err)
	})

	if err != nil {
		return err
	}

	sc.wsConnections[fmt.Sprintf("orderbook_%s", symbol)] = stopC

	// Monitor connection
	go func() {
		<-doneC
		sc.wsMu.Lock()
		delete(sc.wsConnections, fmt.Sprintf("orderbook_%s", symbol))
		sc.wsMu.Unlock()
	}()

	return nil
}

func (sc *SpotConnector) SubscribeTrades(symbol string, callback types.TradeCallback) error {
	sc.callbackMu.Lock()
	sc.tradeCallbacks[symbol] = callback
	sc.callbackMu.Unlock()

	sc.wsMu.Lock()
	defer sc.wsMu.Unlock()

	// Check if already subscribed
	if _, exists := sc.wsConnections[fmt.Sprintf("trades_%s", symbol)]; exists {
		return nil
	}

	symbol = strings.ToLower(symbol)
	doneC, stopC, err := binance.WsTradeServe(symbol, func(event *binance.WsTradeEvent) {
		trade := &types.Trade{
			ID:            strconv.FormatInt(event.TradeID, 10),
			Symbol:        strings.ToUpper(event.Symbol),
			Price:         decimal.RequireFromString(event.Price),
			Quantity:      decimal.RequireFromString(event.Quantity),
			Time:          time.Unix(event.Time/1000, (event.Time%1000)*1000000),
			IsBuyerMaker:  event.IsBuyerMaker,
		}

		sc.callbackMu.RLock()
		if cb, ok := sc.tradeCallbacks[strings.ToUpper(event.Symbol)]; ok {
			cb(trade)
		}
		sc.callbackMu.RUnlock()
	}, func(err error) {
		log.Printf("Trade WebSocket error for %s: %v", symbol, err)
	})

	if err != nil {
		return err
	}

	sc.wsConnections[fmt.Sprintf("trades_%s", symbol)] = stopC

	// Monitor connection
	go func() {
		<-doneC
		sc.wsMu.Lock()
		delete(sc.wsConnections, fmt.Sprintf("trades_%s", symbol))
		sc.wsMu.Unlock()
	}()

	return nil
}

func (sc *SpotConnector) SubscribeTicker(symbol string, callback types.TickerCallback) error {
	sc.callbackMu.Lock()
	sc.tickerCallbacks[symbol] = callback
	sc.callbackMu.Unlock()

	sc.wsMu.Lock()
	defer sc.wsMu.Unlock()

	// Check if already subscribed
	if _, exists := sc.wsConnections[fmt.Sprintf("ticker_%s", symbol)]; exists {
		return nil
	}

	symbol = strings.ToLower(symbol)
	doneC, stopC, err := binance.WsBookTickerServe(symbol, func(event *binance.WsBookTickerEvent) {
		ticker := &types.Ticker{
			Symbol:   strings.ToUpper(event.Symbol),
			BidPrice: decimal.RequireFromString(event.BestBidPrice),
			BidQty:   decimal.RequireFromString(event.BestBidQty),
			AskPrice: decimal.RequireFromString(event.BestAskPrice),
			AskQty:   decimal.RequireFromString(event.BestAskQty),
		}

		sc.callbackMu.RLock()
		if cb, ok := sc.tickerCallbacks[strings.ToUpper(event.Symbol)]; ok {
			cb(ticker)
		}
		sc.callbackMu.RUnlock()
	}, func(err error) {
		log.Printf("Ticker WebSocket error for %s: %v", symbol, err)
	})

	if err != nil {
		return err
	}

	sc.wsConnections[fmt.Sprintf("ticker_%s", symbol)] = stopC

	// Monitor connection
	go func() {
		<-doneC
		sc.wsMu.Lock()
		delete(sc.wsConnections, fmt.Sprintf("ticker_%s", symbol))
		sc.wsMu.Unlock()
	}()

	return nil
}

func (sc *SpotConnector) UnsubscribeAll() error {
	sc.wsMu.Lock()
	defer sc.wsMu.Unlock()

	// Close all WebSocket connections
	for _, stopC := range sc.wsConnections {
		close(stopC)
	}

	// Clear connections map
	sc.wsConnections = make(map[string]chan struct{})

	// Clear callbacks
	sc.callbackMu.Lock()
	sc.orderBookCallbacks = make(map[string]types.OrderBookCallback)
	sc.tradeCallbacks = make(map[string]types.TradeCallback)
	sc.tickerCallbacks = make(map[string]types.TickerCallback)
	sc.callbackMu.Unlock()

	return nil
}

// Helper methods
func (sc *SpotConnector) convertBinanceOrder(order *binance.Order, accountID string) *types.Order {
	price, _ := decimal.NewFromString(order.Price)
	quantity, _ := decimal.NewFromString(order.OrigQuantity)
	executedQty, _ := decimal.NewFromString(order.ExecutedQuantity)
	
	// Calculate executed quote quantity
	cummulativeQuoteQty, _ := decimal.NewFromString(order.CummulativeQuoteQuantity)

	return &types.Order{
		ID:               strconv.FormatInt(order.OrderID, 10),
		ClientOrderID:    order.ClientOrderID,
		AccountID:        accountID,
		Symbol:           order.Symbol,
		Side:             string(order.Side),
		Type:             string(order.Type),
		Status:           string(order.Status),
		Price:            price,
		Quantity:         quantity,
		ExecutedQty:      executedQty,
		ExecutedQuoteQty: cummulativeQuoteQty,
		TimeInForce:      string(order.TimeInForce),
		CreatedAt:        time.Unix(order.Time/1000, (order.Time%1000)*1000000),
		UpdatedAt:        time.Unix(order.UpdateTime/1000, (order.UpdateTime%1000)*1000000),
	}
}

func (sc *SpotConnector) convertBinanceSymbol(symbol *binance.Symbol) *types.SymbolInfo {
	// Find lot size filter
	var minQty, maxQty, stepSize decimal.Decimal
	var minNotional decimal.Decimal

	for _, filter := range symbol.Filters {
		switch filter["filterType"] {
		case "LOT_SIZE":
			minQty, _ = decimal.NewFromString(filter["minQty"].(string))
			maxQty, _ = decimal.NewFromString(filter["maxQty"].(string))
			stepSize, _ = decimal.NewFromString(filter["stepSize"].(string))
		case "MIN_NOTIONAL":
			minNotional, _ = decimal.NewFromString(filter["minNotional"].(string))
		}
	}

	return &types.SymbolInfo{
		Symbol:              symbol.Symbol,
		BaseAsset:           symbol.BaseAsset,
		QuoteAsset:          symbol.QuoteAsset,
		Status:              symbol.Status,
		MinQty:              minQty,
		MaxQty:              maxQty,
		StepSize:            stepSize,
		MinNotional:         minNotional,
		IsSpotTradingAllowed: symbol.IsSpotTradingAllowed,
		IsMarginTradingAllowed: symbol.IsMarginTradingAllowed,
	}
}