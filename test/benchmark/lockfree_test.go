package benchmark

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkAtomicCounter tests atomic counter performance
func BenchmarkAtomicCounter(b *testing.B) {
	var counter atomic.Int64
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			counter.Add(1)
		}
	})
	
	b.ReportMetric(float64(counter.Load())/b.Elapsed().Seconds(), "ops/s")
}

// BenchmarkMutexCounter tests mutex-based counter for comparison
func BenchmarkMutexCounter(b *testing.B) {
	var mu sync.Mutex
	var counter int64
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			counter++
			mu.Unlock()
		}
	})
	
	b.ReportMetric(float64(counter)/b.Elapsed().Seconds(), "ops/s")
}

// BenchmarkAtomicValue tests atomic.Value performance
func BenchmarkAtomicValue(b *testing.B) {
	var value atomic.Value
	value.Store(float64(42.0))
	
	b.Run("Store", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			v := float64(0)
			for pb.Next() {
				v++
				value.Store(v)
			}
		})
	})
	
	b.Run("Load", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = value.Load().(float64)
			}
		})
	})
	
	b.Run("Mixed", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			v := float64(0)
			for pb.Next() {
				if v%10 == 0 {
					value.Store(v)
				} else {
					_ = value.Load()
				}
				v++
			}
		})
	})
}

// BenchmarkSyncMap tests sync.Map performance
func BenchmarkSyncMap(b *testing.B) {
	var m sync.Map
	
	// Pre-populate
	for i := 0; i < 1000; i++ {
		m.Store(i, i*2)
	}
	
	b.Run("Store", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				m.Store(i%1000, i)
				i++
			}
		})
	})
	
	b.Run("Load", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				m.Load(i % 1000)
				i++
			}
		})
	})
	
	b.Run("LoadOrStore", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				m.LoadOrStore(i%1000, i)
				i++
			}
		})
	})
}

// BenchmarkChannel tests channel performance
func BenchmarkChannel(b *testing.B) {
	b.Run("Buffered-100", func(b *testing.B) {
		ch := make(chan int, 100)
		done := make(chan bool)
		
		go func() {
			for range ch {
			}
			done <- true
		}()
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ch <- i
		}
		close(ch)
		<-done
	})
	
	b.Run("Buffered-1000", func(b *testing.B) {
		ch := make(chan int, 1000)
		done := make(chan bool)
		
		go func() {
			for range ch {
			}
			done <- true
		}()
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ch <- i
		}
		close(ch)
		<-done
	})
}

// BenchmarkCAS tests Compare-And-Swap operations
func BenchmarkCAS(b *testing.B) {
	var value atomic.Int64
	
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			for {
				old := value.Load()
				if value.CompareAndSwap(old, old+1) {
					break
				}
			}
		}
	})
	
	b.ReportMetric(float64(value.Load())/b.Elapsed().Seconds(), "successful_cas/s")
}

// BenchmarkRingBuffer tests lock-free ring buffer
func BenchmarkRingBuffer(b *testing.B) {
	type RingBuffer struct {
		buffer   []interface{}
		capacity uint64
		head     atomic.Uint64
		tail     atomic.Uint64
	}
	
	rb := &RingBuffer{
		buffer:   make([]interface{}, 1024),
		capacity: 1024,
	}
	
	b.Run("SingleProducerSingleConsumer", func(b *testing.B) {
		b.ResetTimer()
		
		go func() {
			for i := 0; i < b.N; i++ {
				for {
					head := rb.head.Load()
					next := (head + 1) % rb.capacity
					if next != rb.tail.Load() {
						rb.buffer[head] = i
						rb.head.Store(next)
						break
					}
					runtime.Gosched()
				}
			}
		}()
		
		consumed := 0
		for consumed < b.N {
			tail := rb.tail.Load()
			if tail != rb.head.Load() {
				_ = rb.buffer[tail]
				rb.tail.Store((tail + 1) % rb.capacity)
				consumed++
			} else {
				runtime.Gosched()
			}
		}
	})
}

// BenchmarkMemoryBarrier tests memory ordering costs
func BenchmarkMemoryBarrier(b *testing.B) {
	var value atomic.Int64
	
	b.Run("Relaxed", func(b *testing.B) {
		// Go doesn't expose relaxed ordering, but this simulates it
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				value.Add(1)
			}
		})
	})
	
	b.Run("SeqCst", func(b *testing.B) {
		// Go's atomic operations use sequential consistency
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				value.Add(1)
				atomic.AddInt64((*int64)(&value), 0) // Force memory barrier
			}
		})
	})
}

// BenchmarkConcurrentMapPatterns tests common concurrent map patterns
func BenchmarkConcurrentMapPatterns(b *testing.B) {
	type Config struct {
		value string
	}
	
	b.Run("CopyOnWrite", func(b *testing.B) {
		var configPtr atomic.Value
		configPtr.Store(&Config{value: "initial"})
		
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				if i%100 == 0 {
					// Write (rare)
					newConfig := &Config{value: "updated"}
					configPtr.Store(newConfig)
				} else {
					// Read (common)
					config := configPtr.Load().(*Config)
					_ = config.value
				}
				i++
			}
		})
	})
	
	b.Run("ShardedMap", func(b *testing.B) {
		const shards = 32
		type ShardedMap struct {
			shards [shards]struct {
				mu sync.RWMutex
				m  map[int]int
			}
		}
		
		sm := &ShardedMap{}
		for i := 0; i < shards; i++ {
			sm.shards[i].m = make(map[int]int)
		}
		
		getShard := func(key int) int {
			return key % shards
		}
		
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				key := i % 10000
				shard := getShard(key)
				
				if i%10 == 0 {
					// Write
					sm.shards[shard].mu.Lock()
					sm.shards[shard].m[key] = i
					sm.shards[shard].mu.Unlock()
				} else {
					// Read
					sm.shards[shard].mu.RLock()
					_ = sm.shards[shard].m[key]
					sm.shards[shard].mu.RUnlock()
				}
				i++
			}
		})
	})
}

// BenchmarkSpinLock tests spinning vs blocking
func BenchmarkSpinLock(b *testing.B) {
	b.Run("SpinLock", func(b *testing.B) {
		var locked atomic.Bool
		
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				// Acquire
				for !locked.CompareAndSwap(false, true) {
					runtime.Gosched()
				}
				
				// Critical section
				time.Sleep(time.Nanosecond)
				
				// Release
				locked.Store(false)
			}
		})
	})
	
	b.Run("Mutex", func(b *testing.B) {
		var mu sync.Mutex
		
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				mu.Lock()
				time.Sleep(time.Nanosecond)
				mu.Unlock()
			}
		})
	})
}