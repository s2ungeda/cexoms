#!/bin/bash

# Use system keyring for secure key storage
set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${YELLOW}=== System Keyring Setup ===${NC}"

# Detect OS and available keyring
if command -v secret-tool &> /dev/null; then
    # Linux with GNOME Keyring
    KEYRING_TYPE="gnome"
elif command -v security &> /dev/null; then
    # macOS Keychain
    KEYRING_TYPE="macos"
elif command -v wincred &> /dev/null; then
    # Windows Credential Manager
    KEYRING_TYPE="windows"
else
    echo -e "${RED}No system keyring found!${NC}"
    echo "Please install: gnome-keyring (Linux), or use macOS/Windows"
    exit 1
fi

echo -e "${GREEN}Detected keyring: $KEYRING_TYPE${NC}"

# Function to store keys
store_keys() {
    local api_key=$1
    local secret_key=$2
    
    case $KEYRING_TYPE in
        gnome)
            # GNOME Keyring (Linux)
            echo -n "$api_key" | secret-tool store --label="OMS Binance API Key" \
                service oms account binance type api_key
            echo -n "$secret_key" | secret-tool store --label="OMS Binance Secret Key" \
                service oms account binance type secret_key
            ;;
        macos)
            # macOS Keychain
            security add-generic-password -a "oms-binance" -s "oms-api-key" \
                -w "$api_key" -U
            security add-generic-password -a "oms-binance" -s "oms-secret-key" \
                -w "$secret_key" -U
            ;;
        windows)
            # Windows Credential Manager (WSL)
            cmdkey /generic:OMS_BINANCE_API /user:api_key /pass:"$api_key"
            cmdkey /generic:OMS_BINANCE_SECRET /user:secret_key /pass:"$secret_key"
            ;;
    esac
}

# Function to retrieve keys
retrieve_keys() {
    case $KEYRING_TYPE in
        gnome)
            API_KEY=$(secret-tool lookup service oms account binance type api_key)
            SECRET_KEY=$(secret-tool lookup service oms account binance type secret_key)
            ;;
        macos)
            API_KEY=$(security find-generic-password -a "oms-binance" -s "oms-api-key" -w)
            SECRET_KEY=$(security find-generic-password -a "oms-binance" -s "oms-secret-key" -w)
            ;;
        windows)
            # WSL can't directly access Windows Credential Manager
            echo "For Windows, use PowerShell to retrieve credentials"
            ;;
    esac
}

# Interactive setup
echo -e "\n${GREEN}Enter your Binance API credentials:${NC}"
read -p "API Key: " api_key
echo -n "Secret Key: "
read -s secret_key
echo

# Store in system keyring
store_keys "$api_key" "$secret_key"

echo -e "\n${GREEN}✅ Keys stored in system keyring${NC}"

# Create wrapper script to use keyring
cat > scripts/run-with-keyring.sh << 'EOF'
#!/bin/bash
# Run OMS with keys from system keyring

source scripts/use-system-keyring.sh
retrieve_keys

export BINANCE_API_KEY="$API_KEY"
export BINANCE_SECRET_KEY="$SECRET_KEY"

# Run the command
exec "$@"
EOF
chmod +x scripts/run-with-keyring.sh

echo -e "\nTo run services with keyring:"
echo -e "${YELLOW}./scripts/run-with-keyring.sh go run cmd/binance-spot-balance/main.go${NC}"