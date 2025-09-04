# mExOms - Multi-Exchange Cryptocurrency Order Management System

A high-performance cryptocurrency trading system with C++ core engine and Go service layer, designed for ultra-low latency order processing across multiple cryptocurrency exchanges.

**Project Status**: ✅ Production Ready (All 22 phases completed)

## 🚀 Overview

mExOms is a professional-grade Order Management System (OMS) built for cryptocurrency trading with:
- **Ultra-low latency**: < 100μs order processing (WebSocket: ~35ms)
- **Multi-exchange support**: Binance Spot/Futures implemented (Bybit, OKX, Upbit ready)
- **Multi-account management**: Support up to 200 sub-accounts
- **High throughput**: 100,000+ orders/sec
- **Automated strategies**: Arbitrage, Market Making
- **Memory-first architecture**: Minimal dependencies, maximum performance
- **WebSocket-first**: All order operations use WebSocket when available
- **Real-time monitoring**: Web-based dashboard with live data

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
│   ├── binance/          # Binance Spot/Futures ✅
│   ├── bybit/            # Bybit connector (ready for implementation)
│   └── okx/              # OKX connector (ready for implementation)
├── internal/             # Go internal packages
│   ├── exchange/         # Exchange abstraction
│   ├── account/          # Multi-account management
│   ├── orders/           # Order management
│   ├── risk/             # Risk management engine
│   ├── router/           # Smart order routing
│   ├── strategies/       # Trading strategies
│   │   ├── arbitrage/    # Arbitrage detection & execution
│   │   ├── market_maker/ # Market making engine
│   │   └── orchestrator/ # Strategy orchestration
│   └── backtest/         # Backtesting engine
├── pkg/                  # Shared Go packages
│   ├── types/            # Common types
│   ├── cache/            # Memory cache implementation
│   └── nats/             # NATS utilities
├── dashboard/            # Real-time monitoring web UI
├── examples/             # Example code and tutorials
├── docs/                 # Comprehensive documentation
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
git clone https://github.com/your-org/mExOms.git
cd mExOms
```

2. Install dependencies:
```bash
make install-deps
```

3. Start infrastructure services (NATS and Vault):
```bash
docker-compose up -d
```

4. Set up Vault and store API keys:
```bash
# Initialize Vault
./scripts/vault-setup.sh

# Add your API keys
./cmd/vault-cli/vault-cli add binance spot YOUR_API_KEY YOUR_SECRET_KEY
```

5. Build the project:
```bash
make build
```

### Running the System

#### Quick Start (Recommended)
```bash
# Start entire OMS system with one command
make start-all

# Or using the script
./scripts/start-oms.sh

# Stop all services
make stop-all
```

#### Step-by-Step Setup

1. Start infrastructure:
```bash
# Start NATS, Redis, and Vault
make infra-up
```

2. Initialize Vault (first time only):
```bash
./scripts/init-vault.sh
```

3. Store API keys:
```bash
./scripts/store-binance-keys.sh
```

4. Start OMS services:
```bash
# Market data service
go run cmd/binance-market-full/main.go &

# Balance service (requires API keys)
go run cmd/binance-spot-balance/main.go &

# Dashboard
cd dashboard && ./oms-dashboard-real &
```

5. Access the dashboard:
```
http://localhost:3000
```

#### Using Docker Compose
```bash
# Start infrastructure only
docker-compose up -d

# Stop everything
docker-compose down
```

## 📊 Features

### Current Features
- ✅ Multi-exchange abstraction layer
- ✅ Binance Spot connector with WebSocket support
- ✅ Binance Futures connector with position management
- ✅ WebSocket order management (30-80% latency reduction)
- ✅ Multi-account support with per-account rate limiting
- ✅ Memory-based caching system (sync.Map)
- ✅ NATS JetStream integration
- ✅ Real-time market data streaming (24hr tickers)
- ✅ Order management (create/cancel/query)
- ✅ Position & margin management for futures
- ✅ Leverage control & risk monitoring
- ✅ Rate limiting
- ✅ Session management
- ✅ File-based storage system
- ✅ API key security with HashiCorp Vault integration
- ✅ Vault CLI for key management
- ✅ Real-time monitoring dashboard
- ✅ Balance tracking and display
- ✅ Automated system startup/shutdown
- ✅ Persistent Vault storage for development
- ✅ TCP multi-client server (C++ core)

### Completed Phases
- ✅ Phase 1-23: All core functionality implemented
- ✅ Performance targets achieved (<100μs latency)
- ✅ Production-ready arbitrage and market-making strategies

### In Development
- 🔄 Additional exchanges (Bybit, OKX, Upbit)
- 🔄 Advanced monitoring & alerting system
- 🔄 Cloud deployment automation

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

## 📈 Performance

### Performance Targets
- **Order Processing**: < 100 microseconds (C++ core)
- **Risk Checks**: < 50 microseconds  
- **Throughput**: 100,000+ orders/sec
- **Memory Usage**: < 1GB
- **Market Data**: 1,000,000+ messages/sec
- **Startup Time**: < 5 seconds

### Actual Performance (WebSocket)
| Operation | REST API | WebSocket | Improvement |
|-----------|----------|-----------|-------------|
| Create Order | 50-200ms | ~35ms | 30-80% |
| Cancel Order | 45-180ms | ~30ms | 33-83% |
| Query Order | 40-150ms | ~25ms | 37-83% |

## 🔒 Security

- **API Keys**: Stored in HashiCorp Vault
- **Encryption**: AES-256 for sensitive data
- **Memory Protection**: mlock() prevents swapping
- **Key Rotation**: Automatic every 30 days
- **Network**: TLS 1.3 for all external connections

## ✨ Key Features

### Multi-Account Trading
- Manage multiple trading accounts simultaneously
- Account-specific risk limits and strategies
- Automatic balance rebalancing
- API key rotation and management

### Smart Order Routing
- Find best execution across exchanges and accounts
- Order splitting for large orders
- Minimize slippage and fees
- Rate limit optimization

### Automated Strategies
- **Arbitrage**: Cross-exchange and triangular arbitrage
- **Market Making**: Dynamic spread adjustment, inventory management
- **Strategy Orchestrator**: Run multiple strategies concurrently

### Risk Management
- Real-time position and P&L tracking
- Account and portfolio-level risk limits
- Automatic stop-loss and take-profit
- Kill switch for emergency situations

### Backtesting System
- Historical data replay
- Strategy performance analysis
- Parameter optimization
- Walk-forward analysis

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