package performance

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// BatchProcessor handles batch processing for bulk operations
type BatchProcessor[T any] struct {
	// Configuration
	maxBatchSize    int
	maxBatchDelay   time.Duration
	maxConcurrency  int
	processFn       ProcessFunc[T]
	errorHandler    ErrorHandler[T]

	// Internal state
	queue           chan T
	batches         chan []T
	workers         sync.WaitGroup
	processorsWg    sync.WaitGroup
	ctx             context.Context
	cancel          context.CancelFunc

	// Metrics
	itemsProcessed  atomic.Int64
	batchesProcessed atomic.Int64
	errors          atomic.Int64
	queueSize       atomic.Int32
	activeWorkers   atomic.Int32

	// Buffer pool for batch slices
	bufferPool *sync.Pool
}

// ProcessFunc defines the function to process a batch of items
type ProcessFunc[T any] func(ctx context.Context, batch []T) error

// ErrorHandler handles errors during batch processing
type ErrorHandler[T any] func(ctx context.Context, batch []T, err error)

// BatchProcessorConfig holds configuration for the batch processor
type BatchProcessorConfig struct {
	MaxBatchSize   int
	MaxBatchDelay  time.Duration
	MaxConcurrency int
	QueueSize      int
}

// DefaultBatchConfig returns default batch processor configuration
func DefaultBatchConfig() BatchProcessorConfig {
	return BatchProcessorConfig{
		MaxBatchSize:   100,
		MaxBatchDelay:  100 * time.Millisecond,
		MaxConcurrency: 10,
		QueueSize:      10000,
	}
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor[T any](
	config BatchProcessorConfig,
	processFn ProcessFunc[T],
	errorHandler ErrorHandler[T],
) *BatchProcessor[T] {
	ctx, cancel := context.WithCancel(context.Background())

	bp := &BatchProcessor[T]{
		maxBatchSize:   config.MaxBatchSize,
		maxBatchDelay:  config.MaxBatchDelay,
		maxConcurrency: config.MaxConcurrency,
		processFn:      processFn,
		errorHandler:   errorHandler,
		queue:          make(chan T, config.QueueSize),
		batches:        make(chan []T, config.MaxConcurrency*2),
		ctx:            ctx,
		cancel:         cancel,
		bufferPool: &sync.Pool{
			New: func() interface{} {
				return make([]T, 0, config.MaxBatchSize)
			},
		},
	}

	// Start batch collector
	bp.workers.Add(1)
	go bp.batchCollector()

	// Start processors
	for i := 0; i < config.MaxConcurrency; i++ {
		bp.processorsWg.Add(1)
		go bp.processor()
	}

	return bp
}

// Add adds an item to be processed
func (bp *BatchProcessor[T]) Add(item T) error {
	select {
	case bp.queue <- item:
		bp.queueSize.Add(1)
		return nil
	case <-bp.ctx.Done():
		return fmt.Errorf("batch processor is shutting down")
	default:
		return fmt.Errorf("queue is full")
	}
}

// AddWithContext adds an item with context
func (bp *BatchProcessor[T]) AddWithContext(ctx context.Context, item T) error {
	select {
	case bp.queue <- item:
		bp.queueSize.Add(1)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-bp.ctx.Done():
		return fmt.Errorf("batch processor is shutting down")
	default:
		return fmt.Errorf("queue is full")
	}
}

// AddBatch adds multiple items at once
func (bp *BatchProcessor[T]) AddBatch(items []T) error {
	for _, item := range items {
		if err := bp.Add(item); err != nil {
			return fmt.Errorf("failed to add item: %w", err)
		}
	}
	return nil
}

// batchCollector collects items into batches
func (bp *BatchProcessor[T]) batchCollector() {
	defer bp.workers.Done()

	timer := time.NewTimer(bp.maxBatchDelay)
	timer.Stop()

	batch := bp.bufferPool.Get().([]T)
	batch = batch[:0]

	sendBatch := func() {
		if len(batch) > 0 {
			// Send batch for processing
			select {
			case bp.batches <- batch:
				// Get new batch buffer from pool
				batch = bp.bufferPool.Get().([]T)
				batch = batch[:0]
			case <-bp.ctx.Done():
				return
			}
		}
		timer.Stop()
	}

	for {
		select {
		case item := <-bp.queue:
			bp.queueSize.Add(-1)
			batch = append(batch, item)

			if len(batch) == 1 {
				// Start timer for first item in batch
				timer.Reset(bp.maxBatchDelay)
			}

			if len(batch) >= bp.maxBatchSize {
				sendBatch()
			}

		case <-timer.C:
			sendBatch()

		case <-bp.ctx.Done():
			// Process remaining items
			for len(batch) > 0 || len(bp.queue) > 0 {
				select {
				case item := <-bp.queue:
					bp.queueSize.Add(-1)
					batch = append(batch, item)
					if len(batch) >= bp.maxBatchSize {
						sendBatch()
					}
				default:
					sendBatch()
					close(bp.batches)
					return
				}
			}
			close(bp.batches)
			return
		}
	}
}

// processor processes batches
func (bp *BatchProcessor[T]) processor() {
	defer bp.processorsWg.Done()

	for batch := range bp.batches {
		bp.activeWorkers.Add(1)
		bp.processBatch(batch)
		bp.activeWorkers.Add(-1)

		// Return batch buffer to pool
		batch = batch[:0]
		bp.bufferPool.Put(batch)
	}
}

// processBatch processes a single batch
func (bp *BatchProcessor[T]) processBatch(batch []T) {
	if len(batch) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(bp.ctx, 30*time.Second)
	defer cancel()

	err := bp.processFn(ctx, batch)

	if err != nil {
		bp.errors.Add(1)
		if bp.errorHandler != nil {
			bp.errorHandler(ctx, batch, err)
		}
	} else {
		bp.itemsProcessed.Add(int64(len(batch)))
		bp.batchesProcessed.Add(1)
	}
}

// Flush processes any remaining items in the queue
func (bp *BatchProcessor[T]) Flush(ctx context.Context) error {
	// Stop accepting new items
	close(bp.queue)

	// Wait for batch collector to finish
	done := make(chan struct{})
	go func() {
		bp.workers.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Wait for all processors to finish
		bp.processorsWg.Wait()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop gracefully shuts down the batch processor
func (bp *BatchProcessor[T]) Stop(ctx context.Context) error {
	// Cancel context to signal shutdown
	bp.cancel()

	// Wait for workers to finish
	done := make(chan struct{})
	go func() {
		bp.workers.Wait()
		bp.processorsWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Metrics returns current metrics
func (bp *BatchProcessor[T]) Metrics() BatchProcessorMetrics {
	return BatchProcessorMetrics{
		ItemsProcessed:   bp.itemsProcessed.Load(),
		BatchesProcessed: bp.batchesProcessed.Load(),
		Errors:           bp.errors.Load(),
		QueueSize:        int(bp.queueSize.Load()),
		ActiveWorkers:    int(bp.activeWorkers.Load()),
	}
}

// BatchProcessorMetrics holds batch processor metrics
type BatchProcessorMetrics struct {
	ItemsProcessed   int64
	BatchesProcessed int64
	Errors           int64
	QueueSize        int
	ActiveWorkers    int
}

// DynamicBatchProcessor adjusts batch size based on processing speed
type DynamicBatchProcessor[T any] struct {
	*BatchProcessor[T]
	
	// Dynamic sizing
	minBatchSize     int
	targetLatency    time.Duration
	adjustInterval   time.Duration
	
	// Metrics for adjustment
	lastAdjustment   time.Time
	recentLatencies  []time.Duration
	latencyIndex     int
	mu               sync.Mutex
}

// NewDynamicBatchProcessor creates a batch processor with dynamic sizing
func NewDynamicBatchProcessor[T any](
	config BatchProcessorConfig,
	processFn ProcessFunc[T],
	errorHandler ErrorHandler[T],
	targetLatency time.Duration,
) *DynamicBatchProcessor[T] {
	dbp := &DynamicBatchProcessor[T]{
		BatchProcessor:  NewBatchProcessor(config, processFn, errorHandler),
		minBatchSize:    10,
		targetLatency:   targetLatency,
		adjustInterval:  5 * time.Second,
		recentLatencies: make([]time.Duration, 100),
		lastAdjustment:  time.Now(),
	}

	// Start adjustment routine
	go dbp.adjustBatchSize()

	return dbp
}

// processBatchWithTiming wraps processBatch to measure latency
func (dbp *DynamicBatchProcessor[T]) processBatchWithTiming(batch []T) {
	start := time.Now()
	dbp.BatchProcessor.processBatch(batch)
	latency := time.Since(start)

	dbp.mu.Lock()
	dbp.recentLatencies[dbp.latencyIndex] = latency
	dbp.latencyIndex = (dbp.latencyIndex + 1) % len(dbp.recentLatencies)
	dbp.mu.Unlock()
}

// adjustBatchSize periodically adjusts batch size based on latency
func (dbp *DynamicBatchProcessor[T]) adjustBatchSize() {
	ticker := time.NewTicker(dbp.adjustInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			dbp.performAdjustment()
		case <-dbp.ctx.Done():
			return
		}
	}
}

// performAdjustment adjusts batch size based on recent latencies
func (dbp *DynamicBatchProcessor[T]) performAdjustment() {
	dbp.mu.Lock()
	defer dbp.mu.Unlock()

	// Calculate average latency
	var totalLatency time.Duration
	count := 0
	for _, lat := range dbp.recentLatencies {
		if lat > 0 {
			totalLatency += lat
			count++
		}
	}

	if count == 0 {
		return
	}

	avgLatency := totalLatency / time.Duration(count)

	// Adjust batch size
	if avgLatency > dbp.targetLatency {
		// Decrease batch size
		newSize := int(float64(dbp.maxBatchSize) * 0.9)
		if newSize < dbp.minBatchSize {
			newSize = dbp.minBatchSize
		}
		dbp.maxBatchSize = newSize
	} else if avgLatency < dbp.targetLatency/2 {
		// Increase batch size
		newSize := int(float64(dbp.maxBatchSize) * 1.1)
		if newSize > dbp.maxBatchSize*2 {
			newSize = dbp.maxBatchSize * 2
		}
		dbp.maxBatchSize = newSize
	}

	dbp.lastAdjustment = time.Now()
}

// PriorityBatchProcessor processes items with priority
type PriorityBatchProcessor[T any] struct {
	highPriority   *BatchProcessor[T]
	normalPriority *BatchProcessor[T]
	lowPriority    *BatchProcessor[T]
}

// Priority levels for batch processing
type Priority int

const (
	PriorityLow Priority = iota
	PriorityNormal
	PriorityHigh
)

// NewPriorityBatchProcessor creates a processor with priority queues
func NewPriorityBatchProcessor[T any](
	config BatchProcessorConfig,
	processFn ProcessFunc[T],
	errorHandler ErrorHandler[T],
) *PriorityBatchProcessor[T] {
	// High priority gets more workers
	highConfig := config
	highConfig.MaxConcurrency = config.MaxConcurrency * 2

	// Low priority gets fewer workers
	lowConfig := config
	lowConfig.MaxConcurrency = config.MaxConcurrency / 2
	if lowConfig.MaxConcurrency < 1 {
		lowConfig.MaxConcurrency = 1
	}

	return &PriorityBatchProcessor[T]{
		highPriority:   NewBatchProcessor(highConfig, processFn, errorHandler),
		normalPriority: NewBatchProcessor(config, processFn, errorHandler),
		lowPriority:    NewBatchProcessor(lowConfig, processFn, errorHandler),
	}
}

// Add adds an item with the specified priority
func (pbp *PriorityBatchProcessor[T]) Add(item T, priority Priority) error {
	switch priority {
	case PriorityHigh:
		return pbp.highPriority.Add(item)
	case PriorityNormal:
		return pbp.normalPriority.Add(item)
	case PriorityLow:
		return pbp.lowPriority.Add(item)
	default:
		return pbp.normalPriority.Add(item)
	}
}

// Stop stops all priority processors
func (pbp *PriorityBatchProcessor[T]) Stop(ctx context.Context) error {
	errs := make(chan error, 3)

	go func() {
		errs <- pbp.highPriority.Stop(ctx)
	}()
	go func() {
		errs <- pbp.normalPriority.Stop(ctx)
	}()
	go func() {
		errs <- pbp.lowPriority.Stop(ctx)
	}()

	var firstErr error
	for i := 0; i < 3; i++ {
		if err := <-errs; err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}