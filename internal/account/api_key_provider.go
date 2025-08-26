package account

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mExOms/pkg/security"
	"github.com/mExOms/pkg/types"
)

// APIKeyProvider manages API keys for accounts using Vault
type APIKeyProvider struct {
	mu              sync.RWMutex
	vaultManager    *security.VaultManager
	rotationService *security.KeyRotationService
	permManager     *security.KeyPermissionManager
	keyCache        map[string]*CachedKey
}

// CachedKey holds cached API key information
type CachedKey struct {
	APIKey     string
	SecretKey  string
	Passphrase string
	ExpiresAt  time.Time
	CachedAt   time.Time
}

// NewAPIKeyProvider creates a new API key provider
func NewAPIKeyProvider(vault *security.VaultManager) *APIKeyProvider {
	// Create rotation service
	rotationConfig := &security.KeyRotationConfig{
		Enabled:          true,
		CheckInterval:    24 * time.Hour,
		DaysBeforeExpiry: 7,
		PriorityPatterns: []string{"sub*", "main"},
		MaxConcurrent:    3,
	}
	rotationService := security.NewKeyRotationService(vault, rotationConfig)

	// Create permission manager
	permManager := security.NewKeyPermissionManager()

	return &APIKeyProvider{
		vaultManager:    vault,
		rotationService: rotationService,
		permManager:     permManager,
		keyCache:        make(map[string]*CachedKey),
	}
}

// GetAPIKeys retrieves API keys for an account
func (akp *APIKeyProvider) GetAPIKeys(ctx context.Context, account *types.Account) (*types.APICredentials, error) {
	// Check cache first
	cacheKey := akp.buildCacheKey(account)
	
	akp.mu.RLock()
	if cached, exists := akp.keyCache[cacheKey]; exists {
		if time.Now().Before(cached.ExpiresAt) {
			akp.mu.RUnlock()
			return &types.APICredentials{
				APIKey:     cached.APIKey,
				SecretKey:  cached.SecretKey,
				Passphrase: cached.Passphrase,
			}, nil
		}
	}
	akp.mu.RUnlock()

	// Fetch from Vault
	keySet, err := akp.vaultManager.GetAPIKeys(account)
	if err != nil {
		return nil, fmt.Errorf("failed to get API keys from vault: %w", err)
	}

	// Update usage
	akp.vaultManager.UpdateKeyUsage(account.ID)

	// Cache the keys
	akp.mu.Lock()
	akp.keyCache[cacheKey] = &CachedKey{
		APIKey:     keySet.APIKey,
		SecretKey:  keySet.SecretKey,
		Passphrase: keySet.Passphrase,
		ExpiresAt:  time.Now().Add(5 * time.Minute), // 5 minute cache
		CachedAt:   time.Now(),
	}
	akp.mu.Unlock()

	return &types.APICredentials{
		APIKey:     keySet.APIKey,
		SecretKey:  keySet.SecretKey,
		Passphrase: keySet.Passphrase,
	}, nil
}

// StoreAPIKeys stores new API keys for an account
func (akp *APIKeyProvider) StoreAPIKeys(ctx context.Context, account *types.Account, creds *types.APICredentials) error {
	keySet := &security.APIKeySet{
		AccountID:   account.ID,
		Exchange:    account.Exchange,
		Market:      string(account.Market),
		APIKey:      creds.APIKey,
		SecretKey:   creds.SecretKey,
		Passphrase:  creds.Passphrase,
		Permissions: akp.permManager.GetPermissions(account.ID, string(account.Market)),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(30 * 24 * time.Hour), // 30 days
		Version:     1,
	}

	if err := akp.vaultManager.StoreAPIKeys(account, keySet); err != nil {
		return fmt.Errorf("failed to store API keys: %w", err)
	}

	// Clear cache for this account
	akp.clearCache(account)

	return nil
}

// RotateAPIKeys manually triggers key rotation for an account
func (akp *APIKeyProvider) RotateAPIKeys(ctx context.Context, accountID string) error {
	return akp.rotationService.ManualRotate(accountID)
}

// GetBestAccount returns the best account for a given operation
func (akp *APIKeyProvider) GetBestAccount(exchange string, market types.MarketType, operation string) (string, error) {
	// Get account priority from Vault
	accounts := akp.vaultManager.GetKeyPriority(exchange, market)
	
	// Filter by permission
	for _, accountID := range accounts {
		if akp.permManager.ValidatePermissions(accountID, string(market), operation) {
			return accountID, nil
		}
	}

	return "", fmt.Errorf("no account found with permission for %s operation", operation)
}

// SetAccountPermissions sets custom permissions for an account
func (akp *APIKeyProvider) SetAccountPermissions(accountID string, permissions []string) {
	akp.permManager.SetAccountPermissions(accountID, permissions)
}

// ValidateOperation checks if an account can perform an operation
func (akp *APIKeyProvider) ValidateOperation(account *types.Account, operation string) bool {
	return akp.permManager.ValidatePermissions(account.ID, string(account.Market), operation)
}

// StartRotationService starts the automatic key rotation service
func (akp *APIKeyProvider) StartRotationService() {
	akp.rotationService.Start()
}

// StopRotationService stops the automatic key rotation service
func (akp *APIKeyProvider) StopRotationService() {
	akp.rotationService.Stop()
}

// GetRotationStatus returns the current rotation status
func (akp *APIKeyProvider) GetRotationStatus() map[string]security.RotationTask {
	return akp.rotationService.GetRotationStatus()
}

// Private methods

func (akp *APIKeyProvider) buildCacheKey(account *types.Account) string {
	return fmt.Sprintf("%s_%s_%s", account.Exchange, account.Market, account.ID)
}

func (akp *APIKeyProvider) clearCache(account *types.Account) {
	akp.mu.Lock()
	defer akp.mu.Unlock()
	
	cacheKey := akp.buildCacheKey(account)
	delete(akp.keyCache, cacheKey)
}

// ClearAllCache clears all cached keys
func (akp *APIKeyProvider) ClearAllCache() {
	akp.mu.Lock()
	defer akp.mu.Unlock()
	
	akp.keyCache = make(map[string]*CachedKey)
}

// GetCacheStats returns cache statistics
func (akp *APIKeyProvider) GetCacheStats() map[string]interface{} {
	akp.mu.RLock()
	defer akp.mu.RUnlock()

	stats := map[string]interface{}{
		"total_cached": len(akp.keyCache),
		"cache_entries": make([]map[string]interface{}, 0),
	}

	for key, cached := range akp.keyCache {
		entry := map[string]interface{}{
			"key":        key,
			"cached_at":  cached.CachedAt,
			"expires_at": cached.ExpiresAt,
			"ttl":        cached.ExpiresAt.Sub(time.Now()).Seconds(),
		}
		stats["cache_entries"] = append(stats["cache_entries"].([]map[string]interface{}), entry)
	}

	return stats
}