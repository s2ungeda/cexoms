#!/bin/bash

echo "Setting up mExOms Dashboard..."

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    echo "Docker is not installed. Please install Docker first."
    exit 1
fi

# Check if Docker Compose is installed
if ! command -v docker-compose &> /dev/null; then
    echo "Docker Compose is not installed. Please install Docker Compose first."
    exit 1
fi

# Create necessary directories
mkdir -p frontend/public
mkdir -p server
mkdir -p demo

# Initialize Go modules for server
cd server
if [ ! -f "go.mod" ]; then
    go mod init github.com/your-org/mExOms/dashboard/server
    go get github.com/gorilla/websocket
    go get github.com/nats-io/nats.go
    go get github.com/prometheus/client_golang/prometheus
    go get github.com/prometheus/client_golang/prometheus/promhttp
    go get go.uber.org/zap
fi
cd ..

# Initialize Go modules for demo
cd demo
if [ ! -f "go.mod" ]; then
    go mod init github.com/your-org/mExOms/dashboard/demo
    go get github.com/nats-io/nats.go
fi
cd ..

# Install frontend dependencies
cd frontend
npm install
cd ..

echo "Setup complete!"
echo ""
echo "To start the dashboard:"
echo "1. Development mode:"
echo "   - Terminal 1: cd dashboard/server && go run main.go"
echo "   - Terminal 2: cd dashboard/frontend && npm start"
echo "   - Terminal 3: cd dashboard/demo && go run data_generator.go"
echo ""
echo "2. Docker mode:"
echo "   docker-compose up"
echo ""
echo "Access the dashboard at http://localhost:3000"