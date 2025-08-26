package performance

import (
	"sync"
	"sync/atomic"
)

// BufferPool provides a pool of reusable byte buffers to reduce GC pressure
type BufferPool struct {
	pools   []*sync.Pool
	sizes   []int
	metrics *BufferPoolMetrics

	// Configuration
	maxBufferSize int
	minBufferSize int
}

// BufferPoolMetrics tracks buffer pool usage statistics
type BufferPoolMetrics struct {
	Gets       atomic.Int64
	Puts       atomic.Int64
	Misses     atomic.Int64
	Allocations atomic.Int64
	InUse      atomic.Int64
	TotalBytes atomic.Int64
}

// Buffer represents a pooled buffer
type Buffer struct {
	B    []byte
	pool *BufferPool
	size int
}

// NewBufferPool creates a new buffer pool with predefined size buckets
func NewBufferPool() *BufferPool {
	// Create size buckets: 512B, 1KB, 4KB, 16KB, 64KB, 256KB, 1MB
	sizes := []int{
		512,
		1024,
		4 * 1024,
		16 * 1024,
		64 * 1024,
		256 * 1024,
		1024 * 1024,
	}

	bp := &BufferPool{
		sizes:         sizes,
		pools:         make([]*sync.Pool, len(sizes)),
		metrics:       &BufferPoolMetrics{},
		minBufferSize: sizes[0],
		maxBufferSize: sizes[len(sizes)-1],
	}

	// Initialize sync.Pools for each size
	for i, size := range sizes {
		size := size // Capture loop variable
		bp.pools[i] = &sync.Pool{
			New: func() interface{} {
				bp.metrics.Allocations.Add(1)
				bp.metrics.TotalBytes.Add(int64(size))
				return &Buffer{
					B:    make([]byte, 0, size),
					pool: bp,
					size: size,
				}
			},
		}
	}

	return bp
}

// Get retrieves a buffer from the pool with at least the requested capacity
func (bp *BufferPool) Get(minSize int) *Buffer {
	bp.metrics.Gets.Add(1)
	bp.metrics.InUse.Add(1)

	// Find the appropriate pool
	poolIndex := bp.findPoolIndex(minSize)
	if poolIndex == -1 {
		// Size too large, allocate directly
		bp.metrics.Misses.Add(1)
		bp.metrics.Allocations.Add(1)
		bp.metrics.TotalBytes.Add(int64(minSize))
		return &Buffer{
			B:    make([]byte, 0, minSize),
			pool: bp,
			size: minSize,
		}
	}

	// Get buffer from pool
	buf := bp.pools[poolIndex].Get().(*Buffer)
	buf.B = buf.B[:0] // Reset length to 0
	return buf
}

// Put returns a buffer to the pool for reuse
func (bp *BufferPool) Put(buf *Buffer) {
	if buf == nil || buf.pool != bp {
		return
	}

	bp.metrics.Puts.Add(1)
	bp.metrics.InUse.Add(-1)

	// Don't pool buffers that are too large or too small
	if cap(buf.B) > bp.maxBufferSize || cap(buf.B) < bp.minBufferSize {
		return
	}

	// Find the appropriate pool
	poolIndex := bp.findPoolIndex(cap(buf.B))
	if poolIndex == -1 {
		return
	}

	// Reset buffer before returning to pool
	buf.B = buf.B[:0]
	bp.pools[poolIndex].Put(buf)
}

// findPoolIndex finds the appropriate pool index for a given size
func (bp *BufferPool) findPoolIndex(size int) int {
	for i, poolSize := range bp.sizes {
		if size <= poolSize {
			return i
		}
	}
	return -1
}

// GetBytes is a convenience method that returns just the byte slice
func (bp *BufferPool) GetBytes(minSize int) []byte {
	return bp.Get(minSize).B
}

// PutBytes is a convenience method for returning byte slices
func (bp *BufferPool) PutBytes(b []byte) {
	// This method is less efficient as we need to find the original buffer
	// In practice, use Get/Put with Buffer objects for better performance
	if cap(b) >= bp.minBufferSize && cap(b) <= bp.maxBufferSize {
		buf := &Buffer{
			B:    b[:0],
			pool: bp,
			size: cap(b),
		}
		bp.Put(buf)
	}
}

// Metrics returns current pool metrics
func (bp *BufferPool) Metrics() BufferPoolMetrics {
	return BufferPoolMetrics{
		Gets:        atomic.Int64{},
		Puts:        atomic.Int64{},
		Misses:      atomic.Int64{},
		Allocations: atomic.Int64{},
		InUse:       atomic.Int64{},
		TotalBytes:  atomic.Int64{},
	}
}

// Reset method for Buffer to clear contents efficiently
func (b *Buffer) Reset() {
	b.B = b.B[:0]
}

// Grow ensures the buffer has at least n bytes of capacity
func (b *Buffer) Grow(n int) {
	if cap(b.B) >= n {
		return
	}

	// Need to allocate a new buffer
	newCap := cap(b.B)
	for newCap < n {
		newCap *= 2
	}

	newBuf := make([]byte, len(b.B), newCap)
	copy(newBuf, b.B)
	b.B = newBuf
}

// Write implements io.Writer interface
func (b *Buffer) Write(p []byte) (n int, err error) {
	b.B = append(b.B, p...)
	return len(p), nil
}

// String returns the buffer contents as a string
func (b *Buffer) String() string {
	return string(b.B)
}

// Bytes returns the underlying byte slice
func (b *Buffer) Bytes() []byte {
	return b.B
}

// Len returns the length of the buffer
func (b *Buffer) Len() int {
	return len(b.B)
}

// Cap returns the capacity of the buffer
func (b *Buffer) Cap() int {
	return cap(b.B)
}

// ByteBufferPool is a specialized pool for small byte arrays
type ByteBufferPool struct {
	pool sync.Pool
}

// NewByteBufferPool creates a pool for small byte buffers
func NewByteBufferPool(size int) *ByteBufferPool {
	return &ByteBufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, size)
			},
		},
	}
}

// Get retrieves a byte buffer from the pool
func (p *ByteBufferPool) Get() []byte {
	return p.pool.Get().([]byte)
}

// Put returns a byte buffer to the pool
func (p *ByteBufferPool) Put(b []byte) {
	// Clear sensitive data before returning to pool
	for i := range b {
		b[i] = 0
	}
	p.pool.Put(b)
}

// SlicePool manages pools of slices with different capacities
type SlicePool struct {
	pools map[int]*sync.Pool
	mu    sync.RWMutex
}

// NewSlicePool creates a new slice pool
func NewSlicePool() *SlicePool {
	return &SlicePool{
		pools: make(map[int]*sync.Pool),
	}
}

// GetIntSlice gets an int slice with the specified capacity
func (sp *SlicePool) GetIntSlice(cap int) []int {
	sp.mu.RLock()
	pool, exists := sp.pools[cap]
	sp.mu.RUnlock()

	if !exists {
		sp.mu.Lock()
		pool, exists = sp.pools[cap]
		if !exists {
			pool = &sync.Pool{
				New: func() interface{} {
					return make([]int, 0, cap)
				},
			}
			sp.pools[cap] = pool
		}
		sp.mu.Unlock()
	}

	return pool.Get().([]int)[:0]
}

// PutIntSlice returns an int slice to the pool
func (sp *SlicePool) PutIntSlice(s []int) {
	cap := cap(s)
	sp.mu.RLock()
	pool, exists := sp.pools[cap]
	sp.mu.RUnlock()

	if exists {
		pool.Put(s[:0])
	}
}

// GetStringSlice gets a string slice with the specified capacity
func (sp *SlicePool) GetStringSlice(cap int) []string {
	sp.mu.RLock()
	pool, exists := sp.pools[-cap] // Use negative to distinguish from int slices
	sp.mu.RUnlock()

	if !exists {
		sp.mu.Lock()
		pool, exists = sp.pools[-cap]
		if !exists {
			pool = &sync.Pool{
				New: func() interface{} {
					return make([]string, 0, cap)
				},
			}
			sp.pools[-cap] = pool
		}
		sp.mu.Unlock()
	}

	return pool.Get().([]string)[:0]
}

// PutStringSlice returns a string slice to the pool
func (sp *SlicePool) PutStringSlice(s []string) {
	cap := cap(s)
	sp.mu.RLock()
	pool, exists := sp.pools[-cap]
	sp.mu.RUnlock()

	if exists {
		// Clear strings to help GC
		for i := range s {
			s[i] = ""
		}
		pool.Put(s[:0])
	}
}