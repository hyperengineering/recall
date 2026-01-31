package mcp

import (
	"fmt"
	"sync"
)

// StoreRef represents a lore entry's location in a specific store.
type StoreRef struct {
	StoreID string
	LoreID  string
}

// MultiStoreSession tracks lore across multiple stores with a global counter.
// Session refs (L1, L2, L3...) increment globally regardless of which store
// the lore came from. This enables agents to reference lore from multiple
// stores in a single session without ambiguity.
type MultiStoreSession struct {
	mu      sync.Mutex
	refs    map[string]StoreRef // session ref (L1, L2) -> StoreRef
	reverse map[string]string   // "storeID:loreID" -> session ref
	counter int
}

// NewMultiStoreSession creates a new multi-store session tracker.
func NewMultiStoreSession() *MultiStoreSession {
	return &MultiStoreSession{
		refs:    make(map[string]StoreRef),
		reverse: make(map[string]string),
	}
}

// Track adds a lore entry from a specific store to the session and returns
// its session reference. The global counter increments regardless of which
// store the lore belongs to.
//
// If the same store+lore combination is tracked again, returns the existing ref.
func (s *MultiStoreSession) Track(storeID, loreID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate reverse key
	key := reverseKey(storeID, loreID)

	// Check if already tracked
	if ref, ok := s.reverse[key]; ok {
		return ref
	}

	// Assign new ref
	s.counter++
	ref := fmt.Sprintf("L%d", s.counter)
	s.refs[ref] = StoreRef{StoreID: storeID, LoreID: loreID}
	s.reverse[key] = ref
	return ref
}

// Resolve converts a session reference to a store/lore pair.
// Returns false if the ref doesn't exist in this session.
func (s *MultiStoreSession) Resolve(ref string) (StoreRef, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	storeRef, ok := s.refs[ref]
	return storeRef, ok
}

// ResolveByLore gets the session reference for a specific store/lore combination.
// Returns false if the combination hasn't been tracked in this session.
func (s *MultiStoreSession) ResolveByLore(storeID, loreID string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := reverseKey(storeID, loreID)
	ref, ok := s.reverse[key]
	return ref, ok
}

// All returns a copy of all tracked session entries.
func (s *MultiStoreSession) All() map[string]StoreRef {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make(map[string]StoreRef, len(s.refs))
	for ref, storeRef := range s.refs {
		result[ref] = storeRef
	}
	return result
}

// Clear resets the session tracking, including the counter.
func (s *MultiStoreSession) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.refs = make(map[string]StoreRef)
	s.reverse = make(map[string]string)
	s.counter = 0
}

// reverseKey generates a unique key for the reverse lookup map.
func reverseKey(storeID, loreID string) string {
	return storeID + ":" + loreID
}
