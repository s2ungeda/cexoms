package futures

import (
	"context"
	"fmt"
	"time"
	
	"github.com/adshao/go-binance/v2/futures"
	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// ChangePositionMode changes the position mode (One-way or Hedge mode)
func (bf *BinanceFutures) ChangePositionMode(dualSide bool) error {
	if !bf.rateLimiter.Allow("change_position_mode") {
		return fmt.Errorf("rate limit exceeded")
	}
	
	return bf.client.NewChangePositionModeService().
		DualSide(dualSide).
		Do(context.Background())
}

// GetPositionMode gets the current position mode
func (bf *BinanceFutures) GetPositionMode() (string, error) {
	if !bf.rateLimiter.Allow("get_position_mode") {
		return "", fmt.Errorf("rate limit exceeded")
	}
	
	res, err := bf.client.NewGetPositionModeService().Do(context.Background())
	if err != nil {
		return "", err
	}
	
	if res.DualSidePosition {
		return types.PositionModeHedge, nil
	}
	return types.PositionModeOneWay, nil
}

// ModifyIsolatedPositionMargin modifies isolated margin for a position
func (bf *BinanceFutures) ModifyIsolatedPositionMargin(req *types.MarginChangeRequest) error {
	if !bf.rateLimiter.Allow("modify_margin") {
		return fmt.Errorf("rate limit exceeded")
	}
	
	svc := bf.client.NewUpdatePositionMarginService().
		Symbol(req.Symbol).
		Amount(req.Amount.String()).
		Type(req.Type)
		
	if req.PositionSide != "" {
		svc.PositionSide(futures.PositionSideType(req.PositionSide))
	}
	
	err := svc.Do(context.Background())
	return err
}

// GetPositionRisk gets position risk information
func (bf *BinanceFutures) GetPositionRisk(symbol string) ([]*types.PositionRisk, error) {
	if !bf.rateLimiter.Allow("position_risk") {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	
	svc := bf.client.NewGetPositionRiskService()
	if symbol != "" {
		svc.Symbol(symbol)
	}
	
	risks, err := svc.Do(context.Background())
	if err != nil {
		return nil, err
	}
	
	result := make([]*types.PositionRisk, 0, len(risks))
	for _, risk := range risks {
		leverage := int(parseDecimal(risk.Leverage).IntPart())
		
		posRisk := &types.PositionRisk{
			Symbol:           risk.Symbol,
			PositionAmount:   parseDecimal(risk.PositionAmt),
			EntryPrice:       parseDecimal(risk.EntryPrice),
			MarkPrice:        parseDecimal(risk.MarkPrice),
			UnrealizedPnL:    parseDecimal(risk.UnRealizedProfit),
			LiquidationPrice: parseDecimal(risk.LiquidationPrice),
			Leverage:         int(leverage),
			MaxNotionalValue: parseDecimal(risk.MaxNotionalValue),
			MarginType:       risk.MarginType,
			IsolatedMargin:   parseDecimal(risk.IsolatedMargin),
			IsAutoAddMargin:  risk.IsAutoAddMargin == "true",
			PositionSide:     risk.PositionSide,
			Notional:         parseDecimal(risk.Notional),
			IsolatedWallet:   parseDecimal(risk.IsolatedWallet),
			UpdateTime:       time.Now(), // UpdateTime not available in position risk
		}
		
		result = append(result, posRisk)
	}
	
	return result, nil
}

// ClosePosition closes a position at market price
func (bf *BinanceFutures) ClosePosition(symbol string, positionSide string) (*types.OrderResponse, error) {
	// Get current position
	positions, err := bf.GetPositionRisk(symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get position: %w", err)
	}
	
	var position *types.PositionRisk
	for _, pos := range positions {
		if pos.Symbol == symbol && pos.PositionSide == positionSide {
			position = pos
			break
		}
	}
	
	if position == nil || position.PositionAmount.IsZero() {
		return nil, fmt.Errorf("no position found for %s %s", symbol, positionSide)
	}
	
	// Determine order side
	var orderSide string
	if position.PositionAmount.IsPositive() {
		orderSide = types.OrderSideSell
	} else {
		orderSide = types.OrderSideBuy
	}
	
	// Create market order to close position
	order := &types.Order{
		Symbol:       symbol,
		Side:         orderSide,
		Type:         types.OrderTypeMarket,
		Quantity:     position.PositionAmount.Abs(),
		PositionSide: positionSide,
		ReduceOnly:   true,
	}
	
	return bf.CreateOrder(order)
}

// GetLeverage gets current leverage settings for a symbol
func (bf *BinanceFutures) GetLeverage(symbol string) (*types.LeverageInfo, error) {
	if !bf.rateLimiter.Allow("get_leverage") {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	
	// Get position risk to retrieve leverage info
	risks, err := bf.GetPositionRisk(symbol)
	if err != nil {
		return nil, err
	}
	
	if len(risks) == 0 {
		return nil, fmt.Errorf("no leverage info found for %s", symbol)
	}
	
	return &types.LeverageInfo{
		Symbol:           symbol,
		Leverage:         risks[0].Leverage,
		MaxNotionalValue: risks[0].MaxNotionalValue,
	}, nil
}

// GetFundingRate gets the funding rate for a symbol
func (bf *BinanceFutures) GetFundingRate(symbol string) (*types.FundingRate, error) {
	if !bf.rateLimiter.Allow("funding_rate") {
		return nil, fmt.Errorf("rate limit exceeded")
	}
	
	rates, err := bf.client.NewFundingRateService().
		Symbol(symbol).
		Limit(1).
		Do(context.Background())
		
	if err != nil {
		return nil, err
	}
	
	if len(rates) == 0 {
		return nil, fmt.Errorf("no funding rate found for %s", symbol)
	}
	
	rate := rates[0]
	return &types.FundingRate{
		Symbol:      rate.Symbol,
		FundingRate: parseDecimal(rate.FundingRate),
		FundingTime: parseTimestamp(rate.FundingTime),
		MarkPrice:   parseDecimal(rate.MarkPrice),
	}, nil
}

// GetAllOpenPositions gets all open positions
func (bf *BinanceFutures) GetAllOpenPositions() ([]types.FuturesPosition, error) {
	positions, err := bf.GetPositions()
	if err != nil {
		return nil, err
	}
	
	// Filter out positions with zero quantity
	openPositions := make([]types.FuturesPosition, 0)
	for _, pos := range positions {
		if !pos.Quantity.IsZero() {
			openPositions = append(openPositions, pos)
		}
	}
	
	return openPositions, nil
}

// CalculateLiquidationPrice calculates the liquidation price for a position
func (bf *BinanceFutures) CalculateLiquidationPrice(position *types.FuturesPosition) decimal.Decimal {
	// Simplified calculation - actual calculation is more complex
	// This is a placeholder - Binance provides the liquidation price in position info
	
	if position.MarginType == types.MarginTypeCross {
		// Cross margin liquidation is more complex as it involves account balance
		return position.LiquidationPrice
	}
	
	// For isolated margin
	// Long: Liquidation Price = Entry Price × (1 - Initial Margin Rate)
	// Short: Liquidation Price = Entry Price × (1 + Initial Margin Rate)
	initialMarginRate := decimal.NewFromInt(1).Div(decimal.NewFromInt(int64(position.Leverage)))
	
	if position.Quantity.IsPositive() { // Long position
		return position.EntryPrice.Mul(decimal.NewFromInt(1).Sub(initialMarginRate))
	} else { // Short position
		return position.EntryPrice.Mul(decimal.NewFromInt(1).Add(initialMarginRate))
	}
}