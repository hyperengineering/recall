package mcp_test

import (
	"testing"

	"github.com/hyperengineering/recall/mcp"
)

// =============================================================================
// MultiStoreSession Unit Tests
// =============================================================================

func TestMultiStoreSession_NewSession(t *testing.T) {
	session := mcp.NewMultiStoreSession()
	if session == nil {
		t.Fatal("NewMultiStoreSession() returned nil")
	}
}

func TestMultiStoreSession_Track_AssignsSequentialRefs(t *testing.T) {
	session := mcp.NewMultiStoreSession()

	ref1 := session.Track("store-a", "lore-abc")
	ref2 := session.Track("store-a", "lore-def")
	ref3 := session.Track("store-a", "lore-ghi")

	if ref1 != "L1" {
		t.Errorf("First track returned %q, want L1", ref1)
	}
	if ref2 != "L2" {
		t.Errorf("Second track returned %q, want L2", ref2)
	}
	if ref3 != "L3" {
		t.Errorf("Third track returned %q, want L3", ref3)
	}
}

func TestMultiStoreSession_Track_GlobalCounterAcrossStores(t *testing.T) {
	// AC #7: Session refs use global counter across all stores
	session := mcp.NewMultiStoreSession()

	// Track 3 from store A
	ref1 := session.Track("store-a", "lore-1")
	ref2 := session.Track("store-a", "lore-2")
	ref3 := session.Track("store-a", "lore-3")

	// Track 2 from store B - counter should continue from L3
	ref4 := session.Track("store-b", "lore-4")
	ref5 := session.Track("store-b", "lore-5")

	if ref1 != "L1" || ref2 != "L2" || ref3 != "L3" {
		t.Errorf("Store A refs = %v, %v, %v; want L1, L2, L3", ref1, ref2, ref3)
	}
	if ref4 != "L4" || ref5 != "L5" {
		t.Errorf("Store B refs = %v, %v; want L4, L5", ref4, ref5)
	}
}

func TestMultiStoreSession_Track_ReturnsSameRefForDuplicate(t *testing.T) {
	session := mcp.NewMultiStoreSession()

	ref1 := session.Track("store-a", "lore-abc")
	ref2 := session.Track("store-a", "lore-abc") // Same store+lore

	if ref1 != ref2 {
		t.Errorf("Duplicate track returned different refs: %q vs %q", ref1, ref2)
	}
	if ref1 != "L1" {
		t.Errorf("Expected L1, got %q", ref1)
	}
}

func TestMultiStoreSession_Track_SameLoreIDDifferentStores(t *testing.T) {
	// Same lore ID in different stores should get different refs
	session := mcp.NewMultiStoreSession()

	refA := session.Track("store-a", "lore-xyz")
	refB := session.Track("store-b", "lore-xyz") // Same lore ID, different store

	if refA == refB {
		t.Errorf("Same lore ID in different stores should get different refs, got %q for both", refA)
	}
	if refA != "L1" || refB != "L2" {
		t.Errorf("Refs = %v, %v; want L1, L2", refA, refB)
	}
}

func TestMultiStoreSession_Resolve_ReturnsStoreRef(t *testing.T) {
	// AC #8: Server internally tracks which store each session ref belongs to
	session := mcp.NewMultiStoreSession()

	session.Track("store-a", "lore-abc")
	session.Track("store-b", "lore-xyz")

	storeRef, ok := session.Resolve("L1")
	if !ok {
		t.Fatal("Resolve(L1) returned false")
	}
	if storeRef.StoreID != "store-a" {
		t.Errorf("Resolve(L1).StoreID = %q, want store-a", storeRef.StoreID)
	}
	if storeRef.LoreID != "lore-abc" {
		t.Errorf("Resolve(L1).LoreID = %q, want lore-abc", storeRef.LoreID)
	}

	storeRef2, ok := session.Resolve("L2")
	if !ok {
		t.Fatal("Resolve(L2) returned false")
	}
	if storeRef2.StoreID != "store-b" {
		t.Errorf("Resolve(L2).StoreID = %q, want store-b", storeRef2.StoreID)
	}
	if storeRef2.LoreID != "lore-xyz" {
		t.Errorf("Resolve(L2).LoreID = %q, want lore-xyz", storeRef2.LoreID)
	}
}

func TestMultiStoreSession_Resolve_NotFound(t *testing.T) {
	session := mcp.NewMultiStoreSession()

	_, ok := session.Resolve("L99")
	if ok {
		t.Error("Resolve(L99) should return false for non-existent ref")
	}
}

func TestMultiStoreSession_ResolveByLore_ReturnsSessionRef(t *testing.T) {
	session := mcp.NewMultiStoreSession()

	session.Track("store-a", "lore-abc")
	session.Track("store-b", "lore-xyz")

	ref, ok := session.ResolveByLore("store-a", "lore-abc")
	if !ok {
		t.Fatal("ResolveByLore(store-a, lore-abc) returned false")
	}
	if ref != "L1" {
		t.Errorf("ResolveByLore(store-a, lore-abc) = %q, want L1", ref)
	}

	ref2, ok := session.ResolveByLore("store-b", "lore-xyz")
	if !ok {
		t.Fatal("ResolveByLore(store-b, lore-xyz) returned false")
	}
	if ref2 != "L2" {
		t.Errorf("ResolveByLore(store-b, lore-xyz) = %q, want L2", ref2)
	}
}

func TestMultiStoreSession_ResolveByLore_NotFound(t *testing.T) {
	session := mcp.NewMultiStoreSession()

	session.Track("store-a", "lore-abc")

	// Wrong store
	_, ok := session.ResolveByLore("store-b", "lore-abc")
	if ok {
		t.Error("ResolveByLore with wrong store should return false")
	}

	// Wrong lore
	_, ok = session.ResolveByLore("store-a", "lore-xyz")
	if ok {
		t.Error("ResolveByLore with wrong lore should return false")
	}
}

func TestMultiStoreSession_All_ReturnsAllEntries(t *testing.T) {
	session := mcp.NewMultiStoreSession()

	session.Track("store-a", "lore-1")
	session.Track("store-b", "lore-2")
	session.Track("store-a", "lore-3")

	all := session.All()
	if len(all) != 3 {
		t.Errorf("All() returned %d entries, want 3", len(all))
	}

	// Verify entries
	if ref, ok := all["L1"]; !ok || ref.StoreID != "store-a" || ref.LoreID != "lore-1" {
		t.Errorf("All()[L1] = %+v, want {store-a, lore-1}", ref)
	}
	if ref, ok := all["L2"]; !ok || ref.StoreID != "store-b" || ref.LoreID != "lore-2" {
		t.Errorf("All()[L2] = %+v, want {store-b, lore-2}", ref)
	}
	if ref, ok := all["L3"]; !ok || ref.StoreID != "store-a" || ref.LoreID != "lore-3" {
		t.Errorf("All()[L3] = %+v, want {store-a, lore-3}", ref)
	}
}

func TestMultiStoreSession_Clear_ResetsSession(t *testing.T) {
	session := mcp.NewMultiStoreSession()

	session.Track("store-a", "lore-1")
	session.Track("store-b", "lore-2")

	session.Clear()

	all := session.All()
	if len(all) != 0 {
		t.Errorf("After Clear(), All() returned %d entries, want 0", len(all))
	}

	// Counter should reset too
	ref := session.Track("store-c", "lore-3")
	if ref != "L1" {
		t.Errorf("After Clear(), first track returned %q, want L1", ref)
	}
}

func TestMultiStoreSession_ConcurrentAccess(t *testing.T) {
	session := mcp.NewMultiStoreSession()
	done := make(chan bool)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 100; j++ {
				session.Track("store", "lore-"+string(rune('a'+n)))
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic and should have some entries
	all := session.All()
	if len(all) == 0 {
		t.Error("Concurrent access resulted in no entries")
	}
}
