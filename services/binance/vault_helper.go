package binance

import (
	"github.com/mExOms/pkg/vault"
)

// GetVaultClient creates a new Vault client for accessing secrets
func GetVaultClient() (*vault.Client, error) {
	return vault.NewClient(vault.Config{})
}