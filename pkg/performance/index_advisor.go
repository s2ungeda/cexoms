package performance

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// IndexAdvisor analyzes queries and recommends database indexes
type IndexAdvisor struct {
	db              *sql.DB
	queryLog        *QueryLog
	existingIndexes map[string][]IndexInfo
	metrics         *IndexMetrics
}

// IndexRecommendation represents a recommended index
type IndexRecommendation struct {
	TableName       string
	Columns         []string
	Type            IndexType
	Priority        int // 1-10, higher is more important
	EstimatedImpact float64 // Estimated performance improvement percentage
	Reason          string
	SQL             string
}

// IndexInfo contains information about an existing index
type IndexInfo struct {
	Name       string
	TableName  string
	Columns    []string
	Type       IndexType
	Size       int64
	Usage      int64 // Number of times used
	LastUsed   time.Time
	Selectivity float64
}

// IndexType represents the type of index
type IndexType string

const (
	BTreeIndex      IndexType = "btree"
	HashIndex       IndexType = "hash"
	GINIndex        IndexType = "gin"   // For JSONB columns
	GISTIndex       IndexType = "gist"  // For spatial data
	BRINIndex       IndexType = "brin"  // For large sequential data
	CompositeIndex  IndexType = "composite"
	PartialIndex    IndexType = "partial"
	CoveringIndex   IndexType = "covering"
)

// QueryLog tracks query patterns
type QueryLog struct {
	queries    []QueryPattern
	maxEntries int
}

// QueryPattern represents a query pattern
type QueryPattern struct {
	Pattern         string
	TableName       string
	WhereColumns    []string
	JoinColumns     []string
	OrderByColumns  []string
	SelectColumns   []string
	Frequency       int
	AvgExecutionTime time.Duration
	TotalRows       int64
}

// IndexMetrics tracks index performance
type IndexMetrics struct {
	recommendations *prometheus.GaugeVec
	indexUsage      *prometheus.GaugeVec
	indexSize       *prometheus.GaugeVec
	indexImpact     *prometheus.HistogramVec
}

// NewIndexAdvisor creates a new index advisor
func NewIndexAdvisor(db *sql.DB) *IndexAdvisor {
	metrics := &IndexMetrics{
		recommendations: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "index_recommendations_total",
				Help: "Total number of index recommendations",
			},
			[]string{"table", "priority"},
		),
		indexUsage: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "index_usage_count",
				Help: "Number of times each index is used",
			},
			[]string{"table", "index"},
		),
		indexSize: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "index_size_bytes",
				Help: "Size of indexes in bytes",
			},
			[]string{"table", "index"},
		),
		indexImpact: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "index_impact_percentage",
				Help:    "Estimated performance impact of recommended indexes",
				Buckets: prometheus.LinearBuckets(0, 10, 11),
			},
			[]string{"table"},
		),
	}

	return &IndexAdvisor{
		db: db,
		queryLog: &QueryLog{
			queries:    make([]QueryPattern, 0),
			maxEntries: 10000,
		},
		existingIndexes: make(map[string][]IndexInfo),
		metrics:         metrics,
	}
}

// AnalyzeQueries analyzes query patterns and generates recommendations
func (ia *IndexAdvisor) AnalyzeQueries(ctx context.Context) ([]*IndexRecommendation, error) {
	// Load existing indexes
	if err := ia.loadExistingIndexes(ctx); err != nil {
		return nil, err
	}

	// Analyze slow queries
	slowQueries, err := ia.getSlowQueries(ctx)
	if err != nil {
		return nil, err
	}

	// Analyze query patterns
	patterns := ia.analyzeQueryPatterns(slowQueries)

	// Generate recommendations
	recommendations := ia.generateRecommendations(patterns)

	// Sort by priority
	sort.Slice(recommendations, func(i, j int) bool {
		return recommendations[i].Priority > recommendations[j].Priority
	})

	// Update metrics
	ia.updateMetrics(recommendations)

	return recommendations, nil
}

// loadExistingIndexes loads information about existing indexes
func (ia *IndexAdvisor) loadExistingIndexes(ctx context.Context) error {
	query := `
		SELECT 
			i.relname AS index_name,
			t.relname AS table_name,
			a.attname AS column_name,
			ix.indisunique AS is_unique,
			ix.indisprimary AS is_primary,
			pg_size_pretty(pg_relation_size(i.oid)) AS size,
			pg_relation_size(i.oid) AS size_bytes,
			idx_scan AS usage_count
		FROM pg_index ix
		JOIN pg_class t ON t.oid = ix.indrelid
		JOIN pg_class i ON i.oid = ix.indexrelid
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
		LEFT JOIN pg_stat_user_indexes ui ON ui.indexrelid = i.oid
		WHERE t.relkind = 'r'
		ORDER BY t.relname, i.relname;
	`

	rows, err := ia.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to load indexes: %w", err)
	}
	defer rows.Close()

	ia.existingIndexes = make(map[string][]IndexInfo)

	for rows.Next() {
		var info IndexInfo
		var sizeStr string
		var columnName string
		var isUnique, isPrimary bool

		if err := rows.Scan(&info.Name, &info.TableName, &columnName, &isUnique, &isPrimary, &sizeStr, &info.Size, &info.Usage); err != nil {
			continue
		}

		info.Columns = append(info.Columns, columnName)
		info.Type = BTreeIndex // Default, would need to query pg_am for actual type

		if existing, ok := ia.existingIndexes[info.TableName]; ok {
			// Check if this is the same index with multiple columns
			found := false
			for i, idx := range existing {
				if idx.Name == info.Name {
					existing[i].Columns = append(existing[i].Columns, columnName)
					found = true
					break
				}
			}
			if !found {
				ia.existingIndexes[info.TableName] = append(existing, info)
			}
		} else {
			ia.existingIndexes[info.TableName] = []IndexInfo{info}
		}
	}

	return nil
}

// getSlowQueries retrieves slow queries from pg_stat_statements
func (ia *IndexAdvisor) getSlowQueries(ctx context.Context) ([]QueryPattern, error) {
	// Check if pg_stat_statements is available
	var enabled bool
	err := ia.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')").Scan(&enabled)
	if err != nil || !enabled {
		return ia.queryLog.queries, nil // Fall back to logged queries
	}

	query := `
		SELECT 
			query,
			calls,
			mean_exec_time,
			rows / GREATEST(calls, 1) AS avg_rows
		FROM pg_stat_statements
		WHERE query NOT LIKE '%pg_stat_statements%'
			AND mean_exec_time > 100 -- Only queries taking > 100ms
		ORDER BY mean_exec_time DESC
		LIMIT 100;
	`

	rows, err := ia.db.QueryContext(ctx, query)
	if err != nil {
		return ia.queryLog.queries, nil // Fall back to logged queries
	}
	defer rows.Close()

	var patterns []QueryPattern
	for rows.Next() {
		var pattern QueryPattern
		var queryText string
		var calls int

		if err := rows.Scan(&queryText, &calls, &pattern.AvgExecutionTime, &pattern.TotalRows); err != nil {
			continue
		}

		// Parse the query to extract pattern information
		pattern.Pattern = queryText
		pattern.Frequency = calls
		ia.parseQueryPattern(queryText, &pattern)

		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

// parseQueryPattern extracts pattern information from a query
func (ia *IndexAdvisor) parseQueryPattern(query string, pattern *QueryPattern) {
	query = strings.ToUpper(query)

	// Extract table name (simplified)
	if fromIdx := strings.Index(query, "FROM "); fromIdx != -1 {
		remaining := query[fromIdx+5:]
		if spaceIdx := strings.Index(remaining, " "); spaceIdx != -1 {
			pattern.TableName = strings.ToLower(remaining[:spaceIdx])
		}
	}

	// Extract WHERE columns
	if whereIdx := strings.Index(query, "WHERE "); whereIdx != -1 {
		whereClause := query[whereIdx+6:]
		// Extract column names from conditions
		// This is simplified - real implementation would use SQL parser
		pattern.WhereColumns = extractColumns(whereClause)
	}

	// Extract ORDER BY columns
	if orderIdx := strings.Index(query, "ORDER BY "); orderIdx != -1 {
		orderClause := query[orderIdx+9:]
		pattern.OrderByColumns = extractColumns(orderClause)
	}

	// Extract JOIN columns
	if strings.Contains(query, "JOIN") {
		// Extract join conditions
		pattern.JoinColumns = extractJoinColumns(query)
	}
}

// analyzeQueryPatterns analyzes patterns to identify optimization opportunities
func (ia *IndexAdvisor) analyzeQueryPatterns(queries []QueryPattern) []QueryPattern {
	// Group similar patterns
	patternMap := make(map[string]*QueryPattern)

	for _, query := range queries {
		key := fmt.Sprintf("%s:%s:%s", 
			query.TableName,
			strings.Join(query.WhereColumns, ","),
			strings.Join(query.OrderByColumns, ","))

		if existing, ok := patternMap[key]; ok {
			existing.Frequency += query.Frequency
			existing.TotalRows += query.TotalRows
			if query.AvgExecutionTime > existing.AvgExecutionTime {
				existing.AvgExecutionTime = query.AvgExecutionTime
			}
		} else {
			patternMap[key] = &query
		}
	}

	// Convert back to slice
	patterns := make([]QueryPattern, 0, len(patternMap))
	for _, pattern := range patternMap {
		patterns = append(patterns, *pattern)
	}

	return patterns
}

// generateRecommendations creates index recommendations based on patterns
func (ia *IndexAdvisor) generateRecommendations(patterns []QueryPattern) []*IndexRecommendation {
	var recommendations []*IndexRecommendation

	for _, pattern := range patterns {
		// Check if index already exists
		if ia.hasIndex(pattern.TableName, pattern.WhereColumns) {
			continue
		}

		// Calculate priority based on frequency and execution time
		priority := ia.calculatePriority(pattern)

		// Determine index type
		indexType := ia.determineIndexType(pattern)

		// Generate recommendation
		rec := &IndexRecommendation{
			TableName:       pattern.TableName,
			Columns:         pattern.WhereColumns,
			Type:            indexType,
			Priority:        priority,
			EstimatedImpact: ia.estimateImpact(pattern),
			Reason:          ia.generateReason(pattern),
		}

		// Generate SQL
		rec.SQL = ia.generateIndexSQL(rec)

		// Add covering index if beneficial
		if len(pattern.SelectColumns) > 0 && len(pattern.SelectColumns) < 5 {
			coveringRec := ia.generateCoveringIndex(pattern, rec)
			if coveringRec != nil {
				recommendations = append(recommendations, coveringRec)
			}
		}

		recommendations = append(recommendations, rec)
	}

	return recommendations
}

// hasIndex checks if an index already exists
func (ia *IndexAdvisor) hasIndex(tableName string, columns []string) bool {
	indexes, exists := ia.existingIndexes[tableName]
	if !exists {
		return false
	}

	for _, index := range indexes {
		if columnsMatch(index.Columns, columns) {
			return true
		}
	}

	return false
}

// calculatePriority calculates recommendation priority
func (ia *IndexAdvisor) calculatePriority(pattern QueryPattern) int {
	priority := 5 // Base priority

	// Increase for high frequency
	if pattern.Frequency > 1000 {
		priority += 2
	} else if pattern.Frequency > 100 {
		priority += 1
	}

	// Increase for slow queries
	if pattern.AvgExecutionTime > time.Second {
		priority += 3
	} else if pattern.AvgExecutionTime > 500*time.Millisecond {
		priority += 2
	} else if pattern.AvgExecutionTime > 100*time.Millisecond {
		priority += 1
	}

	// Cap at 10
	if priority > 10 {
		priority = 10
	}

	return priority
}

// determineIndexType determines the appropriate index type
func (ia *IndexAdvisor) determineIndexType(pattern QueryPattern) IndexType {
	// Check for JSONB columns
	for _, col := range pattern.WhereColumns {
		if strings.Contains(col, "->") || strings.Contains(col, "->>") {
			return GINIndex
		}
	}

	// Check for composite index need
	if len(pattern.WhereColumns) > 1 {
		return CompositeIndex
	}

	// Default to B-tree
	return BTreeIndex
}

// estimateImpact estimates the performance impact
func (ia *IndexAdvisor) estimateImpact(pattern QueryPattern) float64 {
	// Simple estimation based on execution time and row count
	if pattern.TotalRows > 10000 && pattern.AvgExecutionTime > time.Second {
		return 80.0 // High impact
	} else if pattern.TotalRows > 1000 && pattern.AvgExecutionTime > 500*time.Millisecond {
		return 60.0 // Medium impact
	} else if pattern.AvgExecutionTime > 100*time.Millisecond {
		return 40.0 // Low impact
	}
	return 20.0
}

// generateReason creates a human-readable reason for the recommendation
func (ia *IndexAdvisor) generateReason(pattern QueryPattern) string {
	return fmt.Sprintf(
		"Query on table '%s' with WHERE columns %v executes %d times with avg time %.2fms. "+
		"Index would reduce full table scans and improve query performance.",
		pattern.TableName,
		pattern.WhereColumns,
		pattern.Frequency,
		pattern.AvgExecutionTime.Seconds()*1000,
	)
}

// generateIndexSQL generates the CREATE INDEX SQL
func (ia *IndexAdvisor) generateIndexSQL(rec *IndexRecommendation) string {
	indexName := fmt.Sprintf("idx_%s_%s", rec.TableName, strings.Join(rec.Columns, "_"))
	
	switch rec.Type {
	case GINIndex:
		return fmt.Sprintf("CREATE INDEX %s ON %s USING gin (%s);",
			indexName, rec.TableName, strings.Join(rec.Columns, ", "))
	case CompositeIndex:
		return fmt.Sprintf("CREATE INDEX %s ON %s (%s);",
			indexName, rec.TableName, strings.Join(rec.Columns, ", "))
	default:
		return fmt.Sprintf("CREATE INDEX %s ON %s (%s);",
			indexName, rec.TableName, strings.Join(rec.Columns, ", "))
	}
}

// generateCoveringIndex generates a covering index recommendation
func (ia *IndexAdvisor) generateCoveringIndex(pattern QueryPattern, baseRec *IndexRecommendation) *IndexRecommendation {
	// Combine WHERE and SELECT columns
	columns := append(pattern.WhereColumns, pattern.SelectColumns...)
	columns = uniqueStrings(columns)

	if len(columns) > 5 || ia.hasIndex(pattern.TableName, columns) {
		return nil
	}

	return &IndexRecommendation{
		TableName:       pattern.TableName,
		Columns:         columns,
		Type:            CoveringIndex,
		Priority:        baseRec.Priority - 1,
		EstimatedImpact: baseRec.EstimatedImpact * 1.2,
		Reason:          "Covering index to eliminate table lookups",
		SQL: fmt.Sprintf("CREATE INDEX idx_%s_covering ON %s (%s) INCLUDE (%s);",
			pattern.TableName,
			pattern.TableName,
			strings.Join(pattern.WhereColumns, ", "),
			strings.Join(pattern.SelectColumns, ", ")),
	}
}

// updateMetrics updates index advisor metrics
func (ia *IndexAdvisor) updateMetrics(recommendations []*IndexRecommendation) {
	for _, rec := range recommendations {
		ia.metrics.recommendations.WithLabelValues(rec.TableName, fmt.Sprintf("%d", rec.Priority)).Inc()
		ia.metrics.indexImpact.WithLabelValues(rec.TableName).Observe(rec.EstimatedImpact)
	}

	// Update existing index metrics
	for table, indexes := range ia.existingIndexes {
		for _, index := range indexes {
			ia.metrics.indexUsage.WithLabelValues(table, index.Name).Set(float64(index.Usage))
			ia.metrics.indexSize.WithLabelValues(table, index.Name).Set(float64(index.Size))
		}
	}
}

// Helper functions

func extractColumns(clause string) []string {
	// Simplified column extraction
	var columns []string
	// Would use proper SQL parser in production
	return columns
}

func extractJoinColumns(query string) []string {
	// Simplified join column extraction
	var columns []string
	// Would use proper SQL parser in production
	return columns
}

func columnsMatch(cols1, cols2 []string) bool {
	if len(cols1) != len(cols2) {
		return false
	}
	for i := range cols1 {
		if cols1[i] != cols2[i] {
			return false
		}
	}
	return true
}

func uniqueStrings(strs []string) []string {
	seen := make(map[string]bool)
	var unique []string
	for _, str := range strs {
		if !seen[str] {
			seen[str] = true
			unique = append(unique, str)
		}
	}
	return unique
}