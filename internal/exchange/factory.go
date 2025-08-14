package exchange

import (
	"fmt"
	
	"github.com/mExOms/oms/pkg/types"
	"github.com/spf13/viper"
)

// Config holds exchange configuration
type Config struct {
	APIKey        string
	SecretKey     string
	TestNet       bool
	APIEndpoint   string
	WSEndpoint    string
	RateLimits    types.RateLimits
}

// Factory creates exchange instances
type Factory struct {
	configs map[types.ExchangeType]*Config
}

// NewFactory creates a new exchange factory
func NewFactory() *Factory {
	return &Factory{
		configs: make(map[types.ExchangeType]*Config),
	}
}

// LoadConfig loads exchange configuration from Vault and config file
func (f *Factory) LoadConfig(exchangeType types.ExchangeType) error {
	// TODO: Load from Vault for API keys
	// TODO: Load from config file for endpoints and limits
	
	// Placeholder config
	config := &Config{
		TestNet: viper.GetBool(fmt.Sprintf("exchanges.%s.test_net", getExchangeName(exchangeType))),
		APIEndpoint: viper.GetString(fmt.Sprintf("exchanges.%s.api_endpoint", getExchangeName(exchangeType))),
		WSEndpoint: viper.GetString(fmt.Sprintf("exchanges.%s.ws_endpoint", getExchangeName(exchangeType))),
		RateLimits: types.RateLimits{
			WeightPerMinute: viper.GetInt(fmt.Sprintf("exchanges.%s.rate_limits.weight_per_minute", getExchangeName(exchangeType))),
			OrdersPerSecond: viper.GetInt(fmt.Sprintf("exchanges.%s.rate_limits.orders_per_second", getExchangeName(exchangeType))),
			OrdersPerDay: viper.GetInt(fmt.Sprintf("exchanges.%s.rate_limits.orders_per_day", getExchangeName(exchangeType))),
		},
	}
	
	f.configs[exchangeType] = config
	return nil
}

// CreateExchange creates an exchange instance based on type
func (f *Factory) CreateExchange(exchangeType types.ExchangeType) (types.Exchange, error) {
	_, exists := f.configs[exchangeType]
	if !exists {
		if err := f.LoadConfig(exchangeType); err != nil {
			return nil, fmt.Errorf("failed to load config for %s: %w", exchangeType, err)
		}
	}
	
	switch exchangeType {
	case types.ExchangeBinanceSpot:
		// TODO: Return BinanceSpot instance
		return nil, fmt.Errorf("binance spot connector not yet implemented")
		
	case types.ExchangeBinanceFutures:
		// TODO: Return BinanceFutures instance
		return nil, fmt.Errorf("binance futures connector not yet implemented")
		
	case types.ExchangeBybitSpot:
		// TODO: Return BybitSpot instance
		return nil, fmt.Errorf("bybit spot connector not yet implemented")
		
	case types.ExchangeBybitFutures:
		// TODO: Return BybitFutures instance
		return nil, fmt.Errorf("bybit futures connector not yet implemented")
		
	case types.ExchangeOKXSpot:
		// TODO: Return OKXSpot instance
		return nil, fmt.Errorf("okx spot connector not yet implemented")
		
	case types.ExchangeOKXFutures:
		// TODO: Return OKXFutures instance
		return nil, fmt.Errorf("okx futures connector not yet implemented")
		
	case types.ExchangeUpbit:
		// TODO: Return Upbit instance
		return nil, fmt.Errorf("upbit connector not yet implemented")
		
	default:
		return nil, fmt.Errorf("unsupported exchange type: %s", exchangeType)
	}
}

// getExchangeName returns the config key name for an exchange type
func getExchangeName(exchangeType types.ExchangeType) string {
	switch exchangeType {
	case types.ExchangeBinanceSpot, types.ExchangeBinanceFutures:
		return "binance"
	case types.ExchangeBybitSpot, types.ExchangeBybitFutures:
		return "bybit"
	case types.ExchangeOKXSpot, types.ExchangeOKXFutures:
		return "okx"
	case types.ExchangeUpbit:
		return "upbit"
	default:
		return ""
	}
}