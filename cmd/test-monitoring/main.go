package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/mExOms/pkg/monitoring"
)

func main() {
	fmt.Println("=== mExOms Monitoring System Test ===")

	// Create logger
	config := zap.NewProductionConfig()
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	
	zapLogger, err := config.Build()
	if err != nil {
		log.Fatal("Failed to create logger:", err)
	}
	defer zapLogger.Sync()

	// Create monitoring components
	logConfig := &monitoring.LogConfig{
		BaseDir:             "./logs",
		Level:               "debug",
		JSONFormat:          true,
		MaxSize:             100,
		MaxBackups:          5,
		MaxAge:              7,
		Compress:            true,
		BufferSize:          256,
		SeparateAccountLogs: true,
	}

	logger, err := monitoring.NewLogger(logConfig)
	if err != nil {
		log.Fatal("Failed to create monitoring logger:", err)
	}

	// Create metrics collector
	metricsCollector := monitoring.NewMetricsCollector(nil, zapLogger)

	// Create strategy P&L tracker
	pnlTracker := monitoring.NewStrategyPnLTracker(nil, zapLogger)

	// Create arbitrage analyzer
	arbitrageAnalyzer := monitoring.NewArbitrageAnalyzer(nil, zapLogger)

	// Create health checker
	healthChecker := monitoring.NewHealthChecker(nil, metricsCollector, zapLogger)
	if err := healthChecker.Start(); err != nil {
		log.Fatal("Failed to start health checker:", err)
	}

	// Create dashboard
	dashboard := monitoring.NewDashboard(
		nil,
		logger,
		metricsCollector,
		pnlTracker,
		arbitrageAnalyzer,
		zapLogger,
	)
	if err := dashboard.Start(); err != nil {
		log.Fatal("Failed to start dashboard:", err)
	}

	fmt.Println("\nMonitoring services started:")
	fmt.Println("- Dashboard: http://localhost:8080")
	fmt.Println("- Health Check: http://localhost:8081/health")
	fmt.Println("- Metrics: http://localhost:8081/metrics")

	// Register test accounts and strategies
	accounts := []string{"main-001", "arb-001", "arb-002", "mm-001", "mm-002"}
	exchanges := []string{"binance", "bybit", "okx"}
	symbols := []string{"BTCUSDT", "ETHUSDT", "BNBUSDT", "SOLUSDT"}

	// Register with health checker
	for _, exchange := range exchanges {
		healthChecker.RegisterExchange(exchange)
	}
	
	for i, account := range accounts {
		exchange := exchanges[i%len(exchanges)]
		healthChecker.RegisterAccount(account, exchange)
	}

	// Register strategies
	strategies := []struct {
		id          string
		strategyType string
		accounts     []string
	}{
		{"arb-spot-001", "arbitrage", []string{"arb-001", "arb-002"}},
		{"mm-btc-001", "market-making", []string{"mm-001"}},
		{"mm-eth-001", "market-making", []string{"mm-002"}},
		{"grid-001", "grid", []string{"main-001"}},
	}

	for _, s := range strategies {
		pnlTracker.RegisterStrategy(s.id, s.strategyType, s.accounts)
	}

	// Start simulation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start simulators
	go simulateAccountActivity(ctx, accounts, symbols, logger, metricsCollector)
	go simulateStrategyTrading(ctx, strategies, symbols, pnlTracker, metricsCollector)
	go simulateArbitrage(ctx, exchanges, symbols, arbitrageAnalyzer)
	go simulateExchangeHealth(ctx, exchanges, healthChecker)
	go updateSystemMetrics(ctx, metricsCollector)

	// Handle shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	
	<-sigChan
	fmt.Println("\nShutting down monitoring system...")
	
	cancel()
	
	// Stop components
	metricsCollector.Stop()
	pnlTracker.Stop()
	arbitrageAnalyzer.Stop()
	dashboard.Stop()
	healthChecker.Stop()
	logger.Sync()
	
	fmt.Println("Monitoring system stopped")
}

// simulateAccountActivity simulates account trading activity
func simulateAccountActivity(ctx context.Context, accounts, symbols []string, logger *monitoring.Logger, collector *monitoring.MetricsCollector) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Random account and symbol
			account := accounts[rand.Intn(len(accounts))]
			symbol := symbols[rand.Intn(len(symbols))]

			// Simulate order
			orderID := fmt.Sprintf("order-%d", time.Now().UnixNano())
			side := "BUY"
			if rand.Float64() > 0.5 {
				side = "SELL"
			}

			// Random latency
			latency := time.Duration(rand.Intn(50)+10) * time.Millisecond

			// Log order
			logger.LogOrder(account, orderID, "NEW",
				zap.String("symbol", symbol),
				zap.String("side", side),
				zap.Float64("quantity", rand.Float64()*10),
				zap.Float64("price", 40000+rand.Float64()*1000),
			)

			// Record metrics
			orderMetric := &monitoring.OrderMetric{
				AccountID: account,
				OrderID:   orderID,
				Symbol:    symbol,
				Side:      side,
				Type:      "LIMIT",
				Status:    "NEW",
				Latency:   latency,
				Timestamp: time.Now(),
			}
			collector.RecordOrder(orderMetric)

			// Simulate order fill
			if rand.Float64() > 0.3 {
				time.Sleep(time.Duration(rand.Intn(500)) * time.Millisecond)
				
				orderMetric.Status = "FILLED"
				collector.RecordOrder(orderMetric)

				// Record trade
				tradeMetric := &monitoring.TradeMetric{
					AccountID: account,
					TradeID:   fmt.Sprintf("trade-%d", time.Now().UnixNano()),
					Symbol:    symbol,
					Side:      side,
					Quantity:  rand.Float64() * 10,
					Price:     40000 + rand.Float64()*1000,
					Fee:       rand.Float64() * 10,
					Timestamp: time.Now(),
				}
				collector.RecordTrade(tradeMetric)
			}

			// Simulate position update
			if rand.Float64() > 0.5 {
				positionMetric := &monitoring.PositionMetric{
					AccountID: account,
					Symbol:    symbol,
					Side:      side,
					Quantity:  rand.Float64() * 100,
					Exposure:  rand.Float64() * 100000,
					PnL:       (rand.Float64() - 0.5) * 1000,
					Timestamp: time.Now(),
				}
				collector.RecordPosition(positionMetric)
			}
		}
	}
}

// simulateStrategyTrading simulates strategy trading activity
func simulateStrategyTrading(ctx context.Context, strategies []struct{id, strategyType string; accounts []string}, symbols []string, tracker *monitoring.StrategyPnLTracker, collector *monitoring.MetricsCollector) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Random strategy
			strategy := strategies[rand.Intn(len(strategies))]
			symbol := symbols[rand.Intn(len(symbols))]
			account := strategy.accounts[rand.Intn(len(strategy.accounts))]

			// Simulate trade
			pnl := (rand.Float64() - 0.45) * 100 // Slight positive bias
			trade := &monitoring.StrategyTrade{
				StrategyID: strategy.id,
				AccountID:  account,
				TradeID:    fmt.Sprintf("trade-%d", time.Now().UnixNano()),
				Symbol:     symbol,
				Side:       "BUY",
				Quantity:   rand.Float64() * 10,
				Price:      40000 + rand.Float64()*1000,
				Fee:        rand.Float64() * 10,
				PnL:        pnl,
				Timestamp:  time.Now(),
			}
			tracker.RecordTrade(trade)

			// Update position
			position := &monitoring.StrategyPosition{
				StrategyID:    strategy.id,
				AccountID:     account,
				Symbol:        symbol,
				Side:          "LONG",
				Quantity:      rand.Float64() * 100,
				EntryPrice:    40000 + rand.Float64()*1000,
				CurrentPrice:  40000 + rand.Float64()*1000,
				UnrealizedPnL: (rand.Float64() - 0.5) * 1000,
				Exposure:      rand.Float64() * 100000,
				Timestamp:     time.Now(),
			}
			tracker.UpdatePosition(position)

			// Set custom metrics
			tracker.SetCustomMetric(strategy.id, "signal_strength", rand.Float64())
			tracker.SetCustomMetric(strategy.id, "market_volatility", rand.Float64()*0.1)
		}
	}
}

// simulateArbitrage simulates arbitrage opportunities
func simulateArbitrage(ctx context.Context, exchanges, symbols []string, analyzer *monitoring.ArbitrageAnalyzer) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	opportunityCounter := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Generate arbitrage opportunity
			if rand.Float64() > 0.3 {
				opportunityCounter++
				symbol := symbols[rand.Intn(len(symbols))]
				buyExchange := exchanges[rand.Intn(len(exchanges))]
				sellExchange := exchanges[rand.Intn(len(exchanges))]
				
				if buyExchange == sellExchange {
					continue
				}

				basePrice := 40000.0
				spread := rand.Float64() * 0.005 // 0-0.5% spread
				buyPrice := basePrice * (1 - spread/2)
				sellPrice := basePrice * (1 + spread/2)

				signal := &monitoring.ArbitrageSignal{
					OpportunityID:   fmt.Sprintf("arb-%d", opportunityCounter),
					Type:            "spot-spot",
					Symbol:          symbol,
					BuyExchange:     buyExchange,
					SellExchange:    sellExchange,
					BuyPrice:        buyPrice,
					SellPrice:       sellPrice,
					MaxVolume:       rand.Float64() * 10,
					EstimatedProfit: (sellPrice - buyPrice) * rand.Float64() * 10,
					Timestamp:       time.Now(),
				}
				analyzer.RecordOpportunity(signal)

				// Simulate execution
				if rand.Float64() > 0.4 {
					time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
					
					// Calculate actual prices with slippage
					slippage := (rand.Float64() - 0.5) * 0.001
					actualBuyPrice := buyPrice * (1 + slippage)
					actualSellPrice := sellPrice * (1 - slippage)
					
					volume := rand.Float64() * signal.MaxVolume
					fees := volume * (actualBuyPrice + actualSellPrice) * 0.001
					netProfit := (actualSellPrice - actualBuyPrice) * volume - fees

					execution := &monitoring.ArbitrageExecution{
						ExecutionID:   fmt.Sprintf("exec-%d", time.Now().UnixNano()),
						OpportunityID: signal.OpportunityID,
						Symbol:        symbol,
						BuyOrderID:    fmt.Sprintf("buy-%d", time.Now().UnixNano()),
						SellOrderID:   fmt.Sprintf("sell-%d", time.Now().UnixNano()),
						Volume:        volume,
						BuyPrice:      actualBuyPrice,
						SellPrice:     actualSellPrice,
						BuyFee:        fees / 2,
						SellFee:       fees / 2,
						NetProfit:     netProfit,
						ExecutionTime: time.Duration(rand.Intn(500)) * time.Millisecond,
						Timestamp:     time.Now(),
						Success:       netProfit > 0,
					}
					analyzer.RecordExecution(execution)
				}
			}
		}
	}
}

// simulateExchangeHealth simulates exchange connectivity
func simulateExchangeHealth(ctx context.Context, exchanges []string, healthChecker *monitoring.HealthChecker) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, exchange := range exchanges {
				// Simulate connection status
				connected := rand.Float64() > 0.1 // 90% uptime
				latency := time.Duration(rand.Intn(50)+10) * time.Millisecond
				
				var err error
				if !connected {
					err = fmt.Errorf("connection timeout")
				}

				healthChecker.UpdateExchangeHealth(exchange, connected, latency, err)
			}
		}
	}
}

// updateSystemMetrics updates system performance metrics
func updateSystemMetrics(ctx context.Context, collector *monitoring.MetricsCollector) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Get system stats
			var m runtime.MemStats
			runtime.ReadMemStats(&m)

			globalMetrics := collector.GetGlobalMetrics()
			
			// Update CPU (simulated)
			cpuUsage := uint32(rand.Float64() * 60 * 100) // 0-60%
			globalMetrics.CPUUsage.Store(cpuUsage)
			
			// Update memory
			globalMetrics.MemoryUsage.Store(m.Alloc)
			
			// Update goroutines
			globalMetrics.GoroutineCount.Store(int32(runtime.NumGoroutine()))
			
			// Update message queue (simulated)
			globalMetrics.MessageQueueDepth.Store(int32(rand.Intn(100)))
			globalMetrics.MessagesProcessed.Add(int64(rand.Intn(100)))
		}
	}
}