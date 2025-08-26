package main

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/your-org/mexoms/pkg/performance"
)

func main() {
	fmt.Println("🚀 Go Service Layer Performance Optimization Demo")
	fmt.Println(strings.Repeat("=", 60))

	// Test Goroutine Pool
	fmt.Println("\n1. Testing Goroutine Pool Performance")
	testGoroutinePool()

	// Test Memory Optimization
	fmt.Println("\n2. Testing Memory Pool Performance")
	testMemoryOptimization()

	// Test Runtime Optimizer
	fmt.Println("\n3. Testing Runtime Optimizer")
	testRuntimeOptimizer()

	// Performance Comparison
	fmt.Println("\n4. Performance Comparison")
	performanceComparison()

	fmt.Println("\n✅ All performance tests completed!")
}

func testGoroutinePool() {
	config := performance.DefaultPoolConfig()
	pool := performance.NewWorkerPool(config)
	defer pool.Shutdown()

	// Submit tasks
	const numTasks = 10000
	var wg sync.WaitGroup
	
	start := time.Now()
	
	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		task := func() {
			defer wg.Done()
			// Simulate some work
			time.Sleep(time.Microsecond * 100)
		}
		
		if !pool.Submit(task) {
			fmt.Printf("❌ Failed to submit task %d\n", i)
			wg.Done()
		}
	}
	
	wg.Wait()
	elapsed := time.Since(start)
	
	stats := pool.Stats()
	fmt.Printf("📊 Pool Stats:\n")
	fmt.Printf("   - Tasks processed: %d in %v\n", stats.CompletedTasks, elapsed)
	fmt.Printf("   - Worker count: %d\n", stats.WorkerCount)
	fmt.Printf("   - Queue size: %d\n", stats.QueueSize)
	fmt.Printf("   - Utilization: %.2f%%\n", stats.Utilization*100)
	fmt.Printf("   - Throughput: %.0f tasks/sec\n", float64(numTasks)/elapsed.Seconds())
}

func testMemoryOptimization() {
	// Test Object Pool
	fmt.Println("🧠 Testing Object Pool...")
	
	type TestStruct struct {
		Data [1000]byte
	}
	
	objPool := performance.NewObjectPool(func() *TestStruct {
		return &TestStruct{}
	})
	
	// Benchmark with pool
	start := time.Now()
	const iterations = 100000
	
	for i := 0; i < iterations; i++ {
		obj := objPool.Get()
		obj.Data[0] = byte(i)
		objPool.Put(obj)
	}
	
	poolTime := time.Since(start)
	
	// Benchmark without pool
	start = time.Now()
	
	for i := 0; i < iterations; i++ {
		obj := &TestStruct{}
		obj.Data[0] = byte(i)
		_ = obj // Prevent optimization
	}
	
	directTime := time.Since(start)
	
	fmt.Printf("📊 Memory Pool Performance:\n")
	fmt.Printf("   - With pool: %v (%v per operation)\n", poolTime, poolTime/iterations)
	fmt.Printf("   - Without pool: %v (%v per operation)\n", directTime, directTime/iterations)
	fmt.Printf("   - Speedup: %.2fx\n", float64(directTime)/float64(poolTime))
	
	// Test Slice Pool
	fmt.Println("🔢 Testing Slice Pool...")
	
	slicePool := performance.NewSlicePool[int]()
	
	start = time.Now()
	
	for i := 0; i < iterations; i++ {
		slice := slicePool.Get(100)
		for j := range slice {
			slice[j] = j
		}
		slicePool.Put(slice)
	}
	
	slicePoolTime := time.Since(start)
	
	start = time.Now()
	
	for i := 0; i < iterations; i++ {
		slice := make([]int, 100)
		for j := range slice {
			slice[j] = j
		}
		_ = slice
	}
	
	sliceDirectTime := time.Since(start)
	
	fmt.Printf("📊 Slice Pool Performance:\n")
	fmt.Printf("   - With pool: %v (%v per operation)\n", slicePoolTime, slicePoolTime/iterations)
	fmt.Printf("   - Without pool: %v (%v per operation)\n", sliceDirectTime, sliceDirectTime/iterations)
	fmt.Printf("   - Speedup: %.2fx\n", float64(sliceDirectTime)/float64(slicePoolTime))
	
	// Test Byte Buffer Pool
	fmt.Println("📝 Testing Byte Buffer Pool...")
	
	bytePool := performance.NewByteBufferPool()
	
	start = time.Now()
	
	for i := 0; i < iterations; i++ {
		buf := bytePool.Get(512)
		copy(buf, []byte("Hello, World!"))
		bytePool.Put(buf)
	}
	
	bytePoolTime := time.Since(start)
	
	start = time.Now()
	
	for i := 0; i < iterations; i++ {
		buf := make([]byte, 512)
		copy(buf, []byte("Hello, World!"))
		_ = buf
	}
	
	byteDirectTime := time.Since(start)
	
	fmt.Printf("📊 Byte Buffer Pool Performance:\n")
	fmt.Printf("   - With pool: %v (%v per operation)\n", bytePoolTime, bytePoolTime/iterations)
	fmt.Printf("   - Without pool: %v (%v per operation)\n", byteDirectTime, byteDirectTime/iterations)
	fmt.Printf("   - Speedup: %.2fx\n", float64(byteDirectTime)/float64(bytePoolTime))
}

func testRuntimeOptimizer() {
	config := performance.DefaultOptimizerConfig()
	config.MonitoringInterval = 2 * time.Second
	
	optimizer := performance.NewRuntimeOptimizer(config)
	optimizer.Start()
	defer optimizer.Stop()
	
	fmt.Println("⚙️ Runtime Optimizer started...")
	
	// Generate some load
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	var wg sync.WaitGroup
	
	// Simulate memory-intensive workload
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			for {
				select {
				case <-ctx.Done():
					return
				default:
					// Allocate and release memory
					data := make([]byte, 1024*1024) // 1MB
					for j := range data {
						data[j] = byte(j % 256)
					}
					runtime.GC() // Force some GC pressure
					time.Sleep(100 * time.Millisecond)
				}
			}
		}()
	}
	
	// Monitor metrics
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				metrics := optimizer.GetMetrics()
				fmt.Printf("📊 Runtime Metrics (%.1fs):\n", time.Since(metrics.Timestamp).Seconds())
				fmt.Printf("   - Goroutines: %d\n", metrics.NumGoroutines)
				fmt.Printf("   - Heap Alloc: %.2f MB\n", float64(metrics.HeapAlloc)/1024/1024)
				fmt.Printf("   - Heap Sys: %.2f MB\n", float64(metrics.HeapSys)/1024/1024)
				fmt.Printf("   - GOGC: %d%%\n", metrics.GOGC)
				fmt.Printf("   - Num GC: %d\n", metrics.NumGC)
				
				if metrics.PauseTotalNs > 0 && metrics.NumGC > 0 {
					avgPause := time.Duration(metrics.PauseTotalNs / uint64(metrics.NumGC))
					fmt.Printf("   - Avg GC Pause: %v\n", avgPause)
				}
			}
		}
	}()
	
	wg.Wait()
	
	// Final metrics
	finalMetrics := optimizer.GetMetrics()
	fmt.Printf("🏁 Final Runtime Metrics:\n")
	fmt.Printf("   - Total Allocations: %d\n", finalMetrics.Mallocs)
	fmt.Printf("   - Total Frees: %d\n", finalMetrics.Frees)
	fmt.Printf("   - Live Objects: %d\n", finalMetrics.Mallocs-finalMetrics.Frees)
	fmt.Printf("   - Total GC Count: %d\n", finalMetrics.NumGC)
}

func performanceComparison() {
	fmt.Println("⚡ Performance Comparison: Optimized vs Standard")
	
	const iterations = 100000
	
	// Test 1: Task Processing
	fmt.Println("🔄 Task Processing Comparison:")
	
	// Optimized with pool
	config := performance.DefaultPoolConfig()
	pool := performance.NewWorkerPool(config)
	defer pool.Shutdown()
	
	var wg sync.WaitGroup
	start := time.Now()
	
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		task := func() {
			defer wg.Done()
			// Simulate CPU work
			sum := 0
			for j := 0; j < 1000; j++ {
				sum += j
			}
			_ = sum
		}
		
		pool.Submit(task)
	}
	
	wg.Wait()
	optimizedTime := time.Since(start)
	
	// Standard goroutines
	start = time.Now()
	wg = sync.WaitGroup{}
	
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Same work
			sum := 0
			for j := 0; j < 1000; j++ {
				sum += j
			}
			_ = sum
		}()
	}
	
	wg.Wait()
	standardTime := time.Since(start)
	
	fmt.Printf("   - Optimized (pool): %v\n", optimizedTime)
	fmt.Printf("   - Standard (goroutines): %v\n", standardTime)
	fmt.Printf("   - Improvement: %.2fx\n", float64(standardTime)/float64(optimizedTime))
	
	// Test 2: Memory allocation
	fmt.Println("💾 Memory Allocation Comparison:")
	
	// Get initial memory stats
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)
	
	// Optimized allocation
	objPool := performance.NewObjectPool(func() *[1024]byte {
		return &[1024]byte{}
	})
	
	start = time.Now()
	
	for i := 0; i < iterations; i++ {
		obj := objPool.Get()
		obj[0] = byte(i)
		objPool.Put(obj)
	}
	
	optimizedAllocTime := time.Since(start)
	runtime.GC()
	runtime.ReadMemStats(&m2)
	optimizedAllocs := m2.TotalAlloc - m1.Alloc
	
	// Standard allocation
	runtime.GC()
	runtime.ReadMemStats(&m1)
	
	start = time.Now()
	
	for i := 0; i < iterations; i++ {
		obj := &[1024]byte{}
		obj[0] = byte(i)
		_ = obj
	}
	
	standardAllocTime := time.Since(start)
	runtime.GC()
	runtime.ReadMemStats(&m2)
	standardAllocs := m2.TotalAlloc - m1.Alloc
	
	fmt.Printf("   - Optimized time: %v\n", optimizedAllocTime)
	fmt.Printf("   - Standard time: %v\n", standardAllocTime)
	fmt.Printf("   - Optimized allocs: %.2f MB\n", float64(optimizedAllocs)/1024/1024)
	fmt.Printf("   - Standard allocs: %.2f MB\n", float64(standardAllocs)/1024/1024)
	if optimizedAllocs > 0 {
		fmt.Printf("   - Memory saved: %.2fx\n", float64(standardAllocs)/float64(optimizedAllocs))
	}
	
	// Summary
	fmt.Println("📈 Performance Summary:")
	fmt.Printf("   - Task processing improvement: %.2fx faster\n", float64(standardTime)/float64(optimizedTime))
	if optimizedAllocs > 0 {
		fmt.Printf("   - Memory usage improvement: %.2fx less allocation\n", float64(standardAllocs)/float64(optimizedAllocs))
	}
}

func init() {
	// Set runtime parameters for better performance
	runtime.GOMAXPROCS(runtime.NumCPU())
	
	// Optional: increase GC target to reduce frequency
	// debug.SetGCPercent(200)
}