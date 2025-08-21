package exchange

import (
	"fmt"
	"sync"

	"github.com/mExOms/pkg/types"
)

// Manager manages multiple exchange connections
type Manager struct {
	mu        sync.RWMutex
	exchanges map[string]types.Exchange
}

// NewManager creates a new exchange manager
func NewManager() *Manager {
	return &Manager{
		exchanges: make(map[string]types.Exchange),
	}
}

// AddExchange adds an exchange to the manager
func (m *Manager) AddExchange(name string, exchange types.Exchange) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.exchanges[name]; exists {
		return fmt.Errorf("exchange %s already exists", name)
	}
	
	m.exchanges[name] = exchange
	return nil
}

// GetExchange gets an exchange by name
func (m *Manager) GetExchange(name string) (types.Exchange, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	exchange, exists := m.exchanges[name]
	if !exists {
		return nil, fmt.Errorf("exchange %s not found", name)
	}
	
	return exchange, nil
}

// ListExchanges returns a list of all exchange names
func (m *Manager) ListExchanges() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	names := make([]string, 0, len(m.exchanges))
	for name := range m.exchanges {
		names = append(names, name)
	}
	
	return names
}

// RemoveExchange removes an exchange from the manager
func (m *Manager) RemoveExchange(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.exchanges[name]; !exists {
		return fmt.Errorf("exchange %s not found", name)
	}
	
	delete(m.exchanges, name)
	return nil
}