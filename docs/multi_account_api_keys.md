# Multi-Account API Key Management

## Overview

The Multi-Account API Key Management system provides secure, centralized management of API keys across multiple trading accounts using HashiCorp Vault. This system supports automatic key rotation, permission management, and intelligent account selection for operations.

## Architecture

### Components

1. **Vault Manager** (`pkg/security/vault_manager.go`)
   - Interfaces with HashiCorp Vault
   - Stores and retrieves encrypted API keys
   - Manages key versioning and archival
   - Provides caching for performance

2. **Key Rotation Service** (`pkg/security/key_rotation.go`)
   - Automatic key rotation based on schedule
   - Priority-based rotation (sub-accounts first)
   - Key validation before rotation
   - Grace period for old key deletion

3. **API Key Provider** (`internal/account/api_key_provider.go`)
   - High-level interface for key operations
   - Account selection based on permissions
   - Cache management
   - Integration with account manager

4. **Encryption Service** (`pkg/security/vault_manager.go`)
   - AES-256 encryption for additional security
   - Unique IV for each encryption
   - Base64 encoding for storage

## Vault Structure

```
secret/
└── exchanges/
    ├── binance_spot_main         # Main account keys
    ├── binance_spot_sub_spot_arb # Arbitrage sub-account
    ├── binance_spot_sub_market_making # Market making sub-account
    ├── binance_futures_sub_futures_trend # Futures trading sub-account
    └── archive/                  # Archived keys after rotation
        └── binance_spot_main_v1_20240115_143022
```

## Key Storage Format

```json
{
  "account_id": "sub_spot_arb",
  "exchange": "binance",
  "market": "spot",
  "api_key": "encrypted_api_key",
  "secret_key": "encrypted_secret_key",
  "passphrase": "encrypted_passphrase",
  "permissions": ["read", "trade"],
  "created_at": "2024-01-15T14:30:22Z",
  "updated_at": "2024-01-15T14:30:22Z",
  "expires_at": "2024-02-14T14:30:22Z",
  "version": 1
}
```

## Usage

### 1. Initialize Vault

```bash
# Start Vault in dev mode (for testing)
vault server -dev -dev-root-token-id="dev-token"

# Run setup script
./scripts/setup_vault.sh
```

### 2. Configure Vault

```yaml
# configs/vault.yaml
vault:
  address: "http://localhost:8200"
  token: "${VAULT_TOKEN}"
  mount_path: "secret"
  ttl: 5m
  rotation_period: 720h  # 30 days
```

### 3. Store API Keys

```go
// Create account
account := &types.Account{
    ID:       "sub_spot_arb",
    Exchange: "binance",
    Market:   types.MarketTypeSpot,
    Type:     types.AccountTypeSub,
}

// Store API keys
creds := &types.APICredentials{
    APIKey:    "your_api_key",
    SecretKey: "your_secret_key",
}

err := accountManager.StoreAPIKeys("sub_spot_arb", creds)
```

### 4. Retrieve API Keys

```go
// Get API keys for account
keys, err := accountManager.GetAPIKeys("sub_spot_arb")
if err != nil {
    log.Fatal(err)
}

// Keys are automatically cached for 5 minutes
fmt.Printf("API Key: %s\n", keys.APIKey)
```

### 5. Account Selection by Permission

```go
// Find account that can perform transfers
account, err := accountManager.GetBestAccountForOperation(
    "binance", 
    types.MarketTypeSpot, 
    "transfer"
)

// Only main account will be returned as it has transfer permission
```

### 6. Permission Management

```go
// Set custom permissions for account
permissions := []string{"read", "trade", "transfer"}
err := accountManager.SetAccountPermissions("main", permissions)

// Validate operation
canTransfer := accountManager.ValidateAccountOperation("sub_spot_arb", "transfer")
// Returns false for sub-accounts
```

### 7. Manual Key Rotation

```go
// Trigger manual key rotation
err := accountManager.RotateAPIKeys("sub_spot_arb")

// Check rotation status
status := accountManager.GetKeyRotationStatus()
```

## Security Features

### 1. Encryption Layers
- **Vault Encryption**: All data encrypted at rest in Vault
- **Additional AES-256**: Optional second layer of encryption
- **Unique IVs**: Each encryption uses a unique initialization vector

### 2. Access Control
- **Vault Policies**: Different policies for account types
- **Permission Validation**: Operations checked against permissions
- **Audit Logging**: All key operations logged

### 3. Key Rotation
- **Automatic Rotation**: Keys rotated every 30 days
- **Priority System**: Sub-accounts rotated first
- **Validation**: New keys tested before old keys deleted
- **Grace Period**: 24-hour grace period before old key deletion

### 4. Rate Limiting
- **Account Priority**: Fresh accounts prioritized for API calls
- **Usage Tracking**: Last usage time tracked for rotation decisions
- **Smart Selection**: Accounts near rotation get lower priority

## Account Permissions

### Default Permissions

```yaml
spot:
  - read
  - trade

futures:
  - read
  - trade
  - position

main_account_override:
  - read
  - trade
  - transfer  # Only main accounts can transfer
```

### Custom Permissions

```go
// Market maker needs different permissions
kpm.SetAccountPermissions("sub_market_making", []string{
    "read",
    "trade", 
    "cancel_all",  // Bulk cancel for market making
})
```

## Key Priority Algorithm

The system uses a scoring algorithm to select the best API key:

1. **Base Score**: 100 points
2. **Rate Limit Availability**: +40% weight
3. **Time Since Last Use**: +20 points if > 5 minutes
4. **Days Until Rotation**: -30 points if < 7 days
5. **Account Health**: Based on recent errors

## Best Practices

### 1. Key Storage
- Never store keys in code or config files
- Use environment variables for Vault token
- Enable Vault audit logging in production

### 2. Rotation Schedule
- Set rotation period based on exchange requirements
- Monitor rotation failures
- Keep at least 2 API keys per critical account

### 3. Permission Management
- Follow principle of least privilege
- Sub-accounts should never have transfer permissions
- Review permissions regularly

### 4. Monitoring
- Track key usage patterns
- Alert on rotation failures
- Monitor cache hit rates

## Troubleshooting

### Common Issues

1. **Vault Connection Failed**
   ```bash
   export VAULT_ADDR="http://localhost:8200"
   export VAULT_TOKEN="your-token"
   vault status
   ```

2. **Key Not Found**
   - Check Vault path structure
   - Verify account exists
   - Check Vault policies

3. **Rotation Failed**
   - Check exchange API limits
   - Verify new key generation API
   - Review rotation logs

### Debug Commands

```bash
# List all keys
vault kv list secret/exchanges/

# Get specific key
vault kv get secret/exchanges/binance_spot_main

# Check policies
vault policy list
vault policy read main-account-policy
```

## Production Deployment

### 1. Vault Setup
```bash
# Use production Vault cluster
vault operator init
vault operator unseal

# Enable audit logging
vault audit enable file file_path=/vault/logs/audit.log
```

### 2. Environment Variables
```bash
export VAULT_ADDR="https://vault.production.com"
export VAULT_TOKEN="s.xxxxxxxxxxxxx"
export VAULT_NAMESPACE="trading"
export ENCRYPTION_KEY="base64_encoded_32_byte_key"
```

### 3. Security Hardening
- Enable TLS for Vault communication
- Use Vault namespaces for isolation
- Implement IP whitelisting
- Enable MFA for sensitive operations

### 4. Monitoring
- Set up alerts for key rotation failures
- Monitor Vault audit logs
- Track API key usage metrics
- Alert on unusual access patterns