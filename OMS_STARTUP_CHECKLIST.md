# OMS 시작 전 필수 체크리스트 및 매뉴얼

## 🔍 시작 전 확인사항

### 1. Docker 서비스 상태 확인
```bash
# Docker 데몬이 실행 중인지 확인
sudo systemctl status docker

# 실행 중이 아니라면 시작
sudo systemctl start docker
```

### 2. 기존 Docker 컨테이너 정리
```bash
# 기존 컨테이너 상태 확인
docker ps -a | grep -E "nats|redis|vault"

# 정지된 컨테이너가 있다면 제거
docker rm -f nats-oms redis-oms vault-oms 2>/dev/null
```

### 3. 포트 사용 확인
```bash
# 필수 포트들이 사용 중인지 확인
lsof -i :3000   # Frontend
lsof -i :8080   # Dashboard Server
lsof -i :4222   # NATS
lsof -i :6379   # Redis  
lsof -i :8200   # Vault

# 포트가 사용 중이라면 프로세스 종료
sudo kill -9 $(lsof -t -i:포트번호)
```

### 4. 이전 프로세스 정리
```bash
# OMS 관련 프로세스가 실행 중인지 확인
ps aux | grep -E "binance|dashboard|go run" | grep -v grep

# 실행 중이라면 종료
pkill -f "binance" 
pkill -f "dashboard"
pkill -f "go run cmd/"
```

### 5. Vault 데이터 확인
```bash
# Vault 토큰 파일 존재 확인
ls -la ~/.mExOms/vault-token

# Vault 키 파일 존재 확인
ls -la ~/.mExOms/vault-keys.json
```

## 🚀 수동 시작 가이드 (문제 발생 시)

### Step 1: 인프라 서비스 시작

#### 1-1. NATS 시작
```bash
# 기존 컨테이너 제거
docker rm -f nats-oms 2>/dev/null

# NATS 시작
docker run -d --name nats-oms \
  -p 4222:4222 -p 8222:8222 \
  nats:latest -js

# 상태 확인
docker logs nats-oms --tail 10
```

#### 1-2. Redis 시작
```bash
# 기존 컨테이너 제거
docker rm -f redis-oms 2>/dev/null

# Redis 시작
docker run -d --name redis-oms \
  -p 6379:6379 \
  redis:latest

# 상태 확인
docker logs redis-oms --tail 10
```

#### 1-3. Vault 시작
```bash
# Vault 시작 (docker-compose 사용)
cd /home/seunge/project/mExOms
docker-compose up -d vault

# 상태 확인
docker logs vault-oms --tail 10

# Sealed 상태 확인
curl -s http://localhost:8200/v1/sys/health | jq .

# Sealed라면 Unseal (vault-keys.json이 있는 경우)
UNSEAL_KEY=$(cat ~/.mExOms/vault-keys.json | jq -r '.keys_base64[0]')
curl -X PUT http://localhost:8200/v1/sys/unseal \
  -H "Content-Type: application/json" \
  -d "{\"key\": \"$UNSEAL_KEY\"}"
```

### Step 2: OMS 서비스 시작

#### 2-1. Market Data Service
```bash
cd /home/seunge/project/mExOms
go run cmd/binance-market-full/main.go > logs/market.log 2>&1 &

# 로그 확인 (정상 동작 확인)
tail -f logs/market.log
# Ctrl+C로 종료
```

#### 2-2. Balance Service  
```bash
# 주의: binance-spot-balance-simple이 아닌 binance-spot-balance 사용
go run cmd/binance-spot-balance/main.go > logs/balance.log 2>&1 &

# 로그 확인
tail -f logs/balance.log
```

#### 2-3. Futures Position Service
```bash
go run cmd/binance-futures-position/main.go > logs/futures.log 2>&1 &

# 로그 확인
tail -f logs/futures.log
```

### Step 3: Dashboard 시작

#### 3-1. Dashboard Server
```bash
cd /home/seunge/project/mExOms/dashboard

# 실행 파일이 없다면 빌드
if [ ! -f "oms-dashboard-real" ]; then
    go build -o oms-dashboard-real server/main_real.go
fi

# 실행
./oms-dashboard-real > ../logs/dashboard-server.log 2>&1 &

# 로그 확인
tail -f ../logs/dashboard-server.log
```

#### 3-2. Frontend
```bash
cd /home/seunge/project/mExOms/dashboard/frontend

# 의존성 설치 (필요한 경우)
npm install

# 실행
npm start > ../../logs/frontend.log 2>&1 &

# 브라우저에서 http://localhost:3000 접속
```

## ✅ 정상 동작 확인

### 1. 서비스 상태 확인
```bash
# Docker 서비스들
docker ps | grep -E "nats|redis|vault"

# Go 서비스들  
ps aux | grep -E "binance|dashboard" | grep -v grep

# 포트 확인
netstat -tlnp | grep -E "3000|8080|4222|6379|8200"
```

### 2. 로그 모니터링
```bash
# 실시간 로그 확인 (멀티 테일)
tail -f logs/*.log
```

### 3. Dashboard 확인
- http://localhost:3000 접속
- Market Data 표시 확인
- Spot Balance 표시 확인  
- Futures Position 표시 확인

## 🔧 자주 발생하는 문제 해결

### 문제 1: NATS 연결 실패
```
Failed to connect to NATS: nats: no servers available for connection
```
**해결**: NATS가 실행되지 않았거나 아직 준비되지 않음. NATS 재시작 후 10초 대기

### 문제 2: Vault API 키 못 찾음
```
api_secret not found in Vault
secret_key not found in Vault
```

**원인**: 
- Vault가 재시작되면서 데이터가 사라짐 (Dev 모드인 경우)
- Vault가 sealed 상태
- API 키가 아예 저장되지 않음

**해결 방법**:

#### 1. Vault 상태 확인
```bash
# Vault health 확인
curl -s http://localhost:8200/v1/sys/health | jq .

# sealed가 true면 unseal 필요
```

#### 2. API 키가 저장되어 있는지 확인
```bash
# Vault 토큰 읽기
VAULT_TOKEN=$(cat ~/.mExOms/vault-token)

# API 키 조회 시도
curl -s -H "X-Vault-Token: $VAULT_TOKEN" \
  http://localhost:8200/v1/secret/data/exchanges/binance_spot | jq .

# 404 에러면 키가 없는 것
```

#### 3. API 키 저장 (키가 없는 경우)
```bash
# store-binance-keys.sh 실행
./scripts/store-binance-keys.sh

# 스크립트가 대화형으로 진행됨:
# 1. Spot API Key 입력
# 2. Spot Secret Key 입력  
# 3. Futures API Key 입력
# 4. Futures Secret Key 입력

# 또는 환경변수로 설정 (임시)
export BINANCE_API_KEY="your-api-key"
export BINANCE_SECRET_KEY="your-secret-key"
export BINANCE_FUTURES_API_KEY="your-futures-api-key"
export BINANCE_FUTURES_SECRET_KEY="your-futures-secret-key"
```

#### 4. 수동으로 API 키 저장
```bash
# Vault 토큰 설정
export VAULT_TOKEN=$(cat ~/.mExOms/vault-token)
export VAULT_ADDR='http://localhost:8200'

# Spot 키 저장
curl -X POST $VAULT_ADDR/v1/secret/data/exchanges/binance_spot \
  -H "X-Vault-Token: $VAULT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "data": {
      "api_key": "YOUR_SPOT_API_KEY",
      "secret_key": "YOUR_SPOT_SECRET_KEY"
    }
  }'

# Futures 키 저장
curl -X POST $VAULT_ADDR/v1/secret/data/exchanges/binance_futures \
  -H "X-Vault-Token: $VAULT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "data": {
      "api_key": "YOUR_FUTURES_API_KEY",
      "secret_key": "YOUR_FUTURES_SECRET_KEY"
    }
  }'

# 저장 확인
curl -s -H "X-Vault-Token: $VAULT_TOKEN" \
  $VAULT_ADDR/v1/secret/data/exchanges/binance_spot | jq .data.data
```

#### 5. 키 이름 불일치 문제
Balance 서비스와 Futures 서비스가 찾는 키 이름이 다를 수 있음:
- `api_secret` vs `secret_key`
- 코드에서는 `secret_key`를 사용하도록 통일됨

### 문제 3: 포트 이미 사용 중
```
bind: address already in use
```
**해결**: 
1. 해당 포트를 사용하는 프로세스 찾기: `lsof -i :포트번호`
2. 프로세스 종료: `kill -9 PID`

### 문제 4: Frontend 빌드 에러
```
npm ERR! 
```
**해결**:
1. node_modules 삭제: `rm -rf node_modules`
2. 캐시 정리: `npm cache clean --force`
3. 재설치: `npm install`

## 📝 권장 시작 순서

1. **Docker 인프라** (NATS → Redis → Vault) - 각각 5초 대기
2. **Vault Unseal** - sealed 상태 확인 필수
3. **Market Data Service** - API 키 불필요
4. **Balance & Futures Service** - Vault 준비 후 시작
5. **Dashboard Server** - NATS 연결 필요
6. **Frontend** - 마지막에 시작

## 🛑 종료 순서

1. Frontend (Ctrl+C 또는 프로세스 종료)
2. Dashboard Server
3. OMS 서비스들 (Market, Balance, Futures)
4. Docker 인프라 (선택사항)

```bash
# 전체 종료 스크립트
./scripts/stop-oms.sh
```