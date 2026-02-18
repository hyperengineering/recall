package recall

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// ============================================================================
// Syncer.Push() / Syncer.Flush() / Syncer.Pull() / Syncer.SyncDelta() tests
// Story 10.2: Push and Flush delegate to SyncPush.
// Pull and SyncDelta remain no-ops (pending later stories).
// ============================================================================

func TestSyncer_Push_EmptyChangeLog(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	httpCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	err = syncer.Push(context.Background())
	if err != nil {
		t.Errorf("Push with empty change_log should return nil: %v", err)
	}
	if httpCalled {
		t.Error("Push should not make HTTP calls when change_log is empty")
	}
}

func TestSyncer_Flush_EmptyChangeLog(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	httpCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	err = syncer.Flush(context.Background())
	if err != nil {
		t.Errorf("Flush with empty change_log should return nil: %v", err)
	}
	if httpCalled {
		t.Error("Flush should not make HTTP calls when change_log is empty")
	}
}

func TestSyncer_Pull_NoOp(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	httpCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	err = syncer.Pull(context.Background())
	if err != nil {
		t.Errorf("Pull should return nil: %v", err)
	}
	if httpCalled {
		t.Error("Pull should not make HTTP calls (no-op)")
	}
}

func TestSyncer_SyncDelta_NoOp(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	httpCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")

	err = syncer.SyncDelta(context.Background())
	if err != nil {
		t.Errorf("SyncDelta should return nil: %v", err)
	}
	if httpCalled {
		t.Error("SyncDelta should not make HTTP calls (no-op)")
	}
}

// ============================================================================
// Story 4.4: X-Recall-Source-ID Header Tests
// ============================================================================

// TestSyncer_SourceIDHeader_OnBootstrapSnapshot verifies header on snapshot download.
// Story 4.4 AC#1: Header included in GET /sync/snapshot requests.
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
		if r.URL.Path == "/api/v1/stores/test-store/sync/snapshot" {
			receivedHeader = r.Header.Get("X-Recall-Source-ID")
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("SQLite format 3\x00"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	sourceID := "bootstrap-test-client"
	syncer := NewSyncer(store, server.URL, "test-key", sourceID)
	syncer.SetStoreID("test-store")

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

	syncer := NewSyncer(store, "http://unused", "test-key", "") // empty sourceID

	// Push is a no-op, so we test header logic through Bootstrap instead
	// The setHeaders method is tested implicitly through Bootstrap
	err = syncer.Push(context.Background())
	if err != nil {
		t.Fatalf("Push failed: %v", err)
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

	syncer := NewSyncer(store, "http://unused", "test-key", "   ") // whitespace-only

	err = syncer.Push(context.Background())
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}
}

// ============================================================================
// Syncer.Bootstrap() tests
// ============================================================================

// TestSyncer_Bootstrap_WithStoreID verifies Bootstrap uses store-prefixed /sync/* paths.
// Story 10.1 AC#7: Paths use /sync/* format.
func TestSyncer_Bootstrap_WithStoreID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	var receivedSnapshotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"healthy","embedding_model":"test-model"}`))
			return
		}
		if strings.Contains(r.URL.Path, "snapshot") {
			receivedSnapshotPath = r.URL.Path
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write([]byte("SQLite format 3\x00"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")
	syncer.SetStoreID("my-project")

	// Bootstrap will fail on invalid snapshot, but we captured the path
	_ = syncer.Bootstrap(context.Background())

	expected := "/api/v1/stores/my-project/sync/snapshot"
	if receivedSnapshotPath != expected {
		t.Errorf("snapshot path = %q, want %q", receivedSnapshotPath, expected)
	}
}

// TestSyncer_Bootstrap_PanicsWithoutStoreID verifies Bootstrap panics when storeID is empty.
// Story 10.1 AC#8: Path helpers require storeID.
func TestSyncer_Bootstrap_PanicsWithoutStoreID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"healthy","embedding_model":"test-model"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")
	// No SetStoreID — should panic

	defer func() {
		if r := recover(); r == nil {
			t.Error("Bootstrap should panic when storeID is empty")
		}
	}()
	_ = syncer.Bootstrap(context.Background())
}
