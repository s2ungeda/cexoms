package spot

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/adshao/go-binance/v2"
	"github.com/mExOms/pkg/cache"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

type BinanceSpot struct {
	client       *binance.Client
	wsClient     map[string]interface{}
	cache        *cache.MemoryCache
	rateLimiter  *cache.RateLimiter
	natsClient   interface{} // Will be set to actual NATS client later
	apiKey       string
	apiSecret    string
	testnet      bool
}

func NewBinanceSpot(apiKey, apiSecret string, testnet bool) (*BinanceSpot, error) {
	var client *binance.Client
	
	if testnet {
		client = binance.NewClient(apiKey, apiSecret)
		client.BaseURL = "https://testnet.binance.vision/api"
	} else {
		client = binance.NewClient(apiKey, apiSecret)
	}
	
	bs := &BinanceSpot{
		client:      client,
		wsClient:    make(map[string]interface{}),
		cache:       cache.NewMemoryCache(),
		rateLimiter: cache.NewRateLimiter(1200, time.Minute), // Binance limit
		apiKey:      apiKey,
		apiSecret:   apiSecret,
		testnet:     testnet,
	}
	
	return bs, nil
}

func (bs *BinanceSpot) GetExchangeInfo() (*types.ExchangeInfo, error) {
	if !bs.rateLimiter.Allow("exchange_info") {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	
	// Check cache first
	if cached, exists := bs.cache.Get("exchange_info"); exists {
		return cached.(*types.ExchangeInfo), nil
	}
	
	info, err := bs.client.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		return nil, err
	}
	
	exchangeInfo := &types.ExchangeInfo{
		Exchange: "binance",
		Market:   "spot",
		Symbols:  make([]types.Symbol, 0, len(info.Symbols)),
	}
	
	for _, s := range info.Symbols {
		if s.Status != "TRADING" {
			continue
		}
		
		symbol := types.Symbol{
			Symbol:     s.Symbol,
			Base:       s.BaseAsset,
			Quote:      s.QuoteAsset,
			MinQty:     s.LotSizeFilter().MinQuantity,
			MaxQty:     s.LotSizeFilter().MaxQuantity,
			StepSize:   s.LotSizeFilter().StepSize,
			MinNotional: "10.0", // Default min notional
			Status:     s.Status,
		}
		exchangeInfo.Symbols = append(exchangeInfo.Symbols, symbol)
	}
	
	// Cache for 1 hour
	bs.cache.Set("exchange_info", exchangeInfo, time.Hour)
	
	return exchangeInfo, nil
}

func (bs *BinanceSpot) CreateOrder(order *types.Order) (*types.OrderResponse, error) {
	if !bs.rateLimiter.Allow("create_order") {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	
	svc := bs.client.NewCreateOrderService().
		Symbol(order.Symbol).
		Side(binance.SideType(order.Side)).
		Type(binance.OrderType(order.Type))
	
	if order.Type == types.OrderTypeLimit {
		svc.TimeInForce(binance.TimeInForceTypeGTC).
			Price(order.Price.String()).
			Quantity(order.Quantity.String())
	} else if order.Type == types.OrderTypeMarket {
		svc.Quantity(order.Quantity.String())
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
		TransactTime: res.TransactTime,
	}
	
	// TODO: Publish to NATS when natsClient is implemented
	// if bs.natsClient != nil {
	// 	subject := fmt.Sprintf("orders.binance.spot.%s", order.Symbol)
	// 	bs.natsClient.PublishOrder(subject, response)
	// }
	
	return response, nil
}

func (bs *BinanceSpot) CancelOrder(ctx context.Context, symbol, orderID string) error {
	if !bs.rateLimiter.Allow("cancel_order") {
		return fmt.Errorf("rate limit exceeded")
	}
	
	// Try to parse orderID as int64 first
	if orderIDInt, parseErr := strconv.ParseInt(orderID, 10, 64); parseErr == nil {
		_, err := bs.client.NewCancelOrderService().
			Symbol(symbol).
			OrderID(orderIDInt).
			Do(context.Background())
		return err
	}
	
	// If not numeric, try as origClientOrderID
	_, err := bs.client.NewCancelOrderService().
		Symbol(symbol).
		OrigClientOrderID(orderID).
		Do(context.Background())
	
	return err
}

func (bs *BinanceSpot) GetOrder(ctx context.Context, symbol, orderID string) (*types.Order, error) {
	if !bs.rateLimiter.Allow("get_order") {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	
	// Try to parse orderID as int64 first
	var order *binance.Order
	var err error
	
	if orderIDInt, parseErr := strconv.ParseInt(orderID, 10, 64); parseErr == nil {
		order, err = bs.client.NewGetOrderService().
			Symbol(symbol).
			OrderID(orderIDInt).
			Do(context.Background())
	} else {
		// If not numeric, try as origClientOrderID
		order, err = bs.client.NewGetOrderService().
			Symbol(symbol).
			OrigClientOrderID(orderID).
			Do(context.Background())
	}
	
	if err != nil {
		return nil, err
	}
	
	// Convert to types.Order
	return &types.Order{
		ID:          fmt.Sprintf("%d", order.OrderID),
		Symbol:      order.Symbol,
		Side:        string(order.Side),
		Type:        string(order.Type),
		Price:       decimal.RequireFromString(order.Price),
		Quantity:    decimal.RequireFromString(order.OrigQuantity),
		TimeInForce: string(order.TimeInForce),
		CreatedAt:   time.Unix(order.Time/1000, (order.Time%1000)*1000000),
	}, nil
}

func (bs *BinanceSpot) GetOpenOrders(ctx context.Context, symbol string) ([]*types.Order, error) {
	if !bs.rateLimiter.Allow("open_orders") {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	
	svc := bs.client.NewListOpenOrdersService()
	if symbol != "" {
		svc.Symbol(symbol)
	}
	
	orders, err := svc.Do(context.Background())
	if err != nil {
		return nil, err
	}
	
	result := make([]*types.Order, 0, len(orders))
	for _, order := range orders {
		result = append(result, &types.Order{
			ID:          fmt.Sprintf("%d", order.OrderID),
			Symbol:      order.Symbol,
			Side:        string(order.Side),
			Type:        string(order.Type),
			Price:       decimal.RequireFromString(order.Price),
			Quantity:    decimal.RequireFromString(order.OrigQuantity),
			TimeInForce: string(order.TimeInForce),
			CreatedAt:   time.Unix(order.Time/1000, (order.Time%1000)*1000000),
		})
	}
	
	return result, nil
}

func (bs *BinanceSpot) GetBalance(ctx context.Context) (*types.Balance, error) {
	if !bs.rateLimiter.Allow("balance") {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	
	account, err := bs.client.NewGetAccountService().Do(context.Background())
	if err != nil {
		return nil, err
	}
	
	balance := &types.Balance{
		Exchange: "binance",
		Market:   "spot",
		Assets:   make(map[string]types.AssetBalance),
	}
	
	for _, b := range account.Balances {
		if b.Free != "0" || b.Locked != "0" {
			balance.Assets[b.Asset] = types.AssetBalance{
				Asset:  b.Asset,
				Free:   b.Free,
				Locked: b.Locked,
			}
		}
	}
	
	// Cache balance for 5 seconds
	bs.cache.Set("balance", balance, 5*time.Second)
	
	return balance, nil
}

func (bs *BinanceSpot) SetNatsClient(nc interface{}) {
	bs.natsClient = nc
}

func (bs *BinanceSpot) Close() error {
	// Close WebSocket connections
	for _, ws := range bs.wsClient {
		if ws != nil {
			// Close WebSocket handler
			// Note: go-binance doesn't provide direct close method
		}
	}
	return nil
}

// Connect establishes connection to Binance
func (bs *BinanceSpot) Connect(ctx context.Context) error {
	// Binance client is already connected when created
	return nil
}

// Disconnect closes the connection
func (bs *BinanceSpot) Disconnect() error {
	return bs.Close()
}

// GetPositions returns positions (spot trading doesn't have positions)
func (bs *BinanceSpot) GetPositions(ctx context.Context) ([]*types.Position, error) {
	// Spot trading doesn't have positions, only balances
	return []*types.Position{}, nil
}

// SubscribeMarketData subscribes to market data streams
func (bs *BinanceSpot) SubscribeMarketData(ctx context.Context, symbols []string) error {
	// Implementation would subscribe to WebSocket streams
	// For now, return nil
	return nil
}

// UnsubscribeMarketData unsubscribes from market data streams
func (bs *BinanceSpot) UnsubscribeMarketData(ctx context.Context, symbols []string) error {
	// Implementation would unsubscribe from WebSocket streams
	// For now, return nil
	return nil
}

// GetSymbolInfo returns symbol trading information
func (bs *BinanceSpot) GetSymbolInfo(symbol string) (*types.SymbolInfo, error) {
	info, err := bs.GetExchangeInfo()
	if err != nil {
		return nil, err
	}
	
	for _, s := range info.Symbols {
		if s.Symbol == symbol {
			return &types.SymbolInfo{
				Symbol:              s.Symbol,
				BaseAsset:           s.Base,
				QuoteAsset:          s.Quote,
				Status:              s.Status,
				MinQuantity:         parseFloat64(s.MinQty),
				MaxQuantity:         parseFloat64(s.MaxQty),
				StepSize:            parseFloat64(s.StepSize),
				MinNotional:         parseFloat64(s.MinNotional),
				IsSpotTradingAllowed: true,
				IsMarginTradingAllowed: false,
			}, nil
		}
	}
	
	return nil, fmt.Errorf("symbol %s not found", symbol)
}

// parseFloat64 safely parses string to float64
func parseFloat64(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}