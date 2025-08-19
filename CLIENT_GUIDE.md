# OMS Client Guide

Multi-Exchange OMS provides client libraries in multiple languages to interact with the gRPC server.

## Available Clients

1. **Go CLI Client** - Command-line interface
2. **Python SDK** - Python library and CLI
3. **JavaScript/Node.js SDK** - Node.js library

## Go CLI Client

### Installation

```bash
# Build the client
go build -o bin/oms-client cmd/oms-client/main.go
```

### Usage Examples

#### Place Orders

```bash
# Place a limit order
./bin/oms-client place -symbol BTCUSDT -side BUY -quantity 0.001 -price 115000

# Place a market order
./bin/oms-client place -symbol ETHUSDT -side SELL -type MARKET -quantity 0.1

# Place a futures order
./bin/oms-client place -symbol BTCUSDT -side BUY -quantity 0.01 -price 115000 -market futures -leverage 10
```

#### Manage Orders

```bash
# Cancel an order
./bin/oms-client cancel -id order_123456

# Get order details
./bin/oms-client get-order -id order_123456

# List all orders
./bin/oms-client list-orders

# List open orders for a symbol
./bin/oms-client list-orders -status OPEN -symbol BTCUSDT
```

#### Account Information

```bash
# Get spot balance
./bin/oms-client balance -exchange binance -market spot

# Get futures balance
./bin/oms-client balance -exchange binance -market futures

# Get open positions
./bin/oms-client positions -exchange binance
```

#### Real-time Streaming

```bash
# Stream real-time prices
./bin/oms-client stream-prices

# Stream order updates
./bin/oms-client stream-orders
```

## Python SDK

### Installation

```bash
# Install dependencies
pip install grpcio grpcio-tools

# Generate proto files
python -m grpc_tools.protoc -I./proto --python_out=./proto --grpc_python_out=./proto ./proto/oms.proto
```

### Usage Examples

```python
from clients.python.oms_client import OMSClient

# Create client
client = OMSClient("localhost:50051")

# Place an order
result = client.place_order(
    symbol="BTCUSDT",
    side="BUY",
    quantity=0.001,
    order_type="LIMIT",
    price=115000
)
print(f"Order placed: {result['order_id']}")

# Get balance
balance = client.get_balance(exchange="binance", market="spot")
for asset, bal in balance.items():
    if bal['total'] > 0:
        print(f"{asset}: {bal['total']:.8f}")

# Stream prices
def on_price_update(update):
    print(f"{update['symbol']}: ${update['last_price']:.2f}")

client.stream_prices(["BTCUSDT", "ETHUSDT"], on_price_update)
```

### CLI Usage

```bash
# Place order
python clients/python/oms_client.py place --symbol BTCUSDT --side BUY --quantity 0.001 --price 115000

# Get balance
python clients/python/oms_client.py balance --exchange binance --market spot

# Stream prices
python clients/python/oms_client.py stream --symbols BTCUSDT ETHUSDT XRPUSDT
```

## JavaScript/Node.js SDK

### Installation

```bash
# Install dependencies
cd clients/javascript
npm init -y
npm install @grpc/grpc-js @grpc/proto-loader
```

### Usage Examples

```javascript
const OMSClient = require('./oms-client');

// Create client
const client = new OMSClient('localhost:50051');

// Place an order
async function placeOrder() {
  try {
    const result = await client.placeOrder({
      symbol: 'BTCUSDT',
      side: 'BUY',
      quantity: 0.001,
      price: 115000,
      orderType: 'LIMIT'
    });
    console.log('Order placed:', result.orderId);
  } catch (error) {
    console.error('Error:', error);
  }
}

// Get balance
async function getBalance() {
  const balance = await client.getBalance();
  console.log('Balance:', balance);
}

// Stream prices
function streamPrices() {
  client.streamPrices(['BTCUSDT', 'ETHUSDT'], (update) => {
    console.log(`${update.symbol}: $${update.lastPrice.toFixed(2)}`);
  });
}
```

## API Reference

### Order Management

#### PlaceOrder
- **Parameters**: symbol, side, orderType, quantity, price, exchange, market, accountId
- **Returns**: orderId, exchangeOrderId, status, createdAt

#### CancelOrder
- **Parameters**: orderId
- **Returns**: orderId, status, cancelledAt

#### GetOrder
- **Parameters**: orderId
- **Returns**: Complete order details

#### ListOrders
- **Parameters**: status (optional), symbol (optional)
- **Returns**: Array of orders

### Account Information

#### GetBalance
- **Parameters**: exchange, market, accountId
- **Returns**: Map of asset balances

#### GetPositions
- **Parameters**: exchange, accountId
- **Returns**: Array of open positions

### Real-time Streaming

#### StreamPrices
- **Parameters**: symbols (array)
- **Returns**: Stream of price updates

#### StreamOrders
- **Parameters**: None
- **Returns**: Stream of order updates

## Error Handling

All clients handle gRPC errors with appropriate error messages:

```python
# Python example
try:
    result = client.place_order(...)
except grpc.RpcError as e:
    print(f"Error: {e.details()}")
    print(f"Status code: {e.code()}")
```

```javascript
// JavaScript example
client.placeOrder(params)
  .then(result => console.log(result))
  .catch(error => console.error('Error:', error.message));
```

## Best Practices

1. **Connection Management**
   - Reuse client connections
   - Implement proper cleanup on exit
   - Handle connection errors gracefully

2. **Streaming**
   - Use streaming for real-time data
   - Implement reconnection logic
   - Handle stream errors

3. **Order Management**
   - Always check order status after placement
   - Implement idempotency for critical operations
   - Use appropriate order types for your strategy

4. **Rate Limiting**
   - Respect exchange rate limits
   - Implement client-side rate limiting
   - Use bulk operations where available

## Testing

### Test Order Placement (Testnet)

```bash
# Set testnet credentials
export BINANCE_TESTNET_API_KEY="your_testnet_key"
export BINANCE_TESTNET_SECRET="your_testnet_secret"

# Place test order
./bin/oms-client place -symbol BTCUSDT -side BUY -quantity 0.001 -price 100000
```

### Integration Testing

```python
# Python test example
import unittest
from oms_client import OMSClient

class TestOMSClient(unittest.TestCase):
    def setUp(self):
        self.client = OMSClient("localhost:50051")
    
    def test_place_order(self):
        result = self.client.place_order(
            symbol="BTCUSDT",
            side="BUY",
            quantity=0.001,
            price=100000
        )
        self.assertIsNotNone(result['order_id'])
        self.assertEqual(result['status'], 'NEW')
```

## Troubleshooting

### Connection Issues
- Verify server is running: `ps aux | grep oms-server`
- Check server address and port
- Ensure firewall allows gRPC port (50051)

### Order Failures
- Check API credentials
- Verify symbol format (e.g., BTCUSDT not BTC/USDT)
- Ensure sufficient balance
- Check minimum order size requirements

### Streaming Issues
- Implement reconnection logic
- Handle network interruptions
- Monitor memory usage for long-running streams