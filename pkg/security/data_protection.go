package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// DataProtectionManager manages data protection and integrity
type DataProtectionManager struct {
	mu               sync.RWMutex
	encryptionMgr    *EncryptionManager
	dataClassifiers  map[string]DataClassification
	accessPolicies   map[string]*AccessPolicy
	integrityChecks  map[string]*IntegrityCheck
}

// DataClassification represents data sensitivity level
type DataClassification string

const (
	ClassificationPublic       DataClassification = "public"
	ClassificationInternal     DataClassification = "internal"
	ClassificationConfidential DataClassification = "confidential"
	ClassificationSecret       DataClassification = "secret"
	ClassificationTopSecret    DataClassification = "top_secret"
)

// AccessPolicy defines access rules for data
type AccessPolicy struct {
	DataType       string
	Classification DataClassification
	AllowedRoles   []string
	AllowedUsers   []string
	RequireMFA     bool
	RequireAudit   bool
	RetentionDays  int
	Restrictions   map[string]interface{}
}

// IntegrityCheck tracks data integrity
type IntegrityCheck struct {
	DataID       string
	Hash         string
	Algorithm    string
	CheckedAt    time.Time
	LastModified time.Time
	Version      int
}

// SensitiveField represents a field that needs protection
type SensitiveField struct {
	Name           string
	Classification DataClassification
	Masked         bool
	Encrypted      bool
}

// NewDataProtectionManager creates a new data protection manager
func NewDataProtectionManager(encryptionMgr *EncryptionManager) *DataProtectionManager {
	dpm := &DataProtectionManager{
		encryptionMgr:   encryptionMgr,
		dataClassifiers: make(map[string]DataClassification),
		accessPolicies:  make(map[string]*AccessPolicy),
		integrityChecks: make(map[string]*IntegrityCheck),
	}
	
	// Initialize default classifications
	dpm.initializeDefaultClassifications()
	
	return dpm
}

// initializeDefaultClassifications sets up default data classifications
func (dpm *DataProtectionManager) initializeDefaultClassifications() {
	// API Keys and Secrets
	dpm.dataClassifiers["api_key"] = ClassificationTopSecret
	dpm.dataClassifiers["secret_key"] = ClassificationTopSecret
	dpm.dataClassifiers["private_key"] = ClassificationTopSecret
	dpm.dataClassifiers["password"] = ClassificationSecret
	dpm.dataClassifiers["passphrase"] = ClassificationSecret
	
	// Personal Information
	dpm.dataClassifiers["email"] = ClassificationConfidential
	dpm.dataClassifiers["phone"] = ClassificationConfidential
	dpm.dataClassifiers["address"] = ClassificationConfidential
	dpm.dataClassifiers["ssn"] = ClassificationSecret
	dpm.dataClassifiers["passport"] = ClassificationSecret
	
	// Financial Data
	dpm.dataClassifiers["bank_account"] = ClassificationSecret
	dpm.dataClassifiers["credit_card"] = ClassificationSecret
	dpm.dataClassifiers["balance"] = ClassificationConfidential
	dpm.dataClassifiers["transaction"] = ClassificationConfidential
	
	// Trading Data
	dpm.dataClassifiers["order"] = ClassificationInternal
	dpm.dataClassifiers["position"] = ClassificationInternal
	dpm.dataClassifiers["strategy"] = ClassificationConfidential
	dpm.dataClassifiers["pnl"] = ClassificationConfidential
}

// ClassifyData classifies data based on content
func (dpm *DataProtectionManager) ClassifyData(dataType string, content interface{}) DataClassification {
	dpm.mu.RLock()
	defer dpm.mu.RUnlock()
	
	// Check if we have a predefined classification
	if classification, exists := dpm.dataClassifiers[strings.ToLower(dataType)]; exists {
		return classification
	}
	
	// Check content for sensitive patterns
	contentStr := fmt.Sprintf("%v", content)
	
	// Check for high-sensitivity patterns
	if dpm.containsSensitivePattern(contentStr, []string{"private_key", "secret_key", "api_key", "BEGIN RSA PRIVATE KEY"}) {
		return ClassificationTopSecret
	}
	
	// Check for medium-sensitivity patterns
	if dpm.containsSensitivePattern(contentStr, []string{"password", "token", "session", "auth"}) {
		return ClassificationSecret
	}
	
	// Check for personal information patterns
	if dpm.containsSensitivePattern(contentStr, []string{"@", "email", "phone", "address"}) {
		return ClassificationConfidential
	}
	
	// Default classification
	return ClassificationInternal
}

// ProtectData applies protection to sensitive data
func (dpm *DataProtectionManager) ProtectData(data map[string]interface{}, policy *AccessPolicy) (map[string]interface{}, error) {
	protectedData := make(map[string]interface{})
	
	for key, value := range data {
		classification := dpm.ClassifyData(key, value)
		
		// Apply protection based on classification
		switch classification {
		case ClassificationTopSecret, ClassificationSecret:
			// Encrypt highly sensitive data
			if strVal, ok := value.(string); ok {
				encrypted, err := dpm.encryptionMgr.Encrypt([]byte(strVal), "default")
				if err != nil {
					return nil, fmt.Errorf("failed to encrypt %s: %w", key, err)
				}
				protectedData[key] = encrypted
			} else {
				protectedData[key] = value
			}
			
		case ClassificationConfidential:
			// Mask or partially encrypt
			if strVal, ok := value.(string); ok {
				protectedData[key] = dpm.maskSensitiveData(strVal, key)
			} else {
				protectedData[key] = value
			}
			
		default:
			protectedData[key] = value
		}
	}
	
	return protectedData, nil
}

// maskSensitiveData masks sensitive information
func (dpm *DataProtectionManager) maskSensitiveData(data string, dataType string) string {
	switch strings.ToLower(dataType) {
	case "email":
		parts := strings.Split(data, "@")
		if len(parts) == 2 {
			masked := maskString(parts[0], 2, 1)
			return masked + "@" + parts[1]
		}
		
	case "phone":
		if len(data) > 6 {
			return data[:3] + strings.Repeat("*", len(data)-6) + data[len(data)-3:]
		}
		
	case "credit_card", "bank_account":
		if len(data) > 4 {
			return strings.Repeat("*", len(data)-4) + data[len(data)-4:]
		}
		
	case "api_key", "secret_key":
		if len(data) > 8 {
			return data[:4] + strings.Repeat("*", len(data)-8) + data[len(data)-4:]
		}
	}
	
	// Default masking
	if len(data) > 4 {
		return maskString(data, 2, 2)
	}
	
	return strings.Repeat("*", len(data))
}

// CheckAccess verifies if user has access to data
func (dpm *DataProtectionManager) CheckAccess(userID string, roles []string, dataType string, classification DataClassification) (bool, error) {
	dpm.mu.RLock()
	defer dpm.mu.RUnlock()
	
	policy, exists := dpm.accessPolicies[dataType]
	if !exists {
		// Default policy based on classification
		policy = dpm.getDefaultPolicy(classification)
	}
	
	// Check user access
	for _, allowedUser := range policy.AllowedUsers {
		if allowedUser == userID {
			return true, nil
		}
	}
	
	// Check role access
	for _, userRole := range roles {
		for _, allowedRole := range policy.AllowedRoles {
			if userRole == allowedRole {
				return true, nil
			}
		}
	}
	
	return false, fmt.Errorf("access denied for user %s to %s data", userID, dataType)
}

// CreateIntegrityHash creates integrity hash for data
func (dpm *DataProtectionManager) CreateIntegrityHash(dataID string, data []byte) (*IntegrityCheck, error) {
	// Create SHA-256 hash
	hash := sha256.Sum256(data)
	
	integrityCheck := &IntegrityCheck{
		DataID:       dataID,
		Hash:         hex.EncodeToString(hash[:]),
		Algorithm:    "SHA256",
		CheckedAt:    time.Now(),
		LastModified: time.Now(),
		Version:      1,
	}
	
	dpm.mu.Lock()
	dpm.integrityChecks[dataID] = integrityCheck
	dpm.mu.Unlock()
	
	return integrityCheck, nil
}

// VerifyIntegrity verifies data integrity
func (dpm *DataProtectionManager) VerifyIntegrity(dataID string, data []byte) (bool, error) {
	dpm.mu.RLock()
	integrityCheck, exists := dpm.integrityChecks[dataID]
	dpm.mu.RUnlock()
	
	if !exists {
		return false, fmt.Errorf("no integrity check found for data ID: %s", dataID)
	}
	
	// Calculate current hash
	hash := sha256.Sum256(data)
	currentHash := hex.EncodeToString(hash[:])
	
	// Compare hashes
	if currentHash != integrityCheck.Hash {
		return false, fmt.Errorf("integrity check failed: hash mismatch")
	}
	
	// Update check time
	dpm.mu.Lock()
	integrityCheck.CheckedAt = time.Now()
	dpm.mu.Unlock()
	
	return true, nil
}

// CreateHMAC creates HMAC for data
func (dpm *DataProtectionManager) CreateHMAC(data []byte, key []byte) string {
	h := hmac.New(sha512.New, key)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyHMAC verifies HMAC
func (dpm *DataProtectionManager) VerifyHMAC(data []byte, key []byte, providedMAC string) bool {
	expectedMAC := dpm.CreateHMAC(data, key)
	return hmac.Equal([]byte(expectedMAC), []byte(providedMAC))
}

// SetAccessPolicy sets access policy for data type
func (dpm *DataProtectionManager) SetAccessPolicy(dataType string, policy *AccessPolicy) {
	dpm.mu.Lock()
	defer dpm.mu.Unlock()
	
	dpm.accessPolicies[dataType] = policy
}

// GetAccessPolicy retrieves access policy
func (dpm *DataProtectionManager) GetAccessPolicy(dataType string) (*AccessPolicy, error) {
	dpm.mu.RLock()
	defer dpm.mu.RUnlock()
	
	policy, exists := dpm.accessPolicies[dataType]
	if !exists {
		return nil, fmt.Errorf("no access policy found for data type: %s", dataType)
	}
	
	return policy, nil
}

// containsSensitivePattern checks if string contains sensitive patterns
func (dpm *DataProtectionManager) containsSensitivePattern(content string, patterns []string) bool {
	contentLower := strings.ToLower(content)
	for _, pattern := range patterns {
		if strings.Contains(contentLower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// getDefaultPolicy returns default policy based on classification
func (dpm *DataProtectionManager) getDefaultPolicy(classification DataClassification) *AccessPolicy {
	switch classification {
	case ClassificationTopSecret:
		return &AccessPolicy{
			Classification: classification,
			AllowedRoles:   []string{"admin", "security_admin"},
			RequireMFA:     true,
			RequireAudit:   true,
			RetentionDays:  90,
		}
		
	case ClassificationSecret:
		return &AccessPolicy{
			Classification: classification,
			AllowedRoles:   []string{"admin", "security_admin", "trader"},
			RequireMFA:     true,
			RequireAudit:   true,
			RetentionDays:  180,
		}
		
	case ClassificationConfidential:
		return &AccessPolicy{
			Classification: classification,
			AllowedRoles:   []string{"admin", "trader", "analyst"},
			RequireMFA:     false,
			RequireAudit:   true,
			RetentionDays:  365,
		}
		
	default:
		return &AccessPolicy{
			Classification: classification,
			AllowedRoles:   []string{"admin", "trader", "analyst", "viewer"},
			RequireMFA:     false,
			RequireAudit:   false,
			RetentionDays:  730,
		}
	}
}

// DataRetention manages data retention policies
func (dpm *DataProtectionManager) DataRetention(dataType string, createdAt time.Time) bool {
	policy, err := dpm.GetAccessPolicy(dataType)
	if err != nil {
		// Use default retention
		return time.Since(createdAt).Hours()/24 <= 365
	}
	
	return time.Since(createdAt).Hours()/24 <= float64(policy.RetentionDays)
}

// SanitizeOutput removes sensitive data from output
func (dpm *DataProtectionManager) SanitizeOutput(data map[string]interface{}, userRoles []string) map[string]interface{} {
	sanitized := make(map[string]interface{})
	
	for key, value := range data {
		classification := dpm.ClassifyData(key, value)
		
		// Check if user has access to this classification
		hasAccess := false
		for _, role := range userRoles {
			if dpm.roleHasAccess(role, classification) {
				hasAccess = true
				break
			}
		}
		
		if hasAccess {
			// Still mask certain fields
			if classification >= ClassificationConfidential {
				if strVal, ok := value.(string); ok {
					sanitized[key] = dpm.maskSensitiveData(strVal, key)
				} else {
					sanitized[key] = value
				}
			} else {
				sanitized[key] = value
			}
		} else {
			// Completely redact
			sanitized[key] = "[REDACTED]"
		}
	}
	
	return sanitized
}

// roleHasAccess checks if role has access to classification
func (dpm *DataProtectionManager) roleHasAccess(role string, classification DataClassification) bool {
	roleAccessMap := map[string][]DataClassification{
		"admin":          {ClassificationPublic, ClassificationInternal, ClassificationConfidential, ClassificationSecret, ClassificationTopSecret},
		"security_admin": {ClassificationPublic, ClassificationInternal, ClassificationConfidential, ClassificationSecret, ClassificationTopSecret},
		"trader":         {ClassificationPublic, ClassificationInternal, ClassificationConfidential},
		"analyst":        {ClassificationPublic, ClassificationInternal, ClassificationConfidential},
		"viewer":         {ClassificationPublic, ClassificationInternal},
	}
	
	allowedClassifications, exists := roleAccessMap[role]
	if !exists {
		return false
	}
	
	for _, allowed := range allowedClassifications {
		if allowed == classification {
			return true
		}
	}
	
	return false
}

// maskString masks a string keeping first and last characters
func maskString(s string, keepStart, keepEnd int) string {
	if len(s) <= keepStart+keepEnd {
		return strings.Repeat("*", len(s))
	}
	
	masked := s[:keepStart]
	masked += strings.Repeat("*", len(s)-keepStart-keepEnd)
	masked += s[len(s)-keepEnd:]
	
	return masked
}