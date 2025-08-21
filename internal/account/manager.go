package account

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// Manager implements types.AccountManager
type Manager struct {
	mu sync.RWMutex
	
	// Account storage (in-memory)
	accounts map[string]*types.Account
	balances map[string]*types.AccountBalance
	positions map[string]*types.AccountPosition
	metrics  map[string]*types.AccountMetrics
	
	// Transfer tracking
	transfers map[string]*types.AccountTransfer
	
	// Rate limit tracking
	rateLimitTracker map[string]*RateLimitInfo
	
	// Configuration
	dataDir string
	config  *Config
}

// Config holds account manager configuration
type Config struct {
	DataDir          string
	SnapshotInterval time.Duration
	MetricsRetention time.Duration
}

// RateLimitInfo tracks rate limit usage
type RateLimitInfo struct {
	UsedWeight      int
	UsedOrders      int
	WindowStart     time.Time
	LastUpdate      time.Time
}

// NewManager creates a new account manager
func NewManager(config *Config) (*Manager, error) {
	m := &Manager{
		accounts:         make(map[string]*types.Account),
		balances:         make(map[string]*types.AccountBalance),
		positions:        make(map[string]*types.AccountPosition),
		metrics:          make(map[string]*types.AccountMetrics),
		transfers:        make(map[string]*types.AccountTransfer),
		rateLimitTracker: make(map[string]*RateLimitInfo),
		dataDir:          config.DataDir,
		config:           config,
	}
	
	// Create data directory
	if err := os.MkdirAll(config.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data dir: %w", err)
	}
	
	// Load accounts from disk
	if err := m.loadAccounts(); err != nil {
		return nil, fmt.Errorf("failed to load accounts: %w", err)
	}
	
	// Start periodic snapshot
	go m.snapshotLoop()
	
	return m, nil
}

// CreateAccount creates a new account
func (m *Manager) CreateAccount(account *types.Account) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Validate account
	if account.ID == "" {
		account.ID = m.generateAccountID(account.Exchange, account.Name)
	}
	
	if _, exists := m.accounts[account.ID]; exists {
		return fmt.Errorf("account %s already exists", account.ID)
	}
	
	// Set timestamps
	account.CreatedAt = time.Now()
	account.UpdatedAt = time.Now()
	account.Active = true
	
	// Store account
	m.accounts[account.ID] = account
	
	// Initialize empty balance and metrics
	m.balances[account.ID] = &types.AccountBalance{
		AccountID: account.ID,
		Exchange:  account.Exchange,
		Balances:  make(map[string]*types.Balance),
		UpdatedAt: time.Now(),
	}
	
	m.metrics[account.ID] = &types.AccountMetrics{
		AccountID: account.ID,
		UpdatedAt: time.Now(),
	}
	
	// Save to disk
	return m.saveAccount(account)
}

// UpdateAccount updates account information
func (m *Manager) UpdateAccount(account *types.Account) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	existing, exists := m.accounts[account.ID]
	if !exists {
		return fmt.Errorf("account %s not found", account.ID)
	}
	
	// Preserve creation time
	account.CreatedAt = existing.CreatedAt
	account.UpdatedAt = time.Now()
	
	m.accounts[account.ID] = account
	
	return m.saveAccount(account)
}

// DeleteAccount deletes an account
func (m *Manager) DeleteAccount(accountID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.accounts[accountID]; !exists {
		return fmt.Errorf("account %s not found", accountID)
	}
	
	// Mark as inactive instead of deleting
	m.accounts[accountID].Active = false
	m.accounts[accountID].UpdatedAt = time.Now()
	
	return m.saveAccount(m.accounts[accountID])
}

// SelectAccount selects the best account for given requirements
func (m *Manager) SelectAccount(strategy string, req types.AccountRequirements) (*types.Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var candidates []*types.Account
	
	for _, account := range m.accounts {
		if !account.Active {
			continue
		}
		
		// Check strategy match
		if strategy != "" && account.Strategy != strategy {
			continue
		}
		
		// Check market type
		if req.Market == types.MarketTypeSpot && !account.SpotEnabled {
			continue
		}
		if req.Market == types.MarketTypeFutures && !account.FuturesEnabled {
			continue
		}
		
		// Check balance requirements
		balance := m.balances[account.ID]
		if balance != nil && !req.MinBalance.IsZero() {
			if balance.TotalUSDT.LessThan(req.MinBalance) {
				continue
			}
		}
		
		// Check rate limit availability
		if !m.hasAvailableRateLimit(account.ID, req.RequiredWeight) {
			continue
		}
		
		// Check risk limits
		if !req.OrderSize.IsZero() && account.MaxPositionUSDT.GreaterThan(decimal.Zero) {
			metrics := m.metrics[account.ID]
			if metrics != nil {
				currentPosition := decimal.NewFromInt(int64(metrics.OpenPositions))
				if currentPosition.Add(req.OrderSize).GreaterThan(account.MaxPositionUSDT) {
					continue
				}
			}
		}
		
		candidates = append(candidates, account)
	}
	
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no suitable account found")
	}
	
	// Select account with most available rate limit
	best := candidates[0]
	bestAvailable := m.getAvailableWeight(best.ID)
	
	for _, account := range candidates[1:] {
		available := m.getAvailableWeight(account.ID)
		if available > bestAvailable {
			best = account
			bestAvailable = available
		}
	}
	
	// Update last used time
	best.LastUsed = time.Now()
	
	return best, nil
}

// SelectAccountForOrder selects account for a specific order
func (m *Manager) SelectAccountForOrder(order *types.Order) (*types.Account, error) {
	// Determine market type from order
	market := types.MarketTypeSpot
	if order.MarginType != "" {
		market = types.MarketTypeFutures
	}
	
	// Calculate order value
	orderValue := order.Quantity.Mul(order.Price)
	
	req := types.AccountRequirements{
		RequiredWeight: 1, // Basic order weight
		Market:         market,
		Symbol:         order.Symbol,
		OrderSize:      orderValue,
	}
	
	// Use strategy from order metadata if available
	strategy := ""
	if order.Metadata != nil {
		if s, ok := order.Metadata["strategy"].(string); ok {
			strategy = s
		}
	}
	
	return m.SelectAccount(strategy, req)
}

// GetAccount retrieves a specific account
func (m *Manager) GetAccount(accountID string) (*types.Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	account, exists := m.accounts[accountID]
	if !exists {
		return nil, fmt.Errorf("account %s not found", accountID)
	}
	
	return account, nil
}

// ListAccounts lists accounts matching filter
func (m *Manager) ListAccounts(filter types.AccountFilter) ([]*types.Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var results []*types.Account
	
	for _, account := range m.accounts {
		// Apply filters
		if filter.Exchange != "" && account.Exchange != filter.Exchange {
			continue
		}
		
		if filter.Type != "" && account.Type != filter.Type {
			continue
		}
		
		if filter.Strategy != "" && account.Strategy != filter.Strategy {
			continue
		}
		
		if filter.Active != nil && account.Active != *filter.Active {
			continue
		}
		
		if filter.Market == types.MarketTypeSpot && !account.SpotEnabled {
			continue
		}
		
		if filter.Market == types.MarketTypeFutures && !account.FuturesEnabled {
			continue
		}
		
		// Check minimum balance
		if !filter.MinBalance.IsZero() {
			balance := m.balances[account.ID]
			if balance == nil || balance.TotalUSDT.LessThan(filter.MinBalance) {
				continue
			}
		}
		
		results = append(results, account)
	}
	
	return results, nil
}

// GetBalance retrieves account balance
func (m *Manager) GetBalance(accountID string) (*types.AccountBalance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	balance, exists := m.balances[accountID]
	if !exists {
		return nil, fmt.Errorf("balance not found for account %s", accountID)
	}
	
	return balance, nil
}

// UpdateBalance updates account balance
func (m *Manager) UpdateBalance(accountID string, balance *types.AccountBalance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.accounts[accountID]; !exists {
		return fmt.Errorf("account %s not found", accountID)
	}
	
	balance.AccountID = accountID
	balance.UpdatedAt = time.Now()
	m.balances[accountID] = balance
	
	return nil
}

// GetPositions retrieves account positions
func (m *Manager) GetPositions(accountID string) (*types.AccountPosition, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	positions, exists := m.positions[accountID]
	if !exists {
		return &types.AccountPosition{
			AccountID: accountID,
			Positions: make(map[string]*types.Position),
			UpdatedAt: time.Now(),
		}, nil
	}
	
	return positions, nil
}

// UpdatePositions updates account positions
func (m *Manager) UpdatePositions(accountID string, positions *types.AccountPosition) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.accounts[accountID]; !exists {
		return fmt.Errorf("account %s not found", accountID)
	}
	
	positions.AccountID = accountID
	positions.UpdatedAt = time.Now()
	m.positions[accountID] = positions
	
	return nil
}

// UpdateAccountMetrics updates account metrics
func (m *Manager) UpdateAccountMetrics(accountID string, metrics types.AccountMetrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.accounts[accountID]; !exists {
		return fmt.Errorf("account %s not found", accountID)
	}
	
	metrics.AccountID = accountID
	metrics.UpdatedAt = time.Now()
	m.metrics[accountID] = &metrics
	
	// Update rate limit info
	if rl, exists := m.rateLimitTracker[accountID]; exists {
		rl.UsedWeight = metrics.UsedWeight
		rl.LastUpdate = time.Now()
	}
	
	return nil
}

// GetMetrics retrieves account metrics
func (m *Manager) GetMetrics(accountID string) (*types.AccountMetrics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	metrics, exists := m.metrics[accountID]
	if !exists {
		return nil, fmt.Errorf("metrics not found for account %s", accountID)
	}
	
	return metrics, nil
}

// Transfer transfers assets between accounts
func (m *Manager) Transfer(transfer *types.AccountTransfer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Validate accounts exist
	fromAccount, exists := m.accounts[transfer.FromAccount]
	if !exists {
		return fmt.Errorf("from account %s not found", transfer.FromAccount)
	}
	
	toAccount, exists := m.accounts[transfer.ToAccount]
	if !exists {
		return fmt.Errorf("to account %s not found", transfer.ToAccount)
	}
	
	// Check if both accounts are on same exchange
	if fromAccount.Exchange != toAccount.Exchange {
		return fmt.Errorf("cross-exchange transfers not supported")
	}
	
	// Generate transfer ID
	transfer.ID = fmt.Sprintf("%s-%d", fromAccount.Exchange, time.Now().UnixNano())
	transfer.CreatedAt = time.Now()
	transfer.Status = "pending"
	
	// Store transfer
	m.transfers[transfer.ID] = transfer
	
	// In production, this would initiate actual transfer via exchange API
	// For now, we'll simulate success
	go func() {
		time.Sleep(2 * time.Second)
		m.mu.Lock()
		defer m.mu.Unlock()
		
		if t, exists := m.transfers[transfer.ID]; exists {
			t.Status = "completed"
			now := time.Now()
			t.CompletedAt = now
		}
	}()
	
	return nil
}

// GetTransferHistory retrieves transfer history
func (m *Manager) GetTransferHistory(accountID string, limit int) ([]*types.AccountTransfer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var transfers []*types.AccountTransfer
	
	for _, transfer := range m.transfers {
		if transfer.FromAccount == accountID || transfer.ToAccount == accountID {
			transfers = append(transfers, transfer)
		}
	}
	
	// Sort by creation time (newest first)
	// In production, implement proper sorting
	
	if limit > 0 && len(transfers) > limit {
		transfers = transfers[:limit]
	}
	
	return transfers, nil
}

// RebalanceAccounts rebalances assets across accounts
func (m *Manager) RebalanceAccounts(rules types.RebalanceRules) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Get main account
	var mainAccount *types.Account
	var subAccounts []*types.Account
	
	for _, account := range m.accounts {
		if !account.Active {
			continue
		}
		
		if account.Type == types.AccountTypeMain {
			mainAccount = account
		} else if account.Type == types.AccountTypeSub {
			subAccounts = append(subAccounts, account)
		}
	}
	
	if mainAccount == nil {
		return fmt.Errorf("no main account found")
	}
	
	// Check main account balance
	mainBalance := m.balances[mainAccount.ID]
	if mainBalance == nil {
		return fmt.Errorf("main account balance not found")
	}
	
	// Calculate required transfers
	var transfers []*types.AccountTransfer
	
	// Ensure main account has minimum balance
	if mainBalance.TotalUSDT.LessThan(rules.MinMainBalance) {
		// Pull from sub accounts
		deficit := rules.MinMainBalance.Sub(mainBalance.TotalUSDT)
		
		for _, subAccount := range subAccounts {
			if deficit.IsZero() {
				break
			}
			
			subBalance := m.balances[subAccount.ID]
			if subBalance == nil || subBalance.TotalUSDT.IsZero() {
				continue
			}
			
			// Calculate transfer amount
			transferAmount := subBalance.TotalUSDT
			if transferAmount.GreaterThan(deficit) {
				transferAmount = deficit
			}
			
			transfers = append(transfers, &types.AccountTransfer{
				FromAccount: subAccount.ID,
				ToAccount:   mainAccount.ID,
				Asset:       "USDT",
				Amount:      transferAmount,
			})
			
			deficit = deficit.Sub(transferAmount)
		}
	}
	
	// Check sub account limits
	for _, subAccount := range subAccounts {
		subBalance := m.balances[subAccount.ID]
		if subBalance == nil {
			continue
		}
		
		// If over limit, transfer excess to main
		if subBalance.TotalUSDT.GreaterThan(rules.MaxSubBalance) {
			excess := subBalance.TotalUSDT.Sub(rules.MaxSubBalance)
			
			transfers = append(transfers, &types.AccountTransfer{
				FromAccount: subAccount.ID,
				ToAccount:   mainAccount.ID,
				Asset:       "USDT",
				Amount:      excess,
			})
		}
		
		// Check target allocations
		if targetAlloc, exists := rules.TargetAllocations[subAccount.Strategy]; exists {
			currentAlloc := subBalance.TotalUSDT
			targetAmount := mainBalance.TotalUSDT.Mul(targetAlloc)
			
			diff := targetAmount.Sub(currentAlloc).Abs()
			if diff.GreaterThan(rules.Threshold) {
				if currentAlloc.LessThan(targetAmount) {
					// Transfer from main to sub
					transfers = append(transfers, &types.AccountTransfer{
						FromAccount: mainAccount.ID,
						ToAccount:   subAccount.ID,
						Asset:       "USDT",
						Amount:      targetAmount.Sub(currentAlloc),
					})
				} else {
					// Transfer from sub to main
					transfers = append(transfers, &types.AccountTransfer{
						FromAccount: subAccount.ID,
						ToAccount:   mainAccount.ID,
						Asset:       "USDT",
						Amount:      currentAlloc.Sub(targetAmount),
					})
				}
			}
		}
	}
	
	// Execute transfers (unless dry run)
	if !rules.DryRun {
		for _, transfer := range transfers {
			if err := m.Transfer(transfer); err != nil {
				return fmt.Errorf("transfer failed: %w", err)
			}
		}
	}
	
	return nil
}

// RotateAccounts rotates accounts for rate limit distribution
func (m *Manager) RotateAccounts(strategy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Find accounts for strategy
	var accounts []*types.Account
	for _, account := range m.accounts {
		if account.Active && account.Strategy == strategy {
			accounts = append(accounts, account)
		}
	}
	
	if len(accounts) == 0 {
		return fmt.Errorf("no accounts found for strategy %s", strategy)
	}
	
	// Sort by last used time (oldest first)
	// In production, implement proper sorting
	
	// Reset rate limit for oldest account
	if len(accounts) > 0 {
		oldest := accounts[0]
		delete(m.rateLimitTracker, oldest.ID)
	}
	
	return nil
}

// Helper methods

func (m *Manager) generateAccountID(exchange, name string) string {
	return fmt.Sprintf("%s_%s_%d", exchange, name, time.Now().Unix())
}

func (m *Manager) hasAvailableRateLimit(accountID string, requiredWeight int) bool {
	account := m.accounts[accountID]
	rl, exists := m.rateLimitTracker[accountID]
	
	if !exists {
		return true
	}
	
	// Check if window has expired (1 minute for most exchanges)
	if time.Since(rl.WindowStart) > time.Minute {
		return true
	}
	
	return rl.UsedWeight+requiredWeight <= account.RateLimitWeight
}

func (m *Manager) getAvailableWeight(accountID string) int {
	account := m.accounts[accountID]
	rl, exists := m.rateLimitTracker[accountID]
	
	if !exists || time.Since(rl.WindowStart) > time.Minute {
		return account.RateLimitWeight
	}
	
	return account.RateLimitWeight - rl.UsedWeight
}

func (m *Manager) loadAccounts() error {
	accountsFile := filepath.Join(m.dataDir, "accounts.json")
	
	data, err := os.ReadFile(accountsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No accounts yet
		}
		return err
	}
	
	var accounts []*types.Account
	if err := json.Unmarshal(data, &accounts); err != nil {
		return err
	}
	
	for _, account := range accounts {
		m.accounts[account.ID] = account
	}
	
	return nil
}

func (m *Manager) saveAccount(account *types.Account) error {
	// Save individual account
	accountFile := filepath.Join(m.dataDir, fmt.Sprintf("account_%s.json", account.ID))
	
	data, err := json.MarshalIndent(account, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(accountFile, data, 0644)
}

func (m *Manager) snapshotLoop() {
	ticker := time.NewTicker(m.config.SnapshotInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		m.saveSnapshot()
	}
}

func (m *Manager) saveSnapshot() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Save all accounts
	var accounts []*types.Account
	for _, account := range m.accounts {
		accounts = append(accounts, account)
	}
	
	data, err := json.MarshalIndent(accounts, "", "  ")
	if err != nil {
		return
	}
	
	snapshotFile := filepath.Join(m.dataDir, "accounts.json")
	os.WriteFile(snapshotFile, data, 0644)
}

// UpdateRateLimit updates rate limit usage for an account
func (m *Manager) UpdateRateLimit(accountID string, weight int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	rl, exists := m.rateLimitTracker[accountID]
	if !exists {
		rl = &RateLimitInfo{
			WindowStart: time.Now(),
		}
		m.rateLimitTracker[accountID] = rl
	}
	
	// Reset if window expired
	if time.Since(rl.WindowStart) > time.Minute {
		rl.WindowStart = time.Now()
		rl.UsedWeight = 0
		rl.UsedOrders = 0
	}
	
	rl.UsedWeight += weight
	rl.LastUpdate = time.Now()
}