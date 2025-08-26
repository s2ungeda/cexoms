package metrics

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// OMSMetrics provides OMS-specific metrics collection
type OMSMetrics struct {
	registry *MetricsRegistry
	mu       sync.RWMutex
	
	// Internal state for complex metrics
	orderTimes    map[string]time.Time
	routeDecisions map[string]RouteDecision
	exchangeHealth map[string]ExchangeHealth
}

// RouteDecision represents a routing decision
type RouteDecision struct {
	Strategy         string
	SelectedExchanges []string
	OrderSize        float64
	Timestamp        time.Time
}

// ExchangeHealth represents exchange health status
type ExchangeHealth struct {
	Exchange     string
	LastHeartbeat time.Time
	Latency      time.Duration
	ErrorRate    float64
	IsHealthy    bool
}

// NewOMSMetrics creates a new OMS metrics instance
func NewOMSMetrics(registry *MetricsRegistry) *OMSMetrics {
	return &OMSMetrics{
		registry:       registry,
		orderTimes:     make(map[string]time.Time),
		routeDecisions: make(map[string]RouteDecision),
		exchangeHealth: make(map[string]ExchangeHealth),
	}
}

// RecordOrderStart records the start of an order
func (om *OMSMetrics) RecordOrderStart(orderID string) {
	om.mu.Lock()
	om.orderTimes[orderID] = time.Now()
	om.mu.Unlock()
}

// RecordOrderComplete records order completion and calculates latency
func (om *OMSMetrics) RecordOrderComplete(orderID, exchange, market, orderType, side, status string, volumeUSD float64) {
	om.mu.Lock()
	startTime, exists := om.orderTimes[orderID]
	if exists {
		duration := time.Since(startTime)
		om.registry.Core.RecordOrderDuration(exchange, market, orderType, duration)
		delete(om.orderTimes, orderID)
	}
	om.mu.Unlock()

	// Record order metrics
	om.registry.Order.RecordOrder(exchange, market, orderType, side, status, volumeUSD)
	
	// Update active orders based on status
	if status == "new" || status == "partial" {
		om.registry.Order.UpdateActiveOrders(exchange, market, orderType, 1)
	} else if status == "filled" || status == "cancelled" || status == "rejected" {
		om.registry.Order.UpdateActiveOrders(exchange, market, orderType, -1)
	}
}

// RecordRouteDecision records a routing decision
func (om *OMSMetrics) RecordRouteDecision(orderID, strategy string, exchanges []string, orderSize float64) {
	om.mu.Lock()
	om.routeDecisions[orderID] = RouteDecision{
		Strategy:          strategy,
		SelectedExchanges: exchanges,
		OrderSize:         orderSize,
		Timestamp:         time.Now(),
	}
	om.mu.Unlock()

	// Record metrics
	for _, exchange := range exchanges {
		om.registry.Core.RecordRouteDecision(strategy, exchange)
	}
	om.registry.Core.RouteSplits.WithLabelValues(strategy).Observe(float64(len(exchanges)))
}

// RecordExchangeHealth records exchange health metrics
func (om *OMSMetrics) RecordExchangeHealth(exchange string, latency time.Duration, errorRate float64, isHealthy bool) {
	om.mu.Lock()
	om.exchangeHealth[exchange] = ExchangeHealth{
		Exchange:      exchange,
		LastHeartbeat: time.Now(),
		Latency:       latency,
		ErrorRate:     errorRate,
		IsHealthy:     isHealthy,
	}
	om.mu.Unlock()

	// Update Prometheus metrics
	healthValue := 0.0
	if isHealthy {
		healthValue = 1.0
	}
	
	// Custom exchange health gauge
	exchangeHealthGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "oms_exchange_health",
			Help: "Exchange health status (1=healthy, 0=unhealthy)",
		},
		[]string{"exchange"},
	)
	exchangeHealthGauge.WithLabelValues(exchange).Set(healthValue)
}

// RecordSlippage records order slippage
func (om *OMSMetrics) RecordSlippage(exchange, symbol string, expectedPrice, executedPrice, quantity float64) {
	slippageBps := ((executedPrice - expectedPrice) / expectedPrice) * 10000
	slippageUSD := (executedPrice - expectedPrice) * quantity
	
	// Slippage histogram
	slippageHist := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "oms_slippage_basis_points",
			Help:    "Order slippage in basis points",
			Buckets: prometheus.LinearBuckets(-50, 10, 11), // -50 to +50 bps
		},
		[]string{"exchange", "symbol"},
	)
	slippageHist.WithLabelValues(exchange, symbol).Observe(slippageBps)
	
	// Slippage cost counter
	slippageCost := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "oms_slippage_cost_usd_total",
			Help: "Total slippage cost in USD",
		},
		[]string{"exchange", "symbol"},
	)
	if slippageUSD > 0 {
		slippageCost.WithLabelValues(exchange, symbol).Add(slippageUSD)
	}
}

// RecordFillRate records order fill rate
func (om *OMSMetrics) RecordFillRate(exchange, market string, orderedQuantity, filledQuantity float64) {
	fillRate := (filledQuantity / orderedQuantity) * 100
	
	fillRateGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "oms_fill_rate_percent",
			Help: "Order fill rate percentage",
		},
		[]string{"exchange", "market"},
	)
	fillRateGauge.WithLabelValues(exchange, market).Set(fillRate)
}

// RecordRejectRate records order rejection rate
func (om *OMSMetrics) RecordRejectRate(exchange, market string, totalOrders, rejectedOrders int) {
	rejectRate := (float64(rejectedOrders) / float64(totalOrders)) * 100
	
	rejectRateGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "oms_reject_rate_percent",
			Help: "Order rejection rate percentage",
		},
		[]string{"exchange", "market"},
	)
	rejectRateGauge.WithLabelValues(exchange, market).Set(rejectRate)
}

// RecordInventory records current inventory levels
func (om *OMSMetrics) RecordInventory(exchange, asset string, quantity, valueUSD float64) {
	// Inventory quantity
	inventoryQty := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "oms_inventory_quantity",
			Help: "Current inventory quantity",
		},
		[]string{"exchange", "asset"},
	)
	inventoryQty.WithLabelValues(exchange, asset).Set(quantity)
	
	// Inventory value
	inventoryValue := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "oms_inventory_value_usd",
			Help: "Current inventory value in USD",
		},
		[]string{"exchange", "asset"},
	)
	inventoryValue.WithLabelValues(exchange, asset).Set(valueUSD)
}

// RecordVWAP records Volume Weighted Average Price
func (om *OMSMetrics) RecordVWAP(exchange, symbol string, vwap, currentPrice float64) {
	vwapGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "oms_vwap",
			Help: "Volume Weighted Average Price",
		},
		[]string{"exchange", "symbol"},
	)
	vwapGauge.WithLabelValues(exchange, symbol).Set(vwap)
	
	// VWAP deviation
	deviation := ((currentPrice - vwap) / vwap) * 100
	vwapDeviation := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "oms_vwap_deviation_percent",
			Help: "Price deviation from VWAP in percentage",
		},
		[]string{"exchange", "symbol"},
	)
	vwapDeviation.WithLabelValues(exchange, symbol).Set(deviation)
}

// RecordOrderBookImbalance records order book imbalance
func (om *OMSMetrics) RecordOrderBookImbalance(exchange, symbol string, bidVolume, askVolume float64) {
	imbalance := (bidVolume - askVolume) / (bidVolume + askVolume)
	
	imbalanceGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "oms_orderbook_imbalance",
			Help: "Order book imbalance (-1 to 1)",
		},
		[]string{"exchange", "symbol"},
	)
	imbalanceGauge.WithLabelValues(exchange, symbol).Set(imbalance)
}

// RecordLatencyArbitrage records latency arbitrage opportunities
func (om *OMSMetrics) RecordLatencyArbitrage(symbol string, fastExchange, slowExchange string, priceDiff float64, latencyDiff time.Duration) {
	// Price difference in basis points
	arbOpportunity := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "oms_latency_arbitrage_bps",
			Help:    "Latency arbitrage opportunity in basis points",
			Buckets: prometheus.ExponentialBuckets(1, 2, 8), // 1 to 128 bps
		},
		[]string{"symbol", "fast_exchange", "slow_exchange"},
	)
	arbOpportunity.WithLabelValues(symbol, fastExchange, slowExchange).Observe(priceDiff)
	
	// Latency difference
	latencyDiffHist := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "oms_exchange_latency_diff_ms",
			Help:    "Latency difference between exchanges in milliseconds",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1ms to 102.4ms
		},
		[]string{"symbol", "fast_exchange", "slow_exchange"},
	)
	latencyDiffHist.WithLabelValues(symbol, fastExchange, slowExchange).Observe(float64(latencyDiff.Milliseconds()))
}

// StartPeriodicCollection starts periodic collection of complex metrics
func (om *OMSMetrics) StartPeriodicCollection(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				om.collectPeriodicMetrics()
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()
}

// collectPeriodicMetrics collects metrics that need periodic calculation
func (om *OMSMetrics) collectPeriodicMetrics() {
	om.mu.RLock()
	defer om.mu.RUnlock()
	
	// Clean up old order times (orders that took too long)
	cutoff := time.Now().Add(-5 * time.Minute)
	for orderID, startTime := range om.orderTimes {
		if startTime.Before(cutoff) {
			// Record as timeout
			om.registry.Order.RecordOrderError("unknown", "unknown", "timeout")
			delete(om.orderTimes, orderID)
		}
	}
	
	// Update exchange health metrics
	for _, health := range om.exchangeHealth {
		if time.Since(health.LastHeartbeat) > 30*time.Second {
			// Mark as unhealthy if no heartbeat for 30 seconds
			health.IsHealthy = false
		}
	}
}

// GetHealthReport generates a health report
func (om *OMSMetrics) GetHealthReport() map[string]interface{} {
	om.mu.RLock()
	defer om.mu.RUnlock()
	
	report := make(map[string]interface{})
	
	// Active orders
	activeOrders := len(om.orderTimes)
	report["active_orders"] = activeOrders
	
	// Exchange health
	exchangeStatus := make(map[string]bool)
	for exchange, health := range om.exchangeHealth {
		exchangeStatus[exchange] = health.IsHealthy
	}
	report["exchange_health"] = exchangeStatus
	
	// Recent routing decisions
	recentRoutes := make([]RouteDecision, 0)
	cutoff := time.Now().Add(-1 * time.Minute)
	for _, decision := range om.routeDecisions {
		if decision.Timestamp.After(cutoff) {
			recentRoutes = append(recentRoutes, decision)
		}
	}
	report["recent_routes"] = len(recentRoutes)
	
	return report
}