package recall_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hyperengineering/recall"
)

func TestImportJSON_Empty(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	// Import empty export
	exportData := `{
		"version": "1.0",
		"exported_at": "2026-01-31T12:00:00Z",
		"store_id": "test",
		"metadata": {},
		"lore": []
	}`

	result, err := store.ImportJSON(context.Background(), strings.NewReader(exportData), recall.MergeStrategyMerge, false)
	if err != nil {
		t.Fatalf("ImportJSON() returned error: %v", err)
	}

	if result.Total != 0 {
		t.Errorf("Total = %d, want 0", result.Total)
	}
	if result.Created != 0 {
		t.Errorf("Created = %d, want 0", result.Created)
	}
}

func TestImportJSON_NewEntries(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	exportData := `{
		"version": "1.0",
		"exported_at": "2026-01-31T12:00:00Z",
		"store_id": "test",
		"metadata": {},
		"lore": [
			{
				"id": "01HXK4ABCDEF123456789",
				"content": "Test content 1",
				"category": "PATTERN_OUTCOME",
				"confidence": 0.8,
				"validation_count": 0,
				"created_at": "2026-01-25T10:00:00Z",
				"updated_at": "2026-01-30T14:30:00Z"
			},
			{
				"id": "01HXK4ABCDEF987654321",
				"content": "Test content 2",
				"context": "Testing",
				"category": "DEPENDENCY_BEHAVIOR",
				"confidence": 0.6,
				"validation_count": 2,
				"created_at": "2026-01-26T10:00:00Z",
				"updated_at": "2026-01-30T15:30:00Z"
			}
		]
	}`

	result, err := store.ImportJSON(context.Background(), strings.NewReader(exportData), recall.MergeStrategyMerge, false)
	if err != nil {
		t.Fatalf("ImportJSON() returned error: %v", err)
	}

	if result.Total != 2 {
		t.Errorf("Total = %d, want 2", result.Total)
	}
	if result.Created != 2 {
		t.Errorf("Created = %d, want 2", result.Created)
	}
	if result.Merged != 0 {
		t.Errorf("Merged = %d, want 0", result.Merged)
	}

	// Verify entries were imported
	count, _ := store.LoreCount()
	if count != 2 {
		t.Errorf("LoreCount() = %d, want 2", count)
	}

	// Verify lore content
	lore, err := store.Get("01HXK4ABCDEF123456789")
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}
	if lore.Content != "Test content 1" {
		t.Errorf("Content = %q, want %q", lore.Content, "Test content 1")
	}
}

func TestImportJSON_SkipStrategy(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	// Create existing entry
	_, err = store.Record(recall.Lore{
		ID:         "01HXK4ABCDEF123456789",
		Content:    "Original content",
		Category:   recall.CategoryPatternOutcome,
		Confidence: 0.5,
	})
	if err != nil {
		t.Fatalf("Record() returned error: %v", err)
	}

	exportData := `{
		"version": "1.0",
		"exported_at": "2026-01-31T12:00:00Z",
		"store_id": "test",
		"metadata": {},
		"lore": [
			{
				"id": "01HXK4ABCDEF123456789",
				"content": "Updated content",
				"category": "PATTERN_OUTCOME",
				"confidence": 0.9,
				"validation_count": 5,
				"created_at": "2026-01-25T10:00:00Z",
				"updated_at": "2026-01-30T14:30:00Z"
			},
			{
				"id": "01HXK4ABCDEF987654321",
				"content": "New content",
				"category": "DEPENDENCY_BEHAVIOR",
				"confidence": 0.6,
				"validation_count": 0,
				"created_at": "2026-01-26T10:00:00Z",
				"updated_at": "2026-01-30T15:30:00Z"
			}
		]
	}`

	result, err := store.ImportJSON(context.Background(), strings.NewReader(exportData), recall.MergeStrategySkip, false)
	if err != nil {
		t.Fatalf("ImportJSON() returned error: %v", err)
	}

	if result.Total != 2 {
		t.Errorf("Total = %d, want 2", result.Total)
	}
	if result.Created != 1 {
		t.Errorf("Created = %d, want 1", result.Created)
	}
	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.Skipped)
	}

	// Verify original content was preserved
	lore, _ := store.Get("01HXK4ABCDEF123456789")
	if lore.Content != "Original content" {
		t.Errorf("Content = %q, want %q (should be preserved)", lore.Content, "Original content")
	}
}

func TestImportJSON_ReplaceStrategy(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	// Create existing entry
	_, err = store.Record(recall.Lore{
		ID:         "01HXK4ABCDEF123456789",
		Content:    "Original content",
		Category:   recall.CategoryPatternOutcome,
		Confidence: 0.5,
	})
	if err != nil {
		t.Fatalf("Record() returned error: %v", err)
	}

	exportData := `{
		"version": "1.0",
		"exported_at": "2026-01-31T12:00:00Z",
		"store_id": "test",
		"metadata": {},
		"lore": [
			{
				"id": "01HXK4ABCDEF123456789",
				"content": "Replaced content",
				"category": "DEPENDENCY_BEHAVIOR",
				"confidence": 0.9,
				"validation_count": 5,
				"created_at": "2026-01-25T10:00:00Z",
				"updated_at": "2026-01-30T14:30:00Z"
			}
		]
	}`

	result, err := store.ImportJSON(context.Background(), strings.NewReader(exportData), recall.MergeStrategyReplace, false)
	if err != nil {
		t.Fatalf("ImportJSON() returned error: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("Total = %d, want 1", result.Total)
	}
	if result.Merged != 1 {
		t.Errorf("Merged = %d, want 1", result.Merged)
	}

	// Verify content was replaced
	lore, _ := store.Get("01HXK4ABCDEF123456789")
	if lore.Content != "Replaced content" {
		t.Errorf("Content = %q, want %q", lore.Content, "Replaced content")
	}
	if lore.Category != recall.CategoryDependencyBehavior {
		t.Errorf("Category = %s, want %s", lore.Category, recall.CategoryDependencyBehavior)
	}
}

func TestImportJSON_MergeStrategy(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	// Create existing entry
	_, err = store.Record(recall.Lore{
		ID:         "01HXK4ABCDEF123456789",
		Content:    "Original content",
		Category:   recall.CategoryPatternOutcome,
		Confidence: 0.5,
	})
	if err != nil {
		t.Fatalf("Record() returned error: %v", err)
	}

	exportData := `{
		"version": "1.0",
		"exported_at": "2026-01-31T12:00:00Z",
		"store_id": "test",
		"metadata": {},
		"lore": [
			{
				"id": "01HXK4ABCDEF123456789",
				"content": "Merged content",
				"category": "DEPENDENCY_BEHAVIOR",
				"confidence": 0.9,
				"validation_count": 5,
				"created_at": "2026-01-25T10:00:00Z",
				"updated_at": "2026-01-30T14:30:00Z"
			}
		]
	}`

	result, err := store.ImportJSON(context.Background(), strings.NewReader(exportData), recall.MergeStrategyMerge, false)
	if err != nil {
		t.Fatalf("ImportJSON() returned error: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("Total = %d, want 1", result.Total)
	}
	if result.Merged != 1 {
		t.Errorf("Merged = %d, want 1", result.Merged)
	}

	// Verify content was merged (updated)
	lore, _ := store.Get("01HXK4ABCDEF123456789")
	if lore.Content != "Merged content" {
		t.Errorf("Content = %q, want %q", lore.Content, "Merged content")
	}
}

func TestImportJSON_DryRun(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	// Create existing entry
	_, err = store.Record(recall.Lore{
		ID:         "01HXK4ABCDEF123456789",
		Content:    "Original content",
		Category:   recall.CategoryPatternOutcome,
		Confidence: 0.5,
	})
	if err != nil {
		t.Fatalf("Record() returned error: %v", err)
	}

	exportData := `{
		"version": "1.0",
		"exported_at": "2026-01-31T12:00:00Z",
		"store_id": "test",
		"metadata": {},
		"lore": [
			{
				"id": "01HXK4ABCDEF123456789",
				"content": "Updated content",
				"category": "PATTERN_OUTCOME",
				"confidence": 0.9,
				"validation_count": 5,
				"created_at": "2026-01-25T10:00:00Z",
				"updated_at": "2026-01-30T14:30:00Z"
			},
			{
				"id": "01HXK4ABCDEF987654321",
				"content": "New content",
				"category": "DEPENDENCY_BEHAVIOR",
				"confidence": 0.6,
				"validation_count": 0,
				"created_at": "2026-01-26T10:00:00Z",
				"updated_at": "2026-01-30T15:30:00Z"
			}
		]
	}`

	result, err := store.ImportJSON(context.Background(), strings.NewReader(exportData), recall.MergeStrategyMerge, true)
	if err != nil {
		t.Fatalf("ImportJSON() returned error: %v", err)
	}

	if result.Total != 2 {
		t.Errorf("Total = %d, want 2", result.Total)
	}
	if result.Created != 1 {
		t.Errorf("Created = %d, want 1 (preview)", result.Created)
	}
	if result.Merged != 1 {
		t.Errorf("Merged = %d, want 1 (preview)", result.Merged)
	}

	// Verify no changes were actually made
	count, _ := store.LoreCount()
	if count != 1 {
		t.Errorf("LoreCount() = %d, want 1 (dry-run should not add entries)", count)
	}

	// Verify original content is preserved
	lore, _ := store.Get("01HXK4ABCDEF123456789")
	if lore.Content != "Original content" {
		t.Errorf("Content = %q, want %q (dry-run should preserve)", lore.Content, "Original content")
	}
}

func TestImportJSON_InvalidVersion(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	exportData := `{
		"version": "2.0",
		"exported_at": "2026-01-31T12:00:00Z",
		"store_id": "test",
		"metadata": {},
		"lore": []
	}`

	_, err = store.ImportJSON(context.Background(), strings.NewReader(exportData), recall.MergeStrategyMerge, false)
	if err == nil {
		t.Error("ImportJSON() should return error for unsupported version")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("Error should mention unsupported version, got: %v", err)
	}
}

func TestImportJSON_MissingVersion(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	exportData := `{
		"exported_at": "2026-01-31T12:00:00Z",
		"store_id": "test",
		"metadata": {},
		"lore": []
	}`

	_, err = store.ImportJSON(context.Background(), strings.NewReader(exportData), recall.MergeStrategyMerge, false)
	if err == nil {
		t.Error("ImportJSON() should return error for missing version")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("Error should mention version, got: %v", err)
	}
}

func TestImportJSON_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	exportData := `{invalid json}`

	_, err = store.ImportJSON(context.Background(), strings.NewReader(exportData), recall.MergeStrategyMerge, false)
	if err == nil {
		t.Error("ImportJSON() should return error for malformed JSON")
	}
}

func TestImportJSON_Cancellation(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	exportData := `{
		"version": "1.0",
		"exported_at": "2026-01-31T12:00:00Z",
		"store_id": "test",
		"metadata": {},
		"lore": [
			{"id": "1", "content": "Content 1", "category": "PATTERN_OUTCOME", "confidence": 0.5, "validation_count": 0, "created_at": "2026-01-25T10:00:00Z", "updated_at": "2026-01-30T14:30:00Z"},
			{"id": "2", "content": "Content 2", "category": "PATTERN_OUTCOME", "confidence": 0.5, "validation_count": 0, "created_at": "2026-01-25T10:00:00Z", "updated_at": "2026-01-30T14:30:00Z"}
		]
	}`

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = store.ImportJSON(ctx, strings.NewReader(exportData), recall.MergeStrategyMerge, false)
	if err == nil {
		t.Error("ImportJSON() should return error for cancelled context")
	}
	if err != context.Canceled {
		t.Errorf("ImportJSON() error = %v, want context.Canceled", err)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Create source store with data
	srcPath := filepath.Join(dir, "src.db")
	srcStore, err := recall.NewStore(srcPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}

	// Record lore with various fields
	_, err = srcStore.Record(recall.Lore{
		Content:    "Test content 1",
		Context:    "Test context",
		Category:   recall.CategoryPatternOutcome,
		Confidence: 0.8,
	})
	if err != nil {
		t.Fatalf("Record() returned error: %v", err)
	}

	_, err = srcStore.Record(recall.Lore{
		Content:    "Test content 2",
		Category:   recall.CategoryDependencyBehavior,
		Confidence: 0.6,
	})
	if err != nil {
		t.Fatalf("Record() returned error: %v", err)
	}

	// Set metadata
	srcStore.SetStoreDescription("Test description")

	// Export
	var buf bytes.Buffer
	err = srcStore.ExportJSON(context.Background(), "test-store", &buf)
	srcStore.Close()
	if err != nil {
		t.Fatalf("ExportJSON() returned error: %v", err)
	}

	// Create destination store
	dstPath := filepath.Join(dir, "dst.db")
	dstStore, err := recall.NewStore(dstPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer dstStore.Close()

	// Import
	result, err := dstStore.ImportJSON(context.Background(), &buf, recall.MergeStrategyMerge, false)
	if err != nil {
		t.Fatalf("ImportJSON() returned error: %v", err)
	}

	if result.Total != 2 {
		t.Errorf("Total = %d, want 2", result.Total)
	}
	if result.Created != 2 {
		t.Errorf("Created = %d, want 2", result.Created)
	}

	// Verify lore count matches
	count, _ := dstStore.LoreCount()
	if count != 2 {
		t.Errorf("LoreCount() = %d, want 2", count)
	}
}

func TestImportJSON_LargeBatch(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	// Generate large export with many entries
	export := recall.ExportFormat{
		Version:    recall.ExportVersion,
		ExportedAt: time.Now(),
		StoreID:    "test",
		Metadata:   recall.ExportMetadata{},
		Lore:       make([]recall.ExportLore, 100),
	}

	for i := 0; i < 100; i++ {
		export.Lore[i] = recall.ExportLore{
			ID:              "ID" + string(rune('A'+i%26)) + string(rune('0'+i/26)),
			Content:         "Test content " + string(rune('0'+i)),
			Category:        "PATTERN_OUTCOME",
			Confidence:      0.5,
			ValidationCount: i,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
	}

	exportBytes, _ := json.Marshal(export)

	result, err := store.ImportJSON(context.Background(), bytes.NewReader(exportBytes), recall.MergeStrategyMerge, false)
	if err != nil {
		t.Fatalf("ImportJSON() returned error: %v", err)
	}

	if result.Total != 100 {
		t.Errorf("Total = %d, want 100", result.Total)
	}
	if result.Created != 100 {
		t.Errorf("Created = %d, want 100", result.Created)
	}

	count, _ := store.LoreCount()
	if count != 100 {
		t.Errorf("LoreCount() = %d, want 100", count)
	}
}
