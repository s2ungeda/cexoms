package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	
	"github.com/mExOms/pkg/types"
	"github.com/mExOms/services/binance/futures"
	"github.com/shopspring/decimal"
)

func main() {
	fmt.Println("=== Testing Binance Futures Connector ===\n")
	
	// Test API keys (use environment variables in production)
	apiKey := os.Getenv("BINANCE_API_KEY")
	apiSecret := os.Getenv("BINANCE_API_SECRET")
	
	// For testing without API keys
	if apiKey == "" || apiSecret == "" {
		fmt.Println("⚠️  No API keys provided, running in demo mode")
		apiKey = "test_key"
		apiSecret = "test_secret"
	}
	
	// Create Binance Futures client
	client, err := futures.NewBinanceFutures(apiKey, apiSecret, true) // testnet
	if err != nil {
		log.Fatal("Failed to create Binance Futures client:", err)
	}
	defer client.Close()
	
	fmt.Println("✓ Binance Futures client created")
	
	// Test basic functionality
	testBasicFunctions(client)
	
	// Test position management
	testPositionManagement(client)
	
	// Test WebSocket subscriptions
	testWebSocketStreams(client)
	
	// Wait for interrupt signal
	fmt.Println("\n✓ Press Ctrl+C to exit...")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan
	
	fmt.Println("\n✓ Shutting down...")
}

func testBasicFunctions(client *futures.BinanceFutures) {
	fmt.Println("\n=== Testing Basic Functions ===")
	
	// Test connection
	if client.IsConnected() {
		fmt.Println("✓ Connected to Binance Futures")
	} else {
		fmt.Println("✗ Not connected to Binance Futures (expected in demo mode)")
	}
	
	// Test with BTCUSDT perpetual
	symbol := "BTCUSDT"
	_ = symbol // Will be used in production
	
	// Get exchange info
	info, err := client.GetExchangeInfo()
	if err != nil {
		fmt.Printf("✗ Failed to get exchange info: %v (expected without API keys)\n", err)
	} else {
		fmt.Printf("✓ Exchange info retrieved: %d trading symbols\n", len(info.Symbols))
	}
	
	// Get account info
	account, err := client.GetAccount()
	if err != nil {
		fmt.Printf("✗ Failed to get account: %v (expected without API keys)\n", err)
	} else {
		fmt.Printf("✓ Account balance: %s USDT\n", account.TotalBalance)
		fmt.Printf("  Available: %s USDT\n", account.AvailableBalance)
		fmt.Printf("  Total margin: %s USDT\n", account.TotalMargin)
		fmt.Printf("  Unrealized PnL: %s USDT\n", account.TotalUnrealizedPnL)
	}
	
	// Get positions
	positions, err := client.GetPositions()
	if err != nil {
		fmt.Printf("✗ Failed to get positions: %v\n", err)
	} else {
		fmt.Printf("✓ Open positions: %d\n", len(positions))
		for _, pos := range positions {
			if !pos.Quantity.IsZero() {
				fmt.Printf("  %s: %s contracts @ %s (PnL: %s)\n", 
					pos.Symbol, pos.Quantity, pos.EntryPrice, pos.UnrealizedPnL)
			}
		}
	}
}

func testPositionManagement(client *futures.BinanceFutures) {
	fmt.Println("\n=== Testing Position Management ===")
	
	symbol := "BTCUSDT"
	
	// Get position mode
	mode, err := client.GetPositionMode()
	if err != nil {
		fmt.Printf("✗ Failed to get position mode: %v\n", err)
	} else {
		fmt.Printf("✓ Position mode: %s\n", mode)
	}
	
	// Get leverage
	leverageInfo, err := client.GetLeverage(symbol)
	if err != nil {
		fmt.Printf("✗ Failed to get leverage: %v\n", err)
	} else {
		fmt.Printf("✓ Current leverage for %s: %dx\n", symbol, leverageInfo.Leverage)
		fmt.Printf("  Max notional: %s\n", leverageInfo.MaxNotionalValue)
	}
	
	// Test setting leverage (demo mode will fail)
	newLeverage := 10
	if err := client.SetLeverage(symbol, newLeverage); err != nil {
		fmt.Printf("✗ Failed to set leverage: %v (expected without API keys)\n", err)
	} else {
		fmt.Printf("✓ Leverage set to %dx\n", newLeverage)
	}
	
	// Get funding rate
	funding, err := client.GetFundingRate(symbol)
	if err != nil {
		fmt.Printf("✗ Failed to get funding rate: %v\n", err)
	} else {
		fmt.Printf("✓ Funding rate for %s: %s%%\n", 
			symbol, funding.FundingRate.Mul(decimal.NewFromInt(100)))
		fmt.Printf("  Next funding time: %s\n", funding.FundingTime.Format("15:04:05"))
	}
	
	// Get position risk
	risks, err := client.GetPositionRisk(symbol)
	if err != nil {
		fmt.Printf("✗ Failed to get position risk: %v\n", err)
	} else {
		fmt.Printf("✓ Position risk info retrieved\n")
		for _, risk := range risks {
			if !risk.PositionAmount.IsZero() {
				fmt.Printf("  %s %s: %s @ %s\n", 
					risk.Symbol, risk.PositionSide, risk.PositionAmount, risk.EntryPrice)
				fmt.Printf("    Mark Price: %s, Liq Price: %s\n", 
					risk.MarkPrice, risk.LiquidationPrice)
				fmt.Printf("    Unrealized PnL: %s\n", risk.UnrealizedPnL)
			}
		}
	}
}

func testWebSocketStreams(client *futures.BinanceFutures) {
	fmt.Println("\n=== Testing WebSocket Streams ===")
	
	symbol := "BTCUSDT"
	
	// Subscribe to ticker
	if err := client.SubscribeTicker(symbol); err != nil {
		fmt.Printf("✗ Failed to subscribe to ticker: %v\n", err)
	} else {
		fmt.Printf("✓ Subscribed to %s ticker\n", symbol)
	}
	
	// Subscribe to trades
	if err := client.SubscribeTrades(symbol); err != nil {
		fmt.Printf("✗ Failed to subscribe to trades: %v\n", err)
	} else {
		fmt.Printf("✓ Subscribed to %s trades\n", symbol)
	}
	
	// Subscribe to order book
	if err := client.SubscribeOrderBook(symbol, 20); err != nil {
		fmt.Printf("✗ Failed to subscribe to order book: %v\n", err)
	} else {
		fmt.Printf("✓ Subscribed to %s order book\n", symbol)
	}
	
	// Subscribe to mark price
	if err := client.SubscribeMarkPrice(symbol); err != nil {
		fmt.Printf("✗ Failed to subscribe to mark price: %v\n", err)
	} else {
		fmt.Printf("✓ Subscribed to %s mark price\n", symbol)
	}
	
	// Subscribe to liquidations
	if err := client.SubscribeLiquidation(symbol); err != nil {
		fmt.Printf("✗ Failed to subscribe to liquidations: %v\n", err)
	} else {
		fmt.Printf("✓ Subscribed to %s liquidations\n", symbol)
	}
	
	// Subscribe to klines
	if err := client.SubscribeKline(symbol, "1m"); err != nil {
		fmt.Printf("✗ Failed to subscribe to klines: %v\n", err)
	} else {
		fmt.Printf("✓ Subscribed to %s 1m klines\n", symbol)
	}
	
	// Subscribe to user data (requires valid API keys)
	if err := client.SubscribeUserData(); err != nil {
		fmt.Printf("✗ Failed to subscribe to user data: %v (expected without API keys)\n", err)
	} else {
		fmt.Printf("✓ Subscribed to user data stream\n")
	}
}

func testOrderPlacement(client *futures.BinanceFutures) {
	fmt.Println("\n=== Testing Order Placement ===")
	
	symbol := "BTCUSDT"
	
	// Create a test limit order (will fail without API keys)
	testOrder := &types.Order{
		Symbol:       symbol,
		Side:         types.OrderSideBuy,
		Type:         types.OrderTypeLimit,
		Price:        decimal.NewFromInt(25000), // Far from market price
		Quantity:     decimal.NewFromFloat(0.001),
		PositionSide: types.PositionSideLong,
		ReduceOnly:   false,
	}
	
	orderResp, err := client.CreateOrder(testOrder)
	if err != nil {
		fmt.Printf("✗ Failed to create test order: %v (expected without API keys)\n", err)
	} else {
		fmt.Printf("✓ Test order created: ID=%s, Status=%s\n", orderResp.OrderID, orderResp.Status)
		
		// Cancel the test order
		time.Sleep(2 * time.Second)
		if err := client.CancelOrder(symbol, orderResp.OrderID); err != nil {
			fmt.Printf("✗ Failed to cancel test order: %v\n", err)
		} else {
			fmt.Println("✓ Test order cancelled")
		}
	}
}