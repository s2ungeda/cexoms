package grpc

import (
	"context"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	pb "github.com/mExOms/proto/oms/v1"
	"github.com/mExOms/internal/account"
	"github.com/mExOms/internal/orders"
	"github.com/mExOms/internal/position"
	"github.com/mExOms/pkg/types"
	"github.com/mExOms/pkg/utils/logger"
)

// Server implements the gRPC server
type Server struct {
	pb.UnimplementedOrderServiceServer
	pb.UnimplementedPositionServiceServer
	pb.UnimplementedMarketDataServiceServer
	pb.UnimplementedAuthServiceServer
	pb.UnimplementedAccountServiceServer
	pb.UnimplementedStrategyServiceServer

	config          *Config
	accountManager  types.AccountManager
	orderManager    *orders.Manager
	positionManager *position.MultiAccountPositionManager
	transferManager *account.TransferManager
	strategyManager types.StrategyManager
	authManager     *AuthManager
	rateLimiter     *RateLimiter
	
	server          *grpc.Server
	logger          logger.Logger
}

// Config contains server configuration
type Config struct {
	Port              int
	MaxConnections    int
	MaxMessageSize    int
	EnableReflection  bool
	EnableAuth        bool
	APIKeyHeader      string
	TLSCertFile       string
	TLSKeyFile        string
	EnableTLS         bool
	ClientCAFile      string // For mutual TLS
	
	// Authentication
	JWTSecret         string
	TokenDuration     time.Duration
	
	// Rate limiting
	GlobalRPS         int
	PerAccountRPS     int
	OrderWeight       int
	QueryWeight       int
}

// NewServer creates a new gRPC server
func NewServer(
	config *Config,
	accountManager types.AccountManager,
	orderManager *orders.Manager,
	positionManager *position.MultiAccountPositionManager,
	transferManager *account.TransferManager,
	strategyManager types.StrategyManager,
) *Server {
	if config == nil {
		config = &Config{
			Port:           50051,
			MaxConnections: 1000,
			MaxMessageSize: 10 * 1024 * 1024, // 10MB
			EnableAuth:     true,
			APIKeyHeader:   "x-api-key",
			TokenDuration:  24 * time.Hour,
			GlobalRPS:      10000,
			PerAccountRPS:  100,
			OrderWeight:    10,
			QueryWeight:    1,
		}
	}

	// Create auth manager
	authManager := NewAuthManager(config.JWTSecret, config.TokenDuration)
	
	// Create rate limiter
	rateLimiter := NewRateLimiter(config.GlobalRPS, config.PerAccountRPS)

	return &Server{
		config:          config,
		accountManager:  accountManager,
		orderManager:    orderManager,
		positionManager: positionManager,
		transferManager: transferManager,
		strategyManager: strategyManager,
		authManager:     authManager,
		rateLimiter:     rateLimiter,
		logger:          logger.NewLogger("grpc-server"),
	}
}

// Start starts the gRPC server
func (s *Server) Start(ctx context.Context) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.config.Port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	// Create gRPC server options
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(s.config.MaxMessageSize),
		grpc.MaxSendMsgSize(s.config.MaxMessageSize),
		grpc.MaxConcurrentStreams(uint32(s.config.MaxConnections)),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     15 * time.Second,
			MaxConnectionAge:      30 * time.Second,
			MaxConnectionAgeGrace: 5 * time.Second,
			Time:                  5 * time.Second,
			Timeout:               1 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             5 * time.Second,
			PermitWithoutStream: true,
		}),
	}

	// Add TLS if enabled
	if s.config.EnableTLS {
		tlsConfig := &TLSConfig{
			CertFile:          s.config.TLSCertFile,
			KeyFile:           s.config.TLSKeyFile,
			ClientCAFile:      s.config.ClientCAFile,
			RequireClientCert: s.config.ClientCAFile != "",
		}
		
		creds, err := LoadTLSCredentials(tlsConfig)
		if err != nil {
			s.logger.Error("Failed to load TLS credentials: %v", err)
		} else {
			opts = append(opts, grpc.Creds(creds))
			s.logger.Info("TLS 1.3 enabled with mutual TLS: %v", tlsConfig.RequireClientCert)
		}
	}

	// Add interceptors
	interceptors := []grpc.UnaryServerInterceptor{
		s.loggingInterceptor,
		s.metricsInterceptor,
	}
	
	if s.config.EnableAuth {
		interceptors = append(interceptors, s.authInterceptor)
	}
	
	opts = append(opts, grpc.ChainUnaryInterceptor(interceptors...))

	// Create gRPC server
	s.server = grpc.NewServer(opts...)

	// Register services
	pb.RegisterOrderServiceServer(s.server, s)
	pb.RegisterPositionServiceServer(s.server, s)
	pb.RegisterMarketDataServiceServer(s.server, s)
	pb.RegisterAuthServiceServer(s.server, s)
	pb.RegisterAccountServiceServer(s.server, s)
	pb.RegisterStrategyServiceServer(s.server, s)

	// Enable reflection for debugging
	if s.config.EnableReflection {
		reflection.Register(s.server)
	}

	s.logger.Info("Starting gRPC server on port %d", s.config.Port)

	// Start server in goroutine
	go func() {
		if err := s.server.Serve(lis); err != nil {
			s.logger.Error("Failed to serve: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	s.logger.Info("Shutting down gRPC server...")
	s.server.GracefulStop()

	return nil
}

// Stop stops the gRPC server
func (s *Server) Stop() {
	if s.server != nil {
		s.server.GracefulStop()
	}
}

// authInterceptor validates API keys and JWT tokens
func (s *Server) authInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	// Skip auth for specific methods
	if info.FullMethod == "/oms.v1.AuthService/Authenticate" {
		return handler(ctx, req)
	}

	// Try JWT token first
	md, _ := metadata.FromIncomingContext(ctx)
	tokens := md.Get("authorization")
	
	if len(tokens) > 0 {
		// Validate JWT token
		claims, err := s.authManager.ValidateToken(tokens[0])
		if err == nil {
			// Add claims to context
			ctx = context.WithValue(ctx, "claims", claims)
			ctx = context.WithValue(ctx, "user_id", claims.UserID)
			
			// Check method permissions
			requiredPerm := getRequiredPermission(info.FullMethod)
			accountID := extractAccountIDFromRequest(req)
			
			if !s.authManager.CheckPermission(claims, requiredPerm, accountID) {
				return nil, status.Errorf(codes.PermissionDenied, "insufficient permissions")
			}
			
			// Check rate limit
			weight := s.rateLimiter.GetWeight(info.FullMethod)
			if !s.rateLimiter.Allow(claims.UserID, weight) {
				return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded")
			}
			
			return handler(ctx, req)
		}
	}
	
	// Fall back to API key
	apiKey, err := GetAPIKeyFromContext(ctx, s.config.APIKeyHeader)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "missing authentication")
	}

	// Validate API key
	apiKeyInfo, err := s.authManager.ValidateAPIKey(apiKey)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid API key: %v", err)
	}
	
	// Create claims from API key
	claims := &Claims{
		UserID:      apiKeyInfo.ID,
		APIKeyID:    apiKeyInfo.ID,
		Permissions: permissionsToStrings(apiKeyInfo.Permissions),
	}
	
	// Check method permissions
	requiredPerm := getRequiredPermission(info.FullMethod)
	accountID := extractAccountIDFromRequest(req)
	
	if !s.authManager.CheckPermission(claims, requiredPerm, accountID) {
		return nil, status.Errorf(codes.PermissionDenied, "insufficient permissions")
	}
	
	// Check rate limit with API key specific limits
	weight := s.rateLimiter.GetWeight(info.FullMethod)
	s.rateLimiter.SetAccountRateLimit(apiKeyInfo.ID, int(apiKeyInfo.RateLimit.RequestsPerSecond))
	
	if !s.rateLimiter.Allow(apiKeyInfo.ID, weight) {
		return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded")
	}

	// Add user context
	ctx = context.WithValue(ctx, "api_key_info", apiKeyInfo)
	ctx = context.WithValue(ctx, "user_id", apiKeyInfo.ID)

	return handler(ctx, req)
}

// validateAPIKey validates the API key
func (s *Server) validateAPIKey(apiKey string) bool {
	// TODO: Implement actual API key validation
	// This should check against stored API keys
	return apiKey != ""
}

// getRequiredPermission returns the required permission for a method
func getRequiredPermission(method string) pb.Permission {
	// Map methods to permissions
	methodPerms := map[string]pb.Permission{
		// Order methods
		"/oms.v1.OrderService/CreateOrder": pb.Permission_PERMISSION_WRITE_ORDERS,
		"/oms.v1.OrderService/CancelOrder": pb.Permission_PERMISSION_WRITE_ORDERS,
		"/oms.v1.OrderService/GetOrder":    pb.Permission_PERMISSION_READ_ORDERS,
		"/oms.v1.OrderService/ListOrders":  pb.Permission_PERMISSION_READ_ORDERS,
		
		// Position methods
		"/oms.v1.PositionService/GetPosition":             pb.Permission_PERMISSION_READ_POSITIONS,
		"/oms.v1.PositionService/ListPositions":           pb.Permission_PERMISSION_READ_POSITIONS,
		"/oms.v1.PositionService/GetAggregatedPositions":  pb.Permission_PERMISSION_READ_POSITIONS,
		"/oms.v1.PositionService/GetRiskMetrics":          pb.Permission_PERMISSION_READ_POSITIONS,
		
		// Account methods
		"/oms.v1.AccountService/CreateAccount":      pb.Permission_PERMISSION_MANAGE_ACCOUNTS,
		"/oms.v1.AccountService/GetAccount":         pb.Permission_PERMISSION_READ_ACCOUNTS,
		"/oms.v1.AccountService/ListAccounts":       pb.Permission_PERMISSION_READ_ACCOUNTS,
		"/oms.v1.AccountService/GetAccountBalance":  pb.Permission_PERMISSION_READ_ACCOUNTS,
		"/oms.v1.AccountService/Transfer":           pb.Permission_PERMISSION_TRANSFER_FUNDS,
		
		// Strategy methods
		"/oms.v1.StrategyService/CreateStrategy":      pb.Permission_PERMISSION_MANAGE_STRATEGIES,
		"/oms.v1.StrategyService/GetStrategy":         pb.Permission_PERMISSION_READ_STRATEGIES,
		"/oms.v1.StrategyService/ListStrategies":      pb.Permission_PERMISSION_READ_STRATEGIES,
		"/oms.v1.StrategyService/StartStrategy":       pb.Permission_PERMISSION_MANAGE_STRATEGIES,
		"/oms.v1.StrategyService/StopStrategy":        pb.Permission_PERMISSION_MANAGE_STRATEGIES,
		
		// Market data (usually public)
		"/oms.v1.MarketDataService/GetOrderBook":     pb.Permission_PERMISSION_READ_MARKET_DATA,
		"/oms.v1.MarketDataService/GetTicker":        pb.Permission_PERMISSION_READ_MARKET_DATA,
		"/oms.v1.MarketDataService/GetRecentTrades":  pb.Permission_PERMISSION_READ_MARKET_DATA,
	}
	
	if perm, ok := methodPerms[method]; ok {
		return perm
	}
	
	// Default to read permission
	return pb.Permission_PERMISSION_READ_MARKET_DATA
}

// extractAccountIDFromRequest extracts account ID from various request types
func extractAccountIDFromRequest(req interface{}) string {
	switch r := req.(type) {
	case *pb.CreateOrderRequest:
		return r.GetAccountId()
	case *pb.GetAccountRequest:
		return r.GetAccountId()
	case *pb.GetAccountBalanceRequest:
		return r.GetAccountId()
	case *pb.TransferRequest:
		return r.GetFromAccount()
	case *pb.GetAccountPositionsRequest:
		return r.GetAccountId()
	default:
		return ""
	}
}

// permissionsToStrings converts permissions to string slice
func permissionsToStrings(perms []pb.Permission) []string {
	result := make([]string, len(perms))
	for i, p := range perms {
		result[i] = p.String()
	}
	return result
}

// loggingInterceptor logs requests and responses
func (s *Server) loggingInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	start := time.Now()
	
	// Extract user ID
	userID := "anonymous"
	if id, ok := ctx.Value("user_id").(string); ok {
		userID = id
	}
	
	// Call handler
	resp, err := handler(ctx, req)
	
	// Log request
	duration := time.Since(start)
	if err != nil {
		s.logger.Error("Method: %s, User: %s, Duration: %v, Error: %v",
			info.FullMethod, userID, duration, err)
	} else {
		s.logger.Info("Method: %s, User: %s, Duration: %v",
			info.FullMethod, userID, duration)
	}
	
	return resp, err
}

// metricsInterceptor collects metrics
func (s *Server) metricsInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	start := time.Now()
	
	// Call handler
	resp, err := handler(ctx, req)
	
	// Collect metrics
	duration := time.Since(start)
	
	// Update metrics (placeholder - implement actual metrics collection)
	// metrics.RecordRPCDuration(info.FullMethod, duration)
	// if err != nil {
	//     metrics.IncrementErrorCount(info.FullMethod)
	// }
	
	return resp, err
}