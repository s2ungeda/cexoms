package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileRotator handles file rotation based on size and time
type FileRotator struct {
	mu          sync.Mutex
	filename    string
	maxSize     int64
	currentSize int64
	rotateTime  time.Time
}

// NewFileRotator creates a new file rotator
func NewFileRotator(filename string, maxSize int64) *FileRotator {
	return &FileRotator{
		filename:   filename,
		maxSize:    maxSize,
		rotateTime: time.Now().Add(24 * time.Hour),
	}
}

// ShouldRotate checks if file should be rotated
func (fr *FileRotator) ShouldRotate() bool {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	// Check size
	if fr.currentSize >= fr.maxSize {
		return true
	}

	// Check time (daily rotation)
	if time.Now().After(fr.rotateTime) {
		return true
	}

	// Check actual file size
	if info, err := os.Stat(fr.filename); err == nil {
		if info.Size() >= fr.maxSize {
			return true
		}
		fr.currentSize = info.Size()
	}

	return false
}

// Rotate performs file rotation
func (fr *FileRotator) Rotate() error {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	// Generate new filename
	timestamp := time.Now().Format("20060102_150405")
	ext := filepath.Ext(fr.filename)
	base := fr.filename[:len(fr.filename)-len(ext)]
	newFilename := fmt.Sprintf("%s_%s%s", base, timestamp, ext)

	// Rename current file
	if err := os.Rename(fr.filename, newFilename); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to rotate file: %w", err)
		}
	}

	// Reset counters
	fr.currentSize = 0
	fr.rotateTime = time.Now().Add(24 * time.Hour)

	return nil
}

// UpdateSize updates the current file size
func (fr *FileRotator) UpdateSize(bytes int64) {
	fr.mu.Lock()
	fr.currentSize += bytes
	fr.mu.Unlock()
}

// RotationManager manages multiple file rotators
type RotationManager struct {
	mu       sync.RWMutex
	rotators map[string]*FileRotator
	config   RotationConfig
}

// RotationConfig holds rotation configuration
type RotationConfig struct {
	MaxFileSize     int64         // Max size per file
	RotationPeriod  time.Duration // How often to rotate
	CompressionAge  time.Duration // Compress files older than this
	RetentionPeriod time.Duration // Delete files older than this
}

// NewRotationManager creates a new rotation manager
func NewRotationManager(config RotationConfig) *RotationManager {
	rm := &RotationManager{
		rotators: make(map[string]*FileRotator),
		config:   config,
	}

	// Start background tasks
	go rm.rotationLoop()
	go rm.compressionLoop()
	go rm.cleanupLoop()

	return rm
}

// GetRotator returns a rotator for a specific file
func (rm *RotationManager) GetRotator(filename string) *FileRotator {
	rm.mu.RLock()
	rotator, exists := rm.rotators[filename]
	rm.mu.RUnlock()

	if exists {
		return rotator
	}

	// Create new rotator
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Double-check
	if rotator, exists = rm.rotators[filename]; exists {
		return rotator
	}

	rotator = NewFileRotator(filename, rm.config.MaxFileSize)
	rm.rotators[filename] = rotator
	return rotator
}

// CheckAndRotate checks if a file needs rotation and rotates if necessary
func (rm *RotationManager) CheckAndRotate(filename string) error {
	rotator := rm.GetRotator(filename)
	
	if rotator.ShouldRotate() {
		return rotator.Rotate()
	}
	
	return nil
}

// Background loops

func (rm *RotationManager) rotationLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rm.mu.RLock()
		filenames := make([]string, 0, len(rm.rotators))
		for filename := range rm.rotators {
			filenames = append(filenames, filename)
		}
		rm.mu.RUnlock()

		for _, filename := range filenames {
			rm.CheckAndRotate(filename)
		}
	}
}

func (rm *RotationManager) compressionLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		// Compress old files
		// Implementation depends on file patterns and compression needs
	}
}

func (rm *RotationManager) cleanupLoop() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		// Delete old files based on retention period
		// Implementation depends on file patterns and retention policy
	}
}