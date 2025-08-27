# Getting Started with mExOms

## Objective

Get the mExOms system up and running on your local machine and execute your first trade.

## Prerequisites

- Go 1.23+ installed
- C++20 compatible compiler (g++ 10+ or clang++ 11+)
- Docker and Docker Compose
- Git
- Make
- At least 8GB RAM
- Exchange API keys (we'll use Binance testnet for this tutorial)

## Step 1: Clone the Repository

```bash
git clone https://github.com/your-org/mExOms.git
cd mExOms
```

## Step 2: Install Dependencies

```bash
# Install system dependencies
make install-deps

# Generate protobuf files
make proto

# Build the project
make build
```

## Step 3: Start Infrastructure Services

```bash
# Start all required services (PostgreSQL, Redis, NATS, Vault)
docker-compose up -d

# Verify services are running
docker-compose ps
```

## Step 4: Configure Exchange API Keys

### Option A: Using Vault (Recommended)

```bash
# Initialize Vault
./scripts/setup_vault.sh

# Add Binance testnet API keys
vault kv put secret/exchanges/binance_spot \
    api_key="your-testnet-api-key" \
    api_secret="your-testnet-api-secret"
```

### Option B: Using Environment Variables

```bash
# Copy example environment file
cp .env.example .env

# Edit .env file and add your API keys
nano .env
```

## Step 5: Configure the System

```bash
# Copy example configuration
cp configs/config.example.yaml configs/config.yaml

# Edit configuration
nano configs/config.yaml
```

Key configuration sections:

```yaml
exchanges:
  binance:
    spot:
      enabled: true
      testnet: true  # Use testnet for tutorials
      websocket_endpoints:
        - wss://testnet.binance.vision/ws
      
risk:
  max_position_size_usd: 1000
  max_order_value_usd: 100
  max_open_orders: 10
```

## Step 6: Start the OMS Monitor

```bash
# Start the monitoring service
go run cmd/monitor/main.go
```

Open your browser at http://localhost:8080 to see the monitoring dashboard.

## Step 7: Start Exchange Connectors

In a new terminal:

```bash
# Start Binance Spot connector
go run cmd/binance-spot/main.go -config configs/config.yaml
```

## Step 8: Execute Your First Trade

### Using the gRPC Client

```bash
# Run the example trading client
go run examples/grpc-client/main.go
```

### Using Code

Create a file `first_trade.go`:

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    pb "github.com/your-org/mExOms/proto/gen/go/oms/v1"
    "google.golang.org/grpc"
)

func main() {
    // Connect to OMS gRPC server
    conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer conn.Close()
    
    client := pb.NewOrderServiceClient(conn)
    
    // Create a market order
    order := &pb.CreateOrderRequest{
        Symbol:       "BTCUSDT",
        Exchange:     "binance",
        Market:       "spot",
        Type:         pb.OrderType_MARKET,
        Side:         pb.OrderSide_BUY,
        Quantity:     0.001,
        TimeInForce:  pb.TimeInForce_IOC,
    }
    
    resp, err := client.CreateOrder(context.Background(), order)
    if err != nil {
        log.Fatalf("Failed to create order: %v", err)
    }
    
    fmt.Printf("Order created successfully!\n")
    fmt.Printf("Order ID: %s\n", resp.OrderId)
    fmt.Printf("Status: %s\n", resp.Status)
}
```

Run it:

```bash
go run first_trade.go
```

## Step 9: Monitor Your Trade

1. Check the monitoring dashboard at http://localhost:8080
2. View order status in the Orders section
3. Check your position in the Positions tab

## Step 10: View System Metrics

```bash
# Check system performance
curl http://localhost:9090/metrics | grep oms_

# View logs
docker-compose logs -f oms-monitor
```

## Troubleshooting

### Common Issues

1. **Connection refused errors**
   - Ensure all Docker services are running: `docker-compose ps`
   - Check service logs: `docker-compose logs [service-name]`

2. **API key errors**
   - Verify API keys are correctly set in Vault or .env
   - Ensure you're using testnet keys for testnet configuration

3. **Build errors**
   - Check Go version: `go version` (should be 1.23+)
   - Verify C++ compiler: `g++ --version` (should be 10+)
   - Run `make clean && make build`

4. **Order rejection**
   - Check risk limits in config.yaml
   - Verify account balance on exchange
   - Review error logs for specific rejection reason

### Getting Help

- Check logs: `tail -f logs/*.log`
- Monitor service health: http://localhost:8080/health
- Review error messages in monitoring dashboard

## Next Steps

Congratulations! You've successfully:
- Set up the mExOms system
- Connected to an exchange
- Executed your first trade
- Monitored system performance

Next tutorials to explore:
- [Basic Trading Tutorial](./basic-trading.md) - Learn different order types
- [Multi-Account Trading](./multi-account-trading.md) - Trade across multiple accounts
- [WebSocket Streaming](./websocket-streaming.md) - Real-time data processing

## Additional Resources

- [API Documentation](/docs/api/)
- [System Architecture](/docs/architecture/)
- [Risk Management Guide](/docs/risk-management/)
- [Performance Tuning](/docs/performance/)