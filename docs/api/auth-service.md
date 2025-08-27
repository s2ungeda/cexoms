# AuthService API Reference

The AuthService handles authentication, authorization, and API key management for the mExOms platform.

## Overview

- **Service**: `oms.v1.AuthService`
- **Base URL**: `grpc://localhost:8080` (development)
- **Authentication**: Public endpoints for initial auth, JWT required for key management
- **Security**: TLS 1.3 required, MFA supported

## Authentication Methods

mExOms supports two authentication methods:

1. **JWT Tokens**: Short-lived tokens (1-24 hours) for interactive sessions
2. **API Keys**: Long-lived credentials for programmatic access

## Methods

### Authenticate

Authenticates a user and returns a JWT token.

**Request**: `AuthRequest`
```protobuf
message AuthRequest {
  string username = 1;       // Username or email
  string password = 2;       // User password
  string mfa_code = 3;       // Optional: MFA code if enabled
  int64 expires_in = 4;      // Optional: token lifetime in seconds
  repeated string scopes = 5; // Optional: requested permissions
}
```

**Response**: `AuthResponse`
```protobuf
message AuthResponse {
  string access_token = 1;    // JWT access token
  string refresh_token = 2;   // Refresh token
  int64 expires_at = 3;       // Token expiration timestamp
  repeated string scopes = 4;  // Granted permissions
  UserInfo user_info = 5;     // User profile information
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

**Example**:
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

**Response**:
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

### RefreshToken

Refreshes an expired JWT token.

**Request**: `RefreshTokenRequest`
```protobuf
message RefreshTokenRequest {
  string refresh_token = 1;   // Refresh token from initial auth
  int64 expires_in = 2;       // Optional: new token lifetime
}
```

**Response**: `RefreshTokenResponse`
```protobuf
message RefreshTokenResponse {
  string access_token = 1;    // New JWT access token
  string refresh_token = 2;   // New refresh token (optional)
  int64 expires_at = 3;       // New expiration timestamp
}
```

**Example**:
```bash
grpcurl -plaintext \
  -d '{"refresh_token": "rt_abc123..."}' \
  localhost:8080 oms.v1.AuthService/RefreshToken
```

### CreateAPIKey

Creates a new API key for programmatic access.

**Request**: `CreateAPIKeyRequest`
```protobuf
message CreateAPIKeyRequest {
  string name = 1;            // Descriptive name for the key
  repeated string scopes = 2; // Permissions for this key
  int64 expires_at = 3;       // Optional: expiration timestamp
  repeated string ip_whitelist = 4; // Optional: IP restrictions
  RateLimitConfig rate_limit = 5;   // Optional: custom rate limits
}

message RateLimitConfig {
  int32 requests_per_minute = 1;
  int32 requests_per_hour = 2;
  int32 requests_per_day = 3;
}
```

**Response**: `CreateAPIKeyResponse`
```protobuf
message CreateAPIKeyResponse {
  string api_key_id = 1;      // Key identifier
  string api_key = 2;         // The actual API key (shown once)
  string api_secret = 3;      // API secret (shown once)
  APIKeyInfo key_info = 4;    // Key metadata
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

**Example**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{
    "name": "Trading Bot API Key",
    "scopes": ["trading", "read_positions", "read_orders"],
    "ip_whitelist": ["203.0.113.1", "203.0.113.2"],
    "rate_limit": {
      "requests_per_minute": 1000,
      "requests_per_hour": 50000
    }
  }' \
  localhost:8080 oms.v1.AuthService/CreateAPIKey
```

**Response**:
```json
{
  "api_key_id": "key-789",
  "api_key": "ak_1234567890abcdef",
  "api_secret": "as_fedcba0987654321",
  "key_info": {
    "api_key_id": "key-789",
    "name": "Trading Bot API Key",
    "scopes": ["trading", "read_positions", "read_orders"],
    "created_at": 1703894400,
    "expires_at": 0,
    "is_active": true,
    "ip_whitelist": ["203.0.113.1", "203.0.113.2"],
    "rate_limit": {
      "requests_per_minute": 1000,
      "requests_per_hour": 50000
    }
  }
}
```

⚠️ **Security Warning**: API keys and secrets are only shown once during creation. Store them securely!

### ListAPIKeys

Lists all API keys for the authenticated user.

**Request**: `ListAPIKeysRequest`
```protobuf
message ListAPIKeysRequest {
  bool include_inactive = 1;  // Include revoked keys
  int32 limit = 2;           // Max results (default 50)
  string page_token = 3;     // Pagination token
}
```

**Response**: `ListAPIKeysResponse`
```protobuf
message ListAPIKeysResponse {
  repeated APIKeyInfo api_keys = 1;
  string next_page_token = 2;
  int32 total_count = 3;
}
```

**Example**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{"include_inactive": false, "limit": 10}' \
  localhost:8080 oms.v1.AuthService/ListAPIKeys
```

### RevokeAPIKey

Revokes an API key, immediately disabling access.

**Request**: `RevokeAPIKeyRequest`
```protobuf
message RevokeAPIKeyRequest {
  string api_key_id = 1;      // Key ID to revoke
}
```

**Response**: `RevokeAPIKeyResponse`
```protobuf
message RevokeAPIKeyResponse {
  string api_key_id = 1;
  bool success = 2;
  int64 revoked_at = 3;
}
```

**Example**:
```bash
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{"api_key_id": "key-789"}' \
  localhost:8080 oms.v1.AuthService/RevokeAPIKey
```

## Using API Keys

Once created, API keys can be used for authentication instead of JWT tokens.

### HTTP Header Method
```bash
grpcurl -plaintext \
  -H "X-API-Key: ak_1234567890abcdef" \
  -H "X-API-Secret: as_fedcba0987654321" \
  localhost:8080 oms.v1.OrderService/ListOrders
```

### HMAC Signature Method (Recommended)
```bash
# Generate HMAC-SHA256 signature
timestamp=$(date +%s)
payload="GET/api/v1/orders${timestamp}"
signature=$(echo -n "$payload" | openssl dgst -sha256 -hmac "$api_secret" -hex | cut -d' ' -f2)

grpcurl -plaintext \
  -H "X-API-Key: ak_1234567890abcdef" \
  -H "X-Timestamp: $timestamp" \
  -H "X-Signature: $signature" \
  localhost:8080 oms.v1.OrderService/ListOrders
```

## Permission Scopes

Available permission scopes:

| Scope | Description | Services |
|-------|-------------|----------|
| `trading` | Create, cancel, modify orders | OrderService |
| `read_orders` | View order history and status | OrderService |
| `read_positions` | View positions and P&L | PositionService |
| `read_balance` | View account balances | AccountService |
| `transfers` | Internal transfers between accounts | AccountService |
| `read_market_data` | Access market data | MarketDataService |
| `manage_strategies` | Create/modify strategies | StrategyService |
| `read_strategies` | View strategy performance | StrategyService |
| `admin` | Full administrative access | All services |

## Multi-Factor Authentication (MFA)

Enable MFA for additional security:

### Setup TOTP
```bash
# Enable MFA (returns QR code)
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  localhost:8080 oms.v1.AuthService/EnableMFA

# Verify setup
grpcurl -plaintext -H "Authorization: Bearer <token>" \
  -d '{"mfa_code": "123456"}' \
  localhost:8080 oms.v1.AuthService/VerifyMFA
```

### Authentication with MFA
```bash
grpcurl -plaintext \
  -d '{
    "username": "trader@example.com",
    "password": "SecurePassword123!",
    "mfa_code": "123456"
  }' \
  localhost:8080 oms.v1.AuthService/Authenticate
```

## Rate Limiting

Default rate limits by scope:

| Scope | Requests/Min | Requests/Hour |
|-------|--------------|---------------|
| `trading` | 1000 | 50000 |
| `read_*` | 2000 | 100000 |
| `admin` | 500 | 20000 |

Rate limit headers returned in responses:
- `X-RateLimit-Limit`: Requests per window
- `X-RateLimit-Remaining`: Remaining requests
- `X-RateLimit-Reset`: Window reset time

## Security Best Practices

### 1. Token Storage
```go
// Store tokens securely
type TokenStore struct {
    accessToken  string
    refreshToken string
    expiresAt    time.Time
}

// Check expiration before use
if time.Now().After(t.expiresAt) {
    token, err = refreshToken(t.refreshToken)
}
```

### 2. API Key Management
```go
// Rotate API keys regularly
func rotateAPIKeys(client AuthServiceClient) error {
    // Create new key
    newKey, err := client.CreateAPIKey(ctx, &CreateAPIKeyRequest{...})
    if err != nil {
        return err
    }
    
    // Update configuration
    updateConfig(newKey.ApiKey, newKey.ApiSecret)
    
    // Revoke old key after successful deployment
    return client.RevokeAPIKey(ctx, &RevokeAPIKeyRequest{
        ApiKeyId: oldKeyId,
    })
}
```

### 3. IP Whitelisting
```go
// Always use IP restrictions for production keys
apiKeyReq := &CreateAPIKeyRequest{
    Name: "Production Trading Bot",
    Scopes: []string{"trading", "read_positions"},
    IpWhitelist: []string{
        "203.0.113.1",    // Primary server
        "203.0.113.2",    // Backup server
    },
}
```

### 4. Least Privilege
```go
// Grant minimal required scopes
scopes := []string{"read_orders", "read_positions"} // Read-only bot
if needsTrading {
    scopes = append(scopes, "trading")
}
```

## Error Codes

| Code | Status | Description |
|------|--------|-------------|
| 3 | INVALID_ARGUMENT | Invalid username/password |
| 7 | PERMISSION_DENIED | Invalid token or insufficient permissions |
| 8 | RESOURCE_EXHAUSTED | Rate limit exceeded |
| 14 | UNAVAILABLE | Authentication service unavailable |
| 16 | UNAUTHENTICATED | Token expired or invalid |

## Related Documentation

- [Security Architecture](../security/README.md)
- [Rate Limiting](../security/rate-limiting.md)
- [Multi-Factor Authentication](../security/mfa.md)