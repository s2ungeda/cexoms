package grpc

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/mExOms/proto/oms/v1"
	"github.com/mExOms/internal/account"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// CreateAccount creates a new trading account
func (s *Server) CreateAccount(ctx context.Context, req *pb.CreateAccountRequest) (*pb.CreateAccountResponse, error) {
	s.logger.Info("CreateAccount request: %v", req)

	// Convert request to internal type
	createReq := &types.CreateAccountRequest{
		Name:             req.Name,
		Type:             convertAccountType(req.Type),
		Exchange:         req.Exchange,
		Strategy:         req.Strategy,
		SpotEnabled:      req.SpotEnabled,
		FuturesEnabled:   req.FuturesEnabled,
		MaxPositionUSDT:  decimal.RequireFromString(req.MaxPositionUsdt),
		MaxLeverage:      int(req.MaxLeverage),
	}

	// Create account
	account, err := s.accountManager.CreateAccount(createReq)
	if err != nil {
		s.logger.Error("Failed to create account: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to create account: %v", err)
	}

	return &pb.CreateAccountResponse{
		Account: convertAccountToProto(account),
	}, nil
}

// GetAccount retrieves account details
func (s *Server) GetAccount(ctx context.Context, req *pb.GetAccountRequest) (*pb.GetAccountResponse, error) {
	account, err := s.accountManager.GetAccount(req.AccountId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "account not found: %v", err)
	}

	return &pb.GetAccountResponse{
		Account: convertAccountToProto(account),
	}, nil
}

// ListAccounts lists all accounts with optional filters
func (s *Server) ListAccounts(ctx context.Context, req *pb.ListAccountsRequest) (*pb.ListAccountsResponse, error) {
	filter := types.AccountFilter{
		Exchange: req.Exchange,
		Type:     convertAccountTypeFromProto(req.Type),
		Strategy: req.Strategy,
	}

	if req.ActiveOnly {
		active := true
		filter.Active = &active
	}

	accounts, err := s.accountManager.ListAccounts(filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list accounts: %v", err)
	}

	// Convert to proto
	pbAccounts := make([]*pb.Account, len(accounts))
	for i, acc := range accounts {
		pbAccounts[i] = convertAccountToProto(acc)
	}

	return &pb.ListAccountsResponse{
		Accounts: pbAccounts,
	}, nil
}

// GetAccountBalance retrieves account balance
func (s *Server) GetAccountBalance(ctx context.Context, req *pb.GetAccountBalanceRequest) (*pb.GetAccountBalanceResponse, error) {
	balance, err := s.accountManager.GetBalance(req.AccountId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "failed to get balance: %v", err)
	}

	// Convert to proto
	pbBalances := []*pb.AccountBalance{
		{
			AccountId:  balance.AccountID,
			Exchange:   balance.Exchange,
			Asset:      "USDT", // Primary asset
			Total:      balance.TotalUSDT.String(),
			Available:  balance.Available.String(),
			Locked:     balance.Locked.String(),
			UpdateTime: timestamppb.New(balance.UpdateTime),
		},
	}

	// Add other assets if available
	for asset, assetBalance := range balance.Balances {
		pbBalances = append(pbBalances, &pb.AccountBalance{
			AccountId:  balance.AccountID,
			Exchange:   balance.Exchange,
			Asset:      asset,
			Total:      assetBalance.Total.String(),
			Available:  assetBalance.Available.String(),
			Locked:     assetBalance.Locked.String(),
			UpdateTime: timestamppb.New(balance.UpdateTime),
		})
	}

	return &pb.GetAccountBalanceResponse{
		Balances: pbBalances,
	}, nil
}

// GetAccountPositions retrieves account positions
func (s *Server) GetAccountPositions(ctx context.Context, req *pb.GetAccountPositionsRequest) (*pb.GetAccountPositionsResponse, error) {
	positions, err := s.positionManager.GetAccountPositions(req.AccountId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get positions: %v", err)
	}

	// Convert to proto
	pbPositions := make([]*pb.AccountPosition, len(positions))
	for i, pos := range positions {
		pbPositions[i] = &pb.AccountPosition{
			AccountId:     pos.AccountID,
			Symbol:        pos.Symbol,
			Side:          convertPositionSideToProto(pos.Side),
			Quantity:      pos.Quantity.String(),
			EntryPrice:    pos.EntryPrice.String(),
			MarkPrice:     pos.MarkPrice.String(),
			UnrealizedPnl: pos.UnrealizedPnL.String(),
			RealizedPnl:   pos.RealizedPnL.String(),
			Margin:        pos.Margin.String(),
			Leverage:      int32(pos.Leverage),
			UpdateTime:    timestamppb.New(pos.UpdateTime),
		}
	}

	return &pb.GetAccountPositionsResponse{
		Positions: pbPositions,
	}, nil
}

// Transfer handles asset transfer between accounts
func (s *Server) Transfer(ctx context.Context, req *pb.TransferRequest) (*pb.TransferResponse, error) {
	// Create transfer request
	transferReq := &account.TransferRequest{
		FromAccount: req.FromAccount,
		ToAccount:   req.ToAccount,
		Asset:       req.Asset,
		Amount:      decimal.RequireFromString(req.Amount),
		Reason:      req.Reason,
	}

	// Request transfer
	transfer, err := s.transferManager.RequestTransfer(ctx, transferReq)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to request transfer: %v", err)
	}

	// Execute transfer
	if err := s.transferManager.ExecuteTransfer(ctx, transfer.ID); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to execute transfer: %v", err)
	}

	// Get updated transfer status
	transfer, err = s.transferManager.GetTransfer(transfer.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get transfer status: %v", err)
	}

	return &pb.TransferResponse{
		Transfer: convertTransferToProto(transfer),
	}, nil
}

// GetTransferHistory retrieves transfer history
func (s *Server) GetTransferHistory(ctx context.Context, req *pb.GetTransferHistoryRequest) (*pb.GetTransferHistoryResponse, error) {
	transfers, err := s.accountManager.GetTransferHistory(req.AccountId, int(req.Limit))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get transfer history: %v", err)
	}

	// Convert to proto
	pbTransfers := make([]*pb.AccountTransfer, len(transfers))
	for i, transfer := range transfers {
		pbTransfers[i] = convertTransferToProto(transfer)
	}

	return &pb.GetTransferHistoryResponse{
		Transfers: pbTransfers,
	}, nil
}

// GetAccountMetrics retrieves account performance metrics
func (s *Server) GetAccountMetrics(ctx context.Context, req *pb.GetAccountMetricsRequest) (*pb.GetAccountMetricsResponse, error) {
	metrics, err := s.accountManager.GetMetrics(req.AccountId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "failed to get metrics: %v", err)
	}

	return &pb.GetAccountMetricsResponse{
		Metrics: &pb.AccountMetrics{
			AccountId:    metrics.AccountID,
			TotalPnl:     metrics.TotalPnL.String(),
			TodayPnl:     metrics.TodayPnL.String(),
			WinRate:      metrics.WinRate,
			TotalTrades:  metrics.TotalTrades,
			MaxDrawdown:  metrics.MaxDrawdown.String(),
			SharpeRatio:  metrics.SharpeRatio,
			UpdatedAt:    timestamppb.New(metrics.UpdatedAt),
		},
	}, nil
}

// SelectAccount selects the best account for a strategy
func (s *Server) SelectAccount(ctx context.Context, req *pb.SelectAccountRequest) (*pb.SelectAccountResponse, error) {
	requirements := types.AccountRequirements{
		Strategy:     req.Strategy,
		MinBalance:   decimal.Zero,
		MaxPositions: int(req.MaxPositions),
	}

	if req.MinBalance != "" {
		requirements.MinBalance = decimal.RequireFromString(req.MinBalance)
	}

	account, err := s.accountManager.SelectAccount(req.Strategy, requirements)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "no suitable account found: %v", err)
	}

	return &pb.SelectAccountResponse{
		Account: convertAccountToProto(account),
	}, nil
}

// Helper functions

func convertAccountToProto(acc *types.Account) *pb.Account {
	return &pb.Account{
		Id:               acc.ID,
		Name:             acc.Name,
		Type:             convertAccountTypeToProto(acc.Type),
		Exchange:         acc.Exchange,
		Strategy:         acc.Strategy,
		SpotEnabled:      acc.SpotEnabled,
		FuturesEnabled:   acc.FuturesEnabled,
		MaxPositionUsdt:  acc.MaxPositionUSDT.String(),
		MaxLeverage:      int32(acc.MaxLeverage),
		Active:           acc.Active,
		CreatedAt:        timestamppb.New(acc.CreatedAt),
		UpdatedAt:        timestamppb.New(acc.UpdatedAt),
	}
}

func convertTransferToProto(transfer *types.AccountTransfer) *pb.AccountTransfer {
	pbTransfer := &pb.AccountTransfer{
		Id:           transfer.ID,
		FromAccount:  transfer.FromAccount,
		ToAccount:    transfer.ToAccount,
		Asset:        transfer.Asset,
		Amount:       transfer.Amount.String(),
		Status:       convertTransferStatusToProto(transfer.Status),
		Reason:       transfer.Reason,
		CreatedAt:    timestamppb.New(transfer.CreatedAt),
	}

	if !transfer.ExecutedAt.IsZero() {
		pbTransfer.ExecutedAt = timestamppb.New(transfer.ExecutedAt)
	}

	return pbTransfer
}

func convertAccountType(t pb.AccountType) types.AccountType {
	switch t {
	case pb.AccountType_ACCOUNT_TYPE_MAIN:
		return types.AccountTypeMain
	case pb.AccountType_ACCOUNT_TYPE_SUB:
		return types.AccountTypeSub
	case pb.AccountType_ACCOUNT_TYPE_STRATEGY:
		return types.AccountTypeStrategy
	default:
		return ""
	}
}

func convertAccountTypeFromProto(t pb.AccountType) string {
	switch t {
	case pb.AccountType_ACCOUNT_TYPE_MAIN:
		return "main"
	case pb.AccountType_ACCOUNT_TYPE_SUB:
		return "sub"
	case pb.AccountType_ACCOUNT_TYPE_STRATEGY:
		return "strategy"
	default:
		return ""
	}
}

func convertAccountTypeToProto(t types.AccountType) pb.AccountType {
	switch t {
	case types.AccountTypeMain:
		return pb.AccountType_ACCOUNT_TYPE_MAIN
	case types.AccountTypeSub:
		return pb.AccountType_ACCOUNT_TYPE_SUB
	case types.AccountTypeStrategy:
		return pb.AccountType_ACCOUNT_TYPE_STRATEGY
	default:
		return pb.AccountType_ACCOUNT_TYPE_UNSPECIFIED
	}
}

func convertPositionSideToProto(side types.PositionSide) pb.PositionSide {
	switch side {
	case types.PositionSideLong:
		return pb.PositionSide_POSITION_SIDE_LONG
	case types.PositionSideShort:
		return pb.PositionSide_POSITION_SIDE_SHORT
	case types.PositionSideBoth:
		return pb.PositionSide_POSITION_SIDE_BOTH
	default:
		return pb.PositionSide_POSITION_SIDE_UNSPECIFIED
	}
}

func convertTransferStatusToProto(status types.TransferStatus) pb.TransferStatus {
	switch status {
	case types.TransferStatusPending:
		return pb.TransferStatus_TRANSFER_STATUS_PENDING
	case types.TransferStatusProcessing:
		return pb.TransferStatus_TRANSFER_STATUS_PROCESSING
	case types.TransferStatusCompleted:
		return pb.TransferStatus_TRANSFER_STATUS_COMPLETED
	case types.TransferStatusFailed:
		return pb.TransferStatus_TRANSFER_STATUS_FAILED
	case types.TransferStatusCancelled:
		return pb.TransferStatus_TRANSFER_STATUS_CANCELLED
	default:
		return pb.TransferStatus_TRANSFER_STATUS_UNSPECIFIED
	}
}