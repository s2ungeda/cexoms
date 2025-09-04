package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/adshao/go-binance/v2"
	"github.com/nats-io/nats.go"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	
	vault "github.com/mExOms/pkg/vault"
)

type BalanceData struct {
	Asset     string    `json:"asset"`
	Free      float64   `json:"free"`
	Locked    float64   `json:"locked"`
	Total     float64   `json:"total"`
	USDValue  float64   `json:"usd_value"`
	Timestamp time.Time `json:"timestamp"`
}

func main() {
	// Initialize logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Get API credentials from environment or Vault
	apiKey := os.Getenv("BINANCE_API_KEY")
	apiSecret := os.Getenv("BINANCE_SECRET_KEY")
	
	// If not in environment, try to get from Vault
	if apiKey == "" || apiSecret == "" {
		logger.Info("API keys not found in environment, checking Vault...")
		
		// Import vault package
		vaultClient, err := vault.NewClient(vault.Config{})
		if err != nil {
			logger.Error("Failed to connect to Vault", zap.Error(err))
			logger.Info("Please ensure Vault is running or set BINANCE_API_KEY and BINANCE_SECRET_KEY environment variables")
			logger.Fatal("Cannot proceed without API keys")
		}
		
		// Get keys from Vault
		keys, err := vaultClient.GetExchangeKeys("binance", "spot")
		if err != nil {
			logger.Error("Failed to get keys from Vault", zap.Error(err))
			logger.Info("Please run: ./scripts/store-binance-keys.sh to store your API keys")
			logger.Fatal("Cannot proceed without API keys")
		}
		
		apiKey = keys["api_key"]
		apiSecret = keys["secret_key"]
		logger.Info("Successfully retrieved API keys from Vault")
	}

	// Connect to NATS
	nc, err := nats.Connect("nats://127.0.0.1:4222")
	if err != nil {
		logger.Fatal("Failed to connect to NATS", zap.Error(err))
	}
	defer nc.Close()

	// Create Binance client
	client := binance.NewClient(apiKey, apiSecret)
	
	// Get current prices for USD conversion
	priceMap := make(map[string]float64)
	
	// Fetch all tickers
	tickers, err := client.NewListPricesService().Do(context.Background())
	if err != nil {
		logger.Error("Failed to get prices", zap.Error(err))
	} else {
		for _, t := range tickers {
			priceMap[t.Symbol] = parseFloat(t.Price)
		}
	}

	// Main loop to fetch and publish balances
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Fetch immediately
	fetchAndPublishBalances(client, nc, priceMap, logger)

	for range ticker.C {
		fetchAndPublishBalances(client, nc, priceMap, logger)
	}
}

func fetchAndPublishBalances(client *binance.Client, nc *nats.Conn, priceMap map[string]float64, logger *zap.Logger) {
	// Get account info
	account, err := client.NewGetAccountService().Do(context.Background())
	if err != nil {
		logger.Error("Failed to get account info", zap.Error(err))
		return
	}

	// Process balances
	var balances []BalanceData
	totalUSDValue := 0.0

	for _, balance := range account.Balances {
		free := parseFloat(balance.Free)
		locked := parseFloat(balance.Locked)
		total := free + locked

		// Skip zero balances
		if total == 0 {
			continue
		}

		// Calculate USD value
		usdValue := 0.0
		if balance.Asset == "USDT" || balance.Asset == "BUSD" || balance.Asset == "USDC" {
			usdValue = total
		} else {
			// Try direct USDT pair
			if price, ok := priceMap[balance.Asset+"USDT"]; ok {
				usdValue = total * price
			} else if price, ok := priceMap[balance.Asset+"BUSD"]; ok {
				usdValue = total * price
			} else if balance.Asset == "BNB" {
				// Special handling for BNB
				if price, ok := priceMap["BNBUSDT"]; ok {
					usdValue = total * price
				}
			}
		}

		balanceData := BalanceData{
			Asset:     balance.Asset,
			Free:      free,
			Locked:    locked,
			Total:     total,
			USDValue:  usdValue,
			Timestamp: time.Now(),
		}

		balances = append(balances, balanceData)
		totalUSDValue += usdValue

		// Publish individual balance
		data, _ := json.Marshal(balanceData)
		subject := fmt.Sprintf("balance.spot.%s", balance.Asset)
		if err := nc.Publish(subject, data); err != nil {
			logger.Error("Failed to publish balance", 
				zap.String("asset", balance.Asset),
				zap.Error(err))
		}
	}

	// Publish aggregated balance data
	aggregatedData := map[string]interface{}{
		"balances":       balances,
		"total_usd_value": totalUSDValue,
		"timestamp":      time.Now(),
		"exchange":       "binance",
		"account_type":   "spot",
	}

	data, _ := json.Marshal(aggregatedData)
	if err := nc.Publish("balance.spot.all", data); err != nil {
		logger.Error("Failed to publish aggregated balance", zap.Error(err))
	}

	logger.Info("Published spot balances",
		zap.Int("count", len(balances)),
		zap.Float64("total_usd", totalUSDValue))
}

func parseFloat(s string) float64 {
	d, _ := decimal.NewFromString(s)
	f, _ := d.Float64()
	return f
}