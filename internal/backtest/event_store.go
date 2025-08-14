package backtest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// EventType represents the type of market event
type EventType string

const (
	EventTypeOrderBook EventType = "orderbook"
	EventTypeTrade     EventType = "trade"
	EventTypeTicker    EventType = "ticker"
	EventTypeOrder     EventType = "order"
	EventTypePosition  EventType = "position"
)

// MarketEvent represents a historical market event
type MarketEvent struct {
	Type      EventType              `json:"type"`
	Exchange  string                 `json:"exchange"`
	Symbol    string                 `json:"symbol"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// EventStore manages historical event storage and retrieval
type EventStore struct {
	mu sync.RWMutex
	
	// Storage configuration
	dataDir        string
	eventsPerFile  int
	currentWriters map[string]*eventWriter
	
	// Index for fast retrieval
	index map[string]*eventIndex // key: "exchange:symbol"
}

// eventWriter handles writing events to files
type eventWriter struct {
	file      *os.File
	writer    *bufio.Writer
	count     int
	timestamp time.Time
}

// eventIndex maintains index of event files
type eventIndex struct {
	files []eventFile
}

// eventFile represents a single event file
type eventFile struct {
	path      string
	startTime time.Time
	endTime   time.Time
	count     int
}

// NewEventStore creates a new event store
func NewEventStore(dataDir string) (*EventStore, error) {
	es := &EventStore{
		dataDir:        dataDir,
		eventsPerFile:  100000, // 100k events per file
		currentWriters: make(map[string]*eventWriter),
		index:          make(map[string]*eventIndex),
	}
	
	// Create data directory
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data dir: %w", err)
	}
	
	// Build index from existing files
	if err := es.buildIndex(); err != nil {
		return nil, fmt.Errorf("failed to build index: %w", err)
	}
	
	return es, nil
}

// RecordEvent records a market event
func (es *EventStore) RecordEvent(event *MarketEvent) error {
	es.mu.Lock()
	defer es.mu.Unlock()
	
	key := fmt.Sprintf("%s:%s", event.Exchange, event.Symbol)
	writer := es.currentWriters[key]
	
	// Create new writer if needed
	if writer == nil || writer.count >= es.eventsPerFile {
		if writer != nil {
			es.closeWriter(writer)
		}
		
		var err error
		writer, err = es.createWriter(event.Exchange, event.Symbol, event.Type)
		if err != nil {
			return fmt.Errorf("failed to create writer: %w", err)
		}
		
		es.currentWriters[key] = writer
	}
	
	// Write event
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}
	
	if _, err := writer.writer.Write(data); err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}
	
	if _, err := writer.writer.Write([]byte("\n")); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}
	
	writer.count++
	
	// Flush periodically
	if writer.count%1000 == 0 {
		writer.writer.Flush()
	}
	
	return nil
}

// GetEvents retrieves events for a time range
func (es *EventStore) GetEvents(exchange, symbol string, startTime, endTime time.Time) ([]*MarketEvent, error) {
	es.mu.RLock()
	defer es.mu.RUnlock()
	
	key := fmt.Sprintf("%s:%s", exchange, symbol)
	index, exists := es.index[key]
	if !exists {
		return nil, nil // No events
	}
	
	var events []*MarketEvent
	
	// Find relevant files
	for _, file := range index.files {
		// Skip files outside time range
		if file.endTime.Before(startTime) || file.startTime.After(endTime) {
			continue
		}
		
		// Read events from file
		fileEvents, err := es.readEventsFromFile(file.path, startTime, endTime)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", file.path, err)
		}
		
		events = append(events, fileEvents...)
	}
	
	// Sort by timestamp
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	
	return events, nil
}

// StreamEvents streams events for replay
func (es *EventStore) StreamEvents(exchange, symbol string, startTime, endTime time.Time) (<-chan *MarketEvent, error) {
	events, err := es.GetEvents(exchange, symbol, startTime, endTime)
	if err != nil {
		return nil, err
	}
	
	ch := make(chan *MarketEvent, 100)
	
	go func() {
		defer close(ch)
		
		for _, event := range events {
			ch <- event
		}
	}()
	
	return ch, nil
}

// createWriter creates a new event writer
func (es *EventStore) createWriter(exchange, symbol string, eventType EventType) (*eventWriter, error) {
	// Create directory structure: data/exchange/symbol/type/
	dir := filepath.Join(es.dataDir, exchange, symbol, string(eventType))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	
	// Create filename with timestamp
	filename := fmt.Sprintf("events_%s.jsonl", time.Now().Format("20060102_150405"))
	path := filepath.Join(dir, filename)
	
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	
	return &eventWriter{
		file:      file,
		writer:    bufio.NewWriterSize(file, 64*1024),
		count:     0,
		timestamp: time.Now(),
	}, nil
}

// closeWriter closes an event writer
func (es *EventStore) closeWriter(writer *eventWriter) error {
	writer.writer.Flush()
	return writer.file.Close()
}

// buildIndex builds the file index
func (es *EventStore) buildIndex() error {
	return filepath.Walk(es.dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		
		// Only process .jsonl files
		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		
		// Extract exchange and symbol from path
		rel, _ := filepath.Rel(es.dataDir, path)
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) < 3 {
			return nil
		}
		
		exchange := parts[0]
		symbol := parts[1]
		key := fmt.Sprintf("%s:%s", exchange, symbol)
		
		// Get file info
		fileInfo, err := es.getFileInfo(path)
		if err != nil {
			return nil // Skip problematic files
		}
		
		// Add to index
		if es.index[key] == nil {
			es.index[key] = &eventIndex{}
		}
		es.index[key].files = append(es.index[key].files, *fileInfo)
		
		return nil
	})
}

// getFileInfo gets information about an event file
func (es *EventStore) getFileInfo(path string) (*eventFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	
	var firstTime, lastTime time.Time
	count := 0
	
	for scanner.Scan() {
		count++
		
		// Parse event to get timestamp
		var event MarketEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		
		if count == 1 {
			firstTime = event.Timestamp
		}
		lastTime = event.Timestamp
	}
	
	return &eventFile{
		path:      path,
		startTime: firstTime,
		endTime:   lastTime,
		count:     count,
	}, nil
}

// readEventsFromFile reads events from a file within time range
func (es *EventStore) readEventsFromFile(path string, startTime, endTime time.Time) ([]*MarketEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	
	var events []*MarketEvent
	scanner := bufio.NewScanner(file)
	
	for scanner.Scan() {
		var event MarketEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		
		// Filter by time range
		if event.Timestamp.Before(startTime) || event.Timestamp.After(endTime) {
			continue
		}
		
		events = append(events, &event)
	}
	
	return events, scanner.Err()
}

// GetStatistics returns statistics about stored events
func (es *EventStore) GetStatistics() map[string]interface{} {
	es.mu.RLock()
	defer es.mu.RUnlock()
	
	stats := make(map[string]interface{})
	totalFiles := 0
	totalEvents := 0
	
	exchangeStats := make(map[string]map[string]int)
	
	for key, index := range es.index {
		parts := strings.Split(key, ":")
		exchange := parts[0]
		symbol := parts[1]
		
		if exchangeStats[exchange] == nil {
			exchangeStats[exchange] = make(map[string]int)
		}
		
		fileCount := len(index.files)
		eventCount := 0
		for _, file := range index.files {
			eventCount += file.count
		}
		
		exchangeStats[exchange][symbol] = eventCount
		totalFiles += fileCount
		totalEvents += eventCount
	}
	
	stats["total_files"] = totalFiles
	stats["total_events"] = totalEvents
	stats["exchanges"] = exchangeStats
	
	return stats
}

// Close closes the event store
func (es *EventStore) Close() error {
	es.mu.Lock()
	defer es.mu.Unlock()
	
	// Close all writers
	for _, writer := range es.currentWriters {
		es.closeWriter(writer)
	}
	
	return nil
}