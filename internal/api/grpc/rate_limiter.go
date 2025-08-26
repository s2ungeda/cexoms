package grpc

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter manages rate limiting for API requests
type RateLimiter struct {
	// Global rate limiter
	globalLimiter *rate.Limiter
	globalRPS     int
	
	// Per-account rate limiters
	accountLimiters sync.Map // accountID -> *accountLimiter
	defaultRPS      int
	
	// Request weights
	orderWeight int
	queryWeight int
	
	// Metrics
	totalRequests   int64
	blockedRequests int64
	
	// Cleanup
	cleanupInterval time.Duration
	ctx             context.Context
	cancel          context.CancelFunc
}

// accountLimiter holds rate limiter for an account
type accountLimiter struct {
	limiter     *rate.Limiter
	lastAccess  time.Time
	requests    int64
	blocked     int64
	customRPS   int // Custom RPS for this account
	mu          sync.RWMutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(globalRPS, defaultAccountRPS int) *RateLimiter {
	ctx, cancel := context.WithCancel(context.Background())
	
	rl := &RateLimiter{
		globalLimiter:   rate.NewLimiter(rate.Limit(globalRPS), globalRPS),
		globalRPS:       globalRPS,
		defaultRPS:      defaultAccountRPS,
		orderWeight:     10,
		queryWeight:     1,
		cleanupInterval: 5 * time.Minute,
		ctx:             ctx,
		cancel:          cancel,
	}
	
	// Start cleanup routine
	go rl.cleanupRoutine()
	
	return rl
}

// Allow checks if a request is allowed
func (rl *RateLimiter) Allow(accountID string, weight int) bool {
	// Increment total requests
	atomic.AddInt64(&rl.totalRequests, 1)
	
	// Check global rate limit first
	if !rl.globalLimiter.AllowN(time.Now(), weight) {
		atomic.AddInt64(&rl.blockedRequests, 1)
		return false
	}
	
	// Get or create account limiter
	accLimiter := rl.getOrCreateAccountLimiter(accountID)
	
	// Check account rate limit
	accLimiter.mu.RLock()
	allowed := accLimiter.limiter.AllowN(time.Now(), weight)
	accLimiter.mu.RUnlock()
	
	// Update metrics
	if allowed {
		atomic.AddInt64(&accLimiter.requests, 1)
	} else {
		atomic.AddInt64(&accLimiter.blocked, 1)
		atomic.AddInt64(&rl.blockedRequests, 1)
	}
	
	// Update last access
	accLimiter.mu.Lock()
	accLimiter.lastAccess = time.Now()
	accLimiter.mu.Unlock()
	
	return allowed
}

// AllowWithBurst checks if a request is allowed with burst capability
func (rl *RateLimiter) AllowWithBurst(accountID string, weight, burst int) bool {
	// Get or create account limiter
	accLimiter := rl.getOrCreateAccountLimiter(accountID)
	
	// Temporarily increase burst
	accLimiter.mu.Lock()
	oldBurst := accLimiter.limiter.Burst()
	accLimiter.limiter.SetBurst(burst)
	allowed := accLimiter.limiter.AllowN(time.Now(), weight)
	accLimiter.limiter.SetBurst(oldBurst)
	accLimiter.mu.Unlock()
	
	return allowed
}

// SetAccountRateLimit sets custom rate limit for an account
func (rl *RateLimiter) SetAccountRateLimit(accountID string, rps int) {
	accLimiter := rl.getOrCreateAccountLimiter(accountID)
	
	accLimiter.mu.Lock()
	defer accLimiter.mu.Unlock()
	
	accLimiter.customRPS = rps
	accLimiter.limiter = rate.NewLimiter(rate.Limit(rps), rps)
}

// GetAccountMetrics returns metrics for an account
func (rl *RateLimiter) GetAccountMetrics(accountID string) (requests, blocked int64) {
	value, ok := rl.accountLimiters.Load(accountID)
	if !ok {
		return 0, 0
	}
	
	accLimiter := value.(*accountLimiter)
	return atomic.LoadInt64(&accLimiter.requests), atomic.LoadInt64(&accLimiter.blocked)
}

// GetGlobalMetrics returns global metrics
func (rl *RateLimiter) GetGlobalMetrics() (total, blocked int64) {
	return atomic.LoadInt64(&rl.totalRequests), atomic.LoadInt64(&rl.blockedRequests)
}

// GetWeight returns the weight for a specific method
func (rl *RateLimiter) GetWeight(method string) int {
	// Order operations have higher weight
	orderMethods := []string{
		"CreateOrder",
		"CancelOrder",
		"ModifyOrder",
		"BatchCreateOrders",
	}
	
	for _, m := range orderMethods {
		if contains(method, m) {
			return rl.orderWeight
		}
	}
	
	// Query operations have lower weight
	queryMethods := []string{
		"Get",
		"List",
		"Query",
	}
	
	for _, m := range queryMethods {
		if contains(method, m) {
			return rl.queryWeight
		}
	}
	
	// Default weight
	return 1
}

// ReserveN reserves n tokens for future use
func (rl *RateLimiter) ReserveN(accountID string, n int) *rate.Reservation {
	accLimiter := rl.getOrCreateAccountLimiter(accountID)
	
	accLimiter.mu.RLock()
	r := accLimiter.limiter.ReserveN(time.Now(), n)
	accLimiter.mu.RUnlock()
	
	return r
}

// Wait blocks until n tokens are available
func (rl *RateLimiter) Wait(ctx context.Context, accountID string, n int) error {
	accLimiter := rl.getOrCreateAccountLimiter(accountID)
	
	accLimiter.mu.RLock()
	err := accLimiter.limiter.WaitN(ctx, n)
	accLimiter.mu.RUnlock()
	
	return err
}

// Stop stops the rate limiter
func (rl *RateLimiter) Stop() {
	rl.cancel()
}

// getOrCreateAccountLimiter gets or creates a rate limiter for an account
func (rl *RateLimiter) getOrCreateAccountLimiter(accountID string) *accountLimiter {
	value, ok := rl.accountLimiters.Load(accountID)
	if ok {
		return value.(*accountLimiter)
	}
	
	// Create new account limiter
	accLimiter := &accountLimiter{
		limiter:    rate.NewLimiter(rate.Limit(rl.defaultRPS), rl.defaultRPS),
		lastAccess: time.Now(),
		customRPS:  rl.defaultRPS,
	}
	
	// Store it (handle race condition)
	actual, loaded := rl.accountLimiters.LoadOrStore(accountID, accLimiter)
	if loaded {
		return actual.(*accountLimiter)
	}
	
	return accLimiter
}

// cleanupRoutine periodically removes inactive account limiters
func (rl *RateLimiter) cleanupRoutine() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.ctx.Done():
			return
		}
	}
}

// cleanup removes inactive account limiters
func (rl *RateLimiter) cleanup() {
	cutoff := time.Now().Add(-24 * time.Hour)
	
	rl.accountLimiters.Range(func(key, value interface{}) bool {
		accLimiter := value.(*accountLimiter)
		
		accLimiter.mu.RLock()
		lastAccess := accLimiter.lastAccess
		accLimiter.mu.RUnlock()
		
		// Remove if inactive for 24 hours
		if lastAccess.Before(cutoff) {
			rl.accountLimiters.Delete(key)
		}
		
		return true
	})
}

// RateLimitConfig returns rate limit configuration for an account
func (rl *RateLimiter) GetRateLimitConfig(accountID string) *RateLimitInfo {
	accLimiter := rl.getOrCreateAccountLimiter(accountID)
	
	accLimiter.mu.RLock()
	defer accLimiter.mu.RUnlock()
	
	tokens := accLimiter.limiter.Tokens()
	burst := accLimiter.limiter.Burst()
	
	return &RateLimitInfo{
		AccountID:    accountID,
		RPS:          accLimiter.customRPS,
		Burst:        burst,
		Available:    int(tokens),
		Requests:     atomic.LoadInt64(&accLimiter.requests),
		Blocked:      atomic.LoadInt64(&accLimiter.blocked),
		LastAccess:   accLimiter.lastAccess,
	}
}

// RateLimitInfo contains rate limit information
type RateLimitInfo struct {
	AccountID  string    `json:"account_id"`
	RPS        int       `json:"rps"`
	Burst      int       `json:"burst"`
	Available  int       `json:"available"`
	Requests   int64     `json:"requests"`
	Blocked    int64     `json:"blocked"`
	LastAccess time.Time `json:"last_access"`
}

// ExportMetrics exports rate limiter metrics
func (rl *RateLimiter) ExportMetrics() map[string]interface{} {
	metrics := map[string]interface{}{
		"global_rps":       rl.globalRPS,
		"total_requests":   atomic.LoadInt64(&rl.totalRequests),
		"blocked_requests": atomic.LoadInt64(&rl.blockedRequests),
		"active_accounts":  0,
		"accounts":         make(map[string]interface{}),
	}
	
	accountCount := 0
	accounts := make(map[string]interface{})
	
	rl.accountLimiters.Range(func(key, value interface{}) bool {
		accountID := key.(string)
		accLimiter := value.(*accountLimiter)
		
		accounts[accountID] = map[string]interface{}{
			"requests": atomic.LoadInt64(&accLimiter.requests),
			"blocked":  atomic.LoadInt64(&accLimiter.blocked),
			"rps":      accLimiter.customRPS,
		}
		
		accountCount++
		return true
	})
	
	metrics["active_accounts"] = accountCount
	metrics["accounts"] = accounts
	
	return metrics
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}