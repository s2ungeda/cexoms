# Order Router Architecture

Detailed documentation of smart order routing algorithms and execution strategies.

## Overview

The Order Router is responsible for determining the optimal execution venue for each order, implementing smart routing algorithms to achieve best execution across multiple exchanges.

## Design Principles

### 1. Best Execution
- Price optimization across venues
- Fee minimization
- Slippage reduction
- Execution speed consideration

### 2. Intelligent Routing
- Real-time market analysis
- Historical performance tracking
- Machine learning for route prediction
- Dynamic venue selection

### 3. Risk Management
- Venue health monitoring
- Exposure limits per exchange
- Failover mechanisms
- Split order management

### 4. Performance
- Sub-millisecond routing decisions
- Parallel venue evaluation
- Cached routing tables
- Optimized algorithms

## Architecture Components

### Smart Router Core

```go
type SmartOrderRouter struct {
    exchanges      map[string]Exchange
    routingEngine  *RoutingEngine
    executionMgr   *ExecutionManager
    analytics      *RoutingAnalytics
    
    // Routing configuration
    config         *RoutingConfig
    
    // Performance tracking
    metrics        *RoutingMetrics
    
    // Circuit breaker for each venue
    breakers       map[string]*CircuitBreaker
}
```

### Routing Decision Engine

```go
type RoutingDecision struct {
    Exchange       string
    ExpectedPrice  decimal.Decimal
    ExpectedFee    decimal.Decimal
    ExpectedTime   time.Duration
    Confidence     float64
    SplitRatio     float64  // For split orders
}

type RoutingEngine struct {
    // Market data aggregator
    marketData     *MarketDataAggregator
    
    // Fee calculator
    feeCalculator  *FeeCalculator
    
    // Historical performance
    perfHistory    *PerformanceHistory
    
    // ML model for prediction
    mlPredictor    *MLRoutePredictor
}
```

## Routing Algorithms

### 1. Best Price Routing

Finds the venue with the best price considering fees:

```go
func (r *RoutingEngine) BestPriceRoute(order *Order) *RoutingDecision {
    bestScore := math.Inf(1)
    var bestRoute *RoutingDecision
    
    for exchange, book := range r.getOrderBooks(order.Symbol) {
        price := r.calculateExecutionPrice(order, book)
        fee := r.feeCalculator.Calculate(exchange, order)
        
        // Calculate total cost (negative for sells)
        totalCost := price.Mul(order.Quantity).Add(fee)
        if order.Side == SELL {
            totalCost = totalCost.Neg()
        }
        
        score := totalCost.InexactFloat64()
        if score < bestScore {
            bestScore = score
            bestRoute = &RoutingDecision{
                Exchange: exchange,
                ExpectedPrice: price,
                ExpectedFee: fee,
            }
        }
    }
    
    return bestRoute
}
```

### 2. Smart Order Splitting

Splits large orders across multiple venues:

```go
func (r *RoutingEngine) SmartSplitRoute(order *Order) []*RoutingDecision {
    // Aggregate liquidity across all venues
    aggregatedBook := r.marketData.GetAggregatedOrderBook(order.Symbol)
    
    // Calculate optimal split
    splits := r.calculateOptimalSplit(order, aggregatedBook)
    
    // Generate routing decisions
    decisions := make([]*RoutingDecision, 0, len(splits))
    for _, split := range splits {
        decisions = append(decisions, &RoutingDecision{
            Exchange:      split.Exchange,
            ExpectedPrice: split.Price,
            ExpectedFee:   split.Fee,
            SplitRatio:    split.Ratio,
        })
    }
    
    return decisions
}
```

### 3. Time-Weighted Routing

Balances execution speed vs price:

```go
func (r *RoutingEngine) TimeWeightedRoute(order *Order, urgency float64) *RoutingDecision {
    type venueScore struct {
        exchange string
        score    float64
        decision *RoutingDecision
    }
    
    venues := make([]venueScore, 0)
    
    for exchange := range r.exchanges {
        latency := r.perfHistory.GetAverageLatency(exchange)
        fillRate := r.perfHistory.GetFillRate(exchange)
        price := r.getExpectedPrice(exchange, order)
        
        // Score = price_weight * price_score + time_weight * time_score
        priceScore := r.normalizePriceScore(price, order.Side)
        timeScore := (1.0 / latency.Seconds()) * fillRate
        
        score := (1-urgency)*priceScore + urgency*timeScore
        
        venues = append(venues, venueScore{
            exchange: exchange,
            score:    score,
            decision: &RoutingDecision{
                Exchange:      exchange,
                ExpectedPrice: price,
                ExpectedTime:  latency,
            },
        })
    }
    
    // Sort by score and return best
    sort.Slice(venues, func(i, j int) bool {
        return venues[i].score > venues[j].score
    })
    
    return venues[0].decision
}
```

### 4. Arbitrage Detection

Identifies cross-exchange arbitrage opportunities:

```go
func (r *RoutingEngine) DetectArbitrage(symbol string) *ArbitrageOpportunity {
    bestBid := decimal.Zero
    bestAsk := decimal.NewFromFloat(math.MaxFloat64)
    var bidExchange, askExchange string
    
    // Find best bid and ask across all venues
    for exchange, book := range r.getOrderBooks(symbol) {
        if book.BestBid.GreaterThan(bestBid) {
            bestBid = book.BestBid
            bidExchange = exchange
        }
        if book.BestAsk.LessThan(bestAsk) {
            bestAsk = book.BestAsk
            askExchange = exchange
        }
    }
    
    // Calculate profit after fees
    buyFee := r.feeCalculator.Calculate(askExchange, BUY, symbol)
    sellFee := r.feeCalculator.Calculate(bidExchange, SELL, symbol)
    
    spread := bestBid.Sub(bestAsk)
    profit := spread.Sub(buyFee).Sub(sellFee)
    
    if profit.GreaterThan(decimal.Zero) {
        return &ArbitrageOpportunity{
            Symbol:       symbol,
            BuyExchange:  askExchange,
            SellExchange: bidExchange,
            BuyPrice:     bestAsk,
            SellPrice:    bestBid,
            ExpectedProfit: profit,
        }
    }
    
    return nil
}
```

## Execution Management

### Order Execution Flow

```go
type ExecutionManager struct {
    router         *SmartOrderRouter
    orderTracker   *OrderTracker
    fillAggregator *FillAggregator
    
    // Retry configuration
    maxRetries     int
    retryDelay     time.Duration
}

func (em *ExecutionManager) ExecuteOrder(ctx context.Context, order *Order) (*ExecutionResult, error) {
    // Get routing decision
    decision := em.router.Route(order)
    
    // Execute based on routing type
    switch decision.Type {
    case SINGLE_VENUE:
        return em.executeSingleVenue(ctx, order, decision)
    case SPLIT_ORDER:
        return em.executeSplitOrder(ctx, order, decision.Splits)
    case ICEBERG:
        return em.executeIceberg(ctx, order, decision)
    default:
        return nil, ErrInvalidRoutingType
    }
}
```

### Split Order Execution

```go
func (em *ExecutionManager) executeSplitOrder(ctx context.Context, 
    parentOrder *Order, splits []*RoutingDecision) (*ExecutionResult, error) {
    
    var wg sync.WaitGroup
    results := make(chan *PartialExecution, len(splits))
    errors := make(chan error, len(splits))
    
    // Execute splits in parallel
    for _, split := range splits {
        wg.Add(1)
        go func(decision *RoutingDecision) {
            defer wg.Done()
            
            childOrder := parentOrder.Clone()
            childOrder.Quantity = parentOrder.Quantity.Mul(decimal.NewFromFloat(decision.SplitRatio))
            
            result, err := em.executeOnVenue(ctx, childOrder, decision.Exchange)
            if err != nil {
                errors <- err
                return
            }
            results <- result
        }(split)
    }
    
    // Wait for all executions
    go func() {
        wg.Wait()
        close(results)
        close(errors)
    }()
    
    // Aggregate results
    return em.fillAggregator.Aggregate(results, errors)
}
```

## Performance Optimization

### 1. Parallel Venue Evaluation

```go
func (r *RoutingEngine) EvaluateVenuesParallel(order *Order) []*VenueScore {
    scores := make(chan *VenueScore, len(r.exchanges))
    
    var wg sync.WaitGroup
    for exchange := range r.exchanges {
        wg.Add(1)
        go func(ex string) {
            defer wg.Done()
            score := r.evaluateVenue(order, ex)
            scores <- score
        }(exchange)
    }
    
    go func() {
        wg.Wait()
        close(scores)
    }()
    
    // Collect results
    results := make([]*VenueScore, 0, len(r.exchanges))
    for score := range scores {
        results = append(results, score)
    }
    
    return results
}
```

### 2. Routing Cache

```go
type RoutingCache struct {
    cache      *lru.Cache
    ttl        time.Duration
    hitRate    atomic.Uint64
    missRate   atomic.Uint64
}

func (rc *RoutingCache) GetOrCompute(key string, 
    compute func() *RoutingDecision) *RoutingDecision {
    
    // Check cache
    if cached, ok := rc.cache.Get(key); ok {
        rc.hitRate.Add(1)
        if decision, valid := cached.(*CachedDecision); valid && !decision.IsExpired() {
            return decision.Decision
        }
    }
    
    rc.missRate.Add(1)
    
    // Compute new decision
    decision := compute()
    
    // Cache result
    rc.cache.Add(key, &CachedDecision{
        Decision:  decision,
        Timestamp: time.Now(),
        TTL:       rc.ttl,
    })
    
    return decision
}
```

## Monitoring and Analytics

### Routing Metrics

```go
type RoutingMetrics struct {
    // Performance metrics
    routingLatency    prometheus.Histogram
    executionLatency  prometheus.Histogram
    fillRate          prometheus.Gauge
    slippage         prometheus.Histogram
    
    // Venue metrics
    venueUsage       prometheus.CounterVec
    venueFillRate    prometheus.GaugeVec
    venueLatency     prometheus.HistogramVec
    
    // Algorithm metrics
    algorithmUsage   prometheus.CounterVec
    splitRatio       prometheus.Histogram
}
```

### Routing Analytics

```go
type RoutingAnalytics struct {
    // Historical performance tracking
    performanceDB    *sql.DB
    
    // Real-time analysis
    realtimeStats    *RealtimeStats
    
    // ML model updater
    modelUpdater     *ModelUpdater
}

func (ra *RoutingAnalytics) AnalyzeExecution(result *ExecutionResult) {
    // Record execution metrics
    ra.recordExecutionMetrics(result)
    
    // Update venue performance scores
    ra.updateVenueScores(result)
    
    // Train ML model with new data
    if ra.shouldUpdateModel() {
        go ra.modelUpdater.UpdateModel(result)
    }
    
    // Generate insights
    if insight := ra.generateInsight(result); insight != nil {
        ra.publishInsight(insight)
    }
}
```

## Configuration

### Routing Configuration

```yaml
routing:
  # Algorithm selection
  default_algorithm: "smart_price"
  
  # Split order thresholds
  split_threshold:
    BTC: 10.0
    ETH: 100.0
    default: 50000.0  # USD value
  
  # Venue preferences
  venue_weights:
    binance: 1.2      # Prefer Binance
    coinbase: 1.0
    kraken: 0.8       # De-prioritize Kraken
  
  # Performance thresholds
  performance:
    max_latency: 100ms
    min_fill_rate: 0.95
    max_slippage: 0.001
  
  # Risk limits
  risk:
    max_single_venue_exposure: 0.3
    max_order_value: 1000000.0
    require_split_above: 100000.0
```

---

*For risk management integration, see [Risk Management System](./risk-management.md).*