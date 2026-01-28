package recall

import "time"

// Lore represents a single piece of experiential knowledge.
type Lore struct {
	ID              string    `json:"id"`
	Content         string    `json:"content"`
	Category        Category  `json:"category"`
	Context         string    `json:"context,omitempty"`
	Confidence      float64   `json:"confidence"`
	Embedding       []byte    `json:"-"`
	ValidationCount int       `json:"validation_count"`
	LastValidated   *time.Time `json:"last_validated,omitempty"`
	SourceID        string    `json:"source_id"`
	Sources         []string  `json:"sources,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	SyncedAt        *time.Time `json:"synced_at,omitempty"`
}

// Category classifies the type of lore.
type Category string

const (
	CategoryArchitecturalDecision  Category = "ARCHITECTURAL_DECISION"
	CategoryPatternOutcome         Category = "PATTERN_OUTCOME"
	CategoryInterfaceLesson        Category = "INTERFACE_LESSON"
	CategoryEdgeCaseDiscovery      Category = "EDGE_CASE_DISCOVERY"
	CategoryImplementationFriction Category = "IMPLEMENTATION_FRICTION"
	CategoryTestingStrategy        Category = "TESTING_STRATEGY"
	CategoryDependencyBehavior     Category = "DEPENDENCY_BEHAVIOR"
	CategoryPerformanceInsight     Category = "PERFORMANCE_INSIGHT"
)

// ValidCategories returns all valid lore categories.
func ValidCategories() []Category {
	return []Category{
		CategoryArchitecturalDecision,
		CategoryPatternOutcome,
		CategoryInterfaceLesson,
		CategoryEdgeCaseDiscovery,
		CategoryImplementationFriction,
		CategoryTestingStrategy,
		CategoryDependencyBehavior,
		CategoryPerformanceInsight,
	}
}

// IsValid checks if the category is a valid lore category.
func (c Category) IsValid() bool {
	for _, valid := range ValidCategories() {
		if c == valid {
			return true
		}
	}
	return false
}

// FeedbackType classifies feedback on lore.
type FeedbackType string

const (
	FeedbackHelpful     FeedbackType = "helpful"
	FeedbackIncorrect   FeedbackType = "incorrect"
	FeedbackNotRelevant FeedbackType = "not_relevant"
)

// RecordParams contains parameters for recording new lore.
type RecordParams struct {
	Content    string   `json:"content"`
	Context    string   `json:"context,omitempty"`
	Category   Category `json:"category"`
	Confidence float64  `json:"confidence,omitempty"`
}

// QueryParams configures a lore query.
type QueryParams struct {
	Query         string     `json:"query"`
	K             int        `json:"k,omitempty"`
	MinConfidence float64    `json:"min_confidence,omitempty"`
	Categories    []Category `json:"categories,omitempty"`
}

// QueryResult contains query results with session tracking.
type QueryResult struct {
	Lore        []Lore            `json:"lore"`
	SessionRefs map[string]string `json:"session_refs"` // L1 -> lore ID
}

// FeedbackParams provides feedback on recalled lore.
type FeedbackParams struct {
	Helpful     []string `json:"helpful,omitempty"`      // Session refs or content snippets
	NotRelevant []string `json:"not_relevant,omitempty"` // Surfaced but didn't apply
	Incorrect   []string `json:"incorrect,omitempty"`    // Wrong or misleading
}

// FeedbackResult contains the results of applying feedback.
type FeedbackResult struct {
	Updated []FeedbackUpdate `json:"updated"`
}

// FeedbackUpdate describes a single confidence update.
type FeedbackUpdate struct {
	ID              string  `json:"id"`
	Previous        float64 `json:"previous"`
	Current         float64 `json:"current"`
	ValidationCount int     `json:"validation_count"`
}

// SessionLore tracks lore surfaced during a session.
type SessionLore struct {
	SessionRef string   `json:"session_ref"` // L1, L2, etc.
	ID         string   `json:"id"`
	Content    string   `json:"content"`    // First 100 chars
	Category   Category `json:"category"`
	Confidence float64  `json:"confidence"`
	Source     string   `json:"source"` // "passive" or "query"
}

// SyncStats contains statistics from a sync operation.
type SyncStats struct {
	Pulled   int           `json:"pulled"`
	Pushed   int           `json:"pushed"`
	Merged   int           `json:"merged"`
	Errors   int           `json:"errors"`
	Duration time.Duration `json:"duration"`
}

// StoreStats contains statistics about the local store.
type StoreStats struct {
	LoreCount     int       `json:"lore_count"`
	PendingSync   int       `json:"pending_sync"`
	LastSync      time.Time `json:"last_sync"`
	SchemaVersion string    `json:"schema_version"`
}

// HealthStatus represents the health of the client.
type HealthStatus struct {
	Healthy       bool   `json:"healthy"`
	StoreOK       bool   `json:"store_ok"`
	EngramReachable bool `json:"engram_reachable"`
	Error         string `json:"error,omitempty"`
}

// Confidence adjustment constants.
const (
	ConfidenceHelpfulDelta    = 0.08
	ConfidenceIncorrectDelta  = -0.15
	ConfidenceNotRelevantDelta = 0.0
	ConfidenceMergeBoost      = 0.10
	ConfidenceDecayPerMonth   = 0.01
	ConfidenceDefault         = 0.5
	ConfidenceMin             = 0.0
	ConfidenceMax             = 1.0
)

// Content limits.
const (
	MaxContentLength = 4000
	MaxContextLength = 1000
)
