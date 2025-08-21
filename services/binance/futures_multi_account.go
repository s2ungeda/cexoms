package binance

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

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
	
	// Position update callbacks
	onPositionUpdate func(accountID string, position *types.Position)
}

// FuturesWebSocketManager manages WebSocket connections for futures
type FuturesWebSocketManager struct {
	orderBookStreams map[string]*WebSocketStream
	tradeStreams     map[string]*WebSocketStream
	userDataStream   *WebSocketStream
	markPriceStream  *WebSocketStream
}

// PositionRisk represents position risk information
type PositionRisk struct {
	Symbol           string          `json:"symbol"`
	PositionAmount   decimal.Decimal `json:"positionAmt"`
	EntryPrice       decimal.Decimal `json:"entryPrice"`
	MarkPrice        decimal.Decimal `json:"markPrice"`
	UnrealizedPnL    decimal.Decimal `json:"unRealizedProfit"`
	LiquidationPrice decimal.Decimal `json:"liquidationPrice"`
	Leverage         int             `json:"leverage"`
	MarginType       string          `json:"marginType"`
	IsolatedMargin   decimal.Decimal `json:"isolatedMargin"`
	PositionSide     string          `json:"positionSide"`
	UpdateTime       time.Time       `json:"updateTime"`
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
func (b *BinanceFuturesMultiAccount) CancelOrder(ctx context.Context, symbol string, orderID string) error {
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
	
	// Use symbol parameter directly
	
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

// GetAccountInfo returns account information
func (b *BinanceFuturesMultiAccount) GetAccountInfo(ctx context.Context) (*types.AccountInfo, error) {
	b.mu.RLock()
	client, exists := b.clients[b.currentAccount]
	accountID := b.currentAccount
	b.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("no client for current account")
	}
	
	account, err := client.NewGetAccountService().Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account info: %w", err)
	}
	
	balances := make([]types.Balance, 0)
	for _, asset := range account.Assets {
		free, _ := decimal.NewFromString(asset.AvailableBalance)
		locked, _ := decimal.NewFromString(asset.InitialMargin)
		total, _ := decimal.NewFromString(asset.WalletBalance)
		
		if free.IsZero() && locked.IsZero() && total.IsZero() {
			continue
		}
		
		balances = append(balances, types.Balance{
			Asset:  asset.Asset,
			Free:   free,
			Locked: locked,
			Total:  total,
		})
	}
	
	return &types.AccountInfo{
		Exchange:    types.ExchangeBinance,
		AccountID:   accountID,
		AccountType: "futures",
		Balances:    balances,
		UpdateTime:  time.Now(),
	}, nil
}

// GetBalances returns all non-zero balances
func (b *BinanceFuturesMultiAccount) GetBalances(ctx context.Context) ([]types.Balance, error) {
	accountInfo, err := b.GetAccountInfo(ctx)
	if err != nil {
		return nil, err
	}
	
	return accountInfo.Balances, nil
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
	
	// Build account info
	accountInfo := &types.AccountInfo{
		Exchange:    types.ExchangeBinance,
		AccountID:   accountID,
		AccountType: "futures",
		Balances:    []types.Balance{},
		UpdateTime:  time.Now(),
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
		
		// Add to balance list
		if asset == "" || bal.Asset == asset {
			accountInfo.Balances = append(accountInfo.Balances, types.Balance{
				Asset:  bal.Asset,
				Free:   free,
				Locked: locked,
				Total:  free.Add(locked),
			})
			
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
	
	// Return single balance if specific asset requested
	if asset != "" && len(accountInfo.Balances) > 0 {
		return &accountInfo.Balances[0], nil
	}
	
	// For multiple assets, return USDT balance
	for _, bal := range accountInfo.Balances {
		if bal.Asset == "USDT" {
			return &bal, nil
		}
	}
	
	// Return empty balance if not found
	return &types.Balance{
		Asset: asset,
		Free:  decimal.Zero,
		Locked: decimal.Zero,
		Total: decimal.Zero,
	}, nil
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
		Symbol:        pos.Symbol,
		Side:          types.PositionSide(side),
		Amount:        positionAmt,
		EntryPrice:    entryPrice,
		MarkPrice:     markPrice,
		UnrealizedPnL: unRealizedProfit,
		RealizedPnL:   decimal.Zero,
		Leverage:      int(leverage.IntPart()),
		UpdateTime:    time.Now(),
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
		callback(symbol, orderBook)
	}
	
	// Start WebSocket
	done, stop, err := futures.WsPartialDepthServe(symbol, 20, wsHandler, nil)
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
			b.handleOrderUpdate(accountID, &event.OrderTradeUpdate)
			
		case futures.UserDataEventTypeAccountUpdate:
			// Handle account update
			b.handleAccountUpdate(accountID, &event.AccountUpdate)
			
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
				Amount:        positionAmt.Abs(),
				Side:          b.getPositionSide(positionAmt),
				EntryPrice:    decimal.RequireFromString(pos.EntryPrice),
				UnrealizedPnL: decimal.RequireFromString(pos.UnrealizedPnL),
				UpdateTime:    time.Now(),
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
		Symbol:     event.Symbol,
		Bids:       bids,
		Asks:       asks,
		UpdateTime: time.Now(),
	}
}

// GetPositionRisk retrieves detailed position risk info for a symbol
func (b *BinanceFuturesMultiAccount) GetPositionRisk(ctx context.Context, accountName, symbol string) (*types.PositionRisk, error) {
	b.mu.RLock()
	accountID := accountName
	if accountID == "" {
		accountID = b.currentAccount
	}
	client, exists := b.clients[accountID]
	b.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("account %s not connected", accountID)
	}
	
	// Check rate limit
	if err := b.checkRateLimit(accountID, 5); err != nil {
		return nil, err
	}
	
	// Get position risk info
	service := client.NewGetPositionRiskService()
	if symbol != "" {
		service.Symbol(symbol)
	}
	
	risks, err := service.Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get position risk: %w", err)
	}
	
	// Update rate limit
	b.updateRateLimit(accountID, 5)
	
	// Find the position for the symbol
	for _, risk := range risks {
		if risk.Symbol == symbol {
			positionAmt, _ := decimal.NewFromString(risk.PositionAmt)
			if positionAmt.IsZero() {
				return nil, fmt.Errorf("no position found for symbol %s", symbol)
			}
			
			entryPrice, _ := decimal.NewFromString(risk.EntryPrice)
			markPrice, _ := decimal.NewFromString(risk.MarkPrice)
			unRealizedProfit, _ := decimal.NewFromString(risk.UnRealizedProfit)
			liquidationPrice, _ := decimal.NewFromString(risk.LiquidationPrice)
			leverage, _ := strconv.Atoi(risk.Leverage)
			// maxNotional, _ := decimal.NewFromString(risk.MaxNotional) // Not available in current binance-go
			
			side := types.Side("LONG")
			if positionAmt.LessThan(decimal.Zero) {
				side = types.Side("SHORT")
				positionAmt = positionAmt.Abs()
			}
			
			return &types.PositionRisk{
				Symbol:           risk.Symbol,
				Side:             side,
				PositionAmt:      positionAmt,
				EntryPrice:       entryPrice,
				MarkPrice:        markPrice,
				UnrealizedProfit: unRealizedProfit,
				LiquidationPrice: liquidationPrice,
				Leverage:         leverage,
				MarginType:       risk.MarginType,
				IsolatedMargin:   decimal.RequireFromString(risk.IsolatedMargin),
				IsAutoAddMargin:  risk.IsAutoAddMargin == "true",
				// MaxNotional:      maxNotional,
				UpdateTime:       time.Now(), // UpdateTime not available in risk struct
			}, nil
		}
	}
	
	return nil, fmt.Errorf("position not found for symbol %s", symbol)
}

// ClosePosition closes a position for a symbol
func (b *BinanceFuturesMultiAccount) ClosePosition(ctx context.Context, accountName, symbol string, reduceOnly bool) error {
	// Get position risk first to determine side and quantity
	positionRisk, err := b.GetPositionRisk(ctx, accountName, symbol)
	if err != nil {
		return fmt.Errorf("failed to get position risk: %w", err)
	}
	
	// Determine order side (opposite of position side)
	var orderSide types.OrderSide
	if positionRisk.Side == types.Side("LONG") {
		orderSide = types.OrderSideSell
	} else {
		orderSide = types.OrderSideBuy
	}
	
	// Create market order to close position
	order := &types.Order{
		Symbol:       symbol,
		Side:         orderSide,
		Type:         types.OrderTypeMarket,
		Quantity:     positionRisk.PositionAmt,
		ReduceOnly:   true, // Always true when closing position
		TimeInForce:  types.TimeInForceGTC,
		PositionSide: "BOTH", // For one-way mode
	}
	
	// Use the account's order creation
	b.mu.Lock()
	oldAccount := b.currentAccount
	if accountName != "" && accountName != b.currentAccount {
		b.currentAccount = accountName
	}
	b.mu.Unlock()
	
	// Create the order
	_, err = b.CreateOrder(ctx, order)
	
	// Restore original account
	b.mu.Lock()
	b.currentAccount = oldAccount
	b.mu.Unlock()
	
	if err != nil {
		return fmt.Errorf("failed to close position: %w", err)
	}
	
	return nil
}

// AdjustPositionMargin adjusts isolated margin for a position
func (b *BinanceFuturesMultiAccount) AdjustPositionMargin(ctx context.Context, accountName, symbol string, amount decimal.Decimal, addOrReduce int) error {
	b.mu.RLock()
	accountID := accountName
	if accountID == "" {
		accountID = b.currentAccount
	}
	client, exists := b.clients[accountID]
	b.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("account %s not connected", accountID)
	}
	
	// Check rate limit
	if err := b.checkRateLimit(accountID, 1); err != nil {
		return err
	}
	
	// Adjust position margin
	// addOrReduce: 1=add, 2=reduce
	service := client.NewUpdatePositionMarginService().
		Symbol(symbol).
		Amount(amount.String()).
		Type(addOrReduce)
	
	err := service.Do(ctx)
	if err != nil {
		return fmt.Errorf("failed to adjust position margin: %w", err)
	}
	
	// Update rate limit
	b.updateRateLimit(accountID, 1)
	
	// Log the result
	fmt.Printf("Position margin adjusted successfully for %s\n", symbol)
	
	return nil
}

// SubscribePositionUpdates subscribes to position updates via WebSocket
func (b *BinanceFuturesMultiAccount) SubscribePositionUpdates(accountName string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	accountID := accountName
	if accountID == "" {
		accountID = b.currentAccount
	}
	
	// Check if user data stream is already active
	wsManager, exists := b.wsManagers[accountID]
	if !exists {
		return fmt.Errorf("no WebSocket manager for account %s", accountID)
	}
	
	// If user data stream is already active, position updates are included
	if wsManager.userDataStream != nil {
		return nil
	}
	
	// Otherwise, start user data stream for this account
	client, exists := b.clients[accountID]
	if !exists {
		return fmt.Errorf("account %s not connected", accountID)
	}
	
	// Get listen key
	listenKey, err := client.NewStartUserStreamService().Do(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get listen key: %w", err)
	}
	
	// Create enhanced handler with position tracking
	wsHandler := b.createEnhancedUserDataHandler(accountID)
	
	// Start WebSocket
	done, stop, err := futures.WsUserDataServe(listenKey, wsHandler, nil)
	if err != nil {
		return fmt.Errorf("failed to start user data stream: %w", err)
	}
	
	wsManager.userDataStream = &WebSocketStream{
		Done: done,
		Stop: stop,
	}
	
	// Keep listen key alive
	go b.keepAliveListenKey(accountID, listenKey)
	
	return nil
}

// createEnhancedUserDataHandler creates an enhanced handler that tracks positions
func (b *BinanceFuturesMultiAccount) createEnhancedUserDataHandler(accountID string) futures.WsUserDataHandler {
	return func(event *futures.WsUserDataEvent) {
		switch event.Event {
		case futures.UserDataEventTypeOrderTradeUpdate:
			// Handle order update
			b.handleOrderUpdate(accountID, &event.OrderTradeUpdate)
			
		case futures.UserDataEventTypeAccountUpdate:
			// Handle account update with position tracking
			b.handleEnhancedAccountUpdate(accountID, &event.AccountUpdate)
			
		case futures.UserDataEventTypeMarginCall:
			// Handle margin call
			b.handleMarginCall(accountID, event)
		}
	}
}

// handleEnhancedAccountUpdate handles account updates with position history tracking
func (b *BinanceFuturesMultiAccount) handleEnhancedAccountUpdate(accountID string, update *futures.WsAccountUpdate) {
	// Update positions
	for _, pos := range update.Positions {
		positionAmt, _ := decimal.NewFromString(pos.Amount)
		entryPrice, _ := decimal.NewFromString(pos.EntryPrice)
		unrealizedPnL, _ := decimal.NewFromString(pos.UnrealizedPnL)
		
		b.mu.Lock()
		if positionAmt.IsZero() {
			// Position closed - record history
			if oldPos, exists := b.positions[accountID][pos.Symbol]; exists {
				// TODO: Save position history to file
				fmt.Printf("[%s] Position closed - Symbol: %s, Entry: %f, PnL: %s\n",
					accountID, pos.Symbol, oldPos.EntryPrice, pos.UnrealizedPnL)
			}
			delete(b.positions[accountID], pos.Symbol)
		} else {
			// Update or create position
			b.positions[accountID][pos.Symbol] = &types.Position{
				Symbol:        pos.Symbol,
				Amount:        positionAmt.Abs(),
				Side:          types.PositionSide(b.getPositionSide(positionAmt)),
				EntryPrice:    entryPrice,
				MarkPrice:     decimal.Zero, // Will be updated on next tick
				UnrealizedPnL: unrealizedPnL,
				RealizedPnL:   decimal.Zero,
				Leverage:      0, // Will be updated from account info
				UpdateTime:    time.Now(),
			}
		}
		b.mu.Unlock()
	}
	
	// Update balances
	for _, bal := range update.Balances {
		// Record balance changes
		fmt.Printf("[%s] Balance update - Asset: %s, Balance: %s, Cross: %s\n",
			accountID, bal.Asset, bal.Balance, bal.CrossWalletBalance)
	}
}

// GetKlines retrieves kline/candlestick data
func (ma *BinanceFuturesMultiAccount) GetKlines(ctx context.Context, symbol string, interval types.KlineInterval, limit int) ([]*types.Kline, error) {
	return []*types.Kline{}, nil
}

// GetMarketData retrieves current market data
func (ma *BinanceFuturesMultiAccount) GetMarketData(ctx context.Context, symbols []string) (map[string]*types.MarketData, error) {
	return map[string]*types.MarketData{}, nil
}

// GetOrder retrieves a specific order
func (ma *BinanceFuturesMultiAccount) GetOrder(ctx context.Context, symbol string, orderID string) (*types.Order, error) {
	return &types.Order{}, nil
}

// GetOrderBook retrieves order book data
func (ma *BinanceFuturesMultiAccount) GetOrderBook(ctx context.Context, symbol string, depth int) (*types.OrderBook, error) {
	return &types.OrderBook{}, nil
}

// GetOrderHistory retrieves historical orders
func (ma *BinanceFuturesMultiAccount) GetOrderHistory(ctx context.Context, symbol string, limit int) ([]*types.Order, error) {
	return []*types.Order{}, nil
}

// GetSymbolInfo retrieves symbol trading information
func (ma *BinanceFuturesMultiAccount) GetSymbolInfo(ctx context.Context, symbol string) (*types.SymbolInfo, error) {
	return &types.SymbolInfo{}, nil
}

// GetTrades retrieves recent trades
func (ma *BinanceFuturesMultiAccount) GetTrades(ctx context.Context, symbol string, limit int) ([]*types.Trade, error) {
	return []*types.Trade{}, nil
}

// SubscribeTicker subscribes to ticker updates
func (ma *BinanceFuturesMultiAccount) SubscribeTicker(symbol string, callback types.TickerCallback) error {
	return nil
}

// SubscribeTrades subscribes to trade updates
func (ma *BinanceFuturesMultiAccount) SubscribeTrades(symbol string, callback types.TradeCallback) error {
	return nil
}

// UnsubscribeAll unsubscribes from all streams
func (ma *BinanceFuturesMultiAccount) UnsubscribeAll() error {
	return nil
}

// GetType returns the exchange type
func (b *BinanceFuturesMultiAccount) GetType() types.ExchangeType {
	return types.ExchangeBinance
}

// GetMarketType returns the market type
func (b *BinanceFuturesMultiAccount) GetMarketType() types.MarketType {
	return types.MarketTypeFutures
}

// Initialize initializes the exchange connector
func (b *BinanceFuturesMultiAccount) Initialize(ctx context.Context) error {
	// Already initialized in constructor
	return nil
}

// PlaceOrder places a new order (alias for CreateOrder)
func (b *BinanceFuturesMultiAccount) PlaceOrder(ctx context.Context, order *types.Order) (*types.Order, error) {
	return b.CreateOrder(ctx, order)
}

// Position Management Methods

// GetPositionRisk gets position risk information for a specific symbol
func (b *BinanceFuturesMultiAccount) GetPositionRisk(accountName, symbol string) (*PositionRisk, error) {
	b.mu.RLock()
	client, exists := b.clients[accountName]
	b.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("account %s not found", accountName)
	}
	
	// Check rate limit
	if err := b.checkRateLimit(accountName, 5); err != nil {
		return nil, err
	}
	
	// Get position risk from Binance
	service := client.NewGetPositionRiskService()
	if symbol != "" {
		service = service.Symbol(symbol)
	}
	
	risks, err := service.Do(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get position risk: %v", err)
	}
	
	// Update rate limit
	b.updateRateLimit(accountName, 5)
	
	// Find the position for the symbol
	for _, risk := range risks {
		if risk.Symbol == symbol {
			posAmt, _ := decimal.NewFromString(risk.PositionAmt)
			if posAmt.IsZero() {
				continue
			}
			
			entryPrice, _ := decimal.NewFromString(risk.EntryPrice)
			markPrice, _ := decimal.NewFromString(risk.MarkPrice)
			unPnl, _ := decimal.NewFromString(risk.UnRealizedProfit)
			liqPrice, _ := decimal.NewFromString(risk.LiquidationPrice)
			isoMargin, _ := decimal.NewFromString(risk.IsolatedMargin)
			leverage, _ := strconv.Atoi(risk.Leverage)
			
			return &PositionRisk{
				Symbol:           risk.Symbol,
				PositionAmount:   posAmt,
				EntryPrice:       entryPrice,
				MarkPrice:        markPrice,
				UnrealizedPnL:    unPnl,
				LiquidationPrice: liqPrice,
				Leverage:         leverage,
				MarginType:       risk.MarginType,
				IsolatedMargin:   isoMargin,
				PositionSide:     risk.PositionSide,
				UpdateTime:       time.Now(),
			}, nil
		}
	}
	
	return nil, fmt.Errorf("no position found for symbol %s", symbol)
}

// ClosePosition closes a position for a specific symbol
func (b *BinanceFuturesMultiAccount) ClosePosition(accountName, symbol string, reduceOnly bool) error {
	// Get current position
	posRisk, err := b.GetPositionRisk(accountName, symbol)
	if err != nil {
		return fmt.Errorf("failed to get position: %v", err)
	}
	
	if posRisk.PositionAmount.IsZero() {
		return fmt.Errorf("no position to close for %s", symbol)
	}
	
	// Determine side for closing
	var side types.OrderSide
	if posRisk.PositionAmount.IsPositive() {
		side = types.OrderSideSell // Close long position
	} else {
		side = types.OrderSideBuy // Close short position
	}
	
	// Create market order to close position
	order := &types.Order{
		Symbol:       symbol,
		Side:         side,
		Type:         types.OrderTypeMarket,
		Quantity:     posRisk.PositionAmount.Abs(),
		ReduceOnly:   reduceOnly,
		PositionSide: types.PositionSide(posRisk.PositionSide),
	}
	
	// Place the order
	_, err = b.CreateOrder(context.Background(), order)
	if err != nil {
		return fmt.Errorf("failed to close position: %v", err)
	}
	
	return nil
}

// AdjustPositionMargin adjusts isolated margin for a position
func (b *BinanceFuturesMultiAccount) AdjustPositionMargin(accountName, symbol string, amount decimal.Decimal, addOrReduce int) error {
	b.mu.RLock()
	client, exists := b.clients[accountName]
	b.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("account %s not found", accountName)
	}
	
	// Check rate limit
	if err := b.checkRateLimit(accountName, 4); err != nil {
		return err
	}
	
	// Adjust position margin
	service := client.NewUpdatePositionMarginService()
	service = service.Symbol(symbol)
	service = service.Amount(amount.String())
	service = service.Type(addOrReduce) // 1: Add, 2: Reduce
	
	_, err := service.Do(context.Background())
	if err != nil {
		return fmt.Errorf("failed to adjust position margin: %v", err)
	}
	
	// Update rate limit
	b.updateRateLimit(accountName, 4)
	
	return nil
}

// SubscribePositionUpdates subscribes to position updates via WebSocket
func (b *BinanceFuturesMultiAccount) SubscribePositionUpdates(accountName string) error {
	b.mu.RLock()
	wsManager, exists := b.wsManagers[accountName]
	b.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("WebSocket manager not found for account %s", accountName)
	}
	
	// User data stream already includes position updates
	// Just ensure the handler processes ACCOUNT_UPDATE events
	if wsManager.userDataStream != nil && wsManager.userDataStream.IsConnected() {
		// Position updates are already being received
		return nil
	}
	
	// If not connected, subscribe to user data
	return b.SubscribeUserData(accountName)
}

// SetPositionUpdateCallback sets the callback for position updates
func (b *BinanceFuturesMultiAccount) SetPositionUpdateCallback(callback func(accountID string, position *types.Position)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onPositionUpdate = callback
}

// Leverage and Margin Type Methods

// ChangeInitialLeverage changes the initial leverage for a symbol
func (b *BinanceFuturesMultiAccount) ChangeInitialLeverage(accountName, symbol string, leverage int) error {
	b.mu.RLock()
	client, exists := b.clients[accountName]
	b.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("account %s not found", accountName)
	}
	
	// Check rate limit
	if err := b.checkRateLimit(accountName, 1); err != nil {
		return err
	}
	
	// Change leverage
	service := client.NewChangeLeverageService()
	service = service.Symbol(symbol)
	service = service.Leverage(leverage)
	
	resp, err := service.Do(context.Background())
	if err != nil {
		return fmt.Errorf("failed to change leverage: %v", err)
	}
	
	// Update rate limit
	b.updateRateLimit(accountName, 1)
	
	fmt.Printf("Leverage changed for %s: %d (max: %d)\n", resp.Symbol, resp.Leverage, resp.MaxLeverage)
	
	return nil
}

// ChangeMarginType changes the margin type for a symbol
func (b *BinanceFuturesMultiAccount) ChangeMarginType(accountName, symbol string, marginType string) error {
	b.mu.RLock()
	client, exists := b.clients[accountName]
	b.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("account %s not found", accountName)
	}
	
	// Check rate limit
	if err := b.checkRateLimit(accountName, 1); err != nil {
		return err
	}
	
	// Validate margin type
	if marginType != "ISOLATED" && marginType != "CROSSED" {
		return fmt.Errorf("invalid margin type: %s (must be ISOLATED or CROSSED)", marginType)
	}
	
	// Change margin type
	service := client.NewChangeMarginTypeService()
	service = service.Symbol(symbol)
	service = service.MarginType(futures.MarginType(marginType))
	
	err := service.Do(context.Background())
	if err != nil {
		return fmt.Errorf("failed to change margin type: %v", err)
	}
	
	// Update rate limit
	b.updateRateLimit(accountName, 1)
	
	return nil
}

// GetMaxLeverage gets the maximum leverage allowed for a symbol
func (b *BinanceFuturesMultiAccount) GetMaxLeverage(accountName, symbol string) (int, error) {
	b.mu.RLock()
	client, exists := b.clients[accountName]
	b.mu.RUnlock()
	
	if !exists {
		return 0, fmt.Errorf("account %s not found", accountName)
	}
	
	// Get exchange info
	info, err := client.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		return 0, fmt.Errorf("failed to get exchange info: %v", err)
	}
	
	// Find symbol info
	for _, s := range info.Symbols {
		if s.Symbol == symbol {
			// Parse leverage brackets
			// Note: This is simplified. In practice, leverage varies by position size
			// You might need to use the leverage brackets endpoint for accurate info
			return 125, nil // Default max for most symbols
		}
	}
	
	return 0, fmt.Errorf("symbol %s not found", symbol)
}