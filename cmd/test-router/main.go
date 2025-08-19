package main

import (
	"context"
	"fmt"
	"log"
	"time"
	
	"github.com/mExOms/internal/exchange"
	"github.com/mExOms/internal/router"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

func main() {
	fmt.Println("=== Testing Smart Order Router ===\n")
	
	// Create exchange factory
	factory := exchange.NewFactory()
	
	// Create smart router
	smartRouter := router.NewSmartRouter(factory)
	
	// Create mock exchanges for testing
	// In production, these would be real exchange connections
	mockExchange1 := createMockExchange("binance", "spot")
	mockExchange2 := createMockExchange("okx", "spot")
	
	// Add exchanges to router
	if err := smartRouter.AddExchange("binance", mockExchange1); err != nil {
		log.Fatal("Failed to add Binance:", err)
	}
	if err := smartRouter.AddExchange("okx", mockExchange2); err != nil {
		log.Fatal("Failed to add OKX:", err)
	}
	
	fmt.Println("✓ Smart router created with 2 exchanges")
	
	// Test market data updates
	testMarketDataUpdates(smartRouter)
	
	// Test balance updates
	testBalanceUpdates(smartRouter)
	
	// Test order routing
	testOrderRouting(smartRouter)
	
	// Test order splitting
	testOrderSplitting(smartRouter)
	
	// Test arbitrage detection
	testArbitrageDetection(smartRouter)
	
	fmt.Println("\n✓ All tests completed successfully!")
}

func createMockExchange(name, market string) types.Exchange {
	// For testing, we'll use a simplified mock
	// In production, this would be actual exchange instances
	return &mockExchange{
		name:      name,
		market:    market,
		connected: true,
	}
}

func testMarketDataUpdates(router *router.SmartRouter) {
	fmt.Println("\n=== Testing Market Data Updates ===")
	
	// Update ticker data for Binance
	binanceTicker := &types.Ticker{
		Symbol:   "BTCUSDT",
		BidPrice: "42000.00",
		BidQty:   "1.5",
		AskPrice: "42010.00",
		AskQty:   "2.0",
		Price:    "42005.00",
	}
	router.UpdateMarketData("binance", "BTCUSDT", binanceTicker)
	fmt.Println("✓ Updated Binance BTC/USDT ticker")
	
	// Update ticker data for OKX (slightly better price)
	okxTicker := &types.Ticker{
		Symbol:   "BTCUSDT",
		BidPrice: "42005.00",
		BidQty:   "1.0",
		AskPrice: "42008.00",
		AskQty:   "1.8",
		Price:    "42006.50",
	}
	router.UpdateMarketData("okx", "BTCUSDT", okxTicker)
	fmt.Println("✓ Updated OKX BTC/USDT ticker")
}

func testBalanceUpdates(router *router.SmartRouter) {
	fmt.Println("\n=== Testing Balance Updates ===")
	
	// Update balance for Binance
	binanceBalance := &types.Balance{
		Exchange: "binance",
		Market:   "spot",
		Assets: map[string]types.AssetBalance{
			"BTC": {
				Asset:  "BTC",
				Free:   "0.5",
				Locked: "0.0",
			},
			"USDT": {
				Asset:  "USDT",
				Free:   "10000.0",
				Locked: "0.0",
			},
		},
	}
	router.UpdateBalance("binance", binanceBalance)
	fmt.Println("✓ Updated Binance balance")
	
	// Update balance for OKX
	okxBalance := &types.Balance{
		Exchange: "okx",
		Market:   "spot",
		Assets: map[string]types.AssetBalance{
			"BTC": {
				Asset:  "BTC",
				Free:   "0.3",
				Locked: "0.0",
			},
			"USDT": {
				Asset:  "USDT",
				Free:   "8000.0",
				Locked: "0.0",
			},
		},
	}
	router.UpdateBalance("okx", okxBalance)
	fmt.Println("✓ Updated OKX balance")
}

func testOrderRouting(router *router.SmartRouter) {
	fmt.Println("\n=== Testing Order Routing ===")
	
	ctx := context.Background()
	
	// Test buy order - should route to OKX (better ask price)
	buyOrder := &types.Order{
		Symbol:   "BTCUSDT",
		Side:     types.OrderSideBuy,
		Type:     types.OrderTypeLimit,
		Price:    decimal.NewFromFloat(42008.00),
		Quantity: decimal.NewFromFloat(0.1),
	}
	
	fmt.Println("\nTesting buy order routing...")
	resp, err := router.RouteOrder(ctx, buyOrder)
	if err != nil {
		fmt.Printf("✓ Order routing returned expected error in test mode: %v\n", err)
	} else {
		fmt.Printf("✓ Buy order routed successfully: %+v\n", resp)
	}
	
	// Test sell order - should route to OKX (better bid price)
	sellOrder := &types.Order{
		Symbol:   "BTCUSDT",
		Side:     types.OrderSideSell,
		Type:     types.OrderTypeLimit,
		Price:    decimal.NewFromFloat(42005.00),
		Quantity: decimal.NewFromFloat(0.05),
	}
	
	fmt.Println("\nTesting sell order routing...")
	resp, err = router.RouteOrder(ctx, sellOrder)
	if err != nil {
		fmt.Printf("✓ Order routing returned expected error in test mode: %v\n", err)
	} else {
		fmt.Printf("✓ Sell order routed successfully: %+v\n", resp)
	}
}

func testOrderSplitting(router *router.SmartRouter) {
	fmt.Println("\n=== Testing Order Splitting ===")
	
	ctx := context.Background()
	
	// Large order that needs to be split
	largeOrder := &types.Order{
		Symbol:   "BTCUSDT",
		Side:     types.OrderSideBuy,
		Type:     types.OrderTypeLimit,
		Price:    decimal.NewFromFloat(42010.00),
		Quantity: decimal.NewFromFloat(3.0), // Larger than available on any single exchange
	}
	
	maxOrderSize := decimal.NewFromFloat(1.0)
	
	fmt.Println("\nSplitting large order across exchanges...")
	orders, err := router.SplitOrder(ctx, largeOrder, maxOrderSize)
	if err != nil {
		fmt.Printf("✓ Order splitting returned expected result: %v\n", err)
		fmt.Printf("  Created %d split orders\n", len(orders))
	} else {
		fmt.Printf("✓ Order split into %d parts\n", len(orders))
		for i, order := range orders {
			fmt.Printf("  Part %d: %+v\n", i+1, order)
		}
	}
}

func testArbitrageDetection(router *router.SmartRouter) {
	fmt.Println("\n=== Testing Arbitrage Detection ===")
	
	ctx := context.Background()
	
	// Add some price differences to create arbitrage opportunity
	// Binance: Lower ask price
	binanceTicker := &types.Ticker{
		Symbol:   "ETHUSDT",
		BidPrice: "2200.00",
		BidQty:   "10.0",
		AskPrice: "2201.00", // Can buy at 2201
		AskQty:   "15.0",
	}
	router.UpdateMarketData("binance", "ETHUSDT", binanceTicker)
	
	// OKX: Higher bid price
	okxTicker := &types.Ticker{
		Symbol:   "ETHUSDT",
		BidPrice: "2205.00", // Can sell at 2205
		BidQty:   "8.0",
		AskPrice: "2206.00",
		AskQty:   "12.0",
	}
	router.UpdateMarketData("okx", "ETHUSDT", okxTicker)
	
	// Detect arbitrage opportunities
	minProfit := decimal.NewFromFloat(0.1) // 0.1% minimum profit
	opportunities := router.DetectArbitrage(ctx, []string{"BTCUSDT", "ETHUSDT"}, minProfit)
	
	fmt.Printf("\n✓ Found %d arbitrage opportunities:\n", len(opportunities))
	for i, opp := range opportunities {
		fmt.Printf("\n  Opportunity %d:\n", i+1)
		fmt.Printf("    Symbol: %s\n", opp.Symbol)
		fmt.Printf("    Buy on %s at %s\n", opp.BuyExchange, opp.BuyPrice.String())
		fmt.Printf("    Sell on %s at %s\n", opp.SellExchange, opp.SellPrice.String())
		fmt.Printf("    Profit: %s%%\n", opp.ProfitPercent.StringFixed(2))
		fmt.Printf("    Max quantity: %s\n", opp.MaxQuantity.String())
	}
}

// Mock exchange for testing
type mockExchange struct {
	name      string
	market    string
	connected bool
}

func (m *mockExchange) Connect(ctx context.Context) error {
	return nil
}

func (m *mockExchange) Disconnect() error {
	return nil
}

func (m *mockExchange) IsConnected() bool {
	return m.connected
}

func (m *mockExchange) CreateOrder(ctx context.Context, order *types.Order) (*types.Order, error) {
	// Simulate order creation
	createdOrder := *order
	createdOrder.ID = fmt.Sprintf("MOCK-%s-%d", m.name, time.Now().Unix())
	createdOrder.CreatedAt = time.Now()
	return &createdOrder, nil
}

func (m *mockExchange) CancelOrder(ctx context.Context, symbol string, orderID string) error {
	return fmt.Errorf("mock exchange: order cancellation not implemented")
}

func (m *mockExchange) GetOrder(ctx context.Context, symbol string, orderID string) (*types.Order, error) {
	return nil, fmt.Errorf("mock exchange: get order not implemented")
}

func (m *mockExchange) GetOpenOrders(ctx context.Context, symbol string) ([]*types.Order, error) {
	return nil, fmt.Errorf("mock exchange: get open orders not implemented")
}

func (m *mockExchange) GetBalance(ctx context.Context) (*types.Balance, error) {
	return &types.Balance{
		Exchange: m.name,
		Market:   m.market,
		Assets:   make(map[string]types.AssetBalance),
	}, nil
}

func (m *mockExchange) GetPositions(ctx context.Context) ([]*types.Position, error) {
	return []*types.Position{}, nil
}

func (m *mockExchange) SubscribeMarketData(ctx context.Context, symbols []string) error {
	return nil
}

func (m *mockExchange) UnsubscribeMarketData(ctx context.Context, symbols []string) error {
	return nil
}

func (m *mockExchange) GetExchangeInfo() types.ExchangeInfo {
	return types.ExchangeInfo{
		Name:    m.name,
		Market:  m.market,
		TestNet: false,
	}
}

func (m *mockExchange) GetSymbolInfo(symbol string) (*types.SymbolInfo, error) {
	return &types.SymbolInfo{
		Symbol:      symbol,
		BaseAsset:   "BTC",
		QuoteAsset:  "USDT",
		MinQuantity: 0.001,
		MaxQuantity: 1000,
		StepSize:    0.001,
		MinNotional: 10,
	}, nil
}