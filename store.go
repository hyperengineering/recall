package recall

import (
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

const schemaVersion = "1"

// Store manages the local SQLite lore database.
type Store struct {
	db     *sql.DB
	mu     sync.RWMutex
	closed bool
	path   string
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

	return store, nil
}

func (s *Store) migrate() error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("store: set goose dialect: %w", err)
	}
	if err := goose.Up(s.db, "."); err != nil {
		return fmt.Errorf("store: run migrations: %w", err)
	}

	// Set schema version if not set
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO metadata (key, value) VALUES ('schema_version', ?)
	`, schemaVersion)
	return err
}

// InsertLore atomically inserts a lore entry and a sync queue entry in one transaction.
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

	// INSERT sync_queue
	_, err = tx.Exec(`
		INSERT INTO sync_queue (lore_id, operation, queued_at)
		VALUES (?, ?, ?)
	`, lore.ID, "INSERT", time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: enqueue sync: %w", err)
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
//  3. INSERT sync_queue entry (FEEDBACK operation)
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

	// Determine outcome for sync based on feedback signal:
	// - isHelpful=true -> "helpful" (user explicitly marked as useful)
	// - delta < 0 -> "incorrect" (negative feedback, confidence decreased)
	// - delta = 0 -> "not_relevant" (neutral/no impact on confidence)
	var outcome string
	switch {
	case isHelpful:
		outcome = "helpful"
	case delta < 0:
		outcome = "incorrect"
	default:
		outcome = "not_relevant"
	}

	// Queue with payload
	payloadJSON, _ := json.Marshal(FeedbackQueuePayload{Outcome: outcome})

	// INSERT sync_queue entry
	_, err = tx.Exec(`
		INSERT INTO sync_queue (lore_id, operation, payload, queued_at)
		VALUES (?, 'FEEDBACK', ?, ?)
	`, loreID, string(payloadJSON), nowStr)
	if err != nil {
		return nil, fmt.Errorf("store: enqueue sync: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: commit: %w", err)
	}

	// Return updated lore
	return s.getLore(loreID)
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
		update, err := s.adjustConfidence(id, ConfidenceHelpfulDelta, true, now)
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
		update, err := s.adjustConfidence(id, ConfidenceIncorrectDelta, false, now)
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

func (s *Store) adjustConfidence(id string, delta float64, incrementValidation bool, now time.Time) (*FeedbackUpdate, error) {
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

	// Queue feedback for sync - intentionally non-failing (see Record comment)
	_ = s.queueSync(id, "FEEDBACK", nil)

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
