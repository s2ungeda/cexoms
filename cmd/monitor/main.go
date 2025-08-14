package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mExOms/oms/internal/monitor"
	"github.com/mExOms/oms/internal/position"
	"github.com/mExOms/oms/internal/risk"
)

var (
	metricsDir   = flag.String("metrics-dir", "./data/metrics", "Directory for metrics files")
	logsDir      = flag.String("logs-dir", "./logs", "Directory for log files")
	httpAddr     = flag.String("http-addr", ":8080", "HTTP server address")
	dashboardAddr = flag.String("dashboard-addr", ":8081", "Dashboard server address")
)

func main() {
	flag.Parse()

	fmt.Println("=== OMS Monitoring System ===\n")

	// Create directories
	for _, dir := range []string{*metricsDir, *logsDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatal("Failed to create directory:", err)
		}
	}

	// Initialize components
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create logger
	logger, err := monitor.NewLogger("monitor", *logsDir)
	if err != nil {
		log.Fatal("Failed to create logger:", err)
	}
	defer logger.Close()

	logger.Info("Starting monitoring system", map[string]interface{}{
		"metrics_dir": *metricsDir,
		"logs_dir":    *logsDir,
		"http_addr":   *httpAddr,
	})

	// Create metrics collector
	metrics, err := monitor.NewMetricsCollector(*metricsDir)
	if err != nil {
		log.Fatal("Failed to create metrics collector:", err)
	}
	defer metrics.Close()

	// Create health checker
	health := monitor.NewHealthChecker("1.0.0")
	
	// Register health checks
	registerHealthChecks(health)

	// Create mock dependencies for demo
	positionManager, _ := position.NewPositionManager("./data/snapshots")
	defer positionManager.Close()
	
	riskEngine := risk.NewRiskEngine()

	// Create dashboard server
	dashboardDeps := monitor.DashboardDeps{
		Metrics:         metrics,
		Health:          health,
		Logger:          logger,
		PositionManager: positionManager,
		RiskEngine:      riskEngine,
	}
	dashboard := monitor.NewDashboardServer(*dashboardAddr, dashboardDeps)

	// Start HTTP server for health and metrics
	mux := http.NewServeMux()
	mux.HandleFunc("/health", health.HTTPHandler())
	mux.HandleFunc("/metrics", handleMetrics(metrics))
	mux.HandleFunc("/logs/query", handleLogsQuery(logger))

	httpServer := &http.Server{
		Addr:    *httpAddr,
		Handler: mux,
	}

	// Start servers
	go func() {
		logger.Info("Starting HTTP server", map[string]interface{}{
			"address": *httpAddr,
		})
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", err, nil)
		}
	}()

	go func() {
		logger.Info("Starting dashboard server", map[string]interface{}{
			"address": *dashboardAddr,
		})
		if err := dashboard.Start(); err != nil {
			logger.Error("Dashboard server error", err, nil)
		}
	}()

	// Start metric collection
	go collectSystemMetrics(ctx, metrics, logger)

	fmt.Println("✓ Monitoring system started")
	fmt.Printf("  HTTP API: http://localhost%s\n", *httpAddr)
	fmt.Printf("  Dashboard: http://localhost%s\n", *dashboardAddr)
	fmt.Println()
	fmt.Println("Endpoints:")
	fmt.Printf("  Health: http://localhost%s/health\n", *httpAddr)
	fmt.Printf("  Metrics: http://localhost%s/metrics\n", *httpAddr)
	fmt.Printf("  Logs: http://localhost%s/logs/query\n", *httpAddr)
	fmt.Println()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down monitoring system")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", err, nil)
	}

	cancel()
	fmt.Println("\n✓ Monitoring system stopped")
}

func registerHealthChecks(health *monitor.HealthChecker) {
	// Register component health checks
	health.RegisterCheck("nats", monitor.NATSHealthCheck("nats://localhost:4222"))
	health.RegisterCheck("filesystem", monitor.FileSystemHealthCheck("./data"))
	health.RegisterCheck("memory", monitor.MemoryHealthCheck(80.0))
	health.RegisterCheck("position_manager", monitor.PositionManagerHealthCheck())
	health.RegisterCheck("risk_engine", monitor.RiskEngineHealthCheck())
	
	// Exchange health checks
	health.RegisterCheck("binance", monitor.ExchangeHealthCheck("binance"))
	health.RegisterCheck("okx", monitor.ExchangeHealthCheck("okx"))
	health.RegisterCheck("bybit", monitor.ExchangeHealthCheck("bybit"))
}

func handleMetrics(metrics *monitor.MetricsCollector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get metrics in Prometheus format
		data := metrics.GetMetrics()
		
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		
		// Convert to Prometheus format
		fmt.Fprintf(w, "# HELP oms_orders_total Total number of orders processed\n")
		fmt.Fprintf(w, "# TYPE oms_orders_total counter\n")
		
		if counters, ok := data["counters"].(map[string]int64); ok {
			for name, value := range counters {
				fmt.Fprintf(w, "oms_%s %d\n", name, value)
			}
		}
		
		fmt.Fprintf(w, "\n# HELP oms_latency_seconds Order processing latency\n")
		fmt.Fprintf(w, "# TYPE oms_latency_seconds histogram\n")
		
		// Add more metrics as needed
	}
}

func handleLogsQuery(logger *monitor.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Simple log query endpoint
		query := r.URL.Query()
		level := query.Get("level")
		component := query.Get("component")
		limit := query.Get("limit")
		
		w.Header().Set("Content-Type", "application/json")
		
		// In production, implement actual log querying
		fmt.Fprintf(w, `{
			"query": {
				"level": "%s",
				"component": "%s",
				"limit": "%s"
			},
			"results": [
				{
					"timestamp": "%s",
					"level": "INFO",
					"component": "monitor",
					"message": "Sample log entry"
				}
			]
		}`, level, component, limit, time.Now().Format(time.RFC3339))
	}
}

func collectSystemMetrics(ctx context.Context, metrics *monitor.MetricsCollector, logger *monitor.Logger) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Collect system metrics
			collectOrderMetrics(metrics)
			collectPerformanceMetrics(metrics)
			collectSystemResourceMetrics(metrics)
			
			logger.Debug("System metrics collected", map[string]interface{}{
				"timestamp": time.Now(),
			})
		}
	}
}

func collectOrderMetrics(metrics *monitor.MetricsCollector) {
	// Simulate order metrics
	metrics.IncrementCounter("orders_placed", map[string]string{
		"exchange": "binance",
		"type":     "limit",
	})
	
	metrics.ObserveHistogram("order_latency_ms", float64(2+time.Now().Unix()%5), map[string]string{
		"exchange": "binance",
	})
}

func collectPerformanceMetrics(metrics *monitor.MetricsCollector) {
	// Simulate performance metrics
	metrics.SetGauge("active_connections", float64(42+time.Now().Unix()%10), nil)
	metrics.SetGauge("goroutines", float64(150+time.Now().Unix()%50), nil)
	
	metrics.ObserveSummary("request_duration_ms", float64(5+time.Now().Unix()%20), map[string]string{
		"endpoint": "/api/orders",
	})
}

func collectSystemResourceMetrics(metrics *monitor.MetricsCollector) {
	// Simulate system resource metrics
	metrics.SetGauge("cpu_percent", 15.5+float64(time.Now().Unix()%20), nil)
	metrics.SetGauge("memory_mb", 2048+float64(time.Now().Unix()%512), nil)
	metrics.SetGauge("disk_usage_percent", 45.2, map[string]string{
		"path": "/data",
	})
}