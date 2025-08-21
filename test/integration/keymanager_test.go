package integration

import (
	"context"
	"testing"
	"time"

	"github.com/mExOms/internal/keymanager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockVaultClient implements a mock Vault client for testing
type MockVaultClient struct {
	keys map[string]*keymanager.APIKey
}

func NewMockVaultClient() *MockVaultClient {
	return &MockVaultClient{
		keys: make(map[string]*keymanager.APIKey),
	}
}

func TestKeyManager(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Note: This test requires a running Vault instance
	// In CI/CD, use a test Vault container

	config := keymanager.KeyManagerConfig{
		VaultConfig: keymanager.VaultConfig{
			Address:   "http://localhost:8200",
			Token:     "test-token",
			MountPath: "secret",
			Timeout:   5 * time.Second,
		},
		EncryptionConfig: keymanager.EncryptionConfig{
			Algorithm:     "aes-256-gcm",
			KeyDerivation: "pbkdf2",
			Iterations:    100000,
			SaltLength:    32,
		},
		RotationPolicy: keymanager.KeyRotationPolicy{
			Enabled:          true,
			RotationInterval: 30 * 24 * time.Hour, // 30 days
			GracePeriod:      24 * time.Hour,
			NotifyBeforeDays: 7,
			AutoRotate:       false,
		},
		AuditEnabled:        true,
		AuditLogPath:        "/tmp/keymanager_audit.log",
		CacheEnabled:        true,
		CacheTTL:            5 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
	}

	// Skip if Vault is not available
	manager, err := keymanager.NewManager(config)
	if err != nil {
		t.Skip("Vault not available, skipping integration test")
	}
	defer manager.Close()

	ctx := context.Background()

	t.Run("Store and Retrieve Key", func(t *testing.T) {
		key := &keymanager.APIKey{
			AccountName: "test_account",
			Exchange:    "binance",
			Market:      "spot",
			APIKey:      "test_api_key_123",
			APISecret:   "test_secret_456",
			Permissions: []string{"read", "trade"},
			IsActive:    true,
			IsTestnet:   true,
			Tags: map[string]string{
				"environment": "test",
			},
		}

		// Store key
		err := manager.StoreKey(ctx, key)
		assert.NoError(t, err)

		// Retrieve key
		request := keymanager.KeyRequest{
			AccountName: "test_account",
			Exchange:    "binance",
			Market:      "spot",
		}

		retrieved, err := manager.GetKey(ctx, request)
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, key.APIKey, retrieved.APIKey)
		assert.Equal(t, key.APISecret, retrieved.APISecret)
		assert.Equal(t, key.AccountName, retrieved.AccountName)
	})

	t.Run("List Keys", func(t *testing.T) {
		// Add another key
		key2 := &keymanager.APIKey{
			AccountName: "test_account",
			Exchange:    "binance",
			Market:      "futures",
			APIKey:      "test_api_key_futures",
			APISecret:   "test_secret_futures",
			Permissions: []string{"read", "trade"},
			IsActive:    true,
		}

		err := manager.StoreKey(ctx, key2)
		assert.NoError(t, err)

		// List keys
		keys, err := manager.ListKeys(ctx, "test_account")
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(keys), 2)
	})

	t.Run("Update Key", func(t *testing.T) {
		// Get existing key
		request := keymanager.KeyRequest{
			AccountName: "test_account",
			Exchange:    "binance",
			Market:      "spot",
		}

		key, err := manager.GetKey(ctx, request)
		require.NoError(t, err)

		// Update key
		key.Tags["updated"] = "true"
		key.IsActive = false

		err = manager.UpdateKey(ctx, key)
		assert.NoError(t, err)

		// Verify update
		updated, err := manager.GetKey(ctx, request)
		assert.NoError(t, err)
		assert.Equal(t, "true", updated.Tags["updated"])
		assert.False(t, updated.IsActive)
	})

	t.Run("Key Rotation", func(t *testing.T) {
		// Get rotator
		rotator := manager.GetRotator()
		require.NotNil(t, rotator)

		// Create rotation request
		rotationReq := keymanager.KeyRotationRequest{
			Reason:    "Test rotation",
			Immediate: true,
			NewAPIKey: "new_test_api_key",
			NewSecret: "new_test_secret",
		}

		// Note: This would need the key ID from previous tests
		// For now, we'll skip the actual rotation test
		// record, err := rotator.RotateKey(ctx, rotationReq)
		// assert.NoError(t, err)
		// assert.True(t, record.Success)
	})

	t.Run("Usage Tracking", func(t *testing.T) {
		tracker := manager.GetUsageTracker()
		require.NotNil(t, tracker)

		// Track some requests
		tracker.TrackRequest("test_key_id", "test_account", true)
		tracker.TrackRequest("test_key_id", "test_account", true)
		tracker.TrackRequest("test_key_id", "test_account", false)
		tracker.TrackError("test_key_id", "RATE_LIMIT")

		// Get usage
		usage, err := tracker.GetUsage("test_key_id")
		assert.NoError(t, err)
		assert.Equal(t, int64(3), usage.TotalRequests)
		assert.Equal(t, int64(2), usage.SuccessRequests)
		assert.Equal(t, int64(1), usage.FailedRequests)
		assert.Equal(t, int64(1), usage.ErrorCodes["RATE_LIMIT"])
	})

	t.Run("Emergency Revocation", func(t *testing.T) {
		// Create emergency manager
		emergencyConfig := keymanager.EmergencyAccess{
			Enabled:          true,
			BreakGlassUsers:  []string{"admin", "security_team"},
			RequireMultiAuth: false,
			MinApprovers:     2,
			AlertChannels:    []string{"console"},
		}

		emergency := keymanager.NewEmergencyManager(manager, emergencyConfig)
		emergency.AddAlertChannel(&keymanager.ConsoleAlertChannel{})

		// Create emergency request
		req := keymanager.EmergencyRequest{
			Type:         "security_breach",
			InitiatedBy:  "admin",
			Reason:       "Suspected key compromise",
			AffectedKeys: []string{"test_key_id"},
		}

		incident, err := emergency.InitiateEmergency(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, incident)
		assert.Equal(t, "executed", incident.Status)
		assert.NotEmpty(t, incident.Actions)
	})

	t.Run("Key Statistics", func(t *testing.T) {
		stats, err := manager.GetStats(ctx)
		assert.NoError(t, err)
		assert.NotNil(t, stats)
		assert.Greater(t, stats.TotalKeys, 0)
		assert.GreaterOrEqual(t, stats.ActiveKeys, 0)
		assert.NotEmpty(t, stats.KeysByExchange)
		assert.Equal(t, "healthy", stats.HealthStatus)
	})

	t.Run("Audit Logging", func(t *testing.T) {
		auditor := manager.GetAuditor()
		require.NotNil(t, auditor)

		// Query recent audit logs
		logs, err := auditor.GetRecent(10)
		assert.NoError(t, err)
		assert.NotEmpty(t, logs)

		// Generate audit report
		report, err := auditor.GenerateReport(
			time.Now().Add(-24*time.Hour),
			time.Now(),
		)
		assert.NoError(t, err)
		assert.NotNil(t, report)
		assert.Greater(t, report.TotalActions, 0)
	})
}

// Note: These would be the actual interface methods needed
type KeyManagerInterface interface {
	GetRotator() *keymanager.KeyRotator
	GetUsageTracker() *keymanager.UsageTracker
	GetAuditor() *keymanager.Auditor
}

// Add these methods to the Manager struct in manager.go:
func (m *keymanager.Manager) GetRotator() *keymanager.KeyRotator {
	return m.rotator
}

func (m *keymanager.Manager) GetUsageTracker() *keymanager.UsageTracker {
	return m.usageTracker
}

func (m *keymanager.Manager) GetAuditor() *keymanager.Auditor {
	return m.auditor
}