# Database Architecture

PostgreSQL schema design, optimization strategies, and data management patterns.

## Overview

The database layer uses PostgreSQL for persistent storage with optimizations for high-throughput trading operations.

## Schema Design Principles

### 1. Performance Optimization
- Partitioned tables for time-series data
- Efficient indexing strategies
- Denormalization for read performance
- Materialized views for analytics

### 2. Data Integrity
- Foreign key constraints
- Check constraints for data validation
- Transaction isolation levels
- Audit trail implementation

### 3. Scalability
- Horizontal partitioning
- Read replica distribution
- Connection pooling
- Query optimization

### 4. High Availability
- Streaming replication
- Automatic failover
- Point-in-time recovery
- Continuous backups

## Database Architecture

### PostgreSQL Cluster Configuration

```yaml
# Primary-Replica Setup
postgresql:
  primary:
    host: pg-primary.mexoms.internal
    port: 5432
    max_connections: 500
    shared_buffers: 8GB
    effective_cache_size: 24GB
    work_mem: 128MB
    
  replicas:
    - host: pg-replica-1.mexoms.internal
      port: 5432
      mode: streaming
      lag_threshold: 1MB
      
    - host: pg-replica-2.mexoms.internal  
      port: 5432
      mode: streaming
      lag_threshold: 1MB
      
  pgbouncer:
    pool_mode: transaction
    max_client_conn: 2000
    default_pool_size: 25
    reserve_pool_size: 5
```

### Connection Pooling

```go
// PgBouncer configuration for connection pooling
type DatabaseConfig struct {
    // Connection pools
    WriterPool *PoolConfig `yaml:"writer_pool"`
    ReaderPool *PoolConfig `yaml:"reader_pool"`
    
    // Pool configuration
    PoolConfig struct {
        MaxConnections    int           `yaml:"max_connections"`
        MinConnections    int           `yaml:"min_connections"`
        MaxIdleTime       time.Duration `yaml:"max_idle_time"`
        ConnectionTimeout time.Duration `yaml:"connection_timeout"`
        HealthCheckPeriod time.Duration `yaml:"health_check_period"`
    }
}

// Example configuration
config := &DatabaseConfig{
    WriterPool: &PoolConfig{
        MaxConnections:    50,
        MinConnections:    10,
        MaxIdleTime:       5 * time.Minute,
        ConnectionTimeout: 30 * time.Second,
        HealthCheckPeriod: 10 * time.Second,
    },
    ReaderPool: &PoolConfig{
        MaxConnections:    200,
        MinConnections:    20,
        MaxIdleTime:       10 * time.Minute,
        ConnectionTimeout: 10 * time.Second,
        HealthCheckPeriod: 30 * time.Second,
    },
}
```

## Partitioning Strategy

### Time-Series Data Partitioning

```sql
-- Create partitioned table for trades
CREATE TABLE trades (
    id BIGSERIAL,
    exchange VARCHAR(20) NOT NULL,
    symbol VARCHAR(20) NOT NULL,
    price DECIMAL(20,8) NOT NULL,
    quantity DECIMAL(20,8) NOT NULL,
    traded_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
) PARTITION BY RANGE (traded_at);

-- Create monthly partitions automatically
CREATE OR REPLACE FUNCTION create_monthly_partitions()
RETURNS void AS $$
DECLARE
    start_date date;
    end_date date;
    partition_name text;
BEGIN
    -- Create partitions for next 3 months
    FOR i IN 0..2 LOOP
        start_date := date_trunc('month', CURRENT_DATE + (i || ' months')::interval);
        end_date := start_date + '1 month'::interval;
        partition_name := 'trades_' || to_char(start_date, 'YYYY_MM');
        
        -- Check if partition exists
        IF NOT EXISTS (
            SELECT 1 FROM pg_class WHERE relname = partition_name
        ) THEN
            EXECUTE format(
                'CREATE TABLE %I PARTITION OF trades FOR VALUES FROM (%L) TO (%L)',
                partition_name, start_date, end_date
            );
            
            -- Create indexes on partition
            EXECUTE format(
                'CREATE INDEX %I ON %I (symbol, traded_at DESC)',
                partition_name || '_symbol_idx', partition_name
            );
        END IF;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- Schedule partition creation
CREATE EXTENSION IF NOT EXISTS pg_cron;
SELECT cron.schedule('create-partitions', '0 0 1 * *', 
    'SELECT create_monthly_partitions()');
```

### Partition Maintenance

```sql
-- Drop old partitions
CREATE OR REPLACE FUNCTION drop_old_partitions()
RETURNS void AS $$
DECLARE
    partition_name text;
    retention_months int := 12; -- Keep 12 months of data
BEGIN
    FOR partition_name IN 
        SELECT tablename FROM pg_tables 
        WHERE schemaname = 'public' 
        AND tablename LIKE 'trades_%'
        AND to_date(substring(tablename from 8), 'YYYY_MM') < 
            date_trunc('month', CURRENT_DATE - (retention_months || ' months')::interval)
    LOOP
        EXECUTE format('DROP TABLE %I', partition_name);
        RAISE NOTICE 'Dropped partition: %', partition_name;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- Archive partitions before dropping
CREATE OR REPLACE FUNCTION archive_partition(partition_name text)
RETURNS void AS $$
BEGIN
    -- Copy to archive storage
    EXECUTE format(
        'COPY %I TO %L WITH (FORMAT CSV, HEADER)',
        partition_name,
        '/archive/' || partition_name || '.csv'
    );
    
    -- Compress archive
    PERFORM pg_background_launch(
        format('gzip /archive/%s.csv', partition_name)
    );
END;
$$ LANGUAGE plpgsql;
```

## Query Optimization

### Index Strategy

```sql
-- B-tree indexes for exact lookups
CREATE INDEX idx_orders_account_status ON orders(account_id, status)
    WHERE status IN ('NEW', 'PARTIALLY_FILLED');

-- Composite indexes for common queries
CREATE INDEX idx_orders_symbol_time ON orders(symbol, created_at DESC);

-- Partial indexes for filtered queries
CREATE INDEX idx_orders_active ON orders(account_id, created_at DESC)
    WHERE status NOT IN ('FILLED', 'CANCELLED', 'REJECTED');

-- BRIN indexes for time-series data
CREATE INDEX idx_trades_traded_at_brin ON trades USING BRIN(traded_at);

-- GIN indexes for JSONB columns
CREATE INDEX idx_orders_metadata ON orders USING GIN(metadata);

-- Expression indexes
CREATE INDEX idx_positions_total_value ON positions((quantity * mark_price));
```

### Query Performance Analysis

```sql
-- Enable query performance insights
ALTER SYSTEM SET shared_preload_libraries = 'pg_stat_statements';
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- Find slow queries
SELECT 
    query,
    calls,
    total_time / 1000 as total_seconds,
    mean_time / 1000 as mean_seconds,
    stddev_time / 1000 as stddev_seconds,
    rows
FROM pg_stat_statements
WHERE query NOT LIKE '%pg_stat_statements%'
ORDER BY mean_time DESC
LIMIT 20;

-- Analyze query plan
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) 
SELECT o.*, e.price, e.quantity
FROM orders o
JOIN executions e ON o.id = e.order_id
WHERE o.account_id = '123e4567-e89b-12d3-a456-426614174000'
  AND o.created_at >= CURRENT_DATE - INTERVAL '7 days';
```

## Data Optimization

### Table Storage Optimization

```sql
-- Use appropriate data types
ALTER TABLE orders 
    ALTER COLUMN symbol TYPE VARCHAR(20),  -- Don't use TEXT for fixed-size data
    ALTER COLUMN side TYPE CHAR(4),        -- Fixed size for enums
    ALTER COLUMN status SET STORAGE PLAIN;  -- No compression for frequently accessed

-- Table storage parameters
ALTER TABLE trades SET (
    fillfactor = 90,              -- Leave space for updates
    autovacuum_vacuum_scale_factor = 0.01,  -- Aggressive vacuuming
    autovacuum_analyze_scale_factor = 0.005
);

-- Column statistics
ALTER TABLE orders ALTER COLUMN symbol SET STATISTICS 1000;
ALTER TABLE orders ALTER COLUMN status SET STATISTICS 100;
```

### Materialized Views

```sql
-- Account performance summary
CREATE MATERIALIZED VIEW account_performance AS
WITH daily_pnl AS (
    SELECT 
        account_id,
        DATE(created_at) as trading_date,
        SUM(realized_pnl) as daily_realized,
        SUM(commission) as daily_commission
    FROM positions
    WHERE closed_at IS NOT NULL
    GROUP BY account_id, DATE(created_at)
)
SELECT 
    a.id as account_id,
    a.account_name,
    COUNT(DISTINCT dp.trading_date) as trading_days,
    SUM(dp.daily_realized) as total_pnl,
    AVG(dp.daily_realized) as avg_daily_pnl,
    STDDEV(dp.daily_realized) as pnl_volatility,
    SUM(dp.daily_commission) as total_commission
FROM accounts a
LEFT JOIN daily_pnl dp ON a.id = dp.account_id
GROUP BY a.id, a.account_name;

-- Refresh strategy
CREATE OR REPLACE FUNCTION refresh_materialized_views()
RETURNS void AS $$
BEGIN
    REFRESH MATERIALIZED VIEW CONCURRENTLY account_performance;
    REFRESH MATERIALIZED VIEW CONCURRENTLY symbol_statistics;
    REFRESH MATERIALIZED VIEW CONCURRENTLY hourly_volume;
END;
$$ LANGUAGE plpgsql;

-- Schedule refresh
SELECT cron.schedule('refresh-views', '*/5 * * * *', 
    'SELECT refresh_materialized_views()');
```

## High Availability Setup

### Streaming Replication

```bash
# Primary server postgresql.conf
wal_level = replica
max_wal_senders = 10
wal_keep_segments = 64
archive_mode = on
archive_command = 'rsync -a %p backup@archive-server:/pgarchive/%f'

# Standby server recovery.conf
standby_mode = on
primary_conninfo = 'host=pg-primary port=5432 user=replicator'
restore_command = 'rsync -a backup@archive-server:/pgarchive/%f %p'
trigger_file = '/tmp/postgresql.trigger'
```

### Automatic Failover with Patroni

```yaml
# Patroni configuration
scope: mexoms-db
namespace: /service/
name: pg-node-1

restapi:
  listen: 0.0.0.0:8008
  connect_address: pg-node-1:8008

etcd:
  hosts:
    - etcd-1:2379
    - etcd-2:2379
    - etcd-3:2379

bootstrap:
  dcs:
    ttl: 30
    loop_wait: 10
    retry_timeout: 10
    maximum_lag_on_failover: 1048576
    
  postgresql:
    use_pg_rewind: true
    parameters:
      max_connections: 500
      shared_buffers: 8GB
      effective_cache_size: 24GB
      
postgresql:
  listen: 0.0.0.0:5432
  connect_address: pg-node-1:5432
  
  authentication:
    replication:
      username: replicator
      password: replicator_password
    superuser:
      username: postgres
      password: postgres_password
```

## Backup Strategy

### Continuous Archiving

```bash
#!/bin/bash
# Continuous backup script

# Base backup
pg_basebackup -D /backup/base -Ft -z -P \
  -h pg-primary -U replicator

# WAL archiving configuration
archive_mode = on
archive_command = 'test ! -f /archive/%f && cp %p /archive/%f'

# Point-in-time recovery
pg_ctl stop -D /var/lib/postgresql/data
rm -rf /var/lib/postgresql/data/*
tar -xzf /backup/base/base.tar.gz -C /var/lib/postgresql/data/

# Create recovery.conf
cat > /var/lib/postgresql/data/recovery.conf <<EOF
restore_command = 'cp /archive/%f %p'
recovery_target_time = '2024-01-15 14:30:00'
recovery_target_action = 'promote'
EOF

pg_ctl start -D /var/lib/postgresql/data
```

### Backup Validation

```sql
-- Backup validation procedure
CREATE OR REPLACE FUNCTION validate_backup(backup_date date)
RETURNS TABLE(
    check_name text,
    status text,
    details text
) AS $$
BEGIN
    -- Check backup files exist
    RETURN QUERY
    SELECT 
        'Backup files',
        CASE WHEN pg_stat_file('/backup/base_' || backup_date || '.tar.gz') IS NOT NULL 
             THEN 'OK' ELSE 'FAILED' END,
        'Base backup file check';
    
    -- Check WAL continuity
    RETURN QUERY
    WITH wal_check AS (
        SELECT COUNT(*) as missing_count
        FROM generate_series(
            (SELECT min(name::bigint) FROM pg_ls_waldir()),
            (SELECT max(name::bigint) FROM pg_ls_waldir())
        ) AS expected(wal)
        LEFT JOIN pg_ls_waldir() actual ON actual.name = expected.wal::text
        WHERE actual.name IS NULL
    )
    SELECT 
        'WAL continuity',
        CASE WHEN missing_count = 0 THEN 'OK' ELSE 'FAILED' END,
        'Missing WALs: ' || missing_count
    FROM wal_check;
END;
$$ LANGUAGE plpgsql;
```

## Performance Monitoring

### Key Metrics

```sql
-- Database performance dashboard
CREATE VIEW performance_metrics AS
SELECT 
    -- Connection metrics
    (SELECT count(*) FROM pg_stat_activity) as active_connections,
    (SELECT count(*) FROM pg_stat_activity WHERE state = 'idle in transaction') as idle_in_transaction,
    
    -- Cache hit ratio
    (SELECT 
        round(100.0 * sum(heap_blks_hit) / 
        (sum(heap_blks_hit) + sum(heap_blks_read)), 2)
     FROM pg_statio_user_tables) as cache_hit_ratio,
    
    -- Transaction metrics
    (SELECT sum(xact_commit + xact_rollback) FROM pg_stat_database) as total_transactions,
    (SELECT sum(xact_rollback) FROM pg_stat_database) as rollback_count,
    
    -- Replication lag
    (SELECT EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp()))::int 
     FROM pg_stat_replication) as replication_lag_seconds,
    
    -- Table bloat
    (SELECT round(avg(n_dead_tup::numeric / NULLIF(n_live_tup, 0)), 4) 
     FROM pg_stat_user_tables) as avg_dead_tuple_ratio;
```

### Automated Maintenance

```sql
-- Automated maintenance tasks
CREATE OR REPLACE FUNCTION perform_maintenance()
RETURNS void AS $$
BEGIN
    -- Update table statistics
    ANALYZE;
    
    -- Reindex bloated indexes
    FOR r IN 
        SELECT schemaname, tablename, indexname 
        FROM pg_stat_user_indexes 
        WHERE pg_relation_size(indexrelid) > 100000000  -- 100MB
        AND idx_scan = 0  -- Unused indexes
    LOOP
        EXECUTE format('DROP INDEX CONCURRENTLY %I.%I', 
                      r.schemaname, r.indexname);
    END LOOP;
    
    -- Vacuum tables with high dead tuples
    FOR r IN
        SELECT schemaname, tablename
        FROM pg_stat_user_tables
        WHERE n_dead_tup > 10000
        AND n_dead_tup > n_live_tup * 0.1
    LOOP
        EXECUTE format('VACUUM ANALYZE %I.%I', 
                      r.schemaname, r.tablename);
    END LOOP;
END;
$$ LANGUAGE plpgsql;
```

---

*For data model details, see [Data Model Architecture](./data-model.md).*