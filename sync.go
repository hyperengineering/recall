package recall

import (
	"bytes"
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

// SetStoreID sets the store context for multi-store sync operations.
// When set, sync operations use store-prefixed API paths (e.g., /api/v1/stores/{storeID}/lore).
// When empty, operations use the original /api/v1/lore/* paths for backward compatibility.
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

// lorePath returns the API path for lore operations, considering store context.
func (s *Syncer) lorePath() string {
	if s.storeID == "" {
		return "/api/v1/lore"
	}
	return fmt.Sprintf("/api/v1/stores/%s/lore", encodeStoreID(s.storeID))
}

// feedbackPath returns the API path for feedback operations, considering store context.
func (s *Syncer) feedbackPath() string {
	if s.storeID == "" {
		return "/api/v1/lore/feedback"
	}
	return fmt.Sprintf("/api/v1/stores/%s/lore/feedback", encodeStoreID(s.storeID))
}

// deltaPath returns the API path for delta operations, considering store context.
func (s *Syncer) deltaPath() string {
	if s.storeID == "" {
		return "/api/v1/lore/delta"
	}
	return fmt.Sprintf("/api/v1/stores/%s/lore/delta", encodeStoreID(s.storeID))
}

// snapshotPath returns the API path for snapshot operations, considering store context.
func (s *Syncer) snapshotPath() string {
	if s.storeID == "" {
		return "/api/v1/lore/snapshot"
	}
	return fmt.Sprintf("/api/v1/stores/%s/lore/snapshot", encodeStoreID(s.storeID))
}

// engramHealthResponse represents the Engram health check response.
type engramHealthResponse struct {
	Status         string `json:"status"`
	Version        string `json:"version"`
	EmbeddingModel string `json:"embedding_model"`
	LoreCount      int    `json:"lore_count"`
	LastSnapshot   string `json:"last_snapshot"`
}

// engramIngestRequest represents a batch of lore to ingest.
type engramIngestRequest struct {
	SourceID string          `json:"source_id"`
	Lore     []engramLoreDTO `json:"lore"`
	Flush    bool            `json:"flush,omitempty"`
}

// engramLoreDTO represents lore in the Engram API format.
// Used for both push (ingest) and delta (pull) operations.
// Note: This type partially mirrors internal/sync.LoreEntry. See engramDeltaResponse
// comment for explanation of why duplication exists.
type engramLoreDTO struct {
	ID              string   `json:"id"`
	Content         string   `json:"content"`
	Context         string   `json:"context,omitempty"`
	Category        string   `json:"category"`
	Confidence      float64  `json:"confidence"`
	Sources         []string `json:"sources"`
	ValidationCount int      `json:"validation_count"`
	SourceID        string   `json:"source_id,omitempty"`
	EmbeddingStatus string   `json:"embedding_status"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at,omitempty"`
}

// engramDeltaResponse represents the delta sync response.
// Note: This type mirrors internal/sync.DeltaResult. The duplication exists because
// internal/sync imports the recall package (for recall.SyncError), creating an import
// cycle that prevents sharing types. A future refactor could extract shared types
// to a separate package (e.g., internal/types) to eliminate this duplication.
type engramDeltaResponse struct {
	Lore       []engramLoreDTO `json:"lore"`
	DeletedIDs []string        `json:"deleted_ids"`
	AsOf       string          `json:"as_of"`
}

// engramFeedbackRequest represents feedback to send.
type engramFeedbackRequest struct {
	SourceID string                   `json:"source_id"`
	Feedback []engramFeedbackEntryDTO `json:"feedback"`
}

// engramFeedbackEntryDTO represents a single feedback entry.
type engramFeedbackEntryDTO struct {
	LoreID string `json:"lore_id"`
	Type   string `json:"type"` // helpful | not_relevant | incorrect
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

// Push sends pending lore and feedback to Engram.
//
// Process:
//  1. Read all pending entries from sync_queue
//  2. Group by operation type (INSERT vs FEEDBACK)
//  3. For INSERT: fetch lore, call POST /api/v1/lore
//  4. For FEEDBACK: decode payloads, call POST /api/v1/lore/feedback
//  5. On success: delete queue entries, update synced_at
//  6. On failure: increment attempts, record error
//
// Returns nil if queue is empty.
// Returns SyncError if Engram is unreachable.
func (s *Syncer) Push(ctx context.Context) error {
	entries, err := s.store.PendingSyncEntries()
	if err != nil {
		return fmt.Errorf("push: read queue: %w", err)
	}

	if len(entries) == 0 {
		return nil
	}

	// Group by operation
	var insertEntries []SyncQueueEntry
	var feedbackEntries []SyncQueueEntry
	for _, e := range entries {
		switch e.Operation {
		case "INSERT":
			insertEntries = append(insertEntries, e)
		case "FEEDBACK":
			feedbackEntries = append(feedbackEntries, e)
		}
	}

	var pushErr error

	// Push INSERT operations
	if len(insertEntries) > 0 {
		if err := s.pushLoreEntries(ctx, insertEntries); err != nil {
			pushErr = err
		}
	}

	// Push FEEDBACK operations
	if len(feedbackEntries) > 0 {
		if err := s.pushFeedbackEntries(ctx, feedbackEntries); err != nil {
			if pushErr == nil {
				pushErr = err
			}
		}
	}

	return pushErr
}

func (s *Syncer) pushLoreEntries(ctx context.Context, entries []SyncQueueEntry) error {
	// Collect lore IDs
	loreIDs := make([]string, len(entries))
	for i, e := range entries {
		loreIDs[i] = e.LoreID
	}

	// Fetch lore
	loreList, err := s.store.GetLoreByIDs(loreIDs)
	if err != nil {
		return fmt.Errorf("fetch lore: %w", err)
	}

	if len(loreList) == 0 {
		// Lore was deleted; clear queue entries
		queueIDs := make([]int64, len(entries))
		for i, e := range entries {
			queueIDs[i] = e.ID
		}
		return s.store.CompleteSyncEntries(queueIDs, nil)
	}

	// Convert to DTO format
	loreDTOs := make([]engramLoreDTO, len(loreList))
	for i, l := range loreList {
		loreDTOs[i] = engramLoreDTO{
			ID:         l.ID,
			Content:    l.Content,
			Context:    l.Context,
			Category:   string(l.Category),
			Confidence: l.Confidence,
			CreatedAt:  l.CreatedAt.Format(time.RFC3339),
		}
	}

	// Build request
	body, err := json.Marshal(engramIngestRequest{
		SourceID: s.sourceID,
		Lore:     loreDTOs,
	})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// Send to Engram
	apiURL := s.engramURL + s.lorePath()
	s.debug.LogRequest("POST", apiURL, body)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		s.debug.LogError("push_lore", err)
		return s.failEntries(entries, err.Error())
	}
	s.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		s.debug.LogError("push_lore", err)
		return s.failEntries(entries, err.Error())
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	s.debug.LogResponse(resp.StatusCode, resp.Status, respBody)

	if resp.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
		s.debug.LogError("push_lore", fmt.Errorf("%s", errMsg))
		return s.failEntries(entries, errMsg)
	}

	// Success: clear queue entries
	queueIDs := make([]int64, len(entries))
	syncedLoreIDs := make([]string, len(loreList))
	for i, e := range entries {
		queueIDs[i] = e.ID
	}
	for i, l := range loreList {
		syncedLoreIDs[i] = l.ID
	}

	return s.store.CompleteSyncEntries(queueIDs, syncedLoreIDs)
}

func (s *Syncer) pushFeedbackEntries(ctx context.Context, entries []SyncQueueEntry) error {
	// Decode feedback payloads and filter out entries for unsynced lore
	feedbackDTOs := make([]engramFeedbackEntryDTO, 0, len(entries))
	var unsyncedEntryIDs []int64 // Track entries to remove without sending

	for _, e := range entries {
		// Belt and suspenders: Check if lore has been synced to central.
		// If synced_at IS NULL, the lore doesn't exist on central yet,
		// so sending feedback would cause HTTP 404 errors.
		lore, err := s.store.Get(e.LoreID)
		if err != nil {
			// Lore not found (deleted?) - remove orphaned queue entry
			unsyncedEntryIDs = append(unsyncedEntryIDs, e.ID)
			continue
		}
		if lore.SyncedAt == nil {
			// Lore exists locally but hasn't been synced yet - remove queue entry
			unsyncedEntryIDs = append(unsyncedEntryIDs, e.ID)
			continue
		}

		var payload FeedbackQueuePayload
		if e.Payload == "" {
			// Skip entries with empty payload to prevent validation errors
			continue
		}
		if err := json.Unmarshal([]byte(e.Payload), &payload); err != nil {
			// Skip malformed entries silently to prevent infinite retry loops
			// on corrupted data - these entries will be cleared with the batch
			continue
		}
		// Validate outcome is a valid FeedbackType per OpenAPI spec
		if payload.Outcome != string(FeedbackHelpful) &&
			payload.Outcome != string(FeedbackIncorrect) &&
			payload.Outcome != string(FeedbackNotRelevant) {
			// Skip entries with invalid outcome to prevent 422 validation errors
			continue
		}
		feedbackDTOs = append(feedbackDTOs, engramFeedbackEntryDTO{
			LoreID: e.LoreID,
			Type:   payload.Outcome,
		})
	}

	// Remove orphaned feedback entries for unsynced lore from queue
	for _, id := range unsyncedEntryIDs {
		_ = s.store.DeleteSyncEntry(id) // Best effort cleanup
	}

	if len(feedbackDTOs) == 0 {
		// All entries were malformed or for unsynced lore; clear them
		queueIDs := make([]int64, len(entries))
		for i, e := range entries {
			queueIDs[i] = e.ID
		}
		return s.store.CompleteSyncEntries(queueIDs, nil)
	}

	// Build request
	body, err := json.Marshal(engramFeedbackRequest{
		SourceID: s.sourceID,
		Feedback: feedbackDTOs,
	})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// Send to Engram
	apiURL := s.engramURL + s.feedbackPath()
	s.debug.LogRequest("POST", apiURL, body)
	s.debug.LogSync("push_feedback", fmt.Sprintf("sending %d feedback entries", len(feedbackDTOs)))

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		s.debug.LogError("push_feedback", err)
		return s.failEntries(entries, err.Error())
	}
	s.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		s.debug.LogError("push_feedback", err)
		return s.failEntries(entries, err.Error())
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	s.debug.LogResponse(resp.StatusCode, resp.Status, respBody)

	if resp.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody))
		s.debug.LogError("push_feedback", fmt.Errorf("%s", errMsg))
		return s.failEntries(entries, truncate(errMsg, 200))
	}

	// Success: clear queue entries (no synced_at update for feedback)
	queueIDs := make([]int64, len(entries))
	for i, e := range entries {
		queueIDs[i] = e.ID
	}

	return s.store.CompleteSyncEntries(queueIDs, nil)
}

func (s *Syncer) failEntries(entries []SyncQueueEntry, errMsg string) error {
	queueIDs := make([]int64, len(entries))
	for i, e := range entries {
		queueIDs[i] = e.ID
	}
	if err := s.store.FailSyncEntries(queueIDs, errMsg); err != nil {
		return fmt.Errorf("record failure: %w", err)
	}
	return &SyncError{Operation: "push", Err: fmt.Errorf("%s", errMsg)}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// parseRFC3339 parses an RFC3339 timestamp string, returning the zero time
// if the string is empty or malformed. This is intentional: delta sync entries
// may have missing timestamps, and we prefer zero-value over errors.
//
// Examples:
//
//	parseRFC3339("2024-01-15T10:30:00Z") // returns parsed time.Time
//	parseRFC3339("")                      // returns time.Time{} (zero value)
//	parseRFC3339("invalid")               // returns time.Time{} (no error)
func parseRFC3339(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

// Pull fetches updates from Engram.
//
// Deprecated: Use SyncDelta() instead, which both fetches and applies delta
// changes to the local store. Pull() only fetches but does not apply changes.
// This method will be removed in v2.0.
//
// Requires prior Bootstrap() - returns error if last_sync metadata is not set.
func (s *Syncer) Pull(ctx context.Context) error {
	// Require last_sync to prevent calling /delta without required since parameter
	lastSync, err := s.store.GetMetadata("last_sync")
	if err != nil {
		return fmt.Errorf("pull: get last_sync: %w", err)
	}
	if lastSync == "" {
		return fmt.Errorf("pull: no last_sync found (run 'recall sync bootstrap' first)")
	}

	apiURL := s.engramURL + s.deltaPath() + "?since=" + lastSync

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return err
	}
	s.setHeaders(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pull failed: %s - %s", resp.Status, string(respBody))
	}

	var delta engramDeltaResponse
	if err := json.NewDecoder(resp.Body).Decode(&delta); err != nil {
		return err
	}

	// Pull() fetches delta but does not apply it to local store.
	// This is intentional: Pull() is part of the legacy Sync() flow which
	// only pushed data. For full delta synchronization, use SyncDelta()
	// which both fetches and applies updates.
	_ = delta // Response available for future use if needed

	return nil
}

// SyncDelta fetches and applies incremental changes from Engram.
//
// Process:
//  1. Check last_sync metadata (requires prior Bootstrap)
//  2. Fetch delta from Engram using GET /api/v1/lore/delta?since={last_sync}
//  3. Upsert new/updated lore entries to local store
//  4. Delete entries matching deleted_ids
//  5. Update last_sync metadata with AsOf timestamp
//
// Returns error if last_sync is empty (client must Bootstrap first).
func (s *Syncer) SyncDelta(ctx context.Context) error {
	// 1. Get last_sync - require prior bootstrap
	lastSync, err := s.store.GetMetadata("last_sync")
	if err != nil {
		return fmt.Errorf("delta: get last_sync: %w", err)
	}
	if lastSync == "" {
		return fmt.Errorf("delta: no last_sync found (run 'recall sync bootstrap' first)")
	}

	// 2. Fetch delta from Engram
	apiURL := fmt.Sprintf("%s%s?since=%s", s.engramURL, s.deltaPath(), lastSync)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("delta: create request: %w", err)
	}
	s.setHeaders(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("delta: fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delta: HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var delta engramDeltaResponse
	if err := json.NewDecoder(resp.Body).Decode(&delta); err != nil {
		return fmt.Errorf("delta: decode response: %w", err)
	}

	// 3. Upsert new/updated lore entries
	for _, entry := range delta.Lore {
		// Determine embedding status: use value from delta, default to "ready"
		embeddingStatus := entry.EmbeddingStatus
		if embeddingStatus == "" {
			embeddingStatus = "ready" // Delta entries from Engram have embeddings
		}

		lore := &Lore{
			ID:              entry.ID,
			Content:         entry.Content,
			Context:         entry.Context,
			Category:        Category(entry.Category),
			Confidence:      entry.Confidence,
			ValidationCount: entry.ValidationCount,
			Sources:         entry.Sources,
			SourceID:        entry.SourceID,
			EmbeddingStatus: embeddingStatus,
		}

		// Parse timestamps (zero values for empty/invalid strings)
		lore.CreatedAt = parseRFC3339(entry.CreatedAt)
		lore.UpdatedAt = parseRFC3339(entry.UpdatedAt)

		if err := s.store.UpsertLore(lore); err != nil {
			return fmt.Errorf("delta: upsert lore %s: %w", entry.ID, err)
		}
	}

	// 4. Delete entries from deleted_ids
	for _, id := range delta.DeletedIDs {
		if err := s.store.DeleteLoreByID(id); err != nil {
			return fmt.Errorf("delta: delete lore %s: %w", id, err)
		}
	}

	// 5. Update last_sync with AsOf timestamp
	if delta.AsOf != "" {
		if err := s.store.SetMetadata("last_sync", delta.AsOf); err != nil {
			return fmt.Errorf("delta: update last_sync: %w", err)
		}
	}

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
// Sets the flush:true flag in the request to signal Engram this is a shutdown flush.
func (s *Syncer) Flush(ctx context.Context) error {
	entries, err := s.store.PendingSyncEntries()
	if err != nil {
		return err
	}

	// Filter for INSERT entries only (lore)
	var insertEntries []SyncQueueEntry
	for _, e := range entries {
		if e.Operation == "INSERT" {
			insertEntries = append(insertEntries, e)
		}
	}

	if len(insertEntries) == 0 {
		return nil
	}

	// Collect lore IDs
	loreIDs := make([]string, len(insertEntries))
	for i, e := range insertEntries {
		loreIDs[i] = e.LoreID
	}

	// Fetch lore
	loreList, err := s.store.GetLoreByIDs(loreIDs)
	if err != nil {
		return err
	}

	if len(loreList) == 0 {
		return nil
	}

	loreDTOs := make([]engramLoreDTO, len(loreList))
	for i, l := range loreList {
		loreDTOs[i] = engramLoreDTO{
			ID:         l.ID,
			Content:    l.Content,
			Context:    l.Context,
			Category:   string(l.Category),
			Confidence: l.Confidence,
			CreatedAt:  l.CreatedAt.Format(time.RFC3339),
		}
	}

	body, err := json.Marshal(engramIngestRequest{
		SourceID: s.sourceID,
		Lore:     loreDTOs,
		Flush:    true,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.engramURL+s.lorePath(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	s.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("flush failed: %s", resp.Status)
	}

	// Clear queue entries and mark synced
	queueIDs := make([]int64, len(insertEntries))
	for i, e := range insertEntries {
		queueIDs[i] = e.ID
	}
	syncedLoreIDs := make([]string, len(loreList))
	for i, l := range loreList {
		syncedLoreIDs[i] = l.ID
	}

	return s.store.CompleteSyncEntries(queueIDs, syncedLoreIDs)
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
