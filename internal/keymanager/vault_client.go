package keymanager

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"time"

	vault "github.com/hashicorp/vault/api"
)

// VaultClient wraps the HashiCorp Vault client
type VaultClient struct {
	client      *vault.Client
	config      VaultConfig
	mountPath   string
	isConnected bool
}

// NewVaultClient creates a new Vault client
func NewVaultClient(config VaultConfig) (*VaultClient, error) {
	vaultConfig := vault.DefaultConfig()
	vaultConfig.Address = config.Address
	vaultConfig.Timeout = config.Timeout
	vaultConfig.MaxRetries = config.MaxRetries

	// Configure TLS if provided
	if config.TLSConfig != nil {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: config.TLSConfig.Insecure,
		}

		// Load CA cert if provided
		if config.TLSConfig.CACert != "" {
			// In production, load actual certificate
			// For now, we'll skip this
		}

		vaultConfig.HttpClient.Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
	}

	client, err := vault.NewClient(vaultConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	vc := &VaultClient{
		client:    client,
		config:    config,
		mountPath: config.MountPath,
	}

	// Authenticate
	if err := vc.authenticate(); err != nil {
		return nil, fmt.Errorf("failed to authenticate: %w", err)
	}

	// Test connection
	if err := vc.testConnection(); err != nil {
		return nil, fmt.Errorf("vault connection test failed: %w", err)
	}

	vc.isConnected = true
	return vc, nil
}

// authenticate handles different authentication methods
func (vc *VaultClient) authenticate() error {
	// Token authentication
	if vc.config.Token != "" {
		vc.client.SetToken(vc.config.Token)
		return nil
	}

	// AppRole authentication
	if vc.config.RoleID != "" && vc.config.SecretID != "" {
		data := map[string]interface{}{
			"role_id":   vc.config.RoleID,
			"secret_id": vc.config.SecretID,
		}

		resp, err := vc.client.Logical().Write("auth/approle/login", data)
		if err != nil {
			return fmt.Errorf("approle login failed: %w", err)
		}

		if resp.Auth == nil {
			return fmt.Errorf("no auth info returned")
		}

		vc.client.SetToken(resp.Auth.ClientToken)
		return nil
	}

	return fmt.Errorf("no authentication method configured")
}

// testConnection verifies the vault connection
func (vc *VaultClient) testConnection() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	health, err := vc.client.Sys().HealthWithContext(ctx)
	if err != nil {
		return err
	}

	if !health.Initialized {
		return fmt.Errorf("vault is not initialized")
	}

	if health.Sealed {
		return fmt.Errorf("vault is sealed")
	}

	return nil
}

// StoreKey stores an API key in Vault
func (vc *VaultClient) StoreKey(ctx context.Context, key *APIKey) error {
	if !vc.isConnected {
		return fmt.Errorf(ErrVaultUnavailable)
	}

	// Prepare data for storage
	data := map[string]interface{}{
		"api_key":     key.APIKey,
		"api_secret":  key.APISecret,
		"passphrase":  key.Passphrase,
		"permissions": key.Permissions,
		"is_active":   key.IsActive,
		"is_testnet":  key.IsTestnet,
		"created_at":  key.CreatedAt.Format(time.RFC3339),
		"updated_at":  key.UpdatedAt.Format(time.RFC3339),
		"metadata": map[string]interface{}{
			"id":           key.ID,
			"account_name": key.AccountName,
			"exchange":     key.Exchange,
			"market":       key.Market,
			"tags":         key.Tags,
		},
	}

	if key.LastUsedAt != nil {
		data["last_used_at"] = key.LastUsedAt.Format(time.RFC3339)
	}

	if key.ExpiresAt != nil {
		data["expires_at"] = key.ExpiresAt.Format(time.RFC3339)
	}

	if key.RotatedFrom != "" {
		data["rotated_from"] = key.RotatedFrom
	}

	// Store in Vault
	secretPath := vc.buildSecretPath(key.AccountName, key.Exchange, key.Market)
	_, err := vc.client.Logical().WriteWithContext(ctx, secretPath, data)
	if err != nil {
		return fmt.Errorf("failed to store key: %w", err)
	}

	return nil
}

// GetKey retrieves an API key from Vault
func (vc *VaultClient) GetKey(ctx context.Context, accountName, exchange, market string) (*APIKey, error) {
	if !vc.isConnected {
		return nil, fmt.Errorf(ErrVaultUnavailable)
	}

	secretPath := vc.buildSecretPath(accountName, exchange, market)
	secret, err := vc.client.Logical().ReadWithContext(ctx, secretPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read key: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf(ErrKeyNotFound)
	}

	// Handle both v1 and v2 KV secrets
	var data map[string]interface{}
	if val, ok := secret.Data["data"]; ok {
		// KV v2
		data = val.(map[string]interface{})
	} else {
		// KV v1
		data = secret.Data
	}

	return vc.parseKeyData(data)
}

// ListKeys lists all keys matching the criteria
func (vc *VaultClient) ListKeys(ctx context.Context, accountName string) ([]*KeyMetadata, error) {
	if !vc.isConnected {
		return nil, fmt.Errorf(ErrVaultUnavailable)
	}

	// List all secrets under the account path
	listPath := path.Join(vc.mountPath, "metadata", accountName)
	secret, err := vc.client.Logical().ListWithContext(ctx, listPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return []*KeyMetadata{}, nil
	}

	// Parse the list of keys
	keys, ok := secret.Data["keys"].([]interface{})
	if !ok {
		return []*KeyMetadata{}, nil
	}

	var metadata []*KeyMetadata
	for _, k := range keys {
		keyPath := k.(string)
		// Extract exchange and market from path
		// Format: exchange_market/
		parts := parseKeyPath(keyPath)
		if len(parts) >= 2 {
			meta := &KeyMetadata{
				AccountName: accountName,
				Exchange:    parts[0],
				Market:      parts[1],
			}
			metadata = append(metadata, meta)
		}
	}

	return metadata, nil
}

// DeleteKey deletes an API key from Vault
func (vc *VaultClient) DeleteKey(ctx context.Context, accountName, exchange, market string) error {
	if !vc.isConnected {
		return fmt.Errorf(ErrVaultUnavailable)
	}

	secretPath := vc.buildSecretPath(accountName, exchange, market)
	_, err := vc.client.Logical().DeleteWithContext(ctx, secretPath)
	if err != nil {
		return fmt.Errorf("failed to delete key: %w", err)
	}

	return nil
}

// BackupKeys creates a backup of all keys
func (vc *VaultClient) BackupKeys(ctx context.Context) (map[string]interface{}, error) {
	if !vc.isConnected {
		return nil, fmt.Errorf(ErrVaultUnavailable)
	}

	// This would implement a full backup of the KV store
	// For now, return a placeholder
	backup := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   "1.0",
		"keys":      []interface{}{},
	}

	return backup, nil
}

// RestoreKeys restores keys from a backup
func (vc *VaultClient) RestoreKeys(ctx context.Context, backup map[string]interface{}) error {
	if !vc.isConnected {
		return fmt.Errorf(ErrVaultUnavailable)
	}

	// This would implement restoration from backup
	// For now, return nil
	return nil
}

// UpdateKeyMetadata updates only the metadata of a key
func (vc *VaultClient) UpdateKeyMetadata(ctx context.Context, accountName, exchange, market string, updates map[string]interface{}) error {
	if !vc.isConnected {
		return fmt.Errorf(ErrVaultUnavailable)
	}

	// First, get the existing key
	key, err := vc.GetKey(ctx, accountName, exchange, market)
	if err != nil {
		return err
	}

	// Update metadata
	key.UpdatedAt = time.Now()
	if lastUsed, ok := updates["last_used_at"].(time.Time); ok {
		key.LastUsedAt = &lastUsed
	}
	if isActive, ok := updates["is_active"].(bool); ok {
		key.IsActive = isActive
	}
	if tags, ok := updates["tags"].(map[string]string); ok {
		key.Tags = tags
	}

	// Store updated key
	return vc.StoreKey(ctx, key)
}

// buildSecretPath constructs the full path for a secret
func (vc *VaultClient) buildSecretPath(accountName, exchange, market string) string {
	// Format: mount_path/data/account_name/exchange_market
	return path.Join(vc.mountPath, "data", accountName, fmt.Sprintf("%s_%s", exchange, market))
}

// parseKeyData parses key data from Vault response
func (vc *VaultClient) parseKeyData(data map[string]interface{}) (*APIKey, error) {
	key := &APIKey{}

	// Parse basic fields
	if v, ok := data["api_key"].(string); ok {
		key.APIKey = v
	}
	if v, ok := data["api_secret"].(string); ok {
		key.APISecret = v
	}
	if v, ok := data["passphrase"].(string); ok {
		key.Passphrase = v
	}
	if v, ok := data["is_active"].(bool); ok {
		key.IsActive = v
	}
	if v, ok := data["is_testnet"].(bool); ok {
		key.IsTestnet = v
	}

	// Parse metadata
	if metadata, ok := data["metadata"].(map[string]interface{}); ok {
		if v, ok := metadata["id"].(string); ok {
			key.ID = v
		}
		if v, ok := metadata["account_name"].(string); ok {
			key.AccountName = v
		}
		if v, ok := metadata["exchange"].(string); ok {
			key.Exchange = v
		}
		if v, ok := metadata["market"].(string); ok {
			key.Market = v
		}
		if v, ok := metadata["tags"].(map[string]interface{}); ok {
			key.Tags = make(map[string]string)
			for k, val := range v {
				if strVal, ok := val.(string); ok {
					key.Tags[k] = strVal
				}
			}
		}
	}

	// Parse permissions
	if perms, ok := data["permissions"].([]interface{}); ok {
		for _, p := range perms {
			if perm, ok := p.(string); ok {
				key.Permissions = append(key.Permissions, perm)
			}
		}
	}

	// Parse timestamps
	if v, ok := data["created_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			key.CreatedAt = t
		}
	}
	if v, ok := data["updated_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			key.UpdatedAt = t
		}
	}
	if v, ok := data["last_used_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			key.LastUsedAt = &t
		}
	}
	if v, ok := data["expires_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			key.ExpiresAt = &t
		}
	}

	if v, ok := data["rotated_from"].(string); ok {
		key.RotatedFrom = v
	}

	return key, nil
}

// parseKeyPath extracts exchange and market from key path
func parseKeyPath(keyPath string) []string {
	// Remove trailing slash if present
	keyPath = path.Clean(keyPath)
	
	// Expected format: exchange_market
	parts := splitExchangeMarket(keyPath)
	return parts
}

// splitExchangeMarket splits "exchange_market" into ["exchange", "market"]
func splitExchangeMarket(s string) []string {
	// Find the last underscore to separate exchange from market
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '_' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

// IsHealthy checks if the Vault connection is healthy
func (vc *VaultClient) IsHealthy() bool {
	if !vc.isConnected {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	health, err := vc.client.Sys().HealthWithContext(ctx)
	if err != nil {
		return false
	}

	return health.Initialized && !health.Sealed
}

// RenewToken renews the client token if needed
func (vc *VaultClient) RenewToken(ctx context.Context) error {
	token, err := vc.client.Auth().Token().RenewSelfWithContext(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to renew token: %w", err)
	}

	if token.Auth != nil {
		vc.client.SetToken(token.Auth.ClientToken)
	}

	return nil
}

// Close closes the Vault client connection
func (vc *VaultClient) Close() error {
	vc.isConnected = false
	// Vault client doesn't need explicit closing
	return nil
}