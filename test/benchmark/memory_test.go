package benchmark

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/mExOms/internal/position"
	"github.com/mExOms/internal/risk"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// BenchmarkMemoryAllocation tests memory allocation patterns
func BenchmarkMemoryAllocation(b *testing.B) {
	b.Run("Order", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			order := &types.Order{
				OrderID:       "12345",
				ClientOrderID: "client-12345",
				Symbol:        "BTCUSDT",
				Side:          types.OrderSideBuy,
				Type:          types.OrderTypeLimit,
				Price:         decimal.NewFromFloat(42000),
				Quantity:      decimal.NewFromFloat(0.1),
				Status:        types.OrderStatusNew,
				CreatedAt:     time.Now(),
			}
			_ = order
		}
	})
	
	b.Run("Position", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			pos := &position.Position{
				Symbol:        "BTCUSDT",
				Exchange:      "binance",
				Market:        "spot",
				Side:          "LONG",
				Quantity:      decimal.NewFromFloat(0.5),
				EntryPrice:    decimal.NewFromFloat(40000),
				MarkPrice:     decimal.NewFromFloat(42000),
				UnrealizedPnL: decimal.NewFromFloat(1000),
				Leverage:      1,
				MarginUsed:    decimal.NewFromFloat(20000),
				UpdatedAt:     time.Now(),
			}
			_ = pos
		}
	})
	
	b.Run("Decimal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			d1 := decimal.NewFromFloat(42000.50)
			d2 := decimal.NewFromFloat(0.1)
			result := d1.Mul(d2)
			_ = result
		}
	})
}

// BenchmarkMemoryUsage tests memory usage of key components
func BenchmarkMemoryUsage(b *testing.B) {
	b.Run("RiskEngine", func(b *testing.B) {
		var m runtime.MemStats
		
		runtime.GC()
		runtime.ReadMemStats(&m)
		before := m.Alloc
		
		// Create risk engine with positions
		engine := risk.NewRiskEngine()
		engine.SetMaxPositionSize(decimal.NewFromFloat(100000))
		
		// Add 1000 positions
		for i := 0; i < 1000; i++ {
			engine.UpdatePosition("binance", 
				b.Name()+"_"+string(rune(i)),
				&risk.PositionRisk{
					Symbol:        "BTCUSDT",
					Quantity:      decimal.NewFromFloat(0.1),
					AvgEntryPrice: decimal.NewFromFloat(40000),
					MarkPrice:     decimal.NewFromFloat(42000),
					UpdatedAt:     time.Now(),
				})
		}
		
		runtime.GC()
		runtime.ReadMemStats(&m)
		after := m.Alloc
		
		b.ReportMetric(float64(after-before)/(1024*1024), "MB")
		b.Logf("Memory used: %.2f MB", float64(after-before)/(1024*1024))
	})
	
	b.Run("PositionManager", func(b *testing.B) {
		var m runtime.MemStats
		
		runtime.GC()
		runtime.ReadMemStats(&m)
		before := m.Alloc
		
		// Create position manager
		posManager, err := position.NewPositionManager("./data/snapshots")
		if err != nil {
			b.Fatal(err)
		}
		defer posManager.Close()
		
		// Add 1000 positions
		for i := 0; i < 1000; i++ {
			pos := &position.Position{
				Symbol:     b.Name() + "_" + string(rune(i)),
				Exchange:   "binance",
				Market:     "spot",
				Side:       "LONG",
				Quantity:   decimal.NewFromFloat(0.1),
				EntryPrice: decimal.NewFromFloat(40000),
				MarkPrice:  decimal.NewFromFloat(42000),
				Leverage:   1,
				MarginUsed: decimal.NewFromFloat(4000),
			}
			posManager.UpdatePosition(pos)
		}
		
		runtime.GC()
		runtime.ReadMemStats(&m)
		after := m.Alloc
		
		b.ReportMetric(float64(after-before)/(1024*1024), "MB")
		b.Logf("Memory used: %.2f MB", float64(after-before)/(1024*1024))
	})
}

// BenchmarkGarbageCollection tests GC impact
func BenchmarkGarbageCollection(b *testing.B) {
	b.Run("WithoutGC", func(b *testing.B) {
		runtime.GC()
		start := time.Now()
		
		// Allocate memory
		data := make([][]byte, b.N)
		for i := 0; i < b.N; i++ {
			data[i] = make([]byte, 1024) // 1KB per allocation
		}
		
		elapsed := time.Since(start)
		b.ReportMetric(float64(elapsed.Nanoseconds())/float64(b.N), "ns/op")
	})
	
	b.Run("WithGC", func(b *testing.B) {
		start := time.Now()
		
		// Allocate memory with periodic GC
		for i := 0; i < b.N; i++ {
			_ = make([]byte, 1024) // 1KB per allocation
			if i%1000 == 0 {
				runtime.GC()
			}
		}
		
		elapsed := time.Since(start)
		b.ReportMetric(float64(elapsed.Nanoseconds())/float64(b.N), "ns/op")
	})
}

// BenchmarkMemoryPooling tests object pooling benefits
func BenchmarkMemoryPooling(b *testing.B) {
	type LargeObject struct {
		Data [1024]byte
		Values []decimal.Decimal
	}
	
	b.Run("WithoutPool", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			obj := &LargeObject{
				Values: make([]decimal.Decimal, 100),
			}
			// Use object
			obj.Data[0] = byte(i)
			// Object becomes garbage
		}
	})
	
	b.Run("WithPool", func(b *testing.B) {
		pool := &sync.Pool{
			New: func() interface{} {
				return &LargeObject{
					Values: make([]decimal.Decimal, 100),
				}
			},
		}
		
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			obj := pool.Get().(*LargeObject)
			// Use object
			obj.Data[0] = byte(i)
			// Reset and return to pool
			obj.Values = obj.Values[:0]
			pool.Put(obj)
		}
	})
}

// BenchmarkMemoryContention tests memory access under contention
func BenchmarkMemoryContention(b *testing.B) {
	// Shared data structure
	data := make([][]int, 100)
	for i := range data {
		data[i] = make([]int, 1000)
	}
	
	b.Run("Sequential", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			row := i % len(data)
			col := i % len(data[0])
			data[row][col] = i
		}
	})
	
	b.Run("Parallel", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				row := i % len(data)
				col := i % len(data[0])
				data[row][col] = i
				i++
			}
		})
	})
	
	b.Run("ParallelWithPadding", func(b *testing.B) {
		// Create data with cache line padding
		type PaddedInt struct {
			value int
			_pad  [7]int64 // 64 bytes total (cache line size)
		}
		
		paddedData := make([][]PaddedInt, 100)
		for i := range paddedData {
			paddedData[i] = make([]PaddedInt, 1000)
		}
		
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				row := i % len(paddedData)
				col := i % len(paddedData[0])
				paddedData[row][col].value = i
				i++
			}
		})
	})
}

// BenchmarkSharedMemoryAccess tests shared memory performance
func BenchmarkSharedMemoryAccess(b *testing.B) {
	// Simulate shared memory struct
	type SharedData struct {
		positions [1000]position.SharedMemoryPosition
		mu        sync.RWMutex
	}
	
	shared := &SharedData{}
	
	b.Run("Write", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				idx := i % len(shared.positions)
				shared.mu.Lock()
				shared.positions[idx].Quantity = float64(i)
				shared.positions[idx].UpdatedAt = time.Now().Unix()
				shared.mu.Unlock()
				i++
			}
		})
	})
	
	b.Run("Read", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				idx := i % len(shared.positions)
				shared.mu.RLock()
				_ = shared.positions[idx].Quantity
				_ = shared.positions[idx].UpdatedAt
				shared.mu.RUnlock()
				i++
			}
		})
	})
	
	b.Run("Mixed", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				idx := i % len(shared.positions)
				if i%10 == 0 {
					// Write
					shared.mu.Lock()
					shared.positions[idx].Quantity = float64(i)
					shared.mu.Unlock()
				} else {
					// Read
					shared.mu.RLock()
					_ = shared.positions[idx].Quantity
					shared.mu.RUnlock()
				}
				i++
			}
		})
	})
}

// BenchmarkMemoryBandwidth tests memory bandwidth
func BenchmarkMemoryBandwidth(b *testing.B) {
	sizes := []int{
		1 * 1024,        // 1KB
		64 * 1024,       // 64KB (L1 cache)
		256 * 1024,      // 256KB (L2 cache)
		8 * 1024 * 1024, // 8MB (L3 cache)
		64 * 1024 * 1024, // 64MB (RAM)
	}
	
	for _, size := range sizes {
		b.Run(b.Name()+"_"+string(rune(size/1024))+"KB", func(b *testing.B) {
			data := make([]byte, size)
			b.SetBytes(int64(size))
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				// Sequential read
				sum := byte(0)
				for j := 0; j < len(data); j++ {
					sum += data[j]
				}
				// Prevent optimization
				if sum == 255 {
					b.Fatal("unexpected sum")
				}
			}
		})
	}
}

// reportMemoryStats reports current memory statistics
func reportMemoryStats(b *testing.B, label string) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	b.Logf("%s Memory Stats:", label)
	b.Logf("  Alloc: %.2f MB", float64(m.Alloc)/(1024*1024))
	b.Logf("  TotalAlloc: %.2f MB", float64(m.TotalAlloc)/(1024*1024))
	b.Logf("  Sys: %.2f MB", float64(m.Sys)/(1024*1024))
	b.Logf("  NumGC: %d", m.NumGC)
	b.Logf("  GC CPU%%: %.2f", m.GCCPUFraction*100)
}