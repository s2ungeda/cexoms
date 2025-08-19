#!/bin/bash

# Service Health Check Script

echo "Multi-Exchange OMS Service Health Check"
echo "======================================="
echo ""

# Color codes
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check function
check_service() {
    local name=$1
    local check_cmd=$2
    
    printf "Checking %-20s ... " "$name"
    
    if eval "$check_cmd" > /dev/null 2>&1; then
        echo -e "${GREEN}✓ HEALTHY${NC}"
        return 0
    else
        echo -e "${RED}✗ UNHEALTHY${NC}"
        return 1
    fi
}

# Check C++ Core
check_service "C++ Core Engine" "pgrep -f oms-core"

# Check gRPC Server
check_service "gRPC Server" "nc -zv localhost 50051"

# Check Binance Spot API
check_service "Binance Spot API" "curl -s https://api.binance.com/api/v3/ping"

# Check Binance Futures API
check_service "Binance Futures API" "curl -s https://fapi.binance.com/fapi/v1/ping"

# Check processes
echo ""
echo "Running Processes:"
echo "------------------"
ps aux | grep -E "(oms-core|oms-server|binance-)" | grep -v grep | awk '{printf "%-20s PID: %-6s CPU: %-5s MEM: %-5s\n", $11, $2, $3, $4}'

# Check memory usage
echo ""
echo "Memory Usage:"
echo "-------------"
free -h | grep -E "(total|Mem:|Swap:)"

# Check disk usage
echo ""
echo "Disk Usage (Project):"
echo "--------------------"
du -sh . 2>/dev/null
du -sh bin/ core/build/ logs/ 2>/dev/null | sed 's/^/  /'

# Performance metrics
echo ""
echo "Performance Metrics:"
echo "-------------------"
if pgrep -f oms-core > /dev/null; then
    pid=$(pgrep -f oms-core | head -1)
    echo "C++ Core Engine (PID: $pid):"
    ps -p $pid -o %cpu,%mem,vsz,rss,etime | tail -1 | awk '{printf "  CPU: %s%%, Memory: %s%%, VSZ: %s, RSS: %s, Uptime: %s\n", $1, $2, $3, $4, $5}'
fi

echo ""
echo "Test Complete!"