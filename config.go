package recall

import (
	"os"
	"time"

	"github.com/hyperengineering/recall/internal/store"
)

// Config configures the Recall client.
type Config struct {
	// LocalPath is the path to the local SQLite database.
	// If empty and Store is set, LocalPath is derived from Store.
	// Deprecated: Use Store field instead for multi-store support.
	LocalPath string

	// Store is the store ID to operate against.
	// If empty, resolved using store resolution (explicit > ENGRAM_STORE env > "default").
	Store string

	// EngramURL is the URL of the Engram central service.
	// If empty, operates in offline-only mode.
	EngramURL string

	// APIKey authenticates with Engram.
	APIKey string

	// SourceID identifies this client instance.
	// Defaults to hostname if not set.
	SourceID string

	// SyncInterval is how often to sync with Engram.
	// Defaults to 5 minutes.
	SyncInterval time.Duration

	// AutoSync enables automatic background syncing.
	// Defaults to true.
	AutoSync bool

	// Debug enables verbose logging of all Engram API communications.
	// When enabled, requests, responses, and full error details are logged.
	Debug bool

	// DebugLogPath is the path to write debug logs.
	// Defaults to stderr if empty.
	DebugLogPath string
}

// DefaultConfig returns a Config with sensible defaults.
// Store defaults to "default", and LocalPath is derived from Store.
func DefaultConfig() Config {
	hostname, _ := os.Hostname()
	return Config{
		Store:        "default",
		LocalPath:    store.StoreDBPath("default"),
		SyncInterval: 5 * time.Minute,
		AutoSync:     true,
		SourceID:     hostname,
	}
}

// ConfigFromEnv reads configuration from environment variables.
//
//	RECALL_DB_PATH     → LocalPath (deprecated, for backward compatibility)
//	ENGRAM_STORE       → Store
//	ENGRAM_URL         → EngramURL
//	ENGRAM_API_KEY     → APIKey
//	RECALL_SOURCE_ID   → SourceID
//	RECALL_DEBUG       → Debug (any non-empty value enables)
//	RECALL_DEBUG_LOG   → DebugLogPath
func ConfigFromEnv() Config {
	return Config{
		LocalPath:    os.Getenv("RECALL_DB_PATH"),
		Store:        os.Getenv("ENGRAM_STORE"),
		EngramURL:    os.Getenv("ENGRAM_URL"),
		APIKey:       os.Getenv("ENGRAM_API_KEY"),
		SourceID:     os.Getenv("RECALL_SOURCE_ID"),
		Debug:        os.Getenv("RECALL_DEBUG") != "",
		DebugLogPath: os.Getenv("RECALL_DEBUG_LOG"),
	}
}

// Validate checks the configuration for errors.
// Returns *ValidationError for invalid fields.
func (c *Config) Validate() error {
	if c.LocalPath == "" {
		return &ValidationError{Field: "LocalPath", Message: "required: path to SQLite database"}
	}

	// Validate store ID if explicitly set
	if c.Store != "" {
		if err := store.ValidateStoreID(c.Store); err != nil {
			return &ValidationError{Field: "Store", Message: err.Error()}
		}
	}

	if c.EngramURL != "" && c.APIKey == "" {
		return &ValidationError{Field: "APIKey", Message: "required when EngramURL is set"}
	}

	if c.SyncInterval < 0 {
		return &ValidationError{Field: "SyncInterval", Message: "must be non-negative"}
	}

	return nil
}

// IsOffline returns true if the client operates in offline-only mode.
// Offline mode is determined by EngramURL being empty.
func (c *Config) IsOffline() bool {
	return c.EngramURL == ""
}

// WithDefaults fills in default values for unset fields.
// Store resolution: explicit Store field > ENGRAM_STORE env > "default"
// LocalPath is derived from resolved Store if not explicitly set.
//
// Auto-migration: When the store resolves to "default" and no default store exists yet,
// checks for an existing database (from RECALL_DB_PATH env or legacy path) and migrates
// it to the new store location, recording the source path in metadata.
func (c Config) WithDefaults() Config {
	defaults := DefaultConfig()

	// Resolve store ID (explicit > env > default)
	if c.Store == "" {
		resolved, err := store.ResolveStore("")
		if err == nil {
			c.Store = resolved
		} else {
			c.Store = "default"
		}
	}

	// Auto-migrate existing database to default store on first run
	// This is best-effort; errors are silently ignored since migration is optional
	if c.Store == "default" {
		envPath := os.Getenv("RECALL_DB_PATH")
		storeRoot := store.DefaultStoreRoot()
		_ = migrateAndSetMetadata(envPath, storeRoot)
	}

	// Set LocalPath from resolved store if not explicitly provided
	if c.LocalPath == "" {
		c.LocalPath = store.StoreDBPath(c.Store)
	}

	if c.SyncInterval == 0 {
		c.SyncInterval = defaults.SyncInterval
	}
	if c.SourceID == "" {
		c.SourceID = defaults.SourceID
	}

	return c
}

// migrateAndSetMetadata performs auto-migration of existing databases and sets migrated_from metadata.
// Returns error only for logging purposes; callers should treat migration as best-effort.
func migrateAndSetMetadata(envPath, storeRoot string) error {
	result, err := store.MigrateExistingDatabase(envPath, storeRoot)
	if err != nil {
		return err
	}

	if !result.Migrated {
		return nil
	}

	// Open the new database to set the migrated_from metadata
	newStore, err := NewStore(result.DestPath)
	if err != nil {
		return err
	}
	defer func() { _ = newStore.Close() }()

	// Record the source path in metadata
	return newStore.SetStoreMigratedFrom(result.SourcePath)
}
