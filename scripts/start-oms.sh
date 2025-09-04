#!/bin/bash

# OMS Start Script
set -e

echo "🚀 Starting mExOms System..."

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# Check if services are already running
check_service() {
    if pgrep -f "$1" > /dev/null; then
        echo -e "${YELLOW}⚠️  $2 is already running${NC}"
        return 1
    fi
    return 0
}

# Start infrastructure
echo -e "\n${GREEN}1. Starting Infrastructure Services${NC}"

# Check Docker services
if ! docker ps | grep -q "nats-oms"; then
    echo "Starting NATS..."
    make run-nats > /dev/null 2>&1 &
    sleep 2
else
    echo "NATS is already running"
fi

if ! docker ps | grep -q "redis-oms"; then
    echo "Starting Redis..."
    make run-redis > /dev/null 2>&1 &
    sleep 2
else
    echo "Redis is already running"
fi

if ! docker ps | grep -q "vault-oms"; then
    echo "Starting Vault..."
    make run-vault > /dev/null 2>&1
    sleep 3
    # Initialize if first time
    if [ ! -f "$HOME/.mExOms/vault-keys.json" ]; then
        echo "First time setup - initializing Vault..."
        ./scripts/init-vault.sh
    else
        # Auto-unseal
        echo "Unsealing Vault..."
        VAULT_ADDR='http://localhost:8200'
        UNSEAL_KEY=$(jq -r '.unseal_keys_b64[0] // .keys_base64[0]' "$HOME/.mExOms/vault-keys.json")
        curl -s -X PUT $VAULT_ADDR/v1/sys/unseal \
            -H "Content-Type: application/json" \
            -d "{\"key\": \"$UNSEAL_KEY\"}" > /dev/null
    fi
else
    echo "Vault is already running"
    # Check if sealed and unseal if needed
    VAULT_ADDR='http://localhost:8200'
    SEALED=$(curl -s $VAULT_ADDR/v1/sys/health | jq -r .sealed 2>/dev/null || echo "true")
    if [ "$SEALED" = "true" ] && [ -f "$HOME/.mExOms/vault-keys.json" ]; then
        echo "Unsealing Vault..."
        UNSEAL_KEY=$(jq -r '.unseal_keys_b64[0] // .keys_base64[0]' "$HOME/.mExOms/vault-keys.json")
        curl -s -X PUT $VAULT_ADDR/v1/sys/unseal \
            -H "Content-Type: application/json" \
            -d "{\"key\": \"$UNSEAL_KEY\"}" > /dev/null
    fi
fi

# Start OMS services
echo -e "\n${GREEN}2. Starting OMS Services${NC}"

# Market data service
if check_service "binance-market-full" "Market service"; then
    echo "Starting Market Data Service..."
    cd /home/seunge/project/mExOms
    go run cmd/binance-market-full/main.go > logs/market.log 2>&1 &
    sleep 2
fi

# Balance service (only if API keys are configured)
if [ ! -z "$BINANCE_API_KEY" ] || [ -f "/home/seunge/project/mExOms/.env" ]; then
    if check_service "binance-spot-balance" "Balance service"; then
        echo "Starting Balance Service..."
        cd /home/seunge/project/mExOms
        if [ -f ".env" ]; then
            source .env
            export BINANCE_API_KEY BINANCE_SECRET_KEY
        fi
        go run cmd/binance-spot-balance/main.go > logs/balance.log 2>&1 &
        sleep 2
    fi
else
    echo -e "${YELLOW}⚠️  Skipping Balance Service (No API keys configured)${NC}"
fi

# Dashboard
echo -e "\n${GREEN}3. Starting Dashboard${NC}"

# Dashboard server
if check_service "oms-dashboard-real" "Dashboard server"; then
    echo "Starting Dashboard Server..."
    cd /home/seunge/project/mExOms/dashboard
    if [ ! -f "oms-dashboard-real" ]; then
        echo "Building dashboard server..."
        go build -o oms-dashboard-real server/main_real.go
    fi
    ./oms-dashboard-real > ../logs/dashboard-server.log 2>&1 &
    sleep 2
fi

# Frontend (if not already running)
if ! curl -s http://localhost:3000 > /dev/null 2>&1; then
    echo "Starting Frontend..."
    cd /home/seunge/project/mExOms/dashboard/frontend
    npm start > ../../logs/frontend.log 2>&1 &
else
    echo "Frontend is already running"
fi

# Wait for services to start
echo -e "\n${GREEN}4. Verifying Services...${NC}"
sleep 5

# Check services
echo -e "\n${GREEN}✅ Service Status:${NC}"
echo "Infrastructure:"
docker ps | grep -E "nats|redis|vault" | awk '{print "  - " $NF ": Running"}'

echo -e "\nOMS Services:"
ps aux | grep -E "binance-market|balance|dashboard" | grep -v grep | awk '{print "  - " $11 ": Running"}' | sort -u

echo -e "\n${GREEN}✅ OMS is ready!${NC}"
echo -e "Dashboard: ${GREEN}http://localhost:3000${NC}"
echo -e "WebSocket: ${GREEN}ws://localhost:8080/ws${NC}"

# Show logs location
echo -e "\n📝 Logs available at:"
echo "  - Market: logs/market.log"
echo "  - Balance: logs/balance.log"
echo "  - Dashboard: logs/dashboard-server.log"
echo "  - Frontend: logs/frontend.log"

echo -e "\nTo stop all services, run: ${YELLOW}./scripts/stop-oms.sh${NC}"