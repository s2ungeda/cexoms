package generators

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// AccountGenerator generates realistic account data for testing
type AccountGenerator struct {
	rand       *rand.Rand
	accountSeq int
}

// NewAccountGenerator creates a new account generator
func NewAccountGenerator(seed int64) *AccountGenerator {
	return &AccountGenerator{
		rand:       rand.New(rand.NewSource(seed)),
		accountSeq: 1000,
	}
}

// GenerateAccountInfo generates account information
func (g *AccountGenerator) GenerateAccountInfo(accountType types.AccountType) *types.AccountInfo {
	g.accountSeq++
	
	balances := make(map[string]types.Balance)
	
	// Common assets
	assets := []string{"USDT", "BTC", "ETH", "BNB"}
	
	for _, asset := range assets {
		if g.rand.Float64() < 0.7 { // 70% chance to have the asset
			balance := g.generateBalance(asset)
			if !balance.Free.IsZero() || !balance.Locked.IsZero() {
				balances[asset] = balance
			}
		}
	}
	
	// Ensure at least USDT balance
	if _, exists := balances["USDT"]; !exists {
		balances["USDT"] = types.Balance{
			Asset:     "USDT",
			Free:      decimal.NewFromFloat(10000),
			Locked:    decimal.Zero,
			Available: decimal.NewFromFloat(10000),
		}
	}
	
	accountInfo := &types.AccountInfo{
		AccountID:   fmt.Sprintf("ACC_%d", g.accountSeq),
		AccountType: accountType,
		Balances:    balances,
		UpdateTime:  time.Now(),
	}
	
	// Add futures-specific fields if applicable
	if accountType == types.AccountTypeFutures {
		totalBalance := g.calculateTotalBalance(balances)
		marginUsed := totalBalance.Mul(decimal.NewFromFloat(0.2 + g.rand.Float64()*0.3))
		
		accountInfo.TotalBalance = totalBalance
		accountInfo.AvailableBalance = totalBalance.Sub(marginUsed)
		accountInfo.TotalMargin = marginUsed
		accountInfo.TotalUnrealizedPnL = g.generatePnL()
	}
	
	return accountInfo
}

// GenerateBalance generates a balance for an asset
func (g *AccountGenerator) generateBalance(asset string) types.Balance {
	// Different ranges for different assets
	var freeAmount float64
	
	switch asset {
	case "USDT":
		// USDT: 100 - 100,000
		freeAmount = 100 + g.rand.Float64()*99900
	case "BTC":
		// BTC: 0.001 - 10
		freeAmount = 0.001 + g.rand.Float64()*9.999
	case "ETH":
		// ETH: 0.01 - 100
		freeAmount = 0.01 + g.rand.Float64()*99.99
	default:
		// Others: 1 - 1000
		freeAmount = 1 + g.rand.Float64()*999
	}
	
	// Some might be locked in orders (10% chance)
	lockedAmount := 0.0
	if g.rand.Float64() < 0.1 {
		lockedAmount = freeAmount * g.rand.Float64() * 0.3 // Up to 30% locked
	}
	
	free := decimal.NewFromFloat(freeAmount)
	locked := decimal.NewFromFloat(lockedAmount)
	
	return types.Balance{
		Asset:     asset,
		Free:      free,
		Locked:    locked,
		Available: free,
	}
}

// GeneratePositions generates multiple positions for an account
func (g *AccountGenerator) GeneratePositions(count int) []*types.Position {
	positions := make([]*types.Position, count)
	
	symbols := []string{"BTCUSDT", "ETHUSDT", "BNBUSDT", "ADAUSDT", "DOGEUSDT"}
	
	for i := 0; i < count; i++ {
		symbol := symbols[g.rand.Intn(len(symbols))]
		positions[i] = g.generatePosition(symbol)
	}
	
	return positions
}

// GenerateOrderHistory generates order history
func (g *AccountGenerator) GenerateOrderHistory(symbol string, count int) []*types.Order {
	orders := make([]*types.Order, count)
	
	// Generate orders going back in time
	currentTime := time.Now()
	
	for i := 0; i < count; i++ {
		// Orders spaced 1-60 minutes apart
		minutesAgo := (i + 1) * (1 + g.rand.Intn(60))
		orderTime := currentTime.Add(-time.Duration(minutesAgo) * time.Minute)
		
		order := g.generateHistoricalOrder(symbol, orderTime)
		orders[count-1-i] = order // Reverse to have newest first
	}
	
	return orders
}

// GenerateTradingMetrics generates trading metrics
func (g *AccountGenerator) GenerateTradingMetrics(period string) map[string]interface{} {
	metrics := make(map[string]interface{})
	
	// Base metrics
	totalTrades := 100 + g.rand.Intn(900)
	winRate := 0.4 + g.rand.Float64()*0.3 // 40% - 70%
	
	wins := int(float64(totalTrades) * winRate)
	losses := totalTrades - wins
	
	// Financial metrics
	totalVolume := decimal.NewFromFloat(10000 + g.rand.Float64()*990000)
	grossProfit := decimal.NewFromFloat(1000 + g.rand.Float64()*9000)
	grossLoss := decimal.NewFromFloat(500 + g.rand.Float64()*4500)
	netProfit := grossProfit.Sub(grossLoss)
	
	// Calculate ratios
	avgWin := grossProfit.Div(decimal.NewFromInt(int64(wins)))
	avgLoss := grossLoss.Div(decimal.NewFromInt(int64(losses)))
	profitFactor := grossProfit.Div(grossLoss)
	
	metrics["period"] = period
	metrics["total_trades"] = totalTrades
	metrics["winning_trades"] = wins
	metrics["losing_trades"] = losses
	metrics["win_rate"] = winRate
	metrics["total_volume"] = totalVolume.String()
	metrics["gross_profit"] = grossProfit.String()
	metrics["gross_loss"] = grossLoss.String()
	metrics["net_profit"] = netProfit.String()
	metrics["average_win"] = avgWin.String()
	metrics["average_loss"] = avgLoss.String()
	metrics["profit_factor"] = profitFactor.InexactFloat64()
	metrics["largest_win"] = avgWin.Mul(decimal.NewFromFloat(2.5)).String()
	metrics["largest_loss"] = avgLoss.Mul(decimal.NewFromFloat(3.0)).String()
	metrics["max_consecutive_wins"] = 3 + g.rand.Intn(7)
	metrics["max_consecutive_losses"] = 2 + g.rand.Intn(5)
	
	return metrics
}

// Helper methods

func (g *AccountGenerator) calculateTotalBalance(balances map[string]types.Balance) decimal.Decimal {
	// Simple calculation - in reality would need price conversion
	total := decimal.Zero
	
	for asset, balance := range balances {
		if asset == "USDT" {
			total = total.Add(balance.Free).Add(balance.Locked)
		} else {
			// Simplified: assume BTC=40000, ETH=2500, others=100
			var price float64
			switch asset {
			case "BTC":
				price = 40000
			case "ETH":
				price = 2500
			default:
				price = 100
			}
			assetValue := balance.Free.Add(balance.Locked).Mul(decimal.NewFromFloat(price))
			total = total.Add(assetValue)
		}
	}
	
	return total
}

func (g *AccountGenerator) generatePnL() decimal.Decimal {
	// Generate realistic PnL (-5% to +10% of typical account size)
	pnlPercent := -0.05 + g.rand.Float64()*0.15
	basePnL := 10000 * pnlPercent
	return decimal.NewFromFloat(basePnL)
}

func (g *AccountGenerator) generatePosition(symbol string) *types.Position {
	// Random position details
	side := types.Side("LONG")
	if g.rand.Float64() > 0.5 {
		side = types.Side("SHORT")
	}
	
	quantity := 0.01 + g.rand.Float64()*4.99
	entryPrice := 30000 + g.rand.Float64()*20000 // $30k - $50k range
	
	// Mark price deviates slightly from entry
	priceChange := -0.02 + g.rand.Float64()*0.04 // -2% to +2%
	markPrice := entryPrice * (1 + priceChange)
	
	// Calculate PnL
	var pnl float64
	if side == types.Side("LONG") {
		pnl = (markPrice - entryPrice) * quantity
	} else {
		pnl = (entryPrice - markPrice) * quantity
	}
	
	// Leverage between 1x and 20x
	leverage := 1 + g.rand.Intn(20)
	
	return &types.Position{
		Symbol:        symbol,
		Side:          side,
		Amount:        decimal.NewFromFloat(quantity),
		EntryPrice:    decimal.NewFromFloat(entryPrice),
		MarkPrice:     decimal.NewFromFloat(markPrice),
		UnrealizedPnL: decimal.NewFromFloat(pnl),
		RealizedPnL:   decimal.Zero,
		Leverage:      leverage,
		MarginType:    "isolated",
	}
}

func (g *AccountGenerator) generateHistoricalOrder(symbol string, orderTime time.Time) *types.Order {
	// Most historical orders are filled
	status := types.OrderStatusFilled
	if g.rand.Float64() < 0.1 {
		status = types.OrderStatusCanceled
	}
	
	orderType := types.OrderTypeLimit
	if g.rand.Float64() < 0.3 {
		orderType = types.OrderTypeMarket
	}
	
	side := types.OrderSideBuy
	if g.rand.Float64() > 0.5 {
		side = types.OrderSideSell
	}
	
	quantity := 0.01 + g.rand.Float64()*4.99
	price := 30000 + g.rand.Float64()*20000
	
	order := &types.Order{
		OrderID:       fmt.Sprintf("HIST_%d_%d", g.accountSeq, g.rand.Intn(100000)),
		ClientOrderID: fmt.Sprintf("CLIENT_HIST_%d_%d", g.accountSeq, g.rand.Intn(100000)),
		Symbol:        symbol,
		Side:          side,
		Type:          orderType,
		Status:        status,
		Quantity:      decimal.NewFromFloat(quantity),
		CreatedAt:     orderTime,
		UpdatedAt:     orderTime.Add(time.Second * time.Duration(g.rand.Intn(10))),
	}
	
	if orderType == types.OrderTypeLimit {
		order.Price = decimal.NewFromFloat(price)
	}
	
	if status == types.OrderStatusFilled {
		order.FilledQuantity = order.Quantity
		order.FilledPrice = decimal.NewFromFloat(price)
		order.FilledAt = &order.UpdatedAt
	}
	
	return order
}