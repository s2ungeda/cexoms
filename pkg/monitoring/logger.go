package monitoring

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Logger is an account-aware structured logger
type Logger struct {
	baseLogger  *zap.Logger
	accountLogs sync.Map // accountID -> *zap.Logger
	config      *LogConfig
}

// LogConfig holds logger configuration
type LogConfig struct {
	// Base log directory
	BaseDir string
	
	// Log levels
	Level      string // "debug", "info", "warn", "error"
	JSONFormat bool
	
	// File rotation
	MaxSize    int // MB
	MaxBackups int
	MaxAge     int // days
	Compress   bool
	
	// Performance
	BufferSize int
	
	// Account separation
	SeparateAccountLogs bool
}

// DefaultLogConfig returns default configuration
func DefaultLogConfig() *LogConfig {
	return &LogConfig{
		BaseDir:             "/var/log/mexoms",
		Level:               "info",
		JSONFormat:          true,
		MaxSize:             100,
		MaxBackups:          30,
		MaxAge:              30,
		Compress:            true,
		BufferSize:          256,
		SeparateAccountLogs: true,
	}
}

// NewLogger creates a new monitoring logger
func NewLogger(config *LogConfig) (*Logger, error) {
	if config == nil {
		config = DefaultLogConfig()
	}
	
	// Ensure base directory exists
	if err := os.MkdirAll(config.BaseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}
	
	// Create base logger
	baseLogger, err := createZapLogger(filepath.Join(config.BaseDir, "system.log"), config)
	if err != nil {
		return nil, err
	}
	
	return &Logger{
		baseLogger: baseLogger,
		config:     config,
	}, nil
}

// GetAccountLogger returns a logger for a specific account
func (l *Logger) GetAccountLogger(accountID string) *zap.Logger {
	if !l.config.SeparateAccountLogs {
		return l.baseLogger.With(zap.String("account_id", accountID))
	}
	
	// Check if logger already exists
	if logger, ok := l.accountLogs.Load(accountID); ok {
		return logger.(*zap.Logger)
	}
	
	// Create new account logger
	logPath := filepath.Join(l.config.BaseDir, "accounts", accountID+".log")
	accountLogger, err := createZapLogger(logPath, l.config)
	if err != nil {
		// Fallback to base logger
		l.baseLogger.Error("Failed to create account logger", 
			zap.String("account_id", accountID),
			zap.Error(err))
		return l.baseLogger.With(zap.String("account_id", accountID))
	}
	
	// Store and return
	actual, _ := l.accountLogs.LoadOrStore(accountID, accountLogger)
	return actual.(*zap.Logger)
}

// GetStrategyLogger returns a logger for a specific strategy
func (l *Logger) GetStrategyLogger(strategyID string) *zap.Logger {
	if !l.config.SeparateAccountLogs {
		return l.baseLogger.With(zap.String("strategy_id", strategyID))
	}
	
	// Create strategy logger
	logPath := filepath.Join(l.config.BaseDir, "strategies", strategyID+".log")
	strategyLogger, err := createZapLogger(logPath, l.config)
	if err != nil {
		return l.baseLogger.With(zap.String("strategy_id", strategyID))
	}
	
	return strategyLogger
}

// LogOrder logs order events
func (l *Logger) LogOrder(accountID, orderID string, event string, fields ...zap.Field) {
	logger := l.GetAccountLogger(accountID)
	
	allFields := append([]zap.Field{
		zap.String("order_id", orderID),
		zap.String("event", event),
		zap.Time("timestamp", time.Now()),
	}, fields...)
	
	logger.Info("Order event", allFields...)
}

// LogPosition logs position events
func (l *Logger) LogPosition(accountID, symbol string, event string, fields ...zap.Field) {
	logger := l.GetAccountLogger(accountID)
	
	allFields := append([]zap.Field{
		zap.String("symbol", symbol),
		zap.String("event", event),
		zap.Time("timestamp", time.Now()),
	}, fields...)
	
	logger.Info("Position event", allFields...)
}

// LogTrade logs trade execution
func (l *Logger) LogTrade(accountID string, trade TradeLog) {
	logger := l.GetAccountLogger(accountID)
	
	logger.Info("Trade executed",
		zap.String("symbol", trade.Symbol),
		zap.String("side", trade.Side),
		zap.Float64("quantity", trade.Quantity),
		zap.Float64("price", trade.Price),
		zap.Float64("fee", trade.Fee),
		zap.String("order_id", trade.OrderID),
		zap.Time("timestamp", trade.Timestamp),
	)
}

// LogError logs error events
func (l *Logger) LogError(accountID string, component string, err error, fields ...zap.Field) {
	logger := l.GetAccountLogger(accountID)
	
	allFields := append([]zap.Field{
		zap.String("component", component),
		zap.Error(err),
		zap.Time("timestamp", time.Now()),
	}, fields...)
	
	logger.Error("Error occurred", allFields...)
}

// LogMetric logs performance metrics
func (l *Logger) LogMetric(metric Metric) {
	logger := l.baseLogger
	if metric.AccountID != "" {
		logger = l.GetAccountLogger(metric.AccountID)
	}
	
	logger.Info("Metric",
		zap.String("name", metric.Name),
		zap.Float64("value", metric.Value),
		zap.String("unit", metric.Unit),
		zap.Any("tags", metric.Tags),
		zap.Time("timestamp", metric.Timestamp),
	)
}

// LogStrategyEvent logs strategy-specific events
func (l *Logger) LogStrategyEvent(strategyID string, event StrategyEvent) {
	logger := l.GetStrategyLogger(strategyID)
	
	logger.Info("Strategy event",
		zap.String("type", event.Type),
		zap.String("status", event.Status),
		zap.Float64("pnl", event.PnL),
		zap.Any("metrics", event.Metrics),
		zap.Time("timestamp", event.Timestamp),
	)
}

// Sync flushes all logs
func (l *Logger) Sync() error {
	// Sync base logger
	if err := l.baseLogger.Sync(); err != nil {
		return err
	}
	
	// Sync all account loggers
	l.accountLogs.Range(func(key, value interface{}) bool {
		if logger, ok := value.(*zap.Logger); ok {
			logger.Sync()
		}
		return true
	})
	
	return nil
}

// createZapLogger creates a zap logger with the given configuration
func createZapLogger(logPath string, config *LogConfig) (*zap.Logger, error) {
	// Ensure directory exists
	logDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}
	
	// Create file rotator
	fileRotator := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    config.MaxSize,
		MaxBackups: config.MaxBackups,
		MaxAge:     config.MaxAge,
		Compress:   config.Compress,
	}
	
	// Create encoder config
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	
	// Create encoder
	var encoder zapcore.Encoder
	if config.JSONFormat {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}
	
	// Parse log level
	level, err := zapcore.ParseLevel(config.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}
	
	// Create core with file and console output
	fileCore := zapcore.NewCore(encoder, zapcore.AddSync(fileRotator), level)
	consoleCore := zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level)
	
	// Combine cores
	core := zapcore.NewTee(fileCore, consoleCore)
	
	// Create logger with buffer
	logger := zap.New(core, 
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
		zap.WrapCore(func(c zapcore.Core) zapcore.Core {
			return &BufferedCore{
				Core:       c,
				bufferSize: config.BufferSize,
			}
		}),
	)
	
	return logger, nil
}

// BufferedCore implements buffered logging for better performance
type BufferedCore struct {
	zapcore.Core
	mu         sync.Mutex
	buffer     []zapcore.Entry
	bufferSize int
}

func (b *BufferedCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	// Add to buffer
	b.buffer = append(b.buffer, entry)
	
	// Flush if buffer is full
	if len(b.buffer) >= b.bufferSize {
		return b.flush()
	}
	
	return nil
}

func (b *BufferedCore) flush() error {
	for _, entry := range b.buffer {
		if err := b.Core.Write(entry, nil); err != nil {
			return err
		}
	}
	b.buffer = b.buffer[:0]
	return nil
}

func (b *BufferedCore) Sync() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	if err := b.flush(); err != nil {
		return err
	}
	
	return b.Core.Sync()
}

// Log structures

type TradeLog struct {
	Symbol    string
	Side      string
	Quantity  float64
	Price     float64
	Fee       float64
	OrderID   string
	Timestamp time.Time
}

type Metric struct {
	Name      string
	Value     float64
	Unit      string
	Tags      map[string]string
	AccountID string
	Timestamp time.Time
}

type StrategyEvent struct {
	Type      string
	Status    string
	PnL       float64
	Metrics   map[string]float64
	Timestamp time.Time
}

// RotateLogs forces rotation of all log files
func (l *Logger) RotateLogs() error {
	// This would trigger log rotation
	// Implementation depends on the rotation library used
	return l.Sync()
}

// GetWriter returns an io.Writer for the specified account
func (l *Logger) GetWriter(accountID string) io.Writer {
	logger := l.GetAccountLogger(accountID)
	return &zapWriter{logger: logger}
}

type zapWriter struct {
	logger *zap.Logger
}

func (w *zapWriter) Write(p []byte) (n int, err error) {
	w.logger.Info(string(p))
	return len(p), nil
}