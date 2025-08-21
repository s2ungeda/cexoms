#!/bin/bash

# Script to store Binance API keys in Vault
# Usage: ./store-binance-keys.sh

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# Vault configuration
export VAULT_ADDR='http://localhost:8200'
export VAULT_TOKEN='root-token'

echo -e "${YELLOW}=== Binance API Key Storage ===${NC}"
echo -e "${RED}WARNING: Make sure to use API keys with appropriate permissions!${NC}"
echo -e "Recommended permissions:"
echo -e "- Spot: Enable Spot Trading, Read Information"
echo -e "- Futures: Enable Futures Trading, Read Information"
echo -e ""

# Function to store keys
store_keys() {
    local market=$1
    local api_key=$2
    local secret_key=$3
    
    echo -e "${YELLOW}Storing Binance ${market} API keys...${NC}"
    
    # Use vault CLI directly
    vault kv put secret/exchanges/binance_${market} \
        api_key="${api_key}" \
        secret_key="${secret_key}" \
        created_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        rotation_required="false"
    
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Binance ${market} API keys stored successfully${NC}"
    else
        echo -e "${RED}✗ Failed to store Binance ${market} API keys${NC}"
        return 1
    fi
}

# Check if vault CLI is available
if ! command -v vault &> /dev/null; then
    echo -e "${YELLOW}Vault CLI not found. Using HTTP API instead...${NC}"
    
    # Function to store via HTTP API
    store_keys_http() {
        local market=$1
        local api_key=$2
        local secret_key=$3
        
        curl -s -X POST \
            -H "X-Vault-Token: ${VAULT_TOKEN}" \
            -H "Content-Type: application/json" \
            -d "{
                \"data\": {
                    \"api_key\": \"${api_key}\",
                    \"secret_key\": \"${secret_key}\",
                    \"created_at\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",
                    \"rotation_required\": false
                }
            }" \
            ${VAULT_ADDR}/v1/secret/data/exchanges/binance_${market}
        
        echo -e "\n${GREEN}✓ Binance ${market} API keys stored via HTTP API${NC}"
    }
    
    # Redefine function
    store_keys() {
        store_keys_http "$@"
    }
fi

# Interactive prompts
echo -e "\n${YELLOW}1. Binance Spot API Keys${NC}"
read -p "Do you want to store Binance Spot API keys? (y/n): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    read -p "Enter Binance Spot API Key: " spot_api_key
    read -s -p "Enter Binance Spot Secret Key: " spot_secret_key
    echo
    store_keys "spot" "$spot_api_key" "$spot_secret_key"
fi

echo -e "\n${YELLOW}2. Binance Futures API Keys${NC}"
read -p "Do you want to store Binance Futures API keys? (y/n): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    read -p "Enter Binance Futures API Key: " futures_api_key
    read -s -p "Enter Binance Futures Secret Key: " futures_secret_key
    echo
    store_keys "futures" "$futures_api_key" "$futures_secret_key"
fi

# Verify stored keys
echo -e "\n${YELLOW}Verifying stored keys...${NC}"
if command -v vault &> /dev/null; then
    echo -e "\nStored keys in Vault:"
    vault kv list secret/exchanges/ 2>/dev/null || echo "No keys found"
else
    echo -e "\nChecking via HTTP API:"
    curl -s -H "X-Vault-Token: ${VAULT_TOKEN}" \
        ${VAULT_ADDR}/v1/secret/metadata/exchanges/ | jq -r '.data.keys[]' 2>/dev/null || echo "No keys found"
fi

echo -e "\n${GREEN}Done!${NC}"
echo -e "\nNext steps:"
echo -e "1. Test the connection with: ${YELLOW}go run cmd/test-binance/main.go${NC}"
echo -e "2. Or use the vault-cli tool: ${YELLOW}./bin/vault-cli${NC}"