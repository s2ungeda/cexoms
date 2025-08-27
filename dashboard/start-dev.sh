#!/bin/bash

echo "Starting mExOms Dashboard in Development Mode..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if NATS is running
if ! nc -z localhost 4222 2>/dev/null; then
    echo -e "${YELLOW}NATS is not running. Starting NATS in Docker...${NC}"
    docker run -d --name nats -p 4222:4222 -p 8222:8222 nats:latest -js -m 8222
    sleep 2
else
    echo -e "${GREEN}NATS is already running${NC}"
fi

# Start dashboard server
echo -e "${YELLOW}Starting Dashboard Server...${NC}"
cd server
go run main.go &
SERVER_PID=$!
cd ..

# Wait for server to start
sleep 3

# Start data generator
echo -e "${YELLOW}Starting Demo Data Generator...${NC}"
cd demo
go run data_generator.go &
DEMO_PID=$!
cd ..

# Start frontend
echo -e "${YELLOW}Starting Frontend...${NC}"
cd frontend
if [ ! -d "node_modules" ]; then
    echo "Installing frontend dependencies..."
    npm install --legacy-peer-deps
fi
npm start &
FRONTEND_PID=$!
cd ..

echo -e "${GREEN}Dashboard started successfully!${NC}"
echo "Access the dashboard at http://localhost:3000"
echo ""
echo "Press Ctrl+C to stop all services"

# Function to cleanup on exit
cleanup() {
    echo -e "${YELLOW}Stopping services...${NC}"
    kill $SERVER_PID 2>/dev/null
    kill $DEMO_PID 2>/dev/null
    kill $FRONTEND_PID 2>/dev/null
    echo -e "${GREEN}All services stopped${NC}"
}

# Set trap to cleanup on Ctrl+C
trap cleanup EXIT

# Wait for user to press Ctrl+C
wait