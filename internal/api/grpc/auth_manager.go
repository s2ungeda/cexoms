package grpc

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dgrijalva/jwt-go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/mExOms/proto/oms/v1"
)

// AuthManager handles authentication and authorization
type AuthManager struct {
	jwtSecret     []byte
	tokenDuration time.Duration
	
	// API key storage (in production, use database)
	apiKeys sync.Map // apiKey -> *APIKeyInfo
	
	// Token blacklist for revoked tokens
	blacklist sync.Map // tokenID -> revokedAt
	
	// Active sessions
	sessions sync.Map // sessionID -> *Session
}

// APIKeyInfo stores API key information
type APIKeyInfo struct {
	ID                 string
	Name               string
	Secret             string
	Permissions        []pb.Permission
	AccountPermissions []*AccountPermission
	RateLimit          *RateLimitConfig
	IPWhitelist        []string
	IsActive           bool
	CreatedAt          time.Time
	LastUsed           time.Time
}

// AccountPermission represents account-specific permissions
type AccountPermission struct {
	AccountID   string
	Permissions []pb.Permission
}

// RateLimitConfig defines rate limiting rules
type RateLimitConfig struct {
	RequestsPerSecond int32
	RequestsPerMinute int32
	RequestsPerHour   int32
	OrderWeight       int32
	QueryWeight       int32
}

// Session represents an active authentication session
type Session struct {
	ID                 string
	UserID             string
	APIKeyID           string
	Permissions        []pb.Permission
	AccountPermissions [ественный]*AccountPermission
	ExpiresAt          time.Time
}

// Claims represents JWT claims
type Claims struct {
	UserID             string                 `json:"user_id"`
	SessionID          string                 `json:"session_id"`
	APIKeyID           string                 `json:"api_key_id"`
	Permissions        []string               `json:"permissions"`
	AccountPermissions []map[string][]string  `json:"account_permissions"`
	jwt.StandardClaims
}

// NewAuthManager creates a new authentication manager
func NewAuthManager(jwtSecret string, tokenDuration time.Duration) *AuthManager {
	return &AuthManager{
		jwtSecret:     []byte(jwtSecret),
		tokenDuration: tokenDuration,
	}
}

// CreateAPIKey creates a new API key
func (am *AuthManager) CreateAPIKey(name string, permissions []pb.Permission, accountPerms []*AccountPermission) (*APIKeyInfo, string, error) {
	// Generate API key and secret
	apiKeyID := generateID("ak_")
	apiKey := generateAPIKey()
	apiSecret := generateAPISecret()
	
	// Create API key info
	info := &APIKeyInfo{
		ID:                 apiKeyID,
		Name:               name,
		Secret:             hashSecret(apiSecret),
		Permissions:        permissions,
		AccountPermissions: accountPerms,
		RateLimit: &RateLimitConfig{
			RequestsPerSecond: 100,
			RequestsPerMinute: 1000,
			RequestsPerHour:   50000,
			OrderWeight:       10,
			QueryWeight:       1,
		},
		IsActive:  true,
		CreatedAt: time.Now(),
	}
	
	// Store API key
	am.apiKeys.Store(apiKey, info)
	
	return info, apiKey + "." + apiSecret, nil
}

// ValidateAPIKey validates an API key
func (am *AuthManager) ValidateAPIKey(apiKeyWithSecret string) (*APIKeyInfo, error) {
	parts := strings.Split(apiKeyWithSecret, ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid API key format")
	}
	
	apiKey, apiSecret := parts[0], parts[1]
	
	// Get API key info
	value, ok := am.apiKeys.Load(apiKey)
	if !ok {
		return nil, fmt.Errorf("API key not found")
	}
	
	info := value.(*APIKeyInfo)
	
	// Check if active
	if !info.IsActive {
		return nil, fmt.Errorf("API key is inactive")
	}
	
	// Verify secret
	if !verifySecret(apiSecret, info.Secret) {
		return nil, fmt.Errorf("invalid API secret")
	}
	
	// Update last used
	info.LastUsed = time.Now()
	
	return info, nil
}

// GenerateToken generates a JWT token
func (am *AuthManager) GenerateToken(apiKeyInfo *APIKeyInfo) (string, error) {
	// Create session
	sessionID := generateID("sess_")
	session := &Session{
		ID:                 sessionID,
		UserID:             apiKeyInfo.ID,
		APIKeyID:           apiKeyInfo.ID,
		Permissions:        apiKeyInfo.Permissions,
		AccountPermissions: apiKeyInfo.AccountPermissions,
		ExpiresAt:          time.Now().Add(am.tokenDuration),
	}
	
	// Store session
	am.sessions.Store(sessionID, session)
	
	// Create claims
	claims := Claims{
		UserID:      apiKeyInfo.ID,
		SessionID:   sessionID,
		APIKeyID:    apiKeyInfo.ID,
		Permissions: permissionsToStrings(apiKeyInfo.Permissions),
		AccountPermissions: accountPermsToMap(apiKeyInfo.AccountPermissions),
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: session.ExpiresAt.Unix(),
			IssuedAt:  time.Now().Unix(),
			Id:        sessionID,
		},
	}
	
	// Create token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(am.jwtSecret)
	if err != nil {
		return "", err
	}
	
	return tokenString, nil
}

// ValidateToken validates a JWT token
func (am *AuthManager) ValidateToken(tokenString string) (*Claims, error) {
	// Remove Bearer prefix if present
	tokenString = strings.TrimPrefix(tokenString, "Bearer ")
	
	// Parse token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return am.jwtSecret, nil
	})
	
	if err != nil {
		return nil, err
	}
	
	// Get claims
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	
	// Check if token is blacklisted
	if _, blacklisted := am.blacklist.Load(claims.SessionID); blacklisted {
		return nil, fmt.Errorf("token has been revoked")
	}
	
	// Check if session is active
	value, ok := am.sessions.Load(claims.SessionID)
	if !ok {
		return nil, fmt.Errorf("session not found")
	}
	
	session := value.(*Session)
	if time.Now().After(session.ExpiresAt) {
		return nil, fmt.Errorf("session expired")
	}
	
	return claims, nil
}

// RevokeToken revokes a token
func (am *AuthManager) RevokeToken(sessionID string) error {
	// Add to blacklist
	am.blacklist.Store(sessionID, time.Now())
	
	// Remove session
	am.sessions.Delete(sessionID)
	
	return nil
}

// CheckPermission checks if a user has the required permission
func (am *AuthManager) CheckPermission(claims *Claims, requiredPerm pb.Permission, accountID string) bool {
	// Check global permissions
	for _, perm := range claims.Permissions {
		if perm == requiredPerm.String() || perm == pb.Permission_PERMISSION_ADMIN.String() {
			return true
		}
	}
	
	// Check account-specific permissions
	if accountID != "" {
		for _, accPerm := range claims.AccountPermissions {
			if perms, ok := accPerm[accountID]; ok {
				for _, perm := range perms {
					if perm == requiredPerm.String() || perm == pb.Permission_PERMISSION_ADMIN.String() {
						return true
					}
				}
			}
		}
	}
	
	return false
}

// ExtractClaimsFromContext extracts claims from gRPC context
func ExtractClaimsFromContext(ctx context.Context) (*Claims, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}
	
	tokens := md.Get("authorization")
	if len(tokens) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing authorization token")
	}
	
	// Token validation should be done in interceptor
	// Here we just extract from context
	claims, ok := ctx.Value("claims").(*Claims)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing claims in context")
	}
	
	return claims, nil
}

// GetAPIKeyFromContext extracts API key from context
func GetAPIKeyFromContext(ctx context.Context, header string) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", fmt.Errorf("missing metadata")
	}
	
	keys := md.Get(header)
	if len(keys) == 0 {
		return "", fmt.Errorf("missing API key")
	}
	
	return keys[0], nil
}

// Helper functions

func generateID(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
}

func generateAPIKey() string {
	// In production, use a secure random generator
	return fmt.Sprintf("mx_%s", generateRandomString(32))
}

func generateAPISecret() string {
	// In production, use a secure random generator
	return generateRandomString(64)
}

func hashSecret(secret string) string {
	h := sha256.New()
	h.Write([]byte(secret))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func verifySecret(secret, hash string) bool {
	return hmac.Equal([]byte(hashSecret(secret)), []byte(hash))
}

func generateRandomString(length int) string {
	// In production, use crypto/rand
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

func permissionsToStrings(perms []pb.Permission) []string {
	result := make([]string, len(perms))
	for i, p := range perms {
		result[i] = p.String()
	}
	return result
}

func accountPermsToMap(perms []*AccountPermission) []map[string][]string {
	result := make([]map[string][]string, len(perms))
	for i, p := range perms {
		result[i] = map[string][]string{
			p.AccountID: permissionsToStrings(p.Permissions),
		}
	}
	return result
}

// CleanupExpiredSessions removes expired sessions
func (am *AuthManager) CleanupExpiredSessions() {
	now := time.Now()
	am.sessions.Range(func(key, value interface{}) bool {
		session := value.(*Session)
		if now.After(session.ExpiresAt) {
			am.sessions.Delete(key)
		}
		return true
	})
}

// StartCleanup starts periodic cleanup of expired data
func (am *AuthManager) StartCleanup(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			am.CleanupExpiredSessions()
			
			// Cleanup old blacklisted tokens (older than 24h)
			cutoff := time.Now().Add(-24 * time.Hour)
			am.blacklist.Range(func(key, value interface{}) bool {
				if revokedAt := value.(time.Time); revokedAt.Before(cutoff) {
					am.blacklist.Delete(key)
				}
				return true
			})
			
		case <-ctx.Done():
			return
		}
	}
}