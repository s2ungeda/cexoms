# Smart Order Routing Tutorial

## Objective

Learn how to use the Smart Order Router (SOR) to optimize trade execution across multiple exchanges, minimize slippage, and achieve best execution prices.

## Prerequisites

- Completed [Multi-Account Trading Tutorial](./multi-account-trading.md)
- Understanding of order types and market microstructure
- Multiple exchange accounts configured

## What is Smart Order Routing?

Smart Order Routing automatically:
- **Finds Best Prices**: Scans multiple exchanges for optimal execution
- **Splits Orders**: Divides large orders to minimize market impact
- **Reduces Slippage**: Executes at multiple price levels
- **Optimizes Fees**: Considers trading fees in routing decisions
- **Handles Failures**: Reroutes orders if exchanges are unavailable

## Part 1: Basic Smart Order Routing

### Simple Best Price Routing

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    pb "github.com/your-org/mExOms/proto/gen/go/oms/v1"
    "google.golang.org/grpc"
)

func basicSmartOrder(client pb.SmartOrderServiceClient, ctx context.Context) {
    // Create a smart order that finds best execution
    smartOrder := &pb.SmartOrderRequest{
        Symbol:      "BTCUSDT",
        Side:        pb.OrderSide_BUY,
        Quantity:    0.5, // Buy 0.5 BTC
        OrderType:   pb.SmartOrderType_BEST_PRICE,
        
        // SOR will check these exchanges
        Exchanges: []string{"binance", "okx", "bybit"},
        
        // Execution parameters
        MaxSlippage: 0.001,  // 0.1% max slippage
        TimeLimit:   60,     // Complete within 60 seconds
    }
    
    resp, err := client.CreateSmartOrder(ctx, smartOrder)
    if err != nil {
        log.Fatalf("Failed to create smart order: %v", err)
    }
    
    fmt.Printf("Smart Order ID: %s\n", resp.SmartOrderId)
    fmt.Printf("Routing Plan:\n")
    
    for _, route := range resp.RoutingPlan {
        fmt.Printf("- %s: %.4f @ ~$%.2f\n", 
            route.Exchange, 
            route.Quantity, 
            route.ExpectedPrice)
    }
    
    // Monitor execution
    monitorSmartOrder(client, ctx, resp.SmartOrderId)
}
```

### Split Order Execution

```go
func splitOrderExecution(client pb.SmartOrderServiceClient, ctx context.Context) {
    // Large order that needs to be split
    largeOrder := &pb.SmartOrderRequest{
        Symbol:      "ETHUSDT",
        Side:        pb.OrderSide_BUY,
        Quantity:    100.0, // Buy 100 ETH (large order)
        OrderType:   pb.SmartOrderType_SPLIT_EXECUTION,
        
        Exchanges: []string{"binance", "okx", "bybit"},
        
        // Split parameters
        SplitStrategy: &pb.SplitStrategy{
            MaxOrderSize:    10.0,  // Max 10 ETH per order
            MinOrderSize:    1.0,   // Min 1 ETH per order
            TimeInterval:    5,     // 5 seconds between orders
            Strategy:        pb.SplitType_TWAP, // Time-weighted
        },
        
        MaxSlippage: 0.002,  // 0.2% max slippage
        TimeLimit:   300,    // 5 minutes to complete
    }
    
    resp, err := client.CreateSmartOrder(ctx, largeOrder)
    if err != nil {
        log.Fatalf("Failed to create split order: %v", err)
    }
    
    fmt.Printf("Smart Order created with %d child orders\n", 
        len(resp.ChildOrders))
    
    // Show execution schedule
    fmt.Println("\nExecution Schedule:")
    for i, child := range resp.ChildOrders {
        fmt.Printf("%d. %s: %.2f ETH @ %s\n",
            i+1,
            child.Exchange,
            child.Quantity,
            child.ScheduledTime.Format("15:04:05"))
    }
}
```

## Part 2: Advanced Routing Strategies

### Iceberg Orders

```go
func icebergOrder(client pb.SmartOrderServiceClient, ctx context.Context) {
    // Iceberg order - only show small portion in order book
    iceberg := &pb.SmartOrderRequest{
        Symbol:      "BTCUSDT",
        Side:        pb.OrderSide_SELL,
        Quantity:    5.0, // Total 5 BTC to sell
        OrderType:   pb.SmartOrderType_ICEBERG,
        
        Exchanges: []string{"binance"}, // Single exchange for iceberg
        
        IcebergParams: &pb.IcebergParameters{
            VisibleQuantity: 0.1,   // Only show 0.1 BTC at a time
            Variance:        0.2,   // 20% randomization
            PriceOffset:     0.0001, // Price improvement
        },
        
        TimeLimit: 3600, // 1 hour
    }
    
    resp, err := client.CreateSmartOrder(ctx, iceberg)
    if err != nil {
        log.Fatalf("Failed to create iceberg order: %v", err)
    }
    
    fmt.Printf("Iceberg order created: %s\n", resp.SmartOrderId)
    fmt.Printf("Total Quantity: %.2f BTC\n", iceberg.Quantity)
    fmt.Printf("Visible Quantity: %.2f BTC\n", 
        iceberg.IcebergParams.VisibleQuantity)
    
    // Monitor fills
    monitorIcebergFills(client, ctx, resp.SmartOrderId)
}
```

### VWAP Execution

```go
func vwapExecution(client pb.SmartOrderServiceClient, ctx context.Context) {
    // Execute at Volume-Weighted Average Price
    vwap := &pb.SmartOrderRequest{
        Symbol:      "BTCUSDT",
        Side:        pb.OrderSide_BUY,
        Quantity:    2.0,
        OrderType:   pb.SmartOrderType_VWAP,
        
        Exchanges: []string{"binance", "okx", "bybit"},
        
        VWAPParams: &pb.VWAPParameters{
            LookbackPeriod: 300,    // Use 5-min VWAP
            MaxDeviation:   0.001,  // Max 0.1% from VWAP
            Aggression:     0.5,    // Medium aggression
            ParticipationRate: 0.1, // Max 10% of volume
        },
        
        TimeLimit: 600, // 10 minutes
    }
    
    resp, err := client.CreateSmartOrder(ctx, vwap)
    if err != nil {
        log.Fatalf("Failed to create VWAP order: %v", err)
    }
    
    fmt.Printf("VWAP Order ID: %s\n", resp.SmartOrderId)
    fmt.Printf("Target VWAP: $%.2f\n", resp.TargetPrice)
    fmt.Printf("Current Market Price: $%.2f\n", resp.CurrentPrice)
}
```

### Liquidity Seeking

```go
func liquiditySeeking(client pb.SmartOrderServiceClient, ctx context.Context) {
    // Find and consume hidden liquidity
    liquidity := &pb.SmartOrderRequest{
        Symbol:      "ETHUSDT",
        Side:        pb.OrderSide_BUY,
        Quantity:    50.0,
        OrderType:   pb.SmartOrderType_LIQUIDITY_SEEKING,
        
        Exchanges: []string{"binance", "okx", "bybit", "huobi"},
        
        LiquidityParams: &pb.LiquidityParameters{
            MinFillSize:    0.5,     // Min 0.5 ETH per fill
            ProbeSize:      0.1,     // Test with 0.1 ETH
            DarkPoolCheck:  true,    // Check dark pools
            HiddenLevels:   5,       // Check 5 price levels
            AdaptiveRouting: true,   // Learn from fills
        },
        
        MaxSlippage: 0.003, // 0.3% max slippage
        TimeLimit:   900,   // 15 minutes
    }
    
    resp, err := client.CreateSmartOrder(ctx, liquidity)
    if err != nil {
        log.Fatalf("Failed to create liquidity seeking order: %v", err)
    }
    
    fmt.Printf("Liquidity Seeking Order: %s\n", resp.SmartOrderId)
    fmt.Printf("Initial liquidity map:\n")
    
    for _, exchange := range resp.LiquidityMap {
        fmt.Printf("- %s: %.2f ETH available\n", 
            exchange.Exchange, exchange.AvailableLiquidity)
    }
}
```

## Part 3: Smart Order Router Configuration

### Route Optimization

```go
func optimizedRouting(client pb.SmartOrderServiceClient, ctx context.Context) {
    // Configure advanced routing optimization
    optimized := &pb.SmartOrderRequest{
        Symbol:      "BTCUSDT",
        Side:        pb.OrderSide_BUY,
        Quantity:    1.0,
        OrderType:   pb.SmartOrderType_OPTIMIZED,
        
        Exchanges: []string{"binance", "okx", "bybit", "kraken", "coinbase"},
        
        OptimizationParams: &pb.OptimizationParameters{
            // Optimization goals (weights must sum to 1.0)
            PriceWeight:      0.4,   // 40% weight on best price
            SpeedWeight:      0.3,   // 30% weight on execution speed  
            FeeWeight:        0.2,   // 20% weight on fees
            SlippageWeight:   0.1,   // 10% weight on slippage
            
            // Constraints
            MaxExchanges:     3,     // Use at most 3 exchanges
            MinExchangeSize:  0.1,   // Min 0.1 BTC per exchange
            AvoidMakerFees:   true,  // Prefer taker orders
            
            // Advanced features
            UseML:            true,  // Use ML predictions
            HistoricalData:   true,  // Consider historical fills
        },
        
        // Fee structure for each exchange
        FeeOverrides: []*pb.FeeStructure{
            {
                Exchange:   "binance",
                MakerFee:   0.0002, // 0.02%
                TakerFee:   0.0004, // 0.04%
                VIPLevel:   3,      // VIP 3 fees
            },
            {
                Exchange:   "okx",
                MakerFee:   0.0001,
                TakerFee:   0.0005,
                VIPLevel:   2,
            },
        },
    }
    
    resp, err := client.CreateSmartOrder(ctx, optimized)
    if err != nil {
        log.Fatalf("Failed to create optimized order: %v", err)
    }
    
    fmt.Printf("Optimization Score: %.2f\n", resp.OptimizationScore)
    fmt.Printf("Predicted Slippage: %.2f%%\n", resp.PredictedSlippage*100)
    fmt.Printf("Estimated Total Cost: $%.2f\n", resp.EstimatedTotalCost)
    
    fmt.Println("\nOptimal Routing:")
    for _, route := range resp.RoutingPlan {
        fmt.Printf("- %s: %.3f BTC @ $%.2f (Fee: $%.2f)\n",
            route.Exchange,
            route.Quantity,
            route.ExpectedPrice,
            route.ExpectedFee)
    }
}
```

### Conditional Routing

```go
func conditionalRouting(client pb.SmartOrderServiceClient, ctx context.Context) {
    // Route based on market conditions
    conditional := &pb.SmartOrderRequest{
        Symbol:      "ETHUSDT",
        Side:        pb.OrderSide_SELL,
        Quantity:    20.0,
        OrderType:   pb.SmartOrderType_CONDITIONAL,
        
        Exchanges: []string{"binance", "okx", "bybit"},
        
        ConditionalParams: &pb.ConditionalParameters{
            Conditions: []*pb.Condition{
                {
                    Type:      pb.ConditionType_SPREAD,
                    Operator:  pb.Operator_LESS_THAN,
                    Value:     5.0, // If spread < $5
                    Action:    pb.Action_USE_AGGRESSIVE,
                    Exchange:  "binance",
                },
                {
                    Type:      pb.ConditionType_VOLUME,
                    Operator:  pb.Operator_GREATER_THAN,
                    Value:     1000000.0, // If 24h volume > $1M
                    Action:    pb.Action_SPLIT_EQUALLY,
                },
                {
                    Type:      pb.ConditionType_VOLATILITY,
                    Operator:  pb.Operator_GREATER_THAN,
                    Value:     0.02, // If volatility > 2%
                    Action:    pb.Action_USE_ICEBERG,
                },
            },
            
            DefaultStrategy: pb.Strategy_BALANCED,
            ReevaluateInterval: 30, // Re-check conditions every 30s
        },
    }
    
    resp, err := client.CreateSmartOrder(ctx, conditional)
    if err != nil {
        log.Fatalf("Failed to create conditional order: %v", err)
    }
    
    fmt.Printf("Conditional Order: %s\n", resp.SmartOrderId)
    fmt.Printf("Active Conditions: %d\n", 
        len(conditional.ConditionalParams.Conditions))
    fmt.Printf("Selected Strategy: %s\n", resp.SelectedStrategy)
}
```

## Part 4: Monitoring and Analytics

### Real-time Execution Monitoring

```go
func monitorSmartOrder(client pb.SmartOrderServiceClient, 
    ctx context.Context, orderId string) {
    
    // Subscribe to smart order updates
    stream, err := client.StreamSmartOrderUpdates(ctx, 
        &pb.StreamSmartOrderRequest{
            SmartOrderId: orderId,
        })
    if err != nil {
        log.Fatalf("Failed to stream updates: %v", err)
    }
    
    startTime := time.Now()
    var totalFilled float64
    var totalCost float64
    
    for {
        update, err := stream.Recv()
        if err != nil {
            log.Printf("Stream error: %v", err)
            break
        }
        
        switch update.Type {
        case pb.UpdateType_CHILD_ORDER_CREATED:
            fmt.Printf("[%s] Created order on %s: %.4f\n",
                update.Timestamp.Format("15:04:05"),
                update.Exchange,
                update.Quantity)
                
        case pb.UpdateType_PARTIAL_FILL:
            fmt.Printf("[%s] Partial fill on %s: %.4f @ $%.2f\n",
                update.Timestamp.Format("15:04:05"),
                update.Exchange,
                update.FilledQuantity,
                update.FilledPrice)
            
            totalFilled += update.FilledQuantity
            totalCost += update.FilledQuantity * update.FilledPrice
                
        case pb.UpdateType_COMPLETED:
            duration := time.Since(startTime)
            avgPrice := totalCost / totalFilled
            
            fmt.Printf("\n✅ Smart Order Completed!\n")
            fmt.Printf("Total Filled: %.4f\n", totalFilled)
            fmt.Printf("Average Price: $%.2f\n", avgPrice)
            fmt.Printf("Total Cost: $%.2f\n", totalCost)
            fmt.Printf("Execution Time: %s\n", duration)
            
            // Show fill distribution
            fmt.Println("\nFill Distribution:")
            for exchange, stats := range update.ExchangeStats {
                fmt.Printf("- %s: %.4f (%.1f%%) @ $%.2f\n",
                    exchange,
                    stats.FilledQuantity,
                    (stats.FilledQuantity/totalFilled)*100,
                    stats.AveragePrice)
            }
            return
            
        case pb.UpdateType_FAILED:
            fmt.Printf("❌ Order failed: %s\n", update.ErrorMessage)
            return
        }
    }
}
```

### Performance Analytics

```go
func analyzeRoutingPerformance(client pb.SmartOrderServiceClient, 
    ctx context.Context, orderId string) {
    
    // Get detailed analytics
    analytics, err := client.GetSmartOrderAnalytics(ctx,
        &pb.GetAnalyticsRequest{
            SmartOrderId: orderId,
        })
    if err != nil {
        log.Fatalf("Failed to get analytics: %v", err)
    }
    
    fmt.Println("=== Smart Order Analytics ===")
    
    // Execution metrics
    fmt.Printf("\nExecution Metrics:\n")
    fmt.Printf("Expected Price: $%.2f\n", analytics.ExpectedPrice)
    fmt.Printf("Actual Avg Price: $%.2f\n", analytics.ActualAvgPrice)
    fmt.Printf("Slippage: %.3f%% ($%.2f)\n", 
        analytics.SlippagePercent*100,
        analytics.SlippageAmount)
    fmt.Printf("Price Improvement: $%.2f\n", analytics.PriceImprovement)
    
    // Time metrics
    fmt.Printf("\nTime Metrics:\n")
    fmt.Printf("Total Duration: %s\n", analytics.TotalDuration)
    fmt.Printf("Avg Fill Time: %s\n", analytics.AvgFillTime)
    fmt.Printf("Fastest Fill: %s on %s\n", 
        analytics.FastestFill.Duration,
        analytics.FastestFill.Exchange)
    
    // Cost analysis
    fmt.Printf("\nCost Analysis:\n")
    fmt.Printf("Trading Fees: $%.2f\n", analytics.TotalFees)
    fmt.Printf("Estimated Market Impact: $%.2f\n", analytics.MarketImpact)
    fmt.Printf("Total Cost: $%.2f\n", 
        analytics.TotalCost + analytics.TotalFees)
    
    // Exchange performance
    fmt.Printf("\nExchange Performance:\n")
    for _, perf := range analytics.ExchangePerformance {
        fmt.Printf("- %s: %.1f%% filled, Avg slippage: %.3f%%, Reject rate: %.1f%%\n",
            perf.Exchange,
            perf.FillRate*100,
            perf.AvgSlippage*100,
            perf.RejectRate*100)
    }
    
    // Recommendations
    if len(analytics.Recommendations) > 0 {
        fmt.Printf("\nRecommendations:\n")
        for _, rec := range analytics.Recommendations {
            fmt.Printf("- %s\n", rec)
        }
    }
}
```

## Part 5: Complete Smart Order Router Example

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

type SmartOrderDemo struct {
    client pb.SmartOrderServiceClient
    ctx    context.Context
}

func NewSmartOrderDemo(conn *grpc.ClientConn) *SmartOrderDemo {
    return &SmartOrderDemo{
        client: pb.NewSmartOrderServiceClient(conn),
        ctx:    context.Background(),
    }
}

func (demo *SmartOrderDemo) ExecuteSmartOrder() {
    // 1. Get market snapshot
    snapshot, err := demo.client.GetMarketSnapshot(demo.ctx,
        &pb.MarketSnapshotRequest{
            Symbol:    "BTCUSDT",
            Exchanges: []string{"binance", "okx", "bybit"},
        })
    if err != nil {
        log.Fatalf("Failed to get market snapshot: %v", err)
    }
    
    fmt.Println("=== Market Snapshot ===")
    for _, market := range snapshot.Markets {
        fmt.Printf("%s - Bid: $%.2f, Ask: $%.2f, Spread: $%.2f\n",
            market.Exchange,
            market.BestBid,
            market.BestAsk,
            market.BestAsk - market.BestBid)
    }
    
    // 2. Simulate order impact
    simulation, err := demo.client.SimulateOrder(demo.ctx,
        &pb.SimulateOrderRequest{
            Symbol:    "BTCUSDT",
            Side:      pb.OrderSide_BUY,
            Quantity:  1.0,
            Exchanges: []string{"binance", "okx", "bybit"},
        })
    if err != nil {
        log.Fatalf("Failed to simulate order: %v", err)
    }
    
    fmt.Printf("\n=== Order Simulation ===\n")
    fmt.Printf("Expected Avg Price: $%.2f\n", simulation.ExpectedAvgPrice)
    fmt.Printf("Expected Slippage: %.3f%%\n", simulation.ExpectedSlippage*100)
    fmt.Printf("Recommended Strategy: %s\n", simulation.RecommendedStrategy)
    
    // 3. Create smart order based on simulation
    smartOrder := &pb.SmartOrderRequest{
        Symbol:      "BTCUSDT",
        Side:        pb.OrderSide_BUY,
        Quantity:    1.0,
        OrderType:   pb.SmartOrderType_ADAPTIVE,
        
        Exchanges: []string{"binance", "okx", "bybit"},
        
        // Adaptive parameters - adjust based on real-time conditions
        AdaptiveParams: &pb.AdaptiveParameters{
            InitialStrategy: simulation.RecommendedStrategy,
            
            // Adapt based on fills
            AdaptOnFills: true,
            FillThreshold: 0.3, // Adapt after 30% filled
            
            // Market condition triggers
            SpreadTrigger:     10.0,  // Adapt if spread > $10
            VolatilityTrigger: 0.02,  // Adapt if volatility > 2%
            
            // Strategies to consider
            AllowedStrategies: []pb.Strategy{
                pb.Strategy_AGGRESSIVE,
                pb.Strategy_PASSIVE,
                pb.Strategy_BALANCED,
                pb.Strategy_STEALTH,
            },
        },
        
        // Risk limits
        MaxSlippage:   0.002,  // 0.2% max
        MaxRetries:    3,
        TimeLimit:     300,    // 5 minutes
        UrgencyLevel:  pb.Urgency_NORMAL,
    }
    
    fmt.Printf("\n=== Executing Smart Order ===\n")
    resp, err := demo.client.CreateSmartOrder(demo.ctx, smartOrder)
    if err != nil {
        log.Fatalf("Failed to create smart order: %v", err)
    }
    
    fmt.Printf("Smart Order ID: %s\n", resp.SmartOrderId)
    fmt.Printf("Initial Strategy: %s\n", resp.InitialStrategy)
    
    // 4. Monitor execution with live updates
    done := make(chan bool)
    go demo.monitorExecution(resp.SmartOrderId, done)
    
    // Wait for completion
    <-done
    
    // 5. Get final analytics
    time.Sleep(2 * time.Second) // Let system finalize
    demo.showFinalAnalytics(resp.SmartOrderId)
}

func (demo *SmartOrderDemo) monitorExecution(orderId string, done chan bool) {
    stream, err := demo.client.StreamSmartOrderUpdates(demo.ctx,
        &pb.StreamSmartOrderRequest{
            SmartOrderId: orderId,
        })
    if err != nil {
        log.Printf("Failed to stream updates: %v", err)
        done <- true
        return
    }
    
    var fills []Fill
    startTime := time.Now()
    
    for {
        update, err := stream.Recv()
        if err != nil {
            log.Printf("Stream error: %v", err)
            break
        }
        
        switch update.Type {
        case pb.UpdateType_STRATEGY_CHANGED:
            fmt.Printf("📊 Strategy changed to: %s (Reason: %s)\n",
                update.NewStrategy, update.Reason)
                
        case pb.UpdateType_PARTIAL_FILL:
            fill := Fill{
                Exchange:  update.Exchange,
                Quantity:  update.FilledQuantity,
                Price:     update.FilledPrice,
                Timestamp: update.Timestamp,
            }
            fills = append(fills, fill)
            
            totalFilled := 0.0
            for _, f := range fills {
                totalFilled += f.Quantity
            }
            
            fmt.Printf("✅ Fill on %s: %.4f @ $%.2f (%.1f%% complete)\n",
                update.Exchange,
                update.FilledQuantity,
                update.FilledPrice,
                (totalFilled/1.0)*100)
                
        case pb.UpdateType_COMPLETED:
            duration := time.Since(startTime)
            fmt.Printf("\n🎉 Order completed in %s!\n", duration)
            done <- true
            return
            
        case pb.UpdateType_FAILED:
            fmt.Printf("❌ Order failed: %s\n", update.ErrorMessage)
            done <- true
            return
        }
    }
    
    done <- true
}

type Fill struct {
    Exchange  string
    Quantity  float64
    Price     float64
    Timestamp time.Time
}

func (demo *SmartOrderDemo) showFinalAnalytics(orderId string) {
    analytics, err := demo.client.GetSmartOrderAnalytics(demo.ctx,
        &pb.GetAnalyticsRequest{
            SmartOrderId: orderId,
            IncludeDetails: true,
        })
    if err != nil {
        log.Printf("Failed to get analytics: %v", err)
        return
    }
    
    fmt.Println("\n=== Final Execution Report ===")
    
    // Summary
    fmt.Printf("\nSummary:\n")
    fmt.Printf("Total Quantity: %.4f BTC\n", analytics.TotalQuantity)
    fmt.Printf("Average Price: $%.2f\n", analytics.ActualAvgPrice)
    fmt.Printf("Total Cost: $%.2f\n", analytics.TotalCost)
    fmt.Printf("Total Fees: $%.2f\n", analytics.TotalFees)
    fmt.Printf("Slippage: %.3f%% ($%.2f)\n",
        analytics.SlippagePercent*100,
        analytics.SlippageAmount)
    
    // Execution breakdown
    fmt.Printf("\nExecution by Exchange:\n")
    for _, exch := range analytics.ExchangeBreakdown {
        fmt.Printf("- %s: %.4f BTC (%.1f%%) @ avg $%.2f\n",
            exch.Exchange,
            exch.Quantity,
            (exch.Quantity/analytics.TotalQuantity)*100,
            exch.AvgPrice)
    }
    
    // Strategy performance
    fmt.Printf("\nStrategy Usage:\n")
    for strategy, duration := range analytics.StrategyDurations {
        fmt.Printf("- %s: %s\n", strategy, duration)
    }
    
    // Cost savings
    if analytics.CostSavings > 0 {
        fmt.Printf("\n💰 Cost Savings: $%.2f vs single exchange execution\n",
            analytics.CostSavings)
    }
    
    // Performance grade
    fmt.Printf("\nPerformance Grade: %s\n", analytics.PerformanceGrade)
}

func main() {
    // Connect to OMS
    conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer conn.Close()
    
    // Run smart order demo
    demo := NewSmartOrderDemo(conn)
    demo.ExecuteSmartOrder()
}
```

## Best Practices

1. **Order Size Considerations**
   - Use split execution for large orders
   - Consider market impact on price
   - Monitor liquidity across exchanges

2. **Strategy Selection**
   - Aggressive for liquid markets
   - Passive for illiquid markets
   - Adaptive for changing conditions

3. **Risk Management**
   - Set appropriate slippage limits
   - Use time limits for execution
   - Monitor partial fill scenarios

4. **Cost Optimization**
   - Consider exchange fees
   - Factor in withdrawal costs
   - Account for spread differences

5. **Monitoring**
   - Track execution quality
   - Analyze slippage patterns
   - Review strategy performance

## Common Routing Strategies

### Time-Based
- **TWAP**: Time-Weighted Average Price
- **VWAP**: Volume-Weighted Average Price
- **POV**: Percentage of Volume

### Price-Based
- **Limit**: Execute at specific price
- **Pegged**: Track best bid/ask
- **Discretionary**: Price range execution

### Liquidity-Based
- **Iceberg**: Hidden quantity
- **Sniper**: Detect hidden liquidity
- **Dark Pool**: Access non-displayed liquidity

## Troubleshooting

1. **High Slippage**
   - Reduce order size
   - Increase time limit
   - Use passive strategies

2. **Slow Execution**
   - Check exchange connectivity
   - Verify liquidity availability
   - Adjust urgency level

3. **Rejections**
   - Verify account balances
   - Check order minimums
   - Review exchange status

## Next Steps

- [Risk Management](./risk-management.md) - Integrate risk controls
- [Performance Monitoring](./performance-monitoring.md) - Track SOR metrics
- [Strategy Development](./strategy-development.md) - Build custom routing logic