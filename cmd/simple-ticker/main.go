package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/adshao/go-binance/v2"
	"github.com/adshao/go-binance/v2/futures"
)

func main() {
	log.Println("Starting Simple Ticker Service...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutdown signal received")
		cancel()
	}()

	symbols := []string{"BTCUSDT", "ETHUSDT", "XRPUSDT"}

	// Create clients
	spotClient := binance.NewClient("", "")
	futuresClient := futures.NewClient("", "")

	// Ticker to fetch prices every second
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Clear screen
			fmt.Print("\033[H\033[2J")
			fmt.Println("=== Binance Real-time Prices ===")
			fmt.Printf("Time: %s\n\n", time.Now().Format("15:04:05"))
			
			fmt.Printf("%-10s %-8s %-12s %-8s %-12s %-12s %-8s\n", 
				"Symbol", "Market", "Price", "Change%", "Volume", "High", "Low")
			fmt.Println(string(make([]byte, 85, 85)))

			// Fetch spot prices
			for _, symbol := range symbols {
				// Get 24hr ticker stats
				stats, err := spotClient.NewListPriceChangeStatsService().Symbol(symbol).Do(ctx)
				if err != nil {
					log.Printf("Error fetching spot stats for %s: %v", symbol, err)
					continue
				}
				
				if len(stats) > 0 {
					stat := stats[0]
					price, _ := strconv.ParseFloat(stat.LastPrice, 64)
					change, _ := strconv.ParseFloat(stat.PriceChangePercent, 64)
					volume, _ := strconv.ParseFloat(stat.Volume, 64)
					high, _ := strconv.ParseFloat(stat.HighPrice, 64)
					low, _ := strconv.ParseFloat(stat.LowPrice, 64)
					
					fmt.Printf("%-10s %-8s $%-11.2f %-7.2f%% %-12.0f $%-11.2f $%-8.2f\n",
						symbol, "SPOT", price, change, volume, high, low)
				}
			}
			
			fmt.Println()
			
			// Fetch futures prices  
			for _, symbol := range symbols {
				stats, err := futuresClient.NewListPriceChangeStatsService().Symbol(symbol).Do(ctx)
				if err != nil {
					log.Printf("Error fetching futures stats for %s: %v", symbol, err)
					continue
				}
				
				if len(stats) > 0 {
					stat := stats[0]
					price, _ := strconv.ParseFloat(stat.LastPrice, 64)
					change, _ := strconv.ParseFloat(stat.PriceChangePercent, 64)
					volume, _ := strconv.ParseFloat(stat.Volume, 64)
					high, _ := strconv.ParseFloat(stat.HighPrice, 64)
					low, _ := strconv.ParseFloat(stat.LowPrice, 64)
					
					fmt.Printf("%-10s %-8s $%-11.2f %-7.2f%% %-12.0f $%-11.2f $%-8.2f\n",
						symbol, "FUTURES", price, change, volume, high, low)
				}
			}
			
			// Calculate premiums
			fmt.Println("\n=== Spot-Futures Premium ===")
			for _, symbol := range symbols {
				spotPrice := getSpotPrice(ctx, spotClient, symbol)
				futuresPrice := getFuturesPrice(ctx, futuresClient, symbol)
				
				if spotPrice > 0 && futuresPrice > 0 {
					premium := ((futuresPrice - spotPrice) / spotPrice) * 100
					fmt.Printf("%s: %.3f%% (Spot: $%.2f, Futures: $%.2f)\n", 
						symbol, premium, spotPrice, futuresPrice)
				}
			}
			
			fmt.Println("\nPress Ctrl+C to exit")
		}
	}
}

func getSpotPrice(ctx context.Context, client *binance.Client, symbol string) float64 {
	prices, err := client.NewListPricesService().Symbol(symbol).Do(ctx)
	if err != nil || len(prices) == 0 {
		return 0
	}
	price, _ := strconv.ParseFloat(prices[0].Price, 64)
	return price
}

func getFuturesPrice(ctx context.Context, client *futures.Client, symbol string) float64 {
	prices, err := client.NewListPricesService().Symbol(symbol).Do(ctx)
	if err != nil || len(prices) == 0 {
		return 0
	}
	price, _ := strconv.ParseFloat(prices[0].Price, 64)
	return price
}