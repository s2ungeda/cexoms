package transfer

import (
	"context"
	"fmt"
	"time"

	"github.com/mExOms/pkg/types"
)

type Validator struct {
	manager *Manager
}

func NewValidator(manager *Manager) *Validator {
	return &Validator{
		manager: manager,
	}
}

func (v *Validator) ValidateTransfer(ctx context.Context, req *TransferRequest) error {
	if err := v.validateBasicRequirements(req); err != nil {
		return err
	}
	
	if err := v.validateAccounts(ctx, req); err != nil {
		return err
	}
	
	if err := v.validateBalance(ctx, req); err != nil {
		return err
	}
	
	if err := v.validateLimits(req); err != nil {
		return err
	}
	
	if err := v.validateTransferRules(req); err != nil {
		return err
	}
	
	return nil
}

func (v *Validator) validateBasicRequirements(req *TransferRequest) error {
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
		return fmt.Errorf("amount must be positive: %.8f", req.Amount)
	}
	
	if req.FromAccount == req.ToAccount {
		return fmt.Errorf("cannot transfer to the same account")
	}
	
	return nil
}

func (v *Validator) validateAccounts(ctx context.Context, req *TransferRequest) error {
	fromAccount, err := v.manager.accountCache.GetAccount(req.Exchange, req.FromAccount)
	if err != nil {
		return fmt.Errorf("from_account not found: %w", err)
	}
	
	toAccount, err := v.manager.accountCache.GetAccount(req.Exchange, req.ToAccount)
	if err != nil {
		return fmt.Errorf("to_account not found: %w", err)
	}
	
	if fromAccount.Status != types.AccountStatusActive {
		return fmt.Errorf("from_account is not active: %s", fromAccount.Status)
	}
	
	if toAccount.Status != types.AccountStatusActive {
		return fmt.Errorf("to_account is not active: %s", toAccount.Status)
	}
	
	if err := v.validateTransferType(req, fromAccount, toAccount); err != nil {
		return err
	}
	
	return nil
}

func (v *Validator) validateTransferType(req *TransferRequest, from, to *types.Account) error {
	switch req.Type {
	case TransferTypeMainToSub:
		if from.Type != types.AccountTypeMain {
			return fmt.Errorf("from_account must be main account for MAIN_TO_SUB transfer")
		}
		if to.Type != types.AccountTypeSub {
			return fmt.Errorf("to_account must be sub account for MAIN_TO_SUB transfer")
		}
		
	case TransferTypeSubToMain:
		if from.Type != types.AccountTypeSub {
			return fmt.Errorf("from_account must be sub account for SUB_TO_MAIN transfer")
		}
		if to.Type != types.AccountTypeMain {
			return fmt.Errorf("to_account must be main account for SUB_TO_MAIN transfer")
		}
		
	case TransferTypeSubToSub:
		if from.Type != types.AccountTypeSub {
			return fmt.Errorf("from_account must be sub account for SUB_TO_SUB transfer")
		}
		if to.Type != types.AccountTypeSub {
			return fmt.Errorf("to_account must be sub account for SUB_TO_SUB transfer")
		}
		
	default:
		return fmt.Errorf("invalid transfer type: %s", req.Type)
	}
	
	return nil
}

func (v *Validator) validateBalance(ctx context.Context, req *TransferRequest) error {
	exchange, exists := v.manager.exchanges[req.Exchange]
	if !exists {
		return fmt.Errorf("exchange not found: %s", req.Exchange)
	}
	
	balances, err := exchange.GetBalances(ctx, req.FromAccount)
	if err != nil {
		return fmt.Errorf("failed to get balances: %w", err)
	}
	
	balance, exists := balances[req.Asset]
	if !exists {
		return fmt.Errorf("asset not found in from_account: %s", req.Asset)
	}
	
	if balance.Available < req.Amount {
		return fmt.Errorf("insufficient balance: available=%.8f, requested=%.8f",
			balance.Available, req.Amount)
	}
	
	minAmount := v.getMinTransferAmount(req.Asset)
	if req.Amount < minAmount {
		return fmt.Errorf("amount below minimum: min=%.8f, requested=%.8f",
			minAmount, req.Amount)
	}
	
	return nil
}

func (v *Validator) validateLimits(req *TransferRequest) error {
	fromAccount, _ := v.manager.accountCache.GetAccount(req.Exchange, req.FromAccount)
	toAccount, _ := v.manager.accountCache.GetAccount(req.Exchange, req.ToAccount)
	
	if req.Type == TransferTypeMainToSub || req.Type == TransferTypeSubToSub {
		maxBalance, exists := v.manager.rebalanceConfig.MaxSubBalance[req.Asset]
		if exists {
			currentBalance := v.getAccountBalance(req.Exchange, req.ToAccount, req.Asset)
			if currentBalance+req.Amount > maxBalance {
				return fmt.Errorf("transfer would exceed max sub account balance: max=%.8f, would be=%.8f",
					maxBalance, currentBalance+req.Amount)
			}
		}
	}
	
	if req.Type == TransferTypeSubToMain || req.Type == TransferTypeSubToSub {
		if fromAccount.Type == types.AccountTypeSub {
			minBalance := v.getMinAccountBalance(fromAccount.Strategy, req.Asset)
			currentBalance := v.getAccountBalance(req.Exchange, req.FromAccount, req.Asset)
			if currentBalance-req.Amount < minBalance {
				return fmt.Errorf("transfer would leave balance below minimum: min=%.8f, would be=%.8f",
					minBalance, currentBalance-req.Amount)
			}
		}
	}
	
	return nil
}

func (v *Validator) validateTransferRules(req *TransferRequest) error {
	history := v.manager.GetTransferHistory(req.Exchange, req.FromAccount, 100)
	
	recentTransfers := 0
	recentVolume := 0.0
	cutoff := time.Now().Add(-time.Hour)
	
	for _, transfer := range history {
		if transfer.CreatedAt.After(cutoff) && transfer.Asset == req.Asset {
			recentTransfers++
			recentVolume += transfer.Amount
		}
	}
	
	if recentTransfers >= 10 {
		return fmt.Errorf("too many transfers in the last hour: %d", recentTransfers)
	}
	
	maxHourlyVolume := v.getMaxHourlyVolume(req.Asset)
	if recentVolume+req.Amount > maxHourlyVolume {
		return fmt.Errorf("hourly volume limit exceeded: max=%.8f, would be=%.8f",
			maxHourlyVolume, recentVolume+req.Amount)
	}
	
	return nil
}

func (v *Validator) getMinTransferAmount(asset string) float64 {
	minAmounts := map[string]float64{
		"BTC":  0.0001,
		"ETH":  0.001,
		"USDT": 10.0,
		"BNB":  0.01,
	}
	
	if min, exists := minAmounts[asset]; exists {
		return min
	}
	
	return 1.0
}

func (v *Validator) getMinAccountBalance(strategy, asset string) float64 {
	if strategy == "" {
		return 0
	}
	
	strategyMinBalances := map[string]map[string]float64{
		"arbitrage": {
			"USDT": 100.0,
			"BTC":  0.001,
			"ETH":  0.01,
		},
		"market_making": {
			"USDT": 500.0,
			"BTC":  0.01,
			"ETH":  0.1,
		},
		"trend_following": {
			"USDT": 1000.0,
			"BTC":  0.02,
			"ETH":  0.2,
		},
	}
	
	if balances, exists := strategyMinBalances[strategy]; exists {
		if min, exists := balances[asset]; exists {
			return min
		}
	}
	
	return 0
}

func (v *Validator) getMaxHourlyVolume(asset string) float64 {
	maxVolumes := map[string]float64{
		"BTC":  1.0,
		"ETH":  10.0,
		"USDT": 100000.0,
		"BNB":  5.0,
	}
	
	if max, exists := maxVolumes[asset]; exists {
		return max
	}
	
	return 10000.0
}

func (v *Validator) getAccountBalance(exchange, accountID, asset string) float64 {
	ctx := context.Background()
	exClient, exists := v.manager.exchanges[exchange]
	if !exists {
		return 0
	}
	
	balances, err := exClient.GetBalances(ctx, accountID)
	if err != nil {
		return 0
	}
	
	if balance, exists := balances[asset]; exists {
		return balance.Available
	}
	
	return 0
}