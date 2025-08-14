package monitor

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LogLevel represents the severity of a log entry
type LogLevel string

const (
	LogLevelDebug LogLevel = "DEBUG"
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
	LogLevelFatal LogLevel = "FATAL"
)

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp   time.Time              `json:"timestamp"`
	Level       LogLevel               `json:"level"`
	Component   string                 `json:"component"`
	Message     string                 `json:"message"`
	Fields      map[string]interface{} `json:"fields,omitempty"`
	TraceID     string                 `json:"trace_id,omitempty"`
	Exchange    string                 `json:"exchange,omitempty"`
	OrderID     string                 `json:"order_id,omitempty"`
	Symbol      string                 `json:"symbol,omitempty"`
	ErrorStack  string                 `json:"error_stack,omitempty"`
}

// Logger provides structured logging with file rotation
type Logger struct {
	mu sync.Mutex
	
	// Configuration
	component      string
	minLevel       LogLevel
	logDir         string
	maxFileSize    int64
	rotateInterval time.Duration
	
	// Current file
	currentFile *os.File
	fileSize    int64
	lastRotate  time.Time
	
	// Writers
	writers []io.Writer
	encoder *json.Encoder
	
	// Metrics
	logCounts map[LogLevel]int64
}

// NewLogger creates a new logger
func NewLogger(component, logDir string) (*Logger, error) {
	logger := &Logger{
		component:      component,
		minLevel:       LogLevelInfo,
		logDir:         logDir,
		maxFileSize:    50 * 1024 * 1024, // 50MB
		rotateInterval: 24 * time.Hour,
		logCounts:      make(map[LogLevel]int64),
		writers:        []io.Writer{os.Stdout}, // Always write to stdout
	}
	
	// Create log directory
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log dir: %w", err)
	}
	
	// Open initial log file
	if err := logger.rotateFile(); err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}
	
	return logger, nil
}

// SetMinLevel sets the minimum log level
func (l *Logger) SetMinLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.minLevel = level
}

// AddWriter adds an additional log writer
func (l *Logger) AddWriter(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writers = append(l.writers, w)
}

// Log methods

// Debug logs a debug message
func (l *Logger) Debug(message string, fields ...map[string]interface{}) {
	l.log(LogLevelDebug, message, mergeFields(fields...))
}

// Info logs an info message
func (l *Logger) Info(message string, fields ...map[string]interface{}) {
	l.log(LogLevelInfo, message, mergeFields(fields...))
}

// Warn logs a warning message
func (l *Logger) Warn(message string, fields ...map[string]interface{}) {
	l.log(LogLevelWarn, message, mergeFields(fields...))
}

// Error logs an error message
func (l *Logger) Error(message string, err error, fields ...map[string]interface{}) {
	f := mergeFields(fields...)
	if err != nil {
		f["error"] = err.Error()
		// Add stack trace for errors
		f["error_stack"] = fmt.Sprintf("%+v", err)
	}
	l.log(LogLevelError, message, f)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(message string, err error, fields ...map[string]interface{}) {
	f := mergeFields(fields...)
	if err != nil {
		f["error"] = err.Error()
		f["error_stack"] = fmt.Sprintf("%+v", err)
	}
	l.log(LogLevelFatal, message, f)
	os.Exit(1)
}

// WithFields returns a logger with preset fields
func (l *Logger) WithFields(fields map[string]interface{}) *LoggerWithFields {
	return &LoggerWithFields{
		logger: l,
		fields: fields,
	}
}

// log writes a log entry
func (l *Logger) log(level LogLevel, message string, fields map[string]interface{}) {
	// Check minimum level
	if !l.shouldLog(level) {
		return
	}
	
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Component: l.component,
		Message:   message,
		Fields:    fields,
	}
	
	// Extract special fields
	if traceID, ok := fields["trace_id"].(string); ok {
		entry.TraceID = traceID
	}
	if exchange, ok := fields["exchange"].(string); ok {
		entry.Exchange = exchange
	}
	if orderID, ok := fields["order_id"].(string); ok {
		entry.OrderID = orderID
	}
	if symbol, ok := fields["symbol"].(string); ok {
		entry.Symbol = symbol
	}
	if stack, ok := fields["error_stack"].(string); ok {
		entry.ErrorStack = stack
		delete(fields, "error_stack") // Remove from fields to avoid duplication
	}
	
	l.mu.Lock()
	defer l.mu.Unlock()
	
	// Update metrics
	l.logCounts[level]++
	
	// Check if rotation needed
	if l.shouldRotate() {
		l.rotateFile()
	}
	
	// Write to all writers
	data, _ := json.Marshal(entry)
	data = append(data, '\n')
	
	for _, w := range l.writers {
		w.Write(data)
	}
	
	// Write to file
	if l.currentFile != nil {
		n, _ := l.currentFile.Write(data)
		l.fileSize += int64(n)
	}
}

// shouldLog checks if a log level should be logged
func (l *Logger) shouldLog(level LogLevel) bool {
	levels := []LogLevel{LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError, LogLevelFatal}
	
	var minIdx, levelIdx int
	for i, lvl := range levels {
		if lvl == l.minLevel {
			minIdx = i
		}
		if lvl == level {
			levelIdx = i
		}
	}
	
	return levelIdx >= minIdx
}

// shouldRotate checks if file rotation is needed
func (l *Logger) shouldRotate() bool {
	if l.fileSize >= l.maxFileSize {
		return true
	}
	
	if time.Since(l.lastRotate) >= l.rotateInterval {
		return true
	}
	
	return false
}

// rotateFile rotates the log file
func (l *Logger) rotateFile() error {
	// Close current file
	if l.currentFile != nil {
		l.currentFile.Close()
		
		// Update writers to remove old file
		newWriters := []io.Writer{os.Stdout}
		for _, w := range l.writers {
			if w != l.currentFile {
				newWriters = append(newWriters, w)
			}
		}
		l.writers = newWriters
	}
	
	// Create new file
	filename := filepath.Join(l.logDir,
		fmt.Sprintf("%s_%s.log", l.component, time.Now().Format("20060102_150405")))
	
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	
	l.currentFile = file
	l.fileSize = 0
	l.lastRotate = time.Now()
	l.writers = append(l.writers, file)
	
	return nil
}

// GetMetrics returns logging metrics
func (l *Logger) GetMetrics() map[string]interface{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	return map[string]interface{}{
		"component":   l.component,
		"log_counts":  l.logCounts,
		"file_size":   l.fileSize,
		"last_rotate": l.lastRotate,
	}
}

// Close closes the logger
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	if l.currentFile != nil {
		return l.currentFile.Close()
	}
	
	return nil
}

// LoggerWithFields provides a logger with preset fields
type LoggerWithFields struct {
	logger *Logger
	fields map[string]interface{}
}

// Debug logs a debug message with preset fields
func (lf *LoggerWithFields) Debug(message string, fields ...map[string]interface{}) {
	lf.logger.log(LogLevelDebug, message, lf.mergeFields(fields...))
}

// Info logs an info message with preset fields
func (lf *LoggerWithFields) Info(message string, fields ...map[string]interface{}) {
	lf.logger.log(LogLevelInfo, message, lf.mergeFields(fields...))
}

// Warn logs a warning message with preset fields
func (lf *LoggerWithFields) Warn(message string, fields ...map[string]interface{}) {
	lf.logger.log(LogLevelWarn, message, lf.mergeFields(fields...))
}

// Error logs an error message with preset fields
func (lf *LoggerWithFields) Error(message string, err error, fields ...map[string]interface{}) {
	f := lf.mergeFields(fields...)
	if err != nil {
		f["error"] = err.Error()
		f["error_stack"] = fmt.Sprintf("%+v", err)
	}
	lf.logger.log(LogLevelError, message, f)
}

// mergeFields merges preset fields with additional fields
func (lf *LoggerWithFields) mergeFields(fields ...map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	
	// Copy preset fields
	for k, v := range lf.fields {
		result[k] = v
	}
	
	// Merge additional fields
	for _, f := range fields {
		for k, v := range f {
			result[k] = v
		}
	}
	
	return result
}

// Helper functions

func mergeFields(fields ...map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	
	for _, f := range fields {
		for k, v := range f {
			result[k] = v
		}
	}
	
	return result
}

// Global logger instance
var (
	globalLogger *Logger
	globalMu     sync.Mutex
)

// InitGlobalLogger initializes the global logger
func InitGlobalLogger(component, logDir string) error {
	globalMu.Lock()
	defer globalMu.Unlock()
	
	logger, err := NewLogger(component, logDir)
	if err != nil {
		return err
	}
	
	globalLogger = logger
	return nil
}

// GetLogger returns the global logger
func GetLogger() *Logger {
	globalMu.Lock()
	defer globalMu.Unlock()
	
	if globalLogger == nil {
		// Create default logger
		globalLogger, _ = NewLogger("default", "./logs")
	}
	
	return globalLogger
}