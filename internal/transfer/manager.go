package transfer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mExOms/pkg/cache"
	"github.com/mExOms/pkg/storage"
	"github.com/mExOms/pkg/types"
)

type TransferType string

const (
	TransferTypeMainToSub TransferType = "MAIN_TO_SUB"
	TransferTypeSubToMain TransferType = "SUB_TO_MAIN"
	TransferTypeSubToSub  TransferType = "SUB_TO_SUB"
)

type TransferStatus string

const (
	TransferStatusPending   TransferStatus = "PENDING"
	TransferStatusExecuting TransferStatus = "EXECUTING"
	TransferStatusCompleted TransferStatus = "COMPLETED"
	TransferStatusFailed    TransferStatus = "FAILED"
	TransferStatusRetrying  TransferStatus = "RETRYING"
)

type TransferRequest struct {
	ID              string         `json:"id"`
	Exchange        string         `json:"exchange"`
	FromAccount     string         `json:"from_account"`
	ToAccount       string         `json:"to_account"`
	Asset           string         `json:"asset"`
	Amount          float64        `json:"amount"`
	Type            TransferType   `json:"type"`
	Status          TransferStatus `json:"status"`
	Reason          string         `json:"reason,omitempty"`
	ErrorMessage    string         `json:"error_message,omitempty"`
	RetryCount      int            `json:"retry_count"`
	CreatedAt       time.Time      `json:"created_at"`
	ExecutedAt      *time.Time     `json:"executed_at,omitempty"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
	TransactionID   string         `json:"transaction_id,omitempty"`
}

type TransferHistory struct {
	Transfers      []TransferRequest `json:"transfers"`
	TotalVolume    map[string]float64 `json:"total_volume"`
	SuccessCount   int               `json:"success_count"`
	FailureCount   int               `json:"failure_count"`
	LastRebalance  *time.Time        `json:"last_rebalance,omitempty"`
}

type RebalanceConfig struct {
	Enabled       bool                       `json:"enabled"`
	Schedule      string                     `json:"schedule"`
	MinMainBalance map[string]float64        `json:"min_main_balance"`
	MaxSubBalance  map[string]float64        `json:"max_sub_balance"`
	TargetRatios   map[string]map[string]float64 `json:"target_ratios"`
}

type Manager struct {
	mu              sync.RWMutex
	exchanges       map[string]types.Exchange
	accountCache    *cache.AccountCache
	storage         storage.Storage
	transferQueue   chan *TransferRequest
	rebalanceConfig RebalanceConfig
	history         *TransferHistory
	stopCh         chan struct{}
	wg             sync.WaitGroup
}

func NewManager(
	exchanges map[string]types.Exchange,
	accountCache *cache.AccountCache,
	storage storage.Storage,
) *Manager {
	return &Manager{
		exchanges:     exchanges,
		accountCache:  accountCache,
		storage:       storage,
		transferQueue: make(chan *TransferRequest, 1000),
		history: &TransferHistory{
			Transfers:   make([]TransferRequest, 0),
			TotalVolume: make(map[string]float64),
		},
		stopCh: make(chan struct{}),
	}
}

func (m *Manager) Start(ctx context.Context) error {
	m.wg.Add(2)
	
	go m.processTransferQueue(ctx)
	go m.runRebalanceScheduler(ctx)
	
	return nil
}

func (m *Manager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

func (m *Manager) RequestTransfer(req *TransferRequest) error {
	if req.ID == "" {
		req.ID = fmt.Sprintf("transfer_%d", time.Now().UnixNano())
	}
	
	if err := m.validateTransferRequest(req); err != nil {
		return fmt.Errorf("invalid transfer request: %w", err)
	}
	
	req.Status = TransferStatusPending
	req.CreatedAt = time.Now()
	
	select {
	case m.transferQueue <- req:
		m.recordTransfer(req)
		return nil
	default:
		return fmt.Errorf("transfer queue is full")
	}
}

func (m *Manager) GetTransferStatus(transferID string) (*TransferRequest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	for _, transfer := range m.history.Transfers {
		if transfer.ID == transferID {
			return &transfer, nil
		}
	}
	
	return nil, fmt.Errorf("transfer not found: %s", transferID)
}

func (m *Manager) GetTransferHistory(exchange, account string, limit int) []TransferRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var filtered []TransferRequest
	for _, transfer := range m.history.Transfers {
		if (exchange == "" || transfer.Exchange == exchange) &&
			(account == "" || transfer.FromAccount == account || transfer.ToAccount == account) {
			filtered = append(filtered, transfer)
		}
	}
	
	if limit > 0 && len(filtered) > limit {
		return filtered[len(filtered)-limit:]
	}
	
	return filtered
}

func (m *Manager) SetRebalanceConfig(config RebalanceConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rebalanceConfig = config
}

func (m *Manager) processTransferQueue(ctx context.Context) {
	defer m.wg.Done()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case req := <-m.transferQueue:
			m.executeTransfer(ctx, req)
		}
	}
}

func (m *Manager) executeTransfer(ctx context.Context, req *TransferRequest) {
	req.Status = TransferStatusExecuting
	now := time.Now()
	req.ExecutedAt = &now
	m.updateTransferStatus(req)
	
	exchange, exists := m.exchanges[req.Exchange]
	if !exists {
		req.Status = TransferStatusFailed
		req.ErrorMessage = fmt.Sprintf("exchange not found: %s", req.Exchange)
		m.updateTransferStatus(req)
		return
	}
	
	err := m.performTransfer(ctx, exchange, req)
	if err != nil {
		req.RetryCount++
		if req.RetryCount < 3 {
			req.Status = TransferStatusRetrying
			time.Sleep(time.Second * time.Duration(req.RetryCount))
			m.executeTransfer(ctx, req)
			return
		}
		
		req.Status = TransferStatusFailed
		req.ErrorMessage = err.Error()
	} else {
		req.Status = TransferStatusCompleted
		completed := time.Now()
		req.CompletedAt = &completed
		
		m.mu.Lock()
		m.history.SuccessCount++
		m.history.TotalVolume[req.Asset] += req.Amount
		m.mu.Unlock()
	}
	
	m.updateTransferStatus(req)
	
	if err := m.storage.WriteTransfer(req); err != nil {
		fmt.Printf("Failed to store transfer: %v\n", err)
	}
}

func (m *Manager) performTransfer(ctx context.Context, exchange types.Exchange, req *TransferRequest) error {
	switch req.Type {
	case TransferTypeMainToSub:
		txID, err := exchange.TransferToSubAccount(ctx, req.ToAccount, req.Asset, req.Amount)
		if err != nil {
			return err
		}
		req.TransactionID = txID
		
	case TransferTypeSubToMain:
		txID, err := exchange.TransferFromSubAccount(ctx, req.FromAccount, req.Asset, req.Amount)
		if err != nil {
			return err
		}
		req.TransactionID = txID
		
	case TransferTypeSubToSub:
		txID, err := exchange.TransferBetweenSubAccounts(ctx, req.FromAccount, req.ToAccount, req.Asset, req.Amount)
		if err != nil {
			return err
		}
		req.TransactionID = txID
		
	default:
		return fmt.Errorf("unsupported transfer type: %s", req.Type)
	}
	
	return nil
}

func (m *Manager) runRebalanceScheduler(ctx context.Context) {
	defer m.wg.Done()
	
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			if m.rebalanceConfig.Enabled {
				m.performRebalance(ctx)
			}
		}
	}
}

func (m *Manager) performRebalance(ctx context.Context) {
	m.mu.Lock()
	now := time.Now()
	m.history.LastRebalance = &now
	m.mu.Unlock()
	
	for exchange, exClient := range m.exchanges {
		accounts, err := m.accountCache.GetAccountsByExchange(exchange)
		if err != nil {
			continue
		}
		
		var mainAccount *types.Account
		subAccounts := make([]*types.Account, 0)
		
		for _, acc := range accounts {
			if acc.Type == types.AccountTypeMain {
				mainAccount = &acc
			} else {
				subAccounts = append(subAccounts, &acc)
			}
		}
		
		if mainAccount == nil {
			continue
		}
		
		mainBalances, err := exClient.GetBalances(ctx, mainAccount.ID)
		if err != nil {
			continue
		}
		
		for _, subAcc := range subAccounts {
			subBalances, err := exClient.GetBalances(ctx, subAcc.ID)
			if err != nil {
				continue
			}
			
			m.rebalanceAccount(ctx, exchange, mainAccount, subAcc, mainBalances, subBalances)
		}
	}
}

func (m *Manager) rebalanceAccount(
	ctx context.Context,
	exchange string,
	mainAccount, subAccount *types.Account,
	mainBalances, subBalances map[string]types.Balance,
) {
	for asset, targetRatio := range m.rebalanceConfig.TargetRatios[subAccount.Strategy] {
		mainBal := mainBalances[asset]
		subBal := subBalances[asset]
		
		totalBalance := mainBal.Available + subBal.Available
		targetAmount := totalBalance * targetRatio
		currentAmount := subBal.Available
		
		diff := targetAmount - currentAmount
		threshold := totalBalance * 0.01
		
		if diff > threshold {
			req := &TransferRequest{
				Exchange:    exchange,
				FromAccount: mainAccount.ID,
				ToAccount:   subAccount.ID,
				Asset:       asset,
				Amount:      diff,
				Type:        TransferTypeMainToSub,
				Reason:      "rebalance",
			}
			m.RequestTransfer(req)
		} else if diff < -threshold {
			req := &TransferRequest{
				Exchange:    exchange,
				FromAccount: subAccount.ID,
				ToAccount:   mainAccount.ID,
				Asset:       asset,
				Amount:      -diff,
				Type:        TransferTypeSubToMain,
				Reason:      "rebalance",
			}
			m.RequestTransfer(req)
		}
	}
}

func (m *Manager) validateTransferRequest(req *TransferRequest) error {
	if req.Exchange == "" {
		return fmt.Errorf("exchange is required")
	}
	if req.FromAccount == "" {
		return fmt.Errorf("from_account is required")
	}
	if req.ToAccount == "" {
		return fmt.Errorf("to_account is required")
	}
	if req.Asset == "" {
		return fmt.Errorf("asset is required")
	}
	if req.Amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	if req.FromAccount == req.ToAccount {
		return fmt.Errorf("from_account and to_account cannot be the same")
	}
	
	return nil
}

func (m *Manager) recordTransfer(req *TransferRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.history.Transfers = append(m.history.Transfers, *req)
	
	if len(m.history.Transfers) > 10000 {
		m.history.Transfers = m.history.Transfers[1000:]
	}
}

func (m *Manager) updateTransferStatus(req *TransferRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for i, transfer := range m.history.Transfers {
		if transfer.ID == req.ID {
			m.history.Transfers[i] = *req
			break
		}
	}
	
	if req.Status == TransferStatusFailed {
		m.history.FailureCount++
	}
}

func (m *Manager) GetRebalanceReport() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return map[string]interface{}{
		"enabled":         m.rebalanceConfig.Enabled,
		"last_rebalance":  m.history.LastRebalance,
		"success_count":   m.history.SuccessCount,
		"failure_count":   m.history.FailureCount,
		"total_volume":    m.history.TotalVolume,
		"transfer_count":  len(m.history.Transfers),
	}
}