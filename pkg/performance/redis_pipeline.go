package performance

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus"
)

// RedisPipeline provides optimized Redis operations using pipelining and Lua scripting
type RedisPipeline struct {
	client          *redis.Client
	scripts         map[string]*redis.Script
	batchProcessor  *BatchProcessor
	metrics         *PipelineMetrics
	mu              sync.RWMutex
}

// BatchProcessor handles batched Redis operations
type BatchProcessor struct {
	pipeline      redis.Pipeliner
	maxBatchSize  int
	flushInterval time.Duration
	operations    []Operation
	results       chan BatchResult
	mu            sync.Mutex
	stopChan      chan struct{}
}

// Operation represents a Redis operation
type Operation struct {
	ID       string
	Type     OperationType
	Key      string
	Value    interface{}
	TTL      time.Duration
	Callback func(interface{}, error)
}

// OperationType defines the type of Redis operation
type OperationType string

const (
	OpGet    OperationType = "GET"
	OpSet    OperationType = "SET"
	OpDel    OperationType = "DEL"
	OpIncr   OperationType = "INCR"
	OpHSet   OperationType = "HSET"
	OpHGet   OperationType = "HGET"
	OpZAdd   OperationType = "ZADD"
	OpZRange OperationType = "ZRANGE"
)

// BatchResult contains results from a batch operation
type BatchResult struct {
	Operations []Operation
	Results    []interface{}
	Errors     []error
}

// PipelineMetrics tracks pipeline performance
type PipelineMetrics struct {
	pipelineOps      *prometheus.CounterVec
	batchSize        *prometheus.HistogramVec
	pipelineDuration *prometheus.HistogramVec
	scriptExecution  *prometheus.HistogramVec
	errors           *prometheus.CounterVec
}

// LuaScripts contains pre-defined Lua scripts
var LuaScripts = map[string]string{
	"rateLimit": `
		local key = KEYS[1]
		local limit = tonumber(ARGV[1])
		local window = tonumber(ARGV[2])
		local current = redis.call('INCR', key)
		if current == 1 then
			redis.call('EXPIRE', key, window)
		end
		if current > limit then
			return 0
		end
		return current
	`,
	"multiGet": `
		local results = {}
		for i, key in ipairs(KEYS) do
			results[i] = redis.call('GET', key)
		end
		return results
	`,
	"conditionalSet": `
		local key = KEYS[1]
		local value = ARGV[1]
		local ttl = tonumber(ARGV[2])
		local condition = ARGV[3]
		
		local current = redis.call('GET', key)
		if condition == 'NX' and current then
			return 0
		elseif condition == 'XX' and not current then
			return 0
		end
		
		redis.call('SET', key, value, 'EX', ttl)
		return 1
	`,
	"atomicUpdate": `
		local key = KEYS[1]
		local field = ARGV[1]
		local increment = tonumber(ARGV[2])
		local max_value = tonumber(ARGV[3])
		
		local current = tonumber(redis.call('HGET', key, field) or 0)
		local new_value = current + increment
		
		if max_value > 0 and new_value > max_value then
			return {current, 0}
		end
		
		redis.call('HSET', key, field, new_value)
		return {new_value, 1}
	`,
	"bulkExpire": `
		local pattern = KEYS[1]
		local ttl = tonumber(ARGV[1])
		local cursor = "0"
		local count = 0
		
		repeat
			local result = redis.call('SCAN', cursor, 'MATCH', pattern, 'COUNT', 100)
			cursor = result[1]
			local keys = result[2]
			
			for i, key in ipairs(keys) do
				redis.call('EXPIRE', key, ttl)
				count = count + 1
			end
		until cursor == "0"
		
		return count
	`,
}

// NewRedisPipeline creates a new Redis pipeline manager
func NewRedisPipeline(client *redis.Client) *RedisPipeline {
	metrics := &PipelineMetrics{
		pipelineOps: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "redis_pipeline_operations_total",
				Help: "Total number of Redis pipeline operations",
			},
			[]string{"operation", "status"},
		),
		batchSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "redis_pipeline_batch_size",
				Help:    "Size of Redis pipeline batches",
				Buckets: prometheus.ExponentialBuckets(1, 2, 10),
			},
			[]string{"operation"},
		),
		pipelineDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "redis_pipeline_duration_seconds",
				Help:    "Duration of Redis pipeline operations",
				Buckets: prometheus.ExponentialBuckets(0.0001, 2, 15),
			},
			[]string{"operation"},
		),
		scriptExecution: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "redis_lua_script_duration_seconds",
				Help:    "Duration of Lua script executions",
				Buckets: prometheus.ExponentialBuckets(0.0001, 2, 15),
			},
			[]string{"script"},
		),
		errors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "redis_pipeline_errors_total",
				Help: "Total number of Redis pipeline errors",
			},
			[]string{"operation", "error_type"},
		),
	}

	rp := &RedisPipeline{
		client:  client,
		scripts: make(map[string]*redis.Script),
		metrics: metrics,
	}

	// Load Lua scripts
	rp.loadScripts()

	// Initialize batch processor
	rp.batchProcessor = &BatchProcessor{
		maxBatchSize:  100,
		flushInterval: 10 * time.Millisecond,
		results:       make(chan BatchResult, 100),
		stopChan:      make(chan struct{}),
	}

	// Start batch processor
	go rp.batchProcessor.run(client)

	return rp
}

// loadScripts loads and prepares Lua scripts
func (rp *RedisPipeline) loadScripts() {
	for name, script := range LuaScripts {
		rp.scripts[name] = redis.NewScript(script)
	}
}

// BatchSet performs batch SET operations
func (rp *RedisPipeline) BatchSet(ctx context.Context, items map[string]interface{}, ttl time.Duration) error {
	start := time.Now()
	defer func() {
		rp.metrics.pipelineDuration.WithLabelValues("batch_set").Observe(time.Since(start).Seconds())
		rp.metrics.batchSize.WithLabelValues("batch_set").Observe(float64(len(items)))
	}()

	pipe := rp.client.Pipeline()
	
	for key, value := range items {
		// Serialize value if necessary
		data, err := json.Marshal(value)
		if err != nil {
			rp.metrics.errors.WithLabelValues("batch_set", "marshal_error").Inc()
			continue
		}
		
		pipe.Set(ctx, key, data, ttl)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		rp.metrics.errors.WithLabelValues("batch_set", "exec_error").Inc()
		return fmt.Errorf("pipeline exec failed: %w", err)
	}

	rp.metrics.pipelineOps.WithLabelValues("batch_set", "success").Add(float64(len(items)))
	return nil
}

// BatchGet performs batch GET operations
func (rp *RedisPipeline) BatchGet(ctx context.Context, keys []string) (map[string]interface{}, error) {
	start := time.Now()
	defer func() {
		rp.metrics.pipelineDuration.WithLabelValues("batch_get").Observe(time.Since(start).Seconds())
		rp.metrics.batchSize.WithLabelValues("batch_get").Observe(float64(len(keys)))
	}()

	// Use Lua script for multi-get
	result, err := rp.scripts["multiGet"].Run(ctx, rp.client, keys).Result()
	if err != nil {
		rp.metrics.errors.WithLabelValues("batch_get", "script_error").Inc()
		return nil, fmt.Errorf("multi-get script failed: %w", err)
	}

	// Parse results
	values := result.([]interface{})
	resultMap := make(map[string]interface{})
	
	for i, key := range keys {
		if i < len(values) && values[i] != nil {
			// Deserialize value
			var decoded interface{}
			if err := json.Unmarshal([]byte(values[i].(string)), &decoded); err == nil {
				resultMap[key] = decoded
			}
		}
	}

	rp.metrics.pipelineOps.WithLabelValues("batch_get", "success").Add(float64(len(keys)))
	return resultMap, nil
}

// RateLimitedIncr performs rate-limited increment
func (rp *RedisPipeline) RateLimitedIncr(ctx context.Context, key string, limit int, window time.Duration) (int64, error) {
	start := time.Now()
	defer func() {
		rp.metrics.scriptExecution.WithLabelValues("rateLimit").Observe(time.Since(start).Seconds())
	}()

	result, err := rp.scripts["rateLimit"].Run(ctx, rp.client, []string{key}, limit, int(window.Seconds())).Result()
	if err != nil {
		rp.metrics.errors.WithLabelValues("rate_limit", "script_error").Inc()
		return 0, err
	}

	count := result.(int64)
	if count == 0 {
		return 0, fmt.Errorf("rate limit exceeded")
	}

	return count, nil
}

// ConditionalSet performs conditional SET operation
func (rp *RedisPipeline) ConditionalSet(ctx context.Context, key string, value interface{}, ttl time.Duration, condition string) (bool, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return false, err
	}

	result, err := rp.scripts["conditionalSet"].Run(ctx, rp.client, []string{key}, string(data), int(ttl.Seconds()), condition).Result()
	if err != nil {
		return false, err
	}

	return result.(int64) == 1, nil
}

// AtomicFieldUpdate performs atomic field update with constraints
func (rp *RedisPipeline) AtomicFieldUpdate(ctx context.Context, key, field string, increment int64, maxValue int64) (int64, bool, error) {
	result, err := rp.scripts["atomicUpdate"].Run(ctx, rp.client, []string{key}, field, increment, maxValue).Result()
	if err != nil {
		return 0, false, err
	}

	values := result.([]interface{})
	newValue := values[0].(int64)
	success := values[1].(int64) == 1

	return newValue, success, nil
}

// BulkExpire sets expiration for keys matching pattern
func (rp *RedisPipeline) BulkExpire(ctx context.Context, pattern string, ttl time.Duration) (int64, error) {
	result, err := rp.scripts["bulkExpire"].Run(ctx, rp.client, []string{pattern}, int(ttl.Seconds())).Result()
	if err != nil {
		return 0, err
	}

	return result.(int64), nil
}

// QueueOperation queues an operation for batch processing
func (rp *RedisPipeline) QueueOperation(op Operation) {
	rp.batchProcessor.queue(op)
}

// BatchProcessor implementation
func (bp *BatchProcessor) queue(op Operation) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	bp.operations = append(bp.operations, op)

	// Flush if batch is full
	if len(bp.operations) >= bp.maxBatchSize {
		go bp.flush()
	}
}

func (bp *BatchProcessor) run(client *redis.Client) {
	ticker := time.NewTicker(bp.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bp.flush()
		case <-bp.stopChan:
			bp.flush() // Final flush
			return
		}
	}
}

func (bp *BatchProcessor) flush() {
	bp.mu.Lock()
	if len(bp.operations) == 0 {
		bp.mu.Unlock()
		return
	}

	// Copy operations and clear buffer
	ops := make([]Operation, len(bp.operations))
	copy(ops, bp.operations)
	bp.operations = bp.operations[:0]
	bp.mu.Unlock()

	// Execute pipeline
	ctx := context.Background()
	pipe := bp.pipeline
	results := make([]redis.Cmder, 0, len(ops))

	for _, op := range ops {
		switch op.Type {
		case OpGet:
			results = append(results, pipe.Get(ctx, op.Key))
		case OpSet:
			data, _ := json.Marshal(op.Value)
			results = append(results, pipe.Set(ctx, op.Key, data, op.TTL))
		case OpDel:
			results = append(results, pipe.Del(ctx, op.Key))
		case OpIncr:
			results = append(results, pipe.Incr(ctx, op.Key))
		case OpHSet:
			results = append(results, pipe.HSet(ctx, op.Key, op.Value))
		case OpHGet:
			field := op.Value.(string)
			results = append(results, pipe.HGet(ctx, op.Key, field))
		}
	}

	// Execute pipeline
	_, err := pipe.Exec(ctx)

	// Process results
	batchResult := BatchResult{
		Operations: ops,
		Results:    make([]interface{}, len(results)),
		Errors:     make([]error, len(results)),
	}

	for i, cmd := range results {
		batchResult.Results[i] = cmd.(*redis.Cmd).Val()
		batchResult.Errors[i] = cmd.Err()

		// Execute callbacks
		if ops[i].Callback != nil {
			ops[i].Callback(batchResult.Results[i], batchResult.Errors[i])
		}
	}

	// Send batch result
	select {
	case bp.results <- batchResult:
	default:
		// Channel full, drop result
	}
}

// OptimizedScan performs optimized SCAN operation
func (rp *RedisPipeline) OptimizedScan(ctx context.Context, pattern string, count int64) ([]string, error) {
	var allKeys []string
	var cursor uint64

	for {
		keys, newCursor, err := rp.client.Scan(ctx, cursor, pattern, count).Result()
		if err != nil {
			return nil, err
		}

		allKeys = append(allKeys, keys...)

		if newCursor == 0 {
			break
		}
		cursor = newCursor
	}

	return allKeys, nil
}

// TransactionalUpdate performs transactional updates
func (rp *RedisPipeline) TransactionalUpdate(ctx context.Context, updates func(tx *redis.Tx) error) error {
	return rp.client.Watch(ctx, updates)
}

// Close closes the pipeline and batch processor
func (rp *RedisPipeline) Close() error {
	close(rp.batchProcessor.stopChan)
	return nil
}