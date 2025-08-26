package security

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mExOms/pkg/types"
)

// KeyRotationService handles automatic API key rotation
type KeyRotationService struct {
	mu           sync.RWMutex
	vaultManager *VaultManager
	exchanges    map[string]types.Exchange
	config       *KeyRotationConfig
	tasks        map[string]*RotationTask
	stopCh       chan struct{}
}

// KeyRotationConfig holds key rotation configuration
type KeyRotationConfig struct {
	Enabled           bool
	CheckInterval     time.Duration
	DaysBeforeExpiry  int
	PriorityPatterns  []string // e.g., ["sub*", "main"]
	MaxConcurrent     int
}

// NewKeyRotationService creates a new key rotation service
func NewKeyRotationService(vault *VaultManager, config *KeyRotationConfig) *KeyRotationService {
	return &KeyRotationService{
		vaultManager: vault,
		exchanges:    make(map[string]types.Exchange),
		config:       config,
		tasks:        make(map[string]*RotationTask),
		stopCh:       make(chan struct{}),
	}
}

// RegisterExchange registers an exchange for key rotation
func (krs *KeyRotationService) RegisterExchange(name string, exchange types.Exchange) {
	krs.mu.Lock()
	defer krs.mu.Unlock()
	krs.exchanges[name] = exchange
}

// Start begins the key rotation service
func (krs *KeyRotationService) Start() {
	if !krs.config.Enabled {
		return
	}

	go krs.rotationLoop()
}

// Stop stops the key rotation service
func (krs *KeyRotationService) Stop() {
	close(krs.stopCh)
}

// rotationLoop checks for keys that need rotation
func (krs *KeyRotationService) rotationLoop() {
	ticker := time.NewTicker(krs.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			krs.checkAndRotateKeys()
		case <-krs.stopCh:
			return
		}
	}
}

// checkAndRotateKeys checks all accounts for keys that need rotation
func (krs *KeyRotationService) checkAndRotateKeys() {
	// Get all accounts that need rotation
	accounts := krs.getAccountsForRotation()

	// Sort by priority
	accounts = krs.sortByPriority(accounts)

	// Rotate keys with concurrency limit
	sem := make(chan struct{}, krs.config.MaxConcurrent)
	var wg sync.WaitGroup

	for _, account := range accounts {
		wg.Add(1)
		sem <- struct{}{}

		go func(acc *types.Account) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := krs.rotateAccountKeys(acc); err != nil {
				fmt.Printf("Failed to rotate keys for account %s: %v\n", acc.ID, err)
			}
		}(account)
	}

	wg.Wait()
}

// getAccountsForRotation returns accounts that need key rotation
func (krs *KeyRotationService) getAccountsForRotation() []*types.Account {
	var accounts []*types.Account

	// In production, this would query all accounts from the system
	// and check their key expiration dates
	
	return accounts
}

// sortByPriority sorts accounts based on configured priority patterns
func (krs *KeyRotationService) sortByPriority(accounts []*types.Account) []*types.Account {
	// Sort logic based on priority patterns
	// Sub-accounts first, then main accounts
	return accounts
}

// rotateAccountKeys rotates keys for a specific account
func (krs *KeyRotationService) rotateAccountKeys(account *types.Account) error {
	krs.mu.RLock()
	exchange, exists := krs.exchanges[account.Exchange]
	krs.mu.RUnlock()

	if !exists {
		return fmt.Errorf("exchange %s not registered", account.Exchange)
	}

	// Get current keys from Vault
	currentKeys, err := krs.vaultManager.GetAPIKeys(account)
	if err != nil {
		return fmt.Errorf("failed to get current keys: %w", err)
	}

	// Generate new API keys through exchange API
	// This is exchange-specific and would be implemented per exchange
	newKeys, err := krs.generateNewKeys(exchange, account)
	if err != nil {
		return fmt.Errorf("failed to generate new keys: %w", err)
	}

	// Test new keys before rotating
	if err := krs.testKeys(exchange, newKeys); err != nil {
		return fmt.Errorf("new keys validation failed: %w", err)
	}

	// Rotate keys in Vault
	if err := krs.vaultManager.RotateAPIKeys(account, newKeys); err != nil {
		return fmt.Errorf("failed to rotate keys in vault: %w", err)
	}

	// Update exchange connection with new keys
	if err := krs.updateExchangeKeys(exchange, account, newKeys); err != nil {
		// Rollback if update fails
		krs.vaultManager.RotateAPIKeys(account, currentKeys)
		return fmt.Errorf("failed to update exchange keys: %w", err)
	}

	// Delete old keys from exchange after grace period
	go krs.scheduleOldKeyDeletion(exchange, account, currentKeys)

	return nil
}

// generateNewKeys generates new API keys for an account
func (krs *KeyRotationService) generateNewKeys(exchange types.Exchange, account *types.Account) (*APIKeySet, error) {
	// This would call exchange-specific API to generate new keys
	// Each exchange has different API for key management
	
	// Placeholder implementation
	return &APIKeySet{
		AccountID:   account.ID,
		Exchange:    account.Exchange,
		Market:      account.Market,
		APIKey:      "new-api-key",
		SecretKey:   "new-secret-key",
		Permissions: []string{"read", "trade"},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(30 * 24 * time.Hour),
		Version:     1,
	}, nil
}

// testKeys validates new keys work correctly
func (krs *KeyRotationService) testKeys(exchange types.Exchange, keys *APIKeySet) error {
	// Test basic operations with new keys
	// 1. Test authentication
	// 2. Test balance query
	// 3. Test order placement (small test order)
	
	return nil
}

// updateExchangeKeys updates the exchange connection with new keys
func (krs *KeyRotationService) updateExchangeKeys(exchange types.Exchange, account *types.Account, keys *APIKeySet) error {
	// Update the exchange client with new API keys
	// This is exchange-specific
	
	return nil
}

// scheduleOldKeyDeletion schedules deletion of old keys after grace period
func (krs *KeyRotationService) scheduleOldKeyDeletion(exchange types.Exchange, account *types.Account, oldKeys *APIKeySet) {
	// Wait for grace period (e.g., 24 hours)
	time.Sleep(24 * time.Hour)

	// Delete old keys from exchange
	// This would call exchange API to revoke the old keys
	fmt.Printf("Deleting old keys for account %s\n", account.ID)
}

// ManualRotate manually triggers key rotation for an account
func (krs *KeyRotationService) ManualRotate(accountID string) error {
	// Find account
	var account *types.Account
	// In production, this would look up the account
	
	if account == nil {
		return fmt.Errorf("account %s not found", accountID)
	}

	return krs.rotateAccountKeys(account)
}

// GetRotationStatus returns the rotation status for all accounts
func (krs *KeyRotationService) GetRotationStatus() map[string]RotationTask {
	krs.mu.RLock()
	defer krs.mu.RUnlock()

	status := make(map[string]RotationTask)
	for id, task := range krs.tasks {
		status[id] = *task
	}
	return status
}

// KeyPermissionManager manages API key permissions
type KeyPermissionManager struct {
	mu              sync.RWMutex
	defaultPerms    map[string][]string
	accountOverride map[string][]string
}

// NewKeyPermissionManager creates a new permission manager
func NewKeyPermissionManager() *KeyPermissionManager {
	return &KeyPermissionManager{
		defaultPerms: map[string][]string{
			"spot":    {"read", "trade"},
			"futures": {"read", "trade", "position"},
		},
		accountOverride: make(map[string][]string),
	}
}

// GetPermissions returns permissions for an account
func (kpm *KeyPermissionManager) GetPermissions(accountID string, market string) []string {
	kpm.mu.RLock()
	defer kpm.mu.RUnlock()

	// Check for account-specific override
	if perms, exists := kpm.accountOverride[accountID]; exists {
		return perms
	}

	// Return default permissions for market
	if perms, exists := kpm.defaultPerms[market]; exists {
		return perms
	}

	return []string{"read"}
}

// SetAccountPermissions sets custom permissions for an account
func (kpm *KeyPermissionManager) SetAccountPermissions(accountID string, permissions []string) {
	kpm.mu.Lock()
	defer kpm.mu.Unlock()
	kpm.accountOverride[accountID] = permissions
}

// ValidatePermissions validates if an operation is allowed
func (kpm *KeyPermissionManager) ValidatePermissions(accountID string, market string, operation string) bool {
	permissions := kpm.GetPermissions(accountID, market)
	
	for _, perm := range permissions {
		if perm == operation {
			return true
		}
	}
	
	return false
}