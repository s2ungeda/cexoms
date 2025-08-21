package benchmark

import (
	"testing"

	"github.com/mExOms/internal/risk"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// BenchmarkRiskCheck tests risk management performance
func BenchmarkRiskCheck(b *testing.B) {
	rm := risk.NewRiskManager()
	
	// Set risk parameters
	rm.SetMaxDrawdown(0.10)
	rm.SetMaxExposure(decimal.NewFromInt(100000))
	rm.SetMaxPositionCount(20)
	
	// Update account balance
	rm.UpdateBalance("test-account", decimal.NewFromInt(10000))
	
	order := &types.Order{
		Symbol:   "BTCUSDT",
		Side:     types.OrderSideBuy,
		Type:     types.OrderTypeLimit,
		Quantity: decimal.NewFromFloat(0.001),
		Price:    decimal.NewFromFloat(40000),
		Metadata: map[string]interface{}{
			"account_id": "test-account",
		},
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		err := rm.CheckOrderRisk(order)
		if err != nil {
			b.Fatal(err)
		}
	}
	
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "checks/sec")
}

// BenchmarkPositionSizing tests position size calculation performance
func BenchmarkPositionSizing(b *testing.B) {
	rm := risk.NewRiskManager()
	
	params := risk.PositionSizeParams{
		AccountBalance: decimal.NewFromInt(10000),
		RiskPercentage: 2.0,
		StopDistance:   decimal.NewFromFloat(0.03),
		Symbol:         "BTCUSDT",
		Leverage:       5,
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_ = rm.CalculatePositionSize(params)
	}
	
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "calculations/sec")
}

// BenchmarkRiskMetrics tests risk metrics calculation performance
func BenchmarkRiskMetrics(b *testing.B) {
	rm := risk.NewRiskManager()
	
	// Set up account with positions
	rm.UpdateBalance("test-account", decimal.NewFromInt(10000))
	
	// Add some positions
	for i := 0; i < 5; i++ {
		position := &types.Position{
			Symbol:        "BTCUSDT",
			Side:          types.Side("LONG"),
			Amount:        decimal.NewFromFloat(0.1),
			EntryPrice:    decimal.NewFromFloat(40000),
			MarkPrice:     decimal.NewFromFloat(40100),
			UnrealizedPnL: decimal.NewFromFloat(10),
		}
		rm.UpdatePosition("test-account", position)
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_ = rm.GetAccountRiskMetrics("test-account")
	}
	
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "calculations/sec")
}

// BenchmarkLimitCheck tests limit checking performance
func BenchmarkLimitCheck(b *testing.B) {
	lm := risk.NewRiskLimitManager()
	
	// Set various limits
	lm.SetLimit("test-account", risk.LimitTypeMaxLoss, decimal.NewFromInt(1000), []risk.LimitAction{
		{Threshold: 0.8, Action: "warn"},
		{Threshold: 1.0, Action: "restrict"},
	})
	
	lm.SetLimit("test-account", risk.LimitTypeMaxExposure, decimal.NewFromInt(50000), []risk.LimitAction{
		{Threshold: 0.9, Action: "warn"},
	})
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		err := lm.CheckLimit("test-account", risk.LimitTypeMaxLoss, decimal.NewFromInt(500))
		if err != nil {
			b.Fatal(err)
		}
	}
	
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "checks/sec")
}

// BenchmarkStopLossCalculation tests stop loss calculation performance
func BenchmarkStopLossCalculation(b *testing.B) {
	config := risk.StopLossConfig{
		Type:            risk.StopLossTypeTrailing,
		TrailingPercent: 2.0,
	}
	
	slm := risk.NewStopLossManager(config)
	
	position := &types.Position{
		Symbol:     "BTCUSDT",
		Side:       types.Side("LONG"),
		Amount:     decimal.NewFromFloat(0.1),
		EntryPrice: decimal.NewFromFloat(40000),
		MarkPrice:  decimal.NewFromFloat(40100),
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_, err := slm.CreateStopLoss("test-account", position, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
	
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "calculations/sec")
}

// BenchmarkRiskMonitoring tests risk monitoring performance
func BenchmarkRiskMonitoring(b *testing.B) {
	rm := risk.NewRiskManager()
	lm := risk.NewRiskLimitManager()
	slm := risk.NewStopLossManager(risk.StopLossConfig{
		Type:            risk.StopLossTypeTrailing,
		TrailingPercent: 2.0,
	})
	
	monitor := risk.NewRiskMonitor(rm, lm, slm)
	
	// Add test positions
	for i := 0; i < 10; i++ {
		position := &types.Position{
			Symbol:        "BTCUSDT",
			Side:          types.Side("LONG"),
			Amount:        decimal.NewFromFloat(0.1),
			EntryPrice:    decimal.NewFromFloat(40000),
			MarkPrice:     decimal.NewFromFloat(40000 + float64(i*100)),
			UnrealizedPnL: decimal.NewFromFloat(float64(i * 10)),
		}
		monitor.UpdatePosition("test-account", position)
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_ = monitor.GetRiskSummary()
	}
	
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "summaries/sec")
}