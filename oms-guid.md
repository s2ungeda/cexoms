# Multi-Exchange Cryptocurrency OMS Development Guide (Simplified)

## 목표
```markdown
멀티거래소 및 멀티계좌 지원 고성능 암호화폐 주문관리시스템(OMS) 구축
- 초기: 바이낸스 Spot/Futures 지원 (메인/서브계좌)
- 확장: Bybit, OKX, Upbit 등 추가 가능한 아키텍처
- 성능: 마이크로초 단위 주문 처리
- 계좌: 서브계좌별 독립 전략 운영
- 최소한의 의존성으로 구현
```

## 기술 스택 (최소 구성)
```markdown
### 핵심 엔진
- C++20: Lock-free 자료구조, Ring buffer, CPU 어피니티
- 공유 메모리: 프로세스 간 데이터 공유 (/dev/shm)
- 목표 레이턴시: < 100 마이크로초

### 서비스 레이어
- Go 1.21+: 거래소 커넥터, 비즈니스 로직
- 거래소 SDK: go-binance/v2
- 메모리 캐시: sync.Map (Redis 대체)

### 메시징 & 저장
- NATS Core: 초저지연 내부 통신
- NATS JetStream: 메시지 영속성, 이벤트 소싱 (DB 대체)
- 파일 시스템: 로그, 백업, 스냅샷

### API
- gRPC: 외부 클라이언트 연동 (내부는 NATS만 사용)
- Protocol Buffers: 스키마 정의

### 보안
- HashiCorp Vault: 멀티계좌 API 키 중앙 관리
- AES-256: 로컬 키 암호화
- mlock: 메모리 스왑 방지

### 모니터링
- 로그 파일: 시스템 모니터링
- 메트릭: 자체 구현 또는 Prometheus (선택적)
```

## 주요 기능
```markdown
### 1. 멀티거래소 지원
- 통합 Exchange 인터페이스
- 거래소별 커넥터 (Binance, Bybit, OKX 등)
- 심볼 정규화 시스템
- 거래소 자동 전환 (장애 시)

### 2. 멀티계좌 관리 (신규)
- 메인/서브계좌 구조 지원
- 계좌별 독립 API 키 관리
- 전략별 계좌 자동 선택
- 계좌 간 자산 이체
- 계좌별 포지션/P&L 추적
- Rate Limit 계좌 분산

### 3. 주문 관리
- 초고속 주문 처리 (< 100μs)
- 계좌별 주문 라우팅
- 주문 생성/수정/취소
- 일괄 주문 처리
- 조건부 주문 (Stop Loss, Take Profit)

### 4. 스마트 라우팅
- 최적 거래소 자동 선택
- 최적 계좌 자동 선택
- 대량 주문 분할 실행
- 거래소 간 차익거래 감지
- 수수료 최적화

### 5. 포지션 관리
- 실시간 포지션 추적 (메모리)
- 계좌별/거래소별 포지션 분리
- 통합 포지션 뷰 (전체 계좌)
- P&L 실시간 계산
- 레버리지 관리 (Futures)

### 6. 리스크 관리
- 실시간 리스크 체크 (< 50μs)
- 계좌별 포지션 한도 관리
- 전체 계좌 통합 리스크 계산
- 레버리지 제한
- 손실 한도 (Stop Loss)

### 7. 시장 데이터
- 실시간 가격 스트리밍
- 통합 주문북 (모든 거래소)
- 거래소 간 스프레드 모니터링
- VWAP/TWAP 계산

### 8. 데이터 저장
- JetStream: 주문/거래 이벤트 (30일)
- 파일: 일별 거래 로그 (JSON/CSV)
- 공유 메모리: 실시간 데이터
- 스냅샷: 주기적 상태 백업
```

## 데이터 저장 전략
```markdown
### 실시간 데이터 (메모리)
- 활성 주문 (계좌별)
- 현재 포지션 (계좌별)
- 주문북
- 잔고 (계좌별)

### 이벤트 스트림 (JetStream)
- 주문 이벤트: orders.{exchange}.{account}.{market}.{symbol}
- 거래 체결: trades.{exchange}.{account}.{market}.{symbol}
- 포지션 변경: positions.{exchange}.{account}.{market}
- 계좌 이체: transfers.{from_account}.{to_account}
- 보관 기간: 30일

### 백업/아카이브 (파일)
- 일별 거래 로그: /data/logs/2024/01/15/{account}/trades.jsonl
- 시간별 스냅샷: /data/snapshots/2024/01/15/14/{account}/state.json
- P&L 보고서: /data/reports/2024/01/{account}/pnl.csv
- 계좌 이체 기록: /data/transfers/2024/01/transfers.jsonl

### 선택적 (필요시 추가)
- PostgreSQL: 복잡한 분석, 규제 보고용
- Redis: 분산 시스템 전환 시
```

## 멀티계좌 설정 예시
```yaml
# configs/accounts.yaml
accounts:
  binance:
    main:
      type: main
      api_key_vault_path: binance_main
      spot_enabled: true
      futures_enabled: false
      
    sub_spot_arb:
      type: sub
      parent: main
      api_key_vault_path: binance_sub_spot_arb
      spot_enabled: true
      futures_enabled: false
      strategy: "arbitrage"
      max_balance_usdt: 10000
      
    sub_futures_trend:
      type: sub
      parent: main
      api_key_vault_path: binance_sub_futures_trend
      spot_enabled: false
      futures_enabled: true
      strategy: "trend_following"
      max_leverage: 10
      max_position_usdt: 50000
      
    sub_market_making:
      type: sub
      parent: main
      api_key_vault_path: binance_sub_mm
      spot_enabled: true
      futures_enabled: false
      strategy: "market_making"

account_routing:
  default: main
  strategies:
    arbitrage: sub_spot_arb
    trend_following: sub_futures_trend
    market_making: sub_market_making
    
  balance_rules:
    min_main_balance: 100000
    max_sub_balance: 50000
    
  rebalance:
    enabled: true
    schedule: "daily"
    time: "00:00 UTC"
```

---

# 개발 단계 (Phase)

## Phase 1: 프로젝트 초기 설정
```markdown
다음 구조의 멀티거래소/멀티계좌 OMS 프로젝트를 생성해주세요:
- C++ 코어 엔진 (고성능)
- Go 서비스 레이어 (거래소 연동)
- NATS 메시징 (내부 통신)
- 멀티계좌 지원 구조
- 파일 기반 저장 (DB 없이)
프로젝트 디렉토리 구조와 초기 설정 파일들을 만들어주세요.
```

## Phase 2: 거래소 추상화 인터페이스
```markdown
멀티거래소 지원을 위한 추상화 레이어를 구현해주세요:
- Exchange 인터페이스 정의 (Go)
- 공통 Order, Position, Balance 구조체
- 거래소별 심볼 정규화 (BTC/USDT → BTCUSDT)
- Factory 패턴으로 거래소 생성 관리
```

## Phase 2.5: 멀티계좌 추상화 레이어
```markdown
멀티계좌 지원을 위한 계좌 관리 시스템을 구현해주세요:
- Account 인터페이스 정의
- 계좌 타입: Main, Sub, Strategy 구분
- 계좌별 API 키 관리 구조
- 계좌 선택 로직 (전략별, 잔고별)
- Rate Limit 분산을 위한 계좌 로테이션
```

## Phase 3: NATS 메시징 설정
```markdown
멀티계좌를 지원하는 NATS 메시징 시스템을 구성해주세요:
- NATS 서버 설정 (단일 노드로 시작)
- JetStream 활성화 (데이터 영속성)
- Subject 구조: {action}.{exchange}.{account}.{market}.{symbol}
- 계좌별 스트림 분리
- 스트림 설정: 30일 보관, 파일 저장
```

## Phase 4: C++ 멀티거래소/멀티계좌 코어 엔진
```markdown
C++로 멀티거래소/멀티계좌 주문 처리 엔진을 구현해주세요:
- Lock-free 자료구조로 초고속 처리
- Ring buffer로 거래소별/계좌별 큐 관리
- 공유 메모리 (/dev/shm) 활용
- CPU 어피니티 설정
- 메모리 내 계좌별 주문/포지션 관리
```

## Phase 5: 메모리 캐시 시스템
```markdown
Go로 멀티계좌를 지원하는 메모리 기반 캐시 시스템을 구현해주세요:
- sync.Map으로 스레드 안전 캐시
- 계좌별 캐시 분리
- TTL 지원 캐시 아이템
- 계좌별 Rate Limiter
- 세션 관리
- Redis 없이 구현
```

## Phase 6: 바이낸스 Spot 연동 (멀티계좌)
```markdown
멀티계좌를 지원하는 바이낸스 Spot connector를 만들어주세요:
- BinanceSpot struct (Exchange 인터페이스 구현)
- 계좌별 클라이언트 관리
- 주문 생성/취소/수정 (계좌 선택)
- 계좌별 잔고 조회
- WebSocket 실시간 데이터 (계좌별)
- 메모리 캐시 활용
```

## Phase 7: 바이낸스 Futures 연동 (멀티계좌)
```markdown
멀티계좌를 지원하는 바이낸스 Futures connector를 만들어주세요:
- BinanceFutures struct (Exchange 인터페이스 구현)
- 계좌별 레버리지 설정
- 계좌별 포지션 관리
- USDT-M 선물 주문
- 계좌별 마진 관리
```

## Phase 8: 파일 기반 저장 시스템
```markdown
멀티계좌 데이터를 위한 파일 시스템 기반 저장을 구현해주세요:
- 계좌별 일별 거래 로그 (JSONL 형식)
- 계좌별 시간별 상태 스냅샷
- 계좌 간 이체 기록
- 로테이션 및 압축
- grep/jq로 조회 가능한 구조
```

## Phase 9: 멀티계좌 API 키 관리
```markdown
멀티계좌 API 키 관리 시스템을 구현해주세요:
- Vault 경로: secret/exchanges/{exchange}_{market}_{account}
  예: binance_spot_main, binance_spot_sub1
- 계좌별 키 저장:
  binance_spot_main_api_key
  binance_spot_sub1_api_key
  binance_futures_sub2_api_key
- 계좌별 독립적 키 순환
- 키 사용 우선순위 설정
```

## Phase 10: 스마트 오더 라우터 (멀티계좌)
```markdown
멀티거래소/멀티계좌 스마트 라우팅을 구현해주세요:
- 최적 거래소 선택 (가격, 유동성 기반)
- 최적 계좌 선택 (잔고, Rate Limit, 전략)
- 대량 주문 분할 (계좌/거래소 분산)
- 계좌별 잔고 확인
- 차익거래 기회 감지
```

## Phase 10.5: 계좌 간 자산 관리
```markdown
서브계좌 간 자산 이체 시스템을 구현해주세요:
- 메인 → 서브계좌 자산 분배
- 서브 → 메인계좌 수익 회수
- 자동 리밸런싱 (일일/주간)
- 최소 유지 잔고 설정
- 이체 내역 기록
```

## Phase 11: 리스크 관리 엔진
```markdown
C++로 멀티계좌 실시간 리스크 관리 엔진을 구현해주세요:
- Lock-free atomic 연산
- 계좌별 포지션 한도 체크
- 전체 계좌 통합 리스크 계산
- 레버리지 제한
- 목표: 리스크 체크 < 50 마이크로초
```

## Phase 12: 멀티계좌 통합 포지션 관리
```markdown
멀티계좌 통합 포지션 시스템을 구현해주세요:
- 계좌별 포지션 추적
- 전체 계좌 통합 뷰
- 계좌별 P&L 계산
- 계좌 간 헷지 포지션 관리
- 공유 메모리로 실시간 업데이트
```

## Phase 13: gRPC API 게이트웨이
```markdown
멀티계좌를 지원하는 외부 클라이언트용 gRPC API를 구현해주세요:
- Proto 정의 (주문, 포지션, 계좌, 마켓데이터)
- 계좌별 인증/인가
- Rate limiting (계좌별)
- TLS 1.3 보안
```

## Phase 14: 모니터링 시스템
```markdown
멀티계좌 모니터링 시스템을 구현해주세요:
- 계좌별 로그 파일
- 계좌별 성능 메트릭
- 계좌별 잔고/포지션 모니터링
- 통합 대시보드
- Health check: /health/{exchange}/{account}
```

## Phase 15: 테스트 및 벤치마크
```markdown
멀티계좌 통합 테스트와 성능 벤치마크를 작성해주세요:
- 멀티계좌 동시 주문 테스트
- 계좌 간 이체 테스트
- 레이턴시 측정 (계좌별)
- Rate Limit 분산 효과 측정
- 메모리 사용량 측정
```

## Phase 16: 백테스팅 시스템
```markdown
멀티계좌 백테스팅을 구현해주세요:
- 계좌별 전략 백테스트
- 계좌 간 상관관계 분석
- 최적 계좌 배분 시뮬레이션
- 파일 기반 결과 저장
```

## Phase 17: 새 거래소 추가 템플릿
```markdown
멀티계좌를 지원하는 새 거래소 추가 템플릿을 만들어주세요:
- Exchange 인터페이스 구현 가이드
- 계좌 관리 통합
- 설정 템플릿
- 테스트 프레임워크
예시: Bybit, OKX 추가 (서브계좌 지원 여부 확인)
```

## Phase 18: 프로덕션 배포
```markdown
멀티계좌 프로덕션 환경 구성을 만들어주세요:
- systemd 서비스 파일
- CPU 코어 할당:
  - 코어 2-3: C++ 엔진
  - 코어 4: 바이낸스 메인계좌
  - 코어 5: 바이낸스 서브계좌들
  - 코어 6-7: 향후 거래소
- 계좌별 로그 로테이션
- 모니터링 스크립트
```

## 프로젝트 구조
```
crypto-oms/
├── cpp-core/           # C++ 고성능 엔진
├── go-services/        # Go 서비스 레이어
│   ├── internal/
│   │   ├── account/   # 멀티계좌 관리 (신규)
│   │   ├── exchange/
│   │   └── router/
├── configs/
│   ├── accounts.yaml  # 멀티계좌 설정 (신규)
│   ├── strategies.yaml
│   └── exchanges.yaml
├── data/              
│   ├── logs/
│   │   └── {account}/ # 계좌별 로그
│   ├── snapshots/
│   │   └── {account}/ # 계좌별 스냅샷
│   └── reports/
│       └── {account}/ # 계좌별 리포트
├── scripts/
└── README.md
```

## 성능 목표
```markdown
- 주문 처리: < 100 마이크로초
- 리스크 체크: < 50 마이크로초
- 처리량: 100,000+ orders/sec
- 계좌 전환: < 1 마이크로초
- 메모리 사용: < 1GB
- 동시 계좌 수: 200+ (바이낸스 최대)
```

## 멀티계좌 관련 주의사항
```markdown
### 바이낸스 서브계좌 제한
- 최대 200개 서브계좌
- 메인계좌에서만 서브계좌 생성 가능
- VIP 레벨에 따라 이체 수수료 차이
- 서브계좌 간 직접 이체 가능 (수수료 없음)

### Rate Limit 활용
- 계좌당 분당 1200 weight
- 10개 계좌 = 12000 weight
- 전략별 계좌 분리로 Rate Limit 회피

### 보안 고려사항
- 서브계좌별 API 권한 최소화
- 출금 권한은 메인계좌만
- 계좌별 IP 화이트리스트 설정
- 전략별 격리로 리스크 분산
```

## 주요 변경사항
```markdown
### 추가된 항목
- ✅ 멀티계좌 관리 시스템
- ✅ 계좌별 API 키 관리
- ✅ 계좌 간 자산 이체
- ✅ 계좌별 포지션/P&L 추적
- ✅ Rate Limit 분산

### 제거된 항목
- ❌ PostgreSQL (JetStream + 파일로 대체)
- ❌ Redis (메모리 + sync.Map으로 대체)
- ❌ Docker/K8s (초기 단계 불필요)
- ❌ 복잡한 클러스터링

### 강화된 항목
- ✅ 공유 메모리 (/dev/shm)
- ✅ 파일 기반 저장
- ✅ 메모리 내 캐싱
- ✅ JetStream 활용 극대화
```

