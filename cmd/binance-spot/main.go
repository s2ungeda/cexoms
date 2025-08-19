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

	// Initialize Binance client (with empty keys for now)
	client := binance.NewClient("", "")
	
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
		}
	}
}