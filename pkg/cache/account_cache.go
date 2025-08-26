package cache

import (
	"fmt"
	"sync"
	"time"
)

// AccountCache provides account-specific caching
type AccountCache struct {
	mu         sync.RWMutex
	mainCache  *MemoryCache
	accountMap map[string]*MemoryCache // Per-account caches
}

// NewAccountCache creates a new account-specific cache
func NewAccountCache() *AccountCache {
	return &AccountCache{
		mainCache:  NewMemoryCache(),
		accountMap: make(map[string]*MemoryCache),
	}
}

// GetAccountCache returns or creates a cache for specific account
func (ac *AccountCache) GetAccountCache(accountID string) *MemoryCache {
	ac.mu.RLock()
	cache, exists := ac.accountMap[accountID]
	ac.mu.RUnlock()
	
	if exists {
		return cache
	}
	
	// Create new cache for account
	ac.mu.Lock()
	defer ac.mu.Unlock()
	
	// Double-check after acquiring write lock
	if cache, exists = ac.accountMap[accountID]; exists {
		return cache
	}
	
	cache = NewMemoryCache()
	ac.accountMap[accountID] = cache
	return cache
}

// Set stores a value in account-specific cache
func (ac *AccountCache) Set(accountID, key string, value interface{}, ttl time.Duration) {
	cache := ac.GetAccountCache(accountID)
	cache.Set(key, value, ttl)
}

// Get retrieves a value from account-specific cache
func (ac *AccountCache) Get(accountID, key string) (interface{}, bool) {
	cache := ac.GetAccountCache(accountID)
	return cache.Get(key)
}

// Delete removes a value from account-specific cache
func (ac *AccountCache) Delete(accountID, key string) {
	if cache := ac.GetAccountCache(accountID); cache != nil {
		cache.Delete(key)
	}
}

// ClearAccount clears all cache for a specific account
func (ac *AccountCache) ClearAccount(accountID string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	
	if cache, exists := ac.accountMap[accountID]; exists {
		cache.Clear()
		delete(ac.accountMap, accountID)
	}
}

// GetAllAccounts returns all account IDs with caches
func (ac *AccountCache) GetAllAccounts() []string {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	
	accounts := make([]string, 0, len(ac.accountMap))
	for accountID := range ac.accountMap {
		accounts = append(accounts, accountID)
	}
	return accounts
}

// AccountCacheKey generates a cache key for account-specific data
func AccountCacheKey(namespace, key string) string {
	return fmt.Sprintf("%s:%s", namespace, key)
}

// Common account cache keys
const (
	CacheKeyBalance    = "balance"
	CacheKeyPositions  = "positions"
	CacheKeyOrders     = "orders"
	CacheKeyRateLimit  = "rate_limit"
	CacheKeyMetrics    = "metrics"
	CacheKeyLastUpdate = "last_update"
)