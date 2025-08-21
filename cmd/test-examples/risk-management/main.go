package main

import (
	"fmt"
	"log"
	"time"

	"github.com/mExOms/internal/risk"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

func main() {
	fmt.Println("=== Risk Management Test ===")
	
	// Create risk manager
	riskManager := risk.NewRiskManager()
	
	// Set risk limits
	riskManager.SetMaxDrawdown(0.10)  // 10% max drawdown
	riskManager.SetMaxExposure(decimal.NewFromInt(50000)) // $50k max exposure
	riskManager.SetMaxPositionCount(5) // Max 5 positions
	
	fmt.Println("\n1. Risk Limits Set:")
	fmt.Printf("   - Max Drawdown: 10%%\n")
	fmt.Printf("   - Max Exposure: $50,000\n")
	fmt.Printf("   - Max Positions: 5\n")
	
	// Create limit manager
	limitManager := risk.NewRiskLimitManager()
	
	// Set various limits
	limitManager.SetLimit("main-account", risk.LimitTypeMaxLoss, decimal.NewFromInt(1000), []risk.LimitAction{
		{Threshold: 0.8, Action: "warn", Notification: true},
		{Threshold: 1.0, Action: "restrict", Notification: true},
	})
	
	limitManager.SetLimit("main-account", risk.LimitTypeDailyLoss, decimal.NewFromInt(500), []risk.LimitAction{
		{Threshold: 0.9, Action: "warn", Notification: true},
	})
	
	// Create stop loss manager
	stopLossConfig := risk.StopLossConfig{
		Type:       risk.StopLossTypeTrailing,
		TrailingPercent: 2.0, // 2% trailing stop
	}
	stopLossManager := risk.NewStopLossManager(stopLossConfig)
	
	// Create risk monitor
	monitor := risk.NewRiskMonitor(riskManager, limitManager, stopLossManager)
	
	// Set alert callback
	monitor.SetAlertCallback(func(alert *risk.Alert) {
		fmt.Printf("\n🚨 ALERT: [%s] %s - %s\n", alert.Severity, alert.Type, alert.Message)
	})
	
	// Set metrics callback
	monitor.SetMetricsCallback(func(metrics map[string]*risk.RiskMetrics) {
		for account, m := range metrics {
			fmt.Printf("\n📊 Metrics for %s:\n", account)
			fmt.Printf("   - Total Exposure: %s\n", m.TotalExposure)
			fmt.Printf("   - Open Positions: %d\n", m.OpenPositions)
			fmt.Printf("   - Current Drawdown: %.2f%%\n", m.CurrentDrawdown*100)
		}
	})
	
	// Start monitoring
	monitor.SetInterval(risk.MonitoringIntervalFast)
	err := monitor.Start()
	if err != nil {
		log.Fatal(err)
	}
	defer monitor.Stop()
	
	fmt.Println("\n2. Risk Monitor Started")
	
	// Simulate some positions
	fmt.Println("\n3. Adding Test Positions:")
	
	// Position 1: BTC Long
	btcPosition := &types.Position{
		Symbol:        "BTCUSDT",
		Side:          types.Side("LONG"),
		Amount:        decimal.NewFromFloat(0.5),
		EntryPrice:    decimal.NewFromInt(40000),
		MarkPrice:     decimal.NewFromInt(41000),
		UnrealizedPnL: decimal.NewFromInt(500),
		Leverage:      5,
	}
	
	monitor.UpdatePosition("main-account", btcPosition)
	riskManager.UpdateBalance("main-account", decimal.NewFromInt(10000))
	
	// Create stop loss for BTC position
	stopLoss, err := stopLossManager.CreateStopLoss("main-account", btcPosition, nil)
	if err == nil {
		fmt.Printf("   - BTC Stop Loss created at: %s\n", stopLoss.StopPrice)
	}
	
	// Position 2: ETH Long
	ethPosition := &types.Position{
		Symbol:        "ETHUSDT",
		Side:          types.Side("LONG"),
		Amount:        decimal.NewFromInt(5),
		EntryPrice:    decimal.NewFromInt(2500),
		MarkPrice:     decimal.NewFromInt(2450),
		UnrealizedPnL: decimal.NewFromInt(-250),
		Leverage:      3,
	}
	
	monitor.UpdatePosition("main-account", ethPosition)
	
	// Test position size calculation
	fmt.Println("\n4. Position Size Calculation:")
	
	params := risk.PositionSizeParams{
		AccountBalance: decimal.NewFromInt(10000),
		RiskPercentage: 2.0,
		StopDistance:   decimal.NewFromFloat(0.03), // 3% stop
		Symbol:         "BTCUSDT",
		Leverage:       5,
	}
	
	posSize := riskManager.CalculatePositionSize(params)
	fmt.Printf("   - Recommended position size: %s\n", posSize)
	
	// Test order risk check
	fmt.Println("\n5. Order Risk Check:")
	
	testOrder := &types.Order{
		Symbol:   "BTCUSDT",
		Side:     types.OrderSideBuy,
		Quantity: decimal.NewFromFloat(2),
		Price:    decimal.NewFromFloat(40000),
		Metadata: map[string]interface{}{
			"account_id": "main-account",
		},
	}
	
	err = riskManager.CheckOrderRisk(testOrder)
	if err != nil {
		fmt.Printf("   ❌ Order rejected: %v\n", err)
	} else {
		fmt.Printf("   ✅ Order passed risk checks\n")
	}
	
	// Update some prices to trigger alerts
	fmt.Println("\n6. Simulating Price Updates:")
	
	// BTC price goes up (trailing stop should adjust)
	monitor.UpdatePrice("BTCUSDT", decimal.NewFromFloat(42000))
	time.Sleep(100 * time.Millisecond)
	
	// ETH price goes down more
	monitor.UpdatePrice("ETHUSDT", decimal.NewFromFloat(2400))
	time.Sleep(100 * time.Millisecond)
	
	// Get current exposure
	exposure := riskManager.GetCurrentExposure()
	fmt.Printf("\n7. Current Total Exposure: %s\n", exposure)
	
	// Get risk summary
	summary := monitor.GetRiskSummary()
	fmt.Printf("\n8. Risk Summary: %+v\n", summary)
	
	// Check active alerts
	alerts := monitor.GetActiveAlerts()
	fmt.Printf("\n9. Active Alerts: %d\n", len(alerts))
	for _, alert := range alerts {
		fmt.Printf("   - %s: %s\n", alert.Type, alert.Message)
	}
	
	// Test position size calculator
	fmt.Println("\n10. Advanced Position Sizing:")
	
	calculator := risk.NewPositionSizeCalculator(2.0)
	
	// Fixed fractional
	ffSize := calculator.FixedFractional(
		decimal.NewFromInt(10000),
		decimal.NewFromFloat(40000),
		decimal.NewFromFloat(39000),
		2.0,
	)
	fmt.Printf("   - Fixed Fractional: %s\n", ffSize)
	
	// Kelly Criterion
	kellySize := calculator.KellyCriterion(
		decimal.NewFromInt(10000),
		0.6,  // 60% win rate
		1.5,  // avg win
		1.0,  // avg loss
	)
	fmt.Printf("   - Kelly Criterion: %s\n", kellySize)
	
	// Wait a bit to see monitoring in action
	fmt.Println("\n\nMonitoring for 5 seconds...")
	time.Sleep(5 * time.Second)
	
	fmt.Println("\n=== Test Complete ===")
}