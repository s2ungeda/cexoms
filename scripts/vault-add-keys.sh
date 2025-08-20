#!/bin/bash

# Script to add or update exchange API keys in Vault
set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Vault configuration
export VAULT_ADDR='http://localhost:8200'
export VAULT_TOKEN='root-token'

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
        updated_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        rotation_required="false"
    
    echo -e "${GREEN}✓ ${exchange} ${market} API keys stored successfully${NC}"
}

# Main menu
echo -e "${YELLOW}=== Exchange API Key Management ===${NC}"
echo "1. Add/Update Binance Spot API keys"
echo "2. Add/Update Binance Futures API keys"
echo "3. Add/Update Bybit API keys"
echo "4. Add/Update OKX API keys"
echo "5. Add/Update Upbit API keys"
echo "6. View existing keys (without secrets)"
echo "7. Exit"

read -p "Select option (1-7): " option

case $option in
    1)
        echo -e "\n${YELLOW}Binance Spot API Keys${NC}"
        read -p "Enter API Key: " api_key
        read -s -p "Enter Secret Key: " secret_key
        echo
        store_api_keys "binance" "spot" "$api_key" "$secret_key"
        ;;
    2)
        echo -e "\n${YELLOW}Binance Futures API Keys${NC}"
        read -p "Enter API Key: " api_key
        read -s -p "Enter Secret Key: " secret_key
        echo
        store_api_keys "binance" "futures" "$api_key" "$secret_key"
        ;;
    3)
        echo -e "\n${YELLOW}Bybit API Keys${NC}"
        read -p "Enter API Key: " api_key
        read -s -p "Enter Secret Key: " secret_key
        echo
        store_api_keys "bybit" "unified" "$api_key" "$secret_key"
        ;;
    4)
        echo -e "\n${YELLOW}OKX API Keys${NC}"
        read -p "Enter API Key: " api_key
        read -s -p "Enter Secret Key: " secret_key
        echo
        read -p "Enter Passphrase: " passphrase
        vault kv put secret/exchanges/okx_unified \
            api_key="${api_key}" \
            secret_key="${secret_key}" \
            passphrase="${passphrase}" \
            updated_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
        echo -e "${GREEN}✓ OKX API keys stored successfully${NC}"
        ;;
    5)
        echo -e "\n${YELLOW}Upbit API Keys${NC}"
        read -p "Enter Access Key: " api_key
        read -s -p "Enter Secret Key: " secret_key
        echo
        store_api_keys "upbit" "spot" "$api_key" "$secret_key"
        ;;
    6)
        echo -e "\n${YELLOW}Existing API Keys in Vault:${NC}"
        vault kv list secret/exchanges/ 2>/dev/null || echo "No keys found"
        ;;
    7)
        echo "Exiting..."
        exit 0
        ;;
    *)
        echo -e "${RED}Invalid option${NC}"
        exit 1
        ;;
esac

echo -e "\n${GREEN}Done!${NC}"