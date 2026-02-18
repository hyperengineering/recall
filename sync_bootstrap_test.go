package recall

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// Story 10.4: Snapshot Bootstrap with Retry
// ============================================================================

// createSnapshotDB creates a valid SQLite snapshot file with lore_entries and
// optionally change_log entries. Returns the file contents as bytes.
func createSnapshotDB(t *testing.T, loreEntries []Lore, changeLogMaxSeq int64) []byte {
	t.Helper()

	tmpPath := filepath.Join(t.TempDir(), "snapshot.db")
	db, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		t.Fatalf("open snapshot db: %v", err)
	}

	// Create lore_entries table matching Engram schema (no synced_at)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS lore_entries (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			context TEXT,
			category TEXT NOT NULL,
			confidence REAL NOT NULL DEFAULT 0.5,
			embedding BLOB,
			embedding_status TEXT NOT NULL DEFAULT 'complete',
			source_id TEXT NOT NULL,
			sources TEXT NOT NULL DEFAULT '[]',
			validation_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			deleted_at TEXT,
			last_validated_at TEXT
		)
	`)
	if err != nil {
		t.Fatalf("create lore_entries table: %v", err)
	}

	// Create change_log table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS change_log (
			sequence INTEGER PRIMARY KEY AUTOINCREMENT,
			table_name TEXT NOT NULL,
			entity_id TEXT NOT NULL,
			operation TEXT NOT NULL,
			payload TEXT,
			source_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			received_at TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`)
	if err != nil {
		t.Fatalf("create change_log table: %v", err)
	}

	// Insert lore entries
	for _, lore := range loreEntries {
		_, err = db.Exec(`
			INSERT INTO lore_entries (id, content, context, category, confidence, embedding_status, source_id, sources, validation_count, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, '[]', ?, ?, ?)
		`,
			lore.ID, lore.Content, nullString(lore.Context),
			string(lore.Category), lore.Confidence,
			"complete", lore.SourceID, lore.ValidationCount,
			lore.CreatedAt.Format(time.RFC3339), lore.UpdatedAt.Format(time.RFC3339),
		)
		if err != nil {
			t.Fatalf("insert snapshot lore %s: %v", lore.ID, err)
		}
	}

	// Insert change_log entries up to maxSeq
	for seq := int64(1); seq <= changeLogMaxSeq; seq++ {
		_, err = db.Exec(`
			INSERT INTO change_log (table_name, entity_id, operation, source_id, created_at)
			VALUES ('lore_entries', ?, 'upsert', 'remote-source', ?)
		`, fmt.Sprintf("entity-%d", seq), time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			t.Fatalf("insert change_log seq %d: %v", seq, err)
		}
	}

	if err := db.Close(); err != nil {
		t.Fatalf("close snapshot db: %v", err)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("read snapshot file: %v", err)
	}
	return data
}

// noopSleep is a sleep function that returns immediately (for tests).
func noopSleep(ctx context.Context, d time.Duration) error {
	return nil
}

// =============================================================================
// AC #1: Bootstrap sends GET /stores/{id}/sync/snapshot
// AC #2: On 200 with application/octet-stream, integrity check + atomic replace
// =============================================================================

func TestBootstrap_SuccessfulSnapshot(t *testing.T) {
	store := newTestStore(t)

	now := time.Now().UTC()
	snapshotLore := []Lore{
		{
			ID: "snap-lore-001", Content: "Snapshot content 1",
			Category: CategoryArchitecturalDecision, Confidence: 0.9,
			SourceID: "remote-source", CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "snap-lore-002", Content: "Snapshot content 2",
			Category: CategoryPatternOutcome, Confidence: 0.7,
			SourceID: "remote-source", CreatedAt: now, UpdatedAt: now,
		},
	}
	snapshotData := createSnapshotDB(t, snapshotLore, 5)

	var receivedPath string
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(engramHealthResponse{
				Status:         "healthy",
				EmbeddingModel: "test-model",
			})
			return
		}
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(snapshotData)
	}))
	defer server.Close()

	syncer := newTestSyncer(t, store, server.URL)

	err := syncer.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	// AC #1: Correct method and path
	if receivedMethod != "GET" {
		t.Errorf("method = %q, want GET", receivedMethod)
	}
	expectedPath := "/api/v1/stores/test-store/sync/snapshot"
	if receivedPath != expectedPath {
		t.Errorf("path = %q, want %q", receivedPath, expectedPath)
	}

	// AC #2: Lore entries from snapshot are in local store
	lore1, err := store.Get("snap-lore-001")
	if err != nil {
		t.Fatalf("Get snap-lore-001 failed: %v", err)
	}
	if lore1.Content != "Snapshot content 1" {
		t.Errorf("lore1.Content = %q, want %q", lore1.Content, "Snapshot content 1")
	}

	lore2, err := store.Get("snap-lore-002")
	if err != nil {
		t.Fatalf("Get snap-lore-002 failed: %v", err)
	}
	if lore2.Content != "Snapshot content 2" {
		t.Errorf("lore2.Content = %q, want %q", lore2.Content, "Snapshot content 2")
	}
}

// =============================================================================
// AC #3: After successful replacement, sync_meta is initialized
// =============================================================================

func TestBootstrap_SyncMetaInitialized(t *testing.T) {
	store := newTestStore(t)

	// Record the pre-bootstrap source_id
	preBootstrapSourceID := store.SourceID()

	now := time.Now().UTC()
	snapshotLore := []Lore{
		{
			ID: "snap-meta-001", Content: "Meta test",
			Category: CategoryTestingStrategy, Confidence: 0.5,
			SourceID: "remote", CreatedAt: now, UpdatedAt: now,
		},
	}
	// Snapshot has 10 change_log entries → MAX(sequence) = 10
	snapshotData := createSnapshotDB(t, snapshotLore, 10)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(engramHealthResponse{
				Status:         "healthy",
				EmbeddingModel: "test-model",
			})
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(snapshotData)
	}))
	defer server.Close()

	syncer := newTestSyncer(t, store, server.URL)

	err := syncer.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	// AC #3: last_pull_seq = MAX(change_log.sequence) from snapshot
	lastPullSeq, err := store.GetSyncMeta("last_pull_seq")
	if err != nil {
		t.Fatalf("GetSyncMeta last_pull_seq failed: %v", err)
	}
	if lastPullSeq != "10" {
		t.Errorf("last_pull_seq = %q, want %q", lastPullSeq, "10")
	}

	// AC #3: last_push_seq = 0
	lastPushSeq, err := store.GetSyncMeta("last_push_seq")
	if err != nil {
		t.Fatalf("GetSyncMeta last_push_seq failed: %v", err)
	}
	if lastPushSeq != "0" {
		t.Errorf("last_push_seq = %q, want %q", lastPushSeq, "0")
	}

	// AC #3: source_id is a fresh UUIDv4 (different from pre-bootstrap)
	newSourceID, err := store.GetSyncMeta("source_id")
	if err != nil {
		t.Fatalf("GetSyncMeta source_id failed: %v", err)
	}
	if newSourceID == "" {
		t.Error("source_id should not be empty after bootstrap")
	}
	if newSourceID == preBootstrapSourceID {
		t.Errorf("source_id should be fresh after bootstrap, got same value: %s", newSourceID)
	}

	// Verify cached source_id is also updated
	if store.SourceID() != newSourceID {
		t.Errorf("cached SourceID() = %q, want %q (from sync_meta)", store.SourceID(), newSourceID)
	}
}

func TestBootstrap_SyncMetaInitialized_EmptyChangeLog(t *testing.T) {
	store := newTestStore(t)

	now := time.Now().UTC()
	snapshotLore := []Lore{
		{
			ID: "snap-empty-cl-001", Content: "Empty change log test",
			Category: CategoryTestingStrategy, Confidence: 0.5,
			SourceID: "remote", CreatedAt: now, UpdatedAt: now,
		},
	}
	// Snapshot has 0 change_log entries → last_pull_seq = 0
	snapshotData := createSnapshotDB(t, snapshotLore, 0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(engramHealthResponse{
				Status:         "healthy",
				EmbeddingModel: "test-model",
			})
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(snapshotData)
	}))
	defer server.Close()

	syncer := newTestSyncer(t, store, server.URL)

	err := syncer.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	// Empty change_log → last_pull_seq = 0
	lastPullSeq, err := store.GetSyncMeta("last_pull_seq")
	if err != nil {
		t.Fatalf("GetSyncMeta last_pull_seq failed: %v", err)
	}
	if lastPullSeq != "0" {
		t.Errorf("last_pull_seq = %q, want %q", lastPullSeq, "0")
	}
}

// =============================================================================
// AC #4: On 503 with Retry-After, client waits and retries up to 3 times
// =============================================================================

func TestBootstrap_RetryOn503WithRetryAfter(t *testing.T) {
	store := newTestStore(t)

	now := time.Now().UTC()
	snapshotData := createSnapshotDB(t, []Lore{
		{
			ID: "snap-retry-001", Content: "Retry test",
			Category: CategoryTestingStrategy, Confidence: 0.5,
			SourceID: "remote", CreatedAt: now, UpdatedAt: now,
		},
	}, 1)

	var snapshotAttempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(engramHealthResponse{
				Status:         "healthy",
				EmbeddingModel: "test-model",
			})
			return
		}
		attempt := atomic.AddInt32(&snapshotAttempts, 1)
		if attempt == 1 {
			// First attempt: 503 with Retry-After
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("snapshot building"))
			return
		}
		// Second attempt: success
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(snapshotData)
	}))
	defer server.Close()

	syncer := newTestSyncer(t, store, server.URL)
	syncer.sleepFn = noopSleep

	err := syncer.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap should succeed after retry: %v", err)
	}

	if atomic.LoadInt32(&snapshotAttempts) != 2 {
		t.Errorf("snapshot attempts = %d, want 2", atomic.LoadInt32(&snapshotAttempts))
	}

	// Verify snapshot was applied
	_, err = store.Get("snap-retry-001")
	if err != nil {
		t.Fatalf("snapshot lore should exist after retry: %v", err)
	}
}

func TestBootstrap_RetryOn503_ParsesRetryAfterDuration(t *testing.T) {
	store := newTestStore(t)

	now := time.Now().UTC()
	snapshotData := createSnapshotDB(t, []Lore{
		{
			ID: "snap-delay-001", Content: "Delay test",
			Category: CategoryTestingStrategy, Confidence: 0.5,
			SourceID: "remote", CreatedAt: now, UpdatedAt: now,
		},
	}, 0)

	var sleepDurations []time.Duration
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(engramHealthResponse{
				Status:         "healthy",
				EmbeddingModel: "test-model",
			})
			return
		}
		if len(sleepDurations) == 0 {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(snapshotData)
	}))
	defer server.Close()

	syncer := newTestSyncer(t, store, server.URL)
	syncer.sleepFn = func(ctx context.Context, d time.Duration) error {
		sleepDurations = append(sleepDurations, d)
		return nil
	}

	err := syncer.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	// Verify Retry-After of 30 seconds was parsed
	if len(sleepDurations) != 1 {
		t.Fatalf("expected 1 sleep, got %d", len(sleepDurations))
	}
	if sleepDurations[0] != 30*time.Second {
		t.Errorf("sleep duration = %v, want %v", sleepDurations[0], 30*time.Second)
	}
}

func TestBootstrap_RetryOn503_DefaultRetryAfter(t *testing.T) {
	store := newTestStore(t)

	now := time.Now().UTC()
	snapshotData := createSnapshotDB(t, []Lore{
		{
			ID: "snap-default-001", Content: "Default retry test",
			Category: CategoryTestingStrategy, Confidence: 0.5,
			SourceID: "remote", CreatedAt: now, UpdatedAt: now,
		},
	}, 0)

	var sleepDurations []time.Duration
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(engramHealthResponse{
				Status:         "healthy",
				EmbeddingModel: "test-model",
			})
			return
		}
		attempt := atomic.AddInt32(&attempts, 1)
		if attempt == 1 {
			// 503 without Retry-After header → default 60s
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(snapshotData)
	}))
	defer server.Close()

	syncer := newTestSyncer(t, store, server.URL)
	syncer.sleepFn = func(ctx context.Context, d time.Duration) error {
		sleepDurations = append(sleepDurations, d)
		return nil
	}

	err := syncer.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	if len(sleepDurations) != 1 {
		t.Fatalf("expected 1 sleep, got %d", len(sleepDurations))
	}
	// Default retry-after is 60 seconds when header is missing
	if sleepDurations[0] != 60*time.Second {
		t.Errorf("sleep duration = %v, want %v", sleepDurations[0], 60*time.Second)
	}
}

// =============================================================================
// AC #5: After 3 consecutive 503 responses, error is returned
// =============================================================================

func TestBootstrap_RetryExhaustion(t *testing.T) {
	store := newTestStore(t)

	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(engramHealthResponse{
				Status:         "healthy",
				EmbeddingModel: "test-model",
			})
			return
		}
		atomic.AddInt32(&attempts, 1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("snapshot building"))
	}))
	defer server.Close()

	syncer := newTestSyncer(t, store, server.URL)
	syncer.sleepFn = noopSleep

	err := syncer.Bootstrap(context.Background())
	if err == nil {
		t.Fatal("Bootstrap should fail after 3 consecutive 503s")
	}

	// Should have attempted exactly 3 times
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("snapshot attempts = %d, want 3", atomic.LoadInt32(&attempts))
	}

	// Error should be descriptive
	if got := err.Error(); !strings.Contains(got, "snapshot unavailable") && !strings.Contains(got, "503") && !strings.Contains(got, "retry") {
		t.Errorf("error should mention snapshot unavailable or retries, got: %s", got)
	}
}

// =============================================================================
// AC #6: PRAGMA integrity_check failure → snapshot discarded, existing DB preserved
// =============================================================================

func TestBootstrap_IntegrityCheckFailure_PreservesExistingDB(t *testing.T) {
	store := newTestStore(t)

	// Pre-insert lore into local DB that should be preserved on failure
	now := time.Now().UTC()
	existingLore := &Lore{
		ID: "existing-lore-001", Content: "Existing content",
		Category: CategoryArchitecturalDecision, Confidence: 0.8,
		SourceID: "local-source", CreatedAt: now, UpdatedAt: now,
		EmbeddingStatus: "pending",
	}
	if err := store.InsertLore(existingLore); err != nil {
		t.Fatalf("InsertLore failed: %v", err)
	}

	// Serve corrupted data that is not a valid SQLite file
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(engramHealthResponse{
				Status:         "healthy",
				EmbeddingModel: "test-model",
			})
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		// Write garbage data — not a valid SQLite file
		w.Write([]byte("THIS IS NOT A VALID SQLITE DATABASE FILE AT ALL"))
	}))
	defer server.Close()

	syncer := newTestSyncer(t, store, server.URL)

	err := syncer.Bootstrap(context.Background())
	if err == nil {
		t.Fatal("Bootstrap should fail with corrupted snapshot")
	}

	// Existing lore should be preserved
	lore, err := store.Get("existing-lore-001")
	if err != nil {
		t.Fatalf("existing lore should be preserved: %v", err)
	}
	if lore.Content != "Existing content" {
		t.Errorf("existing lore content = %q, want %q", lore.Content, "Existing content")
	}
}

// =============================================================================
// AC #7: Health check with embedding model records model name (existing behavior)
// =============================================================================

func TestBootstrap_EmbeddingModelRecorded(t *testing.T) {
	store := newTestStore(t)

	now := time.Now().UTC()
	snapshotData := createSnapshotDB(t, []Lore{
		{
			ID: "snap-model-001", Content: "Model test",
			Category: CategoryTestingStrategy, Confidence: 0.5,
			SourceID: "remote", CreatedAt: now, UpdatedAt: now,
		},
	}, 0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(engramHealthResponse{
				Status:         "healthy",
				EmbeddingModel: "text-embedding-3-small",
			})
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(snapshotData)
	}))
	defer server.Close()

	syncer := newTestSyncer(t, store, server.URL)

	err := syncer.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	// Embedding model should be recorded in metadata
	model, err := store.GetMetadata("embedding_model")
	if err != nil {
		t.Fatalf("GetMetadata embedding_model failed: %v", err)
	}
	if model != "text-embedding-3-small" {
		t.Errorf("embedding_model = %q, want %q", model, "text-embedding-3-small")
	}
}

// =============================================================================
// AC #8: When no Engram URL configured, ErrOffline is returned
// (Tested at Client level — Bootstrap via Client.Bootstrap checks syncer==nil)
// =============================================================================

func TestBootstrap_OfflineMode(t *testing.T) {
	store := newTestStore(t)

	// Syncer with empty URL → offline
	syncer := NewSyncer(store, "", "test-key", "test-source")
	syncer.SetStoreID("test-store")

	// Client-level check: c.syncer == nil → ErrOffline
	// Syncer-level: Bootstrap will fail at health check (empty URL → connection refused)
	err := syncer.Bootstrap(context.Background())
	if err == nil {
		t.Error("Bootstrap with empty URL should fail")
	}
}

// =============================================================================
// Additional edge case: 503 retry respects context cancellation
// =============================================================================

func TestBootstrap_RetryRespectsContextCancellation(t *testing.T) {
	store := newTestStore(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(engramHealthResponse{
				Status:         "healthy",
				EmbeddingModel: "test-model",
			})
			return
		}
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	syncer := newTestSyncer(t, store, server.URL)
	// sleepFn that returns context error
	syncer.sleepFn = func(ctx context.Context, d time.Duration) error {
		return ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := syncer.Bootstrap(ctx)
	if err == nil {
		t.Error("Bootstrap should fail when context is cancelled")
	}
}


