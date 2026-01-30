package sync

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/hyperengineering/recall"
)

// SyncStore defines the store operations needed for sync.
type SyncStore interface {
	// Metadata operations
	GetMetadata(key string) (string, error)
	SetMetadata(key, value string) error

	// Bootstrap operations
	ReplaceFromSnapshot(r io.Reader) error

	// Push operations (for Story 4.3)
	Unsynced() ([]recall.Lore, error)
	MarkSynced(ids []string, syncedAt time.Time) error
	PendingFeedback() ([]FeedbackEntry, error)
	MarkFeedbackSynced(ids []int64) error
}

// FeedbackEntry represents a pending feedback item in the sync queue.
type FeedbackEntry struct {
	ID      int64
	LoreID  string
	Outcome string
}

// Syncer orchestrates synchronization with Engram.
type Syncer struct {
	store  SyncStore
	client EngramClient
}

// NewSyncer creates a syncer with injected dependencies.
func NewSyncer(store SyncStore, client EngramClient) *Syncer {
	return &Syncer{store: store, client: client}
}

// Bootstrap downloads a full snapshot from Engram and replaces the local lore.
//
// Process:
//  1. HealthCheck() to validate connectivity and get embedding model
//  2. Compare embedding model with local metadata
//  3. If mismatch and not first-time, return ErrModelMismatch
//  4. DownloadSnapshot() and stream to store
//  5. Store atomically replaces lore table
//  6. Update metadata (embedding_model, last_sync)
//
// Returns ErrOffline if client is nil (offline mode).
// Returns ErrModelMismatch if embedding models differ.
func (s *Syncer) Bootstrap(ctx context.Context) error {
	if s.client == nil {
		return recall.ErrOffline
	}

	// 1. Health check
	health, err := s.client.HealthCheck(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap: health check: %w", err)
	}

	// 2. Validate embedding model compatibility
	// Ignore error: empty result means first-time sync (model check passes)
	localModel, _ := s.store.GetMetadata("embedding_model")
	if localModel != "" && localModel != health.EmbeddingModel {
		return fmt.Errorf("bootstrap: %w: local=%s, remote=%s",
			recall.ErrModelMismatch, localModel, health.EmbeddingModel)
	}

	// 3. Download snapshot
	snapshot, err := s.client.DownloadSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap: download: %w", err)
	}
	defer func() { _ = snapshot.Close() }()

	// 4. Replace local store (atomic)
	if err := s.store.ReplaceFromSnapshot(snapshot); err != nil {
		return fmt.Errorf("bootstrap: replace store: %w", err)
	}

	// 5. Update metadata
	if err := s.store.SetMetadata("embedding_model", health.EmbeddingModel); err != nil {
		return fmt.Errorf("bootstrap: set embedding_model: %w", err)
	}
	if err := s.store.SetMetadata("last_sync", time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("bootstrap: set last_sync: %w", err)
	}

	return nil
}
