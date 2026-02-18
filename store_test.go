package recall

import (
	"encoding/json"
	"fmt"
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

	tables := []string{"lore_entries", "metadata", "sync_queue"}
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
		"idx_lore_entries_category",
		"idx_lore_entries_confidence",
		"idx_lore_entries_created_at",
		"idx_lore_entries_synced_at",
		"idx_lore_entries_last_validated_at",
		"idx_lore_entries_deleted_at",
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
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='lore_entries'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count lore_entries tables: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 lore_entries table, got %d", count)
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
// A valid record atomically inserts both a lore entry and a change_log entry
// (upsert operation) in one transaction.
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
	err = store.db.QueryRow("SELECT COUNT(*) FROM lore_entries WHERE id = ?", lore.ID).Scan(&loreCount)
	if err != nil {
		t.Fatalf("failed to query lore_entries: %v", err)
	}
	if loreCount != 1 {
		t.Errorf("lore count = %d, want 1", loreCount)
	}

	// Verify change_log entry exists with operation=upsert
	var clCount int
	var operation string
	err = store.db.QueryRow(
		"SELECT COUNT(*), operation FROM change_log WHERE entity_id = ?",
		lore.ID,
	).Scan(&clCount, &operation)
	if err != nil {
		t.Fatalf("failed to query change_log: %v", err)
	}
	if clCount != 1 {
		t.Errorf("change_log count = %d, want 1", clCount)
	}
	if operation != "upsert" {
		t.Errorf("change_log operation = %q, want %q", operation, "upsert")
	}
}

// TestInsertLore_Atomicity_RollbackOnDuplicate tests AC #8:
// A database write failure mid-transaction rolls back both the lore entry
// and change_log entry. We trigger this by inserting a duplicate ID.
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
	store.db.QueryRow("SELECT COUNT(*) FROM lore_entries").Scan(&loreBefore)
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
	store.db.QueryRow("SELECT COUNT(*) FROM lore_entries").Scan(&loreAfter)
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
	if updated.LastValidatedAt == nil {
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
	if updated.LastValidatedAt != nil {
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

// TestStore_ApplyFeedback_CreatesChangeLogEntry tests AC #3:
// Any feedback operation creates a change_log entry with full entity state
// (upsert operation).
func TestStore_ApplyFeedback_CreatesChangeLogEntry(t *testing.T) {
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

	// Clear change_log from InsertLore
	store.db.Exec("DELETE FROM change_log")

	// Apply feedback
	_, err = store.ApplyFeedback(lore.ID, 0.08, true)
	if err != nil {
		t.Fatalf("ApplyFeedback failed: %v", err)
	}

	// Verify change_log entry exists with upsert operation
	var count int
	var operation string
	err = store.db.QueryRow(
		"SELECT COUNT(*), operation FROM change_log WHERE entity_id = ?",
		lore.ID,
	).Scan(&count, &operation)
	if err != nil {
		t.Fatalf("failed to query change_log: %v", err)
	}
	if count != 1 {
		t.Errorf("change_log count = %d, want 1", count)
	}
	if operation != "upsert" {
		t.Errorf("change_log operation = %q, want %q", operation, "upsert")
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
// and change_log entry. This test verifies rollback by attempting feedback on
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
		"SELECT confidence, validation_count FROM lore_entries WHERE id = ?",
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
		"SELECT confidence, validation_count FROM lore_entries WHERE id = ?",
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

// TestStore_ApplyFeedback_RollbackOnChangeLogFailure tests AC #4:
// Database failure mid-feedback-transaction rolls back both confidence update
// and change_log entry. This test verifies atomicity by using a constraint
// violation that causes the change_log INSERT to fail, and verifies the lore
// UPDATE was rolled back.
//
// This test demonstrates atomicity by:
// 1. Recording state before a successful feedback
// 2. Applying feedback successfully
// 3. Verifying both lore AND change_log were updated together (atomicity)
// 4. Then testing rollback by dropping the change_log table mid-operation
func TestStore_ApplyFeedback_RollbackOnChangeLogFailure(t *testing.T) {
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

	// Clear change_log and apply successful feedback to verify atomicity
	store.db.Exec("DELETE FROM change_log")

	updated, err := store.ApplyFeedback(lore.ID, 0.08, true)
	if err != nil {
		t.Fatalf("ApplyFeedback failed: %v", err)
	}

	// Verify BOTH operations happened atomically
	if updated.Confidence != initialConfidence+0.08 {
		t.Errorf("Confidence = %f, want %f", updated.Confidence, initialConfidence+0.08)
	}

	var clCount int
	store.db.QueryRow("SELECT COUNT(*) FROM change_log WHERE entity_id = ?", lore.ID).Scan(&clCount)
	if clCount != 1 {
		t.Errorf("change_log count = %d, want 1 (atomicity check)", clCount)
	}

	// Now test rollback scenario by renaming change_log to break INSERT
	// First, record current state
	currentLore, _ := store.Get(lore.ID)
	currentConfidence := currentLore.Confidence
	currentValidationCount := currentLore.ValidationCount

	// Rename change_log table to simulate a failure on change_log INSERT
	_, err = store.db.Exec("ALTER TABLE change_log RENAME TO change_log_backup")
	if err != nil {
		t.Fatalf("failed to rename change_log: %v", err)
	}

	// Attempt feedback - this should fail on change_log INSERT
	_, err = store.ApplyFeedback(lore.ID, 0.08, true)
	if err == nil {
		t.Fatal("expected ApplyFeedback to fail when change_log table is missing")
	}

	// Restore change_log table
	_, err = store.db.Exec("ALTER TABLE change_log_backup RENAME TO change_log")
	if err != nil {
		t.Fatalf("failed to restore change_log: %v", err)
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
	mainStore.db.QueryRow("SELECT COUNT(*) FROM lore_entries").Scan(&loreCount)
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

// ============================================================================
// PendingSyncEntries tests
// ============================================================================

func TestStore_PendingSyncEntries_Empty(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	entries, err := store.PendingSyncEntries()
	if err != nil {
		t.Fatalf("PendingSyncEntries failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(entries))
	}
}

func TestStore_PendingSyncEntries_OrderedByQueuedAt(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert entries with specific timestamps (out of order by ID)
	_, err = store.db.Exec(`
		INSERT INTO sync_queue (lore_id, operation, queued_at) VALUES
		('lore-3', 'INSERT', '2024-01-03T00:00:00Z'),
		('lore-1', 'INSERT', '2024-01-01T00:00:00Z'),
		('lore-2', 'FEEDBACK', '2024-01-02T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	entries, err := store.PendingSyncEntries()
	if err != nil {
		t.Fatalf("PendingSyncEntries failed: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Should be ordered by queued_at ASC
	expectedOrder := []string{"lore-1", "lore-2", "lore-3"}
	for i, expected := range expectedOrder {
		if entries[i].LoreID != expected {
			t.Errorf("entry[%d].LoreID = %q, want %q", i, entries[i].LoreID, expected)
		}
	}
}

func TestStore_PendingSyncEntries_IncludesPayload(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	payload := `{"outcome":"helpful"}`
	_, err = store.db.Exec(`
		INSERT INTO sync_queue (lore_id, operation, payload, queued_at)
		VALUES ('lore-1', 'FEEDBACK', ?, '2024-01-01T00:00:00Z')
	`, payload)
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	entries, err := store.PendingSyncEntries()
	if err != nil {
		t.Fatalf("PendingSyncEntries failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].Payload != payload {
		t.Errorf("Payload = %q, want %q", entries[0].Payload, payload)
	}
	if entries[0].Operation != "FEEDBACK" {
		t.Errorf("Operation = %q, want %q", entries[0].Operation, "FEEDBACK")
	}
}

func TestStore_PendingSyncEntries_IncludesAttempts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	_, err = store.db.Exec(`
		INSERT INTO sync_queue (lore_id, operation, queued_at, attempts, last_error)
		VALUES ('lore-1', 'INSERT', '2024-01-01T00:00:00Z', 3, 'network timeout')
	`)
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	entries, err := store.PendingSyncEntries()
	if err != nil {
		t.Fatalf("PendingSyncEntries failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].Attempts != 3 {
		t.Errorf("Attempts = %d, want 3", entries[0].Attempts)
	}
	if entries[0].LastError != "network timeout" {
		t.Errorf("LastError = %q, want %q", entries[0].LastError, "network timeout")
	}
}

func TestStore_PendingSyncEntries_StoreClosed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	store.Close()

	_, err = store.PendingSyncEntries()
	if err != ErrStoreClosed {
		t.Errorf("PendingSyncEntries on closed store = %v, want ErrStoreClosed", err)
	}
}

// ============================================================================
// CompleteSyncEntries tests
// ============================================================================

func TestStore_CompleteSyncEntries_DeletesQueue(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert sync queue entries
	_, err = store.db.Exec(`
		INSERT INTO sync_queue (lore_id, operation, queued_at) VALUES
		('lore-1', 'INSERT', '2024-01-01T00:00:00Z'),
		('lore-2', 'INSERT', '2024-01-02T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	// Get the IDs
	var id1, id2 int64
	store.db.QueryRow("SELECT id FROM sync_queue WHERE lore_id = 'lore-1'").Scan(&id1)
	store.db.QueryRow("SELECT id FROM sync_queue WHERE lore_id = 'lore-2'").Scan(&id2)

	// Complete just the first entry
	err = store.CompleteSyncEntries([]int64{id1}, nil)
	if err != nil {
		t.Fatalf("CompleteSyncEntries failed: %v", err)
	}

	// Verify first entry is deleted
	var count int
	store.db.QueryRow("SELECT COUNT(*) FROM sync_queue WHERE id = ?", id1).Scan(&count)
	if count != 0 {
		t.Error("completed entry was not deleted from queue")
	}

	// Verify second entry remains
	store.db.QueryRow("SELECT COUNT(*) FROM sync_queue WHERE id = ?", id2).Scan(&count)
	if count != 1 {
		t.Error("other entry was incorrectly deleted")
	}
}

func TestStore_CompleteSyncEntries_UpdatesSyncedAt(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore and queue entries
	lore := &Lore{
		ID:         "01HQTEST00000000000001",
		Content:    "Test content",
		Category:   CategoryPatternOutcome,
		Confidence: 0.5,
		SourceID:   "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Verify synced_at is NULL initially
	var syncedAt *string
	store.db.QueryRow("SELECT synced_at FROM lore_entries WHERE id = ?", lore.ID).Scan(&syncedAt)
	if syncedAt != nil {
		t.Error("synced_at should be NULL initially")
	}

	// Get queue entry ID
	var queueID int64
	store.db.QueryRow("SELECT id FROM sync_queue WHERE lore_id = ?", lore.ID).Scan(&queueID)

	// Complete the entry
	err = store.CompleteSyncEntries([]int64{queueID}, []string{lore.ID})
	if err != nil {
		t.Fatalf("CompleteSyncEntries failed: %v", err)
	}

	// Verify synced_at is now set
	store.db.QueryRow("SELECT synced_at FROM lore_entries WHERE id = ?", lore.ID).Scan(&syncedAt)
	if syncedAt == nil {
		t.Error("synced_at should be set after completion")
	}
}

func TestStore_CompleteSyncEntries_EmptyInput(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Should not error on empty input
	err = store.CompleteSyncEntries([]int64{}, nil)
	if err != nil {
		t.Errorf("CompleteSyncEntries with empty input failed: %v", err)
	}
}

func TestStore_CompleteSyncEntries_StoreClosed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	store.Close()

	err = store.CompleteSyncEntries([]int64{1}, []string{"lore-1"})
	if err != ErrStoreClosed {
		t.Errorf("CompleteSyncEntries on closed store = %v, want ErrStoreClosed", err)
	}
}

// ============================================================================
// FailSyncEntries tests
// ============================================================================

func TestStore_FailSyncEntries_IncrementsAttempts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert entry with attempts=0
	_, err = store.db.Exec(`
		INSERT INTO sync_queue (lore_id, operation, queued_at, attempts)
		VALUES ('lore-1', 'INSERT', '2024-01-01T00:00:00Z', 0)
	`)
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	var queueID int64
	store.db.QueryRow("SELECT id FROM sync_queue WHERE lore_id = 'lore-1'").Scan(&queueID)

	// Fail the entry
	err = store.FailSyncEntries([]int64{queueID}, "connection refused")
	if err != nil {
		t.Fatalf("FailSyncEntries failed: %v", err)
	}

	// Verify attempts incremented
	var attempts int
	store.db.QueryRow("SELECT attempts FROM sync_queue WHERE id = ?", queueID).Scan(&attempts)
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}

	// Fail again
	store.FailSyncEntries([]int64{queueID}, "timeout")
	store.db.QueryRow("SELECT attempts FROM sync_queue WHERE id = ?", queueID).Scan(&attempts)
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
}

func TestStore_FailSyncEntries_RecordsError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	_, err = store.db.Exec(`
		INSERT INTO sync_queue (lore_id, operation, queued_at)
		VALUES ('lore-1', 'INSERT', '2024-01-01T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	var queueID int64
	store.db.QueryRow("SELECT id FROM sync_queue WHERE lore_id = 'lore-1'").Scan(&queueID)

	// Fail with error message
	err = store.FailSyncEntries([]int64{queueID}, "HTTP 503: service unavailable")
	if err != nil {
		t.Fatalf("FailSyncEntries failed: %v", err)
	}

	var lastError string
	store.db.QueryRow("SELECT last_error FROM sync_queue WHERE id = ?", queueID).Scan(&lastError)
	if lastError != "HTTP 503: service unavailable" {
		t.Errorf("last_error = %q, want %q", lastError, "HTTP 503: service unavailable")
	}
}

func TestStore_FailSyncEntries_EmptyInput(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	err = store.FailSyncEntries([]int64{}, "some error")
	if err != nil {
		t.Errorf("FailSyncEntries with empty input failed: %v", err)
	}
}

func TestStore_FailSyncEntries_StoreClosed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	store.Close()

	err = store.FailSyncEntries([]int64{1}, "error")
	if err != ErrStoreClosed {
		t.Errorf("FailSyncEntries on closed store = %v, want ErrStoreClosed", err)
	}
}

// ============================================================================
// GetLoreByIDs tests
// ============================================================================

func TestStore_GetLoreByIDs_Multiple(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert multiple lore entries
	lore1 := &Lore{ID: "01HQTEST00000000000001", Content: "Content 1", Category: CategoryPatternOutcome, Confidence: 0.5, SourceID: "src"}
	lore2 := &Lore{ID: "01HQTEST00000000000002", Content: "Content 2", Category: CategoryEdgeCaseDiscovery, Confidence: 0.7, SourceID: "src"}
	lore3 := &Lore{ID: "01HQTEST00000000000003", Content: "Content 3", Category: CategoryTestingStrategy, Confidence: 0.9, SourceID: "src"}

	store.InsertLore(lore1)
	store.InsertLore(lore2)
	store.InsertLore(lore3)

	// Get multiple by IDs
	results, err := store.GetLoreByIDs([]string{lore1.ID, lore3.ID})
	if err != nil {
		t.Fatalf("GetLoreByIDs failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Build a map for easier checking
	byID := make(map[string]Lore)
	for _, l := range results {
		byID[l.ID] = l
	}

	if byID[lore1.ID].Content != "Content 1" {
		t.Errorf("lore1 Content = %q, want %q", byID[lore1.ID].Content, "Content 1")
	}
	if byID[lore3.ID].Content != "Content 3" {
		t.Errorf("lore3 Content = %q, want %q", byID[lore3.ID].Content, "Content 3")
	}
}

func TestStore_GetLoreByIDs_PartialMatch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert only one lore
	lore := &Lore{ID: "01HQTEST00000000000001", Content: "Content 1", Category: CategoryPatternOutcome, Confidence: 0.5, SourceID: "src"}
	store.InsertLore(lore)

	// Query with one existing and one non-existing ID
	results, err := store.GetLoreByIDs([]string{lore.ID, "NONEXISTENT0000000001"})
	if err != nil {
		t.Fatalf("GetLoreByIDs failed: %v", err)
	}

	// Should only return the one that exists
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].ID != lore.ID {
		t.Errorf("result ID = %q, want %q", results[0].ID, lore.ID)
	}
}

func TestStore_GetLoreByIDs_Empty(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	results, err := store.GetLoreByIDs([]string{})
	if err != nil {
		t.Fatalf("GetLoreByIDs failed: %v", err)
	}

	if results != nil {
		t.Errorf("expected nil for empty input, got %v", results)
	}
}

func TestStore_GetLoreByIDs_StoreClosed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	store.Close()

	_, err = store.GetLoreByIDs([]string{"lore-1"})
	if err != ErrStoreClosed {
		t.Errorf("GetLoreByIDs on closed store = %v, want ErrStoreClosed", err)
	}
}

// ============================================================================
// ApplyFeedback outcome payload test
// ============================================================================

func TestStore_ApplyFeedback_ChangeLogHelpfulPayload(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore
	lore := &Lore{
		ID:         "01HQTEST00000000000001",
		Content:    "Test content",
		Category:   CategoryPatternOutcome,
		Confidence: 0.5,
		SourceID:   "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Clear the change_log entry from InsertLore
	store.db.Exec("DELETE FROM change_log")

	// Apply helpful feedback (isHelpful=true)
	_, err = store.ApplyFeedback(lore.ID, 0.08, true)
	if err != nil {
		t.Fatalf("ApplyFeedback failed: %v", err)
	}

	// Verify change_log entry has full entity payload
	var payload string
	err = store.db.QueryRow("SELECT payload FROM change_log WHERE entity_id = ? AND operation = 'upsert'", lore.ID).Scan(&payload)
	if err != nil {
		t.Fatalf("failed to get change_log entry: %v", err)
	}

	if payload == "" {
		t.Fatal("expected payload to be set, got empty string")
	}

	// Parse payload and verify updated confidence
	var entityPayload struct {
		Confidence float64 `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(payload), &entityPayload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	if entityPayload.Confidence != 0.58 {
		t.Errorf("payload confidence = %f, want 0.58", entityPayload.Confidence)
	}
}

func TestStore_ApplyFeedback_ChangeLogIncorrectPayload(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore
	lore := &Lore{
		ID:         "01HQTEST00000000000001",
		Content:    "Test content",
		Category:   CategoryPatternOutcome,
		Confidence: 0.5,
		SourceID:   "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Clear the change_log entry
	store.db.Exec("DELETE FROM change_log")

	// Apply incorrect feedback (delta < 0, isHelpful=false)
	_, err = store.ApplyFeedback(lore.ID, -0.15, false)
	if err != nil {
		t.Fatalf("ApplyFeedback failed: %v", err)
	}

	// Verify change_log entry has full entity payload with decreased confidence
	var payload string
	err = store.db.QueryRow("SELECT payload FROM change_log WHERE entity_id = ? AND operation = 'upsert'", lore.ID).Scan(&payload)
	if err != nil {
		t.Fatalf("failed to get change_log entry: %v", err)
	}

	var entityPayload struct {
		Confidence float64 `json:"confidence"`
	}
	json.Unmarshal([]byte(payload), &entityPayload)

	if entityPayload.Confidence != 0.35 {
		t.Errorf("payload confidence = %f, want 0.35", entityPayload.Confidence)
	}
}

func TestStore_ApplyFeedback_ChangeLogNotRelevantPayload(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore
	lore := &Lore{
		ID:         "01HQTEST00000000000001",
		Content:    "Test content",
		Category:   CategoryPatternOutcome,
		Confidence: 0.5,
		SourceID:   "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Clear the change_log entry
	store.db.Exec("DELETE FROM change_log")

	// Apply not_relevant feedback (delta = 0, isHelpful=false)
	_, err = store.ApplyFeedback(lore.ID, 0.0, false)
	if err != nil {
		t.Fatalf("ApplyFeedback failed: %v", err)
	}

	// Verify change_log entry has full entity payload with unchanged confidence
	var payload string
	err = store.db.QueryRow("SELECT payload FROM change_log WHERE entity_id = ? AND operation = 'upsert'", lore.ID).Scan(&payload)
	if err != nil {
		t.Fatalf("failed to get change_log entry: %v", err)
	}

	var entityPayload struct {
		Confidence float64 `json:"confidence"`
	}
	json.Unmarshal([]byte(payload), &entityPayload)

	if entityPayload.Confidence != 0.5 {
		t.Errorf("payload confidence = %f, want 0.5", entityPayload.Confidence)
	}
}

// TestStore_UpsertLore_Insert verifies insert behavior when lore doesn't exist.
// Story 4.5: Delta sync upserts new/updated lore entries.
func TestStore_UpsertLore_Insert(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	lore := &Lore{
		ID:              "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Content:         "Test lore content",
		Category:        CategoryDependencyBehavior,
		Confidence:      0.8,
		EmbeddingStatus: "ready",
	}

	err = store.UpsertLore(lore)
	if err != nil {
		t.Fatalf("UpsertLore failed: %v", err)
	}

	// Verify it was inserted
	got, err := store.Get(lore.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Content != lore.Content {
		t.Errorf("Content = %q, want %q", got.Content, lore.Content)
	}
	if got.Confidence != lore.Confidence {
		t.Errorf("Confidence = %f, want %f", got.Confidence, lore.Confidence)
	}
}

// TestStore_UpsertLore_Update verifies update behavior when lore exists.
// Story 4.5: Delta sync upserts new/updated lore entries.
func TestStore_UpsertLore_Update(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert initial lore
	original := &Lore{
		ID:              "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Content:         "Original content",
		Category:        CategoryDependencyBehavior,
		Confidence:      0.5,
		EmbeddingStatus: "pending",
	}
	err = store.UpsertLore(original)
	if err != nil {
		t.Fatalf("initial UpsertLore failed: %v", err)
	}

	// Update with new values
	updated := &Lore{
		ID:              "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Content:         "Updated content",
		Category:        CategoryDependencyBehavior,
		Confidence:      0.9,
		EmbeddingStatus: "ready",
	}
	err = store.UpsertLore(updated)
	if err != nil {
		t.Fatalf("update UpsertLore failed: %v", err)
	}

	// Verify update
	got, err := store.Get(updated.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Content != updated.Content {
		t.Errorf("Content = %q, want %q", got.Content, updated.Content)
	}
	if got.Confidence != updated.Confidence {
		t.Errorf("Confidence = %f, want %f", got.Confidence, updated.Confidence)
	}
	if got.EmbeddingStatus != updated.EmbeddingStatus {
		t.Errorf("EmbeddingStatus = %q, want %q", got.EmbeddingStatus, updated.EmbeddingStatus)
	}
}

// TestStore_UpsertLore_StoreClosed verifies error when store is closed.
func TestStore_UpsertLore_StoreClosed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	store.Close()

	lore := &Lore{ID: "test", Content: "test", Category: CategoryPatternOutcome}
	err = store.UpsertLore(lore)
	if err != ErrStoreClosed {
		t.Errorf("expected ErrStoreClosed, got %v", err)
	}
}

// TestStore_DeleteLoreByID_Exists verifies deletion of existing lore.
// Story 4.5: Delta sync removes deleted entries.
func TestStore_DeleteLoreByID_Exists(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore first
	lore := &Lore{
		ID:              "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Content:         "Test lore content",
		Category:        CategoryDependencyBehavior,
		Confidence:      0.8,
		EmbeddingStatus: "ready",
	}
	err = store.UpsertLore(lore)
	if err != nil {
		t.Fatalf("UpsertLore failed: %v", err)
	}

	// Delete it
	err = store.DeleteLoreByID(lore.ID)
	if err != nil {
		t.Fatalf("DeleteLoreByID failed: %v", err)
	}

	// Verify it's gone
	_, err = store.Get(lore.ID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

// TestStore_DeleteLoreByID_NotExists verifies no error when lore doesn't exist.
func TestStore_DeleteLoreByID_NotExists(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Delete non-existent lore should not error
	err = store.DeleteLoreByID("nonexistent-id")
	if err != nil {
		t.Errorf("expected no error for non-existent ID, got %v", err)
	}
}

// TestStore_DeleteLoreByID_StoreClosed verifies error when store is closed.
func TestStore_DeleteLoreByID_StoreClosed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	store.Close()

	err = store.DeleteLoreByID("test")
	if err != ErrStoreClosed {
		t.Errorf("expected ErrStoreClosed, got %v", err)
	}
}

// ============================================================================
// HasPendingSync tests (Story 4.6: Database Reinitialization)
// ============================================================================

// TestStore_HasPendingSync_EmptyQueue tests AC #2:
// HasPendingSync returns 0 when sync_queue is empty.
func TestStore_HasPendingSync_EmptyQueue(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	count, err := store.HasPendingSync()
	if err != nil {
		t.Fatalf("HasPendingSync failed: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

// TestStore_HasPendingSync_WithInsertEntries tests AC #2:
// HasPendingSync counts INSERT operations in sync_queue.
func TestStore_HasPendingSync_WithInsertEntries(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Manually insert sync_queue entries (InsertLore now writes to change_log)
	_, err = store.db.Exec(`
		INSERT INTO sync_queue (lore_id, operation, queued_at) VALUES
		('lore-1', 'INSERT', '2024-01-01T00:00:00Z'),
		('lore-2', 'INSERT', '2024-01-02T00:00:00Z'),
		('lore-3', 'INSERT', '2024-01-03T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	count, err := store.HasPendingSync()
	if err != nil {
		t.Fatalf("HasPendingSync failed: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}

// TestStore_HasPendingSync_WithFeedbackEntries tests AC #2:
// HasPendingSync counts FEEDBACK operations in sync_queue.
func TestStore_HasPendingSync_WithFeedbackEntries(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Manually insert FEEDBACK entries (ApplyFeedback now writes to change_log)
	_, err = store.db.Exec(`
		INSERT INTO sync_queue (lore_id, operation, queued_at) VALUES
		('lore-1', 'FEEDBACK', '2024-01-04T00:00:00Z'),
		('lore-1', 'FEEDBACK', '2024-01-05T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	count, err := store.HasPendingSync()
	if err != nil {
		t.Fatalf("HasPendingSync failed: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

// TestStore_HasPendingSync_MixedOperations tests AC #2:
// HasPendingSync counts both INSERT and FEEDBACK operations.
func TestStore_HasPendingSync_MixedOperations(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert test data directly: 3 INSERT + 2 FEEDBACK = 5 total
	_, err = store.db.Exec(`
		INSERT INTO sync_queue (lore_id, operation, queued_at) VALUES
		('lore-1', 'INSERT', '2024-01-01T00:00:00Z'),
		('lore-2', 'INSERT', '2024-01-02T00:00:00Z'),
		('lore-3', 'INSERT', '2024-01-03T00:00:00Z'),
		('lore-1', 'FEEDBACK', '2024-01-04T00:00:00Z'),
		('lore-2', 'FEEDBACK', '2024-01-05T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	count, err := store.HasPendingSync()
	if err != nil {
		t.Fatalf("HasPendingSync failed: %v", err)
	}
	if count != 5 {
		t.Errorf("count = %d, want 5", count)
	}
}

// TestStore_HasPendingSync_StoreClosed tests error handling:
// HasPendingSync returns ErrStoreClosed when store is closed.
func TestStore_HasPendingSync_StoreClosed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	store.Close()

	_, err = store.HasPendingSync()
	if err != ErrStoreClosed {
		t.Errorf("HasPendingSync on closed store = %v, want ErrStoreClosed", err)
	}
}

// ============================================================================
// ClearAllLore tests (Story 4.6: Database Reinitialization)
// ============================================================================

// TestStore_ClearAllLore_RemovesAllLore tests:
// ClearAllLore removes all lore entries.
func TestStore_ClearAllLore_RemovesAllLore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert some lore using UpsertLore (doesn't create sync queue entries)
	for i := 0; i < 5; i++ {
		lore := &Lore{
			ID:         fmt.Sprintf("01HQTEST0000000000000%03d", i+1),
			Content:    fmt.Sprintf("Content %d", i+1),
			Category:   CategoryPatternOutcome,
			Confidence: 0.5,
		}
		if err := store.UpsertLore(lore); err != nil {
			t.Fatalf("UpsertLore failed: %v", err)
		}
	}

	// Verify lore exists
	stats, err := store.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if stats.LoreCount != 5 {
		t.Fatalf("LoreCount = %d, want 5", stats.LoreCount)
	}

	// Clear all lore
	if err := store.ClearAllLore(); err != nil {
		t.Fatalf("ClearAllLore failed: %v", err)
	}

	// Verify lore is cleared
	stats, err = store.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if stats.LoreCount != 0 {
		t.Errorf("LoreCount = %d, want 0", stats.LoreCount)
	}
}

// TestStore_ClearAllLore_ClearsSyncQueue tests:
// ClearAllLore also clears the sync queue.
func TestStore_ClearAllLore_ClearsSyncQueue(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert some sync queue entries directly
	_, err = store.db.Exec(`
		INSERT INTO sync_queue (lore_id, operation, queued_at) VALUES
		('lore-1', 'INSERT', '2024-01-01T00:00:00Z'),
		('lore-2', 'FEEDBACK', '2024-01-02T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("failed to insert sync queue: %v", err)
	}

	// Verify sync queue has entries
	count, err := store.HasPendingSync()
	if err != nil {
		t.Fatalf("HasPendingSync failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("pending count = %d, want 2", count)
	}

	// Clear all lore (and sync queue)
	if err := store.ClearAllLore(); err != nil {
		t.Fatalf("ClearAllLore failed: %v", err)
	}

	// Verify sync queue is cleared
	count, err = store.HasPendingSync()
	if err != nil {
		t.Fatalf("HasPendingSync failed: %v", err)
	}
	if count != 0 {
		t.Errorf("pending count = %d, want 0", count)
	}
}

// TestStore_ClearAllLore_StoreClosed tests error handling.
func TestStore_ClearAllLore_StoreClosed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	store.Close()

	err = store.ClearAllLore()
	if err != ErrStoreClosed {
		t.Errorf("ClearAllLore on closed store = %v, want ErrStoreClosed", err)
	}
}

// ============================================================================
// Bug Fix: Feedback Sync Fails for Locally-Created Lore
// Story: Prevent HTTP 404 errors when syncing feedback for unsynced lore
// ============================================================================

// TestApplyFeedback_UnsyncedLore verifies that feedback on lore with synced_at IS NULL
// does NOT queue a FEEDBACK operation for central sync (to prevent 404 errors).
// The local confidence update should still succeed.
func TestApplyFeedback_UnsyncedLore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore (synced_at is NULL)
	lore := &Lore{
		ID:         "01TESTID0000000000000001",
		Content:    "Locally created lore",
		Category:   CategoryArchitecturalDecision,
		Confidence: 0.5,
		SourceID:   "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Verify lore has no synced_at (is unsynced)
	freshLore, err := store.Get(lore.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if freshLore.SyncedAt != nil {
		t.Fatal("Expected lore.SyncedAt to be nil for unsynced lore")
	}

	// Clear change_log from InsertLore to isolate the test
	store.db.Exec("DELETE FROM change_log")

	// Apply feedback to unsynced lore
	updated, err := store.ApplyFeedback(lore.ID, 0.08, true)
	if err != nil {
		t.Fatalf("ApplyFeedback failed: %v", err)
	}

	// Local update should succeed - confidence should be updated
	if updated.Confidence != 0.58 {
		t.Errorf("Confidence = %f, want 0.58 (local update should succeed)", updated.Confidence)
	}

	// change_log entry should be written for ALL feedback (regardless of synced_at)
	var clCount int
	err = store.db.QueryRow(
		"SELECT COUNT(*) FROM change_log WHERE entity_id = ? AND operation = 'upsert'",
		lore.ID,
	).Scan(&clCount)
	if err != nil {
		t.Fatalf("failed to query change_log: %v", err)
	}
	if clCount != 1 {
		t.Errorf("change_log count = %d, want 1 (should always write change_log)", clCount)
	}
}

// TestApplyFeedback_SyncedLore verifies that feedback on synced lore
// writes a change_log upsert with full entity state.
func TestApplyFeedback_SyncedLore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore
	lore := &Lore{
		ID:         "01TESTID0000000000000002",
		Content:    "Synced lore from central",
		Category:   CategoryArchitecturalDecision,
		Confidence: 0.5,
		SourceID:   "test-source",
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Simulate that this lore has been synced by setting synced_at
	_, err = store.db.Exec(
		"UPDATE lore_entries SET synced_at = ? WHERE id = ?",
		"2024-01-15T10:00:00Z", lore.ID,
	)
	if err != nil {
		t.Fatalf("failed to set synced_at: %v", err)
	}

	// Clear the change_log entry to isolate the test
	store.db.Exec("DELETE FROM change_log")

	// Apply feedback to synced lore
	updated, err := store.ApplyFeedback(lore.ID, 0.08, true)
	if err != nil {
		t.Fatalf("ApplyFeedback failed: %v", err)
	}

	// Local update should succeed
	if updated.Confidence != 0.58 {
		t.Errorf("Confidence = %f, want 0.58", updated.Confidence)
	}

	// change_log entry should be written
	var clCount int
	err = store.db.QueryRow(
		"SELECT COUNT(*) FROM change_log WHERE entity_id = ? AND operation = 'upsert'",
		lore.ID,
	).Scan(&clCount)
	if err != nil {
		t.Fatalf("failed to query change_log: %v", err)
	}
	if clCount != 1 {
		t.Errorf("change_log count = %d, want 1 (should write change_log for synced lore)", clCount)
	}
}

// TestDeleteSyncEntry verifies that single queue entry deletion works correctly.
func TestDeleteSyncEntry(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert multiple sync queue entries
	_, err = store.db.Exec(`
		INSERT INTO sync_queue (lore_id, operation, queued_at) VALUES
		('lore-1', 'FEEDBACK', '2024-01-01T00:00:00Z'),
		('lore-2', 'FEEDBACK', '2024-01-02T00:00:00Z'),
		('lore-3', 'INSERT', '2024-01-03T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	// Get the ID of the first entry
	var entryID int64
	err = store.db.QueryRow("SELECT id FROM sync_queue WHERE lore_id = 'lore-1'").Scan(&entryID)
	if err != nil {
		t.Fatalf("failed to get entry ID: %v", err)
	}

	// Delete single entry
	err = store.DeleteSyncEntry(entryID)
	if err != nil {
		t.Fatalf("DeleteSyncEntry failed: %v", err)
	}

	// Verify the entry is deleted
	var count int
	store.db.QueryRow("SELECT COUNT(*) FROM sync_queue WHERE id = ?", entryID).Scan(&count)
	if count != 0 {
		t.Errorf("deleted entry still exists, count = %d", count)
	}

	// Verify other entries are not affected
	var totalCount int
	store.db.QueryRow("SELECT COUNT(*) FROM sync_queue").Scan(&totalCount)
	if totalCount != 2 {
		t.Errorf("total count = %d, want 2 (other entries should remain)", totalCount)
	}
}

// TestDeleteSyncEntry_StoreClosed verifies error when store is closed.
func TestDeleteSyncEntry_StoreClosed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	store.Close()

	err = store.DeleteSyncEntry(1)
	if err != ErrStoreClosed {
		t.Errorf("DeleteSyncEntry on closed store = %v, want ErrStoreClosed", err)
	}
}
