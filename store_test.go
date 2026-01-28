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
