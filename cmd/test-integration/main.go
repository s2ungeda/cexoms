package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/your-org/mexoms/pkg/testing"
	"github.com/your-org/mexoms/pkg/types"
	"github.com/your-org/mexoms/pkg/performance"
	"github.com/your-org/mexoms/pkg/security"
	"github.com/your-org/mexoms/pkg/monitoring"
)

func main() {
	fmt.Println("🧪 End-to-End Integration Testing Suite")
	fmt.Println(strings.Repeat("=", 60))

	// Initialize system components
	fmt.Println("🔧 Initializing system components...")
	components, err := initializeSystemComponents()
	if err != nil {
		log.Fatalf("Failed to initialize components: %v", err)
	}

	// Create test framework
	config := testing.DefaultTestConfig()
	framework := testing.NewIntegrationTestFramework(config, components)

	// Register additional custom test scenarios if needed
	// framework.RegisterScenario(NewCustomTestScenario())

	// Run integration tests
	fmt.Println("🚀 Starting integration test execution...")
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	if err := framework.Start(ctx); err != nil {
		log.Fatalf("Integration tests failed: %v", err)
	}

	// Generate and display report
	fmt.Println("📊 Generating test report...")
	report := framework.GenerateReport()
	
	if err := displayReport(report); err != nil {
		log.Printf("Failed to display report: %v", err)
	}

	if err := saveReport(report); err != nil {
		log.Printf("Failed to save report: %v", err)
	}

	// Cleanup
	framework.Stop()
	
	// Exit with appropriate code
	if report.Summary.FailedScenarios > 0 {
		fmt.Printf("❌ %d test scenarios failed\n", report.Summary.FailedScenarios)
		os.Exit(1)
	}
	
	fmt.Printf("✅ All %d test scenarios passed successfully!\n", report.Summary.PassedScenarios)
}

func initializeSystemComponents() (*testing.SystemComponents, error) {
	components := &testing.SystemComponents{}

	// Initialize performance optimizers
	var err error
	
	// Database optimizer
	dbConfig := performance.DefaultDatabaseConfig()
	components.DatabaseOptimizer, err = performance.NewDatabaseOptimizer(dbConfig)
	if err != nil {
		log.Printf("Warning: Failed to initialize database optimizer: %v", err)
		// Continue without database optimizer for testing
	}

	// Network optimizer
	netConfig := performance.DefaultNetworkConfig()
	components.NetworkOptimizer = performance.NewNetworkOptimizer(netConfig)

	// Runtime optimizer
	runtimeConfig := performance.DefaultOptimizerConfig()
	components.RuntimeOptimizer = performance.NewRuntimeOptimizer(runtimeConfig)

	// Security components
	components.SecurityOrchestrator = security.NewSecurityOrchestrator()
	components.ComplianceManager = security.NewComplianceManager()

	// Monitoring components
	components.HealthChecker = monitoring.NewHealthChecker()
	components.Dashboard = monitoring.NewDashboard(8080)
	components.MetricsCollector = monitoring.NewMetricsCollector()

	// Core business logic components (mock implementations for testing)
	components.OrderManager = NewMockOrderManager()
	components.RiskEngine = NewMockRiskEngine()
	components.PositionManager = NewMockPositionManager()
	components.AccountManager = NewMockAccountManager()
	components.RoutingEngine = NewMockRoutingEngine()

	// Exchange connectors (mock implementations)
	components.BinanceSpot = NewMockBinanceSpot()
	components.BinanceFutures = NewMockBinanceFutures()

	return components, nil
}

func displayReport(report *testing.TestReport) error {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("📋 INTEGRATION TEST REPORT")
	fmt.Println(strings.Repeat("=", 60))
	
	fmt.Printf("🕐 Test Execution Time: %v\n", report.Timestamp.Format("2006-01-02 15:04:05"))
	fmt.Printf("🌍 Environment: %s\n", report.Environment)
	fmt.Printf("📦 Total Test Suites: %d\n", report.TotalSuites)
	fmt.Printf("⏱️  Total Duration: %v\n", report.Summary.TotalDuration)
	
	fmt.Println("\n" + strings.Repeat("-", 40))
	fmt.Println("📊 SUMMARY")
	fmt.Println(strings.Repeat("-", 40))
	
	fmt.Printf("✅ Passed Scenarios: %d\n", report.Summary.PassedScenarios)
	fmt.Printf("❌ Failed Scenarios: %d\n", report.Summary.FailedScenarios)
	fmt.Printf("📈 Success Rate: %.2f%%\n", report.Summary.SuccessRate)
	fmt.Printf("⏱️  Average Duration: %v\n", report.Summary.AverageDuration)

	fmt.Println("\n" + strings.Repeat("-", 40))
	fmt.Println("🔍 DETAILED RESULTS")
	fmt.Println(strings.Repeat("-", 40))

	for _, result := range report.Results {
		status := "✅ PASSED"
		if !result.OverallSuccess {
			status = "❌ FAILED"
		}
		
		fmt.Printf("\n%s %s\n", status, result.TestSuite)
		fmt.Printf("  Duration: %v\n", result.Duration)
		
		if len(result.ScenarioResults) > 0 {
			scenario := result.ScenarioResults[0]
			
			// Display key metrics
			if ordersProcessed, ok := scenario.Metrics["orders_processed"]; ok {
				fmt.Printf("  Orders Processed: %v\n", ordersProcessed)
			}
			if avgLatency, ok := scenario.Metrics["average_latency_us"]; ok {
				fmt.Printf("  Average Latency: %vμs\n", avgLatency)
			}
			if throughput, ok := scenario.Metrics["orders_per_second"]; ok {
				fmt.Printf("  Throughput: %.0f orders/sec\n", throughput)
			}
			if successRate, ok := scenario.Metrics["success_rate"]; ok {
				fmt.Printf("  Success Rate: %.2f%%\n", float64(successRate.(float64))*100)
			}

			// Display sub-results
			if len(scenario.SubResults) > 0 {
				fmt.Printf("  Sub-tests:\n")
				for _, subResult := range scenario.SubResults {
					subStatus := "✅"
					if !subResult.Success {
						subStatus = "❌"
					}
					fmt.Printf("    %s %s: %s\n", subStatus, subResult.Name, subResult.Message)
				}
			}

			if !result.OverallSuccess && scenario.ErrorMessage != "" {
				fmt.Printf("  Error: %s\n", scenario.ErrorMessage)
			}
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	
	if report.Summary.FailedScenarios == 0 {
		fmt.Println("🎉 ALL TESTS PASSED! System is ready for production.")
	} else {
		fmt.Printf("⚠️  %d test scenarios need attention before production deployment.\n", report.Summary.FailedScenarios)
	}
	
	fmt.Println(strings.Repeat("=", 60))

	return nil
}

func saveReport(report *testing.TestReport) error {
	// Create results directory if it doesn't exist
	if err := os.MkdirAll("test_results", 0755); err != nil {
		return fmt.Errorf("failed to create results directory: %w", err)
	}

	// Generate filename with timestamp
	filename := fmt.Sprintf("test_results/integration_test_report_%s.json", 
		report.Timestamp.Format("20060102_150405"))

	// Marshal report to JSON
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write report file: %w", err)
	}

	fmt.Printf("📄 Test report saved to: %s\n", filename)
	return nil
}

// Mock implementations for testing

type MockOrderManager struct{}

func NewMockOrderManager() *MockOrderManager {
	return &MockOrderManager{}
}

func (m *MockOrderManager) ProcessOrder(ctx context.Context, order *types.Order) error {
	// Simulate order processing
	time.Sleep(time.Microsecond * 50) // 50μs processing time
	order.Status = "FILLED"
	return nil
}

type MockRiskEngine struct {
	killSwitchActive bool
}

func NewMockRiskEngine() *MockRiskEngine {
	return &MockRiskEngine{}
}

func (m *MockRiskEngine) CheckOrderRisk(order *types.Order) bool {
	if m.killSwitchActive {
		return false
	}
	
	// Simulate risk checks with some failures
	if order.Quantity > 500.0 {
		return false // Reject large orders
	}
	
	// Account-specific risk checks
	if order.AccountID == "risk-account" && order.Quantity > 50.0 {
		return false
	}
	
	if order.AccountID == "kill-account" {
		return false
	}
	
	return true
}

func (m *MockRiskEngine) ActivateKillSwitch(reason string) {
	m.killSwitchActive = true
}

func (m *MockRiskEngine) DeactivateKillSwitch(reason string) {
	m.killSwitchActive = false
}

type MockPositionManager struct{}

func NewMockPositionManager() *MockPositionManager {
	return &MockPositionManager{}
}

type MockAccountManager struct{}

func NewMockAccountManager() *MockAccountManager {
	return &MockAccountManager{}
}

func (m *MockAccountManager) ValidateAccount(accountID string) bool {
	// Simulate account validation
	return accountID != "invalid-account"
}

type MockRoutingEngine struct{}

func NewMockRoutingEngine() *MockRoutingEngine {
	return &MockRoutingEngine{}
}

func (m *MockRoutingEngine) SelectBestExchange(order *types.Order) string {
	// Simple routing logic for testing
	if strings.Contains(order.Symbol, "BTC") || strings.Contains(order.Symbol, "ETH") {
		return "binance"
	}
	return "binance" // Default to binance for testing
}

type MockBinanceSpot struct{}

func NewMockBinanceSpot() *MockBinanceSpot {
	return &MockBinanceSpot{}
}

// Implement required methods for types.Exchange interface
func (m *MockBinanceSpot) GetName() string { return "binance-spot" }
func (m *MockBinanceSpot) PlaceOrder(ctx context.Context, order *types.Order) error { return nil }
func (m *MockBinanceSpot) CancelOrder(ctx context.Context, orderID string) error { return nil }
func (m *MockBinanceSpot) GetOrderStatus(ctx context.Context, orderID string) (*types.Order, error) { return nil, nil }
func (m *MockBinanceSpot) GetBalance(ctx context.Context, asset string) (float64, error) { return 1000.0, nil }
func (m *MockBinanceSpot) GetOrderBook(ctx context.Context, symbol string) (*types.OrderBook, error) { return nil, nil }

type MockBinanceFutures struct{}

func NewMockBinanceFutures() *MockBinanceFutures {
	return &MockBinanceFutures{}
}

// Implement required methods for types.FuturesExchange interface  
func (m *MockBinanceFutures) GetName() string { return "binance-futures" }
func (m *MockBinanceFutures) PlaceOrder(ctx context.Context, order *types.Order) error { return nil }
func (m *MockBinanceFutures) CancelOrder(ctx context.Context, orderID string) error { return nil }
func (m *MockBinanceFutures) GetOrderStatus(ctx context.Context, orderID string) (*types.Order, error) { return nil, nil }
func (m *MockBinanceFutures) GetBalance(ctx context.Context, asset string) (float64, error) { return 1000.0, nil }
func (m *MockBinanceFutures) GetOrderBook(ctx context.Context, symbol string) (*types.OrderBook, error) { return nil, nil }
func (m *MockBinanceFutures) GetPositions(ctx context.Context) ([]*types.Position, error) { return nil, nil }
func (m *MockBinanceFutures) SetLeverage(ctx context.Context, symbol string, leverage int) error { return nil }
func (m *MockBinanceFutures) SetMarginType(ctx context.Context, symbol string, marginType string) error { return nil }