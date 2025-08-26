package strategies

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/mExOms/pkg/types"
)

// MarketRegime represents different market conditions
type MarketRegime string

const (
	MarketRegimeTrending    MarketRegime = "trending"
	MarketRegimeRangebound  MarketRegime = "rangebound"
	MarketRegimeVolatile    MarketRegime = "volatile"
	MarketRegimeQuiet       MarketRegime = "quiet"
)

// AdaptationRule defines how parameters should adapt to market conditions
type AdaptationRule struct {
	ParameterName    string
	BaseValue        interface{}
	Adjustments      map[MarketRegime]float64 // Multipliers for each regime
	MinValue         interface{}
	MaxValue         interface{}
	AdaptationSpeed  float64 // 0-1, how quickly to adapt
}

// MarketAnalyzer analyzes market conditions
type MarketAnalyzer struct {
	symbol          string
	priceHistory    []decimal.Decimal
	volumeHistory   []decimal.Decimal
	timestamps      []time.Time
	windowSize      int
	updateInterval  time.Duration
}

// AdaptiveController automatically adapts strategy parameters
type AdaptiveController struct {
	mu sync.RWMutex
	
	// Core components
	strategy         Strategy
	modifier         *RealtimeModifier
	
	// Market analysis
	marketAnalyzers  map[string]*MarketAnalyzer
	currentRegimes   map[string]MarketRegime
	
	// Adaptation rules
	adaptationRules  []AdaptationRule
	
	// Performance tracking
	performanceWindow time.Duration
	performanceMetrics map[time.Time]StrategyMetrics
	
	// Control settings
	enabled          bool
	adaptationInterval time.Duration
	minPerformance   decimal.Decimal
	
	// Logger
	logger          *zap.Logger
	
	// Context
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewAdaptiveController creates a new adaptive controller
func NewAdaptiveController(
	strategy Strategy,
	modifier *RealtimeModifier,
	logger *zap.Logger,
) *AdaptiveController {
	return &AdaptiveController{
		strategy:           strategy,
		modifier:           modifier,
		marketAnalyzers:    make(map[string]*MarketAnalyzer),
		currentRegimes:     make(map[string]MarketRegime),
		adaptationRules:    make([]AdaptationRule, 0),
		performanceMetrics: make(map[time.Time]StrategyMetrics),
		performanceWindow:  24 * time.Hour,
		adaptationInterval: 5 * time.Minute,
		minPerformance:     decimal.NewFromFloat(-0.02), // -2% minimum
		enabled:            true,
		logger:             logger,
	}
}

// Start starts the adaptive controller
func (c *AdaptiveController) Start(ctx context.Context) error {
	c.mu.Lock()
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.mu.Unlock()
	
	// Initialize market analyzers for strategy symbols
	config := c.strategy.GetConfig()
	for _, symbol := range config.Symbols {
		c.marketAnalyzers[symbol] = &MarketAnalyzer{
			symbol:         symbol,
			priceHistory:   make([]decimal.Decimal, 0),
			volumeHistory:  make([]decimal.Decimal, 0),
			timestamps:     make([]time.Time, 0),
			windowSize:     100,
			updateInterval: time.Minute,
		}
		c.currentRegimes[symbol] = MarketRegimeRangebound // Default
	}
	
	// Start adaptation loop
	c.wg.Add(1)
	go c.adaptationLoop()
	
	// Start performance tracking
	c.wg.Add(1)
	go c.trackPerformance()
	
	c.logger.Info("Adaptive controller started")
	return nil
}

// Stop stops the adaptive controller
func (c *AdaptiveController) Stop() error {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Unlock()
	
	c.wg.Wait()
	
	c.logger.Info("Adaptive controller stopped")
	return nil
}

// AddAdaptationRule adds a rule for parameter adaptation
func (c *AdaptiveController) AddAdaptationRule(rule AdaptationRule) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Add validation rule to modifier
	c.modifier.AddValidationRule(rule.ParameterName, ValidationRule{
		Name: "adaptive_bounds",
		Validator: func(oldValue, newValue interface{}) error {
			return c.validateAdaptiveBounds(rule, newValue)
		},
	})
	
	c.adaptationRules = append(c.adaptationRules, rule)
	
	c.logger.Info("Added adaptation rule",
		zap.String("parameter", rule.ParameterName),
		zap.Any("base_value", rule.BaseValue))
}

// adaptationLoop runs the main adaptation logic
func (c *AdaptiveController) adaptationLoop() {
	defer c.wg.Done()
	
	ticker := time.NewTicker(c.adaptationInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if c.isEnabled() {
				c.performAdaptation()
			}
		}
	}
}

// performAdaptation performs parameter adaptation
func (c *AdaptiveController) performAdaptation() {
	// Update market regimes
	c.updateMarketRegimes()
	
	// Check performance
	if !c.isPerformanceAcceptable() {
		c.logger.Warn("Performance below threshold, skipping adaptation")
		return
	}
	
	// Apply adaptation rules
	c.mu.RLock()
	rules := make([]AdaptationRule, len(c.adaptationRules))
	copy(rules, c.adaptationRules)
	c.mu.RUnlock()
	
	for _, rule := range rules {
		c.adaptParameter(rule)
	}
}

// updateMarketRegimes analyzes and updates current market regimes
func (c *AdaptiveController) updateMarketRegimes() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	for symbol, analyzer := range c.marketAnalyzers {
		regime := analyzer.analyzeRegime()
		if regime != c.currentRegimes[symbol] {
			c.logger.Info("Market regime changed",
				zap.String("symbol", symbol),
				zap.String("old_regime", string(c.currentRegimes[symbol])),
				zap.String("new_regime", string(regime)))
			c.currentRegimes[symbol] = regime
		}
	}
}

// adaptParameter adapts a single parameter based on market conditions
func (c *AdaptiveController) adaptParameter(rule AdaptationRule) {
	// Calculate weighted adjustment based on all symbols
	totalWeight := 0.0
	weightedAdjustment := 0.0
	
	c.mu.RLock()
	for symbol, regime := range c.currentRegimes {
		if adjustment, exists := rule.Adjustments[regime]; exists {
			// Weight by symbol importance (simplified - equal weight)
			weight := 1.0
			totalWeight += weight
			weightedAdjustment += adjustment * weight
		}
	}
	c.mu.RUnlock()
	
	if totalWeight == 0 {
		return
	}
	
	// Calculate final adjustment
	avgAdjustment := weightedAdjustment / totalWeight
	
	// Get current value
	currentParams := c.strategy.GetParameters()
	currentValue, exists := currentParams[rule.ParameterName]
	if !exists {
		return
	}
	
	// Calculate new value with adaptation speed
	newValue := c.calculateNewValue(currentValue, rule.BaseValue, avgAdjustment, rule.AdaptationSpeed)
	
	// Apply bounds
	newValue = c.applyBounds(newValue, rule.MinValue, rule.MaxValue)
	
	// Skip if change is too small
	if c.isSignificantChange(currentValue, newValue) {
		// Request update
		requestID, err := c.modifier.UpdateParameter(rule.ParameterName, newValue)
		if err != nil {
			c.logger.Error("Failed to update parameter",
				zap.String("parameter", rule.ParameterName),
				zap.Error(err))
		} else {
			c.logger.Info("Adaptive parameter update requested",
				zap.String("parameter", rule.ParameterName),
				zap.Any("current", currentValue),
				zap.Any("new", newValue),
				zap.Float64("adjustment", avgAdjustment),
				zap.String("request_id", requestID))
		}
	}
}

// calculateNewValue calculates new parameter value
func (c *AdaptiveController) calculateNewValue(
	currentValue, baseValue interface{},
	adjustment, adaptationSpeed float64,
) interface{} {
	switch v := baseValue.(type) {
	case float64:
		current := toFloat64(currentValue)
		target := v * adjustment
		return current + (target-current)*adaptationSpeed
		
	case int:
		current := float64(currentValue.(int))
		target := float64(v) * adjustment
		return int(current + (target-current)*adaptationSpeed)
		
	case decimal.Decimal:
		current := currentValue.(decimal.Decimal)
		target := v.Mul(decimal.NewFromFloat(adjustment))
		diff := target.Sub(current).Mul(decimal.NewFromFloat(adaptationSpeed))
		return current.Add(diff)
		
	default:
		return currentValue
	}
}

// applyBounds applies min/max bounds to a value
func (c *AdaptiveController) applyBounds(value, minValue, maxValue interface{}) interface{} {
	switch v := value.(type) {
	case float64:
		min := toFloat64(minValue)
		max := toFloat64(maxValue)
		return math.Max(min, math.Min(max, v))
		
	case int:
		min := minValue.(int)
		max := maxValue.(int)
		if v < min {
			return min
		}
		if v > max {
			return max
		}
		return v
		
	case decimal.Decimal:
		min := minValue.(decimal.Decimal)
		max := maxValue.(decimal.Decimal)
		if v.LessThan(min) {
			return min
		}
		if v.GreaterThan(max) {
			return max
		}
		return v
		
	default:
		return value
	}
}

// isSignificantChange checks if parameter change is significant
func (c *AdaptiveController) isSignificantChange(oldValue, newValue interface{}) bool {
	switch old := oldValue.(type) {
	case float64:
		new := newValue.(float64)
		return math.Abs(new-old)/math.Abs(old) > 0.01 // 1% change
		
	case int:
		new := newValue.(int)
		return new != old
		
	case decimal.Decimal:
		new := newValue.(decimal.Decimal)
		if old.IsZero() {
			return !new.IsZero()
		}
		change := new.Sub(old).Div(old).Abs()
		return change.GreaterThan(decimal.NewFromFloat(0.01))
		
	default:
		return false
	}
}

// trackPerformance tracks strategy performance
func (c *AdaptiveController) trackPerformance() {
	defer c.wg.Done()
	
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			metrics := c.strategy.GetMetrics()
			c.mu.Lock()
			c.performanceMetrics[time.Now()] = metrics
			// Clean old metrics
			cutoff := time.Now().Add(-c.performanceWindow)
			for ts := range c.performanceMetrics {
				if ts.Before(cutoff) {
					delete(c.performanceMetrics, ts)
				}
			}
			c.mu.Unlock()
		}
	}
}

// isPerformanceAcceptable checks if recent performance is acceptable
func (c *AdaptiveController) isPerformanceAcceptable() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	if len(c.performanceMetrics) < 2 {
		return true // Not enough data
	}
	
	// Get oldest and newest metrics
	var oldest, newest time.Time
	for ts := range c.performanceMetrics {
		if oldest.IsZero() || ts.Before(oldest) {
			oldest = ts
		}
		if newest.IsZero() || ts.After(newest) {
			newest = ts
		}
	}
	
	oldMetrics := c.performanceMetrics[oldest]
	newMetrics := c.performanceMetrics[newest]
	
	// Calculate return
	periodReturn := newMetrics.RealizedPnL.Sub(oldMetrics.RealizedPnL)
	
	// Check if above minimum
	return periodReturn.GreaterThan(c.minPerformance)
}

// validateAdaptiveBounds validates parameter is within adaptive bounds
func (c *AdaptiveController) validateAdaptiveBounds(rule AdaptationRule, value interface{}) error {
	switch v := value.(type) {
	case float64:
		min := toFloat64(rule.MinValue)
		max := toFloat64(rule.MaxValue)
		if v < min || v > max {
			return fmt.Errorf("value %f outside bounds [%f, %f]", v, min, max)
		}
		
	case int:
		min := rule.MinValue.(int)
		max := rule.MaxValue.(int)
		if v < min || v > max {
			return fmt.Errorf("value %d outside bounds [%d, %d]", v, min, max)
		}
		
	case decimal.Decimal:
		min := rule.MinValue.(decimal.Decimal)
		max := rule.MaxValue.(decimal.Decimal)
		if v.LessThan(min) || v.GreaterThan(max) {
			return fmt.Errorf("value %s outside bounds [%s, %s]", 
				v.String(), min.String(), max.String())
		}
	}
	
	return nil
}

// SetEnabled enables or disables adaptation
func (c *AdaptiveController) SetEnabled(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enabled = enabled
}

// isEnabled returns if adaptation is enabled
func (c *AdaptiveController) isEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.enabled
}

// GetCurrentRegimes returns current market regimes
func (c *AdaptiveController) GetCurrentRegimes() map[string]MarketRegime {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	regimes := make(map[string]MarketRegime)
	for k, v := range c.currentRegimes {
		regimes[k] = v
	}
	return regimes
}

// MarketAnalyzer methods

// UpdatePrice updates price data
func (m *MarketAnalyzer) UpdatePrice(price decimal.Decimal, volume decimal.Decimal) {
	m.priceHistory = append(m.priceHistory, price)
	m.volumeHistory = append(m.volumeHistory, volume)
	m.timestamps = append(m.timestamps, time.Now())
	
	// Keep window size
	if len(m.priceHistory) > m.windowSize {
		m.priceHistory = m.priceHistory[1:]
		m.volumeHistory = m.volumeHistory[1:]
		m.timestamps = m.timestamps[1:]
	}
}

// analyzeRegime analyzes current market regime
func (m *MarketAnalyzer) analyzeRegime() MarketRegime {
	if len(m.priceHistory) < m.windowSize/2 {
		return MarketRegimeRangebound // Default
	}
	
	// Calculate metrics
	volatility := m.calculateVolatility()
	trend := m.calculateTrend()
	volumeLevel := m.calculateVolumeLevel()
	
	// Determine regime
	if volatility > 0.03 { // High volatility (>3%)
		return MarketRegimeVolatile
	}
	
	if math.Abs(trend) > 0.02 { // Strong trend (>2%)
		return MarketRegimeTrending
	}
	
	if volumeLevel < 0.5 { // Low volume
		return MarketRegimeQuiet
	}
	
	return MarketRegimeRangebound
}

// calculateVolatility calculates price volatility
func (m *MarketAnalyzer) calculateVolatility() float64 {
	if len(m.priceHistory) < 2 {
		return 0
	}
	
	// Calculate returns
	returns := make([]float64, 0)
	for i := 1; i < len(m.priceHistory); i++ {
		if m.priceHistory[i-1].IsZero() {
			continue
		}
		ret := m.priceHistory[i].Sub(m.priceHistory[i-1]).Div(m.priceHistory[i-1])
		returns = append(returns, ret.InexactFloat64())
	}
	
	if len(returns) == 0 {
		return 0
	}
	
	// Calculate standard deviation
	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))
	
	variance := 0.0
	for _, r := range returns {
		variance += math.Pow(r-mean, 2)
	}
	variance /= float64(len(returns))
	
	return math.Sqrt(variance)
}

// calculateTrend calculates price trend
func (m *MarketAnalyzer) calculateTrend() float64 {
	if len(m.priceHistory) < 2 {
		return 0
	}
	
	// Simple linear regression slope
	n := float64(len(m.priceHistory))
	sumX, sumY, sumXY, sumX2 := 0.0, 0.0, 0.0, 0.0
	
	for i, price := range m.priceHistory {
		x := float64(i)
		y := price.InexactFloat64()
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}
	
	// Calculate slope
	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	
	// Normalize by average price
	avgPrice := sumY / n
	if avgPrice > 0 {
		return slope / avgPrice
	}
	
	return 0
}

// calculateVolumeLevel calculates relative volume level
func (m *MarketAnalyzer) calculateVolumeLevel() float64 {
	if len(m.volumeHistory) < 20 {
		return 1.0
	}
	
	// Calculate average volume
	totalVolume := decimal.Zero
	for _, vol := range m.volumeHistory {
		totalVolume = totalVolume.Add(vol)
	}
	avgVolume := totalVolume.Div(decimal.NewFromInt(int64(len(m.volumeHistory))))
	
	// Calculate recent volume (last 20% of window)
	recentCount := len(m.volumeHistory) / 5
	if recentCount < 1 {
		recentCount = 1
	}
	
	recentVolume := decimal.Zero
	for i := len(m.volumeHistory) - recentCount; i < len(m.volumeHistory); i++ {
		recentVolume = recentVolume.Add(m.volumeHistory[i])
	}
	recentAvg := recentVolume.Div(decimal.NewFromInt(int64(recentCount)))
	
	// Return ratio
	if avgVolume.IsZero() {
		return 1.0
	}
	
	return recentAvg.Div(avgVolume).InexactFloat64()
}