package main

import (
	"context"
	"fmt"
	"time"
	
	"github.com/mExOms/internal/risk"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

func main() {
	fmt.Println("=== Testing Risk Management System ===\n")
	
	// Create risk manager
	rm := risk.NewRiskManager()
	
	// Configure risk limits
	rm.SetMaxDrawdown(0.10)  // 10% max drawdown
	rm.SetMaxExposure(decimal.NewFromInt(50000))  // $50k max exposure
	rm.SetMaxPositionCount(10)  // Max 10 positions
	
	fmt.Println("✓ Risk manager configured:")
	fmt.Println("  Max drawdown: 10%")
	fmt.Println("  Max exposure: $50,000")
	fmt.Println("  Max positions: 10")
	
	// Test 1: Position Size Calculation
	fmt.Println("\n1. Testing Position Size Calculation")
	testPositionSizing(rm)
	
	// Test 2: Order Risk Validation
	fmt.Println("\n2. Testing Order Risk Validation")
	testOrderRiskValidation(rm)
	
	// Test 3: Risk Limits
	fmt.Println("\n3. Testing Risk Limits")
	testRiskLimits(rm)
	
	// Test 4: Stop Loss Management
	fmt.Println("\n4. Testing Stop Loss Management")
	testStopLossManagement()
	
	// Test 5: Real-time Monitoring
	fmt.Println("\n5. Testing Real-time Risk Monitoring")
	testRiskMonitoring(rm)
	
	fmt.Println("\n=== All Risk Management Tests Completed ===")
}

func testPositionSizing(rm risk.Manager) {
	params := risk.PositionSizeParams{
		AccountBalance: decimal.NewFromInt(10000),
		RiskPercentage: 2.0,  // 2% risk per trade
		StopDistance:   decimal.NewFromFloat(0.03),  // 3% stop distance
		Symbol:         "BTCUSDT",
	}
	
	// Test different algorithms
	calculator := risk.NewPositionSizeCalculator(2.0)
	
	// Fixed Fractional
	size := calculator.FixedFractional(
		params.AccountBalance,
		decimal.NewFromInt(40000),  // Entry price
		decimal.NewFromInt(38800),  // Stop price (3% below)
		params.RiskPercentage,
	)
	fmt.Printf("  Fixed Fractional (2%% risk): %s BTC\n", size.String())
	
	// Kelly Criterion
	kellySize := calculator.KellyCriterion(
		params.AccountBalance,
		0.55,  // 55% win rate
		1.5,   // Average win 1.5%
		1.0,   // Average loss 1%
	)
	fmt.Printf("  Kelly Criterion: %s USDT position\n", kellySize.String())
	
	// Volatility-based
	volatilitySize := calculator.VolatilityBased(
		params.AccountBalance,
		decimal.NewFromFloat(1000),  // ATR value
		2.0,    // 2% risk
	)
	fmt.Printf("  Volatility-based: %s BTC\n", volatilitySize.String())
}

func testOrderRiskValidation(rm risk.Manager) {
	// Create test order
	order := &types.Order{
		Symbol:   "ETHUSDT",
		Side:     types.OrderSideBuy,
		Type:     types.OrderTypeLimit,
		Quantity: decimal.NewFromFloat(10),
		Price:    decimal.NewFromFloat(2500),
	}
	
	// Check order risk
	err := rm.CheckOrderRisk(order)
	if err != nil {
		fmt.Printf("  ❌ Order rejected: %v\n", err)
	} else {
		fmt.Printf("  ✓ Order passed risk checks\n")
	}
	
	// Test with excessive size
	bigOrder := &types.Order{
		Symbol:   "BTCUSDT",
		Side:     types.OrderSideBuy,
		Type:     types.OrderTypeLimit,
		Quantity: decimal.NewFromFloat(100),
		Price:    decimal.NewFromFloat(40000),
	}
	
	err = rm.CheckOrderRisk(bigOrder)
	if err != nil {
		fmt.Printf("  ✓ Large order correctly rejected: %v\n", err)
	}
}

func testRiskLimits(rm risk.Manager) {
	limitsManager := risk.NewRiskLimitsManager()
	
	// Set various limits
	limitsManager.SetLimit("max_position_value", decimal.NewFromInt(50000), risk.ActionReject)
	limitsManager.SetLimit("max_leverage", decimal.NewFromInt(5), risk.ActionWarn)
	limitsManager.SetLimit("min_margin_level", decimal.NewFromFloat(1.5), risk.ActionAlert)
	
	// Test limit checks
	fmt.Println("  Testing position value limit:")
	status := limitsManager.CheckLimit("max_position_value", decimal.NewFromInt(45000))
	fmt.Printf("    $45,000 position: %s\n", status.Status)
	
	status = limitsManager.CheckLimit("max_position_value", decimal.NewFromInt(55000))
	fmt.Printf("    $55,000 position: %s (Action: %s)\n", status.Status, status.Action)
	
	// Get all active alerts
	alerts := limitsManager.GetActiveAlerts()
	if len(alerts) > 0 {
		fmt.Printf("  Active alerts: %d\n", len(alerts))
	}
}

func testStopLossManagement() {
	slManager := risk.NewStopLossManager()
	
	// Configure auto stop loss
	slManager.SetAutoStopLoss(true, 0.02)  // 2% stop loss
	
	// Calculate stop loss levels
	entryPrice := decimal.NewFromInt(3000)
	
	// Fixed stop loss
	fixedStop := slManager.CalculateFixedStopLoss(entryPrice, types.OrderSideBuy, 0.03)
	fmt.Printf("  Fixed stop loss (3%%): $%s\n", fixedStop.String())
	
	// ATR-based stop loss
	atrStop := slManager.CalculateATRStopLoss(entryPrice, types.OrderSideBuy, decimal.NewFromFloat(50), 2.0)
	fmt.Printf("  ATR stop loss (2x ATR): $%s\n", atrStop.String())
	
	// Trailing stop
	currentPrice := decimal.NewFromFloat(3150)  // 5% profit
	trailingStop := slManager.UpdateTrailingStop(
		entryPrice,
		currentPrice,
		decimal.NewFromFloat(2940),  // Current stop
		types.OrderSideBuy,
		0.02,  // 2% trailing distance
	)
	fmt.Printf("  Trailing stop updated to: $%s\n", trailingStop.String())
}

func testRiskMonitoring(rm risk.Manager) {
	monitor := risk.NewRiskMonitor(rm)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// Start monitoring
	alertChan := make(chan risk.RiskAlert, 10)
	go monitor.Start(ctx, alertChan)
	
	fmt.Println("  Monitoring risk for 5 seconds...")
	
	// Simulate some risk events
	go func() {
		time.Sleep(1 * time.Second)
		// Simulate adding exposure
		rm.UpdateExposure("BTCUSDT", decimal.NewFromInt(45000))
		
		time.Sleep(1 * time.Second)
		// Simulate exceeding limit
		rm.UpdateExposure("ETHUSDT", decimal.NewFromInt(20000))
	}()
	
	// Listen for alerts
	for {
		select {
		case alert := <-alertChan:
			fmt.Printf("  🚨 Risk Alert: %s (Severity: %s)\n", alert.Message, alert.Severity)
		case <-ctx.Done():
			fmt.Println("  Monitoring stopped")
			return
		}
	}
}