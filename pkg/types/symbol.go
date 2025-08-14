package types

import (
	"fmt"
	"strings"
)

// Symbol normalization for multi-exchange support
type SymbolNormalizer interface {
	// Normalize converts exchange-specific symbol to standard format
	Normalize(exchangeSymbol string) string
	// Denormalize converts standard symbol to exchange-specific format
	Denormalize(standardSymbol string) string
}

// StandardSymbol represents normalized symbol format
type StandardSymbol struct {
	BaseAsset  string
	QuoteAsset string
	Market     MarketType
}

type MarketType string

const (
	MarketSpot    MarketType = "SPOT"
	MarketFutures MarketType = "FUTURES"
	MarketMargin  MarketType = "MARGIN"
)

// Parse parses a standard symbol string
func (s *StandardSymbol) Parse(symbol string) error {
	// Standard format: BTC/USDT or BTC-USDT
	parts := strings.Split(symbol, "/")
	if len(parts) != 2 {
		parts = strings.Split(symbol, "-")
	}
	
	if len(parts) != 2 {
		return fmt.Errorf("invalid symbol format: %s", symbol)
	}
	
	s.BaseAsset = strings.ToUpper(parts[0])
	s.QuoteAsset = strings.ToUpper(parts[1])
	return nil
}

// String returns the standard symbol string
func (s *StandardSymbol) String() string {
	return fmt.Sprintf("%s/%s", s.BaseAsset, s.QuoteAsset)
}

// BinanceSymbolNormalizer handles Binance symbol normalization
type BinanceSymbolNormalizer struct{}

func (n *BinanceSymbolNormalizer) Normalize(exchangeSymbol string) string {
	// Binance format: BTCUSDT -> BTC/USDT
	exchangeSymbol = strings.ToUpper(exchangeSymbol)
	
	// Common quote assets
	quoteAssets := []string{"USDT", "USDC", "BUSD", "BTC", "ETH", "BNB"}
	
	for _, quote := range quoteAssets {
		if strings.HasSuffix(exchangeSymbol, quote) {
			base := strings.TrimSuffix(exchangeSymbol, quote)
			return fmt.Sprintf("%s/%s", base, quote)
		}
	}
	
	return exchangeSymbol
}

func (n *BinanceSymbolNormalizer) Denormalize(standardSymbol string) string {
	// BTC/USDT -> BTCUSDT
	return strings.ReplaceAll(standardSymbol, "/", "")
}

// BybitSymbolNormalizer handles Bybit symbol normalization
type BybitSymbolNormalizer struct{}

func (n *BybitSymbolNormalizer) Normalize(exchangeSymbol string) string {
	// Bybit format: BTCUSDT -> BTC/USDT (similar to Binance)
	exchangeSymbol = strings.ToUpper(exchangeSymbol)
	
	quoteAssets := []string{"USDT", "USDC", "USD", "BTC", "ETH"}
	
	for _, quote := range quoteAssets {
		if strings.HasSuffix(exchangeSymbol, quote) {
			base := strings.TrimSuffix(exchangeSymbol, quote)
			return fmt.Sprintf("%s/%s", base, quote)
		}
	}
	
	return exchangeSymbol
}

func (n *BybitSymbolNormalizer) Denormalize(standardSymbol string) string {
	// BTC/USDT -> BTCUSDT
	return strings.ReplaceAll(standardSymbol, "/", "")
}

// OKXSymbolNormalizer handles OKX symbol normalization
type OKXSymbolNormalizer struct{}

func (n *OKXSymbolNormalizer) Normalize(exchangeSymbol string) string {
	// OKX format: BTC-USDT -> BTC/USDT
	return strings.ReplaceAll(strings.ToUpper(exchangeSymbol), "-", "/")
}

func (n *OKXSymbolNormalizer) Denormalize(standardSymbol string) string {
	// BTC/USDT -> BTC-USDT
	return strings.ReplaceAll(standardSymbol, "/", "-")
}

// UpbitSymbolNormalizer handles Upbit symbol normalization
type UpbitSymbolNormalizer struct{}

func (n *UpbitSymbolNormalizer) Normalize(exchangeSymbol string) string {
	// Upbit format: KRW-BTC -> BTC/KRW (reversed)
	parts := strings.Split(strings.ToUpper(exchangeSymbol), "-")
	if len(parts) == 2 {
		return fmt.Sprintf("%s/%s", parts[1], parts[0])
	}
	return exchangeSymbol
}

func (n *UpbitSymbolNormalizer) Denormalize(standardSymbol string) string {
	// BTC/KRW -> KRW-BTC
	parts := strings.Split(standardSymbol, "/")
	if len(parts) == 2 {
		return fmt.Sprintf("%s-%s", parts[1], parts[0])
	}
	return standardSymbol
}

// GetNormalizer returns the appropriate normalizer for an exchange
func GetNormalizer(exchangeType ExchangeType) SymbolNormalizer {
	switch exchangeType {
	case ExchangeBinanceSpot, ExchangeBinanceFutures:
		return &BinanceSymbolNormalizer{}
	case ExchangeBybitSpot, ExchangeBybitFutures:
		return &BybitSymbolNormalizer{}
	case ExchangeOKXSpot, ExchangeOKXFutures:
		return &OKXSymbolNormalizer{}
	case ExchangeUpbit:
		return &UpbitSymbolNormalizer{}
	default:
		return &BinanceSymbolNormalizer{} // Default
	}
}