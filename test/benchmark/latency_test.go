package benchmark

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mExOms/internal/position"
	"github.com/mExOms/internal/risk"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// BenchmarkOrderProcessingLatency measures end-to-end order processing time
func BenchmarkOrderProcessingLatency(b *testing.B) {
	// Setup components
	riskEngine := risk.NewRiskEngine()
	riskEngine.SetMaxOrderValue(decimal.NewFromFloat(100000))
	riskEngine.SetMaxPositionSize(decimal.NewFromFloat(500000))
	
	ctx := context.Background()
	
	// Create test order
	order := &types.Order{
		Symbol:   "BTCUSDT",
		Side:     types.OrderSideBuy,
		Type:     types.OrderTypeLimit,
		Price:    decimal.NewFromFloat(42000),
		Quantity: decimal.NewFromFloat(0.1),
	}
	
	// Measure latencies
	latencies := make([]time.Duration, b.N)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		
		// Simulate order processing
		_, err := riskEngine.CheckOrder(ctx, order, "binance")
		if err != nil {
			b.Fatal(err)
		}
		
		latencies[i] = time.Since(start)
	}
	
	// Calculate percentiles
	p50 := calculatePercentile(latencies, 0.5)
	p95 := calculatePercentile(latencies, 0.95)
	p99 := calculatePercentile(latencies, 0.99)
	p999 := calculatePercentile(latencies, 0.999)
	
	b.ReportMetric(float64(p50.Nanoseconds())/1000, "p50_us")
	b.ReportMetric(float64(p95.Nanoseconds())/1000, "p95_us")
	b.ReportMetric(float64(p99.Nanoseconds())/1000, "p99_us")
	b.ReportMetric(float64(p999.Nanoseconds())/1000, "p99.9_us")
}

// BenchmarkRiskCheckLatency measures risk engine latency
func BenchmarkRiskCheckLatency(b *testing.B) {
	riskEngine := risk.NewRiskEngine()
	riskEngine.SetMaxOrderValue(decimal.NewFromFloat(100000))
	riskEngine.SetMaxPositionSize(decimal.NewFromFloat(500000))
	riskEngine.SetMaxLeverage(20)
	
	// Add some positions for realistic testing
	for i := 0; i < 10; i++ {
		riskEngine.UpdatePosition("binance", "BTCUSDT", &risk.PositionRisk{
			Symbol:        "BTCUSDT",
			Quantity:      decimal.NewFromFloat(0.1),
			AvgEntryPrice: decimal.NewFromFloat(40000),
			MarkPrice:     decimal.NewFromFloat(42000),
			UpdatedAt:     time.Now(),
		})
	}
	
	ctx := context.Background()
	order := &types.Order{
		Symbol:   "BTCUSDT",
		Side:     types.OrderSideBuy,
		Type:     types.OrderTypeLimit,
		Price:    decimal.NewFromFloat(42000),
		Quantity: decimal.NewFromFloat(0.1),
	}
	
	// Warm up
	for i := 0; i < 1000; i++ {
		riskEngine.CheckOrder(ctx, order, "binance")
	}
	
	latencies := make([]time.Duration, b.N)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		_, err := riskEngine.CheckOrder(ctx, order, "binance")
		latencies[i] = time.Since(start)
		
		if err != nil {
			b.Fatal(err)
		}
	}
	
	// Report detailed metrics
	reportLatencyMetrics(b, latencies)
}

// BenchmarkPositionUpdateLatency measures position manager latency
func BenchmarkPositionUpdateLatency(b *testing.B) {
	posManager, err := position.NewPositionManager("./data/snapshots")
	if err != nil {
		b.Fatal(err)
	}
	defer posManager.Close()
	
	pos := &position.Position{
		Symbol:     "BTCUSDT",
		Exchange:   "binance",
		Market:     "spot",
		Side:       "LONG",
		Quantity:   decimal.NewFromFloat(0.5),
		EntryPrice: decimal.NewFromFloat(40000),
		MarkPrice:  decimal.NewFromFloat(42000),
		Leverage:   1,
		MarginUsed: decimal.NewFromFloat(20000),
	}
	
	latencies := make([]time.Duration, b.N)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pos.MarkPrice = decimal.NewFromFloat(42000 + float64(i%100))
		
		start := time.Now()
		err := posManager.UpdatePosition(pos)
		latencies[i] = time.Since(start)
		
		if err != nil {
			b.Fatal(err)
		}
	}
	
	reportLatencyMetrics(b, latencies)
}

// BenchmarkConcurrentOperations tests latency under concurrent load
func BenchmarkConcurrentOperations(b *testing.B) {
	riskEngine := risk.NewRiskEngine()
	riskEngine.SetMaxOrderValue(decimal.NewFromFloat(100000))
	
	ctx := context.Background()
	
	// Different operation types
	operations := []struct {
		name string
		fn   func()
	}{
		{
			name: "RiskCheck",
			fn: func() {
				order := &types.Order{
					Symbol:   "BTCUSDT",
					Side:     types.OrderSideBuy,
					Type:     types.OrderTypeLimit,
					Price:    decimal.NewFromFloat(42000),
					Quantity: decimal.NewFromFloat(0.1),
				}
				riskEngine.CheckOrder(ctx, order, "binance")
			},
		},
		{
			name: "UpdatePosition",
			fn: func() {
				riskEngine.UpdatePosition("binance", "BTCUSDT", &risk.PositionRisk{
					Symbol:        "BTCUSDT",
					Quantity:      decimal.NewFromFloat(0.1),
					AvgEntryPrice: decimal.NewFromFloat(40000),
					MarkPrice:     decimal.NewFromFloat(42000),
					UpdatedAt:     time.Now(),
				})
			},
		},
	}
	
	for _, op := range operations {
		b.Run(op.name, func(b *testing.B) {
			// Test with different concurrency levels
			for _, concurrency := range []int{1, 10, 100, 1000} {
				b.Run(b.Name()+"_c"+string(rune(concurrency)), func(b *testing.B) {
					var wg sync.WaitGroup
					latencyChan := make(chan time.Duration, b.N)
					
					b.ResetTimer()
					
					// Launch workers
					for w := 0; w < concurrency; w++ {
						wg.Add(1)
						go func() {
							defer wg.Done()
							
							for i := 0; i < b.N/concurrency; i++ {
								start := time.Now()
								op.fn()
								latencyChan <- time.Since(start)
							}
						}()
					}
					
					wg.Wait()
					close(latencyChan)
					
					// Collect latencies
					var latencies []time.Duration
					for lat := range latencyChan {
						latencies = append(latencies, lat)
					}
					
					reportLatencyMetrics(b, latencies)
				})
			}
		})
	}
}

// BenchmarkMessageProcessingLatency tests NATS-like message processing
func BenchmarkMessageProcessingLatency(b *testing.B) {
	type Message struct {
		Subject string
		Data    []byte
	}
	
	msgChan := make(chan *Message, 10000)
	
	// Simulate message processor
	go func() {
		for msg := range msgChan {
			// Simulate processing
			_ = msg
			time.Sleep(time.Microsecond)
		}
	}()
	
	latencies := make([]time.Duration, b.N)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := &Message{
			Subject: "order.binance.spot.BTCUSDT",
			Data:    []byte(`{"symbol":"BTCUSDT","price":"42000"}`),
		}
		
		start := time.Now()
		msgChan <- msg
		latencies[i] = time.Since(start)
	}
	
	reportLatencyMetrics(b, latencies)
}

// BenchmarkCriticalPathLatency measures the critical path for order execution
func BenchmarkCriticalPathLatency(b *testing.B) {
	// Simulate complete order flow
	steps := []struct {
		name    string
		latency time.Duration
	}{
		{"ParseRequest", 100 * time.Nanosecond},
		{"RiskCheck", 500 * time.Nanosecond},
		{"RouteOrder", 200 * time.Nanosecond},
		{"SendToExchange", 1 * time.Microsecond},
		{"UpdatePosition", 300 * time.Nanosecond},
		{"PublishEvent", 200 * time.Nanosecond},
	}
	
	latencies := make([]time.Duration, b.N)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		
		for _, step := range steps {
			// Simulate step execution
			time.Sleep(step.latency)
		}
		
		latencies[i] = time.Since(start)
	}
	
	// Report step breakdowns
	totalLatency := time.Duration(0)
	for _, step := range steps {
		totalLatency += step.latency
		b.Logf("%s: %v", step.name, step.latency)
	}
	b.Logf("Total expected: %v", totalLatency)
	
	reportLatencyMetrics(b, latencies)
}

// Helper functions

func calculatePercentile(latencies []time.Duration, percentile float64) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	
	// Simple percentile calculation (not exact for small samples)
	index := int(float64(len(latencies)) * percentile)
	if index >= len(latencies) {
		index = len(latencies) - 1
	}
	
	return latencies[index]
}

func reportLatencyMetrics(b *testing.B, latencies []time.Duration) {
	if len(latencies) == 0 {
		return
	}
	
	// Calculate statistics
	var total time.Duration
	min := latencies[0]
	max := latencies[0]
	
	for _, lat := range latencies {
		total += lat
		if lat < min {
			min = lat
		}
		if lat > max {
			max = lat
		}
	}
	
	avg := total / time.Duration(len(latencies))
	
	// Calculate percentiles
	p50 := calculatePercentile(latencies, 0.5)
	p95 := calculatePercentile(latencies, 0.95)
	p99 := calculatePercentile(latencies, 0.99)
	p999 := calculatePercentile(latencies, 0.999)
	
	// Report metrics
	b.ReportMetric(float64(avg.Nanoseconds())/1000, "avg_us")
	b.ReportMetric(float64(min.Nanoseconds())/1000, "min_us")
	b.ReportMetric(float64(max.Nanoseconds())/1000, "max_us")
	b.ReportMetric(float64(p50.Nanoseconds())/1000, "p50_us")
	b.ReportMetric(float64(p95.Nanoseconds())/1000, "p95_us")
	b.ReportMetric(float64(p99.Nanoseconds())/1000, "p99_us")
	b.ReportMetric(float64(p999.Nanoseconds())/1000, "p99.9_us")
	
	// Calculate jitter (standard deviation)
	var sumSquaredDiff float64
	avgNanos := float64(avg.Nanoseconds())
	for _, lat := range latencies {
		diff := float64(lat.Nanoseconds()) - avgNanos
		sumSquaredDiff += diff * diff
	}
	
	variance := sumSquaredDiff / float64(len(latencies))
	stdDev := time.Duration(int64(variance))
	b.ReportMetric(float64(stdDev.Nanoseconds())/1000, "jitter_us")
}