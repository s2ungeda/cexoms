package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mExOms/internal/monitor"
	"github.com/mExOms/internal/orders"
	"github.com/mExOms/internal/position"
	"github.com/mExOms/internal/risk"
	"github.com/mExOms/pkg/nats"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	// Initialize NATS client
	natsClient, err := nats.NewClient(nats.Config{
		URL:           "nats://localhost:4222",
		MaxReconnects: 5,
	}, logger)
	if err != nil {
		logger.Fatal("Failed to connect to NATS", zap.Error(err))
	}
	defer natsClient.Close()

	// Initialize services with NATS publishing
	logger.Info("Initializing services with NATS publishing...")

	// 1. Initialize Health Monitor
	healthChecker := monitor.NewHealthChecker("v1.0.0", natsClient, logger)
	
	// Register health checks
	healthChecker.RegisterCheck("nats", monitor.NATSHealthCheck("nats://localhost:4222"))
	healthChecker.RegisterCheck("memory", monitor.MemoryHealthCheck(80))
	healthChecker.RegisterCheck("position_manager", monitor.PositionManagerHealthCheck())
	healthChecker.RegisterCheck("risk_engine", monitor.RiskEngineHealthCheck())
	healthChecker.RegisterCheck("binance", monitor.ExchangeHealthCheck("binance"))

	// 2. Initialize Position Manager
	positionManager, err := position.NewPositionManager("/tmp/oms/snapshots", natsClient, logger)
	if err != nil {
		logger.Fatal("Failed to initialize position manager", zap.Error(err))
	}
	defer positionManager.Close()

	// 3. Initialize Risk Manager
	riskManager := risk.NewRiskManager(natsClient, logger)
	defer riskManager.Stop()

	// 4. Initialize Order Service
	// Note: This requires an OrderManager implementation which we'll mock for this example
	mockOrderManager := &MockOrderManager{}
	orderService := orders.NewService(logger, natsClient, mockOrderManager)
	
	// Register a mock exchange
	mockExchange := &MockExchange{}
	orderService.RegisterExchange("binance", mockExchange)

	// Start order service
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	go func() {
		if err := orderService.Start(ctx); err != nil {
			logger.Error("Order service error", zap.Error(err))
		}
	}()

	// Simulate trading activity
	logger.Info("Starting simulated trading activity...")
	go simulateTradingActivity(orderService, positionManager, riskManager, logger)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	<-sigChan
	logger.Info("Shutting down...")

	// Stop all services
	healthChecker.Stop()
	cancel()
}

func simulateTradingActivity(
	orderService *orders.Service,
	positionManager *position.PositionManager,
	riskManager risk.Manager,
	logger *zap.Logger,
) {
	// Simulate account balance
	riskManager.UpdateBalance("binance_main", decimal.NewFromInt(10000))

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	symbols := []string{"BTCUSDT", "ETHUSDT", "BNBUSDT"}
	orderCount := 0

	for {
		select {
		case <-ticker.C:
			// Rotate through symbols
			symbol := symbols[orderCount%len(symbols)]
			orderCount++

			// Place a simulated order
			orderReq := types.OrderRequest{
				Exchange:    "binance",
				Market:      "spot",
				Symbol:      symbol,
				Type:        types.OrderTypeLimit,
				Side:        types.OrderSideBuy,
				Price:       getSimulatedPrice(symbol),
				Quantity:    decimal.NewFromFloat(0.01),
				TimeInForce: types.TimeInForceGTC,
				AccountID:   "binance_main",
			}

			logger.Info("Placing order", 
				zap.String("symbol", symbol),
				zap.String("side", string(orderReq.Side)),
				zap.String("price", orderReq.Price.String()))

			order, err := orderService.PlaceOrder(context.Background(), orderReq)
			if err != nil {
				logger.Error("Failed to place order", zap.Error(err))
				continue
			}

			// Simulate position update
			go func(ord *types.Order) {
				time.Sleep(2 * time.Second)
				
				pos := &position.Position{
					Symbol:     ord.Symbol,
					Exchange:   ord.Exchange,
					Market:     ord.Market,
					Side:       string(ord.Side),
					Quantity:   ord.Quantity,
					EntryPrice: ord.Price,
					MarkPrice:  ord.Price.Mul(decimal.NewFromFloat(1.001)), // Simulate slight profit
					Leverage:   1,
					MarginUsed: ord.Quantity.Mul(ord.Price),
				}
				
				if err := positionManager.UpdatePosition(pos); err != nil {
					logger.Error("Failed to update position", zap.Error(err))
				}

				// Update risk manager position tracking
				riskPos := &types.Position{
					Symbol:    ord.Symbol,
					Amount:    ord.Quantity,
					MarkPrice: pos.MarkPrice,
				}
				riskManager.UpdatePosition("binance_main", riskPos)
			}(order)
		}
	}
}

func getSimulatedPrice(symbol string) decimal.Decimal {
	// Simulate realistic prices
	switch symbol {
	case "BTCUSDT":
		return decimal.NewFromInt(45000)
	case "ETHUSDT":
		return decimal.NewFromInt(2500)
	case "BNBUSDT":
		return decimal.NewFromInt(300)
	default:
		return decimal.NewFromInt(100)
	}
}

// Mock implementations for demo

type MockOrderManager struct{}

func (m *MockOrderManager) CreateOrder(ctx context.Context, req types.OrderRequest) (*types.Order, error) {
	return &types.Order{
		ID:            generateOrderID(),
		ClientOrderID: generateClientOrderID(),
		Exchange:      req.Exchange,
		Market:        req.Market,
		Symbol:        req.Symbol,
		Type:          req.Type,
		Side:          req.Side,
		Price:         req.Price,
		Quantity:      req.Quantity,
		Status:        types.OrderStatusNew,
		TimeInForce:   req.TimeInForce,
		AccountID:     req.AccountID,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}, nil
}

func (m *MockOrderManager) UpdateOrder(ctx context.Context, order *types.Order) error {
	return nil
}

func (m *MockOrderManager) GetOrder(ctx context.Context, orderID string) (*types.Order, error) {
	return nil, nil
}

func (m *MockOrderManager) GetOrders(ctx context.Context, filter types.OrderFilter) ([]*types.Order, error) {
	return nil, nil
}

func (m *MockOrderManager) CancelOrder(ctx context.Context, orderID string) error {
	return nil
}

type MockExchange struct{}

func (m *MockExchange) GetName() string {
	return "binance"
}

func (m *MockExchange) GetType() types.ExchangeType {
	return types.ExchangeTypeCEX
}

func (m *MockExchange) IsConnected() bool {
	return true
}

func (m *MockExchange) Connect(ctx context.Context) error {
	return nil
}

func (m *MockExchange) Disconnect() error {
	return nil
}

func (m *MockExchange) SubscribeMarketData(ctx context.Context, symbols []string) error {
	return nil
}

func (m *MockExchange) UnsubscribeMarketData(ctx context.Context, symbols []string) error {
	return nil
}

func (m *MockExchange) GetOrderBook(ctx context.Context, symbol string) (*types.OrderBook, error) {
	return nil, nil
}

func (m *MockExchange) GetTicker(ctx context.Context, symbol string) (*types.Ticker, error) {
	return nil, nil
}

func (m *MockExchange) PlaceOrder(ctx context.Context, order types.OrderRequest) (*types.Order, error) {
	// Simulate order placement
	return &types.Order{
		ExchangeOrderID: generateExchangeOrderID(),
		Status:          types.OrderStatusFilled,
		FilledQuantity:  order.Quantity,
		FilledPrice:     order.Price,
	}, nil
}

func (m *MockExchange) CancelOrder(ctx context.Context, orderID string, symbol string) error {
	return nil
}

func (m *MockExchange) GetOrder(ctx context.Context, orderID string, symbol string) (*types.Order, error) {
	return nil, nil
}

func (m *MockExchange) GetOpenOrders(ctx context.Context, symbol string) ([]*types.Order, error) {
	return nil, nil
}

func (m *MockExchange) GetOrderHistory(ctx context.Context, symbol string, limit int) ([]*types.Order, error) {
	return nil, nil
}

func (m *MockExchange) GetBalance(ctx context.Context) (map[string]*types.Balance, error) {
	return nil, nil
}

func (m *MockExchange) GetPositions(ctx context.Context) ([]*types.Position, error) {
	return nil, nil
}

func (m *MockExchange) GetAccountInfo(ctx context.Context) (*types.AccountInfo, error) {
	return nil, nil
}

func (m *MockExchange) GetExchangeInfo(ctx context.Context) (*types.ExchangeInfo, error) {
	return nil, nil
}

func (m *MockExchange) GetSymbolInfo(ctx context.Context, symbol string) (*types.SymbolInfo, error) {
	return nil, nil
}

func (m *MockExchange) GetFees(ctx context.Context) (*types.TradingFees, error) {
	return nil, nil
}

// Helper functions
var orderCounter int

func generateOrderID() string {
	orderCounter++
	return fmt.Sprintf("ORD_%d_%d", time.Now().Unix(), orderCounter)
}

func generateClientOrderID() string {
	return fmt.Sprintf("CLI_%d_%d", time.Now().Unix(), orderCounter)
}

func generateExchangeOrderID() string {
	return fmt.Sprintf("EX_%d_%d", time.Now().Unix(), orderCounter)
}