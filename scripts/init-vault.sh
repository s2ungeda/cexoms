#!/bin/bash

# Initialize Vault with persistent storage
set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${YELLOW}=== Vault Initialization ===${NC}"

# Set Vault address
export VAULT_ADDR='http://localhost:8200'
VAULT_KEYS_FILE="$HOME/.mExOms/vault-keys.json"

# Check if Vault is running
if ! curl -s $VAULT_ADDR/v1/sys/health > /dev/null 2>&1; then
    echo -e "${RED}Error: Vault is not running!${NC}"
    echo "Start Vault first with: make run-vault"
    exit 1
fi

# Check if already initialized
INIT_STATUS=$(curl -s $VAULT_ADDR/v1/sys/init | python3 -c "import sys,json; print(json.load(sys.stdin).get('initialized', False))" 2>/dev/null || echo "false")

if [ "$INIT_STATUS" = "true" ]; then
    echo -e "${YELLOW}Vault is already initialized${NC}"
    
    # Try to unseal if needed
    SEALED=$(curl -s $VAULT_ADDR/v1/sys/health | python3 -c "import sys,json; print(json.load(sys.stdin).get('sealed', True))" 2>/dev/null || echo "true")
    if [ "$SEALED" = "True" ]; then
        if [ -f "$VAULT_KEYS_FILE" ]; then
            echo "Unsealing Vault..."
            UNSEAL_KEY=$(python3 -c "import json; data=json.load(open('$VAULT_KEYS_FILE')); print(data.get('unseal_keys_b64', data.get('keys_base64', []))[0])" 2>/dev/null)
            curl -s -X PUT $VAULT_ADDR/v1/sys/unseal \
                -H "Content-Type: application/json" \
                -d "{\"key\": \"$UNSEAL_KEY\"}" > /dev/null
            echo -e "${GREEN}✓ Vault unsealed${NC}"
        else
            echo -e "${RED}Error: Vault is sealed and keys file not found!${NC}"
            echo "Keys should be at: $VAULT_KEYS_FILE"
            exit 1
        fi
    else
        echo -e "${GREEN}✓ Vault is already unsealed${NC}"
    fi
else
    echo -e "\n${GREEN}Initializing Vault for the first time...${NC}"
    
    # Initialize with 1 key share for development (use more for production)
    INIT_OUTPUT=$(curl -s -X PUT $VAULT_ADDR/v1/sys/init \
        -H "Content-Type: application/json" \
        -d '{"secret_shares": 1, "secret_threshold": 1}')
    
    # Save keys
    mkdir -p $(dirname "$VAULT_KEYS_FILE")
    echo "$INIT_OUTPUT" > "$VAULT_KEYS_FILE"
    chmod 600 "$VAULT_KEYS_FILE"
    
    # Extract keys
    UNSEAL_KEY=$(echo "$INIT_OUTPUT" | python3 -c "import sys,json; data=json.load(sys.stdin); print(data.get('keys_base64', [])[0])")
    ROOT_TOKEN=$(echo "$INIT_OUTPUT" | python3 -c "import sys,json; data=json.load(sys.stdin); print(data.get('root_token', ''))")
    
    # Unseal Vault
    echo "Unsealing Vault..."
    curl -s -X PUT $VAULT_ADDR/v1/sys/unseal \
        -H "Content-Type: application/json" \
        -d "{\"key\": \"$UNSEAL_KEY\"}" > /dev/null
    
    echo -e "${GREEN}✅ Vault initialized and unsealed${NC}"
    echo -e "${YELLOW}IMPORTANT: Vault keys saved to: $VAULT_KEYS_FILE${NC}"
    echo -e "${RED}Keep this file safe! You'll need it to unseal Vault after restarts.${NC}"
fi

# Get root token
if [ -f "$VAULT_KEYS_FILE" ]; then
    ROOT_TOKEN=$(python3 -c "import json; print(json.load(open('$VAULT_KEYS_FILE')).get('root_token', ''))" 2>/dev/null)
    
    # Enable KV v2 secrets engine if not already enabled
    echo -e "\n${GREEN}Setting up secrets engine...${NC}"
    curl -s -X POST $VAULT_ADDR/v1/sys/mounts/secret \
        -H "X-Vault-Token: $ROOT_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{"type": "kv-v2"}' > /dev/null 2>&1 || true
    
    # Save token for easy access
    echo "$ROOT_TOKEN" > "$HOME/.mExOms/vault-token"
    chmod 600 "$HOME/.mExOms/vault-token"
    
    echo -e "${GREEN}✅ Secrets engine ready${NC}"
fi

echo -e "\n${GREEN}Vault is ready to use!${NC}"
echo -e "Root token: ${YELLOW}$(cat $HOME/.mExOms/vault-token)${NC}"
echo -e "Web UI: ${YELLOW}$VAULT_ADDR${NC}"
echo -e "\nTo store API keys: ${YELLOW}./scripts/store-binance-keys.sh${NC}"