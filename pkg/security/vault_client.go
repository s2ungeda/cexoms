package security

import (
	"fmt"
	"os"
	"sync"
	"time"

	vault "github.com/hashicorp/vault/api"
)

type VaultClient struct {
	client    *vault.Client
	mountPath string
	cache     map[string]*CachedSecret
	mu        sync.RWMutex
}

type CachedSecret struct {
	Data      map[string]string
	ExpiresAt time.Time
}

type ExchangeCredentials struct {
	APIKey    string `json:"api_key"`
	APISecret string `json:"api_secret"`
}

func NewVaultClient(address, token, mountPath string) (*VaultClient, error) {
	config := vault.DefaultConfig()
	config.Address = address
	
	client, err := vault.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}
	
	client.SetToken(token)
	
	return &VaultClient{
		client:    client,
		mountPath: mountPath,
		cache:     make(map[string]*CachedSecret),
	}, nil
}

func NewVaultClientFromEnv() (*VaultClient, error) {
	address := os.Getenv("VAULT_ADDR")
	if address == "" {
		address = "http://localhost:8200"
	}
	
	token := os.Getenv("VAULT_TOKEN")
	if token == "" {
		token = "root-token" // Development default
	}
	
	mountPath := os.Getenv("VAULT_MOUNT_PATH")
	if mountPath == "" {
		mountPath = "secret"
	}
	
	return NewVaultClient(address, token, mountPath)
}

// GetExchangeCredentials retrieves API credentials for an exchange
func (vc *VaultClient) GetExchangeCredentials(exchange, market string) (*ExchangeCredentials, error) {
	path := fmt.Sprintf("exchanges/%s_%s", exchange, market)
	
	// Check cache first
	vc.mu.RLock()
	if cached, exists := vc.cache[path]; exists && time.Now().Before(cached.ExpiresAt) {
		vc.mu.RUnlock()
		return &ExchangeCredentials{
			APIKey:    cached.Data["api_key"],
			APISecret: cached.Data["api_secret"],
		}, nil
	}
	vc.mu.RUnlock()
	
	// Fetch from Vault
	secret, err := vc.client.Logical().Read(fmt.Sprintf("%s/data/%s", vc.mountPath, path))
	if err != nil {
		return nil, fmt.Errorf("failed to read secret: %w", err)
	}
	
	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("no credentials found for %s_%s", exchange, market)
	}
	
	// Extract data from v2 secret
	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid secret format")
	}
	
	creds := &ExchangeCredentials{}
	if apiKey, ok := data["api_key"].(string); ok {
		creds.APIKey = apiKey
	}
	if apiSecret, ok := data["api_secret"].(string); ok {
		creds.APISecret = apiSecret
	}
	
	// Cache the secret
	vc.mu.Lock()
	vc.cache[path] = &CachedSecret{
		Data: map[string]string{
			"api_key":    creds.APIKey,
			"api_secret": creds.APISecret,
		},
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	vc.mu.Unlock()
	
	return creds, nil
}

// StoreExchangeCredentials stores API credentials for an exchange
func (vc *VaultClient) StoreExchangeCredentials(exchange, market string, creds *ExchangeCredentials) error {
	path := fmt.Sprintf("exchanges/%s_%s", exchange, market)
	
	data := map[string]interface{}{
		"api_key":    creds.APIKey,
		"api_secret": creds.APISecret,
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	
	_, err := vc.client.Logical().Write(fmt.Sprintf("%s/data/%s", vc.mountPath, path), map[string]interface{}{
		"data": data,
	})
	
	if err != nil {
		return fmt.Errorf("failed to write secret: %w", err)
	}
	
	// Invalidate cache
	vc.mu.Lock()
	delete(vc.cache, path)
	vc.mu.Unlock()
	
	return nil
}

// RotateExchangeCredentials generates new credentials (placeholder for actual implementation)
func (vc *VaultClient) RotateExchangeCredentials(exchange, market string) error {
	// In a real implementation, this would:
	// 1. Generate new API credentials on the exchange
	// 2. Store the new credentials in Vault
	// 3. Keep old credentials for grace period
	// 4. Delete old credentials after confirmation
	
	// For now, just invalidate the cache
	path := fmt.Sprintf("exchanges/%s_%s", exchange, market)
	
	vc.mu.Lock()
	delete(vc.cache, path)
	vc.mu.Unlock()
	
	return nil
}

// ListExchanges returns all configured exchanges
func (vc *VaultClient) ListExchanges() ([]string, error) {
	path := "exchanges"
	
	secret, err := vc.client.Logical().List(fmt.Sprintf("%s/metadata/%s", vc.mountPath, path))
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}
	
	if secret == nil || secret.Data == nil {
		return []string{}, nil
	}
	
	keys, ok := secret.Data["keys"].([]interface{})
	if !ok {
		return []string{}, nil
	}
	
	exchanges := make([]string, 0, len(keys))
	for _, key := range keys {
		if str, ok := key.(string); ok {
			exchanges = append(exchanges, str)
		}
	}
	
	return exchanges, nil
}

// EnableAuditLog enables audit logging for the secrets path
func (vc *VaultClient) EnableAuditLog(logPath string) error {
	options := map[string]string{
		"file_path": logPath,
	}
	
	return vc.client.Sys().EnableAuditWithOptions("file", &vault.EnableAuditOptions{
		Type:    "file",
		Options: options,
	})
}

// RevokeToken revokes a token
func (vc *VaultClient) RevokeToken(token string) error {
	return vc.client.Auth().Token().RevokeSelf(token)
}

// Close cleans up resources
func (vc *VaultClient) Close() error {
	vc.mu.Lock()
	vc.cache = make(map[string]*CachedSecret)
	vc.mu.Unlock()
	return nil
}