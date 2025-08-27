# Basic Trading Tutorial

## Objective

Learn how to execute different types of orders, manage positions, and implement basic trading operations using mExOms.

## Prerequisites

- Completed [Getting Started](./getting-started.md) tutorial
- mExOms system running
- Exchange API keys configured
- Basic understanding of trading concepts

## Order Types Supported

mExOms supports various order types across exchanges:

- **Market Orders**: Immediate execution at best available price
- **Limit Orders**: Execute at specified price or better
- **Stop Loss**: Trigger market order when price reaches stop level
- **Stop Limit**: Trigger limit order when price reaches stop level
- **OCO (One-Cancels-Other)**: Two orders where one cancels the other

## Part 1: Market Orders

### Example: Buy Bitcoin with Market Order

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

func main() {
    // Connect to OMS
    conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer conn.Close()
    
    client := pb.NewOrderServiceClient(conn)
    ctx := context.Background()
    
    // Create market buy order
    marketOrder := &pb.CreateOrderRequest{
        Symbol:      "BTCUSDT",
        Exchange:    "binance",
        Market:      "spot",
        Type:        pb.OrderType_MARKET,
        Side:        pb.OrderSide_BUY,
        Quantity:    0.001,  // Buy 0.001 BTC
        TimeInForce: pb.TimeInForce_IOC,  // Immediate or Cancel
    }
    
    resp, err := client.CreateOrder(ctx, marketOrder)
    if err != nil {
        log.Fatalf("Failed to create order: %v", err)
    }
    
    fmt.Printf("Market order executed!\n")
    fmt.Printf("Order ID: %s\n", resp.OrderId)
    fmt.Printf("Status: %s\n", resp.Status)
    fmt.Printf("Executed Price: %f\n", resp.ExecutedPrice)
    fmt.Printf("Executed Quantity: %f\n", resp.ExecutedQuantity)
}
```

## Part 2: Limit Orders

### Example: Place Limit Buy Order

```go
// Create limit buy order below current market price
limitOrder := &pb.CreateOrderRequest{
    Symbol:      "BTCUSDT",
    Exchange:    "binance",
    Market:      "spot",
    Type:        pb.OrderType_LIMIT,
    Side:        pb.OrderSide_BUY,
    Quantity:    0.001,
    Price:       45000.00,  // Buy if price drops to $45,000
    TimeInForce: pb.TimeInForce_GTC,  // Good Till Cancelled
}

resp, err := client.CreateOrder(ctx, limitOrder)
if err != nil {
    log.Fatalf("Failed to create limit order: %v", err)
}

// Monitor order status
orderId := resp.OrderId
for {
    status, err := client.GetOrder(ctx, &pb.GetOrderRequest{
        OrderId:  orderId,
        Exchange: "binance",
    })
    if err != nil {
        log.Printf("Error checking order: %v", err)
        break
    }
    
    fmt.Printf("Order Status: %s\n", status.Status)
    
    if status.Status == pb.OrderStatus_FILLED || 
       status.Status == pb.OrderStatus_CANCELLED {
        break
    }
    
    time.Sleep(5 * time.Second)
}
```

## Part 3: Stop Loss Orders

### Example: Protect Position with Stop Loss

```go
// First, get current position
positions, err := client.GetPositions(ctx, &pb.GetPositionsRequest{
    Exchange: "binance",
    Market:   "spot",
})

if len(positions.Positions) > 0 && positions.Positions[0].Symbol == "BTCUSDT" {
    position := positions.Positions[0]
    
    // Set stop loss at 5% below entry price
    stopPrice := position.EntryPrice * 0.95
    
    stopLossOrder := &pb.CreateOrderRequest{
        Symbol:      "BTCUSDT",
        Exchange:    "binance",
        Market:      "spot",
        Type:        pb.OrderType_STOP_LOSS,
        Side:        pb.OrderSide_SELL,
        Quantity:    position.Quantity,
        StopPrice:   stopPrice,
        TimeInForce: pb.TimeInForce_GTC,
    }
    
    resp, err := client.CreateOrder(ctx, stopLossOrder)
    if err != nil {
        log.Fatalf("Failed to create stop loss: %v", err)
    }
    
    fmt.Printf("Stop loss set at $%.2f\n", stopPrice)
}
```

## Part 4: OCO Orders

### Example: Take Profit + Stop Loss

```go
// Create OCO order: Take profit at +10% OR stop loss at -5%
ocoOrder := &pb.CreateOCOOrderRequest{
    Symbol:   "BTCUSDT",
    Exchange: "binance",
    Market:   "spot",
    
    // Take profit order
    LimitOrder: &pb.OrderDetails{
        Side:        pb.OrderSide_SELL,
        Quantity:    0.001,
        Price:       55000.00,  // Take profit at $55,000
        TimeInForce: pb.TimeInForce_GTC,
    },
    
    // Stop loss order
    StopOrder: &pb.OrderDetails{
        Side:        pb.OrderSide_SELL,
        Quantity:    0.001,
        StopPrice:   47500.00,  // Stop at $47,500
        LimitPrice:  47400.00,  // Limit price for stop
        TimeInForce: pb.TimeInForce_GTC,
    },
}

ocoResp, err := client.CreateOCOOrder(ctx, ocoOrder)
if err != nil {
    log.Fatalf("Failed to create OCO order: %v", err)
}

fmt.Printf("OCO Order created!\n")
fmt.Printf("List Order ID: %s\n", ocoResp.ListOrderId)
```

## Part 5: Managing Orders

### Cancel Order

```go
// Cancel a specific order
cancelReq := &pb.CancelOrderRequest{
    OrderId:  orderId,
    Exchange: "binance",
    Symbol:   "BTCUSDT",
}

cancelResp, err := client.CancelOrder(ctx, cancelReq)
if err != nil {
    log.Printf("Failed to cancel order: %v", err)
} else {
    fmt.Printf("Order cancelled: %s\n", cancelResp.OrderId)
}
```

### Cancel All Orders

```go
// Cancel all open orders for a symbol
cancelAllReq := &pb.CancelAllOrdersRequest{
    Exchange: "binance",
    Symbol:   "BTCUSDT",
}

cancelAllResp, err := client.CancelAllOrders(ctx, cancelAllReq)
if err != nil {
    log.Printf("Failed to cancel orders: %v", err)
} else {
    fmt.Printf("Cancelled %d orders\n", len(cancelAllResp.CancelledOrderIds))
}
```

## Part 6: Position Management

### Get Current Positions

```go
// Get all positions
posReq := &pb.GetPositionsRequest{
    Exchange: "binance",
    Market:   "spot",
}

positions, err := client.GetPositions(ctx, posReq)
if err != nil {
    log.Fatalf("Failed to get positions: %v", err)
}

for _, pos := range positions.Positions {
    fmt.Printf("Symbol: %s\n", pos.Symbol)
    fmt.Printf("Quantity: %f\n", pos.Quantity)
    fmt.Printf("Entry Price: $%.2f\n", pos.EntryPrice)
    fmt.Printf("Current Price: $%.2f\n", pos.MarkPrice)
    fmt.Printf("Unrealized PnL: $%.2f\n", pos.UnrealizedPnl)
    fmt.Printf("PnL %%: %.2f%%\n", pos.PnlPercentage)
    fmt.Println("---")
}
```

## Part 7: WebSocket Streaming

### Real-time Order Updates

```go
// Subscribe to order updates
stream, err := client.StreamOrders(ctx, &pb.StreamOrdersRequest{
    Exchange: "binance",
})

if err != nil {
    log.Fatalf("Failed to start order stream: %v", err)
}

go func() {
    for {
        update, err := stream.Recv()
        if err != nil {
            log.Printf("Stream error: %v", err)
            return
        }
        
        fmt.Printf("Order Update: %s - Status: %s\n", 
            update.OrderId, update.Status)
    }
}()
```

## Part 8: Risk Management Integration

### Check Risk Before Placing Order

```go
// Validate order against risk limits
riskCheck := &pb.ValidateOrderRequest{
    Order: &pb.CreateOrderRequest{
        Symbol:   "BTCUSDT",
        Exchange: "binance",
        Market:   "spot",
        Type:     pb.OrderType_MARKET,
        Side:     pb.OrderSide_BUY,
        Quantity: 0.01,  // Check if we can buy 0.01 BTC
    },
}

riskResp, err := client.ValidateOrder(ctx, riskCheck)
if err != nil {
    log.Fatalf("Risk validation failed: %v", err)
}

if riskResp.IsValid {
    fmt.Println("Order passes risk checks")
    // Proceed with order
} else {
    fmt.Printf("Order rejected: %s\n", riskResp.Reason)
    for _, violation := range riskResp.Violations {
        fmt.Printf("- %s: %s\n", violation.Rule, violation.Message)
    }
}
```

## Complete Trading Example

Here's a complete example that combines multiple concepts:

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

func main() {
    // Setup connection
    conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer conn.Close()
    
    client := pb.NewOrderServiceClient(conn)
    ctx := context.Background()
    
    // 1. Check account balance
    balance, err := client.GetBalance(ctx, &pb.GetBalanceRequest{
        Exchange: "binance",
        Asset:    "USDT",
    })
    if err != nil {
        log.Fatalf("Failed to get balance: %v", err)
    }
    fmt.Printf("USDT Balance: %f\n", balance.Free)
    
    // 2. Get current market price
    ticker, err := client.GetTicker(ctx, &pb.GetTickerRequest{
        Exchange: "binance",
        Symbol:   "BTCUSDT",
    })
    if err != nil {
        log.Fatalf("Failed to get ticker: %v", err)
    }
    fmt.Printf("Current BTC Price: $%.2f\n", ticker.LastPrice)
    
    // 3. Place limit buy order 2% below market
    buyPrice := ticker.LastPrice * 0.98
    buyQuantity := 100.0 / buyPrice  // $100 worth
    
    buyOrder := &pb.CreateOrderRequest{
        Symbol:      "BTCUSDT",
        Exchange:    "binance",
        Market:      "spot",
        Type:        pb.OrderType_LIMIT,
        Side:        pb.OrderSide_BUY,
        Quantity:    buyQuantity,
        Price:       buyPrice,
        TimeInForce: pb.TimeInForce_GTC,
    }
    
    // 4. Validate against risk limits
    riskCheck, err := client.ValidateOrder(ctx, &pb.ValidateOrderRequest{
        Order: buyOrder,
    })
    if err != nil || !riskCheck.IsValid {
        log.Fatalf("Order failed risk check: %v", riskCheck.Reason)
    }
    
    // 5. Place the order
    orderResp, err := client.CreateOrder(ctx, buyOrder)
    if err != nil {
        log.Fatalf("Failed to create order: %v", err)
    }
    
    fmt.Printf("Order placed! ID: %s\n", orderResp.OrderId)
    
    // 6. Monitor until filled
    for {
        status, err := client.GetOrder(ctx, &pb.GetOrderRequest{
            OrderId:  orderResp.OrderId,
            Exchange: "binance",
        })
        if err != nil {
            log.Printf("Error checking order: %v", err)
            break
        }
        
        if status.Status == pb.OrderStatus_FILLED {
            fmt.Println("Order filled!")
            
            // 7. Set stop loss and take profit
            ocoOrder := &pb.CreateOCOOrderRequest{
                Symbol:   "BTCUSDT",
                Exchange: "binance",
                Market:   "spot",
                
                // Take profit at +5%
                LimitOrder: &pb.OrderDetails{
                    Side:        pb.OrderSide_SELL,
                    Quantity:    buyQuantity,
                    Price:       buyPrice * 1.05,
                    TimeInForce: pb.TimeInForce_GTC,
                },
                
                // Stop loss at -2%
                StopOrder: &pb.OrderDetails{
                    Side:        pb.OrderSide_SELL,
                    Quantity:    buyQuantity,
                    StopPrice:   buyPrice * 0.98,
                    LimitPrice:  buyPrice * 0.975,
                    TimeInForce: pb.TimeInForce_GTC,
                },
            }
            
            ocoResp, err := client.CreateOCOOrder(ctx, ocoOrder)
            if err != nil {
                log.Printf("Failed to create OCO: %v", err)
            } else {
                fmt.Printf("Protection orders set! OCO ID: %s\n", 
                    ocoResp.ListOrderId)
            }
            break
        }
        
        time.Sleep(5 * time.Second)
    }
}
```

## Best Practices

1. **Always validate orders against risk limits** before submission
2. **Use appropriate TimeInForce** values:
   - IOC for immediate execution
   - GTC for orders that should remain open
   - FOK when full execution is required
3. **Set stop losses** to protect positions
4. **Monitor order status** using WebSocket for real-time updates
5. **Handle errors gracefully** - network issues can occur
6. **Use OCO orders** for automated exit strategies
7. **Check account balance** before placing orders

## Common Error Handling

```go
switch err := err.(type) {
case *pb.OrderError:
    switch err.Code {
    case pb.ErrorCode_INSUFFICIENT_BALANCE:
        fmt.Println("Not enough balance")
    case pb.ErrorCode_MIN_NOTIONAL:
        fmt.Println("Order too small")
    case pb.ErrorCode_RATE_LIMIT:
        fmt.Println("Too many requests")
    default:
        fmt.Printf("Order error: %s\n", err.Message)
    }
default:
    log.Printf("Unexpected error: %v", err)
}
```

## Exercises

1. **Create a DCA Bot**: Place market orders every hour
2. **Implement Trailing Stop**: Adjust stop loss as price moves up
3. **Grid Trading**: Place multiple limit orders at intervals
4. **Arbitrage Scanner**: Compare prices across exchanges

## Next Steps

- [Multi-Account Trading](./multi-account-trading.md) - Trade across multiple accounts
- [Smart Order Routing](./smart-order-routing.md) - Optimize execution
- [Strategy Development](./strategy-development.md) - Build automated strategies