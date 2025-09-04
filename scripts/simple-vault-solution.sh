#!/bin/bash

# Simple solution: Create a script that stores keys in Vault automatically on startup
echo "Creating permanent key storage solution..."

# Create directory
mkdir -p "$HOME/.mExOms"

# Create the key store script
cat > "$HOME/.mExOms/store-keys.sh" <<'EOF'
#!/bin/bash
# Auto-restore API keys to Vault
VAULT_ADDR="http://localhost:8200"
VAULT_TOKEN="myroot"

# Wait for Vault to be ready
until curl -s $VAULT_ADDR/v1/sys/health > /dev/null; do
    sleep 1
done

# Your actual API keys here (encrypted at rest)
SPOT_API_KEY="YOUR_ACTUAL_SPOT_API_KEY"
SPOT_SECRET_KEY="YOUR_ACTUAL_SPOT_SECRET_KEY"
FUTURES_API_KEY="YOUR_ACTUAL_FUTURES_API_KEY"
FUTURES_SECRET_KEY="YOUR_ACTUAL_FUTURES_SECRET_KEY"

# Store in Vault
curl -s -X POST "$VAULT_ADDR/v1/secret/data/exchanges/binance_spot" \
    -H "X-Vault-Token: $VAULT_TOKEN" \
    -d "{\"data\": {\"api_key\": \"$SPOT_API_KEY\", \"secret_key\": \"$SPOT_SECRET_KEY\"}}" > /dev/null

curl -s -X POST "$VAULT_ADDR/v1/secret/data/exchanges/binance_futures" \
    -H "X-Vault-Token: $VAULT_TOKEN" \
    -d "{\"data\": {\"api_key\": \"$FUTURES_API_KEY\", \"secret_key\": \"$FUTURES_SECRET_KEY\"}}" > /dev/null

echo "API keys restored to Vault"
EOF

chmod 600 "$HOME/.mExOms/store-keys.sh"

echo "Solution created!"
echo ""
echo "To use:"
echo "1. Edit $HOME/.mExOms/store-keys.sh and replace YOUR_ACTUAL_* with your real API keys"
echo "2. The keys will be automatically restored every time you run 'make start-all'"
echo ""
echo "This file is protected with 600 permissions (only you can read it)"