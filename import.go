package recall

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// ImportJSON imports lore from a JSON export file.
// It uses streaming to handle large files without loading everything into memory.
//
// Note: This function holds the store's write lock for the entire duration of the import.
// For large imports, this may block other operations (reads and writes) until the import
// completes. Consider using dryRun=true first to preview the import scope.
func (s *Store) ImportJSON(ctx context.Context, r io.Reader, strategy MergeStrategy, dryRun bool) (*ImportResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	dec := json.NewDecoder(r)
	result := &ImportResult{}

	// Parse the opening brace
	token, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("read opening token: %w", err)
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("expected opening brace, got %v", token)
	}

	// Parse top-level fields until we find "lore"
	var version string
	for dec.More() {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		// Read field name
		token, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("read field name: %w", err)
		}

		fieldName, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("expected field name, got %v", token)
		}

		switch fieldName {
		case "version":
			if err := dec.Decode(&version); err != nil {
				return nil, fmt.Errorf("decode version: %w", err)
			}
			// Validate version
			if version != ExportVersion {
				return nil, fmt.Errorf("unsupported export version %q (expected %q)", version, ExportVersion)
			}

		case "exported_at", "store_id", "metadata":
			// Skip these fields - decode and discard
			var discard any
			if err := dec.Decode(&discard); err != nil {
				return nil, fmt.Errorf("decode %s: %w", fieldName, err)
			}

		case "lore":
			// Parse the lore array
			if err := s.importLoreArray(ctx, dec, strategy, dryRun, result); err != nil {
				return result, fmt.Errorf("import lore: %w", err)
			}

		default:
			// Skip unknown fields
			var discard any
			if err := dec.Decode(&discard); err != nil {
				return nil, fmt.Errorf("decode unknown field %s: %w", fieldName, err)
			}
		}
	}

	if version == "" {
		return nil, fmt.Errorf("missing version field in export file")
	}

	return result, nil
}

// importLoreArray processes the lore array from the JSON stream.
func (s *Store) importLoreArray(ctx context.Context, dec *json.Decoder, strategy MergeStrategy, dryRun bool, result *ImportResult) error {
	// Read opening bracket of lore array
	token, err := dec.Token()
	if err != nil {
		return fmt.Errorf("read lore array start: %w", err)
	}
	if delim, ok := token.(json.Delim); !ok || delim != '[' {
		return fmt.Errorf("expected lore array, got %v", token)
	}

	// Process each lore entry
	for dec.More() {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var exportLore ExportLore
		if err := dec.Decode(&exportLore); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("decode lore: %v", err))
			continue
		}
		result.Total++

		// Check if lore exists
		exists, err := s.loreExistsUnlocked(exportLore.ID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("check existence %s: %v", exportLore.ID, err))
			continue
		}

		if dryRun {
			// Preview mode - just count what would happen
			if exists {
				switch strategy {
				case MergeStrategySkip:
					result.Skipped++
				case MergeStrategyReplace, MergeStrategyMerge:
					result.Merged++
				}
			} else {
				result.Created++
			}
			continue
		}

		// Apply the merge strategy
		created, err := s.importLoreEntry(&exportLore, strategy, exists)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("import %s: %v", exportLore.ID, err))
			continue
		}

		if created {
			result.Created++
		} else if strategy == MergeStrategySkip && exists {
			result.Skipped++
		} else {
			result.Merged++
		}
	}

	// Read closing bracket of lore array
	token, err = dec.Token()
	if err != nil {
		return fmt.Errorf("read lore array end: %w", err)
	}
	if delim, ok := token.(json.Delim); !ok || delim != ']' {
		return fmt.Errorf("expected lore array end, got %v", token)
	}

	return nil
}

// loreExistsUnlocked checks if a lore entry exists (caller must hold lock).
func (s *Store) loreExistsUnlocked(id string) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM lore_entries WHERE id = ? AND deleted_at IS NULL", id).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// importLoreEntry imports a single lore entry based on the merge strategy.
// Returns true if the entry was created (new), false if it was merged/skipped.
func (s *Store) importLoreEntry(exportLore *ExportLore, strategy MergeStrategy, exists bool) (bool, error) {
	if exists && strategy == MergeStrategySkip {
		return false, nil // Skip existing entries
	}

	// Convert ExportLore to Lore
	lore := exportLoreToLore(exportLore)

	if !exists {
		// New entry - insert
		return true, s.insertLoreForImport(lore)
	}

	// Entry exists - update based on strategy
	switch strategy {
	case MergeStrategyReplace:
		// Replace: overwrite the existing entry completely
		return false, s.replaceLoreForImport(lore)
	case MergeStrategyMerge:
		// Merge: upsert, potentially preserving some fields
		return false, s.mergeLoreForImport(lore)
	default:
		return false, nil
	}
}

// exportLoreToLore converts an ExportLore to a Lore.
func exportLoreToLore(e *ExportLore) *Lore {
	lore := &Lore{
		ID:              e.ID,
		Content:         e.Content,
		Context:         e.Context,
		Category:        Category(e.Category),
		Confidence:      e.Confidence,
		Embedding:       e.Embedding,
		EmbeddingStatus: e.EmbeddingStatus,
		SourceID:        e.SourceID,
		Sources:         e.Sources,
		ValidationCount: e.ValidationCount,
		CreatedAt:       e.CreatedAt,
		UpdatedAt:       e.UpdatedAt,
	}
	if !e.SyncedAt.IsZero() {
		lore.SyncedAt = &e.SyncedAt
	}
	return lore
}

// loreImportParams holds prepared SQL parameters for lore import operations.
type loreImportParams struct {
	embeddingBlob   []byte
	sourcesStr      string
	embeddingStatus string
	syncedAtStr     *string
}

// prepareLoreImportParams prepares common SQL parameters for lore import.
func prepareLoreImportParams(lore *Lore) loreImportParams {
	params := loreImportParams{
		sourcesStr:      "[]",
		embeddingStatus: "pending",
	}

	if len(lore.Embedding) > 0 {
		params.embeddingBlob = lore.Embedding
	}
	if len(lore.Sources) > 0 {
		params.sourcesStr = strings.Join(lore.Sources, ",")
	}
	if lore.EmbeddingStatus != "" {
		params.embeddingStatus = lore.EmbeddingStatus
	}
	if lore.SyncedAt != nil {
		ts := lore.SyncedAt.Format(time.RFC3339)
		params.syncedAtStr = &ts
	}

	return params
}

// insertLoreForImport inserts a lore entry during import (no sync queue).
func (s *Store) insertLoreForImport(lore *Lore) error {
	p := prepareLoreImportParams(lore)

	_, err := s.db.Exec(`
		INSERT INTO lore_entries (id, content, context, category, confidence, embedding, embedding_status,
		                 source_id, sources, validation_count, created_at, updated_at, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		lore.ID,
		lore.Content,
		nullString(lore.Context),
		string(lore.Category),
		lore.Confidence,
		p.embeddingBlob,
		p.embeddingStatus,
		lore.SourceID,
		p.sourcesStr,
		lore.ValidationCount,
		lore.CreatedAt.Format(time.RFC3339),
		lore.UpdatedAt.Format(time.RFC3339),
		p.syncedAtStr,
	)
	return err
}

// replaceLoreForImport replaces an existing lore entry during import.
func (s *Store) replaceLoreForImport(lore *Lore) error {
	p := prepareLoreImportParams(lore)

	_, err := s.db.Exec(`
		UPDATE lore_entries SET
			content = ?,
			context = ?,
			category = ?,
			confidence = ?,
			embedding = ?,
			embedding_status = ?,
			source_id = ?,
			sources = ?,
			validation_count = ?,
			created_at = ?,
			updated_at = ?,
			synced_at = ?,
			deleted_at = NULL
		WHERE id = ?
	`,
		lore.Content,
		nullString(lore.Context),
		string(lore.Category),
		lore.Confidence,
		p.embeddingBlob,
		p.embeddingStatus,
		lore.SourceID,
		p.sourcesStr,
		lore.ValidationCount,
		lore.CreatedAt.Format(time.RFC3339),
		lore.UpdatedAt.Format(time.RFC3339),
		p.syncedAtStr,
		lore.ID,
	)
	return err
}

// mergeLoreForImport merges an imported lore entry with an existing one.
// Uses upsert semantics - updates the entry if it exists.
func (s *Store) mergeLoreForImport(lore *Lore) error {
	p := prepareLoreImportParams(lore)

	// Upsert: insert or update
	_, err := s.db.Exec(`
		INSERT INTO lore_entries (id, content, context, category, confidence, embedding, embedding_status,
		                 source_id, sources, validation_count, created_at, updated_at, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			content = excluded.content,
			context = excluded.context,
			category = excluded.category,
			confidence = excluded.confidence,
			embedding = excluded.embedding,
			embedding_status = excluded.embedding_status,
			source_id = excluded.source_id,
			sources = excluded.sources,
			validation_count = excluded.validation_count,
			updated_at = excluded.updated_at,
			synced_at = excluded.synced_at,
			deleted_at = NULL
	`,
		lore.ID,
		lore.Content,
		nullString(lore.Context),
		string(lore.Category),
		lore.Confidence,
		p.embeddingBlob,
		p.embeddingStatus,
		lore.SourceID,
		p.sourcesStr,
		lore.ValidationCount,
		lore.CreatedAt.Format(time.RFC3339),
		lore.UpdatedAt.Format(time.RFC3339),
		p.syncedAtStr,
	)
	return err
}

// LoreExists checks if a lore entry with the given ID exists.
func (s *Store) LoreExists(id string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return false, ErrStoreClosed
	}

	return s.loreExistsUnlocked(id)
}
