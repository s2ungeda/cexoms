package vault

import (
	"fmt"
	"log"
	"os"

	vault "github.com/hashicorp/vault/api"
)

// Client wraps the Vault API client
type Client struct {
	client *vault.Client
}

// Config holds Vault configuration
type Config struct {
	Address string
	Token   string
}

// NewClient creates a new Vault client
func NewClient(config Config) (*Client, error) {
	// Default config
	if config.Address == "" {
		config.Address = os.Getenv("VAULT_ADDR")
		if config.Address == "" {
			config.Address = "http://localhost:8200"
		}
	}
	if config.Token == "" {
		config.Token = os.Getenv("VAULT_TOKEN")
		if config.Token == "" {
			config.Token = "root-token"
		}
	}

	// Create Vault client
	vaultConfig := vault.DefaultConfig()
	vaultConfig.Address = config.Address

	client, err := vault.NewClient(vaultConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	// Set token
	client.SetToken(config.Token)

	// Test connection
	health, err := client.Sys().Health()
	if err != nil {
		return nil, fmt.Errorf("vault is not healthy: %w", err)
	}

	if health.Sealed {
		return nil, fmt.Errorf("vault is sealed")
	}

	log.Printf("Connected to Vault at %s", config.Address)

	return &Client{client: client}, nil
}

// StoreExchangeKeys stores API keys for an exchange
func (c *Client) StoreExchangeKeys(exchange, market string, apiKey, secretKey string, extras map[string]interface{}) error {
	path := fmt.Sprintf("secret/data/exchanges/%s_%s", exchange, market)
	
	data := map[string]interface{}{
		"data": map[string]interface{}{
			"api_key":    apiKey,
			"secret_key": secretKey,
			"exchange":   exchange,
			"market":     market,
		},
	}

	// Add any extra fields (like passphrase for OKX)
	if extras != nil {
		for k, v := range extras {
			data["data"].(map[string]interface{})[k] = v
		}
	}

	_, err := c.client.Logical().Write(path, data)
	if err != nil {
		return fmt.Errorf("failed to store keys: %w", err)
	}

	log.Printf("Stored API keys for %s %s", exchange, market)
	return nil
}

// GetExchangeKeys retrieves API keys for an exchange
func (c *Client) GetExchangeKeys(exchange, market string) (map[string]string, error) {
	path := fmt.Sprintf("secret/data/exchanges/%s_%s", exchange, market)
	
	secret, err := c.client.Logical().Read(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read keys: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("no keys found for %s %s", exchange, market)
	}

	// Extract data field
	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid secret format")
	}

	// Convert to string map
	result := make(map[string]string)
	for k, v := range data {
		if str, ok := v.(string); ok {
			result[k] = str
		}
	}

	return result, nil
}

// ListExchangeKeys lists all stored exchange keys
func (c *Client) ListExchangeKeys() ([]string, error) {
	path := "secret/metadata/exchanges"
	
	secret, err := c.client.Logical().List(path)
	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return []string{}, nil
	}

	keysInterface, ok := secret.Data["keys"].([]interface{})
	if !ok {
		return []string{}, nil
	}

	keys := make([]string, len(keysInterface))
	for i, k := range keysInterface {
		keys[i] = k.(string)
	}

	return keys, nil
}

// DeleteExchangeKeys deletes API keys for an exchange
func (c *Client) DeleteExchangeKeys(exchange, market string) error {
	path := fmt.Sprintf("secret/metadata/exchanges/%s_%s", exchange, market)
	
	_, err := c.client.Logical().Delete(path)
	if err != nil {
		return fmt.Errorf("failed to delete keys: %w", err)
	}

	log.Printf("Deleted API keys for %s %s", exchange, market)
	return nil
}

// EnableKV2 enables the KV v2 secret engine
func (c *Client) EnableKV2() error {
	// Check if already enabled
	mounts, err := c.client.Sys().ListMounts()
	if err != nil {
		return fmt.Errorf("failed to list mounts: %w", err)
	}

	if _, ok := mounts["secret/"]; ok {
		log.Println("KV v2 secret engine already enabled")
		return nil
	}

	// Enable KV v2
	err = c.client.Sys().Mount("secret", &vault.MountInput{
		Type: "kv-v2",
	})
	if err != nil {
		return fmt.Errorf("failed to enable KV v2: %w", err)
	}

	log.Println("Enabled KV v2 secret engine")
	return nil
}