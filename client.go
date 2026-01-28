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
		searcher: &BruteForceSearcher{},
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
