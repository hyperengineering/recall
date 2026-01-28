package recall

import (
	"fmt"
	"strings"
	"sync"
)

// Session tracks lore surfaced during a single session for feedback purposes.
type Session struct {
	mu      sync.Mutex
	lore    map[string]string // session ref (L1, L2) -> lore ID
	reverse map[string]string // lore ID -> session ref
	counter int
}

// NewSession creates a new session tracker.
func NewSession() *Session {
	return &Session{
		lore:    make(map[string]string),
		reverse: make(map[string]string),
	}
}

// Track adds a lore entry to the session and returns its session reference.
func (s *Session) Track(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already tracked
	if ref, ok := s.reverse[id]; ok {
		return ref
	}

	s.counter++
	ref := fmt.Sprintf("L%d", s.counter)
	s.lore[ref] = id
	s.reverse[id] = ref
	return ref
}

// Resolve converts a session reference to a lore ID.
func (s *Session) Resolve(ref string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, ok := s.lore[ref]
	return id, ok
}

// ResolveByID gets the session reference for a lore ID.
func (s *Session) ResolveByID(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ref, ok := s.reverse[id]
	return ref, ok
}

// All returns all tracked session lore.
func (s *Session) All() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make(map[string]string, len(s.lore))
	for ref, id := range s.lore {
		result[ref] = id
	}
	return result
}

// Count returns the number of lore entries tracked this session.
func (s *Session) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.lore)
}

// Clear resets the session tracking.
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lore = make(map[string]string)
	s.reverse = make(map[string]string)
	s.counter = 0
}

// FuzzyMatch attempts to match a reference string to a tracked lore entry.
// It accepts:
// - Session refs (L1, L2, etc.)
// - Lore IDs directly
// - Content snippets (partial match against stored content)
func (s *Session) FuzzyMatch(ref string, contentLookup func(id string) string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Try direct session ref
	if id, ok := s.lore[ref]; ok {
		return id, true
	}

	// Try as direct lore ID
	if _, ok := s.reverse[ref]; ok {
		return ref, true
	}

	// Try content snippet match
	refLower := strings.ToLower(ref)
	for _, id := range s.lore {
		content := contentLookup(id)
		if strings.Contains(strings.ToLower(content), refLower) {
			return id, true
		}
	}

	return "", false
}
