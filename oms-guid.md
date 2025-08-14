# Multi-Exchange Cryptocurrency OMS Development Guide (Simplified)

## 목표
```markdown
멀티거래소 지원 고성능 암호화폐 주문관리시스템(OMS) 구축
- 초기: 바이낸스 Spot/Futures 지원
- 확장: Bybit, OKX, Upbit 등 추가 가능한 아키텍처
- 성능: 마이크로초 단위 주문 처리
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
- HashiCorp Vault: API 키 중앙 관리
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

### 2. 주문 관리
- 초고속 주문 처리 (< 100μs)
- 주문 생성/수정/취소
- 일괄 주문 처리
- 조건부 주문 (Stop Loss, Take Profit)

### 3. 스마트 라우팅
- 최적 거래소 자동 선택
- 대량 주문 분할 실행
- 거래소 간 차익거래 감지
- 수수료 최적화

### 4. 포지션 관리
- 실시간 포지션 추적 (메모리)
- 통합 포지션 뷰 (전 거래소)
- P&L 실시간 계산
- 레버리지 관리 (Futures)

### 5. 리스크 관리
- 실시간 리스크 체크 (< 50μs)
- 포지션 한도 관리
- 레버리지 제한
- 손실 한도 (Stop Loss)

### 6. 데이터 저장
- JetStream: 주문/거래 이벤트 (30일)
- 파일: 일별 거래 로그 (JSON/CSV)
- 공유 메모리: 실시간 데이터
- 스냅샷: 주기적 상태 백업
```

## 데이터 저장 전략
```markdown
### 실시간 데이터 (메모리)
- 활성 주문
- 현재 포지션
- 주문북
- 잔고

### 이벤트 스트림 (JetStream)
- 주문 이벤트: orders.{exchange}.{market}.{symbol}
- 거래 체결: trades.{exchange}.{market}.{symbol}
- 포지션 변경: positions.{exchange}.{market}
- 보관 기간: 30일

### 백업/아카이브 (파일)
- 일별 거래 로그: /data/logs/2024/01/15/trades.jsonl
- 시간별 스냅샷: /data/snapshots/2024/01/15/14/state.json
- P&L 보고서: /data/reports/2024/01/pnl.csv

### 선택적 (필요시 추가)
- PostgreSQL: 복잡한 분석, 규제 보고용
- Redis: 분산 시스템 전환 시
```

---

# 개발 단계 (Phase)

## Phase 1: 프로젝트 초기 설정
```markdown
다음 구조의 멀티거래소 OMS 프로젝트를 생성해주세요:
- C++ 코어 엔진 (고성능)
- Go 서비스 레이어 (거래소 연동)
- NATS 메시징 (내부 통신)
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

## Phase 3: NATS 메시징 설정
```markdown
NATS 메시징 시스템을 구성해주세요:
- NATS 서버 설정 (단일 노드로 시작)
- JetStream 활성화 (데이터 영속성)
- Subject 구조: {action}.{exchange}.{market}.{symbol}
- 스트림 설정: 30일 보관, 파일 저장
```

## Phase 4: C++ 멀티거래소 코어 엔진
```markdown
C++로 멀티거래소 주문 처리 엔진을 구현해주세요:
- Lock-free 자료구조로 초고속 처리
- Ring buffer로 거래소별 큐 관리
- 공유 메모리 (/dev/shm) 활용
- CPU 어피니티 설정
- 메모리 내 주문/포지션 관리
```

## Phase 5: 메모리 캐시 시스템
```markdown
Go로 메모리 기반 캐시 시스템을 구현해주세요:
- sync.Map으로 스레드 안전 캐시
- TTL 지원 캐시 아이템
- Rate Limiter (메모리 카운터)
- 세션 관리
- Redis 없이 구현
```

## Phase 6: 바이낸스 Spot 연동
```markdown
Exchange 인터페이스를 구현한 바이낸스 Spot connector를 만들어주세요:
- BinanceSpot struct (Exchange 인터페이스 구현)
- 주문 생성/취소/수정
- 잔고 조회
- WebSocket 실시간 데이터
- 메모리 캐시 활용
```

## Phase 7: 바이낸스 Futures 연동
```markdown
Exchange 인터페이스를 구현한 바이낸스 Futures connector를 만들어주세요:
- BinanceFutures struct (Exchange 인터페이스 구현)
- 레버리지 설정
- 포지션 관리
- USDT-M 선물 주문
```

## Phase 8: 파일 기반 저장 시스템
```markdown
파일 시스템 기반 데이터 저장을 구현해주세요:
- 일별 거래 로그 (JSONL 형식)
- 시간별 상태 스냅샷
- 로테이션 및 압축
- grep/jq로 조회 가능한 구조
- PostgreSQL 없이 구현
```

## Phase 9: API 키 보안 관리
```markdown
API 키 관리 시스템을 구현해주세요:
- Vault 또는 암호화 파일 저장
- 메모리 내 복호화
- 거래소별/마켓별 키 분리
- 자동 키 순환 (30일)
```

## Phase 10: 스마트 오더 라우터
```markdown
멀티거래소 스마트 라우팅을 구현해주세요:
- 최적 거래소 선택 (가격, 유동성 기반)
- 대량 주문 분할
- 메모리 내 잔고 확인
- 차익거래 기회 감지
```

## Phase 11: 리스크 관리 엔진
```markdown
C++로 실시간 리스크 관리 엔진을 구현해주세요:
- Lock-free atomic 연산
- 메모리 내 포지션 한도 체크
- 레버리지 제한
- 목표: 리스크 체크 < 50 마이크로초
```

## Phase 12: 통합 포지션 관리
```markdown
멀티거래소 통합 포지션 시스템을 구현해주세요:
- 공유 메모리로 포지션 추적
- 실시간 P&L 계산
- 거래소별 리스크 계산
- 파일 스냅샷 저장
```

## Phase 13: gRPC API 게이트웨이
```markdown
외부 클라이언트용 gRPC API를 구현해주세요:
- Proto 정의 (주문, 포지션, 마켓데이터)
- 인증/인가
- Rate limiting (메모리 카운터)
- TLS 1.3 보안
```

## Phase 14: 모니터링 시스템
```markdown
간단한 모니터링 시스템을 구현해주세요:
- 로그 파일 기반 모니터링
- 자체 메트릭 수집 (메모리)
- Health check 엔드포인트
- 선택적: Prometheus 연동
```

## Phase 15: 테스트 및 벤치마크
```markdown
통합 테스트와 성능 벤치마크를 작성해주세요:
- Lock-free 자료구조 성능 테스트
- 레이턴시 측정 (마이크로초 단위)
- 메모리 사용량 측정
- 파일 I/O 성능 테스트
```

## Phase 16: 백테스팅 시스템
```markdown
JetStream 기반 백테스팅을 구현해주세요:
- 과거 이벤트 재생
- 전략 성능 검증
- 파일 기반 결과 저장
```

## Phase 17: 새 거래소 추가 템플릿
```markdown
새로운 거래소를 쉽게 추가할 수 있는 템플릿을 만들어주세요:
- Exchange 인터페이스 구현 가이드
- 설정 템플릿
- 테스트 프레임워크
예시: Bybit, OKX 추가 준비
```

## Phase 18: 프로덕션 배포
```markdown
프로덕션 환경 구성을 만들어주세요:
- systemd 서비스 파일
- CPU 코어 할당:
  - 코어 2-3: C++ 엔진
  - 코어 4: 바이낸스 커넥터
  - 코어 5-6: 향후 거래소
- 로그 로테이션 설정
- 모니터링 스크립트
```

## 프로젝트 구조
```
crypto-oms/
├── cpp-core/           # C++ 고성능 엔진
├── go-services/        # Go 서비스 레이어
├── configs/            # 설정 파일
├── data/              
│   ├── logs/          # 거래 로그
│   ├── snapshots/     # 상태 스냅샷
│   └── reports/       # 보고서
├── scripts/           # 유틸리티 스크립트
└── README.md
```

## 성능 목표
```markdown
- 주문 처리: < 100 마이크로초
- 리스크 체크: < 50 마이크로초
- 처리량: 100,000+ orders/sec
- 메모리 사용: < 1GB
- 파일 I/O: 비동기 처리
```

## 주요 변경사항
```markdown
### 제거된 항목
- ❌ PostgreSQL (JetStream + 파일로 대체)
- ❌ Redis (메모리 + sync.Map으로 대체)
- ❌ Docker/K8s (초기 단계 불필요)
- ❌ 복잡한 클러스터링

### 추가/강화된 항목
- ✅ 공유 메모리 (/dev/shm)
- ✅ 파일 기반 저장
- ✅ 메모리 내 캐싱
- ✅ JetStream 활용 극대화
```

---

