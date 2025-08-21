package keymanager

import (
	"fmt"
	"sync"
	"time"
)

// UsageTracker tracks API key usage statistics
type UsageTracker struct {
	mu       sync.RWMutex
	usage    map[string]*KeyUsage
	hourly   map[string]map[string]int64 // keyID -> hour -> count
	daily    map[string]map[string]int64 // keyID -> date -> count
}

// NewUsageTracker creates a new usage tracker
func NewUsageTracker() *UsageTracker {
	ut := &UsageTracker{
		usage:  make(map[string]*KeyUsage),
		hourly: make(map[string]map[string]int64),
		daily:  make(map[string]map[string]int64),
	}

	// Start cleanup goroutine
	go ut.cleanupLoop()

	return ut
}

// TrackRequest tracks an API request
func (ut *UsageTracker) TrackRequest(keyID, accountName string, success bool) {
	ut.mu.Lock()
	defer ut.mu.Unlock()

	// Initialize usage if needed
	if _, exists := ut.usage[keyID]; !exists {
		ut.usage[keyID] = &KeyUsage{
			KeyID:         keyID,
			AccountName:   accountName,
			DailyRequests: make(map[string]int64),
			ErrorCodes:    make(map[string]int64),
		}
	}

	usage := ut.usage[keyID]
	usage.TotalRequests++
	usage.LastRequestTime = time.Now()

	if success {
		usage.SuccessRequests++
	} else {
		usage.FailedRequests++
	}

	// Track daily usage
	today := time.Now().Format("2006-01-02")
	usage.DailyRequests[today]++

	// Track hourly usage
	hour := time.Now().Format("2006-01-02_15")
	if _, exists := ut.hourly[keyID]; !exists {
		ut.hourly[keyID] = make(map[string]int64)
	}
	ut.hourly[keyID][hour]++

	// Track daily totals
	if _, exists := ut.daily[keyID]; !exists {
		ut.daily[keyID] = make(map[string]int64)
	}
	ut.daily[keyID][today]++
}

// TrackError tracks an error for a key
func (ut *UsageTracker) TrackError(keyID string, errorCode string) {
	ut.mu.Lock()
	defer ut.mu.Unlock()

	if usage, exists := ut.usage[keyID]; exists {
		usage.ErrorCodes[errorCode]++
	}
}

// GetUsage returns usage statistics for a key
func (ut *UsageTracker) GetUsage(keyID string) (*KeyUsage, error) {
	ut.mu.RLock()
	defer ut.mu.RUnlock()

	usage, exists := ut.usage[keyID]
	if !exists {
		return nil, fmt.Errorf("no usage data for key %s", keyID)
	}

	// Return a copy to avoid race conditions
	usageCopy := &KeyUsage{
		KeyID:            usage.KeyID,
		AccountName:      usage.AccountName,
		TotalRequests:    usage.TotalRequests,
		SuccessRequests:  usage.SuccessRequests,
		FailedRequests:   usage.FailedRequests,
		LastRequestTime:  usage.LastRequestTime,
		DailyRequests:    make(map[string]int64),
		ErrorCodes:       make(map[string]int64),
	}

	for k, v := range usage.DailyRequests {
		usageCopy.DailyRequests[k] = v
	}
	for k, v := range usage.ErrorCodes {
		usageCopy.ErrorCodes[k] = v
	}

	return usageCopy, nil
}

// GetAccountUsage returns aggregated usage for an account
func (ut *UsageTracker) GetAccountUsage(accountName string) (*AccountUsage, error) {
	ut.mu.RLock()
	defer ut.mu.RUnlock()

	accountUsage := &AccountUsage{
		AccountName:   accountName,
		TotalRequests: 0,
		KeyUsage:      make(map[string]*KeyUsage),
		DailyTotals:   make(map[string]int64),
	}

	for keyID, usage := range ut.usage {
		if usage.AccountName == accountName {
			accountUsage.TotalRequests += usage.TotalRequests
			accountUsage.SuccessRequests += usage.SuccessRequests
			accountUsage.FailedRequests += usage.FailedRequests
			accountUsage.KeyUsage[keyID] = usage

			// Aggregate daily totals
			for date, count := range usage.DailyRequests {
				accountUsage.DailyTotals[date] += count
			}
		}
	}

	if len(accountUsage.KeyUsage) == 0 {
		return nil, fmt.Errorf("no usage data for account %s", accountName)
	}

	return accountUsage, nil
}

// GetHourlyStats returns hourly statistics for a key
func (ut *UsageTracker) GetHourlyStats(keyID string, hours int) []HourlyStats {
	ut.mu.RLock()
	defer ut.mu.RUnlock()

	stats := []HourlyStats{}
	hourlyData, exists := ut.hourly[keyID]
	if !exists {
		return stats
	}

	now := time.Now()
	for i := hours - 1; i >= 0; i-- {
		hour := now.Add(time.Duration(-i) * time.Hour)
		hourKey := hour.Format("2006-01-02_15")
		
		count := int64(0)
		if c, exists := hourlyData[hourKey]; exists {
			count = c
		}

		stats = append(stats, HourlyStats{
			Hour:     hour,
			Requests: count,
		})
	}

	return stats
}

// GetDailyStats returns daily statistics for a key
func (ut *UsageTracker) GetDailyStats(keyID string, days int) []DailyStats {
	ut.mu.RLock()
	defer ut.mu.RUnlock()

	stats := []DailyStats{}
	dailyData, exists := ut.daily[keyID]
	if !exists {
		return stats
	}

	now := time.Now()
	for i := days - 1; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		dayKey := day.Format("2006-01-02")
		
		count := int64(0)
		if c, exists := dailyData[dayKey]; exists {
			count = c
		}

		stats = append(stats, DailyStats{
			Date:     day,
			Requests: count,
		})
	}

	return stats
}

// GetTopKeys returns the most used keys
func (ut *UsageTracker) GetTopKeys(limit int) []KeyUsageSummary {
	ut.mu.RLock()
	defer ut.mu.RUnlock()

	summaries := []KeyUsageSummary{}
	
	for keyID, usage := range ut.usage {
		summary := KeyUsageSummary{
			KeyID:         keyID,
			AccountName:   usage.AccountName,
			TotalRequests: usage.TotalRequests,
			SuccessRate:   0,
		}

		if usage.TotalRequests > 0 {
			summary.SuccessRate = float64(usage.SuccessRequests) / float64(usage.TotalRequests)
		}

		summaries = append(summaries, summary)
	}

	// Sort by total requests (simple bubble sort)
	for i := 0; i < len(summaries); i++ {
		for j := i + 1; j < len(summaries); j++ {
			if summaries[j].TotalRequests > summaries[i].TotalRequests {
				summaries[i], summaries[j] = summaries[j], summaries[i]
			}
		}
	}

	if len(summaries) > limit {
		summaries = summaries[:limit]
	}

	return summaries
}

// GetErrorSummary returns error statistics
func (ut *UsageTracker) GetErrorSummary() map[string]int64 {
	ut.mu.RLock()
	defer ut.mu.RUnlock()

	errorSummary := make(map[string]int64)
	
	for _, usage := range ut.usage {
		for errorCode, count := range usage.ErrorCodes {
			errorSummary[errorCode] += count
		}
	}

	return errorSummary
}

// cleanupLoop periodically cleans up old data
func (ut *UsageTracker) cleanupLoop() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		ut.cleanup()
	}
}

// cleanup removes old usage data
func (ut *UsageTracker) cleanup() {
	ut.mu.Lock()
	defer ut.mu.Unlock()

	cutoffDaily := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	cutoffHourly := time.Now().Add(-7 * 24 * time.Hour).Format("2006-01-02_15")

	// Clean up daily data older than 30 days
	for keyID, usage := range ut.usage {
		for date := range usage.DailyRequests {
			if date < cutoffDaily {
				delete(usage.DailyRequests, date)
			}
		}
		
		// Clean up daily tracking
		if dailyData, exists := ut.daily[keyID]; exists {
			for date := range dailyData {
				if date < cutoffDaily {
					delete(dailyData, date)
				}
			}
		}
	}

	// Clean up hourly data older than 7 days
	for keyID, hourlyData := range ut.hourly {
		for hour := range hourlyData {
			if hour < cutoffHourly {
				delete(hourlyData, hour)
			}
		}
	}
}

// Reset clears all usage data (for testing)
func (ut *UsageTracker) Reset() {
	ut.mu.Lock()
	defer ut.mu.Unlock()

	ut.usage = make(map[string]*KeyUsage)
	ut.hourly = make(map[string]map[string]int64)
	ut.daily = make(map[string]map[string]int64)
}

// AccountUsage represents aggregated usage for an account
type AccountUsage struct {
	AccountName     string              `json:"account_name"`
	TotalRequests   int64               `json:"total_requests"`
	SuccessRequests int64               `json:"success_requests"`
	FailedRequests  int64               `json:"failed_requests"`
	KeyUsage        map[string]*KeyUsage `json:"key_usage"`
	DailyTotals     map[string]int64    `json:"daily_totals"`
}

// HourlyStats represents hourly usage statistics
type HourlyStats struct {
	Hour     time.Time `json:"hour"`
	Requests int64     `json:"requests"`
}

// DailyStats represents daily usage statistics
type DailyStats struct {
	Date     time.Time `json:"date"`
	Requests int64     `json:"requests"`
}

// KeyUsageSummary provides a summary of key usage
type KeyUsageSummary struct {
	KeyID         string  `json:"key_id"`
	AccountName   string  `json:"account_name"`
	TotalRequests int64   `json:"total_requests"`
	SuccessRate   float64 `json:"success_rate"`
}

// UsageReport generates a comprehensive usage report
type UsageReport struct {
	Period          string                     `json:"period"`
	TotalRequests   int64                      `json:"total_requests"`
	SuccessRate     float64                    `json:"success_rate"`
	TopKeys         []KeyUsageSummary          `json:"top_keys"`
	ErrorSummary    map[string]int64           `json:"error_summary"`
	AccountSummary  map[string]*AccountSummary `json:"account_summary"`
}

// AccountSummary provides account-level summary
type AccountSummary struct {
	TotalRequests int64   `json:"total_requests"`
	ActiveKeys    int     `json:"active_keys"`
	SuccessRate   float64 `json:"success_rate"`
}

// GenerateUsageReport generates a comprehensive usage report
func (ut *UsageTracker) GenerateUsageReport() *UsageReport {
	ut.mu.RLock()
	defer ut.mu.RUnlock()

	report := &UsageReport{
		Period:         time.Now().Format("2006-01-02"),
		AccountSummary: make(map[string]*AccountSummary),
	}

	totalSuccess := int64(0)
	accountData := make(map[string]*AccountSummary)

	// Aggregate data
	for _, usage := range ut.usage {
		report.TotalRequests += usage.TotalRequests
		totalSuccess += usage.SuccessRequests

		// Account summary
		if _, exists := accountData[usage.AccountName]; !exists {
			accountData[usage.AccountName] = &AccountSummary{}
		}
		
		summary := accountData[usage.AccountName]
		summary.TotalRequests += usage.TotalRequests
		summary.ActiveKeys++
	}

	// Calculate success rates
	if report.TotalRequests > 0 {
		report.SuccessRate = float64(totalSuccess) / float64(report.TotalRequests)
	}

	for account, summary := range accountData {
		if summary.TotalRequests > 0 {
			successCount := int64(0)
			for _, usage := range ut.usage {
				if usage.AccountName == account {
					successCount += usage.SuccessRequests
				}
			}
			summary.SuccessRate = float64(successCount) / float64(summary.TotalRequests)
		}
		report.AccountSummary[account] = summary
	}

	// Get top keys and error summary
	report.TopKeys = ut.GetTopKeys(10)
	report.ErrorSummary = ut.GetErrorSummary()

	return report
}