# Multi-Exchange Cryptocurrency OMS Development Guide

## 목표
```markdown
멀티거래소 지원 고성능 암호화폐 주문관리시스템(OMS) 구축
- 초기: 바이낸스 Spot/Futures 지원
- 확장: Bybit, OKX, Upbit 등 추가 가능한 아키텍처
- 성능: 마이크로초 단위 주문 처리
- 보안: 엔터프라이즈급 API 키 관리
```

## 기술 스택
```markdown
### 핵심 엔진
- C++20: Lock-free 자료구조, Ring buffer, CPU 어피니티
- 목표 레이턴시: < 100 마이크로초

### 서비스 레이어
- Go 1.21+: 거래소 커넥터, 비즈니스 로직
- 거래소 SDK: go-binance/v2

### 메시징
- NATS Core: 초저지연 내부 통신
- NATS JetStream: 메시지 영속성, 이벤트 소싱

### API
- gRPC: 외부 클라이언트 연동 (내부는 NATS만 사용)
- Protocol Buffers: 스키마 정의

### 보안
- HashiCorp Vault: API 키 중앙 관리
- AES-256: 로컬 키 암호화
- mlock: 메모리 스왑 방지

### 데이터베이스
- PostgreSQL: 거래 기록, 감사 로그
- Redis: 실시간 캐시, 세션

### 모니터링
- Prometheus: 메트릭 수집
- Grafana: 실시간 대시보드

### 인프라
- Docker: 컨테이너화
- Kubernetes: 오케스트레이션 (프로덕션)
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
- 실시간 포지션 추적
- 통합 포지션 뷰 (전 거래소)
- P&L 실시간 계산
- 레버리지 관리 (Futures)

### 5. 리스크 관리
- 실시간 리스크 체크 (< 50μs)
- 포지션 한도 관리
- 레버리지 제한
- 손실 한도 (Stop Loss)
- 거래소별 익스포저 관리

### 6. 시장 데이터
- 실시간 가격 스트리밍
- 통합 주문북 (모든 거래소)
- 거래소 간 스프레드 모니터링
- VWAP/TWAP 계산

### 7. 보안
- API 키 자동 순환 (30일)
- 거래소별/마켓별 키 분리
- 암호화된 키 저장
- IP 화이트리스트
- 2FA 지원

### 8. 모니터링 & 알림
- 실시간 성능 메트릭
- 거래소별 상태 모니터링
- 이상 거래 감지
- Slack/Discord 알림

### 9. 백테스팅
- 과거 데이터 시뮬레이션
- 전략 성능 검증
- 거래소 간 레이턴시 시뮬레이션

### 10. 확장성
- 수평 확장 가능
- 새 거래소 쉽게 추가
- 플러그인 아키텍처
- 마이크로서비스 구조
```

## 성능 목표
```markdown
- 주문 처리: < 100 마이크로초
- 리스크 체크: < 50 마이크로초  
- 처리량: 100,000+ orders/sec
- 시장 데이터: 1,000,000+ msgs/sec
- 가동률: 99.99%
- 거래소 추가: < 1일 개발
```

## 보안 요구사항
```markdown
- API 키 암호화: AES-256-GCM
- 키 순환 주기: 30일
- 메모리 보호: mlock() 사용
- 네트워크: TLS 1.3
- 감사 로그: 모든 작업 기록
- 접근 제어: RBAC
```

---

# 개발 단계 (Phase)

## Phase 1: 프로젝트 초기 설정
```markdown
다음 구조의 멀티거래소 OMS 프로젝트를 생성해주세요:
- C++ 코어 엔진 (고성능)
- Go 서비스 레이어 (거래소 연동)
- NATS 메시징 (내부 통신)
- 멀티거래소 확장 가능한 아키텍처
프로젝트 디렉토리 구조와 초기 설정 파일들을 만들어주세요.
```

## Phase 2: 거래소 추상화 인터페이스
```markdown
멀티거래소 지원을 위한 추상화 레이어를 구현해주세요:
- Exchange 인터페이스 정의 (Go)
- 공통 Order, Position, Balance 구조체
- 거래소별 심볼 정규화 (BTC/USDT → BTCUSDT)
- Factory 패턴으로 거래소 생성 관리
- 향후 Bybit, OKX, Upbit 추가 가능한 구조
```

## Phase 3: NATS 메시징 설정
```markdown
멀티거래소용 NATS 메시징 시스템을 구성해주세요:
- Subject 구조: {action}.{exchange}.{market}.{symbol}
  예: orders.binance.spot.BTCUSDT
       market.bybit.futures.ETHUSDT
- 거래소별 스트림 분리
- JetStream으로 거래소별 메시지 영속성
```

## Phase 4: C++ 멀티거래소 코어 엔진
```markdown
C++로 멀티거래소 주문 처리 엔진을 구현해주세요:
- 거래소별 주문 관리 (Exchange enum)
- Lock-free 자료구조로 초고속 처리
- Ring buffer로 거래소별 큐 관리
- 통합 주문북 집계 (Aggregated Order Book)
- CPU 어피니티 설정
- 목표: 거래소 수 증가해도 < 100μs 유지
```

## Phase 5: 바이낸스 Spot 연동
```markdown
Exchange 인터페이스를 구현한 바이낸스 Spot connector를 만들어주세요:
- BinanceSpot struct (Exchange 인터페이스 구현)
- 주문 생성/취소/수정
- 잔고 조회
- WebSocket 실시간 데이터
- NATS 발행: market.binance.spot.{symbol}
```

## Phase 6: 바이낸스 Futures 연동
```markdown
Exchange 인터페이스를 구현한 바이낸스 Futures connector를 만들어주세요:
- BinanceFutures struct (Exchange 인터페이스 구현)
- 레버리지 설정
- 포지션 관리
- USDT-M 선물 주문
- NATS 발행: market.binance.futures.{symbol}
```

## Phase 7: 멀티거래소 API 키 관리
```markdown
멀티거래소 API 키 관리 시스템을 구현해주세요:
- Vault 경로: secret/exchanges/{exchange}_{market}
- 거래소별 키 저장 구조:
  binance_spot_api_key, binance_spot_secret_key
  binance_futures_api_key, binance_futures_secret_key
  bybit_spot_api_key (향후 확장용)
- 거래소별 독립적 키 순환
```

## Phase 8: 스마트 오더 라우터 (멀티거래소)
```markdown
멀티거래소 스마트 라우팅을 구현해주세요:
- 최적 거래소 선택 (가격, 유동성, 수수료 기반)
- 거래소 간 가격 비교
- 대량 주문 분할 (여러 거래소로 분산)
- 거래소별 잔고 확인
- 차익거래 기회 감지
```

## Phase 9: 리스크 관리 엔진
```markdown
C++로 실시간 리스크 관리 엔진을 구현해주세요:
- Lock-free atomic 연산으로 동시성 처리
- 포지션 한도 체크
- 레버리지 제한
- 가격 편차 검증
- 주문 속도 제한
- 목표: 리스크 체크 < 50 마이크로초
```

## Phase 10: 통합 포지션 관리
```markdown
멀티거래소 통합 포지션 시스템을 구현해주세요:
- 거래소별 포지션 추적
- 통합 포지션 뷰 (전체 거래소 합산)
- 거래소별 리스크 계산
- Cross-exchange 마진 계산
```

## Phase 11: gRPC API 게이트웨이
```markdown
외부 클라이언트용 gRPC API를 구현해주세요:
- Proto 정의 (주문, 포지션, 마켓데이터)
- 인증/인가
- Rate limiting
- TLS 1.3 보안
```

## Phase 12: 거래소 상태 모니터링
```markdown
멀티거래소 상태 모니터링을 구현해주세요:
- 거래소별 연결 상태
- API 응답 시간 측정
- 거래소별 Rate Limit 추적
- 거래소 장애 시 자동 전환
- Health check: /health/{exchange}
```

## Phase 13: 멀티거래소 데이터 집계
```markdown
여러 거래소의 시장 데이터를 집계하는 시스템을 구현해주세요:
- 통합 주문북 (모든 거래소 합산)
- 최적 매수/매도 가격 계산
- 거래소 간 스프레드 모니터링
- VWAP 계산 (거래소별 가중치)
```

## Phase 14: 모니터링 시스템
```markdown
Prometheus + Grafana 모니터링을 구성해주세요:
- 주문 레이턴시 메트릭 (p50, p99, p99.9)
- 처리량 통계 (orders/sec)
- CPU 코어별 사용률
- 메모리 사용량
- NATS 메시지 처리량
- 거래소별 성능 대시보드
```

## Phase 15: 테스트 및 벤치마크
```markdown
통합 테스트와 성능 벤치마크를 작성해주세요:
- Lock-free 자료구조 성능 테스트
- 레이턴시 측정 (마이크로초 단위)
- 처리량 테스트 (목표: 100,000 orders/sec)
- CPU 어피니티 효과 검증
- 멀티거래소 동시 처리 테스트
```

## Phase 16: 새 거래소 추가 템플릿
```markdown
새로운 거래소를 쉽게 추가할 수 있는 템플릿을 만들어주세요:
- Exchange 인터페이스 구현 가이드
- 거래소별 설정 템플릿 (configs/exchanges/)
- 심볼 매핑 테이블
- 테스트 프레임워크
예시: Bybit, OKX 추가 준비
```

## Phase 17: 멀티거래소 백테스팅
```markdown
멀티거래소 백테스팅 시스템을 구현해주세요:
- 거래소별 과거 데이터 재생
- 거래소 간 레이턴시 시뮬레이션
- 라우팅 전략 검증
- 차익거래 수익성 분석
```

## Phase 18: 프로덕션 배포 (멀티거래소)
```markdown
멀티거래소 프로덕션 환경을 구성해주세요:
- 거래소별 Docker 컨테이너 분리
- CPU 코어 할당:
  - 코어 2-3: C++ 엔진
  - 코어 4: 바이낸스 커넥터
  - 코어 5: Bybit 커넥터 (향후)
  - 코어 6: OKX 커넥터 (향후)
- 거래소별 모니터링 대시보드
- 커널 파라미터 튜닝
- 자동 배포 스크립트
```

