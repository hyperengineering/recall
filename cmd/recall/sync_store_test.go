package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSync_StoreFlag_Help(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"sync", "--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("sync --help failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "--store") {
		t.Error("sync --help should show --store flag")
	}
}

func TestSync_StoreFlag_Push_Help(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"sync", "push", "--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("sync push --help failed: %v", err)
	}

	output := stdout.String()
	// --store should be inherited from parent
	if !strings.Contains(output, "--store") {
		t.Error("sync push --help should show --store flag (inherited)")
	}
}

func TestSync_StoreFlag_InOutput(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	// Create temp DB for specific store
	tmpDir := t.TempDir()
	storeDBPath := filepath.Join(tmpDir, "stores", "test-store", "lore.db")

	// Set up config to use the temp store
	os.Setenv("RECALL_DB_PATH", storeDBPath)
	cfgLorePath = ""

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	// Test that --store flag is recognized (even if offline/error)
	rootCmd.SetArgs([]string{"sync", "push", "--store", "test-store"})

	// This will fail because no Engram URL, but that's OK - we're testing the flag parsing
	err := rootCmd.Execute()

	// Should fail with offline error, not unknown flag error
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "unknown flag") {
			t.Errorf("--store flag should be recognized, got: %v", err)
		}
		// Expected: "sync unavailable: ENGRAM_URL not configured" - this is fine
	}
}

func TestSync_StoreFlag_VariableSet(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	// Reset the store flag variable
	syncStore = ""

	// Create temp DB
	tmpDir := t.TempDir()
	os.Setenv("RECALL_DB_PATH", filepath.Join(tmpDir, "test.db"))
	cfgLorePath = ""

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"sync", "--store", "my-project", "--help"})

	_ = rootCmd.Execute()

	// After parsing, the syncStore variable should be set
	if syncStore != "my-project" {
		t.Errorf("syncStore = %q, want %q", syncStore, "my-project")
	}
}
