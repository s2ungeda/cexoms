package router

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mExOms/internal/exchange"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// ExecutionEngine handles order execution across multiple exchanges
type ExecutionEngine struct {
	exchangeManager *exchange.Manager
	config          *ExecutionConfig
	
	// Execution tracking
	activeExecutions sync.Map // executionID -> *ExecutionContext
	executionCount   atomic.Int64
	
	// Performance metrics
	metrics *ExecutionMetrics
	
	// Worker pool for parallel execution
	workerPool *WorkerPool
}

// ExecutionConfig contains execution engine configuration
type ExecutionConfig struct {
	// Parallelism settings
	MaxConcurrentOrders int
	WorkerPoolSize      int
	
	// Timeout settings
	OrderTimeout        time.Duration
	ExecutionTimeout    time.Duration
	
	// Retry settings
	MaxRetries          int
	RetryDelay          time.Duration
	
	// Monitoring
	MonitoringInterval  time.Duration
	
	// Fee optimization
	EnableFeeOptimization bool
	EnableRebates        bool
}

// ExecutionContext tracks the execution of a routing decision
type ExecutionContext struct {
	ID              string
	RoutingDecision *RoutingDecision
	Status          ExecutionStatus
	StartTime       time.Time
	
	// Execution state
	executedRoutes  map[string]*ExecutedRoute // routeID -> execution result
	mu              sync.RWMutex
	
	// Channels for coordination
	done            chan struct{}
	cancel          context.CancelFunc
	
	// Results
	report          *ExecutionReport
}

// ExecutedRoute tracks execution of a single route
type ExecutedRoute struct {
	Route           Route
	OrderID         string
	Status          string
	ExecutedQty     decimal.Decimal
	AveragePrice    decimal.Decimal
	Fee             decimal.Decimal
	StartTime       time.Time
	EndTime         time.Time
	Error           error
}

// ExecutionMetrics tracks execution performance
type ExecutionMetrics struct {
	mu                sync.RWMutex
	TotalExecutions   int64
	SuccessfulOrders  int64
	FailedOrders      int64
	PartialFills      int64
	TotalVolume       decimal.Decimal
	TotalFees         decimal.Decimal
	AverageLatency    time.Duration
	LastUpdateTime    time.Time
}

// NewExecutionEngine creates a new execution engine
func NewExecutionEngine(exchangeManager *exchange.Manager, config *ExecutionConfig) *ExecutionEngine {
	if config == nil {
		config = &ExecutionConfig{
			MaxConcurrentOrders:   100,
			WorkerPoolSize:        20,
			OrderTimeout:          30 * time.Second,
			ExecutionTimeout:      5 * time.Minute,
			MaxRetries:            3,
			RetryDelay:            1 * time.Second,
			MonitoringInterval:    100 * time.Millisecond,
			EnableFeeOptimization: true,
			EnableRebates:         true,
		}
	}
	
	engine := &ExecutionEngine{
		exchangeManager: exchangeManager,
		config:          config,
		metrics:         &ExecutionMetrics{},
	}
	
	// Initialize worker pool
	engine.workerPool = NewWorkerPool(config.WorkerPoolSize)
	engine.workerPool.Start()
	
	return engine
}

// Execute executes a routing decision
func (e *ExecutionEngine) Execute(ctx context.Context, decision *RoutingDecision) (*ExecutionReport, error) {
	// Create execution context
	execCtx, cancel := context.WithTimeout(ctx, e.config.ExecutionTimeout)
	defer cancel()
	
	execution := &ExecutionContext{
		ID:              fmt.Sprintf("exec_%d", e.executionCount.Add(1)),
		RoutingDecision: decision,
		Status:          ExecutionStatusPending,
		StartTime:       time.Now(),
		executedRoutes:  make(map[string]*ExecutedRoute),
		done:            make(chan struct{}),
		cancel:          cancel,
	}
	
	// Store active execution
	e.activeExecutions.Store(execution.ID, execution)
	defer e.activeExecutions.Delete(execution.ID)
	
	// Start monitoring
	go e.monitorExecution(execution)
	
	// Execute routes
	if err := e.executeRoutes(execCtx, execution); err != nil {
		execution.Status = ExecutionStatusFailed
		return execution.report, err
	}
	
	// Wait for completion
	select {
	case <-execution.done:
		return execution.report, nil
	case <-execCtx.Done():
		execution.Status = ExecutionStatusCancelled
		return execution.report, fmt.Errorf("execution timeout")
	}
}

// executeRoutes executes all routes in the decision
func (e *ExecutionEngine) executeRoutes(ctx context.Context, execution *ExecutionContext) error {
	routes := execution.RoutingDecision.Routes
	
	// Group routes by priority
	priorityGroups := e.groupRoutesByPriority(routes)
	
	// Execute each priority group
	for _, group := range priorityGroups {
		if err := e.executeRouteGroup(ctx, execution, group); err != nil {
			return err
		}
		
		// Check if we've filled the order
		if e.isOrderFilled(execution) {
			break
		}
	}
	
	return nil
}

// executeRouteGroup executes a group of routes with the same priority
func (e *ExecutionEngine) executeRouteGroup(ctx context.Context, execution *ExecutionContext, routes []Route) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(routes))
	
	// Use semaphore to limit concurrent orders
	sem := make(chan struct{}, e.config.MaxConcurrentOrders)
	
	for _, route := range routes {
		wg.Add(1)
		route := route // capture for goroutine
		
		// Submit to worker pool
		e.workerPool.Submit(func() {
			defer wg.Done()
			
			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			}
			
			// Execute route
			if err := e.executeSingleRoute(ctx, execution, route); err != nil {
				errChan <- fmt.Errorf("route %s failed: %w", route.Exchange, err)
			}
		})
	}
	
	// Wait for all routes in group to complete
	wg.Wait()
	close(errChan)
	
	// Collect errors
	var errs []error
	for err := range errChan {
		if err != nil {
			errs = append(errs, err)
		}
	}
	
	// If all routes failed, return error
	if len(errs) == len(routes) {
		return fmt.Errorf("all routes failed")
	}
	
	return nil
}

// executeSingleRoute executes a single route
func (e *ExecutionEngine) executeSingleRoute(ctx context.Context, execution *ExecutionContext, route Route) error {
	startTime := time.Now()
	
	// Create executed route record
	execRoute := &ExecutedRoute{
		Route:     route,
		StartTime: startTime,
		Status:    "pending",
	}
	
	// Store in execution context
	execution.mu.Lock()
	execution.executedRoutes[route.Exchange] = execRoute
	execution.mu.Unlock()
	
	// Get exchange
	exchange, err := e.exchangeManager.GetExchange(route.Exchange)
	if err != nil {
		execRoute.Error = err
		execRoute.Status = "failed"
		return err
	}
	
	// Create order
	order := &types.Order{
		Symbol:      route.Symbol,
		Side:        execution.RoutingDecision.OriginalOrder.Side,
		Type:        execution.RoutingDecision.OriginalOrder.Type,
		Quantity:    route.Quantity,
		Price:       route.ExpectedPrice,
		TimeInForce: execution.RoutingDecision.OriginalOrder.TimeInForce,
	}
	
	// Apply fee optimization
	if e.config.EnableFeeOptimization {
		order = e.optimizeOrderForFees(order, route.Exchange)
	}
	
	// Execute with retries
	var lastErr error
	for attempt := 0; attempt <= e.config.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(e.config.RetryDelay)
		}
		
		// Create order with timeout
		orderCtx, cancel := context.WithTimeout(ctx, e.config.OrderTimeout)
		result, err := exchange.PlaceOrder(orderCtx, order)
		cancel()
		
		if err == nil {
			// Success
			execRoute.OrderID = result.ExchangeOrderID
			execRoute.Status = "filled"
			execRoute.ExecutedQty = result.Quantity
			execRoute.AveragePrice = result.Price
			execRoute.EndTime = time.Now()
			
			// Update metrics
			e.updateMetrics(execRoute, true)
			
			return nil
		}
		
		lastErr = err
		
		// Check if error is retryable
		if !e.isRetryableError(err) {
			break
		}
	}
	
	// Failed after retries
	execRoute.Error = lastErr
	execRoute.Status = "failed"
	execRoute.EndTime = time.Now()
	
	// Update metrics
	e.updateMetrics(execRoute, false)
	
	return lastErr
}

// monitorExecution monitors execution progress
func (e *ExecutionEngine) monitorExecution(execution *ExecutionContext) {
	ticker := time.NewTicker(e.config.MonitoringInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// Check execution status
			if e.checkExecutionComplete(execution) {
				execution.report = e.generateReport(execution)
				close(execution.done)
				return
			}
			
		case <-execution.done:
			return
		}
	}
}

// checkExecutionComplete checks if execution is complete
func (e *ExecutionEngine) checkExecutionComplete(execution *ExecutionContext) bool {
	execution.mu.RLock()
	defer execution.mu.RUnlock()
	
	// Check if all routes have been executed
	expectedRoutes := len(execution.RoutingDecision.Routes)
	executedRoutes := len(execution.executedRoutes)
	
	if executedRoutes < expectedRoutes {
		return false
	}
	
	// Check if all executions are in final state
	for _, route := range execution.executedRoutes {
		if route.Status == "pending" {
			return false
		}
	}
	
	// Update execution status
	totalExecuted := decimal.Zero
	
	for _, route := range execution.executedRoutes {
		if route.Status == "filled" {
			totalExecuted = totalExecuted.Add(route.ExecutedQty)
		}
	}
	
	originalQty := execution.RoutingDecision.OriginalOrder.Quantity
	
	if totalExecuted.Equal(originalQty) {
		execution.Status = ExecutionStatusCompleted
	} else if totalExecuted.GreaterThan(decimal.Zero) {
		execution.Status = ExecutionStatusPartial
	} else {
		execution.Status = ExecutionStatusFailed
	}
	
	return true
}

// generateReport generates execution report
func (e *ExecutionEngine) generateReport(execution *ExecutionContext) *ExecutionReport {
	execution.mu.RLock()
	defer execution.mu.RUnlock()
	
	report := &ExecutionReport{
		RoutingID:    execution.RoutingDecision.ID,
		Status:       execution.Status,
		ExecutionTime: time.Since(execution.StartTime),
		CompletedAt:  time.Now(),
		Fills:        make([]Fill, 0),
	}
	
	totalExecuted := decimal.Zero
	totalCost := decimal.Zero
	totalFees := decimal.Zero
	
	for _, route := range execution.executedRoutes {
		if route.Status == "filled" {
			fill := Fill{
				Exchange:  route.Route.Exchange,
				OrderID:   route.OrderID,
				Quantity:  route.ExecutedQty,
				Price:     route.AveragePrice,
				Fee:       route.Fee,
				Timestamp: route.EndTime,
			}
			
			report.Fills = append(report.Fills, fill)
			
			totalExecuted = totalExecuted.Add(route.ExecutedQty)
			totalCost = totalCost.Add(route.ExecutedQty.Mul(route.AveragePrice))
			totalFees = totalFees.Add(route.Fee)
		} else if route.Error != nil {
			report.Errors = append(report.Errors, route.Error)
		}
	}
	
	report.ExecutedQuantity = totalExecuted
	report.TotalFees = totalFees
	
	if totalExecuted.GreaterThan(decimal.Zero) {
		report.AveragePrice = totalCost.Div(totalExecuted)
		
		// Calculate slippage
		originalOrder := execution.RoutingDecision.OriginalOrder
		if originalOrder.Type == types.OrderTypeLimit {
			if originalOrder.Side == types.OrderSideBuy {
				report.Slippage = report.AveragePrice.Sub(originalOrder.Price).Div(originalOrder.Price)
			} else {
				report.Slippage = originalOrder.Price.Sub(report.AveragePrice).Div(originalOrder.Price)
			}
		}
	}
	
	return report
}

// Helper methods

func (e *ExecutionEngine) groupRoutesByPriority(routes []Route) [][]Route {
	// Group by priority
	groups := make(map[int][]Route)
	for _, route := range routes {
		groups[route.Priority] = append(groups[route.Priority], route)
	}
	
	// Sort priorities
	var priorities []int
	for p := range groups {
		priorities = append(priorities, p)
	}
	
	// Return groups in priority order
	result := make([][]Route, 0, len(priorities))
	for i := 1; i <= len(priorities); i++ {
		if group, exists := groups[i]; exists {
			result = append(result, group)
		}
	}
	
	return result
}

func (e *ExecutionEngine) isOrderFilled(execution *ExecutionContext) bool {
	execution.mu.RLock()
	defer execution.mu.RUnlock()
	
	totalExecuted := decimal.Zero
	for _, route := range execution.executedRoutes {
		if route.Status == "filled" {
			totalExecuted = totalExecuted.Add(route.ExecutedQty)
		}
	}
	
	return totalExecuted.GreaterThanOrEqual(execution.RoutingDecision.OriginalOrder.Quantity)
}

func (e *ExecutionEngine) optimizeOrderForFees(order *types.Order, exchange string) *types.Order {
	// In a real implementation, this would:
	// 1. Check if exchange offers maker rebates
	// 2. Adjust order type (e.g., post-only) to qualify for rebates
	// 3. Adjust price to ensure maker order
	
	// For now, return order as-is
	return order
}

func (e *ExecutionEngine) isRetryableError(err error) bool {
	// Check if error is retryable
	// Network errors, rate limits, etc. are retryable
	// Invalid parameters, insufficient balance are not
	
	errStr := err.Error()
	
	// Non-retryable errors
	nonRetryable := []string{
		"insufficient balance",
		"invalid symbol",
		"invalid quantity",
		"market closed",
	}
	
	for _, nr := range nonRetryable {
		if contains(errStr, nr) {
			return false
		}
	}
	
	// Default to retryable
	return true
}

func (e *ExecutionEngine) updateMetrics(route *ExecutedRoute, success bool) {
	e.metrics.mu.Lock()
	defer e.metrics.mu.Unlock()
	
	e.metrics.TotalExecutions++
	
	if success {
		e.metrics.SuccessfulOrders++
		e.metrics.TotalVolume = e.metrics.TotalVolume.Add(route.ExecutedQty.Mul(route.AveragePrice))
		e.metrics.TotalFees = e.metrics.TotalFees.Add(route.Fee)
		
		// Update average latency
		latency := route.EndTime.Sub(route.StartTime)
		if e.metrics.AverageLatency == 0 {
			e.metrics.AverageLatency = latency
		} else {
			e.metrics.AverageLatency = (e.metrics.AverageLatency + latency) / 2
		}
	} else {
		e.metrics.FailedOrders++
	}
	
	e.metrics.LastUpdateTime = time.Now()
}

// GetMetrics returns execution metrics
func (e *ExecutionEngine) GetMetrics() ExecutionMetrics {
	e.metrics.mu.RLock()
	defer e.metrics.mu.RUnlock()
	return *e.metrics
}

// Shutdown shuts down the execution engine
func (e *ExecutionEngine) Shutdown() {
	e.workerPool.Stop()
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}