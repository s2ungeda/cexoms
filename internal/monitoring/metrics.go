package monitoring

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics
type Metrics struct {
	// Order metrics
	OrdersTotal         *prometheus.CounterVec
	OrdersActive        *prometheus.GaugeVec
	OrderDuration       *prometheus.HistogramVec
	OrderVolume         *prometheus.CounterVec
	
	// Position metrics
	PositionsOpen       *prometheus.GaugeVec
	PositionPnL         *prometheus.GaugeVec
	PositionMargin      *prometheus.GaugeVec
	PositionLeverage    *prometheus.GaugeVec
	
	// Account metrics
	AccountBalance      *prometheus.GaugeVec
	AccountEquity       *prometheus.GaugeVec
	AccountMarginLevel  *prometheus.GaugeVec
	TransferCount       *prometheus.CounterVec
	TransferVolume      *prometheus.CounterVec
	
	// Risk metrics
	RiskChecks          *prometheus.CounterVec
	RiskCheckDuration   *prometheus.HistogramVec
	RiskViolations      *prometheus.CounterVec
	KillSwitchStatus    *prometheus.GaugeVec
	
	// Strategy metrics
	StrategySignals     *prometheus.CounterVec
	StrategyPerformance *prometheus.GaugeVec
	StrategyLatency     *prometheus.HistogramVec
	
	// System metrics
	APIRequests         *prometheus.CounterVec
	APIRequestDuration  *prometheus.HistogramVec
	APIErrors           *prometheus.CounterVec
	WebSocketConnections *prometheus.GaugeVec
	
	// Exchange metrics
	ExchangeLatency     *prometheus.HistogramVec
	ExchangeErrors      *prometheus.CounterVec
	RateLimitUsage      *prometheus.GaugeVec
}

// NewMetrics creates all Prometheus metrics
func NewMetrics() *Metrics {
	return &Metrics{
		// Order metrics
		OrdersTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "oms_orders_total",
			Help: "Total number of orders by account, exchange, symbol, and status",
		}, []string{"account_id", "exchange", "symbol", "side", "type", "status"}),
		
		OrdersActive: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "oms_orders_active",
			Help: "Number of active orders by account and exchange",
		}, []string{"account_id", "exchange", "symbol"}),
		
		OrderDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "oms_order_duration_seconds",
			Help:    "Order execution duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
		}, []string{"account_id", "exchange", "type"}),
		
		OrderVolume: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "oms_order_volume_usd",
			Help: "Total order volume in USD",
		}, []string{"account_id", "exchange", "symbol", "side"}),
		
		// Position metrics
		PositionsOpen: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "oms_positions_open",
			Help: "Number of open positions by account",
		}, []string{"account_id", "exchange", "symbol", "side"}),
		
		PositionPnL: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "oms_position_pnl_usd",
			Help: "Position P&L in USD",
		}, []string{"account_id", "exchange", "symbol", "side"}),
		
		PositionMargin: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "oms_position_margin_usd",
			Help: "Position margin in USD",
		}, []string{"account_id", "exchange", "symbol"}),
		
		PositionLeverage: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "oms_position_leverage",
			Help: "Position leverage",
		}, []string{"account_id", "exchange", "symbol"}),
		
		// Account metrics
		AccountBalance: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "oms_account_balance_usd",
			Help: "Account balance in USD",
		}, []string{"account_id", "exchange", "asset"}),
		
		AccountEquity: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "oms_account_equity_usd",
			Help: "Account equity (balance + unrealized PnL) in USD",
		}, []string{"account_id", "exchange"}),
		
		AccountMarginLevel: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "oms_account_margin_level",
			Help: "Account margin level (equity / used margin)",
		}, []string{"account_id", "exchange"}),
		
		TransferCount: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "oms_transfers_total",
			Help: "Total number of transfers",
		}, []string{"from_account", "to_account", "asset", "status"}),
		
		TransferVolume: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "oms_transfer_volume_usd",
			Help: "Total transfer volume in USD",
		}, []string{"from_account", "to_account", "asset"}),
		
		// Risk metrics
		RiskChecks: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "oms_risk_checks_total",
			Help: "Total number of risk checks",
		}, []string{"account_id", "check_type", "result"}),
		
		RiskCheckDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "oms_risk_check_duration_seconds",
			Help:    "Risk check duration in seconds",
			Buckets: prometheus.ExponentialBuckets(0.000001, 2, 10), // 1μs to ~1ms
		}, []string{"check_type"}),
		
		RiskViolations: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "oms_risk_violations_total",
			Help: "Total number of risk violations",
		}, []string{"account_id", "violation_type", "severity"}),
		
		KillSwitchStatus: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "oms_kill_switch_status",
			Help: "Kill switch status (0=off, 1=on)",
		}, []string{"account_id", "scope"}),
		
		// Strategy metrics
		StrategySignals: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "oms_strategy_signals_total",
			Help: "Total number of strategy signals",
		}, []string{"strategy", "signal_type", "symbol"}),
		
		StrategyPerformance: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "oms_strategy_performance",
			Help: "Strategy performance metrics",
		}, []string{"strategy", "metric"}), // metric: sharpe_ratio, win_rate, profit_factor
		
		StrategyLatency: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "oms_strategy_latency_seconds",
			Help:    "Strategy execution latency",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
		}, []string{"strategy", "operation"}),
		
		// System metrics
		APIRequests: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "oms_api_requests_total",
			Help: "Total number of API requests",
		}, []string{"method", "service", "status"}),
		
		APIRequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "oms_api_request_duration_seconds",
			Help:    "API request duration in seconds",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "service"}),
		
		APIErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "oms_api_errors_total",
			Help: "Total number of API errors",
		}, []string{"method", "service", "error_type"}),
		
		WebSocketConnections: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "oms_websocket_connections",
			Help: "Number of active WebSocket connections",
		}, []string{"exchange", "stream_type"}),
		
		// Exchange metrics
		ExchangeLatency: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "oms_exchange_latency_seconds",
			Help:    "Exchange API latency",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
		}, []string{"exchange", "operation"}),
		
		ExchangeErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "oms_exchange_errors_total",
			Help: "Total number of exchange errors",
		}, []string{"exchange", "error_type"}),
		
		RateLimitUsage: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "oms_rate_limit_usage",
			Help: "Rate limit usage percentage",
		}, []string{"exchange", "endpoint"}),
	}
}

// RecordOrder records order metrics
func (m *Metrics) RecordOrder(accountID, exchange, symbol, side, orderType, status string, volume float64, duration time.Duration) {
	labels := prometheus.Labels{
		"account_id": accountID,
		"exchange":   exchange,
		"symbol":     symbol,
		"side":       side,
		"type":       orderType,
		"status":     status,
	}
	
	m.OrdersTotal.With(labels).Inc()
	
	if status == "NEW" || status == "PARTIALLY_FILLED" {
		m.OrdersActive.With(prometheus.Labels{
			"account_id": accountID,
			"exchange":   exchange,
			"symbol":     symbol,
		}).Inc()
	}
	
	m.OrderVolume.With(prometheus.Labels{
		"account_id": accountID,
		"exchange":   exchange,
		"symbol":     symbol,
		"side":       side,
	}).Add(volume)
	
	m.OrderDuration.With(prometheus.Labels{
		"account_id": accountID,
		"exchange":   exchange,
		"type":       orderType,
	}).Observe(duration.Seconds())
}

// RecordPosition records position metrics
func (m *Metrics) RecordPosition(accountID, exchange, symbol, side string, pnl, margin, leverage float64) {
	m.PositionPnL.With(prometheus.Labels{
		"account_id": accountID,
		"exchange":   exchange,
		"symbol":     symbol,
		"side":       side,
	}).Set(pnl)
	
	m.PositionMargin.With(prometheus.Labels{
		"account_id": accountID,
		"exchange":   exchange,
		"symbol":     symbol,
	}).Set(margin)
	
	m.PositionLeverage.With(prometheus.Labels{
		"account_id": accountID,
		"exchange":   exchange,
		"symbol":     symbol,
	}).Set(leverage)
}

// RecordAccountBalance records account balance
func (m *Metrics) RecordAccountBalance(accountID, exchange, asset string, balance float64) {
	m.AccountBalance.With(prometheus.Labels{
		"account_id": accountID,
		"exchange":   exchange,
		"asset":      asset,
	}).Set(balance)
}

// RecordRiskCheck records risk check metrics
func (m *Metrics) RecordRiskCheck(accountID, checkType, result string, duration time.Duration) {
	m.RiskChecks.With(prometheus.Labels{
		"account_id": accountID,
		"check_type": checkType,
		"result":     result,
	}).Inc()
	
	m.RiskCheckDuration.With(prometheus.Labels{
		"check_type": checkType,
	}).Observe(duration.Seconds())
}

// RecordAPIRequest records API request metrics
func (m *Metrics) RecordAPIRequest(method, service, status string, duration time.Duration) {
	m.APIRequests.With(prometheus.Labels{
		"method":  method,
		"service": service,
		"status":  status,
	}).Inc()
	
	m.APIRequestDuration.With(prometheus.Labels{
		"method":  method,
		"service": service,
	}).Observe(duration.Seconds())
}