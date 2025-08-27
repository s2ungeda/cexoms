# MarketDataService API Reference

The MarketDataService provides real-time and historical market data from multiple cryptocurrency exchanges.

## Overview

- **Service**: `oms.v1.MarketDataService`
- **Base URL**: `grpc://localhost:8080` (development)
- **Authentication**: Required for some endpoints
- **Data Sources**: Live exchange WebSocket feeds
- **Update Frequency**: Real-time (sub-second latency)

## Methods

### GetOrderBook

Retrieves current order book (bids/asks) for a symbol.

**Request**: `GetOrderBookRequest`
```protobuf
message GetOrderBookRequest {
  string symbol = 1;         // Trading pair (e.g., "BTCUSDT")
  string exchange = 2;       // Optional: specific exchange
  int32 depth = 3;          // Optional: number of levels (default 20)
}
```

**Response**: `OrderBook`
```protobuf
message OrderBook {
  string symbol = 1;
  string exchange = 2;
  repeated OrderBookLevel bids = 3;   // Buy orders
  repeated OrderBookLevel asks = 4;   // Sell orders
  int64 timestamp = 5;               // Last update time
  int64 last_update_id = 6;          // Sequence ID
}

message OrderBookLevel {
  string price = 1;         // Price level
  string quantity = 2;      // Total quantity at level
  int32 count = 3;          // Number of orders (optional)
}
```

**Example**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "symbol": "BTCUSDT",
    "exchange": "binance",
    "depth": 10
  }' \
  localhost:8080 oms.v1.MarketDataService/GetOrderBook
```

**Response**:
```json
{
  "symbol": "BTCUSDT",
  "exchange": "binance",
  "bids": [
    {"price": "45000.00", "quantity": "1.5"},
    {"price": "44999.50", "quantity": "2.1"},
    {"price": "44999.00", "quantity": "0.8"}
  ],
  "asks": [
    {"price": "45000.50", "quantity": "1.2"},
    {"price": "45001.00", "quantity": "1.8"},
    {"price": "45001.50", "quantity": "0.9"}
  ],
  "timestamp": 1703894400000,
  "last_update_id": 12345678
}
```

### GetTicker

Retrieves 24-hour ticker statistics.

**Request**: `GetTickerRequest`
```protobuf
message GetTickerRequest {
  string symbol = 1;         // Trading pair
  string exchange = 2;       // Optional: specific exchange
}
```

**Response**: `Ticker`
```protobuf
message Ticker {
  string symbol = 1;
  string exchange = 2;
  string last_price = 3;     // Last trade price
  string bid_price = 4;      // Best bid price
  string ask_price = 5;      // Best ask price
  string high_price = 6;     // 24h high
  string low_price = 7;      // 24h low
  string volume = 8;         // 24h volume
  string quote_volume = 9;   // 24h quote volume
  string price_change = 10;  // 24h price change
  string price_change_percent = 11; // 24h change %
  int64 timestamp = 12;      // Update timestamp
  int32 count = 13;          // 24h trade count
}
```

**Example**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{"symbol": "BTCUSDT", "exchange": "binance"}' \
  localhost:8080 oms.v1.MarketDataService/GetTicker
```

### GetRecentTrades

Retrieves recent trade history.

**Request**: `GetRecentTradesRequest`
```protobuf
message GetRecentTradesRequest {
  string symbol = 1;         // Trading pair
  string exchange = 2;       // Optional: specific exchange
  int32 limit = 3;          // Optional: max trades (default 100)
}
```

**Response**: `GetRecentTradesResponse`
```protobuf
message GetRecentTradesResponse {
  repeated Trade trades = 1;
}

message Trade {
  string id = 1;            // Trade ID
  string price = 2;         // Trade price
  string quantity = 3;      // Trade quantity
  bool is_buyer_maker = 4;  // true if buyer is maker
  int64 timestamp = 5;      // Trade timestamp
}
```

**Example**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "symbol": "BTCUSDT",
    "exchange": "binance",
    "limit": 50
  }' \
  localhost:8080 oms.v1.MarketDataService/GetRecentTrades
```

### GetKlines

Retrieves historical candlestick data.

**Request**: `GetKlinesRequest`
```protobuf
message GetKlinesRequest {
  string symbol = 1;         // Trading pair
  string exchange = 2;       // Optional: specific exchange
  string interval = 3;       // Timeframe (1m, 5m, 1h, 1d, etc.)
  int64 start_time = 4;      // Optional: start timestamp
  int64 end_time = 5;        // Optional: end timestamp
  int32 limit = 6;          // Optional: max klines (default 500)
}
```

**Response**: `GetKlinesResponse`
```protobuf
message GetKlinesResponse {
  repeated Kline klines = 1;
}

message Kline {
  int64 open_time = 1;       // Candle open time
  string open = 2;           // Open price
  string high = 3;           // High price
  string low = 4;            // Low price
  string close = 5;          // Close price
  string volume = 6;         // Volume
  int64 close_time = 7;      // Candle close time
  string quote_volume = 8;   // Quote asset volume
  int32 trades = 9;          // Number of trades
  string taker_buy_volume = 10;      // Taker buy volume
  string taker_buy_quote_volume = 11; // Taker buy quote volume
}
```

**Example**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "symbol": "BTCUSDT",
    "exchange": "binance",
    "interval": "1h",
    "limit": 24
  }' \
  localhost:8080 oms.v1.MarketDataService/GetKlines
```

### Subscribe (Streaming)

Subscribes to real-time market data updates.

**Request**: `SubscribeRequest`
```protobuf
message SubscribeRequest {
  SubscriptionType subscription_type = 1;
  string symbol = 2;         // Optional: specific symbol
  string exchange = 3;       // Optional: specific exchange
  string account_id = 4;     // Optional: for account-specific data
  repeated string channels = 5; // Specific channels to subscribe
}

enum SubscriptionType {
  TICKER = 0;               // Ticker updates
  ORDER_BOOK = 1;           // Order book changes
  TRADES = 2;               // Trade stream
  KLINES = 3;               // Candlestick updates
  ORDER_UPDATES = 4;        // Order status changes
  POSITION_UPDATES = 5;     // Position changes
  BALANCE_UPDATES = 6;      // Balance changes
}
```

**Response Stream**: `MarketDataUpdate`
```protobuf
message MarketDataUpdate {
  string type = 1;          // Update type
  string symbol = 2;        // Symbol
  string exchange = 3;      // Exchange
  int64 timestamp = 4;      // Update timestamp
  
  oneof data {
    Ticker ticker = 5;
    OrderBook order_book = 6;
    Trade trade = 7;
    Kline kline = 8;
    OrderUpdate order_update = 9;
    PositionUpdate position_update = 10;
    BalanceUpdate balance_update = 11;
  }
}
```

**Example**:
```bash
# Subscribe to BTCUSDT ticker updates
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "subscription_type": "TICKER",
    "symbol": "BTCUSDT",
    "exchange": "binance"
  }' \
  localhost:8080 oms.v1.MarketDataService/Subscribe
```

**Example Stream Response**:
```json
{
  "type": "ticker",
  "symbol": "BTCUSDT",
  "exchange": "binance",
  "timestamp": 1703894400000,
  "ticker": {
    "symbol": "BTCUSDT",
    "last_price": "45123.45",
    "bid_price": "45123.00",
    "ask_price": "45123.90",
    "volume": "1234.56",
    "price_change_percent": "2.34"
  }
}
```

## Supported Exchanges

| Exchange | Spot | Futures | WebSocket | Historical Data |
|----------|------|---------|-----------|-----------------|
| Binance | ✅ | ✅ | ✅ | ✅ |
| Bybit | ⏳ | ⏳ | ⏳ | ⏳ |
| OKX | ⏳ | ⏳ | ⏳ | ⏳ |
| Upbit | ⏳ | ❌ | ⏳ | ⏳ |

## Data Quality

### Real-time Data
- **Source**: Direct exchange WebSocket feeds
- **Latency**: <50ms from exchange
- **Uptime**: 99.9%+
- **Data Integrity**: Checksums and sequence validation

### Historical Data
- **Storage**: Local time-series database
- **Retention**: 
  - Ticks: 30 days
  - 1-minute candles: 2 years
  - 1-hour candles: 5 years
  - Daily candles: Permanent
- **Backfill**: Automatic gap detection and filling

## Rate Limits

| Endpoint | Requests/Minute | Notes |
|----------|-----------------|-------|
| GetOrderBook | 1000 | Per symbol |
| GetTicker | 2000 | All symbols |
| GetRecentTrades | 1000 | Per symbol |
| GetKlines | 500 | Per request |
| Subscribe | 10 connections | Per account |

## WebSocket Channels

### Public Channels (No Auth Required)
```bash
# Ticker for all symbols
channels: ["ticker@all"]

# Order book for specific symbol
channels: ["orderbook@BTCUSDT@binance"]

# Trades for specific symbol
channels: ["trades@BTCUSDT@binance"]

# Klines for specific interval
channels: ["klines@BTCUSDT@binance@1h"]
```

### Private Channels (Auth Required)
```bash
# Account-specific order updates
channels: ["orders@account-123"]

# Position updates
channels: ["positions@account-123"]

# Balance updates
channels: ["balances@account-123"]
```

## Data Normalization

All data is normalized across exchanges:

### Symbol Format
- Standard format: `{BASE}{QUOTE}` (e.g., `BTCUSDT`)
- Original exchange symbols preserved in metadata

### Price/Quantity Precision
```protobuf
message SymbolInfo {
  string symbol = 1;
  string exchange = 2;
  int32 price_precision = 3;    // Decimal places for price
  int32 quantity_precision = 4; // Decimal places for quantity
  string min_quantity = 5;      // Minimum order size
  string min_notional = 6;      // Minimum order value
  string tick_size = 7;         // Price increment
  string step_size = 8;         // Quantity increment
}
```

### Timestamp Format
- All timestamps in Unix milliseconds (UTC)
- Consistent across all data types

## Advanced Features

### Cross-Exchange Arbitrage Data
```bash
# Get arbitrage opportunities
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "subscription_type": "ARBITRAGE",
    "symbol": "BTCUSDT",
    "min_profit_bps": 50
  }' \
  localhost:8080 oms.v1.MarketDataService/Subscribe
```

### Market Depth Analysis
```bash
# Get deep order book (1000 levels)
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "symbol": "BTCUSDT",
    "exchange": "binance",
    "depth": 1000
  }' \
  localhost:8080 oms.v1.MarketDataService/GetOrderBook
```

### Time and Sales
```bash
# Get all trades in time range
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "symbol": "BTCUSDT",
    "start_time": 1703894400000,
    "end_time": 1703898000000
  }' \
  localhost:8080 oms.v1.MarketDataService/GetTimeAndSales
```

## Error Handling

### Common Errors

#### Symbol Not Found
```json
{
  "code": 3,
  "message": "INVALID_ARGUMENT: Symbol INVALID not supported",
  "details": [{
    "type": "UnsupportedSymbolError",
    "symbol": "INVALID",
    "supported_symbols": ["BTCUSDT", "ETHUSDT", "..."]
  }]
}
```

#### Exchange Unavailable
```json
{
  "code": 14,
  "message": "UNAVAILABLE: Exchange binance temporarily unavailable",
  "details": [{
    "type": "ExchangeUnavailableError",
    "exchange": "binance",
    "retry_after": 30,
    "alternative_exchanges": ["bybit", "okx"]
  }]
}
```

### Handling Data Gaps
```go
func handleKlineGaps(klines []Kline) {
    for i := 1; i < len(klines); i++ {
        expectedTime := klines[i-1].CloseTime + 1
        if klines[i].OpenTime > expectedTime {
            // Gap detected, request missing data
            missingKlines := requestMissingData(
                klines[i-1].CloseTime,
                klines[i].OpenTime,
            )
            klines = mergeSorted(klines, missingKlines)
        }
    }
}
```

## Performance Optimization

### Efficient Subscriptions
```go
// Subscribe to multiple symbols efficiently
req := &SubscribeRequest{
    SubscriptionType: SubscriptionType_TICKER,
    Channels: []string{
        "ticker@BTCUSDT@binance",
        "ticker@ETHUSDT@binance",
        "ticker@ADAUSDT@binance",
    },
}
```

### Batch Requests
```go
// Request multiple tickers at once
symbols := []string{"BTCUSDT", "ETHUSDT", "ADAUSDT"}
tickers := make([]*Ticker, len(symbols))

var wg sync.WaitGroup
for i, symbol := range symbols {
    wg.Add(1)
    go func(i int, symbol string) {
        defer wg.Done()
        ticker, err := client.GetTicker(ctx, &GetTickerRequest{
            Symbol: symbol,
        })
        if err == nil {
            tickers[i] = ticker
        }
    }(i, symbol)
}
wg.Wait()
```

### Caching Strategy
```go
type MarketDataCache struct {
    tickers    sync.Map
    orderBooks sync.Map
    ttl        time.Duration
}

func (c *MarketDataCache) GetTicker(symbol string) (*Ticker, bool) {
    if data, ok := c.tickers.Load(symbol); ok {
        entry := data.(*CacheEntry)
        if time.Since(entry.Timestamp) < c.ttl {
            return entry.Ticker, true
        }
        c.tickers.Delete(symbol) // Expired
    }
    return nil, false
}
```

## Integration Examples

### React Real-time Dashboard
```typescript
// WebSocket connection to market data stream
const ws = new WebSocket('wss://api.mexoms.com/v1/stream');

ws.onmessage = (event) => {
  const update: MarketDataUpdate = JSON.parse(event.data);
  
  switch(update.type) {
    case 'ticker':
      updateTickerDisplay(update.ticker);
      break;
    case 'orderbook':
      updateOrderBookDisplay(update.order_book);
      break;
    case 'trade':
      addTradeToHistory(update.trade);
      break;
  }
};
```

### Python Trading Bot
```python
import grpc
from proto.oms.v1 import market_data_service_pb2_grpc as md_grpc
from proto.oms.v1 import market_data_service_pb2 as md_pb2

class MarketDataClient:
    def __init__(self, channel):
        self.stub = md_grpc.MarketDataServiceStub(channel)
    
    def get_ticker(self, symbol, exchange="binance"):
        request = md_pb2.GetTickerRequest(
            symbol=symbol,
            exchange=exchange
        )
        return self.stub.GetTicker(request)
    
    def subscribe_tickers(self, symbols):
        request = md_pb2.SubscribeRequest(
            subscription_type=md_pb2.TICKER,
            channels=[f"ticker@{sym}@binance" for sym in symbols]
        )
        
        for update in self.stub.Subscribe(request):
            yield update
```

## Related Services

- [OrderService](./order-service.md) - Place orders based on market data
- [PositionService](./position-service.md) - Monitor P&L changes
- [StrategyService](./strategy-service.md) - Algorithmic trading strategies