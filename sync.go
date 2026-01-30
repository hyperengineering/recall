package recall

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Syncer handles synchronization with the Engram central service.
type Syncer struct {
	store     *Store
	engramURL string
	apiKey    string
	sourceID  string
	client    *http.Client
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
	req, err := http.NewRequestWithContext(ctx, "POST", s.engramURL+"/api/v1/lore", bytes.NewReader(body))
	if err != nil {
		return s.failEntries(entries, err.Error())
	}
	s.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return s.failEntries(entries, err.Error())
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		errMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
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
	// Decode feedback payloads
	feedbackDTOs := make([]engramFeedbackEntryDTO, 0, len(entries))
	for _, e := range entries {
		var payload FeedbackQueuePayload
		if e.Payload != "" {
			if err := json.Unmarshal([]byte(e.Payload), &payload); err != nil {
				// Skip malformed entries silently to prevent infinite retry loops
				// on corrupted data - these entries will be cleared with the batch
				continue
			}
		}
		feedbackDTOs = append(feedbackDTOs, engramFeedbackEntryDTO{
			LoreID: e.LoreID,
			Type:   payload.Outcome,
		})
	}

	if len(feedbackDTOs) == 0 {
		// All entries were malformed; clear them
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
	req, err := http.NewRequestWithContext(ctx, "POST", s.engramURL+"/api/v1/lore/feedback", bytes.NewReader(body))
	if err != nil {
		return s.failEntries(entries, err.Error())
	}
	s.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return s.failEntries(entries, err.Error())
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		errMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
		return s.failEntries(entries, errMsg)
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
func parseRFC3339(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

// Pull fetches updates from Engram.
func (s *Syncer) Pull(ctx context.Context) error {
	stats, err := s.store.Stats()
	if err != nil {
		return err
	}

	var since string
	if !stats.LastSync.IsZero() {
		since = stats.LastSync.Format(time.RFC3339)
	}

	url := s.engramURL + "/api/v1/lore/delta"
	if since != "" {
		url += "?since=" + since
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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
		return fmt.Errorf("sync delta: get last_sync: %w", err)
	}
	if lastSync == "" {
		return fmt.Errorf("sync delta: no last_sync found - run bootstrap first")
	}

	// 2. Fetch delta from Engram
	url := fmt.Sprintf("%s/api/v1/lore/delta?since=%s", s.engramURL, lastSync)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("sync delta: create request: %w", err)
	}
	s.setHeaders(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("sync delta: fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sync delta: HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var delta engramDeltaResponse
	if err := json.NewDecoder(resp.Body).Decode(&delta); err != nil {
		return fmt.Errorf("sync delta: decode response: %w", err)
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
			return fmt.Errorf("sync delta: upsert lore %s: %w", entry.ID, err)
		}
	}

	// 4. Delete entries from deleted_ids
	for _, id := range delta.DeletedIDs {
		if err := s.store.DeleteLoreByID(id); err != nil {
			return fmt.Errorf("sync delta: delete lore %s: %w", id, err)
		}
	}

	// 5. Update last_sync with AsOf timestamp
	if delta.AsOf != "" {
		if err := s.store.SetMetadata("last_sync", delta.AsOf); err != nil {
			return fmt.Errorf("sync delta: update last_sync: %w", err)
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
	req, err := http.NewRequestWithContext(ctx, "GET", s.engramURL+"/api/v1/lore/snapshot", nil)
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

	req, err := http.NewRequestWithContext(ctx, "POST", s.engramURL+"/api/v1/lore", bytes.NewReader(body))
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
