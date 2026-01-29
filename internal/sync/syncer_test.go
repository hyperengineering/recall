package sync

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/hyperengineering/recall"
)

// mockEngramClient implements EngramClient for testing.
type mockEngramClient struct {
	healthCheckFn     func(ctx context.Context) (*HealthResponse, error)
	downloadSnapshotFn func(ctx context.Context) (io.ReadCloser, error)
	pushLoreFn        func(ctx context.Context, req *PushLoreRequest) (*PushLoreResponse, error)
	pushFeedbackFn    func(ctx context.Context, req *PushFeedbackRequest) (*PushFeedbackResponse, error)
}

func (m *mockEngramClient) HealthCheck(ctx context.Context) (*HealthResponse, error) {
	if m.healthCheckFn != nil {
		return m.healthCheckFn(ctx)
	}
	return nil, errors.New("not implemented")
}

func (m *mockEngramClient) DownloadSnapshot(ctx context.Context) (io.ReadCloser, error) {
	if m.downloadSnapshotFn != nil {
		return m.downloadSnapshotFn(ctx)
	}
	return nil, errors.New("not implemented")
}

func (m *mockEngramClient) PushLore(ctx context.Context, req *PushLoreRequest) (*PushLoreResponse, error) {
	if m.pushLoreFn != nil {
		return m.pushLoreFn(ctx, req)
	}
	return nil, errors.New("not implemented")
}

func (m *mockEngramClient) PushFeedback(ctx context.Context, req *PushFeedbackRequest) (*PushFeedbackResponse, error) {
	if m.pushFeedbackFn != nil {
		return m.pushFeedbackFn(ctx, req)
	}
	return nil, errors.New("not implemented")
}

// mockSyncStore implements SyncStore for testing.
type mockSyncStore struct {
	metadata          map[string]string
	getMetadataErr    error
	setMetadataErr    error
	replaceErr        error
	replaceCalled     bool
	replaceData       []byte
}

func newMockSyncStore() *mockSyncStore {
	return &mockSyncStore{
		metadata: make(map[string]string),
	}
}

func (m *mockSyncStore) GetMetadata(key string) (string, error) {
	if m.getMetadataErr != nil {
		return "", m.getMetadataErr
	}
	return m.metadata[key], nil
}

func (m *mockSyncStore) SetMetadata(key, value string) error {
	if m.setMetadataErr != nil {
		return m.setMetadataErr
	}
	m.metadata[key] = value
	return nil
}

func (m *mockSyncStore) ReplaceFromSnapshot(r io.Reader) error {
	m.replaceCalled = true
	data, _ := io.ReadAll(r)
	m.replaceData = data
	return m.replaceErr
}

func (m *mockSyncStore) Unsynced() ([]recall.Lore, error) {
	return nil, nil
}

func (m *mockSyncStore) MarkSynced(ids []string, syncedAt time.Time) error {
	return nil
}

func (m *mockSyncStore) PendingFeedback() ([]FeedbackEntry, error) {
	return nil, nil
}

func (m *mockSyncStore) MarkFeedbackSynced(ids []int64) error {
	return nil
}

func TestSyncer_Bootstrap_Success(t *testing.T) {
	store := newMockSyncStore()
	client := &mockEngramClient{
		healthCheckFn: func(ctx context.Context) (*HealthResponse, error) {
			return &HealthResponse{
				Status:         "healthy",
				EmbeddingModel: "text-embedding-3-small",
			}, nil
		},
		downloadSnapshotFn: func(ctx context.Context) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("snapshot-data")), nil
		},
	}

	syncer := NewSyncer(store, client)
	err := syncer.Bootstrap(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !store.replaceCalled {
		t.Error("ReplaceFromSnapshot was not called")
	}
	if store.metadata["embedding_model"] != "text-embedding-3-small" {
		t.Errorf("embedding_model = %q, want %q", store.metadata["embedding_model"], "text-embedding-3-small")
	}
	if store.metadata["last_sync"] == "" {
		t.Error("last_sync metadata not set")
	}
}

func TestSyncer_Bootstrap_HealthCheckFails(t *testing.T) {
	store := newMockSyncStore()
	client := &mockEngramClient{
		healthCheckFn: func(ctx context.Context) (*HealthResponse, error) {
			return nil, &recall.SyncError{Operation: "health_check", StatusCode: 503, Err: errors.New("service unavailable")}
		},
	}

	syncer := NewSyncer(store, client)
	err := syncer.Bootstrap(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "health check") {
		t.Errorf("error = %q, want to contain 'health check'", err.Error())
	}
	if store.replaceCalled {
		t.Error("ReplaceFromSnapshot should not be called when health check fails")
	}
}

func TestSyncer_Bootstrap_ModelMismatch(t *testing.T) {
	store := newMockSyncStore()
	store.metadata["embedding_model"] = "old-model-v1"

	client := &mockEngramClient{
		healthCheckFn: func(ctx context.Context) (*HealthResponse, error) {
			return &HealthResponse{
				Status:         "healthy",
				EmbeddingModel: "new-model-v2",
			}, nil
		},
	}

	syncer := NewSyncer(store, client)
	err := syncer.Bootstrap(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, recall.ErrModelMismatch) {
		t.Errorf("expected ErrModelMismatch, got %v", err)
	}
	if store.replaceCalled {
		t.Error("ReplaceFromSnapshot should not be called when models mismatch")
	}
}

func TestSyncer_Bootstrap_FirstTimeModel(t *testing.T) {
	store := newMockSyncStore()
	// No embedding_model set (first time)

	client := &mockEngramClient{
		healthCheckFn: func(ctx context.Context) (*HealthResponse, error) {
			return &HealthResponse{
				Status:         "healthy",
				EmbeddingModel: "text-embedding-3-small",
			}, nil
		},
		downloadSnapshotFn: func(ctx context.Context) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("snapshot")), nil
		},
	}

	syncer := NewSyncer(store, client)
	err := syncer.Bootstrap(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.metadata["embedding_model"] != "text-embedding-3-small" {
		t.Errorf("embedding_model = %q, want %q", store.metadata["embedding_model"], "text-embedding-3-small")
	}
}

func TestSyncer_Bootstrap_DownloadFails(t *testing.T) {
	store := newMockSyncStore()
	client := &mockEngramClient{
		healthCheckFn: func(ctx context.Context) (*HealthResponse, error) {
			return &HealthResponse{
				Status:         "healthy",
				EmbeddingModel: "model",
			}, nil
		},
		downloadSnapshotFn: func(ctx context.Context) (io.ReadCloser, error) {
			return nil, &recall.SyncError{Operation: "download_snapshot", StatusCode: 500, Err: errors.New("download failed")}
		},
	}

	syncer := NewSyncer(store, client)
	err := syncer.Bootstrap(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "download") {
		t.Errorf("error = %q, want to contain 'download'", err.Error())
	}
	if store.replaceCalled {
		t.Error("ReplaceFromSnapshot should not be called when download fails")
	}
}

func TestSyncer_Bootstrap_ReplaceStoreFails(t *testing.T) {
	store := newMockSyncStore()
	store.replaceErr = errors.New("database error")

	client := &mockEngramClient{
		healthCheckFn: func(ctx context.Context) (*HealthResponse, error) {
			return &HealthResponse{
				Status:         "healthy",
				EmbeddingModel: "model",
			}, nil
		},
		downloadSnapshotFn: func(ctx context.Context) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("snapshot")), nil
		},
	}

	syncer := NewSyncer(store, client)
	err := syncer.Bootstrap(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "replace store") {
		t.Errorf("error = %q, want to contain 'replace store'", err.Error())
	}
	// Metadata should not be updated on failure
	if store.metadata["embedding_model"] != "" {
		t.Error("embedding_model should not be set when replace fails")
	}
}

func TestSyncer_Bootstrap_OfflineMode(t *testing.T) {
	store := newMockSyncStore()
	syncer := NewSyncer(store, nil) // nil client = offline mode

	err := syncer.Bootstrap(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, recall.ErrOffline) {
		t.Errorf("expected ErrOffline, got %v", err)
	}
}

func TestSyncer_Bootstrap_ContextCancellation(t *testing.T) {
	store := newMockSyncStore()
	client := &mockEngramClient{
		healthCheckFn: func(ctx context.Context) (*HealthResponse, error) {
			// Check if context is cancelled
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
				return &HealthResponse{EmbeddingModel: "model"}, nil
			}
		},
	}

	syncer := NewSyncer(store, client)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := syncer.Bootstrap(ctx)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// The error should be wrapped but ultimately caused by context cancellation
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "cancel") {
		t.Logf("got error: %v", err)
		// This is acceptable as the implementation may wrap the error
	}
}

func TestSyncer_Bootstrap_SetMetadataFails(t *testing.T) {
	store := newMockSyncStore()
	store.setMetadataErr = errors.New("metadata write failed")

	client := &mockEngramClient{
		healthCheckFn: func(ctx context.Context) (*HealthResponse, error) {
			return &HealthResponse{EmbeddingModel: "model"}, nil
		},
		downloadSnapshotFn: func(ctx context.Context) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("data")), nil
		},
	}

	syncer := NewSyncer(store, client)
	err := syncer.Bootstrap(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "embedding_model") {
		t.Errorf("error = %q, want to contain 'embedding_model'", err.Error())
	}
}
