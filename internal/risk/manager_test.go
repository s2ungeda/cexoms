package risk

import (
	"testing"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestNewRiskManager(t *testing.T) {
	rm := NewRiskManager()
	assert.NotNil(t, rm)
	assert.Equal(t, 0.10, rm.maxDrawdown)
	assert.Equal(t, decimal.NewFromInt(100000), rm.maxExposure)
	assert.Equal(t, 10, rm.maxPositionCount)
}

func TestRiskManager_CheckOrderRisk(t *testing.T) {
	rm := NewRiskManager()
	
	// Set a low max exposure for testing
	rm.SetMaxExposure(decimal.NewFromInt(10000))
	
	// Create test order
	order := &types.Order{
		Symbol:   "BTCUSDT",
		Quantity: decimal.NewFromFloat(0.5),
		Price:    decimal.NewFromFloat(30000),
		Metadata: map[string]interface{}{
			"account_id": "test-account",
		},
	}
	
	// This should exceed max exposure (0.5 * 30000 = 15000 > 10000)
	err := rm.CheckOrderRisk(order)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceed max exposure")
}

func TestRiskManager_CalculatePositionSize(t *testing.T) {
	rm := NewRiskManager()
	
	params := PositionSizeParams{
		AccountBalance: decimal.NewFromInt(10000),
		RiskPercentage: 2.0, // 2% risk
		StopDistance:   decimal.NewFromFloat(0.05), // 5% stop
		Symbol:         "BTCUSDT",
		Leverage:       1,
	}
	
	size := rm.CalculatePositionSize(params)
	
	// Expected: (10000 * 0.02) / 0.05 = 200 / 0.05 = 4000
	expected := decimal.NewFromInt(4000)
	assert.True(t, size.Equal(expected))
}

func TestRiskManager_CalculateStopLoss(t *testing.T) {
	rm := NewRiskManager()
	
	entry := decimal.NewFromFloat(100)
	riskPercent := 2.0
	
	stopLoss := rm.CalculateStopLoss(entry, riskPercent)
	
	// Expected: 100 * (1 - 0.02) = 98
	expected := decimal.NewFromFloat(98)
	assert.True(t, stopLoss.Equal(expected))
}

func TestRiskManager_UpdatePosition(t *testing.T) {
	rm := NewRiskManager()
	
	position := &types.Position{
		Symbol:     "BTCUSDT",
		Quantity:   0.5,
		EntryPrice: 40000,
		MarkPrice:  41000,
	}
	
	rm.UpdatePosition("test-account", position)
	
	// Check that position was stored
	assert.Contains(t, rm.positions, "test-account")
	assert.Contains(t, rm.positions["test-account"], "BTCUSDT")
}

func TestRiskManager_GetCurrentExposure(t *testing.T) {
	rm := NewRiskManager()
	
	// Add some positions
	position1 := &types.Position{
		Symbol:     "BTCUSDT",
		Quantity:   0.5,
		MarkPrice:  40000,
	}
	
	position2 := &types.Position{
		Symbol:     "ETHUSDT",
		Quantity:   2,
		MarkPrice:  2500,
	}
	
	rm.UpdatePosition("test-account", position1)
	rm.UpdatePosition("test-account", position2)
	
	exposure := rm.GetCurrentExposure()
	
	// Expected: (0.5 * 40000) + (2 * 2500) = 20000 + 5000 = 25000
	expected := decimal.NewFromInt(25000)
	assert.True(t, exposure.Equal(expected))
}

func TestRiskManager_SettersAndGetters(t *testing.T) {
	rm := NewRiskManager()
	
	// Test SetMaxDrawdown
	rm.SetMaxDrawdown(0.15)
	assert.Equal(t, 0.15, rm.maxDrawdown)
	
	// Test SetMaxExposure
	newExposure := decimal.NewFromInt(200000)
	rm.SetMaxExposure(newExposure)
	assert.True(t, rm.maxExposure.Equal(newExposure))
	
	// Test SetMaxPositionCount
	rm.SetMaxPositionCount(20)
	assert.Equal(t, 20, rm.maxPositionCount)
	
	// Test SetAutoStopLoss
	rm.SetAutoStopLoss(true, 3.0)
	assert.True(t, rm.autoStopLoss)
	assert.Equal(t, 3.0, rm.autoStopLossPercent)
}