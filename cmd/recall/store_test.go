package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hyperengineering/recall"
	"github.com/hyperengineering/recall/internal/store"
)

// testStoreEnv sets up a test environment with a temporary stores directory.
func testStoreEnv(t *testing.T) (storeRoot string, cleanup func()) {
	t.Helper()

	tmpDir := t.TempDir()
	storeRoot = filepath.Join(tmpDir, ".recall", "stores")

	// Save original env
	origHome := os.Getenv("HOME")
	origEngramStore := os.Getenv("ENGRAM_STORE")

	// Set test env - make tmpDir the home directory
	os.Setenv("HOME", tmpDir)
	os.Unsetenv("ENGRAM_STORE")

	// Reset global flags
	cfgLorePath = ""
	cfgStore = ""
	outputJSON = false
	storeDescription = ""
	storeDeleteConfirm = false
	storeDeleteForce = false

	return storeRoot, func() {
		os.Setenv("HOME", origHome)
		if origEngramStore != "" {
			os.Setenv("ENGRAM_STORE", origEngramStore)
		} else {
			os.Unsetenv("ENGRAM_STORE")
		}
		cfgLorePath = ""
		cfgStore = ""
		outputJSON = false
		storeDescription = ""
		storeDeleteConfirm = false
		storeDeleteForce = false
	}
}

func TestCLI_StoreList_Empty(t *testing.T) {
	_, cleanup := testStoreEnv(t)
	defer cleanup()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"store", "list"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("store list should not error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "No stores found") {
		t.Errorf("output should indicate no stores, got: %s", output)
	}
}

func TestCLI_StoreList_WithStores(t *testing.T) {
	storeRoot, cleanup := testStoreEnv(t)
	defer cleanup()

	// Create a store manually
	storeDir := filepath.Join(storeRoot, "test-project")
	os.MkdirAll(storeDir, 0755)
	dbPath := filepath.Join(storeDir, "lore.db")
	s, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	s.SetStoreDescription("Test project store")
	s.Close()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"store", "list"})

	err = rootCmd.Execute()
	if err != nil {
		t.Fatalf("store list should not error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "test-project") {
		t.Errorf("output should contain store ID, got: %s", output)
	}
	if !strings.Contains(output, "Test project store") {
		t.Errorf("output should contain description, got: %s", output)
	}
}

func TestCLI_StoreList_JSON(t *testing.T) {
	storeRoot, cleanup := testStoreEnv(t)
	defer cleanup()

	// Create a store
	storeDir := filepath.Join(storeRoot, "my-store")
	os.MkdirAll(storeDir, 0755)
	dbPath := filepath.Join(storeDir, "lore.db")
	s, _ := recall.NewStore(dbPath)
	s.Close()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"store", "list", "--json"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("store list --json should not error: %v", err)
	}

	var result StoreListResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Errorf("output should be valid JSON: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("total = %d, want 1", result.Total)
	}
	if len(result.Stores) != 1 || result.Stores[0].ID != "my-store" {
		t.Errorf("stores = %v, want [{ID: my-store}]", result.Stores)
	}
}

func TestCLI_StoreCreate_Valid(t *testing.T) {
	_, cleanup := testStoreEnv(t)
	defer cleanup()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"store", "create", "new-project"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("store create should not error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Store created") {
		t.Errorf("output should confirm creation, got: %s", output)
	}
	if !strings.Contains(output, "new-project") {
		t.Errorf("output should contain store ID, got: %s", output)
	}

	// Verify store was created
	dbPath := store.StoreDBPath("new-project")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("store database should have been created")
	}
}

func TestCLI_StoreCreate_WithDescription(t *testing.T) {
	storeRoot, cleanup := testStoreEnv(t)
	defer cleanup()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"store", "create", "desc-project", "--description", "Project with description"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("store create with description should not error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Project with description") {
		t.Errorf("output should contain description, got: %s", output)
	}

	// Verify description was stored
	dbPath := filepath.Join(storeRoot, "desc-project", "lore.db")
	s, _ := recall.NewStore(dbPath)
	defer s.Close()
	desc, _ := s.GetStoreDescription()
	if desc != "Project with description" {
		t.Errorf("description = %q, want %q", desc, "Project with description")
	}
}

func TestCLI_StoreCreate_InvalidUppercase(t *testing.T) {
	_, cleanup := testStoreEnv(t)
	defer cleanup()

	rootCmd.SetArgs([]string{"store", "create", "My-Project"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("store create with uppercase should error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "invalid store ID") {
		t.Errorf("error should mention invalid store ID, got: %s", errMsg)
	}
}

func TestCLI_StoreCreate_InvalidUnderscore(t *testing.T) {
	_, cleanup := testStoreEnv(t)
	defer cleanup()

	rootCmd.SetArgs([]string{"store", "create", "my_project"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("store create with underscore should error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "invalid store ID") {
		t.Errorf("error should mention invalid store ID, got: %s", errMsg)
	}
}

func TestCLI_StoreCreate_AlreadyExists(t *testing.T) {
	storeRoot, cleanup := testStoreEnv(t)
	defer cleanup()

	// Create store first
	storeDir := filepath.Join(storeRoot, "existing")
	os.MkdirAll(storeDir, 0755)
	dbPath := filepath.Join(storeDir, "lore.db")
	s, _ := recall.NewStore(dbPath)
	s.Close()

	rootCmd.SetArgs([]string{"store", "create", "existing"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("store create for existing store should error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "already exists") {
		t.Errorf("error should mention store exists, got: %s", errMsg)
	}
}

func TestCLI_StoreCreate_Reserved(t *testing.T) {
	_, cleanup := testStoreEnv(t)
	defer cleanup()

	rootCmd.SetArgs([]string{"store", "create", "default"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("store create for reserved ID should error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "reserved") {
		t.Errorf("error should mention reserved, got: %s", errMsg)
	}
}

func TestCLI_StoreDelete_NoConfirm(t *testing.T) {
	_, cleanup := testStoreEnv(t)
	defer cleanup()

	rootCmd.SetArgs([]string{"store", "delete", "some-store"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("store delete without --confirm should error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "--confirm") {
		t.Errorf("error should mention --confirm, got: %s", errMsg)
	}
}

func TestCLI_StoreDelete_ProtectedDefault(t *testing.T) {
	_, cleanup := testStoreEnv(t)
	defer cleanup()

	storeDeleteConfirm = true
	storeDeleteForce = true
	rootCmd.SetArgs([]string{"store", "delete", "default", "--confirm", "--force"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("store delete default should error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "protected") || !strings.Contains(errMsg, "default") {
		t.Errorf("error should mention protected default store, got: %s", errMsg)
	}
}

func TestCLI_StoreDelete_NotFound(t *testing.T) {
	_, cleanup := testStoreEnv(t)
	defer cleanup()

	storeDeleteConfirm = true
	storeDeleteForce = true
	rootCmd.SetArgs([]string{"store", "delete", "nonexistent", "--confirm", "--force"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("store delete nonexistent should error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "not found") {
		t.Errorf("error should mention not found, got: %s", errMsg)
	}
}

func TestCLI_StoreDelete_WithForce(t *testing.T) {
	storeRoot, cleanup := testStoreEnv(t)
	defer cleanup()

	// Create store first
	storeDir := filepath.Join(storeRoot, "to-delete")
	os.MkdirAll(storeDir, 0755)
	dbPath := filepath.Join(storeDir, "lore.db")
	s, _ := recall.NewStore(dbPath)
	s.Close()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	storeDeleteConfirm = true
	storeDeleteForce = true
	rootCmd.SetArgs([]string{"store", "delete", "to-delete", "--confirm", "--force"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("store delete with --force should not error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Store deleted") {
		t.Errorf("output should confirm deletion, got: %s", output)
	}

	// Verify store was deleted
	if _, err := os.Stat(storeDir); !os.IsNotExist(err) {
		t.Error("store directory should have been deleted")
	}
}

func TestCLI_StoreInfo_Explicit(t *testing.T) {
	storeRoot, cleanup := testStoreEnv(t)
	defer cleanup()

	// Create store with some data
	storeDir := filepath.Join(storeRoot, "info-test")
	os.MkdirAll(storeDir, 0755)
	dbPath := filepath.Join(storeDir, "lore.db")
	s, _ := recall.NewStore(dbPath)
	s.SetStoreDescription("Info test store")
	s.Close()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"store", "info", "info-test"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("store info should not error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "info-test") {
		t.Errorf("output should contain store ID, got: %s", output)
	}
	if !strings.Contains(output, "Info test store") {
		t.Errorf("output should contain description, got: %s", output)
	}
	if !strings.Contains(output, "Lore Count") {
		t.Errorf("output should contain lore count, got: %s", output)
	}
}

func TestCLI_StoreInfo_Resolved(t *testing.T) {
	storeRoot, cleanup := testStoreEnv(t)
	defer cleanup()

	// Create default store
	storeDir := filepath.Join(storeRoot, "default")
	os.MkdirAll(storeDir, 0755)
	dbPath := filepath.Join(storeDir, "lore.db")
	s, _ := recall.NewStore(dbPath)
	s.Close()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"store", "info"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("store info without ID should not error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "default") {
		t.Errorf("output should contain resolved store ID, got: %s", output)
	}
	if !strings.Contains(output, "resolved from environment") {
		t.Errorf("output should indicate resolution, got: %s", output)
	}
}

func TestCLI_StoreInfo_JSON(t *testing.T) {
	storeRoot, cleanup := testStoreEnv(t)
	defer cleanup()

	// Create store
	storeDir := filepath.Join(storeRoot, "json-test")
	os.MkdirAll(storeDir, 0755)
	dbPath := filepath.Join(storeDir, "lore.db")
	s, _ := recall.NewStore(dbPath)
	s.Close()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"store", "info", "json-test", "--json"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("store info --json should not error: %v", err)
	}

	var result StoreInfoResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Errorf("output should be valid JSON: %v", err)
	}
	if result.ID != "json-test" {
		t.Errorf("ID = %q, want %q", result.ID, "json-test")
	}
}

func TestCLI_StoreInfo_NotFound(t *testing.T) {
	_, cleanup := testStoreEnv(t)
	defer cleanup()

	rootCmd.SetArgs([]string{"store", "info", "nonexistent"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("store info for nonexistent should error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "not found") {
		t.Errorf("error should mention not found, got: %s", errMsg)
	}
}

func TestCLI_StoreFlag_Record(t *testing.T) {
	storeRoot, cleanup := testStoreEnv(t)
	defer cleanup()

	// Create store
	storeDir := filepath.Join(storeRoot, "flag-test")
	os.MkdirAll(storeDir, 0755)
	dbPath := filepath.Join(storeDir, "lore.db")
	s, _ := recall.NewStore(dbPath)
	s.Close()

	// Ensure offline mode
	os.Unsetenv("ENGRAM_URL")
	cfgEngramURL = ""

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	recordContent = ""
	recordCategory = ""
	rootCmd.SetArgs([]string{"record", "--store", "flag-test", "--content", "Test lore", "-c", "PATTERN_OUTCOME"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("record with --store should not error: %v", err)
	}

	// Verify lore was recorded in the specified store
	s2, _ := recall.NewStore(dbPath)
	defer s2.Close()
	stats, _ := s2.Stats()
	if stats.LoreCount != 1 {
		t.Errorf("lore count = %d, want 1", stats.LoreCount)
	}
}

func TestCLI_StoreFlag_Query(t *testing.T) {
	storeRoot, cleanup := testStoreEnv(t)
	defer cleanup()

	// Create store with some lore
	storeDir := filepath.Join(storeRoot, "query-test")
	os.MkdirAll(storeDir, 0755)
	dbPath := filepath.Join(storeDir, "lore.db")
	s, _ := recall.NewStore(dbPath)
	s.Close()

	// Ensure offline mode
	os.Unsetenv("ENGRAM_URL")
	cfgEngramURL = ""

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	queryTop = 5
	queryMinConfidence = 0.0
	queryCategory = ""
	rootCmd.SetArgs([]string{"query", "test", "--store", "query-test"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("query with --store should not error: %v", err)
	}

	// Command executed without error - that's the main assertion
	// (Empty store returns "No matching lore")
}

func TestCLI_Store_WorksOffline(t *testing.T) {
	_, cleanup := testStoreEnv(t)
	defer cleanup()

	// Ensure no Engram URL is set
	os.Unsetenv("ENGRAM_URL")
	cfgEngramURL = ""

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"store", "create", "offline-test"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("store commands should work offline: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Store created") {
		t.Errorf("store create should succeed offline, got: %s", output)
	}
}

func TestCLI_Store_PathStyle(t *testing.T) {
	storeRoot, cleanup := testStoreEnv(t)
	defer cleanup()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"store", "create", "org/team/project"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("store create with path-style ID should not error: %v", err)
	}

	// Verify store was created with encoded path
	encodedDir := filepath.Join(storeRoot, "org__team__project")
	if _, err := os.Stat(encodedDir); os.IsNotExist(err) {
		t.Error("store directory should exist with encoded path")
	}
}
