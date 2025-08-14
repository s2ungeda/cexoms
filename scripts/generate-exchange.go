package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// ExchangeConfig holds configuration for generating exchange connector
type ExchangeConfig struct {
	// Basic info
	Exchange     string // e.g., "Bybit"
	ExchangeName string // e.g., "Bybit"
	ExchangeLower string // e.g., "bybit"
	ExchangeUpper string // e.g., "BYBIT"
	Package      string // e.g., "bybit"
	MarketType   string // e.g., "Spot" or "Futures"
	MarketTypeLower string // e.g., "spot" or "futures"
	
	// Features
	HasFutures      bool
	HasApiPassphrase bool
	
	// API Configuration
	RestURL       string
	WsPublicURL   string
	WsPrivateURL  string
	RateLimit     int
	
	// Symbol formats
	SymbolFormat     string // e.g., "BTCUSDT"
	BTCSymbol        string
	ETHSymbol        string
	BNBSymbol        string
	BTCFuturesSymbol string
	
	// Documentation URLs
	DocsURL          string
	RateLimitsURL    string
	WebSocketDocsURL string
}

var exchangePresets = map[string]ExchangeConfig{
	"bybit-spot": {
		Exchange:        "Bybit",
		ExchangeName:    "Bybit",
		ExchangeLower:   "bybit",
		ExchangeUpper:   "BYBIT",
		Package:         "bybit",
		MarketType:      "Spot",
		MarketTypeLower: "spot",
		HasFutures:      false,
		RestURL:         "https://api.bybit.com",
		WsPublicURL:     "wss://stream.bybit.com/spot/public/v3",
		WsPrivateURL:    "wss://stream.bybit.com/spot/private/v3",
		RateLimit:       50,
		SymbolFormat:    "BTCUSDT",
		BTCSymbol:       "BTCUSDT",
		ETHSymbol:       "ETHUSDT",
		BNBSymbol:       "BNBUSDT",
		DocsURL:         "https://bybit-exchange.github.io/docs/spot/v3/",
		RateLimitsURL:   "https://bybit-exchange.github.io/docs/spot/v3/#t-ratelimits",
		WebSocketDocsURL: "https://bybit-exchange.github.io/docs/spot/v3/#t-websocket",
	},
	"bybit-futures": {
		Exchange:         "Bybit",
		ExchangeName:     "Bybit",
		ExchangeLower:    "bybit",
		ExchangeUpper:    "BYBIT",
		Package:          "bybit",
		MarketType:       "Futures",
		MarketTypeLower:  "futures",
		HasFutures:       true,
		RestURL:          "https://api.bybit.com",
		WsPublicURL:      "wss://stream.bybit.com/v5/public/linear",
		WsPrivateURL:     "wss://stream.bybit.com/v5/private",
		RateLimit:        50,
		SymbolFormat:     "BTCUSDT",
		BTCSymbol:        "BTCUSDT",
		ETHSymbol:        "ETHUSDT",
		BNBSymbol:        "BNBUSDT",
		BTCFuturesSymbol: "BTCUSDT",
		DocsURL:          "https://bybit-exchange.github.io/docs/v5/intro",
		RateLimitsURL:    "https://bybit-exchange.github.io/docs/v5/rate-limit",
		WebSocketDocsURL: "https://bybit-exchange.github.io/docs/v5/ws/connect",
	},
	"okx-spot": {
		Exchange:         "OKX",
		ExchangeName:     "OKX",
		ExchangeLower:    "okx",
		ExchangeUpper:    "OKX",
		Package:          "okx",
		MarketType:       "Spot",
		MarketTypeLower:  "spot",
		HasFutures:       false,
		HasApiPassphrase: true,
		RestURL:          "https://www.okx.com",
		WsPublicURL:      "wss://ws.okx.com:8443/ws/v5/public",
		WsPrivateURL:     "wss://ws.okx.com:8443/ws/v5/private",
		RateLimit:        20,
		SymbolFormat:     "BTC-USDT",
		BTCSymbol:        "BTC-USDT",
		ETHSymbol:        "ETH-USDT",
		BNBSymbol:        "BNB-USDT",
		DocsURL:          "https://www.okx.com/docs-v5/en/",
		RateLimitsURL:    "https://www.okx.com/docs-v5/en/#rest-api-rate-limit",
		WebSocketDocsURL: "https://www.okx.com/docs-v5/en/#websocket-api",
	},
	"okx-futures": {
		Exchange:         "OKX",
		ExchangeName:     "OKX",
		ExchangeLower:    "okx",
		ExchangeUpper:    "OKX",
		Package:          "okx",
		MarketType:       "Futures",
		MarketTypeLower:  "futures",
		HasFutures:       true,
		HasApiPassphrase: true,
		RestURL:          "https://www.okx.com",
		WsPublicURL:      "wss://ws.okx.com:8443/ws/v5/public",
		WsPrivateURL:     "wss://ws.okx.com:8443/ws/v5/private",
		RateLimit:        20,
		SymbolFormat:     "BTC-USDT-SWAP",
		BTCSymbol:        "BTC-USDT-SWAP",
		ETHSymbol:        "ETH-USDT-SWAP",
		BNBSymbol:        "BNB-USDT-SWAP",
		BTCFuturesSymbol: "BTC-USDT-SWAP",
		DocsURL:          "https://www.okx.com/docs-v5/en/",
		RateLimitsURL:    "https://www.okx.com/docs-v5/en/#rest-api-rate-limit",
		WebSocketDocsURL: "https://www.okx.com/docs-v5/en/#websocket-api",
	},
	"upbit-spot": {
		Exchange:        "Upbit",
		ExchangeName:    "Upbit",
		ExchangeLower:   "upbit",
		ExchangeUpper:   "UPBIT",
		Package:         "upbit",
		MarketType:      "Spot",
		MarketTypeLower: "spot",
		HasFutures:      false,
		RestURL:         "https://api.upbit.com",
		WsPublicURL:     "wss://api.upbit.com/websocket/v1",
		WsPrivateURL:    "wss://api.upbit.com/websocket/v1",
		RateLimit:       10,
		SymbolFormat:    "KRW-BTC",
		BTCSymbol:       "USDT-BTC",
		ETHSymbol:       "USDT-ETH",
		BNBSymbol:       "USDT-BNB",
		DocsURL:         "https://docs.upbit.com/docs/",
		RateLimitsURL:   "https://docs.upbit.com/docs/user-request-guide",
		WebSocketDocsURL: "https://docs.upbit.com/docs/upbit-quotation-websocket",
	},
}

func main() {
	var (
		exchange = flag.String("exchange", "", "Exchange name (e.g., bybit, okx, upbit)")
		market   = flag.String("market", "spot", "Market type (spot or futures)")
		preset   = flag.String("preset", "", "Use preset configuration (e.g., bybit-spot)")
		list     = flag.Bool("list", false, "List available presets")
		output   = flag.String("output", "", "Output directory (default: services/<exchange>)")
	)
	flag.Parse()
	
	if *list {
		fmt.Println("Available presets:")
		for name, config := range exchangePresets {
			fmt.Printf("  %s - %s %s\n", name, config.ExchangeName, config.MarketType)
		}
		return
	}
	
	var config ExchangeConfig
	
	if *preset != "" {
		// Use preset
		var ok bool
		config, ok = exchangePresets[*preset]
		if !ok {
			fmt.Printf("Unknown preset: %s\n", *preset)
			fmt.Println("Use -list to see available presets")
			os.Exit(1)
		}
	} else if *exchange != "" {
		// Build from exchange name
		exchangeLower := strings.ToLower(*exchange)
		presetKey := fmt.Sprintf("%s-%s", exchangeLower, *market)
		
		if presetConfig, ok := exchangePresets[presetKey]; ok {
			config = presetConfig
		} else {
			// Create basic config
			config = ExchangeConfig{
				Exchange:        strings.Title(exchangeLower),
				ExchangeName:    strings.Title(exchangeLower),
				ExchangeLower:   exchangeLower,
				ExchangeUpper:   strings.ToUpper(exchangeLower),
				Package:         exchangeLower,
				MarketType:      strings.Title(*market),
				MarketTypeLower: strings.ToLower(*market),
				HasFutures:      strings.ToLower(*market) == "futures",
				RateLimit:       20,
				SymbolFormat:    "BTCUSDT",
				BTCSymbol:       "BTCUSDT",
				ETHSymbol:       "ETHUSDT",
				BNBSymbol:       "BNBUSDT",
			}
		}
	} else {
		fmt.Println("Usage: generate-exchange -exchange <name> [-market spot|futures]")
		fmt.Println("   or: generate-exchange -preset <preset-name>")
		fmt.Println("   or: generate-exchange -list")
		os.Exit(1)
	}
	
	// Determine output directory
	outputDir := *output
	if outputDir == "" {
		outputDir = filepath.Join("services", config.ExchangeLower)
	}
	
	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("Failed to create output directory: %v\n", err)
		os.Exit(1)
	}
	
	// Generate files
	templates := map[string]string{
		"connector.go":      "templates/exchange/connector.go.tmpl",
		"connector_test.go": "templates/exchange/connector_test.go.tmpl",
		"README.md":         "templates/exchange/README.md.tmpl",
		"config.yaml":       "templates/exchange/config.yaml.tmpl",
	}
	
	for outputFile, templateFile := range templates {
		if err := generateFile(templateFile, filepath.Join(outputDir, outputFile), config); err != nil {
			fmt.Printf("Failed to generate %s: %v\n", outputFile, err)
			os.Exit(1)
		}
		fmt.Printf("Generated: %s\n", filepath.Join(outputDir, outputFile))
	}
	
	// Generate config file in configs directory
	configFile := fmt.Sprintf("configs/%s_%s.yaml", config.ExchangeLower, config.MarketTypeLower)
	if err := generateFile("templates/exchange/config.yaml.tmpl", configFile, config); err != nil {
		fmt.Printf("Failed to generate config file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Generated: %s\n", configFile)
	
	fmt.Printf("\n✅ Successfully generated %s %s connector!\n", config.ExchangeName, config.MarketType)
	fmt.Println("\nNext steps:")
	fmt.Printf("1. Review and customize the generated files in %s/\n", outputDir)
	fmt.Printf("2. Update configs/%s_%s.yaml with actual API endpoints\n", config.ExchangeLower, config.MarketTypeLower)
	fmt.Println("3. Implement exchange-specific logic in the connector")
	fmt.Println("4. Add API credentials to Vault")
	fmt.Println("5. Run tests to verify implementation")
}

func generateFile(templatePath, outputPath string, config ExchangeConfig) error {
	// Read template
	tmplContent, err := ioutil.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read template: %w", err)
	}
	
	// Parse template
	tmpl, err := template.New(filepath.Base(templatePath)).Parse(string(tmplContent))
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}
	
	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, config); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}
	
	// Create output directory if needed
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	
	// Write output file
	if err := ioutil.WriteFile(outputPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	
	return nil
}