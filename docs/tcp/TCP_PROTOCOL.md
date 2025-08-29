# mExOms TCP Protocol Specification

## Overview

mExOms TCP 서버는 고성능 바이너리 프로토콜을 사용하여 실시간 시장 데이터와 주문 처리를 제공합니다.

## 프로토콜 구조

### 메시지 프레임
```
[Length: 4 bytes][Message: N bytes]
- Length: 메시지 길이 (Big Endian)
- Message: Protocol Buffers로 인코딩된 메시지
```

### 연결 플로우
1. TCP 연결 (포트 9090)
2. Login 요청
3. Login 응답 (성공시 session token 포함)
4. Subscribe/주문 요청
5. 실시간 데이터 수신

## 메시지 타입

### 인증 (Authentication)
- `LOGIN_REQUEST`: API Key로 인증
- `LOGIN_RESPONSE`: 세션 토큰 및 권한 반환
- `LOGOUT_REQUEST`: 연결 종료 요청

### 마켓 데이터 (Market Data)
- `SUBSCRIBE_REQUEST`: 시세 구독
- `MARKET_DATA_UPDATE`: 실시간 가격 업데이트
- `ORDERBOOK_UPDATE`: 호가창 업데이트
- `TRADE_UPDATE`: 체결 내역

### 주문 (Orders)
- `ORDER_REQUEST`: 신규 주문
- `ORDER_RESPONSE`: 주문 응답
- `CANCEL_ORDER_REQUEST`: 주문 취소
- `ORDER_STATUS_UPDATE`: 주문 상태 변경

### 계좌 (Account)
- `BALANCE_REQUEST`: 잔고 조회
- `BALANCE_RESPONSE`: 잔고 응답
- `POSITION_REQUEST`: 포지션 조회
- `POSITION_RESPONSE`: 포지션 응답

## 성능 특징

- **레이턴시**: < 100μs (로컬 네트워크)
- **처리량**: 50,000 msg/sec/client
- **동시 접속**: 100+ 클라이언트
- **프로토콜**: Protocol Buffers 3.0
- **전송**: Zero-copy 최적화

## 에러 처리

에러 메시지 형식:
```protobuf
message ErrorMessage {
    int32 code = 1;
    string message = 2;
    string details = 3;
}
```

에러 코드:
- 1000: 인증 실패
- 1001: 권한 부족
- 1002: 잘못된 요청
- 1003: 서버 오류
- 1004: Rate limit 초과

## 보안

- TLS 1.3 지원 (선택적)
- API Key 인증
- 세션 타임아웃: 1시간
- IP 화이트리스트 지원

## 예제 코드

### C++ 클라이언트
```cpp
// TCP 연결
auto client = std::make_unique<TcpClient>("localhost", 9090);
client->Connect();

// 로그인
LoginRequest req;
req.set_api_key("your_api_key");
req.set_secret("your_secret");
client->Send(req);

// 시세 구독
SubscribeRequest sub;
sub.add_symbols("BTC/USDT");
sub.add_channels("ticker");
client->Send(sub);
```

### Python 클라이언트
```python
import socket
import messages_pb2 as pb

# 연결
sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
sock.connect(('localhost', 9090))

# 로그인
login = pb.LoginRequest()
login.api_key = "your_api_key"
login.secret = "your_secret"
send_message(sock, login)
```