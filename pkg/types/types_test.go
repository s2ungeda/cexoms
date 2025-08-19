package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/shopspring/decimal"
)

func TestOrderValidation(t *testing.T) {
	tests := []struct {
		name    string
		order   Order
		wantErr bool
	}{
		{
			name: "valid market order",
			order: Order{
				ID:       "test-123",
				Symbol:   "BTCUSDT",
				Side:     SideBuy,
				Type:     OrderTypeMarket,
				Quantity: decimal.NewFromFloat(0.001),
				Status:   OrderStatusNew,
			},
			wantErr: false,
		},
		{
			name: "valid limit order",
			order: Order{
				ID:       "test-124",
				Symbol:   "BTCUSDT", 
				Side:     SideSell,
				Type:     OrderTypeLimit,
				Quantity: decimal.NewFromFloat(0.001),
				Price:    decimal.NewFromFloat(50000),
				Status:   OrderStatusNew,
			},
			wantErr: false,
		},
		{
			name: "invalid - missing symbol",
			order: Order{
				ID:       "test-125",
				Side:     SideBuy,
				Type:     OrderTypeMarket,
				Quantity: decimal.NewFromFloat(0.001),
			},
			wantErr: true,
		},
		{
			name: "invalid - zero quantity",
			order: Order{
				ID:       "test-126",
				Symbol:   "BTCUSDT",
				Side:     SideBuy,
				Type:     OrderTypeMarket,
				Quantity: decimal.Zero,
			},
			wantErr: true,
		},
		{
			name: "invalid - limit order without price",
			order: Order{
				ID:       "test-127",
				Symbol:   "BTCUSDT",
				Side:     SideBuy,
				Type:     OrderTypeLimit,
				Quantity: decimal.NewFromFloat(0.001),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.order.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPositionPnLCalculation(t *testing.T) {
	position := &Position{
		Symbol:   "BTCUSDT",
		Side:     PositionSideLong,
		Quantity: decimal.NewFromFloat(1.0),
		AvgPrice: decimal.NewFromFloat(40000),
	}

	// Test unrealized PnL
	currentPrice := decimal.NewFromFloat(45000)
	pnl := position.CalculateUnrealizedPnL(currentPrice)
	assert.Equal(t, "5000", pnl.String())

	// Test with short position
	position.Side = PositionSideShort
	pnl = position.CalculateUnrealizedPnL(currentPrice)
	assert.Equal(t, "-5000", pnl.String())
}

func TestAccountTypeString(t *testing.T) {
	tests := []struct {
		accountType AccountType
		expected    string
	}{
		{AccountTypeMain, "main"},
		{AccountTypeSub, "sub"},
		{AccountTypeStrategy, "strategy"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.accountType.String())
	}
}

func TestOrderbookUpdate(t *testing.T) {
	ob := &Orderbook{
		Symbol:    "BTCUSDT",
		Timestamp: time.Now(),
	}

	// Add bids
	ob.UpdateBid(decimal.NewFromFloat(40000), decimal.NewFromFloat(1.0))
	ob.UpdateBid(decimal.NewFromFloat(39999), decimal.NewFromFloat(2.0))
	ob.UpdateBid(decimal.NewFromFloat(39998), decimal.NewFromFloat(3.0))

	// Add asks
	ob.UpdateAsk(decimal.NewFromFloat(40001), decimal.NewFromFloat(1.0))
	ob.UpdateAsk(decimal.NewFromFloat(40002), decimal.NewFromFloat(2.0))
	ob.UpdateAsk(decimal.NewFromFloat(40003), decimal.NewFromFloat(3.0))

	// Test best bid/ask
	bestBid, bestAsk := ob.GetBestBidAsk()
	assert.Equal(t, "40000", bestBid.Price.String())
	assert.Equal(t, "40001", bestAsk.Price.String())

	// Test spread
	spread := ob.GetSpread()
	assert.Equal(t, "1", spread.String())
}

func TestNormalizeSymbol(t *testing.T) {
	tests := []struct {
		exchange string
		symbol   string
		expected string
	}{
		{"binance", "BTCUSDT", "BTC-USDT"},
		{"binance", "ETHUSDT", "ETH-USDT"},
		{"okx", "BTC-USDT", "BTC-USDT"},
		{"bybit", "BTCUSDT", "BTC-USDT"},
	}

	for _, tt := range tests {
		result := NormalizeSymbol(tt.exchange, tt.symbol)
		assert.Equal(t, tt.expected, result)
	}
}