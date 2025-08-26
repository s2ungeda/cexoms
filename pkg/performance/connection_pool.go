package performance

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
	_ "github.com/lib/pq"
)

// ConnectionPool manages database and Redis connections with health checks
type ConnectionPool struct {
	// Database connection pool
	dbPool     *sql.DB
	dbConfig   DatabaseConfig
	dbHealthy  atomic.Bool
	dbMu       sync.RWMutex

	// Redis connection pool
	redisPool    *redis.Client
	redisConfig  RedisConfig
	redisHealthy atomic.Bool
	redisMu      sync.RWMutex

	// Health check configuration
	healthCheckInterval time.Duration
	healthCheckTimeout  time.Duration

	// Metrics
	dbConnActive     atomic.Int64
	dbConnIdle       atomic.Int64
	redisConnActive  atomic.Int64
	redisConnIdle    atomic.Int64
	healthCheckCount atomic.Int64
	failureCount     atomic.Int64

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// DatabaseConfig holds database connection configuration
type DatabaseConfig struct {
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	Host         string
	Port         int
	Password     string
	DB           int
	PoolSize     int
	MinIdleConns int
	MaxRetries   int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	PoolTimeout  time.Duration
	IdleTimeout  time.Duration
}

// PoolMetrics contains connection pool metrics
type PoolMetrics struct {
	DBConnActive     int64
	DBConnIdle       int64
	RedisConnActive  int64
	RedisConnIdle    int64
	HealthCheckCount int64
	FailureCount     int64
	DBHealthy        bool
	RedisHealthy     bool
}

// NewConnectionPool creates a new connection pool with health checks
func NewConnectionPool(dbConfig DatabaseConfig, redisConfig RedisConfig) (*ConnectionPool, error) {
	ctx, cancel := context.WithCancel(context.Background())
	
	pool := &ConnectionPool{
		dbConfig:            dbConfig,
		redisConfig:         redisConfig,
		healthCheckInterval: 10 * time.Second,
		healthCheckTimeout:  3 * time.Second,
		ctx:                 ctx,
		cancel:              cancel,
	}

	// Initialize database connection pool
	if err := pool.initDatabase(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Initialize Redis connection pool
	if err := pool.initRedis(); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to initialize Redis: %w", err)
	}

	// Start health check routine
	pool.wg.Add(1)
	go pool.healthCheckLoop()

	return pool, nil
}

// initDatabase initializes the database connection pool
func (p *ConnectionPool) initDatabase() error {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		p.dbConfig.Host, p.dbConfig.Port, p.dbConfig.User, p.dbConfig.Password, p.dbConfig.Database)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
	}

	// Configure connection pool
	db.SetMaxOpenConns(p.dbConfig.MaxOpenConns)
	db.SetMaxIdleConns(p.dbConfig.MaxIdleConns)
	db.SetConnMaxLifetime(p.dbConfig.ConnMaxLifetime)
	db.SetConnMaxIdleTime(p.dbConfig.ConnMaxIdleTime)

	// Test connection
	ctx, cancel := context.WithTimeout(p.ctx, p.healthCheckTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return err
	}

	p.dbPool = db
	p.dbHealthy.Store(true)
	return nil
}

// initRedis initializes the Redis connection pool
func (p *ConnectionPool) initRedis() error {
	opt := &redis.Options{
		Addr:         fmt.Sprintf("%s:%d", p.redisConfig.Host, p.redisConfig.Port),
		Password:     p.redisConfig.Password,
		DB:           p.redisConfig.DB,
		PoolSize:     p.redisConfig.PoolSize,
		MinIdleConns: p.redisConfig.MinIdleConns,
		MaxRetries:   p.redisConfig.MaxRetries,
		DialTimeout:  p.redisConfig.DialTimeout,
		ReadTimeout:  p.redisConfig.ReadTimeout,
		WriteTimeout: p.redisConfig.WriteTimeout,
		PoolTimeout:  p.redisConfig.PoolTimeout,
		IdleTimeout:  p.redisConfig.IdleTimeout,
	}

	client := redis.NewClient(opt)

	// Test connection
	ctx, cancel := context.WithTimeout(p.ctx, p.healthCheckTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return err
	}

	p.redisPool = client
	p.redisHealthy.Store(true)
	return nil
}

// GetDB returns a database connection from the pool
func (p *ConnectionPool) GetDB() (*sql.DB, error) {
	if !p.dbHealthy.Load() {
		return nil, fmt.Errorf("database connection pool is unhealthy")
	}

	p.dbMu.RLock()
	defer p.dbMu.RUnlock()

	if p.dbPool == nil {
		return nil, fmt.Errorf("database connection pool is not initialized")
	}

	// Update metrics
	stats := p.dbPool.Stats()
	p.dbConnActive.Store(int64(stats.InUse))
	p.dbConnIdle.Store(int64(stats.Idle))

	return p.dbPool, nil
}

// GetRedis returns a Redis client from the pool
func (p *ConnectionPool) GetRedis() (*redis.Client, error) {
	if !p.redisHealthy.Load() {
		return nil, fmt.Errorf("Redis connection pool is unhealthy")
	}

	p.redisMu.RLock()
	defer p.redisMu.RUnlock()

	if p.redisPool == nil {
		return nil, fmt.Errorf("Redis connection pool is not initialized")
	}

	// Update metrics
	stats := p.redisPool.PoolStats()
	p.redisConnActive.Store(int64(stats.Hits + stats.Misses))
	p.redisConnIdle.Store(int64(stats.IdleConns))

	return p.redisPool, nil
}

// healthCheckLoop performs periodic health checks on connections
func (p *ConnectionPool) healthCheckLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.performHealthCheck()
		}
	}
}

// performHealthCheck checks the health of all connections
func (p *ConnectionPool) performHealthCheck() {
	p.healthCheckCount.Add(1)

	// Check database health
	if p.dbPool != nil {
		ctx, cancel := context.WithTimeout(p.ctx, p.healthCheckTimeout)
		err := p.dbPool.PingContext(ctx)
		cancel()

		if err != nil {
			p.dbHealthy.Store(false)
			p.failureCount.Add(1)
			p.attemptDBReconnect()
		} else {
			p.dbHealthy.Store(true)
		}
	}

	// Check Redis health
	if p.redisPool != nil {
		ctx, cancel := context.WithTimeout(p.ctx, p.healthCheckTimeout)
		err := p.redisPool.Ping(ctx).Err()
		cancel()

		if err != nil {
			p.redisHealthy.Store(false)
			p.failureCount.Add(1)
			p.attemptRedisReconnect()
		} else {
			p.redisHealthy.Store(true)
		}
	}
}

// attemptDBReconnect attempts to reconnect to the database
func (p *ConnectionPool) attemptDBReconnect() {
	p.dbMu.Lock()
	defer p.dbMu.Unlock()

	// Close existing connection
	if p.dbPool != nil {
		p.dbPool.Close()
		p.dbPool = nil
	}

	// Attempt to reconnect
	if err := p.initDatabase(); err != nil {
		// Log error (implement proper logging)
		fmt.Printf("Failed to reconnect to database: %v\n", err)
	}
}

// attemptRedisReconnect attempts to reconnect to Redis
func (p *ConnectionPool) attemptRedisReconnect() {
	p.redisMu.Lock()
	defer p.redisMu.Unlock()

	// Close existing connection
	if p.redisPool != nil {
		p.redisPool.Close()
		p.redisPool = nil
	}

	// Attempt to reconnect
	if err := p.initRedis(); err != nil {
		// Log error (implement proper logging)
		fmt.Printf("Failed to reconnect to Redis: %v\n", err)
	}
}

// GetMetrics returns current pool metrics
func (p *ConnectionPool) GetMetrics() PoolMetrics {
	return PoolMetrics{
		DBConnActive:     p.dbConnActive.Load(),
		DBConnIdle:       p.dbConnIdle.Load(),
		RedisConnActive:  p.redisConnActive.Load(),
		RedisConnIdle:    p.redisConnIdle.Load(),
		HealthCheckCount: p.healthCheckCount.Load(),
		FailureCount:     p.failureCount.Load(),
		DBHealthy:        p.dbHealthy.Load(),
		RedisHealthy:     p.redisHealthy.Load(),
	}
}

// ExecuteWithRetry executes a database operation with retry logic
func (p *ConnectionPool) ExecuteWithRetry(ctx context.Context, fn func(*sql.DB) error, maxRetries int) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		db, poolErr := p.GetDB()
		if poolErr != nil {
			err = poolErr
			time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
			continue
		}

		if fnErr := fn(db); fnErr != nil {
			err = fnErr
			// Check if error is retryable
			if isRetryableError(fnErr) {
				time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
				continue
			}
			return fnErr
		}

		return nil
	}
	return fmt.Errorf("max retries exceeded: %w", err)
}

// ExecuteRedisWithRetry executes a Redis operation with retry logic
func (p *ConnectionPool) ExecuteRedisWithRetry(ctx context.Context, fn func(*redis.Client) error, maxRetries int) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		client, poolErr := p.GetRedis()
		if poolErr != nil {
			err = poolErr
			time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
			continue
		}

		if fnErr := fn(client); fnErr != nil {
			err = fnErr
			// Check if error is retryable
			if isRetryableError(fnErr) {
				time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
				continue
			}
			return fnErr
		}

		return nil
	}
	return fmt.Errorf("max retries exceeded: %w", err)
}

// isRetryableError determines if an error is retryable
func isRetryableError(err error) bool {
	// Add logic to determine retryable errors
	// For now, return true for all errors
	return true
}

// Close closes all connections and stops health checks
func (p *ConnectionPool) Close() error {
	p.cancel()
	p.wg.Wait()

	var errs []error

	// Close database connections
	p.dbMu.Lock()
	if p.dbPool != nil {
		if err := p.dbPool.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close database: %w", err))
		}
		p.dbPool = nil
	}
	p.dbMu.Unlock()

	// Close Redis connections
	p.redisMu.Lock()
	if p.redisPool != nil {
		if err := p.redisPool.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close Redis: %w", err))
		}
		p.redisPool = nil
	}
	p.redisMu.Unlock()

	if len(errs) > 0 {
		return fmt.Errorf("errors closing connection pool: %v", errs)
	}

	return nil
}