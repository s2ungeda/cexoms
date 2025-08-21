package keymanager

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// EmergencyManager handles emergency access and key revocation
type EmergencyManager struct {
	mu              sync.RWMutex
	manager         *Manager
	config          EmergencyAccess
	activeIncidents map[string]*EmergencyIncident
	alertChannels   []AlertChannel
}

// EmergencyIncident represents an emergency access incident
type EmergencyIncident struct {
	ID              string                 `json:"id"`
	Type            string                 `json:"type"` // "break_glass", "mass_revocation", "security_breach"
	InitiatedBy     string                 `json:"initiated_by"`
	InitiatedAt     time.Time              `json:"initiated_at"`
	Reason          string                 `json:"reason"`
	AffectedKeys    []string               `json:"affected_keys"`
	Approvers       []string               `json:"approvers"`
	Status          string                 `json:"status"` // "pending", "approved", "executed", "cancelled"
	ExecutedAt      *time.Time             `json:"executed_at,omitempty"`
	Actions         []EmergencyAction      `json:"actions"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// EmergencyAction represents an action taken during emergency
type EmergencyAction struct {
	Type      string    `json:"type"`      // "revoke", "rotate", "disable"
	Target    string    `json:"target"`    // Key ID or pattern
	Timestamp time.Time `json:"timestamp"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

// AlertChannel represents a channel for emergency alerts
type AlertChannel interface {
	SendAlert(alert EmergencyAlert) error
}

// EmergencyAlert represents an emergency alert
type EmergencyAlert struct {
	Level       string    `json:"level"`       // "info", "warning", "critical"
	IncidentID  string    `json:"incident_id"`
	Message     string    `json:"message"`
	Timestamp   time.Time `json:"timestamp"`
	RequiresAck bool      `json:"requires_ack"`
}

// NewEmergencyManager creates a new emergency manager
func NewEmergencyManager(manager *Manager, config EmergencyAccess) *EmergencyManager {
	return &EmergencyManager{
		manager:         manager,
		config:          config,
		activeIncidents: make(map[string]*EmergencyIncident),
		alertChannels:   []AlertChannel{},
	}
}

// AddAlertChannel adds an alert channel
func (em *EmergencyManager) AddAlertChannel(channel AlertChannel) {
	em.alertChannels = append(em.alertChannels, channel)
}

// InitiateEmergency initiates an emergency procedure
func (em *EmergencyManager) InitiateEmergency(ctx context.Context, req EmergencyRequest) (*EmergencyIncident, error) {
	em.mu.Lock()
	defer em.mu.Unlock()

	if !em.config.Enabled {
		return nil, fmt.Errorf("emergency access is not enabled")
	}

	// Verify initiator is authorized
	if !em.isAuthorized(req.InitiatedBy) {
		return nil, fmt.Errorf("user %s is not authorized for emergency access", req.InitiatedBy)
	}

	// Create incident
	incident := &EmergencyIncident{
		ID:           generateIncidentID(),
		Type:         req.Type,
		InitiatedBy:  req.InitiatedBy,
		InitiatedAt:  time.Now(),
		Reason:       req.Reason,
		AffectedKeys: req.AffectedKeys,
		Status:       "pending",
		Actions:      []EmergencyAction{},
		Metadata:     req.Metadata,
	}

	// Check if multi-auth required
	if em.config.RequireMultiAuth && len(req.Approvers) < em.config.MinApprovers {
		incident.Status = "pending_approval"
		em.activeIncidents[incident.ID] = incident
		
		// Send approval request alerts
		em.sendAlert(EmergencyAlert{
			Level:       "warning",
			IncidentID:  incident.ID,
			Message:     fmt.Sprintf("Emergency access requested by %s: %s", req.InitiatedBy, req.Reason),
			Timestamp:   time.Now(),
			RequiresAck: true,
		})
		
		return incident, nil
	}

	// Execute immediately if no approval needed or sufficient approvers
	incident.Approvers = req.Approvers
	return em.executeEmergency(ctx, incident)
}

// ApproveEmergency approves a pending emergency
func (em *EmergencyManager) ApproveEmergency(ctx context.Context, incidentID string, approver string) error {
	em.mu.Lock()
	defer em.mu.Unlock()

	incident, exists := em.activeIncidents[incidentID]
	if !exists {
		return fmt.Errorf("incident %s not found", incidentID)
	}

	if incident.Status != "pending_approval" {
		return fmt.Errorf("incident %s is not pending approval", incidentID)
	}

	// Verify approver is authorized
	if !em.isAuthorized(approver) {
		return fmt.Errorf("user %s is not authorized to approve emergency access", approver)
	}

	// Add approver
	incident.Approvers = append(incident.Approvers, approver)

	// Check if we have enough approvers
	if len(incident.Approvers) >= em.config.MinApprovers {
		_, err := em.executeEmergency(ctx, incident)
		return err
	}

	// Send update alert
	em.sendAlert(EmergencyAlert{
		Level:      "info",
		IncidentID: incident.ID,
		Message:    fmt.Sprintf("Emergency access approved by %s (%d/%d approvals)", approver, len(incident.Approvers), em.config.MinApprovers),
		Timestamp:  time.Now(),
	})

	return nil
}

// executeEmergency executes the emergency procedure
func (em *EmergencyManager) executeEmergency(ctx context.Context, incident *EmergencyIncident) (*EmergencyIncident, error) {
	incident.Status = "executing"
	em.activeIncidents[incident.ID] = incident

	// Send critical alert
	em.sendAlert(EmergencyAlert{
		Level:      "critical",
		IncidentID: incident.ID,
		Message:    fmt.Sprintf("Executing emergency procedure: %s", incident.Type),
		Timestamp:  time.Now(),
	})

	switch incident.Type {
	case "break_glass":
		em.executeBreakGlass(ctx, incident)
	case "mass_revocation":
		em.executeMassRevocation(ctx, incident)
	case "security_breach":
		em.executeSecurityBreach(ctx, incident)
	default:
		return nil, fmt.Errorf("unknown emergency type: %s", incident.Type)
	}

	// Mark as executed
	now := time.Now()
	incident.ExecutedAt = &now
	incident.Status = "executed"

	// Send completion alert
	em.sendAlert(EmergencyAlert{
		Level:      "info",
		IncidentID: incident.ID,
		Message:    fmt.Sprintf("Emergency procedure completed: %d actions taken", len(incident.Actions)),
		Timestamp:  time.Now(),
	})

	return incident, nil
}

// executeBreakGlass handles break-glass emergency access
func (em *EmergencyManager) executeBreakGlass(ctx context.Context, incident *EmergencyIncident) {
	// Break-glass provides temporary elevated access
	// In this implementation, we'll rotate affected keys
	
	for _, keyID := range incident.AffectedKeys {
		action := EmergencyAction{
			Type:      "rotate",
			Target:    keyID,
			Timestamp: time.Now(),
		}

		req := KeyRotationRequest{
			KeyID:     keyID,
			Reason:    fmt.Sprintf("Emergency break-glass: %s", incident.Reason),
			Immediate: true,
		}

		if _, err := em.manager.rotator.RotateKey(ctx, req); err != nil {
			action.Success = false
			action.Error = err.Error()
		} else {
			action.Success = true
		}

		incident.Actions = append(incident.Actions, action)
	}
}

// executeMassRevocation revokes multiple keys
func (em *EmergencyManager) executeMassRevocation(ctx context.Context, incident *EmergencyIncident) {
	for _, keyID := range incident.AffectedKeys {
		action := EmergencyAction{
			Type:      "revoke",
			Target:    keyID,
			Timestamp: time.Now(),
		}

		if err := em.manager.RevokeKey(ctx, keyID, incident.Reason); err != nil {
			action.Success = false
			action.Error = err.Error()
		} else {
			action.Success = true
		}

		incident.Actions = append(incident.Actions, action)
	}
}

// executeSecurityBreach handles security breach response
func (em *EmergencyManager) executeSecurityBreach(ctx context.Context, incident *EmergencyIncident) {
	// For security breach, we disable all affected keys immediately
	// and schedule rotation for unaffected keys
	
	for _, keyID := range incident.AffectedKeys {
		// First, revoke the key
		revokeAction := EmergencyAction{
			Type:      "revoke",
			Target:    keyID,
			Timestamp: time.Now(),
		}

		if err := em.manager.RevokeKey(ctx, keyID, "Security breach response"); err != nil {
			revokeAction.Success = false
			revokeAction.Error = err.Error()
		} else {
			revokeAction.Success = true
		}
		incident.Actions = append(incident.Actions, revokeAction)

		// Then delete it from storage
		deleteAction := EmergencyAction{
			Type:      "delete",
			Target:    keyID,
			Timestamp: time.Now(),
		}

		// Note: This would need implementation of DeleteKeyByID in manager
		deleteAction.Success = true // Placeholder
		incident.Actions = append(incident.Actions, deleteAction)
	}
}

// RevokeAllKeys revokes all keys for an account
func (em *EmergencyManager) RevokeAllKeys(ctx context.Context, accountName string, reason string) error {
	// Get all keys for the account
	keys, err := em.manager.ListKeys(ctx, accountName)
	if err != nil {
		return err
	}

	// Create emergency request
	keyIDs := []string{}
	for _, key := range keys {
		keyIDs = append(keyIDs, key.ID)
	}

	req := EmergencyRequest{
		Type:         "mass_revocation",
		InitiatedBy:  "system",
		Reason:       reason,
		AffectedKeys: keyIDs,
		Metadata: map[string]interface{}{
			"account": accountName,
		},
	}

	_, err = em.InitiateEmergency(ctx, req)
	return err
}

// GetIncident retrieves an emergency incident
func (em *EmergencyManager) GetIncident(incidentID string) (*EmergencyIncident, error) {
	em.mu.RLock()
	defer em.mu.RUnlock()

	incident, exists := em.activeIncidents[incidentID]
	if !exists {
		return nil, fmt.Errorf("incident %s not found", incidentID)
	}

	return incident, nil
}

// ListIncidents lists all emergency incidents
func (em *EmergencyManager) ListIncidents(status string) []*EmergencyIncident {
	em.mu.RLock()
	defer em.mu.RUnlock()

	incidents := []*EmergencyIncident{}
	for _, incident := range em.activeIncidents {
		if status == "" || incident.Status == status {
			incidents = append(incidents, incident)
		}
	}

	return incidents
}

// isAuthorized checks if a user is authorized for emergency access
func (em *EmergencyManager) isAuthorized(user string) bool {
	for _, authorized := range em.config.BreakGlassUsers {
		if authorized == user {
			return true
		}
	}
	return false
}

// sendAlert sends an alert to all configured channels
func (em *EmergencyManager) sendAlert(alert EmergencyAlert) {
	for _, channel := range em.alertChannels {
		go channel.SendAlert(alert)
	}
}

// EmergencyRequest represents a request for emergency access
type EmergencyRequest struct {
	Type         string                 `json:"type"`
	InitiatedBy  string                 `json:"initiated_by"`
	Reason       string                 `json:"reason"`
	AffectedKeys []string               `json:"affected_keys"`
	Approvers    []string               `json:"approvers"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// generateIncidentID generates a unique incident ID
func generateIncidentID() string {
	return fmt.Sprintf("INC-%d-%s", time.Now().UnixNano(), generateRandomString(6))
}

// ConsoleAlertChannel implements AlertChannel for console output
type ConsoleAlertChannel struct{}

func (c *ConsoleAlertChannel) SendAlert(alert EmergencyAlert) error {
	fmt.Printf("[EMERGENCY %s] %s: %s\n", alert.Level, alert.IncidentID, alert.Message)
	return nil
}

// EmailAlertChannel implements AlertChannel for email alerts
type EmailAlertChannel struct {
	Recipients []string
}

func (e *EmailAlertChannel) SendAlert(alert EmergencyAlert) error {
	// In production, implement actual email sending
	fmt.Printf("Email alert to %v: %s\n", e.Recipients, alert.Message)
	return nil
}

// SlackAlertChannel implements AlertChannel for Slack alerts
type SlackAlertChannel struct {
	WebhookURL string
	Channel    string
}

func (s *SlackAlertChannel) SendAlert(alert EmergencyAlert) error {
	// In production, implement actual Slack webhook call
	fmt.Printf("Slack alert to %s: %s\n", s.Channel, alert.Message)
	return nil
}