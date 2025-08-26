package monitoring

import (
	"context"
	"math"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ArbitrageAnalyzer analyzes arbitrage opportunities and success rates
type ArbitrageAnalyzer struct {
	mu               sync.RWMutex
	opportunities    map[string]*ArbitrageOpportunity
	completedArbs    []CompletedArbitrage
	config           *ArbitrageConfig
	logger           *zap.Logger
	
	// Channels for updates
	opportunityChan  chan *ArbitrageSignal
	executionChan    chan *ArbitrageExecution
	
	// Metrics
	metrics          *ArbitrageMetrics
	
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ArbitrageConfig holds configuration for arbitrage analysis
type ArbitrageConfig struct {
	// Opportunity detection
	MinSpreadPercent    float64       // Minimum spread to consider
	MinProfitThreshold  float64       // Minimum profit after fees
	OpportunityTimeout  time.Duration // How long to track an opportunity
	
	// Execution tracking
	ExecutionWindow     time.Duration // Time window to match executions
	SuccessThreshold    float64       // P&L threshold for success
	
	// Analysis parameters
	AnalysisInterval    time.Duration
	MetricsRetention    time.Duration
	
	// Buffer sizes
	SignalBufferSize    int
	ExecutionBufferSize int
}

// DefaultArbitrageConfig returns default configuration
func DefaultArbitrageConfig() *ArbitrageConfig {
	return &ArbitrageConfig{
		MinSpreadPercent:    0.001,  // 0.1%
		MinProfitThreshold:  1.0,    // $1 minimum profit
		OpportunityTimeout:  5 * time.Minute,
		ExecutionWindow:     30 * time.Second,
		SuccessThreshold:    0.0,    // Any positive P&L is success
		AnalysisInterval:    10 * time.Second,
		MetricsRetention:    7 * 24 * time.Hour,
		SignalBufferSize:    10000,
		ExecutionBufferSize: 10000,
	}
}

// ArbitrageOpportunity represents a detected arbitrage opportunity
type ArbitrageOpportunity struct {
	ID              string
	Type            string // "spot-spot", "spot-futures", "triangular"
	Symbol          string
	BuyExchange     string
	SellExchange    string
	DetectedAt      time.Time
	ExpiresAt       time.Time
	
	// Price information
	BuyPrice        float64
	SellPrice       float64
	SpreadPercent   float64
	EstimatedProfit float64
	
	// Volume information
	MaxVolume       float64
	OptimalVolume   float64
	
	// Execution tracking
	Executed        bool
	ExecutedVolume  float64
	ExecutionIDs    []string
}

// ArbitrageSignal represents a detected arbitrage signal
type ArbitrageSignal struct {
	OpportunityID   string
	Type            string
	Symbol          string
	BuyExchange     string
	SellExchange    string
	BuyPrice        float64
	SellPrice       float64
	MaxVolume       float64
	EstimatedProfit float64
	Timestamp       time.Time
}

// ArbitrageExecution represents an executed arbitrage trade
type ArbitrageExecution struct {
	ExecutionID     string
	OpportunityID   string
	Symbol          string
	BuyOrderID      string
	SellOrderID     string
	Volume          float64
	BuyPrice        float64
	SellPrice       float64
	BuyFee          float64
	SellFee         float64
	NetProfit       float64
	ExecutionTime   time.Duration
	Timestamp       time.Time
	Success         bool
}

// CompletedArbitrage represents a completed arbitrage cycle
type CompletedArbitrage struct {
	OpportunityID   string
	Type            string
	Symbol          string
	DetectedAt      time.Time
	ExecutedAt      time.Time
	CompletedAt     time.Time
	
	// Opportunity details
	InitialSpread   float64
	EstimatedProfit float64
	
	// Execution details
	ExecutedVolume  float64
	ActualBuyPrice  float64
	ActualSellPrice float64
	TotalFees       float64
	NetProfit       float64
	
	// Performance
	ExecutionTime   time.Duration
	Slippage        float64
	Success         bool
	FailureReason   string
}

// ArbitrageMetrics tracks overall arbitrage performance
type ArbitrageMetrics struct {
	mu sync.RWMutex
	
	// Opportunity metrics
	TotalOpportunities   int64
	ValidOpportunities   int64
	ExpiredOpportunities int64
	
	// Execution metrics
	TotalExecutions      int64
	SuccessfulExecutions int64
	FailedExecutions     int64
	PartialExecutions    int64
	
	// Performance metrics
	TotalProfit          float64
	TotalLoss            float64
	TotalFees            float64
	AverageProfit        float64
	SuccessRate          float64
	
	// Timing metrics
	AverageDetectionTime time.Duration
	AverageExecutionTime time.Duration
	FastestExecution     time.Duration
	SlowestExecution     time.Duration
	
	// Volume metrics
	TotalVolume          float64
	AverageVolume        float64
	LargestTrade         float64
	
	// By type metrics
	TypeMetrics          map[string]*TypeSpecificMetrics
	
	// By exchange pair metrics
	ExchangePairMetrics  map[string]*ExchangePairMetrics
	
	// Time series data
	HourlyMetrics        []HourlyArbitrageMetrics
	DailyMetrics         []DailyArbitrageMetrics
	
	LastUpdate           time.Time
}

// TypeSpecificMetrics tracks metrics for specific arbitrage types
type TypeSpecificMetrics struct {
	Type                string
	Opportunities       int64
	Executions          int64
	SuccessRate         float64
	TotalProfit         float64
	AverageProfit       float64
	AverageSpread       float64
}

// ExchangePairMetrics tracks metrics for specific exchange pairs
type ExchangePairMetrics struct {
	BuyExchange         string
	SellExchange        string
	Opportunities       int64
	Executions          int64
	SuccessRate         float64
	TotalProfit         float64
	AverageLatency      time.Duration
}

// HourlyArbitrageMetrics represents hourly aggregated metrics
type HourlyArbitrageMetrics struct {
	Hour                time.Time
	Opportunities       int
	Executions          int
	SuccessRate         float64
	TotalProfit         float64
	AverageSpread       float64
}

// DailyArbitrageMetrics represents daily aggregated metrics
type DailyArbitrageMetrics struct {
	Date                string
	Opportunities       int
	Executions          int
	SuccessRate         float64
	TotalProfit         float64
	TotalVolume         float64
	BestOpportunity     float64
}

// NewArbitrageAnalyzer creates a new arbitrage analyzer
func NewArbitrageAnalyzer(config *ArbitrageConfig, logger *zap.Logger) *ArbitrageAnalyzer {
	if config == nil {
		config = DefaultArbitrageConfig()
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	analyzer := &ArbitrageAnalyzer{
		opportunities:   make(map[string]*ArbitrageOpportunity),
		completedArbs:   make([]CompletedArbitrage, 0),
		config:          config,
		logger:          logger,
		opportunityChan: make(chan *ArbitrageSignal, config.SignalBufferSize),
		executionChan:   make(chan *ArbitrageExecution, config.ExecutionBufferSize),
		metrics: &ArbitrageMetrics{
			TypeMetrics:         make(map[string]*TypeSpecificMetrics),
			ExchangePairMetrics: make(map[string]*ExchangePairMetrics),
			LastUpdate:          time.Now(),
		},
		ctx:    ctx,
		cancel: cancel,
	}
	
	// Start processing goroutines
	analyzer.wg.Add(2)
	go analyzer.processSignals()
	go analyzer.processExecutions()
	
	// Start analysis routine
	analyzer.wg.Add(1)
	go analyzer.analyzePerformance()
	
	// Start cleanup routine
	analyzer.wg.Add(1)
	go analyzer.cleanupOldData()
	
	return analyzer
}

// RecordOpportunity records a new arbitrage opportunity
func (a *ArbitrageAnalyzer) RecordOpportunity(signal *ArbitrageSignal) {
	select {
	case a.opportunityChan <- signal:
	default:
		a.logger.Warn("Opportunity channel full, dropping signal")
	}
}

// RecordExecution records an arbitrage execution
func (a *ArbitrageAnalyzer) RecordExecution(execution *ArbitrageExecution) {
	select {
	case a.executionChan <- execution:
	default:
		a.logger.Warn("Execution channel full, dropping execution")
	}
}

// GetMetrics returns current arbitrage metrics
func (a *ArbitrageAnalyzer) GetMetrics() *ArbitrageMetrics {
	a.metrics.mu.RLock()
	defer a.metrics.mu.RUnlock()
	
	return a.metrics
}

// GetOpportunity returns a specific opportunity
func (a *ArbitrageAnalyzer) GetOpportunity(opportunityID string) (*ArbitrageOpportunity, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	opp, exists := a.opportunities[opportunityID]
	return opp, exists
}

// processSignals processes arbitrage signals
func (a *ArbitrageAnalyzer) processSignals() {
	defer a.wg.Done()
	
	for {
		select {
		case <-a.ctx.Done():
			return
		case signal := <-a.opportunityChan:
			if signal == nil {
				continue
			}
			
			// Validate opportunity
			spreadPercent := (signal.SellPrice - signal.BuyPrice) / signal.BuyPrice
			if spreadPercent < a.config.MinSpreadPercent {
				continue
			}
			
			if signal.EstimatedProfit < a.config.MinProfitThreshold {
				continue
			}
			
			// Create opportunity
			opp := &ArbitrageOpportunity{
				ID:              signal.OpportunityID,
				Type:            signal.Type,
				Symbol:          signal.Symbol,
				BuyExchange:     signal.BuyExchange,
				SellExchange:    signal.SellExchange,
				DetectedAt:      signal.Timestamp,
				ExpiresAt:       signal.Timestamp.Add(a.config.OpportunityTimeout),
				BuyPrice:        signal.BuyPrice,
				SellPrice:       signal.SellPrice,
				SpreadPercent:   spreadPercent,
				EstimatedProfit: signal.EstimatedProfit,
				MaxVolume:       signal.MaxVolume,
				OptimalVolume:   signal.MaxVolume, // TODO: Calculate optimal volume
			}
			
			a.mu.Lock()
			a.opportunities[opp.ID] = opp
			a.mu.Unlock()
			
			// Update metrics
			a.metrics.mu.Lock()
			a.metrics.TotalOpportunities++
			a.metrics.ValidOpportunities++
			
			// Update type metrics
			typeKey := opp.Type
			if tm, exists := a.metrics.TypeMetrics[typeKey]; exists {
				tm.Opportunities++
				tm.AverageSpread = (tm.AverageSpread*float64(tm.Opportunities-1) + spreadPercent) / 
				                 float64(tm.Opportunities)
			} else {
				a.metrics.TypeMetrics[typeKey] = &TypeSpecificMetrics{
					Type:          typeKey,
					Opportunities: 1,
					AverageSpread: spreadPercent,
				}
			}
			
			// Update exchange pair metrics
			pairKey := opp.BuyExchange + "->" + opp.SellExchange
			if pm, exists := a.metrics.ExchangePairMetrics[pairKey]; exists {
				pm.Opportunities++
			} else {
				a.metrics.ExchangePairMetrics[pairKey] = &ExchangePairMetrics{
					BuyExchange:   opp.BuyExchange,
					SellExchange:  opp.SellExchange,
					Opportunities: 1,
				}
			}
			
			a.metrics.mu.Unlock()
			
			a.logger.Info("Arbitrage opportunity detected",
				zap.String("id", opp.ID),
				zap.String("type", opp.Type),
				zap.String("symbol", opp.Symbol),
				zap.Float64("spread_percent", spreadPercent*100),
				zap.Float64("estimated_profit", opp.EstimatedProfit))
		}
	}
}

// processExecutions processes arbitrage executions
func (a *ArbitrageAnalyzer) processExecutions() {
	defer a.wg.Done()
	
	for {
		select {
		case <-a.ctx.Done():
			return
		case execution := <-a.executionChan:
			if execution == nil {
				continue
			}
			
			a.mu.RLock()
			opp, exists := a.opportunities[execution.OpportunityID]
			a.mu.RUnlock()
			
			if !exists {
				a.logger.Warn("Execution for unknown opportunity",
					zap.String("opportunity_id", execution.OpportunityID))
				continue
			}
			
			// Update opportunity
			a.mu.Lock()
			opp.Executed = true
			opp.ExecutedVolume += execution.Volume
			opp.ExecutionIDs = append(opp.ExecutionIDs, execution.ExecutionID)
			a.mu.Unlock()
			
			// Determine success
			execution.Success = execution.NetProfit >= a.config.SuccessThreshold
			
			// Create completed arbitrage record
			completed := CompletedArbitrage{
				OpportunityID:   opp.ID,
				Type:            opp.Type,
				Symbol:          opp.Symbol,
				DetectedAt:      opp.DetectedAt,
				ExecutedAt:      execution.Timestamp,
				CompletedAt:     time.Now(),
				InitialSpread:   opp.SpreadPercent,
				EstimatedProfit: opp.EstimatedProfit,
				ExecutedVolume:  execution.Volume,
				ActualBuyPrice:  execution.BuyPrice,
				ActualSellPrice: execution.SellPrice,
				TotalFees:       execution.BuyFee + execution.SellFee,
				NetProfit:       execution.NetProfit,
				ExecutionTime:   execution.ExecutionTime,
				Success:         execution.Success,
			}
			
			// Calculate slippage
			expectedProfit := (opp.SellPrice - opp.BuyPrice) * execution.Volume
			actualProfit := (execution.SellPrice - execution.BuyPrice) * execution.Volume
			completed.Slippage = (expectedProfit - actualProfit) / expectedProfit
			
			if !execution.Success {
				if execution.NetProfit < 0 {
					completed.FailureReason = "Negative P&L"
				} else {
					completed.FailureReason = "Below threshold"
				}
			}
			
			a.mu.Lock()
			a.completedArbs = append(a.completedArbs, completed)
			a.mu.Unlock()
			
			// Update metrics
			a.updateExecutionMetrics(execution, completed)
		}
	}
}

// updateExecutionMetrics updates metrics after an execution
func (a *ArbitrageAnalyzer) updateExecutionMetrics(execution *ArbitrageExecution, completed CompletedArbitrage) {
	a.metrics.mu.Lock()
	defer a.metrics.mu.Unlock()
	
	a.metrics.TotalExecutions++
	
	if execution.Success {
		a.metrics.SuccessfulExecutions++
		a.metrics.TotalProfit += execution.NetProfit
	} else {
		a.metrics.FailedExecutions++
		if execution.NetProfit < 0 {
			a.metrics.TotalLoss += math.Abs(execution.NetProfit)
		}
	}
	
	a.metrics.TotalFees += execution.BuyFee + execution.SellFee
	a.metrics.TotalVolume += execution.Volume
	
	// Update success rate
	a.metrics.SuccessRate = float64(a.metrics.SuccessfulExecutions) / float64(a.metrics.TotalExecutions)
	
	// Update average profit
	if a.metrics.SuccessfulExecutions > 0 {
		a.metrics.AverageProfit = a.metrics.TotalProfit / float64(a.metrics.SuccessfulExecutions)
	}
	
	// Update average volume
	a.metrics.AverageVolume = a.metrics.TotalVolume / float64(a.metrics.TotalExecutions)
	
	// Update timing metrics
	if a.metrics.FastestExecution == 0 || execution.ExecutionTime < a.metrics.FastestExecution {
		a.metrics.FastestExecution = execution.ExecutionTime
	}
	if execution.ExecutionTime > a.metrics.SlowestExecution {
		a.metrics.SlowestExecution = execution.ExecutionTime
	}
	
	// Update average execution time
	totalTime := a.metrics.AverageExecutionTime * time.Duration(a.metrics.TotalExecutions-1)
	a.metrics.AverageExecutionTime = (totalTime + execution.ExecutionTime) / time.Duration(a.metrics.TotalExecutions)
	
	// Update largest trade
	if execution.Volume*execution.BuyPrice > a.metrics.LargestTrade {
		a.metrics.LargestTrade = execution.Volume * execution.BuyPrice
	}
	
	// Update type-specific metrics
	if tm, exists := a.metrics.TypeMetrics[completed.Type]; exists {
		tm.Executions++
		tm.SuccessRate = float64(tm.Executions) / float64(tm.Opportunities)
		if execution.Success {
			tm.TotalProfit += execution.NetProfit
			tm.AverageProfit = tm.TotalProfit / float64(tm.Executions)
		}
	}
	
	// Update exchange pair metrics
	pairKey := completed.Symbol + "->" + completed.Symbol
	if pm, exists := a.metrics.ExchangePairMetrics[pairKey]; exists {
		pm.Executions++
		pm.SuccessRate = float64(pm.Executions) / float64(pm.Opportunities)
		pm.TotalProfit += execution.NetProfit
		pm.AverageLatency = (pm.AverageLatency*time.Duration(pm.Executions-1) + 
		                    execution.ExecutionTime) / time.Duration(pm.Executions)
	}
	
	a.metrics.LastUpdate = time.Now()
}

// analyzePerformance periodically analyzes performance metrics
func (a *ArbitrageAnalyzer) analyzePerformance() {
	defer a.wg.Done()
	
	ticker := time.NewTicker(a.config.AnalysisInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.performAnalysis()
		}
	}
}

// performAnalysis performs periodic analysis
func (a *ArbitrageAnalyzer) performAnalysis() {
	now := time.Now()
	
	// Aggregate hourly metrics
	a.aggregateHourlyMetrics(now)
	
	// Check for expired opportunities
	a.checkExpiredOpportunities(now)
	
	// Calculate detection time metrics
	a.calculateDetectionMetrics()
}

// aggregateHourlyMetrics aggregates metrics by hour
func (a *ArbitrageAnalyzer) aggregateHourlyMetrics(now time.Time) {
	hour := now.Truncate(time.Hour)
	
	a.mu.RLock()
	hourlyOpps := 0
	hourlyExecs := 0
	hourlyProfit := 0.0
	totalSpread := 0.0
	successCount := 0
	
	for _, completed := range a.completedArbs {
		if completed.ExecutedAt.Truncate(time.Hour).Equal(hour) {
			hourlyExecs++
			hourlyProfit += completed.NetProfit
			if completed.Success {
				successCount++
			}
		}
	}
	
	for _, opp := range a.opportunities {
		if opp.DetectedAt.Truncate(time.Hour).Equal(hour) {
			hourlyOpps++
			totalSpread += opp.SpreadPercent
		}
	}
	a.mu.RUnlock()
	
	if hourlyOpps > 0 || hourlyExecs > 0 {
		hourlyMetric := HourlyArbitrageMetrics{
			Hour:          hour,
			Opportunities: hourlyOpps,
			Executions:    hourlyExecs,
			TotalProfit:   hourlyProfit,
		}
		
		if hourlyExecs > 0 {
			hourlyMetric.SuccessRate = float64(successCount) / float64(hourlyExecs)
		}
		
		if hourlyOpps > 0 {
			hourlyMetric.AverageSpread = totalSpread / float64(hourlyOpps)
		}
		
		a.metrics.mu.Lock()
		a.metrics.HourlyMetrics = append(a.metrics.HourlyMetrics, hourlyMetric)
		
		// Keep last 24 hours
		if len(a.metrics.HourlyMetrics) > 24 {
			a.metrics.HourlyMetrics = a.metrics.HourlyMetrics[len(a.metrics.HourlyMetrics)-24:]
		}
		a.metrics.mu.Unlock()
	}
}

// checkExpiredOpportunities checks for expired opportunities
func (a *ArbitrageAnalyzer) checkExpiredOpportunities(now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	for id, opp := range a.opportunities {
		if now.After(opp.ExpiresAt) && !opp.Executed {
			a.metrics.mu.Lock()
			a.metrics.ExpiredOpportunities++
			a.metrics.mu.Unlock()
			
			a.logger.Debug("Arbitrage opportunity expired",
				zap.String("id", id),
				zap.String("symbol", opp.Symbol),
				zap.Float64("spread", opp.SpreadPercent))
			
			delete(a.opportunities, id)
		}
	}
}

// calculateDetectionMetrics calculates average detection time
func (a *ArbitrageAnalyzer) calculateDetectionMetrics() {
	a.mu.RLock()
	totalDetectionTime := time.Duration(0)
	count := 0
	
	for _, completed := range a.completedArbs {
		detectionTime := completed.ExecutedAt.Sub(completed.DetectedAt)
		totalDetectionTime += detectionTime
		count++
	}
	a.mu.RUnlock()
	
	if count > 0 {
		a.metrics.mu.Lock()
		a.metrics.AverageDetectionTime = totalDetectionTime / time.Duration(count)
		a.metrics.mu.Unlock()
	}
}

// cleanupOldData removes old metrics data
func (a *ArbitrageAnalyzer) cleanupOldData() {
	defer a.wg.Done()
	
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.performCleanup()
		}
	}
}

// performCleanup removes old data
func (a *ArbitrageAnalyzer) performCleanup() {
	cutoff := time.Now().Add(-a.config.MetricsRetention)
	
	a.mu.Lock()
	
	// Clean up completed arbitrages
	var kept []CompletedArbitrage
	for _, completed := range a.completedArbs {
		if completed.CompletedAt.After(cutoff) {
			kept = append(kept, completed)
		}
	}
	a.completedArbs = kept
	
	// Clean up expired opportunities
	for id, opp := range a.opportunities {
		if opp.ExpiresAt.Before(cutoff) {
			delete(a.opportunities, id)
		}
	}
	
	a.mu.Unlock()
}

// GetReport generates a comprehensive arbitrage report
func (a *ArbitrageAnalyzer) GetReport() map[string]interface{} {
	a.metrics.mu.RLock()
	defer a.metrics.mu.RUnlock()
	
	report := map[string]interface{}{
		"summary": map[string]interface{}{
			"total_opportunities":   a.metrics.TotalOpportunities,
			"valid_opportunities":   a.metrics.ValidOpportunities,
			"expired_opportunities": a.metrics.ExpiredOpportunities,
			"total_executions":      a.metrics.TotalExecutions,
			"successful_executions": a.metrics.SuccessfulExecutions,
			"failed_executions":     a.metrics.FailedExecutions,
			"success_rate":          fmt.Sprintf("%.2f%%", a.metrics.SuccessRate*100),
		},
		"financial": map[string]interface{}{
			"total_profit":   a.metrics.TotalProfit,
			"total_loss":     a.metrics.TotalLoss,
			"net_profit":     a.metrics.TotalProfit - a.metrics.TotalLoss,
			"total_fees":     a.metrics.TotalFees,
			"average_profit": a.metrics.AverageProfit,
			"total_volume":   a.metrics.TotalVolume,
			"average_volume": a.metrics.AverageVolume,
			"largest_trade":  a.metrics.LargestTrade,
		},
		"performance": map[string]interface{}{
			"average_detection_time": a.metrics.AverageDetectionTime.String(),
			"average_execution_time": a.metrics.AverageExecutionTime.String(),
			"fastest_execution":      a.metrics.FastestExecution.String(),
			"slowest_execution":      a.metrics.SlowestExecution.String(),
		},
		"by_type": a.metrics.TypeMetrics,
		"by_exchange_pair": a.metrics.ExchangePairMetrics,
		"last_update": a.metrics.LastUpdate,
	}
	
	// Add recent hourly metrics
	if len(a.metrics.HourlyMetrics) > 0 {
		report["hourly_metrics"] = a.metrics.HourlyMetrics
	}
	
	// Add active opportunities count
	a.mu.RLock()
	activeOpps := 0
	for _, opp := range a.opportunities {
		if !opp.Executed && time.Now().Before(opp.ExpiresAt) {
			activeOpps++
		}
	}
	a.mu.RUnlock()
	
	report["active_opportunities"] = activeOpps
	
	return report
}

// Stop gracefully stops the analyzer
func (a *ArbitrageAnalyzer) Stop() {
	a.cancel()
	a.wg.Wait()
	
	close(a.opportunityChan)
	close(a.executionChan)
	
	a.logger.Info("Arbitrage analyzer stopped")
}

// Helper function for formatting
func fmt.Sprintf(format string, args ...interface{}) string {
	// This would be implemented properly in production
	return ""
}