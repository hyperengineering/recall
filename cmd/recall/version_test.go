package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// resetVersionFlags resets global version flag state between tests.
func resetVersionFlags() {
	outputJSON = false
}

func TestVersion_Human_ShowsVersionInfo(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()
	resetVersionFlags()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"version"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("version command should not error: %v", err)
	}

	output := stdout.String()

	// Should show version line
	if !strings.Contains(output, "recall ") {
		t.Error("output should start with 'recall '")
	}

	// Should show commit
	if !strings.Contains(output, "commit:") {
		t.Error("output should contain 'commit:'")
	}

	// Should show build date
	if !strings.Contains(output, "built:") {
		t.Error("output should contain 'built:'")
	}

	// Should show Go version
	if !strings.Contains(output, "go:") {
		t.Error("output should contain 'go:'")
	}

	// Should show OS/arch
	if !strings.Contains(output, "os:") {
		t.Error("output should contain 'os:'")
	}
}

func TestVersion_JSON_ReturnsValidJSON(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()
	resetVersionFlags()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"version", "--json"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("version --json should not error: %v", err)
	}

	output := stdout.String()

	// Verify it's valid JSON
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output should be valid JSON: %v", err)
	}

	// Check required fields
	requiredFields := []string{"version", "commit", "date", "go", "os", "arch"}
	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			t.Errorf("JSON should have '%s' field", field)
		}
	}
}

func TestVersion_DevBuild_ShowsDev(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()
	resetVersionFlags()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"version"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("version command should not error: %v", err)
	}

	output := stdout.String()

	// Without ldflags, version should show "dev"
	if !strings.Contains(output, "recall dev") {
		t.Errorf("dev build should show 'recall dev', got: %s", output)
	}
}

func TestVersion_JSON_ShowsDevVersion(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()
	resetVersionFlags()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"version", "--json"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("version --json should not error: %v", err)
	}

	output := stdout.String()

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output should be valid JSON: %v", err)
	}

	if result["version"] != "dev" {
		t.Errorf("dev build JSON should have version='dev', got: %v", result["version"])
	}
}

func TestVersion_InHelpOutput(t *testing.T) {
	cleanup := testEnv(t)
	defer cleanup()

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("--help should not error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "version") {
		t.Error("--help should list 'version' command")
	}
}
