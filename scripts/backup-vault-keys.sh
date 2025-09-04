#!/bin/bash

# Backup API keys from Vault
VAULT_ADDR="http://localhost:8200"
VAULT_KEYS_FILE="$HOME/.mExOms/vault-keys.json"
BACKUP_FILE="$HOME/.mExOms/api-keys-backup.json"

if [ ! -f "$VAULT_KEYS_FILE" ]; then
    echo "No Vault keys found"
    exit 1
fi

ROOT_TOKEN=$(jq -r '.root_token' "$VAULT_KEYS_FILE")

echo "Backing up API keys..."

# Get Spot keys
SPOT_DATA=$(curl -s -H "X-Vault-Token: $ROOT_TOKEN" \
    $VAULT_ADDR/v1/secret/data/exchanges/binance_spot | jq -r '.data.data')

# Get Futures keys
FUTURES_DATA=$(curl -s -H "X-Vault-Token: $ROOT_TOKEN" \
    $VAULT_ADDR/v1/secret/data/exchanges/binance_futures | jq -r '.data.data')

# Create backup
cat > "$BACKUP_FILE" <<EOF
{
  "spot": $SPOT_DATA,
  "futures": $FUTURES_DATA,
  "backup_date": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

chmod 600 "$BACKUP_FILE"
echo "API keys backed up to $BACKUP_FILE"