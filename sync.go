package recall

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Syncer handles synchronization with the Engram central service.
type Syncer struct {
	store     *Store
	engramURL string
	apiKey    string
	client    *http.Client
}

// NewSyncer creates a new syncer.
func NewSyncer(store *Store, engramURL, apiKey string) *Syncer {
	return &Syncer{
		store:     store,
		engramURL: engramURL,
		apiKey:    apiKey,
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
	ID         string  `json:"id"`
	Content    string  `json:"content"`
	Context    string  `json:"context,omitempty"`
	Category   string  `json:"category"`
	Confidence float64 `json:"confidence"`
	CreatedAt  string  `json:"created_at"`
}

// engramIngestResponse represents the ingest response.
type engramIngestResponse struct {
	Accepted int      `json:"accepted"`
	Merged   int      `json:"merged"`
	Rejected int      `json:"rejected"`
	Errors   []string `json:"errors"`
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
	ID      string `json:"id"`
	Outcome string `json:"outcome"` // helpful | not_relevant | incorrect
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("health check failed: %s", resp.Status)
	}

	var health engramHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, err
	}

	return &health, nil
}

// Push sends pending lore to Engram.
func (s *Syncer) Push(ctx context.Context) error {
	unsynced, err := s.store.Unsynced()
	if err != nil {
		return err
	}

	if len(unsynced) == 0 {
		return nil
	}

	// Convert to DTO format
	loreDTOs := make([]engramLoreDTO, len(unsynced))
	for i, l := range unsynced {
		loreDTOs[i] = engramLoreDTO{
			ID:         l.ID,
			Content:    l.Content,
			Context:    l.Context,
			Category:   string(l.Category),
			Confidence: l.Confidence,
			CreatedAt:  l.CreatedAt.Format(time.RFC3339),
		}
	}

	stats, _ := s.store.Stats()
	sourceID := ""
	if stats != nil {
		sourceID = stats.SchemaVersion // Use a proper source ID in production
	}

	body, err := json.Marshal(engramIngestRequest{
		SourceID: sourceID,
		Lore:     loreDTOs,
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("push failed: %s - %s", resp.Status, string(respBody))
	}

	var result engramIngestResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	// Mark as synced
	ids := make([]string, len(unsynced))
	for i, l := range unsynced {
		ids[i] = l.ID
	}

	return s.store.MarkSynced(ids, time.Now().UTC())
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pull failed: %s - %s", resp.Status, string(respBody))
	}

	var delta engramDeltaResponse
	if err := json.NewDecoder(resp.Body).Decode(&delta); err != nil {
		return err
	}

	// TODO: Apply delta updates to local store
	// This would involve upserting the lore entries

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
	defer resp.Body.Close()

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
func (s *Syncer) Flush(ctx context.Context) error {
	unsynced, err := s.store.Unsynced()
	if err != nil {
		return err
	}

	if len(unsynced) == 0 {
		return nil
	}

	loreDTOs := make([]engramLoreDTO, len(unsynced))
	for i, l := range unsynced {
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
		SourceID: "",
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("flush failed: %s", resp.Status)
	}

	ids := make([]string, len(unsynced))
	for i, l := range unsynced {
		ids[i] = l.ID
	}

	return s.store.MarkSynced(ids, time.Now().UTC())
}

func (s *Syncer) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("User-Agent", "recall-client/1.0")
}
