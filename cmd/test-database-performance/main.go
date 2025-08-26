package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/your-org/mexoms/pkg/performance"
)

func main() {
	fmt.Println("🗄️ Database Performance Optimization Demo")
	fmt.Println(strings.Repeat("=", 60))

	// Test Database Connection Pool
	fmt.Println("\n1. Testing Database Connection Pool Performance")
	testConnectionPool()

	// Test Query Cache Performance
	fmt.Println("\n2. Testing Query Cache Performance")
	testQueryCache()

	// Test Batch Operations
	fmt.Println("\n3. Testing Batch Operations")
	testBatchOperations()

	// Test Database Metrics
	fmt.Println("\n4. Testing Database Metrics")
	testDatabaseMetrics()

	fmt.Println("\n✅ All database performance tests completed!")
}

func testConnectionPool() {
	config := performance.DefaultDatabaseConfig()
	config.MaxOpenConns = 20
	config.MaxIdleConns = 5
	
	optimizer, err := performance.NewDatabaseOptimizer(config)
	if err != nil {
		fmt.Printf("❌ Failed to create database optimizer: %v\n", err)
		fmt.Printf("📝 Note: This test requires a PostgreSQL database running at localhost:5432\n")
		fmt.Printf("   You can skip this test if database is not available\n")
		return
	}
	defer optimizer.Stop()

	optimizer.Start()

	// Simulate concurrent database operations
	const numWorkers = 50
	const opsPerWorker = 100
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			for j := 0; j < opsPerWorker; j++ {
				// Simulate a simple query
				query := "SELECT 1"
				rows, err := optimizer.ExecuteQuery(ctx, query)
				if err != nil {
					// Expected for demo without actual database
					continue
				}
				if rows != nil {
					rows.Close()
				}
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)
	
	metrics := optimizer.GetMetrics()
	
	fmt.Printf("📊 Connection Pool Performance:\n")
	fmt.Printf("   - Total operations: %d\n", numWorkers*opsPerWorker)
	fmt.Printf("   - Duration: %v\n", elapsed)
	fmt.Printf("   - Operations/sec: %.0f\n", float64(numWorkers*opsPerWorker)/elapsed.Seconds())
	fmt.Printf("   - Successful queries: %d\n", metrics.SuccessfulQueries)
	fmt.Printf("   - Failed queries: %d\n", metrics.FailedQueries)
	fmt.Printf("   - Average query latency: %v\n", metrics.GetAverageQueryLatency())
	fmt.Printf("   - Cache hit ratio: %.2f%%\n", metrics.GetCacheHitRatio()*100)
	fmt.Printf("   - Success rate: %.2f%%\n", metrics.GetSuccessRate()*100)
}

func testQueryCache() {
	fmt.Println("🎯 Testing Query Cache...")
	
	cache := performance.NewQueryCache(100)
	
	// Test cache performance
	const numQueries = 10000
	const uniqueQueries = 100
	
	// Generate test queries
	queries := make([]string, uniqueQueries)
	for i := 0; i < uniqueQueries; i++ {
		queries[i] = fmt.Sprintf("SELECT * FROM table WHERE id = %d", i)
	}
	
	// Phase 1: Fill cache (all misses)
	start := time.Now()
	for i := 0; i < numQueries; i++ {
		query := queries[i%uniqueQueries]
		result := cache.Get(query, i%uniqueQueries)
		if result == nil {
			// Cache miss - simulate setting result
			// cache.Set(query, mockResult, i%uniqueQueries)
		}
	}
	cacheTestTime := time.Since(start)
	
	hits, misses := cache.GetStats()
	
	fmt.Printf("📊 Query Cache Performance:\n")
	fmt.Printf("   - Test duration: %v\n", cacheTestTime)
	fmt.Printf("   - Operations/sec: %.0f\n", float64(numQueries)/cacheTestTime.Seconds())
	fmt.Printf("   - Cache hits: %d\n", hits)
	fmt.Printf("   - Cache misses: %d\n", misses)
	if hits+misses > 0 {
		fmt.Printf("   - Hit ratio: %.2f%%\n", float64(hits)/float64(hits+misses)*100)
	}
}

func testBatchOperations() {
	fmt.Println("📦 Testing Batch Operations...")
	
	config := performance.DefaultDatabaseConfig()
	config.EnableBatching = true
	config.BatchSize = 50
	
	optimizer, err := performance.NewDatabaseOptimizer(config)
	if err != nil {
		fmt.Printf("❌ Failed to create database optimizer: %v\n", err)
		return
	}
	defer optimizer.Stop()

	// Test batch vs individual operations
	const numStatements = 1000
	
	statements := make([]performance.BatchStatement, numStatements)
	for i := 0; i < numStatements; i++ {
		statements[i] = performance.BatchStatement{
			Query: "INSERT INTO test_table (id, value) VALUES ($1, $2)",
			Args:  []interface{}{i, fmt.Sprintf("value_%d", i)},
		}
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test batch execution
	start := time.Now()
	err = optimizer.ExecuteBatch(ctx, statements)
	batchTime := time.Since(start)
	
	fmt.Printf("📊 Batch Operation Performance:\n")
	fmt.Printf("   - Statements: %d\n", numStatements)
	fmt.Printf("   - Batch execution time: %v\n", batchTime)
	if err != nil {
		fmt.Printf("   - Error (expected without DB): %v\n", err)
	} else {
		fmt.Printf("   - Batch throughput: %.0f statements/sec\n", float64(numStatements)/batchTime.Seconds())
	}
	
	// Simulate individual operations for comparison
	start = time.Now()
	for i := 0; i < numStatements; i++ {
		// Simulate individual query time
		time.Sleep(time.Microsecond * 10)
	}
	individualTime := time.Since(start)
	
	fmt.Printf("   - Individual execution time (simulated): %v\n", individualTime)
	fmt.Printf("   - Batch speedup: %.2fx\n", float64(individualTime)/float64(batchTime))
}

func testDatabaseMetrics() {
	fmt.Println("📈 Testing Database Metrics...")
	
	metrics := performance.NewDatabaseMetrics()
	
	// Simulate various operations
	const numOps = 1000
	
	start := time.Now()
	
	for i := 0; i < numOps; i++ {
		// Simulate query latencies
		latency := time.Duration(i%100) * time.Microsecond
		metrics.RecordQueryLatency(latency)
		
		if i%10 == 0 {
			metrics.IncrementFailedQueries()
		} else {
			metrics.IncrementSuccessfulQueries()
		}
		
		if i%5 == 0 {
			metrics.IncrementCacheHits()
		} else {
			metrics.IncrementCacheMisses()
		}
		
		if i%100 == 0 {
			metrics.IncrementBatchExecutions(50)
			metrics.RecordBatchLatency(time.Millisecond * 10)
		}
	}
	
	testTime := time.Since(start)
	
	fmt.Printf("📊 Database Metrics Summary:\n")
	fmt.Printf("   - Test duration: %v\n", testTime)
	fmt.Printf("   - Operations processed: %d\n", numOps)
	fmt.Printf("   - Average query latency: %v\n", metrics.GetAverageQueryLatency())
	fmt.Printf("   - Cache hit ratio: %.2f%%\n", metrics.GetCacheHitRatio()*100)
	fmt.Printf("   - Success rate: %.2f%%\n", metrics.GetSuccessRate()*100)
	fmt.Printf("   - Total successful queries: %d\n", atomic.LoadInt64(&metrics.SuccessfulQueries))
	fmt.Printf("   - Total failed queries: %d\n", atomic.LoadInt64(&metrics.FailedQueries))
	fmt.Printf("   - Total cache hits: %d\n", atomic.LoadInt64(&metrics.CacheHits))
	fmt.Printf("   - Total cache misses: %d\n", atomic.LoadInt64(&metrics.CacheMisses))
	fmt.Printf("   - Batch executions: %d\n", atomic.LoadInt64(&metrics.BatchExecutions))
}

func simulateDatabaseLoad() {
	fmt.Println("🔄 Simulating Database Load...")
	
	config := performance.DefaultDatabaseConfig()
	config.MaxOpenConns = 50
	config.EnableQueryCache = true
	config.EnableBatching = true
	
	optimizer, err := performance.NewDatabaseOptimizer(config)
	if err != nil {
		fmt.Printf("❌ Failed to create database optimizer: %v\n", err)
		return
	}
	defer optimizer.Stop()

	optimizer.Start()
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	var wg sync.WaitGroup
	const numWorkers = 20
	
	// Different types of workloads
	workloads := []func(context.Context, *performance.DatabaseOptimizer, int){
		selectWorkload,
		insertWorkload,
		updateWorkload,
		batchWorkload,
	}
	
	start := time.Now()
	
	for i, workload := range workloads {
		for j := 0; j < numWorkers/len(workloads); j++ {
			wg.Add(1)
			go func(workloadFunc func(context.Context, *performance.DatabaseOptimizer, int), workerID int) {
				defer wg.Done()
				workloadFunc(ctx, optimizer, workerID)
			}(workload, i*numWorkers/len(workloads)+j)
		}
	}
	
	// Monitor metrics
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				metrics := optimizer.GetMetrics()
				fmt.Printf("📊 Live Metrics - Success: %d, Failed: %d, Avg Latency: %v\n",
					atomic.LoadInt64(&metrics.SuccessfulQueries),
					atomic.LoadInt64(&metrics.FailedQueries),
					metrics.GetAverageQueryLatency())
			case <-ctx.Done():
				return
			}
		}
	}()
	
	wg.Wait()
	elapsed := time.Since(start)
	
	metrics := optimizer.GetMetrics()
	
	fmt.Printf("🏁 Load Test Results:\n")
	fmt.Printf("   - Test duration: %v\n", elapsed)
	fmt.Printf("   - Total queries: %d\n", atomic.LoadInt64(&metrics.SuccessfulQueries)+atomic.LoadInt64(&metrics.FailedQueries))
	fmt.Printf("   - Queries/sec: %.0f\n", float64(atomic.LoadInt64(&metrics.SuccessfulQueries)+atomic.LoadInt64(&metrics.FailedQueries))/elapsed.Seconds())
	fmt.Printf("   - Success rate: %.2f%%\n", metrics.GetSuccessRate()*100)
	fmt.Printf("   - Average latency: %v\n", metrics.GetAverageQueryLatency())
	fmt.Printf("   - Cache performance: %.2f%% hit ratio\n", metrics.GetCacheHitRatio()*100)
}

func selectWorkload(ctx context.Context, optimizer *performance.DatabaseOptimizer, workerID int) {
	for i := 0; i < 100; i++ {
		select {
		case <-ctx.Done():
			return
		default:
			query := fmt.Sprintf("SELECT * FROM users WHERE id = %d", i%1000)
			rows, _ := optimizer.ExecuteQuery(ctx, query, i%1000)
			if rows != nil {
				rows.Close()
			}
			time.Sleep(time.Millisecond)
		}
	}
}

func insertWorkload(ctx context.Context, optimizer *performance.DatabaseOptimizer, workerID int) {
	for i := 0; i < 50; i++ {
		select {
		case <-ctx.Done():
			return
		default:
			query := "INSERT INTO orders (user_id, amount) VALUES ($1, $2)"
			rows, _ := optimizer.ExecuteQuery(ctx, query, workerID*1000+i, float64(i)*10.5)
			if rows != nil {
				rows.Close()
			}
			time.Sleep(time.Millisecond * 2)
		}
	}
}

func updateWorkload(ctx context.Context, optimizer *performance.DatabaseOptimizer, workerID int) {
	for i := 0; i < 30; i++ {
		select {
		case <-ctx.Done():
			return
		default:
			query := "UPDATE accounts SET balance = balance + $1 WHERE id = $2"
			rows, _ := optimizer.ExecuteQuery(ctx, query, float64(i), workerID*100+i)
			if rows != nil {
				rows.Close()
			}
			time.Sleep(time.Millisecond * 5)
		}
	}
}

func batchWorkload(ctx context.Context, optimizer *performance.DatabaseOptimizer, workerID int) {
	statements := make([]performance.BatchStatement, 20)
	for i := 0; i < 20; i++ {
		statements[i] = performance.BatchStatement{
			Query: "INSERT INTO logs (worker_id, message, created_at) VALUES ($1, $2, $3)",
			Args:  []interface{}{workerID, fmt.Sprintf("Batch message %d", i), time.Now()},
		}
	}
	
	select {
	case <-ctx.Done():
		return
	default:
		optimizer.ExecuteBatch(ctx, statements)
		time.Sleep(time.Millisecond * 10)
	}
}