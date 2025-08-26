#!/bin/bash

echo "Stopping OMS Demo Services..."

# Stop mock services
if [ -f .oms-core.pid ]; then
    kill $(cat .oms-core.pid) 2>/dev/null
    rm .oms-core.pid
fi

if [ -f .binance-spot.pid ]; then
    kill $(cat .binance-spot.pid) 2>/dev/null
    rm .binance-spot.pid
fi

if [ -f .binance-futures.pid ]; then
    kill $(cat .binance-futures.pid) 2>/dev/null
    rm .binance-futures.pid
fi

# Kill any remaining mock processes
pkill -f "oms-core.log"
pkill -f "binance-spot.log"
pkill -f "binance-futures.log"

echo "Mock services stopped!"