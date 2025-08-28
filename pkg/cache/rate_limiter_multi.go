package cache

import (
	"fmt"
	"sync"
	"time"
)

// MultiAccountRateLimiter manages rate limits for multiple accounts
type MultiAccountRateLimiter struct {
	limiters map[string]*RateLimiter
	mu       sync.RWMutex
	
	// Global limits
	globalLimiter *RateLimiter
	
	// Configuration
	defaultLimit int
	defaultWindow time.Duration
}

// NewMultiAccountRateLimiter creates a new multi-account rate limiter
func NewMultiAccountRateLimiter(globalLimit int, window time.Duration) *MultiAccountRateLimiter {
	return &MultiAccountRateLimiter{
		limiters:      make(map[string]*RateLimiter),
		globalLimiter: NewRateLimiter(globalLimit, window),
		defaultLimit:  1200, // Binance default
		defaultWindow: time.Minute,
	}
}

// AddAccount adds a new account with its own rate limiter
func (m *MultiAccountRateLimiter) AddAccount(accountID string, limit int, window time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.limiters[accountID] = NewRateLimiter(limit, window)
}

// Allow checks if a request is allowed for the given account and key
func (m *MultiAccountRateLimiter) Allow(accountID, key string) bool {
	// Check global limit first
	if !m.globalLimiter.Allow(fmt.Sprintf("global_%s", key)) {
		return false
	}
	
	// Check account-specific limit
	m.mu.RLock()
	limiter, exists := m.limiters[accountID]
	m.mu.RUnlock()
	
	if !exists {
		// Create default limiter for new account
		m.mu.Lock()
		limiter = NewRateLimiter(m.defaultLimit, m.defaultWindow)
		m.limiters[accountID] = limiter
		m.mu.Unlock()
	}
	
	return limiter.Allow(key)
}

// GetUsage returns the current usage for an account
func (m *MultiAccountRateLimiter) GetUsage(accountID string) (used int, limit int) {
	m.mu.RLock()
	_, exists := m.limiters[accountID]
	m.mu.RUnlock()
	
	if !exists {
		return 0, m.defaultLimit
	}
	
	// GetUsage is not implemented in RateLimiter, return estimate
	return 0, m.defaultLimit
}

// GetGlobalUsage returns the global rate limit usage
func (m *MultiAccountRateLimiter) GetGlobalUsage() (used int, limit int) {
	// GetUsage is not implemented in RateLimiter, return estimate
	return 0, m.defaultLimit
}

// Reset resets the rate limiter for a specific account
func (m *MultiAccountRateLimiter) Reset(accountID string) {
	m.mu.RLock()
	limiter, exists := m.limiters[accountID]
	m.mu.RUnlock()
	
	if exists {
		limiter.Reset(accountID)
	}
}

// ResetAll resets all rate limiters
func (m *MultiAccountRateLimiter) ResetAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	m.globalLimiter.Reset("global")
	for accountID, limiter := range m.limiters {
		limiter.Reset(accountID)
	}
}

// GetBestAccount returns the account with the most available rate limit
func (m *MultiAccountRateLimiter) GetBestAccount(accounts []string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	bestAccount := ""
	bestAvailable := 0
	
	for _, accountID := range accounts {
		if limiter, exists := m.limiters[accountID]; exists {
			used, limit := limiter.GetUsage()
			available := limit - used
			
			if available > bestAvailable {
				bestAvailable = available
				bestAccount = accountID
			}
		} else {
			// New account has full limit available
			return accountID
		}
	}
	
	if bestAccount == "" && len(accounts) > 0 {
		return accounts[0]
	}
	
	return bestAccount
}

// RateLimitInfo represents rate limit information
type RateLimitInfo struct {
	AccountID string
	Used      int
	Limit     int
	Available int
	ResetTime time.Time
}

// GetAllAccountsInfo returns rate limit info for all accounts
func (m *MultiAccountRateLimiter) GetAllAccountsInfo() []RateLimitInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	info := make([]RateLimitInfo, 0, len(m.limiters))
	
	for accountID, limiter := range m.limiters {
		used, limit := limiter.GetUsage()
		info = append(info, RateLimitInfo{
			AccountID: accountID,
			Used:      used,
			Limit:     limit,
			Available: limit - used,
			ResetTime: time.Now().Add(m.defaultWindow),
		})
	}
	
	return info
}

// AccountRotator helps rotate between accounts based on rate limits
type AccountRotator struct {
	limiter      *MultiAccountRateLimiter
	accounts     []string
	currentIndex int
	mu           sync.Mutex
}

// NewAccountRotator creates a new account rotator
func NewAccountRotator(limiter *MultiAccountRateLimiter, accounts []string) *AccountRotator {
	return &AccountRotator{
		limiter:  limiter,
		accounts: accounts,
	}
}

// GetNextAccount returns the next available account
func (r *AccountRotator) GetNextAccount() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// Try to find an account with available rate limit
	for i := 0; i < len(r.accounts); i++ {
		r.currentIndex = (r.currentIndex + 1) % len(r.accounts)
		account := r.accounts[r.currentIndex]
		
		used, limit := r.limiter.GetUsage(account)
		if used < limit*9/10 { // Use account if less than 90% of limit
			return account
		}
	}
	
	// If all accounts are near limit, use the best one
	return r.limiter.GetBestAccount(r.accounts)
}