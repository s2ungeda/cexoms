#!/bin/bash

# Script to rotate exchange API keys
set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Vault configuration
export VAULT_ADDR='http://localhost:8200'
export VAULT_TOKEN='root-token'

echo -e "${YELLOW}=== API Key Rotation ===${NC}"
echo "This script helps you rotate API keys that are older than 30 days"
echo ""

# Function to check key age
check_key_age() {
    local path=$1
    local data=$(vault kv get -format=json secret/exchanges/$path 2>/dev/null)
    
    if [ $? -eq 0 ]; then
        local created_at=$(echo $data | jq -r '.data.data.created_at // .data.data.updated_at // "unknown"')
        local api_key=$(echo $data | jq -r '.data.data.api_key' | sed 's/\(.\{8\}\).*/\1.../')
        
        if [ "$created_at" != "unknown" ]; then
            # Calculate age in days
            local key_date=$(date -d "$created_at" +%s 2>/dev/null || echo "0")
            local current_date=$(date +%s)
            local age_days=$(( (current_date - key_date) / 86400 ))
            
            echo -e "${path}: API Key ${api_key} (Age: ${age_days} days)"
            
            if [ $age_days -gt 30 ]; then
                echo -e "${RED}  ⚠️  This key should be rotated (>30 days old)${NC}"
                return 0
            else
                echo -e "${GREEN}  ✓ Key is still fresh${NC}"
                return 1
            fi
        else
            echo -e "${path}: Unable to determine key age"
            return 1
        fi
    fi
    
    return 1
}

# Check all exchange keys
echo -e "${YELLOW}Checking API key ages...${NC}"
echo ""

keys_to_rotate=()

# Get list of all keys
keys=$(vault kv list -format=json secret/exchanges 2>/dev/null | jq -r '.[]' || echo "")

if [ -z "$keys" ]; then
    echo -e "${RED}No API keys found in Vault${NC}"
    exit 1
fi

for key in $keys; do
    if check_key_age "$key"; then
        keys_to_rotate+=("$key")
    fi
    echo ""
done

# If keys need rotation
if [ ${#keys_to_rotate[@]} -eq 0 ]; then
    echo -e "${GREEN}All API keys are up to date!${NC}"
    exit 0
fi

echo -e "\n${YELLOW}The following keys need rotation:${NC}"
for key in "${keys_to_rotate[@]}"; do
    echo "  - $key"
done

echo ""
read -p "Do you want to rotate these keys now? (y/n): " -n 1 -r
echo

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Key rotation cancelled"
    exit 0
fi

# Rotate each key
for key in "${keys_to_rotate[@]}"; do
    echo -e "\n${YELLOW}Rotating $key${NC}"
    echo "Please generate new API keys from the exchange and enter them below:"
    
    case $key in
        binance_spot|binance_futures)
            read -p "Enter new API Key: " api_key
            read -s -p "Enter new Secret Key: " secret_key
            echo
            vault kv put secret/exchanges/$key \
                api_key="${api_key}" \
                secret_key="${secret_key}" \
                updated_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
                rotation_required="false" \
                previous_rotation="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
            ;;
        okx_unified)
            read -p "Enter new API Key: " api_key
            read -s -p "Enter new Secret Key: " secret_key
            echo
            read -p "Enter new Passphrase: " passphrase
            vault kv put secret/exchanges/$key \
                api_key="${api_key}" \
                secret_key="${secret_key}" \
                passphrase="${passphrase}" \
                updated_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
                rotation_required="false" \
                previous_rotation="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
            ;;
        *)
            read -p "Enter new API Key: " api_key
            read -s -p "Enter new Secret Key: " secret_key
            echo
            vault kv put secret/exchanges/$key \
                api_key="${api_key}" \
                secret_key="${secret_key}" \
                updated_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
                rotation_required="false" \
                previous_rotation="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
            ;;
    esac
    
    echo -e "${GREEN}✓ $key rotated successfully${NC}"
done

echo -e "\n${GREEN}=== Key Rotation Complete ===${NC}"
echo "Don't forget to:"
echo "1. Delete the old API keys from the exchange"
echo "2. Update any external systems using the old keys"
echo "3. Test the new keys with a small trade"