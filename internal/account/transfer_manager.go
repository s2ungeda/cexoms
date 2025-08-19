package account

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/mExOms/pkg/types"
	"github.com/shopspring/decimal"
)

// TransferManager manages asset transfers between accounts
type TransferManager struct {
	mu sync.RWMutex
	
	manager   *Manager
	exchanges map[string]types.ExchangeMultiAccount
	
	// Transfer tracking
	pendingTransfers map[string]*types.AccountTransfer
	transferHistory  []*types.AccountTransfer
	
	// Rebalancing configuration
	rebalanceRules   []*RebalanceRule
	rebalanceEnabled bool
	
	// Transfer limits
	dailyLimit       decimal.Decimal
	singleLimit      decimal.Decimal
	dailyUsed        decimal.Decimal
	limitResetTime   time.Time
	
	// Background workers
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// RebalanceRule defines automatic rebalancing rules
type RebalanceRule struct {
	Name        string
	Priority    int
	Schedule    string // "daily", "weekly", "hourly"
	NextRun     time.Time
	Condition   func(accounts []*types.Account) bool
	Action      func(tm *TransferManager, accounts []*types.Account) error
	
	// Rule configuration
	MinBalance      decimal.Decimal
	MaxBalance      decimal.Decimal
	TargetBalance   decimal.Decimal
	RebalanceRatio  float64 // e.g., 0.8 = move 80% of excess
}

// TransferRequest represents a transfer request
type TransferRequest struct {
	FromAccount string
	ToAccount   string
	Asset       string
	Amount      decimal.Decimal
	Reason      string
	Priority    int
}

// NewTransferManager creates a new transfer manager
func NewTransferManager(manager *Manager) *TransferManager {
	tm := &TransferManager{
		manager:          manager,
		exchanges:        make(map[string]types.ExchangeMultiAccount),
		pendingTransfers: make(map[string]*types.AccountTransfer),
		transferHistory:  make([]*types.AccountTransfer, 0),
		rebalanceRules:   make([]*RebalanceRule, 0),
		rebalanceEnabled: true,
		dailyLimit:       decimal.NewFromInt(1000000), // $1M daily limit
		singleLimit:      decimal.NewFromInt(100000),  // $100k per transfer
		dailyUsed:        decimal.Zero,
		limitResetTime:   time.Now().Add(24 * time.Hour),
		stopCh:           make(chan struct{}),
	}
	
	// Add default rebalancing rules
	tm.addDefaultRules()
	
	// Start background workers
	tm.wg.Add(2)
	go tm.rebalanceWorker()
	go tm.transferWorker()
	
	return tm
}

// RegisterExchange registers an exchange for transfers
func (tm *TransferManager) RegisterExchange(name string, exchange types.ExchangeMultiAccount) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	tm.exchanges[name] = exchange
}

// RequestTransfer creates a new transfer request
func (tm *TransferManager) RequestTransfer(ctx context.Context, req *TransferRequest) (*types.AccountTransfer, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	// Validate accounts
	fromAccount, err := tm.manager.GetAccount(req.FromAccount)
	if err != nil {
		return nil, fmt.Errorf("invalid from account: %w", err)
	}
	
	toAccount, err := tm.manager.GetAccount(req.ToAccount)
	if err != nil {
		return nil, fmt.Errorf("invalid to account: %w", err)
	}
	
	// Check if same exchange
	if fromAccount.Exchange != toAccount.Exchange {
		return nil, fmt.Errorf("cross-exchange transfers not supported")
	}
	
	// Check transfer limits
	if err := tm.checkTransferLimits(req.Amount); err != nil {
		return nil, err
	}
	
	// Check source account balance
	balance, err := tm.manager.GetBalance(req.FromAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}
	
	// Simplified check - in production, check specific asset
	if balance.TotalUSDT.LessThan(req.Amount) {
		return nil, fmt.Errorf("insufficient balance in source account")
	}
	
	// Create transfer record
	transfer := &types.AccountTransfer{
		ID:          fmt.Sprintf("tf_%d", time.Now().UnixNano()),
		FromAccount: req.FromAccount,
		ToAccount:   req.ToAccount,
		Exchange:    fromAccount.Exchange,
		Asset:       req.Asset,
		Amount:      req.Amount,
		Reason:      req.Reason,
		Status:      "pending",
		RequestedAt: time.Now(),
	}
	
	// Add to pending transfers
	tm.pendingTransfers[transfer.ID] = transfer
	
	return transfer, nil
}

// ExecuteTransfer executes a pending transfer
func (tm *TransferManager) ExecuteTransfer(ctx context.Context, transferID string) error {
	tm.mu.Lock()
	transfer, exists := tm.pendingTransfers[transferID]
	if !exists {
		tm.mu.Unlock()
		return fmt.Errorf("transfer %s not found", transferID)
	}
	tm.mu.Unlock()
	
	// Get exchange
	exchange, exists := tm.exchanges[transfer.Exchange]
	if !exists {
		return fmt.Errorf("exchange %s not registered", transfer.Exchange)
	}
	
	// Execute transfer through exchange API
	transferReq := &types.AccountTransferRequest{
		FromAccountID: transfer.FromAccount,
		ToAccountID:   transfer.ToAccount,
		Asset:         transfer.Asset,
		Amount:        transfer.Amount,
	}
	
	resp, err := exchange.TransferBetweenAccounts(ctx, transferReq)
	if err != nil {
		tm.updateTransferStatus(transferID, "failed", err.Error())
		return fmt.Errorf("transfer failed: %w", err)
	}
	
	// Update transfer record
	tm.mu.Lock()
	transfer.ExchangeTransferID = resp.TransferID
	transfer.Status = "completed"
	transfer.CompletedAt = time.Now()
	
	// Move to history
	tm.transferHistory = append(tm.transferHistory, transfer)
	delete(tm.pendingTransfers, transferID)
	
	// Update daily usage
	tm.dailyUsed = tm.dailyUsed.Add(transfer.Amount)
	tm.mu.Unlock()
	
	// Update account balances
	tm.updateAccountBalances(transfer)
	
	return nil
}

// AddRebalanceRule adds a custom rebalancing rule
func (tm *TransferManager) AddRebalanceRule(rule *RebalanceRule) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	tm.rebalanceRules = append(tm.rebalanceRules, rule)
	
	// Sort by priority
	sort.Slice(tm.rebalanceRules, func(i, j int) bool {
		return tm.rebalanceRules[i].Priority > tm.rebalanceRules[j].Priority
	})
}

// RunRebalancing manually triggers rebalancing
func (tm *TransferManager) RunRebalancing(ctx context.Context) error {
	tm.mu.RLock()
	if !tm.rebalanceEnabled {
		tm.mu.RUnlock()
		return fmt.Errorf("rebalancing is disabled")
	}
	rules := tm.rebalanceRules
	tm.mu.RUnlock()
	
	// Get all accounts
	accounts, err := tm.manager.ListAccounts(types.AccountFilter{
		Active: &[]bool{true}[0],
	})
	if err != nil {
		return fmt.Errorf("failed to list accounts: %w", err)
	}
	
	// Group by exchange
	exchangeAccounts := make(map[string][]*types.Account)
	for _, account := range accounts {
		exchangeAccounts[account.Exchange] = append(exchangeAccounts[account.Exchange], account)
	}
	
	// Apply rules
	for _, rule := range rules {
		for exchange, accounts := range exchangeAccounts {
			if rule.Condition(accounts) {
				if err := rule.Action(tm, accounts); err != nil {
					// Log error but continue with other rules
					fmt.Printf("Rebalance rule %s failed for %s: %v\n", rule.Name, exchange, err)
				}
			}
		}
	}
	
	return nil
}

// GetTransferHistory returns transfer history
func (tm *TransferManager) GetTransferHistory(filter TransferFilter) []*types.AccountTransfer {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	filtered := make([]*types.AccountTransfer, 0)
	
	for _, transfer := range tm.transferHistory {
		if tm.matchesFilter(transfer, filter) {
			filtered = append(filtered, transfer)
		}
	}
	
	return filtered
}

// GetPendingTransfers returns pending transfers
func (tm *TransferManager) GetPendingTransfers() []*types.AccountTransfer {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	transfers := make([]*types.AccountTransfer, 0, len(tm.pendingTransfers))
	for _, transfer := range tm.pendingTransfers {
		transfers = append(transfers, transfer)
	}
	
	return transfers
}

// Helper methods

// checkTransferLimits checks if transfer exceeds limits
func (tm *TransferManager) checkTransferLimits(amount decimal.Decimal) error {
	// Reset daily limit if needed
	if time.Now().After(tm.limitResetTime) {
		tm.dailyUsed = decimal.Zero
		tm.limitResetTime = time.Now().Add(24 * time.Hour)
	}
	
	// Check single transfer limit
	if amount.GreaterThan(tm.singleLimit) {
		return fmt.Errorf("transfer amount exceeds single transfer limit of %s", tm.singleLimit.String())
	}
	
	// Check daily limit
	if tm.dailyUsed.Add(amount).GreaterThan(tm.dailyLimit) {
		return fmt.Errorf("transfer would exceed daily limit of %s", tm.dailyLimit.String())
	}
	
	return nil
}

// updateTransferStatus updates transfer status
func (tm *TransferManager) updateTransferStatus(transferID, status, message string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	if transfer, exists := tm.pendingTransfers[transferID]; exists {
		transfer.Status = status
		transfer.ErrorMessage = message
		transfer.UpdatedAt = time.Now()
	}
}

// updateAccountBalances updates account balances after transfer
func (tm *TransferManager) updateAccountBalances(transfer *types.AccountTransfer) {
	// Decrease from account balance
	fromBalance, _ := tm.manager.GetBalance(transfer.FromAccount)
	if fromBalance != nil {
		fromBalance.TotalUSDT = fromBalance.TotalUSDT.Sub(transfer.Amount)
		tm.manager.UpdateBalance(transfer.FromAccount, fromBalance)
	}
	
	// Increase to account balance
	toBalance, _ := tm.manager.GetBalance(transfer.ToAccount)
	if toBalance != nil {
		toBalance.TotalUSDT = toBalance.TotalUSDT.Add(transfer.Amount)
		tm.manager.UpdateBalance(transfer.ToAccount, toBalance)
	}
}

// matchesFilter checks if transfer matches filter
func (tm *TransferManager) matchesFilter(transfer *types.AccountTransfer, filter TransferFilter) bool {
	if filter.AccountID != "" {
		if transfer.FromAccount != filter.AccountID && transfer.ToAccount != filter.AccountID {
			return false
		}
	}
	
	if filter.Status != "" && transfer.Status != filter.Status {
		return false
	}
	
	if !filter.StartTime.IsZero() && transfer.RequestedAt.Before(filter.StartTime) {
		return false
	}
	
	if !filter.EndTime.IsZero() && transfer.RequestedAt.After(filter.EndTime) {
		return false
	}
	
	return true
}

// addDefaultRules adds default rebalancing rules
func (tm *TransferManager) addDefaultRules() {
	// Rule 1: Maintain minimum balance in sub-accounts
	tm.AddRebalanceRule(&RebalanceRule{
		Name:       "maintain_minimum_balance",
		Priority:   100,
		Schedule:   "hourly",
		MinBalance: decimal.NewFromInt(1000), // $1000 minimum
		Condition: func(accounts []*types.Account) bool {
			for _, acc := range accounts {
				if acc.Type == types.AccountTypeSub {
					balance, _ := tm.manager.GetBalance(acc.ID)
					if balance != nil && balance.TotalUSDT.LessThan(decimal.NewFromInt(1000)) {
						return true
					}
				}
			}
			return false
		},
		Action: func(tm *TransferManager, accounts []*types.Account) error {
			// Find main account
			var mainAccount *types.Account
			for _, acc := range accounts {
				if acc.Type == types.AccountTypeMain {
					mainAccount = acc
					break
				}
			}
			
			if mainAccount == nil {
				return fmt.Errorf("no main account found")
			}
			
			// Transfer to accounts below minimum
			for _, acc := range accounts {
				if acc.Type == types.AccountTypeSub {
					balance, _ := tm.manager.GetBalance(acc.ID)
					if balance != nil && balance.TotalUSDT.LessThan(decimal.NewFromInt(1000)) {
						deficit := decimal.NewFromInt(1000).Sub(balance.TotalUSDT)
						
						req := &TransferRequest{
							FromAccount: mainAccount.ID,
							ToAccount:   acc.ID,
							Asset:       "USDT",
							Amount:      deficit,
							Reason:      "maintain_minimum_balance",
						}
						
						transfer, err := tm.RequestTransfer(context.Background(), req)
						if err == nil {
							tm.ExecuteTransfer(context.Background(), transfer.ID)
						}
					}
				}
			}
			
			return nil
		},
	})
	
	// Rule 2: Balance distribution across strategy accounts
	tm.AddRebalanceRule(&RebalanceRule{
		Name:           "balance_distribution",
		Priority:       80,
		Schedule:       "daily",
		RebalanceRatio: 0.8,
		Condition: func(accounts []*types.Account) bool {
			// Check if there's significant imbalance
			var total decimal.Decimal
			strategyTotals := make(map[string]decimal.Decimal)
			
			for _, acc := range accounts {
				if acc.Strategy != "" {
					balance, _ := tm.manager.GetBalance(acc.ID)
					if balance != nil {
						total = total.Add(balance.TotalUSDT)
						strategyTotals[acc.Strategy] = strategyTotals[acc.Strategy].Add(balance.TotalUSDT)
					}
				}
			}
			
			if total.IsZero() {
				return false
			}
			
			// Check if any strategy has more than 50% of total
			for _, strategyTotal := range strategyTotals {
				ratio := strategyTotal.Div(total).InexactFloat64()
				if ratio > 0.5 {
					return true
				}
			}
			
			return false
		},
		Action: func(tm *TransferManager, accounts []*types.Account) error {
			// Implement balanced distribution logic
			// This is simplified - in production, implement proper distribution algorithm
			return nil
		},
	})
	
	// Rule 3: Consolidate dust balances
	tm.AddRebalanceRule(&RebalanceRule{
		Name:     "consolidate_dust",
		Priority: 50,
		Schedule: "weekly",
		Condition: func(accounts []*types.Account) bool {
			dustCount := 0
			for _, acc := range accounts {
				balance, _ := tm.manager.GetBalance(acc.ID)
				if balance != nil && balance.TotalUSDT.LessThan(decimal.NewFromInt(100)) && balance.TotalUSDT.GreaterThan(decimal.Zero) {
					dustCount++
				}
			}
			return dustCount > 3 // More than 3 accounts with dust
		},
		Action: func(tm *TransferManager, accounts []*types.Account) error {
			// Find main account
			var mainAccount *types.Account
			for _, acc := range accounts {
				if acc.Type == types.AccountTypeMain {
					mainAccount = acc
					break
				}
			}
			
			if mainAccount == nil {
				return fmt.Errorf("no main account found")
			}
			
			// Consolidate dust to main account
			for _, acc := range accounts {
				if acc.ID == mainAccount.ID {
					continue
				}
				
				balance, _ := tm.manager.GetBalance(acc.ID)
				if balance != nil && balance.TotalUSDT.LessThan(decimal.NewFromInt(100)) && balance.TotalUSDT.GreaterThan(decimal.Zero) {
					req := &TransferRequest{
						FromAccount: acc.ID,
						ToAccount:   mainAccount.ID,
						Asset:       "USDT",
						Amount:      balance.TotalUSDT,
						Reason:      "consolidate_dust",
					}
					
					transfer, err := tm.RequestTransfer(context.Background(), req)
					if err == nil {
						tm.ExecuteTransfer(context.Background(), transfer.ID)
					}
				}
			}
			
			return nil
		},
	})
}

// Background workers

// rebalanceWorker runs periodic rebalancing
func (tm *TransferManager) rebalanceWorker() {
	defer tm.wg.Done()
	
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			tm.mu.RLock()
			enabled := tm.rebalanceEnabled
			rules := tm.rebalanceRules
			tm.mu.RUnlock()
			
			if !enabled {
				continue
			}
			
			// Check which rules should run
			now := time.Now()
			for _, rule := range rules {
				if now.After(rule.NextRun) {
					// Run rule
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
					tm.RunRebalancing(ctx)
					cancel()
					
					// Update next run time
					tm.updateNextRunTime(rule)
				}
			}
			
		case <-tm.stopCh:
			return
		}
	}
}

// transferWorker processes pending transfers
func (tm *TransferManager) transferWorker() {
	defer tm.wg.Done()
	
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			tm.processPendingTransfers()
			
		case <-tm.stopCh:
			return
		}
	}
}

// processPendingTransfers processes all pending transfers
func (tm *TransferManager) processPendingTransfers() {
	tm.mu.RLock()
	pendingIDs := make([]string, 0, len(tm.pendingTransfers))
	for id := range tm.pendingTransfers {
		pendingIDs = append(pendingIDs, id)
	}
	tm.mu.RUnlock()
	
	for _, id := range pendingIDs {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		tm.ExecuteTransfer(ctx, id)
		cancel()
	}
}

// updateNextRunTime updates rule's next run time
func (tm *TransferManager) updateNextRunTime(rule *RebalanceRule) {
	now := time.Now()
	
	switch rule.Schedule {
	case "hourly":
		rule.NextRun = now.Add(1 * time.Hour)
	case "daily":
		rule.NextRun = now.Add(24 * time.Hour)
	case "weekly":
		rule.NextRun = now.Add(7 * 24 * time.Hour)
	default:
		rule.NextRun = now.Add(1 * time.Hour)
	}
}

// Stop stops the transfer manager
func (tm *TransferManager) Stop() {
	close(tm.stopCh)
	tm.wg.Wait()
}

// GetStats returns transfer statistics
func (tm *TransferManager) GetStats() map[string]interface{} {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	stats := map[string]interface{}{
		"pending_transfers":   len(tm.pendingTransfers),
		"completed_transfers": len(tm.transferHistory),
		"daily_limit":         tm.dailyLimit.String(),
		"daily_used":          tm.dailyUsed.String(),
		"daily_remaining":     tm.dailyLimit.Sub(tm.dailyUsed).String(),
		"rebalance_enabled":   tm.rebalanceEnabled,
		"active_rules":        len(tm.rebalanceRules),
	}
	
	// Transfer summary by status
	statusCount := make(map[string]int)
	for _, transfer := range tm.transferHistory {
		statusCount[transfer.Status]++
	}
	stats["transfer_summary"] = statusCount
	
	return stats
}

// TransferFilter for filtering transfer history
type TransferFilter struct {
	AccountID string
	Status    string
	StartTime time.Time
	EndTime   time.Time
}