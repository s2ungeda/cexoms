package metrics

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// Collector collects system and application metrics
type Collector struct {
	metrics  *Metrics
	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewCollector creates a new metrics collector
func NewCollector(metrics *Metrics, interval time.Duration) *Collector {
	ctx, cancel := context.WithCancel(context.Background())
	return &Collector{
		metrics:  metrics,
		interval: interval,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins collecting metrics
func (c *Collector) Start() {
	c.wg.Add(1)
	go c.collectSystemMetrics()
}

// Stop stops the metrics collector
func (c *Collector) Stop() {
	c.cancel()
	c.wg.Wait()
}

// collectSystemMetrics collects system-level metrics
func (c *Collector) collectSystemMetrics() {
	defer c.wg.Done()
	
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// Memory usage
			if vmStat, err := mem.VirtualMemory(); err == nil {
				c.metrics.MemoryUsage.Set(float64(vmStat.Used))
			}
			
			// CPU usage
			if cpuPercent, err := cpu.Percent(0, false); err == nil && len(cpuPercent) > 0 {
				c.metrics.CPUUsage.Set(cpuPercent[0])
			}
			
			// Goroutine count
			c.metrics.GoroutineCount.Set(float64(runtime.NumGoroutine()))
			
		case <-c.ctx.Done():
			return
		}
	}
}

// OrderMetrics provides an interface for recording order-related metrics
type OrderMetrics struct {
	metrics *Metrics
	mu      sync.RWMutex
}

// NewOrderMetrics creates a new order metrics recorder
func NewOrderMetrics(metrics *Metrics) *OrderMetrics {
	return &OrderMetrics{
		metrics: metrics,
	}
}

// RecordOrder records a new order
func (om *OrderMetrics) RecordOrder(exchange, market, orderType, side, status string, volumeUSD float64) {
	om.metrics.OrdersTotal.WithLabelValues(exchange, market, orderType, side, status).Inc()
	om.metrics.OrderVolume.WithLabelValues(exchange, market, side).Add(volumeUSD)
}

// RecordOrderError records an order error
func (om *OrderMetrics) RecordOrderError(exchange, market, errorType string) {
	om.metrics.OrderErrors.WithLabelValues(exchange, market, errorType).Inc()
}

// UpdateActiveOrders updates the count of active orders
func (om *OrderMetrics) UpdateActiveOrders(exchange, market, orderType string, count float64) {
	om.metrics.ActiveOrders.WithLabelValues(exchange, market, orderType).Set(count)
}

// WebSocketMetrics provides an interface for recording WebSocket metrics
type WebSocketMetrics struct {
	metrics *Metrics
}

// NewWebSocketMetrics creates a new WebSocket metrics recorder
func NewWebSocketMetrics(metrics *Metrics) *WebSocketMetrics {
	return &WebSocketMetrics{
		metrics: metrics,
	}
}

// ConnectionOpened records a new WebSocket connection
func (wm *WebSocketMetrics) ConnectionOpened(exchange, streamType string) {
	wm.metrics.WSConnections.WithLabelValues(exchange, streamType).Inc()
}

// ConnectionClosed records a closed WebSocket connection
func (wm *WebSocketMetrics) ConnectionClosed(exchange, streamType string) {
	wm.metrics.WSConnections.WithLabelValues(exchange, streamType).Dec()
}

// MessageReceived records a received WebSocket message
func (wm *WebSocketMetrics) MessageReceived(exchange, streamType string) {
	wm.metrics.WSMessages.WithLabelValues(exchange, streamType, "inbound").Inc()
}

// MessageSent records a sent WebSocket message
func (wm *WebSocketMetrics) MessageSent(exchange, streamType string) {
	wm.metrics.WSMessages.WithLabelValues(exchange, streamType, "outbound").Inc()
}

// Reconnect records a WebSocket reconnection
func (wm *WebSocketMetrics) Reconnect(exchange, streamType string) {
	wm.metrics.WSReconnects.WithLabelValues(exchange, streamType).Inc()
}

// TradingMetrics provides an interface for recording trading metrics
type TradingMetrics struct {
	metrics *Metrics
}

// NewTradingMetrics creates a new trading metrics recorder
func NewTradingMetrics(metrics *Metrics) *TradingMetrics {
	return &TradingMetrics{
		metrics: metrics,
	}
}

// UpdatePnL updates profit and loss
func (tm *TradingMetrics) UpdatePnL(exchange, market, period string, pnl float64) {
	tm.metrics.PnL.WithLabelValues(exchange, market, period).Set(pnl)
}

// RecordVolume records trading volume
func (tm *TradingMetrics) RecordVolume(exchange, market string, volumeUSD float64) {
	tm.metrics.TradingVolume.WithLabelValues(exchange, market).Add(volumeUSD)
}

// UpdatePosition updates position metrics
func (tm *TradingMetrics) UpdatePosition(exchange, market, symbol, side string, size float64) {
	tm.metrics.Positions.WithLabelValues(exchange, market, symbol, side).Set(size)
}

// UpdateBalance updates account balance
func (tm *TradingMetrics) UpdateBalance(exchange, asset string, balance float64) {
	tm.metrics.Balance.WithLabelValues(exchange, asset).Set(balance)
}

// RiskMetrics provides an interface for recording risk metrics
type RiskMetrics struct {
	metrics *Metrics
}

// NewRiskMetrics creates a new risk metrics recorder
func NewRiskMetrics(metrics *Metrics) *RiskMetrics {
	return &RiskMetrics{
		metrics: metrics,
	}
}

// UpdateRiskScore updates the risk score
func (rm *RiskMetrics) UpdateRiskScore(exchange, market string, score float64) {
	rm.metrics.RiskScore.WithLabelValues(exchange, market).Set(score)
}

// UpdateMarginUsage updates margin usage
func (rm *RiskMetrics) UpdateMarginUsage(exchange, market string, usage float64) {
	rm.metrics.MarginUsage.WithLabelValues(exchange, market).Set(usage)
}

// UpdateLeverage updates leverage
func (rm *RiskMetrics) UpdateLeverage(exchange, market string, leverage float64) {
	rm.metrics.Leverage.WithLabelValues(exchange, market).Set(leverage)
}

// UpdateDrawdown updates drawdown percentage
func (rm *RiskMetrics) UpdateDrawdown(exchange, period string, drawdown float64) {
	rm.metrics.DrawdownPercent.WithLabelValues(exchange, period).Set(drawdown)
}

// MarketDataMetrics provides an interface for recording market data metrics
type MarketDataMetrics struct {
	metrics *Metrics
}

// NewMarketDataMetrics creates a new market data metrics recorder
func NewMarketDataMetrics(metrics *Metrics) *MarketDataMetrics {
	return &MarketDataMetrics{
		metrics: metrics,
	}
}

// RecordTickerUpdate records a ticker update
func (mdm *MarketDataMetrics) RecordTickerUpdate(exchange, symbol string) {
	mdm.metrics.TickerUpdates.WithLabelValues(exchange, symbol).Inc()
}

// UpdateOrderbookDepth updates orderbook depth
func (mdm *MarketDataMetrics) UpdateOrderbookDepth(exchange, symbol, side string, depth float64) {
	mdm.metrics.OrderbookDepth.WithLabelValues(exchange, symbol, side).Set(depth)
}

// MetricsRegistry provides a centralized registry for all metrics collectors
type MetricsRegistry struct {
	Core       *Metrics
	Collector  *Collector
	Order      *OrderMetrics
	WebSocket  *WebSocketMetrics
	Trading    *TradingMetrics
	Risk       *RiskMetrics
	MarketData *MarketDataMetrics
}

// NewMetricsRegistry creates a new metrics registry
func NewMetricsRegistry() *MetricsRegistry {
	metrics := NewMetrics()
	
	return &MetricsRegistry{
		Core:       metrics,
		Collector:  NewCollector(metrics, 10*time.Second),
		Order:      NewOrderMetrics(metrics),
		WebSocket:  NewWebSocketMetrics(metrics),
		Trading:    NewTradingMetrics(metrics),
		Risk:       NewRiskMetrics(metrics),
		MarketData: NewMarketDataMetrics(metrics),
	}
}

// Start starts all metric collectors
func (mr *MetricsRegistry) Start() {
	mr.Collector.Start()
}

// Stop stops all metric collectors
func (mr *MetricsRegistry) Stop() {
	mr.Collector.Stop()
}

// StartServer starts the Prometheus metrics server
func (mr *MetricsRegistry) StartServer(port string) error {
	return mr.Core.StartServer(port)
}