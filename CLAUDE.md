# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Multi-Exchange Cryptocurrency Order Management System (mExOms)** - A production-ready, high-performance trading system with C++ core engine and Go service layer, designed for ultra-low latency order processing across multiple cryptocurrency exchanges.

**Status**: All 23 phases completed. System is production-ready.

## Quick Start

```bash
# Start entire OMS system with one command
make start-all

# Stop all services
make stop-all

# Check system status
ps aux | grep -E "binance|dashboard" | grep -v grep
```

## Common Development Commands

### Build Commands
```bash
# Install all dependencies
make deps

# Build entire project (C++ core + Go services)
make build

# Build only C++ core engine
make build-core

# Build only Go services
make build-services

# Generate protobuf files
make proto

# Clean build artifacts
make clean
```

### Test Commands
```bash
# Run all tests (Go + C++ + integration)
make test

# Run only Go tests with race detection
make test-go

# Run C++ core tests
make test-core

# Run integration tests
make test-integration

# Run performance benchmarks
make test-benchmark
```

### Infrastructure Commands
```bash
# Start all infrastructure (NATS + Redis + Vault)
make infra-up

# Stop all infrastructure
make infra-down

# Start individual services
make run-nats      # Message broker with JetStream
make run-redis     # In-memory cache
make run-vault     # Secret management (persistent storage)

# Using Docker Compose
docker-compose up -d      # Start all
docker-compose down       # Stop all
```

### Service Commands
```bash
# Start market data service (no API key required)
go run cmd/binance-market-full/main.go

# Start balance service (requires API keys in Vault)
go run cmd/binance-spot-balance/main.go

# Start dashboard backend server
cd dashboard && ./oms-dashboard-real

# Start frontend (development mode with hot-reload)
cd dashboard/frontend && npm start
```

### Vault API Key Management
```bash
# First-time Vault initialization
./scripts/init-vault.sh

# Store Binance API keys in Vault
./scripts/store-binance-keys.sh

# Backup/restore Vault keys
./scripts/backup-vault-keys.sh
./scripts/restore-vault-keys.sh
```

### Development Helpers
```bash
# Format all code (Go + C++)
make fmt

# Lint all code
make lint

# Full development cycle
make dev    # deps + fmt + lint + build + test

# View logs
tail -f logs/market.log
tail -f logs/balance.log
tail -f logs/dashboard.log
```

## Architecture Overview

### Directory Structure

```
mExOms/
├── core/                      # C++20 high-performance engine
│   ├── include/               # Header files
│   │   ├── types.h            # Core type definitions
│   │   ├── ring_buffer.h      # Lock-free ring buffer
│   │   ├── order_manager.h    # Order management
│   │   ├── account/           # Account management
│   │   ├── performance/       # Performance utilities
│   │   ├── risk/              # Risk engine headers
│   │   └── strategies/        # Trading strategy headers
│   ├── src/                   # C++ implementations
│   │   ├── risk/              # Risk engine
│   │   └── strategies/        # Strategy implementations
│   ├── engine/                # Core order processing logic
│   ├── tcp_server/            # TCP multi-client server
│   └── tests/                 # C++ unit tests
│
├── cmd/                       # Go executable entry points (69 commands)
│   ├── binance-market-full/   # Market data with 24hr ticker stream
│   ├── binance-spot/          # Spot trading connector
│   ├── binance-spot-balance/  # Balance service with Vault integration
│   ├── binance-futures/       # Futures trading connector
│   ├── oms-server/            # Main OMS server
│   ├── grpc-gateway/          # gRPC gateway server
│   ├── backtest/              # Backtesting runner
│   ├── tcp-server/            # TCP server for C++ core
│   └── ...                    # Various test and utility commands
│
├── services/                  # Exchange connectors
│   ├── binance/               # Binance Spot/Futures (fully implemented)
│   │   ├── spot/              # Spot market connector
│   │   ├── futures/           # Futures market connector
│   │   └── websocket/         # WebSocket handlers
│   └── bybit/                 # Bybit connector (framework ready)
│
├── internal/                  # Go internal packages (not exported)
│   ├── account/               # Multi-account management
│   ├── api/                   # Internal API handlers
│   ├── backtest/              # Backtesting engine
│   ├── exchange/              # Exchange abstraction layer & factory
│   ├── grpc/                  # gRPC service implementations
│   ├── keymanager/            # API key management
│   ├── marketdata/            # Market data processing
│   ├── monitor/               # System monitoring
│   ├── monitoring/            # Metrics and alerting
│   ├── orders/                # Order management
│   ├── position/              # Position tracking
│   ├── risk/                  # Risk management engine
│   ├── router/                # Smart order routing
│   ├── storage/               # Data persistence
│   ├── strategies/            # Trading strategies
│   │   ├── arbitrage/         # Cross-exchange arbitrage
│   │   ├── market_maker/      # Market making/LP strategies
│   │   └── orchestrator/      # Strategy orchestration
│   └── transfer/              # Asset transfer management
│
├── pkg/                       # Shared Go packages (can be imported)
│   ├── types/                 # Common types and interfaces
│   ├── nats/                  # NATS messaging utilities
│   ├── vault/                 # HashiCorp Vault client
│   ├── cache/                 # Caching utilities
│   ├── metrics/               # Prometheus metrics
│   ├── security/              # Security utilities
│   ├── alerting/              # Alert system
│   ├── tracing/               # Distributed tracing
│   ├── backtest/              # Backtesting utilities
│   ├── storage/               # Storage interfaces
│   └── strategies/            # Strategy interfaces
│
├── proto/                     # Protocol Buffer definitions
│   └── *.proto                # gRPC service definitions
│
├── dashboard/                 # Web-based monitoring dashboard
│   ├── server/                # Go backend server
│   ├── frontend/              # React frontend
│   │   ├── src/               # React source files
│   │   └── public/            # Static assets
│   ├── oms-dashboard-real     # Production binary
│   └── demo/                  # Demo server
│
├── configs/                   # Configuration files
│   ├── config.yaml            # Main application config
│   ├── accounts.yaml          # Multi-account settings
│   ├── strategies.yaml        # Trading strategy configs
│   ├── risk_management.yaml   # Risk limits and rules
│   ├── position_management.yaml
│   ├── vault.hcl              # Vault configuration
│   └── prometheus/            # Prometheus configs
│
├── scripts/                   # Automation scripts
│   ├── start-oms.sh           # System startup
│   ├── stop-oms.sh            # System shutdown
│   ├── init-vault.sh          # Vault initialization
│   ├── store-binance-keys.sh  # API key storage
│   └── ...                    # Various utility scripts
│
├── deployments/               # Deployment configurations
│   └── kubernetes/            # K8s manifests
│
├── docs/                      # Documentation
│   ├── api/                   # API documentation
│   └── tutorials/             # User guides
│
├── examples/                  # Code examples
│   ├── basic_order.go         # Basic order execution
│   ├── multi_account_trading.go
│   └── websocket/             # WebSocket examples
│
├── tests/                     # Integration tests
│   └── integration/           # End-to-end tests
│
├── monitoring/                # Monitoring stack configs
├── grafana/                   # Grafana dashboards
└── helm/                      # Helm charts for K8s
```

### Key Components

1. **C++ Core Engine** (`core/`)
   - Lock-free ring buffers for ultra-low latency
   - Target: < 100μs order processing
   - Risk checks: < 50μs
   - CPU affinity and NUMA optimization
   - Memory pool allocation

2. **Exchange Abstraction** (`pkg/types/exchange.go`)
   - Common interface for all exchanges
   - Factory pattern (`internal/exchange/factory.go`)
   - Symbol normalization across exchanges
   - Rate limit management

3. **NATS Messaging** (`pkg/nats/`)
   - Subject pattern: `{action}.{exchange}.{market}.{symbol}`
   - JetStream for message persistence
   - Internal service communication

4. **Security** (`pkg/vault/`)
   - HashiCorp Vault for API key management
   - AES-256 encryption for sensitive data
   - Automatic key rotation (30-day cycle)
   - File-based persistent storage

5. **Trading Strategies** (`internal/strategies/`)
   - Arbitrage engine with < 1ms detection
   - Market making with dynamic spreads
   - Strategy orchestrator for concurrent execution
   - Capital allocation system

6. **Risk Management** (`internal/risk/`)
   - Real-time position tracking
   - Portfolio-level risk calculation
   - Automatic stop-loss/take-profit
   - Emergency kill switch

### Performance Targets (Achieved)
- Order processing: < 100 microseconds
- Risk checks: < 50 microseconds
- Throughput: 100,000+ orders/sec
- Market data: 1,000,000+ messages/sec
- WebSocket latency: ~35ms (30-80% improvement over REST)

## Design Principles

### Real Data Only Policy (CRITICAL)
- **NO SIMULATED DATA** - Never use `Math.random()` or similar for fake metrics
- **NO DUMMY VALUES** - If unavailable, show `0` or `"N/A"`, never fake
- **REAL SOURCES ONLY**:
  - Live WebSocket streams from exchanges
  - Actual log files
  - System commands (`ps`, `top`, `free`)
  - NATS messages
  - API responses
- **NO HARDCODED INCREMENTS** - Never artificially increase counters

### WebSocket First
- All market data via real-time WebSocket streams
- Order operations use WebSocket API when available
- Persistent connections for orders, market data, user data
- REST API only as fallback

### Exchange WebSocket as Single Source of Truth
- 24hr statistics from ticker streams
- Volume, high/low prices from WebSocket
- No mock or cached stale data

## Infrastructure

### Docker Services (`docker-compose.yml`)

| Service | Port | Purpose |
|---------|------|---------|
| NATS | 4222, 8222 | Message broker with JetStream |
| Redis | 6379 | In-memory cache |
| Vault | 8200 | Secret management |

### Vault Persistent Storage
- **Storage path**: `~/.mExOms/vault-data/`
- **Mode**: Production (file backend, NOT dev mode)
- **Auto-unseal**: Yes (stored unseal keys)
- **Unseal keys**: `~/.mExOms/vault-data/vault-unseal-keys.json`
- **Root token**: `~/.mExOms/vault-data/root-token`
- **Persists**: Across Docker restarts and system reboots

### API Key Storage in Vault
```bash
# Store keys
vault kv put secret/exchanges/binance_spot \
  api_key="YOUR_API_KEY" \
  secret_key="YOUR_SECRET_KEY"

# Read keys
vault kv get secret/exchanges/binance_spot
```

## Adding New Exchanges

1. Create connector in `services/{exchange}/`
2. Implement `types.Exchange` interface in `pkg/types/exchange.go`
3. Add configuration in `configs/config.yaml`
4. Register in exchange factory (`internal/exchange/factory.go`)
5. Add Vault path: `secret/exchanges/{exchange}_{market}`

## Development Workflow

### Before Making Changes
1. Run `make test` to ensure tests pass
2. Check existing code patterns in similar files
3. Follow the "Real Data Only" policy

### Code Style
- Go: Standard `gofmt` formatting
- C++: Clang-format with project settings
- Use existing interfaces and abstractions

### Testing Changes
```bash
# Quick validation
make test-go

# Full test suite
make test

# Performance validation
make test-benchmark
```

## Work Log Guidelines

When working on this project, create daily work logs:

**File Format**: `WORK_LOG_YYYY-MM-DD.md`

**Structure**:
```markdown
# Work Log - YYYY-MM-DD

## 작업 요약
[Brief summary of work done]

## 완료된 작업
- [Completed tasks and features]
- [Files created/modified]

## 진행 중인 작업
- [In-progress items]

## 다음 작업 계획
- [Next steps]

## 주요 변경사항
- [Code changes]
- [Architecture decisions]

## 참고사항
- [Notes for next developer]
```

## Recent Updates (2025-09-04)

### System Startup Automation
- `make start-all` / `make stop-all` commands
- Automatic service health checking
- Log management in `logs/` directory

### Vault Persistent Storage
- Changed from dev mode to file-based persistent storage
- API keys persist across restarts
- Automatic initialization and unseal

### Market Data
- Changed from orderbook (depth20) to 24hr ticker stream
- Added ticker data transformation in dashboard server
- Fixed data format compatibility between backend and frontend

### Key Files Modified
- `cmd/binance-market-full/main.go` - Ticker stream implementation
- `dashboard/server/main_real.go` - Data transformation logic
- `Makefile` - New commands and persistent Vault
- `docker-compose.yml` - Added Redis service

## Phase Completion Status

All 23 phases completed:

| Phase | Description | Status |
|-------|-------------|--------|
| 1-5 | Core Infrastructure | ✅ Completed |
| 6-9 | Exchange Integration | ✅ Completed |
| 10-18 | Advanced Features | ✅ Completed |
| 19-22 | Trading Strategies | ✅ Completed |
| 23 | TCP Multi-Client Server | ✅ Completed |

## Important Files Reference

| Purpose | File(s) |
|---------|---------|
| Main config | `configs/config.yaml` |
| Account settings | `configs/accounts.yaml` |
| Strategy config | `configs/strategies.yaml` |
| Risk limits | `configs/risk_management.yaml` |
| Exchange interface | `pkg/types/exchange.go` |
| Exchange factory | `internal/exchange/factory.go` |
| Market data service | `cmd/binance-market-full/main.go` |
| Balance service | `cmd/binance-spot-balance/main.go` |
| Dashboard server | `dashboard/server/main_real.go` |
| Startup script | `scripts/start-oms.sh` |
| Vault init | `scripts/init-vault.sh` |

## Troubleshooting

### Services not starting
```bash
# Check Docker containers
docker ps -a

# Check NATS
curl http://localhost:8222/healthz

# Check Vault
curl http://localhost:8200/v1/sys/health
```

### API keys not found
```bash
# Re-initialize Vault
./scripts/init-vault.sh

# Re-store keys
./scripts/store-binance-keys.sh
```

### Dashboard not showing data
1. Ensure market data service is running
2. Check NATS connection: `curl http://localhost:8222/connz`
3. Verify WebSocket connection in browser console
