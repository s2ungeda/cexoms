# gRPC API Gateway

The gRPC API Gateway provides a high-performance, secure interface for external clients to interact with the OMS.

## Features

- **Authentication**: JWT-based authentication with API key support
- **Authorization**: Fine-grained permission control
- **Rate Limiting**: Per-user rate limiting with configurable limits
- **TLS 1.3**: Secure communication with latest TLS version
- **Service Discovery**: Automatic service registration
- **Monitoring**: Built-in metrics and health checks

## Quick Start

### 1. Generate Proto Files
```bash
make proto
```

### 2. Start the Gateway
```bash
# Without TLS (development)
./bin/grpc-gateway

# With TLS (production)
./bin/grpc-gateway -enable-tls -tls-cert cert.pem -tls-key key.pem
```

### 3. Test Connection
```bash
# List available services
grpcurl -plaintext localhost:9090 list

# Describe a service
grpcurl -plaintext localhost:9090 describe oms.v1.OrderService
```

## Authentication

### API Key Authentication

1. Create an API key:
```go
resp, err := authClient.CreateAPIKey(ctx, &omsv1.CreateAPIKeyRequest{
    Name: "My Trading Bot",
    Permissions: []omsv1.Permission{
        omsv1.Permission_PERMISSION_READ_ORDERS,
        omsv1.Permission_PERMISSION_WRITE_ORDERS,
    },
})
```

2. Authenticate:
```go
authResp, err := authClient.Authenticate(ctx, &omsv1.AuthRequest{
    ApiKey: resp.ApiKey.Id,
    Secret: resp.Secret,
})
```

3. Use the token:
```go
ctx := metadata.AppendToOutgoingContext(context.Background(),
    "authorization", fmt.Sprintf("Bearer %s", authResp.Token))
```

### Permissions

- `PERMISSION_READ_ORDERS`: Read order information
- `PERMISSION_WRITE_ORDERS`: Create/cancel orders
- `PERMISSION_READ_POSITIONS`: Read position data
- `PERMISSION_READ_MARKET_DATA`: Access market data
- `PERMISSION_ADMIN`: Full administrative access

## API Services

### OrderService

Create, cancel, and query orders across all connected exchanges.

```proto
service OrderService {
    rpc CreateOrder(OrderRequest) returns (OrderResponse);
    rpc CancelOrder(CancelOrderRequest) returns (OrderResponse);
    rpc GetOrder(GetOrderRequest) returns (OrderResponse);
    rpc ListOrders(ListOrdersRequest) returns (ListOrdersResponse);
}
```

### PositionService

Monitor positions and risk metrics across exchanges.

```proto
service PositionService {
    rpc GetPosition(GetPositionRequest) returns (GetPositionResponse);
    rpc ListPositions(ListPositionsRequest) returns (ListPositionsResponse);
    rpc GetAggregatedPositions(GetAggregatedPositionsRequest) returns (GetAggregatedPositionsResponse);
    rpc GetRiskMetrics(GetRiskMetricsRequest) returns (GetRiskMetricsResponse);
}
```

### MarketDataService (Coming Soon)

Real-time market data streaming.

```proto
service MarketDataService {
    rpc GetOrderBook(GetOrderBookRequest) returns (OrderBook);
    rpc GetTicker(GetTickerRequest) returns (Ticker);
    rpc Subscribe(SubscribeRequest) returns (stream MarketDataUpdate);
}
```

## Rate Limiting

Default limits:
- 100 requests/second per user
- Burst of 200 requests

Configure custom limits:
```bash
./bin/grpc-gateway -rate-limit 500 -burst-limit 1000
```

## TLS Configuration

The gateway supports TLS 1.3 with secure cipher suites:
- TLS_AES_128_GCM_SHA256
- TLS_AES_256_GCM_SHA384
- TLS_CHACHA20_POLY1305_SHA256

Generate certificates:
```bash
# Self-signed for development
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -nodes

# Production certificates from Let's Encrypt
certbot certonly --standalone -d api.youromain.com
```

## Client Examples

See `/cmd/grpc-client/main.go` for a complete client example.

### Python Client
```python
import grpc
from oms.v1 import auth_pb2, auth_pb2_grpc

# Connect
channel = grpc.insecure_channel('localhost:9090')
auth_stub = auth_pb2_grpc.AuthServiceStub(channel)

# Authenticate
response = auth_stub.Authenticate(auth_pb2.AuthRequest(
    api_key="demo-api-key",
    secret="demo-secret"
))
print(f"Token: {response.token}")
```

### Node.js Client
```javascript
const grpc = require('@grpc/grpc-js');
const protoLoader = require('@grpc/proto-loader');

// Load proto
const packageDefinition = protoLoader.loadSync('proto/oms/v1/service.proto');
const oms = grpc.loadPackageDefinition(packageDefinition).oms.v1;

// Connect
const client = new oms.AuthService('localhost:9090', 
    grpc.credentials.createInsecure());

// Authenticate
client.authenticate({
    api_key: 'demo-api-key',
    secret: 'demo-secret'
}, (err, response) => {
    console.log('Token:', response.token);
});
```

## Performance

- Authentication: < 1ms
- Order placement: < 5ms (excluding exchange latency)
- Position queries: < 1ms
- Supports 10,000+ concurrent connections
- 100,000+ requests/second throughput

## Monitoring

Health check endpoint:
```bash
grpcurl -plaintext localhost:9090 grpc.health.v1.Health/Check
```

Metrics available via gRPC reflection and Prometheus endpoint (coming soon).

## Security Best Practices

1. **Always use TLS in production**
2. **Rotate API secrets regularly**
3. **Implement IP whitelisting for sensitive operations**
4. **Monitor for unusual activity patterns**
5. **Use separate API keys for different applications**
6. **Never expose API secrets in client-side code**

## Troubleshooting

### Connection Refused
- Check if the gateway is running: `ps aux | grep grpc-gateway`
- Verify the port is correct: `netstat -tlnp | grep 9090`

### Authentication Failed
- Verify API key and secret are correct
- Check if API key is active
- Ensure token hasn't expired

### Rate Limit Exceeded
- Implement exponential backoff
- Use batch operations where possible
- Request rate limit increase if needed

### TLS Handshake Failed
- Verify certificate validity
- Check TLS version compatibility
- Ensure cipher suites match