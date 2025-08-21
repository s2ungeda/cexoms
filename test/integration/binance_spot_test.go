package integration

import (
	"context"
	"testing"
	"time"

	"github.com/mExOms/internal/exchange"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBinanceSpotIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create exchange factory
	factory := exchange.NewFactory()
	
	// Create Binance Spot exchange
	config := map[string]interface{}{
		"exchange": "binance",
		"market":   "spot",
		"testnet":  true,
		"apiKey":   "test_key",
		"apiSecret": "test_secret",
	}
	
	ex, err := factory.CreateExchange(config)
	require.NoError(t, err)
	require.NotNil(t, ex)
	
	ctx := context.Background()
	
	t.Run("BasicInfo", func(t *testing.T) {
		assert.Equal(t, "binance", ex.GetName())
		assert.Equal(t, types.ExchangeBinance, ex.GetType())
		assert.Equal(t, types.MarketTypeSpot, ex.GetMarketType())
	})
	
	t.Run("Initialize", func(t *testing.T) {
		err := ex.Initialize(ctx)
		// May fail without real API keys, but should not panic
		t.Logf("Initialize result: %v", err)
	})
	
	t.Run("SymbolInfo", func(t *testing.T) {
		info, err := ex.GetSymbolInfo(ctx, "BTCUSDT")
		if err != nil {
			t.Logf("GetSymbolInfo error (expected without API keys): %v", err)
			return
		}
		
		assert.Equal(t, "BTCUSDT", info.Symbol)
		assert.Equal(t, "BTC", info.BaseAsset)
		assert.Equal(t, "USDT", info.QuoteAsset)
		assert.True(t, info.MinQty.GreaterThan(decimal.Zero))
		assert.True(t, info.TickSize.GreaterThan(decimal.Zero))
	})
	
	t.Run("MarketData", func(t *testing.T) {
		symbols := []string{"BTCUSDT", "ETHUSDT"}
		data, err := ex.GetMarketData(ctx, symbols)
		if err != nil {
			t.Logf("GetMarketData error (expected without API keys): %v", err)
			return
		}
		
		assert.Len(t, data, 2)
		if btc, ok := data["BTCUSDT"]; ok {
			assert.Equal(t, "BTCUSDT", btc.Symbol)
			assert.True(t, btc.Price.GreaterThan(decimal.Zero))
			assert.True(t, btc.Volume24h.GreaterThan(decimal.Zero))
		}
	})
	
	t.Run("OrderBook", func(t *testing.T) {
		book, err := ex.GetOrderBook(ctx, "BTCUSDT", 10)
		if err != nil {
			t.Logf("GetOrderBook error (expected without API keys): %v", err)
			return
		}
		
		assert.Equal(t, "BTCUSDT", book.Symbol)
		assert.NotEmpty(t, book.Bids)
		assert.NotEmpty(t, book.Asks)
		assert.True(t, len(book.Bids) <= 10)
		assert.True(t, len(book.Asks) <= 10)
		
		// Verify bid/ask ordering
		if len(book.Bids) > 1 {
			assert.True(t, book.Bids[0].Price.GreaterThan(book.Bids[1].Price))
		}
		if len(book.Asks) > 1 {
			assert.True(t, book.Asks[0].Price.LessThan(book.Asks[1].Price))
		}
	})
	
	t.Run("Klines", func(t *testing.T) {
		klines, err := ex.GetKlines(ctx, "BTCUSDT", types.KlineInterval1h, 24)
		if err != nil {
			t.Logf("GetKlines error (expected without API keys): %v", err)
			return
		}
		
		assert.NotEmpty(t, klines)
		assert.True(t, len(klines) <= 24)
		
		// Verify kline data
		for _, k := range klines {
			assert.True(t, k.High.GreaterThanOrEqual(k.Low))
			assert.True(t, k.High.GreaterThanOrEqual(k.Open))
			assert.True(t, k.High.GreaterThanOrEqual(k.Close))
			assert.True(t, k.Low.LessThanOrEqual(k.Open))
			assert.True(t, k.Low.LessThanOrEqual(k.Close))
			assert.True(t, k.Volume.GreaterThanOrEqual(decimal.Zero))
		}
	})
	
	t.Run("OrderPlacement", func(t *testing.T) {
		// This will fail without real API keys
		order := &types.Order{
			ClientOrderID: "test_order_001",
			Symbol:        "BTCUSDT",
			Side:          types.OrderSideBuy,
			Type:          types.OrderTypeLimit,
			Quantity:      decimal.NewFromFloat(0.001),
			Price:         decimal.NewFromInt(30000), // Low price to avoid execution
			TimeInForce:   types.TimeInForceGTC,
		}
		
		placedOrder, err := ex.PlaceOrder(ctx, order)
		if err != nil {
			t.Logf("PlaceOrder error (expected without API keys): %v", err)
			return
		}
		
		assert.NotEmpty(t, placedOrder.ExchangeOrderID)
		assert.Equal(t, types.OrderStatusNew, placedOrder.Status)
		
		// Try to cancel
		err = ex.CancelOrder(ctx, order.Symbol, placedOrder.ExchangeOrderID)
		t.Logf("CancelOrder result: %v", err)
	})
}

func TestBinanceSpotWebSocket(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping WebSocket test in short mode")
	}
	
	// This test would verify WebSocket functionality
	// but requires actual connection to Binance
	t.Skip("WebSocket test requires live connection")
}