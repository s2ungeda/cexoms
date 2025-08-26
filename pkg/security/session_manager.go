package security

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

// Session represents an active user session
type Session struct {
	ID           string
	UserID       string
	AccountID    string
	IPAddress    string
	UserAgent    string
	AuthContext  *AuthContext
	CreatedAt    time.Time
	LastActiveAt time.Time
	ExpiresAt    time.Time
	Data         map[string]interface{} // Custom session data
}

// SessionManager manages user sessions
type SessionManager struct {
	mu          sync.RWMutex
	sessions    map[string]*Session
	userSessions map[string][]*Session // userID -> sessions
	timeout     time.Duration
	maxSessions int // Max sessions per user
}

// NewSessionManager creates a new session manager
func NewSessionManager(timeout time.Duration) *SessionManager {
	sm := &SessionManager{
		sessions:     make(map[string]*Session),
		userSessions: make(map[string][]*Session),
		timeout:      timeout,
		maxSessions:  5,
	}
	
	// Start cleanup goroutine
	go sm.cleanup()
	
	return sm
}

// Create creates a new session
func (sm *SessionManager) Create(sessionID string, authContext *AuthContext) (*Session, error) {
	if sessionID == "" {
		sessionID = sm.generateSessionID()
	}
	
	session := &Session{
		ID:           sessionID,
		UserID:       authContext.UserID,
		AccountID:    authContext.AccountID,
		IPAddress:    authContext.IPAddress,
		UserAgent:    authContext.UserAgent,
		AuthContext:  authContext,
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
		ExpiresAt:    time.Now().Add(sm.timeout),
		Data:         make(map[string]interface{}),
	}
	
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	// Check max sessions per user
	userSessions := sm.userSessions[authContext.UserID]
	if len(userSessions) >= sm.maxSessions {
		// Remove oldest session
		oldestSession := userSessions[0]
		delete(sm.sessions, oldestSession.ID)
		sm.userSessions[authContext.UserID] = userSessions[1:]
	}
	
	// Store session
	sm.sessions[sessionID] = session
	
	// Add to user sessions
	if sm.userSessions[authContext.UserID] == nil {
		sm.userSessions[authContext.UserID] = make([]*Session, 0)
	}
	sm.userSessions[authContext.UserID] = append(sm.userSessions[authContext.UserID], session)
	
	return session, nil
}

// Get retrieves a session
func (sm *SessionManager) Get(sessionID string) (*Session, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	session, exists := sm.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found")
	}
	
	// Check if expired
	if time.Now().After(session.ExpiresAt) {
		return nil, fmt.Errorf("session expired")
	}
	
	// Update last active time
	go sm.touch(sessionID)
	
	return session, nil
}

// IsValid checks if session is valid
func (sm *SessionManager) IsValid(sessionID string) bool {
	session, err := sm.Get(sessionID)
	return err == nil && session != nil
}

// Destroy destroys a session
func (sm *SessionManager) Destroy(sessionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found")
	}
	
	// Remove from sessions map
	delete(sm.sessions, sessionID)
	
	// Remove from user sessions
	if userSessions, exists := sm.userSessions[session.UserID]; exists {
		newUserSessions := make([]*Session, 0, len(userSessions)-1)
		for _, s := range userSessions {
			if s.ID != sessionID {
				newUserSessions = append(newUserSessions, s)
			}
		}
		sm.userSessions[session.UserID] = newUserSessions
	}
	
	return nil
}

// DestroyUserSessions destroys all sessions for a user
func (sm *SessionManager) DestroyUserSessions(userID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	userSessions, exists := sm.userSessions[userID]
	if !exists {
		return nil
	}
	
	// Remove all user sessions
	for _, session := range userSessions {
		delete(sm.sessions, session.ID)
	}
	
	// Clear user sessions
	delete(sm.userSessions, userID)
	
	return nil
}

// GetUserSessions returns all active sessions for a user
func (sm *SessionManager) GetUserSessions(userID string) ([]*Session, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	sessions, exists := sm.userSessions[userID]
	if !exists {
		return []*Session{}, nil
	}
	
	// Filter out expired sessions
	activeSessions := make([]*Session, 0, len(sessions))
	now := time.Now()
	
	for _, session := range sessions {
		if now.Before(session.ExpiresAt) {
			activeSessions = append(activeSessions, session)
		}
	}
	
	return activeSessions, nil
}

// SetData sets custom session data
func (sm *SessionManager) SetData(sessionID string, key string, value interface{}) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found")
	}
	
	session.Data[key] = value
	return nil
}

// GetData retrieves custom session data
func (sm *SessionManager) GetData(sessionID string, key string) (interface{}, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	session, exists := sm.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found")
	}
	
	value, exists := session.Data[key]
	if !exists {
		return nil, fmt.Errorf("key not found in session data")
	}
	
	return value, nil
}

// Refresh extends session expiry
func (sm *SessionManager) Refresh(sessionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found")
	}
	
	session.LastActiveAt = time.Now()
	session.ExpiresAt = time.Now().Add(sm.timeout)
	
	return nil
}

// touch updates last active time
func (sm *SessionManager) touch(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	if session, exists := sm.sessions[sessionID]; exists {
		session.LastActiveAt = time.Now()
	}
}

// cleanup removes expired sessions
func (sm *SessionManager) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for {
		<-ticker.C
		sm.cleanupExpired()
	}
}

// cleanupExpired removes all expired sessions
func (sm *SessionManager) cleanupExpired() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	now := time.Now()
	expiredSessions := make([]string, 0)
	
	// Find expired sessions
	for sessionID, session := range sm.sessions {
		if now.After(session.ExpiresAt) {
			expiredSessions = append(expiredSessions, sessionID)
		}
	}
	
	// Remove expired sessions
	for _, sessionID := range expiredSessions {
		session := sm.sessions[sessionID]
		delete(sm.sessions, sessionID)
		
		// Remove from user sessions
		if userSessions, exists := sm.userSessions[session.UserID]; exists {
			newUserSessions := make([]*Session, 0, len(userSessions))
			for _, s := range userSessions {
				if s.ID != sessionID {
					newUserSessions = append(newUserSessions, s)
				}
			}
			sm.userSessions[session.UserID] = newUserSessions
			
			// Clean up empty user entry
			if len(newUserSessions) == 0 {
				delete(sm.userSessions, session.UserID)
			}
		}
	}
}

// generateSessionID generates a secure session ID
func (sm *SessionManager) generateSessionID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// GetActiveSessions returns count of active sessions
func (sm *SessionManager) GetActiveSessions() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	count := 0
	now := time.Now()
	
	for _, session := range sm.sessions {
		if now.Before(session.ExpiresAt) {
			count++
		}
	}
	
	return count
}

// GetSessionStats returns session statistics
func (sm *SessionManager) GetSessionStats() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	stats := make(map[string]interface{})
	stats["total_sessions"] = len(sm.sessions)
	stats["unique_users"] = len(sm.userSessions)
	
	// Calculate active sessions in last hour
	activeInHour := 0
	hourAgo := time.Now().Add(-1 * time.Hour)
	
	for _, session := range sm.sessions {
		if session.LastActiveAt.After(hourAgo) {
			activeInHour++
		}
	}
	
	stats["active_in_last_hour"] = activeInHour
	
	// Calculate average session duration
	var totalDuration time.Duration
	count := 0
	
	for _, session := range sm.sessions {
		duration := session.LastActiveAt.Sub(session.CreatedAt)
		totalDuration += duration
		count++
	}
	
	if count > 0 {
		stats["avg_session_duration_minutes"] = totalDuration.Minutes() / float64(count)
	}
	
	return stats
}