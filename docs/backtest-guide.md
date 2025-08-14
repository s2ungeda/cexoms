# Backtesting System Guide

## Overview

The backtesting system allows you to test trading strategies against historical market data. It simulates order execution, tracks portfolio performance, and generates comprehensive reports.

## Quick Start

1. Load sample historical data:
```bash
./bin/backtest -load-data
```

2. Run a simple moving average strategy:
```bash
./bin/backtest -start 2024-01-01 -end 2024-01-31 -strategy sma
```

3. Run with custom configuration:
```bash
./bin/backtest -config configs/backtest.json
```

## Command Line Options

- `-config`: Configuration file path (default: backtest.json)
- `-data`: Historical data directory (default: ./backtest_data)
- `-strategy`: Strategy name: sma, momentum (default: sma)
- `-start`: Start date in YYYY-MM-DD format
- `-end`: End date in YYYY-MM-DD format
- `-capital`: Initial capital (default: 10000)
- `-output`: Output directory for reports (default: ./backtest_results)
- `-load-data`: Load sample historical data

## Available Strategies

### Simple Moving Average (SMA)
- Trades based on SMA crossovers
- Parameters:
  - `short_period`: Short SMA period (default: 10)
  - `long_period`: Long SMA period (default: 30)

### Momentum
- Trades based on price momentum
- Parameters:
  - `lookback`: Lookback period (default: 20)
  - `threshold`: Momentum threshold (default: 0.02)

## Configuration File

Example configuration file:
```json
{
  "data_dir": "./backtest_data",
  "start_date": "2024-01-01",
  "end_date": "2024-01-31",
  "initial_capital": 10000,
  "trading_fees": 0.001,
  "output_dir": "./backtest_results",
  "strategy": {
    "name": "sma",
    "parameters": {
      "short_period": 10,
      "long_period": 30
    }
  }
}
```

## Output

The backtest generates:
1. Console summary with key metrics
2. JSON report with detailed analysis
3. HTML report with visualizations

Reports include:
- Portfolio performance metrics
- Trade statistics
- Risk analysis
- Time-based patterns
- Per-symbol analysis

## Performance Metrics

- **Total Return**: Overall profit/loss percentage
- **Sharpe Ratio**: Risk-adjusted returns
- **Max Drawdown**: Largest peak-to-trough decline
- **Win Rate**: Percentage of profitable trades
- **Profit Factor**: Ratio of gross profits to gross losses

## Adding Custom Strategies

1. Implement the `TradingStrategy` interface
2. Add strategy creation logic in `createStrategy()` function
3. Define strategy parameters in configuration

Example:
```go
type MyStrategy struct {
    // Strategy fields
}

func (s *MyStrategy) Initialize(config BacktestConfig) error {
    // Initialize strategy
}

func (s *MyStrategy) GenerateSignals(currentTime time.Time, market MarketState, portfolio *Portfolio) []*TradingSignal {
    // Generate trading signals
}

func (s *MyStrategy) Finalize() {
    // Cleanup
}
```

## Historical Data Format

Events are stored in JSONL format:
```json
{"type":"ticker","exchange":"binance","symbol":"BTCUSDT","timestamp":"2024-01-01T00:00:00Z","data":{"last_price":45000}}
{"type":"orderbook","exchange":"binance","symbol":"BTCUSDT","timestamp":"2024-01-01T00:05:00Z","data":{"bids":[[44990,1.5]],"asks":[[45010,2.0]]}}
```

## Tips

1. Start with longer time periods to validate strategy logic
2. Use appropriate data frequency for your strategy
3. Account for trading fees and slippage
4. Test with different market conditions
5. Monitor drawdown periods carefully