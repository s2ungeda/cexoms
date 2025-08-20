package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

// APIKey represents an encrypted API key pair
type APIKey struct {
	Exchange    string    `json:"exchange"`
	Market      string    `json:"market"`
	APIKey      string    `json:"api_key"`
	SecretKey   string    `json:"secret_key"`
	Passphrase  string    `json:"passphrase,omitempty"` // For OKX
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	RotationDue time.Time `json:"rotation_due"`
}

// KeyStore manages encrypted API keys
type KeyStore struct {
	mu       sync.RWMutex
	filePath string
	password []byte
	salt     []byte
}

// NewKeyStore creates a new key store
func NewKeyStore(storagePath string) (*KeyStore, error) {
	// Create directory if it doesn't exist
	dir := filepath.Dir(storagePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create keystore directory: %w", err)
	}

	ks := &KeyStore{
		filePath: storagePath,
	}

	// Generate or load salt
	saltFile := storagePath + ".salt"
	if _, err := os.Stat(saltFile); os.IsNotExist(err) {
		// Generate new salt
		ks.salt = make([]byte, 32)
		if _, err := rand.Read(ks.salt); err != nil {
			return nil, fmt.Errorf("failed to generate salt: %w", err)
		}
		if err := os.WriteFile(saltFile, ks.salt, 0600); err != nil {
			return nil, fmt.Errorf("failed to save salt: %w", err)
		}
	} else {
		// Load existing salt
		salt, err := os.ReadFile(saltFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load salt: %w", err)
		}
		ks.salt = salt
	}

	return ks, nil
}

// SetPassword sets the master password for encryption
func (ks *KeyStore) SetPassword(password string) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	// Derive key from password using PBKDF2
	ks.password = pbkdf2.Key([]byte(password), ks.salt, 100000, 32, sha256.New)
}

// StoreAPIKey stores an encrypted API key
func (ks *KeyStore) StoreAPIKey(key APIKey) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if len(ks.password) == 0 {
		return fmt.Errorf("password not set")
	}

	// Load existing keys
	keys, err := ks.loadKeys()
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Update or add key
	keyID := fmt.Sprintf("%s_%s", key.Exchange, key.Market)
	key.UpdatedAt = time.Now()
	if key.CreatedAt.IsZero() {
		key.CreatedAt = key.UpdatedAt
	}
	key.RotationDue = key.UpdatedAt.Add(30 * 24 * time.Hour) // 30 days

	keys[keyID] = key

	// Save encrypted keys
	return ks.saveKeys(keys)
}

// GetAPIKey retrieves a decrypted API key
func (ks *KeyStore) GetAPIKey(exchange, market string) (*APIKey, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	if len(ks.password) == 0 {
		return nil, fmt.Errorf("password not set")
	}

	keys, err := ks.loadKeys()
	if err != nil {
		return nil, err
	}

	keyID := fmt.Sprintf("%s_%s", exchange, market)
	key, exists := keys[keyID]
	if !exists {
		return nil, fmt.Errorf("API key not found for %s %s", exchange, market)
	}

	return &key, nil
}

// ListKeys lists all stored API keys (without secrets)
func (ks *KeyStore) ListKeys() ([]APIKey, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	if len(ks.password) == 0 {
		return nil, fmt.Errorf("password not set")
	}

	keys, err := ks.loadKeys()
	if err != nil {
		return nil, err
	}

	result := make([]APIKey, 0, len(keys))
	for _, key := range keys {
		// Mask sensitive data
		safeKey := key
		if len(safeKey.APIKey) > 8 {
			safeKey.APIKey = safeKey.APIKey[:8] + "..."
		}
		safeKey.SecretKey = "***"
		if safeKey.Passphrase != "" {
			safeKey.Passphrase = "***"
		}
		result = append(result, safeKey)
	}

	return result, nil
}

// CheckRotation checks which keys need rotation
func (ks *KeyStore) CheckRotation() ([]string, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	if len(ks.password) == 0 {
		return nil, fmt.Errorf("password not set")
	}

	keys, err := ks.loadKeys()
	if err != nil {
		return nil, err
	}

	var needRotation []string
	now := time.Now()

	for keyID, key := range keys {
		if now.After(key.RotationDue) {
			needRotation = append(needRotation, keyID)
		}
	}

	return needRotation, nil
}

// loadKeys loads and decrypts keys from file
func (ks *KeyStore) loadKeys() (map[string]APIKey, error) {
	data, err := os.ReadFile(ks.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]APIKey), nil
		}
		return nil, err
	}

	// Decrypt data
	decrypted, err := ks.decrypt(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt keys: %w", err)
	}

	var keys map[string]APIKey
	if err := json.Unmarshal(decrypted, &keys); err != nil {
		return nil, fmt.Errorf("failed to unmarshal keys: %w", err)
	}

	return keys, nil
}

// saveKeys encrypts and saves keys to file
func (ks *KeyStore) saveKeys(keys map[string]APIKey) error {
	data, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("failed to marshal keys: %w", err)
	}

	// Encrypt data
	encrypted, err := ks.encrypt(data)
	if err != nil {
		return fmt.Errorf("failed to encrypt keys: %w", err)
	}

	// Write to temporary file first
	tmpFile := ks.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, encrypted, 0600); err != nil {
		return fmt.Errorf("failed to write keys: %w", err)
	}

	// Atomic rename
	return os.Rename(tmpFile, ks.filePath)
}

// encrypt encrypts data using AES-GCM
func (ks *KeyStore) encrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(ks.password)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return []byte(base64.StdEncoding.EncodeToString(ciphertext)), nil
}

// decrypt decrypts data using AES-GCM
func (ks *KeyStore) decrypt(data []byte) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(ks.password)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, nil)
}