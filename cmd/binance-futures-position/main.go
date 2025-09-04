package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/adshao/go-binance/v2/futures"
	"github.com/nats-io/nats.go"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

type FuturesPositionData struct {
	Symbol           string    `json:"symbol"`
	Side             string    `json:"side"`              // LONG or SHORT
	Quantity         float64   `json:"quantity"`          // Position size
	EntryPrice       float64   `json:"entry_price"`       // Average entry price
	MarkPrice        float64   `json:"mark_price"`        // Current mark price
	UnrealizedPnL    float64   `json:"unrealized_pnl"`    // Unrealized profit/loss
	RealizedPnL      float64   `json:"realized_pnl"`      // Realized profit/loss
	Percentage       float64   `json:"percentage"`        // PnL percentage
	MarginType       string    `json:"margin_type"`       // cross or isolated
	IsolatedMargin   float64   `json:"isolated_margin"`   // Margin in isolated mode
	PositionValue    float64   `json:"position_value"`    // Position notional value
	InitialMargin    float64   `json:"initial_margin"`    // Initial margin required
	MaintMargin      float64   `json:"maint_margin"`      // Maintenance margin
	Leverage         int       `json:"leverage"`          // Position leverage
	LiquidationPrice float64   `json:"liquidation_price"` // Liquidation price
	Timestamp        time.Time `json:"timestamp"`
}

func main() {
	// Initialize logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Get API credentials
	apiKey := os.Getenv("BINANCE_FUTURES_API_KEY")
	apiSecret := os.Getenv("BINANCE_FUTURES_SECRET_KEY")
	
	// If not in environment, try to get from Vault
	if apiKey == "" || apiSecret == "" {
		logger.Info("API keys not found in environment, checking Vault...")
		
		// Try to get from Vault
		apiKey, apiSecret = getKeysFromVault(logger)
		
		if apiKey == "" || apiSecret == "" {
			logger.Fatal("Failed to get API keys from environment or Vault")
		}
		
		logger.Info("Successfully retrieved API keys from Vault")
	}

	// Connect to NATS
	nc, err := nats.Connect("nats://127.0.0.1:4222")
	if err != nil {
		logger.Fatal("Failed to connect to NATS", zap.Error(err))
	}
	defer nc.Close()

	// Create Binance Futures client
	client := futures.NewClient(apiKey, apiSecret)
	
	// Main loop to fetch and publish positions
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Fetch immediately
	fetchAndPublishPositions(client, nc, logger)

	for range ticker.C {
		fetchAndPublishPositions(client, nc, logger)
	}
}

func fetchAndPublishPositions(client *futures.Client, nc *nats.Conn, logger *zap.Logger) {
	// Get account info including positions
	account, err := client.NewGetAccountService().Do(context.Background())
	if err != nil {
		logger.Error("Failed to get futures account info", zap.Error(err))
		return
	}

	// Get all positions (including zero positions)
	positionRisk, err := client.NewGetPositionRiskService().Do(context.Background())
	if err != nil {
		logger.Error("Failed to get position risk", zap.Error(err))
		return
	}

	var positions []FuturesPositionData
	totalUnrealizedPnL := 0.0

	for _, pos := range positionRisk {
		posAmt := parseFloat(pos.PositionAmt)
		
		// Skip zero positions
		if posAmt == 0 {
			continue
		}

		// Determine side
		side := "LONG"
		if posAmt < 0 {
			side = "SHORT"
			posAmt = -posAmt // Make positive for display
		}

		entryPrice := parseFloat(pos.EntryPrice)
		markPrice := parseFloat(pos.MarkPrice)
		unrealizedPnL := parseFloat(pos.UnRealizedProfit)
		
		// Calculate PnL percentage
		percentage := 0.0
		if entryPrice > 0 {
			if side == "LONG" {
				percentage = ((markPrice - entryPrice) / entryPrice) * 100
			} else {
				percentage = ((entryPrice - markPrice) / entryPrice) * 100
			}
		}

		// Get liquidation price
		liquidationPrice := parseFloat(pos.LiquidationPrice)
		
		position := FuturesPositionData{
			Symbol:           pos.Symbol,
			Side:             side,
			Quantity:         posAmt,
			EntryPrice:       entryPrice,
			MarkPrice:        markPrice,
			UnrealizedPnL:    unrealizedPnL,
			RealizedPnL:      0, // Would need to fetch from trade history
			Percentage:       percentage,
			MarginType:       pos.MarginType,
			IsolatedMargin:   parseFloat(pos.IsolatedMargin),
			PositionValue:    posAmt * markPrice,
			InitialMargin:    parseFloat(pos.IsolatedWallet),  // Use IsolatedWallet instead
			MaintMargin:      0, // Not directly available in API response
			Leverage:         parseInt(pos.Leverage),
			LiquidationPrice: liquidationPrice,
			Timestamp:        time.Now(),
		}

		positions = append(positions, position)
		totalUnrealizedPnL += unrealizedPnL

		// Publish individual position
		data, _ := json.Marshal(position)
		subject := fmt.Sprintf("position.futures.%s", pos.Symbol)
		if err := nc.Publish(subject, data); err != nil {
			logger.Error("Failed to publish position", 
				zap.String("symbol", pos.Symbol),
				zap.Error(err))
		}
	}

	// Get account assets (futures balances)
	var balances []map[string]interface{}
	for _, asset := range account.Assets {
		walletBalance := parseFloat(asset.WalletBalance)
		if walletBalance > 0 {
			balances = append(balances, map[string]interface{}{
				"asset":           asset.Asset,
				"wallet_balance":  walletBalance,
				"cross_wallet":    parseFloat(asset.CrossWalletBalance),
				"available":       parseFloat(asset.AvailableBalance),
				"max_withdrawable": parseFloat(asset.MaxWithdrawAmount),
				"unrealized_profit": parseFloat(asset.UnrealizedProfit),
				"margin_balance":  parseFloat(asset.MarginBalance),
			})
		}
	}

	// Publish aggregated position data
	aggregatedData := map[string]interface{}{
		"positions":          positions,
		"balances":           balances,
		"total_unrealized_pnl": totalUnrealizedPnL,
		"account_balance":     parseFloat(account.TotalWalletBalance),
		"total_margin_balance": parseFloat(account.TotalMarginBalance),
		"available_balance":   parseFloat(account.AvailableBalance),
		"total_initial_margin": parseFloat(account.TotalInitialMargin),
		"total_maint_margin":  parseFloat(account.TotalMaintMargin),
		"timestamp":          time.Now(),
		"exchange":           "binance",
		"account_type":       "futures",
	}

	data, _ := json.Marshal(aggregatedData)
	if err := nc.Publish("position.futures.all", data); err != nil {
		logger.Error("Failed to publish aggregated positions", zap.Error(err))
	}

	logger.Info("Published futures positions",
		zap.Int("count", len(positions)),
		zap.Float64("total_unrealized_pnl", totalUnrealizedPnL))
}

func parseFloat(s string) float64 {
	d, _ := decimal.NewFromString(s)
	f, _ := d.Float64()
	return f
}

func parseInt(s string) int {
	d, _ := decimal.NewFromString(s)
	return int(d.IntPart())
}

func getKeysFromVault(logger *zap.Logger) (string, string) {
	// Read Vault token
	tokenData, err := os.ReadFile(os.ExpandEnv("$HOME/.mExOms/vault-token"))
	if err != nil {
		logger.Error("Failed to read Vault token", zap.Error(err))
		return "", ""
	}
	vaultToken := strings.TrimSpace(string(tokenData))
	
	// Make request to Vault
	req, _ := http.NewRequest("GET", "http://localhost:8200/v1/secret/data/exchanges/binance_futures", nil)
	req.Header.Set("X-Vault-Token", vaultToken)
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("Failed to connect to Vault", zap.Error(err))
		return "", ""
	}
	defer resp.Body.Close()
	
	// Parse response
	var vaultResp struct {
		Data struct {
			Data map[string]string `json:"data"`
		} `json:"data"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&vaultResp); err != nil {
		logger.Error("Failed to decode Vault response", zap.Error(err))
		return "", ""
	}
	
	apiKey := vaultResp.Data.Data["api_key"]
	secretKey := vaultResp.Data.Data["secret_key"]
	
	return apiKey, secretKey
}