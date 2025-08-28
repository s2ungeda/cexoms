package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/gorilla/websocket"
)

func main() {
	// Connect to dashboard WebSocket
	url := "ws://localhost:8080/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatal("WebSocket dial error:", err)
	}
	defer conn.Close()

	fmt.Println("Connected to dashboard WebSocket")

	// Subscribe to market channel
	subscribe := map[string]interface{}{
		"type":     "subscribe",
		"channels": []string{"market"},
	}
	if err := conn.WriteJSON(subscribe); err != nil {
		log.Fatal("Subscribe error:", err)
	}

	fmt.Println("Subscribed to market channel")

	// Read messages
	for {
		var msg map[string]interface{}
		err := conn.ReadJSON(&msg)
		if err != nil {
			log.Println("Read error:", err)
			break
		}

		if msg["type"] == "market_update" {
			// Parse the nested data
			if dataStr, ok := msg["data"].(string); ok {
				var marketData map[string]interface{}
				if err := json.Unmarshal([]byte(dataStr), &marketData); err == nil {
					fmt.Printf("\nMarket Update for %s:\n", marketData["symbol"])
					fmt.Printf("  Price: %v (type: %T)\n", marketData["price"], marketData["price"])
					fmt.Printf("  Full data: %s\n", dataStr[:100])
				}
			}
		} else {
			fmt.Printf("Received: %v\n", msg["type"])
		}
	}
}