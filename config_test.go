package recall_test

import (
	"errors"
	"os"
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

// TestDefaultConfig_IncludesLocalPath verifies DefaultConfig returns sensible default path.
// Story 5.5: Zero-configuration first run.
func TestDefaultConfig_IncludesLocalPath(t *testing.T) {
	cfg := recall.DefaultConfig()
	want := "./data/lore.db"
	if cfg.LocalPath != want {
		t.Errorf("DefaultConfig().LocalPath = %q, want %q", cfg.LocalPath, want)
	}
}

// TestWithDefaults_AppliesLocalPath verifies WithDefaults fills LocalPath when empty.
// Story 5.5: Supersedes DEV-1 decision — UX improvement for zero-config first run.
func TestWithDefaults_AppliesLocalPath(t *testing.T) {
	cfg := recall.Config{}.WithDefaults()
	want := "./data/lore.db"
	if cfg.LocalPath != want {
		t.Errorf("WithDefaults().LocalPath = %q, want %q", cfg.LocalPath, want)
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
// Story 5.5: ConfigFromEnv().WithDefaults() should fill ./data/lore.db when env is unset.
func TestConfigFromEnv_WithDefaults_AppliesLocalPath(t *testing.T) {
	// Save and clear env
	origPath := os.Getenv("RECALL_DB_PATH")
	_ = os.Unsetenv("RECALL_DB_PATH")
	defer func() {
		if origPath == "" {
			_ = os.Unsetenv("RECALL_DB_PATH")
		} else {
			_ = os.Setenv("RECALL_DB_PATH", origPath)
		}
	}()

	cfg := recall.ConfigFromEnv().WithDefaults()
	want := "./data/lore.db"
	if cfg.LocalPath != want {
		t.Errorf("ConfigFromEnv().WithDefaults().LocalPath = %q, want %q", cfg.LocalPath, want)
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
