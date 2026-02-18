package recall

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hyperengineering/recall/internal/store/migrations"
	"github.com/oklog/ulid/v2"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

const schemaVersion = "2"

// Store manages the local SQLite lore database.
type Store struct {
	db       *sql.DB
	mu       sync.RWMutex
	closed   bool
	path     string
	sourceID string // cached from sync_meta for change_log writes
}

// NewStore opens or creates a local lore store.
func NewStore(path string) (*Store, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create store directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for better concurrent access
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable WAL mode: %w", err)
	}

	store := &Store{db: db, path: path}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	// Cache source_id for change_log writes
	if err := store.loadSourceID(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("load source_id: %w", err)
	}

	return store, nil
}

func (s *Store) migrate() error {
	goose.SetLogger(goose.NopLogger())
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("store: set goose dialect: %w", err)
	}
	if err := goose.Up(s.db, "."); err != nil {
		return fmt.Errorf("store: run migrations: %w", err)
	}

	// Upsert schema version so existing databases get updated
	_, err := s.db.Exec(`
		INSERT INTO metadata (key, value) VALUES ('schema_version', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, schemaVersion)
	if err != nil {
		return err
	}

	return s.initSyncMeta()
}

// initSyncMeta initializes client-specific sync_meta keys if not already present.
// Generates a UUIDv4 source_id that persists across restarts.
func (s *Store) initSyncMeta() error {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM sync_meta WHERE key = 'source_id'").Scan(&count)
	if err != nil {
		return fmt.Errorf("store: check source_id: %w", err)
	}
	if count > 0 {
		return nil // Already initialized
	}

	sourceID, err := generateUUIDv4()
	if err != nil {
		return fmt.Errorf("store: generate source_id: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT OR IGNORE INTO sync_meta (key, value) VALUES
		('source_id', ?),
		('last_push_seq', '0'),
		('last_pull_seq', '0')
	`, sourceID)
	if err != nil {
		return fmt.Errorf("store: init sync meta: %w", err)
	}

	return nil
}

// generateUUIDv4 generates a random UUIDv4 string.
func generateUUIDv4() (string, error) {
	var uuid [16]byte
	if _, err := rand.Read(uuid[:]); err != nil {
		return "", fmt.Errorf("generate uuid: %w", err)
	}
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]), nil
}

// loadSourceID reads source_id from sync_meta and caches it on the Store.
func (s *Store) loadSourceID() error {
	var sourceID string
	err := s.db.QueryRow("SELECT value FROM sync_meta WHERE key = 'source_id'").Scan(&sourceID)
	if err != nil {
		return fmt.Errorf("store: read source_id: %w", err)
	}
	s.sourceID = sourceID
	return nil
}

// SourceID returns the cached source_id for this store.
func (s *Store) SourceID() string {
	return s.sourceID
}

// appendChangeLog inserts a change_log entry within a transaction.
func appendChangeLog(tx *sql.Tx, tableName, entityID, operation string, payload []byte, sourceID string) error {
	createdAt := time.Now().UTC().Format(time.RFC3339)
	var payloadArg any
	if payload != nil {
		payloadArg = string(payload)
	}
	_, err := tx.Exec(`
		INSERT INTO change_log (table_name, entity_id, operation, payload, source_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, tableName, entityID, operation, payloadArg, sourceID, createdAt)
	if err != nil {
		return fmt.Errorf("store: append change_log: %w", err)
	}
	return nil
}

// lorePayloadJSON builds the full entity JSON payload for a lore entry.
// This is the format required by Engram's Recall domain plugin validation.
func lorePayloadJSON(lore *Lore) ([]byte, error) {
	payload := struct {
		ID              string   `json:"id"`
		Content         string   `json:"content"`
		Context         string   `json:"context,omitempty"`
		Category        string   `json:"category"`
		Confidence      float64  `json:"confidence"`
		EmbeddingStatus string   `json:"embedding_status"`
		SourceID        string   `json:"source_id"`
		Sources         []string `json:"sources"`
		ValidationCount int      `json:"validation_count"`
		CreatedAt       string   `json:"created_at"`
		UpdatedAt       string   `json:"updated_at"`
		DeletedAt       *string  `json:"deleted_at"`
		LastValidatedAt *string  `json:"last_validated_at"`
	}{
		ID:              lore.ID,
		Content:         lore.Content,
		Context:         lore.Context,
		Category:        string(lore.Category),
		Confidence:      lore.Confidence,
		EmbeddingStatus: lore.EmbeddingStatus,
		SourceID:        lore.SourceID,
		Sources:         lore.Sources,
		ValidationCount: lore.ValidationCount,
		CreatedAt:       lore.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       lore.UpdatedAt.Format(time.RFC3339),
	}
	if lore.DeletedAt != nil {
		ts := lore.DeletedAt.Format(time.RFC3339)
		payload.DeletedAt = &ts
	}
	if lore.LastValidatedAt != nil {
		ts := lore.LastValidatedAt.Format(time.RFC3339)
		payload.LastValidatedAt = &ts
	}
	if payload.Sources == nil {
		payload.Sources = []string{}
	}
	return json.Marshal(payload)
}

// InsertLore atomically inserts a lore entry and a change_log entry in one transaction.
// This is the primary method for storing new lore (used by Client.Record).
func (s *Store) InsertLore(lore *Lore) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	// Begin transaction
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op if committed

	var embeddingBlob []byte
	if len(lore.Embedding) > 0 {
		embeddingBlob = lore.Embedding
	}

	// sources defaults to "[]" to match Engram schema (NOT NULL DEFAULT '[]')
	sourcesStr := "[]"
	if len(lore.Sources) > 0 {
		sourcesStr = strings.Join(lore.Sources, ",")
	}

	// INSERT lore
	// Set embedding_status to 'pending' for locally recorded lore (Recall doesn't generate embeddings)
	embeddingStatus := "pending"
	if lore.EmbeddingStatus != "" {
		embeddingStatus = lore.EmbeddingStatus
	}
	_, err = tx.Exec(`
		INSERT INTO lore_entries (id, content, context, category, confidence, embedding, embedding_status, source_id, sources, validation_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		lore.ID,
		lore.Content,
		nullString(lore.Context),
		string(lore.Category),
		lore.Confidence,
		embeddingBlob,
		embeddingStatus,
		lore.SourceID,
		sourcesStr,
		lore.ValidationCount,
		lore.CreatedAt.Format(time.RFC3339),
		lore.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("store: insert lore: %w", err)
	}

	// Build full entity payload for change_log
	payloadJSON, err := lorePayloadJSON(lore)
	if err != nil {
		return fmt.Errorf("store: marshal change_log payload: %w", err)
	}

	// INSERT change_log
	if err := appendChangeLog(tx, "lore_entries", lore.ID, "upsert", payloadJSON, s.sourceID); err != nil {
		return err
	}

	return tx.Commit()
}

// Record stores a new lore entry.
// Deprecated: Use InsertLore for atomic operations via Client.Record.
func (s *Store) Record(lore Lore) (*Lore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	// Validate
	if lore.Content == "" {
		return nil, ErrEmptyContent
	}
	if len(lore.Content) > MaxContentLength {
		return nil, ErrContentTooLong
	}
	if len(lore.Context) > MaxContextLength {
		return nil, ErrContextTooLong
	}
	if !lore.Category.IsValid() {
		return nil, ErrInvalidCategory
	}

	// Set defaults
	if lore.ID == "" {
		lore.ID = ulid.Make().String()
	}
	if lore.Confidence == 0 {
		lore.Confidence = ConfidenceDefault
	}
	if lore.Confidence < ConfidenceMin || lore.Confidence > ConfidenceMax {
		return nil, ErrInvalidConfidence
	}

	now := time.Now().UTC()
	lore.CreatedAt = now
	lore.UpdatedAt = now

	var embeddingBlob []byte
	if len(lore.Embedding) > 0 {
		embeddingBlob = lore.Embedding
	}

	// sources defaults to "[]" to match Engram schema (NOT NULL DEFAULT '[]')
	sourcesStr := "[]"
	if len(lore.Sources) > 0 {
		sourcesStr = strings.Join(lore.Sources, ",")
	}

	// Set embedding_status to 'pending' for locally recorded lore (Recall doesn't generate embeddings)
	embeddingStatus := "pending"
	if lore.EmbeddingStatus != "" {
		embeddingStatus = lore.EmbeddingStatus
	}
	_, err := s.db.Exec(`
		INSERT INTO lore_entries (id, content, context, category, confidence, embedding, embedding_status, source_id, sources, validation_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		lore.ID,
		lore.Content,
		nullString(lore.Context),
		string(lore.Category),
		lore.Confidence,
		embeddingBlob,
		embeddingStatus,
		lore.SourceID,
		sourcesStr,
		lore.ValidationCount,
		lore.CreatedAt.Format(time.RFC3339),
		lore.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("insert lore: %w", err)
	}

	// Queue for sync - intentionally non-failing; sync errors are handled
	// during background sync, not during local writes. This ensures local
	// operations remain fast and reliable even when sync queue has issues.
	_ = s.queueSync(lore.ID, "INSERT", nil)

	return &lore, nil
}

// Get retrieves a lore entry by ID.
func (s *Store) Get(id string) (*Lore, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	return s.getLore(id)
}

func (s *Store) getLore(id string) (*Lore, error) {
	row := s.db.QueryRow(`
		SELECT id, content, context, category, confidence, embedding, embedding_status, source_id, sources,
		       validation_count, last_validated_at, created_at, updated_at, deleted_at, synced_at
		FROM lore_entries WHERE id = ? AND deleted_at IS NULL
	`, id)

	return s.scanLore(row)
}

// getLoreTx reads a lore entry within a transaction.
func (s *Store) getLoreTx(tx *sql.Tx, id string) (*Lore, error) {
	row := tx.QueryRow(`
		SELECT id, content, context, category, confidence, embedding, embedding_status, source_id, sources,
		       validation_count, last_validated_at, created_at, updated_at, deleted_at, synced_at
		FROM lore_entries WHERE id = ? AND deleted_at IS NULL
	`, id)

	return s.scanLore(row)
}

// Query retrieves lore matching the given parameters.
// Note: This performs brute-force similarity search when embeddings are present.
func (s *Store) Query(params QueryParams) ([]Lore, error) {
	return s.queryLore(params, false)
}

// QueryWithEmbeddings retrieves lore that has embeddings, matching the given parameters.
// This is used for semantic similarity search where embeddings are required.
func (s *Store) QueryWithEmbeddings(params QueryParams) ([]Lore, error) {
	return s.queryLore(params, true)
}

func (s *Store) queryLore(params QueryParams, requireEmbedding bool) ([]Lore, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	// Build query - exclude soft-deleted records
	query := `
		SELECT id, content, context, category, confidence, embedding, embedding_status, source_id, sources,
		       validation_count, last_validated_at, created_at, updated_at, deleted_at, synced_at
		FROM lore_entries WHERE deleted_at IS NULL
	`
	args := []any{}

	if requireEmbedding {
		query += " AND embedding IS NOT NULL"
	}

	if params.MinConfidence != nil && *params.MinConfidence > 0 {
		query += " AND confidence >= ?"
		args = append(args, *params.MinConfidence)
	}

	if len(params.Categories) > 0 {
		placeholders := make([]string, len(params.Categories))
		for i, cat := range params.Categories {
			placeholders[i] = "?"
			args = append(args, string(cat))
		}
		query += fmt.Sprintf(" AND category IN (%s)", strings.Join(placeholders, ","))
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query lore: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []Lore
	for rows.Next() {
		lore, err := s.scanLoreRows(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *lore)
	}

	return results, rows.Err()
}

// ApplyFeedback atomically applies feedback to a lore entry.
// All operations occur in a single transaction:
//  1. UPDATE lore SET confidence (clamped to [0.0, 1.0])
//  2. IF isHelpful: INCREMENT validation_count, SET last_validated
//  3. Write change_log entry with full entity state (upsert operation)
//  4. COMMIT
//
// If any step fails, the entire transaction rolls back.
//
// Returns the updated Lore entry.
// Returns ErrNotFound if lore with given ID does not exist.
func (s *Store) ApplyFeedback(loreID string, delta float64, isHelpful bool) (*Lore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	// Verify lore exists first. This check is outside the transaction but safe
	// because s.mu write lock is held, preventing concurrent modifications.
	lore, err := s.getLore(loreID)
	if err != nil {
		return nil, err
	}

	// Begin transaction
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("store: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op if committed

	// Calculate new confidence with clamping
	newConfidence := lore.Confidence + delta
	if newConfidence < ConfidenceMin {
		newConfidence = ConfidenceMin
	}
	if newConfidence > ConfidenceMax {
		newConfidence = ConfidenceMax
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	// UPDATE lore (with or without validation metadata)
	if isHelpful {
		_, err = tx.Exec(`
			UPDATE lore_entries SET
				confidence = ?,
				validation_count = validation_count + 1,
				last_validated_at = ?,
				updated_at = ?
			WHERE id = ? AND deleted_at IS NULL
		`, newConfidence, nowStr, nowStr, loreID)
	} else {
		_, err = tx.Exec(`
			UPDATE lore_entries SET
				confidence = ?,
				updated_at = ?
			WHERE id = ? AND deleted_at IS NULL
		`, newConfidence, nowStr, loreID)
	}
	if err != nil {
		return nil, fmt.Errorf("store: update confidence: %w", err)
	}

	// Read the full updated entity state within the transaction for change_log
	updatedLore, err := s.getLoreTx(tx, loreID)
	if err != nil {
		return nil, fmt.Errorf("store: read updated lore: %w", err)
	}

	// Write full-state upsert to change_log
	payloadJSON, err := lorePayloadJSON(updatedLore)
	if err != nil {
		return nil, fmt.Errorf("store: marshal change_log payload: %w", err)
	}
	if err := appendChangeLog(tx, "lore_entries", loreID, "upsert", payloadJSON, s.sourceID); err != nil {
		return nil, err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: commit: %w", err)
	}

	return updatedLore, nil
}

// ApplyFeedbackBatch updates lore confidence based on batch feedback.
// Deprecated: Use ApplyFeedback() for single-entry atomic feedback.
func (s *Store) ApplyFeedbackBatch(session *Session, params FeedbackParams) (*FeedbackResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	result := &FeedbackResult{Updated: []FeedbackUpdate{}}
	now := time.Now().UTC()

	// Content lookup for fuzzy matching
	contentLookup := func(id string) string {
		lore, err := s.getLore(id)
		if err != nil {
			return ""
		}
		return lore.Content
	}

	// Process helpful feedback
	for _, ref := range params.Helpful {
		id, ok := session.FuzzyMatch(ref, contentLookup)
		if !ok {
			result.NotFound = append(result.NotFound, ref)
			continue
		}
		update, err := s.adjustConfidence(id, ConfidenceHelpfulDelta, true, now, string(FeedbackHelpful))
		if err == nil {
			result.Updated = append(result.Updated, *update)
		}
	}

	// Process incorrect feedback
	for _, ref := range params.Incorrect {
		id, ok := session.FuzzyMatch(ref, contentLookup)
		if !ok {
			result.NotFound = append(result.NotFound, ref)
			continue
		}
		update, err := s.adjustConfidence(id, ConfidenceIncorrectDelta, false, now, string(FeedbackIncorrect))
		if err == nil {
			result.Updated = append(result.Updated, *update)
		}
	}

	// Process not_relevant feedback - track as not found if ref doesn't exist
	for _, ref := range params.NotRelevant {
		_, ok := session.FuzzyMatch(ref, contentLookup)
		if !ok {
			result.NotFound = append(result.NotFound, ref)
		}
		// not_relevant: no adjustment needed when found
	}

	return result, nil
}

func (s *Store) adjustConfidence(id string, delta float64, incrementValidation bool, now time.Time, outcome string) (*FeedbackUpdate, error) {
	lore, err := s.getLore(id)
	if err != nil {
		return nil, err
	}

	previous := lore.Confidence
	current := previous + delta
	if current < ConfidenceMin {
		current = ConfidenceMin
	}
	if current > ConfidenceMax {
		current = ConfidenceMax
	}

	validationCount := lore.ValidationCount
	var lastValidatedAt *string
	if incrementValidation {
		validationCount++
		ts := now.Format(time.RFC3339)
		lastValidatedAt = &ts
	}

	_, err = s.db.Exec(`
		UPDATE lore_entries
		SET confidence = ?, validation_count = ?, last_validated_at = COALESCE(?, last_validated_at), updated_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`, current, validationCount, lastValidatedAt, now.Format(time.RFC3339), id)
	if err != nil {
		return nil, err
	}

	// Only queue FEEDBACK for lore that has been synced to central.
	// Matches behavior in ApplyFeedback() - skip locally-created lore.
	if lore.SyncedAt != nil {
		payloadBytes, _ := json.Marshal(FeedbackQueuePayload{Outcome: outcome})
		_ = s.queueSync(id, "FEEDBACK", payloadBytes)
	}

	return &FeedbackUpdate{
		ID:              id,
		Previous:        previous,
		Current:         current,
		ValidationCount: validationCount,
	}, nil
}

// Unsynced returns lore entries that haven't been synced yet.
func (s *Store) Unsynced() ([]Lore, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	rows, err := s.db.Query(`
		SELECT id, content, context, category, confidence, embedding, embedding_status, source_id, sources,
		       validation_count, last_validated_at, created_at, updated_at, deleted_at, synced_at
		FROM lore_entries WHERE synced_at IS NULL AND deleted_at IS NULL
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []Lore
	for rows.Next() {
		lore, err := s.scanLoreRows(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *lore)
	}

	return results, rows.Err()
}

// MarkSynced marks lore entries as synced.
func (s *Store) MarkSynced(ids []string, syncedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	if len(ids) == 0 {
		return nil
	}

	placeholders := make([]string, len(ids))
	args := []any{syncedAt.Format(time.RFC3339)}
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}

	query := fmt.Sprintf(`UPDATE lore_entries SET synced_at = ? WHERE id IN (%s)`, strings.Join(placeholders, ","))
	_, err := s.db.Exec(query, args...)
	return err
}

// Stats returns store statistics.
func (s *Store) Stats() (*StoreStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM lore_entries WHERE deleted_at IS NULL").Scan(&count); err != nil {
		return nil, err
	}

	var pendingSync int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sync_queue").Scan(&pendingSync); err != nil {
		return nil, err
	}

	var lastSyncStr sql.NullString
	_ = s.db.QueryRow("SELECT value FROM metadata WHERE key = 'last_sync'").Scan(&lastSyncStr)

	var lastSync time.Time
	if lastSyncStr.Valid {
		lastSync, _ = time.Parse(time.RFC3339, lastSyncStr.String)
	}

	return &StoreStats{
		LoreCount:     count,
		PendingSync:   pendingSync,
		LastSync:      lastSync,
		SchemaVersion: schemaVersion,
	}, nil
}

// Store metadata key constants
const (
	metadataKeyDescription  = "description"
	metadataKeyCreatedAt    = "created_at"
	metadataKeyMigratedFrom = "migrated_from"
)

// GetStoreDescription returns the store's human-readable description.
func (s *Store) GetStoreDescription() (string, error) {
	return s.GetMetadata(metadataKeyDescription)
}

// SetStoreDescription sets the store's human-readable description.
func (s *Store) SetStoreDescription(description string) error {
	return s.SetMetadata(metadataKeyDescription, description)
}

// GetStoreCreatedAt returns when the store was created.
func (s *Store) GetStoreCreatedAt() (time.Time, error) {
	val, err := s.GetMetadata(metadataKeyCreatedAt)
	if err != nil {
		return time.Time{}, err
	}
	if val == "" {
		return time.Time{}, nil
	}
	// Try RFC3339 first, then SQLite datetime format
	t, err := time.Parse(time.RFC3339, val)
	if err != nil {
		// SQLite datetime() format: "2006-01-02 15:04:05"
		t, err = time.Parse("2006-01-02 15:04:05", val)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse created_at: %w", err)
		}
	}
	return t.UTC(), nil
}

// GetStoreMigratedFrom returns the original path if this store was migrated.
// Returns empty string for new stores.
func (s *Store) GetStoreMigratedFrom() (string, error) {
	return s.GetMetadata(metadataKeyMigratedFrom)
}

// SetStoreMigratedFrom records the original path for a migrated store.
func (s *Store) SetStoreMigratedFrom(path string) error {
	return s.SetMetadata(metadataKeyMigratedFrom, path)
}

// GetMetadata retrieves a metadata value by key.
// Returns empty string if key not found.
func (s *Store) GetMetadata(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return "", ErrStoreClosed
	}

	var value sql.NullString
	err := s.db.QueryRow("SELECT value FROM metadata WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value.String, nil
}

// SetMetadata sets a metadata key-value pair (upsert).
func (s *Store) SetMetadata(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	_, err := s.db.Exec(`
		INSERT INTO metadata (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

// DetailedStats returns detailed store statistics including category distribution.
type DetailedStats struct {
	LoreCount            int                `json:"lore_count"`
	AverageConfidence    float64            `json:"average_confidence"`
	CategoryDistribution map[Category]int   `json:"category_distribution"`
	LastUpdated          time.Time          `json:"last_updated"`
}

// GetDetailedStats returns detailed statistics for the store.
func (s *Store) GetDetailedStats() (*DetailedStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	stats := &DetailedStats{
		CategoryDistribution: make(map[Category]int),
	}

	// Count and average confidence
	var avgConf sql.NullFloat64
	err := s.db.QueryRow(`
		SELECT COUNT(*), AVG(confidence)
		FROM lore_entries
		WHERE deleted_at IS NULL
	`).Scan(&stats.LoreCount, &avgConf)
	if err != nil {
		return nil, fmt.Errorf("query lore stats: %w", err)
	}
	if avgConf.Valid {
		stats.AverageConfidence = avgConf.Float64
	}

	// Category distribution
	rows, err := s.db.Query(`
		SELECT category, COUNT(*)
		FROM lore_entries
		WHERE deleted_at IS NULL
		GROUP BY category
	`)
	if err != nil {
		return nil, fmt.Errorf("query category distribution: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var cat string
		var count int
		if err := rows.Scan(&cat, &count); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		stats.CategoryDistribution[Category(cat)] = count
	}

	// Last updated (most recent updated_at)
	var lastUpdatedStr sql.NullString
	err = s.db.QueryRow(`
		SELECT MAX(updated_at)
		FROM lore_entries
		WHERE deleted_at IS NULL
	`).Scan(&lastUpdatedStr)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("query last updated: %w", err)
	}
	if lastUpdatedStr.Valid {
		stats.LastUpdated, _ = time.Parse(time.RFC3339, lastUpdatedStr.String)
	}

	return stats, nil
}

// ReplaceFromSnapshot atomically replaces all lore with data from a snapshot.
//
// The snapshot is expected to be a SQLite database file streamed as io.Reader.
// This method:
//  1. Writes snapshot to a temp file
//  2. Opens temp database and reads all lore
//  3. In a single transaction: DELETE all lore, INSERT all from snapshot
//  4. Cleans up temp file
//
// If any step fails, the local lore data is preserved.
func (s *Store) ReplaceFromSnapshot(r io.Reader) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	// 1. Write to temp file
	tmpFile, err := os.CreateTemp("", "recall-snapshot-*.db")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := io.Copy(tmpFile, r); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write snapshot: %w", err)
	}
	_ = tmpFile.Close()

	// 2. Open snapshot database
	snapshotDB, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		return fmt.Errorf("open snapshot: %w", err)
	}
	defer func() { _ = snapshotDB.Close() }()

	// 3. Read all lore from snapshot (Engram snapshots don't have synced_at column)
	// Note: synced_at is Recall-only and won't be present in Engram snapshots
	rows, err := snapshotDB.Query(`
		SELECT id, content, context, category, confidence, embedding, embedding_status,
		       source_id, sources, validation_count, last_validated_at,
		       created_at, updated_at, deleted_at
		FROM lore_entries WHERE deleted_at IS NULL
	`)
	if err != nil {
		return fmt.Errorf("read snapshot: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var loreEntries []Lore
	for rows.Next() {
		lore, err := s.scanSnapshotLoreRows(rows)
		if err != nil {
			return fmt.Errorf("scan snapshot row: %w", err)
		}
		loreEntries = append(loreEntries, *lore)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate snapshot: %w", err)
	}

	// 4. Atomic replacement in local database
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete all existing lore
	if _, err := tx.Exec("DELETE FROM lore_entries"); err != nil {
		return fmt.Errorf("delete existing lore: %w", err)
	}

	// Clear sync queue (bootstrap replaces everything)
	if _, err := tx.Exec("DELETE FROM sync_queue"); err != nil {
		return fmt.Errorf("clear sync queue: %w", err)
	}

	// Insert all snapshot lore
	for _, lore := range loreEntries {
		if err := s.insertLoreTx(tx, &lore); err != nil {
			return fmt.Errorf("insert lore %s: %w", lore.ID, err)
		}
	}

	return tx.Commit()
}

// insertLoreTx inserts a lore entry within a transaction (no sync queue).
func (s *Store) insertLoreTx(tx *sql.Tx, lore *Lore) error {
	var embeddingBlob []byte
	if len(lore.Embedding) > 0 {
		embeddingBlob = lore.Embedding
	}

	// sources defaults to "[]" to match Engram schema (NOT NULL DEFAULT '[]')
	sourcesStr := "[]"
	if len(lore.Sources) > 0 {
		sourcesStr = strings.Join(lore.Sources, ",")
	}

	var lastValidatedAtStr *string
	if lore.LastValidatedAt != nil {
		ts := lore.LastValidatedAt.Format(time.RFC3339)
		lastValidatedAtStr = &ts
	}

	var deletedAtStr *string
	if lore.DeletedAt != nil {
		ts := lore.DeletedAt.Format(time.RFC3339)
		deletedAtStr = &ts
	}

	var syncedAtStr *string
	if lore.SyncedAt != nil {
		ts := lore.SyncedAt.Format(time.RFC3339)
		syncedAtStr = &ts
	}

	// Default embedding_status to 'complete' for snapshot imports (they have embeddings)
	embeddingStatus := lore.EmbeddingStatus
	if embeddingStatus == "" {
		embeddingStatus = "complete"
	}

	_, err := tx.Exec(`
		INSERT INTO lore_entries (id, content, context, category, confidence, embedding, embedding_status,
		                 source_id, sources, validation_count, last_validated_at,
		                 created_at, updated_at, deleted_at, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		lore.ID,
		lore.Content,
		nullString(lore.Context),
		string(lore.Category),
		lore.Confidence,
		embeddingBlob,
		embeddingStatus,
		lore.SourceID,
		sourcesStr,
		lore.ValidationCount,
		lastValidatedAtStr,
		lore.CreatedAt.Format(time.RFC3339),
		lore.UpdatedAt.Format(time.RFC3339),
		deletedAtStr,
		syncedAtStr,
	)
	return err
}

// Close closes the store.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	return s.db.Close()
}

func (s *Store) queueSync(loreID, operation string, payload []byte) error {
	_, err := s.db.Exec(`
		INSERT INTO sync_queue (lore_id, operation, payload, queued_at)
		VALUES (?, ?, ?, ?)
	`, loreID, operation, payload, time.Now().UTC().Format(time.RFC3339))
	return err
}

// scanner abstracts the Scan method shared by *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanLoreFrom scans a single lore row from any scanner (Row or Rows).
// Returns ErrNotFound only for sql.ErrNoRows from *sql.Row.
func (s *Store) scanLoreFrom(sc scanner) (*Lore, error) {
	var (
		lore            Lore
		context         sql.NullString
		embeddingBlob   []byte
		embeddingStatus string
		sources         sql.NullString
		lastValidatedAt sql.NullString
		deletedAt       sql.NullString
		syncedAt        sql.NullString
		createdAt       string
		updatedAt       string
		category        string
	)

	err := sc.Scan(
		&lore.ID,
		&lore.Content,
		&context,
		&category,
		&lore.Confidence,
		&embeddingBlob,
		&embeddingStatus,
		&lore.SourceID,
		&sources,
		&lore.ValidationCount,
		&lastValidatedAt,
		&createdAt,
		&updatedAt,
		&deletedAt,
		&syncedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	lore.Category = Category(category)
	lore.EmbeddingStatus = embeddingStatus
	if context.Valid {
		lore.Context = context.String
	}
	if len(embeddingBlob) > 0 {
		lore.Embedding = embeddingBlob
	}
	if sources.Valid && sources.String != "" && sources.String != "[]" {
		lore.Sources = strings.Split(sources.String, ",")
	}
	if lastValidatedAt.Valid {
		t, _ := time.Parse(time.RFC3339, lastValidatedAt.String)
		lore.LastValidatedAt = &t
	}
	lore.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	lore.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if deletedAt.Valid {
		t, _ := time.Parse(time.RFC3339, deletedAt.String)
		lore.DeletedAt = &t
	}
	if syncedAt.Valid {
		t, _ := time.Parse(time.RFC3339, syncedAt.String)
		lore.SyncedAt = &t
	}

	return &lore, nil
}

// scanLore scans a single row from QueryRow.
func (s *Store) scanLore(row *sql.Row) (*Lore, error) {
	return s.scanLoreFrom(row)
}

// scanLoreRows scans a single row from Query iteration.
func (s *Store) scanLoreRows(rows *sql.Rows) (*Lore, error) {
	return s.scanLoreFrom(rows)
}

// scanSnapshotLoreRows scans a lore row from Engram snapshots (no synced_at column).
func (s *Store) scanSnapshotLoreRows(rows *sql.Rows) (*Lore, error) {
	var (
		lore            Lore
		context         sql.NullString
		embeddingBlob   []byte
		embeddingStatus string
		sources         sql.NullString
		lastValidatedAt sql.NullString
		deletedAt       sql.NullString
		createdAt       string
		updatedAt       string
		category        string
	)

	err := rows.Scan(
		&lore.ID,
		&lore.Content,
		&context,
		&category,
		&lore.Confidence,
		&embeddingBlob,
		&embeddingStatus,
		&lore.SourceID,
		&sources,
		&lore.ValidationCount,
		&lastValidatedAt,
		&createdAt,
		&updatedAt,
		&deletedAt,
	)
	if err != nil {
		return nil, err
	}

	lore.Category = Category(category)
	lore.EmbeddingStatus = embeddingStatus
	if context.Valid {
		lore.Context = context.String
	}
	if len(embeddingBlob) > 0 {
		lore.Embedding = embeddingBlob
	}
	if sources.Valid && sources.String != "" && sources.String != "[]" {
		lore.Sources = strings.Split(sources.String, ",")
	}
	if lastValidatedAt.Valid {
		t, _ := time.Parse(time.RFC3339, lastValidatedAt.String)
		lore.LastValidatedAt = &t
	}
	lore.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	lore.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if deletedAt.Valid {
		t, _ := time.Parse(time.RFC3339, deletedAt.String)
		lore.DeletedAt = &t
	}
	// synced_at is not in Engram snapshots, leave it nil

	return &lore, nil
}

// ChangeLogEntry represents a single row from the change_log table.
type ChangeLogEntry struct {
	Sequence  int64   `json:"sequence"`
	TableName string  `json:"table_name"`
	EntityID  string  `json:"entity_id"`
	Operation string  `json:"operation"`
	Payload   *string `json:"payload"`
	SourceID  string  `json:"source_id"`
	CreatedAt string  `json:"created_at"`
}

// UnpushedChanges returns change_log entries for a given sourceID after a given
// sequence number, ordered by sequence ASC, limited to limit rows.
func (s *Store) UnpushedChanges(sourceID string, afterSeq int64, limit int) ([]ChangeLogEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	rows, err := s.db.Query(`
		SELECT sequence, table_name, entity_id, operation, payload, source_id, created_at
		FROM change_log
		WHERE sequence > ? AND source_id = ?
		ORDER BY sequence ASC
		LIMIT ?
	`, afterSeq, sourceID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: query unpushed changes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []ChangeLogEntry
	for rows.Next() {
		var e ChangeLogEntry
		var payload sql.NullString
		if err := rows.Scan(&e.Sequence, &e.TableName, &e.EntityID, &e.Operation, &payload, &e.SourceID, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan change_log: %w", err)
		}
		if payload.Valid {
			e.Payload = &payload.String
		}
		entries = append(entries, e)
	}

	return entries, rows.Err()
}

// GetSyncMeta retrieves a value from the sync_meta table.
// Returns empty string if key not found.
func (s *Store) GetSyncMeta(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return "", ErrStoreClosed
	}

	var value sql.NullString
	err := s.db.QueryRow("SELECT value FROM sync_meta WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: get sync meta: %w", err)
	}
	return value.String, nil
}

// SetSyncMeta persists a key-value pair in the sync_meta table via INSERT OR REPLACE.
func (s *Store) SetSyncMeta(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	_, err := s.db.Exec("INSERT OR REPLACE INTO sync_meta (key, value) VALUES (?, ?)", key, value)
	if err != nil {
		return fmt.Errorf("store: set sync meta: %w", err)
	}
	return nil
}

// GetSourceID returns the source_id value from sync_meta (live DB read).
// For the cached value, use SourceID() instead.
func (s *Store) GetSourceID() (string, error) {
	return s.GetSyncMeta("source_id")
}

// PendingFeedback returns feedback entries pending sync.
// This is a stub for Story 4.3 (Push sync).
func (s *Store) PendingFeedback() ([]FeedbackEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	// TODO: Implement in Story 4.3
	return nil, nil
}

// MarkFeedbackSynced marks feedback entries as synced.
// This is a stub for Story 4.3 (Push sync).
func (s *Store) MarkFeedbackSynced(ids []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	// TODO: Implement in Story 4.3
	return nil
}

// FeedbackEntry represents a pending feedback item in the sync queue.
// Note: This type mirrors internal/sync.FeedbackEntry. The duplication exists due to
// an import cycle (internal/sync imports recall). A future refactor could extract
// shared types to internal/types to eliminate this duplication.
type FeedbackEntry struct {
	ID      int64
	LoreID  string
	Outcome string
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// PendingSyncEntries returns all entries from the sync queue.
func (s *Store) PendingSyncEntries() ([]SyncQueueEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	rows, err := s.db.Query(`
		SELECT id, lore_id, operation, payload, queued_at, attempts, last_error
		FROM sync_queue
		ORDER BY queued_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query sync queue: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []SyncQueueEntry
	for rows.Next() {
		var e SyncQueueEntry
		var payload sql.NullString
		var lastError sql.NullString
		var queuedAt string

		if err := rows.Scan(&e.ID, &e.LoreID, &e.Operation, &payload, &queuedAt, &e.Attempts, &lastError); err != nil {
			return nil, fmt.Errorf("scan sync queue: %w", err)
		}

		e.Payload = payload.String
		e.LastError = lastError.String
		e.QueuedAt, _ = time.Parse(time.RFC3339, queuedAt)
		entries = append(entries, e)
	}

	return entries, rows.Err()
}

// CompleteSyncEntries removes entries from the sync queue and updates synced_at.
// Called after successful push to Engram.
func (s *Store) CompleteSyncEntries(queueIDs []int64, loreIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	if len(queueIDs) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)

	// Delete from sync_queue
	queuePlaceholders := make([]string, len(queueIDs))
	queueArgs := make([]any, len(queueIDs))
	for i, id := range queueIDs {
		queuePlaceholders[i] = "?"
		queueArgs[i] = id
	}
	_, err = tx.Exec(
		fmt.Sprintf("DELETE FROM sync_queue WHERE id IN (%s)", strings.Join(queuePlaceholders, ",")),
		queueArgs...,
	)
	if err != nil {
		return fmt.Errorf("delete sync queue: %w", err)
	}

	// Update synced_at on lore entries
	if len(loreIDs) > 0 {
		lorePlaceholders := make([]string, len(loreIDs))
		loreArgs := []any{now}
		for i, id := range loreIDs {
			lorePlaceholders[i] = "?"
			loreArgs = append(loreArgs, id)
		}
		_, err = tx.Exec(
			fmt.Sprintf("UPDATE lore_entries SET synced_at = ? WHERE id IN (%s)", strings.Join(lorePlaceholders, ",")),
			loreArgs...,
		)
		if err != nil {
			return fmt.Errorf("update synced_at: %w", err)
		}
	}

	return tx.Commit()
}

// FailSyncEntries increments attempt count and records error for failed entries.
func (s *Store) FailSyncEntries(queueIDs []int64, lastError string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	if len(queueIDs) == 0 {
		return nil
	}

	placeholders := make([]string, len(queueIDs))
	args := []any{lastError}
	for i, id := range queueIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	_, err := s.db.Exec(
		fmt.Sprintf(`
			UPDATE sync_queue
			SET attempts = attempts + 1, last_error = ?
			WHERE id IN (%s)
		`, strings.Join(placeholders, ",")),
		args...,
	)
	return err
}

// DeleteSyncEntry removes a single entry from the sync queue by its ID.
// Used to clean up orphaned feedback entries when lore hasn't been synced.
func (s *Store) DeleteSyncEntry(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	_, err := s.db.Exec("DELETE FROM sync_queue WHERE id = ?", id)
	return err
}

// UpsertLore inserts or updates a lore entry.
// If lore with the same ID exists, it updates all fields.
// If lore doesn't exist, it inserts a new entry.
// Used by delta sync to apply incremental changes.
func (s *Store) UpsertLore(lore *Lore) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	var embeddingBlob []byte
	if len(lore.Embedding) > 0 {
		embeddingBlob = lore.Embedding
	}

	// sources defaults to "[]" to match Engram schema
	sourcesStr := "[]"
	if len(lore.Sources) > 0 {
		sourcesStr = strings.Join(lore.Sources, ",")
	}

	var lastValidatedAtStr *string
	if lore.LastValidatedAt != nil {
		ts := lore.LastValidatedAt.Format(time.RFC3339)
		lastValidatedAtStr = &ts
	}

	// Default embedding_status to 'pending' if empty
	embeddingStatus := lore.EmbeddingStatus
	if embeddingStatus == "" {
		embeddingStatus = "pending"
	}

	now := time.Now().UTC()
	if lore.CreatedAt.IsZero() {
		lore.CreatedAt = now
	}
	if lore.UpdatedAt.IsZero() {
		lore.UpdatedAt = now
	}

	_, err := s.db.Exec(`
		INSERT INTO lore_entries (id, content, context, category, confidence, embedding, embedding_status,
		                 source_id, sources, validation_count, last_validated_at,
		                 created_at, updated_at, deleted_at, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?)
		ON CONFLICT(id) DO UPDATE SET
			content = excluded.content,
			context = excluded.context,
			category = excluded.category,
			confidence = excluded.confidence,
			embedding = excluded.embedding,
			embedding_status = excluded.embedding_status,
			source_id = excluded.source_id,
			sources = excluded.sources,
			validation_count = excluded.validation_count,
			last_validated_at = excluded.last_validated_at,
			updated_at = excluded.updated_at,
			deleted_at = NULL,
			synced_at = excluded.synced_at
	`,
		lore.ID,
		lore.Content,
		nullString(lore.Context),
		string(lore.Category),
		lore.Confidence,
		embeddingBlob,
		embeddingStatus,
		lore.SourceID,
		sourcesStr,
		lore.ValidationCount,
		lastValidatedAtStr,
		lore.CreatedAt.Format(time.RFC3339),
		lore.UpdatedAt.Format(time.RFC3339),
		nil, // synced_at: NULL because delta-synced entries originate from Engram (already synced)
	)
	if err != nil {
		return fmt.Errorf("store: upsert lore: %w", err)
	}

	return nil
}

// DeleteLoreByID soft-deletes a lore entry and writes a change_log delete entry.
// Sets deleted_at on the lore entry rather than removing the row.
// Returns nil if the entry doesn't exist.
func (s *Store) DeleteLoreByID(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)

	// Soft delete: set deleted_at instead of removing the row
	_, err = tx.Exec(`
		UPDATE lore_entries SET deleted_at = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`, now, now, id)
	if err != nil {
		return fmt.Errorf("store: soft delete lore: %w", err)
	}

	// Write change_log entry with operation=delete, payload=NULL
	if err := appendChangeLog(tx, "lore_entries", id, "delete", nil, s.sourceID); err != nil {
		return err
	}

	return tx.Commit()
}

// HasPendingSync returns the count of unpushed local changes.
// Counts entries in both sync_queue (legacy) and change_log (new).
// Returns 0 if no pending changes exist.
func (s *Store) HasPendingSync() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return 0, ErrStoreClosed
	}

	var sqCount int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM sync_queue
		WHERE operation IN ('INSERT', 'FEEDBACK')
	`).Scan(&sqCount)
	if err != nil {
		return 0, fmt.Errorf("store: count pending sync_queue: %w", err)
	}

	var clCount int
	err = s.db.QueryRow(`SELECT COUNT(*) FROM change_log`).Scan(&clCount)
	if err != nil {
		return 0, fmt.Errorf("store: count pending change_log: %w", err)
	}

	return sqCount + clCount, nil
}

// ClearAllLore removes all lore entries and clears the sync queue.
// Used by Reinitialize when creating an empty database.
func (s *Store) ClearAllLore() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("DELETE FROM lore_entries"); err != nil {
		return fmt.Errorf("store: delete lore: %w", err)
	}

	if _, err := tx.Exec("DELETE FROM sync_queue"); err != nil {
		return fmt.Errorf("store: clear sync queue: %w", err)
	}

	return tx.Commit()
}

// GetLoreByIDs retrieves multiple lore entries by ID.
func (s *Store) GetLoreByIDs(ids []string) ([]Lore, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT id, content, context, category, confidence, embedding, embedding_status, source_id, sources,
		       validation_count, last_validated_at, created_at, updated_at, deleted_at, synced_at
		FROM lore_entries WHERE id IN (%s) AND deleted_at IS NULL
	`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, fmt.Errorf("query lore: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []Lore
	for rows.Next() {
		lore, err := s.scanLoreRows(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *lore)
	}

	return results, rows.Err()
}
