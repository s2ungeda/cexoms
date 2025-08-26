#!/bin/bash

echo "Stopping OMS Services..."

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Function to stop service
stop_service() {
    if pgrep -f "$1" > /dev/null; then
        echo -e "${YELLOW}Stopping $2...${NC}"
        pkill -f "$1"
        sleep 1
        if pgrep -f "$1" > /dev/null; then
            echo -e "${RED}✗${NC} Failed to stop $2, force killing..."
            pkill -9 -f "$1"
        else
            echo -e "${GREEN}✓${NC} $2 stopped"
        fi
    else
        echo -e "${YELLOW}!${NC} $2 is not running"
    fi
}

# Stop all services
stop_service "oms-core" "OMS Core"
stop_service "cmd/oms-server/main.go" "OMS Server"
stop_service "services/binance/cmd/spot/main.go" "Binance Spot"
stop_service "services/binance/cmd/futures/main.go" "Binance Futures"
stop_service "cmd/test-monitoring/main.go" "Monitoring Service"

# Clean up any demo services
./stop-oms-demo.sh 2>/dev/null

echo -e "\n${GREEN}All services stopped.${NC}"

# Check if any services are still running
remaining=$(ps aux | grep -E "oms-core|oms-server|binance|test-monitoring" | grep -v grep | wc -l)
if [ $remaining -gt 0 ]; then
    echo -e "\n${YELLOW}Warning: Some processes may still be running:${NC}"
    ps aux | grep -E "oms-core|oms-server|binance|test-monitoring" | grep -v grep
fi