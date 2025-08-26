package security

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ComplianceReporter generates compliance reports
type ComplianceReporter struct {
	complianceManager *ComplianceManager
	auditLogger       *AuditLogger
	tamperProofLogger *TamperProofLogger
	templates         map[string]*template.Template
	reportPath        string
}

// ReportFormat defines report output format
type ReportFormat string

const (
	FormatHTML ReportFormat = "html"
	FormatJSON ReportFormat = "json"
	FormatPDF  ReportFormat = "pdf"
	FormatCSV  ReportFormat = "csv"
)

// ComplianceReportData contains all report data
type ComplianceReportData struct {
	ReportID         string
	GeneratedAt      time.Time
	Period           ReportPeriod
	Regulations      []RegulationReport
	OverallScore     float64
	ExecutiveSummary string
	Recommendations  []string
	Incidents        []ComplianceIncident
	Trends           []ComplianceTrend
}

// ReportPeriod defines the report time period
type ReportPeriod struct {
	StartDate time.Time
	EndDate   time.Time
	Type      string // daily, weekly, monthly, quarterly, annual
}

// RegulationReport contains compliance data for a regulation
type RegulationReport struct {
	RegulationID    string
	RegulationName  string
	ComplianceScore float64
	Status          ComplianceStatus
	Requirements    []RequirementReport
	Violations      []ComplianceViolation
	Improvements    []string
}

// RequirementReport contains requirement compliance data
type RequirementReport struct {
	RequirementID   string
	Description     string
	Category        string
	Status          ComplianceStatus
	LastChecked     time.Time
	Evidence        []Evidence
	Notes           string
}

// Evidence represents compliance evidence
type Evidence struct {
	Type        string    // log, screenshot, document, configuration
	Description string
	Location    string
	Timestamp   time.Time
	Hash        string
}

// ComplianceViolation represents a compliance violation
type ComplianceViolation struct {
	ID               string
	RequirementID    string
	Severity         string // critical, high, medium, low
	Description      string
	DetectedAt       time.Time
	RemediatedAt     time.Time
	RemediationSteps []string
	Status           string // open, in_progress, remediated
}

// ComplianceIncident represents a compliance incident
type ComplianceIncident struct {
	ID            string
	Type          string
	Severity      string
	Description   string
	OccurredAt    time.Time
	ReportedAt    time.Time
	ResolvedAt    time.Time
	ImpactedUsers int
	RootCause     string
	Actions       []string
}

// ComplianceTrend represents compliance trend data
type ComplianceTrend struct {
	Metric    string
	Period    string
	Values    []float64
	Direction string // improving, stable, declining
}

// NewComplianceReporter creates a new compliance reporter
func NewComplianceReporter(cm *ComplianceManager, al *AuditLogger, tpl *TamperProofLogger, reportPath string) (*ComplianceReporter, error) {
	cr := &ComplianceReporter{
		complianceManager: cm,
		auditLogger:       al,
		tamperProofLogger: tpl,
		reportPath:        reportPath,
		templates:         make(map[string]*template.Template),
	}
	
	// Initialize templates
	if err := cr.initializeTemplates(); err != nil {
		return nil, fmt.Errorf("failed to initialize templates: %w", err)
	}
	
	return cr, nil
}

// GenerateReport generates a compliance report
func (cr *ComplianceReporter) GenerateReport(period ReportPeriod, format ReportFormat) (*ComplianceReportData, error) {
	// Collect compliance data
	report := &ComplianceReportData{
		ReportID:    generateReportID(),
		GeneratedAt: time.Now(),
		Period:      period,
		Regulations: make([]RegulationReport, 0),
		Incidents:   make([]ComplianceIncident, 0),
		Trends:      make([]ComplianceTrend, 0),
	}
	
	// Get compliance status for all regulations
	complianceStatus := cr.complianceManager.GetComplianceStatus()
	
	// Generate report for each regulation
	for regulationID, status := range complianceStatus {
		regReport, err := cr.generateRegulationReport(regulationID, status, period)
		if err != nil {
			return nil, fmt.Errorf("failed to generate report for %s: %w", regulationID, err)
		}
		report.Regulations = append(report.Regulations, *regReport)
	}
	
	// Calculate overall compliance score
	report.OverallScore = cr.calculateOverallScore(report.Regulations)
	
	// Generate executive summary
	report.ExecutiveSummary = cr.generateExecutiveSummary(report)
	
	// Add recommendations
	report.Recommendations = cr.generateRecommendations(report)
	
	// Collect incidents
	incidents, err := cr.collectIncidents(period)
	if err != nil {
		return nil, fmt.Errorf("failed to collect incidents: %w", err)
	}
	report.Incidents = incidents
	
	// Analyze trends
	report.Trends = cr.analyzeTrends(period)
	
	// Save report
	if err := cr.saveReport(report, format); err != nil {
		return nil, fmt.Errorf("failed to save report: %w", err)
	}
	
	return report, nil
}

// generateRegulationReport generates report for a regulation
func (cr *ComplianceReporter) generateRegulationReport(regulationID string, status ComplianceStatus, period ReportPeriod) (*RegulationReport, error) {
	// Run compliance check
	ctx := context.Background()
	complianceReport, err := cr.complianceManager.CheckCompliance(ctx, regulationID)
	if err != nil {
		return nil, err
	}
	
	regReport := &RegulationReport{
		RegulationID:    regulationID,
		RegulationName:  cr.getRegulationName(regulationID),
		ComplianceScore: complianceReport.ComplianceScore,
		Status:          status,
		Requirements:    make([]RequirementReport, 0),
		Violations:      make([]ComplianceViolation, 0),
		Improvements:    make([]string, 0),
	}
	
	// Process each requirement
	for _, result := range complianceReport.Results {
		reqReport := RequirementReport{
			RequirementID: result.RequirementID,
			Description:   result.Description,
			Category:      cr.getRequirementCategory(regulationID, result.RequirementID),
			Status:        cr.determineRequirementStatus(result.Compliant),
			LastChecked:   result.CheckedAt,
			Evidence:      cr.collectEvidence(regulationID, result.RequirementID, period),
			Notes:         result.Details,
		}
		
		regReport.Requirements = append(regReport.Requirements, reqReport)
		
		// Check for violations
		if !result.Compliant {
			violation := ComplianceViolation{
				ID:            generateViolationID(),
				RequirementID: result.RequirementID,
				Severity:      cr.determineViolationSeverity(regulationID, result.RequirementID),
				Description:   fmt.Sprintf("Non-compliant with %s", result.Description),
				DetectedAt:    result.CheckedAt,
				Status:        "open",
			}
			regReport.Violations = append(regReport.Violations, violation)
		}
	}
	
	// Generate improvements
	regReport.Improvements = cr.generateImprovements(regReport)
	
	return regReport, nil
}

// collectEvidence collects evidence for a requirement
func (cr *ComplianceReporter) collectEvidence(regulationID, requirementID string, period ReportPeriod) []Evidence {
	evidence := make([]Evidence, 0)
	
	// Collect audit logs as evidence
	filter := AuditFilter{
		StartTime: period.StartDate,
		EndTime:   period.EndDate,
		EventType: "compliance",
	}
	
	events, err := cr.auditLogger.Query(filter)
	if err == nil {
		for _, event := range events {
			if cr.isRelevantEvidence(event, regulationID, requirementID) {
				evidence = append(evidence, Evidence{
					Type:        "log",
					Description: fmt.Sprintf("Audit log: %s", event.Action),
					Location:    fmt.Sprintf("audit_log_%s", event.ID),
					Timestamp:   event.Timestamp,
					Hash:        cr.calculateEvidenceHash(event),
				})
			}
		}
	}
	
	// Collect tamper-proof logs as evidence
	logFilter := LogFilter{
		StartTime: period.StartDate,
		EndTime:   period.EndDate,
		EventType: "security",
	}
	
	logs, err := cr.tamperProofLogger.SearchLogs(logFilter)
	if err == nil {
		for _, log := range logs {
			evidence = append(evidence, Evidence{
				Type:        "tamper_proof_log",
				Description: fmt.Sprintf("Tamper-proof log: %s", log.Action),
				Location:    fmt.Sprintf("tamper_proof_%s", log.ID),
				Timestamp:   log.Timestamp,
				Hash:        log.Hash,
			})
		}
	}
	
	return evidence
}

// saveReport saves the report in specified format
func (cr *ComplianceReporter) saveReport(report *ComplianceReportData, format ReportFormat) error {
	filename := filepath.Join(cr.reportPath, fmt.Sprintf("compliance_report_%s_%s.%s",
		report.ReportID,
		report.GeneratedAt.Format("20060102_150405"),
		string(format)))
	
	switch format {
	case FormatJSON:
		return cr.saveJSONReport(report, filename)
		
	case FormatHTML:
		return cr.saveHTMLReport(report, filename)
		
	case FormatCSV:
		return cr.saveCSVReport(report, filename)
		
	case FormatPDF:
		// PDF generation would require additional libraries
		return fmt.Errorf("PDF format not yet implemented")
		
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

// saveJSONReport saves report as JSON
func (cr *ComplianceReporter) saveJSONReport(report *ComplianceReportData, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()
	
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	
	return nil
}

// saveHTMLReport saves report as HTML
func (cr *ComplianceReporter) saveHTMLReport(report *ComplianceReportData, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()
	
	tmpl, exists := cr.templates["compliance_report"]
	if !exists {
		return fmt.Errorf("HTML template not found")
	}
	
	if err := tmpl.Execute(file, report); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}
	
	return nil
}

// saveCSVReport saves report as CSV
func (cr *ComplianceReporter) saveCSVReport(report *ComplianceReportData, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()
	
	writer := csv.NewWriter(file)
	defer writer.Flush()
	
	// Write header
	header := []string{
		"Regulation", "Requirement", "Category", "Status", "Score", "Last Checked", "Notes",
	}
	if err := writer.Write(header); err != nil {
		return err
	}
	
	// Write data
	for _, reg := range report.Regulations {
		for _, req := range reg.Requirements {
			row := []string{
				reg.RegulationName,
				req.RequirementID,
				req.Category,
				string(req.Status),
				fmt.Sprintf("%.2f", reg.ComplianceScore),
				req.LastChecked.Format("2006-01-02 15:04:05"),
				req.Notes,
			}
			if err := writer.Write(row); err != nil {
				return err
			}
		}
	}
	
	return nil
}

// Helper functions

func (cr *ComplianceReporter) initializeTemplates() error {
	// HTML template for compliance report
	htmlTemplate := `
<!DOCTYPE html>
<html>
<head>
    <title>Compliance Report - {{.ReportID}}</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        h1 { color: #333; }
        .summary { background: #f0f0f0; padding: 15px; border-radius: 5px; }
        .score { font-size: 24px; font-weight: bold; }
        .compliant { color: green; }
        .non-compliant { color: red; }
        .partial { color: orange; }
        table { border-collapse: collapse; width: 100%; margin-top: 20px; }
        th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
        th { background-color: #4CAF50; color: white; }
        .incident { background: #ffebee; padding: 10px; margin: 10px 0; border-radius: 5px; }
    </style>
</head>
<body>
    <h1>Compliance Report</h1>
    <div class="summary">
        <p><strong>Report ID:</strong> {{.ReportID}}</p>
        <p><strong>Generated:</strong> {{.GeneratedAt.Format "2006-01-02 15:04:05"}}</p>
        <p><strong>Period:</strong> {{.Period.StartDate.Format "2006-01-02"}} to {{.Period.EndDate.Format "2006-01-02"}}</p>
        <p class="score">Overall Compliance Score: <span class="{{if ge .OverallScore 90}}compliant{{else if ge .OverallScore 70}}partial{{else}}non-compliant{{end}}">{{printf "%.1f%%" .OverallScore}}</span></p>
    </div>
    
    <h2>Executive Summary</h2>
    <p>{{.ExecutiveSummary}}</p>
    
    <h2>Compliance by Regulation</h2>
    {{range .Regulations}}
    <h3>{{.RegulationName}}</h3>
    <p>Score: {{printf "%.1f%%" .ComplianceScore}} - Status: {{.Status}}</p>
    
    <table>
        <tr>
            <th>Requirement</th>
            <th>Category</th>
            <th>Status</th>
            <th>Last Checked</th>
            <th>Evidence</th>
        </tr>
        {{range .Requirements}}
        <tr>
            <td>{{.Description}}</td>
            <td>{{.Category}}</td>
            <td class="{{if eq .Status "compliant"}}compliant{{else if eq .Status "partial"}}partial{{else}}non-compliant{{end}}">{{.Status}}</td>
            <td>{{.LastChecked.Format "2006-01-02 15:04:05"}}</td>
            <td>{{len .Evidence}} items</td>
        </tr>
        {{end}}
    </table>
    
    {{if .Violations}}
    <h4>Violations</h4>
    {{range .Violations}}
    <div class="incident">
        <p><strong>{{.Severity}}</strong>: {{.Description}}</p>
        <p>Detected: {{.DetectedAt.Format "2006-01-02 15:04:05"}} - Status: {{.Status}}</p>
    </div>
    {{end}}
    {{end}}
    {{end}}
    
    <h2>Recommendations</h2>
    <ul>
    {{range .Recommendations}}
        <li>{{.}}</li>
    {{end}}
    </ul>
    
    {{if .Incidents}}
    <h2>Compliance Incidents</h2>
    {{range .Incidents}}
    <div class="incident">
        <h4>{{.Type}} - {{.Severity}}</h4>
        <p>{{.Description}}</p>
        <p>Occurred: {{.OccurredAt.Format "2006-01-02 15:04:05"}}</p>
        <p>Impact: {{.ImpactedUsers}} users</p>
    </div>
    {{end}}
    {{end}}
</body>
</html>
`
	
	tmpl, err := template.New("compliance_report").Parse(htmlTemplate)
	if err != nil {
		return err
	}
	
	cr.templates["compliance_report"] = tmpl
	return nil
}

func (cr *ComplianceReporter) calculateOverallScore(regulations []RegulationReport) float64 {
	if len(regulations) == 0 {
		return 0
	}
	
	totalScore := 0.0
	for _, reg := range regulations {
		totalScore += reg.ComplianceScore
	}
	
	return totalScore / float64(len(regulations))
}

func (cr *ComplianceReporter) generateExecutiveSummary(report *ComplianceReportData) string {
	compliantCount := 0
	for _, reg := range report.Regulations {
		if reg.Status == StatusCompliant {
			compliantCount++
		}
	}
	
	summary := fmt.Sprintf("Overall compliance score: %.1f%%. ", report.OverallScore)
	summary += fmt.Sprintf("%d of %d regulations are fully compliant. ", compliantCount, len(report.Regulations))
	
	if len(report.Incidents) > 0 {
		summary += fmt.Sprintf("%d compliance incidents occurred during the reporting period. ", len(report.Incidents))
	}
	
	if report.OverallScore >= 90 {
		summary += "The organization maintains a high level of compliance."
	} else if report.OverallScore >= 70 {
		summary += "The organization shows partial compliance with room for improvement."
	} else {
		summary += "Immediate action is required to address compliance gaps."
	}
	
	return summary
}

func (cr *ComplianceReporter) generateRecommendations(report *ComplianceReportData) []string {
	recommendations := make([]string, 0)
	
	// Check for non-compliant requirements
	for _, reg := range report.Regulations {
		if reg.Status != StatusCompliant {
			for _, req := range reg.Requirements {
				if req.Status != StatusCompliant {
					rec := fmt.Sprintf("Address %s requirement: %s", reg.RegulationName, req.Description)
					recommendations = append(recommendations, rec)
				}
			}
		}
	}
	
	// Check for critical violations
	criticalCount := 0
	for _, reg := range report.Regulations {
		for _, violation := range reg.Violations {
			if violation.Severity == "critical" {
				criticalCount++
			}
		}
	}
	
	if criticalCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("Prioritize remediation of %d critical violations", criticalCount))
	}
	
	// General recommendations based on score
	if report.OverallScore < 70 {
		recommendations = append(recommendations, "Conduct comprehensive compliance audit")
		recommendations = append(recommendations, "Implement automated compliance monitoring")
	}
	
	if report.OverallScore < 90 {
		recommendations = append(recommendations, "Enhance security controls and documentation")
		recommendations = append(recommendations, "Provide compliance training to staff")
	}
	
	return recommendations
}

func (cr *ComplianceReporter) collectIncidents(period ReportPeriod) ([]ComplianceIncident, error) {
	// This would collect actual incidents from your incident management system
	incidents := make([]ComplianceIncident, 0)
	
	// For now, return empty list
	return incidents, nil
}

func (cr *ComplianceReporter) analyzeTrends(period ReportPeriod) []ComplianceTrend {
	trends := make([]ComplianceTrend, 0)
	
	// This would analyze historical data to identify trends
	// For now, return sample trends
	
	trends = append(trends, ComplianceTrend{
		Metric:    "Overall Compliance Score",
		Period:    "monthly",
		Values:    []float64{85.2, 86.5, 88.1, 89.3, 91.2},
		Direction: "improving",
	})
	
	trends = append(trends, ComplianceTrend{
		Metric:    "Security Incidents",
		Period:    "monthly",
		Values:    []float64{12, 10, 8, 6, 4},
		Direction: "improving",
	})
	
	return trends
}

func (cr *ComplianceReporter) getRegulationName(regulationID string) string {
	names := map[string]string{
		"GDPR":    "General Data Protection Regulation",
		"SOC2":    "Service Organization Control 2",
		"MiFID2":  "Markets in Financial Instruments Directive II",
		"CCPA":    "California Consumer Privacy Act",
		"ISO27001": "ISO/IEC 27001 Information Security",
	}
	
	if name, exists := names[regulationID]; exists {
		return name
	}
	return regulationID
}

func (cr *ComplianceReporter) getRequirementCategory(regulationID, requirementID string) string {
	// This would look up the actual category
	// For now, extract from requirement ID
	parts := strings.Split(requirementID, "-")
	if len(parts) >= 2 {
		categoryNum := parts[1]
		categories := map[string]string{
			"1": "Data Protection",
			"2": "Privacy Rights",
			"3": "Data Access",
			"4": "Consent Management",
			"5": "Audit Trail",
		}
		if cat, exists := categories[categoryNum]; exists {
			return cat
		}
	}
	return "General"
}

func (cr *ComplianceReporter) determineRequirementStatus(compliant bool) ComplianceStatus {
	if compliant {
		return StatusCompliant
	}
	return StatusNonCompliant
}

func (cr *ComplianceReporter) determineViolationSeverity(regulationID, requirementID string) string {
	// Determine severity based on regulation and requirement
	// Critical requirements
	if strings.Contains(requirementID, "encryption") || strings.Contains(requirementID, "auth") {
		return "critical"
	}
	
	// High severity for privacy rights
	if strings.Contains(requirementID, "privacy") || strings.Contains(requirementID, "erasure") {
		return "high"
	}
	
	// Medium for audit trails
	if strings.Contains(requirementID, "audit") || strings.Contains(requirementID, "log") {
		return "medium"
	}
	
	return "low"
}

func (cr *ComplianceReporter) isRelevantEvidence(event *AuditEvent, regulationID, requirementID string) bool {
	// Check if event is relevant to the requirement
	// This would have more sophisticated logic in production
	return event.EventType == "compliance" || event.EventType == "security"
}

func (cr *ComplianceReporter) calculateEvidenceHash(event *AuditEvent) string {
	// Calculate hash for evidence integrity
	data := fmt.Sprintf("%s|%s|%s|%s", event.ID, event.Timestamp, event.EventType, event.Action)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

func (cr *ComplianceReporter) generateImprovements(report *RegulationReport) []string {
	improvements := make([]string, 0)
	
	// Analyze requirements and suggest improvements
	nonCompliantCount := 0
	for _, req := range report.Requirements {
		if req.Status != StatusCompliant {
			nonCompliantCount++
		}
	}
	
	if nonCompliantCount > 0 {
		improvements = append(improvements, fmt.Sprintf("Address %d non-compliant requirements", nonCompliantCount))
	}
	
	if report.ComplianceScore < 100 {
		improvements = append(improvements, "Implement automated compliance checks")
		improvements = append(improvements, "Enhance documentation and evidence collection")
	}
	
	if len(report.Violations) > 0 {
		improvements = append(improvements, "Establish violation remediation process")
	}
	
	return improvements
}

func generateViolationID() string {
	return fmt.Sprintf("VIO_%d_%s", time.Now().Unix(), generateRandomString(6))
}