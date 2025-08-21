package market_maker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock exchange for testing
type mockExchange struct {
	mu              sync.Mutex
	orders          map[string]*types.Order
	orderID         int
	executedOrders  []*types.Order
	balance         map[string]float64
	positions       map[string]*types.Position
	orderBook       *types.OrderBook
	accountType     types.AccountType
	accountID       string
	executeCallback func(order *types.Order)
}

func newMockExchange(accountID string, accountType types.AccountType) *mockExchange {
	return &mockExchange{
		orders:      make(map[string]*types.Order),
		balance:     map[string]float64{"USDT": 10000, "BTC": 1.0},
		positions:   make(map[string]*types.Position),
		accountID:   accountID,
		accountType: accountType,
		orderBook: &types.OrderBook{
			Symbol: "BTCUSDT",
			Bids: []types.OrderBookLevel{
				{Price: 49995, Quantity: 10},
				{Price: 49990, Quantity: 20},
			},
			Asks: []types.OrderBookLevel{
				{Price: 50005, Quantity: 10},
				{Price: 50010, Quantity: 20},
			},
			Timestamp: time.Now(),
		},
	}
}

func (m *mockExchange) GetName() string { return "mock" }

func (m *mockExchange) GetAccountType() types.AccountType {
	return m.accountType
}

func (m *mockExchange) GetAccountID() string {
	return m.accountID
}

func (m *mockExchange) CreateOrder(ctx context.Context, order *types.Order) (*types.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.orderID++
	order.ID = string(rune(m.orderID))
	order.Status = types.OrderStatusNew
	order.CreatedAt = time.Now()
	m.orders[order.ID] = order
	return order, nil
}

func (m *mockExchange) CancelOrder(ctx context.Context, orderID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if order, exists := m.orders[orderID]; exists {
		order.Status = types.OrderStatusCanceled
		delete(m.orders, orderID)
	}
	return nil
}

func (m *mockExchange) GetOpenOrders(ctx context.Context, symbol string) ([]*types.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var orders []*types.Order
	for _, order := range m.orders {
		if order.Symbol == symbol && order.Status == types.OrderStatusNew {
			orders = append(orders, order)
		}
	}
	return orders, nil
}

func (m *mockExchange) GetBalance(ctx context.Context, asset string) (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if balance, exists := m.balance[asset]; exists {
		return balance, nil
	}
	return 0, nil
}

func (m *mockExchange) GetPosition(ctx context.Context, symbol string) (*types.Position, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if pos, exists := m.positions[symbol]; exists {
		return pos, nil
	}
	return &types.Position{
		Symbol:   symbol,
		Quantity: 0,
		AvgPrice: 0,
	}, nil
}

func (m *mockExchange) GetOrderBook(ctx context.Context, symbol string, depth int) (*types.OrderBook, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.orderBook, nil
}

func (m *mockExchange) GetTrades(ctx context.Context, symbol string, limit int) ([]*types.Trade, error) {
	return nil, nil
}

func (m *mockExchange) SubscribeMarketData(ctx context.Context, symbols []string, callback func(*types.MarketUpdate)) error {
	// Send periodic market updates
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.mu.Lock()
				update := &types.MarketUpdate{
					Symbol:    "BTCUSDT",
					BidPrice:  m.orderBook.Bids[0].Price,
					BidSize:   m.orderBook.Bids[0].Quantity,
					AskPrice:  m.orderBook.Asks[0].Price,
					AskSize:   m.orderBook.Asks[0].Quantity,
					LastPrice: (m.orderBook.Bids[0].Price + m.orderBook.Asks[0].Price) / 2,
					Timestamp: time.Now(),
				}
				m.mu.Unlock()
				callback(update)
			}
		}
	}()
	return nil
}

func (m *mockExchange) simulateOrderExecution(order *types.Order) {
	m.mu.Lock()
	defer m.mu.Unlock()

	order.Status = types.OrderStatusFilled
	order.FilledQuantity = order.Quantity
	order.UpdatedAt = time.Now()
	m.executedOrders = append(m.executedOrders, order)

	// Update position
	pos, exists := m.positions[order.Symbol]
	if !exists {
		pos = &types.Position{
			Symbol:   order.Symbol,
			Quantity: 0,
			AvgPrice: 0,
		}
		m.positions[order.Symbol] = pos
	}

	if order.Side == types.SideBuy {
		totalCost := pos.Quantity*pos.AvgPrice + order.Quantity*order.Price
		pos.Quantity += order.Quantity
		if pos.Quantity > 0 {
			pos.AvgPrice = totalCost / pos.Quantity
		}
	} else {
		pos.Quantity -= order.Quantity
		if pos.Quantity < 0 {
			pos.AvgPrice = order.Price
		}
	}

	// Update balance
	if order.Side == types.SideBuy {
		m.balance["USDT"] -= order.Quantity * order.Price
		m.balance["BTC"] += order.Quantity
	} else {
		m.balance["USDT"] += order.Quantity * order.Price
		m.balance["BTC"] -= order.Quantity
	}

	if m.executeCallback != nil {
		m.executeCallback(order)
	}
}

func TestMarketMaker_BasicFunctionality(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create mock exchange
	exchange := newMockExchange("test-account", types.AccountTypeSub)

	// Create config
	config := Config{
		Symbol:          "BTCUSDT",
		BaseSpreadBps:   10,
		MinSpreadBps:    5,
		MaxSpreadBps:    50,
		QuoteSize:       0.01,
		QuoteLevels:     3,
		LevelSpacingBps: 2,
		MaxInventory:    1.0,
		InventorySkew:   0.5,
		UpdateInterval:  100 * time.Millisecond,
		RiskLimits: RiskLimits{
			MaxPositionValue: 100000,
			StopLossPercent:  0.02,
			MaxDailyLoss:     1000,
		},
	}

	// Create market maker
	mm := New(exchange, config)
	require.NotNil(t, mm)

	// Start market maker
	err := mm.Start(ctx)
	require.NoError(t, err)

	// Wait for quotes to be generated
	time.Sleep(200 * time.Millisecond)

	// Check that quotes were placed
	orders, err := exchange.GetOpenOrders(ctx, "BTCUSDT")
	require.NoError(t, err)
	assert.Greater(t, len(orders), 0)

	// Verify quotes structure
	var bidCount, askCount int
	for _, order := range orders {
		if order.Side == types.SideBuy {
			bidCount++
		} else {
			askCount++
		}
	}

	// Should have equal bid and ask quotes
	assert.Equal(t, bidCount, askCount)
	assert.Equal(t, config.QuoteLevels, bidCount)

	// Stop market maker
	mm.Stop()
}

func TestMarketMaker_SpreadCalculation(t *testing.T) {
	config := Config{
		BaseSpreadBps:    10,
		MinSpreadBps:     5,
		MaxSpreadBps:     50,
		VolatilityFactor: 1.0,
	}

	calc := &spreadCalculator{config: config}

	tests := []struct {
		name       string
		volatility float64
		inventory  float64
		maxInv     float64
		depth      float64
		minSpread  float64
		maxSpread  float64
	}{
		{
			name:       "Normal conditions",
			volatility: 0.01,
			inventory:  0,
			maxInv:     10,
			depth:      100,
			minSpread:  0.0005,
			maxSpread:  0.005,
		},
		{
			name:       "High volatility",
			volatility: 0.05,
			inventory:  0,
			maxInv:     10,
			depth:      100,
			minSpread:  0.001,
			maxSpread:  0.005,
		},
		{
			name:       "High inventory",
			volatility: 0.01,
			inventory:  8,
			maxInv:     10,
			depth:      100,
			minSpread:  0.0005,
			maxSpread:  0.005,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spread := calc.calculate(tt.volatility, tt.inventory/tt.maxInv, tt.depth)
			assert.GreaterOrEqual(t, spread, tt.minSpread)
			assert.LessOrEqual(t, spread, tt.maxSpread)
		})
	}
}

func TestMarketMaker_InventoryManagement(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exchange := newMockExchange("test-account", types.AccountTypeSub)

	config := Config{
		Symbol:          "BTCUSDT",
		BaseSpreadBps:   10,
		QuoteSize:       0.01,
		QuoteLevels:     1,
		MaxInventory:    0.1,
		InventorySkew:   1.0,
		UpdateInterval:  100 * time.Millisecond,
		RiskLimits: RiskLimits{
			MaxPositionValue: 10000,
		},
	}

	mm := New(exchange, config)

	// Set initial position (long)
	exchange.positions["BTCUSDT"] = &types.Position{
		Symbol:   "BTCUSDT",
		Quantity: 0.08, // 80% of max inventory
		AvgPrice: 50000,
	}

	err := mm.Start(ctx)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	// Check quotes - should skew towards selling
	orders, err := exchange.GetOpenOrders(ctx, "BTCUSDT")
	require.NoError(t, err)

	var bidPrices, askPrices []float64
	for _, order := range orders {
		if order.Side == types.SideBuy {
			bidPrices = append(bidPrices, order.Price)
		} else {
			askPrices = append(askPrices, order.Price)
		}
	}

	// With high long inventory, asks should be more aggressive
	if len(bidPrices) > 0 && len(askPrices) > 0 {
		midPrice := 50000.0
		bidDist := midPrice - bidPrices[0]
		askDist := askPrices[0] - midPrice

		// Ask spread should be tighter than bid spread
		assert.Less(t, askDist, bidDist)
	}

	mm.Stop()
}

func TestMarketMaker_RiskManagement(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exchange := newMockExchange("test-account", types.AccountTypeSub)

	config := Config{
		Symbol:         "BTCUSDT",
		BaseSpreadBps:  10,
		QuoteSize:      0.01,
		QuoteLevels:    1,
		MaxInventory:   1.0,
		UpdateInterval: 100 * time.Millisecond,
		RiskLimits: RiskLimits{
			MaxPositionValue: 5000,
			StopLossPercent:  0.02,
			MaxDailyLoss:     100,
		},
	}

	mm := New(exchange, config)

	// Set position that exceeds risk limits
	exchange.positions["BTCUSDT"] = &types.Position{
		Symbol:   "BTCUSDT",
		Quantity: 0.2,
		AvgPrice: 50000, // Position value = 10,000 > MaxPositionValue
	}

	err := mm.Start(ctx)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	// Should not place buy orders due to position limit
	orders, err := exchange.GetOpenOrders(ctx, "BTCUSDT")
	require.NoError(t, err)

	buyOrderCount := 0
	for _, order := range orders {
		if order.Side == types.SideBuy {
			buyOrderCount++
		}
	}

	assert.Equal(t, 0, buyOrderCount, "Should not place buy orders when position exceeds limits")

	mm.Stop()
}

func TestMarketMaker_OrderExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exchange := newMockExchange("test-account", types.AccountTypeSub)

	config := Config{
		Symbol:         "BTCUSDT",
		BaseSpreadBps:  10,
		QuoteSize:      0.01,
		QuoteLevels:    1,
		MaxInventory:   1.0,
		UpdateInterval: 100 * time.Millisecond,
	}

	mm := New(exchange, config)

	// Track order executions
	var executedOrders []*types.Order
	var mu sync.Mutex

	exchange.executeCallback = func(order *types.Order) {
		mu.Lock()
		executedOrders = append(executedOrders, order)
		mu.Unlock()
	}

	err := mm.Start(ctx)
	require.NoError(t, err)

	// Wait for initial quotes
	time.Sleep(200 * time.Millisecond)

	// Simulate order execution
	orders, _ := exchange.GetOpenOrders(ctx, "BTCUSDT")
	if len(orders) > 0 {
		exchange.simulateOrderExecution(orders[0])
	}

	// Wait for market maker to respond
	time.Sleep(300 * time.Millisecond)

	// Check new quotes were placed
	newOrders, err := exchange.GetOpenOrders(ctx, "BTCUSDT")
	require.NoError(t, err)
	assert.Greater(t, len(newOrders), 0, "Should place new quotes after execution")

	mm.Stop()
}

func TestMarketMaker_ConcurrentOperations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exchange := newMockExchange("test-account", types.AccountTypeSub)

	config := Config{
		Symbol:         "BTCUSDT",
		BaseSpreadBps:  10,
		QuoteSize:      0.01,
		QuoteLevels:    3,
		MaxInventory:   1.0,
		UpdateInterval: 50 * time.Millisecond,
	}

	mm := New(exchange, config)

	err := mm.Start(ctx)
	require.NoError(t, err)

	// Run concurrent operations
	var wg sync.WaitGroup

	// Goroutine 1: Simulate market updates
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			exchange.mu.Lock()
			exchange.orderBook.Bids[0].Price = 50000 + float64(i*10)
			exchange.orderBook.Asks[0].Price = 50010 + float64(i*10)
			exchange.mu.Unlock()
			time.Sleep(50 * time.Millisecond)
		}
	}()

	// Goroutine 2: Simulate order executions
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			time.Sleep(100 * time.Millisecond)
			orders, _ := exchange.GetOpenOrders(ctx, "BTCUSDT")
			if len(orders) > 0 {
				exchange.simulateOrderExecution(orders[0])
			}
		}
	}()

	// Wait for operations to complete
	wg.Wait()

	// Final checks
	orders, err := exchange.GetOpenOrders(ctx, "BTCUSDT")
	require.NoError(t, err)
	assert.Greater(t, len(orders), 0, "Market maker should maintain quotes")

	mm.Stop()
}

// Benchmark tests
func BenchmarkMarketMaker_QuoteGeneration(b *testing.B) {
	ctx := context.Background()
	exchange := newMockExchange("test-account", types.AccountTypeSub)

	config := Config{
		Symbol:         "BTCUSDT",
		BaseSpreadBps:  10,
		QuoteSize:      0.01,
		QuoteLevels:    5,
		MaxInventory:   1.0,
		UpdateInterval: 10 * time.Millisecond,
	}

	mm := New(exchange, config)
	mm.Start(ctx)
	defer mm.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mm.generateQuotes(ctx)
	}
}

func BenchmarkSpreadCalculator(b *testing.B) {
	config := Config{
		BaseSpreadBps:    10,
		MinSpreadBps:     5,
		MaxSpreadBps:     50,
		VolatilityFactor: 1.0,
	}

	calc := &spreadCalculator{config: config}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		calc.calculate(0.02, 0.5, 100)
	}
}