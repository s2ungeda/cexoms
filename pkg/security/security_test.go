package security

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEncryptor(t *testing.T) {
	encryptor := NewEncryptor("test-key-12345")
	
	// Test string encryption
	plaintext := "Hello, World!"
	encrypted, err := encryptor.EncryptString(plaintext)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}
	
	// Test decryption
	decrypted, err := encryptor.DecryptString(encrypted)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}
	
	if decrypted != plaintext {
		t.Errorf("Expected %s, got %s", plaintext, decrypted)
	}
	
	// Test that encrypted text is different each time (due to nonce)
	encrypted2, _ := encryptor.EncryptString(plaintext)
	if encrypted == encrypted2 {
		t.Error("Encrypted text should be different due to random nonce")
	}
	
	// Test invalid ciphertext
	_, err = encryptor.DecryptString("invalid-base64!")
	if err == nil {
		t.Error("Expected error for invalid ciphertext")
	}
}

func TestFileSecretStore(t *testing.T) {
	// Create temporary file
	tempDir, err := os.MkdirTemp("", "secret_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	
	secretFile := filepath.Join(tempDir, "secrets.json")
	
	// Create store
	store, err := NewFileSecretStore(secretFile, "test-encryption-key")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()
	
	// Test storing exchange credentials
	creds := &ExchangeCredentials{
		APIKey:    "test-api-key",
		APISecret: "test-api-secret",
	}
	
	if err := store.StoreExchangeCredentials("binance", "spot", creds); err != nil {
		t.Fatalf("Failed to store credentials: %v", err)
	}
	
	// Test retrieving credentials
	retrieved, err := store.GetExchangeCredentials("binance", "spot")
	if err != nil {
		t.Fatalf("Failed to get credentials: %v", err)
	}
	
	if retrieved.APIKey != creds.APIKey || retrieved.APISecret != creds.APISecret {
		t.Error("Retrieved credentials don't match")
	}
	
	// Test generic secret
	if err := store.SetSecret("test/key", "test-value", time.Hour); err != nil {
		t.Fatalf("Failed to set secret: %v", err)
	}
	
	value, err := store.GetSecret("test/key")
	if err != nil {
		t.Fatalf("Failed to get secret: %v", err)
	}
	
	if value != "test-value" {
		t.Errorf("Expected test-value, got %s", value)
	}
	
	// Test listing keys
	keys := store.ListKeys()
	if len(keys) != 3 { // 2 exchange keys + 1 test key
		t.Errorf("Expected 3 keys, got %d", len(keys))
	}
	
	// Test persistence
	store.Close()
	
	// Create new store and verify data persisted
	store2, err := NewFileSecretStore(secretFile, "test-encryption-key")
	if err != nil {
		t.Fatalf("Failed to create second store: %v", err)
	}
	defer store2.Close()
	
	retrieved2, err := store2.GetExchangeCredentials("binance", "spot")
	if err != nil {
		t.Fatalf("Failed to get credentials from new store: %v", err)
	}
	
	if retrieved2.APIKey != creds.APIKey {
		t.Error("Credentials not persisted correctly")
	}
	
	// Test expiration
	if err := store2.SetSecret("expired", "value", time.Millisecond); err != nil {
		t.Fatalf("Failed to set expiring secret: %v", err)
	}
	
	time.Sleep(2 * time.Millisecond)
	
	_, err = store2.GetSecret("expired")
	if err == nil {
		t.Error("Expected error for expired secret")
	}
	
	// Test deletion
	if err := store2.DeleteSecret("test/key"); err != nil {
		t.Fatalf("Failed to delete secret: %v", err)
	}
	
	_, err = store2.GetSecret("test/key")
	if err == nil {
		t.Error("Expected error for deleted secret")
	}
}

func TestGenerateKey(t *testing.T) {
	key1, err := GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	
	key2, err := GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate second key: %v", err)
	}
	
	if key1 == key2 {
		t.Error("Generated keys should be different")
	}
	
	if len(key1) == 0 {
		t.Error("Generated key should not be empty")
	}
}

func BenchmarkEncryption(b *testing.B) {
	encryptor := NewEncryptor("benchmark-key")
	plaintext := "This is a test message for benchmarking"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encrypted, _ := encryptor.EncryptString(plaintext)
		encryptor.DecryptString(encrypted)
	}
}