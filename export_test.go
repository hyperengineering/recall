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

func TestExportJSON_Empty(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	var buf bytes.Buffer
	err = store.ExportJSON(context.Background(), "test-store", &buf)
	if err != nil {
		t.Fatalf("ExportJSON() returned error: %v", err)
	}

	// Parse the exported JSON
	var export recall.ExportFormat
	if err := json.Unmarshal(buf.Bytes(), &export); err != nil {
		t.Fatalf("json.Unmarshal() returned error: %v", err)
	}

	if export.Version != recall.ExportVersion {
		t.Errorf("Version = %q, want %q", export.Version, recall.ExportVersion)
	}
	if export.StoreID != "test-store" {
		t.Errorf("StoreID = %q, want %q", export.StoreID, "test-store")
	}
	if len(export.Lore) != 0 {
		t.Errorf("Lore count = %d, want 0", len(export.Lore))
	}
}

func TestExportJSON_WithLore(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	// Record some lore
	lore1, err := store.Record(recall.Lore{
		Content:    "Test content 1",
		Category:   recall.CategoryPatternOutcome,
		Confidence: 0.8,
	})
	if err != nil {
		t.Fatalf("Record() returned error: %v", err)
	}

	lore2, err := store.Record(recall.Lore{
		Content:    "Test content 2",
		Context:    "Testing context",
		Category:   recall.CategoryDependencyBehavior,
		Confidence: 0.6,
	})
	if err != nil {
		t.Fatalf("Record() returned error: %v", err)
	}

	var buf bytes.Buffer
	err = store.ExportJSON(context.Background(), "test-store", &buf)
	if err != nil {
		t.Fatalf("ExportJSON() returned error: %v", err)
	}

	// Parse the exported JSON
	var export recall.ExportFormat
	if err := json.Unmarshal(buf.Bytes(), &export); err != nil {
		t.Fatalf("json.Unmarshal() returned error: %v", err)
	}

	if export.Version != recall.ExportVersion {
		t.Errorf("Version = %q, want %q", export.Version, recall.ExportVersion)
	}
	if len(export.Lore) != 2 {
		t.Fatalf("Lore count = %d, want 2", len(export.Lore))
	}

	// Find lore1 and lore2 in export
	var foundLore1, foundLore2 bool
	for _, l := range export.Lore {
		if l.ID == lore1.ID {
			foundLore1 = true
			if l.Content != "Test content 1" {
				t.Errorf("Lore1 content = %q, want %q", l.Content, "Test content 1")
			}
			if l.Category != string(recall.CategoryPatternOutcome) {
				t.Errorf("Lore1 category = %q, want %q", l.Category, recall.CategoryPatternOutcome)
			}
		}
		if l.ID == lore2.ID {
			foundLore2 = true
			if l.Content != "Test content 2" {
				t.Errorf("Lore2 content = %q, want %q", l.Content, "Test content 2")
			}
			if l.Context != "Testing context" {
				t.Errorf("Lore2 context = %q, want %q", l.Context, "Testing context")
			}
		}
	}

	if !foundLore1 {
		t.Error("Lore1 not found in export")
	}
	if !foundLore2 {
		t.Error("Lore2 not found in export")
	}
}

func TestExportJSON_Cancellation(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	// Record some lore
	for i := 0; i < 10; i++ {
		_, err := store.Record(recall.Lore{
			Content:    "Test content",
			Category:   recall.CategoryPatternOutcome,
			Confidence: 0.8,
		})
		if err != nil {
			t.Fatalf("Record() returned error: %v", err)
		}
	}

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var buf bytes.Buffer
	err = store.ExportJSON(ctx, "test-store", &buf)
	if err == nil {
		t.Error("ExportJSON() should return error for cancelled context")
	}
	// Error may be wrapped, so check if it contains "canceled"
	if !strings.Contains(err.Error(), "canceled") {
		t.Errorf("ExportJSON() error = %v, should contain 'canceled'", err)
	}
}

func TestExportSQLite(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	// Record some lore
	_, err = store.Record(recall.Lore{
		Content:    "Test content",
		Category:   recall.CategoryPatternOutcome,
		Confidence: 0.8,
	})
	if err != nil {
		t.Fatalf("Record() returned error: %v", err)
	}

	// Export to SQLite
	exportPath := filepath.Join(dir, "export.db")
	err = store.ExportSQLite(context.Background(), exportPath)
	if err != nil {
		t.Fatalf("ExportSQLite() returned error: %v", err)
	}

	// Verify the exported database can be opened
	exportedStore, err := recall.NewStore(exportPath)
	if err != nil {
		t.Fatalf("NewStore() on exported db returned error: %v", err)
	}
	defer exportedStore.Close()

	count, err := exportedStore.LoreCount()
	if err != nil {
		t.Fatalf("LoreCount() returned error: %v", err)
	}
	if count != 1 {
		t.Errorf("LoreCount() = %d, want 1", count)
	}
}

func TestLoreCount(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	// Empty store
	count, err := store.LoreCount()
	if err != nil {
		t.Fatalf("LoreCount() returned error: %v", err)
	}
	if count != 0 {
		t.Errorf("LoreCount() = %d, want 0", count)
	}

	// Add lore
	_, err = store.Record(recall.Lore{
		Content:    "Test content",
		Category:   recall.CategoryPatternOutcome,
		Confidence: 0.8,
	})
	if err != nil {
		t.Fatalf("Record() returned error: %v", err)
	}

	count, err = store.LoreCount()
	if err != nil {
		t.Fatalf("LoreCount() returned error: %v", err)
	}
	if count != 1 {
		t.Errorf("LoreCount() = %d, want 1", count)
	}
}

func TestExportJSON_StreamingFormat(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	// Record lore
	_, err = store.Record(recall.Lore{
		Content:    "Test content",
		Category:   recall.CategoryPatternOutcome,
		Confidence: 0.8,
	})
	if err != nil {
		t.Fatalf("Record() returned error: %v", err)
	}

	var buf bytes.Buffer
	err = store.ExportJSON(context.Background(), "test-store", &buf)
	if err != nil {
		t.Fatalf("ExportJSON() returned error: %v", err)
	}

	// Verify it's valid JSON
	output := buf.String()
	if !strings.HasPrefix(output, "{") {
		t.Error("Export should start with {")
	}
	if !strings.HasSuffix(strings.TrimSpace(output), "}") {
		t.Error("Export should end with }")
	}

	// Verify required fields are present
	if !strings.Contains(output, `"version"`) {
		t.Error("Export should contain version field")
	}
	if !strings.Contains(output, `"exported_at"`) {
		t.Error("Export should contain exported_at field")
	}
	if !strings.Contains(output, `"store_id"`) {
		t.Error("Export should contain store_id field")
	}
	if !strings.Contains(output, `"lore"`) {
		t.Error("Export should contain lore field")
	}
}

func TestExportJSON_WithMetadata(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	// Set store metadata
	err = store.SetStoreDescription("Test store description")
	if err != nil {
		t.Fatalf("SetStoreDescription() returned error: %v", err)
	}

	var buf bytes.Buffer
	err = store.ExportJSON(context.Background(), "test-store", &buf)
	if err != nil {
		t.Fatalf("ExportJSON() returned error: %v", err)
	}

	// Parse and verify metadata
	var export recall.ExportFormat
	if err := json.Unmarshal(buf.Bytes(), &export); err != nil {
		t.Fatalf("json.Unmarshal() returned error: %v", err)
	}

	if export.Metadata.Description != "Test store description" {
		t.Errorf("Metadata.Description = %q, want %q", export.Metadata.Description, "Test store description")
	}
}

func TestExportImport_EmbeddingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	// Create a lore entry with an embedding
	originalEmbedding := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	lore, err := store.Record(recall.Lore{
		Content:    "Test content with embedding",
		Category:   recall.CategoryPatternOutcome,
		Confidence: 0.9,
		Embedding:  originalEmbedding,
	})
	if err != nil {
		t.Fatalf("Record() returned error: %v", err)
	}

	// Export to JSON
	var buf bytes.Buffer
	err = store.ExportJSON(context.Background(), "test-store", &buf)
	if err != nil {
		t.Fatalf("ExportJSON() returned error: %v", err)
	}

	// Create a new store for import
	importDBPath := filepath.Join(dir, "import.db")
	importStore, err := recall.NewStore(importDBPath)
	if err != nil {
		t.Fatalf("NewStore() for import returned error: %v", err)
	}
	defer importStore.Close()

	// Import from JSON
	result, err := importStore.ImportJSON(context.Background(), &buf, recall.MergeStrategyReplace, false)
	if err != nil {
		t.Fatalf("ImportJSON() returned error: %v", err)
	}

	if result.Created != 1 {
		t.Errorf("ImportResult.Created = %d, want 1", result.Created)
	}

	// Retrieve the imported lore and verify embedding
	importedLore, err := importStore.Get(lore.ID)
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}

	if !bytes.Equal(importedLore.Embedding, originalEmbedding) {
		t.Errorf("Embedding mismatch: got %v, want %v", importedLore.Embedding, originalEmbedding)
	}
}

func TestExportJSON_TimestampFormat(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	_, err = store.Record(recall.Lore{
		Content:    "Test content",
		Category:   recall.CategoryPatternOutcome,
		Confidence: 0.8,
	})
	if err != nil {
		t.Fatalf("Record() returned error: %v", err)
	}

	var buf bytes.Buffer
	err = store.ExportJSON(context.Background(), "test-store", &buf)
	if err != nil {
		t.Fatalf("ExportJSON() returned error: %v", err)
	}

	// Parse and verify timestamps are RFC3339
	var export recall.ExportFormat
	if err := json.Unmarshal(buf.Bytes(), &export); err != nil {
		t.Fatalf("json.Unmarshal() returned error: %v", err)
	}

	if export.ExportedAt.IsZero() {
		t.Error("ExportedAt should not be zero")
	}

	// Verify lore timestamps
	for _, l := range export.Lore {
		if l.CreatedAt.IsZero() {
			t.Error("Lore CreatedAt should not be zero")
		}
		if l.UpdatedAt.IsZero() {
			t.Error("Lore UpdatedAt should not be zero")
		}

		// Verify timestamps are within reasonable range
		now := time.Now()
		if l.CreatedAt.After(now.Add(time.Minute)) {
			t.Error("Lore CreatedAt should not be in the future")
		}
	}
}
