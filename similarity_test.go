package recall

import (
	"math"
	"testing"
)

// =============================================================================
// Task 1: Pack/Unpack Tests (AC #1, #2)
// =============================================================================

func TestPackFloat32_Empty(t *testing.T) {
	result := PackFloat32([]float32{})
	if len(result) != 0 {
		t.Errorf("PackFloat32([]) = %d bytes, want 0 bytes", len(result))
	}
}

func TestPackFloat32_SingleValue(t *testing.T) {
	result := PackFloat32([]float32{1.0})
	if len(result) != 4 {
		t.Errorf("PackFloat32([1.0]) = %d bytes, want 4 bytes", len(result))
	}
}

func TestPackFloat32_1536Dimensions(t *testing.T) {
	vec := make([]float32, 1536)
	for i := range vec {
		vec[i] = float32(i) * 0.001
	}
	result := PackFloat32(vec)
	expectedLen := 1536 * 4 // 6144 bytes
	if len(result) != expectedLen {
		t.Errorf("PackFloat32(1536 floats) = %d bytes, want %d bytes", len(result), expectedLen)
	}
}

func TestUnpackFloat32_InvalidLength(t *testing.T) {
	result := UnpackFloat32([]byte{1, 2, 3, 4, 5}) // 5 bytes, not divisible by 4
	if result != nil {
		t.Error("UnpackFloat32(5 bytes) should return nil for invalid length")
	}
}

func TestPackUnpack_RoundTrip(t *testing.T) {
	testCases := []struct {
		name string
		vec  []float32
	}{
		{"empty", []float32{}},
		{"single", []float32{1.5}},
		{"three", []float32{1.0, 2.0, 3.0}},
		{"negative", []float32{-1.0, -2.5, 0.0, 3.14}},
		{"1536 dimensions", func() []float32 {
			v := make([]float32, 1536)
			for i := range v {
				v[i] = float32(i) * 0.001
			}
			return v
		}()},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			packed := PackFloat32(tc.vec)
			unpacked := UnpackFloat32(packed)

			if len(unpacked) != len(tc.vec) {
				t.Fatalf("round-trip length mismatch: got %d, want %d", len(unpacked), len(tc.vec))
			}

			for i := range tc.vec {
				if unpacked[i] != tc.vec[i] {
					t.Errorf("round-trip mismatch at index %d: got %v, want %v", i, unpacked[i], tc.vec[i])
				}
			}
		})
	}
}

func TestPackUnpack_SpecialValues(t *testing.T) {
	specialValues := []float32{
		float32(math.NaN()),
		float32(math.Inf(1)),
		float32(math.Inf(-1)),
		0.0,
		float32(math.Copysign(0, -1)), // -0
	}

	packed := PackFloat32(specialValues)
	unpacked := UnpackFloat32(packed)

	if len(unpacked) != len(specialValues) {
		t.Fatalf("special values round-trip length mismatch: got %d, want %d", len(unpacked), len(specialValues))
	}

	// NaN
	if !math.IsNaN(float64(unpacked[0])) {
		t.Errorf("NaN not preserved: got %v", unpacked[0])
	}
	// +Inf
	if !math.IsInf(float64(unpacked[1]), 1) {
		t.Errorf("+Inf not preserved: got %v", unpacked[1])
	}
	// -Inf
	if !math.IsInf(float64(unpacked[2]), -1) {
		t.Errorf("-Inf not preserved: got %v", unpacked[2])
	}
	// 0
	if unpacked[3] != 0.0 {
		t.Errorf("0.0 not preserved: got %v", unpacked[3])
	}
	// -0 (check sign bit)
	if !math.Signbit(float64(unpacked[4])) || unpacked[4] != 0.0 {
		t.Errorf("-0.0 not preserved: got %v, signbit=%v", unpacked[4], math.Signbit(float64(unpacked[4])))
	}
}

// =============================================================================
// Task 2: Types and Interface Tests (AC #3)
// =============================================================================

func TestCandidateLore_HasRequiredFields(t *testing.T) {
	candidate := CandidateLore{
		ID:        "test-id",
		Embedding: []float32{1.0, 2.0, 3.0},
	}

	if candidate.ID != "test-id" {
		t.Errorf("CandidateLore.ID = %q, want %q", candidate.ID, "test-id")
	}
	if len(candidate.Embedding) != 3 {
		t.Errorf("CandidateLore.Embedding length = %d, want 3", len(candidate.Embedding))
	}
}

func TestScoredLore_HasRequiredFields(t *testing.T) {
	scored := ScoredLore{
		ID:    "test-id",
		Score: 0.95,
	}

	if scored.ID != "test-id" {
		t.Errorf("ScoredLore.ID = %q, want %q", scored.ID, "test-id")
	}
	if scored.Score != 0.95 {
		t.Errorf("ScoredLore.Score = %v, want %v", scored.Score, 0.95)
	}
}

func TestSearcher_InterfaceExists(t *testing.T) {
	// Verify BruteForceSearcher implements Searcher interface
	var _ Searcher = (*BruteForceSearcher)(nil)
}

func TestBruteForceSearcher_CanBeInstantiated(t *testing.T) {
	searcher := &BruteForceSearcher{}
	if searcher == nil {
		t.Error("BruteForceSearcher should be instantiatable")
	}
}

// =============================================================================
// Task 3: BruteForceSearcher Tests (AC #4, #5, #6, #7)
// =============================================================================

func TestCosineSimilarity_Identical(t *testing.T) {
	// AC #6: Cosine similarity of two identical embeddings equals 1.0
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	sim := CosineSimilarity(a, b)
	if sim != 1.0 {
		t.Errorf("CosineSimilarity(identical) = %v, want 1.0", sim)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	// AC #7: Cosine similarity of two orthogonal embeddings equals 0.0
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}
	sim := CosineSimilarity(a, b)
	if sim != 0.0 {
		t.Errorf("CosineSimilarity(orthogonal) = %v, want 0.0", sim)
	}
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	a := []float32{1, 0}
	b := []float32{-1, 0}
	sim := CosineSimilarity(a, b)
	if sim != -1.0 {
		t.Errorf("CosineSimilarity(opposite) = %v, want -1.0", sim)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float32{0, 0, 0}
	b := []float32{1, 0, 0}
	sim := CosineSimilarity(a, b)
	if sim != 0.0 {
		t.Errorf("CosineSimilarity(zero, non-zero) = %v, want 0.0", sim)
	}
}

func TestCosineSimilarity_MismatchedLength(t *testing.T) {
	a := []float32{1, 0}
	b := []float32{1, 0, 0}
	sim := CosineSimilarity(a, b)
	if sim != 0.0 {
		t.Errorf("CosineSimilarity(mismatched lengths) = %v, want 0.0", sim)
	}
}

func TestSearch_EmptyCandidates(t *testing.T) {
	searcher := &BruteForceSearcher{}
	query := []float32{1, 0, 0}
	result := searcher.Search(query, []CandidateLore{}, 5)
	if len(result) != 0 {
		t.Errorf("Search(empty candidates) returned %d results, want 0", len(result))
	}
}

func TestSearch_TopK(t *testing.T) {
	// AC #5: Search() with k = 5 and 100 candidates returns only the top 5 results
	searcher := &BruteForceSearcher{}
	query := []float32{1, 0, 0}

	// Create 100 candidates
	candidates := make([]CandidateLore, 100)
	for i := range candidates {
		candidates[i] = CandidateLore{
			ID:        string(rune('a' + i%26)),
			Embedding: []float32{float32(i), float32(100 - i), 0},
		}
	}

	result := searcher.Search(query, candidates, 5)
	if len(result) != 5 {
		t.Errorf("Search(100 candidates, k=5) returned %d results, want 5", len(result))
	}
}

func TestSearch_KGreaterThanCandidates(t *testing.T) {
	searcher := &BruteForceSearcher{}
	query := []float32{1, 0, 0}
	candidates := []CandidateLore{
		{ID: "a", Embedding: []float32{1, 0, 0}},
		{ID: "b", Embedding: []float32{0, 1, 0}},
		{ID: "c", Embedding: []float32{0, 0, 1}},
	}

	result := searcher.Search(query, candidates, 10)
	if len(result) != 3 {
		t.Errorf("Search(3 candidates, k=10) returned %d results, want 3", len(result))
	}
}

func TestSearch_SortedDescending(t *testing.T) {
	// AC #4: BruteForceSearcher implementation ranks results by cosine similarity in descending order
	searcher := &BruteForceSearcher{}
	query := []float32{1, 0, 0}
	candidates := []CandidateLore{
		{ID: "orthogonal", Embedding: []float32{0, 1, 0}},  // similarity = 0
		{ID: "identical", Embedding: []float32{1, 0, 0}},   // similarity = 1
		{ID: "partial", Embedding: []float32{0.5, 0.5, 0}}, // similarity ~= 0.707
	}

	result := searcher.Search(query, candidates, 3)
	if len(result) != 3 {
		t.Fatalf("Search returned %d results, want 3", len(result))
	}

	// Should be sorted: identical (1.0), partial (~0.707), orthogonal (0.0)
	if result[0].ID != "identical" {
		t.Errorf("First result ID = %q, want %q", result[0].ID, "identical")
	}
	if result[1].ID != "partial" {
		t.Errorf("Second result ID = %q, want %q", result[1].ID, "partial")
	}
	if result[2].ID != "orthogonal" {
		t.Errorf("Third result ID = %q, want %q", result[2].ID, "orthogonal")
	}

	// Verify scores are descending
	for i := 1; i < len(result); i++ {
		if result[i].Score > result[i-1].Score {
			t.Errorf("Results not sorted descending: result[%d].Score (%v) > result[%d].Score (%v)",
				i, result[i].Score, i-1, result[i-1].Score)
		}
	}
}

func TestSearch_SkipsEmptyEmbedding(t *testing.T) {
	searcher := &BruteForceSearcher{}
	query := []float32{1, 0, 0}
	candidates := []CandidateLore{
		{ID: "valid", Embedding: []float32{1, 0, 0}},
		{ID: "empty", Embedding: []float32{}},
		{ID: "nil", Embedding: nil},
	}

	result := searcher.Search(query, candidates, 10)
	if len(result) != 1 {
		t.Errorf("Search should skip empty embeddings: got %d results, want 1", len(result))
	}
	if result[0].ID != "valid" {
		t.Errorf("Result ID = %q, want %q", result[0].ID, "valid")
	}
}

func TestSearch_SkipsMismatchedDimensions(t *testing.T) {
	searcher := &BruteForceSearcher{}
	query := []float32{1, 0, 0} // 3 dimensions
	candidates := []CandidateLore{
		{ID: "match", Embedding: []float32{0.5, 0.5, 0}},    // 3 dimensions
		{ID: "mismatch", Embedding: []float32{1, 0, 0, 0}},  // 4 dimensions
		{ID: "short", Embedding: []float32{1, 0}},           // 2 dimensions
	}

	result := searcher.Search(query, candidates, 10)
	if len(result) != 1 {
		t.Errorf("Search should skip mismatched dimensions: got %d results, want 1", len(result))
	}
	if result[0].ID != "match" {
		t.Errorf("Result ID = %q, want %q", result[0].ID, "match")
	}
}

func TestSearch_KZero(t *testing.T) {
	searcher := &BruteForceSearcher{}
	query := []float32{1, 0, 0}
	candidates := []CandidateLore{
		{ID: "a", Embedding: []float32{1, 0, 0}},
		{ID: "b", Embedding: []float32{0, 1, 0}},
		{ID: "c", Embedding: []float32{0, 0, 1}},
	}

	result := searcher.Search(query, candidates, 0)
	if len(result) != 3 {
		t.Errorf("Search(k=0) should return all: got %d results, want 3", len(result))
	}
}

func TestSearch_ScoreIsFloat64(t *testing.T) {
	searcher := &BruteForceSearcher{}
	query := []float32{1, 0, 0}
	candidates := []CandidateLore{
		{ID: "test", Embedding: []float32{1, 0, 0}},
	}

	result := searcher.Search(query, candidates, 1)
	if len(result) != 1 {
		t.Fatalf("Search returned %d results, want 1", len(result))
	}

	// AC #6: identical vectors should have score 1.0
	if result[0].Score != 1.0 {
		t.Errorf("Score for identical vectors = %v, want 1.0", result[0].Score)
	}
}
