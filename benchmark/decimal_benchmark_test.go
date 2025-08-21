package benchmark

import (
	"math/big"
	"testing"

	"github.com/shopspring/decimal"
)

// BenchmarkDecimalArithmetic tests decimal arithmetic operations
func BenchmarkDecimalArithmetic(b *testing.B) {
	d1 := decimal.NewFromFloat(40000.123456789)
	d2 := decimal.NewFromFloat(0.001234567)
	
	b.Run("Addition", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = d1.Add(d2)
		}
	})
	
	b.Run("Subtraction", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = d1.Sub(d2)
		}
	})
	
	b.Run("Multiplication", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = d1.Mul(d2)
		}
	})
	
	b.Run("Division", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = d1.Div(d2)
		}
	})
}

// BenchmarkDecimalComparison tests decimal comparison operations
func BenchmarkDecimalComparison(b *testing.B) {
	d1 := decimal.NewFromFloat(40000.123456789)
	d2 := decimal.NewFromFloat(40000.123456788)
	
	b.Run("Equal", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = d1.Equal(d2)
		}
	})
	
	b.Run("GreaterThan", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = d1.GreaterThan(d2)
		}
	})
	
	b.Run("LessThan", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = d1.LessThan(d2)
		}
	})
}

// BenchmarkDecimalCreation tests decimal creation performance
func BenchmarkDecimalCreation(b *testing.B) {
	b.Run("FromFloat", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = decimal.NewFromFloat(40000.123456789)
		}
	})
	
	b.Run("FromString", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = decimal.NewFromString("40000.123456789")
		}
	})
	
	b.Run("FromInt", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = decimal.NewFromInt(40000)
		}
	})
}

// BenchmarkDecimalVsFloat64 compares decimal vs float64 performance
func BenchmarkDecimalVsFloat64(b *testing.B) {
	// Decimal values
	d1 := decimal.NewFromFloat(40000.123456789)
	d2 := decimal.NewFromFloat(0.001234567)
	
	// Float64 values
	f1 := 40000.123456789
	f2 := 0.001234567
	
	b.Run("Decimal_Multiply", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = d1.Mul(d2)
		}
	})
	
	b.Run("Float64_Multiply", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = f1 * f2
		}
	})
	
	b.Run("Decimal_Add", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = d1.Add(d2)
		}
	})
	
	b.Run("Float64_Add", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = f1 + f2
		}
	})
}

// BenchmarkDecimalVsBigFloat compares decimal vs big.Float performance
func BenchmarkDecimalVsBigFloat(b *testing.B) {
	// Decimal values
	d1 := decimal.NewFromFloat(40000.123456789)
	d2 := decimal.NewFromFloat(0.001234567)
	
	// big.Float values
	b1 := big.NewFloat(40000.123456789)
	b2 := big.NewFloat(0.001234567)
	
	b.Run("Decimal_Multiply", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = d1.Mul(d2)
		}
	})
	
	b.Run("BigFloat_Multiply", func(b *testing.B) {
		result := new(big.Float)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			result.Mul(b1, b2)
		}
	})
}

// BenchmarkPriceCalculations tests common price calculations
func BenchmarkPriceCalculations(b *testing.B) {
	price := decimal.NewFromFloat(40000)
	quantity := decimal.NewFromFloat(0.123456)
	fee := decimal.NewFromFloat(0.001)
	
	b.Run("OrderValue", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = price.Mul(quantity)
		}
	})
	
	b.Run("OrderValueWithFee", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			value := price.Mul(quantity)
			feeAmount := value.Mul(fee)
			_ = value.Add(feeAmount)
		}
	})
	
	b.Run("PnLCalculation", func(b *testing.B) {
		entryPrice := decimal.NewFromFloat(39000)
		exitPrice := decimal.NewFromFloat(40000)
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			priceDiff := exitPrice.Sub(entryPrice)
			pnl := priceDiff.Mul(quantity)
			_ = pnl.Div(entryPrice).Mul(decimal.NewFromInt(100))
		}
	})
}