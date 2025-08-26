package position

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mExOms/pkg/types"
)

// AccountPosition represents position state for a single account
type AccountPosition struct {
	AccountID    string                      `json:"account_id"`
	Exchange     string                      `json:"exchange"`
	Positions    map[string]*types.Position  `json:"positions"`     // symbol -> position
	TotalValue   float64                     `json:"total_value"`   // Total position value in USD
	UnrealizedPL float64                     `json:"unrealized_pl"` // Total unrealized P&L
	RealizedPL   float64                     `json:"realized_pl"`   // Daily realized P&L
	Margin       float64                     `json:"margin_used"`   // Total margin used
	LastUpdate   time.Time                   `json:"last_update"`
	mu           sync.RWMutex
}

// StrategyPositions groups positions by strategy
type StrategyPositions struct {
	StrategyID   string                         `json:"strategy_id"`
	Accounts     map[string]*AccountPosition    `json:"accounts"`     // accountID -> positions
	NetPositions map[string]*AggregatedPosition `json:"net_positions"` // symbol -> net position
	TotalValue   float64                        `json:"total_value"`
	UnrealizedPL float64                        `json:"unrealized_pl"`
	RealizedPL   float64                        `json:"realized_pl"`
	LastUpdate   time.Time                      `json:"last_update"`
	mu           sync.RWMutex
}

// AggregatedPosition represents net position across accounts
type AggregatedPosition struct {
	Symbol       string    `json:"symbol"`
	NetQuantity  float64   `json:"net_quantity"`
	TotalValue   float64   `json:"total_value"`
	AvgPrice     float64   `json:"avg_price"`
	UnrealizedPL float64   `json:"unrealized_pl"`
	RealizedPL   float64   `json:"realized_pl"`
	LongQty      float64   `json:"long_qty"`
	ShortQty     float64   `json:"short_qty"`
	Accounts     []string  `json:"accounts"` // List of accounts holding this position
	LastUpdate   time.Time `json:"last_update"`
}

// PositionUpdate represents a position change event
type PositionUpdate struct {
	AccountID  string          `json:"account_id"`
	Exchange   string          `json:"exchange"`
	Symbol     string          `json:"symbol"`
	Position   *types.Position `json:"position"`
	UpdateType string          `json:"update_type"` // "open", "update", "close"
	Timestamp  time.Time       `json:"timestamp"`
}

// IntegratedPositionManager manages positions across multiple accounts and strategies
type IntegratedPositionManager struct {
	// Account positions by accountID
	accounts sync.Map // accountID -> *AccountPosition

	// Strategy positions by strategyID
	strategies sync.Map // strategyID -> *StrategyPositions

	// Global aggregated positions
	globalPositions sync.Map // symbol -> *AggregatedPosition

	// Position update channel
	updateChan chan *PositionUpdate

	// Callbacks
	onPositionUpdate    func(*PositionUpdate)
	onRiskLimitReached  func(accountID string, reason string)
	onHedgeOpportunity  func(symbol string, accounts []string)

	// Metrics
	totalAccounts  int32
	totalPositions int32
	lastUpdateTime atomic.Value // time.Time

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Shared memory for high-performance updates (simulated)
	sharedMem *SharedMemoryPositions
}

// SharedMemoryPositions simulates shared memory structure for ultra-fast updates
type SharedMemoryPositions struct {
	positions map[string]*SharedPosition
	mu        sync.RWMutex
}

// SharedPosition represents position in shared memory
type SharedPosition struct {
	AccountID    string
	Symbol       string
	Quantity     float64
	Value        float64
	UnrealizedPL float64
	LastUpdate   int64 // Unix nano
}

// NewIntegratedPositionManager creates a new position manager
func NewIntegratedPositionManager() *IntegratedPositionManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	ipm := &IntegratedPositionManager{
		updateChan: make(chan *PositionUpdate, 1000),
		ctx:        ctx,
		cancel:     cancel,
		sharedMem: &SharedMemoryPositions{
			positions: make(map[string]*SharedPosition),
		},
	}
	
	ipm.lastUpdateTime.Store(time.Now())
	
	// Start position processor
	ipm.wg.Add(1)
	go ipm.processUpdates()
	
	// Start aggregator
	ipm.wg.Add(1)
	go ipm.aggregatePositions()
	
	return ipm
}

// AddAccount registers a new account
func (ipm *IntegratedPositionManager) AddAccount(accountID, exchange string) error {
	account := &AccountPosition{
		AccountID:  accountID,
		Exchange:   exchange,
		Positions:  make(map[string]*types.Position),
		LastUpdate: time.Now(),
	}
	
	ipm.accounts.Store(accountID, account)
	atomic.AddInt32(&ipm.totalAccounts, 1)
	
	fmt.Printf("Added account %s for exchange %s\n", accountID, exchange)
	return nil
}

// UpdatePosition updates or creates a position for an account
func (ipm *IntegratedPositionManager) UpdatePosition(accountID, exchange, symbol string, pos *types.Position) error {
	// Send update to processing channel
	update := &PositionUpdate{
		AccountID:  accountID,
		Exchange:   exchange,
		Symbol:     symbol,
		Position:   pos,
		UpdateType: "update",
		Timestamp:  time.Now(),
	}
	
	if pos.Quantity == 0 {
		update.UpdateType = "close"
	} else {
		// Check if this is a new position
		if account, ok := ipm.accounts.Load(accountID); ok {
			if accPos := account.(*AccountPosition); accPos != nil {
				accPos.mu.RLock()
				if _, exists := accPos.Positions[symbol]; !exists {
					update.UpdateType = "open"
				}
				accPos.mu.RUnlock()
			}
		}
	}
	
	select {
	case ipm.updateChan <- update:
		return nil
	case <-ipm.ctx.Done():
		return fmt.Errorf("position manager stopped")
	default:
		return fmt.Errorf("update channel full")
	}
}

// GetAccountPositions returns all positions for an account
func (ipm *IntegratedPositionManager) GetAccountPositions(accountID string) (*AccountPosition, error) {
	if account, ok := ipm.accounts.Load(accountID); ok {
		return account.(*AccountPosition).copy(), nil
	}
	return nil, fmt.Errorf("account %s not found", accountID)
}

// GetGlobalPosition returns aggregated position for a symbol
func (ipm *IntegratedPositionManager) GetGlobalPosition(symbol string) (*AggregatedPosition, error) {
	if pos, ok := ipm.globalPositions.Load(symbol); ok {
		return pos.(*AggregatedPosition), nil
	}
	return nil, fmt.Errorf("no global position for %s", symbol)
}

// GetStrategyPositions returns positions for a strategy
func (ipm *IntegratedPositionManager) GetStrategyPositions(strategyID string) (*StrategyPositions, error) {
	if strategy, ok := ipm.strategies.Load(strategyID); ok {
		return strategy.(*StrategyPositions), nil
	}
	return nil, fmt.Errorf("strategy %s not found", strategyID)
}

// AssignPositionToStrategy assigns a position to a strategy
func (ipm *IntegratedPositionManager) AssignPositionToStrategy(accountID, symbol, strategyID string) error {
	// Get or create strategy positions
	var strategy *StrategyPositions
	if s, ok := ipm.strategies.Load(strategyID); ok {
		strategy = s.(*StrategyPositions)
	} else {
		strategy = &StrategyPositions{
			StrategyID:   strategyID,
			Accounts:     make(map[string]*AccountPosition),
			NetPositions: make(map[string]*AggregatedPosition),
			LastUpdate:   time.Now(),
		}
		ipm.strategies.Store(strategyID, strategy)
	}
	
	// Add account to strategy
	if account, ok := ipm.accounts.Load(accountID); ok {
		strategy.mu.Lock()
		strategy.Accounts[accountID] = account.(*AccountPosition)
		strategy.mu.Unlock()
		
		fmt.Printf("Assigned position %s in account %s to strategy %s\n", symbol, accountID, strategyID)
		return nil
	}
	
	return fmt.Errorf("account %s not found", accountID)
}

// GetAccountsPnL returns P&L summary for all accounts
func (ipm *IntegratedPositionManager) GetAccountsPnL() map[string]map[string]float64 {
	pnlSummary := make(map[string]map[string]float64)
	
	ipm.accounts.Range(func(key, value interface{}) bool {
		accountID := key.(string)
		account := value.(*AccountPosition)
		
		account.mu.RLock()
		pnlSummary[accountID] = map[string]float64{
			"unrealized_pl": account.UnrealizedPL,
			"realized_pl":   account.RealizedPL,
			"total_pl":      account.UnrealizedPL + account.RealizedPL,
			"total_value":   account.TotalValue,
			"margin_used":   account.Margin,
		}
		account.mu.RUnlock()
		
		return true
	})
	
	return pnlSummary
}

// GetHedgedPositions returns positions that are hedged across accounts
func (ipm *IntegratedPositionManager) GetHedgedPositions() map[string]*HedgedPosition {
	hedged := make(map[string]*HedgedPosition)
	
	ipm.globalPositions.Range(func(key, value interface{}) bool {
		symbol := key.(string)
		pos := value.(*AggregatedPosition)
		
		// Check if position has both long and short
		if pos.LongQty > 0 && pos.ShortQty > 0 {
			hedged[symbol] = &HedgedPosition{
				Symbol:       symbol,
				LongQty:      pos.LongQty,
				ShortQty:     pos.ShortQty,
				NetExposure:  pos.NetQuantity,
				HedgeRatio:   pos.ShortQty / pos.LongQty,
				Accounts:     pos.Accounts,
				TotalValue:   pos.TotalValue,
			}
			
			// Trigger hedge opportunity callback if significant imbalance
			if ipm.onHedgeOpportunity != nil {
				hedgeRatio := pos.ShortQty / pos.LongQty
				if hedgeRatio < 0.8 || hedgeRatio > 1.2 {
					ipm.onHedgeOpportunity(symbol, pos.Accounts)
				}
			}
		}
		
		return true
	})
	
	return hedged
}

// HedgedPosition represents a position hedged across accounts
type HedgedPosition struct {
	Symbol      string   `json:"symbol"`
	LongQty     float64  `json:"long_qty"`
	ShortQty    float64  `json:"short_qty"`
	NetExposure float64  `json:"net_exposure"`
	HedgeRatio  float64  `json:"hedge_ratio"` // short/long
	Accounts    []string `json:"accounts"`
	TotalValue  float64  `json:"total_value"`
}

// processUpdates processes position updates
func (ipm *IntegratedPositionManager) processUpdates() {
	defer ipm.wg.Done()
	
	for {
		select {
		case update := <-ipm.updateChan:
			ipm.handleUpdate(update)
			
		case <-ipm.ctx.Done():
			return
		}
	}
}

// handleUpdate processes a single position update
func (ipm *IntegratedPositionManager) handleUpdate(update *PositionUpdate) {
	// Get or create account
	var account *AccountPosition
	if acc, ok := ipm.accounts.Load(update.AccountID); ok {
		account = acc.(*AccountPosition)
	} else {
		// Auto-create account if not exists
		account = &AccountPosition{
			AccountID:  update.AccountID,
			Exchange:   update.Exchange,
			Positions:  make(map[string]*types.Position),
			LastUpdate: time.Now(),
		}
		ipm.accounts.Store(update.AccountID, account)
	}
	
	account.mu.Lock()
	defer account.mu.Unlock()
	
	// Update position
	switch update.UpdateType {
	case "open":
		account.Positions[update.Symbol] = update.Position
		atomic.AddInt32(&ipm.totalPositions, 1)
		
	case "update":
		account.Positions[update.Symbol] = update.Position
		
	case "close":
		delete(account.Positions, update.Symbol)
		atomic.AddInt32(&ipm.totalPositions, -1)
	}
	
	// Recalculate account metrics
	ipm.recalculateAccountMetrics(account)
	
	// Update shared memory
	ipm.updateSharedMemory(update)
	
	// Trigger callback
	if ipm.onPositionUpdate != nil {
		ipm.onPositionUpdate(update)
	}
	
	// Update last update time
	ipm.lastUpdateTime.Store(time.Now())
}

// recalculateAccountMetrics recalculates account-level metrics
func (ipm *IntegratedPositionManager) recalculateAccountMetrics(account *AccountPosition) {
	totalValue := 0.0
	unrealizedPL := 0.0
	marginUsed := 0.0
	
	for _, pos := range account.Positions {
		totalValue += pos.Value
		unrealizedPL += pos.UnrealizedPL
		marginUsed += pos.MarginUsed
	}
	
	account.TotalValue = totalValue
	account.UnrealizedPL = unrealizedPL
	account.Margin = marginUsed
	account.LastUpdate = time.Now()
}

// updateSharedMemory updates the shared memory representation
func (ipm *IntegratedPositionManager) updateSharedMemory(update *PositionUpdate) {
	key := fmt.Sprintf("%s:%s", update.AccountID, update.Symbol)
	
	ipm.sharedMem.mu.Lock()
	defer ipm.sharedMem.mu.Unlock()
	
	if update.UpdateType == "close" {
		delete(ipm.sharedMem.positions, key)
	} else {
		ipm.sharedMem.positions[key] = &SharedPosition{
			AccountID:    update.AccountID,
			Symbol:       update.Symbol,
			Quantity:     update.Position.Quantity,
			Value:        update.Position.Value,
			UnrealizedPL: update.Position.UnrealizedPL,
			LastUpdate:   time.Now().UnixNano(),
		}
	}
}

// aggregatePositions periodically aggregates positions
func (ipm *IntegratedPositionManager) aggregatePositions() {
	defer ipm.wg.Done()
	
	ticker := time.NewTicker(100 * time.Millisecond) // High frequency aggregation
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			ipm.performAggregation()
			
		case <-ipm.ctx.Done():
			return
		}
	}
}

// performAggregation aggregates positions across accounts
func (ipm *IntegratedPositionManager) performAggregation() {
	// Map to hold aggregated positions by symbol
	aggregated := make(map[string]*AggregatedPosition)
	
	// Iterate through all accounts
	ipm.accounts.Range(func(key, value interface{}) bool {
		account := value.(*AccountPosition)
		
		account.mu.RLock()
		for symbol, pos := range account.Positions {
			if agg, exists := aggregated[symbol]; exists {
				// Update existing aggregated position
				if pos.Quantity > 0 {
					agg.LongQty += pos.Quantity
				} else {
					agg.ShortQty += -pos.Quantity
				}
				agg.NetQuantity += pos.Quantity
				agg.TotalValue += pos.Value
				agg.UnrealizedPL += pos.UnrealizedPL
				agg.RealizedPL += pos.RealizedPL
				agg.Accounts = append(agg.Accounts, account.AccountID)
			} else {
				// Create new aggregated position
				agg := &AggregatedPosition{
					Symbol:       symbol,
					NetQuantity:  pos.Quantity,
					TotalValue:   pos.Value,
					UnrealizedPL: pos.UnrealizedPL,
					RealizedPL:   pos.RealizedPL,
					Accounts:     []string{account.AccountID},
					LastUpdate:   time.Now(),
				}
				if pos.Quantity > 0 {
					agg.LongQty = pos.Quantity
				} else {
					agg.ShortQty = -pos.Quantity
				}
				aggregated[symbol] = agg
			}
		}
		account.mu.RUnlock()
		
		return true
	})
	
	// Update global positions
	for symbol, agg := range aggregated {
		// Calculate average price
		if agg.NetQuantity != 0 {
			agg.AvgPrice = agg.TotalValue / agg.NetQuantity
		}
		ipm.globalPositions.Store(symbol, agg)
	}
	
	// Also update strategy aggregations
	ipm.aggregateStrategyPositions()
}

// aggregateStrategyPositions aggregates positions by strategy
func (ipm *IntegratedPositionManager) aggregateStrategyPositions() {
	ipm.strategies.Range(func(key, value interface{}) bool {
		strategy := value.(*StrategyPositions)
		
		strategy.mu.Lock()
		defer strategy.mu.Unlock()
		
		// Reset strategy metrics
		strategy.TotalValue = 0
		strategy.UnrealizedPL = 0
		strategy.RealizedPL = 0
		strategy.NetPositions = make(map[string]*AggregatedPosition)
		
		// Aggregate across strategy accounts
		for accountID, account := range strategy.Accounts {
			account.mu.RLock()
			strategy.TotalValue += account.TotalValue
			strategy.UnrealizedPL += account.UnrealizedPL
			strategy.RealizedPL += account.RealizedPL
			
			// Aggregate positions
			for symbol, pos := range account.Positions {
				if netPos, exists := strategy.NetPositions[symbol]; exists {
					netPos.NetQuantity += pos.Quantity
					netPos.TotalValue += pos.Value
					netPos.UnrealizedPL += pos.UnrealizedPL
					netPos.Accounts = append(netPos.Accounts, accountID)
				} else {
					strategy.NetPositions[symbol] = &AggregatedPosition{
						Symbol:       symbol,
						NetQuantity:  pos.Quantity,
						TotalValue:   pos.Value,
						UnrealizedPL: pos.UnrealizedPL,
						Accounts:     []string{accountID},
						LastUpdate:   time.Now(),
					}
				}
			}
			account.mu.RUnlock()
		}
		
		strategy.LastUpdate = time.Now()
		return true
	})
}

// SetPositionUpdateCallback sets the callback for position updates
func (ipm *IntegratedPositionManager) SetPositionUpdateCallback(callback func(*PositionUpdate)) {
	ipm.onPositionUpdate = callback
}

// SetRiskLimitCallback sets the callback for risk limit events
func (ipm *IntegratedPositionManager) SetRiskLimitCallback(callback func(accountID string, reason string)) {
	ipm.onRiskLimitReached = callback
}

// SetHedgeOpportunityCallback sets the callback for hedge opportunities
func (ipm *IntegratedPositionManager) SetHedgeOpportunityCallback(callback func(symbol string, accounts []string)) {
	ipm.onHedgeOpportunity = callback
}

// GetMetrics returns manager metrics
func (ipm *IntegratedPositionManager) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"total_accounts":    atomic.LoadInt32(&ipm.totalAccounts),
		"total_positions":   atomic.LoadInt32(&ipm.totalPositions),
		"last_update":       ipm.lastUpdateTime.Load().(time.Time),
		"update_queue_size": len(ipm.updateChan),
	}
}

// Stop stops the position manager
func (ipm *IntegratedPositionManager) Stop() {
	ipm.cancel()
	ipm.wg.Wait()
	close(ipm.updateChan)
	fmt.Println("Integrated position manager stopped")
}

// copy creates a copy of AccountPosition (thread-safe)
func (ap *AccountPosition) copy() *AccountPosition {
	ap.mu.RLock()
	defer ap.mu.RUnlock()
	
	positions := make(map[string]*types.Position)
	for k, v := range ap.Positions {
		positions[k] = v
	}
	
	return &AccountPosition{
		AccountID:    ap.AccountID,
		Exchange:     ap.Exchange,
		Positions:    positions,
		TotalValue:   ap.TotalValue,
		UnrealizedPL: ap.UnrealizedPL,
		RealizedPL:   ap.RealizedPL,
		Margin:       ap.Margin,
		LastUpdate:   ap.LastUpdate,
	}
}