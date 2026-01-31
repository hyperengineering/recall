// Package store provides multi-store management for Recall.
package store

import (
	"errors"
	"regexp"
	"strings"
)

// Store ID validation errors.
var (
	// ErrInvalidStoreID indicates the store ID format is invalid.
	ErrInvalidStoreID = errors.New("invalid store ID: must be lowercase alphanumeric with hyphens, 1-4 path segments")

	// ErrReservedStoreID indicates the store ID is reserved and cannot be created.
	ErrReservedStoreID = errors.New("reserved store ID: cannot create stores with reserved IDs")
)

// storeIDRegex validates store ID format.
// Format: <segment>[/<segment>]*
// - 1-4 path segments separated by /
// - Segments: lowercase alphanumeric and hyphens (a-z, 0-9, -)
// - Segment length: 1-64 characters
// - No leading/trailing hyphens, no consecutive hyphens
// - Total max length: 256 characters
var storeIDRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?(\/[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?){0,3}$`)

// Reserved store IDs that cannot be created (but can be targeted).
var reservedStoreIDs = map[string]bool{
	"default": true,
	"_system": true,
}

// ValidateStoreID validates a store ID format.
// Returns ErrInvalidStoreID if the ID doesn't match the required pattern.
// Reserved IDs (like "_system") are valid for targeting but not creation.
func ValidateStoreID(id string) error {
	if id == "" {
		return ErrInvalidStoreID
	}
	if len(id) > 256 {
		return ErrInvalidStoreID
	}
	// Allow reserved IDs to pass validation (they can be targeted)
	if reservedStoreIDs[id] {
		return nil
	}
	// Check for consecutive hyphens (not caught by regex)
	if strings.Contains(id, "--") {
		return ErrInvalidStoreID
	}
	if !storeIDRegex.MatchString(id) {
		return ErrInvalidStoreID
	}
	return nil
}

// IsReservedStoreID returns true if the store ID is reserved.
func IsReservedStoreID(id string) bool {
	return reservedStoreIDs[id]
}

// ValidateStoreIDForCreation validates a store ID for creation operations.
// Returns error if invalid format or reserved.
func ValidateStoreIDForCreation(id string) error {
	if err := ValidateStoreID(id); err != nil {
		return err
	}
	if IsReservedStoreID(id) {
		return ErrReservedStoreID
	}
	return nil
}
