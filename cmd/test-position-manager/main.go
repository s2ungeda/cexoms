package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mExOms/internal/position"
	"github.com/mExOms/pkg/types"
)

// Test symbols and accounts
var (
	testSymbols = []string{
		"BTCUSDT", "ETHUSDT", "BNBUSDT", "SOLUSDT", "XRPUSDT",
		"ADAUSDT", "DOGEUSDT", "MATICUSDT", "DOTUSDT", "LINKUSDT",
	}
	
	testAccounts = []struct {
		ID       string
		Exchange string
		Type     string
	}{
		{"main", "binance", "spot"},
		{"sub_arbitrage", "binance", "futures"},
		{"sub_market_making", "binance", "spot"},
		{"sub_trend", "binance", "futures"},
		{"sub_scalping", "binance", "spot"},
	}
	
	testStrategies = []string{
		"arbitrage", "market_making", "trend_following", "scalping",
	}
)

func main() {
	fmt.Println("=== Multi-Account Integrated Position Manager Test ===")
	
	// Create position manager
	positionMgr := position.NewIntegratedPositionManager()
	defer positionMgr.Stop()
	
	// Create correlation analyzer
	corrAnalyzer := position.NewCorrelationAnalyzer(positionMgr)
	defer corrAnalyzer.Stop()
	
	// Create P&L calculator
	pnlCalc := position.NewPnLCalculator(positionMgr)
	defer pnlCalc.Stop()
	
	// Setup callbacks
	setupCallbacks(positionMgr, corrAnalyzer, pnlCalc)
	
	// Add test accounts
	fmt.Println("\n1. Adding test accounts...")
	for _, account := range testAccounts {
		err := positionMgr.AddAccount(account.ID, account.Exchange)
		if err != nil {
			log.Printf("Failed to add account %s: %v", account.ID, err)
		}
	}
	
	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start position simulator
	go simulatePositions(ctx, positionMgr, corrAnalyzer, pnlCalc)
	
	// Start monitoring
	go monitorPositions(ctx, positionMgr, pnlCalc)
	
	// Start correlation analysis
	go analyzeCorrelations(ctx, corrAnalyzer)
	
	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	<-sigChan
	fmt.Println("\nShutting down...")
}

// setupCallbacks sets up event callbacks
func setupCallbacks(positionMgr *position.IntegratedPositionManager, 
	corrAnalyzer *position.CorrelationAnalyzer,
	pnlCalc *position.PnLCalculator) {
	
	// Position update callback
	positionMgr.SetPositionUpdateCallback(func(update *position.PositionUpdate) {
		fmt.Printf("[POSITION UPDATE] Account: %s, Symbol: %s, Type: %s, Qty: %.4f\n",
			update.AccountID, update.Symbol, update.UpdateType, update.Position.Quantity)
	})
	
	// Risk limit callback
	positionMgr.SetRiskLimitCallback(func(accountID string, reason string) {
		fmt.Printf("[RISK ALERT] Account: %s, Reason: %s\n", accountID, reason)
	})
	
	// Hedge opportunity callback
	positionMgr.SetHedgeOpportunityCallback(func(symbol string, accounts []string) {
		fmt.Printf("[HEDGE OPPORTUNITY] Symbol: %s, Accounts: %v\n", symbol, accounts)
	})
	
	// High correlation callback
	corrAnalyzer.SetHighCorrelationCallback(func(symbol1, symbol2 string, correlation float64) {
		fmt.Printf("[HIGH CORRELATION] %s <-> %s: %.3f\n", symbol1, symbol2, correlation)
	})
	
	// Risk concentration callback
	corrAnalyzer.SetRiskConcentrationCallback(func(accounts []string, reason string) {
		fmt.Printf("[RISK CONCENTRATION] Accounts: %v, Reason: %s\n", accounts, reason)
	})
	
	// P&L update callback
	pnlCalc.SetPnLUpdateCallback(func(snapshot *position.PnLSnapshot) {
		if int(time.Now().Unix()) % 10 == 0 { // Log every 10 seconds
			fmt.Printf("[P&L UPDATE] Total: $%.2f (Unrealized: $%.2f, Realized: $%.2f)\n",
				snapshot.TotalPL, snapshot.UnrealizedPL, snapshot.RealizedPL)
		}
	})
	
	// Drawdown callback
	pnlCalc.SetDrawdownCallback(func(accountID string, drawdown float64) {
		fmt.Printf("[DRAWDOWN ALERT] Account: %s, Drawdown: %.2f%%\n", 
			accountID, drawdown*100)
	})
}

// simulatePositions simulates position changes
func simulatePositions(ctx context.Context, 
	positionMgr *position.IntegratedPositionManager,
	corrAnalyzer *position.CorrelationAnalyzer,
	pnlCalc *position.PnLCalculator) {
	
	rand.Seed(time.Now().UnixNano())
	
	// Initial prices
	prices := make(map[string]float64)
	prices["BTCUSDT"] = 50000.0
	prices["ETHUSDT"] = 3000.0
	prices["BNBUSDT"] = 400.0
	prices["SOLUSDT"] = 100.0
	prices["XRPUSDT"] = 0.5
	prices["ADAUSDT"] = 0.6
	prices["DOGEUSDT"] = 0.08
	prices["MATICUSDT"] = 0.9
	prices["DOTUSDT"] = 7.0
	prices["LINKUSDT"] = 15.0
	
	// Simulate opening positions
	fmt.Println("\n2. Opening initial positions...")
	for i, account := range testAccounts {
		// Each account trades different symbols
		startIdx := i * 2
		endIdx := startIdx + 3
		if endIdx > len(testSymbols) {
			endIdx = len(testSymbols)
		}
		
		for _, symbol := range testSymbols[startIdx:endIdx] {
			// Random position
			side := 1.0
			if rand.Float64() > 0.5 {
				side = -1.0 // Short
			}
			
			quantity := (rand.Float64() * 5.0 + 1.0) * side
			price := prices[symbol]
			
			pos := &types.Position{
				Symbol:       symbol,
				Quantity:     quantity,
				AvgPrice:     price,
				MarkPrice:    price,
				Value:        quantity * price,
				UnrealizedPL: 0,
				MarginUsed:   quantity * price / 10, // 10x leverage
			}
			
			err := positionMgr.UpdatePosition(account.ID, account.Exchange, symbol, pos)
			if err != nil {
				log.Printf("Failed to update position: %v", err)
			}
			
			// Assign to strategy
			strategyIdx := i % len(testStrategies)
			positionMgr.AssignPositionToStrategy(account.ID, symbol, testStrategies[strategyIdx])
			
			// Update price data for correlation
			corrAnalyzer.AddPriceUpdate(symbol, price, time.Now())
		}
	}
	
	// Simulate price changes and position updates
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// Update prices with small random changes
			for symbol, currentPrice := range prices {
				// Random walk
				change := (rand.Float64() - 0.5) * 0.02 // ±1% change
				newPrice := currentPrice * (1 + change)
				prices[symbol] = newPrice
				
				// Update mark price
				pnlCalc.UpdateMarkPrice(symbol, newPrice)
				
				// Update correlation data
				corrAnalyzer.AddPriceUpdate(symbol, newPrice, time.Now())
			}
			
			// Randomly close or modify positions
			if rand.Float64() > 0.8 {
				accountIdx := rand.Intn(len(testAccounts))
				account := testAccounts[accountIdx]
				symbolIdx := rand.Intn(len(testSymbols))
				symbol := testSymbols[symbolIdx]
				
				// Get current position if exists
				if accPos, err := positionMgr.GetAccountPositions(account.ID); err == nil {
					if pos, exists := accPos.Positions[symbol]; exists {
						// Close position
						trade := position.Trade{
							AccountID:  account.ID,
							Symbol:     symbol,
							Quantity:   pos.Quantity,
							EntryPrice: pos.AvgPrice,
							ExitPrice:  prices[symbol],
							RealizedPL: (prices[symbol] - pos.AvgPrice) * pos.Quantity,
							Fee:        pos.Value * 0.001, // 0.1% fee
							Timestamp:  time.Now(),
						}
						
						// Record trade
						pnlCalc.RecordTrade(trade)
						
						// Close position
						closedPos := &types.Position{
							Symbol:   symbol,
							Quantity: 0,
						}
						positionMgr.UpdatePosition(account.ID, account.Exchange, symbol, closedPos)
						
						fmt.Printf("Closed position: %s %s %.4f @ %.2f, P&L: $%.2f\n",
							account.ID, symbol, pos.Quantity, prices[symbol], trade.RealizedPL)
					} else {
						// Open new position
						side := 1.0
						if rand.Float64() > 0.5 {
							side = -1.0
						}
						
						quantity := (rand.Float64() * 3.0 + 0.5) * side
						price := prices[symbol]
						
						newPos := &types.Position{
							Symbol:       symbol,
							Quantity:     quantity,
							AvgPrice:     price,
							MarkPrice:    price,
							Value:        quantity * price,
							UnrealizedPL: 0,
							MarginUsed:   quantity * price / 10,
						}
						
						positionMgr.UpdatePosition(account.ID, account.Exchange, symbol, newPos)
						
						fmt.Printf("Opened position: %s %s %.4f @ %.2f\n",
							account.ID, symbol, quantity, price)
					}
				}
			}
			
		case <-ctx.Done():
			return
		}
	}
}

// monitorPositions monitors and displays position status
func monitorPositions(ctx context.Context, 
	positionMgr *position.IntegratedPositionManager,
	pnlCalc *position.PnLCalculator) {
	
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			fmt.Println("\n=== Position Summary ===")
			
			// Display account P&L
			pnlSummary := positionMgr.GetAccountsPnL()
			fmt.Println("\nAccount P&L:")
			for accountID, pnl := range pnlSummary {
				fmt.Printf("  %s: Total P&L: $%.2f (Unrealized: $%.2f, Realized: $%.2f)\n",
					accountID, pnl["total_pl"], pnl["unrealized_pl"], pnl["realized_pl"])
			}
			
			// Display hedged positions
			hedged := positionMgr.GetHedgedPositions()
			if len(hedged) > 0 {
				fmt.Println("\nHedged Positions:")
				for symbol, hedge := range hedged {
					fmt.Printf("  %s: Long %.4f, Short %.4f, Net %.4f, Ratio: %.2f\n",
						symbol, hedge.LongQty, hedge.ShortQty, hedge.NetExposure, hedge.HedgeRatio)
				}
			}
			
			// Display global P&L
			globalPnL := pnlCalc.GetGlobalPnL()
			fmt.Printf("\nGlobal P&L: $%.2f (Unrealized: $%.2f, Realized: $%.2f, Fees: $%.2f)\n",
				globalPnL.TotalPL, globalPnL.UnrealizedPL, globalPnL.RealizedPL, globalPnL.Fees)
			
			// Display metrics
			metrics := positionMgr.GetMetrics()
			fmt.Printf("\nSystem Metrics:\n")
			fmt.Printf("  Total Accounts: %d\n", metrics["total_accounts"])
			fmt.Printf("  Total Positions: %d\n", metrics["total_positions"])
			fmt.Printf("  Update Queue Size: %d\n", metrics["update_queue_size"])
			
		case <-ctx.Done():
			return
		}
	}
}

// analyzeCorrelations periodically analyzes position correlations
func analyzeCorrelations(ctx context.Context, corrAnalyzer *position.CorrelationAnalyzer) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	// Wait for initial data
	time.Sleep(10 * time.Second)
	
	for {
		select {
		case <-ticker.C:
			// Calculate correlations for major pairs
			pairs := [][]string{
				{"BTCUSDT", "ETHUSDT"},
				{"BTCUSDT", "BNBUSDT"},
				{"ETHUSDT", "SOLUSDT"},
			}
			
			fmt.Println("\n=== Correlation Analysis ===")
			for _, pair := range pairs {
				if corr, err := corrAnalyzer.CalculatePositionCorrelation(pair[0], pair[1]); err == nil {
					fmt.Printf("%s <-> %s: %.3f\n", pair[0], pair[1], corr.Correlation)
				}
			}
			
			// Detect risk clusters
			clusters := corrAnalyzer.DetectRiskClusters(0.7)
			if len(clusters) > 0 {
				fmt.Println("\nRisk Clusters (correlation > 0.7):")
				for i, cluster := range clusters {
					fmt.Printf("  Cluster %d: %v\n", i+1, cluster)
				}
			}
			
			// Calculate account correlations
			fmt.Println("\nAccount Correlations:")
			if corrMain, err := corrAnalyzer.CalculateAccountCorrelation("main", "sub_arbitrage"); err == nil {
				fmt.Printf("  main <-> sub_arbitrage: Overlap %.1f%%, Direction %.1f%%\n",
					corrMain.PositionOverlap*100, corrMain.DirectionAlign*100)
			}
			
		case <-ctx.Done():
			return
		}
	}
}