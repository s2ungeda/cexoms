# Developer Guide

Comprehensive guide for developers building applications on top of mExOms.

## Table of Contents

1. [Getting Started](#getting-started)
2. [API Setup](#api-setup)
3. [Authentication](#authentication)
4. [Core APIs](#core-apis)
5. [SDK Usage](#sdk-usage)
6. [Strategy Development](#strategy-development)
7. [WebSocket Streaming](#websocket-streaming)
8. [Testing](#testing)
9. [Deployment](#deployment)
10. [Best Practices](#best-practices)

## Getting Started

### Prerequisites

```bash
# Required tools
node --version    # Node.js 18+
python --version  # Python 3.8+
go version        # Go 1.19+
java -version     # Java 11+
```

### Development Environment

```bash
# Clone SDK repositories
git clone https://github.com/mexoms/mexoms-sdk-js.git
git clone https://github.com/mexoms/mexoms-sdk-python.git
git clone https://github.com/mexoms/mexoms-sdk-go.git
git clone https://github.com/mexoms/mexoms-sdk-java.git

# Install development dependencies
npm install -g @mexoms/cli
pip install mexoms-dev-tools
```

### Quick Start Example

```python
from mexoms import Client

# Initialize client
client = Client(
    api_key="your-api-key",
    api_secret="your-api-secret",
    environment="testnet"  # Use "production" for live trading
)

# Place a simple order
order = client.orders.create(
    symbol="BTCUSDT",
    side="BUY",
    type="MARKET",
    quantity="0.01"
)

print(f"Order created: {order.id}")
```

## API Setup

### Environment Configuration

```bash
# Development environment
export MEXOMS_API_URL="https://api-testnet.mexoms.com"
export MEXOMS_WS_URL="wss://stream-testnet.mexoms.com"

# Production environment
export MEXOMS_API_URL="https://api.mexoms.com"
export MEXOMS_WS_URL="wss://stream.mexoms.com"
```

### API Key Creation

```python
# Create API key programmatically (requires admin privileges)
import requests

api_key_config = {
    "name": "Trading Bot Key",
    "permissions": ["trading", "read_orders", "read_positions"],
    "ip_whitelist": ["203.0.113.0/24"],
    "rate_limit": {
        "requests_per_second": 10,
        "burst_limit": 50
    },
    "expires_at": "2025-12-31T23:59:59Z"
}

response = requests.post(
    'https://api.mexoms.com/v1/api-keys',
    headers={'Authorization': f'Bearer {admin_token}'},
    json=api_key_config
)

api_key = response.json()
print(f"API Key: {api_key['key']}")
print(f"Secret: {api_key['secret']}")
```

## Authentication

### HMAC Signature Authentication

```python
import hmac
import hashlib
import time
import base64
import json

def create_signature(method, endpoint, timestamp, body, secret):
    """
    Create HMAC-SHA256 signature for API requests
    """
    message = f"{method.upper()}{endpoint}{timestamp}"
    if body:
        message += json.dumps(body, separators=(',', ':'))
    
    signature = hmac.new(
        secret.encode('utf-8'),
        message.encode('utf-8'),
        hashlib.sha256
    ).hexdigest()
    
    return signature

# Example usage
method = "POST"
endpoint = "/v1/orders"
timestamp = str(int(time.time() * 1000))
body = {"symbol": "BTCUSDT", "side": "BUY", "type": "MARKET", "quantity": "0.01"}

signature = create_signature(method, endpoint, timestamp, body, api_secret)

headers = {
    'X-API-Key': api_key,
    'X-Timestamp': timestamp,
    'X-Signature': signature,
    'Content-Type': 'application/json'
}
```

### JWT Authentication (for UI applications)

```javascript
// JavaScript/TypeScript example
class MexOmsAuth {
  constructor(baseUrl) {
    this.baseUrl = baseUrl;
    this.token = localStorage.getItem('mexoms_token');
  }

  async login(username, password, mfaCode = null) {
    const response = await fetch(`${this.baseUrl}/v1/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password, mfa_code: mfaCode })
    });

    if (response.ok) {
      const data = await response.json();
      this.token = data.access_token;
      localStorage.setItem('mexoms_token', this.token);
      return true;
    }
    return false;
  }

  getAuthHeaders() {
    return {
      'Authorization': `Bearer ${this.token}`,
      'Content-Type': 'application/json'
    };
  }

  async refreshToken() {
    const response = await fetch(`${this.baseUrl}/v1/auth/refresh`, {
      method: 'POST',
      headers: this.getAuthHeaders()
    });

    if (response.ok) {
      const data = await response.json();
      this.token = data.access_token;
      localStorage.setItem('mexoms_token', this.token);
    }
  }
}
```

## Core APIs

### Order Management

```go
package main

import (
    "context"
    "fmt"
    "github.com/mexoms/mexoms-sdk-go/client"
    "github.com/mexoms/mexoms-sdk-go/types"
)

func main() {
    // Initialize client
    client := client.New(&client.Config{
        APIKey:    "your-api-key",
        APISecret: "your-api-secret",
        BaseURL:   "https://api.mexoms.com",
    })

    // Create limit order
    order, err := client.Orders.Create(context.Background(), &types.OrderRequest{
        Symbol:      "BTCUSDT",
        Side:        types.SideBuy,
        Type:        types.OrderTypeLimit,
        Quantity:    "0.01",
        Price:       "45000",
        TimeInForce: types.TimeInForceGTC,
    })
    if err != nil {
        panic(err)
    }

    fmt.Printf("Order created: %s\n", order.ID)

    // Get order status
    orderStatus, err := client.Orders.Get(context.Background(), order.ID)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Order status: %s\n", orderStatus.Status)

    // Cancel order
    err = client.Orders.Cancel(context.Background(), order.ID)
    if err != nil {
        panic(err)
    }
}
```

### Position Management

```java
import com.mexoms.sdk.MexOmsClient;
import com.mexoms.sdk.model.*;
import java.util.List;

public class PositionManager {
    private final MexOmsClient client;
    
    public PositionManager(String apiKey, String apiSecret) {
        this.client = new MexOmsClient.Builder()
            .apiKey(apiKey)
            .apiSecret(apiSecret)
            .baseUrl("https://api.mexoms.com")
            .build();
    }
    
    public void managePositions() {
        try {
            // Get all positions
            List<Position> positions = client.positions().getAll();
            
            for (Position position : positions) {
                System.out.printf("Symbol: %s, Size: %s, PnL: %s%n", 
                    position.getSymbol(), 
                    position.getSize(), 
                    position.getUnrealizedPnl());
                
                // Set stop loss if not present
                if (position.getStopLoss() == null) {
                    StopLossRequest stopLoss = StopLossRequest.builder()
                        .symbol(position.getSymbol())
                        .stopPrice(calculateStopPrice(position))
                        .build();
                    
                    client.riskManagement().setStopLoss(stopLoss);
                }
            }
            
            // Get aggregated positions across exchanges
            List<AggregatedPosition> aggregated = client.positions()
                .getAggregated("BTCUSDT");
                
            System.out.printf("Net BTC position: %s%n", 
                aggregated.get(0).getNetSize());
                
        } catch (Exception e) {
            e.printStackTrace();
        }
    }
    
    private String calculateStopPrice(Position position) {
        // 2% stop loss
        double stopPercent = 0.02;
        double entryPrice = Double.parseDouble(position.getEntryPrice());
        
        if (position.getSide() == Side.LONG) {
            return String.valueOf(entryPrice * (1 - stopPercent));
        } else {
            return String.valueOf(entryPrice * (1 + stopPercent));
        }
    }
}
```

### Market Data

```python
from mexoms import Client
import asyncio

class MarketDataHandler:
    def __init__(self, api_key, api_secret):
        self.client = Client(api_key=api_key, api_secret=api_secret)
        
    async def stream_market_data(self):
        """Stream real-time market data"""
        async with self.client.websocket.connect() as ws:
            # Subscribe to ticker updates
            await ws.subscribe([
                "btcusdt@ticker",
                "ethusdt@ticker",
                "adausdt@ticker"
            ])
            
            # Subscribe to order book updates
            await ws.subscribe([
                "btcusdt@depth20"
            ])
            
            async for message in ws:
                await self.handle_message(message)
                
    async def handle_message(self, message):
        """Handle incoming WebSocket messages"""
        if message['type'] == 'ticker':
            ticker = message['data']
            print(f"{ticker['symbol']}: ${ticker['price']} "
                  f"({ticker['price_change_percent']:+.2f}%)")
                  
        elif message['type'] == 'depth':
            depth = message['data']
            best_bid = depth['bids'][0]
            best_ask = depth['asks'][0]
            spread = float(best_ask['price']) - float(best_bid['price'])
            print(f"{depth['symbol']} spread: ${spread:.2f}")
            
    def get_historical_data(self, symbol, interval, limit=100):
        """Get historical candlestick data"""
        klines = self.client.market.get_klines(
            symbol=symbol,
            interval=interval,
            limit=limit
        )
        
        # Convert to DataFrame for analysis
        import pandas as pd
        
        df = pd.DataFrame(klines, columns=[
            'open_time', 'open', 'high', 'low', 'close', 'volume',
            'close_time', 'quote_volume', 'trades', 'taker_buy_volume',
            'taker_buy_quote_volume', 'ignore'
        ])
        
        # Convert to proper data types
        numeric_columns = ['open', 'high', 'low', 'close', 'volume']
        df[numeric_columns] = df[numeric_columns].astype(float)
        df['open_time'] = pd.to_datetime(df['open_time'], unit='ms')
        
        return df
        
# Usage
handler = MarketDataHandler("your-api-key", "your-api-secret")

# Get historical data
btc_data = handler.get_historical_data("BTCUSDT", "1h", 100)
print(btc_data.head())

# Stream real-time data
asyncio.run(handler.stream_market_data())
```

## SDK Usage

### Python SDK

```python
# Installation
# pip install mexoms-sdk

from mexoms import Client
from mexoms.exceptions import MexOmsError
from mexoms.types import OrderType, OrderSide, TimeInForce

# Initialize client with configuration
client = Client(
    api_key="your-api-key",
    api_secret="your-api-secret",
    environment="testnet",  # or "production"
    rate_limit_enabled=True,
    retry_config={
        "max_retries": 3,
        "backoff_factor": 2.0
    }
)

# Context manager for automatic cleanup
async with client:
    try:
        # Place orders with type safety
        order = await client.orders.create(
            symbol="BTCUSDT",
            side=OrderSide.BUY,
            type=OrderType.LIMIT,
            quantity="0.01",
            price="45000",
            time_in_force=TimeInForce.GTC
        )
        
        # Get positions with filtering
        positions = await client.positions.list(
            symbol="BTCUSDT",
            status="active"
        )
        
    except MexOmsError as e:
        print(f"API Error: {e.code} - {e.message}")
```

### Node.js SDK

```javascript
// Installation
// npm install @mexoms/sdk

const { MexOmsClient, OrderType, OrderSide } = require('@mexoms/sdk');

const client = new MexOmsClient({
  apiKey: 'your-api-key',
  apiSecret: 'your-api-secret',
  environment: 'testnet',
  rateLimitEnabled: true
});

async function tradingBot() {
  try {
    // Get account information
    const account = await client.accounts.getBalance();
    console.log('Account balance:', account);
    
    // Place bracket order (entry + stop loss + take profit)
    const bracketOrder = await client.orders.createBracket({
      symbol: 'BTCUSDT',
      side: OrderSide.BUY,
      quantity: '0.01',
      entryPrice: '45000',
      stopLoss: '44000',
      takeProfit: '46000'
    });
    
    console.log('Bracket order created:', bracketOrder);
    
    // Monitor order status
    const orderStream = client.orders.stream(bracketOrder.id);
    orderStream.on('update', (order) => {
      console.log(`Order ${order.id} status: ${order.status}`);
    });
    
  } catch (error) {
    console.error('Trading error:', error.message);
  }
}

tradingBot();
```

## Strategy Development

### Strategy Framework

```python
from abc import ABC, abstractmethod
from mexoms import Client
import pandas as pd
import numpy as np

class BaseStrategy(ABC):
    def __init__(self, client: Client, symbol: str, config: dict):
        self.client = client
        self.symbol = symbol
        self.config = config
        self.position = 0
        self.orders = []
        
    @abstractmethod
    async def analyze(self, data: pd.DataFrame) -> dict:
        """Analyze market data and generate signals"""
        pass
        
    @abstractmethod
    async def execute(self, signal: dict) -> None:
        """Execute trading signal"""
        pass
        
    async def run(self):
        """Main strategy loop"""
        while True:
            try:
                # Get market data
                data = await self.get_market_data()
                
                # Generate signal
                signal = await self.analyze(data)
                
                # Execute if signal is strong enough
                if abs(signal['strength']) > self.config['min_signal_strength']:
                    await self.execute(signal)
                    
                # Risk management
                await self.manage_risk()
                
                # Wait for next iteration
                await asyncio.sleep(self.config['interval'])
                
            except Exception as e:
                print(f"Strategy error: {e}")
                await asyncio.sleep(60)  # Wait before retrying
                
    async def get_market_data(self) -> pd.DataFrame:
        """Fetch and prepare market data"""
        klines = await self.client.market.get_klines(
            symbol=self.symbol,
            interval=self.config['timeframe'],
            limit=self.config['lookback_periods']
        )
        
        df = pd.DataFrame(klines)
        df['returns'] = df['close'].pct_change()
        return df
        
    async def manage_risk(self):
        """Implement risk management rules"""
        # Check position size
        if abs(self.position) > self.config['max_position_size']:
            await self.reduce_position()
            
        # Check daily loss limit
        daily_pnl = await self.get_daily_pnl()
        if daily_pnl < -self.config['daily_loss_limit']:
            await self.close_all_positions()

class MomentumStrategy(BaseStrategy):
    async def analyze(self, data: pd.DataFrame) -> dict:
        """Simple momentum strategy"""
        # Calculate indicators
        data['sma_fast'] = data['close'].rolling(self.config['fast_period']).mean()
        data['sma_slow'] = data['close'].rolling(self.config['slow_period']).mean()
        data['rsi'] = self.calculate_rsi(data['close'], self.config['rsi_period'])
        
        # Generate signal
        current = data.iloc[-1]
        prev = data.iloc[-2]
        
        signal_strength = 0
        signal_direction = 0
        
        # Moving average crossover
        if current['sma_fast'] > current['sma_slow'] and prev['sma_fast'] <= prev['sma_slow']:
            signal_strength += 0.5
            signal_direction = 1
        elif current['sma_fast'] < current['sma_slow'] and prev['sma_fast'] >= prev['sma_slow']:
            signal_strength += 0.5
            signal_direction = -1
            
        # RSI confirmation
        if signal_direction == 1 and current['rsi'] < 70:
            signal_strength += 0.3
        elif signal_direction == -1 and current['rsi'] > 30:
            signal_strength += 0.3
            
        return {
            'direction': signal_direction,
            'strength': signal_strength * signal_direction,
            'price': current['close'],
            'indicators': {
                'sma_fast': current['sma_fast'],
                'sma_slow': current['sma_slow'],
                'rsi': current['rsi']
            }
        }
        
    async def execute(self, signal: dict):
        """Execute momentum strategy"""
        direction = signal['direction']
        price = signal['price']
        
        # Calculate position size
        risk_amount = self.config['risk_per_trade']
        stop_distance = price * self.config['stop_loss_percent']
        position_size = risk_amount / stop_distance
        
        if direction == 1 and self.position <= 0:  # Go long
            if self.position < 0:
                await self.close_position()  # Close short first
                
            order = await self.client.orders.create(
                symbol=self.symbol,
                side="BUY",
                type="MARKET",
                quantity=str(position_size)
            )
            
            # Set stop loss
            stop_price = price * (1 - self.config['stop_loss_percent'])
            await self.client.orders.create(
                symbol=self.symbol,
                side="SELL",
                type="STOP_LOSS",
                quantity=str(position_size),
                stop_price=str(stop_price)
            )
            
            self.position = position_size
            
        elif direction == -1 and self.position >= 0:  # Go short
            if self.position > 0:
                await self.close_position()  # Close long first
                
            # Implementation for short selling
            pass
    
    def calculate_rsi(self, prices: pd.Series, period: int) -> pd.Series:
        """Calculate RSI indicator"""
        delta = prices.diff()
        gain = (delta.where(delta > 0, 0)).rolling(window=period).mean()
        loss = (-delta.where(delta < 0, 0)).rolling(window=period).mean()
        rs = gain / loss
        return 100 - (100 / (1 + rs))

# Usage
client = Client(api_key="your-key", api_secret="your-secret")
strategy_config = {
    'fast_period': 10,
    'slow_period': 20,
    'rsi_period': 14,
    'min_signal_strength': 0.6,
    'timeframe': '5m',
    'lookback_periods': 100,
    'max_position_size': 1000,
    'daily_loss_limit': 100,
    'risk_per_trade': 20,
    'stop_loss_percent': 0.02,
    'interval': 300
}

strategy = MomentumStrategy(client, "BTCUSDT", strategy_config)
asyncio.run(strategy.run())
```

### Backtesting Framework

```python
import pandas as pd
import numpy as np
from dataclasses import dataclass
from typing import List, Dict

@dataclass
class BacktestResult:
    total_return: float
    sharpe_ratio: float
    max_drawdown: float
    win_rate: float
    total_trades: int
    profit_factor: float
    
class Backtester:
    def __init__(self, initial_balance: float = 10000):
        self.initial_balance = initial_balance
        self.balance = initial_balance
        self.positions = []
        self.trades = []
        
    def run_backtest(self, strategy, data: pd.DataFrame, 
                    start_date: str, end_date: str) -> BacktestResult:
        """Run strategy backtest on historical data"""
        
        # Filter data by date range
        mask = (data['timestamp'] >= start_date) & (data['timestamp'] <= end_date)
        test_data = data.loc[mask].copy()
        
        # Reset strategy state
        self.balance = self.initial_balance
        self.positions = []
        self.trades = []
        
        # Run strategy on each data point
        for i in range(len(test_data)):
            current_data = test_data.iloc[:i+1]
            
            if len(current_data) > strategy.config['lookback_periods']:
                # Generate signal
                signal = strategy.analyze(current_data)
                
                # Execute if signal is strong enough
                if abs(signal['strength']) > strategy.config['min_signal_strength']:
                    self._execute_backtest_trade(signal, current_data.iloc[-1])
        
        # Close any remaining positions
        if self.positions:
            self._close_all_positions(test_data.iloc[-1])
            
        return self._calculate_metrics()
        
    def _execute_backtest_trade(self, signal: dict, current_bar: pd.Series):
        """Execute trade in backtest environment"""
        direction = signal['direction']
        price = current_bar['close']
        
        # Calculate position size (risk-based)
        risk_amount = self.balance * 0.02  # 2% risk per trade
        stop_distance = price * 0.02  # 2% stop loss
        position_size = risk_amount / stop_distance
        
        # Close existing positions first
        if self.positions:
            self._close_all_positions(current_bar)
            
        # Open new position
        if direction != 0:
            trade = {
                'entry_time': current_bar['timestamp'],
                'entry_price': price,
                'size': position_size * direction,
                'direction': direction
            }
            self.positions.append(trade)
            
    def _close_all_positions(self, current_bar: pd.Series):
        """Close all open positions"""
        exit_price = current_bar['close']
        
        for position in self.positions:
            # Calculate P&L
            pnl = (exit_price - position['entry_price']) * position['size']
            
            # Record trade
            trade = {
                'entry_time': position['entry_time'],
                'exit_time': current_bar['timestamp'],
                'entry_price': position['entry_price'],
                'exit_price': exit_price,
                'size': position['size'],
                'pnl': pnl,
                'return': pnl / (position['entry_price'] * abs(position['size']))
            }
            self.trades.append(trade)
            
            # Update balance
            self.balance += pnl
            
        self.positions = []
        
    def _calculate_metrics(self) -> BacktestResult:
        """Calculate backtest performance metrics"""
        if not self.trades:
            return BacktestResult(0, 0, 0, 0, 0, 0)
            
        df_trades = pd.DataFrame(self.trades)
        
        # Basic metrics
        total_return = (self.balance - self.initial_balance) / self.initial_balance
        total_trades = len(self.trades)
        winning_trades = df_trades[df_trades['pnl'] > 0]
        losing_trades = df_trades[df_trades['pnl'] < 0]
        
        win_rate = len(winning_trades) / total_trades if total_trades > 0 else 0
        
        # Profit factor
        total_wins = winning_trades['pnl'].sum() if len(winning_trades) > 0 else 0
        total_losses = abs(losing_trades['pnl'].sum()) if len(losing_trades) > 0 else 1
        profit_factor = total_wins / total_losses if total_losses > 0 else 0
        
        # Sharpe ratio (assuming daily returns)
        returns = df_trades['return']
        sharpe_ratio = returns.mean() / returns.std() * np.sqrt(252) if returns.std() > 0 else 0
        
        # Maximum drawdown
        cumulative_returns = (1 + returns).cumprod()
        running_max = cumulative_returns.expanding().max()
        drawdown = (cumulative_returns - running_max) / running_max
        max_drawdown = abs(drawdown.min())
        
        return BacktestResult(
            total_return=total_return,
            sharpe_ratio=sharpe_ratio,
            max_drawdown=max_drawdown,
            win_rate=win_rate,
            total_trades=total_trades,
            profit_factor=profit_factor
        )

# Usage
backtester = Backtester(initial_balance=10000)
result = backtester.run_backtest(
    strategy=strategy,
    data=historical_data,
    start_date='2024-01-01',
    end_date='2024-12-31'
)

print(f"Total Return: {result.total_return:.2%}")
print(f"Sharpe Ratio: {result.sharpe_ratio:.2f}")
print(f"Max Drawdown: {result.max_drawdown:.2%}")
print(f"Win Rate: {result.win_rate:.2%}")
print(f"Total Trades: {result.total_trades}")
print(f"Profit Factor: {result.profit_factor:.2f}")
```

## WebSocket Streaming

### Advanced WebSocket Usage

```python
import asyncio
import json
from typing import Callable, Dict, Any
from mexoms.websocket import WebSocketClient

class AdvancedStreamHandler:
    def __init__(self, api_key: str, api_secret: str):
        self.ws_client = WebSocketClient(api_key, api_secret)
        self.subscribers: Dict[str, list[Callable]] = {}
        self.data_buffer: Dict[str, list] = {}
        
    async def start(self):
        """Start WebSocket connection and message handling"""
        await self.ws_client.connect()
        
        # Start message processing task
        asyncio.create_task(self._process_messages())
        
    async def subscribe_ticker(self, symbols: list[str], callback: Callable):
        """Subscribe to ticker updates"""
        channels = [f"{symbol.lower()}@ticker" for symbol in symbols]
        await self.ws_client.subscribe(channels)
        
        for symbol in symbols:
            key = f"ticker_{symbol}"
            if key not in self.subscribers:
                self.subscribers[key] = []
            self.subscribers[key].append(callback)
            
    async def subscribe_orderbook(self, symbols: list[str], depth: int, callback: Callable):
        """Subscribe to order book updates"""
        channels = [f"{symbol.lower()}@depth{depth}" for symbol in symbols]
        await self.ws_client.subscribe(channels)
        
        for symbol in symbols:
            key = f"depth_{symbol}"
            if key not in self.subscribers:
                self.subscribers[key] = []
            self.subscribers[key].append(callback)
            
    async def subscribe_user_data(self, callback: Callable):
        """Subscribe to user data stream (orders, balances, positions)"""
        await self.ws_client.subscribe_user_stream()
        
        if 'user_data' not in self.subscribers:
            self.subscribers['user_data'] = []
        self.subscribers['user_data'].append(callback)
        
    async def _process_messages(self):
        """Process incoming WebSocket messages"""
        async for message in self.ws_client:
            try:
                await self._handle_message(message)
            except Exception as e:
                print(f"Error processing message: {e}")
                
    async def _handle_message(self, message: dict):
        """Route messages to appropriate handlers"""
        msg_type = message.get('type')
        
        if msg_type == 'ticker':
            symbol = message['data']['symbol']
            key = f"ticker_{symbol}"
            await self._notify_subscribers(key, message['data'])
            
        elif msg_type == 'depth':
            symbol = message['data']['symbol']
            key = f"depth_{symbol}"
            await self._notify_subscribers(key, message['data'])
            
        elif msg_type in ['order_update', 'balance_update', 'position_update']:
            await self._notify_subscribers('user_data', message['data'])
            
    async def _notify_subscribers(self, key: str, data: Any):
        """Notify all subscribers for a given key"""
        if key in self.subscribers:
            for callback in self.subscribers[key]:
                try:
                    if asyncio.iscoroutinefunction(callback):
                        await callback(data)
                    else:
                        callback(data)
                except Exception as e:
                    print(f"Error in subscriber callback: {e}")
                    
# Usage example
class TradingBot:
    def __init__(self):
        self.stream_handler = AdvancedStreamHandler("api-key", "api-secret")
        self.order_book = {}
        self.latest_prices = {}
        
    async def start(self):
        await self.stream_handler.start()
        
        # Subscribe to market data
        await self.stream_handler.subscribe_ticker(
            symbols=["BTCUSDT", "ETHUSDT"],
            callback=self.on_ticker_update
        )
        
        await self.stream_handler.subscribe_orderbook(
            symbols=["BTCUSDT"],
            depth=20,
            callback=self.on_orderbook_update
        )
        
        # Subscribe to user data
        await self.stream_handler.subscribe_user_data(
            callback=self.on_user_data_update
        )
        
    async def on_ticker_update(self, ticker: dict):
        """Handle ticker updates"""
        symbol = ticker['symbol']
        price = float(ticker['price'])
        change_percent = float(ticker['price_change_percent'])
        
        self.latest_prices[symbol] = price
        
        print(f"{symbol}: ${price:.2f} ({change_percent:+.2f}%)")
        
        # Check for trading opportunities
        await self.check_trading_signals(symbol, ticker)
        
    async def on_orderbook_update(self, orderbook: dict):
        """Handle order book updates"""
        symbol = orderbook['symbol']
        self.order_book[symbol] = orderbook
        
        # Calculate spread
        best_bid = float(orderbook['bids'][0]['price'])
        best_ask = float(orderbook['asks'][0]['price'])
        spread = best_ask - best_bid
        spread_percent = (spread / best_bid) * 100
        
        if spread_percent > 0.1:  # Alert on wide spreads
            print(f"Wide spread on {symbol}: {spread_percent:.3f}%")
            
    async def on_user_data_update(self, data: dict):
        """Handle user data updates"""
        if data['type'] == 'order_update':
            order = data['order']
            print(f"Order update: {order['id']} - {order['status']}")
            
            if order['status'] == 'FILLED':
                await self.handle_order_filled(order)
                
        elif data['type'] == 'position_update':
            position = data['position']
            print(f"Position update: {position['symbol']} - {position['size']}")
            
    async def check_trading_signals(self, symbol: str, ticker: dict):
        """Check for trading opportunities"""
        # Implement your trading logic here
        price_change = float(ticker['price_change_percent'])
        
        # Example: Buy on significant dips
        if price_change < -5:  # 5% drop
            print(f"Buying opportunity on {symbol}: {price_change}% drop")
            # await self.place_buy_order(symbol)
            
    async def handle_order_filled(self, order: dict):
        """Handle filled orders"""
        print(f"Order filled: {order['symbol']} {order['side']} {order['quantity']} @ {order['price']}")
        
        # Set stop loss for buy orders
        if order['side'] == 'BUY':
            stop_price = float(order['price']) * 0.98  # 2% stop loss
            # await self.place_stop_loss(order['symbol'], order['quantity'], stop_price)

# Run the bot
bot = TradingBot()
asyncio.run(bot.start())
```

## Testing

### Unit Testing

```python
import pytest
import asyncio
from unittest.mock import Mock, AsyncMock, patch
from mexoms import Client
from mexoms.exceptions import MexOmsError

class TestTradingStrategy:
    @pytest.fixture
    def mock_client(self):
        client = Mock(spec=Client)
        client.orders = AsyncMock()
        client.positions = AsyncMock()
        client.market = AsyncMock()
        return client
        
    @pytest.fixture
    def strategy(self, mock_client):
        config = {
            'fast_period': 10,
            'slow_period': 20,
            'min_signal_strength': 0.6
        }
        return MomentumStrategy(mock_client, "BTCUSDT", config)
        
    @pytest.mark.asyncio
    async def test_buy_signal_generation(self, strategy, sample_data):
        """Test that strategy generates buy signals correctly"""
        # Setup test data with bullish crossover
        bullish_data = sample_data.copy()
        bullish_data.iloc[-1, bullish_data.columns.get_loc('sma_fast')] = 45100
        bullish_data.iloc[-1, bullish_data.columns.get_loc('sma_slow')] = 45000
        bullish_data.iloc[-2, bullish_data.columns.get_loc('sma_fast')] = 44900
        bullish_data.iloc[-2, bullish_data.columns.get_loc('sma_slow')] = 45000
        
        signal = await strategy.analyze(bullish_data)
        
        assert signal['direction'] == 1
        assert signal['strength'] > 0
        
    @pytest.mark.asyncio
    async def test_order_placement(self, strategy, mock_client):
        """Test that orders are placed correctly"""
        mock_client.orders.create.return_value = Mock(id="order-123")
        
        signal = {'direction': 1, 'strength': 0.8, 'price': 45000}
        await strategy.execute(signal)
        
        # Verify order was placed
        mock_client.orders.create.assert_called_once()
        call_args = mock_client.orders.create.call_args[1]
        assert call_args['symbol'] == "BTCUSDT"
        assert call_args['side'] == "BUY"
        
    @pytest.mark.asyncio
    async def test_error_handling(self, strategy, mock_client):
        """Test error handling in strategy execution"""
        mock_client.orders.create.side_effect = MexOmsError("Insufficient balance")
        
        signal = {'direction': 1, 'strength': 0.8, 'price': 45000}
        
        # Should not raise exception
        with pytest.raises(MexOmsError):
            await strategy.execute(signal)
            
    def test_risk_calculations(self, strategy):
        """Test risk management calculations"""
        # Test position sizing
        account_balance = 10000
        risk_percent = 0.02
        price = 45000
        stop_distance = price * 0.02
        
        expected_size = (account_balance * risk_percent) / stop_distance
        calculated_size = strategy.calculate_position_size(account_balance, price)
        
        assert abs(calculated_size - expected_size) < 0.001

# Integration tests
class TestAPIIntegration:
    @pytest.fixture
    def testnet_client(self):
        return Client(
            api_key="test-api-key",
            api_secret="test-api-secret",
            environment="testnet"
        )
        
    @pytest.mark.asyncio
    async def test_market_data_retrieval(self, testnet_client):
        """Test actual market data retrieval"""
        klines = await testnet_client.market.get_klines(
            symbol="BTCUSDT",
            interval="1h",
            limit=10
        )
        
        assert len(klines) == 10
        assert all('open' in kline for kline in klines)
        assert all('close' in kline for kline in klines)
        
    @pytest.mark.asyncio
    async def test_order_lifecycle(self, testnet_client):
        """Test complete order lifecycle"""
        # Place order
        order = await testnet_client.orders.create(
            symbol="BTCUSDT",
            side="BUY",
            type="LIMIT",
            quantity="0.001",
            price="30000"  # Far from market price
        )
        
        assert order.id is not None
        assert order.status == "NEW"
        
        # Get order status
        order_status = await testnet_client.orders.get(order.id)
        assert order_status.id == order.id
        
        # Cancel order
        await testnet_client.orders.cancel(order.id)
        
        # Verify cancellation
        cancelled_order = await testnet_client.orders.get(order.id)
        assert cancelled_order.status == "CANCELLED"

# Load testing
class TestPerformance:
    @pytest.mark.asyncio
    async def test_concurrent_requests(self, testnet_client):
        """Test handling of concurrent API requests"""
        async def get_ticker():
            return await testnet_client.market.get_ticker("BTCUSDT")
            
        # Make 10 concurrent requests
        tasks = [get_ticker() for _ in range(10)]
        results = await asyncio.gather(*tasks)
        
        assert len(results) == 10
        assert all(r.symbol == "BTCUSDT" for r in results)
        
    @pytest.mark.asyncio
    async def test_websocket_performance(self, testnet_client):
        """Test WebSocket message handling performance"""
        messages_received = 0
        start_time = asyncio.get_event_loop().time()
        
        async def message_handler(message):
            nonlocal messages_received
            messages_received += 1
            
        async with testnet_client.websocket.connect() as ws:
            await ws.subscribe(["btcusdt@ticker"])
            
            # Collect messages for 10 seconds
            async for message in ws:
                await message_handler(message)
                
                if asyncio.get_event_loop().time() - start_time > 10:
                    break
                    
        # Should receive at least 1 message per second
        assert messages_received >= 5
        
# Run tests
if __name__ == "__main__":
    pytest.main(["-v", "test_trading_strategy.py"])
```

### Mock Data Generation

```python
import pandas as pd
import numpy as np
from datetime import datetime, timedelta

class MockDataGenerator:
    @staticmethod
    def generate_ohlcv_data(symbol: str, start_date: str, end_date: str, 
                           interval: str = "1h", trend: str = "random") -> pd.DataFrame:
        """Generate realistic OHLCV data for testing"""
        start = datetime.strptime(start_date, "%Y-%m-%d")
        end = datetime.strptime(end_date, "%Y-%m-%d")
        
        # Determine interval in minutes
        interval_minutes = {
            "1m": 1, "5m": 5, "15m": 15, "30m": 30,
            "1h": 60, "4h": 240, "1d": 1440
        }[interval]
        
        # Generate timestamps
        timestamps = []
        current = start
        while current <= end:
            timestamps.append(current)
            current += timedelta(minutes=interval_minutes)
            
        num_bars = len(timestamps)
        
        # Generate price data
        base_price = 45000  # Starting price
        volatility = 0.02   # 2% volatility
        
        # Generate returns based on trend
        if trend == "bullish":
            drift = 0.0001  # Slight upward drift
        elif trend == "bearish":
            drift = -0.0001  # Slight downward drift
        else:
            drift = 0  # No drift
            
        returns = np.random.normal(drift, volatility, num_bars)
        
        # Generate prices using geometric Brownian motion
        prices = [base_price]
        for i in range(1, num_bars):
            price = prices[-1] * (1 + returns[i])
            prices.append(max(price, 0.01))  # Ensure positive prices
            
        # Generate OHLCV data
        data = []
        for i, (timestamp, close_price) in enumerate(zip(timestamps, prices)):
            # Generate realistic OHLC from close price
            volatility_factor = abs(returns[i]) * 2
            
            high = close_price * (1 + np.random.uniform(0, volatility_factor))
            low = close_price * (1 - np.random.uniform(0, volatility_factor))
            
            if i == 0:
                open_price = close_price
            else:
                open_price = prices[i-1]  # Previous close becomes current open
                
            # Ensure OHLC relationships are correct
            high = max(high, open_price, close_price)
            low = min(low, open_price, close_price)
            
            # Generate volume
            base_volume = 100
            volume = base_volume * np.random.uniform(0.5, 2.0)
            
            data.append({
                'timestamp': timestamp,
                'open': round(open_price, 2),
                'high': round(high, 2),
                'low': round(low, 2),
                'close': round(close_price, 2),
                'volume': round(volume, 2)
            })
            
        return pd.DataFrame(data)
        
    @staticmethod
    def generate_order_book(symbol: str, base_price: float, 
                          depth: int = 20) -> dict:
        """Generate realistic order book data"""
        spread_percent = 0.001  # 0.1% spread
        spread = base_price * spread_percent
        
        mid_price = base_price
        best_bid = mid_price - spread / 2
        best_ask = mid_price + spread / 2
        
        bids = []
        asks = []
        
        # Generate bid levels
        for i in range(depth):
            price = best_bid - (i * spread * 0.1)
            quantity = np.random.uniform(0.1, 10.0)
            bids.append({'price': round(price, 2), 'quantity': round(quantity, 6)})
            
        # Generate ask levels
        for i in range(depth):
            price = best_ask + (i * spread * 0.1)
            quantity = np.random.uniform(0.1, 10.0)
            asks.append({'price': round(price, 2), 'quantity': round(quantity, 6)})
            
        return {
            'symbol': symbol,
            'bids': bids,
            'asks': asks,
            'timestamp': datetime.now().isoformat()
        }
        
    @staticmethod
    def generate_trade_history(symbol: str, num_trades: int = 100) -> list:
        """Generate realistic trade history"""
        base_price = 45000
        trades = []
        
        for i in range(num_trades):
            # Random price variation
            price = base_price * (1 + np.random.uniform(-0.01, 0.01))
            quantity = np.random.uniform(0.001, 1.0)
            side = np.random.choice(['BUY', 'SELL'])
            
            trade = {
                'id': f'trade-{i}',
                'symbol': symbol,
                'price': round(price, 2),
                'quantity': round(quantity, 6),
                'side': side,
                'timestamp': (datetime.now() - timedelta(minutes=i)).isoformat()
            }
            trades.append(trade)
            
        return trades

# Usage in tests
@pytest.fixture
def sample_data():
    return MockDataGenerator.generate_ohlcv_data(
        symbol="BTCUSDT",
        start_date="2024-01-01",
        end_date="2024-01-02",
        interval="1h",
        trend="bullish"
    )

@pytest.fixture
def sample_orderbook():
    return MockDataGenerator.generate_order_book("BTCUSDT", 45000)
```

## Deployment

### Docker Deployment

```dockerfile
# Dockerfile
FROM python:3.11-slim

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Install Python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy application code
COPY . .

# Create non-root user
RUN useradd -m -u 1000 trader
RUN chown -R trader:trader /app
USER trader

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
  CMD curl -f http://localhost:8000/health || exit 1

EXPOSE 8000

CMD ["python", "-m", "uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
```

```yaml
# docker-compose.yml
version: '3.8'

services:
  trading-bot:
    build: .
    environment:
      - MEXOMS_API_KEY=${MEXOMS_API_KEY}
      - MEXOMS_API_SECRET=${MEXOMS_API_SECRET}
      - MEXOMS_ENVIRONMENT=production
      - LOG_LEVEL=INFO
    volumes:
      - ./data:/app/data
      - ./logs:/app/logs
    restart: unless-stopped
    depends_on:
      - redis
      - postgres
    
  redis:
    image: redis:7-alpine
    command: redis-server --appendonly yes
    volumes:
      - redis_data:/data
    restart: unless-stopped
    
  postgres:
    image: postgres:15
    environment:
      - POSTGRES_DB=trading_bot
      - POSTGRES_USER=trader
      - POSTGRES_PASSWORD=${DB_PASSWORD}
    volumes:
      - postgres_data:/var/lib/postgresql/data
    restart: unless-stopped
    
  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./monitoring/prometheus.yml:/etc/prometheus/prometheus.yml
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
    
  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=${GRAFANA_PASSWORD}
    volumes:
      - grafana_data:/var/lib/grafana
      - ./monitoring/dashboards:/etc/grafana/provisioning/dashboards
      
volumes:
  redis_data:
  postgres_data:
  grafana_data:
```

### Kubernetes Deployment

```yaml
# k8s-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: trading-bot
  namespace: mexoms
spec:
  replicas: 3
  selector:
    matchLabels:
      app: trading-bot
  template:
    metadata:
      labels:
        app: trading-bot
    spec:
      containers:
      - name: trading-bot
        image: your-registry/trading-bot:latest
        ports:
        - containerPort: 8000
        env:
        - name: MEXOMS_API_KEY
          valueFrom:
            secretKeyRef:
              name: mexoms-secrets
              key: api-key
        - name: MEXOMS_API_SECRET
          valueFrom:
            secretKeyRef:
              name: mexoms-secrets
              key: api-secret
        resources:
          requests:
            memory: "512Mi"
            cpu: "250m"
          limits:
            memory: "1Gi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8000
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8000
          initialDelaySeconds: 5
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: trading-bot-service
  namespace: mexoms
spec:
  selector:
    app: trading-bot
  ports:
  - port: 80
    targetPort: 8000
  type: LoadBalancer
```

## Best Practices

### Security Best Practices

```python
from cryptography.fernet import Fernet
import os
import json
from typing import Dict, Any

class SecureConfig:
    def __init__(self, config_file: str):
        self.config_file = config_file
        self.encryption_key = os.getenv('ENCRYPTION_KEY')
        if not self.encryption_key:
            self.encryption_key = Fernet.generate_key()
            print(f"Generated encryption key: {self.encryption_key.decode()}")
            print("Store this key securely in ENCRYPTION_KEY environment variable")
        self.fernet = Fernet(self.encryption_key)
        
    def save_config(self, config: Dict[str, Any]):
        """Save configuration with sensitive data encrypted"""
        # Encrypt sensitive fields
        encrypted_config = config.copy()
        sensitive_fields = ['api_secret', 'private_key', 'password']
        
        for field in sensitive_fields:
            if field in encrypted_config:
                value = encrypted_config[field].encode()
                encrypted_config[field] = self.fernet.encrypt(value).decode()
                
        with open(self.config_file, 'w') as f:
            json.dump(encrypted_config, f, indent=2)
            
    def load_config(self) -> Dict[str, Any]:
        """Load configuration and decrypt sensitive data"""
        with open(self.config_file, 'r') as f:
            encrypted_config = json.load(f)
            
        # Decrypt sensitive fields
        config = encrypted_config.copy()
        sensitive_fields = ['api_secret', 'private_key', 'password']
        
        for field in sensitive_fields:
            if field in config:
                encrypted_value = config[field].encode()
                decrypted_value = self.fernet.decrypt(encrypted_value).decode()
                config[field] = decrypted_value
                
        return config

# API Key rotation
class APIKeyRotator:
    def __init__(self, client):
        self.client = client
        self.rotation_interval = 86400 * 30  # 30 days
        
    async def rotate_keys_if_needed(self):
        """Check if API keys need rotation and rotate if necessary"""
        api_keys = await self.client.api_keys.list()
        
        for key_info in api_keys:
            if self.is_rotation_needed(key_info):
                await self.rotate_key(key_info)
                
    def is_rotation_needed(self, key_info: dict) -> bool:
        """Check if API key needs rotation"""
        import time
        created_time = key_info['created_at']
        current_time = time.time()
        
        return (current_time - created_time) > self.rotation_interval
        
    async def rotate_key(self, old_key_info: dict):
        """Rotate API key"""
        # Create new key with same permissions
        new_key = await self.client.api_keys.create(
            name=f"{old_key_info['name']}_rotated",
            permissions=old_key_info['permissions'],
            ip_whitelist=old_key_info['ip_whitelist']
        )
        
        # Update application configuration
        await self.update_application_config(new_key)
        
        # Disable old key after grace period
        await asyncio.sleep(300)  # 5 minute grace period
        await self.client.api_keys.disable(old_key_info['id'])
        
        print(f"API key {old_key_info['id']} rotated successfully")
```

### Error Handling and Resilience

```python
import asyncio
import logging
from typing import Callable, Any
from functools import wraps
from mexoms.exceptions import MexOmsError, RateLimitError, NetworkError

class ResilientTrading:
    def __init__(self, max_retries: int = 3, base_delay: float = 1.0):
        self.max_retries = max_retries
        self.base_delay = base_delay
        self.logger = logging.getLogger(__name__)
        
    def retry_on_failure(self, exceptions=(MexOmsError,)):
        """Decorator for retrying operations on failure"""
        def decorator(func: Callable) -> Callable:
            @wraps(func)
            async def wrapper(*args, **kwargs) -> Any:
                last_exception = None
                
                for attempt in range(self.max_retries):
                    try:
                        return await func(*args, **kwargs)
                    except exceptions as e:
                        last_exception = e
                        
                        if isinstance(e, RateLimitError):
                            # Respect rate limit
                            delay = e.retry_after or (self.base_delay * (2 ** attempt))
                        else:
                            delay = self.base_delay * (2 ** attempt)
                            
                        self.logger.warning(
                            f"Attempt {attempt + 1} failed: {e}. "
                            f"Retrying in {delay}s..."
                        )
                        
                        await asyncio.sleep(delay)
                        
                raise last_exception
            return wrapper
        return decorator
        
    @retry_on_failure(exceptions=(NetworkError, MexOmsError))
    async def place_order_with_retry(self, client, order_params: dict):
        """Place order with automatic retry on failure"""
        return await client.orders.create(**order_params)
        
    async def safe_order_placement(self, client, order_params: dict):
        """Safely place order with comprehensive error handling"""
        try:
            # Validate order parameters
            self.validate_order_params(order_params)
            
            # Check account balance
            await self.verify_sufficient_balance(client, order_params)
            
            # Place order
            order = await self.place_order_with_retry(client, order_params)
            
            self.logger.info(f"Order placed successfully: {order.id}")
            return order
            
        except MexOmsError as e:
            self.logger.error(f"Order placement failed: {e}")
            
            # Handle specific error cases
            if e.code == 'INSUFFICIENT_BALANCE':
                await self.handle_insufficient_balance(e)
            elif e.code == 'INVALID_SYMBOL':
                await self.handle_invalid_symbol(e)
            else:
                await self.handle_generic_error(e)
                
            raise
            
    def validate_order_params(self, order_params: dict):
        """Validate order parameters"""
        required_fields = ['symbol', 'side', 'type', 'quantity']
        
        for field in required_fields:
            if field not in order_params:
                raise ValueError(f"Missing required field: {field}")
                
        # Validate quantity
        try:
            quantity = float(order_params['quantity'])
            if quantity <= 0:
                raise ValueError("Quantity must be positive")
        except ValueError:
            raise ValueError("Invalid quantity format")
            
    async def verify_sufficient_balance(self, client, order_params: dict):
        """Verify sufficient balance for order"""
        balance = await client.accounts.get_balance()
        # Implementation depends on specific balance structure
        
    async def handle_insufficient_balance(self, error):
        """Handle insufficient balance error"""
        self.logger.warning("Insufficient balance. Consider reducing position size.")
        # Could implement automatic position sizing adjustment
        
    async def handle_invalid_symbol(self, error):
        """Handle invalid symbol error"""
        self.logger.error(f"Invalid trading symbol: {error}")
        # Could implement symbol validation/correction
        
    async def handle_generic_error(self, error):
        """Handle generic errors"""
        self.logger.error(f"Generic trading error: {error}")
        # Could implement alerting/notification system
```

### Performance Monitoring

```python
import time
import psutil
from prometheus_client import Counter, Histogram, Gauge, start_http_server
from functools import wraps

# Prometheus metrics
order_counter = Counter('mexoms_orders_total', 'Total orders placed', ['exchange', 'symbol', 'side'])
order_latency = Histogram('mexoms_order_latency_seconds', 'Order placement latency')
active_positions = Gauge('mexoms_active_positions', 'Number of active positions', ['exchange'])
system_memory_usage = Gauge('mexoms_memory_usage_bytes', 'Memory usage in bytes')
system_cpu_usage = Gauge('mexoms_cpu_usage_percent', 'CPU usage percentage')

class PerformanceMonitor:
    def __init__(self, metrics_port: int = 8080):
        self.metrics_port = metrics_port
        self.start_metrics_server()
        
    def start_metrics_server(self):
        """Start Prometheus metrics server"""
        start_http_server(self.metrics_port)
        print(f"Metrics server started on port {self.metrics_port}")
        
    def measure_latency(self, metric: Histogram):
        """Decorator to measure function execution time"""
        def decorator(func):
            @wraps(func)
            async def wrapper(*args, **kwargs):
                start_time = time.time()
                try:
                    result = await func(*args, **kwargs)
                    return result
                finally:
                    execution_time = time.time() - start_time
                    metric.observe(execution_time)
            return wrapper
        return decorator
        
    @measure_latency(order_latency)
    async def place_monitored_order(self, client, **order_params):
        """Place order with latency monitoring"""
        order = await client.orders.create(**order_params)
        
        # Update metrics
        order_counter.labels(
            exchange=order_params.get('exchange', 'unknown'),
            symbol=order_params['symbol'],
            side=order_params['side']
        ).inc()
        
        return order
        
    async def update_system_metrics(self):
        """Update system resource metrics"""
        while True:
            # Memory usage
            memory_info = psutil.virtual_memory()
            system_memory_usage.set(memory_info.used)
            
            # CPU usage
            cpu_percent = psutil.cpu_percent(interval=1)
            system_cpu_usage.set(cpu_percent)
            
            await asyncio.sleep(30)  # Update every 30 seconds
            
    def log_performance_stats(self):
        """Log performance statistics"""
        process = psutil.Process()
        
        stats = {
            'memory_mb': process.memory_info().rss / 1024 / 1024,
            'cpu_percent': process.cpu_percent(),
            'threads': process.num_threads(),
            'open_files': len(process.open_files()),
            'connections': len(process.connections())
        }
        
        logging.info(f"Performance stats: {stats}")
        
# Usage
monitor = PerformanceMonitor()

# Start system metrics collection
asyncio.create_task(monitor.update_system_metrics())

# Use monitored order placement
order = await monitor.place_monitored_order(
    client,
    symbol="BTCUSDT",
    side="BUY",
    type="MARKET",
    quantity="0.01"
)
```

## Support Resources

### Documentation Links
- [API Reference](../api/README.md)
- [WebSocket API](../api/websocket.md)
- [Error Codes](../api/error-codes.md)
- [Rate Limits](../api/rate-limits.md)

### SDK Resources
- [Python SDK](https://github.com/mexoms/mexoms-sdk-python)
- [Node.js SDK](https://github.com/mexoms/mexoms-sdk-js)
- [Go SDK](https://github.com/mexoms/mexoms-sdk-go)
- [Java SDK](https://github.com/mexoms/mexoms-sdk-java)

### Community
- **Discord**: [mExOms Developers](https://discord.gg/mexoms-dev)
- **GitHub**: [GitHub Discussions](https://github.com/mexoms/mexoms/discussions)
- **Stack Overflow**: Tag `mexoms`
- **Reddit**: [r/mexoms](https://reddit.com/r/mexoms)

### Support
- **Developer Support**: dev-support@mexoms.com
- **API Issues**: api-issues@mexoms.com
- **Feature Requests**: features@mexoms.com
- **Bug Reports**: [GitHub Issues](https://github.com/mexoms/mexoms/issues)

---

*Happy coding! 🚀*