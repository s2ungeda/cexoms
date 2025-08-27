# OMS Dashboard - Real Environment Configuration

This document describes how to run the OMS Dashboard connected to your real OMS backend.

## Prerequisites

1. OMS services must be running and accessible
2. NATS server must be running on `localhost:4222` (or configure `NATS_URL`)
3. Node.js 18+ and Go 1.21+ installed

## Quick Start

### Development Mode

Run the dashboard connected to local OMS services:

```bash
cd dashboard
./start-real.sh
```

This will:
- Build and start the real OMS server (connects to actual OMS NATS subjects)
- Start the React frontend in development mode
- Dashboard will be available at http://localhost:3000

### Production Mode (Docker)

Run with Docker Compose:

```bash
cd dashboard
docker-compose -f docker-compose.real.yml up -d
```

## Real OMS Integration

### NATS Subjects

The dashboard subscribes to the following OMS NATS subjects:

- `order.event.*` - Order lifecycle events (NEW, FILLED, CANCELLED, etc.)
- `position.update.*` - Position updates with P&L
- `market.data.*` - Market data from exchanges
- `risk.metrics.*` - Risk management metrics
- `oms.health.*` - System health and service status
- `trade.executed.*` - Trade execution confirmations

### Data Flow

1. **Orders**: Real order events are published by OMS to `order.event.*`
2. **Positions**: Position manager publishes to `position.update.*`
3. **Market Data**: Exchange connectors publish to `market.data.*`
4. **System Health**: OMS monitor publishes to `oms.health.*`

### Configuration

Environment variables:
- `NATS_URL`: NATS server URL (default: `nats://localhost:4222`)
- `ENVIRONMENT`: Environment name (development/staging/production)

## Differences from Demo Mode

| Feature | Demo Mode | Real Mode |
|---------|-----------|-----------|
| Data Source | Random generator | Real OMS via NATS |
| Order Status | Simulated changes | Actual order lifecycle |
| Positions | Mock positions | Real positions from exchanges |
| System Health | Fake metrics | Actual service health |
| Market Data | Random prices | Live market data |

## Monitoring

### Health Check
```bash
curl http://localhost:8080/health
```

### Prometheus Metrics
```bash
curl http://localhost:8080/metrics
```

### NATS Monitoring
```bash
# View active subscriptions
curl http://localhost:8222/subsz

# View connection info
curl http://localhost:8222/connz
```

## Troubleshooting

### No Data Showing

1. Check NATS connection:
```bash
nats-cli sub ">"  # Should show OMS messages
```

2. Verify OMS services are running:
```bash
docker ps | grep mexoms
```

3. Check server logs:
```bash
docker logs dashboard-real
```

### Connection Issues

1. Verify NATS is accessible:
```bash
nc -zv localhost 4222
```

2. Check firewall/network settings
3. Ensure OMS services are publishing to expected subjects

### Performance Issues

1. Monitor WebSocket connections:
```bash
curl http://localhost:8080/api/summary
```

2. Check NATS message rates:
```bash
nats-cli server report connections
```

## Security Considerations

1. **CORS**: Update allowed origins in `main_real.go` for production domains
2. **Authentication**: Add authentication middleware for production use
3. **TLS**: Enable TLS for WebSocket connections in production
4. **Network**: Use internal networks for NATS communication

## Development Tips

1. To see raw NATS messages:
```bash
nats-cli sub "order.event.*" --raw
```

2. To publish test messages:
```bash
nats-cli pub order.event.BINANCE '{"symbol":"BTCUSDT","status":"NEW"}'
```

3. Monitor WebSocket traffic in browser:
   - Open Developer Tools > Network > WS
   - Filter by "ws" to see WebSocket frames

## Next Steps

1. Add authentication/authorization
2. Implement user preferences storage
3. Add historical data API endpoints
4. Enable alert configuration UI
5. Add strategy performance metrics