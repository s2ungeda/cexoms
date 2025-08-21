package arbitrage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mExOms/internal/account"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// ArbitrageExecutor executes arbitrage opportunities
type ArbitrageExecutor struct {
	mu sync.RWMutex
	
	// Dependencies
	detector       *ArbitrageDetector
	accountRouter  *account.Router
	exchanges      map[string]types.ExchangeMultiAccount
	
	// Execution tracking
	activeExecutions map[string]*Execution
	executionHistory []*ExecutionResult
	
	// Risk management
	dailyVolume    decimal.Decimal
	dailyProfit    decimal.Decimal
	failureCount   int
	
	// Configuration
	config         *ExecutorConfig
	
	// Background workers
	stopCh         chan struct{}
	wg             sync.WaitGroup
}

// Execution represents an active arbitrage execution
type Execution struct {
	Opportunity    *ArbitrageOpportunity
	BuyOrder       *types.Order
	SellOrder      *types.Order
	Status         ExecutionStatus
	StartTime      time.Time
	EndTime        time.Time
	ActualProfit   decimal.Decimal
	ErrorMessage   string
}

// ExecutionStatus represents the status of an execution
type ExecutionStatus string

const (
	ExecutionPending    ExecutionStatus = "pending"
	ExecutionBuyPlaced  ExecutionStatus = "buy_placed"
	ExecutionSellPlaced ExecutionStatus = "sell_placed"
	ExecutionCompleted  ExecutionStatus = "completed"
	ExecutionFailed     ExecutionStatus = "failed"
	ExecutionRolledBack ExecutionStatus = "rolled_back"
)

// ExecutionResult represents the result of an arbitrage execution
type ExecutionResult struct {
	OpportunityID  string
	Symbol         string
	BuyExchange    string
	SellExchange   string
	Quantity       decimal.Decimal
	BuyPrice       decimal.Decimal
	SellPrice      decimal.Decimal
	GrossProfit    decimal.Decimal
	Fees           decimal.Decimal
	NetProfit      decimal.Decimal
	ExecutionTime  time.Duration
	Status         ExecutionStatus
	Timestamp      time.Time
	ErrorMessage   string
}

// ExecutorConfig contains configuration for arbitrage executor
type ExecutorConfig struct {
	// Execution parameters
	MaxConcurrentExecutions int
	ExecutionMode          ExecutionMode
	PartialFillTimeout     time.Duration
	
	// Risk limits
	MaxDailyVolume         decimal.Decimal
	MaxPositionSize        decimal.Decimal
	MaxDailyLoss           decimal.Decimal
	MaxConsecutiveFailures int
	
	// Order settings
	UseMarketOrders        bool
	SlippageTolerance      decimal.Decimal
	
	// Rollback settings
	EnableAutoRollback     bool
	RollbackTimeout        time.Duration
}

// ExecutionMode defines how orders are executed
type ExecutionMode string

const (
	ModeAggressive ExecutionMode = "aggressive" // Market orders, fast execution
	ModePassive    ExecutionMode = "passive"    // Limit orders, lower fees
	ModeHybrid     ExecutionMode = "hybrid"     // Start passive, become aggressive
)

// NewArbitrageExecutor creates a new arbitrage executor
func NewArbitrageExecutor(detector *ArbitrageDetector, accountRouter *account.Router, config *ExecutorConfig) *ArbitrageExecutor {
	if config == nil {
		config = &ExecutorConfig{
			MaxConcurrentExecutions: 5,
			ExecutionMode:           ModeHybrid,
			PartialFillTimeout:      2 * time.Second,
			MaxDailyVolume:          decimal.NewFromInt(1000000), // $1M
			MaxPositionSize:         decimal.NewFromInt(50000),   // $50k
			MaxDailyLoss:            decimal.NewFromInt(1000),    // $1k
			MaxConsecutiveFailures:  3,
			UseMarketOrders:         false,
			SlippageTolerance:       decimal.NewFromFloat(0.001), // 0.1%
			EnableAutoRollback:      true,
			RollbackTimeout:         5 * time.Second,
		}
	}
	
	return &ArbitrageExecutor{
		detector:         detector,
		accountRouter:    accountRouter,
		exchanges:        make(map[string]types.ExchangeMultiAccount),
		activeExecutions: make(map[string]*Execution),
		executionHistory: make([]*ExecutionResult, 0),
		config:           config,
		stopCh:           make(chan struct{}),
	}
}

// RegisterExchange registers an exchange for execution
func (ae *ArbitrageExecutor) RegisterExchange(name string, exchange types.ExchangeMultiAccount) {
	ae.mu.Lock()
	defer ae.mu.Unlock()
	
	ae.exchanges[name] = exchange
}

// Start starts the arbitrage executor
func (ae *ArbitrageExecutor) Start(ctx context.Context) error {
	// Start execution worker
	ae.wg.Add(1)
	go ae.executionWorker(ctx)
	
	// Start monitoring worker
	ae.wg.Add(1)
	go ae.monitoringWorker(ctx)
	
	return nil
}

// Stop stops the arbitrage executor
func (ae *ArbitrageExecutor) Stop() {
	close(ae.stopCh)
	ae.wg.Wait()
}

// executionWorker processes arbitrage opportunities
func (ae *ArbitrageExecutor) executionWorker(ctx context.Context) {
	defer ae.wg.Done()
	
	opportunityChan := ae.detector.GetOpportunityChannel()
	
	for {
		select {
		case opportunity := <-opportunityChan:
			// Check if we can execute
			if ae.canExecute(opportunity) {
				go ae.executeOpportunity(ctx, opportunity)
			}
			
		case <-ae.stopCh:
			return
		}
	}
}

// canExecute checks if an opportunity can be executed
func (ae *ArbitrageExecutor) canExecute(opportunity *ArbitrageOpportunity) bool {
	ae.mu.RLock()
	defer ae.mu.RUnlock()
	
	// Check concurrent executions
	if len(ae.activeExecutions) >= ae.config.MaxConcurrentExecutions {
		return false
	}
	
	// Check daily volume
	potentialVolume := ae.dailyVolume.Add(opportunity.MaxQuantity.Mul(opportunity.BuyPrice))
	if potentialVolume.GreaterThan(ae.config.MaxDailyVolume) {
		return false
	}
	
	// Check position size
	positionSize := opportunity.MaxQuantity.Mul(opportunity.BuyPrice)
	if positionSize.GreaterThan(ae.config.MaxPositionSize) {
		return false
	}
	
	// Check failure count
	if ae.failureCount >= ae.config.MaxConsecutiveFailures {
		return false
	}
	
	// Check daily loss
	if ae.dailyProfit.LessThan(ae.config.MaxDailyLoss.Neg()) {
		return false
	}
	
	return true
}

// executeOpportunity executes an arbitrage opportunity
func (ae *ArbitrageExecutor) executeOpportunity(ctx context.Context, opportunity *ArbitrageOpportunity) {
	execution := &Execution{
		Opportunity: opportunity,
		Status:      ExecutionPending,
		StartTime:   time.Now(),
	}
	
	// Track execution
	ae.mu.Lock()
	ae.activeExecutions[opportunity.ID] = execution
	ae.mu.Unlock()
	
	// Update detector status
	ae.detector.UpdateOpportunityStatus(opportunity.ID, StatusExecuting)
	
	// Execute with timeout
	execCtx, cancel := context.WithTimeout(ctx, ae.config.PartialFillTimeout)
	defer cancel()
	
	// Execute the arbitrage
	result := ae.doExecute(execCtx, execution)
	
	// Record result
	ae.recordExecutionResult(result)
	
	// Clean up
	ae.mu.Lock()
	delete(ae.activeExecutions, opportunity.ID)
	ae.mu.Unlock()
}

// doExecute performs the actual execution
func (ae *ArbitrageExecutor) doExecute(ctx context.Context, execution *Execution) *ExecutionResult {
	opportunity := execution.Opportunity
	result := &ExecutionResult{
		OpportunityID: opportunity.ID,
		Symbol:        opportunity.Symbol,
		BuyExchange:   opportunity.BuyExchange,
		SellExchange:  opportunity.SellExchange,
		Timestamp:     time.Now(),
	}
	
	// Get exchanges
	buyExchange, buyExists := ae.exchanges[opportunity.BuyExchange]
	sellExchange, sellExists := ae.exchanges[opportunity.SellExchange]
	
	if !buyExists || !sellExists {
		result.Status = ExecutionFailed
		result.ErrorMessage = "Exchange not found"
		return result
	}
	
	// Route to best accounts
	buyAccount, err := ae.accountRouter.RouteOrder(ctx, opportunity.BuyExchange, &types.Order{
		Symbol:   opportunity.Symbol,
		Side:     types.OrderSideBuy,
		Quantity: opportunity.MaxQuantity,
		Price:    opportunity.BuyPrice,
	})
	if err != nil {
		result.Status = ExecutionFailed
		result.ErrorMessage = fmt.Sprintf("Failed to route buy order: %v", err)
		return result
	}
	
	sellAccount, err := ae.accountRouter.RouteOrder(ctx, opportunity.SellExchange, &types.Order{
		Symbol:   opportunity.Symbol,
		Side:     types.OrderSideSell,
		Quantity: opportunity.MaxQuantity,
		Price:    opportunity.SellPrice,
	})
	if err != nil {
		result.Status = ExecutionFailed
		result.ErrorMessage = fmt.Sprintf("Failed to route sell order: %v", err)
		return result
	}
	
	// Set accounts
	buyExchange.SetAccount(buyAccount.ID)
	sellExchange.SetAccount(sellAccount.ID)
	
	// Create orders
	buyOrder := &types.Order{
		ClientOrderID: fmt.Sprintf("arb_buy_%s", opportunity.ID),
		Symbol:        opportunity.Symbol,
		Side:          types.OrderSideBuy,
		Quantity:      opportunity.MaxQuantity,
		TimeInForce:   types.TimeInForceIOC, // Immediate or cancel
	}
	
	sellOrder := &types.Order{
		ClientOrderID: fmt.Sprintf("arb_sell_%s", opportunity.ID),
		Symbol:        opportunity.Symbol,
		Side:          types.OrderSideSell,
		Quantity:      opportunity.MaxQuantity,
		TimeInForce:   types.TimeInForceIOC,
	}
	
	// Set order type and price based on execution mode
	if ae.config.UseMarketOrders || ae.config.ExecutionMode == ModeAggressive {
		buyOrder.Type = types.OrderTypeMarket
		sellOrder.Type = types.OrderTypeMarket
	} else {
		buyOrder.Type = types.OrderTypeLimit
		sellOrder.Type = types.OrderTypeLimit
		
		// Add slippage tolerance
		buyOrder.Price = opportunity.BuyPrice.Mul(decimal.NewFromFloat(1).Add(ae.config.SlippageTolerance))
		sellOrder.Price = opportunity.SellPrice.Mul(decimal.NewFromFloat(1).Sub(ae.config.SlippageTolerance))
	}
	
	// Execute orders concurrently
	var buyErr, sellErr error
	var executedBuy, executedSell *types.Order
	var wg sync.WaitGroup
	
	wg.Add(2)
	
	// Execute buy order
	go func() {
		defer wg.Done()
		executedBuy, buyErr = buyExchange.PlaceOrder(ctx, buyOrder)
		if buyErr == nil {
			execution.BuyOrder = executedBuy
			execution.Status = ExecutionBuyPlaced
		}
	}()
	
	// Execute sell order
	go func() {
		defer wg.Done()
		executedSell, sellErr = sellExchange.PlaceOrder(ctx, sellOrder)
		if sellErr == nil {
			execution.SellOrder = executedSell
			execution.Status = ExecutionSellPlaced
		}
	}()
	
	wg.Wait()
	
	// Check execution results
	if buyErr != nil || sellErr != nil {
		// Rollback if enabled
		if ae.config.EnableAutoRollback {
			ae.rollbackExecution(ctx, execution, executedBuy, executedSell, buyExchange, sellExchange)
		}
		
		result.Status = ExecutionFailed
		result.ErrorMessage = fmt.Sprintf("Buy error: %v, Sell error: %v", buyErr, sellErr)
		return result
	}
	
	// Calculate actual execution
	if executedBuy != nil && executedSell != nil {
		result.Quantity = decimal.Min(executedBuy.ExecutedQty, executedSell.ExecutedQty)
		result.BuyPrice = executedBuy.AvgPrice
		result.SellPrice = executedSell.AvgPrice
		
		// Calculate profit
		result.GrossProfit = result.Quantity.Mul(result.SellPrice.Sub(result.BuyPrice))
		result.Fees = opportunity.BuyFee.Add(opportunity.SellFee).Mul(result.Quantity)
		result.NetProfit = result.GrossProfit.Sub(result.Fees)
		
		execution.Status = ExecutionCompleted
		result.Status = ExecutionCompleted
	}
	
	execution.EndTime = time.Now()
	result.ExecutionTime = execution.EndTime.Sub(execution.StartTime)
	
	return result
}

// rollbackExecution attempts to rollback a failed execution
func (ae *ArbitrageExecutor) rollbackExecution(ctx context.Context, execution *Execution, 
	buyOrder, sellOrder *types.Order, buyExchange, sellExchange types.ExchangeMultiAccount) {
	
	rollbackCtx, cancel := context.WithTimeout(ctx, ae.config.RollbackTimeout)
	defer cancel()
	
	// Cancel or reverse executed orders
	if buyOrder != nil && buyOrder.Status != types.OrderStatusCanceled {
		if buyOrder.ExecutedQty.IsZero() {
			// Cancel unfilled buy order
			buyExchange.CancelOrder(rollbackCtx, buyOrder.Symbol, buyOrder.ExchangeOrderID)
		} else {
			// Create opposite sell order to flatten position
			reverseOrder := &types.Order{
				ClientOrderID: fmt.Sprintf("arb_rollback_%s", buyOrder.ClientOrderID),
				Symbol:        buyOrder.Symbol,
				Side:          types.OrderSideSell,
				Type:          types.OrderTypeMarket,
				Quantity:      buyOrder.ExecutedQty,
			}
			buyExchange.PlaceOrder(rollbackCtx, reverseOrder)
		}
	}
	
	if sellOrder != nil && sellOrder.Status != types.OrderStatusCanceled {
		if sellOrder.ExecutedQty.IsZero() {
			// Cancel unfilled sell order
			sellExchange.CancelOrder(rollbackCtx, sellOrder.Symbol, sellOrder.ExchangeOrderID)
		} else {
			// Create opposite buy order to flatten position
			reverseOrder := &types.Order{
				ClientOrderID: fmt.Sprintf("arb_rollback_%s", sellOrder.ClientOrderID),
				Symbol:        sellOrder.Symbol,
				Side:          types.OrderSideBuy,
				Type:          types.OrderTypeMarket,
				Quantity:      sellOrder.FilledQuantity,
			}
			sellExchange.PlaceOrder(rollbackCtx, reverseOrder)
		}
	}
	
	execution.Status = ExecutionRolledBack
}

// recordExecutionResult records the execution result
func (ae *ArbitrageExecutor) recordExecutionResult(result *ExecutionResult) {
	ae.mu.Lock()
	defer ae.mu.Unlock()
	
	// Update history
	ae.executionHistory = append(ae.executionHistory, result)
	
	// Update daily metrics
	if result.Status == ExecutionCompleted {
		ae.dailyVolume = ae.dailyVolume.Add(result.Quantity.Mul(result.BuyPrice))
		ae.dailyProfit = ae.dailyProfit.Add(result.NetProfit)
		ae.failureCount = 0 // Reset failure count on success
	} else {
		ae.failureCount++
	}
	
	// Keep only recent history (last 1000 executions)
	if len(ae.executionHistory) > 1000 {
		ae.executionHistory = ae.executionHistory[len(ae.executionHistory)-1000:]
	}
}

// monitoringWorker monitors active executions
func (ae *ArbitrageExecutor) monitoringWorker(ctx context.Context) {
	defer ae.wg.Done()
	
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			ae.checkActiveExecutions()
			ae.resetDailyMetrics()
			
		case <-ae.stopCh:
			return
		}
	}
}

// checkActiveExecutions checks status of active executions
func (ae *ArbitrageExecutor) checkActiveExecutions() {
	ae.mu.RLock()
	defer ae.mu.RUnlock()
	
	now := time.Now()
	for _, execution := range ae.activeExecutions {
		// Check for stuck executions
		if now.Sub(execution.StartTime) > ae.config.PartialFillTimeout*2 {
			execution.Status = ExecutionFailed
			execution.ErrorMessage = "Execution timeout"
		}
	}
}

// resetDailyMetrics resets daily metrics at midnight
func (ae *ArbitrageExecutor) resetDailyMetrics() {
	now := time.Now()
	if now.Hour() == 0 && now.Minute() == 0 {
		ae.mu.Lock()
		ae.dailyVolume = decimal.Zero
		ae.dailyProfit = decimal.Zero
		ae.mu.Unlock()
	}
}

// GetExecutionStats returns execution statistics
func (ae *ArbitrageExecutor) GetExecutionStats() *ExecutionStats {
	ae.mu.RLock()
	defer ae.mu.RUnlock()
	
	stats := &ExecutionStats{
		ActiveExecutions:   len(ae.activeExecutions),
		TotalExecutions:    len(ae.executionHistory),
		DailyVolume:        ae.dailyVolume,
		DailyProfit:        ae.dailyProfit,
		ConsecutiveFailures: ae.failureCount,
	}
	
	// Calculate success rate
	successCount := 0
	totalProfit := decimal.Zero
	
	for _, result := range ae.executionHistory {
		if result.Status == ExecutionCompleted {
			successCount++
			totalProfit = totalProfit.Add(result.NetProfit)
		}
	}
	
	if stats.TotalExecutions > 0 {
		stats.SuccessRate = float64(successCount) / float64(stats.TotalExecutions)
	}
	
	stats.TotalProfit = totalProfit
	
	return stats
}

// ExecutionStats contains execution statistics
type ExecutionStats struct {
	ActiveExecutions    int
	TotalExecutions     int
	SuccessRate         float64
	DailyVolume         decimal.Decimal
	DailyProfit         decimal.Decimal
	TotalProfit         decimal.Decimal
	ConsecutiveFailures int
}