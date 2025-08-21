package keymanager

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Auditor handles audit logging for key management operations
type Auditor struct {
	mu       sync.Mutex
	logPath  string
	file     *os.File
	encoder  *json.Encoder
}

// NewAuditor creates a new auditor
func NewAuditor(logPath string) *Auditor {
	return &Auditor{
		logPath: logPath,
	}
}

// Start initializes the audit log
func (a *Auditor) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Create directory if needed
	dir := filepath.Dir(a.logPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create audit log directory: %w", err)
	}

	// Open log file
	file, err := os.OpenFile(a.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open audit log: %w", err)
	}

	a.file = file
	a.encoder = json.NewEncoder(file)

	return nil
}

// LogAction logs an audit event
func (a *Auditor) LogAction(ctx context.Context, action, resource string, success bool, details map[string]interface{}) {
	if a.encoder == nil {
		// Try to start if not already started
		if err := a.Start(); err != nil {
			fmt.Printf("Failed to start auditor: %v\n", err)
			return
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	log := AuditLog{
		ID:        generateAuditID(),
		Timestamp: time.Now(),
		Action:    action,
		Resource:  resource,
		Success:   success,
		Details:   details,
	}

	// Extract context information
	if actor := ctx.Value("actor"); actor != nil {
		log.Actor = actor.(string)
	}
	if ip := ctx.Value("ip_address"); ip != nil {
		log.IPAddress = ip.(string)
	}
	if ua := ctx.Value("user_agent"); ua != nil {
		log.UserAgent = ua.(string)
	}

	// Write to log
	if err := a.encoder.Encode(log); err != nil {
		fmt.Printf("Failed to write audit log: %v\n", err)
	}
}

// Query retrieves audit logs based on criteria
func (a *Auditor) Query(criteria AuditCriteria) ([]AuditLog, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Open file for reading
	file, err := os.Open(a.logPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var logs []AuditLog
	decoder := json.NewDecoder(file)

	for decoder.More() {
		var log AuditLog
		if err := decoder.Decode(&log); err != nil {
			continue
		}

		if a.matchesCriteria(log, criteria) {
			logs = append(logs, log)
		}
	}

	return logs, nil
}

// matchesCriteria checks if a log matches the search criteria
func (a *Auditor) matchesCriteria(log AuditLog, criteria AuditCriteria) bool {
	// Time range
	if !criteria.StartTime.IsZero() && log.Timestamp.Before(criteria.StartTime) {
		return false
	}
	if !criteria.EndTime.IsZero() && log.Timestamp.After(criteria.EndTime) {
		return false
	}

	// Action filter
	if criteria.Action != "" && log.Action != criteria.Action {
		return false
	}

	// Actor filter
	if criteria.Actor != "" && log.Actor != criteria.Actor {
		return false
	}

	// Resource filter
	if criteria.Resource != "" && log.Resource != criteria.Resource {
		return false
	}

	// Success filter
	if criteria.SuccessOnly && !log.Success {
		return false
	}
	if criteria.FailureOnly && log.Success {
		return false
	}

	return true
}

// GetRecent returns the most recent audit logs
func (a *Auditor) GetRecent(count int) ([]AuditLog, error) {
	logs, err := a.Query(AuditCriteria{})
	if err != nil {
		return nil, err
	}

	// Return last 'count' logs
	if len(logs) > count {
		return logs[len(logs)-count:], nil
	}

	return logs, nil
}

// Rotate rotates the audit log file
func (a *Auditor) Rotate() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.file != nil {
		a.file.Close()
	}

	// Rename current file with timestamp
	timestamp := time.Now().Format("20060102_150405")
	newPath := fmt.Sprintf("%s.%s", a.logPath, timestamp)
	
	if err := os.Rename(a.logPath, newPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to rotate audit log: %w", err)
	}

	// Start new file
	return a.Start()
}

// Close closes the audit log
func (a *Auditor) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.file != nil {
		return a.file.Close()
	}
	return nil
}

// AuditCriteria defines search criteria for audit logs
type AuditCriteria struct {
	StartTime   time.Time
	EndTime     time.Time
	Action      string
	Actor       string
	Resource    string
	SuccessOnly bool
	FailureOnly bool
}

// AuditReport generates an audit report
type AuditReport struct {
	Period       string                      `json:"period"`
	TotalActions int                         `json:"total_actions"`
	SuccessRate  float64                     `json:"success_rate"`
	ActionCounts map[string]int              `json:"action_counts"`
	ActorCounts  map[string]int              `json:"actor_counts"`
	FailedOps    []AuditLog                  `json:"failed_operations"`
	TopResources []ResourceAccess            `json:"top_resources"`
}

// ResourceAccess tracks resource access frequency
type ResourceAccess struct {
	Resource    string `json:"resource"`
	AccessCount int    `json:"access_count"`
}

// GenerateReport generates an audit report for a time period
func (a *Auditor) GenerateReport(startTime, endTime time.Time) (*AuditReport, error) {
	logs, err := a.Query(AuditCriteria{
		StartTime: startTime,
		EndTime:   endTime,
	})
	if err != nil {
		return nil, err
	}

	report := &AuditReport{
		Period:       fmt.Sprintf("%s to %s", startTime.Format("2006-01-02"), endTime.Format("2006-01-02")),
		ActionCounts: make(map[string]int),
		ActorCounts:  make(map[string]int),
		FailedOps:    []AuditLog{},
	}

	resourceCounts := make(map[string]int)
	successCount := 0

	for _, log := range logs {
		report.TotalActions++
		
		// Count by action
		report.ActionCounts[log.Action]++
		
		// Count by actor
		if log.Actor != "" {
			report.ActorCounts[log.Actor]++
		}
		
		// Track success/failure
		if log.Success {
			successCount++
		} else {
			report.FailedOps = append(report.FailedOps, log)
		}
		
		// Count resource access
		resourceCounts[log.Resource]++
	}

	// Calculate success rate
	if report.TotalActions > 0 {
		report.SuccessRate = float64(successCount) / float64(report.TotalActions)
	}

	// Get top resources
	report.TopResources = a.getTopResources(resourceCounts, 10)

	return report, nil
}

// getTopResources returns the top N most accessed resources
func (a *Auditor) getTopResources(counts map[string]int, limit int) []ResourceAccess {
	var resources []ResourceAccess
	
	for resource, count := range counts {
		resources = append(resources, ResourceAccess{
			Resource:    resource,
			AccessCount: count,
		})
	}

	// Sort by count (simple bubble sort for small data)
	for i := 0; i < len(resources); i++ {
		for j := i + 1; j < len(resources); j++ {
			if resources[j].AccessCount > resources[i].AccessCount {
				resources[i], resources[j] = resources[j], resources[i]
			}
		}
	}

	if len(resources) > limit {
		resources = resources[:limit]
	}

	return resources
}

// generateAuditID generates a unique audit log ID
func generateAuditID() string {
	return fmt.Sprintf("audit_%d_%s", time.Now().UnixNano(), generateRandomString(8))
}

// generateRandomString generates a random string of specified length
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}