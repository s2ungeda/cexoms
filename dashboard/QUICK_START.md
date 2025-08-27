# mExOms Dashboard Quick Start Guide

## Overview

The mExOms Dashboard provides real-time monitoring for your Multi-Exchange Order Management System with:
- Live order tracking
- Position monitoring with P&L
- Market data visualization
- Risk management metrics
- System health monitoring

## Prerequisites

- Go 1.21+
- Node.js 18+
- Docker & Docker Compose (optional)
- NATS server

## Quick Start (Development Mode)

### 1. Start NATS Server

```bash
docker run -d --name nats -p 4222:4222 -p 8222:8222 nats:latest -js -m 8222
```

### 2. Start Dashboard Server

```bash
cd dashboard/server
go mod init github.com/your-org/mExOms/dashboard/server
go get github.com/gorilla/websocket github.com/nats-io/nats.go github.com/prometheus/client_golang/prometheus go.uber.org/zap
go run main.go
```

The server will start on http://localhost:8080

### 3. Start Frontend

```bash
cd dashboard/frontend
npm install
npm start
```

The dashboard will open at http://localhost:3000

### 4. Start Demo Data Generator (Optional)

```bash
cd dashboard/demo
go mod init github.com/your-org/mExOms/dashboard/demo
go get github.com/nats-io/nats.go
go run data_generator.go
```

## Quick Start (Docker Mode)

```bash
cd dashboard
docker-compose up
```

This will start:
- NATS message broker
- Dashboard WebSocket server
- React frontend
- Demo data generator

Access the dashboard at http://localhost:3000

## Dashboard Features

### 1. Overview Page
- Portfolio value and daily P&L
- Position distribution chart
- Recent trades table
- Risk metrics summary

### 2. Orders Page
- Real-time order updates
- Order filtering and search
- Order type distribution
- Execution statistics

### 3. Positions Page
- Current positions with unrealized P&L
- Position concentration analysis
- Risk exposure breakdown
- One-click position closing

### 4. Market Data Page
- Real-time price charts
- Order book visualization
- Market tickers
- Recent trades feed

### 5. Risk Management Page
- VaR and CVaR metrics
- Drawdown tracking
- Exposure limits monitoring
- Risk alerts

### 6. System Health Page
- CPU and memory usage
- Service health status
- Latency monitoring
- Performance metrics

## WebSocket API

Connect to `ws://localhost:8080/ws`

### Subscribe to data streams:
```json
{
  "type": "subscribe",
  "streams": ["orders", "positions", "market", "system", "risk"]
}
```

### Message types:
- `order_update`: Order status changes
- `position_update`: Position changes
- `market_update`: Price updates
- `system_metrics`: System health data
- `risk_update`: Risk metrics

## Configuration

Edit `dashboard/server/main.go` to configure:
- NATS connection URL
- Server port
- CORS origins

## Monitoring the Monitor

Access Prometheus metrics at http://localhost:8080/metrics

Key metrics:
- `dashboard_active_connections`: Active WebSocket connections
- `dashboard_messages_sent_total`: Total messages sent
- `dashboard_errors_total`: Error count
- `dashboard_message_latency_ms`: Processing latency

## Troubleshooting

### WebSocket Connection Issues
1. Check CORS settings in server
2. Verify NATS is running
3. Check browser console for errors

### No Data Showing
1. Ensure data generator is running
2. Check NATS connection
3. Verify WebSocket subscription

### Performance Issues
1. Limit chart history points
2. Reduce update frequency
3. Enable data aggregation

## Production Deployment

1. Use environment variables for configuration
2. Enable TLS for WebSocket connections
3. Set up proper CORS origins
4. Use a reverse proxy (nginx)
5. Enable authentication
6. Set up monitoring alerts

## Next Steps

- Connect to real OMS backend instead of demo data
- Add user authentication
- Implement data persistence
- Add custom alerts
- Create mobile responsive design