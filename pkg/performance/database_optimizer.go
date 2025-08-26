package performance

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

// DatabaseOptimizer manages database performance optimizations
type DatabaseOptimizer struct {
	db         *sql.DB
	config     DatabaseConfig
	connPool   *ConnectionPool
	queryCache *QueryCache
	metrics    *DatabaseMetrics
	running    bool
	stopCh     chan struct{}
	mu         sync.RWMutex
}

type DatabaseConfig struct {
	// Connection settings
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	SSLMode         string
	
	// Connection pool settings
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	
	// Performance settings
	EnablePreparedStmts bool
	PreparedStmtCacheSize int
	EnableQueryCache    bool
	QueryCacheSize      int
	QueryTimeout        time.Duration
	
	// Optimization settings
	EnableBatching      bool
	BatchSize           int
	BatchTimeout        time.Duration
	EnableConnectionRetry bool
	MaxRetryAttempts    int
	RetryDelay          time.Duration
}

func DefaultDatabaseConfig() DatabaseConfig {
	return DatabaseConfig{
		Host:                  "localhost",
		Port:                  5432,
		User:                  "postgres",
		Password:              "",
		Database:              "mexoms",
		SSLMode:               "disable",
		MaxOpenConns:          25,
		MaxIdleConns:          5,
		ConnMaxLifetime:       5 * time.Minute,
		ConnMaxIdleTime:       1 * time.Minute,
		EnablePreparedStmts:   true,
		PreparedStmtCacheSize: 100,
		EnableQueryCache:      true,
		QueryCacheSize:        1000,
		QueryTimeout:          30 * time.Second,
		EnableBatching:        true,
		BatchSize:             100,
		BatchTimeout:          100 * time.Millisecond,
		EnableConnectionRetry: true,
		MaxRetryAttempts:      3,
		RetryDelay:            100 * time.Millisecond,
	}
}

func NewDatabaseOptimizer(config DatabaseConfig) (*DatabaseOptimizer, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.User, config.Password, config.Database, config.SSLMode)
	
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	optimizer := &DatabaseOptimizer{
		db:         db,
		config:     config,
		connPool:   NewConnectionPool(db),
		queryCache: NewQueryCache(config.QueryCacheSize),
		metrics:    NewDatabaseMetrics(),
		stopCh:     make(chan struct{}),
	}

	return optimizer, nil
}

func (d *DatabaseOptimizer) Start() {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return
	}
	d.running = true
	d.mu.Unlock()

	// Start metrics collection
	go d.collectMetrics()
	
	// Start connection health monitoring
	go d.monitorConnections()
}

func (d *DatabaseOptimizer) Stop() {
	d.mu.Lock()
	if !d.running {
		d.mu.Unlock()
		return
	}
	d.running = false
	d.mu.Unlock()

	close(d.stopCh)
	d.db.Close()
}

func (d *DatabaseOptimizer) collectMetrics() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			stats := d.db.Stats()
			d.metrics.UpdateConnectionStats(stats)
		case <-d.stopCh:
			return
		}
	}
}

func (d *DatabaseOptimizer) monitorConnections() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Test connection health
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := d.db.PingContext(ctx); err != nil {
				d.metrics.IncrementConnectionErrors()
			}
			cancel()
		case <-d.stopCh:
			return
		}
	}
}

// Optimized query execution with retry and metrics
func (d *DatabaseOptimizer) ExecuteQuery(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	start := time.Now()
	defer func() {
		d.metrics.RecordQueryLatency(time.Since(start))
	}()

	// Check query cache first
	if d.config.EnableQueryCache {
		if result := d.queryCache.Get(query, args...); result != nil {
			d.metrics.IncrementCacheHits()
			return result, nil
		}
		d.metrics.IncrementCacheMisses()
	}

	// Execute with retry
	return d.executeWithRetry(ctx, query, args...)
}

func (d *DatabaseOptimizer) executeWithRetry(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	var rows *sql.Rows
	var err error

	for attempt := 0; attempt <= d.config.MaxRetryAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(d.config.RetryDelay * time.Duration(attempt)):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		queryCtx, cancel := context.WithTimeout(ctx, d.config.QueryTimeout)
		rows, err = d.db.QueryContext(queryCtx, query, args...)
		cancel()

		if err == nil {
			d.metrics.IncrementSuccessfulQueries()
			
			// Cache successful query result
			if d.config.EnableQueryCache {
				d.queryCache.Set(query, rows, args...)
			}
			
			return rows, nil
		}

		d.metrics.IncrementFailedQueries()
		
		// Check if error is retryable
		if !d.isRetryableError(err) {
			break
		}
	}

	return nil, err
}

func (d *DatabaseOptimizer) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for PostgreSQL-specific errors
	if pqErr, ok := err.(*pq.Error); ok {
		switch pqErr.Code {
		case "53300", // too_many_connections
			 "53400", // configuration_limit_exceeded
			 "08000", // connection_exception
			 "08003", // connection_does_not_exist
			 "08006", // connection_failure
			 "08001", // sqlclient_unable_to_establish_sqlconnection
			 "08004": // sqlserver_rejected_establishment_of_sqlconnection
			return true
		}
	}

	return false
}

// Batch execution for improved performance
func (d *DatabaseOptimizer) ExecuteBatch(ctx context.Context, statements []BatchStatement) error {
	if !d.config.EnableBatching || len(statements) == 0 {
		// Execute individually
		for _, stmt := range statements {
			_, err := d.ExecuteQuery(ctx, stmt.Query, stmt.Args...)
			if err != nil {
				return err
			}
		}
		return nil
	}

	return d.executeBatch(ctx, statements)
}

func (d *DatabaseOptimizer) executeBatch(ctx context.Context, statements []BatchStatement) error {
	start := time.Now()
	defer func() {
		d.metrics.RecordBatchLatency(time.Since(start))
	}()

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare statements if enabled
	if d.config.EnablePreparedStmts {
		stmtCache := make(map[string]*sql.Stmt)
		defer func() {
			for _, stmt := range stmtCache {
				stmt.Close()
			}
		}()

		for _, statement := range statements {
			stmt, exists := stmtCache[statement.Query]
			if !exists {
				stmt, err = tx.PrepareContext(ctx, statement.Query)
				if err != nil {
					return fmt.Errorf("failed to prepare statement: %w", err)
				}
				stmtCache[statement.Query] = stmt
			}

			_, err = stmt.ExecContext(ctx, statement.Args...)
			if err != nil {
				return fmt.Errorf("failed to execute statement: %w", err)
			}
		}
	} else {
		// Execute without prepared statements
		for _, statement := range statements {
			_, err = tx.ExecContext(ctx, statement.Query, statement.Args...)
			if err != nil {
				return fmt.Errorf("failed to execute statement: %w", err)
			}
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	d.metrics.IncrementBatchExecutions(len(statements))
	return nil
}

type BatchStatement struct {
	Query string
	Args  []interface{}
}

// Connection Pool management
type ConnectionPool struct {
	db          *sql.DB
	connections chan *sql.Conn
	metrics     *PoolMetrics
	mu          sync.RWMutex
}

type PoolMetrics struct {
	ActiveConnections   int64
	IdleConnections     int64
	TotalConnections    int64
	ConnectionsCreated  int64
	ConnectionsClosed   int64
	ConnectionErrors    int64
}

func NewConnectionPool(db *sql.DB) *ConnectionPool {
	return &ConnectionPool{
		db:      db,
		metrics: &PoolMetrics{},
	}
}

func (p *ConnectionPool) GetConnection(ctx context.Context) (*sql.Conn, error) {
	start := time.Now()
	conn, err := p.db.Conn(ctx)
	if err != nil {
		atomic.AddInt64(&p.metrics.ConnectionErrors, 1)
		return nil, err
	}

	atomic.AddInt64(&p.metrics.ActiveConnections, 1)
	atomic.AddInt64(&p.metrics.ConnectionsCreated, 1)
	
	// Log slow connection acquisitions
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		// Log slow connection acquisition
	}

	return conn, nil
}

func (p *ConnectionPool) ReleaseConnection(conn *sql.Conn) {
	if conn != nil {
		conn.Close()
		atomic.AddInt64(&p.metrics.ActiveConnections, -1)
		atomic.AddInt64(&p.metrics.ConnectionsClosed, 1)
	}
}

func (p *ConnectionPool) GetMetrics() PoolMetrics {
	return PoolMetrics{
		ActiveConnections:   atomic.LoadInt64(&p.metrics.ActiveConnections),
		IdleConnections:     atomic.LoadInt64(&p.metrics.IdleConnections),
		TotalConnections:    atomic.LoadInt64(&p.metrics.TotalConnections),
		ConnectionsCreated:  atomic.LoadInt64(&p.metrics.ConnectionsCreated),
		ConnectionsClosed:   atomic.LoadInt64(&p.metrics.ConnectionsClosed),
		ConnectionErrors:    atomic.LoadInt64(&p.metrics.ConnectionErrors),
	}
}

// Query Cache for improved performance
type QueryCache struct {
	cache    map[string]*CacheEntry
	maxSize  int
	mu       sync.RWMutex
	hits     int64
	misses   int64
}

type CacheEntry struct {
	Result    *sql.Rows
	Args      []interface{}
	CreatedAt time.Time
	TTL       time.Duration
}

func NewQueryCache(maxSize int) *QueryCache {
	return &QueryCache{
		cache:   make(map[string]*CacheEntry),
		maxSize: maxSize,
	}
}

func (q *QueryCache) Get(query string, args ...interface{}) *sql.Rows {
	q.mu.RLock()
	entry, exists := q.cache[query]
	q.mu.RUnlock()

	if !exists {
		return nil
	}

	// Check TTL
	if time.Since(entry.CreatedAt) > entry.TTL {
		q.mu.Lock()
		delete(q.cache, query)
		q.mu.Unlock()
		return nil
	}

	// Verify args match (simple comparison)
	if !q.argsEqual(entry.Args, args) {
		return nil
	}

	return entry.Result
}

func (q *QueryCache) Set(query string, result *sql.Rows, args ...interface{}) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Evict oldest entries if cache is full
	if len(q.cache) >= q.maxSize {
		q.evictOldest()
	}

	q.cache[query] = &CacheEntry{
		Result:    result,
		Args:      make([]interface{}, len(args)),
		CreatedAt: time.Now(),
		TTL:       5 * time.Minute, // Default TTL
	}
	copy(q.cache[query].Args, args)
}

func (q *QueryCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range q.cache {
		if oldestKey == "" || entry.CreatedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.CreatedAt
		}
	}

	if oldestKey != "" {
		delete(q.cache, oldestKey)
	}
}

func (q *QueryCache) argsEqual(a, b []interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

func (q *QueryCache) GetStats() (hits, misses int64) {
	return atomic.LoadInt64(&q.hits), atomic.LoadInt64(&q.misses)
}

// Database Metrics collection
type DatabaseMetrics struct {
	QueryCount       int64
	QueryLatencySum  int64
	QueryLatencyMax  int64
	SuccessfulQueries int64
	FailedQueries     int64
	CacheHits         int64
	CacheMisses       int64
	BatchExecutions   int64
	BatchLatencySum   int64
	ConnectionErrors  int64
	
	// Connection stats
	OpenConnections  int64
	InUseConnections int64
	IdleConnections  int64
	WaitCount        int64
	WaitDuration     time.Duration
	MaxIdleClosed    int64
	MaxLifetimeClosed int64
}

func NewDatabaseMetrics() *DatabaseMetrics {
	return &DatabaseMetrics{}
}

func (m *DatabaseMetrics) RecordQueryLatency(latency time.Duration) {
	atomic.AddInt64(&m.QueryCount, 1)
	atomic.AddInt64(&m.QueryLatencySum, int64(latency))
	
	// Update max latency
	for {
		current := atomic.LoadInt64(&m.QueryLatencyMax)
		if int64(latency) <= current {
			break
		}
		if atomic.CompareAndSwapInt64(&m.QueryLatencyMax, current, int64(latency)) {
			break
		}
	}
}

func (m *DatabaseMetrics) RecordBatchLatency(latency time.Duration) {
	atomic.AddInt64(&m.BatchLatencySum, int64(latency))
}

func (m *DatabaseMetrics) IncrementSuccessfulQueries() {
	atomic.AddInt64(&m.SuccessfulQueries, 1)
}

func (m *DatabaseMetrics) IncrementFailedQueries() {
	atomic.AddInt64(&m.FailedQueries, 1)
}

func (m *DatabaseMetrics) IncrementCacheHits() {
	atomic.AddInt64(&m.CacheHits, 1)
}

func (m *DatabaseMetrics) IncrementCacheMisses() {
	atomic.AddInt64(&m.CacheMisses, 1)
}

func (m *DatabaseMetrics) IncrementBatchExecutions(count int) {
	atomic.AddInt64(&m.BatchExecutions, int64(count))
}

func (m *DatabaseMetrics) IncrementConnectionErrors() {
	atomic.AddInt64(&m.ConnectionErrors, 1)
}

func (m *DatabaseMetrics) UpdateConnectionStats(stats sql.DBStats) {
	atomic.StoreInt64(&m.OpenConnections, int64(stats.OpenConnections))
	atomic.StoreInt64(&m.InUseConnections, int64(stats.InUse))
	atomic.StoreInt64(&m.IdleConnections, int64(stats.Idle))
	atomic.StoreInt64(&m.WaitCount, stats.WaitCount)
	atomic.StoreInt64(&m.MaxIdleClosed, stats.MaxIdleClosed)
	atomic.StoreInt64(&m.MaxLifetimeClosed, stats.MaxLifetimeClosed)
}

func (m *DatabaseMetrics) GetAverageQueryLatency() time.Duration {
	count := atomic.LoadInt64(&m.QueryCount)
	if count == 0 {
		return 0
	}
	sum := atomic.LoadInt64(&m.QueryLatencySum)
	return time.Duration(sum / count)
}

func (m *DatabaseMetrics) GetCacheHitRatio() float64 {
	hits := atomic.LoadInt64(&m.CacheHits)
	misses := atomic.LoadInt64(&m.CacheMisses)
	total := hits + misses
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total)
}

func (m *DatabaseMetrics) GetSuccessRate() float64 {
	success := atomic.LoadInt64(&m.SuccessfulQueries)
	failed := atomic.LoadInt64(&m.FailedQueries)
	total := success + failed
	if total == 0 {
		return 1.0
	}
	return float64(success) / float64(total)
}

// Database optimizations helper
func (d *DatabaseOptimizer) OptimizeQueries() error {
	// Common database optimizations
	optimizations := []string{
		"ANALYZE;", // Update statistics
		"VACUUM;",  // Reclaim storage space
	}

	for _, query := range optimizations {
		if err := d.executeOptimization(query); err != nil {
			return fmt.Errorf("failed to execute optimization %s: %w", query, err)
		}
	}

	return nil
}

func (d *DatabaseOptimizer) executeOptimization(query string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	
	_, err := d.db.ExecContext(ctx, query)
	return err
}

func (d *DatabaseOptimizer) GetMetrics() *DatabaseMetrics {
	return d.metrics
}