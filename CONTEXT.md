# mExOms 프로젝트 컨텍스트

## 프로젝트 개요
멀티거래소 및 멀티계좌 지원 고성능 암호화폐 주문관리시스템(OMS)
- C++ 코어 엔진 (초고속 처리)
- Go 서비스 레이어 (거래소 연동)
- NATS 메시징 (내부 통신)
- 자동화 거래 전략 (차익거래, LP/마켓메이킹)

## 핵심 아키텍처 결정사항

### 1. 멀티계좌 구조
- 메인계좌 1개 + 서브계좌 최대 200개
- 계좌별 독립 API 키 (Vault 저장)
- Rate Limit 분산: 계좌당 1200 weight/min × 200 = 240,000 weight/min
- 계좌 타입: Main, Sub, Strategy

### 2. 데이터 저장 전략
- **실시간 데이터**: 공유 메모리 (/dev/shm)
- **이벤트 스트림**: NATS JetStream (30일 보관)
- **영구 저장**: 파일 시스템 (JSON/CSV)
- **NO Database**: PostgreSQL/Redis 사용 안 함

### 3. 메시징 패턴
```
Subject: {action}.{exchange}.{account}.{market}.{symbol}
예: orders.binance.sub_arb.spot.BTCUSDT
```

### 4. 성능 목표
- 주문 처리: < 100μs
- 리스크 체크: < 50μs  
- 차익거래 감지: < 1ms
- LP 호가 갱신: < 10ms

## 현재 구현 상태

### Phase 19: 차익거래 엔진 ✅
**주요 파일:**
- `internal/strategies/arbitrage/detector.go` - 기회 감지
- `internal/strategies/arbitrage/executor.go` - 자동 실행
- `core/include/strategies/arbitrage_detector.h` - C++ 헤더
- `core/src/strategies/arbitrage_detector.cpp` - C++ 구현

**핵심 로직:**
```go
// 차익거래 기회 구조체
type ArbitrageOpportunity struct {
    Symbol        string
    BuyExchange   string
    SellExchange  string
    ProfitRate    decimal.Decimal
    // ...
}

// 감지 로직
if sellPrice - buyPrice > minProfit {
    // 수수료 계산 후 실행
}
```

### Phase 20: LP/마켓메이킹 엔진 (진행 중) 🔄

**완료된 Go 파일:**
1. `types.go` - 모든 타입 정의
   - MarketMakerConfig, Quote, MarketState 등

2. `spread_calculator.go` - 동적 스프레드 계산
   - 변동성 기반 조정
   - 재고 기반 스큐
   - 주문북 깊이 반영

3. `inventory_manager.go` - 재고/포지션 관리
   - 실시간 P&L 추적
   - 포지션 한도 관리
   - 리밸런싱 로직

4. `quote_generator.go` - 호가 생성
   - 멀티레벨 호가
   - 경쟁력 있는 가격 보장

5. `market_maker.go` - 메인 전략 엔진
   - 주문 라이프사이클 관리
   - 실시간 시장 데이터 처리

6. `risk_manager.go` - 리스크 관리
   - Kill Switch
   - 일일 손실 한도
   - 포지션 리스크 체크

**미완료 작업:**
- `core/src/strategies/market_maker.cpp` - C++ 구현 필요

**핵심 설정 예시:**
```go
config := &MarketMakerConfig{
    Symbol:       "BTCUSDT",
    SpreadBps:    decimal.NewFromInt(10),  // 0.1%
    QuoteSize:    decimal.NewFromFloat(0.1),
    QuoteLevels:  3,
    MaxInventory: decimal.NewFromInt(1),
}
```

## 다음 세션 시작 가이드

### 1. 필수 읽어야 할 파일
```bash
# 프로젝트 전체 가이드
cat /home/seunge/project/mExOms/oms-guide.md

# 프로젝트 지침
cat /home/seunge/project/mExOms/CLAUDE.md

# 진행 상황
cat /home/seunge/project/mExOms/PROGRESS.md

# 이 컨텍스트 파일
cat /home/seunge/project/mExOms/CONTEXT.md
```

### 2. Phase 20 완료 작업
```bash
# C++ 마켓메이커 구현
vim /home/seunge/project/mExOms/core/src/strategies/market_maker.cpp

# 주요 구현 포인트:
# - MarketMakerEngine::generateQuotes() 
# - SpreadCalculator::calculate()
# - Lock-free quote buffer
# - < 10ms 호가 갱신
```

### 3. Phase 21 시작 작업
```bash
# 전략 통합 관리자 생성
mkdir -p /home/seunge/project/mExOms/internal/strategies/manager
vim /home/seunge/project/mExOms/internal/strategies/manager/orchestrator.go

# 구현할 기능:
# - 전략별 계좌 할당
# - 동시 실행 관리
# - 자본 배분
# - Kill Switch 통합
```

## 중요 상수/설정값

### 거래소별 수수료
- Binance: 0.1% (Maker/Taker)
- Bybit: 0.1% 
- OKX: 0.08% (Maker), 0.1% (Taker)

### 리스크 한도
- 최대 포지션: $10,000 per strategy
- 일일 손실: $1,000
- 차익거래 최소 수익: 0.1%
- LP 최대 재고: 1 BTC

### 계좌 설정
- 메인계좌: 최소 $100,000 유지
- 서브계좌: 최대 $50,000
- 전략별 계좌 매핑:
  - arbitrage → sub_spot_arb
  - market_making → sub_market_making

## 테스트 명령어
```bash
# 빌드
make build

# 테스트
make test

# 벤치마크 (성능 측정)
make test-benchmark

# NATS 실행
make run-nats
```

## 주의사항
1. 모든 가격은 decimal.Decimal 사용 (부동소수점 오류 방지)
2. 시간은 나노초 단위 사용 (초고속 처리)
3. Lock-free 구조 우선 (뮤텍스 최소화)
4. 계좌별 격리 (리스크 분산)