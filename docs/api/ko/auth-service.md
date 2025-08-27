# 인증 서비스(AuthService) API 참조

인증 서비스는 mExOms 플랫폼의 인증, 인가, API 키 관리를 처리합니다.

## 개요

- **서비스**: `oms.v1.AuthService`
- **기본 URL**: `grpc://localhost:8080` (개발환경)
- **인증**: 초기 인증용 공개 엔드포인트, 키 관리용 JWT 필요
- **보안**: TLS 1.3 필수, MFA 지원

## 인증 방법

mExOms는 두 가지 인증 방법을 지원합니다:

1. **JWT 토큰**: 대화형 세션용 단기 토큰 (1-24시간)
2. **API 키**: 프로그래밍 방식 액세스용 장기 자격 증명

## 메서드

### Authenticate (인증)

사용자를 인증하고 JWT 토큰을 반환합니다.

**요청**: `AuthRequest`
```protobuf
message AuthRequest {
  string username = 1;       // 사용자명 또는 이메일
  string password = 2;       // 사용자 비밀번호
  string mfa_code = 3;       // 선택사항: MFA 활성화 시 MFA 코드
  int64 expires_in = 4;      // 선택사항: 토큰 수명(초)
  repeated string scopes = 5; // 선택사항: 요청된 권한
}
```

**응답**: `AuthResponse`
```protobuf
message AuthResponse {
  string access_token = 1;    // JWT 액세스 토큰
  string refresh_token = 2;   // 새로고침 토큰
  int64 expires_at = 3;       // 토큰 만료 타임스탬프
  repeated string scopes = 4;  // 부여된 권한
  UserInfo user_info = 5;     // 사용자 프로필 정보
}

message UserInfo {
  string user_id = 1;
  string username = 2;
  string email = 3;
  repeated string roles = 4;
  bool mfa_enabled = 5;
  int64 last_login = 6;
}
```

**예제**:
```bash
grpcurl -plaintext \
  -d '{
    "username": "trader@example.com",
    "password": "SecurePassword123!",
    "expires_in": 86400,
    "scopes": ["trading", "read_positions"]
  }' \
  localhost:8080 oms.v1.AuthService/Authenticate
```

**응답**:
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIs...",
  "refresh_token": "rt_abc123...",
  "expires_at": 1703980800,
  "scopes": ["trading", "read_positions"],
  "user_info": {
    "user_id": "user-123",
    "username": "trader",
    "email": "trader@example.com",
    "roles": ["trader"],
    "mfa_enabled": true,
    "last_login": 1703894400
  }
}
```

### RefreshToken (토큰 새로고침)

만료된 JWT 토큰을 새로고침합니다.

**요청**: `RefreshTokenRequest`
```protobuf
message RefreshTokenRequest {
  string refresh_token = 1;   // 초기 인증의 새로고침 토큰
  int64 expires_in = 2;       // 선택사항: 새 토큰 수명
}
```

**응답**: `RefreshTokenResponse`
```protobuf
message RefreshTokenResponse {
  string access_token = 1;    // 새 JWT 액세스 토큰
  string refresh_token = 2;   // 새 새로고침 토큰 (선택사항)
  int64 expires_at = 3;       // 새 만료 타임스탬프
}
```

**예제**:
```bash
grpcurl -plaintext \
  -d '{"refresh_token": "rt_abc123..."}' \
  localhost:8080 oms.v1.AuthService/RefreshToken
```

### CreateAPIKey (API 키 생성)

프로그래밍 방식 액세스를 위한 새 API 키를 생성합니다.

**요청**: `CreateAPIKeyRequest`
```protobuf
message CreateAPIKeyRequest {
  string name = 1;            // 키의 설명적 이름
  repeated string scopes = 2; // 이 키의 권한
  int64 expires_at = 3;       // 선택사항: 만료 타임스탬프
  repeated string ip_whitelist = 4; // 선택사항: IP 제한
  RateLimitConfig rate_limit = 5;   // 선택사항: 커스텀 속도 제한
}

message RateLimitConfig {
  int32 requests_per_minute = 1;
  int32 requests_per_hour = 2;
  int32 requests_per_day = 3;
}
```

**응답**: `CreateAPIKeyResponse`
```protobuf
message CreateAPIKeyResponse {
  string api_key_id = 1;      // 키 식별자
  string api_key = 2;         // 실제 API 키 (한 번만 표시)
  string api_secret = 3;      // API 시크릿 (한 번만 표시)
  APIKeyInfo key_info = 4;    // 키 메타데이터
}

message APIKeyInfo {
  string api_key_id = 1;
  string name = 2;
  repeated string scopes = 3;
  int64 created_at = 4;
  int64 expires_at = 5;
  int64 last_used = 6;
  bool is_active = 7;
  repeated string ip_whitelist = 8;
  RateLimitConfig rate_limit = 9;
}
```

**예제**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "name": "거래 봇 API 키",
    "scopes": ["trading", "read_positions", "read_orders"],
    "ip_whitelist": ["203.0.113.1", "203.0.113.2"],
    "rate_limit": {
      "requests_per_minute": 1000,
      "requests_per_hour": 50000
    }
  }' \
  localhost:8080 oms.v1.AuthService/CreateAPIKey
```

⚠️ **보안 경고**: API 키와 시크릿은 생성 시에만 한 번 표시됩니다. 안전하게 저장하세요!

### ListAPIKeys (API 키 목록)

인증된 사용자의 모든 API 키를 나열합니다.

**요청**: `ListAPIKeysRequest`
```protobuf
message ListAPIKeysRequest {
  bool include_inactive = 1;  // 폐기된 키 포함
  int32 limit = 2;           // 최대 결과 수 (기본값 50)
  string page_token = 3;     // 페이지네이션 토큰
}
```

**응답**: `ListAPIKeysResponse`
```protobuf
message ListAPIKeysResponse {
  repeated APIKeyInfo api_keys = 1;
  string next_page_token = 2;
  int32 total_count = 3;
}
```

**예제**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{"include_inactive": false, "limit": 10}' \
  localhost:8080 oms.v1.AuthService/ListAPIKeys
```

### RevokeAPIKey (API 키 폐기)

API 키를 폐기하여 즉시 액세스를 비활성화합니다.

**요청**: `RevokeAPIKeyRequest`
```protobuf
message RevokeAPIKeyRequest {
  string api_key_id = 1;      // 폐기할 키 ID
}
```

**응답**: `RevokeAPIKeyResponse`
```protobuf
message RevokeAPIKeyResponse {
  string api_key_id = 1;
  bool success = 2;
  int64 revoked_at = 3;
}
```

**예제**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{"api_key_id": "key-789"}' \
  localhost:8080 oms.v1.AuthService/RevokeAPIKey
```

## API 키 사용

생성된 API 키는 JWT 토큰 대신 인증에 사용할 수 있습니다.

### HTTP 헤더 방법
```bash
grpcurl -plaintext \
  -H "X-API-Key: ak_1234567890abcdef" \
  -H "X-API-Secret: as_fedcba0987654321" \
  localhost:8080 oms.v1.OrderService/ListOrders
```

### HMAC 서명 방법 (권장)
```bash
# HMAC-SHA256 서명 생성
timestamp=$(date +%s)
payload="GET/api/v1/orders${timestamp}"
signature=$(echo -n "$payload" | openssl dgst -sha256 -hmac "$api_secret" -hex | cut -d' ' -f2)

grpcurl -plaintext \
  -H "X-API-Key: ak_1234567890abcdef" \
  -H "X-Timestamp: $timestamp" \
  -H "X-Signature: $signature" \
  localhost:8080 oms.v1.OrderService/ListOrders
```

## 권한 스코프

사용 가능한 권한 스코프:

| 스코프 | 설명 | 서비스 |
|--------|------|---------|
| `trading` | 주문 생성, 취소, 수정 | OrderService |
| `read_orders` | 주문 내역 및 상태 조회 | OrderService |
| `read_positions` | 포지션 및 손익 조회 | PositionService |
| `read_balance` | 계정 잔고 조회 | AccountService |
| `transfers` | 계정 간 내부 이체 | AccountService |
| `read_market_data` | 시장 데이터 액세스 | MarketDataService |
| `manage_strategies` | 전략 생성/수정 | StrategyService |
| `read_strategies` | 전략 성과 조회 | StrategyService |
| `admin` | 전체 관리자 액세스 | 모든 서비스 |

## 다중 인증(MFA)

추가 보안을 위해 MFA를 활성화하세요:

### TOTP 설정
```bash
# MFA 활성화 (QR 코드 반환)
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  localhost:8080 oms.v1.AuthService/EnableMFA

# 설정 확인
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{"mfa_code": "123456"}' \
  localhost:8080 oms.v1.AuthService/VerifyMFA
```

### MFA를 통한 인증
```bash
grpcurl -plaintext \
  -d '{
    "username": "trader@example.com",
    "password": "SecurePassword123!",
    "mfa_code": "123456"
  }' \
  localhost:8080 oms.v1.AuthService/Authenticate
```

## 속도 제한

스코프별 기본 속도 제한:

| 스코프 | 분당 요청 | 시간당 요청 |
|--------|-----------|-------------|
| `trading` | 1000 | 50000 |
| `read_*` | 2000 | 100000 |
| `admin` | 500 | 20000 |

응답에서 반환되는 속도 제한 헤더:
- `X-RateLimit-Limit`: 윈도우당 요청 수
- `X-RateLimit-Remaining`: 남은 요청 수
- `X-RateLimit-Reset`: 윈도우 재설정 시간

## 보안 모범 사례

### 1. 토큰 저장
```go
// 토큰을 안전하게 저장
type TokenStore struct {
    accessToken  string
    refreshToken string
    expiresAt    time.Time
}

// 사용 전 만료 확인
if time.Now().After(t.expiresAt) {
    token, err = refreshToken(t.refreshToken)
}
```

### 2. API 키 관리
```go
// API 키를 정기적으로 교체
func rotateAPIKeys(client AuthServiceClient) error {
    // 새 키 생성
    newKey, err := client.CreateAPIKey(ctx, &CreateAPIKeyRequest{...})
    if err != nil {
        return err
    }
    
    // 설정 업데이트
    updateConfig(newKey.ApiKey, newKey.ApiSecret)
    
    // 성공적인 배포 후 이전 키 폐기
    return client.RevokeAPIKey(ctx, &RevokeAPIKeyRequest{
        ApiKeyId: oldKeyId,
    })
}
```

### 3. IP 화이트리스트
```go
// 프로덕션 키에는 항상 IP 제한 사용
apiKeyReq := &CreateAPIKeyRequest{
    Name: "프로덕션 거래 봇",
    Scopes: []string{"trading", "read_positions"},
    IpWhitelist: []string{
        "203.0.113.1",    // 주 서버
        "203.0.113.2",    // 백업 서버
    },
}
```

### 4. 최소 권한 원칙
```go
// 필요한 최소한의 스코프만 부여
scopes := []string{"read_orders", "read_positions"} // 읽기 전용 봇
if needsTrading {
    scopes = append(scopes, "trading")
}
```

## 오류 코드

| 코드 | 상태 | 설명 |
|------|------|------|
| 3 | INVALID_ARGUMENT | 잘못된 사용자명/비밀번호 |
| 7 | PERMISSION_DENIED | 잘못된 토큰 또는 권한 부족 |
| 8 | RESOURCE_EXHAUSTED | 속도 제한 초과 |
| 14 | UNAVAILABLE | 인증 서비스 사용 불가 |
| 16 | UNAUTHENTICATED | 토큰 만료 또는 무효 |

## 관련 문서

- [보안 아키텍처](../security/README.md)
- [속도 제한](../security/rate-limiting.md)  
- [다중 인증](../security/mfa.md)