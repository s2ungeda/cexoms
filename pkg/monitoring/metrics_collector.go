package monitoring

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// MetricsCollector collects and aggregates performance metrics
type MetricsCollector struct {
	mu             sync.RWMutex
	accountMetrics map[string]*AccountMetrics
	globalMetrics  *GlobalMetrics
	config         *MetricsConfig
	logger         *zap.Logger
	
	// Channels for metric updates
	orderChan     chan *OrderMetric
	positionChan  chan *PositionMetric
	tradeChan     chan *TradeMetric
	latencyChan   chan *LatencyMetric
	
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// MetricsConfig holds configuration for metrics collection
type MetricsConfig struct {
	// Collection intervals
	AggregationInterval time.Duration
	RetentionPeriod     time.Duration
	
	// Buffer sizes
	OrderBufferSize    int
	PositionBufferSize int
	TradeBufferSize    int
	LatencyBufferSize  int
	
	// Performance thresholds
	SlowOrderThreshold   time.Duration // Orders taking longer than this are flagged
	HighLatencyThreshold time.Duration // Network latency threshold
}

// DefaultMetricsConfig returns default configuration
func DefaultMetricsConfig() *MetricsConfig {
	return &MetricsConfig{
		AggregationInterval:  10 * time.Second,
		RetentionPeriod:      24 * time.Hour,
		OrderBufferSize:      10000,
		PositionBufferSize:   1000,
		TradeBufferSize:      10000,
		LatencyBufferSize:    10000,
		SlowOrderThreshold:   100 * time.Millisecond,
		HighLatencyThreshold: 50 * time.Millisecond,
	}
}

// AccountMetrics tracks metrics for a specific account
type AccountMetrics struct {
	AccountID string
	
	// Order metrics
	OrdersPlaced     atomic.Int64
	OrdersCancelled  atomic.Int64
	OrdersFilled     atomic.Int64
	OrdersRejected   atomic.Int64
	OrderLatencySum  atomic.Int64 // Sum of all latencies in microseconds
	OrderLatencyMax  atomic.Int64 // Max latency in microseconds
	SlowOrderCount   atomic.Int64
	
	// Trade metrics  
	TradeCount       atomic.Int64
	TradeVolume      atomic.Uint64 // In cents to avoid float precision issues
	TradeFees        atomic.Uint64 // In cents
	
	// Position metrics
	ActivePositions  atomic.Int32
	TotalExposure    atomic.Uint64 // In cents
	MaxExposure      atomic.Uint64 // Peak exposure in cents
	
	// Risk metrics
	RiskChecksPassed atomic.Int64
	RiskChecksFailed atomic.Int64
	KillSwitchCount  atomic.Int32
	
	// Network metrics
	WebSocketReconnects atomic.Int32
	APIErrors           atomic.Int32
	RateLimitHits       atomic.Int32
	
	// Time series data (protected by parent mutex)
	OrderLatencyHistory []LatencyDataPoint
	TradeVolumeHistory  []VolumeDataPoint
	ExposureHistory     []ExposureDataPoint
	
	LastUpdate time.Time
}

// GlobalMetrics tracks system-wide metrics
type GlobalMetrics struct {
	// System performance
	CPUUsage         atomic.Uint32 // Percentage * 100
	MemoryUsage      atomic.Uint64 // Bytes
	GoroutineCount   atomic.Int32
	
	// Message processing
	MessagesProcessed atomic.Int64
	MessageQueueDepth atomic.Int32
	
	// Cross-account metrics
	TotalActiveAccounts atomic.Int32
	TotalOrdersPerSec   atomic.Uint32
	TotalTradeVolume    atomic.Uint64 // In cents
	
	// Exchange connectivity
	ExchangeLatency map[string]*ExchangeLatencyStats
	mu              sync.RWMutex
}

// ExchangeLatencyStats tracks latency per exchange
type ExchangeLatencyStats struct {
	Exchange      string
	PingLatency   atomic.Int64 // microseconds
	OrderLatency  atomic.Int64 // microseconds
	DataLatency   atomic.Int64 // microseconds
	LastUpdate    time.Time
}

// Metric types
type OrderMetric struct {
	AccountID  string
	OrderID    string
	Symbol     string
	Side       string
	Type       string
	Status     string
	Latency    time.Duration
	Timestamp  time.Time
}

type PositionMetric struct {
	AccountID string
	Symbol    string
	Side      string
	Quantity  float64
	Exposure  float64
	PnL       float64
	Timestamp time.Time
}

type TradeMetric struct {
	AccountID string
	TradeID   string
	Symbol    string
	Side      string
	Quantity  float64
	Price     float64
	Fee       float64
	Timestamp time.Time
}

type LatencyMetric struct {
	AccountID   string
	Exchange    string
	Type        string // "ping", "order", "data"
	Latency     time.Duration
	Timestamp   time.Time
}

// Time series data points
type LatencyDataPoint struct {
	Timestamp time.Time
	AvgLatency time.Duration
	MaxLatency time.Duration
	Count      int
}

type VolumeDataPoint struct {
	Timestamp time.Time
	Volume    float64
	Count     int
}

type ExposureDataPoint struct {
	Timestamp     time.Time
	TotalExposure float64
	Positions     int
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(config *MetricsConfig, logger *zap.Logger) *MetricsCollector {
	if config == nil {
		config = DefaultMetricsConfig()
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	mc := &MetricsCollector{
		accountMetrics: make(map[string]*AccountMetrics),
		globalMetrics: &GlobalMetrics{
			ExchangeLatency: make(map[string]*ExchangeLatencyStats),
		},
		config:       config,
		logger:       logger,
		orderChan:    make(chan *OrderMetric, config.OrderBufferSize),
		positionChan: make(chan *PositionMetric, config.PositionBufferSize),
		tradeChan:    make(chan *TradeMetric, config.TradeBufferSize),
		latencyChan:  make(chan *LatencyMetric, config.LatencyBufferSize),
		ctx:          ctx,
		cancel:       cancel,
	}
	
	// Start processing goroutines
	mc.wg.Add(4)
	go mc.processOrderMetrics()
	go mc.processPositionMetrics()
	go mc.processTradeMetrics()
	go mc.processLatencyMetrics()
	
	// Start aggregation routine
	mc.wg.Add(1)
	go mc.aggregateMetrics()
	
	// Start cleanup routine
	mc.wg.Add(1)
	go mc.cleanupOldMetrics()
	
	return mc
}

// RecordOrder records an order metric
func (mc *MetricsCollector) RecordOrder(metric *OrderMetric) {
	select {
	case mc.orderChan <- metric:
	default:
		mc.logger.Warn("Order metrics buffer full, dropping metric")
	}
}

// RecordPosition records a position metric
func (mc *MetricsCollector) RecordPosition(metric *PositionMetric) {
	select {
	case mc.positionChan <- metric:
	default:
		mc.logger.Warn("Position metrics buffer full, dropping metric")
	}
}

// RecordTrade records a trade metric
func (mc *MetricsCollector) RecordTrade(metric *TradeMetric) {
	select {
	case mc.tradeChan <- metric:
	default:
		mc.logger.Warn("Trade metrics buffer full, dropping metric")
	}
}

// RecordLatency records a latency metric
func (mc *MetricsCollector) RecordLatency(metric *LatencyMetric) {
	select {
	case mc.latencyChan <- metric:
	default:
		mc.logger.Warn("Latency metrics buffer full, dropping metric")
	}
}

// GetAccountMetrics returns metrics for a specific account
func (mc *MetricsCollector) GetAccountMetrics(accountID string) *AccountMetrics {
	mc.mu.RLock()
	metrics, exists := mc.accountMetrics[accountID]
	mc.mu.RUnlock()
	
	if !exists {
		mc.mu.Lock()
		metrics = mc.getOrCreateAccountMetrics(accountID)
		mc.mu.Unlock()
	}
	
	return metrics
}

// GetGlobalMetrics returns global system metrics
func (mc *MetricsCollector) GetGlobalMetrics() *GlobalMetrics {
	return mc.globalMetrics
}

// GetAccountSummary returns a summary of account metrics
func (mc *MetricsCollector) GetAccountSummary(accountID string) map[string]interface{} {
	metrics := mc.GetAccountMetrics(accountID)
	
	ordersTotal := metrics.OrdersPlaced.Load()
	latencySum := metrics.OrderLatencySum.Load()
	avgLatency := float64(0)
	if ordersTotal > 0 {
		avgLatency = float64(latencySum) / float64(ordersTotal) / 1000.0 // Convert to milliseconds
	}
	
	return map[string]interface{}{
		"account_id":        accountID,
		"orders_placed":     metrics.OrdersPlaced.Load(),
		"orders_filled":     metrics.OrdersFilled.Load(),
		"orders_cancelled":  metrics.OrdersCancelled.Load(),
		"orders_rejected":   metrics.OrdersRejected.Load(),
		"avg_order_latency": avgLatency,
		"max_order_latency": float64(metrics.OrderLatencyMax.Load()) / 1000.0,
		"slow_orders":       metrics.SlowOrderCount.Load(),
		"trade_count":       metrics.TradeCount.Load(),
		"trade_volume":      float64(metrics.TradeVolume.Load()) / 100.0,
		"trade_fees":        float64(metrics.TradeFees.Load()) / 100.0,
		"active_positions":  metrics.ActivePositions.Load(),
		"total_exposure":    float64(metrics.TotalExposure.Load()) / 100.0,
		"max_exposure":      float64(metrics.MaxExposure.Load()) / 100.0,
		"risk_checks_passed": metrics.RiskChecksPassed.Load(),
		"risk_checks_failed": metrics.RiskChecksFailed.Load(),
		"ws_reconnects":     metrics.WebSocketReconnects.Load(),
		"api_errors":        metrics.APIErrors.Load(),
		"rate_limit_hits":   metrics.RateLimitHits.Load(),
		"last_update":       metrics.LastUpdate,
	}
}

// processOrderMetrics processes order metrics from the channel
func (mc *MetricsCollector) processOrderMetrics() {
	defer mc.wg.Done()
	
	for {
		select {
		case <-mc.ctx.Done():
			return
		case metric := <-mc.orderChan:
			if metric == nil {
				continue
			}
			
			am := mc.GetAccountMetrics(metric.AccountID)
			
			// Update order counts based on status
			switch metric.Status {
			case "NEW", "PLACED":
				am.OrdersPlaced.Add(1)
			case "FILLED":
				am.OrdersFilled.Add(1)
			case "CANCELLED":
				am.OrdersCancelled.Add(1)
			case "REJECTED":
				am.OrdersRejected.Add(1)
			}
			
			// Update latency metrics
			latencyMicros := metric.Latency.Microseconds()
			am.OrderLatencySum.Add(latencyMicros)
			
			// Update max latency atomically
			for {
				oldMax := am.OrderLatencyMax.Load()
				if latencyMicros <= oldMax || am.OrderLatencyMax.CompareAndSwap(oldMax, latencyMicros) {
					break
				}
			}
			
			// Check for slow orders
			if metric.Latency > mc.config.SlowOrderThreshold {
				am.SlowOrderCount.Add(1)
				mc.logger.Warn("Slow order detected",
					zap.String("account_id", metric.AccountID),
					zap.String("order_id", metric.OrderID),
					zap.Duration("latency", metric.Latency))
			}
			
			// Update global metrics
			mc.globalMetrics.TotalOrdersPerSec.Add(1)
		}
	}
}

// processPositionMetrics processes position metrics
func (mc *MetricsCollector) processPositionMetrics() {
	defer mc.wg.Done()
	
	for {
		select {
		case <-mc.ctx.Done():
			return
		case metric := <-mc.positionChan:
			if metric == nil {
				continue
			}
			
			am := mc.GetAccountMetrics(metric.AccountID)
			
			// Update exposure
			exposureCents := uint64(metric.Exposure * 100)
			am.TotalExposure.Store(exposureCents)
			
			// Update max exposure atomically
			for {
				oldMax := am.MaxExposure.Load()
				if exposureCents <= oldMax || am.MaxExposure.CompareAndSwap(oldMax, exposureCents) {
					break
				}
			}
		}
	}
}

// processTradeMetrics processes trade metrics
func (mc *MetricsCollector) processTradeMetrics() {
	defer mc.wg.Done()
	
	for {
		select {
		case <-mc.ctx.Done():
			return
		case metric := <-mc.tradeChan:
			if metric == nil {
				continue
			}
			
			am := mc.GetAccountMetrics(metric.AccountID)
			
			// Update trade metrics
			am.TradeCount.Add(1)
			am.TradeVolume.Add(uint64(metric.Quantity * metric.Price * 100))
			am.TradeFees.Add(uint64(metric.Fee * 100))
			
			// Update global volume
			mc.globalMetrics.TotalTradeVolume.Add(uint64(metric.Quantity * metric.Price * 100))
		}
	}
}

// processLatencyMetrics processes latency metrics
func (mc *MetricsCollector) processLatencyMetrics() {
	defer mc.wg.Done()
	
	for {
		select {
		case <-mc.ctx.Done():
			return
		case metric := <-mc.latencyChan:
			if metric == nil {
				continue
			}
			
			// Update exchange latency
			mc.globalMetrics.mu.Lock()
			stats, exists := mc.globalMetrics.ExchangeLatency[metric.Exchange]
			if !exists {
				stats = &ExchangeLatencyStats{
					Exchange: metric.Exchange,
				}
				mc.globalMetrics.ExchangeLatency[metric.Exchange] = stats
			}
			mc.globalMetrics.mu.Unlock()
			
			latencyMicros := metric.Latency.Microseconds()
			
			switch metric.Type {
			case "ping":
				stats.PingLatency.Store(latencyMicros)
			case "order":
				stats.OrderLatency.Store(latencyMicros)
			case "data":
				stats.DataLatency.Store(latencyMicros)
			}
			
			stats.LastUpdate = metric.Timestamp
		}
	}
}

// aggregateMetrics periodically aggregates metrics
func (mc *MetricsCollector) aggregateMetrics() {
	defer mc.wg.Done()
	
	ticker := time.NewTicker(mc.config.AggregationInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-mc.ctx.Done():
			return
		case <-ticker.C:
			mc.performAggregation()
		}
	}
}

// performAggregation performs metric aggregation
func (mc *MetricsCollector) performAggregation() {
	now := time.Now()
	
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	// Count active accounts
	activeAccounts := int32(0)
	for _, am := range mc.accountMetrics {
		if now.Sub(am.LastUpdate) < mc.config.AggregationInterval*2 {
			activeAccounts++
		}
		
		// Add time series data points
		if am.OrdersPlaced.Load() > 0 {
			avgLatency := time.Duration(am.OrderLatencySum.Load()/am.OrdersPlaced.Load()) * time.Microsecond
			maxLatency := time.Duration(am.OrderLatencyMax.Load()) * time.Microsecond
			
			am.OrderLatencyHistory = append(am.OrderLatencyHistory, LatencyDataPoint{
				Timestamp:  now,
				AvgLatency: avgLatency,
				MaxLatency: maxLatency,
				Count:      int(am.OrdersPlaced.Load()),
			})
		}
		
		if am.TradeCount.Load() > 0 {
			am.TradeVolumeHistory = append(am.TradeVolumeHistory, VolumeDataPoint{
				Timestamp: now,
				Volume:    float64(am.TradeVolume.Load()) / 100.0,
				Count:     int(am.TradeCount.Load()),
			})
		}
		
		if am.ActivePositions.Load() > 0 {
			am.ExposureHistory = append(am.ExposureHistory, ExposureDataPoint{
				Timestamp:     now,
				TotalExposure: float64(am.TotalExposure.Load()) / 100.0,
				Positions:     int(am.ActivePositions.Load()),
			})
		}
	}
	
	mc.globalMetrics.TotalActiveAccounts.Store(activeAccounts)
}

// cleanupOldMetrics removes old time series data
func (mc *MetricsCollector) cleanupOldMetrics() {
	defer mc.wg.Done()
	
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	
	for {
		select {
		case <-mc.ctx.Done():
			return
		case <-ticker.C:
			mc.performCleanup()
		}
	}
}

// performCleanup removes old metrics data
func (mc *MetricsCollector) performCleanup() {
	cutoff := time.Now().Add(-mc.config.RetentionPeriod)
	
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	for _, am := range mc.accountMetrics {
		// Cleanup latency history
		am.OrderLatencyHistory = filterLatencyHistory(am.OrderLatencyHistory, cutoff)
		
		// Cleanup volume history
		am.TradeVolumeHistory = filterVolumeHistory(am.TradeVolumeHistory, cutoff)
		
		// Cleanup exposure history
		am.ExposureHistory = filterExposureHistory(am.ExposureHistory, cutoff)
	}
}

// Helper functions
func (mc *MetricsCollector) getOrCreateAccountMetrics(accountID string) *AccountMetrics {
	if am, exists := mc.accountMetrics[accountID]; exists {
		return am
	}
	
	am := &AccountMetrics{
		AccountID:  accountID,
		LastUpdate: time.Now(),
	}
	mc.accountMetrics[accountID] = am
	return am
}

func filterLatencyHistory(history []LatencyDataPoint, cutoff time.Time) []LatencyDataPoint {
	var filtered []LatencyDataPoint
	for _, dp := range history {
		if dp.Timestamp.After(cutoff) {
			filtered = append(filtered, dp)
		}
	}
	return filtered
}

func filterVolumeHistory(history []VolumeDataPoint, cutoff time.Time) []VolumeDataPoint {
	var filtered []VolumeDataPoint
	for _, dp := range history {
		if dp.Timestamp.After(cutoff) {
			filtered = append(filtered, dp)
		}
	}
	return filtered
}

func filterExposureHistory(history []ExposureDataPoint, cutoff time.Time) []ExposureDataPoint {
	var filtered []ExposureDataPoint
	for _, dp := range history {
		if dp.Timestamp.After(cutoff) {
			filtered = append(filtered, dp)
		}
	}
	return filtered
}

// Stop gracefully stops the metrics collector
func (mc *MetricsCollector) Stop() {
	mc.cancel()
	mc.wg.Wait()
	
	// Close channels
	close(mc.orderChan)
	close(mc.positionChan)
	close(mc.tradeChan)
	close(mc.latencyChan)
	
	mc.logger.Info("Metrics collector stopped")
}

// Export methods for integration with monitoring systems

// ExportPrometheus exports metrics in Prometheus format
func (mc *MetricsCollector) ExportPrometheus() string {
	// Implementation would export metrics in Prometheus format
	// For now, return placeholder
	return "# HELP mexoms_orders_total Total orders placed\n# TYPE mexoms_orders_total counter\n"
}

// ExportJSON exports metrics as JSON
func (mc *MetricsCollector) ExportJSON() map[string]interface{} {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	
	accounts := make(map[string]interface{})
	for accountID, metrics := range mc.accountMetrics {
		accounts[accountID] = mc.GetAccountSummary(accountID)
	}
	
	return map[string]interface{}{
		"timestamp": time.Now(),
		"accounts":  accounts,
		"global": map[string]interface{}{
			"active_accounts":   mc.globalMetrics.TotalActiveAccounts.Load(),
			"total_orders_sec":  mc.globalMetrics.TotalOrdersPerSec.Load(),
			"total_volume":      float64(mc.globalMetrics.TotalTradeVolume.Load()) / 100.0,
			"cpu_usage":         float64(mc.globalMetrics.CPUUsage.Load()) / 100.0,
			"memory_bytes":      mc.globalMetrics.MemoryUsage.Load(),
			"goroutines":        mc.globalMetrics.GoroutineCount.Load(),
			"messages_processed": mc.globalMetrics.MessagesProcessed.Load(),
			"queue_depth":       mc.globalMetrics.MessageQueueDepth.Load(),
		},
	}
}