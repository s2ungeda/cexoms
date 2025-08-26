package performance

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// PartitionManager manages database partitioning for time-series data
type PartitionManager struct {
	db              *sql.DB
	config          *PartitionConfig
	metrics         *PartitionMetrics
	maintenanceStop chan struct{}
}

// PartitionConfig defines partitioning configuration
type PartitionConfig struct {
	Tables            []TablePartitionConfig
	RetentionDays     int
	MaintenanceHour   int // Hour of day to run maintenance (0-23)
	PreCreateDays     int // Number of future partitions to pre-create
	MaxPartitionSize  int64 // Max size in bytes before splitting
}

// TablePartitionConfig configures partitioning for a specific table
type TablePartitionConfig struct {
	TableName       string
	PartitionColumn string
	PartitionType   PartitionType
	Interval        time.Duration
	Indexes         []string
}

// PartitionType defines the type of partitioning
type PartitionType string

const (
	RangePartition PartitionType = "RANGE"
	ListPartition  PartitionType = "LIST"
	HashPartition  PartitionType = "HASH"
)

// PartitionInfo holds information about a partition
type PartitionInfo struct {
	Name         string
	TableName    string
	StartRange   time.Time
	EndRange     time.Time
	RowCount     int64
	SizeBytes    int64
	CreatedAt    time.Time
}

// PartitionMetrics tracks partition performance
type PartitionMetrics struct {
	partitionCount  *prometheus.GaugeVec
	partitionSize   *prometheus.GaugeVec
	partitionAge    *prometheus.HistogramVec
	maintenanceTime *prometheus.HistogramVec
}

// NewPartitionManager creates a new partition manager
func NewPartitionManager(db *sql.DB, config *PartitionConfig) *PartitionManager {
	metrics := &PartitionMetrics{
		partitionCount: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "partition_count",
				Help: "Number of partitions per table",
			},
			[]string{"table"},
		),
		partitionSize: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "partition_size_bytes",
				Help: "Size of partitions in bytes",
			},
			[]string{"table", "partition"},
		),
		partitionAge: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "partition_age_days",
				Help:    "Age of partitions in days",
				Buckets: prometheus.LinearBuckets(0, 1, 90),
			},
			[]string{"table"},
		),
		maintenanceTime: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "partition_maintenance_duration_seconds",
				Help:    "Duration of partition maintenance operations",
				Buckets: prometheus.ExponentialBuckets(1, 2, 10),
			},
			[]string{"operation"},
		),
	}

	pm := &PartitionManager{
		db:              db,
		config:          config,
		metrics:         metrics,
		maintenanceStop: make(chan struct{}),
	}

	// Start background maintenance
	go pm.backgroundMaintenance()

	return pm
}

// CreatePartitionedTable creates a new partitioned table
func (pm *PartitionManager) CreatePartitionedTable(ctx context.Context, config TablePartitionConfig) error {
	// Create parent table
	createTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id BIGSERIAL,
			timestamp TIMESTAMPTZ NOT NULL,
			exchange VARCHAR(50) NOT NULL,
			symbol VARCHAR(50) NOT NULL,
			data JSONB,
			PRIMARY KEY (id, timestamp)
		) PARTITION BY %s (%s);
	`, config.TableName, config.PartitionType, config.PartitionColumn)

	if _, err := pm.db.ExecContext(ctx, createTableSQL); err != nil {
		return fmt.Errorf("failed to create partitioned table: %w", err)
	}

	// Create initial partitions
	return pm.createInitialPartitions(ctx, config)
}

// createInitialPartitions creates initial partitions for a table
func (pm *PartitionManager) createInitialPartitions(ctx context.Context, config TablePartitionConfig) error {
	now := time.Now().UTC()
	
	// Create past partitions (for retention period)
	startDate := now.AddDate(0, 0, -pm.config.RetentionDays)
	
	// Create future partitions
	endDate := now.AddDate(0, 0, pm.config.PreCreateDays)

	current := startDate
	for current.Before(endDate) {
		partitionName := pm.generatePartitionName(config.TableName, current)
		
		if err := pm.createPartition(ctx, config, partitionName, current); err != nil {
			return err
		}

		// Move to next interval
		switch config.Interval {
		case 24 * time.Hour:
			current = current.AddDate(0, 0, 1)
		case 7 * 24 * time.Hour:
			current = current.AddDate(0, 0, 7)
		case 30 * 24 * time.Hour:
			current = current.AddDate(0, 1, 0)
		default:
			current = current.Add(config.Interval)
		}
	}

	return nil
}

// createPartition creates a single partition
func (pm *PartitionManager) createPartition(ctx context.Context, config TablePartitionConfig, partitionName string, startTime time.Time) error {
	var endTime time.Time
	
	switch config.Interval {
	case 24 * time.Hour:
		endTime = startTime.AddDate(0, 0, 1)
	case 7 * 24 * time.Hour:
		endTime = startTime.AddDate(0, 0, 7)
	case 30 * 24 * time.Hour:
		endTime = startTime.AddDate(0, 1, 0)
	default:
		endTime = startTime.Add(config.Interval)
	}

	createPartitionSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s PARTITION OF %s
		FOR VALUES FROM ('%s') TO ('%s');
	`, partitionName, config.TableName, 
		startTime.Format("2006-01-02 15:04:05"),
		endTime.Format("2006-01-02 15:04:05"))

	if _, err := pm.db.ExecContext(ctx, createPartitionSQL); err != nil {
		// Partition might already exist
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("failed to create partition: %w", err)
		}
	}

	// Create indexes on partition
	for _, index := range config.Indexes {
		indexName := fmt.Sprintf("%s_%s_idx", partitionName, index)
		createIndexSQL := fmt.Sprintf(`
			CREATE INDEX IF NOT EXISTS %s ON %s (%s);
		`, indexName, partitionName, index)
		
		if _, err := pm.db.ExecContext(ctx, createIndexSQL); err != nil {
			// Continue if index already exists
		}
	}

	return nil
}

// generatePartitionName generates a partition name based on date
func (pm *PartitionManager) generatePartitionName(tableName string, date time.Time) string {
	return fmt.Sprintf("%s_p%s", tableName, date.Format("20060102"))
}

// GetPartitionInfo retrieves information about partitions
func (pm *PartitionManager) GetPartitionInfo(ctx context.Context, tableName string) ([]*PartitionInfo, error) {
	query := `
		SELECT 
			child.relname AS partition_name,
			pg_size_pretty(pg_total_relation_size(child.oid)) AS size,
			pg_total_relation_size(child.oid) AS size_bytes,
			(SELECT COUNT(*) FROM pg_class WHERE oid = child.oid) AS row_count
		FROM pg_inherits
		JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
		JOIN pg_class child ON pg_inherits.inhrelid = child.oid
		WHERE parent.relname = $1
		ORDER BY child.relname;
	`

	rows, err := pm.db.QueryContext(ctx, query, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to query partition info: %w", err)
	}
	defer rows.Close()

	var partitions []*PartitionInfo
	for rows.Next() {
		var info PartitionInfo
		var sizeStr string
		
		if err := rows.Scan(&info.Name, &sizeStr, &info.SizeBytes, &info.RowCount); err != nil {
			continue
		}
		
		info.TableName = tableName
		partitions = append(partitions, &info)
	}

	// Update metrics
	pm.updateMetrics(tableName, partitions)

	return partitions, nil
}

// backgroundMaintenance runs periodic maintenance tasks
func (pm *PartitionManager) backgroundMaintenance() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Check if it's maintenance hour
			if time.Now().Hour() == pm.config.MaintenanceHour {
				ctx := context.Background()
				pm.runMaintenance(ctx)
			}
		case <-pm.maintenanceStop:
			return
		}
	}
}

// runMaintenance performs partition maintenance tasks
func (pm *PartitionManager) runMaintenance(ctx context.Context) {
	start := time.Now()
	defer func() {
		pm.metrics.maintenanceTime.WithLabelValues("full").Observe(time.Since(start).Seconds())
	}()

	for _, tableConfig := range pm.config.Tables {
		// Create future partitions
		if err := pm.createFuturePartitions(ctx, tableConfig); err != nil {
			// Log error but continue
		}

		// Drop old partitions
		if err := pm.dropOldPartitions(ctx, tableConfig); err != nil {
			// Log error but continue
		}

		// Analyze partitions for optimization
		if err := pm.analyzePartitions(ctx, tableConfig); err != nil {
			// Log error but continue
		}
	}
}

// createFuturePartitions ensures future partitions exist
func (pm *PartitionManager) createFuturePartitions(ctx context.Context, config TablePartitionConfig) error {
	now := time.Now().UTC()
	futureDate := now.AddDate(0, 0, pm.config.PreCreateDays)

	// Find the latest existing partition
	latestPartition, err := pm.getLatestPartition(ctx, config.TableName)
	if err != nil {
		return err
	}

	// Create partitions from latest to future date
	current := latestPartition.EndRange
	for current.Before(futureDate) {
		partitionName := pm.generatePartitionName(config.TableName, current)
		if err := pm.createPartition(ctx, config, partitionName, current); err != nil {
			return err
		}

		switch config.Interval {
		case 24 * time.Hour:
			current = current.AddDate(0, 0, 1)
		case 7 * 24 * time.Hour:
			current = current.AddDate(0, 0, 7)
		case 30 * 24 * time.Hour:
			current = current.AddDate(0, 1, 0)
		default:
			current = current.Add(config.Interval)
		}
	}

	return nil
}

// dropOldPartitions removes partitions older than retention period
func (pm *PartitionManager) dropOldPartitions(ctx context.Context, config TablePartitionConfig) error {
	cutoffDate := time.Now().UTC().AddDate(0, 0, -pm.config.RetentionDays)

	partitions, err := pm.GetPartitionInfo(ctx, config.TableName)
	if err != nil {
		return err
	}

	for _, partition := range partitions {
		// Parse date from partition name
		if partition.EndRange.Before(cutoffDate) {
			dropSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s", partition.Name)
			if _, err := pm.db.ExecContext(ctx, dropSQL); err != nil {
				// Log error but continue
			}
		}
	}

	return nil
}

// analyzePartitions updates partition statistics
func (pm *PartitionManager) analyzePartitions(ctx context.Context, config TablePartitionConfig) error {
	partitions, err := pm.GetPartitionInfo(ctx, config.TableName)
	if err != nil {
		return err
	}

	for _, partition := range partitions {
		// Check if partition needs splitting (too large)
		if partition.SizeBytes > pm.config.MaxPartitionSize {
			// Consider splitting the partition
			// This would involve creating sub-partitions
		}

		// Update statistics
		analyzeSQL := fmt.Sprintf("ANALYZE %s", partition.Name)
		pm.db.ExecContext(ctx, analyzeSQL)
	}

	return nil
}

// getLatestPartition finds the most recent partition
func (pm *PartitionManager) getLatestPartition(ctx context.Context, tableName string) (*PartitionInfo, error) {
	partitions, err := pm.GetPartitionInfo(ctx, tableName)
	if err != nil {
		return nil, err
	}

	if len(partitions) == 0 {
		return &PartitionInfo{
			EndRange: time.Now().UTC().Truncate(24 * time.Hour),
		}, nil
	}

	// Find the latest by name (assumes naming convention)
	latest := partitions[0]
	for _, p := range partitions {
		if p.Name > latest.Name {
			latest = p
		}
	}

	return latest, nil
}

// updateMetrics updates partition metrics
func (pm *PartitionManager) updateMetrics(tableName string, partitions []*PartitionInfo) {
	pm.metrics.partitionCount.WithLabelValues(tableName).Set(float64(len(partitions)))

	for _, partition := range partitions {
		pm.metrics.partitionSize.WithLabelValues(tableName, partition.Name).Set(float64(partition.SizeBytes))
		
		// Calculate age if dates are available
		if !partition.CreatedAt.IsZero() {
			age := time.Since(partition.CreatedAt).Hours() / 24
			pm.metrics.partitionAge.WithLabelValues(tableName).Observe(age)
		}
	}
}

// OptimizePartition optimizes a specific partition
func (pm *PartitionManager) OptimizePartition(ctx context.Context, partitionName string) error {
	// Rebuild indexes
	reindexSQL := fmt.Sprintf("REINDEX TABLE %s", partitionName)
	if _, err := pm.db.ExecContext(ctx, reindexSQL); err != nil {
		return fmt.Errorf("failed to reindex partition: %w", err)
	}

	// Cluster table on primary key for better performance
	clusterSQL := fmt.Sprintf("CLUSTER %s USING %s_pkey", partitionName, partitionName)
	if _, err := pm.db.ExecContext(ctx, clusterSQL); err != nil {
		// Clustering might not be available
	}

	// Update statistics
	analyzeSQL := fmt.Sprintf("VACUUM ANALYZE %s", partitionName)
	if _, err := pm.db.ExecContext(ctx, analyzeSQL); err != nil {
		return fmt.Errorf("failed to analyze partition: %w", err)
	}

	return nil
}