# gRPC API Gateway

The gRPC API Gateway provides external access to the OMS system with multi-account support, authentication, and high-performance RPC communication.

## Features

- **Multi-Account Support**: Manage multiple trading accounts across exchanges
- **Authentication**: API key-based authentication with rate limiting
- **High Performance**: gRPC for low-latency communication
- **Service Reflection**: Built-in service discovery for development
- **TLS Support**: Secure communication (optional)

## Services

### 1. AccountService
Manages trading accounts and inter-account operations.

- `CreateAccount`: Create new trading account
- `GetAccount`: Get account details
- `ListAccounts`: List accounts with filters
- `GetAccountBalance`: Get account balance
- `GetAccountPositions`: Get account positions
- `Transfer`: Transfer assets between accounts
- `GetTransferHistory`: Get transfer history
- `GetAccountMetrics`: Get performance metrics
- `SelectAccount`: Auto-select account for strategy

### 2. OrderService
Handles order operations across accounts.

- `CreateOrder`: Submit new order
- `CancelOrder`: Cancel existing order
- `GetOrder`: Get order details
- `ListOrders`: List orders with filters

### 3. PositionService
Manages position tracking and risk metrics.

- `GetPosition`: Get specific position
- `ListPositions`: List all positions
- `GetAggregatedPositions`: Get positions across exchanges
- `GetRiskMetrics`: Get risk metrics

### 4. MarketDataService
Provides market data access.

- `GetOrderBook`: Get current orderbook
- `GetTicker`: Get ticker data
- `GetRecentTrades`: Get recent trades
- `GetKlines`: Get historical klines
- `Subscribe`: Real-time market data stream

### 5. AuthService
Handles authentication and API key management.

- `Authenticate`: Get authentication token
- `RefreshToken`: Refresh token
- `CreateAPIKey`: Create new API key
- `ListAPIKeys`: List API keys
- `RevokeAPIKey`: Revoke API key

## Usage

### Starting the Server

```go
import (
    "github.com/mExOms/internal/api/grpc"
)

// Configure server
config := &grpc.Config{
    Port:             50051,
    EnableReflection: true,
    EnableAuth:       true,
}

// Create server with dependencies
server := grpc.NewServer(
    config,
    accountManager,
    orderManager,
    positionManager,
    transferManager,
)

// Start server
ctx := context.Background()
if err := server.Start(ctx); err != nil {
    log.Fatal(err)
}
```

### Client Example

```go
// Connect to server
conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
if err != nil {
    log.Fatal(err)
}
defer conn.Close()

// Create service client
accountClient := pb.NewAccountServiceClient(conn)

// Add authentication
ctx := metadata.AppendToOutgoingContext(
    context.Background(),
    "x-api-key", "your-api-key",
)

// List accounts
resp, err := accountClient.ListAccounts(ctx, &pb.ListAccountsRequest{
    ActiveOnly: true,
})
```

## Protocol Buffers

The API is defined using Protocol Buffers v3. Key proto files:

- `proto/oms/v1/service.proto`: Service definitions
- `proto/oms/v1/account.proto`: Account types and messages
- `proto/oms/v1/order.proto`: Order types and messages
- `proto/oms/v1/position.proto`: Position types and messages
- `proto/oms/v1/common.proto`: Common enums and types

Generate Go code:
```bash
make proto
```

## Authentication

The API uses API key authentication. Include the API key in request metadata:

```
x-api-key: your-api-key-here
```

Or in the Authorization header:
```
Authorization: Bearer your-api-key-here
```

## Rate Limiting

Rate limits are applied per API key:
- 1000 requests per minute for read operations
- 100 requests per minute for write operations
- 10 requests per minute for account operations

## Error Handling

The API uses standard gRPC status codes:
- `OK`: Success
- `INVALID_ARGUMENT`: Invalid request parameters
- `NOT_FOUND`: Resource not found
- `ALREADY_EXISTS`: Resource already exists
- `PERMISSION_DENIED`: Insufficient permissions
- `UNAUTHENTICATED`: Missing or invalid authentication
- `RESOURCE_EXHAUSTED`: Rate limit exceeded
- `INTERNAL`: Internal server error

## Development

### Using grpcurl

Test the API using grpcurl:

```bash
# List services
grpcurl -plaintext localhost:50051 list

# Describe service
grpcurl -plaintext localhost:50051 describe oms.v1.AccountService

# Call method
grpcurl -plaintext \
  -H "x-api-key: test-key" \
  -d '{"active_only": true}' \
  localhost:50051 oms.v1.AccountService/ListAccounts
```

### Using BloomRPC

BloomRPC provides a GUI for testing gRPC APIs:
1. Import proto files
2. Set server address: `localhost:50051`
3. Add metadata: `x-api-key: your-key`
4. Make requests

## Performance

- Latency: < 1ms for most operations
- Throughput: 10,000+ requests/second
- Connection pooling supported
- HTTP/2 multiplexing
- Protobuf binary encoding

## Security

- TLS encryption (optional)
- API key authentication
- Rate limiting per key
- Request validation
- Audit logging

## Monitoring

The server exposes metrics:
- Request count by method
- Request latency histogram
- Error rate by status code
- Active connection count

Access metrics at: `http://localhost:9090/metrics`