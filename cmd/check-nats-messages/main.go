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

	fmt.Println("Subscribing to market data messages for 10 seconds...")
	
	count := 0
	sub, err := nc.Subscribe("market.data.*", func(msg *nats.Msg) {
		count++
		var data interface{}
		json.Unmarshal(msg.Data, &data)
		fmt.Printf("Subject: %s\n", msg.Subject)
		fmt.Printf("Data: %s\n", string(msg.Data))
		fmt.Println("---")
	})
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Unsubscribe()

	// Also check other subjects
	sub2, _ := nc.Subscribe("order.event.*", func(msg *nats.Msg) {
		fmt.Printf("Order Event - Subject: %s\n", msg.Subject)
	})
	defer sub2.Unsubscribe()

	sub3, _ := nc.Subscribe("position.update.*", func(msg *nats.Msg) {
		fmt.Printf("Position Update - Subject: %s\n", msg.Subject)
	})
	defer sub3.Unsubscribe()

	sub4, _ := nc.Subscribe("oms.health.*", func(msg *nats.Msg) {
		fmt.Printf("Health Update - Subject: %s\n", msg.Subject)
	})
	defer sub4.Unsubscribe()

	time.Sleep(10 * time.Second)
	fmt.Printf("\nReceived %d market data messages\n", count)
}