package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// AlertLevel represents the severity of an alert
type AlertLevel string

const (
	AlertLevelInfo     AlertLevel = "info"
	AlertLevelWarning  AlertLevel = "warning"
	AlertLevelCritical AlertLevel = "critical"
)

// Alert represents a system alert
type Alert struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Level       AlertLevel             `json:"level"`
	Component   string                 `json:"component"`
	Message     string                 `json:"message"`
	Details     map[string]interface{} `json:"details"`
	Timestamp   time.Time              `json:"timestamp"`
	ResolvedAt  *time.Time             `json:"resolved_at,omitempty"`
}

// NotificationChannel represents a channel for sending notifications
type NotificationChannel interface {
	Send(ctx context.Context, alert Alert) error
	Name() string
}

// SlackChannel sends notifications to Slack
type SlackChannel struct {
	webhookURL string
	channel    string
}

// NewSlackChannel creates a new Slack notification channel
func NewSlackChannel(webhookURL, channel string) *SlackChannel {
	return &SlackChannel{
		webhookURL: webhookURL,
		channel:    channel,
	}
}

// Send sends an alert to Slack
func (s *SlackChannel) Send(ctx context.Context, alert Alert) error {
	color := "#36a64f" // green
	switch alert.Level {
	case AlertLevelWarning:
		color = "#ff9800" // orange
	case AlertLevelCritical:
		color = "#f44336" // red
	}

	payload := map[string]interface{}{
		"channel": s.channel,
		"attachments": []map[string]interface{}{
			{
				"color":     color,
				"title":     fmt.Sprintf("[%s] %s", alert.Level, alert.Name),
				"text":      alert.Message,
				"footer":    alert.Component,
				"timestamp": alert.Timestamp.Unix(),
				"fields":    s.formatFields(alert.Details),
			},
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.webhookURL, bytes.NewReader(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send slack notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// Name returns the channel name
func (s *SlackChannel) Name() string {
	return "slack"
}

// formatFields formats alert details for Slack
func (s *SlackChannel) formatFields(details map[string]interface{}) []map[string]interface{} {
	fields := make([]map[string]interface{}, 0, len(details))
	for key, value := range details {
		fields = append(fields, map[string]interface{}{
			"title": key,
			"value": fmt.Sprintf("%v", value),
			"short": true,
		})
	}
	return fields
}

// EmailChannel sends notifications via email
type EmailChannel struct {
	smtpHost string
	smtpPort int
	from     string
	to       []string
}

// NewEmailChannel creates a new email notification channel
func NewEmailChannel(smtpHost string, smtpPort int, from string, to []string) *EmailChannel {
	return &EmailChannel{
		smtpHost: smtpHost,
		smtpPort: smtpPort,
		from:     from,
		to:       to,
	}
}

// Send sends an alert via email
func (e *EmailChannel) Send(ctx context.Context, alert Alert) error {
	// Implementation would use smtp package
	// Simplified for brevity
	return nil
}

// Name returns the channel name
func (e *EmailChannel) Name() string {
	return "email"
}

// PagerDutyChannel sends notifications to PagerDuty
type PagerDutyChannel struct {
	integrationKey string
}

// NewPagerDutyChannel creates a new PagerDuty notification channel
func NewPagerDutyChannel(integrationKey string) *PagerDutyChannel {
	return &PagerDutyChannel{
		integrationKey: integrationKey,
	}
}

// Send sends an alert to PagerDuty
func (p *PagerDutyChannel) Send(ctx context.Context, alert Alert) error {
	severity := "info"
	switch alert.Level {
	case AlertLevelWarning:
		severity = "warning"
	case AlertLevelCritical:
		severity = "critical"
	}

	payload := map[string]interface{}{
		"routing_key":  p.integrationKey,
		"event_action": "trigger",
		"dedup_key":    alert.ID,
		"payload": map[string]interface{}{
			"summary":   alert.Message,
			"severity":  severity,
			"source":    alert.Component,
			"timestamp": alert.Timestamp.Format(time.RFC3339),
			"custom_details": alert.Details,
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal pagerduty payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://events.pagerduty.com/v2/enqueue", bytes.NewReader(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send pagerduty notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("pagerduty returned status %d", resp.StatusCode)
	}

	return nil
}

// Name returns the channel name
func (p *PagerDutyChannel) Name() string {
	return "pagerduty"
}

// Notifier manages alert notifications
type Notifier struct {
	channels []NotificationChannel
	mu       sync.RWMutex
	alerts   map[string]Alert
}

// NewNotifier creates a new notifier
func NewNotifier(channels ...NotificationChannel) *Notifier {
	return &Notifier{
		channels: channels,
		alerts:   make(map[string]Alert),
	}
}

// SendAlert sends an alert to all configured channels
func (n *Notifier) SendAlert(ctx context.Context, alert Alert) error {
	n.mu.Lock()
	n.alerts[alert.ID] = alert
	n.mu.Unlock()

	var errs []error
	for _, channel := range n.channels {
		if err := channel.Send(ctx, alert); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", channel.Name(), err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to send to some channels: %v", errs)
	}

	return nil
}

// ResolveAlert marks an alert as resolved
func (n *Notifier) ResolveAlert(ctx context.Context, alertID string) error {
	n.mu.Lock()
	alert, exists := n.alerts[alertID]
	if !exists {
		n.mu.Unlock()
		return fmt.Errorf("alert %s not found", alertID)
	}

	now := time.Now()
	alert.ResolvedAt = &now
	n.alerts[alertID] = alert
	n.mu.Unlock()

	// Send resolution notification
	alert.Message = fmt.Sprintf("RESOLVED: %s", alert.Message)
	return n.SendAlert(ctx, alert)
}

// GetActiveAlerts returns all active alerts
func (n *Notifier) GetActiveAlerts() []Alert {
	n.mu.RLock()
	defer n.mu.RUnlock()

	active := make([]Alert, 0)
	for _, alert := range n.alerts {
		if alert.ResolvedAt == nil {
			active = append(active, alert)
		}
	}
	return active
}

// AlertManager integrates with Prometheus alerts
type AlertManager struct {
	notifier *Notifier
	rules    []AlertRule
	mu       sync.RWMutex
}

// AlertRule defines a rule for generating alerts
type AlertRule struct {
	Name      string
	Condition func() (bool, map[string]interface{})
	Level     AlertLevel
	Component string
	Message   string
}

// NewAlertManager creates a new alert manager
func NewAlertManager(notifier *Notifier) *AlertManager {
	return &AlertManager{
		notifier: notifier,
		rules:    make([]AlertRule, 0),
	}
}

// AddRule adds an alert rule
func (am *AlertManager) AddRule(rule AlertRule) {
	am.mu.Lock()
	am.rules = append(am.rules, rule)
	am.mu.Unlock()
}

// Start starts the alert manager
func (am *AlertManager) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				am.checkRules(ctx)
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()
}

// checkRules checks all alert rules
func (am *AlertManager) checkRules(ctx context.Context) {
	am.mu.RLock()
	rules := make([]AlertRule, len(am.rules))
	copy(rules, am.rules)
	am.mu.RUnlock()

	for _, rule := range rules {
		triggered, details := rule.Condition()
		if triggered {
			alert := Alert{
				ID:        fmt.Sprintf("%s-%d", rule.Name, time.Now().Unix()),
				Name:      rule.Name,
				Level:     rule.Level,
				Component: rule.Component,
				Message:   rule.Message,
				Details:   details,
				Timestamp: time.Now(),
			}
			
			if err := am.notifier.SendAlert(ctx, alert); err != nil {
				// Log error but continue checking other rules
				fmt.Printf("Failed to send alert: %v\n", err)
			}
		}
	}
}