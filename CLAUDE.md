# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Multi-Exchange Cryptocurrency Order Management System (OMS) - A high-performance trading system with C++ core engine and Go service layer, designed for ultra-low latency order processing across multiple cryptocurrency exchanges.

## Common Development Commands

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
make run-nats      # Message broker
make run-redis     # Cache
make run-postgres  # Database
make run-vault     # Secret management

# Stop all services
docker-compose down
```

### Development Commands
```bash
# Format code
make fmt

# Lint code
make lint

# Clean build artifacts
make clean
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
- **Single Source of Truth**: Binance WebSocket streams are the only data source
- **Real-time Only**: 24hr statistics, volume, high/low prices all from WebSocket ticker streams

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