package backtest

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FileDataProvider provides market data from files
type FileDataProvider struct {
	config     BacktestConfig
	dataFiles  map[string]string // symbol -> file path
	readers    map[string]*bufio.Reader
	files      map[string]*os.File
	buffer     []*MarketDataPoint
	bufferIdx  int
	currentIdx int
	mu         sync.Mutex
	eof        bool
}

// NewFileDataProvider creates a new file-based data provider
func NewFileDataProvider() *FileDataProvider {
	return &FileDataProvider{
		dataFiles: make(map[string]string),
		readers:   make(map[string]*bufio.Reader),
		files:     make(map[string]*os.File),
		buffer:    make([]*MarketDataPoint, 0, 1000),
	}
}

// Initialize initializes the data provider
func (p *FileDataProvider) Initialize(config BacktestConfig) error {
	p.config = config

	// Find data files for each symbol
	for _, symbol := range config.Symbols {
		pattern := filepath.Join(config.DataPath, fmt.Sprintf("*%s*.csv", symbol))
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return fmt.Errorf("failed to find data files for %s: %w", symbol, err)
		}

		if len(matches) == 0 {
			// Try JSONL format
			pattern = filepath.Join(config.DataPath, fmt.Sprintf("*%s*.jsonl", symbol))
			matches, err = filepath.Glob(pattern)
			if err != nil {
				return fmt.Errorf("failed to find data files for %s: %w", symbol, err)
			}
		}

		if len(matches) == 0 {
			return fmt.Errorf("no data files found for symbol %s", symbol)
		}

		// Use the first matching file
		p.dataFiles[symbol] = matches[0]
	}

	// Open all files
	for symbol, filePath := range p.dataFiles {
		file, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open data file %s: %w", filePath, err)
		}

		p.files[symbol] = file
		p.readers[symbol] = bufio.NewReader(file)
	}

	// Load initial buffer
	if err := p.loadBuffer(); err != nil {
		return err
	}

	return nil
}

// Next returns the next data point
func (p *FileDataProvider) Next() (*MarketDataPoint, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.currentIdx >= len(p.buffer) {
		if err := p.loadBuffer(); err != nil {
			return nil, err
		}
		if len(p.buffer) == 0 {
			p.eof = true
			return nil, io.EOF
		}
	}

	if p.currentIdx >= len(p.buffer) {
		p.eof = true
		return nil, io.EOF
	}

	data := p.buffer[p.currentIdx]
	p.currentIdx++
	return data, nil
}

// HasNext checks if more data is available
func (p *FileDataProvider) HasNext() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return !p.eof || p.currentIdx < len(p.buffer)
}

// Reset resets the provider to the beginning
func (p *FileDataProvider) Reset() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Reset all file readers
	for symbol, file := range p.files {
		if _, err := file.Seek(0, 0); err != nil {
			return fmt.Errorf("failed to reset file %s: %w", symbol, err)
		}
		p.readers[symbol] = bufio.NewReader(file)
	}

	// Clear buffer and reset indices
	p.buffer = p.buffer[:0]
	p.currentIdx = 0
	p.bufferIdx = 0
	p.eof = false

	// Reload buffer
	return p.loadBuffer()
}

// GetDataAt returns data at a specific timestamp
func (p *FileDataProvider) GetDataAt(timestamp time.Time) ([]*MarketDataPoint, error) {
	// This is a simplified implementation
	// In production, you might want to maintain an index for faster lookups
	result := make([]*MarketDataPoint, 0)

	for i := 0; i < len(p.buffer); i++ {
		if p.buffer[i].Timestamp.Equal(timestamp) {
			result = append(result, p.buffer[i])
		}
	}

	return result, nil
}

// loadBuffer loads the next batch of data into buffer
func (p *FileDataProvider) loadBuffer() error {
	p.buffer = p.buffer[:0]
	dataMap := make(map[time.Time][]*MarketDataPoint)

	// Read from each symbol's file
	for symbol, reader := range p.readers {
		filePath := p.dataFiles[symbol]
		
		// Read up to 100 lines per symbol
		for i := 0; i < 100; i++ {
			line, err := reader.ReadString('\n')
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("error reading file %s: %w", filePath, err)
			}

			// Parse based on file extension
			var data *MarketDataPoint
			if strings.HasSuffix(filePath, ".csv") {
				data, err = p.parseCSVLine(line, symbol)
			} else if strings.HasSuffix(filePath, ".jsonl") {
				data, err = p.parseJSONLine(line, symbol)
			}

			if err != nil {
				continue // Skip invalid lines
			}

			// Filter by time range
			if data.Timestamp.Before(p.config.StartTime) {
				continue
			}
			if data.Timestamp.After(p.config.EndTime) {
				continue
			}

			// Group by timestamp
			dataMap[data.Timestamp] = append(dataMap[data.Timestamp], data)
		}
	}

	// Sort by timestamp
	timestamps := make([]time.Time, 0, len(dataMap))
	for ts := range dataMap {
		timestamps = append(timestamps, ts)
	}
	sort.Slice(timestamps, func(i, j int) bool {
		return timestamps[i].Before(timestamps[j])
	})

	// Flatten into buffer
	for _, ts := range timestamps {
		p.buffer = append(p.buffer, dataMap[ts]...)
	}

	p.currentIdx = 0
	return nil
}

// parseCSVLine parses a CSV line into MarketDataPoint
func (p *FileDataProvider) parseCSVLine(line string, symbol string) (*MarketDataPoint, error) {
	// Expected format: timestamp,bid,ask,bidSize,askSize,last,volume
	reader := csv.NewReader(strings.NewReader(strings.TrimSpace(line)))
	record, err := reader.Read()
	if err != nil {
		return nil, err
	}

	if len(record) < 7 {
		return nil, fmt.Errorf("insufficient fields in CSV line")
	}

	timestamp, err := time.Parse("2006-01-02 15:04:05", record[0])
	if err != nil {
		// Try Unix timestamp
		unixTs, err2 := strconv.ParseInt(record[0], 10, 64)
		if err2 != nil {
			return nil, fmt.Errorf("failed to parse timestamp: %w", err)
		}
		timestamp = time.Unix(unixTs, 0)
	}

	bid, _ := strconv.ParseFloat(record[1], 64)
	ask, _ := strconv.ParseFloat(record[2], 64)
	bidSize, _ := strconv.ParseFloat(record[3], 64)
	askSize, _ := strconv.ParseFloat(record[4], 64)
	last, _ := strconv.ParseFloat(record[5], 64)
	volume, _ := strconv.ParseFloat(record[6], 64)

	return &MarketDataPoint{
		Timestamp: timestamp,
		Symbol:    symbol,
		Exchange:  "binance", // Default, could be parsed from filename
		Bid:       bid,
		Ask:       ask,
		BidSize:   bidSize,
		AskSize:   askSize,
		Last:      last,
		Volume:    volume,
	}, nil
}

// parseJSONLine parses a JSON line into MarketDataPoint
func (p *FileDataProvider) parseJSONLine(line string, symbol string) (*MarketDataPoint, error) {
	var data struct {
		Timestamp int64   `json:"timestamp"`
		Bid       float64 `json:"bid"`
		Ask       float64 `json:"ask"`
		BidSize   float64 `json:"bid_size"`
		AskSize   float64 `json:"ask_size"`
		Last      float64 `json:"last"`
		Volume    float64 `json:"volume"`
		Exchange  string  `json:"exchange"`
	}

	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &data); err != nil {
		return nil, err
	}

	exchange := data.Exchange
	if exchange == "" {
		exchange = "binance"
	}

	return &MarketDataPoint{
		Timestamp: time.Unix(0, data.Timestamp*int64(time.Millisecond)),
		Symbol:    symbol,
		Exchange:  exchange,
		Bid:       data.Bid,
		Ask:       data.Ask,
		BidSize:   data.BidSize,
		AskSize:   data.AskSize,
		Last:      data.Last,
		Volume:    data.Volume,
	}, nil
}

// Close closes all open files
func (p *FileDataProvider) Close() error {
	for _, file := range p.files {
		if err := file.Close(); err != nil {
			return err
		}
	}
	return nil
}

// SyntheticDataProvider generates synthetic market data for testing
type SyntheticDataProvider struct {
	config       BacktestConfig
	currentTime  time.Time
	priceGen     *PriceGenerator
	currentPrices map[string]float64
	mu           sync.Mutex
}

// NewSyntheticDataProvider creates a synthetic data provider
func NewSyntheticDataProvider() *SyntheticDataProvider {
	return &SyntheticDataProvider{
		currentPrices: make(map[string]float64),
		priceGen:      NewPriceGenerator(),
	}
}

// Initialize initializes the synthetic data provider
func (p *SyntheticDataProvider) Initialize(config BacktestConfig) error {
	p.config = config
	p.currentTime = config.StartTime

	// Initialize starting prices
	for _, symbol := range config.Symbols {
		// Default starting prices
		switch symbol {
		case "BTCUSDT":
			p.currentPrices[symbol] = 50000
		case "ETHUSDT":
			p.currentPrices[symbol] = 3000
		default:
			p.currentPrices[symbol] = 100
		}
	}

	return nil
}

// Next generates the next synthetic data point
func (p *SyntheticDataProvider) Next() (*MarketDataPoint, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.currentTime.After(p.config.EndTime) {
		return nil, io.EOF
	}

	// Generate data for a random symbol
	symbolIdx := int(p.currentTime.Unix()) % len(p.config.Symbols)
	symbol := p.config.Symbols[symbolIdx]

	// Generate new price
	currentPrice := p.currentPrices[symbol]
	newPrice := p.priceGen.NextPrice(currentPrice)
	p.currentPrices[symbol] = newPrice

	// Generate spread
	spread := newPrice * 0.0001 // 0.01% spread
	
	data := &MarketDataPoint{
		Timestamp: p.currentTime,
		Symbol:    symbol,
		Exchange:  "synthetic",
		Bid:       newPrice - spread/2,
		Ask:       newPrice + spread/2,
		BidSize:   100.0,
		AskSize:   100.0,
		Last:      newPrice,
		Volume:    1000.0,
	}

	// Advance time
	p.currentTime = p.currentTime.Add(p.config.TickInterval)

	return data, nil
}

// HasNext checks if more data can be generated
func (p *SyntheticDataProvider) HasNext() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.currentTime.Before(p.config.EndTime) || p.currentTime.Equal(p.config.EndTime)
}

// Reset resets the provider
func (p *SyntheticDataProvider) Reset() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.currentTime = p.config.StartTime
	return p.Initialize(p.config)
}

// GetDataAt returns synthetic data at a specific time
func (p *SyntheticDataProvider) GetDataAt(timestamp time.Time) ([]*MarketDataPoint, error) {
	// Generate data for all symbols at the requested time
	result := make([]*MarketDataPoint, 0, len(p.config.Symbols))

	for _, symbol := range p.config.Symbols {
		price := p.currentPrices[symbol]
		spread := price * 0.0001

		result = append(result, &MarketDataPoint{
			Timestamp: timestamp,
			Symbol:    symbol,
			Exchange:  "synthetic",
			Bid:       price - spread/2,
			Ask:       price + spread/2,
			BidSize:   100.0,
			AskSize:   100.0,
			Last:      price,
			Volume:    1000.0,
		})
	}

	return result, nil
}

// PriceGenerator generates realistic price movements
type PriceGenerator struct {
	volatility float64
	drift      float64
	random     *RandomWalk
}

// NewPriceGenerator creates a new price generator
func NewPriceGenerator() *PriceGenerator {
	return &PriceGenerator{
		volatility: 0.02,  // 2% daily volatility
		drift:      0.0001, // Slight upward drift
		random:     NewRandomWalk(),
	}
}

// NextPrice generates the next price
func (g *PriceGenerator) NextPrice(currentPrice float64) float64 {
	// Geometric Brownian Motion
	dt := 1.0 / (24 * 60 * 60) // 1 second in days
	randomShock := g.random.Next()
	
	return currentPrice * (1 + g.drift*dt + g.volatility*randomShock*dt)
}

// RandomWalk generates random walk values
type RandomWalk struct {
	value float64
}

// NewRandomWalk creates a new random walk generator
func NewRandomWalk() *RandomWalk {
	return &RandomWalk{}
}

// Next returns the next random value
func (r *RandomWalk) Next() float64 {
	// Simple random walk between -1 and 1
	// In production, use proper random number generator
	r.value += (float64(time.Now().UnixNano()%200) - 100) / 100.0
	if r.value > 1 {
		r.value = 1
	} else if r.value < -1 {
		r.value = -1
	}
	return r.value
}