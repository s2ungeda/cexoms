package cache

import (
	"sync"
	"time"
)

type CacheItem struct {
	Value      interface{}
	Expiration int64
}

type MemoryCache struct {
	items sync.Map
	mu    sync.RWMutex
}

func NewMemoryCache() *MemoryCache {
	cache := &MemoryCache{}
	go cache.cleanupExpired()
	return cache
}

func (c *MemoryCache) Set(key string, value interface{}, ttl time.Duration) {
	expiration := time.Now().Add(ttl).UnixNano()
	if ttl == 0 {
		expiration = 0
	}
	
	c.items.Store(key, &CacheItem{
		Value:      value,
		Expiration: expiration,
	})
}

func (c *MemoryCache) Get(key string) (interface{}, bool) {
	item, exists := c.items.Load(key)
	if !exists {
		return nil, false
	}
	
	cacheItem := item.(*CacheItem)
	if cacheItem.Expiration > 0 && time.Now().UnixNano() > cacheItem.Expiration {
		c.items.Delete(key)
		return nil, false
	}
	
	return cacheItem.Value, true
}

func (c *MemoryCache) Delete(key string) {
	c.items.Delete(key)
}

func (c *MemoryCache) Clear() {
	c.items.Range(func(key, value interface{}) bool {
		c.items.Delete(key)
		return true
	})
}

func (c *MemoryCache) cleanupExpired() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		now := time.Now().UnixNano()
		c.items.Range(func(key, value interface{}) bool {
			item := value.(*CacheItem)
			if item.Expiration > 0 && now > item.Expiration {
				c.items.Delete(key)
			}
			return true
		})
	}
}

func (c *MemoryCache) GetAll() map[string]interface{} {
	result := make(map[string]interface{})
	c.items.Range(func(key, value interface{}) bool {
		item := value.(*CacheItem)
		if item.Expiration == 0 || time.Now().UnixNano() <= item.Expiration {
			result[key.(string)] = item.Value
		}
		return true
	})
	return result
}