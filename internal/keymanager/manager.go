package keymanager

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Manager manages API keys across multiple accounts
type Manager struct {
	mu            sync.RWMutex
	vaultClient   *VaultClient
	config        KeyManagerConfig
	cache         *KeyCache
	rotator       *KeyRotator
	auditor       *Auditor
	usageTracker  *UsageTracker
	encryptionKey []byte
}

// KeyCache provides in-memory caching for frequently accessed keys
type KeyCache struct {
	mu    sync.RWMutex
	keys  map[string]*CachedKey
	ttl   time.Duration
}

// CachedKey represents a cached API key
type CachedKey struct {
	Key       *APIKey
	CachedAt  time.Time
	ExpiresAt time.Time
}

// NewManager creates a new key manager
func NewManager(config KeyManagerConfig) (*Manager, error) {
	// Create Vault client
	vaultClient, err := NewVaultClient(config.VaultConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	// Generate encryption key for local operations
	encKey := make([]byte, 32) // AES-256
	if _, err := rand.Read(encKey); err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}

	m := &Manager{
		vaultClient:   vaultClient,
		config:        config,
		encryptionKey: encKey,
	}

	// Initialize cache if enabled
	if config.CacheEnabled {
		m.cache = &KeyCache{
			keys: make(map[string]*CachedKey),
			ttl:  config.CacheTTL,
		}
	}

	// Initialize auditor
	if config.AuditEnabled {
		m.auditor = NewAuditor(config.AuditLogPath)
	}

	// Initialize usage tracker
	m.usageTracker = NewUsageTracker()

	// Initialize rotator
	m.rotator = NewKeyRotator(m, config.RotationPolicy)
	if err := m.rotator.Start(); err != nil {
		return nil, fmt.Errorf("failed to start key rotator: %w", err)
	}

	// Start health checker
	go m.healthCheckLoop()

	return m, nil
}

// StoreKey stores a new API key
func (m *Manager) StoreKey(ctx context.Context, key *APIKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate key
	if err := m.validateKey(key); err != nil {
		return fmt.Errorf("invalid key: %w", err)
	}

	// Generate ID if not provided
	if key.ID == "" {
		key.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	key.CreatedAt = now
	key.UpdatedAt = now

	// Encrypt sensitive data for local storage
	encryptedKey := key // Copy
	if err := m.encryptSensitiveData(&encryptedKey); err != nil {
		return fmt.Errorf("failed to encrypt key: %w", err)
	}

	// Store in Vault
	if err := m.vaultClient.StoreKey(ctx, &encryptedKey); err != nil {
		return fmt.Errorf("failed to store key in vault: %w", err)
	}

	// Audit log
	if m.auditor != nil {
		m.auditor.LogAction(ctx, "create", key.ID, true, map[string]interface{}{
			"account_name": key.AccountName,
			"exchange":     key.Exchange,
			"market":       key.Market,
		})
	}

	// Clear cache for this account
	if m.cache != nil {
		m.clearAccountCache(key.AccountName)
	}

	return nil
}

// GetKey retrieves an API key
func (m *Manager) GetKey(ctx context.Context, request KeyRequest) (*APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check cache first
	cacheKey := m.buildCacheKey(request.AccountName, request.Exchange, request.Market)
	if m.cache != nil {
		if cached := m.getFromCache(cacheKey); cached != nil {
			// Track usage
			m.usageTracker.TrackRequest(cached.ID, request.AccountName, true)
			return cached, nil
		}
	}

	// Get from Vault
	key, err := m.vaultClient.GetKey(ctx, request.AccountName, request.Exchange, request.Market)
	if err != nil {
		// Track failed request
		m.usageTracker.TrackRequest("", request.AccountName, false)
		
		// Audit log
		if m.auditor != nil {
			m.auditor.LogAction(ctx, "read", cacheKey, false, map[string]interface{}{
				"error": err.Error(),
			})
		}
		
		return nil, err
	}

	// Decrypt sensitive data
	if err := m.decryptSensitiveData(key); err != nil {
		return nil, fmt.Errorf("failed to decrypt key: %w", err)
	}

	// Check if key is valid
	if err := m.validateKeyStatus(key); err != nil {
		return nil, err
	}

	// Update cache
	if m.cache != nil {
		m.addToCache(cacheKey, key)
	}

	// Track usage
	m.usageTracker.TrackRequest(key.ID, request.AccountName, true)

	// Update last used
	go m.updateLastUsed(ctx, key)

	// Audit log
	if m.auditor != nil {
		m.auditor.LogAction(ctx, "read", key.ID, true, map[string]interface{}{
			"account_name": key.AccountName,
		})
	}

	return key, nil
}

// ListKeys lists all keys for an account
func (m *Manager) ListKeys(ctx context.Context, accountName string) ([]*KeyMetadata, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metadata, err := m.vaultClient.ListKeys(ctx, accountName)
	if err != nil {
		return nil, err
	}

	// Audit log
	if m.auditor != nil {
		m.auditor.LogAction(ctx, "list", fmt.Sprintf("account:%s", accountName), err == nil, nil)
	}

	return metadata, nil
}

// UpdateKey updates an existing key
func (m *Manager) UpdateKey(ctx context.Context, key *APIKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate
	if err := m.validateKey(key); err != nil {
		return err
	}

	// Update timestamp
	key.UpdatedAt = time.Now()

	// Encrypt and store
	encryptedKey := *key
	if err := m.encryptSensitiveData(&encryptedKey); err != nil {
		return err
	}

	if err := m.vaultClient.StoreKey(ctx, &encryptedKey); err != nil {
		return err
	}

	// Clear cache
	if m.cache != nil {
		m.clearAccountCache(key.AccountName)
	}

	// Audit log
	if m.auditor != nil {
		m.auditor.LogAction(ctx, "update", key.ID, true, map[string]interface{}{
			"account_name": key.AccountName,
		})
	}

	return nil
}

// DeleteKey deletes an API key
func (m *Manager) DeleteKey(ctx context.Context, accountName, exchange, market string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get key first for audit purposes
	key, err := m.vaultClient.GetKey(ctx, accountName, exchange, market)
	if err != nil {
		return err
	}

	// Delete from Vault
	if err := m.vaultClient.DeleteKey(ctx, accountName, exchange, market); err != nil {
		return err
	}

	// Clear cache
	if m.cache != nil {
		cacheKey := m.buildCacheKey(accountName, exchange, market)
		m.removeFromCache(cacheKey)
	}

	// Audit log
	if m.auditor != nil {
		m.auditor.LogAction(ctx, "delete", key.ID, true, map[string]interface{}{
			"account_name": accountName,
			"exchange":     exchange,
			"market":       market,
		})
	}

	return nil
}

// RevokeKey immediately revokes a key
func (m *Manager) RevokeKey(ctx context.Context, keyID string, reason string) error {
	// Get key by ID
	key, err := m.GetKeyByID(ctx, keyID)
	if err != nil {
		return err
	}

	// Mark as inactive
	key.IsActive = false
	key.UpdatedAt = time.Now()

	// Add revocation metadata
	if key.Tags == nil {
		key.Tags = make(map[string]string)
	}
	key.Tags["revoked_at"] = time.Now().Format(time.RFC3339)
	key.Tags["revoked_reason"] = reason

	// Update key
	if err := m.UpdateKey(ctx, key); err != nil {
		return err
	}

	// Audit log
	if m.auditor != nil {
		m.auditor.LogAction(ctx, "revoke", keyID, true, map[string]interface{}{
			"reason": reason,
		})
	}

	return nil
}

// GetKeyByID retrieves a key by its ID
func (m *Manager) GetKeyByID(ctx context.Context, keyID string) (*APIKey, error) {
	// This would need to search through all keys to find by ID
	// In production, maintain an index of keyID -> location
	
	// For now, return error
	return nil, fmt.Errorf("GetKeyByID not fully implemented")
}

// ListAllKeys lists all keys across all accounts
func (m *Manager) ListAllKeys(ctx context.Context) ([]*KeyMetadata, error) {
	// This would iterate through all accounts
	// For now, return empty list
	return []*KeyMetadata{}, nil
}

// GetStats returns key management statistics
func (m *Manager) GetStats(ctx context.Context) (*KeyStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &KeyStats{
		KeysByExchange: make(map[string]int),
		KeysByMarket:   make(map[string]int),
		HealthStatus:   "healthy",
	}

	// Get all keys metadata
	allKeys, err := m.ListAllKeys(ctx)
	if err != nil {
		return nil, err
	}

	// Calculate statistics
	for _, key := range allKeys {
		stats.TotalKeys++
		
		if key.IsActive {
			stats.ActiveKeys++
		}
		
		if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now()) {
			stats.ExpiredKeys++
		}

		stats.KeysByExchange[key.Exchange]++
		stats.KeysByMarket[key.Market]++
	}

	// Check Vault health
	if !m.vaultClient.IsHealthy() {
		stats.HealthStatus = "unhealthy"
	}

	return stats, nil
}

// Utility methods

func (m *Manager) validateKey(key *APIKey) error {
	if key.AccountName == "" {
		return fmt.Errorf("account name required")
	}
	if key.Exchange == "" {
		return fmt.Errorf("exchange required")
	}
	if key.Market == "" {
		return fmt.Errorf("market required")
	}
	if key.APIKey == "" || key.APISecret == "" {
		return fmt.Errorf("API credentials required")
	}
	return nil
}

func (m *Manager) validateKeyStatus(key *APIKey) error {
	if !key.IsActive {
		return fmt.Errorf(ErrKeyInactive)
	}
	if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now()) {
		return fmt.Errorf(ErrKeyExpired)
	}
	return nil
}

func (m *Manager) encryptSensitiveData(key *APIKey) error {
	// Encrypt API secret
	encrypted, err := m.encrypt(key.APISecret)
	if err != nil {
		return err
	}
	key.APISecret = encrypted

	// Encrypt passphrase if present
	if key.Passphrase != "" {
		encrypted, err := m.encrypt(key.Passphrase)
		if err != nil {
			return err
		}
		key.Passphrase = encrypted
	}

	return nil
}

func (m *Manager) decryptSensitiveData(key *APIKey) error {
	// Decrypt API secret
	decrypted, err := m.decrypt(key.APISecret)
	if err != nil {
		return err
	}
	key.APISecret = decrypted

	// Decrypt passphrase if present
	if key.Passphrase != "" {
		decrypted, err := m.decrypt(key.Passphrase)
		if err != nil {
			return err
		}
		key.Passphrase = decrypted
	}

	return nil
}

func (m *Manager) encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(m.encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (m *Manager) decrypt(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(m.encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func (m *Manager) buildCacheKey(accountName, exchange, market string) string {
	return fmt.Sprintf("%s:%s:%s", accountName, exchange, market)
}

func (m *Manager) getFromCache(key string) *APIKey {
	if m.cache == nil {
		return nil
	}

	m.cache.mu.RLock()
	defer m.cache.mu.RUnlock()

	cached, exists := m.cache.keys[key]
	if !exists {
		return nil
	}

	if time.Now().After(cached.ExpiresAt) {
		return nil
	}

	return cached.Key
}

func (m *Manager) addToCache(key string, apiKey *APIKey) {
	if m.cache == nil {
		return
	}

	m.cache.mu.Lock()
	defer m.cache.mu.Unlock()

	m.cache.keys[key] = &CachedKey{
		Key:       apiKey,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(m.cache.ttl),
	}
}

func (m *Manager) removeFromCache(key string) {
	if m.cache == nil {
		return
	}

	m.cache.mu.Lock()
	defer m.cache.mu.Unlock()

	delete(m.cache.keys, key)
}

func (m *Manager) clearAccountCache(accountName string) {
	if m.cache == nil {
		return
	}

	m.cache.mu.Lock()
	defer m.cache.mu.Unlock()

	for key := range m.cache.keys {
		if len(key) > len(accountName) && key[:len(accountName)] == accountName {
			delete(m.cache.keys, key)
		}
	}
}

func (m *Manager) updateLastUsed(ctx context.Context, key *APIKey) {
	now := time.Now()
	updates := map[string]interface{}{
		"last_used_at": now,
	}

	m.vaultClient.UpdateKeyMetadata(ctx, key.AccountName, key.Exchange, key.Market, updates)
}

func (m *Manager) healthCheckLoop() {
	ticker := time.NewTicker(m.config.HealthCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		if !m.vaultClient.IsHealthy() {
			fmt.Println("Warning: Vault connection unhealthy")
			// In production, send alerts
		}
	}
}

// Close closes the key manager
func (m *Manager) Close() error {
	if m.rotator != nil {
		m.rotator.Stop()
	}
	if m.auditor != nil {
		m.auditor.Close()
	}
	if m.vaultClient != nil {
		return m.vaultClient.Close()
	}
	return nil
}

// GetRotator returns the key rotator
func (m *Manager) GetRotator() *KeyRotator {
	return m.rotator
}

// GetUsageTracker returns the usage tracker
func (m *Manager) GetUsageTracker() *UsageTracker {
	return m.usageTracker
}

// GetAuditor returns the auditor
func (m *Manager) GetAuditor() *Auditor {
	return m.auditor
}