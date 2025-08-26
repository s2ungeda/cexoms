package security

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AuthType represents the type of authentication
type AuthType string

const (
	AuthTypeJWT    AuthType = "jwt"
	AuthTypeAPIKey AuthType = "api_key"
	AuthTypeHMAC   AuthType = "hmac"
	AuthTypeMTLS   AuthType = "mtls"
	AuthTypeOAuth2 AuthType = "oauth2"
)

// AuthConfig holds authentication configuration
type AuthConfig struct {
	// JWT Configuration
	JWTSecret          string
	JWTIssuer          string
	JWTAudience        []string
	JWTExpiry          time.Duration
	JWTRefreshExpiry   time.Duration
	
	// API Key Configuration
	APIKeyHeader       string
	APIKeyQueryParam   string
	
	// HMAC Configuration
	HMACSecret         string
	HMACHeader         string
	HMACTimestampTolerance time.Duration
	
	// OAuth2 Configuration
	OAuth2Provider     string
	OAuth2ClientID     string
	OAuth2ClientSecret string
	OAuth2RedirectURL  string
	OAuth2Scopes       []string
	
	// Security Settings
	EnableRateLimiting bool
	EnableIPWhitelist  bool
	RequireMFA         bool
	SessionTimeout     time.Duration
}

// AuthMiddleware provides authentication middleware
type AuthMiddleware struct {
	config         *AuthConfig
	tokenValidator *TokenValidator
	apiKeyStore    *APIKeyStore
	sessionManager *SessionManager
	mfaProvider    *MFAProvider
	auditLogger    *AuditLogger
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware(config *AuthConfig) (*AuthMiddleware, error) {
	tokenValidator, err := NewTokenValidator(config.JWTSecret, config.JWTIssuer)
	if err != nil {
		return nil, err
	}
	
	apiKeyStore := NewAPIKeyStore()
	sessionManager := NewSessionManager(config.SessionTimeout)
	mfaProvider := NewMFAProvider()
	auditLogger := NewAuditLogger()
	
	return &AuthMiddleware{
		config:         config,
		tokenValidator: tokenValidator,
		apiKeyStore:    apiKeyStore,
		sessionManager: sessionManager,
		mfaProvider:    mfaProvider,
		auditLogger:    auditLogger,
	}, nil
}

// HTTPMiddleware returns HTTP authentication middleware
func (am *AuthMiddleware) HTTPMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract authentication credentials
		authType, credentials, err := am.extractHTTPCredentials(r)
		if err != nil {
			am.auditLogger.LogFailedAuth(r.Context(), authType, err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		
		// Validate credentials
		authContext, err := am.validateCredentials(r.Context(), authType, credentials)
		if err != nil {
			am.auditLogger.LogFailedAuth(r.Context(), authType, err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		
		// Check MFA if required
		if am.config.RequireMFA && !authContext.MFAVerified {
			mfaToken := r.Header.Get("X-MFA-Token")
			if !am.mfaProvider.VerifyToken(authContext.UserID, mfaToken) {
				am.auditLogger.LogMFAFailure(r.Context(), authContext.UserID)
				http.Error(w, "MFA Required", http.StatusUnauthorized)
				return
			}
			authContext.MFAVerified = true
		}
		
		// Check session
		if !am.sessionManager.IsValid(authContext.SessionID) {
			http.Error(w, "Session Expired", http.StatusUnauthorized)
			return
		}
		
		// Add auth context to request
		ctx := context.WithValue(r.Context(), "auth", authContext)
		r = r.WithContext(ctx)
		
		// Log successful authentication
		am.auditLogger.LogSuccessfulAuth(ctx, authContext)
		
		// Call next handler
		next(w, r)
	}
}

// GRPCUnaryInterceptor returns gRPC unary interceptor
func (am *AuthMiddleware) GRPCUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Skip auth for health check
		if info.FullMethod == "/grpc.health.v1.Health/Check" {
			return handler(ctx, req)
		}
		
		// Extract metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}
		
		// Validate authentication
		authContext, err := am.validateGRPCAuth(ctx, md)
		if err != nil {
			am.auditLogger.LogFailedAuth(ctx, AuthTypeJWT, err)
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}
		
		// Add auth context
		ctx = context.WithValue(ctx, "auth", authContext)
		
		// Log successful authentication
		am.auditLogger.LogSuccessfulAuth(ctx, authContext)
		
		// Call handler
		return handler(ctx, req)
	}
}

// GRPCStreamInterceptor returns gRPC stream interceptor
func (am *AuthMiddleware) GRPCStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Extract metadata
		md, ok := metadata.FromIncomingContext(ss.Context())
		if !ok {
			return status.Error(codes.Unauthenticated, "missing metadata")
		}
		
		// Validate authentication
		authContext, err := am.validateGRPCAuth(ss.Context(), md)
		if err != nil {
			am.auditLogger.LogFailedAuth(ss.Context(), AuthTypeJWT, err)
			return status.Error(codes.Unauthenticated, err.Error())
		}
		
		// Create wrapped stream with auth context
		wrappedStream := &authServerStream{
			ServerStream: ss,
			ctx:          context.WithValue(ss.Context(), "auth", authContext),
		}
		
		// Log successful authentication
		am.auditLogger.LogSuccessfulAuth(wrappedStream.ctx, authContext)
		
		// Call handler
		return handler(srv, wrappedStream)
	}
}

// extractHTTPCredentials extracts credentials from HTTP request
func (am *AuthMiddleware) extractHTTPCredentials(r *http.Request) (AuthType, string, error) {
	// Check JWT in Authorization header
	if auth := r.Header.Get("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return AuthTypeJWT, strings.TrimPrefix(auth, "Bearer "), nil
		}
	}
	
	// Check API key in header
	if apiKey := r.Header.Get(am.config.APIKeyHeader); apiKey != "" {
		return AuthTypeAPIKey, apiKey, nil
	}
	
	// Check API key in query parameter
	if apiKey := r.URL.Query().Get(am.config.APIKeyQueryParam); apiKey != "" {
		return AuthTypeAPIKey, apiKey, nil
	}
	
	// Check HMAC signature
	if signature := r.Header.Get(am.config.HMACHeader); signature != "" {
		return AuthTypeHMAC, signature, nil
	}
	
	// Check client certificate for mTLS
	if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		return AuthTypeMTLS, r.TLS.PeerCertificates[0].Subject.CommonName, nil
	}
	
	return "", "", fmt.Errorf("no valid authentication credentials found")
}

// validateCredentials validates authentication credentials
func (am *AuthMiddleware) validateCredentials(ctx context.Context, authType AuthType, credentials string) (*AuthContext, error) {
	switch authType {
	case AuthTypeJWT:
		return am.validateJWT(ctx, credentials)
	case AuthTypeAPIKey:
		return am.validateAPIKey(ctx, credentials)
	case AuthTypeHMAC:
		return am.validateHMAC(ctx, credentials)
	case AuthTypeMTLS:
		return am.validateMTLS(ctx, credentials)
	default:
		return nil, fmt.Errorf("unsupported auth type: %s", authType)
	}
}

// validateJWT validates JWT token
func (am *AuthMiddleware) validateJWT(ctx context.Context, tokenString string) (*AuthContext, error) {
	// Parse and validate token
	token, err := am.tokenValidator.ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}
	
	// Extract claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}
	
	// Build auth context
	authContext := &AuthContext{
		AuthType:     AuthTypeJWT,
		UserID:       claims["sub"].(string),
		AccountID:    claims["account_id"].(string),
		Permissions:  extractPermissions(claims),
		SessionID:    generateSessionID(),
		ExpiresAt:    time.Unix(int64(claims["exp"].(float64)), 0),
		MFAVerified:  false,
	}
	
	// Create session
	am.sessionManager.Create(authContext.SessionID, authContext)
	
	return authContext, nil
}

// validateAPIKey validates API key
func (am *AuthMiddleware) validateAPIKey(ctx context.Context, apiKey string) (*AuthContext, error) {
	// Lookup API key
	keyInfo, err := am.apiKeyStore.Get(apiKey)
	if err != nil {
		return nil, err
	}
	
	// Check if key is active
	if !keyInfo.Active {
		return nil, fmt.Errorf("API key is inactive")
	}
	
	// Check expiry
	if keyInfo.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("API key has expired")
	}
	
	// Build auth context
	authContext := &AuthContext{
		AuthType:     AuthTypeAPIKey,
		UserID:       keyInfo.UserID,
		AccountID:    keyInfo.AccountID,
		Permissions:  keyInfo.Permissions,
		SessionID:    generateSessionID(),
		ExpiresAt:    keyInfo.ExpiresAt,
		APIKeyID:     keyInfo.ID,
		MFAVerified:  true, // API keys don't require MFA
		RateLimit:    keyInfo.RateLimit,
	}
	
	// Create session
	am.sessionManager.Create(authContext.SessionID, authContext)
	
	return authContext, nil
}

// validateHMAC validates HMAC signature
func (am *AuthMiddleware) validateHMAC(ctx context.Context, signature string) (*AuthContext, error) {
	// Extract request details from context
	req := ctx.Value("http_request").(*http.Request)
	
	// Get timestamp from header
	timestamp := req.Header.Get("X-Timestamp")
	if timestamp == "" {
		return nil, fmt.Errorf("missing timestamp header")
	}
	
	// Parse timestamp
	ts, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp format")
	}
	
	// Check timestamp is within tolerance
	if time.Since(ts).Abs() > am.config.HMACTimestampTolerance {
		return nil, fmt.Errorf("timestamp outside tolerance window")
	}
	
	// Build signature payload
	payload := fmt.Sprintf("%s\n%s\n%s\n%s",
		req.Method,
		req.URL.Path,
		req.URL.RawQuery,
		timestamp,
	)
	
	// Calculate expected signature
	h := hmac.New(sha256.New, []byte(am.config.HMACSecret))
	h.Write([]byte(payload))
	expectedSig := hex.EncodeToString(h.Sum(nil))
	
	// Compare signatures
	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return nil, fmt.Errorf("invalid HMAC signature")
	}
	
	// Extract client ID from header
	clientID := req.Header.Get("X-Client-ID")
	if clientID == "" {
		return nil, fmt.Errorf("missing client ID")
	}
	
	// Build auth context
	authContext := &AuthContext{
		AuthType:    AuthTypeHMAC,
		UserID:      clientID,
		AccountID:   clientID,
		Permissions: []string{"api:access"}, // Default permissions for HMAC
		SessionID:   generateSessionID(),
		ExpiresAt:   time.Now().Add(1 * time.Hour),
		MFAVerified: true,
	}
	
	return authContext, nil
}

// validateMTLS validates mutual TLS certificate
func (am *AuthMiddleware) validateMTLS(ctx context.Context, commonName string) (*AuthContext, error) {
	// Lookup certificate mapping
	certInfo, err := am.apiKeyStore.GetByCertCN(commonName)
	if err != nil {
		return nil, err
	}
	
	// Build auth context
	authContext := &AuthContext{
		AuthType:     AuthTypeMTLS,
		UserID:       certInfo.UserID,
		AccountID:    certInfo.AccountID,
		Permissions:  certInfo.Permissions,
		SessionID:    generateSessionID(),
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		MFAVerified:  true,
		CertCN:       commonName,
	}
	
	return authContext, nil
}

// validateGRPCAuth validates gRPC authentication
func (am *AuthMiddleware) validateGRPCAuth(ctx context.Context, md metadata.MD) (*AuthContext, error) {
	// Check for JWT token
	if tokens := md.Get("authorization"); len(tokens) > 0 {
		token := strings.TrimPrefix(tokens[0], "Bearer ")
		return am.validateJWT(ctx, token)
	}
	
	// Check for API key
	if keys := md.Get("x-api-key"); len(keys) > 0 {
		return am.validateAPIKey(ctx, keys[0])
	}
	
	return nil, fmt.Errorf("no valid authentication credentials found")
}

// AuthContext represents authenticated context
type AuthContext struct {
	AuthType     AuthType
	UserID       string
	AccountID    string
	Permissions  []string
	SessionID    string
	ExpiresAt    time.Time
	MFAVerified  bool
	APIKeyID     string
	CertCN       string
	RateLimit    int
	IPAddress    string
	UserAgent    string
}

// HasPermission checks if context has specific permission
func (ac *AuthContext) HasPermission(permission string) bool {
	for _, p := range ac.Permissions {
		if p == permission || p == "*" {
			return true
		}
		// Check wildcard permissions
		if strings.HasSuffix(p, "*") {
			prefix := strings.TrimSuffix(p, "*")
			if strings.HasPrefix(permission, prefix) {
				return true
			}
		}
	}
	return false
}

// authServerStream wraps grpc.ServerStream with auth context
type authServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *authServerStream) Context() context.Context {
	return s.ctx
}

// Helper functions
func extractPermissions(claims jwt.MapClaims) []string {
	if perms, ok := claims["permissions"].([]interface{}); ok {
		permissions := make([]string, len(perms))
		for i, p := range perms {
			permissions[i] = p.(string)
		}
		return permissions
	}
	return []string{}
}

func generateSessionID() string {
	b := make([]byte, 32)
	// In production, use crypto/rand
	return base64.URLEncoding.EncodeToString(b)
}