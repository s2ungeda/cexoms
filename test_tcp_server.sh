#!/bin/bash

# Test script for TCP Server

echo "=== mExOms TCP Server Test ==="

# Check if binaries exist
if [ ! -f "bin/tcp-server" ] || [ ! -f "bin/tcp-client-example" ]; then
    echo "Error: Binaries not found. Please run ./build_tcp_server.sh first"
    exit 1
fi

# Start server in background
echo "Starting TCP server on port 9090..."
./bin/tcp-server 9090 &
SERVER_PID=$!

# Wait for server to start
sleep 2

# Check if server is running
if ! ps -p $SERVER_PID > /dev/null; then
    echo "Error: Server failed to start"
    exit 1
fi

echo "Server started with PID: $SERVER_PID"

# Run client test
echo ""
echo "Starting test client..."
./bin/tcp-client-example localhost 9090

# Kill server
echo ""
echo "Stopping server..."
kill $SERVER_PID
wait $SERVER_PID 2>/dev/null

echo "Test completed!"