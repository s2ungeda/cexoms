# Performance Benchmarks

This directory contains performance benchmarks for the mExOms trading system.

## Running Benchmarks

### Run all benchmarks:
```bash
go test -bench=. ./benchmark/...
```

### Run specific benchmark:
```bash
go test -bench=BenchmarkOrderPlacement ./benchmark/
```

### Run with memory allocation stats:
```bash
go test -bench=. -benchmem ./benchmark/...
```

### Run with custom duration:
```bash
go test -bench=. -benchtime=30s ./benchmark/...
```

## Benchmark Categories

### 1. Order Processing (`order_benchmark_test.go`)
- **BenchmarkOrderPlacement**: Single order placement performance
- **BenchmarkOrderPlacementParallel**: Concurrent order placement
- **BenchmarkOrderBookUpdate**: Order book update processing

### 2. Risk Management (`risk_benchmark_test.go`)
- **BenchmarkRiskCheck**: Pre-trade risk validation
- **BenchmarkPositionSizing**: Position size calculation
- **BenchmarkRiskMetrics**: Risk metrics computation
- **BenchmarkLimitCheck**: Risk limit validation
- **BenchmarkStopLossCalculation**: Stop loss price calculation
- **BenchmarkRiskMonitoring**: Real-time risk monitoring

### 3. Smart Routing (`router_benchmark_test.go`)
- **BenchmarkSmartRouting**: Order routing decision making
- **BenchmarkOrderSplitting**: Large order splitting
- **BenchmarkFeeOptimization**: Fee optimization calculations
- **BenchmarkArbitrageDetection**: Arbitrage opportunity scanning
- **BenchmarkConcurrentRouting**: Parallel order routing

### 4. Market Data (`market_data_benchmark_test.go`)
- **BenchmarkTickerProcessing**: Ticker update processing
- **BenchmarkOrderBookProcessing**: Full order book updates
- **BenchmarkTradeProcessing**: Trade stream processing
- **BenchmarkKlineProcessing**: Candlestick data processing
- **BenchmarkMarketDataAggregation**: Multi-exchange data aggregation
- **BenchmarkDepthCalculation**: Market depth analysis

### 5. Decimal Operations (`decimal_benchmark_test.go`)
- **BenchmarkDecimalArithmetic**: Basic arithmetic operations
- **BenchmarkDecimalComparison**: Comparison operations
- **BenchmarkDecimalCreation**: Object creation performance
- **BenchmarkDecimalVsFloat64**: Decimal vs native float comparison
- **BenchmarkPriceCalculations**: Common trading calculations

## Performance Targets

Based on the system requirements:

| Operation | Target | Current |
|-----------|--------|---------|
| Order Processing | < 100μs | TBD |
| Risk Check | < 50μs | TBD |
| Market Data Update | < 10μs | TBD |
| Order Routing | < 200μs | TBD |

## Optimization Tips

1. **Profile CPU usage**:
   ```bash
   go test -bench=BenchmarkOrderPlacement -cpuprofile=cpu.prof
   go tool pprof cpu.prof
   ```

2. **Profile memory usage**:
   ```bash
   go test -bench=BenchmarkOrderPlacement -memprofile=mem.prof
   go tool pprof mem.prof
   ```

3. **Trace execution**:
   ```bash
   go test -bench=BenchmarkOrderPlacement -trace=trace.out
   go tool trace trace.out
   ```

## Continuous Benchmarking

Consider integrating these benchmarks into CI/CD pipeline to track performance regressions:

```yaml
# Example GitHub Actions workflow
- name: Run Benchmarks
  run: |
    go test -bench=. -benchmem ./benchmark/... | tee benchmark.txt
    # Compare with baseline or previous results
```

## Notes

- Benchmarks use mock implementations to isolate component performance
- Real-world performance may vary based on network latency and exchange APIs
- Consider running benchmarks on production-like hardware for accurate results
- Use `-benchtime=10s` or higher for more stable results