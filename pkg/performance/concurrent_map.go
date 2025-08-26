package performance

import (
	"fmt"
	"hash/fnv"
	"sync"
	"sync/atomic"
)

const (
	// DefaultShardCount is the default number of shards
	DefaultShardCount = 32
	// MaxShardCount is the maximum number of shards
	MaxShardCount = 1024
)

// ConcurrentMap is a high-performance concurrent map implementation
// using sharding to reduce lock contention
type ConcurrentMap[K comparable, V any] struct {
	shards    []*mapShard[K, V]
	shardMask uint32
	count     atomic.Int64
}

// mapShard represents a single shard in the concurrent map
type mapShard[K comparable, V any] struct {
	mu    sync.RWMutex
	items map[K]V
}

// NewConcurrentMap creates a new concurrent map with the specified number of shards
func NewConcurrentMap[K comparable, V any](shardCount int) *ConcurrentMap[K, V] {
	if shardCount <= 0 {
		shardCount = DefaultShardCount
	}
	if shardCount > MaxShardCount {
		shardCount = MaxShardCount
	}

	// Ensure shard count is a power of 2
	shardCount = nextPowerOfTwo(shardCount)

	cm := &ConcurrentMap[K, V]{
		shards:    make([]*mapShard[K, V], shardCount),
		shardMask: uint32(shardCount - 1),
	}

	for i := range cm.shards {
		cm.shards[i] = &mapShard[K, V]{
			items: make(map[K]V),
		}
	}

	return cm
}

// getShard returns the shard for a given key
func (m *ConcurrentMap[K, V]) getShard(key K) *mapShard[K, V] {
	hash := hashKey(key)
	return m.shards[hash&m.shardMask]
}

// Set stores a key-value pair in the map
func (m *ConcurrentMap[K, V]) Set(key K, value V) {
	shard := m.getShard(key)
	shard.mu.Lock()
	
	_, exists := shard.items[key]
	shard.items[key] = value
	
	shard.mu.Unlock()

	if !exists {
		m.count.Add(1)
	}
}

// Get retrieves a value from the map
func (m *ConcurrentMap[K, V]) Get(key K) (V, bool) {
	shard := m.getShard(key)
	shard.mu.RLock()
	value, exists := shard.items[key]
	shard.mu.RUnlock()
	return value, exists
}

// GetOrSet retrieves a value or sets it if not present
func (m *ConcurrentMap[K, V]) GetOrSet(key K, value V) (V, bool) {
	shard := m.getShard(key)
	
	// Try read lock first
	shard.mu.RLock()
	if existingValue, exists := shard.items[key]; exists {
		shard.mu.RUnlock()
		return existingValue, true
	}
	shard.mu.RUnlock()

	// Need write lock
	shard.mu.Lock()
	// Double-check after acquiring write lock
	if existingValue, exists := shard.items[key]; exists {
		shard.mu.Unlock()
		return existingValue, true
	}
	
	shard.items[key] = value
	shard.mu.Unlock()
	
	m.count.Add(1)
	return value, false
}

// Delete removes a key from the map
func (m *ConcurrentMap[K, V]) Delete(key K) bool {
	shard := m.getShard(key)
	shard.mu.Lock()
	
	_, exists := shard.items[key]
	if exists {
		delete(shard.items, key)
		m.count.Add(-1)
	}
	
	shard.mu.Unlock()
	return exists
}

// Has checks if a key exists in the map
func (m *ConcurrentMap[K, V]) Has(key K) bool {
	shard := m.getShard(key)
	shard.mu.RLock()
	_, exists := shard.items[key]
	shard.mu.RUnlock()
	return exists
}

// Len returns the number of items in the map
func (m *ConcurrentMap[K, V]) Len() int {
	return int(m.count.Load())
}

// Clear removes all items from the map
func (m *ConcurrentMap[K, V]) Clear() {
	for _, shard := range m.shards {
		shard.mu.Lock()
		shard.items = make(map[K]V)
		shard.mu.Unlock()
	}
	m.count.Store(0)
}

// Keys returns all keys in the map
func (m *ConcurrentMap[K, V]) Keys() []K {
	keys := make([]K, 0, m.Len())
	
	for _, shard := range m.shards {
		shard.mu.RLock()
		for k := range shard.items {
			keys = append(keys, k)
		}
		shard.mu.RUnlock()
	}
	
	return keys
}

// Values returns all values in the map
func (m *ConcurrentMap[K, V]) Values() []V {
	values := make([]V, 0, m.Len())
	
	for _, shard := range m.shards {
		shard.mu.RLock()
		for _, v := range shard.items {
			values = append(values, v)
		}
		shard.mu.RUnlock()
	}
	
	return values
}

// Range calls f for each key-value pair in the map
// If f returns false, iteration stops
func (m *ConcurrentMap[K, V]) Range(f func(key K, value V) bool) {
	for _, shard := range m.shards {
		shard.mu.RLock()
		for k, v := range shard.items {
			if !f(k, v) {
				shard.mu.RUnlock()
				return
			}
		}
		shard.mu.RUnlock()
	}
}

// Update atomically updates a value
func (m *ConcurrentMap[K, V]) Update(key K, updateFn func(value V, exists bool) V) {
	shard := m.getShard(key)
	shard.mu.Lock()
	
	oldValue, exists := shard.items[key]
	newValue := updateFn(oldValue, exists)
	shard.items[key] = newValue
	
	if !exists {
		m.count.Add(1)
	}
	
	shard.mu.Unlock()
}

// BatchSet sets multiple key-value pairs efficiently
func (m *ConcurrentMap[K, V]) BatchSet(items map[K]V) {
	// Group items by shard to minimize lock acquisitions
	shardItems := make(map[uint32]map[K]V)
	
	for k, v := range items {
		hash := hashKey(k)
		shardIdx := hash & m.shardMask
		
		if shardItems[shardIdx] == nil {
			shardItems[shardIdx] = make(map[K]V)
		}
		shardItems[shardIdx][k] = v
	}
	
	// Update each shard
	for shardIdx, items := range shardItems {
		shard := m.shards[shardIdx]
		shard.mu.Lock()
		
		newCount := 0
		for k, v := range items {
			if _, exists := shard.items[k]; !exists {
				newCount++
			}
			shard.items[k] = v
		}
		
		shard.mu.Unlock()
		
		if newCount > 0 {
			m.count.Add(int64(newCount))
		}
	}
}

// BatchDelete deletes multiple keys efficiently
func (m *ConcurrentMap[K, V]) BatchDelete(keys []K) int {
	// Group keys by shard
	shardKeys := make(map[uint32][]K)
	
	for _, k := range keys {
		hash := hashKey(k)
		shardIdx := hash & m.shardMask
		shardKeys[shardIdx] = append(shardKeys[shardIdx], k)
	}
	
	deletedCount := 0
	
	// Delete from each shard
	for shardIdx, keys := range shardKeys {
		shard := m.shards[shardIdx]
		shard.mu.Lock()
		
		for _, k := range keys {
			if _, exists := shard.items[k]; exists {
				delete(shard.items, k)
				deletedCount++
			}
		}
		
		shard.mu.Unlock()
	}
	
	if deletedCount > 0 {
		m.count.Add(-int64(deletedCount))
	}
	
	return deletedCount
}

// Snapshot returns a consistent snapshot of the map
func (m *ConcurrentMap[K, V]) Snapshot() map[K]V {
	snapshot := make(map[K]V, m.Len())
	
	// Lock all shards in order to prevent deadlock
	for _, shard := range m.shards {
		shard.mu.RLock()
		defer shard.mu.RUnlock()
	}
	
	// Copy all items
	for _, shard := range m.shards {
		for k, v := range shard.items {
			snapshot[k] = v
		}
	}
	
	return snapshot
}

// hashKey generates a hash for any comparable key type
func hashKey[K comparable](key K) uint32 {
	h := fnv.New32a()
	// Convert key to string for hashing
	// This is not the most efficient but works for all comparable types
	fmt.Fprintf(h, "%v", key)
	return h.Sum32()
}

// nextPowerOfTwo returns the next power of two greater than or equal to n
func nextPowerOfTwo(n int) int {
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n++
	return n
}

// ConcurrentSet is a thread-safe set implementation
type ConcurrentSet[T comparable] struct {
	m *ConcurrentMap[T, struct{}]
}

// NewConcurrentSet creates a new concurrent set
func NewConcurrentSet[T comparable](shardCount int) *ConcurrentSet[T] {
	return &ConcurrentSet[T]{
		m: NewConcurrentMap[T, struct{}](shardCount),
	}
}

// Add adds an item to the set
func (s *ConcurrentSet[T]) Add(item T) bool {
	_, exists := s.m.GetOrSet(item, struct{}{})
	return !exists
}

// Remove removes an item from the set
func (s *ConcurrentSet[T]) Remove(item T) bool {
	return s.m.Delete(item)
}

// Contains checks if an item is in the set
func (s *ConcurrentSet[T]) Contains(item T) bool {
	return s.m.Has(item)
}

// Size returns the number of items in the set
func (s *ConcurrentSet[T]) Size() int {
	return s.m.Len()
}

// Clear removes all items from the set
func (s *ConcurrentSet[T]) Clear() {
	s.m.Clear()
}

// ToSlice returns all items as a slice
func (s *ConcurrentSet[T]) ToSlice() []T {
	return s.m.Keys()
}

// Union returns a new set containing items from both sets
func (s *ConcurrentSet[T]) Union(other *ConcurrentSet[T]) *ConcurrentSet[T] {
	result := NewConcurrentSet[T](DefaultShardCount)
	
	s.m.Range(func(key T, _ struct{}) bool {
		result.Add(key)
		return true
	})
	
	other.m.Range(func(key T, _ struct{}) bool {
		result.Add(key)
		return true
	})
	
	return result
}

// Intersection returns a new set containing items present in both sets
func (s *ConcurrentSet[T]) Intersection(other *ConcurrentSet[T]) *ConcurrentSet[T] {
	result := NewConcurrentSet[T](DefaultShardCount)
	
	// Iterate over the smaller set for efficiency
	if s.Size() <= other.Size() {
		s.m.Range(func(key T, _ struct{}) bool {
			if other.Contains(key) {
				result.Add(key)
			}
			return true
		})
	} else {
		other.m.Range(func(key T, _ struct{}) bool {
			if s.Contains(key) {
				result.Add(key)
			}
			return true
		})
	}
	
	return result
}