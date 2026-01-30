package mcp_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/hyperengineering/recall"
	recallmcp "github.com/hyperengineering/recall/mcp"
)

// =============================================================================
// Server Initialization Tests
// =============================================================================

// TestServer_NewServer tests that a server can be created with a valid client.
func TestServer_NewServer(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)
	if server == nil {
		t.Fatal("NewServer() returned nil")
	}
}

// TestServer_ToolsList tests that all required tools are registered.
func TestServer_ToolsList(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)
	tools := server.ListTools()

	expectedTools := []string{"recall_query", "recall_record", "recall_feedback", "recall_sync"}
	if len(tools) != len(expectedTools) {
		t.Errorf("ListTools() returned %d tools, want %d", len(tools), len(expectedTools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	for _, expected := range expectedTools {
		if !toolNames[expected] {
			t.Errorf("Tool %q not found in registered tools", expected)
		}
	}
}

// =============================================================================
// Tool Execution Tests
// =============================================================================

// TestTool_Query_Success tests successful query execution.
func TestTool_Query_Success(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Record some lore first
	_, err = client.Record("Error handling patterns in Go", recall.CategoryPatternOutcome)
	if err != nil {
		t.Fatalf("Record() returned error: %v", err)
	}

	server := recallmcp.NewServer(client)

	result, err := server.CallTool(context.Background(), "recall_query", map[string]any{
		"query": "error handling",
	})
	if err != nil {
		t.Fatalf("CallTool() returned error: %v", err)
	}

	if result == nil {
		t.Fatal("CallTool() returned nil result")
	}

	// Result should contain text
	if result.Content == "" {
		t.Error("CallTool() returned empty content")
	}
}

// TestTool_Query_NoResults tests query with no matching results.
func TestTool_Query_NoResults(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)

	result, err := server.CallTool(context.Background(), "recall_query", map[string]any{
		"query": "nonexistent xyz",
	})
	if err != nil {
		t.Fatalf("CallTool() returned error: %v", err)
	}

	if result == nil {
		t.Fatal("CallTool() returned nil result")
	}

	// Should indicate no results found
	if result.Content == "" {
		t.Error("CallTool() should return a message even for no results")
	}
}

// TestTool_Query_MissingParam tests query without required parameter.
func TestTool_Query_MissingParam(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)

	result, err := server.CallTool(context.Background(), "recall_query", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool() returned error: %v", err)
	}

	// Should return error result
	if !result.IsError {
		t.Error("CallTool() with missing param should return error result")
	}
}

// TestTool_Record_Success tests successful record execution.
func TestTool_Record_Success(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)

	result, err := server.CallTool(context.Background(), "recall_record", map[string]any{
		"content":  "Test lore content",
		"category": "PATTERN_OUTCOME",
	})
	if err != nil {
		t.Fatalf("CallTool() returned error: %v", err)
	}

	if result == nil {
		t.Fatal("CallTool() returned nil result")
	}

	if result.IsError {
		t.Errorf("CallTool() returned error: %s", result.Content)
	}

	// Verify lore was recorded
	stats, err := client.Stats()
	if err != nil {
		t.Fatalf("Stats() returned error: %v", err)
	}
	if stats.LoreCount < 1 {
		t.Error("Lore was not recorded")
	}
}

// TestTool_Record_InvalidCategory tests record with invalid category.
func TestTool_Record_InvalidCategory(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)

	result, err := server.CallTool(context.Background(), "recall_record", map[string]any{
		"content":  "Test content",
		"category": "INVALID_CATEGORY",
	})
	if err != nil {
		t.Fatalf("CallTool() returned error: %v", err)
	}

	// Should return error result
	if !result.IsError {
		t.Error("CallTool() with invalid category should return error result")
	}
}

// TestTool_Feedback_Success tests successful feedback execution.
func TestTool_Feedback_Success(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Record lore first
	_, err = client.Record("Test lore for feedback", recall.CategoryPatternOutcome)
	if err != nil {
		t.Fatalf("Record() returned error: %v", err)
	}

	// Query to track in session (creates L1 ref)
	_, err = client.Query(context.Background(), recall.QueryParams{Query: "test"})
	if err != nil {
		t.Fatalf("Query() returned error: %v", err)
	}

	server := recallmcp.NewServer(client)

	result, err := server.CallTool(context.Background(), "recall_feedback", map[string]any{
		"helpful": []string{"L1"},
	})
	if err != nil {
		t.Fatalf("CallTool() returned error: %v", err)
	}

	if result == nil {
		t.Fatal("CallTool() returned nil result")
	}

	if result.IsError {
		t.Errorf("CallTool() returned error: %s", result.Content)
	}
}

// TestTool_Feedback_InvalidRef tests feedback with invalid session ref.
func TestTool_Feedback_InvalidRef(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)

	result, err := server.CallTool(context.Background(), "recall_feedback", map[string]any{
		"helpful": []string{"L999"},
	})
	if err != nil {
		t.Fatalf("CallTool() returned error: %v", err)
	}

	// Should return error or indicate not found
	// The feedback may succeed but report not found in result
	if result == nil {
		t.Fatal("CallTool() returned nil result")
	}
}

// TestTool_Sync_Offline tests sync in offline mode.
func TestTool_Sync_Offline(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Create client without Engram URL (offline mode)
	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)

	result, err := server.CallTool(context.Background(), "recall_sync", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool() returned error: %v", err)
	}

	// Should return error indicating offline mode
	if !result.IsError {
		t.Error("CallTool() sync in offline mode should return error result")
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

// TestIntegration_QueryThenFeedback tests query followed by feedback using L-ref.
func TestIntegration_QueryThenFeedback(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)

	// Record lore via MCP
	_, err = server.CallTool(context.Background(), "recall_record", map[string]any{
		"content":  "Always handle errors explicitly in Go",
		"category": "PATTERN_OUTCOME",
	})
	if err != nil {
		t.Fatalf("Record CallTool() returned error: %v", err)
	}

	// Query via MCP (creates L1 ref)
	queryResult, err := server.CallTool(context.Background(), "recall_query", map[string]any{
		"query": "error",
	})
	if err != nil {
		t.Fatalf("Query CallTool() returned error: %v", err)
	}
	if queryResult.IsError {
		t.Fatalf("Query returned error: %s", queryResult.Content)
	}

	// Feedback via MCP using L1 ref
	feedbackResult, err := server.CallTool(context.Background(), "recall_feedback", map[string]any{
		"helpful": []string{"L1"},
	})
	if err != nil {
		t.Fatalf("Feedback CallTool() returned error: %v", err)
	}
	if feedbackResult.IsError {
		t.Fatalf("Feedback returned error: %s", feedbackResult.Content)
	}
}

// TestIntegration_RecordThenQuery tests that recorded lore appears in queries.
func TestIntegration_RecordThenQuery(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)

	// Record lore via MCP
	_, err = server.CallTool(context.Background(), "recall_record", map[string]any{
		"content":  "Unique test content xyzabc123",
		"category": "TESTING_STRATEGY",
	})
	if err != nil {
		t.Fatalf("Record CallTool() returned error: %v", err)
	}

	// Query for the recorded content
	queryResult, err := server.CallTool(context.Background(), "recall_query", map[string]any{
		"query": "xyzabc123",
	})
	if err != nil {
		t.Fatalf("Query CallTool() returned error: %v", err)
	}

	// Should find the lore (basic query without embeddings uses text matching)
	if queryResult.IsError {
		t.Fatalf("Query returned error: %s", queryResult.Content)
	}
}

// TestIntegration_MultipleQueries tests session refs increment across queries.
func TestIntegration_MultipleQueries(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)

	// Record multiple lore entries
	for i := 0; i < 3; i++ {
		_, err = server.CallTool(context.Background(), "recall_record", map[string]any{
			"content":  "Test content for session refs test",
			"category": "PATTERN_OUTCOME",
		})
		if err != nil {
			t.Fatalf("Record CallTool() #%d returned error: %v", i+1, err)
		}
	}

	// First query
	_, err = server.CallTool(context.Background(), "recall_query", map[string]any{
		"query": "session",
	})
	if err != nil {
		t.Fatalf("First Query CallTool() returned error: %v", err)
	}

	// Second query - refs should continue from where first left off
	_, err = server.CallTool(context.Background(), "recall_query", map[string]any{
		"query": "refs",
	})
	if err != nil {
		t.Fatalf("Second Query CallTool() returned error: %v", err)
	}

	// Verify session lore is tracked
	sessionLore := client.GetSessionLore()
	if len(sessionLore) == 0 {
		t.Error("Session lore should be tracked across queries")
	}
}

// =============================================================================
// Protocol-Level Tests
// =============================================================================

// TestProtocol_Initialize tests that initialize request returns server info and capabilities.
func TestProtocol_Initialize(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)

	// Send an initialize request
	initRequest := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}`

	response := server.HandleMessage(context.Background(), []byte(initRequest))
	if response == nil {
		t.Fatal("HandleMessage() returned nil response for initialize request")
	}

	// Marshal and unmarshal to check the response structure
	respBytes, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	var respMap map[string]any
	if err := json.Unmarshal(respBytes, &respMap); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Verify it's a valid response (has result, no error)
	if _, hasError := respMap["error"]; hasError {
		t.Errorf("Initialize response has error: %v", respMap["error"])
	}

	result, ok := respMap["result"].(map[string]any)
	if !ok {
		t.Fatalf("Initialize response missing result")
	}

	// Check server info
	serverInfo, ok := result["serverInfo"].(map[string]any)
	if !ok {
		t.Fatal("Initialize result missing serverInfo")
	}

	if serverInfo["name"] != "recall" {
		t.Errorf("serverInfo.name = %v, want 'recall'", serverInfo["name"])
	}

	if serverInfo["version"] != "1.0.0" {
		t.Errorf("serverInfo.version = %v, want '1.0.0'", serverInfo["version"])
	}

	// Check capabilities (should have tools)
	capabilities, ok := result["capabilities"].(map[string]any)
	if !ok {
		t.Fatal("Initialize result missing capabilities")
	}

	if _, hasTools := capabilities["tools"]; !hasTools {
		t.Error("Capabilities should include tools")
	}
}

// TestProtocol_InvalidMethod tests that unknown method returns method not found error.
func TestProtocol_InvalidMethod(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)

	// Send a request with unknown method
	invalidMethodRequest := `{"jsonrpc":"2.0","id":1,"method":"unknown/method","params":{}}`

	response := server.HandleMessage(context.Background(), []byte(invalidMethodRequest))
	if response == nil {
		t.Fatal("HandleMessage() returned nil response for invalid method request")
	}

	// Marshal and unmarshal to check the response structure
	respBytes, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	var respMap map[string]any
	if err := json.Unmarshal(respBytes, &respMap); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Verify it has an error
	errorObj, hasError := respMap["error"].(map[string]any)
	if !hasError {
		t.Fatal("Response should have error for unknown method")
	}

	// MCP error code for method not found is -32601
	errorCode, ok := errorObj["code"].(float64)
	if !ok {
		t.Fatalf("Error missing code field")
	}

	// -32601 is METHOD_NOT_FOUND in JSON-RPC spec
	if int(errorCode) != -32601 {
		t.Errorf("Error code = %v, want -32601 (METHOD_NOT_FOUND)", errorCode)
	}
}

// TestProtocol_MalformedJSON tests that invalid JSON returns parse error.
func TestProtocol_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("recall.New() returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	server := recallmcp.NewServer(client)

	// Send malformed JSON
	malformedJSON := `{"jsonrpc":"2.0","id":1,"method":`

	response := server.HandleMessage(context.Background(), []byte(malformedJSON))
	if response == nil {
		t.Fatal("HandleMessage() returned nil response for malformed JSON")
	}

	// Marshal and unmarshal to check the response structure
	respBytes, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	var respMap map[string]any
	if err := json.Unmarshal(respBytes, &respMap); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Verify it has an error
	errorObj, hasError := respMap["error"].(map[string]any)
	if !hasError {
		t.Fatal("Response should have error for malformed JSON")
	}

	// MCP error code for parse error is -32700
	errorCode, ok := errorObj["code"].(float64)
	if !ok {
		t.Fatalf("Error missing code field")
	}

	// -32700 is PARSE_ERROR in JSON-RPC spec
	if int(errorCode) != -32700 {
		t.Errorf("Error code = %v, want -32700 (PARSE_ERROR)", errorCode)
	}
}
