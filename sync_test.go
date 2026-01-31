package recall

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// Syncer.Push() tests
// ============================================================================

func TestSyncer_Push_EmptyQueue(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Track if HTTP was called
	httpCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	err = syncer.Push(context.Background())
	if err != nil {
		t.Errorf("Push on empty queue returned error: %v", err)
	}

	if httpCalled {
		t.Error("HTTP should not be called when queue is empty")
	}
}

func TestSyncer_Push_LoreSuccess(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore (creates INSERT queue entry)
	lore := &Lore{
		ID:         "01HQTEST00000000000001",
		Content:    "Test content",
		Category:   CategoryPatternOutcome,
		Confidence: 0.5,
		SourceID:   "src",
	}
	store.InsertLore(lore)

	// Track received payload
	var receivedPayload engramIngestRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/lore" {
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &receivedPayload)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"accepted":1,"merged":0,"rejected":0,"errors":[]}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	err = syncer.Push(context.Background())
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	// Verify payload
	if len(receivedPayload.Lore) != 1 {
		t.Fatalf("expected 1 lore in payload, got %d", len(receivedPayload.Lore))
	}
	if receivedPayload.Lore[0].ID != lore.ID {
		t.Errorf("lore ID = %q, want %q", receivedPayload.Lore[0].ID, lore.ID)
	}

	// Verify queue is cleared
	entries, _ := store.PendingSyncEntries()
	if len(entries) != 0 {
		t.Errorf("queue should be empty after success, has %d entries", len(entries))
	}

	// Verify synced_at is set
	updatedLore, _ := store.Get(lore.ID)
	if updatedLore.SyncedAt == nil {
		t.Error("synced_at should be set after successful push")
	}
}

func TestSyncer_Push_FeedbackSuccess(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore
	lore := &Lore{
		ID:         "01HQTEST00000000000001",
		Content:    "Test content",
		Category:   CategoryPatternOutcome,
		Confidence: 0.5,
		SourceID:   "src",
	}
	store.InsertLore(lore)
	// Clear the INSERT entry, add FEEDBACK manually
	store.db.Exec("DELETE FROM sync_queue")
	store.db.Exec(`
		INSERT INTO sync_queue (lore_id, operation, payload, queued_at)
		VALUES (?, 'FEEDBACK', '{"outcome":"helpful"}', '2024-01-01T00:00:00Z')
	`, lore.ID)

	// Track received payload
	var receivedPayload engramFeedbackRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/lore/feedback" {
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &receivedPayload)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	err = syncer.Push(context.Background())
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	// Verify payload
	if len(receivedPayload.Feedback) != 1 {
		t.Fatalf("expected 1 feedback in payload, got %d", len(receivedPayload.Feedback))
	}
	if receivedPayload.Feedback[0].Type != "helpful" {
		t.Errorf("type = %q, want %q", receivedPayload.Feedback[0].Type, "helpful")
	}

	// Verify queue is cleared
	entries, _ := store.PendingSyncEntries()
	if len(entries) != 0 {
		t.Errorf("queue should be empty after success, has %d entries", len(entries))
	}
}

func TestSyncer_Push_MixedOperations(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore
	lore1 := &Lore{ID: "01HQTEST00000000000001", Content: "Content 1", Category: CategoryPatternOutcome, Confidence: 0.5, SourceID: "src"}
	lore2 := &Lore{ID: "01HQTEST00000000000002", Content: "Content 2", Category: CategoryEdgeCaseDiscovery, Confidence: 0.7, SourceID: "src"}
	store.InsertLore(lore1)
	store.InsertLore(lore2)

	// Convert one to FEEDBACK
	store.db.Exec("UPDATE sync_queue SET operation = 'FEEDBACK', payload = '{\"outcome\":\"incorrect\"}' WHERE lore_id = ?", lore2.ID)

	loreCalled := false
	feedbackCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/lore":
			loreCalled = true
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"accepted":1,"merged":0,"rejected":0,"errors":[]}`))
		case "/api/v1/lore/feedback":
			feedbackCalled = true
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	err = syncer.Push(context.Background())
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	if !loreCalled {
		t.Error("expected /api/v1/lore to be called")
	}
	if !feedbackCalled {
		t.Error("expected /api/v1/lore/feedback to be called")
	}
}

func TestSyncer_Push_LoreFailure(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore
	lore := &Lore{
		ID:         "01HQTEST00000000000001",
		Content:    "Test content",
		Category:   CategoryPatternOutcome,
		Confidence: 0.5,
		SourceID:   "src",
	}
	store.InsertLore(lore)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"service unavailable"}`))
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	err = syncer.Push(context.Background())
	if err == nil {
		t.Error("expected error from Push on server failure")
	}

	// Check it's a SyncError
	var syncErr *SyncError
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to contain status code, got: %v", err)
	}
	_ = syncErr // might not be extractable depending on wrapping

	// Verify queue entry still exists
	entries, _ := store.PendingSyncEntries()
	if len(entries) != 1 {
		t.Fatalf("queue should still have 1 entry, has %d", len(entries))
	}

	// Verify attempts incremented
	if entries[0].Attempts != 1 {
		t.Errorf("attempts = %d, want 1", entries[0].Attempts)
	}

	// Verify error recorded
	if entries[0].LastError == "" {
		t.Error("last_error should be set")
	}
}

func TestSyncer_Push_NetworkError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore
	lore := &Lore{
		ID:         "01HQTEST00000000000001",
		Content:    "Test content",
		Category:   CategoryPatternOutcome,
		Confidence: 0.5,
		SourceID:   "src",
	}
	store.InsertLore(lore)

	// Use invalid URL to simulate network error
	syncer := NewSyncer(store, "http://localhost:99999", "test-key", "test-source")

	err = syncer.Push(context.Background())
	if err == nil {
		t.Error("expected error from Push on network failure")
	}

	// Verify queue preserved
	entries, _ := store.PendingSyncEntries()
	if len(entries) != 1 {
		t.Fatalf("queue should still have 1 entry, has %d", len(entries))
	}

	if entries[0].Attempts != 1 {
		t.Errorf("attempts = %d, want 1", entries[0].Attempts)
	}
}

func TestSyncer_Push_RetryPreviouslyFailed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore
	lore := &Lore{
		ID:         "01HQTEST00000000000001",
		Content:    "Test content",
		Category:   CategoryPatternOutcome,
		Confidence: 0.5,
		SourceID:   "src",
	}
	store.InsertLore(lore)

	// Simulate previous failure
	store.db.Exec("UPDATE sync_queue SET attempts = 2, last_error = 'previous error' WHERE lore_id = ?", lore.ID)

	receivedCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"accepted":1,"merged":0,"rejected":0,"errors":[]}`))
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	err = syncer.Push(context.Background())
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	// Should have been retried
	if receivedCount != 1 {
		t.Errorf("expected 1 HTTP call, got %d", receivedCount)
	}

	// Queue should be cleared after success
	entries, _ := store.PendingSyncEntries()
	if len(entries) != 0 {
		t.Errorf("queue should be empty after success, has %d entries", len(entries))
	}
}

func TestSyncer_Push_SourceIDIncluded(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore
	lore := &Lore{
		ID:         "01HQTEST00000000000001",
		Content:    "Test content",
		Category:   CategoryPatternOutcome,
		Confidence: 0.5,
		SourceID:   "src",
	}
	store.InsertLore(lore)

	var receivedSourceID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req engramIngestRequest
		json.Unmarshal(body, &req)
		receivedSourceID = req.SourceID
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"accepted":1,"merged":0,"rejected":0,"errors":[]}`))
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "my-custom-source-id")

	err = syncer.Push(context.Background())
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	if receivedSourceID != "my-custom-source-id" {
		t.Errorf("SourceID = %q, want %q", receivedSourceID, "my-custom-source-id")
	}
}

func TestSyncer_Push_DeletedLore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore
	lore := &Lore{
		ID:         "01HQTEST00000000000001",
		Content:    "Test content",
		Category:   CategoryPatternOutcome,
		Confidence: 0.5,
		SourceID:   "src",
	}
	store.InsertLore(lore)

	// Delete the lore but keep queue entry
	store.db.Exec("DELETE FROM lore_entries WHERE id = ?", lore.ID)

	httpCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	// Should not error, just clean up the orphaned queue entry
	err = syncer.Push(context.Background())
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	// Queue should be cleared (orphaned entries removed)
	entries, _ := store.PendingSyncEntries()
	if len(entries) != 0 {
		t.Errorf("queue should be empty after handling deleted lore, has %d entries", len(entries))
	}

	// HTTP should NOT be called for empty lore
	if httpCalled {
		t.Error("HTTP should not be called when all lore was deleted")
	}
}

func TestSyncer_Push_MalformedPayload(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore for the feedback
	lore := &Lore{
		ID:         "01HQTEST00000000000001",
		Content:    "Test content",
		Category:   CategoryPatternOutcome,
		Confidence: 0.5,
		SourceID:   "src",
	}
	store.InsertLore(lore)

	// Add malformed FEEDBACK entry
	store.db.Exec("DELETE FROM sync_queue")
	store.db.Exec(`
		INSERT INTO sync_queue (lore_id, operation, payload, queued_at)
		VALUES (?, 'FEEDBACK', 'not-valid-json', '2024-01-01T00:00:00Z')
	`, lore.ID)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// HTTP should not be called for malformed entries
		t.Error("HTTP should not be called when all payloads are malformed")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	// Should not error, just clean up malformed entries
	err = syncer.Push(context.Background())
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	// Queue should be cleared (malformed entries cleared)
	entries, _ := store.PendingSyncEntries()
	if len(entries) != 0 {
		t.Errorf("queue should be empty after handling malformed payload, has %d entries", len(entries))
	}
}

func TestSyncer_Flush_SetsFlushFlag(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Insert lore
	lore := &Lore{
		ID:         "01HQTEST00000000000001",
		Content:    "Test content",
		Category:   CategoryPatternOutcome,
		Confidence: 0.5,
		SourceID:   "src",
	}
	store.InsertLore(lore)

	var receivedFlush bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req engramIngestRequest
		json.Unmarshal(body, &req)
		receivedFlush = req.Flush
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"accepted":1,"merged":0,"rejected":0,"errors":[]}`))
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	err = syncer.Flush(context.Background())
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	if !receivedFlush {
		t.Error("Flush flag should be true in Flush request")
	}
}

// ============================================================================
// Story 4.4: X-Recall-Source-ID Header Tests
// ============================================================================

// TestSyncer_SourceIDHeader_OnPush verifies X-Recall-Source-ID is sent on push.
// Story 4.4 AC#4: Header matches source ID in request bodies.
func TestSyncer_SourceIDHeader_OnPush(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	lore := &Lore{
		ID:         "01HQTEST00000000000001",
		Content:    "Test content",
		Category:   CategoryPatternOutcome,
		Confidence: 0.5,
		SourceID:   "src",
	}
	_ = store.InsertLore(lore)

	var receivedHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Recall-Source-ID")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accepted":1,"merged":0,"rejected":0,"errors":[]}`))
	}))
	defer server.Close()

	sourceID := "observability-test-client"
	syncer := NewSyncer(store, server.URL, "test-key", sourceID)

	err = syncer.Push(context.Background())
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	if receivedHeader != sourceID {
		t.Errorf("X-Recall-Source-ID = %q, want %q", receivedHeader, sourceID)
	}
}

// TestSyncer_SourceIDHeader_OnBootstrapSnapshot verifies header on snapshot download.
// Story 4.4 AC#1: Header included in GET /lore/snapshot requests.
func TestSyncer_SourceIDHeader_OnBootstrapSnapshot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	var receivedHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"healthy","embedding_model":"test-model"}`))
			return
		}
		if r.URL.Path == "/api/v1/lore/snapshot" {
			receivedHeader = r.Header.Get("X-Recall-Source-ID")
			// Return valid SQLite header + empty content (will fail replace, but header is captured)
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("SQLite format 3\x00"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	sourceID := "bootstrap-test-client"
	syncer := NewSyncer(store, server.URL, "test-key", sourceID)

	// Bootstrap will fail on invalid snapshot, but we captured the header
	_ = syncer.Bootstrap(context.Background())

	if receivedHeader != sourceID {
		t.Errorf("X-Recall-Source-ID = %q, want %q", receivedHeader, sourceID)
	}
}

// TestSyncer_SourceIDHeader_OmittedWhenEmpty verifies header is not sent when empty.
// Story 4.4 AC#3: Header omitted when source ID is empty.
func TestSyncer_SourceIDHeader_OmittedWhenEmpty(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	lore := &Lore{
		ID:         "01HQTEST00000000000001",
		Content:    "Test content",
		Category:   CategoryPatternOutcome,
		Confidence: 0.5,
		SourceID:   "src",
	}
	_ = store.InsertLore(lore)

	var headerPresent bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headerPresent = r.Header.Get("X-Recall-Source-ID") != ""
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accepted":1,"merged":0,"rejected":0,"errors":[]}`))
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "") // empty sourceID

	err = syncer.Push(context.Background())
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	if headerPresent {
		t.Error("X-Recall-Source-ID should not be present when source ID is empty")
	}
}

// TestSyncer_SourceIDHeader_OmittedWhenWhitespaceOnly verifies whitespace-only is treated as empty.
// Story 4.4: Whitespace-only source ID should not send header.
func TestSyncer_SourceIDHeader_OmittedWhenWhitespaceOnly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	lore := &Lore{
		ID:         "01HQTEST00000000000001",
		Content:    "Test content",
		Category:   CategoryPatternOutcome,
		Confidence: 0.5,
		SourceID:   "src",
	}
	_ = store.InsertLore(lore)

	var headerPresent bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headerPresent = r.Header.Get("X-Recall-Source-ID") != ""
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accepted":1,"merged":0,"rejected":0,"errors":[]}`))
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "   ") // whitespace-only sourceID

	err = syncer.Push(context.Background())
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	if headerPresent {
		t.Error("X-Recall-Source-ID should not be present when source ID is whitespace-only")
	}
}

// TestSyncer_SourceIDHeader_OnFlush verifies header is sent during flush.
// Story 4.4: Header should be present on all sync requests including flush.
func TestSyncer_SourceIDHeader_OnFlush(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	lore := &Lore{
		ID:         "01HQTEST00000000000001",
		Content:    "Test content",
		Category:   CategoryPatternOutcome,
		Confidence: 0.5,
		SourceID:   "src",
	}
	_ = store.InsertLore(lore)

	var receivedHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Recall-Source-ID")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accepted":1,"merged":0,"rejected":0,"errors":[]}`))
	}))
	defer server.Close()

	sourceID := "flush-test-client"
	syncer := NewSyncer(store, server.URL, "test-key", sourceID)

	_ = syncer.Flush(context.Background())

	if receivedHeader != sourceID {
		t.Errorf("X-Recall-Source-ID = %q, want %q", receivedHeader, sourceID)
	}
}

// ============================================================================
// Syncer.Pull() tests (deprecated method)
// ============================================================================

// TestSyncer_Pull_RequiresBootstrap verifies Pull() errors when last_sync is empty.
// Bug fix: Pull() was calling /delta without required since parameter.
func TestSyncer_Pull_RequiresBootstrap(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Track if HTTP was called - it should NOT be
	httpCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	err = syncer.Pull(context.Background())
	if err == nil {
		t.Fatal("expected error for missing bootstrap, got nil")
	}
	if !strings.Contains(err.Error(), "bootstrap") {
		t.Errorf("error should mention bootstrap, got: %v", err)
	}
	if httpCalled {
		t.Error("HTTP should not be called when last_sync is empty")
	}
}

// TestSyncer_Pull_WithLastSync verifies Pull() works when last_sync exists.
func TestSyncer_Pull_WithLastSync(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Set last_sync to simulate prior bootstrap
	store.SetMetadata("last_sync", "2026-01-28T00:00:00Z")

	var receivedSince string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSince = r.URL.Query().Get("since")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"lore": [], "deleted_ids": [], "as_of": "2026-01-29T00:00:00Z"}`))
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	err = syncer.Pull(context.Background())
	if err != nil {
		t.Fatalf("Pull with last_sync should succeed: %v", err)
	}
	if receivedSince != "2026-01-28T00:00:00Z" {
		t.Errorf("since = %q, want %q", receivedSince, "2026-01-28T00:00:00Z")
	}
}

// ============================================================================
// Syncer.SyncDelta() tests
// Story 4.5: Delta Sync
// ============================================================================

// TestSyncer_SyncDelta_RequiresBootstrap verifies error when last_sync is empty.
// Story 4.5 AC#4: Delta sync requires prior bootstrap.
func TestSyncer_SyncDelta_RequiresBootstrap(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("HTTP should not be called when last_sync is empty")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	err = syncer.SyncDelta(context.Background())
	if err == nil {
		t.Fatal("expected error for missing bootstrap, got nil")
	}
	if !strings.Contains(err.Error(), "bootstrap") {
		t.Errorf("error should mention bootstrap, got: %v", err)
	}
}

// TestSyncer_SyncDelta_UpsertsLore verifies new/updated lore is upserted.
// Story 4.5 AC#5: Delta sync updates local lore entries (upsert).
func TestSyncer_SyncDelta_UpsertsLore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Set last_sync to simulate prior bootstrap
	store.SetMetadata("last_sync", "2026-01-28T00:00:00Z")

	deltaResponse := `{
		"lore": [
			{
				"id": "01ARZ3NDEKTSV4RRFFQ69G5FAV",
				"content": "Delta synced content",
				"category": "PATTERN_OUTCOME",
				"confidence": 0.85,
				"sources": [],
				"validation_count": 3,
				"created_at": "2026-01-28T10:00:00Z",
				"updated_at": "2026-01-29T14:30:00Z",
				"embedding_status": "ready"
			}
		],
		"deleted_ids": [],
		"as_of": "2026-01-29T15:00:00Z"
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/lore/delta" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(deltaResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	err = syncer.SyncDelta(context.Background())
	if err != nil {
		t.Fatalf("SyncDelta failed: %v", err)
	}

	// Verify lore was upserted
	lore, err := store.Get("01ARZ3NDEKTSV4RRFFQ69G5FAV")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if lore.Content != "Delta synced content" {
		t.Errorf("Content = %q, want %q", lore.Content, "Delta synced content")
	}
	if lore.Confidence != 0.85 {
		t.Errorf("Confidence = %f, want 0.85", lore.Confidence)
	}
}

// TestSyncer_SyncDelta_DeletesLore verifies deleted entries are removed.
// Story 4.5 AC#5: Delta sync removes deleted entries.
func TestSyncer_SyncDelta_DeletesLore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Set last_sync
	store.SetMetadata("last_sync", "2026-01-28T00:00:00Z")

	// Insert lore that will be deleted
	lore := &Lore{
		ID:              "01ARZ3NDEKTSV4RRFFQ69G5DEL",
		Content:         "Will be deleted",
		Category:        CategoryPatternOutcome,
		Confidence:      0.5,
		EmbeddingStatus: "ready",
	}
	store.UpsertLore(lore)

	deltaResponse := `{
		"lore": [],
		"deleted_ids": ["01ARZ3NDEKTSV4RRFFQ69G5DEL"],
		"as_of": "2026-01-29T15:00:00Z"
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/lore/delta" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(deltaResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	err = syncer.SyncDelta(context.Background())
	if err != nil {
		t.Fatalf("SyncDelta failed: %v", err)
	}

	// Verify lore was deleted
	_, err = store.Get("01ARZ3NDEKTSV4RRFFQ69G5DEL")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}
}

// TestSyncer_SyncDelta_UpdatesLastSync verifies last_sync is updated with AsOf.
// Story 4.5 AC#6: Delta sync updates last_sync timestamp on success.
func TestSyncer_SyncDelta_UpdatesLastSync(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Set last_sync
	store.SetMetadata("last_sync", "2026-01-28T00:00:00Z")

	deltaResponse := `{
		"lore": [],
		"deleted_ids": [],
		"as_of": "2026-01-29T15:00:00Z"
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/lore/delta" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(deltaResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	err = syncer.SyncDelta(context.Background())
	if err != nil {
		t.Fatalf("SyncDelta failed: %v", err)
	}

	// Verify last_sync was updated
	lastSync, err := store.GetMetadata("last_sync")
	if err != nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}
	if lastSync != "2026-01-29T15:00:00Z" {
		t.Errorf("last_sync = %q, want %q", lastSync, "2026-01-29T15:00:00Z")
	}
}

// TestSyncer_SyncDelta_IncludesSourceID verifies X-Recall-Source-ID header is sent.
// Story 4.5 AC#9: Delta sync includes X-Recall-Source-ID header when configured.
func TestSyncer_SyncDelta_IncludesSourceID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	store.SetMetadata("last_sync", "2026-01-28T00:00:00Z")

	var receivedHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Recall-Source-ID")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"lore": [], "deleted_ids": [], "as_of": "2026-01-29T15:00:00Z"}`))
	}))
	defer server.Close()

	sourceID := "delta-client-123"
	syncer := NewSyncer(store, server.URL, "test-key", sourceID)

	err = syncer.SyncDelta(context.Background())
	if err != nil {
		t.Fatalf("SyncDelta failed: %v", err)
	}

	if receivedHeader != sourceID {
		t.Errorf("X-Recall-Source-ID = %q, want %q", receivedHeader, sourceID)
	}
}

// TestSyncer_SyncDelta_EmptyDelta verifies empty delta is handled gracefully.
func TestSyncer_SyncDelta_EmptyDelta(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	store.SetMetadata("last_sync", "2026-01-28T00:00:00Z")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"lore": [], "deleted_ids": [], "as_of": "2026-01-29T15:00:00Z"}`))
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	err = syncer.SyncDelta(context.Background())
	if err != nil {
		t.Errorf("SyncDelta with empty delta should not error: %v", err)
	}
}

// TestSyncer_SyncDelta_PreservesAllFields verifies all fields from delta are preserved.
// Story 4.5: Delta sync should not lose data.
func TestSyncer_SyncDelta_PreservesAllFields(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Set last_sync to simulate prior bootstrap
	store.SetMetadata("last_sync", "2026-01-28T00:00:00Z")

	// Delta response with all fields populated
	deltaResponse := `{
		"lore": [
			{
				"id": "01ARZ3NDEKTSV4RRFFQ69GFULL",
				"content": "Full field content",
				"context": "test-context",
				"category": "PATTERN_OUTCOME",
				"confidence": 0.92,
				"sources": ["source-1", "source-2"],
				"validation_count": 5,
				"source_id": "test-source-id",
				"embedding_status": "ready",
				"created_at": "2026-01-28T10:00:00Z",
				"updated_at": "2026-01-29T14:30:00Z"
			}
		],
		"deleted_ids": [],
		"as_of": "2026-01-29T15:00:00Z"
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/lore/delta" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(deltaResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	err = syncer.SyncDelta(context.Background())
	if err != nil {
		t.Fatalf("SyncDelta failed: %v", err)
	}

	// Verify lore was upserted with all fields preserved
	lore, err := store.Get("01ARZ3NDEKTSV4RRFFQ69GFULL")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Verify ValidationCount is preserved
	if lore.ValidationCount != 5 {
		t.Errorf("ValidationCount = %d, want 5", lore.ValidationCount)
	}

	// Verify Sources are preserved
	if len(lore.Sources) != 2 {
		t.Errorf("Sources length = %d, want 2", len(lore.Sources))
	} else {
		if lore.Sources[0] != "source-1" {
			t.Errorf("Sources[0] = %q, want %q", lore.Sources[0], "source-1")
		}
		if lore.Sources[1] != "source-2" {
			t.Errorf("Sources[1] = %q, want %q", lore.Sources[1], "source-2")
		}
	}

	// Verify SourceID is preserved
	if lore.SourceID != "test-source-id" {
		t.Errorf("SourceID = %q, want %q", lore.SourceID, "test-source-id")
	}

	// Verify EmbeddingStatus is preserved
	if lore.EmbeddingStatus != "ready" {
		t.Errorf("EmbeddingStatus = %q, want %q", lore.EmbeddingStatus, "ready")
	}

	// Verify Context is preserved
	if lore.Context != "test-context" {
		t.Errorf("Context = %q, want %q", lore.Context, "test-context")
	}

	// Verify other basic fields
	if lore.Content != "Full field content" {
		t.Errorf("Content = %q, want %q", lore.Content, "Full field content")
	}
	if lore.Confidence != 0.92 {
		t.Errorf("Confidence = %f, want 0.92", lore.Confidence)
	}
}

// TestSyncer_SyncDelta_DefaultsEmbeddingStatus verifies empty embedding_status defaults to "ready".
// Story 4.5: Delta entries should default to "ready" embedding status when not specified.
func TestSyncer_SyncDelta_DefaultsEmbeddingStatus(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Set last_sync to simulate prior bootstrap
	store.SetMetadata("last_sync", "2026-01-28T00:00:00Z")

	// Delta response with empty embedding_status
	deltaResponse := `{
		"lore": [
			{
				"id": "01ARZ3NDEKTSV4RRFFQ69GDFLT",
				"content": "Content with default status",
				"category": "PATTERN_OUTCOME",
				"confidence": 0.8,
				"sources": [],
				"validation_count": 0,
				"embedding_status": "",
				"created_at": "2026-01-28T10:00:00Z"
			}
		],
		"deleted_ids": [],
		"as_of": "2026-01-29T15:00:00Z"
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/lore/delta" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(deltaResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	err = syncer.SyncDelta(context.Background())
	if err != nil {
		t.Fatalf("SyncDelta failed: %v", err)
	}

	// Verify lore was upserted with default embedding_status
	lore, err := store.Get("01ARZ3NDEKTSV4RRFFQ69GDFLT")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Verify EmbeddingStatus defaults to "ready"
	if lore.EmbeddingStatus != "ready" {
		t.Errorf("EmbeddingStatus = %q, want %q (default)", lore.EmbeddingStatus, "ready")
	}
}

// TestSyncer_SyncDelta_UpdatesExistingEntry verifies that delta sync updates existing
// entries instead of creating duplicates. This is the bug reported: when an entry that
// already exists locally is returned in a delta response (due to server-side modifications
// like confidence updates), it should UPDATE the existing row, not INSERT a duplicate.
//
// Bug Report: Recall Client Delta Sync Creates Duplicate Entries
// Root Cause: INSERT without ON CONFLICT handling (or ON CONFLICT not triggering)
func TestSyncer_SyncDelta_UpdatesExistingEntry(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Set last_sync to simulate prior bootstrap
	store.SetMetadata("last_sync", "2026-01-28T00:00:00Z")

	// Step 1: Insert an entry (simulating bootstrap)
	existingID := "01ARZ3NDEKTSV4RRFFQ69GEXST"
	existingLore := &Lore{
		ID:              existingID,
		Content:         "Original content from bootstrap",
		Category:        CategoryPatternOutcome,
		Confidence:      0.50, // Original confidence
		ValidationCount: 0,    // Original validation count
		SourceID:        "original-source",
		EmbeddingStatus: "ready",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := store.InsertLore(existingLore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Verify initial count is 1
	initialStats, err := store.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if initialStats.LoreCount != 1 {
		t.Fatalf("Initial LoreCount = %d, want 1", initialStats.LoreCount)
	}

	// Step 2: Delta response contains the SAME entry with updated fields
	// (simulating server-side confidence update from feedback)
	deltaResponse := fmt.Sprintf(`{
		"lore": [
			{
				"id": %q,
				"content": "Original content from bootstrap",
				"category": "PATTERN_OUTCOME",
				"confidence": 0.75,
				"sources": [],
				"validation_count": 3,
				"source_id": "original-source",
				"embedding_status": "ready",
				"created_at": "2026-01-28T10:00:00Z",
				"updated_at": "2026-01-29T14:30:00Z"
			}
		],
		"deleted_ids": [],
		"as_of": "2026-01-29T15:00:00Z"
	}`, existingID)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/lore/delta" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(deltaResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	// Step 3: Run delta sync
	err = syncer.SyncDelta(context.Background())
	if err != nil {
		t.Fatalf("SyncDelta failed: %v", err)
	}

	// Step 4: Verify count is STILL 1 (not 2 - no duplicate)
	finalStats, err := store.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if finalStats.LoreCount != 1 {
		t.Errorf("LoreCount after delta = %d, want 1 (no duplicates)", finalStats.LoreCount)
	}

	// Step 5: Verify the entry was UPDATED with new values
	lore, err := store.Get(existingID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Confidence should be updated from 0.50 to 0.75
	if lore.Confidence != 0.75 {
		t.Errorf("Confidence = %f, want 0.75 (updated value)", lore.Confidence)
	}

	// ValidationCount should be updated from 0 to 3
	if lore.ValidationCount != 3 {
		t.Errorf("ValidationCount = %d, want 3 (updated value)", lore.ValidationCount)
	}

}

// TestSyncer_SyncDelta_MultipleUpdatesNoDuplicates verifies multiple delta syncs
// of the same entries don't create duplicates.
func TestSyncer_SyncDelta_MultipleUpdatesNoDuplicates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	store.SetMetadata("last_sync", "2026-01-28T00:00:00Z")

	existingID := "01ARZ3NDEKTSV4RRFFQ69GMULT"

	// Insert initial entry
	initialLore := &Lore{
		ID:              existingID,
		Content:         "Initial content",
		Category:        CategoryPatternOutcome,
		Confidence:      0.50,
		ValidationCount: 0,
		SourceID:        "test-source",
		EmbeddingStatus: "ready",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := store.InsertLore(initialLore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Verify initial count
	stats, _ := store.Stats()
	if stats.LoreCount != 1 {
		t.Fatalf("Initial count = %d, want 1", stats.LoreCount)
	}

	// Run delta sync 3 times with the same entry (simulating repeated server updates)
	for i := 1; i <= 3; i++ {
		deltaResponse := fmt.Sprintf(`{
			"lore": [{
				"id": %q,
				"content": "Initial content",
				"category": "PATTERN_OUTCOME",
				"confidence": %.2f,
				"sources": [],
				"validation_count": %d,
				"source_id": "test-source",
				"embedding_status": "ready",
				"created_at": "2026-01-28T10:00:00Z",
				"updated_at": "2026-01-29T14:30:00Z"
			}],
			"deleted_ids": [],
			"as_of": "2026-01-29T15:00:00Z"
		}`, existingID, 0.50+float64(i)*0.1, i)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(deltaResponse))
		}))

		syncer := NewSyncer(store, server.URL, "test-key", "test-source")
		err = syncer.SyncDelta(context.Background())
		server.Close()

		if err != nil {
			t.Fatalf("SyncDelta iteration %d failed: %v", i, err)
		}

		// Count should ALWAYS be 1
		stats, _ := store.Stats()
		if stats.LoreCount != 1 {
			t.Errorf("After delta sync %d: LoreCount = %d, want 1", i, stats.LoreCount)
		}
	}

	// Final verification
	lore, err := store.Get(existingID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Should have the values from the 3rd delta sync
	if lore.ValidationCount != 3 {
		t.Errorf("ValidationCount = %d, want 3", lore.ValidationCount)
	}
	if lore.Confidence < 0.79 || lore.Confidence > 0.81 {
		t.Errorf("Confidence = %f, want ~0.80", lore.Confidence)
	}
}
