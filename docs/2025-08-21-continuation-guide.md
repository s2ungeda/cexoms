# 작업 연속성 가이드 - 2025년 8월 21일

## 🎯 내일 이어갈 주요 작업

### 1. Binance Futures Connector 완성 (Phase 6 마무리)
현재 80% 완성된 Futures 커넥터를 완료해야 합니다.

#### 남은 작업:
- [ ] Position 관리 기능 개선
  - Position 실시간 업데이트 (WebSocket)
  - Position 히스토리 추적
  - Realized PnL 계산

- [ ] Leverage & Margin 관리
  - Symbol별 leverage 설정 API
  - Cross/Isolated 마진 모드 전환
  - 마진 잔고 실시간 모니터링

- [ ] Advanced Order Types
  - Stop Loss / Take Profit 주문
  - Trailing Stop 주문
  - Iceberg 주문

#### 작업 파일:
- `/services/binance/futures_multi_account.go`
- `/services/binance/ws_futures_order_manager.go`

### 2. Risk Management 시스템 구축 (Phase 7 시작)
```go
// 생성해야 할 파일: /internal/risk/manager.go
type RiskManager interface {
    CheckOrderRisk(order *types.Order) error
    CalculatePositionSize(params PositionSizeParams) decimal.Decimal
    SetMaxDrawdown(percentage float64)
    SetMaxExposure(amount decimal.Decimal)
}
```

#### 구현 사항:
- [ ] Position 크기 계산기
- [ ] 최대 손실 한도 체크
- [ ] 자동 Stop Loss 설정
- [ ] 계정별 리스크 한도 관리

### 3. 테스트 코드 작성
현재 테스트 커버리지가 부족합니다. 우선순위:

1. **WebSocket Order Manager 테스트**
   ```bash
   # 생성할 파일
   /services/binance/ws_order_manager_test.go
   /services/binance/ws_futures_order_manager_test.go
   ```

2. **Multi-Account 통합 테스트**
   ```bash
   # 생성할 파일
   /services/binance/spot_multi_account_test.go
   /services/binance/futures_multi_account_test.go
   ```

### 4. 성능 최적화
WebSocket 연결이 35ms로 개선되었지만 추가 최적화 가능:

- [ ] Connection pooling 구현
- [ ] Message batching
- [ ] Binary protocol 검토 (현재 JSON)

## 🔧 환경 설정 확인

### 1. Vault 상태 확인
```bash
# Vault 서버 실행 확인
ps aux | grep vault

# API 키 확인
./cmd/vault-cli/vault-cli get binance spot
```

### 2. 의존성 확인
```bash
# Go 모듈 업데이트
go mod tidy

# C++ 빌드 도구 확인
make build-core
```

## 📝 코드 작성 시 주의사항

### 1. WebSocket 우선 정책
- 모든 주문 작업은 WebSocket을 먼저 시도
- REST API는 폴백으로만 사용
- 성능 메트릭 로깅 필수

### 2. 에러 처리
```go
// 항상 이 패턴 사용
if err != nil {
    // 구체적인 에러 메시지
    return fmt.Errorf("failed to [action] for [target]: %w", err)
}
```

### 3. 타입 안전성
- `string` 대신 `types.OrderType`, `types.TimeInForce` 등 enum 사용
- decimal 패키지로 모든 금액 처리

## 🚨 알려진 이슈

### 1. Binance 제약사항
- 최소 주문 금액: $10 (NOTIONAL filter)
- Rate limit: 1200 weight/min (Spot), 2400 weight/min (Futures)
- WebSocket 연결 수 제한: 계정당 5개

### 2. 해결 필요 사항
- [ ] Order tracking by symbol (현재 하드코딩됨)
- [ ] WebSocket 재연결 시 상태 복구
- [ ] Multi-account rate limit 통합 관리

## 📊 현재 프로젝트 상태

```
Phase 1-4: ✅ Infrastructure (100%)
Phase 5:   ✅ Binance Spot (100%)
Phase 6:   🔄 Binance Futures (80%)
Phase 7:   ⏳ Risk Management (0%)
Phase 8:   ⏳ Order Router (0%)
Phase 9:   ⏳ Bybit Integration (0%)
Phase 10:  ⏳ OKX Integration (0%)
```

## 🎯 이번 주 목표

1. **화요일 (8/21)**: Binance Futures 완성 + Risk Management 설계
2. **수요일 (8/22)**: Risk Management 구현
3. **목요일 (8/23)**: Smart Order Router 시작
4. **금요일 (8/24)**: 통합 테스트 및 문서화

## 💡 유용한 명령어

```bash
# 전체 빌드
make build

# 테스트 실행
make test

# WebSocket 테스트
go run test-ws-spot-trading.go
go run test-ws-futures-trading.go

# 잔고 확인
go run cmd/test-trading/main.go balance

# 로그 확인
tail -f logs/oms.log
```

## 📚 참고 문서

- [Binance WebSocket API](https://binance-docs.github.io/apidocs/websocket_api/en/)
- [프로젝트 아키텍처](./CONTEXT.md)
- [오늘 작업 내역](./WORK_LOG_2025-08-20.md)
- [WebSocket 구현 세부사항](./2025-08-20-websocket-implementation.md)

---

**작업 시작 전 체크리스트:**
- [ ] Vault 서버 실행 중
- [ ] NATS 서버 실행 중  
- [ ] 최신 코드 pull 완료
- [ ] 환경 변수 설정 확인