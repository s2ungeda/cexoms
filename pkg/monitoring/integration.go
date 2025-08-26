package monitoring

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/mExOms/pkg/alerting"
	"github.com/mExOms/pkg/metrics"
	"github.com/mExOms/pkg/tracing"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Config holds monitoring configuration
type Config struct {
	MetricsPort      string
	TracingExporter  string
	TracingEndpoint  string
	SamplingRate     float64
	AlertWebhookURL  string
	SlackWebhookURL  string
	PagerDutyKey     string
	Environment      string
}

// Monitor provides integrated monitoring capabilities
type Monitor struct {
	config   Config
	metrics  *metrics.MetricsRegistry
	oms      *metrics.OMSMetrics
	tracer   *tracing.Tracer
	notifier *alerting.Notifier
	alertMgr *alerting.AlertManager
	server   *http.Server
}

// NewMonitor creates a new monitoring instance
func NewMonitor(cfg Config) (*Monitor, error) {
	// Initialize metrics
	metricsRegistry := metrics.NewMetricsRegistry()
	omsMetrics := metrics.NewOMSMetrics(metricsRegistry)

	// Initialize tracing
	tracerCfg := tracing.Config{
		ServiceName:    "oms",
		ServiceVersion: "1.0.0",
		Environment:    cfg.Environment,
		Exporter:       cfg.TracingExporter,
		Endpoint:       cfg.TracingEndpoint,
		SamplingRate:   cfg.SamplingRate,
	}
	tracer, err := tracing.NewTracer(tracerCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create tracer: %w", err)
	}

	// Initialize notification channels
	var channels []alerting.NotificationChannel
	if cfg.SlackWebhookURL != "" {
		channels = append(channels, alerting.NewSlackChannel(cfg.SlackWebhookURL, "#oms-alerts"))
	}
	if cfg.PagerDutyKey != "" {
		channels = append(channels, alerting.NewPagerDutyChannel(cfg.PagerDutyKey))
	}

	notifier := alerting.NewNotifier(channels...)
	alertMgr := alerting.NewAlertManager(notifier)

	// Set up metrics HTTP server
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", healthHandler(omsMetrics))
	
	server := &http.Server{
		Addr:    ":" + cfg.MetricsPort,
		Handler: mux,
	}

	return &Monitor{
		config:   cfg,
		metrics:  metricsRegistry,
		oms:      omsMetrics,
		tracer:   tracer,
		notifier: notifier,
		alertMgr: alertMgr,
		server:   server,
	}, nil
}

// Start starts all monitoring components
func (m *Monitor) Start(ctx context.Context) error {
	// Start metrics collection
	m.metrics.Start()
	
	// Start OMS metrics periodic collection
	m.oms.StartPeriodicCollection(ctx, 30*time.Second)
	
	// Start alert manager
	m.alertMgr.Start(ctx, 30*time.Second)
	
	// Add default alert rules
	m.setupDefaultAlerts()
	
	// Start metrics HTTP server
	go func() {
		log.Printf("Starting metrics server on port %s", m.config.MetricsPort)
		if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Metrics server error: %v", err)
		}
	}()
	
	return nil
}

// Stop gracefully stops all monitoring components
func (m *Monitor) Stop(ctx context.Context) error {
	// Stop metrics collection
	m.metrics.Stop()
	
	// Shutdown tracer
	if err := m.tracer.Shutdown(ctx); err != nil {
		log.Printf("Failed to shutdown tracer: %v", err)
	}
	
	// Shutdown metrics server
	if err := m.server.Shutdown(ctx); err != nil {
		log.Printf("Failed to shutdown metrics server: %v", err)
	}
	
	return nil
}

// GetMetrics returns the metrics registry
func (m *Monitor) GetMetrics() *metrics.MetricsRegistry {
	return m.metrics
}

// GetOMSMetrics returns OMS-specific metrics
func (m *Monitor) GetOMSMetrics() *metrics.OMSMetrics {
	return m.oms
}

// GetTracer returns the tracer
func (m *Monitor) GetTracer() *tracing.Tracer {
	return m.tracer
}

// GetNotifier returns the notifier
func (m *Monitor) GetNotifier() *alerting.Notifier {
	return m.notifier
}

// setupDefaultAlerts sets up default monitoring alerts
func (m *Monitor) setupDefaultAlerts() {
	// High error rate alert
	m.alertMgr.AddRule(alerting.AlertRule{
		Name:      "HighErrorRate",
		Component: "system",
		Level:     alerting.AlertLevelCritical,
		Message:   "High system error rate detected",
		Condition: func() (bool, map[string]interface{}) {
			// This would check actual metrics
			return false, nil
		},
	})
	
	// Memory usage alert
	m.alertMgr.AddRule(alerting.AlertRule{
		Name:      "HighMemoryUsage",
		Component: "system",
		Level:     alerting.AlertLevelWarning,
		Message:   "Memory usage exceeds threshold",
		Condition: func() (bool, map[string]interface{}) {
			// This would check actual metrics
			return false, nil
		},
	})
	
	// Exchange disconnection alert
	m.alertMgr.AddRule(alerting.AlertRule{
		Name:      "ExchangeDisconnected",
		Component: "exchange",
		Level:     alerting.AlertLevelCritical,
		Message:   "Exchange connection lost",
		Condition: func() (bool, map[string]interface{}) {
			// This would check actual connection status
			return false, nil
		},
	})
}

// healthHandler returns a health check HTTP handler
func healthHandler(oms *metrics.OMSMetrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		health := oms.GetHealthReport()
		
		// Determine overall health status
		isHealthy := true
		if exchanges, ok := health["exchange_health"].(map[string]bool); ok {
			for _, healthy := range exchanges {
				if !healthy {
					isHealthy = false
					break
				}
			}
		}
		
		// Return appropriate status code
		if isHealthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		
		// Write health report
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"status": "%s",
			"timestamp": "%s",
			"details": %v
		}`, 
			map[bool]string{true: "healthy", false: "unhealthy"}[isHealthy],
			time.Now().Format(time.RFC3339),
			health,
		)
	}
}

// MonitoringMiddleware provides HTTP middleware for monitoring
type MonitoringMiddleware struct {
	monitor *Monitor
}

// NewMonitoringMiddleware creates monitoring middleware
func NewMonitoringMiddleware(monitor *Monitor) *MonitoringMiddleware {
	return &MonitoringMiddleware{
		monitor: monitor,
	}
}

// Handler wraps an HTTP handler with monitoring
func (mm *MonitoringMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Start trace span
		ctx, span := mm.monitor.tracer.StartSpan(r.Context(), "http.request")
		defer span.End()
		
		// Add trace attributes
		tracing.SetAttributes(ctx,
			tracing.ExchangeAPISpan("http", r.URL.Path, r.Method)...,
		)
		
		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		
		// Call next handler
		next.ServeHTTP(wrapped, r.WithContext(ctx))
		
		// Record metrics
		duration := time.Since(start)
		mm.monitor.metrics.Core.RecordAPILatency("http", r.URL.Path, r.Method, duration)
		
		// Record errors
		if wrapped.statusCode >= 400 {
			mm.monitor.metrics.Core.ExchangeErrors.WithLabelValues(
				"http",
				fmt.Sprintf("%d", wrapped.statusCode),
				r.URL.Path,
			).Inc()
		}
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}