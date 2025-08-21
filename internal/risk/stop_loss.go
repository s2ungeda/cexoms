package risk

import (
	"fmt"
	"sync"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// StopLossType represents different types of stop loss
type StopLossType string

const (
	StopLossTypeFixed     StopLossType = "FIXED"
	StopLossTypeTrailing  StopLossType = "TRAILING"
	StopLossTypeVolatility StopLossType = "VOLATILITY"
	StopLossTypeTime      StopLossType = "TIME"
)

// StopLossConfig represents stop loss configuration
type StopLossConfig struct {
	Type            StopLossType    `json:"type"`
	Percentage      float64         `json:"percentage"`      // For percentage-based stops
	Amount          decimal.Decimal `json:"amount"`          // For fixed amount stops
	TrailingPercent float64         `json:"trailing_percent"` // For trailing stops
	TimeLimit       time.Duration   `json:"time_limit"`      // For time-based stops
	ATRMultiplier   float64         `json:"atr_multiplier"`  // For volatility-based stops
}

// StopLoss represents an active stop loss order
type StopLoss struct {
	Symbol       string          `json:"symbol"`
	PositionSide types.Side      `json:"position_side"`
	EntryPrice   decimal.Decimal `json:"entry_price"`
	StopPrice    decimal.Decimal `json:"stop_price"`
	Type         StopLossType    `json:"type"`
	Config       StopLossConfig  `json:"config"`
	IsActive     bool            `json:"is_active"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	
	// For trailing stops
	HighWaterMark decimal.Decimal `json:"high_water_mark"`
	LowWaterMark  decimal.Decimal `json:"low_water_mark"`
}

// StopLossManager manages stop loss orders
type StopLossManager struct {
	mu         sync.RWMutex
	stopLosses map[string]map[string]*StopLoss // account -> symbol -> stop loss
	
	// Default configuration
	defaultConfig StopLossConfig
	
	// Callbacks
	onStopTriggered func(account string, stopLoss *StopLoss)
	
	// Price feeds for monitoring
	priceFeeds map[string]decimal.Decimal // symbol -> current price
}

// NewStopLossManager creates a new stop loss manager
func NewStopLossManager(defaultConfig StopLossConfig) *StopLossManager {
	return &StopLossManager{
		stopLosses:    make(map[string]map[string]*StopLoss),
		defaultConfig: defaultConfig,
		priceFeeds:    make(map[string]decimal.Decimal),
	}
}

// CreateStopLoss creates a new stop loss for a position
func (m *StopLossManager) CreateStopLoss(account string, position *types.Position, config *StopLossConfig) (*StopLoss, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if config == nil {
		config = &m.defaultConfig
	}
	
	entryPrice := position.EntryPrice
	
	// Calculate initial stop price based on type
	stopPrice, err := m.calculateStopPrice(entryPrice, position.Side, config)
	if err != nil {
		return nil, err
	}
	
	stopLoss := &StopLoss{
		Symbol:       position.Symbol,
		PositionSide: position.Side,
		EntryPrice:   entryPrice,
		StopPrice:    stopPrice,
		Type:         config.Type,
		Config:       *config,
		IsActive:     true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	
	// Initialize water marks for trailing stops
	if config.Type == StopLossTypeTrailing {
		if position.Side == types.Side("LONG") {
			stopLoss.HighWaterMark = entryPrice
		} else {
			stopLoss.LowWaterMark = entryPrice
		}
	}
	
	// Store stop loss
	if _, exists := m.stopLosses[account]; !exists {
		m.stopLosses[account] = make(map[string]*StopLoss)
	}
	m.stopLosses[account][position.Symbol] = stopLoss
	
	return stopLoss, nil
}

// UpdatePrice updates the current price and checks/updates stop losses
func (m *StopLossManager) UpdatePrice(symbol string, price decimal.Decimal) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.priceFeeds[symbol] = price
	
	var triggeredAccounts []string
	
	// Check all stop losses for this symbol
	for account, accountStops := range m.stopLosses {
		if stopLoss, exists := accountStops[symbol]; exists && stopLoss.IsActive {
			// Check if stop is triggered
			if m.isStopTriggered(stopLoss, price) {
				triggeredAccounts = append(triggeredAccounts, account)
				stopLoss.IsActive = false
				
				if m.onStopTriggered != nil {
					go m.onStopTriggered(account, stopLoss)
				}
			} else {
				// Update trailing stops
				if stopLoss.Type == StopLossTypeTrailing {
					m.updateTrailingStop(stopLoss, price)
				}
			}
		}
	}
	
	return triggeredAccounts
}

// GetStopLoss returns the stop loss for a position
func (m *StopLossManager) GetStopLoss(account, symbol string) (*StopLoss, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if accountStops, exists := m.stopLosses[account]; exists {
		if stopLoss, exists := accountStops[symbol]; exists {
			return stopLoss, true
		}
	}
	
	return nil, false
}

// CancelStopLoss cancels a stop loss
func (m *StopLossManager) CancelStopLoss(account, symbol string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if accountStops, exists := m.stopLosses[account]; exists {
		if stopLoss, exists := accountStops[symbol]; exists {
			stopLoss.IsActive = false
			delete(accountStops, symbol)
			return nil
		}
	}
	
	return fmt.Errorf("stop loss not found for %s/%s", account, symbol)
}

// ModifyStopLoss modifies an existing stop loss
func (m *StopLossManager) ModifyStopLoss(account, symbol string, newStopPrice decimal.Decimal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if accountStops, exists := m.stopLosses[account]; exists {
		if stopLoss, exists := accountStops[symbol]; exists {
			// Validate new stop price
			if stopLoss.PositionSide == types.Side("LONG") {
				if newStopPrice.GreaterThanOrEqual(stopLoss.EntryPrice) {
					return fmt.Errorf("stop price must be below entry price for long positions")
				}
			} else {
				if newStopPrice.LessThanOrEqual(stopLoss.EntryPrice) {
					return fmt.Errorf("stop price must be above entry price for short positions")
				}
			}
			
			stopLoss.StopPrice = newStopPrice
			stopLoss.UpdatedAt = time.Now()
			return nil
		}
	}
	
	return fmt.Errorf("stop loss not found for %s/%s", account, symbol)
}

// SetStopTriggeredCallback sets the callback for when a stop is triggered
func (m *StopLossManager) SetStopTriggeredCallback(callback func(account string, stopLoss *StopLoss)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onStopTriggered = callback
}

// GetAllStopLosses returns all stop losses for an account
func (m *StopLossManager) GetAllStopLosses(account string) map[string]*StopLoss {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if accountStops, exists := m.stopLosses[account]; exists {
		// Return a copy
		result := make(map[string]*StopLoss)
		for k, v := range accountStops {
			result[k] = v
		}
		return result
	}
	
	return nil
}

// Helper methods

func (m *StopLossManager) calculateStopPrice(entryPrice decimal.Decimal, side types.Side, config *StopLossConfig) (decimal.Decimal, error) {
	switch config.Type {
	case StopLossTypeFixed:
		if config.Percentage > 0 {
			// Percentage-based stop
			if side == types.Side("LONG") {
				return entryPrice.Mul(decimal.NewFromFloat(1 - config.Percentage/100)), nil
			} else {
				return entryPrice.Mul(decimal.NewFromFloat(1 + config.Percentage/100)), nil
			}
		} else if config.Amount.GreaterThan(decimal.Zero) {
			// Fixed amount stop
			if side == types.Side("LONG") {
				return entryPrice.Sub(config.Amount), nil
			} else {
				return entryPrice.Add(config.Amount), nil
			}
		}
		
	case StopLossTypeTrailing:
		// Initial stop same as fixed percentage
		if side == types.Side("LONG") {
			return entryPrice.Mul(decimal.NewFromFloat(1 - config.TrailingPercent/100)), nil
		} else {
			return entryPrice.Mul(decimal.NewFromFloat(1 + config.TrailingPercent/100)), nil
		}
		
	case StopLossTypeVolatility:
		// Would need ATR data
		return decimal.Zero, fmt.Errorf("volatility-based stops require ATR data")
		
	case StopLossTypeTime:
		// Time-based stops don't have a specific price initially
		return entryPrice, nil
	}
	
	return decimal.Zero, fmt.Errorf("invalid stop loss configuration")
}

func (m *StopLossManager) isStopTriggered(stopLoss *StopLoss, currentPrice decimal.Decimal) bool {
	if stopLoss.Type == StopLossTypeTime {
		// Check if time limit exceeded
		if time.Since(stopLoss.CreatedAt) > stopLoss.Config.TimeLimit {
			return true
		}
	}
	
	// Price-based stops
	if stopLoss.PositionSide == types.Side("LONG") {
		return currentPrice.LessThanOrEqual(stopLoss.StopPrice)
	} else {
		return currentPrice.GreaterThanOrEqual(stopLoss.StopPrice)
	}
}

func (m *StopLossManager) updateTrailingStop(stopLoss *StopLoss, currentPrice decimal.Decimal) {
	if stopLoss.PositionSide == types.Side("LONG") {
		// Update high water mark
		if currentPrice.GreaterThan(stopLoss.HighWaterMark) {
			stopLoss.HighWaterMark = currentPrice
			
			// Update stop price
			newStop := currentPrice.Mul(decimal.NewFromFloat(1 - stopLoss.Config.TrailingPercent/100))
			if newStop.GreaterThan(stopLoss.StopPrice) {
				stopLoss.StopPrice = newStop
				stopLoss.UpdatedAt = time.Now()
			}
		}
	} else {
		// Update low water mark for short positions
		if currentPrice.LessThan(stopLoss.LowWaterMark) {
			stopLoss.LowWaterMark = currentPrice
			
			// Update stop price
			newStop := currentPrice.Mul(decimal.NewFromFloat(1 + stopLoss.Config.TrailingPercent/100))
			if newStop.LessThan(stopLoss.StopPrice) {
				stopLoss.StopPrice = newStop
				stopLoss.UpdatedAt = time.Now()
			}
		}
	}
}

// BatchUpdatePrices updates multiple prices at once
func (m *StopLossManager) BatchUpdatePrices(prices map[string]decimal.Decimal) map[string][]string {
	triggeredBySymbol := make(map[string][]string)
	
	for symbol, price := range prices {
		triggered := m.UpdatePrice(symbol, price)
		if len(triggered) > 0 {
			triggeredBySymbol[symbol] = triggered
		}
	}
	
	return triggeredBySymbol
}