#!/bin/bash

echo "Starting OMS Demo Services..."

# Create necessary directories
mkdir -p logs data/snapshots data/logs

# Start mock services (for demo)
echo "Starting mock OMS services..."

# Mock OMS Core
(while true; do 
    echo "[$(date)] Risk checks: $(( RANDOM % 1000 + 100 )), avg latency: 0.$(( RANDOM % 200 + 100 )) μs" >> logs/oms-core.log
    sleep 2
done) &
echo $! > .oms-core.pid

# Mock Binance Spot
(while true; do
    echo "[$(date)] Heartbeat - Binance Spot connector running" >> logs/binance-spot.log
    sleep 5
done) &
echo $! > .binance-spot.pid

# Mock Binance Futures
(while true; do
    echo "[$(date)] Heartbeat - Binance Futures connector running" >> logs/binance-futures.log
    sleep 5
done) &
echo $! > .binance-futures.pid

echo "Mock services started!"
echo "Check monitoring dashboard at http://localhost:8080"
echo ""
echo "To stop services, run: ./stop-oms-demo.sh"