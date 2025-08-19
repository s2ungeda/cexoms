package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/mExOms/pkg/security"
)

func main() {
	fmt.Println("=== Testing Security Systems ===\n")
	
	// Test encryption
	testEncryption()
	
	// Test file-based secret storage
	testFileSecretStore()
	
	// Test Vault integration
	testVaultIntegration()
	
	fmt.Println("\n✓ All security tests completed!")
}

func testEncryption() {
	fmt.Println("1. Testing Encryption...")
	
	// Generate a new encryption key
	key, err := security.GenerateKey()
	if err != nil {
		log.Printf("Failed to generate key: %v", err)
		return
	}
	fmt.Printf("✓ Generated encryption key: %s...\n", key[:16])
	
	// Create encryptor
	encryptor := security.NewEncryptor("my-secret-key")
	
	// Test encryption/decryption
	plaintext := "API_KEY=abc123xyz789"
	encrypted, err := encryptor.EncryptString(plaintext)
	if err != nil {
		log.Printf("Failed to encrypt: %v", err)
		return
	}
	fmt.Printf("✓ Encrypted data: %s...\n", encrypted[:32])
	
	decrypted, err := encryptor.DecryptString(encrypted)
	if err != nil {
		log.Printf("Failed to decrypt: %v", err)
		return
	}
	
	if decrypted == plaintext {
		fmt.Println("✓ Decryption successful!")
	} else {
		fmt.Println("✗ Decryption failed - data mismatch")
	}
}

func testFileSecretStore() {
	fmt.Println("\n2. Testing File-Based Secret Store...")
	
	// Create temporary directory
	tempDir := filepath.Join(os.TempDir(), "oms-secrets")
	os.MkdirAll(tempDir, 0700)
	defer os.RemoveAll(tempDir)
	
	secretFile := filepath.Join(tempDir, "secrets.json")
	
	// Create file secret store
	store, err := security.NewFileSecretStore(secretFile, "test-encryption-key")
	if err != nil {
		log.Printf("Failed to create file store: %v", err)
		return
	}
	defer store.Close()
	
	fmt.Printf("✓ Created secret store at: %s\n", secretFile)
	
	// Store exchange credentials
	creds := &security.ExchangeCredentials{
		APIKey:    "BINANCE_TEST_API_KEY",
		APISecret: "BINANCE_TEST_SECRET_KEY",
	}
	
	if err := store.StoreExchangeCredentials("binance", "spot", creds); err != nil {
		log.Printf("Failed to store credentials: %v", err)
		return
	}
	fmt.Println("✓ Stored Binance Spot credentials")
	
	// Retrieve credentials
	retrieved, err := store.GetExchangeCredentials("binance", "spot")
	if err != nil {
		log.Printf("Failed to retrieve credentials: %v", err)
		return
	}
	
	fmt.Printf("✓ Retrieved credentials: API Key = %s...\n", retrieved.APIKey[:10])
	
	// List all keys
	keys := store.ListKeys()
	fmt.Printf("✓ Total secrets stored: %d\n", len(keys))
	for _, key := range keys {
		fmt.Printf("  - %s\n", key)
	}
}

func testVaultIntegration() {
	fmt.Println("\n3. Testing Vault Integration...")
	
	// Check if Vault is available
	vaultAddr := os.Getenv("VAULT_ADDR")
	if vaultAddr == "" {
		vaultAddr = "http://localhost:8200"
	}
	
	vaultToken := os.Getenv("VAULT_TOKEN")
	if vaultToken == "" {
		vaultToken = "root-token" // Development token
	}
	
	// Create Vault client
	vaultClient, err := security.NewVaultClient(vaultAddr, vaultToken, "secret")
	if err != nil {
		log.Printf("Failed to create Vault client: %v", err)
		return
	}
	defer vaultClient.Close()
	
	fmt.Printf("✓ Connected to Vault at: %s\n", vaultAddr)
	
	// Store test credentials
	testCreds := &security.ExchangeCredentials{
		APIKey:    "VAULT_TEST_API_KEY",
		APISecret: "VAULT_TEST_SECRET_KEY",
	}
	
	if err := vaultClient.StoreExchangeCredentials("test", "spot", testCreds); err != nil {
		// Vault might not be running, which is okay for this test
		fmt.Printf("⚠️  Could not store in Vault (is Vault running?): %v\n", err)
		return
	}
	
	fmt.Println("✓ Stored test credentials in Vault")
	
	// Retrieve from Vault
	retrieved, err := vaultClient.GetExchangeCredentials("test", "spot")
	if err != nil {
		fmt.Printf("⚠️  Could not retrieve from Vault: %v\n", err)
		return
	}
	
	fmt.Printf("✓ Retrieved from Vault: API Key = %s...\n", retrieved.APIKey[:10])
	
	// List exchanges
	exchanges, err := vaultClient.ListExchanges()
	if err != nil {
		fmt.Printf("⚠️  Could not list exchanges: %v\n", err)
		return
	}
	
	fmt.Printf("✓ Exchanges in Vault: %v\n", exchanges)
}