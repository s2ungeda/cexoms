package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

// MFAType represents the type of MFA
type MFAType string

const (
	MFATypeTOTP MFAType = "totp"
	MFATypeSMS  MFAType = "sms"
	MFATypeU2F  MFAType = "u2f"
	MFATypeBackupCode MFAType = "backup_code"
)

// MFAProvider manages multi-factor authentication
type MFAProvider struct {
	mu       sync.RWMutex
	users    map[string]*UserMFA
	smsProvider SMSProvider
	issuer   string
	window   int // Time window for TOTP validation
}

// UserMFA represents a user's MFA configuration
type UserMFA struct {
	UserID         string
	Secret         string
	BackupCodes    []string
	UsedBackupCodes map[string]bool
	EnabledMethods []MFAType
	PhoneNumber    string
	U2FDevices     []*U2FDevice
	CreatedAt      time.Time
	LastUsedAt     time.Time
	FailedAttempts int
	LockedUntil    time.Time
}

// U2FDevice represents a U2F/WebAuthn device
type U2FDevice struct {
	ID          string
	Name        string
	PublicKey   string
	Counter     uint32
	RegisteredAt time.Time
	LastUsedAt  time.Time
}

// SMSProvider interface for sending SMS
type SMSProvider interface {
	SendSMS(phoneNumber, code string) error
}

// NewMFAProvider creates a new MFA provider
func NewMFAProvider() *MFAProvider {
	return &MFAProvider{
		users:  make(map[string]*UserMFA),
		issuer: "mExOms",
		window: 2, // Allow 2 time steps before/after
	}
}

// SetupTOTP generates TOTP secret for user
func (mp *MFAProvider) SetupTOTP(userID, email string) (string, string, error) {
	// Generate random secret
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return "", "", fmt.Errorf("failed to generate secret: %w", err)
	}
	
	// Encode secret to base32
	encodedSecret := base32.StdEncoding.EncodeToString(secret)
	
	// Generate backup codes
	backupCodes := mp.generateBackupCodes(8)
	
	// Store user MFA configuration
	mp.mu.Lock()
	mp.users[userID] = &UserMFA{
		UserID:          userID,
		Secret:          encodedSecret,
		BackupCodes:     backupCodes,
		UsedBackupCodes: make(map[string]bool),
		EnabledMethods:  []MFAType{MFATypeTOTP},
		CreatedAt:       time.Now(),
		FailedAttempts:  0,
	}
	mp.mu.Unlock()
	
	// Generate provisioning URI for QR code
	uri := mp.generateProvisioningURI(userID, email, encodedSecret)
	
	return encodedSecret, uri, nil
}

// VerifyToken verifies MFA token
func (mp *MFAProvider) VerifyToken(userID, token string) bool {
	mp.mu.RLock()
	userMFA, exists := mp.users[userID]
	mp.mu.RUnlock()
	
	if !exists {
		return false
	}
	
	// Check if account is locked
	if time.Now().Before(userMFA.LockedUntil) {
		return false
	}
	
	// Try TOTP verification first
	if mp.verifyTOTP(userMFA.Secret, token) {
		mp.updateLastUsed(userID)
		mp.resetFailedAttempts(userID)
		return true
	}
	
	// Try backup code
	if mp.verifyBackupCode(userID, token) {
		mp.updateLastUsed(userID)
		mp.resetFailedAttempts(userID)
		return true
	}
	
	// Increment failed attempts
	mp.incrementFailedAttempts(userID)
	
	return false
}

// verifyTOTP verifies TOTP code
func (mp *MFAProvider) verifyTOTP(secret, token string) bool {
	// Decode secret
	key, err := base32.StdEncoding.DecodeString(secret)
	if err != nil {
		return false
	}
	
	// Get current time step
	now := time.Now().Unix()
	timeStep := now / 30
	
	// Check time window
	for i := -mp.window; i <= mp.window; i++ {
		counter := uint64(timeStep + int64(i))
		expectedToken := mp.generateTOTP(key, counter)
		
		if token == expectedToken {
			return true
		}
	}
	
	return false
}

// generateTOTP generates TOTP code
func (mp *MFAProvider) generateTOTP(key []byte, counter uint64) string {
	// Convert counter to bytes
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)
	
	// Generate HMAC
	h := hmac.New(sha1.New, key)
	h.Write(buf)
	sum := h.Sum(nil)
	
	// Dynamic truncation
	offset := sum[len(sum)-1] & 0xf
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	
	// Generate 6-digit code
	return fmt.Sprintf("%06d", code%1000000)
}

// verifyBackupCode verifies backup code
func (mp *MFAProvider) verifyBackupCode(userID, code string) bool {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	
	userMFA, exists := mp.users[userID]
	if !exists {
		return false
	}
	
	// Check if code has been used
	if userMFA.UsedBackupCodes[code] {
		return false
	}
	
	// Check if code is valid
	for _, backupCode := range userMFA.BackupCodes {
		if backupCode == code {
			userMFA.UsedBackupCodes[code] = true
			return true
		}
	}
	
	return false
}

// SendSMSCode sends SMS verification code
func (mp *MFAProvider) SendSMSCode(userID string) error {
	mp.mu.RLock()
	userMFA, exists := mp.users[userID]
	mp.mu.RUnlock()
	
	if !exists || userMFA.PhoneNumber == "" {
		return fmt.Errorf("phone number not configured")
	}
	
	// Generate 6-digit code
	code := mp.generateSMSCode()
	
	// Store code temporarily (in production, use cache with TTL)
	// For now, we'll just send it
	
	// Send SMS
	if mp.smsProvider != nil {
		return mp.smsProvider.SendSMS(userMFA.PhoneNumber, code)
	}
	
	return fmt.Errorf("SMS provider not configured")
}

// generateProvisioningURI generates URI for QR code
func (mp *MFAProvider) generateProvisioningURI(userID, email, secret string) string {
	params := url.Values{}
	params.Set("secret", secret)
	params.Set("issuer", mp.issuer)
	params.Set("algorithm", "SHA1")
	params.Set("digits", "6")
	params.Set("period", "30")
	
	u := url.URL{
		Scheme: "otpauth",
		Host:   "totp",
		Path:   fmt.Sprintf("%s:%s", mp.issuer, email),
		RawQuery: params.Encode(),
	}
	
	return u.String()
}

// generateBackupCodes generates backup codes
func (mp *MFAProvider) generateBackupCodes(count int) []string {
	codes := make([]string, count)
	
	for i := 0; i < count; i++ {
		b := make([]byte, 4)
		rand.Read(b)
		code := fmt.Sprintf("%08X", binary.BigEndian.Uint32(b))
		codes[i] = fmt.Sprintf("%s-%s", code[:4], code[4:])
	}
	
	return codes
}

// generateSMSCode generates 6-digit SMS code
func (mp *MFAProvider) generateSMSCode() string {
	b := make([]byte, 3)
	rand.Read(b)
	code := binary.BigEndian.Uint32(append([]byte{0}, b...))
	return fmt.Sprintf("%06d", code%1000000)
}

// updateLastUsed updates last used timestamp
func (mp *MFAProvider) updateLastUsed(userID string) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	
	if userMFA, exists := mp.users[userID]; exists {
		userMFA.LastUsedAt = time.Now()
	}
}

// resetFailedAttempts resets failed attempts counter
func (mp *MFAProvider) resetFailedAttempts(userID string) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	
	if userMFA, exists := mp.users[userID]; exists {
		userMFA.FailedAttempts = 0
		userMFA.LockedUntil = time.Time{}
	}
}

// incrementFailedAttempts increments failed attempts and locks if necessary
func (mp *MFAProvider) incrementFailedAttempts(userID string) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	
	if userMFA, exists := mp.users[userID]; exists {
		userMFA.FailedAttempts++
		
		// Lock account after 5 failed attempts
		if userMFA.FailedAttempts >= 5 {
			userMFA.LockedUntil = time.Now().Add(30 * time.Minute)
		}
	}
}

// DisableMFA disables MFA for user
func (mp *MFAProvider) DisableMFA(userID string) error {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	
	delete(mp.users, userID)
	return nil
}

// GetUserMFAStatus returns user's MFA status
func (mp *MFAProvider) GetUserMFAStatus(userID string) (bool, []MFAType) {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	
	userMFA, exists := mp.users[userID]
	if !exists {
		return false, nil
	}
	
	return true, userMFA.EnabledMethods
}

// RegenerateBackupCodes generates new backup codes
func (mp *MFAProvider) RegenerateBackupCodes(userID string) ([]string, error) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	
	userMFA, exists := mp.users[userID]
	if !exists {
		return nil, fmt.Errorf("MFA not configured for user")
	}
	
	// Generate new backup codes
	backupCodes := mp.generateBackupCodes(8)
	userMFA.BackupCodes = backupCodes
	userMFA.UsedBackupCodes = make(map[string]bool)
	
	return backupCodes, nil
}

// AddPhoneNumber adds phone number for SMS MFA
func (mp *MFAProvider) AddPhoneNumber(userID, phoneNumber string) error {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	
	userMFA, exists := mp.users[userID]
	if !exists {
		return fmt.Errorf("MFA not configured for user")
	}
	
	// Validate phone number format
	if !strings.HasPrefix(phoneNumber, "+") {
		return fmt.Errorf("phone number must include country code")
	}
	
	userMFA.PhoneNumber = phoneNumber
	
	// Add SMS to enabled methods
	hasSMS := false
	for _, method := range userMFA.EnabledMethods {
		if method == MFATypeSMS {
			hasSMS = true
			break
		}
	}
	if !hasSMS {
		userMFA.EnabledMethods = append(userMFA.EnabledMethods, MFATypeSMS)
	}
	
	return nil
}