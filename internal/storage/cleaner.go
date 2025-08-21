package storage

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Cleaner handles cleanup and maintenance of storage files
type Cleaner struct {
	config StorageConfig
}

// NewCleaner creates a new storage cleaner
func NewCleaner(config StorageConfig) *Cleaner {
	return &Cleaner{
		config: config,
	}
}

// CleanupOldFiles removes files older than retention period
func (c *Cleaner) CleanupOldFiles() error {
	if c.config.RetentionDays <= 0 {
		return nil // No cleanup if retention not set
	}

	cutoffTime := time.Now().AddDate(0, 0, -c.config.RetentionDays)
	
	return filepath.Walk(c.config.BasePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check if file is a storage file
		if !strings.HasSuffix(path, ".jsonl") && !strings.HasSuffix(path, ".jsonl.gz") {
			return nil
		}

		// Check file age
		if info.ModTime().Before(cutoffTime) {
			fmt.Printf("Removing old file: %s (modified: %s)\n", path, info.ModTime().Format("2006-01-02"))
			if err := os.Remove(path); err != nil {
				fmt.Printf("Failed to remove %s: %v\n", path, err)
			}
		}

		return nil
	})
}

// CompressOldFiles compresses files older than specified days
func (c *Cleaner) CompressOldFiles(daysOld int) error {
	if !c.config.CompressionEnabled {
		return nil
	}

	cutoffTime := time.Now().AddDate(0, 0, -daysOld)
	
	return filepath.Walk(c.config.BasePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Skip directories and already compressed files
		if info.IsDir() || strings.HasSuffix(path, ".gz") {
			return nil
		}

		// Check if file is a storage file
		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		// Check file age
		if info.ModTime().Before(cutoffTime) {
			if err := c.compressFile(path); err != nil {
				fmt.Printf("Failed to compress %s: %v\n", path, err)
			}
		}

		return nil
	})
}

// compressFile compresses a single file
func (c *Cleaner) compressFile(filepath string) error {
	// Open source file
	source, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer source.Close()

	// Create compressed file
	destPath := filepath + ".gz"
	dest, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer dest.Close()

	// Create gzip writer
	gz := gzip.NewWriter(dest)
	gz.Name = filepath
	defer gz.Close()

	// Copy data
	if _, err := io.Copy(gz, source); err != nil {
		os.Remove(destPath) // Clean up on error
		return err
	}

	// Close gzip writer to flush data
	if err := gz.Close(); err != nil {
		os.Remove(destPath)
		return err
	}

	// Close destination file
	if err := dest.Close(); err != nil {
		os.Remove(destPath)
		return err
	}

	// Remove original file
	return os.Remove(filepath)
}

// ArchiveAccount archives all data for a specific account
func (c *Cleaner) ArchiveAccount(account string, archivePath string) error {
	accountPath := filepath.Join(c.config.BasePath, account)
	
	// Check if account directory exists
	if _, err := os.Stat(accountPath); os.IsNotExist(err) {
		return fmt.Errorf("account directory not found: %s", account)
	}

	// Create archive directory
	if err := os.MkdirAll(archivePath, 0755); err != nil {
		return fmt.Errorf("failed to create archive directory: %w", err)
	}

	// Create tar.gz archive
	archiveFile := filepath.Join(archivePath, fmt.Sprintf("%s_%s.tar.gz", account, time.Now().Format("20060102_150405")))
	
	cmd := fmt.Sprintf("tar -czf %s -C %s %s", archiveFile, c.config.BasePath, account)
	if err := exec.Command("bash", "-c", cmd).Run(); err != nil {
		return fmt.Errorf("failed to create archive: %w", err)
	}

	fmt.Printf("Created archive: %s\n", archiveFile)
	return nil
}

// GetStorageStats returns storage statistics
func (c *Cleaner) GetStorageStats() (*StorageStats, error) {
	stats := &StorageStats{
		Accounts:     make(map[string]*AccountStats),
		CheckedAt:    time.Now(),
	}

	// Walk through storage directory
	err := filepath.Walk(c.config.BasePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Skip base directory
		if path == c.config.BasePath {
			return nil
		}

		// Parse path to get account and storage type
		rel, err := filepath.Rel(c.config.BasePath, path)
		if err != nil {
			return nil
		}

		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) == 0 {
			return nil
		}

		account := parts[0]
		
		// Initialize account stats if needed
		if _, exists := stats.Accounts[account]; !exists {
			stats.Accounts[account] = &AccountStats{
				StorageTypes: make(map[string]*TypeStats),
			}
		}

		// Process files
		if !info.IsDir() && (strings.HasSuffix(path, ".jsonl") || strings.HasSuffix(path, ".jsonl.gz")) {
			stats.TotalFiles++
			stats.TotalSize += info.Size()
			stats.Accounts[account].TotalFiles++
			stats.Accounts[account].TotalSize += info.Size()

			// Get storage type if available
			if len(parts) > 1 {
				storageType := parts[1]
				if _, exists := stats.Accounts[account].StorageTypes[storageType]; !exists {
					stats.Accounts[account].StorageTypes[storageType] = &TypeStats{}
				}
				
				typeStats := stats.Accounts[account].StorageTypes[storageType]
				typeStats.FileCount++
				typeStats.TotalSize += info.Size()
				
				// Track oldest and newest files
				if typeStats.OldestFile.IsZero() || info.ModTime().Before(typeStats.OldestFile) {
					typeStats.OldestFile = info.ModTime()
				}
				if info.ModTime().After(typeStats.NewestFile) {
					typeStats.NewestFile = info.ModTime()
				}

				// Track compression
				if strings.HasSuffix(path, ".gz") {
					typeStats.CompressedFiles++
				}
			}
		}

		return nil
	})

	return stats, err
}

// PruneEmptyDirectories removes empty directories
func (c *Cleaner) PruneEmptyDirectories() error {
	// Walk through directories in reverse order (deepest first)
	var dirs []string
	
	err := filepath.Walk(c.config.BasePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		
		if info.IsDir() && path != c.config.BasePath {
			dirs = append(dirs, path)
		}
		
		return nil
	})
	
	if err != nil {
		return err
	}

	// Process directories in reverse order
	for i := len(dirs) - 1; i >= 0; i-- {
		dir := dirs[i]
		
		// Check if directory is empty
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		
		if len(entries) == 0 {
			fmt.Printf("Removing empty directory: %s\n", dir)
			os.Remove(dir)
		}
	}

	return nil
}

// StorageStats represents storage statistics
type StorageStats struct {
	TotalSize  int64                    `json:"total_size"`
	TotalFiles int                      `json:"total_files"`
	Accounts   map[string]*AccountStats `json:"accounts"`
	CheckedAt  time.Time                `json:"checked_at"`
}

// AccountStats represents statistics for an account
type AccountStats struct {
	TotalSize    int64                 `json:"total_size"`
	TotalFiles   int                   `json:"total_files"`
	StorageTypes map[string]*TypeStats `json:"storage_types"`
}

// TypeStats represents statistics for a storage type
type TypeStats struct {
	FileCount       int       `json:"file_count"`
	TotalSize       int64     `json:"total_size"`
	CompressedFiles int       `json:"compressed_files"`
	OldestFile      time.Time `json:"oldest_file"`
	NewestFile      time.Time `json:"newest_file"`
}

// Add missing import
import "os/exec"