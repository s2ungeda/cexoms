#!/bin/bash

# Vault Persistent Setup Script
set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${YELLOW}=== Setting up Persistent Vault ===${NC}"

# Stop and remove existing dev vault
docker stop vault-oms 2>/dev/null || true
docker rm vault-oms 2>/dev/null || true

# Create vault data directory
VAULT_DATA_DIR="$HOME/.mExOms/vault-data"
mkdir -p $VAULT_DATA_DIR

# Start Vault in production mode
echo -e "\n${GREEN}Starting Vault in production mode...${NC}"
docker run -d --name vault-oms \
    -p 8200:8200 \
    -v $VAULT_DATA_DIR:/vault/data \
    -v $(pwd)/configs/vault-config.hcl:/vault/config/config.hcl \
    --cap-add IPC_LOCK \
    hashicorp/vault:latest server

sleep 3

# Check if already initialized
export VAULT_ADDR='http://localhost:8200'

echo -e "\n${GREEN}Checking Vault status...${NC}"
INIT_STATUS=$(docker exec vault-oms vault status -format=json 2>/dev/null | jq -r .initialized || echo "false")

if [ "$INIT_STATUS" = "false" ]; then
    echo -e "\n${GREEN}Initializing Vault...${NC}"
    # Initialize with 1 key share for simplicity (increase for production)
    INIT_OUTPUT=$(docker exec vault-oms vault operator init -key-shares=1 -key-threshold=1 -format=json)
    
    # Extract keys
    UNSEAL_KEY=$(echo $INIT_OUTPUT | jq -r '.unseal_keys_b64[0]')
    ROOT_TOKEN=$(echo $INIT_OUTPUT | jq -r '.root_token')
    
    # Save keys securely
    KEYS_FILE="$HOME/.mExOms/vault-keys.json"
    echo $INIT_OUTPUT > $KEYS_FILE
    chmod 600 $KEYS_FILE
    
    echo -e "${GREEN}✓ Vault initialized${NC}"
    echo -e "${YELLOW}IMPORTANT: Vault keys saved to: $KEYS_FILE${NC}"
    echo -e "${RED}Keep this file safe! You'll need it to unseal Vault after restarts.${NC}"
else
    echo -e "${YELLOW}Vault already initialized${NC}"
    KEYS_FILE="$HOME/.mExOms/vault-keys.json"
    if [ -f "$KEYS_FILE" ]; then
        UNSEAL_KEY=$(jq -r '.unseal_keys_b64[0]' $KEYS_FILE)
        ROOT_TOKEN=$(jq -r '.root_token' $KEYS_FILE)
    else
        echo -e "${RED}Error: Vault keys file not found at $KEYS_FILE${NC}"
        echo -e "${RED}You'll need to manually unseal Vault${NC}"
        exit 1
    fi
fi

# Check if sealed
SEALED=$(docker exec vault-oms vault status -format=json | jq -r .sealed)
if [ "$SEALED" = "true" ]; then
    echo -e "\n${GREEN}Unsealing Vault...${NC}"
    docker exec vault-oms vault operator unseal $UNSEAL_KEY
    echo -e "${GREEN}✓ Vault unsealed${NC}"
fi

# Enable KV v2 secrets engine
echo -e "\n${GREEN}Setting up secrets engine...${NC}"
docker exec -e VAULT_TOKEN=$ROOT_TOKEN vault-oms vault secrets enable -path=secret kv-v2 2>/dev/null || true

# Save root token for easy access (development only)
echo $ROOT_TOKEN > $HOME/.mExOms/vault-token
chmod 600 $HOME/.mExOms/vault-token

echo -e "\n${GREEN}✅ Persistent Vault setup complete!${NC}"
echo -e "\nVault is now running with persistent storage at: ${YELLOW}$VAULT_DATA_DIR${NC}"
echo -e "Root token saved at: ${YELLOW}$HOME/.mExOms/vault-token${NC}"
echo -e "\nTo store Binance keys, run: ${YELLOW}./scripts/store-binance-keys.sh${NC}"

# Create auto-unseal script
cat > scripts/unseal-vault.sh << 'EOF'
#!/bin/bash
# Auto-unseal Vault after restart

KEYS_FILE="$HOME/.mExOms/vault-keys.json"
if [ ! -f "$KEYS_FILE" ]; then
    echo "Error: Vault keys file not found at $KEYS_FILE"
    exit 1
fi

UNSEAL_KEY=$(jq -r '.unseal_keys_b64[0]' $KEYS_FILE)
export VAULT_ADDR='http://localhost:8200'

# Wait for Vault to be ready
for i in {1..10}; do
    if docker exec vault-oms vault status &>/dev/null; then
        break
    fi
    echo "Waiting for Vault to start..."
    sleep 2
done

# Unseal
SEALED=$(docker exec vault-oms vault status -format=json | jq -r .sealed)
if [ "$SEALED" = "true" ]; then
    echo "Unsealing Vault..."
    docker exec vault-oms vault operator unseal $UNSEAL_KEY
    echo "✓ Vault unsealed"
else
    echo "Vault is already unsealed"
fi
EOF

chmod +x scripts/unseal-vault.sh

echo -e "\nCreated auto-unseal script: ${GREEN}./scripts/unseal-vault.sh${NC}"