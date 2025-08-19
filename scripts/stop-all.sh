#!/bin/bash

# Multi-Exchange OMS - Stop All Services Script

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_DIR"

echo "Stopping Multi-Exchange OMS Services..."
echo "======================================"

# Function to stop a service
stop_service() {
    local name=$1
    local pidfile="logs/${name}.pid"
    
    if [ -f "$pidfile" ]; then
        local pid=$(cat "$pidfile")
        if kill -0 "$pid" 2>/dev/null; then
            echo "Stopping $name (PID: $pid)..."
            kill -TERM "$pid"
            sleep 1
            if kill -0 "$pid" 2>/dev/null; then
                echo "Force stopping $name..."
                kill -KILL "$pid"
            fi
            rm -f "$pidfile"
            echo "✓ $name stopped"
        else
            echo "✗ $name not running (stale PID file)"
            rm -f "$pidfile"
        fi
    else
        echo "✗ $name not running (no PID file)"
    fi
}

# Stop all services
stop_service "binance-futures"
stop_service "binance-spot"
stop_service "oms-server"
stop_service "oms-core"

echo ""
echo "All services stopped!"

# Check if any services are still running
echo ""
echo "Checking for remaining processes..."
remaining=$(ps aux | grep -E "(oms-core|oms-server|binance-)" | grep -v grep | grep -v stop-all)
if [ -n "$remaining" ]; then
    echo "Warning: Some processes are still running:"
    echo "$remaining"
else
    echo "✓ All OMS processes terminated"
fi