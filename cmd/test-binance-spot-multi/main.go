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
	log.Println("Starting Binance Spot Multi-Account Test...")

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

	// Create Binance Spot connector
	connector := binance.NewSpotConnector(false) // Use mainnet

	// Add main account
	var mainAPIKey, mainAPISecret string
	if vaultClient != nil {
		mainCreds, err := vaultClient.GetAPIKey("binance_spot_main")
		if err == nil {
			mainAPIKey = mainCreds.APIKey
			mainAPISecret = mainCreds.APISecret
		}
	}
	
	// Fallback to environment variables
	if mainAPIKey == "" {
		mainAPIKey = os.Getenv("BINANCE_API_KEY")
		mainAPISecret = os.Getenv("BINANCE_API_SECRET")
	}

	if mainAPIKey != "" {
		if err := connector.AddAccount("main", mainAPIKey, mainAPISecret, "main"); err != nil {
			log.Fatalf("Failed to add main account: %v", err)
		}
	} else {
		log.Println("Warning: No API keys found for main account")
	}

	// Add sub accounts if available
	subAccounts := []string{"sub1", "sub2", "arbitrage", "market_making"}
	for _, subID := range subAccounts {
		if vaultClient != nil {
			subCreds, err := vaultClient.GetAPIKey(fmt.Sprintf("binance_spot_%s", subID))
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

	// Test 1: Get account information for all accounts
	log.Println("\n=== Testing Account Information ===")
	testAccountInfo(ctx, connector)

	// Test 2: Subscribe to market data
	log.Println("\n=== Testing Market Data Subscription ===")
	if err := testMarketData(ctx, connector); err != nil {
		log.Printf("Market data test failed: %v", err)
	}

	// Test 3: Place test orders (if in testnet or with small amounts)
	log.Println("\n=== Testing Order Management ===")
	testOrderManagement(ctx, connector)

	// Test 4: Test transfers between accounts
	log.Println("\n=== Testing Account Transfers ===")
	testAccountTransfers(ctx, connector)

	// Wait for shutdown signal
	log.Println("\nPress Ctrl+C to stop...")
	<-sigCh
	log.Println("Shutting down...")

	// Cleanup
	if err := connector.UnsubscribeAll(); err != nil {
		log.Printf("Failed to unsubscribe: %v", err)
	}
}

func testAccountInfo(ctx context.Context, connector *binance.SpotConnector) {
	// Get main account info
	info, err := connector.GetAccountInfo(ctx)
	if err != nil {
		log.Printf("Failed to get main account info: %v", err)
	} else {
		log.Printf("Main account - Can trade: %v, Can withdraw: %v", info.CanTrade, info.CanWithdraw)
		for _, balance := range info.Balances {
			if balance.Total.GreaterThan(decimal.Zero) {
				log.Printf("  %s: Total=%.8f (Free=%.8f, Locked=%.8f)", 
					balance.Asset, 
					balance.Total.InexactFloat64(),
					balance.Free.InexactFloat64(),
					balance.Locked.InexactFloat64())
			}
		}
	}

	// Get sub account info
	subInfo, err := connector.GetAccountInfoForAccount(ctx, "sub1")
	if err != nil {
		log.Printf("Failed to get sub1 account info: %v", err)
	} else {
		log.Printf("\nSub1 account - Can trade: %v", subInfo.CanTrade)
		usdtFound := false
		for _, balance := range subInfo.Balances {
			if balance.Asset == "USDT" {
				usdtFound = true
				log.Printf("  USDT Balance: %.8f", balance.Total.InexactFloat64())
			}
		}
		if !usdtFound {
			log.Printf("  USDT Balance: 0.00000000")
		}
	}
}

func testMarketData(ctx context.Context, connector *binance.SpotConnector) error {
	// Subscribe to order book updates
	err := connector.SubscribeOrderBook("BTCUSDT", func(orderBook *types.OrderBook) {
		log.Printf("OrderBook Update - Symbol: %s, Best Bid: %.2f @ %.4f, Best Ask: %.2f @ %.4f",
			orderBook.Symbol,
			orderBook.Bids[0].Price.InexactFloat64(),
			orderBook.Bids[0].Quantity.InexactFloat64(),
			orderBook.Asks[0].Price.InexactFloat64(),
			orderBook.Asks[0].Quantity.InexactFloat64())
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to order book: %v", err)
	}

	// Subscribe to trades
	err = connector.SubscribeTrades("BTCUSDT", func(trade *types.Trade) {
		side := "BUY"
		if trade.IsBuyerMaker {
			side = "SELL"
		}
		log.Printf("Trade - %s %s: %.8f @ %.2f",
			trade.Symbol,
			side,
			trade.Quantity.InexactFloat64(),
			trade.Price.InexactFloat64())
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to trades: %v", err)
	}

	// Get current market data
	symbols := []string{"BTCUSDT", "ETHUSDT"}
	marketData, err := connector.GetMarketData(ctx, symbols)
	if err != nil {
		return fmt.Errorf("failed to get market data: %v", err)
	}

	for symbol, data := range marketData {
		log.Printf("Market Data - %s: Last=%.2f, Bid=%.2f, Ask=%.2f, Volume24h=%.2f",
			symbol,
			data.LastPrice.InexactFloat64(),
			data.BidPrice.InexactFloat64(),
			data.AskPrice.InexactFloat64(),
			data.Volume.InexactFloat64())
	}

	return nil
}

func testOrderManagement(ctx context.Context, connector *binance.SpotConnector) {
	// Get symbol info first
	symbolInfo, err := connector.GetSymbolInfo(ctx, "BTCUSDT")
	if err != nil {
		log.Printf("Failed to get symbol info: %v", err)
		return
	}

	log.Printf("Symbol Info - BTC/USDT: MinQty=%.8f, StepSize=%.8f, MinNotional=%.2f",
		symbolInfo.MinQty.InexactFloat64(),
		symbolInfo.StepSize.InexactFloat64(),
		symbolInfo.MinNotional.InexactFloat64())

	// Get open orders for main account
	openOrders, err := connector.GetOpenOrders(ctx, "BTCUSDT")
	if err != nil {
		log.Printf("Failed to get open orders: %v", err)
	} else {
		log.Printf("Open orders for BTCUSDT: %d", len(openOrders))
		for _, order := range openOrders {
			log.Printf("  Order %s: %s %s %.8f @ %.2f",
				order.ID,
				order.Side,
				order.Type,
				order.Quantity.InexactFloat64(),
				order.Price.InexactFloat64())
		}
	}

	// WARNING: Only place orders if you understand the risks
	// This is just an example - adjust price to be far from market
	/*
	testOrder := &types.Order{
		AccountID:     "main",
		Symbol:        "BTCUSDT",
		Side:          types.OrderSideBuy,
		Type:          types.OrderTypeLimit,
		Price:         decimal.NewFromFloat(30000), // Far below market price
		Quantity:      symbolInfo.MinQty,           // Minimum quantity
		TimeInForce:   types.TimeInForceGTC,
		ClientOrderID: fmt.Sprintf("test_%d", time.Now().Unix()),
	}

	placedOrder, err := connector.PlaceOrder(ctx, testOrder)
	if err != nil {
		log.Printf("Failed to place test order: %v", err)
	} else {
		log.Printf("Placed test order: %s", placedOrder.ID)
		
		// Cancel the order immediately
		time.Sleep(1 * time.Second)
		if err := connector.CancelOrder(ctx, "BTCUSDT", placedOrder.ID); err != nil {
			log.Printf("Failed to cancel test order: %v", err)
		} else {
			log.Printf("Cancelled test order: %s", placedOrder.ID)
		}
	}
	*/
}

func testAccountTransfers(ctx context.Context, connector *binance.SpotConnector) {
	// WARNING: This will actually transfer funds between accounts
	// Only uncomment if you want to test transfers
	
	/*
	// Example: Transfer 1 USDT from main to sub1
	transferID, err := connector.TransferToSubAccount(ctx, "sub1", "USDT", 1.0)
	if err != nil {
		log.Printf("Failed to transfer to sub account: %v", err)
	} else {
		log.Printf("Transfer initiated: %s", transferID)
	}

	// Example: Transfer between sub accounts
	transferID2, err := connector.TransferBetweenSubAccounts(ctx, "sub1", "sub2", "USDT", 0.5)
	if err != nil {
		log.Printf("Failed to transfer between sub accounts: %v", err)
	} else {
		log.Printf("Sub-to-sub transfer initiated: %s", transferID2)
	}
	*/

	log.Println("Transfer test skipped - uncomment code to test actual transfers")
}