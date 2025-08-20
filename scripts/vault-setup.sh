#!/bin/bash

# Vault setup script for Binance API keys
set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Vault configuration
export VAULT_ADDR='http://localhost:8200'
export VAULT_TOKEN='root-token'

echo -e "${YELLOW}Setting up Vault for Binance API keys...${NC}"

# Wait for Vault to be ready
echo "Waiting for Vault to be ready..."
until curl -s ${VAULT_ADDR}/v1/sys/health > /dev/null; do
    echo "Waiting for Vault..."
    sleep 2
done

echo -e "${GREEN}Vault is ready!${NC}"

# Enable KV v2 secret engine if not already enabled
echo "Enabling KV v2 secret engine..."
vault secrets enable -path=secret kv-v2 2>/dev/null || echo "Secret engine already enabled"

# Create directory structure for exchange API keys
echo "Creating exchange API key structure..."

# Function to store API keys
store_api_keys() {
    local exchange=$1
    local market=$2
    local api_key=$3
    local secret_key=$4
    
    echo -e "${YELLOW}Storing ${exchange} ${market} API keys...${NC}"
    
    vault kv put secret/exchanges/${exchange}_${market} \
        api_key="${api_key}" \
        secret_key="${secret_key}" \
        created_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        rotation_required="false"
    
    echo -e "${GREEN}✓ ${exchange} ${market} API keys stored successfully${NC}"
}

# Interactive mode to add API keys
echo -e "\n${YELLOW}=== Binance API Key Setup ===${NC}"
echo "You can add your Binance API keys now or run this script later."
echo "Note: API keys should have appropriate permissions (Read, Trade)"
echo ""

read -p "Do you want to add Binance Spot API keys now? (y/n): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    read -p "Enter Binance Spot API Key: " spot_api_key
    read -s -p "Enter Binance Spot Secret Key: " spot_secret_key
    echo
    store_api_keys "binance" "spot" "$spot_api_key" "$spot_secret_key"
fi

echo ""
read -p "Do you want to add Binance Futures API keys now? (y/n): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    read -p "Enter Binance Futures API Key: " futures_api_key
    read -s -p "Enter Binance Futures Secret Key: " futures_secret_key
    echo
    store_api_keys "binance" "futures" "$futures_api_key" "$futures_secret_key"
fi

# Create policies for API key access
echo -e "\n${YELLOW}Creating Vault policies...${NC}"

# Create read-only policy for OMS services
cat > /tmp/oms-reader-policy.hcl <<EOF
path "secret/data/exchanges/*" {
  capabilities = ["read", "list"]
}
EOF

vault policy write oms-reader /tmp/oms-reader-policy.hcl
echo -e "${GREEN}✓ OMS reader policy created${NC}"

# Create admin policy for key rotation
cat > /tmp/oms-admin-policy.hcl <<EOF
path "secret/data/exchanges/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "secret/metadata/exchanges/*" {
  capabilities = ["list", "read", "delete"]
}
EOF

vault policy write oms-admin /tmp/oms-admin-policy.hcl
echo -e "${GREEN}✓ OMS admin policy created${NC}"

# Clean up
rm -f /tmp/oms-reader-policy.hcl /tmp/oms-admin-policy.hcl

echo -e "\n${GREEN}=== Vault Setup Complete ===${NC}"
echo -e "Vault Address: ${VAULT_ADDR}"
echo -e "Vault Token: ${VAULT_TOKEN}"
echo -e "\nTo manually add/update API keys later, use:"
echo -e "  ./scripts/vault-add-keys.sh"
echo -e "\nTo rotate API keys, use:"
echo -e "  ./scripts/vault-rotate-keys.sh"