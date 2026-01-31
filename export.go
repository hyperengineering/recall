package recall

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// ExportVersion is the current version of the export format.
const ExportVersion = "1.0"

// ExportFormat is the top-level structure for JSON exports.
type ExportFormat struct {
	Version    string         `json:"version"`
	ExportedAt time.Time      `json:"exported_at"`
	StoreID    string         `json:"store_id"`
	Metadata   ExportMetadata `json:"metadata"`
	Lore       []ExportLore   `json:"lore"`
}

// ExportMetadata contains store metadata in exports.
type ExportMetadata struct {
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
}

// ExportLore is a lore entry in export format.
type ExportLore struct {
	ID              string    `json:"id"`
	Content         string    `json:"content"`
	Context         string    `json:"context,omitempty"`
	Category        string    `json:"category"`
	Confidence      float64   `json:"confidence"`
	Embedding       []byte    `json:"embedding,omitempty"`
	EmbeddingStatus string    `json:"embedding_status,omitempty"`
	SourceID        string    `json:"source_id,omitempty"`
	Sources         []string  `json:"sources,omitempty"`
	ValidationCount int       `json:"validation_count"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	SyncedAt        time.Time `json:"synced_at,omitempty"`
}

// MergeStrategy defines how to handle conflicts during import.
type MergeStrategy string

const (
	// MergeStrategySkip skips entries that already exist (by ID).
	MergeStrategySkip MergeStrategy = "skip"
	// MergeStrategyReplace replaces existing entries with imported versions.
	MergeStrategyReplace MergeStrategy = "replace"
	// MergeStrategyMerge upserts entries by ID (default).
	MergeStrategyMerge MergeStrategy = "merge"
)

// ImportResult summarizes an import operation.
type ImportResult struct {
	Total   int      `json:"total"`
	Created int      `json:"created"`
	Merged  int      `json:"merged"`
	Skipped int      `json:"skipped"`
	Errors  []string `json:"errors,omitempty"`
}

// ExportJSON streams store data as JSON to the writer.
// This uses cursor-based iteration to avoid loading all data into memory.
func (s *Store) ExportJSON(ctx context.Context, storeID string, w io.Writer) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return ErrStoreClosed
	}

	// Get metadata
	desc, _ := s.GetMetadata(metadataKeyDescription)
	createdAtStr, _ := s.GetMetadata(metadataKeyCreatedAt)
	var createdAt time.Time
	if createdAtStr != "" {
		createdAt, _ = time.Parse(time.RFC3339, createdAtStr)
	}

	// Write opening structure manually for streaming
	header := fmt.Sprintf(`{"version":"%s","exported_at":"%s","store_id":"%s","metadata":{"description":%s,"created_at":"%s"},"lore":[`,
		ExportVersion,
		time.Now().UTC().Format(time.RFC3339),
		storeID,
		jsonString(desc),
		createdAt.Format(time.RFC3339),
	)
	if _, err := io.WriteString(w, header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	// Stream lore entries using cursor-based iteration
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, content, context, category, confidence, embedding, embedding_status,
		       source_id, sources, validation_count, created_at, updated_at, synced_at
		FROM lore_entries
		WHERE deleted_at IS NULL
		ORDER BY created_at
	`)
	if err != nil {
		return fmt.Errorf("query lore: %w", err)
	}
	defer rows.Close()

	enc := json.NewEncoder(w)
	first := true

	for rows.Next() {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		lore, err := s.scanExportLoreRows(rows)
		if err != nil {
			return fmt.Errorf("scan lore: %w", err)
		}

		if !first {
			if _, err := io.WriteString(w, ","); err != nil {
				return fmt.Errorf("write separator: %w", err)
			}
		}
		first = false

		if err := enc.Encode(lore); err != nil {
			return fmt.Errorf("encode lore: %w", err)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate lore: %w", err)
	}

	// Close JSON structure
	if _, err := io.WriteString(w, "]}"); err != nil {
		return fmt.Errorf("write footer: %w", err)
	}

	return nil
}

// scanExportLoreRows scans a row into ExportLore format.
func (s *Store) scanExportLoreRows(rows interface{ Scan(...any) error }) (*ExportLore, error) {
	var (
		lore            ExportLore
		context         *string
		embeddingBlob   []byte
		embeddingStatus *string
		sourceID        *string
		sources         *string
		createdAt       string
		updatedAt       string
		syncedAt        *string
	)

	err := rows.Scan(
		&lore.ID,
		&lore.Content,
		&context,
		&lore.Category,
		&lore.Confidence,
		&embeddingBlob,
		&embeddingStatus,
		&sourceID,
		&sources,
		&lore.ValidationCount,
		&createdAt,
		&updatedAt,
		&syncedAt,
	)
	if err != nil {
		return nil, err
	}

	if context != nil {
		lore.Context = *context
	}
	if len(embeddingBlob) > 0 {
		lore.Embedding = embeddingBlob
	}
	if embeddingStatus != nil {
		lore.EmbeddingStatus = *embeddingStatus
	}
	if sourceID != nil {
		lore.SourceID = *sourceID
	}
	if sources != nil && *sources != "" && *sources != "[]" {
		// Parse comma-separated sources
		lore.Sources = splitSources(*sources)
	}
	lore.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	lore.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if syncedAt != nil {
		lore.SyncedAt, _ = time.Parse(time.RFC3339, *syncedAt)
	}

	return &lore, nil
}

// splitSources splits a comma-separated sources string.
func splitSources(s string) []string {
	if s == "" || s == "[]" {
		return nil
	}
	var result []string
	for _, part := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// jsonString returns a JSON-encoded string.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// ExportSQLite exports the store to a SQLite database file.
// It performs a WAL checkpoint first to ensure consistency, then copies the database file.
func (s *Store) ExportSQLite(ctx context.Context, destPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	// Perform WAL checkpoint to flush pending writes
	if _, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("checkpoint WAL: %w", err)
	}

	// Copy the database file
	srcFile, err := os.Open(s.path)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer srcFile.Close()

	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, srcFile); err != nil {
		_ = os.Remove(destPath)
		return fmt.Errorf("copy database: %w", err)
	}

	return destFile.Sync()
}

// LoreCount returns the number of active lore entries.
func (s *Store) LoreCount() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return 0, ErrStoreClosed
	}

	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM lore_entries WHERE deleted_at IS NULL").Scan(&count)
	return count, err
}
