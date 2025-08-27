# 개발자 가이드

mExOms 기반 애플리케이션을 개발하는 개발자를 위한 완벽한 가이드입니다.

## 목차

1. [시작하기](#시작하기)
2. [API 설정](#api-설정)
3. [인증](#인증)
4. [핵심 API](#핵심-api)
5. [SDK 사용법](#sdk-사용법)
6. [전략 개발](#전략-개발)
7. [WebSocket 스트리밍](#websocket-스트리밍)
8. [테스팅](#테스팅)
9. [배포](#배포)
10. [모범 사례](#모범-사례)

## 시작하기

### 필수 조건

```bash
# 필수 도구
node --version    # Node.js 18+
python --version  # Python 3.8+
go version        # Go 1.19+
java -version     # Java 11+
```

### 개발 환경

```bash
# SDK 저장소 클론
git clone https://github.com/mexoms/mexoms-sdk-js.git
git clone https://github.com/mexoms/mexoms-sdk-python.git
git clone https://github.com/mexoms/mexoms-sdk-go.git
git clone https://github.com/mexoms/mexoms-sdk-java.git

# 개발 종속성 설치
npm install -g @mexoms/cli
pip install mexoms-dev-tools
```

### 빠른 시작 예제

```python
from mexoms import Client

# 클라이언트 초기화
client = Client(
    api_key="your-api-key",
    api_secret="your-api-secret",
    environment="testnet"  # 실거래는 "production" 사용
)

# 간단한 주문 실행
order = client.orders.create(
    symbol="BTCUSDT",
    side="BUY",
    type="MARKET",
    quantity="0.01"
)

print(f"주문 생성됨: {order.id}")
```

## API 설정

### 환경 구성

```bash
# 개발 환경
export MEXOMS_API_URL="https://api-testnet.mexoms.com"
export MEXOMS_WS_URL="wss://stream-testnet.mexoms.com"

# 프로덕션 환경
export MEXOMS_API_URL="https://api.mexoms.com"
export MEXOMS_WS_URL="wss://stream.mexoms.com"
```

### API 키 생성

```python
# 프로그래밍 방식으로 API 키 생성 (관리자 권한 필요)
import requests

api_key_config = {
    "name": "거래 봇 키",
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
print(f"API 키: {api_key['key']}")
print(f"시크릿: {api_key['secret']}")
```

## 인증

### HMAC 서명 인증

```python
import hmac
import hashlib
import time
import base64
import json

def create_signature(method, endpoint, timestamp, body, secret):
    """
    API 요청을 위한 HMAC-SHA256 서명 생성
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

# 사용 예제
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

### JWT 인증 (UI 애플리케이션용)

```javascript
// JavaScript/TypeScript 예제
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

## 핵심 API

### 주문 관리

```go
package main

import (
    "context"
    "fmt"
    "github.com/mexoms/mexoms-sdk-go/client"
    "github.com/mexoms/mexoms-sdk-go/types"
)

func main() {
    // 클라이언트 초기화
    client := client.New(&client.Config{
        APIKey:    "your-api-key",
        APISecret: "your-api-secret",
        BaseURL:   "https://api.mexoms.com",
    })

    // 지정가 주문 생성
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

    fmt.Printf("주문 생성됨: %s\n", order.ID)

    // 주문 상태 확인
    orderStatus, err := client.Orders.Get(context.Background(), order.ID)
    if err != nil {
        panic(err)
    }

    fmt.Printf("주문 상태: %s\n", orderStatus.Status)

    // 주문 취소
    err = client.Orders.Cancel(context.Background(), order.ID)
    if err != nil {
        panic(err)
    }
}
```

### 포지션 관리

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
            // 모든 포지션 조회
            List<Position> positions = client.positions().getAll();
            
            for (Position position : positions) {
                System.out.printf("심볼: %s, 크기: %s, 손익: %s%n", 
                    position.getSymbol(), 
                    position.getSize(), 
                    position.getUnrealizedPnl());
                
                // 손실 제한이 없으면 설정
                if (position.getStopLoss() == null) {
                    StopLossRequest stopLoss = StopLossRequest.builder()
                        .symbol(position.getSymbol())
                        .stopPrice(calculateStopPrice(position))
                        .build();
                    
                    client.riskManagement().setStopLoss(stopLoss);
                }
            }
            
            // 거래소별 집계 포지션 조회
            List<AggregatedPosition> aggregated = client.positions()
                .getAggregated("BTCUSDT");
                
            System.out.printf("순 BTC 포지션: %s%n", 
                aggregated.get(0).getNetSize());
                
        } catch (Exception e) {
            e.printStackTrace();
        }
    }
    
    private String calculateStopPrice(Position position) {
        // 2% 손실 제한
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

### 시장 데이터

```python
from mexoms import Client
import asyncio

class MarketDataHandler:
    def __init__(self, api_key, api_secret):
        self.client = Client(api_key=api_key, api_secret=api_secret)
        
    async def stream_market_data(self):
        """실시간 시장 데이터 스트리밍"""
        async with self.client.websocket.connect() as ws:
            # 티커 업데이트 구독
            await ws.subscribe([
                "btcusdt@ticker",
                "ethusdt@ticker",
                "adausdt@ticker"
            ])
            
            # 호가창 업데이트 구독
            await ws.subscribe([
                "btcusdt@depth20"
            ])
            
            async for message in ws:
                await self.handle_message(message)
                
    async def handle_message(self, message):
        """들어오는 WebSocket 메시지 처리"""
        if message['type'] == 'ticker':
            ticker = message['data']
            print(f"{ticker['symbol']}: ${ticker['price']} "
                  f"({ticker['price_change_percent']:+.2f}%)")
                  
        elif message['type'] == 'depth':
            depth = message['data']
            best_bid = depth['bids'][0]
            best_ask = depth['asks'][0]
            spread = float(best_ask['price']) - float(best_bid['price'])
            print(f"{depth['symbol']} 스프레드: ${spread:.2f}")
            
    def get_historical_data(self, symbol, interval, limit=100):
        """과거 캔들스틱 데이터 조회"""
        klines = self.client.market.get_klines(
            symbol=symbol,
            interval=interval,
            limit=limit
        )
        
        # 분석을 위해 DataFrame으로 변환
        import pandas as pd
        
        df = pd.DataFrame(klines, columns=[
            'open_time', 'open', 'high', 'low', 'close', 'volume',
            'close_time', 'quote_volume', 'trades', 'taker_buy_volume',
            'taker_buy_quote_volume', 'ignore'
        ])
        
        # 적절한 데이터 타입으로 변환
        numeric_columns = ['open', 'high', 'low', 'close', 'volume']
        df[numeric_columns] = df[numeric_columns].astype(float)
        df['open_time'] = pd.to_datetime(df['open_time'], unit='ms')
        
        return df
        
# 사용법
handler = MarketDataHandler("your-api-key", "your-api-secret")

# 과거 데이터 조회
btc_data = handler.get_historical_data("BTCUSDT", "1h", 100)
print(btc_data.head())

# 실시간 데이터 스트리밍
asyncio.run(handler.stream_market_data())
```

## SDK 사용법

### Python SDK

```python
# 설치
# pip install mexoms-sdk

from mexoms import Client
from mexoms.exceptions import MexOmsError
from mexoms.types import OrderType, OrderSide, TimeInForce

# 구성으로 클라이언트 초기화
client = Client(
    api_key="your-api-key",
    api_secret="your-api-secret",
    environment="testnet",  # 또는 "production"
    rate_limit_enabled=True,
    retry_config={
        "max_retries": 3,
        "backoff_factor": 2.0
    }
)

# 자동 정리를 위한 컨텍스트 매니저
async with client:
    try:
        # 타입 안전성을 갖춘 주문 실행
        order = await client.orders.create(
            symbol="BTCUSDT",
            side=OrderSide.BUY,
            type=OrderType.LIMIT,
            quantity="0.01",
            price="45000",
            time_in_force=TimeInForce.GTC
        )
        
        # 필터링을 통한 포지션 조회
        positions = await client.positions.list(
            symbol="BTCUSDT",
            status="active"
        )
        
    except MexOmsError as e:
        print(f"API 오류: {e.code} - {e.message}")
```

### Node.js SDK

```javascript
// 설치
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
    // 계정 정보 조회
    const account = await client.accounts.getBalance();
    console.log('계정 잔고:', account);
    
    // 브래킷 주문 실행 (진입 + 손실 제한 + 이익 실현)
    const bracketOrder = await client.orders.createBracket({
      symbol: 'BTCUSDT',
      side: OrderSide.BUY,
      quantity: '0.01',
      entryPrice: '45000',
      stopLoss: '44000',
      takeProfit: '46000'
    });
    
    console.log('브래킷 주문 생성됨:', bracketOrder);
    
    // 주문 상태 모니터링
    const orderStream = client.orders.stream(bracketOrder.id);
    orderStream.on('update', (order) => {
      console.log(`주문 ${order.id} 상태: ${order.status}`);
    });
    
  } catch (error) {
    console.error('거래 오류:', error.message);
  }
}

tradingBot();
```

## 전략 개발

### 전략 프레임워크

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
        """시장 데이터를 분석하고 신호 생성"""
        pass
        
    @abstractmethod
    async def execute(self, signal: dict) -> None:
        """거래 신호 실행"""
        pass
        
    async def run(self):
        """메인 전략 루프"""
        while True:
            try:
                # 시장 데이터 조회
                data = await self.get_market_data()
                
                # 신호 생성
                signal = await self.analyze(data)
                
                # 신호가 충분히 강하면 실행
                if abs(signal['strength']) > self.config['min_signal_strength']:
                    await self.execute(signal)
                    
                # 위험 관리
                await self.manage_risk()
                
                # 다음 반복을 위한 대기
                await asyncio.sleep(self.config['interval'])
                
            except Exception as e:
                print(f"전략 오류: {e}")
                await asyncio.sleep(60)  # 재시도 전 대기
                
    async def get_market_data(self) -> pd.DataFrame:
        """시장 데이터 조회 및 준비"""
        klines = await self.client.market.get_klines(
            symbol=self.symbol,
            interval=self.config['timeframe'],
            limit=self.config['lookback_periods']
        )
        
        df = pd.DataFrame(klines)
        df['returns'] = df['close'].pct_change()
        return df
        
    async def manage_risk(self):
        """위험 관리 규칙 구현"""
        # 포지션 크기 확인
        if abs(self.position) > self.config['max_position_size']:
            await self.reduce_position()
            
        # 일일 손실 제한 확인
        daily_pnl = await self.get_daily_pnl()
        if daily_pnl < -self.config['daily_loss_limit']:
            await self.close_all_positions()

class MomentumStrategy(BaseStrategy):
    async def analyze(self, data: pd.DataFrame) -> dict:
        """단순 모멘텀 전략"""
        # 지표 계산
        data['sma_fast'] = data['close'].rolling(self.config['fast_period']).mean()
        data['sma_slow'] = data['close'].rolling(self.config['slow_period']).mean()
        data['rsi'] = self.calculate_rsi(data['close'], self.config['rsi_period'])
        
        # 신호 생성
        current = data.iloc[-1]
        prev = data.iloc[-2]
        
        signal_strength = 0
        signal_direction = 0
        
        # 이동평균선 교차
        if current['sma_fast'] > current['sma_slow'] and prev['sma_fast'] <= prev['sma_slow']:
            signal_strength += 0.5
            signal_direction = 1
        elif current['sma_fast'] < current['sma_slow'] and prev['sma_fast'] >= prev['sma_slow']:
            signal_strength += 0.5
            signal_direction = -1
            
        # RSI 확인
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
        """모멘텀 전략 실행"""
        direction = signal['direction']
        price = signal['price']
        
        # 포지션 크기 계산
        risk_amount = self.config['risk_per_trade']
        stop_distance = price * self.config['stop_loss_percent']
        position_size = risk_amount / stop_distance
        
        if direction == 1 and self.position <= 0:  # 롱 포지션
            if self.position < 0:
                await self.close_position()  # 먼저 숏 포지션 청산
                
            order = await self.client.orders.create(
                symbol=self.symbol,
                side="BUY",
                type="MARKET",
                quantity=str(position_size)
            )
            
            # 손실 제한 설정
            stop_price = price * (1 - self.config['stop_loss_percent'])
            await self.client.orders.create(
                symbol=self.symbol,
                side="SELL",
                type="STOP_LOSS",
                quantity=str(position_size),
                stop_price=str(stop_price)
            )
            
            self.position = position_size
            
        elif direction == -1 and self.position >= 0:  # 숏 포지션
            if self.position > 0:
                await self.close_position()  # 먼저 롱 포지션 청산
                
            # 숏 매도 구현
            pass
    
    def calculate_rsi(self, prices: pd.Series, period: int) -> pd.Series:
        """RSI 지표 계산"""
        delta = prices.diff()
        gain = (delta.where(delta > 0, 0)).rolling(window=period).mean()
        loss = (-delta.where(delta < 0, 0)).rolling(window=period).mean()
        rs = gain / loss
        return 100 - (100 / (1 + rs))

# 사용법
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

## WebSocket 스트리밍

### 고급 WebSocket 사용법

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
        """WebSocket 연결 및 메시지 처리 시작"""
        await self.ws_client.connect()
        
        # 메시지 처리 태스크 시작
        asyncio.create_task(self._process_messages())
        
    async def subscribe_ticker(self, symbols: list[str], callback: Callable):
        """티커 업데이트 구독"""
        channels = [f"{symbol.lower()}@ticker" for symbol in symbols]
        await self.ws_client.subscribe(channels)
        
        for symbol in symbols:
            key = f"ticker_{symbol}"
            if key not in self.subscribers:
                self.subscribers[key] = []
            self.subscribers[key].append(callback)
            
    async def subscribe_orderbook(self, symbols: list[str], depth: int, callback: Callable):
        """호가창 업데이트 구독"""
        channels = [f"{symbol.lower()}@depth{depth}" for symbol in symbols]
        await self.ws_client.subscribe(channels)
        
        for symbol in symbols:
            key = f"depth_{symbol}"
            if key not in self.subscribers:
                self.subscribers[key] = []
            self.subscribers[key].append(callback)
            
    async def _process_messages(self):
        """들어오는 WebSocket 메시지 처리"""
        async for message in self.ws_client:
            try:
                await self._handle_message(message)
            except Exception as e:
                print(f"메시지 처리 오류: {e}")
                
    async def _handle_message(self, message: dict):
        """적절한 핸들러로 메시지 라우팅"""
        msg_type = message.get('type')
        
        if msg_type == 'ticker':
            symbol = message['data']['symbol']
            key = f"ticker_{symbol}"
            await self._notify_subscribers(key, message['data'])
            
        elif msg_type == 'depth':
            symbol = message['data']['symbol']
            key = f"depth_{symbol}"
            await self._notify_subscribers(key, message['data'])
            
    async def _notify_subscribers(self, key: str, data: Any):
        """주어진 키의 모든 구독자에게 알림"""
        if key in self.subscribers:
            for callback in self.subscribers[key]:
                try:
                    if asyncio.iscoroutinefunction(callback):
                        await callback(data)
                    else:
                        callback(data)
                except Exception as e:
                    print(f"구독자 콜백 오류: {e}")
                    
# 사용 예제
class TradingBot:
    def __init__(self):
        self.stream_handler = AdvancedStreamHandler("api-key", "api-secret")
        self.order_book = {}
        self.latest_prices = {}
        
    async def start(self):
        await self.stream_handler.start()
        
        # 시장 데이터 구독
        await self.stream_handler.subscribe_ticker(
            symbols=["BTCUSDT", "ETHUSDT"],
            callback=self.on_ticker_update
        )
        
        await self.stream_handler.subscribe_orderbook(
            symbols=["BTCUSDT"],
            depth=20,
            callback=self.on_orderbook_update
        )
        
    async def on_ticker_update(self, ticker: dict):
        """티커 업데이트 처리"""
        symbol = ticker['symbol']
        price = float(ticker['price'])
        change_percent = float(ticker['price_change_percent'])
        
        self.latest_prices[symbol] = price
        
        print(f"{symbol}: ${price:.2f} ({change_percent:+.2f}%)")
        
        # 거래 기회 확인
        await self.check_trading_signals(symbol, ticker)
        
    async def on_orderbook_update(self, orderbook: dict):
        """호가창 업데이트 처리"""
        symbol = orderbook['symbol']
        self.order_book[symbol] = orderbook
        
        # 스프레드 계산
        best_bid = float(orderbook['bids'][0]['price'])
        best_ask = float(orderbook['asks'][0]['price'])
        spread = best_ask - best_bid
        spread_percent = (spread / best_bid) * 100
        
        if spread_percent > 0.1:  # 와이드 스프레드 경고
            print(f"{symbol}에서 와이드 스프레드: {spread_percent:.3f}%")
            
    async def check_trading_signals(self, symbol: str, ticker: dict):
        """거래 기회 확인"""
        # 여기에 거래 로직 구현
        price_change = float(ticker['price_change_percent'])
        
        # 예: 큰 하락 시 매수 기회
        if price_change < -5:  # 5% 하락
            print(f"{symbol}에서 매수 기회: {price_change}% 하락")
            # await self.place_buy_order(symbol)

# 봇 실행
bot = TradingBot()
asyncio.run(bot.start())
```

## 테스팅

### 단위 테스트

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
        """전략이 매수 신호를 올바르게 생성하는지 테스트"""
        # 강세 교차를 가진 테스트 데이터 설정
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
        """주문이 올바르게 실행되는지 테스트"""
        mock_client.orders.create.return_value = Mock(id="order-123")
        
        signal = {'direction': 1, 'strength': 0.8, 'price': 45000}
        await strategy.execute(signal)
        
        # 주문이 실행되었는지 확인
        mock_client.orders.create.assert_called_once()
        call_args = mock_client.orders.create.call_args[1]
        assert call_args['symbol'] == "BTCUSDT"
        assert call_args['side'] == "BUY"

# 통합 테스트
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
        """실제 시장 데이터 조회 테스트"""
        klines = await testnet_client.market.get_klines(
            symbol="BTCUSDT",
            interval="1h",
            limit=10
        )
        
        assert len(klines) == 10
        assert all('open' in kline for kline in klines)
        assert all('close' in kline for kline in klines)

# 테스트 실행
if __name__ == "__main__":
    pytest.main(["-v", "test_trading_strategy.py"])
```

## 배포

### Docker 배포

```dockerfile
# Dockerfile
FROM python:3.11-slim

WORKDIR /app

# 시스템 종속성 설치
RUN apt-get update && apt-get install -y \
    build-essential \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Python 종속성 설치
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# 애플리케이션 코드 복사
COPY . .

# 비루트 사용자 생성
RUN useradd -m -u 1000 trader
RUN chown -R trader:trader /app
USER trader

# 헬스 체크
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
    
volumes:
  redis_data:
  postgres_data:
```

## 모범 사례

### 보안 모범 사례

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
            print(f"암호화 키 생성됨: {self.encryption_key.decode()}")
            print("이 키를 ENCRYPTION_KEY 환경 변수에 안전하게 저장하세요")
        self.fernet = Fernet(self.encryption_key)
        
    def save_config(self, config: Dict[str, Any]):
        """민감한 데이터를 암호화하여 구성 저장"""
        # 민감한 필드 암호화
        encrypted_config = config.copy()
        sensitive_fields = ['api_secret', 'private_key', 'password']
        
        for field in sensitive_fields:
            if field in encrypted_config:
                value = encrypted_config[field].encode()
                encrypted_config[field] = self.fernet.encrypt(value).decode()
                
        with open(self.config_file, 'w') as f:
            json.dump(encrypted_config, f, indent=2)
            
    def load_config(self) -> Dict[str, Any]:
        """구성을 로드하고 민감한 데이터 복호화"""
        with open(self.config_file, 'r') as f:
            encrypted_config = json.load(f)
            
        # 민감한 필드 복호화
        config = encrypted_config.copy()
        sensitive_fields = ['api_secret', 'private_key', 'password']
        
        for field in sensitive_fields:
            if field in config:
                encrypted_value = config[field].encode()
                decrypted_value = self.fernet.decrypt(encrypted_value).decode()
                config[field] = decrypted_value
                
        return config
```

### 성능 모니터링

```python
import time
import psutil
from prometheus_client import Counter, Histogram, Gauge, start_http_server
from functools import wraps

# Prometheus 메트릭
order_counter = Counter('mexoms_orders_total', '총 주문 수', ['exchange', 'symbol', 'side'])
order_latency = Histogram('mexoms_order_latency_seconds', '주문 실행 지연 시간')
active_positions = Gauge('mexoms_active_positions', '활성 포지션 수', ['exchange'])
system_memory_usage = Gauge('mexoms_memory_usage_bytes', '메모리 사용량(바이트)')
system_cpu_usage = Gauge('mexoms_cpu_usage_percent', 'CPU 사용률')

class PerformanceMonitor:
    def __init__(self, metrics_port: int = 8080):
        self.metrics_port = metrics_port
        self.start_metrics_server()
        
    def start_metrics_server(self):
        """Prometheus 메트릭 서버 시작"""
        start_http_server(self.metrics_port)
        print(f"메트릭 서버가 포트 {self.metrics_port}에서 시작됨")
        
    def measure_latency(self, metric: Histogram):
        """함수 실행 시간을 측정하는 데코레이터"""
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
        """지연 시간 모니터링이 있는 주문 실행"""
        order = await client.orders.create(**order_params)
        
        # 메트릭 업데이트
        order_counter.labels(
            exchange=order_params.get('exchange', 'unknown'),
            symbol=order_params['symbol'],
            side=order_params['side']
        ).inc()
        
        return order

# 사용법
monitor = PerformanceMonitor()

# 모니터링된 주문 실행
order = await monitor.place_monitored_order(
    client,
    symbol="BTCUSDT",
    side="BUY",
    type="MARKET",
    quantity="0.01"
)
```

## 지원 리소스

### 문서 링크
- [API 참조](../api/README.md)
- [WebSocket API](../api/websocket.md)
- [오류 코드](../api/error-codes.md)
- [속도 제한](../api/rate-limits.md)

### SDK 리소스
- [Python SDK](https://github.com/mexoms/mexoms-sdk-python)
- [Node.js SDK](https://github.com/mexoms/mexoms-sdk-js)
- [Go SDK](https://github.com/mexoms/mexoms-sdk-go)
- [Java SDK](https://github.com/mexoms/mexoms-sdk-java)

### 커뮤니티
- **Discord**: [mExOms 개발자](https://discord.gg/mexoms-dev)
- **GitHub**: [GitHub 토론](https://github.com/mexoms/mexoms/discussions)
- **Stack Overflow**: 태그 `mexoms`
- **Reddit**: [r/mexoms](https://reddit.com/r/mexoms)

### 지원
- **개발자 지원**: dev-support@mexoms.com
- **API 이슈**: api-issues@mexoms.com
- **기능 요청**: features@mexoms.com
- **버그 신고**: [GitHub Issues](https://github.com/mexoms/mexoms/issues)

---

*행복한 코딩! 🚀*