package router

import (
	"testing"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestOrderSplitter_SplitFixed(t *testing.T) {
	splitter := NewOrderSplitter(nil)
	
	order := &types.Order{
		ClientOrderID: "test-order-1",
		Symbol:        "BTCUSDT",
		Side:          types.OrderSideBuy,
		Quantity:      decimal.NewFromInt(1000),
		Price:         decimal.NewFromInt(40000),
	}
	
	// Test fixed split
	slices, err := splitter.SplitFixed(order, decimal.NewFromInt(200))
	assert.NoError(t, err)
	assert.Equal(t, 5, len(slices))
	
	// Verify each slice
	for i, slice := range slices {
		assert.Equal(t, decimal.NewFromInt(200), slice.Quantity)
		assert.Equal(t, i+1, slice.Priority)
	}
	
	// Test with remainder
	order.Quantity = decimal.NewFromInt(1050)
	slices, err = splitter.SplitFixed(order, decimal.NewFromInt(200))
	assert.NoError(t, err)
	assert.Equal(t, 5, len(slices))
	assert.Equal(t, decimal.NewFromInt(250), slices[4].Quantity) // Last slice gets remainder
}

func TestOrderSplitter_SplitPercentage(t *testing.T) {
	splitter := NewOrderSplitter(nil)
	
	order := &types.Order{
		ClientOrderID: "test-order-2",
		Symbol:        "BTCUSDT",
		Side:          types.OrderSideSell,
		Quantity:      decimal.NewFromInt(1000),
		Price:         decimal.NewFromInt(40000),
	}
	
	// Test percentage split
	percentages := []float64{50, 30, 20}
	slices, err := splitter.SplitPercentage(order, percentages)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(slices))
	
	assert.Equal(t, decimal.NewFromInt(500), slices[0].Quantity)
	assert.Equal(t, decimal.NewFromInt(300), slices[1].Quantity)
	assert.Equal(t, decimal.NewFromInt(200), slices[2].Quantity)
	
	// Test invalid percentages
	invalidPercentages := []float64{50, 30, 10} // Sum != 100
	_, err = splitter.SplitPercentage(order, invalidPercentages)
	assert.Error(t, err)
}

func TestOrderSplitter_SplitByLiquidity(t *testing.T) {
	splitter := NewOrderSplitter(nil)
	
	order := &types.Order{
		ClientOrderID: "test-order-3",
		Symbol:        "ETHUSDT",
		Side:          types.OrderSideBuy,
		Quantity:      decimal.NewFromInt(100),
		Price:         decimal.NewFromInt(2500),
	}
	
	// Test liquidity-based split
	liquidityMap := map[string]decimal.Decimal{
		"binance": decimal.NewFromInt(60),
		"okx":     decimal.NewFromInt(30),
		"bybit":   decimal.NewFromInt(20),
	}
	
	slices, err := splitter.SplitByLiquidity(order, liquidityMap)
	assert.NoError(t, err)
	assert.True(t, len(slices) > 0)
	
	// Verify total quantity
	totalQty := decimal.Zero
	for _, slice := range slices {
		totalQty = totalQty.Add(slice.Quantity)
	}
	assert.True(t, totalQty.Equal(order.Quantity))
}

func TestOrderSplitter_SplitTWAP(t *testing.T) {
	splitter := NewOrderSplitter(nil)
	
	order := &types.Order{
		ClientOrderID: "test-order-4",
		Symbol:        "BTCUSDT",
		Side:          types.OrderSideBuy,
		Quantity:      decimal.NewFromInt(1000),
		Price:         decimal.NewFromInt(40000),
	}
	
	// Test TWAP split
	duration := 10 * time.Minute
	intervals := 5
	
	slices, err := splitter.SplitTWAP(order, duration, intervals)
	assert.NoError(t, err)
	assert.Equal(t, intervals, len(slices))
	
	// Verify timing
	for i := 1; i < len(slices); i++ {
		timeDiff := slices[i].ExecuteAt.Sub(slices[i-1].ExecuteAt)
		assert.Equal(t, duration/time.Duration(intervals), timeDiff)
	}
}

func TestFeeOptimizer_OptimizeForFees(t *testing.T) {
	optimizer := NewFeeOptimizer()
	
	routes := []Route{
		{
			Exchange:      "binance",
			Symbol:        "BTCUSDT",
			Quantity:      decimal.NewFromInt(100),
			ExpectedPrice: decimal.NewFromInt(40000),
			Market:        types.MarketTypeSpot,
		},
		{
			Exchange:      "okx",
			Symbol:        "BTCUSDT",
			Quantity:      decimal.NewFromInt(100),
			ExpectedPrice: decimal.NewFromInt(40100),
			Market:        types.MarketTypeSpot,
		},
	}
	
	volumeInfo := map[string]decimal.Decimal{
		"binance": decimal.NewFromInt(1000000),
		"okx":     decimal.NewFromInt(500000),
	}
	
	optimized := optimizer.OptimizeForFees(routes, types.OrderSideBuy, volumeInfo)
	assert.Equal(t, len(routes), len(optimized))
	
	// Verify fees are calculated
	for _, route := range optimized {
		assert.True(t, route.ExpectedFee.GreaterThan(decimal.Zero))
	}
}

func TestRoutingEngine_CalculateOptimalSplits(t *testing.T) {
	config := &RoutingConfig{
		MaxSplits:         5,
		MinSplitSize:      decimal.NewFromInt(10),
		OptimalSplitRatio: decimal.NewFromFloat(0.3),
	}
	
	engine := &RoutingEngine{
		config: config,
	}
	
	order := &types.Order{
		Symbol:   "BTCUSDT",
		Side:     types.OrderSideBuy,
		Quantity: decimal.NewFromInt(1000),
		Price:    decimal.NewFromInt(40000),
	}
	
	// Mock market depth
	marketDepth := &AggregatedMarketDepth{
		Symbol: "BTCUSDT",
		ExchangeDepths: map[string]*ExchangeOrderBook{
			"binance": {
				Exchange: "binance",
				Asks: []types.PriceLevel{
					{Price: decimal.NewFromInt(40000), Quantity: decimal.NewFromInt(500)},
					{Price: decimal.NewFromInt(40100), Quantity: decimal.NewFromInt(300)},
				},
			},
			"okx": {
				Exchange: "okx",
				Asks: []types.PriceLevel{
					{Price: decimal.NewFromInt(40050), Quantity: decimal.NewFromInt(400)},
					{Price: decimal.NewFromInt(40150), Quantity: decimal.NewFromInt(200)},
				},
			},
		},
	}
	
	options := RoutingOptions{
		MaxSplits: 5,
	}
	
	splits := engine.calculateOptimalSplits(order, marketDepth, options)
	assert.True(t, len(splits) > 0)
	
	// Verify total quantity
	totalQty := decimal.Zero
	for _, split := range splits {
		totalQty = totalQty.Add(split.Quantity)
	}
	assert.True(t, totalQty.LessThanOrEqual(order.Quantity))
}

func TestExecutionEngine_WorkerPool(t *testing.T) {
	pool := NewWorkerPool(5)
	pool.Start()
	defer pool.Stop()
	
	// Submit tasks
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		i := i
		pool.Submit(func() {
			time.Sleep(10 * time.Millisecond)
			done <- true
		})
	}
	
	// Wait for all tasks
	for i := 0; i < 10; i++ {
		select {
		case <-done:
			// Task completed
		case <-time.After(1 * time.Second):
			t.Fatal("Task timeout")
		}
	}
}

func TestFeeOptimizer_SuggestFeeOptimizations(t *testing.T) {
	optimizer := NewFeeOptimizer()
	
	currentVolume := decimal.NewFromInt(45000000) // $45M volume
	suggestions := optimizer.SuggestFeeOptimizations("binance", currentVolume)
	
	assert.True(t, len(suggestions) > 0)
	
	// Check suggestion types
	hasVolumeSuggestion := false
	hasOrderTypeSuggestion := false
	
	for _, suggestion := range suggestions {
		if suggestion.Type == "volume_tier" {
			hasVolumeSuggestion = true
		}
		if suggestion.Type == "order_type" {
			hasOrderTypeSuggestion = true
		}
	}
	
	assert.True(t, hasVolumeSuggestion || hasOrderTypeSuggestion)
}