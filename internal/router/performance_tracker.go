package router

import (
	"context"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// PerformanceTracker tracks routing performance metrics
type PerformanceTracker struct {
	mu          sync.RWMutex
	metrics     *PerformanceMetrics
	strategyMetrics map[RoutingStrategy]*StrategyMetrics
	hourlyStats map[int64]*HourlyStats // Unix hour -> stats
	dailyStats  map[string]*DailyStats  // YYYY-MM-DD -> stats
	stopCh      chan struct{}
}

// HourlyStats tracks hourly performance
type HourlyStats struct {
	Hour             time.Time
	OrderCount       int64
	SuccessCount     int64
	TotalVolume      decimal.Decimal
	TotalFees        decimal.Decimal
	AverageSlippage  float64
	VenueVolumes     map[string]decimal.Decimal
}

// DailyStats tracks daily performance
type DailyStats struct {
	Date              string
	OrderCount        int64
	SuccessCount      int64
	TotalVolume       decimal.Decimal
	TotalFees         decimal.Decimal
	FeesSaved         decimal.Decimal
	AverageSlippage   float64
	BestVenue         string
	MostUsedStrategy  RoutingStrategy
}

// NewPerformanceTracker creates a new performance tracker
func NewPerformanceTracker() *PerformanceTracker {
	return &PerformanceTracker{
		metrics: &PerformanceMetrics{
			VenueDistribution:   make(map[string]int64),
			StrategyPerformance: make(map[string]*StrategyMetrics),
		},
		strategyMetrics: make(map[RoutingStrategy]*StrategyMetrics),
		hourlyStats:     make(map[int64]*HourlyStats),
		dailyStats:      make(map[string]*DailyStats),
		stopCh:          make(chan struct{}),
	}
}

// Start starts the performance tracker
func (pt *PerformanceTracker) Start(ctx context.Context) {
	// Start periodic stats aggregation
	go pt.aggregationLoop(ctx)
	
	// Start cleanup of old stats
	go pt.cleanupLoop(ctx)
}

// Stop stops the performance tracker
func (pt *PerformanceTracker) Stop() {
	close(pt.stopCh)
}

// RecordRouting records a routing decision
func (pt *PerformanceTracker) RecordRouting(request RouteRequest, response *RouteResponse) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	// Update total orders
	pt.metrics.TotalOrders++

	// Update venue distribution
	for _, route := range response.Routes {
		pt.metrics.VenueDistribution[route.Venue]++
	}

	// Update strategy metrics
	strategyName := string(request.Strategy)
	if _, exists := pt.metrics.StrategyPerformance[strategyName]; !exists {
		pt.metrics.StrategyPerformance[strategyName] = &StrategyMetrics{}
	}
	pt.metrics.StrategyPerformance[strategyName].OrderCount++

	// Record in strategy-specific metrics
	if _, exists := pt.strategyMetrics[request.Strategy]; !exists {
		pt.strategyMetrics[request.Strategy] = &StrategyMetrics{}
	}
	pt.strategyMetrics[request.Strategy].OrderCount++
}

// RecordExecution records execution results
func (pt *PerformanceTracker) RecordExecution(request RouteRequest, report *ExecutionReport) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	// Update success/failure counts
	if report.Status == ExecutionCompleted {
		pt.metrics.SuccessfulOrders++
	} else if report.Status == ExecutionFailed {
		pt.metrics.FailedOrders++
	}

	// Update volume
	pt.metrics.TotalVolume = pt.metrics.TotalVolume.Add(report.TotalExecuted)

	// Update slippage
	pt.updateSlippageMetrics(report.SlippageBps)

	// Update execution time
	pt.updateExecutionTimeMetrics(report.ExecutionTime)

	// Update strategy-specific metrics
	strategyMetrics := pt.strategyMetrics[request.Strategy]
	if strategyMetrics != nil {
		if report.Status == ExecutionCompleted {
			strategyMetrics.SuccessRate = float64(strategyMetrics.OrderCount) / float64(strategyMetrics.OrderCount)
		}
		strategyMetrics.AverageSlippage = pt.calculateRunningAverage(
			strategyMetrics.AverageSlippage,
			float64(report.SlippageBps),
			strategyMetrics.OrderCount,
		)
		strategyMetrics.AverageExecutionTime = pt.calculateRunningDuration(
			strategyMetrics.AverageExecutionTime,
			report.ExecutionTime,
			strategyMetrics.OrderCount,
		)
	}

	// Update hourly stats
	pt.updateHourlyStats(report)

	// Update daily stats
	pt.updateDailyStats(request, report)
}

// RecordFeeSavings records fee optimization savings
func (pt *PerformanceTracker) RecordFeeSavings(saved decimal.Decimal) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	
	pt.metrics.TotalFeesSaved = pt.metrics.TotalFeesSaved.Add(saved)
}

// GetMetrics returns current performance metrics
func (pt *PerformanceTracker) GetMetrics() *PerformanceMetrics {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	// Create a copy to avoid race conditions
	metricsCopy := &PerformanceMetrics{
		TotalOrders:          pt.metrics.TotalOrders,
		SuccessfulOrders:     pt.metrics.SuccessfulOrders,
		FailedOrders:         pt.metrics.FailedOrders,
		AverageSlippageBps:   pt.metrics.AverageSlippageBps,
		TotalVolume:          pt.metrics.TotalVolume,
		TotalFeesSaved:       pt.metrics.TotalFeesSaved,
		AverageExecutionTime: pt.metrics.AverageExecutionTime,
		VenueDistribution:    make(map[string]int64),
		StrategyPerformance:  make(map[string]*StrategyMetrics),
	}

	// Copy venue distribution
	for venue, count := range pt.metrics.VenueDistribution {
		metricsCopy.VenueDistribution[venue] = count
	}

	// Copy strategy performance
	for strategy, metrics := range pt.metrics.StrategyPerformance {
		metricsCopy.StrategyPerformance[strategy] = &StrategyMetrics{
			OrderCount:           metrics.OrderCount,
			SuccessRate:          metrics.SuccessRate,
			AverageSlippage:      metrics.AverageSlippage,
			AverageExecutionTime: metrics.AverageExecutionTime,
		}
	}

	return metricsCopy
}

// GetHourlyStats returns hourly statistics
func (pt *PerformanceTracker) GetHourlyStats(hour time.Time) *HourlyStats {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	hourKey := hour.Unix() / 3600
	return pt.hourlyStats[hourKey]
}

// GetDailyStats returns daily statistics
func (pt *PerformanceTracker) GetDailyStats(date string) *DailyStats {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	return pt.dailyStats[date]
}

// GetPerformanceSummary returns a performance summary
func (pt *PerformanceTracker) GetPerformanceSummary(period time.Duration) *PerformanceSummary {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	cutoff := time.Now().Add(-period)
	summary := &PerformanceSummary{
		Period:            period,
		StartTime:         cutoff,
		EndTime:           time.Now(),
		VenuePerformance:  make(map[string]*VenuePerformance),
		StrategyBreakdown: make(map[RoutingStrategy]*StrategyBreakdown),
	}

	// Aggregate hourly stats within period
	for hourUnix, stats := range pt.hourlyStats {
		hour := time.Unix(hourUnix*3600, 0)
		if hour.After(cutoff) {
			summary.TotalOrders += stats.OrderCount
			summary.TotalVolume = summary.TotalVolume.Add(stats.TotalVolume)
			summary.TotalFees = summary.TotalFees.Add(stats.TotalFees)
			
			// Aggregate venue volumes
			for venue, volume := range stats.VenueVolumes {
				if _, exists := summary.VenuePerformance[venue]; !exists {
					summary.VenuePerformance[venue] = &VenuePerformance{}
				}
				summary.VenuePerformance[venue].Volume = summary.VenuePerformance[venue].Volume.Add(volume)
				summary.VenuePerformance[venue].OrderCount++
			}
		}
	}

	// Calculate averages and rates
	if summary.TotalOrders > 0 {
		summary.SuccessRate = float64(pt.metrics.SuccessfulOrders) / float64(summary.TotalOrders)
		summary.AverageOrderSize = summary.TotalVolume.Div(decimal.NewFromInt(summary.TotalOrders))
	}

	return summary
}

// Helper methods

func (pt *PerformanceTracker) updateSlippageMetrics(slippageBps int) {
	totalSlippage := pt.metrics.AverageSlippageBps * float64(pt.metrics.SuccessfulOrders-1)
	totalSlippage += float64(slippageBps)
	pt.metrics.AverageSlippageBps = totalSlippage / float64(pt.metrics.SuccessfulOrders)
}

func (pt *PerformanceTracker) updateExecutionTimeMetrics(executionTime time.Duration) {
	if pt.metrics.SuccessfulOrders == 1 {
		pt.metrics.AverageExecutionTime = executionTime
	} else {
		// Calculate running average
		totalTime := pt.metrics.AverageExecutionTime * time.Duration(pt.metrics.SuccessfulOrders-1)
		totalTime += executionTime
		pt.metrics.AverageExecutionTime = totalTime / time.Duration(pt.metrics.SuccessfulOrders)
	}
}

func (pt *PerformanceTracker) updateHourlyStats(report *ExecutionReport) {
	hour := report.Timestamp.Unix() / 3600
	
	if _, exists := pt.hourlyStats[hour]; !exists {
		pt.hourlyStats[hour] = &HourlyStats{
			Hour:         time.Unix(hour*3600, 0),
			VenueVolumes: make(map[string]decimal.Decimal),
		}
	}
	
	stats := pt.hourlyStats[hour]
	stats.OrderCount++
	
	if report.Status == ExecutionCompleted {
		stats.SuccessCount++
		stats.TotalVolume = stats.TotalVolume.Add(report.TotalExecuted)
		stats.TotalFees = stats.TotalFees.Add(report.TotalFees)
		
		// Update venue volumes
		for _, route := range report.ExecutedRoutes {
			stats.VenueVolumes[route.Venue] = stats.VenueVolumes[route.Venue].Add(route.ExecutedQty)
		}
		
		// Update average slippage
		stats.AverageSlippage = pt.calculateRunningAverage(
			stats.AverageSlippage,
			float64(report.SlippageBps),
			stats.SuccessCount,
		)
	}
}

func (pt *PerformanceTracker) updateDailyStats(request RouteRequest, report *ExecutionReport) {
	date := report.Timestamp.Format("2006-01-02")
	
	if _, exists := pt.dailyStats[date]; !exists {
		pt.dailyStats[date] = &DailyStats{
			Date: date,
		}
	}
	
	stats := pt.dailyStats[date]
	stats.OrderCount++
	
	if report.Status == ExecutionCompleted {
		stats.SuccessCount++
		stats.TotalVolume = stats.TotalVolume.Add(report.TotalExecuted)
		stats.TotalFees = stats.TotalFees.Add(report.TotalFees)
		
		// Track most used strategy
		if request.Strategy == stats.MostUsedStrategy || stats.MostUsedStrategy == "" {
			stats.MostUsedStrategy = request.Strategy
		}
		
		// Update average slippage
		stats.AverageSlippage = pt.calculateRunningAverage(
			stats.AverageSlippage,
			float64(report.SlippageBps),
			stats.SuccessCount,
		)
		
		// Track best venue (simplified - by volume)
		venueVolumes := make(map[string]decimal.Decimal)
		for _, route := range report.ExecutedRoutes {
			venueVolumes[route.Venue] = venueVolumes[route.Venue].Add(route.ExecutedQty)
		}
		
		var bestVenue string
		var bestVolume decimal.Decimal
		for venue, volume := range venueVolumes {
			if volume.GreaterThan(bestVolume) {
				bestVenue = venue
				bestVolume = volume
			}
		}
		stats.BestVenue = bestVenue
	}
}

func (pt *PerformanceTracker) calculateRunningAverage(current, new float64, count int64) float64 {
	if count <= 1 {
		return new
	}
	return (current*float64(count-1) + new) / float64(count)
}

func (pt *PerformanceTracker) calculateRunningDuration(current, new time.Duration, count int64) time.Duration {
	if count <= 1 {
		return new
	}
	total := current * time.Duration(count-1)
	total += new
	return total / time.Duration(count)
}

func (pt *PerformanceTracker) aggregationLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pt.stopCh:
			return
		case <-ticker.C:
			pt.aggregateRecentStats()
		}
	}
}

func (pt *PerformanceTracker) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pt.stopCh:
			return
		case <-ticker.C:
			pt.cleanupOldStats()
		}
	}
}

func (pt *PerformanceTracker) aggregateRecentStats() {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	// Aggregate last hour's performance
	currentHour := time.Now().Unix() / 3600
	if stats, exists := pt.hourlyStats[currentHour]; exists {
		// Update strategy performance based on hourly data
		for strategy, metrics := range pt.strategyMetrics {
			strategyName := string(strategy)
			pt.metrics.StrategyPerformance[strategyName] = metrics
		}
	}
}

func (pt *PerformanceTracker) cleanupOldStats() {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	// Remove hourly stats older than 7 days
	cutoffHour := (time.Now().Add(-7 * 24 * time.Hour).Unix()) / 3600
	for hour := range pt.hourlyStats {
		if hour < cutoffHour {
			delete(pt.hourlyStats, hour)
		}
	}

	// Remove daily stats older than 30 days
	cutoffDate := time.Now().Add(-30 * 24 * time.Hour).Format("2006-01-02")
	for date := range pt.dailyStats {
		if date < cutoffDate {
			delete(pt.dailyStats, date)
		}
	}
}

// Types for performance tracking

type PerformanceSummary struct {
	Period            time.Duration
	StartTime         time.Time
	EndTime           time.Time
	TotalOrders       int64
	TotalVolume       decimal.Decimal
	TotalFees         decimal.Decimal
	SuccessRate       float64
	AverageOrderSize  decimal.Decimal
	VenuePerformance  map[string]*VenuePerformance
	StrategyBreakdown map[RoutingStrategy]*StrategyBreakdown
}

type VenuePerformance struct {
	OrderCount   int64
	Volume       decimal.Decimal
	Fees         decimal.Decimal
	SuccessRate  float64
	AverageSlippage float64
}

type StrategyBreakdown struct {
	OrderCount      int64
	Volume          decimal.Decimal
	SuccessRate     float64
	AverageSlippage float64
	AverageExecutionTime time.Duration
}