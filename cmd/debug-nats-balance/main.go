package main

import (
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

func main() {
	nc, err := nats.Connect("nats://127.0.0.1:4222")
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	// Subscribe to balance messages
	_, err = nc.Subscribe("balance.spot.all", func(msg *nats.Msg) {
		fmt.Printf("Received balance message:\n")
		fmt.Printf("Subject: %s\n", msg.Subject)
		fmt.Printf("Data: %s\n\n", string(msg.Data))
	})
	if err != nil {
		log.Fatal(err)
	}

	// Subscribe to futures position messages
	_, err = nc.Subscribe("position.futures.all", func(msg *nats.Msg) {
		fmt.Printf("Received futures position message:\n")
		fmt.Printf("Subject: %s\n", msg.Subject)
		fmt.Printf("Data: %s\n\n", string(msg.Data))
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Listening for balance and position messages...")
	time.Sleep(15 * time.Second)
}