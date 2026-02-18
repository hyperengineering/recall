package recall

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Syncer handles synchronization with the Engram central service.
//
// Architecture Note: There are two Syncer implementations in this codebase:
//
//  1. recall.Syncer (this type) - Used by recall.Client for production sync.
//     Directly coupled to recall.Store and uses net/http for Engram communication.
//     This is the implementation used by the CLI and public API.
//
//  2. internal/sync.Syncer - Uses dependency injection via interfaces (SyncStore,
//     EngramClient). Designed for unit testing with mocks. Currently used for
//     Bootstrap testing but not fully integrated with recall.Client.
//
// The split exists because internal/sync.Syncer was designed for testability,
// while recall.Syncer evolved organically with direct Store coupling. A future
// refactor could unify these by having recall.Syncer delegate to internal/sync.Syncer,
// or by replacing recall.Syncer entirely with the interface-based version.
type Syncer struct {
	store     *Store
	storeID   string // Store context for multi-store sync (Story 7.5)
	engramURL string
	apiKey    string
	sourceID  string
	client    *http.Client
	debug     *DebugLogger
}

// NewSyncer creates a new syncer.
func NewSyncer(store *Store, engramURL, apiKey, sourceID string) *Syncer {
	return &Syncer{
		store:     store,
		engramURL: engramURL,
		apiKey:    apiKey,
		sourceID:  sourceID,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetDebugLogger sets the debug logger for the syncer.
func (s *Syncer) SetDebugLogger(logger *DebugLogger) {
	s.debug = logger
}

// SetStoreID sets the store context for sync operations.
// All sync path helpers require a non-empty storeID and will panic if not set.
func (s *Syncer) SetStoreID(storeID string) {
	s.storeID = storeID
}

// StoreID returns the current store context.
func (s *Syncer) StoreID() string {
	return s.storeID
}

// encodeStoreID URL-encodes a store ID for use in path parameters.
// Example: "neuralmux/engram" -> "neuralmux%2Fengram"
//
// Note: This function is duplicated in internal/sync/client.go due to import
// cycle constraints. Both implementations must remain identical.
func encodeStoreID(storeID string) string {
	return url.PathEscape(storeID)
}

// pushPath returns the API path for sync push operations.
// Panics if storeID is not set — all sync operations require a store context.
func (s *Syncer) pushPath() string {
	if s.storeID == "" {
		panic("recall: pushPath requires storeID to be set")
	}
	return fmt.Sprintf("/api/v1/stores/%s/sync/push", encodeStoreID(s.storeID))
}

// deltaPath returns the API path for sync delta operations.
// Panics if storeID is not set — all sync operations require a store context.
func (s *Syncer) deltaPath() string {
	if s.storeID == "" {
		panic("recall: deltaPath requires storeID to be set")
	}
	return fmt.Sprintf("/api/v1/stores/%s/sync/delta", encodeStoreID(s.storeID))
}

// snapshotPath returns the API path for sync snapshot operations.
// Panics if storeID is not set — all sync operations require a store context.
func (s *Syncer) snapshotPath() string {
	if s.storeID == "" {
		panic("recall: snapshotPath requires storeID to be set")
	}
	return fmt.Sprintf("/api/v1/stores/%s/sync/snapshot", encodeStoreID(s.storeID))
}

// engramHealthResponse represents the Engram health check response.
type engramHealthResponse struct {
	Status         string `json:"status"`
	Version        string `json:"version"`
	EmbeddingModel string `json:"embedding_model"`
	LoreCount      int    `json:"lore_count"`
	LastSnapshot   string `json:"last_snapshot"`
}

// =============================================================================
// Sync Protocol DTOs (Story 10.1)
// =============================================================================

// SyncPushRequest is the request body for POST /sync/push.
type SyncPushRequest struct {
	PushID        string           `json:"push_id"`
	SourceID      string           `json:"source_id"`
	SchemaVersion int              `json:"schema_version"`
	Entries       []ChangeLogEntry `json:"entries"`
}

// SyncPushResponse is the response from POST /sync/push.
type SyncPushResponse struct {
	Accepted       int   `json:"accepted"`
	RemoteSequence int64 `json:"remote_sequence"`
}

// SyncDeltaResponse is the response from GET /sync/delta.
type SyncDeltaResponse struct {
	Entries        []DeltaEntry `json:"entries"`
	LastSequence   int64        `json:"last_sequence"`
	LatestSequence int64        `json:"latest_sequence"`
	HasMore        bool         `json:"has_more"`
}

// DeltaEntry represents a single entry in the delta response.
type DeltaEntry struct {
	Sequence   int64           `json:"sequence"`
	TableName  string          `json:"table_name"`
	EntityID   string          `json:"entity_id"`
	Operation  string          `json:"operation"`
	Payload    json.RawMessage `json:"payload"`
	SourceID   string          `json:"source_id"`
	CreatedAt  string          `json:"created_at"`
	ReceivedAt string          `json:"received_at"`
}

// SyncValidationError represents a 422 response from POST /sync/push.
type SyncValidationError struct {
	Accepted int          `json:"accepted"`
	Errors   []EntryError `json:"errors"`
}

// EntryError represents a single entry-level error in a validation response.
type EntryError struct {
	Sequence  int64  `json:"sequence"`
	TableName string `json:"table_name"`
	EntityID  string `json:"entity_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

// SchemaMismatchError represents a 409 response indicating schema version conflict.
type SchemaMismatchError struct {
	ClientVersion int    `json:"client_version"`
	ServerVersion int    `json:"server_version"`
	Detail        string `json:"detail"`
}


// Sync performs a full sync cycle: push pending, then pull updates.
//
// Deprecated: Use SyncPush() to push changes and SyncDelta() to pull updates.
// Sync() will be removed in v2.0. The Pull() component of Sync() does not
// apply delta changes to the local store; use SyncDelta() for full sync.
func (s *Syncer) Sync(ctx context.Context) error {
	if err := s.Push(ctx); err != nil {
		return fmt.Errorf("push: %w", err)
	}
	if err := s.Pull(ctx); err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	return nil
}

// Health checks the Engram service health.
func (s *Syncer) Health(ctx context.Context) (*engramHealthResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.engramURL+"/api/v1/health", nil)
	if err != nil {
		return nil, err
	}
	s.setHeaders(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("health check failed: %s", resp.Status)
	}

	var health engramHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, err
	}

	return &health, nil
}

// Push sends pending changes to Engram.
//
// Note: The legacy sync_queue-based push was removed in Story 9.3.
// The new change_log-based push will be implemented in Epic 10.
// Until then, Push() is a no-op that returns nil.
func (s *Syncer) Push(ctx context.Context) error {
	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}


// Pull fetches updates from Engram.
//
// Deprecated: Pull is a no-op. The legacy lore-based delta protocol was removed
// in Story 10.1. Use the new change_log-based sync protocol (Epic 10) instead.
func (s *Syncer) Pull(ctx context.Context) error {
	return nil
}

// SyncDelta fetches and applies incremental changes from Engram.
//
// Note: The legacy lore-based delta sync was removed in Story 10.1.
// The new change_log-based delta sync will be implemented in a later story.
// Until then, SyncDelta() is a no-op that returns nil.
func (s *Syncer) SyncDelta(ctx context.Context) error {
	return nil
}

// Bootstrap downloads a full snapshot from Engram and replaces the local lore.
//
// Process:
//  1. HealthCheck() to validate connectivity and get embedding model
//  2. Compare embedding model with local metadata
//  3. If mismatch and not first-time, return ErrModelMismatch
//  4. Download snapshot and stream to store
//  5. Store atomically replaces lore table
//  6. Update metadata (embedding_model, last_sync)
func (s *Syncer) Bootstrap(ctx context.Context) error {
	// 1. Health check
	health, err := s.Health(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap: health check: %w", err)
	}

	// 2. Validate embedding model compatibility
	// Ignore error: empty result means first-time sync (model check passes)
	localModel, _ := s.store.GetMetadata("embedding_model")
	if localModel != "" && localModel != health.EmbeddingModel {
		return fmt.Errorf("bootstrap: %w: local=%s, remote=%s",
			ErrModelMismatch, localModel, health.EmbeddingModel)
	}

	// 3. Download snapshot
	req, err := http.NewRequestWithContext(ctx, "GET", s.engramURL+s.snapshotPath(), nil)
	if err != nil {
		return fmt.Errorf("bootstrap: create request: %w", err)
	}
	s.setHeaders(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("bootstrap: download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bootstrap: download failed: %s - %s", resp.Status, string(respBody))
	}

	// 4. Replace local store (atomic)
	if err := s.store.ReplaceFromSnapshot(resp.Body); err != nil {
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

// Flush pushes all pending lore immediately (used on shutdown).
//
// Note: The legacy sync_queue-based flush was removed in Story 9.3.
// The new change_log-based flush will be implemented in Epic 10.
// Until then, Flush() is a no-op that returns nil.
func (s *Syncer) Flush(ctx context.Context) error {
	return nil
}

func (s *Syncer) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("User-Agent", "recall-client/1.0")
	if strings.TrimSpace(s.sourceID) != "" {
		req.Header.Set("X-Recall-Source-ID", s.sourceID)
	}
}

// StoreListItem represents summary information for a store.
// Used by Syncer.ListStores for remote store listing.
//
// Note: Similar types exist in internal/sync/types.go for HTTPClient use.
// The duplication exists due to import cycle constraints between packages.
type StoreListItem struct {
	ID           string `json:"id"`
	RecordCount  int64  `json:"record_count"`
	LastAccessed string `json:"last_accessed"`
	SizeBytes    int64  `json:"size_bytes"`
	Description  string `json:"description,omitempty"`
}

// StoreListResult contains the list of stores.
type StoreListResult struct {
	Stores []StoreListItem `json:"stores"`
	Total  int             `json:"total"`
}

// StoreInfo contains detailed information about a store.
type StoreInfo struct {
	ID           string          `json:"id"`
	Created      string          `json:"created"`
	LastAccessed string          `json:"last_accessed"`
	Description  string          `json:"description,omitempty"`
	SizeBytes    int64           `json:"size_bytes"`
	Stats        StoreDetailStats `json:"stats"`
}

// StoreDetailStats contains detailed statistics for a store.
type StoreDetailStats struct {
	TotalLore         int64            `json:"total_lore"`
	ActiveLore        int64            `json:"active_lore"`
	DeletedLore       int64            `json:"deleted_lore"`
	EmbeddingStats    EmbeddingStats   `json:"embedding_stats"`
	CategoryStats     map[string]int64 `json:"category_stats"`
	QualityStats      StoreQualityStats `json:"quality_stats"`
	UniqueSourceCount int64            `json:"unique_source_count"`
	StatsAsOf         string           `json:"stats_as_of"`
}

// EmbeddingStats contains embedding generation statistics.
type EmbeddingStats struct {
	Complete int64 `json:"complete"`
	Pending  int64 `json:"pending"`
	Failed   int64 `json:"failed"`
}

// StoreQualityStats contains lore quality metrics.
type StoreQualityStats struct {
	AverageConfidence   float64 `json:"average_confidence"`
	ValidatedCount      int64   `json:"validated_count"`
	HighConfidenceCount int64   `json:"high_confidence_count"`
	LowConfidenceCount  int64   `json:"low_confidence_count"`
}

// ListStores returns all available stores from Engram.
// If prefix is non-empty, filters stores by ID prefix (client-side filtering).
func (s *Syncer) ListStores(ctx context.Context, prefix string) (*StoreListResult, error) {
	url := s.engramURL + "/api/v1/stores"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("list stores: create request: %w", err)
	}
	s.setHeaders(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list stores: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list stores: HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var result StoreListResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("list stores: decode response: %w", err)
	}

	// Apply prefix filter client-side if specified
	if prefix != "" {
		filtered := make([]StoreListItem, 0)
		for _, store := range result.Stores {
			if strings.HasPrefix(store.ID, prefix) {
				filtered = append(filtered, store)
			}
		}
		result.Stores = filtered
		result.Total = len(filtered)
	}

	return &result, nil
}

// GetStoreInfo returns detailed information about a specific store.
func (s *Syncer) GetStoreInfo(ctx context.Context, storeID string) (*StoreInfo, error) {
	// URL-encode the store ID (handles "/" in path-style IDs)
	encodedID := encodeStoreID(storeID)
	url := fmt.Sprintf("%s/api/v1/stores/%s", s.engramURL, encodedID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("get store info: create request: %w", err)
	}
	s.setHeaders(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get store info: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("store not found: %s", storeID)
	}
	if resp.StatusCode == http.StatusBadRequest {
		return nil, fmt.Errorf("invalid store ID: %s", storeID)
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get store info: HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var result StoreInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("get store info: decode response: %w", err)
	}

	return &result, nil
}
