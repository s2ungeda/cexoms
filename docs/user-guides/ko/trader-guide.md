# 트레이더 가이드

여러 거래소에서 암호화폐 거래를 위해 mExOms를 사용하는 트레이더를 위한 완벽한 가이드입니다.

## 목차

1. [빠른 시작](#빠른-시작)
2. [인증](#인증)
3. [계정 설정](#계정-설정)
4. [주문 실행](#주문-실행)
5. [포지션 관리](#포지션-관리)
6. [위험 관리](#위험-관리)
7. [시장 데이터](#시장-데이터)
8. [거래 전략](#거래-전략)
9. [성과 분석](#성과-분석)
10. [문제 해결](#문제-해결)

## 빠른 시작

### 1. 액세스 권한 받기
```bash
# 관리자에게 계정 요청
curl -X POST https://api.mexoms.com/v1/account/request \
  -H "Content-Type: application/json" \
  -d '{"email": "trader@company.com", "role": "trader"}'
```

### 2. 인증하기
```bash
# JWT 토큰 받기
curl -X POST https://api.mexoms.com/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "사용자명", "password": "비밀번호"}'
```

### 3. 첫 주문 실행하기
```bash
# 시장가로 0.01 BTC 구매
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

## 인증

### JWT 토큰 (UI용 권장)
```python
import requests

# 로그인하여 토큰 받기
response = requests.post('https://api.mexoms.com/v1/auth/login', json={
    'username': '사용자명',
    'password': '비밀번호',
    'mfa_code': '123456'  # MFA 활성화된 경우
})

token = response.json()['access_token']

# 후속 요청에서 토큰 사용
headers = {'Authorization': f'Bearer {token}'}
```

### API 키 (봇용 권장)
```python
# 웹 인터페이스 또는 API를 통해 API 키 생성
api_key = "ak_1234567890abcdef"
api_secret = "as_fedcba0987654321"

# HMAC 인증 사용
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

### 다중 인증(MFA)
보안 강화를 위해 MFA를 활성화하세요:

1. **계정 설정에서 MFA 활성화**
2. **인증 앱으로 QR 코드 스캔**
3. **로그인 요청에 MFA 코드 포함**

## 계정 설정

### 거래소 연결

거래를 활성화하기 위해 거래소 계정을 연결하세요:

#### 1. 바이낸스 설정
```python
# 바이낸스 계정 추가
account_config = {
    "name": "바이낸스 메인",
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

#### 2. 계정 검증
```python
# 계정 연결 검증
response = requests.get(
    f'https://api.mexoms.com/v1/accounts/{account_id}/verify',
    headers=headers
)

if response.json()['status'] == 'active':
    print("계정이 거래 준비되었습니다!")
```

### 거래 설정

거래 설정을 구성하세요:

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

## 주문 실행

### 주문 유형

#### 시장가 주문
```python
# 현재 시장가로 구매
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

#### 지정가 주문
```python
# 특정 가격 또는 더 나은 가격으로 구매
limit_order = {
    "symbol": "BTCUSDT",
    "side": "BUY",
    "type": "LIMIT",
    "quantity": "0.01",
    "price": "45000",
    "time_in_force": "GTC"  # 취소될 때까지 유효
}

response = requests.post(
    'https://api.mexoms.com/v1/orders',
    headers=headers,
    json=limit_order
)
```

#### 정지 주문
```python
# 손실 제한 주문
stop_order = {
    "symbol": "BTCUSDT",
    "side": "SELL",
    "type": "STOP_LOSS",
    "quantity": "0.01",
    "stop_price": "44000"
}

# 이익 실현 주문
take_profit = {
    "symbol": "BTCUSDT",
    "side": "SELL",
    "type": "TAKE_PROFIT",
    "quantity": "0.01",
    "stop_price": "46000"
}
```

### 고급 주문 기능

#### 조건부 주문
```python
# 조건이 충족될 때만 실행되는 주문
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

#### 빙산 주문
```python
# 큰 주문을 작은 가시 수량으로 분할
iceberg_order = {
    "symbol": "BTCUSDT",
    "side": "BUY",
    "type": "LIMIT",
    "quantity": "1.0",
    "price": "45000",
    "iceberg_qty": "0.1"  # 한 번에 0.1만 표시
}
```

### 주문 관리

#### 주문 상태 확인
```python
# 주문 세부사항 조회
order_id = "order-123456"
response = requests.get(
    f'https://api.mexoms.com/v1/orders/{order_id}',
    headers=headers
)

order = response.json()
print(f"상태: {order['status']}")
print(f"체결: {order['filled_quantity']}/{order['quantity']}")
```

#### 주문 취소
```python
# 특정 주문 취소
requests.delete(
    f'https://api.mexoms.com/v1/orders/{order_id}',
    headers=headers
)

# 심볼의 모든 주문 취소
requests.delete(
    'https://api.mexoms.com/v1/orders',
    headers=headers,
    params={"symbol": "BTCUSDT"}
)
```

#### 주문 수정
```python
# 기존 주문 수정
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

## 포지션 관리

### 포지션 조회
```python
# 모든 포지션 조회
response = requests.get(
    'https://api.mexoms.com/v1/positions',
    headers=headers
)

positions = response.json()['positions']
for pos in positions:
    print(f"{pos['symbol']}: {pos['size']} @ {pos['entry_price']}")
    print(f"손익: {pos['unrealized_pnl']}")
```

### 포지션 분석
```python
# 세부 포지션 지표 조회
response = requests.get(
    f'https://api.mexoms.com/v1/positions/{symbol}/metrics',
    headers=headers
)

metrics = response.json()
print(f"ROI: {metrics['roi_percent']}%")
print(f"샤프 비율: {metrics['sharpe_ratio']}")
print(f"최대 낙폭: {metrics['max_drawdown']}%")
```

### 다중 거래소 포지션
```python
# 거래소별 포지션 집계
response = requests.get(
    'https://api.mexoms.com/v1/positions/aggregated',
    headers=headers,
    params={
        "symbol": "BTCUSDT",
        "group_by": "symbol"
    }
)

aggregated = response.json()
print(f"순 포지션: {aggregated['net_size']}")
print(f"총 손익: {aggregated['total_pnl']}")
```

## 위험 관리

### 포지션 한도
```python
# 포지션 크기 한도 설정
limits = {
    "symbol": "BTCUSDT",
    "max_position_value": "50000",  # 최대 $50k
    "max_leverage": "3",
    "stop_loss_percent": "5"
}

requests.post(
    'https://api.mexoms.com/v1/risk/limits',
    headers=headers,
    json=limits
)
```

### 자동 손실 제한
```python
# 자동 손실 제한 활성화
auto_stop = {
    "enabled": True,
    "stop_loss_percent": "2",  # 2% 손실 제한
    "trailing_stop": True,
    "trailing_distance": "1"   # 1% 추적 거리
}

requests.post(
    'https://api.mexoms.com/v1/risk/auto-stop',
    headers=headers,
    json=auto_stop
)
```

### 위험 알림
```python
# 위험 알림 구성
alerts = {
    "daily_loss_limit": "1000",
    "position_size_alert": "0.1",  # 포트폴리오의 10%에서 알림
    "correlation_alert": "0.8",   # 포지션이 80% 상관관계일 때 알림
    "margin_usage_alert": "0.7"   # 70% 증거금 사용률에서 알림
}

requests.post(
    'https://api.mexoms.com/v1/risk/alerts',
    headers=headers,
    json=alerts
)
```

## 시장 데이터

### 실시간 가격
```python
import websocket
import json

def on_message(ws, message):
    data = json.loads(message)
    if data['type'] == 'ticker':
        ticker = data['data']
        print(f"{ticker['symbol']}: ${ticker['price']}")

def on_open(ws):
    # 티커 구독
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

### 과거 데이터
```python
# 과거 캔들스틱 데이터 조회
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
    print(f"시간: {kline['open_time']}, OHLC: {kline['open']}/{kline['high']}/{kline['low']}/{kline['close']}")
```

### 호가창 데이터
```python
# 현재 호가창 조회
response = requests.get(
    'https://api.mexoms.com/v1/market/orderbook',
    headers=headers,
    params={
        "symbol": "BTCUSDT",
        "depth": 20
    }
)

book = response.json()
print("매수 호가:")
for bid in book['bids'][:5]:
    print(f"  ${bid['price']} x {bid['quantity']}")

print("매도 호가:")
for ask in book['asks'][:5]:
    print(f"  ${ask['price']} x {ask['quantity']}")
```

## 거래 전략

### 간단한 전략 구현
```python
class MomentumStrategy:
    def __init__(self, symbol="BTCUSDT", period=20):
        self.symbol = symbol
        self.period = period
        self.position = 0
    
    def analyze_market(self):
        # 최근 가격 조회
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
        
        # 간단한 모멘텀: 현재 가격 vs 평균
        current_price = prices[-1]
        average_price = sum(prices) / len(prices)
        
        momentum = (current_price - average_price) / average_price
        return momentum
    
    def execute_strategy(self):
        momentum = self.analyze_market()
        
        if momentum > 0.02 and self.position <= 0:
            # 강한 상승 모멘텀, 롱 포지션
            self.place_order("BUY", "0.01")
            self.position = 1
            
        elif momentum < -0.02 and self.position >= 0:
            # 강한 하락 모멘텀, 숏 포지션 또는 롱 포지션 청산
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
            print(f"주문 실행: {side} {quantity} {self.symbol}")
        else:
            print(f"주문 실패: {response.text}")

# 전략 실행
strategy = MomentumStrategy()
while True:
    strategy.execute_strategy()
    time.sleep(3600)  # 매시간 확인
```

### 전략 백테스팅
```python
# 과거 데이터로 전략 백테스팅
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
print(f"총 수익률: {results['total_return']}%")
print(f"샤프 비율: {results['sharpe_ratio']}")
print(f"최대 낙폭: {results['max_drawdown']}%")
print(f"승률: {results['win_rate']}%")
```

## 성과 분석

### 포트폴리오 성과
```python
# 포트폴리오 성과 지표 조회
response = requests.get(
    'https://api.mexoms.com/v1/analytics/portfolio',
    headers=headers,
    params={
        "start_date": "2024-01-01",
        "end_date": "2024-12-31"
    }
)

performance = response.json()
print(f"총 수익률: {performance['total_return']}%")
print(f"연간 수익률: {performance['annualized_return']}%")
print(f"변동성: {performance['volatility']}%")
print(f"샤프 비율: {performance['sharpe_ratio']}")
print(f"최대 낙폭: {performance['max_drawdown']}%")
```

### 거래 분석
```python
# 개별 거래 분석
response = requests.get(
    'https://api.mexoms.com/v1/analytics/trades',
    headers=headers,
    params={
        "symbol": "BTCUSDT",
        "limit": 100
    }
)

trades = response.json()['trades']

# 통계 계산
winning_trades = [t for t in trades if t['pnl'] > 0]
losing_trades = [t for t in trades if t['pnl'] < 0]

win_rate = len(winning_trades) / len(trades) * 100
avg_win = sum(t['pnl'] for t in winning_trades) / len(winning_trades) if winning_trades else 0
avg_loss = sum(t['pnl'] for t in losing_trades) / len(losing_trades) if losing_trades else 0

print(f"승률: {win_rate:.1f}%")
print(f"평균 수익: ${avg_win:.2f}")
print(f"평균 손실: ${avg_loss:.2f}")
print(f"수익 팩터: {abs(avg_win/avg_loss) if avg_loss != 0 else 'N/A'}")
```

### 위험 지표
```python
# 현재 위험 지표 조회
response = requests.get(
    'https://api.mexoms.com/v1/risk/metrics',
    headers=headers
)

risk = response.json()
print(f"포트폴리오 VaR (95%): ${risk['var_95']}")
print(f"예상 손실: ${risk['expected_shortfall']}")
print(f"베타: {risk['beta']}")
print(f"알파: {risk['alpha']}")
```

## 문제 해결

### 일반적인 문제

#### 1. 주문 거부
```python
# 주문 거부 이유 확인
if response.status_code == 400:
    error = response.json()
    if error['code'] == 'INSUFFICIENT_BALANCE':
        print("주문을 위한 잔고가 부족합니다")
        # 계정 잔고 확인
        balance = requests.get('https://api.mexoms.com/v1/accounts/balance', headers=headers)
        print(f"사용 가능한 잔고: {balance.json()}")
    elif error['code'] == 'INVALID_SYMBOL':
        print("잘못된 거래 심볼입니다")
        # 유효한 심볼 조회
        symbols = requests.get('https://api.mexoms.com/v1/market/symbols', headers=headers)
        print(f"유효한 심볼: {[s['symbol'] for s in symbols.json()]}")
```

#### 2. 연결 문제
```python
import time
from requests.adapters import HTTPAdapter
from urllib3.util.retry import Retry

# 재시도 로직 구현
session = requests.Session()
retry_strategy = Retry(
    total=3,
    backoff_factor=1,
    status_forcelist=[429, 500, 502, 503, 504]
)
adapter = HTTPAdapter(max_retries=retry_strategy)
session.mount("http://", adapter)
session.mount("https://", adapter)

# 요청에 세션 사용
response = session.post('https://api.mexoms.com/v1/orders', headers=headers, json=order)
```

#### 3. 속도 제한
```python
import time

def make_request_with_retry(url, **kwargs):
    max_retries = 5
    base_delay = 1
    
    for attempt in range(max_retries):
        response = requests.get(url, **kwargs)
        
        if response.status_code == 429:  # 속도 제한
            retry_after = int(response.headers.get('Retry-After', base_delay * (2 ** attempt)))
            print(f"속도 제한. {retry_after}초 후 재시도...")
            time.sleep(retry_after)
            continue
        
        return response
    
    raise Exception("최대 재시도 횟수 초과")
```

### 성능 최적화

#### 1. 연결 풀링
```python
import requests
from requests.adapters import HTTPAdapter

session = requests.Session()
session.mount('https://', HTTPAdapter(
    pool_connections=10,
    pool_maxsize=20,
    max_retries=3
))

# 모든 요청에서 세션 재사용
response = session.get('https://api.mexoms.com/v1/positions', headers=headers)
```

#### 2. 대량 작업
```python
# 여러 주문을 한 번에 제출
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

### 지원 리소스

- **API 상태**: [status.mexoms.com](https://status.mexoms.com)
- **속도 제한**: 응답의 `X-RateLimit-*` 헤더 확인
- **오류 코드**: [API 문서](../api/README.md#error-codes) 참조
- **커뮤니티**: [디스코드](https://discord.gg/mexoms) | [포럼](https://community.mexoms.com)

---

*행복한 거래하세요! 📈*