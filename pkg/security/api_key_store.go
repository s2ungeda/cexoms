package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// APIKeyInfo represents API key information
type APIKeyInfo struct {
	ID          string
	Key         string // Hashed key
	UserID      string
	AccountID   string
	Name        string
	Description string
	Permissions []string
	Active      bool
	ExpiresAt   time.Time
	CreatedAt   time.Time
	LastUsedAt  time.Time
	RateLimit   int // Requests per minute
	IPWhitelist []string
	Scopes      []string
}

// APIKeyStore manages API keys
type APIKeyStore struct {
	mu    sync.RWMutex
	keys  map[string]*APIKeyInfo // key hash -> info
	users map[string][]*APIKeyInfo // userID -> keys
}

// NewAPIKeyStore creates a new API key store
func NewAPIKeyStore() *APIKeyStore {
	return &APIKeyStore{
		keys:  make(map[string]*APIKeyInfo),
		users: make(map[string][]*APIKeyInfo),
	}
}

// Generate creates a new API key
func (aks *APIKeyStore) Generate(userID, accountID, name string, permissions []string) (*APIKeyInfo, string, error) {
	// Generate random key
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, "", fmt.Errorf("failed to generate random key: %w", err)
	}
	
	// Create readable key with prefix
	rawKey := fmt.Sprintf("mex_%s", hex.EncodeToString(keyBytes))
	
	// Hash key for storage
	hashedKey := aks.hashKey(rawKey)
	
	// Create key info
	keyInfo := &APIKeyInfo{
		ID:          generateID(),
		Key:         hashedKey,
		UserID:      userID,
		AccountID:   accountID,
		Name:        name,
		Permissions: permissions,
		Active:      true,
		ExpiresAt:   time.Now().Add(365 * 24 * time.Hour), // 1 year default
		CreatedAt:   time.Now(),
		RateLimit:   1000, // Default 1000 req/min
		IPWhitelist: []string{},
		Scopes:      []string{"trading", "read"},
	}
	
	// Store key
	aks.mu.Lock()
	defer aks.mu.Unlock()
	
	aks.keys[hashedKey] = keyInfo
	
	// Add to user's keys
	if aks.users[userID] == nil {
		aks.users[userID] = make([]*APIKeyInfo, 0)
	}
	aks.users[userID] = append(aks.users[userID], keyInfo)
	
	return keyInfo, rawKey, nil
}

// Get retrieves API key info
func (aks *APIKeyStore) Get(rawKey string) (*APIKeyInfo, error) {
	hashedKey := aks.hashKey(rawKey)
	
	aks.mu.RLock()
	defer aks.mu.RUnlock()
	
	keyInfo, exists := aks.keys[hashedKey]
	if !exists {
		return nil, fmt.Errorf("API key not found")
	}
	
	// Update last used timestamp
	go aks.updateLastUsed(hashedKey)
	
	return keyInfo, nil
}

// GetByID retrieves API key by ID
func (aks *APIKeyStore) GetByID(keyID string) (*APIKeyInfo, error) {
	aks.mu.RLock()
	defer aks.mu.RUnlock()
	
	for _, keyInfo := range aks.keys {
		if keyInfo.ID == keyID {
			return keyInfo, nil
		}
	}
	
	return nil, fmt.Errorf("API key not found")
}

// GetUserKeys retrieves all keys for a user
func (aks *APIKeyStore) GetUserKeys(userID string) ([]*APIKeyInfo, error) {
	aks.mu.RLock()
	defer aks.mu.RUnlock()
	
	keys, exists := aks.users[userID]
	if !exists {
		return []*APIKeyInfo{}, nil
	}
	
	// Return copy to prevent modification
	result := make([]*APIKeyInfo, len(keys))
	copy(result, keys)
	
	return result, nil
}

// Revoke deactivates an API key
func (aks *APIKeyStore) Revoke(keyID string) error {
	aks.mu.Lock()
	defer aks.mu.Unlock()
	
	for _, keyInfo := range aks.keys {
		if keyInfo.ID == keyID {
			keyInfo.Active = false
			return nil
		}
	}
	
	return fmt.Errorf("API key not found")
}

// Delete permanently removes an API key
func (aks *APIKeyStore) Delete(keyID string) error {
	aks.mu.Lock()
	defer aks.mu.Unlock()
	
	var keyToDelete *APIKeyInfo
	var hashedKey string
	
	// Find key to delete
	for hash, keyInfo := range aks.keys {
		if keyInfo.ID == keyID {
			keyToDelete = keyInfo
			hashedKey = hash
			break
		}
	}
	
	if keyToDelete == nil {
		return fmt.Errorf("API key not found")
	}
	
	// Remove from keys map
	delete(aks.keys, hashedKey)
	
	// Remove from user's keys
	if userKeys, exists := aks.users[keyToDelete.UserID]; exists {
		newUserKeys := make([]*APIKeyInfo, 0, len(userKeys)-1)
		for _, k := range userKeys {
			if k.ID != keyID {
				newUserKeys = append(newUserKeys, k)
			}
		}
		aks.users[keyToDelete.UserID] = newUserKeys
	}
	
	return nil
}

// Update updates API key properties
func (aks *APIKeyStore) Update(keyID string, updates map[string]interface{}) error {
	aks.mu.Lock()
	defer aks.mu.Unlock()
	
	var keyInfo *APIKeyInfo
	for _, k := range aks.keys {
		if k.ID == keyID {
			keyInfo = k
			break
		}
	}
	
	if keyInfo == nil {
		return fmt.Errorf("API key not found")
	}
	
	// Apply updates
	for field, value := range updates {
		switch field {
		case "name":
			if name, ok := value.(string); ok {
				keyInfo.Name = name
			}
		case "description":
			if desc, ok := value.(string); ok {
				keyInfo.Description = desc
			}
		case "permissions":
			if perms, ok := value.([]string); ok {
				keyInfo.Permissions = perms
			}
		case "rate_limit":
			if limit, ok := value.(int); ok {
				keyInfo.RateLimit = limit
			}
		case "ip_whitelist":
			if ips, ok := value.([]string); ok {
				keyInfo.IPWhitelist = ips
			}
		case "scopes":
			if scopes, ok := value.([]string); ok {
				keyInfo.Scopes = scopes
			}
		case "expires_at":
			if exp, ok := value.(time.Time); ok {
				keyInfo.ExpiresAt = exp
			}
		}
	}
	
	return nil
}

// GetByCertCN retrieves key info by certificate common name
func (aks *APIKeyStore) GetByCertCN(commonName string) (*APIKeyInfo, error) {
	// In production, this would lookup certificate mappings
	// For now, return a mock response
	return &APIKeyInfo{
		ID:          "cert-" + commonName,
		UserID:      "cert-user",
		AccountID:   "cert-account",
		Permissions: []string{"api:access"},
		Active:      true,
		ExpiresAt:   time.Now().Add(365 * 24 * time.Hour),
	}, nil
}

// Rotate generates a new key while keeping the same permissions
func (aks *APIKeyStore) Rotate(keyID string) (*APIKeyInfo, string, error) {
	// Get existing key
	oldKey, err := aks.GetByID(keyID)
	if err != nil {
		return nil, "", err
	}
	
	// Generate new key with same properties
	newKey, rawKey, err := aks.Generate(
		oldKey.UserID,
		oldKey.AccountID,
		oldKey.Name + " (rotated)",
		oldKey.Permissions,
	)
	if err != nil {
		return nil, "", err
	}
	
	// Copy additional properties
	newKey.Description = oldKey.Description + " (rotated from " + oldKey.ID + ")"
	newKey.RateLimit = oldKey.RateLimit
	newKey.IPWhitelist = oldKey.IPWhitelist
	newKey.Scopes = oldKey.Scopes
	
	// Revoke old key
	if err := aks.Revoke(keyID); err != nil {
		// Log error but don't fail rotation
		fmt.Printf("Failed to revoke old key: %v\n", err)
	}
	
	return newKey, rawKey, nil
}

// CleanupExpired removes expired keys
func (aks *APIKeyStore) CleanupExpired() int {
	aks.mu.Lock()
	defer aks.mu.Unlock()
	
	now := time.Now()
	count := 0
	
	for hash, keyInfo := range aks.keys {
		if now.After(keyInfo.ExpiresAt) {
			// Remove from keys map
			delete(aks.keys, hash)
			
			// Remove from user's keys
			if userKeys, exists := aks.users[keyInfo.UserID]; exists {
				newUserKeys := make([]*APIKeyInfo, 0, len(userKeys))
				for _, k := range userKeys {
					if k.ID != keyInfo.ID {
						newUserKeys = append(newUserKeys, k)
					}
				}
				aks.users[keyInfo.UserID] = newUserKeys
			}
			
			count++
		}
	}
	
	return count
}

// hashKey hashes an API key
func (aks *APIKeyStore) hashKey(rawKey string) string {
	hash := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(hash[:])
}

// updateLastUsed updates the last used timestamp
func (aks *APIKeyStore) updateLastUsed(hashedKey string) {
	aks.mu.Lock()
	defer aks.mu.Unlock()
	
	if keyInfo, exists := aks.keys[hashedKey]; exists {
		keyInfo.LastUsedAt = time.Now()
	}
}

// generateID generates a unique ID
func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}