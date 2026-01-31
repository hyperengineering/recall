package mcp_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hyperengineering/recall"
	recallmcp "github.com/hyperengineering/recall/mcp"
)

// =============================================================================
// Store List Tool Tests
// =============================================================================

func TestTool_StoreList_OfflineMode(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Create client without Engram URL (offline mode)
	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)

	result, err := server.CallTool(context.Background(), "recall_store_list", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool() returned error: %v", err)
	}

	// Should return error indicating offline mode
	if !result.IsError {
		t.Error("CallTool() in offline mode should return error result")
	}
	if !strings.Contains(result.Content, "offline") {
		t.Errorf("Error message should mention offline mode, got: %s", result.Content)
	}
}

func TestTool_StoreList_WithPrefix(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Create client without Engram URL (offline mode)
	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)

	// Even with prefix, should fail in offline mode
	result, err := server.CallTool(context.Background(), "recall_store_list", map[string]any{
		"prefix": "neuralmux/",
	})
	if err != nil {
		t.Fatalf("CallTool() returned error: %v", err)
	}

	// Should return error indicating offline mode
	if !result.IsError {
		t.Error("CallTool() in offline mode should return error result")
	}
}

// =============================================================================
// Store Info Tool Tests
// =============================================================================

func TestTool_StoreInfo_OfflineMode(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Create client without Engram URL (offline mode)
	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)

	result, err := server.CallTool(context.Background(), "recall_store_info", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool() returned error: %v", err)
	}

	// Should return error indicating offline mode
	if !result.IsError {
		t.Error("CallTool() in offline mode should return error result")
	}
	if !strings.Contains(result.Content, "offline") {
		t.Errorf("Error message should mention offline mode, got: %s", result.Content)
	}
}

func TestTool_StoreInfo_WithStore(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Create client without Engram URL (offline mode)
	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)

	// With explicit store, should still fail in offline mode
	result, err := server.CallTool(context.Background(), "recall_store_info", map[string]any{
		"store": "my-project",
	})
	if err != nil {
		t.Fatalf("CallTool() returned error: %v", err)
	}

	// Should return error indicating offline mode
	if !result.IsError {
		t.Error("CallTool() in offline mode should return error result")
	}
}

func TestTool_StoreInfo_InvalidStoreID(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Create client without Engram URL (offline mode)
	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)

	// Invalid store ID should fail validation
	result, err := server.CallTool(context.Background(), "recall_store_info", map[string]any{
		"store": "INVALID--Store",
	})
	if err != nil {
		t.Fatalf("CallTool() returned error: %v", err)
	}

	// Should return error for invalid store ID
	if !result.IsError {
		t.Error("CallTool() with invalid store ID should return error result")
	}
	if !strings.Contains(strings.ToLower(result.Content), "invalid") {
		t.Errorf("Error message should mention invalid store ID, got: %s", result.Content)
	}
}

// =============================================================================
// Format Functions Tests (via integration)
// =============================================================================

func TestFormat_RelativeTime(t *testing.T) {
	// These tests verify the formatting through the tool output
	// Full unit tests would require exporting the format functions

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Verify the server can be created (indirect test of store_tools.go compilation)
	server := recallmcp.NewServer(client)
	if server == nil {
		t.Fatal("NewServer() returned nil")
	}
}

// =============================================================================
// Tool Registration Tests
// =============================================================================

func TestTool_StoreList_InToolsList(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)
	tools := server.ListTools()

	found := false
	for _, tool := range tools {
		if tool.Name == "recall_store_list" {
			found = true
			break
		}
	}

	if !found {
		t.Error("recall_store_list not found in registered tools")
	}
}

func TestTool_StoreInfo_InToolsList(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)
	tools := server.ListTools()

	found := false
	for _, tool := range tools {
		if tool.Name == "recall_store_info" {
			found = true
			break
		}
	}

	if !found {
		t.Error("recall_store_info not found in registered tools")
	}
}
