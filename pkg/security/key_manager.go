package security

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// KeyManager manages cryptographic keys lifecycle
type KeyManager struct {
	mu           sync.RWMutex
	keys         map[string]*ManagedKey
	vaultManager *VaultManager
	rotationInterval time.Duration
	stopRotation chan bool
}

// ManagedKey represents a managed cryptographic key
type ManagedKey struct {
	ID           string
	Type         KeyType
	Value        []byte
	CreatedAt    time.Time
	LastRotated  time.Time
	NextRotation time.Time
	Version      int
	Metadata     map[string]string
	Active       bool
}

// KeyType represents the type of key
type KeyType string

const (
	KeyTypeDataEncryption KeyType = "data_encryption"
	KeyTypeAPIKey         KeyType = "api_key"
	KeyTypeJWTSigning     KeyType = "jwt_signing"
	KeyTypeHMAC           KeyType = "hmac"
	KeyTypeTLS            KeyType = "tls"
)

// NewKeyManager creates a new key manager
func NewKeyManager(vaultManager *VaultManager, rotationInterval time.Duration) *KeyManager {
	km := &KeyManager{
		keys:             make(map[string]*ManagedKey),
		vaultManager:     vaultManager,
		rotationInterval: rotationInterval,
		stopRotation:     make(chan bool),
	}
	
	// Start key rotation scheduler
	go km.rotationScheduler()
	
	return km
}

// GenerateKey generates a new key
func (km *KeyManager) GenerateKey(keyType KeyType, metadata map[string]string) (*ManagedKey, error) {
	// Generate key based on type
	keySize := km.getKeySize(keyType)
	keyValue := make([]byte, keySize)
	
	if _, err := rand.Read(keyValue); err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}
	
	// Create managed key
	key := &ManagedKey{
		ID:           generateKeyID(keyType),
		Type:         keyType,
		Value:        keyValue,
		CreatedAt:    time.Now(),
		LastRotated:  time.Now(),
		NextRotation: time.Now().Add(km.rotationInterval),
		Version:      1,
		Metadata:     metadata,
		Active:       true,
	}
	
	// Store key in vault
	if err := km.storeKeyInVault(key); err != nil {
		return nil, fmt.Errorf("failed to store key in vault: %w", err)
	}
	
	// Store key reference
	km.mu.Lock()
	km.keys[key.ID] = key
	km.mu.Unlock()
	
	return key, nil
}

// GetKey retrieves a key by ID
func (km *KeyManager) GetKey(keyID string) (*ManagedKey, error) {
	km.mu.RLock()
	key, exists := km.keys[keyID]
	km.mu.RUnlock()
	
	if !exists {
		// Try to load from vault
		key, err := km.loadKeyFromVault(keyID)
		if err != nil {
			return nil, fmt.Errorf("key not found: %s", keyID)
		}
		
		// Cache the key
		km.mu.Lock()
		km.keys[keyID] = key
		km.mu.Unlock()
		
		return key, nil
	}
	
	if !key.Active {
		return nil, fmt.Errorf("key is inactive: %s", keyID)
	}
	
	return key, nil
}

// RotateKey rotates a specific key
func (km *KeyManager) RotateKey(keyID string) (*ManagedKey, error) {
	km.mu.Lock()
	defer km.mu.Unlock()
	
	oldKey, exists := km.keys[keyID]
	if !exists {
		return nil, fmt.Errorf("key not found: %s", keyID)
	}
	
	// Generate new key value
	newKeyValue := make([]byte, len(oldKey.Value))
	if _, err := rand.Read(newKeyValue); err != nil {
		return nil, fmt.Errorf("failed to generate new key: %w", err)
	}
	
	// Create new version
	newKey := &ManagedKey{
		ID:           keyID,
		Type:         oldKey.Type,
		Value:        newKeyValue,
		CreatedAt:    oldKey.CreatedAt,
		LastRotated:  time.Now(),
		NextRotation: time.Now().Add(km.rotationInterval),
		Version:      oldKey.Version + 1,
		Metadata:     oldKey.Metadata,
		Active:       true,
	}
	
	// Store new version in vault
	if err := km.storeKeyInVault(newKey); err != nil {
		return nil, fmt.Errorf("failed to store rotated key: %w", err)
	}
	
	// Archive old version
	if err := km.archiveKeyVersion(oldKey); err != nil {
		return nil, fmt.Errorf("failed to archive old key version: %w", err)
	}
	
	// Update reference
	km.keys[keyID] = newKey
	
	return newKey, nil
}

// ListKeys lists all active keys
func (km *KeyManager) ListKeys() ([]*ManagedKey, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()
	
	keys := make([]*ManagedKey, 0, len(km.keys))
	for _, key := range km.keys {
		if key.Active {
			// Return key without value for security
			keyCopy := *key
			keyCopy.Value = nil
			keys = append(keys, &keyCopy)
		}
	}
	
	return keys, nil
}

// DeactivateKey deactivates a key
func (km *KeyManager) DeactivateKey(keyID string) error {
	km.mu.Lock()
	defer km.mu.Unlock()
	
	key, exists := km.keys[keyID]
	if !exists {
		return fmt.Errorf("key not found: %s", keyID)
	}
	
	key.Active = false
	
	// Update in vault
	if err := km.updateKeyInVault(key); err != nil {
		return fmt.Errorf("failed to deactivate key in vault: %w", err)
	}
	
	return nil
}

// rotationScheduler handles automatic key rotation
func (km *KeyManager) rotationScheduler() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			km.checkAndRotateKeys()
		case <-km.stopRotation:
			return
		}
	}
}

// checkAndRotateKeys checks and rotates keys that need rotation
func (km *KeyManager) checkAndRotateKeys() {
	km.mu.RLock()
	keysToRotate := make([]string, 0)
	now := time.Now()
	
	for keyID, key := range km.keys {
		if key.Active && now.After(key.NextRotation) {
			keysToRotate = append(keysToRotate, keyID)
		}
	}
	km.mu.RUnlock()
	
	// Rotate keys
	for _, keyID := range keysToRotate {
		if _, err := km.RotateKey(keyID); err != nil {
			// Log error but continue with other keys
			fmt.Printf("Failed to rotate key %s: %v\n", keyID, err)
		}
	}
}

// getKeySize returns appropriate key size for key type
func (km *KeyManager) getKeySize(keyType KeyType) int {
	switch keyType {
	case KeyTypeDataEncryption:
		return 32 // 256 bits
	case KeyTypeJWTSigning:
		return 64 // 512 bits for HS512
	case KeyTypeHMAC:
		return 32 // 256 bits
	case KeyTypeAPIKey:
		return 32 // 256 bits
	default:
		return 32
	}
}

// storeKeyInVault stores key in vault
func (km *KeyManager) storeKeyInVault(key *ManagedKey) error {
	if km.vaultManager == nil {
		// If no vault, just return (for testing)
		return nil
	}
	
	path := fmt.Sprintf("keys/%s/v%d", key.ID, key.Version)
	data := map[string]interface{}{
		"id":           key.ID,
		"type":         string(key.Type),
		"value":        hex.EncodeToString(key.Value),
		"created_at":   key.CreatedAt,
		"last_rotated": key.LastRotated,
		"version":      key.Version,
		"metadata":     key.Metadata,
		"active":       key.Active,
	}
	
	return km.vaultManager.StoreSecret(path, data)
}

// loadKeyFromVault loads key from vault
func (km *KeyManager) loadKeyFromVault(keyID string) (*ManagedKey, error) {
	if km.vaultManager == nil {
		return nil, fmt.Errorf("vault not configured")
	}
	
	// Try to find the latest version
	for version := 10; version > 0; version-- {
		path := fmt.Sprintf("keys/%s/v%d", keyID, version)
		data, err := km.vaultManager.GetSecret(path)
		if err != nil {
			continue
		}
		
		// Parse key data
		keyValue, err := hex.DecodeString(data["value"].(string))
		if err != nil {
			return nil, fmt.Errorf("failed to decode key value: %w", err)
		}
		
		key := &ManagedKey{
			ID:           data["id"].(string),
			Type:         KeyType(data["type"].(string)),
			Value:        keyValue,
			CreatedAt:    data["created_at"].(time.Time),
			LastRotated:  data["last_rotated"].(time.Time),
			Version:      data["version"].(int),
			Active:       data["active"].(bool),
		}
		
		if metadata, ok := data["metadata"].(map[string]string); ok {
			key.Metadata = metadata
		}
		
		return key, nil
	}
	
	return nil, fmt.Errorf("key not found in vault")
}

// updateKeyInVault updates key in vault
func (km *KeyManager) updateKeyInVault(key *ManagedKey) error {
	return km.storeKeyInVault(key)
}

// archiveKeyVersion archives old key version
func (km *KeyManager) archiveKeyVersion(key *ManagedKey) error {
	if km.vaultManager == nil {
		return nil
	}
	
	archivePath := fmt.Sprintf("keys/archive/%s/v%d", key.ID, key.Version)
	data := map[string]interface{}{
		"id":           key.ID,
		"type":         string(key.Type),
		"value":        hex.EncodeToString(key.Value),
		"created_at":   key.CreatedAt,
		"last_rotated": key.LastRotated,
		"archived_at":  time.Now(),
		"version":      key.Version,
		"metadata":     key.Metadata,
	}
	
	return km.vaultManager.StoreSecret(archivePath, data)
}

// generateKeyID generates unique key ID
func generateKeyID(keyType KeyType) string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%s_%s", keyType, hex.EncodeToString(b))
}

// Stop stops the key manager
func (km *KeyManager) Stop() {
	close(km.stopRotation)
}