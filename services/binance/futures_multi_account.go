package binance

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	binance "github.com/adshao/go-binance/v2"
	futures "github.com/adshao/go-binance/v2/futures"
	"github.com/mExOms/pkg/types"
	"github.com/mExOms/pkg/vault"
	"github.com/shopspring/decimal"
)

// BinanceFuturesMultiAccount implements multi-account support for Binance Futures
type BinanceFuturesMultiAccount struct {
	mu sync.RWMutex
	
	// Multi-account support
	clients         map[string]*futures.Client
	currentAccount  string
	accountManager  types.AccountManager
	
	// Connection state
	connected       bool
	testnet         bool
	
	// WebSocket managers per account
	wsManagers      map[string]*FuturesWebSocketManager
	
	// Rate limiting per account
	rateLimiters    map[string]*RateLimiter
	
	// Position tracking
	positions       map[string]map[string]*types.Position // accountID -> symbol -> position
	
	// Vault client for API key management
	vaultClient     *vault.Client
}

// FuturesWebSocketManager manages WebSocket connections for futures
type FuturesWebSocketManager struct {
	orderBookStreams map[string]*WebSocketStream
	tradeStreams     map[string]*WebSocketStream
	userDataStream   *WebSocketStream
	markPriceStream  *WebSocketStream
}

// WebSocketStream holds WebSocket stream control channels
type WebSocketStream struct {
	Done chan struct{}
	Stop chan struct{}
}

// NewBinanceFuturesMultiAccount creates a new multi-account Binance Futures connector
func NewBinanceFuturesMultiAccount(accountManager types.AccountManager, testnet bool) (*BinanceFuturesMultiAccount, error) {
	// Create Vault client
	vaultClient, err := vault.NewClient(vault.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %v", err)
	}
	
	return &BinanceFuturesMultiAccount{
		clients:        make(map[string]*futures.Client),
		accountManager: accountManager,
		testnet:        testnet,
		wsManagers:     make(map[string]*FuturesWebSocketManager),
		rateLimiters:   make(map[string]*RateLimiter),
		positions:      make(map[string]map[string]*types.Position),
		vaultClient:    vaultClient,
	}, nil
}

// Connect establishes connections for all configured futures accounts
func (b *BinanceFuturesMultiAccount) Connect(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	// Get all Binance futures accounts
	filter := types.AccountFilter{
		Exchange: "binance",
		Active:   &[]bool{true}[0],
		Market:   types.MarketTypeFutures,
	}
	
	accounts, err := b.accountManager.ListAccounts(filter)
	if err != nil {
		return fmt.Errorf("failed to list accounts: %w", err)
	}
	
	if len(accounts) == 0 {
		return fmt.Errorf("no active Binance futures accounts found")
	}
	
	// Connect each account
	for _, account := range accounts {
		if account.FuturesEnabled {
			if err := b.connectAccount(ctx, account); err != nil {
				return fmt.Errorf("failed to connect account %s: %w", account.ID, err)
			}
		}
	}
	
	// Set default account (main or first)
	for _, account := range accounts {
		if account.Type == types.AccountTypeMain {
			b.currentAccount = account.ID
			break
		}
	}
	if b.currentAccount == "" && len(accounts) > 0 {
		b.currentAccount = accounts[0].ID
	}
	
	b.connected = true
	return nil
}

// connectAccount connects a single futures account
func (b *BinanceFuturesMultiAccount) connectAccount(ctx context.Context, account *types.Account) error {
	// Get API credentials from vault/config
	apiKey, apiSecret, err := b.getAccountCredentials(account)
	if err != nil {
		return err
	}
	
	// Create futures client
	var client *futures.Client
	if b.testnet {
		futures.UseTestnet = true
		client = futures.NewClient(apiKey, apiSecret)
	} else {
		client = futures.NewClient(apiKey, apiSecret)
	}
	
	// Test connection
	err = client.NewPingService().Do(ctx)
	if err != nil {
		return fmt.Errorf("failed to ping: %w", err)
	}
	
	// Get account info to verify futures access
	accountInfo, err := client.NewGetAccountService().Do(ctx)
	if err != nil {
		return fmt.Errorf("failed to get account info: %w", err)
	}
	
	// Store client
	b.clients[account.ID] = client
	
	// Initialize rate limiter
	b.rateLimiters[account.ID] = &RateLimiter{
		windowStart: time.Now(),
	}
	
	// Initialize WebSocket manager
	b.wsManagers[account.ID] = &FuturesWebSocketManager{
		orderBookStreams: make(map[string]*WebSocketStream),
		tradeStreams:     make(map[string]*WebSocketStream),
	}
	
	// Initialize position tracking
	b.positions[account.ID] = make(map[string]*types.Position)
	
	// Load initial positions
	b.loadAccountPositions(ctx, account.ID, accountInfo)
	
	return nil
}

// Disconnect closes all connections
func (b *BinanceFuturesMultiAccount) Disconnect() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	// Close all WebSocket connections
	for accountID, wsManager := range b.wsManagers {
		// Close order book streams
		for _, stream := range wsManager.orderBookStreams {
			if stream != nil && stream.Stop != nil {
				close(stream.Stop)
			}
		}
		
		// Close trade streams
		for _, stream := range wsManager.tradeStreams {
			if stream != nil && stream.Stop != nil {
				close(stream.Stop)
			}
		}
		
		// Close user data stream
		if wsManager.userDataStream != nil && wsManager.userDataStream.Stop != nil {
			close(wsManager.userDataStream.Stop)
		}
		
		// Close mark price stream
		if wsManager.markPriceStream != nil && wsManager.markPriceStream.Stop != nil {
			close(wsManager.markPriceStream.Stop)
		}
		
		delete(b.wsManagers, accountID)
	}
	
	b.connected = false
	return nil
}

// IsConnected returns connection status
func (b *BinanceFuturesMultiAccount) IsConnected() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.connected
}

// GetName returns exchange name
func (b *BinanceFuturesMultiAccount) GetName() string {
	return "binance"
}

// GetMarket returns market type
func (b *BinanceFuturesMultiAccount) GetMarket() types.MarketType {
	return types.MarketTypeFutures
}

// SetAccount sets the current account for operations
func (b *BinanceFuturesMultiAccount) SetAccount(accountID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	if _, exists := b.clients[accountID]; !exists {
		return fmt.Errorf("account %s not connected", accountID)
	}
	
	b.currentAccount = accountID
	return nil
}

// GetCurrentAccount returns the current account ID
func (b *BinanceFuturesMultiAccount) GetCurrentAccount() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.currentAccount
}

// SupportSubAccounts returns true as Binance supports sub-accounts
func (b *BinanceFuturesMultiAccount) SupportSubAccounts() bool {
	return true
}

// CreateOrder creates a futures order using the current account
func (b *BinanceFuturesMultiAccount) CreateOrder(ctx context.Context, order *types.Order) (*types.Order, error) {
	b.mu.RLock()
	client, exists := b.clients[b.currentAccount]
	accountID := b.currentAccount
	b.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("no client for current account")
	}
	
	// Check rate limit
	if err := b.checkRateLimit(accountID, 1); err != nil {
		return nil, err
	}
	
	// Create order service
	service := client.NewCreateOrderService().
		Symbol(order.Symbol).
		Side(futures.SideType(order.Side)).
		Type(futures.OrderType(order.Type))
	
	// Set quantity
	service.Quantity(order.Quantity.String())
	
	// Set price for limit orders
	if order.Type == types.OrderTypeLimit {
		service.Price(order.Price.String())
		service.TimeInForce(futures.TimeInForceType(order.TimeInForce))
	}
	
	// Set position side for hedge mode
	if order.PositionSide != "" {
		service.PositionSide(futures.PositionSideType(order.PositionSide))
	}
	
	// Set reduce only if specified
	if order.ReduceOnly {
		service.ReduceOnly(true)
	}
	
	// Execute order
	response, err := service.Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create order: %w", err)
	}
	
	// Update rate limit
	b.updateRateLimit(accountID, 1)
	
	// Convert response
	order.ExchangeOrderID = fmt.Sprintf("%d", response.OrderID)
	order.Status = string(response.Status)
	order.CreatedAt = time.UnixMilli(response.UpdateTime)
	
	// Record order with account info
	order.Metadata = map[string]interface{}{
		"account_id": accountID,
		"exchange":   "binance",
		"market":     "futures",
	}
	
	return order, nil
}

// CancelOrder cancels a futures order
func (b *BinanceFuturesMultiAccount) CancelOrder(ctx context.Context, orderID string) error {
	b.mu.RLock()
	client, exists := b.clients[b.currentAccount]
	accountID := b.currentAccount
	b.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("no client for current account")
	}
	
	// Check rate limit
	if err := b.checkRateLimit(accountID, 1); err != nil {
		return err
	}
	
	// Parse order info (assuming format: symbol:orderID)
	// In production, maintain order mapping
	symbol := "BTCUSDT" // TODO: Get from order tracking
	
	// Convert orderID string to int64
	orderIDInt, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid order ID format: %w", err)
	}
	
	_, err = client.NewCancelOrderService().
		Symbol(symbol).
		OrderID(orderIDInt).
		Do(ctx)
	
	if err != nil {
		return fmt.Errorf("failed to cancel order: %w", err)
	}
	
	// Update rate limit
	b.updateRateLimit(accountID, 1)
	
	return nil
}

// GetOpenOrders retrieves open orders for the current account
func (b *BinanceFuturesMultiAccount) GetOpenOrders(ctx context.Context, symbol string) ([]*types.Order, error) {
	b.mu.RLock()
	client, exists := b.clients[b.currentAccount]
	accountID := b.currentAccount
	b.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("no client for current account")
	}
	
	// Check rate limit
	weight := 1
	if symbol == "" {
		weight = 40 // All symbols
	}
	
	if err := b.checkRateLimit(accountID, weight); err != nil {
		return nil, err
	}
	
	// Get open orders
	service := client.NewListOpenOrdersService()
	if symbol != "" {
		service.Symbol(symbol)
	}
	
	binanceOrders, err := service.Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get open orders: %w", err)
	}
	
	// Update rate limit
	b.updateRateLimit(accountID, weight)
	
	// Convert orders
	orders := make([]*types.Order, 0, len(binanceOrders))
	for _, bo := range binanceOrders {
		orders = append(orders, b.convertFuturesOrder(bo, accountID))
	}
	
	return orders, nil
}

// GetPositions retrieves all positions for the current account
func (b *BinanceFuturesMultiAccount) GetPositions(ctx context.Context) ([]*types.Position, error) {
	b.mu.RLock()
	client, exists := b.clients[b.currentAccount]
	accountID := b.currentAccount
	b.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("no client for current account")
	}
	
	// Check rate limit
	if err := b.checkRateLimit(accountID, 5); err != nil {
		return nil, err
	}
	
	// Get account info
	accountInfo, err := client.NewGetAccountService().Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}
	
	// Update rate limit
	b.updateRateLimit(accountID, 5)
	
	// Convert positions
	positions := make([]*types.Position, 0)
	for _, pos := range accountInfo.Positions {
		positionAmt, _ := decimal.NewFromString(pos.PositionAmt)
		if positionAmt.IsZero() {
			continue
		}
		
		position := b.convertPosition(pos, accountID)
		positions = append(positions, position)
		
		// Update internal tracking
		b.mu.Lock()
		b.positions[accountID][pos.Symbol] = position
		b.mu.Unlock()
	}
	
	return positions, nil
}

// GetBalance retrieves balance for the current account
func (b *BinanceFuturesMultiAccount) GetBalance(ctx context.Context, asset string) (*types.Balance, error) {
	b.mu.RLock()
	accountID := b.currentAccount
	b.mu.RUnlock()
	
	return b.GetBalanceForAccount(ctx, accountID, asset)
}

// GetBalanceForAccount retrieves balance for a specific account
func (b *BinanceFuturesMultiAccount) GetBalanceForAccount(ctx context.Context, accountID, asset string) (*types.Balance, error) {
	b.mu.RLock()
	client, exists := b.clients[accountID]
	b.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("account %s not connected", accountID)
	}
	
	// Check rate limit
	if err := b.checkRateLimit(accountID, 5); err != nil {
		return nil, err
	}
	
	// Get account info
	account, err := client.NewGetAccountService().Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}
	
	// Update rate limit
	b.updateRateLimit(accountID, 5)
	
	// Build balance
	balance := &types.Balance{
		Exchange: "binance",
		Market:   "futures",
		Assets:   make(map[string]types.AssetBalance),
	}
	
	// Find specific asset or add all assets
	totalUSDT := decimal.Zero
	
	for _, bal := range account.Assets {
		free, _ := decimal.NewFromString(bal.AvailableBalance)
		locked, _ := decimal.NewFromString(bal.InitialMargin)
		
		// Skip zero balances unless specifically requested
		if asset == "" && free.IsZero() && locked.IsZero() {
			continue
		}
		
		// Add to balance map
		if asset == "" || bal.Asset == asset {
			balance.Assets[bal.Asset] = types.AssetBalance{
				Asset:  bal.Asset,
				Free:   bal.AvailableBalance,
				Locked: bal.InitialMargin,
			}
			
			if asset != "" {
				break // Found the requested asset
			}
		}
		
		// Calculate USDT value
		if bal.Asset == "USDT" {
			walletBalance, _ := decimal.NewFromString(bal.WalletBalance)
			totalUSDT = totalUSDT.Add(walletBalance)
		}
	}
	
	// Update account balance in manager
	accountBalance := &types.AccountBalance{
		AccountID: accountID,
		Exchange:  "binance",
		TotalUSDT: totalUSDT,
		UpdatedAt: time.Now(),
	}
	
	b.accountManager.UpdateBalance(accountID, accountBalance)
	
	return balance, nil
}

// SetLeverage sets leverage for a symbol on the current account
func (b *BinanceFuturesMultiAccount) SetLeverage(ctx context.Context, symbol string, leverage int) error {
	b.mu.RLock()
	client, exists := b.clients[b.currentAccount]
	accountID := b.currentAccount
	b.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("no client for current account")
	}
	
	// Check rate limit
	if err := b.checkRateLimit(accountID, 1); err != nil {
		return err
	}
	
	// Check account leverage limit
	account, err := b.accountManager.GetAccount(accountID)
	if err != nil {
		return err
	}
	
	if leverage > account.MaxLeverage {
		return fmt.Errorf("leverage %d exceeds account limit %d", leverage, account.MaxLeverage)
	}
	
	// Set leverage
	_, err = client.NewChangeLeverageService().
		Symbol(symbol).
		Leverage(leverage).
		Do(ctx)
	
	if err != nil {
		return fmt.Errorf("failed to set leverage: %w", err)
	}
	
	// Update rate limit
	b.updateRateLimit(accountID, 1)
	
	return nil
}

// SetMarginType sets margin type for a symbol on the current account
func (b *BinanceFuturesMultiAccount) SetMarginType(ctx context.Context, symbol string, marginType string) error {
	b.mu.RLock()
	client, exists := b.clients[b.currentAccount]
	accountID := b.currentAccount
	b.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("no client for current account")
	}
	
	// Check rate limit
	if err := b.checkRateLimit(accountID, 1); err != nil {
		return err
	}
	
	// Set margin type
	err := client.NewChangeMarginTypeService().
		Symbol(symbol).
		MarginType(futures.MarginType(marginType)).
		Do(ctx)
	
	if err != nil {
		return fmt.Errorf("failed to set margin type: %w", err)
	}
	
	// Update rate limit
	b.updateRateLimit(accountID, 1)
	
	return nil
}

// Helper methods

// getAccountCredentials retrieves API credentials for an account
func (b *BinanceFuturesMultiAccount) getAccountCredentials(account *types.Account) (apiKey, apiSecret string, err error) {
	// Retrieve from Vault
	keys, err := b.vaultClient.GetExchangeKeys("binance", "futures")
	if err != nil {
		return "", "", fmt.Errorf("failed to get API keys from Vault: %v", err)
	}
	
	apiKey, ok := keys["api_key"]
	if !ok {
		return "", "", fmt.Errorf("api_key not found in Vault")
	}
	
	apiSecret, ok = keys["secret_key"]
	if !ok {
		return "", "", fmt.Errorf("secret_key not found in Vault")
	}
	
	return apiKey, apiSecret, nil
}

// checkRateLimit checks if request can proceed
func (b *BinanceFuturesMultiAccount) checkRateLimit(accountID string, weight int) error {
	limiter, exists := b.rateLimiters[accountID]
	if !exists {
		return fmt.Errorf("no rate limiter for account %s", accountID)
	}
	
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	
	// Reset window if expired
	if time.Since(limiter.windowStart) > time.Minute {
		limiter.weight = 0
		limiter.orders = 0
		limiter.windowStart = time.Now()
	}
	
	// Check limits (Binance Futures: 2400 weight/min)
	if limiter.weight+weight > 2400 {
		return fmt.Errorf("rate limit exceeded for account %s", accountID)
	}
	
	return nil
}

// updateRateLimit updates rate limit usage
func (b *BinanceFuturesMultiAccount) updateRateLimit(accountID string, weight int) {
	limiter, exists := b.rateLimiters[accountID]
	if !exists {
		return
	}
	
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	
	limiter.weight += weight
	
	// Update account metrics
	b.accountManager.UpdateAccountMetrics(accountID, types.AccountMetrics{
		UsedWeight:      limiter.weight,
		RemainingWeight: 2400 - limiter.weight,
		UpdatedAt:       time.Now(),
	})
}

// loadAccountPositions loads initial positions for an account
func (b *BinanceFuturesMultiAccount) loadAccountPositions(ctx context.Context, accountID string, accountInfo *futures.Account) {
	for _, pos := range accountInfo.Positions {
		positionAmt, _ := decimal.NewFromString(pos.PositionAmt)
		if positionAmt.IsZero() {
			continue
		}
		
		position := b.convertPosition(pos, accountID)
		b.positions[accountID][pos.Symbol] = position
	}
}

// convertFuturesOrder converts Binance futures order to internal format
func (b *BinanceFuturesMultiAccount) convertFuturesOrder(bo *futures.Order, accountID string) *types.Order {
	price, _ := decimal.NewFromString(bo.Price)
	quantity, _ := decimal.NewFromString(bo.OrigQuantity)
	
	return &types.Order{
		ClientOrderID:   bo.ClientOrderID,
		ExchangeOrderID: fmt.Sprintf("%d", bo.OrderID),
		Symbol:          bo.Symbol,
		Side:            types.OrderSide(bo.Side),
		Type:            types.OrderType(bo.Type),
		Status:          string(bo.Status),
		Price:           price,
		Quantity:        quantity,
		TimeInForce:     types.TimeInForce(bo.TimeInForce),
		PositionSide:    string(bo.PositionSide),
		ReduceOnly:      bo.ReduceOnly,
		CreatedAt:       time.UnixMilli(bo.Time),
		UpdatedAt:       time.UnixMilli(bo.UpdateTime),
		Metadata: map[string]interface{}{
			"account_id": accountID,
			"exchange":   "binance",
			"market":     "futures",
		},
	}
}

// convertPosition converts Binance position to internal format
func (b *BinanceFuturesMultiAccount) convertPosition(pos *futures.AccountPosition, accountID string) *types.Position {
	positionAmt, _ := decimal.NewFromString(pos.PositionAmt)
	entryPrice, _ := decimal.NewFromString(pos.EntryPrice)
	// futures.AccountPosition doesn't have MarkPrice field - use 0 for now
	markPrice := decimal.Zero
	unRealizedProfit, _ := decimal.NewFromString(pos.UnrealizedProfit) // Fixed field name
	
	var side types.Side
	if positionAmt.LessThan(decimal.Zero) {
		side = types.Side("SHORT")
		positionAmt = positionAmt.Abs()
	} else {
		side = types.Side("LONG")
	}
	
	leverage, _ := decimal.NewFromString(pos.Leverage)
	
	return &types.Position{
		Exchange:      "binance",
		Symbol:        pos.Symbol,
		Side:          side,
		Quantity:      positionAmt.InexactFloat64(),
		EntryPrice:    entryPrice.InexactFloat64(),
		MarkPrice:     markPrice.InexactFloat64(),
		UnrealizedPNL: unRealizedProfit.InexactFloat64(),
		Leverage:      leverage.InexactFloat64(),
		UpdatedAt:     time.Now(),
	}
}

// SubscribeOrderBook subscribes to order book updates
func (b *BinanceFuturesMultiAccount) SubscribeOrderBook(symbol string, callback types.OrderBookCallback) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	// Subscribe for current account
	wsManager, exists := b.wsManagers[b.currentAccount]
	if !exists {
		return fmt.Errorf("no WebSocket manager for current account")
	}
	
	// Create WebSocket handler
	wsHandler := func(event *futures.WsDepthEvent) {
		orderBook := b.convertFuturesOrderBook(event)
		callback(orderBook)
	}
	
	// Start WebSocket
	done, stop, err := futures.WsPartialDepthServe(symbol, "20", wsHandler, nil)
	if err != nil {
		return fmt.Errorf("failed to start orderbook stream: %w", err)
	}
	
	// Store handler
	wsManager.orderBookStreams[symbol] = &WebSocketStream{
		Done: done,
		Stop: stop,
	}
	
	return nil
}

// SubscribeUserData subscribes to user data updates for all accounts
func (b *BinanceFuturesMultiAccount) SubscribeUserData() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	for accountID, client := range b.clients {
		// Get listen key
		listenKey, err := client.NewStartUserStreamService().Do(context.Background())
		if err != nil {
			return fmt.Errorf("failed to get listen key for account %s: %w", accountID, err)
		}
		
		// Create handler
		wsHandler := b.createUserDataHandler(accountID)
		
		// Start WebSocket
		done, stop, err := futures.WsUserDataServe(listenKey, wsHandler, nil)
		if err != nil {
			return fmt.Errorf("failed to start user data stream for account %s: %w", accountID, err)
		}
		
		wsManager := b.wsManagers[accountID]
		wsManager.userDataStream = &WebSocketStream{
			Done: done,
			Stop: stop,
		}
		
		// Keep listen key alive
		go b.keepAliveListenKey(accountID, listenKey)
	}
	
	return nil
}

// createUserDataHandler creates user data handler for an account
func (b *BinanceFuturesMultiAccount) createUserDataHandler(accountID string) futures.WsUserDataHandler {
	return func(event *futures.WsUserDataEvent) {
		switch event.Event {
		case futures.UserDataEventTypeOrderTradeUpdate:
			// Handle order update
			b.handleOrderUpdate(accountID, event.OrderTradeUpdate)
			
		case futures.UserDataEventTypeAccountUpdate:
			// Handle account update
			b.handleAccountUpdate(accountID, event.AccountUpdate)
			
		case futures.UserDataEventTypeMarginCall:
			// Handle margin call
			b.handleMarginCall(accountID, event)
		}
	}
}

// keepAliveListenKey keeps the listen key alive
func (b *BinanceFuturesMultiAccount) keepAliveListenKey(accountID, listenKey string) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		b.mu.RLock()
		client, exists := b.clients[accountID]
		b.mu.RUnlock()
		
		if !exists {
			return
		}
		
		err := client.NewKeepaliveUserStreamService().ListenKey(listenKey).Do(context.Background())
		if err != nil {
			// Log error and try to reconnect
			fmt.Printf("Failed to keepalive listen key for account %s: %v\n", accountID, err)
		}
	}
}

// handleOrderUpdate handles order update events
func (b *BinanceFuturesMultiAccount) handleOrderUpdate(accountID string, update *futures.WsOrderTradeUpdate) {
	// Update internal order tracking
	// Send notification to order manager
}

// handleAccountUpdate handles account update events
func (b *BinanceFuturesMultiAccount) handleAccountUpdate(accountID string, update *futures.WsAccountUpdate) {
	// Update positions
	for _, pos := range update.Positions {
		positionAmt, _ := decimal.NewFromString(pos.Amount)
		
		b.mu.Lock()
		if positionAmt.IsZero() {
			delete(b.positions[accountID], pos.Symbol)
		} else {
			b.positions[accountID][pos.Symbol] = &types.Position{
				Symbol:        pos.Symbol,
				Quantity:      positionAmt.Abs(),
				Side:          b.getPositionSide(positionAmt),
				EntryPrice:    decimal.RequireFromString(pos.EntryPrice).InexactFloat64(),
				UnrealizedPNL: decimal.RequireFromString(pos.UnrealizedPnL).InexactFloat64(),
				UpdatedAt:     time.Now(),
			}
		}
		b.mu.Unlock()
	}
}

// handleMarginCall handles margin call events
func (b *BinanceFuturesMultiAccount) handleMarginCall(accountID string, event *futures.WsUserDataEvent) {
	// Send urgent alert
	fmt.Printf("MARGIN CALL for account %s\n", accountID)
	// Trigger risk management actions
}

// getPositionSide determines position side from quantity
func (b *BinanceFuturesMultiAccount) getPositionSide(quantity decimal.Decimal) types.Side {
	if quantity.GreaterThanOrEqual(decimal.Zero) {
		return types.Side("LONG")
	}
	return types.Side("SHORT")
}

// convertFuturesOrderBook converts futures order book to internal format
func (b *BinanceFuturesMultiAccount) convertFuturesOrderBook(event *futures.WsDepthEvent) *types.OrderBook {
	bids := make([]types.PriceLevel, 0, len(event.Bids))
	for _, bid := range event.Bids {
		price, _ := decimal.NewFromString(bid.Price)
		quantity, _ := decimal.NewFromString(bid.Quantity)
		bids = append(bids, types.PriceLevel{
			Price:    price,
			Quantity: quantity,
		})
	}
	
	asks := make([]types.PriceLevel, 0, len(event.Asks))
	for _, ask := range event.Asks {
		price, _ := decimal.NewFromString(ask.Price)
		quantity, _ := decimal.NewFromString(ask.Quantity)
		asks = append(asks, types.PriceLevel{
			Price:    price,
			Quantity: quantity,
		})
	}
	
	return &types.OrderBook{
		Symbol:       event.Symbol,
		Bids:         bids,
		Asks:         asks,
		LastUpdateID: event.LastUpdateID,
		UpdatedAt:    time.Now(),
	}
}