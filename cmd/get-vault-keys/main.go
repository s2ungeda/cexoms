package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	// Vault configuration
	vaultAddr := "http://127.0.0.1:8200"
	vaultToken := os.Getenv("VAULT_TOKEN")
	if vaultToken == "" {
		vaultToken = "root" // Default development token
	}

	// Get Binance Spot keys from Vault
	client := &http.Client{}
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/v1/secret/data/exchanges/binance_spot", vaultAddr), nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}

	req.Header.Set("X-Vault-Token", vaultToken)
	
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error connecting to Vault: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		return
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Vault returned status %d: %s\n", resp.StatusCode, string(body))
		return
	}

	// Parse the response
	var vaultResp struct {
		Data struct {
			Data map[string]interface{} `json:"data"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &vaultResp); err != nil {
		fmt.Printf("Error parsing response: %v\n", err)
		return
	}

	// Extract API keys
	apiKey, _ := vaultResp.Data.Data["api_key"].(string)
	secretKey, _ := vaultResp.Data.Data["secret_key"].(string)

	if apiKey == "" || secretKey == "" {
		fmt.Println("API keys not found in Vault")
		return
	}

	// Export as environment variables
	fmt.Printf("export BINANCE_API_KEY='%s'\n", apiKey)
	fmt.Printf("export BINANCE_SECRET_KEY='%s'\n", secretKey)
}