package performance

import (
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

// ObjectPool provides object pooling for reducing GC pressure
type ObjectPool[T any] struct {
	pool sync.Pool
	new  func() *T
}

func NewObjectPool[T any](newFunc func() *T) *ObjectPool[T] {
	return &ObjectPool[T]{
		pool: sync.Pool{
			New: func() interface{} {
				return newFunc()
			},
		},
		new: newFunc,
	}
}

func (p *ObjectPool[T]) Get() *T {
	return p.pool.Get().(*T)
}

func (p *ObjectPool[T]) Put(obj *T) {
	p.pool.Put(obj)
}

// SlicePool provides slice pooling for reducing allocations
type SlicePool[T any] struct {
	pools map[int]*sync.Pool
	mu    sync.RWMutex
}

func NewSlicePool[T any]() *SlicePool[T] {
	return &SlicePool[T]{
		pools: make(map[int]*sync.Pool),
	}
}

func (p *SlicePool[T]) Get(size int) []T {
	// Round up to next power of 2 for better pool utilization
	capacity := nextPowerOf2(size)
	
	p.mu.RLock()
	pool, exists := p.pools[capacity]
	p.mu.RUnlock()

	if !exists {
		p.mu.Lock()
		pool, exists = p.pools[capacity]
		if !exists {
			pool = &sync.Pool{
				New: func() interface{} {
					return make([]T, 0, capacity)
				},
			}
			p.pools[capacity] = pool
		}
		p.mu.Unlock()
	}

	slice := pool.Get().([]T)
	return slice[:size] // Reset length but keep capacity
}

func (p *SlicePool[T]) Put(slice []T) {
	if cap(slice) == 0 {
		return
	}
	
	capacity := cap(slice)
	
	p.mu.RLock()
	pool, exists := p.pools[capacity]
	p.mu.RUnlock()

	if exists {
		// Clear the slice content to prevent memory leaks
		for i := range slice {
			var zero T
			slice[i] = zero
		}
		slice = slice[:0] // Reset length
		pool.Put(slice)
	}
}

func nextPowerOf2(n int) int {
	if n <= 1 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	return n + 1
}

// ByteBufferPool for byte slice pooling (common use case)
type ByteBufferPool struct {
	small  sync.Pool // <= 1KB
	medium sync.Pool // <= 64KB
	large  sync.Pool // <= 1MB
}

func NewByteBufferPool() *ByteBufferPool {
	return &ByteBufferPool{
		small: sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, 1024) // 1KB
			},
		},
		medium: sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, 65536) // 64KB
			},
		},
		large: sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, 1048576) // 1MB
			},
		},
	}
}

func (p *ByteBufferPool) Get(size int) []byte {
	var buf []byte
	
	switch {
	case size <= 1024:
		buf = p.small.Get().([]byte)
	case size <= 65536:
		buf = p.medium.Get().([]byte)
	case size <= 1048576:
		buf = p.large.Get().([]byte)
	default:
		// For very large buffers, don't pool
		return make([]byte, size)
	}

	// Ensure buffer has the required capacity
	if cap(buf) < size {
		return make([]byte, size)
	}
	
	return buf[:size]
}

func (p *ByteBufferPool) Put(buf []byte) {
	if buf == nil {
		return
	}

	// Clear sensitive data
	for i := range buf {
		buf[i] = 0
	}
	buf = buf[:0]

	switch cap(buf) {
	case 1024:
		p.small.Put(buf)
	case 65536:
		p.medium.Put(buf)
	case 1048576:
		p.large.Put(buf)
	}
}

// GCTuner for automatic GC optimization
type GCTuner struct {
	targetPercent   int
	memStats        runtime.MemStats
	lastGCTime      time.Time
	gcPause         time.Duration
	adjustInterval  time.Duration
	monitoring      bool
	stopCh          chan struct{}
	mu              sync.RWMutex
}

func NewGCTuner() *GCTuner {
	return &GCTuner{
		targetPercent:  100, // Default GOGC
		adjustInterval: 30 * time.Second,
		stopCh:         make(chan struct{}),
	}
}

func (g *GCTuner) Start() {
	g.mu.Lock()
	if g.monitoring {
		g.mu.Unlock()
		return
	}
	g.monitoring = true
	g.mu.Unlock()

	go g.monitor()
}

func (g *GCTuner) Stop() {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	if !g.monitoring {
		return
	}
	
	g.monitoring = false
	close(g.stopCh)
}

func (g *GCTuner) monitor() {
	ticker := time.NewTicker(g.adjustInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			g.adjustGCPercent()
		case <-g.stopCh:
			return
		}
	}
}

func (g *GCTuner) adjustGCPercent() {
	runtime.ReadMemStats(&g.memStats)
	
	currentTime := time.Now()
	gcPause := time.Duration(g.memStats.PauseTotalNs)
	
	if !g.lastGCTime.IsZero() {
		g.gcPause = gcPause
	}
	
	g.lastGCTime = currentTime

	// Adjust based on memory pressure and GC pause times
	heapUsed := float64(g.memStats.Alloc)
	heapSize := float64(g.memStats.HeapSys)
	
	memoryPressure := heapUsed / heapSize
	avgGCPause := time.Duration(g.memStats.PauseTotalNs / uint64(g.memStats.NumGC))

	var newPercent int
	switch {
	case avgGCPause > 10*time.Millisecond:
		// GC pauses too long, reduce frequency
		newPercent = g.targetPercent + 50
	case memoryPressure > 0.8:
		// High memory pressure, increase GC frequency
		newPercent = g.targetPercent - 25
	case memoryPressure < 0.3 && avgGCPause < 1*time.Millisecond:
		// Low memory pressure and fast GC, reduce frequency
		newPercent = g.targetPercent + 25
	default:
		return // No adjustment needed
	}

	// Clamp values
	if newPercent < 50 {
		newPercent = 50
	}
	if newPercent > 500 {
		newPercent = 500
	}

	if newPercent != g.targetPercent {
		g.targetPercent = newPercent
		debug.SetGCPercent(newPercent)
	}
}

type GCStats struct {
	GOGC          int
	HeapAlloc     uint64
	HeapSys       uint64
	NumGC         uint32
	PauseTotalNs  uint64
	LastGCTime    time.Time
	MemoryPressure float64
	AvgGCPause    time.Duration
}

func (g *GCTuner) Stats() GCStats {
	g.mu.RLock()
	defer g.mu.RUnlock()

	runtime.ReadMemStats(&g.memStats)
	
	memoryPressure := float64(g.memStats.Alloc) / float64(g.memStats.HeapSys)
	var avgGCPause time.Duration
	if g.memStats.NumGC > 0 {
		avgGCPause = time.Duration(g.memStats.PauseTotalNs / uint64(g.memStats.NumGC))
	}

	return GCStats{
		GOGC:          g.targetPercent,
		HeapAlloc:     g.memStats.Alloc,
		HeapSys:       g.memStats.HeapSys,
		NumGC:         g.memStats.NumGC,
		PauseTotalNs:  g.memStats.PauseTotalNs,
		LastGCTime:    g.lastGCTime,
		MemoryPressure: memoryPressure,
		AvgGCPause:    avgGCPause,
	}
}

// Memory profiler for detecting leaks and optimizing allocation patterns
type MemoryProfiler struct {
	allocations     map[string]int64
	allocSize       map[string]int64
	mu              sync.RWMutex
	enabled         bool
}

func NewMemoryProfiler() *MemoryProfiler {
	return &MemoryProfiler{
		allocations: make(map[string]int64),
		allocSize:   make(map[string]int64),
		enabled:     true,
	}
}

func (m *MemoryProfiler) RecordAllocation(name string, size int64) {
	if !m.enabled {
		return
	}
	
	m.mu.Lock()
	m.allocations[name]++
	m.allocSize[name] += size
	m.mu.Unlock()
}

type AllocInfo struct {
	Name  string
	Count int64
	Size  int64
}

func (m *MemoryProfiler) TopAllocations(limit int) []AllocInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var infos []AllocInfo
	for name, count := range m.allocations {
		infos = append(infos, AllocInfo{
			Name:  name,
			Count: count,
			Size:  m.allocSize[name],
		})
	}

	// Simple bubble sort for top N
	for i := 0; i < len(infos) && i < limit; i++ {
		maxIdx := i
		for j := i + 1; j < len(infos); j++ {
			if infos[j].Size > infos[maxIdx].Size {
				maxIdx = j
			}
		}
		if maxIdx != i {
			infos[i], infos[maxIdx] = infos[maxIdx], infos[i]
		}
	}

	if len(infos) > limit {
		infos = infos[:limit]
	}

	return infos
}

func (m *MemoryProfiler) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.allocations = make(map[string]int64)
	m.allocSize = make(map[string]int64)
}

func (m *MemoryProfiler) Enable() {
	m.mu.Lock()
	m.enabled = true
	m.mu.Unlock()
}

func (m *MemoryProfiler) Disable() {
	m.mu.Lock()
	m.enabled = false
	m.mu.Unlock()
}