package templates

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	
	"github.com/mExOms/pkg/strategies"
	"github.com/mExOms/pkg/types"
)

// MarketMakingTemplate is a template for market making strategies
type MarketMakingTemplate struct {
	*strategies.BaseStrategy
	
	mu sync.RWMutex
	
	// Market making parameters
	spreadBps          int             // Spread in basis points
	orderDepth         int             // Number of orders on each side
	orderSize          decimal.Decimal // Base order size
	sizingMode         string          // "fixed", "dynamic", "inventory"
	inventoryTarget    decimal.Decimal // Target inventory
	maxInventory       decimal.Decimal // Maximum inventory allowed
	
	// Dynamic spread parameters
	minSpreadBps       int
	maxSpreadBps       int
	volatilityPeriod   int             // Periods for volatility calculation
	
	// Risk parameters
	maxOrderValue      decimal.Decimal
	positionLimit      decimal.Decimal
	drawdownLimit      decimal.Decimal
	
	// Market data
	currentBid         decimal.Decimal
	currentAsk         decimal.Decimal
	midPrice           decimal.Decimal
	volatility         float64
	priceHistory       []decimal.Decimal
	
	// Inventory tracking
	inventory          decimal.Decimal
	inventoryValue     decimal.Decimal
	
	// Active orders
	buyOrders          map[string]*MakerOrder
	sellOrders         map[string]*MakerOrder
	
	// Performance tracking
	filledBuyVolume    decimal.Decimal
	filledSellVolume   decimal.Decimal
	spreadCapture      decimal.Decimal
	roundTrips         int
}

// MakerOrder represents a maker order
type MakerOrder struct {
	OrderID    string
	Price      decimal.Decimal
	Quantity   decimal.Decimal
	Side       types.OrderSide
	Distance   int // Distance from mid price in levels
	PlacedAt   time.Time
	LastUpdate time.Time
}

// NewMarketMakingTemplate creates a new market making template
func NewMarketMakingTemplate(base *strategies.BaseStrategy) *MarketMakingTemplate {
	return &MarketMakingTemplate{
		BaseStrategy:     base,
		spreadBps:        20,  // 0.20% default spread
		orderDepth:       3,
		orderSize:        decimal.NewFromFloat(0.1),
		sizingMode:       "fixed",
		minSpreadBps:     10,
		maxSpreadBps:     100,
		volatilityPeriod: 20,
		buyOrders:        make(map[string]*MakerOrder),
		sellOrders:       make(map[string]*MakerOrder),
		priceHistory:     make([]decimal.Decimal, 0),
	}
}

// Initialize initializes the market making strategy
func (m *MarketMakingTemplate) Initialize(config strategies.StrategyConfig) error {
	// Initialize base strategy
	if err := m.BaseStrategy.Initialize(config); err != nil {
		return err
	}
	
	// Load market making parameters
	if spread, ok := config.Parameters["spread_bps"].(float64); ok {
		m.spreadBps = int(spread)
	}
	
	if depth, ok := config.Parameters["order_depth"].(float64); ok {
		m.orderDepth = int(depth)
	}
	
	if size, ok := config.Parameters["order_size"].(float64); ok {
		m.orderSize = decimal.NewFromFloat(size)
	}
	
	if mode, ok := config.Parameters["sizing_mode"].(string); ok {
		m.sizingMode = mode
	}
	
	if target, ok := config.Parameters["inventory_target"].(float64); ok {
		m.inventoryTarget = decimal.NewFromFloat(target)
	}
	
	if maxInv, ok := config.Parameters["max_inventory"].(float64); ok {
		m.maxInventory = decimal.NewFromFloat(maxInv)
	}
	
	// Set position limit
	m.positionLimit = m.Config.MaxPositionSize
	if m.positionLimit.IsZero() {
		m.positionLimit = decimal.NewFromFloat(10) // Default 10 units
	}
	
	return nil
}

// Start starts the market making strategy
func (m *MarketMakingTemplate) Start(ctx context.Context) error {
	if err := m.BaseStrategy.Start(ctx); err != nil {
		return err
	}
	
	// Start market making loop
	go m.marketMakingLoop()
	
	// Start order management loop
	go m.orderManagementLoop()
	
	return nil
}

// Stop stops the market making strategy
func (m *MarketMakingTemplate) Stop() error {
	// Cancel all active orders
	m.cancelAllMakerOrders()
	
	return m.BaseStrategy.Stop()
}

// OnOrderBook handles order book updates
func (m *MarketMakingTemplate) OnOrderBook(book *types.OrderBook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Update market prices
	if len(book.Bids) > 0 && len(book.Asks) > 0 {
		m.currentBid = book.Bids[0].Price
		m.currentAsk = book.Asks[0].Price
		m.midPrice = m.currentBid.Add(m.currentAsk).Div(decimal.NewFromInt(2))
		
		// Update price history for volatility calculation
		m.updatePriceHistory(m.midPrice)
		
		// Calculate volatility
		m.calculateVolatility()
	}
}

// OnOrderFilled handles filled orders
func (m *MarketMakingTemplate) OnOrderFilled(order *types.Order) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Update inventory
	if order.Side == types.OrderSideBuy {
		m.inventory = m.inventory.Add(order.FilledQuantity)
		m.filledBuyVolume = m.filledBuyVolume.Add(order.FilledQuantity)
		delete(m.buyOrders, order.ID)
	} else {
		m.inventory = m.inventory.Sub(order.FilledQuantity)
		m.filledSellVolume = m.filledSellVolume.Add(order.FilledQuantity)
		delete(m.sellOrders, order.ID)
		
		// Check for round trip
		if m.filledBuyVolume.GreaterThan(decimal.Zero) && 
		   m.filledSellVolume.GreaterThan(decimal.Zero) {
			m.roundTrips++
			
			// Calculate spread capture
			avgBuyPrice := order.Price.Sub(m.midPrice.Mul(
				decimal.NewFromInt(int64(m.spreadBps)).Div(decimal.NewFromInt(10000))))
			avgSellPrice := order.Price.Add(m.midPrice.Mul(
				decimal.NewFromInt(int64(m.spreadBps)).Div(decimal.NewFromInt(10000))))
			spread := avgSellPrice.Sub(avgBuyPrice).Mul(order.FilledQuantity)
			m.spreadCapture = m.spreadCapture.Add(spread)
		}
	}
	
	// Update inventory value
	m.inventoryValue = m.inventory.Mul(m.midPrice)
	
	m.Logger.Info("Market maker order filled",
		zap.String("order_id", order.ID),
		zap.String("side", string(order.Side)),
		zap.String("price", order.Price.String()),
		zap.String("quantity", order.FilledQuantity.String()),
		zap.String("inventory", m.inventory.String()))
}

// marketMakingLoop is the main market making loop
func (m *MarketMakingTemplate) marketMakingLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.updateQuotes()
		}
	}
}

// updateQuotes updates market making quotes
func (m *MarketMakingTemplate) updateQuotes() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Don't quote if no market data
	if m.midPrice.IsZero() {
		return
	}
	
	// Calculate dynamic spread based on volatility and inventory
	spread := m.calculateDynamicSpread()
	
	// Cancel orders that are too far from mid price
	m.cancelStaleOrders()
	
	// Place new orders
	m.placeMarketMakingOrders(spread)
}

// calculateDynamicSpread calculates spread based on market conditions
func (m *MarketMakingTemplate) calculateDynamicSpread() decimal.Decimal {
	baseSpreadBps := m.spreadBps
	
	// Adjust for volatility
	if m.volatility > 0 {
		volAdjustment := math.Min(m.volatility*10000, float64(m.maxSpreadBps-m.minSpreadBps))
		baseSpreadBps = m.minSpreadBps + int(volAdjustment)
	}
	
	// Adjust for inventory
	inventoryRatio := decimal.Zero
	if !m.inventoryTarget.IsZero() {
		inventoryRatio = m.inventory.Div(m.inventoryTarget)
	}
	
	// Skew spread based on inventory
	buySpreadBps := baseSpreadBps
	sellSpreadBps := baseSpreadBps
	
	if m.sizingMode == "inventory" {
		if inventoryRatio.GreaterThan(decimal.NewFromFloat(1.2)) {
			// Too much inventory, widen buy spread, tighten sell spread
			buySpreadBps = int(float64(baseSpreadBps) * 1.5)
			sellSpreadBps = int(float64(baseSpreadBps) * 0.8)
		} else if inventoryRatio.LessThan(decimal.NewFromFloat(0.8)) {
			// Too little inventory, tighten buy spread, widen sell spread
			buySpreadBps = int(float64(baseSpreadBps) * 0.8)
			sellSpreadBps = int(float64(baseSpreadBps) * 1.5)
		}
	}
	
	avgSpread := (buySpreadBps + sellSpreadBps) / 2
	return decimal.NewFromInt(int64(avgSpread)).Div(decimal.NewFromInt(10000))
}

// placeMarketMakingOrders places market making orders
func (m *MarketMakingTemplate) placeMarketMakingOrders(spread decimal.Decimal) {
	// Check risk limits
	if m.inventory.Abs().GreaterThan(m.maxInventory) {
		m.Logger.Warn("Inventory limit reached, skipping quotes",
			zap.String("inventory", m.inventory.String()),
			zap.String("max", m.maxInventory.String()))
		return
	}
	
	// Place buy orders
	for i := 0; i < m.orderDepth; i++ {
		// Calculate price for this level
		levelSpread := spread.Mul(decimal.NewFromInt(int64(i + 1)))
		buyPrice := m.midPrice.Sub(m.midPrice.Mul(levelSpread))
		
		// Calculate size
		size := m.calculateOrderSize(i, types.OrderSideBuy)
		
		// Check if we already have an order at this level
		existingOrder := m.findOrderNearPrice(buyPrice, types.OrderSideBuy)
		if existingOrder != nil {
			continue
		}
		
		// Place order
		req := &types.OrderRequest{
			AccountID:   m.Config.Accounts[0],
			Exchange:    m.Config.Exchanges[0],
			Symbol:      m.Config.Symbols[0],
			Side:        types.OrderSideBuy,
			Type:        types.OrderTypeLimit,
			Quantity:    size,
			Price:       buyPrice,
			TimeInForce: types.TimeInForceGTC,
		}
		
		order, err := m.PlaceOrder(req)
		if err != nil {
			m.Logger.Error("Failed to place buy order",
				zap.Int("level", i),
				zap.Error(err))
			continue
		}
		
		m.buyOrders[order.ID] = &MakerOrder{
			OrderID:    order.ID,
			Price:      buyPrice,
			Quantity:   size,
			Side:       types.OrderSideBuy,
			Distance:   i,
			PlacedAt:   time.Now(),
			LastUpdate: time.Now(),
		}
	}
	
	// Place sell orders
	for i := 0; i < m.orderDepth; i++ {
		// Calculate price for this level
		levelSpread := spread.Mul(decimal.NewFromInt(int64(i + 1)))
		sellPrice := m.midPrice.Add(m.midPrice.Mul(levelSpread))
		
		// Calculate size
		size := m.calculateOrderSize(i, types.OrderSideSell)
		
		// Check if we already have an order at this level
		existingOrder := m.findOrderNearPrice(sellPrice, types.OrderSideSell)
		if existingOrder != nil {
			continue
		}
		
		// Place order
		req := &types.OrderRequest{
			AccountID:   m.Config.Accounts[0],
			Exchange:    m.Config.Exchanges[0],
			Symbol:      m.Config.Symbols[0],
			Side:        types.OrderSideSell,
			Type:        types.OrderTypeLimit,
			Quantity:    size,
			Price:       sellPrice,
			TimeInForce: types.TimeInForceGTC,
		}
		
		order, err := m.PlaceOrder(req)
		if err != nil {
			m.Logger.Error("Failed to place sell order",
				zap.Int("level", i),
				zap.Error(err))
			continue
		}
		
		m.sellOrders[order.ID] = &MakerOrder{
			OrderID:    order.ID,
			Price:      sellPrice,
			Quantity:   size,
			Side:       types.OrderSideSell,
			Distance:   i,
			PlacedAt:   time.Now(),
			LastUpdate: time.Now(),
		}
	}
}

// calculateOrderSize calculates order size based on level and mode
func (m *MarketMakingTemplate) calculateOrderSize(level int, side types.OrderSide) decimal.Decimal {
	baseSize := m.orderSize
	
	switch m.sizingMode {
	case "dynamic":
		// Larger sizes for levels closer to mid price
		multiplier := decimal.NewFromFloat(1.0 / float64(level+1))
		return baseSize.Mul(multiplier)
		
	case "inventory":
		// Adjust size based on inventory
		if side == types.OrderSideBuy && m.inventory.GreaterThan(m.inventoryTarget) {
			// Reduce buy size when inventory is high
			return baseSize.Mul(decimal.NewFromFloat(0.5))
		} else if side == types.OrderSideSell && m.inventory.LessThan(m.inventoryTarget) {
			// Reduce sell size when inventory is low
			return baseSize.Mul(decimal.NewFromFloat(0.5))
		}
		
	default: // "fixed"
		return baseSize
	}
	
	return baseSize
}

// findOrderNearPrice finds an existing order near the given price
func (m *MarketMakingTemplate) findOrderNearPrice(price decimal.Decimal, side types.OrderSide) *MakerOrder {
	tolerance := price.Mul(decimal.NewFromFloat(0.0001)) // 0.01% tolerance
	
	orders := m.buyOrders
	if side == types.OrderSideSell {
		orders = m.sellOrders
	}
	
	for _, order := range orders {
		priceDiff := order.Price.Sub(price).Abs()
		if priceDiff.LessThan(tolerance) {
			return order
		}
	}
	
	return nil
}

// cancelStaleOrders cancels orders that are too far from mid price
func (m *MarketMakingTemplate) cancelStaleOrders() {
	maxDistance := m.midPrice.Mul(decimal.NewFromFloat(0.01)) // 1% max distance
	
	// Check buy orders
	for id, order := range m.buyOrders {
		distance := m.midPrice.Sub(order.Price)
		if distance.GreaterThan(maxDistance) || time.Since(order.LastUpdate) > 30*time.Second {
			if err := m.OrderManager.CancelOrder(order.OrderID); err != nil {
				m.Logger.Error("Failed to cancel stale buy order",
					zap.String("order_id", order.OrderID),
					zap.Error(err))
			}
			delete(m.buyOrders, id)
		}
	}
	
	// Check sell orders
	for id, order := range m.sellOrders {
		distance := order.Price.Sub(m.midPrice)
		if distance.GreaterThan(maxDistance) || time.Since(order.LastUpdate) > 30*time.Second {
			if err := m.OrderManager.CancelOrder(order.OrderID); err != nil {
				m.Logger.Error("Failed to cancel stale sell order",
					zap.String("order_id", order.OrderID),
					zap.Error(err))
			}
			delete(m.sellOrders, id)
		}
	}
}

// cancelAllMakerOrders cancels all market making orders
func (m *MarketMakingTemplate) cancelAllMakerOrders() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Cancel buy orders
	for _, order := range m.buyOrders {
		m.OrderManager.CancelOrder(order.OrderID)
	}
	m.buyOrders = make(map[string]*MakerOrder)
	
	// Cancel sell orders
	for _, order := range m.sellOrders {
		m.OrderManager.CancelOrder(order.OrderID)
	}
	m.sellOrders = make(map[string]*MakerOrder)
}

// updatePriceHistory updates price history for volatility calculation
func (m *MarketMakingTemplate) updatePriceHistory(price decimal.Decimal) {
	m.priceHistory = append(m.priceHistory, price)
	
	// Keep only recent history
	if len(m.priceHistory) > m.volatilityPeriod {
		m.priceHistory = m.priceHistory[len(m.priceHistory)-m.volatilityPeriod:]
	}
}

// calculateVolatility calculates price volatility
func (m *MarketMakingTemplate) calculateVolatility() {
	if len(m.priceHistory) < 2 {
		return
	}
	
	// Calculate returns
	returns := make([]float64, 0, len(m.priceHistory)-1)
	for i := 1; i < len(m.priceHistory); i++ {
		prev := m.priceHistory[i-1]
		curr := m.priceHistory[i]
		if !prev.IsZero() {
			ret := curr.Sub(prev).Div(prev).InexactFloat64()
			returns = append(returns, ret)
		}
	}
	
	// Calculate standard deviation
	if len(returns) > 0 {
		mean := 0.0
		for _, r := range returns {
			mean += r
		}
		mean /= float64(len(returns))
		
		variance := 0.0
		for _, r := range returns {
			variance += math.Pow(r-mean, 2)
		}
		variance /= float64(len(returns))
		
		m.volatility = math.Sqrt(variance)
	}
}

// orderManagementLoop manages order lifecycle
func (m *MarketMakingTemplate) orderManagementLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkOrderHealth()
		}
	}
}

// checkOrderHealth checks health of active orders
func (m *MarketMakingTemplate) checkOrderHealth() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Log status
	m.Logger.Info("Market maker status",
		zap.Int("buy_orders", len(m.buyOrders)),
		zap.Int("sell_orders", len(m.sellOrders)),
		zap.String("inventory", m.inventory.String()),
		zap.String("spread_capture", m.spreadCapture.String()),
		zap.Int("round_trips", m.roundTrips),
		zap.Float64("volatility", m.volatility))
}

// GetMarketMakingMetrics returns market making specific metrics
func (m *MarketMakingTemplate) GetMarketMakingMetrics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return map[string]interface{}{
		"current_spread_bps":  m.spreadBps,
		"active_buy_orders":   len(m.buyOrders),
		"active_sell_orders":  len(m.sellOrders),
		"inventory":           m.inventory.String(),
		"inventory_value":     m.inventoryValue.String(),
		"filled_buy_volume":   m.filledBuyVolume.String(),
		"filled_sell_volume":  m.filledSellVolume.String(),
		"spread_capture":      m.spreadCapture.String(),
		"round_trips":         m.roundTrips,
		"current_volatility":  m.volatility,
		"mid_price":           m.midPrice.String(),
	}
}