package grpc

import (
	"context"

	"github.com/mExOms/internal/position"
	omsv1 "github.com/mExOms/pkg/proto/oms/v1"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PositionService implements the gRPC PositionService
type PositionService struct {
	omsv1.UnimplementedPositionServiceServer
	
	positionManager *position.PositionManager
}

// NewPositionService creates a new position service
func NewPositionService(positionManager *position.PositionManager) *PositionService {
	return &PositionService{
		positionManager: positionManager,
	}
}

// GetPosition retrieves a specific position
func (s *PositionService) GetPosition(ctx context.Context, req *omsv1.GetPositionRequest) (*omsv1.GetPositionResponse, error) {
	if req.Exchange == "" || req.Symbol == "" {
		return nil, status.Errorf(codes.InvalidArgument, "exchange and symbol are required")
	}
	
	pos, exists := s.positionManager.GetPosition(req.Exchange, req.Symbol)
	if !exists {
		return nil, status.Errorf(codes.NotFound, "position not found")
	}
	
	return &omsv1.GetPositionResponse{
		Position: s.positionToProto(pos),
	}, nil
}

// ListPositions lists all positions
func (s *PositionService) ListPositions(ctx context.Context, req *omsv1.ListPositionsRequest) (*omsv1.ListPositionsResponse, error) {
	var positions []*position.Position
	
	if req.Exchange != "" {
		// Get positions for specific exchange
		positions = s.positionManager.GetPositionsByExchange(req.Exchange)
	} else {
		// Get all positions
		positions = s.positionManager.GetAllPositions()
	}
	
	// Filter by market if specified
	if req.Market != omsv1.Market_MARKET_UNSPECIFIED {
		filtered := make([]*position.Position, 0)
		marketStr := s.protoToMarketString(req.Market)
		
		for _, pos := range positions {
			if pos.Market == marketStr {
				filtered = append(filtered, pos)
			}
		}
		positions = filtered
	}
	
	// Convert to proto
	protoPositions := make([]*omsv1.Position, 0, len(positions))
	for _, pos := range positions {
		protoPositions = append(protoPositions, s.positionToProto(pos))
	}
	
	return &omsv1.ListPositionsResponse{
		Positions: protoPositions,
		Total:     int32(len(protoPositions)),
	}, nil
}

// GetAggregatedPositions returns aggregated positions across exchanges
func (s *PositionService) GetAggregatedPositions(ctx context.Context, req *omsv1.GetAggregatedPositionsRequest) (*omsv1.GetAggregatedPositionsResponse, error) {
	aggregated := s.positionManager.GetAggregatedPositions()
	
	// Filter by symbols if specified
	symbolSet := make(map[string]bool)
	if len(req.Symbols) > 0 {
		for _, symbol := range req.Symbols {
			symbolSet[symbol] = true
		}
	}
	
	// Convert to proto
	protoAggregated := make([]*omsv1.AggregatedPosition, 0)
	
	for symbol, agg := range aggregated {
		// Skip if filtering by symbols and this symbol is not in the list
		if len(symbolSet) > 0 && !symbolSet[symbol] {
			continue
		}
		
		// Convert positions
		protoPositions := make([]*omsv1.Position, 0, len(agg.Positions))
		for _, pos := range agg.Positions {
			protoPositions = append(protoPositions, s.positionToProto(pos))
		}
		
		protoAggregated = append(protoAggregated, &omsv1.AggregatedPosition{
			Symbol:         agg.Symbol,
			TotalQuantity:  s.decimalToProto(agg.TotalQuantity),
			AvgEntryPrice:  s.decimalToProto(agg.AvgEntryPrice),
			TotalValue:     s.decimalToProto(agg.TotalValue),
			TotalPnl:       s.decimalToProto(agg.TotalPnL),
			Positions:      protoPositions,
		})
	}
	
	return &omsv1.GetAggregatedPositionsResponse{
		Positions: protoAggregated,
	}, nil
}

// GetRiskMetrics returns risk-related metrics
func (s *PositionService) GetRiskMetrics(ctx context.Context, req *omsv1.GetRiskMetricsRequest) (*omsv1.GetRiskMetricsResponse, error) {
	metrics := s.positionManager.GetRiskMetrics()
	
	// Convert metrics to proto
	protoMetrics := &omsv1.RiskMetrics{
		PositionCount:   int32(metrics["position_count"].(int)),
		UpdatesCount:    metrics["updates_count"].(uint64),
		ReadsCount:      metrics["reads_count"].(uint64),
		AvgCalcTimeUs:   metrics["avg_calc_time_us"].(float64),
	}
	
	// Parse decimal values
	if totalValue, ok := metrics["total_value"].(string); ok {
		protoMetrics.TotalValue = &omsv1.Decimal{Value: totalValue}
	}
	
	if totalMargin, ok := metrics["total_margin_used"].(string); ok {
		protoMetrics.TotalMarginUsed = &omsv1.Decimal{Value: totalMargin}
	}
	
	if maxLeverage, ok := metrics["max_leverage"].(string); ok {
		protoMetrics.MaxLeverage = &omsv1.Decimal{Value: maxLeverage}
	}
	
	if unrealizedPnl, ok := metrics["unrealized_pnl"].(string); ok {
		protoMetrics.UnrealizedPnl = &omsv1.Decimal{Value: unrealizedPnl}
	}
	
	if realizedPnl, ok := metrics["realized_pnl"].(string); ok {
		protoMetrics.RealizedPnl = &omsv1.Decimal{Value: realizedPnl}
	}
	
	if totalPnl, ok := metrics["total_pnl"].(string); ok {
		protoMetrics.TotalPnl = &omsv1.Decimal{Value: totalPnl}
	}
	
	return &omsv1.GetRiskMetricsResponse{
		Metrics: protoMetrics,
	}, nil
}

// Helper methods

func (s *PositionService) positionToProto(pos *position.Position) *omsv1.Position {
	return &omsv1.Position{
		Symbol:        pos.Symbol,
		Exchange:      pos.Exchange,
		Market:        s.marketStringToProto(pos.Market),
		Side:          pos.Side,
		Quantity:      s.decimalToProto(pos.Quantity),
		EntryPrice:    s.decimalToProto(pos.EntryPrice),
		MarkPrice:     s.decimalToProto(pos.MarkPrice),
		UnrealizedPnl: s.decimalToProto(pos.UnrealizedPnL),
		RealizedPnl:   s.decimalToProto(pos.RealizedPnL),
		Leverage:      int32(pos.Leverage),
		MarginUsed:    s.decimalToProto(pos.MarginUsed),
		UpdatedAt:     s.timeToProto(pos.UpdatedAt),
		PositionValue: s.decimalToProto(pos.PositionValue),
		PnlPercent:    s.decimalToProto(pos.PnLPercent),
		MarginRatio:   s.decimalToProto(pos.MarginRatio),
	}
}

func (s *PositionService) decimalToProto(d decimal.Decimal) *omsv1.Decimal {
	return &omsv1.Decimal{
		Value: d.String(),
	}
}

func (s *PositionService) marketStringToProto(market string) omsv1.Market {
	switch market {
	case "spot":
		return omsv1.Market_MARKET_SPOT
	case "futures":
		return omsv1.Market_MARKET_FUTURES
	default:
		return omsv1.Market_MARKET_UNSPECIFIED
	}
}

func (s *PositionService) protoToMarketString(market omsv1.Market) string {
	switch market {
	case omsv1.Market_MARKET_SPOT:
		return "spot"
	case omsv1.Market_MARKET_FUTURES:
		return "futures"
	default:
		return ""
	}
}