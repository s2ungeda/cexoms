package position

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
	
	"github.com/mExOms/pkg/nats"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// SharedMemoryPosition represents position data in shared memory
type SharedMemoryPosition struct {
	Symbol        [32]byte  // Fixed size for shared memory
	Exchange      [16]byte
	Market        [16]byte
	Side          [8]byte
	Quantity      float64
	EntryPrice    float64
	MarkPrice     float64
	UnrealizedPnL float64
	RealizedPnL   float64
	Leverage      int32
	MarginUsed    float64
	UpdatedAt     int64
	_padding      [40]byte // Padding for cache line alignment (128 bytes total)
}

// PositionManager manages positions across all exchanges with shared memory
type PositionManager struct {
	// Shared memory
	shmFd       int
	shmSize     int
	shmPtr      unsafe.Pointer
	maxPositions int
	
	// Local cache (for fast access)
	positions    sync.Map // key: "exchange:symbol" -> *Position
	
	// Performance metrics
	updateCount  atomic.Uint64
	readCount    atomic.Uint64
	pnlCalcTime  atomic.Int64 // nanoseconds
	
	// Snapshot configuration
	snapshotDir  string
	snapshotInterval time.Duration
	stopSnapshot chan struct{}
	
	// Market prices cache
	markPrices   sync.Map // key: "exchange:symbol" -> decimal.Decimal
	
	// NATS client for publishing position updates
	natsClient  *nats.Client
	logger      *zap.Logger
}

// PositionUpdateEvent represents a position update event for NATS
type PositionUpdateEvent struct {
	EventType     string          `json:"event_type"`  // OPEN, UPDATE, CLOSE, LIQUIDATED
	Position      *Position       `json:"position"`
	Timestamp     time.Time       `json:"timestamp"`
	PrevQuantity  decimal.Decimal `json:"prev_quantity,omitempty"`
	PrevMarkPrice decimal.Decimal `json:"prev_mark_price,omitempty"`
	PrevPnL       decimal.Decimal `json:"prev_pnl,omitempty"`
}

// Position represents a trading position
type Position struct {
	Symbol        string
	Exchange      string
	Market        string
	Side          string
	Quantity      decimal.Decimal
	EntryPrice    decimal.Decimal
	MarkPrice     decimal.Decimal
	UnrealizedPnL decimal.Decimal
	RealizedPnL   decimal.Decimal
	Leverage      int
	MarginUsed    decimal.Decimal
	UpdatedAt     time.Time
	
	// Calculated fields
	PositionValue decimal.Decimal
	PnLPercent    decimal.Decimal
	MarginRatio   decimal.Decimal
}

// AggregatedPosition represents positions aggregated across exchanges
type AggregatedPosition struct {
	Symbol         string
	TotalQuantity  decimal.Decimal
	AvgEntryPrice  decimal.Decimal
	TotalValue     decimal.Decimal
	TotalPnL       decimal.Decimal
	Positions      []*Position
}

// NewPositionManager creates a new position manager with shared memory
func NewPositionManager(snapshotDir string, natsClient *nats.Client, logger *zap.Logger) (*PositionManager, error) {
	pm := &PositionManager{
		maxPositions:     1000, // Support up to 1000 positions
		snapshotDir:      snapshotDir,
		snapshotInterval: 5 * time.Minute,
		stopSnapshot:     make(chan struct{}),
		natsClient:       natsClient,
		logger:           logger,
	}
	
	// Initialize shared memory
	if err := pm.initSharedMemory(); err != nil {
		return nil, fmt.Errorf("failed to init shared memory: %w", err)
	}
	
	// Load existing snapshot
	if err := pm.loadSnapshot(); err != nil {
		// Log error but continue
		logger.Warn("Failed to load snapshot", zap.Error(err))
	}
	
	// Start snapshot routine
	go pm.snapshotRoutine()
	
	return pm, nil
}

// initSharedMemory initializes shared memory for position tracking
func (pm *PositionManager) initSharedMemory() error {
	// Calculate shared memory size
	pm.shmSize = int(unsafe.Sizeof(SharedMemoryPosition{})) * pm.maxPositions
	
	// Create shared memory
	shmName := "/dev/shm/oms_positions"
	fd, err := syscall.Open(shmName, syscall.O_RDWR|syscall.O_CREAT, 0666)
	if err != nil {
		return fmt.Errorf("failed to open shared memory: %w", err)
	}
	pm.shmFd = fd
	
	// Resize shared memory
	if err := syscall.Ftruncate(fd, int64(pm.shmSize)); err != nil {
		return fmt.Errorf("failed to resize shared memory: %w", err)
	}
	
	// Map shared memory
	data, err := syscall.Mmap(fd, 0, pm.shmSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("failed to map shared memory: %w", err)
	}
	
	pm.shmPtr = unsafe.Pointer(&data[0])
	
	// Initialize shared memory to zero
	for i := 0; i < pm.shmSize; i++ {
		data[i] = 0
	}
	
	// Lock memory to prevent swapping (for performance)
	if err := syscall.Mlock(data); err != nil {
		// Non-critical error
		pm.logger.Warn("Failed to lock memory", zap.Error(err))
	}
	
	return nil
}

// UpdatePosition updates or creates a position
func (pm *PositionManager) UpdatePosition(pos *Position) error {
	start := time.Now()
	defer func() {
		pm.pnlCalcTime.Store(time.Since(start).Nanoseconds())
		pm.updateCount.Add(1)
	}()
	
	key := fmt.Sprintf("%s:%s", pos.Exchange, pos.Symbol)
	
	// Check if this is a new position or update
	var prevPosition *Position
	var eventType string
	if val, exists := pm.positions.Load(key); exists {
		prevPosition = val.(*Position)
		eventType = "UPDATE"
		
		// Check if position is being closed
		if pos.Quantity.IsZero() && !prevPosition.Quantity.IsZero() {
			eventType = "CLOSE"
		}
	} else {
		eventType = "OPEN"
	}
	
	// Calculate derived fields
	pos.PositionValue = pos.Quantity.Abs().Mul(pos.MarkPrice)
	
	if pos.Side == "LONG" || pos.Side == "BUY" {
		pos.UnrealizedPnL = pos.Quantity.Mul(pos.MarkPrice.Sub(pos.EntryPrice))
	} else {
		pos.UnrealizedPnL = pos.Quantity.Abs().Mul(pos.EntryPrice.Sub(pos.MarkPrice))
	}
	
	if !pos.EntryPrice.IsZero() {
		pos.PnLPercent = pos.UnrealizedPnL.Div(pos.Quantity.Abs().Mul(pos.EntryPrice)).Mul(decimal.NewFromInt(100))
	}
	
	if pos.Leverage > 0 && !pos.MarginUsed.IsZero() {
		pos.MarginRatio = pos.PositionValue.Div(pos.MarginUsed.Mul(decimal.NewFromInt(int64(pos.Leverage))))
	}
	
	pos.UpdatedAt = time.Now()
	
	// Update local cache
	pm.positions.Store(key, pos)
	
	// Update shared memory
	if err := pm.updateSharedMemory(pos); err != nil {
		return fmt.Errorf("failed to update shared memory: %w", err)
	}
	
	// Publish position update event
	event := PositionUpdateEvent{
		EventType: eventType,
		Position:  pos,
		Timestamp: time.Now(),
	}
	
	if prevPosition != nil {
		event.PrevQuantity = prevPosition.Quantity
		event.PrevMarkPrice = prevPosition.MarkPrice
		event.PrevPnL = prevPosition.UnrealizedPnL
	}
	
	if err := pm.publishPositionEvent(event); err != nil {
		pm.logger.Error("Failed to publish position event", 
			zap.Error(err), 
			zap.String("exchange", pos.Exchange),
			zap.String("symbol", pos.Symbol))
	}
	
	return nil
}

// updateSharedMemory updates position in shared memory
func (pm *PositionManager) updateSharedMemory(pos *Position) error {
	// Find empty slot or matching position
	for i := 0; i < pm.maxPositions; i++ {
		shmPos := (*SharedMemoryPosition)(unsafe.Pointer(uintptr(pm.shmPtr) + uintptr(i)*unsafe.Sizeof(SharedMemoryPosition{})))
		
		// Check if slot is empty or matches
		symbolBytes := shmPos.Symbol[:]
		exchangeBytes := shmPos.Exchange[:]
		
		// Check if empty (first byte is 0)
		isEmpty := symbolBytes[0] == 0 && exchangeBytes[0] == 0
		
		// Check if matches
		symbolMatch := trimNull(string(symbolBytes)) == pos.Symbol
		exchangeMatch := trimNull(string(exchangeBytes)) == pos.Exchange
		
		if isEmpty || (symbolMatch && exchangeMatch) {
			// Update shared memory position
			copy(shmPos.Symbol[:], []byte(pos.Symbol))
			copy(shmPos.Exchange[:], []byte(pos.Exchange))
			copy(shmPos.Market[:], []byte(pos.Market))
			copy(shmPos.Side[:], []byte(pos.Side))
			
			shmPos.Quantity = pos.Quantity.InexactFloat64()
			shmPos.EntryPrice = pos.EntryPrice.InexactFloat64()
			shmPos.MarkPrice = pos.MarkPrice.InexactFloat64()
			shmPos.UnrealizedPnL = pos.UnrealizedPnL.InexactFloat64()
			shmPos.RealizedPnL = pos.RealizedPnL.InexactFloat64()
			shmPos.Leverage = int32(pos.Leverage)
			shmPos.MarginUsed = pos.MarginUsed.InexactFloat64()
			shmPos.UpdatedAt = pos.UpdatedAt.Unix()
			
			return nil
		}
	}
	
	return fmt.Errorf("no available slot for position")
}

// GetPosition retrieves a position by exchange and symbol
func (pm *PositionManager) GetPosition(exchange, symbol string) (*Position, bool) {
	pm.readCount.Add(1)
	
	key := fmt.Sprintf("%s:%s", exchange, symbol)
	if val, exists := pm.positions.Load(key); exists {
		return val.(*Position), true
	}
	return nil, false
}

// GetAllPositions returns all positions
func (pm *PositionManager) GetAllPositions() []*Position {
	pm.readCount.Add(1)
	
	var positions []*Position
	pm.positions.Range(func(key, value interface{}) bool {
		positions = append(positions, value.(*Position))
		return true
	})
	
	return positions
}

// GetPositionsByExchange returns all positions for an exchange
func (pm *PositionManager) GetPositionsByExchange(exchange string) []*Position {
	pm.readCount.Add(1)
	
	var positions []*Position
	pm.positions.Range(func(key, value interface{}) bool {
		pos := value.(*Position)
		if pos.Exchange == exchange {
			positions = append(positions, pos)
		}
		return true
	})
	
	return positions
}

// GetAggregatedPositions returns positions aggregated by symbol across exchanges
func (pm *PositionManager) GetAggregatedPositions() map[string]*AggregatedPosition {
	aggregated := make(map[string]*AggregatedPosition)
	
	pm.positions.Range(func(key, value interface{}) bool {
		pos := value.(*Position)
		
		if agg, exists := aggregated[pos.Symbol]; exists {
			// Update aggregated position
			totalValue := agg.AvgEntryPrice.Mul(agg.TotalQuantity)
			newValue := pos.EntryPrice.Mul(pos.Quantity)
			
			agg.TotalQuantity = agg.TotalQuantity.Add(pos.Quantity)
			if !agg.TotalQuantity.IsZero() {
				agg.AvgEntryPrice = totalValue.Add(newValue).Div(agg.TotalQuantity)
			}
			agg.TotalValue = agg.TotalValue.Add(pos.PositionValue)
			agg.TotalPnL = agg.TotalPnL.Add(pos.UnrealizedPnL)
			agg.Positions = append(agg.Positions, pos)
		} else {
			// Create new aggregated position
			aggregated[pos.Symbol] = &AggregatedPosition{
				Symbol:        pos.Symbol,
				TotalQuantity: pos.Quantity,
				AvgEntryPrice: pos.EntryPrice,
				TotalValue:    pos.PositionValue,
				TotalPnL:      pos.UnrealizedPnL,
				Positions:     []*Position{pos},
			}
		}
		
		return true
	})
	
	return aggregated
}

// UpdateMarkPrice updates the mark price for a symbol
func (pm *PositionManager) UpdateMarkPrice(exchange, symbol string, markPrice decimal.Decimal) {
	key := fmt.Sprintf("%s:%s", exchange, symbol)
	pm.markPrices.Store(key, markPrice)
	
	// Update position if exists
	if pos, exists := pm.GetPosition(exchange, symbol); exists {
		pos.MarkPrice = markPrice
		pm.UpdatePosition(pos)
	}
}

// CalculateTotalPnL calculates total P&L across all positions
func (pm *PositionManager) CalculateTotalPnL() (unrealized, realized decimal.Decimal) {
	pm.positions.Range(func(key, value interface{}) bool {
		pos := value.(*Position)
		unrealized = unrealized.Add(pos.UnrealizedPnL)
		realized = realized.Add(pos.RealizedPnL)
		return true
	})
	
	return unrealized, realized
}

// CalculateExchangePnL calculates P&L for a specific exchange
func (pm *PositionManager) CalculateExchangePnL(exchange string) (unrealized, realized decimal.Decimal) {
	pm.positions.Range(func(key, value interface{}) bool {
		pos := value.(*Position)
		if pos.Exchange == exchange {
			unrealized = unrealized.Add(pos.UnrealizedPnL)
			realized = realized.Add(pos.RealizedPnL)
		}
		return true
	})
	
	return unrealized, realized
}

// GetRiskMetrics calculates risk metrics across all positions
func (pm *PositionManager) GetRiskMetrics() map[string]interface{} {
	var totalValue, totalMargin, maxLeverage decimal.Decimal
	positionCount := 0
	
	pm.positions.Range(func(key, value interface{}) bool {
		pos := value.(*Position)
		totalValue = totalValue.Add(pos.PositionValue)
		totalMargin = totalMargin.Add(pos.MarginUsed)
		if decimal.NewFromInt(int64(pos.Leverage)).GreaterThan(maxLeverage) {
			maxLeverage = decimal.NewFromInt(int64(pos.Leverage))
		}
		positionCount++
		return true
	})
	
	unrealizedPnL, realizedPnL := pm.CalculateTotalPnL()
	
	avgCalcTime := float64(0)
	if updates := pm.updateCount.Load(); updates > 0 {
		avgCalcTime = float64(pm.pnlCalcTime.Load()) / float64(updates)
	}
	
	return map[string]interface{}{
		"position_count":      positionCount,
		"total_value":         totalValue.String(),
		"total_margin_used":   totalMargin.String(),
		"max_leverage":        maxLeverage.String(),
		"unrealized_pnl":      unrealizedPnL.String(),
		"realized_pnl":        realizedPnL.String(),
		"total_pnl":           unrealizedPnL.Add(realizedPnL).String(),
		"updates_count":       pm.updateCount.Load(),
		"reads_count":         pm.readCount.Load(),
		"avg_calc_time_ns":    avgCalcTime,
		"avg_calc_time_us":    avgCalcTime / 1000,
	}
}

// SaveSnapshot saves current positions to file
func (pm *PositionManager) SaveSnapshot() error {
	positions := pm.GetAllPositions()
	
	// Create snapshot data
	snapshot := struct {
		Timestamp time.Time    `json:"timestamp"`
		Positions []*Position  `json:"positions"`
		Metrics   map[string]interface{} `json:"metrics"`
	}{
		Timestamp: time.Now(),
		Positions: positions,
		Metrics:   pm.GetRiskMetrics(),
	}
	
	// Marshal to JSON
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}
	
	// Create snapshot directory
	snapshotPath := filepath.Join(pm.snapshotDir, 
		time.Now().Format("2006/01/02/15"))
	if err := os.MkdirAll(snapshotPath, 0755); err != nil {
		return fmt.Errorf("failed to create snapshot dir: %w", err)
	}
	
	// Write snapshot file
	filename := filepath.Join(snapshotPath, 
		fmt.Sprintf("positions_%s.json", time.Now().Format("150405")))
	
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write snapshot: %w", err)
	}
	
	return nil
}

// loadSnapshot loads the most recent snapshot
func (pm *PositionManager) loadSnapshot() error {
	// Find most recent snapshot
	var latestFile string
	var latestTime time.Time
	
	err := filepath.Walk(pm.snapshotDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		
		if filepath.Ext(path) == ".json" && info.ModTime().After(latestTime) {
			latestFile = path
			latestTime = info.ModTime()
		}
		
		return nil
	})
	
	if err != nil || latestFile == "" {
		return fmt.Errorf("no snapshot found")
	}
	
	// Read snapshot file
	data, err := os.ReadFile(latestFile)
	if err != nil {
		return fmt.Errorf("failed to read snapshot: %w", err)
	}
	
	// Unmarshal snapshot
	var snapshot struct {
		Timestamp time.Time    `json:"timestamp"`
		Positions []*Position  `json:"positions"`
		Metrics   map[string]interface{} `json:"metrics"`
	}
	
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}
	
	// Load positions
	for _, pos := range snapshot.Positions {
		key := fmt.Sprintf("%s:%s", pos.Exchange, pos.Symbol)
		pm.positions.Store(key, pos)
		pm.updateSharedMemory(pos)
	}
	
	pm.logger.Info("Loaded snapshot", 
		zap.Time("timestamp", snapshot.Timestamp),
		zap.Int("positions", len(snapshot.Positions)))
	
	return nil
}

// snapshotRoutine runs periodic snapshots
func (pm *PositionManager) snapshotRoutine() {
	ticker := time.NewTicker(pm.snapshotInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			if err := pm.SaveSnapshot(); err != nil {
				pm.logger.Error("Failed to save snapshot", zap.Error(err))
			}
		case <-pm.stopSnapshot:
			return
		}
	}
}

// Close closes the position manager
func (pm *PositionManager) Close() error {
	// Stop snapshot routine
	close(pm.stopSnapshot)
	
	// Save final snapshot
	pm.SaveSnapshot()
	
	// Unmap shared memory
	if pm.shmPtr != nil {
		data := (*[1 << 30]byte)(pm.shmPtr)[:pm.shmSize:pm.shmSize]
		syscall.Munlock(data)
		syscall.Munmap(data)
	}
	
	// Close shared memory file descriptor
	if pm.shmFd > 0 {
		syscall.Close(pm.shmFd)
	}
	
	return nil
}

// publishPositionEvent publishes position event to NATS
func (pm *PositionManager) publishPositionEvent(event PositionUpdateEvent) error {
	if pm.natsClient == nil {
		return nil // NATS client not configured
	}
	
	// Subject format: position.update.{exchange}.{market}.{symbol}
	subject := fmt.Sprintf("position.update.%s.%s.%s", 
		event.Position.Exchange, event.Position.Market, event.Position.Symbol)
	
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal position event: %w", err)
	}
	
	return pm.natsClient.Publish(subject, data)
}

// ClosePosition closes a position
func (pm *PositionManager) ClosePosition(exchange, symbol string, realizedPnL decimal.Decimal) error {
	key := fmt.Sprintf("%s:%s", exchange, symbol)
	
	val, exists := pm.positions.Load(key)
	if !exists {
		return fmt.Errorf("position not found: %s:%s", exchange, symbol)
	}
	
	pos := val.(*Position)
	pos.Quantity = decimal.Zero
	pos.RealizedPnL = pos.RealizedPnL.Add(realizedPnL)
	pos.UnrealizedPnL = decimal.Zero
	pos.UpdatedAt = time.Now()
	
	// Remove from local cache
	pm.positions.Delete(key)
	
	// Publish close event
	event := PositionUpdateEvent{
		EventType: "CLOSE",
		Position:  pos,
		Timestamp: time.Now(),
	}
	
	if err := pm.publishPositionEvent(event); err != nil {
		pm.logger.Error("Failed to publish position close event", 
			zap.Error(err), 
			zap.String("exchange", exchange),
			zap.String("symbol", symbol))
	}
	
	return nil
}

// AddPosition adds a new position or updates existing one
func (pm *PositionManager) AddPosition(pos *Position) error {
	key := fmt.Sprintf("%s:%s", pos.Exchange, pos.Symbol)
	
	// Check if position already exists
	if val, exists := pm.positions.Load(key); exists {
		existingPos := val.(*Position)
		
		// Merge positions
		if existingPos.Side == pos.Side {
			// Same side - average the entry price
			totalValue := existingPos.Quantity.Mul(existingPos.EntryPrice).Add(
				pos.Quantity.Mul(pos.EntryPrice))
			totalQuantity := existingPos.Quantity.Add(pos.Quantity)
			
			pos.Quantity = totalQuantity
			if !totalQuantity.IsZero() {
				pos.EntryPrice = totalValue.Div(totalQuantity)
			}
			pos.RealizedPnL = existingPos.RealizedPnL
		} else {
			// Opposite side - calculate realized P&L
			closeQuantity := decimal.Min(existingPos.Quantity.Abs(), pos.Quantity.Abs())
			
			var realizedPnL decimal.Decimal
			if existingPos.Side == "LONG" || existingPos.Side == "BUY" {
				realizedPnL = closeQuantity.Mul(pos.EntryPrice.Sub(existingPos.EntryPrice))
			} else {
				realizedPnL = closeQuantity.Mul(existingPos.EntryPrice.Sub(pos.EntryPrice))
			}
			
			pos.RealizedPnL = existingPos.RealizedPnL.Add(realizedPnL)
			
			// Update remaining quantity
			remainingQuantity := existingPos.Quantity.Sub(closeQuantity)
			if remainingQuantity.IsNegative() {
				pos.Quantity = remainingQuantity.Neg()
				pos.Side = pos.Side
			} else if remainingQuantity.IsPositive() {
				pos.Quantity = remainingQuantity
				pos.Side = existingPos.Side
				pos.EntryPrice = existingPos.EntryPrice
			} else {
				pos.Quantity = decimal.Zero
			}
		}
	}
	
	return pm.UpdatePosition(pos)
}

// trimNull removes null bytes from string
func trimNull(s string) string {
	for i, c := range s {
		if c == 0 {
			return s[:i]
		}
	}
	return s
}