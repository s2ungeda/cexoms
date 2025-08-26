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

// CacheLevel represents cache hierarchy levels
type CacheLevel int

const (
	L1Memory CacheLevel = iota
	L2Redis
	L3Database
)

// CacheStrategy defines caching behavior
type CacheStrategy interface {
	Get(ctx context.Context, key string) (interface{}, error)
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	InvalidatePattern(ctx context.Context, pattern string) error
}

// WriteThrough implements write-through caching strategy
type WriteThrough struct {
	l1Cache      *MemoryCache
	l2Cache      *RedisCache
	dbWriter     DatabaseWriter
	metrics      *CacheMetrics
	invalidator  *CacheInvalidator
}

// WriteBehind implements write-behind (write-back) caching strategy
type WriteBehind struct {
	l1Cache      *MemoryCache
	l2Cache      *RedisCache
	writeBuffer  *WriteBuffer
	dbWriter     DatabaseWriter
	metrics      *CacheMetrics
	flushTicker  *time.Ticker
	stopChan     chan struct{}
}

// CacheMetrics tracks cache performance
type CacheMetrics struct {
	hits        *prometheus.CounterVec
	misses      *prometheus.CounterVec
	evictions   *prometheus.CounterVec
	latency     *prometheus.HistogramVec
	size        *prometheus.GaugeVec
	writeBuffer *prometheus.GaugeVec
}

// MemoryCache provides L1 in-memory caching
type MemoryCache struct {
	mu       sync.RWMutex
	data     map[string]*CacheEntry
	maxSize  int
	currentSize int
	lru      *LRUList
}

// CacheEntry represents a cache entry
type CacheEntry struct {
	Key        string
	Value      interface{}
	ExpireAt   time.Time
	Size       int
	AccessTime time.Time
	AccessCount int64
}

// RedisCache provides L2 Redis caching
type RedisCache struct {
	client   *redis.Client
	pipeline *redis.Pipeline
	mu       sync.Mutex
}

// WriteBuffer buffers writes for write-behind strategy
type WriteBuffer struct {
	mu         sync.Mutex
	entries    map[string]*BufferEntry
	maxEntries int
	maxAge     time.Duration
}

// BufferEntry represents a buffered write
type BufferEntry struct {
	Key       string
	Value     interface{}
	Timestamp time.Time
	Attempts  int
}

// DatabaseWriter interface for database operations
type DatabaseWriter interface {
	Write(ctx context.Context, key string, value interface{}) error
	WriteBatch(ctx context.Context, entries map[string]interface{}) error
}

// CacheInvalidator handles cache invalidation
type CacheInvalidator struct {
	strategies []CacheStrategy
	patterns   sync.Map // pattern -> time.Time
}

// NewWriteThrough creates a write-through cache strategy
func NewWriteThrough(redisClient *redis.Client, dbWriter DatabaseWriter, maxMemorySize int) *WriteThrough {
	metrics := &CacheMetrics{
		hits: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cache_hits_total",
				Help: "Total number of cache hits",
			},
			[]string{"level", "operation"},
		),
		misses: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cache_misses_total",
				Help: "Total number of cache misses",
			},
			[]string{"level", "operation"},
		),
		evictions: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cache_evictions_total",
				Help: "Total number of cache evictions",
			},
			[]string{"level", "reason"},
		),
		latency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "cache_operation_duration_seconds",
				Help:    "Cache operation duration in seconds",
				Buckets: prometheus.ExponentialBuckets(0.00001, 2, 15),
			},
			[]string{"level", "operation"},
		),
		size: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "cache_size_bytes",
				Help: "Current cache size in bytes",
			},
			[]string{"level"},
		),
	}

	return &WriteThrough{
		l1Cache: NewMemoryCache(maxMemorySize),
		l2Cache: NewRedisCache(redisClient),
		dbWriter: dbWriter,
		metrics: metrics,
		invalidator: &CacheInvalidator{},
	}
}

// Get retrieves value from cache hierarchy
func (wt *WriteThrough) Get(ctx context.Context, key string) (interface{}, error) {
	start := time.Now()
	
	// Try L1 cache first
	if value, found := wt.l1Cache.Get(key); found {
		wt.metrics.hits.WithLabelValues("L1", "get").Inc()
		wt.metrics.latency.WithLabelValues("L1", "get").Observe(time.Since(start).Seconds())
		return value, nil
	}
	wt.metrics.misses.WithLabelValues("L1", "get").Inc()

	// Try L2 cache
	value, err := wt.l2Cache.Get(ctx, key)
	if err == nil && value != nil {
		wt.metrics.hits.WithLabelValues("L2", "get").Inc()
		wt.metrics.latency.WithLabelValues("L2", "get").Observe(time.Since(start).Seconds())
		
		// Populate L1 cache
		wt.l1Cache.Set(key, value, time.Hour)
		return value, nil
	}
	wt.metrics.misses.WithLabelValues("L2", "get").Inc()

	// Cache miss - would fetch from database here
	return nil, fmt.Errorf("cache miss for key: %s", key)
}

// Set writes value through all cache levels
func (wt *WriteThrough) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	start := time.Now()

	// Write to database first (write-through)
	if err := wt.dbWriter.Write(ctx, key, value); err != nil {
		return fmt.Errorf("database write failed: %w", err)
	}

	// Write to L2 cache
	if err := wt.l2Cache.Set(ctx, key, value, ttl); err != nil {
		// Log but don't fail - database write succeeded
	}

	// Write to L1 cache
	wt.l1Cache.Set(key, value, ttl)

	wt.metrics.latency.WithLabelValues("all", "set").Observe(time.Since(start).Seconds())
	return nil
}

// Delete removes value from all cache levels
func (wt *WriteThrough) Delete(ctx context.Context, key string) error {
	// Delete from all levels
	wt.l1Cache.Delete(key)
	wt.l2Cache.Delete(ctx, key)
	
	return nil
}

// InvalidatePattern invalidates cache entries matching pattern
func (wt *WriteThrough) InvalidatePattern(ctx context.Context, pattern string) error {
	return wt.invalidator.InvalidatePattern(ctx, pattern, wt)
}

// NewWriteBehind creates a write-behind cache strategy
func NewWriteBehind(redisClient *redis.Client, dbWriter DatabaseWriter, maxMemorySize int, flushInterval time.Duration) *WriteBehind {
	wb := &WriteBehind{
		l1Cache: NewMemoryCache(maxMemorySize),
		l2Cache: NewRedisCache(redisClient),
		writeBuffer: &WriteBuffer{
			entries:    make(map[string]*BufferEntry),
			maxEntries: 10000,
			maxAge:     5 * time.Minute,
		},
		dbWriter:    dbWriter,
		metrics:     &CacheMetrics{},
		stopChan:    make(chan struct{}),
	}

	// Start background flush process
	wb.flushTicker = time.NewTicker(flushInterval)
	go wb.backgroundFlush()

	return wb
}

// Set buffers writes for later database persistence
func (wb *WriteBehind) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	// Write to cache immediately
	wb.l1Cache.Set(key, value, ttl)
	wb.l2Cache.Set(ctx, key, value, ttl)

	// Buffer for database write
	wb.writeBuffer.Add(key, value)

	// Check if buffer needs flushing
	if wb.writeBuffer.NeedsFlush() {
		go wb.flushBuffer(ctx)
	}

	return nil
}

// backgroundFlush periodically flushes write buffer
func (wb *WriteBehind) backgroundFlush() {
	for {
		select {
		case <-wb.flushTicker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			wb.flushBuffer(ctx)
			cancel()
		case <-wb.stopChan:
			return
		}
	}
}

// flushBuffer writes buffered entries to database
func (wb *WriteBehind) flushBuffer(ctx context.Context) error {
	entries := wb.writeBuffer.GetAndClear()
	if len(entries) == 0 {
		return nil
	}

	// Convert to batch format
	batch := make(map[string]interface{})
	for k, v := range entries {
		batch[k] = v.Value
	}

	// Write batch to database
	if err := wb.dbWriter.WriteBatch(ctx, batch); err != nil {
		// Re-queue failed entries
		for k, v := range entries {
			v.Attempts++
			if v.Attempts < 3 {
				wb.writeBuffer.Add(k, v.Value)
			}
		}
		return err
	}

	wb.metrics.writeBuffer.WithLabelValues("flushed").Add(float64(len(entries)))
	return nil
}

// MemoryCache implementation
func NewMemoryCache(maxSize int) *MemoryCache {
	return &MemoryCache{
		data:    make(map[string]*CacheEntry),
		maxSize: maxSize,
		lru:     NewLRUList(),
	}
}

func (m *MemoryCache) Get(key string) (interface{}, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, exists := m.data[key]
	if !exists || entry.ExpireAt.Before(time.Now()) {
		return nil, false
	}

	entry.AccessTime = time.Now()
	entry.AccessCount++
	m.lru.MoveToFront(key)

	return entry.Value, true
}

func (m *MemoryCache) Set(key string, value interface{}, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Calculate size (simplified)
	size := len(key) + 64 // Rough estimate

	// Evict if necessary
	for m.currentSize+size > m.maxSize && m.lru.Len() > 0 {
		evictKey := m.lru.RemoveLast()
		if oldEntry, exists := m.data[evictKey]; exists {
			m.currentSize -= oldEntry.Size
			delete(m.data, evictKey)
		}
	}

	entry := &CacheEntry{
		Key:        key,
		Value:      value,
		ExpireAt:   time.Now().Add(ttl),
		Size:       size,
		AccessTime: time.Now(),
	}

	m.data[key] = entry
	m.currentSize += size
	m.lru.Add(key)
}

func (m *MemoryCache) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry, exists := m.data[key]; exists {
		m.currentSize -= entry.Size
		delete(m.data, key)
		m.lru.Remove(key)
	}
}

// RedisCache implementation
func NewRedisCache(client *redis.Client) *RedisCache {
	return &RedisCache{
		client: client,
	}
}

func (r *RedisCache) Get(ctx context.Context, key string) (interface{}, error) {
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Deserialize value
	var result interface{}
	if err := json.Unmarshal([]byte(val), &result); err != nil {
		return nil, err
	}

	return result, nil
}

func (r *RedisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	// Serialize value
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return r.client.Set(ctx, key, data, ttl).Err()
}

func (r *RedisCache) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

// WriteBuffer implementation
func (wb *WriteBuffer) Add(key string, value interface{}) {
	wb.mu.Lock()
	defer wb.mu.Unlock()

	wb.entries[key] = &BufferEntry{
		Key:       key,
		Value:     value,
		Timestamp: time.Now(),
	}
}

func (wb *WriteBuffer) GetAndClear() map[string]*BufferEntry {
	wb.mu.Lock()
	defer wb.mu.Unlock()

	entries := wb.entries
	wb.entries = make(map[string]*BufferEntry)
	return entries
}

func (wb *WriteBuffer) NeedsFlush() bool {
	wb.mu.Lock()
	defer wb.mu.Unlock()

	// Flush if buffer is full
	if len(wb.entries) >= wb.maxEntries {
		return true
	}

	// Flush if oldest entry exceeds max age
	for _, entry := range wb.entries {
		if time.Since(entry.Timestamp) > wb.maxAge {
			return true
		}
	}

	return false
}

// LRUList simple LRU implementation
type LRUList struct {
	mu    sync.Mutex
	items []string
}

func NewLRUList() *LRUList {
	return &LRUList{
		items: make([]string, 0),
	}
}

func (l *LRUList) Add(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items = append(l.items, key)
}

func (l *LRUList) Remove(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	for i, k := range l.items {
		if k == key {
			l.items = append(l.items[:i], l.items[i+1:]...)
			break
		}
	}
}

func (l *LRUList) RemoveLast() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	if len(l.items) == 0 {
		return ""
	}
	
	key := l.items[0]
	l.items = l.items[1:]
	return key
}

func (l *LRUList) MoveToFront(key string) {
	l.Remove(key)
	l.Add(key)
}

func (l *LRUList) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.items)
}

// CacheInvalidator implementation
func (ci *CacheInvalidator) InvalidatePattern(ctx context.Context, pattern string, strategy CacheStrategy) error {
	// Record invalidation pattern
	ci.patterns.Store(pattern, time.Now())

	// Invalidate matching entries
	// This is simplified - real implementation would scan keys
	
	return nil
}