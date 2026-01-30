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
