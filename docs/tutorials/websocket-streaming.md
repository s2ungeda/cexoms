# WebSocket Streaming Guide

## Overview

mExOms provides real-time data streaming through WebSocket connections for market data, order updates, and account changes. This guide covers how to connect and consume these streams.

## Prerequisites

- Completed [Basic Trading Tutorial](./basic-trading.md)
- Understanding of WebSocket protocol
- Go channels and goroutines knowledge

## Why WebSocket Streaming?

- **Real-time Updates**: Instant market data without polling
- **Lower Latency**: Sub-millisecond updates
- **Efficient**: Less bandwidth than REST polling
- **Event-Driven**: React immediately to market changes

## Part 1: Market Data Streaming

### Ticker Stream

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"
    
    pb "github.com/your-org/mExOms/proto/gen/go/oms/v1"
    "google.golang.org/grpc"
)

func streamTicker(client pb.MarketDataServiceClient, ctx context.Context) {
    // Subscribe to ticker updates
    stream, err := client.StreamTicker(ctx, &pb.StreamTickerRequest{
        Exchange: "binance",
        Symbols:  []string{"BTCUSDT", "ETHUSDT"},
    })
    if err != nil {
        log.Fatalf("Failed to start ticker stream: %v", err)
    }
    
    fmt.Println("Streaming ticker data...")
    
    for {
        ticker, err := stream.Recv()
        if err != nil {
            log.Printf("Stream error: %v", err)
            break
        }
        
        fmt.Printf("[%s] %s - Price: $%.2f, 24h Change: %.2f%%\n",
            time.Now().Format("15:04:05"),
            ticker.Symbol,
            ticker.LastPrice,
            ticker.PriceChangePercent)
    }
}
```

### Order Book Stream

```go
func streamOrderBook(client pb.MarketDataServiceClient, ctx context.Context) {
    // Subscribe to order book updates
    stream, err := client.StreamOrderBook(ctx, &pb.StreamOrderBookRequest{
        Exchange: "binance",
        Symbol:   "BTCUSDT",
        Depth:    10, // Top 10 levels
    })
    if err != nil {
        log.Fatalf("Failed to start order book stream: %v", err)
    }
    
    fmt.Println("Streaming order book...")
    
    for {
        book, err := stream.Recv()
        if err != nil {
            log.Printf("Stream error: %v", err)
            break
        }
        
        fmt.Printf("\n[%s] Order Book Update:\n", 
            time.Now().Format("15:04:05"))
        
        // Display top 3 levels
        fmt.Println("BIDS:")
        for i := 0; i < 3 && i < len(book.Bids); i++ {
            fmt.Printf("  %.2f @ %.4f\n", 
                book.Bids[i].Price, book.Bids[i].Quantity)
        }
        
        fmt.Println("ASKS:")
        for i := 0; i < 3 && i < len(book.Asks); i++ {
            fmt.Printf("  %.2f @ %.4f\n", 
                book.Asks[i].Price, book.Asks[i].Quantity)
        }
        
        // Calculate spread
        if len(book.Bids) > 0 && len(book.Asks) > 0 {
            spread := book.Asks[0].Price - book.Bids[0].Price
            spreadPct := (spread / book.Bids[0].Price) * 100
            fmt.Printf("Spread: $%.2f (%.3f%%)\n", spread, spreadPct)
        }
    }
}
```

### Trade Stream

```go
func streamTrades(client pb.MarketDataServiceClient, ctx context.Context) {
    // Subscribe to real-time trades
    stream, err := client.StreamTrades(ctx, &pb.StreamTradesRequest{
        Exchange: "binance",
        Symbol:   "BTCUSDT",
    })
    if err != nil {
        log.Fatalf("Failed to start trade stream: %v", err)
    }
    
    fmt.Println("Streaming trades...")
    
    var volumeBuy, volumeSell float64
    startTime := time.Now()
    
    for {
        trade, err := stream.Recv()
        if err != nil {
            log.Printf("Stream error: %v", err)
            break
        }
        
        // Track volume
        if trade.IsBuyerMaker {
            volumeSell += trade.Quantity
        } else {
            volumeBuy += trade.Quantity
        }
        
        // Display large trades only
        if trade.Quantity*trade.Price > 10000 { // > $10k trades
            side := "BUY "
            if trade.IsBuyerMaker {
                side = "SELL"
            }
            
            fmt.Printf("[%s] 🐋 Large %s: %.4f @ $%.2f = $%.0f\n",
                trade.Timestamp.Format("15:04:05"),
                side,
                trade.Quantity,
                trade.Price,
                trade.Quantity*trade.Price)
        }
        
        // Print volume summary every 30 seconds
        if time.Since(startTime) > 30*time.Second {
            fmt.Printf("\n📊 30s Volume - Buy: %.2f BTC, Sell: %.2f BTC\n\n",
                volumeBuy, volumeSell)
            volumeBuy, volumeSell = 0, 0
            startTime = time.Now()
        }
    }
}
```

## Part 2: Order and Account Streams

### Order Update Stream

```go
func streamOrders(client pb.OrderServiceClient, ctx context.Context) {
    // Subscribe to all order updates
    stream, err := client.StreamOrders(ctx, &pb.StreamOrdersRequest{
        Exchange: "binance",
        // Leave Symbol empty to get all symbols
    })
    if err != nil {
        log.Fatalf("Failed to start order stream: %v", err)
    }
    
    fmt.Println("Streaming order updates...")
    
    for {
        update, err := stream.Recv()
        if err != nil {
            log.Printf("Stream error: %v", err)
            break
        }
        
        // Format update based on type
        var emoji string
        switch update.Status {
        case pb.OrderStatus_NEW:
            emoji = "📝"
        case pb.OrderStatus_PARTIALLY_FILLED:
            emoji = "⚡"
        case pb.OrderStatus_FILLED:
            emoji = "✅"
        case pb.OrderStatus_CANCELLED:
            emoji = "❌"
        case pb.OrderStatus_REJECTED:
            emoji = "🚫"
        default:
            emoji = "ℹ️"
        }
        
        fmt.Printf("%s [%s] Order %s: %s %s %.4f @ $%.2f - Status: %s\n",
            emoji,
            update.Timestamp.Format("15:04:05"),
            update.OrderId[:8],
            update.Side,
            update.Symbol,
            update.Quantity,
            update.Price,
            update.Status)
        
        // Show fill details
        if update.ExecutedQuantity > 0 {
            fmt.Printf("   Filled: %.4f @ $%.2f (%.1f%%)\n",
                update.ExecutedQuantity,
                update.ExecutedPrice,
                (update.ExecutedQuantity/update.Quantity)*100)
        }
    }
}
```

### Balance Update Stream

```go
func streamBalances(client pb.AccountServiceClient, ctx context.Context) {
    // Subscribe to balance changes
    stream, err := client.StreamBalances(ctx, &pb.StreamBalancesRequest{
        Exchange: "binance",
    })
    if err != nil {
        log.Fatalf("Failed to start balance stream: %v", err)
    }
    
    fmt.Println("Streaming balance updates...")
    
    balances := make(map[string]*pb.Balance)
    
    for {
        update, err := stream.Recv()
        if err != nil {
            log.Printf("Stream error: %v", err)
            break
        }
        
        // Track balance changes
        oldBalance, exists := balances[update.Asset]
        balances[update.Asset] = update
        
        if exists {
            change := update.Free - oldBalance.Free
            if change != 0 {
                emoji := "📈"
                if change < 0 {
                    emoji = "📉"
                }
                
                fmt.Printf("%s [%s] %s Balance Change: %.6f (%.6f → %.6f)\n",
                    emoji,
                    time.Now().Format("15:04:05"),
                    update.Asset,
                    change,
                    oldBalance.Free,
                    update.Free)
            }
        }
    }
}
```

## Part 3: Advanced Streaming Patterns

### Multi-Exchange Aggregated Stream

```go
type AggregatedTicker struct {
    Symbol      string
    BestBid     float64
    BestBidExch string
    BestAsk     float64
    BestAskExch string
    Timestamp   time.Time
}

func aggregateMultiExchange(client pb.MarketDataServiceClient, 
    ctx context.Context, symbol string) {
    
    exchanges := []string{"binance", "okx", "bybit"}
    tickerChan := make(chan *pb.Ticker, len(exchanges))
    
    // Start streams for each exchange
    for _, exchange := range exchanges {
        go func(exch string) {
            stream, err := client.StreamTicker(ctx, &pb.StreamTickerRequest{
                Exchange: exch,
                Symbols:  []string{symbol},
            })
            if err != nil {
                log.Printf("Failed to stream from %s: %v", exch, err)
                return
            }
            
            for {
                ticker, err := stream.Recv()
                if err != nil {
                    log.Printf("Stream error on %s: %v", exch, err)
                    return
                }
                ticker.Exchange = exch // Tag with exchange
                tickerChan <- ticker
            }
        }(exchange)
    }
    
    // Aggregate best prices
    tickers := make(map[string]*pb.Ticker)
    
    for ticker := range tickerChan {
        tickers[ticker.Exchange] = ticker
        
        // Find best bid and ask across exchanges
        var bestBid, bestAsk *pb.Ticker
        
        for _, t := range tickers {
            if bestBid == nil || t.BidPrice > bestBid.BidPrice {
                bestBid = t
            }
            if bestAsk == nil || t.AskPrice < bestAsk.AskPrice {
                bestAsk = t
            }
        }
        
        if bestBid != nil && bestAsk != nil {
            fmt.Printf("[%s] Best Bid: $%.2f (%s) | Best Ask: $%.2f (%s) | Spread: $%.2f\n",
                time.Now().Format("15:04:05"),
                bestBid.BidPrice,
                bestBid.Exchange,
                bestAsk.AskPrice,
                bestAsk.Exchange,
                bestAsk.AskPrice - bestBid.BidPrice)
        }
    }
}
```

### Stream with Reconnection

```go
func streamWithReconnection(client pb.MarketDataServiceClient, 
    ctx context.Context) {
    
    maxRetries := 5
    retryDelay := time.Second
    
    for retry := 0; retry < maxRetries; retry++ {
        if retry > 0 {
            fmt.Printf("Reconnecting... attempt %d/%d\n", retry+1, maxRetries)
            time.Sleep(retryDelay)
            retryDelay *= 2 // Exponential backoff
        }
        
        stream, err := client.StreamTicker(ctx, &pb.StreamTickerRequest{
            Exchange: "binance",
            Symbols:  []string{"BTCUSDT"},
        })
        if err != nil {
            log.Printf("Failed to connect: %v", err)
            continue
        }
        
        // Reset retry counter on successful connection
        retry = 0
        retryDelay = time.Second
        
        for {
            ticker, err := stream.Recv()
            if err != nil {
                log.Printf("Stream disconnected: %v", err)
                break // Will retry connection
            }
            
            // Process ticker
            fmt.Printf("BTC Price: $%.2f\n", ticker.LastPrice)
        }
    }
    
    log.Fatal("Max retries exceeded")
}
```

### Stream Processing Pipeline

```go
func streamProcessingPipeline(client pb.MarketDataServiceClient, 
    ctx context.Context) {
    
    // Stage 1: Raw data ingestion
    rawChan := make(chan *pb.Trade, 1000)
    
    // Stage 2: Filtering
    filteredChan := make(chan *pb.Trade, 100)
    
    // Stage 3: Aggregation
    aggregatedChan := make(chan *VolumeBar, 10)
    
    // Start pipeline stages
    go ingestTrades(client, ctx, rawChan)
    go filterLargeTrades(rawChan, filteredChan)
    go aggregateVolumeBars(filteredChan, aggregatedChan)
    go processVolumeBars(aggregatedChan)
    
    // Wait for context cancellation
    <-ctx.Done()
}

func ingestTrades(client pb.MarketDataServiceClient, 
    ctx context.Context, out chan<- *pb.Trade) {
    
    stream, _ := client.StreamTrades(ctx, &pb.StreamTradesRequest{
        Exchange: "binance",
        Symbol:   "BTCUSDT",
    })
    
    for {
        trade, err := stream.Recv()
        if err != nil {
            close(out)
            return
        }
        
        select {
        case out <- trade:
        case <-ctx.Done():
            return
        }
    }
}

func filterLargeTrades(in <-chan *pb.Trade, out chan<- *pb.Trade) {
    for trade := range in {
        // Filter trades > $5000
        if trade.Quantity * trade.Price > 5000 {
            out <- trade
        }
    }
    close(out)
}

type VolumeBar struct {
    StartTime   time.Time
    EndTime     time.Time
    Volume      float64
    BuyVolume   float64
    SellVolume  float64
    TradeCount  int
    VWAP        float64
}

func aggregateVolumeBars(in <-chan *pb.Trade, out chan<- *VolumeBar) {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()
    
    currentBar := &VolumeBar{StartTime: time.Now()}
    var totalValue float64
    
    for {
        select {
        case trade := <-in:
            if trade == nil {
                close(out)
                return
            }
            
            currentBar.Volume += trade.Quantity
            if trade.IsBuyerMaker {
                currentBar.SellVolume += trade.Quantity
            } else {
                currentBar.BuyVolume += trade.Quantity
            }
            currentBar.TradeCount++
            totalValue += trade.Quantity * trade.Price
            
        case <-ticker.C:
            // Complete current bar
            currentBar.EndTime = time.Now()
            if currentBar.Volume > 0 {
                currentBar.VWAP = totalValue / currentBar.Volume
                out <- currentBar
            }
            
            // Start new bar
            currentBar = &VolumeBar{StartTime: time.Now()}
            totalValue = 0
        }
    }
}
```

## Part 4: Performance Optimization

### Buffered Processing

```go
func optimizedStreaming(client pb.MarketDataServiceClient, ctx context.Context) {
    // Use buffered channel for better performance
    buffer := 1000
    tradeChan := make(chan *pb.Trade, buffer)
    
    // Multiple workers for processing
    numWorkers := 4
    var wg sync.WaitGroup
    
    // Start workers
    for i := 0; i < numWorkers; i++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()
            processTrades(workerID, tradeChan)
        }(i)
    }
    
    // Stream trades
    stream, _ := client.StreamTrades(ctx, &pb.StreamTradesRequest{
        Exchange: "binance",
        Symbol:   "BTCUSDT",
    })
    
    for {
        trade, err := stream.Recv()
        if err != nil {
            close(tradeChan)
            break
        }
        
        select {
        case tradeChan <- trade:
        default:
            // Channel full, log dropped message
            log.Println("Warning: Trade buffer full, dropping message")
        }
    }
    
    wg.Wait()
}

func processTrades(workerID int, trades <-chan *pb.Trade) {
    for trade := range trades {
        // Process trade
        // Each worker handles trades independently
        fmt.Printf("[Worker %d] Processing trade: %.4f @ %.2f\n",
            workerID, trade.Quantity, trade.Price)
    }
}
```

### Rate Limiting and Throttling

```go
func throttledStreaming(client pb.MarketDataServiceClient, ctx context.Context) {
    // Limit processing rate to prevent overload
    limiter := rate.NewLimiter(rate.Every(100*time.Millisecond), 1) // 10 msg/sec
    
    stream, _ := client.StreamTicker(ctx, &pb.StreamTickerRequest{
        Exchange: "binance",
        Symbols:  []string{"BTCUSDT", "ETHUSDT", "BNBUSDT"},
    })
    
    for {
        ticker, err := stream.Recv()
        if err != nil {
            break
        }
        
        // Wait for rate limiter
        if err := limiter.Wait(ctx); err != nil {
            break
        }
        
        // Process ticker at controlled rate
        processTicker(ticker)
    }
}
```

## Part 5: Complete Streaming Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "os/signal"
    "sync"
    "syscall"
    "time"
    
    pb "github.com/your-org/mExOms/proto/gen/go/oms/v1"
    "google.golang.org/grpc"
)

type StreamManager struct {
    client     pb.MarketDataServiceClient
    orderClient pb.OrderServiceClient
    ctx        context.Context
    cancel     context.CancelFunc
    wg         sync.WaitGroup
}

func NewStreamManager(conn *grpc.ClientConn) *StreamManager {
    ctx, cancel := context.WithCancel(context.Background())
    return &StreamManager{
        client:      pb.NewMarketDataServiceClient(conn),
        orderClient: pb.NewOrderServiceClient(conn),
        ctx:         ctx,
        cancel:      cancel,
    }
}

func (sm *StreamManager) Start() {
    fmt.Println("Starting WebSocket streams...")
    
    // Start multiple streams
    sm.wg.Add(4)
    go sm.streamTickers()
    go sm.streamOrderBook()
    go sm.streamTrades()
    go sm.streamOrders()
}

func (sm *StreamManager) streamTickers() {
    defer sm.wg.Done()
    
    stream, err := sm.client.StreamTicker(sm.ctx, &pb.StreamTickerRequest{
        Exchange: "binance",
        Symbols:  []string{"BTCUSDT", "ETHUSDT"},
    })
    if err != nil {
        log.Printf("Failed to start ticker stream: %v", err)
        return
    }
    
    for {
        select {
        case <-sm.ctx.Done():
            return
        default:
            ticker, err := stream.Recv()
            if err != nil {
                log.Printf("Ticker stream error: %v", err)
                return
            }
            
            fmt.Printf("[TICKER] %s: $%.2f (%.2f%%)\n",
                ticker.Symbol,
                ticker.LastPrice,
                ticker.PriceChangePercent)
        }
    }
}

func (sm *StreamManager) streamOrderBook() {
    defer sm.wg.Done()
    
    stream, err := sm.client.StreamOrderBook(sm.ctx, &pb.StreamOrderBookRequest{
        Exchange: "binance",
        Symbol:   "BTCUSDT",
        Depth:    5,
    })
    if err != nil {
        log.Printf("Failed to start order book stream: %v", err)
        return
    }
    
    for {
        select {
        case <-sm.ctx.Done():
            return
        default:
            book, err := stream.Recv()
            if err != nil {
                log.Printf("Order book stream error: %v", err)
                return
            }
            
            if len(book.Bids) > 0 && len(book.Asks) > 0 {
                spread := book.Asks[0].Price - book.Bids[0].Price
                fmt.Printf("[BOOK] Bid: %.2f, Ask: %.2f, Spread: %.2f\n",
                    book.Bids[0].Price,
                    book.Asks[0].Price,
                    spread)
            }
        }
    }
}

func (sm *StreamManager) streamTrades() {
    defer sm.wg.Done()
    
    stream, err := sm.client.StreamTrades(sm.ctx, &pb.StreamTradesRequest{
        Exchange: "binance",
        Symbol:   "BTCUSDT",
    })
    if err != nil {
        log.Printf("Failed to start trade stream: %v", err)
        return
    }
    
    for {
        select {
        case <-sm.ctx.Done():
            return
        default:
            trade, err := stream.Recv()
            if err != nil {
                log.Printf("Trade stream error: %v", err)
                return
            }
            
            // Only show large trades
            if trade.Quantity*trade.Price > 50000 {
                side := "BUY"
                if trade.IsBuyerMaker {
                    side = "SELL"
                }
                fmt.Printf("[TRADE] 🐋 %s %.3f @ %.2f ($%.0f)\n",
                    side,
                    trade.Quantity,
                    trade.Price,
                    trade.Quantity*trade.Price)
            }
        }
    }
}

func (sm *StreamManager) streamOrders() {
    defer sm.wg.Done()
    
    stream, err := sm.orderClient.StreamOrders(sm.ctx, &pb.StreamOrdersRequest{
        Exchange: "binance",
    })
    if err != nil {
        log.Printf("Failed to start order stream: %v", err)
        return
    }
    
    for {
        select {
        case <-sm.ctx.Done():
            return
        default:
            order, err := stream.Recv()
            if err != nil {
                log.Printf("Order stream error: %v", err)
                return
            }
            
            fmt.Printf("[ORDER] %s %s: %s %.4f @ %.2f\n",
                order.Symbol,
                order.OrderId[:8],
                order.Status,
                order.Quantity,
                order.Price)
        }
    }
}

func (sm *StreamManager) Stop() {
    fmt.Println("\nStopping streams...")
    sm.cancel()
    sm.wg.Wait()
    fmt.Println("All streams stopped")
}

func main() {
    // Connect to OMS
    conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer conn.Close()
    
    // Create stream manager
    manager := NewStreamManager(conn)
    
    // Start streaming
    manager.Start()
    
    // Handle shutdown gracefully
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    
    <-sigChan
    manager.Stop()
}
```

## Best Practices

1. **Connection Management**
   - Implement automatic reconnection
   - Use exponential backoff for retries
   - Monitor connection health

2. **Performance**
   - Use buffered channels
   - Implement backpressure handling
   - Consider sampling for high-frequency data

3. **Error Handling**
   - Log stream errors appropriately
   - Don't panic on stream errors
   - Gracefully degrade functionality

4. **Resource Management**
   - Close streams properly
   - Cancel contexts on shutdown
   - Avoid goroutine leaks

5. **Data Processing**
   - Process data asynchronously
   - Use worker pools for heavy processing
   - Implement rate limiting if needed

## Common Patterns

### Snapshot + Updates
```go
// Get initial snapshot
book, _ := client.GetOrderBook(ctx, &pb.GetOrderBookRequest{
    Symbol: "BTCUSDT",
})

// Then subscribe to updates
stream, _ := client.StreamOrderBook(ctx, &pb.StreamOrderBookRequest{
    Symbol: "BTCUSDT",
})
```

### Multiplexing Streams
```go
// Combine multiple streams into single channel
combined := make(chan interface{}, 100)

go func() {
    // Stream 1
    for ticker := range tickerStream {
        combined <- ticker
    }
}()

go func() {
    // Stream 2  
    for trade := range tradeStream {
        combined <- trade
    }
}()
```

## Troubleshooting

1. **Connection Issues**
   - Check network connectivity
   - Verify API keys and permissions
   - Check rate limits

2. **Missing Data**
   - Ensure proper subscription parameters
   - Check for network packet loss
   - Verify stream buffer sizes

3. **Performance Issues**
   - Monitor channel buffer usage
   - Check for blocking operations
   - Profile CPU and memory usage

## Next Steps

- [Smart Order Routing](./smart-order-routing.md) - Optimize order execution
- [Risk Management](./risk-management.md) - Real-time risk monitoring
- [Strategy Development](./strategy-development.md) - Build event-driven strategies