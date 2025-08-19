package arbitrage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mExOms/oms/pkg/types"
	"github.com/shopspring/decimal"
)

// ArbitrageDetector detects arbitrage opportunities across exchanges
type ArbitrageDetector struct {
	mu sync.RWMutex
	
	// Exchange connections
	exchanges      map[string]types.ExchangeMultiAccount
	
	// Price monitoring
	priceFeeds     map[string]map[string]*PriceFeed  // exchange -> symbol -> price
	opportunities  map[string]*ArbitrageOpportunity
	
	// Configuration
	config         *DetectorConfig
	
	// Channels
	opportunityChan chan *ArbitrageOpportunity
	
	// Background workers
	stopCh         chan struct{}
	wg             sync.WaitGroup
}

// PriceFeed represents real-time price data from an exchange
type PriceFeed struct {
	Exchange      string
	Symbol        string
	BidPrice      decimal.Decimal
	BidQuantity   decimal.Decimal
	AskPrice      decimal.Decimal
	AskQuantity   decimal.Decimal
	Timestamp     time.Time
	LastUpdate    time.Time
}

// ArbitrageOpportunity represents a profitable arbitrage opportunity
type ArbitrageOpportunity struct {
	ID            string
	Symbol        string
	BuyExchange   string
	SellExchange  string
	BuyPrice      decimal.Decimal
	SellPrice     decimal.Decimal
	MaxQuantity   decimal.Decimal
	ProfitRate    decimal.Decimal
	ProfitAmount  decimal.Decimal
	
	// Execution details
	BuyAccount    string
	SellAccount   string
	
	// Fees
	BuyFee        decimal.Decimal
	SellFee       decimal.Decimal
	NetProfit     decimal.Decimal
	
	// Timing
	DetectedAt    time.Time
	ValidUntil    time.Time
	
	// Status
	Status        OpportunityStatus
	Confidence    decimal.Decimal
}

// OpportunityStatus represents the status of an arbitrage opportunity
type OpportunityStatus string

const (
	StatusDetected   OpportunityStatus = "detected"
	StatusExecuting  OpportunityStatus = "executing"
	StatusExecuted   OpportunityStatus = "executed"
	StatusExpired    OpportunityStatus = "expired"
	StatusFailed     OpportunityStatus = "failed"
)

// DetectorConfig contains configuration for arbitrage detection
type DetectorConfig struct {
	// Detection parameters
	MinProfitRate       decimal.Decimal  // Minimum profit rate (e.g., 0.001 = 0.1%)
	MinProfitAmount     decimal.Decimal  // Minimum profit in USDT
	MaxPositionSize     decimal.Decimal  // Maximum position size per opportunity
	
	// Exchange fees
	ExchangeFees        map[string]FeeStructure
	
	// Timing
	PriceUpdateInterval time.Duration
	OpportunityTTL      time.Duration
	ExecutionTimeout    time.Duration
	
	// Risk limits
	MaxConcurrentOpps   int
	MaxDailyVolume      decimal.Decimal
	
	// Symbols to monitor
	MonitoredSymbols    []string
}

// FeeStructure represents exchange fee structure
type FeeStructure struct {
	MakerFee    decimal.Decimal
	TakerFee    decimal.Decimal
	WithdrawFee decimal.Decimal
}

// NewArbitrageDetector creates a new arbitrage detector
func NewArbitrageDetector(config *DetectorConfig) *ArbitrageDetector {
	if config == nil {
		config = &DetectorConfig{
			MinProfitRate:       decimal.NewFromFloat(0.001), // 0.1%
			MinProfitAmount:     decimal.NewFromInt(10),      // $10 minimum
			MaxPositionSize:     decimal.NewFromInt(10000),   // $10k max
			PriceUpdateInterval: 100 * time.Millisecond,
			OpportunityTTL:      500 * time.Millisecond,
			ExecutionTimeout:    1 * time.Second,
			MaxConcurrentOpps:   10,
			MaxDailyVolume:      decimal.NewFromInt(1000000), // $1M daily
			MonitoredSymbols: []string{
				"BTCUSDT", "ETHUSDT", "BNBUSDT", "SOLUSDT", "XRPUSDT",
			},
		}
	}
	
	// Initialize default fees if not provided
	if config.ExchangeFees == nil {
		config.ExchangeFees = map[string]FeeStructure{
			"binance": {
				MakerFee: decimal.NewFromFloat(0.001),
				TakerFee: decimal.NewFromFloat(0.001),
			},
			"bybit": {
				MakerFee: decimal.NewFromFloat(0.001),
				TakerFee: decimal.NewFromFloat(0.001),
			},
			"okx": {
				MakerFee: decimal.NewFromFloat(0.0008),
				TakerFee: decimal.NewFromFloat(0.001),
			},
		}
	}
	
	return &ArbitrageDetector{
		exchanges:       make(map[string]types.ExchangeMultiAccount),
		priceFeeds:      make(map[string]map[string]*PriceFeed),
		opportunities:   make(map[string]*ArbitrageOpportunity),
		config:          config,
		opportunityChan: make(chan *ArbitrageOpportunity, 100),
		stopCh:          make(chan struct{}),
	}
}

// RegisterExchange registers an exchange for arbitrage monitoring
func (ad *ArbitrageDetector) RegisterExchange(name string, exchange types.ExchangeMultiAccount) {
	ad.mu.Lock()
	defer ad.mu.Unlock()
	
	ad.exchanges[name] = exchange
	ad.priceFeeds[name] = make(map[string]*PriceFeed)
}

// Start starts the arbitrage detector
func (ad *ArbitrageDetector) Start(ctx context.Context) error {
	// Subscribe to price feeds
	for _, symbol := range ad.config.MonitoredSymbols {
		for exchangeName, exchange := range ad.exchanges {
			if err := ad.subscribeToPriceFeed(exchangeName, exchange, symbol); err != nil {
				return fmt.Errorf("failed to subscribe to %s on %s: %w", symbol, exchangeName, err)
			}
		}
	}
	
	// Start detection workers
	ad.wg.Add(2)
	go ad.detectionWorker()
	go ad.cleanupWorker()
	
	return nil
}

// Stop stops the arbitrage detector
func (ad *ArbitrageDetector) Stop() {
	close(ad.stopCh)
	ad.wg.Wait()
	close(ad.opportunityChan)
}

// GetOpportunityChannel returns the opportunity channel
func (ad *ArbitrageDetector) GetOpportunityChannel() <-chan *ArbitrageOpportunity {
	return ad.opportunityChan
}

// subscribeToPriceFeed subscribes to price updates from an exchange
func (ad *ArbitrageDetector) subscribeToPriceFeed(exchangeName string, exchange types.ExchangeMultiAccount, symbol string) error {
	// Subscribe to order book updates
	callback := func(orderBook *types.OrderBook) {
		ad.updatePriceFeed(exchangeName, symbol, orderBook)
	}
	
	return exchange.SubscribeOrderBook(symbol, callback)
}

// updatePriceFeed updates price feed with new order book data
func (ad *ArbitrageDetector) updatePriceFeed(exchangeName, symbol string, orderBook *types.OrderBook) {
	ad.mu.Lock()
	defer ad.mu.Unlock()
	
	if len(orderBook.Bids) == 0 || len(orderBook.Asks) == 0 {
		return
	}
	
	priceFeed := &PriceFeed{
		Exchange:    exchangeName,
		Symbol:      symbol,
		BidPrice:    orderBook.Bids[0].Price,
		BidQuantity: orderBook.Bids[0].Quantity,
		AskPrice:    orderBook.Asks[0].Price,
		AskQuantity: orderBook.Asks[0].Quantity,
		Timestamp:   orderBook.UpdatedAt,
		LastUpdate:  time.Now(),
	}
	
	ad.priceFeeds[exchangeName][symbol] = priceFeed
	
	// Trigger opportunity detection
	ad.detectOpportunities(symbol)
}

// detectOpportunities detects arbitrage opportunities for a symbol
func (ad *ArbitrageDetector) detectOpportunities(symbol string) {
	// Collect prices from all exchanges
	var priceData []PriceFeed
	for exchangeName, symbols := range ad.priceFeeds {
		if feed, exists := symbols[symbol]; exists {
			// Skip stale prices
			if time.Since(feed.LastUpdate) > 1*time.Second {
				continue
			}
			priceData = append(priceData, *feed)
		}
	}
	
	if len(priceData) < 2 {
		return // Need at least 2 exchanges
	}
	
	// Check all exchange pairs
	for i := 0; i < len(priceData); i++ {
		for j := i + 1; j < len(priceData); j++ {
			// Check buy on i, sell on j
			ad.checkArbitrageOpportunity(priceData[i], priceData[j], symbol)
			
			// Check buy on j, sell on i
			ad.checkArbitrageOpportunity(priceData[j], priceData[i], symbol)
		}
	}
}

// checkArbitrageOpportunity checks if there's an arbitrage opportunity
func (ad *ArbitrageDetector) checkArbitrageOpportunity(buy, sell PriceFeed, symbol string) {
	// Calculate gross profit
	priceDiff := sell.BidPrice.Sub(buy.AskPrice)
	if priceDiff.LessThanOrEqual(decimal.Zero) {
		return // No profit
	}
	
	// Calculate profit rate
	profitRate := priceDiff.Div(buy.AskPrice)
	if profitRate.LessThan(ad.config.MinProfitRate) {
		return // Below minimum profit rate
	}
	
	// Calculate fees
	buyFee := ad.calculateFee(buy.Exchange, buy.AskPrice, true)
	sellFee := ad.calculateFee(sell.Exchange, sell.BidPrice, false)
	totalFees := buyFee.Add(sellFee)
	
	// Calculate net profit rate
	netProfitRate := profitRate.Sub(totalFees.Div(buy.AskPrice))
	if netProfitRate.LessThan(ad.config.MinProfitRate) {
		return // Not profitable after fees
	}
	
	// Calculate maximum quantity
	maxQty := decimal.Min(buy.AskQuantity, sell.BidQuantity)
	maxValue := maxQty.Mul(buy.AskPrice)
	
	// Apply position size limit
	if maxValue.GreaterThan(ad.config.MaxPositionSize) {
		maxQty = ad.config.MaxPositionSize.Div(buy.AskPrice)
		maxValue = ad.config.MaxPositionSize
	}
	
	// Calculate net profit amount
	grossProfit := maxQty.Mul(priceDiff)
	netProfit := grossProfit.Sub(maxQty.Mul(buy.AskPrice).Mul(totalFees.Div(buy.AskPrice)))
	
	if netProfit.LessThan(ad.config.MinProfitAmount) {
		return // Below minimum profit amount
	}
	
	// Create opportunity
	opportunity := &ArbitrageOpportunity{
		ID:           fmt.Sprintf("%s_%s_%s_%d", symbol, buy.Exchange, sell.Exchange, time.Now().UnixNano()),
		Symbol:       symbol,
		BuyExchange:  buy.Exchange,
		SellExchange: sell.Exchange,
		BuyPrice:     buy.AskPrice,
		SellPrice:    sell.BidPrice,
		MaxQuantity:  maxQty,
		ProfitRate:   netProfitRate,
		ProfitAmount: grossProfit,
		BuyFee:       buyFee,
		SellFee:      sellFee,
		NetProfit:    netProfit,
		DetectedAt:   time.Now(),
		ValidUntil:   time.Now().Add(ad.config.OpportunityTTL),
		Status:       StatusDetected,
		Confidence:   ad.calculateConfidence(netProfitRate, maxValue),
	}
	
	// Store and notify
	ad.mu.Lock()
	ad.opportunities[opportunity.ID] = opportunity
	ad.mu.Unlock()
	
	// Send to channel
	select {
	case ad.opportunityChan <- opportunity:
	default:
		// Channel full, skip
	}
}

// calculateFee calculates trading fee
func (ad *ArbitrageDetector) calculateFee(exchange string, price decimal.Decimal, isTaker bool) decimal.Decimal {
	feeStructure, exists := ad.config.ExchangeFees[exchange]
	if !exists {
		// Default fee
		return decimal.NewFromFloat(0.001)
	}
	
	if isTaker {
		return price.Mul(feeStructure.TakerFee)
	}
	return price.Mul(feeStructure.MakerFee)
}

// calculateConfidence calculates confidence score for an opportunity
func (ad *ArbitrageDetector) calculateConfidence(profitRate, volume decimal.Decimal) decimal.Decimal {
	// Base confidence on profit rate
	confidence := profitRate.Mul(decimal.NewFromInt(100))
	
	// Adjust for volume
	if volume.GreaterThan(decimal.NewFromInt(1000)) {
		confidence = confidence.Mul(decimal.NewFromFloat(1.2))
	}
	
	// Cap at 100
	if confidence.GreaterThan(decimal.NewFromInt(100)) {
		confidence = decimal.NewFromInt(100)
	}
	
	return confidence
}

// detectionWorker continuously detects opportunities
func (ad *ArbitrageDetector) detectionWorker() {
	defer ad.wg.Done()
	
	ticker := time.NewTicker(ad.config.PriceUpdateInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// Re-check all symbols
			for _, symbol := range ad.config.MonitoredSymbols {
				ad.detectOpportunities(symbol)
			}
			
		case <-ad.stopCh:
			return
		}
	}
}

// cleanupWorker removes expired opportunities
func (ad *ArbitrageDetector) cleanupWorker() {
	defer ad.wg.Done()
	
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			ad.mu.Lock()
			now := time.Now()
			for id, opp := range ad.opportunities {
				if now.After(opp.ValidUntil) || opp.Status == StatusExecuted || opp.Status == StatusFailed {
					delete(ad.opportunities, id)
				}
			}
			ad.mu.Unlock()
			
		case <-ad.stopCh:
			return
		}
	}
}

// GetActiveOpportunities returns all active opportunities
func (ad *ArbitrageDetector) GetActiveOpportunities() []*ArbitrageOpportunity {
	ad.mu.RLock()
	defer ad.mu.RUnlock()
	
	var opportunities []*ArbitrageOpportunity
	for _, opp := range ad.opportunities {
		if opp.Status == StatusDetected && time.Now().Before(opp.ValidUntil) {
			opportunities = append(opportunities, opp)
		}
	}
	
	return opportunities
}

// UpdateOpportunityStatus updates the status of an opportunity
func (ad *ArbitrageDetector) UpdateOpportunityStatus(id string, status OpportunityStatus) {
	ad.mu.Lock()
	defer ad.mu.Unlock()
	
	if opp, exists := ad.opportunities[id]; exists {
		opp.Status = status
	}
}