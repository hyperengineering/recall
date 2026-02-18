package recall

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// Story 10.6: Bootstrap Test Suite (AC #3)
// Tests for recall.Syncer.Bootstrap with real httptest.Server
// ============================================================================

// newBootstrapTestServer creates a health + snapshot server for bootstrap tests.
func newBootstrapTestServer(t *testing.T, healthResp *engramHealthResponse, snapshotHandler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(healthResp)
			return
		}
		if strings.Contains(r.URL.Path, "/sync/snapshot") {
			snapshotHandler(w, r)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
}

// newValidSnapshotDB creates a minimal valid SQLite database for bootstrap tests.
func newValidSnapshotDB(t *testing.T) []byte {
	t.Helper()
	tmpPath := t.TempDir() + "/snapshot.db"
	db, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		t.Fatalf("open snapshot db: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS lore_entries (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			context TEXT,
			category TEXT NOT NULL,
			confidence REAL NOT NULL DEFAULT 0.5,
			embedding BLOB,
			embedding_status TEXT NOT NULL DEFAULT 'pending',
			source_id TEXT NOT NULL DEFAULT '',
			sources TEXT NOT NULL DEFAULT '[]',
			validation_count INTEGER NOT NULL DEFAULT 0,
			last_validated_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			deleted_at TEXT,
			synced_at TEXT
		);
		CREATE TABLE IF NOT EXISTS change_log (
			sequence INTEGER PRIMARY KEY AUTOINCREMENT,
			table_name TEXT NOT NULL,
			entity_id TEXT NOT NULL,
			operation TEXT NOT NULL CHECK(operation IN ('upsert','delete')),
			payload TEXT,
			source_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
			received_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		);
		CREATE TABLE IF NOT EXISTS metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS sync_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS sync_queue (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			entity_type TEXT NOT NULL,
			entity_id TEXT NOT NULL,
			action TEXT NOT NULL,
			payload TEXT,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		);
	`)
	if err != nil {
		db.Close()
		t.Fatalf("create snapshot schema: %v", err)
	}

	// Insert a change_log entry so MAX(sequence) is non-zero
	_, err = db.Exec(`
		INSERT INTO change_log (table_name, entity_id, operation, source_id, created_at)
		VALUES ('lore_entries', 'snap-001', 'upsert', 'snapshot-source', '2026-01-01T00:00:00Z')
	`)
	if err != nil {
		db.Close()
		t.Fatalf("insert change_log: %v", err)
	}
	db.Close()

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("read snapshot file: %v", err)
	}
	return data
}

// =============================================================================
// Bootstrap 503 Retry with Retry-After header
// AC #3: 503 retry: mock returns 503 then 200
// =============================================================================

func TestBootstrap_503Retry_ThenSuccess(t *testing.T) {
	store := newTestStore(t)
	snapshotData := newValidSnapshotDB(t)

	var attempts int32

	healthResp := &engramHealthResponse{
		Status:         "healthy",
		EmbeddingModel: "test-model",
	}

	server := newBootstrapTestServer(t, healthResp, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(snapshotData)
	})
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")
	syncer.SetStoreID("test-store")
	syncer.sleepFn = func(ctx context.Context, d time.Duration) error {
		return nil // no-op sleep for testing
	}

	err := syncer.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap should succeed after 503 retry: %v", err)
	}

	if attempts != 2 {
		t.Errorf("expected 2 snapshot attempts, got %d", attempts)
	}
}

// =============================================================================
// Bootstrap 503 Exhaustion after 3 attempts
// AC #3: 503 retry exhaustion after 3 attempts
// =============================================================================

func TestBootstrap_503Exhaustion(t *testing.T) {
	store := newTestStore(t)

	var attempts int32

	healthResp := &engramHealthResponse{
		Status:         "healthy",
		EmbeddingModel: "test-model",
	}

	server := newBootstrapTestServer(t, healthResp, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")
	syncer.SetStoreID("test-store")
	syncer.sleepFn = func(ctx context.Context, d time.Duration) error {
		return nil
	}

	err := syncer.Bootstrap(context.Background())
	if err == nil {
		t.Fatal("Bootstrap should fail after 3x 503")
	}

	if !strings.Contains(err.Error(), "unavailable after") {
		t.Errorf("error should mention retry exhaustion, got: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected exactly 3 snapshot attempts (bootstrapMaxRetries), got %d", attempts)
	}
}

// =============================================================================
// Bootstrap Integrity Check Failure
// AC #3: integrity check failure preserves existing DB
// =============================================================================

func TestBootstrap_IntegrityCheckFailure_PreservesExistingDB(t *testing.T) {
	store := newTestStore(t)

	// Pre-insert a lore entry that should survive the failed bootstrap
	now := time.Now().UTC()
	existingLore := &Lore{
		ID:              "01EXISTING_LORE_PRESERVED",
		Content:         "This should survive failed bootstrap",
		Category:        CategoryArchitecturalDecision,
		Confidence:      0.8,
		EmbeddingStatus: "pending",
		SourceID:        "local-source",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := store.InsertLore(existingLore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	healthResp := &engramHealthResponse{
		Status:         "healthy",
		EmbeddingModel: "test-model",
	}

	server := newBootstrapTestServer(t, healthResp, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte("this is not a valid SQLite database"))
	})
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")
	syncer.SetStoreID("test-store")

	err := syncer.Bootstrap(context.Background())
	if err == nil {
		t.Fatal("Bootstrap should fail with corrupt snapshot")
	}

	if !strings.Contains(err.Error(), "integrity") {
		t.Errorf("error should mention integrity check, got: %v", err)
	}

	// Existing lore should still be there
	lore, err := store.Get("01EXISTING_LORE_PRESERVED")
	if err != nil {
		t.Fatalf("existing lore should survive failed bootstrap: %v", err)
	}
	if lore.Content != "This should survive failed bootstrap" {
		t.Errorf("lore content changed after failed bootstrap")
	}
}

// =============================================================================
// Bootstrap Embedding Model Validation
// AC #3: embedding model match and mismatch
// =============================================================================

func TestBootstrap_EmbeddingModelMatch(t *testing.T) {
	store := newTestStore(t)
	snapshotData := newValidSnapshotDB(t)

	if err := store.SetMetadata("embedding_model", "text-embedding-3-small"); err != nil {
		t.Fatalf("SetMetadata failed: %v", err)
	}

	healthResp := &engramHealthResponse{
		Status:         "healthy",
		EmbeddingModel: "text-embedding-3-small",
	}

	server := newBootstrapTestServer(t, healthResp, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(snapshotData)
	})
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")
	syncer.SetStoreID("test-store")

	err := syncer.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap should succeed when models match: %v", err)
	}
}

func TestBootstrap_EmbeddingModelMismatch(t *testing.T) {
	store := newTestStore(t)

	if err := store.SetMetadata("embedding_model", "text-embedding-3-small"); err != nil {
		t.Fatalf("SetMetadata failed: %v", err)
	}

	healthResp := &engramHealthResponse{
		Status:         "healthy",
		EmbeddingModel: "text-embedding-ada-002",
	}

	server := newBootstrapTestServer(t, healthResp, func(w http.ResponseWriter, r *http.Request) {
		t.Error("snapshot should not be requested when models mismatch")
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")
	syncer.SetStoreID("test-store")

	err := syncer.Bootstrap(context.Background())
	if err == nil {
		t.Fatal("Bootstrap should fail when models mismatch")
	}

	if !errors.Is(err, ErrModelMismatch) {
		t.Errorf("expected ErrModelMismatch, got: %v", err)
	}
}

func TestBootstrap_FirstTimeModel_Succeeds(t *testing.T) {
	store := newTestStore(t)
	snapshotData := newValidSnapshotDB(t)

	healthResp := &engramHealthResponse{
		Status:         "healthy",
		EmbeddingModel: "text-embedding-3-small",
	}

	server := newBootstrapTestServer(t, healthResp, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(snapshotData)
	})
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")
	syncer.SetStoreID("test-store")

	err := syncer.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap should succeed for first-time sync: %v", err)
	}

	model, err := store.GetMetadata("embedding_model")
	if err != nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}
	if model != "text-embedding-3-small" {
		t.Errorf("embedding_model = %q, want %q", model, "text-embedding-3-small")
	}
}

// =============================================================================
// Bootstrap sync_meta initialization from MAX(sequence)
// AC #3: sync_meta initialized with MAX(change_log.sequence)
// =============================================================================

func TestBootstrap_SyncMetaInitialized(t *testing.T) {
	store := newTestStore(t)
	snapshotData := newValidSnapshotDB(t)

	healthResp := &engramHealthResponse{
		Status:         "healthy",
		EmbeddingModel: "test-model",
	}

	server := newBootstrapTestServer(t, healthResp, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(snapshotData)
	})
	defer server.Close()

	syncer := NewSyncer(store, server.URL, "test-key", "test-source")
	syncer.SetStoreID("test-store")

	err := syncer.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	// Verify last_pull_seq was set from MAX(change_log.sequence)
	lastPullSeq, err := store.GetSyncMeta("last_pull_seq")
	if err != nil {
		t.Fatalf("GetSyncMeta last_pull_seq failed: %v", err)
	}
	if lastPullSeq == "" {
		t.Error("last_pull_seq should be set after bootstrap")
	}

	// Verify last_push_seq was set to 0
	lastPushSeq, err := store.GetSyncMeta("last_push_seq")
	if err != nil {
		t.Fatalf("GetSyncMeta last_push_seq failed: %v", err)
	}
	if lastPushSeq != "0" {
		t.Errorf("last_push_seq = %q, want %q", lastPushSeq, "0")
	}

	// Verify source_id was regenerated (UUID format: 8-4-4-4-12)
	sourceID, err := store.GetSyncMeta("source_id")
	if err != nil {
		t.Fatalf("GetSyncMeta source_id failed: %v", err)
	}
	if sourceID == "" {
		t.Error("source_id should be set after bootstrap")
	}
	parts := strings.Split(sourceID, "-")
	if len(parts) != 5 {
		t.Errorf("source_id %q is not UUID format", sourceID)
	}
}

// =============================================================================
// Integration: Round-trip test
// AC #4: insert lore -> push -> mock pull from different source_id -> verify
// =============================================================================

func TestRoundTrip_InsertPushPullVerify(t *testing.T) {
	store := newTestStore(t)

	now := time.Now().UTC()
	lore := &Lore{
		ID:              "01ROUNDTRIP_TEST_000001",
		Content:         "Round-trip test content",
		Category:        CategoryArchitecturalDecision,
		Confidence:      0.75,
		EmbeddingStatus: "pending",
		SourceID:        "test-source",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := store.InsertLore(lore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	var pushedEntries []ChangeLogEntry

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/sync/push") {
			var req SyncPushRequest
			json.NewDecoder(r.Body).Decode(&req)
			pushedEntries = req.Entries

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(SyncPushResponse{Accepted: len(req.Entries), RemoteSequence: 100})
			return
		}

		if strings.Contains(r.URL.Path, "/sync/delta") {
			nowStr := time.Now().UTC().Format(time.RFC3339)
			deltaPayload := makeDeltaPayload(
				"01ROUNDTRIP_FROM_REMOTE",
				"Content from another agent",
				"PATTERN_OUTCOME",
				"other-agent-source",
				nowStr, nowStr,
			)

			json.NewEncoder(w).Encode(SyncDeltaResponse{
				Entries: []DeltaEntry{
					{
						Sequence:   101,
						TableName:  "lore_entries",
						EntityID:   "01ROUNDTRIP_FROM_REMOTE",
						Operation:  "upsert",
						Payload:    deltaPayload,
						SourceID:   "other-agent-source",
						CreatedAt:  nowStr,
						ReceivedAt: nowStr,
					},
				},
				LastSequence:   101,
				LatestSequence: 101,
				HasMore:        false,
			})
			return
		}

		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	syncer := newTestSyncer(t, store, server.URL)

	// Step 1: Push
	pushResult, err := syncer.SyncPush(context.Background())
	if err != nil {
		t.Fatalf("SyncPush failed: %v", err)
	}
	if pushResult.EntriesPushed != 1 {
		t.Errorf("EntriesPushed = %d, want 1", pushResult.EntriesPushed)
	}
	if len(pushedEntries) != 1 {
		t.Fatalf("server received %d entries, want 1", len(pushedEntries))
	}

	// Step 2: Pull (different source_id entry)
	deltaResult, err := syncer.SyncDelta(context.Background())
	if err != nil {
		t.Fatalf("SyncDelta failed: %v", err)
	}
	if deltaResult.EntriesApplied != 1 {
		t.Errorf("EntriesApplied = %d, want 1", deltaResult.EntriesApplied)
	}

	// Step 3: Verify the remote entry exists locally
	remoteLore, err := store.Get("01ROUNDTRIP_FROM_REMOTE")
	if err != nil {
		t.Fatalf("Get remote lore failed: %v", err)
	}
	if remoteLore.Content != "Content from another agent" {
		t.Errorf("remote lore content = %q, want %q", remoteLore.Content, "Content from another agent")
	}

	// Original lore should still exist
	originalLore, err := store.Get("01ROUNDTRIP_TEST_000001")
	if err != nil {
		t.Fatalf("Get original lore failed: %v", err)
	}
	if originalLore.Content != "Round-trip test content" {
		t.Errorf("original lore content = %q, want %q", originalLore.Content, "Round-trip test content")
	}
}

// =============================================================================
// Client-level offline mode: ErrOffline for all sync operations
// AC #4: offline mode returns ErrOffline for SyncPush, SyncDelta, Bootstrap
// =============================================================================

func TestClient_SyncPush_OfflineMode(t *testing.T) {
	cfg := Config{
		LocalPath: fmt.Sprintf("%s/offline-push.db", t.TempDir()),
	}
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer client.Close()

	_, err = client.SyncPush(context.Background())
	if !errors.Is(err, ErrOffline) {
		t.Errorf("SyncPush offline: got %v, want ErrOffline", err)
	}
}

func TestClient_Sync_OfflineMode(t *testing.T) {
	cfg := Config{
		LocalPath: fmt.Sprintf("%s/offline-sync.db", t.TempDir()),
	}
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer client.Close()

	err = client.Sync(context.Background())
	if !errors.Is(err, ErrOffline) {
		t.Errorf("Sync offline: got %v, want ErrOffline", err)
	}
}
