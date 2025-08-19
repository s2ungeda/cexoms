#!/bin/bash

# Multi-Exchange OMS - Run All Services Script

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_DIR"

echo "Starting Multi-Exchange OMS Services..."
echo "======================================"

# Function to start a service in background
start_service() {
    local name=$1
    local cmd=$2
    echo "Starting $name..."
    $cmd > logs/${name}.log 2>&1 &
    echo "$!" > logs/${name}.pid
    sleep 1
    if kill -0 $(cat logs/${name}.pid) 2>/dev/null; then
        echo "✓ $name started (PID: $(cat logs/${name}.pid))"
    else
        echo "✗ $name failed to start"
        return 1
    fi
}

# Create logs directory
mkdir -p logs

# Start C++ Core Engine
start_service "oms-core" "./core/build/oms-core"

# Wait for core to initialize
sleep 2

# Start gRPC Server
start_service "oms-server" "./bin/oms-server"

# Start Exchange Connectors
start_service "binance-spot" "./bin/binance-spot"
start_service "binance-futures" "./bin/binance-futures"

echo ""
echo "All services started!"
echo "===================="
echo ""
echo "Service Status:"
echo "--------------"
ps aux | grep -E "(oms-core|oms-server|binance-)" | grep -v grep

echo ""
echo "Log files:"
echo "----------"
ls -la logs/*.log

echo ""
echo "To stop all services, run: ./scripts/stop-all.sh"