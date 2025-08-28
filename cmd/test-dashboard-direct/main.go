package main

import (
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

func main() {
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	fmt.Println("=== Testing Dashboard NATS Handler ===")
	
	// Try subscribing with exact wildcard pattern
	sub, err := nc.Subscribe("market.data.binance.BTCUSDT", func(msg *nats.Msg) {
		fmt.Printf("Received on %s: %s\n", msg.Subject, string(msg.Data)[:100])
	})
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Unsubscribe()

	// Also try the wildcard
	sub2, err := nc.Subscribe("market.data.*", func(msg *nats.Msg) {
		fmt.Printf("Wildcard received on %s\n", msg.Subject)
	})
	if err != nil {
		log.Fatal(err)
	}
	defer sub2.Unsubscribe()

	// Try multi-level wildcard
	sub3, err := nc.Subscribe("market.data.>", func(msg *nats.Msg) {
		fmt.Printf("Multi-wildcard received on %s\n", msg.Subject)
	})
	if err != nil {
		log.Fatal(err)
	}
	defer sub3.Unsubscribe()

	fmt.Println("\nWaiting for market data...")
	time.Sleep(10 * time.Second)
}