package performance

import (
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

// RuntimeOptimizer manages Go runtime optimizations
type RuntimeOptimizer struct {
	config          OptimizerConfig
	gcTuner         *GCTuner
	workerPool      *AdaptivePool
	memoryProfiler  *MemoryProfiler
	running         bool
	stopCh          chan struct{}
	mu              sync.RWMutex
	metrics         RuntimeMetrics
}

type OptimizerConfig struct {
	// GC tuning
	EnableGCTuning     bool
	InitialGOGC        int
	
	// Worker pool
	EnableWorkerPool   bool
	MinWorkers         int32
	MaxWorkers         int32
	WorkerBufferSize   int
	
	// Memory profiling
	EnableMemProfiling bool
	
	// Runtime settings
	MaxProcs           int
	MaxThreads         int
	
	// Monitoring
	MonitoringInterval time.Duration
}

type RuntimeMetrics struct {
	// Runtime stats
	NumCPU         int
	NumGoroutines  int
	NumCgoCall     int64
	
	// Memory stats
	Alloc          uint64
	TotalAlloc     uint64
	Sys            uint64
	Mallocs        uint64
	Frees          uint64
	HeapAlloc      uint64
	HeapSys        uint64
	HeapIdle       uint64
	HeapInuse      uint64
	HeapReleased   uint64
	
	// GC stats
	NumGC          uint32
	PauseTotalNs   uint64
	LastGC         time.Time
	GOGC           int
	
	// Performance metrics
	Utilization    float64
	ThroughputTPS  float64
	LatencyP99     time.Duration
	
	// Timestamp
	Timestamp      time.Time
}

func DefaultOptimizerConfig() OptimizerConfig {
	numCPU := runtime.NumCPU()
	return OptimizerConfig{
		EnableGCTuning:     true,
		InitialGOGC:        100,
		EnableWorkerPool:   true,
		MinWorkers:         int32(numCPU),
		MaxWorkers:         int32(numCPU * 4),
		WorkerBufferSize:   1000,
		EnableMemProfiling: false, // Disabled by default due to overhead
		MaxProcs:           numCPU,
		MaxThreads:         10000,
		MonitoringInterval: 30 * time.Second,
	}
}

func NewRuntimeOptimizer(config OptimizerConfig) *RuntimeOptimizer {
	optimizer := &RuntimeOptimizer{
		config:         config,
		stopCh:         make(chan struct{}),
		memoryProfiler: NewMemoryProfiler(),
	}

	// Apply initial settings
	optimizer.applyInitialSettings()

	// Initialize components
	if config.EnableGCTuning {
		optimizer.gcTuner = NewGCTuner()
	}

	if config.EnableWorkerPool {
		poolConfig := PoolConfig{
			MinWorkers:  config.MinWorkers,
			MaxWorkers:  config.MaxWorkers,
			BufferSize:  config.WorkerBufferSize,
			IdleTimeout: 30 * time.Second,
		}
		optimizer.workerPool = NewAdaptivePool(poolConfig)
	}

	return optimizer
}

func (r *RuntimeOptimizer) applyInitialSettings() {
	// Set GOMAXPROCS
	if r.config.MaxProcs > 0 {
		runtime.GOMAXPROCS(r.config.MaxProcs)
	}

	// Set initial GOGC
	if r.config.InitialGOGC > 0 {
		debug.SetGCPercent(r.config.InitialGOGC)
	}

	// Set memory limit (if available in Go 1.19+)
	// This would be version-dependent code
	// debug.SetMemoryLimit(memoryLimit)
}

func (r *RuntimeOptimizer) Start() {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.mu.Unlock()

	// Start GC tuner
	if r.gcTuner != nil {
		r.gcTuner.Start()
	}

	// Start monitoring
	go r.monitor()
}

func (r *RuntimeOptimizer) Stop() {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	r.running = false
	r.mu.Unlock()

	// Stop GC tuner
	if r.gcTuner != nil {
		r.gcTuner.Stop()
	}

	// Stop worker pool
	if r.workerPool != nil {
		r.workerPool.Shutdown()
	}

	// Stop monitoring
	close(r.stopCh)
}

func (r *RuntimeOptimizer) monitor() {
	ticker := time.NewTicker(r.config.MonitoringInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.collectMetrics()
			r.optimizeRuntime()
		case <-r.stopCh:
			return
		}
	}
}

func (r *RuntimeOptimizer) collectMetrics() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	var gcStats GCStats
	if r.gcTuner != nil {
		gcStats = r.gcTuner.Stats()
	}

	var utilization float64
	if r.workerPool != nil {
		poolStats := r.workerPool.Stats()
		utilization = poolStats.Utilization
	}

	r.mu.Lock()
	r.metrics = RuntimeMetrics{
		// Runtime stats
		NumCPU:        runtime.NumCPU(),
		NumGoroutines: runtime.NumGoroutine(),
		NumCgoCall:    runtime.NumCgoCall(),
		
		// Memory stats
		Alloc:         memStats.Alloc,
		TotalAlloc:    memStats.TotalAlloc,
		Sys:           memStats.Sys,
		Mallocs:       memStats.Mallocs,
		Frees:         memStats.Frees,
		HeapAlloc:     memStats.HeapAlloc,
		HeapSys:       memStats.HeapSys,
		HeapIdle:      memStats.HeapIdle,
		HeapInuse:     memStats.HeapInuse,
		HeapReleased:  memStats.HeapReleased,
		
		// GC stats
		NumGC:         memStats.NumGC,
		PauseTotalNs:  memStats.PauseTotalNs,
		LastGC:        time.Unix(0, int64(memStats.LastGC)),
		GOGC:          gcStats.GOGC,
		
		// Performance metrics
		Utilization:   utilization,
		
		// Timestamp
		Timestamp:     time.Now(),
	}
	r.mu.Unlock()
}

func (r *RuntimeOptimizer) optimizeRuntime() {
	metrics := r.GetMetrics()

	// Detect goroutine leaks
	if metrics.NumGoroutines > 10000 {
		// Log warning about potential goroutine leak
		// Force GC to see if it helps
		runtime.GC()
	}

	// Memory pressure detection
	memoryPressure := float64(metrics.HeapAlloc) / float64(metrics.HeapSys)
	if memoryPressure > 0.9 {
		// High memory pressure, force GC
		runtime.GC()
	}

	// Detect thrashing (too many allocations)
	allocRate := metrics.Mallocs - metrics.Frees
	if allocRate > 1000000 { // 1M allocations per interval
		// High allocation rate, consider increasing GOGC temporarily
		if r.gcTuner != nil {
			// GC tuner will handle this
		}
	}
}

func (r *RuntimeOptimizer) GetMetrics() RuntimeMetrics {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.metrics
}

func (r *RuntimeOptimizer) GetWorkerPool() *AdaptivePool {
	return r.workerPool
}

func (r *RuntimeOptimizer) GetMemoryProfiler() *MemoryProfiler {
	return r.memoryProfiler
}

func (r *RuntimeOptimizer) ForceGC() {
	runtime.GC()
}

func (r *RuntimeOptimizer) TriggerMemoryOptimization() {
	// Force GC
	runtime.GC()
	
	// Return unused memory to OS
	debug.FreeOSMemory()
}

// High-level performance monitoring
type PerformanceMonitor struct {
	optimizer      *RuntimeOptimizer
	latencyTracker *LatencyTracker
	throughputTracker *ThroughputTracker
}

func NewPerformanceMonitor(config OptimizerConfig) *PerformanceMonitor {
	return &PerformanceMonitor{
		optimizer:      NewRuntimeOptimizer(config),
		latencyTracker: NewLatencyTracker(),
		throughputTracker: NewThroughputTracker(),
	}
}

func (p *PerformanceMonitor) Start() {
	p.optimizer.Start()
	p.latencyTracker.Start()
	p.throughputTracker.Start()
}

func (p *PerformanceMonitor) Stop() {
	p.optimizer.Stop()
	p.latencyTracker.Stop()
	p.throughputTracker.Stop()
}

func (p *PerformanceMonitor) RecordLatency(operation string, duration time.Duration) {
	p.latencyTracker.RecordLatency(operation, duration)
}

func (p *PerformanceMonitor) RecordThroughput(operation string, count int64) {
	p.throughputTracker.RecordThroughput(operation, count)
}

// Simple latency tracking
type LatencyTracker struct {
	latencies map[string][]time.Duration
	mu        sync.RWMutex
	running   bool
	stopCh    chan struct{}
}

func NewLatencyTracker() *LatencyTracker {
	return &LatencyTracker{
		latencies: make(map[string][]time.Duration),
		stopCh:    make(chan struct{}),
	}
}

func (l *LatencyTracker) Start() {
	l.mu.Lock()
	l.running = true
	l.mu.Unlock()
	
	// Cleanup old data periodically
	go l.cleanup()
}

func (l *LatencyTracker) Stop() {
	l.mu.Lock()
	if !l.running {
		l.mu.Unlock()
		return
	}
	l.running = false
	l.mu.Unlock()
	
	close(l.stopCh)
}

func (l *LatencyTracker) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.mu.Lock()
			// Keep only recent measurements
			for operation := range l.latencies {
				if len(l.latencies[operation]) > 1000 {
					// Keep only the latest 1000 measurements
					l.latencies[operation] = l.latencies[operation][len(l.latencies[operation])-1000:]
				}
			}
			l.mu.Unlock()
		case <-l.stopCh:
			return
		}
	}
}

func (l *LatencyTracker) RecordLatency(operation string, duration time.Duration) {
	l.mu.Lock()
	l.latencies[operation] = append(l.latencies[operation], duration)
	l.mu.Unlock()
}

func (l *LatencyTracker) GetP99(operation string) time.Duration {
	l.mu.RLock()
	latencies, exists := l.latencies[operation]
	if !exists || len(latencies) == 0 {
		l.mu.RUnlock()
		return 0
	}
	
	// Copy to avoid holding lock during sort
	data := make([]time.Duration, len(latencies))
	copy(data, latencies)
	l.mu.RUnlock()

	// Simple percentile calculation
	if len(data) == 1 {
		return data[0]
	}
	
	// Sort
	for i := 0; i < len(data)-1; i++ {
		for j := 0; j < len(data)-1-i; j++ {
			if data[j] > data[j+1] {
				data[j], data[j+1] = data[j+1], data[j]
			}
		}
	}
	
	p99Index := int(float64(len(data)) * 0.99)
	if p99Index >= len(data) {
		p99Index = len(data) - 1
	}
	
	return data[p99Index]
}

// Simple throughput tracking
type ThroughputTracker struct {
	counts    map[string]int64
	timestamps map[string]time.Time
	mu        sync.RWMutex
	running   bool
	stopCh    chan struct{}
}

func NewThroughputTracker() *ThroughputTracker {
	return &ThroughputTracker{
		counts:     make(map[string]int64),
		timestamps: make(map[string]time.Time),
		stopCh:     make(chan struct{}),
	}
}

func (t *ThroughputTracker) Start() {
	t.mu.Lock()
	t.running = true
	t.mu.Unlock()
}

func (t *ThroughputTracker) Stop() {
	t.mu.Lock()
	t.running = false
	t.mu.Unlock()
	close(t.stopCh)
}

func (t *ThroughputTracker) RecordThroughput(operation string, count int64) {
	t.mu.Lock()
	t.counts[operation] += count
	t.timestamps[operation] = time.Now()
	t.mu.Unlock()
}

func (t *ThroughputTracker) GetTPS(operation string) float64 {
	t.mu.RLock()
	count, countExists := t.counts[operation]
	timestamp, timeExists := t.timestamps[operation]
	t.mu.RUnlock()

	if !countExists || !timeExists {
		return 0
	}

	elapsed := time.Since(timestamp)
	if elapsed == 0 {
		return 0
	}

	return float64(count) / elapsed.Seconds()
}