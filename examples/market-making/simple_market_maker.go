package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	pb "github.com/your-org/mExOms/proto/gen/go/oms/v1"
	"google.golang.org/grpc"
)

// SimpleMarketMaker demonstrates a basic market making strategy
type SimpleMarketMaker struct {
	client            pb.OrderServiceClient
	ctx               context.Context
	symbol            string
	exchange          string
	spread            float64 // Percentage spread (e.g., 0.002 = 0.2%)
	orderSize         float64 // Size per order in base currency
	maxOrders         int     // Max orders per side
	minSpread         float64 // Minimum spread in quote currency
	positionLimit     float64 // Maximum position size
	updateInterval    time.Duration
	
	mu                sync.Mutex
	currentPosition   float64
	activeOrders      map[string]*pb.Order
	lastMidPrice      float64
}

func NewSimpleMarketMaker(conn *grpc.ClientConn) *SimpleMarketMaker {
	return &SimpleMarketMaker{
		client:         pb.NewOrderServiceClient(conn),
		ctx:           context.Background(),
		symbol:        "ETHUSDT",
		exchange:      "binance",
		spread:        0.002,    // 0.2% spread
		orderSize:     0.1,      // 0.1 ETH per order
		maxOrders:     3,        // 3 orders per side
		minSpread:     5.0,      // Minimum $5 spread
		positionLimit: 1.0,      // Maximum 1 ETH position
		updateInterval: 5 * time.Second,
		activeOrders:  make(map[string]*pb.Order),
	}
}

func (mm *SimpleMarketMaker) Run() {
	fmt.Printf("Starting Simple Market Maker\n")
	fmt.Printf("Symbol: %s on %s\n", mm.symbol, mm.exchange)
	fmt.Printf("Spread: %.2f%%\n", mm.spread*100)
	fmt.Printf("Order Size: %f\n", mm.orderSize)
	fmt.Printf("Max Orders per Side: %d\n", mm.maxOrders)
	
	// Load current position
	if err := mm.loadPosition(); err != nil {
		log.Fatalf("Failed to load position: %v", err)
	}
	
	// Start market data monitoring
	go mm.monitorMarketData()
	
	// Main loop
	ticker := time.NewTicker(mm.updateInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		mm.updateOrders()
	}
}

func (mm *SimpleMarketMaker) loadPosition() error {
	positions, err := mm.client.GetPositions(mm.ctx, &pb.GetPositionsRequest{
		Exchange: mm.exchange,
		Symbol:   mm.symbol,
	})
	if err != nil {
		return err
	}
	
	mm.mu.Lock()
	defer mm.mu.Unlock()
	
	for _, pos := range positions.Positions {
		if pos.Symbol == mm.symbol {
			mm.currentPosition = pos.Quantity
			fmt.Printf("Loaded position: %f %s\n", mm.currentPosition, mm.symbol)
			break
		}
	}
	
	return nil
}

func (mm *SimpleMarketMaker) monitorMarketData() {
	stream, err := mm.client.StreamTicker(mm.ctx, &pb.StreamTickerRequest{
		Exchange: mm.exchange,
		Symbol:   mm.symbol,
	})
	if err != nil {
		log.Fatalf("Failed to stream ticker: %v", err)
	}
	
	for {
		ticker, err := stream.Recv()
		if err != nil {
			log.Printf("Ticker stream error: %v", err)
			return
		}
		
		mm.mu.Lock()
		mm.lastMidPrice = (ticker.BidPrice + ticker.AskPrice) / 2
		mm.mu.Unlock()
	}
}

func (mm *SimpleMarketMaker) updateOrders() {
	mm.mu.Lock()
	midPrice := mm.lastMidPrice
	position := mm.currentPosition
	mm.mu.Unlock()
	
	if midPrice == 0 {
		fmt.Println("Waiting for market data...")
		return
	}
	
	fmt.Printf("\n--- Update at %s ---\n", time.Now().Format("15:04:05"))
	fmt.Printf("Mid Price: $%.2f\n", midPrice)
	fmt.Printf("Position: %f %s\n", position, mm.symbol)
	
	// Cancel all existing orders
	mm.cancelAllOrders()
	
	// Calculate order prices with position skew
	buyPrices, sellPrices := mm.calculateOrderPrices(midPrice, position)
	
	// Place new orders
	mm.placeOrders(buyPrices, sellPrices)
}

func (mm *SimpleMarketMaker) calculateOrderPrices(midPrice, position float64) ([]float64, []float64) {
	// Apply position skew - if long, skew prices down to encourage selling
	positionSkew := position / mm.positionLimit * 0.001 // 0.1% skew at full position
	
	// Calculate spread with skew
	buySpread := mm.spread + positionSkew
	sellSpread := mm.spread - positionSkew
	
	// Ensure minimum spread
	if midPrice*buySpread < mm.minSpread {
		buySpread = mm.minSpread / midPrice
	}
	if midPrice*sellSpread < mm.minSpread {
		sellSpread = mm.minSpread / midPrice
	}
	
	buyPrices := make([]float64, 0, mm.maxOrders)
	sellPrices := make([]float64, 0, mm.maxOrders)
	
	// Calculate prices for each level
	for i := 0; i < mm.maxOrders; i++ {
		levelMultiplier := 1.0 + float64(i)*0.001 // 0.1% between levels
		
		// Only place buy orders if not at position limit
		if position < mm.positionLimit {
			buyPrice := midPrice * (1 - buySpread*levelMultiplier)
			buyPrices = append(buyPrices, math.Round(buyPrice*100)/100)
		}
		
		// Only place sell orders if we have position
		if position > 0 {
			sellPrice := midPrice * (1 + sellSpread*levelMultiplier)
			sellPrices = append(sellPrices, math.Round(sellPrice*100)/100)
		}
	}
	
	return buyPrices, sellPrices
}

func (mm *SimpleMarketMaker) placeOrders(buyPrices, sellPrices []float64) {
	var wg sync.WaitGroup
	
	// Place buy orders
	for i, price := range buyPrices {
		wg.Add(1)
		go func(level int, buyPrice float64) {
			defer wg.Done()
			
			order := &pb.CreateOrderRequest{
				Symbol:      mm.symbol,
				Exchange:    mm.exchange,
				Market:      "spot",
				Type:        pb.OrderType_LIMIT_MAKER, // Post-only order
				Side:        pb.OrderSide_BUY,
				Quantity:    mm.orderSize,
				Price:       buyPrice,
				TimeInForce: pb.TimeInForce_GTC,
				ClientOrderId: fmt.Sprintf("MM_BUY_%d_%d", level, time.Now().Unix()),
			}
			
			resp, err := mm.client.CreateOrder(mm.ctx, order)
			if err != nil {
				log.Printf("Failed to place buy order: %v", err)
				return
			}
			
			mm.mu.Lock()
			mm.activeOrders[resp.OrderId] = &pb.Order{
				OrderId:  resp.OrderId,
				Symbol:   mm.symbol,
				Side:     pb.OrderSide_BUY,
				Price:    buyPrice,
				Quantity: mm.orderSize,
			}
			mm.mu.Unlock()
			
			fmt.Printf("Buy order placed at $%.2f (ID: %s)\n", buyPrice, resp.OrderId)
		}(i, price)
	}
	
	// Place sell orders
	for i, price := range sellPrices {
		wg.Add(1)
		go func(level int, sellPrice float64) {
			defer wg.Done()
			
			// Calculate order size based on position
			mm.mu.Lock()
			orderQty := math.Min(mm.orderSize, mm.currentPosition)
			mm.mu.Unlock()
			
			if orderQty <= 0 {
				return
			}
			
			order := &pb.CreateOrderRequest{
				Symbol:      mm.symbol,
				Exchange:    mm.exchange,
				Market:      "spot",
				Type:        pb.OrderType_LIMIT_MAKER,
				Side:        pb.OrderSide_SELL,
				Quantity:    orderQty,
				Price:       sellPrice,
				TimeInForce: pb.TimeInForce_GTC,
				ClientOrderId: fmt.Sprintf("MM_SELL_%d_%d", level, time.Now().Unix()),
			}
			
			resp, err := mm.client.CreateOrder(mm.ctx, order)
			if err != nil {
				log.Printf("Failed to place sell order: %v", err)
				return
			}
			
			mm.mu.Lock()
			mm.activeOrders[resp.OrderId] = &pb.Order{
				OrderId:  resp.OrderId,
				Symbol:   mm.symbol,
				Side:     pb.OrderSide_SELL,
				Price:    sellPrice,
				Quantity: orderQty,
			}
			mm.mu.Unlock()
			
			fmt.Printf("Sell order placed at $%.2f (ID: %s)\n", sellPrice, resp.OrderId)
		}(i, price)
	}
	
	wg.Wait()
}

func (mm *SimpleMarketMaker) cancelAllOrders() {
	mm.mu.Lock()
	orderIds := make([]string, 0, len(mm.activeOrders))
	for id := range mm.activeOrders {
		orderIds = append(orderIds, id)
	}
	mm.mu.Unlock()
	
	if len(orderIds) == 0 {
		return
	}
	
	fmt.Printf("Canceling %d orders...\n", len(orderIds))
	
	// Cancel all orders for this symbol
	resp, err := mm.client.CancelAllOrders(mm.ctx, &pb.CancelAllOrdersRequest{
		Exchange: mm.exchange,
		Symbol:   mm.symbol,
	})
	
	if err != nil {
		log.Printf("Failed to cancel orders: %v", err)
		return
	}
	
	mm.mu.Lock()
	mm.activeOrders = make(map[string]*pb.Order)
	mm.mu.Unlock()
	
	fmt.Printf("Cancelled %d orders\n", len(resp.CancelledOrderIds))
}

func (mm *SimpleMarketMaker) monitorFills() {
	// Subscribe to order updates
	stream, err := mm.client.StreamOrders(mm.ctx, &pb.StreamOrdersRequest{
		Exchange: mm.exchange,
		Symbol:   mm.symbol,
	})
	if err != nil {
		log.Fatalf("Failed to stream orders: %v", err)
	}
	
	for {
		update, err := stream.Recv()
		if err != nil {
			log.Printf("Order stream error: %v", err)
			return
		}
		
		if update.Status == pb.OrderStatus_FILLED {
			mm.handleFill(update)
		}
	}
}

func (mm *SimpleMarketMaker) handleFill(order *pb.Order) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	
	// Update position
	if order.Side == pb.OrderSide_BUY {
		mm.currentPosition += order.ExecutedQuantity
		fmt.Printf("🟢 BUY filled: %f @ $%.2f\n", 
			order.ExecutedQuantity, order.ExecutedPrice)
	} else {
		mm.currentPosition -= order.ExecutedQuantity
		fmt.Printf("🔴 SELL filled: %f @ $%.2f\n", 
			order.ExecutedQuantity, order.ExecutedPrice)
	}
	
	// Remove from active orders
	delete(mm.activeOrders, order.OrderId)
	
	// Calculate spread captured
	if order.Side == pb.OrderSide_BUY {
		spreadCaptured := (mm.lastMidPrice - order.ExecutedPrice) / mm.lastMidPrice * 100
		fmt.Printf("Spread captured: %.3f%%\n", spreadCaptured)
	} else {
		spreadCaptured := (order.ExecutedPrice - mm.lastMidPrice) / mm.lastMidPrice * 100
		fmt.Printf("Spread captured: %.3f%%\n", spreadCaptured)
	}
	
	fmt.Printf("Updated position: %f %s\n", mm.currentPosition, mm.symbol)
}

func (mm *SimpleMarketMaker) getStatistics() {
	stats, err := mm.client.GetMarketMakerStats(mm.ctx, 
		&pb.GetMarketMakerStatsRequest{
			Exchange: mm.exchange,
			Symbol:   mm.symbol,
			Period:   "24h",
		})
	if err != nil {
		log.Printf("Failed to get stats: %v", err)
		return
	}
	
	fmt.Println("\n=== Market Maker Statistics (24h) ===")
	fmt.Printf("Total Volume: $%.2f\n", stats.TotalVolume)
	fmt.Printf("Number of Trades: %d\n", stats.NumTrades)
	fmt.Printf("Spread Captured: $%.2f\n", stats.SpreadCaptured)
	fmt.Printf("Average Spread: %.3f%%\n", stats.AverageSpread*100)
	fmt.Printf("Fill Rate: %.1f%%\n", stats.FillRate*100)
	fmt.Printf("PnL: $%.2f\n", stats.RealizedPnl)
}

func main() {
	// Connect to OMS
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to connect to OMS: %v", err)
	}
	defer conn.Close()
	
	// Create market maker
	mm := NewSimpleMarketMaker(conn)
	
	// Start fill monitoring in background
	go mm.monitorFills()
	
	// Periodically show statistics
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		
		for range ticker.C {
			mm.getStatistics()
		}
	}()
	
	// Run the market maker
	mm.Run()
}

// Example output:
// Starting Simple Market Maker
// Symbol: ETHUSDT on binance
// Spread: 0.20%
// Order Size: 0.100000
// Max Orders per Side: 3
// Loaded position: 0.300000 ETHUSDT
//
// --- Update at 14:23:45 ---
// Mid Price: $3250.00
// Position: 0.300000 ETHUSDT
// Canceling 6 orders...
// Cancelled 6 orders
// Buy order placed at $3241.50 (ID: binance-123456)
// Buy order placed at $3238.25 (ID: binance-123457)
// Buy order placed at $3235.01 (ID: binance-123458)
// Sell order placed at $3256.50 (ID: binance-123459)
// Sell order placed at $3259.76 (ID: binance-123460)
// Sell order placed at $3263.01 (ID: binance-123461)
//
// 🟢 BUY filled: 0.100000 @ $3241.50
// Spread captured: 0.262%
// Updated position: 0.400000 ETHUSDT
//
// === Market Maker Statistics (24h) ===
// Total Volume: $125420.50
// Number of Trades: 342
// Spread Captured: $251.84
// Average Spread: 0.201%
// Fill Rate: 68.4%
// PnL: $238.92