package benchmark

import (
	"context"
	"testing"

	"github.com/mExOms/internal/router"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// BenchmarkSmartRouting tests smart router performance
func BenchmarkSmartRouting(b *testing.B) {
	ctx := context.Background()
	
	// Create smart router with mock exchanges
	smartRouter := router.NewSmartRouter()
	
	// Add mock exchanges
	for i := 0; i < 3; i++ {
		exchange := &MockExchange{}
		smartRouter.AddExchange(exchange)
	}
	
	order := &types.Order{
		Symbol:   "BTCUSDT",
		Side:     types.OrderSideBuy,
		Type:     types.OrderTypeLimit,
		Quantity: decimal.NewFromFloat(1.0),
		Price:    decimal.NewFromFloat(40000),
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_, err := smartRouter.RouteOrder(ctx, order)
		if err != nil {
			b.Fatal(err)
		}
	}
	
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "routes/sec")
}

// BenchmarkOrderSplitting tests order splitting performance
func BenchmarkOrderSplitting(b *testing.B) {
	ctx := context.Background()
	
	smartRouter := router.NewSmartRouter()
	
	// Add mock exchanges with different liquidity
	for i := 0; i < 5; i++ {
		exchange := &MockExchange{}
		smartRouter.AddExchange(exchange)
	}
	
	order := &types.Order{
		Symbol:   "BTCUSDT",
		Side:     types.OrderSideBuy,
		Type:     types.OrderTypeLimit,
		Quantity: decimal.NewFromFloat(10.0), // Large order to trigger splitting
		Price:    decimal.NewFromFloat(40000),
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		splits := smartRouter.SplitOrder(order, 5)
		if len(splits) == 0 {
			b.Fatal("No order splits generated")
		}
	}
	
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "splits/sec")
}

// BenchmarkFeeOptimization tests fee optimization performance
func BenchmarkFeeOptimization(b *testing.B) {
	smartRouter := router.NewSmartRouter()
	
	// Create fee structures for different exchanges
	feeStructures := []router.FeeStructure{
		{
			Exchange:   "exchange1",
			MakerFee:   decimal.NewFromFloat(0.001),
			TakerFee:   decimal.NewFromFloat(0.002),
			VolumeDiscount: map[decimal.Decimal]decimal.Decimal{
				decimal.NewFromInt(1000000): decimal.NewFromFloat(0.0001),
			},
		},
		{
			Exchange:   "exchange2",
			MakerFee:   decimal.NewFromFloat(0.0008),
			TakerFee:   decimal.NewFromFloat(0.0015),
			VolumeDiscount: map[decimal.Decimal]decimal.Decimal{
				decimal.NewFromInt(500000): decimal.NewFromFloat(0.0001),
			},
		},
		{
			Exchange:   "exchange3",
			MakerFee:   decimal.NewFromFloat(0.0012),
			TakerFee:   decimal.NewFromFloat(0.0018),
		},
	}
	
	orderValue := decimal.NewFromFloat(50000)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_ = smartRouter.CalculateFees(orderValue, feeStructures)
	}
	
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "calculations/sec")
}

// BenchmarkArbitrageDetection tests arbitrage opportunity detection
func BenchmarkArbitrageDetection(b *testing.B) {
	ctx := context.Background()
	smartRouter := router.NewSmartRouter()
	
	// Add mock exchanges with slightly different prices
	for i := 0; i < 3; i++ {
		exchange := &MockExchange{}
		smartRouter.AddExchange(exchange)
	}
	
	symbol := "BTCUSDT"
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		opportunities := smartRouter.FindArbitrageOpportunities(ctx, []string{symbol})
		_ = opportunities
	}
	
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "scans/sec")
}

// BenchmarkConcurrentRouting tests concurrent order routing
func BenchmarkConcurrentRouting(b *testing.B) {
	ctx := context.Background()
	smartRouter := router.NewSmartRouter()
	
	// Add mock exchanges
	for i := 0; i < 5; i++ {
		exchange := &MockExchange{}
		smartRouter.AddExchange(exchange)
	}
	
	b.RunParallel(func(pb *testing.PB) {
		order := &types.Order{
			Symbol:   "BTCUSDT",
			Side:     types.OrderSideBuy,
			Type:     types.OrderTypeLimit,
			Quantity: decimal.NewFromFloat(0.1),
			Price:    decimal.NewFromFloat(40000),
		}
		
		for pb.Next() {
			_, err := smartRouter.RouteOrder(ctx, order)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "routes/sec")
}