# NATS Integration Example

This example demonstrates how the OMS services publish real-time data to NATS for the monitoring dashboard.

## NATS Subjects

The following NATS subjects are published by the OMS services:

### Order Events
- `order.event.{exchange}.{market}.{symbol}` - Order lifecycle events
  - Event types: NEW, FILLED, CANCELLED, REJECTED, STATUS_UPDATE, PARTIALLY_FILLED

### Position Updates  
- `position.update.{exchange}.{market}.{symbol}` - Position changes
  - Event types: OPEN, UPDATE, CLOSE, LIQUIDATED

### Market Data
- `market.data.{exchange}.{symbol}` - Real-time price updates
- `prices.snapshot` - Aggregated price snapshot

### Risk Metrics
- `risk.metrics.{exchange}.{account}` - Account-specific risk metrics
- `risk.metrics.system` - System-wide risk metrics

### System Health
- `oms.health.system` - Overall system health status
- `oms.health.component.{name}` - Individual component health

## Running the Example

1. Start NATS server:
```bash
docker-compose up -d nats
```

2. Run the integration example:
```bash
go run examples/nats_integration/main.go
```

3. Monitor NATS messages:
```bash
# Subscribe to all order events
nats sub "order.event.>"

# Subscribe to position updates
nats sub "position.update.>"

# Subscribe to risk metrics
nats sub "risk.metrics.>"

# Subscribe to system health
nats sub "oms.health.>"
```

## Integration with Real Services

To integrate these publishers with your real services:

1. **Order Service**: Create orders through the Order Service which automatically publishes events
2. **Position Manager**: Update positions which triggers position events
3. **Risk Manager**: Updates account balances and positions to publish risk metrics
4. **Health Monitor**: Automatically publishes health status every 5 seconds

## Message Formats

### Order Event
```json
{
  "event_type": "FILLED",
  "order": {
    "id": "ORD_123",
    "exchange": "binance",
    "symbol": "BTCUSDT",
    "side": "BUY",
    "price": "45000",
    "quantity": "0.01",
    "status": "FILLED"
  },
  "timestamp": "2024-01-01T00:00:00Z"
}
```

### Position Update
```json
{
  "event_type": "UPDATE",
  "position": {
    "symbol": "BTCUSDT",
    "exchange": "binance",
    "quantity": "0.01",
    "entry_price": "45000",
    "mark_price": "45100",
    "unrealized_pnl": "1.00"
  },
  "timestamp": "2024-01-01T00:00:00Z"
}
```

### Risk Metrics
```json
{
  "account_id": "binance_main",
  "exchange": "binance",
  "total_exposure": "1000.00",
  "open_positions": 3,
  "current_drawdown": 0.02,
  "daily_pnl": "50.00",
  "var_95": "-100.00",
  "updated_at": "2024-01-01T00:00:00Z"
}
```