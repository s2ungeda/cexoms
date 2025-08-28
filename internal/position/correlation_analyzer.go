package position

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

// PositionCorrelation represents correlation between two positions
type PositionCorrelation struct {
	Symbol1     string    `json:"symbol1"`
	Symbol2     string    `json:"symbol2"`
	Correlation float64   `json:"correlation"` // -1 to 1
	Covariance  float64   `json:"covariance"`
	SampleSize  int       `json:"sample_size"`
	Period      string    `json:"period"` // "1m", "5m", "1h", etc.
	LastUpdate  time.Time `json:"last_update"`
}

// AccountCorrelation represents correlation between accounts
type AccountCorrelation struct {
	Account1         string    `json:"account1"`
	Account2         string    `json:"account2"`
	PositionOverlap  float64   `json:"position_overlap"`  // % of overlapping positions
	DirectionAlign   float64   `json:"direction_align"`   // % of same direction positions
	PnLCorrelation   float64   `json:"pnl_correlation"`   // Correlation of P&L
	RiskCorrelation  float64   `json:"risk_correlation"`  // Correlation of risk exposure
	LastUpdate       time.Time `json:"last_update"`
}

// CorrelationMatrix holds correlations between all positions
type CorrelationMatrix struct {
	Symbols      []string                         `json:"symbols"`
	Matrix       [][]float64                      `json:"matrix"`
	LastUpdate   time.Time                        `json:"last_update"`
}

// PriceData holds historical price data for correlation calculation
type PriceData struct {
	Symbol    string
	Prices    []float64
	Returns   []float64
	Timestamp []time.Time
}

// CorrelationAnalyzer analyzes correlations between positions and accounts
type CorrelationAnalyzer struct {
	positionMgr *IntegratedPositionManager
	
	// Historical data storage
	priceHistory sync.Map // symbol -> *PriceData
	pnlHistory   sync.Map // accountID -> []float64
	
	// Correlation caches
	positionCorrelations sync.Map // "symbol1:symbol2" -> *PositionCorrelation
	accountCorrelations  sync.Map // "account1:account2" -> *AccountCorrelation
	
	// Configuration
	historySize   int           // Number of data points to keep
	updatePeriod  time.Duration // How often to update correlations
	minSamples    int           // Minimum samples for correlation calculation
	
	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	
	// Callbacks
	onHighCorrelation  func(symbol1, symbol2 string, correlation float64)
	onRiskConcentration func(accounts []string, reason string)
}

// NewCorrelationAnalyzer creates a new correlation analyzer
func NewCorrelationAnalyzer(positionMgr *IntegratedPositionManager) *CorrelationAnalyzer {
	ctx, cancel := context.WithCancel(context.Background())
	
	ca := &CorrelationAnalyzer{
		positionMgr:  positionMgr,
		historySize:  1000,      // Keep last 1000 data points
		updatePeriod: 1 * time.Minute,
		minSamples:   30,
		ctx:          ctx,
		cancel:       cancel,
	}
	
	// Start correlation updater
	ca.wg.Add(1)
	go ca.correlationUpdater()
	
	return ca
}

// AddPriceUpdate adds a new price data point
func (ca *CorrelationAnalyzer) AddPriceUpdate(symbol string, price float64, timestamp time.Time) {
	// Get or create price data
	var priceData *PriceData
	if pd, ok := ca.priceHistory.Load(symbol); ok {
		priceData = pd.(*PriceData)
	} else {
		priceData = &PriceData{
			Symbol:    symbol,
			Prices:    make([]float64, 0, ca.historySize),
			Returns:   make([]float64, 0, ca.historySize),
			Timestamp: make([]time.Time, 0, ca.historySize),
		}
		ca.priceHistory.Store(symbol, priceData)
	}
	
	// Add new price
	priceData.Prices = append(priceData.Prices, price)
	priceData.Timestamp = append(priceData.Timestamp, timestamp)
	
	// Calculate return if we have previous price
	if len(priceData.Prices) > 1 {
		prevPrice := priceData.Prices[len(priceData.Prices)-2]
		if prevPrice > 0 {
			returnVal := (price - prevPrice) / prevPrice
			priceData.Returns = append(priceData.Returns, returnVal)
		}
	}
	
	// Trim to history size
	if len(priceData.Prices) > ca.historySize {
		priceData.Prices = priceData.Prices[1:]
		priceData.Timestamp = priceData.Timestamp[1:]
	}
	if len(priceData.Returns) > ca.historySize-1 {
		priceData.Returns = priceData.Returns[1:]
	}
}

// CalculatePositionCorrelation calculates correlation between two positions
func (ca *CorrelationAnalyzer) CalculatePositionCorrelation(symbol1, symbol2 string) (*PositionCorrelation, error) {
	// Get price data
	pd1, ok1 := ca.priceHistory.Load(symbol1)
	pd2, ok2 := ca.priceHistory.Load(symbol2)
	
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("insufficient price data for %s or %s", symbol1, symbol2)
	}
	
	data1 := pd1.(*PriceData)
	data2 := pd2.(*PriceData)
	
	// Need sufficient overlapping data
	if len(data1.Returns) < ca.minSamples || len(data2.Returns) < ca.minSamples {
		return nil, fmt.Errorf("insufficient data points: %s has %d, %s has %d (min: %d)",
			symbol1, len(data1.Returns), symbol2, len(data2.Returns), ca.minSamples)
	}
	
	// Calculate correlation using returns
	correlation, covariance := ca.calculateCorrelation(data1.Returns, data2.Returns)
	
	result := &PositionCorrelation{
		Symbol1:     symbol1,
		Symbol2:     symbol2,
		Correlation: correlation,
		Covariance:  covariance,
		SampleSize:  len(data1.Returns),
		Period:      "1m", // Assuming minute data
		LastUpdate:  time.Now(),
	}
	
	// Cache result
	key := ca.getCorrelationKey(symbol1, symbol2)
	ca.positionCorrelations.Store(key, result)
	
	// Trigger callback for high correlation
	if ca.onHighCorrelation != nil && math.Abs(correlation) > 0.8 {
		ca.onHighCorrelation(symbol1, symbol2, correlation)
	}
	
	return result, nil
}

// CalculateAccountCorrelation calculates correlation between two accounts
func (ca *CorrelationAnalyzer) CalculateAccountCorrelation(account1, account2 string) (*AccountCorrelation, error) {
	// Get account positions
	acc1, err := ca.positionMgr.GetAccountPositions(account1)
	if err != nil {
		return nil, fmt.Errorf("failed to get positions for account %s: %v", account1, err)
	}
	
	acc2, err := ca.positionMgr.GetAccountPositions(account2)
	if err != nil {
		return nil, fmt.Errorf("failed to get positions for account %s: %v", account2, err)
	}
	
	// Calculate position overlap
	overlap := ca.calculatePositionOverlap(acc1, acc2)
	
	// Calculate direction alignment
	dirAlign := ca.calculateDirectionAlignment(acc1, acc2)
	
	// Calculate P&L correlation if we have history
	pnlCorr := 0.0
	if pnl1, ok1 := ca.pnlHistory.Load(account1); ok1 {
		if pnl2, ok2 := ca.pnlHistory.Load(account2); ok2 {
			pnlCorr, _ = ca.calculateCorrelation(pnl1.([]float64), pnl2.([]float64))
		}
	}
	
	result := &AccountCorrelation{
		Account1:        account1,
		Account2:        account2,
		PositionOverlap: overlap,
		DirectionAlign:  dirAlign,
		PnLCorrelation:  pnlCorr,
		LastUpdate:      time.Now(),
	}
	
	// Cache result
	key := ca.getCorrelationKey(account1, account2)
	ca.accountCorrelations.Store(key, result)
	
	// Check for risk concentration
	if ca.onRiskConcentration != nil {
		if overlap > 0.7 && dirAlign > 0.8 {
			reason := fmt.Sprintf("High position overlap (%.1f%%) and direction alignment (%.1f%%) between accounts",
				overlap*100, dirAlign*100)
			ca.onRiskConcentration([]string{account1, account2}, reason)
		}
	}
	
	return result, nil
}

// GetCorrelationMatrix returns correlation matrix for all active positions
func (ca *CorrelationAnalyzer) GetCorrelationMatrix() (*CorrelationMatrix, error) {
	// Get all symbols with positions
	symbolMap := make(map[string]bool)
	ca.positionMgr.globalPositions.Range(func(key, value interface{}) bool {
		symbol := key.(string)
		symbolMap[symbol] = true
		return true
	})
	
	// Convert to slice
	symbols := make([]string, 0, len(symbolMap))
	for symbol := range symbolMap {
		symbols = append(symbols, symbol)
	}
	
	// Create correlation matrix
	n := len(symbols)
	matrix := make([][]float64, n)
	for i := range matrix {
		matrix[i] = make([]float64, n)
		matrix[i][i] = 1.0 // Self-correlation is 1
	}
	
	// Calculate correlations
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			corr, err := ca.CalculatePositionCorrelation(symbols[i], symbols[j])
			if err == nil {
				matrix[i][j] = corr.Correlation
				matrix[j][i] = corr.Correlation
			}
		}
	}
	
	return &CorrelationMatrix{
		Symbols:    symbols,
		Matrix:     matrix,
		LastUpdate: time.Now(),
	}, nil
}

// DetectRiskClusters identifies groups of highly correlated positions
func (ca *CorrelationAnalyzer) DetectRiskClusters(threshold float64) [][]string {
	matrix, err := ca.GetCorrelationMatrix()
	if err != nil || len(matrix.Symbols) == 0 {
		return nil
	}
	
	// Simple clustering: group symbols with correlation > threshold
	clusters := make([][]string, 0)
	visited := make(map[int]bool)
	
	for i := 0; i < len(matrix.Symbols); i++ {
		if visited[i] {
			continue
		}
		
		cluster := []string{matrix.Symbols[i]}
		visited[i] = true
		
		// Find all symbols correlated with this one
		for j := i + 1; j < len(matrix.Symbols); j++ {
			if !visited[j] && math.Abs(matrix.Matrix[i][j]) > threshold {
				cluster = append(cluster, matrix.Symbols[j])
				visited[j] = true
			}
		}
		
		if len(cluster) > 1 {
			clusters = append(clusters, cluster)
		}
	}
	
	return clusters
}

// calculateCorrelation calculates Pearson correlation coefficient
func (ca *CorrelationAnalyzer) calculateCorrelation(x, y []float64) (correlation, covariance float64) {
	n := len(x)
	if n != len(y) || n < 2 {
		return 0, 0
	}
	
	// Calculate means
	var sumX, sumY float64
	for i := 0; i < n; i++ {
		sumX += x[i]
		sumY += y[i]
	}
	meanX := sumX / float64(n)
	meanY := sumY / float64(n)
	
	// Calculate covariance and standard deviations
	var cov, varX, varY float64
	for i := 0; i < n; i++ {
		dx := x[i] - meanX
		dy := y[i] - meanY
		cov += dx * dy
		varX += dx * dx
		varY += dy * dy
	}
	
	cov /= float64(n - 1)
	stdX := math.Sqrt(varX / float64(n-1))
	stdY := math.Sqrt(varY / float64(n-1))
	
	// Calculate correlation
	if stdX > 0 && stdY > 0 {
		correlation = cov / (stdX * stdY)
	}
	
	return correlation, cov
}

// calculatePositionOverlap calculates percentage of overlapping positions
func (ca *CorrelationAnalyzer) calculatePositionOverlap(acc1, acc2 *AccountPosition) float64 {
	if len(acc1.Positions) == 0 || len(acc2.Positions) == 0 {
		return 0
	}
	
	overlap := 0
	for symbol := range acc1.Positions {
		if _, exists := acc2.Positions[symbol]; exists {
			overlap++
		}
	}
	
	// Overlap percentage based on the smaller account
	minPositions := len(acc1.Positions)
	if len(acc2.Positions) < minPositions {
		minPositions = len(acc2.Positions)
	}
	
	return float64(overlap) / float64(minPositions)
}

// calculateDirectionAlignment calculates percentage of same-direction positions
func (ca *CorrelationAnalyzer) calculateDirectionAlignment(acc1, acc2 *AccountPosition) float64 {
	if len(acc1.Positions) == 0 || len(acc2.Positions) == 0 {
		return 0
	}
	
	sameDirection := 0
	totalCommon := 0
	
	for symbol, pos1 := range acc1.Positions {
		if pos2, exists := acc2.Positions[symbol]; exists {
			totalCommon++
			// Same direction if both long or both short
			if (pos1.Amount.IsPositive() && pos2.Amount.IsPositive()) ||
			   (pos1.Amount.IsNegative() && pos2.Amount.IsNegative()) {
				sameDirection++
			}
		}
	}
	
	if totalCommon == 0 {
		return 0
	}
	
	return float64(sameDirection) / float64(totalCommon)
}

// getCorrelationKey creates a consistent key for correlation pairs
func (ca *CorrelationAnalyzer) getCorrelationKey(item1, item2 string) string {
	if item1 < item2 {
		return fmt.Sprintf("%s:%s", item1, item2)
	}
	return fmt.Sprintf("%s:%s", item2, item1)
}

// correlationUpdater periodically updates correlations
func (ca *CorrelationAnalyzer) correlationUpdater() {
	defer ca.wg.Done()
	
	ticker := time.NewTicker(ca.updatePeriod)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			ca.updateAllCorrelations()
			
		case <-ca.ctx.Done():
			return
		}
	}
}

// updateAllCorrelations updates all cached correlations
func (ca *CorrelationAnalyzer) updateAllCorrelations() {
	// Update position correlations
	ca.positionCorrelations.Range(func(key, value interface{}) bool {
		corr := value.(*PositionCorrelation)
		// Recalculate
		ca.CalculatePositionCorrelation(corr.Symbol1, corr.Symbol2)
		return true
	})
	
	// Update account correlations
	ca.accountCorrelations.Range(func(key, value interface{}) bool {
		corr := value.(*AccountCorrelation)
		// Recalculate
		ca.CalculateAccountCorrelation(corr.Account1, corr.Account2)
		return true
	})
}

// SetHighCorrelationCallback sets callback for high correlation events
func (ca *CorrelationAnalyzer) SetHighCorrelationCallback(callback func(symbol1, symbol2 string, correlation float64)) {
	ca.onHighCorrelation = callback
}

// SetRiskConcentrationCallback sets callback for risk concentration events
func (ca *CorrelationAnalyzer) SetRiskConcentrationCallback(callback func(accounts []string, reason string)) {
	ca.onRiskConcentration = callback
}

// Stop stops the correlation analyzer
func (ca *CorrelationAnalyzer) Stop() {
	ca.cancel()
	ca.wg.Wait()
}