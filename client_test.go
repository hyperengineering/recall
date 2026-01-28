package recall_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

// =============================================================================
// Story 2.2: Query Lore - Acceptance Tests
// =============================================================================

// queryTestHelper creates a test client and populates it with test lore entries.
// It uses Store.InsertLore directly to set embeddings (which Client.Record doesn't support).
type queryTestHelper struct {
	t      *testing.T
	client *recall.Client
	store  *recall.Store
}

func newQueryTestHelper(t *testing.T) *queryTestHelper {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Create store directly to insert lore with embeddings
	store, err := recall.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}

	// Create client using the same DB
	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		store.Close()
		t.Fatalf("New() returned error: %v", err)
	}

	return &queryTestHelper{
		t:      t,
		client: client,
		store:  store,
	}
}

func (h *queryTestHelper) close() {
	h.client.Close() // closes client's internal store
	h.store.Close()  // closes our separate store used for test setup
}

// insertLoreWithEmbedding inserts a lore entry with the given embedding vector.
func (h *queryTestHelper) insertLoreWithEmbedding(id, content string, category recall.Category, confidence float64, embedding []float32) {
	h.t.Helper()
	lore := &recall.Lore{
		ID:         id,
		Content:    content,
		Category:   category,
		Confidence: confidence,
		Embedding:  recall.PackFloat32(embedding),
		SourceID:   "test-source",
		CreatedAt:  timeNow(),
		UpdatedAt:  timeNow(),
	}
	if err := h.store.InsertLore(lore); err != nil {
		h.t.Fatalf("InsertLore failed: %v", err)
	}
}

// insertLoreWithoutEmbedding inserts a lore entry without an embedding.
func (h *queryTestHelper) insertLoreWithoutEmbedding(id, content string, category recall.Category, confidence float64) {
	h.t.Helper()
	lore := &recall.Lore{
		ID:         id,
		Content:    content,
		Category:   category,
		Confidence: confidence,
		SourceID:   "test-source",
		CreatedAt:  timeNow(),
		UpdatedAt:  timeNow(),
	}
	if err := h.store.InsertLore(lore); err != nil {
		h.t.Fatalf("InsertLore failed: %v", err)
	}
}

func timeNow() time.Time {
	return time.Now().UTC()
}

// TestQuery_ReturnsResultsRankedBySimilarity tests AC #1:
// Query returns results ranked by semantic similarity (highest first).
func TestQuery_ReturnsResultsRankedBySimilarity(t *testing.T) {
	h := newQueryTestHelper(t)
	defer h.close()

	// Create test embeddings - vectors that have clear similarity relationships
	// Query vector: [1, 0, 0]
	queryVec := []float32{1.0, 0.0, 0.0}

	// High similarity: [0.9, 0.1, 0.0] - close to query
	highSim := []float32{0.9, 0.1, 0.0}
	// Medium similarity: [0.5, 0.5, 0.0] - 45 degrees from query
	medSim := []float32{0.5, 0.5, 0.0}
	// Low similarity: [0.0, 1.0, 0.0] - orthogonal to query
	lowSim := []float32{0.0, 1.0, 0.0}

	// Insert in reverse order to prove sorting works
	h.insertLoreWithEmbedding("low", "Low similarity content", recall.CategoryPatternOutcome, 0.8, lowSim)
	h.insertLoreWithEmbedding("med", "Medium similarity content", recall.CategoryPatternOutcome, 0.8, medSim)
	h.insertLoreWithEmbedding("high", "High similarity content", recall.CategoryPatternOutcome, 0.8, highSim)

	result, err := h.client.Query(context.Background(), recall.QueryParams{
		Query:          "test query",
		QueryEmbedding: queryVec,
		K:              10,
	})
	if err != nil {
		t.Fatalf("Query() returned error: %v", err)
	}

	if len(result.Lore) != 3 {
		t.Fatalf("Query() returned %d results, want 3", len(result.Lore))
	}

	// Verify order: high, med, low (by similarity)
	if result.Lore[0].ID != "high" {
		t.Errorf("First result ID = %q, want %q", result.Lore[0].ID, "high")
	}
	if result.Lore[1].ID != "med" {
		t.Errorf("Second result ID = %q, want %q", result.Lore[1].ID, "med")
	}
	if result.Lore[2].ID != "low" {
		t.Errorf("Third result ID = %q, want %q", result.Lore[2].ID, "low")
	}
}

// TestQuery_TopKLimitsResults tests AC #2:
// TopK=3 returns at most 3 results.
func TestQuery_TopKLimitsResults(t *testing.T) {
	h := newQueryTestHelper(t)
	defer h.close()

	// Insert 5 entries
	queryVec := []float32{1.0, 0.0, 0.0}
	for i := 0; i < 5; i++ {
		id := string(rune('a' + i))
		// Vary the embedding slightly so they have different similarities
		emb := []float32{1.0 - float32(i)*0.1, float32(i) * 0.1, 0.0}
		h.insertLoreWithEmbedding(id, "Content "+id, recall.CategoryPatternOutcome, 0.8, emb)
	}

	result, err := h.client.Query(context.Background(), recall.QueryParams{
		Query:          "test query",
		QueryEmbedding: queryVec,
		K:              3,
	})
	if err != nil {
		t.Fatalf("Query() returned error: %v", err)
	}

	if len(result.Lore) != 3 {
		t.Errorf("Query() returned %d results, want 3", len(result.Lore))
	}
}

// TestQuery_MinConfidenceFiltersResults tests AC #3:
// MinConfidence=0.7 filters entries below 0.7.
func TestQuery_MinConfidenceFiltersResults(t *testing.T) {
	h := newQueryTestHelper(t)
	defer h.close()

	queryVec := []float32{1.0, 0.0, 0.0}
	sameEmb := []float32{1.0, 0.0, 0.0} // Same as query for high similarity

	// Insert entries with varying confidence
	h.insertLoreWithEmbedding("high-conf", "High confidence", recall.CategoryPatternOutcome, 0.9, sameEmb)
	h.insertLoreWithEmbedding("med-conf", "Medium confidence", recall.CategoryPatternOutcome, 0.7, sameEmb)
	h.insertLoreWithEmbedding("low-conf", "Low confidence", recall.CategoryPatternOutcome, 0.5, sameEmb)

	minConf := 0.7
	result, err := h.client.Query(context.Background(), recall.QueryParams{
		Query:          "test query",
		QueryEmbedding: queryVec,
		K:              10,
		MinConfidence:  &minConf,
	})
	if err != nil {
		t.Fatalf("Query() returned error: %v", err)
	}

	// Should only return entries with confidence >= 0.7
	if len(result.Lore) != 2 {
		t.Errorf("Query() returned %d results, want 2", len(result.Lore))
	}

	for _, l := range result.Lore {
		if l.Confidence < 0.7 {
			t.Errorf("Returned lore with confidence %f, want >= 0.7", l.Confidence)
		}
	}
}

// TestQuery_MinConfidenceZeroAllowsAll tests the fix for zero-value bug:
// Explicitly setting MinConfidence=0.0 should allow all entries (not override to 0.5).
func TestQuery_MinConfidenceZeroAllowsAll(t *testing.T) {
	h := newQueryTestHelper(t)
	defer h.close()

	queryVec := []float32{1.0, 0.0, 0.0}
	sameEmb := []float32{1.0, 0.0, 0.0}

	// Insert entries with very low confidence
	h.insertLoreWithEmbedding("low-conf-1", "Very low confidence 1", recall.CategoryPatternOutcome, 0.1, sameEmb)
	h.insertLoreWithEmbedding("low-conf-2", "Very low confidence 2", recall.CategoryPatternOutcome, 0.2, sameEmb)
	h.insertLoreWithEmbedding("high-conf", "High confidence", recall.CategoryPatternOutcome, 0.9, sameEmb)

	minConf := 0.0
	result, err := h.client.Query(context.Background(), recall.QueryParams{
		Query:          "test query",
		QueryEmbedding: queryVec,
		K:              10,
		MinConfidence:  &minConf,
	})
	if err != nil {
		t.Fatalf("Query() returned error: %v", err)
	}

	// With MinConfidence=0.0, should return all 3 entries
	if len(result.Lore) != 3 {
		t.Errorf("Query() with MinConfidence=0.0 returned %d results, want 3", len(result.Lore))
	}
}

// TestQuery_CategoriesFilterWorks tests AC #4:
// Categories filter restricts results to specified categories.
func TestQuery_CategoriesFilterWorks(t *testing.T) {
	h := newQueryTestHelper(t)
	defer h.close()

	queryVec := []float32{1.0, 0.0, 0.0}
	sameEmb := []float32{1.0, 0.0, 0.0}

	// Insert entries with different categories
	h.insertLoreWithEmbedding("arch", "Architectural decision", recall.CategoryArchitecturalDecision, 0.8, sameEmb)
	h.insertLoreWithEmbedding("pattern", "Pattern outcome", recall.CategoryPatternOutcome, 0.8, sameEmb)
	h.insertLoreWithEmbedding("interface", "Interface lesson", recall.CategoryInterfaceLesson, 0.8, sameEmb)

	result, err := h.client.Query(context.Background(), recall.QueryParams{
		Query:          "test query",
		QueryEmbedding: queryVec,
		K:              10,
		Categories:     []recall.Category{recall.CategoryArchitecturalDecision, recall.CategoryPatternOutcome},
	})
	if err != nil {
		t.Fatalf("Query() returned error: %v", err)
	}

	// Should only return entries with specified categories
	if len(result.Lore) != 2 {
		t.Errorf("Query() returned %d results, want 2", len(result.Lore))
	}

	for _, l := range result.Lore {
		if l.Category != recall.CategoryArchitecturalDecision && l.Category != recall.CategoryPatternOutcome {
			t.Errorf("Returned lore with category %q, want ARCHITECTURAL_DECISION or PATTERN_OUTCOME", l.Category)
		}
	}
}

// TestQuery_CombinedFiltersApplyANDLogic tests AC #5:
// Multiple filters (categories + minConfidence + topK) apply AND logic.
func TestQuery_CombinedFiltersApplyANDLogic(t *testing.T) {
	h := newQueryTestHelper(t)
	defer h.close()

	queryVec := []float32{1.0, 0.0, 0.0}
	sameEmb := []float32{1.0, 0.0, 0.0}

	// Insert varied entries
	// Passes all filters:
	h.insertLoreWithEmbedding("pass1", "Passes all 1", recall.CategoryPatternOutcome, 0.8, sameEmb)
	h.insertLoreWithEmbedding("pass2", "Passes all 2", recall.CategoryPatternOutcome, 0.9, sameEmb)
	// Fails category filter:
	h.insertLoreWithEmbedding("fail-cat", "Wrong category", recall.CategoryInterfaceLesson, 0.8, sameEmb)
	// Fails confidence filter:
	h.insertLoreWithEmbedding("fail-conf", "Low confidence", recall.CategoryPatternOutcome, 0.5, sameEmb)

	minConf := 0.7
	result, err := h.client.Query(context.Background(), recall.QueryParams{
		Query:          "test query",
		QueryEmbedding: queryVec,
		K:              10,
		MinConfidence:  &minConf,
		Categories:     []recall.Category{recall.CategoryPatternOutcome},
	})
	if err != nil {
		t.Fatalf("Query() returned error: %v", err)
	}

	// Should only return entries that pass ALL filters
	if len(result.Lore) != 2 {
		t.Errorf("Query() returned %d results, want 2", len(result.Lore))
	}

	for _, l := range result.Lore {
		if l.Category != recall.CategoryPatternOutcome {
			t.Errorf("Returned lore with category %q, want PATTERN_OUTCOME", l.Category)
		}
		if l.Confidence < 0.7 {
			t.Errorf("Returned lore with confidence %f, want >= 0.7", l.Confidence)
		}
	}
}

// TestQuery_NoMatchesReturnsEmptySlice tests AC #6:
// No matches returns an empty slice, not an error.
func TestQuery_NoMatchesReturnsEmptySlice(t *testing.T) {
	h := newQueryTestHelper(t)
	defer h.close()

	// Don't insert any lore
	queryVec := []float32{1.0, 0.0, 0.0}

	result, err := h.client.Query(context.Background(), recall.QueryParams{
		Query:          "test query",
		QueryEmbedding: queryVec,
		K:              10,
	})
	if err != nil {
		t.Fatalf("Query() returned error: %v, want nil", err)
	}

	if result.Lore == nil {
		t.Error("Query() returned nil Lore slice, want empty slice")
	}

	if len(result.Lore) != 0 {
		t.Errorf("Query() returned %d results, want 0", len(result.Lore))
	}
}

// TestQuery_EntriesWithoutEmbeddingsAreExcluded tests AC #7:
// Entries without embeddings are excluded from similarity search results.
func TestQuery_EntriesWithoutEmbeddingsAreExcluded(t *testing.T) {
	h := newQueryTestHelper(t)
	defer h.close()

	queryVec := []float32{1.0, 0.0, 0.0}
	sameEmb := []float32{1.0, 0.0, 0.0}

	// Insert entries with and without embeddings
	h.insertLoreWithEmbedding("with-emb", "Has embedding", recall.CategoryPatternOutcome, 0.8, sameEmb)
	h.insertLoreWithoutEmbedding("no-emb", "No embedding", recall.CategoryPatternOutcome, 0.8)

	result, err := h.client.Query(context.Background(), recall.QueryParams{
		Query:          "test query",
		QueryEmbedding: queryVec,
		K:              10,
	})
	if err != nil {
		t.Fatalf("Query() returned error: %v", err)
	}

	// Should only return the entry with embedding
	if len(result.Lore) != 1 {
		t.Errorf("Query() returned %d results, want 1", len(result.Lore))
	}

	if len(result.Lore) > 0 && result.Lore[0].ID != "with-emb" {
		t.Errorf("Returned lore ID = %q, want %q", result.Lore[0].ID, "with-emb")
	}
}

// TestQuery_SessionRefsAreReturned tests that session refs are properly returned.
func TestQuery_SessionRefsAreReturned(t *testing.T) {
	h := newQueryTestHelper(t)
	defer h.close()

	queryVec := []float32{1.0, 0.0, 0.0}
	sameEmb := []float32{1.0, 0.0, 0.0}

	h.insertLoreWithEmbedding("lore1", "First lore", recall.CategoryPatternOutcome, 0.8, sameEmb)
	h.insertLoreWithEmbedding("lore2", "Second lore", recall.CategoryPatternOutcome, 0.8, sameEmb)

	result, err := h.client.Query(context.Background(), recall.QueryParams{
		Query:          "test query",
		QueryEmbedding: queryVec,
		K:              10,
	})
	if err != nil {
		t.Fatalf("Query() returned error: %v", err)
	}

	if len(result.SessionRefs) != 2 {
		t.Errorf("SessionRefs has %d entries, want 2", len(result.SessionRefs))
	}

	// Verify session refs map to the correct IDs
	foundIDs := make(map[string]bool)
	for _, id := range result.SessionRefs {
		foundIDs[id] = true
	}
	if !foundIDs["lore1"] || !foundIDs["lore2"] {
		t.Errorf("SessionRefs does not contain expected IDs: %v", result.SessionRefs)
	}
}

// TestQuery_ErrorsAreWrapped tests that errors are properly wrapped with context.
func TestQuery_ErrorsAreWrapped(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	client, err := recall.New(recall.Config{LocalPath: dbPath})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	// Close the client to force an error
	client.Close()

	_, err = client.Query(context.Background(), recall.QueryParams{
		Query:          "test",
		QueryEmbedding: []float32{1.0, 0.0, 0.0},
	})
	if err == nil {
		t.Fatal("Query() on closed client returned nil error")
	}

	// Verify error contains "client: query:" prefix
	errStr := err.Error()
	if !strings.Contains(errStr, "client: query:") {
		t.Errorf("Error should contain 'client: query:' prefix, got: %q", errStr)
	}
}

// TestQuery_DefaultsAppliedWhenUnset tests that defaults are applied correctly.
func TestQuery_DefaultsAppliedWhenUnset(t *testing.T) {
	h := newQueryTestHelper(t)
	defer h.close()

	queryVec := []float32{1.0, 0.0, 0.0}
	sameEmb := []float32{1.0, 0.0, 0.0}

	// Insert 10 entries with varying confidence
	for i := 0; i < 10; i++ {
		id := string(rune('a' + i))
		conf := 0.1 * float64(i+1) // 0.1, 0.2, ..., 1.0
		h.insertLoreWithEmbedding(id, "Content "+id, recall.CategoryPatternOutcome, conf, sameEmb)
	}

	// Query without setting K or MinConfidence
	result, err := h.client.Query(context.Background(), recall.QueryParams{
		Query:          "test query",
		QueryEmbedding: queryVec,
	})
	if err != nil {
		t.Fatalf("Query() returned error: %v", err)
	}

	// Default K=5, default MinConfidence=0.5
	// Entries with confidence >= 0.5: f(0.6), g(0.7), h(0.8), i(0.9), j(1.0) = 5 entries
	// With default K=5, should return 5 results
	if len(result.Lore) != 5 {
		t.Errorf("Query() with defaults returned %d results, want 5", len(result.Lore))
	}

	// All returned entries should have confidence >= 0.5
	for _, l := range result.Lore {
		if l.Confidence < 0.5 {
			t.Errorf("Returned lore with confidence %f, want >= 0.5 (default)", l.Confidence)
		}
	}
}

