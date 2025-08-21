package binance

import (
	"context"
	"testing"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockWebSocketConn is a mock WebSocket connection
type MockWebSocketConn struct {
	mock.Mock
}

func TestBinanceWSOrderManager_Connect(t *testing.T) {
	config := types.WebSocketConfig{
		URL:               "wss://ws-api.binance.com:443/ws-api/v3",
		APIKey:            "test-api-key",
		SecretKey:         "test-secret-key",
		ReconnectInterval: 5 * time.Second,
		EnableHeartbeat:   true,
		PingInterval:      30 * time.Second,
	}
	
	manager := NewBinanceWSOrderManager(config)
	
	// Test connection (will fail without real credentials)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	err := manager.Connect(ctx)
	assert.Error(t, err) // Expected to fail without real WebSocket server
}

func TestBinanceWSOrderManager_IsConnected(t *testing.T) {
	config := types.WebSocketConfig{
		URL:       "wss://ws-api.binance.com:443/ws-api/v3",
		APIKey:    "test-api-key",
		SecretKey: "test-secret-key",
	}
	
	manager := NewBinanceWSOrderManager(config)
	
	// Initially not connected
	assert.False(t, manager.IsConnected())
}

func TestBinanceWSOrderManager_CreateOrder(t *testing.T) {
	config := types.WebSocketConfig{
		URL:       "wss://ws-api.binance.com:443/ws-api/v3",
		APIKey:    "test-api-key",
		SecretKey: "test-secret-key",
	}
	
	manager := NewBinanceWSOrderManager(config)
	
	// Test order creation without connection
	order := &types.Order{
		Symbol:      "BTCUSDT",
		Side:        types.OrderSideBuy,
		Type:        types.OrderTypeLimit,
		Quantity:    decimal.NewFromFloat(0.001),
		Price:       decimal.NewFromFloat(40000),
		TimeInForce: types.TimeInForceGTC,
	}
	
	ctx := context.Background()
	_, err := manager.CreateOrder(ctx, order)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "WebSocket not connected")
}

func TestBinanceWSOrderManager_GetMetrics(t *testing.T) {
	config := types.WebSocketConfig{
		URL:       "wss://ws-api.binance.com:443/ws-api/v3",
		APIKey:    "test-api-key",
		SecretKey: "test-secret-key",
	}
	
	manager := NewBinanceWSOrderManager(config)
	
	metrics := manager.GetMetrics()
	assert.NotNil(t, metrics)
	assert.False(t, metrics.Connected)
	assert.Equal(t, uint64(0), metrics.MessagesSent)
	assert.Equal(t, uint64(0), metrics.MessagesReceived)
}

func TestBinanceWSOrderManager_GenerateSignature(t *testing.T) {
	config := types.WebSocketConfig{
		URL:       "wss://ws-api.binance.com:443/ws-api/v3",
		APIKey:    "test-api-key",
		SecretKey: "NhqPtmdSJYdKjVHjA7PZj4Mge3R5YNiP1e3UZjInClVN65XAbvqqM6A7H5fATj0j",
	}
	
	manager := NewBinanceWSOrderManager(config)
	
	params := map[string]interface{}{
		"symbol":    "BTCUSDT",
		"side":      "BUY",
		"type":      "LIMIT",
		"quantity":  "0.001",
		"price":     "40000",
		"timestamp": int64(1234567890123),
	}
	
	signature := manager.generateSignature(params)
	assert.NotEmpty(t, signature)
	// Known signature for these params with the test secret
	assert.Equal(t, "7c5a96c8681c79e107285c1ce6a51c44e36e9c6b91aeb7bb0ee90c95d6b7a84e", signature)
}