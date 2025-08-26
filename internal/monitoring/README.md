# Monitoring System

Comprehensive monitoring system for the multi-account OMS with Prometheus metrics, alerting, and web dashboard.

## Overview

The monitoring system provides:
- Real-time metrics collection
- Multi-account performance tracking
- Strategy performance monitoring
- Alert management
- Web-based dashboard
- Prometheus integration

## Components

### 1. Metrics (`metrics.go`)
Prometheus metrics for all system components:

- **Order Metrics**
  - `oms_orders_total`: Total orders by account, exchange, symbol, status
  - `oms_orders_active`: Active orders gauge
  - `oms_order_duration_seconds`: Order execution latency
  - `oms_order_volume_usd`: Order volume in USD

- **Position Metrics**
  - `oms_positions_open`: Open positions by account
  - `oms_position_pnl_usd`: Position P&L
  - `oms_position_margin_usd`: Position margin
  - `oms_position_leverage`: Position leverage

- **Account Metrics**
  - `oms_account_balance_usd`: Account balance
  - `oms_account_equity_usd`: Account equity
  - `oms_account_margin_level`: Margin level
  - `oms_transfers_total`: Transfer count
  - `oms_transfer_volume_usd`: Transfer volume

- **Risk Metrics**
  - `oms_risk_checks_total`: Risk check count
  - `oms_risk_check_duration_seconds`: Risk check latency
  - `oms_risk_violations_total`: Risk violations
  - `oms_kill_switch_status`: Kill switch status

- **Strategy Metrics**
  - `oms_strategy_signals_total`: Strategy signals
  - `oms_strategy_performance`: Performance metrics
  - `oms_strategy_latency_seconds`: Strategy latency

- **System Metrics**
  - `oms_api_requests_total`: API request count
  - `oms_api_request_duration_seconds`: API latency
  - `oms_websocket_connections`: WebSocket connections
  - `oms_exchange_latency_seconds`: Exchange API latency

### 2. Collector (`collector.go`)
Collects metrics from system components:

```go
collector := monitoring.NewCollector(metrics, accountManager, positionManager, &CollectorConfig{
    AccountCollectionInterval:  30 * time.Second,
    PositionCollectionInterval: 10 * time.Second,
})
```

Features:
- Automatic periodic collection
- Account balance tracking
- Position monitoring
- Performance calculation
- Event recording

### 3. Alert Manager (`alerts.go`)
Manages system alerts with configurable rules:

```go
alertManager := monitoring.NewAlertManager(collector)

// Add custom alert rule
alertManager.AddRule(&AlertRule{
    ID:        "high_drawdown",
    Name:      "High Drawdown Alert",
    Level:     AlertLevelWarning,
    Threshold: 0.2, // 20%
    Duration:  5 * time.Minute,
    Condition: checkDrawdownCondition,
})
```

Default Alerts:
- High margin usage (>80%)
- Critical margin level (<120%)
- Large position loss (>$1000)
- Account drawdown (>10%)
- High API error rate (>5%)
- WebSocket disconnection

### 4. Monitoring Server (`server.go`)
HTTP server with Prometheus endpoint and dashboard:

```go
server := monitoring.NewServer(&ServerConfig{
    Port:            9090,
    EnableDashboard: true,
})
```

Endpoints:
- `/metrics` - Prometheus metrics
- `/health` - Health check
- `/alerts` - Active alerts
- `/dashboard` - Web dashboard
- `/api/accounts` - Account data
- `/api/positions` - Position data
- `/api/performance` - Performance metrics

## Usage

### Starting the Monitoring System

```go
ctx := context.Background()

// Create components
metrics := monitoring.NewMetrics()
collector := monitoring.NewCollector(metrics, accountManager, positionManager, nil)
alertManager := monitoring.NewAlertManager(collector)
server := monitoring.NewServer(nil, metrics, collector, alertManager)

// Start all components
collector.Start(ctx)
alertManager.Start(ctx)
server.Start(ctx)

// Monitor alerts
go func() {
    for alert := range alertManager.GetAlertChannel() {
        log.Printf("Alert: %s - %s", alert.Level, alert.Message)
    }
}()
```

### Recording Metrics

```go
// Record order
collector.RecordOrderEvent(order, executionTime)

// Record position update
collector.RecordPosition(accountID, exchange, symbol, side, pnl, margin, leverage)

// Record risk check
collector.RecordRiskCheckEvent(accountID, "position_limit", "passed", duration)

// Record strategy signal
collector.RecordStrategySignal("arbitrage", "buy", "BTCUSDT")

// Record exchange latency
collector.RecordExchangeLatency("binance", "order", latency)
```

### Creating Custom Alerts

```go
// Create custom condition
condition := func(ctx context.Context) (float64, bool, map[string]interface{}) {
    // Check condition
    value := getMetricValue()
    triggered := value > threshold
    details := map[string]interface{}{
        "account_id": "main",
        "metric": "custom_metric",
    }
    return value, triggered, details
}

// Add alert rule
alertManager.AddRule(&AlertRule{
    ID:        "custom_alert",
    Name:      "Custom Alert",
    Level:     AlertLevelWarning,
    Condition: condition,
    Threshold: 100,
    Duration:  1 * time.Minute,
    Actions: []AlertAction{
        sendEmailAlert,
        logAlert,
    },
})
```

## Dashboard

The web dashboard provides real-time monitoring at `http://localhost:9090/dashboard`:

- **Overview Cards**: Key metrics summary
- **Active Alerts**: Current system alerts
- **Account Summary**: Balance, equity, margin levels
- **Open Positions**: Real-time position tracking
- **Performance Charts**: P&L and strategy performance

## Prometheus Integration

Configure Prometheus to scrape metrics:

```yaml
scrape_configs:
  - job_name: 'oms'
    static_configs:
      - targets: ['localhost:9090']
    scrape_interval: 15s
```

Example queries:
```promql
# Total order volume last hour
sum(rate(oms_order_volume_usd[1h]))

# Average order latency by exchange
avg(oms_order_duration_seconds) by (exchange)

# Account equity over time
oms_account_equity_usd

# Position P&L by strategy
sum(oms_position_pnl_usd) by (account_id)

# Risk check success rate
sum(rate(oms_risk_checks_total{result="passed"}[5m])) /
sum(rate(oms_risk_checks_total[5m]))
```

## Grafana Dashboard

Import the Grafana dashboard for advanced visualization:
1. Import dashboard JSON from `configs/grafana-dashboard.json`
2. Set Prometheus data source
3. Configure alert notifications

## Performance Considerations

- Metrics are collected asynchronously
- Configurable collection intervals
- Efficient Prometheus metric updates
- Alert evaluation is throttled
- Dashboard updates via polling

## Testing

Run the monitoring test:
```bash
go run cmd/test-monitoring/main.go
```

This will:
1. Start monitoring server on port 9090
2. Simulate trading activity
3. Generate sample metrics
4. Trigger test alerts
5. Serve dashboard

## Production Deployment

1. **Metrics Retention**: Configure Prometheus retention
2. **Alert Routing**: Set up alert notification channels
3. **Dashboard Security**: Add authentication to dashboard
4. **Scaling**: Use Prometheus federation for multiple instances
5. **Backup**: Regular metric data backups