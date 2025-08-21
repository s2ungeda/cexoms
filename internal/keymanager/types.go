package keymanager

import (
	"time"
)

// APIKey represents an API key with metadata
type APIKey struct {
	ID           string            `json:"id"`
	AccountName  string            `json:"account_name"`
	Exchange     string            `json:"exchange"`
	Market       string            `json:"market"` // "spot" or "futures"
	APIKey       string            `json:"api_key"`
	APISecret    string            `json:"api_secret"`
	Passphrase   string            `json:"passphrase,omitempty"` // For exchanges that require it
	Permissions  []string          `json:"permissions"`
	IsActive     bool              `json:"is_active"`
	IsTestnet    bool              `json:"is_testnet"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	LastUsedAt   *time.Time        `json:"last_used_at,omitempty"`
	ExpiresAt    *time.Time        `json:"expires_at,omitempty"`
	RotatedFrom  string            `json:"rotated_from,omitempty"` // Previous key ID if rotated
	Tags         map[string]string `json:"tags,omitempty"`
}

// KeyMetadata contains metadata about an API key without the sensitive data
type KeyMetadata struct {
	ID          string            `json:"id"`
	AccountName string            `json:"account_name"`
	Exchange    string            `json:"exchange"`
	Market      string            `json:"market"`
	Permissions []string          `json:"permissions"`
	IsActive    bool              `json:"is_active"`
	IsTestnet   bool              `json:"is_testnet"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	LastUsedAt  *time.Time        `json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
}

// KeyRotationPolicy defines when and how keys should be rotated
type KeyRotationPolicy struct {
	Enabled           bool          `json:"enabled"`
	RotationInterval  time.Duration `json:"rotation_interval"`
	GracePeriod       time.Duration `json:"grace_period"` // Time to keep old key active
	NotifyBeforeDays  int           `json:"notify_before_days"`
	AutoRotate        bool          `json:"auto_rotate"`
	RequireApproval   bool          `json:"require_approval"`
}

// KeyUsage tracks API key usage statistics
type KeyUsage struct {
	KeyID            string    `json:"key_id"`
	AccountName      string    `json:"account_name"`
	TotalRequests    int64     `json:"total_requests"`
	SuccessRequests  int64     `json:"success_requests"`
	FailedRequests   int64     `json:"failed_requests"`
	LastRequestTime  time.Time `json:"last_request_time"`
	DailyRequests    map[string]int64 `json:"daily_requests"` // date -> count
	ErrorCodes       map[string]int64 `json:"error_codes"`    // error code -> count
}

// AccessPolicy defines who can access which keys
type AccessPolicy struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Rules       []Rule    `json:"rules"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Rule defines a single access rule
type Rule struct {
	Action      string   `json:"action"`      // "read", "write", "rotate", "delete"
	Resource    string   `json:"resource"`    // "keys/*", "keys/binance/*", etc.
	Conditions  []string `json:"conditions"`  // Additional conditions
}

// AuditLog tracks all key management operations
type AuditLog struct {
	ID          string                 `json:"id"`
	Timestamp   time.Time              `json:"timestamp"`
	Action      string                 `json:"action"`      // "create", "read", "update", "delete", "rotate"
	Resource    string                 `json:"resource"`    // Key ID or pattern
	Actor       string                 `json:"actor"`       // Who performed the action
	Success     bool                   `json:"success"`
	Details     map[string]interface{} `json:"details"`
	IPAddress   string                 `json:"ip_address,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
}

// KeyRequest represents a request for API keys
type KeyRequest struct {
	AccountName string   `json:"account_name"`
	Exchange    string   `json:"exchange"`
	Market      string   `json:"market"`
	Tags        []string `json:"tags"` // Optional filtering by tags
}

// KeyRotationRequest represents a key rotation request
type KeyRotationRequest struct {
	KeyID       string `json:"key_id"`
	Reason      string `json:"reason"`
	Immediate   bool   `json:"immediate"` // Skip grace period
	NewAPIKey   string `json:"new_api_key,omitempty"`
	NewSecret   string `json:"new_secret,omitempty"`
}

// EncryptionConfig defines encryption settings
type EncryptionConfig struct {
	Algorithm    string `json:"algorithm"`     // "aes-256-gcm"
	KeyDerivation string `json:"key_derivation"` // "pbkdf2", "argon2"
	Iterations   int    `json:"iterations"`
	SaltLength   int    `json:"salt_length"`
}

// VaultConfig contains HashiCorp Vault configuration
type VaultConfig struct {
	Address         string        `json:"address"`
	Token           string        `json:"token,omitempty"`
	TokenFile       string        `json:"token_file,omitempty"`
	RoleID          string        `json:"role_id,omitempty"`
	SecretID        string        `json:"secret_id,omitempty"`
	MountPath       string        `json:"mount_path"`
	Namespace       string        `json:"namespace,omitempty"`
	TLSConfig       *TLSConfig    `json:"tls_config,omitempty"`
	Timeout         time.Duration `json:"timeout"`
	MaxRetries      int           `json:"max_retries"`
}

// TLSConfig contains TLS configuration for Vault
type TLSConfig struct {
	CACert     string `json:"ca_cert"`
	ClientCert string `json:"client_cert"`
	ClientKey  string `json:"client_key"`
	Insecure   bool   `json:"insecure"`
}

// KeyManagerConfig contains the complete key manager configuration
type KeyManagerConfig struct {
	VaultConfig       VaultConfig       `json:"vault_config"`
	EncryptionConfig  EncryptionConfig  `json:"encryption_config"`
	RotationPolicy    KeyRotationPolicy `json:"rotation_policy"`
	AuditEnabled      bool              `json:"audit_enabled"`
	AuditLogPath      string            `json:"audit_log_path"`
	CacheEnabled      bool              `json:"cache_enabled"`
	CacheTTL          time.Duration     `json:"cache_ttl"`
	HealthCheckInterval time.Duration   `json:"health_check_interval"`
}

// KeyStats provides statistics about key management
type KeyStats struct {
	TotalKeys        int            `json:"total_keys"`
	ActiveKeys       int            `json:"active_keys"`
	ExpiredKeys      int            `json:"expired_keys"`
	KeysByExchange   map[string]int `json:"keys_by_exchange"`
	KeysByMarket     map[string]int `json:"keys_by_market"`
	LastRotation     *time.Time     `json:"last_rotation,omitempty"`
	NextRotation     *time.Time     `json:"next_rotation,omitempty"`
	HealthStatus     string         `json:"health_status"`
}

// BackupConfig defines backup settings for API keys
type BackupConfig struct {
	Enabled         bool          `json:"enabled"`
	BackupPath      string        `json:"backup_path"`
	EncryptBackup   bool          `json:"encrypt_backup"`
	BackupInterval  time.Duration `json:"backup_interval"`
	RetentionDays   int           `json:"retention_days"`
	CloudBackup     bool          `json:"cloud_backup"`
	CloudProvider   string        `json:"cloud_provider,omitempty"` // "s3", "gcs", "azure"
	CloudBucket     string        `json:"cloud_bucket,omitempty"`
}

// EmergencyAccess defines emergency access procedures
type EmergencyAccess struct {
	Enabled          bool     `json:"enabled"`
	BreakGlassUsers  []string `json:"break_glass_users"`
	RequireMultiAuth bool     `json:"require_multi_auth"`
	MinApprovers     int      `json:"min_approvers"`
	AlertChannels    []string `json:"alert_channels"` // "email", "slack", "pagerduty"
}

// Errors
const (
	ErrKeyNotFound      = "key not found"
	ErrKeyExpired       = "key expired"
	ErrKeyInactive      = "key inactive"
	ErrAccessDenied     = "access denied"
	ErrVaultUnavailable = "vault unavailable"
	ErrInvalidKey       = "invalid key"
	ErrRotationFailed   = "rotation failed"
	ErrDecryptionFailed = "decryption failed"
	ErrAuditFailed      = "audit logging failed"
)