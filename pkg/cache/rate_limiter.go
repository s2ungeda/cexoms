package cache

import (
	"sync"
	"time"
)

type RateLimiter struct {
	mu       sync.Mutex
	counters map[string]*rateLimitCounter
	limit    int
	window   time.Duration
}

type rateLimitCounter struct {
	count      int
	windowStart time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		counters: make(map[string]*rateLimitCounter),
		limit:    limit,
		window:   window,
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	counter, exists := rl.counters[key]
	
	if !exists || now.Sub(counter.windowStart) > rl.window {
		rl.counters[key] = &rateLimitCounter{
			count:       1,
			windowStart: now,
		}
		return true
	}
	
	if counter.count < rl.limit {
		counter.count++
		return true
	}
	
	return false
}

func (rl *RateLimiter) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.counters, key)
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()
	
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, counter := range rl.counters {
			if now.Sub(counter.windowStart) > rl.window*2 {
				delete(rl.counters, key)
			}
		}
		rl.mu.Unlock()
	}
}