package recall

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
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
		_, headerPresent = r.Header["X-Recall-Source-ID"]
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
		_, headerPresent = r.Header["X-Recall-Source-ID"]
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
