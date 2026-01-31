package recall_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hyperengineering/recall"
)

func TestConfig_Store_FromEnv(t *testing.T) {
	// Save and set env
	origStore := os.Getenv("ENGRAM_STORE")
	os.Setenv("ENGRAM_STORE", "test-project")
	t.Cleanup(func() {
		if origStore != "" {
			os.Setenv("ENGRAM_STORE", origStore)
		} else {
			os.Unsetenv("ENGRAM_STORE")
		}
	})

	cfg := recall.ConfigFromEnv()
	if cfg.Store != "test-project" {
		t.Errorf("Store = %q, want %q", cfg.Store, "test-project")
	}
}

func TestConfig_Validate_InvalidStoreID(t *testing.T) {
	cfg := recall.Config{
		LocalPath: "/tmp/test.db",
		Store:     "Invalid-Store", // uppercase
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() expected error for invalid store ID, got nil")
	}
}

func TestConfig_Validate_ValidStoreID(t *testing.T) {
	cfg := recall.Config{
		LocalPath: "/tmp/test.db",
		Store:     "valid-store",
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}
}

func TestConfig_Validate_ReservedStoreIDAllowed(t *testing.T) {
	// Reserved IDs are valid for targeting (config just sets the store to use)
	cfg := recall.Config{
		LocalPath: "/tmp/test.db",
		Store:     "default",
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Validate() unexpected error for 'default': %v", err)
	}
}

func TestConfig_WithDefaults_ResolvesStore(t *testing.T) {
	// Clear env
	origStore := os.Getenv("ENGRAM_STORE")
	os.Unsetenv("ENGRAM_STORE")
	t.Cleanup(func() {
		if origStore != "" {
			os.Setenv("ENGRAM_STORE", origStore)
		}
	})

	cfg := recall.Config{}.WithDefaults()

	// Should resolve to "default"
	if cfg.Store != "default" {
		t.Errorf("Store = %q, want %q", cfg.Store, "default")
	}
}

func TestConfig_WithDefaults_ExplicitStorePreserved(t *testing.T) {
	cfg := recall.Config{
		Store: "my-project",
	}.WithDefaults()

	if cfg.Store != "my-project" {
		t.Errorf("Store = %q, want %q", cfg.Store, "my-project")
	}
}

func TestConfig_WithDefaults_SetsLocalPathFromStore(t *testing.T) {
	// Clear env and set up clean state
	origStore := os.Getenv("ENGRAM_STORE")
	os.Unsetenv("ENGRAM_STORE")
	t.Cleanup(func() {
		if origStore != "" {
			os.Setenv("ENGRAM_STORE", origStore)
		}
	})

	cfg := recall.Config{
		Store: "my-project",
	}.WithDefaults()

	// LocalPath should be set to store-based path
	if !strings.Contains(cfg.LocalPath, ".recall") {
		t.Errorf("LocalPath = %q, should contain .recall", cfg.LocalPath)
	}
	if !strings.Contains(cfg.LocalPath, "my-project") {
		t.Errorf("LocalPath = %q, should contain store ID", cfg.LocalPath)
	}
	if !strings.HasSuffix(cfg.LocalPath, "lore.db") {
		t.Errorf("LocalPath = %q, should end with lore.db", cfg.LocalPath)
	}
}

func TestConfig_WithDefaults_LocalPathOverridesStore(t *testing.T) {
	// If LocalPath is explicitly set, it takes precedence (backward compatibility)
	cfg := recall.Config{
		LocalPath: "/custom/path/my.db",
		Store:     "my-project",
	}.WithDefaults()

	if cfg.LocalPath != "/custom/path/my.db" {
		t.Errorf("LocalPath = %q, want %q", cfg.LocalPath, "/custom/path/my.db")
	}
}

func TestConfig_WithDefaults_EnvStoreResolution(t *testing.T) {
	// Set env
	origStore := os.Getenv("ENGRAM_STORE")
	os.Setenv("ENGRAM_STORE", "env-project")
	t.Cleanup(func() {
		if origStore != "" {
			os.Setenv("ENGRAM_STORE", origStore)
		} else {
			os.Unsetenv("ENGRAM_STORE")
		}
	})

	cfg := recall.Config{}.WithDefaults()

	if cfg.Store != "env-project" {
		t.Errorf("Store = %q, want %q", cfg.Store, "env-project")
	}
}

func TestConfig_WithDefaults_PathStyleStore(t *testing.T) {
	cfg := recall.Config{
		Store: "org/team/project",
	}.WithDefaults()

	// Path should use encoded store ID (/ -> __)
	if !strings.Contains(cfg.LocalPath, "org__team__project") {
		t.Errorf("LocalPath = %q, should contain encoded store ID org__team__project", cfg.LocalPath)
	}
}

func TestNew_WithStore(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := recall.Config{
		Store:     "test-store",
		LocalPath: filepath.Join(tmpDir, "test.db"),
	}

	client, err := recall.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer client.Close()
}

// TestConfig_WithDefaults_AutoMigration_SetsMigratedFromMetadata tests AC #6:
// When an existing database exists and default store does not, migration should:
// 1. Copy the database to stores/default/lore.db
// 2. Set the migrated_from metadata to the source path
func TestConfig_WithDefaults_AutoMigration_SetsMigratedFromMetadata(t *testing.T) {
	// Create temp directory for testing
	tmpDir := t.TempDir()

	// Create a legacy database at a custom path (simulating RECALL_DB_PATH)
	legacyDBPath := filepath.Join(tmpDir, "legacy", "lore.db")
	if err := os.MkdirAll(filepath.Dir(legacyDBPath), 0755); err != nil {
		t.Fatalf("create legacy dir: %v", err)
	}

	// Create actual SQLite database (not just a file, since we need to open it)
	legacyStore, err := recall.NewStore(legacyDBPath)
	if err != nil {
		t.Fatalf("create legacy store: %v", err)
	}
	legacyStore.Close()

	// Set up environment to use this database and custom store root
	origDBPath := os.Getenv("RECALL_DB_PATH")
	origStore := os.Getenv("ENGRAM_STORE")
	origHome := os.Getenv("HOME")

	// Point HOME to tmpDir so DefaultStoreRoot uses our temp location
	os.Setenv("HOME", tmpDir)
	os.Setenv("RECALL_DB_PATH", legacyDBPath)
	os.Unsetenv("ENGRAM_STORE")

	t.Cleanup(func() {
		if origDBPath != "" {
			os.Setenv("RECALL_DB_PATH", origDBPath)
		} else {
			os.Unsetenv("RECALL_DB_PATH")
		}
		if origStore != "" {
			os.Setenv("ENGRAM_STORE", origStore)
		} else {
			os.Unsetenv("ENGRAM_STORE")
		}
		if origHome != "" {
			os.Setenv("HOME", origHome)
		}
	})

	// Call WithDefaults which should trigger migration
	cfg := recall.Config{}.WithDefaults()

	// Verify the new database exists
	expectedPath := filepath.Join(tmpDir, ".recall", "stores", "default", "lore.db")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatalf("migrated database not created at %s", expectedPath)
	}

	// Open the new database and verify migrated_from metadata
	newStore, err := recall.NewStore(cfg.LocalPath)
	if err != nil {
		t.Fatalf("open new store: %v", err)
	}
	defer newStore.Close()

	migratedFrom, err := newStore.GetStoreMigratedFrom()
	if err != nil {
		t.Fatalf("GetStoreMigratedFrom: %v", err)
	}
	if migratedFrom != legacyDBPath {
		t.Errorf("migrated_from = %q, want %q", migratedFrom, legacyDBPath)
	}
}

// TestConfig_WithDefaults_AutoMigration_SkipsIfDefaultExists tests that migration
// does not overwrite existing default store.
func TestConfig_WithDefaults_AutoMigration_SkipsIfDefaultExists(t *testing.T) {
	tmpDir := t.TempDir()

	// Create legacy database
	legacyDBPath := filepath.Join(tmpDir, "legacy", "lore.db")
	if err := os.MkdirAll(filepath.Dir(legacyDBPath), 0755); err != nil {
		t.Fatalf("create legacy dir: %v", err)
	}
	legacyStore, err := recall.NewStore(legacyDBPath)
	if err != nil {
		t.Fatalf("create legacy store: %v", err)
	}
	// Add a lore entry to distinguish it
	_, err = legacyStore.Record(recall.Lore{Content: "legacy content", Category: recall.CategoryArchitecturalDecision})
	if err != nil {
		t.Fatalf("record legacy: %v", err)
	}
	legacyStore.Close()

	// Create existing default store with different content
	existingDefaultPath := filepath.Join(tmpDir, ".recall", "stores", "default", "lore.db")
	if err := os.MkdirAll(filepath.Dir(existingDefaultPath), 0755); err != nil {
		t.Fatalf("create default dir: %v", err)
	}
	existingStore, err := recall.NewStore(existingDefaultPath)
	if err != nil {
		t.Fatalf("create existing store: %v", err)
	}
	_, err = existingStore.Record(recall.Lore{Content: "existing content", Category: recall.CategoryPatternOutcome})
	if err != nil {
		t.Fatalf("record existing: %v", err)
	}
	existingStore.Close()

	// Set up env
	origDBPath := os.Getenv("RECALL_DB_PATH")
	origHome := os.Getenv("HOME")
	origStore := os.Getenv("ENGRAM_STORE")

	os.Setenv("HOME", tmpDir)
	os.Setenv("RECALL_DB_PATH", legacyDBPath)
	os.Unsetenv("ENGRAM_STORE")

	t.Cleanup(func() {
		if origDBPath != "" {
			os.Setenv("RECALL_DB_PATH", origDBPath)
		} else {
			os.Unsetenv("RECALL_DB_PATH")
		}
		if origHome != "" {
			os.Setenv("HOME", origHome)
		}
		if origStore != "" {
			os.Setenv("ENGRAM_STORE", origStore)
		} else {
			os.Unsetenv("ENGRAM_STORE")
		}
	})

	// Call WithDefaults - migration should be skipped
	cfg := recall.Config{}.WithDefaults()

	// Open the default database and verify it was NOT overwritten
	newStore, err := recall.NewStore(cfg.LocalPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer newStore.Close()

	// Should not have migrated_from (migration didn't happen)
	migratedFrom, err := newStore.GetStoreMigratedFrom()
	if err != nil {
		t.Fatalf("GetStoreMigratedFrom: %v", err)
	}
	if migratedFrom != "" {
		t.Errorf("migrated_from = %q, want empty (migration should be skipped)", migratedFrom)
	}
}
