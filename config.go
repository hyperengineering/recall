package recall

import (
	"os"
	"time"
)

// Config configures the Recall client.
type Config struct {
	// LocalPath is the path to the local SQLite database.
	LocalPath string

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
// LocalPath defaults to "./data/lore.db" for zero-configuration first run (Story 5.5).
func DefaultConfig() Config {
	hostname, _ := os.Hostname()
	return Config{
		LocalPath:    "./data/lore.db",
		SyncInterval: 5 * time.Minute,
		AutoSync:     true,
		SourceID:     hostname,
	}
}

// ConfigFromEnv reads configuration from environment variables.
//
//	RECALL_DB_PATH     → LocalPath
//	ENGRAM_URL         → EngramURL
//	ENGRAM_API_KEY     → APIKey
//	RECALL_SOURCE_ID   → SourceID
//	RECALL_DEBUG       → Debug (any non-empty value enables)
//	RECALL_DEBUG_LOG   → DebugLogPath
func ConfigFromEnv() Config {
	return Config{
		LocalPath:    os.Getenv("RECALL_DB_PATH"),
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
// LocalPath defaults to "./data/lore.db" for zero-configuration first run (Story 5.5).
func (c Config) WithDefaults() Config {
	defaults := DefaultConfig()

	if c.LocalPath == "" {
		c.LocalPath = defaults.LocalPath
	}
	if c.SyncInterval == 0 {
		c.SyncInterval = defaults.SyncInterval
	}
	if c.SourceID == "" {
		c.SourceID = defaults.SourceID
	}

	return c
}
