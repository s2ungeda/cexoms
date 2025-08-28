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

	fmt.Println("Subscribing to ALL messages for 5 seconds...")
	
	count := 0
	sub, err := nc.Subscribe(">", func(msg *nats.Msg) {
		count++
		fmt.Printf("Subject: %s, Size: %d bytes\n", msg.Subject, len(msg.Data))
		if count < 10 { // Show first 10 messages in detail
			fmt.Printf("Data preview: %s\n", string(msg.Data)[:min(100, len(msg.Data))])
			fmt.Println("---")
		}
	})
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Unsubscribe()

	time.Sleep(5 * time.Second)
	fmt.Printf("\nTotal messages received: %d\n", count)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}