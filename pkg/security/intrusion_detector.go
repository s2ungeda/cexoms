package security

import (
	"context"
	"fmt"
	"math"
	"net"
	"strings"
	"sync"
	"time"
)

// IntrusionDetector detects and responds to security threats
type IntrusionDetector struct {
	mu                sync.RWMutex
	auditLogger       *AuditLogger
	anomalyDetector   *AnomalyDetector
	threatIntelligence *ThreatIntelligence
	rules             map[string]*DetectionRule
	alerts            map[string]*SecurityAlert
	incidents         map[string]*SecurityIncident
	responseActions   map[string]ResponseAction
	metrics           *DetectionMetrics
	config            *DetectionConfig
	running           bool
	stopChan          chan bool
}

// DetectionRule defines a security detection rule
type DetectionRule struct {
	ID          string
	Name        string
	Description string
	Category    RuleCategory
	Severity    ThreatSeverity
	Enabled     bool
	Conditions  []RuleCondition
	Actions     []string
	Threshold   int
	TimeWindow  time.Duration
	LastTriggered time.Time
}

// RuleCategory categorizes detection rules
type RuleCategory string

const (
	CategoryAuthentication RuleCategory = "authentication"
	CategoryAuthorization  RuleCategory = "authorization"
	CategoryDataExfiltration RuleCategory = "data_exfiltration"
	CategoryMaliciousActivity RuleCategory = "malicious_activity"
	CategoryAnomalous RuleCategory = "anomalous_behavior"
	CategoryCompliance RuleCategory = "compliance"
)

// ThreatSeverity defines threat severity levels
type ThreatSeverity string

const (
	SeverityCritical ThreatSeverity = "critical"
	SeverityHigh     ThreatSeverity = "high"
	SeverityMedium   ThreatSeverity = "medium"
	SeverityLow      ThreatSeverity = "low"
	SeverityInfo     ThreatSeverity = "info"
)

// RuleCondition defines conditions for rule matching
type RuleCondition struct {
	Field    string
	Operator string // eq, ne, gt, lt, contains, regex
	Value    interface{}
}

// SecurityAlert represents a security alert
type SecurityAlert struct {
	ID          string
	Timestamp   time.Time
	RuleID      string
	Severity    ThreatSeverity
	Category    RuleCategory
	Title       string
	Description string
	Source      string
	Target      string
	Evidence    []string
	RiskScore   float64
	Status      AlertStatus
}

// AlertStatus defines alert status
type AlertStatus string

const (
	AlertStatusNew         AlertStatus = "new"
	AlertStatusInvestigating AlertStatus = "investigating"
	AlertStatusConfirmed   AlertStatus = "confirmed"
	AlertStatusFalsePositive AlertStatus = "false_positive"
	AlertStatusResolved    AlertStatus = "resolved"
)

// SecurityIncident represents a security incident
type SecurityIncident struct {
	ID            string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Severity      ThreatSeverity
	Type          string
	Description   string
	ImpactedAssets []string
	RelatedAlerts []string
	Timeline      []IncidentEvent
	Status        IncidentStatus
	ResponsePlan  *ResponsePlan
}

// IncidentStatus defines incident status
type IncidentStatus string

const (
	IncidentStatusOpen        IncidentStatus = "open"
	IncidentStatusContained   IncidentStatus = "contained"
	IncidentStatusEradicated  IncidentStatus = "eradicated"
	IncidentStatusRecovered   IncidentStatus = "recovered"
	IncidentStatusClosed      IncidentStatus = "closed"
)

// IncidentEvent represents an event in incident timeline
type IncidentEvent struct {
	Timestamp   time.Time
	Description string
	Actor       string
	Action      string
	Result      string
}

// ResponsePlan defines incident response plan
type ResponsePlan struct {
	ID          string
	Name        string
	Steps       []ResponseStep
	Playbooks   []string
	Escalation  []EscalationLevel
}

// ResponseStep defines a response step
type ResponseStep struct {
	Order       int
	Name        string
	Description string
	Automated   bool
	Script      string
	Timeout     time.Duration
}

// EscalationLevel defines escalation hierarchy
type EscalationLevel struct {
	Level       int
	Threshold   time.Duration
	Contacts    []string
	Actions     []string
}

// ResponseAction defines automated response actions
type ResponseAction func(context.Context, *SecurityAlert) error

// DetectionConfig configures the intrusion detector
type DetectionConfig struct {
	EnableAnomalyDetection bool
	EnableThreatIntel      bool
	AlertThreshold         int
	IncidentThreshold      int
	ResponseTimeout        time.Duration
	MaxConcurrentResponses int
}

// DetectionMetrics tracks detection metrics
type DetectionMetrics struct {
	mu                sync.RWMutex
	EventsAnalyzed    int64
	AlertsGenerated   int64
	IncidentsCreated  int64
	FalsePositives    int64
	TruePositives     int64
	ResponsesExecuted int64
	AverageDetectionTime time.Duration
	CategoryMetrics   map[RuleCategory]*CategoryMetrics
}

// CategoryMetrics tracks per-category metrics
type CategoryMetrics struct {
	AlertCount    int64
	LastAlert     time.Time
	TopSources    map[string]int
	TopTargets    map[string]int
}

// NewIntrusionDetector creates a new intrusion detector
func NewIntrusionDetector(auditLogger *AuditLogger, config *DetectionConfig) *IntrusionDetector {
	id := &IntrusionDetector{
		auditLogger:       auditLogger,
		anomalyDetector:   NewAnomalyDetector(),
		threatIntelligence: NewThreatIntelligence(),
		rules:             make(map[string]*DetectionRule),
		alerts:            make(map[string]*SecurityAlert),
		incidents:         make(map[string]*SecurityIncident),
		responseActions:   make(map[string]ResponseAction),
		config:            config,
		metrics: &DetectionMetrics{
			CategoryMetrics: make(map[RuleCategory]*CategoryMetrics),
		},
		stopChan: make(chan bool),
	}
	
	// Initialize detection rules
	id.initializeRules()
	
	// Register response actions
	id.registerResponseActions()
	
	return id
}

// initializeRules sets up detection rules
func (id *IntrusionDetector) initializeRules() {
	// Authentication attacks
	id.rules["AUTH001"] = &DetectionRule{
		ID:          "AUTH001",
		Name:        "Brute Force Attack",
		Description: "Multiple failed login attempts from same IP",
		Category:    CategoryAuthentication,
		Severity:    SeverityHigh,
		Enabled:     true,
		Threshold:   5,
		TimeWindow:  5 * time.Minute,
		Conditions: []RuleCondition{
			{Field: "event_type", Operator: "eq", Value: "authentication"},
			{Field: "result", Operator: "eq", Value: "failure"},
		},
		Actions: []string{"block_ip", "notify_security"},
	}
	
	id.rules["AUTH002"] = &DetectionRule{
		ID:          "AUTH002",
		Name:        "Password Spray Attack",
		Description: "Same password tried on multiple accounts",
		Category:    CategoryAuthentication,
		Severity:    SeverityHigh,
		Enabled:     true,
		Threshold:   10,
		TimeWindow:  10 * time.Minute,
		Actions:     []string{"alert", "increase_monitoring"},
	}
	
	// Data exfiltration
	id.rules["DATA001"] = &DetectionRule{
		ID:          "DATA001",
		Name:        "Unusual Data Access",
		Description: "Large amount of data accessed in short time",
		Category:    CategoryDataExfiltration,
		Severity:    SeverityCritical,
		Enabled:     true,
		Threshold:   1000, // records
		TimeWindow:  1 * time.Hour,
		Actions:     []string{"alert", "limit_access", "notify_security"},
	}
	
	id.rules["DATA002"] = &DetectionRule{
		ID:          "DATA002",
		Name:        "Sensitive Data Access Pattern",
		Description: "Access to multiple sensitive data categories",
		Category:    CategoryDataExfiltration,
		Severity:    SeverityHigh,
		Enabled:     true,
		Threshold:   5, // different categories
		TimeWindow:  30 * time.Minute,
		Actions:     []string{"alert", "audit_trail"},
	}
	
	// Malicious activity
	id.rules["MAL001"] = &DetectionRule{
		ID:          "MAL001",
		Name:        "SQL Injection Attempt",
		Description: "Detected SQL injection patterns in requests",
		Category:    CategoryMaliciousActivity,
		Severity:    SeverityCritical,
		Enabled:     true,
		Threshold:   1,
		TimeWindow:  1 * time.Minute,
		Conditions: []RuleCondition{
			{Field: "request", Operator: "regex", Value: "(?i)(union.*select|or.*1.*=.*1|drop.*table)"},
		},
		Actions: []string{"block_request", "block_ip", "alert"},
	}
	
	id.rules["MAL002"] = &DetectionRule{
		ID:          "MAL002",
		Name:        "Command Injection Attempt",
		Description: "Detected command injection patterns",
		Category:    CategoryMaliciousActivity,
		Severity:    SeverityCritical,
		Enabled:     true,
		Threshold:   1,
		TimeWindow:  1 * time.Minute,
		Conditions: []RuleCondition{
			{Field: "request", Operator: "regex", Value: "(?i)(;|&&|\\||`|\\$\\()"},
		},
		Actions: []string{"block_request", "block_ip", "alert"},
	}
	
	// Anomalous behavior
	id.rules["ANOM001"] = &DetectionRule{
		ID:          "ANOM001",
		Name:        "Unusual Access Time",
		Description: "Access outside normal business hours",
		Category:    CategoryAnomalous,
		Severity:    SeverityMedium,
		Enabled:     true,
		Threshold:   1,
		TimeWindow:  1 * time.Hour,
		Actions:     []string{"alert", "require_mfa"},
	}
	
	id.rules["ANOM002"] = &DetectionRule{
		ID:          "ANOM002",
		Name:        "Unusual Geographic Location",
		Description: "Access from unusual location",
		Category:    CategoryAnomalous,
		Severity:    SeverityMedium,
		Enabled:     true,
		Threshold:   1,
		TimeWindow:  1 * time.Hour,
		Actions:     []string{"alert", "verify_identity"},
	}
}

// registerResponseActions registers automated response actions
func (id *IntrusionDetector) registerResponseActions() {
	// Block IP address
	id.responseActions["block_ip"] = func(ctx context.Context, alert *SecurityAlert) error {
		// Extract IP from alert
		ip := id.extractIP(alert)
		if ip == "" {
			return fmt.Errorf("no IP address found in alert")
		}
		
		// Add to blocklist
		// This would integrate with firewall/WAF
		fmt.Printf("Blocking IP address: %s\n", ip)
		
		// Log action
		id.auditLogger.LogSecurityEvent(ctx, "response_action", "high", map[string]interface{}{
			"action":   "block_ip",
			"ip":       ip,
			"alert_id": alert.ID,
			"reason":   alert.Description,
		})
		
		return nil
	}
	
	// Send security notification
	id.responseActions["notify_security"] = func(ctx context.Context, alert *SecurityAlert) error {
		// Send notification to security team
		// This would integrate with notification system
		fmt.Printf("Security Alert: %s - %s\n", alert.Title, alert.Description)
		
		return nil
	}
	
	// Increase monitoring
	id.responseActions["increase_monitoring"] = func(ctx context.Context, alert *SecurityAlert) error {
		// Increase logging level for affected resources
		fmt.Printf("Increasing monitoring for: %s\n", alert.Target)
		
		return nil
	}
	
	// Limit access
	id.responseActions["limit_access"] = func(ctx context.Context, alert *SecurityAlert) error {
		// Implement rate limiting or access restrictions
		fmt.Printf("Limiting access for: %s\n", alert.Source)
		
		return nil
	}
	
	// Require MFA
	id.responseActions["require_mfa"] = func(ctx context.Context, alert *SecurityAlert) error {
		// Force MFA for next authentication
		fmt.Printf("Requiring MFA for user: %s\n", alert.Source)
		
		return nil
	}
}

// Start starts the intrusion detector
func (id *IntrusionDetector) Start(ctx context.Context) error {
	id.mu.Lock()
	if id.running {
		id.mu.Unlock()
		return fmt.Errorf("detector already running")
	}
	id.running = true
	id.mu.Unlock()
	
	// Start detection goroutines
	go id.eventAnalyzer(ctx)
	go id.alertCorrelator(ctx)
	go id.incidentManager(ctx)
	go id.metricsCollector(ctx)
	
	// Start anomaly detection if enabled
	if id.config.EnableAnomalyDetection {
		go id.anomalyDetector.Start(ctx)
	}
	
	// Start threat intelligence if enabled
	if id.config.EnableThreatIntel {
		go id.threatIntelligence.Start(ctx)
	}
	
	return nil
}

// Stop stops the intrusion detector
func (id *IntrusionDetector) Stop() error {
	id.mu.Lock()
	if !id.running {
		id.mu.Unlock()
		return fmt.Errorf("detector not running")
	}
	id.running = false
	id.mu.Unlock()
	
	close(id.stopChan)
	
	// Stop components
	if id.config.EnableAnomalyDetection {
		id.anomalyDetector.Stop()
	}
	
	if id.config.EnableThreatIntel {
		id.threatIntelligence.Stop()
	}
	
	return nil
}

// AnalyzeEvent analyzes a security event
func (id *IntrusionDetector) AnalyzeEvent(event *AuditEvent) {
	id.mu.Lock()
	id.metrics.EventsAnalyzed++
	id.mu.Unlock()
	
	// Check against detection rules
	for _, rule := range id.rules {
		if !rule.Enabled {
			continue
		}
		
		if id.matchesRule(event, rule) {
			id.handleRuleMatch(event, rule)
		}
	}
	
	// Check with anomaly detector
	if id.config.EnableAnomalyDetection {
		if anomaly := id.anomalyDetector.Analyze(event); anomaly != nil {
			id.handleAnomaly(anomaly)
		}
	}
	
	// Check threat intelligence
	if id.config.EnableThreatIntel {
		if threat := id.threatIntelligence.Check(event); threat != nil {
			id.handleThreatIntel(threat)
		}
	}
}

// matchesRule checks if event matches rule conditions
func (id *IntrusionDetector) matchesRule(event *AuditEvent, rule *DetectionRule) bool {
	// Check basic conditions
	if rule.Category == CategoryAuthentication && event.EventType != "authentication" {
		return false
	}
	
	// Check all conditions
	for _, condition := range rule.Conditions {
		if !id.evaluateCondition(event, condition) {
			return false
		}
	}
	
	// Check threshold within time window
	count := id.countMatchingEvents(rule, rule.TimeWindow)
	return count >= rule.Threshold
}

// evaluateCondition evaluates a single condition
func (id *IntrusionDetector) evaluateCondition(event *AuditEvent, condition RuleCondition) bool {
	var fieldValue interface{}
	
	// Extract field value from event
	switch condition.Field {
	case "event_type":
		fieldValue = event.EventType
	case "result":
		fieldValue = event.Result
	case "user_id":
		fieldValue = event.UserID
	case "ip_address":
		fieldValue = event.IPAddress
	case "action":
		fieldValue = event.Action
	default:
		if val, exists := event.Metadata[condition.Field]; exists {
			fieldValue = val
		}
	}
	
	// Evaluate operator
	switch condition.Operator {
	case "eq":
		return fieldValue == condition.Value
	case "ne":
		return fieldValue != condition.Value
	case "contains":
		return strings.Contains(fmt.Sprintf("%v", fieldValue), fmt.Sprintf("%v", condition.Value))
	case "regex":
		// Regex matching would be implemented here
		return false
	case "gt", "lt":
		// Numeric comparison would be implemented here
		return false
	default:
		return false
	}
}

// handleRuleMatch handles a rule match
func (id *IntrusionDetector) handleRuleMatch(event *AuditEvent, rule *DetectionRule) {
	// Create alert
	alert := &SecurityAlert{
		ID:          generateAlertID(),
		Timestamp:   time.Now(),
		RuleID:      rule.ID,
		Severity:    rule.Severity,
		Category:    rule.Category,
		Title:       rule.Name,
		Description: fmt.Sprintf("%s detected for %s", rule.Description, event.UserID),
		Source:      event.IPAddress,
		Target:      event.Resource,
		Evidence:    []string{event.ID},
		RiskScore:   id.calculateRiskScore(rule.Severity, event),
		Status:      AlertStatusNew,
	}
	
	// Store alert
	id.mu.Lock()
	id.alerts[alert.ID] = alert
	id.metrics.AlertsGenerated++
	
	// Update category metrics
	if catMetrics, exists := id.metrics.CategoryMetrics[rule.Category]; exists {
		catMetrics.AlertCount++
		catMetrics.LastAlert = time.Now()
	} else {
		id.metrics.CategoryMetrics[rule.Category] = &CategoryMetrics{
			AlertCount: 1,
			LastAlert:  time.Now(),
			TopSources: make(map[string]int),
			TopTargets: make(map[string]int),
		}
	}
	id.mu.Unlock()
	
	// Execute response actions
	for _, action := range rule.Actions {
		if responseFunc, exists := id.responseActions[action]; exists {
			go func(act string) {
				ctx := context.Background()
				if err := responseFunc(ctx, alert); err != nil {
					fmt.Printf("Response action %s failed: %v\n", act, err)
				} else {
					id.mu.Lock()
					id.metrics.ResponsesExecuted++
					id.mu.Unlock()
				}
			}(action)
		}
	}
	
	// Update rule
	rule.LastTriggered = time.Now()
}

// calculateRiskScore calculates risk score for an alert
func (id *IntrusionDetector) calculateRiskScore(severity ThreatSeverity, event *AuditEvent) float64 {
	baseScore := 0.0
	
	// Base score by severity
	switch severity {
	case SeverityCritical:
		baseScore = 90.0
	case SeverityHigh:
		baseScore = 70.0
	case SeverityMedium:
		baseScore = 50.0
	case SeverityLow:
		baseScore = 30.0
	case SeverityInfo:
		baseScore = 10.0
	}
	
	// Adjust based on context
	if event.UserID != "" {
		// Known user slightly lower risk
		baseScore *= 0.9
	}
	
	// Check threat intelligence
	if id.config.EnableThreatIntel {
		if id.threatIntelligence.IsKnownBadIP(event.IPAddress) {
			baseScore *= 1.5
		}
	}
	
	// Cap at 100
	if baseScore > 100 {
		baseScore = 100
	}
	
	return baseScore
}

// countMatchingEvents counts events matching rule in time window
func (id *IntrusionDetector) countMatchingEvents(rule *DetectionRule, window time.Duration) int {
	// This would query recent events matching rule conditions
	// For now, return a simulated count
	return rule.Threshold + 1
}

// eventAnalyzer analyzes incoming events
func (id *IntrusionDetector) eventAnalyzer(ctx context.Context) {
	// This would receive events from audit logger
	// For now, it's a placeholder
	<-ctx.Done()
}

// alertCorrelator correlates alerts into incidents
func (id *IntrusionDetector) alertCorrelator(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			id.correlateAlerts()
		case <-ctx.Done():
			return
		case <-id.stopChan:
			return
		}
	}
}

// correlateAlerts correlates alerts into incidents
func (id *IntrusionDetector) correlateAlerts() {
	id.mu.Lock()
	defer id.mu.Unlock()
	
	// Group alerts by source and time window
	alertGroups := make(map[string][]*SecurityAlert)
	
	for _, alert := range id.alerts {
		if alert.Status == AlertStatusNew || alert.Status == AlertStatusInvestigating {
			key := fmt.Sprintf("%s_%s", alert.Source, alert.Category)
			alertGroups[key] = append(alertGroups[key], alert)
		}
	}
	
	// Create incidents for significant alert groups
	for key, alerts := range alertGroups {
		if len(alerts) >= id.config.IncidentThreshold {
			incident := id.createIncident(alerts)
			id.incidents[incident.ID] = incident
			id.metrics.IncidentsCreated++
			
			// Update alert status
			for _, alert := range alerts {
				alert.Status = AlertStatusConfirmed
			}
		}
	}
}

// createIncident creates a security incident from alerts
func (id *IntrusionDetector) createIncident(alerts []*SecurityAlert) *SecurityIncident {
	// Determine incident severity
	maxSeverity := SeverityLow
	for _, alert := range alerts {
		if id.severityValue(alert.Severity) > id.severityValue(maxSeverity) {
			maxSeverity = alert.Severity
		}
	}
	
	// Create incident
	incident := &SecurityIncident{
		ID:          generateIncidentID(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Severity:    maxSeverity,
		Type:        string(alerts[0].Category),
		Description: fmt.Sprintf("Multiple security alerts detected: %s", alerts[0].Title),
		Status:      IncidentStatusOpen,
		Timeline:    make([]IncidentEvent, 0),
	}
	
	// Add related alerts
	for _, alert := range alerts {
		incident.RelatedAlerts = append(incident.RelatedAlerts, alert.ID)
		
		// Add to timeline
		incident.Timeline = append(incident.Timeline, IncidentEvent{
			Timestamp:   alert.Timestamp,
			Description: alert.Description,
			Actor:       alert.Source,
			Action:      "alert_triggered",
			Result:      string(alert.Status),
		})
	}
	
	// Create response plan
	incident.ResponsePlan = id.createResponsePlan(incident)
	
	return incident
}

// createResponsePlan creates response plan for incident
func (id *IntrusionDetector) createResponsePlan(incident *SecurityIncident) *ResponsePlan {
	plan := &ResponsePlan{
		ID:   generatePlanID(),
		Name: fmt.Sprintf("Response Plan for %s", incident.Type),
		Steps: []ResponseStep{
			{
				Order:       1,
				Name:        "Initial Assessment",
				Description: "Assess the scope and impact of the incident",
				Automated:   false,
				Timeout:     15 * time.Minute,
			},
			{
				Order:       2,
				Name:        "Containment",
				Description: "Contain the threat to prevent further damage",
				Automated:   true,
				Script:      "contain_threat.sh",
				Timeout:     5 * time.Minute,
			},
			{
				Order:       3,
				Name:        "Eradication",
				Description: "Remove the threat from the environment",
				Automated:   false,
				Timeout:     30 * time.Minute,
			},
			{
				Order:       4,
				Name:        "Recovery",
				Description: "Restore normal operations",
				Automated:   false,
				Timeout:     60 * time.Minute,
			},
			{
				Order:       5,
				Name:        "Post-Incident Review",
				Description: "Review and document lessons learned",
				Automated:   false,
				Timeout:     24 * time.Hour,
			},
		},
		Escalation: []EscalationLevel{
			{
				Level:     1,
				Threshold: 30 * time.Minute,
				Contacts:  []string{"security-team@example.com"},
				Actions:   []string{"email", "slack"},
			},
			{
				Level:     2,
				Threshold: 1 * time.Hour,
				Contacts:  []string{"security-manager@example.com"},
				Actions:   []string{"email", "phone", "pagerduty"},
			},
			{
				Level:     3,
				Threshold: 2 * time.Hour,
				Contacts:  []string{"ciso@example.com", "cto@example.com"},
				Actions:   []string{"email", "phone", "emergency_meeting"},
			},
		},
	}
	
	return plan
}

// incidentManager manages security incidents
func (id *IntrusionDetector) incidentManager(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			id.updateIncidents()
		case <-ctx.Done():
			return
		case <-id.stopChan:
			return
		}
	}
}

// updateIncidents updates incident status
func (id *IntrusionDetector) updateIncidents() {
	id.mu.Lock()
	defer id.mu.Unlock()
	
	for _, incident := range id.incidents {
		if incident.Status == IncidentStatusClosed {
			continue
		}
		
		// Check escalation
		elapsed := time.Since(incident.CreatedAt)
		for _, escalation := range incident.ResponsePlan.Escalation {
			if elapsed > escalation.Threshold {
				// Trigger escalation
				fmt.Printf("Escalating incident %s to level %d\n", incident.ID, escalation.Level)
			}
		}
		
		// Update incident based on response progress
		// This would check actual response status
		incident.UpdatedAt = time.Now()
	}
}

// metricsCollector collects detection metrics
func (id *IntrusionDetector) metricsCollector(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			id.collectMetrics()
		case <-ctx.Done():
			return
		case <-id.stopChan:
			return
		}
	}
}

// collectMetrics collects current metrics
func (id *IntrusionDetector) collectMetrics() {
	id.mu.RLock()
	defer id.mu.RUnlock()
	
	// Calculate detection time
	totalDetectionTime := time.Duration(0)
	detectionCount := 0
	
	for _, alert := range id.alerts {
		// This would calculate actual detection time
		totalDetectionTime += 5 * time.Second // Simulated
		detectionCount++
	}
	
	if detectionCount > 0 {
		id.metrics.AverageDetectionTime = totalDetectionTime / time.Duration(detectionCount)
	}
}

// Helper functions

func (id *IntrusionDetector) extractIP(alert *SecurityAlert) string {
	// Extract IP from source
	if net.ParseIP(alert.Source) != nil {
		return alert.Source
	}
	
	// Try to extract from evidence
	for _, evidence := range alert.Evidence {
		if ip := net.ParseIP(evidence); ip != nil {
			return evidence
		}
	}
	
	return ""
}

func (id *IntrusionDetector) severityValue(severity ThreatSeverity) int {
	switch severity {
	case SeverityCritical:
		return 5
	case SeverityHigh:
		return 4
	case SeverityMedium:
		return 3
	case SeverityLow:
		return 2
	case SeverityInfo:
		return 1
	default:
		return 0
	}
}

func (id *IntrusionDetector) handleAnomaly(anomaly *Anomaly) {
	// Convert anomaly to alert
	alert := &SecurityAlert{
		ID:          generateAlertID(),
		Timestamp:   time.Now(),
		RuleID:      "ANOMALY",
		Severity:    id.anomalySeverity(anomaly.Score),
		Category:    CategoryAnomalous,
		Title:       "Anomalous Behavior Detected",
		Description: anomaly.Description,
		Source:      anomaly.Source,
		Target:      anomaly.Target,
		Evidence:    anomaly.Evidence,
		RiskScore:   anomaly.Score,
		Status:      AlertStatusNew,
	}
	
	id.mu.Lock()
	id.alerts[alert.ID] = alert
	id.metrics.AlertsGenerated++
	id.mu.Unlock()
}

func (id *IntrusionDetector) anomalySeverity(score float64) ThreatSeverity {
	switch {
	case score >= 90:
		return SeverityCritical
	case score >= 70:
		return SeverityHigh
	case score >= 50:
		return SeverityMedium
	case score >= 30:
		return SeverityLow
	default:
		return SeverityInfo
	}
}

func (id *IntrusionDetector) handleThreatIntel(threat *ThreatIndicator) {
	// Convert threat indicator to alert
	alert := &SecurityAlert{
		ID:          generateAlertID(),
		Timestamp:   time.Now(),
		RuleID:      "THREAT_INTEL",
		Severity:    threat.Severity,
		Category:    CategoryMaliciousActivity,
		Title:       "Known Threat Detected",
		Description: threat.Description,
		Source:      threat.Indicator,
		Evidence:    []string{threat.Source},
		RiskScore:   90.0, // High risk for known threats
		Status:      AlertStatusConfirmed,
	}
	
	id.mu.Lock()
	id.alerts[alert.ID] = alert
	id.metrics.AlertsGenerated++
	id.mu.Unlock()
}

// GetMetrics returns current detection metrics
func (id *IntrusionDetector) GetMetrics() *DetectionMetrics {
	id.mu.RLock()
	defer id.mu.RUnlock()
	
	// Create a copy of metrics
	metricsCopy := &DetectionMetrics{
		EventsAnalyzed:       id.metrics.EventsAnalyzed,
		AlertsGenerated:      id.metrics.AlertsGenerated,
		IncidentsCreated:     id.metrics.IncidentsCreated,
		FalsePositives:       id.metrics.FalsePositives,
		TruePositives:        id.metrics.TruePositives,
		ResponsesExecuted:    id.metrics.ResponsesExecuted,
		AverageDetectionTime: id.metrics.AverageDetectionTime,
		CategoryMetrics:      make(map[RuleCategory]*CategoryMetrics),
	}
	
	// Copy category metrics
	for cat, metrics := range id.metrics.CategoryMetrics {
		metricsCopy.CategoryMetrics[cat] = &CategoryMetrics{
			AlertCount: metrics.AlertCount,
			LastAlert:  metrics.LastAlert,
			TopSources: make(map[string]int),
			TopTargets: make(map[string]int),
		}
		
		// Copy top sources/targets
		for k, v := range metrics.TopSources {
			metricsCopy.CategoryMetrics[cat].TopSources[k] = v
		}
		for k, v := range metrics.TopTargets {
			metricsCopy.CategoryMetrics[cat].TopTargets[k] = v
		}
	}
	
	return metricsCopy
}

// Helper ID generators
func generateAlertID() string {
	return fmt.Sprintf("ALERT_%d_%s", time.Now().UnixNano(), generateRandomString(6))
}

func generateIncidentID() string {
	return fmt.Sprintf("INC_%d_%s", time.Now().Unix(), generateRandomString(6))
}

func generatePlanID() string {
	return fmt.Sprintf("PLAN_%d_%s", time.Now().Unix(), generateRandomString(6))
}