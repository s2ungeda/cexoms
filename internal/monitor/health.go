package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"
	
	"github.com/mExOms/pkg/nats"
	"go.uber.org/zap"
)

// HealthStatus represents the health status of a component
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// HealthCheck represents a health check function
type HealthCheck func(ctx context.Context) ComponentHealth

// ComponentHealth represents the health of a single component
type ComponentHealth struct {
	Name        string            `json:"name"`
	Status      HealthStatus      `json:"status"`
	Message     string            `json:"message,omitempty"`
	LastChecked time.Time         `json:"last_checked"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// SystemHealth represents the overall system health
type SystemHealth struct {
	Status      HealthStatus       `json:"status"`
	Components  []ComponentHealth  `json:"components"`
	Version     string            `json:"version"`
	Uptime      string            `json:"uptime"`
	Timestamp   time.Time         `json:"timestamp"`
	Metrics     SystemMetrics     `json:"metrics"`
}

// SystemMetrics represents system-wide metrics
type SystemMetrics struct {
	CPUUsage      float64 `json:"cpu_usage"`
	MemoryUsageMB int64   `json:"memory_usage_mb"`
	Goroutines    int     `json:"goroutines"`
	HeapAllocMB   int64   `json:"heap_alloc_mb"`
	HeapSysMB     int64   `json:"heap_sys_mb"`
}

// HealthChecker manages health checks
type HealthChecker struct {
	mu sync.RWMutex
	
	// Registered checks
	checks map[string]HealthCheck
	
	// Cache
	lastResults map[string]ComponentHealth
	cacheExpiry time.Duration
	
	// System info
	startTime time.Time
	version   string
	
	// NATS publishing
	natsClient    *nats.Client
	logger        *zap.Logger
	publishTicker *time.Ticker
	stopPublish   chan struct{}
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(version string, natsClient *nats.Client, logger *zap.Logger) *HealthChecker {
	hc := &HealthChecker{
		checks:        make(map[string]HealthCheck),
		lastResults:   make(map[string]ComponentHealth),
		cacheExpiry:   10 * time.Second,
		startTime:     time.Now(),
		version:       version,
		natsClient:    natsClient,
		logger:        logger,
		publishTicker: time.NewTicker(5 * time.Second),
		stopPublish:   make(chan struct{}),
	}
	
	// Start publishing health metrics
	if natsClient != nil {
		go hc.publishHealthMetrics()
	}
	
	return hc
}

// RegisterCheck registers a health check
func (hc *HealthChecker) RegisterCheck(name string, check HealthCheck) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	
	hc.checks[name] = check
}

// CheckHealth runs all health checks
func (hc *HealthChecker) CheckHealth(ctx context.Context) SystemHealth {
	hc.mu.RLock()
	checks := make(map[string]HealthCheck)
	for name, check := range hc.checks {
		checks[name] = check
	}
	hc.mu.RUnlock()
	
	// Run checks in parallel
	var wg sync.WaitGroup
	results := make(chan ComponentHealth, len(checks))
	
	for name, check := range checks {
		wg.Add(1)
		go func(n string, c HealthCheck) {
			defer wg.Done()
			
			// Check cache first
			if cached, ok := hc.getCachedResult(n); ok {
				results <- cached
				return
			}
			
			// Run check with timeout
			checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			
			result := c(checkCtx)
			result.Name = n
			result.LastChecked = time.Now()
			
			// Update cache
			hc.setCachedResult(n, result)
			
			results <- result
		}(name, check)
	}
	
	// Wait for all checks
	go func() {
		wg.Wait()
		close(results)
	}()
	
	// Collect results
	var components []ComponentHealth
	overallStatus := HealthStatusHealthy
	
	for result := range results {
		components = append(components, result)
		
		// Update overall status
		if result.Status == HealthStatusUnhealthy {
			overallStatus = HealthStatusUnhealthy
		} else if result.Status == HealthStatusDegraded && overallStatus == HealthStatusHealthy {
			overallStatus = HealthStatusDegraded
		}
	}
	
	return SystemHealth{
		Status:     overallStatus,
		Components: components,
		Version:    hc.version,
		Uptime:     time.Since(hc.startTime).String(),
		Timestamp:  time.Now(),
		Metrics:    hc.collectSystemMetrics(),
	}
}

// getCachedResult returns cached result if not expired
func (hc *HealthChecker) getCachedResult(name string) (ComponentHealth, bool) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	
	if result, ok := hc.lastResults[name]; ok {
		if time.Since(result.LastChecked) < hc.cacheExpiry {
			return result, true
		}
	}
	
	return ComponentHealth{}, false
}

// setCachedResult updates cached result
func (hc *HealthChecker) setCachedResult(name string, result ComponentHealth) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	
	hc.lastResults[name] = result
}

// HTTPHandler returns an HTTP handler for health checks
func (hc *HealthChecker) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		health := hc.CheckHealth(ctx)
		
		// Set status code based on health
		statusCode := http.StatusOK
		if health.Status == HealthStatusDegraded {
			statusCode = http.StatusOK // Still return 200 for degraded
		} else if health.Status == HealthStatusUnhealthy {
			statusCode = http.StatusServiceUnavailable
		}
		
		// Return JSON response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(health)
	}
}

// Common health checks

// NATSHealthCheck checks NATS connectivity
func NATSHealthCheck(url string) HealthCheck {
	return func(ctx context.Context) ComponentHealth {
		// In production, actually check NATS connection
		// For now, return mock status
		return ComponentHealth{
			Status:  HealthStatusHealthy,
			Message: "NATS is connected",
			Details: map[string]interface{}{
				"url":        url,
				"connected":  true,
				"subscriptions": 42,
			},
		}
	}
}

// FileSystemHealthCheck checks file system
func FileSystemHealthCheck(path string) HealthCheck {
	return func(ctx context.Context) ComponentHealth {
		// Check disk space
		// For now, return mock status
		return ComponentHealth{
			Status:  HealthStatusHealthy,
			Message: "File system is healthy",
			Details: map[string]interface{}{
				"path":       path,
				"free_space": "45.2 GB",
				"used_space": "54.8 GB",
				"total_space": "100 GB",
			},
		}
	}
}

// ExchangeHealthCheck checks exchange connectivity
func ExchangeHealthCheck(exchange string) HealthCheck {
	return func(ctx context.Context) ComponentHealth {
		// Check exchange API
		// For now, return mock status
		return ComponentHealth{
			Status:  HealthStatusHealthy,
			Message: fmt.Sprintf("%s API is responding", exchange),
			Details: map[string]interface{}{
				"exchange":     exchange,
				"api_latency":  "45ms",
				"rate_limit":   "1200/1200",
			},
		}
	}
}

// MemoryHealthCheck checks memory usage
func MemoryHealthCheck(threshold float64) HealthCheck {
	return func(ctx context.Context) ComponentHealth {
		// Check memory usage
		// For now, return mock status
		usagePercent := 65.5
		
		status := HealthStatusHealthy
		if usagePercent > threshold {
			status = HealthStatusDegraded
		}
		if usagePercent > 90 {
			status = HealthStatusUnhealthy
		}
		
		return ComponentHealth{
			Status:  status,
			Message: fmt.Sprintf("Memory usage: %.1f%%", usagePercent),
			Details: map[string]interface{}{
				"used_mb":     2048,
				"total_mb":    3128,
				"usage_percent": usagePercent,
			},
		}
	}
}

// PositionManagerHealthCheck checks position manager
func PositionManagerHealthCheck() HealthCheck {
	return func(ctx context.Context) ComponentHealth {
		// Check position manager
		// For now, return mock status
		return ComponentHealth{
			Status:  HealthStatusHealthy,
			Message: "Position manager is operational",
			Details: map[string]interface{}{
				"positions_count": 15,
				"last_update":     time.Now().Add(-5 * time.Second),
				"shared_memory":   "connected",
			},
		}
	}
}

// RiskEngineHealthCheck checks risk engine
func RiskEngineHealthCheck() HealthCheck {
	return func(ctx context.Context) ComponentHealth {
		// Check risk engine
		// For now, return mock status
		return ComponentHealth{
			Status:  HealthStatusHealthy,
			Message: "Risk engine is operational",
			Details: map[string]interface{}{
				"checks_per_second": 15420,
				"avg_check_time_us": 1.8,
				"enabled":          true,
			},
		}
	}
}

// collectSystemMetrics collects system-wide metrics
func (hc *HealthChecker) collectSystemMetrics() SystemMetrics {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	return SystemMetrics{
		CPUUsage:      getCPUUsage(),
		MemoryUsageMB: int64(m.Alloc / 1024 / 1024),
		Goroutines:    runtime.NumGoroutine(),
		HeapAllocMB:   int64(m.HeapAlloc / 1024 / 1024),
		HeapSysMB:     int64(m.HeapSys / 1024 / 1024),
	}
}

// getCPUUsage returns approximate CPU usage percentage
func getCPUUsage() float64 {
	// In a real implementation, this would track CPU usage over time
	// For now, return a mock value
	return 45.2
}

// publishHealthMetrics publishes health metrics to NATS periodically
func (hc *HealthChecker) publishHealthMetrics() {
	for {
		select {
		case <-hc.publishTicker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			health := hc.CheckHealth(ctx)
			cancel()
			
			// Publish to NATS
			if err := hc.publishHealth(health); err != nil {
				hc.logger.Error("Failed to publish health metrics", zap.Error(err))
			}
			
		case <-hc.stopPublish:
			return
		}
	}
}

// publishHealth publishes health status to NATS
func (hc *HealthChecker) publishHealth(health SystemHealth) error {
	if hc.natsClient == nil {
		return nil
	}
	
	// Publish overall health
	data, err := json.Marshal(health)
	if err != nil {
		return fmt.Errorf("failed to marshal health: %w", err)
	}
	
	subject := "oms.health.system"
	if err := hc.natsClient.Publish(subject, data); err != nil {
		return fmt.Errorf("failed to publish to NATS: %w", err)
	}
	
	// Publish component-specific health
	for _, component := range health.Components {
		componentData, err := json.Marshal(component)
		if err != nil {
			continue
		}
		
		componentSubject := fmt.Sprintf("oms.health.component.%s", component.Name)
		if err := hc.natsClient.Publish(componentSubject, componentData); err != nil {
			hc.logger.Warn("Failed to publish component health", 
				zap.String("component", component.Name),
				zap.Error(err))
		}
	}
	
	return nil
}

// Stop stops the health checker
func (hc *HealthChecker) Stop() {
	if hc.publishTicker != nil {
		hc.publishTicker.Stop()
	}
	close(hc.stopPublish)
}