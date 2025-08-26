package grpc

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/mExOms/proto/oms/v1"
	"github.com/mExOms/pkg/types"
)

// StrategyService implements the strategy gRPC service
type StrategyService struct {
	pb.UnimplementedStrategyServiceServer
	
	strategyManager types.StrategyManager
	authManager     *AuthManager
}

// NewStrategyService creates a new strategy service
func NewStrategyService(strategyManager types.StrategyManager, authManager *AuthManager) *StrategyService {
	return &StrategyService{
		strategyManager: strategyManager,
		authManager:     authManager,
	}
}

// CreateStrategy creates a new trading strategy
func (s *StrategyService) CreateStrategy(ctx context.Context, req *pb.CreateStrategyRequest) (*pb.CreateStrategyResponse, error) {
	// Check permissions
	claims, err := ExtractClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	
	if !s.authManager.CheckPermission(claims, pb.Permission_PERMISSION_MANAGE_STRATEGIES, "") {
		return nil, status.Error(codes.PermissionDenied, "insufficient permissions")
	}
	
	// Validate request
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "strategy name is required")
	}
	
	if req.Type == pb.StrategyType_STRATEGY_TYPE_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "strategy type is required")
	}
	
	// Create strategy
	strategy := &types.Strategy{
		Name:        req.Name,
		Description: req.Description,
		Type:        convertStrategyType(req.Type),
		Accounts:    req.Accounts,
		Config:      convertStrategyConfig(req.Config),
		Status:      types.StrategyStatusStopped,
	}
	
	createdStrategy, err := s.strategyManager.CreateStrategy(ctx, strategy)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create strategy: %v", err)
	}
	
	return &pb.CreateStrategyResponse{
		Strategy: convertStrategyToProto(createdStrategy),
	}, nil
}

// GetStrategy retrieves a strategy by ID
func (s *StrategyService) GetStrategy(ctx context.Context, req *pb.GetStrategyRequest) (*pb.GetStrategyResponse, error) {
	// Check permissions
	claims, err := ExtractClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	
	if !s.authManager.CheckPermission(claims, pb.Permission_PERMISSION_READ_STRATEGIES, "") {
		return nil, status.Error(codes.PermissionDenied, "insufficient permissions")
	}
	
	// Get strategy
	strategy, err := s.strategyManager.GetStrategy(ctx, req.StrategyId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "strategy not found: %v", err)
	}
	
	// Check if user has access to this strategy's accounts
	hasAccess := false
	for _, accountID := range strategy.Accounts {
		if s.authManager.CheckPermission(claims, pb.Permission_PERMISSION_READ_ACCOUNTS, accountID) {
			hasAccess = true
			break
		}
	}
	
	if !hasAccess && !s.authManager.CheckPermission(claims, pb.Permission_PERMISSION_ADMIN, "") {
		return nil, status.Error(codes.PermissionDenied, "no access to strategy accounts")
	}
	
	return &pb.GetStrategyResponse{
		Strategy: convertStrategyToProto(strategy),
	}, nil
}

// ListStrategies lists all strategies with optional filters
func (s *StrategyService) ListStrategies(ctx context.Context, req *pb.ListStrategiesRequest) (*pb.ListStrategiesResponse, error) {
	// Check permissions
	claims, err := ExtractClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	
	if !s.authManager.CheckPermission(claims, pb.Permission_PERMISSION_READ_STRATEGIES, "") {
		return nil, status.Error(codes.PermissionDenied, "insufficient permissions")
	}
	
	// Create filter
	filter := types.StrategyFilter{
		Type:     convertStrategyTypeFromProto(req.Type),
		Status:   convertStrategyStatusFromProto(req.Status),
		PageSize: int(req.PageSize),
		PageToken: req.PageToken,
	}
	
	// List strategies
	strategies, nextToken, err := s.strategyManager.ListStrategies(ctx, filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list strategies: %v", err)
	}
	
	// Filter strategies based on account permissions
	var filteredStrategies []*types.Strategy
	for _, strategy := range strategies {
		hasAccess := s.authManager.CheckPermission(claims, pb.Permission_PERMISSION_ADMIN, "")
		if !hasAccess {
			for _, accountID := range strategy.Accounts {
				if s.authManager.CheckPermission(claims, pb.Permission_PERMISSION_READ_ACCOUNTS, accountID) {
					hasAccess = true
					break
				}
			}
		}
		
		if hasAccess {
			filteredStrategies = append(filteredStrategies, strategy)
		}
	}
	
	// Convert to proto
	protoStrategies := make([]*pb.Strategy, len(filteredStrategies))
	for i, s := range filteredStrategies {
		protoStrategies[i] = convertStrategyToProto(s)
	}
	
	return &pb.ListStrategiesResponse{
		Strategies:    protoStrategies,
		NextPageToken: nextToken,
	}, nil
}

// UpdateStrategy updates strategy configuration
func (s *StrategyService) UpdateStrategy(ctx context.Context, req *pb.UpdateStrategyRequest) (*pb.UpdateStrategyResponse, error) {
	// Check permissions
	claims, err := ExtractClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	
	if !s.authManager.CheckPermission(claims, pb.Permission_PERMISSION_MANAGE_STRATEGIES, "") {
		return nil, status.Error(codes.PermissionDenied, "insufficient permissions")
	}
	
	// Get existing strategy to check account access
	strategy, err := s.strategyManager.GetStrategy(ctx, req.StrategyId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "strategy not found: %v", err)
	}
	
	// Check account permissions
	for _, accountID := range strategy.Accounts {
		if !s.authManager.CheckPermission(claims, pb.Permission_PERMISSION_MANAGE_ACCOUNTS, accountID) &&
		   !s.authManager.CheckPermission(claims, pb.Permission_PERMISSION_ADMIN, "") {
			return nil, status.Error(codes.PermissionDenied, "no permission to manage strategy accounts")
		}
	}
	
	// Update strategy
	strategy.Config = convertStrategyConfig(req.Config)
	
	updatedStrategy, err := s.strategyManager.UpdateStrategy(ctx, strategy)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update strategy: %v", err)
	}
	
	return &pb.UpdateStrategyResponse{
		Strategy: convertStrategyToProto(updatedStrategy),
	}, nil
}

// StartStrategy starts a trading strategy
func (s *StrategyService) StartStrategy(ctx context.Context, req *pb.StartStrategyRequest) (*pb.StartStrategyResponse, error) {
	// Check permissions
	claims, err := ExtractClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	
	if !s.authManager.CheckPermission(claims, pb.Permission_PERMISSION_MANAGE_STRATEGIES, "") {
		return nil, status.Error(codes.PermissionDenied, "insufficient permissions")
	}
	
	// Start strategy
	err = s.strategyManager.StartStrategy(ctx, req.StrategyId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to start strategy: %v", err)
	}
	
	// Get updated strategy
	strategy, err := s.strategyManager.GetStrategy(ctx, req.StrategyId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get updated strategy: %v", err)
	}
	
	return &pb.StartStrategyResponse{
		Strategy: convertStrategyToProto(strategy),
	}, nil
}

// StopStrategy stops a trading strategy
func (s *StrategyService) StopStrategy(ctx context.Context, req *pb.StopStrategyRequest) (*pb.StopStrategyResponse, error) {
	// Check permissions
	claims, err := ExtractClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	
	if !s.authManager.CheckPermission(claims, pb.Permission_PERMISSION_MANAGE_STRATEGIES, "") {
		return nil, status.Error(codes.PermissionDenied, "insufficient permissions")
	}
	
	// Stop strategy
	err = s.strategyManager.StopStrategy(ctx, req.StrategyId, req.Reason)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to stop strategy: %v", err)
	}
	
	// Get updated strategy
	strategy, err := s.strategyManager.GetStrategy(ctx, req.StrategyId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get updated strategy: %v", err)
	}
	
	return &pb.StopStrategyResponse{
		Strategy: convertStrategyToProto(strategy),
	}, nil
}

// GetStrategyMetrics retrieves strategy performance metrics
func (s *StrategyService) GetStrategyMetrics(ctx context.Context, req *pb.GetStrategyMetricsRequest) (*pb.GetStrategyMetricsResponse, error) {
	// Check permissions
	claims, err := ExtractClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	
	// Get strategy to check accounts
	strategy, err := s.strategyManager.GetStrategy(ctx, req.StrategyId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "strategy not found: %v", err)
	}
	
	// Check if user has access to any of the strategy's accounts
	hasAccess := false
	for _, accountID := range strategy.Accounts {
		if s.authManager.CheckPermission(claims, pb.Permission_PERMISSION_READ_ACCOUNTS, accountID) {
			hasAccess = true
			break
		}
	}
	
	if !hasAccess && !s.authManager.CheckPermission(claims, pb.Permission_PERMISSION_ADMIN, "") {
		return nil, status.Error(codes.PermissionDenied, "no access to strategy accounts")
	}
	
	// Get metrics
	metrics, err := s.strategyManager.GetStrategyMetrics(ctx, req.StrategyId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get strategy metrics: %v", err)
	}
	
	return &pb.GetStrategyMetricsResponse{
		Metrics: convertMetricsToProto(metrics),
	}, nil
}

// GetStrategyPositions retrieves strategy positions
func (s *StrategyService) GetStrategyPositions(ctx context.Context, req *pb.GetStrategyPositionsRequest) (*pb.GetStrategyPositionsResponse, error) {
	// Check permissions
	claims, err := ExtractClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	
	// Get strategy to check accounts
	strategy, err := s.strategyManager.GetStrategy(ctx, req.StrategyId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "strategy not found: %v", err)
	}
	
	// Check if user has access to any of the strategy's accounts
	hasAccess := false
	for _, accountID := range strategy.Accounts {
		if s.authManager.CheckPermission(claims, pb.Permission_PERMISSION_READ_POSITIONS, accountID) {
			hasAccess = true
			break
		}
	}
	
	if !hasAccess && !s.authManager.CheckPermission(claims, pb.Permission_PERMISSION_ADMIN, "") {
		return nil, status.Error(codes.PermissionDenied, "no access to strategy positions")
	}
	
	// Get positions
	positions, err := s.strategyManager.GetStrategyPositions(ctx, req.StrategyId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get strategy positions: %v", err)
	}
	
	// Convert to proto
	protoPositions := make([]*pb.StrategyPosition, len(positions))
	for i, pos := range positions {
		protoPositions[i] = convertStrategyPositionToProto(pos)
	}
	
	return &pb.GetStrategyPositionsResponse{
		Positions: protoPositions,
	}, nil
}

// AssignAccounts assigns accounts to a strategy
func (s *StrategyService) AssignAccounts(ctx context.Context, req *pb.AssignAccountsRequest) (*pb.AssignAccountsResponse, error) {
	// Check permissions
	claims, err := ExtractClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	
	if !s.authManager.CheckPermission(claims, pb.Permission_PERMISSION_MANAGE_STRATEGIES, "") {
		return nil, status.Error(codes.PermissionDenied, "insufficient permissions to manage strategies")
	}
	
	// Check permissions for each account
	for _, accountID := range req.AccountIds {
		if !s.authManager.CheckPermission(claims, pb.Permission_PERMISSION_MANAGE_ACCOUNTS, accountID) &&
		   !s.authManager.CheckPermission(claims, pb.Permission_PERMISSION_ADMIN, "") {
			return nil, status.Errorf(codes.PermissionDenied, "no permission to manage account %s", accountID)
		}
	}
	
	// Assign accounts
	err = s.strategyManager.AssignAccounts(ctx, req.StrategyId, req.AccountIds)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to assign accounts: %v", err)
	}
	
	// Get updated strategy
	strategy, err := s.strategyManager.GetStrategy(ctx, req.StrategyId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get updated strategy: %v", err)
	}
	
	return &pb.AssignAccountsResponse{
		Strategy: convertStrategyToProto(strategy),
	}, nil
}

// Converter functions

func convertStrategyType(t pb.StrategyType) types.StrategyType {
	switch t {
	case pb.StrategyType_STRATEGY_TYPE_ARBITRAGE:
		return types.StrategyTypeArbitrage
	case pb.StrategyType_STRATEGY_TYPE_MARKET_MAKING:
		return types.StrategyTypeMarketMaking
	case pb.StrategyType_STRATEGY_TYPE_TREND_FOLLOWING:
		return types.StrategyTypeTrendFollowing
	case pb.StrategyType_STRATEGY_TYPE_MEAN_REVERSION:
		return types.StrategyTypeMeanReversion
	case pb.StrategyType_STRATEGY_TYPE_SCALPING:
		return types.StrategyTypeScalping
	case pb.StrategyType_STRATEGY_TYPE_GRID_TRADING:
		return types.StrategyTypeGridTrading
	default:
		return types.StrategyTypeUnspecified
	}
}

func convertStrategyTypeFromProto(t pb.StrategyType) types.StrategyType {
	return convertStrategyType(t)
}

func convertStrategyTypeToProto(t types.StrategyType) pb.StrategyType {
	switch t {
	case types.StrategyTypeArbitrage:
		return pb.StrategyType_STRATEGY_TYPE_ARBITRAGE
	case types.StrategyTypeMarketMaking:
		return pb.StrategyType_STRATEGY_TYPE_MARKET_MAKING
	case types.StrategyTypeTrendFollowing:
		return pb.StrategyType_STRATEGY_TYPE_TREND_FOLLOWING
	case types.StrategyTypeMeanReversion:
		return pb.StrategyType_STRATEGY_TYPE_MEAN_REVERSION
	case types.StrategyTypeScalping:
		return pb.StrategyType_STRATEGY_TYPE_SCALPING
	case types.StrategyTypeGridTrading:
		return pb.StrategyType_STRATEGY_TYPE_GRID_TRADING
	default:
		return pb.StrategyType_STRATEGY_TYPE_UNSPECIFIED
	}
}

func convertStrategyStatusFromProto(s pb.StrategyStatus) types.StrategyStatus {
	switch s {
	case pb.StrategyStatus_STRATEGY_STATUS_STOPPED:
		return types.StrategyStatusStopped
	case pb.StrategyStatus_STRATEGY_STATUS_STARTING:
		return types.StrategyStatusStarting
	case pb.StrategyStatus_STRATEGY_STATUS_RUNNING:
		return types.StrategyStatusRunning
	case pb.StrategyStatus_STRATEGY_STATUS_PAUSING:
		return types.StrategyStatusPausing
	case pb.StrategyStatus_STRATEGY_STATUS_PAUSED:
		return types.StrategyStatusPaused
	case pb.StrategyStatus_STRATEGY_STATUS_ERROR:
		return types.StrategyStatusError
	default:
		return types.StrategyStatusUnspecified
	}
}

func convertStrategyStatusToProto(s types.StrategyStatus) pb.StrategyStatus {
	switch s {
	case types.StrategyStatusStopped:
		return pb.StrategyStatus_STRATEGY_STATUS_STOPPED
	case types.StrategyStatusStarting:
		return pb.StrategyStatus_STRATEGY_STATUS_STARTING
	case types.StrategyStatusRunning:
		return pb.StrategyStatus_STRATEGY_STATUS_RUNNING
	case types.StrategyStatusPausing:
		return pb.StrategyStatus_STRATEGY_STATUS_PAUSING
	case types.StrategyStatusPaused:
		return pb.StrategyStatus_STRATEGY_STATUS_PAUSED
	case types.StrategyStatusError:
		return pb.StrategyStatus_STRATEGY_STATUS_ERROR
	default:
		return pb.StrategyStatus_STRATEGY_STATUS_UNSPECIFIED
	}
}

func convertStrategyConfig(config *pb.StrategyConfig) *types.StrategyConfig {
	if config == nil {
		return nil
	}
	
	return &types.StrategyConfig{
		Symbols:             config.Symbols,
		MaxPositionPerSymbol: config.MaxPositionPerSymbol,
		MaxTotalExposure:    config.MaxTotalExposure,
		RiskLimit:           config.RiskLimit,
		MaxOrdersPerSecond:  int(config.MaxOrdersPerSecond),
		Parameters:          config.Parameters.AsMap(),
	}
}

func convertStrategyToProto(s *types.Strategy) *pb.Strategy {
	if s == nil {
		return nil
	}
	
	return &pb.Strategy{
		Id:          s.ID,
		Name:        s.Name,
		Description: s.Description,
		Type:        convertStrategyTypeToProto(s.Type),
		Status:      convertStrategyStatusToProto(s.Status),
		Accounts:    s.Accounts,
		Config:      convertStrategyConfigToProto(s.Config),
		Metrics:     convertMetricsToProto(s.Metrics),
		CreatedAt:   timestamppb.New(s.CreatedAt),
		UpdatedAt:   timestamppb.New(s.UpdatedAt),
	}
}

func convertStrategyConfigToProto(config *types.StrategyConfig) *pb.StrategyConfig {
	if config == nil {
		return nil
	}
	
	// Convert parameters map to protobuf struct
	// Implementation depends on protobuf struct utilities
	
	return &pb.StrategyConfig{
		Symbols:              config.Symbols,
		MaxPositionPerSymbol: config.MaxPositionPerSymbol,
		MaxTotalExposure:     config.MaxTotalExposure,
		RiskLimit:            config.RiskLimit,
		MaxOrdersPerSecond:   int32(config.MaxOrdersPerSecond),
		// Parameters: convertMapToStruct(config.Parameters),
	}
}

func convertMetricsToProto(m *types.StrategyMetrics) *pb.StrategyMetrics {
	if m == nil {
		return nil
	}
	
	return &pb.StrategyMetrics{
		TotalPnl:      fmt.Sprintf("%.2f", m.TotalPnL),
		UnrealizedPnl: fmt.Sprintf("%.2f", m.UnrealizedPnL),
		RealizedPnl:   fmt.Sprintf("%.2f", m.RealizedPnL),
		WinRate:       m.WinRate,
		SharpeRatio:   m.SharpeRatio,
		MaxDrawdown:   m.MaxDrawdown,
		TotalTrades:   m.TotalTrades,
		WinningTrades: m.WinningTrades,
		LosingTrades:  m.LosingTrades,
		ProfitFactor:  m.ProfitFactor,
		UpdatedAt:     timestamppb.New(m.UpdatedAt),
	}
}

func convertStrategyPositionToProto(pos *types.StrategyPosition) *pb.StrategyPosition {
	if pos == nil {
		return nil
	}
	
	// Convert account quantities map
	accountQuantities := make(map[string]string)
	for k, v := range pos.AccountQuantities {
		accountQuantities[k] = fmt.Sprintf("%.8f", v)
	}
	
	return &pb.StrategyPosition{
		StrategyId:        pos.StrategyID,
		Symbol:            pos.Symbol,
		NetQuantity:       fmt.Sprintf("%.8f", pos.NetQuantity),
		TotalValue:        fmt.Sprintf("%.2f", pos.TotalValue),
		UnrealizedPnl:     fmt.Sprintf("%.2f", pos.UnrealizedPnL),
		AccountQuantities: accountQuantities,
		UpdatedAt:         timestamppb.New(pos.UpdatedAt),
	}
}