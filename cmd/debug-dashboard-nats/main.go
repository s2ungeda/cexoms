package main

import (
	"encoding/json"
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

	fmt.Println("=== Dashboard NATS Debug ===")
	fmt.Println("Monitoring what the dashboard server receives...\n")

	// Subscribe to the exact patterns the dashboard uses
	patterns := []string{
		"order.event.*",
		"position.update.*",
		"market.data.*",
		"risk.metrics.*",
		"oms.health.*",
		"trade.executed.*",
	}

	for _, pattern := range patterns {
		pat := pattern // capture pattern
		sub, err := nc.Subscribe(pattern, func(msg *nats.Msg) {
			var data interface{}
			json.Unmarshal(msg.Data, &data)
			fmt.Printf("[%s] Subject: %s\n", time.Now().Format("15:04:05"), msg.Subject)
			if pat == "market.data.*" {
				fmt.Printf("  Data: %s\n", string(msg.Data)[:100])
			}
		})
		if err != nil {
			log.Printf("Failed to subscribe to %s: %v", pattern, err)
		} else {
			defer sub.Unsubscribe()
			fmt.Printf("Subscribed to: %s\n", pattern)
		}
	}

	fmt.Println("\nWaiting for messages...")
	time.Sleep(10 * time.Second)
}