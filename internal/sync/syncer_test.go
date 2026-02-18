package sync

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/hyperengineering/recall"
)

// mockEngramClient implements EngramClient for testing.
type mockEngramClient struct {
	healthCheckFn               func(ctx context.Context) (*HealthResponse, error)
	downloadSnapshotFn          func(ctx context.Context) (io.ReadCloser, error)
	syncPushFn                  func(ctx context.Context, storeID string, req *recall.SyncPushRequest) (*recall.SyncPushResponse, error)
	syncDeltaFn                 func(ctx context.Context, storeID string, after int64, limit int) (*recall.SyncDeltaResponse, error)
	syncSnapshotFn              func(ctx context.Context, storeID string) (io.ReadCloser, error)
	listStoresFn                func(ctx context.Context, prefix string) (*ListStoresResponse, error)
	getStoreInfoFn              func(ctx context.Context, storeID string) (*StoreInfoResponse, error)
	createStoreFn               func(ctx context.Context, req *CreateStoreRequest) (*CreateStoreResponse, error)
	deleteStoreFn               func(ctx context.Context, storeID string) error
	downloadSnapshotFromStoreFn func(ctx context.Context, storeID string) (io.ReadCloser, error)
	deleteLoreFromStoreFn       func(ctx context.Context, storeID, loreID string) error
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

func (m *mockEngramClient) SyncPush(ctx context.Context, storeID string, req *recall.SyncPushRequest) (*recall.SyncPushResponse, error) {
	if m.syncPushFn != nil {
		return m.syncPushFn(ctx, storeID, req)
	}
	return nil, errors.New("not implemented")
}

func (m *mockEngramClient) SyncDelta(ctx context.Context, storeID string, after int64, limit int) (*recall.SyncDeltaResponse, error) {
	if m.syncDeltaFn != nil {
		return m.syncDeltaFn(ctx, storeID, after, limit)
	}
	return nil, errors.New("not implemented")
}

func (m *mockEngramClient) SyncSnapshot(ctx context.Context, storeID string) (io.ReadCloser, error) {
	if m.syncSnapshotFn != nil {
		return m.syncSnapshotFn(ctx, storeID)
	}
	return nil, errors.New("not implemented")
}

func (m *mockEngramClient) ListStores(ctx context.Context, prefix string) (*ListStoresResponse, error) {
	if m.listStoresFn != nil {
		return m.listStoresFn(ctx, prefix)
	}
	return nil, errors.New("not implemented")
}

func (m *mockEngramClient) GetStoreInfo(ctx context.Context, storeID string) (*StoreInfoResponse, error) {
	if m.getStoreInfoFn != nil {
		return m.getStoreInfoFn(ctx, storeID)
	}
	return nil, errors.New("not implemented")
}

func (m *mockEngramClient) CreateStore(ctx context.Context, req *CreateStoreRequest) (*CreateStoreResponse, error) {
	if m.createStoreFn != nil {
		return m.createStoreFn(ctx, req)
	}
	return nil, errors.New("not implemented")
}

func (m *mockEngramClient) DeleteStore(ctx context.Context, storeID string) error {
	if m.deleteStoreFn != nil {
		return m.deleteStoreFn(ctx, storeID)
	}
	return errors.New("not implemented")
}

func (m *mockEngramClient) DownloadSnapshotFromStore(ctx context.Context, storeID string) (io.ReadCloser, error) {
	if m.downloadSnapshotFromStoreFn != nil {
		return m.downloadSnapshotFromStoreFn(ctx, storeID)
	}
	return nil, errors.New("not implemented")
}

func (m *mockEngramClient) DeleteLoreFromStore(ctx context.Context, storeID, loreID string) error {
	if m.deleteLoreFromStoreFn != nil {
		return m.deleteLoreFromStoreFn(ctx, storeID, loreID)
	}
	return errors.New("not implemented")
}

// mockSyncStore implements SyncStore for testing.
type mockSyncStore struct {
	metadata       map[string]string
	getMetadataErr error
	setMetadataErr error
	replaceErr     error
	replaceCalled  bool
	replaceData    []byte
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
	if store.metadata["embedding_model"] != "" {
		t.Error("embedding_model should not be set when replace fails")
	}
}

func TestSyncer_Bootstrap_OfflineMode(t *testing.T) {
	store := newMockSyncStore()
	syncer := NewSyncer(store, nil)

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
	cancel()

	err := syncer.Bootstrap(ctx)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "cancel") {
		t.Logf("got error: %v", err)
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
