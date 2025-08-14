package grpc

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mExOms/oms/internal/exchange"
	"github.com/mExOms/oms/internal/risk"
	"github.com/mExOms/oms/internal/router"
	"github.com/mExOms/oms/pkg/types"
	omsv1 "github.com/mExOms/oms/pkg/proto/oms/v1"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// OrderService implements the gRPC OrderService
type OrderService struct {
	omsv1.UnimplementedOrderServiceServer
	
	exchangeFactory *exchange.Factory
	riskEngine     *risk.RiskEngine
	smartRouter    *router.SmartRouter
}

// NewOrderService creates a new order service
func NewOrderService(factory *exchange.Factory, riskEngine *risk.RiskEngine, smartRouter *router.SmartRouter) *OrderService {
	return &OrderService{
		exchangeFactory: factory,
		riskEngine:     riskEngine,
		smartRouter:    smartRouter,
	}
}

// CreateOrder creates a new order
func (s *OrderService) CreateOrder(ctx context.Context, req *omsv1.OrderRequest) (*omsv1.OrderResponse, error) {
	// Validate request
	if err := s.validateOrderRequest(req); err != nil {
		return nil, err
	}
	
	// Convert proto request to internal order type
	order := s.protoToOrder(req)
	
	// Perform risk check
	riskResult, err := s.riskEngine.CheckOrder(ctx, order, req.Exchange)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "risk check failed: %v", err)
	}
	
	if !riskResult.Passed {
		return nil, status.Errorf(codes.FailedPrecondition, "risk check failed: %s", riskResult.RejectionReason)
	}
	
	// Get exchange client
	exchangeClient, err := s.exchangeFactory.GetExchange(req.Exchange)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "exchange not found: %s", req.Exchange)
	}
	
	// Place order based on market type
	var placedOrder *types.Order
	if req.Market == omsv1.Market_MARKET_SPOT {
		placedOrder, err = exchangeClient.PlaceOrder(ctx, order)
	} else if req.Market == omsv1.Market_MARKET_FUTURES {
		// Check if exchange supports futures
		futuresClient, ok := exchangeClient.(types.FuturesExchange)
		if !ok {
			return nil, status.Errorf(codes.Unimplemented, "exchange %s does not support futures", req.Exchange)
		}
		placedOrder, err = futuresClient.PlaceFuturesOrder(ctx, order)
	}
	
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to place order: %v", err)
	}
	
	// Convert back to proto
	protoOrder := s.orderToProto(placedOrder, req.Exchange)
	
	return &omsv1.OrderResponse{
		Order:   protoOrder,
		Message: "Order placed successfully",
	}, nil
}

// CancelOrder cancels an existing order
func (s *OrderService) CancelOrder(ctx context.Context, req *omsv1.CancelOrderRequest) (*omsv1.OrderResponse, error) {
	// Validate request
	if req.Exchange == "" || req.Symbol == "" {
		return nil, status.Errorf(codes.InvalidArgument, "exchange and symbol are required")
	}
	
	if req.OrderId == "" && req.ClientOrderId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "either order_id or client_order_id is required")
	}
	
	// Get exchange client
	exchangeClient, err := s.exchangeFactory.GetExchange(req.Exchange)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "exchange not found: %s", req.Exchange)
	}
	
	// Cancel order
	orderID := req.OrderId
	if orderID == "" {
		orderID = req.ClientOrderId
	}
	
	err = exchangeClient.CancelOrder(ctx, req.Symbol, orderID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to cancel order: %v", err)
	}
	
	return &omsv1.OrderResponse{
		Message: "Order cancelled successfully",
	}, nil
}

// GetOrder retrieves order details
func (s *OrderService) GetOrder(ctx context.Context, req *omsv1.GetOrderRequest) (*omsv1.OrderResponse, error) {
	// Validate request
	if req.Exchange == "" || req.Symbol == "" {
		return nil, status.Errorf(codes.InvalidArgument, "exchange and symbol are required")
	}
	
	if req.OrderId == "" && req.ClientOrderId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "either order_id or client_order_id is required")
	}
	
	// Get exchange client
	exchangeClient, err := s.exchangeFactory.GetExchange(req.Exchange)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "exchange not found: %s", req.Exchange)
	}
	
	// Get order
	orderID := req.OrderId
	if orderID == "" {
		orderID = req.ClientOrderId
	}
	
	order, err := exchangeClient.GetOrder(ctx, req.Symbol, orderID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get order: %v", err)
	}
	
	// Convert to proto
	protoOrder := s.orderToProto(order, req.Exchange)
	
	return &omsv1.OrderResponse{
		Order: protoOrder,
	}, nil
}

// ListOrders lists orders with filters
func (s *OrderService) ListOrders(ctx context.Context, req *omsv1.ListOrdersRequest) (*omsv1.ListOrdersResponse, error) {
	// Validate request
	if req.Exchange == "" {
		return nil, status.Errorf(codes.InvalidArgument, "exchange is required")
	}
	
	// Get exchange client
	exchangeClient, err := s.exchangeFactory.GetExchange(req.Exchange)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "exchange not found: %s", req.Exchange)
	}
	
	// Get open orders
	orders, err := exchangeClient.GetOpenOrders(ctx, req.Symbol)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get orders: %v", err)
	}
	
	// Filter orders based on request
	filteredOrders := s.filterOrders(orders, req)
	
	// Convert to proto
	protoOrders := make([]*omsv1.Order, 0, len(filteredOrders))
	for _, order := range filteredOrders {
		protoOrders = append(protoOrders, s.orderToProto(order, req.Exchange))
	}
	
	return &omsv1.ListOrdersResponse{
		Orders: protoOrders,
		Total:  int32(len(protoOrders)),
	}, nil
}

// Helper methods

func (s *OrderService) validateOrderRequest(req *omsv1.OrderRequest) error {
	if req.Exchange == "" {
		return status.Errorf(codes.InvalidArgument, "exchange is required")
	}
	
	if req.Symbol == "" {
		return status.Errorf(codes.InvalidArgument, "symbol is required")
	}
	
	if req.Side == omsv1.OrderSide_ORDER_SIDE_UNSPECIFIED {
		return status.Errorf(codes.InvalidArgument, "side is required")
	}
	
	if req.Type == omsv1.OrderType_ORDER_TYPE_UNSPECIFIED {
		return status.Errorf(codes.InvalidArgument, "type is required")
	}
	
	if req.Quantity == nil || req.Quantity.Value == "" {
		return status.Errorf(codes.InvalidArgument, "quantity is required")
	}
	
	if req.Type == omsv1.OrderType_ORDER_TYPE_LIMIT && (req.Price == nil || req.Price.Value == "") {
		return status.Errorf(codes.InvalidArgument, "price is required for limit orders")
	}
	
	return nil
}

func (s *OrderService) protoToOrder(req *omsv1.OrderRequest) *types.Order {
	order := &types.Order{
		ClientOrderID: req.ClientOrderId,
		Symbol:        req.Symbol,
		Side:          s.protoToOrderSide(req.Side),
		Type:          s.protoToOrderType(req.Type),
		TimeInForce:   s.protoToTimeInForce(req.TimeInForce),
		Quantity:      s.decimalFromProto(req.Quantity),
		ReduceOnly:    req.ReduceOnly,
		PostOnly:      req.PostOnly,
	}
	
	if req.Price != nil {
		order.Price = s.decimalFromProto(req.Price)
	}
	
	if req.StopPrice != nil {
		order.StopPrice = s.decimalFromProto(req.StopPrice)
	}
	
	if req.PositionSide != "" {
		order.PositionSide = types.PositionSide(req.PositionSide)
	}
	
	// Generate client order ID if not provided
	if order.ClientOrderID == "" {
		order.ClientOrderID = fmt.Sprintf("oms_%s", uuid.New().String())
	}
	
	return order
}

func (s *OrderService) orderToProto(order *types.Order, exchange string) *omsv1.Order {
	return &omsv1.Order{
		Id:               order.OrderID,
		ClientOrderId:    order.ClientOrderID,
		Exchange:         exchange,
		Symbol:           order.Symbol,
		Side:             s.orderSideToProto(order.Side),
		Type:             s.orderTypeToProto(order.Type),
		Price:            s.decimalToProto(order.Price),
		Quantity:         s.decimalToProto(order.Quantity),
		ExecutedQuantity: s.decimalToProto(order.ExecutedQuantity),
		Status:           s.orderStatusToProto(order.Status),
		TimeInForce:      s.timeInForceToProto(order.TimeInForce),
		CreatedAt:        s.timeToProto(order.CreatedAt),
		UpdatedAt:        s.timeToProto(order.UpdatedAt),
		StopPrice:        s.decimalToProto(order.StopPrice),
		ReduceOnly:       order.ReduceOnly,
		PostOnly:         order.PostOnly,
		PositionSide:     string(order.PositionSide),
	}
}

func (s *OrderService) filterOrders(orders []*types.Order, req *omsv1.ListOrdersRequest) []*types.Order {
	filtered := make([]*types.Order, 0)
	
	for _, order := range orders {
		// Filter by status
		if req.Status != omsv1.OrderStatus_ORDER_STATUS_UNSPECIFIED {
			protoStatus := s.orderStatusToProto(order.Status)
			if protoStatus != req.Status {
				continue
			}
		}
		
		// Filter by time range
		if req.StartTime != nil {
			startTime := time.Unix(req.StartTime.Seconds, int64(req.StartTime.Nanos))
			if order.CreatedAt.Before(startTime) {
				continue
			}
		}
		
		if req.EndTime != nil {
			endTime := time.Unix(req.EndTime.Seconds, int64(req.EndTime.Nanos))
			if order.CreatedAt.After(endTime) {
				continue
			}
		}
		
		filtered = append(filtered, order)
		
		// Apply limit
		if req.Limit > 0 && len(filtered) >= int(req.Limit) {
			break
		}
	}
	
	return filtered
}

// Conversion helpers

func (s *OrderService) protoToOrderSide(side omsv1.OrderSide) types.OrderSide {
	switch side {
	case omsv1.OrderSide_ORDER_SIDE_BUY:
		return types.OrderSideBuy
	case omsv1.OrderSide_ORDER_SIDE_SELL:
		return types.OrderSideSell
	default:
		return types.OrderSideBuy
	}
}

func (s *OrderService) orderSideToProto(side types.OrderSide) omsv1.OrderSide {
	switch side {
	case types.OrderSideBuy:
		return omsv1.OrderSide_ORDER_SIDE_BUY
	case types.OrderSideSell:
		return omsv1.OrderSide_ORDER_SIDE_SELL
	default:
		return omsv1.OrderSide_ORDER_SIDE_UNSPECIFIED
	}
}

func (s *OrderService) protoToOrderType(t omsv1.OrderType) types.OrderType {
	switch t {
	case omsv1.OrderType_ORDER_TYPE_MARKET:
		return types.OrderTypeMarket
	case omsv1.OrderType_ORDER_TYPE_LIMIT:
		return types.OrderTypeLimit
	case omsv1.OrderType_ORDER_TYPE_STOP_LOSS:
		return types.OrderTypeStopLoss
	case omsv1.OrderType_ORDER_TYPE_STOP_LOSS_LIMIT:
		return types.OrderTypeStopLossLimit
	case omsv1.OrderType_ORDER_TYPE_TAKE_PROFIT:
		return types.OrderTypeTakeProfit
	case omsv1.OrderType_ORDER_TYPE_TAKE_PROFIT_LIMIT:
		return types.OrderTypeTakeProfitLimit
	case omsv1.OrderType_ORDER_TYPE_LIMIT_MAKER:
		return types.OrderTypeLimitMaker
	default:
		return types.OrderTypeLimit
	}
}

func (s *OrderService) orderTypeToProto(t types.OrderType) omsv1.OrderType {
	switch t {
	case types.OrderTypeMarket:
		return omsv1.OrderType_ORDER_TYPE_MARKET
	case types.OrderTypeLimit:
		return omsv1.OrderType_ORDER_TYPE_LIMIT
	case types.OrderTypeStopLoss:
		return omsv1.OrderType_ORDER_TYPE_STOP_LOSS
	case types.OrderTypeStopLossLimit:
		return omsv1.OrderType_ORDER_TYPE_STOP_LOSS_LIMIT
	case types.OrderTypeTakeProfit:
		return omsv1.OrderType_ORDER_TYPE_TAKE_PROFIT
	case types.OrderTypeTakeProfitLimit:
		return omsv1.OrderType_ORDER_TYPE_TAKE_PROFIT_LIMIT
	case types.OrderTypeLimitMaker:
		return omsv1.OrderType_ORDER_TYPE_LIMIT_MAKER
	default:
		return omsv1.OrderType_ORDER_TYPE_UNSPECIFIED
	}
}

func (s *OrderService) protoToTimeInForce(tif omsv1.TimeInForce) types.TimeInForce {
	switch tif {
	case omsv1.TimeInForce_TIME_IN_FORCE_GTC:
		return types.TimeInForceGTC
	case omsv1.TimeInForce_TIME_IN_FORCE_IOC:
		return types.TimeInForceIOC
	case omsv1.TimeInForce_TIME_IN_FORCE_FOK:
		return types.TimeInForceFOK
	case omsv1.TimeInForce_TIME_IN_FORCE_GTX:
		return types.TimeInForceGTX
	default:
		return types.TimeInForceGTC
	}
}

func (s *OrderService) timeInForceToProto(tif types.TimeInForce) omsv1.TimeInForce {
	switch tif {
	case types.TimeInForceGTC:
		return omsv1.TimeInForce_TIME_IN_FORCE_GTC
	case types.TimeInForceIOC:
		return omsv1.TimeInForce_TIME_IN_FORCE_IOC
	case types.TimeInForceFOK:
		return omsv1.TimeInForce_TIME_IN_FORCE_FOK
	case types.TimeInForceGTX:
		return omsv1.TimeInForce_TIME_IN_FORCE_GTX
	default:
		return omsv1.TimeInForce_TIME_IN_FORCE_UNSPECIFIED
	}
}

func (s *OrderService) orderStatusToProto(status types.OrderStatus) omsv1.OrderStatus {
	switch status {
	case types.OrderStatusNew:
		return omsv1.OrderStatus_ORDER_STATUS_NEW
	case types.OrderStatusPartiallyFilled:
		return omsv1.OrderStatus_ORDER_STATUS_PARTIALLY_FILLED
	case types.OrderStatusFilled:
		return omsv1.OrderStatus_ORDER_STATUS_FILLED
	case types.OrderStatusCanceled:
		return omsv1.OrderStatus_ORDER_STATUS_CANCELED
	case types.OrderStatusPendingCancel:
		return omsv1.OrderStatus_ORDER_STATUS_PENDING_CANCEL
	case types.OrderStatusRejected:
		return omsv1.OrderStatus_ORDER_STATUS_REJECTED
	case types.OrderStatusExpired:
		return omsv1.OrderStatus_ORDER_STATUS_EXPIRED
	default:
		return omsv1.OrderStatus_ORDER_STATUS_UNSPECIFIED
	}
}

func (s *OrderService) decimalFromProto(d *omsv1.Decimal) decimal.Decimal {
	if d == nil || d.Value == "" {
		return decimal.Zero
	}
	val, _ := decimal.NewFromString(d.Value)
	return val
}

func (s *OrderService) decimalToProto(d decimal.Decimal) *omsv1.Decimal {
	return &omsv1.Decimal{
		Value: d.String(),
	}
}

func (s *OrderService) timeToProto(t time.Time) *omsv1.Timestamp {
	return &omsv1.Timestamp{
		Seconds: t.Unix(),
		Nanos:   int32(t.Nanosecond()),
	}
}