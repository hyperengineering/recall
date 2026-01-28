package recall

import (
	"database/sql"
	"fmt"
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
		db.Close()
		return nil, fmt.Errorf("enable WAL mode: %w", err)
	}

	store := &Store{db: db, path: path}
	if err := store.migrate(); err != nil {
		db.Close()
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
	defer tx.Rollback() // no-op if committed

	var embeddingBlob []byte
	if len(lore.Embedding) > 0 {
		embeddingBlob = lore.Embedding
	}

	var sourcesStr *string
	if len(lore.Sources) > 0 {
		joined := strings.Join(lore.Sources, ",")
		sourcesStr = &joined
	}

	// INSERT lore
	_, err = tx.Exec(`
		INSERT INTO lore (id, content, context, category, confidence, embedding, source_id, sources, validation_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		lore.ID,
		lore.Content,
		nullString(lore.Context),
		string(lore.Category),
		lore.Confidence,
		embeddingBlob,
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

	var sourcesStr *string
	if len(lore.Sources) > 0 {
		joined := strings.Join(lore.Sources, ",")
		sourcesStr = &joined
	}

	_, err := s.db.Exec(`
		INSERT INTO lore (id, content, context, category, confidence, embedding, source_id, sources, validation_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		lore.ID,
		lore.Content,
		nullString(lore.Context),
		string(lore.Category),
		lore.Confidence,
		embeddingBlob,
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
		SELECT id, content, context, category, confidence, embedding, source_id, sources,
		       validation_count, last_validated, created_at, updated_at, synced_at
		FROM lore WHERE id = ?
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

	// Build query
	query := `
		SELECT id, content, context, category, confidence, embedding, source_id, sources,
		       validation_count, last_validated, created_at, updated_at, synced_at
		FROM lore WHERE 1=1
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
	defer rows.Close()

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

// ApplyFeedback updates lore confidence based on feedback.
func (s *Store) ApplyFeedback(session *Session, params FeedbackParams) (*FeedbackResult, error) {
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
			continue
		}
		update, err := s.adjustConfidence(id, ConfidenceIncorrectDelta, false, now)
		if err == nil {
			result.Updated = append(result.Updated, *update)
		}
	}

	// not_relevant: no adjustment needed

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
	var lastValidated *string
	if incrementValidation {
		validationCount++
		ts := now.Format(time.RFC3339)
		lastValidated = &ts
	}

	_, err = s.db.Exec(`
		UPDATE lore
		SET confidence = ?, validation_count = ?, last_validated = COALESCE(?, last_validated), updated_at = ?
		WHERE id = ?
	`, current, validationCount, lastValidated, now.Format(time.RFC3339), id)
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
		SELECT id, content, context, category, confidence, embedding, source_id, sources,
		       validation_count, last_validated, created_at, updated_at, synced_at
		FROM lore WHERE synced_at IS NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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

	query := fmt.Sprintf(`UPDATE lore SET synced_at = ? WHERE id IN (%s)`, strings.Join(placeholders, ","))
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
	if err := s.db.QueryRow("SELECT COUNT(*) FROM lore").Scan(&count); err != nil {
		return nil, err
	}

	var pendingSync int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sync_queue").Scan(&pendingSync); err != nil {
		return nil, err
	}

	var lastSyncStr sql.NullString
	s.db.QueryRow("SELECT value FROM metadata WHERE key = 'last_sync'").Scan(&lastSyncStr)

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
		lore          Lore
		context       sql.NullString
		embeddingBlob []byte
		sources       sql.NullString
		lastValidated sql.NullString
		syncedAt      sql.NullString
		createdAt     string
		updatedAt     string
		category      string
	)

	err := sc.Scan(
		&lore.ID,
		&lore.Content,
		&context,
		&category,
		&lore.Confidence,
		&embeddingBlob,
		&lore.SourceID,
		&sources,
		&lore.ValidationCount,
		&lastValidated,
		&createdAt,
		&updatedAt,
		&syncedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	lore.Category = Category(category)
	if context.Valid {
		lore.Context = context.String
	}
	if len(embeddingBlob) > 0 {
		lore.Embedding = embeddingBlob
	}
	if sources.Valid {
		lore.Sources = strings.Split(sources.String, ",")
	}
	if lastValidated.Valid {
		t, _ := time.Parse(time.RFC3339, lastValidated.String)
		lore.LastValidated = &t
	}
	lore.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	lore.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
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

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
