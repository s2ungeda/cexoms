package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mExOms/internal/risk"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRiskManagementIntegration(t *testing.T) {
	// Create risk manager
	rm := risk.NewRiskManager()
	
	// Configure limits
	rm.SetMaxDrawdown(0.10)                      // 10% max drawdown
	rm.SetMaxExposure(decimal.NewFromInt(50000)) // $50k max exposure
	rm.SetMaxPositionCount(10)                   // Max 10 positions
	
	t.Run("PositionSizing", func(t *testing.T) {
		params := risk.PositionSizeParams{
			AccountBalance: decimal.NewFromInt(10000),
			RiskPercentage: 2.0, // 2% risk per trade
			StopDistance:   decimal.NewFromFloat(0.03), // 3% stop
			Symbol:         "BTCUSDT",
		}
		
		size := rm.CalculatePositionSize(params)
		assert.True(t, size.GreaterThan(decimal.Zero))
		
		// Verify risk is limited to 2% of account
		maxRisk := params.AccountBalance.Mul(decimal.NewFromFloat(0.02))
		positionValue := size.Mul(decimal.NewFromInt(40000)) // Assume BTC price
		potentialLoss := positionValue.Mul(params.StopDistance)
		assert.True(t, potentialLoss.LessThanOrEqual(maxRisk))
	})
	
	t.Run("OrderRiskValidation", func(t *testing.T) {
		// Valid order
		order := &types.Order{
			Symbol:   "ETHUSDT",
			Side:     types.OrderSideBuy,
			Type:     types.OrderTypeLimit,
			Quantity: decimal.NewFromFloat(10),
			Price:    decimal.NewFromFloat(2500),
		}
		
		err := rm.CheckOrderRisk(order)
		assert.NoError(t, err)
		
		// Order exceeding exposure limit
		bigOrder := &types.Order{
			Symbol:   "BTCUSDT",
			Side:     types.OrderSideBuy,
			Type:     types.OrderTypeLimit,
			Quantity: decimal.NewFromFloat(10), // 10 BTC
			Price:    decimal.NewFromFloat(40000), // $400k total
		}
		
		err = rm.CheckOrderRisk(bigOrder)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exposure")
	})
	
	t.Run("RiskLimits", func(t *testing.T) {
		limitsManager := risk.NewRiskLimitsManager()
		
		// Set various limits
		limitsManager.SetLimit("max_position_value", decimal.NewFromInt(50000), risk.ActionReject)
		limitsManager.SetLimit("max_leverage", decimal.NewFromInt(5), risk.ActionWarn)
		limitsManager.SetLimit("min_margin_level", decimal.NewFromFloat(1.5), risk.ActionAlert)
		
		// Test limit checks
		status := limitsManager.CheckLimit("max_position_value", decimal.NewFromInt(45000))
		assert.Equal(t, risk.LimitStatusOK, status.Status)
		
		status = limitsManager.CheckLimit("max_position_value", decimal.NewFromInt(55000))
		assert.Equal(t, risk.LimitStatusExceeded, status.Status)
		assert.Equal(t, risk.ActionReject, status.Action)
		
		// Check active alerts
		alerts := limitsManager.GetActiveAlerts()
		assert.NotEmpty(t, alerts)
	})
	
	t.Run("StopLossCalculation", func(t *testing.T) {
		slManager := risk.NewStopLossManager()
		slManager.SetAutoStopLoss(true, 0.02) // 2% auto stop loss
		
		entryPrice := decimal.NewFromInt(3000)
		
		// Fixed stop loss
		fixedStop := slManager.CalculateFixedStopLoss(entryPrice, types.OrderSideBuy, 0.03)
		expectedStop := entryPrice.Mul(decimal.NewFromFloat(0.97)) // 3% below entry
		assert.True(t, fixedStop.Equal(expectedStop))
		
		// ATR-based stop loss
		atr := decimal.NewFromFloat(50)
		atrStop := slManager.CalculateATRStopLoss(entryPrice, types.OrderSideBuy, atr, 2.0)
		expectedAtrStop := entryPrice.Sub(atr.Mul(decimal.NewFromFloat(2.0)))
		assert.True(t, atrStop.Equal(expectedAtrStop))
		
		// Trailing stop update
		currentPrice := decimal.NewFromFloat(3150) // 5% profit
		currentStop := decimal.NewFromFloat(2940)
		trailingStop := slManager.UpdateTrailingStop(
			entryPrice,
			currentPrice,
			currentStop,
			types.OrderSideBuy,
			0.02, // 2% trailing
		)
		
		// Trailing stop should move up
		assert.True(t, trailingStop.GreaterThan(currentStop))
		expectedTrailing := currentPrice.Mul(decimal.NewFromFloat(0.98))
		assert.True(t, trailingStop.Equal(expectedTrailing))
	})
	
	t.Run("RiskMonitoring", func(t *testing.T) {
		monitor := risk.NewRiskMonitor(rm)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		
		alertChan := make(chan risk.RiskAlert, 10)
		go monitor.Start(ctx, alertChan)
		
		// Simulate adding exposure
		rm.UpdateExposure("BTCUSDT", decimal.NewFromInt(30000))
		rm.UpdateExposure("ETHUSDT", decimal.NewFromInt(25000)) // Total: $55k, exceeds limit
		
		// Wait for alert
		select {
		case alert := <-alertChan:
			assert.Equal(t, risk.SeverityHigh, alert.Severity)
			assert.Contains(t, alert.Message, "exposure")
		case <-time.After(1 * time.Second):
			t.Error("Expected risk alert not received")
		}
	})
	
	t.Run("PortfolioRisk", func(t *testing.T) {
		// Test portfolio-wide risk metrics
		positions := []*types.Position{
			{
				Symbol:        "BTCUSDT",
				Side:          types.PositionSideLong,
				Amount:        decimal.NewFromFloat(0.5),
				EntryPrice:    decimal.NewFromInt(40000),
				MarkPrice:     decimal.NewFromInt(41000),
				UnrealizedPnL: decimal.NewFromInt(500),
			},
			{
				Symbol:        "ETHUSDT",
				Side:          types.PositionSideLong,
				Amount:        decimal.NewFromFloat(10),
				EntryPrice:    decimal.NewFromFloat(2500),
				MarkPrice:     decimal.NewFromFloat(2450),
				UnrealizedPnL: decimal.NewFromInt(-500),
			},
		}
		
		totalExposure := decimal.Zero
		totalPnL := decimal.Zero
		
		for _, pos := range positions {
			positionValue := pos.Amount.Mul(pos.MarkPrice)
			totalExposure = totalExposure.Add(positionValue)
			totalPnL = totalPnL.Add(pos.UnrealizedPnL)
		}
		
		assert.Equal(t, decimal.NewFromInt(45000), totalExposure) // 0.5 * 41000 + 10 * 2450
		assert.Equal(t, decimal.Zero, totalPnL) // +500 - 500 = 0
	})
}

func TestRiskManagerConcurrency(t *testing.T) {
	rm := risk.NewRiskManager()
	rm.SetMaxExposure(decimal.NewFromInt(100000))
	
	// Test concurrent access
	done := make(chan bool)
	
	// Multiple goroutines updating exposure
	for i := 0; i < 10; i++ {
		go func(id int) {
			symbol := fmt.Sprintf("TEST%d", id)
			for j := 0; j < 100; j++ {
				rm.UpdateExposure(symbol, decimal.NewFromInt(int64(j*100)))
			}
			done <- true
		}(i)
	}
	
	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
	
	// Verify state is consistent
	exposure := rm.GetCurrentExposure()
	assert.True(t, exposure.GreaterThanOrEqual(decimal.Zero))
}