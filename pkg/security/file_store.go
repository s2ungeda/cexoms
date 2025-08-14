package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileSecretStore provides encrypted file-based secret storage (alternative to Vault)
type FileSecretStore struct {
	filePath  string
	encryptor *Encryptor
	data      map[string]*SecretData
	mu        sync.RWMutex
}

type SecretData struct {
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

type FileStore struct {
	Secrets map[string]*SecretData `json:"secrets"`
	Version int                    `json:"version"`
}

// NewFileSecretStore creates a new file-based secret store
func NewFileSecretStore(filePath, encryptionKey string) (*FileSecretStore, error) {
	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}
	
	store := &FileSecretStore{
		filePath:  filePath,
		encryptor: NewEncryptor(encryptionKey),
		data:      make(map[string]*SecretData),
	}
	
	// Load existing secrets
	if err := store.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load secrets: %w", err)
	}
	
	// Start cleanup routine
	go store.cleanupExpired()
	
	return store, nil
}

// GetExchangeCredentials retrieves API credentials for an exchange
func (fs *FileSecretStore) GetExchangeCredentials(exchange, market string) (*ExchangeCredentials, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	
	apiKeyPath := fmt.Sprintf("exchanges/%s_%s/api_key", exchange, market)
	apiSecretPath := fmt.Sprintf("exchanges/%s_%s/api_secret", exchange, market)
	
	apiKeyData, exists := fs.data[apiKeyPath]
	if !exists {
		return nil, fmt.Errorf("API key not found for %s_%s", exchange, market)
	}
	
	apiSecretData, exists := fs.data[apiSecretPath]
	if !exists {
		return nil, fmt.Errorf("API secret not found for %s_%s", exchange, market)
	}
	
	// Check expiration
	if !apiKeyData.ExpiresAt.IsZero() && time.Now().After(apiKeyData.ExpiresAt) {
		return nil, fmt.Errorf("credentials expired for %s_%s", exchange, market)
	}
	
	// Decrypt values
	apiKey, err := fs.encryptor.DecryptString(apiKeyData.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt API key: %w", err)
	}
	
	apiSecret, err := fs.encryptor.DecryptString(apiSecretData.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt API secret: %w", err)
	}
	
	return &ExchangeCredentials{
		APIKey:    apiKey,
		APISecret: apiSecret,
	}, nil
}

// StoreExchangeCredentials stores API credentials for an exchange
func (fs *FileSecretStore) StoreExchangeCredentials(exchange, market string, creds *ExchangeCredentials) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	
	// Encrypt credentials
	encryptedKey, err := fs.encryptor.EncryptString(creds.APIKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt API key: %w", err)
	}
	
	encryptedSecret, err := fs.encryptor.EncryptString(creds.APISecret)
	if err != nil {
		return fmt.Errorf("failed to encrypt API secret: %w", err)
	}
	
	now := time.Now()
	expiry := now.Add(30 * 24 * time.Hour) // 30 days
	
	fs.data[fmt.Sprintf("exchanges/%s_%s/api_key", exchange, market)] = &SecretData{
		Value:     encryptedKey,
		UpdatedAt: now,
		ExpiresAt: expiry,
	}
	
	fs.data[fmt.Sprintf("exchanges/%s_%s/api_secret", exchange, market)] = &SecretData{
		Value:     encryptedSecret,
		UpdatedAt: now,
		ExpiresAt: expiry,
	}
	
	return fs.save()
}

// SetSecret stores a generic secret
func (fs *FileSecretStore) SetSecret(key, value string, ttl time.Duration) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	
	encrypted, err := fs.encryptor.EncryptString(value)
	if err != nil {
		return fmt.Errorf("failed to encrypt value: %w", err)
	}
	
	data := &SecretData{
		Value:     encrypted,
		UpdatedAt: time.Now(),
	}
	
	if ttl > 0 {
		data.ExpiresAt = time.Now().Add(ttl)
	}
	
	fs.data[key] = data
	return fs.save()
}

// GetSecret retrieves a generic secret
func (fs *FileSecretStore) GetSecret(key string) (string, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	
	data, exists := fs.data[key]
	if !exists {
		return "", fmt.Errorf("secret not found: %s", key)
	}
	
	// Check expiration
	if !data.ExpiresAt.IsZero() && time.Now().After(data.ExpiresAt) {
		return "", fmt.Errorf("secret expired: %s", key)
	}
	
	return fs.encryptor.DecryptString(data.Value)
}

// DeleteSecret removes a secret
func (fs *FileSecretStore) DeleteSecret(key string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	
	delete(fs.data, key)
	return fs.save()
}

// ListKeys returns all secret keys
func (fs *FileSecretStore) ListKeys() []string {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	
	keys := make([]string, 0, len(fs.data))
	for k := range fs.data {
		keys = append(keys, k)
	}
	return keys
}

// load reads and decrypts the secrets file
func (fs *FileSecretStore) load() error {
	data, err := os.ReadFile(fs.filePath)
	if err != nil {
		return err
	}
	
	var store FileStore
	if err := json.Unmarshal(data, &store); err != nil {
		return fmt.Errorf("failed to unmarshal secrets: %w", err)
	}
	
	fs.data = store.Secrets
	return nil
}

// save encrypts and writes the secrets file
func (fs *FileSecretStore) save() error {
	store := FileStore{
		Secrets: fs.data,
		Version: 1,
	}
	
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal secrets: %w", err)
	}
	
	// Write to temporary file first
	tempFile := fs.filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write secrets: %w", err)
	}
	
	// Atomic rename
	return os.Rename(tempFile, fs.filePath)
}

// cleanupExpired removes expired secrets periodically
func (fs *FileSecretStore) cleanupExpired() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	
	for range ticker.C {
		fs.mu.Lock()
		now := time.Now()
		changed := false
		
		for key, data := range fs.data {
			if !data.ExpiresAt.IsZero() && now.After(data.ExpiresAt) {
				delete(fs.data, key)
				changed = true
			}
		}
		
		if changed {
			fs.save()
		}
		fs.mu.Unlock()
	}
}

// Close saves any pending changes
func (fs *FileSecretStore) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.save()
}