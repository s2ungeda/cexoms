package benchmark

import (
	"testing"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// BenchmarkTickerProcessing tests ticker update processing performance
func BenchmarkTickerProcessing(b *testing.B) {
	ticker := &types.Ticker{
		Symbol: "BTCUSDT",
		Price:  decimal.NewFromFloat(40000),
		Volume: decimal.NewFromFloat(1000),
		Bid:    decimal.NewFromFloat(39999),
		Ask:    decimal.NewFromFloat(40001),
	}
	
	// Simulate ticker callback processing
	callback := func(symbol string, t *types.Ticker) {
		// Minimal processing
		_ = t.Price.Mul(t.Volume)
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		callback(ticker.Symbol, ticker)
	}
	
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "tickers/sec")
}

// BenchmarkOrderBookProcessing tests order book update processing
func BenchmarkOrderBookProcessing(b *testing.B) {
	orderBook := &types.OrderBook{
		Symbol: "BTCUSDT",
		Bids:   make([][2]decimal.Decimal, 100),
		Asks:   make([][2]decimal.Decimal, 100),
	}
	
	// Fill order book with data
	for i := 0; i < 100; i++ {
		orderBook.Bids[i] = [2]decimal.Decimal{
			decimal.NewFromFloat(40000 - float64(i)),
			decimal.NewFromFloat(float64(i + 1)),
		}
		orderBook.Asks[i] = [2]decimal.Decimal{
			decimal.NewFromFloat(40001 + float64(i)),
			decimal.NewFromFloat(float64(i + 1)),
		}
	}
	
	// Simulate order book callback processing
	callback := func(symbol string, ob *types.OrderBook) {
		// Calculate mid price
		if len(ob.Bids) > 0 && len(ob.Asks) > 0 {
			_ = ob.Bids[0][0].Add(ob.Asks[0][0]).Div(decimal.NewFromInt(2))
		}
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		callback(orderBook.Symbol, orderBook)
	}
	
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "updates/sec")
}

// BenchmarkTradeProcessing tests trade update processing
func BenchmarkTradeProcessing(b *testing.B) {
	trade := &types.Trade{
		ID:        "12345",
		Symbol:    "BTCUSDT",
		Price:     decimal.NewFromFloat(40000),
		Quantity:  decimal.NewFromFloat(0.1),
		Side:      types.OrderSideBuy,
		Timestamp: time.Now(),
	}
	
	// Simulate trade callback processing
	callback := func(symbol string, t *types.Trade) {
		// Calculate trade value
		_ = t.Price.Mul(t.Quantity)
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		callback(trade.Symbol, trade)
	}
	
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "trades/sec")
}

// BenchmarkKlineProcessing tests kline/candlestick processing
func BenchmarkKlineProcessing(b *testing.B) {
	kline := &types.Kline{
		Symbol:    "BTCUSDT",
		Interval:  "1m",
		OpenTime:  time.Now().Add(-time.Minute),
		CloseTime: time.Now(),
		Open:      decimal.NewFromFloat(39900),
		High:      decimal.NewFromFloat(40100),
		Low:       decimal.NewFromFloat(39800),
		Close:     decimal.NewFromFloat(40000),
		Volume:    decimal.NewFromFloat(100),
	}
	
	// Simulate kline callback processing
	callback := func(symbol string, k *types.Kline) {
		// Calculate price range
		_ = k.High.Sub(k.Low)
		// Calculate typical price
		_ = k.High.Add(k.Low).Add(k.Close).Div(decimal.NewFromInt(3))
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		callback(kline.Symbol, kline)
	}
	
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "klines/sec")
}

// BenchmarkMarketDataAggregation tests aggregating market data from multiple sources
func BenchmarkMarketDataAggregation(b *testing.B) {
	// Create multiple ticker sources
	tickers := make([]*types.Ticker, 5)
	for i := 0; i < 5; i++ {
		tickers[i] = &types.Ticker{
			Symbol: "BTCUSDT",
			Price:  decimal.NewFromFloat(40000 + float64(i)),
			Volume: decimal.NewFromFloat(1000),
			Bid:    decimal.NewFromFloat(39999 + float64(i)),
			Ask:    decimal.NewFromFloat(40001 + float64(i)),
		}
	}
	
	// Aggregate function
	aggregate := func(tickers []*types.Ticker) *types.Ticker {
		if len(tickers) == 0 {
			return nil
		}
		
		// Calculate weighted average price
		totalVolume := decimal.Zero
		weightedPrice := decimal.Zero
		
		for _, t := range tickers {
			totalVolume = totalVolume.Add(t.Volume)
			weightedPrice = weightedPrice.Add(t.Price.Mul(t.Volume))
		}
		
		if totalVolume.IsZero() {
			return nil
		}
		
		return &types.Ticker{
			Symbol: tickers[0].Symbol,
			Price:  weightedPrice.Div(totalVolume),
			Volume: totalVolume,
		}
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_ = aggregate(tickers)
	}
	
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "aggregations/sec")
}

// BenchmarkDepthCalculation tests market depth calculation
func BenchmarkDepthCalculation(b *testing.B) {
	orderBook := &types.OrderBook{
		Symbol: "BTCUSDT",
		Bids:   make([][2]decimal.Decimal, 1000),
		Asks:   make([][2]decimal.Decimal, 1000),
	}
	
	// Fill with realistic order book data
	for i := 0; i < 1000; i++ {
		orderBook.Bids[i] = [2]decimal.Decimal{
			decimal.NewFromFloat(40000 - float64(i)*0.1),
			decimal.NewFromFloat(float64(i+1) * 0.01),
		}
		orderBook.Asks[i] = [2]decimal.Decimal{
			decimal.NewFromFloat(40000.1 + float64(i)*0.1),
			decimal.NewFromFloat(float64(i+1) * 0.01),
		}
	}
	
	// Calculate market depth at different price levels
	calculateDepth := func(ob *types.OrderBook, priceLevel decimal.Decimal) (bidDepth, askDepth decimal.Decimal) {
		bidDepth = decimal.Zero
		askDepth = decimal.Zero
		
		for _, bid := range ob.Bids {
			if bid[0].GreaterThanOrEqual(priceLevel) {
				bidDepth = bidDepth.Add(bid[1])
			}
		}
		
		for _, ask := range ob.Asks {
			if ask[0].LessThanOrEqual(priceLevel) {
				askDepth = askDepth.Add(ask[1])
			}
		}
		
		return bidDepth, askDepth
	}
	
	priceLevel := decimal.NewFromFloat(40000)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_, _ = calculateDepth(orderBook, priceLevel)
	}
	
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "calculations/sec")
}