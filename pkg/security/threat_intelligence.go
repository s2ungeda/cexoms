package security

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ThreatIntelligence manages threat intelligence feeds and indicators
type ThreatIntelligence struct {
	mu              sync.RWMutex
	indicators      map[string]*ThreatIndicator
	feeds           map[string]*ThreatFeed
	ipReputation    map[string]*IPReputation
	domainReputation map[string]*DomainReputation
	fileHashes      map[string]*MalwareHash
	updateInterval  time.Duration
	httpClient      *http.Client
	running         bool
	stopChan        chan bool
}

// ThreatIndicator represents a threat indicator
type ThreatIndicator struct {
	ID          string
	Type        IndicatorType
	Indicator   string
	Severity    ThreatSeverity
	Confidence  float64
	Source      string
	FirstSeen   time.Time
	LastSeen    time.Time
	Description string
	Tags        []string
	TTL         time.Duration
}

// IndicatorType defines types of threat indicators
type IndicatorType string

const (
	IndicatorTypeIP         IndicatorType = "ip"
	IndicatorTypeDomain     IndicatorType = "domain"
	IndicatorTypeURL        IndicatorType = "url"
	IndicatorTypeFileHash   IndicatorType = "file_hash"
	IndicatorTypeEmail      IndicatorType = "email"
	IndicatorTypeCVE        IndicatorType = "cve"
)

// ThreatFeed represents a threat intelligence feed
type ThreatFeed struct {
	ID          string
	Name        string
	URL         string
	Type        FeedType
	Format      FeedFormat
	APIKey      string
	LastUpdate  time.Time
	UpdateFreq  time.Duration
	Enabled     bool
	Priority    int
}

// FeedType defines types of threat feeds
type FeedType string

const (
	FeedTypeOpen        FeedType = "open"
	FeedTypeCommercial  FeedType = "commercial"
	FeedTypeInternal    FeedType = "internal"
	FeedTypeGovernment  FeedType = "government"
)

// FeedFormat defines feed data formats
type FeedFormat string

const (
	FormatJSON  FeedFormat = "json"
	FormatCSV   FeedFormat = "csv"
	FormatSTIX  FeedFormat = "stix"
	FormatTAXII FeedFormat = "taxii"
	FormatText  FeedFormat = "text"
)

// IPReputation represents IP reputation data
type IPReputation struct {
	IP             string
	Score          float64 // 0-100, 0 = malicious, 100 = safe
	Country        string
	ASN            string
	ISP            string
	Categories     []string
	ThreatTypes    []string
	LastActivity   time.Time
	ReportCount    int
}

// DomainReputation represents domain reputation data
type DomainReputation struct {
	Domain         string
	Score          float64
	Categories     []string
	MalwareFamily  []string
	WhoisAge       int // days
	DNSRecords     map[string][]string
	SSLCertificate *SSLInfo
	LastChecked    time.Time
}

// MalwareHash represents known malware file hashes
type MalwareHash struct {
	Hash          string
	HashType      string // md5, sha1, sha256
	MalwareFamily string
	FirstSeen     time.Time
	Prevalence    int
	FileType      string
	FileSize      int64
	Behaviors     []string
}

// SSLInfo represents SSL certificate information
type SSLInfo struct {
	Issuer      string
	Subject     string
	NotBefore   time.Time
	NotAfter    time.Time
	SerialNumber string
	Fingerprint string
}

// NewThreatIntelligence creates a new threat intelligence system
func NewThreatIntelligence() *ThreatIntelligence {
	ti := &ThreatIntelligence{
		indicators:       make(map[string]*ThreatIndicator),
		feeds:           make(map[string]*ThreatFeed),
		ipReputation:    make(map[string]*IPReputation),
		domainReputation: make(map[string]*DomainReputation),
		fileHashes:      make(map[string]*MalwareHash),
		updateInterval:  1 * time.Hour,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		stopChan: make(chan bool),
	}
	
	// Initialize default feeds
	ti.initializeFeeds()
	
	return ti
}

// initializeFeeds sets up default threat intelligence feeds
func (ti *ThreatIntelligence) initializeFeeds() {
	// Open threat feeds
	ti.feeds["abuse_ch_urlhaus"] = &ThreatFeed{
		ID:         "abuse_ch_urlhaus",
		Name:       "Abuse.ch URLhaus",
		URL:        "https://urlhaus.abuse.ch/api/",
		Type:       FeedTypeOpen,
		Format:     FormatJSON,
		UpdateFreq: 1 * time.Hour,
		Enabled:    true,
		Priority:   1,
	}
	
	ti.feeds["abuse_ch_feodo"] = &ThreatFeed{
		ID:         "abuse_ch_feodo",
		Name:       "Abuse.ch Feodo Tracker",
		URL:        "https://feodotracker.abuse.ch/api/",
		Type:       FeedTypeOpen,
		Format:     FormatJSON,
		UpdateFreq: 6 * time.Hour,
		Enabled:    true,
		Priority:   1,
	}
	
	ti.feeds["otx_alienvault"] = &ThreatFeed{
		ID:         "otx_alienvault",
		Name:       "AlienVault OTX",
		URL:        "https://otx.alienvault.com/api/v1/",
		Type:       FeedTypeOpen,
		Format:     FormatJSON,
		UpdateFreq: 4 * time.Hour,
		Enabled:    false, // Requires API key
		Priority:   2,
	}
	
	// Internal feeds
	ti.feeds["internal_blocklist"] = &ThreatFeed{
		ID:         "internal_blocklist",
		Name:       "Internal Blocklist",
		URL:        "internal://blocklist",
		Type:       FeedTypeInternal,
		Format:     FormatJSON,
		UpdateFreq: 30 * time.Minute,
		Enabled:    true,
		Priority:   0, // Highest priority
	}
	
	// Initialize sample threat indicators
	ti.initializeSampleIndicators()
}

// initializeSampleIndicators adds sample threat indicators
func (ti *ThreatIntelligence) initializeSampleIndicators() {
	// Known bad IPs (examples)
	badIPs := []string{
		"192.0.2.1",     // Documentation IP (safe for examples)
		"198.51.100.1",  // Documentation IP
		"203.0.113.1",   // Documentation IP
	}
	
	for _, ip := range badIPs {
		indicator := &ThreatIndicator{
			ID:          fmt.Sprintf("ip_%s", strings.ReplaceAll(ip, ".", "_")),
			Type:        IndicatorTypeIP,
			Indicator:   ip,
			Severity:    SeverityHigh,
			Confidence:  0.9,
			Source:      "internal_blocklist",
			FirstSeen:   time.Now().Add(-30 * 24 * time.Hour),
			LastSeen:    time.Now(),
			Description: "Known malicious IP",
			Tags:        []string{"malware", "c2"},
			TTL:         7 * 24 * time.Hour,
		}
		
		ti.indicators[indicator.ID] = indicator
		
		// Add IP reputation
		ti.ipReputation[ip] = &IPReputation{
			IP:           ip,
			Score:        10.0, // Low score = bad
			Country:      "XX",
			ThreatTypes:  []string{"malware", "botnet"},
			LastActivity: time.Now(),
			ReportCount:  42,
		}
	}
	
	// Known bad domains
	badDomains := []string{
		"malicious.example.com",
		"phishing.example.net",
		"malware.example.org",
	}
	
	for _, domain := range badDomains {
		indicator := &ThreatIndicator{
			ID:          fmt.Sprintf("domain_%s", strings.ReplaceAll(domain, ".", "_")),
			Type:        IndicatorTypeDomain,
			Indicator:   domain,
			Severity:    SeverityHigh,
			Confidence:  0.85,
			Source:      "internal_blocklist",
			FirstSeen:   time.Now().Add(-60 * 24 * time.Hour),
			LastSeen:    time.Now(),
			Description: "Known phishing domain",
			Tags:        []string{"phishing"},
			TTL:         14 * 24 * time.Hour,
		}
		
		ti.indicators[indicator.ID] = indicator
		
		// Add domain reputation
		ti.domainReputation[domain] = &DomainReputation{
			Domain:        domain,
			Score:         5.0,
			Categories:    []string{"phishing", "malware"},
			MalwareFamily: []string{"emotet"},
			WhoisAge:      30,
			LastChecked:   time.Now(),
		}
	}
	
	// Known malware hashes
	malwareHashes := map[string]string{
		"d41d8cd98f00b204e9800998ecf8427e": "TestMalware",
		"da39a3ee5e6b4b0d3255bfef95601890": "TestRansomware",
	}
	
	for hash, family := range malwareHashes {
		ti.fileHashes[hash] = &MalwareHash{
			Hash:          hash,
			HashType:      "md5",
			MalwareFamily: family,
			FirstSeen:     time.Now().Add(-90 * 24 * time.Hour),
			Prevalence:    100,
			FileType:      "exe",
			Behaviors:     []string{"file_encryption", "network_communication"},
		}
	}
}

// Start starts the threat intelligence service
func (ti *ThreatIntelligence) Start(ctx context.Context) {
	ti.mu.Lock()
	if ti.running {
		ti.mu.Unlock()
		return
	}
	ti.running = true
	ti.mu.Unlock()
	
	// Start feed updater
	go ti.feedUpdater(ctx)
	
	// Start indicator cleaner
	go ti.indicatorCleaner(ctx)
}

// Stop stops the threat intelligence service
func (ti *ThreatIntelligence) Stop() {
	ti.mu.Lock()
	if !ti.running {
		ti.mu.Unlock()
		return
	}
	ti.running = false
	ti.mu.Unlock()
	
	close(ti.stopChan)
}

// Check checks an event against threat intelligence
func (ti *ThreatIntelligence) Check(event *AuditEvent) *ThreatIndicator {
	ti.mu.RLock()
	defer ti.mu.RUnlock()
	
	// Check IP address
	if event.IPAddress != "" {
		if ti.IsKnownBadIP(event.IPAddress) {
			for _, indicator := range ti.indicators {
				if indicator.Type == IndicatorTypeIP && indicator.Indicator == event.IPAddress {
					return indicator
				}
			}
		}
	}
	
	// Check domain/URL in metadata
	if url, ok := event.Metadata["url"].(string); ok {
		if domain := extractDomain(url); domain != "" {
			if ti.IsKnownBadDomain(domain) {
				for _, indicator := range ti.indicators {
					if indicator.Type == IndicatorTypeDomain && indicator.Indicator == domain {
						return indicator
					}
				}
			}
		}
	}
	
	// Check file hash
	if hash, ok := event.Metadata["file_hash"].(string); ok {
		if ti.IsKnownMalwareHash(hash) {
			indicator := &ThreatIndicator{
				ID:          fmt.Sprintf("hash_%s", hash[:8]),
				Type:        IndicatorTypeFileHash,
				Indicator:   hash,
				Severity:    SeverityCritical,
				Confidence:  0.95,
				Source:      "malware_database",
				Description: "Known malware hash",
				Tags:        []string{"malware"},
			}
			return indicator
		}
	}
	
	return nil
}

// IsKnownBadIP checks if IP is known bad
func (ti *ThreatIntelligence) IsKnownBadIP(ip string) bool {
	ti.mu.RLock()
	defer ti.mu.RUnlock()
	
	// Check exact match
	if rep, exists := ti.ipReputation[ip]; exists {
		return rep.Score < 30.0 // Score below 30 is considered bad
	}
	
	// Check CIDR ranges
	for _, indicator := range ti.indicators {
		if indicator.Type == IndicatorTypeIP {
			if strings.Contains(indicator.Indicator, "/") {
				// CIDR notation
				_, network, err := net.ParseCIDR(indicator.Indicator)
				if err == nil && network.Contains(net.ParseIP(ip)) {
					return true
				}
			}
		}
	}
	
	return false
}

// IsKnownBadDomain checks if domain is known bad
func (ti *ThreatIntelligence) IsKnownBadDomain(domain string) bool {
	ti.mu.RLock()
	defer ti.mu.RUnlock()
	
	// Check exact match
	if rep, exists := ti.domainReputation[domain]; exists {
		return rep.Score < 30.0
	}
	
	// Check parent domains
	parts := strings.Split(domain, ".")
	for i := 1; i < len(parts); i++ {
		parentDomain := strings.Join(parts[i:], ".")
		if rep, exists := ti.domainReputation[parentDomain]; exists {
			return rep.Score < 30.0
		}
	}
	
	return false
}

// IsKnownMalwareHash checks if file hash is known malware
func (ti *ThreatIntelligence) IsKnownMalwareHash(hash string) bool {
	ti.mu.RLock()
	defer ti.mu.RUnlock()
	
	_, exists := ti.fileHashes[strings.ToLower(hash)]
	return exists
}

// AddIndicator adds a new threat indicator
func (ti *ThreatIntelligence) AddIndicator(indicator *ThreatIndicator) error {
	ti.mu.Lock()
	defer ti.mu.Unlock()
	
	// Validate indicator
	if indicator.ID == "" {
		indicator.ID = generateIndicatorID()
	}
	
	if indicator.FirstSeen.IsZero() {
		indicator.FirstSeen = time.Now()
	}
	
	indicator.LastSeen = time.Now()
	
	// Store indicator
	ti.indicators[indicator.ID] = indicator
	
	// Update specific maps
	switch indicator.Type {
	case IndicatorTypeIP:
		ti.updateIPReputation(indicator)
	case IndicatorTypeDomain:
		ti.updateDomainReputation(indicator)
	case IndicatorTypeFileHash:
		ti.updateFileHash(indicator)
	}
	
	return nil
}

// updateIPReputation updates IP reputation from indicator
func (ti *ThreatIntelligence) updateIPReputation(indicator *ThreatIndicator) {
	ip := indicator.Indicator
	
	rep, exists := ti.ipReputation[ip]
	if !exists {
		rep = &IPReputation{
			IP: ip,
		}
		ti.ipReputation[ip] = rep
	}
	
	// Update reputation based on indicator
	switch indicator.Severity {
	case SeverityCritical:
		rep.Score = 0.0
	case SeverityHigh:
		rep.Score = 10.0
	case SeverityMedium:
		rep.Score = 30.0
	case SeverityLow:
		rep.Score = 50.0
	default:
		rep.Score = 70.0
	}
	
	rep.ThreatTypes = indicator.Tags
	rep.LastActivity = indicator.LastSeen
	rep.ReportCount++
}

// updateDomainReputation updates domain reputation from indicator
func (ti *ThreatIntelligence) updateDomainReputation(indicator *ThreatIndicator) {
	domain := indicator.Indicator
	
	rep, exists := ti.domainReputation[domain]
	if !exists {
		rep = &DomainReputation{
			Domain: domain,
		}
		ti.domainReputation[domain] = rep
	}
	
	// Update reputation
	switch indicator.Severity {
	case SeverityCritical:
		rep.Score = 0.0
	case SeverityHigh:
		rep.Score = 10.0
	case SeverityMedium:
		rep.Score = 30.0
	case SeverityLow:
		rep.Score = 50.0
	default:
		rep.Score = 70.0
	}
	
	rep.Categories = indicator.Tags
	rep.LastChecked = time.Now()
}

// updateFileHash updates file hash from indicator
func (ti *ThreatIntelligence) updateFileHash(indicator *ThreatIndicator) {
	hash := strings.ToLower(indicator.Indicator)
	
	malware, exists := ti.fileHashes[hash]
	if !exists {
		malware = &MalwareHash{
			Hash:      hash,
			HashType:  detectHashType(hash),
			FirstSeen: indicator.FirstSeen,
		}
		ti.fileHashes[hash] = malware
	}
	
	// Extract malware family from tags
	for _, tag := range indicator.Tags {
		if strings.HasPrefix(tag, "family:") {
			malware.MalwareFamily = strings.TrimPrefix(tag, "family:")
			break
		}
	}
	
	malware.Prevalence++
}

// feedUpdater periodically updates threat feeds
func (ti *ThreatIntelligence) feedUpdater(ctx context.Context) {
	ticker := time.NewTicker(ti.updateInterval)
	defer ticker.Stop()
	
	// Initial update
	ti.updateAllFeeds()
	
	for {
		select {
		case <-ticker.C:
			ti.updateAllFeeds()
			
		case <-ctx.Done():
			return
		case <-ti.stopChan:
			return
		}
	}
}

// updateAllFeeds updates all enabled feeds
func (ti *ThreatIntelligence) updateAllFeeds() {
	ti.mu.RLock()
	feeds := make([]*ThreatFeed, 0)
	for _, feed := range ti.feeds {
		if feed.Enabled && time.Since(feed.LastUpdate) > feed.UpdateFreq {
			feeds = append(feeds, feed)
		}
	}
	ti.mu.RUnlock()
	
	// Update feeds concurrently
	var wg sync.WaitGroup
	for _, feed := range feeds {
		wg.Add(1)
		go func(f *ThreatFeed) {
			defer wg.Done()
			if err := ti.updateFeed(f); err != nil {
				fmt.Printf("Failed to update feed %s: %v\n", f.Name, err)
			}
		}(feed)
	}
	
	wg.Wait()
}

// updateFeed updates a single threat feed
func (ti *ThreatIntelligence) updateFeed(feed *ThreatFeed) error {
	// Handle internal feeds differently
	if feed.Type == FeedTypeInternal {
		return ti.updateInternalFeed(feed)
	}
	
	// For external feeds, fetch data
	// This is a placeholder - actual implementation would fetch real data
	feed.LastUpdate = time.Now()
	
	return nil
}

// updateInternalFeed updates internal threat feed
func (ti *ThreatIntelligence) updateInternalFeed(feed *ThreatFeed) error {
	// This would load from internal sources
	// For now, just update timestamp
	feed.LastUpdate = time.Now()
	return nil
}

// indicatorCleaner removes expired indicators
func (ti *ThreatIntelligence) indicatorCleaner(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			ti.cleanExpiredIndicators()
			
		case <-ctx.Done():
			return
		case <-ti.stopChan:
			return
		}
	}
}

// cleanExpiredIndicators removes expired indicators
func (ti *ThreatIntelligence) cleanExpiredIndicators() {
	ti.mu.Lock()
	defer ti.mu.Unlock()
	
	now := time.Now()
	toDelete := make([]string, 0)
	
	for id, indicator := range ti.indicators {
		if indicator.TTL > 0 && now.Sub(indicator.LastSeen) > indicator.TTL {
			toDelete = append(toDelete, id)
		}
	}
	
	// Delete expired indicators
	for _, id := range toDelete {
		indicator := ti.indicators[id]
		delete(ti.indicators, id)
		
		// Clean from specific maps
		switch indicator.Type {
		case IndicatorTypeIP:
			delete(ti.ipReputation, indicator.Indicator)
		case IndicatorTypeDomain:
			delete(ti.domainReputation, indicator.Indicator)
		case IndicatorTypeFileHash:
			delete(ti.fileHashes, indicator.Indicator)
		}
	}
}

// GetIPReputation returns IP reputation
func (ti *ThreatIntelligence) GetIPReputation(ip string) (*IPReputation, bool) {
	ti.mu.RLock()
	defer ti.mu.RUnlock()
	
	rep, exists := ti.ipReputation[ip]
	return rep, exists
}

// GetDomainReputation returns domain reputation
func (ti *ThreatIntelligence) GetDomainReputation(domain string) (*DomainReputation, bool) {
	ti.mu.RLock()
	defer ti.mu.RUnlock()
	
	rep, exists := ti.domainReputation[domain]
	return rep, exists
}

// Helper functions

func extractDomain(url string) string {
	// Simple domain extraction
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "https://")
	
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[0]
	}
	
	return ""
}

func detectHashType(hash string) string {
	switch len(hash) {
	case 32:
		return "md5"
	case 40:
		return "sha1"
	case 64:
		return "sha256"
	default:
		return "unknown"
	}
}

func generateIndicatorID() string {
	return fmt.Sprintf("IND_%d_%s", time.Now().Unix(), generateRandomString(8))
}