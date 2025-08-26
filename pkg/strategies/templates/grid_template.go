package templates

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	
	"github.com/mExOms/pkg/strategies"
	"github.com/mExOms/pkg/types"
)

// GridTemplate is a template for grid trading strategies
type GridTemplate struct {
	*strategies.BaseStrategy
	
	mu sync.RWMutex
	
	// Grid configuration
	gridLevels      int
	gridSpacing     decimal.Decimal // Percentage spacing between levels
	upperBound      decimal.Decimal
	lowerBound      decimal.Decimal
	orderSize       decimal.Decimal
	
	// Grid state
	gridOrders      map[string]*GridOrder
	currentPrice    decimal.Decimal
	activeGrids     int
	
	// Mode
	gridMode        string // "neutral", "long", "short"
	rebalanceMode   bool   // Auto-rebalance grid when price moves out of range
	
	// Performance
	gridProfits     decimal.Decimal
	completedGrids  int
}

// GridOrder represents an order in the grid
type GridOrder struct {
	Level         int
	Price         decimal.Decimal
	Side          types.OrderSide
	OrderID       string
	Status        string // "pending", "placed", "filled", "cancelled"
	Quantity      decimal.Decimal
	FilledQty     decimal.Decimal
	PairedOrderID string // ID of the paired order (buy paired with sell)
	CreatedAt     time.Time
}

// NewGridTemplate creates a new grid trading template
func NewGridTemplate(base *strategies.BaseStrategy) *GridTemplate {
	return &GridTemplate{
		BaseStrategy:   base,
		gridOrders:     make(map[string]*GridOrder),
		gridLevels:     10,
		gridSpacing:    decimal.NewFromFloat(0.01), // 1% default
		orderSize:      decimal.NewFromFloat(0.1),
		gridMode:       "neutral",
		rebalanceMode:  false,
	}
}

// Initialize initializes the grid strategy
func (g *GridTemplate) Initialize(config strategies.StrategyConfig) error {
	// Initialize base strategy
	if err := g.BaseStrategy.Initialize(config); err != nil {
		return err
	}
	
	// Load grid-specific parameters
	if levels, ok := config.Parameters["grid_levels"].(float64); ok {
		g.gridLevels = int(levels)
	}
	
	if spacing, ok := config.Parameters["grid_spacing"].(float64); ok {
		g.gridSpacing = decimal.NewFromFloat(spacing)
	}
	
	if size, ok := config.Parameters["order_size"].(float64); ok {
		g.orderSize = decimal.NewFromFloat(size)
	}
	
	if mode, ok := config.Parameters["grid_mode"].(string); ok {
		g.gridMode = mode
	}
	
	if rebalance, ok := config.Parameters["rebalance_mode"].(bool); ok {
		g.rebalanceMode = rebalance
	}
	
	// Set initial bounds if provided
	if upper, ok := config.Parameters["upper_bound"].(float64); ok {
		g.upperBound = decimal.NewFromFloat(upper)
	}
	
	if lower, ok := config.Parameters["lower_bound"].(float64); ok {
		g.lowerBound = decimal.NewFromFloat(lower)
	}
	
	return nil
}

// Start starts the grid strategy
func (g *GridTemplate) Start(ctx context.Context) error {
	if err := g.BaseStrategy.Start(ctx); err != nil {
		return err
	}
	
	// Wait for initial price
	if g.currentPrice.IsZero() {
		g.Logger.Info("Waiting for initial price...")
		// In production, would wait for first tick
		// For now, we'll assume price is set
	}
	
	// Set up initial grid
	if err := g.setupGrid(); err != nil {
		return fmt.Errorf("failed to setup grid: %w", err)
	}
	
	// Start monitoring
	go g.monitorGrid()
	
	return nil
}

// Stop stops the grid strategy
func (g *GridTemplate) Stop() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	// Cancel all grid orders
	for _, gridOrder := range g.gridOrders {
		if gridOrder.Status == "placed" && gridOrder.OrderID != "" {
			if err := g.OrderManager.CancelOrder(gridOrder.OrderID); err != nil {
				g.Logger.Error("Failed to cancel grid order",
					zap.String("order_id", gridOrder.OrderID),
					zap.Error(err))
			}
		}
	}
	
	return g.BaseStrategy.Stop()
}

// OnTick handles price updates
func (g *GridTemplate) OnTick(tick *types.Tick) {
	g.mu.Lock()
	g.currentPrice = tick.Price
	g.mu.Unlock()
	
	// Check if price moved out of grid range
	if g.rebalanceMode {
		if tick.Price.GreaterThan(g.upperBound) || tick.Price.LessThan(g.lowerBound) {
			g.Logger.Info("Price out of grid range, rebalancing",
				zap.String("price", tick.Price.String()),
				zap.String("upper", g.upperBound.String()),
				zap.String("lower", g.lowerBound.String()))
			
			go g.rebalanceGrid(tick.Price)
		}
	}
}

// OnOrderFilled handles filled orders
func (g *GridTemplate) OnOrderFilled(order *types.Order) {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	// Find grid order
	gridOrder, exists := g.gridOrders[order.ID]
	if !exists {
		return
	}
	
	gridOrder.Status = "filled"
	gridOrder.FilledQty = order.FilledQuantity
	
	g.Logger.Info("Grid order filled",
		zap.Int("level", gridOrder.Level),
		zap.String("side", string(gridOrder.Side)),
		zap.String("price", gridOrder.Price.String()),
		zap.String("quantity", gridOrder.FilledQty.String()))
	
	// Place opposite order
	go g.placeOppositeOrder(gridOrder)
	
	// Update metrics
	g.activeGrids--
	if gridOrder.Side == types.OrderSideSell {
		// Selling at higher price = profit
		profit := gridOrder.Price.Sub(g.currentPrice).Mul(gridOrder.FilledQty)
		g.gridProfits = g.gridProfits.Add(profit)
	}
}

// setupGrid sets up the initial grid
func (g *GridTemplate) setupGrid() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	// Calculate grid bounds if not set
	if g.upperBound.IsZero() || g.lowerBound.IsZero() {
		g.calculateGridBounds()
	}
	
	// Validate bounds
	if g.upperBound.LessThanOrEqual(g.lowerBound) {
		return fmt.Errorf("invalid grid bounds: upper=%s, lower=%s", 
			g.upperBound, g.lowerBound)
	}
	
	// Calculate grid levels
	gridRange := g.upperBound.Sub(g.lowerBound)
	levelSpacing := gridRange.Div(decimal.NewFromInt(int64(g.gridLevels - 1)))
	
	g.Logger.Info("Setting up grid",
		zap.Int("levels", g.gridLevels),
		zap.String("upper", g.upperBound.String()),
		zap.String("lower", g.lowerBound.String()),
		zap.String("spacing", levelSpacing.String()))
	
	// Create grid orders
	for i := 0; i < g.gridLevels; i++ {
		price := g.lowerBound.Add(levelSpacing.Mul(decimal.NewFromInt(int64(i))))
		
		// Determine order side based on current price
		var side types.OrderSide
		if price.GreaterThan(g.currentPrice) {
			side = types.OrderSideSell // Sell above current price
		} else {
			side = types.OrderSideBuy // Buy below current price
		}
		
		// Skip orders too close to current price
		priceDistance := price.Sub(g.currentPrice).Abs()
		minDistance := g.currentPrice.Mul(decimal.NewFromFloat(0.001)) // 0.1% minimum distance
		
		if priceDistance.LessThan(minDistance) {
			continue
		}
		
		// Create grid order
		gridOrder := &GridOrder{
			Level:     i,
			Price:     price,
			Side:      side,
			Quantity:  g.orderSize,
			Status:    "pending",
			CreatedAt: time.Now(),
		}
		
		// Place the order
		if err := g.placeGridOrder(gridOrder); err != nil {
			g.Logger.Error("Failed to place grid order",
				zap.Int("level", i),
				zap.String("price", price.String()),
				zap.Error(err))
			continue
		}
		
		g.gridOrders[gridOrder.OrderID] = gridOrder
		g.activeGrids++
	}
	
	g.Logger.Info("Grid setup complete",
		zap.Int("active_orders", g.activeGrids))
	
	return nil
}

// placeGridOrder places a single grid order
func (g *GridTemplate) placeGridOrder(gridOrder *GridOrder) error {
	req := &types.OrderRequest{
		AccountID:   g.Config.Accounts[0], // Use first account
		Exchange:    g.Config.Exchanges[0],
		Symbol:      g.Config.Symbols[0],
		Side:        gridOrder.Side,
		Type:        types.OrderTypeLimit,
		Quantity:    gridOrder.Quantity,
		Price:       gridOrder.Price,
		TimeInForce: types.TimeInForceGTC,
	}
	
	order, err := g.PlaceOrder(req)
	if err != nil {
		return err
	}
	
	gridOrder.OrderID = order.ID
	gridOrder.Status = "placed"
	
	return nil
}

// placeOppositeOrder places the opposite order after a fill
func (g *GridTemplate) placeOppositeOrder(filledOrder *GridOrder) {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	// Calculate opposite price based on grid spacing
	var oppositePrice decimal.Decimal
	var oppositeSide types.OrderSide
	
	if filledOrder.Side == types.OrderSideBuy {
		// We bought, now place sell order above
		oppositeSide = types.OrderSideSell
		oppositePrice = filledOrder.Price.Mul(decimal.NewFromFloat(1).Add(g.gridSpacing))
	} else {
		// We sold, now place buy order below
		oppositeSide = types.OrderSideBuy
		oppositePrice = filledOrder.Price.Mul(decimal.NewFromFloat(1).Sub(g.gridSpacing))
	}
	
	// Check bounds
	if oppositePrice.GreaterThan(g.upperBound) || oppositePrice.LessThan(g.lowerBound) {
		g.Logger.Debug("Opposite order outside grid bounds",
			zap.String("price", oppositePrice.String()))
		return
	}
	
	// Create opposite order
	oppositeOrder := &GridOrder{
		Level:         filledOrder.Level,
		Price:         oppositePrice,
		Side:          oppositeSide,
		Quantity:      filledOrder.FilledQty,
		Status:        "pending",
		PairedOrderID: filledOrder.OrderID,
		CreatedAt:     time.Now(),
	}
	
	// Place the order
	if err := g.placeGridOrder(oppositeOrder); err != nil {
		g.Logger.Error("Failed to place opposite order",
			zap.String("price", oppositePrice.String()),
			zap.Error(err))
		return
	}
	
	g.gridOrders[oppositeOrder.OrderID] = oppositeOrder
	g.activeGrids++
	g.completedGrids++
	
	g.Logger.Info("Placed opposite grid order",
		zap.String("side", string(oppositeSide)),
		zap.String("price", oppositePrice.String()))
}

// calculateGridBounds calculates grid bounds based on current price
func (g *GridTemplate) calculateGridBounds() {
	if g.currentPrice.IsZero() {
		return
	}
	
	// Calculate bounds as +/- percentage from current price
	rangePercent := g.gridSpacing.Mul(decimal.NewFromInt(int64(g.gridLevels)))
	
	g.upperBound = g.currentPrice.Mul(decimal.NewFromFloat(1).Add(rangePercent.Div(decimal.NewFromInt(2))))
	g.lowerBound = g.currentPrice.Mul(decimal.NewFromFloat(1).Sub(rangePercent.Div(decimal.NewFromInt(2))))
}

// rebalanceGrid rebalances the grid when price moves out of range
func (g *GridTemplate) rebalanceGrid(newPrice decimal.Decimal) {
	g.Logger.Info("Rebalancing grid",
		zap.String("new_price", newPrice.String()))
	
	// Cancel all existing orders
	g.mu.Lock()
	for _, gridOrder := range g.gridOrders {
		if gridOrder.Status == "placed" && gridOrder.OrderID != "" {
			g.OrderManager.CancelOrder(gridOrder.OrderID)
		}
	}
	g.gridOrders = make(map[string]*GridOrder)
	g.activeGrids = 0
	g.mu.Unlock()
	
	// Recalculate bounds
	g.currentPrice = newPrice
	g.calculateGridBounds()
	
	// Setup new grid
	if err := g.setupGrid(); err != nil {
		g.Logger.Error("Failed to rebalance grid", zap.Error(err))
	}
}

// monitorGrid monitors grid performance
func (g *GridTemplate) monitorGrid() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
			g.logGridStatus()
		}
	}
}

// logGridStatus logs current grid status
func (g *GridTemplate) logGridStatus() {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	buyOrders := 0
	sellOrders := 0
	
	for _, order := range g.gridOrders {
		if order.Status == "placed" {
			if order.Side == types.OrderSideBuy {
				buyOrders++
			} else {
				sellOrders++
			}
		}
	}
	
	g.Logger.Info("Grid status",
		zap.Int("active_grids", g.activeGrids),
		zap.Int("buy_orders", buyOrders),
		zap.Int("sell_orders", sellOrders),
		zap.Int("completed_grids", g.completedGrids),
		zap.String("grid_profits", g.gridProfits.String()),
		zap.String("current_price", g.currentPrice.String()))
}

// GetGridAnalytics returns grid analytics
func (g *GridTemplate) GetGridAnalytics() map[string]interface{} {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	analytics := map[string]interface{}{
		"grid_levels":      g.gridLevels,
		"grid_spacing":     g.gridSpacing.String(),
		"upper_bound":      g.upperBound.String(),
		"lower_bound":      g.lowerBound.String(),
		"active_grids":     g.activeGrids,
		"completed_grids":  g.completedGrids,
		"grid_profits":     g.gridProfits.String(),
		"current_price":    g.currentPrice.String(),
		"grid_mode":        g.gridMode,
		"rebalance_mode":   g.rebalanceMode,
	}
	
	// Calculate grid utilization
	if g.gridLevels > 0 {
		utilization := float64(g.activeGrids) / float64(g.gridLevels) * 100
		analytics["utilization"] = fmt.Sprintf("%.1f%%", utilization)
	}
	
	// Calculate average profit per grid
	if g.completedGrids > 0 {
		avgProfit := g.gridProfits.Div(decimal.NewFromInt(int64(g.completedGrids)))
		analytics["avg_profit_per_grid"] = avgProfit.String()
	}
	
	return analytics
}

// AdjustGridParameters adjusts grid parameters dynamically
func (g *GridTemplate) AdjustGridParameters(params map[string]interface{}) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	needsResetup := false
	
	// Update grid levels
	if levels, ok := params["grid_levels"].(float64); ok {
		if int(levels) != g.gridLevels {
			g.gridLevels = int(levels)
			needsResetup = true
		}
	}
	
	// Update grid spacing
	if spacing, ok := params["grid_spacing"].(float64); ok {
		newSpacing := decimal.NewFromFloat(spacing)
		if !newSpacing.Equal(g.gridSpacing) {
			g.gridSpacing = newSpacing
			needsResetup = true
		}
	}
	
	// Update order size
	if size, ok := params["order_size"].(float64); ok {
		g.orderSize = decimal.NewFromFloat(size)
		// This doesn't require grid resetup
	}
	
	// Update mode
	if mode, ok := params["grid_mode"].(string); ok {
		g.gridMode = mode
	}
	
	// If grid structure changed, resetup
	if needsResetup {
		go g.rebalanceGrid(g.currentPrice)
	}
	
	return nil
}

// EstimateGridProfitability estimates potential profitability
func (g *GridTemplate) EstimateGridProfitability(volatility float64) decimal.Decimal {
	// Simple estimation based on:
	// - Grid spacing
	// - Number of levels
	// - Expected volatility
	// - Trading fees
	
	expectedTradesPerDay := volatility * float64(g.gridLevels) * 2
	profitPerTrade := g.orderSize.Mul(g.gridSpacing).Sub(
		g.orderSize.Mul(decimal.NewFromFloat(0.002))) // Assume 0.1% fee each side
	
	dailyProfit := profitPerTrade.Mul(decimal.NewFromFloat(expectedTradesPerDay))
	
	return dailyProfit
}