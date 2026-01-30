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

// TestCLI_Record_DefaultPath verifies record works with default path when no config is set.
// Story 5.5: Zero-configuration first run.
func TestCLI_Record_DefaultPath(t *testing.T) {
	// Save original env and flags
	origDBPath := os.Getenv("RECALL_DB_PATH")
	origEngramURL := os.Getenv("ENGRAM_URL")

	// Use temp dir for default path to avoid polluting workspace
	tmpDir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origWd)

	// Clear all config - should use default ./data/lore.db
	os.Setenv("RECALL_DB_PATH", "")
	os.Setenv("ENGRAM_URL", "")
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

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"record", "--content", "zero config test", "-c", "PATTERN_OUTCOME"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("record with default path should succeed, got error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Recorded:") {
		t.Errorf("output should contain 'Recorded:', got: %s", output)
	}

	// Verify database was created in default location
	if _, err := os.Stat(filepath.Join(tmpDir, "data", "lore.db")); os.IsNotExist(err) {
		t.Error("default database ./data/lore.db should have been created")
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

// TestCLI_Session_DefaultPath verifies session works with default path when no config is set.
// Story 5.5 AC#1: Zero-configuration first run.
func TestCLI_Session_DefaultPath(t *testing.T) {
	origDBPath := os.Getenv("RECALL_DB_PATH")
	origEngramURL := os.Getenv("ENGRAM_URL")

	tmpDir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origWd)

	os.Setenv("RECALL_DB_PATH", "")
	os.Setenv("ENGRAM_URL", "")
	cfgLorePath = ""
	cfgEngramURL = ""

	defer func() {
		os.Setenv("RECALL_DB_PATH", origDBPath)
		os.Setenv("ENGRAM_URL", origEngramURL)
		cfgLorePath = ""
		cfgEngramURL = ""
	}()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"session"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("session with default path should succeed, got error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "No lore surfaced") {
		t.Errorf("output should indicate empty session, got: %s", output)
	}
}

// TestCLI_Stats_DefaultPath verifies stats works with default path when no config is set.
// Story 5.5 AC#2: Zero-configuration first run.
func TestCLI_Stats_DefaultPath(t *testing.T) {
	origDBPath := os.Getenv("RECALL_DB_PATH")
	origEngramURL := os.Getenv("ENGRAM_URL")

	tmpDir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origWd)

	os.Setenv("RECALL_DB_PATH", "")
	os.Setenv("ENGRAM_URL", "")
	cfgLorePath = ""
	cfgEngramURL = ""

	defer func() {
		os.Setenv("RECALL_DB_PATH", origDBPath)
		os.Setenv("ENGRAM_URL", origEngramURL)
		cfgLorePath = ""
		cfgEngramURL = ""
	}()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"stats"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("stats with default path should succeed, got error: %v", err)
	}

	// Stats command succeeded - that's the main assertion for AC#2
	// The output format is tested elsewhere; here we just verify it works with default path
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

// TestCLI_Config_DefaultPath verifies loadConfig uses default when neither flag nor env is set.
// Story 5.5: Zero-configuration first run. Precedence: flag > env > default.
func TestCLI_Config_DefaultPath(t *testing.T) {
	// Save original env
	origDBPath := os.Getenv("RECALL_DB_PATH")

	// Clear all config sources
	os.Setenv("RECALL_DB_PATH", "")
	cfgLorePath = ""

	defer func() {
		os.Setenv("RECALL_DB_PATH", origDBPath)
		cfgLorePath = ""
	}()

	cfg := loadConfig()
	want := "./data/lore.db"
	if cfg.LocalPath != want {
		t.Errorf("loadConfig().LocalPath = %q, want %q (default)", cfg.LocalPath, want)
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

// ============================================================================
// Story 5.2 Tests: Query, Feedback, Sync, Session
// ============================================================================

func resetQueryFlags() {
	queryTop = 5
	queryMinConfidence = 0.0
	queryCategory = ""
}

func resetFeedbackFlags() {
	feedbackID = ""
	feedbackType = ""
	feedbackHelpful = ""
	feedbackNotRelevant = ""
	feedbackIncorrect = ""
}

func TestCLI_Query_NoResults(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()
	resetQueryFlags()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"query", "nonexistent search term xyz"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("query with no results should not error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "No matching lore found") {
		t.Errorf("output should indicate no results, got: %s", output)
	}
}

func TestCLI_Query_TopFlag(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()
	resetQueryFlags()

	// The query should work even with --top flag (even if no results)
	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"query", "test", "--top", "3"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("query with --top should work: %v", err)
	}
}

func TestCLI_Query_ShortFlag(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()
	resetQueryFlags()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"query", "test", "-k", "10"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("query with -k should work: %v", err)
	}
}

func TestCLI_Query_CategoryFlag(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()
	resetQueryFlags()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"query", "test", "--category", "PATTERN_OUTCOME"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("query with --category should work: %v", err)
	}
}

func TestCLI_Query_JSON(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()
	resetQueryFlags()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"query", "test", "--json"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("query with --json should work: %v", err)
	}

	output := stdout.String()

	// Verify it's valid JSON with expected structure
	var result recall.QueryResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Errorf("output should be valid QueryResult JSON: %v", err)
	}

	// Check for snake_case fields
	if !strings.Contains(output, `"lore"`) {
		t.Error("JSON should have 'lore' field")
	}
	if !strings.Contains(output, `"session_refs"`) {
		t.Error("JSON should have 'session_refs' field (snake_case)")
	}
}

func TestCLI_Feedback_InvalidType(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()
	resetFeedbackFlags()

	rootCmd.SetArgs([]string{"feedback", "--id", "L1", "--type", "wrong"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("feedback with invalid type should error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "invalid feedback type") {
		t.Errorf("error should mention invalid type, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "helpful") {
		t.Errorf("error should list valid types, got: %s", errMsg)
	}
}

func TestCLI_Feedback_MissingID(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()
	resetFeedbackFlags()

	rootCmd.SetArgs([]string{"feedback", "--type", "helpful"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("feedback without --id should error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "--id") {
		t.Errorf("error should mention --id required, got: %s", errMsg)
	}
}

func TestCLI_Feedback_MissingType(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()
	resetFeedbackFlags()

	rootCmd.SetArgs([]string{"feedback", "--id", "L1"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("feedback without --type should error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "--type") {
		t.Errorf("error should mention --type required, got: %s", errMsg)
	}
}

func TestCLI_Feedback_NoFlags(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()
	resetFeedbackFlags()

	rootCmd.SetArgs([]string{"feedback"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("feedback with no flags should error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "provide") {
		t.Errorf("error should ask to provide flags, got: %s", errMsg)
	}
}

func TestCLI_Feedback_MixedModes(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()
	resetFeedbackFlags()

	rootCmd.SetArgs([]string{"feedback", "--id", "L1", "--type", "helpful", "--helpful", "L2"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("mixing single and batch modes should error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "cannot mix") {
		t.Errorf("error should mention cannot mix modes, got: %s", errMsg)
	}
}

func TestCLI_Feedback_TypeVariants(t *testing.T) {
	// Test that different variants of not_relevant are accepted
	tests := []struct {
		input    string
		expected recall.FeedbackType
	}{
		{"helpful", recall.Helpful},
		{"HELPFUL", recall.Helpful},
		{"Helpful", recall.Helpful},
		{"incorrect", recall.Incorrect},
		{"INCORRECT", recall.Incorrect},
		{"not_relevant", recall.NotRelevant},
		{"not-relevant", recall.NotRelevant},
		{"notrelevant", recall.NotRelevant},
	}

	for _, tc := range tests {
		ft, err := parseFeedbackType(tc.input)
		if err != nil {
			t.Errorf("parseFeedbackType(%q) should not error: %v", tc.input, err)
			continue
		}
		if ft != tc.expected {
			t.Errorf("parseFeedbackType(%q) = %v, want %v", tc.input, ft, tc.expected)
		}
	}
}

func TestCLI_Sync_Help(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"sync", "--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("sync --help should work: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "push") {
		t.Error("sync help should list push subcommand")
	}
	if !strings.Contains(output, "bootstrap") {
		t.Error("sync help should list bootstrap subcommand")
	}
}

func TestCLI_SyncPush_Offline(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	// Ensure ENGRAM_URL is empty (offline mode)
	os.Setenv("ENGRAM_URL", "")

	rootCmd.SetArgs([]string{"sync", "push"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("sync push in offline mode should error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "offline") {
		t.Errorf("error should mention offline mode, got: %s", errMsg)
	}
}

func TestCLI_SyncBootstrap_Offline(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	// Ensure ENGRAM_URL is empty (offline mode)
	os.Setenv("ENGRAM_URL", "")

	rootCmd.SetArgs([]string{"sync", "bootstrap"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("sync bootstrap in offline mode should error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "offline") {
		t.Errorf("error should mention offline mode, got: %s", errMsg)
	}
}

func TestCLI_Session_JSON(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"session", "--json"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("session --json should work: %v", err)
	}

	output := strings.TrimSpace(stdout.String())

	// Should be valid JSON array
	var result []recall.SessionLore
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Errorf("output should be valid JSON array: %v", err)
	}
}

// TestCLI_Query_DefaultPath verifies query works with default path when no config is set.
// Story 5.5: Zero-configuration first run.
func TestCLI_Query_DefaultPath(t *testing.T) {
	// Save original env and flags
	origDBPath := os.Getenv("RECALL_DB_PATH")
	origEngramURL := os.Getenv("ENGRAM_URL")

	// Use temp dir for default path to avoid polluting workspace
	tmpDir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origWd)

	// Clear all config - should use default ./data/lore.db
	os.Setenv("RECALL_DB_PATH", "")
	os.Setenv("ENGRAM_URL", "")
	cfgLorePath = ""
	cfgEngramURL = ""
	resetQueryFlags()

	defer func() {
		os.Setenv("RECALL_DB_PATH", origDBPath)
		os.Setenv("ENGRAM_URL", origEngramURL)
		cfgLorePath = ""
		cfgEngramURL = ""
	}()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"query", "test"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("query with default path should succeed, got error: %v", err)
	}

	output := stdout.String()
	// Query with no results should show "No matching lore"
	if !strings.Contains(output, "No matching lore") {
		t.Errorf("output should indicate no results (fresh db), got: %s", output)
	}
}
