#!/bin/bash

# Restore Vault keys and unseal if needed
VAULT_KEYS_FILE="$HOME/.mExOms/vault-keys.json"
VAULT_ADDR="http://localhost:8200"

if [ ! -f "$VAULT_KEYS_FILE" ]; then
    echo "No saved Vault keys found"
    exit 0
fi

# Check if Vault is sealed
SEALED=$(curl -s $VAULT_ADDR/v1/sys/health | jq -r '.sealed' 2>/dev/null || echo "true")

if [ "$SEALED" = "true" ]; then
    echo "Unsealing Vault..."
    UNSEAL_KEY=$(jq -r '.keys_base64[0] // .keys[0]' "$VAULT_KEYS_FILE")
    
    curl -s -X PUT $VAULT_ADDR/v1/sys/unseal \
        -d "{\"key\": \"$UNSEAL_KEY\"}" > /dev/null
    
    echo "Vault unsealed"
fi

# Restore saved API keys if they exist
KEYS_BACKUP_FILE="$HOME/.mExOms/api-keys-backup.json"
if [ -f "$KEYS_BACKUP_FILE" ]; then
    ROOT_TOKEN=$(jq -r '.root_token' "$VAULT_KEYS_FILE")
    
    echo "Restoring API keys..."
    
    # Restore Spot keys
    SPOT_API_KEY=$(jq -r '.spot.api_key' "$KEYS_BACKUP_FILE" 2>/dev/null)
    SPOT_SECRET_KEY=$(jq -r '.spot.secret_key' "$KEYS_BACKUP_FILE" 2>/dev/null)
    
    if [ "$SPOT_API_KEY" != "null" ] && [ -n "$SPOT_API_KEY" ]; then
        curl -s -X POST $VAULT_ADDR/v1/secret/data/exchanges/binance_spot \
            -H "X-Vault-Token: $ROOT_TOKEN" \
            -d "{\"data\": {\"api_key\": \"$SPOT_API_KEY\", \"secret_key\": \"$SPOT_SECRET_KEY\"}}" > /dev/null
        echo "Spot keys restored"
    fi
    
    # Restore Futures keys
    FUTURES_API_KEY=$(jq -r '.futures.api_key' "$KEYS_BACKUP_FILE" 2>/dev/null)
    FUTURES_SECRET_KEY=$(jq -r '.futures.secret_key' "$KEYS_BACKUP_FILE" 2>/dev/null)
    
    if [ "$FUTURES_API_KEY" != "null" ] && [ -n "$FUTURES_API_KEY" ]; then
        curl -s -X POST $VAULT_ADDR/v1/secret/data/exchanges/binance_futures \
            -H "X-Vault-Token: $ROOT_TOKEN" \
            -d "{\"data\": {\"api_key\": \"$FUTURES_API_KEY\", \"secret_key\": \"$FUTURES_SECRET_KEY\"}}" > /dev/null
        echo "Futures keys restored"
    fi
fi