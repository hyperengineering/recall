package store

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultStoreRoot returns the root directory for all stores.
// Defaults to ~/.recall/stores, falls back to ./.recall/stores if home dir unavailable.
func DefaultStoreRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		// Fallback to current working directory
		cwd, _ := os.Getwd()
		return filepath.Join(cwd, ".recall", "stores")
	}
	return filepath.Join(home, ".recall", "stores")
}

// EncodeStorePath encodes a store ID for filesystem use.
// Replaces "/" with "__" for path-style store IDs.
func EncodeStorePath(storeID string) string {
	return strings.ReplaceAll(storeID, "/", "__")
}

// DecodeStorePath decodes an encoded store path back to store ID.
func DecodeStorePath(encoded string) string {
	return strings.ReplaceAll(encoded, "__", "/")
}

// StoreDBPath returns the full path to a store's database file.
// Example: StoreDBPath("org/team") -> ~/.recall/stores/org__team/lore.db
func StoreDBPath(storeID string) string {
	encoded := EncodeStorePath(storeID)
	return filepath.Join(DefaultStoreRoot(), encoded, "lore.db")
}
