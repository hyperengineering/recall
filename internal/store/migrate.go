package store

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// DefaultLegacyDBPath returns the default path for legacy Recall databases.
// This was the pre-multi-store default: ./data/lore.db relative to current directory.
func DefaultLegacyDBPath() string {
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, "data", "lore.db")
}

// MigrationResult contains the result of a migration operation.
type MigrationResult struct {
	// Migrated is true if migration occurred, false if no migration needed.
	Migrated bool
	// SourcePath is the path of the database that was migrated (empty if not migrated).
	SourcePath string
	// DestPath is the path of the new database (empty if not migrated).
	DestPath string
}

// MigrateExistingDatabase checks for an existing database and migrates it to the default store.
//
// Parameters:
//   - envPath: value of RECALL_DB_PATH env var (empty if not set)
//   - storeRoot: root directory for stores (typically ~/.recall/stores)
//
// Returns:
//   - result: migration result containing migrated flag and paths
//   - error: any error during migration
//
// Migration logic:
//  1. If default store already exists, skip migration
//  2. Check envPath if provided, otherwise check DefaultLegacyDBPath
//  3. If existing DB found, copy to storeRoot/default/lore.db
func MigrateExistingDatabase(envPath, storeRoot string) (result MigrationResult, err error) {
	// Check if default store already exists
	defaultDBPath := filepath.Join(storeRoot, "default", "lore.db")
	if _, err := os.Stat(defaultDBPath); err == nil {
		// Default store exists, no migration needed
		return MigrationResult{Migrated: false}, nil
	}

	// Determine source path
	sourcePath := envPath
	if sourcePath == "" {
		sourcePath = DefaultLegacyDBPath()
	}

	// Check if source exists
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		// No existing database to migrate
		return MigrationResult{Migrated: false}, nil
	}

	// Create default store directory
	defaultDir := filepath.Dir(defaultDBPath)
	if err := os.MkdirAll(defaultDir, 0755); err != nil {
		return MigrationResult{Migrated: false}, fmt.Errorf("create default store directory: %w", err)
	}

	// Copy database file
	if err := copyFile(sourcePath, defaultDBPath); err != nil {
		return MigrationResult{Migrated: false}, fmt.Errorf("copy database: %w", err)
	}

	return MigrationResult{
		Migrated:   true,
		SourcePath: sourcePath,
		DestPath:   defaultDBPath,
	}, nil
}

// copyFile copies a file from src to dst with durability guarantees.
// On failure, attempts to clean up any partial destination file.
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	dest, err := os.Create(dst)
	if err != nil {
		return err
	}

	// Track whether we succeeded to decide cleanup
	success := false
	defer func() {
		dest.Close()
		if !success {
			_ = os.Remove(dst) // Best-effort cleanup on failure
		}
	}()

	if _, err = io.Copy(dest, source); err != nil {
		return err
	}

	// Ensure data is flushed to disk for SQLite durability
	if err := dest.Sync(); err != nil {
		return err
	}

	success = true
	return nil
}
