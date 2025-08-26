package position

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/mExOms/pkg/types"
)

// PnLSnapshot represents P&L at a specific time
type PnLSnapshot struct {
	Timestamp    time.Time          `json:"timestamp"`
	UnrealizedPL float64            `json:"unrealized_pl"`
	RealizedPL   float64            `json:"realized_pl"`
	TotalPL      float64            `json:"total_pl"`
	Fees         float64            `json:"fees"`
	Funding      float64            `json:"funding"` // Futures funding fees
	ByAccount    map[string]float64 `json:"by_account"`
	ByStrategy   map[string]float64 `json:"by_strategy"`
	BySymbol     map[string]float64 `json:"by_symbol"`
}

// AccountPnL tracks P&L for a single account
type AccountPnL struct {
	AccountID       string             `json:"account_id"`
	UnrealizedPL    float64            `json:"unrealized_pl"`
	RealizedPL      float64            `json:"realized_pl"`
	DailyPL         float64            `json:"daily_pl"`
	WeeklyPL        float64            `json:"weekly_pl"`
	MonthlyPL       float64            `json:"monthly_pl"`
	Fees            float64            `json:"fees"`
	Funding         float64            `json:"funding"`
	MaxDrawdown     float64            `json:"max_drawdown"`
	SharpeRatio     float64            `json:"sharpe_ratio"`
	WinRate         float64            `json:"win_rate"`
	ProfitFactor    float64            `json:"profit_factor"`
	PositionPnL     map[string]float64 `json:"position_pnl"` // symbol -> P&L
	LastUpdate      time.Time          `json:"last_update"`
}

// StrategyPnL tracks P&L for a strategy across accounts
type StrategyPnL struct {
	StrategyID      string             `json:"strategy_id"`
	UnrealizedPL    float64            `json:"unrealized_pl"`
	RealizedPL      float64            `json:"realized_pl"`
	TotalPL         float64            `json:"total_pl"`
	Fees            float64            `json:"fees"`
	AccountsPnL     map[string]float64 `json:"accounts_pnl"` // accountID -> P&L
	ROI             float64            `json:"roi"`          // Return on Investment
	SharpeRatio     float64            `json:"sharpe_ratio"`
	MaxDrawdown     float64            `json:"max_drawdown"`
	WinningTrades   int                `json:"winning_trades"`
	LosingTrades    int                `json:"losing_trades"`
	LastUpdate      time.Time          `json:"last_update"`
}

// Trade represents a completed trade for P&L calculation
type Trade struct {
	AccountID    string    `json:"account_id"`
	Symbol       string    `json:"symbol"`
	Side         string    `json:"side"` // "BUY" or "SELL"
	Quantity     float64   `json:"quantity"`
	EntryPrice   float64   `json:"entry_price"`
	ExitPrice    float64   `json:"exit_price"`
	RealizedPL   float64   `json:"realized_pl"`
	Fee          float64   `json:"fee"`
	Timestamp    time.Time `json:"timestamp"`
}

// PnLCalculator calculates P&L across accounts and strategies
type PnLCalculator struct {
	positionMgr *IntegratedPositionManager
	
	// P&L tracking
	accountPnL   sync.Map // accountID -> *AccountPnL
	strategyPnL  sync.Map // strategyID -> *StrategyPnL
	
	// Trade history
	trades       []Trade
	tradesMu     sync.RWMutex
	
	// P&L snapshots
	snapshots    []*PnLSnapshot
	snapshotsMu  sync.RWMutex
	maxSnapshots int
	
	// Price feeds for mark-to-market
	markPrices   sync.Map // symbol -> float64
	
	// Metrics calculation
	riskFreeRate float64 // For Sharpe ratio calculation
	
	// Control
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	
	// Callbacks
	onPnLUpdate  func(*PnLSnapshot)
	onDrawdown   func(accountID string, drawdown float64)
}

// NewPnLCalculator creates a new P&L calculator
func NewPnLCalculator(positionMgr *IntegratedPositionManager) *PnLCalculator {
	ctx, cancel := context.WithCancel(context.Background())
	
	calc := &PnLCalculator{
		positionMgr:  positionMgr,
		maxSnapshots: 10000,
		riskFreeRate: 0.02, // 2% annual
		ctx:          ctx,
		cancel:       cancel,
	}
	
	// Start P&L updater
	calc.wg.Add(1)
	go calc.pnlUpdater()
	
	return calc
}

// UpdateMarkPrice updates mark price for a symbol
func (calc *PnLCalculator) UpdateMarkPrice(symbol string, price float64) {
	calc.markPrices.Store(symbol, price)
	
	// Trigger P&L recalculation for positions with this symbol
	calc.recalculateUnrealizedPnL(symbol)
}

// RecordTrade records a completed trade
func (calc *PnLCalculator) RecordTrade(trade Trade) {
	calc.tradesMu.Lock()
	calc.trades = append(calc.trades, trade)
	calc.tradesMu.Unlock()
	
	// Update account realized P&L
	calc.updateAccountRealizedPnL(trade)
	
	// Update strategy P&L if position is assigned to one
	// This would require position-strategy mapping
}

// GetAccountPnL returns P&L for a specific account
func (calc *PnLCalculator) GetAccountPnL(accountID string) (*AccountPnL, error) {
	if pnl, ok := calc.accountPnL.Load(accountID); ok {
		return pnl.(*AccountPnL), nil
	}
	return nil, fmt.Errorf("P&L not found for account %s", accountID)
}

// GetStrategyPnL returns P&L for a specific strategy
func (calc *PnLCalculator) GetStrategyPnL(strategyID string) (*StrategyPnL, error) {
	if pnl, ok := calc.strategyPnL.Load(strategyID); ok {
		return pnl.(*StrategyPnL), nil
	}
	return nil, fmt.Errorf("P&L not found for strategy %s", strategyID)
}

// GetGlobalPnL returns combined P&L across all accounts
func (calc *PnLCalculator) GetGlobalPnL() *PnLSnapshot {
	snapshot := &PnLSnapshot{
		Timestamp:  time.Now(),
		ByAccount:  make(map[string]float64),
		ByStrategy: make(map[string]float64),
		BySymbol:   make(map[string]float64),
	}
	
	// Aggregate from all accounts
	calc.accountPnL.Range(func(key, value interface{}) bool {
		accountID := key.(string)
		pnl := value.(*AccountPnL)
		
		totalPL := pnl.UnrealizedPL + pnl.RealizedPL
		snapshot.UnrealizedPL += pnl.UnrealizedPL
		snapshot.RealizedPL += pnl.RealizedPL
		snapshot.TotalPL += totalPL
		snapshot.Fees += pnl.Fees
		snapshot.Funding += pnl.Funding
		snapshot.ByAccount[accountID] = totalPL
		
		// Aggregate by symbol
		for symbol, symbolPL := range pnl.PositionPnL {
			snapshot.BySymbol[symbol] += symbolPL
		}
		
		return true
	})
	
	// Aggregate from strategies
	calc.strategyPnL.Range(func(key, value interface{}) bool {
		strategyID := key.(string)
		pnl := value.(*StrategyPnL)
		snapshot.ByStrategy[strategyID] = pnl.TotalPL
		return true
	})
	
	return snapshot
}

// GetPnLHistory returns historical P&L snapshots
func (calc *PnLCalculator) GetPnLHistory(limit int) []*PnLSnapshot {
	calc.snapshotsMu.RLock()
	defer calc.snapshotsMu.RUnlock()
	
	if limit > len(calc.snapshots) || limit <= 0 {
		limit = len(calc.snapshots)
	}
	
	// Return last 'limit' snapshots
	start := len(calc.snapshots) - limit
	if start < 0 {
		start = 0
	}
	
	result := make([]*PnLSnapshot, limit)
	copy(result, calc.snapshots[start:])
	
	return result
}

// recalculateUnrealizedPnL recalculates unrealized P&L for a symbol
func (calc *PnLCalculator) recalculateUnrealizedPnL(symbol string) {
	markPrice, ok := calc.markPrices.Load(symbol)
	if !ok {
		return
	}
	
	price := markPrice.(float64)
	
	// Update all account positions with this symbol
	calc.positionMgr.accounts.Range(func(key, value interface{}) bool {
		accountID := key.(string)
		account := value.(*AccountPosition)
		
		account.mu.RLock()
		if pos, exists := account.Positions[symbol]; exists {
			// Calculate unrealized P&L
			var unrealizedPL float64
			if pos.Quantity > 0 {
				// Long position
				unrealizedPL = (price - pos.AvgPrice) * pos.Quantity
			} else {
				// Short position
				unrealizedPL = (pos.AvgPrice - price) * math.Abs(pos.Quantity)
			}
			
			// Update position P&L
			pos.UnrealizedPL = unrealizedPL
			pos.MarkPrice = price
			
			// Update account P&L
			calc.updateAccountUnrealizedPnL(accountID)
		}
		account.mu.RUnlock()
		
		return true
	})
}

// updateAccountRealizedPnL updates realized P&L for an account
func (calc *PnLCalculator) updateAccountRealizedPnL(trade Trade) {
	// Get or create account P&L
	var pnl *AccountPnL
	if p, ok := calc.accountPnL.Load(trade.AccountID); ok {
		pnl = p.(*AccountPnL)
	} else {
		pnl = &AccountPnL{
			AccountID:   trade.AccountID,
			PositionPnL: make(map[string]float64),
			LastUpdate:  time.Now(),
		}
		calc.accountPnL.Store(trade.AccountID, pnl)
	}
	
	// Update realized P&L
	pnl.RealizedPL += trade.RealizedPL
	pnl.Fees += trade.Fee
	pnl.DailyPL += trade.RealizedPL // Would need date tracking for accurate daily
	
	// Update position P&L
	pnl.PositionPnL[trade.Symbol] += trade.RealizedPL
	
	// Update win rate
	if trade.RealizedPL > 0 {
		// This is simplified - would need to track wins/losses properly
		pnl.WinRate = (pnl.WinRate + 1) / 2 // Placeholder calculation
	}
	
	pnl.LastUpdate = time.Now()
}

// updateAccountUnrealizedPnL recalculates total unrealized P&L for an account
func (calc *PnLCalculator) updateAccountUnrealizedPnL(accountID string) {
	account, err := calc.positionMgr.GetAccountPositions(accountID)
	if err != nil {
		return
	}
	
	// Get or create account P&L
	var pnl *AccountPnL
	if p, ok := calc.accountPnL.Load(accountID); ok {
		pnl = p.(*AccountPnL)
	} else {
		pnl = &AccountPnL{
			AccountID:   accountID,
			PositionPnL: make(map[string]float64),
			LastUpdate:  time.Now(),
		}
		calc.accountPnL.Store(accountID, pnl)
	}
	
	// Sum unrealized P&L from all positions
	totalUnrealized := 0.0
	for symbol, pos := range account.Positions {
		totalUnrealized += pos.UnrealizedPL
		pnl.PositionPnL[symbol] = pos.UnrealizedPL + pos.RealizedPL
	}
	
	pnl.UnrealizedPL = totalUnrealized
	pnl.LastUpdate = time.Now()
	
	// Check drawdown
	calc.checkDrawdown(accountID, pnl)
}

// calculateMetrics calculates performance metrics
func (calc *PnLCalculator) calculateMetrics() {
	// Calculate metrics for each account
	calc.accountPnL.Range(func(key, value interface{}) bool {
		accountID := key.(string)
		pnl := value.(*AccountPnL)
		
		// Get historical P&L for calculations
		history := calc.getAccountPnLHistory(accountID, 252) // 252 trading days
		
		if len(history) > 30 { // Need sufficient data
			// Calculate Sharpe ratio
			pnl.SharpeRatio = calc.calculateSharpeRatio(history)
			
			// Calculate max drawdown
			pnl.MaxDrawdown = calc.calculateMaxDrawdown(history)
			
			// Calculate profit factor
			pnl.ProfitFactor = calc.calculateProfitFactor(accountID)
		}
		
		return true
	})
	
	// Calculate metrics for strategies
	calc.strategyPnL.Range(func(key, value interface{}) bool {
		strategyID := key.(string)
		pnl := value.(*StrategyPnL)
		
		// Get strategy positions
		if strategy, err := calc.positionMgr.GetStrategyPositions(strategyID); err == nil {
			// Update P&L from accounts
			totalUnrealized := 0.0
			totalRealized := 0.0
			
			for accountID := range strategy.Accounts {
				if accPnL, ok := calc.accountPnL.Load(accountID); ok {
					ap := accPnL.(*AccountPnL)
					totalUnrealized += ap.UnrealizedPL
					totalRealized += ap.RealizedPL
					pnl.AccountsPnL[accountID] = ap.UnrealizedPL + ap.RealizedPL
				}
			}
			
			pnl.UnrealizedPL = totalUnrealized
			pnl.RealizedPL = totalRealized
			pnl.TotalPL = totalUnrealized + totalRealized
		}
		
		return true
	})
}

// calculateSharpeRatio calculates the Sharpe ratio from P&L history
func (calc *PnLCalculator) calculateSharpeRatio(pnlHistory []float64) float64 {
	if len(pnlHistory) < 2 {
		return 0
	}
	
	// Calculate returns
	returns := make([]float64, len(pnlHistory)-1)
	for i := 1; i < len(pnlHistory); i++ {
		if pnlHistory[i-1] != 0 {
			returns[i-1] = (pnlHistory[i] - pnlHistory[i-1]) / pnlHistory[i-1]
		}
	}
	
	// Calculate mean and std dev
	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))
	
	variance := 0.0
	for _, r := range returns {
		variance += (r - mean) * (r - mean)
	}
	variance /= float64(len(returns) - 1)
	stdDev := math.Sqrt(variance)
	
	if stdDev == 0 {
		return 0
	}
	
	// Annualized Sharpe ratio
	annualizedReturn := mean * 252
	annualizedStdDev := stdDev * math.Sqrt(252)
	
	return (annualizedReturn - calc.riskFreeRate) / annualizedStdDev
}

// calculateMaxDrawdown calculates maximum drawdown from P&L history
func (calc *PnLCalculator) calculateMaxDrawdown(pnlHistory []float64) float64 {
	if len(pnlHistory) == 0 {
		return 0
	}
	
	maxDrawdown := 0.0
	peak := pnlHistory[0]
	
	for _, pnl := range pnlHistory {
		if pnl > peak {
			peak = pnl
		}
		
		drawdown := 0.0
		if peak > 0 {
			drawdown = (peak - pnl) / peak
		}
		
		if drawdown > maxDrawdown {
			maxDrawdown = drawdown
		}
	}
	
	return maxDrawdown
}

// calculateProfitFactor calculates profit factor (gross profit / gross loss)
func (calc *PnLCalculator) calculateProfitFactor(accountID string) float64 {
	grossProfit := 0.0
	grossLoss := 0.0
	
	calc.tradesMu.RLock()
	for _, trade := range calc.trades {
		if trade.AccountID == accountID {
			if trade.RealizedPL > 0 {
				grossProfit += trade.RealizedPL
			} else {
				grossLoss += math.Abs(trade.RealizedPL)
			}
		}
	}
	calc.tradesMu.RUnlock()
	
	if grossLoss == 0 {
		return 0
	}
	
	return grossProfit / grossLoss
}

// checkDrawdown checks for drawdown alerts
func (calc *PnLCalculator) checkDrawdown(accountID string, pnl *AccountPnL) {
	// Simple drawdown check - would need historical high water mark
	totalPL := pnl.UnrealizedPL + pnl.RealizedPL
	if totalPL < 0 {
		drawdown := math.Abs(totalPL) // Simplified
		if calc.onDrawdown != nil && drawdown > pnl.MaxDrawdown*0.8 {
			calc.onDrawdown(accountID, drawdown)
		}
	}
}

// getAccountPnLHistory returns historical P&L for an account
func (calc *PnLCalculator) getAccountPnLHistory(accountID string, days int) []float64 {
	// This is a placeholder - would need actual historical data storage
	history := make([]float64, 0, days)
	
	calc.snapshotsMu.RLock()
	for _, snapshot := range calc.snapshots {
		if pnl, exists := snapshot.ByAccount[accountID]; exists {
			history = append(history, pnl)
		}
	}
	calc.snapshotsMu.RUnlock()
	
	return history
}

// pnlUpdater periodically updates P&L calculations
func (calc *PnLCalculator) pnlUpdater() {
	defer calc.wg.Done()
	
	ticker := time.NewTicker(1 * time.Second) // Update every second
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// Take snapshot
			snapshot := calc.GetGlobalPnL()
			
			// Store snapshot
			calc.snapshotsMu.Lock()
			calc.snapshots = append(calc.snapshots, snapshot)
			if len(calc.snapshots) > calc.maxSnapshots {
				calc.snapshots = calc.snapshots[1:]
			}
			calc.snapshotsMu.Unlock()
			
			// Calculate metrics
			calc.calculateMetrics()
			
			// Trigger callback
			if calc.onPnLUpdate != nil {
				calc.onPnLUpdate(snapshot)
			}
			
		case <-calc.ctx.Done():
			return
		}
	}
}

// SetPnLUpdateCallback sets callback for P&L updates
func (calc *PnLCalculator) SetPnLUpdateCallback(callback func(*PnLSnapshot)) {
	calc.onPnLUpdate = callback
}

// SetDrawdownCallback sets callback for drawdown alerts
func (calc *PnLCalculator) SetDrawdownCallback(callback func(accountID string, drawdown float64)) {
	calc.onDrawdown = callback
}

// Stop stops the P&L calculator
func (calc *PnLCalculator) Stop() {
	calc.cancel()
	calc.wg.Wait()
}