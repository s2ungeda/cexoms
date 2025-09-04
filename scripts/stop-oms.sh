#!/bin/bash

# OMS Stop Script
echo "🛑 Stopping mExOms System..."

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# Stop OMS services
echo -e "\n${YELLOW}Stopping OMS Services...${NC}"

# Kill processes
pkill -f "binance-market-full" 2>/dev/null && echo "  - Stopped Market Service"
pkill -f "binance-spot-balance" 2>/dev/null && echo "  - Stopped Balance Service"
pkill -f "oms-dashboard-real" 2>/dev/null && echo "  - Stopped Dashboard Server"

# Stop frontend (find the npm process)
FRONTEND_PID=$(ps aux | grep "react-scripts start" | grep -v grep | awk '{print $2}')
if [ ! -z "$FRONTEND_PID" ]; then
    kill $FRONTEND_PID 2>/dev/null && echo "  - Stopped Frontend"
fi

# Stop infrastructure (optional)
read -p "Stop infrastructure services (NATS, Redis, Vault)? [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo -e "\n${YELLOW}Stopping Infrastructure...${NC}"
    docker stop nats-oms 2>/dev/null && echo "  - Stopped NATS"
    docker stop redis-oms 2>/dev/null && echo "  - Stopped Redis"
    docker stop vault-oms 2>/dev/null && echo "  - Stopped Vault"
fi

echo -e "\n${GREEN}✅ OMS stopped successfully${NC}"

# Check if any services are still running
echo -e "\n${YELLOW}Checking remaining processes...${NC}"
REMAINING=$(ps aux | grep -E "binance|dashboard|react-scripts" | grep -v grep | wc -l)
if [ $REMAINING -gt 0 ]; then
    echo -e "${RED}⚠️  Some processes may still be running:${NC}"
    ps aux | grep -E "binance|dashboard|react-scripts" | grep -v grep
else
    echo -e "${GREEN}✅ All OMS processes stopped${NC}"
fi