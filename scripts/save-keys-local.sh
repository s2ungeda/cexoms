#!/bin/bash

# Save API keys to local encrypted file
KEYS_FILE="$HOME/.mExOms/api-keys.enc"

echo "Enter Binance Spot API Key:"
read -s SPOT_API_KEY
echo "Enter Binance Spot Secret Key:"
read -s SPOT_SECRET_KEY
echo "Enter Binance Futures API Key:"
read -s FUTURES_API_KEY
echo "Enter Binance Futures Secret Key:"
read -s FUTURES_SECRET_KEY

# Create keys JSON
KEYS_JSON=$(cat <<EOF
{
  "spot": {
    "api_key": "$SPOT_API_KEY",
    "secret_key": "$SPOT_SECRET_KEY"
  },
  "futures": {
    "api_key": "$FUTURES_API_KEY",
    "secret_key": "$FUTURES_SECRET_KEY"
  }
}
EOF
)

# Encrypt and save
mkdir -p "$HOME/.mExOms"
echo "$KEYS_JSON" | openssl enc -aes-256-cbc -salt -pbkdf2 -out "$KEYS_FILE"
chmod 600 "$KEYS_FILE"

echo "Keys encrypted and saved to $KEYS_FILE"

# Create auto-restore script
cat > "$HOME/.mExOms/restore-keys.sh" <<'EOF'
#!/bin/bash
KEYS_FILE="$HOME/.mExOms/api-keys.enc"
VAULT_ADDR="http://localhost:8200"
VAULT_TOKEN="myroot"

if [ ! -f "$KEYS_FILE" ]; then
    echo "No saved keys found"
    exit 0
fi

# Decrypt keys
KEYS_JSON=$(openssl enc -aes-256-cbc -d -pbkdf2 -in "$KEYS_FILE")

# Extract keys
SPOT_API=$(echo "$KEYS_JSON" | python3 -c "import sys, json; print(json.load(sys.stdin)['spot']['api_key'])")
SPOT_SECRET=$(echo "$KEYS_JSON" | python3 -c "import sys, json; print(json.load(sys.stdin)['spot']['secret_key'])")
FUTURES_API=$(echo "$KEYS_JSON" | python3 -c "import sys, json; print(json.load(sys.stdin)['futures']['api_key'])")
FUTURES_SECRET=$(echo "$KEYS_JSON" | python3 -c "import sys, json; print(json.load(sys.stdin)['futures']['secret_key'])")

# Store in Vault
curl -s -X POST "$VAULT_ADDR/v1/secret/data/exchanges/binance_spot" \
    -H "X-Vault-Token: $VAULT_TOKEN" \
    -d "{\"data\": {\"api_key\": \"$SPOT_API\", \"secret_key\": \"$SPOT_SECRET\"}}" > /dev/null

curl -s -X POST "$VAULT_ADDR/v1/secret/data/exchanges/binance_futures" \
    -H "X-Vault-Token: $VAULT_TOKEN" \
    -d "{\"data\": {\"api_key\": \"$FUTURES_API\", \"secret_key\": \"$FUTURES_SECRET\"}}" > /dev/null

echo "Keys restored to Vault"
EOF

chmod +x "$HOME/.mExOms/restore-keys.sh"
echo "Auto-restore script created"