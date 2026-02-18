package recall

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// Client is the main interface for interacting with lore.
type Client struct {
	store    *Store
	syncer   *Syncer
	session  *Session
	searcher Searcher
	config   Config
	debug    *DebugLogger

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

	// Create debug logger if enabled
	debug, err := NewDebugLogger(cfg.Debug, cfg.DebugLogPath)
	if err != nil {
		return nil, fmt.Errorf("client: %w", err)
	}

	c := &Client{
		store:    store,
		session:  NewSession(),
		searcher: &BruteForceSearcher{},
		config:   cfg,
		debug:    debug,
		stopSync: make(chan struct{}),
		syncDone: make(chan struct{}),
	}

	if !cfg.IsOffline() {
		c.syncer = NewSyncer(store, cfg.EngramURL, cfg.APIKey, cfg.SourceID)
		c.syncer.SetDebugLogger(debug)
	}

	// Start background sync if enabled
	if c.syncer != nil && cfg.AutoSync {
		go c.backgroundSync()
	} else {
		// No background sync - signal done immediately to avoid 5s timeout in Close()
		close(c.syncDone)
	}

	return c, nil
}

// RecordOption configures optional parameters for Record.
type RecordOption func(*recordOptions)

type recordOptions struct {
	context    string
	confidence *float64 // nil means use default (0.5)
}

// WithContext sets the context for the lore entry.
func WithContext(ctx string) RecordOption {
	return func(o *recordOptions) {
		o.context = ctx
	}
}

// WithConfidence sets the confidence for the lore entry.
// Must be in range [0.0, 1.0].
func WithConfidence(c float64) RecordOption {
	return func(o *recordOptions) {
		o.confidence = &c
	}
}

// Record captures new lore with content and category.
// Optional parameters can be provided via WithContext and WithConfidence.
func (c *Client) Record(content string, category Category, opts ...RecordOption) (*Lore, error) {
	// Apply options
	options := recordOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	// Validate inputs (fail fast)
	if content == "" {
		return nil, &ValidationError{Field: "Content", Message: "cannot be empty"}
	}
	if len(content) > MaxContentLength {
		return nil, &ValidationError{Field: "Content", Message: "exceeds 4000 character limit"}
	}
	if len(options.context) > MaxContextLength {
		return nil, &ValidationError{Field: "Context", Message: "exceeds 1000 character limit"}
	}
	if !category.IsValid() {
		return nil, &ValidationError{Field: "Category", Message: "invalid: must be one of " + validCategoriesString()}
	}

	// Validate confidence if provided
	confidence := ConfidenceDefault
	if options.confidence != nil {
		if *options.confidence < ConfidenceMin || *options.confidence > ConfidenceMax {
			return nil, &ValidationError{Field: "Confidence", Message: "must be between 0.0 and 1.0"}
		}
		confidence = *options.confidence
	}

	// Build lore entry
	now := time.Now().UTC()
	lore := &Lore{
		ID:         ulid.Make().String(),
		Content:    content,
		Category:   category,
		Context:    options.context,
		Confidence: confidence,
		SourceID:   c.config.SourceID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// Atomically insert lore + sync queue entry
	if err := c.store.InsertLore(lore); err != nil {
		return nil, fmt.Errorf("client: record: %w", err)
	}

	return lore, nil
}

// validCategoriesString returns a comma-separated list of valid categories.
func validCategoriesString() string {
	cats := ValidCategories()
	result := ""
	for i, cat := range cats {
		if i > 0 {
			result += ", "
		}
		result += string(cat)
	}
	return result
}

// RecordLegacy captures new lore using the legacy API.
// Deprecated: Use Record(content, category, opts...) instead.
func (c *Client) RecordLegacy(ctx context.Context, params RecordParams) (*Lore, error) {
	opts := []RecordOption{}
	if params.Context != "" {
		opts = append(opts, WithContext(params.Context))
	}
	if params.Confidence != 0 {
		opts = append(opts, WithConfidence(params.Confidence))
	}
	return c.Record(params.Content, params.Category, opts...)
}

// Query retrieves relevant lore based on semantic similarity.
//
// Query path selection:
//   - If QueryEmbedding is provided: performs semantic similarity search,
//     ranking results by cosine similarity to the query vector.
//   - If QueryEmbedding is empty: falls back to basic filtering by category
//     and confidence, returning results in creation order.
func (c *Client) Query(ctx context.Context, params QueryParams) (*QueryResult, error) {
	// Set defaults only when both K and MinConfidence are unset
	if params.K == 0 {
		params.K = 5
	}
	if params.MinConfidence == nil {
		defaultConfidence := 0.5
		params.MinConfidence = &defaultConfidence
	}

	var lore []Lore
	var err error

	if len(params.QueryEmbedding) > 0 {
		lore, err = c.queryWithSimilarity(params)
	} else {
		// No embedding provided, fall back to basic query
		lore, err = c.store.Query(params)
		if err != nil {
			return nil, fmt.Errorf("client: query: %w", err)
		}

		// Apply K limit (basic query doesn't rank by similarity)
		if params.K > 0 && len(lore) > params.K {
			lore = lore[:params.K]
		}
	}
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

// queryWithSimilarity performs semantic similarity search using the query embedding.
// It retrieves candidates matching filters, then ranks them by cosine similarity.
func (c *Client) queryWithSimilarity(params QueryParams) ([]Lore, error) {
	// Get all lore with embeddings that match filters
	lore, err := c.store.QueryWithEmbeddings(params)
	if err != nil {
		return nil, fmt.Errorf("client: query: %w", err)
	}

	// Convert to candidates for similarity search
	candidates := make([]CandidateLore, 0, len(lore))
	loreByID := make(map[string]Lore, len(lore))
	for _, l := range lore {
		if len(l.Embedding) > 0 {
			embedding := UnpackFloat32(l.Embedding)
			if embedding != nil {
				candidates = append(candidates, CandidateLore{
					ID:        l.ID,
					Embedding: embedding,
				})
				loreByID[l.ID] = l
			}
		}
	}

	// Perform similarity search
	scored := c.searcher.Search(params.QueryEmbedding, candidates, params.K)

	// Rebuild lore slice in similarity order
	result := make([]Lore, 0, len(scored))
	for _, s := range scored {
		if l, ok := loreByID[s.ID]; ok {
			result = append(result, l)
		}
	}

	return result, nil
}

// Feedback applies feedback to a single lore entry, adjusting its confidence.
//
// The ref parameter can be:
//   - An L-ref (L1, L2, etc.) from the current session
//   - A lore ID (26-character ULID) directly
//
// Confidence adjustments:
//   - Helpful:     +0.08
//   - Incorrect:   -0.15
//   - NotRelevant:  0.00 (unchanged)
//
// Returns the updated Lore entry with new confidence value.
// Returns ErrNotFound if:
//   - L-ref does not exist in the current session
//   - Lore ID does not exist in the store
func (c *Client) Feedback(ref string, ft FeedbackType) (*Lore, error) {
	var loreID string

	if isLRef(ref) {
		// Try direct resolve first
		id, ok := c.session.Resolve(ref)
		if !ok {
			// Try fuzzy match as fallback
			contentLookup := func(id string) string {
				lore, err := c.store.Get(id)
				if err != nil {
					return ""
				}
				return lore.Content
			}
			id, ok = c.session.FuzzyMatch(ref, contentLookup)
			if !ok {
				return nil, ErrNotFound
			}
		}
		loreID = id
	} else {
		// Assume it's a lore ID - validate it exists
		loreID = ref
	}

	delta := feedbackDelta(ft)
	isHelpful := ft == Helpful
	lore, err := c.store.ApplyFeedback(loreID, delta, isHelpful)
	if err != nil {
		return nil, fmt.Errorf("client: feedback: %w", err)
	}
	return lore, nil
}

// isLRef returns true if ref matches L-ref format (L followed by digits).
func isLRef(ref string) bool {
	if len(ref) < 2 || ref[0] != 'L' {
		return false
	}
	for _, ch := range ref[1:] {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// FeedbackBatch provides batch feedback on recalled lore.
// Deprecated: Use Feedback() for single-entry feedback.
func (c *Client) FeedbackBatch(ctx context.Context, params FeedbackParams) (*FeedbackResult, error) {
	return c.store.ApplyFeedbackBatch(c.session, params)
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
// Delegates to SyncDelta for incremental change-log based sync.
func (c *Client) SyncPull(ctx context.Context) error {
	if c.syncer == nil {
		return ErrOffline
	}
	return c.syncer.SyncDelta(ctx)
}

// SyncDelta fetches and applies incremental changes from Engram.
//
// This performs an efficient delta sync, fetching only changes since the last
// sync rather than downloading the full database. Requires prior Bootstrap.
//
// Returns ErrOffline if Engram is not configured.
// Returns error if Bootstrap has not been run (no last_sync timestamp).
func (c *Client) SyncDelta(ctx context.Context) error {
	if c.syncer == nil {
		return ErrOffline
	}
	return c.syncer.SyncDelta(ctx)
}

// Bootstrap downloads a full snapshot from Engram and replaces the local lore.
//
// This is used to initialize or refresh the local database with the complete
// knowledge base from Engram, including embeddings for similarity queries.
//
// The bootstrap process:
//  1. Validates connectivity via health check
//  2. Checks embedding model compatibility (aborts if models mismatch)
//  3. Downloads the full snapshot
//  4. Atomically replaces local lore (preserves data on failure)
//  5. Updates metadata (embedding_model, last_sync)
//
// Returns ErrOffline if Engram is not configured.
// Returns ErrModelMismatch if local embedding model differs from remote.
func (c *Client) Bootstrap(ctx context.Context) error {
	if c.syncer == nil {
		return ErrOffline
	}
	return c.syncer.Bootstrap(ctx)
}

// Reinitialize replaces the local database with a fresh copy from Engram.
//
// The reinit process:
//  1. Check for pending sync entries (aborts if any exist)
//  2. Attempt to bootstrap from Engram
//  3. If Engram is unreachable and opts.AllowEmpty is true, create empty database
//  4. Return result with source, lore count, and timestamp
//
// Returns ErrPendingSyncExists if unsynced local changes exist.
// Returns ErrOffline if Engram is not configured and opts.AllowEmpty is false.
func (c *Client) Reinitialize(ctx context.Context, opts ReinitOptions) (*ReinitResult, error) {
	// 1. Check for pending sync entries
	pendingCount, err := c.store.HasPendingSync()
	if err != nil {
		return nil, fmt.Errorf("reinit: check pending sync: %w", err)
	}
	if pendingCount > 0 {
		return nil, ErrPendingSyncExists
	}

	// 2. Check if we're in offline mode
	if c.syncer == nil {
		if !opts.AllowEmpty {
			return nil, ErrOffline
		}
		// Create empty database
		return c.reinitEmpty()
	}

	// 3. Try to bootstrap from Engram
	err = c.syncer.Bootstrap(ctx)
	if err != nil {
		// Check if Engram is unreachable and we're allowed to create empty
		if opts.AllowEmpty {
			return c.reinitEmpty()
		}
		return nil, fmt.Errorf("reinit: bootstrap: %w", err)
	}

	// 4. Get stats for result
	stats, err := c.store.Stats()
	if err != nil {
		return nil, fmt.Errorf("reinit: get stats: %w", err)
	}

	return &ReinitResult{
		Source:    "engram",
		LoreCount: stats.LoreCount,
		Timestamp: time.Now().UTC(),
	}, nil
}

// reinitEmpty creates an empty database by clearing all lore entries.
func (c *Client) reinitEmpty() (*ReinitResult, error) {
	// Clear the database by replacing with an empty snapshot
	// Use ReplaceFromSnapshot with an empty reader to trigger the clear
	if err := c.store.ClearAllLore(); err != nil {
		return nil, fmt.Errorf("reinit: clear lore: %w", err)
	}

	return &ReinitResult{
		Source:    "empty",
		LoreCount: 0,
		Timestamp: time.Now().UTC(),
	}, nil
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
		_ = c.syncer.Flush(ctx)
	}

	// Close debug logger
	if c.debug != nil {
		_ = c.debug.Close()
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
			// Create cancellable context
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

			// Run sync, but also listen for stop signal
			done := make(chan struct{})
			go func() {
				_ = c.syncer.Sync(ctx)
				close(done)
			}()

			select {
			case <-done:
				// Sync completed normally
			case <-c.stopSync:
				cancel() // Cancel in-flight HTTP requests
				<-done   // Wait for Sync to return
				return
			}
			cancel()
		}
	}
}

// ListStores returns all available stores from Engram.
// If prefix is non-empty, filters stores by ID prefix.
// Returns ErrOffline if Engram is not configured.
func (c *Client) ListStores(ctx context.Context, prefix string) (*StoreListResult, error) {
	if c.syncer == nil {
		return nil, ErrOffline
	}
	return c.syncer.ListStores(ctx, prefix)
}

// GetStoreInfo returns detailed information about a specific store.
// Returns ErrOffline if Engram is not configured.
func (c *Client) GetStoreInfo(ctx context.Context, storeID string) (*StoreInfo, error) {
	if c.syncer == nil {
		return nil, ErrOffline
	}
	return c.syncer.GetStoreInfo(ctx, storeID)
}
