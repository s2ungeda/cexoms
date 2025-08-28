package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/nats-io/nats.go"
)

type TickerData struct {
	Symbol    string `json:"symbol"`
	Price     string `json:"price"`
	Volume    string `json:"volume"`
	High      string `json:"high"`
	Low       string `json:"low"`
	Change    string `json:"change"`
	ChangePct string `json:"change_pct"`
}

func main() {
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	fmt.Println("=== Real Binance Market Data Stream ===")
	fmt.Println("Subscribing to market.data.binance.* for real-time updates...\n")

	sub, err := nc.Subscribe("market.data.binance.*", func(msg *nats.Msg) {
		var ticker TickerData
		if err := json.Unmarshal(msg.Data, &ticker); err != nil {
			return
		}
		
		fmt.Printf("%-10s | Price: $%-12s | 24h Change: %6s%% | Volume: %s\n", 
			ticker.Symbol, 
			ticker.Price, 
			ticker.ChangePct,
			ticker.Volume)
	})
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Unsubscribe()

	fmt.Println("Connected! Displaying real-time Binance market data...")
	fmt.Println("Press Ctrl+C to exit\n")

	// Keep running
	select {}
}