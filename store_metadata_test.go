package recall_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperengineering/recall"
)

func TestStore_GetSetDescription(t *testing.T) {
	// Create temp store
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// Initially empty
	desc, err := store.GetStoreDescription()
	if err != nil {
		t.Fatalf("GetStoreDescription: %v", err)
	}
	if desc != "" {
		t.Errorf("initial description = %q, want empty", desc)
	}

	// Set description
	if err := store.SetStoreDescription("Test store for unit tests"); err != nil {
		t.Fatalf("SetStoreDescription: %v", err)
	}

	// Get description
	desc, err = store.GetStoreDescription()
	if err != nil {
		t.Fatalf("GetStoreDescription after set: %v", err)
	}
	if desc != "Test store for unit tests" {
		t.Errorf("description = %q, want %q", desc, "Test store for unit tests")
	}
}

func TestStore_GetCreatedAt(t *testing.T) {
	before := time.Now().UTC()

	// Create temp store
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	after := time.Now().UTC()

	// Get created_at
	createdAt, err := store.GetStoreCreatedAt()
	if err != nil {
		t.Fatalf("GetStoreCreatedAt: %v", err)
	}

	// Should be between before and after
	if createdAt.Before(before.Add(-time.Second)) {
		t.Errorf("created_at %v is before test start %v", createdAt, before)
	}
	if createdAt.After(after.Add(time.Second)) {
		t.Errorf("created_at %v is after test end %v", createdAt, after)
	}
}

func TestStore_GetMigratedFrom_NewStore(t *testing.T) {
	// Create temp store
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// New stores should have empty migrated_from
	migratedFrom, err := store.GetStoreMigratedFrom()
	if err != nil {
		t.Fatalf("GetStoreMigratedFrom: %v", err)
	}
	if migratedFrom != "" {
		t.Errorf("migrated_from = %q, want empty for new store", migratedFrom)
	}
}

func TestStore_SetMigratedFrom(t *testing.T) {
	// Create temp store
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// Set migrated_from
	origPath := "/old/path/to/lore.db"
	if err := store.SetStoreMigratedFrom(origPath); err != nil {
		t.Fatalf("SetStoreMigratedFrom: %v", err)
	}

	// Get migrated_from
	got, err := store.GetStoreMigratedFrom()
	if err != nil {
		t.Fatalf("GetStoreMigratedFrom: %v", err)
	}
	if got != origPath {
		t.Errorf("migrated_from = %q, want %q", got, origPath)
	}
}

func TestStore_MetadataPersistedAcrossReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// Create store and set metadata
	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.SetStoreDescription("Persisted description"); err != nil {
		t.Fatalf("SetStoreDescription: %v", err)
	}
	createdAt1, _ := store.GetStoreCreatedAt()
	store.Close()

	// Reopen store
	store2, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore (reopen): %v", err)
	}
	defer store2.Close()

	// Verify metadata persisted
	desc, _ := store2.GetStoreDescription()
	if desc != "Persisted description" {
		t.Errorf("description after reopen = %q, want %q", desc, "Persisted description")
	}

	createdAt2, _ := store2.GetStoreCreatedAt()
	if !createdAt1.Equal(createdAt2) {
		t.Errorf("created_at changed after reopen: %v -> %v", createdAt1, createdAt2)
	}
}
