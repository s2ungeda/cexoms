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
	positions      map[string]map[string]*MultiAccountPosition // accountID -> symbol -> position
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

// MultiAccountPosition alias removed - use types.Position instead

// MultiAccountPosition represents a single position
type MultiAccountPosition struct {
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
	Strategy       string
	OpenTime       time.Time
	UpdateTime     time.Time
	StopLoss       decimal.Decimal
	TakeProfit     decimal.Decimal
	MaxLoss        decimal.Decimal
	ParentOrderID  string
	LinkedAccounts []string // For hedge positions
}

// AccountSummary summarizes positions for an account
type AccountSummary struct {
	AccountID         string
	TotalPositions    int
	TotalValue        decimal.Decimal
	TotalPnL          decimal.Decimal
	TotalMargin       decimal.Decimal
	AvailableMargin   decimal.Decimal
	MarginLevel       decimal.Decimal
	UpdateTime        time.Time
	PositionsBySymbol map[string]*MultiAccountPosition
}

// GlobalPosition represents aggregated position across accounts
type GlobalPosition struct {
	Symbol           string
	TotalLong        decimal.Decimal
	TotalShort       decimal.Decimal
	NetPosition      decimal.Decimal
	TotalValue       decimal.Decimal
	AveragePrice     decimal.Decimal
	TotalPnL         decimal.Decimal
	AccountCount     int
	Accounts         map[string]*MultiAccountPosition
}

// MultiAccountConfig configuration for multi-account position manager
type MultiAccountConfig struct {
	MaxPositionsPerAccount int
	MaxGlobalExposure      decimal.Decimal
	MaxAccountRisk         decimal.Decimal
	EnableHedging          bool
	EnableAutoBalance      bool
	UpdateInterval         time.Duration
}

// PositionUpdate represents a position update event
type PositionUpdate struct {
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
	Timestamp      time.Time
}

// PositionAlert represents a position alert
type PositionAlert struct {
	AccountID   string
	Symbol      string
	AlertType   string
	Message     string
	Severity    string
	Value       decimal.Decimal
	Timestamp   time.Time
}

// NewMultiAccountPositionManager creates a new multi-account position manager
func NewMultiAccountPositionManager(accountManager types.AccountManager, config *MultiAccountConfig) *MultiAccountPositionManager {
	if config == nil {
		config = &MultiAccountConfig{
			MaxPositionsPerAccount: 20,
			MaxGlobalExposure:      decimal.NewFromInt(1000000),
			MaxAccountRisk:         decimal.NewFromFloat(0.1),
			EnableHedging:          true,
			EnableAutoBalance:      false,
			UpdateInterval:         5 * time.Second,
		}
	}
	
	return &MultiAccountPositionManager{
		accountManager:  accountManager,
		positions:       make(map[string]map[string]*MultiAccountPosition),
		accountSummary:  make(map[string]*AccountSummary),
		globalPositions: make(map[string]*GlobalPosition),
		config:          config,
		updateChan:      make(chan PositionUpdate, 1000),
		alertChan:       make(chan PositionAlert, 100),
		stopCh:          make(chan struct{}),
	}
}

// Start starts the position manager
func (pm *MultiAccountPositionManager) Start(ctx context.Context) error {
	// Start update worker
	pm.wg.Add(1)
	go pm.updateWorker(ctx)
	
	// Start monitoring worker
	pm.wg.Add(1)
	go pm.monitorWorker(ctx)
	
	return nil
}

// Stop stops the position manager
func (pm *MultiAccountPositionManager) Stop() error {
	close(pm.stopCh)
	pm.wg.Wait()
	return nil
}

// UpdatePosition updates a position
func (pm *MultiAccountPositionManager) UpdatePosition(update PositionUpdate) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	// Ensure account positions map exists
	if _, exists := pm.positions[update.AccountID]; !exists {
		pm.positions[update.AccountID] = make(map[string]*MultiAccountPosition)
	}
	
	// Update position
	pos, exists := pm.positions[update.AccountID][update.Symbol]
	if !exists {
		pos = &MultiAccountPosition{
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
	
	// Update global position
	pm.updateGlobalPosition(update.Symbol)
	
	// Update account summary
	pm.updateAccountSummary(update.AccountID)
	
	// Send update notification
	select {
	case pm.updateChan <- update:
	default:
	}
	
	return nil
}

// GetPosition gets a position for an account
func (pm *MultiAccountPositionManager) GetPosition(accountID, symbol string) (*MultiAccountPosition, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	if positions, exists := pm.positions[accountID]; exists {
		if pos, exists := positions[symbol]; exists {
			return pos, nil
		}
	}
	
	return nil, fmt.Errorf("position not found: %s %s", accountID, symbol)
}

// GetAccountPositions gets all positions for an account
func (pm *MultiAccountPositionManager) GetAccountPositions(accountID string) ([]*MultiAccountPosition, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	positions, exists := pm.positions[accountID]
	if !exists {
		return nil, nil
	}
	
	result := make([]*MultiAccountPosition, 0, len(positions))
	for _, pos := range positions {
		result = append(result, pos)
	}
	
	return result, nil
}

// GetGlobalPosition gets aggregated position for a symbol
func (pm *MultiAccountPositionManager) GetGlobalPosition(symbol string) (*GlobalPosition, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	if pos, exists := pm.globalPositions[symbol]; exists {
		return pos, nil
	}
	
	return nil, fmt.Errorf("global position not found: %s", symbol)
}

// GetAccountSummary gets account summary
func (pm *MultiAccountPositionManager) GetAccountSummary(accountID string) (*AccountSummary, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	if summary, exists := pm.accountSummary[accountID]; exists {
		return summary, nil
	}
	
	return nil, fmt.Errorf("account summary not found: %s", accountID)
}

// GetPortfolioSummary returns portfolio summary across all accounts
func (pm *MultiAccountPositionManager) GetPortfolioSummary() *PortfolioSummary {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	summary := &PortfolioSummary{
		TotalValue:       decimal.Zero,
		GlobalPositions:  make(map[string]*GlobalPosition),
		AccountSummaries: make(map[string]*AccountSummary),
	}
	
	// Aggregate positions by symbol
	for _, globalPos := range pm.globalPositions {
		summary.GlobalPositions[globalPos.Symbol] = globalPos
		summary.TotalValue = summary.TotalValue.Add(globalPos.TotalValue)
	}
	
	// Build account summaries
	for accountID, positions := range pm.positions {
		accountSummary := &AccountSummary{
			AccountID:         accountID,
			TotalValue:        decimal.Zero,
			PositionsBySymbol: positions,
		}
		
		// Calculate account total value
		for _, pos := range positions {
			positionValue := pos.Quantity.Mul(pos.MarkPrice)
			accountSummary.TotalValue = accountSummary.TotalValue.Add(positionValue)
		}
		
		summary.AccountSummaries[accountID] = accountSummary
	}
	
	return summary
}

// ClosePosition closes a position
func (pm *MultiAccountPositionManager) ClosePosition(accountID, symbol string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	if positions, exists := pm.positions[accountID]; exists {
		if _, exists := positions[symbol]; exists {
			delete(positions, symbol)
			
			// Update global position
			pm.updateGlobalPosition(symbol)
			
			// Update account summary
			pm.updateAccountSummary(accountID)
			
			return nil
		}
	}
	
	return fmt.Errorf("position not found: %s %s", accountID, symbol)
}

// updateWorker processes position updates
func (pm *MultiAccountPositionManager) updateWorker(ctx context.Context) {
	defer pm.wg.Done()
	
	ticker := time.NewTicker(pm.config.UpdateInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-pm.stopCh:
			return
		case <-ticker.C:
			pm.refreshPositions()
		}
	}
}

// monitorWorker monitors positions for alerts
func (pm *MultiAccountPositionManager) monitorWorker(ctx context.Context) {
	defer pm.wg.Done()
	
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-pm.stopCh:
			return
		case <-ticker.C:
			pm.checkPositionAlerts()
		}
	}
}

// refreshPositions refreshes all positions
func (pm *MultiAccountPositionManager) refreshPositions() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	// Update all account summaries
	for accountID := range pm.positions {
		pm.updateAccountSummary(accountID)
	}
}

// checkPositionAlerts checks for position alerts
func (pm *MultiAccountPositionManager) checkPositionAlerts() {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	// Check each position
	for accountID, positions := range pm.positions {
		for symbol, pos := range positions {
			// Check stop loss
			if !pos.StopLoss.IsZero() && pos.MarkPrice.LessThanOrEqual(pos.StopLoss) {
				pm.sendAlert(accountID, symbol, "stop_loss", "Stop loss triggered", "high", pos.MarkPrice)
			}
			
			// Check max loss
			if !pos.MaxLoss.IsZero() && pos.UnrealizedPnL.Abs().GreaterThan(pos.MaxLoss) {
				pm.sendAlert(accountID, symbol, "max_loss", "Max loss exceeded", "critical", pos.UnrealizedPnL.Abs())
			}
			
			// Check margin level
			if summary, exists := pm.accountSummary[accountID]; exists {
				if summary.MarginLevel.LessThan(decimal.NewFromFloat(1.5)) {
					pm.sendAlert(accountID, symbol, "margin_call", "Low margin level", "high", summary.MarginLevel)
				}
			}
		}
	}
}

// updateGlobalPosition updates global position for a symbol
func (pm *MultiAccountPositionManager) updateGlobalPosition(symbol string) {
	global, exists := pm.globalPositions[symbol]
	if !exists {
		global = &GlobalPosition{
			Symbol:   symbol,
			Accounts: make(map[string]*MultiAccountPosition),
		}
		pm.globalPositions[symbol] = global
	}
	
	// Reset values
	global.TotalLong = decimal.Zero
	global.TotalShort = decimal.Zero
	global.TotalValue = decimal.Zero
	global.TotalPnL = decimal.Zero
	global.AccountCount = 0
	
	// Aggregate positions
	for accountID, positions := range pm.positions {
		if pos, exists := positions[symbol]; exists {
			global.Accounts[accountID] = pos
			global.AccountCount++
			
			if pos.Side == types.PositionSideLong {
				global.TotalLong = global.TotalLong.Add(pos.Quantity)
			} else {
				global.TotalShort = global.TotalShort.Add(pos.Quantity)
			}
			
			global.TotalValue = global.TotalValue.Add(pos.PositionValue)
			global.TotalPnL = global.TotalPnL.Add(pos.UnrealizedPnL)
		}
	}
	
	// Calculate net position
	global.NetPosition = global.TotalLong.Sub(global.TotalShort)
	
	// Calculate average price
	if !global.NetPosition.IsZero() {
		global.AveragePrice = global.TotalValue.Div(global.NetPosition.Abs())
	}
}

// updateAccountSummary updates account summary
func (pm *MultiAccountPositionManager) updateAccountSummary(accountID string) {
	summary, exists := pm.accountSummary[accountID]
	if !exists {
		summary = &AccountSummary{
			AccountID: accountID,
		}
		pm.accountSummary[accountID] = summary
	}
	
	// Reset values
	summary.TotalPositions = 0
	summary.TotalValue = decimal.Zero
	summary.TotalPnL = decimal.Zero
	summary.TotalMargin = decimal.Zero
	
	// Aggregate account positions
	if positions, exists := pm.positions[accountID]; exists {
		for _, pos := range positions {
			summary.TotalPositions++
			summary.TotalValue = summary.TotalValue.Add(pos.PositionValue)
			summary.TotalPnL = summary.TotalPnL.Add(pos.UnrealizedPnL)
			summary.TotalMargin = summary.TotalMargin.Add(pos.Margin)
		}
	}
	
	// Calculate margin level
	if !summary.TotalMargin.IsZero() {
		equity := summary.TotalValue.Add(summary.TotalPnL)
		summary.MarginLevel = equity.Div(summary.TotalMargin)
	}
	
	summary.UpdateTime = time.Now()
}

// sendAlert sends an alert
func (pm *MultiAccountPositionManager) sendAlert(accountID, symbol, alertType, message, severity string, value decimal.Decimal) {
	alert := PositionAlert{
		AccountID: accountID,
		Symbol:    symbol,
		AlertType: alertType,
		Message:   message,
		Severity:  severity,
		Value:     value,
		Timestamp: time.Now(),
	}
	
	select {
	case pm.alertChan <- alert:
	default:
	}
}

// GetAlertChannel returns the alert channel
func (pm *MultiAccountPositionManager) GetAlertChannel() <-chan PositionAlert {
	return pm.alertChan
}

// GetUpdateChannel returns the update channel
func (pm *MultiAccountPositionManager) GetUpdateChannel() <-chan PositionUpdate {
	return pm.updateChan
}