package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hyperengineering/recall/internal/store"
)

func TestEncodeStorePath(t *testing.T) {
	tests := []struct {
		name     string
		storeID  string
		expected string
	}{
		{"simple", "my-project", "my-project"},
		{"with slash", "org/team", "org__team"},
		{"deep path", "a/b/c/d", "a__b__c__d"},
		{"no change needed", "project123", "project123"},
		{"multiple slashes", "org/team/project/sub", "org__team__project__sub"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := store.EncodeStorePath(tt.storeID)
			if got != tt.expected {
				t.Errorf("EncodeStorePath(%q) = %q, want %q", tt.storeID, got, tt.expected)
			}
		})
	}
}

func TestDecodeStorePath(t *testing.T) {
	tests := []struct {
		name     string
		encoded  string
		expected string
	}{
		{"simple", "my-project", "my-project"},
		{"with double underscore", "org__team", "org/team"},
		{"deep path", "a__b__c__d", "a/b/c/d"},
		{"no change needed", "project123", "project123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := store.DecodeStorePath(tt.encoded)
			if got != tt.expected {
				t.Errorf("DecodeStorePath(%q) = %q, want %q", tt.encoded, got, tt.expected)
			}
		})
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	testIDs := []string{
		"my-project",
		"org/team",
		"a/b/c/d",
		"project123",
		"long-project-name-with-many-parts/team-name/sub-project",
	}

	for _, id := range testIDs {
		t.Run(id, func(t *testing.T) {
			encoded := store.EncodeStorePath(id)
			decoded := store.DecodeStorePath(encoded)
			if decoded != id {
				t.Errorf("roundtrip failed: %q -> %q -> %q", id, encoded, decoded)
			}
		})
	}
}

func TestDefaultStoreRoot(t *testing.T) {
	root := store.DefaultStoreRoot()

	// Should contain ".recall/stores"
	if !strings.Contains(root, ".recall") {
		t.Errorf("DefaultStoreRoot() = %q, should contain .recall", root)
	}
	if !strings.HasSuffix(root, "stores") {
		t.Errorf("DefaultStoreRoot() = %q, should end with stores", root)
	}

	// Should be an absolute path
	if !filepath.IsAbs(root) {
		t.Errorf("DefaultStoreRoot() = %q, should be absolute path", root)
	}
}

func TestDefaultStoreRoot_RECALL_HOME_Override(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("RECALL_HOME", tmp)

	root := store.DefaultStoreRoot()
	expected := filepath.Join(tmp, "stores")

	if root != expected {
		t.Errorf("DefaultStoreRoot() = %q, want %q", root, expected)
	}
}

func TestDefaultStoreRoot_RECALL_HOME_Empty_FallsBack(t *testing.T) {
	t.Setenv("RECALL_HOME", "")

	root := store.DefaultStoreRoot()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home directory: %v", err)
	}
	expected := filepath.Join(home, ".recall", "stores")

	if root != expected {
		t.Errorf("DefaultStoreRoot() = %q, want %q", root, expected)
	}
}

func TestStoreDBPath_RECALL_HOME_Override(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("RECALL_HOME", tmp)

	got := store.StoreDBPath("org/team")
	expected := filepath.Join(tmp, "stores", "org__team", "lore.db")

	if got != expected {
		t.Errorf("StoreDBPath() = %q, want %q", got, expected)
	}
}

func TestDefaultStoreRoot_UsesHomeDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home directory: %v", err)
	}

	root := store.DefaultStoreRoot()
	expected := filepath.Join(home, ".recall", "stores")

	if root != expected {
		t.Errorf("DefaultStoreRoot() = %q, want %q", root, expected)
	}
}

func TestStoreDBPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home directory: %v", err)
	}

	tests := []struct {
		name     string
		storeID  string
		expected string
	}{
		{
			"simple",
			"my-project",
			filepath.Join(home, ".recall", "stores", "my-project", "lore.db"),
		},
		{
			"with slash",
			"org/team",
			filepath.Join(home, ".recall", "stores", "org__team", "lore.db"),
		},
		{
			"deep path",
			"a/b/c/d",
			filepath.Join(home, ".recall", "stores", "a__b__c__d", "lore.db"),
		},
		{
			"default store",
			"default",
			filepath.Join(home, ".recall", "stores", "default", "lore.db"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := store.StoreDBPath(tt.storeID)
			if got != tt.expected {
				t.Errorf("StoreDBPath(%q) = %q, want %q", tt.storeID, got, tt.expected)
			}
		})
	}
}

func TestStoreDBPath_EndsWithLoreDB(t *testing.T) {
	path := store.StoreDBPath("any-store")
	if !strings.HasSuffix(path, "lore.db") {
		t.Errorf("StoreDBPath() = %q, should end with lore.db", path)
	}
}
