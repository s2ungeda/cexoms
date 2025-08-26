package security

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LogAggregator centralizes log collection and processing
type LogAggregator struct {
	mu            sync.RWMutex
	sources       map[string]LogSource
	processors    []LogProcessor
	outputs       []LogOutput
	bufferSize    int
	logBuffer     chan *LogRecord
	errorBuffer   chan error
	metrics       *LogMetrics
	encryptionMgr *EncryptionManager
	running       bool
	stopChan      chan bool
	wg            sync.WaitGroup
}

// LogSource represents a log source
type LogSource interface {
	Name() string
	Type() string
	Collect(ctx context.Context) (<-chan *LogRecord, error)
	Stop() error
}

// LogProcessor processes log records
type LogProcessor interface {
	Name() string
	Process(record *LogRecord) (*LogRecord, error)
	ShouldProcess(record *LogRecord) bool
}

// LogOutput represents a log destination
type LogOutput interface {
	Name() string
	Type() string
	Write(records []*LogRecord) error
	Flush() error
}

// LogRecord represents a single log record
type LogRecord struct {
	ID         string                 `json:"id"`
	Timestamp  time.Time              `json:"timestamp"`
	Source     string                 `json:"source"`
	Level      string                 `json:"level"`
	Category   string                 `json:"category"`
	Message    string                 `json:"message"`
	UserID     string                 `json:"user_id,omitempty"`
	AccountID  string                 `json:"account_id,omitempty"`
	TraceID    string                 `json:"trace_id,omitempty"`
	SpanID     string                 `json:"span_id,omitempty"`
	Fields     map[string]interface{} `json:"fields,omitempty"`
	Encrypted  bool                   `json:"encrypted"`
	Compressed bool                   `json:"compressed"`
	Hash       string                 `json:"hash"`
}

// LogMetrics tracks aggregation metrics
type LogMetrics struct {
	mu               sync.RWMutex
	RecordsReceived  int64
	RecordsProcessed int64
	RecordsDropped   int64
	BytesProcessed   int64
	ErrorCount       int64
	LastError        error
	LastErrorTime    time.Time
	SourceMetrics    map[string]*SourceMetrics
	OutputMetrics    map[string]*OutputMetrics
}

// SourceMetrics tracks per-source metrics
type SourceMetrics struct {
	RecordsReceived int64
	RecordsDropped  int64
	LastReceived    time.Time
	ErrorCount      int64
}

// OutputMetrics tracks per-output metrics
type OutputMetrics struct {
	RecordsWritten int64
	BytesWritten   int64
	LastWritten    time.Time
	ErrorCount     int64
}

// NewLogAggregator creates a new log aggregator
func NewLogAggregator(encryptionMgr *EncryptionManager) *LogAggregator {
	return &LogAggregator{
		sources:       make(map[string]LogSource),
		processors:    make([]LogProcessor, 0),
		outputs:       make([]LogOutput, 0),
		bufferSize:    10000,
		logBuffer:     make(chan *LogRecord, 10000),
		errorBuffer:   make(chan error, 100),
		encryptionMgr: encryptionMgr,
		metrics: &LogMetrics{
			SourceMetrics: make(map[string]*SourceMetrics),
			OutputMetrics: make(map[string]*OutputMetrics),
		},
		stopChan: make(chan bool),
	}
}

// AddSource adds a log source
func (la *LogAggregator) AddSource(source LogSource) error {
	la.mu.Lock()
	defer la.mu.Unlock()
	
	if _, exists := la.sources[source.Name()]; exists {
		return fmt.Errorf("source already exists: %s", source.Name())
	}
	
	la.sources[source.Name()] = source
	la.metrics.SourceMetrics[source.Name()] = &SourceMetrics{}
	
	return nil
}

// AddProcessor adds a log processor
func (la *LogAggregator) AddProcessor(processor LogProcessor) {
	la.mu.Lock()
	defer la.mu.Unlock()
	
	la.processors = append(la.processors, processor)
}

// AddOutput adds a log output
func (la *LogAggregator) AddOutput(output LogOutput) {
	la.mu.Lock()
	defer la.mu.Unlock()
	
	la.outputs = append(la.outputs, output)
	la.metrics.OutputMetrics[output.Name()] = &OutputMetrics{}
}

// Start starts the log aggregator
func (la *LogAggregator) Start(ctx context.Context) error {
	la.mu.Lock()
	if la.running {
		la.mu.Unlock()
		return fmt.Errorf("aggregator already running")
	}
	la.running = true
	la.mu.Unlock()
	
	// Start collection from all sources
	for name, source := range la.sources {
		la.wg.Add(1)
		go la.collectFromSource(ctx, name, source)
	}
	
	// Start processing pipeline
	la.wg.Add(1)
	go la.processRecords()
	
	// Start batch writer
	la.wg.Add(1)
	go la.batchWriter()
	
	// Start error handler
	la.wg.Add(1)
	go la.errorHandler()
	
	return nil
}

// Stop stops the log aggregator
func (la *LogAggregator) Stop() error {
	la.mu.Lock()
	if !la.running {
		la.mu.Unlock()
		return fmt.Errorf("aggregator not running")
	}
	la.running = false
	la.mu.Unlock()
	
	// Stop all sources
	for _, source := range la.sources {
		if err := source.Stop(); err != nil {
			la.errorBuffer <- fmt.Errorf("failed to stop source %s: %w", source.Name(), err)
		}
	}
	
	// Signal stop
	close(la.stopChan)
	
	// Wait for all goroutines
	la.wg.Wait()
	
	// Flush all outputs
	for _, output := range la.outputs {
		if err := output.Flush(); err != nil {
			return fmt.Errorf("failed to flush output %s: %w", output.Name(), err)
		}
	}
	
	return nil
}

// collectFromSource collects logs from a source
func (la *LogAggregator) collectFromSource(ctx context.Context, name string, source LogSource) {
	defer la.wg.Done()
	
	recordChan, err := source.Collect(ctx)
	if err != nil {
		la.errorBuffer <- fmt.Errorf("failed to start collection from %s: %w", name, err)
		return
	}
	
	for {
		select {
		case record, ok := <-recordChan:
			if !ok {
				return
			}
			
			// Update metrics
			la.metrics.mu.Lock()
			la.metrics.RecordsReceived++
			sourceMetrics := la.metrics.SourceMetrics[name]
			sourceMetrics.RecordsReceived++
			sourceMetrics.LastReceived = time.Now()
			la.metrics.mu.Unlock()
			
			// Send to processing
			select {
			case la.logBuffer <- record:
				// Success
			default:
				// Buffer full, drop record
				la.metrics.mu.Lock()
				la.metrics.RecordsDropped++
				sourceMetrics.RecordsDropped++
				la.metrics.mu.Unlock()
			}
			
		case <-la.stopChan:
			return
		case <-ctx.Done():
			return
		}
	}
}

// processRecords processes log records
func (la *LogAggregator) processRecords() {
	defer la.wg.Done()
	
	batch := make([]*LogRecord, 0, 100)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case record := <-la.logBuffer:
			// Apply processors
			processedRecord := record
			for _, processor := range la.processors {
				if processor.ShouldProcess(processedRecord) {
					var err error
					processedRecord, err = processor.Process(processedRecord)
					if err != nil {
						la.errorBuffer <- fmt.Errorf("processor %s failed: %w", processor.Name(), err)
						continue
					}
				}
			}
			
			// Add to batch
			batch = append(batch, processedRecord)
			
			// Update metrics
			la.metrics.mu.Lock()
			la.metrics.RecordsProcessed++
			la.metrics.BytesProcessed += int64(len(processedRecord.Message))
			la.metrics.mu.Unlock()
			
			// Write batch if full
			if len(batch) >= 100 {
				la.writeBatch(batch)
				batch = make([]*LogRecord, 0, 100)
			}
			
		case <-ticker.C:
			// Write partial batch
			if len(batch) > 0 {
				la.writeBatch(batch)
				batch = make([]*LogRecord, 0, 100)
			}
			
		case <-la.stopChan:
			// Write remaining batch
			if len(batch) > 0 {
				la.writeBatch(batch)
			}
			return
		}
	}
}

// writeBatch writes a batch of records to outputs
func (la *LogAggregator) writeBatch(batch []*LogRecord) {
	for _, output := range la.outputs {
		if err := output.Write(batch); err != nil {
			la.errorBuffer <- fmt.Errorf("output %s write failed: %w", output.Name(), err)
			
			// Update metrics
			la.metrics.mu.Lock()
			outputMetrics := la.metrics.OutputMetrics[output.Name()]
			outputMetrics.ErrorCount++
			la.metrics.mu.Unlock()
		} else {
			// Update success metrics
			la.metrics.mu.Lock()
			outputMetrics := la.metrics.OutputMetrics[output.Name()]
			outputMetrics.RecordsWritten += int64(len(batch))
			outputMetrics.LastWritten = time.Now()
			
			// Estimate bytes written
			totalBytes := 0
			for _, record := range batch {
				totalBytes += len(record.Message) + 200 // Estimate metadata overhead
			}
			outputMetrics.BytesWritten += int64(totalBytes)
			la.metrics.mu.Unlock()
		}
	}
}

// batchWriter handles batch writing
func (la *LogAggregator) batchWriter() {
	defer la.wg.Done()
	
	// This is a placeholder for more complex batch writing logic
	// The actual writing is handled in processRecords
	<-la.stopChan
}

// errorHandler handles errors
func (la *LogAggregator) errorHandler() {
	defer la.wg.Done()
	
	for {
		select {
		case err := <-la.errorBuffer:
			// Log error
			fmt.Printf("Log aggregator error: %v\n", err)
			
			// Update metrics
			la.metrics.mu.Lock()
			la.metrics.ErrorCount++
			la.metrics.LastError = err
			la.metrics.LastErrorTime = time.Now()
			la.metrics.mu.Unlock()
			
		case <-la.stopChan:
			return
		}
	}
}

// GetMetrics returns current metrics
func (la *LogAggregator) GetMetrics() *LogMetrics {
	la.metrics.mu.RLock()
	defer la.metrics.mu.RUnlock()
	
	// Create a copy to avoid race conditions
	metricsCopy := &LogMetrics{
		RecordsReceived:  la.metrics.RecordsReceived,
		RecordsProcessed: la.metrics.RecordsProcessed,
		RecordsDropped:   la.metrics.RecordsDropped,
		BytesProcessed:   la.metrics.BytesProcessed,
		ErrorCount:       la.metrics.ErrorCount,
		LastError:        la.metrics.LastError,
		LastErrorTime:    la.metrics.LastErrorTime,
		SourceMetrics:    make(map[string]*SourceMetrics),
		OutputMetrics:    make(map[string]*OutputMetrics),
	}
	
	// Copy source metrics
	for name, metrics := range la.metrics.SourceMetrics {
		metricsCopy.SourceMetrics[name] = &SourceMetrics{
			RecordsReceived: metrics.RecordsReceived,
			RecordsDropped:  metrics.RecordsDropped,
			LastReceived:    metrics.LastReceived,
			ErrorCount:      metrics.ErrorCount,
		}
	}
	
	// Copy output metrics
	for name, metrics := range la.metrics.OutputMetrics {
		metricsCopy.OutputMetrics[name] = &OutputMetrics{
			RecordsWritten: metrics.RecordsWritten,
			BytesWritten:   metrics.BytesWritten,
			LastWritten:    metrics.LastWritten,
			ErrorCount:     metrics.ErrorCount,
		}
	}
	
	return metricsCopy
}

// Built-in processors

// EncryptionProcessor encrypts sensitive log data
type EncryptionProcessor struct {
	encryptionMgr    *EncryptionManager
	sensitiveFields  []string
}

func NewEncryptionProcessor(encryptionMgr *EncryptionManager) *EncryptionProcessor {
	return &EncryptionProcessor{
		encryptionMgr: encryptionMgr,
		sensitiveFields: []string{
			"password", "api_key", "secret", "token", "private_key",
		},
	}
}

func (ep *EncryptionProcessor) Name() string {
	return "encryption"
}

func (ep *EncryptionProcessor) Process(record *LogRecord) (*LogRecord, error) {
	// Check if record contains sensitive fields
	for _, field := range ep.sensitiveFields {
		if _, exists := record.Fields[field]; exists {
			// Encrypt the entire fields map
			fieldsJSON, err := json.Marshal(record.Fields)
			if err != nil {
				return record, err
			}
			
			encrypted, err := ep.encryptionMgr.Encrypt(fieldsJSON, "log_encryption")
			if err != nil {
				return record, err
			}
			
			record.Fields = map[string]interface{}{
				"encrypted_data": encrypted,
			}
			record.Encrypted = true
			break
		}
	}
	
	return record, nil
}

func (ep *EncryptionProcessor) ShouldProcess(record *LogRecord) bool {
	return !record.Encrypted
}

// CompressionProcessor compresses log data
type CompressionProcessor struct {
	threshold int // Compress if message size exceeds threshold
}

func NewCompressionProcessor(threshold int) *CompressionProcessor {
	return &CompressionProcessor{
		threshold: threshold,
	}
}

func (cp *CompressionProcessor) Name() string {
	return "compression"
}

func (cp *CompressionProcessor) Process(record *LogRecord) (*LogRecord, error) {
	if len(record.Message) < cp.threshold {
		return record, nil
	}
	
	compressed, err := compressString(record.Message)
	if err != nil {
		return record, err
	}
	
	record.Message = compressed
	record.Compressed = true
	
	return record, nil
}

func (cp *CompressionProcessor) ShouldProcess(record *LogRecord) bool {
	return !record.Compressed && len(record.Message) >= cp.threshold
}

// FilterProcessor filters logs based on criteria
type FilterProcessor struct {
	minLevel zapcore.Level
	excludeCategories []string
}

func NewFilterProcessor(minLevel zapcore.Level, excludeCategories []string) *FilterProcessor {
	return &FilterProcessor{
		minLevel: minLevel,
		excludeCategories: excludeCategories,
	}
}

func (fp *FilterProcessor) Name() string {
	return "filter"
}

func (fp *FilterProcessor) Process(record *LogRecord) (*LogRecord, error) {
	// Check if should be filtered
	level, _ := zapcore.ParseLevel(record.Level)
	if level < fp.minLevel {
		return nil, fmt.Errorf("filtered: level too low")
	}
	
	for _, category := range fp.excludeCategories {
		if record.Category == category {
			return nil, fmt.Errorf("filtered: excluded category")
		}
	}
	
	return record, nil
}

func (fp *FilterProcessor) ShouldProcess(record *LogRecord) bool {
	return true
}

// Built-in outputs

// FileOutput writes logs to files
type FileOutput struct {
	basePath     string
	rotateSize   int64
	maxFiles     int
	currentFile  *os.File
	currentSize  int64
	mu           sync.Mutex
	encoder      *json.Encoder
}

func NewFileOutput(basePath string, rotateSize int64, maxFiles int) *FileOutput {
	return &FileOutput{
		basePath:   basePath,
		rotateSize: rotateSize,
		maxFiles:   maxFiles,
	}
}

func (fo *FileOutput) Name() string {
	return "file"
}

func (fo *FileOutput) Type() string {
	return "file"
}

func (fo *FileOutput) Write(records []*LogRecord) error {
	fo.mu.Lock()
	defer fo.mu.Unlock()
	
	for _, record := range records {
		// Open file if needed
		if fo.currentFile == nil {
			if err := fo.openFile(); err != nil {
				return err
			}
		}
		
		// Write record
		if err := fo.encoder.Encode(record); err != nil {
			return err
		}
		
		// Update size
		recordSize := int64(len(record.Message) + 200) // Estimate
		fo.currentSize += recordSize
		
		// Rotate if needed
		if fo.currentSize >= fo.rotateSize {
			if err := fo.rotateFile(); err != nil {
				return err
			}
		}
	}
	
	return nil
}

func (fo *FileOutput) Flush() error {
	fo.mu.Lock()
	defer fo.mu.Unlock()
	
	if fo.currentFile != nil {
		if err := fo.currentFile.Sync(); err != nil {
			return err
		}
		return fo.currentFile.Close()
	}
	
	return nil
}

func (fo *FileOutput) openFile() error {
	filename := filepath.Join(fo.basePath, fmt.Sprintf("logs_%s.json", time.Now().Format("20060102_150405")))
	
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	
	fo.currentFile = file
	fo.encoder = json.NewEncoder(file)
	fo.currentSize = 0
	
	return nil
}

func (fo *FileOutput) rotateFile() error {
	if fo.currentFile != nil {
		fo.currentFile.Close()
	}
	
	return fo.openFile()
}

// Helper functions

func compressString(s string) (string, error) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	
	if _, err := gz.Write([]byte(s)); err != nil {
		return "", err
	}
	
	if err := gz.Close(); err != nil {
		return "", err
	}
	
	return b.String(), nil
}

func decompressString(s string) (string, error) {
	r := bytes.NewReader([]byte(s))
	gz, err := gzip.NewReader(r)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	
	data, err := io.ReadAll(gz)
	if err != nil {
		return "", err
	}
	
	return string(data), nil
}