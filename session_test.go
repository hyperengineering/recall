package recall

import (
	"fmt"
	"sync"
	"testing"
)

// =============================================================================
// Story 2.3: Session Tracking - White-box Tests
// =============================================================================

// TestSession_Track_AssignsSequentialRefs tests AC #1:
// A query returning 3 lore entries assigns sequential session references L1, L2, L3.
func TestSession_Track_AssignsSequentialRefs(t *testing.T) {
	s := NewSession()

	ref1 := s.Track("lore-a")
	ref2 := s.Track("lore-b")
	ref3 := s.Track("lore-c")

	if ref1 != "L1" {
		t.Errorf("First Track() returned %q, want %q", ref1, "L1")
	}
	if ref2 != "L2" {
		t.Errorf("Second Track() returned %q, want %q", ref2, "L2")
	}
	if ref3 != "L3" {
		t.Errorf("Third Track() returned %q, want %q", ref3, "L3")
	}
}

// TestSession_Track_ContinuesSequenceAcrossQueries tests AC #2:
// A second query with new (unseen) entries continues the sequence L4, L5.
func TestSession_Track_ContinuesSequenceAcrossQueries(t *testing.T) {
	s := NewSession()

	// First "query" - 3 entries
	s.Track("lore-a")
	s.Track("lore-b")
	s.Track("lore-c")

	// Second "query" - 2 new entries
	ref4 := s.Track("lore-d")
	ref5 := s.Track("lore-e")

	if ref4 != "L4" {
		t.Errorf("Fourth Track() returned %q, want %q", ref4, "L4")
	}
	if ref5 != "L5" {
		t.Errorf("Fifth Track() returned %q, want %q", ref5, "L5")
	}
}

// TestSession_Track_SameLoreRetainsOriginalRef tests AC #3:
// The same lore entry appearing in multiple queries retains its original L-ref.
func TestSession_Track_SameLoreRetainsOriginalRef(t *testing.T) {
	s := NewSession()

	// First query: A, B, C → L1, L2, L3
	s.Track("lore-a")
	s.Track("lore-b")
	s.Track("lore-c")

	// Second query: A (repeat), D → L1 (reused), L4
	refA2 := s.Track("lore-a") // Should return L1, not L4
	refD := s.Track("lore-d")

	if refA2 != "L1" {
		t.Errorf("Re-tracking lore-a returned %q, want %q (original ref)", refA2, "L1")
	}
	if refD != "L4" {
		t.Errorf("New lore-d returned %q, want %q", refD, "L4")
	}

	// Counter should be 4, not 5
	if s.Count() != 4 {
		t.Errorf("Count() = %d, want 4 (lore-a tracked once)", s.Count())
	}
}

// TestSession_Resolve_ValidRef tests AC #5:
// A session reference like "L3" resolves to the correct lore ID.
func TestSession_Resolve_ValidRef(t *testing.T) {
	s := NewSession()

	s.Track("lore-a")
	s.Track("lore-b")
	s.Track("lore-c")

	id, ok := s.Resolve("L3")
	if !ok {
		t.Error("Resolve(L3) returned false, want true")
	}
	if id != "lore-c" {
		t.Errorf("Resolve(L3) returned %q, want %q", id, "lore-c")
	}
}

// TestSession_Resolve_InvalidRef tests error case:
// Resolving a non-existent ref returns false.
func TestSession_Resolve_InvalidRef(t *testing.T) {
	s := NewSession()

	s.Track("lore-a")

	_, ok := s.Resolve("L99")
	if ok {
		t.Error("Resolve(L99) returned true, want false for non-existent ref")
	}
}

// TestSession_Resolve_EmptySession tests edge case:
// Resolving on empty session returns false.
func TestSession_Resolve_EmptySession(t *testing.T) {
	s := NewSession()

	_, ok := s.Resolve("L1")
	if ok {
		t.Error("Resolve(L1) on empty session returned true, want false")
	}
}

// TestSession_ResolveByID_ValidID tests reverse lookup:
// ResolveByID returns the correct session ref for a tracked lore ID.
func TestSession_ResolveByID_ValidID(t *testing.T) {
	s := NewSession()

	s.Track("lore-a")
	s.Track("lore-b")
	s.Track("lore-c")

	ref, ok := s.ResolveByID("lore-b")
	if !ok {
		t.Error("ResolveByID(lore-b) returned false, want true")
	}
	if ref != "L2" {
		t.Errorf("ResolveByID(lore-b) returned %q, want %q", ref, "L2")
	}
}

// TestSession_ResolveByID_InvalidID tests error case:
// ResolveByID for non-existent ID returns false.
func TestSession_ResolveByID_InvalidID(t *testing.T) {
	s := NewSession()

	s.Track("lore-a")

	_, ok := s.ResolveByID("nonexistent")
	if ok {
		t.Error("ResolveByID(nonexistent) returned true, want false")
	}
}

// TestSession_All_ReturnsAllMappings tests AC #4:
// All() returns all lore surfaced during the session with their L-ref mappings.
func TestSession_All_ReturnsAllMappings(t *testing.T) {
	s := NewSession()

	s.Track("lore-a")
	s.Track("lore-b")
	s.Track("lore-c")

	all := s.All()

	if len(all) != 3 {
		t.Errorf("All() returned %d entries, want 3", len(all))
	}

	expected := map[string]string{
		"L1": "lore-a",
		"L2": "lore-b",
		"L3": "lore-c",
	}

	for ref, wantID := range expected {
		gotID, ok := all[ref]
		if !ok {
			t.Errorf("All() missing ref %q", ref)
			continue
		}
		if gotID != wantID {
			t.Errorf("All()[%q] = %q, want %q", ref, gotID, wantID)
		}
	}
}

// TestSession_All_ReturnsDefensiveCopy tests that All() returns a copy.
func TestSession_All_ReturnsDefensiveCopy(t *testing.T) {
	s := NewSession()

	s.Track("lore-a")

	all := s.All()
	all["L1"] = "modified" // Modify returned map

	// Original should be unchanged
	id, ok := s.Resolve("L1")
	if !ok || id != "lore-a" {
		t.Error("Modifying All() result affected internal state")
	}
}

// TestSession_All_EmptySession tests edge case:
// All() on empty session returns empty map.
func TestSession_All_EmptySession(t *testing.T) {
	s := NewSession()

	all := s.All()

	if all == nil {
		t.Error("All() returned nil, want empty map")
	}
	if len(all) != 0 {
		t.Errorf("All() returned %d entries, want 0", len(all))
	}
}

// TestSession_Count tests Count() method.
func TestSession_Count(t *testing.T) {
	s := NewSession()

	if s.Count() != 0 {
		t.Errorf("Count() on new session = %d, want 0", s.Count())
	}

	s.Track("lore-a")
	s.Track("lore-b")

	if s.Count() != 2 {
		t.Errorf("Count() after 2 tracks = %d, want 2", s.Count())
	}

	// Re-tracking shouldn't increase count
	s.Track("lore-a")
	if s.Count() != 2 {
		t.Errorf("Count() after re-track = %d, want 2 (no change)", s.Count())
	}
}

// TestSession_Clear_ResetsState tests Clear() method:
// Clear resets all session state.
func TestSession_Clear_ResetsState(t *testing.T) {
	s := NewSession()

	s.Track("lore-a")
	s.Track("lore-b")
	s.Track("lore-c")

	s.Clear()

	// Count should be 0
	if s.Count() != 0 {
		t.Errorf("Count() after Clear() = %d, want 0", s.Count())
	}

	// All should be empty
	if len(s.All()) != 0 {
		t.Errorf("All() after Clear() has %d entries, want 0", len(s.All()))
	}

	// Resolve should fail
	_, ok := s.Resolve("L1")
	if ok {
		t.Error("Resolve(L1) after Clear() returned true, want false")
	}
}

// TestSession_Clear_RestartsCounter tests that Clear() resets the counter.
func TestSession_Clear_RestartsCounter(t *testing.T) {
	s := NewSession()

	s.Track("lore-a")
	s.Track("lore-b")
	s.Track("lore-c")

	s.Clear()

	// New tracks should start from L1 again
	ref := s.Track("lore-new")
	if ref != "L1" {
		t.Errorf("First Track() after Clear() returned %q, want %q", ref, "L1")
	}
}

// TestSession_FuzzyMatch_DirectRef tests FuzzyMatch with session refs.
func TestSession_FuzzyMatch_DirectRef(t *testing.T) {
	s := NewSession()

	s.Track("lore-a")
	s.Track("lore-b")

	contentLookup := func(id string) string {
		return "content for " + id
	}

	id, ok := s.FuzzyMatch("L2", contentLookup)
	if !ok {
		t.Error("FuzzyMatch(L2) returned false, want true")
	}
	if id != "lore-b" {
		t.Errorf("FuzzyMatch(L2) returned %q, want %q", id, "lore-b")
	}
}

// TestSession_FuzzyMatch_DirectID tests FuzzyMatch with direct lore ID.
func TestSession_FuzzyMatch_DirectID(t *testing.T) {
	s := NewSession()

	s.Track("lore-a")
	s.Track("lore-b")

	contentLookup := func(id string) string {
		return "content for " + id
	}

	id, ok := s.FuzzyMatch("lore-b", contentLookup)
	if !ok {
		t.Error("FuzzyMatch(lore-b) returned false, want true")
	}
	if id != "lore-b" {
		t.Errorf("FuzzyMatch(lore-b) returned %q, want %q", id, "lore-b")
	}
}

// TestSession_FuzzyMatch_ContentSnippet tests FuzzyMatch with content snippet.
func TestSession_FuzzyMatch_ContentSnippet(t *testing.T) {
	s := NewSession()

	s.Track("lore-a")
	s.Track("lore-b")

	contentLookup := func(id string) string {
		switch id {
		case "lore-a":
			return "Always use context.Context as the first parameter"
		case "lore-b":
			return "Interface segregation improves testability"
		default:
			return ""
		}
	}

	// Search by content snippet (case insensitive)
	id, ok := s.FuzzyMatch("interface segregation", contentLookup)
	if !ok {
		t.Error("FuzzyMatch('interface segregation') returned false, want true")
	}
	if id != "lore-b" {
		t.Errorf("FuzzyMatch('interface segregation') returned %q, want %q", id, "lore-b")
	}
}

// TestSession_FuzzyMatch_CaseInsensitive tests FuzzyMatch case insensitivity.
func TestSession_FuzzyMatch_CaseInsensitive(t *testing.T) {
	s := NewSession()

	s.Track("lore-a")

	contentLookup := func(id string) string {
		return "Always use Context.Context"
	}

	id, ok := s.FuzzyMatch("CONTEXT.CONTEXT", contentLookup)
	if !ok {
		t.Error("FuzzyMatch (case insensitive) returned false, want true")
	}
	if id != "lore-a" {
		t.Errorf("FuzzyMatch (case insensitive) returned %q, want %q", id, "lore-a")
	}
}

// TestSession_FuzzyMatch_NoMatch tests FuzzyMatch when nothing matches.
func TestSession_FuzzyMatch_NoMatch(t *testing.T) {
	s := NewSession()

	s.Track("lore-a")

	contentLookup := func(id string) string {
		return "Some content"
	}

	_, ok := s.FuzzyMatch("nonexistent pattern", contentLookup)
	if ok {
		t.Error("FuzzyMatch('nonexistent pattern') returned true, want false")
	}
}

// TestSession_FuzzyMatch_EmptySession tests FuzzyMatch on empty session.
func TestSession_FuzzyMatch_EmptySession(t *testing.T) {
	s := NewSession()

	contentLookup := func(id string) string {
		return "content"
	}

	_, ok := s.FuzzyMatch("L1", contentLookup)
	if ok {
		t.Error("FuzzyMatch on empty session returned true, want false")
	}
}

// TestSession_FuzzyMatch_PrioritizesExactMatches tests that direct matches
// take priority over content matches.
func TestSession_FuzzyMatch_PrioritizesExactMatches(t *testing.T) {
	s := NewSession()

	// Track a lore with ID that could be confused with a ref
	s.Track("L2") // Actual lore ID is "L2"

	contentLookup := func(id string) string {
		return "content mentioning L2"
	}

	// FuzzyMatch should find direct match first
	id, ok := s.FuzzyMatch("L2", contentLookup)
	if !ok {
		t.Error("FuzzyMatch(L2) returned false, want true")
	}
	// Since "L2" is both a session ref (L1 → "L2") and we're searching "L2",
	// it should resolve via the direct lore ID path
	if id != "L2" {
		t.Errorf("FuzzyMatch(L2) returned %q, want %q", id, "L2")
	}
}

// =============================================================================
// Concurrency Tests (AC #6)
// =============================================================================

// TestSession_Track_ConcurrentAccessIsRaceFree tests AC #6:
// Multiple goroutines calling Track() concurrently assign L-refs sequentially
// and race-free (protected by mutex). Run with: go test -race
func TestSession_Track_ConcurrentAccessIsRaceFree(t *testing.T) {
	s := NewSession()
	const numGoroutines = 100

	var wg sync.WaitGroup
	results := make(chan string, numGoroutines)

	// Launch goroutines that all call Track concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			loreID := fmt.Sprintf("lore-%d", id)
			ref := s.Track(loreID)
			results <- ref
		}(i)
	}

	wg.Wait()
	close(results)

	// Collect all refs
	refs := make(map[string]bool)
	for ref := range results {
		refs[ref] = true
	}

	// Should have exactly numGoroutines unique refs
	if len(refs) != numGoroutines {
		t.Errorf("Got %d unique refs, want %d", len(refs), numGoroutines)
	}

	// All refs should be L1 through L100
	for i := 1; i <= numGoroutines; i++ {
		expected := fmt.Sprintf("L%d", i)
		if !refs[expected] {
			t.Errorf("Missing expected ref %q", expected)
		}
	}
}

// TestSession_ConcurrentReadWrite tests concurrent reads and writes.
func TestSession_ConcurrentReadWrite(t *testing.T) {
	s := NewSession()
	const numOps = 50

	var wg sync.WaitGroup

	// Writers
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			s.Track(fmt.Sprintf("lore-%d", id))
		}(i)
	}

	// Readers - Resolve
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Resolve("L1")
		}()
	}

	// Readers - All
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.All()
		}()
	}

	// Readers - Count
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Count()
		}()
	}

	wg.Wait()

	// Just verify no panics occurred - race detector will catch data races
	if s.Count() != numOps {
		t.Errorf("Count() = %d, want %d", s.Count(), numOps)
	}
}

// TestSession_Track_RetrackingIsConcurrencySafe tests that re-tracking
// the same lore ID from multiple goroutines is safe.
func TestSession_Track_RetrackingIsConcurrencySafe(t *testing.T) {
	s := NewSession()
	const numGoroutines = 100

	// Pre-track one lore entry
	initialRef := s.Track("shared-lore")

	var wg sync.WaitGroup
	results := make(chan string, numGoroutines)

	// All goroutines try to track the same lore ID
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ref := s.Track("shared-lore")
			results <- ref
		}()
	}

	wg.Wait()
	close(results)

	// All should return the same ref
	for ref := range results {
		if ref != initialRef {
			t.Errorf("Re-tracking returned %q, want %q (original)", ref, initialRef)
		}
	}

	// Count should be 1
	if s.Count() != 1 {
		t.Errorf("Count() = %d, want 1", s.Count())
	}
}
