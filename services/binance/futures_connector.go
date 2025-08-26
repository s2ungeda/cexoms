package binance

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/adshao/go-binance/v2/futures"
	"github.com/mExOms/pkg/cache"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// FuturesConnector implements types.FuturesExchange interface for Binance Futures
type FuturesConnector struct {
	clients     map[string]*futures.Client           // accountID -> client
	wsManagers  map[string]*FuturesWsOrderManager    // accountID -> websocket manager
	cache       *cache.MemoryCache
	rateLimiter *cache.MultiAccountRateLimiter
	accountType string // "main" or "sub"
	testnet     bool
	mu          sync.RWMutex

	// Position tracking
	positions     map[string]map[string]*types.Position // accountID -> symbol -> position
	positionMu    sync.RWMutex
	
	// WebSocket callbacks
	orderBookCallbacks map[string]types.OrderBookCallback
	tradeCallbacks     map[string]types.TradeCallback
	tickerCallbacks    map[string]types.TickerCallback
	callbackMu         sync.RWMutex
}

// NewFuturesConnector creates a new Binance Futures connector with multi-account support
func NewFuturesConnector(testnet bool) *FuturesConnector {
	return &FuturesConnector{
		clients:            make(map[string]*futures.Client),
		wsManagers:         make(map[string]*FuturesWsOrderManager),
		cache:              cache.NewMemoryCache(),
		rateLimiter:        cache.NewMultiAccountRateLimiter(10000, time.Minute), // Global limit
		testnet:            testnet,
		positions:          make(map[string]map[string]*types.Position),
		orderBookCallbacks: make(map[string]types.OrderBookCallback),
		tradeCallbacks:     make(map[string]types.TradeCallback),
		tickerCallbacks:    make(map[string]types.TickerCallback),
	}
}

// AddAccount adds a new account client
func (fc *FuturesConnector) AddAccount(accountID, apiKey, apiSecret string, accountType string) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if fc.testnet {
		futures.UseTestnet = true
	}
	
	client := futures.NewClient(apiKey, apiSecret)
	fc.clients[accountID] = client
	
	// Add rate limiter for this account
	fc.rateLimiter.AddAccount(accountID, 2400, time.Minute) // Futures has higher limit
	
	// Initialize position map for this account
	fc.positions[accountID] = make(map[string]*types.Position)
	
	// Initialize WebSocket order manager
	wsManager := NewFuturesWsOrderManager(apiKey, apiSecret, fc.testnet)
	fc.wsManagers[accountID] = wsManager

	log.Printf("Added Binance Futures account: %s (type: %s)", accountID, accountType)
	return nil
}

// GetClient returns the client for the specified account
func (fc *FuturesConnector) GetClient(accountID string) (*futures.Client, error) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	client, exists := fc.clients[accountID]
	if !exists {
		return nil, fmt.Errorf("account %s not found", accountID)
	}
	return client, nil
}

// Base Exchange methods
func (fc *FuturesConnector) GetName() string {
	return "binance"
}

func (fc *FuturesConnector) GetType() types.ExchangeType {
	return types.ExchangeTypeCEX
}

func (fc *FuturesConnector) GetMarketType() types.MarketType {
	return types.MarketTypeFutures
}

// Initialize initializes all account connections
func (fc *FuturesConnector) Initialize(ctx context.Context) error {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	// Test connectivity for all accounts
	for accountID, client := range fc.clients {
		if err := client.NewPingService().Do(ctx); err != nil {
			log.Printf("Warning: Cannot ping Binance Futures for account %s: %v", accountID, err)
			continue
		}
		
		// Initialize WebSocket manager
		if wsManager, exists := fc.wsManagers[accountID]; exists {
			if err := wsManager.Initialize(ctx); err != nil {
				log.Printf("Warning: Cannot initialize WebSocket for account %s: %v", accountID, err)
			}
		}
		
		// Load initial positions
		if err := fc.loadPositions(ctx, accountID); err != nil {
			log.Printf("Warning: Cannot load positions for account %s: %v", accountID, err)
		}
		
		log.Printf("Successfully initialized Binance Futures account: %s", accountID)
	}

	return nil
}

// GetAccountInfo returns account information
func (fc *FuturesConnector) GetAccountInfo(ctx context.Context) (*types.AccountInfo, error) {
	return fc.GetAccountInfoForAccount(ctx, "main")
}

// GetAccountInfoForAccount returns account information for a specific account
func (fc *FuturesConnector) GetAccountInfoForAccount(ctx context.Context, accountID string) (*types.AccountInfo, error) {
	client, err := fc.GetClient(accountID)
	if err != nil {
		return nil, err
	}

	if !fc.rateLimiter.Allow(accountID, "account_info") {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	account, err := client.NewGetAccountService().Do(ctx)
	if err != nil {
		return nil, err
	}

	balances := make([]types.Balance, 0)
	
	// Aggregate all assets
	for _, asset := range account.Assets {
		walletBalance, _ := strconv.ParseFloat(asset.WalletBalance, 64)
		unrealizedPnL, _ := strconv.ParseFloat(asset.UnrealizedProfit, 64)
		marginBalance, _ := strconv.ParseFloat(asset.MarginBalance, 64)
		availableBalance, _ := strconv.ParseFloat(asset.AvailableBalance, 64)
		
		if walletBalance > 0 || unrealizedPnL != 0 {
			balances = append(balances, types.Balance{
				Asset:         asset.Asset,
				Free:          decimal.NewFromFloat(availableBalance),
				Locked:        decimal.NewFromFloat(marginBalance - availableBalance),
				Total:         decimal.NewFromFloat(walletBalance),
				Available:     decimal.NewFromFloat(availableBalance),
				UnrealizedPnL: decimal.NewFromFloat(unrealizedPnL),
			})
		}
	}

	totalBalance, _ := strconv.ParseFloat(account.TotalWalletBalance, 64)
	totalUnrealizedPnL, _ := strconv.ParseFloat(account.TotalUnrealizedProfit, 64)
	
	return &types.AccountInfo{
		AccountID:       accountID,
		AccountType:     fc.accountType,
		CanTrade:       account.CanTrade,
		CanWithdraw:    account.CanWithdraw,
		CanDeposit:     account.CanDeposit,
		Balances:       balances,
		UpdateTime:     account.UpdateTime,
		TotalBalance:   decimal.NewFromFloat(totalBalance),
		UnrealizedPnL:  decimal.NewFromFloat(totalUnrealizedPnL),
		MakerFee:       float64(account.FeeTier) * 0.0001, // Convert basis points
		TakerFee:       float64(account.FeeTier) * 0.0001,
	}, nil
}

// GetBalances returns all balances
func (fc *FuturesConnector) GetBalances(ctx context.Context) ([]types.Balance, error) {
	return fc.GetBalancesForAccount(ctx, "main")
}

// GetBalancesForAccount returns balances for a specific account
func (fc *FuturesConnector) GetBalancesForAccount(ctx context.Context, accountID string) ([]types.Balance, error) {
	info, err := fc.GetAccountInfoForAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return info.Balances, nil
}

// Transfer operations for futures sub-accounts
func (fc *FuturesConnector) TransferToSubAccount(ctx context.Context, toAccountID string, asset string, amount float64) (string, error) {
	client, err := fc.GetClient("main")
	if err != nil {
		return "", err
	}

	// For futures, we need to use the futures transfer endpoint
	// This is a placeholder - actual implementation depends on Binance API
	_ = client
	return "", fmt.Errorf("futures sub-account transfer not implemented")
}

func (fc *FuturesConnector) TransferFromSubAccount(ctx context.Context, fromAccountID string, asset string, amount float64) (string, error) {
	return "", fmt.Errorf("futures sub-account transfer not implemented")
}

func (fc *FuturesConnector) TransferBetweenSubAccounts(ctx context.Context, fromAccountID, toAccountID string, asset string, amount float64) (string, error) {
	return "", fmt.Errorf("futures sub-account transfer not implemented")
}

// PlaceOrder places a new order
func (fc *FuturesConnector) PlaceOrder(ctx context.Context, order *types.Order) (*types.Order, error) {
	return fc.PlaceOrderForAccount(ctx, order.AccountID, order)
}

// PlaceOrderForAccount places an order for a specific account
func (fc *FuturesConnector) PlaceOrderForAccount(ctx context.Context, accountID string, order *types.Order) (*types.Order, error) {
	// Check if we should use WebSocket
	if wsManager, exists := fc.wsManagers[accountID]; exists && wsManager.IsConnected() {
		return wsManager.PlaceOrder(ctx, order)
	}

	// Fallback to REST API
	client, err := fc.GetClient(accountID)
	if err != nil {
		return nil, err
	}

	if !fc.rateLimiter.Allow(accountID, "place_order") {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	svc := client.NewCreateOrderService().
		Symbol(order.Symbol).
		Side(futures.SideType(order.Side)).
		Type(futures.OrderType(order.Type))

	// Set position side for hedge mode
	if order.PositionSide != "" {
		svc.PositionSide(futures.PositionSideType(order.PositionSide))
	} else {
		// Default to one-way mode
		if order.Side == types.OrderSideBuy {
			svc.PositionSide(futures.PositionSideTypeLong)
		} else {
			svc.PositionSide(futures.PositionSideTypeShort)
		}
	}

	switch order.Type {
	case types.OrderTypeLimit:
		svc.TimeInForce(futures.TimeInForceTypeGTC).
			Price(order.Price.String()).
			Quantity(order.Quantity.String())
	case types.OrderTypeMarket:
		svc.Quantity(order.Quantity.String())
	case types.OrderTypeStop:
		svc.StopPrice(order.StopPrice.String()).
			Quantity(order.Quantity.String())
	case types.OrderTypeTakeProfit:
		svc.StopPrice(order.StopPrice.String()).
			Quantity(order.Quantity.String())
	}

	if order.ClientOrderID != "" {
		svc.NewClientOrderID(order.ClientOrderID)
	}

	if order.ReduceOnly {
		svc.ReduceOnly(true)
	}

	res, err := svc.Do(ctx)
	if err != nil {
		return nil, err
	}

	// Update order with response
	order.ID = strconv.FormatInt(res.OrderID, 10)
	order.Status = string(res.Status)
	order.CreatedAt = time.Unix(res.UpdateTime/1000, (res.UpdateTime%1000)*1000000)
	order.UpdatedAt = order.CreatedAt

	executedQty, _ := decimal.NewFromString(res.ExecutedQuantity)
	order.ExecutedQty = executedQty

	// Update position tracking
	if order.Status == "FILLED" || order.Status == "PARTIALLY_FILLED" {
		fc.updatePositionFromOrder(accountID, order)
	}

	return order, nil
}

// CancelOrder cancels an order
func (fc *FuturesConnector) CancelOrder(ctx context.Context, symbol string, orderID string) error {
	return fc.CancelOrderForAccount(ctx, "main", symbol, orderID)
}

// CancelOrderForAccount cancels an order for a specific account
func (fc *FuturesConnector) CancelOrderForAccount(ctx context.Context, accountID string, symbol string, orderID string) error {
	// Check if we should use WebSocket
	if wsManager, exists := fc.wsManagers[accountID]; exists && wsManager.IsConnected() {
		return wsManager.CancelOrder(ctx, symbol, orderID, "")
	}

	// Fallback to REST API
	client, err := fc.GetClient(accountID)
	if err != nil {
		return err
	}

	if !fc.rateLimiter.Allow(accountID, "cancel_order") {
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
func (fc *FuturesConnector) GetOrder(ctx context.Context, symbol string, orderID string) (*types.Order, error) {
	return fc.GetOrderForAccount(ctx, "main", symbol, orderID)
}

// GetOrderForAccount retrieves order information for a specific account
func (fc *FuturesConnector) GetOrderForAccount(ctx context.Context, accountID string, symbol string, orderID string) (*types.Order, error) {
	client, err := fc.GetClient(accountID)
	if err != nil {
		return nil, err
	}

	if !fc.rateLimiter.Allow(accountID, "get_order") {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	var order *futures.Order
	
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

	return fc.convertBinanceOrder(order, accountID), nil
}

// GetOpenOrders retrieves all open orders
func (fc *FuturesConnector) GetOpenOrders(ctx context.Context, symbol string) ([]*types.Order, error) {
	return fc.GetOpenOrdersForAccount(ctx, "main", symbol)
}

// GetOpenOrdersForAccount retrieves open orders for a specific account
func (fc *FuturesConnector) GetOpenOrdersForAccount(ctx context.Context, accountID string, symbol string) ([]*types.Order, error) {
	client, err := fc.GetClient(accountID)
	if err != nil {
		return nil, err
	}

	if !fc.rateLimiter.Allow(accountID, "open_orders") {
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
		result = append(result, fc.convertBinanceOrder(order, accountID))
	}

	return result, nil
}

// GetOrderHistory retrieves order history
func (fc *FuturesConnector) GetOrderHistory(ctx context.Context, symbol string, limit int) ([]*types.Order, error) {
	return fc.GetOrderHistoryForAccount(ctx, "main", symbol, limit)
}

// GetOrderHistoryForAccount retrieves order history for a specific account
func (fc *FuturesConnector) GetOrderHistoryForAccount(ctx context.Context, accountID string, symbol string, limit int) ([]*types.Order, error) {
	client, err := fc.GetClient(accountID)
	if err != nil {
		return nil, err
	}

	if !fc.rateLimiter.Allow(accountID, "order_history") {
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
		result = append(result, fc.convertBinanceOrder(order, accountID))
	}

	return result, nil
}

// GetTrades retrieves recent trades
func (fc *FuturesConnector) GetTrades(ctx context.Context, symbol string, limit int) ([]*types.Trade, error) {
	return fc.GetTradesForAccount(ctx, "main", symbol, limit)
}

// GetTradesForAccount retrieves trades for a specific account
func (fc *FuturesConnector) GetTradesForAccount(ctx context.Context, accountID string, symbol string, limit int) ([]*types.Trade, error) {
	client, err := fc.GetClient(accountID)
	if err != nil {
		return nil, err
	}

	if !fc.rateLimiter.Allow(accountID, "trades") {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	svc := client.NewListAccountTradeService().Symbol(symbol)
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
		quantity, _ := strconv.ParseFloat(trade.Qty, 64)
		quoteQty, _ := strconv.ParseFloat(trade.QuoteQty, 64)
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
			IsBuyer:         trade.Buyer,
			IsMaker:         trade.Maker,
		})
	}

	return result, nil
}

// Futures-specific methods

// GetPositions returns all positions
func (fc *FuturesConnector) GetPositions(ctx context.Context) ([]*types.Position, error) {
	return fc.GetPositionsForAccount(ctx, "main")
}

// GetPositionsForAccount returns positions for a specific account
func (fc *FuturesConnector) GetPositionsForAccount(ctx context.Context, accountID string) ([]*types.Position, error) {
	client, err := fc.GetClient(accountID)
	if err != nil {
		return nil, err
	}

	if !fc.rateLimiter.Allow(accountID, "positions") {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	risks, err := client.NewGetPositionRiskService().Do(ctx)
	if err != nil {
		return nil, err
	}

	positions := make([]*types.Position, 0)
	for _, risk := range risks {
		posAmt, _ := strconv.ParseFloat(risk.PositionAmt, 64)
		if posAmt == 0 {
			continue
		}

		position := fc.convertPositionRisk(&risk, accountID)
		positions = append(positions, position)
		
		// Update cached position
		fc.updateCachedPosition(accountID, position)
	}

	return positions, nil
}

// GetPosition returns position for a specific symbol
func (fc *FuturesConnector) GetPosition(ctx context.Context, symbol string) (*types.Position, error) {
	return fc.GetPositionForAccount(ctx, "main", symbol)
}

// GetPositionForAccount returns position for a specific symbol and account
func (fc *FuturesConnector) GetPositionForAccount(ctx context.Context, accountID string, symbol string) (*types.Position, error) {
	// Try cache first
	fc.positionMu.RLock()
	if accountPositions, exists := fc.positions[accountID]; exists {
		if position, exists := accountPositions[symbol]; exists {
			fc.positionMu.RUnlock()
			return position, nil
		}
	}
	fc.positionMu.RUnlock()

	// Load from API
	positions, err := fc.GetPositionsForAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}

	for _, pos := range positions {
		if pos.Symbol == symbol {
			return pos, nil
		}
	}

	// Return empty position
	return &types.Position{
		Symbol:    symbol,
		AccountID: accountID,
		Side:      types.PositionSideNone,
		Quantity:  decimal.Zero,
	}, nil
}

// SetLeverage sets the leverage for a symbol
func (fc *FuturesConnector) SetLeverage(ctx context.Context, symbol string, leverage int) error {
	return fc.SetLeverageForAccount(ctx, "main", symbol, leverage)
}

// SetLeverageForAccount sets leverage for a specific account
func (fc *FuturesConnector) SetLeverageForAccount(ctx context.Context, accountID string, symbol string, leverage int) error {
	client, err := fc.GetClient(accountID)
	if err != nil {
		return err
	}

	if !fc.rateLimiter.Allow(accountID, "set_leverage") {
		return fmt.Errorf("rate limit exceeded")
	}

	_, err = client.NewChangeLeverageService().
		Symbol(symbol).
		Leverage(leverage).
		Do(ctx)

	return err
}

// SetMarginMode sets the margin mode
func (fc *FuturesConnector) SetMarginMode(ctx context.Context, symbol string, marginMode types.MarginMode) error {
	return fc.SetMarginModeForAccount(ctx, "main", symbol, marginMode)
}

// SetMarginModeForAccount sets margin mode for a specific account
func (fc *FuturesConnector) SetMarginModeForAccount(ctx context.Context, accountID string, symbol string, marginMode types.MarginMode) error {
	client, err := fc.GetClient(accountID)
	if err != nil {
		return err
	}

	if !fc.rateLimiter.Allow(accountID, "set_margin_mode") {
		return fmt.Errorf("rate limit exceeded")
	}

	var binanceMarginType futures.MarginType
	switch marginMode {
	case types.MarginModeCross:
		binanceMarginType = futures.MarginTypeCrossed
	case types.MarginModeIsolated:
		binanceMarginType = futures.MarginTypeIsolated
	default:
		return fmt.Errorf("invalid margin mode: %s", marginMode)
	}

	err = client.NewChangeMarginTypeService().
		Symbol(symbol).
		MarginType(binanceMarginType).
		Do(ctx)

	return err
}

// GetFundingRate returns the current funding rate
func (fc *FuturesConnector) GetFundingRate(ctx context.Context, symbol string) (*types.FundingRate, error) {
	client, err := fc.GetClient("main")
	if err != nil {
		return nil, err
	}

	if !fc.rateLimiter.Allow("main", "funding_rate") {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	rates, err := client.NewPremiumIndexService().Symbol(symbol).Do(ctx)
	if err != nil {
		return nil, err
	}

	if len(rates) == 0 {
		return nil, fmt.Errorf("no funding rate found for %s", symbol)
	}

	rate := rates[0]
	fundingRateValue, _ := strconv.ParseFloat(rate.LastFundingRate, 64)
	nextFundingTime := time.Unix(rate.NextFundingTime/1000, 0)

	return &types.FundingRate{
		Symbol:          symbol,
		Rate:            decimal.NewFromFloat(fundingRateValue),
		NextFundingTime: nextFundingTime,
		Timestamp:       time.Unix(rate.Time/1000, 0),
	}, nil
}

// Market data methods (shared with spot interface)
func (fc *FuturesConnector) GetSymbolInfo(ctx context.Context, symbol string) (*types.SymbolInfo, error) {
	// Use cache first
	cacheKey := fmt.Sprintf("futures_symbol_info_%s", symbol)
	if cached, exists := fc.cache.Get(cacheKey); exists {
		return cached.(*types.SymbolInfo), nil
	}

	client, err := fc.GetClient("main")
	if err != nil {
		return nil, err
	}

	if !fc.rateLimiter.Allow("main", "exchange_info") {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	info, err := client.NewExchangeInfoService().Do(ctx)
	if err != nil {
		return nil, err
	}

	for _, s := range info.Symbols {
		if s.Symbol == symbol {
			symbolInfo := fc.convertBinanceSymbol(&s)
			
			// Cache for 1 hour
			fc.cache.Set(cacheKey, symbolInfo, time.Hour)
			
			return symbolInfo, nil
		}
	}

	return nil, fmt.Errorf("symbol %s not found", symbol)
}

func (fc *FuturesConnector) GetMarketData(ctx context.Context, symbols []string) (map[string]*types.MarketData, error) {
	client, err := fc.GetClient("main")
	if err != nil {
		return nil, err
	}

	if !fc.rateLimiter.Allow("main", "market_data") {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	// Get 24hr ticker for all symbols
	tickers, err := client.NewListPriceChangeStatsService().Do(ctx)
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
			volume, _ := strconv.ParseFloat(ticker.Volume, 64)
			quoteVolume, _ := strconv.ParseFloat(ticker.QuoteVolume, 64)
			high, _ := strconv.ParseFloat(ticker.HighPrice, 64)
			low, _ := strconv.ParseFloat(ticker.LowPrice, 64)
			openPrice, _ := strconv.ParseFloat(ticker.OpenPrice, 64)

			result[ticker.Symbol] = &types.MarketData{
				Symbol:      ticker.Symbol,
				LastPrice:   decimal.NewFromFloat(lastPrice),
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

func (fc *FuturesConnector) GetOrderBook(ctx context.Context, symbol string, depth int) (*types.OrderBook, error) {
	client, err := fc.GetClient("main")
	if err != nil {
		return nil, err
	}

	if !fc.rateLimiter.Allow("main", "order_book") {
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
		orderBook.Bids = append(orderBook.Bids, types.OrderBookEntry{
			Price:    bid.Price,
			Quantity: bid.Quantity,
		})
	}

	for _, ask := range book.Asks {
		orderBook.Asks = append(orderBook.Asks, types.OrderBookEntry{
			Price:    ask.Price,
			Quantity: ask.Quantity,
		})
	}

	return orderBook, nil
}

func (fc *FuturesConnector) GetKlines(ctx context.Context, symbol string, interval types.KlineInterval, limit int) ([]*types.Kline, error) {
	client, err := fc.GetClient("main")
	if err != nil {
		return nil, err
	}

	if !fc.rateLimiter.Allow("main", "klines") {
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

// WebSocket operations - TODO: implement
func (fc *FuturesConnector) SubscribeOrderBook(symbol string, callback types.OrderBookCallback) error {
	fc.callbackMu.Lock()
	fc.orderBookCallbacks[symbol] = callback
	fc.callbackMu.Unlock()
	
	// TODO: Implement WebSocket subscription
	return fmt.Errorf("WebSocket not implemented yet")
}

func (fc *FuturesConnector) SubscribeTrades(symbol string, callback types.TradeCallback) error {
	fc.callbackMu.Lock()
	fc.tradeCallbacks[symbol] = callback
	fc.callbackMu.Unlock()
	
	// TODO: Implement WebSocket subscription
	return fmt.Errorf("WebSocket not implemented yet")
}

func (fc *FuturesConnector) SubscribeTicker(symbol string, callback types.TickerCallback) error {
	fc.callbackMu.Lock()
	fc.tickerCallbacks[symbol] = callback
	fc.callbackMu.Unlock()
	
	// TODO: Implement WebSocket subscription
	return fmt.Errorf("WebSocket not implemented yet")
}

func (fc *FuturesConnector) UnsubscribeAll() error {
	fc.callbackMu.Lock()
	fc.orderBookCallbacks = make(map[string]types.OrderBookCallback)
	fc.tradeCallbacks = make(map[string]types.TradeCallback)
	fc.tickerCallbacks = make(map[string]types.TickerCallback)
	fc.callbackMu.Unlock()
	
	return nil
}

// Helper methods
func (fc *FuturesConnector) convertBinanceOrder(order *futures.Order, accountID string) *types.Order {
	price, _ := decimal.NewFromString(order.Price)
	quantity, _ := decimal.NewFromString(order.OrigQuantity)
	executedQty, _ := decimal.NewFromString(order.ExecutedQuantity)
	stopPrice, _ := decimal.NewFromString(order.StopPrice)
	
	// Calculate executed quote quantity
	cummulativeQuoteQty, _ := decimal.NewFromString(order.CumQuote)

	return &types.Order{
		ID:               strconv.FormatInt(order.OrderID, 10),
		ClientOrderID:    order.ClientOrderID,
		AccountID:        accountID,
		Symbol:           order.Symbol,
		Side:             string(order.Side),
		Type:             string(order.Type),
		Status:           string(order.Status),
		Price:            price,
		StopPrice:        stopPrice,
		Quantity:         quantity,
		ExecutedQty:      executedQty,
		ExecutedQuoteQty: cummulativeQuoteQty,
		TimeInForce:      string(order.TimeInForce),
		PositionSide:     string(order.PositionSide),
		ReduceOnly:       order.ReduceOnly,
		CreatedAt:        time.Unix(order.Time/1000, (order.Time%1000)*1000000),
		UpdatedAt:        time.Unix(order.UpdateTime/1000, (order.UpdateTime%1000)*1000000),
	}
}

func (fc *FuturesConnector) convertBinanceSymbol(symbol *futures.Symbol) *types.SymbolInfo {
	// Find filters
	var minQty, maxQty, stepSize decimal.Decimal
	var minNotional decimal.Decimal
	var maxLeverage int = 125 // Default

	for _, filter := range symbol.Filters {
		switch filter["filterType"] {
		case "LOT_SIZE":
			minQty, _ = decimal.NewFromString(fmt.Sprintf("%v", filter["minQty"]))
			maxQty, _ = decimal.NewFromString(fmt.Sprintf("%v", filter["maxQty"]))
			stepSize, _ = decimal.NewFromString(fmt.Sprintf("%v", filter["stepSize"]))
		case "MIN_NOTIONAL":
			minNotional, _ = decimal.NewFromString(fmt.Sprintf("%v", filter["notional"]))
		}
	}

	// Get max leverage from symbol info if available
	if leverageVal, ok := symbol.LeverageBrackets[0]["initialLeverage"].(float64); ok {
		maxLeverage = int(leverageVal)
	}

	return &types.SymbolInfo{
		Symbol:               symbol.Symbol,
		BaseAsset:            symbol.BaseAsset,
		QuoteAsset:           symbol.QuoteAsset,
		Status:               symbol.Status,
		MinQty:               minQty,
		MaxQty:               maxQty,
		StepSize:             stepSize,
		MinNotional:          minNotional,
		IsFuturesTradingAllowed: symbol.Status == "TRADING",
		ContractType:         symbol.ContractType,
		MaxLeverage:          maxLeverage,
	}
}

func (fc *FuturesConnector) convertPositionRisk(risk *futures.PositionRisk, accountID string) *types.Position {
	quantity, _ := strconv.ParseFloat(risk.PositionAmt, 64)
	entryPrice, _ := strconv.ParseFloat(risk.EntryPrice, 64)
	markPrice, _ := strconv.ParseFloat(risk.MarkPrice, 64)
	unrealizedPnL, _ := strconv.ParseFloat(risk.UnRealizedProfit, 64)
	leverage, _ := strconv.ParseFloat(risk.Leverage, 64)
	liquidationPrice, _ := strconv.ParseFloat(risk.LiquidationPrice, 64)
	notional, _ := strconv.ParseFloat(risk.Notional, 64)
	margin, _ := strconv.ParseFloat(risk.IsolatedMargin, 64)

	side := types.PositionSideNone
	if quantity > 0 {
		side = types.PositionSideLong
	} else if quantity < 0 {
		side = types.PositionSideShort
		quantity = -quantity // Make positive
	}

	return &types.Position{
		Symbol:           risk.Symbol,
		AccountID:        accountID,
		Side:             side,
		PositionSide:     risk.PositionSide,
		Quantity:         decimal.NewFromFloat(quantity),
		EntryPrice:       decimal.NewFromFloat(entryPrice),
		MarkPrice:        decimal.NewFromFloat(markPrice),
		UnrealizedPnL:    decimal.NewFromFloat(unrealizedPnL),
		RealizedPnL:      decimal.Zero, // Not available in position risk
		Margin:           decimal.NewFromFloat(margin),
		MarginType:       risk.MarginType,
		Leverage:         int(leverage),
		LiquidationPrice: decimal.NewFromFloat(liquidationPrice),
		Notional:         decimal.NewFromFloat(notional),
		UpdateTime:       time.Now(),
	}
}

func (fc *FuturesConnector) loadPositions(ctx context.Context, accountID string) error {
	positions, err := fc.GetPositionsForAccount(ctx, accountID)
	if err != nil {
		return err
	}

	fc.positionMu.Lock()
	defer fc.positionMu.Unlock()

	if fc.positions[accountID] == nil {
		fc.positions[accountID] = make(map[string]*types.Position)
	}

	for _, pos := range positions {
		fc.positions[accountID][pos.Symbol] = pos
	}

	return nil
}

func (fc *FuturesConnector) updateCachedPosition(accountID string, position *types.Position) {
	fc.positionMu.Lock()
	defer fc.positionMu.Unlock()

	if fc.positions[accountID] == nil {
		fc.positions[accountID] = make(map[string]*types.Position)
	}

	fc.positions[accountID][position.Symbol] = position
}

func (fc *FuturesConnector) updatePositionFromOrder(accountID string, order *types.Order) {
	fc.positionMu.Lock()
	defer fc.positionMu.Unlock()

	if fc.positions[accountID] == nil {
		fc.positions[accountID] = make(map[string]*types.Position)
	}

	position, exists := fc.positions[accountID][order.Symbol]
	if !exists {
		// Create new position
		position = &types.Position{
			Symbol:    order.Symbol,
			AccountID: accountID,
			Side:      types.PositionSideNone,
			Quantity:  decimal.Zero,
		}
		fc.positions[accountID][order.Symbol] = position
	}

	// Update position based on order
	// This is a simplified version - real implementation would need more logic
	if order.Side == types.OrderSideBuy {
		position.Quantity = position.Quantity.Add(order.ExecutedQty)
	} else {
		position.Quantity = position.Quantity.Sub(order.ExecutedQty)
	}

	// Determine position side
	if position.Quantity.IsPositive() {
		position.Side = types.PositionSideLong
	} else if position.Quantity.IsNegative() {
		position.Side = types.PositionSideShort
		position.Quantity = position.Quantity.Abs()
	} else {
		position.Side = types.PositionSideNone
	}

	position.UpdateTime = time.Now()
}