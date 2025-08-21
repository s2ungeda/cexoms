# Testing Guide for mExOms

This guide provides comprehensive information about testing the Multi-Exchange Order Management System.

## Table of Contents
1. [Overview](#overview)
2. [Test Structure](#test-structure)
3. [Running Tests](#running-tests)
4. [Integration Tests](#integration-tests)
5. [Performance Benchmarks](#performance-benchmarks)
6. [Test Data Generators](#test-data-generators)
7. [Continuous Integration](#continuous-integration)
8. [Troubleshooting](#troubleshooting)

## Overview

The mExOms testing framework consists of:
- Unit tests for individual components
- Integration tests for end-to-end scenarios
- Performance benchmarks for critical paths
- Test data generators for realistic scenarios
- Stress tests for high-load conditions

## Test Structure

```
mExOms/
├── test/
│   ├── integration/         # Integration test suites
│   │   ├── binance_spot_test.go
│   │   ├── risk_management_test.go
│   │   ├── smart_router_test.go
│   │   └── suite_test.go
│   └── generators/         # Test data generators
│       ├── market_data.go
│       ├── order_data.go
│       ├── account_data.go
│       └── scenario_data.go
├── benchmark/              # Performance benchmarks
│   ├── order_benchmark_test.go
│   ├── risk_benchmark_test.go
│   ├── router_benchmark_test.go
│   ├── market_data_benchmark_test.go
│   └── decimal_benchmark_test.go
└── cmd/test-*/            # Test utilities and examples
```

## Running Tests

### Unit Tests

Run all unit tests:
```bash
go test -v ./...
```

Run tests for a specific package:
```bash
go test -v ./internal/risk/...
```

Run with race detection:
```bash
go test -race -v ./...
```

Run with coverage:
```bash
go test -cover -v ./...
```

Generate coverage report:
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### Integration Tests

Integration tests require the `-integration` flag:

```bash
# Run all integration tests
go test -v -integration ./test/integration/...

# Run specific integration test
go test -v -integration -run TestBinanceSpotIntegration ./test/integration/

# Run with real exchange connections (requires API keys)
export BINANCE_API_KEY="your_key"
export BINANCE_API_SECRET="your_secret"
go test -v -integration -live ./test/integration/
```

### Skip Integration Tests

For quick unit test runs:
```bash
go test -short ./...
```

## Integration Tests

### 1. Binance Spot Integration

Tests the complete Binance Spot exchange integration:

```go
// test/integration/binance_spot_test.go
func TestBinanceSpotIntegration(t *testing.T) {
    // Tests market data subscriptions
    // Tests order placement and cancellation
    // Tests account information retrieval
}
```

**Prerequisites:**
- NATS server running (`make run-nats`)
- Redis server running (`make run-redis`)
- Valid API keys for live testing

### 2. Risk Management Integration

Tests risk management components working together:

```go
// test/integration/risk_management_test.go
func TestRiskManagementIntegration(t *testing.T) {
    // Tests position sizing
    // Tests risk limit enforcement
    // Tests stop loss management
    // Tests real-time monitoring
}
```

### 3. Smart Router Integration

Tests order routing and optimization:

```go
// test/integration/smart_router_test.go
func TestSmartRouterIntegration(t *testing.T) {
    // Tests order splitting
    // Tests fee optimization
    // Tests arbitrage detection
    // Tests concurrent routing
}
```

## Performance Benchmarks

### Running Benchmarks

Use the provided script:
```bash
./scripts/run-benchmarks.sh
```

Or run manually:
```bash
# Run all benchmarks
go test -bench=. ./benchmark/...

# Run specific benchmark
go test -bench=BenchmarkOrderPlacement ./benchmark/

# Run with memory profiling
go test -bench=. -benchmem ./benchmark/...

# Run for longer duration (more accurate)
go test -bench=. -benchtime=30s ./benchmark/...
```

### Key Benchmarks

1. **Order Processing**: Target < 100μs
   ```bash
   go test -bench=BenchmarkOrderPlacement ./benchmark/
   ```

2. **Risk Checks**: Target < 50μs
   ```bash
   go test -bench=BenchmarkRiskCheck ./benchmark/
   ```

3. **Market Data**: Target < 10μs
   ```bash
   go test -bench=BenchmarkTickerProcessing ./benchmark/
   ```

### Profiling

Generate CPU profile:
```bash
go test -bench=BenchmarkOrderPlacement -cpuprofile=cpu.prof ./benchmark/
go tool pprof -http=:8080 cpu.prof
```

Generate memory profile:
```bash
go test -bench=BenchmarkOrderPlacement -memprofile=mem.prof ./benchmark/
go tool pprof -http=:8080 mem.prof
```

## Test Data Generators

### Market Data Generator

Generate realistic market data:

```go
import "github.com/mExOms/test/generators"

// Create generator
gen := generators.NewMarketDataGenerator(seed)

// Generate ticker
ticker := gen.GenerateTicker("BTCUSDT")

// Generate order book
orderBook := gen.GenerateOrderBook("BTCUSDT", 100)

// Generate historical klines
klines := gen.GenerateHistoricalKlines("BTCUSDT", "1m", 1000)
```

### Order Generator

Generate various order types:

```go
gen := generators.NewOrderGenerator(seed)

// Generate market order
marketOrder := gen.GenerateMarketOrder("BTCUSDT", types.OrderSideBuy)

// Generate limit order
limitOrder := gen.GenerateLimitOrder("BTCUSDT", types.OrderSideSell, 0.01)

// Generate order batch
orders := gen.GenerateOrderBatch("BTCUSDT", 100)
```

### Scenario Generator

Generate complete market scenarios:

```go
gen := generators.NewScenarioGenerator(seed)

// Set scenario
gen.SetScenario(generators.ScenarioBullRun)

// Generate market movement
updates := gen.GenerateMarketMovement("BTCUSDT", time.Hour, time.Second)

// Generate trading session
session := gen.GenerateTradingSession("BTCUSDT", time.Hour)

// Generate stress test data
stressData := gen.GenerateStressTestData()
```

### Available Scenarios

- `ScenarioNormal`: Normal market conditions
- `ScenarioBullRun`: Upward trending market
- `ScenarioBearMarket`: Downward trending market
- `ScenarioHighVolatile`: High volatility conditions
- `ScenarioFlashCrash`: Sudden price drops
- `ScenarioSideways`: Range-bound market

## Continuous Integration

### GitHub Actions Example

```yaml
name: Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    
    services:
      nats:
        image: nats:latest
        ports:
          - 4222:4222
      
      redis:
        image: redis:latest
        ports:
          - 6379:6379
    
    steps:
    - uses: actions/checkout@v3
    
    - uses: actions/setup-go@v4
      with:
        go-version: '1.21'
    
    - name: Install dependencies
      run: make install-deps
    
    - name: Run unit tests
      run: go test -v -race ./...
    
    - name: Run integration tests
      run: go test -v -integration ./test/integration/...
    
    - name: Run benchmarks
      run: |
        go test -bench=. -benchmem -benchtime=10s ./benchmark/... | tee benchmark.txt
        # Optional: Compare with baseline
    
    - name: Upload coverage
      run: |
        go test -coverprofile=coverage.out ./...
        # Upload to coverage service
```

## Troubleshooting

### Common Issues

1. **Integration tests failing**
   - Ensure infrastructure services are running
   - Check network connectivity
   - Verify API credentials

2. **Benchmarks showing high variance**
   - Run with longer duration: `-benchtime=30s`
   - Ensure system is not under load
   - Disable CPU frequency scaling

3. **Race conditions detected**
   - Always test with `-race` flag during development
   - Review concurrent access patterns
   - Use proper synchronization

### Debug Flags

```bash
# Verbose output
go test -v ./...

# Show test coverage per function
go test -cover -coverprofile=c.out ./...
go tool cover -func=c.out

# Run specific test with debugging
go test -v -run TestSpecificFunction ./package/

# Set custom timeout
go test -timeout 30m ./...
```

### Environment Variables

```bash
# API Keys (for live testing)
export BINANCE_API_KEY="your_key"
export BINANCE_API_SECRET="your_secret"

# Test configuration
export TEST_INTEGRATION=true
export TEST_LIVE_TRADING=false
export TEST_LOG_LEVEL=debug

# Infrastructure endpoints
export NATS_URL="nats://localhost:4222"
export REDIS_URL="redis://localhost:6379"
```

## Best Practices

1. **Test Isolation**
   - Each test should be independent
   - Clean up resources after tests
   - Use fresh test data

2. **Realistic Data**
   - Use generators for consistent test data
   - Test edge cases and error conditions
   - Include market scenario testing

3. **Performance Testing**
   - Run benchmarks regularly
   - Monitor for performance regressions
   - Profile hot paths

4. **Integration Testing**
   - Test complete workflows
   - Verify error handling
   - Test timeout and retry logic

5. **Continuous Improvement**
   - Add tests for bug fixes
   - Increase coverage incrementally
   - Review and refactor tests regularly

## Adding New Tests

### Unit Test Template

```go
func TestNewFeature(t *testing.T) {
    // Arrange
    // ... setup test data
    
    // Act
    // ... execute function
    
    // Assert
    assert.Equal(t, expected, actual)
}
```

### Integration Test Template

```go
func TestFeatureIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }
    
    // Setup
    ctx := context.Background()
    // ... initialize components
    
    // Test scenarios
    t.Run("Scenario1", func(t *testing.T) {
        // ... test code
    })
    
    // Cleanup
    // ... cleanup resources
}
```

### Benchmark Template

```go
func BenchmarkFeature(b *testing.B) {
    // Setup
    // ... prepare data
    
    b.ResetTimer()
    
    for i := 0; i < b.N; i++ {
        // ... code to benchmark
    }
    
    b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "ops/sec")
}
```