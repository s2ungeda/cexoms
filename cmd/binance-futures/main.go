package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	
	"github.com/mExOms/oms/internal/exchange"
	natsClient "github.com/mExOms/oms/pkg/nats"
	"github.com/mExOms/oms/pkg/types"
	"github.com/mExOms/oms/services/binance/futures"
)

func main() {
	// Initialize logger
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetLevel(logrus.InfoLevel)
	
	// Load configuration
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/configs")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath("../../../configs")
	
	if err := viper.ReadInConfig(); err != nil {
		logger.Fatalf("Failed to read config: %v", err)
	}
	
	// Create NATS client
	natsConfig := &natsClient.Config{
		URL:       viper.GetString("nats.url"),
		ClusterID: viper.GetString("nats.cluster_id"),
		ClientID:  "binance-futures-connector",
		Streams: []natsClient.StreamConfig{
			{
				Name:      "ORDERS",
				Subjects:  []string{"orders.>"},
				Retention: nats.LimitsPolicy,
				MaxAge:    24 * time.Hour,
				MaxMsgs:   1000000,
			},
			{
				Name:      "MARKET",
				Subjects:  []string{"market.>"},
				Retention: nats.InterestPolicy,
				MaxAge:    time.Hour,
			},
			{
				Name:      "POSITIONS",
				Subjects:  []string{"positions.>"},
				Retention: nats.LimitsPolicy,
				MaxMsgs:   100000,
			},
		},
	}
	
	nc, err := natsClient.NewClient(natsConfig)
	if err != nil {
		logger.Fatalf("Failed to create NATS client: %v", err)
	}
	defer nc.Close()
	
	// TODO: Load API keys from Vault
	exchangeConfig := &exchange.Config{
		APIKey:      os.Getenv("BINANCE_FUTURES_API_KEY"),
		SecretKey:   os.Getenv("BINANCE_FUTURES_SECRET_KEY"),
		TestNet:     viper.GetBool("exchanges.binance.futures.test_net"),
		APIEndpoint: viper.GetString("exchanges.binance.futures.api_endpoint"),
		WSEndpoint:  viper.GetString("exchanges.binance.futures.ws_endpoint"),
		RateLimits: types.RateLimits{
			WeightPerMinute: viper.GetInt("exchanges.binance.futures.rate_limits.weight_per_minute"),
			OrdersPerSecond: viper.GetInt("exchanges.binance.futures.rate_limits.orders_per_second"),
			OrdersPerDay:    viper.GetInt("exchanges.binance.futures.rate_limits.orders_per_day"),
		},
	}
	
	// Create Binance Futures connector
	connector, err := futures.NewBinanceFutures(exchangeConfig, nc)
	if err != nil {
		logger.Fatalf("Failed to create Binance Futures connector: %v", err)
	}
	
	// Connect to Binance
	ctx := context.Background()
	if err := connector.Connect(ctx); err != nil {
		logger.Fatalf("Failed to connect to Binance Futures: %v", err)
	}
	
	// Subscribe to order commands from NATS
	subscription, err := nc.SubscribeOrders("binance", "futures", "*", func(subject string, data []byte) error {
		// Parse subject to get action
		action, _, _, symbol, err := natsClient.ParseOrderSubject(subject)
		if err != nil {
			return fmt.Errorf("failed to parse subject: %w", err)
		}
		
		// Handle different actions
		switch action {
		case "create":
			var orderMsg natsClient.OrderMessage
			if err := json.Unmarshal(data, &orderMsg); err != nil {
				return fmt.Errorf("failed to unmarshal order: %w", err)
			}
			
			// Create order on Binance Futures
			_, err := connector.CreateOrder(ctx, &orderMsg.Order)
			if err != nil {
				logger.Errorf("Failed to create order: %v", err)
				return err
			}
			
		case "cancel":
			var orderMsg natsClient.OrderMessage
			if err := json.Unmarshal(data, &orderMsg); err != nil {
				return fmt.Errorf("failed to unmarshal order: %w", err)
			}
			
			// Cancel order on Binance Futures
			if err := connector.CancelOrder(ctx, symbol, orderMsg.Order.ID); err != nil {
				logger.Errorf("Failed to cancel order: %v", err)
				return err
			}
		}
		
		return nil
	})
	
	if err != nil {
		logger.Fatalf("Failed to subscribe to orders: %v", err)
	}
	defer subscription.Unsubscribe()
	
	// Subscribe to market data requests
	marketSub, err := nc.Subscribe("market.subscribe.binance.futures.*", func(msg *nats.Msg) {
		// Extract symbol from subject
		parts := strings.Split(msg.Subject, ".")
		if len(parts) < 5 {
			return
		}
		symbol := parts[4]
		
		// Subscribe to market data
		if err := connector.SubscribeMarketData(ctx, []string{symbol}); err != nil {
			logger.Errorf("Failed to subscribe market data for %s: %v", symbol, err)
		}
	})
	
	if err != nil {
		logger.Fatalf("Failed to subscribe to market requests: %v", err)
	}
	defer marketSub.Unsubscribe()
	
	// Subscribe to leverage change requests
	leverageSub, err := nc.Subscribe("futures.leverage.binance.*", func(msg *nats.Msg) {
		// Extract symbol from subject
		parts := strings.Split(msg.Subject, ".")
		if len(parts) < 4 {
			return
		}
		symbol := parts[3]
		
		// Parse leverage value
		var leverage int
		if err := json.Unmarshal(msg.Data, &leverage); err != nil {
			logger.Errorf("Failed to parse leverage: %v", err)
			return
		}
		
		// Set leverage
		if err := connector.SetLeverage(ctx, symbol, leverage); err != nil {
			logger.Errorf("Failed to set leverage for %s: %v", symbol, err)
		}
	})
	
	if err != nil {
		logger.Fatalf("Failed to subscribe to leverage requests: %v", err)
	}
	defer leverageSub.Unsubscribe()
	
	// Periodically update positions
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		
		for range ticker.C {
			if _, err := connector.GetPositions(ctx); err != nil {
				logger.Errorf("Failed to update positions: %v", err)
			}
		}
	}()
	
	logger.Info("Binance Futures connector started")
	
	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	
	logger.Info("Shutting down Binance Futures connector...")
	
	// Disconnect from Binance
	if err := connector.Disconnect(); err != nil {
		logger.Errorf("Failed to disconnect: %v", err)
	}
	
	logger.Info("Binance Futures connector stopped")
}