package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"time"

	"github.com/nats-io/nats.go"
)

// OrderUpdate represents an order update message
type OrderUpdate struct {
	OrderID        string    `json:"orderId"`
	Symbol         string    `json:"symbol"`
	Side           string    `json:"side"`
	Type           string    `json:"type"`
	Quantity       float64   `json:"quantity"`
	Price          float64   `json:"price"`
	FilledQuantity float64   `json:"filledQuantity"`
	AvgPrice       float64   `json:"avgPrice"`
	Status         string    `json:"status"`
	Timestamp      time.Time `json:"timestamp"`
	Exchange       string    `json:"exchange"`
}

// PositionUpdate represents a position update message
type PositionUpdate struct {
	Symbol        string  `json:"symbol"`
	Quantity      float64 `json:"quantity"`
	Side          string  `json:"side"`
	AvgPrice      float64 `json:"avgPrice"`
	CurrentPrice  float64 `json:"currentPrice"`
	Value         float64 `json:"value"`
	UnrealizedPnL float64 `json:"unrealizedPnL"`
	RealizedPnL   float64 `json:"realizedPnL"`
	PnLPercent    float64 `json:"pnlPercent"`
	Exchange      string  `json:"exchange"`
}

// MarketUpdate represents a market data update
type MarketUpdate struct {
	Symbol   string  `json:"symbol"`
	Price    float64 `json:"price"`
	Bid      float64 `json:"bid"`
	Ask      float64 `json:"ask"`
	Volume   float64 `json:"volume"`
	High24h  float64 `json:"high24h"`
	Low24h   float64 `json:"low24h"`
	Change24h float64 `json:"change24h"`
}

// SystemMetrics represents system metrics
type SystemMetrics struct {
	CPU               float64 `json:"cpu"`
	Memory            int     `json:"memory"`
	UsedMemory        int     `json:"usedMemory"`
	Latency           float64 `json:"latency"`
	OrdersPerSecond   int     `json:"ordersPerSecond"`
	ActiveConnections int     `json:"activeConnections"`
}

// RiskMetrics represents risk metrics
type RiskMetrics struct {
	Metrics struct {
		PortfolioVaR    float64 `json:"portfolioVaR"`
		CurrentDrawdown float64 `json:"currentDrawdown"`
		Leverage        float64 `json:"leverage"`
		MarginUsage     float64 `json:"marginUsage"`
	} `json:"metrics"`
}

func main() {
	// Connect to NATS
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatal("Failed to connect to NATS:", err)
	}
	defer nc.Close()

	log.Println("Connected to NATS. Starting data generation...")

	// Initialize random seed
	rand.Seed(time.Now().UnixNano())

	// Start data generators
	go generateOrders(nc)
	go generatePositions(nc)
	go generateMarketData(nc)
	go generateSystemMetrics(nc)
	go generateRiskMetrics(nc)

	// Keep running
	select {}
}

func generateOrders(nc *nats.Conn) {
	symbols := []string{"BTCUSDT", "ETHUSDT", "BNBUSDT"}
	statuses := []string{"NEW", "PARTIALLY_FILLED", "FILLED", "CANCELLED"}
	orderTypes := []string{"MARKET", "LIMIT", "STOP_LOSS"}
	
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	orderID := 1000

	for range ticker.C {
		// Generate random order update
		order := OrderUpdate{
			OrderID:   fmt.Sprintf("ORD-%d", orderID),
			Symbol:    symbols[rand.Intn(len(symbols))],
			Side:      randomSide(),
			Type:      orderTypes[rand.Intn(len(orderTypes))],
			Quantity:  roundFloat(rand.Float64() * 2, 4),
			Price:     getRandomPrice(symbols[rand.Intn(len(symbols))]),
			Status:    statuses[rand.Intn(len(statuses))],
			Timestamp: time.Now(),
			Exchange:  "BINANCE",
		}

		// Set filled quantity based on status
		if order.Status == "FILLED" {
			order.FilledQuantity = order.Quantity
			order.AvgPrice = order.Price + (rand.Float64()-0.5)*10
		} else if order.Status == "PARTIALLY_FILLED" {
			order.FilledQuantity = order.Quantity * rand.Float64()
			order.AvgPrice = order.Price + (rand.Float64()-0.5)*5
		}

		data, _ := json.Marshal(order)
		nc.Publish("orders.update", data)
		log.Printf("Published order: %s %s %s %.4f @ %.2f - %s",
			order.OrderID, order.Symbol, order.Side, order.Quantity, order.Price, order.Status)

		orderID++
	}
}

func generatePositions(nc *nats.Conn) {
	positions := []PositionUpdate{
		{Symbol: "BTCUSDT", Quantity: 0.5, Side: "LONG", AvgPrice: 49000, Exchange: "BINANCE"},
		{Symbol: "ETHUSDT", Quantity: 5.0, Side: "LONG", AvgPrice: 3100, Exchange: "BINANCE"},
		{Symbol: "BNBUSDT", Quantity: 10.0, Side: "SHORT", AvgPrice: 320, Exchange: "BINANCE"},
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Update positions with current prices
		for i := range positions {
			pos := &positions[i]
			pos.CurrentPrice = getRandomPrice(pos.Symbol)
			
			if pos.Side == "LONG" {
				pos.UnrealizedPnL = (pos.CurrentPrice - pos.AvgPrice) * pos.Quantity
			} else {
				pos.UnrealizedPnL = (pos.AvgPrice - pos.CurrentPrice) * pos.Quantity
			}
			
			pos.Value = pos.CurrentPrice * pos.Quantity
			pos.PnLPercent = (pos.UnrealizedPnL / (pos.AvgPrice * pos.Quantity)) * 100
			pos.RealizedPnL = rand.Float64() * 100 - 50

			data, _ := json.Marshal(pos)
			nc.Publish("positions.update", data)
		}

		log.Println("Published position updates")
	}
}

func generateMarketData(nc *nats.Conn) {
	symbols := map[string]float64{
		"BTCUSDT": 50000,
		"ETHUSDT": 3000,
		"BNBUSDT": 320,
		"ADAUSDT": 1.2,
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		for symbol, basePrice := range symbols {
			// Generate price movement
			change := (rand.Float64() - 0.5) * basePrice * 0.001
			newPrice := basePrice + change
			symbols[symbol] = newPrice

			update := MarketUpdate{
				Symbol:    symbol,
				Price:     newPrice,
				Bid:       newPrice - 5,
				Ask:       newPrice + 5,
				Volume:    rand.Float64() * 1000000,
				High24h:   newPrice * 1.02,
				Low24h:    newPrice * 0.98,
				Change24h: (rand.Float64() - 0.5) * 5,
			}

			data, _ := json.Marshal(update)
			nc.Publish("market.data", data)
		}
	}
}

func generateSystemMetrics(nc *nats.Conn) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	baseCPU := 45.0
	baseMemory := 1024

	for range ticker.C {
		metrics := SystemMetrics{
			CPU:               math.Max(0, math.Min(100, baseCPU+(rand.Float64()-0.5)*20)),
			Memory:            2048,
			UsedMemory:        baseMemory + rand.Intn(500),
			Latency:           0.5 + rand.Float64()*2,
			OrdersPerSecond:   100 + rand.Intn(100),
			ActiveConnections: 10 + rand.Intn(20),
		}

		data, _ := json.Marshal(metrics)
		nc.Publish("system.metrics", data)
	}
}

func generateRiskMetrics(nc *nats.Conn) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		risk := RiskMetrics{}
		risk.Metrics.PortfolioVaR = 2000 + rand.Float64()*1000
		risk.Metrics.CurrentDrawdown = rand.Float64() * 10
		risk.Metrics.Leverage = 1 + rand.Float64()*2
		risk.Metrics.MarginUsage = rand.Float64() * 100

		data, _ := json.Marshal(risk)
		nc.Publish("risk.update", data)

		log.Println("Published risk metrics")
	}
}

func randomSide() string {
	if rand.Float64() > 0.5 {
		return "BUY"
	}
	return "SELL"
}

func getRandomPrice(symbol string) float64 {
	basePrices := map[string]float64{
		"BTCUSDT": 50000,
		"ETHUSDT": 3000,
		"BNBUSDT": 320,
		"ADAUSDT": 1.2,
	}
	
	base, exists := basePrices[symbol]
	if !exists {
		base = 100
	}
	
	return base + (rand.Float64()-0.5)*base*0.01
}

func roundFloat(val float64, precision int) float64 {
	pow := math.Pow(10, float64(precision))
	return math.Round(val*pow) / pow
}