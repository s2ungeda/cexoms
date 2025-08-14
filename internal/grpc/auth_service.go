package grpc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	omsv1 "github.com/mExOms/oms/pkg/proto/oms/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AuthService implements the gRPC AuthService
type AuthService struct {
	omsv1.UnimplementedAuthServiceServer
	
	// In-memory storage for demo (use database in production)
	ApiKeys     sync.Map // key: apiKey -> APIKeyData
	tokens      sync.Map // key: token -> TokenData
	JwtSecret   []byte
	tokenExpiry time.Duration
}

// APIKeyData stores API key information
type APIKeyData struct {
	ID          string
	Name        string
	Secret      string
	Permissions []omsv1.Permission
	CreatedAt   time.Time
	LastUsed    time.Time
	IsActive    bool
}

// TokenData stores token information
type TokenData struct {
	UserID      string
	Permissions []string
	ExpiresAt   time.Time
}

// NewAuthService creates a new auth service
func NewAuthService() *AuthService {
	// Generate random JWT secret
	secret := make([]byte, 32)
	rand.Read(secret)
	
	return &AuthService{
		JwtSecret:   secret,
		tokenExpiry: 24 * time.Hour,
	}
}

// Authenticate handles authentication requests
func (s *AuthService) Authenticate(ctx context.Context, req *omsv1.AuthRequest) (*omsv1.AuthResponse, error) {
	if req.ApiKey == "" || req.Secret == "" {
		return nil, status.Errorf(codes.InvalidArgument, "api key and secret are required")
	}
	
	// Look up API key
	data, ok := s.ApiKeys.Load(req.ApiKey)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "invalid credentials")
	}
	
	apiKeyData := data.(*APIKeyData)
	
	// Verify secret
	if apiKeyData.Secret != req.Secret {
		return nil, status.Errorf(codes.Unauthenticated, "invalid credentials")
	}
	
	// Check if active
	if !apiKeyData.IsActive {
		return nil, status.Errorf(codes.PermissionDenied, "api key is inactive")
	}
	
	// Update last used
	apiKeyData.LastUsed = time.Now()
	s.ApiKeys.Store(req.ApiKey, apiKeyData)
	
	// Generate JWT token
	token, expiresAt, err := s.generateToken(apiKeyData.ID, apiKeyData.Permissions)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate token")
	}
	
	// Convert permissions to strings
	permissions := make([]string, len(apiKeyData.Permissions))
	for i, p := range apiKeyData.Permissions {
		permissions[i] = p.String()
	}
	
	return &omsv1.AuthResponse{
		Token: token,
		ExpiresAt: &omsv1.Timestamp{
			Seconds: expiresAt.Unix(),
			Nanos:   int32(expiresAt.Nanosecond()),
		},
		Permissions: permissions,
	}, nil
}

// RefreshToken refreshes an authentication token
func (s *AuthService) RefreshToken(ctx context.Context, req *omsv1.RefreshTokenRequest) (*omsv1.RefreshTokenResponse, error) {
	// Parse and validate refresh token
	claims, err := s.validateToken(req.RefreshToken)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid refresh token")
	}
	
	// Generate new tokens
	userID := claims["user_id"].(string)
	permissions := s.getPermissionsFromClaims(claims)
	
	token, expiresAt, err := s.generateToken(userID, permissions)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate token")
	}
	
	refreshToken, _, err := s.generateRefreshToken(userID, permissions)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate refresh token")
	}
	
	return &omsv1.RefreshTokenResponse{
		AccessToken:  token,
		RefreshToken: refreshToken,
		ExpiresAt: &omsv1.Timestamp{
			Seconds: expiresAt.Unix(),
			Nanos:   int32(expiresAt.Nanosecond()),
		},
	}, nil
}

// CreateAPIKey creates a new API key
func (s *AuthService) CreateAPIKey(ctx context.Context, req *omsv1.CreateAPIKeyRequest) (*omsv1.CreateAPIKeyResponse, error) {
	if req.Name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}
	
	// Generate API key and secret
	apiKey := s.generateAPIKey()
	secret := s.generateSecret()
	
	// Create API key data
	apiKeyData := &APIKeyData{
		ID:          apiKey,
		Name:        req.Name,
		Secret:      secret,
		Permissions: req.Permissions,
		CreatedAt:   time.Now(),
		IsActive:    true,
	}
	
	// Store API key
	s.ApiKeys.Store(apiKey, apiKeyData)
	
	return &omsv1.CreateAPIKeyResponse{
		ApiKey: &omsv1.APIKey{
			Id:          apiKey,
			Name:        req.Name,
			Permissions: req.Permissions,
			CreatedAt: &omsv1.Timestamp{
				Seconds: apiKeyData.CreatedAt.Unix(),
				Nanos:   int32(apiKeyData.CreatedAt.Nanosecond()),
			},
			IsActive: true,
		},
		Secret: secret,
	}, nil
}

// ListAPIKeys lists all API keys
func (s *AuthService) ListAPIKeys(ctx context.Context, req *omsv1.ListAPIKeysRequest) (*omsv1.ListAPIKeysResponse, error) {
	var apiKeys []*omsv1.APIKey
	
	s.ApiKeys.Range(func(key, value interface{}) bool {
		data := value.(*APIKeyData)
		apiKeys = append(apiKeys, &omsv1.APIKey{
			Id:          data.ID,
			Name:        data.Name,
			Permissions: data.Permissions,
			CreatedAt: &omsv1.Timestamp{
				Seconds: data.CreatedAt.Unix(),
				Nanos:   int32(data.CreatedAt.Nanosecond()),
			},
			LastUsed: &omsv1.Timestamp{
				Seconds: data.LastUsed.Unix(),
				Nanos:   int32(data.LastUsed.Nanosecond()),
			},
			IsActive: data.IsActive,
		})
		return true
	})
	
	return &omsv1.ListAPIKeysResponse{
		ApiKeys: apiKeys,
	}, nil
}

// RevokeAPIKey revokes an API key
func (s *AuthService) RevokeAPIKey(ctx context.Context, req *omsv1.RevokeAPIKeyRequest) (*omsv1.RevokeAPIKeyResponse, error) {
	data, ok := s.ApiKeys.Load(req.ApiKeyId)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "api key not found")
	}
	
	apiKeyData := data.(*APIKeyData)
	apiKeyData.IsActive = false
	s.ApiKeys.Store(req.ApiKeyId, apiKeyData)
	
	return &omsv1.RevokeAPIKeyResponse{
		Success: true,
	}, nil
}

// Helper methods

func (s *AuthService) generateToken(userID string, permissions []omsv1.Permission) (string, time.Time, error) {
	expiresAt := time.Now().Add(s.tokenExpiry)
	
	// Convert permissions to strings
	permStrings := make([]string, len(permissions))
	for i, p := range permissions {
		permStrings[i] = p.String()
	}
	
	claims := jwt.MapClaims{
		"user_id":     userID,
		"permissions": permStrings,
		"exp":         expiresAt.Unix(),
		"iat":         time.Now().Unix(),
	}
	
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.JwtSecret)
	if err != nil {
		return "", time.Time{}, err
	}
	
	return tokenString, expiresAt, nil
}

func (s *AuthService) generateRefreshToken(userID string, permissions []omsv1.Permission) (string, time.Time, error) {
	expiresAt := time.Now().Add(30 * 24 * time.Hour) // 30 days
	return s.generateToken(userID, permissions)
}

func (s *AuthService) validateToken(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.JwtSecret, nil
	})
	
	if err != nil {
		return nil, err
	}
	
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}
	
	return nil, fmt.Errorf("invalid token")
}

func (s *AuthService) getPermissionsFromClaims(claims jwt.MapClaims) []omsv1.Permission {
	permStrings, ok := claims["permissions"].([]interface{})
	if !ok {
		return nil
	}
	
	permissions := make([]omsv1.Permission, 0, len(permStrings))
	for _, p := range permStrings {
		if str, ok := p.(string); ok {
			// Parse permission string to enum
			if perm, ok := omsv1.Permission_value[str]; ok {
				permissions = append(permissions, omsv1.Permission(perm))
			}
		}
	}
	
	return permissions
}

func (s *AuthService) generateAPIKey() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func (s *AuthService) generateSecret() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}