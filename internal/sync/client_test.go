package sync

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hyperengineering/recall"
)

func TestHTTPClient_HealthCheck_Success(t *testing.T) {
	expected := &HealthResponse{
		Status:         "healthy",
		Version:        "1.0.0",
		EmbeddingModel: "text-embedding-3-small",
		LoreCount:      42,
		LastSnapshot:   "2024-01-15T10:00:00Z",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(expected)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "test-api-key")
	result, err := client.HealthCheck(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.EmbeddingModel != expected.EmbeddingModel {
		t.Errorf("EmbeddingModel = %q, want %q", result.EmbeddingModel, expected.EmbeddingModel)
	}
	if result.Status != expected.Status {
		t.Errorf("Status = %q, want %q", result.Status, expected.Status)
	}
}

func TestHTTPClient_HealthCheck_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": "invalid api key"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "bad-key")
	_, err := client.HealthCheck(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var syncErr *recall.SyncError
	if !errors.As(err, &syncErr) {
		t.Fatalf("expected SyncError, got %T", err)
	}
	if syncErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want %d", syncErr.StatusCode, http.StatusUnauthorized)
	}
	if syncErr.Operation != "health_check" {
		t.Errorf("Operation = %q, want %q", syncErr.Operation, "health_check")
	}
}

func TestHTTPClient_HealthCheck_NetworkError(t *testing.T) {
	client := NewHTTPClient("http://localhost:1", "test-api-key")
	_, err := client.HealthCheck(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var syncErr *recall.SyncError
	if !errors.As(err, &syncErr) {
		t.Fatalf("expected SyncError, got %T", err)
	}
	if syncErr.Operation != "health_check" {
		t.Errorf("Operation = %q, want %q", syncErr.Operation, "health_check")
	}
}

func TestHTTPClient_DownloadSnapshot_Success(t *testing.T) {
	snapshotData := []byte("binary snapshot data here")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/lore/snapshot" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(snapshotData)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "test-api-key")
	reader, err := client.DownloadSnapshot(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read snapshot: %v", err)
	}
	if string(data) != string(snapshotData) {
		t.Errorf("snapshot data = %q, want %q", string(data), string(snapshotData))
	}
}

func TestHTTPClient_DownloadSnapshot_ServiceUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("service temporarily unavailable"))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "test-api-key")
	_, err := client.DownloadSnapshot(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var syncErr *recall.SyncError
	if !errors.As(err, &syncErr) {
		t.Fatalf("expected SyncError, got %T", err)
	}
	if syncErr.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("StatusCode = %d, want %d", syncErr.StatusCode, http.StatusServiceUnavailable)
	}
}

func TestHTTPClient_PushLore_Success(t *testing.T) {
	expected := &PushLoreResponse{
		Accepted: 5,
		Merged:   2,
		Rejected: 0,
		Errors:   nil,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/lore" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(expected)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "test-api-key")
	req := &PushLoreRequest{
		SourceID: "test-source",
		Lore: []LorePayload{
			{ID: "1", Content: "test", Category: "preference", Confidence: 0.9, CreatedAt: "2024-01-15T10:00:00Z"},
		},
	}
	result, err := client.PushLore(context.Background(), req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Accepted != expected.Accepted {
		t.Errorf("Accepted = %d, want %d", result.Accepted, expected.Accepted)
	}
}

func TestHTTPClient_PushLore_ValidationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error": "validation failed"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "test-api-key")
	req := &PushLoreRequest{SourceID: "test", Lore: []LorePayload{}}
	_, err := client.PushLore(context.Background(), req)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var syncErr *recall.SyncError
	if !errors.As(err, &syncErr) {
		t.Fatalf("expected SyncError, got %T", err)
	}
	if syncErr.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("StatusCode = %d, want %d", syncErr.StatusCode, http.StatusUnprocessableEntity)
	}
}

func TestHTTPClient_PushFeedback_Success(t *testing.T) {
	expected := &PushFeedbackResponse{
		Updates: []FeedbackUpdate{
			{LoreID: "1", PreviousConfidence: 0.8, CurrentConfidence: 0.9, ValidationCount: 5},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/lore/feedback" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(expected)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "test-api-key")
	req := &PushFeedbackRequest{
		SourceID: "test-source",
		Feedback: []FeedbackPayload{
			{LoreID: "1", Type: "helpful"},
		},
	}
	result, err := client.PushFeedback(context.Background(), req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Updates) != 1 {
		t.Errorf("Updates count = %d, want 1", len(result.Updates))
	}
	if result.Updates[0].LoreID != "1" {
		t.Errorf("Update LoreID = %q, want %q", result.Updates[0].LoreID, "1")
	}
}

func TestHTTPClient_PushFeedback_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": "lore not found"}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "test-api-key")
	req := &PushFeedbackRequest{SourceID: "test", Feedback: []FeedbackPayload{{LoreID: "missing", Type: "helpful"}}}
	_, err := client.PushFeedback(context.Background(), req)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var syncErr *recall.SyncError
	if !errors.As(err, &syncErr) {
		t.Fatalf("expected SyncError, got %T", err)
	}
	if syncErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", syncErr.StatusCode, http.StatusNotFound)
	}
}

func TestHTTPClient_AuthorizationHeader(t *testing.T) {
	apiKey := "secret-test-key-12345"
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&HealthResponse{Status: "ok"})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, apiKey)
	_, _ = client.HealthCheck(context.Background())

	expectedAuth := "Bearer " + apiKey
	if receivedAuth != expectedAuth {
		t.Errorf("Authorization = %q, want %q", receivedAuth, expectedAuth)
	}
}

func TestHTTPClient_UserAgentHeader(t *testing.T) {
	var receivedUA string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&HealthResponse{Status: "ok"})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "test-key")
	_, _ = client.HealthCheck(context.Background())

	if receivedUA != "recall-client/1.0" {
		t.Errorf("User-Agent = %q, want %q", receivedUA, "recall-client/1.0")
	}
}

func TestHTTPClient_ContentTypeHeader(t *testing.T) {
	var receivedCT string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCT = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&PushLoreResponse{Accepted: 1})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "test-key")
	req := &PushLoreRequest{SourceID: "test", Lore: []LorePayload{{ID: "1", Content: "test", Category: "preference", Confidence: 0.9, CreatedAt: "2024-01-15T10:00:00Z"}}}
	_, _ = client.PushLore(context.Background(), req)

	if receivedCT != "application/json" {
		t.Errorf("Content-Type = %q, want %q", receivedCT, "application/json")
	}
}

func TestHTTPClient_ErrorDoesNotContainAPIKey(t *testing.T) {
	apiKey := "super-secret-api-key-should-not-leak"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, apiKey)
	_, err := client.HealthCheck(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errStr := err.Error()
	if strings.Contains(errStr, apiKey) {
		t.Errorf("error message contains API key: %s", errStr)
	}
}

func TestHTTPClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "test-key")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.HealthCheck(ctx)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var syncErr *recall.SyncError
	if !errors.As(err, &syncErr) {
		t.Fatalf("expected SyncError, got %T", err)
	}
}

func TestHTTPClient_ErrorBodyTruncation(t *testing.T) {
	// Create a large error body (> 200 chars)
	largeBody := strings.Repeat("x", 500)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(largeBody))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "test-key")
	_, err := client.HealthCheck(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var syncErr *recall.SyncError
	if !errors.As(err, &syncErr) {
		t.Fatalf("expected SyncError, got %T", err)
	}

	// The underlying error message should be truncated
	errStr := syncErr.Err.Error()
	// Should contain "..." indicating truncation and be reasonably short
	if len(errStr) > 250 { // Allow some overhead for "HTTP 400: " prefix
		t.Errorf("error message too long (%d chars), expected truncation", len(errStr))
	}
}

func TestHTTPClient_WithHTTPClient(t *testing.T) {
	customTimeout := 60 * time.Second
	customClient := &http.Client{
		Timeout: customTimeout,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&HealthResponse{Status: "ok"})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "test-key").WithHTTPClient(customClient)

	// Verify the custom client is used by checking it works
	result, err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q, want %q", result.Status, "ok")
	}

	// Verify the custom client was actually set
	if client.httpClient != customClient {
		t.Error("WithHTTPClient did not set the custom client")
	}
	if client.httpClient.Timeout != customTimeout {
		t.Errorf("Timeout = %v, want %v", client.httpClient.Timeout, customTimeout)
	}
}
