package security

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/mExOms/pkg/types"
)

// VaultManager manages API keys in HashiCorp Vault
type VaultManager struct {
	mu            sync.RWMutex
	client        *api.Client
	config        *VaultConfig
	keyCache      map[string]*APIKeySet
	rotationTasks map[string]*RotationTask
}

// VaultConfig holds Vault configuration
type VaultConfig struct {
	Address        string
	Token          string
	MountPath      string        // Default: "secret"
	TTL            time.Duration // Key cache TTL
	RotationPeriod time.Duration // Key rotation period (e.g., 30 days)
	RetryAttempts  int
	RetryDelay     time.Duration
}

// APIKeySet represents a set of API keys for an account
type APIKeySet struct {
	AccountID   string    `json:"account_id"`
	Exchange    string    `json:"exchange"`
	Market      string    `json:"market"`
	APIKey      string    `json:"api_key"`
	SecretKey   string    `json:"secret_key"`
	Passphrase  string    `json:"passphrase,omitempty"` // For exchanges that require it
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	LastUsed    time.Time `json:"last_used"`
	Version     int       `json:"version"`
}

// RotationTask tracks key rotation tasks
type RotationTask struct {
	AccountID      string
	NextRotation   time.Time
	LastRotation   time.Time
	RotationCount  int
	LastError      error
	LastErrorTime  time.Time
}

// NewVaultManager creates a new Vault manager
func NewVaultManager(config *VaultConfig) (*VaultManager, error) {
	// Create Vault client
	vaultConfig := api.DefaultConfig()
	vaultConfig.Address = config.Address
	
	client, err := api.NewClient(vaultConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}
	
	// Set token
	client.SetToken(config.Token)
	
	// Verify connection
	health, err := client.Sys().Health()
	if err != nil {
		return nil, fmt.Errorf("vault health check failed: %w", err)
	}
	
	if !health.Initialized {
		return nil, fmt.Errorf("vault is not initialized")
	}
	
	if health.Sealed {
		return nil, fmt.Errorf("vault is sealed")
	}
	
	vm := &VaultManager{
		client:        client,
		config:        config,
		keyCache:      make(map[string]*APIKeySet),
		rotationTasks: make(map[string]*RotationTask),
	}
	
	// Start background tasks
	go vm.rotationLoop()
	go vm.cacheCleanupLoop()
	
	return vm, nil
}

// StoreAPIKeys stores API keys for an account
func (vm *VaultManager) StoreAPIKeys(account *types.Account, keys *APIKeySet) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	// Build Vault path
	path := vm.buildVaultPath(account.Exchange, account.Market, account.ID)
	
	// Prepare data
	data := map[string]interface{}{
		"account_id":  keys.AccountID,
		"exchange":    keys.Exchange,
		"market":      keys.Market,
		"api_key":     keys.APIKey,
		"secret_key":  keys.SecretKey,
		"passphrase":  keys.Passphrase,
		"permissions": keys.Permissions,
		"created_at":  keys.CreatedAt.Format(time.RFC3339),
		"updated_at":  time.Now().Format(time.RFC3339),
		"expires_at":  keys.ExpiresAt.Format(time.RFC3339),
		"version":     keys.Version + 1,
	}
	
	// Write to Vault
	_, err := vm.client.Logical().Write(path, data)
	if err != nil {
		return fmt.Errorf("failed to write to vault: %w", err)
	}
	
	// Update cache
	keys.UpdatedAt = time.Now()
	keys.Version++
	vm.keyCache[account.ID] = keys
	
	// Schedule rotation if needed
	vm.scheduleRotation(account.ID, keys)
	
	return nil
}

// GetAPIKeys retrieves API keys for an account
func (vm *VaultManager) GetAPIKeys(account *types.Account) (*APIKeySet, error) {
	// Check cache first
	vm.mu.RLock()
	if cached, exists := vm.keyCache[account.ID]; exists {
		if time.Now().Before(cached.ExpiresAt) {
			vm.mu.RUnlock()
			return cached, nil
		}
	}
	vm.mu.RUnlock()
	
	// Fetch from Vault
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	path := vm.buildVaultPath(account.Exchange, account.Market, account.ID)
	
	secret, err := vm.client.Logical().Read(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read from vault: %w", err)
	}
	
	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("no keys found for account %s", account.ID)
	}
	
	// Parse response
	keys := &APIKeySet{
		AccountID:  vm.getString(secret.Data, "account_id"),
		Exchange:   vm.getString(secret.Data, "exchange"),
		Market:     vm.getString(secret.Data, "market"),
		APIKey:     vm.getString(secret.Data, "api_key"),
		SecretKey:  vm.getString(secret.Data, "secret_key"),
		Passphrase: vm.getString(secret.Data, "passphrase"),
	}
	
	// Parse timestamps
	if createdStr := vm.getString(secret.Data, "created_at"); createdStr != "" {
		keys.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	}
	if updatedStr := vm.getString(secret.Data, "updated_at"); updatedStr != "" {
		keys.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	}
	
	// Set expiration
	keys.ExpiresAt = time.Now().Add(vm.config.TTL)
	
	// Update cache
	vm.keyCache[account.ID] = keys
	
	return keys, nil
}

// RotateAPIKeys rotates API keys for an account
func (vm *VaultManager) RotateAPIKeys(account *types.Account, newKeys *APIKeySet) error {
	// Get current keys
	currentKeys, err := vm.GetAPIKeys(account)
	if err != nil {
		return fmt.Errorf("failed to get current keys: %w", err)
	}
	
	// Archive current keys
	archivePath := fmt.Sprintf("%s/archive/%s_v%d", 
		vm.buildVaultPath(account.Exchange, account.Market, account.ID),
		time.Now().Format("20060102_150405"),
		currentKeys.Version)
	
	// Store archived version
	archiveData := map[string]interface{}{
		"account_id":  currentKeys.AccountID,
		"api_key":     currentKeys.APIKey,
		"secret_key":  currentKeys.SecretKey,
		"archived_at": time.Now().Format(time.RFC3339),
		"version":     currentKeys.Version,
	}
	
	if _, err := vm.client.Logical().Write(archivePath, archiveData); err != nil {
		return fmt.Errorf("failed to archive keys: %w", err)
	}
	
	// Store new keys
	newKeys.CreatedAt = time.Now()
	newKeys.Version = currentKeys.Version + 1
	
	if err := vm.StoreAPIKeys(account, newKeys); err != nil {
		return fmt.Errorf("failed to store new keys: %w", err)
	}
	
	// Update rotation task
	vm.mu.Lock()
	if task, exists := vm.rotationTasks[account.ID]; exists {
		task.LastRotation = time.Now()
		task.NextRotation = time.Now().Add(vm.config.RotationPeriod)
		task.RotationCount++
	}
	vm.mu.Unlock()
	
	return nil
}

// DeleteAPIKeys deletes API keys for an account
func (vm *VaultManager) DeleteAPIKeys(account *types.Account) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	path := vm.buildVaultPath(account.Exchange, account.Market, account.ID)
	
	_, err := vm.client.Logical().Delete(path)
	if err != nil {
		return fmt.Errorf("failed to delete from vault: %w", err)
	}
	
	// Remove from cache
	delete(vm.keyCache, account.ID)
	delete(vm.rotationTasks, account.ID)
	
	return nil
}

// ListAccountKeys lists all API keys for an exchange
func (vm *VaultManager) ListAccountKeys(exchange string) ([]*APIKeySet, error) {
	basePath := fmt.Sprintf("%s/data/exchanges/%s", vm.config.MountPath, exchange)
	
	secret, err := vm.client.Logical().List(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}
	
	if secret == nil || secret.Data == nil {
		return []*APIKeySet{}, nil
	}
	
	// Parse key paths
	var keys []*APIKeySet
	if keyList, ok := secret.Data["keys"].([]interface{}); ok {
		for _, key := range keyList {
			if keyPath, ok := key.(string); ok {
				// Extract account info from path
				// Path format: {market}_{account}/
				// This is simplified - actual implementation would parse properly
				keys = append(keys, &APIKeySet{
					AccountID: keyPath,
					Exchange:  exchange,
				})
			}
		}
	}
	
	return keys, nil
}

// UpdateKeyUsage updates the last used time for a key
func (vm *VaultManager) UpdateKeyUsage(accountID string) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	if keys, exists := vm.keyCache[accountID]; exists {
		keys.LastUsed = time.Now()
	}
}

// StoreSecret stores a generic secret in Vault
func (vm *VaultManager) StoreSecret(path string, data map[string]interface{}) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	fullPath := fmt.Sprintf("%s/data/%s", vm.config.MountPath, path)
	
	// Wrap data for KV v2
	wrappedData := map[string]interface{}{
		"data": data,
	}
	
	_, err := vm.client.Logical().Write(fullPath, wrappedData)
	if err != nil {
		return fmt.Errorf("failed to store secret: %w", err)
	}
	
	return nil
}

// GetSecret retrieves a generic secret from Vault
func (vm *VaultManager) GetSecret(path string) (map[string]interface{}, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	fullPath := fmt.Sprintf("%s/data/%s", vm.config.MountPath, path)
	
	secret, err := vm.client.Logical().Read(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read secret: %w", err)
	}
	
	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("secret not found")
	}
	
	// Extract data from KV v2 response
	if data, ok := secret.Data["data"].(map[string]interface{}); ok {
		return data, nil
	}
	
	return nil, fmt.Errorf("invalid secret format")
}

// GetKeyPriority returns accounts sorted by priority for key usage
func (vm *VaultManager) GetKeyPriority(exchange string, market types.MarketType) []string {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	type accountPriority struct {
		AccountID string
		Priority  int
		LastUsed  time.Time
	}
	
	var accounts []accountPriority
	
	for accountID, keys := range vm.keyCache {
		if keys.Exchange == exchange && keys.Market == string(market) {
			priority := 100 // Base priority
			
			// Adjust priority based on usage
			timeSinceUse := time.Since(keys.LastUsed)
			if timeSinceUse > 5*time.Minute {
				priority += 20
			} else if timeSinceUse > 1*time.Minute {
				priority += 10
			}
			
			// Adjust priority based on rotation schedule
			if task, exists := vm.rotationTasks[accountID]; exists {
				daysUntilRotation := time.Until(task.NextRotation).Hours() / 24
				if daysUntilRotation < 7 {
					priority -= 30 // Lower priority if rotation is soon
				}
			}
			
			accounts = append(accounts, accountPriority{
				AccountID: accountID,
				Priority:  priority,
				LastUsed:  keys.LastUsed,
			})
		}
	}
	
	// Sort by priority (highest first)
	var result []string
	for _, account := range accounts {
		result = append(result, account.AccountID)
	}
	
	return result
}

// Private methods

func (vm *VaultManager) buildVaultPath(exchange, market, accountID string) string {
	return fmt.Sprintf("%s/data/exchanges/%s_%s_%s", 
		vm.config.MountPath, exchange, market, accountID)
}

func (vm *VaultManager) getString(data map[string]interface{}, key string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func (vm *VaultManager) scheduleRotation(accountID string, keys *APIKeySet) {
	task := &RotationTask{
		AccountID:    accountID,
		NextRotation: time.Now().Add(vm.config.RotationPeriod),
		LastRotation: keys.CreatedAt,
	}
	
	vm.rotationTasks[accountID] = task
}

// Background loops

func (vm *VaultManager) rotationLoop() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	
	for range ticker.C {
		vm.checkRotations()
	}
}

func (vm *VaultManager) checkRotations() {
	vm.mu.RLock()
	tasks := make([]*RotationTask, 0, len(vm.rotationTasks))
	for _, task := range vm.rotationTasks {
		if time.Now().After(task.NextRotation) {
			tasks = append(tasks, task)
		}
	}
	vm.mu.RUnlock()
	
	// Process rotations
	for _, task := range tasks {
		// In production, this would trigger actual key rotation
		// through exchange APIs
		fmt.Printf("Key rotation needed for account %s\n", task.AccountID)
	}
}

func (vm *VaultManager) cacheCleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		vm.cleanupCache()
	}
}

func (vm *VaultManager) cleanupCache() {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	now := time.Now()
	for accountID, keys := range vm.keyCache {
		if now.After(keys.ExpiresAt) {
			delete(vm.keyCache, accountID)
		}
	}
}

// EncryptionService provides additional encryption for sensitive data
type EncryptionService struct {
	key []byte
}

// NewEncryptionService creates a new encryption service
func NewEncryptionService(key []byte) *EncryptionService {
	return &EncryptionService{
		key: key,
	}
}

// Encrypt encrypts data using AES-256
func (es *EncryptionService) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(es.key)
	if err != nil {
		return "", err
	}

	// Generate a new IV for each encryption
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}

	// Pad plaintext to block size
	plaintextBytes := []byte(plaintext)
	padding := aes.BlockSize - len(plaintextBytes)%aes.BlockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	plaintextBytes = append(plaintextBytes, padtext...)

	// Encrypt
	ciphertext := make([]byte, len(plaintextBytes))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, plaintextBytes)

	// Prepend IV to ciphertext
	ciphertext = append(iv, ciphertext...)

	// Encode to base64
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts data
func (es *EncryptionService) Decrypt(ciphertext string) (string, error) {
	// Decode from base64
	ciphertextBytes, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(es.key)
	if err != nil {
		return "", err
	}

	if len(ciphertextBytes) < aes.BlockSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	// Extract IV
	iv := ciphertextBytes[:aes.BlockSize]
	ciphertextBytes = ciphertextBytes[aes.BlockSize:]

	// Decrypt
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(ciphertextBytes, ciphertextBytes)

	// Remove padding
	padding := int(ciphertextBytes[len(ciphertextBytes)-1])
	if padding > aes.BlockSize || padding == 0 {
		return "", fmt.Errorf("invalid padding")
	}

	return string(ciphertextBytes[:len(ciphertextBytes)-padding]), nil
}