package sync

import "time"

// HealthResponse from GET /api/v1/health
type HealthResponse struct {
	Status         string `json:"status"`
	Version        string `json:"version"`
	EmbeddingModel string `json:"embedding_model"`
	LoreCount      int    `json:"lore_count"`
	LastSnapshot   string `json:"last_snapshot"`
}

// =============================================================================
// Store Management Types (Story 7.5: Multi-Store Sync)
// =============================================================================

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
	SnapshotStats     *SnapshotStats   `json:"snapshot_stats,omitempty"`
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

// SnapshotStats contains snapshot observability metrics.
// Matches OpenAPI SnapshotStats schema.
type SnapshotStats struct {
	LoreCount      int64  `json:"lore_count"`
	SizeBytes      int64  `json:"size_bytes"`
	GeneratedAt    string `json:"generated_at,omitempty"`
	AgeSeconds     int64  `json:"age_seconds"`
	PendingEntries int64  `json:"pending_entries"`
	Available      bool   `json:"available"`
}

// CreateStoreRequest for POST /stores.
// Matches OpenAPI CreateStoreRequest schema.
type CreateStoreRequest struct {
	StoreID     string `json:"store_id"`
	Description string `json:"description,omitempty"`
}

// CreateStoreResponse for POST /stores response.
// Matches OpenAPI CreateStoreResponse schema.
type CreateStoreResponse struct {
	ID          string    `json:"id"`
	Created     time.Time `json:"created"`
	Description string    `json:"description,omitempty"`
}
