#!/bin/bash

echo "Starting OMS Monitoring System..."

# Kill any existing monitoring process
pkill -f test-monitoring-simple 2>/dev/null
pkill -f test-monitoring 2>/dev/null

# Wait a moment
sleep 1

# Run the monitoring system
cd /home/seunge/project/mExOms
go run cmd/test-monitoring/main.go