package integration

import (
	"context"
	"testing"
	"time"

	"github.com/mExOms/internal/exchange"
	"github.com/mExOms/internal/position"
	"github.com/mExOms/internal/risk"
	"github.com/mExOms/internal/router"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEndToEndOrderFlow tests complete order flow
func TestEndToEndOrderFlow(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup components
	ctx := context.Background()
	
	// Create exchange factory
	factory := exchange.NewFactory()
	
	// Create risk engine
	riskEngine := risk.NewRiskEngine()
	riskEngine.SetMaxOrderValue(decimal.NewFromFloat(50000))
	riskEngine.SetMaxPositionSize(decimal.NewFromFloat(100000))
	
	// Create position manager
	posManager, err := position.NewPositionManager("./test_data/snapshots")
	require.NoError(t, err)
	defer posManager.Close()
	
	// Create smart router
	exchanges := factory.GetAvailableExchanges()
	smartRouter := router.NewSmartRouter(exchanges)
	
	// Test order creation
	t.Run("OrderCreation", func(t *testing.T) {
		order := &types.Order{
			ClientOrderID: "test-order-001",
			Symbol:        "BTCUSDT",
			Side:          types.OrderSideBuy,
			Type:          types.OrderTypeLimit,
			Price:         decimal.NewFromFloat(42000),
			Quantity:      decimal.NewFromFloat(0.1),
			TimeInForce:   types.TimeInForceGTC,
		}
		
		// Risk check
		result, err := riskEngine.CheckOrder(ctx, order, "binance")
		assert.NoError(t, err)
		assert.True(t, result.Passed)
		assert.Empty(t, result.RejectionReason)
		
		// Route order (mock for now)
		routedOrder, err := smartRouter.RouteOrder(ctx, order)
		assert.NoError(t, err)
		assert.NotNil(t, routedOrder)
		
		// Update position
		pos := &position.Position{
			Symbol:     order.Symbol,
			Exchange:   "binance",
			Market:     "spot",
			Side:       "LONG",
			Quantity:   order.Quantity,
			EntryPrice: order.Price,
			MarkPrice:  order.Price,
			Leverage:   1,
			MarginUsed: order.Price.Mul(order.Quantity),
		}
		
		err = posManager.UpdatePosition(pos)
		assert.NoError(t, err)
		
		// Verify position
		retrieved, exists := posManager.GetPosition("binance", "BTCUSDT")
		assert.True(t, exists)
		assert.Equal(t, pos.Symbol, retrieved.Symbol)
		assert.Equal(t, pos.Quantity.String(), retrieved.Quantity.String())
	})
	
	// Test risk limits
	t.Run("RiskLimits", func(t *testing.T) {
		// Order exceeding max value
		largeOrder := &types.Order{
			Symbol:   "BTCUSDT",
			Side:     types.OrderSideBuy,
			Type:     types.OrderTypeLimit,
			Price:    decimal.NewFromFloat(42000),
			Quantity: decimal.NewFromFloat(2), // $84,000 > $50,000 limit
		}
		
		result, err := riskEngine.CheckOrder(ctx, largeOrder, "binance")
		assert.NoError(t, err)
		assert.False(t, result.Passed)
		assert.Contains(t, result.RejectionReason, "exceeds max order value")
	})
	
	// Test position aggregation
	t.Run("PositionAggregation", func(t *testing.T) {
		// Add positions on multiple exchanges
		exchanges := []string{"binance", "okx", "bybit"}
		for i, exch := range exchanges {
			pos := &position.Position{
				Symbol:     "ETHUSDT",
				Exchange:   exch,
				Market:     "spot",
				Side:       "LONG",
				Quantity:   decimal.NewFromFloat(float64(i + 1)),
				EntryPrice: decimal.NewFromFloat(2000),
				MarkPrice:  decimal.NewFromFloat(2100),
				Leverage:   1,
				MarginUsed: decimal.NewFromFloat(2000 * float64(i+1)),
			}
			
			err := posManager.UpdatePosition(pos)
			assert.NoError(t, err)
		}
		
		// Get aggregated positions
		aggregated := posManager.GetAggregatedPositions()
		ethAgg, exists := aggregated["ETHUSDT"]
		assert.True(t, exists)
		assert.Equal(t, 3, len(ethAgg.Positions))
		assert.Equal(t, "6", ethAgg.TotalQuantity.String()) // 1+2+3
	})
	
	// Test concurrent operations
	t.Run("ConcurrentOperations", func(t *testing.T) {
		done := make(chan bool, 3)
		
		// Concurrent risk checks
		go func() {
			for i := 0; i < 100; i++ {
				order := &types.Order{
					Symbol:   "BTCUSDT",
					Side:     types.OrderSideBuy,
					Type:     types.OrderTypeLimit,
					Price:    decimal.NewFromFloat(42000),
					Quantity: decimal.NewFromFloat(0.01),
				}
				riskEngine.CheckOrder(ctx, order, "binance")
			}
			done <- true
		}()
		
		// Concurrent position updates
		go func() {
			for i := 0; i < 100; i++ {
				pos := &position.Position{
					Symbol:     "SOLUSDT",
					Exchange:   "binance",
					Market:     "spot",
					Side:       "LONG",
					Quantity:   decimal.NewFromFloat(0.1),
					EntryPrice: decimal.NewFromFloat(100),
					MarkPrice:  decimal.NewFromFloat(101 + float64(i%5)),
					Leverage:   1,
					MarginUsed: decimal.NewFromFloat(10),
				}
				posManager.UpdatePosition(pos)
			}
			done <- true
		}()
		
		// Concurrent reads
		go func() {
			for i := 0; i < 100; i++ {
				posManager.GetAllPositions()
				posManager.CalculateTotalPnL()
			}
			done <- true
		}()
		
		// Wait for completion
		for i := 0; i < 3; i++ {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("Concurrent test timeout")
			}
		}
	})
}

// TestSystemPerformance tests system performance requirements
func TestSystemPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	ctx := context.Background()
	
	// Create risk engine
	riskEngine := risk.NewRiskEngine()
	riskEngine.SetMaxOrderValue(decimal.NewFromFloat(100000))
	
	// Test risk check performance
	t.Run("RiskCheckPerformance", func(t *testing.T) {
		order := &types.Order{
			Symbol:   "BTCUSDT",
			Side:     types.OrderSideBuy,
			Type:     types.OrderTypeLimit,
			Price:    decimal.NewFromFloat(42000),
			Quantity: decimal.NewFromFloat(0.1),
		}
		
		// Warm up
		for i := 0; i < 1000; i++ {
			riskEngine.CheckOrder(ctx, order, "binance")
		}
		
		// Measure
		iterations := 10000
		start := time.Now()
		
		for i := 0; i < iterations; i++ {
			_, err := riskEngine.CheckOrder(ctx, order, "binance")
			require.NoError(t, err)
		}
		
		elapsed := time.Since(start)
		avgLatency := elapsed / time.Duration(iterations)
		
		// Verify < 50 microseconds
		assert.Less(t, avgLatency, 50*time.Microsecond,
			"Risk check latency %.2f μs exceeds target of 50 μs", 
			float64(avgLatency.Nanoseconds())/1000)
		
		t.Logf("Risk check average latency: %.2f μs", float64(avgLatency.Nanoseconds())/1000)
	})
	
	// Test position update performance
	t.Run("PositionUpdatePerformance", func(t *testing.T) {
		posManager, err := position.NewPositionManager("./test_data/snapshots")
		require.NoError(t, err)
		defer posManager.Close()
		
		pos := &position.Position{
			Symbol:     "BTCUSDT",
			Exchange:   "binance",
			Market:     "spot",
			Side:       "LONG",
			Quantity:   decimal.NewFromFloat(0.5),
			EntryPrice: decimal.NewFromFloat(40000),
			MarkPrice:  decimal.NewFromFloat(42000),
			Leverage:   1,
			MarginUsed: decimal.NewFromFloat(20000),
		}
		
		iterations := 10000
		start := time.Now()
		
		for i := 0; i < iterations; i++ {
			pos.MarkPrice = decimal.NewFromFloat(42000 + float64(i%100))
			err := posManager.UpdatePosition(pos)
			require.NoError(t, err)
		}
		
		elapsed := time.Since(start)
		avgLatency := elapsed / time.Duration(iterations)
		
		// Log performance
		t.Logf("Position update average latency: %.2f μs", float64(avgLatency.Nanoseconds())/1000)
		t.Logf("Position updates per second: %.0f", float64(iterations)/elapsed.Seconds())
	})
}

// TestFailoverScenarios tests system resilience
func TestFailoverScenarios(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping failover test in short mode")
	}

	t.Run("ExchangeDisconnection", func(t *testing.T) {
		// Test handling of exchange disconnection
		// In production, this would test actual reconnection logic
		assert.True(t, true, "Failover test placeholder")
	})
	
	t.Run("DataCorruption", func(t *testing.T) {
		// Test handling of corrupted data
		// In production, this would test data validation and recovery
		assert.True(t, true, "Data corruption test placeholder")
	})
}