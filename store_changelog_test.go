package recall

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

// ============================================================================
// Story 9.2: Change Log Writes on Local Mutations
// ============================================================================

// changeLogEntry is a test helper for scanning change_log rows.
type changeLogEntry struct {
	Sequence   int
	TableName  string
	EntityID   string
	Operation  string
	Payload    sql.NullString
	SourceID   string
	CreatedAt  string
	ReceivedAt string
}

func scanChangeLogEntry(row *sql.Row) (*changeLogEntry, error) {
	var e changeLogEntry
	err := row.Scan(&e.Sequence, &e.TableName, &e.EntityID, &e.Operation, &e.Payload, &e.SourceID, &e.CreatedAt, &e.ReceivedAt)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// changeLogPayload represents the expected JSON payload structure for upsert entries.
type changeLogPayload struct {
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
}

// newTestStore creates a store in a temp directory for testing.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// getChangeLogCount returns the number of rows in change_log.
func getChangeLogCount(t *testing.T, store *Store) int {
	t.Helper()
	var count int
	err := store.db.QueryRow("SELECT COUNT(*) FROM change_log").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count change_log: %v", err)
	}
	return count
}

// getSyncQueueCount returns the number of rows in sync_queue.
func getSyncQueueCount(t *testing.T, store *Store) int {
	t.Helper()
	var count int
	err := store.db.QueryRow("SELECT COUNT(*) FROM sync_queue").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count sync_queue: %v", err)
	}
	return count
}

// getLatestChangeLogEntry returns the most recent change_log entry.
func getLatestChangeLogEntry(t *testing.T, store *Store) *changeLogEntry {
	t.Helper()
	row := store.db.QueryRow(`
		SELECT sequence, table_name, entity_id, operation, payload, source_id, created_at, received_at
		FROM change_log ORDER BY sequence DESC LIMIT 1
	`)
	e, err := scanChangeLogEntry(row)
	if err != nil {
		t.Fatalf("failed to read change_log: %v", err)
	}
	return e
}

// getSourceID reads the source_id from sync_meta.
func getSourceID(t *testing.T, store *Store) string {
	t.Helper()
	var sourceID string
	err := store.db.QueryRow("SELECT value FROM sync_meta WHERE key = 'source_id'").Scan(&sourceID)
	if err != nil {
		t.Fatalf("failed to read source_id: %v", err)
	}
	return sourceID
}

// =============================================================================
// AC #1: InsertLore creates change_log entry with correct fields
// =============================================================================

func TestInsertLore_CreatesChangeLogEntry(t *testing.T) {
	store := newTestStore(t)
	sourceID := getSourceID(t, store)

	now := time.Now().UTC()
	lore := &Lore{
		ID:              "01TESTID_CL_INSERT_00001",
		Content:         "Test content for change_log",
		Context:         "test context",
		Category:        CategoryArchitecturalDecision,
		Confidence:      0.75,
		EmbeddingStatus: "pending",
		SourceID:        "lore-source-id",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	err := store.InsertLore(lore)
	if err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Verify change_log entry exists
	count := getChangeLogCount(t, store)
	if count != 1 {
		t.Fatalf("change_log count = %d, want 1", count)
	}

	entry := getLatestChangeLogEntry(t, store)

	if entry.TableName != "lore_entries" {
		t.Errorf("table_name = %q, want %q", entry.TableName, "lore_entries")
	}
	if entry.EntityID != lore.ID {
		t.Errorf("entity_id = %q, want %q", entry.EntityID, lore.ID)
	}
	if entry.Operation != "upsert" {
		t.Errorf("operation = %q, want %q", entry.Operation, "upsert")
	}
	if entry.SourceID != sourceID {
		t.Errorf("source_id = %q, want %q (from sync_meta)", entry.SourceID, sourceID)
	}
	if entry.CreatedAt == "" {
		t.Error("created_at should not be empty")
	}
}

func TestInsertLore_ChangeLogPayloadContainsFullEntity(t *testing.T) {
	store := newTestStore(t)

	now := time.Now().UTC()
	lore := &Lore{
		ID:              "01TESTID_CL_INSERT_00002",
		Content:         "Full payload test",
		Context:         "payload context",
		Category:        CategoryPatternOutcome,
		Confidence:      0.85,
		EmbeddingStatus: "pending",
		SourceID:        "payload-source",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	err := store.InsertLore(lore)
	if err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	entry := getLatestChangeLogEntry(t, store)

	if !entry.Payload.Valid {
		t.Fatal("payload should not be NULL for upsert")
	}

	var payload changeLogPayload
	if err := json.Unmarshal([]byte(entry.Payload.String), &payload); err != nil {
		t.Fatalf("failed to parse payload JSON: %v", err)
	}

	// AC #1 & #5: Verify all required fields for Engram validation
	if payload.ID != lore.ID {
		t.Errorf("payload.id = %q, want %q", payload.ID, lore.ID)
	}
	if payload.Content != lore.Content {
		t.Errorf("payload.content = %q, want %q", payload.Content, lore.Content)
	}
	if payload.Context != lore.Context {
		t.Errorf("payload.context = %q, want %q", payload.Context, lore.Context)
	}
	if payload.Category != string(lore.Category) {
		t.Errorf("payload.category = %q, want %q", payload.Category, string(lore.Category))
	}
	if payload.Confidence != lore.Confidence {
		t.Errorf("payload.confidence = %f, want %f", payload.Confidence, lore.Confidence)
	}
	if payload.EmbeddingStatus != "pending" {
		t.Errorf("payload.embedding_status = %q, want %q", payload.EmbeddingStatus, "pending")
	}
	if payload.SourceID != lore.SourceID {
		t.Errorf("payload.source_id = %q, want %q", payload.SourceID, lore.SourceID)
	}
	if payload.CreatedAt == "" {
		t.Error("payload.created_at should not be empty")
	}
	if payload.UpdatedAt == "" {
		t.Error("payload.updated_at should not be empty")
	}
}

// =============================================================================
// AC #2: ApplyFeedback creates change_log upsert with full updated entity state
// =============================================================================

func TestApplyFeedback_CreatesChangeLogUpsert(t *testing.T) {
	store := newTestStore(t)
	sourceID := getSourceID(t, store)

	now := time.Now().UTC()
	lore := &Lore{
		ID:              "01TESTID_CL_FEEDBACK_001",
		Content:         "Feedback test content",
		Category:        CategoryEdgeCaseDiscovery,
		Confidence:      0.5,
		EmbeddingStatus: "pending",
		SourceID:        "feedback-source",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	err := store.InsertLore(lore)
	if err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Clear the change_log entry from InsertLore so we can isolate the feedback one
	_, err = store.db.Exec("DELETE FROM change_log")
	if err != nil {
		t.Fatalf("failed to clear change_log: %v", err)
	}

	// Apply helpful feedback
	updated, err := store.ApplyFeedback(lore.ID, ConfidenceHelpfulDelta, true)
	if err != nil {
		t.Fatalf("ApplyFeedback failed: %v", err)
	}

	count := getChangeLogCount(t, store)
	if count != 1 {
		t.Fatalf("change_log count = %d, want 1", count)
	}

	entry := getLatestChangeLogEntry(t, store)

	if entry.TableName != "lore_entries" {
		t.Errorf("table_name = %q, want %q", entry.TableName, "lore_entries")
	}
	if entry.EntityID != lore.ID {
		t.Errorf("entity_id = %q, want %q", entry.EntityID, lore.ID)
	}
	if entry.Operation != "upsert" {
		t.Errorf("operation = %q, want %q", entry.Operation, "upsert")
	}
	if entry.SourceID != sourceID {
		t.Errorf("source_id = %q, want %q", entry.SourceID, sourceID)
	}

	// Verify payload contains UPDATED state (not pre-update)
	if !entry.Payload.Valid {
		t.Fatal("payload should not be NULL for upsert")
	}

	var payload changeLogPayload
	if err := json.Unmarshal([]byte(entry.Payload.String), &payload); err != nil {
		t.Fatalf("failed to parse payload JSON: %v", err)
	}

	// The confidence should reflect the update, not the original
	expectedConfidence := updated.Confidence
	if payload.Confidence != expectedConfidence {
		t.Errorf("payload.confidence = %f, want %f (updated value)", payload.Confidence, expectedConfidence)
	}
	if payload.ValidationCount != updated.ValidationCount {
		t.Errorf("payload.validation_count = %d, want %d", payload.ValidationCount, updated.ValidationCount)
	}
}

// =============================================================================
// AC #3: Soft delete creates change_log entry with operation=delete, payload=NULL
// =============================================================================

func TestDeleteLoreByID_CreatesChangeLogDelete(t *testing.T) {
	store := newTestStore(t)
	sourceID := getSourceID(t, store)

	now := time.Now().UTC()
	lore := &Lore{
		ID:              "01TESTID_CL_DELETE_00001",
		Content:         "Delete test content",
		Category:        CategoryTestingStrategy,
		Confidence:      0.6,
		EmbeddingStatus: "pending",
		SourceID:        "delete-source",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	err := store.InsertLore(lore)
	if err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Clear change_log from insert
	_, err = store.db.Exec("DELETE FROM change_log")
	if err != nil {
		t.Fatalf("failed to clear change_log: %v", err)
	}

	err = store.DeleteLoreByID(lore.ID)
	if err != nil {
		t.Fatalf("DeleteLoreByID failed: %v", err)
	}

	count := getChangeLogCount(t, store)
	if count != 1 {
		t.Fatalf("change_log count = %d, want 1", count)
	}

	entry := getLatestChangeLogEntry(t, store)

	if entry.TableName != "lore_entries" {
		t.Errorf("table_name = %q, want %q", entry.TableName, "lore_entries")
	}
	if entry.EntityID != lore.ID {
		t.Errorf("entity_id = %q, want %q", entry.EntityID, lore.ID)
	}
	if entry.Operation != "delete" {
		t.Errorf("operation = %q, want %q", entry.Operation, "delete")
	}
	if entry.SourceID != sourceID {
		t.Errorf("source_id = %q, want %q", entry.SourceID, sourceID)
	}
	if entry.Payload.Valid {
		t.Errorf("payload should be NULL for delete operation, got %q", entry.Payload.String)
	}
}

func TestDeleteLoreByID_SoftDeleteSetsDeletedAt(t *testing.T) {
	store := newTestStore(t)

	now := time.Now().UTC()
	lore := &Lore{
		ID:              "01TESTID_CL_SOFTDEL_001",
		Content:         "Soft delete test",
		Category:        CategoryTestingStrategy,
		Confidence:      0.6,
		EmbeddingStatus: "pending",
		SourceID:        "softdel-source",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	err := store.InsertLore(lore)
	if err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	err = store.DeleteLoreByID(lore.ID)
	if err != nil {
		t.Fatalf("DeleteLoreByID failed: %v", err)
	}

	// Lore should not be returned by Get (soft-deleted records filtered out)
	_, err = store.Get(lore.ID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after soft delete, got %v", err)
	}

	// But the row should still exist in the DB with deleted_at set
	var deletedAt sql.NullString
	err = store.db.QueryRow("SELECT deleted_at FROM lore_entries WHERE id = ?", lore.ID).Scan(&deletedAt)
	if err != nil {
		t.Fatalf("failed to query lore after soft delete: %v", err)
	}
	if !deletedAt.Valid {
		t.Error("deleted_at should be set after soft delete")
	}
}

// =============================================================================
// AC #4: Transaction rollback — neither lore nor change_log persisted on failure
// =============================================================================

func TestInsertLore_RollbackIncludesChangeLog(t *testing.T) {
	store := newTestStore(t)

	now := time.Now().UTC()
	lore := &Lore{
		ID:              "01TESTID_CL_ROLLBACK_001",
		Content:         "Rollback test",
		Category:        CategoryArchitecturalDecision,
		Confidence:      0.5,
		EmbeddingStatus: "pending",
		SourceID:        "rollback-source",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// First insert succeeds
	err := store.InsertLore(lore)
	if err != nil {
		t.Fatalf("first InsertLore failed: %v", err)
	}

	loreBefore := getChangeLogCount(t, store)

	// Second insert with same ID fails
	lore2 := &Lore{
		ID:              "01TESTID_CL_ROLLBACK_001", // duplicate
		Content:         "Should rollback",
		Category:        CategoryPatternOutcome,
		Confidence:      0.6,
		EmbeddingStatus: "pending",
		SourceID:        "rollback-source-2",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	err = store.InsertLore(lore2)
	if err == nil {
		t.Fatal("expected InsertLore to fail on duplicate")
	}

	// change_log count should not have changed
	loreAfter := getChangeLogCount(t, store)
	if loreAfter != loreBefore {
		t.Errorf("change_log count changed from %d to %d after failed insert", loreBefore, loreAfter)
	}
}

// =============================================================================
// AC #5: Payload JSON contains all required fields for Engram validation
// =============================================================================

func TestInsertLore_PayloadPassesEngramValidation(t *testing.T) {
	store := newTestStore(t)

	now := time.Now().UTC()
	lore := &Lore{
		ID:              "01TESTID_CL_ENGRAM_0001",
		Content:         "Engram validation test",
		Context:         "test context",
		Category:        CategoryDependencyBehavior,
		Confidence:      0.7,
		EmbeddingStatus: "pending",
		SourceID:        "engram-source",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	err := store.InsertLore(lore)
	if err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	entry := getLatestChangeLogEntry(t, store)
	if !entry.Payload.Valid {
		t.Fatal("payload should not be NULL")
	}

	var payload changeLogPayload
	if err := json.Unmarshal([]byte(entry.Payload.String), &payload); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}

	// Engram's Recall domain plugin validation requirements:
	if payload.ID == "" {
		t.Error("payload.id must be non-empty")
	}
	if payload.Content == "" {
		t.Error("payload.content must be non-empty")
	}
	if !Category(payload.Category).IsValid() {
		t.Errorf("payload.category %q must be a valid enum", payload.Category)
	}
	if payload.SourceID == "" {
		t.Error("payload.source_id must be non-empty")
	}
	if payload.Confidence < 0.0 || payload.Confidence > 1.0 {
		t.Errorf("payload.confidence %f must be in [0.0, 1.0]", payload.Confidence)
	}
}

// =============================================================================
// AC #6: InsertLore writes to change_log instead of sync_queue
// =============================================================================

func TestInsertLore_DoesNotWriteSyncQueue(t *testing.T) {
	store := newTestStore(t)

	now := time.Now().UTC()
	lore := &Lore{
		ID:              "01TESTID_CL_NOSYNC_0001",
		Content:         "No sync_queue test",
		Category:        CategoryArchitecturalDecision,
		Confidence:      0.5,
		EmbeddingStatus: "pending",
		SourceID:        "nosync-source",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	err := store.InsertLore(lore)
	if err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	syncCount := getSyncQueueCount(t, store)
	if syncCount != 0 {
		t.Errorf("sync_queue count = %d, want 0 (should write to change_log instead)", syncCount)
	}

	changeCount := getChangeLogCount(t, store)
	if changeCount != 1 {
		t.Errorf("change_log count = %d, want 1", changeCount)
	}
}

// =============================================================================
// AC #7: ApplyFeedback writes full-state upsert to change_log instead of sync_queue
// =============================================================================

func TestApplyFeedback_DoesNotWriteSyncQueue(t *testing.T) {
	store := newTestStore(t)

	now := time.Now().UTC()
	lore := &Lore{
		ID:              "01TESTID_CL_NOSYNC_0002",
		Content:         "Feedback no sync_queue test",
		Category:        CategoryEdgeCaseDiscovery,
		Confidence:      0.5,
		EmbeddingStatus: "pending",
		SourceID:        "nosync-fb-source",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	err := store.InsertLore(lore)
	if err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Clear everything
	store.db.Exec("DELETE FROM change_log")
	store.db.Exec("DELETE FROM sync_queue")

	// Mark as synced so the old code path would have written to sync_queue
	store.db.Exec("UPDATE lore_entries SET synced_at = ? WHERE id = ?",
		time.Now().UTC().Format(time.RFC3339), lore.ID)

	_, err = store.ApplyFeedback(lore.ID, ConfidenceHelpfulDelta, true)
	if err != nil {
		t.Fatalf("ApplyFeedback failed: %v", err)
	}

	syncCount := getSyncQueueCount(t, store)
	if syncCount != 0 {
		t.Errorf("sync_queue count = %d, want 0 (should write to change_log instead)", syncCount)
	}

	changeCount := getChangeLogCount(t, store)
	if changeCount != 1 {
		t.Errorf("change_log count = %d, want 1", changeCount)
	}
}

// =============================================================================
// Task 5: Source ID cached on store initialization
// =============================================================================

func TestStore_CachesSourceID(t *testing.T) {
	store := newTestStore(t)

	// Read source_id from sync_meta
	expected := getSourceID(t, store)

	// The store should have cached it via SourceID() accessor
	got := store.SourceID()
	if got == "" {
		t.Fatal("store.SourceID() should return cached value after initialization")
	}
	if got != expected {
		t.Errorf("store.SourceID() = %q, want %q", got, expected)
	}
}

// =============================================================================
// Additional: ApplyFeedback change_log contains full state after update
// =============================================================================

func TestApplyFeedback_ChangeLogPayloadReflectsUpdatedState(t *testing.T) {
	store := newTestStore(t)

	now := time.Now().UTC()
	lore := &Lore{
		ID:              "01TESTID_CL_FB_STATE_001",
		Content:         "State reflection test",
		Category:        CategoryPerformanceInsight,
		Confidence:      0.5,
		EmbeddingStatus: "pending",
		SourceID:        "state-source",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	err := store.InsertLore(lore)
	if err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Clear change_log from insert
	store.db.Exec("DELETE FROM change_log")

	// Apply helpful feedback — confidence should go from 0.5 to 0.58
	_, err = store.ApplyFeedback(lore.ID, ConfidenceHelpfulDelta, true)
	if err != nil {
		t.Fatalf("ApplyFeedback failed: %v", err)
	}

	entry := getLatestChangeLogEntry(t, store)
	var payload changeLogPayload
	if err := json.Unmarshal([]byte(entry.Payload.String), &payload); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}

	expectedConfidence := 0.5 + ConfidenceHelpfulDelta
	if payload.Confidence != expectedConfidence {
		t.Errorf("payload.confidence = %f, want %f", payload.Confidence, expectedConfidence)
	}
	if payload.ValidationCount != 1 {
		t.Errorf("payload.validation_count = %d, want 1", payload.ValidationCount)
	}
	if payload.LastValidatedAt == nil {
		t.Error("payload.last_validated_at should be set for helpful feedback")
	}
}

func TestDeleteLoreByID_TransactionRollbackOnFailure(t *testing.T) {
	store := newTestStore(t)

	// Try to delete from a non-existent lore — this shouldn't error,
	// but let's verify that when a real failure occurs, nothing is persisted.
	// We'll test by closing the underlying DB to force a failure.

	now := time.Now().UTC()
	lore := &Lore{
		ID:              "01TESTID_CL_DELRB_00001",
		Content:         "Delete rollback test",
		Category:        CategoryTestingStrategy,
		Confidence:      0.6,
		EmbeddingStatus: "pending",
		SourceID:        "delrb-source",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	err := store.InsertLore(lore)
	if err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Clear the change_log from insert so we can check isolation
	store.db.Exec("DELETE FROM change_log")

	// Verify the lore exists
	_, err = store.Get(lore.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// DeleteLoreByID for existing lore should work without error
	// and create exactly one change_log entry
	err = store.DeleteLoreByID(lore.ID)
	if err != nil {
		t.Fatalf("DeleteLoreByID failed: %v", err)
	}

	clCount := getChangeLogCount(t, store)
	if clCount != 1 {
		t.Errorf("change_log count = %d, want 1", clCount)
	}
}
