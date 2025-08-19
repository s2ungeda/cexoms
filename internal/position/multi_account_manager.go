package position

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// MultiAccountPositionManager manages positions across multiple accounts
type MultiAccountPositionManager struct {
	mu sync.RWMutex
	
	// Account management
	accountManager types.AccountManager
	
	// Position tracking
	positions      map[string]map[string]*Position // accountID -> symbol -> position
	accountSummary map[string]*AccountSummary      // accountID -> summary
	
	// Global position tracking
	globalPositions map[string]*GlobalPosition     // symbol -> aggregated position
	
	// Configuration
	config         *MultiAccountConfig
	
	// Event channels
	updateChan     chan PositionUpdate
	alertChan      chan PositionAlert
	
	// Background workers
	stopCh         chan struct{}
	wg             sync.WaitGroup
}

// Position represents a single position
type Position struct {
	AccountID      string
	Symbol         string
	Side           types.PositionSide
	Quantity       decimal.Decimal
	EntryPrice     decimal.Decimal
	MarkPrice      decimal.Decimal
	RealizedPnL    decimal.Decimal
	UnrealizedPnL  decimal.Decimal
	Margin         decimal.Decimal
	Leverage       int
	PositionValue  decimal.Decimal
	
	// Risk metrics
	StopLoss       decimal.Decimal
	TakeProfit     decimal.Decimal
	MaxLoss        decimal.Decimal
	
	// Tracking
	OpenTime       time.Time
	UpdateTime     time.Time
	
	// Multi-account specific
	Strategy       string
	ParentOrderID  string
	LinkedAccounts []string // For hedge positions
}

// AccountSummary summarizes positions for an account
type AccountSummary struct {
	AccountID        string
	TotalPositions   int
	LongPositions    int
	ShortPositions   int
	TotalValue       decimal.Decimal
	TotalMargin      decimal.Decimal
	RealizedPnL      decimal.Decimal
	UnrealizedPnL    decimal.Decimal
	MarginRatio      decimal.Decimal
	EffectiveLeverage decimal.Decimal
	
	// Position breakdown
	PositionsBySymbol map[string]*Position
	LargestPosition   *Position
	RiskiestPosition  *Position
	
	LastUpdate       time.Time
}

// GlobalPosition aggregates positions across accounts
type GlobalPosition struct {
	Symbol          string
	NetQuantity     decimal.Decimal
	TotalLong       decimal.Decimal
	TotalShort      decimal.Decimal
	AvgEntryPrice   decimal.Decimal
	TotalValue      decimal.Decimal
	TotalPnL        decimal.Decimal
	
	// Account distribution
	AccountPositions map[string]*Position
	NumAccounts      int
	
	// Hedge analysis
	HedgeRatio      decimal.Decimal
	IsHedged        bool
	HedgeQuality    string // "perfect", "partial", "none"
}

// MultiAccountConfig contains configuration
type MultiAccountConfig struct {
	// Update settings
	UpdateInterval      time.Duration
	BatchUpdates        bool
	
	// Position limits
	MaxPositionsPerAccount   int
	MaxGlobalPositions       int
	MaxPositionValue        decimal.Decimal
	
	// Hedge detection
	HedgeDetectionEnabled   bool
	PerfectHedgeThreshold   decimal.Decimal // e.g., 0.95 for 95% hedged
	
	// Alerts
	AlertsEnabled           bool
	MarginAlertThreshold    decimal.Decimal
	PnLAlertThreshold       decimal.Decimal
}

// NewMultiAccountPositionManager creates a new position manager
func NewMultiAccountPositionManager(accountManager types.AccountManager, config *MultiAccountConfig) *MultiAccountPositionManager {
	if config == nil {
		config = &MultiAccountConfig{
			UpdateInterval:          5 * time.Second,
			BatchUpdates:            true,
			MaxPositionsPerAccount:  100,
			MaxGlobalPositions:      1000,
			MaxPositionValue:        decimal.NewFromInt(1000000),
			HedgeDetectionEnabled:   true,
			PerfectHedgeThreshold:   decimal.NewFromFloat(0.95),
			AlertsEnabled:           true,
			MarginAlertThreshold:    decimal.NewFromFloat(0.8),
			PnLAlertThreshold:       decimal.NewFromInt(-10000),
		}
	}
	
	pm := &MultiAccountPositionManager{
		accountManager:   accountManager,
		positions:        make(map[string]map[string]*Position),
		accountSummary:   make(map[string]*AccountSummary),
		globalPositions:  make(map[string]*GlobalPosition),
		config:           config,
		updateChan:       make(chan PositionUpdate, 1000),
		alertChan:        make(chan PositionAlert, 100),
		stopCh:           make(chan struct{}),
	}
	
	// Start background workers
	pm.wg.Add(2)
	go pm.updateWorker()
	go pm.aggregationWorker()
	
	return pm
}

// UpdatePosition updates or creates a position
func (pm *MultiAccountPositionManager) UpdatePosition(update PositionUpdate) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	// Get or create account positions map
	if _, exists := pm.positions[update.AccountID]; !exists {
		pm.positions[update.AccountID] = make(map[string]*Position)
	}
	
	// Update position
	pos, exists := pm.positions[update.AccountID][update.Symbol]
	if !exists {
		pos = &Position{
			AccountID: update.AccountID,
			Symbol:    update.Symbol,
			OpenTime:  time.Now(),
		}
		pm.positions[update.AccountID][update.Symbol] = pos
	}
	
	// Update position fields
	pos.Side = update.Side
	pos.Quantity = update.Quantity
	pos.EntryPrice = update.EntryPrice
	pos.MarkPrice = update.MarkPrice
	pos.RealizedPnL = update.RealizedPnL
	pos.UnrealizedPnL = update.UnrealizedPnL
	pos.Margin = update.Margin
	pos.Leverage = update.Leverage
	pos.UpdateTime = time.Now()
	
	// Calculate position value
	pos.PositionValue = pos.Quantity.Mul(pos.MarkPrice)
	
	// Get account info for strategy
	account, err := pm.accountManager.GetAccount(update.AccountID)
	if err == nil {
		pos.Strategy = account.Strategy
	}
	
	// Check for position removal
	if pos.Quantity.IsZero() {
		delete(pm.positions[update.AccountID], update.Symbol)
		if len(pm.positions[update.AccountID]) == 0 {
			delete(pm.positions, update.AccountID)
		}
	}
	
	// Send update to channel
	select {
	case pm.updateChan <- update:
	default:
		// Channel full, skip
	}
	
	// Check alerts
	pm.checkPositionAlerts(pos)
	
	return nil
}

// GetPosition retrieves a specific position
func (pm *MultiAccountPositionManager) GetPosition(accountID, symbol string) (*Position, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	if accountPositions, exists := pm.positions[accountID]; exists {
		if pos, exists := accountPositions[symbol]; exists {
			return pos, nil
		}
	}
	
	return nil, fmt.Errorf("position not found")
}

// GetAccountPositions retrieves all positions for an account
func (pm *MultiAccountPositionManager) GetAccountPositions(accountID string) ([]*Position, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	accountPositions, exists := pm.positions[accountID]
	if !exists {
		return []*Position{}, nil
	}
	
	positions := make([]*Position, 0, len(accountPositions))
	for _, pos := range accountPositions {
		positions = append(positions, pos)
	}
	
	return positions, nil
}

// GetGlobalPosition retrieves aggregated position for a symbol
func (pm *MultiAccountPositionManager) GetGlobalPosition(symbol string) (*GlobalPosition, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	globalPos, exists := pm.globalPositions[symbol]
	if !exists {
		return nil, fmt.Errorf("no global position for symbol %s", symbol)
	}
	
	return globalPos, nil
}

// GetAccountSummary retrieves summary for an account
func (pm *MultiAccountPositionManager) GetAccountSummary(accountID string) (*AccountSummary, error) {
	pm.mu.RLock()
	summary, exists := pm.accountSummary[accountID]
	pm.mu.RUnlock()
	
	if !exists {
		// Generate summary on demand
		summary = pm.generateAccountSummary(accountID)
	}
	
	return summary, nil
}

// GetPortfolioSummary retrieves portfolio-wide summary
func (pm *MultiAccountPositionManager) GetPortfolioSummary() *PortfolioSummary {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	summary := &PortfolioSummary{
		Timestamp:          time.Now(),
		AccountSummaries:   make(map[string]*AccountSummary),
		GlobalPositions:    make(map[string]*GlobalPosition),
		StrategyBreakdown:  make(map[string]*StrategyStats),
	}
	
	// Aggregate metrics
	for accountID := range pm.positions {
		accountSummary := pm.generateAccountSummary(accountID)
		summary.AccountSummaries[accountID] = accountSummary
		
		summary.TotalPositions += accountSummary.TotalPositions
		summary.TotalValue = summary.TotalValue.Add(accountSummary.TotalValue)
		summary.TotalMargin = summary.TotalMargin.Add(accountSummary.TotalMargin)
		summary.TotalRealizedPnL = summary.TotalRealizedPnL.Add(accountSummary.RealizedPnL)
		summary.TotalUnrealizedPnL = summary.TotalUnrealizedPnL.Add(accountSummary.UnrealizedPnL)
		
		// Track by strategy
		account, _ := pm.accountManager.GetAccount(accountID)
		if account != nil && account.Strategy != "" {
			if _, exists := summary.StrategyBreakdown[account.Strategy]; !exists {
				summary.StrategyBreakdown[account.Strategy] = &StrategyStats{
					Strategy: account.Strategy,
				}
			}
			
			stats := summary.StrategyBreakdown[account.Strategy]
			stats.NumAccounts++
			stats.TotalPositions += accountSummary.TotalPositions
			stats.TotalValue = stats.TotalValue.Add(accountSummary.TotalValue)
			stats.TotalPnL = stats.TotalPnL.Add(accountSummary.RealizedPnL).Add(accountSummary.UnrealizedPnL)
		}
	}
	
	// Copy global positions
	for symbol, globalPos := range pm.globalPositions {
		summary.GlobalPositions[symbol] = globalPos
	}
	
	// Calculate portfolio metrics
	if !summary.TotalMargin.IsZero() {
		summary.PortfolioLeverage = summary.TotalValue.Div(summary.TotalMargin)
	}
	
	// Identify hedged positions
	summary.HedgedPositions = pm.identifyHedgedPositions()
	summary.NetExposure = pm.calculateNetExposure()
	
	return summary
}

// ClosePosition closes a position
func (pm *MultiAccountPositionManager) ClosePosition(ctx context.Context, accountID, symbol string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	// Get position
	accountPositions, exists := pm.positions[accountID]
	if !exists {
		return fmt.Errorf("no positions for account %s", accountID)
	}
	
	pos, exists := accountPositions[symbol]
	if !exists {
		return fmt.Errorf("no position for symbol %s", symbol)
	}
	
	// Mark as closed
	pos.Quantity = decimal.Zero
	pos.UnrealizedPnL = decimal.Zero
	pos.UpdateTime = time.Now()
	
	// Remove from map
	delete(accountPositions, symbol)
	if len(accountPositions) == 0 {
		delete(pm.positions, accountID)
	}
	
	// Send close event
	pm.sendPositionEvent(PositionEvent{
		Type:      "position_closed",
		AccountID: accountID,
		Symbol:    symbol,
		Position:  pos,
		Timestamp: time.Now(),
	})
	
	return nil
}

// CloseAllPositions closes all positions for an account
func (pm *MultiAccountPositionManager) CloseAllPositions(ctx context.Context, accountID string) error {
	positions, err := pm.GetAccountPositions(accountID)
	if err != nil {
		return err
	}
	
	for _, pos := range positions {
		if err := pm.ClosePosition(ctx, accountID, pos.Symbol); err != nil {
			return fmt.Errorf("failed to close position %s: %w", pos.Symbol, err)
		}
	}
	
	return nil
}

// SetStopLoss sets stop loss for a position
func (pm *MultiAccountPositionManager) SetStopLoss(accountID, symbol string, stopLoss decimal.Decimal) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pos, err := pm.getPositionUnsafe(accountID, symbol)
	if err != nil {
		return err
	}
	
	pos.StopLoss = stopLoss
	
	// Calculate max loss
	if pos.Side == types.PositionSideLong {
		pos.MaxLoss = pos.EntryPrice.Sub(stopLoss).Mul(pos.Quantity)
	} else {
		pos.MaxLoss = stopLoss.Sub(pos.EntryPrice).Mul(pos.Quantity)
	}
	
	return nil
}

// SetTakeProfit sets take profit for a position
func (pm *MultiAccountPositionManager) SetTakeProfit(accountID, symbol string, takeProfit decimal.Decimal) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pos, err := pm.getPositionUnsafe(accountID, symbol)
	if err != nil {
		return err
	}
	
	pos.TakeProfit = takeProfit
	
	return nil
}

// LinkPositions links positions across accounts (for hedging)
func (pm *MultiAccountPositionManager) LinkPositions(links []PositionLink) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	for _, link := range links {
		// Get positions
		pos1, err := pm.getPositionUnsafe(link.AccountID1, link.Symbol1)
		if err != nil {
			return err
		}
		
		pos2, err := pm.getPositionUnsafe(link.AccountID2, link.Symbol2)
		if err != nil {
			return err
		}
		
		// Link positions
		pos1.LinkedAccounts = append(pos1.LinkedAccounts, link.AccountID2)
		pos2.LinkedAccounts = append(pos2.LinkedAccounts, link.AccountID1)
	}
	
	return nil
}

// Helper methods

// generateAccountSummary generates summary for an account
func (pm *MultiAccountPositionManager) generateAccountSummary(accountID string) *AccountSummary {
	summary := &AccountSummary{
		AccountID:         accountID,
		PositionsBySymbol: make(map[string]*Position),
		LastUpdate:        time.Now(),
	}
	
	positions, exists := pm.positions[accountID]
	if !exists {
		return summary
	}
	
	var largestValue decimal.Decimal
	var highestRisk decimal.Decimal
	
	for symbol, pos := range positions {
		summary.TotalPositions++
		summary.PositionsBySymbol[symbol] = pos
		
		if pos.Side == types.PositionSideLong {
			summary.LongPositions++
		} else {
			summary.ShortPositions++
		}
		
		summary.TotalValue = summary.TotalValue.Add(pos.PositionValue.Abs())
		summary.TotalMargin = summary.TotalMargin.Add(pos.Margin)
		summary.RealizedPnL = summary.RealizedPnL.Add(pos.RealizedPnL)
		summary.UnrealizedPnL = summary.UnrealizedPnL.Add(pos.UnrealizedPnL)
		
		// Track largest position
		if pos.PositionValue.Abs().GreaterThan(largestValue) {
			largestValue = pos.PositionValue.Abs()
			summary.LargestPosition = pos
		}
		
		// Track riskiest position (by max loss)
		if pos.MaxLoss.GreaterThan(highestRisk) {
			highestRisk = pos.MaxLoss
			summary.RiskiestPosition = pos
		}
	}
	
	// Calculate metrics
	if !summary.TotalMargin.IsZero() {
		summary.EffectiveLeverage = summary.TotalValue.Div(summary.TotalMargin)
	}
	
	// Get account balance for margin ratio
	balance, _ := pm.accountManager.GetBalance(accountID)
	if balance != nil && !balance.TotalUSDT.IsZero() {
		summary.MarginRatio = summary.TotalMargin.Div(balance.TotalUSDT)
	}
	
	return summary
}

// aggregateGlobalPositions aggregates positions across accounts
func (pm *MultiAccountPositionManager) aggregateGlobalPositions() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	// Clear existing global positions
	pm.globalPositions = make(map[string]*GlobalPosition)
	
	// Aggregate by symbol
	for accountID, accountPositions := range pm.positions {
		for symbol, pos := range accountPositions {
			globalPos, exists := pm.globalPositions[symbol]
			if !exists {
				globalPos = &GlobalPosition{
					Symbol:           symbol,
					AccountPositions: make(map[string]*Position),
				}
				pm.globalPositions[symbol] = globalPos
			}
			
			// Add to global position
			globalPos.AccountPositions[accountID] = pos
			globalPos.NumAccounts++
			
			if pos.Side == types.PositionSideLong {
				globalPos.TotalLong = globalPos.TotalLong.Add(pos.Quantity)
			} else {
				globalPos.TotalShort = globalPos.TotalShort.Add(pos.Quantity)
			}
			
			globalPos.TotalValue = globalPos.TotalValue.Add(pos.PositionValue)
			globalPos.TotalPnL = globalPos.TotalPnL.Add(pos.UnrealizedPnL)
		}
	}
	
	// Calculate net positions and hedge ratios
	for _, globalPos := range pm.globalPositions {
		globalPos.NetQuantity = globalPos.TotalLong.Sub(globalPos.TotalShort)
		
		// Calculate average entry price (weighted)
		totalCost := decimal.Zero
		totalQty := decimal.Zero
		for _, pos := range globalPos.AccountPositions {
			cost := pos.EntryPrice.Mul(pos.Quantity)
			totalCost = totalCost.Add(cost)
			totalQty = totalQty.Add(pos.Quantity)
		}
		
		if !totalQty.IsZero() {
			globalPos.AvgEntryPrice = totalCost.Div(totalQty)
		}
		
		// Calculate hedge ratio
		if pm.config.HedgeDetectionEnabled {
			globalPos.HedgeRatio = pm.calculateHedgeRatio(globalPos)
			globalPos.IsHedged = globalPos.HedgeRatio.GreaterThan(pm.config.PerfectHedgeThreshold)
			
			if globalPos.IsHedged {
				globalPos.HedgeQuality = "perfect"
			} else if globalPos.HedgeRatio.GreaterThan(decimal.NewFromFloat(0.5)) {
				globalPos.HedgeQuality = "partial"
			} else {
				globalPos.HedgeQuality = "none"
			}
		}
	}
}

// calculateHedgeRatio calculates hedge ratio for a global position
func (pm *MultiAccountPositionManager) calculateHedgeRatio(globalPos *GlobalPosition) decimal.Decimal {
	if globalPos.TotalLong.IsZero() || globalPos.TotalShort.IsZero() {
		return decimal.Zero
	}
	
	// Hedge ratio = min(long, short) / max(long, short)
	minQty := globalPos.TotalLong
	maxQty := globalPos.TotalShort
	
	if globalPos.TotalShort.LessThan(globalPos.TotalLong) {
		minQty = globalPos.TotalShort
		maxQty = globalPos.TotalLong
	}
	
	return minQty.Div(maxQty)
}

// identifyHedgedPositions identifies hedged position pairs
func (pm *MultiAccountPositionManager) identifyHedgedPositions() []HedgedPosition {
	hedged := make([]HedgedPosition, 0)
	
	for symbol, globalPos := range pm.globalPositions {
		if globalPos.IsHedged {
			// Find matching long/short pairs
			longPositions := make([]*Position, 0)
			shortPositions := make([]*Position, 0)
			
			for _, pos := range globalPos.AccountPositions {
				if pos.Side == types.PositionSideLong {
					longPositions = append(longPositions, pos)
				} else {
					shortPositions = append(shortPositions, pos)
				}
			}
			
			// Simple pairing - in production use more sophisticated matching
			for i := 0; i < len(longPositions) && i < len(shortPositions); i++ {
				hedged = append(hedged, HedgedPosition{
					Symbol:        symbol,
					LongAccount:   longPositions[i].AccountID,
					ShortAccount:  shortPositions[i].AccountID,
					HedgedQuantity: decimal.Min(longPositions[i].Quantity, shortPositions[i].Quantity),
					HedgeRatio:    globalPos.HedgeRatio,
				})
			}
		}
	}
	
	return hedged
}

// calculateNetExposure calculates net exposure across all positions
func (pm *MultiAccountPositionManager) calculateNetExposure() decimal.Decimal {
	netExposure := decimal.Zero
	
	for _, globalPos := range pm.globalPositions {
		// Net exposure = long value - short value
		longValue := globalPos.TotalLong.Mul(globalPos.AvgEntryPrice)
		shortValue := globalPos.TotalShort.Mul(globalPos.AvgEntryPrice)
		netExposure = netExposure.Add(longValue.Sub(shortValue))
	}
	
	return netExposure
}

// checkPositionAlerts checks for position-related alerts
func (pm *MultiAccountPositionManager) checkPositionAlerts(pos *Position) {
	if !pm.config.AlertsEnabled {
		return
	}
	
	// Check margin alert
	account, _ := pm.accountManager.GetAccount(pos.AccountID)
	if account != nil {
		balance, _ := pm.accountManager.GetBalance(pos.AccountID)
		if balance != nil && !balance.TotalUSDT.IsZero() {
			marginRatio := pos.Margin.Div(balance.TotalUSDT)
			if marginRatio.GreaterThan(pm.config.MarginAlertThreshold) {
				pm.sendAlert(PositionAlert{
					Type:      "high_margin",
					Severity:  "warning",
					AccountID: pos.AccountID,
					Symbol:    pos.Symbol,
					Message:   fmt.Sprintf("High margin usage: %.2f%%", marginRatio.Mul(decimal.NewFromInt(100)).InexactFloat64()),
					Timestamp: time.Now(),
				})
			}
		}
	}
	
	// Check P&L alert
	if pos.UnrealizedPnL.LessThan(pm.config.PnLAlertThreshold) {
		pm.sendAlert(PositionAlert{
			Type:      "large_loss",
			Severity:  "critical",
			AccountID: pos.AccountID,
			Symbol:    pos.Symbol,
			Message:   fmt.Sprintf("Large unrealized loss: %s", pos.UnrealizedPnL.String()),
			Timestamp: time.Now(),
		})
	}
}

// getPositionUnsafe gets position without lock (caller must hold lock)
func (pm *MultiAccountPositionManager) getPositionUnsafe(accountID, symbol string) (*Position, error) {
	accountPositions, exists := pm.positions[accountID]
	if !exists {
		return nil, fmt.Errorf("no positions for account %s", accountID)
	}
	
	pos, exists := accountPositions[symbol]
	if !exists {
		return nil, fmt.Errorf("no position for symbol %s", symbol)
	}
	
	return pos, nil
}

// sendAlert sends a position alert
func (pm *MultiAccountPositionManager) sendAlert(alert PositionAlert) {
	select {
	case pm.alertChan <- alert:
	default:
		// Channel full, drop alert
	}
}

// sendPositionEvent sends a position event
func (pm *MultiAccountPositionManager) sendPositionEvent(event PositionEvent) {
	// Implementation depends on event system
}

// Background workers

// updateWorker processes position updates
func (pm *MultiAccountPositionManager) updateWorker() {
	defer pm.wg.Done()
	
	batchTimer := time.NewTicker(100 * time.Millisecond)
	defer batchTimer.Stop()
	
	updates := make([]PositionUpdate, 0, 100)
	
	for {
		select {
		case update := <-pm.updateChan:
			if pm.config.BatchUpdates {
				updates = append(updates, update)
				if len(updates) >= 100 {
					pm.processBatchUpdates(updates)
					updates = updates[:0]
				}
			} else {
				pm.UpdatePosition(update)
			}
			
		case <-batchTimer.C:
			if len(updates) > 0 {
				pm.processBatchUpdates(updates)
				updates = updates[:0]
			}
			
		case <-pm.stopCh:
			return
		}
	}
}

// processBatchUpdates processes a batch of position updates
func (pm *MultiAccountPositionManager) processBatchUpdates(updates []PositionUpdate) {
	for _, update := range updates {
		pm.UpdatePosition(update)
	}
}

// aggregationWorker periodically aggregates global positions
func (pm *MultiAccountPositionManager) aggregationWorker() {
	defer pm.wg.Done()
	
	ticker := time.NewTicker(pm.config.UpdateInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			pm.aggregateGlobalPositions()
			pm.updateAccountSummaries()
			
		case <-pm.stopCh:
			return
		}
	}
}

// updateAccountSummaries updates all account summaries
func (pm *MultiAccountPositionManager) updateAccountSummaries() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	for accountID := range pm.positions {
		pm.accountSummary[accountID] = pm.generateAccountSummary(accountID)
	}
	
	// Clean up empty accounts
	for accountID, summary := range pm.accountSummary {
		if summary.TotalPositions == 0 {
			delete(pm.accountSummary, accountID)
		}
	}
}

// Stop stops the position manager
func (pm *MultiAccountPositionManager) Stop() {
	close(pm.stopCh)
	pm.wg.Wait()
	close(pm.updateChan)
	close(pm.alertChan)
}

// GetUpdateChannel returns the update channel
func (pm *MultiAccountPositionManager) GetUpdateChannel() chan<- PositionUpdate {
	return pm.updateChan
}

// GetAlertChannel returns the alert channel
func (pm *MultiAccountPositionManager) GetAlertChannel() <-chan PositionAlert {
	return pm.alertChan
}

// Supporting types

// PositionUpdate represents a position update
type PositionUpdate struct {
	AccountID     string
	Symbol        string
	Side          types.PositionSide
	Quantity      decimal.Decimal
	EntryPrice    decimal.Decimal
	MarkPrice     decimal.Decimal
	RealizedPnL   decimal.Decimal
	UnrealizedPnL decimal.Decimal
	Margin        decimal.Decimal
	Leverage      int
	Timestamp     time.Time
}

// PositionAlert represents a position alert
type PositionAlert struct {
	Type      string
	Severity  string
	AccountID string
	Symbol    string
	Message   string
	Timestamp time.Time
}

// PositionEvent represents a position event
type PositionEvent struct {
	Type      string
	AccountID string
	Symbol    string
	Position  *Position
	Timestamp time.Time
}

// PortfolioSummary summarizes the entire portfolio
type PortfolioSummary struct {
	Timestamp          time.Time
	TotalPositions     int
	TotalValue         decimal.Decimal
	TotalMargin        decimal.Decimal
	TotalRealizedPnL   decimal.Decimal
	TotalUnrealizedPnL decimal.Decimal
	PortfolioLeverage  decimal.Decimal
	NetExposure        decimal.Decimal
	
	AccountSummaries   map[string]*AccountSummary
	GlobalPositions    map[string]*GlobalPosition
	StrategyBreakdown  map[string]*StrategyStats
	HedgedPositions    []HedgedPosition
}

// StrategyStats contains statistics for a strategy
type StrategyStats struct {
	Strategy       string
	NumAccounts    int
	TotalPositions int
	TotalValue     decimal.Decimal
	TotalPnL       decimal.Decimal
}

// HedgedPosition represents a hedged position pair
type HedgedPosition struct {
	Symbol         string
	LongAccount    string
	ShortAccount   string
	HedgedQuantity decimal.Decimal
	HedgeRatio     decimal.Decimal
}

// PositionLink links positions across accounts
type PositionLink struct {
	AccountID1 string
	Symbol1    string
	AccountID2 string
	Symbol2    string
	LinkType   string // "hedge", "spread", "arbitrage"
}