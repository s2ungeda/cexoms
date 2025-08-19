package grpc

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	omsv1 "github.com/mExOms/pkg/proto/oms/v1"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// contextKey is a custom type for context keys
type contextKey string

const (
	// Context keys
	contextKeyUserID      contextKey = "user_id"
	contextKeyPermissions contextKey = "permissions"
	
	// Headers
	authHeader = "authorization"
	apiKeyHeader = "x-api-key"
)

// AuthInterceptor handles authentication
type AuthInterceptor struct {
	authService *AuthService
	jwtSecret   []byte
	// Whitelist of methods that don't require auth
	publicMethods map[string]bool
}

// NewAuthInterceptor creates a new auth interceptor
func NewAuthInterceptor(authService *AuthService) *AuthInterceptor {
	return &AuthInterceptor{
		authService: authService,
		jwtSecret:   authService.JwtSecret,
		publicMethods: map[string]bool{
			"/oms.v1.AuthService/Authenticate": true,
		},
	}
}

// Unary returns a unary server interceptor for authentication
func (a *AuthInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Skip auth for public methods
		if a.publicMethods[info.FullMethod] {
			return handler(ctx, req)
		}
		
		// Extract and validate token
		newCtx, err := a.authenticate(ctx)
		if err != nil {
			return nil, err
		}
		
		// Check permissions
		if err := a.checkPermissions(newCtx, info.FullMethod); err != nil {
			return nil, err
		}
		
		return handler(newCtx, req)
	}
}

// Stream returns a stream server interceptor for authentication
func (a *AuthInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Skip auth for public methods
		if a.publicMethods[info.FullMethod] {
			return handler(srv, ss)
		}
		
		// Extract and validate token
		newCtx, err := a.authenticate(ss.Context())
		if err != nil {
			return err
		}
		
		// Check permissions
		if err := a.checkPermissions(newCtx, info.FullMethod); err != nil {
			return err
		}
		
		// Create wrapped stream with new context
		wrapped := &wrappedStream{
			ServerStream: ss,
			ctx:         newCtx,
		}
		
		return handler(srv, wrapped)
	}
}

func (a *AuthInterceptor) authenticate(ctx context.Context) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing metadata")
	}
	
	// Check for Bearer token
	if auth := md.Get(authHeader); len(auth) > 0 {
		token := strings.TrimPrefix(auth[0], "Bearer ")
		return a.validateJWT(ctx, token)
	}
	
	// Check for API key
	if apiKeys := md.Get(apiKeyHeader); len(apiKeys) > 0 {
		return a.validateAPIKey(ctx, apiKeys[0])
	}
	
	return nil, status.Errorf(codes.Unauthenticated, "missing authentication")
}

func (a *AuthInterceptor) validateJWT(ctx context.Context, tokenString string) (context.Context, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return a.jwtSecret, nil
	})
	
	if err != nil || !token.Valid {
		return nil, status.Errorf(codes.Unauthenticated, "invalid token")
	}
	
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "invalid token claims")
	}
	
	// Extract user ID and permissions
	userID, _ := claims["user_id"].(string)
	permissions := a.extractPermissions(claims)
	
	// Add to context
	ctx = context.WithValue(ctx, contextKeyUserID, userID)
	ctx = context.WithValue(ctx, contextKeyPermissions, permissions)
	
	return ctx, nil
}

func (a *AuthInterceptor) validateAPIKey(ctx context.Context, apiKey string) (context.Context, error) {
	// Look up API key
	data, ok := a.authService.ApiKeys.Load(apiKey)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "invalid api key")
	}
	
	apiKeyData := data.(*APIKeyData)
	
	// Check if active
	if !apiKeyData.IsActive {
		return nil, status.Errorf(codes.PermissionDenied, "api key is inactive")
	}
	
	// Update last used
	apiKeyData.LastUsed = time.Now()
	a.authService.ApiKeys.Store(apiKey, apiKeyData)
	
	// Convert permissions
	permissions := make([]string, len(apiKeyData.Permissions))
	for i, p := range apiKeyData.Permissions {
		permissions[i] = p.String()
	}
	
	// Add to context
	ctx = context.WithValue(ctx, contextKeyUserID, apiKeyData.ID)
	ctx = context.WithValue(ctx, contextKeyPermissions, permissions)
	
	return ctx, nil
}

func (a *AuthInterceptor) checkPermissions(ctx context.Context, method string) error {
	permissions, ok := ctx.Value(contextKeyPermissions).([]string)
	if !ok {
		return status.Errorf(codes.Internal, "missing permissions in context")
	}
	
	// Check for admin permission (bypasses all checks)
	for _, p := range permissions {
		if p == omsv1.Permission_PERMISSION_ADMIN.String() {
			return nil
		}
	}
	
	// Map methods to required permissions
	requiredPerm := a.getRequiredPermission(method)
	if requiredPerm == "" {
		return nil // No specific permission required
	}
	
	// Check if user has required permission
	for _, p := range permissions {
		if p == requiredPerm {
			return nil
		}
	}
	
	return status.Errorf(codes.PermissionDenied, "insufficient permissions")
}

func (a *AuthInterceptor) getRequiredPermission(method string) string {
	switch {
	case strings.Contains(method, "OrderService/CreateOrder"),
		strings.Contains(method, "OrderService/CancelOrder"):
		return omsv1.Permission_PERMISSION_WRITE_ORDERS.String()
		
	case strings.Contains(method, "OrderService/GetOrder"),
		strings.Contains(method, "OrderService/ListOrders"):
		return omsv1.Permission_PERMISSION_READ_ORDERS.String()
		
	case strings.Contains(method, "PositionService"):
		return omsv1.Permission_PERMISSION_READ_POSITIONS.String()
		
	case strings.Contains(method, "MarketDataService"):
		return omsv1.Permission_PERMISSION_READ_MARKET_DATA.String()
		
	default:
		return ""
	}
}

func (a *AuthInterceptor) extractPermissions(claims jwt.MapClaims) []string {
	permStrings, ok := claims["permissions"].([]interface{})
	if !ok {
		return nil
	}
	
	permissions := make([]string, 0, len(permStrings))
	for _, p := range permStrings {
		if str, ok := p.(string); ok {
			permissions = append(permissions, str)
		}
	}
	
	return permissions
}

// RateLimiter provides rate limiting middleware
type RateLimiter struct {
	limiters sync.Map // userID -> *rate.Limiter
	rps      int      // requests per second
	burst    int      // burst size
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rps, burst int) *RateLimiter {
	return &RateLimiter{
		rps:   rps,
		burst: burst,
	}
}

// Unary returns a unary server interceptor for rate limiting
func (r *RateLimiter) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if err := r.checkLimit(ctx); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// Stream returns a stream server interceptor for rate limiting
func (r *RateLimiter) Stream() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := r.checkLimit(ss.Context()); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

func (r *RateLimiter) checkLimit(ctx context.Context) error {
	// Get user ID from context
	userID, ok := ctx.Value(contextKeyUserID).(string)
	if !ok {
		userID = "anonymous"
	}
	
	// Get or create limiter for user
	limiterI, _ := r.limiters.LoadOrStore(userID, rate.NewLimiter(rate.Limit(r.rps), r.burst))
	limiter := limiterI.(*rate.Limiter)
	
	// Check rate limit
	if !limiter.Allow() {
		return status.Errorf(codes.ResourceExhausted, "rate limit exceeded")
	}
	
	return nil
}

// wrappedStream wraps a ServerStream with a custom context
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context {
	return w.ctx
}