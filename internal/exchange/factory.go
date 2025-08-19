package exchange

import (
	"fmt"
	
	"github.com/mExOms/pkg/types"
	"github.com/mExOms/services/binance"
	// TODO: Import new exchange packages here
	// "github.com/mExOms/services/bybit"
	// "github.com/mExOms/services/okx"
	// "github.com/mExOms/services/upbit"
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
	
	config := f.configs[exchangeType]
	
	switch exchangeType {
	case types.ExchangeBinanceSpot:
		return binance.NewBinanceSpotConnector(
			config.APIKey,
			config.SecretKey,
			config.TestNet,
		), nil
		
	case types.ExchangeBinanceFutures:
		return binance.NewBinanceFuturesConnector(
			config.APIKey,
			config.SecretKey,
			config.TestNet,
		), nil
		
	// TODO: Add new exchanges here following this pattern:
	// case types.ExchangeBybitSpot:
	//     return bybit.NewBybitConnector(
	//         config.APIKey,
	//         config.SecretKey,
	//     ), nil
		
	// case types.ExchangeBybitFutures:
	//     return bybit.NewBybitFuturesConnector(
	//         config.APIKey,
	//         config.SecretKey,
	//     ), nil
		
	// case types.ExchangeOKXSpot, types.ExchangeOKXFutures:
	//     // OKX uses same connector for spot and futures
	//     return okx.NewOKXConnector(
	//         config.APIKey,
	//         config.SecretKey,
	//         config.APIPassphrase, // OKX requires passphrase
	//     ), nil
		
	// case types.ExchangeUpbit:
	//     return upbit.NewUpbitConnector(
	//         config.APIKey,
	//         config.SecretKey,
	//     ), nil
		
	case types.ExchangeBybitSpot, types.ExchangeBybitFutures,
		types.ExchangeOKXSpot, types.ExchangeOKXFutures,
		types.ExchangeUpbit:
		return nil, fmt.Errorf("%s connector not yet implemented - use generate-exchange tool", exchangeType)
		
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