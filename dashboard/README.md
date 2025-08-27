# mExOms Monitoring Dashboard

Real-time monitoring dashboard for the Multi-Exchange Order Management System.

## Features

- **Real-time Order Monitoring**: Track all orders across exchanges
- **Position Overview**: Current positions and P&L
- **Market Data Visualization**: Live price charts and order books
- **System Health**: CPU, memory, latency metrics
- **Risk Metrics**: Exposure, VaR, drawdown tracking
- **Trading Statistics**: Volume, win rate, performance metrics
- **Alert Management**: Real-time alerts and notifications

## Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   React Web UI  │────▶│ WebSocket Server │────▶│   OMS Backend   │
└─────────────────┘     └──────────────────┘     └─────────────────┘
         │                       │                         │
         │                       │                         │
         ▼                       ▼                         ▼
   ┌──────────┐           ┌──────────┐             ┌──────────┐
   │  Charts  │           │   NATS   │             │ Exchange │
   └──────────┘           └──────────┘             └──────────┘
```

## Quick Start

### Demo Mode (Simulated Data)

#### Backend (WebSocket Server)

```bash
# From project root
cd dashboard/server
go run main.go
```

#### Frontend (React)

```bash
# From project root
cd dashboard/frontend
npm install
npm start
```

Access the dashboard at http://localhost:3000

### Real Mode (Connected to OMS)

Use the provided script to connect to your real OMS backend:

```bash
cd dashboard
./start-real.sh
```

This connects to actual OMS NATS subjects and displays real trading data.
See [README_REAL_OMS.md](README_REAL_OMS.md) for detailed configuration.

## WebSocket API

### Connection
```javascript
ws://localhost:8080/ws
```

### Message Types

#### Subscribe to Data Streams
```json
{
  "type": "subscribe",
  "streams": ["orders", "positions", "market", "system"]
}
```

#### Order Updates
```json
{
  "type": "order_update",
  "data": {
    "orderId": "123",
    "symbol": "BTCUSDT",
    "side": "BUY",
    "status": "FILLED",
    "price": 50000,
    "quantity": 0.1,
    "timestamp": "2024-01-01T12:00:00Z"
  }
}
```

#### Position Updates
```json
{
  "type": "position_update",
  "data": {
    "symbol": "BTCUSDT",
    "quantity": 0.5,
    "avgPrice": 49500,
    "currentPrice": 50000,
    "pnl": 250,
    "pnlPercent": 0.5
  }
}
```

#### Market Data
```json
{
  "type": "market_update",
  "data": {
    "symbol": "BTCUSDT",
    "price": 50000,
    "volume": 1234.56,
    "bid": 49999,
    "ask": 50001
  }
}
```

#### System Metrics
```json
{
  "type": "system_metrics",
  "data": {
    "cpu": 45.2,
    "memory": 2048,
    "latency": 0.5,
    "ordersPerSecond": 150,
    "activeConnections": 10
  }
}
```

## Dashboard Pages

### 1. Overview
- Portfolio summary
- P&L chart
- Recent trades
- Active positions

### 2. Orders
- Active orders table
- Order history
- Order execution timeline
- Fill statistics

### 3. Positions
- Current positions
- Position P&L
- Risk metrics
- Exposure by symbol

### 4. Market Data
- Real-time price charts
- Order book visualization
- Volume analysis
- Market microstructure

### 5. Risk Management
- VaR calculation
- Exposure limits
- Drawdown analysis
- Risk alerts

### 6. System Health
- Service status
- Latency monitoring
- Error rates
- Resource usage

## Configuration

Edit `dashboard/config.yaml`:

```yaml
server:
  port: 8080
  cors_origins:
    - http://localhost:3000
    - https://yourdomain.com

nats:
  url: nats://localhost:4222
  
redis:
  addr: localhost:6379
  
monitoring:
  metrics_interval: 1s
  cleanup_interval: 5m
  max_history: 1000
```

## Development

### Adding New Metrics

1. Define the metric type in `types.go`
2. Add NATS subscription in `subscriber.go`
3. Create WebSocket handler in `websocket.go`
4. Add React component in `frontend/src/components`

### Custom Charts

The dashboard uses Chart.js for visualization. To add custom charts:

```javascript
import { Line } from 'react-chartjs-2';

const CustomChart = ({ data }) => {
  const chartData = {
    labels: data.timestamps,
    datasets: [{
      label: 'Custom Metric',
      data: data.values,
      borderColor: 'rgb(75, 192, 192)',
    }]
  };
  
  return <Line data={chartData} />;
};
```

## Security

- WebSocket connections support JWT authentication
- CORS configured for specific origins
- Rate limiting on API endpoints
- SSL/TLS support for production

## Deployment

### Docker

Demo mode:
```bash
# Build and run with Docker Compose
docker-compose -f dashboard/docker-compose.yml up
```

Real mode (connected to OMS):
```bash
# Use the real environment compose file
docker-compose -f dashboard/docker-compose.real.yml up
```

### Kubernetes

```bash
# Deploy to Kubernetes
kubectl apply -f dashboard/k8s/
```

## Monitoring the Monitor

The dashboard itself exports Prometheus metrics:

- `dashboard_active_connections`: Number of active WebSocket connections
- `dashboard_messages_sent`: Total messages sent to clients
- `dashboard_errors_total`: Total errors encountered
- `dashboard_latency_ms`: Message processing latency