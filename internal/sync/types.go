package sync

// HealthResponse from GET /api/v1/health
type HealthResponse struct {
	Status         string `json:"status"`
	Version        string `json:"version"`
	EmbeddingModel string `json:"embedding_model"`
	LoreCount      int    `json:"lore_count"`
	LastSnapshot   string `json:"last_snapshot"`
}

// PushLoreRequest for POST /api/v1/lore
type PushLoreRequest struct {
	SourceID string        `json:"source_id"`
	Lore     []LorePayload `json:"lore"`
	Flush    bool          `json:"flush,omitempty"`
}

// LorePayload represents a single lore entry for push operations.
type LorePayload struct {
	ID         string  `json:"id"`
	Content    string  `json:"content"`
	Context    string  `json:"context,omitempty"`
	Category   string  `json:"category"`
	Confidence float64 `json:"confidence"`
	CreatedAt  string  `json:"created_at"`
}

// PushLoreResponse from POST /api/v1/lore
type PushLoreResponse struct {
	Accepted int      `json:"accepted"`
	Merged   int      `json:"merged"`
	Rejected int      `json:"rejected"`
	Errors   []string `json:"errors"`
}

// PushFeedbackRequest for POST /api/v1/lore/feedback
type PushFeedbackRequest struct {
	SourceID string            `json:"source_id"`
	Feedback []FeedbackPayload `json:"feedback"`
}

// FeedbackPayload represents a single feedback entry.
type FeedbackPayload struct {
	LoreID string `json:"lore_id"`
	Type   string `json:"type"` // helpful | not_relevant | incorrect
}

// PushFeedbackResponse from POST /api/v1/lore/feedback
type PushFeedbackResponse struct {
	Updates []FeedbackUpdate `json:"updates"`
}

// FeedbackUpdate represents a single feedback result.
type FeedbackUpdate struct {
	LoreID             string  `json:"lore_id"`
	PreviousConfidence float64 `json:"previous_confidence"`
	CurrentConfidence  float64 `json:"current_confidence"`
	ValidationCount    int     `json:"validation_count"`
}

// DeltaResult contains incremental changes from Engram.
// Returned by GET /api/v1/lore/delta endpoint.
type DeltaResult struct {
	Lore       []LoreEntry `json:"lore"`
	DeletedIDs []string    `json:"deleted_ids"`
	AsOf       string      `json:"as_of"`
}

// LoreEntry represents a full lore entry from Engram delta response.
type LoreEntry struct {
	ID              string   `json:"id"`
	Content         string   `json:"content"`
	Context         string   `json:"context,omitempty"`
	Category        string   `json:"category"`
	Confidence      float64  `json:"confidence"`
	Embedding       []byte   `json:"embedding,omitempty"`
	SourceID        string   `json:"source_id,omitempty"`
	Sources         []string `json:"sources"`
	ValidationCount int      `json:"validation_count"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
	EmbeddingStatus string   `json:"embedding_status"`
}

// ListStoresResponse from GET /api/v1/stores
type ListStoresResponse struct {
	Stores []StoreListItem `json:"stores"`
	Total  int             `json:"total"`
}

// StoreListItem represents summary information for a store.
type StoreListItem struct {
	ID           string `json:"id"`
	RecordCount  int64  `json:"record_count"`
	LastAccessed string `json:"last_accessed"`
	SizeBytes    int64  `json:"size_bytes"`
	Description  string `json:"description,omitempty"`
}

// StoreInfoResponse from GET /api/v1/stores/{store_id}
type StoreInfoResponse struct {
	ID           string        `json:"id"`
	Created      string        `json:"created"`
	LastAccessed string        `json:"last_accessed"`
	Description  string        `json:"description,omitempty"`
	SizeBytes    int64         `json:"size_bytes"`
	Stats        ExtendedStats `json:"stats"`
}

// ExtendedStats contains detailed statistics for a store.
type ExtendedStats struct {
	TotalLore         int64            `json:"total_lore"`
	ActiveLore        int64            `json:"active_lore"`
	DeletedLore       int64            `json:"deleted_lore"`
	EmbeddingStats    EmbeddingStats   `json:"embedding_stats"`
	CategoryStats     map[string]int64 `json:"category_stats"`
	QualityStats      QualityStats     `json:"quality_stats"`
	UniqueSourceCount int64            `json:"unique_source_count"`
	StatsAsOf         string           `json:"stats_as_of"`
}

// EmbeddingStats contains embedding generation statistics.
type EmbeddingStats struct {
	Complete int64 `json:"complete"`
	Pending  int64 `json:"pending"`
	Failed   int64 `json:"failed"`
}

// QualityStats contains lore quality metrics.
type QualityStats struct {
	AverageConfidence   float64 `json:"average_confidence"`
	ValidatedCount      int64   `json:"validated_count"`
	HighConfidenceCount int64   `json:"high_confidence_count"`
	LowConfidenceCount  int64   `json:"low_confidence_count"`
}
