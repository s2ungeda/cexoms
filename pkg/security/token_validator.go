package security

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenValidator validates JWT tokens
type TokenValidator struct {
	secret       []byte
	publicKey    *rsa.PublicKey
	issuer       string
	audience     []string
	algorithms   []string
	leeway       time.Duration
	cache        *TokenCache
}

// TokenCache caches validated tokens
type TokenCache struct {
	tokens map[string]*CachedToken
	maxAge time.Duration
}

// CachedToken represents a cached token
type CachedToken struct {
	Token     *jwt.Token
	ExpiresAt time.Time
	CreatedAt time.Time
}

// NewTokenValidator creates a new token validator
func NewTokenValidator(secret, issuer string) (*TokenValidator, error) {
	return &TokenValidator{
		secret:     []byte(secret),
		issuer:     issuer,
		algorithms: []string{"HS256", "HS384", "HS512"},
		leeway:     5 * time.Minute,
		cache:      NewTokenCache(5 * time.Minute),
	}, nil
}

// NewTokenValidatorWithRSA creates validator with RSA public key
func NewTokenValidatorWithRSA(publicKeyPEM, issuer string) (*TokenValidator, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}
	
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}
	
	publicKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}
	
	return &TokenValidator{
		publicKey:  publicKey,
		issuer:     issuer,
		algorithms: []string{"RS256", "RS384", "RS512"},
		leeway:     5 * time.Minute,
		cache:      NewTokenCache(5 * time.Minute),
	}, nil
}

// ValidateToken validates a JWT token
func (tv *TokenValidator) ValidateToken(tokenString string) (*jwt.Token, error) {
	// Check cache first
	if cached := tv.cache.Get(tokenString); cached != nil {
		return cached.Token, nil
	}
	
	// Parse token with validation
	token, err := jwt.Parse(tokenString, tv.keyFunc, tv.parserOptions()...)
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}
	
	// Validate claims
	if err := tv.validateClaims(token); err != nil {
		return nil, err
	}
	
	// Cache valid token
	tv.cache.Set(tokenString, token)
	
	return token, nil
}

// keyFunc returns the key for validating token
func (tv *TokenValidator) keyFunc(token *jwt.Token) (interface{}, error) {
	// Validate signing algorithm
	alg := token.Method.Alg()
	validAlg := false
	for _, a := range tv.algorithms {
		if a == alg {
			validAlg = true
			break
		}
	}
	if !validAlg {
		return nil, fmt.Errorf("unexpected signing algorithm: %v", alg)
	}
	
	// Return appropriate key based on algorithm
	if token.Method.Alg() == "HS256" || token.Method.Alg() == "HS384" || token.Method.Alg() == "HS512" {
		return tv.secret, nil
	}
	if token.Method.Alg() == "RS256" || token.Method.Alg() == "RS384" || token.Method.Alg() == "RS512" {
		return tv.publicKey, nil
	}
	
	return nil, fmt.Errorf("unsupported algorithm: %s", alg)
}

// parserOptions returns JWT parser options
func (tv *TokenValidator) parserOptions() []jwt.ParserOption {
	return []jwt.ParserOption{
		jwt.WithLeeway(tv.leeway),
		jwt.WithIssuer(tv.issuer),
		jwt.WithValidMethods(tv.algorithms),
		jwt.WithStrictDecoding(),
	}
}

// validateClaims validates token claims
func (tv *TokenValidator) validateClaims(token *jwt.Token) error {
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return fmt.Errorf("invalid claims type")
	}
	
	// Validate issuer
	if iss, ok := claims["iss"].(string); ok {
		if iss != tv.issuer {
			return fmt.Errorf("invalid issuer: %s", iss)
		}
	} else {
		return fmt.Errorf("missing issuer claim")
	}
	
	// Validate audience if configured
	if len(tv.audience) > 0 {
		if aud, ok := claims["aud"].([]interface{}); ok {
			validAud := false
			for _, a := range aud {
				audStr, ok := a.(string)
				if !ok {
					continue
				}
				for _, expectedAud := range tv.audience {
					if audStr == expectedAud {
						validAud = true
						break
					}
				}
				if validAud {
					break
				}
			}
			if !validAud {
				return fmt.Errorf("invalid audience")
			}
		} else if aud, ok := claims["aud"].(string); ok {
			validAud := false
			for _, expectedAud := range tv.audience {
				if aud == expectedAud {
					validAud = true
					break
				}
			}
			if !validAud {
				return fmt.Errorf("invalid audience: %s", aud)
			}
		} else {
			return fmt.Errorf("missing audience claim")
		}
	}
	
	// Validate required claims
	requiredClaims := []string{"sub", "exp", "iat"}
	for _, claim := range requiredClaims {
		if _, ok := claims[claim]; !ok {
			return fmt.Errorf("missing required claim: %s", claim)
		}
	}
	
	// Validate token is not used before issued
	if iat, ok := claims["iat"].(float64); ok {
		if time.Now().Unix() < int64(iat) {
			return fmt.Errorf("token used before issued")
		}
	}
	
	// Validate not before if present
	if nbf, ok := claims["nbf"].(float64); ok {
		if time.Now().Unix() < int64(nbf) {
			return fmt.Errorf("token not valid yet")
		}
	}
	
	// Custom validation for account_id
	if _, ok := claims["account_id"].(string); !ok {
		return fmt.Errorf("missing account_id claim")
	}
	
	// Validate permissions format
	if perms, ok := claims["permissions"]; ok {
		if _, ok := perms.([]interface{}); !ok {
			return fmt.Errorf("invalid permissions format")
		}
	}
	
	return nil
}

// RefreshToken creates a new token from existing claims
func (tv *TokenValidator) RefreshToken(oldToken *jwt.Token) (string, error) {
	oldClaims, ok := oldToken.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid claims type")
	}
	
	// Create new claims
	newClaims := jwt.MapClaims{
		"iss": tv.issuer,
		"sub": oldClaims["sub"],
		"account_id": oldClaims["account_id"],
		"permissions": oldClaims["permissions"],
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(30 * time.Minute).Unix(),
	}
	
	// Add audience if configured
	if len(tv.audience) > 0 {
		newClaims["aud"] = tv.audience
	}
	
	// Create new token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, newClaims)
	
	// Sign token
	tokenString, err := token.SignedString(tv.secret)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}
	
	return tokenString, nil
}

// NewTokenCache creates a new token cache
func NewTokenCache(maxAge time.Duration) *TokenCache {
	tc := &TokenCache{
		tokens: make(map[string]*CachedToken),
		maxAge: maxAge,
	}
	
	// Start cleanup goroutine
	go tc.cleanup()
	
	return tc
}

// Get retrieves token from cache
func (tc *TokenCache) Get(tokenString string) *CachedToken {
	cached, ok := tc.tokens[tokenString]
	if !ok {
		return nil
	}
	
	// Check if expired
	if time.Now().After(cached.ExpiresAt) {
		delete(tc.tokens, tokenString)
		return nil
	}
	
	return cached
}

// Set adds token to cache
func (tc *TokenCache) Set(tokenString string, token *jwt.Token) {
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return
	}
	
	exp, ok := claims["exp"].(float64)
	if !ok {
		return
	}
	
	tc.tokens[tokenString] = &CachedToken{
		Token:     token,
		ExpiresAt: time.Unix(int64(exp), 0),
		CreatedAt: time.Now(),
	}
}

// cleanup removes expired tokens
func (tc *TokenCache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for {
		<-ticker.C
		now := time.Now()
		
		for key, cached := range tc.tokens {
			if now.After(cached.ExpiresAt) || now.Sub(cached.CreatedAt) > tc.maxAge {
				delete(tc.tokens, key)
			}
		}
	}
}