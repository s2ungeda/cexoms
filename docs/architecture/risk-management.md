# Risk Management System Architecture

Comprehensive documentation of the risk engine architecture and control mechanisms.

## Overview

The Risk Management System provides real-time risk assessment and control for all trading activities. It operates with sub-50μs latency to ensure risk checks don't impact trading performance.

## Core Components

### 1. Pre-Trade Risk Engine
- Real-time validation before order submission
- Position limit checks
- Buying power validation
- Leverage calculation
- Symbol restrictions

### 2. Post-Trade Risk Monitoring
- Continuous P&L tracking
- Exposure monitoring
- VaR (Value at Risk) calculation
- Stress testing
- Mark-to-market updates

### 3. Risk Limits Framework
- Account-level limits
- Symbol-level limits
- Exchange-level limits
- Strategy-level limits
- Time-based limits (daily, hourly)

### 4. Alert System
- Real-time risk alerts
- Threshold breach notifications
- Automatic position reduction
- Emergency stop triggers

## Architecture Design

### Risk Engine Architecture

```cpp
class RiskEngine {
public:
    struct RiskLimits {
        double max_position_size;
        double max_order_value;
        double max_daily_loss;
        double max_leverage;
        double position_concentration;
        double var_limit;
    };
    
    struct RiskMetrics {
        double current_exposure;
        double unrealized_pnl;
        double realized_pnl;
        double var_95;
        double var_99;
        double leverage;
        double concentration;
    };
    
private:
    // Lock-free data structures
    LockFreeHashMap<AccountId, RiskLimits> account_limits_;
    LockFreeHashMap<AccountId, RiskMetrics> account_metrics_;
    
    // Real-time position tracking
    PositionManager* position_manager_;
    
    // Market data for risk calculations
    MarketDataFeed* market_data_;
    
    // Risk calculators
    VaRCalculator var_calculator_;
    StressTestEngine stress_tester_;
};
```

### Pre-Trade Risk Checks

```cpp
RiskCheckResult RiskEngine::CheckOrder(const Order& order) {
    auto start = std::chrono::high_resolution_clock::now();
    
    // Parallel risk checks
    std::array<std::future<bool>, 6> checks = {
        std::async(std::launch::async, [&]() { return CheckPositionLimit(order); }),
        std::async(std::launch::async, [&]() { return CheckOrderValue(order); }),
        std::async(std::launch::async, [&]() { return CheckLeverage(order); }),
        std::async(std::launch::async, [&]() { return CheckConcentration(order); }),
        std::async(std::launch::async, [&]() { return CheckDailyLoss(order); }),
        std::async(std::launch::async, [&]() { return CheckVaR(order); })
    };
    
    // Collect results
    RiskCheckResult result;
    for (auto& check : checks) {
        if (!check.get()) {
            result.passed = false;
            break;
        }
    }
    
    auto latency = std::chrono::duration_cast<std::chrono::microseconds>
                  (std::chrono::high_resolution_clock::now() - start).count();
    
    // Must complete within 50μs
    if (latency > 50) {
        LogWarning("Risk check exceeded latency target: {}μs", latency);
    }
    
    return result;
}
```

## Risk Calculations

### Value at Risk (VaR)

```cpp
class VaRCalculator {
public:
    struct VaRResult {
        double var_95;
        double var_99;
        double cvar_95;  // Conditional VaR
        double cvar_99;
    };
    
    VaRResult Calculate(const Portfolio& portfolio, int horizon_days = 1) {
        // Historical VaR using 252 days of data
        auto returns = CalculateHistoricalReturns(portfolio);
        
        // Sort returns for percentile calculation
        std::sort(returns.begin(), returns.end());
        
        int n = returns.size();
        VaRResult result;
        
        // 95% VaR (5th percentile)
        int idx_95 = static_cast<int>(0.05 * n);
        result.var_95 = -returns[idx_95] * portfolio.total_value * sqrt(horizon_days);
        
        // 99% VaR (1st percentile)
        int idx_99 = static_cast<int>(0.01 * n);
        result.var_99 = -returns[idx_99] * portfolio.total_value * sqrt(horizon_days);
        
        // Conditional VaR (average of losses beyond VaR)
        result.cvar_95 = CalculateCVaR(returns, idx_95) * portfolio.total_value * sqrt(horizon_days);
        result.cvar_99 = CalculateCVaR(returns, idx_99) * portfolio.total_value * sqrt(horizon_days);
        
        return result;
    }
    
private:
    std::vector<double> CalculateHistoricalReturns(const Portfolio& portfolio) {
        // SIMD-optimized return calculation
        return simd::CalculatePortfolioReturns(portfolio, historical_prices_);
    }
};
```

### Real-time P&L Tracking

```go
type PnLTracker struct {
    positions    map[string]*Position
    marketPrices map[string]decimal.Decimal
    
    realizedPnL   decimal.Decimal
    unrealizedPnL decimal.Decimal
    
    mu           sync.RWMutex
}

func (pt *PnLTracker) UpdatePnL() {
    pt.mu.Lock()
    defer pt.mu.Unlock()
    
    unrealized := decimal.Zero
    
    for symbol, position := range pt.positions {
        if price, ok := pt.marketPrices[symbol]; ok {
            // Calculate unrealized P&L
            if position.Side == LONG {
                unrealized = unrealized.Add(
                    price.Sub(position.AvgPrice).Mul(position.Quantity)
                )
            } else {
                unrealized = unrealized.Add(
                    position.AvgPrice.Sub(price).Mul(position.Quantity)
                )
            }
        }
    }
    
    pt.unrealizedPnL = unrealized
}

func (pt *PnLTracker) OnTrade(trade *Trade) {
    pt.mu.Lock()
    defer pt.mu.Unlock()
    
    position := pt.positions[trade.Symbol]
    
    if position.Side == LONG && trade.Side == SELL ||
       position.Side == SHORT && trade.Side == BUY {
        // Closing trade - calculate realized P&L
        pnl := trade.Price.Sub(position.AvgPrice).Mul(trade.Quantity)
        if position.Side == SHORT {
            pnl = pnl.Neg()
        }
        pt.realizedPnL = pt.realizedPnL.Add(pnl)
    }
    
    // Update position
    position.Update(trade)
}
```

## Risk Monitoring

### Real-time Risk Dashboard

```go
type RiskMonitor struct {
    engine       *RiskEngine
    alertManager *AlertManager
    
    // Metrics collection
    metrics      *RiskMetrics
    
    // WebSocket for real-time updates
    wsHub        *WebSocketHub
}

func (rm *RiskMonitor) Start(ctx context.Context) {
    // Update risk metrics every 100ms
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            rm.updateMetrics()
            rm.checkAlerts()
            rm.broadcastUpdate()
        }
    }
}

func (rm *RiskMonitor) updateMetrics() {
    accounts := rm.engine.GetAllAccounts()
    
    for _, account := range accounts {
        metrics := rm.engine.CalculateRiskMetrics(account)
        
        // Update Prometheus metrics
        rm.metrics.exposure.WithLabelValues(account).Set(metrics.Exposure)
        rm.metrics.leverage.WithLabelValues(account).Set(metrics.Leverage)
        rm.metrics.var95.WithLabelValues(account).Set(metrics.VaR95)
        rm.metrics.pnl.WithLabelValues(account).Set(metrics.UnrealizedPnL)
    }
}
```

### Alert Generation

```go
type AlertManager struct {
    rules        []AlertRule
    subscribers  []AlertSubscriber
    
    // Alert history
    history      *CircularBuffer
    
    // Rate limiting
    rateLimiter  *rate.Limiter
}

type AlertRule struct {
    Name         string
    Condition    func(*RiskMetrics) bool
    Severity     AlertSeverity
    Actions      []AlertAction
    Cooldown     time.Duration
}

func (am *AlertManager) CheckAlerts(metrics *RiskMetrics) {
    for _, rule := range am.rules {
        if rule.Condition(metrics) {
            alert := &Alert{
                Rule:      rule.Name,
                Severity:  rule.Severity,
                Timestamp: time.Now(),
                Metrics:   metrics,
            }
            
            // Rate limit alerts
            if !am.rateLimiter.Allow() {
                continue
            }
            
            // Execute actions
            for _, action := range rule.Actions {
                go action.Execute(alert)
            }
            
            // Notify subscribers
            am.notifySubscribers(alert)
            
            // Record in history
            am.history.Add(alert)
        }
    }
}
```

## Risk Controls

### Position Limits

```go
type PositionLimitChecker struct {
    limits map[string]*PositionLimit
}

type PositionLimit struct {
    MaxPositionSize   decimal.Decimal
    MaxPositionValue  decimal.Decimal
    MaxPositionCount  int
    ConcentrationPct  decimal.Decimal
}

func (plc *PositionLimitChecker) CheckOrder(order *Order, 
    currentPositions map[string]*Position) error {
    
    limit := plc.getLimit(order.Symbol)
    position := currentPositions[order.Symbol]
    
    // Calculate new position after order
    newSize := position.Size
    if order.Side == position.Side {
        newSize = newSize.Add(order.Quantity)
    } else {
        newSize = newSize.Sub(order.Quantity).Abs()
    }
    
    // Check size limit
    if newSize.GreaterThan(limit.MaxPositionSize) {
        return ErrPositionSizeExceeded
    }
    
    // Check value limit
    marketPrice := GetMarketPrice(order.Symbol)
    newValue := newSize.Mul(marketPrice)
    if newValue.GreaterThan(limit.MaxPositionValue) {
        return ErrPositionValueExceeded
    }
    
    // Check concentration
    totalValue := calculateTotalValue(currentPositions)
    concentration := newValue.Div(totalValue)
    if concentration.GreaterThan(limit.ConcentrationPct) {
        return ErrConcentrationExceeded
    }
    
    return nil
}
```

### Kill Switch

```cpp
class KillSwitch {
public:
    void Activate(const std::string& reason) {
        activated_.store(true, std::memory_order_release);
        activation_time_ = std::chrono::steady_clock::now();
        reason_ = reason;
        
        // Cancel all open orders
        CancelAllOrders();
        
        // Prevent new orders
        BlockNewOrders();
        
        // Send alerts
        SendEmergencyAlert(reason);
        
        // Log activation
        LogCritical("Kill switch activated: {}", reason);
    }
    
    bool IsActive() const {
        return activated_.load(std::memory_order_acquire);
    }
    
    void Deactivate(const std::string& authorized_by) {
        if (!VerifyAuthorization(authorized_by)) {
            LogError("Unauthorized kill switch deactivation attempt");
            return;
        }
        
        activated_.store(false, std::memory_order_release);
        LogInfo("Kill switch deactivated by {}", authorized_by);
    }
    
private:
    std::atomic<bool> activated_{false};
    std::chrono::steady_clock::time_point activation_time_;
    std::string reason_;
};
```

## Stress Testing

### Market Stress Scenarios

```cpp
class StressTestEngine {
public:
    struct StressScenario {
        std::string name;
        std::map<std::string, double> price_shocks;  // symbol -> shock %
        double correlation_increase;
        double volatility_multiplier;
    };
    
    struct StressTestResult {
        double portfolio_loss;
        double worst_var;
        std::map<std::string, double> position_impacts;
        bool margin_call;
    };
    
    StressTestResult RunScenario(const Portfolio& portfolio, 
                                 const StressScenario& scenario) {
        StressTestResult result;
        
        // Apply price shocks
        for (const auto& [symbol, position] : portfolio.positions) {
            double shock = scenario.price_shocks.at(symbol);
            double shocked_price = position.mark_price * (1 + shock);
            
            double impact = (shocked_price - position.mark_price) * position.quantity;
            result.position_impacts[symbol] = impact;
            result.portfolio_loss += impact;
        }
        
        // Recalculate VaR with stressed parameters
        VaRParameters stressed_params;
        stressed_params.volatility_mult = scenario.volatility_multiplier;
        stressed_params.correlation_mult = scenario.correlation_increase;
        
        result.worst_var = var_calculator_.Calculate(portfolio, stressed_params).var_99;
        
        // Check margin requirements
        double required_margin = CalculateStressedMargin(portfolio, scenario);
        result.margin_call = (portfolio.available_balance < required_margin);
        
        return result;
    }
};
```

## Configuration

### Risk Parameters

```yaml
risk:
  # Global limits
  global:
    max_daily_loss: 100000.0
    max_leverage: 3.0
    max_var_95: 50000.0
    kill_switch_loss: 200000.0
  
  # Per-account limits
  account_limits:
    default:
      max_position_value: 1000000.0
      max_order_value: 100000.0
      max_positions: 20
      concentration_limit: 0.25
    
    institutional:
      max_position_value: 10000000.0
      max_order_value: 1000000.0
      max_positions: 100
      concentration_limit: 0.15
  
  # Symbol-specific limits
  symbol_limits:
    "BTC-USD":
      max_position_size: 100.0
      max_order_size: 10.0
      min_order_size: 0.001
    
    "ETH-USD":
      max_position_size: 1000.0
      max_order_size: 100.0
      min_order_size: 0.01
  
  # Alert thresholds
  alerts:
    - name: "High Leverage Warning"
      condition: "leverage > 2.5"
      severity: "warning"
      
    - name: "Daily Loss Alert"
      condition: "daily_pnl < -50000"
      severity: "critical"
      
    - name: "VaR Breach"
      condition: "var_95 > limit * 0.9"
      severity: "high"
```

## Performance Metrics

### Risk Check Latencies

- Position limit check: < 5μs
- Leverage calculation: < 10μs  
- VaR calculation: < 30μs (cached)
- Full pre-trade check: < 50μs (parallel)

### Monitoring Update Rates

- Position updates: Real-time (< 1ms)
- P&L calculation: 100ms intervals
- VaR recalculation: 1 second (configurable)
- Stress tests: On-demand or scheduled

---

*For integration with order routing, see [Order Router Design](./order-router.md).*