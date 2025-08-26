package performance

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type Task func()

type WorkerPool struct {
	workCh        chan Task
	workerCount   int32
	maxWorkers    int32
	minWorkers    int32
	idleTimeout   time.Duration
	workers       sync.WaitGroup
	shutdown      chan struct{}
	shutdownOnce  sync.Once
	taskCount     int64
	completedTasks int64
	activeWorkers int32
	mu            sync.RWMutex
}

type PoolConfig struct {
	MinWorkers   int32
	MaxWorkers   int32
	BufferSize   int
	IdleTimeout  time.Duration
}

func DefaultPoolConfig() PoolConfig {
	numCPU := int32(runtime.NumCPU())
	return PoolConfig{
		MinWorkers:  numCPU,
		MaxWorkers:  numCPU * 4,
		BufferSize:  1000,
		IdleTimeout: 30 * time.Second,
	}
}

func NewWorkerPool(config PoolConfig) *WorkerPool {
	if config.MinWorkers <= 0 {
		config.MinWorkers = 1
	}
	if config.MaxWorkers < config.MinWorkers {
		config.MaxWorkers = config.MinWorkers
	}
	if config.BufferSize <= 0 {
		config.BufferSize = 100
	}
	if config.IdleTimeout <= 0 {
		config.IdleTimeout = 30 * time.Second
	}

	pool := &WorkerPool{
		workCh:      make(chan Task, config.BufferSize),
		minWorkers:  config.MinWorkers,
		maxWorkers:  config.MaxWorkers,
		idleTimeout: config.IdleTimeout,
		shutdown:    make(chan struct{}),
	}

	// Start minimum number of workers
	for i := int32(0); i < config.MinWorkers; i++ {
		pool.startWorker(true) // permanent workers
	}

	return pool
}

func (p *WorkerPool) startWorker(permanent bool) {
	atomic.AddInt32(&p.workerCount, 1)
	atomic.AddInt32(&p.activeWorkers, 1)
	p.workers.Add(1)

	go func() {
		defer func() {
			atomic.AddInt32(&p.workerCount, -1)
			atomic.AddInt32(&p.activeWorkers, -1)
			p.workers.Done()
		}()

		timer := time.NewTimer(p.idleTimeout)
		defer timer.Stop()

		for {
			select {
			case task := <-p.workCh:
				if task == nil {
					return // pool is shutting down
				}
				
				// Reset idle timer
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(p.idleTimeout)

				// Execute task
				func() {
					defer func() {
						if r := recover(); r != nil {
							// Log panic recovery if needed
							atomic.AddInt64(&p.completedTasks, 1)
						}
					}()
					task()
					atomic.AddInt64(&p.completedTasks, 1)
				}()

			case <-timer.C:
				// Worker idle timeout
				if !permanent && atomic.LoadInt32(&p.workerCount) > p.minWorkers {
					// This worker can exit
					return
				}
				timer.Reset(p.idleTimeout)

			case <-p.shutdown:
				return
			}
		}
	}()
}

func (p *WorkerPool) Submit(task Task) bool {
	if task == nil {
		return false
	}

	atomic.AddInt64(&p.taskCount, 1)

	select {
	case p.workCh <- task:
		return true
	default:
		// Channel is full, try to start additional worker
		currentWorkers := atomic.LoadInt32(&p.workerCount)
		if currentWorkers < p.maxWorkers {
			p.startWorker(false) // temporary worker
		}

		// Try again
		select {
		case p.workCh <- task:
			return true
		default:
			// Pool is overloaded
			atomic.AddInt64(&p.taskCount, -1)
			return false
		}
	}
}

func (p *WorkerPool) SubmitWithTimeout(task Task, timeout time.Duration) bool {
	if task == nil {
		return false
	}

	atomic.AddInt64(&p.taskCount, 1)

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case p.workCh <- task:
		return true
	case <-timer.C:
		atomic.AddInt64(&p.taskCount, -1)
		return false
	}
}

func (p *WorkerPool) SubmitWithContext(ctx context.Context, task Task) bool {
	if task == nil {
		return false
	}

	atomic.AddInt64(&p.taskCount, 1)

	select {
	case p.workCh <- task:
		return true
	case <-ctx.Done():
		atomic.AddInt64(&p.taskCount, -1)
		return false
	}
}

func (p *WorkerPool) Shutdown() {
	p.shutdownOnce.Do(func() {
		close(p.shutdown)
		close(p.workCh)
		p.workers.Wait()
	})
}

func (p *WorkerPool) ShutdownWithTimeout(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.Shutdown()
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

type PoolStats struct {
	WorkerCount     int32
	ActiveWorkers   int32
	QueueSize       int
	TaskCount       int64
	CompletedTasks  int64
	PendingTasks    int64
	Utilization     float64
}

func (p *WorkerPool) Stats() PoolStats {
	queueSize := len(p.workCh)
	taskCount := atomic.LoadInt64(&p.taskCount)
	completedTasks := atomic.LoadInt64(&p.completedTasks)
	pendingTasks := taskCount - completedTasks
	workerCount := atomic.LoadInt32(&p.workerCount)
	activeWorkers := atomic.LoadInt32(&p.activeWorkers)
	
	var utilization float64
	if workerCount > 0 {
		utilization = float64(activeWorkers) / float64(workerCount)
	}

	return PoolStats{
		WorkerCount:    workerCount,
		ActiveWorkers:  activeWorkers,
		QueueSize:      queueSize,
		TaskCount:      taskCount,
		CompletedTasks: completedTasks,
		PendingTasks:   pendingTasks,
		Utilization:    utilization,
	}
}

// Adaptive pool that automatically adjusts worker count based on load
type AdaptivePool struct {
	*WorkerPool
	monitoring   chan struct{}
	adjustPeriod time.Duration
}

func NewAdaptivePool(config PoolConfig) *AdaptivePool {
	pool := &AdaptivePool{
		WorkerPool:   NewWorkerPool(config),
		monitoring:   make(chan struct{}),
		adjustPeriod: 5 * time.Second,
	}

	go pool.monitor()
	return pool
}

func (p *AdaptivePool) monitor() {
	ticker := time.NewTicker(p.adjustPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.adjustWorkers()
		case <-p.monitoring:
			return
		case <-p.shutdown:
			return
		}
	}
}

func (p *AdaptivePool) adjustWorkers() {
	stats := p.Stats()
	queueSize := float64(stats.QueueSize)
	utilization := stats.Utilization
	currentWorkers := stats.WorkerCount

	// Scale up if queue is building up and utilization is high
	if queueSize > 50 && utilization > 0.8 && currentWorkers < p.maxWorkers {
		p.startWorker(false)
	}

	// Scale down if utilization is low and we have excess workers
	if utilization < 0.2 && currentWorkers > p.minWorkers {
		// Let natural idle timeout handle worker reduction
		// This prevents aggressive scaling down
	}
}

func (p *AdaptivePool) Shutdown() {
	close(p.monitoring)
	p.WorkerPool.Shutdown()
}

// FixedPool for scenarios where consistent worker count is needed
type FixedPool struct {
	*WorkerPool
}

func NewFixedPool(workers int32, bufferSize int) *FixedPool {
	config := PoolConfig{
		MinWorkers:  workers,
		MaxWorkers:  workers,
		BufferSize:  bufferSize,
		IdleTimeout: time.Hour, // Long timeout since workers are fixed
	}
	return &FixedPool{
		WorkerPool: NewWorkerPool(config),
	}
}