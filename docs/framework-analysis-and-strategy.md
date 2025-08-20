# 프레임워크 분석 및 확장 전략

## 목차
1. [현재 시스템의 확장성 분석](#현재-시스템의-확장성-분석)
2. [프레임워크 필요성 평가](#프레임워크-필요성-평가)
3. [기존 트레이딩 프레임워크 분석](#기존-트레이딩-프레임워크-분석)
4. [자체 프레임워크 개발 전략](#자체-프레임워크-개발-전략)
5. [단계별 구현 로드맵](#단계별-구현-로드맵)
6. [결론 및 추천사항](#결론-및-추천사항)

## 현재 시스템의 확장성 분석

### 잘 설계된 부분
1. **인터페이스 기반 아키텍처**
   ```go
   type Exchange interface {
       Connect() error
       CreateOrder() (*Order, error)
       GetBalance() (*Balance, error)
   }
   ```

2. **Factory 패턴 적용**
   - 새 거래소 추가가 용이
   - 기존 시스템 영향 최소화

3. **메시지 기반 통신**
   - NATS 주제: `{action}.{exchange}.{market}.{symbol}`
   - 느슨한 결합

### 확장 시 도전 과제
1. **거래소별 특이사항**
   - Binance: 특수한 서명 방식
   - Bybit: 다른 Position 구조
   - OKX: 독특한 계정 체계
   - Upbit: 원화 마켓 처리

2. **코드 중복 증가**
   - 거래소당 4-5개 파일
   - 비슷한 로직 반복

3. **테스트 복잡도**
   - 10개 거래소 = 40-50개 파일
   - 통합 테스트 기하급수적 증가

## 프레임워크 필요성 평가

### 현재 (거래소 2-3개)
- **프레임워크 불필요**
- 직접 구현이 더 효율적
- 성능 최적화 가능

### 중기 (거래소 5-10개)
- **경량 프레임워크 고려**
- 공통 기능 추출 필요
- 코드 생성 도구 유용

### 장기 (거래소 10개+)
- **프레임워크 필수**
- 플러그인 아키텍처
- 자동화 도구 필요

## 기존 트레이딩 프레임워크 분석

### Go 기반 프레임워크

#### 1. GoEx (⭐ 추천도: 중)
```go
exchange := goex.New(goex.BINANCE)
ticker, _ := exchange.GetTicker(goex.BTC_USDT)
```
- **장점**: 아시아 거래소 지원 우수
- **단점**: 아키텍처가 단순함

#### 2. Kelp (추천도: 낮음)
- Stellar 블록체인 전용
- 일반 거래소 부적합

### Python 기반 프레임워크 (참고용)

#### 1. Freqtrade (⭐⭐ 추천도: 높음)
- **장점**: 
  - 가장 성숙한 오픈소스
  - 백테스팅 기능 강력
  - 전략 프레임워크 우수
- **단점**: Python 성능 한계

#### 2. Hummingbot (⭐⭐⭐ 추천도: 매우 높음)
```python
class ExchangeBase(ABC):
    @abstractmethod
    async def create_order(self) -> str:
        pass
```
- **장점**: 
  - 커넥터 아키텍처 훌륭
  - 확장성 설계 우수
  - 문서화 잘됨
- **단점**: Python 기반

### C++ 기반 옵션

#### 1. QuantLib
- 파생상품 가격 계산용
- 트레이딩 실행 기능 없음

#### 2. 상용 시스템
- TT (Trading Technologies)
- CQG
- **문제**: 비공개 소스, 고가 라이선스

## 자체 프레임워크 개발 전략

### 왜 자체 개발인가?

1. **특수 요구사항**
   ```go
   type TradingFramework interface {
       // 초저지연 (마이크로초)
       ProcessOrderNative([]byte) error
       
       // 멀티 계정
       SwitchAccount(accountID string) error
       
       // WebSocket 우선
       GetWebSocketManager() WSManager
       
       // 한국 시장 특화
       HandleKRWMarket() error
   }
   ```

2. **성능 최적화**
   - C++ 코어 직접 연동
   - Zero-copy 메시지 전달
   - Lock-free 자료구조

### 경량 프레임워크 설계

```go
// internal/framework/core.go
package framework

// 모든 거래소가 구현해야 할 인터페이스
type ExchangeConnector interface {
    // 필수 구현
    GetCapabilities() Capabilities
    Connect(config Config) error
    Subscribe(channels []Channel) error
    
    // 선택적 구현
    BaseConnector  // Embedding으로 공통 기능 제공
}

// 공통 기능 제공
type BaseConnector struct {
    ws         *websocket.Conn
    httpClient *http.Client
    rateLimit  RateLimiter
    logger     *zap.Logger
    
    // 재사용 가능한 메서드들
    func (b *BaseConnector) Reconnect() error
    func (b *BaseConnector) Heartbeat() error
    func (b *BaseConnector) HandleError(err error)
}
```

### 실제 구현 예시

```go
// pkg/framework/base_exchange.go
type BaseExchange struct {
    // 모든 거래소 공통 컴포넌트
    name        string
    httpClient  *RateLimitedClient
    wsManager   *WebSocketManager
    orderBook   *OrderBookManager
    
    // 전략 패턴으로 주입
    Authenticator AuthStrategy
    OrderParser   ParseStrategy
    EventHandler  EventStrategy
}

// 거래소별 구현 (Bybit 예시)
type BybitConnector struct {
    *BaseExchange  // 공통 기능 상속
    
    // Bybit 특화 필드
    inverseMode bool
    testnet     bool
}

// Bybit만의 특수 구현
func (b *BybitConnector) SignRequest(req *Request) error {
    // Bybit 특화 서명 로직
    return b.Authenticator.SignWithTimestamp(req, "time_stamp")
}
```

## 단계별 구현 로드맵

### Phase 1: 공통 컴포넌트 추출 (1-2주)
```bash
pkg/common/
├── websocket/
│   ├── manager.go      # WebSocket 연결 관리
│   ├── reconnect.go    # 자동 재연결
│   └── heartbeat.go    # 하트비트
├── auth/
│   ├── signature.go    # 서명 생성
│   └── hmac.go        # HMAC 구현
└── ratelimit/
    └── limiter.go      # Rate limiting
```

### Phase 2: 코드 생성기 개발 (1주)
```bash
# 새 거래소 스캐폴딩 명령
$ go run cmd/codegen/main.go new-exchange bybit

Generated files:
✓ services/bybit/connector.go
✓ services/bybit/websocket.go
✓ services/bybit/types.go
✓ services/bybit/config.yaml
✓ services/bybit/tests/connector_test.go
```

### Phase 3: 플러그인 시스템 (2주)
```go
// 플러그인 인터페이스
type Plugin interface {
    Name() string
    Version() string
    Init(core *Core) error
    Start() error
    Stop() error
}

// 동적 로딩
type PluginManager struct {
    plugins map[string]Plugin
    
    func LoadPlugin(path string) error
    func EnablePlugin(name string) error
    func DisablePlugin(name string) error
}
```

### Phase 4: 설정 기반 차이점 관리 (1주)
```yaml
# exchanges.yaml
exchanges:
  binance:
    api:
      signature_method: "hmac_sha256"
      timestamp_param: "timestamp"
      timestamp_in_header: false
    types:
      order_id: "int64"
      price: "string"
    features:
      websocket_orders: true
      multi_account: true
      
  bybit:
    api:
      signature_method: "hmac_sha256_sorted"
      timestamp_param: "time_stamp"
      timestamp_in_header: true
    types:
      order_id: "string"
      price: "float64"
    features:
      websocket_orders: true
      multi_account: false
```

## 프레임워크 도입 결정 기준

### 즉시 도입 신호
- [ ] 코드 중복률 > 50%
- [ ] 새 거래소 추가 시간 > 1주
- [ ] 버그 발생률 증가
- [ ] 팀 규모 > 5명

### 도입 보류 신호
- [ ] 거래소 < 5개
- [ ] 성능 요구사항 엄격
- [ ] 팀 규모 < 3명
- [ ] 빠른 프로토타이핑 필요

## 하이브리드 접근법 (추천)

### 개념
- 핵심 기능은 직접 구현
- 좋은 아이디어는 차용
- 성능 크리티컬 부분은 최적화

### 구현 예시
```go
type HybridFramework struct {
    // 핵심 - 직접 구현
    Core         *CoreEngine      // C++ 연동
    EventBus     *NATSBus        // 고성능 메시징
    OrderManager *OrderManager    // 주문 관리
    
    // 차용 - 오픈소스 참고
    BacktestEngine *BacktestEngine  // Freqtrade 아이디어
    StrategyBase   *StrategyBase    // Hummingbot 패턴
    Indicators     *TechIndicators  // TA-Lib 래핑
}
```

## 결론 및 추천사항

### 단기 전략 (0-3개월)
1. **프레임워크 없이 진행**
2. 공통 코드를 `pkg/common/`으로 추출
3. 3-4개 거래소 구현 후 패턴 파악
4. 문서화 및 테스트 강화

### 중기 전략 (3-6개월)
1. **경량 자체 프레임워크 개발 시작**
2. Hummingbot 커넥터 아키텍처 참고
3. 코드 생성기 구축
4. 플러그인 시스템 설계

### 장기 전략 (6개월+)
1. 완전한 플러그인 아키텍처
2. 백테스팅 엔진 통합
3. 전략 프레임워크 구축
4. 모니터링/분석 도구

### 핵심 원칙
> "Make it work, make it right, make it fast"

1. **작동하게 만들기** (현재)
2. **올바르게 만들기** (리팩토링)
3. **빠르게 만들기** (최적화)

### 의사결정 체크리스트

프레임워크 도입 전 확인사항:
- [ ] 현재 아키텍처의 한계점이 명확한가?
- [ ] 성능 요구사항을 만족할 수 있는가?
- [ ] 팀이 학습하고 유지보수할 수 있는가?
- [ ] ROI(투자 대비 수익)가 긍정적인가?

---

*이 문서는 2025-08-20 기준으로 작성되었습니다.*
*프로젝트 진행에 따라 정기적으로 업데이트가 필요합니다.*