# Multi-Account Trading Tutorial

## Objective

Learn how to manage and trade across multiple exchange accounts simultaneously, implement account rotation strategies, and optimize trading distribution.

## Prerequisites

- Completed [Basic Trading Tutorial](./basic-trading.md)
- Multiple exchange accounts configured
- Understanding of account management concepts

## Why Multi-Account Trading?

- **Risk Distribution**: Spread risk across multiple accounts
- **Rate Limit Management**: Avoid hitting exchange limits
- **Strategy Isolation**: Run different strategies on separate accounts
- **Compliance**: Meet regulatory requirements
- **Performance**: Increase overall trading capacity

## Part 1: Account Configuration

### Setting Up Multiple Accounts

```yaml
# configs/accounts.yaml
accounts:
  binance:
    spot:
      - account_id: "binance_spot_main"
        api_key_vault_path: "secret/exchanges/binance_spot_main"
        weight_limit: 1200
        order_limit: 10
        
      - account_id: "binance_spot_secondary"
        api_key_vault_path: "secret/exchanges/binance_spot_secondary"
        weight_limit: 1200
        order_limit: 10
        
      - account_id: "binance_spot_arbitrage"
        api_key_vault_path: "secret/exchanges/binance_spot_arbitrage"
        weight_limit: 1200
        order_limit: 10
        
    futures:
      - account_id: "binance_futures_main"
        api_key_vault_path: "secret/exchanges/binance_futures_main"
        leverage_limit: 10
        position_limit: 50
```

### Add API Keys to Vault

```bash
# Add multiple account API keys
vault kv put secret/exchanges/binance_spot_main \
    api_key="main-account-api-key" \
    api_secret="main-account-secret"

vault kv put secret/exchanges/binance_spot_secondary \
    api_key="secondary-account-api-key" \
    api_secret="secondary-account-secret"

vault kv put secret/exchanges/binance_spot_arbitrage \
    api_key="arbitrage-account-api-key" \
    api_secret="arbitrage-account-secret"
```

## Part 2: Account Management

### List Available Accounts

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    pb "github.com/your-org/mExOms/proto/gen/go/oms/v1"
    "google.golang.org/grpc"
)

func main() {
    conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer conn.Close()
    
    client := pb.NewAccountServiceClient(conn)
    ctx := context.Background()
    
    // List all accounts
    accounts, err := client.ListAccounts(ctx, &pb.ListAccountsRequest{})
    if err != nil {
        log.Fatalf("Failed to list accounts: %v", err)
    }
    
    fmt.Println("Available Accounts:")
    for _, acc := range accounts.Accounts {
        fmt.Printf("- %s (%s %s)\n", 
            acc.AccountId, acc.Exchange, acc.Market)
        fmt.Printf("  Status: %s\n", acc.Status)
        fmt.Printf("  Rate Limit: %d/%d\n", 
            acc.CurrentWeight, acc.WeightLimit)
        fmt.Printf("  Open Orders: %d/%d\n", 
            acc.OpenOrders, acc.OrderLimit)
        fmt.Println()
    }
}
```

### Get Account Balances

```go
// Get balances for all accounts
for _, acc := range accounts.Accounts {
    balances, err := client.GetAccountBalances(ctx, 
        &pb.GetAccountBalancesRequest{
            AccountId: acc.AccountId,
        })
    
    if err != nil {
        log.Printf("Error getting balance for %s: %v", 
            acc.AccountId, err)
        continue
    }
    
    fmt.Printf("Balances for %s:\n", acc.AccountId)
    for _, bal := range balances.Balances {
        if bal.Free > 0 || bal.Locked > 0 {
            fmt.Printf("  %s: %.4f free, %.4f locked\n", 
                bal.Asset, bal.Free, bal.Locked)
        }
    }
}
```

## Part 3: Trading Across Accounts

### Execute Trade on Specific Account

```go
// Trade on specific account
order := &pb.CreateOrderRequest{
    AccountId:   "binance_spot_secondary",  // Specify account
    Symbol:      "ETHUSDT",
    Exchange:    "binance",
    Market:      "spot",
    Type:        pb.OrderType_LIMIT,
    Side:        pb.OrderSide_BUY,
    Quantity:    0.1,
    Price:       3000.00,
    TimeInForce: pb.TimeInForce_GTC,
}

resp, err := client.CreateOrder(ctx, order)
if err != nil {
    log.Fatalf("Failed to create order: %v", err)
}

fmt.Printf("Order placed on account %s\n", order.AccountId)
fmt.Printf("Order ID: %s\n", resp.OrderId)
```

### Automatic Account Selection

```go
// Let system choose best account based on criteria
autoOrder := &pb.CreateOrderRequest{
    // No AccountId specified - system will choose
    Symbol:      "BTCUSDT",
    Exchange:    "binance",
    Market:      "spot",
    Type:        pb.OrderType_MARKET,
    Side:        pb.OrderSide_BUY,
    Quantity:    0.001,
    
    // Account selection criteria
    AccountSelectionCriteria: &pb.AccountSelectionCriteria{
        Strategy: pb.SelectionStrategy_LOWEST_UTILIZATION,
        RequiredBalance: 100.0,  // Need at least $100 USDT
    },
}

resp, err := client.CreateOrder(ctx, autoOrder)
if err != nil {
    log.Fatalf("Failed to create order: %v", err)
}

fmt.Printf("Order placed on account: %s\n", resp.AccountId)
fmt.Printf("Selection reason: %s\n", resp.SelectionReason)
```

## Part 4: Account Rotation Strategies

### Round-Robin Distribution

```go
// Distribute orders evenly across accounts
func distributeOrdersRoundRobin(orders []*pb.CreateOrderRequest, 
    accounts []string) {
    
    for i, order := range orders {
        // Rotate through accounts
        order.AccountId = accounts[i % len(accounts)]
        
        resp, err := client.CreateOrder(ctx, order)
        if err != nil {
            log.Printf("Failed on account %s: %v", 
                order.AccountId, err)
            // Try next account
            continue
        }
        
        fmt.Printf("Order %d placed on %s\n", i, order.AccountId)
    }
}
```

### Load-Based Distribution

```go
// Distribute based on account load
func distributeByLoad(order *pb.CreateOrderRequest) (string, error) {
    // Get account metrics
    metrics, err := client.GetAccountMetrics(ctx, 
        &pb.GetAccountMetricsRequest{
            Exchange: order.Exchange,
            Market:   order.Market,
        })
    if err != nil {
        return "", err
    }
    
    // Find account with lowest utilization
    var bestAccount string
    lowestUtil := float32(100.0)
    
    for _, m := range metrics.Metrics {
        utilization := float32(m.CurrentWeight) / float32(m.WeightLimit)
        if utilization < lowestUtil && m.Status == "active" {
            lowestUtil = utilization
            bestAccount = m.AccountId
        }
    }
    
    order.AccountId = bestAccount
    return bestAccount, nil
}
```

### Balance-Based Distribution

```go
// Choose account with highest balance
func selectByBalance(asset string, minRequired float64) (string, error) {
    accounts, err := client.ListAccounts(ctx, &pb.ListAccountsRequest{
        WithBalances: true,
    })
    if err != nil {
        return "", err
    }
    
    var bestAccount string
    highestBalance := 0.0
    
    for _, acc := range accounts.Accounts {
        for _, bal := range acc.Balances {
            if bal.Asset == asset && bal.Free > highestBalance {
                if bal.Free >= minRequired {
                    highestBalance = bal.Free
                    bestAccount = acc.AccountId
                }
            }
        }
    }
    
    if bestAccount == "" {
        return "", fmt.Errorf("no account with sufficient %s balance", 
            asset)
    }
    
    return bestAccount, nil
}
```

## Part 5: Parallel Trading

### Execute Orders on Multiple Accounts Simultaneously

```go
import (
    "sync"
)

// Trade same strategy on multiple accounts
func parallelTrade(symbol string, quantity float64, 
    accountIds []string) {
    
    var wg sync.WaitGroup
    results := make(chan *pb.CreateOrderResponse, len(accountIds))
    
    for _, accountId := range accountIds {
        wg.Add(1)
        go func(accId string) {
            defer wg.Done()
            
            order := &pb.CreateOrderRequest{
                AccountId:   accId,
                Symbol:      symbol,
                Exchange:    "binance",
                Market:      "spot",
                Type:        pb.OrderType_MARKET,
                Side:        pb.OrderSide_BUY,
                Quantity:    quantity,
                TimeInForce: pb.TimeInForce_IOC,
            }
            
            resp, err := client.CreateOrder(ctx, order)
            if err != nil {
                log.Printf("Error on %s: %v", accId, err)
                return
            }
            
            results <- resp
        }(accountId)
    }
    
    // Wait for all orders to complete
    go func() {
        wg.Wait()
        close(results)
    }()
    
    // Collect results
    successCount := 0
    totalExecuted := 0.0
    
    for resp := range results {
        successCount++
        totalExecuted += resp.ExecutedQuantity
        fmt.Printf("Account %s: Order %s filled %.4f\n",
            resp.AccountId, resp.OrderId, resp.ExecutedQuantity)
    }
    
    fmt.Printf("\nSummary: %d/%d orders successful\n", 
        successCount, len(accountIds))
    fmt.Printf("Total executed: %.4f %s\n", totalExecuted, symbol)
}
```

## Part 6: Account Monitoring

### Real-time Account Health Monitoring

```go
// Monitor all accounts health
func monitorAccountHealth(done <-chan struct{}) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-done:
            return
        case <-ticker.C:
            health, err := client.GetAccountsHealth(ctx,
                &pb.GetAccountsHealthRequest{})
            if err != nil {
                log.Printf("Health check error: %v", err)
                continue
            }
            
            for _, acc := range health.Accounts {
                if acc.Health < 80 {  // Below 80% health
                    fmt.Printf("⚠️  Account %s health: %d%%\n",
                        acc.AccountId, acc.Health)
                    fmt.Printf("   Issues: %v\n", acc.Issues)
                    
                    // Take action based on issues
                    if contains(acc.Issues, "RATE_LIMIT_WARNING") {
                        // Reduce trading on this account
                        reduceAccountLoad(acc.AccountId)
                    }
                }
            }
        }
    }
}
```

### Account Performance Comparison

```go
// Compare account performance
func compareAccountPerformance(period string) {
    perf, err := client.GetAccountPerformance(ctx,
        &pb.GetAccountPerformanceRequest{
            Period: period,  // "24h", "7d", "30d"
        })
    if err != nil {
        log.Fatalf("Failed to get performance: %v", err)
    }
    
    fmt.Printf("Account Performance (%s):\n", period)
    fmt.Println("Account ID          | PnL ($)  | PnL (%) | Trades | Win Rate")
    fmt.Println("-------------------|----------|---------|--------|----------")
    
    for _, p := range perf.Performance {
        fmt.Printf("%-18s | %8.2f | %7.2f | %6d | %7.2f%%\n",
            p.AccountId,
            p.RealizedPnl,
            p.PnlPercentage,
            p.TotalTrades,
            p.WinRate)
    }
}
```

## Part 7: Advanced Multi-Account Strategies

### Cross-Account Arbitrage

```go
// Find arbitrage opportunities across accounts
func crossAccountArbitrage(symbol string) {
    // Get orderbooks from different accounts
    // (useful if accounts have different fee tiers)
    
    var bestBid, bestAsk float64
    var bidAccount, askAccount string
    
    for _, acc := range accounts {
        book, err := client.GetOrderBook(ctx,
            &pb.GetOrderBookRequest{
                AccountId: acc.AccountId,
                Symbol:    symbol,
                Limit:     5,
            })
        if err != nil {
            continue
        }
        
        if len(book.Bids) > 0 && book.Bids[0].Price > bestBid {
            bestBid = book.Bids[0].Price
            bidAccount = acc.AccountId
        }
        
        if len(book.Asks) > 0 && book.Asks[0].Price < bestAsk {
            bestAsk = book.Asks[0].Price
            askAccount = acc.AccountId
        }
    }
    
    // Check if profitable after fees
    spread := bestBid - bestAsk
    if spread > minProfitThreshold {
        // Execute arbitrage
        fmt.Printf("Arbitrage opportunity: Buy on %s @ %.2f, Sell on %s @ %.2f\n",
            askAccount, bestAsk, bidAccount, bestBid)
    }
}
```

### Account Segregation by Strategy

```go
// Assign accounts to different strategies
type AccountStrategy struct {
    AccountId string
    Strategy  string
    Config    map[string]interface{}
}

var accountStrategies = []AccountStrategy{
    {
        AccountId: "binance_spot_main",
        Strategy:  "market_making",
        Config: map[string]interface{}{
            "spread": 0.002,
            "size":   100.0,
        },
    },
    {
        AccountId: "binance_spot_secondary",
        Strategy:  "arbitrage",
        Config: map[string]interface{}{
            "min_profit": 0.001,
        },
    },
    {
        AccountId: "binance_spot_arbitrage",
        Strategy:  "dca",
        Config: map[string]interface{}{
            "interval": "1h",
            "amount":   50.0,
        },
    },
}

// Run strategies on assigned accounts
func runMultiAccountStrategies() {
    for _, as := range accountStrategies {
        go runStrategy(as.AccountId, as.Strategy, as.Config)
    }
}
```

## Part 8: Risk Management Across Accounts

### Aggregate Risk Monitoring

```go
// Monitor total exposure across all accounts
func monitorAggregateRisk() {
    risk, err := client.GetAggregateRisk(ctx,
        &pb.GetAggregateRiskRequest{})
    if err != nil {
        log.Fatalf("Failed to get risk: %v", err)
    }
    
    fmt.Println("Aggregate Risk Summary:")
    fmt.Printf("Total Exposure: $%.2f\n", risk.TotalExposure)
    fmt.Printf("Total Open Orders: %d\n", risk.TotalOpenOrders)
    fmt.Printf("Accounts at Risk: %d\n", risk.AccountsAtRisk)
    
    // Check if over limits
    if risk.TotalExposure > maxTotalExposure {
        fmt.Println("⚠️  WARNING: Total exposure exceeds limit!")
        // Trigger risk reduction
        reduceExposure(risk.TotalExposure - maxTotalExposure)
    }
}
```

### Correlated Position Management

```go
// Prevent correlated positions across accounts
func checkCorrelatedPositions(newOrder *pb.CreateOrderRequest) bool {
    positions, err := client.GetAllPositions(ctx,
        &pb.GetAllPositionsRequest{})
    if err != nil {
        return false
    }
    
    correlationLimit := 0.8
    totalCorrelatedSize := 0.0
    
    for _, pos := range positions.Positions {
        correlation := getCorrelation(newOrder.Symbol, pos.Symbol)
        if correlation > correlationLimit {
            // Consider this a correlated position
            totalCorrelatedSize += pos.NotionalValue
        }
    }
    
    // Check if adding this order would exceed correlated exposure limit
    orderNotional := newOrder.Quantity * getPrice(newOrder.Symbol)
    if totalCorrelatedSize + orderNotional > maxCorrelatedExposure {
        return false  // Reject order
    }
    
    return true
}
```

## Complete Multi-Account Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    "sync"
    "time"
    
    pb "github.com/your-org/mExOms/proto/gen/go/oms/v1"
    "google.golang.org/grpc"
)

type MultiAccountTrader struct {
    client    pb.OrderServiceClient
    accounts  []string
    ctx       context.Context
}

func NewMultiAccountTrader(conn *grpc.ClientConn) *MultiAccountTrader {
    return &MultiAccountTrader{
        client: pb.NewOrderServiceClient(conn),
        ctx:    context.Background(),
    }
}

func (m *MultiAccountTrader) Initialize() error {
    // Get all active accounts
    resp, err := m.client.ListAccounts(m.ctx, 
        &pb.ListAccountsRequest{
            Status: "active",
        })
    if err != nil {
        return err
    }
    
    for _, acc := range resp.Accounts {
        m.accounts = append(m.accounts, acc.AccountId)
    }
    
    fmt.Printf("Initialized with %d accounts\n", len(m.accounts))
    return nil
}

func (m *MultiAccountTrader) ExecuteStrategy() {
    // 1. Check aggregate risk
    if !m.checkRiskLimits() {
        log.Println("Risk limits exceeded, skipping trade")
        return
    }
    
    // 2. Select best accounts for trading
    selectedAccounts := m.selectAccounts(3)  // Use top 3 accounts
    
    // 3. Execute trades in parallel
    var wg sync.WaitGroup
    results := make(chan bool, len(selectedAccounts))
    
    for _, accountId := range selectedAccounts {
        wg.Add(1)
        go func(accId string) {
            defer wg.Done()
            
            // Place order
            order := &pb.CreateOrderRequest{
                AccountId:   accId,
                Symbol:      "BTCUSDT",
                Exchange:    "binance",
                Market:      "spot",
                Type:        pb.OrderType_LIMIT,
                Side:        pb.OrderSide_BUY,
                Quantity:    0.001,
                Price:       50000.00,
                TimeInForce: pb.TimeInForce_GTC,
            }
            
            resp, err := m.client.CreateOrder(m.ctx, order)
            if err != nil {
                log.Printf("Order failed on %s: %v", accId, err)
                results <- false
                return
            }
            
            fmt.Printf("Order %s placed on account %s\n", 
                resp.OrderId, accId)
            results <- true
        }(accountId)
    }
    
    wg.Wait()
    close(results)
    
    // 4. Analyze results
    successCount := 0
    for success := range results {
        if success {
            successCount++
        }
    }
    
    fmt.Printf("Strategy executed: %d/%d orders successful\n",
        successCount, len(selectedAccounts))
}

func (m *MultiAccountTrader) checkRiskLimits() bool {
    risk, err := m.client.GetAggregateRisk(m.ctx,
        &pb.GetAggregateRiskRequest{})
    if err != nil {
        return false
    }
    
    return risk.TotalExposure < 10000.00  // $10k limit
}

func (m *MultiAccountTrader) selectAccounts(count int) []string {
    // Select accounts based on:
    // 1. Available balance
    // 2. Current utilization
    // 3. Recent performance
    
    metrics, _ := m.client.GetAccountMetrics(m.ctx,
        &pb.GetAccountMetricsRequest{})
    
    // Sort by score and return top N
    // ... sorting logic ...
    
    if len(m.accounts) < count {
        return m.accounts
    }
    return m.accounts[:count]
}

func main() {
    conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer conn.Close()
    
    trader := NewMultiAccountTrader(conn)
    
    if err := trader.Initialize(); err != nil {
        log.Fatalf("Failed to initialize: %v", err)
    }
    
    // Run strategy every minute
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        trader.ExecuteStrategy()
    }
}
```

## Best Practices

1. **Account Isolation**: Keep strategies separate across accounts
2. **Load Balancing**: Distribute orders based on account capacity
3. **Risk Aggregation**: Monitor total exposure across all accounts
4. **Failover**: Have backup accounts ready
5. **Performance Tracking**: Compare account performance regularly
6. **Compliance**: Ensure account segregation meets regulatory requirements
7. **API Key Security**: Use unique keys per account with minimal permissions

## Troubleshooting

### Common Issues

1. **Account Selection Failures**
   - Check account status and balance
   - Verify API key permissions
   - Review selection criteria

2. **Uneven Distribution**
   - Adjust weight algorithms
   - Check account limits
   - Review rate limit usage

3. **Cross-Account Conflicts**
   - Implement proper locking mechanisms
   - Use correlation checks
   - Monitor aggregate positions

## Next Steps

- [Smart Order Routing](./smart-order-routing.md) - Optimize order execution
- [Risk Management](./risk-management.md) - Advanced risk controls
- [Performance Monitoring](./performance-monitoring.md) - Track multi-account metrics