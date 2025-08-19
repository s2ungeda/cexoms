package exchange

import (
	"fmt"
	"sync"
	"time"
	
	"github.com/mExOms/pkg/types"
	"github.com/sirupsen/logrus"
)

// BaseExchange provides common functionality for all exchange implementations
type BaseExchange struct {
	config         *Config
	exchangeType   types.ExchangeType
	normalizer     types.SymbolNormalizer
	logger         *logrus.Entry
	connected      bool
	mu             sync.RWMutex
	rateLimiter    *RateLimiter
	symbolInfoCache map[string]*types.SymbolInfo
	cacheMu        sync.RWMutex
}

// NewBaseExchange creates a new base exchange instance
func NewBaseExchange(exchangeType types.ExchangeType, config *Config) *BaseExchange {
	return &BaseExchange{
		config:          config,
		exchangeType:    exchangeType,
		normalizer:      types.GetNormalizer(exchangeType),
		logger:          logrus.WithField("exchange", exchangeType),
		symbolInfoCache: make(map[string]*types.SymbolInfo),
		rateLimiter:     NewRateLimiter(config.RateLimits),
	}
}

// IsConnected returns connection status
func (b *BaseExchange) IsConnected() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.connected
}

// SetConnected sets connection status
func (b *BaseExchange) SetConnected(connected bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.connected = connected
}

// GetExchangeInfo returns exchange information
func (b *BaseExchange) GetExchangeInfo() types.ExchangeInfo {
	return types.ExchangeInfo{
		Name:       string(b.exchangeType),
		Type:       b.exchangeType,
		TestNet:    b.config.TestNet,
		RateLimits: b.config.RateLimits,
	}
}

// NormalizeSymbol converts exchange symbol to standard format
func (b *BaseExchange) NormalizeSymbol(exchangeSymbol string) string {
	return b.normalizer.Normalize(exchangeSymbol)
}

// DenormalizeSymbol converts standard symbol to exchange format
func (b *BaseExchange) DenormalizeSymbol(standardSymbol string) string {
	return b.normalizer.Denormalize(standardSymbol)
}

// GetSymbolInfo retrieves symbol info from cache or fetches it
func (b *BaseExchange) GetSymbolInfo(symbol string) (*types.SymbolInfo, error) {
	b.cacheMu.RLock()
	info, exists := b.symbolInfoCache[symbol]
	b.cacheMu.RUnlock()
	
	if exists {
		return info, nil
	}
	
	return nil, fmt.Errorf("symbol info not found for %s", symbol)
}

// UpdateSymbolInfo updates symbol info in cache
func (b *BaseExchange) UpdateSymbolInfo(symbol string, info *types.SymbolInfo) {
	b.cacheMu.Lock()
	defer b.cacheMu.Unlock()
	b.symbolInfoCache[symbol] = info
}

// CheckRateLimit checks if request can be made
func (b *BaseExchange) CheckRateLimit(weight int) error {
	return b.rateLimiter.CheckLimit(weight)
}

// RateLimiter implements rate limiting for exchanges
type RateLimiter struct {
	limits         types.RateLimits
	weightCounter  int
	orderCounter   int
	dailyOrderCount int
	lastReset      time.Time
	lastMinuteReset time.Time
	mu             sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(limits types.RateLimits) *RateLimiter {
	now := time.Now()
	return &RateLimiter{
		limits:          limits,
		lastReset:       now,
		lastMinuteReset: now,
	}
}

// CheckLimit checks if request can be made within rate limits
func (r *RateLimiter) CheckLimit(weight int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	currentTime := time.Now()
	
	// Reset counters if needed
	if currentTime.Sub(r.lastMinuteReset) >= time.Minute {
		r.weightCounter = 0
		r.orderCounter = 0
		r.lastMinuteReset = currentTime
	}
	
	if currentTime.Day() != r.lastReset.Day() {
		r.dailyOrderCount = 0
		r.lastReset = currentTime
	}
	
	// Check weight limit
	if r.weightCounter+weight > r.limits.WeightPerMinute {
		return fmt.Errorf("rate limit exceeded: weight limit %d/%d", 
			r.weightCounter+weight, r.limits.WeightPerMinute)
	}
	
	r.weightCounter += weight
	return nil
}

// IncrementOrderCount increments order counters
func (r *RateLimiter) IncrementOrderCount() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// Check per-second limit
	if r.orderCounter >= r.limits.OrdersPerSecond {
		return fmt.Errorf("rate limit exceeded: orders per second %d/%d",
			r.orderCounter, r.limits.OrdersPerSecond)
	}
	
	// Check daily limit
	if r.dailyOrderCount >= r.limits.OrdersPerDay {
		return fmt.Errorf("rate limit exceeded: daily order limit %d/%d",
			r.dailyOrderCount, r.limits.OrdersPerDay)
	}
	
	r.orderCounter++
	r.dailyOrderCount++
	
	// Reset per-second counter after 1 second
	go func() {
		time.Sleep(time.Second)
		r.mu.Lock()
		r.orderCounter--
		r.mu.Unlock()
	}()
	
	return nil
}

// OrderConverter handles order conversion between standard and exchange formats
type OrderConverter struct {
	exchangeType types.ExchangeType
	normalizer   types.SymbolNormalizer
}

// NewOrderConverter creates a new order converter
func NewOrderConverter(exchangeType types.ExchangeType) *OrderConverter {
	return &OrderConverter{
		exchangeType: exchangeType,
		normalizer:   types.GetNormalizer(exchangeType),
	}
}

// ToStandardOrder converts exchange-specific order to standard format
func (c *OrderConverter) ToStandardOrder(exchangeOrder interface{}) (*types.Order, error) {
	// Implementation depends on exchange-specific order format
	// This is a placeholder that each exchange will override
	return nil, fmt.Errorf("ToStandardOrder not implemented for %s", c.exchangeType)
}

// FromStandardOrder converts standard order to exchange-specific format
func (c *OrderConverter) FromStandardOrder(order *types.Order) (interface{}, error) {
	// Implementation depends on exchange-specific order format
	// This is a placeholder that each exchange will override
	return nil, fmt.Errorf("FromStandardOrder not implemented for %s", c.exchangeType)
}