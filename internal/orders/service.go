package orders

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/mExOms/pkg/nats"
	"github.com/mExOms/pkg/types"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// OrderEvent represents an order event for NATS publishing
type OrderEvent struct {
	EventType   string          `json:"event_type"`
	Order       *types.Order    `json:"order"`
	Timestamp   time.Time       `json:"timestamp"`
	Exchange    string          `json:"exchange"`
	Market      string          `json:"market"`
	Symbol      string          `json:"symbol"`
	AccountID   string          `json:"account_id"`
	PrevStatus  types.OrderStatus `json:"prev_status,omitempty"`
	Reason      string          `json:"reason,omitempty"`
}

// Service manages order lifecycle and publishes events to NATS
type Service struct {
	logger       *zap.Logger
	natsClient   *nats.Client
	orderManager types.OrderManager
	exchanges    map[string]types.Exchange
	mu           sync.RWMutex
	
	// Order tracking
	activeOrders map[string]*types.Order // orderID -> order
	ordersByClientID map[string]string   // clientOrderID -> orderID
}

// NewService creates a new order service
func NewService(logger *zap.Logger, natsClient *nats.Client, orderManager types.OrderManager) *Service {
	return &Service{
		logger:           logger,
		natsClient:       natsClient,
		orderManager:     orderManager,
		exchanges:        make(map[string]types.Exchange),
		activeOrders:     make(map[string]*types.Order),
		ordersByClientID: make(map[string]string),
	}
}

// RegisterExchange registers an exchange for order placement
func (s *Service) RegisterExchange(name string, exchange types.Exchange) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exchanges[name] = exchange
	s.logger.Info("Registered exchange", zap.String("exchange", name))
}

// PlaceOrder places an order and publishes NEW event
func (s *Service) PlaceOrder(ctx context.Context, req types.OrderRequest) (*types.Order, error) {
	s.logger.Info("Placing order",
		zap.String("exchange", req.Exchange),
		zap.String("symbol", req.Symbol),
		zap.String("type", string(req.Type)),
		zap.Float64("price", req.Price),
		zap.Float64("quantity", req.Quantity))

	// Get exchange
	exchange, ok := s.exchanges[req.Exchange]
	if !ok {
		return nil, fmt.Errorf("exchange %s not registered", req.Exchange)
	}

	// Create order through OrderManager
	order, err := s.orderManager.CreateOrder(ctx, req)
	if err != nil {
		s.logger.Error("Failed to create order", zap.Error(err))
		return nil, err
	}

	// Track order
	s.mu.Lock()
	s.activeOrders[order.ID] = order
	s.ordersByClientID[order.ClientOrderID] = order.ID
	s.mu.Unlock()

	// Publish NEW order event
	event := OrderEvent{
		EventType: "NEW",
		Order:     order,
		Timestamp: time.Now(),
		Exchange:  order.Exchange,
		Market:    order.Market,
		Symbol:    order.Symbol,
		AccountID: order.AccountID,
	}
	
	if err := s.publishOrderEvent(event); err != nil {
		s.logger.Error("Failed to publish order event", zap.Error(err))
	}

	// Place order on exchange
	go s.placeOrderAsync(ctx, exchange, order)

	return order, nil
}

// placeOrderAsync places order on exchange asynchronously
func (s *Service) placeOrderAsync(ctx context.Context, exchange types.Exchange, order *types.Order) {
	req := types.OrderRequest{
		Exchange:      order.Exchange,
		Market:        order.Market,
		Symbol:        order.Symbol,
		Type:          order.Type,
		Side:          order.Side,
		Price:         order.Price,
		Quantity:      order.Quantity,
		TimeInForce:   order.TimeInForce,
		ClientOrderID: order.ClientOrderID,
	}

	// Place order on exchange
	exchangeOrder, err := exchange.PlaceOrder(ctx, req)
	if err != nil {
		s.logger.Error("Failed to place order on exchange", zap.Error(err))
		s.updateOrderStatus(order, types.OrderStatusRejected, err.Error())
		return
	}

	// Update order with exchange response
	s.mu.Lock()
	order.ExchangeOrderID = exchangeOrder.ExchangeOrderID
	order.Status = exchangeOrder.Status
	order.FilledQuantity = exchangeOrder.FilledQuantity
	order.FilledPrice = exchangeOrder.FilledPrice
	s.mu.Unlock()

	// Update in OrderManager
	if err := s.orderManager.UpdateOrder(ctx, order); err != nil {
		s.logger.Error("Failed to update order", zap.Error(err))
	}

	// Publish status update
	event := OrderEvent{
		EventType: "STATUS_UPDATE",
		Order:     order,
		Timestamp: time.Now(),
		Exchange:  order.Exchange,
		Market:    order.Market,
		Symbol:    order.Symbol,
		AccountID: order.AccountID,
	}
	s.publishOrderEvent(event)
}

// updateOrderStatus updates order status and publishes event
func (s *Service) updateOrderStatus(order *types.Order, newStatus types.OrderStatus, reason string) {
	prevStatus := order.Status
	order.Status = newStatus
	order.UpdatedAt = time.Now()

	// Determine event type
	eventType := "STATUS_UPDATE"
	switch newStatus {
	case types.OrderStatusFilled:
		eventType = "FILLED"
	case types.OrderStatusCancelled:
		eventType = "CANCELLED"
	case types.OrderStatusRejected:
		eventType = "REJECTED"
	case types.OrderStatusPartiallyFilled:
		eventType = "PARTIALLY_FILLED"
	}

	// Publish event
	event := OrderEvent{
		EventType:  eventType,
		Order:      order,
		Timestamp:  time.Now(),
		Exchange:   order.Exchange,
		Market:     order.Market,
		Symbol:     order.Symbol,
		AccountID:  order.AccountID,
		PrevStatus: prevStatus,
		Reason:     reason,
	}
	
	if err := s.publishOrderEvent(event); err != nil {
		s.logger.Error("Failed to publish order event", zap.Error(err))
	}

	// Update in OrderManager
	ctx := context.Background()
	if err := s.orderManager.UpdateOrder(ctx, order); err != nil {
		s.logger.Error("Failed to update order in manager", zap.Error(err))
	}
}

// CancelOrder cancels an order
func (s *Service) CancelOrder(ctx context.Context, orderID string) error {
	s.mu.Lock()
	order, ok := s.activeOrders[orderID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("order %s not found", orderID)
	}
	s.mu.Unlock()

	// Get exchange
	exchange, ok := s.exchanges[order.Exchange]
	if !ok {
		return fmt.Errorf("exchange %s not registered", order.Exchange)
	}

	// Cancel on exchange
	if err := exchange.CancelOrder(ctx, order.ExchangeOrderID, order.Symbol); err != nil {
		return fmt.Errorf("failed to cancel order: %w", err)
	}

	// Update status
	s.updateOrderStatus(order, types.OrderStatusCancelled, "User requested")

	return nil
}

// GetOrder retrieves an order by ID
func (s *Service) GetOrder(ctx context.Context, orderID string) (*types.Order, error) {
	s.mu.RLock()
	order, ok := s.activeOrders[orderID]
	s.mu.RUnlock()
	
	if ok {
		return order, nil
	}

	// Try to get from OrderManager
	return s.orderManager.GetOrder(ctx, orderID)
}

// GetActiveOrders returns all active orders
func (s *Service) GetActiveOrders() []*types.Order {
	s.mu.RLock()
	defer s.mu.RUnlock()

	orders := make([]*types.Order, 0, len(s.activeOrders))
	for _, order := range s.activeOrders {
		if order.Status == types.OrderStatusNew || 
		   order.Status == types.OrderStatusPartiallyFilled {
			orders = append(orders, order)
		}
	}
	return orders
}

// publishOrderEvent publishes order event to NATS
func (s *Service) publishOrderEvent(event OrderEvent) error {
	// Subject format: order.event.{exchange}.{market}.{symbol}
	subject := fmt.Sprintf("order.event.%s.%s.%s", event.Exchange, event.Market, event.Symbol)

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	return s.natsClient.Publish(subject, data)
}

// HandleOrderUpdate handles order updates from exchanges
func (s *Service) HandleOrderUpdate(update *types.Order) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find order by exchange order ID or client order ID
	var order *types.Order
	for _, o := range s.activeOrders {
		if o.ExchangeOrderID == update.ExchangeOrderID || 
		   o.ClientOrderID == update.ClientOrderID {
			order = o
			break
		}
	}

	if order == nil {
		s.logger.Warn("Order not found for update", 
			zap.String("exchangeOrderID", update.ExchangeOrderID))
		return
	}

	// Update order fields
	prevStatus := order.Status
	order.Status = update.Status
	order.FilledQuantity = update.FilledQuantity
	order.FilledPrice = update.FilledPrice
	order.UpdatedAt = time.Now()

	// Remove from active orders if final status
	if order.Status == types.OrderStatusFilled || 
	   order.Status == types.OrderStatusCancelled || 
	   order.Status == types.OrderStatusRejected {
		delete(s.activeOrders, order.ID)
		delete(s.ordersByClientID, order.ClientOrderID)
	}

	s.mu.Unlock()
	
	// Update status and publish event
	s.updateOrderStatus(order, update.Status, "Exchange update")
}

// Start starts the order service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting order service")
	
	// Subscribe to order updates from exchanges
	sub, err := s.natsClient.Subscribe("order.update.*", func(msg *nats.Msg) {
		var update types.Order
		if err := json.Unmarshal(msg.Data, &update); err != nil {
			s.logger.Error("Failed to unmarshal order update", zap.Error(err))
			return
		}
		s.HandleOrderUpdate(&update)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to order updates: %w", err)
	}

	// Wait for context cancellation
	<-ctx.Done()
	
	// Cleanup
	if err := sub.Unsubscribe(); err != nil {
		s.logger.Error("Failed to unsubscribe", zap.Error(err))
	}

	return nil
}