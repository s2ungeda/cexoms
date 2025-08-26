# Performance Optimization Components

This package provides high-performance utilities for Go services in the mExOms system.

## Components

### 1. Connection Pool (`connection_pool.go`)
- Database and Redis connection pooling with health checks
- Automatic reconnection on failure
- Connection metrics and monitoring
- Retry logic for transient failures

Usage:
```go
dbConfig := performance.DatabaseConfig{
    Host:         "localhost",
    Port:         5432,
    MaxOpenConns: 100,
    // ...
}

pool, err := performance.NewConnectionPool(dbConfig, redisConfig)
defer pool.Close()

// Execute with retry
err = pool.ExecuteWithRetry(ctx, func(db *sql.DB) error {
    // Database operations
}, 3)
```

### 2. Buffer Pool (`buffer_pool.go`)
- Reusable byte buffers to reduce GC pressure
- Multiple size buckets for efficiency
- Thread-safe buffer management
- Significant memory allocation reduction

Usage:
```go
pool := performance.NewBufferPool()

buf := pool.Get(1024) // Get buffer with at least 1KB capacity
defer pool.Put(buf)   // Return to pool when done

buf.Write([]byte("data"))
```

### 3. Concurrent Map (`concurrent_map.go`)
- High-performance thread-safe map using sharding
- Reduces lock contention compared to sync.Map
- Batch operations for efficiency
- Concurrent set implementation included

Usage:
```go
m := performance.NewConcurrentMap[string, int](32) // 32 shards

m.Set("key", 100)
value, exists := m.Get("key")

// Batch operations
items := map[string]int{"k1": 1, "k2": 2}
m.BatchSet(items)
```

### 4. Batch Processor (`batch_processor.go`)
- Automatic batching of operations
- Configurable batch size and delay
- Concurrent processing with worker pool
- Dynamic batch sizing based on latency
- Priority-based processing

Usage:
```go
config := performance.BatchProcessorConfig{
    MaxBatchSize:   100,
    MaxBatchDelay:  50 * time.Millisecond,
    MaxConcurrency: 10,
}

processor := performance.NewBatchProcessor(config, processFn, errorHandler)
defer processor.Stop(ctx)

// Add items - they'll be automatically batched
processor.Add(item)
```

### 5. Zero-Allocation JSON (`zero_alloc_json.go`)
- JSON encoding/decoding with minimal allocations
- Reusable writers and object pools
- Unsafe string conversions for performance
- Streaming JSON writer for large datasets

Usage:
```go
pool := performance.NewBufferPool()
encoder := performance.NewFastJSONEncoder(pool)

// Zero-allocation encoding
data := encoder.EncodeOrder(order)

// Manual JSON building
writer := performance.NewJSONWriter(pool)
defer writer.Release()

writer.WriteObjectStart()
writer.WriteKey("id")
writer.WriteString("123")
writer.WriteObjectEnd()
```

## Performance Improvements

Based on benchmarks in `cmd/test-go-performance/main.go`:

- **Buffer Pool**: ~3-5x faster than regular allocation, 80-90% memory reduction
- **Concurrent Map**: ~2-3x faster than sync.Map, ~5-10x faster than mutex-protected map
- **Batch Processor**: Processes 100k+ items/sec with configurable concurrency
- **Zero-Alloc JSON**: ~4-6x faster than standard json.Marshal, 95% memory reduction
- **Connection Pool**: Handles 10k+ concurrent operations with health monitoring

## Best Practices

1. **Buffer Pool**
   - Always return buffers to the pool
   - Use appropriate size hints
   - Clear sensitive data before returning

2. **Concurrent Map**
   - Choose shard count based on expected concurrency (power of 2)
   - Use batch operations when possible
   - Consider memory vs performance trade-offs

3. **Batch Processor**
   - Tune batch size and delay for your workload
   - Monitor queue size to prevent overload
   - Use appropriate error handling

4. **Connection Pool**
   - Configure health check intervals appropriately
   - Set reasonable connection limits
   - Monitor metrics for capacity planning

5. **Zero-Alloc JSON**
   - Reuse writers and encoders
   - Be careful with unsafe string conversions
   - Profile to ensure allocations are actually reduced

## Integration Example

```go
// In your service initialization
bufferPool := performance.NewBufferPool()
connPool, _ := performance.NewConnectionPool(dbConfig, redisConfig)
concurrentCache := performance.NewConcurrentMap[string, interface{}](64)

// In your order processing
batchProcessor := performance.NewBatchProcessor(
    performance.DefaultBatchConfig(),
    func(ctx context.Context, orders []Order) error {
        // Process batch of orders
        return connPool.ExecuteWithRetry(ctx, func(db *sql.DB) error {
            // Bulk insert orders
            return bulkInsertOrders(db, orders)
        }, 3)
    },
    nil,
)

// In your API handlers
encoder := performance.NewFastJSONEncoder(bufferPool)
responseData := encoder.EncodeOrder(order)
```

## Monitoring

All components provide metrics that can be integrated with Prometheus:

```go
metrics := connPool.GetMetrics()
prometheus.NewGaugeVec(prometheus.GaugeOpts{
    Name: "connection_pool_active",
}, []string{"type"}).WithLabelValues("db").Set(float64(metrics.DBConnActive))
```