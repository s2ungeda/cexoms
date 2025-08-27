# Microservices Architecture

Detailed documentation of the mExOms microservices architecture and design patterns.

## Overview

The mExOms platform follows a microservices architecture pattern where each service is responsible for a specific business capability. Services communicate through well-defined interfaces using gRPC for synchronous calls and NATS for asynchronous messaging.

## Service Architecture Principles

### 1. Domain-Driven Design (DDD)
- Each service represents a bounded context
- Clear aggregate boundaries
- Ubiquitous language within each domain
- Anti-corruption layers between contexts

### 2. Service Autonomy
- Independent deployment capability
- Own data storage (no shared databases)
- Failure isolation
- Technology flexibility

### 3. API-First Design
- Protocol Buffers for API definition
- Versioned APIs with backward compatibility
- Clear service contracts
- Generated client libraries

### 4. Observability
- Distributed tracing (OpenTelemetry)
- Structured logging
- Metrics collection
- Health checks and readiness probes

## Service Catalog

### Core Services

#### 1. Authentication Service
```yaml
Service: auth-service
Technology: Go 1.21
Database: PostgreSQL
Cache: Redis
Port: 50051 (gRPC), 8081 (HTTP)
Dependencies:
  - PostgreSQL (user storage)
  - Redis (session cache)
  - Vault (secret management)
```

**Responsibilities:**
- User authentication (JWT, OAuth2)
- Session management
- API key generation
- Multi-factor authentication
- Permission validation

**API Example:**
```protobuf
service AuthService {
  rpc Login(LoginRequest) returns (LoginResponse);
  rpc Logout(LogoutRequest) returns (LogoutResponse);
  rpc ValidateToken(ValidateTokenRequest) returns (ValidateTokenResponse);
  rpc RefreshToken(RefreshTokenRequest) returns (RefreshTokenResponse);
  rpc CreateAPIKey(CreateAPIKeyRequest) returns (CreateAPIKeyResponse);
  rpc RevokeAPIKey(RevokeAPIKeyRequest) returns (RevokeAPIKeyResponse);
}

message LoginRequest {
  string username = 1;
  string password = 2;
  string mfa_code = 3;
  string device_id = 4;
}

message LoginResponse {
  string access_token = 1;
  string refresh_token = 2;
  int64 expires_in = 3;
  User user = 4;
}
```

#### 2. Order Service
```yaml
Service: order-service
Technology: Go 1.21
Database: PostgreSQL
Cache: Redis
Port: 50052 (gRPC), 8082 (HTTP)
Dependencies:
  - Core Engine (order processing)
  - Auth Service (authentication)
  - Risk Service (pre-trade checks)
  - NATS (event streaming)
```

**Responsibilities:**
- Order validation and submission
- Order lifecycle management
- Order history and search
- Order modifications and cancellations
- Event publishing

**Implementation:**
```go
type OrderService struct {
    pb.UnimplementedOrderServiceServer
    engine      *engine.TradingEngine
    authClient  auth.AuthServiceClient
    riskClient  risk.RiskServiceClient
    publisher   *nats.Conn
    repository  *OrderRepository
    cache       *redis.Client
    logger      *zap.Logger
}

func (s *OrderService) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.CreateOrderResponse, error) {
    // Extract user from context
    userClaims, err := s.extractUserClaims(ctx)
    if err != nil {
        return nil, status.Errorf(codes.Unauthenticated, "invalid authentication: %v", err)
    }
    
    // Validate order request
    if err := s.validateOrderRequest(req); err != nil {
        return nil, status.Errorf(codes.InvalidArgument, "invalid order: %v", err)
    }
    
    // Pre-trade risk check
    riskReq := &risk.CheckOrderRequest{
        UserId:    userClaims.UserID,
        AccountId: req.AccountId,
        Symbol:    req.Symbol,
        Side:      req.Side,
        Quantity:  req.Quantity,
        Price:     req.Price,
    }
    
    riskResp, err := s.riskClient.CheckOrder(ctx, riskReq)
    if err != nil {
        return nil, status.Errorf(codes.Internal, "risk check failed: %v", err)
    }
    
    if !riskResp.Approved {
        return nil, status.Errorf(codes.FailedPrecondition, "risk check rejected: %s", riskResp.Reason)
    }
    
    // Submit to trading engine
    order := &engine.Order{
        ID:        generateOrderID(),
        UserID:    userClaims.UserID,
        AccountID: req.AccountId,
        Symbol:    req.Symbol,
        Side:      engine.Side(req.Side),
        Type:      engine.OrderType(req.Type),
        Quantity:  req.Quantity,
        Price:     req.Price,
        TimeInForce: engine.TimeInForce(req.TimeInForce),
        Timestamp: time.Now(),
    }
    
    // Send to engine through shared memory IPC
    if err := s.engine.SubmitOrder(order); err != nil {
        return nil, status.Errorf(codes.Internal, "engine submission failed: %v", err)
    }
    
    // Store in database
    if err := s.repository.CreateOrder(ctx, order); err != nil {
        s.logger.Error("failed to persist order", zap.Error(err))
    }
    
    // Publish order created event
    event := &events.OrderCreated{
        OrderId:   order.ID,
        UserId:    order.UserID,
        Symbol:    order.Symbol,
        Side:      string(order.Side),
        Type:      string(order.Type),
        Quantity:  order.Quantity,
        Price:     order.Price,
        Timestamp: order.Timestamp.UnixNano(),
    }
    
    if err := s.publishEvent("orders.created", event); err != nil {
        s.logger.Error("failed to publish event", zap.Error(err))
    }
    
    return &pb.CreateOrderResponse{
        OrderId: order.ID,
        Status:  "PENDING",
        Timestamp: order.Timestamp.UnixNano(),
    }, nil
}
```

#### 3. Market Data Service
```yaml
Service: market-data-service
Technology: Go 1.21
Database: TimescaleDB
Cache: Redis
Port: 50053 (gRPC), 8083 (HTTP), 9001 (WebSocket)
Dependencies:
  - Exchange Connectors
  - NATS (data distribution)
  - Redis (real-time cache)
```

**Responsibilities:**
- Market data aggregation
- Real-time price streaming
- Order book management
- Historical data API
- Data normalization

**WebSocket Handler:**
```go
type MarketDataHub struct {
    clients    map[string]*Client
    register   chan *Client
    unregister chan *Client
    broadcast  chan *MarketUpdate
    
    natsConn   *nats.Conn
    cache      *redis.Client
    
    mu sync.RWMutex
}

func (h *MarketDataHub) handleWebSocket(w http.ResponseWriter, r *http.Request) {
    upgrader := websocket.Upgrader{
        ReadBufferSize:  1024,
        WriteBufferSize: 1024,
        CheckOrigin: func(r *http.Request) bool {
            return true // Configure appropriately for production
        },
    }
    
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("WebSocket upgrade failed: %v", err)
        return
    }
    
    client := &Client{
        id:           generateClientID(),
        conn:         conn,
        send:         make(chan []byte, 256),
        hub:          h,
        subscriptions: make(map[string]bool),
    }
    
    h.register <- client
    
    go client.writePump()
    go client.readPump()
}

func (h *MarketDataHub) subscribeToNATS() {
    // Subscribe to all market data updates
    h.natsConn.Subscribe("market.*.ticker", func(msg *nats.Msg) {
        var update MarketUpdate
        if err := json.Unmarshal(msg.Data, &update); err != nil {
            log.Printf("Failed to unmarshal market update: %v", err)
            return
        }
        
        // Broadcast to all subscribed clients
        h.broadcast <- &update
    })
    
    // Subscribe to orderbook updates
    h.natsConn.Subscribe("market.*.orderbook", func(msg *nats.Msg) {
        var orderbook OrderBookUpdate
        if err := json.Unmarshal(msg.Data, &orderbook); err != nil {
            log.Printf("Failed to unmarshal orderbook update: %v", err)
            return
        }
        
        // Update cache
        key := fmt.Sprintf("orderbook:%s", orderbook.Symbol)
        data, _ := json.Marshal(orderbook)
        h.cache.Set(context.Background(), key, data, 5*time.Second)
        
        // Broadcast to subscribed clients
        h.broadcastOrderBook(&orderbook)
    })
}
```

#### 4. Risk Service
```yaml
Service: risk-service
Technology: Go 1.21
Database: PostgreSQL
Cache: Redis
Port: 50054 (gRPC), 8084 (HTTP)
Dependencies:
  - Position Service
  - Market Data Service
  - Account Service
```

**Responsibilities:**
- Pre-trade risk validation
- Real-time position monitoring
- Exposure calculation
- VaR computation
- Margin requirements
- Risk alerts

**Risk Check Implementation:**
```go
type RiskService struct {
    pb.UnimplementedRiskServiceServer
    
    positionClient position.PositionServiceClient
    marketClient   market.MarketDataServiceClient
    accountClient  account.AccountServiceClient
    
    riskEngine     *RiskEngine
    cache          *redis.Client
    alertManager   *AlertManager
}

func (s *RiskService) CheckOrder(ctx context.Context, req *pb.CheckOrderRequest) (*pb.CheckOrderResponse, error) {
    // Get account information
    accountResp, err := s.accountClient.GetAccount(ctx, &account.GetAccountRequest{
        AccountId: req.AccountId,
    })
    if err != nil {
        return nil, err
    }
    
    // Get current positions
    positionsResp, err := s.positionClient.GetPositions(ctx, &position.GetPositionsRequest{
        AccountId: req.AccountId,
    })
    if err != nil {
        return nil, err
    }
    
    // Get current market price
    marketResp, err := s.marketClient.GetTicker(ctx, &market.GetTickerRequest{
        Symbol: req.Symbol,
    })
    if err != nil {
        return nil, err
    }
    
    // Perform risk checks
    checks := []RiskCheck{
        s.checkAccountBalance,
        s.checkPositionLimits,
        s.checkDailyLossLimit,
        s.checkConcentrationLimit,
        s.checkLeverageLimit,
    }
    
    for _, check := range checks {
        result := check(req, accountResp.Account, positionsResp.Positions, marketResp.Price)
        if !result.Passed {
            return &pb.CheckOrderResponse{
                Approved: false,
                Reason:   result.Reason,
                RiskScore: result.RiskScore,
            }, nil
        }
    }
    
    // Calculate risk metrics
    metrics := s.calculateRiskMetrics(req, accountResp.Account, positionsResp.Positions)
    
    return &pb.CheckOrderResponse{
        Approved:  true,
        RiskScore: metrics.RiskScore,
        Metrics:   metrics,
    }, nil
}

func (s *RiskService) checkPositionLimits(req *pb.CheckOrderRequest, account *account.Account, 
    positions []*position.Position, marketPrice float64) RiskCheckResult {
    
    // Find existing position
    var currentPosition float64
    for _, pos := range positions {
        if pos.Symbol == req.Symbol {
            currentPosition = pos.Quantity
            break
        }
    }
    
    // Calculate new position
    orderQuantity := req.Quantity
    if req.Side == "SELL" {
        orderQuantity = -orderQuantity
    }
    newPosition := currentPosition + orderQuantity
    
    // Check against limits
    limits := s.getPositionLimits(account.Type)
    if math.Abs(newPosition) > limits.MaxPositionSize {
        return RiskCheckResult{
            Passed: false,
            Reason: fmt.Sprintf("Position size %.2f exceeds limit %.2f", 
                math.Abs(newPosition), limits.MaxPositionSize),
            RiskScore: 1.0,
        }
    }
    
    // Check position value
    positionValue := math.Abs(newPosition) * marketPrice
    if positionValue > limits.MaxPositionValue {
        return RiskCheckResult{
            Passed: false,
            Reason: fmt.Sprintf("Position value $%.2f exceeds limit $%.2f", 
                positionValue, limits.MaxPositionValue),
            RiskScore: 0.9,
        }
    }
    
    return RiskCheckResult{Passed: true, RiskScore: 0.1}
}
```

#### 5. Position Service
```yaml
Service: position-service
Technology: Go 1.21
Database: PostgreSQL
Cache: Redis
Port: 50055 (gRPC), 8085 (HTTP)
Dependencies:
  - Market Data Service
  - NATS (trade events)
```

**Responsibilities:**
- Real-time position tracking
- P&L calculation
- Position aggregation
- Multi-exchange positions
- Position history

#### 6. Account Service
```yaml
Service: account-service
Technology: Go 1.21
Database: PostgreSQL
Cache: Redis
Port: 50056 (gRPC), 8086 (HTTP)
Dependencies:
  - Auth Service
  - Vault (API keys)
```

**Responsibilities:**
- Account management
- Balance tracking
- Exchange account linking
- API key management
- Account permissions

### Exchange Connector Services

Each exchange has a dedicated connector service that implements the common exchange interface:

```go
type ExchangeConnector interface {
    // Connection management
    Connect(ctx context.Context) error
    Disconnect() error
    IsConnected() bool
    
    // Order management
    PlaceOrder(ctx context.Context, order *Order) (*OrderResponse, error)
    CancelOrder(ctx context.Context, orderID string) error
    GetOrder(ctx context.Context, orderID string) (*Order, error)
    GetOpenOrders(ctx context.Context, symbol string) ([]*Order, error)
    
    // Market data
    SubscribeTicker(symbol string, handler TickerHandler) error
    SubscribeOrderBook(symbol string, depth int, handler OrderBookHandler) error
    GetTicker(ctx context.Context, symbol string) (*Ticker, error)
    
    // Account information
    GetBalances(ctx context.Context) (map[string]*Balance, error)
    GetPositions(ctx context.Context) ([]*Position, error)
}
```

#### Binance Connector
```go
type BinanceConnector struct {
    config      *Config
    wsClient    *websocket.Conn
    restClient  *http.Client
    
    orderBook   map[string]*OrderBook
    mu          sync.RWMutex
    
    reconnector *Reconnector
    rateLimiter *RateLimiter
    
    logger      *zap.Logger
}

func (b *BinanceConnector) Connect(ctx context.Context) error {
    // Initialize REST client
    b.restClient = &http.Client{
        Timeout: 10 * time.Second,
        Transport: &http.Transport{
            MaxIdleConns:        100,
            MaxIdleConnsPerHost: 10,
        },
    }
    
    // Connect WebSocket
    dialer := websocket.Dialer{
        Proxy:            http.ProxyFromEnvironment,
        HandshakeTimeout: 45 * time.Second,
    }
    
    wsURL := fmt.Sprintf("%s/ws/%s", b.config.WebSocketURL, b.generateListenKey())
    conn, _, err := dialer.DialContext(ctx, wsURL, nil)
    if err != nil {
        return fmt.Errorf("websocket dial failed: %w", err)
    }
    
    b.wsClient = conn
    
    // Start message handler
    go b.handleWebSocketMessages()
    
    // Start listen key renewal
    go b.renewListenKey()
    
    return nil
}

func (b *BinanceConnector) PlaceOrder(ctx context.Context, order *Order) (*OrderResponse, error) {
    // Rate limiting
    if err := b.rateLimiter.Wait(ctx); err != nil {
        return nil, err
    }
    
    // Build request
    params := url.Values{}
    params.Set("symbol", order.Symbol)
    params.Set("side", string(order.Side))
    params.Set("type", string(order.Type))
    params.Set("quantity", fmt.Sprintf("%.8f", order.Quantity))
    
    if order.Type == OrderTypeLIMIT {
        params.Set("price", fmt.Sprintf("%.2f", order.Price))
        params.Set("timeInForce", string(order.TimeInForce))
    }
    
    // Sign request
    timestamp := time.Now().UnixMilli()
    params.Set("timestamp", fmt.Sprintf("%d", timestamp))
    
    signature := b.signRequest(params)
    params.Set("signature", signature)
    
    // Send request
    req, err := http.NewRequestWithContext(ctx, "POST", 
        b.config.APIURL+"/api/v3/order", 
        strings.NewReader(params.Encode()))
    if err != nil {
        return nil, err
    }
    
    req.Header.Set("X-MBX-APIKEY", b.config.APIKey)
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    
    resp, err := b.restClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    // Parse response
    var binanceResp BinanceOrderResponse
    if err := json.NewDecoder(resp.Body).Decode(&binanceResp); err != nil {
        return nil, err
    }
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("order rejected: %s", binanceResp.Msg)
    }
    
    return &OrderResponse{
        OrderID:      binanceResp.OrderID,
        ClientID:     binanceResp.ClientOrderID,
        Status:       b.translateOrderStatus(binanceResp.Status),
        FilledQty:    binanceResp.ExecutedQty,
        Price:        binanceResp.Price,
    }, nil
}
```

## Service Communication Patterns

### Synchronous Communication (gRPC)

Used for:
- Request-response patterns
- Real-time queries
- Service-to-service calls requiring immediate response

```protobuf
syntax = "proto3";

package mexoms.common;

import "google/protobuf/timestamp.proto";

// Common error response
message Error {
  string code = 1;
  string message = 2;
  map<string, string> details = 3;
}

// Pagination request
message PaginationRequest {
  int32 page = 1;
  int32 page_size = 2;
  string sort_by = 3;
  bool ascending = 4;
}

// Pagination response
message PaginationResponse {
  int32 page = 1;
  int32 page_size = 2;
  int32 total_pages = 3;
  int64 total_items = 4;
}
```

### Asynchronous Communication (NATS)

Used for:
- Event streaming
- Market data distribution
- Service decoupling
- Background processing

```go
// Event definitions
type Event struct {
    ID        string    `json:"id"`
    Type      string    `json:"type"`
    Source    string    `json:"source"`
    Timestamp time.Time `json:"timestamp"`
    Data      json.RawMessage `json:"data"`
}

// Order events
type OrderCreatedEvent struct {
    OrderID   string  `json:"order_id"`
    UserID    string  `json:"user_id"`
    Symbol    string  `json:"symbol"`
    Side      string  `json:"side"`
    Type      string  `json:"type"`
    Quantity  float64 `json:"quantity"`
    Price     float64 `json:"price,omitempty"`
}

type OrderFilledEvent struct {
    OrderID      string  `json:"order_id"`
    UserID       string  `json:"user_id"`
    Symbol       string  `json:"symbol"`
    Side         string  `json:"side"`
    FilledQty    float64 `json:"filled_qty"`
    FilledPrice  float64 `json:"filled_price"`
    Remaining    float64 `json:"remaining_qty"`
    ExecutionID  string  `json:"execution_id"`
}

// Event publisher
type EventPublisher struct {
    conn *nats.Conn
    js   nats.JetStreamContext
}

func (p *EventPublisher) PublishOrderCreated(event *OrderCreatedEvent) error {
    subject := fmt.Sprintf("orders.created.%s", event.Symbol)
    
    evt := Event{
        ID:        uuid.New().String(),
        Type:      "order.created",
        Source:    "order-service",
        Timestamp: time.Now(),
    }
    
    data, _ := json.Marshal(event)
    evt.Data = data
    
    msg, _ := json.Marshal(evt)
    
    _, err := p.js.Publish(subject, msg)
    return err
}

// Event subscriber
type EventSubscriber struct {
    conn *nats.Conn
    js   nats.JetStreamContext
}

func (s *EventSubscriber) SubscribeToOrderEvents(handler func(*OrderCreatedEvent)) error {
    _, err := s.js.Subscribe(
        "orders.created.*",
        func(msg *nats.Msg) {
            var evt Event
            if err := json.Unmarshal(msg.Data, &evt); err != nil {
                log.Printf("Failed to unmarshal event: %v", err)
                msg.Nak()
                return
            }
            
            var orderEvent OrderCreatedEvent
            if err := json.Unmarshal(evt.Data, &orderEvent); err != nil {
                log.Printf("Failed to unmarshal order event: %v", err)
                msg.Nak()
                return
            }
            
            handler(&orderEvent)
            msg.Ack()
        },
        nats.Durable("position-service"),
        nats.DeliverAll(),
        nats.AckExplicit(),
    )
    
    return err
}
```

## Service Mesh and Networking

### Istio Configuration

```yaml
# Virtual Service for order-service
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: order-service
  namespace: mexoms
spec:
  hosts:
  - order-service
  http:
  - match:
    - headers:
        x-version:
          exact: v2
    route:
    - destination:
        host: order-service
        subset: v2
      weight: 100
  - route:
    - destination:
        host: order-service
        subset: v1
      weight: 90
    - destination:
        host: order-service
        subset: v2
      weight: 10

---
# Destination Rule
apiVersion: networking.istio.io/v1beta1
kind: DestinationRule
metadata:
  name: order-service
  namespace: mexoms
spec:
  host: order-service
  trafficPolicy:
    connectionPool:
      tcp:
        maxConnections: 100
      http:
        http1MaxPendingRequests: 50
        h2UpgradePolicy: UPGRADE
    loadBalancer:
      simple: LEAST_REQUEST
    outlierDetection:
      consecutive5xxErrors: 5
      interval: 30s
      baseEjectionTime: 30s
  subsets:
  - name: v1
    labels:
      version: v1
  - name: v2
    labels:
      version: v2
```

### Circuit Breaker Pattern

```go
type CircuitBreaker struct {
    name          string
    maxRequests   uint32
    interval      time.Duration
    timeout       time.Duration
    failureRatio  float64
    
    state         State
    failures      uint32
    successes     uint32
    lastFailure   time.Time
    counts        *Counts
    
    mu sync.Mutex
}

func (cb *CircuitBreaker) Execute(fn func() error) error {
    cb.mu.Lock()
    
    if cb.state == StateOpen {
        if time.Since(cb.lastFailure) > cb.timeout {
            cb.state = StateHalfOpen
            cb.resetCounts()
        } else {
            cb.mu.Unlock()
            return ErrCircuitOpen
        }
    }
    
    if cb.state == StateHalfOpen && cb.counts.total >= cb.maxRequests {
        cb.mu.Unlock()
        return ErrTooManyRequests
    }
    
    cb.counts.total++
    cb.mu.Unlock()
    
    err := fn()
    
    cb.mu.Lock()
    defer cb.mu.Unlock()
    
    if err != nil {
        cb.failures++
        cb.lastFailure = time.Now()
        
        if cb.state == StateHalfOpen || 
           float64(cb.failures)/float64(cb.counts.total) > cb.failureRatio {
            cb.state = StateOpen
        }
        
        return err
    }
    
    cb.successes++
    
    if cb.state == StateHalfOpen && cb.successes >= cb.maxRequests {
        cb.state = StateClosed
        cb.resetCounts()
    }
    
    return nil
}

// Usage in service calls
func (s *OrderService) callRiskService(ctx context.Context, req *risk.CheckOrderRequest) (*risk.CheckOrderResponse, error) {
    var resp *risk.CheckOrderResponse
    var err error
    
    cbErr := s.riskServiceCB.Execute(func() error {
        resp, err = s.riskClient.CheckOrder(ctx, req)
        return err
    })
    
    if cbErr != nil {
        if cbErr == ErrCircuitOpen {
            // Fallback logic
            return s.fallbackRiskCheck(req)
        }
        return nil, cbErr
    }
    
    return resp, err
}
```

## Service Deployment

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: order-service
  namespace: mexoms
  labels:
    app: order-service
    version: v1
spec:
  replicas: 3
  selector:
    matchLabels:
      app: order-service
      version: v1
  template:
    metadata:
      labels:
        app: order-service
        version: v1
      annotations:
        sidecar.istio.io/inject: "true"
        prometheus.io/scrape: "true"
        prometheus.io/port: "9090"
    spec:
      serviceAccountName: order-service
      containers:
      - name: order-service
        image: mexoms/order-service:v1.2.3
        ports:
        - name: grpc
          containerPort: 50052
        - name: http
          containerPort: 8082
        - name: metrics
          containerPort: 9090
        env:
        - name: ENV
          value: "production"
        - name: LOG_LEVEL
          value: "info"
        - name: DB_HOST
          valueFrom:
            secretKeyRef:
              name: order-service-db
              key: host
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: order-service-db
              key: password
        - name: NATS_URL
          value: "nats://nats:4222"
        - name: REDIS_URL
          value: "redis://redis:6379"
        - name: JAEGER_AGENT_HOST
          value: "jaeger-agent.istio-system"
        - name: JAEGER_AGENT_PORT
          value: "6831"
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
        livenessProbe:
          grpc:
            port: 50052
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          grpc:
            port: 50052
          initialDelaySeconds: 5
          periodSeconds: 5
        volumeMounts:
        - name: config
          mountPath: /etc/order-service
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: order-service-config
```

### Service Configuration

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: order-service-config
  namespace: mexoms
data:
  config.yaml: |
    server:
      grpc_port: 50052
      http_port: 8082
      metrics_port: 9090
      
    database:
      driver: postgres
      max_open_conns: 25
      max_idle_conns: 5
      conn_max_lifetime: 5m
      
    redis:
      max_retries: 3
      pool_size: 10
      dial_timeout: 5s
      read_timeout: 3s
      write_timeout: 3s
      
    nats:
      max_reconnect: 10
      reconnect_wait: 2s
      
    engine:
      ipc_path: /dev/shm/mexoms-engine
      timeout: 100ms
      
    rate_limiting:
      requests_per_second: 100
      burst: 200
      
    circuit_breaker:
      failure_ratio: 0.5
      max_requests: 10
      interval: 10s
      timeout: 30s
```

## Monitoring and Observability

### Metrics Collection

```go
var (
    orderCounter = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "mexoms_orders_total",
            Help: "Total number of orders processed",
        },
        []string{"status", "exchange", "symbol"},
    )
    
    orderDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "mexoms_order_duration_seconds",
            Help: "Order processing duration",
            Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
        },
        []string{"operation"},
    )
    
    activeConnections = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "mexoms_websocket_connections",
            Help: "Number of active WebSocket connections",
        },
        []string{"service"},
    )
)
```

### Distributed Tracing

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/trace"
    "go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
)

func initTracing() (func(), error) {
    // Create Jaeger exporter
    exp, err := jaeger.New(jaeger.WithCollectorEndpoint(
        jaeger.WithEndpoint("http://jaeger-collector:14268/api/traces"),
    ))
    if err != nil {
        return nil, err
    }
    
    // Create trace provider
    tp := tracesdk.NewTracerProvider(
        tracesdk.WithBatcher(exp),
        tracesdk.WithResource(resource.NewWithAttributes(
            semconv.SchemaURL,
            semconv.ServiceNameKey.String("order-service"),
            semconv.ServiceVersionKey.String("v1.2.3"),
        )),
    )
    
    otel.SetTracerProvider(tp)
    
    return func() {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        tp.Shutdown(ctx)
    }, nil
}

// Instrument gRPC server
func createGRPCServer() *grpc.Server {
    return grpc.NewServer(
        grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()),
        grpc.StreamInterceptor(otelgrpc.StreamServerInterceptor()),
    )
}

// Manual span creation
func (s *OrderService) processOrder(ctx context.Context, order *Order) error {
    tr := otel.Tracer("order-service")
    ctx, span := tr.Start(ctx, "processOrder")
    defer span.End()
    
    span.SetAttributes(
        attribute.String("order.id", order.ID),
        attribute.String("order.symbol", order.Symbol),
        attribute.Float64("order.quantity", order.Quantity),
    )
    
    // Validation span
    _, validationSpan := tr.Start(ctx, "validateOrder")
    err := s.validateOrder(order)
    validationSpan.End()
    if err != nil {
        span.RecordError(err)
        return err
    }
    
    // Risk check span
    _, riskSpan := tr.Start(ctx, "checkRisk")
    err = s.checkRisk(ctx, order)
    riskSpan.End()
    if err != nil {
        span.RecordError(err)
        return err
    }
    
    return nil
}
```

### Health Checks

```go
type HealthChecker struct {
    checks []HealthCheck
}

type HealthCheck struct {
    Name    string
    Check   func(context.Context) error
    Timeout time.Duration
}

func (h *HealthChecker) CheckHealth(ctx context.Context) map[string]string {
    results := make(map[string]string)
    
    for _, check := range h.checks {
        checkCtx, cancel := context.WithTimeout(ctx, check.Timeout)
        err := check.Check(checkCtx)
        cancel()
        
        if err != nil {
            results[check.Name] = fmt.Sprintf("unhealthy: %v", err)
        } else {
            results[check.Name] = "healthy"
        }
    }
    
    return results
}

// Service health checks
func setupHealthChecks(s *OrderService) *HealthChecker {
    return &HealthChecker{
        checks: []HealthCheck{
            {
                Name:    "database",
                Timeout: 5 * time.Second,
                Check: func(ctx context.Context) error {
                    return s.db.PingContext(ctx)
                },
            },
            {
                Name:    "redis",
                Timeout: 2 * time.Second,
                Check: func(ctx context.Context) error {
                    return s.cache.Ping(ctx).Err()
                },
            },
            {
                Name:    "nats",
                Timeout: 2 * time.Second,
                Check: func(ctx context.Context) error {
                    if !s.natsConn.IsConnected() {
                        return fmt.Errorf("nats disconnected")
                    }
                    return nil
                },
            },
            {
                Name:    "engine",
                Timeout: 100 * time.Millisecond,
                Check: func(ctx context.Context) error {
                    return s.engine.Ping()
                },
            },
        },
    }
}
```

## Security Considerations

### Service-to-Service Authentication

```go
// mTLS configuration
func createTLSConfig(certFile, keyFile, caFile string) (*tls.Config, error) {
    cert, err := tls.LoadX509KeyPair(certFile, keyFile)
    if err != nil {
        return nil, err
    }
    
    caCert, err := ioutil.ReadFile(caFile)
    if err != nil {
        return nil, err
    }
    
    caCertPool := x509.NewCertPool()
    caCertPool.AppendCertsFromPEM(caCert)
    
    return &tls.Config{
        Certificates: []tls.Certificate{cert},
        RootCAs:      caCertPool,
        ClientCAs:    caCertPool,
        ClientAuth:   tls.RequireAndVerifyClientCert,
        MinVersion:   tls.VersionTLS13,
    }, nil
}

// Service token validation
type ServiceAuthInterceptor struct {
    validator TokenValidator
}

func (i *ServiceAuthInterceptor) Unary() grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, 
                handler grpc.UnaryHandler) (interface{}, error) {
        
        // Extract service token from metadata
        md, ok := metadata.FromIncomingContext(ctx)
        if !ok {
            return nil, status.Error(codes.Unauthenticated, "missing metadata")
        }
        
        tokens := md.Get("service-token")
        if len(tokens) == 0 {
            return nil, status.Error(codes.Unauthenticated, "missing service token")
        }
        
        // Validate service token
        claims, err := i.validator.ValidateServiceToken(tokens[0])
        if err != nil {
            return nil, status.Error(codes.Unauthenticated, "invalid service token")
        }
        
        // Add service info to context
        ctx = context.WithValue(ctx, "service", claims.Service)
        ctx = context.WithValue(ctx, "permissions", claims.Permissions)
        
        return handler(ctx, req)
    }
}
```

---

*This document provides detailed microservices architecture. For system-wide architecture, see the [System Overview](./system-overview.md).*