#!/bin/bash

# Load API keys from Vault and set as environment variables

VAULT_ADDR="http://127.0.0.1:8200"
VAULT_TOKEN="root-token"

# Get Binance Spot keys
echo "Loading Binance Spot API keys from Vault..."
SPOT_KEYS=$(curl -s -H "X-Vault-Token: $VAULT_TOKEN" $VAULT_ADDR/v1/secret/data/exchanges/binance_spot 2>/dev/null)

if [ $? -eq 0 ] && [ -n "$SPOT_KEYS" ]; then
    API_KEY=$(echo $SPOT_KEYS | python3 -c "import sys, json; data=json.load(sys.stdin); print(data['data']['data']['api_key'] if 'data' in data else '')" 2>/dev/null)
    SECRET_KEY=$(echo $SPOT_KEYS | python3 -c "import sys, json; data=json.load(sys.stdin); print(data['data']['data']['secret_key'] if 'data' in data else '')" 2>/dev/null)
    
    if [ -n "$API_KEY" ] && [ -n "$SECRET_KEY" ]; then
        export BINANCE_API_KEY="$API_KEY"
        export BINANCE_SECRET_KEY="$SECRET_KEY"
        echo "✓ Binance Spot API keys loaded successfully"
    else
        echo "✗ Failed to parse Binance Spot API keys"
    fi
else
    echo "✗ Failed to fetch Binance Spot API keys from Vault"
fi

# Get Binance Futures keys (if needed)
echo "Loading Binance Futures API keys from Vault..."
FUTURES_KEYS=$(curl -s -H "X-Vault-Token: $VAULT_TOKEN" $VAULT_ADDR/v1/secret/data/exchanges/binance_futures 2>/dev/null)

if [ $? -eq 0 ] && [ -n "$FUTURES_KEYS" ]; then
    FUTURES_API_KEY=$(echo $FUTURES_KEYS | python3 -c "import sys, json; data=json.load(sys.stdin); print(data['data']['data']['api_key'] if 'data' in data else '')" 2>/dev/null)
    FUTURES_SECRET_KEY=$(echo $FUTURES_KEYS | python3 -c "import sys, json; data=json.load(sys.stdin); print(data['data']['data']['secret_key'] if 'data' in data else '')" 2>/dev/null)
    
    if [ -n "$FUTURES_API_KEY" ] && [ -n "$FUTURES_SECRET_KEY" ]; then
        # If futures keys are different, you might want to handle them separately
        # For now, we'll use the same env vars as Binance API uses the same keys for both
        if [ -z "$BINANCE_API_KEY" ]; then
            export BINANCE_API_KEY="$FUTURES_API_KEY"
            export BINANCE_SECRET_KEY="$FUTURES_SECRET_KEY"
            echo "✓ Binance Futures API keys loaded successfully"
        fi
    else
        echo "✗ Failed to parse Binance Futures API keys"
    fi
else
    echo "✗ Failed to fetch Binance Futures API keys from Vault"
fi

# Display status
if [ -n "$BINANCE_API_KEY" ]; then
    echo ""
    echo "Environment variables set:"
    echo "- BINANCE_API_KEY: ${BINANCE_API_KEY:0:10}..."
    echo "- BINANCE_SECRET_KEY: ${BINANCE_SECRET_KEY:0:10}..."
    echo ""
    echo "You can now run the Binance connectors:"
    echo "  cd /home/seunge/project/mExOms/cmd/binance-spot-balance && go run main.go"
    echo "  cd /home/seunge/project/mExOms/cmd/binance-futures-position && go run main.go"
else
    echo ""
    echo "No API keys loaded. Please use vault-cli to add your API keys first."
fi