package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mExOms/pkg/security"
	"github.com/mExOms/pkg/types"
	"github.com/mExOms/services/binance"
	"github.com/shopspring/decimal"
)

func main() {
	log.Println("Starting Binance Futures Multi-Account Test...")

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Initialize Vault client
	vaultClient, err := security.NewVaultManager("http://127.0.0.1:8200", os.Getenv("VAULT_TOKEN"))
	if err != nil {
		log.Printf("Warning: Could not initialize Vault client: %v", err)
	}

	// Create Binance Futures connector
	connector := binance.NewFuturesConnector(false) // Use mainnet

	// Add main account
	var mainAPIKey, mainAPISecret string
	if vaultClient != nil {
		mainCreds, err := vaultClient.GetAPIKey("binance_futures_main")
		if err == nil {
			mainAPIKey = mainCreds.APIKey
			mainAPISecret = mainCreds.APISecret
		}
	}
	
	// Fallback to environment variables
	if mainAPIKey == "" {
		mainAPIKey = os.Getenv("BINANCE_FUTURES_API_KEY")
		mainAPISecret = os.Getenv("BINANCE_FUTURES_API_SECRET")
	}

	if mainAPIKey != "" {
		if err := connector.AddAccount("main", mainAPIKey, mainAPISecret, "main"); err != nil {
			log.Fatalf("Failed to add main account: %v", err)
		}
	} else {
		log.Println("Warning: No API keys found for main account")
	}

	// Add sub accounts if available
	subAccounts := []string{"sub_futures_trend", "sub_futures_hedge", "sub_futures_mm"}
	for _, subID := range subAccounts {
		if vaultClient != nil {
			subCreds, err := vaultClient.GetAPIKey(fmt.Sprintf("binance_futures_%s", subID))
			if err == nil {
				if err := connector.AddAccount(subID, subCreds.APIKey, subCreds.APISecret, "sub"); err != nil {
					log.Printf("Failed to add sub account %s: %v", subID, err)
				} else {
					log.Printf("Added sub account: %s", subID)
				}
			}
		}
	}

	// Initialize connector
	if err := connector.Initialize(ctx); err != nil {
		log.Printf("Warning: Failed to initialize connector: %v", err)
	}

	// Test 1: Get account information
	log.Println("\n=== Testing Account Information ===")
	testAccountInfo(ctx, connector)

	// Test 2: Get positions
	log.Println("\n=== Testing Position Management ===")
	testPositions(ctx, connector)

	// Test 3: Test leverage and margin settings
	log.Println("\n=== Testing Leverage & Margin ===")
	testLeverageMargin(ctx, connector)

	// Test 4: Get funding rates
	log.Println("\n=== Testing Funding Rates ===")
	testFundingRates(ctx, connector)

	// Test 5: Market data
	log.Println("\n=== Testing Market Data ===")
	testMarketData(ctx, connector)

	// Test 6: Order management (WARNING: This will place real orders!)
	log.Println("\n=== Testing Order Management (DEMO) ===")
	testOrderManagement(ctx, connector)

	// Wait for shutdown signal
	log.Println("\nPress Ctrl+C to stop...")
	<-sigCh
	log.Println("Shutting down...")
}

func testAccountInfo(ctx context.Context, connector *binance.FuturesConnector) {
	// Get main account info
	info, err := connector.GetAccountInfo(ctx)
	if err != nil {
		log.Printf("Failed to get main account info: %v", err)
	} else {
		log.Printf("Main account - Can trade: %v, Can withdraw: %v", info.CanTrade, info.CanWithdraw)
		log.Printf("  Total Balance: %.8f USDT", info.TotalBalance.InexactFloat64())
		log.Printf("  Unrealized PnL: %.8f USDT", info.UnrealizedPnL.InexactFloat64())
		
		for _, balance := range info.Balances {
			if balance.Total.GreaterThan(decimal.Zero) || balance.UnrealizedPnL.IsNonZero() {
				log.Printf("  %s: Total=%.8f, Free=%.8f, PnL=%.8f", 
					balance.Asset, 
					balance.Total.InexactFloat64(),
					balance.Free.InexactFloat64(),
					balance.UnrealizedPnL.InexactFloat64())
			}
		}
	}

	// Get sub account info
	subInfo, err := connector.GetAccountInfoForAccount(ctx, "sub_futures_trend")
	if err != nil {
		log.Printf("Failed to get sub account info: %v", err)
	} else {
		log.Printf("\nSub account - Total Balance: %.8f USDT", subInfo.TotalBalance.InexactFloat64())
	}
}

func testPositions(ctx context.Context, connector *binance.FuturesConnector) {
	positions, err := connector.GetPositions(ctx)
	if err != nil {
		log.Printf("Failed to get positions: %v", err)
		return
	}

	if len(positions) == 0 {
		log.Println("No open positions")
	} else {
		log.Printf("Found %d open positions:", len(positions))
		for _, pos := range positions {
			log.Printf("  %s %s: %.4f @ %.2f, PnL=%.8f, Margin=%.8f",
				pos.Symbol,
				pos.Side,
				pos.Quantity.InexactFloat64(),
				pos.EntryPrice.InexactFloat64(),
				pos.UnrealizedPnL.InexactFloat64(),
				pos.Margin.InexactFloat64())
			
			if pos.LiquidationPrice.GreaterThan(decimal.Zero) {
				log.Printf("    Liquidation Price: %.2f", pos.LiquidationPrice.InexactFloat64())
			}
		}
	}

	// Get specific position
	btcPosition, err := connector.GetPosition(ctx, "BTCUSDT")
	if err != nil {
		log.Printf("Failed to get BTCUSDT position: %v", err)
	} else if btcPosition.Quantity.IsZero() {
		log.Println("No position in BTCUSDT")
	} else {
		log.Printf("BTCUSDT position: %s %.4f @ %.2f",
			btcPosition.Side,
			btcPosition.Quantity.InexactFloat64(),
			btcPosition.EntryPrice.InexactFloat64())
	}
}

func testLeverageMargin(ctx context.Context, connector *binance.FuturesConnector) {
	symbol := "BTCUSDT"
	
	// Get symbol info first
	symbolInfo, err := connector.GetSymbolInfo(ctx, symbol)
	if err != nil {
		log.Printf("Failed to get symbol info: %v", err)
		return
	}
	
	log.Printf("Symbol %s - Max Leverage: %dx", symbol, symbolInfo.MaxLeverage)
	
	// WARNING: This will actually change leverage settings!
	// Only uncomment if you want to test
	/*
	// Set leverage
	if err := connector.SetLeverage(ctx, symbol, 10); err != nil {
		log.Printf("Failed to set leverage: %v", err)
	} else {
		log.Printf("Set leverage to 10x for %s", symbol)
	}
	
	// Set margin mode
	if err := connector.SetMarginMode(ctx, symbol, types.MarginModeIsolated); err != nil {
		log.Printf("Failed to set margin mode: %v", err)
	} else {
		log.Printf("Set margin mode to ISOLATED for %s", symbol)
	}
	*/
}

func testFundingRates(ctx context.Context, connector *binance.FuturesConnector) {
	symbols := []string{"BTCUSDT", "ETHUSDT"}
	
	for _, symbol := range symbols {
		fundingRate, err := connector.GetFundingRate(ctx, symbol)
		if err != nil {
			log.Printf("Failed to get funding rate for %s: %v", symbol, err)
			continue
		}
		
		ratePercent := fundingRate.Rate.Mul(decimal.NewFromInt(100))
		log.Printf("%s Funding Rate: %.4f%% (Next: %s)",
			symbol,
			ratePercent.InexactFloat64(),
			fundingRate.NextFundingTime.Format("15:04:05"))
	}
}

func testMarketData(ctx context.Context, connector *binance.FuturesConnector) {
	// Get market data
	symbols := []string{"BTCUSDT", "ETHUSDT"}
	marketData, err := connector.GetMarketData(ctx, symbols)
	if err != nil {
		log.Printf("Failed to get market data: %v", err)
		return
	}

	for symbol, data := range marketData {
		log.Printf("%s: Last=%.2f, Volume24h=%.2f, High=%.2f, Low=%.2f",
			symbol,
			data.LastPrice.InexactFloat64(),
			data.Volume.InexactFloat64(),
			data.High24h.InexactFloat64(),
			data.Low24h.InexactFloat64())
	}

	// Get order book
	orderBook, err := connector.GetOrderBook(ctx, "BTCUSDT", 10)
	if err != nil {
		log.Printf("Failed to get order book: %v", err)
	} else {
		log.Printf("\nBTCUSDT Order Book:")
		log.Printf("Best Bid: %.2f @ %.4f", 
			orderBook.Bids[0].Price.InexactFloat64(),
			orderBook.Bids[0].Quantity.InexactFloat64())
		log.Printf("Best Ask: %.2f @ %.4f",
			orderBook.Asks[0].Price.InexactFloat64(),
			orderBook.Asks[0].Quantity.InexactFloat64())
		
		spread := orderBook.Asks[0].Price.Sub(orderBook.Bids[0].Price)
		spreadPercent := spread.Div(orderBook.Bids[0].Price).Mul(decimal.NewFromInt(100))
		log.Printf("Spread: %.2f (%.4f%%)", 
			spread.InexactFloat64(),
			spreadPercent.InexactFloat64())
	}
}

func testOrderManagement(ctx context.Context, connector *binance.FuturesConnector) {
	// Get open orders
	openOrders, err := connector.GetOpenOrders(ctx, "")
	if err != nil {
		log.Printf("Failed to get open orders: %v", err)
	} else {
		log.Printf("Open orders: %d", len(openOrders))
		for _, order := range openOrders {
			log.Printf("  %s %s %s: %.4f @ %.2f (%s)",
				order.Symbol,
				order.Side,
				order.Type,
				order.Quantity.InexactFloat64(),
				order.Price.InexactFloat64(),
				order.Status)
		}
	}

	// WARNING: Only place orders if you understand the risks!
	// This is just a demo - adjust parameters carefully
	/*
	// Example: Place a limit order far from market
	symbolInfo, _ := connector.GetSymbolInfo(ctx, "BTCUSDT")
	if symbolInfo != nil {
		testOrder := &types.Order{
			AccountID:    "main",
			Symbol:       "BTCUSDT",
			Side:         types.OrderSideBuy,
			Type:         types.OrderTypeLimit,
			Price:        decimal.NewFromFloat(30000), // Far below market
			Quantity:     symbolInfo.MinQty,          // Minimum quantity
			TimeInForce:  types.TimeInForceGTC,
			PositionSide: "LONG", // For hedge mode
			ClientOrderID: fmt.Sprintf("test_%d", time.Now().Unix()),
		}

		placedOrder, err := connector.PlaceOrder(ctx, testOrder)
		if err != nil {
			log.Printf("Failed to place test order: %v", err)
		} else {
			log.Printf("Placed test order: %s", placedOrder.ID)
			
			// Cancel it immediately
			time.Sleep(1 * time.Second)
			if err := connector.CancelOrder(ctx, "BTCUSDT", placedOrder.ID); err != nil {
				log.Printf("Failed to cancel test order: %v", err)
			} else {
				log.Printf("Cancelled test order: %s", placedOrder.ID)
			}
		}
	}
	*/
	
	log.Println("Order placement test skipped - uncomment code to test actual orders")
}