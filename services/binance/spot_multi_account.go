package binance

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	binance "github.com/adshao/go-binance/v2"
	"github.com/mExOms/pkg/types"
	"github.com/mExOms/pkg/vault"
	"github.com/shopspring/decimal"
)

// BinanceSpotMultiAccount implements multi-account support for Binance Spot
type BinanceSpotMultiAccount struct {
	mu sync.RWMutex
	
	// Multi-account support
	clients         map[string]*binance.Client
	currentAccount  string
	accountManager  types.AccountManager
	
	// Connection state
	connected       bool
	testnet         bool
	
	// WebSocket managers per account
	wsManagers      map[string]*WebSocketManager
	
	// WebSocket order manager
	wsOrderManager  types.WebSocketOrderManager
	
	// Rate limiting per account
	rateLimiters    map[string]*RateLimiter
	
	// Vault client for API key management
	vaultClient     *vault.Client
}

// WebSocketManager manages WebSocket connections for an account
type WebSocketManager struct {
	orderBookStreams map[string]*WebSocketStream
	tradeStreams     map[string]*WebSocketStream
	userDataStream   *WebSocketStream
}

// WebSocketStream holds WebSocket stream control channels
type WebSocketStream struct {
	Done chan struct{}
	Stop chan struct{}
}

// RateLimiter tracks rate limit usage
type RateLimiter struct {
	weight        int
	orders        int
	windowStart   time.Time
	mu            sync.Mutex
}

// NewBinanceSpotMultiAccount creates a new multi-account Binance Spot connector
func NewBinanceSpotMultiAccount(accountManager types.AccountManager, testnet bool) (*BinanceSpotMultiAccount, error) {
	// Create Vault client
	vaultClient, err := vault.NewClient(vault.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %v", err)
	}

	return &BinanceSpotMultiAccount{
		clients:        make(map[string]*binance.Client),
		accountManager: accountManager,
		testnet:        testnet,
		wsManagers:     make(map[string]*WebSocketManager),
		rateLimiters:   make(map[string]*RateLimiter),
		vaultClient:    vaultClient,
	}, nil
}

// Connect establishes connections for all configured accounts
func (b *BinanceSpotMultiAccount) Connect(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	// Initialize WebSocket order manager first
	if b.wsOrderManager == nil {
		// Get credentials from Vault for WebSocket
		keys, err := b.vaultClient.GetExchangeKeys("binance", "spot")
		if err != nil {
			return fmt.Errorf("failed to get API keys for WebSocket: %v", err)
		}
		
		wsConfig := types.WebSocketConfig{
			URL:                "wss://ws-api.binance.com:443/ws-api/v3",
			APIKey:             keys["api_key"],
			SecretKey:          keys["secret_key"],
			PingInterval:       30 * time.Second,
			ReconnectInterval:  5 * time.Second,
			MessageTimeout:     10 * time.Second,
			EnableCompression:  true,
			EnableHeartbeat:    true,
		}
		
		if b.testnet {
			wsConfig.URL = "wss://testnet.binance.vision/ws-api/v3"
		}
		
		b.wsOrderManager = NewBinanceWSOrderManager(wsConfig)
		if err := b.wsOrderManager.Connect(ctx); err != nil {
			return fmt.Errorf("failed to connect WebSocket order manager: %v", err)
		}
	}
	
	// Get all Binance accounts
	filter := types.AccountFilter{
		Exchange: "binance",
		Active:   &[]bool{true}[0],
		Market:   types.MarketTypeSpot,
	}
	
	accounts, err := b.accountManager.ListAccounts(filter)
	if err != nil {
		return fmt.Errorf("failed to list accounts: %w", err)
	}
	
	if len(accounts) == 0 {
		return fmt.Errorf("no active Binance spot accounts found")
	}
	
	// Connect each account
	for _, account := range accounts {
		if account.SpotEnabled {
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

// connectAccount connects a single account
func (b *BinanceSpotMultiAccount) connectAccount(ctx context.Context, account *types.Account) error {
	// Get API credentials from vault/config
	apiKey, apiSecret, err := b.getAccountCredentials(account)
	if err != nil {
		return err
	}
	
	// Create client
	var client *binance.Client
	if b.testnet {
		binance.UseTestnet = true
		client = binance.NewClient(apiKey, apiSecret)
	} else {
		client = binance.NewClient(apiKey, apiSecret)
	}
	
	// Test connection
	err = client.NewPingService().Do(ctx)
	if err != nil {
		return fmt.Errorf("failed to ping: %w", err)
	}
	
	// Store client
	b.clients[account.ID] = client
	
	// Initialize rate limiter
	b.rateLimiters[account.ID] = &RateLimiter{
		windowStart: time.Now(),
	}
	
	// Initialize WebSocket manager
	b.wsManagers[account.ID] = &WebSocketManager{
		orderBookStreams: make(map[string]*WebSocketStream),
		tradeStreams:     make(map[string]*WebSocketStream),
	}
	
	return nil
}

// Disconnect closes all connections
func (b *BinanceSpotMultiAccount) Disconnect() error {
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
		
		delete(b.wsManagers, accountID)
	}
	
	b.connected = false
	return nil
}

// IsConnected returns connection status
func (b *BinanceSpotMultiAccount) IsConnected() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.connected
}

// GetName returns exchange name
func (b *BinanceSpotMultiAccount) GetName() string {
	return "binance"
}

// GetMarket returns market type
func (b *BinanceSpotMultiAccount) GetMarket() types.MarketType {
	return types.MarketTypeSpot
}

// SetAccount sets the current account for operations
func (b *BinanceSpotMultiAccount) SetAccount(accountID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	if _, exists := b.clients[accountID]; !exists {
		return fmt.Errorf("account %s not connected", accountID)
	}
	
	b.currentAccount = accountID
	return nil
}

// GetCurrentAccount returns the current account ID
func (b *BinanceSpotMultiAccount) GetCurrentAccount() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.currentAccount
}

// SupportSubAccounts returns true as Binance supports sub-accounts
func (b *BinanceSpotMultiAccount) SupportSubAccounts() bool {
	return true
}

// CreateOrder creates an order using WebSocket (preferred) or REST API
func (b *BinanceSpotMultiAccount) CreateOrder(ctx context.Context, order *types.Order) (*types.Order, error) {
	// Try WebSocket first if available
	if b.wsOrderManager != nil && b.wsOrderManager.IsConnected() {
		orderResp, err := b.wsOrderManager.CreateOrder(ctx, order)
		if err == nil {
			// Convert WebSocket response to Order
			return &types.Order{
				ID:           orderResp.OrderID,
				Symbol:       orderResp.Symbol,
				Side:         orderResp.Side,
				Type:         orderResp.Type,
				Price:        decimal.RequireFromString(orderResp.Price),
				Quantity:     decimal.RequireFromString(orderResp.Quantity),
				Status:       orderResp.Status,
				TimeInForce:  orderResp.TimeInForce,
				CreatedAt:    time.Unix(0, orderResp.TransactTime*int64(time.Millisecond)),
			}, nil
		}
		// Fall back to REST if WebSocket fails
		// Log the WebSocket error for monitoring
		fmt.Printf("WebSocket order failed, falling back to REST: %v\n", err)
	}
	
	// REST API fallback
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
	
	// Create order service via REST
	service := client.NewCreateOrderService().
		Symbol(order.Symbol).
		Side(binance.SideType(order.Side)).
		Type(binance.OrderType(order.Type))
	
	// Set quantity
	service.Quantity(order.Quantity.String())
	
	// Set price for limit orders
	if order.Type == types.OrderTypeLimit {
		service.Price(order.Price.String())
		service.TimeInForce(binance.TimeInForceType(order.TimeInForce))
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
	order.CreatedAt = time.UnixMilli(response.TransactTime)
	
	// Record order with account info
	order.Metadata = map[string]interface{}{
		"account_id": accountID,
		"exchange":   "binance",
		"market":     "spot",
	}
	
	return order, nil
}

// CancelOrder cancels an order
func (b *BinanceSpotMultiAccount) CancelOrder(ctx context.Context, orderID string) error {
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

// GetOrder retrieves order details
func (b *BinanceSpotMultiAccount) GetOrder(ctx context.Context, orderID string) (*types.Order, error) {
	b.mu.RLock()
	_, exists := b.clients[b.currentAccount] // client will be used when implemented
	accountID := b.currentAccount
	b.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("no client for current account")
	}
	
	// Check rate limit
	if err := b.checkRateLimit(accountID, 2); err != nil {
		return nil, err
	}
	
	// TODO: Implement order query
	// Need to track symbol with order ID
	
	// Update rate limit
	b.updateRateLimit(accountID, 2)
	
	return nil, fmt.Errorf("not implemented")
}

// GetOpenOrders retrieves open orders for the current account
func (b *BinanceSpotMultiAccount) GetOpenOrders(ctx context.Context, symbol string) ([]*types.Order, error) {
	b.mu.RLock()
	client, exists := b.clients[b.currentAccount]
	accountID := b.currentAccount
	b.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("no client for current account")
	}
	
	// Check rate limit
	weight := 3
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
		orders = append(orders, b.convertOrder(bo, accountID))
	}
	
	return orders, nil
}

// GetBalance retrieves balance for the current account
func (b *BinanceSpotMultiAccount) GetBalance(ctx context.Context, asset string) (*types.Balance, error) {
	b.mu.RLock()
	accountID := b.currentAccount
	b.mu.RUnlock()
	
	return b.GetBalanceForAccount(ctx, accountID, asset)
}

// GetBalanceForAccount retrieves balance for a specific account
func (b *BinanceSpotMultiAccount) GetBalanceForAccount(ctx context.Context, accountID, asset string) (*types.Balance, error) {
	b.mu.RLock()
	client, exists := b.clients[accountID]
	b.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("account %s not connected", accountID)
	}
	
	// Check rate limit
	if err := b.checkRateLimit(accountID, 10); err != nil {
		return nil, err
	}
	
	// Get account info
	account, err := client.NewGetAccountService().Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}
	
	// Update rate limit
	b.updateRateLimit(accountID, 10)
	
	// Build balance
	balance := &types.Balance{
		Exchange: "binance",
		Market:   "spot",
		Assets:   make(map[string]types.AssetBalance),
	}
	
	// Find specific asset or add all assets
	totalUSDT := decimal.Zero
	
	for _, bal := range account.Balances {
		free, _ := decimal.NewFromString(bal.Free)
		locked, _ := decimal.NewFromString(bal.Locked)
		
		// Skip zero balances unless specifically requested
		if asset == "" && free.IsZero() && locked.IsZero() {
			continue
		}
		
		// Add to balance map
		if asset == "" || bal.Asset == asset {
			balance.Assets[bal.Asset] = types.AssetBalance{
				Asset:  bal.Asset,
				Free:   bal.Free,
				Locked: bal.Locked,
			}
			
			if asset != "" {
				break // Found the requested asset
			}
		}
		
		// Calculate USDT value (simplified - need price data)
		if bal.Asset == "USDT" {
			totalUSDT = totalUSDT.Add(free).Add(locked)
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

// ListSubAccounts lists all sub-accounts
func (b *BinanceSpotMultiAccount) ListSubAccounts(ctx context.Context) ([]*types.SubAccountInfo, error) {
	// This requires master account API
	// For now, return accounts from account manager
	
	filter := types.AccountFilter{
		Exchange: "binance",
		Type:     types.AccountTypeSub,
	}
	
	accounts, err := b.accountManager.ListAccounts(filter)
	if err != nil {
		return nil, err
	}
	
	subAccounts := make([]*types.SubAccountInfo, 0, len(accounts))
	for _, acc := range accounts {
		subAccounts = append(subAccounts, &types.SubAccountInfo{
			AccountID:      acc.ID,
			Name:           acc.Name,
			Type:           string(acc.Type),
			SpotEnabled:    acc.SpotEnabled,
			FuturesEnabled: acc.FuturesEnabled,
			Active:         acc.Active,
			CreateTime:     acc.CreatedAt,
		})
	}
	
	return subAccounts, nil
}

// TransferBetweenAccounts transfers assets between accounts
func (b *BinanceSpotMultiAccount) TransferBetweenAccounts(ctx context.Context, transfer *types.AccountTransferRequest) (*types.AccountTransferResponse, error) {
	// This requires master account API with sub-account transfer permission
	// Implementation depends on Binance sub-account API
	
	// For now, record transfer request in account manager
	accountTransfer := &types.AccountTransfer{
		FromAccount: transfer.FromAccountID,
		ToAccount:   transfer.ToAccountID,
		Asset:       transfer.Asset,
		Amount:      transfer.Amount,
		Status:      "pending",
	}
	
	if err := b.accountManager.Transfer(accountTransfer); err != nil {
		return nil, err
	}
	
	return &types.AccountTransferResponse{
		TransferID:   accountTransfer.ID,
		Status:       accountTransfer.Status,
		Amount:       transfer.Amount,
		Asset:        transfer.Asset,
		FromAccount:  transfer.FromAccountID,
		ToAccount:    transfer.ToAccountID,
		TransferTime: time.Now(),
	}, nil
}

// Helper methods

// getAccountCredentials retrieves API credentials for an account
func (b *BinanceSpotMultiAccount) getAccountCredentials(account *types.Account) (apiKey, apiSecret string, err error) {
	// Retrieve from Vault
	keys, err := b.vaultClient.GetExchangeKeys("binance", "spot")
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
func (b *BinanceSpotMultiAccount) checkRateLimit(accountID string, weight int) error {
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
	
	// Check limits (Binance: 1200 weight/min, 10 orders/sec)
	if limiter.weight+weight > 1200 {
		return fmt.Errorf("rate limit exceeded for account %s", accountID)
	}
	
	return nil
}

// updateRateLimit updates rate limit usage
func (b *BinanceSpotMultiAccount) updateRateLimit(accountID string, weight int) {
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
		RemainingWeight: 1200 - limiter.weight,
		UpdatedAt:       time.Now(),
	})
}

// GetWebSocketOrderManager returns the WebSocket order manager
func (b *BinanceSpotMultiAccount) GetWebSocketOrderManager() types.WebSocketOrderManager {
	return b.wsOrderManager
}

// GetWebSocketInfo returns WebSocket capabilities info
func (b *BinanceSpotMultiAccount) GetWebSocketInfo() types.ExchangeWebSocketInfo {
	return types.ExchangeWebSocketInfo{
		Supported: true,
		OrderSupport: types.WebSocketOrderSupport{
			CreateOrder:  true,
			CancelOrder:  true,
			ModifyOrder:  false, // Binance doesn't support order modification
			OrderStatus:  true,
			OpenOrders:   true,
			OrderUpdates: true,
		},
		BaseURL:       "wss://ws-api.binance.com:443/ws-api/v3",
		TestnetURL:    "wss://testnet.binance.vision/ws-api/v3",
		Documentation: "https://binance-docs.github.io/apidocs/websocket_api/en/",
	}
}

// convertOrder converts Binance order to internal format
func (b *BinanceSpotMultiAccount) convertOrder(bo *binance.Order, accountID string) *types.Order {
	price, _ := decimal.NewFromString(bo.Price)
	quantity, _ := decimal.NewFromString(bo.OrigQuantity)
	
	return &types.Order{
		ClientOrderID:   bo.ClientOrderID,
		ExchangeOrderID: fmt.Sprintf("%d", bo.OrderID),
		Symbol:          bo.Symbol,
		Side:            string(bo.Side),
		Type:            string(bo.Type),
		Status:          string(bo.Status),
		Price:           price,
		Quantity:        quantity,
		TimeInForce:     string(bo.TimeInForce),
		CreatedAt:       time.UnixMilli(bo.Time),
		UpdatedAt:       time.UnixMilli(bo.UpdateTime),
		Metadata: map[string]interface{}{
			"account_id": accountID,
			"exchange":   "binance",
			"market":     "spot",
		},
	}
}

// SubscribeOrderBook subscribes to order book updates
func (b *BinanceSpotMultiAccount) SubscribeOrderBook(symbol string, callback types.OrderBookCallback) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	// Subscribe for current account
	wsManager, exists := b.wsManagers[b.currentAccount]
	if !exists {
		return fmt.Errorf("no WebSocket manager for current account")
	}
	
	// Create WebSocket handler
	wsHandler := func(event *binance.WsDepthEvent) {
		orderBook := b.convertOrderBook(event)
		callback(symbol, orderBook)
	}
	
	// Start WebSocket
	done, stop, err := binance.WsDepthServe(symbol, wsHandler, nil)
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

// convertOrderBook converts Binance order book to internal format
func (b *BinanceSpotMultiAccount) convertOrderBook(event *binance.WsDepthEvent) *types.OrderBook {
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