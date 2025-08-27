# Trader Guide

Complete guide for traders using mExOms for cryptocurrency trading across multiple exchanges.

## Table of Contents

1. [Quick Start](#quick-start)
2. [Authentication](#authentication)
3. [Account Setup](#account-setup)
4. [Placing Orders](#placing-orders)
5. [Position Management](#position-management)
6. [Risk Management](#risk-management)
7. [Market Data](#market-data)
8. [Trading Strategies](#trading-strategies)
9. [Performance Analytics](#performance-analytics)
10. [Troubleshooting](#troubleshooting)

## Quick Start

### 1. Get Access
```bash
# Request account from your administrator
curl -X POST https://api.mexoms.com/v1/account/request \
  -H "Content-Type: application/json" \
  -d '{"email": "trader@company.com", "role": "trader"}'
```

### 2. Authenticate
```bash
# Get your JWT token
curl -X POST https://api.mexoms.com/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "your-username", "password": "your-password"}'
```

### 3. Place Your First Order
```bash
# Buy 0.01 BTC at market price
curl -X POST https://api.mexoms.com/v1/orders \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "symbol": "BTCUSDT",
    "side": "BUY",
    "type": "MARKET",
    "quantity": "0.01"
  }'
```

## Authentication

### JWT Tokens (Recommended for UI)
```python
import requests

# Login and get token
response = requests.post('https://api.mexoms.com/v1/auth/login', json={
    'username': 'your-username',
    'password': 'your-password',
    'mfa_code': '123456'  # If MFA enabled
})

token = response.json()['access_token']

# Use token for subsequent requests
headers = {'Authorization': f'Bearer {token}'}
```

### API Keys (Recommended for Bots)
```python
# Create API key via web interface or API
api_key = "ak_1234567890abcdef"
api_secret = "as_fedcba0987654321"

# Use HMAC authentication
import hmac
import hashlib
import time

timestamp = str(int(time.time() * 1000))
message = f"POST/v1/orders{timestamp}{json.dumps(order_data)}"
signature = hmac.new(
    api_secret.encode(), 
    message.encode(), 
    hashlib.sha256
).hexdigest()

headers = {
    'X-API-Key': api_key,
    'X-Timestamp': timestamp,
    'X-Signature': signature
}
```

### Multi-Factor Authentication (MFA)
For enhanced security, enable MFA:

1. **Enable MFA in your account settings**
2. **Scan QR code with authenticator app**
3. **Include MFA code in login requests**

## Account Setup

### Exchange Connections

Connect your exchange accounts to enable trading:

#### 1. Binance Setup
```python
# Add Binance account
account_config = {
    "name": "Binance Main",
    "exchange": "binance",
    "api_key": "your-binance-api-key",
    "api_secret": "your-binance-secret",
    "is_testnet": False,
    "permissions": ["SPOT", "FUTURES"]
}

response = requests.post(
    'https://api.mexoms.com/v1/accounts',
    headers=headers,
    json=account_config
)
```

#### 2. Account Verification
```python
# Verify account connection
response = requests.get(
    f'https://api.mexoms.com/v1/accounts/{account_id}/verify',
    headers=headers
)

if response.json()['status'] == 'active':
    print("Account ready for trading!")
```

### Trading Preferences

Configure your trading preferences:

```python
preferences = {
    "default_exchange": "binance",
    "risk_limits": {
        "max_position_size": "10000",
        "max_daily_loss": "1000",
        "max_drawdown": "0.1"
    },
    "notifications": {
        "order_fills": True,
        "large_moves": True,
        "risk_alerts": True
    }
}

requests.put(
    'https://api.mexoms.com/v1/users/preferences',
    headers=headers,
    json=preferences
)
```

## Placing Orders

### Order Types

#### Market Orders
```python
# Buy at current market price
market_order = {
    "symbol": "BTCUSDT",
    "side": "BUY",
    "type": "MARKET",
    "quantity": "0.01"
}

response = requests.post(
    'https://api.mexoms.com/v1/orders',
    headers=headers,
    json=market_order
)
```

#### Limit Orders
```python
# Buy at specific price or better
limit_order = {
    "symbol": "BTCUSDT",
    "side": "BUY",
    "type": "LIMIT",
    "quantity": "0.01",
    "price": "45000",
    "time_in_force": "GTC"  # Good Till Cancelled
}

response = requests.post(
    'https://api.mexoms.com/v1/orders',
    headers=headers,
    json=limit_order
)
```

#### Stop Orders
```python
# Stop loss order
stop_order = {
    "symbol": "BTCUSDT",
    "side": "SELL",
    "type": "STOP_LOSS",
    "quantity": "0.01",
    "stop_price": "44000"
}

# Take profit order
take_profit = {
    "symbol": "BTCUSDT",
    "side": "SELL",
    "type": "TAKE_PROFIT",
    "quantity": "0.01",
    "stop_price": "46000"
}
```

### Advanced Order Features

#### Conditional Orders
```python
# Order executes only if condition is met
conditional_order = {
    "symbol": "BTCUSDT",
    "side": "BUY",
    "type": "LIMIT",
    "quantity": "0.01",
    "price": "45000",
    "condition": {
        "type": "PRICE_ABOVE",
        "symbol": "ETHUSDT",
        "threshold": "3000"
    }
}
```

#### Iceberg Orders
```python
# Large order split into smaller visible quantities
iceberg_order = {
    "symbol": "BTCUSDT",
    "side": "BUY",
    "type": "LIMIT",
    "quantity": "1.0",
    "price": "45000",
    "iceberg_qty": "0.1"  # Only show 0.1 at a time
}
```

### Order Management

#### Check Order Status
```python
# Get order details
order_id = "order-123456"
response = requests.get(
    f'https://api.mexoms.com/v1/orders/{order_id}',
    headers=headers
)

order = response.json()
print(f"Status: {order['status']}")
print(f"Filled: {order['filled_quantity']}/{order['quantity']}")
```

#### Cancel Orders
```python
# Cancel specific order
requests.delete(
    f'https://api.mexoms.com/v1/orders/{order_id}',
    headers=headers
)

# Cancel all orders for symbol
requests.delete(
    'https://api.mexoms.com/v1/orders',
    headers=headers,
    params={"symbol": "BTCUSDT"}
)
```

#### Modify Orders
```python
# Modify existing order
modification = {
    "price": "45500",
    "quantity": "0.02"
}

requests.patch(
    f'https://api.mexoms.com/v1/orders/{order_id}',
    headers=headers,
    json=modification
)
```

## Position Management

### View Positions
```python
# Get all positions
response = requests.get(
    'https://api.mexoms.com/v1/positions',
    headers=headers
)

positions = response.json()['positions']
for pos in positions:
    print(f"{pos['symbol']}: {pos['size']} @ {pos['entry_price']}")
    print(f"P&L: {pos['unrealized_pnl']}")
```

### Position Analytics
```python
# Get detailed position metrics
response = requests.get(
    f'https://api.mexoms.com/v1/positions/{symbol}/metrics',
    headers=headers
)

metrics = response.json()
print(f"ROI: {metrics['roi_percent']}%")
print(f"Sharpe Ratio: {metrics['sharpe_ratio']}")
print(f"Max Drawdown: {metrics['max_drawdown']}%")
```

### Cross-Exchange Positions
```python
# Aggregate positions across exchanges
response = requests.get(
    'https://api.mexoms.com/v1/positions/aggregated',
    headers=headers,
    params={
        "symbol": "BTCUSDT",
        "group_by": "symbol"
    }
)

aggregated = response.json()
print(f"Net Position: {aggregated['net_size']}")
print(f"Total P&L: {aggregated['total_pnl']}")
```

## Risk Management

### Position Limits
```python
# Set position size limits
limits = {
    "symbol": "BTCUSDT",
    "max_position_value": "50000",  # $50k max
    "max_leverage": "3",
    "stop_loss_percent": "5"
}

requests.post(
    'https://api.mexoms.com/v1/risk/limits',
    headers=headers,
    json=limits
)
```

### Automatic Stop Losses
```python
# Enable automatic stop losses
auto_stop = {
    "enabled": True,
    "stop_loss_percent": "2",  # 2% stop loss
    "trailing_stop": True,
    "trailing_distance": "1"   # 1% trailing distance
}

requests.post(
    'https://api.mexoms.com/v1/risk/auto-stop',
    headers=headers,
    json=auto_stop
)
```

### Risk Alerts
```python
# Configure risk alerts
alerts = {
    "daily_loss_limit": "1000",
    "position_size_alert": "0.1",  # Alert at 10% of portfolio
    "correlation_alert": "0.8",   # Alert when positions are 80% correlated
    "margin_usage_alert": "0.7"   # Alert at 70% margin usage
}

requests.post(
    'https://api.mexoms.com/v1/risk/alerts',
    headers=headers,
    json=alerts
)
```

## Market Data

### Real-Time Prices
```python
import websocket
import json

def on_message(ws, message):
    data = json.loads(message)
    if data['type'] == 'ticker':
        ticker = data['data']
        print(f"{ticker['symbol']}: ${ticker['price']}")

def on_open(ws):
    # Subscribe to tickers
    subscribe_msg = {
        "method": "SUBSCRIBE",
        "params": ["btcusdt@ticker", "ethusdt@ticker"],
        "id": 1
    }
    ws.send(json.dumps(subscribe_msg))

ws = websocket.WebSocketApp(
    "wss://stream.mexoms.com/ws",
    on_message=on_message,
    on_open=on_open,
    header={"Authorization": f"Bearer {token}"}
)
ws.run_forever()
```

### Historical Data
```python
# Get historical candlestick data
response = requests.get(
    'https://api.mexoms.com/v1/market/klines',
    headers=headers,
    params={
        "symbol": "BTCUSDT",
        "interval": "1h",
        "limit": 100
    }
)

klines = response.json()
for kline in klines:
    print(f"Time: {kline['open_time']}, OHLC: {kline['open']}/{kline['high']}/{kline['low']}/{kline['close']}")
```

### Order Book Data
```python
# Get current order book
response = requests.get(
    'https://api.mexoms.com/v1/market/orderbook',
    headers=headers,
    params={
        "symbol": "BTCUSDT",
        "depth": 20
    }
)

book = response.json()
print("Bids:")
for bid in book['bids'][:5]:
    print(f"  ${bid['price']} x {bid['quantity']}")

print("Asks:")
for ask in book['asks'][:5]:
    print(f"  ${ask['price']} x {ask['quantity']}")
```

## Trading Strategies

### Simple Strategy Implementation
```python
class MomentumStrategy:
    def __init__(self, symbol="BTCUSDT", period=20):
        self.symbol = symbol
        self.period = period
        self.position = 0
    
    def analyze_market(self):
        # Get recent prices
        response = requests.get(
            'https://api.mexoms.com/v1/market/klines',
            headers=headers,
            params={
                "symbol": self.symbol,
                "interval": "1h",
                "limit": self.period
            }
        )
        
        klines = response.json()
        prices = [float(k['close']) for k in klines]
        
        # Simple momentum: current price vs average
        current_price = prices[-1]
        average_price = sum(prices) / len(prices)
        
        momentum = (current_price - average_price) / average_price
        return momentum
    
    def execute_strategy(self):
        momentum = self.analyze_market()
        
        if momentum > 0.02 and self.position <= 0:
            # Strong upward momentum, go long
            self.place_order("BUY", "0.01")
            self.position = 1
            
        elif momentum < -0.02 and self.position >= 0:
            # Strong downward momentum, go short or close long
            self.place_order("SELL", "0.01")
            self.position = -1
    
    def place_order(self, side, quantity):
        order = {
            "symbol": self.symbol,
            "side": side,
            "type": "MARKET",
            "quantity": quantity
        }
        
        response = requests.post(
            'https://api.mexoms.com/v1/orders',
            headers=headers,
            json=order
        )
        
        if response.status_code == 200:
            print(f"Order placed: {side} {quantity} {self.symbol}")
        else:
            print(f"Order failed: {response.text}")

# Run strategy
strategy = MomentumStrategy()
while True:
    strategy.execute_strategy()
    time.sleep(3600)  # Check every hour
```

### Strategy Backtesting
```python
# Backtest strategy on historical data
backtest_config = {
    "strategy_name": "MomentumStrategy",
    "symbol": "BTCUSDT",
    "start_date": "2024-01-01",
    "end_date": "2024-12-31",
    "initial_balance": 10000,
    "parameters": {
        "period": 20,
        "momentum_threshold": 0.02
    }
}

response = requests.post(
    'https://api.mexoms.com/v1/backtesting/run',
    headers=headers,
    json=backtest_config
)

results = response.json()
print(f"Total Return: {results['total_return']}%")
print(f"Sharpe Ratio: {results['sharpe_ratio']}")
print(f"Max Drawdown: {results['max_drawdown']}%")
print(f"Win Rate: {results['win_rate']}%")
```

## Performance Analytics

### Portfolio Performance
```python
# Get portfolio performance metrics
response = requests.get(
    'https://api.mexoms.com/v1/analytics/portfolio',
    headers=headers,
    params={
        "start_date": "2024-01-01",
        "end_date": "2024-12-31"
    }
)

performance = response.json()
print(f"Total Return: {performance['total_return']}%")
print(f"Annualized Return: {performance['annualized_return']}%")
print(f"Volatility: {performance['volatility']}%")
print(f"Sharpe Ratio: {performance['sharpe_ratio']}")
print(f"Maximum Drawdown: {performance['max_drawdown']}%")
```

### Trade Analysis
```python
# Analyze individual trades
response = requests.get(
    'https://api.mexoms.com/v1/analytics/trades',
    headers=headers,
    params={
        "symbol": "BTCUSDT",
        "limit": 100
    }
)

trades = response.json()['trades']

# Calculate statistics
winning_trades = [t for t in trades if t['pnl'] > 0]
losing_trades = [t for t in trades if t['pnl'] < 0]

win_rate = len(winning_trades) / len(trades) * 100
avg_win = sum(t['pnl'] for t in winning_trades) / len(winning_trades) if winning_trades else 0
avg_loss = sum(t['pnl'] for t in losing_trades) / len(losing_trades) if losing_trades else 0

print(f"Win Rate: {win_rate:.1f}%")
print(f"Average Win: ${avg_win:.2f}")
print(f"Average Loss: ${avg_loss:.2f}")
print(f"Profit Factor: {abs(avg_win/avg_loss) if avg_loss != 0 else 'N/A'}")
```

### Risk Metrics
```python
# Get current risk metrics
response = requests.get(
    'https://api.mexoms.com/v1/risk/metrics',
    headers=headers
)

risk = response.json()
print(f"Portfolio Value at Risk (95%): ${risk['var_95']}")
print(f"Expected Shortfall: ${risk['expected_shortfall']}")
print(f"Beta: {risk['beta']}")
print(f"Alpha: {risk['alpha']}")
```

## Troubleshooting

### Common Issues

#### 1. Order Rejected
```python
# Check order rejection reasons
if response.status_code == 400:
    error = response.json()
    if error['code'] == 'INSUFFICIENT_BALANCE':
        print("Not enough balance for order")
        # Check account balance
        balance = requests.get('https://api.mexoms.com/v1/accounts/balance', headers=headers)
        print(f"Available balance: {balance.json()}")
    elif error['code'] == 'INVALID_SYMBOL':
        print("Invalid trading symbol")
        # Get valid symbols
        symbols = requests.get('https://api.mexoms.com/v1/market/symbols', headers=headers)
        print(f"Valid symbols: {[s['symbol'] for s in symbols.json()]}")
```

#### 2. Connection Issues
```python
import time
from requests.adapters import HTTPAdapter
from urllib3.util.retry import Retry

# Implement retry logic
session = requests.Session()
retry_strategy = Retry(
    total=3,
    backoff_factor=1,
    status_forcelist=[429, 500, 502, 503, 504]
)
adapter = HTTPAdapter(max_retries=retry_strategy)
session.mount("http://", adapter)
session.mount("https://", adapter)

# Use session for requests
response = session.post('https://api.mexoms.com/v1/orders', headers=headers, json=order)
```

#### 3. Rate Limiting
```python
import time

def make_request_with_retry(url, **kwargs):
    max_retries = 5
    base_delay = 1
    
    for attempt in range(max_retries):
        response = requests.get(url, **kwargs)
        
        if response.status_code == 429:  # Rate limited
            retry_after = int(response.headers.get('Retry-After', base_delay * (2 ** attempt)))
            print(f"Rate limited. Retrying after {retry_after} seconds...")
            time.sleep(retry_after)
            continue
        
        return response
    
    raise Exception("Max retries exceeded")
```

### Performance Optimization

#### 1. Connection Pooling
```python
import requests
from requests.adapters import HTTPAdapter

session = requests.Session()
session.mount('https://', HTTPAdapter(
    pool_connections=10,
    pool_maxsize=20,
    max_retries=3
))

# Reuse session for all requests
response = session.get('https://api.mexoms.com/v1/positions', headers=headers)
```

#### 2. Bulk Operations
```python
# Submit multiple orders at once
orders = [
    {"symbol": "BTCUSDT", "side": "BUY", "type": "LIMIT", "quantity": "0.01", "price": "45000"},
    {"symbol": "ETHUSDT", "side": "BUY", "type": "LIMIT", "quantity": "0.1", "price": "3000"},
    {"symbol": "ADAUSDT", "side": "BUY", "type": "LIMIT", "quantity": "100", "price": "0.5"}
]

response = requests.post(
    'https://api.mexoms.com/v1/orders/batch',
    headers=headers,
    json={"orders": orders}
)
```

### Support Resources

- **API Status**: [status.mexoms.com](https://status.mexoms.com)
- **Rate Limits**: Check `X-RateLimit-*` headers in responses
- **Error Codes**: See [API Documentation](../api/README.md#error-codes)
- **Community**: [Discord](https://discord.gg/mexoms) | [Forum](https://community.mexoms.com)

---

*Happy Trading! 📈*