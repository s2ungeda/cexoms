package keymanager

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

// KeyRotator handles automatic key rotation
type KeyRotator struct {
	mu              sync.RWMutex
	manager         *Manager
	policy          KeyRotationPolicy
	cron            *cron.Cron
	rotationHistory map[string][]RotationRecord
	notifications   chan RotationNotification
}

// RotationRecord tracks key rotation history
type RotationRecord struct {
	KeyID        string    `json:"key_id"`
	AccountName  string    `json:"account_name"`
	Exchange     string    `json:"exchange"`
	Market       string    `json:"market"`
	OldKeyID     string    `json:"old_key_id"`
	NewKeyID     string    `json:"new_key_id"`
	RotatedAt    time.Time `json:"rotated_at"`
	Reason       string    `json:"reason"`
	Success      bool      `json:"success"`
	ErrorMessage string    `json:"error_message,omitempty"`
}

// RotationNotification represents a rotation notification
type RotationNotification struct {
	Type        string    `json:"type"` // "upcoming", "started", "completed", "failed"
	KeyID       string    `json:"key_id"`
	AccountName string    `json:"account_name"`
	Exchange    string    `json:"exchange"`
	Market      string    `json:"market"`
	Message     string    `json:"message"`
	Timestamp   time.Time `json:"timestamp"`
}

// NewKeyRotator creates a new key rotator
func NewKeyRotator(manager *Manager, policy KeyRotationPolicy) *KeyRotator {
	kr := &KeyRotator{
		manager:         manager,
		policy:          policy,
		rotationHistory: make(map[string][]RotationRecord),
		notifications:   make(chan RotationNotification, 100),
	}

	if policy.Enabled {
		kr.cron = cron.New()
		kr.setupRotationSchedule()
	}

	return kr
}

// Start starts the key rotator
func (kr *KeyRotator) Start() error {
	if !kr.policy.Enabled {
		return nil
	}

	kr.cron.Start()
	
	// Start notification processor
	go kr.processNotifications()

	// Initial check for keys needing rotation
	go kr.checkKeysForRotation()

	return nil
}

// Stop stops the key rotator
func (kr *KeyRotator) Stop() {
	if kr.cron != nil {
		kr.cron.Stop()
	}
	close(kr.notifications)
}

// setupRotationSchedule sets up the cron schedule for key rotation
func (kr *KeyRotator) setupRotationSchedule() {
	// Daily check at 2 AM
	kr.cron.AddFunc("0 2 * * *", kr.checkKeysForRotation)

	// Check for upcoming rotations every 6 hours
	kr.cron.AddFunc("0 */6 * * *", kr.checkUpcomingRotations)
}

// RotateKey rotates a specific key
func (kr *KeyRotator) RotateKey(ctx context.Context, req KeyRotationRequest) (*RotationRecord, error) {
	kr.mu.Lock()
	defer kr.mu.Unlock()

	// Get current key
	currentKey, err := kr.manager.GetKeyByID(ctx, req.KeyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get current key: %w", err)
	}

	// Create rotation record
	record := RotationRecord{
		KeyID:       req.KeyID,
		AccountName: currentKey.AccountName,
		Exchange:    currentKey.Exchange,
		Market:      currentKey.Market,
		OldKeyID:    currentKey.ID,
		RotatedAt:   time.Now(),
		Reason:      req.Reason,
	}

	// Send notification
	kr.sendNotification(RotationNotification{
		Type:        "started",
		KeyID:       req.KeyID,
		AccountName: currentKey.AccountName,
		Exchange:    currentKey.Exchange,
		Market:      currentKey.Market,
		Message:     fmt.Sprintf("Key rotation started: %s", req.Reason),
		Timestamp:   time.Now(),
	})

	// Perform rotation
	newKey, err := kr.performRotation(ctx, currentKey, req)
	if err != nil {
		record.Success = false
		record.ErrorMessage = err.Error()
		kr.addRotationRecord(record)

		kr.sendNotification(RotationNotification{
			Type:        "failed",
			KeyID:       req.KeyID,
			AccountName: currentKey.AccountName,
			Exchange:    currentKey.Exchange,
			Market:      currentKey.Market,
			Message:     fmt.Sprintf("Key rotation failed: %v", err),
			Timestamp:   time.Now(),
		})

		return &record, err
	}

	// Update record
	record.NewKeyID = newKey.ID
	record.Success = true
	kr.addRotationRecord(record)

	// Send success notification
	kr.sendNotification(RotationNotification{
		Type:        "completed",
		KeyID:       newKey.ID,
		AccountName: currentKey.AccountName,
		Exchange:    currentKey.Exchange,
		Market:      currentKey.Market,
		Message:     "Key rotation completed successfully",
		Timestamp:   time.Now(),
	})

	return &record, nil
}

// performRotation performs the actual key rotation
func (kr *KeyRotator) performRotation(ctx context.Context, currentKey *APIKey, req KeyRotationRequest) (*APIKey, error) {
	// Create new key
	newKey := &APIKey{
		ID:          uuid.New().String(),
		AccountName: currentKey.AccountName,
		Exchange:    currentKey.Exchange,
		Market:      currentKey.Market,
		APIKey:      req.NewAPIKey,
		APISecret:   req.NewSecret,
		Passphrase:  currentKey.Passphrase,
		Permissions: currentKey.Permissions,
		IsActive:    true,
		IsTestnet:   currentKey.IsTestnet,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		RotatedFrom: currentKey.ID,
		Tags:        currentKey.Tags,
	}

	// If new credentials not provided, this would be where we'd
	// interface with the exchange to generate new keys
	if newKey.APIKey == "" || newKey.APISecret == "" {
		// In production, this would call exchange API to generate new keys
		return nil, fmt.Errorf("new API credentials must be provided")
	}

	// Store new key
	if err := kr.manager.StoreKey(ctx, newKey); err != nil {
		return nil, fmt.Errorf("failed to store new key: %w", err)
	}

	// Handle grace period
	if !req.Immediate && kr.policy.GracePeriod > 0 {
		// Schedule old key deactivation
		go kr.scheduleKeyDeactivation(currentKey.ID, kr.policy.GracePeriod)
	} else {
		// Immediate deactivation
		currentKey.IsActive = false
		currentKey.UpdatedAt = time.Now()
		if err := kr.manager.UpdateKey(ctx, currentKey); err != nil {
			// Log error but don't fail the rotation
			fmt.Printf("Failed to deactivate old key: %v\n", err)
		}
	}

	return newKey, nil
}

// checkKeysForRotation checks all keys for rotation needs
func (kr *KeyRotator) checkKeysForRotation() {
	ctx := context.Background()
	
	// Get all keys
	keys, err := kr.manager.ListAllKeys(ctx)
	if err != nil {
		fmt.Printf("Failed to list keys for rotation check: %v\n", err)
		return
	}

	now := time.Now()
	for _, key := range keys {
		// Skip inactive keys
		if !key.IsActive {
			continue
		}

		// Check if key needs rotation
		if kr.needsRotation(key, now) {
			if kr.policy.AutoRotate && !kr.policy.RequireApproval {
				// Auto-rotate
				req := KeyRotationRequest{
					KeyID:  key.ID,
					Reason: "Scheduled rotation",
				}
				kr.RotateKey(ctx, req)
			} else {
				// Send notification for manual rotation
				kr.sendNotification(RotationNotification{
					Type:        "required",
					KeyID:       key.ID,
					AccountName: key.AccountName,
					Exchange:    key.Exchange,
					Market:      key.Market,
					Message:     "Key rotation required",
					Timestamp:   now,
				})
			}
		}
	}
}

// checkUpcomingRotations checks for keys that will need rotation soon
func (kr *KeyRotator) checkUpcomingRotations() {
	ctx := context.Background()
	
	keys, err := kr.manager.ListAllKeys(ctx)
	if err != nil {
		return
	}

	now := time.Now()
	notifyBefore := time.Duration(kr.policy.NotifyBeforeDays) * 24 * time.Hour

	for _, key := range keys {
		if !key.IsActive {
			continue
		}

		nextRotation := key.CreatedAt.Add(kr.policy.RotationInterval)
		if now.Add(notifyBefore).After(nextRotation) && now.Before(nextRotation) {
			kr.sendNotification(RotationNotification{
				Type:        "upcoming",
				KeyID:       key.ID,
				AccountName: key.AccountName,
				Exchange:    key.Exchange,
				Market:      key.Market,
				Message:     fmt.Sprintf("Key rotation due in %d days", int(nextRotation.Sub(now).Hours()/24)),
				Timestamp:   now,
			})
		}
	}
}

// needsRotation checks if a key needs rotation
func (kr *KeyRotator) needsRotation(key *KeyMetadata, now time.Time) bool {
	// Check expiration
	if key.ExpiresAt != nil && now.After(*key.ExpiresAt) {
		return true
	}

	// Check rotation interval
	if kr.policy.RotationInterval > 0 {
		rotationDue := key.CreatedAt.Add(kr.policy.RotationInterval)
		if now.After(rotationDue) {
			return true
		}
	}

	return false
}

// scheduleKeyDeactivation schedules a key for deactivation after grace period
func (kr *KeyRotator) scheduleKeyDeactivation(keyID string, gracePeriod time.Duration) {
	time.Sleep(gracePeriod)

	ctx := context.Background()
	key, err := kr.manager.GetKeyByID(ctx, keyID)
	if err != nil {
		return
	}

	key.IsActive = false
	key.UpdatedAt = time.Now()
	kr.manager.UpdateKey(ctx, key)
}

// GetRotationHistory returns rotation history for an account
func (kr *KeyRotator) GetRotationHistory(accountName string) []RotationRecord {
	kr.mu.RLock()
	defer kr.mu.RUnlock()

	history := []RotationRecord{}
	if records, exists := kr.rotationHistory[accountName]; exists {
		history = append(history, records...)
	}

	return history
}

// GetNextRotations returns upcoming rotations
func (kr *KeyRotator) GetNextRotations(days int) []RotationSchedule {
	ctx := context.Background()
	keys, err := kr.manager.ListAllKeys(ctx)
	if err != nil {
		return nil
	}

	var schedules []RotationSchedule
	now := time.Now()
	cutoff := now.Add(time.Duration(days) * 24 * time.Hour)

	for _, key := range keys {
		if !key.IsActive {
			continue
		}

		nextRotation := key.CreatedAt.Add(kr.policy.RotationInterval)
		if nextRotation.After(now) && nextRotation.Before(cutoff) {
			schedules = append(schedules, RotationSchedule{
				KeyID:            key.ID,
				AccountName:      key.AccountName,
				Exchange:         key.Exchange,
				Market:           key.Market,
				ScheduledAt:      nextRotation,
				DaysUntilRotation: int(nextRotation.Sub(now).Hours() / 24),
			})
		}
	}

	return schedules
}

// RotationSchedule represents a scheduled rotation
type RotationSchedule struct {
	KeyID             string    `json:"key_id"`
	AccountName       string    `json:"account_name"`
	Exchange          string    `json:"exchange"`
	Market            string    `json:"market"`
	ScheduledAt       time.Time `json:"scheduled_at"`
	DaysUntilRotation int       `json:"days_until_rotation"`
}

// addRotationRecord adds a rotation record to history
func (kr *KeyRotator) addRotationRecord(record RotationRecord) {
	kr.mu.Lock()
	defer kr.mu.Unlock()

	if _, exists := kr.rotationHistory[record.AccountName]; !exists {
		kr.rotationHistory[record.AccountName] = []RotationRecord{}
	}

	kr.rotationHistory[record.AccountName] = append(kr.rotationHistory[record.AccountName], record)

	// Keep only last 100 records per account
	if len(kr.rotationHistory[record.AccountName]) > 100 {
		kr.rotationHistory[record.AccountName] = kr.rotationHistory[record.AccountName][1:]
	}
}

// sendNotification sends a rotation notification
func (kr *KeyRotator) sendNotification(notification RotationNotification) {
	select {
	case kr.notifications <- notification:
	default:
		// Channel full, drop notification
	}
}

// processNotifications processes rotation notifications
func (kr *KeyRotator) processNotifications() {
	for notification := range kr.notifications {
		// In production, this would send to various channels
		// (email, Slack, PagerDuty, etc.)
		fmt.Printf("[ROTATION] %s: %s - %s\n", 
			notification.Type, 
			notification.AccountName,
			notification.Message)
	}
}

// GetNotificationChannel returns the notification channel for external consumers
func (kr *KeyRotator) GetNotificationChannel() <-chan RotationNotification {
	return kr.notifications
}