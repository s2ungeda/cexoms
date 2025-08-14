package cache

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type Session struct {
	ID        string
	UserID    string
	Data      map[string]interface{}
	CreatedAt time.Time
	ExpiresAt time.Time
}

type SessionManager struct {
	sessions sync.Map
	ttl      time.Duration
}

func NewSessionManager(ttl time.Duration) *SessionManager {
	sm := &SessionManager{
		ttl: ttl,
	}
	go sm.cleanupExpired()
	return sm
}

func (sm *SessionManager) CreateSession(userID string) (*Session, error) {
	sessionID, err := generateSessionID()
	if err != nil {
		return nil, err
	}
	
	session := &Session{
		ID:        sessionID,
		UserID:    userID,
		Data:      make(map[string]interface{}),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(sm.ttl),
	}
	
	sm.sessions.Store(sessionID, session)
	return session, nil
}

func (sm *SessionManager) GetSession(sessionID string) (*Session, bool) {
	value, exists := sm.sessions.Load(sessionID)
	if !exists {
		return nil, false
	}
	
	session := value.(*Session)
	if time.Now().After(session.ExpiresAt) {
		sm.sessions.Delete(sessionID)
		return nil, false
	}
	
	return session, true
}

func (sm *SessionManager) UpdateSession(sessionID string, data map[string]interface{}) bool {
	value, exists := sm.sessions.Load(sessionID)
	if !exists {
		return false
	}
	
	session := value.(*Session)
	if time.Now().After(session.ExpiresAt) {
		sm.sessions.Delete(sessionID)
		return false
	}
	
	for k, v := range data {
		session.Data[k] = v
	}
	
	session.ExpiresAt = time.Now().Add(sm.ttl)
	return true
}

func (sm *SessionManager) DeleteSession(sessionID string) {
	sm.sessions.Delete(sessionID)
}

func (sm *SessionManager) cleanupExpired() {
	ticker := time.NewTicker(time.Minute * 5)
	defer ticker.Stop()
	
	for range ticker.C {
		now := time.Now()
		sm.sessions.Range(func(key, value interface{}) bool {
			session := value.(*Session)
			if now.After(session.ExpiresAt) {
				sm.sessions.Delete(key)
			}
			return true
		})
	}
}

func generateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}