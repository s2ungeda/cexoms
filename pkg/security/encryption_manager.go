package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

// EncryptionManager handles encryption/decryption operations
type EncryptionManager struct {
	mu              sync.RWMutex
	masterKey       []byte
	dataKeys        map[string]*DataKey
	rsaKeyPair      *RSAKeyPair
	rotationEnabled bool
	keyDerivation   KeyDerivationFunc
}

// DataKey represents an encryption key
type DataKey struct {
	ID         string
	Key        []byte
	Algorithm  string
	CreatedAt  int64
	ExpiresAt  int64
	Usage      string
	Rotatable  bool
}

// RSAKeyPair holds RSA key pair
type RSAKeyPair struct {
	PrivateKey *rsa.PrivateKey
	PublicKey  *rsa.PublicKey
}

// KeyDerivationFunc is a function for deriving keys
type KeyDerivationFunc func(password, salt []byte) []byte

// EncryptedData represents encrypted data with metadata
type EncryptedData struct {
	CipherText  string `json:"cipher_text"`
	Nonce       string `json:"nonce"`
	Algorithm   string `json:"algorithm"`
	KeyID       string `json:"key_id"`
	AAD         string `json:"aad,omitempty"`
}

// NewEncryptionManager creates a new encryption manager
func NewEncryptionManager(masterKey []byte) (*EncryptionManager, error) {
	if len(masterKey) < 32 {
		return nil, fmt.Errorf("master key must be at least 32 bytes")
	}
	
	// Generate RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}
	
	em := &EncryptionManager{
		masterKey:       masterKey,
		dataKeys:        make(map[string]*DataKey),
		rotationEnabled: true,
		rsaKeyPair: &RSAKeyPair{
			PrivateKey: privateKey,
			PublicKey:  &privateKey.PublicKey,
		},
		keyDerivation: defaultKeyDerivation,
	}
	
	// Generate default data encryption key
	if err := em.generateDataKey("default", "AES-256-GCM"); err != nil {
		return nil, err
	}
	
	return em, nil
}

// Encrypt encrypts data using AES-256-GCM
func (em *EncryptionManager) Encrypt(plaintext []byte, keyID string) (*EncryptedData, error) {
	em.mu.RLock()
	dataKey, exists := em.dataKeys[keyID]
	em.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("encryption key not found: %s", keyID)
	}
	
	// Create cipher
	block, err := aes.NewCipher(dataKey.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	
	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	
	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	
	// Encrypt data
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	
	return &EncryptedData{
		CipherText: base64.StdEncoding.EncodeToString(ciphertext),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Algorithm:  dataKey.Algorithm,
		KeyID:      keyID,
	}, nil
}

// Decrypt decrypts data
func (em *EncryptionManager) Decrypt(encData *EncryptedData) ([]byte, error) {
	em.mu.RLock()
	dataKey, exists := em.dataKeys[encData.KeyID]
	em.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("decryption key not found: %s", encData.KeyID)
	}
	
	// Decode ciphertext and nonce
	ciphertext, err := base64.StdEncoding.DecodeString(encData.CipherText)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}
	
	nonce, err := base64.StdEncoding.DecodeString(encData.Nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to decode nonce: %w", err)
	}
	
	// Create cipher
	block, err := aes.NewCipher(dataKey.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	
	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	
	// Decrypt data
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}
	
	return plaintext, nil
}

// EncryptWithAAD encrypts data with additional authenticated data
func (em *EncryptionManager) EncryptWithAAD(plaintext, aad []byte, keyID string) (*EncryptedData, error) {
	em.mu.RLock()
	dataKey, exists := em.dataKeys[keyID]
	em.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("encryption key not found: %s", keyID)
	}
	
	// Create cipher
	block, err := aes.NewCipher(dataKey.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	
	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	
	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	
	// Encrypt data with AAD
	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)
	
	return &EncryptedData{
		CipherText: base64.StdEncoding.EncodeToString(ciphertext),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Algorithm:  dataKey.Algorithm,
		KeyID:      keyID,
		AAD:        base64.StdEncoding.EncodeToString(aad),
	}, nil
}

// DecryptWithAAD decrypts data with additional authenticated data
func (em *EncryptionManager) DecryptWithAAD(encData *EncryptedData) ([]byte, error) {
	em.mu.RLock()
	dataKey, exists := em.dataKeys[encData.KeyID]
	em.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("decryption key not found: %s", encData.KeyID)
	}
	
	// Decode ciphertext, nonce, and AAD
	ciphertext, err := base64.StdEncoding.DecodeString(encData.CipherText)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}
	
	nonce, err := base64.StdEncoding.DecodeString(encData.Nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to decode nonce: %w", err)
	}
	
	aad, err := base64.StdEncoding.DecodeString(encData.AAD)
	if err != nil {
		return nil, fmt.Errorf("failed to decode AAD: %w", err)
	}
	
	// Create cipher
	block, err := aes.NewCipher(dataKey.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	
	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	
	// Decrypt data with AAD
	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}
	
	return plaintext, nil
}

// EncryptRSA encrypts data using RSA
func (em *EncryptionManager) EncryptRSA(plaintext []byte) ([]byte, error) {
	em.mu.RLock()
	publicKey := em.rsaKeyPair.PublicKey
	em.mu.RUnlock()
	
	// Encrypt with RSA-OAEP
	ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, plaintext, nil)
	if err != nil {
		return nil, fmt.Errorf("RSA encryption failed: %w", err)
	}
	
	return ciphertext, nil
}

// DecryptRSA decrypts data using RSA
func (em *EncryptionManager) DecryptRSA(ciphertext []byte) ([]byte, error) {
	em.mu.RLock()
	privateKey := em.rsaKeyPair.PrivateKey
	em.mu.RUnlock()
	
	// Decrypt with RSA-OAEP
	plaintext, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("RSA decryption failed: %w", err)
	}
	
	return plaintext, nil
}

// GenerateDataKey generates a new data encryption key
func (em *EncryptionManager) generateDataKey(keyID, algorithm string) error {
	keySize := 32 // 256 bits for AES-256
	
	// Generate random key
	key := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return fmt.Errorf("failed to generate key: %w", err)
	}
	
	// Encrypt key with master key
	encryptedKey, err := em.encryptKeyWithMaster(key)
	if err != nil {
		return fmt.Errorf("failed to encrypt data key: %w", err)
	}
	
	em.mu.Lock()
	defer em.mu.Unlock()
	
	em.dataKeys[keyID] = &DataKey{
		ID:        keyID,
		Key:       encryptedKey,
		Algorithm: algorithm,
		CreatedAt: timeNow().Unix(),
		ExpiresAt: timeNow().Add(365 * 24 * time.Hour).Unix(), // 1 year
		Usage:     "data_encryption",
		Rotatable: true,
	}
	
	return nil
}

// RotateKey rotates an encryption key
func (em *EncryptionManager) RotateKey(keyID string) error {
	if !em.rotationEnabled {
		return fmt.Errorf("key rotation is disabled")
	}
	
	em.mu.Lock()
	defer em.mu.Unlock()
	
	oldKey, exists := em.dataKeys[keyID]
	if !exists {
		return fmt.Errorf("key not found: %s", keyID)
	}
	
	if !oldKey.Rotatable {
		return fmt.Errorf("key is not rotatable: %s", keyID)
	}
	
	// Generate new key
	newKey := make([]byte, len(oldKey.Key))
	if _, err := io.ReadFull(rand.Reader, newKey); err != nil {
		return fmt.Errorf("failed to generate new key: %w", err)
	}
	
	// Create new key with rotated suffix
	newKeyID := fmt.Sprintf("%s_rotated_%d", keyID, timeNow().Unix())
	em.dataKeys[newKeyID] = &DataKey{
		ID:        newKeyID,
		Key:       newKey,
		Algorithm: oldKey.Algorithm,
		CreatedAt: timeNow().Unix(),
		ExpiresAt: timeNow().Add(365 * 24 * time.Hour).Unix(),
		Usage:     oldKey.Usage,
		Rotatable: true,
	}
	
	// Mark old key as expired but keep for decryption
	oldKey.ExpiresAt = timeNow().Unix()
	oldKey.Rotatable = false
	
	return nil
}

// ExportPublicKey exports RSA public key in PEM format
func (em *EncryptionManager) ExportPublicKey() (string, error) {
	em.mu.RLock()
	publicKey := em.rsaKeyPair.PublicKey
	em.mu.RUnlock()
	
	// Marshal public key
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal public key: %w", err)
	}
	
	// Encode to PEM
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: pubKeyBytes,
	})
	
	return string(pubKeyPEM), nil
}

// ImportPublicKey imports RSA public key from PEM format
func (em *EncryptionManager) ImportPublicKey(pubKeyPEM string) error {
	// Decode PEM
	block, _ := pem.Decode([]byte(pubKeyPEM))
	if block == nil {
		return fmt.Errorf("failed to decode PEM block")
	}
	
	// Parse public key
	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}
	
	rsaPubKey, ok := pubKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("not an RSA public key")
	}
	
	em.mu.Lock()
	em.rsaKeyPair.PublicKey = rsaPubKey
	em.mu.Unlock()
	
	return nil
}

// DeriveKey derives a key from password
func (em *EncryptionManager) DeriveKey(password string, salt []byte) []byte {
	return em.keyDerivation([]byte(password), salt)
}

// encryptKeyWithMaster encrypts a key with master key
func (em *EncryptionManager) encryptKeyWithMaster(key []byte) ([]byte, error) {
	// For production, use a proper key wrapping algorithm
	// This is a simplified version
	block, err := aes.NewCipher(em.masterKey)
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
	
	// Prepend nonce to ciphertext
	return gcm.Seal(nonce, nonce, key, nil), nil
}

// defaultKeyDerivation is the default key derivation function
func defaultKeyDerivation(password, salt []byte) []byte {
	return pbkdf2.Key(password, salt, 100000, 32, sha256.New)
}

// GenerateSalt generates a random salt
func GenerateSalt(size int) ([]byte, error) {
	salt := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}
	return salt, nil
}

// timeNow is a wrapper for time.Now to aid testing
var timeNow = time.Now