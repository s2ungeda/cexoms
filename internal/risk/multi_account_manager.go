package risk

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mExOms/oms/pkg/types"
	"github.com/shopspring/decimal"
)

// MultiAccountRiskManager extends risk management across multiple accounts
type MultiAccountRiskManager struct {
	mu sync.RWMutex
	
	// Account management
	accountManager types.AccountManager
	
	// Risk tracking per account
	accountRisks    map[string]*AccountRisk
	
	// Global risk aggregation
	globalRisk      *GlobalRisk
	
	// Risk rules and limits
	rules           []RiskRule
	globalLimits    *GlobalRiskLimits
	
	// Configuration
	config          *MultiAccountRiskConfig
	
	// Alert channel
	alertChan       chan *RiskAlert
	
	// Background workers
	stopCh          chan struct{}
	wg              sync.WaitGroup
}

// AccountRisk tracks risk metrics for a single account
type AccountRisk struct {
	AccountID       string
	Exchange        string
	
	// Position metrics
	OpenPositions   int
	TotalExposure   decimal.Decimal
	NetExposure     decimal.Decimal
	
	// P&L tracking
	RealizedPnL     decimal.Decimal
	UnrealizedPnL   decimal.Decimal
	DailyPnL        decimal.Decimal
	
	// Risk metrics
	CurrentLeverage decimal.Decimal
	MarginUsed      decimal.Decimal
	MarginAvailable decimal.Decimal
	
	// Drawdown tracking
	PeakBalance     decimal.Decimal
	CurrentDrawdown decimal.Decimal
	MaxDrawdown     decimal.Decimal
	
	// Rate limit usage
	RateLimitUsage  float64
	
	LastUpdate      time.Time
}

// GlobalRisk aggregates risk across all accounts
type GlobalRisk struct {
	// Aggregate exposure
	TotalExposure      decimal.Decimal
	NetExposure        decimal.Decimal
	ExposureByExchange map[string]decimal.Decimal
	ExposureByStrategy map[string]decimal.Decimal
	
	// Aggregate P&L
	TotalRealizedPnL   decimal.Decimal
	TotalUnrealizedPnL decimal.Decimal
	DailyPnL           decimal.Decimal
	
	// Risk concentration
	LargestPosition    decimal.Decimal
	ConcentrationRatio float64
	
	// Account utilization
	ActiveAccounts     int
	TotalAccounts      int
	AccountsAtRisk     int
	
	LastUpdate         time.Time
}

// MultiAccountRiskConfig contains configuration for multi-account risk management
type MultiAccountRiskConfig struct {
	// Update intervals
	AccountUpdateInterval time.Duration
	GlobalUpdateInterval  time.Duration
	
	// Alert settings
	AlertsEnabled         bool
	AlertCooldown         time.Duration
	
	// Risk calculation
	VaRConfidence         float64
	VaRTimeHorizon        time.Duration
	StressTestScenarios   []StressScenario
}

// GlobalRiskLimits defines risk limits across all accounts
type GlobalRiskLimits struct {
	// Exposure limits
	MaxTotalExposure       decimal.Decimal
	MaxNetExposure         decimal.Decimal
	MaxExchangeExposure    decimal.Decimal
	MaxStrategyExposure    decimal.Decimal
	
	// Position limits
	MaxConcentrationRatio  float64
	MaxPositionSize        decimal.Decimal
	MaxAccountsPerStrategy int
	
	// Loss limits
	MaxDailyLoss           decimal.Decimal
	MaxDrawdown            decimal.Decimal
	
	// Leverage limits
	MaxAccountLeverage     int
	MaxGlobalLeverage      decimal.Decimal
}

// NewMultiAccountRiskManager creates a new multi-account risk manager
func NewMultiAccountRiskManager(accountManager types.AccountManager, config *MultiAccountRiskConfig) *MultiAccountRiskManager {
	if config == nil {
		config = &MultiAccountRiskConfig{
			AccountUpdateInterval: 5 * time.Second,
			GlobalUpdateInterval:  10 * time.Second,
			AlertsEnabled:         true,
			AlertCooldown:         5 * time.Minute,
			VaRConfidence:         0.99,
			VaRTimeHorizon:        24 * time.Hour,
		}
	}
	
	rm := &MultiAccountRiskManager{
		accountManager: accountManager,
		accountRisks:   make(map[string]*AccountRisk),
		globalRisk:     &GlobalRisk{
			ExposureByExchange: make(map[string]decimal.Decimal),
			ExposureByStrategy: make(map[string]decimal.Decimal),
		},
		rules:        make([]RiskRule, 0),
		config:       config,
		alertChan:    make(chan *RiskAlert, 100),
		stopCh:       make(chan struct{}),
	}
	
	// Set default global limits
	rm.globalLimits = &GlobalRiskLimits{
		MaxTotalExposure:       decimal.NewFromInt(10000000), // $10M
		MaxNetExposure:         decimal.NewFromInt(5000000),  // $5M
		MaxExchangeExposure:    decimal.NewFromInt(3000000),  // $3M per exchange
		MaxStrategyExposure:    decimal.NewFromInt(2000000),  // $2M per strategy
		MaxConcentrationRatio:  0.2,                           // 20% max in single position
		MaxPositionSize:        decimal.NewFromInt(500000),   // $500k max position
		MaxAccountsPerStrategy: 10,
		MaxDailyLoss:           decimal.NewFromInt(100000),   // $100k daily loss limit
		MaxDrawdown:            decimal.NewFromFloat(0.15),   // 15% max drawdown
		MaxAccountLeverage:     10,
		MaxGlobalLeverage:      decimal.NewFromInt(5),
	}
	
	// Add default risk rules
	rm.addDefaultRules()
	
	// Start background workers
	rm.wg.Add(2)
	go rm.accountMonitorWorker()
	go rm.globalRiskWorker()
	
	return rm
}

// ValidateOrder validates an order against risk rules across accounts
func (rm *MultiAccountRiskManager) ValidateOrder(ctx context.Context, accountID string, order *types.Order) error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	// Get account
	account, err := rm.accountManager.GetAccount(accountID)
	if err != nil {
		return fmt.Errorf("account not found: %w", err)
	}
	
	// Get account risk
	accountRisk, exists := rm.accountRisks[accountID]
	if !exists {
		// Initialize if not exists
		accountRisk = &AccountRisk{
			AccountID: accountID,
			Exchange:  account.Exchange,
		}
		rm.accountRisks[accountID] = accountRisk
	}
	
	// Calculate order exposure
	orderExposure := order.Quantity.Mul(order.Price)
	
	// Check account-level limits
	if err := rm.checkAccountLimits(account, accountRisk, orderExposure); err != nil {
		return fmt.Errorf("account limit exceeded: %w", err)
	}
	
	// Check global limits
	if err := rm.checkGlobalLimits(account, orderExposure); err != nil {
		return fmt.Errorf("global limit exceeded: %w", err)
	}
	
	// Apply custom risk rules
	for _, rule := range rm.rules {
		if err := rule.Validate(rm, account, order); err != nil {
			return fmt.Errorf("risk rule %s failed: %w", rule.Name, err)
		}
	}
	
	// Check cross-account constraints
	if err := rm.checkCrossAccountConstraints(account, order); err != nil {
		return err
	}
	
	return nil
}

// checkAccountLimits validates account-specific limits
func (rm *MultiAccountRiskManager) checkAccountLimits(account *types.Account, risk *AccountRisk, orderExposure decimal.Decimal) error {
	// Check position limit
	newExposure := risk.TotalExposure.Add(orderExposure)
	if !account.MaxPositionUSDT.IsZero() && newExposure.GreaterThan(account.MaxPositionUSDT) {
		return fmt.Errorf("would exceed account position limit: %s > %s", 
			newExposure.String(), account.MaxPositionUSDT.String())
	}
	
	// Check daily loss limit
	if !account.DailyLossLimit.IsZero() && risk.DailyPnL.LessThan(account.DailyLossLimit.Neg()) {
		return fmt.Errorf("account has reached daily loss limit: %s", risk.DailyPnL.String())
	}
	
	// Check leverage
	if account.MaxLeverage > 0 {
		balance, _ := rm.accountManager.GetBalance(account.ID)
		if balance != nil && !balance.TotalUSDT.IsZero() {
			leverage := newExposure.Div(balance.TotalUSDT)
			if leverage.GreaterThan(decimal.NewFromInt(int64(account.MaxLeverage))) {
				return fmt.Errorf("would exceed account leverage limit: %sx > %dx",
					leverage.String(), account.MaxLeverage)
			}
		}
	}
	
	// Check drawdown
	if !risk.PeakBalance.IsZero() {
		drawdown := risk.CurrentDrawdown
		maxDrawdown := decimal.NewFromFloat(0.2) // 20% default
		if drawdown.GreaterThan(maxDrawdown) {
			return fmt.Errorf("account in maximum drawdown: %s", drawdown.String())
		}
	}
	
	return nil
}

// checkGlobalLimits validates global risk limits
func (rm *MultiAccountRiskManager) checkGlobalLimits(account *types.Account, orderExposure decimal.Decimal) error {
	// Check total exposure
	newTotalExposure := rm.globalRisk.TotalExposure.Add(orderExposure)
	if newTotalExposure.GreaterThan(rm.globalLimits.MaxTotalExposure) {
		return fmt.Errorf("would exceed global exposure limit: %s > %s",
			newTotalExposure.String(), rm.globalLimits.MaxTotalExposure.String())
	}
	
	// Check exchange exposure
	exchangeExposure := rm.globalRisk.ExposureByExchange[account.Exchange].Add(orderExposure)
	if exchangeExposure.GreaterThan(rm.globalLimits.MaxExchangeExposure) {
		return fmt.Errorf("would exceed exchange exposure limit: %s > %s",
			exchangeExposure.String(), rm.globalLimits.MaxExchangeExposure.String())
	}
	
	// Check strategy exposure
	if account.Strategy != "" {
		strategyExposure := rm.globalRisk.ExposureByStrategy[account.Strategy].Add(orderExposure)
		if strategyExposure.GreaterThan(rm.globalLimits.MaxStrategyExposure) {
			return fmt.Errorf("would exceed strategy exposure limit: %s > %s",
				strategyExposure.String(), rm.globalLimits.MaxStrategyExposure.String())
		}
	}
	
	// Check position concentration
	if orderExposure.GreaterThan(rm.globalLimits.MaxPositionSize) {
		return fmt.Errorf("position size exceeds limit: %s > %s",
			orderExposure.String(), rm.globalLimits.MaxPositionSize.String())
	}
	
	// Check concentration ratio
	concentrationRatio := orderExposure.Div(newTotalExposure).InexactFloat64()
	if concentrationRatio > rm.globalLimits.MaxConcentrationRatio {
		return fmt.Errorf("would exceed concentration limit: %.2f%% > %.2f%%",
			concentrationRatio*100, rm.globalLimits.MaxConcentrationRatio*100)
	}
	
	// Check daily loss
	if rm.globalRisk.DailyPnL.LessThan(rm.globalLimits.MaxDailyLoss.Neg()) {
		return fmt.Errorf("global daily loss limit reached: %s",
			rm.globalRisk.DailyPnL.String())
	}
	
	return nil
}

// checkCrossAccountConstraints validates constraints across accounts
func (rm *MultiAccountRiskManager) checkCrossAccountConstraints(account *types.Account, order *types.Order) error {
	// Check correlated positions across accounts
	correlatedExposure := decimal.Zero
	for accID, risk := range rm.accountRisks {
		if accID == account.ID {
			continue
		}
		
		// Simple correlation check - in production use proper correlation matrix
		if rm.isCorrelated(order.Symbol, risk) {
			correlatedExposure = correlatedExposure.Add(risk.TotalExposure)
		}
	}
	
	// Limit correlated exposure
	maxCorrelatedExposure := rm.globalLimits.MaxTotalExposure.Mul(decimal.NewFromFloat(0.5))
	if correlatedExposure.GreaterThan(maxCorrelatedExposure) {
		return fmt.Errorf("high correlated exposure across accounts: %s",
			correlatedExposure.String())
	}
	
	// Check strategy account limits
	if account.Strategy != "" {
		strategyAccountCount := 0
		for _, acc := range rm.getAccountsByStrategy(account.Strategy) {
			if risk, exists := rm.accountRisks[acc.ID]; exists && risk.OpenPositions > 0 {
				strategyAccountCount++
			}
		}
		
		if strategyAccountCount >= rm.globalLimits.MaxAccountsPerStrategy {
			return fmt.Errorf("strategy %s has reached max accounts limit: %d",
				account.Strategy, rm.globalLimits.MaxAccountsPerStrategy)
		}
	}
	
	return nil
}

// UpdateAccountRisk updates risk metrics for an account
func (rm *MultiAccountRiskManager) UpdateAccountRisk(accountID string, positions []*types.Position) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	account, err := rm.accountManager.GetAccount(accountID)
	if err != nil {
		return err
	}
	
	risk, exists := rm.accountRisks[accountID]
	if !exists {
		risk = &AccountRisk{
			AccountID: accountID,
			Exchange:  account.Exchange,
		}
		rm.accountRisks[accountID] = risk
	}
	
	// Reset metrics
	risk.OpenPositions = 0
	risk.TotalExposure = decimal.Zero
	risk.NetExposure = decimal.Zero
	risk.UnrealizedPnL = decimal.Zero
	
	// Calculate position metrics
	for _, pos := range positions {
		if pos.Quantity.IsZero() {
			continue
		}
		
		risk.OpenPositions++
		positionValue := pos.Quantity.Mul(pos.MarkPrice)
		risk.TotalExposure = risk.TotalExposure.Add(positionValue.Abs())
		
		if pos.Side == types.PositionSideLong {
			risk.NetExposure = risk.NetExposure.Add(positionValue)
		} else {
			risk.NetExposure = risk.NetExposure.Sub(positionValue)
		}
		
		risk.UnrealizedPnL = risk.UnrealizedPnL.Add(pos.UnrealizedPnL)
	}
	
	// Update balance and drawdown
	balance, err := rm.accountManager.GetBalance(accountID)
	if err == nil && balance != nil {
		currentBalance := balance.TotalUSDT
		
		// Update peak balance
		if currentBalance.GreaterThan(risk.PeakBalance) {
			risk.PeakBalance = currentBalance
		}
		
		// Calculate drawdown
		if !risk.PeakBalance.IsZero() {
			risk.CurrentDrawdown = risk.PeakBalance.Sub(currentBalance).Div(risk.PeakBalance)
			if risk.CurrentDrawdown.GreaterThan(risk.MaxDrawdown) {
				risk.MaxDrawdown = risk.CurrentDrawdown
			}
		}
		
		// Calculate leverage
		if !currentBalance.IsZero() {
			risk.CurrentLeverage = risk.TotalExposure.Div(currentBalance)
		}
	}
	
	// Get account metrics for rate limit
	metrics, _ := rm.accountManager.GetMetrics(accountID)
	if metrics != nil {
		risk.RateLimitUsage = float64(metrics.UsedWeight) / float64(account.RateLimitWeight)
	}
	
	risk.LastUpdate = time.Now()
	
	// Check for alerts
	rm.checkAccountAlerts(account, risk)
	
	return nil
}

// AggregateGlobalRisk aggregates risk across all accounts
func (rm *MultiAccountRiskManager) AggregateGlobalRisk() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	// Reset global metrics
	rm.globalRisk.TotalExposure = decimal.Zero
	rm.globalRisk.NetExposure = decimal.Zero
	rm.globalRisk.TotalUnrealizedPnL = decimal.Zero
	rm.globalRisk.DailyPnL = decimal.Zero
	rm.globalRisk.ActiveAccounts = 0
	rm.globalRisk.AccountsAtRisk = 0
	
	// Clear maps
	for k := range rm.globalRisk.ExposureByExchange {
		delete(rm.globalRisk.ExposureByExchange, k)
	}
	for k := range rm.globalRisk.ExposureByStrategy {
		delete(rm.globalRisk.ExposureByStrategy, k)
	}
	
	// Aggregate from all accounts
	largestPosition := decimal.Zero
	
	for accountID, risk := range rm.accountRisks {
		if risk.OpenPositions == 0 {
			continue
		}
		
		rm.globalRisk.ActiveAccounts++
		
		// Aggregate exposure
		rm.globalRisk.TotalExposure = rm.globalRisk.TotalExposure.Add(risk.TotalExposure)
		rm.globalRisk.NetExposure = rm.globalRisk.NetExposure.Add(risk.NetExposure)
		
		// Aggregate P&L
		rm.globalRisk.TotalUnrealizedPnL = rm.globalRisk.TotalUnrealizedPnL.Add(risk.UnrealizedPnL)
		rm.globalRisk.DailyPnL = rm.globalRisk.DailyPnL.Add(risk.DailyPnL)
		
		// Track largest position
		if risk.TotalExposure.GreaterThan(largestPosition) {
			largestPosition = risk.TotalExposure
		}
		
		// Aggregate by exchange
		rm.globalRisk.ExposureByExchange[risk.Exchange] = 
			rm.globalRisk.ExposureByExchange[risk.Exchange].Add(risk.TotalExposure)
		
		// Aggregate by strategy
		account, _ := rm.accountManager.GetAccount(accountID)
		if account != nil && account.Strategy != "" {
			rm.globalRisk.ExposureByStrategy[account.Strategy] = 
				rm.globalRisk.ExposureByStrategy[account.Strategy].Add(risk.TotalExposure)
		}
		
		// Count accounts at risk
		if rm.isAccountAtRisk(risk) {
			rm.globalRisk.AccountsAtRisk++
		}
	}
	
	// Calculate concentration
	rm.globalRisk.LargestPosition = largestPosition
	if !rm.globalRisk.TotalExposure.IsZero() {
		rm.globalRisk.ConcentrationRatio = largestPosition.Div(rm.globalRisk.TotalExposure).InexactFloat64()
	}
	
	// Count total accounts
	accounts, _ := rm.accountManager.ListAccounts(types.AccountFilter{})
	rm.globalRisk.TotalAccounts = len(accounts)
	
	rm.globalRisk.LastUpdate = time.Now()
	
	// Check for global alerts
	rm.checkGlobalAlerts()
}

// GetRiskReport generates a comprehensive risk report
func (rm *MultiAccountRiskManager) GetRiskReport() *RiskReport {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	report := &RiskReport{
		Timestamp:    time.Now(),
		GlobalRisk:   rm.globalRisk,
		AccountRisks: make(map[string]*AccountRisk),
		Alerts:       make([]*RiskAlert, 0),
		Violations:   make([]*RiskViolation, 0),
	}
	
	// Copy account risks
	for id, risk := range rm.accountRisks {
		report.AccountRisks[id] = risk
	}
	
	// Add recent alerts
	// In production, maintain alert history
	
	// Check for current violations
	report.Violations = rm.checkCurrentViolations()
	
	// Calculate additional metrics
	report.Metrics = rm.calculateRiskMetrics()
	
	return report
}

// Helper methods

// isCorrelated checks if positions are correlated
func (rm *MultiAccountRiskManager) isCorrelated(symbol string, risk *AccountRisk) bool {
	// Simplified correlation check
	// In production, use proper correlation matrix
	return false
}

// getAccountsByStrategy returns accounts for a strategy
func (rm *MultiAccountRiskManager) getAccountsByStrategy(strategy string) []*types.Account {
	accounts, _ := rm.accountManager.ListAccounts(types.AccountFilter{
		Strategy: strategy,
	})
	return accounts
}

// isAccountAtRisk determines if an account is at risk
func (rm *MultiAccountRiskManager) isAccountAtRisk(risk *AccountRisk) bool {
	// High leverage
	if risk.CurrentLeverage.GreaterThan(decimal.NewFromInt(8)) {
		return true
	}
	
	// High drawdown
	if risk.CurrentDrawdown.GreaterThan(decimal.NewFromFloat(0.1)) {
		return true
	}
	
	// Negative daily P&L
	if risk.DailyPnL.LessThan(decimal.Zero) {
		return true
	}
	
	// High rate limit usage
	if risk.RateLimitUsage > 0.8 {
		return true
	}
	
	return false
}

// checkAccountAlerts checks for account-specific alerts
func (rm *MultiAccountRiskManager) checkAccountAlerts(account *types.Account, risk *AccountRisk) {
	// High leverage alert
	if risk.CurrentLeverage.GreaterThan(decimal.NewFromInt(int64(account.MaxLeverage) * 8 / 10)) {
		rm.sendAlert(&RiskAlert{
			Level:     "warning",
			Type:      "high_leverage",
			AccountID: account.ID,
			Message:   fmt.Sprintf("Account %s approaching leverage limit: %sx", account.ID, risk.CurrentLeverage),
		})
	}
	
	// Drawdown alert
	if risk.CurrentDrawdown.GreaterThan(decimal.NewFromFloat(0.1)) {
		rm.sendAlert(&RiskAlert{
			Level:     "critical",
			Type:      "drawdown",
			AccountID: account.ID,
			Message:   fmt.Sprintf("Account %s in drawdown: %.2f%%", account.ID, risk.CurrentDrawdown.Mul(decimal.NewFromInt(100)).InexactFloat64()),
		})
	}
}

// checkGlobalAlerts checks for global risk alerts
func (rm *MultiAccountRiskManager) checkGlobalAlerts() {
	// High concentration alert
	if rm.globalRisk.ConcentrationRatio > 0.15 {
		rm.sendAlert(&RiskAlert{
			Level:   "warning",
			Type:    "concentration",
			Message: fmt.Sprintf("High position concentration: %.2f%%", rm.globalRisk.ConcentrationRatio*100),
		})
	}
	
	// Multiple accounts at risk
	if rm.globalRisk.AccountsAtRisk > 3 {
		rm.sendAlert(&RiskAlert{
			Level:   "critical",
			Type:    "multiple_accounts_at_risk",
			Message: fmt.Sprintf("%d accounts currently at risk", rm.globalRisk.AccountsAtRisk),
		})
	}
}

// sendAlert sends a risk alert
func (rm *MultiAccountRiskManager) sendAlert(alert *RiskAlert) {
	alert.Timestamp = time.Now()
	
	select {
	case rm.alertChan <- alert:
	default:
		// Alert channel full, drop alert
	}
}

// checkCurrentViolations checks for current risk violations
func (rm *MultiAccountRiskManager) checkCurrentViolations() []*RiskViolation {
	violations := make([]*RiskViolation, 0)
	
	// Check global exposure violation
	if rm.globalRisk.TotalExposure.GreaterThan(rm.globalLimits.MaxTotalExposure) {
		violations = append(violations, &RiskViolation{
			Type:     "global_exposure",
			Severity: "critical",
			Message:  fmt.Sprintf("Global exposure exceeds limit: %s > %s", rm.globalRisk.TotalExposure, rm.globalLimits.MaxTotalExposure),
		})
	}
	
	// Check per-exchange violations
	for exchange, exposure := range rm.globalRisk.ExposureByExchange {
		if exposure.GreaterThan(rm.globalLimits.MaxExchangeExposure) {
			violations = append(violations, &RiskViolation{
				Type:     "exchange_exposure",
				Severity: "high",
				Message:  fmt.Sprintf("Exchange %s exposure exceeds limit: %s > %s", exchange, exposure, rm.globalLimits.MaxExchangeExposure),
			})
		}
	}
	
	return violations
}

// calculateRiskMetrics calculates additional risk metrics
func (rm *MultiAccountRiskManager) calculateRiskMetrics() map[string]interface{} {
	metrics := make(map[string]interface{})
	
	// Calculate average leverage
	totalLeverage := decimal.Zero
	count := 0
	for _, risk := range rm.accountRisks {
		if risk.OpenPositions > 0 {
			totalLeverage = totalLeverage.Add(risk.CurrentLeverage)
			count++
		}
	}
	
	if count > 0 {
		metrics["average_leverage"] = totalLeverage.Div(decimal.NewFromInt(int64(count))).InexactFloat64()
	}
	
	// Calculate utilization rate
	if rm.globalRisk.TotalAccounts > 0 {
		metrics["account_utilization"] = float64(rm.globalRisk.ActiveAccounts) / float64(rm.globalRisk.TotalAccounts)
	}
	
	// Risk score (0-100)
	riskScore := 0.0
	
	// Factor in exposure ratio
	exposureRatio := rm.globalRisk.TotalExposure.Div(rm.globalLimits.MaxTotalExposure).InexactFloat64()
	riskScore += exposureRatio * 30
	
	// Factor in concentration
	riskScore += rm.globalRisk.ConcentrationRatio * 100 * 20
	
	// Factor in accounts at risk
	if rm.globalRisk.ActiveAccounts > 0 {
		atRiskRatio := float64(rm.globalRisk.AccountsAtRisk) / float64(rm.globalRisk.ActiveAccounts)
		riskScore += atRiskRatio * 30
	}
	
	// Factor in P&L
	if rm.globalRisk.DailyPnL.LessThan(decimal.Zero) {
		lossRatio := rm.globalRisk.DailyPnL.Abs().Div(rm.globalLimits.MaxDailyLoss).InexactFloat64()
		riskScore += lossRatio * 20
	}
	
	metrics["risk_score"] = riskScore
	
	return metrics
}

// Background workers

// accountMonitorWorker monitors individual account risks
func (rm *MultiAccountRiskManager) accountMonitorWorker() {
	defer rm.wg.Done()
	
	ticker := time.NewTicker(rm.config.AccountUpdateInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// Update account risks
			accounts, _ := rm.accountManager.ListAccounts(types.AccountFilter{
				Active: &[]bool{true}[0],
			})
			
			for _, account := range accounts {
				// Get positions for account
				positions, err := rm.accountManager.GetPositions(account.ID)
				if err != nil {
					continue
				}
				
				// Convert to position slice
				var posSlice []*types.Position
				if positions != nil {
					for _, pos := range positions.Positions {
						posSlice = append(posSlice, pos)
					}
				}
				
				rm.UpdateAccountRisk(account.ID, posSlice)
			}
			
		case <-rm.stopCh:
			return
		}
	}
}

// globalRiskWorker aggregates global risk metrics
func (rm *MultiAccountRiskManager) globalRiskWorker() {
	defer rm.wg.Done()
	
	ticker := time.NewTicker(rm.config.GlobalUpdateInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			rm.AggregateGlobalRisk()
			
		case <-rm.stopCh:
			return
		}
	}
}

// Stop stops the risk manager
func (rm *MultiAccountRiskManager) Stop() {
	close(rm.stopCh)
	rm.wg.Wait()
	close(rm.alertChan)
}

// GetAlertChannel returns the alert channel
func (rm *MultiAccountRiskManager) GetAlertChannel() <-chan *RiskAlert {
	return rm.alertChan
}

// addDefaultRules adds default risk rules
func (rm *MultiAccountRiskManager) addDefaultRules() {
	// Add correlation risk rule
	rm.rules = append(rm.rules, RiskRule{
		Name: "correlation_limit",
		Validate: func(rm *MultiAccountRiskManager, account *types.Account, order *types.Order) error {
			// Implement correlation checking
			return nil
		},
	})
	
	// Add time-based risk rule
	rm.rules = append(rm.rules, RiskRule{
		Name: "trading_hours",
		Validate: func(rm *MultiAccountRiskManager, account *types.Account, order *types.Order) error {
			// Implement trading hours restriction
			return nil
		},
	})
}

// Supporting types

// RiskRule defines a custom risk validation rule
type RiskRule struct {
	Name     string
	Validate func(rm *MultiAccountRiskManager, account *types.Account, order *types.Order) error
}

// RiskAlert represents a risk alert
type RiskAlert struct {
	Timestamp time.Time
	Level     string // info, warning, critical
	Type      string
	AccountID string
	Message   string
}

// RiskViolation represents a risk limit violation
type RiskViolation struct {
	Type     string
	Severity string
	Message  string
}

// RiskReport contains comprehensive risk information
type RiskReport struct {
	Timestamp    time.Time
	GlobalRisk   *GlobalRisk
	AccountRisks map[string]*AccountRisk
	Alerts       []*RiskAlert
	Violations   []*RiskViolation
	Metrics      map[string]interface{}
}

// StressScenario defines a stress test scenario
type StressScenario struct {
	Name          string
	MarketShock   decimal.Decimal
	VolumeShock   decimal.Decimal
	VolatilityMul decimal.Decimal
}