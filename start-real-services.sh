#!/bin/bash

echo "Starting Real OMS Services..."

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Create necessary directories
mkdir -p logs data/snapshots data/logs

# Function to check if service is running
check_service() {
    if pgrep -f "$1" > /dev/null; then
        echo -e "${GREEN}✓${NC} $2 is already running"
        return 0
    else
        return 1
    fi
}

# Start C++ Core Engine
echo -e "\n${YELLOW}Starting C++ Core Engine...${NC}"
if ! check_service "oms-core" "OMS Core"; then
    cd core/build
    nohup ./oms-core > ../../logs/oms-core.log 2>&1 &
    cd ../..
    sleep 2
    if check_service "oms-core" "OMS Core"; then
        echo -e "${GREEN}✓${NC} OMS Core started successfully"
    else
        echo -e "${RED}✗${NC} Failed to start OMS Core"
    fi
fi

# Start OMS Server
echo -e "\n${YELLOW}Starting OMS Server...${NC}"
if ! check_service "oms-server" "OMS Server"; then
    if [ -f cmd/oms-server/main.go ]; then
        nohup go run cmd/oms-server/main.go > logs/oms-server.log 2>&1 &
        sleep 3
        if check_service "oms-server" "OMS Server"; then
            echo -e "${GREEN}✓${NC} OMS Server started successfully"
        else
            echo -e "${RED}✗${NC} Failed to start OMS Server"
        fi
    else
        echo -e "${YELLOW}!${NC} OMS Server not found, skipping..."
    fi
fi

# Start Binance Spot Connector
echo -e "\n${YELLOW}Starting Binance Spot Connector...${NC}"
if ! check_service "binance-spot" "Binance Spot"; then
    if [ -f services/binance/cmd/spot/main.go ]; then
        nohup go run services/binance/cmd/spot/main.go > logs/binance-spot.log 2>&1 &
        sleep 3
        if check_service "binance-spot" "Binance Spot"; then
            echo -e "${GREEN}✓${NC} Binance Spot started successfully"
        else
            echo -e "${RED}✗${NC} Failed to start Binance Spot"
        fi
    else
        echo -e "${YELLOW}!${NC} Binance Spot connector not found, skipping..."
    fi
fi

# Start Binance Futures Connector
echo -e "\n${YELLOW}Starting Binance Futures Connector...${NC}"
if ! check_service "binance-futures" "Binance Futures"; then
    if [ -f services/binance/cmd/futures/main.go ]; then
        nohup go run services/binance/cmd/futures/main.go > logs/binance-futures.log 2>&1 &
        sleep 3
        if check_service "binance-futures" "Binance Futures"; then
            echo -e "${GREEN}✓${NC} Binance Futures started successfully"
        else
            echo -e "${RED}✗${NC} Failed to start Binance Futures"
        fi
    else
        echo -e "${YELLOW}!${NC} Binance Futures connector not found, skipping..."
    fi
fi

# Start Test Monitoring Service
echo -e "\n${YELLOW}Starting Monitoring Service...${NC}"
if ! check_service "test-monitoring" "Monitoring"; then
    nohup go run cmd/test-monitoring/main.go > logs/monitoring.log 2>&1 &
    sleep 3
    if check_service "test-monitoring" "Monitoring"; then
        echo -e "${GREEN}✓${NC} Monitoring Service started on http://localhost:9090"
    else
        echo -e "${RED}✗${NC} Failed to start Monitoring Service"
    fi
fi

echo -e "\n${GREEN}Services Status:${NC}"
echo "================================"
ps aux | grep -E "oms-core|oms-server|binance|test-monitoring" | grep -v grep | awk '{printf "%-20s PID: %-8s CPU: %s%% MEM: %s%%\n", $11, $2, $3, $4}'

echo -e "\n${GREEN}Log Files:${NC}"
echo "================================"
echo "- OMS Core:        tail -f logs/oms-core.log"
echo "- OMS Server:      tail -f logs/oms-server.log"
echo "- Binance Spot:    tail -f logs/binance-spot.log"
echo "- Binance Futures: tail -f logs/binance-futures.log"
echo "- Monitoring:      tail -f logs/monitoring.log"

echo -e "\n${GREEN}Monitoring Dashboard:${NC}"
echo "================================"
echo "- Web Dashboard: http://localhost:9090/dashboard"
echo "- Metrics:       http://localhost:9090/metrics"
echo "- Health:        http://localhost:9090/health"

echo -e "\n${YELLOW}To stop all services, run:${NC} ./stop-real-services.sh"