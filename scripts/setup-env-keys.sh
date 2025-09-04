#!/bin/bash

# Setup environment variables for API keys
set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${YELLOW}=== API Key Setup ===${NC}"

# Check if .env exists
if [ -f .env ]; then
    echo -e "${YELLOW}Found existing .env file${NC}"
    read -p "Do you want to update it? (y/n): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 0
    fi
fi

# Interactive prompts
echo -e "\n${GREEN}Enter your Binance API credentials:${NC}"
read -p "API Key: " api_key
read -s -p "Secret Key: " secret_key
echo

# Create .env file
cat > .env << EOF
# Binance API Keys
BINANCE_API_KEY=$api_key
BINANCE_SECRET_KEY=$secret_key

# Vault config (optional)
VAULT_ADDR=http://localhost:8200
VAULT_TOKEN=root-token
EOF

# Secure the file
chmod 600 .env

echo -e "\n${GREEN}✅ API keys saved to .env file${NC}"
echo -e "${YELLOW}Note: Make sure .env is in .gitignore${NC}"

# Check .gitignore
if ! grep -q "^\.env$" .gitignore 2>/dev/null; then
    echo -e "\n${RED}Warning: .env is not in .gitignore!${NC}"
    echo ".env" >> .gitignore
    echo -e "${GREEN}Added .env to .gitignore${NC}"
fi