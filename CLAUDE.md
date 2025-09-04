# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Multi-Exchange Cryptocurrency Order Management System (OMS) - A high-performance trading system with C++ core engine and Go service layer, designed for ultra-low latency order processing across multiple cryptocurrency exchanges.

## Common Development Commands

### Quick Start Commands
```bash
# Start entire OMS system with one command
make start-all

# Stop all services
make stop-all

# Start only infrastructure
make infra-up

# Stop infrastructure
make infra-down
```

### Build Commands
```bash
# Install dependencies
make install-deps

# Build entire project (C++ core + Go services)
make build

# Build only C++ core engine
make build-core

# Build only Go services
make build-services

# Generate protobuf files
make proto
```

### Test Commands
```bash
# Run all tests
make test

# Run performance benchmarks
make test-benchmark

# Run only Go tests
go test -v -race ./...

# Run only C++ tests (when implemented)
./bin/core-tests
```

### Infrastructure Commands
```bash
# Start all infrastructure services
docker-compose up -d

# Start individual services
make run-nats      # Message broker (with JetStream)
make run-redis     # Cache (in-memory)
make run-vault     # Secret management (persistent storage)

# Stop all services
docker-compose down

# Initialize Vault (first time only)
./scripts/init-vault.sh

# Store API keys in Vault
./scripts/store-binance-keys.sh
```

### Service Commands
```bash
# Start market data service (no API key required)
go run cmd/binance-market-full/main.go

# Start balance service (requires API keys)
go run cmd/binance-spot-balance/main.go

# Start dashboard server
cd dashboard && ./oms-dashboard-real

# Start frontend (development mode with hot-reload)
cd dashboard/frontend && npm start
```

### Development Commands
```bash
# Format code
make fmt

# Lint code
make lint

# Clean build artifacts
make clean

# Check system status
ps aux | grep -E "binance|dashboard" | grep -v grep

# View logs
tail -f logs/market.log
tail -f logs/balance.log
tail -f logs/dashboard.log
```

## Architecture Overview

### Directory Structure
- `core/` - C++20 high-performance engine
  - `engine/` - Core order processing logic
  - `include/` - Header files (types.h, ring_buffer.h)
  - `lib/` - Static libraries
  - `tests/` - C++ unit tests

- `services/` - Go exchange connectors
  - `binance/` - Binance Spot/Futures connectors
  - `bybit/`, `okx/`, `upbit/` - Future exchange connectors

- `internal/` - Go internal packages
  - `exchange/` - Exchange abstraction layer and factory
  - `orders/` - Order management
  - `risk/` - Risk management
  - `router/` - Smart order routing

- `pkg/` - Shared Go packages
  - `types/` - Common types and interfaces
  - `nats/` - NATS messaging utilities
  - `utils/` - Helper functions

- `proto/` - Protocol Buffer definitions for gRPC

### Key Components

1. **C++ Core Engine** (core/)
   - Lock-free ring buffers for ultra-low latency
   - Target: < 100μs order processing
   - CPU affinity for performance optimization

2. **Exchange Abstraction** (pkg/types/exchange.go)
   - Common interface for all exchanges
   - Factory pattern for exchange creation
   - Symbol normalization across exchanges

3. **NATS Messaging**
   - Subject pattern: `{action}.{exchange}.{market}.{symbol}`
   - JetStream for message persistence
   - Internal service communication

4. **Security**
   - HashiCorp Vault for API key management
   - AES-256 encryption for sensitive data
   - Key rotation every 30 days

### Performance Targets
- Order processing: < 100 microseconds
- Risk checks: < 50 microseconds
- Throughput: 100,000+ orders/sec
- Market data: 1,000,000+ messages/sec

### Design Principles
- **No Mock Data**: All market data must come from real-time WebSocket streams
- **Single Source of Truth**: Exchange WebSocket streams are the only data source
- **Real-time Only**: 24hr statistics, volume, high/low prices all from WebSocket ticker streams
- **WebSocket First**: All order operations must use WebSocket API when available
- **Low Latency**: Persistent WebSocket connections for orders, market data, and user data
- **REST as Fallback**: REST API only for initialization and when WebSocket is unavailable

### CRITICAL: Real Data Only Policy
- **NO SIMULATED DATA** - Never use Math.random() or similar to generate fake metrics
- **NO DUMMY VALUES** - If data is unavailable, show 0 or "N/A", never fake values
- **REAL SOURCES ONLY** - Data must come from:
  - Live WebSocket streams
  - Actual log files
  - System commands (ps, top, free)
  - NATS messages
  - API responses
- **NO HARDCODED INCREMENTS** - Never artificially increase counters or metrics

### Adding New Exchanges

To add a new exchange:
1. Create connector in `services/{exchange}/`
2. Implement `types.Exchange` interface
3. Add configuration in `configs/config.yaml`
4. Register in exchange factory (`internal/exchange/factory.go`)
5. Add Vault path for API keys: `secret/exchanges/{exchange}_{market}`

### Development Phase Reference

The project follows the 18-phase development plan in `oms-guide.md`:
- Phase 1-4: Core infrastructure (completed)
- Phase 5-6: Binance connectors (next)
- Phase 7-10: Advanced features
- Phase 11-18: Production readiness

### Work Log Guidelines

When working on this project, create daily work logs following these rules:

1. **File Format**: `WORK_LOG_YYYY-MM-DD.md` (e.g., WORK_LOG_2025-08-26.md)

2. **Log Structure**:
```markdown
# Work Log - YYYY-MM-DD

## 작업 요약
[오늘 작업한 내용의 간략한 요약]

## 완료된 작업
- [완료된 Phase 및 주요 기능]
- [구현된 파일 목록]
- [해결된 이슈]

## 진행 중인 작업
- [현재 작업 중인 내용]
- [완료율 %]
- [예상 완료 시점]

## 다음 작업 계획
- [다음에 진행할 Phase]
- [구현해야 할 기능]
- [우선순위]

## Phase 진행 현황
- 전체: X/22 완료
- [각 Phase별 상태]

## 주요 변경사항
- [코드 변경사항]
- [아키텍처 결정사항]
- [발견된 이슈]

## 참고사항
- [다음 작업자를 위한 메모]
- [주의사항]
```

3. **Important Rules**:
- Always reference previous work logs for continuity
- Track incomplete tasks for next session
- Accurately record Phase progress (total 23 phases - all completed)
- Include all created/modified files
- Note any architectural decisions or issues

## Recent Updates (2025-09-04)

### System Startup Automation
- Added `make start-all` and `make stop-all` commands
- Created `scripts/start-oms.sh` and `scripts/stop-oms.sh`
- Automatic service health checking and log management

### Vault Persistent Storage
- Vault now uses file-based persistent storage at `~/.mExOms/vault-data/`
- Automatic initialization and unseal on startup
- API keys persist across system restarts

### Market Data Changes
- Changed from orderbook (depth20) to 24hr ticker stream
- Added ticker data transformation in dashboard server
- Fixed data format compatibility between backend and frontend

### Key Files Modified
- `cmd/binance-market-full/main.go` - Ticker stream implementation
- `dashboard/server/main_real.go` - Data transformation logic
- `Makefile` - New commands and persistent Vault
- `docker-compose.yml` - Added Redis service
- Various scripts in `scripts/` directory