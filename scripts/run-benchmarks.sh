#!/bin/bash

# Run performance benchmarks for mExOms

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_ROOT"

echo "=== mExOms Performance Benchmarks ==="
echo "Date: $(date)"
echo "======================================"
echo

# Create benchmark results directory
RESULTS_DIR="benchmark-results"
mkdir -p "$RESULTS_DIR"

# Generate timestamp for results
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
RESULT_FILE="$RESULTS_DIR/benchmark_${TIMESTAMP}.txt"

# Function to run benchmarks
run_benchmark() {
    local name=$1
    local package=$2
    local pattern=$3
    
    echo "Running $name benchmarks..."
    go test -bench="$pattern" -benchmem -benchtime=10s "$package" 2>&1 | tee -a "$RESULT_FILE"
    echo
}

# Run all benchmarks
echo "Running all benchmarks (this may take a few minutes)..."
echo

# Order processing benchmarks
run_benchmark "Order Processing" "./benchmark" "BenchmarkOrder"

# Risk management benchmarks
run_benchmark "Risk Management" "./benchmark" "BenchmarkRisk"

# Smart routing benchmarks
run_benchmark "Smart Routing" "./benchmark" "BenchmarkSmart|BenchmarkFee|BenchmarkArbitrage"

# Market data benchmarks
run_benchmark "Market Data" "./benchmark" "BenchmarkTicker|BenchmarkOrderBook|BenchmarkTrade|BenchmarkKline|BenchmarkMarket|BenchmarkDepth"

# Decimal operations benchmarks
run_benchmark "Decimal Operations" "./benchmark" "BenchmarkDecimal"

echo "======================================"
echo "Benchmark Summary"
echo "======================================"

# Extract key metrics
echo
echo "Top Performance Metrics:"
grep -E "ns/op|orders/sec|updates/sec|checks/sec|routes/sec" "$RESULT_FILE" | sort -k2 -n | head -20

echo
echo "Memory Usage Summary:"
grep -E "B/op|allocs/op" "$RESULT_FILE" | sort -k2 -n | head -20

echo
echo "Results saved to: $RESULT_FILE"

# Optional: Generate comparison with baseline
if [ -f "$RESULTS_DIR/baseline.txt" ]; then
    echo
    echo "Comparing with baseline..."
    # Use benchstat if available
    if command -v benchstat &> /dev/null; then
        benchstat "$RESULTS_DIR/baseline.txt" "$RESULT_FILE"
    else
        echo "Install benchstat for detailed comparison: go install golang.org/x/perf/cmd/benchstat@latest"
    fi
fi

echo
echo "=== Benchmark Complete ===