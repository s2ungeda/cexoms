#!/bin/bash

# Setup script for HashiCorp Vault
# This script initializes Vault with the required structure for multi-account API key management

set -e

VAULT_ADDR=${VAULT_ADDR:-"http://localhost:8200"}
VAULT_TOKEN=${VAULT_TOKEN:-"dev-token"}

echo "Setting up Vault at $VAULT_ADDR"

# Wait for Vault to be ready
until vault status >/dev/null 2>&1; do
    echo "Waiting for Vault to be ready..."
    sleep 2
done

# Login to Vault
export VAULT_ADDR
export VAULT_TOKEN

echo "Logged into Vault"

# Enable KV v2 secrets engine if not already enabled
if ! vault secrets list | grep -q "^secret/"; then
    vault secrets enable -path=secret kv-v2
    echo "Enabled KV v2 secrets engine at path 'secret/'"
else
    echo "KV v2 secrets engine already enabled at path 'secret/'"
fi

# Create the exchanges path structure
echo "Creating exchanges path structure..."

# Function to create account structure
create_account_structure() {
    local exchange=$1
    local market=$2
    local account=$3
    local path="secret/exchanges/${exchange}_${market}_${account}"
    
    echo "Creating structure for $path"
    
    # Create initial empty data to establish the path
    vault kv put "$path" \
        account_id="$account" \
        exchange="$exchange" \
        market="$market" \
        api_key="" \
        secret_key="" \
        created_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        version=0
}

# Binance accounts
create_account_structure "binance" "spot" "main"
create_account_structure "binance" "spot" "sub_spot_arb"
create_account_structure "binance" "spot" "sub_market_making"
create_account_structure "binance" "futures" "sub_futures_trend"

# Bybit accounts (for future expansion)
create_account_structure "bybit" "spot" "main"
create_account_structure "bybit" "spot" "sub1"
create_account_structure "bybit" "futures" "sub1"

# OKX accounts (for future expansion)
create_account_structure "okx" "spot" "main"
create_account_structure "okx" "spot" "sub1"
create_account_structure "okx" "futures" "sub1"

# Upbit accounts (for future expansion)
create_account_structure "upbit" "spot" "main"
create_account_structure "upbit" "spot" "sub1"

# Create archive paths for key rotation
echo "Creating archive structure..."
vault kv put secret/exchanges/archive/readme \
    description="This path stores archived API keys after rotation" \
    created_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# Create audit path
echo "Setting up audit logging..."
if ! vault audit list | grep -q "file"; then
    vault audit enable file file_path=/data/audit/vault/audit.log
    echo "Enabled file audit logging"
else
    echo "File audit logging already enabled"
fi

# Create policies for different account types
echo "Creating Vault policies..."

# Main account policy - full access
cat <<EOF | vault policy write main-account-policy -
path "secret/data/exchanges/*_main" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "secret/data/exchanges/archive/*" {
  capabilities = ["create", "read", "list"]
}

path "secret/metadata/exchanges/*" {
  capabilities = ["list"]
}
EOF

# Sub-account policy - limited access
cat <<EOF | vault policy write sub-account-policy -
path "secret/data/exchanges/*_sub*" {
  capabilities = ["read"]
}

path "secret/metadata/exchanges/*_sub*" {
  capabilities = ["list"]
}
EOF

# Strategy-specific policies
cat <<EOF | vault policy write arbitrage-policy -
path "secret/data/exchanges/*_arb" {
  capabilities = ["read"]
}
EOF

cat <<EOF | vault policy write market-making-policy -
path "secret/data/exchanges/*_market_making" {
  capabilities = ["read"]
}
EOF

# Create test data (development only)
if [ "$ENV" = "development" ]; then
    echo "Creating test API keys (DEVELOPMENT ONLY)..."
    
    vault kv put secret/exchanges/binance_spot_main \
        account_id="main" \
        exchange="binance" \
        market="spot" \
        api_key="test_binance_main_api_key" \
        secret_key="test_binance_main_secret_key" \
        permissions='["read","trade","transfer"]' \
        created_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        updated_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        expires_at="$(date -u -d '+30 days' +%Y-%m-%dT%H:%M:%SZ)" \
        version=1
    
    vault kv put secret/exchanges/binance_spot_sub_spot_arb \
        account_id="sub_spot_arb" \
        exchange="binance" \
        market="spot" \
        api_key="test_binance_sub_arb_api_key" \
        secret_key="test_binance_sub_arb_secret_key" \
        permissions='["read","trade"]' \
        created_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        updated_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        expires_at="$(date -u -d '+30 days' +%Y-%m-%dT%H:%M:%SZ)" \
        version=1
fi

echo "Vault setup complete!"
echo ""
echo "Summary:"
echo "- KV v2 secrets engine enabled at 'secret/'"
echo "- Exchange account structures created"
echo "- Policies created for different account types"
echo "- Audit logging enabled"
if [ "$ENV" = "development" ]; then
    echo "- Test API keys created (DEVELOPMENT ONLY)"
fi

echo ""
echo "Next steps:"
echo "1. Store actual API keys using: vault kv put secret/exchanges/{exchange}_{market}_{account} api_key=... secret_key=..."
echo "2. Configure your application with VAULT_ADDR and VAULT_TOKEN"
echo "3. Test key retrieval: vault kv get secret/exchanges/binance_spot_main"