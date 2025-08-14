package risk

import (
	"math"
	"sort"

	"github.com/shopspring/decimal"
)

// VaRCalculator calculates Value at Risk
type VaRCalculator struct {
	method     string
	confidence float64
}

// NewVaRCalculator creates a new VaR calculator
func NewVaRCalculator(method string, confidence float64) *VaRCalculator {
	return &VaRCalculator{
		method:     method,
		confidence: confidence,
	}
}

// Calculate calculates VaR for given returns
func (vc *VaRCalculator) Calculate(returns []decimal.Decimal) decimal.Decimal {
	switch vc.method {
	case "historical":
		return vc.historicalVaR(returns)
	case "parametric":
		return vc.parametricVaR(returns)
	case "monte_carlo":
		return vc.monteCarloVaR(returns)
	default:
		return vc.historicalVaR(returns)
	}
}

// CalculateCVaR calculates Conditional Value at Risk (Expected Shortfall)
func (vc *VaRCalculator) CalculateCVaR(returns []decimal.Decimal) decimal.Decimal {
	if len(returns) == 0 {
		return decimal.Zero
	}
	
	// Sort returns in ascending order
	sorted := make([]decimal.Decimal, len(returns))
	copy(sorted, returns)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].LessThan(sorted[j])
	})
	
	// Find VaR cutoff index
	cutoffIndex := int(float64(len(sorted)) * (1 - vc.confidence))
	if cutoffIndex == 0 {
		cutoffIndex = 1
	}
	
	// Calculate average of returns below VaR
	sum := decimal.Zero
	for i := 0; i < cutoffIndex; i++ {
		sum = sum.Add(sorted[i])
	}
	
	return sum.Div(decimal.NewFromInt(int64(cutoffIndex))).Neg()
}

// historicalVaR calculates VaR using historical simulation
func (vc *VaRCalculator) historicalVaR(returns []decimal.Decimal) decimal.Decimal {
	if len(returns) == 0 {
		return decimal.Zero
	}
	
	// Sort returns in ascending order
	sorted := make([]decimal.Decimal, len(returns))
	copy(sorted, returns)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].LessThan(sorted[j])
	})
	
	// Find percentile
	index := int(float64(len(sorted)) * (1 - vc.confidence))
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	if index < 0 {
		index = 0
	}
	
	return sorted[index].Neg()
}

// parametricVaR calculates VaR using parametric method
func (vc *VaRCalculator) parametricVaR(returns []decimal.Decimal) decimal.Decimal {
	if len(returns) == 0 {
		return decimal.Zero
	}
	
	// Calculate mean and standard deviation
	mean := decimal.Zero
	for _, r := range returns {
		mean = mean.Add(r)
	}
	mean = mean.Div(decimal.NewFromInt(int64(len(returns))))
	
	// Calculate variance
	variance := decimal.Zero
	for _, r := range returns {
		diff := r.Sub(mean)
		variance = variance.Add(diff.Mul(diff))
	}
	variance = variance.Div(decimal.NewFromInt(int64(len(returns) - 1)))
	
	// Calculate standard deviation
	stdDev := decimal.NewFromFloat(math.Sqrt(variance.InexactFloat64()))
	
	// Z-score for confidence level
	zScore := decimal.NewFromFloat(vc.getZScore())
	
	// VaR = mean - z * sigma
	return mean.Sub(zScore.Mul(stdDev)).Neg()
}

// monteCarloVaR calculates VaR using Monte Carlo simulation
func (vc *VaRCalculator) monteCarloVaR(returns []decimal.Decimal) decimal.Decimal {
	// Simplified Monte Carlo - in production use proper random number generation
	// For now, fallback to historical method
	return vc.historicalVaR(returns)
}

// getZScore returns z-score for confidence level
func (vc *VaRCalculator) getZScore() float64 {
	// Common z-scores
	switch vc.confidence {
	case 0.99:
		return 2.33
	case 0.95:
		return 1.65
	case 0.90:
		return 1.28
	default:
		// Use inverse normal approximation
		return 2.33
	}
}

// StressTester performs stress testing
type StressTester struct {
	scenarios []StressScenario
}

// NewStressTester creates a new stress tester
func NewStressTester() *StressTester {
	return &StressTester{
		scenarios: make([]StressScenario, 0),
	}
}

// RunScenarios runs stress test scenarios
func (st *StressTester) RunScenarios(rm *MultiAccountRiskManager, scenarios []StressScenario) *StressTestResults {
	results := &StressTestResults{
		Scenarios: make(map[string]*ScenarioResult),
	}
	
	// Get current state
	baseReport := rm.GetRiskReport()
	
	for _, scenario := range scenarios {
		result := st.runSingleScenario(rm, baseReport, scenario)
		results.Scenarios[scenario.Name] = result
	}
	
	return results
}

// runSingleScenario runs a single stress test scenario
func (st *StressTester) runSingleScenario(rm *MultiAccountRiskManager, baseReport *RiskReport, scenario StressScenario) *ScenarioResult {
	result := &ScenarioResult{
		ScenarioName: scenario.Name,
		Impact:       make(map[string]decimal.Decimal),
	}
	
	// Calculate impact on each account
	totalImpact := decimal.Zero
	for accountID, risk := range baseReport.AccountRisks {
		// Apply market shock
		exposureImpact := risk.TotalExposure.Mul(scenario.MarketShock)
		
		// Apply volatility multiplier
		volImpact := risk.TotalExposure.Mul(decimal.NewFromFloat(0.01)).Mul(scenario.VolatilityMul.Sub(decimal.NewFromInt(1)))
		
		accountImpact := exposureImpact.Add(volImpact)
		result.Impact[accountID] = accountImpact
		totalImpact = totalImpact.Add(accountImpact)
	}
	
	result.TotalImpact = totalImpact
	result.ImpactPercent = totalImpact.Div(baseReport.GlobalRisk.TotalExposure).Mul(decimal.NewFromInt(100))
	
	// Check if would breach limits
	newExposure := baseReport.GlobalRisk.TotalExposure.Add(totalImpact)
	if newExposure.LessThan(decimal.Zero) {
		result.BreachesLimits = true
		result.BreachedLimits = append(result.BreachedLimits, "negative_exposure")
	}
	
	return result
}

// StressTestResults contains stress test results
type StressTestResults struct {
	Scenarios map[string]*ScenarioResult
}

// ScenarioResult contains results for a single scenario
type ScenarioResult struct {
	ScenarioName   string
	TotalImpact    decimal.Decimal
	ImpactPercent  decimal.Decimal
	Impact         map[string]decimal.Decimal // Per account impact
	BreachesLimits bool
	BreachedLimits []string
}