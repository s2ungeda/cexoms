package benchmark

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestData represents sample data for file I/O tests
type TestData struct {
	ID        string    `json:"id"`
	Symbol    string    `json:"symbol"`
	Price     float64   `json:"price"`
	Quantity  float64   `json:"quantity"`
	Timestamp time.Time `json:"timestamp"`
	Exchange  string    `json:"exchange"`
	OrderType string    `json:"order_type"`
	Status    string    `json:"status"`
}

// BenchmarkFileWrite tests file write performance
func BenchmarkFileWrite(b *testing.B) {
	// Test different write methods
	testData := generateTestData(1000)
	
	b.Run("DirectWrite", func(b *testing.B) {
		filename := filepath.Join(b.TempDir(), "direct_write.jsonl")
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			file, err := os.Create(filename)
			if err != nil {
				b.Fatal(err)
			}
			
			for _, data := range testData {
				bytes, _ := json.Marshal(data)
				file.Write(bytes)
				file.Write([]byte("\n"))
			}
			
			file.Close()
		}
		
		// Report file size
		info, _ := os.Stat(filename)
		b.ReportMetric(float64(info.Size())/(1024*1024), "MB")
	})
	
	b.Run("BufferedWrite", func(b *testing.B) {
		filename := filepath.Join(b.TempDir(), "buffered_write.jsonl")
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			file, err := os.Create(filename)
			if err != nil {
				b.Fatal(err)
			}
			
			writer := bufio.NewWriterSize(file, 64*1024) // 64KB buffer
			
			for _, data := range testData {
				bytes, _ := json.Marshal(data)
				writer.Write(bytes)
				writer.Write([]byte("\n"))
			}
			
			writer.Flush()
			file.Close()
		}
	})
	
	b.Run("AsyncWrite", func(b *testing.B) {
		filename := filepath.Join(b.TempDir(), "async_write.jsonl")
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			file, err := os.Create(filename)
			if err != nil {
				b.Fatal(err)
			}
			
			// Channel for async writes
			writeChan := make(chan []byte, 100)
			done := make(chan bool)
			
			// Writer goroutine
			go func() {
				writer := bufio.NewWriterSize(file, 64*1024)
				for data := range writeChan {
					writer.Write(data)
				}
				writer.Flush()
				file.Close()
				done <- true
			}()
			
			// Send data
			for _, data := range testData {
				bytes, _ := json.Marshal(data)
				bytes = append(bytes, '\n')
				writeChan <- bytes
			}
			
			close(writeChan)
			<-done
		}
	})
}

// BenchmarkFileRead tests file read performance
func BenchmarkFileRead(b *testing.B) {
	// Create test file
	testData := generateTestData(10000)
	filename := filepath.Join(b.TempDir(), "test_data.jsonl")
	createTestFile(filename, testData)
	
	b.Run("DirectRead", func(b *testing.B) {
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			file, err := os.Open(filename)
			if err != nil {
				b.Fatal(err)
			}
			
			content, _ := io.ReadAll(file)
			_ = content
			
			file.Close()
		}
	})
	
	b.Run("BufferedRead", func(b *testing.B) {
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			file, err := os.Open(filename)
			if err != nil {
				b.Fatal(err)
			}
			
			scanner := bufio.NewScanner(file)
			count := 0
			for scanner.Scan() {
				line := scanner.Bytes()
				_ = line
				count++
			}
			
			file.Close()
		}
	})
	
	b.Run("JSONStreamRead", func(b *testing.B) {
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			file, err := os.Open(filename)
			if err != nil {
				b.Fatal(err)
			}
			
			decoder := json.NewDecoder(file)
			count := 0
			for {
				var data TestData
				if err := decoder.Decode(&data); err == io.EOF {
					break
				} else if err != nil {
					// Handle newline-delimited JSON
					continue
				}
				count++
			}
			
			file.Close()
		}
	})
}

// BenchmarkFileAppend tests append performance
func BenchmarkFileAppend(b *testing.B) {
	filename := filepath.Join(b.TempDir(), "append_test.jsonl")
	
	// Create initial file
	initialData := generateTestData(1000)
	createTestFile(filename, initialData)
	
	// Test data for appending
	appendData := generateTestData(100)
	
	b.Run("SimpleAppend", func(b *testing.B) {
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			file, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				b.Fatal(err)
			}
			
			for _, data := range appendData {
				bytes, _ := json.Marshal(data)
				file.Write(bytes)
				file.Write([]byte("\n"))
			}
			
			file.Close()
		}
	})
	
	b.Run("BufferedAppend", func(b *testing.B) {
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			file, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				b.Fatal(err)
			}
			
			writer := bufio.NewWriterSize(file, 64*1024)
			
			for _, data := range appendData {
				bytes, _ := json.Marshal(data)
				writer.Write(bytes)
				writer.Write([]byte("\n"))
			}
			
			writer.Flush()
			file.Close()
		}
	})
}

// BenchmarkFileRotation tests file rotation performance
func BenchmarkFileRotation(b *testing.B) {
	dir := b.TempDir()
	maxSize := int64(10 * 1024 * 1024) // 10MB
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		currentFile := filepath.Join(dir, "current.log")
		file, _ := os.Create(currentFile)
		writer := bufio.NewWriter(file)
		
		fileSize := int64(0)
		rotationCount := 0
		
		// Write until we need to rotate
		for j := 0; j < 100000; j++ {
			data := fmt.Sprintf("Log entry %d: %s\n", j, time.Now())
			n, _ := writer.WriteString(data)
			fileSize += int64(n)
			
			// Check if rotation needed
			if fileSize >= maxSize {
				writer.Flush()
				file.Close()
				
				// Rotate file
				rotatedFile := filepath.Join(dir, fmt.Sprintf("rotated_%d.log", rotationCount))
				os.Rename(currentFile, rotatedFile)
				
				// Create new file
				file, _ = os.Create(currentFile)
				writer = bufio.NewWriter(file)
				fileSize = 0
				rotationCount++
			}
		}
		
		writer.Flush()
		file.Close()
		
		b.ReportMetric(float64(rotationCount), "rotations")
	}
}

// BenchmarkConcurrentFileAccess tests concurrent file access patterns
func BenchmarkConcurrentFileAccess(b *testing.B) {
	b.Run("MultipleWriters", func(b *testing.B) {
		dir := b.TempDir()
		numWriters := 10
		
		b.ResetTimer()
		
		var wg sync.WaitGroup
		for i := 0; i < numWriters; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				
				filename := filepath.Join(dir, fmt.Sprintf("writer_%d.jsonl", id))
				file, _ := os.Create(filename)
				writer := bufio.NewWriter(file)
				
				for j := 0; j < b.N/numWriters; j++ {
					data := TestData{
						ID:        fmt.Sprintf("%d-%d", id, j),
						Symbol:    "BTCUSDT",
						Price:     42000.0 + float64(j),
						Timestamp: time.Now(),
					}
					bytes, _ := json.Marshal(data)
					writer.Write(bytes)
					writer.Write([]byte("\n"))
				}
				
				writer.Flush()
				file.Close()
			}(i)
		}
		
		wg.Wait()
	})
	
	b.Run("WriterReaders", func(b *testing.B) {
		filename := filepath.Join(b.TempDir(), "shared.jsonl")
		
		// Create initial file
		file, _ := os.Create(filename)
		file.Close()
		
		b.ResetTimer()
		
		var wg sync.WaitGroup
		
		// Writer
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			file, _ := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0644)
			writer := bufio.NewWriter(file)
			
			for i := 0; i < b.N; i++ {
				data := TestData{
					ID:        fmt.Sprintf("write-%d", i),
					Symbol:    "BTCUSDT",
					Price:     42000.0 + float64(i),
					Timestamp: time.Now(),
				}
				bytes, _ := json.Marshal(data)
				writer.Write(bytes)
				writer.Write([]byte("\n"))
				
				if i%100 == 0 {
					writer.Flush()
				}
			}
			
			writer.Flush()
			file.Close()
		}()
		
		// Readers
		numReaders := 5
		for i := 0; i < numReaders; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				
				time.Sleep(10 * time.Millisecond) // Let writer start
				
				for j := 0; j < 10; j++ {
					file, err := os.Open(filename)
					if err != nil {
						continue
					}
					
					scanner := bufio.NewScanner(file)
					count := 0
					for scanner.Scan() {
						count++
					}
					
					file.Close()
					time.Sleep(100 * time.Millisecond)
				}
			}(i)
		}
		
		wg.Wait()
	})
}

// BenchmarkFileSync tests fsync performance
func BenchmarkFileSync(b *testing.B) {
	testData := generateTestData(100)
	
	b.Run("WithoutSync", func(b *testing.B) {
		filename := filepath.Join(b.TempDir(), "no_sync.jsonl")
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			file, _ := os.Create(filename)
			
			for _, data := range testData {
				bytes, _ := json.Marshal(data)
				file.Write(bytes)
				file.Write([]byte("\n"))
			}
			
			file.Close()
		}
	})
	
	b.Run("WithSync", func(b *testing.B) {
		filename := filepath.Join(b.TempDir(), "with_sync.jsonl")
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			file, _ := os.Create(filename)
			
			for _, data := range testData {
				bytes, _ := json.Marshal(data)
				file.Write(bytes)
				file.Write([]byte("\n"))
			}
			
			file.Sync() // Force flush to disk
			file.Close()
		}
	})
	
	b.Run("PeriodicSync", func(b *testing.B) {
		filename := filepath.Join(b.TempDir(), "periodic_sync.jsonl")
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			file, _ := os.Create(filename)
			
			for j, data := range testData {
				bytes, _ := json.Marshal(data)
				file.Write(bytes)
				file.Write([]byte("\n"))
				
				// Sync every 10 writes
				if j%10 == 0 {
					file.Sync()
				}
			}
			
			file.Close()
		}
	})
}

// Helper functions

func generateTestData(count int) []TestData {
	data := make([]TestData, count)
	for i := 0; i < count; i++ {
		data[i] = TestData{
			ID:        fmt.Sprintf("order-%d", i),
			Symbol:    "BTCUSDT",
			Price:     42000.0 + float64(i%1000),
			Quantity:  0.1 + float64(i%10)/100,
			Timestamp: time.Now(),
			Exchange:  "binance",
			OrderType: "LIMIT",
			Status:    "NEW",
		}
	}
	return data
}

func createTestFile(filename string, data []TestData) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	
	writer := bufio.NewWriter(file)
	for _, d := range data {
		bytes, _ := json.Marshal(d)
		writer.Write(bytes)
		writer.Write([]byte("\n"))
	}
	
	return writer.Flush()
}