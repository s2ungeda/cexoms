package orchestrator

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// AllocationMethod defines how capital is allocated
type AllocationMethod string

const (
	AllocationEqual       AllocationMethod = "equal"         // Equal allocation
	AllocationRiskParity  AllocationMethod = "risk_parity"   // Risk-adjusted allocation
	AllocationPerformance AllocationMethod = "performance"   // Performance-based allocation
	AllocationKelly       AllocationMethod = "kelly"         // Kelly criterion
	AllocationCustom      AllocationMethod = "custom"        // Custom allocation
)

// CapitalAllocation represents capital allocated to a strategy
type CapitalAllocation struct {
	StrategyID       string
	AllocatedAmount  float64
	UsedAmount       float64
	ReservedAmount   float64
	LastUpdate       time.Time
	Performance      AllocationPerformance
	RiskMetrics      RiskMetrics
}

// AllocationPerformance tracks performance for allocation decisions
type AllocationPerformance struct {
	TotalReturn      float64
	SharpeRatio      float64
	MaxDrawdown      float64
	WinRate          float64
	ProfitFactor     float64
	RecoveryFactor   float64
	LastUpdate       time.Time
}

// RiskMetrics contains risk measurements for allocation
type RiskMetrics struct {
	VaR95            float64 // Value at Risk at 95% confidence
	CVaR95           float64 // Conditional VaR at 95%
	Volatility       float64
	Beta             float64
	Correlation      float64
	MaxLeverage      float64
	CurrentLeverage  float64
}

// AllocationConfig contains configuration for capital allocation
type AllocationConfig struct {
	Method              AllocationMethod
	TotalCapital        float64
	MinAllocation       float64
	MaxAllocation       float64
	RebalanceInterval   time.Duration
	RiskLimit           float64
	UseKellyCriterion   bool
	KellyFraction       float64 // Fraction of Kelly criterion to use (e.g., 0.25)
	DynamicRebalancing  bool
}

// CapitalAllocator manages capital allocation across strategies
type CapitalAllocator struct {
	config          AllocationConfig
	allocations     map[string]*CapitalAllocation
	totalAllocated  float64
	availableCapital float64
	performanceHistory map[string][]AllocationPerformance
	mu              sync.RWMutex
	lastRebalance   time.Time
}

// NewCapitalAllocator creates a new capital allocator
func NewCapitalAllocator() *CapitalAllocator {
	return &CapitalAllocator{
		config: AllocationConfig{
			Method:            AllocationRiskParity,
			TotalCapital:      1000000, // Default $1M
			MinAllocation:     10000,    // Min $10k per strategy
			MaxAllocation:     200000,   // Max $200k per strategy
			RebalanceInterval: 24 * time.Hour,
			RiskLimit:         0.2,      // 20% max risk
			UseKellyCriterion: true,
			KellyFraction:     0.25,     // Conservative Kelly
			DynamicRebalancing: true,
		},
		allocations:        make(map[string]*CapitalAllocation),
		performanceHistory: make(map[string][]AllocationPerformance),
		lastRebalance:      time.Now(),
		availableCapital:   1000000, // Initialize with total capital
	}
}
EOF < /dev/null
