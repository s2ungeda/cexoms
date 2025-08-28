package cache

import (
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// StrategyState holds the state of a trading strategy
type StrategyState struct {
	StrategyID   string                 `json:"strategy_id"`
	Type         string                 `json:"type"` // arbitrage, market_making, trend_following
	Status       string                 `json:"status"` // running, paused, stopped
	StartedAt    time.Time              `json:"started_at"`
	LastUpdateAt time.Time              `json:"last_update_at"`
	Parameters   map[string]interface{} `json:"parameters"`
	Metrics      StrategyMetrics        `json:"metrics"`
	Positions    map[string]Position    `json:"positions"`
	Orders       map[string]Order       `json:"orders"`
}

// StrategyMetrics holds performance metrics for a strategy
type StrategyMetrics struct {
	TotalPnL       decimal.Decimal `json:"total_pnl"`
	TodayPnL       decimal.Decimal `json:"today_pnl"`
	TotalTrades    int             `json:"total_trades"`
	WinningTrades  int             `json:"winning_trades"`
	LosingTrades   int             `json:"losing_trades"`
	WinRate        float64         `json:"win_rate"`
	AvgWin         decimal.Decimal `json:"avg_win"`
	AvgLoss        decimal.Decimal `json:"avg_loss"`
	MaxDrawdown    decimal.Decimal `json:"max_drawdown"`
	SharpeRatio    float64         `json:"sharpe_ratio"`
	LastTradeTime  time.Time       `json:"last_trade_time"`
}

// Position represents a strategy position
type Position struct {
	Symbol    string          `json:"symbol"`
	Side      string          `json:"side"`
	Quantity  decimal.Decimal `json:"quantity"`
	EntryPrice decimal.Decimal `json:"entry_price"`
	MarkPrice decimal.Decimal `json:"mark_price"`
	UnrealizedPnL decimal.Decimal `json:"unrealized_pnl"`
	OpenedAt  time.Time       `json:"opened_at"`
}

// Order represents a strategy order
type Order struct {
	OrderID   string          `json:"order_id"`
	Symbol    string          `json:"symbol"`
	Side      string          `json:"side"`
	Type      string          `json:"type"`
	Quantity  decimal.Decimal `json:"quantity"`
	Price     decimal.Decimal `json:"price"`
	Status    string          `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
}

// StrategyCache manages strategy state caching
type StrategyCache struct {
	mu         sync.RWMutex
	states     map[string]*StrategyState
	cache      *MemoryCache
	updateChan chan StrategyUpdate
}

// StrategyUpdate represents a strategy state update
type StrategyUpdate struct {
	StrategyID string
	UpdateType string // metrics, position, order, status, parameters
	Data       interface{}
}

// NewStrategyCache creates a new strategy cache
func NewStrategyCache() *StrategyCache {
	sc := &StrategyCache{
		states:     make(map[string]*StrategyState),
		cache:      NewMemoryCache(),
		updateChan: make(chan StrategyUpdate, 1000),
	}
	
	go sc.processUpdates()
	return sc
}

// GetState returns the current state of a strategy
func (sc *StrategyCache) GetState(strategyID string) (*StrategyState, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	
	state, exists := sc.states[strategyID]
	if !exists {
		// Try to load from cache
		if cached, ok := sc.cache.Get(fmt.Sprintf("strategy:%s", strategyID)); ok {
			if state, ok := cached.(*StrategyState); ok {
				return state, true
			}
		}
		return nil, false
	}
	
	return state, true
}

// SetState sets the complete state of a strategy
func (sc *StrategyCache) SetState(state *StrategyState) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	
	state.LastUpdateAt = time.Now()
	sc.states[state.StrategyID] = state
	
	// Also cache it
	sc.cache.Set(fmt.Sprintf("strategy:%s", state.StrategyID), state, 24*time.Hour)
}

// UpdateStatus updates strategy status
func (sc *StrategyCache) UpdateStatus(strategyID, status string) {
	sc.updateChan <- StrategyUpdate{
		StrategyID: strategyID,
		UpdateType: "status",
		Data:       status,
	}
}

// UpdateMetrics updates strategy metrics
func (sc *StrategyCache) UpdateMetrics(strategyID string, metrics StrategyMetrics) {
	sc.updateChan <- StrategyUpdate{
		StrategyID: strategyID,
		UpdateType: "metrics",
		Data:       metrics,
	}
}

// UpdatePosition updates a strategy position
func (sc *StrategyCache) UpdatePosition(strategyID, symbol string, position Position) {
	sc.updateChan <- StrategyUpdate{
		StrategyID: strategyID,
		UpdateType: "position",
		Data: map[string]interface{}{
			"symbol":   symbol,
			"position": position,
		},
	}
}

// UpdateOrder updates a strategy order
func (sc *StrategyCache) UpdateOrder(strategyID, orderID string, order Order) {
	sc.updateChan <- StrategyUpdate{
		StrategyID: strategyID,
		UpdateType: "order",
		Data: map[string]interface{}{
			"order_id": orderID,
			"order":    order,
		},
	}
}

// UpdateParameters updates strategy parameters
func (sc *StrategyCache) UpdateParameters(strategyID string, parameters map[string]interface{}) {
	sc.updateChan <- StrategyUpdate{
		StrategyID: strategyID,
		UpdateType: "parameters",
		Data:       parameters,
	}
}

// GetAllStrategies returns all cached strategies
func (sc *StrategyCache) GetAllStrategies() map[string]*StrategyState {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	
	result := make(map[string]*StrategyState)
	for id, state := range sc.states {
		result[id] = state
	}
	return result
}

// GetStrategiesByType returns strategies of a specific type
func (sc *StrategyCache) GetStrategiesByType(strategyType string) []*StrategyState {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	
	var result []*StrategyState
	for _, state := range sc.states {
		if state.Type == strategyType {
			result = append(result, state)
		}
	}
	return result
}

// GetRunningStrategies returns all running strategies
func (sc *StrategyCache) GetRunningStrategies() []*StrategyState {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	
	var result []*StrategyState
	for _, state := range sc.states {
		if state.Status == "running" {
			result = append(result, state)
		}
	}
	return result
}

// processUpdates processes strategy updates asynchronously
func (sc *StrategyCache) processUpdates() {
	for update := range sc.updateChan {
		sc.mu.Lock()
		
		state, exists := sc.states[update.StrategyID]
		if !exists {
			state = &StrategyState{
				StrategyID: update.StrategyID,
				Positions:  make(map[string]Position),
				Orders:     make(map[string]Order),
				Parameters: make(map[string]interface{}),
			}
			sc.states[update.StrategyID] = state
		}
		
		switch update.UpdateType {
		case "status":
			if status, ok := update.Data.(string); ok {
				state.Status = status
			}
			
		case "metrics":
			if metrics, ok := update.Data.(StrategyMetrics); ok {
				state.Metrics = metrics
			}
			
		case "position":
			if data, ok := update.Data.(map[string]interface{}); ok {
				if symbol, ok := data["symbol"].(string); ok {
					if pos, ok := data["position"].(Position); ok {
						state.Positions[symbol] = pos
					}
				}
			}
			
		case "order":
			if data, ok := update.Data.(map[string]interface{}); ok {
				if orderID, ok := data["order_id"].(string); ok {
					if order, ok := data["order"].(Order); ok {
						state.Orders[orderID] = order
					}
				}
			}
			
		case "parameters":
			if params, ok := update.Data.(map[string]interface{}); ok {
				for k, v := range params {
					state.Parameters[k] = v
				}
			}
		}
		
		state.LastUpdateAt = time.Now()
		
		// Update cache
		sc.cache.Set(fmt.Sprintf("strategy:%s", update.StrategyID), state, 24*time.Hour)
		
		sc.mu.Unlock()
	}
}

// ClearStrategy removes a strategy from cache
func (sc *StrategyCache) ClearStrategy(strategyID string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	
	delete(sc.states, strategyID)
	sc.cache.Delete(fmt.Sprintf("strategy:%s", strategyID))
}

// Close closes the strategy cache
func (sc *StrategyCache) Close() {
	close(sc.updateChan)
}

// StrategySession represents a strategy trading session
type StrategySession struct {
	SessionID   string    `json:"session_id"`
	StrategyID  string    `json:"strategy_id"`
	AccountID   string    `json:"account_id"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	IsActive    bool      `json:"is_active"`
}

// SessionManager manages strategy sessions
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*StrategySession
	cache    *MemoryCache
}

// NewSessionManager creates a new session manager
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*StrategySession),
		cache:    NewMemoryCache(),
	}
}

// CreateSession creates a new strategy session
func (sm *SessionManager) CreateSession(strategyID, accountID string) string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	sessionID := fmt.Sprintf("%s-%s-%d", strategyID, accountID, time.Now().UnixNano())
	session := &StrategySession{
		SessionID:  sessionID,
		StrategyID: strategyID,
		AccountID:  accountID,
		StartTime:  time.Now(),
		IsActive:   true,
	}
	
	sm.sessions[sessionID] = session
	sm.cache.Set(fmt.Sprintf("session:%s", sessionID), session, 24*time.Hour)
	
	return sessionID
}

// EndSession ends a strategy session
func (sm *SessionManager) EndSession(sessionID string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	session, exists := sm.sessions[sessionID]
	if !exists {
		return false
	}
	
	session.EndTime = time.Now()
	session.IsActive = false
	
	sm.cache.Set(fmt.Sprintf("session:%s", sessionID), session, 24*time.Hour)
	return true
}

// GetSession returns a session by ID
func (sm *SessionManager) GetSession(sessionID string) (*StrategySession, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	session, exists := sm.sessions[sessionID]
	return session, exists
}

// GetActiveSessions returns all active sessions
func (sm *SessionManager) GetActiveSessions() []*StrategySession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	var active []*StrategySession
	for _, session := range sm.sessions {
		if session.IsActive {
			active = append(active, session)
		}
	}
	return active
}

// GetSessionsByStrategy returns all sessions for a strategy
func (sm *SessionManager) GetSessionsByStrategy(strategyID string) []*StrategySession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	var sessions []*StrategySession
	for _, session := range sm.sessions {
		if session.StrategyID == strategyID {
			sessions = append(sessions, session)
		}
	}
	return sessions
}