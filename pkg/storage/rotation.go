package storage

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type LogRotator struct {
	dataDir       string
	retentionDays int
	compressDays  int
}

func NewLogRotator(dataDir string, retentionDays, compressDays int) *LogRotator {
	return &LogRotator{
		dataDir:       dataDir,
		retentionDays: retentionDays,
		compressDays:  compressDays,
	}
}

// RotateLogs performs log rotation, compression, and cleanup
func (lr *LogRotator) RotateLogs() error {
	logsDir := filepath.Join(lr.dataDir, "logs")
	now := time.Now()
	
	// Walk through all log directories
	err := filepath.Walk(logsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Skip if not a file
		if info.IsDir() {
			return nil
		}
		
		// Skip if already compressed
		if filepath.Ext(path) == ".gz" {
			// Check if old compressed file should be deleted
			if now.Sub(info.ModTime()) > time.Duration(lr.retentionDays)*24*time.Hour {
				if err := os.Remove(path); err != nil {
					return fmt.Errorf("failed to remove old compressed file %s: %w", path, err)
				}
			}
			return nil
		}
		
		// Skip if not a log file
		if filepath.Ext(path) != ".jsonl" {
			return nil
		}
		
		fileAge := now.Sub(info.ModTime())
		
		// Delete if older than retention period
		if fileAge > time.Duration(lr.retentionDays)*24*time.Hour {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("failed to remove old file %s: %w", path, err)
			}
			return nil
		}
		
		// Compress if older than compress days
		if fileAge > time.Duration(lr.compressDays)*24*time.Hour {
			if err := lr.compressFile(path); err != nil {
				return fmt.Errorf("failed to compress file %s: %w", path, err)
			}
		}
		
		return nil
	})
	
	return err
}

// compressFile compresses a single file
func (lr *LogRotator) compressFile(path string) error {
	// Open source file
	source, err := os.Open(path)
	if err != nil {
		return err
	}
	defer source.Close()
	
	// Create compressed file
	destPath := path + ".gz"
	dest, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer dest.Close()
	
	// Create gzip writer
	gz := gzip.NewWriter(dest)
	gz.Name = filepath.Base(path)
	gz.ModTime = time.Now()
	defer gz.Close()
	
	// Copy data
	if _, err := io.Copy(gz, source); err != nil {
		return err
	}
	
	// Close gzip writer to flush data
	if err := gz.Close(); err != nil {
		return err
	}
	
	// Close files before removing
	source.Close()
	dest.Close()
	
	// Remove original file
	if err := os.Remove(path); err != nil {
		return err
	}
	
	// Preserve modification time
	info, _ := os.Stat(path + ".gz")
	if info != nil {
		os.Chtimes(destPath, info.ModTime(), info.ModTime())
	}
	
	return nil
}

// CleanOldSnapshots removes old snapshot files
func (lr *LogRotator) CleanOldSnapshots() error {
	snapshotsDir := filepath.Join(lr.dataDir, "snapshots")
	now := time.Now()
	cutoff := now.Add(-time.Duration(lr.retentionDays) * 24 * time.Hour)
	
	return filepath.Walk(snapshotsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Skip directories
		if info.IsDir() {
			// Check if directory is old and empty
			if info.ModTime().Before(cutoff) {
				isEmpty, _ := isDirEmpty(path)
				if isEmpty && path != snapshotsDir {
					os.Remove(path)
				}
			}
			return nil
		}
		
		// Remove old snapshot files
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("failed to remove old snapshot %s: %w", path, err)
			}
		}
		
		return nil
	})
}

// StartRotationSchedule starts automatic log rotation
func (lr *LogRotator) StartRotationSchedule() {
	ticker := time.NewTicker(24 * time.Hour)
	go func() {
		for range ticker.C {
			if err := lr.RotateLogs(); err != nil {
				fmt.Printf("Log rotation error: %v\n", err)
			}
			if err := lr.CleanOldSnapshots(); err != nil {
				fmt.Printf("Snapshot cleanup error: %v\n", err)
			}
		}
	}()
}

// isDirEmpty checks if a directory is empty
func isDirEmpty(dir string) (bool, error) {
	f, err := os.Open(dir)
	if err != nil {
		return false, err
	}
	defer f.Close()
	
	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err
}