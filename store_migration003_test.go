package recall

import (
	"path/filepath"
	"regexp"
	"testing"
)

// ============================================================================
// Story 9.1: Database Migration — Change Log & Sync Meta Tables
// ============================================================================

// TestMigration003_CreatesNewTables verifies that migration 003 creates
// change_log, push_idempotency, and sync_meta tables (AC #1).
func TestMigration003_CreatesNewTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	tables := []string{"change_log", "push_idempotency", "sync_meta"}
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

// TestMigration003_ChangeLogColumns verifies change_log table has all required columns
// and correct default behavior (AC #2).
func TestMigration003_ChangeLogColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert a valid row to verify column structure works
	_, err = store.db.Exec(`
		INSERT INTO change_log (table_name, entity_id, operation, payload, source_id, created_at)
		VALUES ('lore_entries', 'entity-1', 'upsert', '{}', 'source-1', '2024-01-01T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("INSERT into change_log failed: %v", err)
	}

	// Verify sequence was auto-generated
	var seq int
	err = store.db.QueryRow("SELECT sequence FROM change_log").Scan(&seq)
	if err != nil {
		t.Fatalf("failed to read sequence: %v", err)
	}
	if seq != 1 {
		t.Errorf("sequence = %d, want 1", seq)
	}

	// Verify received_at got a default value
	var receivedAt string
	err = store.db.QueryRow("SELECT received_at FROM change_log").Scan(&receivedAt)
	if err != nil {
		t.Fatalf("failed to read received_at: %v", err)
	}
	if receivedAt == "" {
		t.Error("received_at should have a default value")
	}
}

// TestMigration003_ChangeLogOperationConstraint verifies CHECK constraint on operation (AC #2).
func TestMigration003_ChangeLogOperationConstraint(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Valid operations should succeed
	for _, op := range []string{"upsert", "delete"} {
		_, err = store.db.Exec(`
			INSERT INTO change_log (table_name, entity_id, operation, source_id, created_at)
			VALUES ('lore_entries', 'e1', ?, 'src', '2024-01-01T00:00:00Z')
		`, op)
		if err != nil {
			t.Errorf("INSERT with operation %q should succeed: %v", op, err)
		}
	}

	// Invalid operation should fail CHECK constraint
	_, err = store.db.Exec(`
		INSERT INTO change_log (table_name, entity_id, operation, source_id, created_at)
		VALUES ('lore_entries', 'e2', 'invalid', 'src', '2024-01-01T00:00:00Z')
	`)
	if err == nil {
		t.Error("INSERT with invalid operation should fail CHECK constraint")
	}
}

// TestMigration003_ChangeLogIndex verifies composite index on (table_name, entity_id) (AC #2).
func TestMigration003_ChangeLogIndex(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	var name string
	err = store.db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='index' AND name='idx_change_log_table_entity'",
	).Scan(&name)
	if err != nil {
		t.Errorf("index idx_change_log_table_entity not found: %v", err)
	}
}

// TestMigration003_PushIdempotencyColumns verifies push_idempotency schema (AC #3).
func TestMigration003_PushIdempotencyColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert a valid row
	_, err = store.db.Exec(`
		INSERT INTO push_idempotency (push_id, store_id, response, expires_at)
		VALUES ('push-1', 'store-1', '{"ok":true}', '2024-12-31T23:59:59Z')
	`)
	if err != nil {
		t.Fatalf("INSERT into push_idempotency failed: %v", err)
	}

	// Verify created_at got a default value
	var createdAt string
	err = store.db.QueryRow("SELECT created_at FROM push_idempotency WHERE push_id = 'push-1'").Scan(&createdAt)
	if err != nil {
		t.Fatalf("failed to read created_at: %v", err)
	}
	if createdAt == "" {
		t.Error("created_at should have a default value")
	}

	// Verify push_id is primary key (duplicate should fail)
	_, err = store.db.Exec(`
		INSERT INTO push_idempotency (push_id, store_id, response, expires_at)
		VALUES ('push-1', 'store-2', '{}', '2025-01-01T00:00:00Z')
	`)
	if err == nil {
		t.Error("duplicate push_id should violate PRIMARY KEY constraint")
	}
}

// TestMigration003_PushIdempotencyIndex verifies index on expires_at (AC #3).
func TestMigration003_PushIdempotencyIndex(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	var name string
	err = store.db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='index' AND name='idx_push_idempotency_expires_at'",
	).Scan(&name)
	if err != nil {
		t.Errorf("index idx_push_idempotency_expires_at not found: %v", err)
	}
}

// TestMigration003_SyncMetaInitialValues verifies server-level sync_meta initialization (AC #4).
func TestMigration003_SyncMetaInitialValues(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	expected := map[string]string{
		"schema_version":      "2",
		"last_compaction_seq": "0",
		"last_compaction_at":  "",
	}

	for key, want := range expected {
		var value string
		err := store.db.QueryRow("SELECT value FROM sync_meta WHERE key = ?", key).Scan(&value)
		if err != nil {
			t.Errorf("sync_meta key %q not found: %v", key, err)
			continue
		}
		if value != want {
			t.Errorf("sync_meta[%q] = %q, want %q", key, value, want)
		}
	}
}

// TestMigration003_ClientSyncMetaKeys verifies client-specific sync_meta keys (AC #5).
func TestMigration003_ClientSyncMetaKeys(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Verify last_push_seq
	var pushSeq string
	err = store.db.QueryRow("SELECT value FROM sync_meta WHERE key = 'last_push_seq'").Scan(&pushSeq)
	if err != nil {
		t.Fatalf("last_push_seq not found: %v", err)
	}
	if pushSeq != "0" {
		t.Errorf("last_push_seq = %q, want %q", pushSeq, "0")
	}

	// Verify last_pull_seq
	var pullSeq string
	err = store.db.QueryRow("SELECT value FROM sync_meta WHERE key = 'last_pull_seq'").Scan(&pullSeq)
	if err != nil {
		t.Fatalf("last_pull_seq not found: %v", err)
	}
	if pullSeq != "0" {
		t.Errorf("last_pull_seq = %q, want %q", pullSeq, "0")
	}

	// Verify source_id exists and is a valid UUIDv4
	var sourceID string
	err = store.db.QueryRow("SELECT value FROM sync_meta WHERE key = 'source_id'").Scan(&sourceID)
	if err != nil {
		t.Fatalf("source_id not found: %v", err)
	}
	uuidPattern := `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`
	if !regexp.MustCompile(uuidPattern).MatchString(sourceID) {
		t.Errorf("source_id %q is not a valid UUIDv4", sourceID)
	}
}

// TestMigration003_SourceIdPersistsAcrossRestarts verifies source_id is not regenerated (AC #6).
func TestMigration003_SourceIdPersistsAcrossRestarts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// First open
	store1, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("first NewStore failed: %v", err)
	}

	var sourceID1 string
	err = store1.db.QueryRow("SELECT value FROM sync_meta WHERE key = 'source_id'").Scan(&sourceID1)
	if err != nil {
		t.Fatalf("source_id not found on first open: %v", err)
	}
	store1.Close()

	// Second open
	store2, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("second NewStore failed: %v", err)
	}
	defer store2.Close()

	var sourceID2 string
	err = store2.db.QueryRow("SELECT value FROM sync_meta WHERE key = 'source_id'").Scan(&sourceID2)
	if err != nil {
		t.Fatalf("source_id not found on second open: %v", err)
	}

	if sourceID1 != sourceID2 {
		t.Errorf("source_id changed across restarts: %q -> %q", sourceID1, sourceID2)
	}
}

// TestMigration003_FreshDatabaseGetsAllMigrations verifies all tables exist on fresh DB (AC #7).
func TestMigration003_FreshDatabaseGetsAllMigrations(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// All 6 application tables should exist (3 from 001 + 3 from 003)
	allTables := []string{
		"lore_entries", "metadata", "sync_queue",
		"change_log", "push_idempotency", "sync_meta",
	}
	for _, table := range allTables {
		var name string
		err := store.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
			table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found on fresh database: %v", table, err)
		}
	}
}

// TestMigration003_SchemaVersionConstantUpdated verifies schemaVersion = "2" (AC #8).
func TestMigration003_SchemaVersionConstantUpdated(t *testing.T) {
	if schemaVersion != "2" {
		t.Errorf("schemaVersion = %q, want %q", schemaVersion, "2")
	}
}

// TestMigration003_MetadataSchemaVersionUpdated verifies metadata table reflects new version (AC #8).
func TestMigration003_MetadataSchemaVersionUpdated(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	var value string
	err = store.db.QueryRow("SELECT value FROM metadata WHERE key = 'schema_version'").Scan(&value)
	if err != nil {
		t.Fatalf("schema_version not in metadata: %v", err)
	}
	if value != "2" {
		t.Errorf("metadata schema_version = %q, want %q", value, "2")
	}
}
