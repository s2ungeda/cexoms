package monitor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// MetricType represents the type of metric
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeSummary   MetricType = "summary"
)

// Metric represents a single metric
type Metric struct {
	Name        string                 `json:"name"`
	Type        MetricType             `json:"type"`
	Value       interface{}            `json:"value"`
	Labels      map[string]string      `json:"labels,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
	Description string                 `json:"description,omitempty"`
}

// MetricsCollector collects and stores metrics
type MetricsCollector struct {
	mu sync.RWMutex
	
	// In-memory metrics storage
	counters   map[string]*atomic.Int64
	gauges     map[string]*atomic.Value
	histograms map[string]*Histogram
	summaries  map[string]*Summary
	
	// File storage
	metricsDir     string
	rotateInterval time.Duration
	maxFileSize    int64
	
	// Channels
	metricsChan chan *Metric
	stopChan    chan struct{}
	
	// Current file
	currentFile *os.File
	fileSize    atomic.Int64
}

// Histogram tracks distribution of values
type Histogram struct {
	mu      sync.Mutex
	buckets []float64
	counts  []uint64
	sum     float64
	count   uint64
}

// Summary tracks quantiles of values
type Summary struct {
	mu         sync.Mutex
	values     []float64
	maxSamples int
	sum        float64
	count      uint64
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(metricsDir string) (*MetricsCollector, error) {
	mc := &MetricsCollector{
		counters:       make(map[string]*atomic.Int64),
		gauges:         make(map[string]*atomic.Value),
		histograms:     make(map[string]*Histogram),
		summaries:      make(map[string]*Summary),
		metricsDir:     metricsDir,
		rotateInterval: 1 * time.Hour,
		maxFileSize:    100 * 1024 * 1024, // 100MB
		metricsChan:    make(chan *Metric, 10000),
		stopChan:       make(chan struct{}),
	}
	
	// Create metrics directory
	if err := os.MkdirAll(metricsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create metrics dir: %w", err)
	}
	
	// Open initial metrics file
	if err := mc.rotateFile(); err != nil {
		return nil, fmt.Errorf("failed to create metrics file: %w", err)
	}
	
	// Start background workers
	go mc.fileWriter()
	go mc.rotateWorker()
	
	return mc, nil
}

// Counter operations

// IncrementCounter increments a counter metric
func (mc *MetricsCollector) IncrementCounter(name string, labels map[string]string) {
	key := mc.metricKey(name, labels)
	
	counter, _ := mc.counters[key]
	if counter == nil {
		mc.mu.Lock()
		counter, _ = mc.counters[key]
		if counter == nil {
			counter = &atomic.Int64{}
			mc.counters[key] = counter
		}
		mc.mu.Unlock()
	}
	
	counter.Add(1)
	
	// Send to file writer
	mc.metricsChan <- &Metric{
		Name:      name,
		Type:      MetricTypeCounter,
		Value:     counter.Load(),
		Labels:    labels,
		Timestamp: time.Now(),
	}
}

// AddCounter adds a value to a counter
func (mc *MetricsCollector) AddCounter(name string, value int64, labels map[string]string) {
	key := mc.metricKey(name, labels)
	
	counter, _ := mc.counters[key]
	if counter == nil {
		mc.mu.Lock()
		counter, _ = mc.counters[key]
		if counter == nil {
			counter = &atomic.Int64{}
			mc.counters[key] = counter
		}
		mc.mu.Unlock()
	}
	
	counter.Add(value)
	
	mc.metricsChan <- &Metric{
		Name:      name,
		Type:      MetricTypeCounter,
		Value:     counter.Load(),
		Labels:    labels,
		Timestamp: time.Now(),
	}
}

// Gauge operations

// SetGauge sets a gauge metric
func (mc *MetricsCollector) SetGauge(name string, value float64, labels map[string]string) {
	key := mc.metricKey(name, labels)
	
	gauge, _ := mc.gauges[key]
	if gauge == nil {
		mc.mu.Lock()
		gauge, _ = mc.gauges[key]
		if gauge == nil {
			gauge = &atomic.Value{}
			mc.gauges[key] = gauge
		}
		mc.mu.Unlock()
	}
	
	gauge.Store(value)
	
	mc.metricsChan <- &Metric{
		Name:      name,
		Type:      MetricTypeGauge,
		Value:     value,
		Labels:    labels,
		Timestamp: time.Now(),
	}
}

// Histogram operations

// ObserveHistogram observes a value for histogram
func (mc *MetricsCollector) ObserveHistogram(name string, value float64, labels map[string]string) {
	key := mc.metricKey(name, labels)
	
	hist, _ := mc.histograms[key]
	if hist == nil {
		mc.mu.Lock()
		hist, _ = mc.histograms[key]
		if hist == nil {
			hist = NewHistogram(defaultBuckets())
			mc.histograms[key] = hist
		}
		mc.mu.Unlock()
	}
	
	hist.Observe(value)
	
	mc.metricsChan <- &Metric{
		Name:      name,
		Type:      MetricTypeHistogram,
		Value:     hist.Snapshot(),
		Labels:    labels,
		Timestamp: time.Now(),
	}
}

// Summary operations

// ObserveSummary observes a value for summary
func (mc *MetricsCollector) ObserveSummary(name string, value float64, labels map[string]string) {
	key := mc.metricKey(name, labels)
	
	summary, _ := mc.summaries[key]
	if summary == nil {
		mc.mu.Lock()
		summary, _ = mc.summaries[key]
		if summary == nil {
			summary = NewSummary(1000) // Keep last 1000 samples
			mc.summaries[key] = summary
		}
		mc.mu.Unlock()
	}
	
	summary.Observe(value)
	
	mc.metricsChan <- &Metric{
		Name:      name,
		Type:      MetricTypeSummary,
		Value:     summary.Snapshot(),
		Labels:    labels,
		Timestamp: time.Now(),
	}
}

// GetMetrics returns current metrics snapshot
func (mc *MetricsCollector) GetMetrics() map[string]interface{} {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	
	metrics := make(map[string]interface{})
	
	// Collect counters
	counters := make(map[string]int64)
	for key, counter := range mc.counters {
		counters[key] = counter.Load()
	}
	metrics["counters"] = counters
	
	// Collect gauges
	gauges := make(map[string]float64)
	for key, gauge := range mc.gauges {
		if val := gauge.Load(); val != nil {
			gauges[key] = val.(float64)
		}
	}
	metrics["gauges"] = gauges
	
	// Collect histograms
	histograms := make(map[string]interface{})
	for key, hist := range mc.histograms {
		histograms[key] = hist.Snapshot()
	}
	metrics["histograms"] = histograms
	
	// Collect summaries
	summaries := make(map[string]interface{})
	for key, summary := range mc.summaries {
		summaries[key] = summary.Snapshot()
	}
	metrics["summaries"] = summaries
	
	return metrics
}

// fileWriter writes metrics to file
func (mc *MetricsCollector) fileWriter() {
	for {
		select {
		case metric := <-mc.metricsChan:
			if err := mc.writeMetric(metric); err != nil {
				// Log error but continue
				fmt.Printf("Failed to write metric: %v\n", err)
			}
		case <-mc.stopChan:
			return
		}
	}
}

// writeMetric writes a single metric to file
func (mc *MetricsCollector) writeMetric(metric *Metric) error {
	data, err := json.Marshal(metric)
	if err != nil {
		return fmt.Errorf("failed to marshal metric: %w", err)
	}
	
	data = append(data, '\n')
	
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	if mc.currentFile == nil {
		return fmt.Errorf("no metrics file open")
	}
	
	n, err := mc.currentFile.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write metric: %w", err)
	}
	
	mc.fileSize.Add(int64(n))
	
	// Check if rotation needed
	if mc.fileSize.Load() >= mc.maxFileSize {
		return mc.rotateFile()
	}
	
	return nil
}

// rotateFile rotates the metrics file
func (mc *MetricsCollector) rotateFile() error {
	// Close current file
	if mc.currentFile != nil {
		mc.currentFile.Close()
	}
	
	// Create new file
	filename := filepath.Join(mc.metricsDir,
		fmt.Sprintf("metrics_%s.jsonl", time.Now().Format("20060102_150405")))
	
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to create metrics file: %w", err)
	}
	
	mc.currentFile = file
	mc.fileSize.Store(0)
	
	return nil
}

// rotateWorker handles periodic file rotation
func (mc *MetricsCollector) rotateWorker() {
	ticker := time.NewTicker(mc.rotateInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			mc.mu.Lock()
			mc.rotateFile()
			mc.mu.Unlock()
		case <-mc.stopChan:
			return
		}
	}
}

// metricKey creates a unique key for a metric
func (mc *MetricsCollector) metricKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	
	key := name
	for k, v := range labels {
		key += fmt.Sprintf("_%s_%s", k, v)
	}
	return key
}

// Close closes the metrics collector
func (mc *MetricsCollector) Close() error {
	close(mc.stopChan)
	close(mc.metricsChan)
	
	mc.mu.Lock()
	defer mc.mu.Unlock()
	
	if mc.currentFile != nil {
		return mc.currentFile.Close()
	}
	
	return nil
}

// Histogram implementation

func NewHistogram(buckets []float64) *Histogram {
	return &Histogram{
		buckets: buckets,
		counts:  make([]uint64, len(buckets)+1),
	}
}

func (h *Histogram) Observe(value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	h.sum += value
	h.count++
	
	// Find bucket
	for i, bucket := range h.buckets {
		if value <= bucket {
			h.counts[i]++
			return
		}
	}
	h.counts[len(h.counts)-1]++ // Overflow bucket
}

func (h *Histogram) Snapshot() map[string]interface{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	return map[string]interface{}{
		"buckets": h.buckets,
		"counts":  h.counts,
		"sum":     h.sum,
		"count":   h.count,
	}
}

// Summary implementation

func NewSummary(maxSamples int) *Summary {
	return &Summary{
		values:     make([]float64, 0, maxSamples),
		maxSamples: maxSamples,
	}
}

func (s *Summary) Observe(value float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.sum += value
	s.count++
	
	if len(s.values) < s.maxSamples {
		s.values = append(s.values, value)
	} else {
		// Random replacement
		idx := int(time.Now().UnixNano() % int64(s.maxSamples))
		s.values[idx] = value
	}
}

func (s *Summary) Snapshot() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	quantiles := calculateQuantiles(s.values, []float64{0.5, 0.9, 0.95, 0.99})
	
	return map[string]interface{}{
		"quantiles": quantiles,
		"sum":       s.sum,
		"count":     s.count,
		"avg":       s.sum / float64(s.count),
	}
}

// Helper functions

func defaultBuckets() []float64 {
	return []float64{
		0.001, 0.002, 0.005, 0.01, 0.02, 0.05, 0.1,
		0.2, 0.5, 1.0, 2.0, 5.0, 10.0, 20.0, 50.0, 100.0,
	}
}

func calculateQuantiles(values []float64, quantiles []float64) map[float64]float64 {
	if len(values) == 0 {
		return nil
	}
	
	// Simple implementation - in production use better algorithm
	result := make(map[float64]float64)
	for _, q := range quantiles {
		idx := int(float64(len(values)) * q)
		if idx >= len(values) {
			idx = len(values) - 1
		}
		result[q] = values[idx]
	}
	
	return result
}