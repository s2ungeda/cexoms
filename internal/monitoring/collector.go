package monitoring

import (
	"context"
	"sync"
	"time"

	"github.com/mExOms/internal/account"
	"github.com/mExOms/internal/position"
	"github.com/mExOms/pkg/types"
	"log"
)

// Collector collects metrics from various system components
type Collector struct {
	mu sync.RWMutex
	
	metrics         *Metrics
	accountManager  types.AccountManager
	positionManager *position.MultiAccountPositionManager
	
	// Collection intervals
	accountInterval  time.Duration
	positionInterval time.Duration
	
	// Control
	stopCh chan struct{}
	wg     sync.WaitGroup
	
}

// CollectorConfig contains collector configuration
type CollectorConfig struct {
	AccountCollectionInterval  time.Duration
	PositionCollectionInterval time.Duration
}

// NewCollector creates a new metrics collector
func NewCollector(
	metrics *Metrics,
	accountManager types.AccountManager,
	positionManager *position.MultiAccountPositionManager,
	config *CollectorConfig,
) *Collector {
	if config == nil {
		config = &CollectorConfig{
			AccountCollectionInterval:  30 * time.Second,
			PositionCollectionInterval: 10 * time.Second,
		}
	}
	
	return &Collector{
		metrics:          metrics,
		accountManager:   accountManager,
		positionManager:  positionManager,
		accountInterval:  config.AccountCollectionInterval,
		positionInterval: config.PositionCollectionInterval,
		stopCh:          make(chan struct{}),
	}
}

// Start starts the metrics collector
func (c *Collector) Start(ctx context.Context) error {
	log.Println("Starting metrics collector")
	
	// Start collection workers
	c.wg.Add(2)
	go c.collectAccountMetrics(ctx)
	go c.collectPositionMetrics(ctx)
	
	return nil
}

// Stop stops the metrics collector
func (c *Collector) Stop() {
	log.Println("Stopping metrics collector")
	close(c.stopCh)
	c.wg.Wait()
}

// collectAccountMetrics collects account-related metrics
func (c *Collector) collectAccountMetrics(ctx context.Context) {
	defer c.wg.Done()
	
	ticker := time.NewTicker(c.accountInterval)
	defer ticker.Stop()
	
	// Collect immediately on start
	c.doCollectAccountMetrics()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.doCollectAccountMetrics()
		}
	}
}

// doCollectAccountMetrics performs actual account metrics collection
func (c *Collector) doCollectAccountMetrics() {
	// Get all accounts
	accounts, err := c.accountManager.ListAccounts(types.AccountFilter{})
	if err != nil {
		log.Printf("Failed to list accounts: %v", err)
		return
	}
	
	for _, account := range accounts {
		// Get balance
		balance, err := c.accountManager.GetBalance(account.ID)
		if err != nil {
			log.Printf("Failed to get balance for %s: %v", account.ID, err)
			continue
		}
		
		// Record balance metrics
		c.metrics.RecordAccountBalance(
			account.ID,
			account.Exchange,
			"USDT",
			balance.TotalUSDT.InexactFloat64(),
		)
		
		// Record individual asset balances
		for asset, assetBalance := range balance.Balances {
			c.metrics.RecordAccountBalance(
				account.ID,
				account.Exchange,
				asset,
				assetBalance.Total.InexactFloat64(),
			)
		}
		
		// Calculate and record equity
		positions, _ := c.accountManager.GetPositions(account.ID)
		totalPnL := 0.0
		if positions != nil {
			for _, pos := range positions.Positions {
				totalPnL += pos.UnrealizedPnL.InexactFloat64()
			}
		}
		
		equity := balance.TotalUSDT.InexactFloat64() + totalPnL
		c.metrics.AccountEquity.WithLabelValues(account.ID, account.Exchange).Set(equity)
		
		// Get and record metrics
		metrics, err := c.accountManager.GetMetrics(account.ID)
		if err == nil {
			// Record performance metrics
			c.metrics.StrategyPerformance.WithLabelValues(account.Strategy, "total_pnl").
				Set(metrics.TotalPnL.InexactFloat64())
			c.metrics.StrategyPerformance.WithLabelValues(account.Strategy, "win_rate").
				Set(metrics.WinRate)
			c.metrics.StrategyPerformance.WithLabelValues(account.Strategy, "sharpe_ratio").
				Set(metrics.SharpeRatio)
		}
	}
}

// collectPositionMetrics collects position-related metrics
func (c *Collector) collectPositionMetrics(ctx context.Context) {
	defer c.wg.Done()
	
	ticker := time.NewTicker(c.positionInterval)
	defer ticker.Stop()
	
	// Collect immediately on start
	c.doCollectPositionMetrics()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.doCollectPositionMetrics()
		}
	}
}

// doCollectPositionMetrics performs actual position metrics collection
func (c *Collector) doCollectPositionMetrics() {
	// Get portfolio summary
	portfolio := c.positionManager.GetPortfolioSummary()
	
	// Clear gauges first
	c.metrics.PositionsOpen.Reset()
	c.metrics.PositionPnL.Reset()
	c.metrics.PositionMargin.Reset()
	c.metrics.PositionLeverage.Reset()
	
	// Collect metrics for each account
	for accountID, summary := range portfolio.AccountSummaries {
		positionCount := 0
		
		for symbol, pos := range summary.PositionsBySymbol {
			positionCount++
			
			// Record position count
			c.metrics.PositionsOpen.WithLabelValues(
				accountID,
				"", // exchange not available in summary
				symbol,
				string(pos.Side),
			).Set(1)
			
			// Record position metrics
			c.metrics.RecordPosition(
				accountID,
				"", // exchange
				symbol,
				string(pos.Side),
				pos.UnrealizedPnL.InexactFloat64(),
				pos.Margin.InexactFloat64(),
				float64(pos.Leverage),
			)
		}
		
		// Record margin level
		if !summary.TotalMargin.IsZero() {
			marginLevel := summary.MarginLevel.InexactFloat64()
			c.metrics.AccountMarginLevel.WithLabelValues(accountID, "").Set(marginLevel)
		}
	}
}

// RecordOrderEvent records an order event
func (c *Collector) RecordOrderEvent(order *types.Order, duration time.Duration) {
	c.metrics.RecordOrder(
		order.AccountID,
		order.Exchange,
		order.Symbol,
		string(order.Side),
		string(order.Type),
		string(order.Status),
		order.Quantity.Mul(order.Price).InexactFloat64(),
		duration,
	)
}

// RecordTransferEvent records a transfer event
func (c *Collector) RecordTransferEvent(transfer *types.AccountTransfer) {
	c.metrics.TransferCount.WithLabelValues(
		transfer.FromAccount,
		transfer.ToAccount,
		transfer.Asset,
		string(transfer.Status),
	).Inc()
	
	c.metrics.TransferVolume.WithLabelValues(
		transfer.FromAccount,
		transfer.ToAccount,
		transfer.Asset,
	).Add(transfer.Amount.InexactFloat64())
}

// RecordRiskCheckEvent records a risk check event
func (c *Collector) RecordRiskCheckEvent(accountID, checkType, result string, duration time.Duration) {
	c.metrics.RecordRiskCheck(accountID, checkType, result, duration)
}

// RecordStrategySignal records a strategy signal
func (c *Collector) RecordStrategySignal(strategy, signalType, symbol string) {
	c.metrics.StrategySignals.WithLabelValues(strategy, signalType, symbol).Inc()
}

// RecordExchangeLatency records exchange API latency
func (c *Collector) RecordExchangeLatency(exchange, operation string, duration time.Duration) {
	c.metrics.ExchangeLatency.WithLabelValues(exchange, operation).Observe(duration.Seconds())
}

// RecordExchangeError records exchange errors
func (c *Collector) RecordExchangeError(exchange, errorType string) {
	c.metrics.ExchangeErrors.WithLabelValues(exchange, errorType).Inc()
}

// RecordWebSocketConnection records WebSocket connection status
func (c *Collector) RecordWebSocketConnection(exchange, streamType string, delta float64) {
	c.metrics.WebSocketConnections.WithLabelValues(exchange, streamType).Add(delta)
}

// RecordRateLimitUsage records rate limit usage
func (c *Collector) RecordRateLimitUsage(exchange, endpoint string, usage float64) {
	c.metrics.RateLimitUsage.WithLabelValues(exchange, endpoint).Set(usage)
}