package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/your-org/mexoms/pkg/performance"
)

func main() {
	fmt.Println("🌐 Network Performance Optimization Demo")
	fmt.Println(strings.Repeat("=", 60))

	// Test Network Optimizer
	fmt.Println("\n1. Testing Network Optimizer")
	testNetworkOptimizer()

	// Test Connection Pool
	fmt.Println("\n2. Testing Connection Pool")
	testConnectionPool()

	// Test Zero-Copy Operations
	fmt.Println("\n3. Testing Zero-Copy Operations")
	testZeroCopyOperations()

	// Test Batch Operations
	fmt.Println("\n4. Testing Batch Write Operations")
	testBatchOperations()

	// Performance Comparison
	fmt.Println("\n5. Performance Comparison")
	performanceComparison()

	fmt.Println("\n✅ All network performance tests completed!")
}

func testNetworkOptimizer() {
	config := performance.DefaultNetworkConfig()
	optimizer := performance.NewNetworkOptimizer(config)
	optimizer.Start()
	defer optimizer.Stop()

	// Start a simple echo server for testing
	server, serverAddr := startEchoServer()
	defer server.Close()

	// Test optimized connections
	const numConnections = 100
	const messagesPerConn = 10

	var wg sync.WaitGroup
	var totalLatency int64
	var totalMessages int64

	start := time.Now()

	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func(connID int) {
			defer wg.Done()

			// Use optimized dial
			conn, err := optimizer.OptimizedDial("tcp", serverAddr)
			if err != nil {
				fmt.Printf("❌ Connection %d failed: %v\n", connID, err)
				return
			}
			defer conn.Close()

			// Send messages and measure latency
			for j := 0; j < messagesPerConn; j++ {
				message := fmt.Sprintf("Hello from connection %d, message %d", connID, j)
				
				msgStart := time.Now()
				
				// Send message
				_, err := conn.Write([]byte(message))
				if err != nil {
					fmt.Printf("❌ Write failed: %v\n", err)
					continue
				}

				// Read response
				buffer := make([]byte, len(message))
				_, err = io.ReadFull(conn, buffer)
				if err != nil {
					fmt.Printf("❌ Read failed: %v\n", err)
					continue
				}

				latency := time.Since(msgStart)
				atomic.AddInt64(&totalLatency, int64(latency))
				atomic.AddInt64(&totalMessages, 1)
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	// Get metrics
	metrics := optimizer.GetMetrics()
	poolMetrics := optimizer.GetConnPool().GetMetrics()

	avgLatency := time.Duration(0)
	if totalMessages > 0 {
		avgLatency = time.Duration(atomic.LoadInt64(&totalLatency) / atomic.LoadInt64(&totalMessages))
	}

	fmt.Printf("📊 Network Optimizer Performance:\n")
	fmt.Printf("   - Total connections: %d\n", numConnections)
	fmt.Printf("   - Messages per connection: %d\n", messagesPerConn)
	fmt.Printf("   - Total duration: %v\n", elapsed)
	fmt.Printf("   - Messages processed: %d\n", atomic.LoadInt64(&totalMessages))
	fmt.Printf("   - Messages/sec: %.0f\n", float64(atomic.LoadInt64(&totalMessages))/elapsed.Seconds())
	fmt.Printf("   - Average message latency: %v\n", avgLatency)
	fmt.Printf("   - Connection errors: %d\n", atomic.LoadInt64(&metrics.ConnectionErrors))
	fmt.Printf("   - Pool reuse count: %d\n", atomic.LoadInt64(&poolMetrics.ReuseCount))
}

func testConnectionPool() {
	fmt.Println("🏊 Testing Connection Pool...")

	config := performance.DefaultNetworkConfig()
	optimizer := performance.NewNetworkOptimizer(config)
	optimizer.Start()
	defer optimizer.Stop()

	pool := optimizer.GetConnPool()

	// Start echo server
	server, serverAddr := startEchoServer()
	defer server.Close()

	// Pre-populate pool with connections
	const poolSize = 10
	connections := make([]net.Conn, poolSize)

	for i := 0; i < poolSize; i++ {
		conn, err := net.Dial("tcp", serverAddr)
		if err != nil {
			fmt.Printf("❌ Failed to create connection %d: %v\n", i, err)
			continue
		}
		connections[i] = conn
		pool.Put(conn)
	}

	// Test connection reuse
	const numRequests = 100
	var poolHits int64
	var poolMisses int64

	start := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(reqID int) {
			defer wg.Done()

			// Try to get connection from pool
			conn := pool.Get()
			if conn != nil {
				atomic.AddInt64(&poolHits, 1)
				defer pool.Put(conn)
			} else {
				atomic.AddInt64(&poolMisses, 1)
				// Create new connection
				var err error
				conn, err = net.Dial("tcp", serverAddr)
				if err != nil {
					return
				}
				defer conn.Close()
			}

			// Send test message
			message := fmt.Sprintf("Pool test %d", reqID)
			conn.Write([]byte(message))
			
			buffer := make([]byte, len(message))
			io.ReadFull(conn, buffer)
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	poolMetrics := pool.GetMetrics()

	fmt.Printf("📊 Connection Pool Performance:\n")
	fmt.Printf("   - Requests: %d\n", numRequests)
	fmt.Printf("   - Duration: %v\n", elapsed)
	fmt.Printf("   - Requests/sec: %.0f\n", float64(numRequests)/elapsed.Seconds())
	fmt.Printf("   - Pool hits: %d\n", atomic.LoadInt64(&poolHits))
	fmt.Printf("   - Pool misses: %d\n", atomic.LoadInt64(&poolMisses))
	fmt.Printf("   - Hit ratio: %.2f%%\n", float64(atomic.LoadInt64(&poolHits))/float64(numRequests)*100)
	fmt.Printf("   - Pool metrics: %+v\n", poolMetrics)

	// Clean up connections
	for _, conn := range connections {
		if conn != nil {
			conn.Close()
		}
	}
}

func testZeroCopyOperations() {
	fmt.Println("⚡ Testing Zero-Copy Operations...")

	config := performance.DefaultNetworkConfig()
	config.EnableZeroCopy = true
	optimizer := performance.NewNetworkOptimizer(config)
	optimizer.Start()
	defer optimizer.Stop()

	// Create a temporary file for testing
	tempFile, err := os.CreateTemp("", "zerocopy_test_*.txt")
	if err != nil {
		fmt.Printf("❌ Failed to create temp file: %v\n", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Write test data to file
	testData := strings.Repeat("This is test data for zero-copy operations. ", 1000)
	tempFile.WriteString(testData)
	tempFile.Seek(0, 0)

	// Start echo server that can handle file data
	server, serverAddr := startFileEchoServer()
	defer server.Close()

	// Test zero-copy transfer
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		fmt.Printf("❌ Failed to connect: %v\n", err)
		return
	}
	defer conn.Close()

	fileSize := int64(len(testData))
	
	// Test zero-copy write
	start := time.Now()
	written, err := optimizer.ZeroCopyWrite(conn, tempFile, fileSize)
	zeroCopyTime := time.Since(start)

	if err != nil {
		fmt.Printf("❌ Zero-copy write failed: %v\n", err)
		fmt.Printf("📝 Note: Zero-copy requires Linux with proper kernel support\n")
		
		// Fall back to regular copy
		tempFile.Seek(0, 0)
		start = time.Now()
		written, err = io.Copy(conn, tempFile)
		regularCopyTime := time.Since(start)
		
		if err == nil {
			fmt.Printf("📊 Regular Copy Performance:\n")
			fmt.Printf("   - Bytes transferred: %d\n", written)
			fmt.Printf("   - Transfer time: %v\n", regularCopyTime)
			fmt.Printf("   - Throughput: %.2f MB/s\n", float64(written)/regularCopyTime.Seconds()/(1024*1024))
		}
		return
	}

	fmt.Printf("📊 Zero-Copy Performance:\n")
	fmt.Printf("   - Bytes transferred: %d\n", written)
	fmt.Printf("   - Transfer time: %v\n", zeroCopyTime)
	fmt.Printf("   - Throughput: %.2f MB/s\n", float64(written)/zeroCopyTime.Seconds()/(1024*1024))

	// Compare with regular copy
	tempFile.Seek(0, 0)
	conn2, err := net.Dial("tcp", serverAddr)
	if err != nil {
		return
	}
	defer conn2.Close()

	start = time.Now()
	regularWritten, err := io.Copy(conn2, tempFile)
	regularTime := time.Since(start)

	if err == nil {
		fmt.Printf("📊 Comparison with Regular Copy:\n")
		fmt.Printf("   - Regular copy time: %v\n", regularTime)
		fmt.Printf("   - Regular throughput: %.2f MB/s\n", float64(regularWritten)/regularTime.Seconds()/(1024*1024))
		if zeroCopyTime > 0 {
			fmt.Printf("   - Zero-copy speedup: %.2fx\n", float64(regularTime)/float64(zeroCopyTime))
		}
	}
}

func testBatchOperations() {
	fmt.Println("📦 Testing Batch Write Operations...")

	config := performance.DefaultNetworkConfig()
	config.EnableBatching = true
	optimizer := performance.NewNetworkOptimizer(config)
	optimizer.Start()
	defer optimizer.Stop()

	// Start echo server
	server, serverAddr := startEchoServer()
	defer server.Close()

	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		fmt.Printf("❌ Failed to connect: %v\n", err)
		return
	}
	defer conn.Close()

	// Prepare batch data
	const numMessages = 100
	messages := make([][]byte, numMessages)
	var totalSize int64

	for i := 0; i < numMessages; i++ {
		message := fmt.Sprintf("Batch message %d with some data", i)
		messages[i] = []byte(message)
		totalSize += int64(len(message))
	}

	// Test batch write
	start := time.Now()
	written, err := optimizer.BatchWrite(conn, messages)
	batchTime := time.Since(start)

	if err != nil {
		fmt.Printf("❌ Batch write failed: %v\n", err)
		return
	}

	// Test individual writes for comparison
	conn2, err := net.Dial("tcp", serverAddr)
	if err != nil {
		return
	}
	defer conn2.Close()

	start = time.Now()
	var individualWritten int64
	for _, message := range messages {
		n, err := conn2.Write(message)
		if err != nil {
			break
		}
		individualWritten += int64(n)
	}
	individualTime := time.Since(start)

	fmt.Printf("📊 Batch Write Performance:\n")
	fmt.Printf("   - Messages: %d\n", numMessages)
	fmt.Printf("   - Total bytes: %d\n", totalSize)
	fmt.Printf("   - Batch write time: %v\n", batchTime)
	fmt.Printf("   - Batch throughput: %.2f MB/s\n", float64(written)/batchTime.Seconds()/(1024*1024))
	fmt.Printf("   - Individual write time: %v\n", individualTime)
	fmt.Printf("   - Individual throughput: %.2f MB/s\n", float64(individualWritten)/individualTime.Seconds()/(1024*1024))
	if batchTime > 0 {
		fmt.Printf("   - Batch speedup: %.2fx\n", float64(individualTime)/float64(batchTime))
	}
}

func performanceComparison() {
	fmt.Println("⚡ Performance Comparison: Optimized vs Standard")

	// Start echo server
	server, serverAddr := startEchoServer()
	defer server.Close()

	const numConnections = 50
	const messagesPerConn = 20

	// Test with optimized network
	fmt.Println("🚀 Testing optimized network...")
	config := performance.DefaultNetworkConfig()
	optimizer := performance.NewNetworkOptimizer(config)
	optimizer.Start()
	
	optimizedTime := runNetworkTest(optimizer, serverAddr, numConnections, messagesPerConn, true)
	optimizedMetrics := optimizer.GetMetrics()
	optimizer.Stop()

	// Test with standard network
	fmt.Println("📡 Testing standard network...")
	standardTime := runNetworkTest(nil, serverAddr, numConnections, messagesPerConn, false)

	// Results
	fmt.Printf("📊 Performance Comparison Results:\n")
	fmt.Printf("   - Optimized time: %v\n", optimizedTime)
	fmt.Printf("   - Standard time: %v\n", standardTime)
	if optimizedTime > 0 {
		fmt.Printf("   - Speedup: %.2fx\n", float64(standardTime)/float64(optimizedTime))
	}
	fmt.Printf("   - Optimized connections created: %d\n", atomic.LoadInt64(&optimizedMetrics.ConnectionsCreated))
	fmt.Printf("   - Optimized connection errors: %d\n", atomic.LoadInt64(&optimizedMetrics.ConnectionErrors))
	fmt.Printf("   - Optimized avg connection latency: %v\n", optimizedMetrics.GetAverageConnectionLatency())

	readMBps, writeMBps := optimizedMetrics.GetThroughput()
	fmt.Printf("   - Read throughput: %.2f MB/s\n", readMBps)
	fmt.Printf("   - Write throughput: %.2f MB/s\n", writeMBps)
}

func runNetworkTest(optimizer *performance.NetworkOptimizer, serverAddr string, numConnections, messagesPerConn int, useOptimized bool) time.Duration {
	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func(connID int) {
			defer wg.Done()

			var conn net.Conn
			var err error

			if useOptimized && optimizer != nil {
				conn, err = optimizer.OptimizedDial("tcp", serverAddr)
			} else {
				conn, err = net.Dial("tcp", serverAddr)
			}

			if err != nil {
				return
			}
			defer conn.Close()

			// Send messages
			for j := 0; j < messagesPerConn; j++ {
				message := fmt.Sprintf("Test message %d from connection %d", j, connID)
				conn.Write([]byte(message))
				
				buffer := make([]byte, len(message))
				io.ReadFull(conn, buffer)
			}
		}(i)
	}

	wg.Wait()
	return time.Since(start)
}

// Helper functions for test servers

func startEchoServer() (net.Listener, string) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatal("Failed to start echo server:", err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleEchoConnection(conn)
		}
	}()

	return listener, listener.Addr().String()
}

func startFileEchoServer() (net.Listener, string) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatal("Failed to start file echo server:", err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleFileEchoConnection(conn)
		}
	}()

	return listener, listener.Addr().String()
}

func handleEchoConnection(conn net.Conn) {
	defer conn.Close()
	
	buffer := make([]byte, 1024)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			return
		}
		
		_, err = conn.Write(buffer[:n])
		if err != nil {
			return
		}
	}
}

func handleFileEchoConnection(conn net.Conn) {
	defer conn.Close()
	
	// Simply read all data and discard it (simulating file reception)
	buffer := make([]byte, 1024)
	for {
		_, err := conn.Read(buffer)
		if err != nil {
			return
		}
		// In a real server, we would process the file data
	}
}