# mExOms - Multi-Exchange Cryptocurrency Order Management System

A high-performance cryptocurrency trading system with C++ core engine and Go service layer, designed for ultra-low latency order processing across multiple cryptocurrency exchanges.

## 🚀 Overview

mExOms is a professional-grade Order Management System (OMS) built for cryptocurrency trading with:
- **Ultra-low latency**: < 100μs order processing
- **Multi-exchange support**: Binance, Bybit, OKX, Upbit (extensible)
- **High throughput**: 100,000+ orders/sec
- **Memory-first architecture**: Minimal dependencies, maximum performance

## 🏗️ Architecture

### Technology Stack (Simplified)
- **C++20 Core Engine**: Lock-free data structures, ring buffers, CPU affinity
- **Go Service Layer**: Exchange connectors, business logic
- **NATS JetStream**: Message streaming and event sourcing (replaces traditional DB)
- **Memory Cache**: sync.Map based caching (no Redis dependency)
- **File Storage**: JSON/CSV based persistence (no database dependency)
- **Security**: HashiCorp Vault for API keys

### Project Structure
```
mExOms/
├── core/                    # C++ high-performance engine
│   ├── include/            # Header files
│   ├── src/               # Implementation files
│   └── tests/             # Unit tests
├── services/              # Go exchange connectors
│   ├── binance/          # Binance Spot/Futures
│   ├── bybit/            # Bybit connector (future)
│   └── okx/              # OKX connector (future)
├── internal/             # Go internal packages
│   ├── exchange/         # Exchange abstraction
│   ├── orders/           # Order management
│   └── router/           # Smart order routing
├── pkg/                  # Shared Go packages
│   ├── types/            # Common types
│   ├── cache/            # Memory cache implementation
│   └── nats/             # NATS utilities
├── cmd/                  # Application entry points
├── configs/              # Configuration files
└── data/                 # Data storage
    ├── logs/            # Trading logs
    ├── snapshots/       # State snapshots
    └── reports/         # P&L reports
```

## 🚦 Quick Start

### Prerequisites
- Go 1.21+
- C++20 compiler (GCC 11+ or Clang 13+)
- Docker & Docker Compose
- Make

### Installation

1. Clone the repository:
```bash
git clone https://github.com/yourusername/mExOms.git
cd mExOms
```

2. Install dependencies:
```bash
make install-deps
```

3. Start infrastructure services (NATS and Vault only):
```bash
docker-compose up -d
```

4. Build the project:
```bash
make build
```

### Running the System

1. Build the project:
```bash
make build
```

2. Run all services:
```bash
# Start all services with logging
./scripts/run-all.sh

# Check service health
./scripts/test-services.sh

# Stop all services
./scripts/stop-all.sh
```

3. Or run individually:
```bash
# Start C++ core engine
./core/build/oms-core

# Start gRPC server
./bin/oms-server

# Start exchange connectors
./bin/binance-spot
./bin/binance-futures
```

## 📊 Features

### Current Features
- ✅ Multi-exchange abstraction layer
- ✅ Binance Spot connector with WebSocket support
- ✅ Binance Futures connector with position management
- ✅ Memory-based caching system (sync.Map)
- ✅ NATS JetStream integration
- ✅ Real-time market data streaming
- ✅ Order management (create/cancel/query)
- ✅ Position & margin management for futures
- ✅ Leverage control & risk monitoring
- ✅ Rate limiting
- ✅ Session management
- ✅ File-based storage system
- ✅ API key security with Vault integration

### In Development
- 🔄 Smart order routing
- 🔄 Risk management engine
- 🔄 C++ core engine integration
- 🔄 Additional exchanges (Bybit, OKX, Upbit)
- 🔄 gRPC API Gateway
- 🔄 Monitoring & alerting system

## 🔧 Development

### Build Commands
```bash
# Build entire project
make build

# Build C++ core only
make build-core

# Build Go services only
make build-services

# Run tests
make test

# Run benchmarks
make test-benchmark

# Format code
make fmt

# Lint code
make lint

# Clean build artifacts
make clean
```

### Adding a New Exchange

1. Create connector in `services/{exchange}/`
2. Implement `types.Exchange` interface
3. Add configuration in `configs/config.yaml`
4. Register in exchange factory
5. Add Vault path for API keys: `secret/exchanges/{exchange}_{market}`

Example structure:
```go
type NewExchange struct {
    // Implement types.Exchange interface
}

func (e *NewExchange) GetName() string { return "newexchange" }
func (e *NewExchange) GetMarket() string { return "spot" }
// ... implement other methods
```

## 📈 Performance Targets

- **Order Processing**: < 100 microseconds
- **Risk Checks**: < 50 microseconds  
- **Throughput**: 100,000+ orders/sec
- **Memory Usage**: < 1GB
- **Market Data**: 1,000,000+ messages/sec
- **Startup Time**: < 5 seconds

## 🔒 Security

- **API Keys**: Stored in HashiCorp Vault
- **Encryption**: AES-256 for sensitive data
- **Memory Protection**: mlock() prevents swapping
- **Key Rotation**: Automatic every 30 days
- **Network**: TLS 1.3 for all external connections

## 📝 Configuration

Configuration is managed through `configs/config.yaml`:

```yaml
exchanges:
  binance:
    spot:
      enabled: true
      test_net: true
      rate_limits:
        weight_per_minute: 1200
        orders_per_second: 10
        orders_per_day: 200000

nats:
  url: "nats://localhost:4222"
  cluster_id: "oms-cluster"
  
storage:
  data_dir: "./data"
  snapshot_interval: "1h"
  retention_days: 30
  
cache:
  default_ttl: "5m"
  max_size: 10000
```

## 🗄️ Data Storage Strategy

### Real-time Data (Memory)
- Active orders
- Current positions  
- Order books
- Account balances

### Event Stream (NATS JetStream)
- Order events: `orders.{exchange}.{market}.{symbol}`
- Trade executions: `trades.{exchange}.{market}.{symbol}`
- Position changes: `positions.{exchange}.{market}`
- Retention: 30 days

### Archive (File System)
- Daily trade logs: `/data/logs/2024/01/15/trades.jsonl`
- Hourly snapshots: `/data/snapshots/2024/01/15/14/state.json`
- P&L reports: `/data/reports/2024/01/pnl.csv`

## 🤝 Contributing

Contributions are welcome! Please read our contributing guidelines and submit pull requests to our repository.

## 📄 License

This project is licensed under the MIT License - see the LICENSE file for details.

## 🙏 Acknowledgments

- Binance API SDK: github.com/adshao/go-binance
- NATS.io: High-performance messaging system
- Protocol Buffers: Google's data interchange format

---

**Note**: This project follows the simplified architecture outlined in `oms-guid.md`, emphasizing performance and minimal dependencies. PostgreSQL and Redis are optional and can be added when needed for complex analytics or distributed deployments.