package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metrics for the OMS
type Metrics struct {
	// Order metrics
	OrdersTotal         *prometheus.CounterVec
	OrderDuration       *prometheus.HistogramVec
	OrderErrors         *prometheus.CounterVec
	ActiveOrders        *prometheus.GaugeVec
	OrderVolume         *prometheus.CounterVec
	
	// WebSocket metrics
	WSConnections       *prometheus.GaugeVec
	WSMessages          *prometheus.CounterVec
	WSReconnects        *prometheus.CounterVec
	WSLatency           *prometheus.HistogramVec
	
	// Exchange metrics
	ExchangeAPILatency  *prometheus.HistogramVec
	ExchangeRateLimit   *prometheus.GaugeVec
	ExchangeErrors      *prometheus.CounterVec
	
	// System metrics
	MemoryUsage         prometheus.Gauge
	CPUUsage            prometheus.Gauge
	GoroutineCount      prometheus.Gauge
	
	// Trading metrics
	PnL                 *prometheus.GaugeVec
	TradingVolume       *prometheus.CounterVec
	Positions           *prometheus.GaugeVec
	Balance             *prometheus.GaugeVec
	
	// Risk metrics
	RiskScore           *prometheus.GaugeVec
	MarginUsage         *prometheus.GaugeVec
	Leverage            *prometheus.GaugeVec
	DrawdownPercent     *prometheus.GaugeVec
	
	// Market data metrics
	TickerUpdates       *prometheus.CounterVec
	OrderbookDepth      *prometheus.GaugeVec
	SpreadBasisPoints   *prometheus.GaugeVec
	
	// Router metrics
	RouteDecisions      *prometheus.CounterVec
	RouteLatency        *prometheus.HistogramVec
	RouteSplits         *prometheus.HistogramVec
}

// NewMetrics creates and registers all Prometheus metrics
func NewMetrics() *Metrics {
	m := &Metrics{
		// Order metrics
		OrdersTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "oms_orders_total",
				Help: "Total number of orders processed",
			},
			[]string{"exchange", "market", "type", "side", "status"},
		),
		OrderDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "oms_order_duration_seconds",
				Help:    "Order processing duration in seconds",
				Buckets: prometheus.ExponentialBuckets(0.0001, 2, 15), // 100μs to 1.6s
			},
			[]string{"exchange", "market", "type"},
		),
		OrderErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "oms_order_errors_total",
				Help: "Total number of order errors",
			},
			[]string{"exchange", "market", "error_type"},
		),
		ActiveOrders: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "oms_active_orders",
				Help: "Number of currently active orders",
			},
			[]string{"exchange", "market", "type"},
		),
		OrderVolume: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "oms_order_volume_total",
				Help: "Total order volume in USD",
			},
			[]string{"exchange", "market", "side"},
		),
		
		// WebSocket metrics
		WSConnections: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "oms_websocket_connections",
				Help: "Number of active WebSocket connections",
			},
			[]string{"exchange", "stream_type"},
		),
		WSMessages: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "oms_websocket_messages_total",
				Help: "Total number of WebSocket messages",
			},
			[]string{"exchange", "stream_type", "direction"},
		),
		WSReconnects: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "oms_websocket_reconnects_total",
				Help: "Total number of WebSocket reconnections",
			},
			[]string{"exchange", "stream_type"},
		),
		WSLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "oms_websocket_latency_seconds",
				Help:    "WebSocket message latency",
				Buckets: prometheus.ExponentialBuckets(0.00001, 2, 15), // 10μs to 160ms
			},
			[]string{"exchange", "stream_type"},
		),
		
		// Exchange metrics
		ExchangeAPILatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "oms_exchange_api_latency_seconds",
				Help:    "Exchange API call latency",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 12), // 1ms to 2s
			},
			[]string{"exchange", "endpoint", "method"},
		),
		ExchangeRateLimit: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "oms_exchange_rate_limit_remaining",
				Help: "Remaining rate limit for exchange API",
			},
			[]string{"exchange", "limit_type"},
		),
		ExchangeErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "oms_exchange_errors_total",
				Help: "Total number of exchange API errors",
			},
			[]string{"exchange", "error_code", "endpoint"},
		),
		
		// System metrics
		MemoryUsage: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "oms_memory_usage_bytes",
				Help: "Current memory usage in bytes",
			},
		),
		CPUUsage: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "oms_cpu_usage_percent",
				Help: "Current CPU usage percentage",
			},
		),
		GoroutineCount: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "oms_goroutines",
				Help: "Number of active goroutines",
			},
		),
		
		// Trading metrics
		PnL: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "oms_pnl_usd",
				Help: "Profit and Loss in USD",
			},
			[]string{"exchange", "market", "period"},
		),
		TradingVolume: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "oms_trading_volume_usd_total",
				Help: "Total trading volume in USD",
			},
			[]string{"exchange", "market"},
		),
		Positions: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "oms_positions",
				Help: "Current open positions",
			},
			[]string{"exchange", "market", "symbol", "side"},
		),
		Balance: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "oms_balance_usd",
				Help: "Account balance in USD",
			},
			[]string{"exchange", "asset"},
		),
		
		// Risk metrics
		RiskScore: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "oms_risk_score",
				Help: "Current risk score (0-100)",
			},
			[]string{"exchange", "market"},
		),
		MarginUsage: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "oms_margin_usage_percent",
				Help: "Margin usage percentage",
			},
			[]string{"exchange", "market"},
		),
		Leverage: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "oms_leverage",
				Help: "Current leverage",
			},
			[]string{"exchange", "market"},
		),
		DrawdownPercent: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "oms_drawdown_percent",
				Help: "Maximum drawdown percentage",
			},
			[]string{"exchange", "period"},
		),
		
		// Market data metrics
		TickerUpdates: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "oms_ticker_updates_total",
				Help: "Total number of ticker updates received",
			},
			[]string{"exchange", "symbol"},
		),
		OrderbookDepth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "oms_orderbook_depth",
				Help: "Current orderbook depth",
			},
			[]string{"exchange", "symbol", "side"},
		),
		SpreadBasisPoints: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "oms_spread_basis_points",
				Help: "Bid-ask spread in basis points",
			},
			[]string{"exchange", "symbol"},
		),
		
		// Router metrics
		RouteDecisions: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "oms_route_decisions_total",
				Help: "Total number of routing decisions",
			},
			[]string{"strategy", "selected_exchange"},
		),
		RouteLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "oms_route_latency_seconds",
				Help:    "Routing decision latency",
				Buckets: prometheus.ExponentialBuckets(0.00001, 2, 12), // 10μs to 20ms
			},
			[]string{"strategy"},
		),
		RouteSplits: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "oms_route_splits",
				Help:    "Number of exchanges an order was split across",
				Buckets: prometheus.LinearBuckets(1, 1, 10),
			},
			[]string{"strategy"},
		),
	}
	
	// Register all metrics
	prometheus.MustRegister(
		m.OrdersTotal,
		m.OrderDuration,
		m.OrderErrors,
		m.ActiveOrders,
		m.OrderVolume,
		m.WSConnections,
		m.WSMessages,
		m.WSReconnects,
		m.WSLatency,
		m.ExchangeAPILatency,
		m.ExchangeRateLimit,
		m.ExchangeErrors,
		m.MemoryUsage,
		m.CPUUsage,
		m.GoroutineCount,
		m.PnL,
		m.TradingVolume,
		m.Positions,
		m.Balance,
		m.RiskScore,
		m.MarginUsage,
		m.Leverage,
		m.DrawdownPercent,
		m.TickerUpdates,
		m.OrderbookDepth,
		m.SpreadBasisPoints,
		m.RouteDecisions,
		m.RouteLatency,
		m.RouteSplits,
	)
	
	return m
}

// StartServer starts the Prometheus metrics HTTP server
func (m *Metrics) StartServer(port string) error {
	http.Handle("/metrics", promhttp.Handler())
	return http.ListenAndServe(":"+port, nil)
}

// RecordOrderDuration records the duration of an order operation
func (m *Metrics) RecordOrderDuration(exchange, market, orderType string, duration time.Duration) {
	m.OrderDuration.WithLabelValues(exchange, market, orderType).Observe(duration.Seconds())
}

// RecordWSLatency records WebSocket message latency
func (m *Metrics) RecordWSLatency(exchange, streamType string, latency time.Duration) {
	m.WSLatency.WithLabelValues(exchange, streamType).Observe(latency.Seconds())
}

// RecordAPILatency records Exchange API call latency
func (m *Metrics) RecordAPILatency(exchange, endpoint, method string, latency time.Duration) {
	m.ExchangeAPILatency.WithLabelValues(exchange, endpoint, method).Observe(latency.Seconds())
}

// RecordRouteDecision records a routing decision
func (m *Metrics) RecordRouteDecision(strategy, selectedExchange string) {
	m.RouteDecisions.WithLabelValues(strategy, selectedExchange).Inc()
}

// UpdateSpread updates the bid-ask spread for a symbol
func (m *Metrics) UpdateSpread(exchange, symbol string, spreadBps float64) {
	m.SpreadBasisPoints.WithLabelValues(exchange, symbol).Set(spreadBps)
}