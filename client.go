package recall

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Client is the main interface for interacting with lore.
type Client struct {
	store   *Store
	syncer  *Syncer
	session *Session
	config  Config

	mu       sync.Mutex
	stopSync chan struct{}
	syncDone chan struct{}
}

// New creates a new Recall client.
func New(cfg Config) (*Client, error) {
	cfg = cfg.WithDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	store, err := NewStore(cfg.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("client: %w", err)
	}

	c := &Client{
		store:    store,
		session:  NewSession(),
		config:   cfg,
		stopSync: make(chan struct{}),
		syncDone: make(chan struct{}),
	}

	if cfg.EngramURL != "" && !cfg.OfflineMode {
		c.syncer = NewSyncer(store, cfg.EngramURL, cfg.APIKey)
	}

	// Start background sync if enabled
	if c.syncer != nil && cfg.AutoSync {
		go c.backgroundSync()
	}

	return c, nil
}

// Record captures new lore.
func (c *Client) Record(ctx context.Context, params RecordParams) (*Lore, error) {
	lore := Lore{
		Content:    params.Content,
		Context:    params.Context,
		Category:   params.Category,
		Confidence: params.Confidence,
		SourceID:   c.config.SourceID,
	}

	if lore.Confidence == 0 {
		lore.Confidence = ConfidenceDefault
	}

	return c.store.Record(lore)
}

// Query retrieves relevant lore based on semantic similarity.
func (c *Client) Query(ctx context.Context, params QueryParams) (*QueryResult, error) {
	// Set defaults
	if params.K == 0 {
		params.K = 5
	}
	if params.MinConfidence == 0 {
		params.MinConfidence = 0.5
	}

	lore, err := c.store.Query(params)
	if err != nil {
		return nil, err
	}

	// Track in session for feedback
	refs := make(map[string]string)
	for _, l := range lore {
		ref := c.session.Track(l.ID)
		refs[ref] = l.ID
	}

	return &QueryResult{Lore: lore, SessionRefs: refs}, nil
}

// Feedback provides feedback on recalled lore.
func (c *Client) Feedback(ctx context.Context, params FeedbackParams) (*FeedbackResult, error) {
	return c.store.ApplyFeedback(c.session, params)
}

// GetSessionLore returns all lore surfaced this session.
func (c *Client) GetSessionLore() []SessionLore {
	all := c.session.All()
	result := make([]SessionLore, 0, len(all))

	for ref, id := range all {
		lore, err := c.store.Get(id)
		if err != nil {
			continue
		}

		content := lore.Content
		if len(content) > 100 {
			content = content[:100] + "..."
		}

		result = append(result, SessionLore{
			SessionRef: ref,
			ID:         id,
			Content:    content,
			Category:   lore.Category,
			Confidence: lore.Confidence,
			Source:     "query",
		})
	}

	return result
}

// Sync synchronizes with Engram (if configured).
func (c *Client) Sync(ctx context.Context) error {
	if c.syncer == nil {
		return ErrOffline
	}
	return c.syncer.Sync(ctx)
}

// SyncPush pushes pending lore to Engram.
func (c *Client) SyncPush(ctx context.Context) error {
	if c.syncer == nil {
		return ErrOffline
	}
	return c.syncer.Push(ctx)
}

// SyncPull pulls updates from Engram.
func (c *Client) SyncPull(ctx context.Context) error {
	if c.syncer == nil {
		return ErrOffline
	}
	return c.syncer.Pull(ctx)
}

// Stats returns store statistics.
func (c *Client) Stats() (*StoreStats, error) {
	return c.store.Stats()
}

// HealthCheck returns the health status of the client.
func (c *Client) HealthCheck(ctx context.Context) HealthStatus {
	status := HealthStatus{
		Healthy: true,
		StoreOK: true,
	}

	// Check store
	_, err := c.store.Stats()
	if err != nil {
		status.StoreOK = false
		status.Healthy = false
		status.Error = err.Error()
		return status
	}

	// Check Engram connectivity
	if c.syncer != nil {
		_, err := c.syncer.Health(ctx)
		status.EngramReachable = err == nil
		if err != nil && status.Error == "" {
			status.Error = err.Error()
		}
	}

	return status
}

// Close closes the client and flushes pending changes.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Stop background sync
	close(c.stopSync)

	// Wait for sync to complete (with timeout)
	select {
	case <-c.syncDone:
	case <-time.After(5 * time.Second):
	}

	// Flush pending changes
	if c.syncer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		c.syncer.Flush(ctx)
	}

	return c.store.Close()
}

func (c *Client) backgroundSync() {
	defer close(c.syncDone)

	ticker := time.NewTicker(c.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopSync:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			c.syncer.Sync(ctx)
			cancel()
		}
	}
}
