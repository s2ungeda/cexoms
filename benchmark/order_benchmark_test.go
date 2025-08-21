package benchmark

import (
	"context"
	"testing"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// MockExchange is a mock implementation for benchmarking
type MockExchange struct {
	orderCount int
}

func (m *MockExchange) GetName() string {
	return "mock"
}

func (m *MockExchange) GetAccountType() types.AccountType {
	return types.AccountTypeSpot
}

func (m *MockExchange) IsConnected() bool {
	return true
}

func (m *MockExchange) Connect() error {
	return nil
}

func (m *MockExchange) Disconnect() error {
	return nil
}

func (m *MockExchange) PlaceOrder(ctx context.Context, order *types.Order) (*types.OrderResponse, error) {
	m.orderCount++
	return &types.OrderResponse{
		OrderID: "BENCH_" + order.ClientOrderID,
		Status:  types.OrderStatusFilled,
		FilledQuantity: order.Quantity,
		FilledPrice: order.Price,
	}, nil
}

func (m *MockExchange) CancelOrder(ctx context.Context, symbol, orderID string) error {
	return nil
}

func (m *MockExchange) GetOrder(ctx context.Context, symbol, orderID string) (*types.Order, error) {
	return &types.Order{
		OrderID: orderID,
		Symbol:  symbol,
		Status:  types.OrderStatusFilled,
	}, nil
}

func (m *MockExchange) GetOpenOrders(ctx context.Context, symbol string) ([]*types.Order, error) {
	return []*types.Order{}, nil
}

func (m *MockExchange) GetOrderHistory(ctx context.Context, symbol string, limit int) ([]*types.Order, error) {
	return []*types.Order{}, nil
}

func (m *MockExchange) GetAccountInfo(ctx context.Context) (*types.AccountInfo, error) {
	return &types.AccountInfo{
		Balances: map[string]types.Balance{
			"USDT": {
				Asset:     "USDT",
				Free:      decimal.NewFromInt(10000),
				Locked:    decimal.Zero,
				Available: decimal.NewFromInt(10000),
			},
		},
	}, nil
}

func (m *MockExchange) GetBalances(ctx context.Context) (map[string]types.Balance, error) {
	return map[string]types.Balance{
		"USDT": {
			Asset:     "USDT",
			Free:      decimal.NewFromInt(10000),
			Locked:    decimal.Zero,
			Available: decimal.NewFromInt(10000),
		},
	}, nil
}

func (m *MockExchange) GetTicker(ctx context.Context, symbol string) (*types.Ticker, error) {
	return &types.Ticker{
		Symbol: symbol,
		Price:  decimal.NewFromFloat(40000),
		Volume: decimal.NewFromInt(1000),
	}, nil
}

func (m *MockExchange) GetOrderBook(ctx context.Context, symbol string, depth int) (*types.OrderBook, error) {
	return &types.OrderBook{
		Symbol: symbol,
		Bids: [][2]decimal.Decimal{
			{decimal.NewFromFloat(39999), decimal.NewFromFloat(10)},
			{decimal.NewFromFloat(39998), decimal.NewFromFloat(20)},
		},
		Asks: [][2]decimal.Decimal{
			{decimal.NewFromFloat(40001), decimal.NewFromFloat(10)},
			{decimal.NewFromFloat(40002), decimal.NewFromFloat(20)},
		},
	}, nil
}

func (m *MockExchange) GetKlines(ctx context.Context, symbol, interval string, limit int) ([]*types.Kline, error) {
	return []*types.Kline{}, nil
}

func (m *MockExchange) SubscribeTicker(symbol string) error {
	return nil
}

func (m *MockExchange) SubscribeTrades(symbol string) error {
	return nil
}

func (m *MockExchange) SubscribeOrderBook(symbol string, depth int) error {
	return nil
}

func (m *MockExchange) SubscribeKline(symbol string, interval string) error {
	return nil
}

func (m *MockExchange) UnsubscribeTicker(symbol string) error {
	return nil
}

func (m *MockExchange) UnsubscribeTrades(symbol string) error {
	return nil
}

func (m *MockExchange) UnsubscribeOrderBook(symbol string) error {
	return nil
}

func (m *MockExchange) UnsubscribeKline(symbol string, interval string) error {
	return nil
}

func (m *MockExchange) SetTickerCallback(callback types.TickerCallback) {}
func (m *MockExchange) SetTradeCallback(callback types.TradeCallback) {}
func (m *MockExchange) SetOrderBookCallback(callback types.OrderBookCallback) {}
func (m *MockExchange) SetKlineCallback(callback types.KlineCallback) {}
func (m *MockExchange) SetOrderUpdateCallback(callback types.OrderUpdateCallback) {}
func (m *MockExchange) SetAccountUpdateCallback(callback types.AccountUpdateCallback) {}

func (m *MockExchange) Initialize(ctx context.Context) error {
	return nil
}

// BenchmarkOrderPlacement tests order placement performance
func BenchmarkOrderPlacement(b *testing.B) {
	exchange := &MockExchange{}
	ctx := context.Background()
	
	order := &types.Order{
		Symbol:   "BTCUSDT",
		Side:     types.OrderSideBuy,
		Type:     types.OrderTypeLimit,
		Quantity: decimal.NewFromFloat(0.001),
		Price:    decimal.NewFromFloat(40000),
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		order.ClientOrderID = string(rune(i))
		_, err := exchange.PlaceOrder(ctx, order)
		if err != nil {
			b.Fatal(err)
		}
	}
	
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "orders/sec")
}

// BenchmarkOrderPlacementParallel tests concurrent order placement
func BenchmarkOrderPlacementParallel(b *testing.B) {
	exchange := &MockExchange{}
	ctx := context.Background()
	
	b.RunParallel(func(pb *testing.PB) {
		order := &types.Order{
			Symbol:   "BTCUSDT",
			Side:     types.OrderSideBuy,
			Type:     types.OrderTypeLimit,
			Quantity: decimal.NewFromFloat(0.001),
			Price:    decimal.NewFromFloat(40000),
		}
		
		i := 0
		for pb.Next() {
			order.ClientOrderID = string(rune(i))
			_, err := exchange.PlaceOrder(ctx, order)
			if err != nil {
				b.Fatal(err)
			}
			i++
		}
	})
	
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "orders/sec")
}

// BenchmarkOrderBookUpdate tests order book update performance
func BenchmarkOrderBookUpdate(b *testing.B) {
	exchange := &MockExchange{}
	ctx := context.Background()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_, err := exchange.GetOrderBook(ctx, "BTCUSDT", 20)
		if err != nil {
			b.Fatal(err)
		}
	}
	
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "updates/sec")
}