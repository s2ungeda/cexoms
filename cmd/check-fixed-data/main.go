package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/nats-io/nats.go"
)

func main() {
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	fmt.Println("=== Checking Fixed Market Data Format ===\n")

	count := 0
	sub, err := nc.Subscribe("market.data.binance.BTCUSDT", func(msg *nats.Msg) {
		count++
		if count <= 3 {
			var data map[string]interface{}
			json.Unmarshal(msg.Data, &data)
			
			fmt.Printf("Raw JSON: %s\n", string(msg.Data))
			fmt.Printf("Parsed data:\n")
			fmt.Printf("  Symbol: %v (type: %T)\n", data["symbol"], data["symbol"])
			fmt.Printf("  Price: %v (type: %T)\n", data["price"], data["price"])
			fmt.Printf("  ChangePct: %v (type: %T)\n", data["change_pct"], data["change_pct"])
			fmt.Println("---")
		}
	})
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Unsubscribe()

	// Wait and unsubscribe after checking
	select {}
}