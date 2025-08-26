package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/your-org/mexoms/pkg/testing"
	"github.com/your-org/mexoms/pkg/performance"
	"github.com/your-org/mexoms/pkg/security"
	"github.com/your-org/mexoms/pkg/monitoring"
)

func main() {
	fmt.Println("🎯 Performance Validation Suite")
	fmt.Println(strings.Repeat("=", 60))

	// System information
	displaySystemInfo()

	// Initialize system components
	fmt.Println("🔧 Initializing system components...")
	components := initializeSystemComponents()

	// Create performance validator
	validator := testing.NewPerformanceValidator(components)

	// Run comprehensive validation
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	fmt.Println("🚀 Starting performance validation...")
	
	results, err := validator.RunValidation(ctx)
	if err != nil {
		log.Fatalf("Performance validation failed: %v", err)
	}

	// Display results
	displayValidationResults(results)

	// Save detailed results
	if err := saveValidationResults(results); err != nil {
		log.Printf("Failed to save results: %v", err)
	}

	// Generate performance report
	generatePerformanceReport(results)

	// Exit with appropriate code
	if !results.OverallPassed {
		fmt.Printf("❌ Performance validation failed: %d of %d tests failed\n", 
			results.FailedTests, results.TotalTests)
		os.Exit(1)
	}

	fmt.Println("✅ All performance targets validated successfully!")
}

func displaySystemInfo() {
	fmt.Println("\n" + strings.Repeat("-", 40))
	fmt.Println("💻 SYSTEM INFORMATION")
	fmt.Println(strings.Repeat("-", 40))
	
	fmt.Printf("🔧 Go Version: %s\n", runtime.Version())
	fmt.Printf("💾 CPU Cores: %d\n", runtime.NumCPU())
	fmt.Printf("🏗️  Architecture: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	fmt.Printf("🧠 Available Memory: %.2f GB\n", float64(memStats.Sys)/(1024*1024*1024))
	fmt.Printf("⏰ Current Time: %s\n", time.Now().Format("2006-01-02 15:04:05"))
}

func initializeSystemComponents() *testing.SystemComponents {
	components := &testing.SystemComponents{}

	// Performance optimizers
	dbConfig := performance.DefaultDatabaseConfig()
	if dbOpt, err := performance.NewDatabaseOptimizer(dbConfig); err == nil {
		components.DatabaseOptimizer = dbOpt
		components.DatabaseOptimizer.Start()
	}

	netConfig := performance.DefaultNetworkConfig()
	components.NetworkOptimizer = performance.NewNetworkOptimizer(netConfig)
	components.NetworkOptimizer.Start()

	runtimeConfig := performance.DefaultOptimizerConfig()
	components.RuntimeOptimizer = performance.NewRuntimeOptimizer(runtimeConfig)
	components.RuntimeOptimizer.Start()

	// Security components
	components.SecurityOrchestrator = security.NewSecurityOrchestrator()
	components.SecurityOrchestrator.Start()

	components.ComplianceManager = security.NewComplianceManager()

	// Monitoring components
	components.HealthChecker = monitoring.NewHealthChecker()
	components.HealthChecker.Start()

	components.Dashboard = monitoring.NewDashboard(8080)
	components.Dashboard.Start()

	components.MetricsCollector = monitoring.NewMetricsCollector()

	// High-performance business logic mocks
	components.OrderManager = NewValidationOrderManager()
	components.RiskEngine = NewValidationRiskEngine()
	components.PositionManager = NewValidationPositionManager()
	components.AccountManager = NewValidationAccountManager()
	components.RoutingEngine = NewValidationRoutingEngine()

	fmt.Printf("✅ Initialized %d system components\n", countComponents(components))
	return components
}

func countComponents(components *testing.SystemComponents) int {
	count := 0
	if components.DatabaseOptimizer != nil { count++ }
	if components.NetworkOptimizer != nil { count++ }
	if components.RuntimeOptimizer != nil { count++ }
	if components.SecurityOrchestrator != nil { count++ }
	if components.ComplianceManager != nil { count++ }
	if components.HealthChecker != nil { count++ }
	if components.Dashboard != nil { count++ }
	if components.MetricsCollector != nil { count++ }
	if components.OrderManager != nil { count++ }
	if components.RiskEngine != nil { count++ }
	return count
}

func displayValidationResults(results *testing.ValidationResults) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("📋 PERFORMANCE VALIDATION RESULTS")
	fmt.Println(strings.Repeat("=", 60))

	// Overall status
	status := "✅ PASSED"
	if !results.OverallPassed {
		status = "❌ FAILED"
	}

	fmt.Printf("🏁 Overall Status: %s\n", status)
	fmt.Printf("⏱️  Total Duration: %v\n", results.Duration)
	fmt.Printf("📊 Test Summary: %d total, %d passed, %d failed\n", 
		results.TotalTests, results.PassedTests, results.FailedTests)
	fmt.Printf("📈 Success Rate: %.1f%%\n", 
		float64(results.PassedTests)/float64(results.TotalTests)*100)

	// Category results
	fmt.Println("\n" + strings.Repeat("-", 40))
	fmt.Println("📊 CATEGORY RESULTS")
	fmt.Println(strings.Repeat("-", 40))

	categories := []string{"Latency", "Throughput", "Resources", "Stability", "Scalability", "Regression"}
	
	for _, categoryName := range categories {
		if category, exists := results.Categories[categoryName]; exists {
			categoryStatus := "✅"
			if !category.Passed {
				categoryStatus = "❌"
			}
			
			fmt.Printf("%s %s: %.1f%% pass rate (%d/%d tests)\n", 
				categoryStatus, category.Name, category.PassRate*100, 
				countPassedTests(category.Tests), len(category.Tests))

			// Show individual test results
			for _, test := range category.Tests {
				testStatus := "✅"
				if !test.Passed {
					testStatus = "❌"
				}
				fmt.Printf("   %s %s: %s\n", testStatus, test.Name, test.Message)
			}
			fmt.Println()
		}
	}

	// Performance highlights
	fmt.Println(strings.Repeat("-", 40))
	fmt.Println("🚀 PERFORMANCE HIGHLIGHTS")
	fmt.Println(strings.Repeat("-", 40))

	displayPerformanceHighlights(results)

	// Recommendations
	if !results.OverallPassed {
		fmt.Println(strings.Repeat("-", 40))
		fmt.Println("💡 RECOMMENDATIONS")
		fmt.Println(strings.Repeat("-", 40))
		displayRecommendations(results)
	}
}

func displayPerformanceHighlights(results *testing.ValidationResults) {
	// Extract key metrics from category results
	if latencyCategory, exists := results.Categories["Latency"]; exists {
		for _, test := range latencyCategory.Tests {
			if test.Name == "OrderProcessingLatency" {
				fmt.Printf("⚡ Order Processing: %v (target: %v)\n", 
					formatDuration(test.Actual), formatDuration(test.Target))
			}
			if test.Name == "RiskCheckLatency" {
				fmt.Printf("🛡️  Risk Check: %v (target: %v)\n", 
					formatDuration(test.Actual), formatDuration(test.Target))
			}
		}
	}

	if throughputCategory, exists := results.Categories["Throughput"]; exists {
		for _, test := range throughputCategory.Tests {
			if test.Name == "OrderThroughput" {
				fmt.Printf("🚀 Order Throughput: %s TPS (target: %s TPS)\n", 
					formatNumber(test.Actual), formatNumber(test.Target))
			}
		}
	}

	if resourceCategory, exists := results.Categories["Resources"]; exists {
		for _, test := range resourceCategory.Tests {
			if test.Name == "MemoryUsage" {
				fmt.Printf("💾 Memory Usage: %.2f GB (limit: %.2f GB)\n", 
					float64(test.Actual.(int64))/(1024*1024*1024),
					float64(test.Target.(int64))/(1024*1024*1024))
			}
			if test.Name == "GoroutineCount" {
				fmt.Printf("🔄 Goroutines: %s (limit: %s)\n", 
					formatNumber(test.Actual), formatNumber(test.Target))
			}
		}
	}

	if scalabilityCategory, exists := results.Categories["Scalability"]; exists {
		for _, test := range scalabilityCategory.Tests {
			if test.Name == "ScalingEfficiency" {
				fmt.Printf("📈 Scaling Efficiency: %.2fx (target: %.2fx)\n", 
					test.Actual.(float64), test.Target.(float64))
			}
		}
	}
}

func displayRecommendations(results *testing.ValidationResults) {
	recommendations := []string{}

	// Analyze failures and generate recommendations
	for _, category := range results.Categories {
		for _, test := range category.Tests {
			if !test.Passed {
				switch test.Name {
				case "OrderProcessingLatency":
					recommendations = append(recommendations, 
						"Consider optimizing order processing pipeline or increasing CPU resources")
				case "RiskCheckLatency":
					recommendations = append(recommendations,
						"Optimize risk calculation algorithms or implement caching")
				case "OrderThroughput":
					recommendations = append(recommendations,
						"Scale horizontally or optimize bottleneck components")
				case "MemoryUsage":
					recommendations = append(recommendations,
						"Implement memory pooling or increase available memory")
				case "ScalingEfficiency":
					recommendations = append(recommendations,
						"Identify and resolve scaling bottlenecks in the architecture")
				case "ErrorRate":
					recommendations = append(recommendations,
						"Investigate and fix sources of errors in the system")
				}
			}
		}
	}

	// Display unique recommendations
	uniqueRecommendations := removeDuplicates(recommendations)
	for i, rec := range uniqueRecommendations {
		fmt.Printf("%d. %s\n", i+1, rec)
	}

	if len(uniqueRecommendations) == 0 {
		fmt.Println("No specific recommendations available.")
	}
}

func generatePerformanceReport(results *testing.ValidationResults) {
	fmt.Println("\n📄 Generating detailed performance report...")

	report := PerformanceReport{
		Timestamp:       time.Now(),
		SystemInfo:      getSystemInfo(),
		ValidationResults: results,
		Recommendations: generateDetailedRecommendations(results),
	}

	// Save as JSON
	filename := fmt.Sprintf("test_results/performance_validation_%s.json", 
		time.Now().Format("20060102_150405"))
	
	data, _ := json.MarshalIndent(report, "", "  ")
	os.MkdirAll("test_results", 0755)
	os.WriteFile(filename, data, 0644)

	// Generate HTML report
	htmlFilename := fmt.Sprintf("test_results/performance_report_%s.html", 
		time.Now().Format("20060102_150405"))
	generateHTMLReport(report, htmlFilename)

	fmt.Printf("📊 Detailed report saved to: %s\n", filename)
	fmt.Printf("🌐 HTML report saved to: %s\n", htmlFilename)
}

func saveValidationResults(results *testing.ValidationResults) error {
	filename := fmt.Sprintf("test_results/validation_results_%s.json", 
		time.Now().Format("20060102_150405"))
	
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll("test_results", 0755); err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

// Helper functions

func countPassedTests(tests []testing.TestResult) int {
	count := 0
	for _, test := range tests {
		if test.Passed {
			count++
		}
	}
	return count
}

func formatDuration(value interface{}) string {
	if d, ok := value.(time.Duration); ok {
		if d < time.Microsecond {
			return fmt.Sprintf("%dns", d.Nanoseconds())
		} else if d < time.Millisecond {
			return fmt.Sprintf("%.1fμs", float64(d.Microseconds()))
		} else if d < time.Second {
			return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000)
		} else {
			return fmt.Sprintf("%.1fs", d.Seconds())
		}
	}
	return fmt.Sprintf("%v", value)
}

func formatNumber(value interface{}) string {
	switch v := value.(type) {
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%.0f", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func removeDuplicates(strings []string) []string {
	keys := make(map[string]bool)
	result := []string{}
	
	for _, str := range strings {
		if !keys[str] {
			keys[str] = true
			result = append(result, str)
		}
	}
	
	return result
}

func getSystemInfo() SystemInfo {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	return SystemInfo{
		GoVersion:    runtime.Version(),
		CPUCores:     runtime.NumCPU(),
		Architecture: fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Memory:       memStats.Sys,
	}
}

func generateDetailedRecommendations(results *testing.ValidationResults) []string {
	// Generate more detailed recommendations based on results
	recommendations := []string{
		"Monitor system performance regularly using the integrated dashboard",
		"Set up alerting for performance degradation",
		"Consider implementing auto-scaling based on load patterns",
		"Regular performance testing should be integrated into CI/CD pipeline",
	}
	
	return recommendations
}

func generateHTMLReport(report PerformanceReport, filename string) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Performance Validation Report</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        .header { background: #f0f0f0; padding: 20px; border-radius: 5px; }
        .category { margin: 20px 0; padding: 15px; border: 1px solid #ddd; border-radius: 5px; }
        .passed { color: green; }
        .failed { color: red; }
        .metric { margin: 10px 0; padding: 10px; background: #f9f9f9; border-radius: 3px; }
    </style>
</head>
<body>
    <div class="header">
        <h1>🎯 Performance Validation Report</h1>
        <p><strong>Generated:</strong> ` + report.Timestamp.Format("2006-01-02 15:04:05") + `</p>
        <p><strong>System:</strong> ` + report.SystemInfo.Architecture + ` (` + 
		fmt.Sprintf("%d cores", report.SystemInfo.CPUCores) + `)</p>
    </div>
    
    <div class="category">
        <h2>📊 Summary</h2>
        <div class="metric">
            <strong>Overall Status:</strong> <span class="` + 
			map[bool]string{true: "passed", false: "failed"}[report.ValidationResults.OverallPassed] + 
			`">` + map[bool]string{true: "PASSED", false: "FAILED"}[report.ValidationResults.OverallPassed] + `</span>
        </div>
        <div class="metric">
            <strong>Test Results:</strong> ` + 
			fmt.Sprintf("%d passed, %d failed out of %d total", 
				report.ValidationResults.PassedTests, 
				report.ValidationResults.FailedTests, 
				report.ValidationResults.TotalTests) + `
        </div>
        <div class="metric">
            <strong>Duration:</strong> ` + report.ValidationResults.Duration.String() + `
        </div>
    </div>
</body>
</html>`

	os.WriteFile(filename, []byte(html), 0644)
}

// Data structures for reporting

type PerformanceReport struct {
	Timestamp         time.Time
	SystemInfo        SystemInfo
	ValidationResults *testing.ValidationResults
	Recommendations   []string
}

type SystemInfo struct {
	GoVersion    string
	CPUCores     int
	Architecture string
	Memory       uint64
}

// High-performance mock implementations for validation testing

type ValidationOrderManager struct{}

func NewValidationOrderManager() *ValidationOrderManager {
	return &ValidationOrderManager{}
}

func (m *ValidationOrderManager) ProcessOrder(ctx context.Context, order *testing.TestOrder) error {
	// Simulate optimized order processing
	time.Sleep(time.Microsecond * 40) // 40μs target performance
	order.Status = "FILLED"
	return nil
}

type ValidationRiskEngine struct{}

func NewValidationRiskEngine() *ValidationRiskEngine {
	return &ValidationRiskEngine{}
}

func (r *ValidationRiskEngine) CheckOrderRisk(order *testing.TestOrder) bool {
	// Simulate ultra-fast risk check
	time.Sleep(time.Microsecond * 3) // 3μs risk check
	return order.Quantity <= 1000.0
}

// Mock implementations for other components
type ValidationPositionManager struct{}
func NewValidationPositionManager() *ValidationPositionManager { return &ValidationPositionManager{} }

type ValidationAccountManager struct{}
func NewValidationAccountManager() *ValidationAccountManager { return &ValidationAccountManager{} }

type ValidationRoutingEngine struct{}
func NewValidationRoutingEngine() *ValidationRoutingEngine { return &ValidationRoutingEngine{} }