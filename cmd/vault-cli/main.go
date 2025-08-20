package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"

	"github.com/mExOms/pkg/vault"
	"golang.org/x/term"
)

func main() {
	// Create Vault client
	client, err := vault.NewClient(vault.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to Vault: %v", err)
	}

	// Enable KV v2 if needed
	if err := client.EnableKV2(); err != nil {
		log.Printf("Warning: %v", err)
	}

	// Show menu
	showMenu(client)
}

func showMenu(client *vault.Client) {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println("\n=== OMS Vault API Key Manager ===")
		fmt.Println("1. Add/Update Binance Spot API keys")
		fmt.Println("2. Add/Update Binance Futures API keys")
		fmt.Println("3. Add/Update Bybit API keys")
		fmt.Println("4. Add/Update OKX API keys")
		fmt.Println("5. Add/Update Upbit API keys")
		fmt.Println("6. List all stored keys")
		fmt.Println("7. View specific key details")
		fmt.Println("8. Delete API keys")
		fmt.Println("9. Exit")
		fmt.Print("\nSelect option (1-9): ")

		input, _ := reader.ReadString('\n')
		choice := strings.TrimSpace(input)

		switch choice {
		case "1":
			addBinanceKeys(client, "spot", reader)
		case "2":
			addBinanceKeys(client, "futures", reader)
		case "3":
			addBybitKeys(client, reader)
		case "4":
			addOKXKeys(client, reader)
		case "5":
			addUpbitKeys(client, reader)
		case "6":
			listKeys(client)
		case "7":
			viewKeys(client, reader)
		case "8":
			deleteKeys(client, reader)
		case "9":
			fmt.Println("Exiting...")
			return
		default:
			fmt.Println("Invalid option. Please try again.")
		}
	}
}

func addBinanceKeys(client *vault.Client, market string, reader *bufio.Reader) {
	fmt.Printf("\n=== Binance %s API Keys ===\n", strings.Title(market))
	
	fmt.Print("Enter API Key: ")
	apiKey, _ := reader.ReadString('\n')
	apiKey = strings.TrimSpace(apiKey)

	fmt.Print("Enter Secret Key: ")
	secretKey, err := readPassword()
	if err != nil {
		fmt.Printf("Error reading password: %v\n", err)
		return
	}

	err = client.StoreExchangeKeys("binance", market, apiKey, string(secretKey), nil)
	if err != nil {
		fmt.Printf("Error storing keys: %v\n", err)
		return
	}

	fmt.Printf("\n✓ Binance %s API keys stored successfully\n", market)
}

func addBybitKeys(client *vault.Client, reader *bufio.Reader) {
	fmt.Println("\n=== Bybit API Keys ===")
	
	fmt.Print("Enter API Key: ")
	apiKey, _ := reader.ReadString('\n')
	apiKey = strings.TrimSpace(apiKey)

	fmt.Print("Enter Secret Key: ")
	secretKey, err := readPassword()
	if err != nil {
		fmt.Printf("Error reading password: %v\n", err)
		return
	}

	err = client.StoreExchangeKeys("bybit", "unified", apiKey, string(secretKey), nil)
	if err != nil {
		fmt.Printf("Error storing keys: %v\n", err)
		return
	}

	fmt.Println("\n✓ Bybit API keys stored successfully")
}

func addOKXKeys(client *vault.Client, reader *bufio.Reader) {
	fmt.Println("\n=== OKX API Keys ===")
	
	fmt.Print("Enter API Key: ")
	apiKey, _ := reader.ReadString('\n')
	apiKey = strings.TrimSpace(apiKey)

	fmt.Print("Enter Secret Key: ")
	secretKey, err := readPassword()
	if err != nil {
		fmt.Printf("Error reading password: %v\n", err)
		return
	}

	fmt.Print("\nEnter Passphrase: ")
	passphrase, _ := reader.ReadString('\n')
	passphrase = strings.TrimSpace(passphrase)

	extras := map[string]interface{}{
		"passphrase": passphrase,
	}

	err = client.StoreExchangeKeys("okx", "unified", apiKey, string(secretKey), extras)
	if err != nil {
		fmt.Printf("Error storing keys: %v\n", err)
		return
	}

	fmt.Println("\n✓ OKX API keys stored successfully")
}

func addUpbitKeys(client *vault.Client, reader *bufio.Reader) {
	fmt.Println("\n=== Upbit API Keys ===")
	
	fmt.Print("Enter Access Key: ")
	apiKey, _ := reader.ReadString('\n')
	apiKey = strings.TrimSpace(apiKey)

	fmt.Print("Enter Secret Key: ")
	secretKey, err := readPassword()
	if err != nil {
		fmt.Printf("Error reading password: %v\n", err)
		return
	}

	err = client.StoreExchangeKeys("upbit", "spot", apiKey, string(secretKey), nil)
	if err != nil {
		fmt.Printf("Error storing keys: %v\n", err)
		return
	}

	fmt.Println("\n✓ Upbit API keys stored successfully")
}

func listKeys(client *vault.Client) {
	fmt.Println("\n=== Stored API Keys ===")
	
	keys, err := client.ListExchangeKeys()
	if err != nil {
		fmt.Printf("Error listing keys: %v\n", err)
		return
	}

	if len(keys) == 0 {
		fmt.Println("No API keys stored")
		return
	}

	for _, key := range keys {
		fmt.Printf("- %s\n", key)
	}
}

func viewKeys(client *vault.Client, reader *bufio.Reader) {
	fmt.Print("\nEnter exchange name (binance/bybit/okx/upbit): ")
	exchange, _ := reader.ReadString('\n')
	exchange = strings.TrimSpace(strings.ToLower(exchange))

	fmt.Print("Enter market (spot/futures/unified): ")
	market, _ := reader.ReadString('\n')
	market = strings.TrimSpace(strings.ToLower(market))

	keys, err := client.GetExchangeKeys(exchange, market)
	if err != nil {
		fmt.Printf("Error retrieving keys: %v\n", err)
		return
	}

	fmt.Printf("\n=== %s %s API Keys ===\n", strings.Title(exchange), strings.Title(market))
	
	// Show masked API key
	if apiKey, ok := keys["api_key"]; ok && len(apiKey) > 8 {
		fmt.Printf("API Key: %s...\n", apiKey[:8])
	}
	
	// Don't show secret key
	if _, ok := keys["secret_key"]; ok {
		fmt.Println("Secret Key: ***")
	}
	
	// Show passphrase if exists (for OKX)
	if _, ok := keys["passphrase"]; ok {
		fmt.Println("Passphrase: ***")
	}
}

func deleteKeys(client *vault.Client, reader *bufio.Reader) {
	fmt.Print("\nEnter exchange name (binance/bybit/okx/upbit): ")
	exchange, _ := reader.ReadString('\n')
	exchange = strings.TrimSpace(strings.ToLower(exchange))

	fmt.Print("Enter market (spot/futures/unified): ")
	market, _ := reader.ReadString('\n')
	market = strings.TrimSpace(strings.ToLower(market))

	fmt.Printf("\nAre you sure you want to delete %s %s API keys? (y/n): ", exchange, market)
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(strings.ToLower(confirm))

	if confirm != "y" {
		fmt.Println("Deletion cancelled")
		return
	}

	err := client.DeleteExchangeKeys(exchange, market)
	if err != nil {
		fmt.Printf("Error deleting keys: %v\n", err)
		return
	}

	fmt.Printf("\n✓ %s %s API keys deleted successfully\n", exchange, market)
}

func readPassword() ([]byte, error) {
	fd := int(syscall.Stdin)
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	defer term.Restore(fd, oldState)

	password, err := term.ReadPassword(fd)
	fmt.Println() // New line after password
	return password, err
}