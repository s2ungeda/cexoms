package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

type Balance struct {
	AccountType    string        `json:"account_type"`
	Exchange       string        `json:"exchange"`
	Balances       []AssetBalance `json:"balances"`
	TotalUSDValue  float64       `json:"total_usd_value"`
	Timestamp      time.Time     `json:"timestamp"`
}

type AssetBalance struct {
	Asset    string    `json:"asset"`
	Free     float64   `json:"free"`
	Locked   float64   `json:"locked"`
	Total    float64   `json:"total"`
	USDValue float64   `json:"usd_value"`
	Timestamp time.Time `json:"timestamp"`
}

func main() {
	nc, err := nats.Connect("nats://127.0.0.1:4222")
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	// Test balance message
	balance := Balance{
		AccountType: "spot",
		Exchange:    "binance",
		TotalUSDValue: 1234.56,
		Timestamp: time.Now(),
		Balances: []AssetBalance{
			{
				Asset:     "BTC",
				Free:      0.1,
				Locked:    0,
				Total:     0.1,
				USDValue:  1200,
				Timestamp: time.Now(),
			},
			{
				Asset:     "USDT",
				Free:      34.56,
				Locked:    0,
				Total:     34.56,
				USDValue:  34.56,
				Timestamp: time.Now(),
			},
		},
	}

	data, err := json.Marshal(balance)
	if err != nil {
		log.Fatal(err)
	}

	// Publish test message
	err = nc.Publish("balance.spot.all", data)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Published test balance message to balance.spot.all")
	nc.Flush()
	time.Sleep(1 * time.Second)
}