package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adshao/go-binance/v2"
)

func main() {
	log.Println("Starting Binance Spot Connector...")

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutdown signal received")
		cancel()
	}()

	// Initialize Binance client with API keys from environment
	apiKey := os.Getenv("BINANCE_API_KEY")
	secretKey := os.Getenv("BINANCE_API_SECRET")
	
	if apiKey == "" || secretKey == "" {
		log.Println("Warning: BINANCE_API_KEY or BINANCE_API_SECRET not set")
	}
	
	client := binance.NewClient(apiKey, secretKey)
	
	// Test connectivity
	err := client.NewPingService().Do(ctx)
	if err != nil {
		log.Printf("Warning: Cannot ping Binance: %v", err)
	} else {
		log.Println("Successfully connected to Binance")
	}

	// Get server time
	serverTime, err := client.NewServerTimeService().Do(ctx)
	if err != nil {
		log.Printf("Warning: Cannot get server time: %v", err)
	} else {
		log.Printf("Binance server time: %v", time.Unix(serverTime/1000, 0))
	}

	// Start WebSocket connections
	log.Println("Starting WebSocket connections...")
	
	// Market data WebSocket
	doneC, stopC, err := binance.WsKlineServe("BTCUSDT", "1m", func(event *binance.WsKlineEvent) {
		// Market data handler - just log for now
	}, func(err error) {
		log.Printf("Market WebSocket error: %v", err)
	})
	if err != nil {
		log.Printf("Failed to start market WebSocket: %v", err)
	} else {
		log.Println("Market data WebSocket connected")
	}
	defer close(stopC)
	
	// User data WebSocket (for orders)
	listenKey, err := client.NewStartUserStreamService().Do(ctx)
	if err != nil {
		log.Printf("Failed to get listen key: %v", err)
	} else {
		log.Printf("Got listen key for user stream")
		
		doneC2, stopC2, err := binance.WsUserDataServe(listenKey, func(event *binance.WsUserDataEvent) {
			// Order update handler
		}, func(err error) {
			log.Printf("User data WebSocket error: %v", err)
		})
		if err != nil {
			log.Printf("Failed to start user data WebSocket: %v", err)
		} else {
			log.Println("User data WebSocket connected")
		}
		defer close(stopC2)
		
		// Keep listen key alive
		go func() {
			ticker := time.NewTicker(30 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					err := client.NewKeepaliveUserStreamService().ListenKey(listenKey).Do(ctx)
					if err != nil {
						log.Printf("Failed to keepalive listen key: %v", err)
					}
				}
			}
		}()
		
		<-doneC2
	}
	
	// Main loop
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled, shutting down...")
			return
		case <-ticker.C:
			// Heartbeat
			log.Println("Binance Spot Connector heartbeat")
		case <-doneC:
			log.Println("Market WebSocket closed")
			return
		}
	}
}