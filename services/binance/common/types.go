package common

import (
	"fmt"
	"time"
	
	"github.com/adshao/go-binance/v2"
	"github.com/mExOms/oms/pkg/types"
	"github.com/shopspring/decimal"
)

// ConvertSymbol converts between standard and Binance format
func ConvertSymbol(standardSymbol string, toBinance bool) string {
	normalizer := &types.BinanceSymbolNormalizer{}
	if toBinance {
		return normalizer.Denormalize(standardSymbol)
	}
	return normalizer.Normalize(standardSymbol)
}

// ConvertSide converts between types.Side and binance.SideType
func ConvertSide(side string) binance.SideType {
	switch side {
	case types.OrderSideBuy:
		return binance.SideTypeBuy
	case types.OrderSideSell:
		return binance.SideTypeSell
	default:
		return binance.SideTypeBuy
	}
}

// ConvertSideFromBinance converts binance.SideType to types.Side
func ConvertSideFromBinance(side binance.SideType) string {
	switch side {
	case binance.SideTypeBuy:
		return types.OrderSideBuy
	case binance.SideTypeSell:
		return types.OrderSideSell
	default:
		return types.OrderSideBuy
	}
}

// ConvertOrderType converts between types.OrderType and binance.OrderType
func ConvertOrderType(orderType string) binance.OrderType {
	switch orderType {
	case types.OrderTypeMarket:
		return binance.OrderTypeMarket
	case types.OrderTypeLimit:
		return binance.OrderTypeLimit
	case types.OrderTypeStop:
		return binance.OrderTypeStopLoss
	case types.OrderTypeStopLimit:
		return binance.OrderTypeStopLossLimit
	case types.OrderTypeTakeProfit:
		return binance.OrderTypeTakeProfit
	case types.OrderTypeTakeProfitLimit:
		return binance.OrderTypeTakeProfitLimit
	default:
		return binance.OrderTypeLimit
	}
}

// ConvertOrderTypeFromBinance converts binance.OrderType to types.OrderType
func ConvertOrderTypeFromBinance(orderType binance.OrderType) string {
	switch orderType {
	case binance.OrderTypeMarket:
		return types.OrderTypeMarket
	case binance.OrderTypeLimit:
		return types.OrderTypeLimit
	case binance.OrderTypeStopLoss:
		return types.OrderTypeStop
	case binance.OrderTypeStopLossLimit:
		return types.OrderTypeStopLimit
	case binance.OrderTypeTakeProfit:
		return types.OrderTypeTakeProfit
	case binance.OrderTypeTakeProfitLimit:
		return types.OrderTypeTakeProfitLimit
	default:
		return types.OrderTypeLimit
	}
}

// ConvertOrderStatus converts between types.OrderStatus and binance.OrderStatusType
func ConvertOrderStatus(status string) binance.OrderStatusType {
	switch status {
	case types.OrderStatusNew:
		return binance.OrderStatusTypeNew
	case types.OrderStatusPartiallyFilled:
		return binance.OrderStatusTypePartiallyFilled
	case types.OrderStatusFilled:
		return binance.OrderStatusTypeFilled
	case types.OrderStatusCanceled:
		return binance.OrderStatusTypeCanceled
	case types.OrderStatusRejected:
		return binance.OrderStatusTypeRejected
	case types.OrderStatusExpired:
		return binance.OrderStatusTypeExpired
	default:
		return binance.OrderStatusTypeNew
	}
}

// ConvertOrderStatusFromBinance converts binance.OrderStatusType to types.OrderStatus
func ConvertOrderStatusFromBinance(status binance.OrderStatusType) string {
	switch status {
	case binance.OrderStatusTypeNew:
		return types.OrderStatusNew
	case binance.OrderStatusTypePartiallyFilled:
		return types.OrderStatusPartiallyFilled
	case binance.OrderStatusTypeFilled:
		return types.OrderStatusFilled
	case binance.OrderStatusTypeCanceled:
		return types.OrderStatusCanceled
	case binance.OrderStatusTypeRejected:
		return types.OrderStatusRejected
	case binance.OrderStatusTypeExpired:
		return types.OrderStatusExpired
	default:
		return types.OrderStatusNew
	}
}

// ConvertTimeInForce converts between types.TimeInForce and binance.TimeInForceType
func ConvertTimeInForce(tif string) binance.TimeInForceType {
	switch tif {
	case types.TimeInForceGTC:
		return binance.TimeInForceTypeGTC
	case types.TimeInForceIOC:
		return binance.TimeInForceTypeIOC
	case types.TimeInForceFOK:
		return binance.TimeInForceTypeFOK
	default:
		return binance.TimeInForceTypeGTC
	}
}

// ConvertTimeInForceFromBinance converts binance.TimeInForceType to types.TimeInForce
func ConvertTimeInForceFromBinance(tif binance.TimeInForceType) string {
	switch tif {
	case binance.TimeInForceTypeGTC:
		return types.TimeInForceGTC
	case binance.TimeInForceTypeIOC:
		return types.TimeInForceIOC
	case binance.TimeInForceTypeFOK:
		return types.TimeInForceFOK
	default:
		return types.TimeInForceGTC
	}
}

// ParseFloat64 safely parses string to float64
func ParseFloat64(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

// ConvertBinanceOrder converts Binance order to standard Order
func ConvertBinanceOrder(bo *binance.Order, exchange string) *types.Order {
	price, _ := decimal.NewFromString(bo.Price)
	quantity, _ := decimal.NewFromString(bo.OrigQuantity)
	
	return &types.Order{
		ID:          fmt.Sprintf("%d", bo.OrderID),
		Symbol:      ConvertSymbol(bo.Symbol, false),
		Side:        ConvertSideFromBinance(bo.Side),
		Type:        ConvertOrderTypeFromBinance(bo.Type),
		Price:       price,
		Quantity:    quantity,
		TimeInForce: ConvertTimeInForceFromBinance(bo.TimeInForce),
		CreatedAt:   time.Unix(bo.Time/1000, (bo.Time%1000)*1000000),
	}
}