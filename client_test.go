package recall_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hyperengineering/recall"
)

func TestNew_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Fatal("New() returned nil client")
	}
}

func TestNew_MissingLocalPath(t *testing.T) {
	_, err := recall.New(recall.Config{LocalPath: ""})
	if err == nil {
		t.Fatal("New() returned nil error, want ValidationError")
	}

	var ve *recall.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("New() returned %T, want *ValidationError", err)
	}
	if ve.Field != "LocalPath" {
		t.Errorf("ValidationError.Field = %q, want %q", ve.Field, "LocalPath")
	}
}

func TestNew_NoEngramURL_OfflineMode(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("New() returned error for offline-only config: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Fatal("New() returned nil client for offline-only config")
	}
}

func TestNew_EngramURLWithoutAPIKey(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	_, err := recall.New(recall.Config{
		LocalPath: dbPath,
		EngramURL: "http://engram:8080",
	})
	if err == nil {
		t.Fatal("New() returned nil error, want ValidationError for missing APIKey")
	}

	var ve *recall.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("New() returned %T, want *ValidationError", err)
	}
	if ve.Field != "APIKey" {
		t.Errorf("ValidationError.Field = %q, want %q", ve.Field, "APIKey")
	}
}

func TestNew_StoreInitError_WrapsWithClientPrefix(t *testing.T) {
	// Use a path that will cause store initialization to fail
	// (directory that doesn't exist and can't be created)
	invalidPath := "/nonexistent/deeply/nested/path/that/cannot/exist/test.db"

	_, err := recall.New(recall.Config{LocalPath: invalidPath})
	if err == nil {
		t.Fatal("New() returned nil error for invalid path")
	}

	// Verify error is wrapped with "client:" prefix
	errStr := err.Error()
	if len(errStr) < 7 || errStr[:7] != "client:" {
		t.Errorf("error should have 'client:' prefix, got: %q", errStr)
	}
}

func TestClient_ConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "concurrent.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	const numGoroutines = 10
	var wg sync.WaitGroup

	// Launch goroutines performing Record concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, err := client.Record("Concurrent lore content", recall.CategoryArchitecturalDecision)
			if err != nil {
				t.Errorf("goroutine %d: Record() error: %v", id, err)
			}
		}(i)
	}

	// ctx is used below for Query
	_ = ctx

	// Launch goroutines performing Query concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, err := client.Query(ctx, recall.QueryParams{
				Query: "concurrent",
				K:     5,
			})
			if err != nil {
				t.Errorf("goroutine %d: Query() error: %v", id, err)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
}

// =============================================================================
// Story 1.4: Record Lore - Acceptance Tests
// =============================================================================

// TestRecord_ValidInputs_ReturnsLoreWithDefaults tests AC #1:
// client.Record(content, category) with valid inputs returns a Lore entry
// with a ULID identifier, default confidence of 0.5, and timestamps set.
func TestRecord_ValidInputs_ReturnsLoreWithDefaults(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer client.Close()

	content := "Always use context.Context as the first parameter"
	category := recall.CategoryPatternOutcome

	lore, err := client.Record(content, category)
	if err != nil {
		t.Fatalf("Record() returned error: %v", err)
	}

	// Verify ULID format (26 characters, alphanumeric)
	if len(lore.ID) != 26 {
		t.Errorf("ID length = %d, want 26 (ULID format)", len(lore.ID))
	}

	// Verify default confidence is 0.5
	if lore.Confidence != 0.5 {
		t.Errorf("Confidence = %f, want 0.5", lore.Confidence)
	}

	// Verify timestamps are set
	if lore.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero, want non-zero timestamp")
	}
	if lore.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero, want non-zero timestamp")
	}

	// Verify content and category stored correctly
	if lore.Content != content {
		t.Errorf("Content = %q, want %q", lore.Content, content)
	}
	if lore.Category != category {
		t.Errorf("Category = %q, want %q", lore.Category, category)
	}
}

// TestRecord_WithContextAndConfidence_StoresAllValues tests AC #2:
// client.Record() with content, category, context, and specific confidence
// stores all provided values.
func TestRecord_WithContextAndConfidence_StoresAllValues(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer client.Close()

	content := "Interface segregation improves testability"
	category := recall.CategoryInterfaceLesson
	ctx := "Discovered during payment service refactoring"
	confidence := 0.85

	lore, err := client.Record(content, category,
		recall.WithContext(ctx),
		recall.WithConfidence(confidence),
	)
	if err != nil {
		t.Fatalf("Record() returned error: %v", err)
	}

	if lore.Content != content {
		t.Errorf("Content = %q, want %q", lore.Content, content)
	}
	if lore.Category != category {
		t.Errorf("Category = %q, want %q", lore.Category, category)
	}
	if lore.Context != ctx {
		t.Errorf("Context = %q, want %q", lore.Context, ctx)
	}
	if lore.Confidence != confidence {
		t.Errorf("Confidence = %f, want %f", lore.Confidence, confidence)
	}
}

// TestRecord_ContentExceedsLimit_ReturnsValidationError tests AC #3:
// Content exceeding 4,000 characters returns ValidationError identifying
// the content field and limit.
func TestRecord_ContentExceedsLimit_ReturnsValidationError(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer client.Close()

	// 4001 characters exceeds limit
	longContent := string(make([]byte, 4001))
	for i := range longContent {
		longContent = longContent[:i] + "x" + longContent[i+1:]
	}
	longContent = strings.Repeat("x", 4001)

	_, err = client.Record(longContent, recall.CategoryArchitecturalDecision)

	var ve *recall.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Record() returned %T, want *ValidationError", err)
	}
	if ve.Field != "Content" {
		t.Errorf("ValidationError.Field = %q, want %q", ve.Field, "Content")
	}
}

// TestRecord_ContentExactlyAtLimit_Succeeds tests edge case:
// Content exactly 4000 characters should succeed.
func TestRecord_ContentExactlyAtLimit_Succeeds(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer client.Close()

	exactContent := strings.Repeat("x", 4000)

	lore, err := client.Record(exactContent, recall.CategoryArchitecturalDecision)
	if err != nil {
		t.Fatalf("Record() with 4000 chars returned error: %v", err)
	}
	if len(lore.Content) != 4000 {
		t.Errorf("Content length = %d, want 4000", len(lore.Content))
	}
}

// TestRecord_ContextExceedsLimit_ReturnsValidationError tests AC #4:
// Context exceeding 1,000 characters returns ValidationError identifying
// the context field and limit.
func TestRecord_ContextExceedsLimit_ReturnsValidationError(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer client.Close()

	longContext := strings.Repeat("y", 1001)

	_, err = client.Record("Valid content", recall.CategoryArchitecturalDecision,
		recall.WithContext(longContext),
	)

	var ve *recall.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Record() returned %T, want *ValidationError", err)
	}
	if ve.Field != "Context" {
		t.Errorf("ValidationError.Field = %q, want %q", ve.Field, "Context")
	}
}

// TestRecord_ContextExactlyAtLimit_Succeeds tests edge case:
// Context exactly 1000 characters should succeed.
func TestRecord_ContextExactlyAtLimit_Succeeds(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer client.Close()

	exactContext := strings.Repeat("y", 1000)

	lore, err := client.Record("Valid content", recall.CategoryArchitecturalDecision,
		recall.WithContext(exactContext),
	)
	if err != nil {
		t.Fatalf("Record() with 1000 char context returned error: %v", err)
	}
	if len(lore.Context) != 1000 {
		t.Errorf("Context length = %d, want 1000", len(lore.Context))
	}
}

// TestRecord_InvalidCategory_ReturnsValidationError tests AC #5:
// Unrecognized category returns ValidationError identifying the category
// field and listing valid categories.
func TestRecord_InvalidCategory_ReturnsValidationError(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer client.Close()

	invalidCategory := recall.Category("INVALID_CATEGORY")

	_, err = client.Record("Valid content", invalidCategory)

	var ve *recall.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Record() returned %T, want *ValidationError", err)
	}
	if ve.Field != "Category" {
		t.Errorf("ValidationError.Field = %q, want %q", ve.Field, "Category")
	}
}

// TestRecord_ConfidenceOutOfRange_ReturnsValidationError tests AC #6:
// Confidence outside [0.0, 1.0] returns ValidationError identifying the
// confidence field and valid range.
func TestRecord_ConfidenceOutOfRange_ReturnsValidationError(t *testing.T) {
	tests := []struct {
		name       string
		confidence float64
	}{
		{"below zero", -0.001},
		{"above one", 1.001},
		{"negative", -0.5},
		{"way above", 2.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			dbPath := filepath.Join(dir, "test.db")

			client, err := recall.New(recall.Config{LocalPath: dbPath})
			if err != nil {
				t.Fatalf("New() returned error: %v", err)
			}
			defer client.Close()

			_, err = client.Record("Valid content", recall.CategoryArchitecturalDecision,
				recall.WithConfidence(tt.confidence),
			)

			var ve *recall.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("Record() returned %T, want *ValidationError", err)
			}
			if ve.Field != "Confidence" {
				t.Errorf("ValidationError.Field = %q, want %q", ve.Field, "Confidence")
			}
		})
	}
}

// TestRecord_ConfidenceBoundaries_Succeed tests edge cases:
// Confidence exactly 0.0 and exactly 1.0 should succeed.
func TestRecord_ConfidenceBoundaries_Succeed(t *testing.T) {
	tests := []struct {
		name       string
		confidence float64
	}{
		{"exactly zero", 0.0},
		{"exactly one", 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			dbPath := filepath.Join(dir, "test.db")

			client, err := recall.New(recall.Config{LocalPath: dbPath})
			if err != nil {
				t.Fatalf("New() returned error: %v", err)
			}
			defer client.Close()

			lore, err := client.Record("Valid content", recall.CategoryArchitecturalDecision,
				recall.WithConfidence(tt.confidence),
			)
			if err != nil {
				t.Fatalf("Record() with confidence %f returned error: %v", tt.confidence, err)
			}
			if lore.Confidence != tt.confidence {
				t.Errorf("Confidence = %f, want %f", lore.Confidence, tt.confidence)
			}
		})
	}
}

// TestRecord_EmptyContent_ReturnsValidationError tests validation:
// Empty content returns ValidationError.
func TestRecord_EmptyContent_ReturnsValidationError(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer client.Close()

	_, err = client.Record("", recall.CategoryArchitecturalDecision)

	var ve *recall.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Record() returned %T, want *ValidationError", err)
	}
	if ve.Field != "Content" {
		t.Errorf("ValidationError.Field = %q, want %q", ve.Field, "Content")
	}
}

