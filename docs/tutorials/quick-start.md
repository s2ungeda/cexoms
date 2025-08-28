# mExOms Quick Start Guide

## 소개

mExOms(Multi-Exchange Order Management System)는 여러 암호화폐 거래소에서 동시에 거래할 수 있는 고성능 주문 관리 시스템입니다. 이 가이드는 시스템을 빠르게 시작하고 첫 번째 거래를 실행하는 방법을 안내합니다.

## 전제 조건

- Docker 및 Docker Compose
- Go 1.21 이상
- C++ 컴파일러 (GCC 11+ 또는 Clang 14+)
- Make
- Git

## 1단계: 저장소 클론

```bash
git clone https://github.com/your-org/mExOms.git
cd mExOms
```

## 2단계: 의존성 설치

```bash
make install-deps
```

## 3단계: 환경 설정

### Vault 설정 (API 키 보안 저장소)

```bash
# Vault 시작
docker-compose up -d vault

# Vault 초기화
make setup-vault

# API 키 저장 (Binance 예시)
./scripts/vault-add-keys.sh binance_spot YOUR_API_KEY YOUR_SECRET_KEY
```

### 설정 파일 준비

```bash
# 기본 설정 복사
cp configs/config.yaml.example configs/config.yaml

# 설정 편집 (필요시)
nano configs/config.yaml
```

## 4단계: 인프라 시작

```bash
# 모든 인프라 서비스 시작
docker-compose up -d

# 서비스 상태 확인
docker-compose ps
```

## 5단계: 시스템 빌드 및 시작

```bash
# 전체 시스템 빌드
make build

# OMS 서버 시작
make run-server

# 거래소 커넥터 시작 (별도 터미널)
make run-binance-spot
make run-binance-futures
```

## 6단계: 모니터링 대시보드 접속

브라우저에서 http://localhost:3000 접속

## 7단계: 첫 거래 실행

### Python 클라이언트 사용

```python
from oms_client import OMSClient

# 클라이언트 초기화
client = OMSClient("localhost:50051")

# 시장가 매수 주문
order = client.place_order(
    exchange="binance",
    market="spot",
    symbol="BTC/USDT",
    side="buy",
    order_type="market",
    quantity=0.001
)

print(f"Order placed: {order.order_id}")
```

### gRPC CLI 사용

```bash
# 지정가 매도 주문
./bin/oms-client order place \
  --exchange binance \
  --market spot \
  --symbol ETH/USDT \
  --side sell \
  --type limit \
  --price 2500 \
  --quantity 0.1
```

## 8단계: 시스템 종료

```bash
# 모든 서비스 안전하게 종료
make stop-all

# Docker 컨테이너 정리
docker-compose down
```

## 다음 단계

- [기본 거래 튜토리얼](basic-trading.md) - 다양한 주문 유형 학습
- [멀티계좌 거래](multi-account-trading.md) - 여러 계좌 동시 관리
- [스마트 오더 라우팅](smart-order-routing.md) - 최적의 거래 경로 찾기
- [WebSocket 스트리밍](websocket-streaming.md) - 실시간 데이터 스트리밍

## 문제 해결

### Vault 연결 실패
```bash
# Vault 상태 확인
docker logs vault

# Vault 재시작
docker-compose restart vault
```

### NATS 연결 오류
```bash
# NATS 클러스터 상태 확인
./bin/oms-client nats check

# NATS 로그 확인
docker logs nats
```

### 빌드 실패
```bash
# 캐시 정리 후 재빌드
make clean
make build
```

## 지원

- GitHub Issues: https://github.com/your-org/mExOms/issues
- Discord: https://discord.gg/mExOms
- Email: support@mexoms.io