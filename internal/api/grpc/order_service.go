package grpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/mExOms/proto/oms/v1"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// CreateOrder creates a new order
func (s *Server) CreateOrder(ctx context.Context, req *pb.OrderRequest) (*pb.OrderResponse, error) {
	s.logger.Info("CreateOrder request: %v", req)

	// Extract API key from context
	apiKey := getAPIKeyFromContext(ctx)
	if apiKey == "" {
		return nil, status.Errorf(codes.Unauthenticated, "missing authentication")
	}

	// TODO: Map API key to account
	// For now, use the account from request if provided
	accountID := req.AccountId
	if accountID == "" {
		// Select account based on strategy or use default
		account, err := s.accountManager.SelectAccount("", types.AccountRequirements{})
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "no suitable account found")
		}
		accountID = account.ID
	}

	// Create order
	order := &types.Order{
		AccountID:    accountID,
		Exchange:     req.Exchange,
		Symbol:       req.Symbol,
		Side:         convertOrderSide(req.Side),
		Type:         convertOrderType(req.Type),
		Quantity:     decimal.RequireFromString(req.Quantity),
		TimeInForce:  convertTimeInForce(req.TimeInForce),
		ClientID:     req.ClientOrderId,
	}

	if req.Price != "" {
		order.Price = decimal.RequireFromString(req.Price)
	}

	// Submit order
	result, err := s.orderManager.SubmitOrder(ctx, order)
	if err != nil {
		s.logger.Error("Failed to submit order: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to submit order: %v", err)
	}

	return &pb.OrderResponse{
		Order: convertOrderToProto(result),
	}, nil
}

// CancelOrder cancels an existing order
func (s *Server) CancelOrder(ctx context.Context, req *pb.CancelOrderRequest) (*pb.OrderResponse, error) {
	// Cancel order
	err := s.orderManager.CancelOrder(ctx, req.OrderId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to cancel order: %v", err)
	}

	// Get updated order
	order, err := s.orderManager.GetOrder(req.OrderId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "order not found")
	}

	return &pb.OrderResponse{
		Order: convertOrderToProto(order),
	}, nil
}

// GetOrder retrieves order details
func (s *Server) GetOrder(ctx context.Context, req *pb.GetOrderRequest) (*pb.OrderResponse, error) {
	order, err := s.orderManager.GetOrder(req.OrderId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "order not found")
	}

	return &pb.OrderResponse{
		Order: convertOrderToProto(order),
	}, nil
}

// ListOrders lists orders with filters
func (s *Server) ListOrders(ctx context.Context, req *pb.ListOrdersRequest) (*pb.ListOrdersResponse, error) {
	// TODO: Implement order listing with filters
	return &pb.ListOrdersResponse{
		Orders: []*pb.Order{},
	}, nil
}

// Helper functions

func convertOrderToProto(order *types.Order) *pb.Order {
	// TODO: Implement order to proto conversion
	return &pb.Order{
		OrderId:       order.ID,
		ClientOrderId: order.ClientID,
		AccountId:     order.AccountID,
		Exchange:      order.Exchange,
		Symbol:        order.Symbol,
		Side:          convertOrderSideToProto(order.Side),
		Type:          convertOrderTypeToProto(order.Type),
		Status:        convertOrderStatusToProto(order.Status),
		Price:         order.Price.String(),
		Quantity:      order.Quantity.String(),
		TimeInForce:   convertTimeInForceToProto(order.TimeInForce),
	}
}

func convertOrderSide(side pb.OrderSide) types.OrderSide {
	switch side {
	case pb.OrderSide_ORDER_SIDE_BUY:
		return types.OrderSideBuy
	case pb.OrderSide_ORDER_SIDE_SELL:
		return types.OrderSideSell
	default:
		return ""
	}
}

func convertOrderType(orderType pb.OrderType) types.OrderType {
	switch orderType {
	case pb.OrderType_ORDER_TYPE_MARKET:
		return types.OrderTypeMarket
	case pb.OrderType_ORDER_TYPE_LIMIT:
		return types.OrderTypeLimit
	case pb.OrderType_ORDER_TYPE_STOP_LOSS:
		return types.OrderTypeStopLoss
	case pb.OrderType_ORDER_TYPE_STOP_LOSS_LIMIT:
		return types.OrderTypeStopLossLimit
	case pb.OrderType_ORDER_TYPE_TAKE_PROFIT:
		return types.OrderTypeTakeProfit
	case pb.OrderType_ORDER_TYPE_TAKE_PROFIT_LIMIT:
		return types.OrderTypeTakeProfitLimit
	case pb.OrderType_ORDER_TYPE_LIMIT_MAKER:
		return types.OrderTypeLimitMaker
	default:
		return ""
	}
}

func convertTimeInForce(tif pb.TimeInForce) types.TimeInForce {
	switch tif {
	case pb.TimeInForce_TIME_IN_FORCE_GTC:
		return types.TimeInForceGTC
	case pb.TimeInForce_TIME_IN_FORCE_IOC:
		return types.TimeInForceIOC
	case pb.TimeInForce_TIME_IN_FORCE_FOK:
		return types.TimeInForceFOK
	default:
		return ""
	}
}

func convertOrderSideToProto(side types.OrderSide) pb.OrderSide {
	switch side {
	case types.OrderSideBuy:
		return pb.OrderSide_ORDER_SIDE_BUY
	case types.OrderSideSell:
		return pb.OrderSide_ORDER_SIDE_SELL
	default:
		return pb.OrderSide_ORDER_SIDE_UNSPECIFIED
	}
}

func convertOrderTypeToProto(orderType types.OrderType) pb.OrderType {
	switch orderType {
	case types.OrderTypeMarket:
		return pb.OrderType_ORDER_TYPE_MARKET
	case types.OrderTypeLimit:
		return pb.OrderType_ORDER_TYPE_LIMIT
	case types.OrderTypeStopLoss:
		return pb.OrderType_ORDER_TYPE_STOP_LOSS
	case types.OrderTypeStopLossLimit:
		return pb.OrderType_ORDER_TYPE_STOP_LOSS_LIMIT
	case types.OrderTypeTakeProfit:
		return pb.OrderType_ORDER_TYPE_TAKE_PROFIT
	case types.OrderTypeTakeProfitLimit:
		return pb.OrderType_ORDER_TYPE_TAKE_PROFIT_LIMIT
	case types.OrderTypeLimitMaker:
		return pb.OrderType_ORDER_TYPE_LIMIT_MAKER
	default:
		return pb.OrderType_ORDER_TYPE_UNSPECIFIED
	}
}

func convertOrderStatusToProto(status types.OrderStatus) pb.OrderStatus {
	switch status {
	case types.OrderStatusNew:
		return pb.OrderStatus_ORDER_STATUS_NEW
	case types.OrderStatusPartiallyFilled:
		return pb.OrderStatus_ORDER_STATUS_PARTIALLY_FILLED
	case types.OrderStatusFilled:
		return pb.OrderStatus_ORDER_STATUS_FILLED
	case types.OrderStatusCanceled:
		return pb.OrderStatus_ORDER_STATUS_CANCELED
	case types.OrderStatusPendingCancel:
		return pb.OrderStatus_ORDER_STATUS_PENDING_CANCEL
	case types.OrderStatusRejected:
		return pb.OrderStatus_ORDER_STATUS_REJECTED
	case types.OrderStatusExpired:
		return pb.OrderStatus_ORDER_STATUS_EXPIRED
	default:
		return pb.OrderStatus_ORDER_STATUS_UNSPECIFIED
	}
}

func convertTimeInForceToProto(tif types.TimeInForce) pb.TimeInForce {
	switch tif {
	case types.TimeInForceGTC:
		return pb.TimeInForce_TIME_IN_FORCE_GTC
	case types.TimeInForceIOC:
		return pb.TimeInForce_TIME_IN_FORCE_IOC
	case types.TimeInForceFOK:
		return pb.TimeInForce_TIME_IN_FORCE_FOK
	default:
		return pb.TimeInForce_TIME_IN_FORCE_UNSPECIFIED
	}
}