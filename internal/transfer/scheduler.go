package transfer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/mExOms/pkg/types"
)

type ScheduledTask struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Schedule    string    `json:"schedule"`
	Enabled     bool      `json:"enabled"`
	LastRun     time.Time `json:"last_run"`
	NextRun     time.Time `json:"next_run"`
	RunCount    int       `json:"run_count"`
	ErrorCount  int       `json:"error_count"`
	taskFunc    func(context.Context) error
}

type Scheduler struct {
	mu       sync.RWMutex
	cron     *cron.Cron
	tasks    map[string]*ScheduledTask
	manager  *Manager
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

func NewScheduler(manager *Manager) *Scheduler {
	return &Scheduler{
		cron:    cron.New(cron.WithSeconds()),
		tasks:   make(map[string]*ScheduledTask),
		manager: manager,
		stopCh:  make(chan struct{}),
	}
}

func (s *Scheduler) Start(ctx context.Context) error {
	s.registerDefaultTasks()
	
	s.cron.Start()
	
	s.wg.Add(1)
	go s.monitorTasks(ctx)
	
	return nil
}

func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.cron.Stop()
	s.wg.Wait()
}

func (s *Scheduler) RegisterTask(task *ScheduledTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if _, exists := s.tasks[task.ID]; exists {
		return fmt.Errorf("task already exists: %s", task.ID)
	}
	
	entryID, err := s.cron.AddFunc(task.Schedule, func() {
		ctx := context.Background()
		s.executeTask(ctx, task)
	})
	
	if err != nil {
		return fmt.Errorf("failed to add cron job: %w", err)
	}
	
	entry := s.cron.Entry(entryID)
	task.NextRun = entry.Next
	
	s.tasks[task.ID] = task
	return nil
}

func (s *Scheduler) UnregisterTask(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	task, exists := s.tasks[taskID]
	if !exists {
		return fmt.Errorf("task not found: %s", taskID)
	}
	
	task.Enabled = false
	delete(s.tasks, taskID)
	
	return nil
}

func (s *Scheduler) EnableTask(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	task, exists := s.tasks[taskID]
	if !exists {
		return fmt.Errorf("task not found: %s", taskID)
	}
	
	task.Enabled = true
	return nil
}

func (s *Scheduler) DisableTask(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	task, exists := s.tasks[taskID]
	if !exists {
		return fmt.Errorf("task not found: %s", taskID)
	}
	
	task.Enabled = false
	return nil
}

func (s *Scheduler) GetTaskStatus(taskID string) (*ScheduledTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	task, exists := s.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	
	return task, nil
}

func (s *Scheduler) GetAllTasks() map[string]*ScheduledTask {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	tasks := make(map[string]*ScheduledTask)
	for id, task := range s.tasks {
		tasks[id] = task
	}
	
	return tasks
}

func (s *Scheduler) registerDefaultTasks() {
	s.RegisterTask(&ScheduledTask{
		ID:       "daily_rebalance",
		Name:     "Daily Account Rebalance",
		Schedule: "0 0 0 * * *", // Daily at midnight
		Enabled:  true,
		taskFunc: s.dailyRebalanceTask,
	})
	
	s.RegisterTask(&ScheduledTask{
		ID:       "hourly_rebalance",
		Name:     "Hourly Account Rebalance",
		Schedule: "0 0 * * * *", // Every hour
		Enabled:  false,
		taskFunc: s.hourlyRebalanceTask,
	})
	
	s.RegisterTask(&ScheduledTask{
		ID:       "profit_collection",
		Name:     "Daily Profit Collection",
		Schedule: "0 0 2 * * *", // Daily at 2 AM
		Enabled:  true,
		taskFunc: s.profitCollectionTask,
	})
	
	s.RegisterTask(&ScheduledTask{
		ID:       "min_balance_check",
		Name:     "Minimum Balance Check",
		Schedule: "0 */15 * * * *", // Every 15 minutes
		Enabled:  true,
		taskFunc: s.minBalanceCheckTask,
	})
}

func (s *Scheduler) executeTask(ctx context.Context, task *ScheduledTask) {
	if !task.Enabled {
		return
	}
	
	s.mu.Lock()
	task.LastRun = time.Now()
	task.RunCount++
	s.mu.Unlock()
	
	err := task.taskFunc(ctx)
	if err != nil {
		s.mu.Lock()
		task.ErrorCount++
		s.mu.Unlock()
		fmt.Printf("Task %s failed: %v\n", task.ID, err)
	}
	
	entries := s.cron.Entries()
	for _, entry := range entries {
		s.mu.Lock()
		task.NextRun = entry.Next
		s.mu.Unlock()
		break
	}
}

func (s *Scheduler) monitorTasks(ctx context.Context) {
	defer s.wg.Done()
	
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.checkTaskHealth()
		}
	}
}

func (s *Scheduler) checkTaskHealth() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	for _, task := range s.tasks {
		if task.Enabled && task.ErrorCount > 10 {
			fmt.Printf("Task %s has high error count: %d\n", task.ID, task.ErrorCount)
		}
	}
}

func (s *Scheduler) dailyRebalanceTask(ctx context.Context) error {
	s.manager.performRebalance(ctx)
	return nil
}

func (s *Scheduler) hourlyRebalanceTask(ctx context.Context) error {
	s.manager.performRebalance(ctx)
	return nil
}

func (s *Scheduler) profitCollectionTask(ctx context.Context) error {
	for exchange, exClient := range s.manager.exchanges {
		accounts, err := s.manager.accountCache.GetAccountsByExchange(exchange)
		if err != nil {
			continue
		}
		
		var mainAccount *types.Account
		subAccounts := make([]*types.Account, 0)
		
		for _, acc := range accounts {
			if acc.Type == types.AccountTypeMain {
				mainAccount = &acc
			} else if acc.Type == types.AccountTypeSub {
				subAccounts = append(subAccounts, &acc)
			}
		}
		
		if mainAccount == nil {
			continue
		}
		
		for _, subAcc := range subAccounts {
			balances, err := exClient.GetBalances(ctx, subAcc.ID)
			if err != nil {
				continue
			}
			
			for asset, balance := range balances {
				maxBalance, exists := s.manager.rebalanceConfig.MaxSubBalance[asset]
				if !exists {
					continue
				}
				
				if balance.Available > maxBalance {
					excess := balance.Available - maxBalance
					req := &TransferRequest{
						Exchange:    exchange,
						FromAccount: subAcc.ID,
						ToAccount:   mainAccount.ID,
						Asset:       asset,
						Amount:      excess,
						Type:        TransferTypeSubToMain,
						Reason:      "profit_collection",
					}
					s.manager.RequestTransfer(req)
				}
			}
		}
	}
	
	return nil
}

func (s *Scheduler) minBalanceCheckTask(ctx context.Context) error {
	for exchange, exClient := range s.manager.exchanges {
		accounts, err := s.manager.accountCache.GetAccountsByExchange(exchange)
		if err != nil {
			continue
		}
		
		var mainAccount *types.Account
		
		for _, acc := range accounts {
			if acc.Type == types.AccountTypeMain {
				mainAccount = &acc
				break
			}
		}
		
		if mainAccount == nil {
			continue
		}
		
		balances, err := exClient.GetBalances(ctx, mainAccount.ID)
		if err != nil {
			continue
		}
		
		for asset, balance := range balances {
			minBalance, exists := s.manager.rebalanceConfig.MinMainBalance[asset]
			if !exists {
				continue
			}
			
			if balance.Available < minBalance {
				fmt.Printf("WARNING: Main account balance for %s is below minimum: %.8f < %.8f\n",
					asset, balance.Available, minBalance)
			}
		}
	}
	
	return nil
}