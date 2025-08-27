# Backtesting Examples

This directory contains examples of how to use the mExOms backtesting framework to test and optimize trading strategies.

## Examples

### 1. Simple Moving Average Crossover (`sma_crossover_backtest.go`)
A classic trend-following strategy using SMA crossovers with backtesting and optimization.

### 2. Mean Reversion Strategy (`mean_reversion_backtest.go`)
Tests a mean reversion strategy using Bollinger Bands with multiple parameter sets.

### 3. Momentum Strategy (`momentum_backtest.go`)
Backtests a momentum-based strategy with RSI and volume filters.

### 4. Multi-Strategy Portfolio (`portfolio_backtest.go`)
Demonstrates how to backtest a portfolio of multiple strategies with capital allocation.

## Running the Examples

```bash
# Run SMA crossover backtest
go run examples/backtesting/sma_crossover_backtest.go

# Run with optimization
go run examples/backtesting/sma_crossover_backtest.go --optimize

# Run mean reversion backtest
go run examples/backtesting/mean_reversion_backtest.go

# Run momentum backtest
go run examples/backtesting/momentum_backtest.go

# Run portfolio backtest
go run examples/backtesting/portfolio_backtest.go
```

## Key Features Demonstrated

1. **Historical Data Loading**: How to load and prepare historical data
2. **Strategy Implementation**: Creating strategies that work with the backtesting engine
3. **Performance Analysis**: Analyzing backtest results and metrics
4. **Parameter Optimization**: Finding optimal strategy parameters
5. **Risk Management**: Implementing position sizing and stop losses
6. **Multi-Asset Testing**: Testing strategies across multiple symbols
7. **Walk-Forward Analysis**: Out-of-sample validation

## Backtest Configuration

All examples use a common configuration structure:

```go
config := backtest.BacktestConfig{
    StartTime:       time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
    EndTime:         time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
    InitialCapital:  100000.0,
    Symbols:         []string{"BTCUSDT", "ETHUSDT"},
    DataPath:        "./data/historical",
    CommissionRate:  0.001,
    SlippageModel:   backtest.ModelLinear,
}
```

## Performance Metrics

The backtesting framework provides comprehensive metrics:

- Total Return
- Annualized Return
- Sharpe Ratio
- Sortino Ratio
- Maximum Drawdown
- Calmar Ratio
- Win Rate
- Profit Factor
- Value at Risk (VaR)
- Conditional VaR (CVaR)

## Optimization Methods

Examples demonstrate various optimization approaches:

1. **Grid Search**: Exhaustive parameter search
2. **Random Search**: Efficient exploration of parameter space
3. **Genetic Algorithm**: Evolution-based optimization
4. **Bayesian Optimization**: Smart parameter selection

## Best Practices

1. Always test with realistic slippage and commission models
2. Use walk-forward analysis to avoid overfitting
3. Test across different market conditions
4. Monitor for look-ahead bias in your strategies
5. Validate results with out-of-sample data