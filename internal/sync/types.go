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
	ID      string `json:"id"`
	Outcome string `json:"outcome"` // helpful | not_relevant | incorrect
}

// PushFeedbackResponse from POST /api/v1/lore/feedback
type PushFeedbackResponse struct {
	Updates []FeedbackUpdate `json:"updates"`
}

// FeedbackUpdate represents a single feedback result.
type FeedbackUpdate struct {
	ID              string  `json:"id"`
	Previous        float64 `json:"previous"`
	Current         float64 `json:"current"`
	ValidationCount int     `json:"validation_count"`
}
