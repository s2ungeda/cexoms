package security

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

// AnomalyDetector detects anomalous behavior using statistical methods
type AnomalyDetector struct {
	mu              sync.RWMutex
	baseline        *SecurityBaseline
	models          map[string]*AnomalyModel
	detectionWindow time.Duration
	sensitivity     float64
	running         bool
	stopChan        chan bool
}

// SecurityBaseline represents normal behavior patterns
type SecurityBaseline struct {
	UserProfiles    map[string]*UserProfile
	SystemMetrics   *SystemMetrics
	NetworkPatterns *NetworkPatterns
	LastUpdated     time.Time
}

// UserProfile represents normal user behavior
type UserProfile struct {
	UserID            string
	LoginTimes        []time.Time
	CommonIPs         map[string]int
	CommonLocations   map[string]int
	AccessPatterns    map[string]*AccessPattern
	TypicalDataVolume float64
	RiskScore         float64
}

// AccessPattern represents resource access patterns
type AccessPattern struct {
	Resource      string
	Frequency     float64 // accesses per hour
	TimeDistribution map[int]float64 // hour of day -> probability
	DataVolume    float64 // average bytes accessed
	LastAccess    time.Time
}

// SystemMetrics represents normal system behavior
type SystemMetrics struct {
	NormalAPIRate     float64
	NormalErrorRate   float64
	NormalDataRate    float64
	PeakHours         []int
	AuthFailureRate   float64
}

// NetworkPatterns represents normal network behavior
type NetworkPatterns struct {
	CommonPorts      map[int]int
	CommonProtocols  map[string]int
	TrafficVolume    float64
	ConnectionRate   float64
}

// AnomalyModel represents a statistical model for anomaly detection
type AnomalyModel struct {
	Type          ModelType
	Parameters    map[string]float64
	Thresholds    map[string]float64
	LastTraining  time.Time
	Accuracy      float64
}

// ModelType defines types of anomaly detection models
type ModelType string

const (
	ModelTypeStatistical ModelType = "statistical"
	ModelTypeML          ModelType = "machine_learning"
	ModelTypeRule        ModelType = "rule_based"
	ModelTypeHybrid      ModelType = "hybrid"
)

// Anomaly represents detected anomalous behavior
type Anomaly struct {
	ID          string
	Timestamp   time.Time
	Type        AnomalyType
	Source      string
	Target      string
	Description string
	Score       float64 // 0-100, higher = more anomalous
	Evidence    []string
	Model       string
}

// AnomalyType categorizes anomalies
type AnomalyType string

const (
	AnomalyTypeLogin       AnomalyType = "unusual_login"
	AnomalyTypeAccess      AnomalyType = "unusual_access"
	AnomalyTypeDataVolume  AnomalyType = "unusual_data_volume"
	AnomalyTypeFrequency   AnomalyType = "unusual_frequency"
	AnomalyTypeSequence    AnomalyType = "unusual_sequence"
	AnomalyTypeNetwork     AnomalyType = "unusual_network"
)

// NewAnomalyDetector creates a new anomaly detector
func NewAnomalyDetector() *AnomalyDetector {
	ad := &AnomalyDetector{
		baseline: &SecurityBaseline{
			UserProfiles:    make(map[string]*UserProfile),
			SystemMetrics:   &SystemMetrics{},
			NetworkPatterns: &NetworkPatterns{},
		},
		models:          make(map[string]*AnomalyModel),
		detectionWindow: 1 * time.Hour,
		sensitivity:     0.7, // 0-1, higher = more sensitive
		stopChan:        make(chan bool),
	}
	
	// Initialize models
	ad.initializeModels()
	
	return ad
}

// initializeModels sets up anomaly detection models
func (ad *AnomalyDetector) initializeModels() {
	// Statistical model for login anomalies
	ad.models["login_anomaly"] = &AnomalyModel{
		Type: ModelTypeStatistical,
		Parameters: map[string]float64{
			"mean_login_hour":     9.0,  // Average login hour
			"std_login_hour":      3.0,  // Standard deviation
			"login_rate_mean":     10.0, // Logins per hour
			"login_rate_std":      5.0,
		},
		Thresholds: map[string]float64{
			"time_zscore":     3.0, // Z-score threshold
			"rate_zscore":     3.0,
			"new_ip_score":    0.8, // Score for new IP
			"new_location":    0.9, // Score for new location
		},
	}
	
	// Statistical model for data access anomalies
	ad.models["access_anomaly"] = &AnomalyModel{
		Type: ModelTypeStatistical,
		Parameters: map[string]float64{
			"access_rate_mean": 100.0, // Accesses per hour
			"access_rate_std":  30.0,
			"data_volume_mean": 1024 * 1024 * 10, // 10MB average
			"data_volume_std":  1024 * 1024 * 5,  // 5MB std
		},
		Thresholds: map[string]float64{
			"rate_zscore":   3.0,
			"volume_zscore": 3.0,
			"pattern_deviation": 0.7,
		},
	}
	
	// ML-based model for sequence anomalies
	ad.models["sequence_anomaly"] = &AnomalyModel{
		Type: ModelTypeML,
		Parameters: map[string]float64{
			"sequence_length":    5.0,
			"markov_order":       2.0,
			"min_probability":    0.01,
		},
		Thresholds: map[string]float64{
			"sequence_score": 0.8,
		},
	}
}

// Start starts the anomaly detector
func (ad *AnomalyDetector) Start(ctx context.Context) {
	ad.mu.Lock()
	if ad.running {
		ad.mu.Unlock()
		return
	}
	ad.running = true
	ad.mu.Unlock()
	
	// Start baseline updater
	go ad.baselineUpdater(ctx)
	
	// Start model trainer
	go ad.modelTrainer(ctx)
}

// Stop stops the anomaly detector
func (ad *AnomalyDetector) Stop() {
	ad.mu.Lock()
	if !ad.running {
		ad.mu.Unlock()
		return
	}
	ad.running = false
	ad.mu.Unlock()
	
	close(ad.stopChan)
}

// Analyze analyzes an event for anomalies
func (ad *AnomalyDetector) Analyze(event *AuditEvent) *Anomaly {
	ad.mu.RLock()
	defer ad.mu.RUnlock()
	
	// Check login anomalies
	if event.EventType == "authentication" {
		if anomaly := ad.checkLoginAnomaly(event); anomaly != nil {
			return anomaly
		}
	}
	
	// Check access anomalies
	if event.EventType == "access" {
		if anomaly := ad.checkAccessAnomaly(event); anomaly != nil {
			return anomaly
		}
	}
	
	// Check data volume anomalies
	if anomaly := ad.checkDataVolumeAnomaly(event); anomaly != nil {
		return anomaly
	}
	
	// Check sequence anomalies
	if anomaly := ad.checkSequenceAnomaly(event); anomaly != nil {
		return anomaly
	}
	
	return nil
}

// checkLoginAnomaly checks for login anomalies
func (ad *AnomalyDetector) checkLoginAnomaly(event *AuditEvent) *Anomaly {
	userProfile, exists := ad.baseline.UserProfiles[event.UserID]
	if !exists {
		// New user, create profile
		userProfile = &UserProfile{
			UserID:          event.UserID,
			CommonIPs:       make(map[string]int),
			CommonLocations: make(map[string]int),
			AccessPatterns:  make(map[string]*AccessPattern),
		}
		ad.baseline.UserProfiles[event.UserID] = userProfile
	}
	
	anomalyScore := 0.0
	evidence := make([]string, 0)
	
	// Check login time anomaly
	loginHour := float64(time.Now().Hour())
	model := ad.models["login_anomaly"]
	
	meanHour := model.Parameters["mean_login_hour"]
	stdHour := model.Parameters["std_login_hour"]
	timeZScore := math.Abs(loginHour - meanHour) / stdHour
	
	if timeZScore > model.Thresholds["time_zscore"] {
		anomalyScore += 30.0
		evidence = append(evidence, fmt.Sprintf("Unusual login time: %d:00", int(loginHour)))
	}
	
	// Check IP anomaly
	if _, known := userProfile.CommonIPs[event.IPAddress]; !known && event.IPAddress != "" {
		anomalyScore += model.Thresholds["new_ip_score"] * 40.0
		evidence = append(evidence, fmt.Sprintf("New IP address: %s", event.IPAddress))
	}
	
	// Check location anomaly (would use GeoIP in production)
	location := ad.getLocationFromIP(event.IPAddress)
	if _, known := userProfile.CommonLocations[location]; !known && location != "" {
		anomalyScore += model.Thresholds["new_location"] * 30.0
		evidence = append(evidence, fmt.Sprintf("New location: %s", location))
	}
	
	// Apply sensitivity
	anomalyScore *= ad.sensitivity
	
	if anomalyScore > 50.0 {
		return &Anomaly{
			ID:          generateAnomalyID(),
			Timestamp:   time.Now(),
			Type:        AnomalyTypeLogin,
			Source:      event.UserID,
			Target:      "authentication_system",
			Description: fmt.Sprintf("Unusual login pattern detected for user %s", event.UserID),
			Score:       anomalyScore,
			Evidence:    evidence,
			Model:       "login_anomaly",
		}
	}
	
	return nil
}

// checkAccessAnomaly checks for access pattern anomalies
func (ad *AnomalyDetector) checkAccessAnomaly(event *AuditEvent) *Anomaly {
	userProfile, exists := ad.baseline.UserProfiles[event.UserID]
	if !exists {
		return nil
	}
	
	anomalyScore := 0.0
	evidence := make([]string, 0)
	
	// Get access pattern for resource
	pattern, exists := userProfile.AccessPatterns[event.Resource]
	if !exists {
		// First time accessing this resource
		anomalyScore += 40.0
		evidence = append(evidence, fmt.Sprintf("First access to resource: %s", event.Resource))
	} else {
		// Check frequency anomaly
		currentHour := time.Now().Hour()
		expectedProb := pattern.TimeDistribution[currentHour]
		
		if expectedProb < 0.1 {
			anomalyScore += 30.0
			evidence = append(evidence, fmt.Sprintf("Unusual access time for resource: %s", event.Resource))
		}
		
		// Check access rate
		timeSinceLastAccess := time.Since(pattern.LastAccess)
		if timeSinceLastAccess < 1*time.Minute {
			anomalyScore += 20.0
			evidence = append(evidence, "Rapid successive access")
		}
	}
	
	// Check against system baseline
	model := ad.models["access_anomaly"]
	if ad.baseline.SystemMetrics.NormalAPIRate > 0 {
		// This would calculate actual rate
		currentRate := 150.0 // Simulated
		meanRate := model.Parameters["access_rate_mean"]
		stdRate := model.Parameters["access_rate_std"]
		rateZScore := math.Abs(currentRate - meanRate) / stdRate
		
		if rateZScore > model.Thresholds["rate_zscore"] {
			anomalyScore += 30.0
			evidence = append(evidence, fmt.Sprintf("Abnormal access rate: %.0f/hour", currentRate))
		}
	}
	
	// Apply sensitivity
	anomalyScore *= ad.sensitivity
	
	if anomalyScore > 50.0 {
		return &Anomaly{
			ID:          generateAnomalyID(),
			Timestamp:   time.Now(),
			Type:        AnomalyTypeAccess,
			Source:      event.UserID,
			Target:      event.Resource,
			Description: fmt.Sprintf("Unusual access pattern detected for %s accessing %s", event.UserID, event.Resource),
			Score:       anomalyScore,
			Evidence:    evidence,
			Model:       "access_anomaly",
		}
	}
	
	return nil
}

// checkDataVolumeAnomaly checks for data volume anomalies
func (ad *AnomalyDetector) checkDataVolumeAnomaly(event *AuditEvent) *Anomaly {
	// Extract data volume from event metadata
	dataVolume, ok := event.Metadata["data_volume"].(float64)
	if !ok {
		return nil
	}
	
	model := ad.models["access_anomaly"]
	meanVolume := model.Parameters["data_volume_mean"]
	stdVolume := model.Parameters["data_volume_std"]
	
	volumeZScore := math.Abs(dataVolume - meanVolume) / stdVolume
	
	if volumeZScore > model.Thresholds["volume_zscore"] {
		anomalyScore := math.Min(volumeZScore * 20.0, 100.0) * ad.sensitivity
		
		return &Anomaly{
			ID:          generateAnomalyID(),
			Timestamp:   time.Now(),
			Type:        AnomalyTypeDataVolume,
			Source:      event.UserID,
			Target:      event.Resource,
			Description: fmt.Sprintf("Unusual data volume: %.2f MB", dataVolume/1024/1024),
			Score:       anomalyScore,
			Evidence:    []string{fmt.Sprintf("Z-score: %.2f", volumeZScore)},
			Model:       "access_anomaly",
		}
	}
	
	return nil
}

// checkSequenceAnomaly checks for sequence anomalies
func (ad *AnomalyDetector) checkSequenceAnomaly(event *AuditEvent) *Anomaly {
	// This would implement sequence anomaly detection
	// using Markov chains or similar techniques
	// For now, return nil
	return nil
}

// UpdateBaseline updates the security baseline with new data
func (ad *AnomalyDetector) UpdateBaseline(event *AuditEvent) {
	ad.mu.Lock()
	defer ad.mu.Unlock()
	
	// Update user profile
	userProfile, exists := ad.baseline.UserProfiles[event.UserID]
	if !exists {
		userProfile = &UserProfile{
			UserID:          event.UserID,
			CommonIPs:       make(map[string]int),
			CommonLocations: make(map[string]int),
			AccessPatterns:  make(map[string]*AccessPattern),
		}
		ad.baseline.UserProfiles[event.UserID] = userProfile
	}
	
	// Update login patterns
	if event.EventType == "authentication" && event.Result == "success" {
		userProfile.LoginTimes = append(userProfile.LoginTimes, event.Timestamp)
		
		// Update common IPs
		if event.IPAddress != "" {
			userProfile.CommonIPs[event.IPAddress]++
		}
		
		// Update locations
		location := ad.getLocationFromIP(event.IPAddress)
		if location != "" {
			userProfile.CommonLocations[location]++
		}
	}
	
	// Update access patterns
	if event.EventType == "access" {
		pattern, exists := userProfile.AccessPatterns[event.Resource]
		if !exists {
			pattern = &AccessPattern{
				Resource:         event.Resource,
				TimeDistribution: make(map[int]float64),
			}
			userProfile.AccessPatterns[event.Resource] = pattern
		}
		
		// Update time distribution
		hour := event.Timestamp.Hour()
		pattern.TimeDistribution[hour]++
		pattern.LastAccess = event.Timestamp
		
		// Update data volume if available
		if volume, ok := event.Metadata["data_volume"].(float64); ok {
			// Moving average
			if pattern.DataVolume == 0 {
				pattern.DataVolume = volume
			} else {
				pattern.DataVolume = (pattern.DataVolume*0.9) + (volume*0.1)
			}
		}
	}
	
	ad.baseline.LastUpdated = time.Now()
}

// TrainModels trains the anomaly detection models
func (ad *AnomalyDetector) TrainModels() {
	ad.mu.Lock()
	defer ad.mu.Unlock()
	
	// Update login anomaly model parameters
	if len(ad.baseline.UserProfiles) > 10 {
		ad.updateLoginModelParameters()
	}
	
	// Update access anomaly model parameters
	ad.updateAccessModelParameters()
	
	// Mark models as trained
	for _, model := range ad.models {
		model.LastTraining = time.Now()
	}
}

// updateLoginModelParameters updates login model parameters from baseline
func (ad *AnomalyDetector) updateLoginModelParameters() {
	model := ad.models["login_anomaly"]
	
	// Calculate mean and std of login hours
	allLoginHours := make([]float64, 0)
	for _, profile := range ad.baseline.UserProfiles {
		for _, loginTime := range profile.LoginTimes {
			allLoginHours = append(allLoginHours, float64(loginTime.Hour()))
		}
	}
	
	if len(allLoginHours) > 0 {
		mean, std := calculateMeanStd(allLoginHours)
		model.Parameters["mean_login_hour"] = mean
		model.Parameters["std_login_hour"] = std
	}
}

// updateAccessModelParameters updates access model parameters
func (ad *AnomalyDetector) updateAccessModelParameters() {
	model := ad.models["access_anomaly"]
	
	// Calculate access rates and volumes
	accessRates := make([]float64, 0)
	dataVolumes := make([]float64, 0)
	
	for _, profile := range ad.baseline.UserProfiles {
		for _, pattern := range profile.AccessPatterns {
			// Calculate rate
			totalAccesses := 0.0
			for _, count := range pattern.TimeDistribution {
				totalAccesses += count
			}
			
			// Simple rate calculation
			rate := totalAccesses / 24.0 // per hour
			accessRates = append(accessRates, rate)
			
			if pattern.DataVolume > 0 {
				dataVolumes = append(dataVolumes, pattern.DataVolume)
			}
		}
	}
	
	if len(accessRates) > 0 {
		mean, std := calculateMeanStd(accessRates)
		model.Parameters["access_rate_mean"] = mean
		model.Parameters["access_rate_std"] = std
	}
	
	if len(dataVolumes) > 0 {
		mean, std := calculateMeanStd(dataVolumes)
		model.Parameters["data_volume_mean"] = mean
		model.Parameters["data_volume_std"] = std
	}
}

// baselineUpdater periodically updates the baseline
func (ad *AnomalyDetector) baselineUpdater(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// Clean old data
			ad.cleanOldBaselineData()
			
		case <-ctx.Done():
			return
		case <-ad.stopChan:
			return
		}
	}
}

// modelTrainer periodically trains models
func (ad *AnomalyDetector) modelTrainer(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			ad.TrainModels()
			
		case <-ctx.Done():
			return
		case <-ad.stopChan:
			return
		}
	}
}

// cleanOldBaselineData removes old baseline data
func (ad *AnomalyDetector) cleanOldBaselineData() {
	ad.mu.Lock()
	defer ad.mu.Unlock()
	
	cutoffTime := time.Now().Add(-30 * 24 * time.Hour) // 30 days
	
	for _, profile := range ad.baseline.UserProfiles {
		// Clean old login times
		newLoginTimes := make([]time.Time, 0)
		for _, loginTime := range profile.LoginTimes {
			if loginTime.After(cutoffTime) {
				newLoginTimes = append(newLoginTimes, loginTime)
			}
		}
		profile.LoginTimes = newLoginTimes
	}
}

// getLocationFromIP gets location from IP address
func (ad *AnomalyDetector) getLocationFromIP(ip string) string {
	// This would use a GeoIP database in production
	// For now, return a simulated location
	if ip == "" {
		return ""
	}
	
	// Simple simulation based on IP
	if strings.HasPrefix(ip, "192.168.") {
		return "Internal"
	} else if strings.HasPrefix(ip, "10.") {
		return "VPN"
	}
	
	return "External"
}

// Helper functions

func calculateMeanStd(values []float64) (mean, std float64) {
	if len(values) == 0 {
		return 0, 0
	}
	
	// Calculate mean
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean = sum / float64(len(values))
	
	// Calculate standard deviation
	variance := 0.0
	for _, v := range values {
		variance += math.Pow(v-mean, 2)
	}
	variance /= float64(len(values))
	std = math.Sqrt(variance)
	
	return mean, std
}

func generateAnomalyID() string {
	return fmt.Sprintf("ANOM_%d_%s", time.Now().UnixNano(), generateRandomString(6))
}