# mExOms API Documentation

Multi-Exchange Order Management System (mExOms) provides comprehensive gRPC and REST APIs for high-frequency cryptocurrency trading across multiple exchanges.

## Overview

mExOms offers six core services:

- **[OrderService](./order-service.md)** - Order management and execution
- **[PositionService](./position-service.md)** - Position tracking and risk metrics  
- **[MarketDataService](./market-data-service.md)** - Real-time and historical market data
- **[AuthService](./auth-service.md)** - Authentication and API key management
- **[AccountService](./account-service.md)** - Multi-account operations and transfers
- **[StrategyService](./strategy-service.md)** - Strategy deployment and management

## Quick Start

### Authentication

All API calls require authentication via JWT token or API key:

```bash
# Get JWT token
grpcurl -plaintext -d '{"username":"trader", "password":"secret"}' \
  localhost:8080 oms.v1.AuthService/Authenticate

# Use token in subsequent calls
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  localhost:8080 oms.v1.OrderService/ListOrders
```

### Basic Order Flow

```bash
# 1. Create an order
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{"order": {"symbol": "BTCUSDT", "side": "BUY", "quantity": "1.0", "price": "50000"}}' \
  localhost:8080 oms.v1.OrderService/CreateOrder

# 2. Check order status
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{"order_id": "order-123"}' \
  localhost:8080 oms.v1.OrderService/GetOrder

# 3. Cancel if needed
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{"order_id": "order-123"}' \
  localhost:8080 oms.v1.OrderService/CancelOrder
```

## Protocol Specifications

- **gRPC Protocol**: HTTP/2 with protobuf serialization
- **TLS**: TLS 1.3 required for production
- **Authentication**: JWT tokens or API keys
- **Rate Limiting**: Per-account limits apply
- **Error Handling**: Standard gRPC status codes

## Performance Characteristics

- **Latency**: <100μs order processing
- **Throughput**: 100,000+ orders/second
- **Availability**: 99.9%+ uptime
- **Data Freshness**: Real-time WebSocket streams

## Error Codes

Common gRPC status codes:

| Code | Status | Description |
|------|--------|-------------|
| 0 | OK | Success |
| 3 | INVALID_ARGUMENT | Invalid request parameters |
| 7 | PERMISSION_DENIED | Authentication required |
| 8 | RESOURCE_EXHAUSTED | Rate limit exceeded |
| 14 | UNAVAILABLE | Service temporarily unavailable |

## SDK Support

Official SDKs available for:

- Go: `github.com/mExOms/go-client`
- Python: `pip install mexoms-client`
- JavaScript: `npm install @mexoms/client`
- Java: Maven coordinates in documentation

## Environment URLs

| Environment | gRPC | REST Gateway |
|-------------|------|--------------|
| Development | localhost:8080 | http://localhost:8081 |
| Staging | grpc.staging.mexoms.com:443 | https://api.staging.mexoms.com |
| Production | grpc.mexoms.com:443 | https://api.mexoms.com |

## Next Steps

1. Review [Authentication Guide](./auth-service.md) for API access
2. Explore [Order Service](./order-service.md) for trading operations
3. Check [Market Data Service](./market-data-service.md) for real-time data
4. See [Examples](../examples/) for practical implementations

For support, contact: api-support@mexoms.com