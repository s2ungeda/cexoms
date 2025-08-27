#!/bin/bash

# Start real OMS dashboard (connects to actual OMS backend)

echo "Starting OMS Dashboard in Real Mode..."

# Check if NATS is running
if ! nc -z localhost 4222 > /dev/null 2>&1; then
    echo "ERROR: NATS server is not running on localhost:4222"
    echo "Please start your OMS services first"
    exit 1
fi

# Build the real server
echo "Building real OMS server..."
cd server
go build -o oms-dashboard-real main_real.go
if [ $? -ne 0 ]; then
    echo "Build failed"
    exit 1
fi

# Start the server
echo "Starting real OMS dashboard server..."
NATS_URL=nats://localhost:4222 ./oms-dashboard-real &
SERVER_PID=$!

echo "Server started with PID: $SERVER_PID"

# Wait a bit for server to start
sleep 2

# Start the frontend in development mode
echo "Starting frontend..."
cd ../frontend
npm start &
FRONTEND_PID=$!

echo ""
echo "=================================="
echo "OMS Dashboard (Real Mode) Started"
echo "=================================="
echo "Dashboard URL: http://localhost:3000"
echo "WebSocket URL: ws://localhost:8080/ws"
echo "Metrics URL: http://localhost:8080/metrics"
echo "Health Check: http://localhost:8080/health"
echo ""
echo "Server PID: $SERVER_PID"
echo "Frontend PID: $FRONTEND_PID"
echo ""
echo "Press Ctrl+C to stop all services"
echo "=================================="

# Function to handle cleanup
cleanup() {
    echo ""
    echo "Stopping services..."
    kill $SERVER_PID 2>/dev/null
    kill $FRONTEND_PID 2>/dev/null
    echo "Services stopped"
    exit 0
}

# Set up signal handler
trap cleanup SIGINT SIGTERM

# Wait for interrupt
while true; do
    sleep 1
done