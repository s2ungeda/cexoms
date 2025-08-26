package security

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// TamperProofLogger provides tamper-proof audit logging
type TamperProofLogger struct {
	mu              sync.RWMutex
	auditLogger     *AuditLogger
	encryptionMgr   *EncryptionManager
	logChain        []*LogEntry
	currentBlock    *LogBlock
	blockSize       int
	secretKey       []byte
	verificationKey []byte
	blockStorage    BlockStorage
}

// LogEntry represents a single log entry
type LogEntry struct {
	ID           string                 `json:"id"`
	Timestamp    time.Time              `json:"timestamp"`
	EventType    string                 `json:"event_type"`
	UserID       string                 `json:"user_id,omitempty"`
	Action       string                 `json:"action"`
	Resource     string                 `json:"resource,omitempty"`
	Result       string                 `json:"result"`
	Details      map[string]interface{} `json:"details,omitempty"`
	Hash         string                 `json:"hash"`
	PreviousHash string                 `json:"previous_hash"`
	Signature    string                 `json:"signature"`
}

// LogBlock represents a block of log entries
type LogBlock struct {
	ID            string      `json:"id"`
	BlockNumber   int         `json:"block_number"`
	Timestamp     time.Time   `json:"timestamp"`
	Entries       []*LogEntry `json:"entries"`
	MerkleRoot    string      `json:"merkle_root"`
	PreviousBlock string      `json:"previous_block"`
	BlockHash     string      `json:"block_hash"`
	Sealed        bool        `json:"sealed"`
}

// BlockStorage interface for storing sealed blocks
type BlockStorage interface {
	StoreBlock(block *LogBlock) error
	GetBlock(blockID string) (*LogBlock, error)
	GetLatestBlock() (*LogBlock, error)
	VerifyBlockChain() (bool, error)
}

// VerificationResult represents log verification result
type VerificationResult struct {
	Valid          bool
	BlocksVerified int
	EntriesVerified int
	InvalidBlocks  []string
	InvalidEntries []string
	Details        string
}

// NewTamperProofLogger creates a new tamper-proof logger
func NewTamperProofLogger(auditLogger *AuditLogger, encryptionMgr *EncryptionManager, blockStorage BlockStorage) (*TamperProofLogger, error) {
	// Generate keys for HMAC
	secretKey := make([]byte, 32)
	if _, err := rand.Read(secretKey); err != nil {
		return nil, fmt.Errorf("failed to generate secret key: %w", err)
	}
	
	verificationKey := make([]byte, 32)
	if _, err := rand.Read(verificationKey); err != nil {
		return nil, fmt.Errorf("failed to generate verification key: %w", err)
	}
	
	tpl := &TamperProofLogger{
		auditLogger:     auditLogger,
		encryptionMgr:   encryptionMgr,
		logChain:        make([]*LogEntry, 0),
		blockSize:       100, // Seal block after 100 entries
		secretKey:       secretKey,
		verificationKey: verificationKey,
		blockStorage:    blockStorage,
	}
	
	// Initialize first block
	tpl.currentBlock = &LogBlock{
		ID:          generateBlockID(),
		BlockNumber: 1,
		Timestamp:   time.Now(),
		Entries:     make([]*LogEntry, 0),
		Sealed:      false,
	}
	
	// Load latest block if exists
	if latestBlock, err := blockStorage.GetLatestBlock(); err == nil && latestBlock != nil {
		tpl.currentBlock.BlockNumber = latestBlock.BlockNumber + 1
		tpl.currentBlock.PreviousBlock = latestBlock.BlockHash
	}
	
	return tpl, nil
}

// LogSecurityEvent logs a security event with tamper protection
func (tpl *TamperProofLogger) LogSecurityEvent(eventType, userID, action, resource, result string, details map[string]interface{}) error {
	tpl.mu.Lock()
	defer tpl.mu.Unlock()
	
	// Create log entry
	entry := &LogEntry{
		ID:        generateEntryID(),
		Timestamp: time.Now(),
		EventType: eventType,
		UserID:    userID,
		Action:    action,
		Resource:  resource,
		Result:    result,
		Details:   details,
	}
	
	// Set previous hash
	if len(tpl.logChain) > 0 {
		entry.PreviousHash = tpl.logChain[len(tpl.logChain)-1].Hash
	} else if tpl.currentBlock.PreviousBlock != "" {
		entry.PreviousHash = tpl.currentBlock.PreviousBlock
	} else {
		entry.PreviousHash = "GENESIS"
	}
	
	// Calculate entry hash
	entry.Hash = tpl.calculateEntryHash(entry)
	
	// Sign the entry
	entry.Signature = tpl.signEntry(entry)
	
	// Add to current block
	tpl.currentBlock.Entries = append(tpl.currentBlock.Entries, entry)
	tpl.logChain = append(tpl.logChain, entry)
	
	// Check if block should be sealed
	if len(tpl.currentBlock.Entries) >= tpl.blockSize {
		if err := tpl.sealBlock(); err != nil {
			return fmt.Errorf("failed to seal block: %w", err)
		}
	}
	
	// Also log to regular audit logger
	tpl.auditLogger.LogSecurityEvent(context.Background(), eventType, "high", details)
	
	return nil
}

// sealBlock seals the current block and starts a new one
func (tpl *TamperProofLogger) sealBlock() error {
	if tpl.currentBlock.Sealed {
		return fmt.Errorf("block already sealed")
	}
	
	// Calculate Merkle root
	tpl.currentBlock.MerkleRoot = tpl.calculateMerkleRoot(tpl.currentBlock.Entries)
	
	// Calculate block hash
	tpl.currentBlock.BlockHash = tpl.calculateBlockHash(tpl.currentBlock)
	
	// Mark as sealed
	tpl.currentBlock.Sealed = true
	
	// Store sealed block
	if err := tpl.blockStorage.StoreBlock(tpl.currentBlock); err != nil {
		return fmt.Errorf("failed to store block: %w", err)
	}
	
	// Create new block
	newBlock := &LogBlock{
		ID:            generateBlockID(),
		BlockNumber:   tpl.currentBlock.BlockNumber + 1,
		Timestamp:     time.Now(),
		Entries:       make([]*LogEntry, 0),
		PreviousBlock: tpl.currentBlock.BlockHash,
		Sealed:        false,
	}
	
	tpl.currentBlock = newBlock
	
	return nil
}

// calculateEntryHash calculates hash for a log entry
func (tpl *TamperProofLogger) calculateEntryHash(entry *LogEntry) string {
	data := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%v",
		entry.ID,
		entry.Timestamp.Format(time.RFC3339Nano),
		entry.EventType,
		entry.UserID,
		entry.Action,
		entry.Resource,
		entry.Result,
		entry.Details,
	)
	
	if entry.PreviousHash != "" {
		data += "|" + entry.PreviousHash
	}
	
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// signEntry creates HMAC signature for entry
func (tpl *TamperProofLogger) signEntry(entry *LogEntry) string {
	h := hmac.New(sha256.New, tpl.secretKey)
	h.Write([]byte(entry.Hash))
	return hex.EncodeToString(h.Sum(nil))
}

// verifyEntrySignature verifies entry signature
func (tpl *TamperProofLogger) verifyEntrySignature(entry *LogEntry) bool {
	h := hmac.New(sha256.New, tpl.secretKey)
	h.Write([]byte(entry.Hash))
	expectedSignature := hex.EncodeToString(h.Sum(nil))
	return hmac.Equal([]byte(expectedSignature), []byte(entry.Signature))
}

// calculateMerkleRoot calculates Merkle root for entries
func (tpl *TamperProofLogger) calculateMerkleRoot(entries []*LogEntry) string {
	if len(entries) == 0 {
		return ""
	}
	
	// Get all entry hashes
	hashes := make([]string, len(entries))
	for i, entry := range entries {
		hashes[i] = entry.Hash
	}
	
	// Build Merkle tree
	for len(hashes) > 1 {
		newLevel := make([]string, 0)
		
		for i := 0; i < len(hashes); i += 2 {
			var combined string
			if i+1 < len(hashes) {
				combined = hashes[i] + hashes[i+1]
			} else {
				combined = hashes[i] + hashes[i] // Duplicate last hash if odd number
			}
			
			hash := sha256.Sum256([]byte(combined))
			newLevel = append(newLevel, hex.EncodeToString(hash[:]))
		}
		
		hashes = newLevel
	}
	
	return hashes[0]
}

// calculateBlockHash calculates hash for a block
func (tpl *TamperProofLogger) calculateBlockHash(block *LogBlock) string {
	data := fmt.Sprintf("%s|%d|%s|%s|%s",
		block.ID,
		block.BlockNumber,
		block.Timestamp.Format(time.RFC3339Nano),
		block.MerkleRoot,
		block.PreviousBlock,
	)
	
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// VerifyLogIntegrity verifies the integrity of the log chain
func (tpl *TamperProofLogger) VerifyLogIntegrity() (*VerificationResult, error) {
	tpl.mu.RLock()
	defer tpl.mu.RUnlock()
	
	result := &VerificationResult{
		Valid:          true,
		InvalidBlocks:  make([]string, 0),
		InvalidEntries: make([]string, 0),
	}
	
	// Verify blockchain integrity
	blockValid, err := tpl.blockStorage.VerifyBlockChain()
	if err != nil {
		return nil, fmt.Errorf("failed to verify blockchain: %w", err)
	}
	
	if !blockValid {
		result.Valid = false
		result.Details = "Blockchain verification failed"
	}
	
	// Verify current block entries
	previousHash := ""
	if tpl.currentBlock.PreviousBlock != "" {
		previousHash = tpl.currentBlock.PreviousBlock
	} else {
		previousHash = "GENESIS"
	}
	
	for _, entry := range tpl.currentBlock.Entries {
		// Verify hash chain
		if entry.PreviousHash != previousHash {
			result.Valid = false
			result.InvalidEntries = append(result.InvalidEntries, entry.ID)
		}
		
		// Verify entry hash
		calculatedHash := tpl.calculateEntryHash(entry)
		if calculatedHash != entry.Hash {
			result.Valid = false
			result.InvalidEntries = append(result.InvalidEntries, entry.ID)
		}
		
		// Verify signature
		if !tpl.verifyEntrySignature(entry) {
			result.Valid = false
			result.InvalidEntries = append(result.InvalidEntries, entry.ID)
		}
		
		previousHash = entry.Hash
		result.EntriesVerified++
	}
	
	// Verify Merkle root if entries exist
	if len(tpl.currentBlock.Entries) > 0 && tpl.currentBlock.MerkleRoot != "" {
		calculatedRoot := tpl.calculateMerkleRoot(tpl.currentBlock.Entries)
		if calculatedRoot != tpl.currentBlock.MerkleRoot {
			result.Valid = false
			result.InvalidBlocks = append(result.InvalidBlocks, tpl.currentBlock.ID)
		}
	}
	
	if result.Valid {
		result.Details = "All logs verified successfully"
	} else {
		result.Details = fmt.Sprintf("Verification failed: %d invalid blocks, %d invalid entries",
			len(result.InvalidBlocks), len(result.InvalidEntries))
	}
	
	return result, nil
}

// ExportLogs exports logs for external verification
func (tpl *TamperProofLogger) ExportLogs(startTime, endTime time.Time) (*LogExport, error) {
	tpl.mu.RLock()
	defer tpl.mu.RUnlock()
	
	export := &LogExport{
		ExportID:         generateExportID(),
		ExportTime:       time.Now(),
		StartTime:        startTime,
		EndTime:          endTime,
		Entries:          make([]*LogEntry, 0),
		VerificationKeys: make(map[string]string),
	}
	
	// Export entries within time range
	for _, entry := range tpl.logChain {
		if entry.Timestamp.After(startTime) && entry.Timestamp.Before(endTime) {
			export.Entries = append(export.Entries, entry)
		}
	}
	
	// Include verification info
	export.VerificationKeys["algorithm"] = "HMAC-SHA256"
	export.VerificationKeys["merkle_algorithm"] = "SHA256"
	
	// Sign the export
	exportData, _ := json.Marshal(export)
	h := hmac.New(sha256.New, tpl.verificationKey)
	h.Write(exportData)
	export.Signature = hex.EncodeToString(h.Sum(nil))
	
	return export, nil
}

// SearchLogs searches tamper-proof logs
func (tpl *TamperProofLogger) SearchLogs(filter LogFilter) ([]*LogEntry, error) {
	tpl.mu.RLock()
	defer tpl.mu.RUnlock()
	
	results := make([]*LogEntry, 0)
	
	for _, entry := range tpl.logChain {
		if tpl.matchesFilter(entry, filter) {
			results = append(results, entry)
		}
	}
	
	return results, nil
}

// matchesFilter checks if entry matches filter criteria
func (tpl *TamperProofLogger) matchesFilter(entry *LogEntry, filter LogFilter) bool {
	// Check time range
	if !filter.StartTime.IsZero() && entry.Timestamp.Before(filter.StartTime) {
		return false
	}
	if !filter.EndTime.IsZero() && entry.Timestamp.After(filter.EndTime) {
		return false
	}
	
	// Check event type
	if filter.EventType != "" && entry.EventType != filter.EventType {
		return false
	}
	
	// Check user
	if filter.UserID != "" && entry.UserID != filter.UserID {
		return false
	}
	
	// Check action
	if filter.Action != "" && entry.Action != filter.Action {
		return false
	}
	
	// Check result
	if filter.Result != "" && entry.Result != filter.Result {
		return false
	}
	
	return true
}

// GetCurrentBlockInfo returns info about current block
func (tpl *TamperProofLogger) GetCurrentBlockInfo() *BlockInfo {
	tpl.mu.RLock()
	defer tpl.mu.RUnlock()
	
	return &BlockInfo{
		BlockNumber: tpl.currentBlock.BlockNumber,
		EntryCount:  len(tpl.currentBlock.Entries),
		MaxEntries:  tpl.blockSize,
		StartTime:   tpl.currentBlock.Timestamp,
		Sealed:      tpl.currentBlock.Sealed,
	}
}

// ForceBlockSeal forces sealing of current block
func (tpl *TamperProofLogger) ForceBlockSeal() error {
	tpl.mu.Lock()
	defer tpl.mu.Unlock()
	
	if len(tpl.currentBlock.Entries) == 0 {
		return fmt.Errorf("cannot seal empty block")
	}
	
	return tpl.sealBlock()
}

// Types

// LogExport represents exported logs
type LogExport struct {
	ExportID         string            `json:"export_id"`
	ExportTime       time.Time         `json:"export_time"`
	StartTime        time.Time         `json:"start_time"`
	EndTime          time.Time         `json:"end_time"`
	Entries          []*LogEntry       `json:"entries"`
	VerificationKeys map[string]string `json:"verification_keys"`
	Signature        string            `json:"signature"`
}

// LogFilter for searching logs
type LogFilter struct {
	StartTime time.Time
	EndTime   time.Time
	EventType string
	UserID    string
	Action    string
	Result    string
}

// BlockInfo provides information about a block
type BlockInfo struct {
	BlockNumber int
	EntryCount  int
	MaxEntries  int
	StartTime   time.Time
	Sealed      bool
}

// Helper functions
func generateBlockID() string {
	return fmt.Sprintf("BLK_%d_%s", time.Now().UnixNano(), generateRandomString(6))
}

func generateEntryID() string {
	return fmt.Sprintf("LOG_%d_%s", time.Now().UnixNano(), generateRandomString(8))
}

func generateExportID() string {
	return fmt.Sprintf("EXP_%d_%s", time.Now().Unix(), generateRandomString(6))
}