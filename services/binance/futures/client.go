package futures

import (
	"context"
	"fmt"
	"strconv"
	"time"
	
	"github.com/adshao/go-binance/v2/futures"
	"github.com/mExOms/pkg/cache"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

type BinanceFutures struct {
	client       *futures.Client
	wsClient     map[string]interface{}
	cache        *cache.MemoryCache
	rateLimiter  *cache.RateLimiter
	natsClient   interface{} // Will be set to actual NATS client later
	apiKey       string
	apiSecret    string
	testnet      bool
}

func NewBinanceFutures(apiKey, apiSecret string, testnet bool) (*BinanceFutures, error) {
	var client *futures.Client
	
	if testnet {
		futures.UseTestnet = true
	}
	
	client = futures.NewClient(apiKey, apiSecret)
	
	bf := &BinanceFutures{
		client:      client,
		wsClient:    make(map[string]interface{}),
		cache:       cache.NewMemoryCache(),
		rateLimiter: cache.NewRateLimiter(2400, time.Minute), // Futures has higher limits
		apiKey:      apiKey,
		apiSecret:   apiSecret,
		testnet:     testnet,
	}
	
	return bf, nil
}

// GetName returns the exchange name
func (bf *BinanceFutures) GetName() string {
	return "binance"
}

// GetMarket returns the market type
func (bf *BinanceFutures) GetMarket() string {
	return "futures"
}

// IsConnected checks if the connection is active
func (bf *BinanceFutures) IsConnected() bool {
	err := bf.client.NewPingService().Do(context.Background())
	return err == nil
}

// GetExchangeInfo retrieves exchange information for futures
func (bf *BinanceFutures) GetExchangeInfo() (*types.ExchangeInfo, error) {
	if !bf.rateLimiter.Allow("exchange_info") {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	
	// Check cache first
	if cached, exists := bf.cache.Get("exchange_info"); exists {
		return cached.(*types.ExchangeInfo), nil
	}
	
	info, err := bf.client.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		return nil, err
	}
	
	exchangeInfo := &types.ExchangeInfo{
		Exchange: "binance",
		Market:   "futures",
		Symbols:  make([]types.Symbol, 0, len(info.Symbols)),
	}
	
	for _, s := range info.Symbols {
		if s.Status != "TRADING" || s.ContractType != "PERPETUAL" {
			continue
		}
		
		symbol := types.Symbol{
			Symbol:      s.Symbol,
			Base:        s.BaseAsset,
			Quote:       s.QuoteAsset,
			MinQty:      s.LotSizeFilter().MinQuantity,
			MaxQty:      s.LotSizeFilter().MaxQuantity,
			StepSize:    s.LotSizeFilter().StepSize,
			MinNotional: s.MinNotionalFilter().Notional,
			Status:      s.Status,
		}
		exchangeInfo.Symbols = append(exchangeInfo.Symbols, symbol)
	}
	
	// Cache for 1 hour
	bf.cache.Set("exchange_info", exchangeInfo, time.Hour)
	
	return exchangeInfo, nil
}

// GetAccount retrieves futures account information
func (bf *BinanceFutures) GetAccount() (*types.FuturesAccount, error) {
	if !bf.rateLimiter.Allow("account") {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	
	account, err := bf.client.NewGetAccountService().Do(context.Background())
	if err != nil {
		return nil, err
	}
	
	futuresAccount := &types.FuturesAccount{
		TotalBalance:           parseDecimal(account.TotalWalletBalance),
		AvailableBalance:       parseDecimal(account.AvailableBalance),
		TotalMargin:            parseDecimal(account.TotalMarginBalance),
		TotalUnrealizedPnL:     parseDecimal(account.TotalUnrealizedProfit),
		TotalMaintenanceMargin: parseDecimal(account.TotalMaintMargin),
		Assets:                 make([]types.FuturesAsset, 0, len(account.Assets)),
		Positions:              make([]types.FuturesPosition, 0, len(account.Positions)),
		UpdateTime:             time.Unix(account.UpdateTime/1000, 0),
	}
	
	// Process assets
	for _, asset := range account.Assets {
		futuresAccount.Assets = append(futuresAccount.Assets, types.FuturesAsset{
			Asset:             asset.Asset,
			Balance:           parseDecimal(asset.WalletBalance),
			CrossBalance:      parseDecimal(asset.CrossWalletBalance),
			CrossUnPnL:        parseDecimal(asset.CrossUnPnl),
			AvailableBalance:  parseDecimal(asset.AvailableBalance),
			MaxWithdrawAmount: parseDecimal(asset.MaxWithdrawAmount),
			MarginAvailable:   asset.MarginAvailable,
			UpdateTime:        time.Unix(asset.UpdateTime/1000, 0),
		})
	}
	
	// Process positions
	for _, pos := range account.Positions {
		if pos.PositionAmt == "0" {
			continue
		}
		
		leverage := int(parseDecimal(pos.Leverage).IntPart())
		futuresAccount.Positions = append(futuresAccount.Positions, types.FuturesPosition{
			Symbol:                 pos.Symbol,
			PositionSide:           string(pos.PositionSide),
			MarginType:             "ISOLATED", // Default, as it's not in account position
			Quantity:               parseDecimal(pos.PositionAmt),
			EntryPrice:             parseDecimal(pos.EntryPrice),
			MarkPrice:              decimal.Zero, // Not available in account position
			UnrealizedPnL:          parseDecimal(pos.UnrealizedProfit),
			Margin:                 parseDecimal(pos.Notional),
			IsolatedMargin:         decimal.Zero, // Not available in account position
			Leverage:               int(leverage),
			MaintenanceMargin:      parseDecimal(pos.MaintMargin),
			InitialMargin:          parseDecimal(pos.PositionInitialMargin),
			PositionInitialMargin:  parseDecimal(pos.PositionInitialMargin),
			OpenOrderInitialMargin: parseDecimal(pos.OpenOrderInitialMargin),
			UpdateTime:             time.Unix(pos.UpdateTime/1000, 0),
		})
	}
	
	// Cache for 5 seconds
	bf.cache.Set("futures_account", futuresAccount, 5*time.Second)
	
	return futuresAccount, nil
}

// GetPositions retrieves current positions
func (bf *BinanceFutures) GetPositions() ([]types.FuturesPosition, error) {
	if !bf.rateLimiter.Allow("positions") {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	
	risks, err := bf.client.NewGetPositionRiskService().Do(context.Background())
	if err != nil {
		return nil, err
	}
	
	positions := make([]types.FuturesPosition, 0)
	for _, risk := range risks {
		if risk.PositionAmt == "0" {
			continue
		}
		
		leverage := int(parseDecimal(risk.Leverage).IntPart())
		position := types.FuturesPosition{
			Symbol:           risk.Symbol,
			PositionSide:     risk.PositionSide,
			MarginType:       risk.MarginType,
			Quantity:         parseDecimal(risk.PositionAmt),
			EntryPrice:       parseDecimal(risk.EntryPrice),
			MarkPrice:        parseDecimal(risk.MarkPrice),
			UnrealizedPnL:    parseDecimal(risk.UnRealizedProfit),
			Margin:           parseDecimal(risk.Notional),
			IsolatedMargin:   parseDecimal(risk.IsolatedMargin),
			Leverage:         int(leverage),
			LiquidationPrice: parseDecimal(risk.LiquidationPrice),
			UpdateTime:       time.Now(), // UpdateTime not available in position risk
		}
		positions = append(positions, position)
	}
	
	return positions, nil
}

// SetLeverage sets the leverage for a symbol
func (bf *BinanceFutures) SetLeverage(symbol string, leverage int) error {
	if !bf.rateLimiter.Allow("set_leverage") {
		return fmt.Errorf("rate limit exceeded")
	}
	
	_, err := bf.client.NewChangeLeverageService().
		Symbol(symbol).
		Leverage(leverage).
		Do(context.Background())
		
	return err
}

// SetMarginType sets the margin type for a symbol
func (bf *BinanceFutures) SetMarginType(symbol string, marginType string) error {
	if !bf.rateLimiter.Allow("set_margin_type") {
		return fmt.Errorf("rate limit exceeded")
	}
	
	svc := bf.client.NewChangeMarginTypeService().Symbol(symbol)
	
	switch marginType {
	case types.MarginTypeCross:
		svc.MarginType(futures.MarginTypeCrossed)
	case types.MarginTypeIsolated:
		svc.MarginType(futures.MarginTypeIsolated)
	default:
		return fmt.Errorf("invalid margin type: %s", marginType)
	}
	
	return svc.Do(context.Background())
}

// CreateOrder creates a new futures order
func (bf *BinanceFutures) CreateOrder(order *types.Order) (*types.OrderResponse, error) {
	if !bf.rateLimiter.Allow("create_order") {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	
	svc := bf.client.NewCreateOrderService().
		Symbol(order.Symbol).
		Side(futures.SideType(order.Side)).
		Type(futures.OrderType(order.Type))
	
	// Set position side if specified
	if order.PositionSide != "" {
		svc.PositionSide(futures.PositionSideType(order.PositionSide))
	}
	
	if order.Type == types.OrderTypeLimit {
		svc.TimeInForce(futures.TimeInForceTypeGTC).
			Price(order.Price.String()).
			Quantity(order.Quantity.String())
	} else if order.Type == types.OrderTypeMarket {
		svc.Quantity(order.Quantity.String())
	}
	
	// Add reduce only if specified
	if order.ReduceOnly {
		svc.ReduceOnly(true)
	}
	
	res, err := svc.Do(context.Background())
	if err != nil {
		return nil, err
	}
	
	response := &types.OrderResponse{
		OrderID:      fmt.Sprintf("%d", res.OrderID),
		ClientID:     res.ClientOrderID,
		Symbol:       res.Symbol,
		Side:         string(res.Side),
		Type:         string(res.Type),
		Status:       string(res.Status),
		Price:        res.Price,
		Quantity:     res.OrigQuantity,
		ExecutedQty:  res.ExecutedQuantity,
		TransactTime: res.UpdateTime,
	}
	
	return response, nil
}

// CancelOrder cancels an existing order
func (bf *BinanceFutures) CancelOrder(symbol, orderID string) error {
	if !bf.rateLimiter.Allow("cancel_order") {
		return fmt.Errorf("rate limit exceeded")
	}
	
	// Try to parse orderID as int64 first
	if orderIDInt, err := strconv.ParseInt(orderID, 10, 64); err == nil {
		_, err = bf.client.NewCancelOrderService().
			Symbol(symbol).
			OrderID(orderIDInt).
			Do(context.Background())
		return err
	}
	
	// If not numeric, try as origClientOrderID
	_, err := bf.client.NewCancelOrderService().
		Symbol(symbol).
		OrigClientOrderID(orderID).
		Do(context.Background())
		
	return err
}

// GetOrder retrieves order information
func (bf *BinanceFutures) GetOrder(symbol, orderID string) (*types.OrderResponse, error) {
	if !bf.rateLimiter.Allow("get_order") {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	
	// Try to parse orderID as int64 first
	var order *futures.Order
	var err error
	
	if orderIDInt, parseErr := strconv.ParseInt(orderID, 10, 64); parseErr == nil {
		order, err = bf.client.NewGetOrderService().
			Symbol(symbol).
			OrderID(orderIDInt).
			Do(context.Background())
	} else {
		// If not numeric, try as origClientOrderID
		order, err = bf.client.NewGetOrderService().
			Symbol(symbol).
			OrigClientOrderID(orderID).
			Do(context.Background())
	}
		
	if err != nil {
		return nil, err
	}
	
	return &types.OrderResponse{
		OrderID:      fmt.Sprintf("%d", order.OrderID),
		ClientID:     order.ClientOrderID,
		Symbol:       order.Symbol,
		Side:         string(order.Side),
		Type:         string(order.Type),
		Status:       string(order.Status),
		Price:        order.Price,
		Quantity:     order.OrigQuantity,
		ExecutedQty:  order.ExecutedQuantity,
		TransactTime: order.UpdateTime,
	}, nil
}

// GetOpenOrders retrieves all open orders
func (bf *BinanceFutures) GetOpenOrders(symbol string) ([]*types.OrderResponse, error) {
	if !bf.rateLimiter.Allow("open_orders") {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	
	svc := bf.client.NewListOpenOrdersService()
	if symbol != "" {
		svc.Symbol(symbol)
	}
	
	orders, err := svc.Do(context.Background())
	if err != nil {
		return nil, err
	}
	
	result := make([]*types.OrderResponse, 0, len(orders))
	for _, order := range orders {
		result = append(result, &types.OrderResponse{
			OrderID:      fmt.Sprintf("%d", order.OrderID),
			ClientID:     order.ClientOrderID,
			Symbol:       order.Symbol,
			Side:         string(order.Side),
			Type:         string(order.Type),
			Status:       string(order.Status),
			Price:        order.Price,
			Quantity:     order.OrigQuantity,
			ExecutedQty:  order.ExecutedQuantity,
			TransactTime: order.UpdateTime,
		})
	}
	
	return result, nil
}

// GetBalance returns futures account balance
func (bf *BinanceFutures) GetBalance() (*types.Balance, error) {
	account, err := bf.GetAccount()
	if err != nil {
		return nil, err
	}
	
	balance := &types.Balance{
		Exchange: "binance",
		Market:   "futures",
		Assets:   make(map[string]types.AssetBalance),
	}
	
	for _, asset := range account.Assets {
		if asset.Balance.IsPositive() {
			balance.Assets[asset.Asset] = types.AssetBalance{
				Asset:  asset.Asset,
				Free:   asset.AvailableBalance.String(),
				Locked: asset.Balance.Sub(asset.AvailableBalance).String(),
			}
		}
	}
	
	return balance, nil
}

// Helper function to parse decimal
func parseDecimal(s string) decimal.Decimal {
	d, _ := decimal.NewFromString(s)
	return d
}

// SetNatsClient sets the NATS client
func (bf *BinanceFutures) SetNatsClient(nc interface{}) {
	bf.natsClient = nc
}

// Close closes the client
func (bf *BinanceFutures) Close() error {
	// Close WebSocket connections
	for _, ws := range bf.wsClient {
		// Close WebSocket handler
		_ = ws
	}
	return nil
}