# Vault 영구 저장소 솔루션

## 현재 상황
- Vault를 개발 모드로 실행 중 (메모리 저장)
- 재부팅 시 API 키가 사라짐
- 프로덕션 모드 설정 시 권한 문제 발생

## 해결 방안

### 방안 1: 환경 변수 사용 (추천)
가장 간단하고 안정적인 방법입니다.

```bash
# ~/.bashrc 또는 ~/.profile에 추가
export BINANCE_SPOT_API_KEY="실제_API_키"
export BINANCE_SPOT_SECRET_KEY="실제_시크릿_키"
export BINANCE_FUTURES_API_KEY="실제_API_키"
export BINANCE_FUTURES_SECRET_KEY="실제_시크릿_키"
```

### 방안 2: 파일 기반 저장
```bash
# ~/.mExOms/keys.json 파일 생성
{
  "spot": {
    "api_key": "실제_API_키",
    "secret_key": "실제_시크릿_키"
  },
  "futures": {
    "api_key": "실제_API_키",
    "secret_key": "실제_시크릿_키"
  }
}
```

권한 설정:
```bash
chmod 600 ~/.mExOms/keys.json
```

### 방안 3: Systemd 시크릿 사용 (Linux)
```bash
# /etc/systemd/system/mexoms.service.d/override.conf
[Service]
Environment="BINANCE_SPOT_API_KEY=실제_API_키"
Environment="BINANCE_SPOT_SECRET_KEY=실제_시크릿_키"
```

## 현재 임시 해결책
1. Vault는 개발 모드로 유지
2. 재부팅 후 다시 키 저장:
```bash
curl -X POST http://localhost:8200/v1/secret/data/exchanges/binance_spot \
  -H "X-Vault-Token: myroot" \
  -H "Content-Type: application/json" \
  -d '{"data": {"api_key": "실제_API_키", "secret_key": "실제_시크릿_키"}}'
```