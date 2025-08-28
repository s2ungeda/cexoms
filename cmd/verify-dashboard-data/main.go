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

	fmt.Println("=== Verifying Dashboard Data Flow ===\n")

	// Monitor what dashboard server sees
	marketCount := 0
	sub, err := nc.Subscribe("market.data.>", func(msg *nats.Msg) {
		marketCount++
		if marketCount <= 5 {
			var data map[string]interface{}
			json.Unmarshal(msg.Data, &data)
			fmt.Printf("Market Data - Symbol: %s, Price: %v, Change: %v%%\n", 
				data["symbol"], data["price"], data["change_pct"])
		}
	})
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Unsubscribe()

	// Also publish a test message to see if dashboard responds
	time.Sleep(2 * time.Second)
	
	testData := map[string]interface{}{
		"symbol":     "TEST_SYMBOL",
		"price":      "12345.67",
		"change_pct": "5.55",
		"volume":     "1000000",
	}
	
	data, _ := json.Marshal(testData)
	nc.Publish("market.data.test.TEST", data)
	fmt.Println("\nPublished test market data")
	
	time.Sleep(5 * time.Second)
	fmt.Printf("\nTotal market messages received: %d\n", marketCount)
}