# OrderService API Reference

The OrderService handles all order-related operations including creation, cancellation, and status queries.

## Overview

- **Service**: `oms.v1.OrderService`
- **Base URL**: `grpc://localhost:8080` (development)
- **Authentication**: Required (JWT token or API key)
- **Rate Limit**: 1000 requests/minute per account

## Methods

### CreateOrder

Creates a new order for execution.

**Request**: `OrderRequest`
```protobuf
message OrderRequest {
  Order order = 1;
  string account_id = 2;  // Optional: specific account
  string strategy_id = 3; // Optional: strategy association
}

message Order {
  string symbol = 1;        // Trading pair (e.g., "BTCUSDT")
  OrderSide side = 2;       // BUY or SELL
  OrderType type = 3;       // MARKET, LIMIT, STOP_LOSS, etc.
  string quantity = 4;      // Order quantity (decimal string)
  string price = 5;         // Price (for limit orders)
  string stop_price = 6;    // Stop price (for stop orders)
  TimeInForce time_in_force = 7; // GTC, IOC, FOK
  bool reduce_only = 8;     // Futures only: reduce position
  bool post_only = 9;       // Maker-only orders
}
```

**Response**: `OrderResponse`
```protobuf
message OrderResponse {
  Order order = 1;
  string order_id = 2;
  OrderStatus status = 3;
  string exchange = 4;
  string account_id = 5;
  int64 created_at = 6;
  string error_message = 7;
}
```

**Example**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "order": {
      "symbol": "BTCUSDT",
      "side": "BUY",
      "type": "LIMIT", 
      "quantity": "0.01",
      "price": "45000",
      "time_in_force": "GTC"
    },
    "account_id": "main-account"
  }' \
  localhost:8080 oms.v1.OrderService/CreateOrder
```

**Performance**: 
- Latency: <100μs
- Success Rate: >99.9%

### CancelOrder

Cancels an existing order.

**Request**: `CancelOrderRequest`
```protobuf
message CancelOrderRequest {
  string order_id = 1;      // Required: order ID to cancel
  string symbol = 2;        // Optional: for verification
  string account_id = 3;    // Optional: specific account
}
```

**Response**: `OrderResponse`

**Example**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{"order_id": "order-123456"}' \
  localhost:8080 oms.v1.OrderService/CancelOrder
```

### GetOrder

Retrieves details for a specific order.

**Request**: `GetOrderRequest`
```protobuf
message GetOrderRequest {
  string order_id = 1;      // Required: order ID
  string account_id = 2;    // Optional: specific account
}
```

**Response**: `OrderResponse`

**Example**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{"order_id": "order-123456"}' \
  localhost:8080 oms.v1.OrderService/GetOrder
```

### ListOrders

Lists orders with optional filtering.

**Request**: `ListOrdersRequest`
```protobuf
message ListOrdersRequest {
  string account_id = 1;    // Optional: filter by account
  string symbol = 2;        // Optional: filter by symbol
  OrderStatus status = 3;   // Optional: filter by status
  int64 start_time = 4;     // Optional: start timestamp
  int64 end_time = 5;       // Optional: end timestamp
  int32 limit = 6;          // Optional: max results (default 100)
  string page_token = 7;    // Optional: pagination token
}
```

**Response**: `ListOrdersResponse`
```protobuf
message ListOrdersResponse {
  repeated OrderResponse orders = 1;
  string next_page_token = 2;
  int32 total_count = 3;
}
```

**Example**:
```bash
# Get all active BTCUSDT orders
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "symbol": "BTCUSDT",
    "status": "PENDING",
    "limit": 50
  }' \
  localhost:8080 oms.v1.OrderService/ListOrders
```

## Order Types

| Type | Description | Required Fields |
|------|-------------|-----------------|
| MARKET | Execute immediately at market price | symbol, side, quantity |
| LIMIT | Execute at specific price or better | symbol, side, quantity, price |
| STOP_LOSS | Market order triggered by stop price | symbol, side, quantity, stop_price |
| STOP_LIMIT | Limit order triggered by stop price | symbol, side, quantity, price, stop_price |
| TAKE_PROFIT | Profit-taking order | symbol, side, quantity, stop_price |

## Order Status Flow

```
NEW → PENDING → PARTIALLY_FILLED → FILLED
                    ↓
               CANCELLED/REJECTED/EXPIRED
```

| Status | Description |
|--------|-------------|
| NEW | Order created but not sent to exchange |
| PENDING | Order sent to exchange, waiting for fill |
| PARTIALLY_FILLED | Order partially executed |
| FILLED | Order completely executed |
| CANCELLED | Order cancelled by user |
| REJECTED | Order rejected by exchange |
| EXPIRED | Order expired (time-based orders) |

## Error Handling

Common error scenarios:

### Insufficient Balance
```json
{
  "code": 3,
  "message": "INVALID_ARGUMENT: Insufficient balance for order",
  "details": [{
    "type": "InsufficientBalanceError",
    "available": "100.00",
    "required": "150.00",
    "asset": "USDT"
  }]
}
```

### Invalid Symbol
```json
{
  "code": 3,
  "message": "INVALID_ARGUMENT: Unknown symbol INVALID",
  "details": [{
    "type": "InvalidSymbolError",
    "supported_symbols": ["BTCUSDT", "ETHUSDT", "..."]
  }]
}
```

### Rate Limit Exceeded
```json
{
  "code": 8,
  "message": "RESOURCE_EXHAUSTED: Rate limit exceeded",
  "details": [{
    "type": "RateLimitError",
    "retry_after": 60,
    "limit": 1000,
    "remaining": 0
  }]
}
```

## Best Practices

### 1. Order Size Validation
```go
// Always validate order size against exchange limits
if quantity < minQuantity || quantity > maxQuantity {
    return errors.New("quantity out of range")
}
```

### 2. Price Precision
```go
// Round prices to exchange precision
price = roundToPrecision(price, symbolInfo.PricePrecision)
```

### 3. Risk Management
```go
// Check position limits before creating orders
if newPosition > maxPosition {
    return errors.New("position limit exceeded")
}
```

### 4. Error Handling
```go
// Handle retryable errors
if isRetryable(err) && retries < maxRetries {
    time.Sleep(exponentialBackoff(retries))
    return retry(request)
}
```

## WebSocket Updates

Subscribe to real-time order updates via MarketDataService:

```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{"subscription_type": "ORDER_UPDATES", "account_id": "main"}' \
  localhost:8080 oms.v1.MarketDataService/Subscribe
```

Order updates include:
- Status changes
- Partial fills
- Final execution details
- Cancellation confirmations

## Supported Exchanges

| Exchange | Spot | Futures | Options |
|----------|------|---------|---------|
| Binance | ✅ | ✅ | ❌ |
| Bybit | ⏳ | ⏳ | ❌ |
| OKX | ⏳ | ⏳ | ❌ |
| Upbit | ⏳ | ❌ | ❌ |

## Related Services

- [PositionService](./position-service.md) - Monitor resulting positions
- [AccountService](./account-service.md) - Check account balances
- [MarketDataService](./market-data-service.md) - Real-time order updates