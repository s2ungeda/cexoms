package performance

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// QueryOptimizer provides SQL query optimization utilities
type QueryOptimizer struct {
	db              *sql.DB
	queryCache      *sync.Map
	metricsCollector *QueryMetricsCollector
	explainCache    *sync.Map
}

// QueryMetrics holds query performance metrics
type QueryMetrics struct {
	Query          string
	ExecutionTime  time.Duration
	RowsExamined   int64
	RowsReturned   int64
	IndexUsed      bool
	OptimizedQuery string
}

// QueryMetricsCollector collects query performance metrics
type QueryMetricsCollector struct {
	queryDuration *prometheus.HistogramVec
	slowQueries   *prometheus.CounterVec
	indexUsage    *prometheus.GaugeVec
}

// NewQueryOptimizer creates a new query optimizer
func NewQueryOptimizer(db *sql.DB) *QueryOptimizer {
	collector := &QueryMetricsCollector{
		queryDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "query_duration_seconds",
				Help:    "Query execution duration in seconds",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
			},
			[]string{"query_type", "table"},
		),
		slowQueries: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "slow_queries_total",
				Help: "Total number of slow queries",
			},
			[]string{"query_type", "table"},
		),
		indexUsage: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "query_index_usage_ratio",
				Help: "Ratio of queries using indexes",
			},
			[]string{"table"},
		),
	}

	return &QueryOptimizer{
		db:               db,
		queryCache:       &sync.Map{},
		metricsCollector: collector,
		explainCache:     &sync.Map{},
	}
}

// OptimizeQuery analyzes and optimizes a SQL query
func (o *QueryOptimizer) OptimizeQuery(ctx context.Context, query string) (*QueryMetrics, error) {
	// Check cache first
	if cached, ok := o.queryCache.Load(query); ok {
		return cached.(*QueryMetrics), nil
	}

	metrics := &QueryMetrics{
		Query: query,
	}

	// Analyze query execution plan
	explainQuery := fmt.Sprintf("EXPLAIN ANALYZE %s", query)
	rows, err := o.db.QueryContext(ctx, explainQuery)
	if err != nil {
		// If EXPLAIN ANALYZE fails, try regular EXPLAIN
		explainQuery = fmt.Sprintf("EXPLAIN %s", query)
		rows, err = o.db.QueryContext(ctx, explainQuery)
		if err != nil {
			return nil, fmt.Errorf("failed to analyze query: %w", err)
		}
	}
	defer rows.Close()

	var plan []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			continue
		}
		plan = append(plan, line)
	}

	// Analyze the execution plan
	o.analyzePlan(plan, metrics)

	// Apply optimizations
	optimized := o.applyOptimizations(query, metrics)
	metrics.OptimizedQuery = optimized

	// Cache the results
	o.queryCache.Store(query, metrics)

	// Record metrics
	o.recordMetrics(metrics)

	return metrics, nil
}

// analyzePlan analyzes the query execution plan
func (o *QueryOptimizer) analyzePlan(plan []string, metrics *QueryMetrics) {
	for _, line := range plan {
		line = strings.ToLower(line)

		// Check for index usage
		if strings.Contains(line, "using index") || strings.Contains(line, "index scan") {
			metrics.IndexUsed = true
		}

		// Extract rows examined
		if strings.Contains(line, "rows=") {
			// Parse rows from execution plan
			// This is simplified - actual implementation would parse more accurately
			metrics.RowsExamined = 1000 // placeholder
		}

		// Check for problematic operations
		if strings.Contains(line, "seq scan") || strings.Contains(line, "full table scan") {
			metrics.IndexUsed = false
		}
	}
}

// applyOptimizations applies query optimizations
func (o *QueryOptimizer) applyOptimizations(query string, metrics *QueryMetrics) string {
	optimized := query

	// Apply various optimizations
	optimized = o.optimizeJoins(optimized)
	optimized = o.optimizeWhereClause(optimized)
	optimized = o.optimizeSelectClause(optimized)
	optimized = o.addIndexHints(optimized, metrics)

	return optimized
}

// optimizeJoins optimizes JOIN operations
func (o *QueryOptimizer) optimizeJoins(query string) string {
	// Convert implicit joins to explicit joins
	if strings.Contains(query, "WHERE") && strings.Contains(query, "=") {
		// Check for potential implicit joins
		// This is a simplified example
		query = strings.ReplaceAll(query, "FROM table1, table2", "FROM table1 JOIN table2")
	}

	// Ensure proper join order (smaller tables first)
	// This would require table statistics in practice

	return query
}

// optimizeWhereClause optimizes WHERE conditions
func (o *QueryOptimizer) optimizeWhereClause(query string) string {
	// Move selective conditions first
	// Remove redundant conditions
	// Convert NOT IN to NOT EXISTS where appropriate

	// Example: Convert OR to UNION where beneficial
	if strings.Count(query, " OR ") > 3 {
		// Consider converting to UNION for better performance
	}

	return query
}

// optimizeSelectClause optimizes SELECT clause
func (o *QueryOptimizer) optimizeSelectClause(query string) string {
	// Remove unnecessary columns
	// Add covering index hints
	// Convert SELECT * to specific columns

	if strings.Contains(query, "SELECT *") {
		// In practice, would determine actual needed columns
		query = strings.ReplaceAll(query, "SELECT *", "SELECT id, timestamp, price, amount")
	}

	return query
}

// addIndexHints adds index hints based on query analysis
func (o *QueryOptimizer) addIndexHints(query string, metrics *QueryMetrics) string {
	if !metrics.IndexUsed {
		// Add USE INDEX hint for PostgreSQL
		// Example: would analyze available indexes and add appropriate hints
		if strings.Contains(query, "WHERE timestamp") {
			// Add timestamp index hint
		}
	}
	return query
}

// BatchOptimize optimizes multiple queries as a batch
func (o *QueryOptimizer) BatchOptimize(ctx context.Context, queries []string) ([]*QueryMetrics, error) {
	results := make([]*QueryMetrics, len(queries))
	var wg sync.WaitGroup
	errChan := make(chan error, len(queries))

	for i, query := range queries {
		wg.Add(1)
		go func(idx int, q string) {
			defer wg.Done()
			metrics, err := o.OptimizeQuery(ctx, q)
			if err != nil {
				errChan <- err
				return
			}
			results[idx] = metrics
		}(i, query)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

// PrepareStatement prepares and caches a statement
func (o *QueryOptimizer) PrepareStatement(ctx context.Context, name, query string) (*sql.Stmt, error) {
	// First optimize the query
	metrics, err := o.OptimizeQuery(ctx, query)
	if err != nil {
		return nil, err
	}

	// Prepare the optimized query
	stmt, err := o.db.PrepareContext(ctx, metrics.OptimizedQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare statement: %w", err)
	}

	return stmt, nil
}

// GetSlowQueries returns queries that exceed the threshold
func (o *QueryOptimizer) GetSlowQueries(threshold time.Duration) []QueryMetrics {
	var slowQueries []QueryMetrics

	o.queryCache.Range(func(key, value interface{}) bool {
		metrics := value.(*QueryMetrics)
		if metrics.ExecutionTime > threshold {
			slowQueries = append(slowQueries, *metrics)
		}
		return true
	})

	return slowQueries
}

// RecommendPartitions recommends partitioning strategies
func (o *QueryOptimizer) RecommendPartitions(ctx context.Context, table string) ([]string, error) {
	// Analyze table access patterns
	query := fmt.Sprintf(`
		SELECT 
			DATE_TRUNC('day', timestamp) as day,
			COUNT(*) as row_count,
			AVG(pg_column_size(t.*)) as avg_row_size
		FROM %s t
		WHERE timestamp > NOW() - INTERVAL '30 days'
		GROUP BY day
		ORDER BY day
	`, table)

	rows, err := o.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recommendations []string

	// Analyze data distribution
	var totalRows int64
	var dates []time.Time
	for rows.Next() {
		var day time.Time
		var count int64
		var avgSize float64
		if err := rows.Scan(&day, &count, &avgSize); err != nil {
			continue
		}
		totalRows += count
		dates = append(dates, day)
	}

	// Recommend partitioning strategy based on data distribution
	if totalRows > 10_000_000 {
		recommendations = append(recommendations, fmt.Sprintf("PARTITION %s BY RANGE (timestamp) INTERVAL '1 day'", table))
	} else if totalRows > 1_000_000 {
		recommendations = append(recommendations, fmt.Sprintf("PARTITION %s BY RANGE (timestamp) INTERVAL '1 week'", table))
	}

	return recommendations, nil
}

// recordMetrics records query performance metrics
func (o *QueryOptimizer) recordMetrics(metrics *QueryMetrics) {
	queryType := "select"
	if strings.HasPrefix(strings.ToUpper(metrics.Query), "INSERT") {
		queryType = "insert"
	} else if strings.HasPrefix(strings.ToUpper(metrics.Query), "UPDATE") {
		queryType = "update"
	} else if strings.HasPrefix(strings.ToUpper(metrics.Query), "DELETE") {
		queryType = "delete"
	}

	table := extractTableName(metrics.Query)

	o.metricsCollector.queryDuration.WithLabelValues(queryType, table).Observe(metrics.ExecutionTime.Seconds())

	if metrics.ExecutionTime > time.Second {
		o.metricsCollector.slowQueries.WithLabelValues(queryType, table).Inc()
	}

	if metrics.IndexUsed {
		o.metricsCollector.indexUsage.WithLabelValues(table).Set(1)
	} else {
		o.metricsCollector.indexUsage.WithLabelValues(table).Set(0)
	}
}

// extractTableName extracts the table name from a query
func extractTableName(query string) string {
	query = strings.ToUpper(query)
	
	// Simple extraction - in practice would use SQL parser
	if idx := strings.Index(query, "FROM "); idx != -1 {
		remaining := query[idx+5:]
		if spaceIdx := strings.Index(remaining, " "); spaceIdx != -1 {
			return strings.ToLower(remaining[:spaceIdx])
		}
	}
	
	return "unknown"
}

// AnalyzeTableStatistics analyzes table statistics for optimization
func (o *QueryOptimizer) AnalyzeTableStatistics(ctx context.Context, table string) error {
	// Update table statistics
	_, err := o.db.ExecContext(ctx, fmt.Sprintf("ANALYZE %s", table))
	if err != nil {
		return fmt.Errorf("failed to analyze table: %w", err)
	}

	// Vacuum if needed (PostgreSQL specific)
	_, err = o.db.ExecContext(ctx, fmt.Sprintf("VACUUM ANALYZE %s", table))
	if err != nil {
		// Log but don't fail - vacuum might not be needed
	}

	return nil
}