package router

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// ArbitrageEngine detects and manages arbitrage opportunities
type ArbitrageEngine struct {
	mu                sync.RWMutex
	threshold         decimal.Decimal
	opportunities     map[string]*ArbitrageOpportunity
	historicalProfits map[string]decimal.Decimal
}

// NewArbitrageEngine creates a new arbitrage engine
func NewArbitrageEngine(threshold decimal.Decimal) *ArbitrageEngine {
	return &ArbitrageEngine{
		threshold:         threshold,
		opportunities:     make(map[string]*ArbitrageOpportunity),
		historicalProfits: make(map[string]decimal.Decimal),
	}
}

// FindOpportunities finds arbitrage opportunities from market data
func (ae *ArbitrageEngine) FindOpportunities(marketData map[string]*MarketData, accounts []*AccountRoute) []*ArbitrageOpportunity {
	ae.mu.Lock()
	defer ae.mu.Unlock()

	// Clear old opportunities
	ae.cleanupExpiredOpportunities()

	var opportunities []*ArbitrageOpportunity

	// Group accounts by exchange
	accountsByExchange := make(map[string][]*AccountRoute)
	for _, acc := range accounts {
		accountsByExchange[acc.Exchange] = append(accountsByExchange[acc.Exchange], acc)
	}

	// Find cross-exchange arbitrage
	exchanges := make([]string, 0, len(marketData))
	for ex := range marketData {
		exchanges = append(exchanges, ex)
	}

	// Check all exchange pairs
	for i := 0; i < len(exchanges); i++ {
		for j := i + 1; j < len(exchanges); j++ {
			ex1, ex2 := exchanges[i], exchanges[j]
			data1, data2 := marketData[ex1], marketData[ex2]

			// Skip if no accounts available
			if len(accountsByExchange[ex1]) == 0 || len(accountsByExchange[ex2]) == 0 {
				continue
			}

			// Check buy on ex1, sell on ex2
			if data1.AskPrice.LessThan(data2.BidPrice) {
				profit := ae.calculateProfit(data1.AskPrice, data2.BidPrice, data1.AskVolume, data2.BidVolume)
				if profit.ProfitPercent.GreaterThan(ae.threshold) {
					opp := ae.createOpportunity(
						data1.Symbol, ex1, ex2,
						accountsByExchange[ex1][0], accountsByExchange[ex2][0],
						data1.AskPrice, data2.BidPrice,
						data1.AskVolume, data2.BidVolume,
						profit,
					)
					opportunities = append(opportunities, opp)
					ae.opportunities[opp.ID] = opp
				}
			}

			// Check buy on ex2, sell on ex1
			if data2.AskPrice.LessThan(data1.BidPrice) {
				profit := ae.calculateProfit(data2.AskPrice, data1.BidPrice, data2.AskVolume, data1.BidVolume)
				if profit.ProfitPercent.GreaterThan(ae.threshold) {
					opp := ae.createOpportunity(
						data1.Symbol, ex2, ex1,
						accountsByExchange[ex2][0], accountsByExchange[ex1][0],
						data2.AskPrice, data1.BidPrice,
						data2.AskVolume, data1.BidVolume,
						profit,
					)
					opportunities = append(opportunities, opp)
					ae.opportunities[opp.ID] = opp
				}
			}
		}
	}

	// Sort by profit potential
	sort.Slice(opportunities, func(i, j int) bool {
		return opportunities[i].NetProfit.GreaterThan(opportunities[j].NetProfit)
	})

	return opportunities
}

// calculateProfit calculates arbitrage profit
func (ae *ArbitrageEngine) calculateProfit(buyPrice, sellPrice, buyVolume, sellVolume decimal.Decimal) ArbitrageProfitInfo {
	// Maximum quantity we can trade
	maxQty := decimal.Min(buyVolume, sellVolume)
	
	// Required capital
	requiredCapital := buyPrice.Mul(maxQty)
	
	// Gross profit
	grossProfit := sellPrice.Sub(buyPrice).Mul(maxQty)
	profitPercent := sellPrice.Sub(buyPrice).Div(buyPrice).Mul(decimal.NewFromInt(100))
	
	// Estimate fees (0.1% on each side)
	buyFee := buyPrice.Mul(maxQty).Mul(decimal.NewFromFloat(0.001))
	sellFee := sellPrice.Mul(maxQty).Mul(decimal.NewFromFloat(0.001))
	totalFees := buyFee.Add(sellFee)
	
	// Net profit
	netProfit := grossProfit.Sub(totalFees)
	netProfitPercent := netProfit.Div(requiredCapital).Mul(decimal.NewFromInt(100))

	return ArbitrageProfitInfo{
		MaxQuantity:      maxQty,
		RequiredCapital:  requiredCapital,
		GrossProfit:      grossProfit,
		ProfitPercent:    profitPercent,
		EstimatedFees:    totalFees,
		NetProfit:        netProfit,
		NetProfitPercent: netProfitPercent,
	}
}

// createOpportunity creates an arbitrage opportunity
func (ae *ArbitrageEngine) createOpportunity(
	symbol, buyEx, sellEx string,
	buyAcc, sellAcc *AccountRoute,
	buyPrice, sellPrice, buyVol, sellVol decimal.Decimal,
	profit ArbitrageProfitInfo,
) *ArbitrageOpportunity {
	return &ArbitrageOpportunity{
		ID:              fmt.Sprintf("arb_%s_%s_%s_%d", symbol, buyEx, sellEx, time.Now().UnixNano()),
		Symbol:          symbol,
		BuyExchange:     buyEx,
		BuyAccount:      buyAcc.AccountID,
		SellExchange:    sellEx,
		SellAccount:     sellAcc.AccountID,
		BuyPrice:        buyPrice,
		SellPrice:       sellPrice,
		ProfitPercent:   profit.ProfitPercent,
		ProfitAmount:    profit.GrossProfit,
		MaxQuantity:     profit.MaxQuantity,
		RequiredCapital: profit.RequiredCapital,
		EstimatedFees:   profit.EstimatedFees,
		NetProfit:       profit.NetProfit,
		Confidence:      ae.calculateConfidence(buyEx, sellEx, profit.NetProfitPercent),
		ExpiresAt:       time.Now().Add(5 * time.Second),
		Metadata: map[string]interface{}{
			"buy_volume":        buyVol.String(),
			"sell_volume":       sellVol.String(),
			"net_profit_percent": profit.NetProfitPercent.String(),
		},
	}
}

// calculateConfidence calculates confidence score for arbitrage
func (ae *ArbitrageEngine) calculateConfidence(buyEx, sellEx string, netProfitPercent decimal.Decimal) decimal.Decimal {
	baseConfidence := decimal.NewFromFloat(0.8)
	
	// Higher profit = higher confidence
	if netProfitPercent.GreaterThan(decimal.NewFromFloat(1.0)) {
		baseConfidence = baseConfidence.Add(decimal.NewFromFloat(0.1))
	}
	
	// Historical success rate
	pairKey := fmt.Sprintf("%s_%s", buyEx, sellEx)
	if historicalProfit, exists := ae.historicalProfits[pairKey]; exists {
		if historicalProfit.IsPositive() {
			baseConfidence = baseConfidence.Add(decimal.NewFromFloat(0.05))
		}
	}
	
	// Cap at 0.99
	if baseConfidence.GreaterThan(decimal.NewFromFloat(0.99)) {
		baseConfidence = decimal.NewFromFloat(0.99)
	}
	
	return baseConfidence
}

// RecordExecution records the execution of an arbitrage opportunity
func (ae *ArbitrageEngine) RecordExecution(oppID string, actualProfit decimal.Decimal) {
	ae.mu.Lock()
	defer ae.mu.Unlock()

	if opp, exists := ae.opportunities[oppID]; exists {
		pairKey := fmt.Sprintf("%s_%s", opp.BuyExchange, opp.SellExchange)
		ae.historicalProfits[pairKey] = ae.historicalProfits[pairKey].Add(actualProfit)
		delete(ae.opportunities, oppID)
	}
}

// GetActiveOpportunities returns all active arbitrage opportunities
func (ae *ArbitrageEngine) GetActiveOpportunities() []*ArbitrageOpportunity {
	ae.mu.RLock()
	defer ae.mu.RUnlock()

	var active []*ArbitrageOpportunity
	now := time.Now()

	for _, opp := range ae.opportunities {
		if opp.ExpiresAt.After(now) {
			active = append(active, opp)
		}
	}

	// Sort by net profit
	sort.Slice(active, func(i, j int) bool {
		return active[i].NetProfit.GreaterThan(active[j].NetProfit)
	})

	return active
}

// cleanupExpiredOpportunities removes expired opportunities
func (ae *ArbitrageEngine) cleanupExpiredOpportunities() {
	now := time.Now()
	for id, opp := range ae.opportunities {
		if opp.ExpiresAt.Before(now) {
			delete(ae.opportunities, id)
		}
	}
}

// GetHistoricalStats returns historical arbitrage statistics
func (ae *ArbitrageEngine) GetHistoricalStats() map[string]interface{} {
	ae.mu.RLock()
	defer ae.mu.RUnlock()

	totalProfit := decimal.Zero
	pairStats := make(map[string]decimal.Decimal)

	for pair, profit := range ae.historicalProfits {
		totalProfit = totalProfit.Add(profit)
		pairStats[pair] = profit
	}

	return map[string]interface{}{
		"total_profit": totalProfit.String(),
		"pair_stats":   pairStats,
		"active_count": len(ae.opportunities),
	}
}

// ArbitrageProfitInfo holds detailed profit calculations
type ArbitrageProfitInfo struct {
	MaxQuantity      decimal.Decimal
	RequiredCapital  decimal.Decimal
	GrossProfit      decimal.Decimal
	ProfitPercent    decimal.Decimal
	EstimatedFees    decimal.Decimal
	NetProfit        decimal.Decimal
	NetProfitPercent decimal.Decimal
}