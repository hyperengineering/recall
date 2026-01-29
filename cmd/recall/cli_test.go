package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hyperengineering/recall"
)

// testEnv sets up a test environment with a temporary database.
// Returns a cleanup function.
func testEnv(t *testing.T) func() {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Save original env
	origDBPath := os.Getenv("RECALL_DB_PATH")
	origEngramURL := os.Getenv("ENGRAM_URL")
	origAPIKey := os.Getenv("ENGRAM_API_KEY")
	origSourceID := os.Getenv("RECALL_SOURCE_ID")

	// Set test env
	os.Setenv("RECALL_DB_PATH", dbPath)
	os.Setenv("ENGRAM_URL", "")
	os.Setenv("ENGRAM_API_KEY", "")
	os.Setenv("RECALL_SOURCE_ID", "test-client")

	// Reset global flags
	cfgLorePath = ""
	cfgEngramURL = ""
	cfgAPIKey = ""
	cfgSourceID = ""
	outputJSON = false

	return func() {
		os.Setenv("RECALL_DB_PATH", origDBPath)
		os.Setenv("ENGRAM_URL", origEngramURL)
		os.Setenv("ENGRAM_API_KEY", origAPIKey)
		os.Setenv("RECALL_SOURCE_ID", origSourceID)

		// Reset globals
		cfgLorePath = ""
		cfgEngramURL = ""
		cfgAPIKey = ""
		cfgSourceID = ""
		outputJSON = false
		recordContent = ""
		recordCategory = ""
		recordContext = ""
		recordConfidence = 0.5
	}
}

func TestCLI_Help_ListsAllCommands(t *testing.T) {
	defer testEnv(t)()

	// Capture output
	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	expectedCommands := []string{"record", "query", "feedback", "sync", "session", "stats"}

	for _, cmd := range expectedCommands {
		if !strings.Contains(output, cmd) {
			t.Errorf("--help output should contain %q command", cmd)
		}
	}
}

func TestCLI_Record_MissingConfig(t *testing.T) {
	// Don't use testEnv - test with missing RECALL_DB_PATH
	origDBPath := os.Getenv("RECALL_DB_PATH")
	origEngramURL := os.Getenv("ENGRAM_URL")
	os.Setenv("RECALL_DB_PATH", "")
	os.Setenv("ENGRAM_URL", "")

	// Reset globals
	cfgLorePath = ""
	cfgEngramURL = ""
	cfgAPIKey = ""
	cfgSourceID = ""
	outputJSON = false
	recordContent = ""
	recordCategory = ""

	defer func() {
		os.Setenv("RECALL_DB_PATH", origDBPath)
		os.Setenv("ENGRAM_URL", origEngramURL)
		cfgLorePath = ""
		cfgEngramURL = ""
		cfgAPIKey = ""
		cfgSourceID = ""
		outputJSON = false
		recordContent = ""
		recordCategory = ""
	}()

	var stderr bytes.Buffer
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs([]string{"record", "--content", "test", "-c", "PATTERN_OUTCOME"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing config")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "LocalPath") {
		t.Errorf("error should mention LocalPath, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "RECALL_DB_PATH") {
		t.Errorf("error should mention RECALL_DB_PATH env var, got: %s", errMsg)
	}
}

func TestCLI_Record_Success(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"record", "--content", "Test lore entry", "-c", "PATTERN_OUTCOME"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Recorded:") {
		t.Error("output should contain 'Recorded:'")
	}
	if !strings.Contains(output, "Category: PATTERN_OUTCOME") {
		t.Error("output should contain category")
	}
	if !strings.Contains(output, "Confidence: 0.50") {
		t.Error("output should contain default confidence")
	}
}

func TestCLI_Record_WithContext(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"record", "--content", "Test", "-c", "PATTERN_OUTCOME", "--context", "story-1"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Context: story-1") {
		t.Errorf("output should contain context, got: %s", output)
	}
}

func TestCLI_Record_WithConfidence(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"record", "--content", "Test", "-c", "PATTERN_OUTCOME", "--confidence", "0.8"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Confidence: 0.80") {
		t.Errorf("output should show confidence 0.80, got: %s", output)
	}
}

func TestCLI_Record_JSONOutput(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"record", "--content", "Test JSON", "-c", "PATTERN_OUTCOME", "--json"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()

	// Verify it's valid JSON
	var lore recall.Lore
	if err := json.Unmarshal([]byte(output), &lore); err != nil {
		t.Errorf("output should be valid JSON: %v", err)
	}

	// Verify snake_case fields by checking raw JSON
	if !strings.Contains(output, `"id"`) {
		t.Error("JSON should have 'id' field (snake_case)")
	}
	if !strings.Contains(output, `"category"`) {
		t.Error("JSON should have 'category' field")
	}
	if !strings.Contains(output, `"validation_count"`) {
		t.Error("JSON should have 'validation_count' field (snake_case)")
	}
	if !strings.Contains(output, `"source_id"`) {
		t.Error("JSON should have 'source_id' field (snake_case)")
	}
	if !strings.Contains(output, `"created_at"`) {
		t.Error("JSON should have 'created_at' field (snake_case)")
	}
}

func TestCLI_JSONOutput_ExcludesEmbedding(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"record", "--content", "Test embedding exclusion", "-c", "PATTERN_OUTCOME", "--json"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if strings.Contains(output, `"embedding"`) {
		t.Error("JSON should NOT contain 'embedding' field")
	}
}

func TestCLI_Record_InvalidCategory(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	rootCmd.SetArgs([]string{"record", "--content", "Test", "-c", "INVALID"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid category")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "ARCHITECTURAL_DECISION") {
		t.Errorf("error should list valid categories, got: %s", errMsg)
	}
}

func TestCLI_Record_MissingContent(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	rootCmd.SetArgs([]string{"record", "-c", "PATTERN_OUTCOME"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing content flag")
	}

	errMsg := err.Error()
	// Library validation catches empty content
	if !strings.Contains(errMsg, "Content") && !strings.Contains(errMsg, "content") {
		t.Errorf("error should mention content, got: %s", errMsg)
	}
}

func TestCLI_Record_MissingCategory(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	rootCmd.SetArgs([]string{"record", "--content", "Test"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing category flag")
	}

	errMsg := err.Error()
	// Library validation catches invalid/empty category
	if !strings.Contains(errMsg, "Category") && !strings.Contains(errMsg, "category") {
		t.Errorf("error should mention category, got: %s", errMsg)
	}
}

func TestCLI_Session_EmptySession(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"session"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "No lore surfaced") {
		t.Errorf("output should indicate empty session, got: %s", output)
	}
}

func TestCLI_Session_JSONEmpty(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"session", "--json"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := strings.TrimSpace(stdout.String())
	if output != "[]" {
		t.Errorf("JSON output should be empty array, got: %s", output)
	}
}

func TestCLI_Config_FlagOverridesEnv(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	// Set env to one path
	os.Setenv("RECALL_DB_PATH", "/env/path.db")

	// Set flag to another
	tmpDir := t.TempDir()
	flagPath := filepath.Join(tmpDir, "flag.db")
	cfgLorePath = flagPath

	cfg := loadConfig()
	if cfg.LocalPath != flagPath {
		t.Errorf("flag should override env, got LocalPath=%s, want %s", cfg.LocalPath, flagPath)
	}
}

func TestCLI_Config_EnvFallback(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	envPath := "/env/fallback.db"
	os.Setenv("RECALL_DB_PATH", envPath)
	cfgLorePath = "" // No flag set

	cfg := loadConfig()
	if cfg.LocalPath != envPath {
		t.Errorf("should use env when flag not set, got LocalPath=%s, want %s", cfg.LocalPath, envPath)
	}
}

func TestCLI_Config_SourceIDFromEnv(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	os.Setenv("RECALL_SOURCE_ID", "my-client-id")
	cfgSourceID = "" // No flag set

	cfg := loadConfig()
	if cfg.SourceID != "my-client-id" {
		t.Errorf("should load SourceID from env, got %s, want my-client-id", cfg.SourceID)
	}
}

func TestCLI_APIKey_NeverInOutput(t *testing.T) {
	// Set a fake API key
	secretKey := "sk-super-secret-key-12345"
	cfgAPIKey = secretKey

	// Test scrubSensitiveData
	input := "connection failed: auth error with " + secretKey + " token"
	scrubbed := scrubSensitiveData(input)

	if strings.Contains(scrubbed, secretKey) {
		t.Error("scrubSensitiveData should remove API key from messages")
	}
	if !strings.Contains(scrubbed, "[REDACTED]") {
		t.Error("scrubSensitiveData should replace API key with [REDACTED]")
	}

	cfgAPIKey = "" // Reset
}

func TestCLI_Record_ConfidenceZero(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"record", "--content", "Test zero confidence", "-c", "PATTERN_OUTCOME", "--confidence", "0"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("confidence 0 should be valid, got error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Confidence: 0.00") {
		t.Errorf("output should show confidence 0.00, got: %s", output)
	}
}

func TestCLI_Record_ConfidenceOne(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"record", "--content", "Test max confidence", "-c", "PATTERN_OUTCOME", "--confidence", "1"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("confidence 1 should be valid, got error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Confidence: 1.00") {
		t.Errorf("output should show confidence 1.00, got: %s", output)
	}
}

func TestCLI_Record_InvalidConfidence(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	rootCmd.SetArgs([]string{"record", "--content", "Test", "-c", "PATTERN_OUTCOME", "--confidence", "1.5"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("confidence > 1 should be invalid")
	}
}

func TestCLI_Record_UnicodeContent(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"record", "--content", "日本語テスト Unicode 测试 🎉", "-c", "PATTERN_OUTCOME"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unicode content should be valid, got error: %v", err)
	}
}
