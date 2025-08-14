package cache

import (
	"testing"
	"time"
)

func TestMemoryCache(t *testing.T) {
	cache := NewMemoryCache()
	
	// Test Set and Get
	cache.Set("key1", "value1", time.Hour)
	value, exists := cache.Get("key1")
	if !exists {
		t.Error("Expected key1 to exist")
	}
	if value != "value1" {
		t.Errorf("Expected value1, got %v", value)
	}
	
	// Test TTL expiration
	cache.Set("key2", "value2", time.Millisecond*100)
	time.Sleep(time.Millisecond * 200)
	_, exists = cache.Get("key2")
	if exists {
		t.Error("Expected key2 to be expired")
	}
	
	// Test Delete
	cache.Set("key3", "value3", time.Hour)
	cache.Delete("key3")
	_, exists = cache.Get("key3")
	if exists {
		t.Error("Expected key3 to be deleted")
	}
	
	// Test Clear
	cache.Set("key4", "value4", time.Hour)
	cache.Set("key5", "value5", time.Hour)
	cache.Clear()
	all := cache.GetAll()
	if len(all) != 0 {
		t.Error("Expected cache to be empty after Clear")
	}
}

func TestRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(3, time.Second)
	
	// Test within limit
	for i := 0; i < 3; i++ {
		if !limiter.Allow("user1") {
			t.Errorf("Expected request %d to be allowed", i+1)
		}
	}
	
	// Test over limit
	if limiter.Allow("user1") {
		t.Error("Expected request to be rate limited")
	}
	
	// Test different key
	if !limiter.Allow("user2") {
		t.Error("Expected request for different user to be allowed")
	}
	
	// Test reset
	limiter.Reset("user1")
	if !limiter.Allow("user1") {
		t.Error("Expected request after reset to be allowed")
	}
}

func TestSessionManager(t *testing.T) {
	sm := NewSessionManager(time.Hour)
	
	// Create session
	session, err := sm.CreateSession("user123")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	
	// Get session
	retrieved, exists := sm.GetSession(session.ID)
	if !exists {
		t.Error("Expected session to exist")
	}
	if retrieved.UserID != "user123" {
		t.Errorf("Expected UserID user123, got %s", retrieved.UserID)
	}
	
	// Update session
	data := map[string]interface{}{
		"role": "admin",
		"permissions": []string{"read", "write"},
	}
	if !sm.UpdateSession(session.ID, data) {
		t.Error("Failed to update session")
	}
	
	retrieved, _ = sm.GetSession(session.ID)
	if retrieved.Data["role"] != "admin" {
		t.Error("Session data not updated correctly")
	}
	
	// Delete session
	sm.DeleteSession(session.ID)
	_, exists = sm.GetSession(session.ID)
	if exists {
		t.Error("Expected session to be deleted")
	}
}