package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperengineering/recall/internal/store"
)

func TestMigrateExistingDatabase_NoExisting(t *testing.T) {
	// No existing database - should return false, nil
	tmpDir := t.TempDir()
	storeRoot := filepath.Join(tmpDir, "stores")

	result, err := store.MigrateExistingDatabase("", storeRoot)
	if err != nil {
		t.Fatalf("MigrateExistingDatabase: %v", err)
	}
	if result.Migrated {
		t.Error("expected migrated=false when no existing DB")
	}
}

func TestMigrateExistingDatabase_FromEnvPath(t *testing.T) {
	tmpDir := t.TempDir()
	storeRoot := filepath.Join(tmpDir, "stores")

	// Create a fake existing database
	existingDBPath := filepath.Join(tmpDir, "old", "lore.db")
	if err := os.MkdirAll(filepath.Dir(existingDBPath), 0755); err != nil {
		t.Fatalf("create old dir: %v", err)
	}
	if err := os.WriteFile(existingDBPath, []byte("fake-db-content"), 0644); err != nil {
		t.Fatalf("write old db: %v", err)
	}

	result, err := store.MigrateExistingDatabase(existingDBPath, storeRoot)
	if err != nil {
		t.Fatalf("MigrateExistingDatabase: %v", err)
	}
	if !result.Migrated {
		t.Error("expected migrated=true when existing DB found")
	}
	if result.SourcePath != existingDBPath {
		t.Errorf("SourcePath = %q, want %q", result.SourcePath, existingDBPath)
	}

	// Verify default store database was created
	defaultDBPath := filepath.Join(storeRoot, "default", "lore.db")
	if _, err := os.Stat(defaultDBPath); os.IsNotExist(err) {
		t.Errorf("default store DB not created at %s", defaultDBPath)
	}
	if result.DestPath != defaultDBPath {
		t.Errorf("DestPath = %q, want %q", result.DestPath, defaultDBPath)
	}

	// Verify content was copied
	content, err := os.ReadFile(defaultDBPath)
	if err != nil {
		t.Fatalf("read default db: %v", err)
	}
	if string(content) != "fake-db-content" {
		t.Errorf("content = %q, want %q", string(content), "fake-db-content")
	}
}

func TestMigrateExistingDatabase_DefaultStoreExists(t *testing.T) {
	tmpDir := t.TempDir()
	storeRoot := filepath.Join(tmpDir, "stores")

	// Create existing database
	existingDBPath := filepath.Join(tmpDir, "old", "lore.db")
	if err := os.MkdirAll(filepath.Dir(existingDBPath), 0755); err != nil {
		t.Fatalf("create old dir: %v", err)
	}
	if err := os.WriteFile(existingDBPath, []byte("old-content"), 0644); err != nil {
		t.Fatalf("write old db: %v", err)
	}

	// Also create default store (simulating migration already happened)
	defaultDBPath := filepath.Join(storeRoot, "default", "lore.db")
	if err := os.MkdirAll(filepath.Dir(defaultDBPath), 0755); err != nil {
		t.Fatalf("create default dir: %v", err)
	}
	if err := os.WriteFile(defaultDBPath, []byte("existing-default-content"), 0644); err != nil {
		t.Fatalf("write default db: %v", err)
	}

	// Migration should skip (not overwrite)
	result, err := store.MigrateExistingDatabase(existingDBPath, storeRoot)
	if err != nil {
		t.Fatalf("MigrateExistingDatabase: %v", err)
	}
	if result.Migrated {
		t.Error("expected migrated=false when default store already exists")
	}

	// Verify existing default content is preserved
	content, err := os.ReadFile(defaultDBPath)
	if err != nil {
		t.Fatalf("read default db: %v", err)
	}
	if string(content) != "existing-default-content" {
		t.Errorf("content = %q, want %q", string(content), "existing-default-content")
	}
}

func TestMigrateExistingDatabase_NonExistentSourcePath(t *testing.T) {
	tmpDir := t.TempDir()
	storeRoot := filepath.Join(tmpDir, "stores")

	// Point to non-existent file
	result, err := store.MigrateExistingDatabase("/nonexistent/path/lore.db", storeRoot)
	if err != nil {
		t.Fatalf("MigrateExistingDatabase: %v", err)
	}
	if result.Migrated {
		t.Error("expected migrated=false when source path doesn't exist")
	}
}

func TestMigrateExistingDatabase_ReturnsSourcePath(t *testing.T) {
	tmpDir := t.TempDir()
	storeRoot := filepath.Join(tmpDir, "stores")

	// Create a fake existing database
	existingDBPath := filepath.Join(tmpDir, "old", "lore.db")
	if err := os.MkdirAll(filepath.Dir(existingDBPath), 0755); err != nil {
		t.Fatalf("create old dir: %v", err)
	}
	if err := os.WriteFile(existingDBPath, []byte("fake-db"), 0644); err != nil {
		t.Fatalf("write old db: %v", err)
	}

	result, err := store.MigrateExistingDatabase(existingDBPath, storeRoot)
	if err != nil {
		t.Fatalf("MigrateExistingDatabase: %v", err)
	}
	if !result.Migrated {
		t.Error("expected migrated=true")
	}
	if result.SourcePath != existingDBPath {
		t.Errorf("SourcePath = %q, want %q", result.SourcePath, existingDBPath)
	}
}

func TestDefaultLegacyDBPath(t *testing.T) {
	path := store.DefaultLegacyDBPath()

	// Should end with data/lore.db (the old default)
	if !filepath.IsAbs(path) {
		t.Errorf("DefaultLegacyDBPath() = %q, should be absolute", path)
	}
}
