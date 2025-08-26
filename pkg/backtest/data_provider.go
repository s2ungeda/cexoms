package backtest

import (
	"encoding/csv"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
	
	"github.com/mExOms/pkg/types"
)

// CSVDataProvider provides historical data from CSV files
type CSVDataProvider struct {
	// Configuration
	dataDir      string
	symbol       string
	startTime    time.Time
	endTime      time.Time
	
	// Data storage
	ticks        []TickData
	candles      []CandleData
	currentIndex int
	
	// Order book simulation
	orderBook    *SimulatedOrderBook
}

// TickData represents historical tick data
type TickData struct {
	Timestamp time.Time
	Symbol    string
	Price     decimal.Decimal
	Volume    decimal.Decimal
	Side      string // "buy" or "sell"
}

// CandleData represents historical candle data
type CandleData struct {
	Timestamp time.Time
	Symbol    string
	Open      decimal.Decimal
	High      decimal.Decimal
	Low       decimal.Decimal
	Close     decimal.Decimal
	Volume    decimal.Decimal
}

// NewCSVDataProvider creates a new CSV data provider
func NewCSVDataProvider(dataDir, symbol string, startTime, endTime time.Time) *CSVDataProvider {
	return &CSVDataProvider{
		dataDir:   dataDir,
		symbol:    symbol,
		startTime: startTime,
		endTime:   endTime,
		orderBook: NewSimulatedOrderBook(20), // 20 bps default spread
	}
}

// LoadData loads historical data from CSV files
func (p *CSVDataProvider) LoadData() error {
	// Try to load tick data
	tickFile := filepath.Join(p.dataDir, fmt.Sprintf("%s_ticks.csv", p.symbol))
	if err := p.loadTickData(tickFile); err != nil {
		// Try candle data if tick data not available
		candleFile := filepath.Join(p.dataDir, fmt.Sprintf("%s_1m.csv", p.symbol))
		if err := p.loadCandleData(candleFile); err != nil {
			return fmt.Errorf("failed to load data: %w", err)
		}
		
		// Convert candles to ticks
		p.convertCandlesToTicks()
	}
	
	// Filter data by time range
	p.filterByTimeRange()
	
	// Sort by timestamp
	sort.Slice(p.ticks, func(i, j int) bool {
		return p.ticks[i].Timestamp.Before(p.ticks[j].Timestamp)
	})
	
	return nil
}

// loadTickData loads tick data from CSV
func (p *CSVDataProvider) loadTickData(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	
	reader := csv.NewReader(file)
	
	// Skip header
	if _, err := reader.Read(); err != nil {
		return err
	}
	
	p.ticks = make([]TickData, 0)
	
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		
		// Parse CSV row: timestamp, symbol, price, volume, side
		if len(record) < 5 {
			continue
		}
		
		timestamp, err := time.Parse("2006-01-02 15:04:05", record[0])
		if err != nil {
			continue
		}
		
		price, err := decimal.NewFromString(record[2])
		if err != nil {
			continue
		}
		
		volume, err := decimal.NewFromString(record[3])
		if err != nil {
			continue
		}
		
		tick := TickData{
			Timestamp: timestamp,
			Symbol:    record[1],
			Price:     price,
			Volume:    volume,
			Side:      record[4],
		}
		
		p.ticks = append(p.ticks, tick)
	}
	
	return nil
}

// loadCandleData loads candle data from CSV
func (p *CSVDataProvider) loadCandleData(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	
	reader := csv.NewReader(file)
	
	// Skip header
	if _, err := reader.Read(); err != nil {
		return err
	}
	
	p.candles = make([]CandleData, 0)
	
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		
		// Parse CSV row: timestamp, open, high, low, close, volume
		if len(record) < 6 {
			continue
		}
		
		timestamp, err := time.Parse("2006-01-02 15:04:05", record[0])
		if err != nil {
			continue
		}
		
		open, _ := decimal.NewFromString(record[1])
		high, _ := decimal.NewFromString(record[2])
		low, _ := decimal.NewFromString(record[3])
		close, _ := decimal.NewFromString(record[4])
		volume, _ := decimal.NewFromString(record[5])
		
		candle := CandleData{
			Timestamp: timestamp,
			Symbol:    p.symbol,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close,
			Volume:    volume,
		}
		
		p.candles = append(p.candles, candle)
	}
	
	return nil
}

// convertCandlesToTicks converts candle data to tick data
func (p *CSVDataProvider) convertCandlesToTicks() {
	p.ticks = make([]TickData, 0, len(p.candles)*4)
	
	for _, candle := range p.candles {
		// Create ticks for OHLC
		// Open
		p.ticks = append(p.ticks, TickData{
			Timestamp: candle.Timestamp,
			Symbol:    candle.Symbol,
			Price:     candle.Open,
			Volume:    candle.Volume.Div(decimal.NewFromInt(4)),
			Side:      "buy",
		})
		
		// High (if different from open)
		if !candle.High.Equal(candle.Open) {
			p.ticks = append(p.ticks, TickData{
				Timestamp: candle.Timestamp.Add(15 * time.Second),
				Symbol:    candle.Symbol,
				Price:     candle.High,
				Volume:    candle.Volume.Div(decimal.NewFromInt(4)),
				Side:      "buy",
			})
		}
		
		// Low (if different from open and high)
		if !candle.Low.Equal(candle.Open) && !candle.Low.Equal(candle.High) {
			p.ticks = append(p.ticks, TickData{
				Timestamp: candle.Timestamp.Add(30 * time.Second),
				Symbol:    candle.Symbol,
				Price:     candle.Low,
				Volume:    candle.Volume.Div(decimal.NewFromInt(4)),
				Side:      "sell",
			})
		}
		
		// Close (if different from others)
		if !candle.Close.Equal(candle.Open) && !candle.Close.Equal(candle.High) && !candle.Close.Equal(candle.Low) {
			p.ticks = append(p.ticks, TickData{
				Timestamp: candle.Timestamp.Add(45 * time.Second),
				Symbol:    candle.Symbol,
				Price:     candle.Close,
				Volume:    candle.Volume.Div(decimal.NewFromInt(4)),
				Side:      "buy",
			})
		}
	}
}

// filterByTimeRange filters data to the specified time range
func (p *CSVDataProvider) filterByTimeRange() {
	filtered := make([]TickData, 0)
	
	for _, tick := range p.ticks {
		if tick.Timestamp.After(p.startTime) && tick.Timestamp.Before(p.endTime) {
			filtered = append(filtered, tick)
		}
	}
	
	p.ticks = filtered
	p.currentIndex = 0
}

// GetNextTick returns the next tick data
func (p *CSVDataProvider) GetNextTick() (*types.Tick, error) {
	if p.currentIndex >= len(p.ticks) {
		return nil, io.EOF
	}
	
	tickData := p.ticks[p.currentIndex]
	p.currentIndex++
	
	// Update order book simulation
	p.orderBook.UpdatePrice(tickData.Symbol, tickData.Price)
	
	tick := &types.Tick{
		Symbol:   tickData.Symbol,
		Price:    tickData.Price,
		Quantity: tickData.Volume,
		Time:     tickData.Timestamp,
	}
	
	return tick, nil
}

// GetNextCandle returns the next candle data
func (p *CSVDataProvider) GetNextCandle() (*types.Kline, error) {
	// For now, we don't support direct candle iteration
	return nil, fmt.Errorf("candle iteration not implemented")
}

// GetOrderBook returns order book snapshot at current time
func (p *CSVDataProvider) GetOrderBook(symbol string) (*types.OrderBook, error) {
	return p.orderBook.GetOrderBook(symbol, "backtest"), nil
}

// HasMoreData checks if more data is available
func (p *CSVDataProvider) HasMoreData() bool {
	return p.currentIndex < len(p.ticks)
}

// Reset resets the data provider
func (p *CSVDataProvider) Reset() error {
	p.currentIndex = 0
	return nil
}

// GenerateSyntheticData generates synthetic market data for testing
type SyntheticDataProvider struct {
	symbol       string
	startTime    time.Time
	endTime      time.Time
	currentTime  time.Time
	basePrice    decimal.Decimal
	volatility   float64
	tickInterval time.Duration
	orderBook    *SimulatedOrderBook
	random       *rand.Rand
}

// NewSyntheticDataProvider creates a synthetic data provider
func NewSyntheticDataProvider(symbol string, startTime, endTime time.Time, basePrice decimal.Decimal, volatility float64) *SyntheticDataProvider {
	return &SyntheticDataProvider{
		symbol:       symbol,
		startTime:    startTime,
		endTime:      endTime,
		currentTime:  startTime,
		basePrice:    basePrice,
		volatility:   volatility,
		tickInterval: time.Second,
		orderBook:    NewSimulatedOrderBook(20),
		random:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// GetNextTick generates the next synthetic tick
func (p *SyntheticDataProvider) GetNextTick() (*types.Tick, error) {
	if p.currentTime.After(p.endTime) {
		return nil, io.EOF
	}
	
	// Generate price using random walk
	priceChange := p.random.NormFloat64() * p.volatility
	priceMultiplier := 1.0 + priceChange/100
	newPrice := p.basePrice.Mul(decimal.NewFromFloat(priceMultiplier))
	
	// Update base price for next tick
	p.basePrice = newPrice
	
	// Update order book
	p.orderBook.UpdatePrice(p.symbol, newPrice)
	
	// Generate volume (random between 0.1 and 10)
	volume := decimal.NewFromFloat(0.1 + p.random.Float64()*9.9)
	
	tick := &types.Tick{
		Symbol:   p.symbol,
		Price:    newPrice,
		Quantity: volume,
		Time:     p.currentTime,
	}
	
	// Advance time
	p.currentTime = p.currentTime.Add(p.tickInterval)
	
	return tick, nil
}

// GetNextCandle is not implemented for synthetic provider
func (p *SyntheticDataProvider) GetNextCandle() (*types.Kline, error) {
	return nil, fmt.Errorf("candle data not available for synthetic provider")
}

// GetOrderBook returns simulated order book
func (p *SyntheticDataProvider) GetOrderBook(symbol string) (*types.OrderBook, error) {
	return p.orderBook.GetOrderBook(symbol, "synthetic"), nil
}

// HasMoreData checks if more data can be generated
func (p *SyntheticDataProvider) HasMoreData() bool {
	return p.currentTime.Before(p.endTime)
}

// Reset resets the synthetic data provider
func (p *SyntheticDataProvider) Reset() error {
	p.currentTime = p.startTime
	p.basePrice = p.basePrice // Reset to original base price
	return nil
}