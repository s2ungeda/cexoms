package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// HealthChecker provides health check functionality
type HealthChecker struct {
	mu sync.RWMutex
	
	// Component checks
	exchangeChecks    map[string]*ExchangeHealthCheck
	accountChecks     map[string]*AccountHealthCheck
	serviceChecks     map[string]*ServiceHealthCheck
	
	// Health status
	overallHealth     atomic.Value // *HealthStatus
	lastCheck         time.Time
	
	// Configuration
	config            *HealthCheckConfig
	logger            *zap.Logger
	
	// HTTP server
	server            *http.Server
	router            *mux.Router
	
	// Dependencies
	metricsCollector  *MetricsCollector
	
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// HealthCheckConfig holds health check configuration
type HealthCheckConfig struct {
	Port              int
	CheckInterval     time.Duration
	Timeout           time.Duration
	
	// Thresholds
	MaxLatency        time.Duration
	MinSuccessRate    float64
	MaxErrorRate      float64
	MaxQueueDepth     int
	MinFreeMemoryMB   int
	MaxCPUPercent     float64
}

// DefaultHealthCheckConfig returns default configuration
func DefaultHealthCheckConfig() *HealthCheckConfig {
	return &HealthCheckConfig{
		Port:            8081,
		CheckInterval:   10 * time.Second,
		Timeout:         5 * time.Second,
		MaxLatency:      500 * time.Millisecond,
		MinSuccessRate:  0.95,
		MaxErrorRate:    0.05,
		MaxQueueDepth:   1000,
		MinFreeMemoryMB: 100,
		MaxCPUPercent:   80.0,
	}
}

// HealthStatus represents overall health status
type HealthStatus struct {
	Status      string                 `json:"status"` // "healthy", "degraded", "unhealthy"
	Timestamp   time.Time              `json:"timestamp"`
	Version     string                 `json:"version"`
	Uptime      string                 `json:"uptime"`
	Checks      map[string]CheckResult `json:"checks"`
	Metrics     map[string]interface{} `json:"metrics"`
}

// CheckResult represents a health check result
type CheckResult struct {
	Status      string                 `json:"status"`
	Message     string                 `json:"message,omitempty"`
	Details     map[string]interface{} `json:"details,omitempty"`
	LastChecked time.Time              `json:"last_checked"`
	Duration    time.Duration          `json:"duration_ms"`
}

// ExchangeHealthCheck tracks exchange health
type ExchangeHealthCheck struct {
	Exchange         string
	LastPing         time.Time
	LastPingLatency  time.Duration
	Connected        bool
	ErrorCount       int64
	SuccessCount     int64
	LastError        error
}

// AccountHealthCheck tracks account health
type AccountHealthCheck struct {
	AccountID        string
	Exchange         string
	Active           bool
	LastActivity     time.Time
	OrdersPlaced     int64
	OrdersFailed     int64
	RateLimitHits    int64
	Balance          float64
	LastError        error
}

// ServiceHealthCheck tracks service health
type ServiceHealthCheck struct {
	Service          string
	Status           string
	LastCheck        time.Time
	ResponseTime     time.Duration
	ErrorCount       int64
	Details          map[string]interface{}
}

var startTime = time.Now()

// NewHealthChecker creates a new health checker
func NewHealthChecker(config *HealthCheckConfig, metricsCollector *MetricsCollector, logger *zap.Logger) *HealthChecker {
	if config == nil {
		config = DefaultHealthCheckConfig()
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	hc := &HealthChecker{
		exchangeChecks:   make(map[string]*ExchangeHealthCheck),
		accountChecks:    make(map[string]*AccountHealthCheck),
		serviceChecks:    make(map[string]*ServiceHealthCheck),
		config:           config,
		logger:           logger,
		metricsCollector: metricsCollector,
		ctx:              ctx,
		cancel:           cancel,
	}
	
	// Initialize overall health
	hc.overallHealth.Store(&HealthStatus{
		Status:    "unknown",
		Timestamp: time.Now(),
		Version:   "1.0.0",
		Checks:    make(map[string]CheckResult),
		Metrics:   make(map[string]interface{}),
	})
	
	// Setup routes
	hc.setupRoutes()
	
	return hc
}

// Start starts the health checker
func (hc *HealthChecker) Start() error {
	// Start health check routine
	hc.wg.Add(1)
	go hc.runHealthChecks()
	
	// Start HTTP server
	hc.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", hc.config.Port),
		Handler:      hc.router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	
	hc.logger.Info("Starting health check server",
		zap.Int("port", hc.config.Port))
	
	go func() {
		if err := hc.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			hc.logger.Error("Health check server error", zap.Error(err))
		}
	}()
	
	return nil
}

// setupRoutes configures HTTP routes
func (hc *HealthChecker) setupRoutes() {
	hc.router = mux.NewRouter()
	
	// Health check endpoints
	hc.router.HandleFunc("/health", hc.handleHealth).Methods("GET")
	hc.router.HandleFunc("/health/live", hc.handleLiveness).Methods("GET")
	hc.router.HandleFunc("/health/ready", hc.handleReadiness).Methods("GET")
	
	// Detailed health endpoints
	hc.router.HandleFunc("/health/exchanges", hc.handleExchangeHealth).Methods("GET")
	hc.router.HandleFunc("/health/exchanges/{exchange}", hc.handleExchangeHealthDetail).Methods("GET")
	hc.router.HandleFunc("/health/accounts", hc.handleAccountHealth).Methods("GET")
	hc.router.HandleFunc("/health/accounts/{accountId}", hc.handleAccountHealthDetail).Methods("GET")
	hc.router.HandleFunc("/health/services", hc.handleServiceHealth).Methods("GET")
	hc.router.HandleFunc("/health/services/{service}", hc.handleServiceHealthDetail).Methods("GET")
	
	// Metrics endpoint
	hc.router.HandleFunc("/metrics", hc.handleMetrics).Methods("GET")
}

// RegisterExchange registers an exchange for health checking
func (hc *HealthChecker) RegisterExchange(exchange string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	
	if _, exists := hc.exchangeChecks[exchange]; !exists {
		hc.exchangeChecks[exchange] = &ExchangeHealthCheck{
			Exchange:  exchange,
			Connected: false,
		}
	}
}

// RegisterAccount registers an account for health checking
func (hc *HealthChecker) RegisterAccount(accountID, exchange string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	
	if _, exists := hc.accountChecks[accountID]; !exists {
		hc.accountChecks[accountID] = &AccountHealthCheck{
			AccountID: accountID,
			Exchange:  exchange,
			Active:    true,
		}
	}
}

// RegisterService registers a service for health checking
func (hc *HealthChecker) RegisterService(service string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	
	if _, exists := hc.serviceChecks[service]; !exists {
		hc.serviceChecks[service] = &ServiceHealthCheck{
			Service: service,
			Status:  "unknown",
			Details: make(map[string]interface{}),
		}
	}
}

// UpdateExchangeHealth updates exchange health status
func (hc *HealthChecker) UpdateExchangeHealth(exchange string, connected bool, latency time.Duration, err error) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	
	check, exists := hc.exchangeChecks[exchange]
	if !exists {
		check = &ExchangeHealthCheck{Exchange: exchange}
		hc.exchangeChecks[exchange] = check
	}
	
	check.Connected = connected
	check.LastPing = time.Now()
	check.LastPingLatency = latency
	
	if err != nil {
		check.ErrorCount++
		check.LastError = err
	} else {
		check.SuccessCount++
	}
}

// UpdateAccountHealth updates account health status
func (hc *HealthChecker) UpdateAccountHealth(accountID string, activity AccountActivity) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	
	check, exists := hc.accountChecks[accountID]
	if !exists {
		return
	}
	
	check.LastActivity = time.Now()
	check.OrdersPlaced += activity.OrdersPlaced
	check.OrdersFailed += activity.OrdersFailed
	check.RateLimitHits += activity.RateLimitHits
	check.Balance = activity.Balance
	
	if activity.Error != nil {
		check.LastError = activity.Error
	}
}

// AccountActivity represents account activity update
type AccountActivity struct {
	OrdersPlaced  int64
	OrdersFailed  int64
	RateLimitHits int64
	Balance       float64
	Error         error
}

// runHealthChecks periodically runs health checks
func (hc *HealthChecker) runHealthChecks() {
	defer hc.wg.Done()
	
	// Run initial check immediately
	hc.performHealthCheck()
	
	ticker := time.NewTicker(hc.config.CheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-hc.ctx.Done():
			return
		case <-ticker.C:
			hc.performHealthCheck()
		}
	}
}

// performHealthCheck performs all health checks
func (hc *HealthChecker) performHealthCheck() {
	startTime := time.Now()
	
	status := &HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now(),
		Version:   "1.0.0",
		Uptime:    time.Since(startTime).Round(time.Second).String(),
		Checks:    make(map[string]CheckResult),
		Metrics:   make(map[string]interface{}),
	}
	
	// System health check
	systemResult := hc.checkSystem()
	status.Checks["system"] = systemResult
	if systemResult.Status != "healthy" {
		status.Status = "degraded"
	}
	
	// Exchange health checks
	exchangeResults := hc.checkExchanges()
	for exchange, result := range exchangeResults {
		status.Checks["exchange_"+exchange] = result
		if result.Status == "unhealthy" {
			status.Status = "unhealthy"
		} else if result.Status == "degraded" && status.Status == "healthy" {
			status.Status = "degraded"
		}
	}
	
	// Account health checks
	accountResult := hc.checkAccounts()
	status.Checks["accounts"] = accountResult
	if accountResult.Status == "unhealthy" && status.Status != "unhealthy" {
		status.Status = "degraded"
	}
	
	// Service health checks
	serviceResults := hc.checkServices()
	for service, result := range serviceResults {
		status.Checks["service_"+service] = result
		if result.Status == "unhealthy" {
			status.Status = "unhealthy"
		}
	}
	
	// Add metrics
	if hc.metricsCollector != nil {
		globalMetrics := hc.metricsCollector.GetGlobalMetrics()
		status.Metrics = map[string]interface{}{
			"orders_per_second": globalMetrics.TotalOrdersPerSec.Load(),
			"active_accounts":   globalMetrics.TotalActiveAccounts.Load(),
			"cpu_percent":       float64(globalMetrics.CPUUsage.Load()) / 100.0,
			"memory_mb":         globalMetrics.MemoryUsage.Load() / 1024 / 1024,
			"goroutines":        globalMetrics.GoroutineCount.Load(),
		}
	}
	
	hc.lastCheck = time.Now()
	hc.overallHealth.Store(status)
	
	// Log if status changed
	if prev := hc.overallHealth.Load().(*HealthStatus); prev.Status != status.Status {
		hc.logger.Info("Health status changed",
			zap.String("from", prev.Status),
			zap.String("to", status.Status),
			zap.Duration("check_duration", time.Since(startTime)))
	}
}

// checkSystem checks system health
func (hc *HealthChecker) checkSystem() CheckResult {
	checkStart := time.Now()
	
	result := CheckResult{
		Status:      "healthy",
		LastChecked: checkStart,
		Details:     make(map[string]interface{}),
	}
	
	if hc.metricsCollector != nil {
		globalMetrics := hc.metricsCollector.GetGlobalMetrics()
		
		cpuPercent := float64(globalMetrics.CPUUsage.Load()) / 100.0
		memoryMB := globalMetrics.MemoryUsage.Load() / 1024 / 1024
		queueDepth := globalMetrics.MessageQueueDepth.Load()
		
		result.Details["cpu_percent"] = cpuPercent
		result.Details["memory_mb"] = memoryMB
		result.Details["queue_depth"] = queueDepth
		
		// Check thresholds
		if cpuPercent > hc.config.MaxCPUPercent {
			result.Status = "degraded"
			result.Message = fmt.Sprintf("High CPU usage: %.1f%%", cpuPercent)
		}
		
		if int(memoryMB) < hc.config.MinFreeMemoryMB {
			result.Status = "degraded"
			if result.Message != "" {
				result.Message += "; "
			}
			result.Message += fmt.Sprintf("Low memory: %dMB", memoryMB)
		}
		
		if int(queueDepth) > hc.config.MaxQueueDepth {
			result.Status = "degraded"
			if result.Message != "" {
				result.Message += "; "
			}
			result.Message += fmt.Sprintf("High queue depth: %d", queueDepth)
		}
	}
	
	result.Duration = time.Since(checkStart)
	return result
}

// checkExchanges checks all exchange health
func (hc *HealthChecker) checkExchanges() map[string]CheckResult {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	
	results := make(map[string]CheckResult)
	
	for exchange, check := range hc.exchangeChecks {
		checkStart := time.Now()
		
		result := CheckResult{
			Status:      "healthy",
			LastChecked: checkStart,
			Details:     make(map[string]interface{}),
		}
		
		// Check connection status
		if !check.Connected {
			result.Status = "unhealthy"
			result.Message = "Not connected"
		} else if time.Since(check.LastPing) > hc.config.CheckInterval*2 {
			result.Status = "degraded"
			result.Message = "No recent ping"
		}
		
		// Check latency
		if check.LastPingLatency > hc.config.MaxLatency {
			result.Status = "degraded"
			if result.Message != "" {
				result.Message += "; "
			}
			result.Message += fmt.Sprintf("High latency: %v", check.LastPingLatency)
		}
		
		// Calculate success rate
		totalChecks := check.SuccessCount + check.ErrorCount
		if totalChecks > 0 {
			successRate := float64(check.SuccessCount) / float64(totalChecks)
			result.Details["success_rate"] = successRate
			
			if successRate < hc.config.MinSuccessRate {
				result.Status = "degraded"
				if result.Message != "" {
					result.Message += "; "
				}
				result.Message += fmt.Sprintf("Low success rate: %.1f%%", successRate*100)
			}
		}
		
		result.Details["connected"] = check.Connected
		result.Details["last_ping"] = check.LastPing
		result.Details["latency_ms"] = check.LastPingLatency.Milliseconds()
		result.Details["error_count"] = check.ErrorCount
		
		if check.LastError != nil {
			result.Details["last_error"] = check.LastError.Error()
		}
		
		result.Duration = time.Since(checkStart)
		results[exchange] = result
	}
	
	return results
}

// checkAccounts checks account health
func (hc *HealthChecker) checkAccounts() CheckResult {
	checkStart := time.Now()
	
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	
	result := CheckResult{
		Status:      "healthy",
		LastChecked: checkStart,
		Details:     make(map[string]interface{}),
	}
	
	activeAccounts := 0
	totalAccounts := len(hc.accountChecks)
	accountsWithErrors := 0
	accountsWithRateLimits := 0
	
	for _, check := range hc.accountChecks {
		if check.Active && time.Since(check.LastActivity) < time.Hour {
			activeAccounts++
		}
		
		if check.LastError != nil {
			accountsWithErrors++
		}
		
		if check.RateLimitHits > 0 {
			accountsWithRateLimits++
		}
	}
	
	result.Details["total_accounts"] = totalAccounts
	result.Details["active_accounts"] = activeAccounts
	result.Details["accounts_with_errors"] = accountsWithErrors
	result.Details["accounts_with_rate_limits"] = accountsWithRateLimits
	
	// Check error rate
	if totalAccounts > 0 {
		errorRate := float64(accountsWithErrors) / float64(totalAccounts)
		if errorRate > hc.config.MaxErrorRate {
			result.Status = "degraded"
			result.Message = fmt.Sprintf("High error rate: %.1f%%", errorRate*100)
		}
	}
	
	result.Duration = time.Since(checkStart)
	return result
}

// checkServices checks service health
func (hc *HealthChecker) checkServices() map[string]CheckResult {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	
	results := make(map[string]CheckResult)
	
	for service, check := range hc.serviceChecks {
		checkStart := time.Now()
		
		result := CheckResult{
			Status:      check.Status,
			LastChecked: check.LastCheck,
			Details:     check.Details,
			Duration:    check.ResponseTime,
		}
		
		if check.Status == "unhealthy" || check.Status == "unknown" {
			result.Message = fmt.Sprintf("Service %s is %s", service, check.Status)
		}
		
		if check.ErrorCount > 0 {
			result.Details["error_count"] = check.ErrorCount
		}
		
		results[service] = result
	}
	
	return results
}

// HTTP handlers

// handleHealth returns overall health status
func (hc *HealthChecker) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := hc.overallHealth.Load().(*HealthStatus)
	
	// Set appropriate HTTP status code
	httpStatus := http.StatusOK
	switch status.Status {
	case "degraded":
		httpStatus = http.StatusOK // Still operational
	case "unhealthy":
		httpStatus = http.StatusServiceUnavailable
	case "unknown":
		httpStatus = http.StatusServiceUnavailable
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(status)
}

// handleLiveness returns liveness check (is the service running)
func (hc *HealthChecker) handleLiveness(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "alive",
		"timestamp": time.Now(),
		"uptime":    time.Since(startTime).Round(time.Second).String(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleReadiness returns readiness check (is the service ready to accept traffic)
func (hc *HealthChecker) handleReadiness(w http.ResponseWriter, r *http.Request) {
	status := hc.overallHealth.Load().(*HealthStatus)
	
	ready := status.Status == "healthy" || status.Status == "degraded"
	
	response := map[string]interface{}{
		"ready":     ready,
		"status":    status.Status,
		"timestamp": time.Now(),
	}
	
	httpStatus := http.StatusOK
	if !ready {
		httpStatus = http.StatusServiceUnavailable
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(response)
}

// handleExchangeHealth returns exchange health status
func (hc *HealthChecker) handleExchangeHealth(w http.ResponseWriter, r *http.Request) {
	results := hc.checkExchanges()
	
	response := map[string]interface{}{
		"timestamp": time.Now(),
		"exchanges": results,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleExchangeHealthDetail returns detailed health for a specific exchange
func (hc *HealthChecker) handleExchangeHealthDetail(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	exchange := vars["exchange"]
	
	hc.mu.RLock()
	check, exists := hc.exchangeChecks[exchange]
	hc.mu.RUnlock()
	
	if !exists {
		http.Error(w, "Exchange not found", http.StatusNotFound)
		return
	}
	
	response := map[string]interface{}{
		"exchange":          exchange,
		"connected":         check.Connected,
		"last_ping":         check.LastPing,
		"latency_ms":        check.LastPingLatency.Milliseconds(),
		"success_count":     check.SuccessCount,
		"error_count":       check.ErrorCount,
		"last_checked":      check.LastPing,
	}
	
	if check.LastError != nil {
		response["last_error"] = check.LastError.Error()
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleAccountHealth returns account health status
func (hc *HealthChecker) handleAccountHealth(w http.ResponseWriter, r *http.Request) {
	hc.mu.RLock()
	
	accounts := make([]map[string]interface{}, 0)
	for accountID, check := range hc.accountChecks {
		accounts = append(accounts, map[string]interface{}{
			"account_id":      accountID,
			"exchange":        check.Exchange,
			"active":          check.Active,
			"last_activity":   check.LastActivity,
			"orders_placed":   check.OrdersPlaced,
			"orders_failed":   check.OrdersFailed,
			"rate_limit_hits": check.RateLimitHits,
		})
	}
	
	hc.mu.RUnlock()
	
	response := map[string]interface{}{
		"timestamp": time.Now(),
		"accounts":  accounts,
		"count":     len(accounts),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleAccountHealthDetail returns detailed health for a specific account
func (hc *HealthChecker) handleAccountHealthDetail(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	accountID := vars["accountId"]
	
	hc.mu.RLock()
	check, exists := hc.accountChecks[accountID]
	hc.mu.RUnlock()
	
	if !exists {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}
	
	// Get detailed metrics if available
	var metrics map[string]interface{}
	if hc.metricsCollector != nil {
		metrics = hc.metricsCollector.GetAccountSummary(accountID)
	}
	
	response := map[string]interface{}{
		"account_id":      accountID,
		"exchange":        check.Exchange,
		"active":          check.Active,
		"last_activity":   check.LastActivity,
		"orders_placed":   check.OrdersPlaced,
		"orders_failed":   check.OrdersFailed,
		"rate_limit_hits": check.RateLimitHits,
		"balance":         check.Balance,
		"metrics":         metrics,
	}
	
	if check.LastError != nil {
		response["last_error"] = check.LastError.Error()
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleServiceHealth returns service health status
func (hc *HealthChecker) handleServiceHealth(w http.ResponseWriter, r *http.Request) {
	results := hc.checkServices()
	
	response := map[string]interface{}{
		"timestamp": time.Now(),
		"services":  results,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleServiceHealthDetail returns detailed health for a specific service
func (hc *HealthChecker) handleServiceHealthDetail(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	service := vars["service"]
	
	hc.mu.RLock()
	check, exists := hc.serviceChecks[service]
	hc.mu.RUnlock()
	
	if !exists {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}
	
	response := map[string]interface{}{
		"service":       service,
		"status":        check.Status,
		"last_check":    check.LastCheck,
		"response_time": check.ResponseTime.Milliseconds(),
		"error_count":   check.ErrorCount,
		"details":       check.Details,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleMetrics returns Prometheus-style metrics
func (hc *HealthChecker) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	
	// System metrics
	if hc.metricsCollector != nil {
		globalMetrics := hc.metricsCollector.GetGlobalMetrics()
		
		fmt.Fprintf(w, "# HELP mexoms_cpu_usage_percent CPU usage percentage\n")
		fmt.Fprintf(w, "# TYPE mexoms_cpu_usage_percent gauge\n")
		fmt.Fprintf(w, "mexoms_cpu_usage_percent %.2f\n", float64(globalMetrics.CPUUsage.Load())/100.0)
		
		fmt.Fprintf(w, "# HELP mexoms_memory_usage_bytes Memory usage in bytes\n")
		fmt.Fprintf(w, "# TYPE mexoms_memory_usage_bytes gauge\n")
		fmt.Fprintf(w, "mexoms_memory_usage_bytes %d\n", globalMetrics.MemoryUsage.Load())
		
		fmt.Fprintf(w, "# HELP mexoms_goroutines Number of goroutines\n")
		fmt.Fprintf(w, "# TYPE mexoms_goroutines gauge\n")
		fmt.Fprintf(w, "mexoms_goroutines %d\n", globalMetrics.GoroutineCount.Load())
		
		fmt.Fprintf(w, "# HELP mexoms_orders_per_second Orders processed per second\n")
		fmt.Fprintf(w, "# TYPE mexoms_orders_per_second gauge\n")
		fmt.Fprintf(w, "mexoms_orders_per_second %d\n", globalMetrics.TotalOrdersPerSec.Load())
	}
	
	// Health status
	status := hc.overallHealth.Load().(*HealthStatus)
	healthValue := 0
	switch status.Status {
	case "healthy":
		healthValue = 1
	case "degraded":
		healthValue = 2
	case "unhealthy":
		healthValue = 3
	}
	
	fmt.Fprintf(w, "# HELP mexoms_health_status Overall health status (1=healthy, 2=degraded, 3=unhealthy)\n")
	fmt.Fprintf(w, "# TYPE mexoms_health_status gauge\n")
	fmt.Fprintf(w, "mexoms_health_status %d\n", healthValue)
	
	// Uptime
	fmt.Fprintf(w, "# HELP mexoms_uptime_seconds Uptime in seconds\n")
	fmt.Fprintf(w, "# TYPE mexoms_uptime_seconds counter\n")
	fmt.Fprintf(w, "mexoms_uptime_seconds %.0f\n", time.Since(startTime).Seconds())
}

// Stop gracefully stops the health checker
func (hc *HealthChecker) Stop() error {
	hc.cancel()
	
	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := hc.server.Shutdown(ctx); err != nil {
		hc.logger.Error("Failed to shutdown health check server", zap.Error(err))
		return err
	}
	
	hc.wg.Wait()
	
	hc.logger.Info("Health checker stopped")
	return nil
}