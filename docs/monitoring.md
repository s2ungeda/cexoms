# Monitoring System

The OMS monitoring system provides comprehensive observability with file-based metrics, structured logging, health checks, and a real-time dashboard.

## Architecture

```
┌─────────────────┐     ┌──────────────┐     ┌───────────────┐
│   Components    │────▶│   Metrics    │────▶│  JSONL Files  │
│  (OMS Services) │     │  Collector   │     │   (Metrics)   │
└─────────────────┘     └──────────────┘     └───────────────┘
                               │
┌─────────────────┐            │              ┌───────────────┐
│  Health Checks  │────────────┼─────────────▶│   Dashboard   │
└─────────────────┘            │              │  (Web UI)     │
                               │              └───────────────┘
┌─────────────────┐            │
│ Structured Logs │────────────┘              ┌───────────────┐
└─────────────────┘                          │   Log Files   │
                                             │   (Rotated)   │
                                             └───────────────┘
```

## Components

### 1. Metrics Collector

File-based metrics collection with in-memory aggregation:

```go
// Counter metrics
metrics.IncrementCounter("orders_placed", map[string]string{
    "exchange": "binance",
    "type": "limit",
})

// Gauge metrics
metrics.SetGauge("active_connections", 42.0, nil)

// Histogram metrics (with buckets)
metrics.ObserveHistogram("order_latency_ms", 2.3, map[string]string{
    "exchange": "binance",
})

// Summary metrics (with quantiles)
metrics.ObserveSummary("request_duration_ms", 15.2, map[string]string{
    "endpoint": "/api/orders",
})
```

**Features:**
- Lock-free atomic operations for high performance
- Automatic file rotation (hourly or by size)
- JSONL format for easy parsing
- Label support for multi-dimensional metrics

### 2. Health Check System

Component health monitoring with caching:

```go
// Register health checks
health.RegisterCheck("nats", NATSHealthCheck("nats://localhost:4222"))
health.RegisterCheck("filesystem", FileSystemHealthCheck("./data"))
health.RegisterCheck("memory", MemoryHealthCheck(80.0))

// HTTP endpoint: GET /health
{
    "status": "healthy",
    "components": [
        {
            "name": "nats",
            "status": "healthy",
            "message": "NATS is connected",
            "last_checked": "2024-01-15T10:30:00Z",
            "details": {
                "url": "nats://localhost:4222",
                "connected": true,
                "subscriptions": 42
            }
        }
    ],
    "version": "1.0.0",
    "uptime": "24h15m30s",
    "timestamp": "2024-01-15T10:30:00Z"
}
```

**Health Status:**
- `healthy`: All systems operational
- `degraded`: Some issues but functional
- `unhealthy`: Critical issues

### 3. Structured Logger

JSON-formatted logging with automatic rotation:

```go
// Basic logging
logger.Info("Order placed", map[string]interface{}{
    "exchange": "binance",
    "symbol": "BTCUSDT",
    "order_id": "12345",
})

// With context fields
orderLogger := logger.WithFields(map[string]interface{}{
    "exchange": "binance",
    "symbol": "BTCUSDT",
})
orderLogger.Info("Order placed")
orderLogger.Error("Order failed", err)

// Log levels: DEBUG, INFO, WARN, ERROR, FATAL
```

**Log Format:**
```json
{
    "timestamp": "2024-01-15T10:30:00.123Z",
    "level": "INFO",
    "component": "order-service",
    "message": "Order placed successfully",
    "fields": {
        "exchange": "binance",
        "symbol": "BTCUSDT",
        "order_id": "12345",
        "latency_ms": 23.5
    },
    "trace_id": "abc123",
    "exchange": "binance",
    "order_id": "12345",
    "symbol": "BTCUSDT"
}
```

### 4. Real-time Dashboard

Web-based monitoring dashboard with live updates:

- **URL**: http://localhost:8081
- **Features**:
  - System health overview
  - Position summary
  - Risk metrics
  - Performance graphs
  - Recent activity log
  - WebSocket updates (coming soon)

## Usage

### Starting the Monitor

```bash
# Start with default settings
./bin/monitor

# Custom configuration
./bin/monitor \
    -metrics-dir ./data/metrics \
    -logs-dir ./logs \
    -http-addr :8080 \
    -dashboard-addr :8081
```

### API Endpoints

#### Health Check
```bash
curl http://localhost:8080/health
```

#### Metrics (Prometheus Format)
```bash
curl http://localhost:8080/metrics
```

#### Log Query
```bash
curl "http://localhost:8080/logs/query?level=ERROR&component=order-service&limit=100"
```

## File Storage

### Metrics Files

Location: `./data/metrics/`
Format: JSONL (one metric per line)

```json
{"name":"orders_placed","type":"counter","value":12345,"labels":{"exchange":"binance"},"timestamp":"2024-01-15T10:30:00Z"}
{"name":"order_latency_ms","type":"histogram","value":{"buckets":[0.001,0.002,0.005],"counts":[100,200,50],"sum":125.5,"count":350},"timestamp":"2024-01-15T10:30:01Z"}
```

### Log Files

Location: `./logs/`
Format: JSON (one entry per line)
Rotation: Daily or by size (50MB default)

```
order-service_20240115_103000.log
order-service_20240115_143000.log
risk-engine_20240115_103000.log
```

## Performance Considerations

1. **Metrics Collection**: < 1μs per metric update
2. **Log Writing**: Async with 10k entry buffer
3. **Health Checks**: 10s cache to reduce overhead
4. **File Rotation**: Background process, non-blocking
5. **Dashboard Updates**: 2s refresh interval

## Integration Example

```go
// In your service
type Service struct {
    metrics *monitor.MetricsCollector
    logger  *monitor.Logger
}

func (s *Service) PlaceOrder(order *Order) error {
    start := time.Now()
    
    // Log order attempt
    s.logger.Info("Placing order", map[string]interface{}{
        "order_id": order.ID,
        "symbol": order.Symbol,
    })
    
    // Process order...
    
    // Record metrics
    latency := time.Since(start).Milliseconds()
    s.metrics.ObserveHistogram("order_latency_ms", float64(latency), map[string]string{
        "exchange": order.Exchange,
        "type": order.Type,
    })
    
    s.metrics.IncrementCounter("orders_placed", map[string]string{
        "exchange": order.Exchange,
    })
    
    return nil
}
```

## Alerting (Future Enhancement)

While not implemented yet, the monitoring system is designed to support alerting:

```yaml
# Future: alerts.yaml
alerts:
  - name: high_error_rate
    condition: rate(errors) > 10
    duration: 5m
    action: notify
    
  - name: high_latency
    condition: p95(order_latency_ms) > 100
    duration: 1m
    action: page
```

## Best Practices

1. **Use Labels Wisely**: Keep cardinality low (< 100 unique label combinations)
2. **Log Levels**: Use appropriate levels (INFO for normal, ERROR for failures)
3. **Metric Names**: Use descriptive names with units (e.g., `_ms`, `_bytes`)
4. **Health Checks**: Keep checks fast (< 5s timeout)
5. **File Cleanup**: Implement retention policy for old files

## Troubleshooting

### High Disk Usage
- Check metric/log file sizes
- Adjust rotation settings
- Implement cleanup script

### Missing Metrics
- Verify collector is running
- Check file permissions
- Review metric registration

### Dashboard Not Loading
- Check server is running
- Verify port availability
- Check browser console for errors