#!/bin/bash

# Setup script for HashiCorp Vault for mExOms key management

echo "=== HashiCorp Vault Setup for mExOms ==="

# Check if Vault is installed
if ! command -v vault &> /dev/null; then
    echo "Vault is not installed. Please install HashiCorp Vault first."
    echo "Visit: https://www.vaultproject.io/downloads"
    exit 1
fi

# Start Vault in dev mode (for development only)
echo "Starting Vault in development mode..."
vault server -dev -dev-root-token-id="dev-token" &
VAULT_PID=$!

# Wait for Vault to start
sleep 2

# Set environment variables
export VAULT_ADDR='http://127.0.0.1:8200'
export VAULT_TOKEN='dev-token'

echo "Vault started with PID: $VAULT_PID"
echo "Vault Address: $VAULT_ADDR"
echo "Vault Token: $VAULT_TOKEN"

# Enable KV v2 secrets engine
echo "Enabling KV v2 secrets engine..."
vault secrets enable -path=secret kv-v2

# Create policies for different access levels
echo "Creating access policies..."

# Admin policy - full access
cat <<EOF | vault policy write admin-policy -
path "secret/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "sys/policies/*" {
  capabilities = ["read", "list"]
}

path "auth/approle/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}
EOF

# Read-only policy
cat <<EOF | vault policy write readonly-policy -
path "secret/data/*" {
  capabilities = ["read", "list"]
}
EOF

# Application policy - for the OMS application
cat <<EOF | vault policy write oms-app-policy -
path "secret/data/+/+/*" {
  capabilities = ["read", "list"]
}

path "secret/metadata/*" {
  capabilities = ["list"]
}
EOF

echo "Policies created successfully"

# Create AppRole for application authentication
echo "Setting up AppRole authentication..."
vault auth enable approle

# Create role for OMS application
vault write auth/approle/role/oms-app \
    token_policies="oms-app-policy" \
    token_ttl=24h \
    token_max_ttl=168h

# Get role ID and secret ID
ROLE_ID=$(vault read -field=role_id auth/approle/role/oms-app/role-id)
SECRET_ID=$(vault write -field=secret_id -f auth/approle/role/oms-app/secret-id)

echo "AppRole created:"
echo "  Role ID: $ROLE_ID"
echo "  Secret ID: $SECRET_ID"

# Create example structure and keys
echo "Creating example key structure..."

# Example for Binance main account
vault kv put secret/binance_main/binance_spot \
    api_key="demo_binance_main_spot_key" \
    api_secret="demo_binance_main_spot_secret" \
    permissions='["read","trade"]' \
    is_active=true \
    is_testnet=true \
    created_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    metadata='{"id":"binance_main_spot_001","account_name":"binance_main","exchange":"binance","market":"spot"}'

vault kv put secret/binance_main/binance_futures \
    api_key="demo_binance_main_futures_key" \
    api_secret="demo_binance_main_futures_secret" \
    permissions='["read","trade","futures"]' \
    is_active=true \
    is_testnet=true \
    created_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    metadata='{"id":"binance_main_futures_001","account_name":"binance_main","exchange":"binance","market":"futures"}'

# Example for Binance algo account
vault kv put secret/binance_algo/binance_spot \
    api_key="demo_binance_algo_spot_key" \
    api_secret="demo_binance_algo_spot_secret" \
    permissions='["read","trade"]' \
    is_active=true \
    is_testnet=true \
    created_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    metadata='{"id":"binance_algo_spot_001","account_name":"binance_algo","exchange":"binance","market":"spot"}'

echo "Example keys created"

# Create transit engine for encryption (optional)
echo "Setting up transit engine for additional encryption..."
vault secrets enable transit
vault write -f transit/keys/oms-encryption

echo ""
echo "=== Vault Setup Complete ==="
echo ""
echo "To use Vault in your application:"
echo "  export VAULT_ADDR='http://127.0.0.1:8200'"
echo "  export VAULT_TOKEN='dev-token'"
echo ""
echo "Or use AppRole authentication:"
echo "  Role ID: $ROLE_ID"
echo "  Secret ID: $SECRET_ID"
echo ""
echo "To stop Vault: kill $VAULT_PID"
echo ""
echo "WARNING: This is a development setup. For production:"
echo "  - Use proper TLS certificates"
echo "  - Enable audit logging"
echo "  - Use stronger authentication methods"
echo "  - Run Vault in HA mode"
echo "  - Use auto-unseal"
echo ""