package futures

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/adshao/go-binance/v2/futures"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// PositionManager handles position-related operations for Binance Futures
type PositionManager struct {
	client      *futures.Client
	positions   map[string]*types.Position  // symbol -> position
	mu          sync.RWMutex
	updateCallbacks []types.PositionUpdateCallback
}

// NewPositionManager creates a new position manager
func NewPositionManager(client *futures.Client) *PositionManager {
	return &PositionManager{
		client:    client,
		positions: make(map[string]*types.Position),
	}
}

// ClosePosition closes a position
func (pm *PositionManager) ClosePosition(ctx context.Context, symbol string, side types.PositionSide) error {
	pm.mu.RLock()
	position, exists := pm.positions[symbol]
	pm.mu.RUnlock()
	
	if !exists || position.Quantity.IsZero() {
		return fmt.Errorf("no position found for %s", symbol)
	}
	
	// Determine order side to close position
	var orderSide futures.SideType
	if position.Side == types.PositionSideLong {
		orderSide = futures.SideTypeSell
	} else {
		orderSide = futures.SideTypeBuy
	}
	
	// Place market order to close position
	svc := pm.client.NewCreateOrderService().
		Symbol(symbol).
		Side(orderSide).
		Type(futures.OrderTypeMarket).
		Quantity(position.Quantity.String()).
		ReduceOnly(true)
	
	if position.PositionSide != "" {
		svc.PositionSide(futures.PositionSideType(position.PositionSide))
	}
	
	_, err := svc.Do(ctx)
	return err
}

// CloseAllPositions closes all open positions
func (pm *PositionManager) CloseAllPositions(ctx context.Context) error {
	pm.mu.RLock()
	positionsToClose := make([]*types.Position, 0)
	for _, pos := range pm.positions {
		if !pos.Quantity.IsZero() {
			positionsToClose = append(positionsToClose, pos)
		}
	}
	pm.mu.RUnlock()
	
	var errors []error
	for _, pos := range positionsToClose {
		if err := pm.ClosePosition(ctx, pos.Symbol, pos.Side); err != nil {
			errors = append(errors, fmt.Errorf("failed to close %s: %v", pos.Symbol, err))
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("failed to close some positions: %v", errors)
	}
	
	return nil
}

// AdjustPositionMargin adjusts isolated margin for a position
func (pm *PositionManager) AdjustPositionMargin(ctx context.Context, symbol string, amount decimal.Decimal, addMargin bool) error {
	var marginType int
	if addMargin {
		marginType = 1 // Add margin
	} else {
		marginType = 2 // Reduce margin
	}
	
	_, err := pm.client.NewUpdatePositionMarginService().
		Symbol(symbol).
		Amount(amount.String()).
		Type(marginType).
		Do(ctx)
		
	return err
}

// GetPositionRisk gets detailed position risk information
func (pm *PositionManager) GetPositionRisk(ctx context.Context, symbol string) (*types.PositionRisk, error) {
	risks, err := pm.client.NewGetPositionRiskService().Symbol(symbol).Do(ctx)
	if err != nil {
		return nil, err
	}
	
	if len(risks) == 0 {
		return nil, fmt.Errorf("no position risk found for %s", symbol)
	}
	
	risk := risks[0]
	
	// Parse values
	positionAmt := parseDecimalString(risk.PositionAmt)
	entryPrice := parseDecimalString(risk.EntryPrice)
	markPrice := parseDecimalString(risk.MarkPrice)
	unRealizedProfit := parseDecimalString(risk.UnRealizedProfit)
	liquidationPrice := parseDecimalString(risk.LiquidationPrice)
	leverage := parseDecimalString(risk.Leverage)
	maxNotional := parseDecimalString(risk.MaxNotionalValue)
	
	// Calculate additional risk metrics
	notional := positionAmt.Abs().Mul(markPrice)
	marginRatio := decimal.Zero
	maintenanceMargin := decimal.Zero
	
	// Calculate liquidation distance
	liquidationDistance := decimal.Zero
	if !liquidationPrice.IsZero() && !markPrice.IsZero() {
		if positionAmt.IsPositive() { // Long position
			liquidationDistance = markPrice.Sub(liquidationPrice).Div(markPrice)
		} else { // Short position
			liquidationDistance = liquidationPrice.Sub(markPrice).Div(markPrice)
		}
	}
	
	return &types.PositionRisk{
		Symbol:              symbol,
		PositionSide:        risk.PositionSide,
		PositionAmt:         positionAmt,
		EntryPrice:          entryPrice,
		MarkPrice:           markPrice,
		UnrealizedProfit:    unRealizedProfit,
		LiquidationPrice:    liquidationPrice,
		Leverage:            int(leverage.IntPart()),
		MaxNotional:         maxNotional,
		MarginType:          risk.MarginType,
		IsolatedMargin:      parseDecimalString(risk.IsolatedMargin),
		IsAutoAddMargin:     risk.IsAutoAddMargin == "true",
		Notional:            notional,
		MarginRatio:         marginRatio,
		MaintenanceMargin:   maintenanceMargin,
		LiquidationDistance: liquidationDistance,
		UpdateTime:          time.Now(),
	}, nil
}

// SetAutoAddMargin enables/disables auto add margin for isolated positions
func (pm *PositionManager) SetAutoAddMargin(ctx context.Context, symbol string, autoAdd bool) error {
	autoAddStr := "false"
	if autoAdd {
		autoAddStr = "true"
	}
	
	return pm.client.NewSetMarginAutoAddService().
		Symbol(symbol).
		AutoAddMargin(autoAddStr).
		Do(ctx)
}

// GetMaxLeverage gets the maximum leverage for a symbol
func (pm *PositionManager) GetMaxLeverage(ctx context.Context, symbol string) (int, error) {
	brackets, err := pm.client.NewGetLeverageBracketService().Symbol(symbol).Do(ctx)
	if err != nil {
		return 0, err
	}
	
	if len(brackets) == 0 || len(brackets[0].Brackets) == 0 {
		return 0, fmt.Errorf("no leverage bracket found for %s", symbol)
	}
	
	// Return the initial leverage from the first bracket
	return brackets[0].Brackets[0].InitialLeverage, nil
}

// ChangePositionMode changes between one-way and hedge mode
func (pm *PositionManager) ChangePositionMode(ctx context.Context, dualSide bool) error {
	dualSideStr := "false"
	if dualSide {
		dualSideStr = "true"
	}
	
	return pm.client.NewChangePositionModeService().DualSide(dualSideStr).Do(ctx)
}

// GetPositionMode gets the current position mode
func (pm *PositionManager) GetPositionMode(ctx context.Context) (bool, error) {
	result, err := pm.client.NewGetPositionModeService().Do(ctx)
	if err != nil {
		return false, err
	}
	
	return result.DualSidePosition, nil
}

// UpdatePosition updates the cached position
func (pm *PositionManager) UpdatePosition(position *types.Position) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pm.positions[position.Symbol] = position
	
	// Notify callbacks
	for _, callback := range pm.updateCallbacks {
		go callback(position)
	}
}

// GetPosition returns the cached position
func (pm *PositionManager) GetPosition(symbol string) (*types.Position, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	position, exists := pm.positions[symbol]
	return position, exists
}

// RegisterCallback registers a position update callback
func (pm *PositionManager) RegisterCallback(callback types.PositionUpdateCallback) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.updateCallbacks = append(pm.updateCallbacks, callback)
}

// RefreshPositions refreshes all positions from the exchange
func (pm *PositionManager) RefreshPositions(ctx context.Context) error {
	risks, err := pm.client.NewGetPositionRiskService().Do(ctx)
	if err != nil {
		return err
	}
	
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	// Clear existing positions
	pm.positions = make(map[string]*types.Position)
	
	// Update with fresh data
	for _, risk := range risks {
		posAmt := parseDecimalString(risk.PositionAmt)
		if posAmt.IsZero() {
			continue
		}
		
		side := types.PositionSideNone
		if posAmt.IsPositive() {
			side = types.PositionSideLong
		} else {
			side = types.PositionSideShort
			posAmt = posAmt.Abs()
		}
		
		position := &types.Position{
			Symbol:           risk.Symbol,
			Side:             side,
			PositionSide:     risk.PositionSide,
			Quantity:         posAmt,
			EntryPrice:       parseDecimalString(risk.EntryPrice),
			MarkPrice:        parseDecimalString(risk.MarkPrice),
			UnrealizedPnL:    parseDecimalString(risk.UnRealizedProfit),
			Margin:           parseDecimalString(risk.IsolatedMargin),
			MarginType:       risk.MarginType,
			Leverage:         int(parseDecimalString(risk.Leverage).IntPart()),
			LiquidationPrice: parseDecimalString(risk.LiquidationPrice),
			Notional:         parseDecimalString(risk.Notional),
			UpdateTime:       time.Now(),
		}
		
		pm.positions[risk.Symbol] = position
	}
	
	return nil
}

// GetTotalExposure calculates total exposure across all positions
func (pm *PositionManager) GetTotalExposure() decimal.Decimal {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	totalExposure := decimal.Zero
	for _, pos := range pm.positions {
		if !pos.Quantity.IsZero() {
			totalExposure = totalExposure.Add(pos.Notional.Abs())
		}
	}
	
	return totalExposure
}

// GetTotalUnrealizedPnL calculates total unrealized PnL
func (pm *PositionManager) GetTotalUnrealizedPnL() decimal.Decimal {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	totalPnL := decimal.Zero
	for _, pos := range pm.positions {
		if !pos.Quantity.IsZero() {
			totalPnL = totalPnL.Add(pos.UnrealizedPnL)
		}
	}
	
	return totalPnL
}

// Helper function to parse decimal from string
func parseDecimalString(s string) decimal.Decimal {
	if s == "" {
		return decimal.Zero
	}
	d, _ := decimal.NewFromString(s)
	return d
}