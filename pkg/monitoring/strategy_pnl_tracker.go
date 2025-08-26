package monitoring

import (
	"context"
	"math"
	"sync"
	"time"

	"go.uber.org/zap"
)

// StrategyPnLTracker tracks P&L and performance metrics for strategies
type StrategyPnLTracker struct {
	mu               sync.RWMutex
	strategies       map[string]*StrategyMetrics
	accountStrategies map[string]string // accountID -> strategyID mapping
	config           *PnLConfig
	logger           *zap.Logger
	
	// Channels for updates
	tradeChan    chan *StrategyTrade
	positionChan chan *StrategyPosition
	
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// PnLConfig holds configuration for P&L tracking
type PnLConfig struct {
	// Update intervals
	CalculationInterval time.Duration
	SnapshotInterval    time.Duration
	
	// Performance calculation
	RiskFreeRate        float64 // Annual risk-free rate for Sharpe ratio
	DrawdownWindow      time.Duration
	PerformanceWindow   time.Duration
	
	// Alert thresholds
	MaxDrawdownAlert    float64 // Percentage
	DailyLossAlert      float64 // Absolute value
	ConsecutiveLossDays int
	
	// Buffer sizes
	TradeBufferSize    int
	PositionBufferSize int
}

// DefaultPnLConfig returns default configuration
func DefaultPnLConfig() *PnLConfig {
	return &PnLConfig{
		CalculationInterval: 1 * time.Second,
		SnapshotInterval:    1 * time.Minute,
		RiskFreeRate:        0.02, // 2% annual
		DrawdownWindow:      30 * 24 * time.Hour,
		PerformanceWindow:   24 * time.Hour,
		MaxDrawdownAlert:    0.10, // 10%
		DailyLossAlert:      10000, // $10,000
		ConsecutiveLossDays: 3,
		TradeBufferSize:     10000,
		PositionBufferSize:  1000,
	}
}

// StrategyMetrics tracks metrics for a specific strategy
type StrategyMetrics struct {
	StrategyID   string
	StrategyType string
	StartTime    time.Time
	
	mu sync.RWMutex
	
	// P&L metrics
	RealizedPnL      float64
	UnrealizedPnL    float64
	TotalPnL         float64
	DailyPnL         map[string]float64 // date -> P&L
	
	// Trade statistics
	TotalTrades      int
	WinningTrades    int
	LosingTrades     int
	TotalVolume      float64
	TotalFees        float64
	
	// Performance metrics
	WinRate          float64
	AvgWin           float64
	AvgLoss          float64
	ProfitFactor     float64
	SharpeRatio      float64
	MaxDrawdown      float64
	MaxDrawdownDate  time.Time
	CurrentDrawdown  float64
	
	// Position tracking
	ActivePositions  map[string]*StrategyPosition
	MaxPositions     int
	MaxExposure      float64
	CurrentExposure  float64
	
	// Account association
	Accounts         map[string]bool
	
	// Time series data
	PnLHistory       []PnLSnapshot
	DrawdownHistory  []DrawdownSnapshot
	
	// Risk metrics
	ConsecutiveLosses int
	ConsecutiveWins   int
	LargestWin       float64
	LargestLoss      float64
	
	// Strategy-specific metrics
	CustomMetrics    map[string]float64
	
	LastUpdate       time.Time
}

// StrategyTrade represents a trade executed by a strategy
type StrategyTrade struct {
	StrategyID string
	AccountID  string
	TradeID    string
	Symbol     string
	Side       string
	Quantity   float64
	Price      float64
	Fee        float64
	PnL        float64
	Timestamp  time.Time
}

// StrategyPosition represents an open position
type StrategyPosition struct {
	StrategyID    string
	AccountID     string
	Symbol        string
	Side          string
	Quantity      float64
	EntryPrice    float64
	CurrentPrice  float64
	UnrealizedPnL float64
	Exposure      float64
	Timestamp     time.Time
}

// PnLSnapshot represents a point-in-time P&L snapshot
type PnLSnapshot struct {
	Timestamp     time.Time
	RealizedPnL   float64
	UnrealizedPnL float64
	TotalPnL      float64
	Positions     int
	Exposure      float64
}

// DrawdownSnapshot represents drawdown at a point in time
type DrawdownSnapshot struct {
	Timestamp       time.Time
	DrawdownPercent float64
	DrawdownValue   float64
	PeakValue       float64
}

// NewStrategyPnLTracker creates a new P&L tracker
func NewStrategyPnLTracker(config *PnLConfig, logger *zap.Logger) *StrategyPnLTracker {
	if config == nil {
		config = DefaultPnLConfig()
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	tracker := &StrategyPnLTracker{
		strategies:        make(map[string]*StrategyMetrics),
		accountStrategies: make(map[string]string),
		config:            config,
		logger:            logger,
		tradeChan:         make(chan *StrategyTrade, config.TradeBufferSize),
		positionChan:      make(chan *StrategyPosition, config.PositionBufferSize),
		ctx:               ctx,
		cancel:            cancel,
	}
	
	// Start processing goroutines
	tracker.wg.Add(2)
	go tracker.processTrades()
	go tracker.processPositions()
	
	// Start calculation routine
	tracker.wg.Add(1)
	go tracker.calculateMetrics()
	
	// Start snapshot routine
	tracker.wg.Add(1)
	go tracker.takeSnapshots()
	
	return tracker
}

// RegisterStrategy registers a new strategy
func (t *StrategyPnLTracker) RegisterStrategy(strategyID, strategyType string, accounts []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	if _, exists := t.strategies[strategyID]; exists {
		return
	}
	
	sm := &StrategyMetrics{
		StrategyID:      strategyID,
		StrategyType:    strategyType,
		StartTime:       time.Now(),
		DailyPnL:        make(map[string]float64),
		ActivePositions: make(map[string]*StrategyPosition),
		Accounts:        make(map[string]bool),
		CustomMetrics:   make(map[string]float64),
		LastUpdate:      time.Now(),
	}
	
	// Associate accounts
	for _, accountID := range accounts {
		sm.Accounts[accountID] = true
		t.accountStrategies[accountID] = strategyID
	}
	
	t.strategies[strategyID] = sm
	
	t.logger.Info("Registered strategy",
		zap.String("strategy_id", strategyID),
		zap.String("type", strategyType),
		zap.Int("accounts", len(accounts)))
}

// RecordTrade records a trade for a strategy
func (t *StrategyPnLTracker) RecordTrade(trade *StrategyTrade) {
	select {
	case t.tradeChan <- trade:
	default:
		t.logger.Warn("Trade channel full, dropping trade",
			zap.String("strategy_id", trade.StrategyID),
			zap.String("trade_id", trade.TradeID))
	}
}

// UpdatePosition updates a position for a strategy
func (t *StrategyPnLTracker) UpdatePosition(position *StrategyPosition) {
	select {
	case t.positionChan <- position:
	default:
		t.logger.Warn("Position channel full, dropping update",
			zap.String("strategy_id", position.StrategyID),
			zap.String("symbol", position.Symbol))
	}
}

// GetStrategyMetrics returns metrics for a specific strategy
func (t *StrategyPnLTracker) GetStrategyMetrics(strategyID string) (*StrategyMetrics, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	metrics, exists := t.strategies[strategyID]
	if !exists {
		return nil, false
	}
	
	// Return a copy to avoid race conditions
	metrics.mu.RLock()
	defer metrics.mu.RUnlock()
	
	return metrics, true
}

// GetAllStrategiesSnapshot returns a snapshot of all strategies
func (t *StrategyPnLTracker) GetAllStrategiesSnapshot() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	snapshot := make(map[string]interface{})
	
	for strategyID, metrics := range t.strategies {
		metrics.mu.RLock()
		
		strategySnapshot := map[string]interface{}{
			"strategy_id":       strategyID,
			"strategy_type":     metrics.StrategyType,
			"start_time":        metrics.StartTime,
			"total_pnl":         metrics.TotalPnL,
			"realized_pnl":      metrics.RealizedPnL,
			"unrealized_pnl":    metrics.UnrealizedPnL,
			"total_trades":      metrics.TotalTrades,
			"winning_trades":    metrics.WinningTrades,
			"losing_trades":     metrics.LosingTrades,
			"win_rate":          metrics.WinRate,
			"profit_factor":     metrics.ProfitFactor,
			"sharpe_ratio":      metrics.SharpeRatio,
			"max_drawdown":      metrics.MaxDrawdown,
			"current_drawdown":  metrics.CurrentDrawdown,
			"active_positions":  len(metrics.ActivePositions),
			"current_exposure":  metrics.CurrentExposure,
			"total_volume":      metrics.TotalVolume,
			"total_fees":        metrics.TotalFees,
			"largest_win":       metrics.LargestWin,
			"largest_loss":      metrics.LargestLoss,
			"consecutive_wins":  metrics.ConsecutiveWins,
			"consecutive_losses": metrics.ConsecutiveLosses,
			"accounts":          len(metrics.Accounts),
			"last_update":       metrics.LastUpdate,
		}
		
		// Add custom metrics
		if len(metrics.CustomMetrics) > 0 {
			strategySnapshot["custom_metrics"] = metrics.CustomMetrics
		}
		
		metrics.mu.RUnlock()
		
		snapshot[strategyID] = strategySnapshot
	}
	
	return snapshot
}

// processTrades processes trade updates
func (t *StrategyPnLTracker) processTrades() {
	defer t.wg.Done()
	
	for {
		select {
		case <-t.ctx.Done():
			return
		case trade := <-t.tradeChan:
			if trade == nil {
				continue
			}
			
			t.mu.RLock()
			metrics, exists := t.strategies[trade.StrategyID]
			t.mu.RUnlock()
			
			if !exists {
				t.logger.Warn("Trade for unknown strategy",
					zap.String("strategy_id", trade.StrategyID))
				continue
			}
			
			metrics.mu.Lock()
			
			// Update trade statistics
			metrics.TotalTrades++
			metrics.TotalVolume += trade.Quantity * trade.Price
			metrics.TotalFees += trade.Fee
			metrics.RealizedPnL += trade.PnL
			metrics.TotalPnL = metrics.RealizedPnL + metrics.UnrealizedPnL
			
			// Update win/loss statistics
			if trade.PnL > 0 {
				metrics.WinningTrades++
				metrics.ConsecutiveWins++
				metrics.ConsecutiveLosses = 0
				
				if trade.PnL > metrics.LargestWin {
					metrics.LargestWin = trade.PnL
				}
			} else if trade.PnL < 0 {
				metrics.LosingTrades++
				metrics.ConsecutiveLosses++
				metrics.ConsecutiveWins = 0
				
				if math.Abs(trade.PnL) > math.Abs(metrics.LargestLoss) {
					metrics.LargestLoss = trade.PnL
				}
			}
			
			// Update daily P&L
			dateKey := trade.Timestamp.Format("2006-01-02")
			metrics.DailyPnL[dateKey] += trade.PnL
			
			metrics.LastUpdate = time.Now()
			
			metrics.mu.Unlock()
			
			// Check alerts
			t.checkAlerts(metrics, trade)
		}
	}
}

// processPositions processes position updates
func (t *StrategyPnLTracker) processPositions() {
	defer t.wg.Done()
	
	for {
		select {
		case <-t.ctx.Done():
			return
		case position := <-t.positionChan:
			if position == nil {
				continue
			}
			
			t.mu.RLock()
			metrics, exists := t.strategies[position.StrategyID]
			t.mu.RUnlock()
			
			if !exists {
				continue
			}
			
			metrics.mu.Lock()
			
			// Update or add position
			posKey := position.AccountID + ":" + position.Symbol
			
			if position.Quantity == 0 {
				// Position closed
				delete(metrics.ActivePositions, posKey)
			} else {
				// Update position
				metrics.ActivePositions[posKey] = position
			}
			
			// Recalculate unrealized P&L and exposure
			metrics.UnrealizedPnL = 0
			metrics.CurrentExposure = 0
			
			for _, pos := range metrics.ActivePositions {
				metrics.UnrealizedPnL += pos.UnrealizedPnL
				metrics.CurrentExposure += pos.Exposure
			}
			
			metrics.TotalPnL = metrics.RealizedPnL + metrics.UnrealizedPnL
			
			// Track max positions and exposure
			if len(metrics.ActivePositions) > metrics.MaxPositions {
				metrics.MaxPositions = len(metrics.ActivePositions)
			}
			
			if metrics.CurrentExposure > metrics.MaxExposure {
				metrics.MaxExposure = metrics.CurrentExposure
			}
			
			metrics.LastUpdate = time.Now()
			
			metrics.mu.Unlock()
		}
	}
}

// calculateMetrics periodically calculates performance metrics
func (t *StrategyPnLTracker) calculateMetrics() {
	defer t.wg.Done()
	
	ticker := time.NewTicker(t.config.CalculationInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.performCalculations()
		}
	}
}

// performCalculations calculates all performance metrics
func (t *StrategyPnLTracker) performCalculations() {
	t.mu.RLock()
	strategies := make([]*StrategyMetrics, 0, len(t.strategies))
	for _, metrics := range t.strategies {
		strategies = append(strategies, metrics)
	}
	t.mu.RUnlock()
	
	for _, metrics := range strategies {
		metrics.mu.Lock()
		
		// Calculate win rate
		if metrics.TotalTrades > 0 {
			metrics.WinRate = float64(metrics.WinningTrades) / float64(metrics.TotalTrades)
		}
		
		// Calculate average win/loss
		if metrics.WinningTrades > 0 {
			totalWins := 0.0
			for _, pnl := range metrics.DailyPnL {
				if pnl > 0 {
					totalWins += pnl
				}
			}
			metrics.AvgWin = totalWins / float64(metrics.WinningTrades)
		}
		
		if metrics.LosingTrades > 0 {
			totalLosses := 0.0
			for _, pnl := range metrics.DailyPnL {
				if pnl < 0 {
					totalLosses += math.Abs(pnl)
				}
			}
			metrics.AvgLoss = totalLosses / float64(metrics.LosingTrades)
		}
		
		// Calculate profit factor
		if metrics.AvgLoss > 0 && metrics.WinRate > 0 {
			lossRate := 1 - metrics.WinRate
			metrics.ProfitFactor = (metrics.WinRate * metrics.AvgWin) / (lossRate * metrics.AvgLoss)
		}
		
		// Calculate Sharpe ratio
		metrics.SharpeRatio = t.calculateSharpeRatio(metrics)
		
		// Calculate drawdown
		t.calculateDrawdown(metrics)
		
		metrics.mu.Unlock()
	}
}

// calculateSharpeRatio calculates the Sharpe ratio
func (t *StrategyPnLTracker) calculateSharpeRatio(metrics *StrategyMetrics) float64 {
	if len(metrics.PnLHistory) < 2 {
		return 0
	}
	
	// Calculate returns
	var returns []float64
	for i := 1; i < len(metrics.PnLHistory); i++ {
		if metrics.PnLHistory[i-1].TotalPnL != 0 {
			ret := (metrics.PnLHistory[i].TotalPnL - metrics.PnLHistory[i-1].TotalPnL) / 
			       metrics.PnLHistory[i-1].TotalPnL
			returns = append(returns, ret)
		}
	}
	
	if len(returns) == 0 {
		return 0
	}
	
	// Calculate mean return
	sum := 0.0
	for _, r := range returns {
		sum += r
	}
	meanReturn := sum / float64(len(returns))
	
	// Calculate standard deviation
	sumSquared := 0.0
	for _, r := range returns {
		sumSquared += math.Pow(r-meanReturn, 2)
	}
	stdDev := math.Sqrt(sumSquared / float64(len(returns)))
	
	if stdDev == 0 {
		return 0
	}
	
	// Annualize and calculate Sharpe ratio
	periodsPerYear := 365.0 * 24.0 * 60.0 / t.config.CalculationInterval.Minutes()
	annualizedReturn := meanReturn * periodsPerYear
	annualizedStdDev := stdDev * math.Sqrt(periodsPerYear)
	
	return (annualizedReturn - t.config.RiskFreeRate) / annualizedStdDev
}

// calculateDrawdown calculates current and maximum drawdown
func (t *StrategyPnLTracker) calculateDrawdown(metrics *StrategyMetrics) {
	if len(metrics.PnLHistory) == 0 {
		return
	}
	
	// Find peak value
	peak := 0.0
	for _, snapshot := range metrics.PnLHistory {
		if snapshot.TotalPnL > peak {
			peak = snapshot.TotalPnL
		}
	}
	
	// Calculate current drawdown
	if peak > 0 {
		metrics.CurrentDrawdown = (peak - metrics.TotalPnL) / peak
	}
	
	// Calculate max drawdown over the window
	windowStart := time.Now().Add(-t.config.DrawdownWindow)
	
	localPeak := 0.0
	maxDD := 0.0
	var maxDDDate time.Time
	
	for _, snapshot := range metrics.PnLHistory {
		if snapshot.Timestamp.Before(windowStart) {
			continue
		}
		
		if snapshot.TotalPnL > localPeak {
			localPeak = snapshot.TotalPnL
		}
		
		if localPeak > 0 {
			dd := (localPeak - snapshot.TotalPnL) / localPeak
			if dd > maxDD {
				maxDD = dd
				maxDDDate = snapshot.Timestamp
			}
		}
	}
	
	metrics.MaxDrawdown = maxDD
	metrics.MaxDrawdownDate = maxDDDate
}

// takeSnapshots periodically takes P&L snapshots
func (t *StrategyPnLTracker) takeSnapshots() {
	defer t.wg.Done()
	
	ticker := time.NewTicker(t.config.SnapshotInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.performSnapshot()
		}
	}
}

// performSnapshot takes a snapshot of all strategies
func (t *StrategyPnLTracker) performSnapshot() {
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	now := time.Now()
	
	for _, metrics := range t.strategies {
		metrics.mu.Lock()
		
		// Add P&L snapshot
		snapshot := PnLSnapshot{
			Timestamp:     now,
			RealizedPnL:   metrics.RealizedPnL,
			UnrealizedPnL: metrics.UnrealizedPnL,
			TotalPnL:      metrics.TotalPnL,
			Positions:     len(metrics.ActivePositions),
			Exposure:      metrics.CurrentExposure,
		}
		
		metrics.PnLHistory = append(metrics.PnLHistory, snapshot)
		
		// Limit history size (keep last 24 hours at minute intervals)
		maxSnapshots := int(24 * time.Hour / t.config.SnapshotInterval)
		if len(metrics.PnLHistory) > maxSnapshots {
			metrics.PnLHistory = metrics.PnLHistory[len(metrics.PnLHistory)-maxSnapshots:]
		}
		
		// Add drawdown snapshot if in drawdown
		if metrics.CurrentDrawdown > 0 {
			ddSnapshot := DrawdownSnapshot{
				Timestamp:       now,
				DrawdownPercent: metrics.CurrentDrawdown,
				DrawdownValue:   metrics.CurrentDrawdown * metrics.TotalPnL,
				PeakValue:       metrics.TotalPnL / (1 - metrics.CurrentDrawdown),
			}
			
			metrics.DrawdownHistory = append(metrics.DrawdownHistory, ddSnapshot)
			
			// Limit drawdown history
			if len(metrics.DrawdownHistory) > maxSnapshots {
				metrics.DrawdownHistory = metrics.DrawdownHistory[len(metrics.DrawdownHistory)-maxSnapshots:]
			}
		}
		
		metrics.mu.Unlock()
	}
}

// checkAlerts checks for alert conditions
func (t *StrategyPnLTracker) checkAlerts(metrics *StrategyMetrics, trade *StrategyTrade) {
	metrics.mu.RLock()
	defer metrics.mu.RUnlock()
	
	// Check max drawdown alert
	if metrics.MaxDrawdown > t.config.MaxDrawdownAlert {
		t.logger.Error("Max drawdown exceeded",
			zap.String("strategy_id", metrics.StrategyID),
			zap.Float64("drawdown", metrics.MaxDrawdown),
			zap.Float64("threshold", t.config.MaxDrawdownAlert))
	}
	
	// Check daily loss alert
	today := time.Now().Format("2006-01-02")
	if dailyPnL, exists := metrics.DailyPnL[today]; exists && dailyPnL < -t.config.DailyLossAlert {
		t.logger.Error("Daily loss limit exceeded",
			zap.String("strategy_id", metrics.StrategyID),
			zap.Float64("daily_pnl", dailyPnL),
			zap.Float64("threshold", -t.config.DailyLossAlert))
	}
	
	// Check consecutive losses
	if metrics.ConsecutiveLosses >= t.config.ConsecutiveLossDays {
		t.logger.Warn("Consecutive losses alert",
			zap.String("strategy_id", metrics.StrategyID),
			zap.Int("consecutive_losses", metrics.ConsecutiveLosses))
	}
}

// SetCustomMetric sets a custom metric for a strategy
func (t *StrategyPnLTracker) SetCustomMetric(strategyID, metricName string, value float64) {
	t.mu.RLock()
	metrics, exists := t.strategies[strategyID]
	t.mu.RUnlock()
	
	if !exists {
		return
	}
	
	metrics.mu.Lock()
	metrics.CustomMetrics[metricName] = value
	metrics.mu.Unlock()
}

// Stop gracefully stops the P&L tracker
func (t *StrategyPnLTracker) Stop() {
	t.cancel()
	t.wg.Wait()
	
	close(t.tradeChan)
	close(t.positionChan)
	
	t.logger.Info("Strategy P&L tracker stopped")
}

// GetStrategyReport generates a detailed report for a strategy
func (t *StrategyPnLTracker) GetStrategyReport(strategyID string) (map[string]interface{}, error) {
	metrics, exists := t.GetStrategyMetrics(strategyID)
	if !exists {
		return nil, nil
	}
	
	metrics.mu.RLock()
	defer metrics.mu.RUnlock()
	
	report := map[string]interface{}{
		"summary": map[string]interface{}{
			"strategy_id":   strategyID,
			"strategy_type": metrics.StrategyType,
			"running_since": metrics.StartTime,
			"runtime_hours": time.Since(metrics.StartTime).Hours(),
		},
		"pnl": map[string]interface{}{
			"total":      metrics.TotalPnL,
			"realized":   metrics.RealizedPnL,
			"unrealized": metrics.UnrealizedPnL,
			"daily_avg":  metrics.TotalPnL / math.Max(1, time.Since(metrics.StartTime).Hours()/24),
		},
		"trading": map[string]interface{}{
			"total_trades":   metrics.TotalTrades,
			"winning_trades": metrics.WinningTrades,
			"losing_trades":  metrics.LosingTrades,
			"win_rate":       metrics.WinRate,
			"avg_win":        metrics.AvgWin,
			"avg_loss":       metrics.AvgLoss,
			"profit_factor":  metrics.ProfitFactor,
			"total_volume":   metrics.TotalVolume,
			"total_fees":     metrics.TotalFees,
		},
		"risk": map[string]interface{}{
			"sharpe_ratio":      metrics.SharpeRatio,
			"max_drawdown":      metrics.MaxDrawdown,
			"current_drawdown":  metrics.CurrentDrawdown,
			"max_exposure":      metrics.MaxExposure,
			"current_exposure":  metrics.CurrentExposure,
			"active_positions":  len(metrics.ActivePositions),
			"max_positions":     metrics.MaxPositions,
		},
		"streaks": map[string]interface{}{
			"consecutive_wins":   metrics.ConsecutiveWins,
			"consecutive_losses": metrics.ConsecutiveLosses,
			"largest_win":        metrics.LargestWin,
			"largest_loss":       metrics.LargestLoss,
		},
	}
	
	// Add recent P&L history
	if len(metrics.PnLHistory) > 0 {
		recentHistory := metrics.PnLHistory
		if len(recentHistory) > 100 {
			recentHistory = recentHistory[len(recentHistory)-100:]
		}
		
		report["recent_history"] = recentHistory
	}
	
	// Add custom metrics
	if len(metrics.CustomMetrics) > 0 {
		report["custom_metrics"] = metrics.CustomMetrics
	}
	
	return report, nil
}