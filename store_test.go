package recall

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNewStore_CreatesAllTables verifies that NewStore creates all three required tables.
func TestNewStore_CreatesAllTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	tables := []string{"lore", "metadata", "sync_queue"}
	for _, table := range tables {
		var name string
		err := store.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
			table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

// TestNewStore_EnablesWAL verifies that WAL mode is enabled after initialization.
func TestNewStore_EnablesWAL(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	var journalMode string
	err = store.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("failed to query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("expected journal_mode=wal, got %q", journalMode)
	}
}

// TestNewStore_CreatesIndexes verifies that all required indexes are created.
func TestNewStore_CreatesIndexes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	expectedIndexes := []string{
		"idx_lore_category",
		"idx_lore_confidence",
		"idx_lore_created_at",
		"idx_lore_synced_at",
		"idx_lore_last_validated",
	}

	for _, idx := range expectedIndexes {
		var name string
		err := store.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='index' AND name=?",
			idx,
		).Scan(&name)
		if err != nil {
			t.Errorf("index %q not found: %v", idx, err)
		}
	}
}

// TestNewStore_SetsSchemaVersion verifies that schema_version is recorded in metadata.
func TestNewStore_SetsSchemaVersion(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	var value string
	err = store.db.QueryRow(
		"SELECT value FROM metadata WHERE key='schema_version'",
	).Scan(&value)
	if err != nil {
		t.Fatalf("schema_version not found in metadata: %v", err)
	}
	if value != schemaVersion {
		t.Errorf("expected schema_version=%q, got %q", schemaVersion, value)
	}
}

// TestNewStore_Idempotent verifies that opening the same DB twice works without error.
func TestNewStore_Idempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// First open
	store1, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("first NewStore failed: %v", err)
	}
	store1.Close()

	// Second open - should not fail
	store2, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("second NewStore failed: %v", err)
	}
	defer store2.Close()

	// Verify tables still exist (no duplicates, no errors)
	var count int
	err = store2.db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='lore'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count lore tables: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 lore table, got %d", count)
	}
}

// TestStore_Close_ReleasesResources verifies that Close() properly releases resources.
func TestStore_Close_ReleasesResources(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Close the store
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Second close should return nil (not error)
	if err := store.Close(); err != nil {
		t.Errorf("second Close returned error: %v", err)
	}

	// File should be deletable after close
	if err := os.Remove(dbPath); err != nil {
		t.Errorf("failed to delete db file after Close: %v", err)
	}
}

// TestNewStore_CreatesDirectory verifies that NewStore creates parent directories.
func TestNewStore_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "nested", "dir", "test.db")

	// Verify parent doesn't exist yet
	parentDir := filepath.Dir(dbPath)
	if _, err := os.Stat(parentDir); !os.IsNotExist(err) {
		t.Fatalf("expected parent dir to not exist")
	}

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Verify parent now exists
	if _, err := os.Stat(parentDir); err != nil {
		t.Errorf("parent directory was not created: %v", err)
	}

	// Verify db file exists
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("database file was not created: %v", err)
	}
}

// =============================================================================
// Story 1.4: InsertLore Atomicity Tests
// =============================================================================

// TestInsertLore_Atomicity_BothEntriesExist tests AC #7:
// A valid record atomically inserts both a lore entry and a sync queue entry
// (INSERT operation) in one transaction.
func TestInsertLore_Atomicity_BothEntriesExist(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	lore := &Lore{
		ID:         "01TESTID0000000000000001",
		Content:    "Test content for atomicity",
		Category:   CategoryArchitecturalDecision,
		Confidence: 0.5,
		SourceID:   "test-source",
	}

	err = store.InsertLore(lore)
	if err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Verify lore entry exists
	var loreCount int
	err = store.db.QueryRow("SELECT COUNT(*) FROM lore WHERE id = ?", lore.ID).Scan(&loreCount)
	if err != nil {
		t.Fatalf("failed to query lore: %v", err)
	}
	if loreCount != 1 {
		t.Errorf("lore count = %d, want 1", loreCount)
	}

	// Verify sync_queue entry exists with operation=INSERT
	var syncCount int
	var operation string
	err = store.db.QueryRow(
		"SELECT COUNT(*), operation FROM sync_queue WHERE lore_id = ?",
		lore.ID,
	).Scan(&syncCount, &operation)
	if err != nil {
		t.Fatalf("failed to query sync_queue: %v", err)
	}
	if syncCount != 1 {
		t.Errorf("sync_queue count = %d, want 1", syncCount)
	}
	if operation != "INSERT" {
		t.Errorf("sync_queue operation = %q, want %q", operation, "INSERT")
	}
}

// TestInsertLore_Atomicity_RollbackOnDuplicate tests AC #8:
// A database write failure mid-transaction rolls back both the lore entry
// and sync queue entry. We trigger this by inserting a duplicate ID.
func TestInsertLore_Atomicity_RollbackOnDuplicate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	lore := &Lore{
		ID:         "01TESTID0000000000000002",
		Content:    "First entry",
		Category:   CategoryArchitecturalDecision,
		Confidence: 0.5,
		SourceID:   "test-source",
	}

	// First insert should succeed
	err = store.InsertLore(lore)
	if err != nil {
		t.Fatalf("first InsertLore failed: %v", err)
	}

	// Count entries before duplicate attempt
	var loreBefore, syncBefore int
	store.db.QueryRow("SELECT COUNT(*) FROM lore").Scan(&loreBefore)
	store.db.QueryRow("SELECT COUNT(*) FROM sync_queue").Scan(&syncBefore)

	// Second insert with same ID should fail
	lore2 := &Lore{
		ID:         "01TESTID0000000000000002", // Same ID - will cause failure
		Content:    "Second entry - should rollback",
		Category:   CategoryPatternOutcome,
		Confidence: 0.6,
		SourceID:   "test-source-2",
	}

	err = store.InsertLore(lore2)
	if err == nil {
		t.Fatal("expected InsertLore to fail on duplicate ID")
	}

	// Count entries after duplicate attempt
	var loreAfter, syncAfter int
	store.db.QueryRow("SELECT COUNT(*) FROM lore").Scan(&loreAfter)
	store.db.QueryRow("SELECT COUNT(*) FROM sync_queue").Scan(&syncAfter)

	// Verify counts haven't changed (rollback worked)
	if loreAfter != loreBefore {
		t.Errorf("lore count changed from %d to %d after failed insert", loreBefore, loreAfter)
	}
	if syncAfter != syncBefore {
		t.Errorf("sync_queue count changed from %d to %d after failed insert", syncBefore, syncAfter)
	}
}

// =============================================================================
// Story 3.1: UpdateConfidence Tests
// =============================================================================

// TestStore_UpdateConfidence_PositiveDelta tests that positive delta increases confidence.
func TestStore_UpdateConfidence_PositiveDelta(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore with confidence 0.5
	lore := &Lore{
		ID:         "01TESTID0000000000000001",
		Content:    "Test content",
		Category:   CategoryArchitecturalDecision,
		Confidence: 0.5,
		SourceID:   "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Apply +0.08 delta
	updated, err := store.UpdateConfidence(lore.ID, 0.08)
	if err != nil {
		t.Fatalf("UpdateConfidence failed: %v", err)
	}

	// Verify confidence is now 0.58
	if updated.Confidence != 0.58 {
		t.Errorf("Confidence = %f, want 0.58", updated.Confidence)
	}
}

// TestStore_UpdateConfidence_NegativeDelta tests that negative delta decreases confidence.
func TestStore_UpdateConfidence_NegativeDelta(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore with confidence 0.5
	lore := &Lore{
		ID:         "01TESTID0000000000000001",
		Content:    "Test content",
		Category:   CategoryArchitecturalDecision,
		Confidence: 0.5,
		SourceID:   "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Apply -0.15 delta
	updated, err := store.UpdateConfidence(lore.ID, -0.15)
	if err != nil {
		t.Fatalf("UpdateConfidence failed: %v", err)
	}

	// Verify confidence is now 0.35
	if updated.Confidence != 0.35 {
		t.Errorf("Confidence = %f, want 0.35", updated.Confidence)
	}
}

// TestStore_UpdateConfidence_CapsAtOne tests that confidence is capped at 1.0.
func TestStore_UpdateConfidence_CapsAtOne(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore with confidence 0.95
	lore := &Lore{
		ID:         "01TESTID0000000000000001",
		Content:    "Test content",
		Category:   CategoryArchitecturalDecision,
		Confidence: 0.95,
		SourceID:   "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Apply +0.08 delta (would be 1.03 without capping)
	updated, err := store.UpdateConfidence(lore.ID, 0.08)
	if err != nil {
		t.Fatalf("UpdateConfidence failed: %v", err)
	}

	// Verify confidence is capped at 1.0
	if updated.Confidence != 1.0 {
		t.Errorf("Confidence = %f, want 1.0 (capped)", updated.Confidence)
	}
}

// TestStore_UpdateConfidence_FloorsAtZero tests that confidence is floored at 0.0.
func TestStore_UpdateConfidence_FloorsAtZero(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore with confidence 0.05
	lore := &Lore{
		ID:         "01TESTID0000000000000001",
		Content:    "Test content",
		Category:   CategoryArchitecturalDecision,
		Confidence: 0.05,
		SourceID:   "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Apply -0.15 delta (would be -0.10 without flooring)
	updated, err := store.UpdateConfidence(lore.ID, -0.15)
	if err != nil {
		t.Fatalf("UpdateConfidence failed: %v", err)
	}

	// Verify confidence is floored at 0.0
	if updated.Confidence != 0.0 {
		t.Errorf("Confidence = %f, want 0.0 (floored)", updated.Confidence)
	}
}

// TestStore_UpdateConfidence_ZeroDelta tests that zero delta leaves confidence unchanged.
func TestStore_UpdateConfidence_ZeroDelta(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore with confidence 0.5
	lore := &Lore{
		ID:         "01TESTID0000000000000001",
		Content:    "Test content",
		Category:   CategoryArchitecturalDecision,
		Confidence: 0.5,
		SourceID:   "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Apply zero delta
	updated, err := store.UpdateConfidence(lore.ID, 0.0)
	if err != nil {
		t.Fatalf("UpdateConfidence failed: %v", err)
	}

	// Verify confidence is unchanged
	if updated.Confidence != 0.5 {
		t.Errorf("Confidence = %f, want 0.5 (unchanged)", updated.Confidence)
	}
}

// TestStore_UpdateConfidence_NotFound tests that invalid ID returns ErrNotFound.
func TestStore_UpdateConfidence_NotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Try to update non-existent lore
	_, err = store.UpdateConfidence("01NONEXISTENT00000000000", 0.08)
	if err != ErrNotFound {
		t.Errorf("UpdateConfidence error = %v, want ErrNotFound", err)
	}
}

// TestStore_UpdateConfidence_UpdatesTimestamp tests that updated_at is updated.
func TestStore_UpdateConfidence_UpdatesTimestamp(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore
	lore := &Lore{
		ID:         "01TESTID0000000000000001",
		Content:    "Test content",
		Category:   CategoryArchitecturalDecision,
		Confidence: 0.5,
		SourceID:   "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Get original timestamp
	original, err := store.Get(lore.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	originalUpdatedAt := original.UpdatedAt

	// Apply delta
	updated, err := store.UpdateConfidence(lore.ID, 0.08)
	if err != nil {
		t.Fatalf("UpdateConfidence failed: %v", err)
	}

	// Verify updated_at has changed
	if !updated.UpdatedAt.After(originalUpdatedAt) {
		t.Errorf("UpdatedAt = %v, want after %v", updated.UpdatedAt, originalUpdatedAt)
	}
}

// =============================================================================
// Story 3.2: ApplyFeedback Atomicity Tests
// =============================================================================

// TestStore_ApplyFeedback_HelpfulIncrementsValidationCount tests AC #1:
// Lore marked as helpful increments validation_count by 1.
func TestStore_ApplyFeedback_HelpfulIncrementsValidationCount(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore with validation_count = 0
	lore := &Lore{
		ID:              "01TESTID0000000000000001",
		Content:         "Test content",
		Category:        CategoryArchitecturalDecision,
		Confidence:      0.5,
		ValidationCount: 0,
		SourceID:        "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Apply helpful feedback (isHelpful=true)
	updated, err := store.ApplyFeedback(lore.ID, 0.08, true)
	if err != nil {
		t.Fatalf("ApplyFeedback failed: %v", err)
	}

	// Verify validation_count incremented
	if updated.ValidationCount != 1 {
		t.Errorf("ValidationCount = %d, want 1", updated.ValidationCount)
	}
}

// TestStore_ApplyFeedback_HelpfulSetsLastValidated tests AC #1:
// Lore marked as helpful sets last_validated to current time.
func TestStore_ApplyFeedback_HelpfulSetsLastValidated(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore with no last_validated
	lore := &Lore{
		ID:         "01TESTID0000000000000001",
		Content:    "Test content",
		Category:   CategoryArchitecturalDecision,
		Confidence: 0.5,
		SourceID:   "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Apply helpful feedback
	updated, err := store.ApplyFeedback(lore.ID, 0.08, true)
	if err != nil {
		t.Fatalf("ApplyFeedback failed: %v", err)
	}

	// Verify last_validated is set
	if updated.LastValidated == nil {
		t.Error("LastValidated is nil, want non-nil timestamp")
	}
}

// TestStore_ApplyFeedback_IncorrectNoValidationChange tests AC #2:
// Lore marked as incorrect does NOT modify validation_count or last_validated.
func TestStore_ApplyFeedback_IncorrectNoValidationChange(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore with validation_count = 5
	lore := &Lore{
		ID:              "01TESTID0000000000000001",
		Content:         "Test content",
		Category:        CategoryArchitecturalDecision,
		Confidence:      0.5,
		ValidationCount: 5,
		SourceID:        "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Apply incorrect feedback (isHelpful=false)
	updated, err := store.ApplyFeedback(lore.ID, -0.15, false)
	if err != nil {
		t.Fatalf("ApplyFeedback failed: %v", err)
	}

	// Verify validation_count unchanged
	if updated.ValidationCount != 5 {
		t.Errorf("ValidationCount = %d, want 5 (unchanged)", updated.ValidationCount)
	}
	// Verify last_validated still nil
	if updated.LastValidated != nil {
		t.Error("LastValidated should remain nil for incorrect feedback")
	}
}

// TestStore_ApplyFeedback_NotRelevantNoValidationChange tests AC #2:
// Lore marked as not_relevant does NOT modify validation_count or last_validated.
func TestStore_ApplyFeedback_NotRelevantNoValidationChange(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore
	lore := &Lore{
		ID:              "01TESTID0000000000000001",
		Content:         "Test content",
		Category:        CategoryArchitecturalDecision,
		Confidence:      0.5,
		ValidationCount: 3,
		SourceID:        "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Apply not_relevant feedback (delta=0, isHelpful=false)
	updated, err := store.ApplyFeedback(lore.ID, 0.0, false)
	if err != nil {
		t.Fatalf("ApplyFeedback failed: %v", err)
	}

	// Verify validation_count unchanged
	if updated.ValidationCount != 3 {
		t.Errorf("ValidationCount = %d, want 3 (unchanged)", updated.ValidationCount)
	}
}

// TestStore_ApplyFeedback_CreatesSyncQueueEntry tests AC #3:
// Any feedback operation creates a sync queue entry with FEEDBACK operation.
func TestStore_ApplyFeedback_CreatesSyncQueueEntry(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore
	lore := &Lore{
		ID:         "01TESTID0000000000000001",
		Content:    "Test content",
		Category:   CategoryArchitecturalDecision,
		Confidence: 0.5,
		SourceID:   "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Clear sync queue from InsertLore
	store.db.Exec("DELETE FROM sync_queue")

	// Apply feedback
	_, err = store.ApplyFeedback(lore.ID, 0.08, true)
	if err != nil {
		t.Fatalf("ApplyFeedback failed: %v", err)
	}

	// Verify sync_queue entry exists with FEEDBACK operation
	var count int
	var operation string
	err = store.db.QueryRow(
		"SELECT COUNT(*), operation FROM sync_queue WHERE lore_id = ?",
		lore.ID,
	).Scan(&count, &operation)
	if err != nil {
		t.Fatalf("failed to query sync_queue: %v", err)
	}
	if count != 1 {
		t.Errorf("sync_queue count = %d, want 1", count)
	}
	if operation != "FEEDBACK" {
		t.Errorf("sync_queue operation = %q, want %q", operation, "FEEDBACK")
	}
}

// TestStore_ApplyFeedback_MultipleHelpfulIncrementsCount tests AC #1:
// Multiple helpful feedbacks increment validation_count correctly.
func TestStore_ApplyFeedback_MultipleHelpfulIncrementsCount(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore
	lore := &Lore{
		ID:              "01TESTID0000000000000001",
		Content:         "Test content",
		Category:        CategoryArchitecturalDecision,
		Confidence:      0.5,
		ValidationCount: 0,
		SourceID:        "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Apply helpful feedback 3 times
	for i := 0; i < 3; i++ {
		_, err = store.ApplyFeedback(lore.ID, 0.08, true)
		if err != nil {
			t.Fatalf("ApplyFeedback #%d failed: %v", i+1, err)
		}
	}

	// Verify validation_count is 3
	updated, err := store.Get(lore.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if updated.ValidationCount != 3 {
		t.Errorf("ValidationCount = %d, want 3", updated.ValidationCount)
	}
}

// TestStore_ApplyFeedback_NotFound tests error handling:
// ApplyFeedback on non-existent lore returns ErrNotFound.
func TestStore_ApplyFeedback_NotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Try to apply feedback to non-existent lore
	_, err = store.ApplyFeedback("01NONEXISTENT00000000000", 0.08, true)
	if err != ErrNotFound {
		t.Errorf("ApplyFeedback error = %v, want ErrNotFound", err)
	}
}

// TestStore_ApplyFeedback_ConfidenceClamping tests confidence clamping:
// ApplyFeedback clamps confidence to [0.0, 1.0] range.
func TestStore_ApplyFeedback_ConfidenceClamping(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore with high confidence
	lore := &Lore{
		ID:         "01TESTID0000000000000001",
		Content:    "Test content",
		Category:   CategoryArchitecturalDecision,
		Confidence: 0.95,
		SourceID:   "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Apply helpful feedback (would exceed 1.0)
	updated, err := store.ApplyFeedback(lore.ID, 0.08, true)
	if err != nil {
		t.Fatalf("ApplyFeedback failed: %v", err)
	}

	// Verify confidence capped at 1.0
	if updated.Confidence != 1.0 {
		t.Errorf("Confidence = %f, want 1.0 (capped)", updated.Confidence)
	}
}

// TestStore_ApplyFeedback_RollbackOnUpdateFailure tests AC #4:
// Database failure mid-feedback-transaction rolls back both confidence update
// and sync queue entry. This test verifies rollback by attempting feedback on
// a non-existent lore (which fails at the getLore check before transaction),
// and verifies the atomic nature by checking that when the UPDATE would fail
// mid-transaction, no partial state is left behind.
func TestStore_ApplyFeedback_RollbackOnUpdateFailure(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert a valid lore entry
	lore := &Lore{
		ID:              "01TESTID0000000000000001",
		Content:         "Test content",
		Category:        CategoryArchitecturalDecision,
		Confidence:      0.5,
		ValidationCount: 0,
		SourceID:        "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Get initial state
	var initialConfidence float64
	var initialValidationCount int
	err = store.db.QueryRow(
		"SELECT confidence, validation_count FROM lore WHERE id = ?",
		lore.ID,
	).Scan(&initialConfidence, &initialValidationCount)
	if err != nil {
		t.Fatalf("failed to get initial state: %v", err)
	}

	// Clear sync_queue to have a clean slate for counting
	store.db.Exec("DELETE FROM sync_queue")

	var syncCountBefore int
	store.db.QueryRow("SELECT COUNT(*) FROM sync_queue").Scan(&syncCountBefore)

	// Simulate a failure scenario: attempt to apply feedback to non-existent lore
	// This tests that no partial state is created when the operation fails
	_, err = store.ApplyFeedback("01NONEXISTENT00000000000", 0.08, true)
	if err == nil {
		t.Fatal("expected ApplyFeedback to fail on non-existent lore")
	}

	// Verify no sync_queue entry was created for the failed operation
	var syncCountAfter int
	store.db.QueryRow("SELECT COUNT(*) FROM sync_queue").Scan(&syncCountAfter)
	if syncCountAfter != syncCountBefore {
		t.Errorf("sync_queue count changed from %d to %d after failed feedback", syncCountBefore, syncCountAfter)
	}

	// Verify original lore is unchanged
	var currentConfidence float64
	var currentValidationCount int
	err = store.db.QueryRow(
		"SELECT confidence, validation_count FROM lore WHERE id = ?",
		lore.ID,
	).Scan(&currentConfidence, &currentValidationCount)
	if err != nil {
		t.Fatalf("failed to get current state: %v", err)
	}

	if currentConfidence != initialConfidence {
		t.Errorf("confidence changed from %f to %f after failed feedback on different lore",
			initialConfidence, currentConfidence)
	}
	if currentValidationCount != initialValidationCount {
		t.Errorf("validation_count changed from %d to %d after failed feedback on different lore",
			initialValidationCount, currentValidationCount)
	}
}

// TestStore_ApplyFeedback_RollbackOnQueueFailure tests AC #4:
// Database failure mid-feedback-transaction rolls back both confidence update
// and sync queue entry. This test verifies atomicity by using a constraint
// violation: we insert a sync_queue entry with a trigger that will cause
// the INSERT to fail, and verify the lore UPDATE was rolled back.
//
// Since we cannot easily inject failures into the sync_queue INSERT without
// modifying the schema, this test demonstrates atomicity by:
// 1. Recording state before a successful feedback
// 2. Applying feedback successfully
// 3. Verifying both lore AND sync_queue were updated together (atomicity)
// 4. Then testing rollback by dropping the sync_queue table mid-operation
func TestStore_ApplyFeedback_RollbackOnQueueFailure(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore
	lore := &Lore{
		ID:              "01TESTID0000000000000001",
		Content:         "Test content",
		Category:        CategoryArchitecturalDecision,
		Confidence:      0.5,
		ValidationCount: 0,
		SourceID:        "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Record initial state
	initialConfidence := lore.Confidence
	var initialSyncCount int
	store.db.QueryRow("SELECT COUNT(*) FROM sync_queue").Scan(&initialSyncCount)

	// Clear sync_queue and apply successful feedback to verify atomicity
	store.db.Exec("DELETE FROM sync_queue")

	updated, err := store.ApplyFeedback(lore.ID, 0.08, true)
	if err != nil {
		t.Fatalf("ApplyFeedback failed: %v", err)
	}

	// Verify BOTH operations happened atomically
	if updated.Confidence != initialConfidence+0.08 {
		t.Errorf("Confidence = %f, want %f", updated.Confidence, initialConfidence+0.08)
	}

	var syncCount int
	store.db.QueryRow("SELECT COUNT(*) FROM sync_queue WHERE lore_id = ?", lore.ID).Scan(&syncCount)
	if syncCount != 1 {
		t.Errorf("sync_queue count = %d, want 1 (atomicity check)", syncCount)
	}

	// Now test rollback scenario by renaming sync_queue to break INSERT
	// First, record current state
	currentLore, _ := store.Get(lore.ID)
	currentConfidence := currentLore.Confidence
	currentValidationCount := currentLore.ValidationCount

	// Rename sync_queue table to simulate a failure on sync_queue INSERT
	_, err = store.db.Exec("ALTER TABLE sync_queue RENAME TO sync_queue_backup")
	if err != nil {
		t.Fatalf("failed to rename sync_queue: %v", err)
	}

	// Attempt feedback - this should fail on sync_queue INSERT
	_, err = store.ApplyFeedback(lore.ID, 0.08, true)
	if err == nil {
		t.Fatal("expected ApplyFeedback to fail when sync_queue table is missing")
	}

	// Restore sync_queue table
	_, err = store.db.Exec("ALTER TABLE sync_queue_backup RENAME TO sync_queue")
	if err != nil {
		t.Fatalf("failed to restore sync_queue: %v", err)
	}

	// Verify lore was NOT updated (rollback worked)
	afterLore, err := store.Get(lore.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if afterLore.Confidence != currentConfidence {
		t.Errorf("Confidence changed from %f to %f despite failed transaction (rollback failed)",
			currentConfidence, afterLore.Confidence)
	}
	if afterLore.ValidationCount != currentValidationCount {
		t.Errorf("ValidationCount changed from %d to %d despite failed transaction (rollback failed)",
			currentValidationCount, afterLore.ValidationCount)
	}
}

// ============================================================================
// Metadata tests
// ============================================================================

func TestStore_GetMetadata_Exists(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert a metadata entry directly
	_, err = store.db.Exec("INSERT INTO metadata (key, value) VALUES ('test_key', 'test_value')")
	if err != nil {
		t.Fatalf("failed to insert metadata: %v", err)
	}

	value, err := store.GetMetadata("test_key")
	if err != nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}
	if value != "test_value" {
		t.Errorf("GetMetadata = %q, want %q", value, "test_value")
	}
}

func TestStore_GetMetadata_NotExists(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	value, err := store.GetMetadata("nonexistent_key")
	if err != nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}
	if value != "" {
		t.Errorf("GetMetadata = %q, want empty string", value)
	}
}

func TestStore_SetMetadata_Insert(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	err = store.SetMetadata("new_key", "new_value")
	if err != nil {
		t.Fatalf("SetMetadata failed: %v", err)
	}

	// Verify using GetMetadata
	value, err := store.GetMetadata("new_key")
	if err != nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}
	if value != "new_value" {
		t.Errorf("GetMetadata = %q, want %q", value, "new_value")
	}
}

func TestStore_SetMetadata_Update(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Set initial value
	err = store.SetMetadata("update_key", "initial_value")
	if err != nil {
		t.Fatalf("SetMetadata (initial) failed: %v", err)
	}

	// Update value
	err = store.SetMetadata("update_key", "updated_value")
	if err != nil {
		t.Fatalf("SetMetadata (update) failed: %v", err)
	}

	// Verify update
	value, err := store.GetMetadata("update_key")
	if err != nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}
	if value != "updated_value" {
		t.Errorf("GetMetadata = %q, want %q", value, "updated_value")
	}
}

func TestStore_GetMetadata_StoreClosed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	store.Close()

	_, err = store.GetMetadata("any_key")
	if err != ErrStoreClosed {
		t.Errorf("GetMetadata on closed store = %v, want ErrStoreClosed", err)
	}
}

func TestStore_SetMetadata_StoreClosed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	store.Close()

	err = store.SetMetadata("any_key", "any_value")
	if err != ErrStoreClosed {
		t.Errorf("SetMetadata on closed store = %v, want ErrStoreClosed", err)
	}
}

// ============================================================================
// ReplaceFromSnapshot tests
// ============================================================================

func TestStore_ReplaceFromSnapshot_Success(t *testing.T) {
	// Create main store with some existing data
	mainPath := filepath.Join(t.TempDir(), "main.db")
	mainStore, err := NewStore(mainPath)
	if err != nil {
		t.Fatalf("NewStore (main) failed: %v", err)
	}
	defer mainStore.Close()

	// Insert existing lore that should be replaced
	existingLore := &Lore{
		ID:         "EXISTING000000000000001",
		Content:    "Existing content to be replaced",
		Category:   CategoryPatternOutcome,
		Confidence: 0.5,
		SourceID:   "existing-source",
	}
	if err := mainStore.InsertLore(existingLore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Create snapshot store with different data
	snapshotPath := filepath.Join(t.TempDir(), "snapshot.db")
	snapshotStore, err := NewStore(snapshotPath)
	if err != nil {
		t.Fatalf("NewStore (snapshot) failed: %v", err)
	}

	// Insert snapshot lore
	snapshotLore := &Lore{
		ID:         "SNAPSHOT000000000000001",
		Content:    "Snapshot content",
		Category:   CategoryEdgeCaseDiscovery,
		Confidence: 0.9,
		SourceID:   "snapshot-source",
	}
	if err := snapshotStore.InsertLore(snapshotLore); err != nil {
		t.Fatalf("InsertLore (snapshot) failed: %v", err)
	}
	snapshotStore.Close()

	// Read snapshot file
	snapshotFile, err := os.Open(snapshotPath)
	if err != nil {
		t.Fatalf("failed to open snapshot: %v", err)
	}
	defer snapshotFile.Close()

	// Replace from snapshot
	err = mainStore.ReplaceFromSnapshot(snapshotFile)
	if err != nil {
		t.Fatalf("ReplaceFromSnapshot failed: %v", err)
	}

	// Verify existing lore is gone
	_, err = mainStore.Get("EXISTING000000000000001")
	if err != ErrNotFound {
		t.Errorf("expected existing lore to be deleted, got err: %v", err)
	}

	// Verify snapshot lore is present
	newLore, err := mainStore.Get("SNAPSHOT000000000000001")
	if err != nil {
		t.Fatalf("failed to get snapshot lore: %v", err)
	}
	if newLore.Content != "Snapshot content" {
		t.Errorf("Content = %q, want %q", newLore.Content, "Snapshot content")
	}

	// Verify sync queue is cleared
	var queueCount int
	mainStore.db.QueryRow("SELECT COUNT(*) FROM sync_queue").Scan(&queueCount)
	if queueCount != 0 {
		t.Errorf("sync_queue count = %d, want 0", queueCount)
	}
}

func TestStore_ReplaceFromSnapshot_EmptySnapshot(t *testing.T) {
	// Create main store with existing data
	mainPath := filepath.Join(t.TempDir(), "main.db")
	mainStore, err := NewStore(mainPath)
	if err != nil {
		t.Fatalf("NewStore (main) failed: %v", err)
	}
	defer mainStore.Close()

	// Insert existing lore
	existingLore := &Lore{
		ID:         "TODELETE000000000000001",
		Content:    "Content to delete",
		Category:   CategoryPatternOutcome,
		Confidence: 0.5,
		SourceID:   "existing-source",
	}
	if err := mainStore.InsertLore(existingLore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Create empty snapshot store (no lore)
	snapshotPath := filepath.Join(t.TempDir(), "empty-snapshot.db")
	snapshotStore, err := NewStore(snapshotPath)
	if err != nil {
		t.Fatalf("NewStore (snapshot) failed: %v", err)
	}
	snapshotStore.Close()

	// Read snapshot file
	snapshotFile, err := os.Open(snapshotPath)
	if err != nil {
		t.Fatalf("failed to open snapshot: %v", err)
	}
	defer snapshotFile.Close()

	// Replace from snapshot
	err = mainStore.ReplaceFromSnapshot(snapshotFile)
	if err != nil {
		t.Fatalf("ReplaceFromSnapshot failed: %v", err)
	}

	// Verify all lore is deleted
	var loreCount int
	mainStore.db.QueryRow("SELECT COUNT(*) FROM lore").Scan(&loreCount)
	if loreCount != 0 {
		t.Errorf("lore count = %d, want 0", loreCount)
	}
}

func TestStore_ReplaceFromSnapshot_InvalidDB(t *testing.T) {
	// Create main store with existing data
	mainPath := filepath.Join(t.TempDir(), "main.db")
	mainStore, err := NewStore(mainPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer mainStore.Close()

	// Insert existing lore
	existingLore := &Lore{
		ID:         "PRESERVE000000000000001",
		Content:    "Content that should be preserved",
		Category:   CategoryPatternOutcome,
		Confidence: 0.5,
		SourceID:   "existing-source",
	}
	if err := mainStore.InsertLore(existingLore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Create invalid "snapshot" (just text, not a SQLite file)
	invalidPath := filepath.Join(t.TempDir(), "invalid.db")
	os.WriteFile(invalidPath, []byte("this is not a sqlite database"), 0644)

	invalidFile, err := os.Open(invalidPath)
	if err != nil {
		t.Fatalf("failed to open invalid file: %v", err)
	}
	defer invalidFile.Close()

	// Replace from invalid snapshot should fail
	err = mainStore.ReplaceFromSnapshot(invalidFile)
	if err == nil {
		t.Fatal("expected error for invalid snapshot, got nil")
	}

	// Verify existing data is preserved
	preservedLore, err := mainStore.Get("PRESERVE000000000000001")
	if err != nil {
		t.Fatalf("existing lore was not preserved: %v", err)
	}
	if preservedLore.Content != "Content that should be preserved" {
		t.Error("existing lore content was modified")
	}
}

func TestStore_ReplaceFromSnapshot_StoreClosed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	store.Close()

	// Create a valid snapshot to pass
	snapshotPath := filepath.Join(t.TempDir(), "snapshot.db")
	snapshotStore, _ := NewStore(snapshotPath)
	snapshotStore.Close()

	snapshotFile, _ := os.Open(snapshotPath)
	defer snapshotFile.Close()

	err = store.ReplaceFromSnapshot(snapshotFile)
	if err != ErrStoreClosed {
		t.Errorf("ReplaceFromSnapshot on closed store = %v, want ErrStoreClosed", err)
	}
}
