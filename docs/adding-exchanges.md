# Adding New Exchanges Guide

This guide explains how to add support for new cryptocurrency exchanges to the OMS.

## Quick Start

Use the exchange generator to create a new connector:

```bash
# Build the generator
go build -o bin/generate-exchange ./scripts/generate-exchange.go

# Generate Bybit Spot connector
./bin/generate-exchange -preset bybit-spot

# Generate OKX Futures connector
./bin/generate-exchange -preset okx-futures

# Or specify exchange and market type
./bin/generate-exchange -exchange bybit -market spot
```

## Step-by-Step Guide

### 1. Generate Exchange Connector

```bash
# List available presets
./bin/generate-exchange -list

# Generate connector files
./bin/generate-exchange -preset <exchange>-<market>
```

This creates:
- `services/<exchange>/connector.go` - Main connector implementation
- `services/<exchange>/connector_test.go` - Unit tests
- `services/<exchange>/README.md` - Documentation
- `configs/<exchange>_<market>.yaml` - Configuration template

### 2. Implement Exchange-Specific Logic

Edit the generated connector to implement:

#### REST API Methods
```go
// In connector.go, implement these TODO sections:

// Connect - Initialize API client
func (c *BybitConnector) Connect(ctx context.Context) error {
    // Initialize REST client with apiKey, apiSecret
    // Set up authentication
    // Verify connection with a test request
}

// CreateOrder - Convert and send order
func (c *BybitConnector) CreateOrder(ctx context.Context, order *types.Order) (*types.Order, error) {
    // Convert order to exchange format
    // Send POST request to order endpoint
    // Parse response and update order
}
```

#### WebSocket Streams
```go
// SubscribeOrderBook - Real-time orderbook updates
func (c *BybitConnector) SubscribeOrderBook(symbol string, callback types.OrderBookCallback) error {
    // Connect to WebSocket
    // Subscribe to orderbook channel
    // Parse updates and call callback
}
```

### 3. Handle Symbol Formats

Each exchange has different symbol formats:
- Binance: `BTCUSDT`
- Bybit: `BTCUSDT`
- OKX: `BTC-USDT`
- Upbit: `KRW-BTC`

Implement normalization:
```go
func (c *OKXConnector) NormalizeSymbol(symbol string) string {
    // BTC/USDT -> BTC-USDT
    parts := strings.Split(symbol, "/")
    if len(parts) == 2 {
        return parts[0] + "-" + parts[1]
    }
    return symbol
}
```

### 4. Configure API Endpoints

Update the generated config file:
```yaml
# configs/bybit_spot.yaml
api:
  rest:
    base_url: https://api.bybit.com
    # Add specific endpoints if needed
    endpoints:
      create_order: /v5/order/create
      cancel_order: /v5/order/cancel
```

### 5. Set Up Authentication

Add API credentials to Vault:
```bash
# For HashiCorp Vault
vault kv put secret/exchanges/bybit_spot \
  api_key="your-api-key" \
  api_secret="your-api-secret"

# For file-based storage (development)
# Create configs/secrets/bybit_spot.json
{
  "api_key": "your-api-key",
  "api_secret": "your-api-secret"
}
```

### 6. Implement Rate Limiting

Respect exchange rate limits:
```go
// In connector struct
rateLimiter: types.NewRateLimiter(50, time.Second), // 50 req/sec

// In API methods
if err := c.rateLimiter.Wait(ctx); err != nil {
    return nil, err
}
```

### 7. Add to Exchange Factory

Register the new connector in `internal/exchange/factory.go`:
```go
func NewExchange(config ExchangeConfig) (types.Exchange, error) {
    switch config.Name {
    case "binance":
        // existing code
    case "bybit":
        if config.Market == "spot" {
            return bybit.NewBybitConnector(config.APIKey, config.APISecret), nil
        }
        // Add futures support
    // ... other exchanges
    }
}
```

### 8. Test Implementation

#### Unit Tests
```bash
# Run generated tests
go test -v ./services/bybit/

# Run with coverage
go test -v -cover ./services/bybit/
```

#### Integration Tests
Create integration tests:
```go
// services/bybit/integration_test.go
// +build integration

func TestBybitIntegration(t *testing.T) {
    // Test with real API (sandbox)
    connector := NewBybitConnector(
        os.Getenv("BYBIT_API_KEY"),
        os.Getenv("BYBIT_API_SECRET"),
    )
    
    // Test connection
    err := connector.Connect(context.Background())
    require.NoError(t, err)
    
    // Test market data
    ticker, err := connector.GetTicker(context.Background(), "BTC/USDT")
    require.NoError(t, err)
    assert.NotNil(t, ticker)
}
```

Run integration tests:
```bash
go test -v -tags=integration ./services/bybit/
```

### 9. Document Exchange Specifics

Update the generated README with:
- Supported order types
- Special features (e.g., stop orders, OCO)
- Known limitations
- Example usage

### 10. Production Checklist

Before deploying:
- [ ] All required methods implemented
- [ ] Unit tests pass with >80% coverage
- [ ] Integration tests pass
- [ ] Rate limiting tested
- [ ] Error handling comprehensive
- [ ] Logging added for debugging
- [ ] Documentation complete
- [ ] Configuration validated
- [ ] Security review (API key handling)

## Exchange-Specific Considerations

### Bybit
- Supports both REST and WebSocket
- Different endpoints for Spot vs Futures
- Requires signature for authenticated requests
- Symbol format: `BTCUSDT`

### OKX
- Requires API passphrase in addition to key/secret
- Complex order types (algo orders)
- Symbol format: `BTC-USDT` (spot), `BTC-USDT-SWAP` (futures)
- Position mode: net/long-short

### Upbit
- Korean exchange with KRW pairs
- Different API structure
- Symbol format: `KRW-BTC`
- Requires additional compliance

## Common Pitfalls

1. **Symbol Format**: Always test symbol normalization thoroughly
2. **Timestamp Format**: Exchanges use different timestamp formats
3. **Rate Limits**: Implement proper backoff strategies
4. **Order Types**: Not all exchanges support all order types
5. **Decimal Precision**: Handle precision limits correctly

## Testing New Exchanges

### Manual Testing Script
```go
// cmd/test-exchange/main.go
func main() {
    connector := bybit.NewBybitConnector(apiKey, apiSecret)
    
    // Test connection
    if err := connector.Connect(ctx); err != nil {
        log.Fatal(err)
    }
    
    // Test market data
    ticker, _ := connector.GetTicker(ctx, "BTC/USDT")
    fmt.Printf("BTC Price: %s\n", ticker.Last)
    
    // Test order placement (small amount)
    order := &types.Order{
        Symbol:   "BTC/USDT",
        Side:     types.OrderSideBuy,
        Type:     types.OrderTypeLimit,
        Quantity: decimal.NewFromFloat(0.0001),
        Price:    ticker.Bid,
    }
    
    created, err := connector.CreateOrder(ctx, order)
    if err != nil {
        log.Printf("Order failed: %v", err)
    } else {
        fmt.Printf("Order created: %s\n", created.ExchangeOrderID)
    }
}
```

## Support Resources

- Exchange API Documentation
- Discord/Telegram developer channels
- GitHub issues for similar projects
- Exchange-specific SDKs for reference