package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

func main() {
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	fmt.Println("=== Checking Exact NATS Subjects ===\n")

	subjects := make(map[string]int)
	
	sub, err := nc.Subscribe(">", func(msg *nats.Msg) {
		subjects[msg.Subject]++
		
		// Show market.data subjects in detail
		if strings.Contains(msg.Subject, "market.data") {
			fmt.Printf("Market Data Subject: %s (count: %d)\n", msg.Subject, subjects[msg.Subject])
		}
	})
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Unsubscribe()

	time.Sleep(5 * time.Second)
	
	fmt.Println("\n=== All Subjects Summary ===")
	for subject, count := range subjects {
		fmt.Printf("%s: %d messages\n", subject, count)
	}
}