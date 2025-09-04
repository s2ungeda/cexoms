#!/bin/bash

echo "=== Binance API Key Storage ==="
echo "Enter your Binance API credentials:"
echo

read -p "API Key: " API_KEY
read -s -p "Secret Key: " SECRET_KEY
echo

VAULT_TOKEN=$(cat ~/.mExOms/vault-token)
VAULT_ADDR="http://localhost:8200"

# Store the keys
curl -X POST $VAULT_ADDR/v1/secret/data/exchanges/binance_spot \
  -H "X-Vault-Token: $VAULT_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"data\": {
      \"api_key\": \"$API_KEY\",
      \"secret_key\": \"$SECRET_KEY\"
    }
  }"

echo
echo "Keys stored successfully!"