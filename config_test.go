package recall_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/hyperengineering/recall"
)

func TestConfig_Validate_ValidLocalOnly(t *testing.T) {
	cfg := recall.Config{LocalPath: "/tmp/test.db"}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() returned error for valid local-only config: %v", err)
	}
}

func TestConfig_Validate_ValidWithEngram(t *testing.T) {
	cfg := recall.Config{
		LocalPath: "/tmp/test.db",
		EngramURL: "http://engram:8080",
		APIKey:    "test-key",
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() returned error for valid config with Engram: %v", err)
	}
}

func TestConfig_Validate_MissingLocalPath(t *testing.T) {
	cfg := recall.Config{}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() returned nil, want ValidationError for missing LocalPath")
	}

	var ve *recall.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Validate() returned %T, want *ValidationError", err)
	}
	if ve.Field != "LocalPath" {
		t.Errorf("ValidationError.Field = %q, want %q", ve.Field, "LocalPath")
	}
}

func TestConfig_Validate_EngramURLWithoutAPIKey(t *testing.T) {
	cfg := recall.Config{
		LocalPath: "/tmp/test.db",
		EngramURL: "http://engram:8080",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() returned nil, want ValidationError for missing APIKey")
	}

	var ve *recall.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Validate() returned %T, want *ValidationError", err)
	}
	if ve.Field != "APIKey" {
		t.Errorf("ValidationError.Field = %q, want %q", ve.Field, "APIKey")
	}
}

func TestConfigFromEnv_ReadsVars(t *testing.T) {
	// Save and restore env
	envVars := map[string]string{
		"RECALL_DB_PATH":   "/tmp/env-test.db",
		"ENGRAM_URL":       "http://engram:9090",
		"ENGRAM_API_KEY":   "env-key",
		"RECALL_SOURCE_ID": "env-source",
	}

	originals := make(map[string]string)
	for k, v := range envVars {
		originals[k] = os.Getenv(k)
		_ = os.Setenv(k, v)
	}
	defer func() {
		for k, v := range originals {
			if v == "" {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, v)
			}
		}
	}()

	cfg := recall.ConfigFromEnv()

	if cfg.LocalPath != "/tmp/env-test.db" {
		t.Errorf("LocalPath = %q, want %q", cfg.LocalPath, "/tmp/env-test.db")
	}
	if cfg.EngramURL != "http://engram:9090" {
		t.Errorf("EngramURL = %q, want %q", cfg.EngramURL, "http://engram:9090")
	}
	if cfg.APIKey != "env-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "env-key")
	}
	if cfg.SourceID != "env-source" {
		t.Errorf("SourceID = %q, want %q", cfg.SourceID, "env-source")
	}
}

func TestConfigFromEnv_UnsetVarsDefaultToEmpty(t *testing.T) {
	// Ensure env vars are unset
	vars := []string{"RECALL_DB_PATH", "ENGRAM_URL", "ENGRAM_API_KEY", "RECALL_SOURCE_ID"}
	originals := make(map[string]string)
	for _, k := range vars {
		originals[k] = os.Getenv(k)
		_ = os.Unsetenv(k)
	}
	defer func() {
		for k, v := range originals {
			if v == "" {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, v)
			}
		}
	}()

	cfg := recall.ConfigFromEnv()

	if cfg.LocalPath != "" {
		t.Errorf("LocalPath = %q, want empty", cfg.LocalPath)
	}
	if cfg.EngramURL != "" {
		t.Errorf("EngramURL = %q, want empty", cfg.EngramURL)
	}
	if cfg.APIKey != "" {
		t.Errorf("APIKey = %q, want empty", cfg.APIKey)
	}
	if cfg.SourceID != "" {
		t.Errorf("SourceID = %q, want empty", cfg.SourceID)
	}
}

// TestDefaultConfig_IncludesLocalPath verifies DefaultConfig returns store-based default path.
// Story 7.1: Multi-store support. Default store is "default".
func TestDefaultConfig_IncludesLocalPath(t *testing.T) {
	cfg := recall.DefaultConfig()
	// Default path should be store-based: ~/.recall/stores/default/lore.db
	if !strings.Contains(cfg.LocalPath, ".recall") || !strings.Contains(cfg.LocalPath, "stores") {
		t.Errorf("DefaultConfig().LocalPath = %q, should contain .recall/stores", cfg.LocalPath)
	}
	if !strings.HasSuffix(cfg.LocalPath, "lore.db") {
		t.Errorf("DefaultConfig().LocalPath = %q, should end with lore.db", cfg.LocalPath)
	}
	if cfg.Store != "default" {
		t.Errorf("DefaultConfig().Store = %q, want %q", cfg.Store, "default")
	}
}

// TestWithDefaults_AppliesLocalPath verifies WithDefaults fills LocalPath based on resolved store.
// Story 7.1: Multi-store support. WithDefaults resolves store and derives LocalPath.
func TestWithDefaults_AppliesLocalPath(t *testing.T) {
	// Clear ENGRAM_STORE to ensure default resolution
	origStore := os.Getenv("ENGRAM_STORE")
	os.Unsetenv("ENGRAM_STORE")
	defer func() {
		if origStore != "" {
			os.Setenv("ENGRAM_STORE", origStore)
		}
	}()

	cfg := recall.Config{}.WithDefaults()
	// LocalPath should be derived from resolved store (default)
	if !strings.Contains(cfg.LocalPath, ".recall") || !strings.Contains(cfg.LocalPath, "stores") {
		t.Errorf("WithDefaults().LocalPath = %q, should contain .recall/stores", cfg.LocalPath)
	}
	if cfg.Store != "default" {
		t.Errorf("WithDefaults().Store = %q, want %q", cfg.Store, "default")
	}
}

// TestWithDefaults_PreservesExplicitLocalPath verifies explicit paths are not overwritten.
func TestWithDefaults_PreservesExplicitLocalPath(t *testing.T) {
	explicit := "/custom/path/to/lore.db"
	cfg := recall.Config{LocalPath: explicit}.WithDefaults()
	if cfg.LocalPath != explicit {
		t.Errorf("WithDefaults().LocalPath = %q, want %q (should preserve explicit)", cfg.LocalPath, explicit)
	}
}

// TestConfigFromEnv_WithDefaults_AppliesLocalPath verifies the common usage pattern.
// Story 7.1: ConfigFromEnv().WithDefaults() should use store-based path when env is unset.
func TestConfigFromEnv_WithDefaults_AppliesLocalPath(t *testing.T) {
	// Save and clear env
	origPath := os.Getenv("RECALL_DB_PATH")
	origStore := os.Getenv("ENGRAM_STORE")
	os.Unsetenv("RECALL_DB_PATH")
	os.Unsetenv("ENGRAM_STORE")
	defer func() {
		if origPath != "" {
			os.Setenv("RECALL_DB_PATH", origPath)
		} else {
			os.Unsetenv("RECALL_DB_PATH")
		}
		if origStore != "" {
			os.Setenv("ENGRAM_STORE", origStore)
		} else {
			os.Unsetenv("ENGRAM_STORE")
		}
	}()

	cfg := recall.ConfigFromEnv().WithDefaults()
	// LocalPath should be store-based (default store)
	if !strings.Contains(cfg.LocalPath, ".recall") || !strings.Contains(cfg.LocalPath, "stores") {
		t.Errorf("ConfigFromEnv().WithDefaults().LocalPath = %q, should contain .recall/stores", cfg.LocalPath)
	}
	if cfg.Store != "default" {
		t.Errorf("ConfigFromEnv().WithDefaults().Store = %q, want %q", cfg.Store, "default")
	}
}

func TestConfig_IsOffline_EmptyEngramURL(t *testing.T) {
	// Offline mode is now derived from EngramURL being empty.
	// Resolves DD-6 ambiguity from Epic 1.
	cfg := recall.Config{
		LocalPath: "/tmp/test.db",
		EngramURL: "", // empty = offline
	}
	if !cfg.IsOffline() {
		t.Error("IsOffline() = false, want true when EngramURL is empty")
	}
}

func TestConfig_IsOffline_WithEngramURL(t *testing.T) {
	// Offline mode is false when EngramURL is set.
	cfg := recall.Config{
		LocalPath: "/tmp/test.db",
		EngramURL: "http://engram:8080",
		APIKey:    "test-key",
	}
	if cfg.IsOffline() {
		t.Error("IsOffline() = true, want false when EngramURL is set")
	}
}

func TestConfig_Validate_NegativeSyncInterval(t *testing.T) {
	cfg := recall.Config{
		LocalPath:    "/tmp/test.db",
		SyncInterval: -1,
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want ValidationError for negative SyncInterval")
	}

	var ve *recall.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Validate() = %T, want *ValidationError", err)
	}
	if ve.Field != "SyncInterval" {
		t.Errorf("ValidationError.Field = %q, want %q", ve.Field, "SyncInterval")
	}
}
