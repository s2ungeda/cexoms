package security

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// AuditEvent represents an audit event
type AuditEvent struct {
	ID          string                 `json:"id"`
	Timestamp   time.Time              `json:"timestamp"`
	EventType   string                 `json:"event_type"`
	UserID      string                 `json:"user_id,omitempty"`
	AccountID   string                 `json:"account_id,omitempty"`
	SessionID   string                 `json:"session_id,omitempty"`
	IPAddress   string                 `json:"ip_address,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
	Action      string                 `json:"action"`
	Resource    string                 `json:"resource,omitempty"`
	Result      string                 `json:"result"`
	Reason      string                 `json:"reason,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	RiskScore   float64                `json:"risk_score,omitempty"`
}

// EventType constants
const (
	EventTypeAuth         = "authentication"
	EventTypeAccess       = "access"
	EventTypeModification = "modification"
	EventTypeSecurity     = "security"
	EventTypeCompliance   = "compliance"
)

// Result constants
const (
	ResultSuccess = "success"
	ResultFailure = "failure"
	ResultBlocked = "blocked"
)

// AuditLogger handles security audit logging
type AuditLogger struct {
	logger      *zap.Logger
	buffer      chan *AuditEvent
	storage     AuditStorage
	riskScorer  *RiskScorer
	mu          sync.RWMutex
	stopped     bool
	wg          sync.WaitGroup
}

// AuditStorage interface for storing audit events
type AuditStorage interface {
	Store(event *AuditEvent) error
	Query(filter AuditFilter) ([]*AuditEvent, error)
}

// AuditFilter for querying audit events
type AuditFilter struct {
	UserID    string
	AccountID string
	EventType string
	StartTime time.Time
	EndTime   time.Time
	Limit     int
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger() *AuditLogger {
	// Configure logger for audit events
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"stdout", "/var/log/mexoms/audit.log"}
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	
	logger, _ := config.Build()
	
	al := &AuditLogger{
		logger:     logger,
		buffer:     make(chan *AuditEvent, 1000),
		riskScorer: NewRiskScorer(),
	}
	
	// Start event processor
	al.wg.Add(1)
	go al.processEvents()
	
	return al
}

// LogSuccessfulAuth logs successful authentication
func (al *AuditLogger) LogSuccessfulAuth(ctx context.Context, authContext *AuthContext) {
	event := &AuditEvent{
		ID:        generateEventID(),
		Timestamp: time.Now(),
		EventType: EventTypeAuth,
		UserID:    authContext.UserID,
		AccountID: authContext.AccountID,
		SessionID: authContext.SessionID,
		IPAddress: authContext.IPAddress,
		UserAgent: authContext.UserAgent,
		Action:    "login",
		Result:    ResultSuccess,
		Metadata: map[string]interface{}{
			"auth_type":    string(authContext.AuthType),
			"mfa_verified": authContext.MFAVerified,
		},
	}
	
	al.logEvent(event)
}

// LogFailedAuth logs failed authentication
func (al *AuditLogger) LogFailedAuth(ctx context.Context, authType AuthType, err error) {
	ipAddress := al.extractIPAddress(ctx)
	userAgent := al.extractUserAgent(ctx)
	
	event := &AuditEvent{
		ID:        generateEventID(),
		Timestamp: time.Now(),
		EventType: EventTypeAuth,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Action:    "login",
		Result:    ResultFailure,
		Reason:    err.Error(),
		Metadata: map[string]interface{}{
			"auth_type": string(authType),
		},
	}
	
	// Calculate risk score for failed auth
	event.RiskScore = al.riskScorer.CalculateAuthFailureRisk(ipAddress, userAgent)
	
	al.logEvent(event)
}

// LogMFAFailure logs MFA verification failure
func (al *AuditLogger) LogMFAFailure(ctx context.Context, userID string) {
	event := &AuditEvent{
		ID:        generateEventID(),
		Timestamp: time.Now(),
		EventType: EventTypeAuth,
		UserID:    userID,
		IPAddress: al.extractIPAddress(ctx),
		UserAgent: al.extractUserAgent(ctx),
		Action:    "mfa_verification",
		Result:    ResultFailure,
		RiskScore: 0.7, // High risk for MFA failure
	}
	
	al.logEvent(event)
}

// LogAccess logs resource access
func (al *AuditLogger) LogAccess(ctx context.Context, userID, resource, action string, allowed bool) {
	result := ResultSuccess
	if !allowed {
		result = ResultBlocked
	}
	
	event := &AuditEvent{
		ID:        generateEventID(),
		Timestamp: time.Now(),
		EventType: EventTypeAccess,
		UserID:    userID,
		IPAddress: al.extractIPAddress(ctx),
		Action:    action,
		Resource:  resource,
		Result:    result,
		Metadata: map[string]interface{}{
			"method": al.extractMethod(ctx),
		},
	}
	
	al.logEvent(event)
}

// LogModification logs data modification
func (al *AuditLogger) LogModification(ctx context.Context, userID, resource, action string, before, after interface{}) {
	event := &AuditEvent{
		ID:        generateEventID(),
		Timestamp: time.Now(),
		EventType: EventTypeModification,
		UserID:    userID,
		IPAddress: al.extractIPAddress(ctx),
		Action:    action,
		Resource:  resource,
		Result:    ResultSuccess,
		Metadata: map[string]interface{}{
			"before": before,
			"after":  after,
		},
	}
	
	al.logEvent(event)
}

// LogSecurityEvent logs security-related events
func (al *AuditLogger) LogSecurityEvent(ctx context.Context, eventName string, severity string, details map[string]interface{}) {
	event := &AuditEvent{
		ID:        generateEventID(),
		Timestamp: time.Now(),
		EventType: EventTypeSecurity,
		IPAddress: al.extractIPAddress(ctx),
		Action:    eventName,
		Result:    ResultSuccess,
		Metadata:  details,
		RiskScore: al.calculateSecurityEventRisk(severity),
	}
	
	// Extract user context if available
	if authCtx, ok := ctx.Value("auth").(*AuthContext); ok {
		event.UserID = authCtx.UserID
		event.AccountID = authCtx.AccountID
		event.SessionID = authCtx.SessionID
	}
	
	al.logEvent(event)
}

// LogComplianceEvent logs compliance-related events
func (al *AuditLogger) LogComplianceEvent(ctx context.Context, regulation, requirement, status string, details map[string]interface{}) {
	event := &AuditEvent{
		ID:        generateEventID(),
		Timestamp: time.Now(),
		EventType: EventTypeCompliance,
		Action:    requirement,
		Result:    status,
		Metadata: map[string]interface{}{
			"regulation": regulation,
			"details":    details,
		},
	}
	
	al.logEvent(event)
}

// LogAPICall logs API calls for audit
func (al *AuditLogger) LogAPICall(ctx context.Context, method, path string, statusCode int, duration time.Duration) {
	result := ResultSuccess
	if statusCode >= 400 {
		result = ResultFailure
	}
	
	event := &AuditEvent{
		ID:        generateEventID(),
		Timestamp: time.Now(),
		EventType: EventTypeAccess,
		IPAddress: al.extractIPAddress(ctx),
		UserAgent: al.extractUserAgent(ctx),
		Action:    fmt.Sprintf("%s %s", method, path),
		Result:    result,
		Metadata: map[string]interface{}{
			"status_code": statusCode,
			"duration_ms": duration.Milliseconds(),
		},
	}
	
	// Add user context if authenticated
	if authCtx, ok := ctx.Value("auth").(*AuthContext); ok {
		event.UserID = authCtx.UserID
		event.AccountID = authCtx.AccountID
		event.SessionID = authCtx.SessionID
	}
	
	al.logEvent(event)
}

// Query queries audit events
func (al *AuditLogger) Query(filter AuditFilter) ([]*AuditEvent, error) {
	if al.storage == nil {
		return nil, fmt.Errorf("audit storage not configured")
	}
	
	return al.storage.Query(filter)
}

// logEvent logs an audit event
func (al *AuditLogger) logEvent(event *AuditEvent) {
	al.mu.RLock()
	if al.stopped {
		al.mu.RUnlock()
		return
	}
	al.mu.RUnlock()
	
	// Send to buffer
	select {
	case al.buffer <- event:
	default:
		// Buffer full, log directly
		al.writeEvent(event)
	}
}

// processEvents processes buffered events
func (al *AuditLogger) processEvents() {
	defer al.wg.Done()
	
	for event := range al.buffer {
		al.writeEvent(event)
	}
}

// writeEvent writes event to logger and storage
func (al *AuditLogger) writeEvent(event *AuditEvent) {
	// Log to file
	fields := []zap.Field{
		zap.String("id", event.ID),
		zap.String("event_type", event.EventType),
		zap.String("user_id", event.UserID),
		zap.String("account_id", event.AccountID),
		zap.String("session_id", event.SessionID),
		zap.String("ip_address", event.IPAddress),
		zap.String("action", event.Action),
		zap.String("resource", event.Resource),
		zap.String("result", event.Result),
		zap.String("reason", event.Reason),
		zap.Float64("risk_score", event.RiskScore),
		zap.Any("metadata", event.Metadata),
	}
	
	switch event.Result {
	case ResultFailure, ResultBlocked:
		al.logger.Warn("Audit event", fields...)
	default:
		al.logger.Info("Audit event", fields...)
	}
	
	// Store in persistent storage
	if al.storage != nil {
		if err := al.storage.Store(event); err != nil {
			al.logger.Error("Failed to store audit event", zap.Error(err))
		}
	}
}

// extractIPAddress extracts IP address from context
func (al *AuditLogger) extractIPAddress(ctx context.Context) string {
	// Try gRPC peer
	if p, ok := peer.FromContext(ctx); ok {
		return p.Addr.String()
	}
	
	// Try HTTP request
	if req, ok := ctx.Value("http_request").(*http.Request); ok {
		// Check X-Forwarded-For
		if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
			return xff
		}
		// Check X-Real-IP
		if xri := req.Header.Get("X-Real-IP"); xri != "" {
			return xri
		}
		return req.RemoteAddr
	}
	
	return ""
}

// extractUserAgent extracts user agent from context
func (al *AuditLogger) extractUserAgent(ctx context.Context) string {
	// Try HTTP request
	if req, ok := ctx.Value("http_request").(*http.Request); ok {
		return req.Header.Get("User-Agent")
	}
	
	// Try gRPC metadata
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if ua := md.Get("user-agent"); len(ua) > 0 {
			return ua[0]
		}
	}
	
	return ""
}

// extractMethod extracts method from context
func (al *AuditLogger) extractMethod(ctx context.Context) string {
	// Try HTTP request
	if req, ok := ctx.Value("http_request").(*http.Request); ok {
		return req.Method
	}
	
	// Try gRPC method
	if method, ok := ctx.Value("grpc_method").(string); ok {
		return method
	}
	
	return ""
}

// calculateSecurityEventRisk calculates risk score for security events
func (al *AuditLogger) calculateSecurityEventRisk(severity string) float64 {
	switch severity {
	case "critical":
		return 1.0
	case "high":
		return 0.8
	case "medium":
		return 0.5
	case "low":
		return 0.2
	default:
		return 0.1
	}
}

// Stop stops the audit logger
func (al *AuditLogger) Stop() {
	al.mu.Lock()
	al.stopped = true
	al.mu.Unlock()
	
	close(al.buffer)
	al.wg.Wait()
	al.logger.Sync()
}

// generateEventID generates unique event ID
func generateEventID() string {
	return fmt.Sprintf("evt_%d_%s", time.Now().UnixNano(), generateRandomString(8))
}

// generateRandomString generates random string
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// RiskScorer calculates risk scores
type RiskScorer struct {
	failedAttempts map[string][]time.Time
	mu             sync.RWMutex
}

// NewRiskScorer creates a new risk scorer
func NewRiskScorer() *RiskScorer {
	return &RiskScorer{
		failedAttempts: make(map[string][]time.Time),
	}
}

// CalculateAuthFailureRisk calculates risk score for auth failure
func (rs *RiskScorer) CalculateAuthFailureRisk(ipAddress, userAgent string) float64 {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	
	key := fmt.Sprintf("%s:%s", ipAddress, userAgent)
	now := time.Now()
	
	// Clean old attempts
	if attempts, exists := rs.failedAttempts[key]; exists {
		validAttempts := make([]time.Time, 0)
		for _, attempt := range attempts {
			if now.Sub(attempt) < 1*time.Hour {
				validAttempts = append(validAttempts, attempt)
			}
		}
		rs.failedAttempts[key] = validAttempts
	}
	
	// Add new attempt
	rs.failedAttempts[key] = append(rs.failedAttempts[key], now)
	
	// Calculate risk based on frequency
	attempts := len(rs.failedAttempts[key])
	switch {
	case attempts >= 10:
		return 1.0
	case attempts >= 5:
		return 0.8
	case attempts >= 3:
		return 0.5
	default:
		return 0.2
	}
}