package backtest

import (
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// TradingStrategy defines the interface for trading strategies
type TradingStrategy interface {
	// Initialize sets up the strategy
	Initialize(config BacktestConfig) error
	
	// GenerateSignals generates trading signals based on market state
	GenerateSignals(currentTime time.Time, market MarketState, portfolio *Portfolio) []*TradingSignal
	
	// Finalize cleans up after backtest
	Finalize()
}

// TradingSignal represents a trading signal
type TradingSignal struct {
	Symbol    string
	Side      types.OrderSide
	OrderType types.OrderType
	Price     decimal.Decimal
	Quantity  decimal.Decimal
	StopLoss  decimal.Decimal
	TakeProfit decimal.Decimal
	Reason    string
	Confidence float64
}

// MarketState holds current market data
type MarketState interface {
	GetPrice(exchange, symbol string) decimal.Decimal
	GetOrderBook(exchange, symbol string) map[string]interface{}
	GetLastTrade(exchange, symbol string) map[string]interface{}
	GetTicker(exchange, symbol string) map[string]interface{}
	UpdateOrderBook(exchange, symbol string, data map[string]interface{})
	UpdateLastTrade(exchange, symbol string, data map[string]interface{})
	UpdateTicker(exchange, symbol string, data map[string]interface{})
}

// marketStateImpl implements MarketState
type marketStateImpl struct {
	orderbooks map[string]map[string]interface{} // key: "exchange:symbol"
	trades     map[string]map[string]interface{}
	tickers    map[string]map[string]interface{}
}

// NewMarketState creates a new market state
func NewMarketState() MarketState {
	return &marketStateImpl{
		orderbooks: make(map[string]map[string]interface{}),
		trades:     make(map[string]map[string]interface{}),
		tickers:    make(map[string]map[string]interface{}),
	}
}

func (ms *marketStateImpl) GetPrice(exchange, symbol string) decimal.Decimal {
	key := exchange + ":" + symbol
	
	// Try ticker first
	if ticker, ok := ms.tickers[key]; ok {
		if lastPrice, ok := ticker["last_price"].(float64); ok {
			return decimal.NewFromFloat(lastPrice)
		}
	}
	
	// Try last trade
	if trade, ok := ms.trades[key]; ok {
		if price, ok := trade["price"].(float64); ok {
			return decimal.NewFromFloat(price)
		}
	}
	
	// Try orderbook mid price
	if orderbook, ok := ms.orderbooks[key]; ok {
		if bids, ok := orderbook["bids"].([]interface{}); ok && len(bids) > 0 {
			if asks, ok := orderbook["asks"].([]interface{}); ok && len(asks) > 0 {
				bidPrice := bids[0].([]interface{})[0].(float64)
				askPrice := asks[0].([]interface{})[0].(float64)
				return decimal.NewFromFloat((bidPrice + askPrice) / 2)
			}
		}
	}
	
	return decimal.Zero
}

func (ms *marketStateImpl) GetOrderBook(exchange, symbol string) map[string]interface{} {
	key := exchange + ":" + symbol
	return ms.orderbooks[key]
}

func (ms *marketStateImpl) GetLastTrade(exchange, symbol string) map[string]interface{} {
	key := exchange + ":" + symbol
	return ms.trades[key]
}

func (ms *marketStateImpl) GetTicker(exchange, symbol string) map[string]interface{} {
	key := exchange + ":" + symbol
	return ms.tickers[key]
}

func (ms *marketStateImpl) UpdateOrderBook(exchange, symbol string, data map[string]interface{}) {
	key := exchange + ":" + symbol
	ms.orderbooks[key] = data
}

func (ms *marketStateImpl) UpdateLastTrade(exchange, symbol string, data map[string]interface{}) {
	key := exchange + ":" + symbol
	ms.trades[key] = data
}

func (ms *marketStateImpl) UpdateTicker(exchange, symbol string, data map[string]interface{}) {
	key := exchange + ":" + symbol
	ms.tickers[key] = data
}

// Example Strategies

// SimpleMovingAverageStrategy is a basic SMA crossover strategy
type SimpleMovingAverageStrategy struct {
	config      BacktestConfig
	shortPeriod int
	longPeriod  int
	priceHistory map[string][]decimal.Decimal
	positions    map[string]bool
}

// NewSimpleMovingAverageStrategy creates a new SMA strategy
func NewSimpleMovingAverageStrategy(shortPeriod, longPeriod int) *SimpleMovingAverageStrategy {
	return &SimpleMovingAverageStrategy{
		shortPeriod:  shortPeriod,
		longPeriod:   longPeriod,
		priceHistory: make(map[string][]decimal.Decimal),
		positions:    make(map[string]bool),
	}
}

func (s *SimpleMovingAverageStrategy) Initialize(config BacktestConfig) error {
	s.config = config
	return nil
}

func (s *SimpleMovingAverageStrategy) GenerateSignals(currentTime time.Time, market MarketState, portfolio *Portfolio) []*TradingSignal {
	var signals []*TradingSignal
	
	symbols := []string{"BTCUSDT", "ETHUSDT"}
	
	for _, symbol := range symbols {
		price := market.GetPrice("binance", symbol)
		if price.IsZero() {
			continue
		}
		
		// Update price history
		if s.priceHistory[symbol] == nil {
			s.priceHistory[symbol] = make([]decimal.Decimal, 0)
		}
		s.priceHistory[symbol] = append(s.priceHistory[symbol], price)
		
		// Keep only needed history
		maxPeriod := s.longPeriod
		if len(s.priceHistory[symbol]) > maxPeriod*2 {
			s.priceHistory[symbol] = s.priceHistory[symbol][len(s.priceHistory[symbol])-maxPeriod*2:]
		}
		
		// Need enough data
		if len(s.priceHistory[symbol]) < s.longPeriod {
			continue
		}
		
		// Calculate SMAs
		shortSMA := s.calculateSMA(s.priceHistory[symbol], s.shortPeriod)
		longSMA := s.calculateSMA(s.priceHistory[symbol], s.longPeriod)
		
		// Generate signals
		hasPosition := s.positions[symbol]
		
		if shortSMA.GreaterThan(longSMA) && !hasPosition {
			// Buy signal
			quantity := s.calculatePositionSize(portfolio, price)
			if quantity.GreaterThan(decimal.Zero) {
				signals = append(signals, &TradingSignal{
					Symbol:    symbol,
					Side:      types.OrderSideBuy,
					OrderType: types.OrderTypeMarket,
					Price:     price,
					Quantity:  quantity,
					Reason:    "SMA crossover - bullish",
					Confidence: 0.7,
				})
				s.positions[symbol] = true
			}
		} else if longSMA.GreaterThan(shortSMA) && hasPosition {
			// Sell signal
			if pos, exists := portfolio.Positions[symbol]; exists {
				signals = append(signals, &TradingSignal{
					Symbol:    symbol,
					Side:      types.OrderSideSell,
					OrderType: types.OrderTypeMarket,
					Price:     price,
					Quantity:  pos.Quantity,
					Reason:    "SMA crossover - bearish",
					Confidence: 0.7,
				})
				s.positions[symbol] = false
			}
		}
	}
	
	return signals
}

func (s *SimpleMovingAverageStrategy) Finalize() {
	// Clean up
	s.priceHistory = nil
	s.positions = nil
}

func (s *SimpleMovingAverageStrategy) calculateSMA(prices []decimal.Decimal, period int) decimal.Decimal {
	if len(prices) < period {
		return decimal.Zero
	}
	
	sum := decimal.Zero
	start := len(prices) - period
	for i := start; i < len(prices); i++ {
		sum = sum.Add(prices[i])
	}
	
	return sum.Div(decimal.NewFromInt(int64(period)))
}

func (s *SimpleMovingAverageStrategy) calculatePositionSize(portfolio *Portfolio, price decimal.Decimal) decimal.Decimal {
	// Use 20% of available cash per position
	availableCash := portfolio.Cash.Mul(decimal.NewFromFloat(0.2))
	
	// Account for fees
	availableCash = availableCash.Div(decimal.NewFromFloat(1).Add(s.config.TradingFees))
	
	// Calculate quantity
	quantity := availableCash.Div(price)
	
	// Round to reasonable precision
	return quantity.Round(4)
}

// MomentumStrategy trades based on price momentum
type MomentumStrategy struct {
	config       BacktestConfig
	lookback     int
	threshold    decimal.Decimal
	priceHistory map[string][]decimal.Decimal
	positions    map[string]*positionInfo
}

type positionInfo struct {
	entryPrice decimal.Decimal
	stopLoss   decimal.Decimal
	takeProfit decimal.Decimal
}

// NewMomentumStrategy creates a new momentum strategy
func NewMomentumStrategy(lookback int, threshold float64) *MomentumStrategy {
	return &MomentumStrategy{
		lookback:     lookback,
		threshold:    decimal.NewFromFloat(threshold),
		priceHistory: make(map[string][]decimal.Decimal),
		positions:    make(map[string]*positionInfo),
	}
}

func (m *MomentumStrategy) Initialize(config BacktestConfig) error {
	m.config = config
	return nil
}

func (m *MomentumStrategy) GenerateSignals(currentTime time.Time, market MarketState, portfolio *Portfolio) []*TradingSignal {
	var signals []*TradingSignal
	
	symbols := []string{"BTCUSDT", "ETHUSDT", "BNBUSDT"}
	
	for _, symbol := range symbols {
		price := market.GetPrice("binance", symbol)
		if price.IsZero() {
			continue
		}
		
		// Update price history
		if m.priceHistory[symbol] == nil {
			m.priceHistory[symbol] = make([]decimal.Decimal, 0)
		}
		m.priceHistory[symbol] = append(m.priceHistory[symbol], price)
		
		// Keep limited history
		if len(m.priceHistory[symbol]) > m.lookback*2 {
			m.priceHistory[symbol] = m.priceHistory[symbol][len(m.priceHistory[symbol])-m.lookback*2:]
		}
		
		// Need enough data
		if len(m.priceHistory[symbol]) < m.lookback {
			continue
		}
		
		// Calculate momentum
		oldPrice := m.priceHistory[symbol][len(m.priceHistory[symbol])-m.lookback]
		momentum := price.Sub(oldPrice).Div(oldPrice)
		
		// Check existing position
		if posInfo, hasPosition := m.positions[symbol]; hasPosition {
			// Check stop loss and take profit
			if price.LessThanOrEqual(posInfo.stopLoss) || price.GreaterThanOrEqual(posInfo.takeProfit) {
				if pos, exists := portfolio.Positions[symbol]; exists {
					reason := "Stop loss hit"
					if price.GreaterThanOrEqual(posInfo.takeProfit) {
						reason = "Take profit hit"
					}
					
					signals = append(signals, &TradingSignal{
						Symbol:    symbol,
						Side:      types.OrderSideSell,
						OrderType: types.OrderTypeMarket,
						Price:     price,
						Quantity:  pos.Quantity,
						Reason:    reason,
						Confidence: 0.9,
					})
					delete(m.positions, symbol)
				}
			}
		} else {
			// Look for entry signals
			if momentum.GreaterThan(m.threshold) {
				// Strong positive momentum
				quantity := m.calculatePositionSize(portfolio, price)
				if quantity.GreaterThan(decimal.Zero) {
					stopLoss := price.Mul(decimal.NewFromFloat(0.98))   // 2% stop loss
					takeProfit := price.Mul(decimal.NewFromFloat(1.05)) // 5% take profit
					
					signals = append(signals, &TradingSignal{
						Symbol:     symbol,
						Side:       types.OrderSideBuy,
						OrderType:  types.OrderTypeMarket,
						Price:      price,
						Quantity:   quantity,
						StopLoss:   stopLoss,
						TakeProfit: takeProfit,
						Reason:     "Strong positive momentum",
						Confidence: 0.8,
					})
					
					m.positions[symbol] = &positionInfo{
						entryPrice: price,
						stopLoss:   stopLoss,
						takeProfit: takeProfit,
					}
				}
			}
		}
	}
	
	return signals
}

func (m *MomentumStrategy) Finalize() {
	m.priceHistory = nil
	m.positions = nil
}

func (m *MomentumStrategy) calculatePositionSize(portfolio *Portfolio, price decimal.Decimal) decimal.Decimal {
	// Risk 2% of portfolio per trade
	riskAmount := portfolio.TotalValue.Mul(decimal.NewFromFloat(0.02))
	
	// Calculate position size based on stop loss distance
	stopLossDistance := price.Mul(decimal.NewFromFloat(0.02)) // 2% stop loss
	quantity := riskAmount.Div(stopLossDistance)
	
	// Limit to 30% of available cash
	maxCash := portfolio.Cash.Mul(decimal.NewFromFloat(0.3))
	maxQuantity := maxCash.Div(price.Mul(decimal.NewFromFloat(1).Add(m.config.TradingFees)))
	
	if quantity.GreaterThan(maxQuantity) {
		quantity = maxQuantity
	}
	
	return quantity.Round(4)
}