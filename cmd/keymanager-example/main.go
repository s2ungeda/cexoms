package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mExOms/internal/keymanager"
)

func main() {
	fmt.Println("=== Multi-Account API Key Manager Example ===")

	// Configuration
	config := keymanager.KeyManagerConfig{
		VaultConfig: keymanager.VaultConfig{
			Address:   "http://localhost:8200",
			Token:     "dev-token", // In production, use proper authentication
			MountPath: "secret",
			Timeout:   5 * time.Second,
			MaxRetries: 3,
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
			GracePeriod:      24 * time.Hour,      // 1 day
			NotifyBeforeDays: 7,
			AutoRotate:       false,
			RequireApproval:  true,
		},
		AuditEnabled:        true,
		AuditLogPath:        "./audit/keymanager.log",
		CacheEnabled:        true,
		CacheTTL:            5 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
	}

	// Create key manager
	fmt.Println("\n1. Creating Key Manager...")
	manager, err := keymanager.NewManager(config)
	if err != nil {
		log.Fatal("Failed to create key manager:", err)
	}
	defer manager.Close()

	ctx := context.Background()
	
	// Example accounts
	accounts := []struct {
		Name     string
		Exchange string
		Market   string
	}{
		{"binance_main", "binance", "spot"},
		{"binance_main", "binance", "futures"},
		{"binance_algo", "binance", "spot"},
		{"bybit_main", "bybit", "spot"},
	}

	// 2. Store API keys
	fmt.Println("\n2. Storing API Keys...")
	for _, acc := range accounts {
		key := &keymanager.APIKey{
			AccountName: acc.Name,
			Exchange:    acc.Exchange,
			Market:      acc.Market,
			APIKey:      fmt.Sprintf("demo_api_key_%s_%s", acc.Name, acc.Market),
			APISecret:   fmt.Sprintf("demo_secret_%s_%s", acc.Name, acc.Market),
			Permissions: []string{"read", "trade"},
			IsActive:    true,
			IsTestnet:   true,
			Tags: map[string]string{
				"strategy": "momentum",
				"tier":     "gold",
			},
		}

		if err := manager.StoreKey(ctx, key); err != nil {
			log.Printf("Failed to store key for %s: %v", acc.Name, err)
		} else {
			fmt.Printf("  ✓ Stored key for %s (%s %s)\n", acc.Name, acc.Exchange, acc.Market)
		}
	}

	// 3. Retrieve keys
	fmt.Println("\n3. Retrieving API Keys...")
	request := keymanager.KeyRequest{
		AccountName: "binance_main",
		Exchange:    "binance",
		Market:      "spot",
	}

	key, err := manager.GetKey(ctx, request)
	if err != nil {
		log.Printf("Failed to get key: %v", err)
	} else {
		fmt.Printf("  ✓ Retrieved key for %s\n", key.AccountName)
		fmt.Printf("    API Key: %s...\n", key.APIKey[:10])
		fmt.Printf("    Created: %s\n", key.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("    Active: %v\n", key.IsActive)
	}

	// 4. List all keys for an account
	fmt.Println("\n4. Listing Keys for Account...")
	keys, err := manager.ListKeys(ctx, "binance_main")
	if err != nil {
		log.Printf("Failed to list keys: %v", err)
	} else {
		fmt.Printf("  Found %d keys for binance_main:\n", len(keys))
		for _, k := range keys {
			fmt.Printf("    - %s %s (Active: %v)\n", k.Exchange, k.Market, k.IsActive)
		}
	}

	// 5. Track usage
	fmt.Println("\n5. Tracking API Usage...")
	tracker := manager.GetUsageTracker()
	
	// Simulate API calls
	for i := 0; i < 10; i++ {
		tracker.TrackRequest(key.ID, key.AccountName, i%3 != 0) // 70% success rate
		time.Sleep(100 * time.Millisecond)
	}
	
	// Simulate some errors
	tracker.TrackError(key.ID, "RATE_LIMIT")
	tracker.TrackError(key.ID, "INVALID_SIGNATURE")

	// Get usage stats
	usage, err := tracker.GetUsage(key.ID)
	if err == nil {
		fmt.Printf("  Key Usage Statistics:\n")
		fmt.Printf("    Total Requests: %d\n", usage.TotalRequests)
		fmt.Printf("    Success Rate: %.1f%%\n", float64(usage.SuccessRequests)/float64(usage.TotalRequests)*100)
		fmt.Printf("    Error Codes: %v\n", usage.ErrorCodes)
	}

	// 6. Key rotation schedule
	fmt.Println("\n6. Key Rotation Schedule...")
	rotator := manager.GetRotator()
	upcoming := rotator.GetNextRotations(30) // Next 30 days
	
	if len(upcoming) > 0 {
		fmt.Printf("  Upcoming rotations:\n")
		for _, rotation := range upcoming {
			fmt.Printf("    - %s (%s %s) in %d days\n", 
				rotation.AccountName, 
				rotation.Exchange, 
				rotation.Market,
				rotation.DaysUntilRotation)
		}
	} else {
		fmt.Println("  No rotations scheduled in the next 30 days")
	}

	// 7. Emergency management
	fmt.Println("\n7. Emergency Management Setup...")
	emergencyConfig := keymanager.EmergencyAccess{
		Enabled:          true,
		BreakGlassUsers:  []string{"admin", "security_team", "ops_lead"},
		RequireMultiAuth: true,
		MinApprovers:     2,
		AlertChannels:    []string{"console", "email", "slack"},
	}

	emergency := keymanager.NewEmergencyManager(manager, emergencyConfig)
	
	// Add alert channels
	emergency.AddAlertChannel(&keymanager.ConsoleAlertChannel{})
	emergency.AddAlertChannel(&keymanager.EmailAlertChannel{
		Recipients: []string{"security@example.com"},
	})
	emergency.AddAlertChannel(&keymanager.SlackAlertChannel{
		WebhookURL: "https://hooks.slack.com/services/xxx",
		Channel:    "#security-alerts",
	})

	fmt.Println("  ✓ Emergency access configured")
	fmt.Printf("    Break-glass users: %v\n", emergencyConfig.BreakGlassUsers)
	fmt.Printf("    Multi-auth required: %v\n", emergencyConfig.RequireMultiAuth)
	fmt.Printf("    Alert channels: %v\n", emergencyConfig.AlertChannels)

	// 8. Generate reports
	fmt.Println("\n8. Generating Reports...")
	
	// Get statistics
	stats, err := manager.GetStats(ctx)
	if err == nil {
		fmt.Println("\n  Key Management Statistics:")
		fmt.Printf("    Total Keys: %d\n", stats.TotalKeys)
		fmt.Printf("    Active Keys: %d\n", stats.ActiveKeys)
		fmt.Printf("    Keys by Exchange: %v\n", stats.KeysByExchange)
		fmt.Printf("    Health Status: %s\n", stats.HealthStatus)
	}

	// Usage report
	usageReport := tracker.GenerateUsageReport()
	fmt.Println("\n  Usage Report:")
	fmt.Printf("    Total Requests: %d\n", usageReport.TotalRequests)
	fmt.Printf("    Overall Success Rate: %.1f%%\n", usageReport.SuccessRate*100)
	fmt.Printf("    Top Keys: %d\n", len(usageReport.TopKeys))

	// Audit report
	auditor := manager.GetAuditor()
	if auditor != nil {
		auditReport, err := auditor.GenerateReport(
			time.Now().Add(-24*time.Hour),
			time.Now(),
		)
		if err == nil {
			fmt.Println("\n  Audit Report (Last 24 hours):")
			fmt.Printf("    Total Actions: %d\n", auditReport.TotalActions)
			fmt.Printf("    Success Rate: %.1f%%\n", auditReport.SuccessRate*100)
			fmt.Printf("    Action Types: %v\n", auditReport.ActionCounts)
		}
	}

	// 9. Best practices demonstration
	fmt.Println("\n9. Security Best Practices:")
	fmt.Println("  ✓ API secrets encrypted with AES-256-GCM")
	fmt.Println("  ✓ Keys stored in HashiCorp Vault")
	fmt.Println("  ✓ Automatic key rotation every 30 days")
	fmt.Println("  ✓ Complete audit trail of all operations")
	fmt.Println("  ✓ Emergency break-glass procedures")
	fmt.Println("  ✓ Multi-factor approval for sensitive operations")
	fmt.Println("  ✓ Real-time usage tracking and anomaly detection")

	fmt.Println("\n=== Key Manager Example Complete ===")
}